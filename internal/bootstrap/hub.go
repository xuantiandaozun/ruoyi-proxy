package bootstrap

import (
	"log"

	"ruoyi-proxy/internal/buildinfo"
	"ruoyi-proxy/internal/hub"
)

// CLIO CLI 交互回调
type CLIO struct {
	Print func(string)
	Ask   func(prompt string) (string, error)
}

// RunHubServerBootstrap Hub 代理进程启动自检（只读，不自动修改）
func RunHubServerBootstrap() {
	if !shouldRunHubBootstrap() {
		return
	}

	log.Println("[bootstrap] Hub 模式启动自检...")
	for _, it := range RunHubChecks() {
		if it.Skipped {
			continue
		}
		if it.OK {
			log.Printf("[bootstrap] ✓ %s — %s", it.Name, it.Detail)
		} else {
			log.Printf("[bootstrap] ✗ %s — %s", it.Name, it.Detail)
		}
	}

	res := CheckHubNginxRoute()
	LogNginxHubResult(res)

	state := LoadState()
	state.HubServerDone = true
	_ = SaveState(state)
}

// RunHubCLI Hub CLI 启动自检（只读，不自动修改；修复交给 AI）
func RunHubCLI(io *CLIO) {
	if !shouldRunHubBootstrap() {
		return
	}
	if buildinfo.IsHub() {
		settings, _ := hub.LoadHubSettings()
		if !settings.Enabled {
			_ = hub.SaveHubEnabled(true)
		}
	}

	items := RunHubChecks()
	io.Print(FormatCheckReport("Hub 节点自检", items))

	res := CheckHubNginxRoute()
	if !res.AlreadyOK && res.Message != "" {
		io.Print("\n\033[1;33m注意:\033[0m " + res.Message)
		io.Print("\033[1;33m可输入 '/fix-nginx-hub' 或让 AI 修复 Nginx Hub 路由。\033[0m")
	}

	state := LoadState()
	if state.HubCLIDone {
		io.Print("")
		return
	}

	io.Print("\n\033[1;33m首次使用提示:\033[0m")
	io.Print("  • 网关需无参数启动: ./ruoyi-proxy-linux-hub")
	io.Print("  • 生成 Spoke 注册 Token: /hub-token")
	io.Print("  • 查看已注册节点: /hub-status")
	io.Print("  • 修改 AI 配置: /agent-config\n")

	state.HubCLIDone = true
	_ = SaveState(state)
}

// RunSelfCheck 手动触发完整自检（/self-check）
func RunSelfCheck(io *CLIO) {
	if shouldRunHubBootstrap() {
		io.Print(FormatCheckReport("Hub 节点自检", RunHubChecks()))
		return
	}
	if shouldRunSpokeBootstrap() {
		io.Print(FormatCheckReport("Spoke 节点环境自检", RunSpokeChecks()))
		return
	}
	io.Print(FormatCheckReport("环境自检", RunEnvChecks()))
}

func shouldRunHubBootstrap() bool {
	if buildinfo.IsHub() {
		return true
	}
	settings, _ := hub.LoadHubSettings()
	return settings.Enabled
}
