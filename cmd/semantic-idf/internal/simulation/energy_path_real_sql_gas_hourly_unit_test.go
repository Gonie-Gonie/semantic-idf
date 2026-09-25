package simulation

import (
	"database/sql"
	"math"
	"reflect"
	"testing"
)

// The gas extension adds exactly two thermal J identities. It does not admit
// fuel consumption, W alternatives, or any candidate-selected key/quantity.
func epathSQLGasHourlyFixture(t *testing.T, name string, zero bool) (string, epathRealSQLModel, []epathRealSQLSource, epathSQLFrames) {
	t.Helper()
	path, model, _, frames := epathSQLHourlyCompanionUnitFixture(t, "J", func(index int) float64 {
		if zero {
			return 0
		}
		if index == 0 {
			return 2
		}
		if index == 1 {
			return .00049
		}
		return 0
	})
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE ReportDataDictionary SET Name=?,KeyValue='Office' WHERE ReportDataDictionaryIndex IN (40,41)`, name); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLValidateOriginalZoneMultipliers(db, "Zone,Office,,,,,,7;", epathSQLOriginalMultiplierContract); err != nil {
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
	frames.SourceIdentities = map[int]epathRealSQLSource{}
	for _, s := range observed.Sources {
		if s.DictionaryIndex == 41 {
			frames.SourceIdentities[41] = s
		}
	}
	family, component := "equipment.sensible", "sensible"
	if name == "Zone Gas Equipment Latent Gain Energy" {
		family, component = "equipment.latent", "latent"
	}
	frames.Cells = map[string]*epathSQLCell{}
	for month := 1; month <= 12; month++ {
		raw := 0.0
		if month == 1 && !zero {
			raw = 2.00049
		}
		frames.Cells[epathSQLKey("office", family, month)] = &epathSQLCell{Zone: "Office", Family: family, Category: "internal.equipment", Component: component, Month: month, SourceIDs: []int{41}, Raw: epathSQLQuantity{Value: raw}, Effective: epathSQLQuantity{Value: raw * 7}}
	}
	return path, model, observed.Sources, frames
}

func TestEnergyPathRealSQLGasHourlySelectedMonthlyAndKnownZero(t *testing.T) {
	for _, name := range []string{"Zone Gas Equipment Convective Heating Energy", "Zone Gas Equipment Latent Gain Energy"} {
		for _, zero := range []bool{false, true} {
			path, model, observed, frames := epathSQLGasHourlyFixture(t, name, zero)
			before := map[string]epathSQLCell{}
			for key, cell := range frames.Cells {
				before[key] = *cell
			}
			if err := epathSQLBindHourlyCompanions(path, observed, model, &frames); err != nil {
				t.Fatal(err)
			}
			proof := frames.TraceSourceIdentities[40].NativeCompanion
			if proof == nil || proof.Source.Name != name || proof.Authority.DictionaryIndex != 41 || proof.Multiplier != 7 || proof.ZoneName != "Office" {
				t.Fatal("gas thermal companion escaped exact selected Monthly/Zone identity")
			}
			wantNative, wantWire := 2.00049, 2.0
			if zero {
				wantNative, wantWire = 0, 0
			}
			if math.Abs(*proof.Source.EnergyKWh-wantNative) > 1e-12 || proof.ReportedScalar != wantWire || len(proof.HourlyValues) != 8760 || proof.HourlyValues[0] != wantWire || proof.HourlyValues[1] != 0 {
				t.Fatal("native gas thermal transport lost original row rounding or known zero")
			}
			for key, cell := range frames.Cells {
				if !reflect.DeepEqual(*cell, before[key]) || !reflect.DeepEqual(frames.CellTraceSourceIDs[key], []int{40}) {
					t.Fatal("gas Hourly provenance changed Monthly pressure/denominator")
				}
			}
			if len(frames.SourceRaw) != 0 || len(frames.SourceEffective) != 0 || len(frames.Loads) != 0 || len(frames.SourceZone) != 1 {
				t.Fatal("gas Hourly trace became additive numeric/Zone authority")
			}
			if _, err := epathSQLTemporalTraceAnnualQuantity(frames.TraceSourceIdentities[40], "rawValue"); err == nil {
				t.Fatal("gas cell-local provenance promoted to annual numeric authority")
			}
			source := epathSQLHourlyCompanionUnitSource(*proof)
			if err := epathSQLMatchHourlyCompanionSource(source, *proof); err != nil {
				t.Fatal(err)
			}
			for _, mutation := range []string{"fuel-consumption", "rate", "wrong-zone", "twice", "unknown", "invented-vector"} {
				bad := source
				bad.HourlyEnergy = &EnergySourceHourlyEnergy{Unit: "kWh", Basis: "reported_source", Values: append([]float64(nil), source.HourlyEnergy.Values...)}
				switch mutation {
				case "fuel-consumption":
					bad.Name = "Zone Gas Equipment NaturalGas Energy"
				case "rate":
					bad.Name, bad.Units, bad.SourceUnit = "Zone Gas Equipment Convective Heating Rate", "W", "W"
				case "wrong-zone":
					bad.ZoneName = "Elsewhere"
				case "twice":
					bad.EffectiveMultiplier = 49
					bad.EffectiveValue *= 7
				case "unknown":
					bad.inspectorValuePresence = 0
				case "invented-vector":
					bad.HourlyEnergy.Values[1] = .001
				}
				if err := epathSQLMatchHourlyCompanionSource(bad, *proof); err == nil {
					t.Fatalf("accepted gas thermal companion mutation %s/%s", name, mutation)
				}
			}
		}
	}
}

func TestEnergyPathRealSQLGasHourlyRejectsUnselectedForeignAndNativeMismatch(t *testing.T) {
	for _, name := range []string{"Zone Gas Equipment Convective Heating Energy", "Zone Gas Equipment Latent Gain Energy"} {
		path, model, observed, frames := epathSQLGasHourlyFixture(t, name, false)
		for _, mutation := range []string{"unselected-monthly", "unbound-cell", "wrong-factor", "wrong-key", "wrong-unit"} {
			f, m := frames, model
			m.HourlyCompanions = append([]epathRealSQLHourlyCompanion(nil), model.HourlyCompanions...)
			switch mutation {
			case "unselected-monthly":
				f.SourceIdentities = map[int]epathRealSQLSource{}
			case "unbound-cell":
				f.Cells = map[string]*epathSQLCell{}
			case "wrong-factor":
				f.Zones = map[string]epathSQLZone{"office": {Name: "Office", Multiplier: 49}}
			case "wrong-key":
				m.HourlyCompanions[0].Keys = []string{"Other"}
			case "wrong-unit":
				m.HourlyCompanions[0].Unit = "W"
			}
			if _, _, err := epathCompileSQLHourlyCompanions(path, observed, m, f); err == nil {
				t.Fatalf("accepted gas native authority mutation %s/%s", name, mutation)
			}
		}
		epathOracleEditSQL(t, path, `UPDATE ReportData SET Value=Value+36000 WHERE ReportDataDictionaryIndex=41 AND TimeIndex=8761`)
		changed, err := epathReadRealSQLOracle(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range changed.Sources {
			if s.DictionaryIndex == 41 {
				frames.SourceIdentities[41] = s
			}
		}
		if _, _, err := epathCompileSQLHourlyCompanions(path, changed.Sources, model, frames); err == nil {
			t.Fatal("gas native H/M mismatch accepted presentation-sized tolerance")
		}
	}
	for _, name := range []string{"Zone Gas Equipment NaturalGas Energy", "Zone Gas Equipment Convective Heating Rate", "Zone Gas Equipment Latent Gain Rate", "Zone Gas Equipment Total Heating Energy"} {
		if epathSQLHourlyCompanionUnit(name) != "" {
			t.Fatalf("unreviewed gas/fuel identity admitted as thermal companion: %s", name)
		}
	}
}
