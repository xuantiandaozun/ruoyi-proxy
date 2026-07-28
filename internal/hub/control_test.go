package hub

import (
	"os"
	"testing"
	"time"
)

func TestControlJobLifecycle(t *testing.T) {
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	defaultStore = &spokeStore{spokes: map[string]*SpokeRecord{
		"spoke-test": {ID: "spoke-test", LastSeen: time.Now()},
	}}
	defaultControlStore = &controlJobStore{jobs: make(map[string]*ControlJob)}

	created, err := EnqueueControlJob("spoke-test", "echo ok", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "pending" {
		t.Fatalf("新任务状态 = %s", created.Status)
	}
	claimed, err := ClaimControlJob("spoke-test")
	if err != nil || claimed == nil {
		t.Fatalf("领取任务失败: job=%v err=%v", claimed, err)
	}
	if claimed.Status != ControlJobClaimed {
		t.Fatalf("领取后状态 = %s", claimed.Status)
	}
	if err := StartControlJob("spoke-test", claimed.ID); err != nil {
		t.Fatal(err)
	}
	if err := CompleteControlJob("spoke-test", claimed.ID, "succeeded", "ok", ""); err != nil {
		t.Fatal(err)
	}
	items, err := ListControlJobs("spoke-test", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != "succeeded" || items[0].Output != "ok" {
		t.Fatalf("任务结果异常: %#v", items)
	}
}

func TestMultipleRegisterTokensCanCoexist(t *testing.T) {
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	defaultStore = &spokeStore{
		spokes:        make(map[string]*SpokeRecord),
		pendingTokens: make(map[string]time.Time),
	}
	first, err := GenerateRegisterToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateRegisterToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("两个注册 Token 不应相同")
	}
	if !consumeRegisterToken(first) || !consumeRegisterToken(second) {
		t.Fatal("并存的注册 Token 应可分别消费")
	}
	if consumeRegisterToken(first) {
		t.Fatal("一次性 Token 不应重复消费")
	}
}
