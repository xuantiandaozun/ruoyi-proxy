package cli

import (
	"testing"

	"ruoyi-proxy/internal/commandcatalog"
)

func TestSlashMenuUsesSharedCommandCatalog(t *testing.T) {
	cli := New()
	items := cli.buildSlashMenuItems()
	commands := commandcatalog.All()
	if len(items) != len(commands) {
		t.Fatalf("斜杠菜单数量=%d，共享目录数量=%d", len(items), len(commands))
	}
	for i := range commands {
		if items[i].Command != commands[i].Name || items[i].Description != commands[i].Description {
			t.Fatalf("斜杠菜单未同步目录项: %+v != %+v", items[i], commands[i])
		}
	}
}
