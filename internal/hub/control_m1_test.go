package hub

import (
	"os"
	"testing"
	"time"
)

func prepareControlJobTest(t *testing.T) {
	t.Helper()
	useTemporaryHubStore(t)
	defaultStore.spokes["spoke-test"] = &SpokeRecord{
		ID:       "spoke-test",
		LastSeen: time.Now(),
	}
}

func TestControlJobIdempotentEnqueue(t *testing.T) {
	prepareControlJobTest(t)
	options := ControlJobOptions{IdempotencyKey: "deploy-release-42", MaxAttempts: 2}
	first, err := EnqueueControlJobWithOptions("spoke-test", "echo ok", "", 10, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EnqueueControlJobWithOptions("spoke-test", "echo ok", "", 10, options)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("相同幂等键创建了不同任务: %s != %s", first.ID, second.ID)
	}
	items, err := ListControlJobs("spoke-test", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("幂等创建后的任务数 = %d", len(items))
	}
	if _, err := EnqueueControlJobWithOptions("spoke-test", "echo changed", "", 10, options); err == nil {
		t.Fatal("不同任务复用幂等键应被拒绝")
	}
}

func TestLegacySpokeCanCompleteClaimedJob(t *testing.T) {
	prepareControlJobTest(t)
	created, err := EnqueueControlJob("spoke-test", "echo legacy", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := ClaimControlJob("spoke-test")
	if err != nil || claimed == nil {
		t.Fatalf("领取失败: job=%v err=%v", claimed, err)
	}
	if err := CompleteControlJob("spoke-test", created.ID, ControlJobSucceeded, "legacy-ok", ""); err != nil {
		t.Fatalf("旧 Spoke 直接回传终态失败: %v", err)
	}
}

func TestExpiredControlLeaseRetriesThenExhausts(t *testing.T) {
	prepareControlJobTest(t)
	created, err := EnqueueControlJobWithOptions(
		"spoke-test", "echo retry", "", 10, ControlJobOptions{MaxAttempts: 2},
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ClaimControlJob("spoke-test")
	if err != nil || first == nil {
		t.Fatalf("首次领取失败: job=%v err=%v", first, err)
	}
	defaultControlStore.jobs[created.ID].ClaimedUntil = time.Now().Add(-time.Second)
	second, err := ClaimControlJob("spoke-test")
	if err != nil || second == nil {
		t.Fatalf("租约过期后未重新领取: job=%v err=%v", second, err)
	}
	if second.ID != created.ID || second.Attempt != 2 {
		t.Fatalf("重领任务异常: %#v", second)
	}
	defaultControlStore.jobs[created.ID].ClaimedUntil = time.Now().Add(-time.Second)
	items, err := ListControlJobs("spoke-test", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != ControlJobFailed {
		t.Fatalf("达到最大尝试次数后状态异常: %#v", items)
	}
}

func TestCancelAndRetryControlJob(t *testing.T) {
	prepareControlJobTest(t)
	created, err := EnqueueControlJob("spoke-test", "echo later", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	canceled, err := CancelControlJob(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if canceled.Status != ControlJobCanceled || canceled.FinishedAt.IsZero() {
		t.Fatalf("取消结果异常: %#v", canceled)
	}
	retried, err := RetryControlJob(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != ControlJobPending || !retried.FinishedAt.IsZero() {
		t.Fatalf("重试结果异常: %#v", retried)
	}
}

func TestLegacyRunningControlJobFileMigration(t *testing.T) {
	useTemporaryHubStore(t)
	legacy := `[{"id":"job-old","spoke_id":"spoke-test","command":"echo old","timeout_seconds":60,"status":"running","created_at":"2025-01-01T00:00:00Z","started_at":"2025-01-01T00:00:01Z"}]`
	if err := os.MkdirAll("configs", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(controlJobsFile, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := ListControlJobs("", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != ControlJobPending ||
		items[0].MaxAttempts != defaultControlMaxAttempts || !items[0].StartedAt.IsZero() {
		t.Fatalf("旧任务迁移异常: %#v", items)
	}
}
