//go:build windows

package simulation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"time"

	"golang.org/x/sys/windows"
)

func acquireStorageInstanceGate(root, _ string, wait time.Duration) (func(), error) {
	type acquired struct {
		release func()
		err     error
	}
	result := make(chan acquired, 1)
	go func() {
		// Windows mutex ownership belongs to an OS thread, not a goroutine.
		// Keep creation, acquisition and release on one pinned helper thread.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		digest := sha256.Sum256([]byte(storagePathKey(root)))
		name, err := windows.UTF16PtrFromString(`Local\SemanticIDF.Storage.` + hex.EncodeToString(digest[:16]))
		if err != nil {
			result <- acquired{err: err}
			return
		}
		handle, err := windows.CreateMutex(nil, false, name)
		if handle == 0 || (err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS)) {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
			result <- acquired{err: err}
			return
		}
		defer windows.CloseHandle(handle)
		status, err := windows.WaitForSingleObject(handle, uint32(wait/time.Millisecond))
		if err != nil || (status != windows.WAIT_OBJECT_0 && status != windows.WAIT_ABANDONED) {
			if err == nil {
				err = fmt.Errorf("storage is in use by another app operation; retry after it finishes")
			}
			result <- acquired{err: err}
			return
		}
		release, done := make(chan struct{}), make(chan struct{})
		result <- acquired{release: func() { close(release); <-done }}
		<-release
		_ = windows.ReleaseMutex(handle)
		close(done)
	}()
	value := <-result
	return value.release, value.err
}
