package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"ruoyi-proxy/internal/buildinfo"
	"ruoyi-proxy/internal/cli"
	"ruoyi-proxy/internal/config"
	"ruoyi-proxy/internal/hub"
	"ruoyi-proxy/internal/proxy"
	"ruoyi-proxy/internal/spokecontrol"
	"ruoyi-proxy/internal/taskruntime"
)

//go:embed scripts/*
var scriptsFS embed.FS

//go:embed configs/*
var configsFS embed.FS

func main() {
	mode := ""
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if mode == "spoke-agent" {
		runSpokeAgent()
		return
	}

	// Spoke 包默认进入 Agent/CLI；代理必须通过 /proxy-start 显式启动。
	if mode == "cli" || (mode == "" && buildinfo.IsSpoke()) {
		runCLI()
		return
	}
	if err := runProxy(); err != nil {
		log.Fatalf("代理服务异常退出: %v", err)
	}
}

func runSpokeAgent() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := spokecontrol.RunStandalone(ctx, log.Printf); err != nil {
		log.Fatalf("Spoke 常驻执行器异常退出: %v", err)
	}
}

func runCLI() {
	// 注入嵌入的文件系统
	cli.SetEmbedFS(scriptsFS, configsFS)

	// 启动交互式CLI
	c := cli.New()
	c.Start()
}

func runProxy() error {
	log.Println("蓝绿代理程序启动中...")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hubSettings, _ := hub.LoadHubSettings()
	hubActive := hubSettings.Enabled || buildinfo.IsHub()
	if hubActive {
		if err := hub.LoadSpokes(); err != nil {
			log.Printf("加载 Hub spoke 注册表失败: %v", err)
		} else {
			log.Println("Hub AI 网关已启用")
		}
	}

	p, err := proxy.New()
	if err != nil {
		return fmt.Errorf("代理初始化失败: %v", err)
	}

	var background sync.WaitGroup
	background.Add(1)
	go func() {
		defer background.Done()
		if runErr := taskruntime.Run(ctx, log.Printf); runErr != nil && ctx.Err() == nil {
			log.Printf("AI 定时任务调度器退出: %v", runErr)
		}
	}()

	// 已运行代理的 Spoke 直接由代理进程领取 Hub 任务，无需额外常驻服务。
	if buildinfo.IsSpoke() {
		background.Add(1)
		go func() {
			defer background.Done()
			if runErr := spokecontrol.RunProxyOwned(ctx, log.Printf); runErr != nil && ctx.Err() == nil {
				log.Printf("代理内置 Spoke 远程控制退出: %v", runErr)
			}
		}()
	}

	proxyServer := newProxyServer(p, hubActive)
	mgmtServer := newMgmtServer(p, hubActive)
	serverErrors := make(chan error, 2)
	go serveHTTPServer(proxyServer, "代理服务器", serverErrors)
	go serveHTTPServer(mgmtServer, "管理服务器", serverErrors)

	log.Printf("代理服务器启动在端口 %s", config.ProxyPort)
	log.Printf("nginx upstream配置: server 127.0.0.1%s;", config.ProxyPort)
	log.Printf("管理服务器启动在端口 %s", config.MgmtPort)

	var exitErr error
	select {
	case serverErr := <-serverErrors:
		log.Printf("服务异常退出: %v", serverErr)
		exitErr = serverErr
		stop()
	case <-ctx.Done():
		stop()
		log.Println("收到退出信号，正在停止代理服务...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	shutdownServers(shutdownCtx, proxyServer, mgmtServer)

	backgroundDone := make(chan struct{})
	go func() {
		background.Wait()
		close(backgroundDone)
	}()
	select {
	case <-backgroundDone:
		log.Println("代理服务已安全停止")
	case <-shutdownCtx.Done():
		log.Printf("等待后台任务退出超时: %v", shutdownCtx.Err())
	}
	return exitErr
}

// newProxyServer 创建公开代理服务器。
func newProxyServer(p *proxy.Proxy, hubEnabled bool) *http.Server {
	proxyMux := http.NewServeMux()
	proxyMux.HandleFunc("/", p.HandleProxy)
	if hubEnabled {
		proxyMux.HandleFunc("/__hub__/v1/token", hub.RegisterTokenHandler)
		proxyMux.HandleFunc("/__hub__/v1/register", hub.RegisterHandler)
		proxyMux.HandleFunc("/__hub__/v1/profile", hub.ProfileHandler)
		proxyMux.HandleFunc("/__hub__/v1/chat", hub.ChatHandler)
		proxyMux.HandleFunc("/__hub__/v1/control/poll", hub.ControlPollHandler)
		proxyMux.HandleFunc("/__hub__/v1/control/result", hub.ControlResultHandler)
	}

	return &http.Server{
		Addr:    config.ProxyPort,
		Handler: proxyMux,
		// 超时设置：支持长时间请求与SSE，避免被代理提前断开。
		ReadTimeout:       900 * time.Second,
		WriteTimeout:      900 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 30 * time.Second,
	}
}

// newMgmtServer 创建仅监听回环地址的管理服务器。
func newMgmtServer(p *proxy.Proxy, hubEnabled bool) *http.Server {
	mgmtMux := http.NewServeMux()
	mgmtMux.HandleFunc("/switch", func(w http.ResponseWriter, r *http.Request) {
		handleSwitch(p, w, r)
	})
	mgmtMux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		handleStatus(p, w, r)
	})
	if hubEnabled {
		mgmtMux.HandleFunc("/hub/token", hub.TokenAdminHandler)
		mgmtMux.HandleFunc("/hub/status", hub.StatusAdminHandler)
		mgmtMux.HandleFunc("/hub/spoke", hub.SpokeAdminHandler)
		mgmtMux.HandleFunc("/hub/spoke/governance", hub.SpokeGovernanceAdminHandler)
		mgmtMux.HandleFunc("/hub/revoke", hub.RevokeAdminHandler)
		mgmtMux.HandleFunc("/hub/control", hub.ControlEnqueueAdminHandler)
		mgmtMux.HandleFunc("/hub/control/cancel", hub.ControlCancelAdminHandler)
		mgmtMux.HandleFunc("/hub/control/retry", hub.ControlRetryAdminHandler)
		mgmtMux.HandleFunc("/hub/jobs", hub.ControlJobsAdminHandler)
	}
	return &http.Server{Addr: "127.0.0.1" + config.MgmtPort, Handler: mgmtMux}
}

func serveHTTPServer(server *http.Server, name string, errorCh chan<- error) {
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errorCh <- fmt.Errorf("%s启动失败: %v", name, err)
	}
}

func shutdownServers(ctx context.Context, servers ...*http.Server) {
	var shutdown sync.WaitGroup
	for _, server := range servers {
		if server == nil {
			continue
		}
		shutdown.Add(1)
		go func(current *http.Server) {
			defer shutdown.Done()
			if err := current.Shutdown(ctx); err != nil {
				log.Printf("停止 HTTP 服务失败，将强制关闭: %v", err)
				if closeErr := current.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
					log.Printf("强制关闭 HTTP 服务失败: %v", closeErr)
				}
			}
		}(server)
	}
	shutdown.Wait()
}

// handleSwitch 处理切换环境请求
func handleSwitch(p *proxy.Proxy, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许POST请求", http.StatusMethodNotAllowed)
		return
	}

	// 获取目标环境
	env := r.URL.Query().Get("env")
	if env != "blue" && env != "green" {
		http.Error(w, "无效的环境参数，必须是 blue 或 green", http.StatusBadRequest)
		return
	}

	// 获取服务ID（可选，如果不指定则切换所有服务）
	serviceID := r.URL.Query().Get("service")

	if serviceID != "" {
		// 切换指定服务
		if err := p.SwitchService(serviceID, env); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("服务[%s]已切换到 %s 环境", serviceID, env)
	} else {
		// 切换所有服务
		if err := p.SwitchAll(env); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("所有服务已切换到 %s 环境", env)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"env":    env,
	})
}

// handleStatus 处理状态查询请求
func handleStatus(p *proxy.Proxy, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只允许GET请求", http.StatusMethodNotAllowed)
		return
	}

	cfg := p.GetConfig()
	services := make(map[string]interface{})

	for id, svc := range cfg.Services {
		services[id] = map[string]string{
			"name":         svc.Name,
			"active_env":   svc.ActiveEnv,
			"blue_target":  svc.BlueTarget,
			"green_target": svc.GreenTarget,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "running",
		"services": services,
	})
}
