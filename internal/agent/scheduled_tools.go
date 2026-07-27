package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ruoyi-proxy/internal/scheduler"
)

var scheduledTaskToolDefinition = ToolDef{
	Name:        "scheduled_tasks",
	Description: "管理由本机 SQLite 持久化的 AI 定时任务。list/history 为只读；create/update/enable/disable/delete/run_now 需要用户确认。默认任务只允许只读工具，allow_write=true 表示用户明确预授权无人值守写操作",
	ReadOnly:    false,
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type": "string",
				"enum": []string{"list", "create", "update", "enable", "disable", "delete", "run_now", "history"},
			},
			"id":         map[string]interface{}{"type": "integer", "description": "任务 ID，更新、状态操作和查询单任务历史时使用"},
			"name":       map[string]interface{}{"type": "string", "description": "用户可识别的任务名称"},
			"service_id": map[string]interface{}{"type": "string", "description": "任务操作的本机服务 ID，默认 default"},
			"prompt":     map[string]interface{}{"type": "string", "description": "每次触发时交给 AI 完成的完整指令"},
			"schedule": map[string]interface{}{
				"type":        "string",
				"description": "五段 Cron，或 @every 10m、@daily、@at 2026-08-01T02:00:00+08:00",
			},
			"timezone":        map[string]interface{}{"type": "string", "description": "IANA 时区，默认 Local，例如 Asia/Shanghai"},
			"allow_write":     map[string]interface{}{"type": "boolean", "description": "是否预授权后台执行写工具，默认 false"},
			"enabled":         map[string]interface{}{"type": "boolean", "description": "创建后是否立即启用，默认 true"},
			"timeout_seconds": map[string]interface{}{"type": "integer", "description": "单次任务超时，默认 600，最大 3600"},
			"limit":           map[string]interface{}{"type": "integer", "description": "history 返回条数，默认 20，最大 100"},
		},
		"required": []string{"action"},
	},
}

func init() {
	AllTools = append(AllTools, scheduledTaskToolDefinition)
}

func (e *ToolExecutor) scheduledTasks(args map[string]interface{}) (string, error) {
	store, err := scheduler.Open()
	if err != nil {
		return "", err
	}
	defer store.Close()
	action, _ := args["action"].(string)
	id := int64Number(args["id"])
	switch action {
	case "list":
		tasks, err := store.ListTasks()
		if err != nil {
			return "", err
		}
		if len(tasks) == 0 {
			return "尚未创建 AI 定时任务", nil
		}
		var lines []string
		for _, task := range tasks {
			state := "已暂停"
			if task.Enabled {
				state = "已启用"
			}
			permission := "只读"
			if task.AllowWrite {
				permission = "允许写操作"
			}
			next := "-"
			if task.NextRunAt != nil {
				next = task.NextRunAt.Local().Format("2006-01-02 15:04:05")
			}
			lines = append(lines, fmt.Sprintf("[%d] %s[%s] | %s | %s | %s | 下次: %s | 上次: %s",
				task.ID, task.Name, task.ServiceID, state, permission, task.Schedule, next, emptyDash(task.LastStatus)))
		}
		return strings.Join(lines, "\n"), nil
	case "create", "update":
		input, err := scheduledTaskInput(store, action, id, args)
		if err != nil {
			return "", err
		}
		task, err := store.SaveTask(input)
		if err != nil {
			return "", err
		}
		return formatScheduledTaskSaved(task), nil
	case "enable", "disable":
		if id <= 0 {
			return "", fmt.Errorf("请提供任务 id")
		}
		task, err := store.SetEnabled(id, action == "enable")
		if err != nil {
			return "", err
		}
		return formatScheduledTaskSaved(task), nil
	case "delete":
		if id <= 0 {
			return "", fmt.Errorf("请提供任务 id")
		}
		if err := store.DeleteTask(id); err != nil {
			return "", err
		}
		return fmt.Sprintf("已删除定时任务 %d；历史执行记录仍保留用于审计", id), nil
	case "run_now":
		if id <= 0 {
			return "", fmt.Errorf("请提供任务 id")
		}
		if err := store.QueueNow(id); err != nil {
			return "", err
		}
		return fmt.Sprintf("任务 %d 已进入立即执行队列", id), nil
	case "history":
		limit := int(int64Number(args["limit"]))
		runs, err := store.ListRuns(id, limit)
		if err != nil {
			return "", err
		}
		if len(runs) == 0 {
			return "暂无执行历史", nil
		}
		var lines []string
		for _, run := range runs {
			detail := run.Error
			if detail == "" {
				detail = truncateOutput(strings.TrimSpace(run.Output), 500)
			}
			lines = append(lines, fmt.Sprintf("运行#%d 任务[%d:%s] %s %s\n%s",
				run.ID, run.TaskID, run.TaskName, run.Status,
				run.StartedAt.Local().Format("2006-01-02 15:04:05"), emptyDash(detail)))
		}
		return strings.Join(lines, "\n\n"), nil
	default:
		return "", fmt.Errorf("不支持的定时任务操作: %s", action)
	}
}

func scheduledTaskInput(store *scheduler.Store, action string, id int64, args map[string]interface{}) (scheduler.TaskInput, error) {
	input := scheduler.TaskInput{Enabled: true}
	if action == "update" {
		if id <= 0 {
			return input, fmt.Errorf("更新任务时必须提供 id")
		}
		existing, err := store.GetTask(id)
		if err != nil {
			return input, err
		}
		input = scheduler.TaskInput{
			ID: existing.ID, Name: existing.Name, ServiceID: existing.ServiceID, Prompt: existing.Prompt,
			Schedule: existing.Schedule, Timezone: existing.Timezone,
			AllowWrite: existing.AllowWrite, Enabled: existing.Enabled,
			TimeoutSecs: existing.TimeoutSecs,
		}
	}
	if value, ok := args["name"].(string); ok {
		input.Name = value
	}
	if value, ok := args["service_id"].(string); ok {
		input.ServiceID = value
	}
	if value, ok := args["prompt"].(string); ok {
		input.Prompt = value
	}
	if value, ok := args["schedule"].(string); ok {
		input.Schedule = value
	}
	if value, ok := args["timezone"].(string); ok {
		input.Timezone = value
	}
	if value, ok := args["allow_write"].(bool); ok {
		input.AllowWrite = value
	}
	if value, ok := args["enabled"].(bool); ok {
		input.Enabled = value
	}
	if value := int(int64Number(args["timeout_seconds"])); value > 0 {
		input.TimeoutSecs = value
	}
	return input, nil
}

func formatScheduledTaskSaved(task scheduler.Task) string {
	next := "-"
	if task.NextRunAt != nil {
		next = task.NextRunAt.Local().Format(time.DateTime)
	}
	permission := "只读工具"
	if task.AllowWrite {
		permission = "已预授权写操作"
	}
	return fmt.Sprintf("定时任务已保存: [%d] %s\n调度: %s (%s)\n权限: %s\n下次执行: %s",
		task.ID, task.Name, task.Schedule, task.Timezone, permission, next)
}

func int64Number(value interface{}) int64 {
	switch number := value.(type) {
	case float64:
		return int64(number)
	case int64:
		return number
	case json.Number:
		parsed, _ := number.Int64()
		return parsed
	default:
		return 0
	}
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func isReadOnlyScheduledTaskCall(argsJSON string) bool {
	var args struct {
		Action string `json:"action"`
	}
	if json.Unmarshal([]byte(argsJSON), &args) != nil {
		return false
	}
	return args.Action == "list" || args.Action == "history"
}
