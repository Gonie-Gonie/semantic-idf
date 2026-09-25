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

func fanOutputShopOriginal(t *testing.T) idf.Document {
	t.Helper()
	raw, err := os.ReadFile("testdata/energy_path_real_models/models/25.1/ShopWithPVandBattery.idf")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != "9c3f9b637b1e04c2e4b8911854c36ffd6442ea86cfe8e165fccaaeb8b2be9cd0" {
		t.Fatalf("original Shop changed: %s", got)
	}
	doc, err := idf.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func fanOutputLiteralKeys() []string {
	return []string{"ZN_1_FLR_1_SEC_1:Sys", "ZN_1_FLR_1_SEC_2:Sys", "ZN_1_FLR_1_SEC_3:Sys", "ZN_1_FLR_1_SEC_4:Sys", "ZN_1_FLR_1_SEC_5:Sys"}
}

func fanOutputOnly(plan PurposeRunPlan) []PurposeOutputObject {
	var out []PurposeOutputObject
	for _, output := range plan.OutputObjects {
		if strings.EqualFold(output.ObjectType, "Output:Variable") && strings.EqualFold(output.VariableName, "Air System Fan Electricity Energy") {
			out = append(out, output)
		}
	}
	return out
}

func fanOutputAppend(t *testing.T, doc *idf.Document, text string) {
	t.Helper()
	if strings.TrimSpace(text) == "" {
		return
	}
	extra, err := idf.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range extra.Objects {
		object.Index = len(doc.Objects)
		doc.Objects = append(doc.Objects, object)
	}
}

func fanOutputRequest() SimulationPurposeRequest {
	return SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
}

func TestEnergyPathFanPoolOutputPlanShopFiveLiteralHourlyKeys(t *testing.T) {
	for _, scope := range []SimulationPurposeScope{{}, {ZoneMode: "selected", ZoneNames: []string{"ZN_1_FLR_1_SEC_3"}}} {
		doc := fanOutputShopOriginal(t)
		before := doc.String()
		if got := energyPathAirLoopFanOutputKeys(doc); !reflect.DeepEqual(got, fanOutputLiteralKeys()) {
			t.Fatalf("literal original supply fan/terminal owners: got %v", got)
		}
		request := fanOutputRequest()
		request.Scope = scope
		plan := BuildPurposeRunPlan(doc, request)
		outputs := fanOutputOnly(plan)
		if len(outputs) != 5 {
			t.Fatalf("Energy Path must request five native loop pools, got %+v", outputs)
		}
		for index, output := range outputs {
			if output.KeyValue != fanOutputLiteralKeys()[index] || output.ReportingFrequency != "Hourly" || output.ScopeZoneName != "" || output.ObjectIndex != nil || output.State != PurposeOutputStateTemporary || output.Reason != "Basic Energy Path" || !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
				t.Fatalf("wrong native pool request: %+v", output)
			}
		}
		apply := PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd)
		if len(apply.Updates) != 0 || len(apply.RemoveObjectIndexes) != 0 {
			t.Fatal("fan observation plan may only add run-copy output objects")
		}
		applied, preview := idf.ApplyOutput(doc, apply)
		if !preview.CanApply {
			t.Fatalf("run-copy add failed: %+v", preview)
		}
		for index := range doc.Objects {
			if !reflect.DeepEqual(doc.Objects[index], applied.Objects[index]) {
				t.Fatalf("original physical/output object %d changed", index)
			}
		}
		if doc.String() != before {
			t.Fatal("input document mutated")
		}
		for _, output := range fanOutputOnly(BuildPurposeRunPlan(applied, request)) {
			if output.State != PurposeOutputStateExisting || output.ObjectIndex == nil {
				t.Fatalf("second plan duplicates existing loop output: %+v", output)
			}
		}
	}
}

func TestEnergyPathFanPoolOutputPlanReusesOnlyCompleteHourlyRequests(t *testing.T) {
	for _, tc := range []struct {
		name, original     string
		wantTotal, wantNew int
	}{
		{"wildcard", "Output:Variable,*,Air System Fan Electricity Energy,Hourly;", 1, 0},
		{"blank wildcard", "Output:Variable,,Air System Fan Electricity Energy,Hourly;", 1, 0},
		{"blank wildcard and schedule", "Output:Variable,,Air System Fan Electricity Energy,Hourly,;", 1, 0},
		{"duplicate wildcard preserved", "Output:Variable,*,Air System Fan Electricity Energy,Hourly;Output:Variable,*,Air System Fan Electricity Energy,Hourly;", 1, 0},
		{"one exact", "Output:Variable,ZN_1_FLR_1_SEC_1:Sys,Air System Fan Electricity Energy,Hourly;", 5, 4},
		{"explicit blank schedule", "Output:Variable,ZN_1_FLR_1_SEC_1:Sys,Air System Fan Electricity Energy,Hourly,;", 5, 4},
		{"monthly not hourly", "Output:Variable,*,Air System Fan Electricity Energy,Monthly;", 5, 5},
		{"scheduled not complete", "Output:Variable,*,Air System Fan Electricity Energy,Hourly,OnOnly;", 5, 5},
		{"physical fan not loop", "Output:Variable,ZN_1_FLR_1_SEC_1:Sys Fan,Fan Electricity Energy,Hourly;", 5, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := fanOutputShopOriginal(t)
			fanOutputAppend(t, &doc, tc.original)
			before := doc.String()
			outputs := fanOutputOnly(BuildPurposeRunPlan(doc, fanOutputRequest()))
			added := 0
			for _, output := range outputs {
				if output.ReportingFrequency != "Hourly" || strings.TrimSpace(purposeFieldValue(output.Fields, "Schedule Name")) != "" {
					t.Fatalf("incomplete original covers Hourly: %+v", output)
				}
				if output.State != PurposeOutputStateExisting {
					added++
				} else if output.ObjectIndex == nil {
					t.Fatalf("reused literal output lost its original navigation index: %+v", output)
				}
			}
			if len(outputs) != tc.wantTotal || added != tc.wantNew || doc.String() != before {
				t.Fatalf("request count/new or original changed: %d/%d want %d/%d", len(outputs), added, tc.wantTotal, tc.wantNew)
			}
		})
	}
	// Custom outputs are built before the late Energy Path request hook.
	doc := fanOutputShopOriginal(t)
	request := fanOutputRequest()
	request.Purposes = append(request.Purposes, SimulationPurposeCustomOutputs)
	request.Scope.CustomOutputs = []PurposeCustomOutput{{ObjectType: "Output:Variable", KeyValue: "*", VariableName: "Air System Fan Electricity Energy", ReportingFrequency: "Hourly"}}
	if outputs := fanOutputOnly(BuildPurposeRunPlan(doc, request)); len(outputs) != 1 || outputs[0].KeyValue != "*" || !purposeIDsContain(outputs[0].PurposeIDs, SimulationPurposeBasicEnergy) {
		t.Fatalf("pending custom wildcard duplicated: %+v", outputs)
	}
	request.Scope.CustomOutputs[0].KeyValue = ""
	if outputs := fanOutputOnly(BuildPurposeRunPlan(doc, request)); len(outputs) != 1 || !purposeIDsContain(outputs[0].PurposeIDs, SimulationPurposeBasicEnergy) {
		t.Fatalf("pending custom blank wildcard duplicated: %+v", outputs)
	}
}

func TestEnergyPathFanPoolOutputPlanPreservesLiteralBlankPendingKey(t *testing.T) {
	builder := newPurposePlanBuilder(fanOutputShopOriginal(t), fanOutputRequest())
	fields := []idf.OutputFieldValue{{Name: "Key Value", Value: ""}, {Name: "Variable Name", Value: "Air System Fan Electricity Energy"}, {Name: "Reporting Frequency", Value: "Hourly"}}
	builder.objects = []PurposeOutputObject{{ObjectType: "Output:Variable", KeyValue: "", VariableName: "Air System Fan Electricity Energy", ReportingFrequency: "Hourly", Fields: append([]idf.OutputFieldValue(nil), fields...), PurposeIDs: []SimulationPurposeID{SimulationPurposeCustomOutputs}, State: PurposeOutputStateTemporary}}
	builder.addEnergyPathFanPoolOutputs()
	builder.addEnergyPathFanPoolOutputs()
	if len(builder.objects) != 1 {
		t.Fatalf("native blank wildcard duplicated: %+v", builder.objects)
	}
	output := builder.objects[0]
	if output.KeyValue != "" || !reflect.DeepEqual(output.Fields, fields) || output.ObjectIndex != nil || !purposeIDsContain(output.PurposeIDs, SimulationPurposeCustomOutputs) || !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
		t.Fatalf("native blank literal or purpose provenance changed: %+v", output)
	}
}

func TestEnergyPathFanPoolOutputPlanDoesNotGuessInvalidOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, removeType, removeName, appendText string
		want                                     int
	}{
		{"missing fan", "Fan:ConstantVolume", "ZN_1_FLR_1_SEC_1:Sys Fan", "", 4},
		{"missing terminal", "AirTerminal:SingleDuct:ConstantVolume:NoReheat", "", "", 0},
		{"missing branch list", "BranchList", "", "", 0},
		{"missing Zone equipment connection", "ZoneHVAC:EquipmentConnections", "", "", 0},
		{"duplicate loop identity", "", "", "AirLoopHVAC,ZN_1_FLR_1_SEC_1:Sys;", 0},
		{"duplicate fan identity", "", "", "Fan:ConstantVolume,ZN_1_FLR_1_SEC_1:Sys Fan;", 0},
		{"orphan fan", "", "", "Fan:ConstantVolume,Loose,Always On,0.7,100,1,0.9,1,In,Out;", 0},
		{"mixed exhaust fan", "", "", "Fan:ZoneExhaust,Exhaust,Always On,0.7,100,1,Zone Exhaust,Outside;", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := fanOutputShopOriginal(t)
			kept := doc.Objects[:0]
			for _, object := range doc.Objects {
				if strings.EqualFold(object.Type, tc.removeType) && (tc.removeName == "" || strings.EqualFold(purposeObjectName(object), tc.removeName)) {
					continue
				}
				object.Index = len(kept)
				kept = append(kept, object)
			}
			doc.Objects = kept
			fanOutputAppend(t, &doc, tc.appendText)
			outputs := fanOutputOnly(BuildPurposeRunPlan(doc, fanOutputRequest()))
			if len(outputs) != tc.want {
				t.Fatalf("invalid ownership requested a pool: got %d want %d: %+v", len(outputs), tc.want, outputs)
			}
		})
	}
	zoneOnly, err := idf.Parse("Version,25.1;Zone,Office;Fan:ZoneExhaust,Exhaust,Always On,0.7,100,1,Office Exhaust,Outside;Fan:ConstantVolume,Loose,Always On,0.7,100,1,0.9,1,In,Out;")
	if err != nil {
		t.Fatal(err)
	}
	if outputs := fanOutputOnly(BuildPurposeRunPlan(zoneOnly, fanOutputRequest())); len(outputs) != 0 {
		t.Fatalf("Zone-only/unowned fans became AirLoop pools: %+v", outputs)
	}
}

func TestEnergyPathFanPoolOutputPlanDoesNotChangeOtherPurposes(t *testing.T) {
	for _, request := range []SimulationPurposeRequest{
		{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailLight},
		{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailExplain},
		{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}},
	} {
		if outputs := fanOutputOnly(BuildPurposeRunPlan(fanOutputShopOriginal(t), request)); len(outputs) != 0 {
			t.Fatalf("fan pool request escaped Energy Path: %+v", outputs)
		}
	}
}
