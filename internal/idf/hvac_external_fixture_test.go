package idf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type hvacExternalFixture struct {
	Name          string
	Env           string
	Defaults      []string
	WantPathTypes []string
	WantNetworks  []string
}

func TestExternalHVACFixtureMatrix(t *testing.T) {
	if strings.TrimSpace(os.Getenv("SEMANTIC_IDF_RUN_HVAC_FIXTURES")) != "1" {
		t.Skip("set SEMANTIC_IDF_RUN_HVAC_FIXTURES=1 to run the external HVAC fixture matrix")
	}

	home, _ := os.UserHomeDir()
	fixtures := []hvacExternalFixture{
		{
			Name: "HospitalLowEnergy",
			Env:  "SEMANTIC_IDF_HVAC_HOSPITAL_IDF",
			Defaults: []string{
				filepath.Join("testdata", "hvac_external", "HospitalLowEnergy.idf"),
				filepath.Join("C:\\", "EnergyPlusV24-2-0", "ExampleFiles", "HospitalLowEnergy.idf"),
				filepath.Join("C:\\", "EnergyPlusV23-2-0", "ExampleFiles", "HospitalLowEnergy.idf"),
				filepath.Join("C:\\", "EnergyPlusV9-6-0", "ExampleFiles", "HospitalLowEnergy.idf"),
			},
			WantNetworks: []string{"service_water"},
		},
		{
			Name:          "Domestic health-clinic operating IDF",
			Env:           "SEMANTIC_IDF_HVAC_DOMESTIC_HEALTH_IDF",
			Defaults:      []string{filepath.Join("testdata", "hvac_external", "heatfloor.idf"), filepath.Join(home, "Downloads", "heatfloor.idf")},
			WantPathTypes: []string{"radiant"},
		},
		{
			Name: "Domestic childcare operating IDF",
			Env:  "SEMANTIC_IDF_HVAC_DOMESTIC_CHILDCARE_IDF",
			Defaults: []string{
				filepath.Join("testdata", "hvac_external", "Office_1_default.idf"),
				filepath.Join(home, "Downloads", "Office_1_default.idf"),
				filepath.Join(home, "Downloads", "GRSimulator", "GRSimulator", "pyGRsim", "_data", "dummyIDF", "yejeon_jeongsang_file_maybe_expanded.idf"),
			},
			WantPathTypes: []string{"direct_zone_refrigerant"},
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			path := resolveExternalHVACFixturePath(t, fixture)
			text, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			doc, err := Parse(string(text))
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}

			report := AnalyzeHVAC(doc)
			if len(report.ServiceModel.ZoneServices) == 0 && len(report.ServiceModel.Couplings) == 0 && len(report.ServiceModel.Networks) == 0 {
				t.Fatalf("%s produced empty HVAC service model", path)
			}
			for _, pathType := range fixture.WantPathTypes {
				if !serviceModelHasPathType(report.ServiceModel, pathType) {
					t.Fatalf("%s path types = %#v, want %s", fixture.Name, serviceModelPathTypes(report.ServiceModel), pathType)
				}
			}
			for _, networkType := range fixture.WantNetworks {
				if !hasEnergyNetwork(report.ServiceModel, networkType) {
					t.Fatalf("%s networks = %#v, want %s", fixture.Name, report.ServiceModel.Networks, networkType)
				}
			}
		})
	}
}

func resolveExternalHVACFixturePath(t *testing.T, fixture hvacExternalFixture) string {
	t.Helper()
	candidates := append([]string{strings.TrimSpace(os.Getenv(fixture.Env))}, fixture.Defaults...)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return candidate
		}
	}
	t.Fatalf("%s fixture not found; set %s", fixture.Name, fixture.Env)
	return ""
}

func serviceModelHasPathType(model HVACServiceModel, pathType string) bool {
	for _, zone := range model.ZoneServices {
		for _, path := range zone.Paths {
			if path.PathType == pathType {
				return true
			}
		}
	}
	return false
}

func serviceModelPathTypes(model HVACServiceModel) []string {
	seen := map[string]bool{}
	for _, zone := range model.ZoneServices {
		for _, path := range zone.Paths {
			if path.PathType != "" {
				seen[path.PathType] = true
			}
		}
	}
	return sortedStringSet(seen)
}
