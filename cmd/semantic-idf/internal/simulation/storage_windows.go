//go:build windows

package simulation

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func storagePlatformPathKey(path string) string {
	// Expand DOS 8.3 spellings without following reparse points. Following links
	// here would weaken the separate no-link ownership/deletion checks.
	original := path
	var missing []string
	for {
		if expanded, ok := storageLongPathName(path); ok {
			for index := len(missing) - 1; index >= 0; index-- {
				expanded = filepath.Join(expanded, missing[index])
			}
			return strings.ToLower(expanded)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return strings.ToLower(original)
		}
		missing = append(missing, filepath.Base(path))
		path = parent
	}
}

func storageLongPathName(path string) (string, bool) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", false
	}
	buffer := make([]uint16, 260)
	length, err := windows.GetLongPathName(name, &buffer[0], uint32(len(buffer)))
	if err != nil || length == 0 || length > 32768 {
		return "", false
	}
	if length >= uint32(len(buffer)) {
		buffer = make([]uint16, length+1)
		length, err = windows.GetLongPathName(name, &buffer[0], uint32(len(buffer)))
		if err != nil || length == 0 || length >= uint32(len(buffer)) {
			return "", false
		}
	}
	return windows.UTF16ToString(buffer[:length]), true
}

func storagePathIsLink(path string, info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return true
	}
	attributes, err := syscall.GetFileAttributes(name)
	return err != nil || attributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func storageProcessIsRunning(pid int) bool {
	process, err := syscall.OpenProcess(0x1000, false, uint32(pid)) // PROCESS_QUERY_LIMITED_INFORMATION
	if err != nil {
		// Access denied means a process exists but cannot be inspected.
		return err != syscall.Errno(87) // ERROR_INVALID_PARAMETER: PID no longer exists.
	}
	defer syscall.CloseHandle(process)
	var code uint32
	return syscall.GetExitCodeProcess(process, &code) != nil || code == 259 // STILL_ACTIVE
}
