package nodeinfo

import (
	"sort"
	"testing"
)

func TestSnapshotIncludesProtocolAndBaseCapabilities(t *testing.T) {
	version, protocol, capabilities, resources := Snapshot()
	if version == "" {
		t.Fatal("版本不能为空")
	}
	if protocol != ControlProtocolVersion {
		t.Fatalf("控制协议 = %d", protocol)
	}
	for _, capability := range []string{
		CapabilityControlV2,
		CapabilityShell,
		CapabilityDatabaseRead,
	} {
		if !HasCapability(capabilities, capability) {
			t.Fatalf("缺少基础能力: %s，全部能力=%v", capability, capabilities)
		}
	}
	if !sort.StringsAreSorted(capabilities) {
		t.Fatalf("能力列表未排序: %v", capabilities)
	}
	if resources.CPUCount < 1 || resources.CollectedAt.IsZero() {
		t.Fatalf("资源快照异常: %#v", resources)
	}
}
