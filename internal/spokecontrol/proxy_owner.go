package spokecontrol

import (
	"context"
	"time"
)

// RunProxyOwned 由代理进程持有远程控制，并等待后续 Hub 注册配置生效。
func RunProxyOwned(ctx context.Context, logf func(format string, args ...interface{})) error {
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}
	ticker := time.NewTicker(ownershipCheckInterval)
	defer ticker.Stop()

	for {
		worker, err := NewFromLocalConfig(logf)
		if err == nil {
			logf("代理进程已接管 Spoke 远程控制")
			return worker.Run(ctx)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
