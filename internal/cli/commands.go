package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"ruoyi-proxy/internal/agent"
	"ruoyi-proxy/internal/buildinfo"
	"ruoyi-proxy/internal/config"
	"ruoyi-proxy/internal/hub"
)

// ServiceStatus 服务状态
type ServiceStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ActiveEnv   string `json:"active_env"`
	BlueTarget  string `json:"blue_target"`
	GreenTarget string `json:"green_target"`
}

func servicesFromConfig(cfg *config.Config) []ServiceStatus {
	ids := make([]string, 0, len(cfg.Services))
	for id := range cfg.Services {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	services := make([]ServiceStatus, 0, len(ids))
	for _, id := range ids {
		svc := cfg.Services[id]
		name := svc.Name
		if name == "" {
			name = id
		}
		services = append(services, ServiceStatus{
			ID:          id,
			Name:        name,
			ActiveEnv:   svc.ActiveEnv,
			BlueTarget:  svc.BlueTarget,
			GreenTarget: svc.GreenTarget,
		})
	}
	return services
}

// ShowDetailedStatus 显示详细状态
func (c *CLI) ShowDetailedStatus() {
	c.printInfo("获取系统状态...")

	cfg, err := c.loadProxyConfig()
	if err != nil {
		c.printError(fmt.Sprintf("读取配置失败: %v", err))
		return
	}

	status := "stopped"
	if c.isProxyRunning() {
		status = "running"
	}

	services := servicesFromConfig(cfg)

	fmt.Println("\n" + strings.Repeat("-", 70))
	fmt.Println("\033[1;34m系统状态\033[0m")
	fmt.Println(strings.Repeat("-", 70))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "\033[1;33m代理状态\033[0m\t\033[1;32m%s\033[0m\n", status)
	fmt.Fprintf(w, "\033[1;33m服务数量:\033[0m\t\033[1;36m%d\033[0m\n", len(services))
	fmt.Fprintf(w, "\033[1;33m代理端口:\033[0m\t%s\n", config.ProxyPort)
	fmt.Fprintf(w, "\033[1;33m时间:\033[0m\t%s\n", time.Now().Format("2006-01-02 15:04:05"))
	w.Flush()

	fmt.Println(strings.Repeat("-", 70))

	fmt.Println("\n\033[1;34m服务列表\033[0m")
	fmt.Println(strings.Repeat("-", 70))

	w2 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w2, "\033[1;33m  %-12s\t%-15s\t%-8s\t%s\033[0m\n", "ID", "名称", "环境", "目标地址")
	fmt.Fprintf(w2, "  %s\t%s\t%s\t%s\n", strings.Repeat("-", 12), strings.Repeat("-", 15), strings.Repeat("-", 8), strings.Repeat("-", 25))

	for _, svc := range services {
		envColor := "\033[1;34m"
		if svc.ActiveEnv == "green" {
			envColor = "\033[1;32m"
		}
		target := svc.BlueTarget
		if svc.ActiveEnv == "green" {
			target = svc.GreenTarget
		}
		fmt.Fprintf(w2, "  %-12s\t%-15s\t%s%-8s\033[0m\t%s\n",
			svc.ID, svc.Name, envColor, svc.ActiveEnv, target)
	}
	w2.Flush()

	fmt.Println(strings.Repeat("-", 70))

	c.checkAllServicesHealth(services)
}

// checkAllServicesHealth 检查所有服务健康状态
func (c *CLI) checkAllServicesHealth(services []ServiceStatus) {
	fmt.Println("\n\033[1;34m健康检查\033[0m")
	fmt.Println(strings.Repeat("─", 70))

	client := &http.Client{Timeout: 3 * time.Second}

	for _, svc := range services {
		target := svc.BlueTarget
		if svc.ActiveEnv == "green" {
			target = svc.GreenTarget
		}

		resp, err := client.Get(target + "/actuator/health")
		if err != nil {
			fmt.Printf("  \033[1;31m✗\033[0m %s(%s): \033[1;31m不可用\033[0m [%s]\n", svc.Name, svc.ID, target)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == 200 {
			fmt.Printf("  \033[1;32m✓\033[0m %s(%s): \033[1;32m健康\033[0m [%s]\n", svc.Name, svc.ID, target)
		} else {
			fmt.Printf("  \033[1;33m⚠\033[0m %s(%s): \033[1;33m异常 (HTTP %d)\033[0m [%s]\n", svc.Name, svc.ID, resp.StatusCode, target)
		}
	}

	fmt.Println(strings.Repeat("─", 70))
}

// QuickDeploy 快速部署向导
func (c *CLI) QuickDeploy() {
	fmt.Println("\n\033[1;34m═══ 快速部署向导 ═══\033[0m\n")

	steps := []string{
		"准备AppCDS归档",
		"启动待机环境",
		"健康检查",
		"切换流量",
		"清理备用环境",
	}

	fmt.Println("部署步骤:")
	for i, step := range steps {
		fmt.Printf("  [%d/%d] %s\n", i+1, len(steps), step)
	}

	if !c.confirmDangerAction("开始快速部署", []string{"将按部署向导依次执行启动、健康检查、切换流量等步骤。"}) {
		return
	}

	fmt.Println()
	c.executeScript("service.sh", "deploy")
}

// ShowLogs 显示日志（带颜色高亮）
func (c *CLI) ShowLogs(lines string) {
	c.printInfo(fmt.Sprintf("查看最新%s行日志", lines))
	fmt.Println(strings.Repeat("─", 60))

	c.executeScript("service.sh", "logs", lines)
}

// InteractiveSwitch 交互式环境切换
func (c *CLI) InteractiveSwitch() {
	fmt.Println("\n\033[1;34m=== 环境切换 ===\033[0m\n")

	cfg, err := c.loadProxyConfig()
	if err != nil {
		c.printError(fmt.Sprintf("读取配置失败: %v", err))
		return
	}

	services := servicesFromConfig(cfg)
	if len(services) == 0 {
		c.printError("未配置服务")
		return
	}

	fmt.Println("服务列表:")
	for i, svc := range services {
		envColor := "\033[1;34m"
		if svc.ActiveEnv == "green" {
			envColor = "\033[1;32m"
		}
		fmt.Printf("  %d. %s (%s) - 当前环境: %s%s\033[0m\n", i+1, svc.Name, svc.ID, envColor, svc.ActiveEnv)
	}

	fmt.Println("\n切换方式:")
	fmt.Println("  1. 切换所有服务")
	fmt.Println("  2. 切换单个服务")
	fmt.Println("  0. 取消")

	choice, ok := c.selectSimpleMenu("选择切换方式", []string{"切换所有服务", "切换单个服务", "取消"}, 0)
	if !ok {
		c.printInfo("已取消")
		return
	}

	switch choice {
	case 0:
		c.switchAllServices()
	case 1:
		c.switchSingleService(services)
	default:
		c.printInfo("已取消")
	}
}

// switchAllServices 切换所有服务
func (c *CLI) switchAllServices() {
	env, ok := c.selectEnvMenu(-1)
	if !ok {
		c.printInfo("已取消")
		return
	}
	if !c.confirmDangerAction(fmt.Sprintf("切换所有服务到 %s", env), []string{"该操作会批量修改所有服务的活跃环境配置。"}) {
		return
	}

	c.printInfo(fmt.Sprintf("切换所有服务到 %s...", env))

	cfg, err := c.loadProxyConfig()
	if err != nil {
		c.printError(fmt.Sprintf("读取配置失败: %v", err))
		return
	}
	for _, svc := range cfg.Services {
		svc.ActiveEnv = env
	}
	if err := config.SaveConfig(cfg); err != nil {
		c.printError(fmt.Sprintf("保存配置失败: %v", err))
		return
	}

	c.printSuccess(fmt.Sprintf("已切换所有服务到 %s (配置已更新)", env))
	c.promptProxyRestart()
}

// switchSingleService 切换单个服务
func (c *CLI) switchSingleService(services []ServiceStatus) {
	serviceID, ok := c.selectServiceStatusMenu(services)
	if !ok {
		c.printInfo("已取消")
		return
	}
	env, ok := c.selectEnvMenu(-1)
	if !ok {
		c.printInfo("已取消")
		return
	}
	if !c.confirmDangerAction(fmt.Sprintf("切换服务[%s]到 %s", serviceID, env), []string{"该操作会修改该服务的活跃环境配置。"}) {
		return
	}

	c.printInfo(fmt.Sprintf("切换服务[%s]到%s...", serviceID, env))

	cfg, err := c.loadProxyConfig()
	if err != nil {
		c.printError(fmt.Sprintf("读取配置失败: %v", err))
		return
	}
	svc := cfg.GetService(serviceID)
	if svc == nil {
		c.printError(fmt.Sprintf("服务不存在: %s", serviceID))
		return
	}
	svc.ActiveEnv = env
	if err := config.SaveConfig(cfg); err != nil {
		c.printError(fmt.Sprintf("保存配置失败: %v", err))
		return
	}

	c.printSuccess(fmt.Sprintf("服务[%s]已切换到 %s (配置已更新)", serviceID, env))
	c.promptProxyRestart()
}

func (c *CLI) selectEnvMenu(current int) (string, bool) {
	idx, ok := c.selectSimpleMenu("目标环境", []string{"blue", "green"}, current)
	if !ok {
		return "", false
	}
	if idx == 0 {
		return "blue", true
	}
	return "green", true
}

func (c *CLI) selectServiceStatusMenu(services []ServiceStatus) (string, bool) {
	options := make([]string, 0, len(services))
	selected := 0
	for i, svc := range services {
		options = append(options, fmt.Sprintf("%-12s  %s  [%s]", svc.ID, svc.Name, svc.ActiveEnv))
		if svc.ID == c.currentService {
			selected = i
		}
	}
	idx, ok := c.selectSimpleMenu("选择服务", options, selected)
	if !ok {
		return "", false
	}
	return services[idx].ID, true
}

// ShowSystemInfo 显示系统信息
func (c *CLI) ShowSystemInfo() {
	fmt.Println("\n\033[1;34m═══ 系统信息 ═══\033[0m\n")

	// Java版本
	c.printCommandOutput("Java版本", "java", "-version")

	// Docker版本
	c.printCommandOutput("Docker版本", "docker", "--version")

	// Nginx版本
	c.printCommandOutput("Nginx版本", "nginx", "-v")

	// 磁盘使用
	c.printCommandOutput("磁盘使用", "df", "-h", ".")

	// 内存使用
	c.printCommandOutput("内存使用", "free", "-h")
}

// printCommandOutput 打印命令输出
func (c *CLI) printCommandOutput(label string, name string, args ...string) {
	fmt.Printf("\033[1;33m%s:\033[0m\n", label)
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("  \033[1;31m未安装或不可用\033[0m\n\n")
		return
	}
	fmt.Printf("  %s\n", strings.TrimSpace(string(output)))
}

// ShowQuickCommands 显示快捷命令
func (c *CLI) ShowQuickCommands() {
	fmt.Println("\n\033[1;34m═══ 快捷命令 ═══\033[0m\n")

	commands := []struct {
		cmd  string
		desc string
	}{
		{"start", "启动服务"},
		{"stop", "停止服务"},
		{"restart", "重启服务"},
		{"deploy", "蓝绿部署"},
		{"deploy-lowmem", "低内存部署"},
		{"status", "查看状态"},
		{"logs", "查看日志"},
		{"switch", "交互式切换环境"},
		{"switch blue", "切换所有服务到blue"},
		{"switch green", "切换所有服务到green"},
		{"init", "初始化系统"},
		{"cert <域名>", "申请SSL证书"},
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	for _, cmd := range commands {
		fmt.Fprintf(w, "  \033[1;36m%-20s\033[0m\t%s\n", cmd.cmd, cmd.desc)
	}
	w.Flush()
	fmt.Println()
}

const jvmConfigFile = "configs/app_config.json"

// JVMConfig JVM配置管理
func (c *CLI) JVMConfig() {
	fmt.Println("\n\033[1;34m═══ JVM配置管理 ═══\033[0m\n")

	data, err := os.ReadFile(jvmConfigFile)
	if err != nil {
		c.printError("配置文件不存在，请先运行 init 命令初始化")
		return
	}

	var appConfig map[string]interface{}
	if err := json.Unmarshal(data, &appConfig); err != nil {
		c.printError("配置文件格式错误")
		return
	}

	jvm, ok := appConfig["jvm"].(map[string]interface{})
	if !ok {
		c.printWarning("JVM配置不存在，正在创建默认配置...")
		jvm = createDefaultJVMConfig()
		appConfig["jvm"] = jvm
		if err := saveAppConfig(appConfig); err != nil {
			c.printError(fmt.Sprintf("保存配置文件失败: %v", err))
			return
		}
		c.printSuccess("已创建默认JVM配置（档位2: 2核4G）")
		fmt.Println()
	}

	for {
		// 重新从内存读取最新 jvm（每次操作后可能已更新）
		jvm = appConfig["jvm"].(map[string]interface{})

		currentPreset := int(jvm["preset"].(float64))
		customOpts := ""
		if co, ok := jvm["custom_opts"].(string); ok {
			customOpts = co
		}

		presets, ok := jvm["presets"].(map[string]interface{})
		if !ok {
			c.printError("预设配置不存在")
			return
		}

		fmt.Printf("\n当前JVM预设档位: \033[1;36m%d\033[0m\n", currentPreset)
		if customOpts != "" {
			fmt.Printf("自定义参数: \033[1;36m%s\033[0m\n", customOpts)
		}

		fmt.Println("\n\033[1;33m可用预设档位:\033[0m")
		for i := 1; i <= 3; i++ {
			key := fmt.Sprintf("%d", i)
			if preset, ok := presets[key].(map[string]interface{}); ok {
				name := preset["name"].(string)
				xms := preset["xms"].(string)
				xmx := preset["xmx"].(string)
				gcThreads := int(preset["gc_threads"].(float64))
				mark := ""
				if i == currentPreset {
					mark = " \033[1;32m← 当前\033[0m"
				}
				fmt.Printf("  %d. %s - 堆内存:%s-%s, GC线程:%d%s\n", i, name, xms, xmx, gcThreads, mark)
			}
		}

		fmt.Println("\n\033[1;33m操作选项:\033[0m")
		fmt.Println("  1. 切换预设档位")
		fmt.Println("  2. 设置自定义参数")
		fmt.Println("  3. 查看详细配置")
		fmt.Println("  0. 返回")

		choice, ok := c.selectSimpleMenu("JVM 配置操作", []string{
			"切换预设档位",
			"设置自定义参数",
			"查看详细配置",
			"返回",
		}, 0)
		if !ok || choice == 3 {
			return
		}

		switch choice {
		case 0:
			c.switchJVMPreset(appConfig, jvm)
		case 1:
			c.setJVMCustomOpts(appConfig, jvm)
		case 2:
			c.showJVMDetail(jvm)
		}
	}
}

// switchJVMPreset 切换JVM预设档位
func (c *CLI) switchJVMPreset(appConfig map[string]interface{}, jvm map[string]interface{}) {
	presets, ok := jvm["presets"].(map[string]interface{})
	if !ok {
		c.printError("预设配置不存在")
		return
	}
	options := make([]string, 0, 3)
	currentPreset := int(jvm["preset"].(float64))
	selected := 0
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("%d", i)
		preset, ok := presets[key].(map[string]interface{})
		if !ok {
			continue
		}
		name := preset["name"].(string)
		xms := preset["xms"].(string)
		xmx := preset["xmx"].(string)
		gcThreads := int(preset["gc_threads"].(float64))
		line := fmt.Sprintf("档位 %d  %s  堆:%s-%s  GC线程:%d", i, name, xms, xmx, gcThreads)
		options = append(options, line)
		if i == currentPreset {
			selected = len(options) - 1
		}
	}
	choice, ok := c.selectSimpleMenu("选择 JVM 预设档位", options, selected)
	if !ok {
		c.printInfo("已取消")
		return
	}
	num := choice + 1
	jvm["preset"] = float64(num)
	appConfig["jvm"] = jvm

	if err := saveAppConfig(appConfig); err != nil {
		c.printError(fmt.Sprintf("保存配置文件失败: %v", err))
		return
	}

	c.printSuccess(fmt.Sprintf("JVM预设已切换到档位 %d", num))
	c.printInfo("重启Java应用后生效")
}

// setJVMCustomOpts 设置自定义JVM参数
func (c *CLI) setJVMCustomOpts(appConfig map[string]interface{}, jvm map[string]interface{}) {
	currentOpts := ""
	if co, ok := jvm["custom_opts"].(string); ok {
		currentOpts = co
	}

	fmt.Printf("当前自定义参数: \033[1;36m%s\033[0m\n", currentOpts)
	fmt.Println("示例: -XX:+UseZGC -Dspring.profiles.active=prod")

	newOpts, err := c.readLineWithPrompt("\033[1;33m新的自定义参数 (留空清除): \033[0m")
	if err != nil {
		return
	}

	jvm["custom_opts"] = strings.TrimSpace(newOpts)
	appConfig["jvm"] = jvm

	if err := saveAppConfig(appConfig); err != nil {
		c.printError(fmt.Sprintf("保存配置文件失败: %v", err))
		return
	}

	c.printSuccess("自定义JVM参数已更新")
	c.printInfo("重启Java应用后生效")
}

// saveAppConfig 保存 app_config.json
func saveAppConfig(appConfig map[string]interface{}) error {
	data, err := json.MarshalIndent(appConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("配置序列化失败: %v", err)
	}
	return os.WriteFile(jvmConfigFile, data, 0644)
}

// showJVMDetail 显示JVM详细配置
func (c *CLI) showJVMDetail(jvm map[string]interface{}) {
	fmt.Println("\n\033[1;33mJVM详细配置:\033[0m")
	fmt.Println(strings.Repeat("-", 60))

	currentPreset := int(jvm["preset"].(float64))
	fmt.Printf("当前预设: %d\n", currentPreset)

	customOpts := ""
	if co, ok := jvm["custom_opts"].(string); ok && co != "" {
		customOpts = co
		fmt.Printf("自定义参数: %s\n", customOpts)
	}

	presets, ok := jvm["presets"].(map[string]interface{})
	if !ok {
		return
	}

	fmt.Println("\n预设详情:")
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("%d", i)
		if preset, ok := presets[key].(map[string]interface{}); ok {
			name := preset["name"].(string)
			xms := preset["xms"].(string)
			xmx := preset["xmx"].(string)
			metaspace := preset["metaspace_size"].(string)
			maxMetaspace := preset["max_metaspace_size"].(string)
			gcThreads := int(preset["gc_threads"].(float64))
			parallelGC := int(preset["parallel_gc_threads"].(float64))

			mark := ""
			if i == currentPreset {
				mark = " ← 当前"
			}

			fmt.Printf("\n%d. %s%s\n", i, name, mark)
			fmt.Printf("   堆内存: -Xms%s -Xmx%s\n", xms, xmx)
			fmt.Printf("   元空间: -XX:MetaspaceSize=%s -XX:MaxMetaspaceSize=%s\n", metaspace, maxMetaspace)
			fmt.Printf("   GC线程: -XX:ParallelGCThreads=%d -XX:ConcGCThreads=%d\n", parallelGC, gcThreads)
		}
	}

	fmt.Println(strings.Repeat("-", 60))
}

// createDefaultJVMConfig 创建默认JVM配置
func createDefaultJVMConfig() map[string]interface{} {
	return map[string]interface{}{
		"preset":      float64(2),
		"custom_opts": "",
		"presets": map[string]interface{}{
			"1": map[string]interface{}{
				"name":                "1核2G",
				"description":         "1核CPU，2G内存",
				"xms":                 "512m",
				"xmx":                 "1g",
				"metaspace_size":      "128m",
				"max_metaspace_size":  "256m",
				"gc_threads":          float64(1),
				"parallel_gc_threads": float64(1),
			},
			"2": map[string]interface{}{
				"name":                "2核4G",
				"description":         "2核CPU，4G内存",
				"xms":                 "1g",
				"xmx":                 "3g",
				"metaspace_size":      "256m",
				"max_metaspace_size":  "512m",
				"gc_threads":          float64(2),
				"parallel_gc_threads": float64(2),
			},
			"3": map[string]interface{}{
				"name":                "4核8G",
				"description":         "4核CPU，8G内存",
				"xms":                 "2g",
				"xmx":                 "6g",
				"metaspace_size":      "256m",
				"max_metaspace_size":  "512m",
				"gc_threads":          float64(4),
				"parallel_gc_threads": float64(4),
			},
		},
	}
}

// MonitorMode 监控模式
func (c *CLI) MonitorMode() {
	fmt.Println("\n\033[1;34m═══ 监控模式 ═══\033[0m")
	fmt.Println("按 Ctrl+C 退出监控\n")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.clearScreen()
			fmt.Println("\033[1;34m═══ 实时监控 ═══\033[0m")
			fmt.Printf("更新时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
			c.ShowDetailedStatus()
		}
	}
}

// RunAgentPrimary 启动 Agent 作为主交互入口
func (c *CLI) RunAgentPrimary() {
	aiCfg, _ := agent.LoadAIConfig()

	execCtx := agent.BuildExecContext(c.currentService)

	confirm := func(_ string) bool {
		line, err := c.readLineWithPrompt("")
		if err != nil {
			return false
		}
		s := strings.ToLower(strings.TrimSpace(line))
		if s == "" || s == "y" || s == "yes" || s == "ok" ||
			s == "确认" || s == "同意" || s == "好" || s == "好的" ||
			s == "可以" || s == "是" || s == "是的" {
			return true
		}
		return false
	}

	var agentRef *agent.Agent
	readInput := func(prompt string) (string, error) {
		line, err := c.readLineWithPrompt(prompt)
		if err != nil {
			return "", err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "/" && agentRef != nil {
			if cmd, ok := agentRef.PickSlashCommand(""); ok {
				return cmd, nil
			}
			return "", nil
		}
		return line, nil
	}

	print := func(s string) {
		fmt.Println(s)
	}

	a, err := agent.New(aiCfg, execCtx, confirm, readInput, print)
	if err != nil {
		fmt.Printf("\033[1;31m✗ 创建 Agent 失败: %v\033[0m\n", err)
		return
	}
	agentRef = a

	a.SetOpsHooks(func(cmd string, args []string) bool {
		return c.dispatchOpsCommand(a, cmd, args)
	}, c.printHelp)
	a.SetSlashMenuItems(c.buildSlashMenuItems)

	c.agentCancel = a.Cancel
	defer func() { c.agentCancel = nil }()

	a.Run()
	c.running = false
}

// buildSlashMenuItems 构建 / 命令菜单项（会话 + 运维）
func (c *CLI) buildSlashMenuItems() []agent.SlashCommandItem {
	return []agent.SlashCommandItem{
		{Command: "/sessions", Description: "查看历史会话"},
		{Command: "/load", Description: "加载历史会话"},
		{Command: "/new", Description: "新建会话"},
		{Command: "/current", Description: "当前会话信息"},
		{Command: "/help", Description: "查看命令说明"},
		{Command: "/commands", Description: "运维命令列表"},
		{Command: "/start", Description: "启动服务"},
		{Command: "/stop", Description: "停止服务"},
		{Command: "/restart", Description: "重启服务"},
		{Command: "/deploy", Description: "蓝绿部署"},
		{Command: "/deploy-lowmem", Description: "低内存部署"},
		{Command: "/status", Description: "服务状态"},
		{Command: "/detail", Description: "详细状态"},
		{Command: "/logs", Description: "查看日志"},
		{Command: "/logs-follow", Description: "实时日志"},
		{Command: "/switch", Description: "切换蓝绿环境"},
		{Command: "/proxy-status", Description: "代理状态"},
		{Command: "/proxy-start", Description: "启动代理"},
		{Command: "/proxy-stop", Description: "停止代理"},
		{Command: "/service-list", Description: "服务列表"},
		{Command: "/service-switch", Description: "切换当前服务"},
		{Command: "/config", Description: "查看配置"},
		{Command: "/agent-config", Description: "配置 AI / Hub 注册"},
		{Command: "/hub-token", Description: "生成 Hub 注册 Token"},
		{Command: "/hub-status", Description: "Hub Spoke 列表"},
		{Command: "/hub-spoke", Description: "查看单个 Spoke 详情"},
		{Command: "/hub-enable", Description: "启用 Hub 网关"},
		{Command: "/hub-disable", Description: "禁用 Hub 网关"},
		{Command: "/hub-revoke", Description: "吊销 Spoke"},
		{Command: "/self-check", Description: "运行环境自检"},
		{Command: "/fix-nginx-hub", Description: "让 AI 修复 Nginx Hub 路由"},
		{Command: "/init", Description: "环境初始化"},
		{Command: "/cls", Description: "清屏"},
		{Command: "/exit", Description: "退出"},
	}
}

var knownOpsCommands = map[string]bool{
	"start": true, "stop": true, "restart": true, "deploy": true, "deploy-lowmem": true,
	"status": true, "logs": true, "logs-follow": true, "logs-search": true, "logs-export": true,
	"init": true, "cert": true, "enable-https": true, "disable-https": true,
	"proxy-start": true, "proxy-stop": true, "proxy-restart": true, "proxy-status": true,
	"switch": true, "detail": true, "quick": true, "info": true, "monitor": true,
	"quick-deploy": true, "config": true, "config-edit": true,
	"service-add": true, "service-list": true, "service-remove": true, "service-switch": true,
	"jvm-config": true, "agent-config": true, "commands": true, "cls": true,
	"hub-enable": true, "hub-disable": true, "hub-token": true, "hub-status": true, "hub-spoke": true, "hub-revoke": true,
	"self-check": true, "fix-nginx-hub": true,
}

// dispatchOpsCommand 处理 Agent 模式下的 /运维命令
func (c *CLI) dispatchOpsCommand(a *agent.Agent, cmd string, args []string) bool {
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	if !knownOpsCommands[cmd] {
		return false
	}
	switch cmd {
	case "agent-config":
		c.AgentConfig(a)
		return true
	case "commands":
		c.printHelp()
		return true
	case "self-check":
		c.runSelfCheck()
		return true
	case "fix-nginx-hub":
		c.runFixNginxHub(a)
		return true
	case "cls":
		c.clearScreen()
		return true
	default:
		input := cmd
		if len(args) > 0 {
			input += " " + strings.Join(args, " ")
		}
		c.handleCommand(input)
		return true
	}
}

// StartAgent 保留兼容别名
func (c *CLI) StartAgent() {
	c.RunAgentPrimary()
}

// AgentConfig 配置 AI 提供商
func (c *CLI) AgentConfig(a ...*agent.Agent) {
	aiCfg, _ := agent.LoadAIConfig()

	fmt.Println("\033[1;34m═══ AI Agent 配置 ═══\033[0m")
	fmt.Printf("当前提供商: \033[1;36m%s\033[0m\n", aiCfg.Provider)
	fmt.Printf("当前模型:   \033[1;36m%s\033[0m\n", aiCfg.Model)
	fmt.Printf("API Key:    \033[1;36m%s\033[0m\n", aiCfg.MaskedKey())
	if aiCfg.BaseURL != "" {
		fmt.Printf("Base URL:   \033[1;36m%s\033[0m\n", aiCfg.BaseURL)
	}
	fmt.Println()

	choice, ok := c.selectSimpleMenu("选择提供商", []string{
		"anthropic  (Claude)",
		"openai     (GPT / 兼容 API)",
		"ollama     (本地模型)",
		"hub        (通过中心服务器转发 AI)",
		"取消",
	}, 0)
	if !ok || choice == 4 {
		return
	}

	var provider string
	switch choice {
	case 0:
		provider = "anthropic"
	case 1:
		provider = "openai"
	case 2:
		provider = "ollama"
	case 3:
		provider = "hub"
	default:
		fmt.Println("\033[1;31m✗ 无效选择\033[0m")
		return
	}

	defaults := agent.DefaultAIConfig(provider)
	aiCfg.Provider = provider

	if provider == "hub" {
		hubURL := strings.TrimSpace(aiCfg.BaseURL)
		if hubURL == "" {
			input, err := c.readLineWithPrompt("Hub 地址 (如 https://hub.example.com): ")
			if err != nil || strings.TrimSpace(input) == "" {
				c.printError("Hub 地址不能为空")
				return
			}
			hubURL = strings.TrimSpace(input)
		} else {
			c.printInfo(fmt.Sprintf("使用已预置 Hub 地址: %s", hubURL))
		}
		aiCfg.BaseURL = strings.TrimRight(hubURL, "/")
		c.printInfo("正在向 Hub 申请注册 Token...")
		regToken, err := agent.RequestHubRegisterToken(aiCfg.BaseURL)
		if err != nil {
			c.printError(fmt.Sprintf("申请注册 Token 失败: %v", err))
			return
		}
		secret, spokeID, err := agent.RegisterWithHub(aiCfg.BaseURL, regToken)
		if err != nil {
			c.printError(fmt.Sprintf("Hub 注册失败: %v", err))
			return
		}
		aiCfg.APIKey = secret
		aiCfg.Model = "hub-relay"
		fmt.Printf("\033[1;32m✓ Hub 注册成功，Spoke ID: %s\033[0m\n", spokeID)
	} else if provider != "ollama" {
		apiKey, err := c.readLineWithPrompt(fmt.Sprintf("API Key (当前: %s): ", aiCfg.MaskedKey()))
		if err != nil {
			return
		}
		if strings.TrimSpace(apiKey) != "" {
			aiCfg.APIKey = strings.TrimSpace(apiKey)
		}

		baseURLPrompt := fmt.Sprintf("Base URL (留空使用默认 %s): ", defaults.BaseURL)
		if aiCfg.BaseURL != "" {
			baseURLPrompt = fmt.Sprintf("Base URL (当前: %s, 留空保持): ", aiCfg.BaseURL)
		}
		baseURL, err := c.readLineWithPrompt(baseURLPrompt)
		if err != nil {
			return
		}
		if strings.TrimSpace(baseURL) != "" {
			aiCfg.BaseURL = strings.TrimSpace(baseURL)
		} else if aiCfg.BaseURL == "" {
			aiCfg.BaseURL = defaults.BaseURL
		}

		modelPrompt := fmt.Sprintf("模型名称 (留空使用默认 %s): ", defaults.Model)
		if aiCfg.Model != "" {
			modelPrompt = fmt.Sprintf("模型名称 (当前: %s, 留空保持): ", aiCfg.Model)
		}
		model, err := c.readLineWithPrompt(modelPrompt)
		if err != nil {
			return
		}
		if strings.TrimSpace(model) != "" {
			aiCfg.Model = strings.TrimSpace(model)
		} else if aiCfg.Model == "" {
			aiCfg.Model = defaults.Model
		}
	} else {
		baseURLPrompt := fmt.Sprintf("Base URL (留空使用默认 %s): ", defaults.BaseURL)
		if aiCfg.BaseURL != "" {
			baseURLPrompt = fmt.Sprintf("Base URL (当前: %s, 留空保持): ", aiCfg.BaseURL)
		}
		baseURL, err := c.readLineWithPrompt(baseURLPrompt)
		if err != nil {
			return
		}
		if strings.TrimSpace(baseURL) != "" {
			aiCfg.BaseURL = strings.TrimSpace(baseURL)
		} else if aiCfg.BaseURL == "" {
			aiCfg.BaseURL = defaults.BaseURL
		}

		modelPrompt := fmt.Sprintf("模型名称 (留空使用默认 %s): ", defaults.Model)
		if aiCfg.Model != "" {
			modelPrompt = fmt.Sprintf("模型名称 (当前: %s, 留空保持): ", aiCfg.Model)
		}
		model, err := c.readLineWithPrompt(modelPrompt)
		if err != nil {
			return
		}
		if strings.TrimSpace(model) != "" {
			aiCfg.Model = strings.TrimSpace(model)
		} else if aiCfg.Model == "" {
			aiCfg.Model = defaults.Model
		}
	}

	if err := agent.SaveAIConfig(aiCfg); err != nil {
		fmt.Printf("\033[1;31m✗ 保存配置失败: %v\033[0m\n", err)
		return
	}

	fmt.Printf("\033[1;32m✔ 配置已保存 — 提供商: %s  模型: %s\033[0m\n", aiCfg.Provider, aiCfg.Model)
	if len(a) > 0 && a[0] != nil {
		if err := a[0].ReloadConfig(aiCfg); err != nil {
			c.printWarning(fmt.Sprintf("热重载失败: %v", err))
		} else {
			c.printInfo("AI 配置已热重载，无需重启")
		}
	}
	if provider == "hub" {
		c.printInfo("Hub 注册已完成；如需环境自检，请运行 /self-check")
	}
}

func mgmtBaseURL() string {
	port := config.MgmtPort
	if strings.HasPrefix(port, ":") {
		return "http://127.0.0.1" + port
	}
	return "http://" + port
}

func (c *CLI) handleHubEnable(enable bool) {
	if err := hub.SaveHubEnabled(enable); err != nil {
		c.printError(fmt.Sprintf("保存 Hub 配置失败: %v", err))
		return
	}
	if enable {
		c.printSuccess("Hub AI 网关已启用")
		c.printInfo("请重启代理服务使路由生效，然后在 Hub 上运行 /hub-token 生成注册 Token")
	} else {
		c.printSuccess("Hub AI 网关已禁用")
	}
	c.promptProxyRestart()
}

func (c *CLI) handleHubToken() {
	settings, _ := hub.LoadHubSettings()
	hubActive := settings.Enabled || buildinfo.IsHub()
	if !hubActive {
		c.printError("Hub 未启用，请运行 /hub-enable 后重启代理")
		return
	}

	// 优先走管理端口（代理进程内生成）
	if token, ok := c.fetchHubTokenViaHTTP(); ok {
		c.printHubToken(token)
		return
	}

	// 回退：CLI 本地生成并写入 configs/hub_pending_token.json，供代理进程校验注册
	if err := hub.LoadSpokes(); err != nil {
		c.printWarning(fmt.Sprintf("加载 spoke 注册表: %v", err))
	}
	token, err := hub.GenerateRegisterToken()
	if err != nil {
		c.printError(fmt.Sprintf("生成 Token 失败: %v", err))
		return
	}
	c.printHubToken(token)
	c.printWarning("管理端口 /hub/token 不可用（常见原因：旧版代理在运行或未用 Hub 包启动）")
	c.printInfo("请确保 Hub 网关已启动: ./ruoyi-proxy-linux-hub  或  /proxy-restart")
}

func (c *CLI) fetchHubTokenViaHTTP() (string, bool) {
	resp, err := http.Post(mgmtBaseURL()+"/hub/token", "application/json", nil)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var out map[string]interface{}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", false
	}
	token, _ := out["token"].(string)
	return token, token != ""
}

func (c *CLI) printHubToken(token string) {
	c.printSuccess("注册 Token 已生成（15 分钟内有效）")
	fmt.Printf("\033[1;36mToken: %s\033[0m\n", token)
	c.printInfo("在 spoke 服务器运行 /agent-config，选择 hub 并填入此 Token")
}

func (c *CLI) handleHubStatus() {
	var out struct {
		Count  int               `json:"count"`
		Spokes []hub.SpokeRecord `json:"spokes"`
	}

	resp, err := http.Get(mgmtBaseURL() + "/hub/status")
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if json.Unmarshal(body, &out) == nil {
			c.printHubStatusList(out.Count, out.Spokes)
			return
		}
	}
	if resp != nil {
		resp.Body.Close()
	}

	// 回退：读本地注册表
	if err := hub.LoadSpokes(); err != nil {
		c.printError(fmt.Sprintf("查询失败: %v", err))
		return
	}
	spokes := hub.ListSpokes()
	c.printHubStatusList(len(spokes), spokes)
}

func (c *CLI) handleHubSpoke(spokeID string) {
	spokeID = strings.TrimSpace(spokeID)
	if spokeID == "" {
		c.printError("请指定 spoke ID，例如: /hub-spoke spoke-abc12345")
		return
	}

	resp, err := http.Get(mgmtBaseURL() + "/hub/spoke?spoke=" + url.QueryEscape(spokeID))
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var item hub.SpokeRecord
		if json.Unmarshal(body, &item) == nil {
			c.printHubSpokeDetail(item)
			return
		}
	}
	if resp != nil {
		resp.Body.Close()
	}

	// 回退：读本地注册表
	if err := hub.LoadSpokes(); err != nil {
		c.printError(fmt.Sprintf("查询失败: %v", err))
		return
	}
	item, ok := hub.GetSpoke(spokeID)
	if !ok {
		c.printError("spoke 不存在: " + spokeID)
		return
	}
	c.printHubSpokeDetail(item)
}

// runFixNginxHub 提示用户直接让 AI 修复 Nginx Hub 路由
func (c *CLI) runFixNginxHub(a *agent.Agent) {
	if a == nil {
		c.printError("Agent 未启动")
		return
	}
	c.printInfo("请在下方输入框直接告诉 AI：请帮我修复 Nginx 的 Hub 路由")
	c.printInfo("AI 会读取配置、删除旧块、插入 location ^~ /__hub__/ 并验证 reload")
}

func (c *CLI) printHubStatusList(count int, spokes []hub.SpokeRecord) {
	fmt.Printf("\n\033[1;34m已注册 Spoke (%d)\033[0m\n", count)
	for _, s := range spokes {
		status := "活跃"
		if s.Revoked {
			status = "已吊销"
		}
		fmt.Printf("  \033[1;36m%s\033[0m  [%s]  创建: %s  最近: %s\n",
			s.ID, status, s.CreatedAt.Format("2006-01-02 15:04"), s.LastSeen.Format("2006-01-02 15:04"))
		if s.Profile != nil {
			p := s.Profile
			label := p.Label
			if label == "" {
				label = p.Hostname
			}
			fmt.Printf("      用途: %s  项目: %s (%s)\n", label, p.ProjectName, p.ProjectType)
			if p.Description != "" {
				fmt.Printf("      说明: %s\n", p.Description)
			}
			if p.Domain != "" {
				fmt.Printf("      域名: %s\n", p.Domain)
			}
			if len(p.Services) > 0 {
				fmt.Printf("      服务: ")
				for i, svc := range p.Services {
					if i > 0 {
						fmt.Print(", ")
					}
					fmt.Printf("%s(%s)", svc.ID, svc.ActiveEnv)
				}
				fmt.Println()
			}
		}
	}
	fmt.Println()
}

func (c *CLI) printHubSpokeDetail(s hub.SpokeRecord) {
	status := "活跃"
	if s.Revoked {
		status = "已吊销"
	}
	fmt.Printf("\n\033[1;34mSpoke 详情: %s\033[0m\n", s.ID)
	fmt.Printf("  状态: %s\n", status)
	fmt.Printf("  创建: %s\n", s.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  最近: %s\n", s.LastSeen.Format("2006-01-02 15:04:05"))
	if s.Profile == nil {
		fmt.Println("  档案: 未上报（请在 Spoke 端重新运行 /agent-config 或重启 CLI 触发引导）")
		fmt.Println()
		return
	}
	p := s.Profile
	label := p.Label
	if label == "" {
		label = p.Hostname
	}
	fmt.Printf("  用途: %s\n", label)
	fmt.Printf("  主机: %s\n", p.Hostname)
	if p.ProjectName != "" || p.ProjectType != "" {
		fmt.Printf("  项目: %s (%s)\n", p.ProjectName, p.ProjectType)
	}
	if p.Domain != "" {
		fmt.Printf("  域名: %s\n", p.Domain)
	}
	if p.AppHome != "" {
		fmt.Printf("  目录: %s\n", p.AppHome)
	}
	if p.Description != "" {
		fmt.Printf("  说明: %s\n", p.Description)
	}
	if !p.UpdatedAt.IsZero() {
		fmt.Printf("  档案更新时间: %s\n", p.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
	if len(p.Services) > 0 {
		fmt.Println("  服务:")
		for _, svc := range p.Services {
			fmt.Printf("    - %s  %s  %s  %s\n", svc.ID, svc.Name, svc.ProjectType, svc.ActiveEnv)
		}
	}
	fmt.Println()
}

func (c *CLI) handleHubRevoke(spokeID string) {
	if !c.confirmDangerAction(fmt.Sprintf("吊销 Spoke: %s", spokeID), []string{"吊销后该节点将无法再通过 Hub 调用 AI"}) {
		return
	}
	req, err := http.NewRequest(http.MethodPost, mgmtBaseURL()+"/hub/revoke?spoke="+spokeID, nil)
	if err != nil {
		c.printError(err.Error())
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.printError(fmt.Sprintf("请求失败: %v", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		c.printError(fmt.Sprintf("吊销失败 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
		return
	}
	c.printSuccess(fmt.Sprintf("Spoke[%s] 已吊销", spokeID))
}
