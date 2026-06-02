package main

import (
	"embed"
	"encoding/json"
	"net/http"

	"github.com/Gonie-Gonie/idf-analyzer/internal/idf"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/src
var assets embed.FS

//go:embed wails.json
var wailsConfigBytes []byte

type AppInfo struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	Title          string `json:"title"`
	OutputFilename string `json:"outputFilename"`
}

type wailsAppConfig struct {
	Name           string `json:"name"`
	OutputFilename string `json:"outputfilename"`
	Info           struct {
		ProductName    string `json:"productName"`
		ProductVersion string `json:"productVersion"`
	} `json:"info"`
}

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  currentAppInfo().Title,
		Width:  1600,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: appAssetHandler(app),
		},
		BackgroundColour: &options.RGBA{R: 247, G: 249, B: 251, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}

func currentAppInfo() AppInfo {
	info := AppInfo{
		Name:           "IDF Analyzer",
		Version:        "0.0.0",
		Title:          "IDF Analyzer v0.0.0",
		OutputFilename: "idf-analyzer",
	}

	var config wailsAppConfig
	if err := json.Unmarshal(wailsConfigBytes, &config); err != nil {
		return info
	}

	if config.Info.ProductName != "" {
		info.Name = config.Info.ProductName
	} else if config.Name != "" {
		info.Name = config.Name
	}
	if config.Info.ProductVersion != "" {
		info.Version = config.Info.ProductVersion
	}
	if config.OutputFilename != "" {
		info.OutputFilename = config.OutputFilename
	}
	info.Title = info.Name
	if info.Version != "" {
		info.Title += " v" + info.Version
	}
	return info
}

func appAssetHandler(app *App) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/app-info":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := json.NewEncoder(w).Encode(currentAppInfo()); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/summary-metric-guides":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := json.NewEncoder(w).Encode(idf.SummaryGuides()); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/settings":
			if r.Method != http.MethodGet && r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if r.Method == http.MethodPost {
				var settings AppSettings
				if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				result, err := app.SaveSettings(settings)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if err := json.NewEncoder(w).Encode(result); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			path, settings, err := loadAppSettings()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(SettingsResult{Path: path, Settings: settings}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/multi-idf-summary":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				RunID string `json:"runId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.AnalyzeMultiIDFSummary(request.RunID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/cleanup-scan":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Text     string `json:"text"`
				Path     string `json:"path"`
				Filename string `json:"filename"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.ScanCleanupText(request.Text, request.Path, request.Filename)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/field-suggestions":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Text        string `json:"text"`
				ObjectIndex int    `json:"objectIndex"`
				FieldIndex  int    `json:"fieldIndex"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.SuggestFieldValuesText(request.Text, request.ObjectIndex, request.FieldIndex)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/cleanup-preview":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Text                  string   `json:"text"`
				RuleIDs               []string `json:"ruleIds"`
				ExcludedCandidateKeys []string `json:"excludedCandidateKeys"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.PreviewCleanupText(request.Text, request.RuleIDs, request.ExcludedCandidateKeys)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/cleanup-save-as":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Text                  string   `json:"text"`
				SuggestedFilename     string   `json:"suggestedFilename"`
				RuleIDs               []string `json:"ruleIds"`
				ExcludedCandidateKeys []string `json:"excludedCandidateKeys"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.SaveCleanupAs(request.Text, request.SuggestedFilename, request.RuleIDs, request.ExcludedCandidateKeys)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/cleanup-save":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Path                  string   `json:"path"`
				Text                  string   `json:"text"`
				RuleIDs               []string `json:"ruleIds"`
				ExcludedCandidateKeys []string `json:"excludedCandidateKeys"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.SaveCleanupToFile(request.Path, request.Text, request.RuleIDs, request.ExcludedCandidateKeys)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/profile-apply-preview":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Text  string                  `json:"text"`
				Apply idf.ProfileApplyRequest `json:"apply"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.PreviewProfileApplyText(request.Text, request.Apply)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/profile-apply":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Text  string                  `json:"text"`
				Apply idf.ProfileApplyRequest `json:"apply"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.ApplyProfileText(request.Text, request.Apply)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/hvac-apply-preview":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Text  string               `json:"text"`
				Apply idf.HVACApplyRequest `json:"apply"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.PreviewHVACApplyText(request.Text, request.Apply)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/hvac-apply":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Text  string               `json:"text"`
				Apply idf.HVACApplyRequest `json:"apply"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.ApplyHVACText(request.Text, request.Apply)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/output-apply-preview":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Text  string                 `json:"text"`
				Apply idf.OutputApplyRequest `json:"apply"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.PreviewOutputApplyText(request.Text, request.Apply)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/output-apply":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Text  string                 `json:"text"`
				Apply idf.OutputApplyRequest `json:"apply"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.ApplyOutputText(request.Text, request.Apply)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/simulation-environment":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			result, err := app.GetSimulationEnvironment()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/simulation-run":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request SimulationRunRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.RunSimulationText(request)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/api/multi-simulation-run":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request MultiSimulationRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := app.RunMultipleSimulations(request)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(result); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		default:
			http.NotFound(w, r)
		}
	})
}
