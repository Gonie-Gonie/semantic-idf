package simulation

import (
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func heatOnlyFanOutputOriginal(t *testing.T) idf.Document {
	t.Helper()
	raw, err := os.ReadFile("testdata/energy_path_real_models/models/25.1/Furnace.idf")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "2a221defefdffa3792d69924b439f466094ccb4a9fe14aa750e5ffe817fcd1f8" {
		t.Fatal("original Furnace changed")
	}
	doc, err := idf.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func heatOnlyFanOutputObject(t *testing.T, doc *idf.Document, kind, name string) *idf.Object {
	t.Helper()
	for index := range doc.Objects {
		if strings.EqualFold(doc.Objects[index].Type, kind) && strings.EqualFold(purposeObjectName(doc.Objects[index]), name) {
			return &doc.Objects[index]
		}
	}
	t.Fatalf("missing literal original %s/%s", kind, name)
	return nil
}

func TestEnergyPathHeatOnlyFanOutputUsesNativeNestedOwner(t *testing.T) {
	for _, selected := range []string{"", "EAST ZONE", "West Zone"} {
		doc := heatOnlyFanOutputOriginal(t)
		before := doc.String()
		if keys := energyPathAirLoopFanOutputKeys(doc); !reflect.DeepEqual(keys, []string{"Typical Terminal Reheat 1"}) {
			t.Fatalf("native nested owner keys=%v", keys)
		}
		served := map[string]bool{}
		for _, zone := range idf.AnalyzeHVAC(doc).ServiceModel.ZoneServices {
			for _, path := range zone.Paths {
				if path.AirLoop == nil || path.AirLoop.Name != "Typical Terminal Reheat 1" {
					continue
				}
				if path.ServiceKind == "cooling" {
					t.Fatal("HeatOnly invented a Cooling service")
				}
				if path.ServiceKind == "heating" {
					served[strings.ToLower(path.ZoneName)] = true
				}
			}
		}
		if !reflect.DeepEqual(served, map[string]bool{"west zone": true, "east zone": true, "north zone": true}) {
			t.Fatalf("controller must not replace physical recipients: %v", served)
		}
		request := fanOutputRequest()
		if selected != "" {
			request.Scope = SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{selected}}
		}
		plan := BuildPurposeRunPlan(doc, request)
		outputs := fanOutputOnly(plan)
		if len(outputs) != 1 || outputs[0].KeyValue != "Typical Terminal Reheat 1" || outputs[0].ReportingFrequency != "Hourly" || outputs[0].ScopeZoneName != "" || outputs[0].State != PurposeOutputStateTemporary {
			t.Fatalf("exact native AirLoop Hourly/J request missing: %+v", outputs)
		}
		apply := PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd)
		applied, preview := idf.ApplyOutput(doc, apply)
		if !preview.CanApply || len(apply.Updates) != 0 || len(apply.RemoveObjectIndexes) != 0 || doc.String() != before {
			t.Fatal("output request changed original or default model controls")
		}
		for i := range doc.Objects {
			if !reflect.DeepEqual(doc.Objects[i], applied.Objects[i]) {
				t.Fatalf("original object %d modified", i)
			}
		}
		second := fanOutputOnly(BuildPurposeRunPlan(applied, request))
		if len(second) != 1 || second[0].State != PurposeOutputStateExisting {
			t.Fatal("native Furnace fan output duplicated")
		}
	}
}

func TestEnergyPathHeatOnlyFanOutputRejectsUnprovedNestedOwner(t *testing.T) {
	for _, mutation := range []string{"no-conditioning", "disconnected-fan", "missing-control", "wrong-placement", "duplicate-fan", "shared-fan", "shared-wrapper", "missing-terminals"} {
		t.Run(mutation, func(t *testing.T) {
			doc := heatOnlyFanOutputOriginal(t)
			parent := heatOnlyFanOutputObject(t, &doc, "AirLoopHVAC:Unitary:Furnace:HeatOnly", "Gas Furnace 1")
			switch mutation {
			case "no-conditioning":
				parent.Fields[12].Value = "Missing coil"
			case "disconnected-fan":
				heatOnlyFanOutputObject(t, &doc, "Fan:OnOff", "Supply Fan 1").Fields[8].Value = "Unconnected"
			case "missing-control":
				parent.Fields[7].Value = "Missing Zone"
			case "wrong-placement":
				parent.Fields[10].Value = "DrawThrough"
			case "duplicate-fan":
				fanOutputAppend(t, &doc, "Fan:OnOff,Supply Fan 1;")
			case "shared-fan":
				fanOutputAppend(t, &doc, "AirLoopHVAC:Unitary:Furnace:HeatOnly,Other,,In,Out,,80,1,EAST ZONE,Fan:OnOff,Supply Fan 1,BlowThrough,Coil:Heating:Fuel,Other;")
			case "shared-wrapper":
				fanOutputAppend(t, &doc, "Branch,Other,,AirLoopHVAC:Unitary:Furnace:HeatOnly,Gas Furnace 1,In,Out;")
			case "missing-terminals":
				for i := range doc.Objects {
					if strings.EqualFold(doc.Objects[i].Type, "AirTerminal:SingleDuct:ConstantVolume:NoReheat") {
						doc.Objects[i].Fields[2].Value = "Unconnected"
					}
				}
			}
			if outputs := fanOutputOnly(BuildPurposeRunPlan(doc, fanOutputRequest())); len(outputs) != 0 {
				t.Fatalf("unproved nested owner requested pool: %+v", outputs)
			}
		})
	}
	// Other native unitary families and loose Zone fans are not an extension.
	doc, err := idf.Parse("Version,25.1;Zone,Office;Fan:OnOff,Loose,Always On,0.7,100,1,0.9,1,In,Out;")
	if err != nil {
		t.Fatal(err)
	}
	if outputs := fanOutputOnly(BuildPurposeRunPlan(doc, fanOutputRequest())); len(outputs) != 0 {
		t.Fatal("unowned OnOff fan became an AirLoop pool")
	}
}
