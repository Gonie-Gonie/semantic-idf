package simulation

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathConvectiveBaseboardContextMonthlyIntentAndActualFrequencyTrace(t *testing.T) {
	target := energyPathBaseboardTargets(energyPathConvectiveBaseboardHand(t))[0]
	for _, name := range []string{"Baseboard Total Heating Energy", "Baseboard Total Heating Rate", "Baseboard Electricity Rate"} {
		unit := "W"
		if name == "Baseboard Total Heating Energy" {
			unit = "J"
		}
		for _, frequency := range []string{"Monthly", "Hourly"} {
			t.Run(name+"/"+frequency, func(t *testing.T) {
				dictionary := energyExplanationDictionary{row: sqlOutputDictionaryRow{keyValue: "Baseboard", name: name, units: unit}, reportingFrequency: frequency}
				plan := PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, OutputObjects: []PurposeOutputObject{
					energyPathBaseboardTestRequest(name, "Baseboard", "Monthly", "Office", 42),
					energyPathBaseboardTestRequest(name, "Baseboard", "Hourly", "Office", 43),
				}}
				binding, ok := energyPathBaseboardContextDictionaryScope(dictionary, &plan, []energyPathBaseboardTarget{target})
				want := 42
				if frequency == "Hourly" {
					want = 43
				}
				if !ok || binding.Target.Component.ObjectType != energyPathBaseboardConvectiveElectricType || binding.Target.ZoneName != "Office" || binding.IntegratesRate != (unit == "W") || binding.ObjectIndex == nil || *binding.ObjectIndex != want {
					t.Fatalf("exact context/frequency binding=%+v", binding)
				}
				for _, mutation := range []string{"unit", "key", "frequency", "Monthly intent", "Zone owner", "purpose", "duplicate owner"} {
					d, p, targets := dictionary, plan, []energyPathBaseboardTarget{target}
					p.OutputObjects = append([]PurposeOutputObject(nil), plan.OutputObjects...)
					switch mutation {
					case "unit":
						d.row.units = "kWh"
					case "key":
						d.row.keyValue = "Office"
					case "frequency":
						d.reportingFrequency = "Daily"
					case "Monthly intent":
						p.OutputObjects = p.OutputObjects[1:]
					case "Zone owner":
						p.OutputObjects[0].ScopeZoneName = "Other"
					case "purpose":
						p.OutputObjects[0].PurposeIDs = []SimulationPurposeID{SimulationPurposeHVACLoopCheck}
					case "duplicate owner":
						targets = append(targets, target)
					}
					if got, valid := energyPathBaseboardContextDictionaryScope(d, &p, targets); valid {
						t.Fatalf("%s accepted %+v", mutation, got)
					}
				}
				if frequency == "Hourly" {
					// An original pair of wildcard outputs remains ambiguous even
					// though the purpose plan keeps just one signature.
					wildcard := energyPathBaseboardTestRequest(name, "*", "Hourly", "", 8)
					plan.OutputObjects[1] = wildcard
					copy := target
					copy.OriginalOutputs = []energyPathBaseboardOriginalOutput{{KeyValue: "*", Name: name, Frequency: "Hourly", ObjectIndex: 8}, {KeyValue: "*", Name: name, Frequency: "Hourly", ObjectIndex: 9}}
					got, valid := energyPathBaseboardContextDictionaryScope(dictionary, &plan, []energyPathBaseboardTarget{copy})
					if !valid || got.ObjectIndex != nil {
						t.Fatalf("duplicate opener was fabricated: %+v", got)
					}
				}
			})
		}
	}
}

func TestEnergyPathConvectiveBaseboardContextNonAdditiveZeroNULLAndModelTotal(t *testing.T) {
	doc, _ := energyPathConvectiveBaseboardOriginal(t)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	for _, target := range context.BaseboardTargets {
		for _, frequency := range []string{"Monthly", "Hourly"} {
			for _, tc := range []struct {
				name  string
				raw   float64
				known bool
			}{{"positive", 19, true}, {"zero", 0, true}, {"NULL", 0, false}} {
				t.Run(target.ZoneName+"/"+frequency+"/"+tc.name, func(t *testing.T) {
					canonical := energyExplanationSeries{Level: "load", Kind: "load.heating", ZoneName: target.ZoneName, Total: 100, Monthly: map[int]float64{1: 100}, SourceIDs: []string{"zone-air"}, sourceName: "Zone Air System Sensible Heating Energy"}
					input := []energyExplanationSeries{canonical}
					var sources []EnergyDataSource
					for _, name := range []string{"Baseboard Total Heating Energy", "Baseboard Total Heating Rate", "Baseboard Electricity Rate"} {
						binding := energyPathBaseboardContextBinding{Target: target, Name: name, IntegratesRate: name != "Baseboard Total Heating Energy"}
						builder := energyExplanationSeriesBuilder{dictionary: energyExplanationDictionary{row: sqlOutputDictionaryRow{name: name, keyValue: target.KeyValue}, reportingFrequency: frequency, baseboardContext: &binding}, unit: "kWh", total: 999, monthly: map[int]float64{1: 999}, hourly: map[int]float64{0: tc.raw}}
						input = append(input, energyPathBaseboardContextSeriesForBuilder(&builder, name))
						source := EnergyDataSource{ID: name, Name: name, KeyValue: target.KeyValue, ReportingFrequency: frequency, RawValue: tc.raw, HourlyEnergy: &EnergySourceHourlyEnergy{Unit: "kWh", Values: []float64{tc.raw}}}
						if tc.known {
							source.observedValuePresence = energySourceObservedRaw
						}
						sources = append(sources, source)
					}
					out, got, warnings := bindEnergyPathBaseboardContextSeries(input, sources, context)
					if !reflect.DeepEqual(out, []energyExplanationSeries{canonical}) || len(warnings) != 0 {
						t.Fatal("native response added/replaced canonical load")
					}
					for _, source := range got {
						component := "load.baseboard_response.convective"
						if source.Name == "Baseboard Electricity Rate" {
							component = "energy.baseboard_electricity_rate"
						}
						if source.DriverComponent != component || source.DriverRole != energyDriverSourceRoleContext || source.InspectorSection != energyDriverInspectorSectionContext || source.ZoneName != target.ZoneName || source.RawValue != tc.raw || source.EffectiveValue != tc.raw || source.EffectiveMultiplier != 1 || source.MultiplierApplication != energyMultiplierAlreadyModelTotal || energyDataSourceValueKnown(source, energySourceObservedRaw) != tc.known || energyDataSourceValueKnown(source, energySourceObservedEffective) != tc.known || source.HourlyEnergy.Values[0] != tc.raw || source.Formula != "" || len(source.InputSourceIDs) != 0 || !reflect.DeepEqual(source.RelatedEntityIDs, []string{target.Component.ID}) {
							t.Fatalf("native context/knownness/multiplier changed: %+v", source)
						}
						if source.Name != "Baseboard Electricity Rate" && (!strings.Contains(source.Explanation, "no radiant recipients") || !strings.Contains(source.Explanation, "non-additive")) {
							t.Fatal("convective response mislabeled radiant")
						}
					}
				})
			}
		}
	}
}

func TestEnergyPathConvectiveBaseboardNeverQualifiesRadiantSurfaceEvenMalformedTarget(t *testing.T) {
	doc := energyPathBaseboardHand(t)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	if len(energyPathBaseboardSurfaceReferences(context, "Floor", "Office")) != 1 {
		t.Fatal("radiant positive control missing")
	}
	context.BaseboardTargets[0].Component.ObjectType = energyPathBaseboardConvectiveElectricType
	// Keep the deliberately contradictory positive radiant fractions and
	// recipient list: exact type, not just recipient presence, is required.
	source := EnergyDataSource{ID: "surface", Name: "Surface Inside Face Convection Heat Gain Energy", KeyValue: "Floor", ZoneName: "Office", SourceType: "sql_report_data", RawValue: 17, Explanation: "original", RelatedEntityIDs: []string{"surface:floor"}}
	item := energyExplanationSeries{Level: "heat", Kind: "heat.surface_inside_face_convection", ZoneName: "Office", SurfaceScoped: true, Total: 17, Monthly: map[int]float64{1: 17}, SourceIDs: []string{"surface"}, sourceKeyValue: "Floor", DriverExplanation: "original"}
	series, sources := []energyExplanationSeries{item}, []EnergyDataSource{source}
	applyEnergyPathBaseboardRecipientQualification(series, sources, context)
	if len(energyPathBaseboardSurfaceReferences(context, "Floor", "Office")) != 0 || !reflect.DeepEqual(series, []energyExplanationSeries{item}) || !reflect.DeepEqual(sources, []EnergyDataSource{source}) {
		t.Fatal("convective device acquired radiation metadata or changed surface quantities")
	}
}

func TestEnergyPathConvectiveBaseboardDirectCohortRejectsMissingAndDuplicateNotZero(t *testing.T) {
	target := energyPathBaseboardTargets(energyPathConvectiveBaseboardHand(t))[0]
	second := target
	second.KeyValue = "Second Baseboard"
	direct := energyPathBaseboardDirectTargets([]energyPathBaseboardTarget{target, second})
	if len(direct) != 2 || direct[0].Definition.ObjectType != energyPathBaseboardConvectiveElectricType {
		t.Fatal("convective source reused wrong typed definition")
	}
	first := energyExplanationSeries{Level: "energy", Kind: "energy.heating", EndUse: "heating", Carrier: "electricity", ZoneName: "Office", Total: 4, Monthly: map[int]float64{1: 4}, directComponentID: "heating.baseboard.electricity", sourceKeyValue: "Baseboard", sourceName: "Baseboard Electricity Energy", sourceFrequency: "Monthly"}
	zero := first
	zero.sourceKeyValue = second.KeyValue
	zero.Total = 0
	zero.Monthly = map[int]float64{1: 0}
	for _, tc := range []struct {
		name     string
		input    []energyExplanationSeries
		complete bool
	}{
		{"complete native zero", []energyExplanationSeries{first, zero}, true},
		{"missing constituent", []energyExplanationSeries{first}, false},
		{"duplicate constituent", []energyExplanationSeries{first, zero, first}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := energyPathCompleteDirectHVACComponentSeries(tc.input, direct)
			if tc.complete && !reflect.DeepEqual(got, tc.input) || !tc.complete && len(got) != 0 {
				t.Fatalf("cohort boundary changed: %s %+v", fmt.Sprint(tc.complete), got)
			}
		})
	}
}
