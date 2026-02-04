package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/chzyer/readline"
	"golang.org/x/term"
)

// CLI 交互式命令行界面
type CLI struct {
	rl             *readline.Instance
	running        bool
	proxyPID       int    // 保存代理进程的PID
	currentService string // 当前操作的服务ID
}

// New 创建CLI实例
func New() *CLI {
	return &CLI{
		running:        true,
		currentService: "default",
	}
}

// Start 启动交互式终端
func (c *CLI) Start() {
	// 初始化 readline
	var err error
	c.rl, err = readline.New("\033[1;36mruoyi>\033[0m ")
	if err != nil {
		fmt.Println("错误: 初始化输入系统失败 -", err)
		return
	}
	defer c.rl.Close()

	// 设置自动完成
	c.rl.Config.AutoComplete = readline.NewPrefixCompleter(
		readline.PcItem("help"),
		readline.PcItem("start"),
		readline.PcItem("stop"),
		readline.PcItem("restart"),
		readline.PcItem("deploy"),
		readline.PcItem("status"),
		readline.PcItem("exit"),
		readline.PcItem("clear"),
		readline.PcItem("config"),
		readline.PcItem("config-edit"),
		readline.PcItem("logs"),
		readline.PcItem("logs-follow"),
		readline.PcItem("logs-search", readline.PcItemDynamic(func(line string) []string {
			return c.logFileCompletionItems(line)
		})),
		readline.PcItem("logs-export", readline.PcItemDynamic(func(line string) []string {
			return c.logFileCompletionItems(line)
		})),
		readline.PcItem("init"),
		readline.PcItem("cert"),
		readline.PcItem("enable-https"),
		readline.PcItem("disable-https"),
		readline.PcItem("proxy-start"),
		readline.PcItem("proxy-stop"),
		readline.PcItem("proxy-status"),
		readline.PcItem("switch"),
		readline.PcItem("detail"),
		readline.PcItem("quick"),
		readline.PcItem("info"),
		readline.PcItem("monitor"),
		readline.PcItem("quick-deploy"),
		readline.PcItem("service-add"),
		readline.PcItem("service-list"),
		readline.PcItem("service-remove"),
		readline.PcItem("service-switch"),
	)

	// 初始化脚本和配置文件
	if err := c.InitializeFiles(); err != nil {
		c.printError(fmt.Sprintf("初始化文件失败: %v", err))
	}

	c.printBanner()
	c.printHelp()

	// 设置信号处理，捕获 Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println()
		c.running = false
	}()

	for c.running {
		input, err := c.rl.Readline()
		if err != nil {
			if err.Error() == "Interrupt" {
				fmt.Println()
				c.running = false
				break
			}
			break
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		c.handleCommand(input)
	}

	fmt.Println("\n再见！")
}

// readLine 读取一行输入
func (c *CLI) readLine() (string, error) {
	line, err := c.rl.Readline()
	if err != nil {
		return "", err
	}
	return line, nil
}

// readLineWithPrompt 使用自定义提示符读取一行输入
func (c *CLI) readLineWithPrompt(prompt string) (string, error) {
	oldPrompt := c.rl.Config.Prompt
	c.rl.SetPrompt(prompt)
	line, err := c.rl.Readline()
	c.rl.SetPrompt(oldPrompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// printBanner 打印欢迎横幅
func (c *CLI) printBanner() {
	banner := `
╔════════════════════════════════════════════════════════╗
║                                                        ║
║      若依蓝绿部署管理面板 v1.0                        ║
║      Ruoyi Blue-Green Deployment Manager              ║
║                                                        ║
╚════════════════════════════════════════════════════════╝
`
	fmt.Println("\033[1;34m" + banner + "\033[0m")
	fmt.Printf("当前服务: \033[1;32m%s\033[0m\n", c.currentService)
	fmt.Println("输入 '\033[1;33mhelp\033[0m' 查看所有命令")
}

// printHelp 打印帮助信息
func (c *CLI) printHelp() {
	fmt.Println()
	fmt.Println("\033[1;32m可用命令:\033[0m")
	fmt.Println()
	fmt.Println("  \033[1;33m服务管理:\033[0m")
	fmt.Println("    start          - 启动服务")
	fmt.Println("    stop           - 停止服务")
	fmt.Println("    restart        - 重启服务")
	fmt.Println("    deploy         - 蓝绿部署新版本")
	fmt.Println("    quick-deploy   - 快速部署向导")
	fmt.Println("    status         - 查看服务状态")
	fmt.Println("    detail         - 查看详细状态")
	fmt.Println("    logs [行数]    - 查看日志（默认600行）")
	fmt.Println("    logs-follow    - 实时查看日志")
	fmt.Println("    logs-search [日志名/日志文件] [关键字] [行数] | logs-export [日志名/日志文件] [输出名] - 日志查询/导出（行数默认600）")
	fmt.Println("    logs-search|logs-export / - 进入日志文件选择器；? - 列出日志文件")
	fmt.Println()
	fmt.Println("  \033[1;33m环境管理:\033[0m")
	fmt.Println("    init           - 完整初始化（环境安装+服务配置+启动）")
	fmt.Println("    cert <域名>    - 申请SSL证书")
	fmt.Println("    enable-https   - 开启HTTPS（切换到HTTPS配置）")
	fmt.Println("    disable-https  - 关闭HTTPS（切换到HTTP配置）")
	fmt.Println()
	fmt.Println("  \033[1;33m代理管理:\033[0m")
	fmt.Println("    proxy-start    - 启动代理服务")
	fmt.Println("    proxy-stop     - 停止代理服务")
	fmt.Println("    proxy-status   - 查看代理状态")
	fmt.Println("    switch [env]   - 切换环境（不带参数则交互式选择）")
	fmt.Println()
	fmt.Println("  \033[1;33m服务管理:\033[0m")
	fmt.Println("    service-add    - 添加新服务")
	fmt.Println("    service-list   - 查看服务列表")
	fmt.Println("    service-remove - 删除服务")
	fmt.Println("    service-switch - 切换当前服务")
	fmt.Println()
	fmt.Println("  \033[1;33m配置管理:\033[0m")
	fmt.Println("    config         - 查看完整配置")
	fmt.Println("    config-edit    - 编辑配置")
	fmt.Println()
	fmt.Println("  \033[1;33m系统信息:\033[0m")
	fmt.Println("    info           - 显示系统信息")
	fmt.Println("    monitor        - 实时监控模式")
	fmt.Println("    quick          - 显示快捷命令列表")
	fmt.Println()
	fmt.Println("  \033[1;33m其他:\033[0m")
	fmt.Println("    help           - 显示此帮助信息")
	fmt.Println("    clear          - 清屏")
	fmt.Println("    exit           - 退出管理面板")
	fmt.Println()
	fmt.Println("\033[1;36m提示:\033[0m 大部分命令支持简写，例如 'h' = 'help', 'q' = 'exit'")
	fmt.Println()
}

// handleCommand 处理命令
func (c *CLI) handleCommand(input string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "help", "h", "?":
		c.printHelp()

	case "clear", "cls":
		c.clearScreen()

	case "exit", "quit", "q":
		c.running = false

	case "start":
		c.executeServiceCommand("start")

	case "stop":
		c.executeServiceCommand("stop")

	case "restart":
		c.executeServiceCommand("restart")

	case "deploy":
		c.executeServiceCommand("deploy")

	case "status":
		c.executeServiceCommand("status")

	case "logs":
		lines := "600"
		if len(args) > 0 {
			lines = args[0]
		}
		c.executeServiceLogCommand("logs", lines)

	case "logs-follow":
		c.executeServiceLogCommand("logs-follow")

	case "logs-search":
		if len(args) == 0 {
			c.interactiveLogsSearch()
			return
		}
		if c.isLogsSelectorHint(args[0]) {
			c.interactiveLogsSearch()
			return
		}
		if c.isLogsListHint(args[0]) {
			c.printLogFileList()
			return
		}
		if len(args) == 1 && !c.isDateArg(args[0]) {
			c.interactiveLogsSearchWithBase(args[0])
			return
		}
		c.executeServiceLogCommand("logs-search", args...)

	case "logs-export":
		if len(args) == 0 {
			c.interactiveLogsExport()
			return
		}
		if c.isLogsSelectorHint(args[0]) {
			c.interactiveLogsExport()
			return
		}
		if c.isLogsListHint(args[0]) {
			c.printLogFileList()
			return
		}
		if len(args) == 1 && !c.isDateArg(args[0]) {
			c.interactiveLogsExportWithBase(args[0])
			return
		}
		c.executeServiceLogCommand("logs-export", args...)

	case "init":
		c.handleInit()

	case "cert":
		c.handleCert(args)

	case "enable-https":
		c.EnableHTTPS()

	case "disable-https":
		c.DisableHTTPS()

	case "proxy-start":
		c.startProxyService()

	case "proxy-stop":
		c.stopProxyService()

	case "proxy-status":
		c.getProxyStatus()

	case "switch":
		if len(args) == 0 {
			c.InteractiveSwitch()
			return
		}
		env := args[0]
		if env != "blue" && env != "green" {
			c.printError("环境必须是 blue 或 green")
			return
		}
		c.switchEnvironment(env)

	case "detail", "detailed":
		c.ShowDetailedStatus()

	case "quick":
		c.ShowQuickCommands()

	case "info", "sysinfo":
		c.ShowSystemInfo()

	case "monitor", "watch":
		c.MonitorMode()

	case "quick-deploy":
		c.QuickDeploy()

	case "config":
		c.ShowConfig()

	case "config-edit":
		c.EditConfig()

	case "service-add":
		c.addService()

	case "service-list":
		c.listServices()

	case "service-remove":
		c.removeService()

	case "service-switch":
		c.switchService()

	default:
		c.printError(fmt.Sprintf("未知命令: %s", cmd))
		fmt.Println("输入 'help' 查看所有可用命令")
	}
}

// executeScript 执行脚本
func (c *CLI) executeScript(args ...string) {
	if len(args) == 0 {
		return
	}

	scriptName := args[0]
	scriptArgs := args[1:]

	// 查找脚本路径
	scriptPath := c.findScript(scriptName)
	if scriptPath == "" {
		c.printError(fmt.Sprintf("未找到脚本: %s", scriptName))
		return
	}

	c.printInfo(fmt.Sprintf("执行: bash %s %s", scriptPath, strings.Join(scriptArgs, " ")))
	fmt.Println(strings.Repeat("─", 60))

	// 执行脚本
	cmdArgs := append([]string{scriptPath}, scriptArgs...)
	cmd := exec.Command("bash", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		c.printError(fmt.Sprintf("执行失败: %v", err))
	}

	fmt.Println(strings.Repeat("─", 60))
}

// findScript 查找脚本文件
func (c *CLI) findScript(name string) string {
	// 可能的脚本路径
	paths := []string{
		"scripts/" + name,
		"./scripts/" + name,
		"../scripts/" + name,
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// startProxyService 启动代理服务
func (c *CLI) startProxyService() {
	c.printInfo("启动代理服务...")

	// 检查是否已经在运行
	if c.isProxyRunning() {
		c.printWarning("代理服务已经在运行中")
		return
	}

	// 查找可执行文件
	proxyBin := c.findProxyBinary()
	if proxyBin == "" {
		c.printError("未找到代理程序，请先编译: make build")
		return
	}

	// 后台启动代理服务
	cmd := exec.Command(proxyBin)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		c.printError(fmt.Sprintf("启动失败: %v", err))
		return
	}

	// 保存进程PID
	c.proxyPID = cmd.Process.Pid
	c.printInfo(fmt.Sprintf("代理进程PID: %d", c.proxyPID))

	// 等待服务启动
	c.printInfo("等待服务启动...")
	time.Sleep(2 * time.Second)

	// 验证服务是否启动成功
	if c.isProxyRunning() {
		c.printSuccess("代理服务已启动")
		c.printInfo("管理端口: http://localhost:8001")
		c.printInfo("代理端口: http://localhost:8080")
	} else {
		c.printError("代理服务启动失败，请检查日志")
	}
}

// stopProxyService 停止代理服务
func (c *CLI) stopProxyService() {
	c.printInfo("停止代理服务...")

	// 检查服务是否在运行
	if !c.isProxyRunning() {
		c.printWarning("代理服务未运行")
		return
	}

	// 方法1: 如果有保存的PID，先尝试用PID停止
	if c.proxyPID > 0 {
		c.printInfo(fmt.Sprintf("尝试使用PID %d 停止进程...", c.proxyPID))
		killCmd := exec.Command("kill", "-15", fmt.Sprintf("%d", c.proxyPID))
		killCmd.Run()
		time.Sleep(1 * time.Second)

		// 检查是否成功
		if !c.isProxyRunning() {
			c.printSuccess("代理服务已停止")
			c.proxyPID = 0
			return
		}

		// 如果还在运行，强制杀死
		forceKillCmd := exec.Command("kill", "-9", fmt.Sprintf("%d", c.proxyPID))
		forceKillCmd.Run()
		time.Sleep(500 * time.Millisecond)

		if !c.isProxyRunning() {
			c.printSuccess("代理服务已停止")
			c.proxyPID = 0
			return
		}
	}

	// 方法2: 通过端口查找并停止进程
	if c.killProxyByPort() {
		c.printSuccess("代理服务已停止")
		c.proxyPID = 0
	} else {
		c.printError("停止失败，请手动检查进程")
	}
}

// getProxyStatus 获取代理状态
func (c *CLI) getProxyStatus() {
	c.printInfo("查询代理状态...")

	// 检查服务是否运行
	if c.isProxyRunning() {
		fmt.Printf("\033[1;32m● 代理服务运行中\033[0m\n")
	} else {
		fmt.Printf("\033[1;31m● 代理服务未运行\033[0m\n")
	}

	fmt.Println()

	// 尝试获取详细状态
	cmd := exec.Command("curl", "-s", "http://localhost:8001/status")
	output, err := cmd.Output()
	if err != nil {
		c.printWarning("无法连接到管理接口")
		return
	}

	fmt.Println(string(output))
}

// switchEnvironment 切换环境
func (c *CLI) switchEnvironment(env string) {
	c.printInfo(fmt.Sprintf("切换到 %s 环境...", env))

	cmd := exec.Command("curl", "-s", "-X", "POST", fmt.Sprintf("http://localhost:8001/switch?env=%s", env))
	output, err := cmd.Output()
	if err != nil {
		c.printError("切换失败")
		return
	}

	fmt.Println(string(output))
	c.printSuccess(fmt.Sprintf("已切换到 %s 环境", env))
}

// confirmAndExecute 确认后执行
func (c *CLI) confirmAndExecute(action string, fn func()) {
	prompt := fmt.Sprintf("\033[1;33m确认要执行: %s? (y/n): \033[0m", action)
	confirm, err := c.readLineWithPrompt(prompt)
	if err != nil {
		return
	}

	confirm = strings.ToLower(confirm)
	if confirm == "y" || confirm == "yes" {
		fn()
	} else {
		c.printInfo("已取消")
	}
}

// clearScreen 清屏
func (c *CLI) clearScreen() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
	c.printBanner()
}

// 输出辅助函数
func (c *CLI) printSuccess(msg string) {
	fmt.Printf("\033[1;32m✓ %s\033[0m\n", msg)
}

func (c *CLI) printError(msg string) {
	fmt.Printf("\033[1;31m✗ %s\033[0m\n", msg)
}

func (c *CLI) printInfo(msg string) {
	fmt.Printf("\033[1;36mℹ %s\033[0m\n", msg)
}

func (c *CLI) printWarning(msg string) {
	fmt.Printf("\033[1;33m⚠ %s\033[0m\n", msg)
}

// ProgressBar 进度条
type ProgressBar struct {
	total   int
	current int
	width   int
}

// NewProgressBar 创建进度条
func NewProgressBar(total int) *ProgressBar {
	return &ProgressBar{
		total: total,
		width: 50,
	}
}

// Update 更新进度
func (p *ProgressBar) Update(current int) {
	p.current = current
	percent := float64(current) / float64(p.total) * 100
	filled := int(float64(p.width) * float64(current) / float64(p.total))

	bar := strings.Repeat("█", filled) + strings.Repeat("░", p.width-filled)
	fmt.Printf("\r[%s] %.1f%% (%d/%d)", bar, percent, current, p.total)

	if current >= p.total {
		fmt.Println()
	}
}

// Spinner 加载动画
type Spinner struct {
	frames []string
	index  int
	active bool
}

// NewSpinner 创建加载动画
func NewSpinner() *Spinner {
	return &Spinner{
		frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		active: false,
	}
}

// Start 启动动画
func (s *Spinner) Start(msg string) {
	s.active = true
	go func() {
		for s.active {
			fmt.Printf("\r\033[1;36m%s\033[0m %s", s.frames[s.index], msg)
			s.index = (s.index + 1) % len(s.frames)
			time.Sleep(100 * time.Millisecond)
		}
	}()
}

// Stop 停止动画
func (s *Spinner) Stop() {
	s.active = false
	fmt.Print("\r\033[K") // 清除当前行
}

// EnableHTTPS 开启HTTPS
func (c *CLI) EnableHTTPS() {
	c.printInfo("开启HTTPS模式...")

	// 检查证书是否存在
	if !c.checkSSLCertificate() {
		c.printError("未找到SSL证书，请先使用 'cert <域名>' 命令申请证书")
		return
	}

	c.confirmAndExecute("切换到HTTPS配置", func() {
		// 执行配置脚本
		c.executeScript("configure-nginx.sh", "true")

		// 更新配置文件
		if err := c.updateHTTPSConfig(true); err != nil {
			c.printError(fmt.Sprintf("更新配置文件失败: %v", err))
			return
		}

		c.printSuccess("HTTPS已开启")
		c.printInfo("Nginx已切换到HTTPS配置，HTTP请求将自动重定向到HTTPS")
	})
}

// DisableHTTPS 关闭HTTPS
func (c *CLI) DisableHTTPS() {
	c.printInfo("关闭HTTPS模式...")

	c.confirmAndExecute("切换到HTTP配置", func() {
		// 执行配置脚本
		c.executeScript("configure-nginx.sh", "false")

		// 更新配置文件
		if err := c.updateHTTPSConfig(false); err != nil {
			c.printError(fmt.Sprintf("更新配置文件失败: %v", err))
			return
		}

		c.printSuccess("HTTPS已关闭")
		c.printInfo("Nginx已切换到HTTP配置")
	})
}

// checkSSLCertificate 检查SSL证书是否存在
func (c *CLI) checkSSLCertificate() bool {
	configPath := c.findConfigFile()
	if configPath == "" {
		return false
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	// 简单解析获取域名和证书路径
	content := string(data)

	// 提取域名
	domainStart := strings.Index(content, `"domain"`)
	if domainStart == -1 {
		return false
	}
	domainLine := content[domainStart:]
	domainEnd := strings.Index(domainLine, ",")
	if domainEnd == -1 {
		domainEnd = strings.Index(domainLine, "\n")
	}
	domainPart := domainLine[:domainEnd]
	domain := strings.Trim(strings.Split(domainPart, ":")[1], ` "`)

	// 提取证书路径
	certPathStart := strings.Index(content, `"cert_path"`)
	if certPathStart == -1 {
		return false
	}
	certPathLine := content[certPathStart:]
	certPathEnd := strings.Index(certPathLine, ",")
	if certPathEnd == -1 {
		certPathEnd = strings.Index(certPathLine, "\n")
	}
	certPathPart := certPathLine[:certPathEnd]
	certPath := strings.Trim(strings.Split(certPathPart, ":")[1], ` "`)

	// 检查证书文件
	certFile := fmt.Sprintf("%s/%s.pem", certPath, domain)
	keyFile := fmt.Sprintf("%s/%s.key", certPath, domain)

	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return false
	}
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return false
	}

	return true
}

// updateHTTPSConfig 更新配置文件中的HTTPS设置
func (c *CLI) updateHTTPSConfig(enable bool) error {
	configPath := c.findConfigFile()
	if configPath == "" {
		return fmt.Errorf("未找到配置文件")
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	// 简单的字符串替换更新enable_https字段
	content := string(data)
	if enable {
		content = strings.Replace(content, `"enable_https": false`, `"enable_https": true`, 1)
	} else {
		content = strings.Replace(content, `"enable_https": true`, `"enable_https": false`, 1)
	}

	// 写回文件
	return os.WriteFile(configPath, []byte(content), 0644)
}

// findConfigFile 查找配置文件
func (c *CLI) findConfigFile() string {
	paths := []string{
		"configs/app_config.json",
		"./configs/app_config.json",
		"../configs/app_config.json",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// findProxyBinary 查找代理程序
func (c *CLI) findProxyBinary() string {
	paths := []string{
		// Makefile 构建的文件名（优先查找当前目录）
		"./ruoyi-proxy-linux", // Linux版本（当前目录）
		"./ruoyi-proxy",       // Windows版本（当前目录）
		"./ruoyi-proxy.exe",
		"./bin/ruoyi-proxy-linux", // bin目录
		"./bin/ruoyi-proxy",
		"./bin/ruoyi-proxy.exe",
		"bin/ruoyi-proxy-linux",
		"bin/ruoyi-proxy",
		"bin/ruoyi-proxy.exe",
		// 旧的文件名（向后兼容）
		"./proxy",
		"./bin/proxy",
		"./proxy.exe",
		"./bin/proxy.exe",
		"bin/proxy",
		"bin/proxy.exe",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// isProxyRunning 检查代理服务是否运行（通过健康检查接口）
func (c *CLI) isProxyRunning() bool {
	cmd := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "http://localhost:8001/health")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return string(output) == "200"
}

// killProxyByPort 通过端口查找并停止进程
func (c *CLI) killProxyByPort() bool {
	var pid string

	// 方法1: 尝试使用 lsof (需要正确的参数格式)
	lsofCmd := exec.Command("lsof", "-t", "-i:8001")
	output, err := lsofCmd.Output()
	if err == nil && len(output) > 0 {
		pid = strings.TrimSpace(string(output))
	}

	// 方法2: 如果 lsof 失败，尝试使用 ss 命令
	if pid == "" {
		ssCmd := exec.Command("sh", "-c", "ss -tlnp | grep :8001 | grep -oP 'pid=\\K[0-9]+'")
		output, err = ssCmd.Output()
		if err == nil && len(output) > 0 {
			pid = strings.TrimSpace(string(output))
		}
	}

	// 方法3: 如果 ss 也失败，尝试使用 netstat
	if pid == "" {
		netstatCmd := exec.Command("sh", "-c", "netstat -tlnp 2>/dev/null | grep :8001 | awk '{print $7}' | cut -d/ -f1")
		output, err = netstatCmd.Output()
		if err == nil && len(output) > 0 {
			pid = strings.TrimSpace(string(output))
		}
	}

	// 如果所有方法都失败
	if pid == "" || pid == "-" {
		c.printWarning("未找到占用8001端口的进程")
		c.printInfo("提示: 请尝试手动执行 'netstat -tlnp | grep 8001' 或 'lsof -i:8001' 查看进程")
		return false
	}

	c.printInfo(fmt.Sprintf("找到进程 PID: %s", pid))

	// 先尝试优雅关闭（SIGTERM）
	killCmd := exec.Command("kill", "-15", pid)
	killCmd.Run()

	// 等待进程关闭
	time.Sleep(1 * time.Second)

	// 检查进程是否还在运行
	checkCmd := exec.Command("kill", "-0", pid)
	if checkCmd.Run() != nil {
		// 进程已关闭
		c.printSuccess(fmt.Sprintf("进程 %s 已停止", pid))
		return true
	}

	// 如果还在运行，强制杀死（SIGKILL）
	c.printWarning(fmt.Sprintf("进程 %s 未响应，强制停止...", pid))
	forceKillCmd := exec.Command("kill", "-9", pid)
	if err := forceKillCmd.Run(); err != nil {
		c.printWarning(fmt.Sprintf("强制杀死进程失败: %v", err))
		return false
	}

	time.Sleep(500 * time.Millisecond)
	return !c.isProxyRunning()
}

// handleInit 处理初始化命令
func (c *CLI) handleInit() {
	configPath := c.findConfigFile()
	configExists := configPath != ""

	if configExists {
		// 配置文件已存在，显示并询问
		c.handleExistingConfig(configPath)
	} else {
		// 配置文件不存在，运行初始化脚本
		c.printInfo("首次初始化系统...")
		c.executeScript("init.sh")
	}
}

// handleExistingConfig 处理已存在的配置文件
func (c *CLI) handleExistingConfig(configPath string) {
	fmt.Println("\n\033[1;34m═══ 初始化向导 ═══\033[0m")
	fmt.Println("检测到配置文件已存在，选择操作:")
	fmt.Println()

	// 读取并显示当前配置
	data, err := os.ReadFile(configPath)
	if err != nil {
		c.printError("读取配置文件失败")
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		c.printError("解析配置文件失败")
		return
	}

	c.printInfo("当前配置摘要:")
	fmt.Println(strings.Repeat("─", 60))

	if domain, ok := config["domain"].(string); ok {
		fmt.Printf("  域名: \033[1;36m%s\033[0m\n", domain)
	}

	if proxy, ok := config["proxy"].(map[string]interface{}); ok {
		if blue, ok := proxy["blue_target"].(string); ok {
			fmt.Printf("  蓝色环境: \033[1;33m%s\033[0m\n", blue)
		}
		if green, ok := proxy["green_target"].(string); ok {
			fmt.Printf("  绿色环境: \033[1;33m%s\033[0m\n", green)
		}
		if active, ok := proxy["active_env"].(string); ok {
			fmt.Printf("  活跃环境: \033[1;32m%s\033[0m\n", active)
		}
	}

	if sync, ok := config["sync"].(map[string]interface{}); ok {
		if enabled, ok := sync["enabled"].(bool); ok {
			status := "未启用"
			if enabled {
				status = "已启用"
			}
			fmt.Printf("  文件同步: %s\n", status)
		}
	}

	fmt.Println(strings.Repeat("─", 60))

	fmt.Println("\n选择操作:")
	fmt.Println("  1. 重新初始化（覆盖现有配置）")
	fmt.Println("  2. 编辑配置文件")
	fmt.Println("  3. 查看完整配置")
	fmt.Println("  4. 取消")

	choice, err := c.readLineWithPrompt("\n\033[1;33m请选择 (1-4): \033[0m")
	if err != nil {
		return
	}

	switch choice {
	case "1":
		c.printInfo("重新初始化系统...")
		c.executeScript("init.sh")

	case "2":
		c.EditConfig()

	case "3":
		c.ShowConfig()

	case "4":
		c.printInfo("已取消")

	default:
		c.printError("无效选择")
	}
}

// handleCert 处理证书申请命令
func (c *CLI) handleCert(args []string) {
	// 先尝试从配置文件读取域名
	configPath := c.findConfigFile()
	var configDomain string

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err == nil {
			var config map[string]interface{}
			if err := json.Unmarshal(data, &config); err == nil {
				if domain, ok := config["domain"].(string); ok && domain != "" && domain != "example.com" {
					configDomain = domain
				}
			}
		}
	}

	var domain string

	if configDomain != "" {
		// 配置文件中有域名，询问是否使用
		c.printInfo(fmt.Sprintf("检测到配置文件中的域名: %s", configDomain))

		choice, err := c.readLineWithPrompt("\033[1;33m是否为此域名申请证书? (y/n): \033[0m")
		if err != nil {
			return
		}

		choice = strings.ToLower(choice)

		if choice == "y" || choice == "yes" {
			domain = configDomain
		} else {
			// 用户选择不使用配置文件中的域名，手动输入
			input, err := c.readLineWithPrompt("\033[1;33m请输入要申请证书的域名: \033[0m")
			if err != nil {
				return
			}

			domain = input
			if domain == "" {
				c.printError("域名不能为空")
				return
			}
		}
	} else if len(args) > 0 {
		// 命令行直接指定了域名
		domain = strings.Join(args, " ")
	} else {
		// 没有配置文件域名也没有命令行参数，需要手动输入
		input, err := c.readLineWithPrompt("\033[1;33m请输入要申请证书的域名 (例如: example.com): \033[0m")
		if err != nil {
			return
		}

		domain = input
		if domain == "" {
			c.printError("域名不能为空")
			return
		}
	}

	// 申请证书
	c.printInfo(fmt.Sprintf("申请证书: %s", domain))
	c.executeScript("https.sh", domain)
}

// addService 添加新服务
func (c *CLI) addService() {
	fmt.Println("\n\033[1;34m═══ 添加新服务 ═══\033[0m\n")

	// 服务ID
	serviceID, err := c.readLineWithPrompt("\033[1;33m服务ID (英文标识，如 admin/collector): \033[0m")
	if err != nil || serviceID == "" {
		c.printError("服务ID不能为空")
		return
	}

	// 检查服务ID是否已存在
	cmd := exec.Command("curl", "-s", "http://localhost:8001/status")
	output, err := cmd.Output()
	if err == nil {
		var status map[string]interface{}
		if err := json.Unmarshal(output, &status); err == nil {
			if services, ok := status["services"].([]interface{}); ok {
				for _, s := range services {
					if svc, ok := s.(map[string]interface{}); ok {
						if id, ok := svc["id"].(string); ok && id == serviceID {
							c.printError(fmt.Sprintf("服务ID[%s]已存在", serviceID))
							return
						}
					}
				}
			}
		}
	}

	// 服务名称
	serviceName, err := c.readLineWithPrompt("\033[1;33m服务名称 (显示名): \033[0m")
	if err != nil || serviceName == "" {
		serviceName = serviceID
	}

	// JAR文件名模式(必填)
	defaultJarPattern := fmt.Sprintf("ruoyi-%s-*.jar", serviceID)
	jarFilePrompt := fmt.Sprintf("\033[1;33mJAR文件名模式 (用于匹配带时间戳的JAR,默认: %s): \033[0m", defaultJarPattern)
	jarFile, err := c.readLineWithPrompt(jarFilePrompt)
	if err != nil || jarFile == "" {
		jarFile = defaultJarPattern
		c.printInfo(fmt.Sprintf("使用默认JAR模式: %s", jarFile))
	}

	// 检查JAR文件名是否和默认服务冲突
	if jarFile == "ruoyi-*.jar" || jarFile == "ruoyi-[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-*.jar" {
		c.printError("JAR文件名模式不能和默认服务冲突")
		c.printInfo("默认服务使用: ruoyi-[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-*.jar (匹配 ruoyi-YYYYMMDD-HHMMSS.jar)")
		c.printInfo(fmt.Sprintf("建议使用: %s", defaultJarPattern))
		return
	}

	// APP名称（用于生成PID文件等）
	appName, err := c.readLineWithPrompt("\033[1;33mAPP名称 (用于PID文件等，默认与服务ID相同): \033[0m")
	if err != nil || appName == "" {
		appName = serviceID
	}

	// 蓝色环境端口
	bluePort, err := c.readLineWithPrompt("\033[1;33m蓝色环境端口 (如 8080): \033[0m")
	if err != nil || bluePort == "" {
		c.printError("端口不能为空")
		return
	}

	// 绿色环境端口
	greenPort, err := c.readLineWithPrompt("\033[1;33m绿色环境端口 (如 8081): \033[0m")
	if err != nil || greenPort == "" {
		c.printError("端口不能为空")
		return
	}

	// 调用API添加服务
	payload := fmt.Sprintf(`{"id":"%s","name":"%s","blue_target":"http://127.0.0.1:%s","green_target":"http://127.0.0.1:%s","jar_file":"%s","app_name":"%s"}`,
		serviceID, serviceName, bluePort, greenPort, jarFile, appName)

	cmd = exec.Command("curl", "-s", "-X", "POST", "-H", "Content-Type: application/json",
		"-d", payload, "http://localhost:8001/service/add")
	output, err = cmd.Output()
	if err != nil {
		c.printError("添加失败，请确保代理服务已启动")
		return
	}

	fmt.Println(string(output))
	c.printSuccess(fmt.Sprintf("服务[%s]添加成功", serviceID))

	// 询问是否切换到新服务
	confirm, err := c.readLineWithPrompt("\033[1;33m是否切换到新服务? (y/n): \033[0m")
	if err == nil && (confirm == "y" || confirm == "Y" || confirm == "yes") {
		c.currentService = serviceID
		c.printSuccess(fmt.Sprintf("已切换到服务[%s]", serviceID))
		c.printInfo("现在可以使用 start/stop/deploy 命令操作此服务")
	}
}

// listServices 查看服务列表
func (c *CLI) listServices() {
	c.printInfo("查询服务列表...")

	cmd := exec.Command("curl", "-s", "http://localhost:8001/status")
	output, err := cmd.Output()
	if err != nil {
		c.printError("获取失败，请确保代理服务已启动")
		return
	}

	var status map[string]interface{}
	if err := json.Unmarshal(output, &status); err != nil {
		fmt.Println(string(output))
		return
	}

	services, ok := status["services"].([]interface{})
	if !ok {
		fmt.Println(string(output))
		return
	}

	fmt.Println("\n" + strings.Repeat("═", 90))
	fmt.Printf("\033[1;34m服务列表 (共 %d 个)\033[0m\n", len(services))
	fmt.Println(strings.Repeat("═", 90))

	fmt.Printf("  \033[1;33m%-12s  %-15s  %-20s  %-8s  %s\033[0m\n", "ID", "名称", "JAR文件", "环境", "目标地址")
	fmt.Printf("  %s  %s  %s  %s  %s\n",
		strings.Repeat("-", 12), strings.Repeat("-", 15), strings.Repeat("-", 20),
		strings.Repeat("-", 8), strings.Repeat("-", 25))

	for _, s := range services {
		svc, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		id := svc["id"].(string)
		name := svc["name"].(string)
		env := svc["active_env"].(string)
		target := svc["blue_target"].(string)
		if env == "green" {
			target = svc["green_target"].(string)
		}

		// 获取jar_file，如果没有则显示默认值
		jarFile := "ruoyi-*.jar"
		if jf, ok := svc["jar_file"].(string); ok && jf != "" {
			jarFile = jf
		}

		envColor := "\033[1;34m"
		if env == "green" {
			envColor = "\033[1;32m"
		}

		// 标记当前服务
		mark := ""
		if id == c.currentService {
			mark = " \033[1;32m← 当前\033[0m"
		}

		fmt.Printf("  %-12s  %-15s  %-20s  %s%-8s\033[0m  %s%s\n",
			id, name, jarFile, envColor, env, target, mark)
	}
	fmt.Println(strings.Repeat("─", 90))
}

// removeService 删除服务
func (c *CLI) removeService() {
	fmt.Println("\n\033[1;34m═══ 删除服务 ═══\033[0m\n")

	// 先显示服务列表
	c.listServices()

	serviceID, err := c.readLineWithPrompt("\033[1;33m输入要删除的服务ID: \033[0m")
	if err != nil || serviceID == "" {
		c.printError("服务ID不能为空")
		return
	}

	confirm, err := c.readLineWithPrompt(fmt.Sprintf("\033[1;31m确认删除服务[%s]? (yes/no): \033[0m", serviceID))
	if err != nil {
		c.printInfo("已取消")
		return
	}

	confirm = strings.ToLower(strings.TrimSpace(confirm))
	if confirm != "y" && confirm != "yes" {
		c.printInfo("已取消")
		return
	}

	cmd := exec.Command("curl", "-s", "-X", "DELETE",
		fmt.Sprintf("http://localhost:8001/service/remove?id=%s", serviceID))
	output, err := cmd.Output()
	if err != nil {
		c.printError("删除失败")
		return
	}

	fmt.Println(string(output))
	c.printSuccess(fmt.Sprintf("服务[%s]已删除", serviceID))

	// 如果删除的是当前服务，切换回default
	if serviceID == c.currentService {
		c.currentService = "default"
		c.printInfo("已自动切换回默认服务")
	}
}

// switchService 切换当前服务
func (c *CLI) switchService() {
	fmt.Println("\n\033[1;34m═══ 切换当前服务 ═══\033[0m\n")

	// 获取服务列表
	cmd := exec.Command("curl", "-s", "http://localhost:8001/status")
	output, err := cmd.Output()
	if err != nil {
		c.printError("获取服务列表失败，请确保代理服务已启动")
		return
	}

	var status map[string]interface{}
	if err := json.Unmarshal(output, &status); err != nil {
		c.printError("解析服务列表失败")
		return
	}

	services, ok := status["services"].([]interface{})
	if !ok {
		c.printError("服务列表格式错误")
		return
	}

	// 显示服务列表
	fmt.Println("\033[1;33m可用服务:\033[0m")
	fmt.Println(strings.Repeat("─", 60))

	serviceIDs := make([]string, 0)
	for i, s := range services {
		svc, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		id := svc["id"].(string)
		name := svc["name"].(string)

		serviceIDs = append(serviceIDs, id)

		// 标记当前服务
		mark := ""
		if id == c.currentService {
			mark = " \033[1;32m← 当前\033[0m"
		}

		fmt.Printf("  %d. %-12s  %s%s\n", i+1, id, name, mark)
	}
	fmt.Println(strings.Repeat("─", 60))

	// 输入选择
	choice, err := c.readLineWithPrompt("\033[1;33m选择服务 (输入编号或ID): \033[0m")
	if err != nil || choice == "" {
		c.printInfo("已取消")
		return
	}

	var selectedID string

	// 尝试解析为数字
	var index int
	if n, err := fmt.Sscanf(choice, "%d", &index); err == nil && n == 1 && index > 0 && index <= len(serviceIDs) {
		selectedID = serviceIDs[index-1]
	} else {
		// 直接作为ID
		selectedID = choice

		// 验证ID是否存在
		found := false
		for _, id := range serviceIDs {
			if id == selectedID {
				found = true
				break
			}
		}

		if !found {
			c.printError(fmt.Sprintf("服务[%s]不存在", selectedID))
			return
		}
	}

	c.currentService = selectedID
	c.printSuccess(fmt.Sprintf("已切换到服务[%s]", selectedID))
	c.printInfo("现在可以使用 start/stop/deploy 命令操作此服务")
}

// executeServiceCommand 执行当前服务的命令
func (c *CLI) executeServiceCommand(command string) {
	// 获取当前服务配置
	cmd := exec.Command("curl", "-s", "http://localhost:8001/status")
	output, err := cmd.Output()
	if err != nil {
		c.printError("获取服务配置失败，请确保代理服务已启动")
		c.printInfo("提示：请先执行 proxy-start 启动代理服务")
		return
	}

	var status map[string]interface{}
	if err := json.Unmarshal(output, &status); err != nil {
		c.printError("解析服务配置失败")
		return
	}

	services, ok := status["services"].([]interface{})
	if !ok {
		c.printError("服务列表格式错误")
		return
	}

	// 查找当前服务
	var currentSvc map[string]interface{}
	for _, s := range services {
		svc, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := svc["id"].(string); ok && id == c.currentService {
			currentSvc = svc
			break
		}
	}

	if currentSvc == nil {
		c.printError(fmt.Sprintf("未找到当前服务[%s]的配置", c.currentService))
		c.printInfo("提示：使用 service-list 查看所有服务")
		return
	}

	// 提取服务配置
	jarFile, _ := currentSvc["jar_file"].(string)
	appName, _ := currentSvc["app_name"].(string)
	blueTarget, _ := currentSvc["blue_target"].(string)
	greenTarget, _ := currentSvc["green_target"].(string)

	// 从目标地址提取端口
	bluePort := extractPort(blueTarget)
	greenPort := extractPort(greenTarget)

	// 默认值
	if jarFile == "" {
		jarFile = "ruoyi-*.jar"
	}
	if appName == "" {
		appName = "ruoyi"
	}
	if bluePort == "" {
		bluePort = "8080"
	}
	if greenPort == "" {
		greenPort = "8081"
	}

	// 显示当前操作的服务信息
	serviceName := currentSvc["name"].(string)
	c.printInfo(fmt.Sprintf("操作服务: %s (%s)", serviceName, c.currentService))

	// 查找脚本路径
	scriptPath := c.findScript("service.sh")
	if scriptPath == "" {
		c.printError("未找到脚本: service.sh")
		return
	}

	c.printInfo(fmt.Sprintf("执行: SERVICE_ID=%s APP_NAME=%s JAR=%s BLUE=%s GREEN=%s %s %s",
		c.currentService, appName, jarFile, bluePort, greenPort, scriptPath, command))
	fmt.Println(strings.Repeat("─", 60))

	// 准备环境变量
	env := os.Environ()
	env = append(env, fmt.Sprintf("SERVICE_ID=%s", c.currentService))
	env = append(env, fmt.Sprintf("APP_NAME=%s", appName))
	env = append(env, fmt.Sprintf("APP_JAR_PATTERN=%s", jarFile))
	env = append(env, fmt.Sprintf("BLUE_PORT=%s", bluePort))
	env = append(env, fmt.Sprintf("GREEN_PORT=%s", greenPort))

	// 执行命令
	execCmd := exec.Command("bash", scriptPath, command)
	execCmd.Env = env
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Stdin = os.Stdin

	if err := execCmd.Run(); err != nil {
		c.printError(fmt.Sprintf("执行失败: %v", err))
	}

	fmt.Println(strings.Repeat("─", 60))
}

// executeServiceLogCommand 执行日志相关命令（优先使用当前服务配置，失败则回退默认）
func (c *CLI) executeServiceLogCommand(command string, args ...string) {
	// 尝试获取当前服务配置
	cmd := exec.Command("curl", "-s", "http://localhost:8001/status")
	output, err := cmd.Output()
	if err != nil {
		c.printWarning("无法获取服务配置，回退为默认日志")
		c.executeScript(append([]string{"service.sh", command}, args...)...)
		return
	}

	var status map[string]interface{}
	if err := json.Unmarshal(output, &status); err != nil {
		c.printWarning("解析服务配置失败，回退为默认日志")
		c.executeScript(append([]string{"service.sh", command}, args...)...)
		return
	}

	services, ok := status["services"].([]interface{})
	if !ok {
		c.printWarning("服务配置格式异常，回退为默认日志")
		c.executeScript(append([]string{"service.sh", command}, args...)...)
		return
	}

	// 查找当前服务
	var currentSvc map[string]interface{}
	for _, s := range services {
		svc, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := svc["id"].(string); ok && id == c.currentService {
			currentSvc = svc
			break
		}
	}

	if currentSvc == nil {
		c.printError(fmt.Sprintf("未找到当前服务[%s]的配置", c.currentService))
		c.printInfo("提示：使用 service-list 查看所有服务")
		return
	}

	// 提取服务配置
	jarFile, _ := currentSvc["jar_file"].(string)
	appName, _ := currentSvc["app_name"].(string)
	blueTarget, _ := currentSvc["blue_target"].(string)
	greenTarget, _ := currentSvc["green_target"].(string)

	bluePort := extractPort(blueTarget)
	greenPort := extractPort(greenTarget)

	if jarFile == "" {
		jarFile = "ruoyi-*.jar"
	}
	if appName == "" {
		appName = "ruoyi"
	}
	if bluePort == "" {
		bluePort = "8080"
	}
	if greenPort == "" {
		greenPort = "8081"
	}

	serviceName := currentSvc["name"].(string)
	c.printInfo(fmt.Sprintf("查看服务日志: %s (%s)", serviceName, c.currentService))

	scriptPath := c.findScript("service.sh")
	if scriptPath == "" {
		c.printError("未找到脚本: service.sh")
		return
	}

	c.printInfo(fmt.Sprintf("执行: SERVICE_ID=%s APP_NAME=%s JAR=%s BLUE=%s GREEN=%s %s %s %s",
		c.currentService, appName, jarFile, bluePort, greenPort, scriptPath, command, strings.Join(args, " ")))
	fmt.Println(strings.Repeat("─", 60))

	env := os.Environ()
	env = append(env, fmt.Sprintf("SERVICE_ID=%s", c.currentService))
	env = append(env, fmt.Sprintf("APP_NAME=%s", appName))
	env = append(env, fmt.Sprintf("APP_JAR_PATTERN=%s", jarFile))
	env = append(env, fmt.Sprintf("BLUE_PORT=%s", bluePort))
	env = append(env, fmt.Sprintf("GREEN_PORT=%s", greenPort))

	cmdArgs := append([]string{scriptPath, command}, args...)
	execCmd := exec.Command("bash", cmdArgs...)
	execCmd.Env = env
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Stdin = os.Stdin

	if err := execCmd.Run(); err != nil {
		c.printError(fmt.Sprintf("执行失败: %v", err))
	}

	fmt.Println(strings.Repeat("─", 60))
}

// isLogsListHint 判断是否为日志文件列表提示
func (c *CLI) isLogsListHint(arg string) bool {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "?", "list", "-l", "--list":
		return true
	default:
		return false
	}
}

// isLogsSelectorHint 判断是否为选择器提示
func (c *CLI) isLogsSelectorHint(arg string) bool {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "/", "select", "--select":
		return true
	default:
		return false
	}
}

// isDateArg 判断是否为 YYYY-MM-DD 格式日期
func (c *CLI) isDateArg(arg string) bool {
	if strings.TrimSpace(arg) == "" {
		return false
	}
	_, err := time.Parse("2006-01-02", strings.TrimSpace(arg))
	return err == nil
}

// getAppHome 获取应用根目录（scripts 的父目录）
func (c *CLI) getAppHome() (string, error) {
	scriptPath := c.findScript("service.sh")
	if scriptPath == "" {
		return "", fmt.Errorf("未找到脚本: service.sh")
	}
	absPath, err := filepath.Abs(scriptPath)
	if err != nil {
		return "", err
	}
	scriptsDir := filepath.Dir(absPath)
	return filepath.Dir(scriptsDir), nil
}

// listLogFiles 获取 logs 目录下可用的日志文件（排序）
func (c *CLI) listLogFiles() ([]string, error) {
	appHome, err := c.getAppHome()
	if err != nil {
		return nil, err
	}
	logsDir := filepath.Join(appHome, "logs")
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".log") {
			files = append(files, name)
		}
	}

	sort.Strings(files)
	return files, nil
}

// logFileCompletionItems 自动补全日志文件（用于 readline）
func (c *CLI) logFileCompletionItems(_ string) []string {
	files, err := c.listLogFiles()
	if err != nil {
		return []string{}
	}
	return files
}

// printLogFileList 打印日志文件列表
func (c *CLI) printLogFileList() {
	files, err := c.listLogFiles()
	if err != nil {
		c.printError(fmt.Sprintf("读取日志文件失败: %v", err))
		return
	}
	if len(files) == 0 {
		c.printWarning("logs 目录下未找到日志文件")
		return
	}
	fmt.Println("\n\033[1;33m可用日志文件:\033[0m")
	fmt.Println(strings.Repeat("─", 60))
	for i, f := range files {
		fmt.Printf("  %d. %s\n", i+1, f)
	}
	fmt.Println(strings.Repeat("─", 60))
}

// selectLogFile 交互选择日志文件
func (c *CLI) selectLogFile() (string, bool) {
	files, err := c.listLogFiles()
	if err != nil {
		c.printError(fmt.Sprintf("读取日志文件失败: %v", err))
		return "", false
	}
	if len(files) == 0 {
		c.printWarning("logs 目录下未找到日志文件")
		return "", false
	}

	fmt.Println("\n\033[1;33m可用日志文件:\033[0m")
	fmt.Println(strings.Repeat("─", 60))
	for i, f := range files {
		fmt.Printf("  %d. %s\n", i+1, f)
	}
	fmt.Println(strings.Repeat("─", 60))

	choice, err := c.readLineWithPrompt("\033[1;33m选择日志文件 (输入编号或名称，回车取消): \033[0m")
	if err != nil || strings.TrimSpace(choice) == "" {
		c.printInfo("已取消")
		return "", false
	}

	choice = strings.TrimSpace(choice)
	var index int
	if n, err := fmt.Sscanf(choice, "%d", &index); err == nil && n == 1 && index > 0 && index <= len(files) {
		return files[index-1], true
	}

	for _, f := range files {
		if f == choice {
			return choice, true
		}
	}

	c.printError(fmt.Sprintf("日志文件[%s]不存在", choice))
	return "", false
}

// selectLogFileMenu 交互式选择器（支持方向键，若不可用则回退为编号选择）
func (c *CLI) selectLogFileMenu() (string, bool) {
	files, err := c.listLogFiles()
	if err != nil {
		c.printError(fmt.Sprintf("读取日志文件失败: %v", err))
		return "", false
	}
	if len(files) == 0 {
		c.printWarning("logs 目录下未找到日志文件")
		return "", false
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return c.selectLogFile()
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return c.selectLogFile()
	}
	defer term.Restore(fd, oldState)

	hideCursor := func() { fmt.Print("\033[?25l") }
	showCursor := func() { fmt.Print("\033[?25h") }
	hideCursor()
	defer showCursor()

	width, height, err := term.GetSize(fd)
	_ = width
	if err != nil || height < 8 {
		height = 12
	}
	maxVisible := height - 4
	if maxVisible < 5 {
		maxVisible = 5
	}

	selected := 0
	start := 0
	renderedLines := 0
	reader := bufio.NewReader(os.Stdin)

	render := func() {
		if renderedLines > 0 {
			fmt.Printf("\033[%dA", renderedLines)
			fmt.Print("\r\033[0J")
		}

		fmt.Print("\r\033[1;33m可用日志文件（↑/↓ 或 j/k 选择，Enter 确认，Esc 取消）:\033[0m\r\n")
		renderedLines = 1

		if selected < start {
			start = selected
		} else if selected >= start+maxVisible {
			start = selected - maxVisible + 1
		}

		end := start + maxVisible
		if end > len(files) {
			end = len(files)
		}

		for i := start; i < end; i++ {
			prefix := "  "
			line := files[i]
			if i == selected {
				prefix = "\033[1;32m> \033[0m"
				line = "\033[1;32m" + line + "\033[0m"
			}
			fmt.Printf("\r%s%s\r\n", prefix, line)
			renderedLines++
		}
	}

	readKey := func() rune {
		b, err := reader.ReadByte()
		if err != nil {
			return 0
		}
		if b == 0x1b {
			next, _ := reader.ReadByte()
			if next == '[' {
				third, _ := reader.ReadByte()
				switch third {
				case 'A':
					return 'U' // up
				case 'B':
					return 'D' // down
				}
			}
			return 0x1b
		}
		return rune(b)
	}

	render()
	for {
		k := readKey()
		switch k {
		case 'U', 'k', 'K':
			if selected > 0 {
				selected--
				render()
			}
		case 'D', 'j', 'J':
			if selected < len(files)-1 {
				selected++
				render()
			}
		case '\r', '\n':
			if renderedLines > 0 {
				fmt.Printf("\033[%dA", renderedLines)
				fmt.Print("\r\033[0J")
			}
			return files[selected], true
		case 0x1b, 3:
			if renderedLines > 0 {
				fmt.Printf("\033[%dA", renderedLines)
				fmt.Print("\r\033[0J")
			}
			c.printInfo("已取消")
			return "", false
		}
	}
}

// interactiveLogsSearch 交互式日志查询
func (c *CLI) interactiveLogsSearch() {
	logFile, ok := c.selectLogFileMenu()
	if !ok {
		return
	}

	keyword, _ := c.readLineWithPrompt("\033[1;33m请输入关键字 (可选，回车跳过): \033[0m")
	limit, _ := c.readLineWithPrompt("\033[1;33m请输入行数 (可选，默认600): \033[0m")

	args := []string{logFile}
	if strings.TrimSpace(keyword) != "" {
		args = append(args, strings.TrimSpace(keyword))
	}
	if strings.TrimSpace(limit) != "" {
		args = append(args, strings.TrimSpace(limit))
	}

	c.executeServiceLogCommand("logs-search", args...)
}

// interactiveLogsSearchWithBase 交互式日志查询（指定日志名/日志文件）
func (c *CLI) interactiveLogsSearchWithBase(base string) {
	base = strings.TrimSpace(base)
	if base == "" {
		c.printError("日志名/日志文件不能为空")
		return
	}

	keyword, _ := c.readLineWithPrompt("\033[1;33m请输入关键字 (可选，回车跳过): \033[0m")
	limit, _ := c.readLineWithPrompt("\033[1;33m请输入行数 (可选，默认600): \033[0m")

	args := []string{base}
	if strings.TrimSpace(keyword) != "" {
		args = append(args, strings.TrimSpace(keyword))
	}
	if strings.TrimSpace(limit) != "" {
		args = append(args, strings.TrimSpace(limit))
	}

	c.executeServiceLogCommand("logs-search", args...)
}

// interactiveLogsExport 交互式日志导出
func (c *CLI) interactiveLogsExport() {
	logFile, ok := c.selectLogFileMenu()
	if !ok {
		return
	}

	output, _ := c.readLineWithPrompt("\033[1;33m输出文件名 (可选，回车使用默认): \033[0m")

	args := []string{logFile}
	if strings.TrimSpace(output) != "" {
		args = append(args, strings.TrimSpace(output))
	}

	c.executeServiceLogCommand("logs-export", args...)
}

// interactiveLogsExportWithBase 交互式日志导出（指定日志名/日志文件）
func (c *CLI) interactiveLogsExportWithBase(base string) {
	base = strings.TrimSpace(base)
	if base == "" {
		c.printError("日志名/日志文件不能为空")
		return
	}

	output, _ := c.readLineWithPrompt("\033[1;33m输出文件名 (可选，回车使用默认): \033[0m")

	args := []string{base}
	if strings.TrimSpace(output) != "" {
		args = append(args, strings.TrimSpace(output))
	}

	c.executeServiceLogCommand("logs-export", args...)
}

// extractPort 从URL中提取端口号
func extractPort(target string) string {
	// 格式：http://127.0.0.1:8080
	parts := strings.Split(target, ":")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}
