package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"ruoyi-proxy/internal/config"
)

func TestServerAddressesRespectTrustBoundary(t *testing.T) {
	proxyServer := newProxyServer(nil, false)
	if proxyServer.Addr != config.ProxyPort {
		t.Fatalf("代理监听地址=%q，期望 %q", proxyServer.Addr, config.ProxyPort)
	}
	mgmtServer := newMgmtServer(nil, false)
	if mgmtServer.Addr != "127.0.0.1"+config.MgmtPort {
		t.Fatalf("管理监听地址=%q，期望仅监听回环地址", mgmtServer.Addr)
	}
}

func TestShutdownServersStopsActiveServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建监听器失败: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(listener)
	}()

	response, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("请求测试服务失败: %v", err)
	}
	_ = response.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	shutdownServers(ctx, server)

	select {
	case serveErr := <-serveDone:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			t.Fatalf("服务退出错误=%v，期望 http.ErrServerClosed", serveErr)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP 服务未在超时内退出")
	}
}

func TestServeHTTPServerReportsStartupFailure(t *testing.T) {
	errorCh := make(chan error, 1)
	go serveHTTPServer(&http.Server{Addr: "127.0.0.1:-1"}, "测试服务", errorCh)
	select {
	case err := <-errorCh:
		if err == nil {
			t.Fatal("无效监听地址应返回启动错误")
		}
	case <-time.After(time.Second):
		t.Fatal("未收到 HTTP 服务启动错误")
	}
}
