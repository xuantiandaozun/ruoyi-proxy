package spokecontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ruoyi-proxy/internal/agent"
	"ruoyi-proxy/internal/hub"
)

func TestWorkerReportsRunningBeforeTerminalResult(t *testing.T) {
	var mu sync.Mutex
	var statuses []string
	polled := false
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/__hub__/v1/control/poll":
			if r.Header.Get("X-Ruoyi-Control-Version") != "2" {
				t.Errorf("控制协议版本头 = %q", r.Header.Get("X-Ruoyi-Control-Version"))
			}
			if !strings.Contains(r.Header.Get("X-Ruoyi-Capabilities"), "control.v2") {
				t.Errorf("能力头 = %q", r.Header.Get("X-Ruoyi-Capabilities"))
			}
			mu.Lock()
			if polled {
				mu.Unlock()
				w.WriteHeader(http.StatusNoContent)
				return
			}
			polled = true
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(hub.ControlJob{
				ID: "job-test", Command: "echo ok", TimeoutSecs: 5,
			})
		case "/__hub__/v1/control/result":
			var result Result
			if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
				t.Errorf("解析结果失败: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			statuses = append(statuses, result.Status)
			mu.Unlock()
			if result.Status == hub.ControlJobSucceeded {
				cancel()
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	worker := &Worker{
		config:       agent.AIConfig{BaseURL: server.URL, APIKey: "test"},
		client:       server.Client(),
		pollInterval: time.Millisecond,
		logf:         func(string, ...interface{}) {},
	}
	if err := worker.Run(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(statuses) != 2 ||
		statuses[0] != hub.ControlJobRunning ||
		statuses[1] != hub.ControlJobSucceeded {
		t.Fatalf("状态上报顺序异常: %#v", statuses)
	}
}
