package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// anthropicProvider 实现 Anthropic Messages API（原生格式与 OpenAI 差异较大）
type anthropicProvider struct {
	apiKey    string
	baseURL   string
	model     string
	maxTokens int
	timeout   int
}

func (p *anthropicProvider) Name() string  { return "anthropic" }
func (p *anthropicProvider) Model() string { return p.model }

// ——— Anthropic 请求/响应结构 ———

type anthropicRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	System    string              `json:"system,omitempty"`
	Messages  []anthropicMessage  `json:"messages"`
	Tools     []anthropicToolDef  `json:"tools,omitempty"`
	Stream    bool                `json:"stream"`
}

type anthropicMessage struct {
	Role    string        `json:"role"`
	Content []anthropicBlock `json:"content"`
}

type anthropicBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string      `json:"id,omitempty"`
	Name  string      `json:"name,omitempty"`
	Input interface{} `json:"input,omitempty"`
	// tool_result — Content 必须是字符串且不能缺失，不加 omitempty
	// 避免空字符串被 Go json 的 omitempty+interface{} 省略成 null
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"` // 固定 string，由调用方保证非空
}

type anthropicToolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

type anthropicResponse struct {
	Content []struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Input interface{} `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// ——— 消息格式转换 ———

// convertMessages 把内部 []Message 转成 Anthropic 格式
// Anthropic 规则：
//   - system 抽出作为顶层字段
//   - role=tool 的消息要合并成一个 user 消息，content 类型为 tool_result
//   - 相邻同 role 消息需合并（Anthropic 不允许连续同 role）
func (p *anthropicProvider) convertMessages(messages []Message) (system string, out []anthropicMessage) {
	i := 0
	for i < len(messages) {
		m := messages[i]
		switch m.Role {
		case "system":
			system = m.Content
			i++

		case "user":
			out = append(out, anthropicMessage{
				Role:    "user",
				Content: []anthropicBlock{{Type: "text", Text: m.Content}},
			})
			i++

		case "assistant":
			blocks := []anthropicBlock{}
			if m.Content != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				var input interface{}
				_ = json.Unmarshal([]byte(tc.Arguments), &input)
				if input == nil {
					input = map[string]interface{}{}
				}
				blocks = append(blocks, anthropicBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				})
			}
			out = append(out, anthropicMessage{Role: "assistant", Content: blocks})
			i++

		case "tool":
			// 收集连续的 tool 消息，合并为一个 user 消息
			var results []anthropicBlock
			for i < len(messages) && messages[i].Role == "tool" {
				c := messages[i].Content
				if strings.TrimSpace(c) == "" {
					c = "执行成功（命令无输出）" // 兜底：Anthropic 不接受空 content
				}
				results = append(results, anthropicBlock{
					Type:      "tool_result",
					ToolUseID: messages[i].ToolCallID,
					Content:   c,
				})
				i++
			}
			out = append(out, anthropicMessage{Role: "user", Content: results})

		default:
			i++
		}
	}
	return system, out
}

func (p *anthropicProvider) convertTools(tools []ToolDef) []anthropicToolDef {
	var out []anthropicToolDef
	for _, t := range tools {
		out = append(out, anthropicToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}
	return out
}

// ——— HTTP 辅助 ———

func (p *anthropicProvider) newHTTPReq(ctx context.Context, endpoint string, body []byte, stream bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.baseURL, "/")+endpoint,
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	return req, nil
}

// ——— Chat（非流式）———

func (p *anthropicProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*ChatResponse, error) {
	system, anthropicMsgs := p.convertMessages(messages)
	reqBody := anthropicRequest{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		System:    system,
		Messages:  anthropicMsgs,
		Tools:     p.convertTools(tools),
		Stream:    false,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	httpReq, err := p.newHTTPReq(ctx, "/v1/messages", bodyBytes, false)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: time.Duration(p.timeout) * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var apiResp anthropicResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("API 错误: %s", apiResp.Error.Message)
	}

	var text string
	var toolCalls []ToolCall
	for _, block := range apiResp.Content {
		switch block.Type {
		case "text":
			text += block.Text
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(args),
			})
		}
	}
	return &ChatResponse{Content: text, ToolCalls: toolCalls}, nil
}

// ——— Stream（SSE）———

func (p *anthropicProvider) Stream(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	system, anthropicMsgs := p.convertMessages(messages)
	reqBody := anthropicRequest{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		System:    system,
		Messages:  anthropicMsgs,
		Tools:     p.convertTools(tools),
		Stream:    true,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	httpReq, err := p.newHTTPReq(ctx, "/v1/messages", bodyBytes, true)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("流式请求失败: %v", err)
	}
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var errResp anthropicResponse
		if json.Unmarshal(data, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("API 错误 (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		p.parseAnthropicSSE(resp.Body, ch)
	}()
	return ch, nil
}

// anthropicSSEBlock 用于追踪进行中的 content block
type anthropicSSEBlock struct {
	blockType   string // "text" | "tool_use"
	toolID      string
	toolName    string
	jsonBuilder strings.Builder
}

func (p *anthropicProvider) parseAnthropicSSE(body io.Reader, ch chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	blocks := make(map[int]*anthropicSSEBlock)
	var currentEvent string

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			currentEvent = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

		switch currentEvent {
		case "content_block_start":
			var ev struct {
				Index        int `json:"index"`
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			blocks[ev.Index] = &anthropicSSEBlock{
				blockType: ev.ContentBlock.Type,
				toolID:    ev.ContentBlock.ID,
				toolName:  ev.ContentBlock.Name,
			}

		case "content_block_delta":
			var ev struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			b, ok := blocks[ev.Index]
			if !ok {
				continue
			}
			switch ev.Delta.Type {
			case "text_delta":
				if ev.Delta.Text != "" {
					ch <- StreamEvent{Type: "text", Text: ev.Delta.Text}
				}
			case "input_json_delta":
				b.jsonBuilder.WriteString(ev.Delta.PartialJSON)
			}

		case "content_block_stop":
			var ev struct {
				Index int `json:"index"`
			}
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			b, ok := blocks[ev.Index]
			if !ok {
				continue
			}
			if b.blockType == "tool_use" {
				ch <- StreamEvent{
					Type: "tool_calls",
					ToolCalls: []ToolCall{{
						ID:        b.toolID,
						Name:      b.toolName,
						Arguments: b.jsonBuilder.String(),
					}},
				}
			}
			delete(blocks, ev.Index)

		case "message_stop":
			ch <- StreamEvent{Type: "done"}
			return
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		ch <- StreamEvent{Type: "error", Err: fmt.Errorf("读取流失败: %v", err)}
	}
	ch <- StreamEvent{Type: "done"}
}
