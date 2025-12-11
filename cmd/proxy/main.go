package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"time"

	"ruoyi-proxy/internal/cli"
	"ruoyi-proxy/internal/config"
	"ruoyi-proxy/internal/handler"
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

	// 创建处理器
	h := handler.New(p)

	// 启动管理服务器
	go startManagementServer(h)

	// 启动代理服务器
	startProxyServer(p)
}

// startManagementServer 启动管理服务器
func startManagementServer(h *handler.Handler) {
	mgmtMux := http.NewServeMux()
	mgmtMux.HandleFunc("/status", h.HandleStatus)                // 获取状态 ?service=xxx 可选
	mgmtMux.HandleFunc("/switch", h.HandleSwitch)                // 切换环境 ?service=xxx&env=blue|green
	mgmtMux.HandleFunc("/config", h.HandleUpdateConfig)          // 更新配置
	mgmtMux.HandleFunc("/health", h.HandleHealth)                // 健康检查
	mgmtMux.HandleFunc("/health/", h.HandleServiceHealth)        // 服务健康检查 /health/{serviceID}
	mgmtMux.HandleFunc("/services", h.HandleServices)            // 获取服务列表
	mgmtMux.HandleFunc("/service/add", h.HandleAddService)       // 添加服务
	mgmtMux.HandleFunc("/service/remove", h.HandleRemoveService) // 删除服务

	mgmtServer := &http.Server{
		Addr:              config.MgmtPort,
		Handler:           mgmtMux,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       15 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
	}

	log.Printf("管理接口启动在端口 %s", config.MgmtPort)
	log.Printf("可用接口:")
	log.Printf("  GET  %s/status              - 查看所有服务状态", config.MgmtPort)
	log.Printf("  GET  %s/status?service=xxx  - 查看指定服务状态", config.MgmtPort)
	log.Printf("  POST %s/switch?env=xxx      - 切换所有服务环境", config.MgmtPort)
	log.Printf("  POST %s/switch?service=xxx&env=xxx - 切换指定服务环境", config.MgmtPort)
	log.Printf("  GET  %s/services            - 获取服务列表", config.MgmtPort)
	log.Printf("  POST %s/service/add         - 添加服务", config.MgmtPort)
	log.Printf("  DELETE %s/service/remove?id=xxx - 删除服务", config.MgmtPort)
	log.Printf("  GET  %s/health              - 健康检查", config.MgmtPort)

	if err := mgmtServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("管理服务器启动失败: %v", err)
	}
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
