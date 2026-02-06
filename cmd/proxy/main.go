package main

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"ruoyi-proxy/internal/cli"
	"ruoyi-proxy/internal/config"
	"ruoyi-proxy/internal/proxy"
)

//go:embed scripts/*
var scriptsFS embed.FS

//go:embed configs/*
var configsFS embed.FS

func main() {
	// 检查是否是CLI模式
	if len(os.Args) > 1 && os.Args[1] == "cli" {
		// 注入嵌入的文件系统
		cli.SetEmbedFS(scriptsFS, configsFS)

		// 启动交互式CLI
		c := cli.New()
		c.Start()
		return
	}

	log.Println("蓝绿代理程序启动中...")

	// 初始化代理
	p, err := proxy.New()
	if err != nil {
		log.Fatalf("代理初始化失败: %v", err)
	}

	// 启动管理服务器（在后台goroutine中）
	go startMgmtServer(p)

	// 启动代理服务器
	startProxyServer(p)
}

// startProxyServer 启动代理服务器
func startProxyServer(p *proxy.Proxy) {
	proxyMux := http.NewServeMux()
	proxyMux.HandleFunc("/", p.HandleProxy)

	proxyServer := &http.Server{
		Addr:    config.ProxyPort,
		Handler: proxyMux,
		// 超时设置：支持长时间请求与SSE，避免被代理提前断开
		ReadTimeout:       900 * time.Second, // 读取请求体超时
		WriteTimeout:      900 * time.Second, // 写入响应超时（流式或长响应建议较大或0）
		IdleTimeout:       120 * time.Second, // 空闲连接超时
		ReadHeaderTimeout: 30 * time.Second,  // 读取请求头超时
	}

	log.Printf("代理服务器启动在端口 %s", config.ProxyPort)
	log.Printf("nginx upstream配置: server 127.0.0.1%s;", config.ProxyPort)

	if err := proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("代理服务器启动失败: %v", err)
	}
}

// startMgmtServer 启动管理服务器
func startMgmtServer(p *proxy.Proxy) {
	mgmtMux := http.NewServeMux()
	mgmtMux.HandleFunc("/switch", func(w http.ResponseWriter, r *http.Request) {
		handleSwitch(p, w, r)
	})
	mgmtMux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		handleStatus(p, w, r)
	})

	mgmtServer := &http.Server{
		Addr:    config.MgmtPort,
		Handler: mgmtMux,
	}

	log.Printf("管理服务器启动在端口 %s", config.MgmtPort)

	if err := mgmtServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("管理服务器启动失败: %v", err)
	}
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
