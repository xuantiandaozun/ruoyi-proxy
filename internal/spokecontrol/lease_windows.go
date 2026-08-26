//go:build windows

package spokecontrol

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

var errFileLocked = errors.New("文件已锁定")

func tryLockFile(file *os.File) error {
	overlapped := new(windows.Overlapped)
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped)
	if err == nil {
		return nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return errFileLocked
	}
	return err
}

func unlockFile(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, new(windows.Overlapped))
}
