package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Gonie-Gonie/idf-analyzer/internal/epinput"
	"github.com/Gonie-Gonie/idf-analyzer/internal/idf"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
}

type TextEditResult struct {
	Text     string      `json:"text"`
	Format   string      `json:"format"`
	Version  string      `json:"version,omitempty"`
	Report   *idf.Report `json:"report"`
	Warnings []string    `json:"warnings,omitempty"`
}

type InputAnalysisResult struct {
	Text    string         `json:"text,omitempty"`
	Format  string         `json:"format"`
	Version string         `json:"version,omitempty"`
	Model   *epinput.Model `json:"model"`
	EPJSON  string         `json:"epjson,omitempty"`
	Report  *idf.Report    `json:"report"`
}

type ConversionResult struct {
	Text     string   `json:"text"`
	Format   string   `json:"format"`
	Version  string   `json:"version,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type SummaryExportResult struct {
	Text     string `json:"text"`
	Format   string `json:"format"`
	Filename string `json:"filename"`
	MIME     string `json:"mime"`
}

type InputFileResult struct {
	Canceled bool   `json:"canceled,omitempty"`
	Path     string `json:"path,omitempty"`
	Filename string `json:"filename,omitempty"`
	Text     string `json:"text,omitempty"`
}

type SaveFileResult struct {
	Canceled bool   `json:"canceled,omitempty"`
	Path     string `json:"path,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type AppSettings struct {
	Version int `json:"version"`
}

type SettingsResult struct {
	Path     string      `json:"path"`
	Settings AppSettings `json:"settings"`
}

type ModelPatchResult = InputAnalysisResult

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) AnalyzeIDFText(text string) (*idf.Report, error) {
	result, err := a.AnalyzeInputText(text)
	if err != nil {
		return nil, err
	}
	return result.Report, nil
}

func (a *App) AnalyzeInputText(text string) (*InputAnalysisResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	report := idf.Analyze(doc)
	epjsonText, err := epinput.Write(model, epinput.FormatEPJSON)
	if err != nil {
		return nil, err
	}

	return &InputAnalysisResult{
		Text:    text,
		Format:  string(model.Format),
		Version: model.Version.Raw,
		Model:   model,
		EPJSON:  epjsonText,
		Report:  &report,
	}, nil
}

func (a *App) PatchModelValueText(text string, objectIndex int, fieldIndex int, jsonPath []string, rawValue string) (*ModelPatchResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	if err := epinput.PatchFieldValue(model, objectIndex, fieldIndex, jsonPath, rawValue); err != nil {
		return nil, err
	}

	resultText, err := epinput.Write(model, model.Format)
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	report := idf.Analyze(doc)
	epjsonText, err := epinput.Write(model, epinput.FormatEPJSON)
	if err != nil {
		return nil, err
	}

	return &ModelPatchResult{
		Text:    resultText,
		Format:  string(model.Format),
		Version: model.Version.Raw,
		Model:   model,
		EPJSON:  epjsonText,
		Report:  &report,
	}, nil
}

func (a *App) OpenIDF(path string) (*idf.Report, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return a.AnalyzeIDFText(string(content))
}

func (a *App) OpenInputFile() (*InputFileResult, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "Open EnergyPlus input",
		Filters: inputFileFilters(),
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &InputFileResult{Canceled: true}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &InputFileResult{
		Path:     path,
		Filename: filepath.Base(path),
		Text:     string(content),
	}, nil
}

func (a *App) SaveIDF(path string, text string) error {
	return os.WriteFile(path, []byte(text), 0o644)
}

func (a *App) SaveInputFile(path string, text string) (*SaveFileResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return a.SaveInputFileAs(text, "")
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return nil, err
	}
	return &SaveFileResult{Path: path, Filename: filepath.Base(path)}, nil
}

func (a *App) SaveInputFileAs(text string, suggestedFilename string) (*SaveFileResult, error) {
	suggestedFilename = strings.TrimSpace(suggestedFilename)
	if suggestedFilename == "" {
		suggestedFilename = "model.idf"
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Save EnergyPlus input",
		DefaultFilename: suggestedFilename,
		Filters:         inputFileFilters(),
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &SaveFileResult{Canceled: true}, nil
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return nil, err
	}
	return &SaveFileResult{Path: path, Filename: filepath.Base(path)}, nil
}

func (a *App) UpdateFieldText(text string, objectIndex int, fieldIndex int, value string) (*TextEditResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	updated, err := idf.UpdateField(doc, objectIndex, fieldIndex, value)
	if err != nil {
		return nil, err
	}
	resultText := writeDocumentInOriginalFormat(updated, model)
	report := idf.Analyze(updated)
	return &TextEditResult{
		Text:    resultText,
		Format:  string(model.Format),
		Version: model.Version.Raw,
		Report:  &report,
	}, nil
}

func (a *App) RemoveUnusedObjectsText(text string) (*TextEditResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	updated, _ := idf.RemoveUnusedObjects(doc)
	resultText := writeDocumentInOriginalFormat(updated, model)
	report := idf.Analyze(updated)
	return &TextEditResult{
		Text:    resultText,
		Format:  string(model.Format),
		Version: model.Version.Raw,
		Report:  &report,
	}, nil
}

func (a *App) ConvertInputText(text string, targetFormat string) (*ConversionResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}

	target := epinput.Format(targetFormat)
	target = epinput.NormalizeFormat(target)
	output, err := epinput.Write(model, target)
	if err != nil {
		return nil, err
	}

	return &ConversionResult{
		Text:    output,
		Format:  string(target),
		Version: model.Version.Raw,
	}, nil
}

func (a *App) ExportSummaryText(text string, format string) (*SummaryExportResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	summary := idf.AnalyzeSummary(epinput.ToIDFDocument(model))

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		output, err := idf.ExportSummaryJSON(summary)
		if err != nil {
			return nil, err
		}
		return &SummaryExportResult{
			Text:     output,
			Format:   "json",
			Filename: "summary.json",
			MIME:     "application/json",
		}, nil
	case "csv":
		output, err := idf.ExportSummaryCSV(summary)
		if err != nil {
			return nil, err
		}
		return &SummaryExportResult{
			Text:     output,
			Format:   "csv",
			Filename: "summary.csv",
			MIME:     "text/csv",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported summary export format %q; use json or csv", format)
	}
}

func (a *App) GetSummaryMetricGuides() []idf.SummaryGuide {
	return idf.SummaryGuides()
}

func (a *App) GetSettings() (*SettingsResult, error) {
	path, settings, err := loadAppSettings()
	if err != nil {
		return nil, err
	}
	return &SettingsResult{Path: path, Settings: settings}, nil
}

func (a *App) SaveSettings(settings AppSettings) (*SettingsResult, error) {
	if settings.Version == 0 {
		settings.Version = defaultAppSettings().Version
	}
	path, err := appSettingsPath()
	if err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return nil, err
	}
	return &SettingsResult{Path: path, Settings: settings}, nil
}

func writeDocumentInOriginalFormat(doc idf.Document, original *epinput.Model) string {
	if original != nil && original.Format == epinput.FormatEPJSON {
		model := epinput.FromIDFDocument(doc, epinput.FormatEPJSON)
		output, err := epinput.Write(model, epinput.FormatEPJSON)
		if err == nil {
			return output
		}
	}
	return doc.String()
}

func inputFileFilters() []wailsruntime.FileFilter {
	return []wailsruntime.FileFilter{
		{DisplayName: "EnergyPlus input", Pattern: "*.idf;*.imf;*.epjson;*.json;*.txt"},
		{DisplayName: "All files", Pattern: "*.*"},
	}
}

func defaultAppSettings() AppSettings {
	return AppSettings{Version: 1}
}

func loadAppSettings() (string, AppSettings, error) {
	path, err := appSettingsPath()
	if err != nil {
		return "", AppSettings{}, err
	}
	settings := defaultAppSettings()
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			payload, marshalErr := json.MarshalIndent(settings, "", "  ")
			if marshalErr != nil {
				return "", AppSettings{}, marshalErr
			}
			if writeErr := os.WriteFile(path, append(payload, '\n'), 0o644); writeErr != nil {
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
	if settings.Version == 0 {
		settings.Version = defaultAppSettings().Version
	}
	return path, settings, nil
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
	dir := filepath.Join(root, "IDF Analyzer")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}
