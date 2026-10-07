package simulation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	runStorageRootsSchema   = "semantic-idf.run-roots/v1"
	maxRunStorageRoots      = 256
	maxRunStorageRootsBytes = 128 << 10
)

type runStorageRootsIndex struct {
	Schema string   `json:"schema"`
	Roots  []string `json:"roots"`
}

// Remember previous configured locations so changing the run directory does not
// hide generated files. This index is discovery metadata, never proof that a
// directory or its contents can be deleted.
func rememberRunStorageRoot(root string) error {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	// Record the configured spelling for discovery, including junctions that
	// remain valid run destinations. Inspection/deletion still reject links;
	// only app-owned index/session paths require no-link validation here.
	appRoot, sessions, err := storageInstanceDirectory()
	if err != nil {
		return err
	}
	release, err := acquireStorageInstanceGate(appRoot, sessions, 5*time.Second)
	if err != nil {
		return err
	}
	defer release()
	path := filepath.Join(appRoot, "storage-roots.json")
	roots, err := readRunStorageRoots(path)
	if err != nil {
		return err
	}
	for _, known := range roots {
		if storagePathKey(known) == storagePathKey(absolute) {
			return nil
		}
	}
	kept := make([]string, 0, len(roots)+1)
	for _, known := range roots {
		if !emptyRunStorageRoot(known) {
			kept = append(kept, known)
		}
	}
	if len(kept) >= maxRunStorageRoots {
		return fmt.Errorf("managed run directory limit reached; clean old run locations before adding another")
	}
	kept = append(kept, absolute)
	payload, err := json.Marshal(runStorageRootsIndex{Schema: runStorageRootsSchema, Roots: kept})
	if err != nil {
		return err
	}
	if len(payload) > maxRunStorageRootsBytes {
		return fmt.Errorf("managed run directory index exceeds its size limit")
	}
	if err := validateStoragePath(appRoot); err != nil {
		return err
	}
	file, err := os.CreateTemp(appRoot, ".storage-roots-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	_, writeErr := file.Write(append(payload, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := validateStoragePath(appRoot); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		if err := validateStoragePath(path); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func knownRunStorageRoots() ([]string, *StorageIssue) {
	// Discovery must not create directories or need the cleanup gate, which may
	// already be held by the caller. Replacement exposes only a complete index.
	path := filepath.Join(filepath.Dir(DefaultSettings().RunDirectory), "storage-roots.json")
	roots, err := readRunStorageRoots(path)
	if err != nil {
		return nil, &StorageIssue{Path: path, Reason: "managed run directory index could not be read: " + err.Error()}
	}
	return roots, nil
}

func readRunStorageRoots(path string) ([]string, error) {
	if err := validateStoragePath(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxRunStorageRootsBytes {
		return nil, fmt.Errorf("invalid managed run directory index")
	}
	var index runStorageRootsIndex
	decoder := json.NewDecoder(io.LimitReader(file, maxRunStorageRootsBytes))
	if err := decoder.Decode(&index); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("managed run directory index must contain one JSON object")
	}
	if index.Schema != runStorageRootsSchema || len(index.Roots) > maxRunStorageRoots {
		return nil, fmt.Errorf("invalid managed run directory index schema or root count")
	}
	roots := make([]string, 0, len(index.Roots))
	seen := make(map[string]bool, len(index.Roots))
	for _, root := range index.Roots {
		if strings.TrimSpace(root) != root || !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return nil, fmt.Errorf("managed run directory index contains an invalid path")
		}
		key := storagePathKey(root)
		if !seen[key] {
			roots = append(roots, root)
			seen[key] = true
		}
	}
	return roots, nil
}

func emptyRunStorageRoot(path string) bool {
	if err := validateStoragePath(path); err != nil {
		return errors.Is(err, os.ErrNotExist)
	}
	directory, err := os.Open(path)
	if err != nil {
		return false
	}
	defer directory.Close()
	_, err = directory.ReadDir(1)
	return err == io.EOF
}
