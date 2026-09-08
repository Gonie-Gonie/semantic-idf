package simulation

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathDirectHVACTraceExactMonthlyRequest(t *testing.T) {
	doc, _, _ := directHVACSQLDocument(t)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	path := directHVACSQLFixture(t)
	request := func(index int, frequency, zone, name string) PurposeOutputObject {
		return PurposeOutputObject{ObjectType: "Output:Variable", KeyValue: "SPACE1-1 PTAC CCoil", VariableName: name,
			ReportingFrequency: frequency, ScopeZoneName: zone, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, ObjectIndex: &index}
	}
	canonical := "Cooling Coil Electricity Energy"
	monthly := request(42, "Monthly", "SPACE1-1", canonical)
	for _, tc := range []struct {
		name    string
		outputs []PurposeOutputObject
		present bool
		index   int // -1 means that conflicting matching request indices remain unknown.
	}{
		{"earlier Hourly is not the Monthly opener", []PurposeOutputObject{request(7, "Hourly", "SPACE1-1", canonical), monthly}, true, 42},
		{"earlier other scope and frequency is not the opener", []PurposeOutputObject{request(8, "Hourly", "PLENUM-1", canonical), monthly}, true, 42},
		{"casefolded and trimmed exact request", []PurposeOutputObject{request(9, "Hourly", "SPACE1-1", canonical), request(43, " monthly ", " space1-1 ", " cooling coil electricity energy ")}, true, 43},
		{"contradictory Monthly owner is rejected", []PurposeOutputObject{request(10, "Monthly", "PLENUM-1", canonical), monthly}, false, -1},
		{"ambiguous exact request index stays unknown", []PurposeOutputObject{monthly, request(44, "Monthly", "SPACE1-1", canonical)}, true, -1},
		{"undeclared semantic alias is not invented", []PurposeOutputObject{request(45, "Monthly", "SPACE1-1", "PTAC Cooling Electricity Energy")}, false, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := directHVACSQLPlan(doc)
			plan.OutputObjects = tc.outputs
			parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			found := 0
			for _, source := range parsed.Sources {
				if source.ID != "sql-rdd-110" {
					continue
				}
				found++
				if !tc.present {
					t.Fatal("unbound request became an owned component source")
				}
				if tc.index < 0 && source.ObjectIndex != nil || tc.index >= 0 && (source.ObjectIndex == nil || *source.ObjectIndex != tc.index) {
					t.Errorf("source opener index=%v, want %d", source.ObjectIndex, tc.index)
				}
				if source.Name != canonical || source.KeyValue != "SPACE1-1 PTAC CCoil" || source.ZoneName != "SPACE1-1" || source.ReportingFrequency != "Monthly" || source.Units != "J" || source.NormalizedUnit != "kWh" || source.RawValue != 30 {
					t.Errorf("exact original source identity/value changed: %+v", source)
				}
			}
			want := 0
			if tc.present {
				want = 1
			}
			if found != want {
				t.Errorf("source cardinality=%d, want %d", found, want)
			}
		})
	}
}

func TestEnergyPathDirectHVACTracePreservesAnnualBuildingTabular(t *testing.T) {
	doc, _, _ := directHVACSQLDocument(t)
	plan := directHVACSQLPlan(doc)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	path := directHVACSQLFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// The independent Building cooling observation is 600 kWh, whereas the
	// five local main/crankcase observations sum to 495 kWh. Neither can stand
	// in for the other. No Monthly broad cooling observation is supplied.
	for _, query := range []string{
		`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=3`,
		`DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=3`,
		`UPDATE ReportData SET Value=260*TimeIndex*3600000 WHERE ReportDataDictionaryIndex=1`,
		`CREATE TABLE TabularDataWithStrings (TabularDataIndex INTEGER PRIMARY KEY, ReportName TEXT, ReportForString TEXT, TableName TEXT, RowName TEXT, ColumnName TEXT, Units TEXT, Value TEXT)`,
		`INSERT INTO TabularDataWithStrings VALUES(1,'AnnualBuildingUtilityPerformanceSummary','Entire Facility','End Uses','Cooling','Electricity','kWh','600.00')`,
	} {
		if _, err := db.Exec(query); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	tabularID, tabularCount := "", 0
	for _, source := range parsed.Sources {
		if source.SourceType == "sql_tabular" && source.TableName == "End Uses" && source.RowName == "Cooling" && source.ColumnName == "Electricity [kWh]" {
			tabularID = source.ID
			tabularCount++
			if source.Name != "Cooling:Electricity" || source.KeyValue != source.Name || source.ReportingFrequency != "Annual" || source.ZoneName != "" {
				t.Errorf("Building Tabular source was relabeled as a component: %+v", source)
			}
		}
	}
	if tabularCount != 1 || tabularID == "" {
		t.Fatalf("local component suppressed independent Building Tabular source: count=%d", tabularCount)
	}
	seriesCount := 0
	for _, item := range parsed.Series {
		if !stringSliceContains(item.SourceIDs, tabularID) {
			continue
		}
		seriesCount++
		if item.Total != 600 || item.ZoneName != "" || item.EndUse != "cooling" || item.Carrier != "electricity" || len(item.Monthly) != 0 {
			t.Errorf("Annual broad observation was replaced or distributed: %+v", item)
		}
	}
	if seriesCount != 1 {
		t.Fatalf("Annual broad series cardinality=%d, want 1", seriesCount)
	}
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.zoneDirectUseSeries) != 25 {
		t.Fatalf("independent direct constituents were lost: %d", len(legacy.zoneDirectUseSeries))
	}
	result := UpgradeEnergyExplanationV1(legacy)
	count := 0
	for _, node := range result.Nodes {
		if node.Level != "end_use" || node.EndUse != "cooling" {
			continue
		}
		count++
		if node.Value != 600 || node.ZoneName != "" || !stringSliceContains(node.SourceIDs, tabularID) {
			t.Errorf("Building Annual cooling lost its 600 kWh Tabular authority: %+v", node)
		}
		for _, sourceID := range node.SourceIDs {
			if strings.HasPrefix(sourceID, "sql-rdd-1") {
				t.Errorf("Building broad total was combined with local coil source %s", sourceID)
			}
		}
	}
	if count != 1 {
		t.Errorf("Building Annual cooling node cardinality=%d, want 1", count)
	}
	for _, period := range result.Periods {
		if period.Kind != "monthly" {
			continue
		}
		for _, node := range period.Nodes {
			if node.Level == "end_use" && node.EndUse == "cooling" || stringSliceContains(node.SourceIDs, tabularID) {
				t.Errorf("Annual Tabular became a Monthly Building observation in %s: %+v", period.ID, node)
			}
		}
	}
}
