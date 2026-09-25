package simulation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func simpleVentilationProbeCounts(plan PurposeRunPlan) map[string]int {
	counts := map[string]int{}
	for _, output := range plan.OutputObjects {
		for _, name := range []string{"Heating:Electricity", "Cooling:Electricity", "Heating:NaturalGas"} {
			if strings.EqualFold(output.KeyValue, name) && strings.HasPrefix(strings.ToLower(output.ObjectType), "output:meter") {
				counts[name+"|"+output.ReportingFrequency]++
			}
		}
	}
	return counts
}

func TestEnergyPathSimpleVentilationConditioningProbesBounded(t *testing.T) {
	base := simpleVentilationHandDocument(t).String()
	for _, tc := range []struct {
		name, input, detail string
		purposes            []SimulationPurposeID
		scope               SimulationPurposeScope
		want                int
	}{
		{"eligible", base, PurposeBasicEnergyDetailEnergyPath, []SimulationPurposeID{SimulationPurposeBasicEnergy}, SimulationPurposeScope{}, 1},
		{"selected owner", base, PurposeBasicEnergyDetailEnergyPath, []SimulationPurposeID{SimulationPurposeBasicEnergy}, SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"Intake"}}, 1},
		{"no selected owner", base + "\nZone,Unused;", PurposeBasicEnergyDetailEnergyPath, []SimulationPurposeID{SimulationPurposeBasicEnergy}, SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"Unused"}}, 0},
		{"light", base, PurposeBasicEnergyDetailLight, []SimulationPurposeID{SimulationPurposeBasicEnergy}, SimulationPurposeScope{}, 0},
		{"explain", base, PurposeBasicEnergyDetailExplain, []SimulationPurposeID{SimulationPurposeBasicEnergy}, SimulationPurposeScope{}, 0},
		{"heat drivers", base, PurposeBasicEnergyDetailHeatDrivers, []SimulationPurposeID{SimulationPurposeBasicEnergy}, SimulationPurposeScope{}, 0},
		{"non energy purpose", base, PurposeBasicEnergyDetailEnergyPath, []SimulationPurposeID{SimulationPurposeZoneHeatFlow}, SimulationPurposeScope{}, 0},
		{"no ventilation", "Version,25.1;\nZone,Plain;", PurposeBasicEnergyDetailEnergyPath, []SimulationPurposeID{SimulationPurposeBasicEnergy}, SimulationPurposeScope{}, 0},
		{"unsupported version", strings.Replace(base, "25.1", "24.2", 1), PurposeBasicEnergyDetailEnergyPath, []SimulationPurposeID{SimulationPurposeBasicEnergy}, SimulationPurposeScope{}, 0},
		{"ambiguous namespace", base + "\nZoneList,Intake,Natural;", PurposeBasicEnergyDetailEnergyPath, []SimulationPurposeID{SimulationPurposeBasicEnergy}, SimulationPurposeScope{}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := parsePurposePlanFixture(t, tc.input)
			before := doc.String()
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: tc.purposes, BasicEnergyDetail: tc.detail, Scope: tc.scope})
			counts := simpleVentilationProbeCounts(plan)
			for _, name := range []string{"Heating:Electricity", "Cooling:Electricity", "Heating:NaturalGas"} {
				if counts[name+"|Monthly"] != tc.want || counts[name+"|Hourly"] != 0 {
					t.Errorf("probe %s count=%v, want %d Monthly only", name, counts, tc.want)
				}
			}
			if doc.String() != before {
				t.Fatal("probe planning rewrote original input")
			}
		})
	}
}

func TestEnergyPathSimpleVentilationConditioningProbeReuse(t *testing.T) {
	for _, objectType := range []string{"Output:Meter", "Output:Meter:MeterFileOnly"} {
		t.Run(objectType, func(t *testing.T) {
			input := simpleVentilationHandDocument(t).String()
			for _, name := range []string{"Heating:Electricity", "Cooling:Electricity", "Heating:NaturalGas"} {
				input += "\n" + objectType + "," + name + ",Monthly;"
			}
			doc := parsePurposePlanFixture(t, input)
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
			counts := simpleVentilationProbeCounts(plan)
			for _, name := range []string{"Heating:Electricity", "Cooling:Electricity", "Heating:NaturalGas"} {
				if counts[name+"|Monthly"] != 1 || counts[name+"|Hourly"] != 0 {
					t.Fatalf("duplicated/promoted actual probe transport: %v", counts)
				}
			}
			for _, output := range plan.OutputObjects {
				if counts[output.KeyValue+"|Monthly"] == 0 {
					continue
				}
				wantState := PurposeOutputStateExisting
				if objectType == "Output:Meter:MeterFileOnly" {
					wantState = PurposeOutputStateTemporary
				}
				if output.ObjectType != "Output:Meter" || output.State != wantState || (output.ObjectIndex != nil) != (objectType == "Output:Meter") ||
					output.ScopeZoneName != "" || !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
					t.Errorf("lost exact original probe opener: %#v", output)
				}
			}
			after, preview := idf.ApplyOutput(doc, PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd))
			if !preview.CanApply {
				t.Fatal("probe output application blocked")
			}
			for _, name := range []string{"Heating:Electricity", "Cooling:Electricity", "Heating:NaturalGas"} {
				count := 0
				for _, object := range after.Objects {
					if strings.EqualFold(object.Type, objectType) && len(object.Fields) >= 2 && strings.EqualFold(object.Fields[0].Value, name) && strings.EqualFold(object.Fields[1].Value, "Monthly") {
						count++
					}
				}
				if count != 1 {
					t.Fatalf("executed probe %s count=%d, want exact original once", name, count)
				}
			}
		})
	}
}

func TestEnergyPathSimpleVentilationOriginalConditioningProbes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "VentilationSimpleTest.idf"))
	if err != nil {
		t.Fatal(err)
	}
	if epathRealHash(data) != "e38887ebcbfec13df596bcd77ed1d5965a65a4bd1e0ad026672f449d8fbe1074" {
		t.Fatal("original changed")
	}
	doc := parsePurposePlanFixture(t, string(data))
	before := doc.String()
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	for _, name := range []string{"Heating:Electricity", "Cooling:Electricity", "Heating:NaturalGas"} {
		if simpleVentilationProbeCounts(plan)[name+"|Monthly"] != 1 {
			t.Errorf("original lacks explicit paid-absence probe %s", name)
		}
	}
	for _, service := range idf.AnalyzeHVAC(doc).ServiceModel.ZoneServices {
		for _, path := range service.Paths {
			if path.ServiceKind == "heating" || path.ServiceKind == "cooling" {
				t.Fatal("an observation probe created a conditioning service")
			}
		}
	}
	if doc.String() != before {
		t.Fatal("original physics modified")
	}
}
