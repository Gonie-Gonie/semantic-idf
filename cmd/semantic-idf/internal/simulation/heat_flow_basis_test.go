package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestHeatFlowRateBasisAcrossReaders(t *testing.T) {
	for _, format := range []string{"csv", "eso", "sql"} {
		t.Run(format, func(t *testing.T) {
			input, path := heatFlowRateBasisFixture(t, format, "")
			read := func(source heatFlowSourceOptions) (HeatFlowDataset, error) {
				switch format {
				case "csv":
					return parseSimulationHeatFlowCSV(path, source)
				case "eso":
					return parseSimulationHeatFlowESO(path, source)
				default:
					return parseSimulationHeatFlowSQL(path, source)
				}
			}
			data, err := read(heatFlowSourceOptions{InputPath: input, EngineVersion: "25.1.0"})
			if err != nil {
				t.Fatal(err)
			}
			if len(data.Zones) != 1 {
				t.Fatalf("zones=%d", len(data.Zones))
			}
			zone := data.Zones[0]
			if !reflect.DeepEqual(zone.Values, [][]float64{{100, 100, 100}, {-2.058, 0, 0}, {12, 0, 0}}) || !reflect.DeepEqual(zone.Observed, [][]bool{{true, true, true}, {true, true, false}, {true, false, true}}) {
				t.Fatalf("basis/masks changed: %#v", zone)
			}
			basis := zone.RateBasis
			if basis == nil || basis.EffectiveMultiplier != 6 || basis.SystemAir != "modeled_zone" || basis.SystemConvective != "modeled_zone" || !reflect.DeepEqual(basis.ReportedSystemAir, []float64{-12.345678, 0, 0}) || !reflect.DeepEqual(basis.ReportedSystemConvective, []float64{72, 0, 0}) {
				t.Fatalf("raw values or applied factor lost: %#v", basis)
			}
			if len(data.Warnings) != 0 {
				t.Fatal(data.Warnings)
			}
			wire, err := json.Marshal(zone)
			if err != nil {
				t.Fatal(err)
			}
			var restored HeatFlowZoneSeries
			if err := json.Unmarshal(wire, &restored); err != nil || !reflect.DeepEqual(zone, restored) {
				t.Fatal("rate basis/raw observation metadata did not round-trip", err)
			}
			if format == "sql" {
				series, err := parseSimulationSQLSeries(path)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, item := range series {
					if strings.Contains(item.Name, "System Air Transfer") {
						found = true
						if item.Points[0].Value != -12.345678 {
							t.Fatalf("generic series was normalized: %#v", item)
						}
					}
				}
				if !found {
					t.Fatal("generic raw series missing")
				}
			}
			for _, version := range []string{"", "99.1.0"} {
				unknown, err := read(heatFlowSourceOptions{InputPath: input, EngineVersion: version})
				if err != nil {
					t.Fatal(err)
				}
				if unknown.Zones[0].RateBasis != nil || unknown.Zones[0].Values[1][0] != -12.346 || unknown.Zones[0].Values[2][0] != 72 || !reflect.DeepEqual(unknown.Zones[0].Observed, zone.Observed) {
					t.Fatalf("IDF25.1 incorrectly certified producing engine %q: %#v", version, unknown.Zones[0])
				}
			}
		})
	}
}

func TestHeatFlowRateBasisConservativeMixedAndUnknown(t *testing.T) {
	input, path := heatFlowRateBasisFixture(t, "csv", "SwimmingPool:Indoor, Pool;\n")
	source := heatFlowSourceOptions{InputPath: input, EngineVersion: "25.1.0"}
	data, err := parseSimulationHeatFlowCSV(path, source)
	if err != nil {
		t.Fatal(err)
	}
	zone := data.Zones[0]
	if zone.Values[1][0] != -2.058 || zone.Values[2][0] != 72 || zone.RateBasis.SystemConvective != "energyplus_reported" || len(data.Warnings) != 1 {
		t.Fatalf("mixed aggregate was divided or not disclosed: %#v / %v", zone, data.Warnings)
	}
	unknown, err := parseSimulationHeatFlowCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Zones[0].RateBasis != nil || unknown.Zones[0].Values[1][0] != -12.346 {
		t.Fatalf("unknown/legacy basis was guessed: %#v", unknown.Zones[0])
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(content), ",72\n", ",0\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	zero, err := parseSimulationHeatFlowCSV(path, source)
	if err != nil {
		t.Fatal(err)
	}
	if zero.Zones[0].Values[2][0] != 0 || !zero.Zones[0].Observed[2][0] || len(zero.Warnings) != 1 {
		t.Fatal("reported mixed-basis zero concealed uncertainty", zero.Warnings)
	}
}

func TestHeatFlowRateBasisSQLUsesProducingMetadata(t *testing.T) {
	input, path := heatFlowRateBasisFixture(t, "sql", "")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE Simulations (SimulationIndex INTEGER, EnergyPlusVersion TEXT)`,
		`INSERT INTO Simulations VALUES (1,'EnergyPlus, Version 25.1.0-68a4a7c774')`,
		`CREATE TABLE Zones (ZoneName TEXT, Multiplier REAL, ListMultiplier REAL)`,
		`INSERT INTO Zones VALUES ('OFFICE',4,3)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// Producing SQL metadata wins over an explicit conflicting version too.
	data, err := parseSimulationHeatFlowSQL(path, heatFlowSourceOptions{InputPath: input, EngineVersion: "99.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if data.Zones[0].Values[1][0] != -1.029 || data.Zones[0].Values[2][0] != 6 || data.Zones[0].RateBasis.EffectiveMultiplier != 12 {
		t.Fatalf("SQL producing multiplier not authoritative: %#v", data.Zones[0])
	}
	data, err = parseSimulationHeatFlowSQL(path)
	if err != nil {
		t.Fatal(err)
	}
	if data.Zones[0].Values[1][0] != -1.029 || data.Zones[0].Values[2][0] != 72 || len(data.Warnings) != 1 {
		t.Fatalf("SQL without executed input guessed nonair composition: %#v", data)
	}
}

func TestHeatFlowRateBasisSavedSQL(t *testing.T) {
	// Optional original engine outputs are read-only local evidence, not fixtures
	// required in a fresh clone. No simulation or oracle regeneration occurs.
	directory := filepath.Join("..", "..", "..", "..", ".runtime", "energy-path-acceptance", "25.1", "large-office-25-1", "real-large-office-25-1-20260907T162304.141598300")
	path := filepath.Join(directory, "eplusout.sql")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("optional original EnergyPlus SQL unavailable")
	}
	data, err := parseSimulationHeatFlowSQL(path, heatFlowSourceOptions{InputPath: filepath.Join(directory, "executed-model.idf")})
	if err != nil {
		t.Fatal(err)
	}
	for _, zone := range data.Zones {
		if zone.Name != "CORE_MID" {
			continue
		}
		if zone.RateBasis == nil || zone.RateBasis.EffectiveMultiplier != 10 {
			t.Fatalf("missing producing multiplier: %#v", zone.RateBasis)
		}
		for i, category := range data.Categories {
			if category.ID == "systemAir" {
				count := 0
				for frame, raw := range zone.RateBasis.ReportedSystemAir {
					if !zone.Observed[i][frame] {
						continue
					}
					count++
					if zone.Values[i][frame] != roundedHeatFlowNumber(raw/10) {
						t.Fatalf("real source mismatch at %s: raw %.9fW normalized %.9fW", data.Labels[frame], raw, zone.Values[i][frame])
					}
				}
				if count != 12 {
					t.Fatalf("native monthly CORE_MID system-air observations lost: %d", count)
				}
				// Check the actual peak SQL observation through the identical basis
				// context as an independent quantity/provenance cross-check.
				db, err := openSimulationSQLiteReadOnly(path)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				var raw float64
				if err := db.QueryRow(`SELECT r.Value FROM ReportData r JOIN ReportDataDictionary d USING (ReportDataDictionaryIndex) JOIN Time t USING (TimeIndex) WHERE d.KeyValue='CORE_MID' AND d.Name='Zone Air Heat Balance System Air Transfer Rate' AND d.ReportingFrequency='Monthly' AND t.Month=8 LIMIT 1`).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				builder := &heatFlowZoneBuilder{name: "CORE_MID"}
				corrected := newHeatFlowBasisContext(db, heatFlowSourceOptions{InputPath: filepath.Join(directory, "executed-model.idf")}).value(builder, category, 0, raw)
				if math.Abs(raw-(-260112.50058018052)) > 0.000001 || math.Abs(corrected-(-26011.25005801805)) > 0.000001 {
					t.Fatalf("peak source basis mismatch: raw %.9fW corrected %.9fW", raw, corrected)
				}
				t.Logf("CORE_MID peak raw %.9fW /10 = %.9fW; %d native rates verified", raw, corrected, count)
				return
			}
		}
	}
	t.Fatal("CORE_MID systemAir missing")
}

func heatFlowRateBasisFixture(t *testing.T, format, extra string) (string, string) {
	t.Helper()
	directory := t.TempDir()
	input := filepath.Join(directory, "executed-model.idf")
	if err := os.WriteFile(input, []byte("Version,25.1;\nZone,Office,0,0,0,0,1,2;\nZoneList,GroupZones,Office;\nZoneGroup,Group,GroupZones,3;\n"+extra), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "eplusout."+format)
	names := []string{"Zone Air Heat Balance Internal Convective Heat Gain Rate", "Zone Air Heat Balance System Air Transfer Rate", "Zone Air Heat Balance System Convective Heat Gain Rate"}
	values := [][]string{{"100", "-12.345678", "72"}, {"100", "0", ""}, {"100", "", "0"}}
	if format == "sql" {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		for _, statement := range []string{`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT)`, `CREATE TABLE Time (TimeIndex INTEGER PRIMARY KEY, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER)`, `CREATE TABLE ReportData (TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`} {
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
		for i, name := range names {
			if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES (?,?,?,?)`, i+10, "Office", name, "W"); err != nil {
				t.Fatal(err)
			}
		}
		for frame, row := range values {
			if _, err := db.Exec(`INSERT INTO Time VALUES (?,1,1,?,0)`, frame+1, frame+1); err != nil {
				t.Fatal(err)
			}
			for i, value := range row {
				var raw any
				if value != "" {
					raw = value
				}
				if _, err := db.Exec(`INSERT INTO ReportData VALUES (?,?,?)`, frame+1, i+10, raw); err != nil {
					t.Fatal(err)
				}
			}
		}
		return input, path
	}
	var text strings.Builder
	if format == "csv" {
		text.WriteString("Date/Time")
		for _, name := range names {
			fmt.Fprintf(&text, ",Office:%s [W](Hourly)", name)
		}
		text.WriteByte('\n')
		for frame, row := range values {
			fmt.Fprintf(&text, "01-01 %02d:00,%s\n", frame+1, strings.Join(row, ","))
		}
	} else {
		text.WriteString("Program Version,EnergyPlus\n2,8,Day of Simulation[],Month[],Day of Month[],DST Indicator[],Hour[],StartMinute[],EndMinute[],DayType\n")
		for i, name := range names {
			fmt.Fprintf(&text, "%d,1,Office,%s [W] !Hourly\n", i+10, name)
		}
		text.WriteString("End of Data Dictionary\n")
		for frame, row := range values {
			fmt.Fprintf(&text, "2,1,1,1,0,%d,0,60,Monday\n", frame+1)
			for i, value := range row {
				if value != "" {
					fmt.Fprintf(&text, "%d,%s\n", i+10, value)
				}
			}
		}
		text.WriteString("End of Data\n")
	}
	if err := os.WriteFile(path, []byte(text.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return input, path
}
