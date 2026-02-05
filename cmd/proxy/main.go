package main

import (
	"embed"
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
