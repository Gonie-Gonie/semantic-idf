//go:build !windows

package simulation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

func acquireStorageInstanceGate(_, directory string, wait time.Duration) (func(), error) {
	path := filepath.Join(directory, ".cleanup.lock")
	if _, err := os.Lstat(path); err == nil {
		if err := validateStoragePath(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { _ = unix.Flock(fd, unix.LOCK_UN); _ = unix.Close(fd) }, nil
		}
		if (!errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN)) || !time.Now().Before(deadline) {
			_ = unix.Close(fd)
			return nil, fmt.Errorf("storage is in use by another app operation; retry after it finishes: %w", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
