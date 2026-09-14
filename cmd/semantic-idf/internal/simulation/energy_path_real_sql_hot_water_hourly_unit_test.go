package simulation

import (
	"database/sql"
	"math"
	"reflect"
	"testing"
)

func epathSQLHotWaterHourlyFixture(t *testing.T, name string, zero bool) (string, epathRealSQLModel, []epathRealSQLSource, epathSQLFrames) {
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
	if _, err := db.Exec(`UPDATE Zones SET Multiplier=4,ListMultiplier=8`); err != nil {
		t.Fatal(err)
	}
	// Native SQL ownership is checked separately against exact original fields.
	if err := epathSQLValidateOriginalZoneMultipliers(db, "Zone,Office,,,,,,4;ZoneList,Floor,Office;ZoneGroup,Group,Floor,8;", epathSQLOriginalMultiplierContract); err != nil {
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
	frames.Zones["office"] = epathSQLZone{Name: "Office", Multiplier: 32}
	frames.SourceIdentities = map[int]epathRealSQLSource{}
	for _, s := range observed.Sources {
		if s.DictionaryIndex == 41 {
			frames.SourceIdentities[41] = s
		}
	}
	family := "equipment.sensible"
	component := "sensible"
	if name == "Zone Hot Water Equipment Latent Gain Energy" {
		family = "equipment.latent"
		component = "latent"
	}
	frames.Cells = map[string]*epathSQLCell{}
	for month := 1; month <= 12; month++ {
		raw := 0.0
		if month == 1 && !zero {
			raw = 2.00049
		}
		frames.Cells[epathSQLKey("office", family, month)] = &epathSQLCell{Zone: "Office", Family: family, Category: "internal.equipment", Component: component, Month: month, SourceIDs: []int{41}, Raw: epathSQLQuantity{Value: raw}, Effective: epathSQLQuantity{Value: raw * 32}}
	}
	return path, model, observed.Sources, frames
}

func TestEnergyPathRealSQLHotWaterHourlySelectedMonthlyAndMultiplierOnce(t *testing.T) {
	for _, name := range []string{"Zone Hot Water Equipment Convective Heating Energy", "Zone Hot Water Equipment Latent Gain Energy"} {
		for _, zero := range []bool{false, true} {
			path, model, observed, frames := epathSQLHotWaterHourlyFixture(t, name, zero)
			before := map[string]epathSQLCell{}
			for key, cell := range frames.Cells {
				before[key] = *cell
			}
			if err := epathSQLBindHourlyCompanions(path, observed, model, &frames); err != nil {
				t.Fatal(err)
			}
			proof := frames.TraceSourceIdentities[40].NativeCompanion
			if proof == nil || proof.Authority.DictionaryIndex != 41 || proof.Multiplier != 32 || proof.ZoneName != "Office" || proof.Source.Name != name {
				t.Fatal("native HWE companion escaped selected Monthly/Zone identity")
			}
			wantNative, wantWire := 2.00049, 2.0
			if zero {
				wantNative, wantWire = 0, 0
			}
			if math.Abs(*proof.Source.EnergyKWh-wantNative) > 1e-12 || proof.ReportedScalar != wantWire || len(proof.HourlyValues) != 8760 || proof.HourlyValues[0] != wantWire || proof.HourlyValues[1] != 0 {
				t.Fatal("native HWE Hourly scalar/vector used a Monthly or multiplied quantity")
			}
			for key, cell := range frames.Cells {
				if !reflect.DeepEqual(*cell, before[key]) || !reflect.DeepEqual(frames.CellTraceSourceIDs[key], []int{40}) {
					t.Fatal("Hourly HWE changed its Monthly driver pressure or denominator")
				}
			}
			if len(frames.SourceRaw) != 0 || len(frames.SourceEffective) != 0 || len(frames.Loads) != 0 {
				t.Fatal("Hourly HWE became another numeric load/driver")
			}
			source := epathSQLHourlyCompanionUnitSource(*proof)
			source.EffectiveMultiplier = 32
			source.EffectiveValue = wantWire * 32
			if err := epathSQLMatchHourlyCompanionSource(source, *proof); err != nil {
				t.Fatal(err)
			}
			for _, mutation := range []string{"twice", "unknown", "wrong-zone", "wrong-monthly-role", "invented-vector"} {
				bad := source
				bad.HourlyEnergy = &EnergySourceHourlyEnergy{Unit: "kWh", Basis: "reported_source", Values: append([]float64(nil), source.HourlyEnergy.Values...)}
				switch mutation {
				case "twice":
					bad.EffectiveMultiplier = 1024
					bad.EffectiveValue *= 32
				case "unknown":
					bad.inspectorValuePresence = 0
				case "wrong-zone":
					bad.ZoneName = "Elsewhere"
				case "wrong-monthly-role":
					bad.MultiplierApplication = "already_model_total"
				case "invented-vector":
					bad.HourlyEnergy.Values[1] = .001
				}
				if err := epathSQLMatchHourlyCompanionSource(bad, *proof); err == nil {
					t.Fatalf("accepted HWE companion mutation %s/%s", name, mutation)
				}
			}
		}
	}
}

func TestEnergyPathRealSQLHotWaterHourlyRejectsAuthorityOwnershipAndNativeMismatch(t *testing.T) {
	for _, name := range []string{"Zone Hot Water Equipment Convective Heating Energy", "Zone Hot Water Equipment Latent Gain Energy"} {
		path, model, observed, frames := epathSQLHotWaterHourlyFixture(t, name, false)
		for _, mutation := range []string{"unselected-monthly", "unbound-cell", "wrong-factor", "wrong-key", "wrong-unit"} {
			f, m := frames, model
			m.HourlyCompanions = append([]epathRealSQLHourlyCompanion(nil), model.HourlyCompanions...)
			switch mutation {
			case "unselected-monthly":
				f.SourceIdentities = map[int]epathRealSQLSource{}
			case "unbound-cell":
				f.Cells = map[string]*epathSQLCell{}
			case "wrong-factor":
				f.Zones = map[string]epathSQLZone{"office": {Name: "Office", Multiplier: 1024}}
			case "wrong-key":
				m.HourlyCompanions[0].Keys = []string{"Other"}
			case "wrong-unit":
				m.HourlyCompanions[0].Unit = "W"
			}
			if _, _, err := epathCompileSQLHourlyCompanions(path, observed, m, f); err == nil {
				t.Fatalf("accepted native HWE %s", mutation)
			}
		}
		// Preserve valid observed identities but honestly alter the original
		// Monthly row: the native Hourly equivalence gate must reject 0.01 kWh.
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
			t.Fatal("native HWE Hourly mismatch accepted a presentation-sized tolerance")
		}
	}
	if epathSQLHourlyCompanionUnit("Zone Hot Water Equipment District Heating Energy") != "" {
		t.Fatal("site consumption became a thermal driver companion")
	}
}
