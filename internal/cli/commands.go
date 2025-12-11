package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"
)

// ServiceStatus 服务状态
type ServiceStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ActiveEnv   string `json:"active_env"`
	BlueTarget  string `json:"blue_target"`
	GreenTarget string `json:"green_target"`
}

// ProxyStatus 代理状态结构
type ProxyStatus struct {
	Status       string          `json:"status"`
	ServiceCount int             `json:"service_count"`
	Services     []ServiceStatus `json:"services"`
	ProxyPort    string          `json:"proxy_port"`
	MgmtPort     string          `json:"mgmt_port"`
	Time         string          `json:"time"`
	Version      string          `json:"version"`
}

// ShowDetailedStatus 显示详细状态
func (c *CLI) ShowDetailedStatus() {
	c.printInfo("获取系统状态...")

	resp, err := http.Get("http://localhost:8001/status")
	if err != nil {
		c.printError("无法连接到代理服务")
		c.printWarning("请确保代理服务已启动")
		return
	}
	defer resp.Body.Close()

	var status ProxyStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		c.printError("解析状态失败")
		return
	}

	fmt.Println("\n" + strings.Repeat("═", 70))
	fmt.Println("\033[1;34m系统状态\033[0m")
	fmt.Println(strings.Repeat("═", 70))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "\033[1;33m代理状态:\033[0m\t\033[1;32m%s\033[0m\n", status.Status)
	fmt.Fprintf(w, "\033[1;33m服务数量:\033[0m\t\033[1;36m%d\033[0m\n", status.ServiceCount)
	fmt.Fprintf(w, "\033[1;33m代理端口:\033[0m\t%s\n", status.ProxyPort)
	fmt.Fprintf(w, "\033[1;33m管理端口:\033[0m\t%s\n", status.MgmtPort)
	fmt.Fprintf(w, "\033[1;33m版本:\033[0m\t%s\n", status.Version)
	fmt.Fprintf(w, "\033[1;33m时间:\033[0m\t%s\n", status.Time)
	w.Flush()

	fmt.Println(strings.Repeat("═", 70))

	// 显示各服务状态
	fmt.Println("\n\033[1;34m服务列表\033[0m")
	fmt.Println(strings.Repeat("─", 70))

	w2 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w2, "\033[1;33m  %-12s\t%-15s\t%-8s\t%s\033[0m\n", "ID", "名称", "环境", "目标地址")
	fmt.Fprintf(w2, "  %s\t%s\t%s\t%s\n", strings.Repeat("-", 12), strings.Repeat("-", 15), strings.Repeat("-", 8), strings.Repeat("-", 25))

	for _, svc := range status.Services {
		envColor := "\033[1;34m" // blue
		if svc.ActiveEnv == "green" {
			envColor = "\033[1;32m" // green
		}
		target := svc.BlueTarget
		if svc.ActiveEnv == "green" {
			target = svc.GreenTarget
		}
		fmt.Fprintf(w2, "  %-12s\t%-15s\t%s%-8s\033[0m\t%s\n",
			svc.ID, svc.Name, envColor, svc.ActiveEnv, target)
	}
	w2.Flush()

	fmt.Println(strings.Repeat("─", 70))

	// 检查环境健康状态
	c.checkAllServicesHealth(status.Services)
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
		"清理旧环境",
	}

	fmt.Println("部署步骤:")
	for i, step := range steps {
		fmt.Printf("  [%d/%d] %s\n", i+1, len(steps), step)
	}

	fmt.Print("\n\033[1;33m确认开始部署? (y/n): \033[0m")
	confirm, err := c.readLine()
	if err != nil {
		return
	}

	confirm = strings.ToLower(strings.TrimSpace(confirm))
	if confirm != "y" && confirm != "yes" {
		c.printInfo("已取消部署")
		return
	}

	fmt.Println()
	c.executeScript("service.sh", "deploy")
}

// ShowLogs 显示日志（带颜色高亮）
func (c *CLI) ShowLogs(lines string) {
	c.printInfo(fmt.Sprintf("查看最近 %s 行日志", lines))
	fmt.Println(strings.Repeat("─", 60))

	c.executeScript("service.sh", "logs", lines)
}

// InteractiveSwitch 交互式环境切换
func (c *CLI) InteractiveSwitch() {
	fmt.Println("\n\033[1;34m═══ 环境切换 ═══\033[0m\n")

	// 获取当前状态
	resp, err := http.Get("http://localhost:8001/status")
	if err != nil {
		c.printError("无法连接到代理服务")
		return
	}
	defer resp.Body.Close()

	var status ProxyStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		c.printError("解析状态失败")
		return
	}

	if len(status.Services) == 0 {
		c.printError("未配置服务")
		return
	}

	// 显示服务列表
	fmt.Println("服务列表:")
	for i, svc := range status.Services {
		envColor := "\033[1;34m"
		if svc.ActiveEnv == "green" {
			envColor = "\033[1;32m"
		}
		fmt.Printf("  %d. %s (%s) - 当前: %s%s\033[0m\n", i+1, svc.Name, svc.ID, envColor, svc.ActiveEnv)
	}

	fmt.Println("\n选择操作:")
	fmt.Println("  1. 切换所有服务")
	fmt.Println("  2. 切换单个服务")
	fmt.Println("  0. 取消")

	choice, err := c.readLineWithPrompt("\n\033[1;33m请选择: \033[0m")
	if err != nil {
		return
	}

	switch strings.TrimSpace(choice) {
	case "1":
		c.switchAllServices()
	case "2":
		c.switchSingleService(status.Services)
	case "0":
		c.printInfo("已取消")
	default:
		c.printError("无效选择")
	}
}

// switchAllServices 切换所有服务
func (c *CLI) switchAllServices() {
	env, err := c.readLineWithPrompt("\033[1;33m目标环境 (blue/green): \033[0m")
	if err != nil {
		return
	}
	env = strings.TrimSpace(env)
	if env != "blue" && env != "green" {
		c.printError("环境必须是 blue 或 green")
		return
	}

	c.printInfo(fmt.Sprintf("切换所有服务到 %s...", env))

	resp, err := http.Post(fmt.Sprintf("http://localhost:8001/switch?env=%s", env), "", nil)
	if err != nil {
		c.printError("切换失败")
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["success"] == true {
		c.printSuccess(fmt.Sprintf("%v", result["message"]))
	} else {
		c.printError("切换失败")
	}
}

// switchSingleService 切换单个服务
func (c *CLI) switchSingleService(services []ServiceStatus) {
	serviceID, err := c.readLineWithPrompt("\033[1;33m服务ID: \033[0m")
	if err != nil {
		return
	}
	serviceID = strings.TrimSpace(serviceID)

	// 验证服务ID
	found := false
	for _, svc := range services {
		if svc.ID == serviceID {
			found = true
			break
		}
	}
	if !found {
		c.printError(fmt.Sprintf("服务不存在: %s", serviceID))
		return
	}

	env, err := c.readLineWithPrompt("\033[1;33m目标环境 (blue/green): \033[0m")
	if err != nil {
		return
	}
	env = strings.TrimSpace(env)
	if env != "blue" && env != "green" {
		c.printError("环境必须是 blue 或 green")
		return
	}

	c.printInfo(fmt.Sprintf("切换服务[%s]到 %s...", serviceID, env))

	resp, err := http.Post(fmt.Sprintf("http://localhost:8001/switch?service=%s&env=%s", serviceID, env), "", nil)
	if err != nil {
		c.printError("切换失败")
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["success"] == true {
		c.printSuccess(fmt.Sprintf("%v", result["message"]))
	} else {
		c.printError("切换失败")
	}
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
		{"status", "查看状态"},
		{"logs", "查看日志"},
		{"switch", "交互式切换环境"},
		{"switch blue", "切换所有服务到blue"},
		{"switch green", "切换所有服务到green"},
		{"init", "初始化环境"},
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
