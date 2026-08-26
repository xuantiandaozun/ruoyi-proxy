//go:build !windows

package agent

import "os"

func replaceFile(source, target string) error {
	return os.Rename(source, target)
}
