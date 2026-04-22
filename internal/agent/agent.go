package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	maxReActIterations = 30 // 单次 ReAct 最大推理轮数
	maxAutoResume      = 5  // 超限后最多自动续接次数（总计最多 30×5=150 轮）
	toolOutputMaxChars = 3000
)

// Agent AI 对话代理，实现 ReAct 循环
type Agent struct {
	provider     Provider
	executor     *ToolExecutor
	ctx          *ContextManager
	aiCfg        AIConfig
	execCtx      ExecContext
	lastInput    string // 用户最后一条消息，用于判断是否已提前确认
	turnApproved bool   // 当前用户轮次是否已批准写操作（一次批准覆盖整轮）
	// 回调函数（由 CLI 注入）
	confirm   func(prompt string) bool           // 写操作确认
	readInput func(prompt string) (string, error) // 读用户输入
	print     func(s string)                     // 普通输出
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

		a.lastInput = input    // 记录用户最后一条消息，供写操作确认使用
		a.turnApproved = false // 新用户轮次，重置批准状态
		a.ctx.Add(Message{Role: "user", Content: input})
		if err := a.runReAct(); err != nil {
			a.print(fmt.Sprintf("\n\033[1;31m✗ 错误: %v\033[0m\n", err))
		}
	}
}

// runReAct 执行 ReAct 循环，支持自动续接
func (a *Agent) runReAct() error {
	for resume := 0; resume <= maxAutoResume; resume++ {
		done, err := a.runReActOnce()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		// 未完成（达到单次迭代上限），自动续接
		if resume < maxAutoResume {
			fmt.Printf("\n\033[1;36mℹ 任务较复杂，自动继续处理中（第 %d/%d 次续接）...\033[0m\n\n",
				resume+1, maxAutoResume)
			// 注入续接消息，让 AI 知道需要继续
			a.ctx.Add(Message{
				Role:    "user",
				Content: "请继续完成上面未完成的任务。",
			})
		}
	}
	return fmt.Errorf("任务超出最大处理轮数（%d 轮 × %d 次续接），请尝试分步执行",
		maxReActIterations, maxAutoResume)
}

// runReActOnce 执行单次 ReAct 循环，返回 (是否正常完成, error)
// 返回 false, nil 表示达到迭代上限，需要续接
func (a *Agent) runReActOnce() (bool, error) {
	bgCtx := context.Background()

	for iter := 0; iter < maxReActIterations; iter++ {
		// —— Think：调用 LLM ——
		eventCh, err := a.provider.Stream(bgCtx, a.ctx.Messages(), AllTools)
		if err != nil {
			return false, fmt.Errorf("调用 AI 失败: %v", err)
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
				return false, fmt.Errorf("流式输出错误: %v", event.Err)
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

		// 没有工具调用 → 正常完成
		if len(toolCalls) == 0 {
			fmt.Println()
			return true, nil
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

	// 达到单次迭代上限，需要外层续接
	return false, nil
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
		// 情况1：用户消息本身是确认词，或本轮次已手动批准过 → 静默自动确认
		if isStandaloneAffirmative(a.lastInput) || a.turnApproved {
			fmt.Printf("\033[1;32m  ✓ 自动确认（已获得授权）\033[0m\n")
		} else {
			// 情况2：需要用户手动确认 → 弹出确认框
			fmt.Println()
			fmt.Println("\033[1;33m┌────────────────────────────────────┐\033[0m")
			fmt.Printf("\033[1;33m│  ⚠  即将执行写操作                   │\033[0m\n")
			fmt.Printf("\033[1;33m│  工具: %-30s│\033[0m\n", tc.Name)
			if argsDisplay != "" {
				fmt.Printf("\033[1;33m│  参数: %-30s│\033[0m\n", argsDisplay)
			}
			fmt.Println("\033[1;33m└────────────────────────────────────┘\033[0m")
			fmt.Printf("\033[1;33m▶ 直接按 Enter 确认执行，输入 n 取消: \033[0m")

			if !a.confirm("") {
				return "用户取消了此操作", nil
			}
			// 手动确认后，本轮次剩余写操作均自动放行
			a.turnApproved = true
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

## 部署架构（重要）

外部请求的完整链路如下：

  用户浏览器
      ↓ HTTP/HTTPS
  Nginx（反向代理，负责 SSL 终止、域名路由）
      ↓ HTTP（内网）
  若依代理程序（本程序，负责蓝绿流量切换）
      ↓
  ┌─────────────┐
  │ 蓝色实例    │  Java 应用（如 :8080）
  │ 绿色实例    │  Java 应用（如 :8081）
  └─────────────┘

**关键含义**：
- Nginx 是流量入口，SSL 证书、域名绑定、访问限制都在 Nginx 配置
- 若依代理程序在 Nginx 下游，只处理蓝绿切换逻辑
- 排查外部访问问题时，应先检查 Nginx 状态和配置，再检查代理程序，最后检查 Java 应用
- Nginx 配置通常在 /etc/nginx/，修改后需 nginx -t 验证再 reload

## 能力范围

**若依应用管理**
- 查询服务状态、日志、JVM 配置、蓝绿环境
- 执行启动/停止/重启/部署/环境切换操作

**服务器文件管理**
- read_file: 查看任意文件内容（Nginx 配置、应用配置等）
- list_directory: 列出目录内容
- write_file: 创建或修改文件（重要配置文件自动备份）
- delete_file: 删除文件（重要文件自动备份到 ~/.ruoyi-backup/）

**系统服务与软件安装**
- systemd_info: 查询 nginx/mysql/redis 等系统服务状态和日志
- manage_systemd: 启动/停止/重启/开机自启系统服务
- install_package: 自动识别发行版（apt/yum/dnf/pacman/apk），安装软件包
- run_shell: 执行任意 shell 命令

## 工具调用原则
- 只读操作（查状态/看日志/读文件/列目录）直接调用，无需提前告知
- 写操作（文件修改/删除/安装软件/服务控制）系统会弹出确认框，你无需再次询问
- 重要配置文件写入或删除前会自动备份，可放心操作
- 修改 Nginx 配置后，应主动用 run_shell 执行 nginx -t 验证，再 manage_systemd reload nginx
- 工具结果较长时，提炼关键信息回复；遇到错误，分析原因并提供解决方案

## 批量操作规范（重要）
- 需要删除多个文件时，**必须一次性**调用 delete_file 并传入 paths 数组，不要多次调用
  示例：{"paths": ["/path/a", "/path/b", "/path/c"]}
- 需要对多个文件做相似操作时，优先用 run_shell 组合命令一次完成
  示例：rm -f file1 file2 file3 / cp -r src1 src2 dst/
- 绝对不要将一个逻辑操作拆分成多次工具调用，用户只需确认一次

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

// isStandaloneAffirmative 判断用户消息是否是独立的确认词
// 用于：用户已经发送了确认意图时，自动跳过写操作的二次确认框
func isStandaloneAffirmative(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	// 纯确认词列表
	exact := []string{
		"y", "yes", "ok", "好", "是", "嗯",
		"确认", "同意", "可以", "行", "好的",
		"执行", "继续", "去做", "做吧", "没问题",
		"是的", "对", "对的", "去吧",
	}
	for _, w := range exact {
		if s == w {
			return true
		}
	}
	// 以确认词开头且整体较短（≤8个字符）的情况，如"确认删除"、"好的去做"
	if len([]rune(s)) <= 8 {
		prefixes := []string{"确认", "同意", "好的", "可以", "执行", "去做"}
		for _, p := range prefixes {
			if strings.HasPrefix(s, p) {
				return true
			}
		}
	}
	return false
}
