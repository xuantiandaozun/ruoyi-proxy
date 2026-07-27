package spokecontrol

import (
	"context"
	"net/http"
	"strings"
	"time"

	"ruoyi-proxy/internal/config"
)

const ownershipCheckInterval = 3 * time.Second

var proxyStatusURL = localProxyStatusURL()
var proxyStatusClient = &http.Client{Timeout: 800 * time.Millisecond}

// ProxyRunning 判断本机代理进程是否正在提供管理接口。
func ProxyRunning() bool {
	req, err := http.NewRequest(http.MethodGet, proxyStatusURL, nil)
	if err != nil {
		return false
	}
	resp, err := proxyStatusClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// RunStandalone 仅在代理进程未运行时执行独立 Spoke 轮询。
// 已安装的常驻服务会在代理运行时待机，并在代理停止后自动接管。
func RunStandalone(ctx context.Context, logf func(format string, args ...interface{})) error {
	worker, err := NewFromLocalConfig(logf)
	if err != nil {
		return err
	}
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}

	ticker := time.NewTicker(ownershipCheckInterval)
	defer ticker.Stop()

	var workerCancel context.CancelFunc
	var workerDone chan error
	proxyOwned := false

	stopWorker := func() {
		if workerCancel == nil {
			return
		}
		workerCancel()
		<-workerDone
		workerCancel = nil
		workerDone = nil
	}
	defer stopWorker()

	for {
		proxyRunning := ProxyRunning()
		switch {
		case proxyRunning && workerCancel != nil:
			stopWorker()
			logf("检测到代理进程，远程控制改由代理进程接管")
			proxyOwned = true
		case proxyRunning && !proxyOwned:
			logf("代理进程已接管远程控制，独立 spoke-agent 进入待机")
			proxyOwned = true
		case !proxyRunning && workerCancel == nil:
			workerCtx, cancel := context.WithCancel(ctx)
			done := make(chan error, 1)
			workerCancel = cancel
			workerDone = done
			proxyOwned = false
			logf("未检测到代理进程，独立 spoke-agent 接管远程控制")
			go func() {
				done <- worker.Run(workerCtx)
			}()
		}

		select {
		case <-ctx.Done():
			return nil
		case err := <-workerDone:
			workerCancel = nil
			workerDone = nil
			if err != nil {
				return err
			}
		case <-ticker.C:
		}
	}
}

func localProxyStatusURL() string {
	port := config.MgmtPort
	if strings.HasPrefix(port, ":") {
		return "http://127.0.0.1" + port + "/status"
	}
	return "http://" + port + "/status"
}
