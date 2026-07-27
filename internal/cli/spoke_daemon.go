package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const spokeAgentServiceName = "ruoyi-proxy-spoke-agent"
const spokeAgentUnitPath = "/etc/systemd/system/ruoyi-proxy-spoke-agent.service"

func (c *CLI) installSpokeAgentService() {
	if runtime.GOOS != "linux" {
		c.printError("spoke-agent systemd 安装仅支持 Linux")
		return
	}
	if c.isProxyRunning() {
		c.printInfo("代理服务已运行并自动接管远程控制，无需安装 spoke-agent")
		return
	}
	if !c.confirmDangerAction("安装 Spoke 常驻服务", []string{
		"创建 " + spokeAgentUnitPath,
		"执行 systemctl enable --now " + spokeAgentServiceName,
	}) {
		return
	}
	binaryPath, err := os.Executable()
	if err != nil {
		c.printError(fmt.Sprintf("获取当前程序路径失败: %v", err))
		return
	}
	if resolved, resolveErr := filepath.EvalSymlinks(binaryPath); resolveErr == nil {
		binaryPath = resolved
	}
	binaryPath, err = filepath.Abs(binaryPath)
	if err != nil {
		c.printError(fmt.Sprintf("解析程序路径失败: %v", err))
		return
	}
	workDir, err := os.Getwd()
	if err != nil {
		c.printError(fmt.Sprintf("获取工作目录失败: %v", err))
		return
	}
	if strings.ContainsAny(binaryPath+workDir, "\r\n") {
		c.printError("程序路径或工作目录包含非法换行")
		return
	}
	unit := fmt.Sprintf(`[Unit]
Description=ruoyi-proxy Spoke Remote Control Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s spoke-agent
Restart=always
RestartSec=5
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
`, systemdQuote(workDir), systemdQuote(binaryPath))

	if old, readErr := os.ReadFile(spokeAgentUnitPath); readErr == nil {
		backup := spokeAgentUnitPath + ".bak-" + time.Now().Format("20060102-150405")
		if err := os.WriteFile(backup, old, 0644); err != nil {
			c.printError(fmt.Sprintf("备份旧服务文件失败: %v", err))
			return
		}
		c.printInfo("旧服务文件已备份: " + backup)
	}
	if err := os.WriteFile(spokeAgentUnitPath, []byte(unit), 0644); err != nil {
		c.printError(fmt.Sprintf("写入 systemd 服务失败（请确认使用 root 运行）: %v", err))
		return
	}
	if output, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		c.printError(fmt.Sprintf("systemctl daemon-reload 失败: %v\n%s", err, strings.TrimSpace(string(output))))
		return
	}
	if output, err := exec.Command("systemctl", "enable", "--now", spokeAgentServiceName).CombinedOutput(); err != nil {
		c.printError(fmt.Sprintf("启动 Spoke 常驻服务失败: %v\n%s", err, strings.TrimSpace(string(output))))
		return
	}
	c.printSuccess("Spoke 常驻服务已安装并启动")
	c.printInfo("使用 /spoke-agent-status 查看状态")
}

func (c *CLI) showSpokeAgentStatus() {
	if runtime.GOOS != "linux" {
		c.printError("spoke-agent systemd 状态仅支持 Linux")
		return
	}
	if c.isProxyRunning() {
		c.printSuccess("代理服务运行中，Spoke 远程控制由代理进程接管")
		return
	}
	cmd := exec.Command("systemctl", "status", spokeAgentServiceName, "--no-pager", "--lines=30")
	output, err := cmd.CombinedOutput()
	fmt.Print(sanitizeTerminalText(string(output)))
	if err != nil {
		c.printWarning(fmt.Sprintf("服务当前未正常运行: %v", err))
	}
}

func systemdQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
