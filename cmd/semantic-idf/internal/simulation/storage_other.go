//go:build !windows

package simulation

import (
	"errors"
	"os"
	"syscall"
)

func storagePlatformPathKey(path string) string { return path }

func storagePathIsLink(_ string, info os.FileInfo) bool { return info.Mode()&os.ModeSymlink != 0 }

func storageProcessIsRunning(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
