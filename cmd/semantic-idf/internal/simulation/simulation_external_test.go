package simulation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalEnergyPlusSmoke(t *testing.T) {
	if strings.TrimSpace(os.Getenv("SEMANTIC_IDF_RUN_EP_SMOKE")) != "1" {
		t.Skip("set SEMANTIC_IDF_RUN_EP_SMOKE=1 to run the external EnergyPlus smoke test")
	}

	executable := strings.TrimSpace(os.Getenv("SEMANTIC_IDF_ENERGYPLUS_EXE"))
	if executable == "" {
		for _, install := range AutoDetectEnergyPlusInstallations() {
			if install.ExecutablePath != "" {
				executable = install.ExecutablePath
				break
			}
		}
	}
	if executable == "" {
		t.Skip("EnergyPlus executable was not found")
	}
	root := filepath.Dir(executable)
	example := envOrDefaultPath("SEMANTIC_IDF_ENERGYPLUS_EXAMPLE", filepath.Join(root, "ExampleFiles", "1ZoneUncontrolled.idf"))
	weather := envOrDefaultPath("SEMANTIC_IDF_ENERGYPLUS_WEATHER", filepath.Join(root, "WeatherData", "USA_IL_Chicago-OHare.Intl.AP.725300_TMY3.epw"))
	for _, path := range []string{executable, example, weather} {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("required EnergyPlus smoke-test file is unavailable: %s", path)
		}
	}

	result, err := RunSimulation(SimulationRunRequest{
		RunID:                    "external-smoke",
		InputPath:                example,
		Filename:                 filepath.Base(example),
		EnergyPlusExecutablePath: executable,
		WeatherPath:              weather,
		OutputDirectory:          t.TempDir(),
		Silent:                   true,
	}, nil, DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("simulation status = %s, error = %s, stdout = %s, stderr = %s", result.Status, result.Error, result.Stdout, result.Stderr)
	}
	if result.ERR.Path == "" {
		t.Fatalf("ERR summary was not parsed: %+v", result.Files)
	}
	if filepath.Base(result.ERR.Path) != "eplusout.err" {
		t.Fatalf("ERR filename = %s, want eplusout.err", filepath.Base(result.ERR.Path))
	}
	if len(result.CSVs) == 0 {
		t.Fatalf("no CSV outputs were parsed from %s", result.OutputDirectory)
	}
}

func envOrDefaultPath(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
