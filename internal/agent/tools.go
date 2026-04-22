package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	{
		Name:        "get_status",
		Description: "查询所有服务的运行状态和代理状态",
		ReadOnly:    true,
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
			"required":   []string{},
		},
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
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
			"required":   []string{},
		},
	},
	{
		Name:        "get_system_info",
		Description: "查看系统资源：内存、磁盘、Java 版本",
		ReadOnly:    true,
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
			"required":   []string{},
		},
	},
	{
		Name:        "service_control",
		Description: "控制服务：start（启动）、stop（停止）、restart（重启）、deploy（蓝绿部署）。此操作需要用户确认",
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
		Description: "切换蓝绿环境：将当前服务的流量切换到 blue 或 green 环境。此操作需要用户确认",
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
		Description: "切换 JVM 预设档位：1=1核2G, 2=2核4G, 3=4核8G。此操作需要用户确认，重启后生效",
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
	default:
		return "", fmt.Errorf("未知工具: %s", name)
	}
}

// ——— 工具实现 ———

func (e *ToolExecutor) getStatus() (string, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return "", fmt.Errorf("读取配置失败: %v", err)
	}

	var sb strings.Builder
	// 代理状态
	proxyRunning := isPortOpen("127.0.0.1" + config.ProxyPort)
	if proxyRunning {
		sb.WriteString("代理服务: ✓ 运行中\n")
	} else {
		sb.WriteString("代理服务: ✗ 未运行\n")
	}
	sb.WriteString(fmt.Sprintf("代理端口: %s\n\n", config.ProxyPort))

	// 服务列表
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

		// 健康检查
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
	out, err := runWithTimeout(cmd, 15*time.Second)
	return out, err
}

func (e *ToolExecutor) getConfig() (string, error) {
	var sb strings.Builder

	// 代理配置
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

	// JVM 配置
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

	cmds := []struct {
		label string
		name  string
		args  []string
	}{
		{"Java 版本", "java", []string{"-version"}},
		{"内存使用", "free", []string{"-h"}},
		{"磁盘使用", "df", []string{"-h", "."}},
	}

	for _, c := range cmds {
		sb.WriteString(fmt.Sprintf("--- %s ---\n", c.label))
		cmd := exec.Command(c.name, c.args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			sb.WriteString("  未安装或不可用\n")
		} else {
			sb.WriteString(strings.TrimSpace(string(out)) + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

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
	out, err := runWithTimeout(cmd, 120*time.Second)
	return out, err
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

// ——— 辅助函数 ———

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

// truncateOutput 截断过长的工具输出，避免 token 浪费
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
