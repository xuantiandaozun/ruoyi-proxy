package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHubRemoteToolsUseManagementAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hub/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"spokes":[{"id":"spoke-farm","profile":{"label":"农药"}}]}`))
		case "/hub/control":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"job-1","spoke_id":"spoke-farm","command":"ps aux","status":"pending"}`))
		case "/hub/jobs":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1,"jobs":[{"id":"job-1","spoke_id":"spoke-farm","command":"ps aux","status":"succeeded","output":"java running"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	oldBaseURL := hubMgmtBaseURL
	oldClient := hubMgmtHTTPClient
	hubMgmtBaseURL = server.URL
	hubMgmtHTTPClient = &http.Client{Timeout: time.Second}
	t.Cleanup(func() {
		hubMgmtBaseURL = oldBaseURL
		hubMgmtHTTPClient = oldClient
	})

	executor := NewToolExecutor(BuildExecContext("default"))
	spokes, err := executor.Execute("hub_spokes", `{"action":"list"}`)
	if err != nil || !strings.Contains(spokes, "spoke-farm") {
		t.Fatalf("节点查询异常: output=%q err=%v", spokes, err)
	}
	result, err := executor.Execute("hub_remote_command", `{"spoke_id":"spoke-farm","command":"ps aux","wait_seconds":1}`)
	if err != nil {
		t.Fatalf("远程执行失败: %v", err)
	}
	if !strings.Contains(result, "succeeded") || !strings.Contains(result, "java running") {
		t.Fatalf("远程结果异常: %s", result)
	}
}

func TestHubRemoteCommandAlwaysRequiresConfirmation(t *testing.T) {
	call := ToolCall{Name: "hub_remote_command", Arguments: `{"spoke_id":"spoke-1","command":"uptime"}`}
	if !scheduledToolNeedsConfirmation(call) {
		t.Fatal("远程命令即使是只读 Shell，也必须经过 Hub 端确认")
	}
}
