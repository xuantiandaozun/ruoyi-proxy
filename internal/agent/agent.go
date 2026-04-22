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
	return fmt.Sprintf(`你是若依蓝绿部署管理助手。当前操作的服务: %s

你的职责:
- 通过工具查询服务状态、日志和配置，分析问题并给出建议
- 执行用户确认过的服务操作（启动、停止、重启、部署、环境切换）
- 解答部署、Java 运维、蓝绿发布相关的技术问题

工具调用规则:
- 只读查询（状态、日志、配置）直接调用工具获取信息
- 服务控制操作（restart/deploy/switch）系统会请用户确认，无需重复询问
- 工具返回内容可能较长，请提炼关键信息告知用户
- 回复简洁明了，使用中文`, a.execCtx.CurrentService)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
