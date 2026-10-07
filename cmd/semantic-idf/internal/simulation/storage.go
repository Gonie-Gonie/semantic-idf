package simulation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	runStorageMarkerName = ".semantic-idf-storage.json"
	runStorageSchema     = "semantic-idf.run-storage/v1"
	maxStorageEntries    = 100000
	maxStorageRuns       = 10000
	maxStorageJSONBytes  = 8 << 20
)

// Run directories are managed only when the runner created an empty directory.
// Explicit output directories and unknown files never become cleanup targets.
type runStorageMarker struct {
	Schema             string                      `json:"schema"`
	Root               string                      `json:"root"`
	Directory          string                      `json:"directory"`
	RunID              string                      `json:"runId"`
	ProcessID          int                         `json:"processId"`
	Managed            bool                        `json:"managed"`
	ProtectedReason    string                      `json:"protectedReason,omitempty"`
	CreatedAt          string                      `json:"createdAt"`
	FinishedAt         string                      `json:"finishedAt,omitempty"`
	GeneratedFiles     []string                    `json:"generatedFiles,omitempty"`
	GeneratedFileState map[string]storageFileState `json:"generatedFileState"`
}

type storageFileState struct {
	Size       int64 `json:"size"`
	ModifiedAt int64 `json:"modifiedAt"`
}

type StorageIssue struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type RunStorageRootUsage struct {
	Path                string `json:"path"`
	IsDefault           bool   `json:"isDefault"`
	TotalBytes          int64  `json:"totalBytes"`
	ReclaimableBytes    int64  `json:"reclaimableBytes"`
	ProtectedBytes      int64  `json:"protectedBytes"`
	RunCount            int    `json:"runCount"`
	ReclaimableRunCount int    `json:"reclaimableRunCount"`
	ProtectedRunCount   int    `json:"protectedRunCount"`
}

type RunStorageUsage struct {
	ScannedAt           string                `json:"scannedAt"`
	TotalBytes          int64                 `json:"totalBytes"`
	ReclaimableBytes    int64                 `json:"reclaimableBytes"`
	ProtectedBytes      int64                 `json:"protectedBytes"`
	RunCount            int                   `json:"runCount"`
	ReclaimableRunCount int                   `json:"reclaimableRunCount"`
	ProtectedRunCount   int                   `json:"protectedRunCount"`
	Roots               []RunStorageRootUsage `json:"roots"`
	Warnings            []StorageIssue        `json:"warnings"`
}

type RunStorageCleanupRequest struct {
	OlderThanDays              int      `json:"olderThanDays"`
	ProtectedOutputDirectories []string `json:"protectedOutputDirectories,omitempty"`
}

type RunStorageCleanupResult struct {
	FreedBytes      int64           `json:"freedBytes"`
	RemovedRunCount int             `json:"removedRunCount"`
	SkippedRunCount int             `json:"skippedRunCount"`
	Failures        []StorageIssue  `json:"failures"`
	Usage           RunStorageUsage `json:"usage"`
}

var runStorageState = struct {
	sync.Mutex
	active map[string]int
}{active: make(map[string]int)}

type storageCandidate struct {
	root       string
	path       string
	bytes      int64
	files      []string
	fileStates map[string]storageFileState
	finished   time.Time
	reason     string
	legacy     bool
}

type storageScanBudget struct {
	entries  int
	deadline time.Time
}

func (b *storageScanBudget) take() bool {
	b.entries++
	return b.entries <= maxStorageEntries && time.Now().Before(b.deadline)
}

// beginRunStorage serializes preparation against cleanup and protects the output
// until EnergyPlus execution and all result reading have finished.
func beginRunStorage(directory, root, runID, inputCopyName string, automatic bool) (func(), error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if automatic {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return nil, err
		}
		if err := rememberRunStorageRoot(root); err != nil {
			return nil, fmt.Errorf("record simulation storage location: %w", err)
		}
	}
	runStorageState.Lock()
	defer runStorageState.Unlock()
	if err := os.MkdirAll(filepath.Dir(directory), 0o755); err != nil {
		return nil, err
	}
	// Atomic child creation distinguishes our empty directory from one another
	// process created between an existence check and MkdirAll.
	mkdirErr := os.Mkdir(directory, 0o755)
	created := mkdirErr == nil
	if mkdirErr != nil {
		if !errors.Is(mkdirErr, os.ErrExist) {
			return nil, mkdirErr
		}
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("simulation output directory is unavailable")
		}
	}
	key := storagePathKey(directory)
	runStorageState.active[key]++
	marked := automatic && created && storageDirectChild(root, directory) && validateStoragePath(directory) == nil
	marker := runStorageMarker{Schema: runStorageSchema, Root: root, Directory: directory, RunID: runID, ProcessID: os.Getpid(), Managed: true, CreatedAt: time.Now().Format(time.RFC3339)}
	if !marked {
		// Never replace a pre-existing user file merely because its name is
		// reserved. Only verified ownership metadata can be invalidated.
		var existing runStorageMarker
		if readStorageJSON(filepath.Join(directory, runStorageMarkerName), &existing) == nil && validRunStorageMarker(root, directory, existing) {
			existing.Managed = false
			_ = writeRunStorageMarker(directory, existing)
		}
	}
	if marked {
		if err := writeRunStorageMarker(directory, marker); err != nil {
			// Running remains possible, but unmarked results remain protected.
			marked = false
		}
	}
	return func() {
		appRoot, sessions, gateErr := storageInstanceDirectory()
		var release func()
		if gateErr == nil {
			release, gateErr = acquireStorageInstanceGate(appRoot, sessions, 5*time.Second)
		}
		if gateErr == nil {
			defer release()
		}
		runStorageState.Lock()
		defer runStorageState.Unlock()
		if gateErr != nil {
			marked = false
		}
		if marked && validateStoragePath(directory) == nil {
			var current runStorageMarker
			if readStorageJSON(filepath.Join(directory, runStorageMarkerName), &current) != nil || current.Schema != marker.Schema || current.RunID != marker.RunID || current.CreatedAt != marker.CreatedAt || current.ProcessID != marker.ProcessID || !validRunStorageMarker(root, directory, current) {
				marked = false
			} else {
				marker.Managed = current.Managed
				marker.ProtectedReason = current.ProtectedReason
			}
		}
		if marked && validateStoragePath(directory) == nil {
			entries, err := os.ReadDir(directory)
			if err == nil {
				marker.GeneratedFileState = make(map[string]storageFileState)
				for _, entry := range entries {
					if entry.Type().IsRegular() && generatedStorageFilename(entry.Name(), inputCopyName) {
						marker.GeneratedFiles = append(marker.GeneratedFiles, entry.Name())
						if entry.Name() != runStorageMarkerName {
							if info, err := entry.Info(); err == nil {
								marker.GeneratedFileState[entry.Name()] = storageFileState{Size: info.Size(), ModifiedAt: info.ModTime().UnixNano()}
							}
						}
					}
				}
				marker.FinishedAt = time.Now().Format(time.RFC3339)
				_ = writeRunStorageMarker(directory, marker)
			}
		}
		runStorageState.active[key]--
		if runStorageState.active[key] <= 0 {
			delete(runStorageState.active, key)
		}
	}, nil
}

// A run-copy explicitly selected as a model is user data from that point on.
// Promote only verified app metadata; ordinary source folders remain untouched.
func PreserveRunStorageDirectory(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	markerPath := filepath.Join(absolute, runStorageMarkerName)
	var marker runStorageMarker
	marked := readStorageJSON(markerPath, &marker) == nil && validRunStorageMarker(marker.Root, absolute, marker)
	legacy := false
	manifestPath := filepath.Join(absolute, "semantic-idf-run.json")
	var manifest SimulationRunManifest
	if !marked && storageDirectChild(DefaultSettings().RunDirectory, absolute) {
		legacy = readStorageJSON(manifestPath, &manifest) == nil && validLegacyStorageManifest(absolute, manifest)
	}
	if !marked && !legacy {
		return nil
	}
	appRoot, sessions, err := storageInstanceDirectory()
	if err != nil {
		return err
	}
	release, err := acquireStorageInstanceGate(appRoot, sessions, 5*time.Second)
	if err != nil {
		return err
	}
	defer release()
	runStorageState.Lock()
	defer runStorageState.Unlock()
	if marked {
		if err := readStorageJSON(markerPath, &marker); err != nil {
			return err
		}
		if !validRunStorageMarker(marker.Root, absolute, marker) {
			return fmt.Errorf("simulation storage ownership changed")
		}
		marker.Managed = false
		marker.ProtectedReason = "user_input"
		return writeRunStorageMarker(absolute, marker)
	}
	if err := readStorageJSON(manifestPath, &manifest); err != nil {
		return err
	}
	if !validLegacyStorageManifest(absolute, manifest) {
		return fmt.Errorf("simulation storage ownership changed")
	}
	var fields map[string]json.RawMessage
	if err := readStorageJSON(manifestPath, &fields); err != nil {
		return err
	}
	fields["storageManaged"] = json.RawMessage("false")
	payload, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	if err := validateStoragePath(manifestPath); err != nil {
		return err
	}
	return os.WriteFile(manifestPath, append(payload, '\n'), 0o644)
}

func generatedStorageFilename(name, inputCopyName string) bool {
	if name == inputCopyName && name != "" {
		return true
	}
	switch name {
	case runStorageMarkerName, "semantic-idf-run.json", "semantic-idf-run-plan.json", "temporary_outputs.diff":
		return true
	}
	if !strings.HasPrefix(strings.ToLower(name), "eplus") {
		return false
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".sql", ".sql-journal", ".sql-shm", ".sql-wal", ".csv", ".err", ".eso", ".mtr", ".rdd", ".mdd", ".audit", ".bnd", ".eio", ".json", ".htm", ".html", ".xml", ".dxf", ".svg", ".shd", ".out", ".end", ".edd", ".sln", ".dbg", ".sci", ".map", ".screen", ".delightout", ".dfs", ".wrl", ".ssz", ".zsz", ".spsz":
		return true
	}
	return false
}

func writeRunStorageMarker(directory string, marker runStorageMarker) error {
	payload, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, runStorageMarkerName), append(payload, '\n'), 0o600)
}

func InspectRunStorage(settings SimulationSettings, protected []string) RunStorageUsage {
	usage, _ := inspectRunStorage(settings, protected)
	return usage
}

// MeasureStorageDirectory counts logical file lengths without reading file
// contents or following links. A bounded, partial scan reports its limitation.
func MeasureStorageDirectory(path string) (int64, *StorageIssue) {
	if err := validateStoragePath(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, &StorageIssue{path, "unsafe_or_unreadable_path"}
	}
	budget := &storageScanBudget{deadline: time.Now().Add(2 * time.Second)}
	var bytes int64
	err := filepath.WalkDir(path, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !budget.take() {
			return fmt.Errorf("scan_limit_reached")
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if storagePathIsLink(current, info) {
			return fmt.Errorf("unsafe_or_unreadable_path")
		}
		if info.Mode().IsRegular() {
			bytes += info.Size()
		}
		return nil
	})
	if err != nil {
		return bytes, &StorageIssue{path, err.Error()}
	}
	return bytes, nil
}

func inspectRunStorage(settings SimulationSettings, protected []string) (RunStorageUsage, []storageCandidate) {
	usage := RunStorageUsage{ScannedAt: time.Now().Format(time.RFC3339), Roots: []RunStorageRootUsage{}, Warnings: []StorageIssue{}}
	protectedKeys := make(map[string]bool, len(protected))
	for _, path := range protected {
		if strings.TrimSpace(path) != "" {
			protectedKeys[storagePathKey(path)] = true
		}
	}
	runStorageState.Lock()
	for path := range runStorageState.active {
		protectedKeys[path] = true
	}
	runStorageState.Unlock()
	defaultRoot := DefaultSettings().RunDirectory
	roots := []string{defaultRoot}
	if strings.TrimSpace(settings.RunDirectory) != "" && storagePathKey(settings.RunDirectory) != storagePathKey(defaultRoot) {
		roots = append(roots, settings.RunDirectory)
	}
	known, issue := knownRunStorageRoots()
	if issue != nil {
		usage.Warnings = append(usage.Warnings, *issue)
	}
	seenRoots := make(map[string]bool, len(roots)+len(known))
	for _, root := range roots {
		seenRoots[storagePathKey(root)] = true
	}
	for _, root := range known {
		if !seenRoots[storagePathKey(root)] {
			roots = append(roots, root)
			seenRoots[storagePathKey(root)] = true
		}
	}
	budget := &storageScanBudget{deadline: time.Now().Add(5 * time.Second)}
	candidates := []storageCandidate{}
	for _, root := range roots {
		absolute, err := filepath.Abs(root)
		if err != nil {
			usage.Warnings = append(usage.Warnings, StorageIssue{root, err.Error()})
			continue
		}
		row := RunStorageRootUsage{Path: absolute, IsDefault: storagePathKey(root) == storagePathKey(defaultRoot)}
		if err := validateStoragePath(absolute); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				usage.Warnings = append(usage.Warnings, StorageIssue{absolute, "unsafe_or_unreadable_path"})
			}
			usage.Roots = append(usage.Roots, row)
			continue
		}
		dir, err := os.Open(absolute)
		if err != nil {
			usage.Warnings = append(usage.Warnings, StorageIssue{absolute, err.Error()})
			usage.Roots = append(usage.Roots, row)
			continue
		}
		entriesSeen := 0
		for {
			entries, readErr := dir.ReadDir(128)
			for _, entry := range entries {
				entriesSeen++
				if entriesSeen > maxStorageRuns || !budget.take() {
					usage.Warnings = append(usage.Warnings, StorageIssue{absolute, "scan_limit_reached"})
					readErr = io.EOF
					break
				}
				path := filepath.Join(absolute, entry.Name())
				if !entry.IsDir() {
					continue
				}
				candidate, recognized := inspectStorageCandidate(absolute, path, row.IsDefault, budget)
				if !recognized {
					// A custom root may contain arbitrary user folders. Inventory
					// only marked runs there, never the whole user's directory.
					if !row.IsDefault {
						continue
					}
					if candidate.reason == "" || candidate.reason == "unfinished_or_unknown_run" {
						candidate.reason = "unknown_ownership"
					}
				}
				if protectedKeys[storagePathKey(path)] {
					candidate.reason = "in_use"
				}
				row.RunCount++
				row.TotalBytes += candidate.bytes
				if candidate.reason == "" {
					row.ReclaimableRunCount++
					row.ReclaimableBytes += candidate.bytes
				} else {
					row.ProtectedRunCount++
					row.ProtectedBytes += candidate.bytes
					usage.Warnings = append(usage.Warnings, StorageIssue{path, candidate.reason})
				}
				candidates = append(candidates, candidate)
			}
			if readErr != nil {
				if readErr != io.EOF {
					usage.Warnings = append(usage.Warnings, StorageIssue{absolute, readErr.Error()})
				}
				break
			}
		}
		_ = dir.Close()
		usage.TotalBytes += row.TotalBytes
		usage.ReclaimableBytes += row.ReclaimableBytes
		usage.ProtectedBytes += row.ProtectedBytes
		usage.RunCount += row.RunCount
		usage.ReclaimableRunCount += row.ReclaimableRunCount
		usage.ProtectedRunCount += row.ProtectedRunCount
		usage.Roots = append(usage.Roots, row)
	}
	return usage, candidates
}

func inspectStorageCandidate(root, path string, legacyAllowed bool, budget *storageScanBudget) (storageCandidate, bool) {
	result := storageCandidate{root: root, path: path, fileStates: make(map[string]storageFileState)}
	if err := validateStoragePath(path); err != nil {
		result.reason = "unsafe_or_unreadable_path"
		return result, true
	}
	allowed := map[string]bool{}
	var states map[string]storageFileState
	recognized := false
	var marker runStorageMarker
	markerPath := filepath.Join(path, runStorageMarkerName)
	_, markerStatErr := os.Lstat(markerPath)
	if readStorageJSON(markerPath, &marker) == nil && validRunStorageMarker(root, path, marker) {
		recognized = true
		result.finished, _ = time.Parse(time.RFC3339, marker.FinishedAt)
		if !marker.Managed {
			result.reason = "explicit_output_directory"
			if marker.ProtectedReason != "" {
				result.reason = marker.ProtectedReason
			}
		} else if marker.ProcessID > 0 && marker.ProcessID != os.Getpid() && storageProcessIsRunning(marker.ProcessID) {
			result.reason = "another_app_instance"
		}
		for _, name := range marker.GeneratedFiles {
			if filepath.Base(name) == name && name != "." && name != ".." {
				allowed[name] = true
			}
		}
		allowed[runStorageMarkerName] = true
		states = marker.GeneratedFileState
	} else if legacyAllowed && errors.Is(markerStatErr, os.ErrNotExist) {
		var manifest SimulationRunManifest
		if readStorageJSON(filepath.Join(path, "semantic-idf-run.json"), &manifest) == nil && validLegacyStorageManifest(path, manifest) {
			recognized = true
			result.legacy = true
			result.finished, _ = time.Parse(time.RFC3339, manifest.FinishedAt)
			allowed["semantic-idf-run.json"] = true
			allowed["semantic-idf-run-plan.json"] = true
			allowed["temporary_outputs.diff"] = true
			inputCopyName := ""
			if storagePathKey(filepath.Dir(manifest.InputPath)) == storagePathKey(path) {
				inputCopyName = filepath.Base(manifest.InputPath)
				allowed[filepath.Base(manifest.InputPath)] = true
			}
			for _, file := range manifest.ResultFiles {
				if filepath.Base(file.Name) == file.Name && file.Name != "." && file.Name != ".." && storagePathKey(file.Path) == storagePathKey(filepath.Join(path, file.Name)) && generatedStorageFilename(file.Name, inputCopyName) {
					allowed[file.Name] = true
				}
			}
			if storagePathKey(filepath.Dir(manifest.InputPath)) == storagePathKey(path) && !storageInputHashMatches(manifest.InputPath, manifest.InputHash, budget.deadline) {
				result.reason = "unrecognized_files"
			}
		}
	}
	if !recognized && !legacyAllowed {
		return result, false
	}
	if result.finished.IsZero() && result.reason == "" {
		result.reason = "unfinished_or_unknown_run"
	}
	err := filepath.WalkDir(path, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !budget.take() {
			return fmt.Errorf("scan_limit_reached")
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if storagePathIsLink(current, info) {
			result.reason = "unsafe_or_unreadable_path"
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if current == path {
			return nil
		}
		if entry.IsDir() {
			result.reason = "unrecognized_files"
			return nil
		}
		if info.Mode().IsRegular() {
			result.bytes += info.Size()
		}
		if filepath.Dir(current) != path || !info.Mode().IsRegular() || !allowed[entry.Name()] {
			result.reason = "unrecognized_files"
			return nil
		}
		if states != nil && entry.Name() != runStorageMarkerName {
			expected, ok := states[entry.Name()]
			if !ok || expected.Size != info.Size() || expected.ModifiedAt != info.ModTime().UnixNano() {
				result.reason = "unrecognized_files"
			}
		}
		result.files = append(result.files, current)
		result.fileStates[current] = storageFileState{Size: info.Size(), ModifiedAt: info.ModTime().UnixNano()}
		return nil
	})
	if err != nil {
		result.reason = err.Error()
	}
	return result, recognized
}

func storageInputHashMatches(path, expected string, deadline time.Time) bool {
	if !time.Now().Before(deadline) || validateStoragePath(path) != nil {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 128<<20 {
		return false
	}
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	for {
		if !time.Now().Before(deadline) {
			return false
		}
		n, err := file.Read(buffer)
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
		}
		if err == io.EOF {
			return strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expected)
		}
		if err != nil {
			return false
		}
	}
}

func validRunStorageMarker(root, path string, marker runStorageMarker) bool {
	if marker.Schema != runStorageSchema || marker.RunID == "" || storagePathKey(marker.Root) != storagePathKey(root) || storagePathKey(marker.Directory) != storagePathKey(path) {
		return false
	}
	created, err := time.Parse(time.RFC3339, marker.CreatedAt)
	if err != nil || created.After(time.Now().Add(time.Minute)) {
		return false
	}
	if marker.FinishedAt != "" {
		finished, err := time.Parse(time.RFC3339, marker.FinishedAt)
		if err != nil || finished.Before(created) || finished.After(time.Now().Add(time.Minute)) {
			return false
		}
		if marker.GeneratedFileState == nil {
			return false
		}
	}
	return true
}

func validLegacyStorageManifest(path string, manifest SimulationRunManifest) bool {
	if manifest.StorageManaged != nil && !*manifest.StorageManaged {
		return false
	}
	if manifest.RunID == "" || manifest.Filename == "" || storagePathKey(manifest.OutputDirectory) != storagePathKey(path) || (manifest.Status != "succeeded" && manifest.Status != "failed") {
		return false
	}
	hash, err := hex.DecodeString(manifest.InputHash)
	if err != nil || len(hash) != 32 {
		return false
	}
	started, startErr := time.Parse(time.RFC3339, manifest.StartedAt)
	finished, finishErr := time.Parse(time.RFC3339, manifest.FinishedAt)
	_, createdErr := time.Parse(time.RFC3339, manifest.CreatedAt)
	if startErr != nil || finishErr != nil || createdErr != nil || finished.Before(started) {
		return false
	}
	name := filepath.Base(path)
	if len(name) <= 16 || name[15] != '-' {
		return false
	}
	prepared, err := time.ParseInLocation("20060102-150405", name[:15], time.Local)
	if err != nil || prepared.Before(started.Add(-time.Second)) || prepared.After(finished.Add(time.Second)) {
		return false
	}
	label := strings.TrimSuffix(manifest.Filename, filepath.Ext(manifest.Filename))
	return name[16:] == sanitizePathSegment(manifest.RunID+"-"+label)
}

func CleanRunStorage(settings SimulationSettings, request RunStorageCleanupRequest, protected []string) (RunStorageCleanupResult, error) {
	if request.OlderThanDays < 0 || request.OlderThanDays > 36500 {
		return RunStorageCleanupResult{}, fmt.Errorf("olderThanDays must be between 0 and 36500")
	}
	deadline := time.Now().Add(10 * time.Second)
	protected = append(append([]string(nil), protected...), request.ProtectedOutputDirectories...)
	_, candidates := inspectRunStorage(settings, protected)
	result := RunStorageCleanupResult{Failures: []StorageIssue{}}
	cutoff := time.Now().Add(-time.Duration(request.OlderThanDays) * 24 * time.Hour)
	for index, candidate := range candidates {
		if !time.Now().Before(deadline) {
			result.SkippedRunCount += len(candidates) - index
			result.Failures = append(result.Failures, StorageIssue{Reason: "cleanup_limit_reached"})
			break
		}
		if candidate.reason != "" || candidate.finished.After(cutoff) {
			result.SkippedRunCount++
			continue
		}
		runStorageState.Lock()
		if runStorageState.active[storagePathKey(candidate.path)] > 0 {
			result.SkippedRunCount++
			runStorageState.Unlock()
			continue
		}
		fresh, recognized := inspectStorageCandidate(candidate.root, candidate.path, storagePathKey(candidate.root) == storagePathKey(DefaultSettings().RunDirectory), &storageScanBudget{deadline: deadline})
		if !recognized || fresh.reason != "" || fresh.finished.After(cutoff) {
			result.SkippedRunCount++
			runStorageState.Unlock()
			continue
		}
		createdBytes := int64(0)
		if fresh.legacy {
			var upgradeErr error
			fresh, createdBytes, upgradeErr = upgradeLegacyStorageCandidate(fresh, deadline)
			if upgradeErr != nil {
				result.Failures = append(result.Failures, StorageIssue{candidate.path, upgradeErr.Error()})
				runStorageState.Unlock()
				continue
			}
		}
		freed, err := removeStorageCandidate(fresh, deadline)
		result.FreedBytes += freed - createdBytes
		if err != nil {
			result.Failures = append(result.Failures, StorageIssue{candidate.path, err.Error()})
		} else {
			result.RemovedRunCount++
		}
		runStorageState.Unlock()
	}
	result.FreedBytes = max(0, result.FreedBytes)
	result.Usage = InspectRunStorage(settings, protected)
	return result, nil
}

// Legacy ownership depended on an input hash. Keep independent ownership
// evidence before removing that input, so a locked later file can be retried.
func upgradeLegacyStorageCandidate(candidate storageCandidate, deadline time.Time) (storageCandidate, int64, error) {
	marker := runStorageMarker{Schema: runStorageSchema, Root: candidate.root, Directory: candidate.path, RunID: filepath.Base(candidate.path), ProcessID: os.Getpid(), Managed: true, CreatedAt: candidate.finished.Add(-time.Second).Format(time.RFC3339), FinishedAt: candidate.finished.Format(time.RFC3339), GeneratedFileState: make(map[string]storageFileState)}
	for _, path := range candidate.files {
		if !time.Now().Before(deadline) {
			return candidate, 0, fmt.Errorf("cleanup_limit_reached")
		}
		if err := validateStoragePath(path); err != nil {
			return candidate, 0, err
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return candidate, 0, fmt.Errorf("legacy storage contents changed")
		}
		if expected, ok := candidate.fileStates[path]; !ok || expected.Size != info.Size() || expected.ModifiedAt != info.ModTime().UnixNano() {
			return candidate, 0, fmt.Errorf("legacy storage contents changed")
		}
		name := filepath.Base(path)
		marker.GeneratedFiles = append(marker.GeneratedFiles, name)
		marker.GeneratedFileState[name] = storageFileState{Size: info.Size(), ModifiedAt: info.ModTime().UnixNano()}
	}
	marker.GeneratedFiles = append(marker.GeneratedFiles, runStorageMarkerName)
	payload, err := json.Marshal(marker)
	if err != nil {
		return candidate, 0, err
	}
	payload = append(payload, '\n')
	path := filepath.Join(candidate.path, runStorageMarkerName)
	if err := validateStoragePath(candidate.path); err != nil {
		return candidate, 0, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return candidate, 0, err
	}
	_, writeErr := file.Write(payload)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return candidate, int64(len(payload)), fmt.Errorf("could not write legacy storage ownership evidence")
	}
	candidate.files = append(candidate.files, path)
	info, err := os.Lstat(path)
	if err != nil {
		return candidate, int64(len(payload)), err
	}
	candidate.fileStates[path] = storageFileState{Size: info.Size(), ModifiedAt: info.ModTime().UnixNano()}
	candidate.legacy = false
	return candidate, int64(len(payload)), nil
}

func removeStorageCandidate(candidate storageCandidate, deadline time.Time) (int64, error) {
	if !storageDirectChild(candidate.root, candidate.path) {
		return 0, fmt.Errorf("unsafe storage boundary")
	}
	// Keep ownership evidence until the payload is gone, allowing a partial
	// failure (for example a Windows file lock) to be retried safely.
	sort.SliceStable(candidate.files, func(i, j int) bool {
		priority := func(path string) int {
			switch filepath.Base(path) {
			case runStorageMarkerName:
				return 2
			case "semantic-idf-run.json":
				return 1
			default:
				return 0
			}
		}
		return priority(candidate.files[i]) < priority(candidate.files[j])
	})
	var freed int64
	for _, path := range candidate.files {
		if !time.Now().Before(deadline) {
			return freed, fmt.Errorf("cleanup_limit_reached")
		}
		if err := validateStoragePath(path); err != nil {
			return freed, err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return freed, err
		}
		if !info.Mode().IsRegular() || storagePathIsLink(path, info) {
			return freed, fmt.Errorf("unsafe storage file")
		}
		if expected, ok := candidate.fileStates[path]; !ok || expected.Size != info.Size() || expected.ModifiedAt != info.ModTime().UnixNano() {
			return freed, fmt.Errorf("storage file changed before cleanup")
		}
		if err := os.Remove(path); err != nil {
			return freed, err
		}
		freed += info.Size()
	}
	if err := validateStoragePath(candidate.path); err != nil {
		return freed, err
	}
	return freed, os.Remove(candidate.path)
}

func readStorageJSON(path string, target any) error {
	if err := validateStoragePath(path); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > maxStorageJSONBytes {
		return fmt.Errorf("storage metadata exceeds the read limit")
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxStorageJSONBytes))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("storage metadata must contain exactly one JSON object")
	}
	return nil
}

func storageDirectChild(root, path string) bool {
	return storagePathKey(filepath.Dir(path)) == storagePathKey(root) && storagePathKey(path) != storagePathKey(root)
}

func storagePathKey(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return storagePlatformPathKey(filepath.Clean(absolute))
}

func validateStoragePath(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for current := absolute; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if storagePathIsLink(current, info) {
			return fmt.Errorf("linked storage paths are protected")
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}
