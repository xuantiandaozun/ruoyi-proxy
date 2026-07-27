package hub

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPFromTrustedLocalProxy(t *testing.T) {
	req := httptest.NewRequest("POST", "/__hub__/v1/profile", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Real-IP", "203.0.113.9")
	if got := clientIPFromRequest(req); got != "203.0.113.9" {
		t.Fatalf("来源 IP = %q", got)
	}
}

func TestClientIPIgnoresUntrustedForwardedHeader(t *testing.T) {
	req := httptest.NewRequest("POST", "/__hub__/v1/profile", nil)
	req.RemoteAddr = "198.51.100.7:12345"
	req.Header.Set("X-Real-IP", "203.0.113.9")
	if got := clientIPFromRequest(req); got != "198.51.100.7" {
		t.Fatalf("来源 IP = %q", got)
	}
}
