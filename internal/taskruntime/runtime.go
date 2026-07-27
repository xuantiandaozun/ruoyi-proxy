package taskruntime

import (
	"context"
	"fmt"

	"ruoyi-proxy/internal/agent"
	"ruoyi-proxy/internal/scheduler"
)

// Run 启动本机 AI 定时任务运行时。
func Run(ctx context.Context, logf scheduler.LogFunc) error {
	store, err := scheduler.Open()
	if err != nil {
		return err
	}
	defer store.Close()
	execute := func(taskCtx context.Context, task scheduler.Task) (string, error) {
		aiCfg, err := agent.LoadAIConfig()
		if err != nil {
			return "", fmt.Errorf("读取 AI 配置失败: %v", err)
		}
		return agent.RunScheduledTask(
			taskCtx, aiCfg, agent.BuildExecContext(task.ServiceID), task.Prompt, task.AllowWrite,
		)
	}
	service := scheduler.NewService(store, execute)
	service.SetLogger(logf)
	return service.Run(ctx)
}
