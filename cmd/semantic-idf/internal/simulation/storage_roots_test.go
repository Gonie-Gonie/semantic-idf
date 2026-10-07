package simulation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageTracksGeneratedRunsAfterConfiguredRootChanges(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	firstRoot := filepath.Join(t.TempDir(), "first-location")
	secondRoot := t.TempDir()
	if err := os.MkdirAll(firstRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(firstRoot, "old-location-run")
	finish, err := beginRunStorage(output, firstRoot, "old-location-run", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "eplus.sql"), []byte("generated SQL"), 0o644); err != nil {
		finish()
		t.Fatal(err)
	}
	finish()
	settings := DefaultSettings()
	settings.RunDirectory = secondRoot
	usage := InspectRunStorage(settings, nil)
	if usage.ReclaimableRunCount != 1 || usage.ReclaimableBytes == 0 {
		t.Fatalf("changing the configured directory hid old generated files: %#v", usage)
	}
	result, err := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || result.RemovedRunCount != 1 {
		t.Fatalf("previous location cleanup: %#v, %v", result, err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("old generated run remains: %v", err)
	}
	if err := rememberRunStorageRoot(secondRoot); err != nil {
		t.Fatal(err)
	}
	roots, issue := knownRunStorageRoots()
	if issue != nil || len(roots) != 1 || storagePathKey(roots[0]) != storagePathKey(secondRoot) {
		t.Fatalf("empty old roots should leave the bounded index: %v, %v", roots, issue)
	}
}

func TestStorageRootIndexRejectsUnverifiedPathsAndKeepsOriginalMetadata(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	root := t.TempDir()
	if err := rememberRunStorageRoot(root); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(filepath.Dir(DefaultSettings().RunDirectory), "storage-roots.json")
	invalid := []byte(`{"schema":"semantic-idf.run-roots/v1","roots":["../user-models"]}`)
	if err := os.WriteFile(indexPath, invalid, 0o644); err != nil {
		t.Fatal(err)
	}
	if roots, issue := knownRunStorageRoots(); len(roots) != 0 || issue == nil {
		t.Fatalf("invalid path was accepted: %v, %v", roots, issue)
	}
	if err := rememberRunStorageRoot(t.TempDir()); err == nil {
		t.Fatal("unverified metadata must not be silently replaced")
	}
	content, err := os.ReadFile(indexPath)
	if err != nil || string(content) != string(invalid) {
		t.Fatalf("original metadata changed: %q, %v", content, err)
	}
}
