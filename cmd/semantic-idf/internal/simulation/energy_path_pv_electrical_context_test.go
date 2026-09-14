package simulation

// Hand-authored reporting-identity and output-plan regressions.
import (
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func pvElectricalDraftOriginal(t *testing.T) idf.Document {
	t.Helper()
	raw, err := os.ReadFile("testdata/energy_path_real_models/models/25.1/ShopWithPVandBattery.idf")
	if err != nil {
		t.Fatal(err)
	}
	if hash := fmt.Sprintf("%x", sha256.Sum256(raw)); hash != "9c3f9b637b1e04c2e4b8911854c36ffd6442ea86cfe8e165fccaaeb8b2be9cd0" {
		t.Fatalf("original changed: %s", hash)
	}
	doc, err := idf.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func pvElectricalDraftAppend(doc *idf.Document, kind string, fields ...string) {
	max := -1
	for _, object := range doc.Objects {
		if object.Index > max {
			max = object.Index
		}
	}
	object := idf.Object{Type: kind, Index: max + 1}
	for _, value := range fields {
		object.Fields = append(object.Fields, idf.Field{Value: value})
	}
	doc.Objects = append(doc.Objects, object)
}

func pvElectricalDraftHand(t *testing.T) idf.Document {
	t.Helper()
	doc, err := idf.Parse(`Version,25.1;
ElectricLoadCenter:Distribution, Center, MissingList, TrackElectrical,,,,DirectCurrentWithInverterDCStorage, Inverter, Battery;
ElectricLoadCenter:Inverter:LookUpTable, Inverter;
ElectricLoadCenter:Storage:Battery, Battery;
Generator:Photovoltaic, PV1,MissingSurface,PhotovoltaicPerformance:Simple,MissingPerformance,Decoupled;
Generator:Photovoltaic, PV2,MissingSurface,PhotovoltaicPerformance:Simple,MissingPerformance,Decoupled;
Generator:Photovoltaic, PV3,MissingSurface,PhotovoltaicPerformance:Simple,MissingPerformance,Decoupled;
Generator:Photovoltaic, PV4,MissingSurface,PhotovoltaicPerformance:Simple,MissingPerformance,Decoupled;
Generator:Photovoltaic, PV5,MissingSurface,PhotovoltaicPerformance:Simple,MissingPerformance,Decoupled;`)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestPVElectricalDraftOriginalTwentyIdentitiesAndFortyIntents(t *testing.T) {
	doc := pvElectricalDraftOriginal(t)
	before := doc.String()
	inventory := energyPathBuildPVElectricalInventory(doc)
	if !inventory.HasOriginal || !inventory.SchemaReviewed || len(inventory.Targets) != 20 || len(inventory.UnreviewedOwners) != 0 {
		t.Fatalf("wrong native20 roster: %+v", inventory)
	}
	seen := map[string]bool{}
	variables, meters := 0, 0
	wantOwner := map[string]int{"PV Array Load Center": 460, "Kibam": 462, "PV Inverter": 465, "PV:ZN_1_FLR_1_SEC_1_Ceiling": 467, "PV:ZN_1_FLR_1_SEC_2_Ceiling": 470, "PV:ZN_1_FLR_1_SEC_3_Ceiling": 472, "PV:ZN_1_FLR_1_SEC_4_Ceiling": 474, "PV:ZN_1_FLR_1_SEC_5_Ceiling": 476}
	ownerCensus := map[string]int{}
	for _, target := range inventory.Targets {
		key := target.Definition.Name + "\x00" + target.Key
		if !target.IdentityValid || seen[key] {
			t.Fatalf("ambiguous/duplicate native identity: %+v", target)
		}
		seen[key] = true
		if target.Definition.IsMeter {
			meters++
			if target.Key != "" || len(target.OriginalOwners) != 0 {
				t.Fatal("global meter invented an equipment owner")
			}
		} else {
			variables++
			if len(target.OriginalOwners) != 1 {
				t.Fatal("original variable owner not unique")
			}
			owner := target.OriginalOwners[0]
			want, found := wantOwner[target.Key]
			if !found || owner.ObjectIndex != want || owner.ID != fmt.Sprintf("component:%d", want) || owner.ObjectName != target.Key || owner.ObjectType != target.Definition.OwnerType {
				t.Fatalf("original/executed/output-owner identities conflated: %+v", target)
			}
			ownerCensus[target.Key]++
		}
		basis, factor, application := target.sourceBasis()
		if basis != "model_total" || factor != 1 || application != "already_model_total" {
			t.Fatal("native source acquired representative Zone factor")
		}
		for _, frequency := range []string{"Monthly", "Hourly"} {
			if !target.matchesNativeDictionary(target.Definition.Name, target.Key, "J", frequency, target.Definition.IsMeter) {
				t.Fatal("exact native dictionary family rejected")
			}
		}
		if target.matchesNativeDictionary(target.Definition.Name, target.Key, "W", "Monthly", target.Definition.IsMeter) || target.matchesNativeDictionary(target.Definition.Name, target.Key, "J", "Timestep", target.Definition.IsMeter) || target.matchesNativeDictionary(target.Definition.Name, "foreign", "J", "Monthly", target.Definition.IsMeter) {
			t.Fatal("wrong unit/frequency/key borrowed native proof")
		}
	}
	if variables != 16 || meters != 4 || len(ownerCensus) != 8 || ownerCensus["PV Inverter"] != 5 || ownerCensus["Kibam"] != 4 || ownerCensus["PV Array Load Center"] != 2 {
		t.Fatalf("16 variable +4 meter roster lost: %d/%d %v", variables, meters, ownerCensus)
	}
	intents := energyPathPVElectricalIntents(inventory)
	if len(intents) != 40 {
		t.Fatalf("M/H native intent census %d, not appended-object count", len(intents))
	}
	seenIntents := map[string]bool{}
	for _, intent := range intents {
		key := intent.Target.Definition.Name + "\x00" + intent.Target.Key + "\x00" + intent.Frequency
		if seenIntents[key] {
			t.Fatal("duplicate intent")
		}
		seenIntents[key] = true
		request := intent.request()
		if request.ScopeZoneName != "" || request.ObjectIndex != nil || request.Reason != "Basic Energy Path" || !purposeIDsContain(request.PurposeIDs, SimulationPurposeBasicEnergy) {
			t.Fatalf("request invented Zone/output owner: %+v", request)
		}
	}
	if before != doc.String() {
		t.Fatal("original modified by target discovery")
	}
}

func TestPVElectricalDraftTopologyAndReportingIdentityAreSeparate(t *testing.T) {
	doc := pvElectricalDraftHand(t)
	inventory := energyPathBuildPVElectricalInventory(doc)
	// Deliberately incomplete physical input: target validity means reporting
	// identity only, not a five-Zone, inverter, battery-model or energy closure.
	if len(inventory.Targets) != 20 || len(energyPathPVElectricalIntents(inventory)) != 40 {
		t.Fatal("disconnected original targets disappeared")
	}
	for _, mutation := range []struct {
		kind, name     string
		badDefinitions int
	}{
		{"Generator:PVWatts", "PV1", 1}, {"Generator:Photovoltaic", "PV1", 1},
		{"ElectricLoadCenter:Inverter:Simple", "Inverter", 5}, {"ElectricLoadCenter:Inverter:PVWatts", "Inverter", 5},
		{"ElectricLoadCenter:Storage:Simple", "Battery", 4}, {"ElectricLoadCenter:Storage:LiIonNMCBattery", "Battery", 4},
		{"ElectricLoadCenter:Distribution", "Center", 2},
	} {
		t.Run(mutation.kind, func(t *testing.T) {
			copy := pvElectricalDraftHand(t)
			pvElectricalDraftAppend(&copy, mutation.kind, mutation.name)
			got := energyPathBuildPVElectricalInventory(copy)
			bad := 0
			for _, target := range got.Targets {
				if !target.IdentityValid {
					bad++
					if len(target.OriginalOwners) != 2 || target.Reason != "unresolved_reporting_owner" {
						t.Fatalf("ambiguous original evidence lost: %+v", target)
					}
				}
			}
			if len(got.Targets) != 20 || bad != mutation.badDefinitions || len(energyPathPVElectricalIntents(got)) != 40-2*bad {
				t.Fatalf("collision not source-local: bad=%d inventory=%+v", bad, got)
			}
		})
	}
	for _, kind := range []string{"Generator:FuelCell", "ElectricLoadCenter:Storage:Converter", "Coil:Heating:Fuel"} {
		copy := pvElectricalDraftHand(t)
		pvElectricalDraftAppend(&copy, kind, "PV1")
		pvElectricalDraftAppend(&copy, kind, "Inverter")
		pvElectricalDraftAppend(&copy, kind, "Battery")
		if got := energyPathBuildPVElectricalInventory(copy); len(energyPathPVElectricalIntents(got)) != 40 {
			t.Fatalf("unrelated same name fabricated a reporting collision: %s", kind)
		}
	}
	if got := energyPathBuildPVElectricalInventory(idf.Document{}); !got.HasOriginal || got.SchemaReviewed || len(energyPathPVElectricalIntents(got)) != 0 {
		t.Fatal("explicit empty original confused with absent original")
	}
	if got := energyPathPVElectricalIntents(energyPathPVElectricalInventory{}); got != nil {
		t.Fatal("no-original legacy unexpectedly opted in")
	}
	for _, version := range []string{"22.1", "26.1", ""} {
		copy := pvElectricalDraftHand(t)
		copy.Objects[0].Fields[0].Value = version
		got := energyPathBuildPVElectricalInventory(copy)
		if got.SchemaReviewed || len(got.Targets) != 20 || len(energyPathPVElectricalIntents(got)) != 0 {
			t.Fatalf("unreviewed schema gained native authority: %q", version)
		}
	}
}

func TestPVElectricalDraftSignedObservationsAndNullAreNotRepaired(t *testing.T) {
	roles := map[string]int{}
	for _, definition := range energyPathPVElectricalDefinitions() {
		roles[definition.Role]++
		zero, positive, negative := 0.0, 3.6e6, -3.6e6
		if definition.acceptsNativeEnergy(nil) || !definition.acceptsNativeEnergy(&zero) {
			t.Fatalf("missing became0 or native0 became missing: %s", definition.ID)
		}
		for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			if definition.acceptsNativeEnergy(&value) {
				t.Fatal("nonfinite native value accepted")
			}
		}
		switch definition.SignPolicy {
		case "nonpositive":
			if !definition.acceptsNativeEnergy(&negative) || definition.acceptsNativeEnergy(&positive) || definition.ParentMeter != "ElectricityProduced:Facility" {
				t.Fatalf("production decrement was clamped or became consumption: %+v", definition)
			}
		case "nonnegative":
			if !definition.acceptsNativeEnergy(&positive) || definition.acceptsNativeEnergy(&negative) {
				t.Fatal("nonnegative purchased/gross source boundary lost")
			}
		case "signed":
			if !definition.acceptsNativeEnergy(&negative) || !definition.acceptsNativeEnergy(&positive) {
				t.Fatal("net/thermal context signed native value was erased")
			}
		}
		if positive != 3.6e6 || negative != -3.6e6 || zero != 0 {
			t.Fatal("metadata policy changed observation")
		}
		if definition.Role == "purchased_constituent" && (definition.Name != "Inverter Ancillary AC Electricity Energy" || definition.ParentMeter != "Electricity:Facility" || definition.NativeEndUse != "Cogeneration") {
			t.Fatalf("ancillary accounting role erased: %+v", definition)
		}
	}
	if roles["produced_decrement"] != 2 || roles["purchased_constituent"] != 1 || roles["thermal_context"] != 2 {
		t.Fatalf("exact native sign/role roster: %v", roles)
	}
}

func TestPVElectricalDraftRequestsHonorOriginalAndPendingWildcards(t *testing.T) {
	for _, scope := range []SimulationPurposeScope{{ZoneMode: "all"}, {ZoneMode: "selected", ZoneNames: []string{"unrelated"}}} {
		doc := pvElectricalDraftHand(t)
		pvElectricalDraftAppend(&doc, "Output:Variable", "*", "Generator Produced DC Electricity Energy", "Monthly")
		pvElectricalDraftAppend(&doc, "Output:Variable", "", "Generator Produced DC Electricity Energy", "") // Native default key*/Hourly.
		pvElectricalDraftAppend(&doc, "Output:Variable", "Battery", "Electric Storage Charge Energy", "Timestep")
		pvElectricalDraftAppend(&doc, "Output:Variable", "*", "Electric Storage Charge Energy", "Monthly", "LimitedSchedule")
		pvElectricalDraftAppend(&doc, "Output:Meter", "Electricity:*", "Monthly")
		before := doc.String()
		builder := newPurposePlanBuilder(doc, NormalizeSimulationPurposeRequest(&SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: scope}))
		builder.addVariableWithReason(SimulationPurposeBasicEnergy, "*", "Electric Storage Discharge Energy", "Monthly", "medium", "Existing staged wildcard", "Basic Energy Path")
		builder.addEnergyPathPVElectricalOutputs()
		count := len(builder.objects)
		builder.addEnergyPathPVElectricalOutputs()
		if len(builder.objects) != count {
			t.Fatal("second helper call appended duplicates")
		}
		intents := energyPathPVElectricalIntents(energyPathBuildPVElectricalInventory(doc))
		for _, intent := range intents {
			covered := false
			for _, request := range builder.objects {
				covered = covered || energyPathPVElectricalRequestCovers(request, intent)
			}
			if !covered {
				t.Fatalf("missing exact intent: %+v", intent)
			}
		}
		for _, request := range builder.objects {
			if request.ObjectType == "Output:Variable" && request.VariableName == "Generator Produced DC Electricity Energy" && request.KeyValue != "*" && request.KeyValue != "" {
				t.Fatalf("five keyed requests duplicated existing wildcard: %+v", request)
			}
			if request.VariableName == "Electric Storage Charge Energy" && request.ReportingFrequency == "Monthly" && (request.KeyValue != "Battery" || purposeFieldValue(request.Fields, "Schedule Name") != "") {
				t.Fatal("scheduled output replaced independent unscheduled native observation")
			}
		}
		plan := builder.plan()
		apply := PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeAddMissingOnly)
		if len(apply.Updates) != 0 || len(apply.RemoveObjectIndexes) != 0 {
			t.Fatal("native request rewrites original")
		}
		applied, preview := idf.ApplyOutput(doc, apply)
		if !preview.CanApply || len(applied.Objects) < len(doc.Objects) {
			t.Fatalf("add-only preview failed: %+v", preview)
		}
		for i, original := range doc.Objects {
			if !reflect.DeepEqual(original, applied.Objects[i]) {
				t.Fatalf("original%d fields/index/comments changed", i)
			}
		}
		if before != doc.String() {
			t.Fatal("planning changed input")
		}
	}
}

func TestPVElectricalDraftCoverageDoesNotBorrowFrequencyOrCumulative(t *testing.T) {
	inventory := energyPathBuildPVElectricalInventory(pvElectricalDraftHand(t))
	for _, intent := range energyPathPVElectricalIntents(inventory) {
		request := intent.request()
		if !energyPathPVElectricalRequestCovers(request, intent) {
			t.Fatal("literal self request not covered")
		}
		wrong := request
		wrong.Fields = append([]idf.OutputFieldValue(nil), request.Fields...)
		for i := range wrong.Fields {
			if wrong.Fields[i].Name == "Reporting Frequency" {
				wrong.Fields[i].Value = "Timestep"
			}
		}
		if energyPathPVElectricalRequestCovers(wrong, intent) {
			t.Fatal("old Zone Timestep relabeled Monthly/Hourly")
		}
		if intent.Target.Definition.IsMeter {
			wrong = request
			wrong.ObjectType = "Output:Meter:Cumulative"
			if energyPathPVElectricalRequestCovers(wrong, intent) {
				t.Fatal("cumulative meter replaced interval source")
			}
		} else {
			wrong = request
			wrong.Fields = append([]idf.OutputFieldValue(nil), request.Fields...)
			wrong.Fields[0].Value = "Foreign"
			if energyPathPVElectricalRequestCovers(wrong, intent) {
				t.Fatal("another owner request covered this key")
			}
		}
		if strings.Contains(intent.Target.Definition.Name, "Rate") || strings.Contains(intent.Target.Definition.Name, "Power") {
			t.Fatal("invented Rate alias expanded twenty Energy identities")
		}
	}
}

// This deliberately uses the public normal pipeline, not just the new helper.
// Installation must include its scoped hook INSIDE addEnergyPathHourlyOutputs
// as well as the later explicit PV-intent call in BuildPurposeRunPlan.
func TestPVElectricalDraftNormalPlanReusesOriginalHourlyStorageWildcard(t *testing.T) {
	for _, form := range []struct{ name, key, frequency string }{
		{"blank_key_hourly", "", "Hourly"}, {"native_defaults", "", ""}, {"explicit_star", "*", "Hourly"},
	} {
		t.Run(form.name, func(t *testing.T) {
			for _, scope := range []SimulationPurposeScope{{ZoneMode: "all"}, {ZoneMode: "selected", ZoneNames: []string{"unrelated"}}} {
				doc := pvElectricalDraftHand(t)
				originalIndices := map[string]int{}
				for _, name := range []string{"Electric Storage Charge Energy", "Electric Storage Discharge Energy"} {
					pvElectricalDraftAppend(&doc, "Output:Variable", form.key, name, form.frequency)
					originalIndices[name] = doc.Objects[len(doc.Objects)-1].Index
				}
				before := doc.String()
				request := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: scope}
				plan := BuildPurposeRunPlan(doc, request)
				for name, index := range originalIndices {
					monthly, hourly := 0, 0
					for _, object := range plan.OutputObjects {
						if object.ObjectType != "Output:Variable" || object.VariableName != name {
							continue
						}
						switch object.ReportingFrequency {
						case "Monthly":
							monthly++
							if object.KeyValue != "*" {
								t.Fatal("standard Monthly reporting scope was narrowed")
							}
						case "Hourly":
							hourly++
							// Existing planner metadata canonicalizes a blank frequency
							// to Hourly; add-only application below preserves raw IDF.
							if object.KeyValue != form.key || object.ObjectIndex == nil || *object.ObjectIndex != index || object.State != PurposeOutputStateExisting || purposeFieldValue(object.Fields, "Reporting Frequency") != canonicalPurposeFrequency(form.frequency) {
								t.Fatalf("native original Hourly key/opener not reused: %+v", object)
							}
						default:
							t.Fatalf("unexpected storage request frequency: %+v", object)
						}
					}
					if monthly != 1 || hourly != 1 {
						t.Fatalf("normal Basic Energy/automatic Hourly duplicated %s: Monthly=%d Hourly=%d", name, monthly, hourly)
					}
				}
				for _, intent := range energyPathPVElectricalIntents(energyPathBuildPVElectricalInventory(doc)) {
					covered := false
					for _, object := range plan.OutputObjects {
						covered = covered || energyPathPVElectricalRequestCovers(object, intent)
					}
					if !covered {
						t.Fatalf("full normal plan lost native intent: %+v", intent)
					}
				}
				apply := PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeAddMissingOnly)
				if len(apply.Updates) != 0 || len(apply.RemoveObjectIndexes) != 0 {
					t.Fatal("coverage reuse rewrites original declarations")
				}
				applied, preview := idf.ApplyOutput(doc, apply)
				if !preview.CanApply {
					t.Fatalf("normal add-only plan cannot apply: %+v", preview)
				}
				for i, original := range doc.Objects {
					if !reflect.DeepEqual(original, applied.Objects[i]) {
						t.Fatalf("original object %d changed", i)
					}
				}
				if doc.String() != before {
					t.Fatal("normal planning modified original")
				}
			}
		})
	}
}

func TestPVElectricalDraftHourlyCoverageCannotNarrowNativeReportingScope(t *testing.T) {
	for _, trial := range []struct {
		name, variable, monthlyKey, originalKey, frequency, schedule, version, peerType, peerName string
		want                                                                                      bool
	}{
		{name: "mixed_unreviewed_storage_key_subset", variable: "Electric Storage Charge Energy", monthlyKey: "*", originalKey: "Battery", frequency: "Hourly", peerType: "ElectricLoadCenter:Storage:Simple", peerName: "OtherStorage"},
		{name: "multiple_reviewed_storage_key_subset", variable: "Electric Storage Charge Energy", monthlyKey: "*", originalKey: "Battery", frequency: "Hourly", peerType: "ElectricLoadCenter:Storage:Battery", peerName: "OtherBattery"},
		{name: "multiple_pv_key_subset", variable: "Generator Produced DC Electricity Energy", monthlyKey: "*", originalKey: "PV1", frequency: "Hourly"},
		{name: "even_single_key_does_not_prove_star_scope", variable: "Electric Storage Charge Energy", monthlyKey: "*", originalKey: "Battery", frequency: "Hourly"},
		{name: "scheduled_blank_key", variable: "Electric Storage Charge Energy", monthlyKey: "*", originalKey: "", frequency: "Hourly", schedule: "LimitedSchedule"},
		{name: "wrong_frequency", variable: "Electric Storage Charge Energy", monthlyKey: "*", originalKey: "", frequency: "Timestep"},
		{name: "unreviewed_version", variable: "Electric Storage Charge Energy", monthlyKey: "*", originalKey: "", frequency: "Hourly", version: "22.1"},
		{name: "foreign_variable", variable: "Fan Electricity Energy", monthlyKey: "*", originalKey: "", frequency: "Hourly"},
		{name: "exact_key_is_exact_scope", variable: "Electric Storage Charge Energy", monthlyKey: "Battery", originalKey: "Battery", frequency: "Hourly", peerType: "ElectricLoadCenter:Storage:Simple", peerName: "OtherStorage", want: true},
		{name: "foreign_key_not_same_scope", variable: "Electric Storage Charge Energy", monthlyKey: "Battery", originalKey: "OtherBattery", frequency: "Hourly", peerType: "ElectricLoadCenter:Storage:Battery", peerName: "OtherBattery"},
		{name: "blank_star_covers_mixed_native_scope", variable: "Electric Storage Charge Energy", monthlyKey: "*", originalKey: "", frequency: "Hourly", peerType: "ElectricLoadCenter:Storage:Simple", peerName: "OtherStorage", want: true},
		{name: "literal_star_covers_all_pv", variable: "Generator Produced DC Electricity Energy", monthlyKey: "*", originalKey: "*", frequency: "Hourly", want: true},
	} {
		t.Run(trial.name, func(t *testing.T) {
			doc := pvElectricalDraftHand(t)
			if trial.version != "" {
				doc.Objects[0].Fields[0].Value = trial.version
			}
			if trial.peerType != "" {
				pvElectricalDraftAppend(&doc, trial.peerType, trial.peerName)
			}
			pvElectricalDraftAppend(&doc, "Output:Variable", trial.originalKey, trial.variable, trial.frequency, trial.schedule)
			builder := newPurposePlanBuilder(doc, NormalizeSimulationPurposeRequest(&SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}))
			builder.addVariableWithReason(SimulationPurposeBasicEnergy, trial.monthlyKey, trial.variable, "Monthly", "medium", "hand Monthly reporting scope", "Basic Energy Path")
			monthly := builder.objects[0]
			before := len(builder.objects)
			if got := builder.reuseEnergyPathPVElectricalHourlyCoverage(monthly); got != trial.want {
				t.Fatalf("coverage=%v want %v; Monthly=%+v", got, trial.want, monthly)
			}
			if !trial.want && len(builder.objects) != before {
				t.Fatal("rejected coverage modified pending plan")
			}
			if trial.want {
				if len(builder.objects) != before+1 || builder.objects[before].ObjectIndex == nil || builder.objects[before].State != PurposeOutputStateExisting {
					t.Fatal("accepted coverage did not bind existing declaration")
				}
				return
			}
			// Original key subsets are not in builder.objects. The normal automatic
			// pass must still generate the full unscheduled Hourly '*' request.
			if trial.monthlyKey == "*" {
				builder.addEnergyPathHourlyOutputs()
				found := false
				for _, object := range builder.objects {
					if object.VariableName == trial.variable && object.KeyValue == "*" && object.ReportingFrequency == "Hourly" && purposeFieldValue(object.Fields, "Schedule Name") == "" {
						found = true
					}
				}
				if !found {
					t.Fatal("a key subset/foreign proof suppressed automatic whole-scope Hourly observation")
				}
			}
		})
	}
}

func TestPVElectricalDraftHourlyMeterCoverageKeepsNativeMeterScope(t *testing.T) {
	for _, trial := range []struct {
		kind, key, frequency string
		want                 bool
	}{
		{"Output:Meter", "Electricity:*", "Hourly", true},
		{"Output:Meter", "Electricity:Facility", "Hourly", true},
		{"Output:Meter", "Electricity:*", "Monthly", false},
		{"Output:Meter", "ElectricityProduced:Facility", "Hourly", false},
		{"Output:Meter:Cumulative", "Electricity:Facility", "Hourly", false},
	} {
		doc := pvElectricalDraftHand(t)
		pvElectricalDraftAppend(&doc, trial.kind, trial.key, trial.frequency)
		builder := newPurposePlanBuilder(doc, NormalizeSimulationPurposeRequest(&SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}))
		monthly := PurposeOutputObject{ObjectType: "Output:Meter", Fields: []idf.OutputFieldValue{{Name: "Key Name", Value: "Electricity:Facility"}, {Name: "Reporting Frequency", Value: "Monthly"}}, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Reason: "Basic Energy Path"}
		if got := builder.reuseEnergyPathPVElectricalHourlyCoverage(monthly); got != trial.want {
			t.Fatalf("native meter coverage for %+v = %v", trial, got)
		}
		if trial.want && (len(builder.objects) != 1 || builder.objects[0].KeyValue != trial.key || builder.objects[0].ObjectIndex == nil) {
			t.Fatal("meter wildcard declaration was rewritten or widened")
		}
	}
}
