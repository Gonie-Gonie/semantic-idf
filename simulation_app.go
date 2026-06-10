package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/idf-analyzer/internal/epinput"
	"github.com/Gonie-Gonie/idf-analyzer/internal/idf"
	"github.com/Gonie-Gonie/idf-analyzer/internal/simulation"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type SimulationRunRequest = simulation.SimulationRunRequest
type MultiSimulationRequest = simulation.MultiSimulationRequest

func (a *App) GetSimulationEnvironment() (*simulation.SimulationEnvironment, error) {
	_, settings, err := loadAppSettings()
	if err != nil {
		return nil, err
	}
	return simulation.BuildEnvironment(settings.Simulation), nil
}

func (a *App) SelectEnergyPlusExecutable() (*simulation.EnergyPlusInstallSetting, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("desktop runtime is not ready")
	}
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select EnergyPlus executable",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "EnergyPlus executable", Pattern: "*.exe"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &simulation.EnergyPlusInstallSetting{}, nil
	}
	install := simulation.EnergyPlusInstallFromExecutable(path, false)
	return &install, nil
}

func (a *App) SelectWeatherFile() (*simulation.WeatherFile, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("desktop runtime is not ready")
	}
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select EnergyPlus weather file",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "EnergyPlus weather", Pattern: "*.epw"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &simulation.WeatherFile{}, nil
	}
	file := simulation.WeatherFileFromPath(path, "User selected")
	return &file, nil
}

func (a *App) SelectWeatherDirectory() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("desktop runtime is not ready")
	}
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:                "Select extra weather data directory",
		CanCreateDirectories: false,
	})
}

func (a *App) SelectSimulationInputFiles() (*simulation.SimulationFileSelectionResult, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("desktop runtime is not ready")
	}
	paths, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "Select EnergyPlus inputs",
		Filters: inputFileFilters(),
	})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return &simulation.SimulationFileSelectionResult{Canceled: true}, nil
	}
	sort.Strings(paths)
	return &simulation.SimulationFileSelectionResult{Paths: paths}, nil
}

func (a *App) SelectSimulationInputFolder(recursive bool) (*simulation.SimulationFileSelectionResult, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("desktop runtime is not ready")
	}
	root, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:                "Select folder containing EnergyPlus inputs",
		CanCreateDirectories: false,
	})
	if err != nil {
		return nil, err
	}
	if root == "" {
		return &simulation.SimulationFileSelectionResult{Canceled: true}, nil
	}
	paths, err := simulation.FindInputFiles(root, recursive)
	if err != nil {
		return nil, err
	}
	return &simulation.SimulationFileSelectionResult{Paths: paths, RootDirectory: root}, nil
}

func (a *App) RunSimulationText(request simulation.SimulationRunRequest) (*simulation.SimulationRunResult, error) {
	_, settings, err := loadAppSettings()
	if err != nil {
		return nil, err
	}
	if request.WeatherPath == "" {
		requiresWeather, err := simulationRequestRequiresWeatherFile(request)
		if err != nil {
			return nil, err
		}
		if requiresWeather {
			return blockedSimulationResult(request, "This IDF uses weather-file design days or weather run periods. Select an EPW weather file before running."), nil
		}
	}
	if request.PurposeRequest != nil {
		request, err = preparePurposeSimulationRequest(request)
		if err != nil {
			return nil, err
		}
	} else if request.StandardOutput {
		request, err = prepareStandardOutputSimulationRequest(request)
		if err != nil {
			return nil, err
		}
	}
	progress := func(item simulation.SimulationProgress) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "idfAnalyzer:simulationProgress", item)
		}
	}
	if request.Filename == "" && request.InputPath != "" {
		request.Filename = filepath.Base(request.InputPath)
	}
	return simulation.RunSimulation(request, progress, settings.Simulation)
}

func (a *App) RunPurposeSimulationText(request simulation.SimulationRunRequest) (*simulation.SimulationRunResult, error) {
	if request.PurposeRequest == nil {
		purposeRequest := simulation.NormalizeSimulationPurposeRequest(nil)
		request.PurposeRequest = &purposeRequest
	}
	return a.RunSimulationText(request)
}

func (a *App) BuildSimulationRunPlan(request simulation.SimulationRunRequest) (*simulation.PurposeRunPlan, error) {
	model, doc, err := simulationRequestModelAndDocument(request)
	if err != nil {
		return nil, err
	}
	_ = model
	purposeRequest := simulation.NormalizeSimulationPurposeRequest(request.PurposeRequest)
	plan := simulation.BuildPurposeRunPlan(doc, purposeRequest)
	return &plan, nil
}

func (a *App) DiscoverAvailableOutputs(request simulation.OutputDiscoveryRequest) (*simulation.OutputDiscoveryResult, error) {
	result, err := simulation.DiscoverAvailableOutputs(request)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *App) ApplyPurposeOutputsText(text string, request simulation.SimulationPurposeRequest) (*OutputApplyTextResult, error) {
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, err
	}
	doc := epinput.ToIDFDocument(model)
	request = simulation.NormalizeSimulationPurposeRequest(&request)
	plan := simulation.BuildPurposeRunPlan(doc, request)
	updated, preview := idf.ApplyOutput(doc, simulation.PurposeRunPlanApplyRequest(plan))
	if !preview.CanApply {
		return nil, fmt.Errorf("purpose output preview has blocking warnings")
	}
	resultText := writeOutputDocumentInOriginalFormat(text, updated, model)
	updatedModel, err := epinput.Parse("", []byte(resultText))
	if err != nil {
		return nil, err
	}
	updatedDoc := epinput.ToIDFDocument(updatedModel)
	report := idf.Analyze(updatedDoc)
	epjsonText, err := epinput.Write(updatedModel, epinput.FormatEPJSON)
	if err != nil {
		return nil, err
	}
	return &OutputApplyTextResult{
		Text:     resultText,
		Format:   string(updatedModel.Format),
		Version:  updatedModel.Version.Raw,
		Model:    updatedModel,
		EPJSON:   epjsonText,
		Semantic: semanticProjectionForModelDoc(updatedModel, updatedDoc),
		Report:   &report,
		Preview:  preview,
	}, nil
}

func blockedSimulationResult(request simulation.SimulationRunRequest, message string) *simulation.SimulationRunResult {
	return &simulation.SimulationRunResult{
		RunID:                    request.RunID,
		Status:                   "blocked",
		InputPath:                strings.TrimSpace(request.InputPath),
		Filename:                 strings.TrimSpace(request.Filename),
		WeatherPath:              strings.TrimSpace(request.WeatherPath),
		EnergyPlusExecutablePath: strings.TrimSpace(request.EnergyPlusExecutablePath),
		ExitCode:                 -1,
		Error:                    message,
	}
}

func simulationRequestModelAndDocument(request simulation.SimulationRunRequest) (*epinput.Model, idf.Document, error) {
	text := request.Text
	if strings.TrimSpace(text) == "" && strings.TrimSpace(request.InputPath) != "" {
		content, err := os.ReadFile(request.InputPath)
		if err != nil {
			return nil, idf.Document{}, err
		}
		text = string(content)
	}
	if strings.TrimSpace(text) == "" {
		return nil, idf.Document{}, fmt.Errorf("simulation run plan needs input text or an input path")
	}
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return nil, idf.Document{}, err
	}
	return model, epinput.ToIDFDocument(model), nil
}

func simulationRequestRequiresWeatherFile(request simulation.SimulationRunRequest) (bool, error) {
	text := request.Text
	if strings.TrimSpace(text) == "" && strings.TrimSpace(request.InputPath) != "" {
		content, err := os.ReadFile(request.InputPath)
		if err != nil {
			return false, err
		}
		text = string(content)
	}
	if strings.TrimSpace(text) == "" {
		return false, nil
	}
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return false, err
	}
	return idf.InputRequiresWeatherFile(epinput.ToIDFDocument(model)), nil
}

func preparePurposeSimulationRequest(request simulation.SimulationRunRequest) (simulation.SimulationRunRequest, error) {
	text := request.Text
	if strings.TrimSpace(text) == "" && strings.TrimSpace(request.InputPath) != "" {
		content, err := os.ReadFile(request.InputPath)
		if err != nil {
			return request, err
		}
		text = string(content)
	}
	if strings.TrimSpace(text) == "" {
		return request, fmt.Errorf("purpose run needs input text or an input path")
	}
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return request, err
	}
	doc := epinput.ToIDFDocument(model)
	purposeRequest := simulation.NormalizeSimulationPurposeRequest(request.PurposeRequest)
	plan := simulation.BuildPurposeRunPlan(doc, purposeRequest)
	updated, preview := idf.ApplyOutput(doc, simulation.PurposeRunPlanApplyRequest(plan))
	if !preview.CanApply {
		return request, fmt.Errorf("purpose output plan has blocking warnings")
	}
	request.Text = writeOutputDocumentInOriginalFormat(text, updated, model)
	request.PurposeRequest = &purposeRequest
	request.PurposeRunPlan = &plan
	request.TemporaryOutputDiff = simulation.PurposeRunPlanTemporaryOutputDiff(plan)
	if strings.TrimSpace(request.ResultMode) == "" {
		request.ResultMode = "sql_first"
	}
	return request, nil
}

func prepareStandardOutputSimulationRequest(request simulation.SimulationRunRequest) (simulation.SimulationRunRequest, error) {
	text := request.Text
	if strings.TrimSpace(text) == "" && strings.TrimSpace(request.InputPath) != "" {
		content, err := os.ReadFile(request.InputPath)
		if err != nil {
			return request, err
		}
		text = string(content)
	}
	if strings.TrimSpace(text) == "" {
		return request, fmt.Errorf("standard output run needs input text or an input path")
	}
	model, err := epinput.Parse("", []byte(text))
	if err != nil {
		return request, err
	}
	doc := epinput.ToIDFDocument(model)
	mode := strings.TrimSpace(request.StandardOutputMode)
	if mode == "" {
		mode = "replace"
	}
	updated, preview := idf.ApplyOutput(doc, idf.StandardOutputApplyRequest(doc, mode))
	if !preview.CanApply {
		return request, fmt.Errorf("standard output preset has blocking warnings")
	}
	request.Text = writeOutputDocumentInOriginalFormat(text, updated, model)
	return request, nil
}

func (a *App) RunMultipleSimulations(request simulation.MultiSimulationRequest) (*simulation.MultiSimulationResult, error) {
	_, settings, err := loadAppSettings()
	if err != nil {
		return nil, err
	}
	progress := func(item simulation.SimulationProgress) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "idfAnalyzer:multiSimulationProgress", item)
		}
	}
	return simulation.RunMultipleSimulations(request, progress, settings.Simulation)
}
