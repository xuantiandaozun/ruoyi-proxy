package hub

import (
	"testing"
	"time"
)

func TestStructuredControlActionAndAuditEvents(t *testing.T) {
	prepareControlJobTest(t)
	defaultStore.spokes["spoke-test"].Profile = &SpokeProfile{Capabilities: []string{"service.status"}}
	confirmedAt := time.Now().Add(-time.Second)
	action := &ControlAction{
		Type: ControlActionStatus,
		Params: map[string]string{
			"service_id": "api",
		},
	}
	created, err := EnqueueControlJobWithOptions(
		"spoke-test",
		"",
		"",
		30,
		ControlJobOptions{
			Action:      action,
			Actor:       "operator-1",
			Source:      "cli",
			ConfirmedBy: "operator-1",
			ConfirmedAt: confirmedAt,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.Command != "" || created.Action == nil || created.Action.Type != ControlActionStatus {
		t.Fatalf("结构化任务异常: %#v", created)
	}
	if len(created.Events) != 2 ||
		created.Events[0].Type != "confirmed" ||
		created.Events[1].Type != "created" {
		t.Fatalf("创建审计事件异常: %#v", created.Events)
	}

	claimed, err := ClaimControlJobWithVersion("spoke-test", 2)
	if err != nil || claimed == nil {
		t.Fatalf("领取失败: job=%v err=%v", claimed, err)
	}
	if err := StartControlJob("spoke-test", claimed.ID); err != nil {
		t.Fatal(err)
	}
	if err := CompleteControlJob("spoke-test", claimed.ID, ControlJobSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}
	items, err := ListControlJobs("spoke-test", 20)
	if err != nil {
		t.Fatal(err)
	}
	events := items[0].Events
	if len(events) != 5 {
		t.Fatalf("完整生命周期事件数 = %d, events=%#v", len(events), events)
	}
	for index, event := range events {
		if event.Sequence != index+1 {
			t.Fatalf("事件序号不连续: %#v", events)
		}
	}
}

func TestLegacyWorkerDoesNotClaimStructuredAction(t *testing.T) {
	prepareControlJobTest(t)
	defaultStore.spokes["spoke-test"].Profile = &SpokeProfile{Capabilities: []string{"service.status"}}
	_, err := EnqueueControlJobWithOptions(
		"spoke-test", "", "", 10,
		ControlJobOptions{Action: &ControlAction{Type: ControlActionStatus}},
	)
	if err != nil {
		t.Fatal(err)
	}
	legacyJob, err := ClaimControlJob("spoke-test")
	if err != nil {
		t.Fatal(err)
	}
	if legacyJob != nil {
		t.Fatalf("旧 Worker 不应领取结构化动作: %#v", legacyJob)
	}
	newJob, err := ClaimControlJobWithVersion("spoke-test", 2)
	if err != nil || newJob == nil {
		t.Fatalf("新 Worker 应能领取结构化动作: job=%#v err=%v", newJob, err)
	}
}
func TestControlActionValidation(t *testing.T) {
	tests := []struct {
		name    string
		action  ControlAction
		wantErr bool
	}{
		{name: "status default service", action: ControlAction{Type: ControlActionStatus}},
		{name: "logs bounded", action: ControlAction{Type: ControlActionLogs, Params: map[string]string{"lines": "200"}}},
		{name: "logs too many", action: ControlAction{Type: ControlActionLogs, Params: map[string]string{"lines": "1001"}}, wantErr: true},
		{name: "invalid service", action: ControlAction{Type: ControlActionRestart, Params: map[string]string{"service_id": "api;rm"}}, wantErr: true},
		{name: "database missing sql", action: ControlAction{Type: ControlActionDatabaseQuery, Params: map[string]string{"profile": "prod"}}, wantErr: true},
		{name: "unsupported", action: ControlAction{Type: "unknown"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.action.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() err=%v wantErr=%v", err, test.wantErr)
			}
		})
	}
}

func TestControlJobBatchCreatesIndependentTargets(t *testing.T) {
	prepareControlJobTest(t)
	defaultStore.spokes["spoke-test"].Profile = &SpokeProfile{Capabilities: []string{"service.status"}}
	defaultStore.spokes["spoke-second"] = &SpokeRecord{
		ID:       "spoke-second",
		LastSeen: time.Now(),
		Profile:  &SpokeProfile{Capabilities: []string{"service.status"}},
	}
	options := ControlJobOptions{
		Action:         &ControlAction{Type: ControlActionStatus},
		IdempotencyKey: "fleet-status-42",
		Source:         "test",
	}
	first, err := EnqueueControlJobBatch(
		[]string{"spoke-second", "spoke-test", "spoke-test"}, "", "", 10, options,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Jobs) != 2 || first.Jobs[0].BatchID != first.ID || first.Jobs[1].BatchID != first.ID {
		t.Fatalf("批量任务异常: %#v", first)
	}
	if first.Jobs[0].ID == first.Jobs[1].ID || first.Jobs[0].SpokeID == first.Jobs[1].SpokeID {
		t.Fatalf("目标任务未独立创建: %#v", first.Jobs)
	}
	second, err := EnqueueControlJobBatch(
		[]string{"spoke-test", "spoke-second"}, "", "", 10, options,
	)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID ||
		second.Jobs[0].ID != first.Jobs[0].ID ||
		second.Jobs[1].ID != first.Jobs[1].ID {
		t.Fatalf("批量幂等结果异常: first=%#v second=%#v", first, second)
	}
}
