package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Hand observations, not an acceptance oracle or a production snapshot. Four
// independent native packages have representative Zone factors 1/4/8/32;
// component electricity is already model total and must remain factor one.
func windowACSQLFixture(t *testing.T) (idf.Document, string, PurposeRunPlan) {
	t.Helper()
	doc := windowACHandDocument(t)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare})
	path := directHVACSQLFixture(t)
	if err := os.WriteFile(windowACSQLInputPath(path), []byte(doc.String()), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{`DELETE FROM ReportData`, `DELETE FROM ReportDataDictionary`} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	add := func(id int, key, name string, meter int, first float64) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,'J',?,'Monthly','HVAC')`, id, key, name, meter); err != nil {
			t.Fatal(err)
		}
		for month := 1; month <= 2; month++ {
			if _, err := db.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`, month, id, first*float64(month)*3600000); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Each month: cooling=sum(10*i+i-1)=106, fans=sum(2*i)=20.
	add(1, "", "Electricity:Facility", 1, 126)
	add(2, "", "Cooling:Electricity", 1, 106)
	add(3, "", "Fans:Electricity", 1, 20)
	for i := 1; i <= 4; i++ {
		zone := fmt.Sprintf("Window Zone %d", i)
		add(100+i*10, zone+" Coil", "Cooling Coil Electricity Energy", 0, float64(10*i))
		add(101+i*10, zone+" Coil", "Cooling Coil Crankcase Heater Electricity Energy", 0, float64(i-1))
		add(102+i*10, zone+" Fan", "Fan Electricity Energy", 0, float64(2*i))
		add(200+i, zone, "Zone Air System Sensible Cooling Energy", 0, float64(100*i))
		add(250+i, zone, "Zone Air System Sensible Heating Energy", 0, 0)
		// Deliberately tempting overlapping package output is neither an owned
		// constituent nor a replacement for missing coil/fan observations.
		add(900+i, zone+" Window AC", "Zone Window Air Conditioner Electricity Energy", 0, 10000)
	}
	return doc, path, plan
}

func windowACSQLInputPath(sqlPath string) string {
	return filepath.Join(filepath.Dir(sqlPath), "window-ac-input.idf")
}

func TestEnergyPathWindowACSQLNativeValuesFanBoundaryMultipliersPathsAndReload(t *testing.T) {
	doc, path, plan := windowACSQLFixture(t)
	inputPath := windowACSQLInputPath(path)
	original, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	direct := 0
	for _, item := range parsed.Series {
		if item.directComponentID == "" {
			continue
		}
		direct++
		owner := 0
		for i := 1; i <= 4; i++ {
			if item.ZoneName == fmt.Sprintf("Window Zone %d", i) {
				owner = i
			}
		}
		want := float64(30 * owner)
		if item.directComponentID == "cooling.coil.crankcase_electricity" {
			want = float64(3 * (owner - 1))
		}
		if item.directComponentID == "fans.zone_equipment.electricity" {
			want = float64(6 * owner)
		}
		// This graph-free parser precedes multiplier application. Its native
		// scalar must equal the literal SQL observation; final sources below
		// separately assert the applied factor and already-model-total label.
		if owner == 0 || item.RawTotal != want || item.Total != want || item.Unit != "kWh" {
			t.Errorf("native component parser changed observation or owner: %+v want%g", item, want)
		}
		if item.EndUse == "fans" && item.sourceName != "Fan Electricity Energy" || item.EndUse == "cooling" && item.sourceName == "Fan Electricity Energy" {
			t.Fatalf("fan and coil consumption were mixed: %+v", item)
		}
	}
	if direct != 12 {
		t.Fatalf("native component roster=%d want12", direct)
	}
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(enrichEnergyExplanationWithServicePaths(legacy, inputPath))
	if len(legacy.servicePathIndex.nativeWindowACPaths) != 12 {
		t.Fatal("full original enrichment lost source-local WindowAC paths")
	}
	result := UpgradeEnergyExplanationV1(legacy)
	baseline := baseboardPathNumericSourceSnapshot(result)
	for reload := 0; reload < 3; reload++ {
		assertMixedHeatingNode(t, result.Nodes, "carrier", "electricity", 378)
		assertMixedHeatingNode(t, result.Nodes, "end_use", "cooling", 318)
		assertMixedHeatingNode(t, result.Nodes, "end_use", "fans", 60)
		seen := map[string]bool{}
		for _, zone := range result.ZoneResults {
			owner := 0
			for i := 1; i <= 4; i++ {
				if zone.Scope.ZoneName == fmt.Sprintf("Window Zone %d", i) {
					owner = i
				}
			}
			if owner == 0 || seen[zone.Scope.ZoneName] {
				t.Fatalf("unexpected/duplicate native Zone %s", zone.Scope.ZoneName)
			}
			seen[zone.Scope.ZoneName] = true
			factor := []float64{1, 4, 8, 32}[owner-1]
			object := directHVACFixtureObject(t, &doc, "ZoneHVAC:WindowAirConditioner", zone.Scope.ZoneName+" Window AC")
			wantPath := fmt.Sprintf("service-path:zone:%s:cooling:direct_zone_air:component:%d", strings.ToLower(zone.Scope.ZoneName), object.Index)
			periods := append([]EnergyPeriod{{ID: "root", Nodes: zone.Nodes, Links: zone.Links}}, zone.Periods...)
			seenPeriods := map[string]bool{}
			for _, period := range periods {
				if seenPeriods[period.ID] {
					t.Fatalf("duplicate native Zone period %s/%s", zone.Scope.ZoneName, period.ID)
				}
				monthFactor := 0.0
				switch period.ID {
				case "root", "annual":
					monthFactor = 3
				case "M1":
					monthFactor = 1
				case "M2":
					monthFactor = 2
				default:
					t.Fatalf("unexpected native Zone period %s/%s", zone.Scope.ZoneName, period.ID)
				}
				seenPeriods[period.ID] = true
				cooling := float64(owner*11-1) * monthFactor
				fan := float64(owner*2) * monthFactor
				assertMixedHeatingNode(t, period.Nodes, "carrier", "electricity", cooling+fan)
				assertMixedHeatingNode(t, period.Nodes, "end_use", "cooling", cooling)
				assertMixedHeatingNode(t, period.Nodes, "end_use", "fans", fan)
				assertMixedHeatingNode(t, period.Nodes, "load", "cooling", float64(100*owner)*factor*monthFactor)
				branchCount := map[string]int{}
				for _, link := range period.Links {
					if !energyPathLinkIsCarrierSplit(link) || link.Basis != "direct_zone_energy" {
						continue
					}
					kind, want := "cooling", cooling
					wantSources := []string{fmt.Sprintf("sql-rdd-%d", 100+owner*10), fmt.Sprintf("sql-rdd-%d", 101+owner*10)}
					// Zero-width constituents stay known in Sources, but are not
					// additive contributors to a nonzero presentation ribbon.
					if owner == 1 {
						wantSources = wantSources[:1]
					}
					if stringSliceContains(link.SourceIDs, fmt.Sprintf("sql-rdd-%d", 102+owner*10)) {
						kind, want = "fans", fan
						wantSources = []string{fmt.Sprintf("sql-rdd-%d", 102+owner*10)}
					}
					branchCount[kind]++
					if math.Abs(link.FromValue-want) > 1e-9 || math.Abs(link.ToValue-want) > 1e-9 || !reflect.DeepEqual(link.RelatedPathIDs, []string{wantPath}) || !reflect.DeepEqual(link.SourceIDs, wantSources) {
						t.Errorf("reload%d %s/%s %s branch changed native quantity/source/path: %+v wantSources%v", reload, zone.Scope.ZoneName, period.ID, kind, link, wantSources)
					}
				}
				if branchCount["cooling"] != 1 || branchCount["fans"] != 1 {
					t.Errorf("native branch cardinalities %s/%s=%v", zone.Scope.ZoneName, period.ID, branchCount)
				}
			}
			if len(seenPeriods) != 4 || !seenPeriods["M1"] || !seenPeriods["M2"] || !seenPeriods["root"] || !seenPeriods["annual"] {
				t.Errorf("native period census must be exactly root, annual, M1, M2: %v", seenPeriods)
			}
		}
		if len(seen) != 4 {
			t.Fatalf("native precomputed Zones=%d want4", len(seen))
		}
		nativeSources, loadSources := 0, 0
		for _, source := range result.Sources {
			for owner := 1; owner <= 4; owner++ {
				if source.ID == fmt.Sprintf("sql-rdd-%d", 200+owner) {
					loadSources++
					factor := []float64{1, 4, 8, 32}[owner-1]
					if source.RawValue != float64(300*owner) || source.EffectiveValue != float64(300*owner)*factor || source.EffectiveMultiplier != factor {
						t.Errorf("representative Zone load multiplier is not applied once: %+v", source)
					}
				}
				for family := 0; family < 3; family++ {
					if source.ID != fmt.Sprintf("sql-rdd-%d", 100+owner*10+family) {
						continue
					}
					nativeSources++
					want := []float64{float64(10 * owner), float64(owner - 1), float64(2 * owner)}[family] * 3
					if source.RawValue != want || source.EffectiveValue != want || source.EffectiveMultiplier != 1 || source.MultiplierApplication != energyMultiplierAlreadyModelTotal || source.ZoneName != fmt.Sprintf("Window Zone %d", owner) || !energyDataSourceValueKnown(source, energySourceObservedRaw) || !energyDataSourceValueKnown(source, energySourceObservedEffective) {
						t.Errorf("reload%d native source lost value/owner/factor/knownness: %+v", reload, source)
					}
				}
			}
			if strings.HasPrefix(source.ID, "sql-rdd-90") {
				t.Errorf("overlapping package subtotal became native evidence: %+v", source)
			}
		}
		if nativeSources != 12 || loadSources != 4 {
			t.Errorf("source roster=%d/%d want12/4", nativeSources, loadSources)
		}
		if !reflect.DeepEqual(baseboardPathNumericSourceSnapshot(result), baseline) {
			t.Errorf("reload%d changed numeric/source identities", reload)
		}
		if reload < 2 {
			wire, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var reopened EnergyExplanationResult
			if err := json.Unmarshal(wire, &reopened); err != nil {
				t.Fatal(err)
			}
			result = reopened
		}
	}
	after, err := os.ReadFile(inputPath)
	if err != nil || !reflect.DeepEqual(original, after) {
		t.Fatal("parse/build/reload changed original fixture")
	}
}

func TestEnergyPathWindowACSQLIncompleteCoolingCannotBorrowFanOrPackage(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		wantCooling int
	}{
		{"complete known zero crankcase", "", 8},
		{"missing main", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=110`, 6},
		{"NULL main", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=110`, 6},
		{"wrong main units", `UPDATE ReportDataDictionary SET Units='kJ' WHERE ReportDataDictionaryIndex=110`, 6},
		{"Hourly cannot fill Monthly main", `UPDATE ReportDataDictionary SET ReportingFrequency='Hourly' WHERE ReportDataDictionaryIndex=110`, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, path, plan := windowACSQLFixture(t)
			if tc.query != "" {
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(tc.query); err != nil {
					db.Close()
					t.Fatal(err)
				}
				db.Close()
			}
			parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
			if err != nil {
				t.Fatal(err)
			}
			cooling, fans := 0, 0
			for _, series := range parsed.Series {
				if series.directComponentID == "" {
					continue
				}
				switch series.EndUse {
				case "cooling":
					cooling++
				case "fans":
					fans++
				}
			}
			if cooling != tc.wantCooling || fans != 4 {
				t.Fatalf("incomplete cooling borrowed a surviving constituent or erased independent fans: cooling%d/fans%d want%d/4", cooling, fans, tc.wantCooling)
			}
		})
	}
}
