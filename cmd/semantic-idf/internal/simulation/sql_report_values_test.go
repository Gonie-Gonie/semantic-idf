package simulation

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func compactReportDataFixture(t *testing.T, statements ...string) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "관측 값 # unchanged.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openSimulationSQLiteReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

func assertCompactReportDataEquivalent(t *testing.T, db *sql.DB, query SQLSeriesQuery) []SQLSeriesRow {
	t.Helper()
	want, wantErr := QueryReportData(db, query)
	got := []SQLSeriesRow{}
	err := walkReportDataCompact(db, query, func(row SQLSeriesRow) error {
		got = append(got, row)
		return nil
	})
	if (err == nil) != (wantErr == nil) {
		t.Fatalf("compact error %v, original error %v", err, wantErr)
	}
	if err == nil && !reflect.DeepEqual(got, want) {
		t.Fatalf("compact rows differ\ngot: %#v\nwant: %#v", got, want)
	}
	return got
}

func TestCompactReportDataPreservesObservationsAndFilters(t *testing.T) {
	db, path := compactReportDataFixture(t,
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER, KeyValue TEXT, Name TEXT, Units TEXT, IsMeter TEXT, ReportingFrequency TEXT, IndexGroup TEXT)`,
		`CREATE TABLE Time (TimeIndex INTEGER, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER, IntervalType INTEGER)`,
		`CREATE TABLE ReportData (TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		`INSERT INTO ReportDataDictionary VALUES
(1,' Office ','Surface Heat','J','false','Hourly','Surface'),
(2,'Office','Surface Heat','J','0','Monthly','Surface'),
(3,'Electricity:Facility','Electricity:Facility','J','yes','RunPeriod','Facility'),
(4,'No observations','Unused','W','0','Timestep','Zone'),
(5,NULL,NULL,NULL,NULL,NULL,NULL)`,
		`INSERT INTO Time VALUES (1,1,1,1,0,1),(2,1,1,2,0,1),(3,1,31,24,0,3),(4,NULL,NULL,NULL,NULL,4),(5,2,1,1,0,NULL)`,
		`INSERT INTO ReportData VALUES
(3,2,0),(1,1,-12),(2,1,NULL),(1,1,7),(4,3,1e999),(5,5,42),
(999,1,3),(2,999,123),(5,1,-1e999)`,
	)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	trueValue, falseValue := true, false
	queries := []SQLSeriesQuery{
		{}, {DictionaryIndexes: []int{1}}, {DictionaryIndexes: []int{999}},
		{Names: []string{"Surface Heat"}}, {KeyValues: []string{"Office"}},
		{Frequency: []string{"Hourly"}}, {Frequency: []string{"Monthly", "RunPeriod"}},
		{Units: []string{"J"}}, {IndexGroups: []string{"Surface"}},
		{IsMeter: &trueValue}, {IsMeter: &falseValue},
		{DictionaryIndexes: []int{1, 2}, Names: []string{"Surface Heat"}, KeyValues: []string{"Office"}, Frequency: []string{"Hourly"}, Units: []string{"J"}, IndexGroups: []string{"Surface"}, IsMeter: &falseValue},
	}
	for index, query := range queries {
		t.Run(string(rune('A'+index)), func(t *testing.T) { assertCompactReportDataEquivalent(t, db, query) })
	}
	rows := assertCompactReportDataEquivalent(t, db, SQLSeriesQuery{})
	if len(rows) != 8 || rows[0].Value.Float64 != -12 || rows[1].Value.Float64 != 7 || rows[2].Value.Valid || !rows[3].Value.Valid || rows[3].Value.Float64 != 0 {
		t.Fatalf("fixture must retain duplicate times, signed, NULL and explicit zero: %#v", rows)
	}
	if rows[len(rows)-1].Month.Valid || rows[len(rows)-1].IntervalType != "" {
		t.Fatal("missing Time row must remain LEFT JOIN unknown, not invent a timestamp")
	}
	stop := errors.New("visitor stopped")
	calls := 0
	if err := walkReportDataCompact(db, SQLSeriesQuery{}, func(SQLSeriesRow) error { calls++; return stop }); !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("visitor error lost: %v, calls %d", err, calls)
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("compact read modified the original database", err)
	}
}

func TestCompactReportDataPreservesSparseAndNonstandardSchemas(t *testing.T) {
	for _, duplicate := range []string{"none", "dictionary", "time", "unused malformed metadata"} {
		t.Run(duplicate, func(t *testing.T) {
			statements := []string{
				`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER)`,
				`CREATE TABLE Time (TimeIndex INTEGER, Month INTEGER)`,
				`CREATE TABLE ReportData (TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
				`INSERT INTO ReportDataDictionary VALUES (1),(NULL)`,
				`INSERT INTO Time VALUES (1,1),(NULL,2)`,
				`INSERT INTO ReportData VALUES (1,1,4),(8,1,NULL)`,
			}
			switch duplicate {
			case "dictionary":
				statements = append(statements, `INSERT INTO ReportDataDictionary VALUES (1)`)
			case "time":
				statements = append(statements, `INSERT INTO Time VALUES (1,2)`)
			case "unused malformed metadata":
				statements = append(statements, `INSERT INTO Time VALUES (100,'invalid integer')`)
			}
			db, _ := compactReportDataFixture(t, statements...)
			rows := assertCompactReportDataEquivalent(t, db, SQLSeriesQuery{})
			if len(rows) < 2 || rows[0].ReportingFrequency != "" || rows[0].Day.Valid {
				t.Fatal("sparse schema metadata changed", rows)
			}
		})
	}
}

func TestCompactReportDataPreservesMissingRequiredColumnError(t *testing.T) {
	db, _ := compactReportDataFixture(t,
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER)`,
		`CREATE TABLE Time (TimeIndex INTEGER)`,
		`CREATE TABLE ReportData (TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER)`,
	)
	assertCompactReportDataEquivalent(t, db, SQLSeriesQuery{})
}

func TestCompactEnergyDictionariesKeepOnlyObservedIdentities(t *testing.T) {
	db, _ := compactReportDataFixture(t,
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER, KeyValue TEXT, Name TEXT, Units TEXT, IsMeter TEXT, ReportingFrequency TEXT, IndexGroup TEXT)`,
		`CREATE TABLE ReportData (TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		`INSERT INTO ReportDataDictionary VALUES
(1,'Electricity:Facility','Electricity:Facility','J','1','Monthly','Facility'),
(2,'Electricity:Facility','Electricity:Facility','J','1','Hourly','Facility'),
(3,'NaturalGas:Facility','NaturalGas:Facility','J','1','Monthly','Facility')`,
		`INSERT INTO ReportData VALUES (1,1,0),(2,1,NULL),(1,2,-10),(2,2,10),(3,99,15)`,
	)
	dictionaries, err := sqlEnergyExplanationDictionaries(db, "actual.sql", &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(dictionaries) != 2 || dictionaries[0].row.index != 1 || dictionaries[1].row.index != 2 ||
		dictionaries[0].reportingFrequency != "Monthly" || dictionaries[1].reportingFrequency != "Hourly" {
		t.Fatalf("observed NULL/zero and distinct frequencies must survive, absent identities must not: %#v", dictionaries)
	}
}

func TestCompactReportDataRetainsSQLiteIdentifierEquality(t *testing.T) {
	for _, fixture := range []struct {
		name, dictionaryType, timeType, reportDictionaryType, reportTimeType string
		dictionaryID, frameID, reportedDictionaryID, reportedTimeID          string
	}{
		{"text dictionary leading zero", "TEXT", "INTEGER", "TEXT", "INTEGER", "'01'", "1", "'01'", "1"},
		{"text Time leading zero is not an integer join", "INTEGER", "TEXT", "INTEGER", "TEXT", "1", "'01'", "1", "'1'"},
		{"integer affinity with blob dictionary", "INTEGER", "INTEGER", "INTEGER", "INTEGER", "x'3031'", "1", "x'3031'", "1"},
		{"integer affinity with blob Time", "INTEGER", "INTEGER", "INTEGER", "INTEGER", "1", "x'3031'", "1", "x'3031'"},
		{"blob report Time must not invent integer match", "INTEGER", "INTEGER", "INTEGER", "INTEGER", "1", "1", "1", "x'3031'"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			db, _ := compactReportDataFixture(t,
				"CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex "+fixture.dictionaryType+")",
				"CREATE TABLE Time (TimeIndex "+fixture.timeType+", Month INTEGER)",
				"CREATE TABLE ReportData (TimeIndex "+fixture.reportTimeType+", ReportDataDictionaryIndex "+fixture.reportDictionaryType+", Value REAL)",
				"INSERT INTO ReportDataDictionary VALUES ("+fixture.dictionaryID+")",
				"INSERT INTO Time VALUES ("+fixture.frameID+",7)",
				"INSERT INTO ReportData VALUES ("+fixture.reportedTimeID+","+fixture.reportedDictionaryID+",42)",
			)
			rows := assertCompactReportDataEquivalent(t, db, SQLSeriesQuery{})
			if len(rows) != 1 || rows[0].Value.Float64 != 42 {
				t.Fatalf("fixture lost its actual joined measurement: %#v", rows)
			}
		})
	}
}
