package simulation

import (
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Literal thermal names only. Fan electricity stays in the typed, already-
// model-total direct-fan path; it must never acquire a Zone multiplier here.
var epathSQLVentilationHourlyUnitNames = []string{
	"Zone Ventilation Sensible Heat Gain Energy",
	"Zone Ventilation Sensible Heat Loss Energy",
	"Zone Ventilation Latent Heat Gain Energy",
	"Zone Ventilation Latent Heat Loss Energy",
}

// Reuse only the ordinary companion's independent SQLite/calendar fixture.
// No production alias, classifier, candidate or real-capture quantity is used.
func epathSQLVentilationHourlyUnitFixture(t *testing.T, name string, zero bool) (string, epathRealSQLModel, []epathRealSQLSource, epathSQLFrames) {
	t.Helper()
	path, model, _, frames := epathSQLHourlyCompanionUnitFixture(t, "J", func(i int) float64 {
		if zero {
			return 0
		}
		values := []float64{.00049, .00051, 0, 1.23456}
		if i < len(values) {
			return values[i]
		}
		return 0
	})
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE ReportDataDictionary SET Name=?,KeyValue='Office' WHERE ReportDataDictionaryIndex IN (40,41)", name); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	model.HourlyCompanions = []epathRealSQLHourlyCompanion{{Name: name, Unit: "J", Keys: []string{"Office"}}}
	for _, source := range observed.Sources {
		if source.DictionaryIndex == 41 {
			frames.SourceIdentities[41] = source
		}
	}
	// This hand cell establishes selected-Monthly membership, not an equation
	// for the real fixture's outdoor-air reconciliation.
	frames.Cells = map[string]*epathSQLCell{
		epathSQLKey("office", "ventilation.thermal.hand", 1): {
			Zone: "Office", Family: "ventilation.thermal.hand", Category: "air.mechanical_ventilation",
			Month: 1, SourceIDs: []int{41}, Raw: epathSQLQuantity{Value: 3}, Effective: epathSQLQuantity{Value: 21},
		},
	}
	return path, model, observed.Sources, frames
}

func TestEnergyPathRealSQLVentilationHourlyCompanionFiniteThermalNames(t *testing.T) {
	for _, name := range epathSQLVentilationHourlyUnitNames {
		if got := epathSQLHourlyCompanionUnit(name); got != "J" {
			t.Fatalf("%s unit=%q, want J", name, got)
		}
	}
	for _, name := range []string{
		"Zone Ventilation Fan Electricity Energy",
		"Fan Electricity Energy",
		"Zone Ventilation Sensible Heat Gain Rate",
		"Zone Ventilation Sensible Heat Loss Rate",
		"Zone Ventilation Latent Heat Gain Rate",
		"Zone Ventilation Latent Heat Loss Rate",
		"AFN Zone Ventilation Sensible Heat Gain Energy",
		"Zone Ideal Loads Outdoor Air Sensible Heating Energy",
		"Zone Ventilation Total Heat Gain Energy",
	} {
		if got := epathSQLHourlyCompanionUnit(name); got != "" {
			t.Fatalf("unreviewed resource/rate/context/aggregate %q admitted as %q", name, got)
		}
	}
}

func TestEnergyPathRealSQLVentilationHourlyCompanionSelectedNativeThermalSources(t *testing.T) {
	for _, name := range epathSQLVentilationHourlyUnitNames {
		t.Run(name, func(t *testing.T) {
			path, model, observed, frames := epathSQLVentilationHourlyUnitFixture(t, name, false)
			key := epathSQLKey("office", "ventilation.thermal.hand", 1)
			before := *frames.Cells[key]
			if err := epathSQLBindHourlyCompanions(path, observed, model, &frames); err != nil {
				t.Fatal(err)
			}
			p := frames.TraceSourceIdentities[40].NativeCompanion
			if p == nil || p.Source.Name != name || p.Authority.Name != name || p.Source.KeyValue != "Office" ||
				p.Multiplier != 7 || p.ReportedScalar != 1.236 || math.Abs(*p.Source.EnergyKWh-1.23556) > 1e-12 ||
				!reflect.DeepEqual(p.HourlyValues[:4], []float64{0, .001, 0, 1.235}) {
				t.Fatal("thermal same-identity native/row-quantized transport changed")
			}
			if !reflect.DeepEqual(*frames.Cells[key], before) || !reflect.DeepEqual(frames.CellTraceSourceIDs[key], []int{40}) ||
				len(frames.SourceRaw) != 0 || len(frames.SourceEffective) != 0 || len(frames.SourceZone) != 1 || len(frames.Loads) != 0 {
				t.Fatal("Hourly companion became additive or changed selected Monthly pressure")
			}
			source := epathSQLHourlyCompanionUnitSource(*p)
			if source.EffectiveValue != 8.652 || epathSQLMatchHourlyCompanionSource(source, *p) != nil {
				t.Fatal("thermal Zone multiplier must apply exactly once")
			}
			source.MultiplierApplication, source.EffectiveMultiplier, source.EffectiveValue = "already_model_total", 1, source.RawValue
			if epathSQLMatchHourlyCompanionSource(source, *p) == nil {
				t.Fatal("thermal source was accepted with direct-fan model-total semantics")
			}
			if _, err := epathSQLTemporalTraceAnnualQuantity(frames.TraceSourceIdentities[40], "rawValue"); err == nil {
				t.Fatal("thermal companion became an independent annual amount")
			}
		})
	}
}

func TestEnergyPathRealSQLVentilationHourlyCompanionObservedZeroAndMissingAreDistinct(t *testing.T) {
	path, model, observed, frames := epathSQLVentilationHourlyUnitFixture(t, "Zone Ventilation Latent Heat Loss Energy", true)
	proofs, _, err := epathCompileSQLHourlyCompanions(path, observed, model, frames)
	if err != nil {
		t.Fatal(err)
	}
	p := proofs[40]
	source := epathSQLHourlyCompanionUnitSource(p)
	if source.RawValue != 0 || source.EffectiveValue != 0 || epathSQLMatchHourlyCompanionSource(source, p) != nil {
		t.Fatal("complete literal known-zero observation was lost")
	}
	source.inspectorValuePresence = 0
	if epathSQLMatchHourlyCompanionSource(source, p) == nil {
		t.Fatal("missing scalar was accepted as observed zero")
	}

	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, query string }{
		{"missing Hourly zero", "DELETE FROM ReportData WHERE TimeIndex=1 AND ReportDataDictionaryIndex=40"},
		{"NULL Hourly zero", "UPDATE ReportData SET Value=NULL WHERE TimeIndex=1 AND ReportDataDictionaryIndex=40"},
		{"missing Monthly zero", "DELETE FROM ReportData WHERE TimeIndex=8761 AND ReportDataDictionaryIndex=41"},
		{"different Monthly amount", "UPDATE ReportData SET Value=36 WHERE TimeIndex=8761 AND ReportDataDictionaryIndex=41"},
		{"foreign native key", "UPDATE ReportDataDictionary SET KeyValue='Foreign' WHERE ReportDataDictionaryIndex=40"},
		{"different native Zone multiplier", "UPDATE Zones SET Multiplier=49"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copyPath := filepath.Join(t.TempDir(), "changed.sql")
			if err := os.WriteFile(copyPath, original, 0600); err != nil {
				t.Fatal(err)
			}
			epathOracleEditSQL(t, copyPath, tc.query)
			if _, _, err := epathCompileSQLHourlyCompanions(copyPath, observed, model, frames); err == nil {
				t.Fatal("missing/foreign/non-equivalent native authority accepted")
			}
		})
	}
	delete(frames.SourceIdentities, 41)
	if _, _, err := epathCompileSQLHourlyCompanions(path, observed, model, frames); err == nil {
		t.Fatal("unselected Monthly zero was accepted as a driver authority")
	}
}
