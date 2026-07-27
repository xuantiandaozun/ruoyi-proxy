package spokecontrol

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyRunningUsesManagementStatus(t *testing.T) {
	oldURL := proxyStatusURL
	oldClient := proxyStatusClient
	defer func() {
		proxyStatusURL = oldURL
		proxyStatusClient = oldClient
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	proxyStatusURL = server.URL + "/status"
	proxyStatusClient = server.Client()
	if !ProxyRunning() {
		t.Fatal("代理管理接口返回 200 时应识别为运行中")
	}

	proxyStatusURL = server.URL + "/missing"
	if ProxyRunning() {
		t.Fatal("非代理管理接口不应识别为运行中")
	}
}
