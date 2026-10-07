package simulation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func storageTestSettings(t *testing.T) SimulationSettings {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	settings := DefaultSettings()
	if err := os.MkdirAll(settings.RunDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	return settings
}

func TestStorageRunFolderDiscoveryExcludesProgramMetadata(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"in.idf":                     "Version,24.1;",
		"model.json":                 "{}",
		runStorageMarkerName:         "{}",
		"semantic-idf-run.json":      "{}",
		"semantic-idf-run-plan.json": "{}",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, recursive := range []bool{false, true} {
		paths, err := FindInputFiles(root, recursive)
		if err != nil || len(paths) != 2 || paths[0] != filepath.Join(root, "in.idf") || paths[1] != filepath.Join(root, "model.json") {
			t.Fatalf("run folder inputs recursive=%t: %#v %v", recursive, paths, err)
		}
	}
}

func TestStorageLinkedRunRootRemainsUsableAndUnmanaged(t *testing.T) {
	settings := storageTestSettings(t)
	target := t.TempDir()
	root := filepath.Join(t.TempDir(), "linked-runs")
	if err := os.Symlink(target, root); err != nil {
		t.Skipf("linked run root creation is unavailable: %v", err)
	}
	directory := filepath.Join(root, "unmanaged-run")
	finish, err := beginRunStorage(directory, root, "unmanaged-run", "in.idf", true)
	if err != nil {
		t.Fatalf("storage bookkeeping blocked a valid user-selected run destination: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "in.idf"), []byte("Version,24.1;"), 0o644); err != nil {
		finish()
		t.Fatal(err)
	}
	finish()
	if _, err := os.Stat(filepath.Join(target, "unmanaged-run", runStorageMarkerName)); !os.IsNotExist(err) {
		t.Fatalf("linked output destination must not acquire ownership: %v", err)
	}
	settings.RunDirectory = root
	result, err := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || result.RemovedRunCount != 0 || len(result.Usage.Warnings) == 0 {
		t.Fatalf("linked root cleanup must be skipped and reported: %#v %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(target, "unmanaged-run", "in.idf")); err != nil {
		t.Fatalf("cleanup modified the unmanaged linked output: %v", err)
	}
	settings.RunDirectory = t.TempDir()
	usage := InspectRunStorage(settings, nil)
	found := false
	for _, issue := range usage.Warnings {
		if storagePathKey(issue.Path) == storagePathKey(root) {
			found = true
		}
	}
	if !found {
		t.Fatalf("old linked run location disappeared after changing settings: %#v", usage)
	}
}

func storageTestManagedRun(t *testing.T, root, name string, files map[string]string, daysOld int) string {
	t.Helper()
	directory := filepath.Join(root, name)
	finish, err := beginRunStorage(directory, root, name, "in.idf", true)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	finish()
	var marker runStorageMarker
	if err := readStorageJSON(filepath.Join(directory, runStorageMarkerName), &marker); err != nil {
		t.Fatal(err)
	}
	marker.FinishedAt = time.Now().Add(-time.Duration(daysOld)*24*time.Hour - time.Minute).Format(time.RFC3339)
	finished, _ := time.Parse(time.RFC3339, marker.FinishedAt)
	marker.CreatedAt = finished.Add(-time.Second).Format(time.RFC3339)
	if err := writeRunStorageMarker(directory, marker); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestRunStorageCleanupHonorsOwnershipAgeAndCurrentResults(t *testing.T) {
	settings := storageTestSettings(t)
	old := storageTestManagedRun(t, settings.RunDirectory, "old", map[string]string{"in.idf": "original run copy", "eplus.sql": "sql bytes"}, 45)
	recent := storageTestManagedRun(t, settings.RunDirectory, "recent", map[string]string{"eplus.sql": "recent SQL"}, 2)
	current := storageTestManagedRun(t, settings.RunDirectory, "current", map[string]string{"eplus.sql": "current SQL"}, 50)
	userAdded := storageTestManagedRun(t, settings.RunDirectory, "export-added", map[string]string{"eplus.sql": "SQL"}, 50)
	if err := os.WriteFile(filepath.Join(userAdded, "my-workbook.xlsx"), []byte("user export"), 0o644); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(settings.RunDirectory, "unknown-user-folder")
	if err := os.MkdirAll(unknown, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unknown, "original.idf"), []byte("user original"), 0o644); err != nil {
		t.Fatal(err)
	}
	usage := InspectRunStorage(settings, []string{current})
	if usage.RunCount != 5 || usage.ReclaimableRunCount != 2 || usage.ProtectedRunCount != 3 || usage.TotalBytes != usage.ReclaimableBytes+usage.ProtectedBytes {
		t.Fatalf("usage = %#v", usage)
	}
	oldBytes, issue := MeasureStorageDirectory(old)
	if issue != nil {
		t.Fatal(issue)
	}
	result, err := CleanRunStorage(settings, RunStorageCleanupRequest{OlderThanDays: 30}, []string{current})
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedRunCount != 1 || result.SkippedRunCount != 4 || result.FreedBytes != oldBytes || len(result.Failures) != 0 {
		t.Fatalf("cleanup = %#v", result)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old managed run was not removed: %v", err)
	}
	for _, path := range []string{recent, current, userAdded, unknown} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("protected/recent data disappeared: %s: %v", path, err)
		}
	}
	if result.Usage.TotalBytes != usage.TotalBytes-oldBytes {
		t.Fatalf("usage after cleanup = %d, want %d", result.Usage.TotalBytes, usage.TotalBytes-oldBytes)
	}
}

func TestRunStorageCustomRootKeepsUnmarkedAndExplicitDirectories(t *testing.T) {
	settings := storageTestSettings(t)
	settings.RunDirectory = t.TempDir()
	managed := storageTestManagedRun(t, settings.RunDirectory, "created-by-app", map[string]string{"eplus.sql": "SQL"}, 1)
	explicit := filepath.Join(settings.RunDirectory, "explicit-output")
	finish, err := beginRunStorage(explicit, settings.RunDirectory, "explicit", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(explicit, "eplus.sql"), []byte("explicit output"), 0o644); err != nil {
		t.Fatal(err)
	}
	finish()
	user := filepath.Join(settings.RunDirectory, "personal-data")
	if err := os.MkdirAll(user, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, "model.idf"), []byte("user data"), 0o644); err != nil {
		t.Fatal(err)
	}
	usage := InspectRunStorage(settings, nil)
	if usage.RunCount != 1 || usage.ReclaimableRunCount != 1 || usage.ProtectedRunCount != 0 || len(usage.Roots) != 2 {
		t.Fatalf("custom inventory = %#v", usage)
	}
	result, err := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || result.RemovedRunCount != 1 {
		t.Fatalf("cleanup result=%#v error=%v", result, err)
	}
	if _, err := os.Stat(managed); !os.IsNotExist(err) {
		t.Fatalf("managed run remains: %v", err)
	}
	for _, path := range []string{explicit, user} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("non-managed data disappeared: %v", err)
		}
	}
}

func TestRunStorageActiveIncompleteAndReusedDirectoriesRemainProtected(t *testing.T) {
	settings := storageTestSettings(t)
	active := filepath.Join(settings.RunDirectory, "active")
	finish, err := beginRunStorage(active, settings.RunDirectory, "active", "", true)
	if err != nil {
		t.Fatal(err)
	}
	result, err := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || result.RemovedRunCount != 0 || result.Usage.ProtectedRunCount != 1 {
		t.Fatalf("active cleanup=%#v error=%v", result, err)
	}
	finish()
	// An automatic naming collision must not adopt the user's added files.
	reused := storageTestManagedRun(t, settings.RunDirectory, "reused", map[string]string{"eplus.sql": "SQL"}, 1)
	if err := os.WriteFile(filepath.Join(reused, "my-export.csv"), []byte("user data"), 0o644); err != nil {
		t.Fatal(err)
	}
	finishAgain, err := beginRunStorage(reused, settings.RunDirectory, "reused", "", true)
	if err != nil {
		t.Fatal(err)
	}
	finishAgain()
	result, err = CleanRunStorage(settings, RunStorageCleanupRequest{ProtectedOutputDirectories: []string{active}}, nil)
	if err != nil || result.RemovedRunCount != 0 || result.Usage.ProtectedRunCount != 2 {
		t.Fatalf("reused cleanup=%#v error=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(reused, "my-export.csv")); err != nil {
		t.Fatalf("export disappeared: %v", err)
	}
}

func TestRunStorageLegacyRequiresMatchingGeneratedCompletedManifest(t *testing.T) {
	settings := storageTestSettings(t)
	started := time.Now().Add(-40 * 24 * time.Hour).Truncate(time.Second)
	name := started.Format("20060102-150405") + "-legacy-model"
	directory := filepath.Join(settings.RunDirectory, name)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(directory, "model.idf")
	sql := filepath.Join(directory, "eplus.sql")
	for _, path := range []string{input, sql} {
		if err := os.WriteFile(path, []byte("run data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := SimulationRunManifest{RunID: "legacy", CreatedAt: started.Add(time.Second).Format(time.RFC3339), StartedAt: started.Format(time.RFC3339), FinishedAt: started.Add(time.Second).Format(time.RFC3339), Status: "succeeded", Filename: "model.idf", InputPath: input, InputHash: fileSHA256(input), OutputDirectory: directory, ResultFiles: []SimulationFileInfo{{Name: "eplus.sql", Path: sql}}}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "semantic-idf-run.json"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	usage := InspectRunStorage(settings, nil)
	if usage.ReclaimableRunCount != 1 {
		t.Fatalf("valid legacy run = %#v", usage)
	}
	manifest.OutputDirectory = t.TempDir()
	payload, _ = json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(directory, "semantic-idf-run.json"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || result.RemovedRunCount != 0 || result.Usage.ProtectedRunCount != 1 {
		t.Fatalf("mismatched legacy cleanup=%#v error=%v", result, err)
	}
	manifest.OutputDirectory = directory
	payload, _ = json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(directory, "semantic-idf-run.json"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	before, issue := MeasureStorageDirectory(directory)
	if issue != nil {
		t.Fatal(issue)
	}
	result, err = CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || result.RemovedRunCount != 1 || result.FreedBytes != before {
		t.Fatalf("legacy marker upgrade inflated reclaimed bytes: cleanup=%#v before=%d error=%v", result, before, err)
	}
}

func TestRunStorageLinksNeverFollowOrDeleteTheirTargets(t *testing.T) {
	settings := storageTestSettings(t)
	managed := storageTestManagedRun(t, settings.RunDirectory, "linked", map[string]string{"eplus.sql": "SQL"}, 1)
	outside := t.TempDir()
	protectedFile := filepath.Join(outside, "original.idf")
	if err := os.WriteFile(protectedFile, []byte("outside user file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(managed, "outside")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	result, err := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || result.RemovedRunCount != 0 || result.Usage.ProtectedRunCount != 1 {
		t.Fatalf("linked cleanup=%#v error=%v", result, err)
	}
	if _, err := os.Stat(protectedFile); err != nil {
		t.Fatalf("symlink target changed: %v", err)
	}
	if _, err := MeasureStorageDirectory(managed); err == nil {
		t.Fatal("linked directory scan should report unavailable/partial data")
	}
}

func TestRunStorageCleanupRejectsInvalidAge(t *testing.T) {
	settings := storageTestSettings(t)
	for _, age := range []int{-1, 36501} {
		if _, err := CleanRunStorage(settings, RunStorageCleanupRequest{OlderThanDays: age}, nil); err == nil {
			t.Fatalf("age %d accepted", age)
		}
	}
}

func TestRunStorageInvalidMetadataAndLiveOtherProcessRemainProtected(t *testing.T) {
	settings := storageTestSettings(t)
	invalid := storageTestManagedRun(t, settings.RunDirectory, "invalid-marker", map[string]string{"eplus.sql": "SQL"}, 1)
	path := filepath.Join(invalid, runStorageMarkerName)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\n{\"another\":\"object\"}"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	other := storageTestManagedRun(t, settings.RunDirectory, "other-instance", map[string]string{"eplus.sql": "other SQL"}, 1)
	var marker runStorageMarker
	if err := readStorageJSON(filepath.Join(other, runStorageMarkerName), &marker); err != nil {
		t.Fatal(err)
	}
	marker.ProcessID = os.Getppid()
	if marker.ProcessID <= 0 || !storageProcessIsRunning(marker.ProcessID) {
		t.Skip("parent process cannot be inspected")
	}
	if err := writeRunStorageMarker(other, marker); err != nil {
		t.Fatal(err)
	}
	result, err := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || result.RemovedRunCount != 0 || result.Usage.ProtectedRunCount != 2 {
		t.Fatalf("invalid/live metadata cleanup=%#v error=%v", result, err)
	}
}

func TestRunStorageUserFilesAddedDuringExecutionAreNeverAdopted(t *testing.T) {
	settings := storageTestSettings(t)
	directory := filepath.Join(settings.RunDirectory, "while-running")
	finish, err := beginRunStorage(directory, settings.RunDirectory, "while-running", "run-copy.idf", true)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"run-copy.idf": "run model", "eplus.sql": "SQL", "my-export.xlsx": "user export"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	finish()
	result, err := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || result.RemovedRunCount != 0 || result.Usage.ProtectedRunCount != 1 {
		t.Fatalf("user export was adopted: cleanup=%#v error=%v", result, err)
	}
}

func TestRunStorageKeepsChangedGeneratedFilesAndPreExistingReservedFiles(t *testing.T) {
	settings := storageTestSettings(t)
	changed := storageTestManagedRun(t, settings.RunDirectory, "edited", map[string]string{"in.idf": "run input", "eplus.sql": "SQL"}, 1)
	if err := os.WriteFile(filepath.Join(changed, "in.idf"), []byte("user-edited input"), 0o644); err != nil {
		t.Fatal(err)
	}
	reserved := filepath.Join(settings.RunDirectory, "user-folder")
	if err := os.MkdirAll(reserved, 0o755); err != nil {
		t.Fatal(err)
	}
	reservedPath := filepath.Join(reserved, runStorageMarkerName)
	if err := os.WriteFile(reservedPath, []byte("user reserved filename"), 0o644); err != nil {
		t.Fatal(err)
	}
	finish, err := beginRunStorage(reserved, settings.RunDirectory, "user-folder", "", true)
	if err != nil {
		t.Fatal(err)
	}
	finish()
	if content, err := os.ReadFile(reservedPath); err != nil || string(content) != "user reserved filename" {
		t.Fatalf("pre-existing user marker changed: %q %v", content, err)
	}
	result, err := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || result.RemovedRunCount != 0 || result.Usage.ProtectedRunCount != 2 {
		t.Fatalf("changed/reserved cleanup=%#v error=%v", result, err)
	}
}

func TestRunStorageInputPromotionSurvivesCompletionAndRestart(t *testing.T) {
	settings := storageTestSettings(t)
	directory := filepath.Join(settings.RunDirectory, "running-input")
	finish, err := beginRunStorage(directory, settings.RunDirectory, "running-input", "in.idf", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "in.idf"), []byte("run input"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PreserveRunStorageDirectory(directory); err != nil {
		t.Fatal(err)
	}
	finish()
	result, err := CleanRunStorage(settings, RunStorageCleanupRequest{}, nil)
	if err != nil || result.RemovedRunCount != 0 || result.Usage.ProtectedRunCount != 1 {
		t.Fatalf("promoted input lost after completion: %#v %v", result, err)
	}
	var marker runStorageMarker
	if err := readStorageJSON(filepath.Join(directory, runStorageMarkerName), &marker); err != nil || marker.Managed || marker.ProtectedReason != "user_input" {
		t.Fatalf("durable input preservation marker=%#v error=%v", marker, err)
	}
}
