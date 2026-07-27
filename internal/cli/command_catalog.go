package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"ruoyi-proxy/internal/commandcatalog"
)

// printCommandCatalog 从共享目录输出当前版本实际支持的斜杠命令。
func (c *CLI) printCommandCatalog() {
	commands := commandcatalog.All()
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	currentCategory := ""
	for _, command := range commands {
		if command.Category != currentCategory {
			if currentCategory != "" {
				fmt.Fprintln(writer)
			}
			currentCategory = command.Category
			fmt.Fprintf(writer, "【%s】\n", currentCategory)
		}
		fmt.Fprintf(writer, "  %s\t%s\n", command.Usage, command.Description)
	}
	_ = writer.Flush()
}
