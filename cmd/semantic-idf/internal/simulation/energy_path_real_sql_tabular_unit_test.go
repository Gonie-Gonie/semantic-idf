package simulation

import (
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func epathSQLTabularUnitSelector() epathRealSQLTabularSelector {
	return epathRealSQLTabularSelector{ReportName: "AnnualBuildingUtilityPerformanceSummary", ReportForString: "Entire Facility", TableName: "End Uses", RowName: "Cooling", ColumnName: "District Cooling", Unit: "kWh", DecimalPlaces: 2}
}

func epathSQLTabularUnitFixture(t *testing.T) string {
	t.Helper()
	path := epathOracleUnitSQL(t)
	epathOracleEditSQL(t, path, `CREATE TABLE TabularDataWithStrings(TabularDataIndex INTEGER,Value TEXT,ReportName TEXT,ReportForString TEXT,TableName TEXT,RowName TEXT,ColumnName TEXT,Units TEXT);
INSERT INTO TabularDataWithStrings VALUES(1,'   12.34','AnnualBuildingUtilityPerformanceSummary','Entire Facility','End Uses','Cooling','District Cooling','kWh');
INSERT INTO TabularDataWithStrings VALUES(2,'999.99','SourceEnergyEndUseComponentsSummary','Entire Facility','Source Energy End Use Components Summary','Cooling','District Cooling','kWh');
INSERT INTO TabularDataWithStrings VALUES(3,'888.88','DemandEndUseComponentsSummary','Entire Facility','End Uses','Cooling','District Cooling','kW');
INSERT INTO TabularDataWithStrings VALUES(4,'777.77','AnnualBuildingUtilityPerformanceSummary','Some Other Facility','End Uses','Cooling','District Cooling','kWh');
INSERT INTO TabularDataWithStrings VALUES(5,'0.00','AnnualBuildingUtilityPerformanceSummary','Entire Facility','End Uses','Cooling','District Heating Steam','kWh');`)
	return path
}

func TestEnergyPathRealSQLTabularExactAnnualObservation(t *testing.T) {
	path, selector := epathSQLTabularUnitFixture(t), epathSQLTabularUnitSelector()
	before := epathRealFileHash(t, path)
	observed, err := epathReadSQLModelTabular(path, selector)
	if err != nil || observed == nil {
		t.Fatalf("exact annual observation unavailable: %v", err)
	}
	if !reflect.DeepEqual(observed.Selector, selector) || observed.TabularDataIndex != 1 || observed.RawText != "   12.34" || observed.RawValue != 12.34 || observed.Quantity.Value != 12.34 || observed.Weather.Year != 2017 || len(observed.Weather.Months) != 12 || observed.Weather.EnvironmentIndex != 1 {
		t.Fatalf("original annual identity/value/weather changed: %+v", observed)
	}
	low, high := observed.Quantity.bounds()
	if math.Abs(low-12.335) > 1e-12 || math.Abs(high-12.345) > 1e-12 {
		t.Fatalf("source display uncertainty was not half of0.01 kWh: [%g,%g]", low, high)
	}
	selector.ColumnName = "District Heating Steam"
	zero, err := epathReadSQLModelTabular(path, selector)
	if err != nil || zero == nil || zero.RawValue != 0 || zero.Quantity.Value != 0 || !zero.Quantity.includesZero() {
		t.Fatalf("literal observed zero became unknown: %+v %v", zero, err)
	}
	selector.ColumnName = "Absent Column"
	missing, err := epathReadSQLModelTabular(path, selector)
	if err != nil || missing != nil {
		t.Fatalf("exact absent cell became a zero/alternate observation: %+v %v", missing, err)
	}
	if epathRealFileHash(t, path) != before {
		t.Fatal("independent annual read mutated original SQL")
	}
}

func TestEnergyPathRealSQLTabularExactUnitConversion(t *testing.T) {
	for _, test := range []struct {
		unit, raw     string
		value, factor float64
	}{
		{"J", "3600000.00", 1, 1.0 / 3600000},
		{"MJ", "3.60", 1, 1.0 / 3.6},
		{"GJ", "3.60", 1000, 1000.0 / 3.6},
		{"Wh", "1000.00", 1, .001},
		{"kWh", "1.00", 1, 1},
	} {
		t.Run(test.unit, func(t *testing.T) {
			path, selector := epathSQLTabularUnitFixture(t), epathSQLTabularUnitSelector()
			selector.Unit = test.unit
			epathOracleEditSQL(t, path, fmt.Sprintf("UPDATE TabularDataWithStrings SET Units='%s',Value='%s' WHERE TabularDataIndex=1", test.unit, test.raw))
			observed, err := epathReadSQLModelTabular(path, selector)
			if err != nil || observed == nil {
				t.Fatalf("explicit conversion failed: %v", err)
			}
			low, high := observed.Quantity.bounds()
			if math.Abs(observed.Quantity.Value-test.value) > 1e-12 || math.Abs(low-(test.value-.005*test.factor)) > 1e-12 || math.Abs(high-(test.value+.005*test.factor)) > 1e-12 {
				t.Fatalf("conversion changed source uncertainty or distributed energy: %+v", observed.Quantity)
			}
		})
	}
}

func TestEnergyPathRealSQLTabularRejectAmbiguousInvalidCells(t *testing.T) {
	for name, query := range map[string]string{
		"duplicate identical":            `INSERT INTO TabularDataWithStrings SELECT 9,Value,ReportName,ReportForString,TableName,RowName,ColumnName,Units FROM TabularDataWithStrings WHERE TabularDataIndex=1`,
		"duplicate different":            `INSERT INTO TabularDataWithStrings SELECT 9,'54.32',ReportName,ReportForString,TableName,RowName,ColumnName,Units FROM TabularDataWithStrings WHERE TabularDataIndex=1`,
		"NULL value":                     `UPDATE TabularDataWithStrings SET Value=NULL WHERE TabularDataIndex=1`,
		"NULL unit":                      `UPDATE TabularDataWithStrings SET Units=NULL WHERE TabularDataIndex=1`,
		"wrong exact unit":               `UPDATE TabularDataWithStrings SET Units='GJ' WHERE TabularDataIndex=1`,
		"invalid original index":         `UPDATE TabularDataWithStrings SET TabularDataIndex=0 WHERE TabularDataIndex=1`,
		"NULL original index":            `UPDATE TabularDataWithStrings SET TabularDataIndex=NULL WHERE TabularDataIndex=1`,
		"missing schema":                 `DROP TABLE TabularDataWithStrings`,
		"partial weather":                `DELETE FROM "Time" WHERE TimeIndex=6`,
		"design-day substitution":        `UPDATE "Time" SET EnvironmentPeriodIndex=2 WHERE TimeIndex=6`,
		"warmup substitution":            `UPDATE "Time" SET WarmupFlag=1 WHERE TimeIndex=6`,
		"multiple weather environments":  `INSERT INTO EnvironmentPeriods VALUES(3,'OTHER WEATHER',3); INSERT INTO "Time" SELECT 9000,3,Year,Month,Day,Hour,Minute,"Interval",WarmupFlag,IntervalType,SimulationDays FROM "Time" WHERE TimeIndex=1`,
		"duplicated month":               `INSERT INTO "Time" SELECT 9000,EnvironmentPeriodIndex,Year,Month,Day,Hour,Minute,"Interval",WarmupFlag,IntervalType,SimulationDays FROM "Time" WHERE TimeIndex=1`,
		"incomplete cumulative calendar": `UPDATE "Time" SET SimulationDays=17 WHERE TimeIndex=1`,
	} {
		t.Run(name, func(t *testing.T) {
			path := epathSQLTabularUnitFixture(t)
			epathOracleEditSQL(t, path, query)
			if value, err := epathReadSQLModelTabular(path, epathSQLTabularUnitSelector()); err == nil {
				t.Fatalf("ambiguous/invalid original cell accepted: %+v", value)
			}
		})
	}
	for _, raw := range []string{"", "NaN", "Inf", "-1.00", "1,000.00", "1.2", "1.234", "1e2", strings.Repeat("9", 400) + ".00"} {
		t.Run("invalid raw "+raw[:min(12, len(raw))], func(t *testing.T) {
			path := epathSQLTabularUnitFixture(t)
			epathOracleEditSQL(t, path, "UPDATE TabularDataWithStrings SET Value='"+raw+"' WHERE TabularDataIndex=1")
			if value, err := epathReadSQLModelTabular(path, epathSQLTabularUnitSelector()); err == nil {
				t.Fatalf("invalid/precision-mismatched raw value accepted: %+v", value)
			}
		})
	}
}

func TestEnergyPathRealSQLTabularRejectWrongPhysicalSelector(t *testing.T) {
	for name, edit := range map[string]func(*epathRealSQLTabularSelector){
		"source energy":     func(s *epathRealSQLTabularSelector) { s.ReportName = "SourceEnergyEndUseComponentsSummary" },
		"demand energy":     func(s *epathRealSQLTabularSelector) { s.ReportName = "DemandEndUseComponentsSummary" },
		"wrong facility":    func(s *epathRealSQLTabularSelector) { s.ReportForString = "Some Other Facility" },
		"other table":       func(s *epathRealSQLTabularSelector) { s.TableName = "Source Energy End Use Components Summary" },
		"power not energy":  func(s *epathRealSQLTabularSelector) { s.Unit = "kW" },
		"missing precision": func(s *epathRealSQLTabularSelector) { s.DecimalPlaces = 0 },
		"empty row":         func(s *epathRealSQLTabularSelector) { s.RowName = "" },
		"padded column":     func(s *epathRealSQLTabularSelector) { s.ColumnName = "District Cooling " },
	} {
		t.Run(name, func(t *testing.T) {
			selector := epathSQLTabularUnitSelector()
			edit(&selector)
			if _, err := epathValidateSQLTabularSelector(selector); err == nil {
				t.Fatal("unreviewed physical selector accepted")
			}
		})
	}
	path := epathSQLTabularUnitFixture(t)
	epathOracleEditSQL(t, path, `DELETE FROM TabularDataWithStrings WHERE TabularDataIndex=1`)
	if value, err := epathReadSQLModelTabular(path, epathSQLTabularUnitSelector()); err != nil || value != nil {
		t.Fatalf("unrelated SourceEnergy/Demand report supplied missing annual site cell: %+v %v", value, err)
	}
}

func TestEnergyPathRealSQLTabularExplicitSiteBoundary(t *testing.T) {
	selector := epathSQLTabularUnitSelector()
	site := epathRealSQLSite{ID: "cooling.district_cooling", EndUse: "cooling", Carrier: "district_cooling", Tabular: &selector}
	if err := epathValidateSQLSiteSource(site); err != nil {
		t.Fatal(err)
	}
	for _, source := range []epathRealSQLSelector{
		{Alternatives: []epathRealSQLAlternative{{Name: "Cooling:DistrictCooling", Unit: "J"}}},
		{Keys: []string{""}}, {IsMeter: true}, {AllowAbsent: true},
	} {
		mixed := site
		mixed.Source = source
		if err := epathValidateSQLSiteSource(mixed); err == nil {
			t.Fatal("mixed monthly/Tabular site sources accepted")
		}
	}
	path, model := epathSQLModelUnitFixture(t)
	epathOracleEditSQL(t, path, `CREATE TABLE TabularDataWithStrings(TabularDataIndex INTEGER,Value TEXT,ReportName TEXT,ReportForString TEXT,TableName TEXT,RowName TEXT,ColumnName TEXT,Units TEXT);
INSERT INTO TabularDataWithStrings VALUES(1,'12.34','AnnualBuildingUtilityPerformanceSummary','Entire Facility','End Uses','Cooling','District Cooling','kWh');`)
	model.Site = []epathRealSQLSite{site}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	annual, found := frames.SiteAnnual[site.ID]
	low, high := annual.Quantity.bounds()
	if !found || annual.TabularDataIndex != 1 || annual.RawText != "12.34" || annual.Quantity.Value != 12.34 || math.Abs(low-12.335) > 1e-12 || math.Abs(high-12.345) > 1e-12 {
		t.Fatalf("compiler lost exact annual source/value/precision: %+v", annual)
	}
	if _, exists := frames.Site[site.ID]; exists || len(frames.SiteSources[site.ID]) != 0 {
		t.Fatal("annual cell gained fabricated monthly series or RDD identities")
	}
	for month := 1; month <= 12; month++ {
		q, err := epathSQLSitePeriod(frames, site.ID, fmt.Sprintf("M%d", month))
		if err != nil || q != nil {
			t.Fatal("annual-only source became a monthly zero or divided quantity")
		}
		if load := frames.Loads[epathSQLKey("office", "cooling", month)]; load.Value != 18 {
			t.Fatal("annual source changed separately observed monthly delivered load")
		}
	}
}

func TestEnergyPathRealSQLTabularOriginalIdealLoads(t *testing.T) {
	path := os.Getenv("EPATH_REAL_SQL_TABULAR_SQL")
	if path == "" {
		t.Skip("explicit original IdealLoads SQL path required; read-only source proof, not acceptance")
	}
	before := epathRealFileHash(t, path)
	if before != "c43c777904b93b313617f8dcc8e40388bec4b3ac3549e4279d15f13c5019849d" {
		t.Fatal("SQL does not match the reviewed original IdealLoads capture")
	}
	for _, cell := range []struct {
		row, column string
		value       float64
	}{
		{"Cooling", "District Cooling", 19149.37}, {"Total End Uses", "District Cooling", 19149.37},
		{"Heating", "District Heating Water", 7129.51}, {"Total End Uses", "District Heating Water", 7129.51},
		{"Heating", "District Heating Steam", 0},
	} {
		selector := epathSQLTabularUnitSelector()
		selector.RowName, selector.ColumnName = cell.row, cell.column
		observed, err := epathReadSQLModelTabular(path, selector)
		if err != nil || observed == nil {
			t.Fatalf("original %s/%s missing: %v", cell.row, cell.column, err)
		}
		low, high := observed.Quantity.bounds()
		if observed.RawValue != cell.value || observed.Quantity.Value != cell.value || math.Abs(low-math.Max(0, cell.value-.005)) > 1e-10 || math.Abs(high-(cell.value+.005)) > 1e-10 || observed.Weather.Year != 2017 || len(observed.Weather.Months) != 12 {
			t.Fatalf("original exact annual cell/uncertainty changed: %+v", observed)
		}
		t.Logf("ORIGINAL SQL ONLY, NOT ACCEPTANCE: %s/%s %.2f kWh [%g,%g], cell%d", cell.row, cell.column, observed.RawValue, low, high, observed.TabularDataIndex)
	}
	if after := epathRealFileHash(t, path); after != before {
		t.Fatal("original IdealLoads SQL changed during read-only proof")
	}
}
