package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

func TestStorageAppProtectsWorkspaceReferencesAndReportsBusy(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	app := NewApp()
	app.simulationWorkspaceCache.outputDirectory = "current-single"
	app.storageBatchDirectories = []string{"current-batch"}
	app.storageLoadedDirectory = "opened-saved-run"
	paths := app.protectedStorageDirectories()
	if len(paths) != 3 || paths[0] != "current-batch" || paths[1] != "opened-saved-run" || paths[2] != "current-single" {
		t.Fatalf("protected paths = %#v", paths)
	}
	app.storageMu.RLock()
	_, err := app.CleanStorage(StorageCleanupRequest{})
	app.storageMu.RUnlock()
	if err == nil {
		t.Fatal("cleanup must not race an executing simulation/result read")
	}
	usage, err := app.GetStorageUsage()
	if err != nil {
		t.Fatal(err)
	}
	if usage.TotalBytes != 0 || usage.RunCount != 0 || len(usage.Roots) != 1 {
		t.Fatalf("fresh usage = %#v", usage)
	}
	payload, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"totalBytes", "reclaimableBytes", "roots", "browserDataPath", "browserDataBytes", "browserDataUnavailable"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("usage wire field missing: %s", field)
		}
	}
}

func writeStorageAppRunFixture(t *testing.T, root, name string) string {
	t.Helper()
	return writeStorageAppRunFixtureFiles(t, root, name, map[string][]byte{"eplus.sql": []byte("fixture SQL"), "in.idf": []byte("Version,24.1;\nZone,Office;\n")})
}

func writeStorageAppRunFixtureFiles(t *testing.T, root, name string, files map[string][]byte) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	states := make(map[string]any)
	generated := []string{".semantic-idf-storage.json"}
	for name, content := range files {
		file := filepath.Join(path, name)
		if err := os.WriteFile(file, content, 0o644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		states[name] = map[string]any{"size": info.Size(), "modifiedAt": info.ModTime().UnixNano()}
		generated = append(generated, name)
	}
	marker := map[string]any{"schema": "semantic-idf.run-storage/v1", "root": root, "directory": path, "runId": name, "processId": os.Getpid(), "managed": true, "createdAt": time.Now().Add(-2 * time.Hour).Format(time.RFC3339), "finishedAt": time.Now().Add(-time.Hour).Format(time.RFC3339), "generatedFiles": generated, "generatedFileState": states}
	payload, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, ".semantic-idf-storage.json"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStorageAppAutomaticPolicyUsesLatestChoiceAndReleasesResultsAtClose(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	app := NewApp()
	_, settings, err := loadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	old := writeStorageAppRunFixture(t, settings.Simulation.RunDirectory, "previous-run")
	current := writeStorageAppRunFixture(t, settings.Simulation.RunDirectory, "current-run")
	app.storageSingleDirectory = current
	// A queued "on" choice must not override a later persisted "off" choice.
	staleOn := settings
	staleOn.Storage.AutoClean = true
	app.applyStorageCleanupPolicy(staleOn)
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("stale preference removed results while the current choice was off: %v", err)
	}
	settings.Storage.AutoClean = true
	if _, err := app.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("unreferenced previous run was retained: %v", err)
	}
	if _, err := os.Stat(current); err != nil {
		t.Fatalf("current result disappeared: %v", err)
	}
	usage, err := app.GetStorageUsage()
	if err != nil || usage.LastCleanup == nil || !usage.LastCleanup.Automatic || usage.LastCleanup.RemovedRunCount != 1 {
		t.Fatalf("last automatic cleanup usage=%#v error=%v", usage, err)
	}
	app.shutdown(context.Background())
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatalf("completed current result should be cleaned at close: %v", err)
	}
}

func TestStorageAppBrowserProfileIsMeasuredAndNeverCleaned(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	app := NewApp()
	usage, err := app.GetStorageUsage()
	if err != nil {
		t.Fatal(err)
	}
	if usage.BrowserDataPath == "" {
		t.Skip("browser profile exists only on Windows")
	}
	profile := filepath.Join(usage.BrowserDataPath, "Default", "Local Storage")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(profile, "workspace-fixture")
	if err := os.WriteFile(workspace, []byte("saved-workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	usage, err = app.GetStorageUsage()
	if err != nil || usage.BrowserDataBytes != 15 || usage.BrowserDataUnavailable {
		t.Fatalf("profile usage=%#v error=%v", usage, err)
	}
	result, err := app.CleanStorage(StorageCleanupRequest{})
	if err != nil || result.FreedBytes != 0 || result.Usage.BrowserDataBytes != 15 {
		t.Fatalf("profile cleanup=%#v error=%v", result, err)
	}
	if content, err := os.ReadFile(workspace); err != nil || string(content) != "saved-workspace" {
		t.Fatalf("browser workspace altered: %q %v", content, err)
	}
}

func TestStorageAppSourceInputPromotionProtectsUserFilesAtClose(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	app := NewApp()
	_, settings, err := loadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	directory := writeStorageAppRunFixture(t, settings.Simulation.RunDirectory, "chosen-input")
	if err := app.SetStorageInputPath(filepath.Join(directory, "eplus.sql")); err != nil {
		t.Fatal(err)
	}
	settings.Storage.AutoClean = true
	if _, err := app.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	app.shutdown(context.Background())
	if _, err := os.Stat(directory); err != nil {
		t.Fatalf("chosen source disappeared at close: %v", err)
	}
	if err := app.SetStorageInputPath(""); err != nil {
		t.Fatal(err)
	}
	result, err := app.CleanStorage(StorageCleanupRequest{})
	if err != nil || result.RemovedRunCount != 0 || result.Usage.ProtectedRunCount != 1 {
		t.Fatalf("chosen source protection was not durable: %#v %v", result, err)
	}
}

func TestStorageAppDirectPurposeInputsSurviveAutomaticCleanup(t *testing.T) {
	for _, mode := range []string{"batch_paths", "batch_folder", "batch_blank_paths", "single", "plan"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("LOCALAPPDATA", t.TempDir())
			t.Setenv("APPDATA", t.TempDir())
			app := NewApp()
			_, settings, err := loadAppSettings()
			if err != nil {
				t.Fatal(err)
			}
			settings.Storage.AutoClean = true
			if _, err := app.SaveSettings(settings); err != nil {
				t.Fatal(err)
			}
			sourceDirectory := writeStorageAppRunFixture(t, settings.Simulation.RunDirectory, "selected-original")
			source := filepath.Join(sourceDirectory, "in.idf")
			before, err := app.GetStorageUsage()
			if err != nil || before.ReclaimableRunCount != 1 {
				t.Fatalf("source must start as a reclaimable generated input: %#v %v", before, err)
			}
			mainSource := filepath.Join(t.TempDir(), "main.idf")
			if err := app.SetStorageInputPath(mainSource); err != nil {
				t.Fatal(err)
			}
			// The test binary rejects EnergyPlus flags before running any tests. Its
			// failed execution still exercises purpose preparation, copied InputPath,
			// completed marker generation and the result/cleanup handoff.
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			purpose := simulation.SimulationPurposeRequest{Purposes: []simulation.SimulationPurposeID{simulation.SimulationPurposeBasicEnergy}, BasicEnergyDetail: simulation.PurposeBasicEnergyDetailLight}
			var run *simulation.SimulationRunResult
			switch mode {
			case "batch_paths", "batch_folder", "batch_blank_paths":
				request := simulation.MultiSimulationRequest{RunID: mode, EnergyPlusExecutablePath: executable, WorkerCount: 1, PurposeRequest: &purpose}
				if mode == "batch_paths" {
					request.InputPaths = []string{source}
				} else {
					request.RootDirectory = sourceDirectory
					if mode == "batch_blank_paths" {
						request.InputPaths = []string{"", "  ", "\t"}
					}
				}
				result, err := app.RunMultipleSimulations(request)
				if err != nil || result == nil || len(result.Results) != 1 {
					t.Fatalf("batch result=%#v error=%v", result, err)
				}
				run = &result.Results[0]
			case "single":
				run, err = app.RunSimulationText(simulation.SimulationRunRequest{RunID: mode, InputPath: source, EnergyPlusExecutablePath: executable, PurposeRequest: &purpose})
				if err != nil {
					t.Fatal(err)
				}
			case "plan":
				plan, err := app.BuildSimulationRunPlan(simulation.SimulationRunRequest{InputPath: source, PurposeRequest: &purpose})
				if err != nil || plan == nil {
					t.Fatalf("plan=%#v error=%v", plan, err)
				}
			}
			if mode != "plan" && (run == nil || run.OutputDirectory == "" || run.InputPath == source || filepath.Dir(run.InputPath) != run.OutputDirectory) {
				t.Fatalf("purpose execution did not replace InputPath with its generated copy: %#v", run)
			}
			if app.storageInputDirectory != filepath.Dir(mainSource) {
				t.Fatal("selection overwrote the main document reference")
			}
			app.applyStorageCleanupPolicy(settings)
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("automatic cleanup removed the original selected model: %v", err)
			}
			app.shutdown(context.Background())
			if run != nil {
				if _, err := os.Stat(run.OutputDirectory); !os.IsNotExist(err) {
					t.Fatalf("generated result should be released at close: %v", err)
				}
			}
			// Recreate App without the original RAM references: selected models must
			// remain user data across shutdown and future cleanup operations.
			fresh := NewApp()
			result, err := fresh.CleanStorage(StorageCleanupRequest{})
			if err != nil || result.RemovedRunCount != 0 || result.Usage.ProtectedRunCount != 1 {
				t.Fatalf("source protection was not durable: %#v %v", result, err)
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("selected source disappeared after restart: %v", err)
			}
		})
	}
}

func TestStorageAppBatchMetricsSelectionPreservesOriginalInputs(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	app := NewApp()
	_, settings, err := loadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	directory := writeStorageAppRunFixture(t, settings.Simulation.RunDirectory, "selected-for-metrics")
	source := filepath.Join(directory, "in.idf")
	app.storageMu.RLock()
	err = app.setStorageBatchInputPaths([]string{source, source})
	if err != nil {
		app.storageMu.RUnlock()
		t.Fatal(err)
	}
	metrics := analyzeBatchMetricsPaths([]string{source}, BatchMetricsRequest{RunID: "storage-metrics"}, nil)
	app.storageMu.RUnlock()
	if metrics.Completed != 1 || metrics.Succeeded != 1 || metrics.Files[0].Path != source {
		t.Fatalf("selected source analysis failed: %#v", metrics)
	}
	if len(app.storageBatchInputDirectories) != 1 || app.storageBatchInputDirectories[0] != directory {
		t.Fatalf("selected source references = %#v", app.storageBatchInputDirectories)
	}
	settings.Storage.AutoClean = true
	if _, err := app.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	app.shutdown(context.Background())
	if _, err := NewApp().CleanStorage(StorageCleanupRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("Batch Metrics selected source disappeared: %v", err)
	}
}

func TestStorageAppSourceAliasesPreserveTheirManagedTargets(t *testing.T) {
	for _, selection := range []string{"main", "batch", "single"} {
		for _, aliasType := range []string{"directory", "file"} {
			t.Run(selection+"_"+aliasType, func(t *testing.T) {
				t.Setenv("LOCALAPPDATA", t.TempDir())
				t.Setenv("APPDATA", t.TempDir())
				app := NewApp()
				_, settings, err := loadAppSettings()
				if err != nil {
					t.Fatal(err)
				}
				directory := writeStorageAppRunFixture(t, settings.Simulation.RunDirectory, "alias-original")
				source := filepath.Join(directory, "in.idf")
				alias := filepath.Join(t.TempDir(), "linked-model.idf")
				target := source
				if aliasType == "directory" {
					target = directory
				}
				if err := os.Symlink(target, alias); err != nil {
					t.Skipf("source alias creation is unavailable: %v", err)
				}
				if aliasType == "directory" {
					alias = filepath.Join(alias, "in.idf")
				}
				app.storageMu.RLock()
				switch selection {
				case "main":
					err = app.setStorageInputPath(alias)
				case "batch":
					err = app.setStorageBatchInputPaths([]string{alias})
				case "single":
					err = app.setStorageSingleSourcePath(alias)
				}
				app.storageMu.RUnlock()
				if err != nil {
					t.Fatal(err)
				}
				settings.Storage.AutoClean = true
				if _, err := app.SaveSettings(settings); err != nil {
					t.Fatal(err)
				}
				app.shutdown(context.Background())
				fresh := NewApp()
				result, err := fresh.CleanStorage(StorageCleanupRequest{})
				if err != nil || result.RemovedRunCount != 0 || result.Usage.ProtectedRunCount != 1 {
					t.Fatalf("source alias target lost durable protection: %#v %v", result, err)
				}
				if _, err := os.Stat(source); err != nil {
					t.Fatalf("alias target source disappeared: %v", err)
				}
			})
		}
	}
}

func TestStorageAppLoadedSQLAliasesRemainProtectedUntilClose(t *testing.T) {
	for _, aliasType := range []string{"directory", "file"} {
		t.Run(aliasType, func(t *testing.T) {
			t.Setenv("LOCALAPPDATA", t.TempDir())
			t.Setenv("APPDATA", t.TempDir())
			app := NewApp()
			_, settings, err := loadAppSettings()
			if err != nil {
				t.Fatal(err)
			}
			_, sql, externalInput := epath181CreateStoredRun(t)
			data, err := os.ReadFile(sql)
			if err != nil {
				t.Fatal(err)
			}
			directory := writeStorageAppRunFixtureFiles(t, settings.Simulation.RunDirectory, "opened-result", map[string][]byte{"eplus.sql": data})
			target := filepath.Join(directory, "eplus.sql")
			alias := filepath.Join(t.TempDir(), "linked-result")
			if aliasType == "directory" {
				target = directory
			}
			if err := os.Symlink(target, alias); err != nil {
				t.Skipf("SQL alias creation is unavailable: %v", err)
			}
			if aliasType == "directory" {
				alias = filepath.Join(alias, "eplus.sql")
			}
			projection, err := app.LoadEnergyPath(simulation.EnergyPathProjectionRequest{ResultPath: alias, InputPath: externalInput})
			if err != nil || projection.Provenance == nil {
				t.Fatalf("stored SQL alias load failed: %#v %v", projection.Provenance, err)
			}
			settings.Storage.AutoClean = true
			if _, err := app.SaveSettings(settings); err != nil {
				t.Fatal(err)
			}
			result, err := app.CleanStorage(StorageCleanupRequest{})
			if err != nil || result.RemovedRunCount != 0 || result.Usage.ProtectedRunCount != 1 {
				t.Fatalf("loaded SQL alias target lost its reference: %#v %v", result, err)
			}
			app.shutdown(context.Background())
			if _, err := os.Stat(directory); !os.IsNotExist(err) {
				t.Fatalf("closed result alias must release its generated bundle: %v", err)
			}
		})
	}
}
