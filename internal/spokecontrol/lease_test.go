package spokecontrol

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestControlLeaseAllowsOnlyOneOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spoke-control.lock")
	first, err := acquireControlLease(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	if _, err := acquireControlLease(path); !errors.Is(err, ErrControlAlreadyOwned) {
		t.Fatalf("重复获取所有权错误 = %v，期望 %v", err, ErrControlAlreadyOwned)
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := acquireControlLease(path)
	if err != nil {
		t.Fatalf("释放后应可重新获取所有权: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}
