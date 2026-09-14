package simulation

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathDistrictPlanKeepsMonthlyLedgerAndAddsHourlyCharts(t *testing.T) {
	doc := epathDistrictPurposeOriginal(t)
	identity := func(objectType, key, name, frequency string) [4]string {
		return [4]string{strings.ToLower(strings.TrimSpace(objectType)), strings.ToLower(strings.TrimSpace(key)),
			strings.ToLower(strings.TrimSpace(name)), strings.ToLower(strings.TrimSpace(frequency))}
	}
	originalHourly := map[[4]string]idf.Object{}
	for _, object := range doc.Objects {
		var key, name, frequency string
		switch {
		case strings.EqualFold(object.Type, "Output:Variable") && len(object.Fields) >= 3:
			key, name, frequency = object.Fields[0].Value, object.Fields[1].Value, object.Fields[2].Value
		case strings.EqualFold(object.Type, "Output:Meter") && len(object.Fields) >= 2:
			key, frequency = object.Fields[0].Value, object.Fields[1].Value
		default:
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(frequency), "Hourly") {
			continue
		}
		tuple := identity(object.Type, key, name, frequency)
		if _, duplicate := originalHourly[tuple]; duplicate {
			t.Fatalf("original has ambiguous exact Hourly request identity: %v", tuple)
		}
		originalHourly[tuple] = object
	}
	for _, scope := range []SimulationPurposeScope{
		{ZoneMode: "all", PeriodMode: "full"},
		{ZoneMode: "selected", ZoneNames: []string{"SPACE1-1"}, PeriodMode: "full"},
	} {
		t.Run(scope.ZoneMode, func(t *testing.T) {
			plan := epathDistrictPurposePlan(doc, scope)
			if !plan.RequiresSQL || !plan.RequiresDiscovery || plan.EstimatedFrames != 8760 {
				t.Fatalf("Monthly ledger/Hourly chart SQL contract changed: SQL=%t discovery=%t frames=%d", plan.RequiresSQL, plan.RequiresDiscovery, plan.EstimatedFrames)
			}
			assertEnergyPathMonthlyHourlyRequestPairs(t, plan)
			counts, monthlyCounts, hourlyCounts := map[string]int{}, map[string]int{}, map[string]int{}
			meters, signatures := map[string]bool{}, map[string]bool{}
			for _, output := range plan.OutputObjects {
				if signatures[output.Signature] || output.Signature == "" {
					t.Fatalf("duplicate/missing exact request signature: %#v", output)
				}
				signatures[output.Signature] = true
				counts[output.ObjectType]++
				// Reuse only an exact original Hourly identity, including its
				// literal key: a wildcard cannot supply an exact-key opener.
				// Monthly, support outputs and unmatched Hourly requests remain
				// temporary additions without an original object index.
				original, exists := originalHourly[identity(output.ObjectType, output.KeyValue, output.VariableName, output.ReportingFrequency)]
				if exists {
					if output.State != PurposeOutputStateExisting || output.ObjectIndex == nil || *output.ObjectIndex != original.Index || len(output.Fields) != len(original.Fields) {
						t.Fatalf("exact original Hourly request lost its state/index/fields: output=%#v original=%#v", output, original)
					}
					for index, field := range original.Fields {
						if !strings.EqualFold(strings.TrimSpace(field.Value), strings.TrimSpace(output.Fields[index].Value)) {
							t.Fatalf("reused original Hourly index %d changed field %d: %q -> %q", original.Index, index, field.Value, output.Fields[index].Value)
						}
					}
				} else if output.State != PurposeOutputStateTemporary || output.ObjectIndex != nil {
					t.Fatalf("new request acquired an original output identity: %#v", output)
				}
				if output.ObjectType != "Output:Variable" && output.ObjectType != "Output:Meter" {
					continue
				}
				if !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) {
					t.Fatalf("Energy Path added an unscoped measurement: %#v", output)
				}
				switch output.ReportingFrequency {
				case "Monthly":
					monthlyCounts[output.ObjectType]++
				case "Hourly":
					hourlyCounts[output.ObjectType]++
					if output.Weight != "heavy" || output.Reason != "Basic Energy Path" {
						t.Fatalf("Hourly source chart lost its explicit purpose/weight: %#v", output)
					}
				default:
					t.Fatalf("Energy Path added an unsupported chart/ledger frequency: %#v", output)
				}
				if output.ObjectType == "Output:Meter" {
					meters[output.KeyValue] = true
					continue
				}
				name := strings.ToLower(output.VariableName)
				if strings.Contains(name, "fan electric") || strings.Contains(name, "pump electric") ||
					strings.Contains(name, "mass flow") || strings.Contains(name, "volume flow") || strings.Contains(name, "supply air volume") ||
					name == "heat exchanger electricity rate" || name == "heat exchanger electricity energy" {
					t.Fatalf("allocation added heavy component/airflow evidence: %#v", output)
				}
			}
			wantMeters := map[string]bool{}
			for _, name := range []string{
				"Cooling:DistrictCooling", "Cooling:Electricity", "DistrictCooling:Facility",
				"DistrictHeating:Facility", "DistrictHeating:Heating", "DistrictHeatingWater:Facility",
				"Electricity:Facility", "Fans:Electricity", "Heating:DistrictHeating",
				"Heating:DistrictHeatingWater", "Heating:Electricity", "HeatRecovery:Electricity",
				"InteriorEquipment:Electricity", "InteriorLights:Electricity", "Pumps:Electricity",
			} {
				wantMeters[name] = true
			}
			if !reflect.DeepEqual(meters, wantMeters) {
				t.Fatalf("original broad-meter request roster changed: %v", meters)
			}
			if scope.ZoneMode == "all" {
				wantSeries := map[string]int{"Output:Meter": 15, "Output:Variable": 952}
				wantCounts := map[string]int{"Output:Meter": 30, "Output:Variable": 1904, "Output:SQLite": 1, "Output:VariableDictionary": 1}
				if !reflect.DeepEqual(monthlyCounts, wantSeries) || !reflect.DeepEqual(hourlyCounts, wantSeries) ||
					len(plan.OutputObjects) != 1936 || !reflect.DeepEqual(counts, wantCounts) {
					t.Fatalf("District must keep 967 Monthly measurements and add exactly their 967 Hourly companions: total=%d / %v; Monthly=%v Hourly=%v", len(plan.OutputObjects), counts, monthlyCounts, hourlyCounts)
				}
			}
			for _, name := range []string{"Zone Air System Sensible Cooling Energy", "Zone Air System Sensible Heating Energy"} {
				for _, zone := range []string{"PLENUM-1", "SPACE1-1", "SPACE2-1", "SPACE3-1", "SPACE4-1", "SPACE5-1"} {
					got := epathDistrictPurposeOutputs(plan, "Output:Variable", zone, name, "Monthly")
					want := 1
					if scope.ZoneMode == "selected" && zone != "SPACE1-1" {
						want = 0
					}
					if len(got) != want {
						t.Fatalf("selected Monthly load request %s/%s count=%d, want %d", zone, name, len(got), want)
					}
				}
			}
			// Thermal context is not purchased HX electricity or airflow.
			for _, component := range []string{"Sensible", "Latent", "Total"} {
				for _, service := range []string{"Cooling", "Heating"} {
					for _, kind := range []string{"Energy", "Rate"} {
						name := "Heat Exchanger " + component + " " + service + " " + kind
						if got := epathDistrictPurposeOutputs(plan, "Output:Variable", "DOAS Heat Recovery", name, "Monthly"); len(got) != 1 {
							t.Fatalf("lost exact shared DOAS Monthly thermal context %s: %d", name, len(got))
						}
					}
				}
			}
		})
	}
}

func TestEnergyPathDistrictPreservesManualHourlyOutputs(t *testing.T) {
	doc := epathDistrictPurposeOriginal(t)
	originalText := doc.String()
	before := epathDistrictPurposeManualHourly(doc)
	for _, tuple := range [][2]string{
		{"DOAS Supply Fan Outlet", "System Node Standard Density Volume Flow Rate"},
		{"DOAS Mixed Air Outlet", "System Node Mass Flow Rate"},
		{"DOAS Outdoor Air Inlet", "System Node Mass Flow Rate"},
		{"*", "Heat Exchanger Electricity Rate"},
		{"*", "Heat Exchanger Latent Gain Rate"},
	} {
		found := 0
		for _, fields := range before {
			if fields[0] == tuple[0] && fields[1] == tuple[1] {
				found++
			}
		}
		if found != 1 {
			t.Fatalf("original manual Hourly tuple %v count=%d, want one", tuple, found)
		}
	}
	for _, scope := range []SimulationPurposeScope{
		{ZoneMode: "all", PeriodMode: "full"},
		{ZoneMode: "selected", ZoneNames: []string{"SPACE1-1"}, PeriodMode: "full"},
	} {
		t.Run(scope.ZoneMode, func(t *testing.T) {
			plan := epathDistrictPurposePlan(doc, scope)
			request := PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd)
			if len(request.Updates) != 0 || len(request.RemoveObjectIndexes) != 0 {
				t.Fatalf("Monthly additions rewrite/remove original requests: %#v", request)
			}
			applied, preview := idf.ApplyOutput(doc, request)
			if !preview.CanApply {
				t.Fatalf("original run-copy output additions blocked: %#v", preview)
			}
			after := epathDistrictPurposeManualHourly(applied)
			for index, fields := range before {
				if !reflect.DeepEqual(fields, after[index]) {
					t.Fatalf("original Hourly index %d changed fields: before=%v after=%v", index, fields, after[index])
				}
			}
			// The run copy now also includes requested Hourly source charts.
			// Every appended entry must match one exact planned field tuple,
			// and every planned addition must occur once (no extras or gaps).
			addedHourly := map[string]int{}
			for _, output := range plan.OutputObjects {
				if output.ObjectType != "Output:Variable" || output.ReportingFrequency != "Hourly" || output.State == PurposeOutputStateExisting {
					continue
				}
				fields := make([]string, len(output.Fields))
				for index, field := range output.Fields {
					fields[index] = field.Value
				}
				addedHourly[strings.Join(fields, "\x00")]++
			}
			for index, fields := range after {
				if _, original := before[index]; original {
					continue
				}
				key := strings.Join(fields, "\x00")
				if index < len(doc.Objects) || addedHourly[key] != 1 {
					t.Fatalf("unexpected or duplicate appended Hourly output at %d: %v", index, fields)
				}
				delete(addedHourly, key)
			}
			if len(addedHourly) != 0 {
				t.Fatalf("run copy omitted %d exact requested Hourly source charts", len(addedHourly))
			}
			if doc.String() != originalText {
				t.Fatal("building a run copy mutated the original parsed document")
			}
			// An actual temporary Monthly request remains unindexed even when
			// a same-name wildcard Hourly output exists in the original IDF.
			name := "Heat Exchanger Sensible Cooling Rate"
			outputs := epathDistrictPurposeOutputs(plan, "Output:Variable", "DOAS Heat Recovery", name, "Monthly")
			if len(outputs) != 1 || outputs[0].ObjectIndex != nil || outputs[0].State != PurposeOutputStateTemporary {
				t.Fatalf("Monthly context borrowed an original Hourly opener: %#v", outputs)
			}
			dictionary := energyExplanationDictionary{
				row:                sqlOutputDictionaryRow{name: name, keyValue: "DOAS HEAT RECOVERY", units: "W", reportingFrequency: "Monthly"},
				reportingFrequency: "Monthly",
			}
			if got := energyExplanationObjectIndexForDictionary(dictionary, &plan); got != nil {
				t.Fatalf("unindexed actual Monthly request invented an Output index %d", *got)
			}
			// Only the executed copy has a real new Monthly Output index. It
			// must not be confused with an earlier original Hourly index.
			monthly := 0
			for _, object := range applied.Objects {
				if object.Type == "Output:Variable" && len(object.Fields) >= 3 &&
					object.Fields[0].Value == "DOAS Heat Recovery" && object.Fields[1].Value == name && object.Fields[2].Value == "Monthly" {
					monthly++
					if object.Index < len(doc.Objects) {
						t.Fatal("Monthly context overwrote an original object")
					}
				}
			}
			if monthly != 1 {
				t.Fatalf("run copy has %d exact Monthly context requests, want one", monthly)
			}
		})
	}
}

// This generic-reader regression is intentionally independent of the private
// PTAC/VRF matchers. A source's Output opener must have its actual frequency.
func TestEnergyPathDistrictOutputIndexRequiresMatchingFrequency(t *testing.T) {
	for _, meter := range []bool{false, true} {
		label, objectType, key, name := "variable", "Output:Variable", "DOAS HEAT RECOVERY", "Heat Exchanger Sensible Cooling Rate"
		if meter {
			label, objectType, key, name = "meter", "Output:Meter", "HeatRecovery:Electricity", ""
		}
		t.Run(label, func(t *testing.T) {
			hourlyIndex, monthlyIndex := 7, 42
			hourly := PurposeOutputObject{ObjectType: objectType, KeyValue: key, VariableName: name, ReportingFrequency: "Hourly", ObjectIndex: &hourlyIndex, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}}
			monthly := hourly
			monthly.ReportingFrequency, monthly.ObjectIndex = "Monthly", &monthlyIndex
			row := sqlOutputDictionaryRow{name: name, keyValue: key, units: "W", reportingFrequency: "Monthly"}
			if meter {
				row.name, row.keyValue, row.units = key, "", "J"
			}
			dictionary := energyExplanationDictionary{row: row, isMeter: meter, reportingFrequency: "Monthly"}
			for _, test := range []struct {
				name    string
				outputs []PurposeOutputObject
				want    *int
			}{
				{"earlier Hourly does not shadow Monthly", []PurposeOutputObject{hourly, monthly}, &monthlyIndex},
				{"Monthly first remains the opener", []PurposeOutputObject{monthly, hourly}, &monthlyIndex},
				{"no matching frequency remains unknown", []PurposeOutputObject{hourly}, nil},
			} {
				t.Run(test.name, func(t *testing.T) {
					got := energyExplanationObjectIndexForDictionary(dictionary, &PurposeRunPlan{OutputObjects: test.outputs})
					if !reflect.DeepEqual(got, test.want) {
						t.Errorf("source Output index=%v, want %v (Hourly=7, Monthly=42)", got, test.want)
					}
				})
			}
		})
	}
}

func TestEnergyPathDistrictOutputIndexFrequencyBoundaries(t *testing.T) {
	for _, meter := range []bool{false, true} {
		label, objectType, key, name := "variable", "Output:Variable", "DOAS HEAT RECOVERY", "Heat Exchanger Sensible Cooling Rate"
		if meter {
			label, objectType, key, name = "meter", "Output:Meter", "HeatRecovery:Electricity", ""
		}
		t.Run(label, func(t *testing.T) {
			for _, test := range []struct {
				name, sourceFrequency, rowFrequency, outputFrequency, fieldFrequency string
				match                                                                bool
			}{
				{"trim and case", " monthly ", "", " MONTHLY ", "", true},
				{"source row fallback", "", "Monthly", "Monthly", "", true},
				{"blank source metadata uses row", "  ", "Monthly", "Monthly", "", true},
				{"source metadata precedes row", "Monthly", "Hourly", "Monthly", "", true},
				{"row cannot override source metadata", "Hourly", "Monthly", "Monthly", "", false},
				{"output field fallback", "Monthly", "", "", "Monthly", true},
				{"blank output metadata uses field", "Monthly", "", "  ", " monthly ", true},
				{"output metadata precedes field", "Monthly", "", "Monthly", "Hourly", true},
				{"field cannot override output metadata", "Monthly", "", "Hourly", "Monthly", false},
				{"omitted input frequency defaults Hourly", "Hourly", "", "", "", true},
				{"input default is not Monthly", "Monthly", "", "", "", false},
				{"unknown SQL is not explicit Hourly", "", "", "Hourly", "", false},
				{"unknown SQL is not default Hourly", "", "", "", "", false},
				{"SQL spaced Run Period", " Run Period ", "", "RunPeriod", "", true},
				{"SQL Run Period row fallback", "", "run period", "", " RunPeriod ", true},
				{"SQL canonical RunPeriod", "RunPeriod", "", "runperiod", "", true},
				{"Run Period is not Annual", "Run Period", "", "Annual", "", false},
				{"Annual is not RunPeriod", "Annual", "", "RunPeriod", "", false},
			} {
				t.Run(test.name, func(t *testing.T) {
					index := 42
					output := PurposeOutputObject{
						ObjectType: objectType, KeyValue: key, VariableName: name,
						ReportingFrequency: test.outputFrequency, ObjectIndex: &index,
						PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy},
					}
					if test.fieldFrequency != "" {
						output.Fields = []idf.OutputFieldValue{{Name: "Reporting Frequency", Value: test.fieldFrequency}}
					}
					row := sqlOutputDictionaryRow{name: name, keyValue: key, reportingFrequency: test.rowFrequency}
					if meter {
						row.name, row.keyValue = key, ""
					}
					dictionary := energyExplanationDictionary{row: row, isMeter: meter, reportingFrequency: test.sourceFrequency}
					got := energyExplanationObjectIndexForDictionary(dictionary, &PurposeRunPlan{OutputObjects: []PurposeOutputObject{output}})
					if test.match && (got == nil || *got != index) || !test.match && got != nil {
						t.Fatalf("frequency boundary selected Output index=%v, want match=%t", got, test.match)
					}
				})
			}
		})
	}
}

func TestEnergyPathDistrictOutputIndexWildcardRespectsFrequency(t *testing.T) {
	hourlyIndex, wildcardIndex, monthlyIndex := 7, 11, 42
	hourly := PurposeOutputObject{
		ObjectType: "Output:Variable", KeyValue: "*", VariableName: "Heat Exchanger Sensible Cooling Rate",
		ReportingFrequency: "Hourly", ObjectIndex: &hourlyIndex, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy},
	}
	monthlyWildcard := hourly
	monthlyWildcard.ReportingFrequency, monthlyWildcard.ObjectIndex = "Monthly", &wildcardIndex
	monthly := monthlyWildcard
	monthly.KeyValue, monthly.ObjectIndex = "DOAS HEAT RECOVERY", &monthlyIndex
	exactHourly := hourly
	exactHourly.KeyValue = monthly.KeyValue
	foreignMonthly := monthly
	foreignMonthly.KeyValue = "OTHER HEAT RECOVERY"
	dictionary := energyExplanationDictionary{
		row:                sqlOutputDictionaryRow{name: monthly.VariableName, keyValue: monthly.KeyValue, reportingFrequency: "Monthly"},
		reportingFrequency: "Monthly",
	}
	for _, test := range []struct {
		name    string
		outputs []PurposeOutputObject
		want    *int
	}{
		{"Hourly wildcard alone is not Monthly evidence", []PurposeOutputObject{hourly}, nil},
		{"earlier Hourly wildcard cannot shadow exact Monthly", []PurposeOutputObject{hourly, monthly}, &monthlyIndex},
		{"later Hourly wildcard cannot replace exact Monthly", []PurposeOutputObject{monthly, hourly}, &monthlyIndex},
		{"matching Monthly wildcard remains usable", []PurposeOutputObject{monthlyWildcard}, &wildcardIndex},
		{"exact Hourly cannot override matching Monthly wildcard", []PurposeOutputObject{monthlyWildcard, exactHourly}, &wildcardIndex},
		{"foreign Monthly key cannot heal Hourly wildcard", []PurposeOutputObject{foreignMonthly, hourly}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := energyExplanationObjectIndexForDictionary(dictionary, &PurposeRunPlan{OutputObjects: test.outputs})
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("wildcard source index=%v, want %v (Hourly=7, Monthly wildcard=11, exact Monthly=42)", got, test.want)
			}
		})
	}
}

func epathDistrictPurposeOriginal(t *testing.T) idf.Document {
	t.Helper()
	path := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "5ZoneFanCoilDOAS_ERVOnAirLoopMainBranch.idf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "3b7712676a8ea7cb034cec7eb0b843731d12ba79cf72d3d54f3b3e6481b765da" {
		t.Fatalf("original District model bytes changed: %s", got)
	}
	t.Cleanup(func() {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, after) {
			t.Error("read-only original District model changed")
		}
	})
	return parsePurposePlanFixture(t, string(data))
}

func epathDistrictPurposePlan(doc idf.Document, scope SimulationPurposeScope) PurposeRunPlan {
	return BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare, Scope: scope,
	})
}

func epathDistrictPurposeOutputs(plan PurposeRunPlan, objectType, key, name, frequency string) []PurposeOutputObject {
	out := []PurposeOutputObject{}
	for _, item := range plan.OutputObjects {
		if strings.EqualFold(item.ObjectType, objectType) && strings.EqualFold(item.KeyValue, key) &&
			strings.EqualFold(item.VariableName, name) && strings.EqualFold(item.ReportingFrequency, frequency) {
			out = append(out, item)
		}
	}
	return out
}

func epathDistrictPurposeManualHourly(doc idf.Document) map[int][]string {
	out := map[int][]string{}
	for _, object := range doc.Objects {
		if !strings.EqualFold(object.Type, "Output:Variable") || len(object.Fields) < 3 || !strings.EqualFold(object.Fields[2].Value, "Hourly") {
			continue
		}
		fields := make([]string, len(object.Fields))
		for i, field := range object.Fields {
			fields[i] = field.Value
		}
		out[object.Index] = fields
	}
	return out
}
