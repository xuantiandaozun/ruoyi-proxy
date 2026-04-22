package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	maxReActIterations = 10
	toolOutputMaxChars = 3000
)

// Agent AI 对话代理，实现 ReAct 循环
type Agent struct {
	provider  Provider
	executor  *ToolExecutor
	ctx       *ContextManager
	aiCfg     AIConfig
	execCtx   ExecContext
	// 回调函数（由 CLI 注入）
	confirm   func(prompt string) bool      // 写操作确认
	readInput func(prompt string) (string, error) // 读用户输入
	print     func(s string)                // 普通输出
}

// New 创建 Agent
func New(
	aiCfg AIConfig,
	execCtx ExecContext,
	confirm func(string) bool,
	readInput func(string) (string, error),
	print func(string),
) (*Agent, error) {
	provider, err := NewProvider(aiCfg)
	if err != nil {
		return nil, err
	}
	return &Agent{
		provider:  provider,
		executor:  NewToolExecutor(execCtx),
		ctx:       NewContextManager(aiCfg.ContextLimit),
		aiCfg:     aiCfg,
		execCtx:   execCtx,
		confirm:   confirm,
		readInput: readInput,
		print:     print,
	}, nil
}

// Run 启动 Agent 交互循环
func (a *Agent) Run() {
	// 系统提示词
	systemPrompt := a.aiCfg.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = a.defaultSystemPrompt()
	}
	a.ctx.Add(Message{Role: "system", Content: systemPrompt})

	a.print(fmt.Sprintf("\n\033[1;34m═══ AI Agent 模式 ═══\033[0m"))
	a.print(fmt.Sprintf("提供商: \033[1;36m%s\033[0m  模型: \033[1;36m%s\033[0m",
		a.aiCfg.Provider, a.aiCfg.Model))
	a.print("输入问题或指令，\033[1;33m'exit'\033[0m 退出，\033[1;33m'clear'\033[0m 清空历史\n")

	for {
		input, err := a.readInput("\033[1;32mYou\033[0m: ")
		if err != nil {
			break
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		switch strings.ToLower(input) {
		case "exit", "quit", "q":
			a.print("已退出 Agent 模式")
			return
		case "clear":
			a.ctx.Clear()
			a.ctx.Add(Message{Role: "system", Content: systemPrompt})
			a.print("\033[1;36mℹ 对话历史已清空\033[0m")
			continue
		case "history":
			a.print("\033[1;36mℹ " + a.ctx.Summary() + "\033[0m")
			continue
		}

		a.ctx.Add(Message{Role: "user", Content: input})
		if err := a.runReAct(); err != nil {
			a.print(fmt.Sprintf("\n\033[1;31m✗ 错误: %v\033[0m\n", err))
		}
	}
}

// runReAct 执行 ReAct 循环（think → act → observe）
func (a *Agent) runReAct() error {
	bgCtx := context.Background()

	for iter := 0; iter < maxReActIterations; iter++ {
		// —— Think：调用 LLM ——
		eventCh, err := a.provider.Stream(bgCtx, a.ctx.Messages(), AllTools)
		if err != nil {
			return fmt.Errorf("调用 AI 失败: %v", err)
		}

		var textBuf strings.Builder
		var toolCalls []ToolCall
		printedPrefix := false

		// 流式打印 AI 输出
		for event := range eventCh {
			switch event.Type {
			case "text":
				if !printedPrefix {
					fmt.Print("\n\033[1;35mAI\033[0m: ")
					printedPrefix = true
				}
				fmt.Print(event.Text)
				textBuf.WriteString(event.Text)
			case "tool_calls":
				toolCalls = append(toolCalls, event.ToolCalls...)
			case "error":
				return fmt.Errorf("流式输出错误: %v", event.Err)
			}
		}

		if printedPrefix {
			fmt.Println()
		}

		assistantContent := strings.TrimSpace(textBuf.String())

		// 把 assistant 消息写入历史
		a.ctx.Add(Message{
			Role:      "assistant",
			Content:   assistantContent,
			ToolCalls: toolCalls,
		})

		// 没有工具调用 → 本轮结束
		if len(toolCalls) == 0 {
			fmt.Println()
			return nil
		}

		// —— Act + Observe：执行工具调用 ——
		for _, tc := range toolCalls {
			result, err := a.executeToolCall(tc)
			content := result
			if err != nil {
				content = fmt.Sprintf("执行失败: %v", err)
			}
			a.ctx.Add(Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Name,
				Content:    content,
			})
		}
		// 继续下一轮 LLM 推理（分析工具结果）
	}

	return fmt.Errorf("超过最大推理轮数 (%d)", maxReActIterations)
}

// executeToolCall 执行单个工具调用，写操作需要用户确认
func (a *Agent) executeToolCall(tc ToolCall) (string, error) {
	// 查找工具定义
	var toolDef *ToolDef
	for i := range AllTools {
		if AllTools[i].Name == tc.Name {
			toolDef = &AllTools[i]
			break
		}
	}

	// 打印工具调用信息
	fmt.Printf("\n\033[1;36m[工具调用]\033[0m %s", tc.Name)
	argsDisplay := formatArgs(tc.Arguments)
	if argsDisplay != "" {
		fmt.Printf("  %s", argsDisplay)
	}
	fmt.Println()

	// 写操作需要确认
	if toolDef != nil && !toolDef.ReadOnly {
		fmt.Println()
		fmt.Println("\033[1;33m┌────────────────────────────────────┐\033[0m")
		fmt.Printf("\033[1;33m│  ⚠  即将执行写操作                   │\033[0m\n")
		fmt.Printf("\033[1;33m│  工具: %-30s│\033[0m\n", tc.Name)
		if argsDisplay != "" {
			fmt.Printf("\033[1;33m│  参数: %-30s│\033[0m\n", argsDisplay)
		}
		fmt.Println("\033[1;33m└────────────────────────────────────┘\033[0m")

		if !a.confirm("\033[1;33m确认执行? (y/n): \033[0m") {
			return "用户取消了此操作", nil
		}
	}

	// 执行工具
	result, err := a.executor.Execute(tc.Name, tc.Arguments)
	if err != nil {
		return "", err
	}

	// 截断过长输出
	result = truncateOutput(result, toolOutputMaxChars)

	// 简短打印执行结果摘要（前 2 行）
	lines := strings.SplitN(result, "\n", 4)
	preview := strings.Join(lines[:min(2, len(lines))], "\n")
	if strings.TrimSpace(preview) != "" {
		fmt.Printf("\033[0;37m%s\033[0m\n", preview)
		if len(lines) > 2 {
			fmt.Printf("\033[0;37m...(共 %d 行)\033[0m\n", len(strings.Split(result, "\n")))
		}
	}

	return result, nil
}

// formatArgs 格式化工具参数为可读形式
func formatArgs(argsJSON string) string {
	if argsJSON == "" || argsJSON == "{}" {
		return ""
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return argsJSON
	}
	var parts []string
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, "  ")
}

// defaultSystemPrompt 返回默认系统提示词
func (a *Agent) defaultSystemPrompt() string {
	return fmt.Sprintf(`你是若依蓝绿部署管理助手，同时也是一名经验丰富的 Linux 运维工程师。当前操作的服务: %s

## 能力范围

**若依应用管理**
- 查询服务状态、日志、JVM 配置、蓝绿环境
- 执行启动/停止/重启/部署/环境切换操作

**服务器文件管理**
- read_file: 查看任意文件内容
- list_directory: 列出目录内容
- write_file: 创建或修改文件（重要配置文件自动备份）
- delete_file: 删除文件（重要文件自动备份到 ~/.ruoyi-backup/）

**系统服务与软件安装**
- systemd_info: 查询 nginx/mysql/redis 等服务状态和日志
- manage_systemd: 启动/停止/重启/开机自启系统服务
- install_package: 自动识别发行版（apt/yum/dnf/pacman/apk），安装软件包
- run_shell: 执行任意 shell 命令（解压、复制、权限等复杂操作）

## 工具调用原则
- 只读操作（查状态/看日志/读文件/列目录）直接调用，无需提前告知
- **写操作**（文件修改/删除/安装软件/服务控制）系统会弹出确认框，你无需再次询问
- 重要配置文件（*.conf/*.json/*.sh 等）写入或删除前会**自动备份**，可放心操作
- install_package 会自动识别当前系统的包管理器，你只需提供包名
- 工具结果较长时，提炼关键信息回复；遇到错误，分析原因并提供解决方案

## 回复风格
- 使用中文，简洁明了
- 执行操作后说明结果和影响
- 遇到问题主动建议下一步排查方向`, a.execCtx.CurrentService)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
