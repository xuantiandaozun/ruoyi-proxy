package agent

import (
	"strings"
	"testing"
)

func TestVisibleSessionMessagesExcludeInternalLayers(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "检查服务"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "1", Name: "get_status"}}},
		{Role: "tool", Name: "get_status", Content: "ok"},
		{Role: "assistant", Content: "服务正常"},
		{Role: "user", Content: autoResumePrompt},
	}

	if got := countVisibleMessages(messages); got != 2 {
		t.Fatalf("可见消息数量 = %d，期望 2", got)
	}
	transcript := formatSessionTranscript(messages)
	if strings.Contains(transcript, "get_status") || strings.Contains(transcript, autoResumePrompt) {
		t.Fatalf("历史回放泄漏内部消息: %q", transcript)
	}
	if !strings.Contains(transcript, "检查服务") || !strings.Contains(transcript, "服务正常") {
		t.Fatalf("历史回放缺少真实对话: %q", transcript)
	}
}

func TestRedactDatabasePasswordFromSession(t *testing.T) {
	messages := []Message{{Role: "assistant", ToolCalls: []ToolCall{{ID: "1", Name: "database_save_connection", Arguments: `{"project_name":"订单","host":"db.example.com","password":"very-secret"}`}}}}
	redacted := redactSessionSecrets(messages)
	if strings.Contains(redacted[0].ToolCalls[0].Arguments, "very-secret") {
		t.Fatal("数据库密码被写入会话")
	}
	if !strings.Contains(redacted[0].ToolCalls[0].Arguments, "已保存") {
		t.Fatal("会话中缺少密码脱敏标记")
	}
	if !strings.Contains(messages[0].ToolCalls[0].Arguments, "very-secret") {
		t.Fatal("脱敏不应修改运行中的上下文")
	}
}

func TestFormatArgsMasksSecrets(t *testing.T) {
	formatted := formatArgs(`{"host":"db.example.com","password":"secret","token":"abc"}`)
	if strings.Contains(formatted, "secret") || strings.Contains(formatted, "abc") {
		t.Fatalf("确认信息泄漏密钥: %s", formatted)
	}
	if !strings.Contains(formatted, "db.example.com") {
		t.Fatalf("非敏感参数不应隐藏: %s", formatted)
	}
}
