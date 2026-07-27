package spokecontrol

import (
	"strings"
	"testing"
)

func TestTruncateOutput(t *testing.T) {
	value := strings.Repeat("x", 20)
	got := truncateOutput(value, 8)
	if !strings.HasPrefix(got, strings.Repeat("x", 8)) || !strings.Contains(got, "已截断") {
		t.Fatalf("截断结果异常: %q", got)
	}
}
