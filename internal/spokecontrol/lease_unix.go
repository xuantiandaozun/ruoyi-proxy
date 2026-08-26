//go:build !windows

package spokecontrol

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

var errFileLocked = errors.New("文件已锁定")

func tryLockFile(file *os.File) error {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return errFileLocked
		}
		return err
	}
	return nil
}

func unlockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
