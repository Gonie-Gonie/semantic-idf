package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

type AppSettings struct {
	Version     int                         `json:"version"`
	Appearance  AppearanceSettings          `json:"appearance"`
	Interaction InteractionSettings         `json:"interaction"`
	Profile     idf.ProfileAnalysisSettings `json:"profile"`
	Simulation  SimulationSettings          `json:"simulation"`
	Storage     StorageSettings             `json:"storage"`
}

type StorageSettings struct {
	AutoClean bool `json:"autoClean"`
}

type AppearanceSettings struct {
	Theme            string                     `json:"theme"`
	Language         string                     `json:"language"`
	DefaultInputView string                     `json:"defaultInputView"`
	GraphFontSize    int                        `json:"graphFontSize"`
	AnalysisTabOrder []string                   `json:"analysisTabOrder"`
	Geometry         GeometryAppearanceSettings `json:"geometry"`
}

type GeometryAppearanceSettings struct {
	Background string `json:"background"`
	Zone       string `json:"zone"`
	Wall       string `json:"wall"`
	Roof       string `json:"roof"`
	Window     string `json:"window"`
	Selected   string `json:"selected"`
}

type SimulationSettings = simulation.SimulationSettings
type EnergyPlusInstallSetting = simulation.EnergyPlusInstallSetting

type InteractionSettings struct {
	Shortcuts map[string]string `json:"shortcuts"`
}

type SettingsResult struct {
	Path     string      `json:"path"`
	Settings AppSettings `json:"settings"`
}

var settingsFileMu sync.Mutex

func (a *App) GetSettings() (*SettingsResult, error) {
	path, settings, err := loadAppSettings()
	if err != nil {
		return nil, err
	}
	return &SettingsResult{Path: path, Settings: settings}, nil
}

func (a *App) SaveSettings(settings AppSettings) (*SettingsResult, error) {
	result, err := saveAppSettings(settings)
	if err == nil && result.Settings.Storage.AutoClean {
		a.applyStorageCleanupPolicy(result.Settings)
	}
	return result, err
}

func saveAppSettings(settings AppSettings) (*SettingsResult, error) {
	settingsFileMu.Lock()
	defer settingsFileMu.Unlock()
	settings = normalizeAppSettings(settings)
	path, err := appSettingsPath()
	if err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeSettingsFile(path, append(payload, '\n')); err != nil {
		return nil, err
	}
	return &SettingsResult{Path: path, Settings: settings}, nil
}

func defaultAppSettings() AppSettings {
	return AppSettings{
		Version: 1,
		Appearance: AppearanceSettings{
			Theme:            "system",
			Language:         "en",
			DefaultInputView: "text",
			GraphFontSize:    11,
			AnalysisTabOrder: []string{"metrics", "topology", "profile", "hvac", "simulation"},
			Geometry: GeometryAppearanceSettings{
				Background: "#f7fafc",
				Zone:       "#b8d7b0",
				Wall:       "#7b9cbc",
				Roof:       "#b8b0a1",
				Window:     "#3fb6d4",
				Selected:   "#f0a202",
			},
		},
		Interaction: InteractionSettings{
			Shortcuts: map[string]string{
				"save":                 "Ctrl+S",
				"open":                 "Ctrl+O",
				"undoView":             "Alt+Left",
				"redoView":             "Alt+Right",
				"jumpDefinition":       "F12",
				"jumpReferences":       "Shift+F12",
				"commandPalette":       "Ctrl+K",
				"revealSource":         "Ctrl+Shift+S",
				"paneFocus":            "F6",
				"currentSearch":        "/",
				"primaryOpen":          "Enter",
				"availableViews":       "Alt+Enter",
				"clearSelection":       "Escape",
				"inputSemantic":        "Ctrl+1",
				"inputText":            "Ctrl+2",
				"inputJson":            "Ctrl+3",
				"inputTable":           "Ctrl+4",
				"tabMetrics":           "Ctrl+Alt+1",
				"tabProfile":           "Ctrl+Alt+2",
				"tabHVAC":              "Ctrl+Alt+3",
				"tabSimulation":        "Ctrl+Alt+4",
				"tabTopology":          "Ctrl+Alt+6",
				"topology3D":           "1",
				"topologyPlan":         "2",
				"topologyNetwork":      "3",
				"topologyFit":          "F",
				"topologyConnectivity": "T",
				"topologyArea":         "A",
				"topologyUA":           "U",
				"topologyQA":           "Q",
			},
		},
		Profile:    idf.DefaultProfileAnalysisSettings(),
		Simulation: simulation.DefaultSettings(),
	}
}

func normalizeAppSettings(settings AppSettings) AppSettings {
	defaults := defaultAppSettings()
	if settings.Version == 0 {
		settings.Version = defaults.Version
	}
	switch strings.ToLower(strings.TrimSpace(settings.Appearance.Theme)) {
	case "light", "dark", "system":
		settings.Appearance.Theme = strings.ToLower(strings.TrimSpace(settings.Appearance.Theme))
	default:
		settings.Appearance.Theme = defaults.Appearance.Theme
	}
	settings.Appearance.Language = normalizeAppLanguage(settings.Appearance.Language, defaults.Appearance.Language)
	switch strings.ToLower(strings.TrimSpace(settings.Appearance.DefaultInputView)) {
	case "text", "semantic", "json", "table":
		settings.Appearance.DefaultInputView = strings.ToLower(strings.TrimSpace(settings.Appearance.DefaultInputView))
	default:
		settings.Appearance.DefaultInputView = defaults.Appearance.DefaultInputView
	}
	if settings.Appearance.GraphFontSize == 0 {
		settings.Appearance.GraphFontSize = defaults.Appearance.GraphFontSize
	}
	settings.Appearance.GraphFontSize = max(9, min(18, settings.Appearance.GraphFontSize))
	settings.Appearance.AnalysisTabOrder = normalizeAnalysisTabOrder(settings.Appearance.AnalysisTabOrder, defaults.Appearance.AnalysisTabOrder)
	settings.Appearance.Geometry.Background = normalizeHexColor(settings.Appearance.Geometry.Background, defaults.Appearance.Geometry.Background)
	settings.Appearance.Geometry.Zone = normalizeHexColor(settings.Appearance.Geometry.Zone, defaults.Appearance.Geometry.Zone)
	settings.Appearance.Geometry.Wall = normalizeHexColor(settings.Appearance.Geometry.Wall, defaults.Appearance.Geometry.Wall)
	settings.Appearance.Geometry.Roof = normalizeHexColor(settings.Appearance.Geometry.Roof, defaults.Appearance.Geometry.Roof)
	settings.Appearance.Geometry.Window = normalizeHexColor(settings.Appearance.Geometry.Window, defaults.Appearance.Geometry.Window)
	settings.Appearance.Geometry.Selected = normalizeHexColor(settings.Appearance.Geometry.Selected, defaults.Appearance.Geometry.Selected)
	settings.Interaction.Shortcuts = normalizeShortcutSettings(settings.Interaction.Shortcuts, defaults.Interaction.Shortcuts)
	settings.Profile = normalizeProfileSettings(settings.Profile, defaults.Profile)
	settings.Simulation = simulation.NormalizeSettings(settings.Simulation, defaults.Simulation)
	return settings
}

func normalizeShortcutSettings(values map[string]string, defaults map[string]string) map[string]string {
	out := make(map[string]string, len(defaults))
	if _, hasSemantic := values["inputSemantic"]; !hasSemantic {
		values = migrateInputViewShortcutDefaults(values)
	}
	values = migrateTopologyShortcutNames(values)
	for key, fallback := range defaults {
		value := strings.TrimSpace(values[key])
		if value == "" {
			value = fallback
		}
		out[key] = value
	}
	return out
}

func migrateTopologyShortcutNames(values map[string]string) map[string]string {
	migrated := make(map[string]string, len(values)+4)
	for key, value := range values {
		migrated[key] = value
	}
	for canonical, legacy := range map[string]string{
		"topology3D":      "geometry3D",
		"topologyPlan":    "geometryPlan",
		"topologyNetwork": "geometryThermal",
		"topologyFit":     "geometryFit",
	} {
		if _, exists := migrated[canonical]; !exists {
			migrated[canonical] = migrated[legacy]
		}
		delete(migrated, legacy)
	}
	return migrated
}

func migrateInputViewShortcutDefaults(values map[string]string) map[string]string {
	migrated := make(map[string]string, len(values)+1)
	for key, value := range values {
		migrated[key] = value
	}
	if isBlankOrShortcut(migrated["inputText"], "Ctrl+1") {
		migrated["inputText"] = "Ctrl+2"
	}
	if isBlankOrShortcut(migrated["inputJson"], "Ctrl+2") {
		migrated["inputJson"] = "Ctrl+3"
	}
	if isBlankOrShortcut(migrated["inputTable"], "Ctrl+3") {
		migrated["inputTable"] = "Ctrl+4"
	}
	return migrated
}

func isBlankOrShortcut(value string, expected string) bool {
	value = strings.TrimSpace(value)
	return value == "" || strings.EqualFold(value, expected)
}

func normalizeAppLanguage(value string, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if index := strings.IndexAny(normalized, "-_"); index >= 0 {
		normalized = normalized[:index]
	}
	switch normalized {
	case "en", "ko", "ja", "hi", "es", "fr":
		return normalized
	case "kr":
		return "ko"
	case "jp":
		return "ja"
	default:
		return fallback
	}
}

func normalizeAnalysisTabOrder(values []string, fallback []string) []string {
	allowed := map[string]bool{
		"metrics":    true,
		"profile":    true,
		"hvac":       true,
		"simulation": true,
		"topology":   true,
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(allowed))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "summary" {
			normalized = "metrics"
		} else if normalized == "geometry" {
			normalized = "topology"
		}
		if !allowed[normalized] || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	source := fallback
	if len(source) == 0 {
		source = []string{"metrics", "topology", "profile", "hvac", "simulation"}
	}
	for _, value := range source {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if allowed[normalized] && !seen[normalized] {
			seen[normalized] = true
			out = append(out, normalized)
		}
	}
	return out
}

func normalizeProfileSettings(settings idf.ProfileAnalysisSettings, defaults idf.ProfileAnalysisSettings) idf.ProfileAnalysisSettings {
	allowedDimensions := map[string]bool{
		idf.ProfileDimensionOccupancy:    true,
		idf.ProfileDimensionLighting:     true,
		idf.ProfileDimensionEquipment:    true,
		idf.ProfileDimensionInfiltration: true,
		idf.ProfileDimensionVentilation:  true,
		idf.ProfileDimensionOutdoorAir:   true,
	}
	settings.EnabledDimensions = normalizeProfileChoiceList(settings.EnabledDimensions, defaults.EnabledDimensions, allowedDimensions)
	if settings.DisplayMetrics == nil {
		settings.DisplayMetrics = defaults.DisplayMetrics
	}
	if settings.GroupingMetrics == nil {
		settings.GroupingMetrics = defaults.GroupingMetrics
	}
	for dimension, metric := range defaults.DisplayMetrics {
		if strings.TrimSpace(settings.DisplayMetrics[dimension]) == "" {
			settings.DisplayMetrics[dimension] = metric
		}
	}
	for dimension, metric := range defaults.GroupingMetrics {
		if strings.TrimSpace(settings.GroupingMetrics[dimension]) == "" {
			settings.GroupingMetrics[dimension] = metric
		}
	}
	if settings.NumericTolerance <= 0 {
		settings.NumericTolerance = defaults.NumericTolerance
	}
	settings.ScheduleCompareMode = normalizeProfileChoice(settings.ScheduleCompareMode, []string{"none", "name", "resolved"}, defaults.ScheduleCompareMode)
	settings.TimeView = normalizeProfileChoice(settings.TimeView, []string{"day", "week", "month", "year", "duration"}, defaults.TimeView)
	settings.ScaleMode = normalizeProfileChoice(settings.ScaleMode, []string{"auto", "shared", "design_peak", "multiplier_0_1", "percentile"}, defaults.ScaleMode)
	settings.ApplyBehavior.DefaultMode = normalizeProfileChoice(settings.ApplyBehavior.DefaultMode, []string{"clone", "shared"}, defaults.ApplyBehavior.DefaultMode)
	if strings.TrimSpace(settings.ApplyBehavior.NameSuffix) == "" {
		settings.ApplyBehavior.NameSuffix = defaults.ApplyBehavior.NameSuffix
	}
	settings.ApplyBehavior.ReplaceExistingPolicy = normalizeProfileChoice(settings.ApplyBehavior.ReplaceExistingPolicy, []string{"replace", "keep", "duplicate"}, defaults.ApplyBehavior.ReplaceExistingPolicy)
	return settings
}
func normalizeProfileChoice(value string, allowed []string, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, choice := range allowed {
		if normalized == choice {
			return normalized
		}
	}
	return fallback
}

func normalizeProfileChoiceList(values []string, fallback []string, allowed map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if !allowed[normalized] || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

func normalizeHexColor(value string, fallback string) string {
	color := strings.ToLower(strings.TrimSpace(value))
	if color == "" {
		return fallback
	}
	if !strings.HasPrefix(color, "#") {
		color = "#" + color
	}
	if len(color) == 4 && isHexColor(color[1:]) {
		return "#" + string(color[1]) + string(color[1]) + string(color[2]) + string(color[2]) + string(color[3]) + string(color[3])
	}
	if len(color) == 7 && isHexColor(color[1:]) {
		return color
	}
	return fallback
}

func isHexColor(value string) bool {
	for _, char := range value {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F') {
			continue
		}
		return false
	}
	return len(value) > 0
}

func loadAppSettings() (string, AppSettings, error) {
	settingsFileMu.Lock()
	defer settingsFileMu.Unlock()
	path, err := appSettingsPath()
	if err != nil {
		return "", AppSettings{}, err
	}
	settings := defaultAppSettings()
	content, err := readSettingsFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			payload, marshalErr := json.MarshalIndent(settings, "", "  ")
			if marshalErr != nil {
				return "", AppSettings{}, marshalErr
			}
			if writeErr := writeSettingsFile(path, append(payload, '\n')); writeErr != nil {
				return "", AppSettings{}, writeErr
			}
			return path, settings, nil
		}
		return "", AppSettings{}, err
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return path, settings, nil
	}
	if err := json.Unmarshal(content, &settings); err != nil {
		return "", AppSettings{}, err
	}
	settings = normalizeAppSettings(settings)
	return path, settings, nil
}

// Replace the complete file only after it has been written successfully. Other
// app instances and settings readers never observe an in-progress JSON write.
func writeSettingsFile(path string, payload []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err = file.Chmod(0o644); err == nil {
		_, err = file.Write(payload)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return retrySettingsFileAccess(func() error { return os.Rename(temporaryPath, path) })
}

func readSettingsFile(path string) ([]byte, error) {
	var payload []byte
	err := retrySettingsFileAccess(func() error {
		var readErr error
		payload, readErr = os.ReadFile(path)
		return readErr
	})
	return payload, err
}

func retrySettingsFileAccess(operation func() error) error {
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		err := operation()
		// Windows briefly denies access during replacement or while another
		// process holds a file handle. Permanent failures remain visible.
		transient := runtime.GOOS == "windows" && (errors.Is(err, syscall.Errno(5)) || errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33)))
		if !transient || !time.Now().Before(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func appSettingsPath() (string, error) {
	root := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if root == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = configDir
	}
	dir := filepath.Join(root, "SemanticIDF")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}
