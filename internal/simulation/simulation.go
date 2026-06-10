package simulation

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultSimulationWorkerFraction = 0.5
	maxWeatherFiles                 = 5000
	maxCSVSeriesColumns             = 16
	maxCSVSeriesPoints              = 1200
	maxCapturedOutputBytes          = 16000
)

type SimulationEnvironment struct {
	Settings            SimulationSettings         `json:"settings"`
	Installations       []EnergyPlusInstallSetting `json:"installations"`
	WeatherFolders      []WeatherFolder            `json:"weatherFolders"`
	DefaultRunDirectory string                     `json:"defaultRunDirectory"`
	DefaultWorkerCount  int                        `json:"defaultWorkerCount"`
	CPUCount            int                        `json:"cpuCount"`
	Warnings            []string                   `json:"warnings,omitempty"`
}

type WeatherFolder struct {
	Label  string        `json:"label"`
	Path   string        `json:"path"`
	Source string        `json:"source"`
	Files  []WeatherFile `json:"files"`
}

type WeatherFile struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Folder string `json:"folder"`
	Source string `json:"source"`
}

type SimulationSettings struct {
	EnergyPlusInstallations []EnergyPlusInstallSetting `json:"energyPlusInstallations"`
	ExtraWeatherDataPaths   []string                   `json:"extraWeatherDataPaths"`
	RunDirectory            string                     `json:"runDirectory"`
	WorkerFraction          float64                    `json:"workerFraction"`
	MaxWorkers              int                        `json:"maxWorkers"`
	AutoRunOnOpen           bool                       `json:"autoRunOnOpen"`
}

type EnergyPlusInstallSetting struct {
	ID              string `json:"id"`
	Version         string `json:"version"`
	Name            string `json:"name"`
	ExecutablePath  string `json:"executablePath"`
	RootPath        string `json:"rootPath"`
	WeatherDataPath string `json:"weatherDataPath"`
	AutoDetected    bool   `json:"autoDetected"`
}

type SimulationRunRequest struct {
	RunID                    string                    `json:"runId"`
	Text                     string                    `json:"text"`
	InputPath                string                    `json:"inputPath"`
	Filename                 string                    `json:"filename"`
	EnergyPlusExecutablePath string                    `json:"energyPlusExecutablePath"`
	WeatherPath              string                    `json:"weatherPath"`
	OutputDirectory          string                    `json:"outputDirectory"`
	StandardOutput           bool                      `json:"standardOutput"`
	StandardOutputMode       string                    `json:"standardOutputMode,omitempty"`
	PurposeRequest           *SimulationPurposeRequest `json:"purposeRequest,omitempty"`
	PurposeRunPlan           *PurposeRunPlan           `json:"purposeRunPlan,omitempty"`
	ResultMode               string                    `json:"resultMode,omitempty"`
	UseReadVarsESO           bool                      `json:"useReadVarsESO,omitempty"`
	Silent                   bool                      `json:"silent"`
	Auto                     bool                      `json:"auto"`
}

type MultiSimulationRequest struct {
	RunID                    string   `json:"runId"`
	InputPaths               []string `json:"inputPaths"`
	RootDirectory            string   `json:"rootDirectory"`
	Recursive                bool     `json:"recursive"`
	EnergyPlusExecutablePath string   `json:"energyPlusExecutablePath"`
	WeatherMode              string   `json:"weatherMode"`
	WeatherPath              string   `json:"weatherPath"`
	WorkerCount              int      `json:"workerCount"`
}

type SimulationFileSelectionResult struct {
	Canceled      bool     `json:"canceled,omitempty"`
	Paths         []string `json:"paths,omitempty"`
	RootDirectory string   `json:"rootDirectory,omitempty"`
}

type SimulationProgress struct {
	RunID     string  `json:"runId"`
	Phase     string  `json:"phase"`
	Status    string  `json:"status"`
	Message   string  `json:"message"`
	Completed int     `json:"completed"`
	Total     int     `json:"total"`
	Percent   float64 `json:"percent"`
	Path      string  `json:"path,omitempty"`
}

type SimulationRunResult struct {
	RunID                    string               `json:"runId"`
	Status                   string               `json:"status"`
	InputPath                string               `json:"inputPath,omitempty"`
	Filename                 string               `json:"filename,omitempty"`
	WeatherPath              string               `json:"weatherPath,omitempty"`
	EnergyPlusExecutablePath string               `json:"energyPlusExecutablePath,omitempty"`
	OutputDirectory          string               `json:"outputDirectory,omitempty"`
	StartedAt                string               `json:"startedAt,omitempty"`
	FinishedAt               string               `json:"finishedAt,omitempty"`
	DurationMS               int64                `json:"durationMs"`
	ExitCode                 int                  `json:"exitCode"`
	Error                    string               `json:"error,omitempty"`
	Stdout                   string               `json:"stdout,omitempty"`
	Stderr                   string               `json:"stderr,omitempty"`
	Files                    []SimulationFileInfo `json:"files,omitempty"`
	ERR                      ERRSummary           `json:"err"`
	CSVs                     []CSVSummary         `json:"csvs,omitempty"`
	Series                   []SimulationSeries   `json:"series,omitempty"`
	HeatFlow                 HeatFlowDataset      `json:"heatFlow,omitempty"`
	PurposeRunPlan           *PurposeRunPlan      `json:"purposeRunPlan,omitempty"`
	PurposeResults           *PurposeResultBundle `json:"purposeResults,omitempty"`
}

type SimulationRunManifest struct {
	RunID                    string                `json:"runId"`
	CreatedAt                string                `json:"createdAt"`
	StartedAt                string                `json:"startedAt,omitempty"`
	FinishedAt               string                `json:"finishedAt,omitempty"`
	Status                   string                `json:"status"`
	InputPath                string                `json:"inputPath,omitempty"`
	InputHash                string                `json:"inputHash,omitempty"`
	Filename                 string                `json:"filename,omitempty"`
	EnergyPlusExecutablePath string                `json:"energyPlusExecutablePath,omitempty"`
	WeatherPath              string                `json:"weatherPath,omitempty"`
	OutputDirectory          string                `json:"outputDirectory,omitempty"`
	Purposes                 []SimulationPurposeID `json:"purposes,omitempty"`
	OutputPlan               *PurposeRunPlan       `json:"outputPlan,omitempty"`
	ResultMode               string                `json:"resultMode,omitempty"`
	UseReadVarsESO           bool                  `json:"useReadVarsESO,omitempty"`
	ResultFiles              []SimulationFileInfo  `json:"resultFiles,omitempty"`
}

type MultiSimulationResult struct {
	Canceled  bool                  `json:"canceled,omitempty"`
	RunID     string                `json:"runId"`
	Total     int                   `json:"total"`
	Completed int                   `json:"completed"`
	Succeeded int                   `json:"succeeded"`
	Failed    int                   `json:"failed"`
	Workers   int                   `json:"workers"`
	Results   []SimulationRunResult `json:"results"`
}

type SimulationFileInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"`
	Size int64  `json:"size"`
}

type ERRSummary struct {
	Path      string     `json:"path,omitempty"`
	Warnings  int        `json:"warnings"`
	Severe    int        `json:"severe"`
	Fatal     int        `json:"fatal"`
	Total     int        `json:"total"`
	Completed bool       `json:"completed"`
	Issues    []ERRIssue `json:"issues,omitempty"`
}

type ERRIssue struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Line     int    `json:"line"`
}

type CSVSummary struct {
	Path       string             `json:"path"`
	Filename   string             `json:"filename"`
	RowCount   int                `json:"rowCount"`
	ColumnInfo []CSVColumnSummary `json:"columnInfo"`
}

type CSVColumnSummary struct {
	Index        int     `json:"index"`
	Name         string  `json:"name"`
	NumericCount int     `json:"numericCount"`
	Min          float64 `json:"min"`
	Max          float64 `json:"max"`
	Average      float64 `json:"average"`
	Last         float64 `json:"last"`
}

type SimulationSeries struct {
	File     string            `json:"file"`
	Column   string            `json:"column"`
	Min      float64           `json:"min"`
	Max      float64           `json:"max"`
	Average  float64           `json:"average"`
	Points   []SimulationPoint `json:"points"`
	RowCount int               `json:"rowCount"`
}

type SimulationPoint struct {
	X     int     `json:"x"`
	Label string  `json:"label,omitempty"`
	Value float64 `json:"value"`
}

type columnAccumulator struct {
	index        int
	name         string
	numericCount int
	sum          float64
	min          float64
	max          float64
	last         float64
}

func DefaultSettings() SimulationSettings {
	return SimulationSettings{
		RunDirectory:   defaultSimulationRunDirectory(),
		WorkerFraction: defaultSimulationWorkerFraction,
		MaxWorkers:     0,
		AutoRunOnOpen:  false,
	}
}

func NormalizeSettings(settings SimulationSettings, defaults SimulationSettings) SimulationSettings {
	settings.RunDirectory = strings.TrimSpace(settings.RunDirectory)
	if settings.RunDirectory == "" {
		settings.RunDirectory = defaults.RunDirectory
	}
	if settings.WorkerFraction <= 0 {
		settings.WorkerFraction = defaults.WorkerFraction
	}
	if settings.WorkerFraction < 0.1 {
		settings.WorkerFraction = 0.1
	}
	if settings.WorkerFraction > 1 {
		settings.WorkerFraction = 1
	}
	if settings.MaxWorkers < 0 {
		settings.MaxWorkers = 0
	}
	settings.EnergyPlusInstallations = normalizeEnergyPlusInstallations(settings.EnergyPlusInstallations)
	settings.ExtraWeatherDataPaths = normalizePathList(settings.ExtraWeatherDataPaths)
	return settings
}

func normalizeEnergyPlusInstallations(values []EnergyPlusInstallSetting) []EnergyPlusInstallSetting {
	out := make([]EnergyPlusInstallSetting, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value.ExecutablePath = strings.TrimSpace(value.ExecutablePath)
		value.RootPath = strings.TrimSpace(value.RootPath)
		value.WeatherDataPath = strings.TrimSpace(value.WeatherDataPath)
		if value.ExecutablePath == "" && value.RootPath == "" {
			continue
		}
		if value.ExecutablePath != "" {
			value.ExecutablePath = filepath.Clean(value.ExecutablePath)
		}
		if value.RootPath != "" {
			value.RootPath = filepath.Clean(value.RootPath)
		}
		if value.WeatherDataPath != "" {
			value.WeatherDataPath = filepath.Clean(value.WeatherDataPath)
		}
		value.Version = strings.TrimSpace(value.Version)
		value.Name = strings.TrimSpace(value.Name)
		if value.ExecutablePath == "" && value.RootPath != "" {
			value.ExecutablePath = filepath.Join(value.RootPath, energyPlusExecutableName())
		}
		if value.RootPath == "" && value.ExecutablePath != "" {
			value.RootPath = filepath.Dir(value.ExecutablePath)
		}
		if value.WeatherDataPath == "" && value.RootPath != "" {
			value.WeatherDataPath = filepath.Join(value.RootPath, "WeatherData")
		}
		if value.ID == "" {
			value.ID = installationID(value)
		}
		key := strings.ToLower(value.ExecutablePath)
		if key == "" {
			key = strings.ToLower(value.RootPath)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func normalizePathList(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		value = filepath.Clean(value)
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func BuildEnvironment(settings SimulationSettings) *SimulationEnvironment {
	settings = NormalizeSettings(settings, DefaultSettings())
	warnings := []string{}
	installations := mergeEnergyPlusInstallations(settings.EnergyPlusInstallations, AutoDetectEnergyPlusInstallations())
	if len(installations) == 0 {
		warnings = append(warnings, "No EnergyPlus installation was found. Register energyplus.exe in Settings.")
	}
	weatherFolders := collectWeatherFolders(installations, settings.ExtraWeatherDataPaths, &warnings)
	return &SimulationEnvironment{
		Settings:            settings,
		Installations:       installations,
		WeatherFolders:      weatherFolders,
		DefaultRunDirectory: settings.RunDirectory,
		DefaultWorkerCount:  DefaultWorkerCount(settings),
		CPUCount:            goruntime.NumCPU(),
		Warnings:            warnings,
	}
}

func mergeEnergyPlusInstallations(configured []EnergyPlusInstallSetting, detected []EnergyPlusInstallSetting) []EnergyPlusInstallSetting {
	seen := map[string]bool{}
	out := make([]EnergyPlusInstallSetting, 0, len(configured)+len(detected))
	for _, install := range append(configured, detected...) {
		normalized := normalizeEnergyPlusInstallations([]EnergyPlusInstallSetting{install})
		if len(normalized) == 0 {
			continue
		}
		install = normalized[0]
		key := strings.ToLower(install.ExecutablePath)
		if key == "" {
			key = strings.ToLower(install.RootPath)
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, install)
	}
	sortEnergyPlusInstallations(out)
	return out
}

func AutoDetectEnergyPlusInstallations() []EnergyPlusInstallSetting {
	candidates := []string{}
	if exe := strings.TrimSpace(os.Getenv("ENERGYPLUS_EXE")); exe != "" {
		candidates = append(candidates, exe)
	}
	if root := strings.TrimSpace(os.Getenv("ENERGYPLUS_ROOT")); root != "" {
		candidates = append(candidates, filepath.Join(root, energyPlusExecutableName()))
	}
	if exe, err := exec.LookPath("energyplus"); err == nil {
		candidates = append(candidates, exe)
	}
	for _, pattern := range defaultEnergyPlusRootPatterns() {
		matches, _ := filepath.Glob(pattern)
		for _, root := range matches {
			candidates = append(candidates, filepath.Join(root, energyPlusExecutableName()))
		}
	}
	out := []EnergyPlusInstallSetting{}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		key := strings.ToLower(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, EnergyPlusInstallFromExecutable(candidate, true))
	}
	sortEnergyPlusInstallations(out)
	return out
}

func sortEnergyPlusInstallations(installations []EnergyPlusInstallSetting) {
	sort.SliceStable(installations, func(i, j int) bool {
		if cmp := compareEnergyPlusVersions(installations[i].Version, installations[j].Version); cmp != 0 {
			return cmp > 0
		}
		if installations[i].AutoDetected != installations[j].AutoDetected {
			return !installations[i].AutoDetected
		}
		return strings.ToLower(installations[i].Name) < strings.ToLower(installations[j].Name)
	})
}

func compareEnergyPlusVersions(a string, b string) int {
	left := versionNumbers(a)
	right := versionNumbers(b)
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return -1
	}
	if len(right) == 0 {
		return 1
	}
	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}
	for i := 0; i < maxLen; i++ {
		lv, rv := 0, 0
		if i < len(left) {
			lv = left[i]
		}
		if i < len(right) {
			rv = right[i]
		}
		if lv > rv {
			return 1
		}
		if lv < rv {
			return -1
		}
	}
	return 0
}

func versionNumbers(value string) []int {
	out := []int{}
	current := strings.Builder{}
	flush := func() {
		if current.Len() == 0 {
			return
		}
		number, err := strconv.Atoi(current.String())
		if err == nil {
			out = append(out, number)
		}
		current.Reset()
	}
	for _, r := range value {
		if r >= '0' && r <= '9' {
			current.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func defaultEnergyPlusRootPatterns() []string {
	patterns := []string{`C:\EnergyPlusV*`}
	if programFiles := strings.TrimSpace(os.Getenv("ProgramFiles")); programFiles != "" {
		patterns = append(patterns, filepath.Join(programFiles, "EnergyPlusV*"))
	}
	if programFilesX86 := strings.TrimSpace(os.Getenv("ProgramFiles(x86)")); programFilesX86 != "" {
		patterns = append(patterns, filepath.Join(programFilesX86, "EnergyPlusV*"))
	}
	return patterns
}

func EnergyPlusInstallFromExecutable(executable string, autoDetected bool) EnergyPlusInstallSetting {
	executable = filepath.Clean(strings.TrimSpace(executable))
	root := filepath.Dir(executable)
	version := detectEnergyPlusVersion(executable, root)
	name := "EnergyPlus"
	if version != "" {
		name += " " + version
	}
	return EnergyPlusInstallSetting{
		ID:              installationID(EnergyPlusInstallSetting{ExecutablePath: executable, Version: version}),
		Version:         version,
		Name:            name,
		ExecutablePath:  executable,
		RootPath:        root,
		WeatherDataPath: filepath.Join(root, "WeatherData"),
		AutoDetected:    autoDetected,
	}
}

func detectEnergyPlusVersion(executable string, root string) string {
	base := filepath.Base(root)
	if strings.HasPrefix(strings.ToLower(base), "energyplusv") {
		version := strings.TrimPrefix(base, "EnergyPlusV")
		version = strings.TrimPrefix(version, "energyplusv")
		version = strings.ReplaceAll(version, "-", ".")
		version = strings.Trim(version, ". ")
		if version != "" {
			return version
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "--version")
	configureBackgroundCommand(command)
	output, err := command.Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(output))
	for _, field := range fields {
		cleaned := strings.Trim(field, " ,;")
		if strings.Count(cleaned, ".") >= 1 && containsDigit(cleaned) {
			return cleaned
		}
	}
	return ""
}

func containsDigit(value string) bool {
	for _, r := range value {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func collectWeatherFolders(installations []EnergyPlusInstallSetting, extraPaths []string, warnings *[]string) []WeatherFolder {
	type sourcePath struct {
		path   string
		source string
	}
	paths := []sourcePath{}
	for _, install := range installations {
		if install.WeatherDataPath != "" {
			label := install.Name
			if label == "" {
				label = "EnergyPlus"
			}
			paths = append(paths, sourcePath{path: install.WeatherDataPath, source: label})
		}
	}
	for _, path := range extraPaths {
		paths = append(paths, sourcePath{path: path, source: "Extra weather path"})
	}

	folderMap := map[string]*WeatherFolder{}
	for _, source := range paths {
		if source.path == "" {
			continue
		}
		info, err := os.Stat(source.path)
		if err != nil || !info.IsDir() {
			if warnings != nil {
				*warnings = append(*warnings, fmt.Sprintf("Weather directory not found: %s", source.path))
			}
			continue
		}
		files := findWeatherFiles(source.path, source.source)
		for _, file := range files {
			key := strings.ToLower(file.Folder)
			folder := folderMap[key]
			if folder == nil {
				label := filepath.Base(file.Folder)
				if strings.EqualFold(filepath.Clean(file.Folder), filepath.Clean(source.path)) {
					label = source.source
				}
				folder = &WeatherFolder{Label: label, Path: file.Folder, Source: source.source}
				folderMap[key] = folder
			}
			folder.Files = append(folder.Files, file)
		}
	}
	out := make([]WeatherFolder, 0, len(folderMap))
	for _, folder := range folderMap {
		sort.Slice(folder.Files, func(i, j int) bool {
			return strings.ToLower(folder.Files[i].Name) < strings.ToLower(folder.Files[j].Name)
		})
		out = append(out, *folder)
	}
	sort.Slice(out, func(i, j int) bool {
		if strings.ToLower(out[i].Source) != strings.ToLower(out[j].Source) {
			return strings.ToLower(out[i].Source) < strings.ToLower(out[j].Source)
		}
		return strings.ToLower(out[i].Label) < strings.ToLower(out[j].Label)
	})
	return out
}

func findWeatherFiles(root string, source string) []WeatherFile {
	files := []WeatherFile{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || len(files) >= maxWeatherFiles {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".epw") {
			files = append(files, WeatherFileFromPath(path, source))
		}
		return nil
	})
	return files
}

func WeatherFileFromPath(path string, source string) WeatherFile {
	path = filepath.Clean(strings.TrimSpace(path))
	return WeatherFile{
		Name:   filepath.Base(path),
		Path:   path,
		Folder: filepath.Dir(path),
		Source: source,
	}
}

func RunSimulation(request SimulationRunRequest, progress func(SimulationProgress), settings SimulationSettings) (*SimulationRunResult, error) {
	settings = NormalizeSettings(settings, DefaultSettings())
	request.RunID = defaultRunID(request.RunID)
	started := time.Now()
	result := &SimulationRunResult{
		RunID:                    request.RunID,
		Status:                   "running",
		InputPath:                strings.TrimSpace(request.InputPath),
		Filename:                 strings.TrimSpace(request.Filename),
		WeatherPath:              strings.TrimSpace(request.WeatherPath),
		EnergyPlusExecutablePath: strings.TrimSpace(request.EnergyPlusExecutablePath),
		StartedAt:                started.Format(time.RFC3339),
		ExitCode:                 -1,
	}
	emitSimulationProgress(progress, request.RunID, "prepare", "running", "Preparing EnergyPlus run", 0, 4, result.InputPath)

	if result.EnergyPlusExecutablePath == "" {
		installations := AutoDetectEnergyPlusInstallations()
		if len(installations) > 0 {
			result.EnergyPlusExecutablePath = installations[0].ExecutablePath
		}
	}
	if result.EnergyPlusExecutablePath == "" {
		result.Status = "missing_energyplus"
		result.Error = "EnergyPlus executable is not configured."
		finishSimulationResult(result, started)
		emitSimulationProgress(progress, request.RunID, "complete", result.Status, result.Error, 4, 4, result.InputPath)
		return result, nil
	}
	if _, err := os.Stat(result.EnergyPlusExecutablePath); err != nil {
		result.Status = "missing_energyplus"
		result.Error = fmt.Sprintf("EnergyPlus executable was not found: %s", result.EnergyPlusExecutablePath)
		finishSimulationResult(result, started)
		emitSimulationProgress(progress, request.RunID, "complete", result.Status, result.Error, 4, 4, result.InputPath)
		return result, nil
	}

	outputDir, err := simulationOutputDirectory(request.OutputDirectory, request.RunID, result.Filename, result.InputPath, settings.RunDirectory)
	if err != nil {
		return nil, err
	}
	result.OutputDirectory = outputDir
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}

	inputPath := strings.TrimSpace(request.InputPath)
	if strings.TrimSpace(request.Text) != "" {
		inputPath = filepath.Join(outputDir, simulationInputFilename(result.Filename, inputPath))
		if err := os.WriteFile(inputPath, []byte(request.Text), 0o644); err != nil {
			return nil, err
		}
		result.InputPath = inputPath
	} else if inputPath == "" {
		result.Status = "failed"
		result.Error = "Simulation needs either input text or an input path."
		finishSimulationResult(result, started)
		emitSimulationProgress(progress, request.RunID, "complete", result.Status, result.Error, 4, 4, result.InputPath)
		return result, nil
	}
	if result.Filename == "" {
		result.Filename = filepath.Base(inputPath)
	}

	emitSimulationProgress(progress, request.RunID, "execute", "running", "EnergyPlus is running", 1, 4, result.InputPath)
	args := []string{"-d", outputDir, "-p", "eplus"}
	if simulationUsesReadVarsESO(request) {
		args = append(args, "-r")
	}
	if result.WeatherPath != "" {
		args = append(args, "-w", result.WeatherPath)
	}
	args = append(args, inputPath)
	command := exec.CommandContext(context.Background(), result.EnergyPlusExecutablePath, args...)
	command.Dir = outputDir
	stdout, stderr, exitCode, runErr := runCommandCaptured(command)
	result.Stdout = stdout
	result.Stderr = stderr
	result.ExitCode = exitCode
	if runErr != nil {
		result.Error = runErr.Error()
	}

	emitSimulationProgress(progress, request.RunID, "parse", "running", "Reading simulation output", 3, 4, result.InputPath)
	readSimulationOutputs(result)
	if request.PurposeRunPlan != nil {
		result.PurposeRunPlan = request.PurposeRunPlan
	}
	if request.PurposeRequest != nil {
		bundle := BuildPurposeResultBundle(result, *request.PurposeRequest)
		result.PurposeResults = &bundle
	}
	if result.Error != "" || result.ExitCode != 0 || result.ERR.Fatal > 0 || result.ERR.Severe > 0 {
		result.Status = "failed"
	} else {
		result.Status = "succeeded"
	}
	finishSimulationResult(result, started)
	writeSimulationRunManifest(result, request)
	emitSimulationProgress(progress, request.RunID, "complete", result.Status, simulationCompletionMessage(result), 4, 4, result.InputPath)
	return result, nil
}

func RunMultipleSimulations(request MultiSimulationRequest, progress func(SimulationProgress), settings SimulationSettings) (*MultiSimulationResult, error) {
	settings = NormalizeSettings(settings, DefaultSettings())
	request.RunID = defaultRunID(request.RunID)
	paths := normalizePathList(request.InputPaths)
	if len(paths) == 0 && strings.TrimSpace(request.RootDirectory) != "" {
		found, err := FindInputFiles(request.RootDirectory, request.Recursive)
		if err != nil {
			return nil, err
		}
		paths = found
	}
	if len(paths) == 0 {
		return &MultiSimulationResult{Canceled: true, RunID: request.RunID}, nil
	}
	workers := request.WorkerCount
	if workers <= 0 {
		workers = DefaultWorkerCount(settings)
	}
	if workers > len(paths) {
		workers = len(paths)
	}
	if workers < 1 {
		workers = 1
	}
	result := &MultiSimulationResult{RunID: request.RunID, Total: len(paths), Workers: workers}
	emitSimulationProgress(progress, request.RunID, "prepare", "running", "Preparing batch simulation", 0, len(paths), "")

	jobs := make(chan string)
	results := make(chan SimulationRunResult)
	var wg sync.WaitGroup
	var completedMu sync.Mutex
	completed := 0
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				weatherPath := resolveBatchWeather(path, request)
				runResult, err := RunSimulation(SimulationRunRequest{
					RunID:                    request.RunID + "-" + shortPathHash(path),
					InputPath:                path,
					Filename:                 filepath.Base(path),
					EnergyPlusExecutablePath: request.EnergyPlusExecutablePath,
					WeatherPath:              weatherPath,
					Silent:                   true,
				}, nil, settings)
				if err != nil {
					runResult = &SimulationRunResult{
						RunID:     request.RunID + "-" + shortPathHash(path),
						Status:    "failed",
						InputPath: path,
						Filename:  filepath.Base(path),
						Error:     err.Error(),
						ExitCode:  -1,
					}
				}
				completedMu.Lock()
				completed++
				currentCompleted := completed
				completedMu.Unlock()
				emitSimulationProgress(progress, request.RunID, "execute", runResult.Status, filepath.Base(path), currentCompleted, len(paths), path)
				results <- *runResult
			}
		}()
	}
	go func() {
		for _, path := range paths {
			jobs <- path
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	for item := range results {
		result.Results = append(result.Results, item)
		result.Completed++
		if item.Status == "succeeded" {
			result.Succeeded++
		} else {
			result.Failed++
		}
	}
	sort.Slice(result.Results, func(i, j int) bool {
		return strings.ToLower(result.Results[i].Filename) < strings.ToLower(result.Results[j].Filename)
	})
	emitSimulationProgress(progress, request.RunID, "complete", "complete", "Batch simulation complete", result.Completed, result.Total, "")
	return result, nil
}

func emitSimulationProgress(progress func(SimulationProgress), runID string, phase string, status string, message string, completed int, total int, path string) {
	if progress == nil {
		return
	}
	percent := 0.0
	if total > 0 {
		percent = math.Max(0, math.Min(100, float64(completed)/float64(total)*100))
	}
	progress(SimulationProgress{
		RunID:     runID,
		Phase:     phase,
		Status:    status,
		Message:   message,
		Completed: completed,
		Total:     total,
		Percent:   percent,
		Path:      path,
	})
}

func finishSimulationResult(result *SimulationRunResult, started time.Time) {
	finished := time.Now()
	result.FinishedAt = finished.Format(time.RFC3339)
	result.DurationMS = finished.Sub(started).Milliseconds()
}

func writeSimulationRunManifest(result *SimulationRunResult, request SimulationRunRequest) {
	if result == nil || strings.TrimSpace(result.OutputDirectory) == "" {
		return
	}
	manifest := SimulationRunManifest{
		RunID:                    result.RunID,
		CreatedAt:                time.Now().Format(time.RFC3339),
		StartedAt:                result.StartedAt,
		FinishedAt:               result.FinishedAt,
		Status:                   result.Status,
		InputPath:                result.InputPath,
		InputHash:                fileSHA256(result.InputPath),
		Filename:                 result.Filename,
		EnergyPlusExecutablePath: result.EnergyPlusExecutablePath,
		WeatherPath:              result.WeatherPath,
		OutputDirectory:          result.OutputDirectory,
		OutputPlan:               request.PurposeRunPlan,
		ResultMode:               request.ResultMode,
		UseReadVarsESO:           simulationUsesReadVarsESO(request),
		ResultFiles:              append([]SimulationFileInfo(nil), result.Files...),
	}
	if request.PurposeRequest != nil {
		manifest.Purposes = append([]SimulationPurposeID(nil), request.PurposeRequest.Purposes...)
	}
	path := filepath.Join(result.OutputDirectory, "idf-analyzer-run.json")
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	result.Files = append(result.Files, SimulationFileInfo{
		Name: filepath.Base(path),
		Path: path,
		Kind: "manifest",
		Size: info.Size(),
	})
	sort.Slice(result.Files, func(i, j int) bool {
		return strings.ToLower(result.Files[i].Name) < strings.ToLower(result.Files[j].Name)
	})
}

func fileSHA256(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func simulationUsesReadVarsESO(request SimulationRunRequest) bool {
	mode := strings.ToLower(strings.TrimSpace(request.ResultMode))
	switch mode {
	case "sql_first", "sql-only", "sql_only":
		return request.UseReadVarsESO
	case "csv", "csv_fallback", "readvarseso":
		return true
	default:
		return true
	}
}

func simulationCompletionMessage(result *SimulationRunResult) string {
	if result.Status == "succeeded" {
		return fmt.Sprintf("Simulation complete: %d warnings", result.ERR.Warnings)
	}
	if result.Error != "" {
		return result.Error
	}
	return "Simulation finished with errors"
}

func runCommandCaptured(command *exec.Cmd) (string, string, int, error) {
	configureBackgroundCommand(command)
	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return "", "", -1, err
	}
	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return "", "", -1, err
	}
	if err := command.Start(); err != nil {
		return "", "", -1, err
	}
	var stdoutBuilder, stderrBuilder strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		copyLimited(&stdoutBuilder, stdoutPipe, maxCapturedOutputBytes)
	}()
	go func() {
		defer wg.Done()
		copyLimited(&stderrBuilder, stderrPipe, maxCapturedOutputBytes)
	}()
	err = command.Wait()
	wg.Wait()
	exitCode := 0
	if command.ProcessState != nil {
		exitCode = command.ProcessState.ExitCode()
	}
	return stdoutBuilder.String(), stderrBuilder.String(), exitCode, err
}

func copyLimited(builder *strings.Builder, reader io.Reader, limit int) {
	buffer := make([]byte, 4096)
	for {
		n, err := reader.Read(buffer)
		if n > 0 && builder.Len() < limit {
			remaining := limit - builder.Len()
			if n > remaining {
				n = remaining
			}
			builder.Write(buffer[:n])
		}
		if err != nil {
			return
		}
	}
}

func readSimulationOutputs(result *SimulationRunResult) {
	result.Files = collectSimulationFiles(result.OutputDirectory)
	parseERR(result)
	sort.Slice(result.Files, func(i, j int) bool {
		return strings.ToLower(result.Files[i].Name) < strings.ToLower(result.Files[j].Name)
	})
	parseSQLResults(result)
	parseCSVResults(result)
	parseHeatFlowFallback(result)
}

func collectSimulationFiles(outputDirectory string) []SimulationFileInfo {
	files, _ := os.ReadDir(outputDirectory)
	out := []SimulationFileInfo{}
	for _, entry := range files {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(outputDirectory, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		kind := simulationFileKind(entry.Name())
		out = append(out, SimulationFileInfo{
			Name: entry.Name(),
			Path: path,
			Kind: kind,
			Size: info.Size(),
		})
	}
	return out
}

func parseERR(result *SimulationRunResult) {
	for _, file := range result.Files {
		if file.Kind == "err" {
			result.ERR = parseERRFile(file.Path)
			return
		}
	}
}

func parseSQLResults(result *SimulationRunResult) {
	for _, file := range result.Files {
		if file.Kind != "sqlite" {
			continue
		}
		if len(result.Series) == 0 {
			series, err := parseSimulationSQLSeries(file.Path)
			if err == nil && len(series) > 0 {
				result.Series = append(result.Series, series...)
			}
		}
		if len(result.HeatFlow.Zones) == 0 {
			heatFlow, err := parseSimulationHeatFlowSQL(file.Path)
			if err == nil && len(heatFlow.Zones) > 0 {
				result.HeatFlow = heatFlow
			}
		}
	}
}

func parseCSVResults(result *SimulationRunResult) {
	for _, file := range result.Files {
		if file.Kind != "csv" {
			continue
		}
		summary, series, err := parseSimulationCSV(file.Path)
		if err != nil {
			continue
		}
		result.CSVs = append(result.CSVs, summary)
		if len(result.Series) == 0 {
			result.Series = append(result.Series, series...)
		}
		if len(result.HeatFlow.Zones) == 0 {
			heatFlow, err := parseSimulationHeatFlowCSV(file.Path)
			if err == nil && len(heatFlow.Zones) > 0 {
				result.HeatFlow = heatFlow
			}
		}
	}
}

func parseHeatFlowFallback(result *SimulationRunResult) {
	if len(result.HeatFlow.Zones) == 0 {
		for _, file := range result.Files {
			if file.Kind != "eso" {
				continue
			}
			heatFlow, err := parseSimulationHeatFlowESO(file.Path)
			if err == nil && len(heatFlow.Zones) > 0 {
				result.HeatFlow = heatFlow
				break
			}
		}
	}
}

func simulationFileKind(name string) string {
	if strings.EqualFold(filepath.Base(name), "idf-analyzer-run.json") {
		return "manifest"
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".err":
		return "err"
	case ".csv":
		return "csv"
	case ".eso":
		return "eso"
	case ".mtr":
		return "meter"
	case ".rdd":
		return "variable_dictionary"
	case ".mdd":
		return "meter_dictionary"
	case ".sql":
		return "sqlite"
	case ".htm", ".html":
		return "html"
	default:
		return "other"
	}
}

func parseERRFile(path string) ERRSummary {
	summary := ERRSummary{Path: path}
	file, err := os.Open(path)
	if err != nil {
		return summary
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		lower := strings.ToLower(line)
		if strings.Contains(lower, "energyplus completed successfully") {
			summary.Completed = true
		}
		severity := ""
		switch {
		case strings.Contains(line, "** Warning **"):
			severity = "warning"
			summary.Warnings++
		case strings.Contains(line, "** Severe  **"), strings.Contains(line, "** Severe **"):
			severity = "severe"
			summary.Severe++
		case strings.Contains(line, "** Fatal  **"), strings.Contains(line, "** Fatal **"):
			severity = "fatal"
			summary.Fatal++
		}
		if severity != "" {
			summary.Issues = append(summary.Issues, ERRIssue{
				Severity: severity,
				Message:  strings.TrimSpace(strings.ReplaceAll(line, "**", "")),
				Line:     lineNo,
			})
		}
	}
	summary.Total = summary.Warnings + summary.Severe + summary.Fatal
	return summary
}

func parseSimulationCSV(path string) (CSVSummary, []SimulationSeries, error) {
	file, err := os.Open(path)
	if err != nil {
		return CSVSummary{}, nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return CSVSummary{}, nil, err
	}
	accumulators := make([]columnAccumulator, len(header))
	for index, name := range header {
		accumulators[index] = columnAccumulator{index: index, name: strings.TrimSpace(name), min: math.Inf(1), max: math.Inf(-1)}
	}
	seriesPoints := make(map[int][]SimulationPoint)
	rowCount := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		rowCount++
		label := ""
		if len(record) > 0 {
			label = strings.TrimSpace(record[0])
		}
		for index := 1; index < len(record) && index < len(accumulators); index++ {
			value, ok := parseCSVFloat(record[index])
			if !ok {
				continue
			}
			acc := &accumulators[index]
			acc.numericCount++
			acc.sum += value
			acc.last = value
			if value < acc.min {
				acc.min = value
			}
			if value > acc.max {
				acc.max = value
			}
			if index <= maxCSVSeriesColumns {
				seriesPoints[index] = append(seriesPoints[index], SimulationPoint{X: rowCount, Label: label, Value: value})
			}
		}
	}
	summary := CSVSummary{
		Path:     path,
		Filename: filepath.Base(path),
		RowCount: rowCount,
	}
	series := []SimulationSeries{}
	for index := 1; index < len(accumulators); index++ {
		acc := accumulators[index]
		if acc.numericCount == 0 {
			continue
		}
		average := acc.sum / float64(acc.numericCount)
		summary.ColumnInfo = append(summary.ColumnInfo, CSVColumnSummary{
			Index:        acc.index,
			Name:         acc.name,
			NumericCount: acc.numericCount,
			Min:          acc.min,
			Max:          acc.max,
			Average:      average,
			Last:         acc.last,
		})
		if points := seriesPoints[index]; len(points) > 0 {
			series = append(series, SimulationSeries{
				File:     filepath.Base(path),
				Column:   acc.name,
				Min:      acc.min,
				Max:      acc.max,
				Average:  average,
				Points:   downsamplePoints(points, maxCSVSeriesPoints),
				RowCount: rowCount,
			})
		}
	}
	return summary, series, nil
}

func parseCSVFloat(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

func downsamplePoints(points []SimulationPoint, limit int) []SimulationPoint {
	if limit <= 0 || len(points) <= limit {
		return points
	}
	out := make([]SimulationPoint, 0, limit)
	step := float64(len(points)-1) / float64(limit-1)
	for i := 0; i < limit; i++ {
		index := int(math.Round(float64(i) * step))
		if index < 0 {
			index = 0
		}
		if index >= len(points) {
			index = len(points) - 1
		}
		out = append(out, points[index])
	}
	return out
}

func simulationOutputDirectory(requested string, runID string, filename string, inputPath string, defaultRunDirectory string) (string, error) {
	if strings.TrimSpace(requested) != "" {
		return filepath.Clean(requested), nil
	}
	root := strings.TrimSpace(defaultRunDirectory)
	if root == "" {
		root = defaultSimulationRunDirectory()
	}
	label := strings.TrimSpace(filename)
	if label == "" && inputPath != "" {
		label = filepath.Base(inputPath)
	}
	if label == "" {
		label = "current-input"
	}
	return filepath.Join(root, sanitizePathSegment(time.Now().Format("20060102-150405")+"-"+runID+"-"+strings.TrimSuffix(label, filepath.Ext(label)))), nil
}

func defaultSimulationRunDirectory() string {
	root := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if root == "" {
		if cacheDir, err := os.UserCacheDir(); err == nil {
			root = cacheDir
		}
	}
	if root == "" {
		root = os.TempDir()
	}
	return filepath.Join(root, "IDF Analyzer", "simulations")
}

func DefaultWorkerCount(settings SimulationSettings) int {
	cpus := goruntime.NumCPU()
	if cpus < 1 {
		cpus = 1
	}
	workers := int(math.Ceil(float64(cpus) * settings.WorkerFraction))
	if workers < 1 {
		workers = 1
	}
	if settings.MaxWorkers > 0 && workers > settings.MaxWorkers {
		workers = settings.MaxWorkers
	}
	return workers
}

func FindInputFiles(root string, recursive bool) ([]string, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return nil, nil
	}
	paths := []string{}
	if recursive {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			if isSimulationInputFile(path) {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if isSimulationInputFile(path) {
				paths = append(paths, path)
			}
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func isSimulationInputFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".idf", ".imf", ".epjson", ".json":
		return true
	default:
		return false
	}
}

func resolveBatchWeather(inputPath string, request MultiSimulationRequest) string {
	mode := strings.ToLower(strings.TrimSpace(request.WeatherMode))
	if mode == "" || mode == "same" || mode == "same_weather" {
		return strings.TrimSpace(request.WeatherPath)
	}
	if mode != "subfolder" && mode != "nearest" {
		return strings.TrimSpace(request.WeatherPath)
	}
	root := filepath.Clean(strings.TrimSpace(request.RootDirectory))
	dir := filepath.Dir(inputPath)
	for {
		weather := firstWeatherInDirectory(dir)
		if weather != "" {
			return weather
		}
		if root == "" || strings.EqualFold(filepath.Clean(dir), root) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return strings.TrimSpace(request.WeatherPath)
}

func firstWeatherInDirectory(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	candidates := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".epw") {
			candidates = append(candidates, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func simulationInputFilename(filename string, inputPath string) string {
	name := strings.TrimSpace(filename)
	if name == "" && inputPath != "" {
		name = filepath.Base(inputPath)
	}
	if name == "" {
		name = "in.idf"
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".idf" && ext != ".imf" && ext != ".epjson" && ext != ".json" {
		name += ".idf"
	}
	return sanitizePathSegment(name)
}

func defaultRunID(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID != "" {
		return sanitizePathSegment(runID)
	}
	return "sim-" + time.Now().Format("20060102150405")
}

func sanitizePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "run"
	}
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	value = replacer.Replace(value)
	value = strings.Trim(value, ". ")
	if value == "" {
		return "run"
	}
	return value
}

func shortPathHash(path string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.ToLower(path)))
	return fmt.Sprintf("%08x", hash.Sum32())
}

func installationID(install EnergyPlusInstallSetting) string {
	source := install.ExecutablePath
	if source == "" {
		source = install.RootPath + install.Version
	}
	return "ep-" + shortPathHash(source)
}

func energyPlusExecutableName() string {
	if goruntime.GOOS == "windows" {
		return "energyplus.exe"
	}
	return "energyplus"
}
