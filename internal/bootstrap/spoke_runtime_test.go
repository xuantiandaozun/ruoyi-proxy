package bootstrap

import (
	"testing"

	"ruoyi-proxy/internal/hub"
)

func TestSummarizeSpokeHealth(t *testing.T) {
	tests := []struct {
		name     string
		services []hub.SpokeServiceRef
		status   string
	}{
		{name: "empty", status: "unknown"},
		{
			name: "healthy",
			services: []hub.SpokeServiceRef{
				{ID: "a", Health: "healthy"},
				{ID: "b", Health: "healthy"},
			},
			status: "healthy",
		},
		{
			name: "degraded",
			services: []hub.SpokeServiceRef{
				{ID: "a", Health: "healthy"},
				{ID: "b", Health: "unreachable"},
			},
			status: "degraded",
		},
		{
			name: "unhealthy",
			services: []hub.SpokeServiceRef{
				{ID: "a", Health: "unreachable"},
			},
			status: "unhealthy",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := summarizeSpokeHealth(test.services)
			if got.Status != test.status || got.CheckedAt.IsZero() {
				t.Fatalf("健康摘要 = %#v", got)
			}
		})
	}
}
