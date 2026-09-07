package simulation

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEPATH140SQLSeriesKeepDictionaryIdentityForDuplicateFrequencies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	createTestEnergyPlusSQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`ALTER TABLE ReportDataDictionary ADD COLUMN ReportingFrequency TEXT`,
		`ALTER TABLE ReportDataDictionary ADD COLUMN IsMeter INTEGER`,
		`UPDATE ReportDataDictionary SET ReportingFrequency='Monthly', IsMeter=0 WHERE ReportDataDictionaryIndex=10`,
		`INSERT INTO ReportDataDictionary VALUES (14, 'ZONE ONE', 'Zone Mean Air Temperature', 'C', 'Hourly', 0)`,
		`INSERT INTO ReportData VALUES (100, 1, 14, 30), (101, 2, 14, 31)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	series, err := parseSimulationSQLSeries(path)
	if err != nil {
		t.Fatal(err)
	}
	bySource := map[string]SimulationSeries{}
	for _, item := range series {
		bySource[item.SourceID] = item
	}
	monthly, monthlyOK := bySource["sql-rdd-10"]
	hourly, hourlyOK := bySource["sql-rdd-14"]
	if !monthlyOK || !hourlyOK || monthly.File != hourly.File || monthly.Column != hourly.Column || monthly.Column != "ZONE ONE:Zone Mean Air Temperature [C]" {
		t.Fatalf("duplicate semantic series lost exact dictionary identity or legacy labels: %#v", series)
	}
	if monthly.ReportingFrequency != "Monthly" || hourly.ReportingFrequency != "Hourly" || monthly.Name != "Zone Mean Air Temperature" || monthly.KeyValue != "ZONE ONE" || monthly.IsMeter == nil || *monthly.IsMeter || hourly.IsMeter == nil || *hourly.IsMeter {
		t.Fatalf("source identity metadata was missing or fabricated: monthly=%#v hourly=%#v", monthly, hourly)
	}
	if len(monthly.Points) != 2 || monthly.Points[0].Value != 20 || monthly.Points[1].Value != 21.5 || monthly.Points[0].Label != "01-01 01:00" || len(hourly.Points) != 2 || hourly.Points[0].Value != 30 || hourly.Points[1].Value != 31 {
		t.Fatal("metadata projection changed original observations or labels")
	}
	encoded, err := json.Marshal(monthly)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"isMeter":false`) {
		t.Fatalf("explicit variable class became unknown on JSON write: %s", encoded)
	}
	var restored SimulationSeries
	if err := json.Unmarshal(encoded, &restored); err != nil || restored.SourceID != monthly.SourceID || restored.ReportingFrequency != monthly.ReportingFrequency || restored.IsMeter == nil || *restored.IsMeter {
		t.Fatalf("series provenance did not survive JSON: %#v err=%v", restored, err)
	}
}

func TestEPATH140SQLSeriesOptionalMetadataRemainsUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	createTestEnergyPlusSQL(t, path)
	series, err := parseSimulationSQLSeries(path)
	if err != nil || len(series) != 4 {
		t.Fatalf("older dictionary schema failed: %#v %v", series, err)
	}
	for _, item := range series {
		if item.SourceID == "" || item.ReportingFrequency != "" || item.IsMeter != nil {
			t.Errorf("missing frequency/type inferred from name or point spacing: %#v", item)
		}
	}
}

func TestEPATH140CSVSeriesRetainsLegacyIdentityWithoutSQLMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.csv")
	if err := os.WriteFile(path, []byte("Date/Time,ZONE ONE:Zone Air Temperature [C](Hourly)\n01/01 01:00:00,20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, series, err := parseSimulationCSV(path)
	if err != nil || len(series) != 1 {
		t.Fatalf("CSV parser changed: %#v %v", series, err)
	}
	item := series[0]
	if item.File != "eplusout.csv" || item.Column != "ZONE ONE:Zone Air Temperature [C](Hourly)" || item.SourceID != "" || item.ReportingFrequency != "" || item.Name != "" || item.KeyValue != "" || item.IsMeter != nil {
		t.Fatalf("SQL metadata leaked into legacy CSV series: %#v", item)
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"sourceId", "reportingFrequency", "isMeter", "name", "keyValue"} {
		if strings.Contains(string(encoded), `"`+key+`":`) {
			t.Errorf("optional field %s changed CSV JSON shape: %s", key, encoded)
		}
	}
}
