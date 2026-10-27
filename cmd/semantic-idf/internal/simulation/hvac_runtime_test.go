package simulation

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestHVACRuntimeExecutedProfilesAndCoverage(t *testing.T) {
	doc := parsePurposePlanFixture(t, hvacTwoLoopInput+"\nSchedule:Constant,Target,,14;\nSetpointManager:Scheduled,SAT,Temperature,Target,Fan Outlet;\n")
	report := idf.AnalyzeHVAC(doc)
	before, _ := json.Marshal(report)
	request := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}}
	plan := BuildPurposeRunPlan(doc, request)
	var series []SimulationSeries
	nonMeter := false
	for _, output := range plan.OutputObjects {
		if output.ObjectType == "Output:Variable" && (purposeTimestepFrequency(output.ReportingFrequency) || purposeDetailedFrequency(output.ReportingFrequency)) {
			item := SimulationSeries{File: "native.sql", SourceID: "native:" + output.Signature, Name: output.VariableName, KeyValue: output.KeyValue, ReportingFrequency: output.ReportingFrequency, IsMeter: &nonMeter, Points: []SimulationPoint{{X: 7, Label: "01-01 01:10", Value: 0}, {X: 9, Label: "01-01 01:30", Value: 0.5}}, NativeIntervals: []SimulationNativeInterval{{TimeIndex: 70, EnvironmentPeriodIndex: 2, IntervalMinutes: 10, SimulationDays: 1}, {TimeIndex: 90, EnvironmentPeriodIndex: 2, IntervalMinutes: 20, SimulationDays: 1}}}
			if purposeDetailedFrequency(output.ReportingFrequency) {
				item.Points = []SimulationPoint{{X: 7, Label: "01-01 01:10", Value: 0}, {X: 8, Label: "01-01 01:20", Value: 2}, {X: 9, Label: "01-01 01:30", Value: 2}}
				item.NativeIntervals = []SimulationNativeInterval{{TimeIndex: 70, EnvironmentPeriodIndex: 2, IntervalMinutes: 10, SimulationDays: 1}, {TimeIndex: 80, EnvironmentPeriodIndex: 2, IntervalMinutes: 10, SimulationDays: 1}, {TimeIndex: 90, EnvironmentPeriodIndex: 2, IntervalMinutes: 10, SimulationDays: 1}}
			}
			series = append(series, item)
		}
	}
	if len(series) == 0 {
		t.Fatal("no source-specific runtime outputs planned")
	}
	for _, loop := range report.Loops {
		profiles := hvacRuntimeProfiles(report, loop, request.Scope)
		result := HVACLoopRunResult{}
		attachHVACRuntimeControls(&result, profiles, loop, &doc, series)
		if len(profiles) == 0 || result.ControlCoverage == nil || result.ControlCoverage.Status != "complete" || result.ControlCoverage.Missing != 0 {
			t.Fatalf("complete native observations must bind: %+v", result.ControlCoverage)
		}
		pooled := map[string]bool{}
		for _, item := range result.ControlSeries {
			if pooled[item.SourceID] {
				t.Fatal("runtime observations duplicated per AST node")
			}
			pooled[item.SourceID] = true
			if item.Points[0].Value != 0 || item.Points[len(item.Points)-1].X != 9 {
				t.Fatal("zero or missing-frame coordinates changed")
			}
		}
		owners := map[string]bool{}
		for _, item := range result.ControlSeries {
			owners[item.SourceID] = len(item.NativeIntervals) == len(item.Points)
		}
		for _, item := range result.ControlSeries {
			if item.NativeIntervalsSourceID != "" && !owners[item.NativeIntervalsSourceID] {
				t.Fatal("a loop-local interval reference escaped its delivered control pool")
			}
		}
		for _, signal := range result.ControlSignals {
			if signal.Status != "collected" || !pooled[signal.SeriesID] || signal.ComponentIndex < 0 || signal.Source.ObjectType == "" {
				t.Fatalf("source binding lost: %+v", signal)
			}
		}
		missing := HVACLoopRunResult{}
		attachHVACRuntimeControls(&missing, profiles, loop, &doc, nil)
		if missing.ControlCoverage.Status != "error" || missing.ControlCoverage.Missing != missing.ControlCoverage.Required {
			t.Fatal("missing data must be an acquisition error, not OFF or silently optional")
		}
		duplicate := append(append([]SimulationSeries{}, series...), series[0])
		ambiguous := HVACLoopRunResult{}
		attachHVACRuntimeControls(&ambiguous, profiles, loop, &doc, duplicate)
		for _, signal := range ambiguous.ControlSignals {
			if signal.KeyValue == series[0].KeyValue && signal.VariableName == series[0].Name && signal.Status != "ambiguous" {
				t.Fatal("ambiguous native observations were arbitrarily selected")
			}
		}
	}
	for _, regression := range []struct {
		name string
		edit func(*SimulationSeries)
	}{
		{"Detailed duration missing", func(s *SimulationSeries) { s.NativeIntervals = nil }},
		{"Detailed duration nonpositive", func(s *SimulationSeries) { s.NativeIntervals[1].IntervalMinutes = 0 }},
		{"Detailed duplicate native row", func(s *SimulationSeries) { s.NativeIntervals[1].TimeIndex = s.NativeIntervals[0].TimeIndex }},
		{"Detailed native overlap", func(s *SimulationSeries) { s.NativeIntervals[1].IntervalMinutes = 5 }},
		{"Detailed different environment", func(s *SimulationSeries) {
			for i := range s.NativeIntervals {
				s.NativeIntervals[i].EnvironmentPeriodIndex = 3
			}
		}},
		{"Detailed first substep missing", func(s *SimulationSeries) { s.Points = s.Points[1:]; s.NativeIntervals = s.NativeIntervals[1:] }},
		{"Detailed extra initial substep", func(s *SimulationSeries) { s.NativeIntervals[0].IntervalMinutes = 20 }},
		{"averaged native action", func(s *SimulationSeries) { s.Points[1].Value = .5 }},
		{"wrong reporting frequency", func(s *SimulationSeries) { s.ReportingFrequency = "Timestep" }},
	} {
		t.Run(regression.name, func(t *testing.T) {
			changed := append([]SimulationSeries{}, series...)
			for i := range changed {
				if purposeDetailedFrequency(changed[i].ReportingFrequency) {
					changed[i].Points = append([]SimulationPoint{}, changed[i].Points...)
					changed[i].NativeIntervals = append([]SimulationNativeInterval{}, changed[i].NativeIntervals...)
					// Change every Detailed channel together: same-cadence axis
					// union alone cannot detect a shared missing first substep.
					regression.edit(&changed[i])
				}
			}
			for _, loop := range report.Loops {
				result := HVACLoopRunResult{}
				attachHVACRuntimeControls(&result, hvacRuntimeProfiles(report, loop, request.Scope), loop, &doc, changed)
				if result.ControlCoverage.Status != "error" || result.ControlCoverage.Missing == 0 {
					t.Fatal("invalid Detailed native acquisition reported complete", result.ControlCoverage)
				}
			}
		})
	}
	after, _ := json.Marshal(report)
	if string(before) != string(after) {
		t.Fatal("runtime binding mutated the static source graph")
	}
	selected := SimulationPurposeRequest{Purposes: request.Purposes, Scope: SimulationPurposeScope{ComponentIDs: []string{"Fan:ConstantVolume:Second Fan"}}}
	selectedPlan := BuildPurposeRunPlan(doc, selected)
	for _, output := range selectedPlan.OutputObjects {
		if (purposeTimestepFrequency(output.ReportingFrequency) || purposeDetailedFrequency(output.ReportingFrequency)) && strings.Contains(output.KeyValue, "First") {
			t.Fatalf("runtime plan broadened selected equipment scope: %+v", output)
		}
	}
	inputPath := filepath.Join(t.TempDir(), "executed.idf")
	if err := os.WriteFile(inputPath, []byte(doc.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	hourly := []SimulationSeries{hvacResultTestSeries("First Inlet", "System Node Temperature", "C"), hvacResultTestSeries("First Fan", "Fan Electricity Rate", "W")}
	run := SimulationRunResult{Status: "succeeded", InputPath: inputPath, Series: hourly}
	incomplete := BuildPurposeResultBundle(&run, request)
	controlFound := false
	for _, item := range incomplete.Completeness {
		if item.RequiredOutput == "HVAC control diagnostics" {
			controlFound = true
			if item.Found || item.Status != "missing" || !strings.Contains(item.Message, "First Loop") || !strings.Contains(item.Message, "required HVAC control observations") {
				t.Fatalf("required acquisition failure disappeared from bundle completeness: %+v", item)
			}
		} else if item.PurposeID == SimulationPurposeHVACLoopCheck && !item.Found {
			t.Fatalf("control acquisition failure invalidated present Hourly observations: %+v", item)
		}
	}
	if !controlFound || run.Status != "succeeded" {
		t.Fatal("control coverage must affect purpose completeness without changing successful engine status")
	}
	for _, loop := range incomplete.HVACLoops {
		found := false
		for _, item := range loop.Completeness {
			if item.RequiredOutput == "HVAC control diagnostics" {
				found = true
				if item.Found || !strings.Contains(item.Message, loop.Name) {
					t.Fatalf("per-loop acquisition failure lost its concrete coverage: %+v", item)
				}
			}
		}
		if !found {
			t.Fatal("executed runtime profile lacks per-loop control completeness")
		}
	}
	run.Series = append(append([]SimulationSeries{}, hourly...), series...)
	complete := BuildPurposeResultBundle(&run, request)
	for _, item := range complete.Completeness {
		if item.RequiredOutput == "HVAC control diagnostics" && !item.Found {
			t.Fatalf("complete native controls remained incomplete: %+v", item)
		}
	}
	for _, regression := range []struct {
		name string
		pick func(SimulationSeries) bool
		edit func([]SimulationPoint) []SimulationPoint
	}{
		{"required interval gap", func(s SimulationSeries) bool { return strings.Contains(s.Name, "Setpoint Temperature") }, func(p []SimulationPoint) []SimulationPoint { return p[:1] }},
		{"unset native target", func(s SimulationSeries) bool { return strings.Contains(s.Name, "Setpoint Temperature") }, func(p []SimulationPoint) []SimulationPoint { p[1].Value = -999; return p }},
		{"invalid manager status", func(s SimulationSeries) bool { return strings.Contains(s.Name, "Cycle On Off Status") }, func(p []SimulationPoint) []SimulationPoint { p[1].Value = 4; return p }},
		{"averaged manager status", func(s SimulationSeries) bool { return strings.Contains(s.Name, "Cycle On Off Status") }, func(p []SimulationPoint) []SimulationPoint { p[1].Value = .5; return p }},
		{"nonfinite required observation", func(s SimulationSeries) bool { return strings.Contains(s.Name, "Setpoint Temperature") }, func(p []SimulationPoint) []SimulationPoint { p[1].Value = math.NaN(); return p }},
	} {
		t.Run(regression.name, func(t *testing.T) {
			changed := append([]SimulationSeries{}, series...)
			picked := false
			for i := range changed {
				if regression.pick(changed[i]) {
					changed[i].Points = regression.edit(append([]SimulationPoint{}, changed[i].Points...))
					picked = true
					break
				}
			}
			if !picked {
				t.Fatal("fixture has no required native channel for regression")
			}
			run.Series = append(append([]SimulationSeries{}, hourly...), changed...)
			broken := BuildPurposeResultBundle(&run, request)
			failed := false
			for _, item := range broken.Completeness {
				failed = failed || item.RequiredOutput == "HVAC control diagnostics" && !item.Found
			}
			if !failed || broken.HVACLoops[0].ControlCoverage.Status != "error" {
				t.Fatalf("required interval failure reported successful collection: %+v", broken.Completeness)
			}
			for _, signal := range broken.HVACLoops[0].ControlSignals {
				if signal.Message != "" && signal.Status == "collected" {
					// The valid interval and raw native series must remain usable;
					// an incomplete channel is not globally rewritten as OFF/missing.
					return
				}
			}
			t.Fatal("interval validation lost the collected channel or its coverage explanation")
		})
	}
	run.InputPath = ""
	legacy := BuildPurposeResultBundle(&run, request)
	for _, item := range legacy.Completeness {
		if item.RequiredOutput == "HVAC control diagnostics" {
			t.Fatal("legacy observations without executed profiles acquired a new control completeness marker")
		}
	}
}

func TestHVACRuntimeManagerDomainsAndFrequencyPreservation(t *testing.T) {
	profile := idf.HVACOperatingProfile{ComponentIndex: 8, AvailabilityDefinitions: idf.HVACControlSequence{Nodes: []idf.HVACControlNode{{ID: "m", Code: "availability_manager", Source: idf.HVACOperatingSource{ObjectIndex: 2, ObjectType: "AvailabilityManager:HybridVentilation", ObjectName: "Hybrid"}, Parameters: []idf.HVACControlParameter{{Name: "HVAC Air Loop Name", Value: "Actual Air"}}}}}}
	signals := hvacRuntimeSignals(profile, idf.HVACLoop{}, nil)
	if len(signals) != 1 || signals[0].Domain != "hybrid_ventilation_status" || signals[0].KeyValue != "Actual Air" || signals[0].ReportingFrequency != "Detailed" {
		t.Fatalf("hybrid status has its own native domain and key: %+v", signals)
	}
	doc := parsePurposePlanFixture(t, hvacTwoLoopInput+"\nOutput:Variable,Second Fan,Fan Electricity Rate,Hourly;\n")
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}})
	found := false
	for _, output := range plan.OutputObjects {
		if output.KeyValue == "Second Fan" && output.VariableName == "Fan Electricity Rate" && output.ReportingFrequency == "Timestep" {
			found = true
			if output.State == PurposeOutputStateConflict || output.ReportingFrequency != "Timestep" {
				t.Fatal("required diagnostic cadence must be added without replacing existing Hourly output")
			}
		}
	}
	if !found {
		t.Fatal("required Timestep channel was omitted")
	}
	for _, output := range plan.OutputObjects {
		if hvacRuntimeStatusVariable(output.VariableName) && output.ReportingFrequency != "Detailed" {
			t.Fatalf("discrete availability actions must not use an averaging cadence: %+v", output)
		}
	}
	for _, frequency := range []string{"Hourly", "Timestep"} {
		for _, key := range []string{"First Loop", "*"} {
			for _, policy := range []string{PurposeFrequencyPolicyDefault, PurposeFrequencyPolicyPreserve, PurposeFrequencyPolicyHighestResolution} {
				t.Run(key+"/"+frequency+"/"+policy, func(t *testing.T) {
					const variable = "Air System Simulation Cycle On Off Status"
					input := hvacTwoLoopInput + "\nOutput:Variable," + key + "," + variable + "," + frequency + ";\n"
					if key == "*" {
						input += "HVACTemplate:System:VAV,Expanded System;\n"
					}
					doc := parsePurposePlanFixture(t, input)
					before := doc.String()
					plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}, FrequencyPolicy: policy})
					detailed, warning := false, false
					for _, output := range plan.OutputObjects {
						if output.KeyValue == key && output.VariableName == variable && output.ReportingFrequency == "Detailed" {
							detailed = output.State == PurposeOutputStateTemporary && output.ObjectIndex == nil && output.Reason == "HVAC control diagnostics"
							for _, item := range plan.Warnings {
								warning = warning || item.OutputSignature == output.Signature && item.Code == "hvac_control_detailed_added" && strings.Contains(item.Message, "required Detailed") && strings.Contains(item.Message, frequency+" frequency")
							}
						}
					}
					if !detailed || !warning {
						t.Fatalf("existing status request displaced required Detailed diagnostics: detailed=%v warning=%v", detailed, warning)
					}
					apply := PurposeRunPlanApplyRequest(plan)
					if len(apply.Updates) != 0 || len(apply.RemoveObjectIndexes) != 0 {
						t.Fatal("adding diagnostic cadence must not edit existing output definitions", apply)
					}
					updated, preview := idf.ApplyOutput(doc, apply)
					if !preview.CanApply || doc.String() != before {
						t.Fatal("applying the plan changed its shared source document", preview)
					}
					original, added := 0, 0
					for _, object := range updated.Objects {
						if strings.EqualFold(object.Type, "Output:Variable") && len(object.Fields) >= 3 && object.Fields[0].Value == key && object.Fields[1].Value == variable {
							if object.Fields[2].Value == frequency {
								original++
							}
							if object.Fields[2].Value == "Detailed" {
								added++
							}
						}
					}
					if original != 1 || added != 1 {
						t.Fatalf("existing frequency and exact Detailed companion must coexist once: original=%d added=%d", original, added)
					}
				})
			}
		}
	}
	profile.ParameterControl.Nodes = []idf.HVACControlNode{{ID: "target", Code: "node_setpoint", Source: idf.HVACOperatingSource{ObjectType: "SetpointManager:Scheduled", ObjectName: "Provider"}, Parameters: []idf.HVACControlParameter{{Name: "Setpoint Node or NodeList Name", Value: "Resolved Node"}, {Name: "Control Variable", Value: "Temperature"}}}}
	setpoints := hvacRuntimeSignals(profile, idf.HVACLoop{}, nil)
	found = false
	for _, signal := range setpoints {
		found = found || signal.Role == "setpoint" && signal.KeyValue == "Resolved Node" && signal.VariableName == "System Node Setpoint Temperature"
	}
	if !found {
		t.Fatal("resolved node-list provider target was omitted")
	}
	for _, test := range []struct{ temperatureType, variable string }{
		{"NodeDryBulb", "System Node Temperature"},
		{"NodeWetBulb", "System Node Wetbulb Temperature"},
		{"", "System Node Temperature"},
		{"Unrecognized", ""},
	} {
		followDoc := parsePurposePlanFixture(t, "Version,24.2;\nSchedule:Constant,On,,1;\nFan:ConstantVolume,Local Fan,On,0.7,600,Autosize,0.9,1,Local Inlet,Local Outlet;\nSetpointManager:FollowSystemNodeTemperature,Follow,Temperature,Reference Node,"+test.temperatureType+",0,30,5,Local Outlet;\n")
		report := idf.AnalyzeHVAC(followDoc)
		found = false
		for _, p := range report.OperatingProfiles {
			before, _ := json.Marshal(p.ParameterControl)
			for _, signal := range hvacRuntimeSignals(p, idf.HVACLoop{}, &followDoc) {
				if signal.KeyValue != "Reference Node" {
					continue
				}
				found = true
				if signal.Role != "sensor" || signal.VariableName != test.variable || signal.ParameterName != "Reference Node Name" {
					t.Fatalf("FollowSystemNodeTemperature reference is a native measured input, not a setpoint: %+v", signal)
				}
				if test.variable == "" && signal.Status != "unsupported" {
					t.Fatal("unknown reference temperature type must fail required acquisition rather than guess")
				}
			}
			after, _ := json.Marshal(p.ParameterControl)
			if string(before) != string(after) {
				t.Fatal("concrete runtime binding changed original control definitions")
			}
		}
		if !found {
			t.Fatal("native FollowSystemNodeTemperature reference input was omitted")
		}
		followPlan := BuildPurposeRunPlan(followDoc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}})
		for _, output := range followPlan.OutputObjects {
			if output.KeyValue == "Reference Node" && output.ReportingFrequency == "Timestep" && strings.Contains(output.VariableName, "Setpoint") {
				t.Fatalf("reference measurement requested a spurious native target: %+v", output)
			}
		}
	}
}

func TestHVACRuntimeLocalAndExpandedPlanning(t *testing.T) {
	doc := parsePurposePlanFixture(t, `Version,24.2;
Schedule:Constant,On,,1;
Fan:ConstantVolume,Local Fan,On,0.7,600,Autosize,0.9,1,Local Inlet,Local Outlet;
`)
	request := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}}
	loops := buildHVACLoopRunResultsWithDocument([]SimulationSeries{hvacResultTestSeries("Local Inlet", "System Node Temperature", "C"), hvacResultTestSeries("Local Fan", "Fan Electricity Rate", "W"), hvacResultTestSeries("Foreign", "System Node Temperature", "C")}, request, &doc)
	if len(loops) != 1 || loops[0].LoopType != "ZoneHVAC" || loops[0].Topology != nil || len(loops[0].OperatingProfiles) != 1 || len(loops[0].Series) != 1 || len(loops[0].Components) != 1 {
		t.Fatalf("local control grouping must preserve exact legacy observations without invented topology: %+v", loops)
	}
	plan := BuildPurposeRunPlan(parsePurposePlanFixture(t, "Version,24.2;\nHVACTemplate:System:VAV,Generated;"), request)
	found := false
	for _, output := range plan.OutputObjects {
		found = found || output.KeyValue == "*" && output.VariableName == "Schedule Value" && output.ReportingFrequency == "Timestep"
	}
	if !found {
		t.Fatal("unknown expanded-source names need explicit runtime dictionary fallback")
	}
}

func TestHVACRuntimeManagerReasonInputs(t *testing.T) {
	doc := parsePurposePlanFixture(t, "Version,24.2;\nZone,Served;\nZone,Explicit;\nZone,Unrelated;\nZoneList,Control List,Explicit;\n")
	loop := idf.HVACLoop{Name: "Actual Loop", RelatedZones: []string{"Served"}}
	manager := idf.HVACControlNode{ID: "night", Code: "availability_manager", Source: idf.HVACOperatingSource{ObjectIndex: 50, ObjectType: "AvailabilityManager:NightCycle", ObjectName: "Night"}, Parameters: []idf.HVACControlParameter{{Name: "Control Type", Value: "CycleOnAny"}, {Name: "Control Zone or Zone List Name", Value: "Control List"}}}
	profile := idf.HVACOperatingProfile{AvailabilityDefinitions: idf.HVACControlSequence{Nodes: []idf.HVACControlNode{manager}}}
	signals := hvacRuntimeSignals(profile, loop, &doc)
	quantities := map[string]bool{}
	for _, signal := range signals {
		if signal.Role != "manager_status" {
			if signal.KeyValue != "Served" || signal.ParameterName != "Served zones" {
				t.Fatalf("CycleOnAny must use native served zones, not unused explicit fields: %+v", signal)
			}
			quantities[signal.VariableName] = true
		}
	}
	for _, variable := range []string{"Zone Thermostat Air Temperature", "Zone Thermostat Control Type", "Zone Thermostat Heating Setpoint Temperature", "Zone Thermostat Cooling Setpoint Temperature"} {
		if !quantities[variable] {
			t.Fatalf("NightCycle omitted its actual reason input %s", variable)
		}
	}
	manager.Parameters[0].Value = "CycleOnAnyHeatingZone"
	manager.Parameters = append(manager.Parameters, idf.HVACControlParameter{Name: "Heating Control Zone or Zone List Name", Value: "Control List"})
	profile.AvailabilityDefinitions.Nodes[0] = manager
	for _, signal := range hvacRuntimeSignals(profile, loop, &doc) {
		if signal.Role != "manager_status" && (signal.KeyValue != "Explicit" || strings.Contains(signal.VariableName, "Cooling Setpoint")) {
			t.Fatalf("Heating-only native mode collected an unrelated reason input: %+v", signal)
		}
	}
	for _, test := range []struct{ typ, parameter, value, variable string }{
		{"SetpointManager:OutdoorAirReset", "", "", "Site Outdoor Air Drybulb Temperature"},
		{"SetpointManager:FollowOutdoorAirTemperature", "Reference Temperature Type", "OutdoorAirDryBulb", "Site Outdoor Air Drybulb Temperature"},
		{"SetpointManager:FollowOutdoorAirTemperature", "Reference Temperature Type", "OutdoorAirWetBulb", "Site Outdoor Air Wetbulb Temperature"},
		{"SetpointManager:FollowGroundTemperature", "Reference Ground Temperature Object Type", "Site:GroundTemperature:Deep", "Site Deep Ground Temperature"},
	} {
		node := idf.HVACControlNode{Code: "setpoint_manager", Source: idf.HVACOperatingSource{ObjectType: test.typ}, Parameters: []idf.HVACControlParameter{{Name: test.parameter, Value: test.value}}}
		inputs := hvacRuntimeReasonInputs(node, loop, &doc)
		if len(inputs) != 1 || inputs[0].key != "Environment" || inputs[0].variable != test.variable {
			t.Fatalf("source-specific setpoint weather input: %+v", inputs)
		}
	}
	for _, test := range []struct{ typ, field, value, zone, demand string }{
		{"SetpointManager:SingleZone:Heating", "Control Zone Name", "Explicit", "Explicit", "Zone Predicted Sensible Load to Heating Setpoint Heat Transfer Rate"},
		{"SetpointManager:SingleZone:Cooling", "Control Zone Name", "Explicit", "Explicit", "Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate"},
		{"SetpointManager:Warmest", "HVAC Air Loop Name", "Actual Loop", "Served", "Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate"},
		{"SetpointManager:Coldest", "HVAC Air Loop Name", "Actual Loop", "Served", "Zone Predicted Sensible Load to Heating Setpoint Heat Transfer Rate"},
	} {
		node := idf.HVACControlNode{Code: "setpoint_manager", Source: idf.HVACOperatingSource{ObjectType: test.typ}, Parameters: []idf.HVACControlParameter{{Name: test.field, Value: test.value}}}
		found := false
		for _, input := range hvacRuntimeReasonInputs(node, loop, &doc) {
			if input.key != test.zone || strings.Contains(input.variable, "Cooling Setpoint") && strings.Contains(test.demand, "Heating Setpoint") || strings.Contains(input.variable, "Heating Setpoint") && strings.Contains(test.demand, "Cooling Setpoint") {
				t.Fatalf("setpoint strategy borrowed unrelated zone or mode: %+v", input)
			}
			found = found || input.variable == test.demand
		}
		if !found {
			t.Fatalf("%s omitted native mode's applicable demand", test.typ)
		}
		if test.field == "HVAC Air Loop Name" {
			node.Parameters[0].Value = "Other Loop"
			if inputs := hvacRuntimeReasonInputs(node, loop, &doc); len(inputs) != 0 {
				t.Fatalf("a shared occurrence borrowed another loop's zones: %+v", inputs)
			}
		}
	}
	for _, test := range []struct{ typ, variable string }{
		{"PlantEquipmentOperation:OutdoorRelativeHumidity", "Site Outdoor Air Relative Humidity"},
		{"PlantEquipmentOperation:OutdoorDewpoint", "Site Outdoor Air Dewpoint Temperature"},
		{"PlantEquipmentOperation:OutdoorDryBulbDifference", "Site Outdoor Air Drybulb Temperature"},
	} {
		node := idf.HVACControlNode{Code: "plant_operation_definition", Source: idf.HVACOperatingSource{ObjectType: test.typ}, Parameters: []idf.HVACControlParameter{{Name: "Reference Temperature Node Name", Value: "Reference Node"}}}
		inputs := hvacRuntimeReasonInputs(node, loop, &doc)
		if len(inputs) == 0 || inputs[0].key != "Environment" || inputs[0].variable != test.variable {
			t.Fatalf("plant dispatch omitted native environment context: %+v", inputs)
		}
		if strings.HasSuffix(test.typ, "Difference") && (len(inputs) != 2 || inputs[1].key != "Reference Node" || inputs[1].variable != "System Node Temperature") {
			t.Fatalf("differential outdoor dispatch omitted its declared reference node: %+v", inputs)
		}
	}
}

// Optional native acceptance: a fresh two-day owned copy, never an annual suite
// dependency. Set SEMANTIC_IDF_HVAC_RUNTIME_ENGINE to a matching 24.2 executable.
func TestHVACRuntimeNativeAcceptance(t *testing.T) {
	engine := os.Getenv("SEMANTIC_IDF_HVAC_RUNTIME_ENGINE")
	if engine == "" {
		t.Skip("set SEMANTIC_IDF_HVAC_RUNTIME_ENGINE for native two-day acceptance")
	}
	path := filepath.Join("..", "..", "frontend", "src", "samples", "RefBldgLargeOfficeNew2004_Chicago.idf")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := idf.Parse(string(original))
	if err != nil {
		t.Fatal(err)
	}
	for i := range doc.Objects {
		obj := &doc.Objects[i]
		if strings.EqualFold(obj.Type, "RunPeriod") && len(obj.Fields) > 6 {
			obj.Fields[1].Value = "7"
			obj.Fields[2].Value = "21"
			obj.Fields[3].Value = ""
			obj.Fields[4].Value = "7"
			obj.Fields[5].Value = "22"
			obj.Fields[6].Value = ""
		}
	}
	directory := os.Getenv("SEMANTIC_IDF_HVAC_RUNTIME_DIR")
	if directory == "" {
		directory = t.TempDir()
	}
	weather := os.Getenv("SEMANTIC_IDF_HVAC_RUNTIME_WEATHER")
	if weather == "" {
		weather = filepath.Join(filepath.Dir(engine), "WeatherData", "USA_IL_Chicago-OHare.Intl.AP.725300_TMY3.epw")
	}
	request := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}}
	prepared, err := prepareBatchPurposeSimulationRequest(SimulationRunRequest{RunID: "hvac-runtime-native", Text: doc.String(), Filename: "runtime-two-days.idf", EnergyPlusExecutablePath: engine, WeatherPath: weather, OutputDirectory: directory}, request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunSimulation(prepared, nil, DefaultSettings())
	if err != nil || result == nil || result.Status != "succeeded" {
		t.Fatalf("native run failed: %v %+v", err, result)
	}
	if result.PurposeResults == nil {
		t.Fatal("native run has no purpose bundle")
	}
	if capture := os.Getenv("SEMANTIC_IDF_HVAC_RUNTIME_EXPORT"); capture != "" {
		payload, err := json.Marshal(result.PurposeResults)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(capture, payload, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	missing := map[string]int{}
	families := map[string]bool{}
	meaningfulFlow, targetObserved := false, false
	for _, loop := range result.PurposeResults.HVACLoops {
		if loop.ControlCoverage == nil || loop.ControlCoverage.Status != "complete" {
			for _, signal := range loop.ControlSignals {
				if signal.Message != "" {
					t.Logf("%s: %s", signal.VariableName, signal.Message)
				}
			}
			t.Fatalf("native loop %s has incomplete exact interval coverage: %+v", loop.Name, loop.ControlCoverage)
		}
		bySeries := map[string]SimulationSeries{}
		for _, item := range loop.ControlSeries {
			bySeries[item.SourceID] = item
		}
		intervalAxes := map[string]bool{}
		for _, item := range loop.ControlSeries {
			owner := item
			if item.NativeIntervalsSourceID != "" {
				owner = bySeries[item.NativeIntervalsSourceID]
			}
			if len(owner.NativeIntervals) != len(item.Points) || !simulationPointAxisEqual(item.Points, owner.Points) {
				t.Fatalf("native metadata is missing or references another axis: %s", item.SourceID)
			}
			intervalAxes[owner.SourceID] = true
			if purposeDetailedFrequency(item.ReportingFrequency) {
				for _, point := range item.Points {
					if point.Value != math.Trunc(point.Value) {
						t.Fatalf("averaged native status: %+v", point)
					}
				}
			}
		}
		if len(intervalAxes) > 2 {
			t.Fatalf("shared native metadata expanded per control channel: %d axes", len(intervalAxes))
		}
		for _, profile := range loop.OperatingProfiles {
			source := hvacRuntimeComponent(profile)
			for _, family := range []string{"fan:", "coil:heating:water", "chiller:", "boiler:"} {
				if strings.HasPrefix(strings.ToLower(source.ObjectType), family) {
					families[family] = true
				}
			}
			hasController := false
			for _, node := range profile.ParameterControl.Nodes {
				hasController = hasController || node.Code == "feedback_controller" && node.Source.ObjectType == "Controller:WaterCoil"
			}
			if hasController {
				roles := map[string]bool{}
				requested, actual := map[int]float64{}, map[int]float64{}
				for _, signal := range loop.ControlSignals {
					if signal.ComponentIndex == profile.ComponentIndex {
						roles[signal.Role] = true
						for _, point := range bySeries[signal.SeriesID].Points {
							if signal.Role == "setpoint" && signal.VariableName == "System Node Setpoint Temperature" && hvacHasTemperatureSetpoint(point.Value) {
								targetObserved = true
							}
							if signal.Role == "actuator_request" {
								if signal.VariableName != "System Node Requested Mass Flow Rate" {
									t.Errorf("unused flow-setpoint output was promoted to actuator request: %+v", signal)
								}
								requested[point.X] = point.Value
							}
							if signal.Role == "actuator" {
								actual[point.X] = point.Value
							}
						}
					}
				}
				on, off := false, false
				for frame, request := range requested {
					value, known := actual[frame]
					if known {
						on = on || request > 1e-6 && value > 1e-6
						off = off || request == 0 && value == 0
					}
				}
				meaningfulFlow = meaningfulFlow || on && off
				for _, role := range []string{"sensor", "setpoint", "actuator", "actuator_request"} {
					if !roles[role] {
						t.Errorf("native water-coil profile %d omitted required %s channel", profile.ComponentIndex, role)
					}
				}
			}
		}
		for _, signal := range loop.ControlSignals {
			if signal.Required && signal.Status != "collected" {
				missing[signal.Status+":"+signal.VariableName+":"+signal.KeyValue]++
			}
		}
	}
	if len(missing) > 0 {
		for key, count := range missing {
			t.Errorf("required acquisition %s (%d bindings)", key, count)
		}
	}
	if len(families) != 4 {
		t.Fatalf("native evidence lacks expected families: %+v", families)
	}
	if !meaningfulFlow || !targetObserved {
		t.Fatalf("native run must show meaningful requested+actual flow on/off windows and a real temperature target, got flow=%v target=%v", meaningfulFlow, targetObserved)
	}
	unchanged, _ := os.ReadFile(path)
	if string(original) != string(unchanged) {
		t.Fatal("native acceptance modified original input fixture")
	}
	t.Logf("native controls: %d loops; files preserved at %s", len(result.PurposeResults.HVACLoops), result.OutputDirectory)
}
