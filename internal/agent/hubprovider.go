package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type hubProvider struct {
	hubURL  string
	token   string
	model   string
	timeout int
}

func (h *hubProvider) Name() string { return "hub" }
func (h *hubProvider) Model() string {
	if h.model != "" {
		return h.model
	}
	return "hub-relay"
}

type hubChatRequest struct {
	Messages []Message `json:"messages"`
	Tools    []ToolDef `json:"tools,omitempty"`
}

type hubChatResponse struct {
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	Error            string     `json:"error,omitempty"`
}

func (h *hubProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*ChatResponse, error) {
	body, err := json.Marshal(hubChatRequest{Messages: messages, Tools: tools})
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(h.hubURL, "/") + "/__hub__/v1/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.token)

	client := &http.Client{Timeout: time.Duration(h.timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Hub 请求失败: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var hubResp hubChatResponse
	if err := json.Unmarshal(data, &hubResp); err != nil {
		return nil, fmt.Errorf("解析 Hub 响应失败: %v", err)
	}
	if hubResp.Error != "" {
		return nil, fmt.Errorf("Hub 转发错误: %s", hubResp.Error)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Hub HTTP %d: %s", resp.StatusCode, hubResp.Error)
	}
	return &ChatResponse{
		Content:          hubResp.Content,
		ReasoningContent: hubResp.ReasoningContent,
		ToolCalls:        hubResp.ToolCalls,
	}, nil
}

func (h *hubProvider) Stream(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 8)
	go func() {
		defer close(ch)
		resp, err := h.Chat(ctx, messages, tools)
		if err != nil {
			ch <- StreamEvent{Type: "error", Err: err}
			return
		}
		if resp.Content != "" {
			ch <- StreamEvent{Type: "text", Text: resp.Content}
		}
		if len(resp.ToolCalls) > 0 {
			ch <- StreamEvent{Type: "tool_calls", ToolCalls: resp.ToolCalls}
		}
		ch <- StreamEvent{Type: "done", ReasoningContent: resp.ReasoningContent}
	}()
	return ch, nil
}
