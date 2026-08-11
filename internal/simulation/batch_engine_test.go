package simulation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveBatchEnergyPlusExecutableSelectsMatchingVersionPerInput(t *testing.T) {
	directory := t.TempDir()
	idfPath := writeBatchEngineInput(t, directory, "model.idf", "Version, 24.2;\nZone, Office;\n")
	epJSONPath := writeBatchEngineInput(t, directory, "model.epJSON", `{
  "Version": {
    "Version 1": {
      "version_identifier": "25.1"
    }
  },
  "Zone": {
    "Office": {}
  }
}`)
	installations := []EnergyPlusInstallSetting{
		{Version: "25.1.0", ExecutablePath: filepath.Join(directory, "EnergyPlus-25-1")},
		{Version: "24.2", ExecutablePath: filepath.Join(directory, "EnergyPlus-24-2")},
	}

	idfResolution := resolveBatchEnergyPlusExecutable(idfPath, "", installations)
	if idfResolution.Error != "" || idfResolution.ExecutablePath != installations[1].ExecutablePath {
		t.Fatalf("IDF engine resolution = %#v, want %q", idfResolution, installations[1].ExecutablePath)
	}
	epJSONResolution := resolveBatchEnergyPlusExecutable(epJSONPath, "", installations)
	if epJSONResolution.Error != "" || epJSONResolution.ExecutablePath != installations[0].ExecutablePath {
		t.Fatalf("epJSON engine resolution = %#v, want %q", epJSONResolution, installations[0].ExecutablePath)
	}
}

func TestResolveBatchEnergyPlusExecutableUsesNewestForUnknownInputVersion(t *testing.T) {
	directory := t.TempDir()
	inputPath := writeBatchEngineInput(t, directory, "unknown.idf", "Zone, Office;\n")
	installations := []EnergyPlusInstallSetting{
		{Version: "25.1", ExecutablePath: filepath.Join(directory, "EnergyPlus-25-1")},
		{Version: "24.2", ExecutablePath: filepath.Join(directory, "EnergyPlus-24-2")},
	}

	resolution := resolveBatchEnergyPlusExecutable(inputPath, "", installations)
	if resolution.Error != "" || resolution.ExecutablePath != installations[0].ExecutablePath {
		t.Fatalf("unknown-version engine resolution = %#v, want newest %q", resolution, installations[0].ExecutablePath)
	}
}

func TestResolveBatchEnergyPlusExecutablePreservesExplicitPath(t *testing.T) {
	explicitPath := filepath.Join(t.TempDir(), "explicit-energyplus")
	resolution := resolveBatchEnergyPlusExecutable("missing-input.idf", explicitPath, nil)
	if resolution.Error != "" || resolution.ExecutablePath != explicitPath {
		t.Fatalf("explicit engine resolution = %#v, want %q", resolution, explicitPath)
	}
}

func TestRunMultipleSimulationsPreservesExplicitPathDespiteVersionMismatch(t *testing.T) {
	directory := t.TempDir()
	inputPath := writeBatchEngineInput(t, directory, "future.idf", "Version, 99.1;\nZone, Office;\n")
	explicitPath := filepath.Join(directory, "explicit-energyplus")
	result, err := RunMultipleSimulations(MultiSimulationRequest{
		RunID:                    "batch-explicit-engine",
		InputPaths:               []string{inputPath},
		WorkerCount:              1,
		EnergyPlusExecutablePath: explicitPath,
	}, nil, SimulationSettings{})
	if err != nil {
		t.Fatalf("RunMultipleSimulations returned error: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("batch result = %#v", result)
	}
	item := result.Results[0]
	if item.EnergyPlusExecutablePath != explicitPath || !strings.Contains(item.Error, "EnergyPlus executable was not found") {
		t.Fatalf("explicit-path batch result = %#v", item)
	}
}

func TestRunMultipleSimulationsRejectsMissingCompatibleEnergyPlusBeforeExecution(t *testing.T) {
	directory := t.TempDir()
	inputPath := writeBatchEngineInput(t, directory, "future.idf", "Version, 99.1;\nZone, Office;\n")
	configuredPath := filepath.Join(directory, "EnergyPlus-24-2")
	result, err := RunMultipleSimulations(MultiSimulationRequest{
		RunID:       "batch-version-mismatch",
		InputPaths:  []string{inputPath},
		WorkerCount: 1,
	}, nil, SimulationSettings{
		EnergyPlusInstallations: []EnergyPlusInstallSetting{{Version: "24.2", ExecutablePath: configuredPath}},
	})
	if err != nil {
		t.Fatalf("RunMultipleSimulations returned error: %v", err)
	}
	if result.Completed != 1 || result.Failed != 1 || result.Succeeded != 0 || len(result.Results) != 1 {
		t.Fatalf("batch result = %#v", result)
	}
	item := result.Results[0]
	if item.Status != "missing_energyplus" || item.ExitCode != -1 || item.EnergyPlusExecutablePath != "" {
		t.Fatalf("mismatched-version result = %#v", item)
	}
	wantError := "No compatible EnergyPlus installation is configured for input Version 99.1."
	if item.Error != wantError {
		t.Fatalf("mismatched-version error = %q, want %q", item.Error, wantError)
	}
	if strings.Contains(item.Error, configuredPath) {
		t.Fatalf("mismatched-version case reached executable validation: %q", item.Error)
	}
}

func writeBatchEngineInput(t *testing.T, directory string, name string, content string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write batch engine fixture: %v", err)
	}
	return path
}
