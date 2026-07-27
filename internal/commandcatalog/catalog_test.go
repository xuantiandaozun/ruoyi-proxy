package commandcatalog

import "testing"

func TestCatalogNamesAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, command := range All() {
		if command.Name == "" || command.Name[0] != '/' {
			t.Fatalf("命令名必须以 / 开头: %q", command.Name)
		}
		if seen[command.Name] {
			t.Fatalf("命令重复登记: %s", command.Name)
		}
		seen[command.Name] = true
	}
}

func TestOperationalLookup(t *testing.T) {
	if !IsOperational("hub-exec") || !IsOperational("/db-discover") {
		t.Fatal("运维命令未被识别")
	}
	if IsOperational("/sessions") || IsOperational("/not-exists") {
		t.Fatal("非运维命令被错误识别")
	}
}
