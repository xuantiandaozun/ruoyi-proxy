package cli

import (
	"testing"
	"time"

	"ruoyi-proxy/internal/hub"
)

func TestSystemdQuote(t *testing.T) {
	got := systemdQuote(`/opt/ruoyi proxy/"node"`)
	want := `"/opt/ruoyi proxy/\"node\""`
	if got != want {
		t.Fatalf("systemd 路径转义 = %q，期望 %q", got, want)
	}
}

func TestSpokeActivityStatus(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		item hub.SpokeRecord
		want string
	}{
		{name: "online", item: hub.SpokeRecord{LastSeen: now.Add(-5 * time.Second)}, want: "在线"},
		{name: "idle", item: hub.SpokeRecord{LastSeen: now.Add(-time.Minute)}, want: "空闲"},
		{name: "offline", item: hub.SpokeRecord{LastSeen: now.Add(-time.Hour)}, want: "离线"},
		{name: "revoked", item: hub.SpokeRecord{Revoked: true, LastSeen: now}, want: "已吊销"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := spokeActivityStatus(tc.item, now); got != tc.want {
				t.Fatalf("状态 = %s，期望 %s", got, tc.want)
			}
		})
	}
}
