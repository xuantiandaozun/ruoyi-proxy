package hub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestControlAdminHandlers(t *testing.T) {
	prepareControlJobTest(t)
	body := []byte(`{"spoke_id":"spoke-test","command":"echo api","timeout_seconds":10,"idempotency_key":"api-request-1","max_attempts":2}`)
	enqueueRecorder := httptest.NewRecorder()
	enqueueRequest := httptest.NewRequest(http.MethodPost, "/hub/control", bytes.NewReader(body))
	ControlEnqueueAdminHandler(enqueueRecorder, enqueueRequest)
	if enqueueRecorder.Code != http.StatusAccepted {
		t.Fatalf("创建任务失败: HTTP %d body=%s", enqueueRecorder.Code, enqueueRecorder.Body.String())
	}
	var created ControlJob
	if err := json.Unmarshal(enqueueRecorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.IdempotencyKey != "api-request-1" || created.MaxAttempts != 2 {
		t.Fatalf("任务可选字段未生效: %#v", created)
	}

	cancelBody, _ := json.Marshal(map[string]string{"job_id": created.ID})
	cancelRecorder := httptest.NewRecorder()
	cancelRequest := httptest.NewRequest(http.MethodPost, "/hub/control/cancel", bytes.NewReader(cancelBody))
	ControlCancelAdminHandler(cancelRecorder, cancelRequest)
	if cancelRecorder.Code != http.StatusOK {
		t.Fatalf("取消任务失败: HTTP %d body=%s", cancelRecorder.Code, cancelRecorder.Body.String())
	}

	retryRecorder := httptest.NewRecorder()
	retryRequest := httptest.NewRequest(http.MethodPost, "/hub/control/retry", bytes.NewReader(cancelBody))
	ControlRetryAdminHandler(retryRecorder, retryRequest)
	if retryRecorder.Code != http.StatusOK {
		t.Fatalf("重试任务失败: HTTP %d body=%s", retryRecorder.Code, retryRecorder.Body.String())
	}
}
