package simulation

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// All quantities are independently chosen kWh, not a snapshot of a builder.
// The five exact families deliberately have unequal values; the electrical
// ancillary consumption of the first fuel coil is a fully observed zero.
var directHVACSQLNames = []string{
	"Cooling Coil Electricity Energy",
	"Cooling Coil Crankcase Heater Electricity Energy",
	"Heating Coil NaturalGas Energy",
	"Heating Coil Ancillary NaturalGas Energy",
	"Heating Coil Electricity Energy",
}

func TestEnergyPathDirectHVACSQLFiveOwnedComponentsRemainAdditive(t *testing.T) {
	doc, originalPath, original := directHVACSQLDocument(t)
	plan := directHVACSQLPlan(doc)
	path := directHVACSQLFixture(t)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	direct := 0
	for _, item := range parsed.Series {
		if item.MeterHierarchyLevel != "zone_direct_use" {
			continue
		}
		direct++
		wantOwner := ""
		for i := 1; i <= 5; i++ {
			zone := fmt.Sprintf("SPACE%d-1", i)
			if item.SourceKey == zone+" PTAC CCoil" || item.SourceKey == zone+" Heating Coil" {
				wantOwner = zone
			}
		}
		if wantOwner == "" || item.ZoneName != wantOwner || item.Unit != "kWh" {
			t.Errorf("component dictionary key was not independently bound to its actual Zone: key=%q zone=%q unit=%q", item.SourceKey, item.ZoneName, item.Unit)
		}
	}
	if direct != 25 {
		t.Fatalf("captured %d direct components, want all 5 families x 5 actual owners", direct)
	}
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.zoneDirectUseSeries) != 25 {
		t.Fatalf("constituent source selection collapsed a main/crankcase/ancillary family: got %d, want25", len(legacy.zoneDirectUseSeries))
	}
	result := UpgradeEnergyExplanationV1(legacy)
	directHVACSQLAssertBuilding(t, result)
	for i := 1; i <= 5; i++ {
		zone := fmt.Sprintf("SPACE%d-1", i)
		var found *EnergyExplanationZoneResult
		for index := range result.ZoneResults {
			if result.ZoneResults[index].Scope.ZoneName == zone {
				found = &result.ZoneResults[index]
			}
		}
		if found == nil {
			t.Fatalf("missing actual Zone %s", zone)
		}
		directHVACSQLAssertZone(t, found.Nodes, found.Links, zone, i, 3)
		for month := 1; month <= 2; month++ {
			period := fmt.Sprintf("M%d", month)
			seen := false
			for _, row := range found.Periods {
				if row.ID == period {
					seen = true
					directHVACSQLAssertZone(t, row.Nodes, row.Links, zone, i, float64(month))
				}
			}
			if !seen {
				t.Errorf("missing directly observed %s/%s", zone, period)
			}
		}
	}
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var resultWire struct {
		Sources []map[string]json.RawMessage `json:"sources"`
	}
	if err := json.Unmarshal(wire, &resultWire); err != nil {
		t.Fatal(err)
	}
	zeroFound := false
	for _, source := range result.Sources {
		if source.ID != "sql-rdd-114" {
			continue
		}
		zeroFound = true
		if source.ZoneName != "SPACE1-1" || !energyDataSourceValueKnown(source, energySourceObservedRaw) || !energyDataSourceValueKnown(source, energySourceObservedEffective) {
			t.Errorf("observed two-month zero lost native scalar proof or Zone ownership: %#v", source)
		}
		wireFound := false
		for _, fields := range resultWire.Sources {
			if string(fields["id"]) != `"sql-rdd-114"` {
				continue
			}
			wireFound = true
			if string(fields["rawValue"]) != "0" || string(fields["effectiveValue"]) != "0" {
				t.Errorf("observed two-month zero became missing/unknown on actual v2 wire: %v", fields)
			}
		}
		if !wireFound {
			t.Error("known-zero source was omitted from actual v2 result wire")
		}
	}
	if !zeroFound {
		t.Error("known-zero component source was pruned with its presentation node")
	}
	after, err := os.ReadFile(originalPath)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("original PTAC input was modified")
	}
}

func TestEnergyPathDirectHVACSQLRequiresExactPlanAndParsedOwnership(t *testing.T) {
	base, _, _ := directHVACSQLDocument(t)
	path := directHVACSQLFixture(t)
	cases := []struct {
		name   string
		mutate func(*idf.Document, **PurposeRunPlan, *energyDriverBuildContext)
		want   int
	}{
		{"no plan", func(_ *idf.Document, plan **PurposeRunPlan, _ *energyDriverBuildContext) { *plan = nil }, 0},
		{"legacy plan", func(_ *idf.Document, plan **PurposeRunPlan, _ *energyDriverBuildContext) {
			(*plan).BasicEnergyDetail = ""
		}, 0},
		{"no executed requests", func(_ *idf.Document, plan **PurposeRunPlan, _ *energyDriverBuildContext) { (*plan).OutputObjects = nil }, 0},
		{"no parsed document", func(_ *idf.Document, _ **PurposeRunPlan, context *energyDriverBuildContext) {
			*context = newEnergyDriverBuildContext(idf.GeometryReport{})
		}, 0},
		{"wrong owner scope", func(_ *idf.Document, plan **PurposeRunPlan, _ *energyDriverBuildContext) {
			for i := range (*plan).OutputObjects {
				(*plan).OutputObjects[i].ScopeZoneName = "PLENUM-1"
			}
		}, 0},
		{"wrong purpose", func(_ *idf.Document, plan **PurposeRunPlan, _ *energyDriverBuildContext) {
			for i := range (*plan).OutputObjects {
				(*plan).OutputObjects[i].PurposeIDs = []SimulationPurposeID{SimulationPurposeZoneHeatFlow}
			}
		}, 0},
		{"wrong frequency", func(_ *idf.Document, plan **PurposeRunPlan, _ *energyDriverBuildContext) {
			for i := range (*plan).OutputObjects {
				(*plan).OutputObjects[i].ReportingFrequency = "Hourly"
			}
		}, 0},
		{"shared cooling component cannot use saved scope", func(doc *idf.Document, _ **PurposeRunPlan, context *energyDriverBuildContext) {
			for i := range doc.Objects {
				object := &doc.Objects[i]
				if object.Type != "ZoneHVAC:PackagedTerminalAirConditioner" || object.Fields[0].Value != "SPACE2-1 PTAC" {
					continue
				}
				for j := range object.Fields {
					if object.Fields[j].Value == "SPACE2-1 PTAC CCoil" {
						object.Fields[j].Value = "SPACE1-1 PTAC CCoil"
					}
				}
			}
			*context = newEnergyDriverBuildContext(idf.AnalyzeGeometry(*doc), *doc)
		}, 15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parsePurposePlanFixture(t, base.String())
			value := directHVACSQLPlan(doc)
			plan := &value
			context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
			tc.mutate(&doc, &plan, &context)
			parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, plan, context)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, item := range parsed.Series {
				if item.MeterHierarchyLevel == "zone_direct_use" {
					count++
					if tc.want == 15 && (item.ZoneName == "SPACE1-1" || item.ZoneName == "SPACE2-1") {
						t.Error("shared or orphaned cooling coil retained direct ownership")
					}
				}
			}
			if count != tc.want {
				t.Errorf("direct observations=%d, want%d", count, tc.want)
			}
		})
	}
	// A forged scoped output for an unknown component and for whole-unit PTAC
	// electricity is present in every plan. Neither is an ownership authority.
}

func TestEnergyPathDirectHVACSQLComponentValuesAreAlreadyModelTotal(t *testing.T) {
	doc, _, _ := directHVACSQLDocument(t)
	path := directHVACSQLFixture(t)
	plan := directHVACSQLPlan(doc)
	read := func(doc idf.Document) EnergyExplanationV1 {
		result, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	baseline := read(doc)
	changed := false
	for i := range doc.Objects {
		object := &doc.Objects[i]
		if object.Type == "Zone" && object.Fields[0].Value == "SPACE1-1" {
			object.Fields[6].Value = "2"
			changed = true
		}
	}
	if !changed {
		t.Fatal("actual SPACE1 Zone not found")
	}
	actual := read(doc)
	if len(actual.zoneDirectUseSeries) != 25 {
		t.Fatalf("lost component series: %d", len(actual.zoneDirectUseSeries))
	}
	for i, item := range actual.zoneDirectUseSeries {
		prior := baseline.zoneDirectUseSeries[i]
		if item.Total != prior.Total || !reflect.DeepEqual(item.Monthly, prior.Monthly) || item.RawTotal != prior.RawTotal {
			t.Errorf("already model-total component was multiplied again: %s %g -> %g", item.SourceKey, prior.Total, item.Total)
		}
	}
}

func TestEnergyPathDirectHVACSQLInvalidObservationCannotBecomeDirectConsumption(t *testing.T) {
	doc, _, _ := directHVACSQLDocument(t)
	plan := directHVACSQLPlan(doc)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	for _, tc := range []struct{ name, query string }{
		{"duplicate month", `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(1,110,36000000)`},
		{"missing month", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=110 AND TimeIndex=2`},
		{"null observation", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=110 AND TimeIndex=2`},
		{"negative consumption", `UPDATE ReportData SET Value=-72000000 WHERE ReportDataDictionaryIndex=110 AND TimeIndex=2`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := directHVACSQLFixture(t)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(tc.query)
			_ = db.Close()
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			direct := 0
			for _, item := range parsed.Series {
				if item.MeterHierarchyLevel != "zone_direct_use" {
					continue
				}
				direct++
				if stringSliceContains(item.SourceIDs, "sql-rdd-110") || stringSliceContains(item.SourceIDs, "sql-rdd-111") {
					t.Error("incomplete main+crankcase cohort was silently promoted to direct cooling consumption")
				}
			}
			if direct != 23 {
				t.Errorf("only the incomplete cooling cohort must be excluded: count%d want23", direct)
			}
			found := false
			validSibling := false
			for _, source := range parsed.Sources {
				if source.ID == "sql-rdd-111" {
					validSibling = true
					if !energyDataSourceValueKnown(source, energySourceObservedRaw) || source.RawValue != 3 {
						t.Error("valid crankcase observation must remain known context, not become missing/zero")
					}
				}
				if source.ID != "sql-rdd-110" {
					continue
				}
				found = true
				if energyDataSourceValueKnown(source, energySourceObservedRaw) || energyDataSourceValueKnown(source, energySourceObservedEffective) {
					t.Error("invalid component retained a known annual value")
				}
			}
			if !found {
				t.Error("invalid original source metadata was discarded instead of retained unknown")
			}
			if !validSibling {
				t.Error("known sibling context was discarded with incomplete cohort")
			}
			legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range legacy.zoneDirectUseSeries {
				if stringSliceContains(item.SourceIDs, "sql-rdd-110") || stringSliceContains(item.SourceIDs, "sql-rdd-111") {
					t.Error("incomplete cohort reappeared as direct annual graph evidence")
				}
			}
			result := UpgradeEnergyExplanationV1(legacy)
			directHVACSQLAssertBuilding(t, result)
			selectedSeen := false
			for _, zone := range result.ZoneResults {
				if zone.Scope.ZoneName != "SPACE1-1" {
					for i := 2; i <= 5; i++ {
						if zone.Scope.ZoneName == fmt.Sprintf("SPACE%d-1", i) {
							directHVACSQLAssertZone(t, zone.Nodes, zone.Links, zone.Scope.ZoneName, i, 3)
						}
					}
					continue
				}
				selectedSeen = true
				check := func(nodes []EnergyExplanationNode, links []EnergyPathLink) {
					for _, node := range nodes {
						if node.Level == "end_use" && node.EndUse == "cooling" && node.Basis == "direct_zone_energy" {
							t.Errorf("incomplete main observation is hidden by a %g direct cooling subtotal", node.Value)
						}
					}
					for _, link := range links {
						if link.Relation == "load_to_end_use" && link.ServiceKind == "cooling" && link.Basis == "direct_zone_energy" {
							t.Error("crankcase-only cohort falsely covers the entire Zone cooling load")
						}
					}
				}
				check(zone.Nodes, zone.Links)
				for _, period := range zone.Periods {
					check(period.Nodes, period.Links)
				}
			}
			if !selectedSeen {
				t.Error("missing direct consumption must not discard the Zone's observed loads")
			}
		})
	}
}

func directHVACSQLDocument(t *testing.T) (idf.Document, string, []byte) {
	t.Helper()
	path := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "DOAToPTAC.idf")
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return parsePurposePlanFixture(t, string(input)), path, input
}

func directHVACSQLPlan(doc idf.Document) PurposeRunPlan {
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	plan.AllocationPolicy = PurposeAllocationPolicyDirectOnly
	for _, pair := range [][2]string{{"ALIEN CCoil", directHVACSQLNames[0]}, {"SPACE1-1 PTAC", "Zone Packaged Terminal Air Conditioner Electricity Energy"}} {
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", KeyValue: pair[0], VariableName: pair[1], ReportingFrequency: "Monthly", ScopeZoneName: "SPACE1-1", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	}
	return plan
}

func directHVACSQLFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	epath094AuditCreateZoneDirectSQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{
		`DELETE FROM ReportData`, `DELETE FROM ReportDataDictionary`,
		`ALTER TABLE "Time" ADD COLUMN Year INTEGER`, `ALTER TABLE "Time" ADD COLUMN "Interval" REAL`,
		`ALTER TABLE "Time" ADD COLUMN IntervalType INTEGER`, `ALTER TABLE "Time" ADD COLUMN EnvironmentPeriodIndex INTEGER`,
		`ALTER TABLE "Time" ADD COLUMN WarmupFlag INTEGER`,
		`CREATE TABLE EnvironmentPeriods (EnvironmentPeriodIndex INTEGER PRIMARY KEY, EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(3,3)`,
		`UPDATE "Time" SET Year=2017,"Interval"=CASE Month WHEN 1 THEN 44640 ELSE 40320 END,IntervalType=3,EnvironmentPeriodIndex=3,WarmupFlag=NULL`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	add := func(id int, key, name string, meter bool, first float64) {
		t.Helper()
		group, flag := "HVAC", 0
		if meter {
			group, flag = "Meter", 1
		}
		if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,'J',?,'Monthly',?)`, id, key, name, flag, group); err != nil {
			t.Fatal(err)
		}
		for month := 1; month <= 2; month++ {
			if _, err := db.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`, month, id, first*float64(month)*3600000); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, row := range []struct {
		id    int
		name  string
		value float64
	}{
		{1, "Electricity:Facility", 225}, {2, "NaturalGas:Facility", 330}, {3, "Cooling:Electricity", 165},
		{4, "Heating:NaturalGas", 330}, {5, "Heating:Electricity", 30}, {6, "Fans:Electricity", 30},
	} {
		add(row.id, "", row.name, true, row.value)
	}
	for i := 1; i <= 5; i++ {
		zone := fmt.Sprintf("SPACE%d-1", i)
		values := []float64{float64(10 * i), float64(i), float64(20 * i), float64(2 * i), float64(3 * (i - 1))}
		for family, name := range directHVACSQLNames {
			key := zone + " Heating Coil"
			if family < 2 {
				key = zone + " PTAC CCoil"
			}
			add(100+i*10+family, key, name, false, values[family])
		}
		add(200+i*2, zone, "Zone Air System Sensible Cooling Energy", false, float64(100*i))
		add(201+i*2, zone, "Zone Air System Sensible Heating Energy", false, float64(200*i))
	}
	add(900, "ALIEN CCoil", directHVACSQLNames[0], false, 7000)
	add(901, "SPACE1-1 PTAC", "Zone Packaged Terminal Air Conditioner Electricity Energy", false, 9000)
	return path
}

func directHVACSQLAssertBuilding(t *testing.T, result EnergyExplanationResult) {
	t.Helper()
	for _, want := range []struct {
		level, category string
		value           float64
	}{
		{"carrier", "electricity", 675}, {"carrier", "natural_gas", 990},
		{"end_use", "cooling", 495}, {"end_use", "heating", 1080}, {"end_use", "fans", 90},
	} {
		found := 0
		for _, node := range result.Nodes {
			if node.Level != want.level || want.level == "carrier" && node.Carrier != want.category || want.level == "end_use" && node.EndUse != want.category {
				continue
			}
			found++
			if node.Value != want.value {
				t.Errorf("Building %s/%s=%g, want%g (direct observations must not be added to facility meters)", want.level, want.category, node.Value, want.value)
			}
			for _, id := range node.SourceIDs {
				if strings.HasPrefix(id, "sql-rdd-1") && id != "sql-rdd-1" {
					t.Errorf("Building meter acquired component source %s", id)
				}
			}
		}
		if found != 1 {
			t.Errorf("Building %s/%s cardinality%d, want1", want.level, want.category, found)
		}
	}
	for carrier, total := range map[string]float64{"electricity": 675, "natural_gas": 990} {
		row := epath094AuditCarrierReconciliation(result.Reconciliation, carrier)
		if row == nil || row.ExpectedValue != total || row.ExplainedValue != total || row.ResidualValue != 0 {
			t.Errorf("Building %s independent meter closure changed after direct component capture: %#v", carrier, row)
		}
	}
}

func directHVACSQLAssertZone(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink, zone string, owner int, factor float64) {
	t.Helper()
	cooling, gas, electric := float64(owner*11)*factor, float64(owner*22)*factor, float64((owner-1)*3)*factor
	for _, want := range []struct {
		level, category string
		value           float64
	}{
		{"end_use", "cooling", cooling}, {"end_use", "heating", gas + electric},
		{"carrier", "electricity", cooling + electric}, {"carrier", "natural_gas", gas},
	} {
		count := 0
		for _, node := range nodes {
			if node.Level != want.level || want.level == "carrier" && node.Carrier != want.category || want.level == "end_use" && node.EndUse != want.category {
				continue
			}
			count++
			if node.Value != want.value || node.ZoneName != zone || node.Basis != "direct_zone_energy" || node.Unit != "kWh" {
				t.Errorf("%s %s/%s got value%g basis%s owner%s, want%g direct_zone_energy", zone, want.level, want.category, node.Value, node.Basis, node.ZoneName, want.value)
			}
			for _, source := range node.SourceIDs {
				valid := false
				for family := range directHVACSQLNames {
					if source == fmt.Sprintf("sql-rdd-%d", 100+owner*10+family) {
						valid = true
					}
				}
				if !valid {
					t.Errorf("%s direct subtotal claims facility/another owner/whole-unit source %s", zone, source)
				}
			}
		}
		if count != 1 {
			t.Errorf("%s %s/%s cardinality%d, want1", zone, want.level, want.category, count)
		}
	}
	conversions := 0
	for _, link := range links {
		if link.Relation != "load_to_end_use" {
			continue
		}
		conversions++
		thermal, site := float64(owner*100)*factor, cooling
		if link.ServiceKind == "heating" {
			thermal, site = float64(owner*200)*factor, gas+electric
		}
		if link.FromValue != thermal || link.ToValue != site || link.Basis != "direct_zone_energy" || link.ZoneName != zone || link.FromUnit != "kWh" || link.ToUnit != "kWh" {
			t.Errorf("%s direct conversion got %g->%g basis%s, want%g->%g", zone, link.FromValue, link.ToValue, link.Basis, thermal, site)
		}
	}
	if conversions != 2 {
		t.Errorf("%s has%d direct service conversions, want2", zone, conversions)
	}
}
