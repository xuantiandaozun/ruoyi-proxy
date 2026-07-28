package hub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestControlHandlerCreatesStructuredBatch(t *testing.T) {
	prepareControlJobTest(t)
	defaultStore.spokes["spoke-test"].Profile = &SpokeProfile{Capabilities: []string{"service.status"}}
	defaultStore.spokes["spoke-second"] = &SpokeRecord{ID: "spoke-second", LastSeen: time.Now(), Profile: &SpokeProfile{Capabilities: []string{"service.status"}}}
	body := []byte(`{
		"spoke_ids":["spoke-test","spoke-second"],
		"action":{"type":"status","params":{"service_id":"default"}},
		"idempotency_key":"handler-batch-1",
		"source":"api",
		"actor":"operator"
	}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/hub/control", bytes.NewReader(body))
	ControlEnqueueAdminHandler(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("创建批量任务失败: HTTP %d body=%s", recorder.Code, recorder.Body.String())
	}
	var batch ControlJobBatch
	if err := json.Unmarshal(recorder.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if batch.ID == "" || len(batch.Jobs) != 2 {
		t.Fatalf("批量响应异常: %#v", batch)
	}
	for _, job := range batch.Jobs {
		if job.Action == nil || job.Action.Type != ControlActionStatus || job.Source != "api" {
			t.Fatalf("结构化任务字段异常: %#v", job)
		}
	}
}
