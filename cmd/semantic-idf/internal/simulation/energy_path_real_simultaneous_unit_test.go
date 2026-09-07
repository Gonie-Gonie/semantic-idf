package simulation

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"
)

func epathSimultaneousUnitSQL(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "same-time-operation.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(1,3),(2,1)`,
		`CREATE TABLE "Time"(TimeIndex INTEGER PRIMARY KEY,EnvironmentPeriodIndex INTEGER,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" REAL,WarmupFlag INTEGER,IntervalType INTEGER)`,
		`INSERT INTO "Time" VALUES(100,1,2017,4,3,8,0,10,0,-1)`,
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,Name TEXT,KeyValue TEXT,IsMeter INTEGER,ReportingFrequency TEXT,Units TEXT,"Type" TEXT,TimestepType TEXT)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,ReportDataDictionaryIndex INTEGER,TimeIndex INTEGER,Value REAL)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for index, source := range epathRealSimultaneousSources {
		if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,'CHILLERBANK',0,'Zone Timestep',?,'Avg','HVAC System')`, index+10, source.Name, source.Unit); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO ReportData VALUES(?,?,100,?)`, index+1, index+10, []float64{0.8, 0.9, 14, 6, 56, 60}[index]); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func epathSimultaneousUnitEdit(t *testing.T, path, statement string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(statement, args...); err != nil {
		t.Fatal(err)
	}
}

func TestEnergyPathRealSimultaneousExactSameTimeAndReadOnly(t *testing.T) {
	path := epathSimultaneousUnitSQL(t)
	before := epathRealFileHash(t, path)
	observed, err := epathReadRealSimultaneousSQL(path)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Intervals != 1 || observed.JointIntervals != 1 || observed.First == nil || observed.First.TimeIndex != 100 || observed.First.CoolingMassFlow != 0.8 || observed.First.HeatingOutlet != 60 || observed.First.IntervalMinutes != 10 {
		t.Fatalf("lost exact joint operation: %+v", observed)
	}
	if epathRealFileHash(t, path) != before {
		t.Fatal("independent reader changed SQL")
	}
}

func TestEnergyPathRealSimultaneousRejectsUnknownAndAmbiguous(t *testing.T) {
	for _, test := range []struct {
		name, statement string
		args            []any
	}{
		{"missing identity", `DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=10`, nil},
		{"monthly-only identity", `UPDATE ReportDataDictionary SET ReportingFrequency='Monthly'`, nil},
		{"wrong key", `UPDATE ReportDataDictionary SET KeyValue='OTHER CHILLERBANK' WHERE ReportDataDictionaryIndex=10`, nil},
		{"wrong unit", `UPDATE ReportDataDictionary SET Units='W' WHERE ReportDataDictionaryIndex=10`, nil},
		{"meter instead of variable", `UPDATE ReportDataDictionary SET IsMeter=1 WHERE ReportDataDictionaryIndex=10`, nil},
		{"alias spelling", `UPDATE ReportDataDictionary SET Name=lower(Name) WHERE ReportDataDictionaryIndex=10`, nil},
		{"duplicate dictionary", `INSERT INTO ReportDataDictionary SELECT 90,Name,KeyValue,IsMeter,ReportingFrequency,Units,"Type",TimestepType FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=10`, nil},
		{"case-fold duplicate alias", `INSERT INTO ReportDataDictionary SELECT 90,lower(Name),lower(KeyValue),IsMeter,ReportingFrequency,Units,"Type",TimestepType FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=10`, nil},
		{"duplicate observed row", `INSERT INTO ReportData SELECT 90,ReportDataDictionaryIndex,TimeIndex,Value FROM ReportData WHERE ReportDataIndex=1`, nil},
		{"missing same-time row", `DELETE FROM ReportData WHERE ReportDataIndex=1`, nil},
		{"null value", `UPDATE ReportData SET Value=NULL WHERE ReportDataIndex=1`, nil},
		{"infinite value", `UPDATE ReportData SET Value=? WHERE ReportDataIndex=1`, []any{math.Inf(1)}},
		{"negative infinite value", `UPDATE ReportData SET Value=? WHERE ReportDataIndex=1`, []any{math.Inf(-1)}},
		{"NaN maps to missing not zero", `UPDATE ReportData SET Value=? WHERE ReportDataIndex=1`, []any{math.NaN()}},
		{"invalid numeric text", `UPDATE ReportData SET Value='NaN' WHERE ReportDataIndex=1`, nil},
		{"missing interval", `UPDATE "Time" SET "Interval"=NULL`, nil},
		{"annual interval mislabeled timestep", `UPDATE "Time" SET "Interval"=525600`, nil},
		{"invalid calendar", `UPDATE "Time" SET Month=2,Day=30`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := epathSimultaneousUnitSQL(t)
			epathSimultaneousUnitEdit(t, path, test.statement, test.args...)
			if _, err := epathReadRealSimultaneousSQL(path); err == nil {
				t.Fatal("unknown/ambiguous data became accepted joint operation")
			}
		})
	}
}

func TestEnergyPathRealSimultaneousDoesNotInferFromUnalignedOrContextData(t *testing.T) {
	for _, test := range []struct{ name, statement string }{
		{"warmup", `UPDATE "Time" SET WarmupFlag=1`},
		{"unknown warmup", `UPDATE "Time" SET WarmupFlag=NULL`},
		{"design environment", `UPDATE "Time" SET EnvironmentPeriodIndex=2`},
		{"month coincidence", `UPDATE "Time" SET IntervalType=3,"Interval"=43200`},
		{"hour instead of reviewed native timestep", `UPDATE "Time" SET IntervalType=1`},
		{"zero cooling flow", `UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=10`},
		{"zero heating flow", `UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=11`},
		{"negative cooling flow", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=10`},
		{"negative heating flow", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=11`},
		{"no cooling temperature drop", `UPDATE ReportData SET Value=14 WHERE ReportDataDictionaryIndex=13`},
		{"no heating temperature rise", `UPDATE ReportData SET Value=56 WHERE ReportDataDictionaryIndex=15`},
		{"below temperature threshold", `UPDATE ReportData SET Value=13.999 WHERE ReportDataDictionaryIndex=13`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := epathSimultaneousUnitSQL(t)
			epathSimultaneousUnitEdit(t, path, test.statement)
			observed, err := epathReadRealSimultaneousSQL(path)
			if err != nil {
				t.Fatal(err)
			}
			if observed.Intervals != 0 || observed.First != nil {
				t.Fatalf("non-operation/context counted: %+v", observed)
			}
		})
	}
	t.Run("cooling and heating at different complete times", func(t *testing.T) {
		path := epathSimultaneousUnitSQL(t)
		epathSimultaneousUnitEdit(t, path, `INSERT INTO "Time" VALUES(200,1,2017,4,3,8,10,10,0,-1)`)
		epathSimultaneousUnitEdit(t, path, `INSERT INTO ReportData SELECT ReportDataIndex+20,ReportDataDictionaryIndex,200,Value FROM ReportData`)
		epathSimultaneousUnitEdit(t, path, `UPDATE ReportData SET Value=0 WHERE (TimeIndex=100 AND ReportDataDictionaryIndex=11) OR (TimeIndex=200 AND ReportDataDictionaryIndex=10)`)
		observed, err := epathReadRealSimultaneousSQL(path)
		if err != nil {
			t.Fatal(err)
		}
		if observed.JointIntervals != 2 || observed.Intervals != 0 || observed.First != nil {
			t.Fatal("different-time cooling/heating became simultaneous")
		}
	})
}
