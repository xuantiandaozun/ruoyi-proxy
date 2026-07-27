package cli

import (
	"context"

	"ruoyi-proxy/internal/spokecontrol"
)

func (c *CLI) restartSpokeControlWorker() {
	if c.spokeWorkerCancel != nil {
		c.spokeWorkerCancel()
	}
	c.spokeWorkerCancel = c.startSpokeControlWorker()
}

// startSpokeControlWorker 在无代理进程时启动交互 CLI 兼容轮询。
func (c *CLI) startSpokeControlWorker() func() {
	if spokecontrol.ProxyRunning() {
		c.printInfo("代理进程已接管 Spoke 远程控制，无需独立常驻")
		return func() {}
	}
	worker, err := spokecontrol.NewFromLocalConfig(nil)
	if err != nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = worker.Run(ctx) }()
	c.printInfo("当前未运行代理，由交互 CLI 临时接管 Spoke 远程控制")
	return cancel
}
