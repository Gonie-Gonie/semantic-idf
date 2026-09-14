package simulation

// Native consumed-input integration tests. Literal observations only, no candidate or
// expected artifact and no engine. Requires root native Cogeneration reader,
// per-period budget selector, source transport and typed-role integration.
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

type epathCogenPipelineSpec struct {
	Parent, Member         string // positive, zero, absent, all_null, no_rows, partial
	ParentResource         string // empty means Electricity; nonconsumed resources remain context
	ExtraOriginal          string
	Charge                 bool
	TableMode, TableColumn string // absent/positive/zero/null; Electricity/Natural Gas
	TableValue             float64
}
type epathCogenPipelineFixture struct {
	Spec      epathCogenPipelineSpec
	Document  idf.Document
	InputPath string
	Files     []SimulationFileInfo
}

func epathCogenPipelineSQL(t *testing.T, spec epathCogenPipelineSpec) epathCogenPipelineFixture {
	t.Helper()
	text := `Version,25.1;
Zone,Office,0,0,0,0,1,1,3,60,20;
Schedule:Constant,Always,,1;
Lights,Office Lights,Office,Always,LightingLevel,1000;
ElectricLoadCenter:Distribution,LC,,,,,,DirectCurrentWithInverterDCStorage,INV,Battery;
ElectricLoadCenter:Inverter:LookUpTable,INV;
ElectricLoadCenter:Storage:Battery,Battery;
Output:Meter,Electricity:Facility,Monthly;
Output:Meter,InteriorLights:Electricity,Monthly;
Output:Meter,Cogeneration:Electricity,Monthly;
Output:Variable,Office,Zone Lights Electricity Energy,Monthly;
Output:Variable,INV,Inverter Ancillary AC Electricity Energy,Monthly;
Output:Variable,Battery,Electric Storage Charge Energy,Monthly;
` + spec.ExtraOriginal
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
	exec(`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentName TEXT,EnvironmentType INTEGER)`)
	exec(`INSERT INTO EnvironmentPeriods VALUES(1,'ANNUAL WEATHER',3)`)
	exec(`CREATE TABLE "Time"(TimeIndex INTEGER PRIMARY KEY,EnvironmentPeriodIndex INTEGER,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" REAL,WarmupFlag INTEGER,IntervalType INTEGER,SimulationDays INTEGER)`)
	exec(`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,IsMeter INTEGER,Type TEXT,IndexGroup TEXT,TimestepType TEXT,KeyValue TEXT,Name TEXT,ReportingFrequency TEXT,ScheduleName TEXT,Units TEXT)`)
	exec(`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`)
	exec(`CREATE TABLE TabularDataWithStrings(TabularDataIndex INTEGER PRIMARY KEY,ReportName TEXT,ReportForString TEXT,TableName TEXT,RowName TEXT,ColumnName TEXT,Units TEXT,RowId INTEGER,ColumnId INTEGER,Value TEXT)`)
	type dictionary struct {
		id, meter         int
		name, key, mode   string
		january, february float64
	}
	roster := []dictionary{{1, 1, "Electricity:Facility", "", "positive", 100, 0}, {2, 1, "InteriorLights:Electricity", "", "positive", 90, 0}, {3, 0, "Zone Lights Electricity Energy", "Office", "positive", 90, 0}}
	if spec.Parent == "partial" {
		roster[0].february = 20
		roster[1].february, roster[2].february = 10, 10
	}
	resource := spec.ParentResource
	if resource == "" {
		resource = "Electricity"
	}
	if spec.Parent != "absent" {
		roster = append(roster, dictionary{61, 1, "Cogeneration:" + resource, "", spec.Parent, 4, 0})
	}
	if spec.Member != "absent" {
		member := dictionary{62, 0, "Inverter Ancillary AC Electricity Energy", "INV", spec.Member, 4, 0}
		if spec.Parent == "partial" {
			member.february = 6
		}
		roster = append(roster, member)
	}
	if spec.Charge {
		roster = append(roster, dictionary{63, 0, "Electric Storage Charge Energy", "Battery", "positive", 10, 0})
	}
	if spec.TableColumn == "Natural Gas" {
		roster = append(roster, dictionary{7, 1, "NaturalGas:Facility", "", "positive", 8, 0})
	}
	for _, d := range roster {
		var key any = d.key
		timestep, indexGroup := "HVAC System", "System"
		if d.meter == 1 {
			key = nil
			timestep = "Zone"
			parts := strings.Split(d.name, ":")
			if len(parts) == 2 && parts[1] == "Facility" {
				indexGroup = "Facility:" + parts[0]
			} else if len(parts) == 2 {
				indexGroup = "Facility:" + parts[1] + ":" + parts[0]
			}
		} else if d.id == 3 {
			timestep, indexGroup = "Zone", "Zone"
		}
		exec(`INSERT INTO ReportDataDictionary VALUES(?,?,'Sum',?,?,?,?,'Monthly',NULL,'J')`, d.id, d.meter, indexGroup, timestep, key, d.name)
	}
	row := 0
	for month := 1; month <= 12; month++ {
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC)
		exec(`INSERT INTO "Time" VALUES(?,1,2017,?,?,24,0,?,NULL,3,?)`, month, month, last.Day(), last.Day()*24*60, last.YearDay())
		for _, d := range roster {
			if d.mode == "no_rows" {
				continue
			}
			var value any = 0.0
			if month == 1 {
				value = d.january * 3.6e6
			}
			if month == 2 {
				value = d.february * 3.6e6
			}
			if d.mode == "zero" {
				value = 0.0
			}
			if d.mode == "all_null" || d.mode == "partial" && month == 2 {
				value = nil
			}
			row++
			exec(`INSERT INTO ReportData VALUES(?,?,?,?)`, row, month, d.id, value)
		}
	}
	if spec.TableMode != "" && spec.TableMode != "absent" {
		var value any = fmt.Sprintf("%.6f", spec.TableValue)
		if spec.TableMode == "zero" {
			value = "        0.00"
		}
		if spec.TableMode == "null" {
			value = nil
		}
		column := spec.TableColumn
		if column == "" {
			column = "Electricity"
		}
		exec(`INSERT INTO TabularDataWithStrings VALUES(101,'AnnualBuildingUtilityPerformanceSummary','Entire Facility','End Uses','Generators',?,'kWh',12,1,?)`, column, value)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return epathCogenPipelineFixture{Spec: spec, Document: doc, InputPath: inputPath, Files: []SimulationFileInfo{{Name: "eplusout.sql", Path: path, Kind: "sqlite"}}}
}

func epathCogenPipelinePlan(f epathCogenPipelineFixture, policy string) (PurposeRunPlan, SimulationPurposeRequest) {
	request := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, AllocationPolicy: policy}
	plan := BuildPurposeRunPlan(f.Document, request)
	resource := f.Spec.ParentResource
	if resource == "" {
		resource = "Electricity"
	}
	meterName := "Cogeneration:" + resource
	for _, output := range []PurposeOutputObject{
		{ObjectType: "Output:Meter", KeyValue: meterName, ReportingFrequency: "Monthly", Fields: []idf.OutputFieldValue{{Name: "Key Name", Value: meterName}, {Name: "Reporting Frequency", Value: "Monthly"}}, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}},
		{ObjectType: "Output:Variable", KeyValue: "INV", VariableName: "Inverter Ancillary AC Electricity Energy", ReportingFrequency: "Monthly", Fields: []idf.OutputFieldValue{{Name: "Key Value", Value: "INV"}, {Name: "Variable Name", Value: "Inverter Ancillary AC Electricity Energy"}, {Name: "Reporting Frequency", Value: "Monthly"}}, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}},
	} {
		found := false
		for _, existing := range plan.OutputObjects {
			if existing.ObjectType == output.ObjectType && strings.EqualFold(existing.KeyValue, output.KeyValue) && strings.EqualFold(existing.VariableName, output.VariableName) && existing.ReportingFrequency == output.ReportingFrequency {
				found = true
			}
		}
		if !found {
			plan.OutputObjects = append(plan.OutputObjects, output)
		}
	}
	return plan, request
}

func epathCogenPipelineNear(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > 1e-9 {
		t.Errorf("%s=%g want literal %g", name, got, want)
	}
}
func epathCogenPipelineContains(ids []string, id string) bool {
	for _, actual := range ids {
		if actual == id {
			return true
		}
	}
	return false
}
func epathCogenPipelineAccounting(t *testing.T, rows []EnergyReconciliation, carrier, period string, total, explained float64) {
	t.Helper()
	id := "reconcile.energy." + carrier + "." + period
	count := 0
	for _, row := range rows {
		if row.ID == id {
			count++
			epathCogenPipelineNear(t, id+" expected", row.ExpectedValue, total)
			epathCogenPipelineNear(t, id+" explained", row.ExplainedValue, explained)
			epathCogenPipelineNear(t, id+" residual", row.ResidualValue, total-explained)
			if epathCogenPipelineContains(row.SourceIDs, "sql-rdd-63") {
				t.Error("native charge was included in consumption budget")
			}
		}
	}
	if count != 1 {
		t.Errorf("%s count=%d want1", id, count)
	}
}
func epathCogenPipelineNoProduction(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink, edges []EnergyExplanationEdge, sources []EnergyDataSource) {
	t.Helper()
	relevant := map[string]bool{"sql-rdd-61": true, "sql-rdd-62": true, "sql-rdd-63": true}
	for _, source := range sources {
		if source.SourceType == "sql_tabular" && source.TableName == "End Uses" && source.RowName == "Generators" {
			relevant[source.ID] = true
		}
	}
	contains := func(ids []string) bool {
		for _, id := range ids {
			if relevant[id] {
				return true
			}
		}
		return false
	}
	supplyNodes := map[string]bool{}
	for _, node := range nodes {
		if contains(node.SourceIDs) && (node.EndUse == "generators" || node.Kind == "energy.generators" || node.ID == "support.generators.building") {
			supplyNodes[node.ID] = true
			t.Errorf("native consumed/transfer source became produced node: %+v", node)
		}
	}
	for _, edge := range edges {
		if (contains(edge.SourceIDs) || supplyNodes[edge.FromID] || supplyNodes[edge.ToID]) && (edge.Relation == "onsite_production" || edge.Relation == "support_supply" || edge.Relation == "delivered_load") {
			t.Errorf("native consumption obtained production/thermal-delivery relation: %+v", edge)
		}
	}
	for _, link := range links {
		if (contains(link.SourceIDs) || supplyNodes[link.FromID] || supplyNodes[link.ToID]) && (link.Relation == "onsite_production" || link.Relation == "support_supply" || link.Relation == "load_to_end_use") {
			t.Errorf("native consumption obtained production/thermal-delivery link: %+v", link)
		}
	}
}

func epathCogenPipelineSourceProof(t *testing.T, sources []EnergyDataSource, f epathCogenPipelineFixture) {
	t.Helper()
	resource := f.Spec.ParentResource
	if resource == "" {
		resource = "Electricity"
	}
	for _, tc := range []struct {
		id, name, key, mode string
		meter               bool
	}{{"sql-rdd-61", "Cogeneration:" + resource, "", f.Spec.Parent, true}, {"sql-rdd-62", "Inverter Ancillary AC Electricity Energy", "INV", f.Spec.Member, false}} {
		source := energyExplanationSourceByID(sources, tc.id)
		if tc.mode == "absent" {
			if source != nil {
				t.Errorf("absent %s acquired an observation", tc.id)
			}
			continue
		}
		if source == nil {
			t.Errorf("actual %s dictionary identity was dropped in %s", tc.id, tc.mode)
			continue
		}
		if source.SourceType != "sql_report_data" || source.Name != tc.name || source.KeyValue != tc.key || source.IsMeter != tc.meter || source.SourceUnit != "J" || source.NormalizedUnit != "kWh" || source.ReportingFrequency != "Monthly" {
			t.Errorf("native source identity changed: %+v", source)
		}
		known := tc.mode == "positive" || tc.mode == "zero"
		for _, bit := range []uint8{energySourceObservedRaw, energySourceObservedEffective} {
			if energyDataSourceValueKnown(*source, bit) != known {
				t.Errorf("%s %s knownness changed bit%d", tc.id, tc.mode, bit)
			}
		}
		if known {
			want := 4.0
			if tc.mode == "zero" {
				want = 0
			}
			if tc.id == "sql-rdd-62" && f.Spec.Parent == "partial" {
				want = 10
			}
			epathCogenPipelineNear(t, tc.id+" raw", source.RawValue, want)
			epathCogenPipelineNear(t, tc.id+" effective", source.EffectiveValue, want)
		}
		if source.ZoneName != "" || source.AllocationApplied {
			t.Errorf("native global consumption became Zone measurement/allocation: %+v", source)
		}
		for _, detail := range source.ScopeDetails {
			if detail.Scope.Kind == "zone" {
				t.Errorf("unproved Zone detail on global Cogeneration source: %+v", detail)
			}
		}
	}
}

func epathCogenPipelineLegacy(f epathCogenPipelineFixture, plan PurposeRunPlan, withOriginal bool) EnergyExplanationV1 {
	context := energyDriverBuildContext{Enabled: true}
	if withOriginal {
		context = newEnergyDriverBuildContext(idf.AnalyzeGeometry(f.Document), f.Document)
	}
	legacy := buildEnergyExplanationResultFromFilesWithDriverContext(f.Files, buildEnergyDashboardResultFromFiles(f.Files), &plan, context)
	if withOriginal {
		legacy = enrichEnergyExplanationWithServicePaths(legacy, f.InputPath)
	}
	return applyEnergyExplanationV1ServicePathLoadShareAllocation(legacy)
}

func TestEnergyPathCogenerationFullPipelineParentMemberOneBudget(t *testing.T) {
	ems := `EnergyManagementSystem:MeteredOutputVariable,ExtraConsumption,ErlVar,SystemTimestep,,Electricity,Plant,OnSiteGeneration,,J;`
	for _, tc := range []struct {
		name     string
		spec     epathCogenPipelineSpec
		consumed float64
		sourceID string
	}{
		{"parent4_member4", epathCogenPipelineSpec{Parent: "positive", Member: "positive"}, 4, "sql-rdd-61"},
		{"parent4_member4_charge10", epathCogenPipelineSpec{Parent: "positive", Member: "positive", Charge: true}, 4, "sql-rdd-61"},
		{"parent4_member_absent", epathCogenPipelineSpec{Parent: "positive", Member: "absent"}, 4, "sql-rdd-61"},
		{"parent4_member_NULL", epathCogenPipelineSpec{Parent: "positive", Member: "all_null"}, 4, "sql-rdd-61"},
		{"absent_parent_sole_member4", epathCogenPipelineSpec{Parent: "absent", Member: "positive"}, 4, "sql-rdd-62"},
		{"NULL_parent_member4", epathCogenPipelineSpec{Parent: "all_null", Member: "positive"}, 0, ""},
		{"no_rows_parent_member4", epathCogenPipelineSpec{Parent: "no_rows", Member: "positive"}, 0, ""},
		{"known_zero_parent_member4", epathCogenPipelineSpec{Parent: "zero", Member: "positive"}, 0, ""},
		{"both_native_zero", epathCogenPipelineSpec{Parent: "zero", Member: "zero"}, 0, ""},
		{"absent_parent_sole_member0", epathCogenPipelineSpec{Parent: "absent", Member: "zero"}, 0, ""},
		{"absent_parent_NULL_member", epathCogenPipelineSpec{Parent: "absent", Member: "all_null"}, 0, ""},
		{"absent_parent_ambiguous_census", epathCogenPipelineSpec{Parent: "absent", Member: "positive", ExtraOriginal: "ElectricLoadCenter:Inverter:Simple,Other Inverter;"}, 0, ""},
		{"absent_parent_EMS_consumer", epathCogenPipelineSpec{Parent: "absent", Member: "positive", ExtraOriginal: ems}, 0, ""},
		{"observed_parent_EMS_consumer", epathCogenPipelineSpec{Parent: "positive", Member: "positive", ExtraOriginal: ems}, 4, "sql-rdd-61"},
		{"custom_parent_name_collision", epathCogenPipelineSpec{Parent: "positive", Member: "positive", ExtraOriginal: "Meter:Custom,Cogeneration:Electricity,Electricity;"}, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := epathCogenPipelineSQL(t, tc.spec)
			for _, policy := range []string{PurposeAllocationPolicyDirectOnly, PurposeAllocationPolicyByServicePathLoadShare} {
				t.Run(policy, func(t *testing.T) {
					plan, request := epathCogenPipelinePlan(f, policy)
					legacy := epathCogenPipelineLegacy(f, plan, true)
					epathCogenPipelineAccounting(t, legacy.Reconciliation, "electricity", "annual", 100, 90+tc.consumed)
					consumed := 0.0
					selected := false
					for _, node := range legacy.Nodes {
						if node.EndUse == "cogeneration_input" {
							consumed += node.Value
							selected = selected || epathCogenPipelineContains(node.SourceIDs, tc.sourceID)
						}
					}
					epathCogenPipelineNear(t, "native consumed input, before Other presentation", consumed, tc.consumed)
					if tc.consumed > 0 && !selected {
						t.Error("consumed subtotal lost actual parent/sole member authority")
					}
					epathCogenPipelineNoProduction(t, legacy.Nodes, nil, legacy.Edges, legacy.Sources)
					bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: f.InputPath, Files: f.Files, PurposeRunPlan: &plan}, request)
					for pass := 0; pass < 3; pass++ {
						result := bundle.EnergyExplanation
						epathCogenPipelineAccounting(t, result.Reconciliation, "electricity", "annual", 100, 90+tc.consumed)
						epathCogenPipelineNoProduction(t, result.Nodes, result.Links, nil, result.Sources)
						epathCogenPipelineSourceProof(t, result.Sources, f)
						found := false
						for _, period := range result.Periods {
							if period.ID == "M1" {
								found = true
								epathCogenPipelineAccounting(t, period.Reconciliation, "electricity", "M1", 100, 90+tc.consumed)
								epathCogenPipelineNoProduction(t, period.Nodes, period.Links, nil, result.Sources)
							}
						}
						if !found {
							t.Error("observed Monthly authority missing")
						}
						for _, zone := range result.ZoneResults {
							epathCogenPipelineNoProduction(t, zone.Nodes, zone.Links, nil, result.Sources)
							for _, node := range zone.Nodes {
								if node.Level == "end_use" && (epathCogenPipelineContains(node.SourceIDs, "sql-rdd-61") || epathCogenPipelineContains(node.SourceIDs, "sql-rdd-62")) {
									t.Error("global plant consumption allocated to Office without proof")
								}
							}
						}
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
		})
	}
}

func TestEnergyPathCogenerationPartialParentStaysPeriodLocal(t *testing.T) {
	// M1 parent4/member4 is known. M2 parent=NULL/member6 is not permission
	// to fill six from a constituent. Literal annual = sums of known budgets.
	f := epathCogenPipelineSQL(t, epathCogenPipelineSpec{Parent: "partial", Member: "positive"})
	plan, request := epathCogenPipelinePlan(f, PurposeAllocationPolicyByServicePathLoadShare)
	bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: f.InputPath, Files: f.Files, PurposeRunPlan: &plan}, request)
	for pass := 0; pass < 3; pass++ {
		epathCogenPipelineAccounting(t, bundle.EnergyExplanation.Reconciliation, "electricity", "annual", 120, 104)
		epathCogenPipelineSourceProof(t, bundle.EnergyExplanation.Sources, f)
		found := map[string]bool{}
		for _, period := range bundle.EnergyExplanation.Periods {
			switch period.ID {
			case "M1":
				found[period.ID] = true
				epathCogenPipelineAccounting(t, period.Reconciliation, "electricity", period.ID, 100, 94)
			case "M2":
				found[period.ID] = true
				epathCogenPipelineAccounting(t, period.Reconciliation, "electricity", period.ID, 20, 10)
			}
		}
		if !found["M1"] || !found["M2"] {
			t.Error("native monthly periods missing")
		}
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
}

func TestEnergyPathCogenerationNativeGeneratorsTabularIsConsumedAnnualOnly(t *testing.T) {
	for _, column := range []string{"Electricity", "Natural Gas"} {
		for _, mode := range []string{"positive", "zero", "null"} {
			t.Run(column+"/"+mode, func(t *testing.T) {
				f := epathCogenPipelineSQL(t, epathCogenPipelineSpec{Parent: "absent", Member: "absent", TableColumn: column, TableMode: mode, TableValue: 4})
				plan, request := epathCogenPipelinePlan(f, PurposeAllocationPolicyDirectOnly)
				context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(f.Document), f.Document)
				parsed, err := parseSimulationEnergyExplanationCanonicalSQL(f.Files[0].Path, &plan, context)
				if err != nil {
					t.Fatal(err)
				}
				value := 0.0
				foundSeries := false
				for _, item := range parsed.Series {
					if item.EndUse == "cogeneration_input" {
						foundSeries = true
						value += item.Total
						if item.Stage != "end_use" || len(item.Monthly) != 0 || len(item.RawMonthly) != 0 {
							t.Error("annual native table source became production or invented monthly observations")
						}
					}
				}
				want := 0.0
				if mode == "positive" {
					want = 4
				}
				epathCogenPipelineNear(t, "native table consumed scalar", value, want)
				if mode != "null" && !foundSeries {
					t.Error("actual native tabular consumed input, including zero, was dropped")
				}
				bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: f.InputPath, Files: f.Files, PurposeRunPlan: &plan}, request)
				for pass := 0; pass < 3; pass++ {
					result := bundle.EnergyExplanation
					epathCogenPipelineNoProduction(t, result.Nodes, result.Links, nil, result.Sources)
					if column == "Electricity" {
						epathCogenPipelineAccounting(t, result.Reconciliation, "electricity", "annual", 100, 90+want)
					} else {
						epathCogenPipelineAccounting(t, result.Reconciliation, "electricity", "annual", 100, 90)
						epathCogenPipelineAccounting(t, result.Reconciliation, "natural_gas", "annual", 8, want)
					}
					for _, period := range result.Periods {
						if period.ID == "M1" {
							epathCogenPipelineAccounting(t, period.Reconciliation, "electricity", "M1", 100, 90)
						}
					}
					observed := 0
					for _, source := range result.Sources {
						if source.SourceType == "sql_tabular" && source.TableName == "End Uses" && source.RowName == "Generators" {
							observed++
							if strings.Contains(strings.ToLower(source.Name), "produced") || strings.Contains(strings.ToLower(source.KeyValue), "produced") {
								t.Error("consumed native table provenance relabeled ElectricityProduced")
							}
							known := mode != "null"
							for _, bit := range []uint8{energySourceObservedRaw, energySourceObservedEffective} {
								if energyDataSourceValueKnown(source, bit) != known {
									t.Error("tabular zero/NULL observation proof changed")
								}
							}
							if known {
								epathCogenPipelineNear(t, "tabular source raw", source.RawValue, want)
								epathCogenPipelineNear(t, "tabular source effective", source.EffectiveValue, want)
							}
						}
					}
					if mode != "null" && observed != 1 {
						t.Errorf("native observed tabular source count=%d want1", observed)
					}
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

func TestEnergyPathCogenerationMonthlyParentOutranksNativeTabularWithoutDoubleCount(t *testing.T) {
	// The mismatching annual table is a deliberately separate observation, not
	// a reference used to manufacture Monthly values. Monthly parent4 wins.
	f := epathCogenPipelineSQL(t, epathCogenPipelineSpec{Parent: "positive", Member: "positive", TableMode: "positive", TableColumn: "Electricity", TableValue: 9})
	plan, request := epathCogenPipelinePlan(f, PurposeAllocationPolicyByServicePathLoadShare)
	bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: f.InputPath, Files: f.Files, PurposeRunPlan: &plan}, request)
	for pass := 0; pass < 3; pass++ {
		epathCogenPipelineAccounting(t, bundle.EnergyExplanation.Reconciliation, "electricity", "annual", 100, 94)
		epathCogenPipelineSourceProof(t, bundle.EnergyExplanation.Sources, f)
		for _, period := range bundle.EnergyExplanation.Periods {
			if period.ID == "M1" {
				epathCogenPipelineAccounting(t, period.Reconciliation, "electricity", "M1", 100, 94)
			}
		}
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
}

func TestEnergyPathCogenerationOriginalOptInDoesNotChangeLegacyNameOnlyPipeline(t *testing.T) {
	f := epathCogenPipelineSQL(t, epathCogenPipelineSpec{Parent: "positive", Member: "absent"})
	plan, _ := epathCogenPipelinePlan(f, PurposeAllocationPolicyDirectOnly)
	definition, ok := energyMeterAliasOrOtherDefinitionForName("Cogeneration:Electricity")
	if !ok || definition.EndUse != "generators" {
		t.Fatal("frozen no-original name-only alias API changed")
	}
	legacy := epathCogenPipelineLegacy(f, plan, false)
	epathCogenPipelineAccounting(t, legacy.Reconciliation, "electricity", "annual", 100, 90)
	legacySupport := false
	for _, node := range legacy.Nodes {
		if node.EndUse == "cogeneration_input" {
			t.Error("native-only consumed qualifier leaked into omitted-original builder")
		}
		if node.EndUse == "generators" && epathCogenPipelineContains(node.SourceIDs, "sql-rdd-61") {
			legacySupport = true
			epathCogenPipelineNear(t, "legacy support quantity", node.Value, 4)
		}
	}
	if !legacySupport {
		t.Error("omitted-original legacy source meaning silently replaced")
	}
	native := epathCogenPipelineLegacy(f, plan, true)
	epathCogenPipelineAccounting(t, native.Reconciliation, "electricity", "annual", 100, 94)
	epathCogenPipelineNoProduction(t, native.Nodes, nil, native.Edges, native.Sources)
}

func TestEnergyPathCogenerationNonConsumedResourcesRemainSourceOnly(t *testing.T) {
	// Atomic integration does not create a second supply graph from a native
	// end-use/resource subset. Existing four Facility meters keep their own
	// separately reviewed supply roles; none is synthesized by this fixture.
	for _, resource := range []string{"ElectricityProduced", "ElectricityPurchased", "ElectricitySurplusSold", "ElectricityNet", "EnergyTransfer"} {
		t.Run(resource, func(t *testing.T) {
			f := epathCogenPipelineSQL(t, epathCogenPipelineSpec{Parent: "positive", Member: "absent", ParentResource: resource})
			plan, request := epathCogenPipelinePlan(f, PurposeAllocationPolicyDirectOnly)
			bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: f.InputPath, Files: f.Files, PurposeRunPlan: &plan}, request)
			for pass := 0; pass < 3; pass++ {
				result := bundle.EnergyExplanation
				epathCogenPipelineAccounting(t, result.Reconciliation, "electricity", "annual", 100, 90)
				epathCogenPipelineSourceProof(t, result.Sources, f)
				check := func(nodes []EnergyExplanationNode, links []EnergyPathLink) {
					for _, node := range nodes {
						if epathCogenPipelineContains(node.SourceIDs, "sql-rdd-61") {
							t.Error("Cogeneration-specific nonconsumed resource generated a node instead of source-only context")
						}
					}
					for _, link := range links {
						if epathCogenPipelineContains(link.SourceIDs, "sql-rdd-61") {
							t.Error("Cogeneration-specific nonconsumed resource became another flow authority")
						}
					}
				}
				check(result.Nodes, result.Links)
				for _, period := range result.Periods {
					check(period.Nodes, period.Links)
				}
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
