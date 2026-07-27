package agent

import "testing"

func TestScheduledTaskWritePermissionBoundary(t *testing.T) {
	tests := []struct {
		name string
		call ToolCall
		want bool
	}{
		{name: "只读状态工具", call: ToolCall{Name: "get_status", Arguments: `{}`}, want: false},
		{name: "服务写操作", call: ToolCall{Name: "service_control", Arguments: `{"action":"restart"}`}, want: true},
		{name: "只读数据库查询", call: ToolCall{Name: "database_query", Arguments: `{"sql":"SELECT 1"}`}, want: false},
		{name: "数据库写操作", call: ToolCall{Name: "database_query", Arguments: `{"sql":"DELETE FROM users"}`}, want: true},
		{name: "查看定时任务", call: ToolCall{Name: "scheduled_tasks", Arguments: `{"action":"list"}`}, want: false},
		{name: "创建定时任务", call: ToolCall{Name: "scheduled_tasks", Arguments: `{"action":"create"}`}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := scheduledToolNeedsConfirmation(test.call); got != test.want {
				t.Fatalf("权限判断=%v，期望=%v", got, test.want)
			}
		})
	}
}
