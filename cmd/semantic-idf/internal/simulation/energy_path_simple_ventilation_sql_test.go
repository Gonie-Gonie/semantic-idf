package simulation

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Hand observations only. A private two-month SQLite fixture is intentionally
// not an annual native capture or acceptance oracle. The original physics is
// not executed or modified. Native M1 fan observations are 0,2,3 kWh; M2 is
// twice M1. Both parent meters are 5,10 kWh, not 32,64 from Zone expansion.
func simpleVentilationSQLFixture(t *testing.T) (idf.Document, string, PurposeRunPlan) {
	t.Helper()
	doc := simpleVentilationHandDocument(t)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare})
	path := directHVACSQLFixture(t)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "ventilation-input.idf"), []byte(doc.String()), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM ReportData`, `DELETE FROM ReportDataDictionary`} {
		if _, err := tx.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	add := func(id int, key, name string, meter int, first float64) {
		t.Helper()
		if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,'J',?,'Monthly','HVAC')`, id, key, name, meter); err != nil {
			t.Fatal(err)
		}
		for month := 1; month <= 2; month++ {
			if _, err := tx.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`, month, id, first*float64(month)*3600000); err != nil {
				t.Fatal(err)
			}
		}
	}
	add(1, "", "Electricity:Facility", 1, 5)
	add(2, "", "Fans:Electricity", 1, 5)
	for i, zone := range []string{"Natural", "Intake", "Exhaust"} {
		add(10+i, zone, energyPathSimpleVentilationFanName, 0, []float64{0, 2, 3}[i])
		add(20+i, zone, "Zone Air System Sensible Heating Energy", 0, 0)
		add(30+i, zone, "Zone Air System Sensible Cooling Energy", 0, 0)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return doc, path, plan
}

func TestEnergyPathSimpleVentilationPlanExactZoneAggregateAndHourlyCompanions(t *testing.T) {
	doc := simpleVentilationHandDocument(t)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	expected := map[string]bool{}
	for _, zone := range []string{"Natural", "Intake", "Exhaust"} {
		for _, frequency := range []string{"Monthly", "Hourly"} {
			expected[zone+"/"+frequency] = false
		}
	}
	fanMeter := false
	for _, o := range plan.OutputObjects {
		if strings.EqualFold(o.KeyValue, "Fans:Electricity") && o.ReportingFrequency == "Monthly" {
			fanMeter = true
		}
		if o.VariableName != energyPathSimpleVentilationFanName {
			continue
		}
		key := o.KeyValue + "/" + o.ReportingFrequency
		seen, exists := expected[key]
		if !exists || seen || o.ScopeZoneName != o.KeyValue || !purposeIDsContain(o.PurposeIDs, SimulationPurposeBasicEnergy) {
			t.Fatalf("unexpected duplicate/native owner/request: %+v", o)
		}
		expected[key] = true
	}
	if !fanMeter {
		t.Fatal("native ventilation fan was omitted merely because no Fan:* exists")
	}
	for key, seen := range expected {
		if !seen {
			t.Fatalf("missing native request %s", key)
		}
	}
}

func TestEnergyPathSimpleVentilationSQLZeroAndPositiveNativeTotalsNoDoubleCount(t *testing.T) {
	doc, path, plan := simpleVentilationSQLFixture(t)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, s := range parsed.Series {
		if s.directComponentID != energyPathSimpleVentilationFanID {
			continue
		}
		count++
		want, ok := map[string]float64{"Natural": 0, "Intake": 6, "Exhaust": 9}[s.ZoneName]
		if !ok || s.Total != want || s.RawTotal != want || s.EndUse != "fans" || s.sourceKeyValue != s.ZoneName || len(s.Monthly) != 2 || s.sourceFrequency != "Monthly" {
			t.Fatalf("native observation changed: %+v", s)
		}
	}
	if count != 3 {
		t.Fatalf("aggregate source census=%d want3 (not4 objects, not6 frequencies)", count)
	}
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(enrichEnergyExplanationWithServicePaths(legacy, filepath.Join(filepath.Dir(path), "ventilation-input.idf")))
	result := UpgradeEnergyExplanationV1(legacy)
	snapshot := baseboardPathNumericSourceSnapshot(result)
	for pass := 0; pass < 3; pass++ {
		assertMixedHeatingNode(t, result.Nodes, "carrier", "electricity", 15)
		assertMixedHeatingNode(t, result.Nodes, "end_use", "fans", 15)
		ledger := epath101AuditAuxiliaryReconciliation(result.Reconciliation, "fans", "electricity", "annual")
		if ledger == nil || ledger.ExpectedValue != 15 || ledger.DirectValue != 15 || ledger.AllocatedValue != 0 || ledger.UnassignedValue != 0 || ledger.OvermappedValue != 0 {
			t.Fatalf("broad/direct fan double-count or allocation: %+v", ledger)
		}
		seenSources := map[string]bool{}
		for _, source := range result.Sources {
			if source.Name != energyPathSimpleVentilationFanName {
				continue
			}
			if source.ReportingFrequency != "Monthly" {
				t.Fatalf("unexpected hand source frequency: %+v", source)
			}
			want, ok := map[string]float64{"Natural": 0, "Intake": 6, "Exhaust": 9}[source.ZoneName]
			if !ok || seenSources[source.ZoneName] || source.RawValue != want || source.EffectiveValue != want || source.EffectiveMultiplier != 1 || source.MultiplierApplication != energyMultiplierAlreadyModelTotal || !energyDataSourceValueKnown(source, energySourceObservedRaw) || !energyDataSourceValueKnown(source, energySourceObservedEffective) {
				t.Fatalf("zero/native value/knownness/factor changed after reload%d: %+v", pass, source)
			}
			seenSources[source.ZoneName] = true
		}
		if len(seenSources) != 3 {
			t.Fatalf("native source roster=%v", seenSources)
		}
		zones := map[string]bool{}
		for _, zone := range result.ZoneResults {
			zoneName := zone.Scope.ZoneName
			annual, ok := map[string]float64{"Natural": 0, "Intake": 6, "Exhaust": 9}[zoneName]
			if !ok || zones[zoneName] {
				t.Fatalf("missing/foreign/duplicate Zone: %s", zoneName)
			}
			zones[zoneName] = true
			check := func(nodes []EnergyExplanationNode, links []EnergyPathLink, want float64) {
				t.Helper()
				fanCount := 0
				for _, node := range nodes {
					if node.Level == "end_use" && node.EndUse == "fans" {
						fanCount++
						if node.Value != want || node.Basis != "direct_zone_energy" || len(node.RelatedPathIDs) != 0 {
							t.Fatalf("native fan acquired foreign quantity/basis/path: %+v", node)
						}
					}
					if (node.EndUse == "heating" || node.EndUse == "cooling") && node.Level == "end_use" && node.Value != 0 {
						t.Fatalf("uncontrolled fixture invented conditioning consumption: %+v", node)
					}
				}
				if want > 0 && fanCount != 1 || want == 0 && fanCount > 1 {
					t.Fatalf("fan node census=%d want%g", fanCount, want)
				}
				for _, link := range links {
					if link.Relation == "load_to_end_use" {
						t.Fatalf("uncontrolled/native fan invented thermal conversion: %+v", link)
					}
				}
			}
			check(zone.Nodes, zone.Links, annual)
			periods := map[string]bool{}
			for _, period := range zone.Periods {
				if periods[period.ID] {
					t.Fatalf("duplicate period %s", period.ID)
				}
				periods[period.ID] = true
				switch period.ID {
				case "annual":
					check(period.Nodes, period.Links, annual)
				case "M1":
					check(period.Nodes, period.Links, annual/3)
				case "M2":
					check(period.Nodes, period.Links, annual*2/3)
				default:
					t.Fatalf("unexpected hand period %s", period.ID)
				}
			}
			if !periods["M1"] || !periods["M2"] {
				t.Fatalf("missing hand months: %v", periods)
			}
		}
		if len(zones) != 3 {
			t.Fatalf("native Zone roster=%v", zones)
		}
		if !reflect.DeepEqual(snapshot, baseboardPathNumericSourceSnapshot(result)) {
			t.Fatalf("reload%d changed numeric/source identity", pass)
		}
		if pass < 2 {
			wire, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded EnergyExplanationResult
			if err = json.Unmarshal(wire, &reloaded); err != nil {
				t.Fatal(err)
			}
			result = reloaded
		}
	}
}

func TestEnergyPathSimpleVentilationSQLMissingIsNotNaturalObservedZero(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		count       int
	}{
		{"observed natural zero", "", 3},
		{"missing natural observation", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=10`, 2},
		{"NULL intake", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=11 AND TimeIndex=1`, 2},
		{"missing intake month", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=11 AND TimeIndex=1`, 2},
		{"negative intake", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=11 AND TimeIndex=1`, 2},
		{"duplicate intake observation", `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(1,11,7200000)`, 2},
		{"same native identity in two dictionaries", `INSERT INTO ReportDataDictionary SELECT 111,KeyValue,Name,Units,IsMeter,ReportingFrequency,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=11; INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,111,Value FROM ReportData WHERE ReportDataDictionaryIndex=11`, 2},
		{"foreign Zone key", `UPDATE ReportDataDictionary SET KeyValue='Other' WHERE ReportDataDictionaryIndex=11`, 2},
		{"wrong unit", `UPDATE ReportDataDictionary SET Units='kJ' WHERE ReportDataDictionaryIndex=11`, 2},
		{"meter cannot become native variable", `UPDATE ReportDataDictionary SET IsMeter=1 WHERE ReportDataDictionaryIndex=11`, 2},
		{"Hourly cannot fill Monthly", `UPDATE ReportDataDictionary SET ReportingFrequency='Hourly' WHERE ReportDataDictionaryIndex=11`, 2},
		{"unrelated similarly named fan", `UPDATE ReportDataDictionary SET Name='Fan Electricity Energy' WHERE ReportDataDictionaryIndex=11`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, path, plan := simpleVentilationSQLFixture(t)
			if tc.query != "" {
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				for _, query := range strings.Split(tc.query, ";") {
					if _, err = db.Exec(query); err != nil {
						db.Close()
						t.Fatal(err)
					}
				}
				db.Close()
			}
			parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, s := range parsed.Series {
				if s.directComponentID == energyPathSimpleVentilationFanID {
					count++
					if s.Total < 0 {
						t.Fatal("negative became direct fan")
					}
				}
			}
			if count != tc.count {
				t.Fatalf("direct native source count=%d want%d", count, tc.count)
			}
		})
	}
}

func TestEnergyPathSimpleVentilationDictionaryScopeIsNotEquipmentNameOrUserScope(t *testing.T) {
	doc := simpleVentilationHandDocument(t)
	targets := energyPathSimpleVentilationDirectTargets(energyPathSimpleVentilationTargets(doc))
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	dictionary := energyExplanationDictionary{row: sqlOutputDictionaryRow{index: 11, keyValue: "Intake", name: energyPathSimpleVentilationFanName, units: "J"}, reportingFrequency: "Monthly"}
	definition, zone, _, ok := energyPathDirectHVACComponentDictionaryScope(dictionary, &plan, targets)
	if !ok || zone != "Intake" || definition.ID != energyPathSimpleVentilationFanID {
		t.Fatalf("exact original aggregate not bound: %s %s %t", definition.ID, zone, ok)
	}
	dictionary.row.keyValue = "Intake inlet"
	if _, _, _, ok := energyPathDirectHVACComponentDictionaryScope(dictionary, &plan, targets); ok {
		t.Fatal("ventilation object name substituted for reporting Zone")
	}
}

func TestEnergyPathSimpleVentilationSelectedScopeKeepsWholeOriginalAggregation(t *testing.T) {
	doc := simpleVentilationHandDocument(t)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"Exhaust"}}})
	monthly, hourly := 0, 0
	for _, o := range plan.OutputObjects {
		if o.VariableName != energyPathSimpleVentilationFanName {
			continue
		}
		if o.KeyValue != "Exhaust" || o.ScopeZoneName != "Exhaust" {
			t.Fatalf("selected request reassigned a foreign reporting Zone: %+v", o)
		}
		switch o.ReportingFrequency {
		case "Monthly":
			monthly++
		case "Hourly":
			hourly++
		default:
			t.Fatalf("unexpected frequency %s", o.ReportingFrequency)
		}
	}
	if monthly != 1 || hourly != 1 {
		t.Fatalf("two ventilation objects became duplicate selected sources: M%d H%d", monthly, hourly)
	}
	targets := energyPathSimpleVentilationDirectTargets(energyPathSimpleVentilationTargets(doc))
	if len(targets) != 3 {
		t.Fatal("user scope changed original ownership census")
	}
	d := energyExplanationDictionary{row: sqlOutputDictionaryRow{index: 11, keyValue: "Intake", name: energyPathSimpleVentilationFanName, units: "J"}, reportingFrequency: "Monthly"}
	if _, _, _, ok := energyPathDirectHVACComponentDictionaryScope(d, &plan, targets); ok {
		t.Fatal("selected-only request authorized unrequested foreign direct source")
	}
}

func TestEnergyPathSimpleVentilationHourlyEvidenceNeverDuplicatesMonthlyBudget(t *testing.T) {
	doc, path, plan := simpleVentilationSQLFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO "Time"(TimeIndex,Month,Day,Hour,Minute,Year,"Interval",IntervalType,EnvironmentPeriodIndex,WarmupFlag) VALUES(101,1,1,1,0,2017,60,1,3,0),(102,2,1,1,0,2017,60,1,3,0)`,
		`INSERT INTO ReportDataDictionary SELECT ReportDataDictionaryIndex+100,KeyValue,Name,Units,IsMeter,'Hourly',IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex BETWEEN 10 AND 12`,
		`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex+100,ReportDataDictionaryIndex+100,Value*100 FROM ReportData WHERE ReportDataDictionaryIndex BETWEEN 10 AND 12`,
	} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	db.Close()
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
	if err != nil {
		t.Fatal(err)
	}
	native, sum, hourly := 0, 0.0, 0
	for _, s := range parsed.Series {
		if s.directComponentID == energyPathSimpleVentilationFanID {
			native++
			sum += s.Total
			if s.sourceFrequency != "Monthly" {
				t.Fatal("Hourly source entered native Monthly budget")
			}
		}
	}
	for _, s := range parsed.Sources {
		if s.Name == energyPathSimpleVentilationFanName && s.ReportingFrequency == "Hourly" {
			hourly++
		}
	}
	if native != 3 || sum != 15 || hourly != 3 {
		t.Fatalf("poisoned independent Hourly evidence changed Monthly native budget: count%d total%g charts%d", native, sum, hourly)
	}
}

func TestEnergyPathSimpleVentilationUnobservedBroadFanRemainderStaysUnassigned(t *testing.T) {
	doc, path, plan := simpleVentilationSQLFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`UPDATE ReportData SET Value=Value*1.6 WHERE ReportDataDictionaryIndex IN (1,2)`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
	if err != nil {
		t.Fatal(err)
	}
	legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(enrichEnergyExplanationWithServicePaths(legacy, filepath.Join(filepath.Dir(path), "ventilation-input.idf")))
	result := UpgradeEnergyExplanationV1(legacy)
	row := epath101AuditAuxiliaryReconciliation(result.Reconciliation, "fans", "electricity", "annual")
	if row == nil || row.ExpectedValue != 24 || row.DirectValue != 15 || row.AllocatedValue != 0 || row.UnassignedValue != 9 || row.OvermappedValue != 0 {
		t.Fatalf("unobserved remainder became ventilation or thermal allocation: %+v", row)
	}
	total := 0.0
	for _, zone := range result.ZoneResults {
		for _, node := range zone.Nodes {
			if node.Level == "end_use" && node.EndUse == "fans" {
				total += node.Value
			}
		}
	}
	if total != 15 {
		t.Fatalf("unknown broad fan remainder was duplicated into Zones: %g", total)
	}
}
