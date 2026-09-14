package simulation

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Native 25.1 input contract, not schema padding in the source document:
// IdfParser.cc:354-368 raises idf_max_fields to min-fields7; InputProcessor.cc
// supplies missing defaults while retaining NumBlank, which the selected
// capacity branches in BaseboardElectric.cc:213-281 explicitly reject.
func TestEnergyPathConvectiveBaseboardNativeDefaultsAndSelectedCapacity(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		valid        bool
	}{
		{"raw4 design with omitted efficiency", "Baseboard,,HeatingDesignCapacity,100", true},
		{"raw4 default method and omitted efficiency", "Baseboard,,,100", true},
		{"raw4 mixed case method and autosize", "Baseboard,,hEaTiNgDeSiGnCaPaCiTy,aUtOsIzE", true},
		{"raw4 explicit zero design capacity", "Baseboard,,HeatingDesignCapacity,0", true},
		{"raw5 design with omitted tail", "Baseboard,,HeatingDesignCapacity,100,", true},
		{"raw6 design with omitted efficiency", "Baseboard,,HeatingDesignCapacity,100,,", true},
		{"raw7 blank efficiency", "Baseboard,,HeatingDesignCapacity,100,,,", true},
		{"raw7 all optional defaults", "Baseboard,,,Autosize,,,", true},
		{"raw7 explicit efficiency", "Baseboard,,HeatingDesignCapacity,1e3,,,+9.5e-1", true},
		{"raw5 per-area with omitted efficiency", "Baseboard,,capacityperfloorarea,,10", true},
		{"raw6 fractional with omitted efficiency", "Baseboard,,fractionofautosizedheatingcapacity,,,0.5", true},
		{"raw6 explicit zero fraction", "Baseboard,,FractionOfAutosizedHeatingCapacity,,,0", true},
		{"raw7 fraction greater than one", "Baseboard,,FractionOfAutosizedHeatingCapacity,,,2,1", true},
		{"method without selected capacity", "Baseboard,,HeatingDesignCapacity", false},
		{"blank design capacity is not autosize", "Baseboard,,HeatingDesignCapacity,,,,", false},
		{"all blank method and capacity", "Baseboard,,,,,,", false},
		{"negative design capacity", "Baseboard,,HeatingDesignCapacity,-1", false},
		{"numeric autosize sentinel is not IDD autosize", "Baseboard,,HeatingDesignCapacity,-99999", false},
		{"design invalid token", "Baseboard,,HeatingDesignCapacity,100W", false},
		{"design NaN", "Baseboard,,HeatingDesignCapacity,NaN", false},
		{"design infinity", "Baseboard,,HeatingDesignCapacity,+Inf", false},
		{"design overflow", "Baseboard,,HeatingDesignCapacity,1e309", false},
		{"design hex literal", "Baseboard,,HeatingDesignCapacity,0x1p4", false},
		{"design underscore literal", "Baseboard,,HeatingDesignCapacity,1_000", false},
		{"unknown method", "Baseboard,,Autosize,100", false},
		{"selected per-area missing despite other capacity", "Baseboard,,CapacityPerFloorArea,100", false},
		{"selected per-area blank", "Baseboard,,CapacityPerFloorArea,100,,1,1", false},
		{"per-area zero", "Baseboard,,CapacityPerFloorArea,,0", false},
		{"per-area negative", "Baseboard,,CapacityPerFloorArea,,-1", false},
		{"per-area autosize", "Baseboard,,CapacityPerFloorArea,,Autosize", false},
		{"per-area NaN", "Baseboard,,CapacityPerFloorArea,,NaN", false},
		{"selected fraction omitted", "Baseboard,,FractionOfAutosizedHeatingCapacity,100,10", false},
		{"selected fraction blank is not default one", "Baseboard,,FractionOfAutosizedHeatingCapacity,100,10,,1", false},
		{"fraction negative", "Baseboard,,FractionOfAutosizedHeatingCapacity,,,-0.1", false},
		{"fraction autosize", "Baseboard,,FractionOfAutosizedHeatingCapacity,,,Autosize", false},
		{"fraction infinity", "Baseboard,,FractionOfAutosizedHeatingCapacity,,,Inf", false},
		{"efficiency zero", "Baseboard,,HeatingDesignCapacity,100,,,0", false},
		{"efficiency negative", "Baseboard,,HeatingDesignCapacity,100,,,-0.5", false},
		{"efficiency above one", "Baseboard,,HeatingDesignCapacity,100,,,1.01", false},
		{"efficiency invalid token", "Baseboard,,HeatingDesignCapacity,100,,,Autosize", false},
		{"efficiency NaN", "Baseboard,,HeatingDesignCapacity,100,,,NaN", false},
		{"efficiency infinity", "Baseboard,,HeatingDesignCapacity,100,,,Inf", false},
		{"efficiency hex", "Baseboard,,HeatingDesignCapacity,100,,,0x1p-1", false},
		{"eighth nonextensible field", "Baseboard,,HeatingDesignCapacity,100,,,1,0", false},
		{"radiant people tail", "Baseboard,,HeatingDesignCapacity,100,,,1,0,0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := "Version,25.1;\nZone,Office,0,0,0,0,1,4;\n" +
				"ZoneHVAC:EquipmentConnections,Office,List,,,Office Air,;\n" +
				"ZoneHVAC:EquipmentList,List,SequentialLoad,ZoneHVAC:Baseboard:Convective:Electric,Baseboard,1,1,,;\n" +
				energyPathBaseboardConvectiveElectricType + "," + tc.fields + ";"
			doc := parsePurposePlanFixture(t, input)
			before := doc.String()
			targets := energyPathBaseboardTargets(doc)
			if (len(targets) == 1) != tc.valid || len(targets) > 1 {
				t.Fatalf("native selected/default contract valid=%t targets=%+v", tc.valid, targets)
			}
			if tc.valid && (targets[0].ZoneName != "Office" || targets[0].Component.ObjectType != energyPathBaseboardConvectiveElectricType || targets[0].RadiantFraction != 0 || targets[0].PeopleFraction != 0 || len(targets[0].Recipients) != 0) {
				t.Fatalf("default interpretation changed physical ownership: %+v", targets)
			}
			plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
			outputs := map[string]bool{}
			for _, output := range plan.OutputObjects {
				if strings.HasPrefix(output.VariableName, "Zone Baseboard Total ") {
					t.Fatal("default validation revived a phantom Zone-key alias")
				}
				if !strings.HasPrefix(output.VariableName, "Baseboard ") {
					continue
				}
				if !tc.valid || output.KeyValue != "Baseboard" || output.ScopeZoneName != "Office" {
					t.Fatalf("invalid/defaulted owner acquired wrong output: %+v", output)
				}
				outputs[output.VariableName+"/"+output.ReportingFrequency] = true
			}
			if tc.valid && len(outputs) != 8 || !tc.valid && len(outputs) != 0 {
				t.Fatalf("native output cardinality=%d", len(outputs))
			}
			if doc.String() != before {
				t.Fatal("interpreting defaults rewrote or padded original IDF")
			}
			if tc.valid {
				// The additive plan may append outputs, never pad a source object
				// to seven fields or insert a default method/efficiency into it.
				applied, preview := idf.ApplyOutput(doc, PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd))
				if !preview.CanApply {
					t.Fatal("valid default plan cannot be applied")
				}
				for i, object := range doc.Objects {
					if !reflect.DeepEqual(object, applied.Objects[i]) {
						t.Fatalf("defaulted original object %d changed", object.Index)
					}
				}
			}
		})
	}
}
