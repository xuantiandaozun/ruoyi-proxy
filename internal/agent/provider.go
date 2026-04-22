package agent

import (
	"context"
	"fmt"
)

// Provider 统一 LLM 接口，各实现负责转换自己的 API 格式
type Provider interface {
	Name() string
	Model() string
	// Chat 非流式，用于工具结果回传后继续推理
	Chat(ctx context.Context, messages []Message, tools []ToolDef) (*ChatResponse, error)
	// Stream 流式，首轮及后续推理使用，边生成边打印
	Stream(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
}

// NewProvider 根据 AIConfig 构造对应 Provider
func NewProvider(cfg AIConfig) (Provider, error) {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 60
	}
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	switch cfg.Provider {
	case "anthropic":
		return &anthropicProvider{
			apiKey:    cfg.APIKey,
			baseURL:   cfg.EffectiveBaseURL(),
			model:     cfg.Model,
			maxTokens: maxTokens,
			timeout:   timeout,
		}, nil

	case "openai", "ollama", "":
		apiKey := cfg.APIKey
		if cfg.Provider == "ollama" && apiKey == "" {
			apiKey = "ollama"
		}
		return &openAIProvider{
			apiKey:    apiKey,
			baseURL:   cfg.EffectiveBaseURL(),
			model:     cfg.Model,
			maxTokens: maxTokens,
			timeout:   timeout,
		}, nil

	default:
		return nil, fmt.Errorf("不支持的 provider: %s（支持 openai / anthropic / ollama）", cfg.Provider)
	}
}
