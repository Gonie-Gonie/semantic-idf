package simulation

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathSimpleVentOriginalAutocalculateSurfaces(t *testing.T) {
	original := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "VentilationSimpleTest.idf")
	bytes, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(bytes)); got != "e38887ebcbfec13df596bcd77ed1d5965a65a4bd1e0ad026672f449d8fbe1074" {
		t.Fatalf("pinned original changed: %s", got)
	}
	doc, err := idf.Parse(string(bytes))
	if err != nil {
		t.Fatal(err)
	}
	geometry := idf.AnalyzeGeometry(doc)
	owners := map[string]string{}
	six := map[string]bool{}
	for zone := 1; zone <= 3; zone++ {
		for _, suffix := range []string{"Flr001", "Roof001"} {
			six[fmt.Sprintf("zn%03d:%s", zone, strings.ToLower(suffix))] = true
		}
	}
	shades, opaque := 0, 0
	for _, surface := range geometry.Surfaces {
		if surface.IsShading {
			shades++
			continue
		}
		opaque++
		key := strings.ToLower(surface.Name)
		owners[key] = surface.ZoneName
		if six[key] {
			wantZone := "ZONE " + key[4:5]
			if surface.ZoneName != wantZone || len(surface.RawVertices) != 16 || len(surface.WorldVertices) != 16 ||
				surface.PhysicalArea <= 0 || surface.ZoneMultiplier != 1 ||
				surface.SurfaceMultiplier != 1 || surface.EffectiveArea != surface.PhysicalArea {
				t.Errorf("autocalculate physical owner/vertices/factors lost: %#v", surface)
			}
			if strings.Contains(key, "flr") && (surface.SurfaceType != "Floor" || surface.OutsideBoundary != "Ground") {
				t.Errorf("floor physical boundary = %#v", surface)
			}
			if strings.Contains(key, "roof") && (surface.SurfaceType != "Roof" || surface.OutsideBoundary != "Outdoors") {
				t.Errorf("roof physical boundary = %#v", surface)
			}
		}
	}
	for _, window := range geometry.Windows {
		owners[strings.ToLower(window.Name)] = window.ZoneName
	}
	if opaque != 54 || len(geometry.Windows) != 24 || len(owners) != 78 || shades != 21 {
		t.Fatalf("original physical census opaque/windows/heat-transfer/shades = %d/%d/%d/%d, want 54/24/78/21", opaque, len(geometry.Windows), len(owners), shades)
	}
	for key := range six {
		if owners[key] == "" {
			t.Errorf("missing original autocalculate owner %s", key)
		}
	}
	for _, scope := range []SimulationPurposeScope{{}, {ZoneMode: "selected", ZoneNames: []string{"ZONE 2"}}} {
		plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
			Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
			BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
			Scope:             scope,
		})
		for _, name := range []string{"Surface Inside Face Convection Heat Gain Energy", "Surface Inside Face Convection Heat Gain Rate"} {
			for _, frequency := range []string{"Monthly", "Hourly"} {
				seen := map[string]int{}
				for _, output := range plan.OutputObjects {
					if output.ObjectType != "Output:Variable" || output.VariableName != name || output.ReportingFrequency != frequency {
						continue
					}
					key := strings.ToLower(output.KeyValue)
					seen[key]++
					if owners[key] == "" || (scope.ZoneMode == "selected" && owners[key] != "ZONE 2") {
						t.Errorf("unknown/shading/out-of-scope request %s/%s/%s", key, name, frequency)
					}
					if !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
						t.Errorf("request lacks Energy Path purpose %s", key)
					}
				}
				want := 78
				if scope.ZoneMode == "selected" {
					want = 26
				}
				if len(seen) != want {
					t.Errorf("%s %s/%s owner count = %d, want %d", scope.ZoneMode, name, frequency, len(seen), want)
				}
				for key, owner := range owners {
					if scope.ZoneMode != "selected" || owner == "ZONE 2" {
						if seen[key] != 1 {
							t.Errorf("%s %s/%s/%s request count = %d, want exactly one", scope.ZoneMode, key, name, frequency, seen[key])
						}
					}
				}
			}
		}
	}
}
