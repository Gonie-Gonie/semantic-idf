package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// The actual SQL surface output is signed toward the surface, opposite to
// the zone-air driver. Winter positive surface convection therefore supplies
// heating pressure; summer negative convection supplies cooling pressure.
// Netting the twelve source observations first would erase both contributions.
func TestEPATH192WallContributionsAggregateTwelveMonthsBeforeAnnual(t *testing.T) {
	doc, err := idf.Parse(`Version, 24.2;
Zone, Office, 0, 0, 0, 0, 1, 1;
Material:NoMass, Insulation, Rough, 2;
Construction, Wall Construction, Insulation;
BuildingSurface:Detailed, Seasonal Wall, Wall, Wall Construction, Office, , Outdoors, , SunExposed, WindExposed, 0.5, 4,
  0,0,0, 0,0,3, 4,0,3, 4,0,0;
`)
	if err != nil {
		t.Fatal(err)
	}
	geometry := idf.AnalyzeGeometry(doc)
	if len(geometry.Surfaces) != 1 || geometry.Surfaces[0].Name != "Seasonal Wall" || len(geometry.Topology.Boundaries) != 1 {
		t.Fatalf("actual parsed wall geometry/topology missing: %#v", geometry)
	}
	monthlyRaw := [12]float64{12, 10, 6, 2, -2, -6, -12, -10, -6, -2, 2, 6}
	path := epath192SeasonalWallSQL(t, monthlyRaw)
	context := newEnergyDriverBuildContext(geometry, doc)
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, plan, context)
	if err != nil {
		t.Fatal(err)
	}
	physicalSources := 0
	for _, item := range parsed.Series {
		if !item.parseCategorySource || !stringSliceContains(item.SourceIDs, "sql-rdd-1") {
			continue
		}
		physicalSources++
		if item.Total != 0 || len(item.Monthly) != 12 {
			t.Fatalf("canonical wall source lost zero net or twelve months: %#v", item)
		}
		for month, want := range monthlyRaw {
			if value, exists := item.Monthly[month+1]; !exists || value != want {
				t.Fatalf("raw wall M%d = %g (present %v), want %g", month+1, value, exists, want)
			}
		}
	}
	if physicalSources != 1 {
		t.Fatalf("canonical SQL source count = %d, want one physical wall", physicalSources)
	}
	legacy := buildEnergyExplanationResultWithDriverContext(parsed.Series, parsed.Sources, plan, context)
	for label, sources := range map[string][]EnergyDataSource{"parsed SQL": parsed.Sources, "before V2 upgrade": legacy.Sources} {
		source := energyExplanationSourceByID(sources, "sql-rdd-1")
		if source == nil || source.RawValue != 0 || source.EffectiveValue != 0 {
			t.Fatalf("%s wall raw/effective net must be zero: %#v", label, source)
		}
	}
	result := UpgradeEnergyExplanationV1(legacy)
	for _, phase := range []string{"generated", "first JSON reload", "second JSON reload"} {
		if phase != "generated" {
			result = epath120Reload(t, result)
		}
		t.Run(phase, func(t *testing.T) {
			epath192AssertWireAccounting(t, result)
			source := energyExplanationSourceByID(result.Sources, "sql-rdd-1")
			if source == nil {
				t.Fatal("annual physical wall source missing")
			}
			if source.KeyValue != "Seasonal Wall" || source.RawValue != 0 || source.EffectiveValue != 0 || source.SourceUnit != "J" || source.NormalizedUnit != "kWh" || source.DriverCategory != energyDriverCategoryExteriorWalls {
				t.Errorf("V2 annual wall raw/effective = %g/%g, want zero net; identity/unit/category = %q/%q/%q/%q", source.RawValue, source.EffectiveValue, source.KeyValue, source.SourceUnit, source.NormalizedUnit, source.DriverCategory)
			}
			if !source.AllocationApplied || source.AllocatedValue != 76 {
				t.Errorf("annual wall allocated contribution = %g (applied %v), want heating38 + cooling38 independently of raw net0", source.AllocatedValue, source.AllocationApplied)
			}
			officeDetails := 0
			for _, detail := range source.ScopeDetails {
				if detail.Scope.Kind != "zone" || detail.Scope.ZoneName != "Office" {
					continue
				}
				officeDetails++
				if detail.RawValue != 0 || detail.EffectiveValue != 0 || detail.AllocatedValue != 76 || !detail.AllocationApplied {
					t.Errorf("Office source accounting = raw/effective/allocated %g/%g/%g (applied %v), want 0/0/76", detail.RawValue, detail.EffectiveValue, detail.AllocatedValue, detail.AllocationApplied)
				}
			}
			if officeDetails != 1 {
				t.Errorf("Office source scope-detail count = %d, want exactly one annual record", officeDetails)
			}
			if !stringSliceContains(source.RelatedEntityIDs, geometry.Surfaces[0].ID) {
				t.Fatalf("wall source lost actual geometry linkage: %#v", source.RelatedEntityIDs)
			}
			epath192AssertScope(t, "building", result.Nodes, result.Links, result.Periods, monthlyRaw)
			office := review080ZoneResult(t, result.ZoneResults, "Office")
			epath192AssertScope(t, "Office", office.Nodes, office.Links, office.Periods, monthlyRaw)
		})
	}
}

func epath192AssertWireAccounting(t *testing.T, result EnergyExplanationResult) {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Sources []map[string]json.RawMessage `json:"sources"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	assertValues := func(label string, fields map[string]json.RawMessage) {
		for key, want := range map[string]string{"rawValue": "0", "effectiveValue": "0", "allocatedValue": "76"} {
			if got, exists := fields[key]; !exists || string(got) != want {
				t.Errorf("%s wire %s = %s (present %v), want explicit numeric %s, not omitted/unknown", label, key, got, exists, want)
			}
		}
	}
	for _, source := range wire.Sources {
		if string(source["id"]) != `"sql-rdd-1"` {
			continue
		}
		assertValues("annual source", source)
		var details []map[string]json.RawMessage
		if err := json.Unmarshal(source["scopeDetails"], &details); err != nil {
			t.Fatal(err)
		}
		for _, detail := range details {
			var scope EnergyExplanationScope
			if err := json.Unmarshal(detail["scope"], &scope); err != nil {
				t.Fatal(err)
			}
			if scope.Kind == "zone" && scope.ZoneName == "Office" {
				assertValues("Office source", detail)
			}
		}
		return
	}
	t.Fatal("canonical JSON omitted the physical wall source")
}

func epath192SeasonalWallSQL(t *testing.T, monthlyRaw [12]float64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT, IsMeter INTEGER, ReportingFrequency TEXT, IndexGroup TEXT)`,
		`CREATE TABLE "Time" (TimeIndex INTEGER PRIMARY KEY, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER)`,
		`CREATE TABLE ReportData (ReportDataIndex INTEGER PRIMARY KEY, TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		`INSERT INTO ReportDataDictionary VALUES
		 (1, 'Seasonal Wall', 'Surface Inside Face Convection Heat Gain Energy', 'J', 0, 'Monthly', 'Surface'),
		 (2, 'Office', 'Zone Air System Sensible Heating Energy', 'J', 0, 'Monthly', 'Zone'),
		 (3, 'Office', 'Zone Air System Sensible Cooling Energy', 'J', 0, 'Monthly', 'Zone')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for index, raw := range monthlyRaw {
		month := index + 1
		lastDay := time.Date(2023, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
		if _, err := db.Exec(`INSERT INTO "Time" VALUES (?, ?, ?, 24, 0)`, month, month, lastDay); err != nil {
			t.Fatal(err)
		}
		heating, cooling := 0.0, 0.0
		if raw > 0 {
			heating = raw
		} else {
			cooling = -raw
		}
		for dictionary, value := range []float64{raw, heating, cooling} {
			if _, err := db.Exec(`INSERT INTO ReportData VALUES (?, ?, ?, ?)`, index*3+dictionary+1, month, dictionary+1, value*3_600_000); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func epath192AssertScope(t *testing.T, scope string, nodes []EnergyExplanationNode, links []EnergyPathLink, periods []EnergyPeriod, monthlyRaw [12]float64) {
	t.Helper()
	months := map[string]EnergyPeriod{}
	for _, period := range periods {
		if period.Kind != "monthly" {
			continue
		}
		if _, duplicate := months[period.ID]; duplicate {
			t.Fatalf("%s duplicates monthly wrapper %s", scope, period.ID)
		}
		months[period.ID] = period
	}
	if len(months) != 12 {
		t.Fatalf("%s has %d actual months, want 12", scope, len(months))
	}
	for _, service := range []string{"cooling", "heating"} {
		annualNode, annualLink := epath192Contribution(t, scope+" annual", nodes, links, service, 38)
		if annualNode.RawValue != 38 || annualNode.EffectiveValue != 38 {
			t.Fatalf("%s %s directional pressure = raw/effective %g/%g, want 38/38", scope, service, annualNode.RawValue, annualNode.EffectiveValue)
		}
		nodeSum, fromSum, toSum := 0.0, 0.0, 0.0
		for index, raw := range monthlyRaw {
			id := fmt.Sprintf("M%d", index+1)
			period, exists := months[id]
			if !exists {
				t.Fatalf("%s missing actual %s", scope, id)
			}
			want := 0.0
			if service == "heating" && raw > 0 {
				want = raw
			} else if service == "cooling" && raw < 0 {
				want = -raw
			}
			node, link := epath192Contribution(t, scope+" "+id, period.Nodes, period.Links, service, want)
			if node != nil {
				nodeSum += node.Value
			}
			if link != nil {
				fromSum += link.FromValue
				toSum += link.ToValue
			}
		}
		if annualNode.Value != nodeSum || annualLink.FromValue != fromSum || annualLink.ToValue != toSum || nodeSum != 38 || fromSum != 38 || toSum != 38 {
			t.Fatalf("%s %s annual node/link %g/%g/%g != twelve-month sums %g/%g/%g", scope, service, annualNode.Value, annualLink.FromValue, annualLink.ToValue, nodeSum, fromSum, toSum)
		}
	}
}

func epath192Contribution(t *testing.T, context string, nodes []EnergyExplanationNode, links []EnergyPathLink, service string, want float64) (*EnergyExplanationNode, *EnergyPathLink) {
	t.Helper()
	var driver *EnergyExplanationNode
	for index := range nodes {
		node := &nodes[index]
		if node.Level != "driver" || node.DriverCategory != energyDriverCategoryExteriorWalls || node.ServiceKind != service {
			continue
		}
		if driver != nil {
			t.Fatalf("%s %s has duplicate wall contribution nodes", context, service)
		}
		driver = node
	}
	if want == 0 && driver == nil {
		for _, link := range links {
			if link.Relation == "driver_to_load" && link.ServiceKind == service && stringSliceContains(link.SourceIDs, "sql-rdd-1") {
				t.Fatalf("%s %s zero month retains an orphan wall contribution link: %#v", context, service, link)
			}
		}
		return nil, nil // Explicit zero-load months may prune zero-width nodes.
	}
	if driver == nil || driver.Value != want || driver.AllocatedValue != want || !driver.AllocationApplied || !stringSliceContains(driver.SourceIDs, "sql-rdd-1") {
		t.Fatalf("%s %s wall contribution = %#v, want %g", context, service, driver, want)
	}
	var contribution *EnergyPathLink
	for index := range links {
		link := &links[index]
		if link.Relation != "driver_to_load" || link.FromID != driver.ID {
			continue
		}
		if contribution != nil {
			t.Fatalf("%s %s has duplicate wall contribution links", context, service)
		}
		contribution = link
	}
	if want == 0 && contribution == nil {
		return driver, nil
	}
	load := (*EnergyExplanationNode)(nil)
	if contribution != nil {
		load = energyPathV2NodeByID(nodes, contribution.ToID)
	}
	if contribution == nil || contribution.FromValue != want || contribution.ToValue != want || contribution.ServiceKind != service || contribution.Basis != "heat_balance_share" || contribution.FromUnit != driver.Unit || contribution.ToUnit != driver.Unit || (driver.Unit != "kWh" && driver.Unit != "kWh thermal") || driver.ScaleDomain != "thermal" || !stringSliceContains(contribution.SourceIDs, "sql-rdd-1") || load == nil || load.Level != "load" || load.ServiceKind != service || load.Value != want || load.ScaleDomain != "thermal" || load.Unit != driver.Unit {
		t.Fatalf("%s %s wall-to-load contribution = %#v / load %#v, want %g", context, service, contribution, load, want)
	}
	return driver, contribution
}
