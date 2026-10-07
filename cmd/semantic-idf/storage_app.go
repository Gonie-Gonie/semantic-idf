package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type StorageUsage struct {
	simulation.RunStorageUsage
	BrowserDataPath        string                 `json:"browserDataPath"`
	BrowserDataBytes       int64                  `json:"browserDataBytes"`
	BrowserDataUnavailable bool                   `json:"browserDataUnavailable"`
	LastCleanup            *StorageCleanupSummary `json:"lastCleanup,omitempty"`
}

type StorageCleanupSummary struct {
	CompletedAt     string                    `json:"completedAt"`
	Automatic       bool                      `json:"automatic"`
	FreedBytes      int64                     `json:"freedBytes"`
	RemovedRunCount int                       `json:"removedRunCount"`
	SkippedRunCount int                       `json:"skippedRunCount"`
	FailureCount    int                       `json:"failureCount"`
	Failures        []simulation.StorageIssue `json:"failures"`
}

type StorageCleanupRequest = simulation.RunStorageCleanupRequest

type StorageCleanupResult struct {
	FreedBytes      int64                     `json:"freedBytes"`
	RemovedRunCount int                       `json:"removedRunCount"`
	SkippedRunCount int                       `json:"skippedRunCount"`
	Failures        []simulation.StorageIssue `json:"failures"`
	Usage           StorageUsage              `json:"usage"`
}

func (a *App) GetStorageUsage() (*StorageUsage, error) {
	_, settings, err := loadAppSettings()
	if err != nil {
		return nil, err
	}
	usage := a.storageUsage(settings.Simulation)
	return &usage, nil
}

func (a *App) CleanStorage(request StorageCleanupRequest) (*StorageCleanupResult, error) {
	// Do not block the Settings window behind a long EnergyPlus run or SQL read.
	// A retry after the operation finishes keeps its outputs and result handoff
	// protected as one transaction.
	if !a.storageMu.TryLock() {
		return nil, fmt.Errorf("storage is in use by a simulation or result operation; retry after it finishes")
	}
	defer a.storageMu.Unlock()
	if err := a.storageRegistrationError(); err != nil {
		return nil, fmt.Errorf("storage cleanup is unavailable: %w", err)
	}
	_, settings, err := loadAppSettings()
	if err != nil {
		return nil, err
	}
	var result simulation.RunStorageCleanupResult
	err = simulation.WithStorageCleanupGate(func() error {
		var cleanErr error
		result, cleanErr = simulation.CleanRunStorage(settings.Simulation, request, a.protectedStorageDirectories())
		return cleanErr
	})
	if err != nil {
		return nil, err
	}
	a.recordStorageCleanup(result, false)
	usage := StorageUsage{RunStorageUsage: result.Usage}
	a.decorateStorageUsage(&usage)
	return &StorageCleanupResult{FreedBytes: result.FreedBytes, RemovedRunCount: result.RemovedRunCount, SkippedRunCount: result.SkippedRunCount, Failures: result.Failures, Usage: usage}, nil
}

func (a *App) SelectSimulationRunDirectory() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("desktop runtime is not ready")
	}
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "Select simulation run directory", CanCreateDirectories: true})
}

func (a *App) SetStorageInputPath(path string) error {
	if err := a.ensureStorageInstance(); err != nil {
		return err
	}
	a.storageMu.RLock()
	err := a.setStorageInputPath(path)
	a.storageMu.RUnlock()
	if err == nil {
		a.applyStartupStorageCleanupPolicy()
	}
	return err
}

func (a *App) setStorageInputPath(path string) error {
	directories, err := storageSourceDirectories([]string{path})
	if err != nil {
		return err
	}
	directory := ""
	if len(directories) != 0 {
		directory = directories[len(directories)-1]
	}
	a.storageReferencesMu.Lock()
	a.storageInputDirectory = directory
	a.storageInputDirectories = directories
	a.storageReferencesMu.Unlock()
	return preserveStorageSourceDirectories(directories)
}

func (a *App) setStorageBatchInputPaths(paths []string) error {
	directories, err := storageSourceDirectories(paths)
	if err != nil {
		return err
	}
	// Preserve references even if durable promotion fails, so automatic cleanup
	// after the failed operation cannot remove the selected original models.
	a.storageReferencesMu.Lock()
	a.storageBatchInputDirectories = directories
	a.storageReferencesMu.Unlock()
	return preserveStorageSourceDirectories(directories)
}

func (a *App) setStorageSingleSourcePath(path string) error {
	directories, err := storageSourceDirectories([]string{path})
	if err != nil {
		return err
	}
	a.storageReferencesMu.Lock()
	a.storageSingleSourceDirectory = ""
	a.storageSingleSourceDirectories = directories
	if len(directories) != 0 {
		a.storageSingleSourceDirectory = directories[len(directories)-1]
	}
	a.storageReferencesMu.Unlock()
	return preserveStorageSourceDirectories(directories)
}

func storageSourceDirectories(paths []string) ([]string, error) {
	return simulation.ResolveStorageSourceDirectories(paths)
}

func preserveStorageSourceDirectories(directories []string) error {
	for _, directory := range directories {
		if err := simulation.PreserveRunStorageDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) storageUsage(settings simulation.SimulationSettings) StorageUsage {
	usage := StorageUsage{RunStorageUsage: simulation.InspectRunStorage(settings, a.protectedStorageDirectories())}
	a.decorateStorageUsage(&usage)
	return usage
}

func (a *App) protectedStorageDirectories() []string {
	a.simulationWorkspaceCache.mu.RLock()
	current := a.simulationWorkspaceCache.outputDirectory
	input := a.simulationWorkspaceCache.inputPath
	a.simulationWorkspaceCache.mu.RUnlock()
	a.storageReferencesMu.Lock()
	paths := append([]string(nil), a.storageBatchDirectories...)
	paths = append(paths, a.storageBatchInputDirectories...)
	paths = append(paths, a.storageLoadedDirectories...)
	paths = append(paths, a.storageInputDirectories...)
	paths = append(paths, a.storageSingleSourceDirectories...)
	paths = append(paths, a.storageLoadedDirectory, current)
	if a.storageSingleDirectory != "" {
		paths = append(paths, a.storageSingleDirectory)
	}
	if a.storageSingleInputDirectory != "" {
		paths = append(paths, a.storageSingleInputDirectory)
	}
	if a.storageSingleSourceDirectory != "" {
		paths = append(paths, a.storageSingleSourceDirectory)
	}
	if a.storageLoadedInputDirectory != "" {
		paths = append(paths, a.storageLoadedInputDirectory)
	}
	if a.storageInputDirectory != "" {
		paths = append(paths, a.storageInputDirectory)
	}
	if input != "" {
		paths = append(paths, filepath.Dir(input))
	}
	a.storageReferencesMu.Unlock()
	return paths
}

func (a *App) applyStoredStorageCleanupPolicy() {
	if a.ctx == nil {
		return
	}
	_, settings, err := loadAppSettings()
	if err == nil {
		a.applyStorageCleanupPolicy(settings)
	}
}

func (a *App) applyStartupStorageCleanupPolicy() {
	if a.ctx == nil {
		return
	}
	a.storageReferencesMu.Lock()
	if a.storageInputInitialized {
		a.storageReferencesMu.Unlock()
		return
	}
	a.storageInputInitialized = true
	a.storageReferencesMu.Unlock()
	a.applyStoredStorageCleanupPolicy()
}

func (a *App) applyStorageCleanupPolicy(_ AppSettings) {
	if !a.storageMu.TryLock() {
		return
	}
	defer a.storageMu.Unlock()
	_, settings, err := loadAppSettings()
	if err != nil || !settings.Storage.AutoClean || a.storageRegistrationError() != nil {
		return
	}
	a.runAutomaticStorageCleanup(settings, a.protectedStorageDirectories())
}

func (a *App) shutdown(_ context.Context) {
	if !a.storageMu.TryLock() {
		// Keep presence until process exit if an in-flight operation still uses
		// files, so a second instance also postpones its cleanup.
		return
	}
	defer a.storageMu.Unlock()
	if a.storageRegistrationError() == nil {
		if err := simulation.UnregisterStorageInstance(); err != nil {
			return
		}
	}
	_, settings, err := loadAppSettings()
	if err != nil || !settings.Storage.AutoClean || a.storageRegistrationError() != nil {
		return
	}
	// The windows are closing; their completed snapshots no longer need files.
	// Active runs and another running app instance remain protected by the runner.
	a.storageReferencesMu.Lock()
	inputs := append([]string(nil), a.storageBatchInputDirectories...)
	inputs = append(inputs, a.storageInputDirectories...)
	inputs = append(inputs, a.storageSingleSourceDirectories...)
	inputs = append(inputs, a.storageInputDirectory, a.storageSingleSourceDirectory)
	a.storageReferencesMu.Unlock()
	a.runAutomaticStorageCleanup(settings, inputs)
}

func (a *App) runAutomaticStorageCleanup(settings AppSettings, protected []string) {
	var result simulation.RunStorageCleanupResult
	err := simulation.WithStorageCleanupGate(func() error {
		var cleanErr error
		result, cleanErr = simulation.CleanRunStorage(settings.Simulation, StorageCleanupRequest{}, protected)
		return cleanErr
	})
	if err != nil {
		result.Failures = []simulation.StorageIssue{{Reason: err.Error()}}
	}
	a.recordStorageCleanup(result, true)
}

func (a *App) recordStorageCleanup(result simulation.RunStorageCleanupResult, automatic bool) {
	failures := append([]simulation.StorageIssue(nil), result.Failures...)
	if len(failures) > 32 {
		failures = failures[:32]
	}
	summary := &StorageCleanupSummary{CompletedAt: time.Now().Format(time.RFC3339), Automatic: automatic, FreedBytes: result.FreedBytes, RemovedRunCount: result.RemovedRunCount, SkippedRunCount: result.SkippedRunCount, FailureCount: len(result.Failures), Failures: failures}
	a.storageReferencesMu.Lock()
	a.storageLastCleanup = summary
	a.storageReferencesMu.Unlock()
}

func (a *App) decorateStorageUsage(usage *StorageUsage) {
	addBrowserStorageUsage(usage)
	a.storageReferencesMu.Lock()
	if a.storageLastCleanup != nil {
		copy := *a.storageLastCleanup
		copy.Failures = append([]simulation.StorageIssue(nil), copy.Failures...)
		usage.LastCleanup = &copy
	}
	a.storageReferencesMu.Unlock()
	if a.storageRegistrationError() != nil {
		usage.Warnings = append(usage.Warnings, simulation.StorageIssue{Reason: "storage_instance_registration_failed"})
	}
}

func (a *App) storageRegistrationError() error {
	a.storageReferencesMu.Lock()
	defer a.storageReferencesMu.Unlock()
	return a.storageInstanceError
}

func (a *App) ensureStorageInstance() error {
	if a.storageRegistrationError() == nil {
		return nil
	}
	err := simulation.RegisterStorageInstance()
	a.storageReferencesMu.Lock()
	a.storageInstanceError = err
	a.storageReferencesMu.Unlock()
	if err != nil {
		return fmt.Errorf("storage instance registration is unavailable: %w", err)
	}
	return nil
}

func addBrowserStorageUsage(usage *StorageUsage) {
	// Match go-webview2's existing default exactly, including the .exe suffix.
	// This profile also owns Settings/workspace localStorage, so it is read-only.
	appData := strings.TrimSpace(os.Getenv("APPDATA"))
	executable, err := os.Executable()
	if runtime.GOOS != "windows" || appData == "" || err != nil {
		usage.BrowserDataUnavailable = true
		return
	}
	usage.BrowserDataPath = filepath.Join(appData, filepath.Base(executable))
	bytes, issue := simulation.MeasureStorageDirectory(usage.BrowserDataPath)
	usage.BrowserDataBytes = bytes
	if issue != nil {
		usage.BrowserDataUnavailable = true
		usage.Warnings = append(usage.Warnings, *issue)
	}
}
