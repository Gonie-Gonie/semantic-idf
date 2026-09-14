package simulation

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func cogenCoverageReviewSource(id, name, frequency string, value float64, known bool) EnergyDataSource {
	source := EnergyDataSource{ID: id, SourceType: "sql_report_data", IsMeter: true, Name: name, SourceUnit: "J", Units: "J", NormalizedUnit: "kWh", ReportingFrequency: frequency, RawValue: value, EffectiveValue: value}
	if known {
		source.observedValuePresence = energySourceObservedRaw | energySourceObservedEffective
	}
	return source
}

func cogenCoverageReviewRequest(name, frequency string) PurposeOutputObject {
	return PurposeOutputObject{ObjectType: "Output:Meter", KeyValue: name, ReportingFrequency: frequency, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Fields: []idf.OutputFieldValue{{Name: "Key Name", Value: name}, {Name: "Reporting Frequency", Value: frequency}}}
}

func cogenCoverageReviewStatus(t *testing.T, completeness EnergyCompleteness, name, want string) {
	t.Helper()
	count := 0
	for _, entry := range completeness.SourceAvailability {
		if strings.EqualFold(entry.Name, name) {
			count++
			if entry.Status != want {
				t.Fatalf("%s availability=%s want%s", name, entry.Status, want)
			}
		}
	}
	if count != 1 {
		t.Fatalf("%s availability entries=%d want1", name, count)
	}
}

func TestCogenerationCoverageReviewOnlyRequestedFullMonthlyParentCounts(t *testing.T) {
	name := "Cogeneration:Electricity"
	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{cogenCoverageReviewRequest(name, "Monthly")}}
	source := cogenCoverageReviewSource("sql-rdd-61", name, "Monthly", 4, true)
	item := energyExplanationSeries{Level: "energy", Stage: "end_use", Kind: "energy.cogeneration_input", EndUse: "cogeneration_input", Carrier: "electricity", SourceName: name, sourceName: name, sourceFrequency: "Monthly", SourceIDs: []string{source.ID}, Monthly: map[int]float64{1: 4}}
	for _, test := range []struct {
		name   string
		change func(*energyExplanationSeries, *EnergyDataSource, *[]string)
		want   bool
	}{
		{"full_parent", func(*energyExplanationSeries, *EnergyDataSource, *[]string) {}, true},
		{"known_zero", func(i *energyExplanationSeries, s *EnergyDataSource, _ *[]string) {
			s.RawValue = 0
			s.EffectiveValue = 0
			i.Monthly = map[int]float64{1: 0}
		}, true},
		{"no_request", func(_ *energyExplanationSeries, _ *EnergyDataSource, n *[]string) {
			*n = []string{"Electricity:Facility"}
		}, false},
		{"partial_parent", func(_ *energyExplanationSeries, s *EnergyDataSource, _ *[]string) { s.observedValuePresence = 0 }, false},
		{"annual_table", func(i *energyExplanationSeries, s *EnergyDataSource, _ *[]string) {
			i.sourceFrequency = "Annual"
			i.Monthly = nil
			s.SourceType = "sql_tabular"
			s.ReportingFrequency = "Annual"
		}, false},
		{"sole_member", func(i *energyExplanationSeries, s *EnergyDataSource, _ *[]string) {
			i.SourceName = "Inverter Ancillary AC Electricity Energy"
			i.sourceName = i.SourceName
			s.Name = i.SourceName
			s.IsMeter = false
			s.KeyValue = "INV"
		}, false},
		{"wrong_carrier", func(i *energyExplanationSeries, _ *EnergyDataSource, _ *[]string) { i.Carrier = "natural_gas" }, false},
		{"private_name", func(i *energyExplanationSeries, _ *EnergyDataSource, _ *[]string) {
			i.sourceName = "Cogeneration:NaturalGas"
		}, false},
		{"hourly_source", func(_ *energyExplanationSeries, s *EnergyDataSource, _ *[]string) { s.ReportingFrequency = "Hourly" }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			actualItem, actualSource := item, source
			names := []string{name}
			test.change(&actualItem, &actualSource, &names)
			key, handled := energyPathCogenerationCompletenessGroupKey(actualItem, names, []EnergyDataSource{actualSource}, plan)
			if !handled || (key != "") != test.want {
				t.Fatalf("key=%q handled=%v want count=%v", key, handled, test.want)
			}
			if test.want && key != expectedEnergyExplanationOutputGroupKey(name, "energy") {
				t.Fatal("did not count the actual requested group identity")
			}
		})
	}
	if key, handled := energyPathCogenerationCompletenessGroupKey(item, []string{name}, []EnergyDataSource{source, source}, plan); !handled || key != "" {
		t.Fatal("duplicate source roster fulfilled a request")
	}
	legacy := energyExplanationSeries{Level: "energy", Kind: "energy.generators", EndUse: "generators"}
	if _, handled := energyPathCogenerationCompletenessGroupKey(legacy, []string{name}, []EnergyDataSource{source}, plan); handled {
		t.Fatal("legacy found-counting changed")
	}
	if key, _ := energyPathCogenerationCompletenessGroupKey(item, []string{name}, []EnergyDataSource{source}, nil); key != "" {
		t.Fatal("missing actual request plan was inferred")
	}
	plan.OutputObjects = append(plan.OutputObjects, cogenCoverageReviewRequest(name, "Hourly"))
	if key, _ := energyPathCogenerationCompletenessGroupKey(item, []string{name}, []EnergyDataSource{source}, plan); key != "" {
		t.Fatal("Monthly observation filled missing requested Hourly coverage")
	}
	hourly := cogenCoverageReviewSource("sql-rdd-62", name, "Hourly", 4, true)
	if key, _ := energyPathCogenerationCompletenessGroupKey(item, []string{name}, []EnergyDataSource{source, hourly}, plan); key == "" {
		t.Fatal("complete requested M/H parent was not counted")
	}
}

func TestCogenerationCoverageReviewUnrelatedTableZeroCannotFillMissingFacility(t *testing.T) {
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, OutputObjects: []PurposeOutputObject{cogenCoverageReviewRequest("Electricity:Facility", "Monthly")}}
	table := energyExplanationSeries{Level: "energy", Stage: "end_use", Kind: "energy.cogeneration_input", EndUse: "cogeneration_input", Carrier: "natural_gas", sourceName: "Cogeneration:NaturalGas", sourceFrequency: "Annual", SourceIDs: []string{"sql-tabular-cogeneration-101"}}
	sources := []EnergyDataSource{{ID: "sql-tabular-cogeneration-101", SourceType: "sql_tabular", Name: "Generators", ReportingFrequency: "Annual", SourceUnit: "kWh", NormalizedUnit: "kWh", observedValuePresence: energySourceObservedRaw | energySourceObservedEffective}}
	before := append([]EnergyDataSource(nil), sources...)
	got := buildEnergyExplanationCompleteness([]energyExplanationSeries{table}, sources, plan, 0)
	if got.EnergyUse.Found != 0 || got.EnergyUse.Total != 1 || got.EnergyUse.Status != "missing" {
		t.Fatalf("unrelated native zero masked missing requested Facility: %+v", got.EnergyUse)
	}
	cogenCoverageReviewStatus(t, got, "Electricity:Facility", "missing")
	if !reflect.DeepEqual(before, sources) || !energyDataSourceValueKnown(sources[0], energySourceObservedRaw) {
		t.Fatal("coverage fix destroyed native zero observation")
	}
}

func TestCogenerationCoverageReviewAvailabilityIsExactFrequencyObservation(t *testing.T) {
	name := "Cogeneration:Electricity"
	monthly := cogenCoverageReviewSource("sql-rdd-61", name, "Monthly", 0, true)
	hourly := cogenCoverageReviewSource("sql-rdd-62", name, "Hourly", 4, true)
	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{cogenCoverageReviewRequest(name, "Monthly"), cogenCoverageReviewRequest(name, "Hourly")}}
	for _, test := range []struct {
		name    string
		sources []EnergyDataSource
		want    string
	}{
		{"zero_and_positive", []EnergyDataSource{monthly, hourly}, "found"},
		{"missing_hourly", []EnergyDataSource{monthly}, "missing"},
		{"missing_monthly", []EnergyDataSource{hourly}, "missing"},
		{"unknown_monthly", []EnergyDataSource{cogenCoverageReviewSource("sql-rdd-61", name, "Monthly", 0, false), hourly}, "missing"},
		{"partial_stale_number", []EnergyDataSource{cogenCoverageReviewSource("sql-rdd-61", name, "Monthly", 4, false), hourly}, "missing"},
		{"duplicate", []EnergyDataSource{monthly, cogenCoverageReviewSource("sql-rdd-63", name, "Monthly", 0, true), hourly}, "missing"},
		{"known_signed_measurement", []EnergyDataSource{cogenCoverageReviewSource("sql-rdd-61", name, "Monthly", -1, true), hourly}, "found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := EnergyCompleteness{SourceAvailability: []EnergySourceAvailabilityEntry{{Name: name, Level: "energy", Status: "found"}, {Name: "Unrelated", Level: "context", Status: "found", SourceIDs: []string{"untouched"}}}}
			evidence := energyPathCogenerationReadResult{Enabled: true, SourceSnapshots: test.sources}
			applyEnergyPathCogenerationAvailability(&got, plan, evidence)
			cogenCoverageReviewStatus(t, got, name, test.want)
			if !reflect.DeepEqual(got.SourceAvailability[1], EnergySourceAvailabilityEntry{Name: "Unrelated", Level: "context", Status: "found", SourceIDs: []string{"untouched"}}) {
				t.Fatal("unrelated availability changed")
			}
			before := got
			applyEnergyPathCogenerationAvailability(&got, plan, evidence)
			if !reflect.DeepEqual(got, before) {
				t.Fatal("availability is not idempotent")
			}
		})
	}
	got := EnergyCompleteness{SourceAvailability: []EnergySourceAvailabilityEntry{{Name: name, Status: "found"}}}
	before := got
	applyEnergyPathCogenerationAvailability(&got, plan, energyPathCogenerationReadResult{})
	if !reflect.DeepEqual(got, before) {
		t.Fatal("omitted-original compatibility changed")
	}
	monthlyOnly := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{cogenCoverageReviewRequest(name, "Monthly")}}
	applyEnergyPathCogenerationAvailability(&got, monthlyOnly, energyPathCogenerationReadResult{Enabled: true, SourceSnapshots: []EnergyDataSource{monthly}})
	cogenCoverageReviewStatus(t, got, name, "found")
	wildcardPlan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{cogenCoverageReviewRequest("Cogeneration:*", "Monthly")}}
	wildcard := EnergyCompleteness{SourceAvailability: []EnergySourceAvailabilityEntry{{Name: "Cogeneration:*", Status: "found"}}}
	applyEnergyPathCogenerationAvailability(&wildcard, wildcardPlan, energyPathCogenerationReadResult{Enabled: true, SourceSnapshots: []EnergyDataSource{monthly}})
	cogenCoverageReviewStatus(t, wildcard, "Cogeneration:*", "missing") // Explicit finite-roster limitation, not guessed expansion.
}

func TestCogenerationCoverageReviewActualSQLUnknownAndCollisionSurviveTwoJSONs(t *testing.T) {
	for _, test := range []struct{ name, parent, extra, want string }{{"observed_zero", "zero", "", "found"}, {"all_NULL", "all_null", "", "missing"}, {"no_rows", "no_rows", "", "missing"}, {"partial", "partial", "", "missing"}, {"custom_known", "positive", "Meter:Custom,Cogeneration:Electricity,Electricity;", "found"}} {
		t.Run(test.name, func(t *testing.T) {
			fixture := epathCogenPipelineSQL(t, epathCogenPipelineSpec{Parent: test.parent, Member: "positive", ExtraOriginal: test.extra})
			plan, _ := epathCogenPipelinePlan(fixture, PurposeAllocationPolicyDirectOnly)
			outputs := []PurposeOutputObject{}
			for _, output := range plan.OutputObjects {
				if strings.EqualFold(output.KeyValue, "Cogeneration:Electricity") && !strings.EqualFold(output.ReportingFrequency, "Monthly") {
					continue
				}
				outputs = append(outputs, output)
			}
			plan.OutputObjects = outputs // Literal SQL intentionally reports only Monthly parent.
			context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(fixture.Document), fixture.Document)
			legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(fixture.Files[0].Path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			for pass := 0; pass < 3; pass++ {
				cogenCoverageReviewStatus(t, legacy.Completeness, "Cogeneration:Electricity", test.want)
				if pass == 2 {
					break
				}
				raw, err := json.Marshal(legacy)
				if err != nil {
					t.Fatal(err)
				}
				var next EnergyExplanationV1
				if err = json.Unmarshal(raw, &next); err != nil {
					t.Fatal(err)
				}
				legacy = next
			}
			result := UpgradeEnergyExplanationV1(legacy)
			for pass := 0; pass < 3; pass++ {
				cogenCoverageReviewStatus(t, result.Completeness, "Cogeneration:Electricity", test.want)
				if pass == 2 {
					break
				}
				raw, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				var next EnergyExplanationResult
				if err = json.Unmarshal(raw, &next); err != nil {
					t.Fatal(err)
				}
				result = next
			}
		})
	}
}

func TestCogenerationCoverageReviewActualUnrelatedTABZeroDoesNotCompleteMissingFacility(t *testing.T) {
	fixture := epathCogenPipelineSQL(t, epathCogenPipelineSpec{Parent: "absent", Member: "absent", TableMode: "zero", TableColumn: "Natural Gas"})
	db, err := sql.Open("sqlite", fixture.Files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	// Only this fresh literal t.TempDir file. Leave the native annual zero and
	// Time axis intact but remove every requested carrier/end-use observation.
	if _, err = db.Exec(`DELETE FROM ReportData; DELETE FROM ReportDataDictionary`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, OutputObjects: []PurposeOutputObject{cogenCoverageReviewRequest("Electricity:Facility", "Monthly")}}
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(fixture.Document), fixture.Document)
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(fixture.Files[0].Path, plan, context)
	if err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 3; pass++ {
		if legacy.Completeness.EnergyUse.Found != 0 || legacy.Completeness.EnergyUse.Total != 1 || legacy.Completeness.EnergyUse.Status != "missing" {
			t.Fatalf("TAB zero falsely completed requested Facility: %+v", legacy.Completeness.EnergyUse)
		}
		source := energyExplanationSourceByID(legacy.Sources, "sql-tabular-cogeneration-101")
		if source == nil || source.RawValue != 0 || !energyDataSourceValueKnown(*source, energySourceObservedRaw) {
			t.Fatal("native known-zero source was removed to fix counts")
		}
		if pass == 2 {
			break
		}
		raw, err := json.Marshal(legacy)
		if err != nil {
			t.Fatal(err)
		}
		var next EnergyExplanationV1
		if err = json.Unmarshal(raw, &next); err != nil {
			t.Fatal(err)
		}
		legacy = next
	}
	result := UpgradeEnergyExplanationV1(legacy)
	for pass := 0; pass < 3; pass++ {
		if result.Completeness.EnergyUse.Found != 0 || result.Completeness.EnergyUse.Total != 1 || result.Completeness.EnergyUse.Status != "missing" {
			t.Fatalf("v2/reload counted unrelated TAB zero: %+v", result.Completeness.EnergyUse)
		}
		cogenCoverageReviewStatus(t, result.Completeness, "Electricity:Facility", "missing")
		source := energyExplanationSourceByID(result.Sources, "sql-tabular-cogeneration-101")
		if source == nil || source.RawValue != 0 || !energyDataSourceValueKnown(*source, energySourceObservedRaw) {
			t.Fatal("v2/reload lost native observed zero")
		}
		if pass == 2 {
			break
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var next EnergyExplanationResult
		if err = json.Unmarshal(raw, &next); err != nil {
			t.Fatal(err)
		}
		result = next
	}
}
