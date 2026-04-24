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

// openAIProvider 实现 OpenAI Chat Completions API（兼容 DeepSeek/Qwen/Kimi/Ollama 等）
type openAIProvider struct {
	apiKey    string
	baseURL   string
	model     string
	maxTokens int
	timeout   int
}

func (p *openAIProvider) Name() string  { return "openai" }
func (p *openAIProvider) Model() string { return p.model }

// ——— 请求/响应结构 ———

type openAIMessage struct {
	Role             string           `json:"role"`
	Content          interface{}      `json:"content"` // string 或 nil
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	Name             string           `json:"name,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Parameters  interface{} `json:"parameters"`
	} `json:"function"`
}

type openAIRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	Tools     []openAITool    `json:"tools,omitempty"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	Stream    bool            `json:"stream"`
}

type openAIResponse struct {
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// SSE chunk
type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// ——— 转换 ———

func (p *openAIProvider) convertMessages(messages []Message) []openAIMessage {
	var out []openAIMessage
	for _, m := range messages {
		om := openAIMessage{
			Role:             m.Role,
			ReasoningContent: m.ReasoningContent,
			ToolCallID:       m.ToolCallID,
			Name:             m.Name,
		}
		content := m.Content
		// tool 角色的消息内容不能为空，空字符串会导致部分 API（如 Anthropic 兼容层）报错
		if m.Role == "tool" && strings.TrimSpace(content) == "" {
			content = "执行成功（命令无输出）"
		}
		if content != "" {
			om.Content = content
		}
		for _, tc := range m.ToolCalls {
			otc := openAIToolCall{ID: tc.ID, Type: "function"}
			otc.Function.Name = tc.Name
			otc.Function.Arguments = tc.Arguments
			om.ToolCalls = append(om.ToolCalls, otc)
		}
		out = append(out, om)
	}
	return out
}

func (p *openAIProvider) convertTools(tools []ToolDef) []openAITool {
	var out []openAITool
	for _, t := range tools {
		ot := openAITool{Type: "function"}
		ot.Function.Name = t.Name
		ot.Function.Description = t.Description
		ot.Function.Parameters = t.Parameters
		out = append(out, ot)
	}
	return out
}

func parseToolCalls(raw []openAIToolCall) []ToolCall {
	var out []ToolCall
	for _, tc := range raw {
		out = append(out, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return out
}

// ——— HTTP 辅助 ———

func (p *openAIProvider) doRequest(ctx context.Context, req openAIRequest) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.baseURL, "/")+"/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	client := &http.Client{Timeout: time.Duration(p.timeout) * time.Second}
	return client.Do(httpReq)
}

// ——— Chat（非流式）———

func (p *openAIProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*ChatResponse, error) {
	req := openAIRequest{
		Model:     p.model,
		Messages:  p.convertMessages(messages),
		Tools:     p.convertTools(tools),
		MaxTokens: p.maxTokens,
		Stream:    false,
	}

	resp, err := p.doRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var apiResp openAIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("API 错误: %s", apiResp.Error.Message)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("API 返回空 choices")
	}

	msg := apiResp.Choices[0].Message
	content := ""
	if s, ok := msg.Content.(string); ok {
		content = s
	}
	return &ChatResponse{
		Content:          content,
		ReasoningContent: msg.ReasoningContent,
		ToolCalls:        parseToolCalls(msg.ToolCalls),
	}, nil
}

// ——— Stream（流式 SSE）———

func (p *openAIProvider) Stream(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	req := openAIRequest{
		Model:     p.model,
		Messages:  p.convertMessages(messages),
		Tools:     p.convertTools(tools),
		MaxTokens: p.maxTokens,
		Stream:    true,
	}

	// 流式请求不设总超时，用 context 控制
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.baseURL, "/")+"/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	client := &http.Client{} // 无超时，由 ctx 控制
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("流式请求失败: %v", err)
	}
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		// 尝试解析 error 字段
		var errResp openAIResponse
		if json.Unmarshal(data, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("API 错误 (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		p.parseSSEStream(resp.Body, ch)
	}()
	return ch, nil
}

func (p *openAIProvider) parseSSEStream(body io.Reader, ch chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	// tool_calls 按 index 累积参数
	type tcAccum struct {
		id        string
		name      string
		arguments strings.Builder
	}
	accum := make(map[int]*tcAccum)

	var reasoningBuf strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk openAIChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		// 推理内容（DeepSeek 等思考模式）
		if delta.ReasoningContent != "" {
			reasoningBuf.WriteString(delta.ReasoningContent)
		}

		// 文本内容
		if delta.Content != "" {
			ch <- StreamEvent{Type: "text", Text: delta.Content}
		}

		// 工具调用 chunk
		for _, tc := range delta.ToolCalls {
			if _, ok := accum[tc.Index]; !ok {
				accum[tc.Index] = &tcAccum{}
			}
			a := accum[tc.Index]
			if tc.ID != "" {
				a.id = tc.ID
			}
			if tc.Function.Name != "" {
				a.name = tc.Function.Name
			}
			a.arguments.WriteString(tc.Function.Arguments)
		}

		// finish_reason == "tool_calls" 时发射工具调用事件
		if chunk.Choices[0].FinishReason == "tool_calls" {
			var toolCalls []ToolCall
			for i := 0; i < len(accum); i++ {
				a, ok := accum[i]
				if !ok {
					continue
				}
				toolCalls = append(toolCalls, ToolCall{
					ID:        a.id,
					Name:      a.name,
					Arguments: a.arguments.String(),
				})
			}
			if len(toolCalls) > 0 {
				ch <- StreamEvent{Type: "tool_calls", ToolCalls: toolCalls}
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		ch <- StreamEvent{Type: "error", Err: fmt.Errorf("读取流失败: %v", err)}
	}
	ch <- StreamEvent{Type: "done", ReasoningContent: reasoningBuf.String()}
}
