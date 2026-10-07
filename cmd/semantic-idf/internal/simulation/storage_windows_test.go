//go:build windows

package simulation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestRunStoragePartialLockedFileCleanupReportsActualRemovedBytes(t *testing.T) {
	settings := storageTestSettings(t)
	directory := storageTestManagedRun(t, settings.RunDirectory, "locked", map[string]string{"eplusa.sql": "aaaa", "eplusb.sql": "bbbbb"}, 1)
	name, err := syscall.UTF16PtrFromString(filepath.Join(directory, "eplusb.sql"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	result, cleanErr := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	closeErr := syscall.CloseHandle(file)
	if cleanErr != nil || closeErr != nil {
		t.Fatalf("clean=%v close=%v", cleanErr, closeErr)
	}
	if result.FreedBytes != 4 || result.RemovedRunCount != 0 || len(result.Failures) != 1 {
		t.Fatalf("partial cleanup = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(directory, runStorageMarkerName)); err != nil {
		t.Fatalf("ownership evidence must survive a partial failure: %v", err)
	}
	retry, err := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || retry.RemovedRunCount != 1 || len(retry.Failures) != 0 || retry.Usage.RunCount != 0 {
		t.Fatalf("retry cleanup=%#v error=%v", retry, err)
	}
}

func TestRunStorageLegacyPartialCleanupRetainsIndependentRetryProof(t *testing.T) {
	settings := storageTestSettings(t)
	started := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	directory := filepath.Join(settings.RunDirectory, started.Format("20060102-150405")+"-legacy-model")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(directory, "model.idf")
	if err := os.WriteFile(input, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	sql := filepath.Join(directory, "eplus.sql")
	if err := os.WriteFile(sql, []byte("SQL"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := SimulationRunManifest{RunID: "legacy", CreatedAt: started.Add(time.Second).Format(time.RFC3339), StartedAt: started.Format(time.RFC3339), FinishedAt: started.Add(time.Second).Format(time.RFC3339), Status: "succeeded", Filename: "model.idf", InputPath: input, InputHash: fileSHA256(input), OutputDirectory: directory, ResultFiles: []SimulationFileInfo{{Name: "eplus.sql", Path: sql}}}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(directory, "semantic-idf-run.json")
	if err := os.WriteFile(manifestPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	before, issue := MeasureStorageDirectory(directory)
	if issue != nil {
		t.Fatal(issue)
	}
	name, err := syscall.UTF16PtrFromString(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	file, err := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	first, cleanErr := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	closeErr := syscall.CloseHandle(file)
	if cleanErr != nil || closeErr != nil || len(first.Failures) != 1 || first.RemovedRunCount != 0 {
		t.Fatalf("legacy partial cleanup=%#v clean=%v close=%v", first, cleanErr, closeErr)
	}
	if _, err := os.Stat(input); !os.IsNotExist(err) {
		t.Fatalf("fixture must exercise missing-input retry: %v", err)
	}
	if first.Usage.ReclaimableRunCount != 1 {
		t.Fatalf("independent ownership proof was not retained: %#v", first.Usage)
	}
	if first.FreedBytes != max(0, before-first.Usage.TotalBytes) {
		t.Fatalf("partial reclaimed bytes do not match the actual file footprint: before=%d remaining=%d freed=%d", before, first.Usage.TotalBytes, first.FreedBytes)
	}
	retry, err := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || len(retry.Failures) != 0 || retry.RemovedRunCount != 1 || retry.Usage.RunCount != 0 {
		t.Fatalf("legacy retry=%#v error=%v", retry, err)
	}
	// The new ownership marker must not inflate the net reclaimed bytes.
	if retry.FreedBytes != first.Usage.TotalBytes {
		t.Fatalf("retry reclaimed accounting is inconsistent: remaining=%d retry=%d", first.Usage.TotalBytes, retry.FreedBytes)
	}
}
