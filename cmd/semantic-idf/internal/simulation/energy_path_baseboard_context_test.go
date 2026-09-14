package simulation

import (
	"reflect"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func energyPathBaseboardTestRequest(name, key, frequency, zone string, index int) PurposeOutputObject {
	output := PurposeOutputObject{ObjectType: "Output:Variable", VariableName: name, KeyValue: key, ReportingFrequency: frequency, ScopeZoneName: zone, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}}
	if index >= 0 {
		output.ObjectIndex = &index
	}
	return output
}

func TestEnergyPathBaseboardContextRequiresExactNativeOwnerNameUnitAndMonthlyIntent(t *testing.T) {
	targets := energyPathBaseboardTargets(energyPathBaseboardHand(t))
	if len(targets) != 1 {
		t.Fatalf("hand target invalid: %+v", targets)
	}
	for _, name := range []string{"Baseboard Total Heating Energy", "Baseboard Total Heating Rate", "Baseboard Electricity Rate"} {
		unit := "W"
		if name == "Baseboard Total Heating Energy" {
			unit = "J"
		}
		for _, frequency := range []string{"Monthly", "Hourly"} {
			t.Run(name+"/"+frequency, func(t *testing.T) {
				dictionary := energyExplanationDictionary{row: sqlOutputDictionaryRow{keyValue: "Baseboard", name: name, units: unit}, reportingFrequency: frequency}
				plan := PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, OutputObjects: []PurposeOutputObject{energyPathBaseboardTestRequest(name, "Baseboard", "Monthly", "Office", 42)}}
				binding, ok := energyPathBaseboardContextDictionaryScope(dictionary, &plan, targets)
				if !ok || binding.Target.ZoneName != "Office" || binding.Name != name || binding.Units != unit || binding.IntegratesRate != (unit == "W") {
					t.Fatalf("native context binding lost: %+v valid=%t", binding, ok)
				}
				for _, tc := range []struct {
					name   string
					mutate func(*energyExplanationDictionary, *PurposeRunPlan, *[]energyPathBaseboardTarget)
				}{
					{"meter", func(d *energyExplanationDictionary, p *PurposeRunPlan, ts *[]energyPathBaseboardTarget) {
						d.isMeter = true
					}},
					{"wrong native unit", func(d *energyExplanationDictionary, p *PurposeRunPlan, ts *[]energyPathBaseboardTarget) {
						d.row.units = "kWh"
					}},
					{"unsupported frequency", func(d *energyExplanationDictionary, p *PurposeRunPlan, ts *[]energyPathBaseboardTarget) {
						d.reportingFrequency = "RunPeriod"
					}},
					{"wildcard dictionary", func(d *energyExplanationDictionary, p *PurposeRunPlan, ts *[]energyPathBaseboardTarget) {
						d.row.keyValue = "*"
					}},
					{"fake Zone alias", func(d *energyExplanationDictionary, p *PurposeRunPlan, ts *[]energyPathBaseboardTarget) {
						d.row.name = "Zone " + name
					}},
					{"electric consumption is not context", func(d *energyExplanationDictionary, p *PurposeRunPlan, ts *[]energyPathBaseboardTarget) {
						d.row.name = "Baseboard Electricity Energy"
						d.row.units = "J"
					}},
					{"owner missing", func(d *energyExplanationDictionary, p *PurposeRunPlan, ts *[]energyPathBaseboardTarget) { *ts = nil }},
					{"owner duplicate", func(d *energyExplanationDictionary, p *PurposeRunPlan, ts *[]energyPathBaseboardTarget) {
						*ts = append(*ts, (*ts)[0])
					}},
					{"no Energy Path", func(d *energyExplanationDictionary, p *PurposeRunPlan, ts *[]energyPathBaseboardTarget) {
						p.BasicEnergyDetail = ""
					}},
					{"Hourly request cannot prove Monthly intent", func(d *energyExplanationDictionary, p *PurposeRunPlan, ts *[]energyPathBaseboardTarget) {
						p.OutputObjects[0].ReportingFrequency = "Hourly"
					}},
					{"contradictory Zone", func(d *energyExplanationDictionary, p *PurposeRunPlan, ts *[]energyPathBaseboardTarget) {
						p.OutputObjects = append(p.OutputObjects, energyPathBaseboardTestRequest(name, "Baseboard", "Monthly", "Other", 43))
					}},
					{"wrong purpose", func(d *energyExplanationDictionary, p *PurposeRunPlan, ts *[]energyPathBaseboardTarget) {
						p.OutputObjects[0].PurposeIDs = []SimulationPurposeID{SimulationPurposeHVACLoopCheck}
					}},
				} {
					t.Run(tc.name, func(t *testing.T) {
						d, p, ts := dictionary, plan, append([]energyPathBaseboardTarget(nil), targets...)
						p.OutputObjects = append([]PurposeOutputObject(nil), plan.OutputObjects...)
						tc.mutate(&d, &p, &ts)
						if got, valid := energyPathBaseboardContextDictionaryScope(d, &p, ts); valid {
							t.Fatalf("invalid source context accepted: %+v", got)
						}
					})
				}
			})
		}
	}
}

func TestEnergyPathBaseboardContextOpenerUsesActualFrequencyAndAllOriginalDuplicates(t *testing.T) {
	target := energyPathBaseboardTargets(energyPathBaseboardHand(t))[0]
	target.OriginalOutputs = nil
	name := "Baseboard Total Heating Energy"
	dictionary := energyExplanationDictionary{row: sqlOutputDictionaryRow{keyValue: "Baseboard", name: name, units: "J"}, reportingFrequency: "Hourly"}
	monthly := energyPathBaseboardTestRequest(name, "Baseboard", "Monthly", "Office", 42)
	hourly := energyPathBaseboardTestRequest(name, "Baseboard", "Hourly", "Office", 7)
	wildcard := energyPathBaseboardTestRequest(name, "*", "Hourly", "", 8)
	for _, tc := range []struct {
		name     string
		outputs  []PurposeOutputObject
		original []energyPathBaseboardOriginalOutput
		want     int
	}{
		{"Monthly never opens Hourly", []PurposeOutputObject{monthly}, nil, -1},
		{"exact Hourly", []PurposeOutputObject{monthly, hourly}, nil, 7},
		{"order independent", []PurposeOutputObject{hourly, monthly}, nil, 7},
		{"unique native wildcard", []PurposeOutputObject{monthly, wildcard}, nil, 8},
		{"exact preferred over wildcard", []PurposeOutputObject{monthly, wildcard, hourly}, nil, 7},
		{"contradictory Hourly Zone", []PurposeOutputObject{monthly, energyPathBaseboardTestRequest(name, "Baseboard", "Hourly", "Other", 7)}, nil, -1},
		{"duplicate exact indices", []PurposeOutputObject{monthly, hourly, energyPathBaseboardTestRequest(name, "Baseboard", "Hourly", "Office", 9)}, nil, -1},
		{"deduplicated plan cannot erase original wildcard", []PurposeOutputObject{monthly, wildcard}, []energyPathBaseboardOriginalOutput{{KeyValue: "*", Name: name, Frequency: "Hourly", ObjectIndex: 8}, {KeyValue: "*", Name: name, Frequency: "Hourly", ObjectIndex: 9}}, -1},
		{"same original identity does not multiply", []PurposeOutputObject{monthly, hourly}, []energyPathBaseboardOriginalOutput{{KeyValue: "Baseboard", Name: name, Frequency: "Hourly", ObjectIndex: 7}}, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := target
			copy.OriginalOutputs = tc.original
			plan := PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, OutputObjects: tc.outputs}
			binding, ok := energyPathBaseboardContextDictionaryScope(dictionary, &plan, []energyPathBaseboardTarget{copy})
			if !ok {
				t.Fatal("trace ambiguity must not discard native observation")
			}
			if tc.want < 0 && binding.ObjectIndex != nil || tc.want >= 0 && (binding.ObjectIndex == nil || *binding.ObjectIndex != tc.want) {
				t.Fatalf("opener=%v want=%d", binding.ObjectIndex, tc.want)
			}
		})
	}
}

func TestEnergyPathBaseboardContextIsNeverAdditionalZoneLoadOrElectricity(t *testing.T) {
	doc := energyPathBaseboardHand(t)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	target := context.BaseboardTargets[0]
	canonical := energyExplanationSeries{Level: "load", Kind: "load.heating", ZoneName: "Office", Unit: "kWh", Total: 11, Monthly: map[int]float64{1: 11}, SourceIDs: []string{"zone-air"}, sourceName: "Zone Air System Sensible Heating Energy"}
	for _, withZoneAir := range []bool{false, true} {
		for _, tc := range []struct {
			name     string
			raw      float64
			observed bool
		}{{"positive", 2.75, true}, {"signed", -1.5, true}, {"native zero", 0, true}, {"all NULL placeholder", 0, false}} {
			t.Run(tc.name+map[bool]string{false: " without ZoneAir", true: " with ZoneAir"}[withZoneAir], func(t *testing.T) {
				var input []energyExplanationSeries
				var sources []EnergyDataSource
				if withZoneAir {
					input = append(input, canonical)
				}
				for _, name := range []string{"Baseboard Total Heating Energy", "Baseboard Total Heating Rate", "Baseboard Electricity Rate"} {
					binding := energyPathBaseboardContextBinding{Target: target, Name: name, IntegratesRate: name != "Baseboard Total Heating Energy"}
					builder := energyExplanationSeriesBuilder{dictionary: energyExplanationDictionary{row: sqlOutputDictionaryRow{name: name, keyValue: "Baseboard"}, reportingFrequency: "Hourly", baseboardContext: &binding}, unit: "kWh", total: 999, monthly: map[int]float64{1: 999}, hourly: map[int]float64{0: tc.raw}, selectedRange: 999, hasSelectedRange: true}
					item := energyPathBaseboardContextSeriesForBuilder(&builder, name)
					if item.Level != "context" || item.Stage != "context" || item.Total != 999 || item.Hourly[0] != tc.raw {
						t.Fatalf("context builder silently reclassified/altered observation: %+v", item)
					}
					input = append(input, item)
					source := EnergyDataSource{ID: name, Name: name, KeyValue: "Baseboard", ReportingFrequency: "Hourly", RawValue: tc.raw, HourlyEnergy: &EnergySourceHourlyEnergy{Unit: "kWh", Basis: "native", Values: []float64{tc.raw}}}
					if tc.observed {
						source.observedValuePresence = energySourceObservedRaw
					}
					sources = append(sources, source)
				}
				out, got, warnings := bindEnergyPathBaseboardContextSeries(input, sources, context)
				want := []energyExplanationSeries{}
				if withZoneAir {
					want = append(want, canonical)
				}
				if !reflect.DeepEqual(out, want) || len(warnings) != 0 {
					t.Fatalf("context became load/fallback or changed canonical values: out=%+v warnings=%+v", out, warnings)
				}
				for _, source := range got {
					if source.ZoneName != "Office" || source.DriverRole != energyDriverSourceRoleContext || source.InspectorSection != energyDriverInspectorSectionContext || source.EffectiveMultiplier != 1 || source.MultiplierApplication != energyMultiplierAlreadyModelTotal || source.RawValue != tc.raw || source.EffectiveValue != tc.raw || source.HourlyEnergy.Values[0] != tc.raw || source.Formula != "" || len(source.InputSourceIDs) != 0 || energyDataSourceValueKnown(source, energySourceObservedEffective) != tc.observed {
						t.Fatalf("native source knownness/zero/sign/multiplier changed: %+v", source)
					}
				}
			})
		}
	}
	item := energyExplanationSeries{Level: "context", sourceName: "Baseboard Total Heating Energy", sourceKeyValue: "Unknown", SourceIDs: []string{"unknown"}, Total: 999}
	out, sources, warnings := bindEnergyPathBaseboardContextSeries([]energyExplanationSeries{item}, []EnergyDataSource{{ID: "unknown", Name: item.sourceName, KeyValue: "Unknown"}}, context)
	if len(out) != 0 || len(warnings) != 1 || sources[0].ZoneName != "" || energyDataSourceValueKnown(sources[0], energySourceObservedRaw) || energyDataSourceValueKnown(sources[0], energySourceObservedEffective) {
		t.Fatal("unknown owner/NULL became an additive fallback or fabricated observation")
	}
}

func TestEnergyPathBaseboardElectricityUsesExistingDirectCohortWithoutParentExpansion(t *testing.T) {
	target := energyPathBaseboardTargets(energyPathBaseboardHand(t))[0]
	second := target
	second.KeyValue = "Second Baseboard"
	direct := energyPathBaseboardDirectTargets([]energyPathBaseboardTarget{target, second})
	plan := PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, OutputObjects: []PurposeOutputObject{energyPathBaseboardTestRequest("Baseboard Electricity Energy", "Baseboard", "Monthly", "Office", 42)}}
	dictionary := energyExplanationDictionary{row: sqlOutputDictionaryRow{keyValue: "Baseboard", name: "Baseboard Electricity Energy", units: "J"}, reportingFrequency: "Monthly"}
	definition, owner, index, ok := energyPathDirectHVACComponentDictionaryScope(dictionary, &plan, direct)
	if !ok || definition.ID != "heating.baseboard.electricity" || owner != "Office" || index == nil || *index != 42 {
		t.Fatalf("native direct electricity contract not reused: definition=%+v owner=%s index=%v valid=%t", definition, owner, index, ok)
	}
	first := energyExplanationSeries{Level: "energy", Kind: "energy.heating", EndUse: "heating", Carrier: "electricity", ZoneName: "Office", Total: 4, Monthly: map[int]float64{1: 4}, directComponentID: definition.ID, sourceKeyValue: "Baseboard", sourceName: "Baseboard Electricity Energy", sourceFrequency: "Monthly"}
	if got := energyPathCompleteDirectHVACComponentSeries([]energyExplanationSeries{first}, direct); len(got) != 0 {
		t.Fatal("one observation stood in for missing same-Zone electric constituent")
	}
	other := first
	other.sourceKeyValue = "Second Baseboard"
	other.Total = 0
	other.Monthly = map[int]float64{1: 0}
	input := []energyExplanationSeries{first, other}
	if got := energyPathCompleteDirectHVACComponentSeries(input, direct); !reflect.DeepEqual(got, input) {
		t.Fatal("complete direct cohort lost native zero or changed values")
	}
	if got := energyPathCompleteDirectHVACComponentSeries(append(input, first), direct); len(got) != 0 {
		t.Fatal("duplicate observation remained a complete cohort")
	}
}
