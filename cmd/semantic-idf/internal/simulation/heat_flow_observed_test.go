package simulation

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseSimulationHeatFlowPreservesObservedValuesAcrossFormats(t *testing.T) {
	// Zero temperatures and rates are valid observations. Absent columns for a
	// particular zone, blanks, NaN and Infinity must remain distinguishable from
	// those zeros, including leading/interior/trailing gaps.
	records := [][4]string{
		{"0", "0", "10", ""},
		{"NaN", "NaN", "+Inf", "0"},
		{"-5", "-12.3456", "", "20"},
	}
	for _, format := range []string{"csv", "eso", "sql"} {
		t.Run(format, func(t *testing.T) {
			dataset := heatFlowObservedFixture(t, format, records)
			if dataset.FrameCount != 3 || dataset.OriginalFrameCount != 3 || len(dataset.Categories) != 2 || len(dataset.Zones) != 2 {
				t.Fatalf("unexpected dataset dimensions: %#v", dataset)
			}
			office, lab := dataset.Zones[1], dataset.Zones[0]
			if office.Name != "Office" || lab.Name != "Lab" {
				t.Fatal("zone order or source identity changed")
			}
			if !reflect.DeepEqual(office.Values, [][]float64{{0, 0, -12.346}, {10, 0, 0}}) ||
				!reflect.DeepEqual(office.Observed, [][]bool{{true, false, true}, {true, false, false}}) ||
				!reflect.DeepEqual(office.Temperature, []float64{0, 0, -5}) ||
				!reflect.DeepEqual(office.TemperatureObserved, []bool{true, false, true}) {
				t.Fatalf("Office lost zero/missing/signed observation distinctions: %#v", office)
			}
			if !reflect.DeepEqual(lab.Values, [][]float64{{0, 0, 0}, {0, 0, 20}}) ||
				!reflect.DeepEqual(lab.Observed, [][]bool{{false, false, false}, {false, true, true}}) ||
				len(lab.Temperature) != 0 || len(lab.TemperatureObserved) != 0 {
				t.Fatalf("Lab borrowed another zone's missing category or temperature: %#v", lab)
			}
			wire, err := json.Marshal(office)
			if err != nil {
				t.Fatal(err)
			}
			var restored HeatFlowZoneSeries
			if err := json.Unmarshal(wire, &restored); err != nil || !reflect.DeepEqual(restored, office) {
				t.Fatalf("observation masks did not survive saved-result JSON: %s / %v", wire, err)
			}
		})
	}
}

func TestParseSimulationHeatFlowSamplingPreservesObservationMasks(t *testing.T) {
	// 730 native frames select every second frame, plus the otherwise skipped
	// final frame. The selected middle gap and final signed readings exercise
	// masks against sampling rather than merely checking their array shape.
	records := make([][4]string, 730)
	for i := range records {
		records[i] = [4]string{"0", "0", "", "1"}
	}
	records[0][2] = "10"
	records[364] = [4]string{"", "NaN", "NaN", "0"}
	records[729] = [4]string{"-3", "-25", "invalid", "1"}
	for _, format := range []string{"csv", "eso", "sql"} {
		t.Run(format, func(t *testing.T) {
			dataset := heatFlowObservedFixture(t, format, records)
			if dataset.FrameCount != 366 || dataset.OriginalFrameCount != 730 || len(dataset.Warnings) != 1 {
				t.Fatalf("sampling policy changed: frames=%d/%d warnings=%v", dataset.FrameCount, dataset.OriginalFrameCount, dataset.Warnings)
			}
			office, lab := dataset.Zones[1], dataset.Zones[0]
			for frame := 0; frame < dataset.FrameCount; frame++ {
				wantObserved := frame != 182
				if office.Observed[0][frame] != wantObserved || office.TemperatureObserved[frame] != wantObserved ||
					office.Observed[1][frame] != (frame == 0) || lab.Observed[0][frame] || !lab.Observed[1][frame] {
					t.Fatalf("observation masks drifted from selected frame %d: Office=%#v Lab=%#v", frame, office, lab)
				}
			}
			if office.Values[0][365] != -25 || office.Temperature[365] != -3 || lab.Values[1][182] != 0 || !lab.Observed[1][182] {
				t.Fatal("forced last-frame values or the sampled measured zero were lost")
			}
		})
	}
}

func heatFlowObservedFixture(t *testing.T, format string, records [][4]string) HeatFlowDataset {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eplusout."+format)
	variables := []string{
		"Zone Mean Air Temperature",
		"Zone Air Heat Balance Internal Convective Heat Gain Rate",
		"Zone Air Heat Balance Surface Convection Rate",
		"Zone Air Heat Balance Surface Convection Rate",
	}
	zones := []string{"Office", "Office", "Office", "Lab"}
	units := []string{"C", "W", "W", "W"}
	date := func(frame int) time.Time {
		return time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(frame+1) * time.Hour)
	}
	var dataset HeatFlowDataset
	var err error
	switch format {
	case "csv":
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		writer := csv.NewWriter(file)
		header := []string{"Date/Time"}
		for i, variable := range variables {
			header = append(header, zones[i]+":"+variable+" ["+units[i]+"](Hourly)")
		}
		writer.Write(header)
		for frame, values := range records {
			writer.Write(append([]string{date(frame).Format("01-02 15:04")}, values[:]...))
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		dataset, err = parseSimulationHeatFlowCSV(path)
	case "eso":
		var content strings.Builder
		content.WriteString("Program Version,EnergyPlus\n2,8,Day of Simulation[],Month[],Day of Month[],DST Indicator[],Hour[],StartMinute[],EndMinute[],DayType\n")
		for i, variable := range variables {
			fmt.Fprintf(&content, "%d,1,%s,%s [%s] !Hourly\n", i+10, zones[i], variable, units[i])
		}
		content.WriteString("End of Data Dictionary\n")
		for frame, values := range records {
			current := date(frame)
			fmt.Fprintf(&content, "2,1,%d,%d,0,%d,0,60,Monday\n", current.Month(), current.Day(), current.Hour())
			for i, value := range values {
				if value != "" {
					fmt.Fprintf(&content, "%d,%s\n", i+10, value)
				}
			}
		}
		if err := os.WriteFile(path, []byte(content.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		dataset, err = parseSimulationHeatFlowESO(path)
	case "sql":
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range []string{
			`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT)`,
			`CREATE TABLE Time (TimeIndex INTEGER PRIMARY KEY, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER)`,
			`CREATE TABLE ReportData (TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		} {
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		for i, variable := range variables {
			if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES (?,?,?,?)`, i+10, zones[i], variable, units[i]); err != nil {
				t.Fatal(err)
			}
		}
		for frame, values := range records {
			current := date(frame)
			if _, err := tx.Exec(`INSERT INTO Time VALUES (?,?,?,?,0)`, frame+1, int(current.Month()), current.Day(), current.Hour()); err != nil {
				t.Fatal(err)
			}
			for i, value := range values {
				var observation any
				if parsed, parseErr := strconv.ParseFloat(value, 64); parseErr == nil {
					observation = parsed
				}
				if _, err := tx.Exec(`INSERT INTO ReportData VALUES (?,?,?)`, frame+1, i+10, observation); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		dataset, err = parseSimulationHeatFlowSQL(path)
	default:
		t.Fatalf("unsupported fixture format %q", format)
	}
	if err != nil {
		t.Fatal(err)
	}
	return dataset
}
