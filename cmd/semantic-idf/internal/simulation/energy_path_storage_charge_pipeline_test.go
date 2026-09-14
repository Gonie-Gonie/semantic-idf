package simulation

// Full-builder regression. Hand IDF and 12-month SQL only; no engine,
// candidate, expected artifact or native capture supplies these quantities.
import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathStoragePipelineFixture struct {
	Document   idf.Document
	InputPath  string
	Files      []SimulationFileInfo
	ChargeMode string
	Mixed      bool
}

// This validates reporting ownership, not an engine-ready battery or PV
// operating model. No inverter, SOC, efficiency or supply-bus equation is
// inferred from this intentionally small non-engine fixture.
func epathStoragePipelineSQL(t *testing.T, mode string, mixed bool) epathStoragePipelineFixture {
	t.Helper()
	text := `Version,25.1;
Zone,Office,0,0,0,0,1,1,3,60,20;
Schedule:Constant,Always,,1;
Lights,Office Lights,Office,Always,LightingLevel,1000;
ElectricLoadCenter:Distribution,DC Center,,,,,,DirectCurrentWithInverterDCStorage,,DC Battery;
ElectricLoadCenter:Storage:Battery,DC Battery;
Output:Meter,Electricity:Facility,Monthly;
Output:Meter,InteriorLights:Electricity,Monthly;
Output:Variable,Office,Zone Lights Electricity Energy,Monthly;
Output:Variable,Office,Zone Air System Sensible Heating Energy,Monthly;
Output:Variable,Office,Zone Air System Sensible Cooling Energy,Monthly;
Output:Variable,DC Battery,Electric Storage Charge Energy,Monthly;
`
	if mixed {
		text += `ElectricLoadCenter:Distribution,AC Center,,,,,,AlternatingCurrentWithStorage,,AC Battery;
ElectricLoadCenter:Storage:Simple,AC Battery;
Output:Variable,AC Battery,Electric Storage Charge Energy,Monthly;
`
	}
	doc, err := idf.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	inputPath, path := filepath.Join(dir, "literal.idf"), filepath.Join(dir, "eplusout.sql")
	if err := os.WriteFile(inputPath, []byte(text), 0600); err != nil {
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
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	// All modest schema/data inserts share one transaction: no fsync loop.
	exec(`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentName TEXT,EnvironmentType INTEGER)`)
	exec(`INSERT INTO EnvironmentPeriods VALUES(1,'ANNUAL WEATHER',3)`)
	exec(`CREATE TABLE "Time"(TimeIndex INTEGER PRIMARY KEY,EnvironmentPeriodIndex INTEGER,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" REAL,WarmupFlag INTEGER,IntervalType INTEGER,SimulationDays INTEGER)`)
	exec(`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,Name TEXT,KeyValue TEXT,IsMeter INTEGER,ReportingFrequency TEXT,Units TEXT,IndexGroup TEXT,Type TEXT,TimestepType TEXT,ScheduleName TEXT)`)
	exec(`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,ReportDataDictionaryIndex INTEGER,TimeIndex INTEGER,Value REAL)`)
	type dictionary struct {
		id        int
		name, key string
		meter     int
		january   float64
	}
	dictionaries := []dictionary{
		{1, "Electricity:Facility", "", 1, 100},
		{2, "InteriorLights:Electricity", "", 1, 90},
		{3, "Zone Lights Electricity Energy", "Office", 0, 90},
		{4, "Zone Air System Sensible Heating Energy", "Office", 0, 0},
		{5, "Zone Air System Sensible Cooling Energy", "Office", 0, 0},
	}
	if mode != "absent" {
		value := 10.0
		if mode == "zero" {
			value = 0
		}
		dictionaries = append(dictionaries, dictionary{51, "Electric Storage Charge Energy", "DC Battery", 0, value})
	}
	if mixed {
		dictionaries = append(dictionaries, dictionary{52, "Electric Storage Charge Energy", "AC Battery", 0, 4})
	}
	for _, d := range dictionaries {
		timestep := "HVAC System"
		if d.meter == 1 || d.id == 3 {
			timestep = "Zone"
		}
		exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,?, 'Monthly','J','Facility','Sum',?,NULL)`, d.id, d.name, d.key, d.meter, timestep)
	}
	row := 0
	for month := 1; month <= 12; month++ {
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC)
		exec(`INSERT INTO "Time" VALUES(?,1,2017,?,?,24,0,?,NULL,3,?)`, month, month, last.Day(), last.Day()*24*60, last.YearDay())
		for _, d := range dictionaries {
			if d.id == 51 && mode == "no_rows" {
				continue
			}
			var value any = 0.0
			if month == 1 {
				value = d.january * 3.6e6
			}
			if d.id == 51 && (mode == "all_null" || mode == "one_null" && month == 6) {
				value = nil
			}
			row++
			exec(`INSERT INTO ReportData VALUES(?,?,?,?)`, row, d.id, month, value)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return epathStoragePipelineFixture{Document: doc, InputPath: inputPath, Files: []SimulationFileInfo{{Name: "eplusout.sql", Path: path, Kind: "sqlite"}}, ChargeMode: mode, Mixed: mixed}
}

func epathStoragePipelinePlan(fixture epathStoragePipelineFixture, policy, zone string) (PurposeRunPlan, SimulationPurposeRequest) {
	request := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, AllocationPolicy: policy}
	if zone != "" {
		request.Scope = SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{zone}}
	}
	plan := BuildPurposeRunPlan(fixture.Document, request)
	// The hand test explicitly requests its native Monthly charge observations.
	// Output-discovery/recapture coverage is tested elsewhere, not inferred from
	// this small IDF's deliberately omitted PV/inverter operating parameters.
	keys := []string{"DC Battery"}
	if fixture.Mixed {
		keys = append(keys, "AC Battery")
	}
	for _, key := range keys {
		found := false
		for _, object := range plan.OutputObjects {
			if object.ObjectType == "Output:Variable" && strings.EqualFold(object.VariableName, "Electric Storage Charge Energy") && strings.EqualFold(object.KeyValue, key) && strings.EqualFold(object.ReportingFrequency, "Monthly") {
				found = true
			}
		}
		if !found {
			plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", KeyValue: key, VariableName: "Electric Storage Charge Energy", ReportingFrequency: "Monthly", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
		}
	}
	return plan, request
}

func epathStoragePipelineLegacy(fixture epathStoragePipelineFixture, plan PurposeRunPlan) EnergyExplanationV1 {
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(fixture.Document), fixture.Document)
	dashboard := buildEnergyDashboardResultFromFiles(fixture.Files)
	legacy := buildEnergyExplanationResultFromFilesWithDriverContext(fixture.Files, dashboard, &plan, context)
	legacy = enrichEnergyExplanationWithServicePaths(legacy, fixture.InputPath)
	return applyEnergyExplanationV1ServicePathLoadShareAllocation(legacy)
}

func epathStoragePipelineNear(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %.12g, want literal %.12g", name, got, want)
	}
}

func epathStoragePipelineChargeSource(id string) bool {
	return id == "sql-rdd-51" || id == "sql-rdd-52"
}

func epathStoragePipelineAccounting(t *testing.T, rows []EnergyReconciliation, period string, expected, explained, residual float64) {
	t.Helper()
	id := "reconcile.energy.electricity." + period
	count := 0
	for _, row := range rows {
		if row.ID != id {
			continue
		}
		count++
		epathStoragePipelineNear(t, id+" expected", row.ExpectedValue, expected)
		epathStoragePipelineNear(t, id+" explained", row.ExplainedValue, explained)
		epathStoragePipelineNear(t, id+" residual", row.ResidualValue, residual)
		for _, sourceID := range row.SourceIDs {
			if epathStoragePipelineChargeSource(sourceID) {
				t.Errorf("non-Facility charge contributed to %s source budget", id)
			}
		}
	}
	if count != 1 {
		t.Errorf("%s accounting count=%d want1", id, count)
	}
}

// Allow deliberate zero-node pruning, never disappearance of observed sources.
// V1 may retain separate keys and V2 may aggregate their non-flow context; the
// distinct typed keys/owners and total charge must remain recoverable either way.
func epathStoragePipelineGraph(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink, edges []EnergyExplanationEdge, fixture epathStoragePipelineFixture, zone bool) {
	t.Helper()
	keys, chargeIDs := map[string]bool{}, map[string]bool{}
	total := 0.0
	for _, node := range nodes {
		if len(node.storageChargeBoundaries) == 0 {
			for _, id := range node.SourceIDs {
				if epathStoragePipelineChargeSource(id) && (node.Level == "energy" || node.Level == "end_use" || node.Level == "carrier" || node.Level == "load") {
					t.Errorf("charge source leaked into additive node: %+v", node)
				}
			}
			continue
		}
		chargeIDs[node.ID] = true
		if zone || node.Level != "support" || node.ZoneName != "" {
			t.Errorf("native global charge projected as consumption/Zone: %+v", node)
		}
		total += node.Value
		for _, boundary := range node.storageChargeBoundaries {
			if boundary.State != energyPathStorageChargeNative || validateEnergyPathStorageChargeBoundary(&boundary) != nil {
				t.Errorf("lost native reporting proof: %+v", boundary)
			}
			keys[boundary.SourceKey] = true
			wantSource := "sql-rdd-51"
			if boundary.SourceKey == "ac battery" {
				wantSource = "sql-rdd-52"
			}
			if len(boundary.SourceIDs) != 1 || boundary.SourceIDs[0] != wantSource {
				t.Errorf("per-key boundary lost exact native constituent: %+v", boundary)
			}
			for _, ref := range boundary.Objects {
				if ref.ObjectIndex < 0 || ref.ObjectIndex >= len(fixture.Document.Objects) {
					t.Fatalf("boundary index outside original: %+v", ref)
				}
				original := fixture.Document.Objects[ref.ObjectIndex]
				if original.Index != ref.ObjectIndex || original.Type != ref.ObjectType || len(original.Fields) == 0 || original.Fields[0].Value != ref.ObjectName {
					t.Errorf("boundary owner not exact original: %+v", ref)
				}
			}
			if boundary.SourceKey == "dc battery" && boundary.BusType != "DirectCurrentWithInverterDCStorage" {
				t.Error("DC native bus changed")
			}
			if boundary.SourceKey == "ac battery" && boundary.BusType != "AlternatingCurrentWithStorage" {
				t.Error("AC native bus changed")
			}
		}
	}
	for _, edge := range edges {
		if chargeIDs[edge.FromID] || chargeIDs[edge.ToID] {
			t.Errorf("legacy flow still touches charge: %+v", edge)
		}
	}
	for _, link := range links {
		if chargeIDs[link.FromID] || chargeIDs[link.ToID] {
			t.Errorf("supply/consumption flow still touches charge: %+v", link)
		}
		for _, id := range link.SourceIDs {
			if epathStoragePipelineChargeSource(id) {
				t.Errorf("charge context became another link's quantity authority: %+v", link)
			}
		}
	}
	if !zone && fixture.ChargeMode == "positive" {
		want := 10.0
		if !keys["dc battery"] {
			t.Error("positive DC context/typed key missing")
		}
		if fixture.Mixed {
			want += 4
			if !keys["ac battery"] {
				t.Error("same-name AC source was lost to DC source preference")
			}
		}
		epathStoragePipelineNear(t, "non-flow context total", total, want)
	}
}

func epathStoragePipelineSources(t *testing.T, sources []EnergyDataSource, fixture epathStoragePipelineFixture, zone bool) {
	t.Helper()
	for _, id := range []string{"sql-rdd-51", "sql-rdd-52"} {
		source := energyExplanationSourceByID(sources, id)
		// Dictionary identity survives unknown/all-NULL/no-row observations;
		// only a genuinely absent dictionary has no source record.
		wantPresent := id == "sql-rdd-52" && fixture.Mixed || id == "sql-rdd-51" && fixture.ChargeMode != "absent"
		if zone {
			if source != nil {
				for _, detail := range source.ScopeDetails {
					if detail.Scope.Kind == "zone" {
						t.Errorf("global charge source gained a Zone accounting detail: %+v", detail)
					}
				}
			}
			continue
		}
		if (source != nil) != wantPresent {
			t.Errorf("%s source presence=%v want%v in %s", id, source != nil, wantPresent, fixture.ChargeMode)
			continue
		}
		if source == nil {
			continue
		}
		wantKnown := !(id == "sql-rdd-51" && fixture.ChargeMode != "positive" && fixture.ChargeMode != "zero")
		for _, bit := range []uint8{energySourceObservedRaw, energySourceObservedEffective} {
			if energyDataSourceValueKnown(*source, bit) != wantKnown {
				t.Errorf("%s native scalar knownness changed (bit%d) in %s", id, bit, fixture.ChargeMode)
			}
		}
		if source.SourceType != "sql_report_data" || source.Name != "Electric Storage Charge Energy" || source.IsMeter || source.SourceUnit != "J" || source.NormalizedUnit != "kWh" || source.ReportingFrequency != "Monthly" || source.ZoneName != "" || source.AllocationApplied {
			t.Errorf("native charge metadata changed: %+v", source)
		}
		wantKey := "DC Battery"
		if id == "sql-rdd-52" {
			wantKey = "AC Battery"
		}
		if source.KeyValue != wantKey {
			t.Errorf("%s reporting key=%q want%q", id, source.KeyValue, wantKey)
		}
		for _, detail := range source.ScopeDetails {
			if detail.Scope.Kind == "zone" {
				t.Errorf("global charge source gained a Zone accounting detail: %+v", detail)
			}
		}
		if wantKnown {
			want := 10.0
			if id == "sql-rdd-52" {
				want = 4
			} else if fixture.ChargeMode == "zero" {
				want = 0
			}
			epathStoragePipelineNear(t, id+" raw", source.RawValue, want)
			epathStoragePipelineNear(t, id+" effective", source.EffectiveValue, want)
			if source.EffectiveMultiplier != 1 {
				t.Errorf("model-total charge multiplier = %g", source.EffectiveMultiplier)
			}
		}
	}
}

func epathStoragePipelineSummary(t *testing.T, summary EnergyExplanationSummary) {
	t.Helper()
	for _, collection := range [][]EnergyExplanationSummaryItem{summary.EndUses, summary.Carriers, summary.Loads, summary.Ratios, summary.Residuals, summary.EnergyByEndUse, summary.EnergyByCarrier, summary.DerivedKPIs} {
		for _, item := range collection {
			if item.EndUse == "storage_charge" || item.Kind == "energy.storage_charge" {
				t.Errorf("charge context became a consumption/ratio summary row: %+v", item)
			}
			for _, id := range item.SourceIDs {
				if epathStoragePipelineChargeSource(id) {
					t.Errorf("charge source leaked into numeric summary: %+v", item)
				}
			}
		}
	}
}

func epathStoragePipelineCheckBundle(t *testing.T, bundle PurposeResultBundle, fixture epathStoragePipelineFixture, zone bool) {
	t.Helper()
	result := bundle.EnergyExplanation
	if result.Schema != energyExplanationSchema {
		t.Fatalf("full builder produced no v2: %q", result.Schema)
	}
	if zone != (result.Scope.Kind == "zone") {
		t.Fatalf("wrong requested scope: %+v", result.Scope)
	}
	epathStoragePipelineGraph(t, result.Nodes, result.Links, nil, fixture, zone)
	epathStoragePipelineSources(t, result.Sources, fixture, zone)
	epathStoragePipelineSummary(t, bundle.EnergyExplanationSummary)
	epathStoragePipelineSummary(t, buildEnergyExplanationSummary(result))
	if !zone {
		epathStoragePipelineAccounting(t, result.Reconciliation, "annual", 100, 90, 10)
		for _, key := range []string{"DC Battery", "AC Battery"} {
			if key == "AC Battery" && !fixture.Mixed {
				continue
			}
			count := 0
			for _, entry := range result.Completeness.SourceAvailability {
				if !strings.EqualFold(entry.Name, "Electric Storage Charge Energy ["+key+"; Monthly]") {
					continue
				}
				count++
				wantStatus := "found"
				if key == "DC Battery" && fixture.ChargeMode != "positive" && fixture.ChargeMode != "zero" {
					wantStatus = "missing"
				}
				if entry.Level != "context" || entry.Status != wantStatus {
					t.Errorf("requested charge context availability became consumption/falsezero: %+v", entry)
				}
			}
			if count != 1 {
				t.Errorf("%s requested Monthly context availability count=%d want1", key, count)
			}
		}
	}
	monthFound := false
	for _, period := range result.Periods {
		if period.ID != "M1" {
			continue
		}
		monthFound = true
		epathStoragePipelineGraph(t, period.Nodes, period.Links, nil, fixture, zone)
		if !zone {
			epathStoragePipelineAccounting(t, period.Reconciliation, "M1", 100, 90, 10)
		}
		if period.Summary != nil {
			epathStoragePipelineSummary(t, *period.Summary)
		}
	}
	if !monthFound {
		t.Error("native Monthly authority missing")
	}
	if zone {
		lighting := 0.0
		for _, node := range result.Nodes {
			if node.Level == "end_use" && node.EndUse == "lighting" {
				lighting += node.Value
			}
		}
		epathStoragePipelineNear(t, "Office observed lighting", lighting, 90)
	}
	for _, scoped := range result.ZoneResults {
		epathStoragePipelineGraph(t, scoped.Nodes, scoped.Links, nil, fixture, true)
		epathStoragePipelineSummary(t, scoped.Summary)
		for _, period := range scoped.Periods {
			if period.ID == "M1" {
				epathStoragePipelineGraph(t, period.Nodes, period.Links, nil, fixture, true)
				if period.Summary != nil {
					epathStoragePipelineSummary(t, *period.Summary)
				}
			}
		}
	}
}

func TestEnergyPathStorageChargeFullBuilderConsumptionAndTwoBundleReloads(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		fixture := epathStoragePipelineSQL(t, "positive", mixed)
		for _, policy := range []string{PurposeAllocationPolicyDirectOnly, PurposeAllocationPolicyByServicePathLoadShare} {
			for _, zone := range []string{"", "Office"} {
				t.Run(fmt.Sprintf("mixed=%v/%s/%s", mixed, policy, zone), func(t *testing.T) {
					plan, request := epathStoragePipelinePlan(fixture, policy, zone)
					if zone == "" {
						legacy := epathStoragePipelineLegacy(fixture, plan)
						epathStoragePipelineAccounting(t, legacy.Reconciliation, "annual", 100, 90, 10)
						epathStoragePipelineGraph(t, legacy.Nodes, nil, legacy.Edges, fixture, false)
						for _, period := range legacy.Periods {
							if period.ID == "M1" {
								epathStoragePipelineAccounting(t, period.Reconciliation, "M1", 100, 90, 10)
								epathStoragePipelineGraph(t, period.Nodes, nil, period.Edges, fixture, false)
							}
						}
						// Public legacy storage transports no private original context;
						// positive node metadata alone must preserve deny semantics.
						for pass := 0; pass < 2; pass++ {
							raw, err := json.Marshal(legacy)
							if err != nil {
								t.Fatal(err)
							}
							var next EnergyExplanationV1
							if err := json.Unmarshal(raw, &next); err != nil {
								t.Fatal(err)
							}
							legacy = next
						}
						upgraded := UpgradeEnergyExplanationV1(legacy)
						epathStoragePipelineGraph(t, upgraded.Nodes, upgraded.Links, nil, fixture, false)
						epathStoragePipelineAccounting(t, upgraded.Reconciliation, "annual", 100, 90, 10)
					}
					bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: fixture.InputPath, Files: fixture.Files, PurposeRunPlan: &plan}, request)
					for pass := 0; pass < 3; pass++ {
						epathStoragePipelineCheckBundle(t, bundle, fixture, zone != "")
						if pass == 2 {
							break
						}
						raw, err := json.Marshal(bundle)
						if err != nil {
							t.Fatal(err)
						}
						var next PurposeResultBundle
						if err := json.Unmarshal(raw, &next); err != nil {
							t.Fatal(err)
						}
						bundle = next
					}
				})
			}
		}
	}
}

func TestEnergyPathStorageChargeNativeZeroNULLAndAbsenceThroughActualWire(t *testing.T) {
	for _, mode := range []string{"zero", "all_null", "one_null", "absent", "no_rows"} {
		t.Run(mode, func(t *testing.T) {
			fixture := epathStoragePipelineSQL(t, mode, false)
			plan, request := epathStoragePipelinePlan(fixture, PurposeAllocationPolicyByServicePathLoadShare, "")
			bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: fixture.InputPath, Files: fixture.Files, PurposeRunPlan: &plan}, request)
			for pass := 0; pass < 3; pass++ {
				epathStoragePipelineCheckBundle(t, bundle, fixture, false)
				if pass == 2 {
					break
				}
				raw, err := json.Marshal(bundle)
				if err != nil {
					t.Fatal(err)
				}
				// Read the actual full-result source wire, not naked EnergyDataSource
				// whose legacy omitempty contract cannot represent observed zero.
				var wire struct {
					EnergyExplanation struct {
						Sources []map[string]json.RawMessage `json:"sources"`
					} `json:"energyExplanation"`
				}
				if err := json.Unmarshal(raw, &wire); err != nil {
					t.Fatal(err)
				}
				for _, source := range wire.EnergyExplanation.Sources {
					if string(source["id"]) != `"sql-rdd-51"` {
						continue
					}
					for _, key := range []string{"rawValue", "effectiveValue"} {
						value, present := source[key]
						if mode == "zero" && (!present || string(value) != "0") {
							t.Errorf("native known0 missing from actual bundle wire: %s=%s", key, value)
						}
						if mode != "zero" && present {
							t.Errorf("incomplete native observation acquired %s proof: %s", key, value)
						}
					}
				}
				var next PurposeResultBundle
				if err := json.Unmarshal(raw, &next); err != nil {
					t.Fatal(err)
				}
				bundle = next
			}
		})
	}
}

func TestEnergyPathStorageChargeStaleRootAndPeriodSummaryCannotRestoreConsumption(t *testing.T) {
	fixture := epathStoragePipelineSQL(t, "positive", true)
	plan, request := epathStoragePipelinePlan(fixture, PurposeAllocationPolicyByServicePathLoadShare, "")
	bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: fixture.InputPath, Files: fixture.Files, PurposeRunPlan: &plan}, request)
	var contextID string
	for _, node := range bundle.EnergyExplanation.Nodes {
		if len(node.storageChargeBoundaries) > 0 {
			contextID = node.ID
			break
		}
	}
	if contextID == "" {
		t.Fatal("native context prerequisite missing")
	}
	stale := EnergyExplanationSummaryItem{ID: contextID, Level: "end_use", Kind: "energy.storage_charge", EndUse: "storage_charge", Carrier: "electricity", Value: 14, RawValue: 14, AllocatedValue: 14, Unit: "kWh", SourceIDs: []string{"sql-rdd-51", "sql-rdd-52"}}
	// Append a discrete stale charge row, not an irreversibly mixed Other sum.
	// No observation or correct lighting/residual summary value is overwritten.
	bundle.EnergyExplanationSummary.EndUses = append(bundle.EnergyExplanationSummary.EndUses, stale)
	for index := range bundle.EnergyExplanation.Periods {
		period := &bundle.EnergyExplanation.Periods[index]
		if period.ID != "M1" {
			continue
		}
		if period.Summary == nil {
			summary := buildEnergyExplanationSummaryForPeriod(bundle.EnergyExplanation, "M1")
			period.Summary = &summary
		}
		period.Summary.EndUses = append(period.Summary.EndUses, stale)
	}
	for pass := 0; pass < 2; pass++ {
		raw, err := json.Marshal(bundle)
		if err != nil {
			t.Fatal(err)
		}
		var next PurposeResultBundle
		if err := json.Unmarshal(raw, &next); err != nil {
			t.Fatal(err)
		}
		bundle = next
		epathStoragePipelineCheckBundle(t, bundle, fixture, false)
	}
}
