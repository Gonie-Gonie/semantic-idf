package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Seven independently chosen positive monthly kWh coefficients. In particular,
// the identically named DX and Fuel electricity measurements have 13 vs31.
var pthpDirectCoefficients = []float64{11, 13, 17, 19, 23, 29, 31}

func pthpDirectSQLFixture(t *testing.T) string {
	t.Helper()
	path := directHVACSQLFixture(t) // Reuse only the existing small SQLite schema.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// This is one test-fixture transaction, not a production SQL change. Avoid
	// hundreds of durable journal commits for these independently chosen rows.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, query := range []string{`DELETE FROM ReportData`, `DELETE FROM ReportDataDictionary`, `DELETE FROM "Time"`} {
		if _, err := tx.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	for month := 1; month <= 12; month++ {
		days := time.Date(2017, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if _, err := tx.Exec(`INSERT INTO "Time" (TimeIndex,Month,Day,Hour,Minute,Year,"Interval",IntervalType,EnvironmentPeriodIndex,WarmupFlag) VALUES(?,?,?,24,0,2017,?,3,3,NULL)`, month, month, days, days*1440); err != nil {
			t.Fatal(err)
		}
	}
	add := func(index int, key, name string, meter bool, coefficient float64) {
		t.Helper()
		flag, group := 0, "HVAC"
		if meter {
			flag, group = 1, "Meter"
		}
		if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,'J',?,'Monthly',?)`, index, key, name, flag, group); err != nil {
			t.Fatal(err)
		}
		for month := 1; month <= 12; month++ {
			if _, err := tx.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`, month, index, coefficient*float64(month)*3600000); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Monthly electricity1415 = cooling165 + heating1200 + fans50. Gas780
	// equals the five supplemental primary+ancillary consumptions; no doubling.
	for _, row := range []struct {
		id    int
		name  string
		value float64
	}{
		{1, "Electricity:Facility", 1415}, {2, "NaturalGas:Facility", 780}, {3, "Cooling:Electricity", 165},
		{4, "Heating:NaturalGas", 780}, {5, "Heating:Electricity", 1200}, {6, "Fans:Electricity", 50},
	} {
		add(row.id, "", row.name, true, row.value)
	}
	for owner := 1; owner <= 5; owner++ {
		zone := fmt.Sprintf("SPACE%d-1", owner)
		for role, item := range pthpDirectRoles {
			add(1000+owner*10+role, zone+item.suffix, item.name, false, float64(owner)*pthpDirectCoefficients[role])
		}
		add(2000+owner*2, zone, "Zone Air System Sensible Cooling Energy", false, float64(owner)*100)
		add(2001+owner*2, zone, "Zone Air System Sensible Heating Energy", false, float64(owner)*200)
	}
	add(2090, "PLENUM-1", "Zone Air System Sensible Cooling Energy", false, 0)
	add(2091, "PLENUM-1", "Zone Air System Sensible Heating Energy", false, 0)
	add(9000, "ALIEN HP Heating Mode", "Heating Coil Electricity Energy", false, 10000)
	add(9001, "SPACE1-1 Heat Pump", "Zone Packaged Terminal Heat Pump Electricity Energy", false, 11000)
	add(9002, "SPACE1-1 HP Cooling Mode", "Cooling Coil Crankcase Heater Electricity Energy", false, 12000)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path
}

func pthpDirectSQLPlan(doc idf.Document) PurposeRunPlan {
	plan := pthpDirectPlan(doc, SimulationPurposeScope{})
	for index := range plan.OutputObjects {
		value := 10000 + index
		plan.OutputObjects[index].ObjectIndex = &value
	}
	// Forged saved Scope cannot turn a package, unsupported PTHP role, or unknown
	// component into an exclusively owned native constituent.
	for _, pair := range [][2]string{{"ALIEN HP Heating Mode", "Heating Coil Electricity Energy"}, {"SPACE1-1 Heat Pump", "Zone Packaged Terminal Heat Pump Electricity Energy"}, {"SPACE1-1 HP Cooling Mode", "Cooling Coil Crankcase Heater Electricity Energy"}} {
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", KeyValue: pair[0], VariableName: pair[1], ReportingFrequency: "Monthly", ScopeZoneName: "SPACE1-1", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	}
	return plan
}

func pthpDirectRead(t *testing.T, doc idf.Document, path string, plan *PurposeRunPlan) EnergyExplanationV1 {
	t.Helper()
	result, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestEnergyPathDirectHVACPTHPSQLSevenRolesAndWholeSiteTotals(t *testing.T) {
	doc := pthpDirectDocument(t)
	before := doc.String()
	path := pthpDirectSQLFixture(t)
	plan := pthpDirectSQLPlan(doc)
	legacy := pthpDirectRead(t, doc, path, &plan)
	if len(legacy.zoneDirectUseSeries) != 35 {
		t.Fatalf("direct series%d want35", len(legacy.zoneDirectUseSeries))
	}
	seen := map[string]bool{}
	for _, item := range legacy.zoneDirectUseSeries {
		matched := false
		for owner := 1; owner <= 5; owner++ {
			zone := fmt.Sprintf("SPACE%d-1", owner)
			for role, definition := range pthpDirectRoles {
				if item.ZoneName != zone || item.directComponentID != definition.id {
					continue
				}
				matched = true
				key := zone + "/" + definition.id
				if seen[key] {
					t.Errorf("duplicate typed constituent%s", key)
				}
				seen[key] = true
				want := float64(owner) * pthpDirectCoefficients[role]
				if item.SourceKey != zone+definition.suffix || item.Total != 78*want || item.RawTotal != 78*want || item.EffectiveMultiplier != 1 || item.Unit != "kWh" ||
					!reflect.DeepEqual(item.SourceIDs, []string{fmt.Sprintf("sql-rdd-%d", 1000+owner*10+role)}) {
					t.Errorf("wrong typed source or model-total sum %s: %#v", key, item)
				}
				for month := 1; month <= 12; month++ {
					if item.Monthly[month] != float64(month)*want {
						t.Errorf("%s M%d wrong observed amount", key, month)
					}
				}
			}
		}
		if !matched {
			t.Errorf("unsupported/package/unknown source became direct: %#v", item)
		}
	}
	result := UpgradeEnergyExplanationV1(legacy)
	for _, want := range []struct {
		level, category string
		value           float64
	}{{"carrier", "electricity", 1415 * 78}, {"carrier", "natural_gas", 780 * 78}, {"end_use", "cooling", 165 * 78}, {"end_use", "heating", 1980 * 78}, {"end_use", "fans", 50 * 78}} {
		count := 0
		for _, node := range result.Nodes {
			if node.Level != want.level || want.level == "carrier" && node.Carrier != want.category || want.level == "end_use" && node.EndUse != want.category {
				continue
			}
			count++
			if node.Value != want.value {
				t.Errorf("Building%s/%s=%g want%g; components must not duplicate central meters", want.level, want.category, node.Value, want.value)
			}
		}
		if count != 1 {
			t.Errorf("Building%s/%s count%d", want.level, want.category, count)
		}
	}
	for owner := 1; owner <= 5; owner++ {
		zone := fmt.Sprintf("SPACE%d-1", owner)
		found := false
		for _, view := range result.ZoneResults {
			if view.Scope.ZoneName != zone {
				continue
			}
			found = true
			pthpDirectAssertView(t, view.Nodes, view.Links, zone, owner, 78)
			months := 0
			for _, period := range view.Periods {
				var month int
				if _, err := fmt.Sscanf(period.ID, "M%d", &month); err == nil && month >= 1 && month <= 12 {
					months++
					pthpDirectAssertView(t, period.Nodes, period.Links, zone, owner, float64(month))
				}
			}
			if months != 12 {
				t.Errorf("%s observed months%d want12", zone, months)
			}
		}
		if !found {
			t.Errorf("missing original owner%s", zone)
		}
	}
	for _, source := range result.Sources {
		var index int
		if _, err := fmt.Sscanf(source.ID, "sql-rdd-%d", &index); err != nil || index < 1010 || index > 1056 {
			continue
		}
		matched := false
		for _, output := range plan.OutputObjects {
			if output.KeyValue == source.KeyValue && output.VariableName == source.Name {
				matched = true
				if source.ObjectIndex == nil || output.ObjectIndex == nil || *source.ObjectIndex != *output.ObjectIndex {
					t.Errorf("same-name DX/Fuel lost exact request ObjectIndex: %#v", source)
				}
			}
		}
		if !matched || source.SourceUnit != "J" || source.NormalizedUnit != "kWh" || source.ReportingFrequency != "Monthly" || source.EffectiveMultiplier != 1 {
			t.Errorf("original source identity/units/factor lost: %#v", source)
		}
	}
	if before != doc.String() {
		t.Fatal("original PTHP document mutated")
	}
}

func pthpDirectAssertView(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink, zone string, owner int, factor float64) {
	t.Helper()
	for _, want := range []struct {
		level, category string
		value           float64
	}{{"end_use", "cooling", 11}, {"end_use", "heating", 132}, {"carrier", "electricity", 91}, {"carrier", "natural_gas", 52}} {
		count := 0
		for _, node := range nodes {
			if node.Level != want.level || want.level == "carrier" && node.Carrier != want.category || want.level == "end_use" && node.EndUse != want.category {
				continue
			}
			count++
			if node.Value != want.value*float64(owner)*factor || node.Basis != "direct_zone_energy" || node.ZoneName != zone {
				t.Errorf("%s %s/%s value%g basis%s, want%g direct", zone, want.level, want.category, node.Value, node.Basis, want.value*float64(owner)*factor)
			}
			for _, source := range node.SourceIDs {
				valid := false
				for role := range pthpDirectRoles {
					valid = valid || source == fmt.Sprintf("sql-rdd-%d", 1000+owner*10+role)
				}
				if !valid {
					t.Errorf("%s direct aggregate claims unrelated source%s", zone, source)
				}
			}
		}
		if count != 1 {
			t.Errorf("%s %s/%s count%d", zone, want.level, want.category, count)
		}
	}
	conversions := 0
	for _, link := range links {
		if link.Relation != "load_to_end_use" {
			continue
		}
		conversions++
		thermal, site, kind := 100.0, 11.0, "coefficient_of_performance"
		if link.ServiceKind == "heating" {
			thermal, site, kind = 200, 132, "load_to_site_energy"
		}
		if link.FromValue != thermal*float64(owner)*factor || link.ToValue != site*float64(owner)*factor || link.RatioKind != kind || link.Basis != "direct_zone_energy" {
			t.Errorf("%s invalid mixed-carrier pairing/kind: %#v", zone, link)
		}
		if link.ServiceKind == "heating" {
			for role := 1; role < 7; role++ {
				if !stringSliceContains(link.SourceIDs, fmt.Sprintf("sql-rdd-%d", 1000+owner*10+role)) {
					t.Errorf("%s heating conversion omitted constituent%d", zone, role)
				}
			}
		}
	}
	if conversions != 2 {
		t.Errorf("%s conversions%d want2", zone, conversions)
	}
}

func TestEnergyPathDirectHVACPTHPSQLKnownZeroAndModelTotal(t *testing.T) {
	doc := pthpDirectDocument(t)
	path := pthpDirectSQLFixture(t)
	plan := pthpDirectSQLPlan(doc)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=1016`)
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	baseline := pthpDirectRead(t, doc, path, &plan)
	if len(baseline.zoneDirectUseSeries) != 35 {
		t.Fatal("known-zero Fuel electricity invalidated complete heating cohort")
	}
	result := UpgradeEnergyExplanationV1(baseline)
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Sources []map[string]json.RawMessage `json:"sources"`
	}
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, source := range decoded.Sources {
		if string(source["id"]) != `"sql-rdd-1016"` {
			continue
		}
		found = true
		if string(source["rawValue"]) != "0" || string(source["effectiveValue"]) != "0" || string(source["zoneName"]) != `"SPACE1-1"` {
			t.Errorf("reported0 became unknown on actual wire: %v", source)
		}
	}
	if !found {
		t.Fatal("known-zero original source disappeared")
	}
	directHVACFixtureObject(t, &doc, "Zone", "SPACE1-1").Fields[6].Value = "7"
	changed := pthpDirectRead(t, doc, path, &plan)
	if len(changed.zoneDirectUseSeries) != 35 {
		t.Fatal("changed Zone multiplier discarded direct components")
	}
	for i, item := range changed.zoneDirectUseSeries {
		prior := baseline.zoneDirectUseSeries[i]
		if item.Total != prior.Total || item.RawTotal != prior.RawTotal || !reflect.DeepEqual(item.Monthly, prior.Monthly) || item.EffectiveMultiplier != 1 {
			t.Errorf("model-total component multiplied twice: %s", item.SourceKey)
		}
	}
}

func TestEnergyPathDirectHVACPTHPSQLMissingMonthlyRejectsWholeElectricCohort(t *testing.T) {
	doc := pthpDirectDocument(t)
	plan := pthpDirectSQLPlan(doc)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	for _, test := range []struct{ name, query string }{
		{"missing DX month", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=1011 AND TimeIndex=7`},
		{"null DX month", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=1011 AND TimeIndex=7`},
		{"duplicate DX month", `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(7,1011,327600000)`},
		{"negative DX month", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=1011 AND TimeIndex=7`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := pthpDirectSQLFixture(t)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(test.query)
			_ = db.Close()
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, item := range parsed.Series {
				if item.MeterHierarchyLevel != "zone_direct_use" {
					continue
				}
				count++
				if item.ZoneName == "SPACE1-1" && item.EndUse == "heating" && item.Carrier == "electricity" {
					t.Errorf("incomplete DX/Fuel electrical cohort promoted: %#v", item)
				}
			}
			if count != 31 {
				t.Errorf("direct series%d want31 (only4-member heating/electricity cohort excluded)", count)
			}
			unknown, sibling := false, false
			for _, source := range parsed.Sources {
				if source.ID == "sql-rdd-1011" {
					unknown = true
					if energyDataSourceValueKnown(source, energySourceObservedRaw) || energyDataSourceValueKnown(source, energySourceObservedEffective) {
						t.Error("incomplete source became known annual amount")
					}
				}
				if source.ID == "sql-rdd-1016" {
					sibling = true
					if source.RawValue != 31*78 || !energyDataSourceValueKnown(source, energySourceObservedRaw) {
						t.Error("valid Fuel electrical sibling no longer retained as observed context")
					}
				}
			}
			if !unknown || !sibling {
				t.Error("invalid metadata or valid sibling was dropped instead of preserved honestly")
			}
		})
	}
}

func TestEnergyPathDirectHVACPTHPSQLScopedRequestCannotLaunderSharedOwner(t *testing.T) {
	doc := pthpDirectDocument(t)
	path := pthpDirectSQLFixture(t)
	plan := pthpDirectSQLPlan(doc)
	parent := directHVACFixtureObject(t, &doc, "ZoneHVAC:PackagedTerminalHeatPump", "SPACE2-1 Heat Pump")
	for i := range parent.Fields {
		if strings.EqualFold(parent.Fields[i].Value, "SPACE2-1 HP Heating Mode") {
			parent.Fields[i].Value = "SPACE1-1 HP Heating Mode"
		}
	}
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range parsed.Series {
		if item.MeterHierarchyLevel != "zone_direct_use" {
			continue
		}
		count++
		if item.ZoneName == "SPACE1-1" || item.ZoneName == "SPACE2-1" {
			t.Errorf("saved correct-looking Scope revived shared parent: %#v", item)
		}
	}
	if count != 21 {
		t.Errorf("direct series%d want21 unaffected owners", count)
	}
}
