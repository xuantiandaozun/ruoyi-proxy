package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ruoyi-proxy/internal/agent"
	"ruoyi-proxy/internal/buildinfo"
	"ruoyi-proxy/internal/config"
	"ruoyi-proxy/internal/hub"
)

// RunSpokeCLI Spoke CLI 首次启动：自检 + 交互采集 + 同步 Hub
func RunSpokeCLI(io *CLIO) {
	if !shouldRunSpokeBootstrap() {
		return
	}

	state := LoadState()
	if !state.SpokeCLIDone {
		if err := RunSpokeOnboardingNow(io); err != nil {
			io.Print("\033[1;33m引导未完成: " + err.Error() + "\033[0m")
		}
	} else {
		io.Print(FormatCheckReport("Spoke 节点环境自检", RunEnvChecks()))
		// 已引导过仍尝试同步档案（例如 Hub 刚恢复）
		if profile, err := loadLocalSpokeProfile(); err == nil {
			_ = SyncProfileToHub(profile)
		} else {
			io.Print("\033[1;33m未找到本机 Spoke 档案，将重新进入配置引导。\033[0m")
			if err := runSpokeOnboarding(io); err != nil {
				io.Print("\033[1;33m引导未完成: " + err.Error() + "\033[0m")
				return
			}
			state.SpokeCLIDone = true
			_ = SaveState(state)
		}
	}
}

func shouldRunSpokeBootstrap() bool {
	if buildinfo.IsSpoke() {
		return true
	}
	aiCfg, _ := agent.LoadAIConfig()
	return aiCfg.Provider == "hub" && aiCfg.IsConfigured()
}

// RunSpokeOnboardingNow 立即执行 Spoke 自检与建档引导
func RunSpokeOnboardingNow(io *CLIO) error {
	if io == nil {
		return fmt.Errorf("缺少交互回调")
	}
	io.Print(FormatCheckReport("Spoke 节点环境自检", RunEnvChecks()))
	if err := runSpokeOnboarding(io); err != nil {
		return err
	}
	state := LoadState()
	state.SpokeCLIDone = true
	return SaveState(state)
}

func runSpokeOnboarding(io *CLIO) error {
	aiCfg, _ := agent.LoadAIConfig()
	if !aiCfg.IsConfigured() || aiCfg.Provider != "hub" {
		io.Print("\n\033[1;33m尚未连接 Hub，请先运行 /agent-config 选择 hub 并完成注册。\033[0m")
		io.Print("注册完成后重新启动 CLI，将继续采集本机项目信息。\n")
		return fmt.Errorf("尚未连接 Hub")
	}

	io.Print("\n\033[1;34m═══ Spoke 首次配置 ═══\033[0m")
	io.Print("请简要描述本服务器，信息将保存到本地并同步到 Hub 统一管理。\n")

	detected := DetectProjectType()
	if detected != "" {
		io.Print(fmt.Sprintf("\033[1;36m检测到项目类型: %s\033[0m", detected))
	}

	label, _ := io.Ask("\033[1;33m服务器用途/别名\033[0m (如: 生产-订单服务): ")
	label = strings.TrimSpace(label)
	if label == "" {
		label = Hostname()
	}

	projectName, _ := io.Ask("\033[1;33m项目名称\033[0m (如: ruoyi-admin): ")
	projectName = strings.TrimSpace(projectName)

	typeHint := detected
	if typeHint == "" {
		typeHint = "java"
	}
	io.Print("\n项目类型: 1=java  2=node  3=python  4=docker  5=go  6=其他")
	typeChoice, _ := io.Ask(fmt.Sprintf("\033[1;33m项目类型\033[0m [默认 %s，直接回车确认]: ", typeHint))
	projectType := mapProjectType(strings.TrimSpace(typeChoice), typeHint)

	desc, _ := io.Ask("\033[1;33m简要说明\033[0m (可选): ")
	desc = strings.TrimSpace(desc)

	paths := LoadAppPaths()
	appHome, _ := os.Getwd()

	profile := hub.SpokeProfile{
		Hostname:    Hostname(),
		Label:       label,
		ProjectName: projectName,
		ProjectType: projectType,
		Description: desc,
		Domain:      paths.Domain,
		AppHome:     appHome,
		UpdatedAt:   time.Now(),
	}
	profile.Services = collectServiceRefs()

	if err := applyProfileLocally(&profile); err != nil {
		return fmt.Errorf("写入本地配置: %w", err)
	}

	raw, _ := json.MarshalIndent(profile, "", "  ")
	if err := SaveSpokeProfile(raw); err != nil {
		return err
	}

	if err := SyncProfileToHub(profile); err != nil {
		io.Print("\033[1;33m本地已保存，同步 Hub 失败: " + err.Error() + "\033[0m")
		io.Print("可稍后重新启动 CLI 自动重试同步。\n")
	} else {
		io.Print("\033[1;32m✓ 本机信息已保存并同步到 Hub\033[0m")
		io.Print("在 Hub 端使用 /hub-status 可查看所有 Spoke 节点。\n")
	}
	return nil
}

func mapProjectType(choice, fallback string) string {
	switch choice {
	case "1", "java":
		return "java"
	case "2", "node":
		return "node"
	case "3", "python":
		return "python"
	case "4", "docker":
		return "docker"
	case "5", "go":
		return "go"
	case "6", "other", "其他":
		return "other"
	case "":
		return fallback
	default:
		return choice
	}
}

func collectServiceRefs() []hub.SpokeServiceRef {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil
	}
	refs := make([]hub.SpokeServiceRef, 0, len(cfg.Services))
	for id, svc := range cfg.Services {
		if svc == nil {
			continue
		}
		refs = append(refs, hub.SpokeServiceRef{
			ID:          id,
			Name:        svc.Name,
			ProjectType: svc.ProjectType,
			ActiveEnv:   svc.ActiveEnv,
		})
	}
	return refs
}

func applyProfileLocally(profile *hub.SpokeProfile) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	targetID := "default"
	if svc := cfg.GetService(targetID); svc != nil {
		if profile.ProjectName != "" {
			svc.Name = profile.ProjectName
		}
		if profile.ProjectType != "" {
			svc.ProjectType = profile.ProjectType
		}
	}
	if err := config.SaveConfig(cfg); err != nil {
		return err
	}

	// 更新 app_config domain（若用户填了且仍是 example.com）
	if profile.Domain != "" && profile.Domain != "example.com" {
		updateAppDomain(profile.Domain)
	}
	return nil
}

func updateAppDomain(domain string) {
	data, err := os.ReadFile(appConfigFile)
	if err != nil {
		return
	}
	var root map[string]interface{}
	if json.Unmarshal(data, &root) != nil {
		return
	}
	cur, _ := root["domain"].(string)
	if cur != "" && cur != "example.com" {
		return
	}
	root["domain"] = domain
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(appConfigFile, out, 0644)
}

func loadLocalSpokeProfile() (hub.SpokeProfile, error) {
	var profile hub.SpokeProfile
	raw, err := LoadSpokeProfileRaw()
	if err != nil {
		return profile, err
	}
	if err := json.Unmarshal(raw, &profile); err != nil {
		return profile, err
	}
	profile.Hostname = Hostname()
	profile.UpdatedAt = time.Now()
	profile.Services = collectServiceRefs()
	// 刷新 app_home
	if wd, err := os.Getwd(); err == nil {
		profile.AppHome = wd
	}
	// 规范化路径
	if profile.AppHome != "" {
		profile.AppHome = filepath.Clean(profile.AppHome)
	}
	raw2, _ := json.MarshalIndent(profile, "", "  ")
	_ = SaveSpokeProfile(raw2)
	return profile, nil
}
