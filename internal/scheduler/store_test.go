package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreTaskLifecycle(t *testing.T) {
	store, err := OpenPath(filepath.Join(t.TempDir(), "scheduler.db"))
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	defer store.Close()
	task, err := store.SaveTask(TaskInput{
		Name: "每日巡检", ServiceID: "orders", Prompt: "检查服务状态", Schedule: "@every 1h",
		Timezone: "Local", Enabled: true, TimeoutSecs: 60,
	})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if task.ID == 0 || task.NextRunAt == nil || task.AllowWrite || task.ServiceID != "orders" {
		t.Fatalf("任务字段异常: %+v", task)
	}
	if err := store.QueueNow(task.ID); err != nil {
		t.Fatalf("立即运行入队失败: %v", err)
	}
	claimed, err := store.ClaimDue(context.Background(), time.Now().Add(time.Second))
	if err != nil || claimed == nil || claimed.ID != task.ID {
		t.Fatalf("领取任务异常: task=%+v err=%v", claimed, err)
	}
	second, err := store.ClaimDue(context.Background(), time.Now().Add(time.Second))
	if err != nil || second != nil {
		t.Fatalf("同一任务不应被重复领取: task=%+v err=%v", second, err)
	}
	run, err := store.StartRun(*claimed, "manual")
	if err != nil {
		t.Fatalf("创建执行记录失败: %v", err)
	}
	if err := store.FinishRun(*claimed, run.ID, "一切正常", nil); err != nil {
		t.Fatalf("完成执行记录失败: %v", err)
	}
	runs, err := store.ListRuns(task.ID, 10)
	if err != nil || len(runs) != 1 || runs[0].Status != "success" {
		t.Fatalf("执行历史异常: runs=%+v err=%v", runs, err)
	}
}

func TestNextTimeSupportsCronEveryAndAt(t *testing.T) {
	now := time.Now()
	if _, err := NextTime("0 2 * * *", "Local", now); err != nil {
		t.Fatalf("五段 Cron 应有效: %v", err)
	}
	if _, err := NextTime("@every 10m", "Local", now); err != nil {
		t.Fatalf("@every 应有效: %v", err)
	}
	at := now.Add(time.Hour).Format(time.RFC3339)
	if _, err := NextTime("@at "+at, "Local", now); err != nil {
		t.Fatalf("@at 应有效: %v", err)
	}
	if _, err := NextTime("* * *", "Local", now); err == nil {
		t.Fatal("无效 Cron 应返回错误")
	}
}
