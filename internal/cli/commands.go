package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"ruoyi-proxy/internal/config"
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
	c.printInfo("获取系统状态..")

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
	fmt.Println("\033[1;34mϵͳ״̬\033[0m")
	fmt.Println(strings.Repeat("-", 70))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "[1;33m代理状态[0m	[1;32m%s[0m\n", status)

	fmt.Fprintf(w, "[1;33m服务数量:[0m	[1;36m%d[0m\n", len(services))

	fmt.Fprintf(w, "[1;33m代理端口:[0m	%s\n", config.ProxyPort)

	fmt.Fprintf(w, "[1;33m时间:[0m	%s\n", time.Now().Format("2006-01-02 15:04:05"))

	w.Flush()

	fmt.Println(strings.Repeat("-", 70))

	fmt.Println("\n\033[1;34m�����б�\033[0m")
	fmt.Println(strings.Repeat("-", 70))

	w2 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w2, "[1;33m  %-12s	%-15s	%-8s	%s[0m\n", "ID", "名称", "环境", "目标地址")

	fmt.Fprintf(w2, "  %s	%s	%s	%s\n", strings.Repeat("-", 12), strings.Repeat("-", 15), strings.Repeat("-", 8), strings.Repeat("-", 25))

	for _, svc := range services {
		envColor := "[1;34m"
		if svc.ActiveEnv == "green" {
			envColor = "[1;32m"
		}
		target := svc.BlueTarget
		if svc.ActiveEnv == "green" {
			target = svc.GreenTarget
		}
		fmt.Fprintf(w2, "  %-12s	%-15s	%s%-8s[0m	%s\n",

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

// QuickDeploy 快速部署向�?
func (c *CLI) QuickDeploy() {
	fmt.Println("\n\033[1;34m══�?快速部署向�?═══\033[0m\n")

	steps := []string{
		"准备AppCDS归档",
		"启动待机环境",
		"�������",
		"切换流量",
		"����ɻ���",
	}

	fmt.Println("部署步骤:")
	for i, step := range steps {
		fmt.Printf("  [%d/%d] %s\n", i+1, len(steps), step)
	}

	fmt.Print("\n\033[1;33m确认开始部�? (y/n): \033[0m")
	confirm, err := c.readLine()
	if err != nil {
		return
	}

	confirm = strings.ToLower(strings.TrimSpace(confirm))
	if confirm != "y" && confirm != "yes" {
		c.printInfo("��ȡ������")
		return
	}

	fmt.Println()
	c.executeScript("service.sh", "deploy")
}

// ShowLogs 显示日志（带颜色高亮�?
func (c *CLI) ShowLogs(lines string) {
	c.printInfo(fmt.Sprintf("�鿴���%s����־", lines))
	fmt.Println(strings.Repeat("─", 60))

	c.executeScript("service.sh", "logs", lines)
}

// InteractiveSwitch 交互式环境切�?
func (c *CLI) InteractiveSwitch() {
	fmt.Println("\n\033[1;34m=== �����л� ===\033[0m\n")

	cfg, err := c.loadProxyConfig()
	if err != nil {
		c.printError(fmt.Sprintf("��ȡ����ʧ��: %v", err))
		return
	}

	services := servicesFromConfig(cfg)
	if len(services) == 0 {
		c.printError("δ���÷���")
		return
	}

	fmt.Println("�����б�:")
	for i, svc := range services {
		envColor := "[1;34m"
		if svc.ActiveEnv == "green" {
			envColor = "[1;32m"
		}
		fmt.Printf("  %d. %s (%s) - ����: %s%s\033[0m\n", i+1, svc.Name, svc.ID, envColor, svc.ActiveEnv)

	}

	fmt.Println("\n�л���ʽ:")
	fmt.Println("  1. �л����з���")
	fmt.Println("  2. �л���������")
	fmt.Println("  0. ȡ��")

	choice, err := c.readLineWithPrompt("\n\033[1;33mѡ��: \033[0m")

	if err != nil {
		return
	}

	switch strings.TrimSpace(choice) {
	case "1":
		c.switchAllServices()
	case "2":
		c.switchSingleService(services)
	case "0":
		c.printInfo("��ȡ��")
	default:
		c.printError("��Чѡ��")
	}
}

// switchAllServices 切换所有服�?
func (c *CLI) switchAllServices() {
	env, err := c.readLineWithPrompt("[1;33m目标环境 (blue/green): [0m")
	if err != nil {
		return
	}
	env = strings.TrimSpace(env)
	if env != "blue" && env != "green" {
		c.printError("环境必须�?blue �?green")
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

	c.printSuccess(fmt.Sprintf("已切换所有服务到 %s (配置已更�?", env))
	c.promptProxyRestart()
}

// switchSingleService 切换单个服务
func (c *CLI) switchSingleService(services []ServiceStatus) {
	serviceID, err := c.readLineWithPrompt("[1;33m服务ID: [0m")
	if err != nil {
		return
	}
	serviceID = strings.TrimSpace(serviceID)

	found := false
	for _, svc := range services {
		if svc.ID == serviceID {
			found = true
			break
		}
	}
	if !found {
		c.printError(fmt.Sprintf("服务不存�? %s", serviceID))
		return
	}

	env, err := c.readLineWithPrompt("[1;33m目标环境 (blue/green): [0m")
	if err != nil {
		return
	}
	env = strings.TrimSpace(env)
	if env != "blue" && env != "green" {
		c.printError("环境必须�?blue �?green")
		return
	}

	c.printInfo(fmt.Sprintf("切换服务[%s]�?%s...", serviceID, env))

	cfg, err := c.loadProxyConfig()
	if err != nil {
		c.printError(fmt.Sprintf("读取配置失败: %v", err))
		return
	}
	svc := cfg.GetService(serviceID)
	if svc == nil {
		c.printError(fmt.Sprintf("服务不存�? %s", serviceID))
		return
	}
	svc.ActiveEnv = env
	if err := config.SaveConfig(cfg); err != nil {
		c.printError(fmt.Sprintf("保存配置失败: %v", err))
		return
	}

	c.printSuccess(fmt.Sprintf("服务[%s]已切换到 %s (配置已更�?", serviceID, env))
	c.promptProxyRestart()
}

// ShowSystemInfo 显示系统信息
func (c *CLI) ShowSystemInfo() {
	fmt.Println("\n\033[1;34m══�?系统信息 ═══\033[0m\n")

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
	fmt.Println("\n\033[1;34m══�?快捷命令 ═══\033[0m\n")

	commands := []struct {
		cmd  string
		desc string
	}{
		{"start", "启动服务"},
		{"stop", "停止服务"},
		{"restart", "重启服务"},
		{"deploy", "蓝绿部署"},
		{"status", "�鿴״̬"},
		{"logs", "查看日志"},
		{"switch", "����ʽ�л�����"},
		{"switch blue", "切换所有服务到blue"},
		{"switch green", "切换所有服务到green"},
		{"init", "��ʼ������"},
		{"cert <域名>", "申请SSL证书"},
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	for _, cmd := range commands {
		fmt.Fprintf(w, "  \033[1;36m%-20s\033[0m\t%s\n", cmd.cmd, cmd.desc)
	}
	w.Flush()
	fmt.Println()
}

// JVMConfig JVM配置管理
func (c *CLI) JVMConfig() {
	fmt.Println("\n\033[1;34m═══ JVM配置管理 ═══\033[0m\n")

	configFile := "configs/app_config.json"

	data, err := os.ReadFile(configFile)
	if err != nil {
		c.printError("配置文件不存在，请先运行 init 命令初始化")
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		c.printError("配置文件格式错误")
		return
	}

	jvm, ok := config["jvm"].(map[string]interface{})
	if !ok {
		c.printError("JVM配置不存在")
		return
	}

	// 显示当前配置
	currentPreset := int(jvm["preset"].(float64))
	customOpts := ""
	if co, ok := jvm["custom_opts"].(string); ok {
		customOpts = co
	}

	fmt.Printf("当前JVM预设档位: \033[1;36m%d\033[0m\n", currentPreset)
	if customOpts != "" {
		fmt.Printf("自定义参数: \033[1;36m%s\033[0m\n", customOpts)
	}

	// 显示预设配置
	presets, ok := jvm["presets"].(map[string]interface{})
	if !ok {
		c.printError("预设配置不存在")
		return
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

	choice, err := c.readLineWithPrompt("\n\033[1;33m请选择: \033[0m")
	if err != nil {
		return
	}

	switch strings.TrimSpace(choice) {
	case "1":
		c.switchJVMPreset(config, jvm, presets)
	case "2":
		c.setJVMCustomOpts(config, jvm)
	case "3":
		c.showJVMDetail(jvm)
	case "0":
		return
	default:
		c.printError("无效选择")
	}
}

// switchJVMPreset 切换JVM预设档位
func (c *CLI) switchJVMPreset(config map[string]interface{}, jvm map[string]interface{}, presets map[string]interface{}) {
	choice, err := c.readLineWithPrompt("\033[1;33m选择预设档位 (1-3): \033[0m")
	if err != nil {
		return
	}

	presetNum := strings.TrimSpace(choice)
	if presetNum != "1" && presetNum != "2" && presetNum != "3" {
		c.printError("无效的预设档位，必须是1、2或3")
		return
	}

	jvm["preset"] = float64([]int{1, 2, 3}[presetNum[0]-'1'])
	config["jvm"] = jvm

	// 保存配置
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		c.printError("配置序列化失败")
		return
	}

	if err := os.WriteFile("configs/app_config.json", data, 0644); err != nil {
		c.printError("保存配置文件失败")
		return
	}

	c.printSuccess(fmt.Sprintf("JVM预设已切换到档位 %s", presetNum))
	c.printInfo("重启Java应用后生效")
}

// setJVMCustomOpts 设置自定义JVM参数
func (c *CLI) setJVMCustomOpts(config map[string]interface{}, jvm map[string]interface{}) {
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
	config["jvm"] = jvm

	// 保存配置
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		c.printError("配置序列化失败")
		return
	}

	if err := os.WriteFile("configs/app_config.json", data, 0644); err != nil {
		c.printError("保存配置文件失败")
		return
	}

	c.printSuccess("自定义JVM参数已更新")
	c.printInfo("重启Java应用后生效")
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
