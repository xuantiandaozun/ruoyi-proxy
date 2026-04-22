package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ruoyi-proxy/internal/config"
)

// ——— 工具定义 ———

// AllTools 所有可用工具列表
var AllTools = []ToolDef{
	// ── 只读工具 ──────────────────────────────────────────────
	{
		Name:        "get_status",
		Description: "查询所有服务的运行状态和代理状态",
		ReadOnly:    true,
		Parameters:  emptyParams(),
	},
	{
		Name:        "get_logs",
		Description: "读取服务日志，支持关键字过滤。log_name 为日志文件名（如 ruoyi.log），留空时读取主日志；keyword 为过滤关键字；lines 为读取行数",
		ReadOnly:    true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"log_name": map[string]interface{}{
					"type":        "string",
					"description": "日志文件名，如 ruoyi.log，留空则使用主日志",
				},
				"keyword": map[string]interface{}{
					"type":        "string",
					"description": "过滤关键字，留空则不过滤",
				},
				"lines": map[string]interface{}{
					"type":        "integer",
					"description": "读取行数，默认 200，最大 1000",
					"default":     200,
				},
			},
			"required": []string{},
		},
	},
	{
		Name:        "get_config",
		Description: "查看当前代理配置（服务列表、蓝绿端口、活跃环境）和 JVM 配置",
		ReadOnly:    true,
		Parameters:  emptyParams(),
	},
	{
		Name:        "get_system_info",
		Description: "查看系统资源：CPU、内存、磁盘、Java 版本、操作系统发行版",
		ReadOnly:    true,
		Parameters:  emptyParams(),
	},
	{
		Name:        "read_file",
		Description: "读取服务器文件内容。path 为文件的绝对路径或相对路径；max_lines 限制返回行数（默认 200）",
		ReadOnly:    true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "文件路径，如 /etc/nginx/nginx.conf 或 configs/app_config.json",
				},
				"max_lines": map[string]interface{}{
					"type":        "integer",
					"description": "最大返回行数，默认 200，最大 2000",
					"default":     200,
				},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "list_directory",
		Description: "列出目录内容，显示文件名、大小、权限、修改时间",
		ReadOnly:    true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "目录路径，留空则列出当前工作目录",
				},
				"show_hidden": map[string]interface{}{
					"type":        "boolean",
					"description": "是否显示隐藏文件（以.开头），默认 false",
					"default":     false,
				},
			},
			"required": []string{},
		},
	},
	{
		Name:        "systemd_info",
		Description: "查询 systemd 服务状态、是否启用、查看 journal 日志。action: status/is-active/is-enabled/journal",
		ReadOnly:    true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"status", "is-active", "is-enabled", "journal"},
					"description": "查询操作",
				},
				"service": map[string]interface{}{
					"type":        "string",
					"description": "服务名称，如 nginx、mysql、redis",
				},
				"lines": map[string]interface{}{
					"type":        "integer",
					"description": "journal 日志行数，默认 50",
					"default":     50,
				},
			},
			"required": []string{"action", "service"},
		},
	},

	// ── 写操作工具（需用户确认）──────────────────────────────
	{
		Name:        "service_control",
		Description: "控制若依应用服务：start（启动）、stop（停止）、restart（重启）、deploy（蓝绿部署）。需要用户确认",
		ReadOnly:    false,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"start", "stop", "restart", "deploy"},
					"description": "要执行的操作",
				},
			},
			"required": []string{"action"},
		},
	},
	{
		Name:        "switch_env",
		Description: "切换蓝绿环境：将当前服务的流量切换到 blue 或 green 环境。需要用户确认",
		ReadOnly:    false,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"env": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"blue", "green"},
					"description": "目标环境",
				},
			},
			"required": []string{"env"},
		},
	},
	{
		Name:        "update_jvm",
		Description: "切换 JVM 预设档位：1=1核2G, 2=2核4G, 3=4核8G。需要用户确认，重启后生效",
		ReadOnly:    false,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"preset": map[string]interface{}{
					"type":        "integer",
					"enum":        []int{1, 2, 3},
					"description": "预设档位编号",
				},
			},
			"required": []string{"preset"},
		},
	},
	{
		Name:        "write_file",
		Description: "写入或修改服务器文件内容。重要配置文件（*.conf/*.json/*.sh 等）写入前会自动备份到 ~/.ruoyi-backup/。需要用户确认",
		ReadOnly:    false,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "文件路径（绝对或相对），不存在时自动创建",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "要写入的完整文件内容",
				},
				"append": map[string]interface{}{
					"type":        "boolean",
					"description": "true=追加到文件末尾，false=覆盖整个文件（默认）",
					"default":     false,
				},
			},
			"required": []string{"path", "content"},
		},
	},
	{
		Name:        "delete_file",
		Description: "删除服务器文件或空目录。支持同时删除多个文件（传 paths 数组）。重要文件删除前自动备份到 ~/.ruoyi-backup/。需要用户确认",
		ReadOnly:    false,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "要删除的单个文件路径（与 paths 二选一）",
				},
				"paths": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "要批量删除的文件路径列表（推荐用于多文件场景）",
				},
			},
			"required": []string{},
		},
	},
	{
		Name:        "install_package",
		Description: "使用系统包管理器安装软件包（自动识别 apt/yum/dnf/pacman/apk）。也可用于安装 Nginx、MySQL、Redis 等服务。需要用户确认",
		ReadOnly:    false,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"packages": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "要安装的包名列表，如 [\"nginx\", \"redis\"]",
				},
				"update_first": map[string]interface{}{
					"type":        "boolean",
					"description": "安装前是否先更新包索引（apt update / yum check-update），默认 true",
					"default":     true,
				},
			},
			"required": []string{"packages"},
		},
	},
	{
		Name:        "manage_systemd",
		Description: "管理 systemd 服务：start/stop/restart/reload/enable/disable。适用于 nginx、mysql、redis 等系统服务。需要用户确认",
		ReadOnly:    false,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"start", "stop", "restart", "reload", "enable", "disable"},
					"description": "要执行的操作",
				},
				"service": map[string]interface{}{
					"type":        "string",
					"description": "服务名称，如 nginx、mysql、redis",
				},
			},
			"required": []string{"action", "service"},
		},
	},
	{
		Name:        "run_shell",
		Description: "在服务器上执行任意 shell 命令。用于复杂运维操作（如解压、复制、权限变更等）。需要用户确认",
		ReadOnly:    false,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "要执行的 shell 命令，如 tar -xzf app.tar.gz -C /opt/",
				},
				"workdir": map[string]interface{}{
					"type":        "string",
					"description": "命令执行目录，留空则使用当前目录",
				},
				"timeout": map[string]interface{}{
					"type":        "integer",
					"description": "超时秒数，默认 60，最大 300",
					"default":     60,
				},
			},
			"required": []string{"command"},
		},
	},
}

// ——— 工具执行器 ———

// ToolExecutor 执行工具调用，结果以字符串返回给 AI
type ToolExecutor struct {
	execCtx ExecContext
}

// NewToolExecutor 创建工具执行器
func NewToolExecutor(ctx ExecContext) *ToolExecutor {
	return &ToolExecutor{execCtx: ctx}
}

// Execute 执行工具，返回结果字符串
func (e *ToolExecutor) Execute(name, argsJSON string) (string, error) {
	var args map[string]interface{}
	if argsJSON != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}
	if args == nil {
		args = map[string]interface{}{}
	}

	switch name {
	case "get_status":
		return e.getStatus()
	case "get_logs":
		logName, _ := args["log_name"].(string)
		keyword, _ := args["keyword"].(string)
		lines := 200
		if v, ok := args["lines"].(float64); ok && v > 0 {
			lines = int(v)
			if lines > 1000 {
				lines = 1000
			}
		}
		return e.getLogs(logName, keyword, lines)
	case "get_config":
		return e.getConfig()
	case "get_system_info":
		return e.getSystemInfo()
	case "read_file":
		path, _ := args["path"].(string)
		maxLines := 200
		if v, ok := args["max_lines"].(float64); ok && v > 0 {
			maxLines = int(v)
			if maxLines > 2000 {
				maxLines = 2000
			}
		}
		return e.readFile(path, maxLines)
	case "list_directory":
		path, _ := args["path"].(string)
		showHidden, _ := args["show_hidden"].(bool)
		return e.listDirectory(path, showHidden)
	case "systemd_info":
		action, _ := args["action"].(string)
		svc, _ := args["service"].(string)
		lines := 50
		if v, ok := args["lines"].(float64); ok && v > 0 {
			lines = int(v)
		}
		return e.systemdInfo(action, svc, lines)
	case "service_control":
		action, _ := args["action"].(string)
		return e.serviceControl(action)
	case "switch_env":
		env, _ := args["env"].(string)
		return e.switchEnv(env)
	case "update_jvm":
		preset := 0
		if v, ok := args["preset"].(float64); ok {
			preset = int(v)
		}
		return e.updateJVM(preset)
	case "write_file":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		appendMode, _ := args["append"].(bool)
		return e.writeFile(path, content, appendMode)
	case "delete_file":
		// 支持单路径（path）或批量路径（paths 数组）
		var paths []string
		if v, ok := args["paths"].([]interface{}); ok {
			for _, p := range v {
				if s, ok := p.(string); ok && s != "" {
					paths = append(paths, s)
				}
			}
		}
		if singlePath, ok := args["path"].(string); ok && singlePath != "" {
			paths = append(paths, singlePath)
		}
		return e.deleteFiles(paths)
	case "install_package":
		var packages []string
		if v, ok := args["packages"].([]interface{}); ok {
			for _, p := range v {
				if s, ok := p.(string); ok && s != "" {
					packages = append(packages, s)
				}
			}
		}
		updateFirst := true
		if v, ok := args["update_first"].(bool); ok {
			updateFirst = v
		}
		return e.installPackage(packages, updateFirst)
	case "manage_systemd":
		action, _ := args["action"].(string)
		svc, _ := args["service"].(string)
		return e.manageSystemd(action, svc)
	case "run_shell":
		command, _ := args["command"].(string)
		workdir, _ := args["workdir"].(string)
		timeout := 60
		if v, ok := args["timeout"].(float64); ok && v > 0 {
			timeout = int(v)
			if timeout > 300 {
				timeout = 300
			}
		}
		return e.runShell(command, workdir, time.Duration(timeout)*time.Second)
	default:
		return "", fmt.Errorf("未知工具: %s", name)
	}
}

// ——— 只读工具实现 ───────────────────────────────────────────

func (e *ToolExecutor) getStatus() (string, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return "", fmt.Errorf("读取配置失败: %v", err)
	}

	var sb strings.Builder
	proxyRunning := isPortOpen("127.0.0.1" + config.ProxyPort)
	if proxyRunning {
		sb.WriteString("代理服务: ✓ 运行中\n")
	} else {
		sb.WriteString("代理服务: ✗ 未运行\n")
	}
	sb.WriteString(fmt.Sprintf("代理端口: %s\n\n", config.ProxyPort))

	sb.WriteString(fmt.Sprintf("服务数量: %d\n", len(cfg.Services)))
	sb.WriteString(strings.Repeat("-", 60) + "\n")

	client := &http.Client{Timeout: 2 * time.Second}
	for id, svc := range cfg.Services {
		name := svc.Name
		if name == "" {
			name = id
		}
		target := svc.BlueTarget
		if svc.ActiveEnv == "green" {
			target = svc.GreenTarget
		}
		mark := ""
		if id == e.execCtx.CurrentService {
			mark = " [当前]"
		}
		sb.WriteString(fmt.Sprintf("\n服务: %s (%s)%s\n", name, id, mark))
		sb.WriteString(fmt.Sprintf("  活跃环境: %s\n", svc.ActiveEnv))
		sb.WriteString(fmt.Sprintf("  蓝色端口: %s → %s\n", extractPort(svc.BlueTarget), svc.BlueTarget))
		sb.WriteString(fmt.Sprintf("  绿色端口: %s → %s\n", extractPort(svc.GreenTarget), svc.GreenTarget))

		resp, err := client.Get(target + "/actuator/health")
		if err != nil {
			sb.WriteString("  健康状态: ✗ 不可达\n")
		} else {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				sb.WriteString("  健康状态: ✓ 健康\n")
			} else {
				sb.WriteString(fmt.Sprintf("  健康状态: ⚠ HTTP %d\n", resp.StatusCode))
			}
		}
	}
	return sb.String(), nil
}

func (e *ToolExecutor) getLogs(logName, keyword string, lines int) (string, error) {
	if e.execCtx.ScriptPath == "" {
		return "", fmt.Errorf("未找到 service.sh 脚本")
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return "", fmt.Errorf("读取配置失败: %v", err)
	}
	svc := cfg.GetService(e.execCtx.CurrentService)
	if svc == nil {
		return "", fmt.Errorf("未找到服务配置: %s", e.execCtx.CurrentService)
	}

	env := buildScriptEnv(e.execCtx, svc)
	var cmd *exec.Cmd
	linesStr := fmt.Sprintf("%d", lines)
	if logName != "" && keyword != "" {
		cmd = exec.Command("bash", e.execCtx.ScriptPath, "logs-search", logName, keyword, linesStr)
	} else if logName != "" {
		cmd = exec.Command("bash", e.execCtx.ScriptPath, "logs-search", logName, linesStr)
	} else if keyword != "" {
		cmd = exec.Command("bash", e.execCtx.ScriptPath, "logs-search", keyword, linesStr)
	} else {
		cmd = exec.Command("bash", e.execCtx.ScriptPath, "logs", linesStr)
	}
	cmd.Env = env
	return runWithTimeout(cmd, 15*time.Second)
}

func (e *ToolExecutor) getConfig() (string, error) {
	var sb strings.Builder

	cfg, err := config.LoadConfig()
	if err != nil {
		sb.WriteString(fmt.Sprintf("读取代理配置失败: %v\n", err))
	} else {
		sb.WriteString("=== 代理配置 ===\n")
		for id, svc := range cfg.Services {
			sb.WriteString(fmt.Sprintf("服务[%s] %s\n", id, svc.Name))
			sb.WriteString(fmt.Sprintf("  蓝色: %s  绿色: %s  当前: %s\n",
				svc.BlueTarget, svc.GreenTarget, svc.ActiveEnv))
			sb.WriteString(fmt.Sprintf("  JAR: %s  AppName: %s\n", svc.JarFile, svc.AppName))
		}
	}

	data, err := os.ReadFile(appConfigFile)
	if err == nil {
		var root map[string]json.RawMessage
		if json.Unmarshal(data, &root) == nil {
			if raw, ok := root["jvm"]; ok {
				var jvm map[string]interface{}
				if json.Unmarshal(raw, &jvm) == nil {
					sb.WriteString("\n=== JVM 配置 ===\n")
					preset := int(jvm["preset"].(float64))
					sb.WriteString(fmt.Sprintf("当前档位: %d\n", preset))
					if co, ok := jvm["custom_opts"].(string); ok && co != "" {
						sb.WriteString(fmt.Sprintf("自定义参数: %s\n", co))
					}
					if presets, ok := jvm["presets"].(map[string]interface{}); ok {
						key := fmt.Sprintf("%d", preset)
						if p, ok := presets[key].(map[string]interface{}); ok {
							sb.WriteString(fmt.Sprintf("档位详情: %s — Xms:%s Xmx:%s\n",
								p["name"], p["xms"], p["xmx"]))
						}
					}
				}
			}
		}
	}

	return sb.String(), nil
}

func (e *ToolExecutor) getSystemInfo() (string, error) {
	var sb strings.Builder

	// 操作系统信息
	sb.WriteString("--- 系统信息 ---\n")
	distro, pkgMgr := detectDistro()
	sb.WriteString(fmt.Sprintf("发行版: %s  包管理器: %s\n", distro, pkgMgr))

	// uname
	if out, err := exec.Command("uname", "-a").Output(); err == nil {
		sb.WriteString(strings.TrimSpace(string(out)) + "\n")
	}

	sb.WriteString("\n--- Java 版本 ---\n")
	if out, err := exec.Command("java", "-version").CombinedOutput(); err == nil {
		sb.WriteString(strings.TrimSpace(string(out)) + "\n")
	} else {
		sb.WriteString("未安装\n")
	}

	sb.WriteString("\n--- 内存使用 ---\n")
	if out, err := exec.Command("free", "-h").Output(); err == nil {
		sb.WriteString(strings.TrimSpace(string(out)) + "\n")
	} else {
		// macOS fallback
		if out, err := exec.Command("vm_stat").Output(); err == nil {
			sb.WriteString(strings.TrimSpace(string(out)) + "\n")
		}
	}

	sb.WriteString("\n--- 磁盘使用 ---\n")
	if out, err := exec.Command("df", "-h", ".").Output(); err == nil {
		sb.WriteString(strings.TrimSpace(string(out)) + "\n")
	}

	sb.WriteString("\n--- CPU 负载 ---\n")
	if out, err := exec.Command("uptime").Output(); err == nil {
		sb.WriteString(strings.TrimSpace(string(out)) + "\n")
	}

	return sb.String(), nil
}

func (e *ToolExecutor) readFile(path string, maxLines int) (string, error) {
	if path == "" {
		return "", fmt.Errorf("请提供文件路径")
	}

	// 展开 ~
	path = expandHome(path)

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("文件不存在或无法访问: %v", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s 是目录，请使用 list_directory 工具", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("读取失败: %v", err)
	}
	defer f.Close()

	// 读取全部内容，截取行数
	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("读取失败: %v", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	total := len(lines)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("文件: %s  大小: %d 字节  共 %d 行\n", path, info.Size(), total))
	sb.WriteString(strings.Repeat("-", 60) + "\n")

	if total > maxLines {
		// 显示前 maxLines 行
		sb.WriteString(strings.Join(lines[:maxLines], "\n"))
		sb.WriteString(fmt.Sprintf("\n\n[... 仅显示前 %d 行，共 %d 行，使用 max_lines 参数可调整 ...]", maxLines, total))
	} else {
		sb.WriteString(content)
	}

	return sb.String(), nil
}

func (e *ToolExecutor) listDirectory(path string, showHidden bool) (string, error) {
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			path = "."
		}
	}
	path = expandHome(path)

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("读取目录失败: %v", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("目录: %s\n", path))
	sb.WriteString(strings.Repeat("-", 70) + "\n")
	sb.WriteString(fmt.Sprintf("%-10s  %-8s  %-20s  %s\n", "权限", "大小", "修改时间", "名称"))
	sb.WriteString(strings.Repeat("-", 70) + "\n")

	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		mode := info.Mode().String()
		size := formatSize(info.Size())
		modTime := info.ModTime().Format("2006-01-02 15:04:05")

		dirMark := ""
		if entry.IsDir() {
			dirMark = "/"
			size = "-"
		}

		sb.WriteString(fmt.Sprintf("%-10s  %-8s  %-20s  %s%s\n",
			mode, size, modTime, name, dirMark))
		count++
	}

	sb.WriteString(strings.Repeat("-", 70) + "\n")
	sb.WriteString(fmt.Sprintf("共 %d 项\n", count))
	return sb.String(), nil
}

func (e *ToolExecutor) systemdInfo(action, svcName string, lines int) (string, error) {
	if svcName == "" {
		return "", fmt.Errorf("请指定服务名称")
	}

	// 检测 init 系统
	hasSystemctl := commandExists("systemctl")
	hasService := commandExists("service")

	var cmd *exec.Cmd
	switch action {
	case "status":
		if hasSystemctl {
			cmd = exec.Command("systemctl", "status", svcName, "--no-pager", "-l")
		} else if hasService {
			cmd = exec.Command("service", svcName, "status")
		} else {
			return "", fmt.Errorf("未找到 systemctl 或 service 命令")
		}
	case "is-active":
		if hasSystemctl {
			cmd = exec.Command("systemctl", "is-active", svcName)
		} else {
			cmd = exec.Command("service", svcName, "status")
		}
	case "is-enabled":
		if hasSystemctl {
			cmd = exec.Command("systemctl", "is-enabled", svcName)
		} else {
			return "无法查询 is-enabled（非 systemd 系统）", nil
		}
	case "journal":
		if hasSystemctl {
			cmd = exec.Command("journalctl", "-u", svcName, "-n", fmt.Sprintf("%d", lines), "--no-pager")
		} else {
			// 回退到 /var/log/syslog 或 messages
			logFile := "/var/log/syslog"
			if _, err := os.Stat(logFile); os.IsNotExist(err) {
				logFile = "/var/log/messages"
			}
			cmd = exec.Command("grep", svcName, logFile)
		}
	default:
		return "", fmt.Errorf("不支持的操作: %s", action)
	}

	out, _ := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), nil
}

// ——— 写操作工具实现 ───────────────────────────────────────────

func (e *ToolExecutor) serviceControl(action string) (string, error) {
	if e.execCtx.ScriptPath == "" {
		return "", fmt.Errorf("未找到 service.sh 脚本")
	}
	if action != "start" && action != "stop" && action != "restart" && action != "deploy" {
		return "", fmt.Errorf("无效操作: %s", action)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return "", fmt.Errorf("读取配置失败: %v", err)
	}
	svc := cfg.GetService(e.execCtx.CurrentService)
	if svc == nil {
		return "", fmt.Errorf("未找到服务配置: %s", e.execCtx.CurrentService)
	}

	env := buildScriptEnv(e.execCtx, svc)
	cmd := exec.Command("bash", e.execCtx.ScriptPath, action)
	cmd.Env = env
	return runWithTimeout(cmd, 120*time.Second)
}

func (e *ToolExecutor) switchEnv(env string) (string, error) {
	if env != "blue" && env != "green" {
		return "", fmt.Errorf("无效环境: %s", env)
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return "", fmt.Errorf("读取配置失败: %v", err)
	}
	svc := cfg.GetService(e.execCtx.CurrentService)
	if svc == nil {
		return "", fmt.Errorf("未找到服务配置: %s", e.execCtx.CurrentService)
	}
	svc.ActiveEnv = env
	if err := config.SaveConfig(cfg); err != nil {
		return "", fmt.Errorf("保存配置失败: %v", err)
	}
	return fmt.Sprintf("服务[%s]已切换到 %s 环境（配置已保存，代理重启后生效）",
		e.execCtx.CurrentService, env), nil
}

func (e *ToolExecutor) updateJVM(preset int) (string, error) {
	if preset < 1 || preset > 3 {
		return "", fmt.Errorf("无效档位: %d（必须是 1、2 或 3）", preset)
	}

	data, err := os.ReadFile(appConfigFile)
	if err != nil {
		return "", fmt.Errorf("读取配置文件失败: %v", err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return "", fmt.Errorf("解析配置文件失败: %v", err)
	}

	jvm, ok := root["jvm"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("JVM 配置不存在，请先在 CLI 中运行 jvm-config 初始化")
	}
	jvm["preset"] = float64(preset)
	root["jvm"] = jvm

	out, _ := json.MarshalIndent(root, "", "  ")
	if err := os.WriteFile(appConfigFile, out, 0644); err != nil {
		return "", fmt.Errorf("保存失败: %v", err)
	}
	return fmt.Sprintf("JVM 预设已切换到档位 %d，重启 Java 应用后生效", preset), nil
}

func (e *ToolExecutor) writeFile(path, content string, appendMode bool) (string, error) {
	if path == "" {
		return "", fmt.Errorf("请提供文件路径")
	}
	path = expandHome(path)

	// 重要文件先备份
	backed := false
	if isImportantFile(path) {
		if _, err := os.Stat(path); err == nil {
			if bpath, err := backupFile(path); err == nil {
				backed = true
				_ = bpath
			}
		}
	}

	// 确保父目录存在
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}

	var flag int
	if appendMode {
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	} else {
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	f, err := os.OpenFile(path, flag, 0644)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return "", fmt.Errorf("写入失败: %v", err)
	}

	mode := "覆盖写入"
	if appendMode {
		mode = "追加写入"
	}
	result := fmt.Sprintf("✓ %s成功: %s（%d 字节）", mode, path, len(content))
	if backed {
		result += "\n已自动备份原文件到 ~/.ruoyi-backup/"
	}
	return result, nil
}

func (e *ToolExecutor) deleteFiles(paths []string) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("请提供要删除的文件路径（path 或 paths）")
	}

	var sb strings.Builder
	backedUp := []string{}
	failed := []string{}
	deleted := []string{}

	for _, path := range paths {
		path = expandHome(strings.TrimSpace(path))
		if path == "" {
			continue
		}

		info, err := os.Stat(path)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s（不存在）", path))
			continue
		}

		// 重要文件先备份
		if isImportantFile(path) && !info.IsDir() {
			if _, err := backupFile(path); err == nil {
				backedUp = append(backedUp, path)
			}
		}

		// 执行删除
		if info.IsDir() {
			err = os.Remove(path) // 只删空目录
		} else {
			err = os.Remove(path)
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s（%v）", path, err))
		} else {
			deleted = append(deleted, path)
		}
	}

	// 汇总结果
	if len(deleted) > 0 {
		sb.WriteString(fmt.Sprintf("✓ 已删除 %d 个文件:\n", len(deleted)))
		for _, p := range deleted {
			sb.WriteString(fmt.Sprintf("  - %s\n", p))
		}
	}
	if len(backedUp) > 0 {
		sb.WriteString(fmt.Sprintf("\n已自动备份 %d 个重要文件到 ~/.ruoyi-backup/\n", len(backedUp)))
	}
	if len(failed) > 0 {
		sb.WriteString(fmt.Sprintf("\n✗ %d 个文件删除失败:\n", len(failed)))
		for _, f := range failed {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}

	return strings.TrimSpace(sb.String()), nil
}

func (e *ToolExecutor) installPackage(packages []string, updateFirst bool) (string, error) {
	if len(packages) == 0 {
		return "", fmt.Errorf("请指定要安装的包名")
	}

	pkgMgrCmd, installArgs := getPackageManagerCmd()
	if pkgMgrCmd == "" {
		return "", fmt.Errorf("未找到支持的包管理器（apt/yum/dnf/pacman/apk）")
	}

	var sb strings.Builder
	distro, pkgMgr := detectDistro()
	sb.WriteString(fmt.Sprintf("系统: %s  包管理器: %s\n\n", distro, pkgMgr))

	// 更新包索引
	if updateFirst {
		sb.WriteString("正在更新包索引...\n")
		var updateCmd *exec.Cmd
		switch pkgMgrCmd {
		case "apt-get":
			updateCmd = exec.Command("apt-get", "update", "-qq")
		case "yum":
			updateCmd = exec.Command("yum", "check-update")
		case "dnf":
			updateCmd = exec.Command("dnf", "check-update")
		case "pacman":
			updateCmd = exec.Command("pacman", "-Sy")
		case "apk":
			updateCmd = exec.Command("apk", "update")
		}
		if updateCmd != nil {
			out, _ := updateCmd.CombinedOutput()
			sb.WriteString(truncateOutput(string(out), 500))
			sb.WriteString("\n")
		}
	}

	// 安装包
	sb.WriteString(fmt.Sprintf("正在安装: %s\n", strings.Join(packages, " ")))
	args := append(installArgs, packages...)
	cmd := exec.Command(pkgMgrCmd, args...)
	out, err := runWithTimeout(cmd, 180*time.Second)
	sb.WriteString(out)
	if err != nil {
		sb.WriteString(fmt.Sprintf("\n✗ 安装失败: %v", err))
		return sb.String(), nil
	}
	sb.WriteString(fmt.Sprintf("\n✓ 安装完成: %s", strings.Join(packages, " ")))
	return sb.String(), nil
}

func (e *ToolExecutor) manageSystemd(action, svcName string) (string, error) {
	if svcName == "" {
		return "", fmt.Errorf("请指定服务名称")
	}
	validActions := map[string]bool{
		"start": true, "stop": true, "restart": true,
		"reload": true, "enable": true, "disable": true,
	}
	if !validActions[action] {
		return "", fmt.Errorf("不支持的操作: %s", action)
	}

	hasSystemctl := commandExists("systemctl")
	hasService := commandExists("service")

	var cmd *exec.Cmd
	if hasSystemctl {
		cmd = exec.Command("systemctl", action, svcName)
	} else if hasService {
		// SysV init 兼容
		switch action {
		case "enable", "disable":
			// 尝试 chkconfig（CentOS 6）或 update-rc.d（Debian 旧版）
			if commandExists("chkconfig") {
				onoff := "on"
				if action == "disable" {
					onoff = "off"
				}
				cmd = exec.Command("chkconfig", svcName, onoff)
			} else if commandExists("update-rc.d") {
				onoff := "enable"
				if action == "disable" {
					onoff = "disable"
				}
				cmd = exec.Command("update-rc.d", svcName, onoff)
			} else {
				return "", fmt.Errorf("无法 %s 服务（非 systemd 系统，未找到 chkconfig/update-rc.d）", action)
			}
		default:
			cmd = exec.Command("service", svcName, action)
		}
	} else {
		return "", fmt.Errorf("未找到 systemctl 或 service 命令")
	}

	out, err := runWithTimeout(cmd, 30*time.Second)
	if err != nil {
		return fmt.Sprintf("执行结果:\n%s\n✗ %v", out, err), nil
	}
	return fmt.Sprintf("✓ %s %s 执行成功\n%s", action, svcName, out), nil
}

func (e *ToolExecutor) runShell(command, workdir string, timeout time.Duration) (string, error) {
	if command == "" {
		return "", fmt.Errorf("请提供命令")
	}

	cmd := exec.Command("bash", "-c", command)

	if workdir != "" {
		workdir = expandHome(workdir)
		if _, err := os.Stat(workdir); err == nil {
			cmd.Dir = workdir
		}
	}

	out, err := runWithTimeout(cmd, timeout)
	if err != nil {
		return fmt.Sprintf("命令输出:\n%s\n✗ %v", out, err), nil
	}
	return out, nil
}

// ——— 备份相关 ───────────────────────────────────────────────

// isImportantFile 判断是否为需要备份的重要文件
func isImportantFile(path string) bool {
	// 跳过不重要的路径
	skipPaths := []string{"/tmp/", "/proc/", "/sys/", "/dev/", "/run/"}
	for _, skip := range skipPaths {
		if strings.HasPrefix(path, skip) {
			return false
		}
	}

	// 跳过不重要的扩展名
	skipExts := []string{".log", ".tmp", ".bak", ".swp", ".pid", ".lock"}
	ext := strings.ToLower(filepath.Ext(path))
	for _, se := range skipExts {
		if ext == se {
			return false
		}
	}

	// 跳过 /var/log/
	if strings.HasPrefix(path, "/var/log/") {
		return false
	}

	// 重要扩展名
	importantExts := []string{
		".conf", ".cfg", ".json", ".yaml", ".yml", ".xml",
		".properties", ".env", ".ini", ".toml", ".sh",
		".htpasswd", ".pem", ".crt", ".key",
	}
	for _, ie := range importantExts {
		if ext == ie {
			return true
		}
	}

	// 重要路径关键词
	importantPaths := []string{
		"/etc/", "nginx", "config", "conf.d", "sites-available",
		"application.properties", "application.yml",
	}
	lowerPath := strings.ToLower(path)
	for _, ip := range importantPaths {
		if strings.Contains(lowerPath, ip) {
			return true
		}
	}

	return false
}

// backupFile 备份文件到 ~/.ruoyi-backup/<timestamp>/<相对路径>
func backupFile(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}

	ts := time.Now().Format("20060102-150405")
	backupDir := filepath.Join(home, ".ruoyi-backup", ts)

	// 构造备份目标路径（保留原始路径结构）
	relPath := absPath
	if filepath.IsAbs(relPath) {
		relPath = strings.TrimPrefix(relPath, "/")
	}
	destPath := filepath.Join(backupDir, relPath)

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", err
	}

	src, err := os.Open(absPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}

	return destPath, nil
}

// ——— 系统检测 ───────────────────────────────────────────────

// detectDistro 检测 Linux 发行版和包管理器名称（用于展示）
func detectDistro() (distro, pkgMgr string) {
	// 读取 /etc/os-release
	data, err := os.ReadFile("/etc/os-release")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				distro = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
				break
			}
			if distro == "" && strings.HasPrefix(line, "NAME=") {
				distro = strings.Trim(strings.TrimPrefix(line, "NAME="), `"`)
			}
		}
	}

	// 检测包管理器
	switch {
	case commandExists("apt-get"):
		pkgMgr = "apt"
	case commandExists("dnf"):
		pkgMgr = "dnf"
	case commandExists("yum"):
		pkgMgr = "yum"
	case commandExists("pacman"):
		pkgMgr = "pacman"
	case commandExists("apk"):
		pkgMgr = "apk"
	case commandExists("zypper"):
		pkgMgr = "zypper"
	default:
		pkgMgr = "unknown"
	}

	if distro == "" {
		// macOS
		if out, err := exec.Command("sw_vers", "-productName").Output(); err == nil {
			ver, _ := exec.Command("sw_vers", "-productVersion").Output()
			distro = strings.TrimSpace(string(out)) + " " + strings.TrimSpace(string(ver))
			pkgMgr = "brew"
		} else {
			distro = "Unknown"
		}
	}

	return distro, pkgMgr
}

// getPackageManagerCmd 返回 (命令名, 安装子命令参数列表)
func getPackageManagerCmd() (string, []string) {
	type pm struct {
		cmd  string
		args []string
	}
	managers := []pm{
		{"apt-get", []string{"install", "-y"}},
		{"dnf", []string{"install", "-y"}},
		{"yum", []string{"install", "-y"}},
		{"pacman", []string{"-S", "--noconfirm"}},
		{"apk", []string{"add"}},
		{"zypper", []string{"install", "-y"}},
	}
	for _, m := range managers {
		if commandExists(m.cmd) {
			return m.cmd, m.args
		}
	}
	return "", nil
}

// commandExists 检查命令是否存在于 PATH
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// expandHome 展开路径中的 ~
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// formatSize 格式化文件大小
func formatSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case size >= GB:
		return fmt.Sprintf("%.1fG", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.1fM", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.1fK", float64(size)/KB)
	default:
		return fmt.Sprintf("%dB", size)
	}
}

// emptyParams 返回无参数的 JSON Schema
func emptyParams() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
		"required":   []string{},
	}
}

// ——— 辅助函数 ───────────────────────────────────────────────

func buildScriptEnv(execCtx ExecContext, svc *config.ServiceConfig) []string {
	env := os.Environ()
	bluePort := extractPort(svc.BlueTarget)
	greenPort := extractPort(svc.GreenTarget)
	appName := svc.AppName
	if appName == "" {
		appName = "ruoyi"
	}
	jarFile := svc.JarFile
	if jarFile == "" {
		jarFile = "ruoyi-*.jar"
	}
	env = append(env, "SERVICE_ID="+execCtx.CurrentService)
	env = append(env, "APP_NAME="+appName)
	env = append(env, "APP_JAR_PATTERN="+jarFile)
	env = append(env, "BLUE_PORT="+bluePort)
	env = append(env, "GREEN_PORT="+greenPort)
	if execCtx.AppHome != "" {
		env = append(env, "APP_HOME="+execCtx.AppHome)
	}
	return env
}

func extractPort(target string) string {
	parts := strings.Split(target, ":")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

func isPortOpen(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) (string, error) {
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return "", err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		out := strings.TrimSpace(buf.String())
		if err != nil {
			return out, fmt.Errorf("命令执行失败: %v", err)
		}
		return out, nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("命令超时（%s）", timeout)
	}
}

// truncateOutput 截断过长的工具输出
func truncateOutput(s string, maxChars int) string {
	if len(s) <= maxChars {
		return s
	}
	half := maxChars / 2
	return s[:half] + fmt.Sprintf("\n\n[... 中间 %d 字节已省略 ...]\n\n", len(s)-maxChars) + s[len(s)-half:]
}

// findScriptPath 查找 service.sh 的绝对路径
func findScriptPath() string {
	paths := []string{"scripts/service.sh", "./scripts/service.sh", "../scripts/service.sh"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			abs, err := filepath.Abs(p)
			if err == nil {
				return abs
			}
		}
	}
	return ""
}

// BuildExecContext 从当前工作目录构建 ExecContext
func BuildExecContext(currentService string) ExecContext {
	scriptPath := findScriptPath()
	appHome := ""
	if scriptPath != "" {
		appHome = filepath.Dir(filepath.Dir(scriptPath))
	}
	return ExecContext{
		CurrentService: currentService,
		AppHome:        appHome,
		ScriptPath:     scriptPath,
	}
}
