package hub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSpokeGovernancePatchAndDeepCopy(t *testing.T) {
	prepareControlJobTest(t)
	alias := "订单生产节点"
	group := "production"
	tags := []string{"java", "critical", "java"}
	maintenance := true
	record, err := PatchSpokeGovernance("spoke-test", SpokeGovernancePatch{
		Alias:       &alias,
		Group:       &group,
		Tags:        &tags,
		Maintenance: &maintenance,
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Alias != alias || record.Group != group || len(record.Tags) != 2 || !record.Maintenance {
		t.Fatalf("治理字段异常: %#v", record)
	}
	if _, err := EnqueueControlJob("spoke-test", "uptime", "", 10); err == nil {
		t.Fatal("维护节点不应接受新任务")
	}

	if err := UpdateSpokeProfile("spoke-test", SpokeProfile{Hostname: "refreshed"}); err != nil {
		t.Fatal(err)
	}
	record.Tags[0] = "mutated"
	stored, ok := GetSpoke("spoke-test")
	if !ok || stored.Tags[0] == "mutated" || stored.Group != group || !stored.Maintenance {
		t.Fatalf("GetSpoke 未返回深拷贝: %#v", stored)
	}
}

func TestCapabilityWhitelistControlsStructuredActions(t *testing.T) {
	prepareControlJobTest(t)
	defaultStore.spokes["spoke-test"].Profile = &SpokeProfile{
		Capabilities: []string{"service.status", "service.logs"},
	}
	allowed := []string{"service.status"}
	if _, err := PatchSpokeGovernance("spoke-test", SpokeGovernancePatch{
		AllowedCapabilities: &allowed,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := EnqueueControlJobWithOptions(
		"spoke-test", "", "", 10,
		ControlJobOptions{Action: &ControlAction{Type: ControlActionStatus}},
	); err != nil {
		t.Fatalf("白名单内能力被拒绝: %v", err)
	}
	if _, err := EnqueueControlJobWithOptions(
		"spoke-test", "", "", 10,
		ControlJobOptions{Action: &ControlAction{Type: ControlActionLogs}},
	); err == nil {
		t.Fatal("白名单外能力应被拒绝")
	}
	job, err := ClaimControlJobWithCapabilities("spoke-test", 2, []string{"service.status"})
	if err != nil || job == nil {
		t.Fatalf("能力匹配任务未能领取: job=%#v err=%v", job, err)
	}
}

func TestGroupTargetAndGovernanceHandler(t *testing.T) {
	prepareControlJobTest(t)
	defaultStore.spokes["spoke-test"].Profile = &SpokeProfile{
		Capabilities: []string{"service.status"},
	}
	defaultStore.spokes["spoke-second"] = &SpokeRecord{
		ID:       "spoke-second",
		LastSeen: time.Now(),
		Profile:  &SpokeProfile{Capabilities: []string{"service.status"}},
	}
	defaultStore.spokes["spoke-maintenance"] = &SpokeRecord{
		ID:          "spoke-maintenance",
		LastSeen:    time.Now(),
		Group:       "production",
		Maintenance: true,
		Profile:     &SpokeProfile{Capabilities: []string{"service.status"}},
	}
	for _, spokeID := range []string{"spoke-test", "spoke-second"} {
		body := bytes.NewBufferString(`{"group":"production","environment":"prod","maintenance":false}`)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/hub/spoke/governance?spoke="+spokeID, body)
		SpokeGovernanceAdminHandler(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("更新节点 %s 失败: HTTP %d body=%s", spokeID, recorder.Code, recorder.Body.String())
		}
	}

	payload := []byte(`{"group":"production","action":{"type":"status"},"idempotency_key":"group-status-1"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/hub/control", bytes.NewReader(payload))
	ControlEnqueueAdminHandler(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("按组创建任务失败: HTTP %d body=%s", recorder.Code, recorder.Body.String())
	}
	var batch ControlJobBatch
	if err := json.Unmarshal(recorder.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Jobs) != 2 {
		t.Fatalf("按组任务数 = %d", len(batch.Jobs))
	}
}
