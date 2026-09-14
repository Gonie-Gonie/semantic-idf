package simulation

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathDirectHVACHourlyTraceDoesNotBorrowMonthlyRequest(t *testing.T) {
	doc, _, _ := directHVACSQLDocument(t)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	path := directHVACSQLFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO "Time" (TimeIndex,Month,Day,Hour,Minute,Year,"Interval",IntervalType,EnvironmentPeriodIndex,WarmupFlag) VALUES(3,1,1,1,0,2017,60,1,3,0)`,
		`INSERT INTO ReportDataDictionary VALUES(910,'SPACE1-1 PTAC CCoil','Cooling Coil Electricity Energy','J',0,'Hourly','HVAC')`,
		`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(3,910,5400000)`,
	} {
		if _, err := db.Exec(query); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	request := func(index int, key, frequency, zone string) PurposeOutputObject {
		return PurposeOutputObject{ObjectType: "Output:Variable", KeyValue: key, VariableName: "Cooling Coil Electricity Energy",
			ReportingFrequency: frequency, ScopeZoneName: zone, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, ObjectIndex: &index}
	}
	monthly := request(42, "SPACE1-1 PTAC CCoil", "Monthly", "SPACE1-1")
	hourly := request(7, "SPACE1-1 PTAC CCoil", "Hourly", "SPACE1-1")
	wildcard := request(8, "*", "Hourly", "")
	for _, tc := range []struct {
		name    string
		outputs []PurposeOutputObject
		index   int
	}{
		{"exact Hourly after Monthly", []PurposeOutputObject{monthly, hourly}, 7},
		{"exact Hourly before Monthly", []PurposeOutputObject{hourly, monthly}, 7},
		{"Monthly ownership alone gives no Hourly index", []PurposeOutputObject{monthly}, -1},
		{"native wildcard gives actual Hourly index", []PurposeOutputObject{monthly, wildcard}, 8},
		{"duplicate native wildcard indices remain unknown", []PurposeOutputObject{monthly, wildcard, request(9, "*", "Hourly", "")}, -1},
		{"duplicate exact indices remain unknown", []PurposeOutputObject{monthly, hourly, request(9, "SPACE1-1 PTAC CCoil", "Hourly", "SPACE1-1")}, -1},
		{"same original index is not ambiguous", []PurposeOutputObject{monthly, hourly, hourly}, 7},
		{"exact key takes precedence over wildcard", []PurposeOutputObject{monthly, wildcard, hourly}, 7},
		{"other Zone cannot supply Hourly index", []PurposeOutputObject{monthly, request(9, "SPACE1-1 PTAC CCoil", "Hourly", "PLENUM-1")}, -1},
		{"other frequency cannot supply Hourly index", []PurposeOutputObject{monthly, request(9, "SPACE1-1 PTAC CCoil", "Daily", "SPACE1-1")}, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := directHVACSQLPlan(doc)
			plan.OutputObjects = tc.outputs
			parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for _, source := range parsed.Sources {
				if source.ID != "sql-rdd-110" && source.ID != "sql-rdd-910" {
					continue
				}
				seen[source.ID] = true
				wantIndex, wantValue, wantFrequency := 42, 30.0, "Monthly"
				if source.ID == "sql-rdd-910" {
					wantIndex, wantValue, wantFrequency = tc.index, 1.5, "Hourly"
					if source.HourlyEnergy == nil || !reflect.DeepEqual(source.HourlyEnergy.Values, []float64{1.5}) || !reflect.DeepEqual(parsed.HourlyLabels, []string{"01-01 01:00"}) {
						t.Errorf("native Hourly observation lost: source=%+v labels=%v", source, parsed.HourlyLabels)
					}
				}
				if wantIndex < 0 && source.ObjectIndex != nil || wantIndex >= 0 && (source.ObjectIndex == nil || *source.ObjectIndex != wantIndex) {
					t.Errorf("%s request index=%v, want %d", source.ID, source.ObjectIndex, wantIndex)
				}
				if source.RawValue != wantValue || source.ReportingFrequency != wantFrequency || source.ZoneName != "SPACE1-1" || source.KeyValue != "SPACE1-1 PTAC CCoil" || source.Units != "J" || source.NormalizedUnit != "kWh" {
					t.Errorf("source boundary/value changed: %+v", source)
				}
			}
			if !seen["sql-rdd-110"] || !seen["sql-rdd-910"] {
				t.Errorf("missing distinct Monthly/Hourly source: %v", seen)
			}
			for _, item := range parsed.Series {
				if stringSliceContains(item.SourceIDs, "sql-rdd-910") {
					t.Error("source chart became an additive direct constituent")
				}
			}
		})
	}
}
