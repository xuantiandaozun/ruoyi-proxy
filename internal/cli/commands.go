package cli

import (
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

// MonitorMode 监控模式
func (c *CLI) MonitorMode() {
	fmt.Println("\n\033[1;34m══�?监控模式 ═══\033[0m")
	fmt.Println("�?Ctrl+C 退出监控\n")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.clearScreen()
			fmt.Println("\033[1;34m══�?实时监控 ═══\033[0m")
			fmt.Printf("更新时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
			c.ShowDetailedStatus()
		}
	}
}
