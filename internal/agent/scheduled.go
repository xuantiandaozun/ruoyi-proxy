package agent

import (
	"context"
	"fmt"
	"strings"
)

// RunScheduledTask 在无交互终端的上下文中执行一条 AI 任务。
// allowWrite=false 时，所有原本需要弹窗确认的工具都会被拒绝。
func RunScheduledTask(
	ctx context.Context,
	aiCfg AIConfig,
	execCtx ExecContext,
	prompt string,
	allowWrite bool,
) (string, error) {
	if !aiCfg.IsConfigured() {
		return "", fmt.Errorf("AI 未配置，无法执行定时任务")
	}
	provider, err := NewProvider(aiCfg)
	if err != nil {
		return "", fmt.Errorf("创建 AI Provider 失败: %v", err)
	}
	template := &Agent{aiCfg: aiCfg, execCtx: execCtx}
	systemPrompt := aiCfg.SystemPrompt
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = template.defaultSystemPrompt()
	}
	policy := "本次为无人值守定时任务。不要向用户提问；信息不足时停止并明确记录缺少的信息。"
	if allowWrite {
		policy += " 用户已在创建任务时预先授权执行必要的写操作，但仍需严格限制在任务指令范围内。"
	} else {
		policy += " 本任务仅授权只读操作，不得尝试修改文件、服务、数据库或系统状态。"
	}
	messages := []Message{
		{Role: "system", Content: systemPrompt + "\n\n## 定时任务执行策略\n" + policy},
		{Role: "user", Content: strings.TrimSpace(prompt)},
	}
	executor := NewToolExecutor(execCtx)
	var finalText string
	var audit []string
	for iteration := 0; iteration < maxReActIterations; iteration++ {
		if err := ctx.Err(); err != nil {
			return strings.Join(audit, "\n"), fmt.Errorf("定时任务已取消: %v", err)
		}
		response, err := provider.Chat(ctx, messages, AllTools)
		if err != nil {
			return strings.Join(audit, "\n"), fmt.Errorf("调用 AI 失败: %v", err)
		}
		messages = append(messages, Message{
			Role: "assistant", Content: response.Content,
			ReasoningContent: response.ReasoningContent, ToolCalls: response.ToolCalls,
		})
		if strings.TrimSpace(response.Content) != "" {
			finalText = strings.TrimSpace(response.Content)
		}
		if len(response.ToolCalls) == 0 {
			if finalText == "" {
				finalText = "任务执行完成（AI 未返回文字摘要）"
			}
			if len(audit) == 0 {
				return finalText, nil
			}
			return finalText + "\n\n执行步骤:\n" + strings.Join(audit, "\n"), nil
		}
		for _, call := range response.ToolCalls {
			content := ""
			if scheduledToolNeedsConfirmation(call) && !allowWrite {
				content = "执行失败: 此定时任务未预先授权写操作"
				audit = append(audit, fmt.Sprintf("- %s: 已拦截未授权写操作", call.Name))
			} else {
				result, runErr := executor.Execute(call.Name, call.Arguments)
				if runErr != nil {
					content = fmt.Sprintf("执行失败: %v", runErr)
					audit = append(audit, fmt.Sprintf("- %s: 失败（%v）", call.Name, runErr))
				} else {
					content = truncateOutput(result, toolOutputMaxChars)
					audit = append(audit, fmt.Sprintf("- %s: 成功", call.Name))
				}
			}
			if strings.TrimSpace(content) == "" {
				content = "执行成功（命令无输出）"
			}
			messages = append(messages, Message{
				Role: "tool", ToolCallID: call.ID, Name: call.Name, Content: content,
			})
		}
	}
	return strings.Join(audit, "\n"), fmt.Errorf("定时任务超过最大 AI 推理轮数（%d）", maxReActIterations)
}

func scheduledToolNeedsConfirmation(call ToolCall) bool {
	var definition *ToolDef
	for i := range AllTools {
		if AllTools[i].Name == call.Name {
			definition = &AllTools[i]
			break
		}
	}
	needsConfirm := definition != nil && !definition.ReadOnly
	if needsConfirm && call.Name == "run_shell" && isReadOnlyShellCmd(call.Arguments) {
		return false
	}
	if needsConfirm && call.Name == "database_query" && isReadOnlyDatabaseToolCall(call.Arguments) {
		return false
	}
	if needsConfirm && call.Name == "database_connections" && isReadOnlyDatabaseConnectionsCall(call.Arguments) {
		return false
	}
	if needsConfirm && call.Name == "scheduled_tasks" && isReadOnlyScheduledTaskCall(call.Arguments) {
		return false
	}
	return needsConfirm
}
