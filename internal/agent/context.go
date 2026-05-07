package agent

import "strings"

const (
	// 工具输出单条最大字符数
	maxToolOutputChars = 3000
	// tool 消息在压缩后保留的内容长度
	compactToolOutputChars = 200
)

// ContextManager 管理对话历史，并在 token 超限时自动压缩
type ContextManager struct {
	messages     []Message
	contextLimit int // 估算 token 上限（4 char ≈ 1 token）
}

// NewContextManager 创建 ContextManager
func NewContextManager(contextLimit int) *ContextManager {
	if contextLimit <= 0 {
		contextLimit = 24000
	}
	return &ContextManager{contextLimit: contextLimit}
}

// Add 追加一条消息，并在需要时触发压缩
func (c *ContextManager) Add(msg Message) {
	// 截断单条工具输出，避免单条消息就把 context 撑满
	if msg.Role == "tool" {
		msg.Content = truncateOutput(msg.Content, maxToolOutputChars)
	}
	c.messages = append(c.messages, msg)
	c.maybeCompact()
}

// Messages 返回当前全部消息（供 provider 使用）
func (c *ContextManager) Messages() []Message {
	return c.messages
}

// Clear 清空历史（保留 system 消息）
func (c *ContextManager) Clear() {
	var system []Message
	for _, m := range c.messages {
		if m.Role == "system" {
			system = append(system, m)
			break
		}
	}
	c.messages = system
}

// ReplaceSystem 替换或添加 system 消息（用于恢复历史时更新上下文）
func (c *ContextManager) ReplaceSystem(content string) {
	systemMsg := Message{Role: "system", Content: content}
	for i, m := range c.messages {
		if m.Role == "system" {
			c.messages[i] = systemMsg
			return
		}
	}
	// 没有 system 消息则插入到最前面
	c.messages = append([]Message{systemMsg}, c.messages...)
}

// Len 返回消息数量
func (c *ContextManager) Len() int { return len(c.messages) }

// estimatedTokens 粗略估算当前 token 数（4 char ≈ 1 token）
func (c *ContextManager) estimatedTokens() int {
	total := 0
	for _, m := range c.messages {
		total += len(m.Content) / 4
		for _, tc := range m.ToolCalls {
			total += len(tc.Arguments) / 4
		}
	}
	return total
}

// maybeCompact 若估算 token 超过 contextLimit 的 80%，压缩旧的工具消息
func (c *ContextManager) maybeCompact() {
	threshold := int(float64(c.contextLimit) * 0.8)
	if c.estimatedTokens() <= threshold {
		return
	}

	// 策略：从最早的 tool 消息开始，截短内容为摘要
	compacted := 0
	for i := range c.messages {
		if c.messages[i].Role != "tool" {
			continue
		}
		content := c.messages[i].Content
		if len(content) <= compactToolOutputChars {
			continue
		}
		// 保留前 compactToolOutputChars 字符 + 省略提示
		c.messages[i].Content = content[:compactToolOutputChars] + "\n[...内容已压缩以节省上下文...]"
		compacted++

		// 压缩后重新检查
		if c.estimatedTokens() <= threshold {
			break
		}
	}

	// 若压缩工具消息后仍超限，删除最早的几轮（保留 system 消息）
	if c.estimatedTokens() > threshold {
		c.dropOldestRound()
	}
}

// dropOldestRound 删除最早的一轮对话（user + assistant + 可能的 tool 消息）
func (c *ContextManager) dropOldestRound() {
	start := 0
	// 跳过 system 消息
	for start < len(c.messages) && c.messages[start].Role == "system" {
		start++
	}
	if start >= len(c.messages) {
		return
	}

	// 找到第一个 user 消息之后的下一个 user 消息，删除中间这一段
	firstUser := -1
	secondUser := -1
	for i := start; i < len(c.messages); i++ {
		if c.messages[i].Role == "user" {
			if firstUser == -1 {
				firstUser = i
			} else {
				secondUser = i
				break
			}
		}
	}

	if firstUser == -1 {
		return
	}

	end := secondUser
	if end == -1 {
		end = len(c.messages)
	}

	// 删除 [firstUser, end) 这段
	c.messages = append(c.messages[:firstUser], c.messages[end:]...)
}

// Summary 返回当前对话摘要（用于显示）
func (c *ContextManager) Summary() string {
	var sb strings.Builder
	tokens := c.estimatedTokens()
	sb.WriteString("对话历史: ")
	counts := map[string]int{}
	for _, m := range c.messages {
		counts[m.Role]++
	}
	sb.WriteString("用户消息 ")
	sb.WriteString(itoa(counts["user"]))
	sb.WriteString(" 条，AI 回复 ")
	sb.WriteString(itoa(counts["assistant"]))
	sb.WriteString(" 条，工具调用 ")
	sb.WriteString(itoa(counts["tool"]))
	sb.WriteString(" 次，估算 ")
	sb.WriteString(itoa(tokens))
	sb.WriteString(" tokens")
	return sb.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
