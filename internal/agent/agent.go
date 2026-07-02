package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"

	"ruoyi-proxy/internal/buildinfo"
)

const (
	maxReActIterations = 30 // 单次 ReAct 最大推理轮数
	maxAutoResume      = 5  // 超限后最多自动续接次数（总计最多 30×5=150 轮）
	toolOutputMaxChars = 3000
	autoResumePrompt   = "请继续完成上面未完成的任务。"
	autoTitleTurns     = 5
)

var errAgentInterrupted = errors.New("agent interrupted")

// Agent AI 对话代理，实现 ReAct 循环
type Agent struct {
	provider     Provider
	executor     *ToolExecutor
	ctx          *ContextManager
	aiCfg        AIConfig
	execCtx      ExecContext
	sessionStore *SessionStore
	current      *SessionMeta
	systemPrompt string
	runMu        sync.Mutex
	runCancel    context.CancelFunc // 当前用户轮次的取消函数，用于 Ctrl+C 打断任务
	lastInput    string             // 用户最后一条消息，用于判断是否已提前确认
	turnApproved bool               // 当前用户轮次是否已批准写操作（一次批准覆盖整轮）
	// 回调函数（由 CLI 注入）
	confirm        func(prompt string) bool             // 写操作确认
	readInput      func(prompt string) (string, error)  // 读用户输入
	print          func(s string)                       // 普通输出
	opsCommand     func(cmd string, args []string) bool // 运维斜杠命令（由 CLI 注入）
	opsHelp        func()                               // 运维命令帮助
	slashMenuItems func() []SlashCommandItem            // 完整斜杠命令菜单（由 CLI 注入）
}

// New 创建 Agent
func New(
	aiCfg AIConfig,
	execCtx ExecContext,
	confirm func(string) bool,
	readInput func(string) (string, error),
	print func(string),
) (*Agent, error) {
	var provider Provider
	if aiCfg.IsConfigured() {
		p, err := NewProvider(aiCfg)
		if err != nil {
			return nil, err
		}
		provider = p
	}
	store, err := NewSessionStore()
	if err != nil {
		return nil, fmt.Errorf("初始化会话存储失败: %v", err)
	}
	return &Agent{
		provider:     provider,
		executor:     NewToolExecutor(execCtx),
		ctx:          NewContextManager(aiCfg.ContextLimit),
		aiCfg:        aiCfg,
		execCtx:      execCtx,
		sessionStore: store,
		confirm:      confirm,
		readInput:    readInput,
		print:        print,
	}, nil
}

// SetOpsHooks 注入运维斜杠命令回调（由 CLI 包设置，避免 agent 依赖 cli）
func (a *Agent) SetOpsHooks(dispatch func(cmd string, args []string) bool, help func()) {
	a.opsCommand = dispatch
	a.opsHelp = help
}

// SetSlashMenuItems 注入完整斜杠命令列表（输入 / 时展示）
func (a *Agent) SetSlashMenuItems(fn func() []SlashCommandItem) {
	a.slashMenuItems = fn
}

func (a *Agent) pickSlashCommand(filter string) (string, bool) {
	items := a.defaultSlashMenuItems()
	if a.slashMenuItems != nil {
		items = a.slashMenuItems()
	}
	items = filterSlashCommands(items, filter)
	if len(items) == 0 {
		return "", false
	}
	selected, ok, err := selectSlashCommandInteractive(items, a.readInput)
	if err != nil || !ok {
		return "", false
	}
	return selected, true
}

// PickSlashCommand 打开斜杠命令菜单（filter 为空=全部，否则前缀过滤）
func (a *Agent) PickSlashCommand(filter string) (string, bool) {
	return a.pickSlashCommand(filter)
}

func (a *Agent) defaultSlashMenuItems() []SlashCommandItem {
	return []SlashCommandItem{
		{Command: "/sessions", Description: "查看历史会话列表"},
		{Command: "/load", Description: "打开会话选择器并加载"},
		{Command: "/new", Description: "创建新的空会话"},
		{Command: "/current", Description: "查看当前会话信息"},
		{Command: "/help", Description: "查看命令说明"},
		{Command: "/exit", Description: "退出 Agent 模式"},
	}
}

// ReloadConfig 热重载 AI 配置（/agent-config 保存后调用）
func (a *Agent) ReloadConfig(cfg AIConfig) error {
	var provider Provider
	if cfg.IsConfigured() {
		p, err := NewProvider(cfg)
		if err != nil {
			return err
		}
		provider = p
	}
	a.provider = provider
	a.aiCfg = cfg
	return nil
}

// Cancel 取消当前执行中的用户轮次，不退出 Agent 模式。
func (a *Agent) Cancel() {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	if a.runCancel != nil {
		a.runCancel()
	}
}

// Run 启动 Agent 交互循环
func (a *Agent) Run() {
	a.systemPrompt = a.aiCfg.SystemPrompt
	if a.systemPrompt == "" {
		a.systemPrompt = a.defaultSystemPrompt()
	}
	if err := a.startNewSession(); err != nil {
		a.print(fmt.Sprintf("\033[1;31m✗ 创建初始会话失败: %v\033[0m", err))
		return
	}

	a.print(fmt.Sprintf("\n\033[1;34m═══ AI Agent 模式 ═══\033[0m"))
	if a.aiCfg.IsConfigured() {
		a.print(fmt.Sprintf("提供商: \033[1;36m%s\033[0m  模型: \033[1;36m%s\033[0m",
			a.aiCfg.Provider, a.aiCfg.Model))
	} else {
		a.print("\033[1;33m⚠ AI 未配置，可用 /agent-config 配置；运维命令如 /status /deploy 可直接使用\033[0m")
	}
	a.print("输入问题或指令，\033[1;33m'/help'\033[0m 查看命令，\033[1;33m'/' \033[0m打开命令菜单（↑/↓ 选择），\033[1;33m'/exit'\033[0m 退出")
	a.print("\033[1;33m'Ctrl+C'\033[0m 运行中断任务/空闲时退出，\033[1;33m'clear'\033[0m 清空会话\n")

	inputRetry := 0
	for {
		input, err := a.readInput("\033[1;32mYou\033[0m: ")
		if err != nil {
			if errors.Is(err, io.EOF) {
				a.persistSession()
				a.print("已退出 Agent 模式")
				return
			}
			inputRetry++
			if inputRetry >= 3 {
				a.print(fmt.Sprintf("\n\033[1;31m✗ 连续%d次读取输入失败，退出 Agent\033[0m", inputRetry))
				a.persistSession()
				break
			}
			a.print(fmt.Sprintf("\n\033[1;33m⚠ 读取输入失败（%d/3），5秒后重试...\033[0m", inputRetry))
			time.Sleep(5 * time.Second)
			continue
		}
		inputRetry = 0
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if strings.HasPrefix(input, "/") {
			if ok := a.handleSlashCommand(input); !ok {
				return
			}
			continue
		}
		switch strings.ToLower(input) {
		case "exit", "quit", "q":
			a.persistSession()
			a.print("已退出 Agent 模式")
			return
		case "clear":
			a.ctx.Clear()
			a.ctx.ReplaceSystem(a.systemPrompt)
			a.persistSession()
			a.print("\033[1;36mℹ 对话历史已清空\033[0m")
			continue
		case "history":
			a.print("\033[1;36mℹ " + a.ctx.Summary() + "\033[0m")
			continue
		case "help", "h", "?":
			a.printCombinedHelp()
			continue
		}

		if !a.aiCfg.IsConfigured() {
			a.print("\033[1;33m⚠ AI 未配置，请使用 /agent-config 完成配置后再对话；运维命令可直接用 /status 等\033[0m")
			continue
		}

		a.lastInput = input    // 记录用户最后一条消息，供写操作确认使用
		a.turnApproved = false // 新用户轮次，重置批准状态
		a.ctx.Add(Message{Role: "user", Content: input})
		a.persistSession()
		if err := a.runReAct(); err != nil {
			if errors.Is(err, errAgentInterrupted) {
				a.print("\n\033[1;33m◉ 已中断当前任务\033[0m\n")
				continue
			}
			a.print(fmt.Sprintf("\n\033[1;31m✗ 错误: %v\033[0m\n", err))
			continue
		}
		a.maybeAutoTitle()
		a.persistSession()
	}
}

func (a *Agent) handleSlashCommand(input string) bool {
	parts := strings.Fields(strings.TrimSpace(input))
	cmd := strings.ToLower(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(strings.Join(parts[1:], " "))
	}
	if cmd == "/" {
		selected, ok := a.pickSlashCommand("")
		if !ok {
			return true
		}
		cmd = selected
		if arg != "" {
			// 保留原参数，例如 /logs 200 在菜单选中 /logs 后仍可用
		}
		_ = arg
	}

	switch cmd {
	case "/help", "/commands":
		a.printCombinedHelp()
		return true
	case "/sessions":
		a.showSessions()
		return true
	case "/load":
		if err := a.loadSessionByRef(arg); err != nil {
			a.print(fmt.Sprintf("\033[1;31m✗ %v\033[0m", err))
		}
		return true
	case "/new":
		a.persistSession()
		if err := a.startNewSession(); err != nil {
			a.print(fmt.Sprintf("\033[1;31m✗ 新建会话失败: %v\033[0m", err))
		} else {
			a.print(fmt.Sprintf("\033[1;36mℹ 已创建新会话: %s (%s)\033[0m", a.current.Title, a.current.ID))
		}
		return true
	case "/current":
		if a.current != nil {
			a.print(fmt.Sprintf("\033[1;36mℹ 当前会话: %s (%s)\033[0m", a.current.Title, a.current.ID))
		}
		return true
	case "/exit":
		a.persistSession()
		a.print("已退出 Agent 模式")
		return false
	default:
		if a.opsCommand != nil {
			opCmd := strings.TrimPrefix(cmd, "/")
			opArgs := []string{}
			if arg != "" {
				opArgs = strings.Fields(arg)
			}
			if a.opsCommand(opCmd, opArgs) {
				return true
			}
		}
		if selected, ok := a.pickSlashCommand(strings.TrimPrefix(cmd, "/")); ok {
			return a.handleSlashCommand(selected)
		}
		a.print("\033[1;33m⚠ 未知命令，输入 / 打开命令菜单，或 /help 查看说明\033[0m")
		return true
	}
}

func (a *Agent) printCombinedHelp() {
	a.print("\033[1;34m═══ 会话命令 ═══\033[0m")
	a.print("  /sessions  /load [编号|ID]  /new  /current  /help  /exit")
	a.print("  clear=清空对话  history=上下文摘要  Ctrl+C=中断当前任务")
	if a.opsHelp != nil {
		a.print("")
		a.opsHelp()
	}
}

func (a *Agent) startNewSession() error {
	a.ctx = NewContextManager(a.aiCfg.ContextLimit)
	a.ctx.Add(Message{Role: "system", Content: a.systemPrompt})
	meta, err := a.sessionStore.CreateSession(a.ctx.Messages())
	if err != nil {
		return err
	}
	a.current = meta
	return nil
}

func (a *Agent) persistSession() {
	if a.current == nil {
		return
	}
	if err := a.sessionStore.SaveSession(a.current, a.ctx.Messages()); err != nil {
		a.print(fmt.Sprintf("\033[1;31m✗ 保存会话失败: %v\033[0m", err))
	}
}

func (a *Agent) showSessions() {
	items, err := a.sessionStore.ListSessions()
	if err != nil {
		a.print(fmt.Sprintf("\033[1;31m✗ 读取会话列表失败: %v\033[0m", err))
		return
	}
	a.print(formatSessionList(items))
	a.print("\033[1;36mℹ 使用 /load 打开交互式会话选择器\033[0m")
}

func (a *Agent) loadSessionByRef(ref string) error {
	a.persistSession()
	items, err := a.sessionStore.ListSessions()
	if err != nil {
		return fmt.Errorf("读取会话列表失败: %v", err)
	}
	if len(items) == 0 {
		return fmt.Errorf("暂无可加载的历史会话")
	}
	if strings.TrimSpace(ref) == "" {
		choice, ok, err := selectSessionInteractive(items, a.currentID(), a.readInput)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		ref = choice
	}
	id, err := a.sessionStore.ResolveSessionID(ref)
	if err != nil {
		return err
	}
	meta, messages, err := a.sessionStore.LoadSession(id)
	if err != nil {
		return fmt.Errorf("加载会话失败: %v", err)
	}
	a.ctx = NewContextManager(a.aiCfg.ContextLimit)
	for _, msg := range messages {
		a.ctx.Add(msg)
	}
	a.ctx.ReplaceSystem(a.systemPrompt)
	a.current = meta
	a.persistSession()
	a.print(fmt.Sprintf("\033[1;36mℹ 已加载会话: %s (%s)\033[0m", meta.Title, meta.ID))
	a.print("\033[1;36mℹ " + a.ctx.Summary() + "\033[0m")
	return nil
}

func (a *Agent) currentID() string {
	if a.current == nil {
		return ""
	}
	return a.current.ID
}

func (a *Agent) maybeAutoTitle() {
	if a.current == nil || a.current.AutoTitled {
		return
	}
	if countRealUserTurns(a.ctx.Messages()) < autoTitleTurns {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	title, err := a.generateSessionTitle(ctx)
	if err != nil || title == "" {
		return
	}
	a.current.Title = title
	a.current.AutoTitled = true
	a.persistSession()
	a.print(fmt.Sprintf("\033[1;36mℹ 会话已命名为: %s\033[0m", title))
}

func (a *Agent) generateSessionTitle(ctx context.Context) (string, error) {
	snippet := buildTitleSnippet(a.ctx.Messages())
	if snippet == "" {
		return "", nil
	}
	resp, err := a.provider.Chat(ctx, []Message{
		{Role: "system", Content: "你是会话标题生成器。请基于给定对话内容，输出一个 8 到 18 个字的中文标题，只输出标题本身，不要解释，不要引号，不要序号。"},
		{Role: "user", Content: snippet},
	}, nil)
	if err != nil {
		return "", err
	}
	title := sanitizeSessionTitle(resp.Content)
	if title == "" {
		return "", nil
	}
	return title, nil
}

// runReAct 执行 ReAct 循环，支持自动续接
func (a *Agent) runReAct() error {
	ctx, cancel := context.WithCancel(context.Background())
	a.runMu.Lock()
	a.runCancel = cancel
	a.runMu.Unlock()
	defer func() {
		cancel()
		a.runMu.Lock()
		a.runCancel = nil
		a.runMu.Unlock()
	}()

	for resume := 0; resume <= maxAutoResume; resume++ {
		done, err := a.runReActOnce(ctx)
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
				Content: autoResumePrompt,
			})
		}
	}
	return fmt.Errorf("任务超出最大处理轮数（%d 轮 × %d 次续接），请尝试分步执行",
		maxReActIterations, maxAutoResume)
}

// runReActOnce 执行单次 ReAct 循环，返回 (是否正常完成, error)
// 返回 false, nil 表示达到迭代上限，需要续接
func (a *Agent) runReActOnce(ctx context.Context) (bool, error) {
	for iter := 0; iter < maxReActIterations; iter++ {
		// 检查用户是否中断
		select {
		case <-ctx.Done():
			return false, errAgentInterrupted
		default:
		}

		// —— Think：调用 LLM ——
		eventCh, err := a.provider.Stream(ctx, a.ctx.Messages(), AllTools)
		if err != nil {
			// 区分取消错误和真实错误
			if ctx.Err() != nil {
				return false, errAgentInterrupted
			}
			return false, fmt.Errorf("调用 AI 失败: %v", err)
		}

		var textBuf strings.Builder
		var toolCalls []ToolCall
		var reasoningContent string

		fmt.Print("\n\033[1;35mAI\033[0m:\n")
		ms := newMDStream()

		for event := range eventCh {
			// 流式输出过程中也检查中断
			select {
			case <-ctx.Done():
				ms.finish()
				fmt.Println()
				return false, errAgentInterrupted
			default:
			}

			switch event.Type {
			case "text":
				ms.feed(event.Text)
				textBuf.WriteString(event.Text)
			case "tool_calls":
				toolCalls = append(toolCalls, event.ToolCalls...)
			case "done":
				reasoningContent = event.ReasoningContent
			case "error":
				if ctx.Err() != nil {
					return false, errAgentInterrupted
				}
				return false, fmt.Errorf("流式输出错误: %v", event.Err)
			}
		}

		ms.finish()
		fmt.Println()

		assistantContent := strings.TrimSpace(textBuf.String())

		// 把 assistant 消息写入历史（reasoning_content 需原样传回，否则思考模式模型报 400）
		a.ctx.Add(Message{
			Role:             "assistant",
			Content:          assistantContent,
			ReasoningContent: reasoningContent,
			ToolCalls:        toolCalls,
		})

		// 没有工具调用 → 正常完成
		if len(toolCalls) == 0 {
			fmt.Println()
			return true, nil
		}

		// —— Act + Observe：执行工具调用 ——
		for _, tc := range toolCalls {
			// 执行前再次检查中断
			select {
			case <-ctx.Done():
				return false, errAgentInterrupted
			default:
			}

			result, err := a.executeToolCall(tc)
			content := result
			if err != nil {
				content = fmt.Sprintf("执行失败: %v", err)
			}
			// 确保工具结果非空：空字符串会导致 Anthropic API 的 content 字段被
			// omitempty 省略，进而被解析为 null，触发 400 错误
			if strings.TrimSpace(content) == "" {
				content = "执行成功（命令无输出）"
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

	argsDisplay := formatArgs(tc.Arguments)
	// 仅展示工具名称，参数和结果只进入上下文，不刷屏给用户。
	fmt.Printf("\n\033[1;36m[工具调用]\033[0m %s\n", tc.Name)

	// 判断本次调用是否真正需要确认：
	// - ToolDef.ReadOnly=true  → 无需确认
	// - run_shell 但命令是只读性质 → 无需确认
	// - 其他写操作 → 需要确认
	needsConfirm := toolDef != nil && !toolDef.ReadOnly
	if needsConfirm && tc.Name == "run_shell" && isReadOnlyShellCmd(tc.Arguments) {
		needsConfirm = false
	}

	if needsConfirm {
		// 情况1：用户消息本身是确认词，或本轮次已手动批准过 → 静默自动确认
		if !(isStandaloneAffirmative(a.lastInput) || a.turnApproved) {
			// 情况2：需要用户手动确认 → 弹出确认框
			printConfirmBox(tc.Name, argsDisplay)
			if !a.confirm("") {
				fmt.Printf("\033[1;31m  ✗ 已取消\033[0m\n")
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

## 当前节点角色（最高优先级）

%s

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

## 操作优先级（重要）

你的所有操作必须按以下优先级执行，**只有上一级无法满足需求时才降级到下一级**：

### 第一优先级：调用封装好的 CLI 命令（run_shell ./ruoyi-proxy cli <命令>）
所有以下操作**必须优先**使用 CLI 命令，而不是直接操作工具或原始系统命令：

**服务控制** — start / stop / restart / deploy / deploy-lowmem / quick-deploy
**状态查询** — status（概要） / detail（详细）
**日志操作** — logs [行数] / logs-follow / logs-search [名] [关键字] [行数] / logs-export [名] [输出名]
**环境切换** — switch（交互式） / switch blue / switch green
**服务管理** — service-add / service-list / service-remove / service-switch
**代理管理** — proxy-start / proxy-stop / proxy-restart / proxy-status
**HTTPS** — cert <域名> / enable-https / disable-https
**配置管理** — config / config-edit / jvm-config
**初始化** — init

例如：
  run_shell {"command": "cd /path/to/app && ./ruoyi-proxy cli init"}
  run_shell {"command": "cd /path/to/app && ./ruoyi-proxy cli status"}
  run_shell {"command": "cd /path/to/app && ./ruoyi-proxy cli deploy"}
  run_shell {"command": "cd /path/to/app && ./ruoyi-proxy cli switch blue"}
  run_shell {"command": "cd /path/to/app && ./ruoyi-proxy cli logs 100"}
  run_shell {"command": "cd /path/to/app && ./ruoyi-proxy cli cert example.com"}
  run_shell {"command": "cd /path/to/app && ./ruoyi-proxy cli enable-https"}

### 新服务器初始化（非常重要）
当用户要求「安装环境」「装Java」「配置新服务器」「初始化」等时，**必须使用 init 命令**，它一次性完成以下所有操作：
  - Java 17（OpenJDK 17，不是 Java 8）
  - Nginx（配置反向代理）
  - Docker（配置华为云镜像加速）
  - Redis（Docker 容器，默认密码 Redis@200722）
  - 网络工具（netcat、curl）
  - 代理程序 systemd 服务注册
  - 应用配置 + Nginx 配置 + 可选 HTTPS
禁止手动逐个安装这些组件。用户要求安装任何组件时，先确认是否应该运行 init 一次性完成。

### 第二优先级：使用专用工具
当 CLI 命令不满足需求时（如需要查看 Nginx 配置内容、编辑特定文件、排查系统问题等），使用专用工具：

**只读工具**（无需确认）— get_status / get_logs / get_config / get_system_info / read_file / list_directory / systemd_info
**写工具**（需确认）— service_control / switch_env / update_jvm / write_file / delete_file / manage_systemd / install_package

### 第三优先级：原始 shell 命令
仅当上述两级都无法满足需求时，使用 run_shell 直接执行 shell 命令。

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

## 项目自适应（重要）

默认 scripts/service.sh 针对 Java(jar) + 蓝绿部署。当部署失败（找不到 jar）、用户说明非 Java 项目、或目录特征不匹配时，必须主动检测并适配：

1. 用 list_directory / read_file 检查 APP_HOME：pom.xml 或 *.jar → Java；package.json → Node；requirements.txt/manage.py → Python；go.mod → Go；Dockerfile/docker-compose.yml → 容器
2. 判定为非标准 Java 项目后，向用户说明检测结果
3. 参照现有 scripts/service.sh，编写 scripts/service-<serviceID>.sh，**必须实现相同子命令**：start/stop/restart/deploy/deploy-lowmem/status/logs/logs-follow/logs-search/logs-export/help
4. **必须读取环境变量**：SERVICE_ID、APP_NAME、BLUE_PORT、GREEN_PORT、APP_HOME（APP_JAR_PATTERN 可选）
5. 蓝绿切换走代理管理 API（curl localhost:8001/switch?env=...），健康检查用端口连通（nc/curl），勿依赖 Java 日志关键字
6. write_file 创建脚本后，调用 configure_service 注册 script_path 和 project_type

## Hub / Spoke 自检与修复（重要）

- 只有当前节点是 Hub 时，才允许检查或修复 Nginx 的 /__hub__/ 路由。
- 当前节点是 Spoke 时，/self-check 只检查本机基础环境与 Hub 连接；不要检查本机 Nginx 的 location ^~ /__hub__/，也不要用当前服务器域名测试 Hub 路由。
- Spoke 的 Hub 地址是远端 Hub 服务地址（ai.base_url），不是当前服务器域名；排查注册/聊天问题时应请求 ai.base_url 下的 /__hub__/v1/token、/__hub__/v1/register、/__hub__/v1/chat。
- 当用户运行 /self-check 或 Hub 自检发现 nginx:hub路由 失败时，不要自动修改 Nginx，而是先向用户说明风险，再按用户指示使用工具修复。
- Hub 修复 Nginx Hub 路由的标准步骤：
  1. 用 read_file 读取 /etc/nginx/conf.d/ruoyi.conf，确认是否已有 location /__hub__/ 或 location ^~ /__hub__/
  2. 若有旧版 location /__hub__/，先删除整个块（包括前面的 # Hub AI 注释）
  3. 在 server {} 块的靠前位置插入 location ^~ /__hub__/ { ... }，确保用 ^~ 前缀匹配并优先于其他 location
  4. 写入后用 run_shell 执行 sudo nginx -t 验证
  5. 验证通过后执行 manage_systemd reload nginx 或 sudo nginx -s reload
- Hub 路由必须转发到 http://127.0.0.1:8000/__hub__/（如果配置里有 upstream ruoyi_backend，也可写 http://ruoyi_backend/__hub__/，但优先用 127.0.0.1:8000 更直观）
- 修复前必须备份原配置（可用 write_file 时 tool 自动备份，或先 run_shell cp 备份）

## 回复风格
- 使用中文，简洁明了
- 执行操作后说明结果和影响
- 遇到问题主动建议下一步排查方向`, a.execCtx.CurrentService, nodeRolePrompt(a.aiCfg))
}

func nodeRolePrompt(aiCfg AIConfig) string {
	switch {
	case buildinfo.IsHub():
		return `当前是 Hub 智能体：负责集中持有 AI 配置、生成/颁发 Spoke 凭证、转发 AI 请求，并可检查 Hub 服务器上的 /__hub__/ Nginx 路由。`
	case buildinfo.IsSpoke(), aiCfg.Provider == "hub":
		return `当前是 Spoke 智能体：本机通过远端 Hub 调用 AI，本机不是 Hub。不要在本机检查或修复 Hub 的 /__hub__/ Nginx 路由；需要验证 Hub 时，只访问配置的 Hub 地址，不要使用当前服务器域名代替 Hub 域名。`
	default:
		return `当前是普通代理节点：只管理本机蓝绿代理与服务；除非用户明确说明本机是 Hub，否则不要检查或修复 Hub 的 /__hub__/ Nginx 路由。`
	}
}

func buildTitleSnippet(messages []Message) string {
	var lines []string
	for _, msg := range messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" || content == autoResumePrompt {
			continue
		}
		label := "用户"
		if msg.Role == "assistant" {
			label = "助手"
		}
		lines = append(lines, label+": "+trimRunes(strings.ReplaceAll(content, "\n", " "), 120))
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	return strings.Join(lines, "\n")
}

func sanitizeSessionTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "\"'“”‘’`")
	title = strings.ReplaceAll(title, "\r", " ")
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.Join(strings.Fields(title), " ")
	return trimRunes(title, 18)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// printConfirmBox 打印带确认/取消说明的操作确认框
func printConfirmBox(toolName, argsDisplay string) {
	const W = 52 // 框内可见总宽度（中文算2列）

	border := strings.Repeat("─", W)

	// row 打印一行，自动用可见宽度补齐右边空格
	row := func(leftColor, text string) {
		w := visibleWidth(text)
		pad := W - w
		if pad < 0 {
			pad = 0
		}
		// 格式：左边框(黄) + 彩色内容 + 补空格 + 右边框(黄)
		fmt.Printf("\033[1;33m│\033[0m%s%s%s\033[1;33m│\033[0m\n",
			leftColor, text, strings.Repeat(" ", pad))
	}

	// truncLine 截断超宽内容并加省略号
	truncLine := func(prefix, content string) {
		maxContent := W - visibleWidth(prefix)
		lines := splitToWidth(content, maxContent)
		for i, l := range lines {
			if i == 0 {
				row("\033[1;33m", prefix+l)
			} else {
				indent := strings.Repeat(" ", visibleWidth(prefix))
				row("\033[1;33m", indent+l)
			}
		}
	}

	fmt.Println()
	fmt.Printf("\033[1;33m┌%s┐\033[0m\n", border)
	row("\033[1;33m", "  ⚠  即将执行写操作")
	truncLine("  工具: ", toolName)
	if argsDisplay != "" {
		truncLine("  参数: ", argsDisplay)
	}
	fmt.Printf("\033[1;33m├%s┤\033[0m\n", border)
	row("\033[1;32m", "  ✓ 确认: 直接按 Enter 或输入 y")
	row("\033[1;31m", "  ✗ 取消: 输入 n")
	fmt.Printf("\033[1;33m└%s┘\033[0m\n", border)
	fmt.Printf("\033[1;33m▶ \033[0m")
}

// visibleWidth 计算字符串的可见终端宽度（ASCII=1，中文/全角=2）
func visibleWidth(s string) int {
	w := 0
	for _, r := range s {
		if r > 0x7F {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// splitToWidth 将字符串按可见宽度切分为多行
func splitToWidth(s string, maxW int) []string {
	if visibleWidth(s) <= maxW {
		return []string{s}
	}
	var lines []string
	runes := []rune(s)
	cur := 0
	start := 0
	for i, r := range runes {
		cw := 1
		if r > 0x7F {
			cw = 2
		}
		if cur+cw > maxW {
			lines = append(lines, string(runes[start:i]))
			start = i
			cur = cw
		} else {
			cur += cw
		}
	}
	if start < len(runes) {
		lines = append(lines, string(runes[start:]))
	}
	return lines
}

// isReadOnlyShellCmd 判断 run_shell 的 arguments JSON 中的命令是否为只读操作
// 只读命令直接执行，不弹确认框
func isReadOnlyShellCmd(argsJSON string) bool {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return false
	}
	cmd := strings.TrimSpace(fmt.Sprintf("%v", args["command"]))
	if cmd == "" {
		return false
	}

	// 包含这些特征 → 明确是写操作，不跳过确认
	writeSignals := []string{
		">", ">>", // 重定向写入
		"rm ", "rmdir ", // 删除
		"mv ", "cp ", // 移动/复制（可能覆盖）
		"chmod ", "chown ", // 权限修改
		"dd ", "mkfs", "fdisk", // 磁盘操作
		"systemctl ", "service ", // 服务控制（由 manage_systemd 负责）
		"apt", "yum", "dnf", "pacman", "apk", "pip", "npm install", // 安装
		"curl -o", "wget -O", "wget -P", // 下载写入
		"tar -x", "unzip", "gunzip", // 解压
		"kill ", "pkill ", "killall ", // 进程终止
		"passwd ", "useradd ", "userdel", // 用户管理
		"crontab ",          // 定时任务修改
		"iptables ", "ufw ", // 防火墙修改
		"mount ", "umount ", // 挂载
		"sed -i", "awk '", // 原地修改
	}
	for _, sig := range writeSignals {
		if strings.Contains(cmd, sig) {
			return false
		}
	}

	// 这些命令（精确匹配或前缀匹配）→ 明确是只读
	readOnlyPrefixes := []string{
		// nginx 检测
		"nginx -t", "nginx -T",
		// 目录/文件列表
		"ls", "ll", "dir", "pwd", "realpath",
		// 文件查看
		"cat", "less", "more", "head", "tail", "strings",
		// 文本处理（非原地修改）
		"grep", "awk", "sed -n", "sort", "uniq", "wc", "cut", "tr", "diff", "comm",
		// 文件查找
		"find", "locate", "which", "type", "whereis",
		// 磁盘信息
		"df", "du", "lsblk", "blkid", "fdisk -l",
		// 进程/负载
		"ps", "top", "htop", "uptime", "pstree", "pgrep",
		// 内存/IO
		"free", "vmstat", "iostat", "sar", "mpstat",
		// 网络信息（只查看）
		"netstat", "ss", "lsof", "ifconfig", "ip addr", "ip route", "ip link",
		// 网络测试（不修改状态）
		"ping", "traceroute", "tracepath", "nslookup", "dig", "host",
		"curl -s", "curl -I", "curl --silent", "curl --head", "curl --head",
		// 系统信息
		"uname", "hostname", "date", "timedatectl status",
		"lscpu", "lshw", "dmidecode", "inxi",
		// 用户信息
		"whoami", "id", "groups", "w", "who", "last", "lastlog",
		// 文件信息
		"stat", "file", "md5sum", "sha256sum", "sha1sum", "xxd",
		// 环境/变量查看
		"env", "printenv", "set",
		// 版本信息
		"java -version", "java --version",
		"python --version", "python3 --version",
		"node --version", "node -v",
		"go version", "ruby --version", "php --version",
		"nginx -v", "nginx -V",
		"mysql --version", "redis-cli --version",
		// systemd 只读查询
		"systemctl status", "systemctl is-active", "systemctl is-enabled",
		"systemctl list-units", "systemctl list-services",
		// 日志查看
		"journalctl", "dmesg",
		// 包查询（不安装）
		"rpm -q", "rpm -qa", "dpkg -l", "dpkg -s",
		"apt list", "apt show", "yum list", "yum info",
		"dnf list", "dnf info",
	}
	lowerCmd := strings.ToLower(cmd)
	for _, prefix := range readOnlyPrefixes {
		p := strings.ToLower(prefix)
		if lowerCmd == p {
			return true // 精确匹配（如 pwd、ls、date）
		}
		if strings.HasPrefix(lowerCmd, p+" ") || strings.HasPrefix(lowerCmd, p+"\t") {
			return true // 前缀匹配（如 ls -la、cat /etc/nginx.conf）
		}
	}

	return false
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

// renderMarkdown 将 Markdown 文本渲染为带 ANSI 颜色的终端输出
func renderMarkdown(text string) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(120),
	)
	if err != nil {
		return text
	}
	out, err := r.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(out, "\n")
}
