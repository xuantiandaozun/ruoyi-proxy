package agent

import (
	"encoding/json"
	"fmt"
	"os"
)

const appConfigFile = "configs/app_config.json"

// AIConfig LLM 提供商配置
type AIConfig struct {
	Provider       string `json:"provider"`        // openai | anthropic | ollama
	APIKey         string `json:"api_key"`         // API 密钥（ollama 可留空）
	BaseURL        string `json:"base_url"`        // 覆盖默认 endpoint
	Model          string `json:"model"`           // 模型名称
	MaxTokens      int    `json:"max_tokens"`      // 单次最大生成 token 数
	ContextLimit   int    `json:"context_limit"`   // 保留的历史 token 上限
	TimeoutSeconds int    `json:"timeout_seconds"` // HTTP 超时
	SystemPrompt   string `json:"system_prompt"`   // 留空则用默认提示词
}

// DefaultAIConfig 返回各 provider 的默认配置模板
func DefaultAIConfig(provider string) AIConfig {
	cfg := AIConfig{
		Provider:       provider,
		MaxTokens:      4096,
		ContextLimit:   24000,
		TimeoutSeconds: 60,
	}
	switch provider {
	case "anthropic":
		cfg.Model = "claude-sonnet-4-5"
		cfg.BaseURL = "https://api.anthropic.com"
	case "ollama":
		cfg.Model = "qwen2.5:7b"
		cfg.BaseURL = "http://localhost:11434/v1"
		cfg.APIKey = "ollama"
	default: // openai
		cfg.Provider = "openai"
		cfg.Model = "gpt-4o-mini"
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	return cfg
}

// LoadAIConfig 从 app_config.json 读取 AI 配置，文件不存在或无 ai 字段时返回零值
func LoadAIConfig() (AIConfig, error) {
	data, err := os.ReadFile(appConfigFile)
	if err != nil {
		return AIConfig{}, nil // 文件不存在视为未配置
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return AIConfig{}, fmt.Errorf("解析配置文件失败: %v", err)
	}

	raw, ok := root["ai"]
	if !ok {
		return AIConfig{}, nil
	}

	var cfg AIConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return AIConfig{}, fmt.Errorf("解析 AI 配置失败: %v", err)
	}
	return cfg, nil
}

// SaveAIConfig 将 AI 配置写回 app_config.json（保留其他字段）
func SaveAIConfig(cfg AIConfig) error {
	// 读现有内容
	var root map[string]json.RawMessage
	if data, err := os.ReadFile(appConfigFile); err == nil {
		_ = json.Unmarshal(data, &root)
	}
	if root == nil {
		root = make(map[string]json.RawMessage)
	}

	aiBytes, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化 AI 配置失败: %v", err)
	}
	root["ai"] = aiBytes

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置文件失败: %v", err)
	}

	if err := os.MkdirAll("configs", 0755); err != nil {
		return err
	}
	return os.WriteFile(appConfigFile, out, 0644)
}

// IsConfigured 判断 AI 配置是否足够可用
func (c AIConfig) IsConfigured() bool {
	if c.Provider == "" || c.Model == "" {
		return false
	}
	// ollama 不需要 API Key
	if c.Provider == "ollama" {
		return true
	}
	return c.APIKey != ""
}

// MaskedKey 返回脱敏后的 API Key（用于显示）
func (c AIConfig) MaskedKey() string {
	if len(c.APIKey) <= 8 {
		return "****"
	}
	return c.APIKey[:4] + "****" + c.APIKey[len(c.APIKey)-4:]
}

// EffectiveBaseURL 返回实际使用的 BaseURL
func (c AIConfig) EffectiveBaseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	switch c.Provider {
	case "anthropic":
		return "https://api.anthropic.com"
	case "ollama":
		return "http://localhost:11434/v1"
	default:
		return "https://api.openai.com/v1"
	}
}
