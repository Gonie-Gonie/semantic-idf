package simulation

// Independent literal regressions for the native-series rebuild.
// No candidate/accepted quantities, engine, or production-derived expectation.
// Root owns installation and Go. Private-name conflict is an intentional
// negative regression for the read-only review finding, not a relaxed check.
import (
	"crypto/sha256"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func pvNativeRebuildReviewNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > 1e-12 {
		t.Fatalf("got %.15g want %.15g", got, want)
	}
}

func pvNativeRebuildReviewMap(t *testing.T, got, want map[int]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unexpected native cells: got=%v want=%v", got, want)
	}
	for key, wantValue := range want {
		gotValue, present := got[key]
		if !present {
			t.Fatalf("missing native cell %d (including known zero)", key)
		}
		pvNativeRebuildReviewNear(t, gotValue, wantValue)
	}
}

func pvNativeRebuildReviewItem(name, key, frequency, id string) energyExplanationSeries {
	return energyExplanationSeries{
		Level: "energy", Kind: "energy.electricity.total", Stage: "carrier", Unit: "kWh", Carrier: "electricity",
		SourceName: name, sourceName: name, SourceKey: key, sourceKeyValue: key, sourceFrequency: frequency, SourceIDs: []string{id},
		Total: -999, RawTotal: -999, Monthly: map[int]float64{8: -999}, RawMonthly: map[int]float64{8: -999},
		Daily: map[int]float64{200: -999}, RawDaily: map[int]float64{200: -999}, Hourly: map[int]float64{4800: -999}, RawHourly: map[int]float64{4800: -999},
		SelectedRange: -999, RawSelectedRange: -999, HasSelectedRange: true,
		AnnualSourceIDs: []string{"stale-annual"}, MonthlySourceIDs: []string{"stale-month"}, DailySourceIDs: []string{"stale-day"}, HourlySourceIDs: []string{"stale-hour"}, SelectedRangeSourceIDs: []string{"stale-selection"},
	}
}

func pvNativeRebuildReviewEvidence(name, key, frequency, id string, total float64, monthly map[int]float64, hourly []energyPathPVElectricalHourlyValue) energyPathPVElectricalEvidence {
	isMeter := key == ""
	source := EnergyDataSource{ID: id, Name: name, KeyValue: key, ReportingFrequency: frequency, IsMeter: isMeter, SourceUnit: "J", NormalizedUnit: "kWh", RawValue: total, EffectiveValue: total, observedValuePresence: energySourceObservedRaw | energySourceObservedEffective}
	return energyPathPVElectricalEvidence{
		Inventory: energyPathPVElectricalInventory{HasOriginal: true, SchemaReviewed: true}, SourceSnapshots: []EnergyDataSource{source},
		Observations: []energyPathPVElectricalObservation{{Intent: energyPathPVElectricalIntent{Target: energyPathPVElectricalTarget{Definition: energyPathPVElectricalDefinition{Name: name, IsMeter: isMeter}, Key: key, IdentityValid: true}, Frequency: frequency}, Status: "observed", SourceIDs: []string{id}, MonthlyValues: monthly, HourlyValues: hourly}},
	}
}

func TestPVNativeRebuildReviewMonthlyCannotInventSelectedDays(t *testing.T) {
	input := pvNativeRebuildReviewItem("Electricity:Facility", "", "Monthly", "sql-rdd-501")
	evidence := pvNativeRebuildReviewEvidence("Electricity:Facility", "", "Monthly", "sql-rdd-501", 5.125, map[int]float64{1: 5.125, 2: 0}, nil)
	for _, plan := range []*PurposeRunPlan{nil, {PeriodMode: "custom", PeriodStart: "01-02", PeriodEnd: "01-02"}, {PeriodMode: "custom", PeriodStart: "01-01", PeriodEnd: "12-31"}} {
		kept, warnings := filterEnergyPathPVElectricalSeries([]energyExplanationSeries{input}, evidence, plan)
		if len(kept) != 1 || len(warnings) != 0 {
			t.Fatalf("valid monthly source lost: %v %+v", kept, warnings)
		}
		got := kept[0]
		pvNativeRebuildReviewNear(t, got.Total, 5.125)
		pvNativeRebuildReviewNear(t, got.RawTotal, 5.125)
		pvNativeRebuildReviewMap(t, got.Monthly, map[int]float64{1: 5.125, 2: 0})
		pvNativeRebuildReviewMap(t, got.RawMonthly, map[int]float64{1: 5.125, 2: 0})
		if got.Daily != nil || got.RawDaily != nil || got.Hourly != nil || got.RawHourly != nil || got.SelectedRange != 0 || got.RawSelectedRange != 0 || got.HasSelectedRange {
			t.Fatalf("monthly cell invented selected-day/hour values: %+v", got)
		}
		if len(got.AnnualSourceIDs)+len(got.MonthlySourceIDs)+len(got.DailySourceIDs)+len(got.HourlySourceIDs)+len(got.SelectedRangeSourceIDs) != 0 {
			t.Fatal("stale period source IDs survived rebuild")
		}
		got.Monthly[1] = 77
		if got.RawMonthly[1] != 5.125 || evidence.Observations[0].MonthlyValues[1] != 5.125 || input.Monthly[8] != -999 {
			t.Fatal("native/raw/input maps alias")
		}
	}
}

func TestPVNativeRebuildReviewHourlySelectedDaysUseUnroundedNativeCells(t *testing.T) {
	input := pvNativeRebuildReviewItem("Electricity:Facility", "", "Hourly", "sql-rdd-502")
	cells := []energyPathPVElectricalHourlyValue{{Month: 1, Day: 1, Hour: 24, Value: 1.0004}, {Month: 1, Day: 2, Hour: 1, Value: 2.0004}, {Month: 1, Day: 2, Hour: 24, Value: 0}, {Month: 1, Day: 3, Hour: 1, Value: 0}, {Month: 2, Day: 1, Hour: 1, Value: 4.0004}, {Month: 12, Day: 31, Hour: 24, Value: .5}}
	evidence := pvNativeRebuildReviewEvidence("Electricity:Facility", "", "Hourly", "sql-rdd-502", 7.5012, nil, cells)
	// Deliberately inconsistent rounded chart: it is not an arithmetic input.
	evidence.SourceSnapshots[0].HourlyEnergy = &EnergySourceHourlyEnergy{Unit: "kWh", Values: []float64{1, 2, 0, 0, 4, .5}}
	for _, test := range []struct {
		name, start, end string
		known            bool
		value            float64
	}{{"one_day", "01-02", "01-02", true, 2.0004}, {"known_zero_day", "01-03", "01-03", true, 0}, {"wrap_year", "12-31", "01-01", true, 1.5004}, {"outside_observation", "03-01", "03-01", false, 0}} {
		t.Run(test.name, func(t *testing.T) {
			plan := &PurposeRunPlan{PeriodMode: "custom", PeriodStart: test.start, PeriodEnd: test.end}
			kept, warnings := filterEnergyPathPVElectricalSeries([]energyExplanationSeries{input}, evidence, plan)
			if len(kept) != 1 || len(warnings) != 0 {
				t.Fatalf("valid hourly source lost: %v %+v", kept, warnings)
			}
			got := kept[0]
			pvNativeRebuildReviewNear(t, got.Total, 7.5012)
			pvNativeRebuildReviewNear(t, got.RawTotal, 7.5012)
			pvNativeRebuildReviewMap(t, got.Monthly, map[int]float64{1: 3.0008, 2: 4.0004, 12: .5})
			pvNativeRebuildReviewMap(t, got.Daily, map[int]float64{1: 1.0004, 2: 2.0004, 3: 0, 32: 4.0004, 365: .5})
			pvNativeRebuildReviewMap(t, got.Hourly, map[int]float64{24: 1.0004, 25: 2.0004, 48: 0, 49: 0, 745: 4.0004, 8760: .5})
			pvNativeRebuildReviewMap(t, got.RawMonthly, got.Monthly)
			pvNativeRebuildReviewMap(t, got.RawDaily, got.Daily)
			pvNativeRebuildReviewMap(t, got.RawHourly, got.Hourly)
			if got.HasSelectedRange != test.known {
				t.Fatalf("selected zero/absence confused: %+v", got)
			}
			pvNativeRebuildReviewNear(t, got.SelectedRange, test.value)
			pvNativeRebuildReviewNear(t, got.RawSelectedRange, test.value)
			got.Hourly[24] = 99
			if got.RawHourly[24] != 1.0004 || evidence.Observations[0].HourlyValues[0].Value != 1.0004 {
				t.Fatal("native/raw maps aliased")
			}
		})
	}
}

func TestPVNativeRebuildReviewIdentityCannotHideConflictingPrivateNameOrKey(t *testing.T) {
	evidence := pvNativeRebuildReviewEvidence("Electric Storage Charge Energy", "BATTERY", "Monthly", "sql-rdd-510", 7, map[int]float64{1: 7, 2: 0}, nil)
	for _, test := range []struct {
		name   string
		mutate func(*energyExplanationSeries)
	}{
		{"private_name", func(item *energyExplanationSeries) { item.sourceName = "Heating:Electricity" }},
		{"public_name", func(item *energyExplanationSeries) { item.SourceName = "Heating:Electricity" }},
		{"private_key", func(item *energyExplanationSeries) { item.sourceKeyValue = "FOREIGN" }},
		{"public_key", func(item *energyExplanationSeries) { item.SourceKey = "FOREIGN" }},
		{"mixed_ids", func(item *energyExplanationSeries) { item.SourceIDs = append(item.SourceIDs, "sql-rdd-foreign") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := pvNativeRebuildReviewItem("Electric Storage Charge Energy", "BATTERY", "Monthly", "sql-rdd-510")
			test.mutate(&item)
			if kept, _ := filterEnergyPathPVElectricalSeries([]energyExplanationSeries{item}, evidence); len(kept) != 0 {
				t.Fatalf("conflicting identity gained native budget: %+v", kept)
			}
		})
	}
}

func TestPVNativeRebuildReviewActualSQLSelectedDayIgnoresDesignWarmup(t *testing.T) {
	literals := pvElectricalReaderLiterals()
	literals[17].value = 4
	path := pvElectricalReaderCreateSQLWithLiterals(t, literals,
		`UPDATE ReportData SET Value=-999999999999 WHERE TimeIndex IN (1,2)`,
		`UPDATE Time SET Month=1,Day=CASE TimeIndex WHEN 201 THEN 1 WHEN 202 THEN 2 ELSE 3 END,Hour=CASE TimeIndex WHEN 201 THEN 24 ELSE 1 END WHERE TimeIndex IN (201,202,203)`,
		`UPDATE ReportData SET Value=CASE TimeIndex WHEN 201 THEN 3601440 WHEN 202 THEN 7201440 WHEN 203 THEN 0 ELSE Value END WHERE ReportDataDictionaryIndex=933`)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := pvElectricalReaderDocument(t)
	plan := pvElectricalReaderPlan(doc)
	plan.PeriodMode, plan.PeriodStart, plan.PeriodEnd = "custom", "01-02", "01-02"
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	var monthly, hourly *energyExplanationSeries
	for index := range parsed.Series {
		item := &parsed.Series[index]
		if item.SourceName != "Electricity:Facility" {
			continue
		}
		switch item.sourceFrequency {
		case "Monthly":
			monthly = item
		case "Hourly":
			hourly = item
		}
	}
	if monthly == nil || hourly == nil {
		t.Fatalf("native meter lost from canonical pipeline: M=%v H=%v", monthly, hourly)
	}
	pvNativeRebuildReviewNear(t, monthly.Total, 9)
	if monthly.HasSelectedRange || monthly.Daily != nil || monthly.Hourly != nil {
		t.Fatal("Monthly cell became a selected-day estimate")
	}
	pvNativeRebuildReviewNear(t, hourly.Total, 3.0008)
	pvNativeRebuildReviewNear(t, hourly.SelectedRange, 2.0004)
	if !hourly.HasSelectedRange {
		t.Fatal("observed selected day lost")
	}
	pvNativeRebuildReviewMap(t, hourly.Daily, map[int]float64{1: 1.0004, 2: 2.0004, 3: 0})
	pvNativeRebuildReviewMap(t, hourly.Hourly, map[int]float64{24: 1.0004, 25: 2.0004, 49: 0})
	var observed *energyPathPVElectricalObservation
	for index := range parsed.PVElectricalEvidence.Observations {
		o := &parsed.PVElectricalEvidence.Observations[index]
		if o.Intent.Target.Definition.Name == "Electricity:Facility" && o.Intent.Frequency == "Hourly" {
			observed = o
		}
	}
	if observed == nil || observed.Status != "observed" || observed.HasNegativeValue || observed.ExcludedDesignOrWarmupRows != 2 || len(observed.HourlyValues) != 3 {
		t.Fatalf("native weather scope contaminated: %+v", observed)
	}
	pvNativeRebuildReviewNear(t, observed.HourlyValues[0].Value, 1.0004)
	pvNativeRebuildReviewNear(t, observed.HourlyValues[1].Value, 2.0004)
	producedCount := 0
	for _, item := range parsed.Series {
		if item.SourceName == "ElectricityProduced:Facility" {
			producedCount++
			pvNativeRebuildReviewNear(t, item.Total, 4)
		}
	}
	if producedCount != 2 {
		t.Fatalf("negative excluded rows suppressed valid M/H production: %d", producedCount)
	}
	// Canonical preparation assigns per-period IDs after native maps are rebuilt.
	selected := preferredEnergyExplanationSeries([]energyExplanationSeries{canonicalEnergyExplanationSeries(*monthly), canonicalEnergyExplanationSeries(*hourly)})
	if len(selected) != 1 {
		t.Fatalf("M/H duplicated meter: %+v", selected)
	}
	pvNativeRebuildReviewNear(t, selected[0].Total, 9)
	pvNativeRebuildReviewNear(t, selected[0].SelectedRange, 2.0004)
	if !reflect.DeepEqual(selected[0].AnnualSourceIDs, []string{"sql-rdd-932"}) || !reflect.DeepEqual(selected[0].MonthlySourceIDs, []string{"sql-rdd-932"}) || !reflect.DeepEqual(selected[0].SelectedRangeSourceIDs, []string{"sql-rdd-933"}) {
		t.Fatalf("period authority mixed: %+v", selected[0])
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("read path changed literal SQL")
	}
}

func TestPVNativeRebuildReviewLegacyMarkerOnlyForActuallyRemovedComponents(t *testing.T) {
	evidence := pvNativeRebuildReviewEvidence("Electric Storage Discharge Energy", "BATTERY", "Monthly", "sql-rdd-524", 8, map[int]float64{1: 8}, nil)
	before := []energyExplanationSeries{
		pvNativeRebuildReviewItem("Electric Storage Discharge Energy", "BATTERY", "Timestep", "sql-rdd-601"),
		pvNativeRebuildReviewItem("Electric Storage Charge Energy", "BATTERY", "Timestep", "sql-rdd-602"),
		pvNativeRebuildReviewItem("ElectricityProduced:Facility", "", "Timestep", "sql-rdd-603"),
		pvNativeRebuildReviewItem("Electric Storage Discharge Energy", "BATTERY", "Monthly", "sql-rdd-524"),
		pvNativeRebuildReviewItem("InteriorLights:Electricity", "", "Timestep", "sql-rdd-604"),
	}
	after, _ := filterEnergyPathPVElectricalSeries(before, evidence)
	removed := energyPathPVElectricalRemovedLegacyComponentSourceIDs(before, after, evidence)
	if !reflect.DeepEqual(removed, []string{"sql-rdd-601"}) {
		t.Fatalf("not the exact removed non-M/H component roster: %v", removed)
	}
	sources := []EnergyDataSource{{ID: "sql-rdd-601", RawValue: 6, AggregationBasis: "old", observedValuePresence: energySourceObservedRaw}, {ID: "sql-rdd-602", RawValue: 6}, {ID: "sql-rdd-603", RawValue: 20}, {ID: "sql-rdd-604", RawValue: 5}, {ID: "same-name-unused", Name: "Electric Storage Discharge Energy", RawValue: 99}}
	protected := protectEnergyPathPVLegacyContextSources(sources, removed)
	for index, source := range protected {
		if energyPathPVSourceObservationProtected(source) != (index == 0) {
			t.Fatalf("source was marked by name/context/absence instead of actual removal: %+v", source)
		}
	}
	if protected[0].EffectiveMultiplier != 0 || protected[0].AggregationBasis != "old" || protected[0].RawValue != 6 || energyDataSourceValueKnown(protected[0], energySourceObservedEffective) {
		t.Fatal("legacy protection invented native basis/knownness")
	}
	if got := energyPathPVElectricalRemovedLegacyComponentSourceIDs(before, after, energyPathPVElectricalEvidence{}); got != nil {
		t.Fatal("no-original source gained marker permission")
	}
	// Retention anywhere denies the removal claim for that actual source ID.
	keptAlso := append(append([]energyExplanationSeries(nil), after...), before[0])
	if got := energyPathPVElectricalRemovedLegacyComponentSourceIDs(before, keptAlso, evidence); len(got) != 0 {
		t.Fatal("retained source was marked as removed")
	}
}
