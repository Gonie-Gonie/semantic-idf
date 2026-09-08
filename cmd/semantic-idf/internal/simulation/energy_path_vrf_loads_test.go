package simulation

import (
	"database/sql"
	"math"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func vrfLoadTestDictionary(index int, name string) energyExplanationDictionary {
	definition, _ := energyLoadAliasDefinitionForName(name)
	return energyExplanationDictionary{row: sqlOutputDictionaryRow{index: index, keyValue: "SPACE1-1", name: name, units: "J"},
		reportingFrequency: "Monthly", load: &definition}
}

func vrfLoadTestRow(index int, month int, kWh float64) SQLSeriesRow {
	return SQLSeriesRow{DictionaryIndex: index, TimeIndex: int64(month), Month: sql.NullInt64{Int64: int64(month), Valid: true},
		Value: sql.NullFloat64{Float64: kWh * 3.6e6, Valid: true}}
}

func TestEnergyPathVRFLoadShadowRetainsPrecisionAndKnownness(t *testing.T) {
	doc := energyPathVRFDocument(t)
	plan := vrfSQLPlan(doc)
	systems := energyPathVRFSystems(doc)
	dictionary := vrfLoadTestDictionary(501, "Zone Air System Sensible Cooling Energy")
	collector := newEnergyPathVRFLoadCollector([]energyExplanationDictionary{dictionary}, systems, &plan, map[int64]bool{1: true, 2: true, 3: true})
	collector.observe(vrfLoadTestRow(501, 1, 0))
	collector.observe(vrfLoadTestRow(501, 2, 0.00014921830534))
	collector.observe(vrfLoadTestRow(501, 99, 1000)) // Not a Weather Monthly row.
	evidence := collector.evidence()["sql-rdd-501"]
	if value, exists := evidence.Monthly[1]; !exists || value != 0 {
		t.Fatal("observed zero load was lost")
	}
	if math.Abs(evidence.Monthly[2]-0.00014921830534) > 1e-18 {
		t.Fatalf("native load was rounded before allocation: %.18g", evidence.Monthly[2])
	}
	if _, exists := evidence.Monthly[3]; exists || len(evidence.Monthly) != 2 {
		t.Fatal("missing/excluded load was filled with an observed zero")
	}
	for _, test := range []struct {
		name   string
		mutate func(*SQLSeriesRow)
	}{
		{"null", func(row *SQLSeriesRow) { row.Value.Valid = false }},
		{"negative", func(row *SQLSeriesRow) { row.Value.Float64 = -1 }},
		{"infinite", func(row *SQLSeriesRow) { row.Value.Float64 = math.Inf(1) }},
		{"nan", func(row *SQLSeriesRow) { row.Value.Float64 = math.NaN() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := newEnergyPathVRFLoadCollector([]energyExplanationDictionary{dictionary}, systems, &plan, map[int64]bool{1: true, 2: true})
			bad := vrfLoadTestRow(501, 1, 1)
			test.mutate(&bad)
			current.observe(bad)
			current.observe(vrfLoadTestRow(501, 1, 10)) // A duplicate cannot heal NULL.
			current.observe(vrfLoadTestRow(501, 2, 0))
			item := current.evidence()["sql-rdd-501"]
			if !item.InvalidMonths[1] || item.InvalidMonths[2] {
				t.Fatalf("bad month contaminated or was repaired: %#v", item)
			}
			if value, ok := item.Monthly[2]; !ok || value != 0 {
				t.Fatal("unaffected observed zero month was lost")
			}
		})
	}
	collector.observe(vrfLoadTestRow(501, 2, 0.00014921830534))
	if !collector.evidence()["sql-rdd-501"].InvalidMonths[2] {
		t.Fatal("duplicate valid monthly rows were added")
	}
}

func TestEnergyPathVRFLoadShadowRejectsIdentityAndRequestContradictions(t *testing.T) {
	doc := energyPathVRFDocument(t)
	plan := vrfSQLPlan(doc)
	systems := energyPathVRFSystems(doc)
	dictionary := vrfLoadTestDictionary(501, "Zone Air System Sensible Cooling Energy")
	for _, test := range []struct {
		name   string
		mutate func(*energyExplanationDictionary)
	}{
		{"wrong key", func(item *energyExplanationDictionary) { item.row.keyValue = "TU1" }},
		{"unserved plenum", func(item *energyExplanationDictionary) { item.row.keyValue = "PLENUM-1" }},
		{"rate", func(item *energyExplanationDictionary) { item.row.units = "W" }},
		{"timestep", func(item *energyExplanationDictionary) { item.reportingFrequency = "Zone Timestep" }},
		{"meter", func(item *energyExplanationDictionary) { item.isMeter = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := dictionary
			test.mutate(&copy)
			collector := newEnergyPathVRFLoadCollector([]energyExplanationDictionary{copy}, systems, &plan, map[int64]bool{1: true})
			if len(collector.evidence()) != 0 {
				t.Fatal("contradictory load became denominator evidence")
			}
		})
	}
	duplicate := dictionary
	duplicate.row.index = 502
	collector := newEnergyPathVRFLoadCollector([]energyExplanationDictionary{dictionary, duplicate}, systems, &plan, map[int64]bool{1: true})
	for _, item := range collector.evidence() {
		if !item.Invalid {
			t.Fatal("ambiguous original name/key selected by first dictionary")
		}
	}
	for i := range plan.OutputObjects {
		if plan.OutputObjects[i].KeyValue == "SPACE1-1" && plan.OutputObjects[i].VariableName == dictionary.row.name && plan.OutputObjects[i].ReportingFrequency == "Monthly" {
			plan.OutputObjects[i].ScopeZoneName = "SPACE2-1"
		}
	}
	collector = newEnergyPathVRFLoadCollector([]energyExplanationDictionary{dictionary}, systems, &plan, map[int64]bool{1: true})
	if len(collector.evidence()) != 0 {
		t.Fatal("forged request owner was accepted")
	}
}

func vrfLoadBridgeFixture() ([]energyExplanationSeries, []EnergyDataSource, map[string]energyPathVRFLoadEvidence, []energyPathVRFConsumptionCohort) {
	selected := []energyExplanationSeries{{Level: "load", Kind: "load.zone_cooling", ServiceKind: "cooling", ZoneName: "SPACE1-1", Unit: "kWh",
		SourceIDs: []string{"sql-rdd-501", "unselected-system-detail"}, MonthlySourceIDs: []string{"sql-rdd-501"}, Monthly: map[int]float64{1: 0, 2: 0.001, 3: 0}}}
	sources := []EnergyDataSource{{ID: "sql-rdd-501", EffectiveMultiplier: 7, MultiplierApplication: energyMultiplierRequiresZone}}
	evidence := map[string]energyPathVRFLoadEvidence{"sql-rdd-501": {ZoneName: "SPACE1-1", ServiceKind: "cooling", Monthly: map[int]float64{1: 0, 2: 0.00014921830534, 3: 0}, InvalidMonths: map[int]bool{}}}
	cohorts := []energyPathVRFConsumptionCohort{{System: energyPathVRFSystem{Terminals: []energyPathVRFTerminal{{ZoneName: "SPACE1-1"}}}, Months: map[int]bool{1: true, 2: true, 3: true}}}
	return selected, sources, evidence, cohorts
}

func TestEnergyPathVRFLoadBridgeUsesSelectedSourcesAndOneMultiplier(t *testing.T) {
	selected, sources, evidence, cohorts := vrfLoadBridgeFixture()
	observations := buildEnergyPathVRFLoadObservations(selected, sources, evidence, cohorts)
	if len(observations) != 1 || len(observations[0].SourceIDs) != 1 || len(observations[0].Monthly) != 3 || len(observations[0].InvalidMonths) != 0 {
		t.Fatalf("canonical monthly selection lost: %#v", observations)
	}
	if got := observations[0].Monthly[2]; math.Abs(got-0.00104452813738) > 1e-17 {
		t.Fatalf("native effective load = %.18g, want one factor7 before display rounding", got)
	}
	if zero, known := observations[0].Monthly[1]; !known || zero != 0 {
		t.Fatal("known-zero owner was dropped from denominator")
	}
	if evidence["sql-rdd-501"].Monthly[2] != 0.00014921830534 {
		t.Fatal("load bridge mutated original raw observation")
	}
}

func TestEnergyPathVRFLoadBridgeKeepsUnknownMonthsUnknown(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func([]EnergyDataSource, map[string]energyPathVRFLoadEvidence)
		allBad bool
	}{
		{"missing zero month", func(_ []EnergyDataSource, e map[string]energyPathVRFLoadEvidence) {
			delete(e["sql-rdd-501"].Monthly, 1)
		}, false},
		{"invalid zero month", func(_ []EnergyDataSource, e map[string]energyPathVRFLoadEvidence) {
			e["sql-rdd-501"].InvalidMonths[1] = true
		}, false},
		{"unknown source", func(s []EnergyDataSource, _ map[string]energyPathVRFLoadEvidence) { s[0].ID = "elsewhere" }, true},
		{"unknown multiplier", func(s []EnergyDataSource, _ map[string]energyPathVRFLoadEvidence) {
			s[0].MultiplierApplication = energyMultiplierUnknown
		}, true},
		{"nonfinite multiplier", func(s []EnergyDataSource, _ map[string]energyPathVRFLoadEvidence) {
			s[0].EffectiveMultiplier = math.Inf(1)
		}, true},
		{"wrong owner", func(_ []EnergyDataSource, e map[string]energyPathVRFLoadEvidence) {
			v := e["sql-rdd-501"]
			v.ZoneName = "SPACE2-1"
			e["sql-rdd-501"] = v
		}, true},
		{"wrong service", func(_ []EnergyDataSource, e map[string]energyPathVRFLoadEvidence) {
			v := e["sql-rdd-501"]
			v.ServiceKind = "heating"
			e["sql-rdd-501"] = v
		}, true},
		{"ambiguous identity", func(_ []EnergyDataSource, e map[string]energyPathVRFLoadEvidence) {
			v := e["sql-rdd-501"]
			v.Invalid = true
			e["sql-rdd-501"] = v
		}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			selected, sources, evidence, cohorts := vrfLoadBridgeFixture()
			test.mutate(sources, evidence)
			got := buildEnergyPathVRFLoadObservations(selected, sources, evidence, cohorts)
			if len(got) != 1 || !got[0].InvalidMonths[1] {
				t.Fatalf("unknown source was made into a load weight: %#v", got)
			}
			if _, known := got[0].Monthly[1]; known {
				t.Fatal("missing zero month became observed zero")
			}
			if !test.allBad && len(got[0].Monthly) != 2 {
				t.Fatal("an invalid month discarded unrelated valid months")
			}
		})
	}
}

func TestEnergyPathVRFLoadBridgeCanonicalSQLRoundTrip(t *testing.T) {
	doc := energyPathVRFDocument(t)
	plan := vrfSQLPlan(doc)
	path := vrfSQLFixture(t)
	read := func() []energyPathVRFLoadObservation {
		t.Helper()
		legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
		if err != nil {
			t.Fatal(err)
		}
		if len(legacy.vrfLoadSeries) != 10 || len(legacy.vrfLoadEvidence) != 10 || len(legacy.vrfConsumption) != 1 {
			t.Fatalf("canonical SQL lost the full original load roster: selected%d evidence%d systems%d", len(legacy.vrfLoadSeries), len(legacy.vrfLoadEvidence), len(legacy.vrfConsumption))
		}
		return buildEnergyPathVRFLoadObservations(legacy.vrfLoadSeries, legacy.Sources, legacy.vrfLoadEvidence, legacy.vrfConsumption)
	}
	observed := read()
	if len(observed) != 10 {
		t.Fatalf("original owner load observations = %d, want10", len(observed))
	}
	for _, item := range observed {
		if len(item.Monthly) != 12 || len(item.InvalidMonths) != 0 || len(item.SourceIDs) != 1 {
			t.Fatalf("source selection/precision evidence lost in canonical processing: %#v", item)
		}
		if item.ZoneName == "SPACE1-1" && item.ServiceKind == "cooling" && (item.Monthly[1] != 10 || item.Monthly[2] != 100) {
			t.Fatalf("actual odd/even monthly load weights = %#v", item.Monthly)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=302 AND TimeIndex=2`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range read() {
		if item.ZoneName != "SPACE1-1" || item.ServiceKind != "cooling" {
			if len(item.Monthly) != 12 || len(item.InvalidMonths) != 0 {
				t.Fatal("one invalid owner load erased an unrelated source")
			}
			continue
		}
		if len(item.Monthly) != 11 || !item.InvalidMonths[2] {
			t.Fatalf("canonical selection silently healed an unknown load denominator: %#v", item)
		}
	}
}
