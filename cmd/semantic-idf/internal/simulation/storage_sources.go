package simulation

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveStorageSourcePath follows an existing source's file/directory aliases.
// New or restored missing files resolve their existing parent when possible.
func ResolveStorageSourcePath(path string) (string, error) {
	if path = strings.TrimSpace(path); path == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	target, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return target, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err == nil {
		return filepath.Join(parent, filepath.Base(absolute)), nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	return absolute, nil
}

// ResolveStorageSourceDirectories keeps both the original and resolved parents
// so selecting an alias cannot leave its actual generated input unprotected.
func ResolveStorageSourceDirectories(paths []string) ([]string, error) {
	directories := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if path = strings.TrimSpace(path); path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		target, err := ResolveStorageSourcePath(absolute)
		if err != nil {
			return nil, err
		}
		for _, directory := range []string{filepath.Dir(absolute), filepath.Dir(target)} {
			key := storagePathKey(directory)
			if !seen[key] {
				seen[key] = true
				directories = append(directories, directory)
			}
		}
	}
	return directories, nil
}
