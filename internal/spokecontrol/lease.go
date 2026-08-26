package spokecontrol

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const controlLeaseFile = "configs/spoke_control.lock"

// ErrControlAlreadyOwned 表示本机已有其他进程负责 Spoke 远程控制。
var ErrControlAlreadyOwned = errors.New("Spoke 远程控制已由其他进程接管")

// ControlLease 保持跨进程的 Spoke 远程控制所有权。
type ControlLease struct {
	file *os.File
}

// AcquireControlLease 尝试非阻塞获取本机 Spoke 远程控制所有权。
func AcquireControlLease() (*ControlLease, error) {
	return acquireControlLease(controlLeaseFile)
}

func acquireControlLease(path string) (*ControlLease, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("创建 Spoke 控制锁目录: %v", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("打开 Spoke 控制锁: %v", err)
	}
	if err := tryLockFile(file); err != nil {
		_ = file.Close()
		if errors.Is(err, errFileLocked) {
			return nil, ErrControlAlreadyOwned
		}
		return nil, fmt.Errorf("获取 Spoke 控制锁: %v", err)
	}
	return &ControlLease{file: file}, nil
}

// Close 释放 Spoke 远程控制所有权。
func (l *ControlLease) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	if err := unlockFile(file); err != nil {
		_ = file.Close()
		return fmt.Errorf("释放 Spoke 控制锁: %v", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭 Spoke 控制锁: %v", err)
	}
	return nil
}
