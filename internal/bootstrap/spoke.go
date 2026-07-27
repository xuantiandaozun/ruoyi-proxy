package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"ruoyi-proxy/internal/agent"
	"ruoyi-proxy/internal/buildinfo"
	"ruoyi-proxy/internal/config"
	"ruoyi-proxy/internal/hub"
)

const currentSpokeProfileVersion = 2

var publicIPDetector = detectPublicIP

// RunSpokeCLI Spoke CLI 首次启动：自检 + 交互采集 + 同步 Hub
func RunSpokeCLI(io *CLIO) {
	if !shouldRunSpokeBootstrap() {
		return
	}

	state := LoadState()
	profile, err := loadLocalSpokeProfile()
	if err != nil {
		if err := RunSpokeOnboardingNow(io); err != nil {
			io.Print("\033[1;33m引导未完成: " + err.Error() + "\033[0m")
		}
		return
	}

	if spokeProfileNeedsUpgrade(profile) {
		title := "Spoke 档案资料补全"
		if profile.SchemaVersion < currentSpokeProfileVersion {
			title = fmt.Sprintf("Spoke 档案升级 v%d → v%d", profile.SchemaVersion, currentSpokeProfileVersion)
		}
		io.Print("\n\033[1;34m═══ " + title + " ═══\033[0m")
		io.Print("检测到新版需要补充服务器资料；已有信息会保留，只询问缺失项。\n")
		if err := upgradeSpokeProfile(io, &profile); err != nil {
			io.Print("\033[1;33m档案升级未完成: " + err.Error() + "\033[0m")
			return
		}
		io.Print("\033[1;32m✓ Spoke 档案升级并同步完成\033[0m\n")
	} else if err := SyncProfileToHub(profile); err != nil {
		io.Print("\033[1;33mSpoke 档案同步失败，将在下次启动重试: " + err.Error() + "\033[0m")
	}

	state.SpokeCLIDone = true
	_ = SaveState(state)
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
	io.Print(FormatCheckReport("Spoke 节点环境自检", RunSpokeChecks()))
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

	publicIP := publicIPDetector()
	if publicIP != "" {
		io.Print(fmt.Sprintf("\033[1;36m检测到公网 IP: %s\033[0m", publicIP))
		input, _ := io.Ask(fmt.Sprintf("\033[1;33m公网 IP\033[0m [默认 %s，回车确认]: ", publicIP))
		if strings.TrimSpace(input) != "" {
			if value := normalizeIPAddress(input); value != "" {
				publicIP = value
			} else {
				io.Print("\033[1;33m输入不是有效 IP，将继续使用自动检测结果。\033[0m")
			}
		}
	} else {
		input, _ := io.Ask("\033[1;33m公网 IP\033[0m (自动检测失败，请输入；不确定可留空由 Hub 记录来源 IP): ")
		publicIP = normalizeIPAddress(input)
		if strings.TrimSpace(input) != "" && publicIP == "" {
			io.Print("\033[1;33m输入不是有效 IP，将由 Hub 使用连接来源 IP 兜底。\033[0m")
		}
	}

	desc, _ := io.Ask("\033[1;33m服务器备注\033[0m (如负责人、用途或注意事项，可选): ")
	desc = strings.TrimSpace(desc)

	paths := LoadAppPaths()
	appHome, _ := os.Getwd()

	profile := hub.SpokeProfile{
		SchemaVersion: currentSpokeProfileVersion,
		Hostname:      Hostname(),
		Label:         label,
		PublicIP:      publicIP,
		PrivateIPs:    localIPAddresses(),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		ProjectName:   projectName,
		ProjectType:   projectType,
		Description:   desc,
		Domain:        paths.Domain,
		AppHome:       appHome,
		UpdatedAt:     time.Now(),
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

func upgradeSpokeProfile(io *CLIO, profile *hub.SpokeProfile) error {
	if err := migrateSpokeProfile(io, profile); err != nil {
		return err
	}
	if err := applyProfileLocally(profile); err != nil {
		return fmt.Errorf("更新本地项目配置: %v", err)
	}
	raw, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("编码 Spoke 档案: %v", err)
	}
	if err := SaveSpokeProfile(raw); err != nil {
		return fmt.Errorf("保存 Spoke 档案: %v", err)
	}
	if err := SyncProfileToHub(*profile); err != nil {
		return fmt.Errorf("提交 Hub: %v", err)
	}
	return nil
}

func migrateSpokeProfile(io *CLIO, profile *hub.SpokeProfile) error {
	if io == nil || profile == nil {
		return fmt.Errorf("缺少档案迁移参数")
	}
	originalVersion := profile.SchemaVersion
	profile.Hostname = Hostname()
	profile.OS = runtime.GOOS
	profile.Arch = runtime.GOARCH
	profile.PrivateIPs = localIPAddresses()
	profile.Services = collectServiceRefs()
	profile.UpdatedAt = time.Now()
	if wd, err := os.Getwd(); err == nil {
		profile.AppHome = filepath.Clean(wd)
	}
	if profile.Domain == "" {
		profile.Domain = LoadAppPaths().Domain
	}

	if strings.TrimSpace(profile.Label) == "" {
		value, err := io.Ask("\033[1;33m服务器用途/别名\033[0m (如: 生产-订单服务，回车使用主机名): ")
		if err != nil {
			return err
		}
		profile.Label = strings.TrimSpace(value)
		if profile.Label == "" {
			profile.Label = profile.Hostname
		}
	}
	if strings.TrimSpace(profile.ProjectName) == "" {
		value, err := io.Ask("\033[1;33m项目名称\033[0m (如: ruoyi-admin，可选): ")
		if err != nil {
			return err
		}
		profile.ProjectName = strings.TrimSpace(value)
	}
	if strings.TrimSpace(profile.ProjectType) == "" {
		detected := DetectProjectType()
		if detected != "" {
			io.Print(fmt.Sprintf("\033[1;36m检测到项目类型: %s\033[0m", detected))
			profile.ProjectType = detected
		} else {
			value, err := io.Ask("\033[1;33m项目类型\033[0m (java/node/python/docker/go/other): ")
			if err != nil {
				return err
			}
			profile.ProjectType = mapProjectType(strings.TrimSpace(value), "other")
		}
	}
	if strings.TrimSpace(profile.PublicIP) == "" {
		profile.PublicIP = publicIPDetector()
		if profile.PublicIP != "" {
			io.Print(fmt.Sprintf("\033[1;36m检测到公网 IP: %s\033[0m", profile.PublicIP))
		} else if originalVersion < currentSpokeProfileVersion {
			value, err := io.Ask("\033[1;33m公网 IP\033[0m (自动检测失败，请输入；不确定可留空由 Hub 兜底): ")
			if err != nil {
				return err
			}
			profile.PublicIP = normalizeIPAddress(value)
			if strings.TrimSpace(value) != "" && profile.PublicIP == "" {
				io.Print("\033[1;33m输入不是有效 IP，将由 Hub 使用连接来源 IP 兜底。\033[0m")
			}
		}
	}
	if originalVersion < currentSpokeProfileVersion && strings.TrimSpace(profile.Description) == "" {
		value, err := io.Ask("\033[1;33m服务器备注\033[0m (负责人、用途或注意事项，可选): ")
		if err != nil {
			return err
		}
		profile.Description = strings.TrimSpace(value)
	}
	profile.SchemaVersion = currentSpokeProfileVersion
	return nil
}

func spokeProfileNeedsUpgrade(profile hub.SpokeProfile) bool {
	return profile.SchemaVersion < currentSpokeProfileVersion ||
		strings.TrimSpace(profile.Label) == "" ||
		strings.TrimSpace(profile.ProjectType) == "" ||
		strings.TrimSpace(profile.OS) == "" ||
		strings.TrimSpace(profile.Arch) == ""
}

func detectPublicIP() string {
	client := &http.Client{Timeout: 4 * time.Second}
	endpoints := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
	}
	for _, endpoint := range endpoints {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "ruoyi-proxy/spoke-register")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 128))
		resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		if value := normalizeIPAddress(string(data)); value != "" {
			return value
		}
	}
	return ""
}

func normalizeIPAddress(value string) string {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return ""
	}
	return ip.String()
}

func localIPAddresses() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	for _, addr := range addrs {
		var ip net.IP
		switch value := addr.(type) {
		case *net.IPNet:
			ip = value.IP
		case *net.IPAddr:
			ip = value.IP
		}
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
			continue
		}
		seen[ip.String()] = true
	}
	items := make([]string, 0, len(seen))
	for value := range seen {
		items = append(items, value)
	}
	sort.Strings(items)
	return items
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
