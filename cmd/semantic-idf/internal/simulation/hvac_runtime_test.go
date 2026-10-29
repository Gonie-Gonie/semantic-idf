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
	sharedIndex := newHVACRuntimeSeriesIndex(series)
	seriesBefore, _ := json.Marshal(series)
	indexBefore, _ := json.Marshal([]any{sharedIndex.byOutput, sharedIndex.byNativeID})
	for _, loop := range report.Loops {
		profiles := hvacRuntimeProfiles(report, loop, request.Scope)
		result := HVACLoopRunResult{}
		attachHVACRuntimeControls(&result, profiles, loop, &doc, series)
		indexed := HVACLoopRunResult{}
		attachHVACRuntimeControlsWithIndex(&indexed, profiles, loop, &doc, sharedIndex)
		freshJSON, _ := json.Marshal(result)
		sharedJSON, _ := json.Marshal(indexed)
		if string(freshJSON) != string(sharedJSON) {
			t.Fatal("sharing immutable native source indexes changed loop controls")
		}
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
	sequential := buildHVACLoopRunResultsWithDocument(series, request, &doc)
	parallel := buildHVACLoopRunResultsWithDocumentWithExecutor(series, request, &doc, newResultBuildExecutor(4))
	sequentialJSON, _ := json.Marshal(sequential)
	parallelJSON, _ := json.Marshal(parallel)
	if string(sequentialJSON) != string(parallelJSON) {
		t.Fatal("parallel native controls changed loop order, coverage, observations or interval references")
	}
	seriesAfter, _ := json.Marshal(series)
	indexAfter, _ := json.Marshal([]any{sharedIndex.byOutput, sharedIndex.byNativeID})
	if string(seriesBefore) != string(seriesAfter) || string(indexBefore) != string(indexAfter) {
		t.Fatal("loop-local native reference rebasing mutated source arrays or shared indexes")
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
	t.Run("elective equipment channels do not change control coverage", func(t *testing.T) {
		loop := report.Loops[0]
		profiles := hvacRuntimeProfiles(report, loop, request.Scope)
		required := map[string]bool{}
		for _, profile := range profiles {
			for _, signal := range hvacRuntimeSignals(profile, loop, &doc) {
				required[signal.KeyValue+"/"+signal.VariableName+"/"+signal.ReportingFrequency] = true
			}
		}
		var observations []SimulationSeries
		for _, item := range series {
			if required[item.KeyValue+"/"+item.Name+"/"+item.ReportingFrequency] {
				observations = append(observations, item)
			}
		}
		baseline := HVACLoopRunResult{}
		attachHVACRuntimeControls(&baseline, profiles, loop, &doc, observations)
		baselineCoverage, _ := json.Marshal(baseline.ControlCoverage)
		extra := SimulationSeries{File: "another.csv", SourceID: "elective-energy", KeyValue: "First Fan", Name: "Fan Electricity Energy", ReportingFrequency: "Timestep", Column: "First Fan:Fan Electricity Energy [J](Timestep)", Points: []SimulationPoint{{X: 9999, Label: "12-31 24:00", Value: 0}}}
		for _, duplicate := range []bool{false, true} {
			changed := append(append([]SimulationSeries{}, observations...), extra)
			if duplicate {
				changed = append(changed, extra)
			}
			result := HVACLoopRunResult{}
			attachHVACRuntimeControls(&result, profiles, loop, &doc, changed)
			coverage, _ := json.Marshal(result.ControlCoverage)
			if string(coverage) != string(baselineCoverage) || result.ControlCoverage.Status != "complete" {
				t.Fatalf("elective file/time union changed required controls: %s != %s", coverage, baselineCoverage)
			}
			found := 0
			for _, signal := range result.ControlSignals {
				if signal.VariableName == extra.Name {
					found++
					if signal.Required || signal.Role != "operating" || signal.Graph != "equipment" || signal.SeriesID != extra.SourceID || signal.Status != "collected" {
						t.Fatalf("elective output lost its source or became a required control: %+v", signal)
					}
				}
			}
			if !duplicate && found != 1 || duplicate && found != 0 {
				t.Fatalf("missing/ambiguous elective output acquired a placeholder or arbitrary channel: %d", found)
			}
		}
	})
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
	requested := func(variable, frequency string) bool {
		for _, output := range plan.OutputObjects {
			if output.ObjectType == "Output:Variable" && output.KeyValue == "*" && output.VariableName == variable && output.ReportingFrequency == frequency {
				return true
			}
		}
		return false
	}
	wanted := append(hvacRuntimeActivityVariableNames(), "Plant Component Distributed Demand Rate", "VRF Heat Pump Terminal Unit Cooling Load Rate", "VRF Heat Pump Terminal Unit Heating Load Rate")
	wanted = append(wanted, "Water Heater Tank Temperature", "Water Heater Heat Pump Control Tank Temperature", "Water Heater Temperature Node 1", "Water Heater Temperature Node 12")
	for _, variable := range wanted {
		if !requested(variable, "Timestep") {
			t.Fatalf("template expansion omitted required family response/dispatch: %s", variable)
		}
	}
	advanced := false
	for _, output := range plan.OutputObjects {
		if output.ObjectType == "Output:Diagnostics" {
			for _, field := range output.Fields {
				advanced = advanced || strings.EqualFold(field.Value, "DisplayAdvancedReportVariables")
			}
		}
	}
	if !advanced {
		t.Fatal("expanded native per-component dispatch requires advanced output registration")
	}
	// The template output plan precedes ExpandObjects, but observation keys
	// and ownership must be rebound exclusively to the executed model.
	expanded, err := simulationDocumentFromInput(filepath.Join("..", "..", "frontend", "src", "samples", "RefBldgLargeOfficeNew2004_Chicago.idf"))
	if err != nil {
		t.Fatal(err)
	}
	report := idf.AnalyzeHVAC(expanded)
	expandedAssignment := false
	for _, loop := range report.Loops {
		if loop.Name != "CoolSys1" {
			continue
		}
		for _, profile := range hvacRuntimeProfiles(report, loop, request.Scope) {
			if !strings.HasPrefix(hvacRuntimeComponent(profile).ObjectType, "Chiller:") {
				continue
			}
			var observations []SimulationSeries
			seen := map[string]bool{}
			for _, binding := range hvacRuntimeSignals(profile, loop, &expanded) {
				if !requested(binding.VariableName, binding.ReportingFrequency) {
					t.Fatalf("executed source was not acquired by template plan: %+v", binding)
				}
				identity := binding.KeyValue + "|" + binding.VariableName
				if seen[identity] {
					continue
				}
				seen[identity] = true
				observations = append(observations, SimulationSeries{File: "expanded.sql", SourceID: identity, KeyValue: binding.KeyValue, Name: binding.VariableName, ReportingFrequency: binding.ReportingFrequency,
					Points: []SimulationPoint{{X: 1, Label: "01-01 00:10", Value: 0}, {X: 2, Label: "01-01 00:20", Value: 0}}, NativeIntervals: []SimulationNativeInterval{{TimeIndex: 1, EnvironmentPeriodIndex: 3, IntervalMinutes: 10, SimulationDays: 1}, {TimeIndex: 2, EnvironmentPeriodIndex: 3, IntervalMinutes: 10, SimulationDays: 1}}})
			}
			bound := HVACLoopRunResult{}
			attachHVACRuntimeControls(&bound, []idf.HVACOperatingProfile{profile}, loop, &expanded, observations)
			if bound.ControlCoverage.Status != "complete" {
				t.Fatalf("expanded exact source binding failed: %+v", bound.ControlCoverage)
			}
			for _, binding := range bound.ControlSignals {
				if binding.Role == "dispatch" {
					expandedAssignment = binding.Status == "collected" && binding.Source == hvacRuntimeComponent(profile) && binding.KeyValue != "*" && binding.ComponentIndex == profile.ComponentIndex
				}
			}
			break
		}
	}
	if !expandedAssignment {
		t.Fatal("expanded template lost its native individual assignment or exact executed owner")
	}
}

func TestHVACRuntimeManagerReasonInputs(t *testing.T) {
	t.Run("tank thermostat quantities retain native ownership", func(t *testing.T) {
		tank := idf.Object{Index: 4, Type: "WaterHeater:Mixed", Fields: make([]idf.Field, 33)}
		tank.Fields[0].Value, tank.Fields[2].Value = "Actual Tank", "Tank Target"
		schedule := idf.HVACOperatingSource{ObjectIndex: 8, ObjectType: "Schedule:Constant", ObjectName: "Tank Target"}
		source := idf.HVACOperatingSource{ObjectIndex: tank.Index, ObjectType: tank.Type, ObjectName: "Actual Tank"}
		profile := idf.HVACOperatingProfile{ComponentIndex: tank.Index, ParameterControl: idf.HVACControlSequence{Nodes: []idf.HVACControlNode{{ID: "equipment", Code: "equipment_response", Source: source}, {ID: "target", Kind: "schedule", Source: schedule}, {ID: "availability", Kind: "schedule", Source: idf.HVACOperatingSource{ObjectIndex: 9, ObjectType: "Schedule:Constant", ObjectName: "Hours"}}}}}
		doc := idf.Document{Objects: []idf.Object{tank}}
		before, _ := json.Marshal(doc)
		found := map[string]bool{}
		var observations []SimulationSeries
		for _, binding := range hvacRuntimeSignals(profile, idf.HVACLoop{}, &doc) {
			if binding.VariableName == "Water Heater Tank Temperature" {
				found["temperature"] = binding.Graph == "onOff" && binding.Role == "sensor" && binding.Source == source && binding.KeyValue == "Actual Tank"
			}
			if binding.VariableName == "Water Heater Runtime Fraction" {
				found["runtime"] = binding.Role == "activity" && binding.Source == source
			}
			if binding.Role == "setpoint" {
				found["target"] = binding.Domain == "temperature_schedule" && binding.KeyValue == "Tank Target" && binding.Source == source
			}
			if binding.NodeID == "target" {
				found["tagged"] = binding.Domain == "temperature_schedule"
			}
			if binding.NodeID == "availability" && binding.Domain == "temperature_schedule" {
				t.Fatal("unrelated availability schedule acquired temperature units")
			}
			value := 60.0
			if strings.Contains(binding.VariableName, "Runtime Fraction") {
				value = .375
			}
			if binding.Graph == "parameterControl" && binding.Role == "schedule" && binding.KeyValue == "Tank Target" {
				continue
			}
			observations = append(observations, SimulationSeries{File: "native.sql", SourceID: binding.ID, KeyValue: binding.KeyValue, Name: binding.VariableName, ReportingFrequency: binding.ReportingFrequency, Points: []SimulationPoint{{X: 1, Label: "01-01 00:10", Value: value}}, NativeIntervals: []SimulationNativeInterval{{TimeIndex: 1, EnvironmentPeriodIndex: 3, IntervalMinutes: 10, SimulationDays: 1}}})
		}
		if len(found) != 4 || !found["temperature"] || !found["runtime"] || !found["target"] || !found["tagged"] {
			t.Fatalf("tank thermostat lost native context: %v", found)
		}
		bound := HVACLoopRunResult{}
		attachHVACRuntimeControls(&bound, []idf.HVACOperatingProfile{profile}, idf.HVACLoop{}, &doc, observations)
		if bound.ControlCoverage.Status != "complete" {
			t.Fatalf("native tank quantities did not bind: %+v", bound.ControlCoverage)
		}
		for i, series := range observations {
			if series.Name == "Water Heater Tank Temperature" {
				observations = append(observations[:i:i], observations[i+1:]...)
				break
			}
		}
		missing := HVACLoopRunResult{}
		attachHVACRuntimeControls(&missing, []idf.HVACOperatingProfile{profile}, idf.HVACLoop{}, &doc, observations)
		if missing.ControlCoverage.Status != "error" {
			t.Fatal("missing tank thermostat measurement silently became complete")
		}
		after, _ := json.Marshal(doc)
		if string(before) != string(after) {
			t.Fatal("tank observation binding mutated executed source")
		}
	})
	t.Run("stratified native nodes and element duties remain distinct", func(t *testing.T) {
		tank := idf.Object{Index: 7, Type: "WaterHeater:Stratified", Fields: make([]idf.Field, 52)}
		tank.Fields[0].Value, tank.Fields[3].Value, tank.Fields[4].Value, tank.Fields[51].Value = "Layered", "2", "VerticalCylinder", "4.0"
		tank.Fields[8].Value, tank.Fields[12].Value = "Upper Target", "Lower Target"
		tank.Fields[11].Value, tank.Fields[15].Value = "1.5", "0"
		source := idf.HVACOperatingSource{ObjectIndex: tank.Index, ObjectType: tank.Type, ObjectName: "Layered"}
		inputs := hvacRuntimeWaterHeaterObservations(source, &idf.Document{Objects: []idf.Object{tank}})
		var temperatures []string
		targets := 0
		for _, input := range inputs {
			if input.Role == "sensor" {
				temperatures = append(temperatures, input.VariableName)
			}
			if input.Role == "setpoint" && input.Domain == "temperature_schedule" {
				targets++
			}
		}
		// Native top-down (lower,upper]: exactly 1.5 m belongs to node 2.
		if strings.Join(temperatures, ",") != "Water Heater Temperature Node 2,Water Heater Temperature Node 4" || targets != 2 {
			t.Fatalf("native element nodes/targets changed: %v %d", temperatures, targets)
		}
		variables := hvacRuntimeActivityVariables(tank.Type)
		if strings.Join(variables, ",") != "Water Heater Heating Rate,Water Heater Heater 1 Runtime Fraction,Water Heater Heater 2 Runtime Fraction" {
			t.Fatalf("stratified heaters collapsed to one duty: %v", variables)
		}
		for _, change := range []func(*idf.Object){func(o *idf.Object) { o.Fields[4].Value = "HorizontalCylinder" }, func(o *idf.Object) { o.Fields[3].Value = "Autocalculate" }} {
			copy := tank
			copy.Fields = append([]idf.Field{}, tank.Fields...)
			change(&copy)
			if nodes := hvacRuntimeStratifiedTemperatureNodes(copy); len(nodes) != 4 || nodes[0] != 1 || nodes[3] != 4 {
				t.Fatalf("unproven geometry guessed a heater sensor: %v", nodes)
			}
		}
		tank.Fields[51].Value = ""
		if nodes := hvacRuntimeStratifiedTemperatureNodes(tank); len(nodes) != 1 || nodes[0] != 1 {
			t.Fatalf("native Number of Nodes default changed: %v", nodes)
		}
	})
	t.Run("heat pump wrapper uses compressor owned observations", func(t *testing.T) {
		for _, typ := range []string{"WaterHeater:HeatPump:PumpedCondenser", "WaterHeater:HeatPump:WrappedCondenser"} {
			wrapper := idf.Object{Index: 2, Type: typ, Fields: make([]idf.Field, 39)}
			wrapper.Fields[0].Value, wrapper.Fields[2].Value = "Compressor", "Compressor Target"
			typeIndex, nameIndex := 16, 17
			if strings.Contains(typ, "Wrapped") {
				typeIndex, nameIndex = 15, 16
			}
			wrapper.Fields[typeIndex].Value, wrapper.Fields[nameIndex].Value = "WaterHeater:Mixed", "Backup Tank"
			tank := idf.Object{Index: 5, Type: "WaterHeater:Mixed", Fields: []idf.Field{{Value: "Backup Tank"}, {}, {Value: "Backup Target"}}}
			source := idf.HVACOperatingSource{ObjectIndex: wrapper.Index, ObjectType: wrapper.Type, ObjectName: "Compressor"}
			doc := idf.Document{Objects: []idf.Object{wrapper, tank}}
			profile := idf.HVACOperatingProfile{ComponentIndex: wrapper.Index, ParameterControl: idf.HVACControlSequence{Nodes: []idf.HVACControlNode{{ID: "equipment", Code: "equipment_response", Source: source}}}}
			control, compressor, target := false, false, false
			for _, binding := range hvacRuntimeSignals(profile, idf.HVACLoop{}, &doc) {
				if binding.Source != source || binding.VariableName == "Water Heater Runtime Fraction" || binding.VariableName == "Water Heater Tank Temperature" {
					t.Fatalf("wrapper borrowed backup heater state: %+v", binding)
				}
				control = control || binding.VariableName == "Water Heater Heat Pump Control Tank Temperature" && binding.KeyValue == "Compressor"
				compressor = compressor || binding.Role == "activity" && binding.VariableName == "Water Heater Compressor Part Load Ratio"
				target = target || binding.Role == "setpoint" && binding.KeyValue == "Compressor Target"
			}
			if !control || !compressor || !target {
				t.Fatal("wrapper lost its native compressor thermostat quantities")
			}
			doc.Objects = append(doc.Objects, tank)
			if len(hvacRuntimeWaterHeaterObservations(source, &doc)) != 0 {
				t.Fatal("duplicate typed tank relation was arbitrarily selected")
			}
		}
	})
	t.Run("on off retains assigned load and exact occurrence", func(t *testing.T) {
		component := idf.HVACOperatingSource{ObjectIndex: 8, ObjectType: "Chiller:Electric:EIR", ObjectName: "Selected Chiller"}
		profile := idf.HVACOperatingProfile{ComponentIndex: 8, LoopID: "plant:cooling", OnOff: idf.HVACControlSequence{Nodes: []idf.HVACControlNode{
			{ID: "dispatch:4", Code: "plant_dispatch", Source: idf.HVACOperatingSource{ObjectIndex: 4, ObjectType: "PlantEquipmentOperation:CoolingLoad", ObjectName: "Cooling Scheme"}},
			{ID: "mode", Kind: "schedule", Source: idf.HVACOperatingSource{ObjectIndex: 7, ObjectType: "Schedule:Constant", ObjectName: "Mode"}},
		}}, ParameterControl: idf.HVACControlSequence{Nodes: []idf.HVACControlNode{{ID: "equipment", Code: "equipment_response", Source: component}}}}
		loop := idf.HVACLoop{ID: profile.LoopID, Type: "PlantLoop", Name: "Cooling Loop", SupplySide: idf.HVACLoopSide{Branches: []idf.HVACBranch{{Components: []idf.HVACComponent{{ObjectIndex: 8, ObjectType: component.ObjectType, ObjectName: component.ObjectName}}}}}}
		dispatch, demand, activity, schedule := false, false, false, false
		for _, binding := range hvacRuntimeSignals(profile, loop, nil) {
			switch {
			case binding.Role == "dispatch":
				dispatch = binding.Graph == "onOff" && binding.NodeID == "decision" && binding.KeyValue == component.ObjectName && binding.Source == component && binding.VariableName == "Plant Component Distributed Demand Rate" && binding.ReportingFrequency == "Timestep"
			case binding.Graph == "onOff" && binding.Role == "demand":
				demand = binding.NodeID == "dispatch:4" && binding.Source.ObjectIndex == 4 && binding.KeyValue == loop.Name && binding.VariableName == "Plant Supply Side Cooling Demand Rate"
			case binding.Role == "activity":
				activity = binding.Graph == "parameterControl" && binding.VariableName == "Chiller Evaporator Cooling Rate"
			case binding.NodeID == "mode":
				schedule = binding.Domain == "continuous"
			}
		}
		if !dispatch || !demand || !activity || !schedule {
			t.Fatalf("assignment, loop context, measured output and raw mode schedule must remain distinct: %t %t %t %t", dispatch, demand, activity, schedule)
		}
		for _, change := range []func(*idf.HVACLoop){
			func(l *idf.HVACLoop) { l.DemandSide, l.SupplySide = l.SupplySide, idf.HVACLoopSide{} },
			func(l *idf.HVACLoop) { l.ID = "plant:other" },
			func(l *idf.HVACLoop) { l.SupplySide.Branches[0].Components[0].ObjectType = "Boiler:HotWater" },
			func(l *idf.HVACLoop) {
				l.SupplySide.Branches[0].Components = append(l.SupplySide.Branches[0].Components, l.SupplySide.Branches[0].Components[0])
			},
		} {
			copy := loop
			copy.SupplySide.Branches = append([]idf.HVACBranch{}, loop.SupplySide.Branches...)
			copy.SupplySide.Branches[0].Components = append([]idf.HVACComponent{}, loop.SupplySide.Branches[0].Components...)
			change(&copy)
			for _, binding := range hvacRuntimeSignals(profile, copy, nil) {
				if binding.Role == "dispatch" {
					t.Fatalf("individual dispatch borrowed another or ambiguous circuit: %+v", binding)
				}
			}
		}
	})
	t.Run("component setpoint native demand scheme", func(t *testing.T) {
		for _, test := range []struct {
			scheme, operation string
			wanted            []string
		}{
			{"", "Heating", []string{"System Node Setpoint Temperature"}},
			{"SingleSetpoint", "Cooling", []string{"System Node Setpoint Temperature"}},
			{"DualSetpointDeadband", "Cooling", []string{"System Node Setpoint High Temperature"}},
			{"DualSetpointDeadband", "Heating", []string{"System Node Setpoint Low Temperature"}},
			{"DualSetpointDeadband", "Dual", []string{"System Node Setpoint High Temperature", "System Node Setpoint Low Temperature"}},
		} {
			node := idf.HVACControlNode{Code: "plant_dispatch", Source: idf.HVACOperatingSource{ObjectType: "PlantEquipmentOperation:ComponentSetpoint"}, Parameters: []idf.HVACControlParameter{{Name: "Demand Calculation Node Name", Value: "Inlet"}, {Name: "Setpoint Node Name", Value: "Target"}, {Name: "Operation Type", Value: test.operation}, {Name: "Plant Loop Demand Calculation Scheme", Value: test.scheme}}}
			var setpoints []string
			for _, input := range hvacRuntimeReasonInputs(node, idf.HVACLoop{Type: "PlantLoop", Name: "Native"}, nil) {
				if input.role == "setpoint" {
					if input.key != "Target" {
						t.Fatal("target borrowed inlet node")
					}
					setpoints = append(setpoints, input.variable)
				}
			}
			if strings.Join(setpoints, ",") != strings.Join(test.wanted, ",") {
				t.Fatalf("%s %s requested wrong native target: %v", test.scheme, test.operation, setpoints)
			}
		}
	})
	t.Run("fan input and thermostat request are observations", func(t *testing.T) {
		doc := parsePurposePlanFixture(t, "Version,24.2;\nZone,Served;\nZone,Other;\n")
		fan := idf.HVACOperatingSource{ObjectIndex: 4, ObjectType: "Fan:VariableVolume", ObjectName: "Fan"}
		thermostat := idf.HVACOperatingSource{ObjectIndex: 7, ObjectType: "ZoneControl:Thermostat", ObjectName: "Thermostat"}
		profile := idf.HVACOperatingProfile{ComponentIndex: 4, OnOff: idf.HVACControlSequence{Nodes: []idf.HVACControlNode{{ID: "thermostat:7", Code: "zone_thermostat", Source: thermostat, Parameters: []idf.HVACControlParameter{{Name: "Zone or ZoneList Name", Value: "Served"}}}}}, ParameterControl: idf.HVACControlSequence{Nodes: []idf.HVACControlNode{{ID: "equipment", Code: "equipment_response", Source: fan}, {ID: "port:inlet", Code: "inlet_state", Source: fan, Parameters: []idf.HVACControlParameter{{Name: "Air Inlet Node Name", Value: "Inlet"}}}}}}
		flow, requests := false, 0
		for _, binding := range hvacRuntimeSignals(profile, idf.HVACLoop{}, &doc) {
			if binding.Graph != "onOff" {
				continue
			}
			if binding.NodeID == "demand" {
				flow = binding.Source == fan && binding.Role == "sensor" && binding.KeyValue == "Inlet" && binding.VariableName == "System Node Mass Flow Rate"
			} else {
				if binding.Source != thermostat || binding.KeyValue != "Served" {
					t.Fatalf("thermostat borrowed another source or zone: %+v", binding)
				}
				if binding.Role == "demand" {
					requests++
				}
			}
		}
		if !flow || requests != 2 {
			t.Fatalf("missing actual flow or distinct zone heat/cool requests: %v %d", flow, requests)
		}
	})
	t.Run("response families retain native meaning", func(t *testing.T) {
		for _, test := range []struct{ typ, variable string }{
			{"Fan:VariableVolume", "Fan Electricity Rate"}, {"Pump:VariableSpeed", "Pump Electricity Rate"},
			{"Coil:Heating:Water", "Heating Coil Heating Rate"}, {"Chiller:Electric:ReformulatedEIR", "Chiller Evaporator Cooling Rate"},
			{"Boiler:HotWater", "Boiler Heating Rate"}, {"CoolingTower:VariableSpeed", "Cooling Tower Heat Transfer Rate"},
			{"FluidCooler:TwoSpeed", "Cooling Tower Heat Transfer Rate"}, {"EvaporativeFluidCooler:SingleSpeed", "Cooling Tower Heat Transfer Rate"},
			{"HeatExchanger:FluidToFluid", "Fluid Heat Exchanger Heat Transfer Rate"}, {"HeatPump:PlantLoop:EIR:Cooling", "Heat Pump Load Side Heat Transfer Rate"},
			{"Humidifier:Steam:Electric", "Humidifier Water Volume Flow Rate"},
			{"EvaporativeCooler:Direct:CelDekPad", "Evaporative Cooler Electricity Rate"},
		} {
			variables := hvacRuntimeActivityVariables(test.typ)
			if len(variables) != 1 || variables[0] != test.variable {
				t.Fatalf("%s: %v", test.typ, variables)
			}
		}
		if len(hvacRuntimeActivityVariables("Pipe:Adiabatic")) != 0 {
			t.Fatal("passive flow invented powered activity")
		}
		for _, test := range []struct{ typ, first, second string }{
			{"WaterHeater:Mixed", "Water Heater Heating Rate", "Water Heater Runtime Fraction"},
			{"ZoneHVAC:IdealLoadsAirSystem", "Zone Ideal Loads Zone Total Heating Rate", "Zone Ideal Loads Zone Total Cooling Rate"},
			{"ZoneHVAC:PackagedTerminalHeatPump", "Zone Packaged Terminal Heat Pump Total Heating Rate", "Zone Packaged Terminal Heat Pump Total Cooling Rate"},
			{"ZoneHVAC:FourPipeFanCoil", "Fan Coil Heating Rate", "Fan Coil Total Cooling Rate"},
			{"ZoneHVAC:LowTemperatureRadiant:VariableFlow", "Zone Radiant HVAC Heating Rate", "Zone Radiant HVAC Cooling Rate"},
			{"HeatExchanger:AirToAir:FlatPlate", "Heat Exchanger Total Heating Rate", "Heat Exchanger Total Cooling Rate"},
			{"AirConditioner:VariableRefrigerantFlow", "VRF Heat Pump Total Heating Rate", "VRF Heat Pump Total Cooling Rate"},
		} {
			variables := hvacRuntimeActivityVariables(test.typ)
			if len(variables) != 2 || variables[0] != test.first || variables[1] != test.second {
				t.Fatalf("family heat/cool responses collapsed or renamed: %s %v", test.typ, variables)
			}
		}
		vrf := idf.HVACControlNode{Code: "terminal_load_and_operating_mode", Source: idf.HVACOperatingSource{ObjectType: "AirConditioner:VariableRefrigerantFlow", ObjectName: "VRF"}}
		inputs := hvacRuntimeReasonInputs(vrf, idf.HVACLoop{}, nil)
		if len(inputs) != 2 || inputs[0].role != "demand" || inputs[0].key != "VRF" || inputs[0].variable != "VRF Heat Pump Terminal Unit Cooling Load Rate" || inputs[1].variable != "VRF Heat Pump Terminal Unit Heating Load Rate" {
			t.Fatalf("VRF selected terminal requests replaced by reported capacity: %+v", inputs)
		}
	})
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
	assignments := map[string][3]int{}
	tankTemperature, tankTarget, tankDuty := false, false, false
	meaningfulFlow, targetObserved := false, false
	operating := map[string]SimulationSeries{}
	operatingKinds := map[string]bool{}
	boilerEfficiencyZero := false
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
			if signal.Role == "operating" {
				item := bySeries[signal.SeriesID]
				if signal.Required || signal.Graph != "equipment" || signal.Status != "collected" || signal.KeyValue != signal.Source.ObjectName || !purposeTimestepFrequency(item.ReportingFrequency) || len(item.Points) != 288 {
					t.Fatalf("native operating channel lost exact source/cadence/full samples: %+v points=%d", signal, len(item.Points))
				}
				operating[signal.SeriesID] = item
				operatingKinds[signal.VariableName] = true
				if signal.Source.ObjectName == "HeatSys1 Boiler" && hvacOperatingBoilerFuelVariable(signal.VariableName) && !strings.HasPrefix(signal.VariableName, "Boiler NaturalGas ") {
					t.Fatalf("native hot-water boiler acquired another fuel: %+v", signal)
				}
				if signal.VariableName == "Boiler Efficiency" {
					for _, point := range item.Points {
						boilerEfficiencyZero = boilerEfficiencyZero || point.Value == 0
					}
				}
			}
			if signal.Source.ObjectType == "WaterHeater:Mixed" && signal.Source.ObjectName == "SWHSys1 Water Heater" {
				if signal.Graph == "onOff" && signal.Role == "sensor" && signal.VariableName == "Water Heater Tank Temperature" {
					tankTemperature = len(bySeries[signal.SeriesID].Points) == 288
				}
				if signal.Graph == "onOff" && signal.Role == "setpoint" && signal.Domain == "temperature_schedule" && signal.KeyValue == "SWHSys1 Water Heater Setpoint Temperature Schedule Name" {
					tankTarget = true
				}
				if signal.Role == "activity" && signal.VariableName == "Water Heater Runtime Fraction" {
					for _, point := range bySeries[signal.SeriesID].Points {
						if point.Value < 0 || point.Value > 1 {
							t.Fatalf("native tank duty outside reported fraction domain: %v", point.Value)
						}
						tankDuty = tankDuty || point.Value > 0 && point.Value < 1
					}
				}
			}
			if signal.Role == "dispatch" {
				if signal.Graph != "onOff" || signal.NodeID != "decision" || signal.ReportingFrequency != "Timestep" || signal.VariableName != "Plant Component Distributed Demand Rate" || loop.Name == "TowerWaterSys" && strings.HasPrefix(signal.Source.ObjectType, "Chiller:") {
					t.Fatalf("native assignment lost selected supply occurrence or cadence: %+v", signal)
				}
				counts := [3]int{}
				for _, point := range bySeries[signal.SeriesID].Points {
					if point.Value < 0 {
						counts[0]++
					} else if point.Value > 0 {
						counts[2]++
					} else {
						counts[1]++
					}
				}
				assignments[signal.Source.ObjectName] = counts
			}
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
	for _, name := range []string{"CoolSys1 Chiller 1", "CoolSys1 Chiller 2", "TowerWaterSys CoolTower", "HeatSys1 Boiler", "SWHSys1 Water Heater"} {
		counts, found := assignments[name]
		if !found || counts[0]+counts[1]+counts[2] != 288 {
			t.Fatalf("native assignment lacks complete two-day Timestep coverage: %s %v", name, counts)
		}
		if strings.Contains(name, "Chiller") || strings.Contains(name, "CoolTower") {
			if counts[0] == 0 || counts[2] != 0 {
				t.Fatalf("native cooling assignment sign changed: %s %v", name, counts)
			}
		} else if counts[2] == 0 || counts[0] != 0 {
			t.Fatalf("native heating assignment sign changed: %s %v", name, counts)
		}
	}
	if !meaningfulFlow || !targetObserved {
		t.Fatalf("native run must show meaningful requested+actual flow on/off windows and a real temperature target, got flow=%v target=%v", meaningfulFlow, targetObserved)
	}
	if !tankTemperature || !tankTarget || !tankDuty {
		t.Fatalf("native tank thermostat evidence missing: temperature=%v target=%v fractional duty=%v", tankTemperature, tankTarget, tankDuty)
	}
	for _, variable := range []string{"Fan Electricity Rate", "Fan Electricity Energy", "Fan Air Mass Flow Rate", "Pump Electricity Rate", "Pump Electricity Energy", "Pump Shaft Power", "Pump Mass Flow Rate", "Chiller Part Load Ratio", "Chiller COP", "Chiller Electricity Energy", "Boiler Part Load Ratio", "Boiler Efficiency", "Boiler NaturalGas Rate", "Boiler NaturalGas Energy"} {
		if !operatingKinds[variable] {
			t.Fatalf("native result omitted core equipment operating quantity: %s", variable)
		}
	}
	if !boilerEfficiencyZero {
		t.Fatal("native OFF boiler efficiency zero was dropped or replaced")
	}
	t.Logf("native operating quantities: %d unique Timestep channels, %d variable names; 288 observations per channel", len(operating), len(operatingKinds))
	unchanged, _ := os.ReadFile(path)
	if string(original) != string(unchanged) {
		t.Fatal("native acceptance modified original input fixture")
	}
	t.Logf("native controls: %d loops; files preserved at %s", len(result.PurposeResults.HVACLoops), result.OutputDirectory)
}
