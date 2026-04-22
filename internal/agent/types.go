package agent

// Message 对话消息（内部统一格式，各 provider 负责转换）
type Message struct {
	Role       string     // system | user | assistant | tool
	Content    string
	ToolCalls  []ToolCall // role=assistant 且 AI 要调工具时非空
	ToolCallID string     // role=tool 时填，对应 ToolCall.ID
	Name       string     // role=tool 时填工具名
}

// ToolCall AI 请求调用的单个工具
type ToolCall struct {
	ID        string // provider 返回的唯一 ID
	Name      string
	Arguments string // JSON 字符串
}

// ChatResponse 非流式响应
type ChatResponse struct {
	Content   string
	ToolCalls []ToolCall
}

// StreamEvent 流式输出事件
type StreamEvent struct {
	Type      string     // "text" | "tool_calls" | "done" | "error"
	Text      string     // Type=="text" 时有值
	ToolCalls []ToolCall // Type=="tool_calls" 时有值
	Err       error      // Type=="error" 时有值
}

// ToolDef 工具定义（发给 LLM 的 schema）
type ToolDef struct {
	Name        string
	Description string
	Parameters  interface{} // JSON Schema object
	ReadOnly    bool        // false = 需要用户确认才执行
}

// ExecContext CLI 运行时上下文，传给工具执行器
type ExecContext struct {
	CurrentService string
	AppHome        string // scripts/ 的父目录
	ScriptPath     string // service.sh 的绝对路径
}
