package simulation

// Native identity integration: literal SQL only. Requires the installed native
// Cogeneration reader, SourceSnapshot-ID-first merge and exact TAB provenance.
import (
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"
)

func TestEnergyPathCogenerationOwnedSourceIDPrecedesLegacyAlias(t *testing.T) {
	foreign := energyExplanationSeries{SourceName: "Heating:Electricity", sourceName: "Heating:Electricity", SourceIDs: []string{"sql-rdd-61"}, Stage: "end_use", EndUse: "heating", Total: 4}
	legitimate := energyExplanationSeries{SourceName: "Heating:Electricity", sourceName: "Heating:Electricity", SourceIDs: []string{"sql-rdd-99"}, Stage: "end_use", EndUse: "heating", Total: 7}
	input := []energyExplanationSeries{foreign, legitimate}
	result := energyPathCogenerationReadResult{Enabled: true, SourceSnapshots: []EnergyDataSource{{ID: "sql-rdd-61", Name: "Cogeneration:Electricity", KeyValue: "Heating:Electricity"}}}
	got := mergeEnergyPathCogenerationSeries(input, result)
	if !reflect.DeepEqual(got, []energyExplanationSeries{legitimate}) {
		t.Errorf("actual Cogeneration source escaped through its foreign alias, or unrelated Heating was removed: %+v", got)
	}
	if !reflect.DeepEqual(input, []energyExplanationSeries{foreign, legitimate}) {
		t.Fatal("merge changed caller-owned series")
	}
	if !reflect.DeepEqual(mergeEnergyPathCogenerationSeries(input, energyPathCogenerationReadResult{}), input) {
		t.Fatal("omitted-original compatibility changed")
	}
}

func epathCogenIdentityWireSource(t *testing.T, result EnergyExplanationResult, id string) map[string]json.RawMessage {
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
	var found map[string]json.RawMessage
	for _, source := range wire.Sources {
		var actual string
		if err := json.Unmarshal(source["id"], &actual); err != nil {
			t.Fatal(err)
		}
		if actual == id {
			if found != nil {
				t.Fatalf("duplicate persisted source %s", id)
			}
			found = source
		}
	}
	if found == nil {
		t.Fatalf("persisted source %s missing", id)
	}
	return found
}

func TestEnergyPathCogenerationForeignHeatingKeyCannotBecomeConsumption(t *testing.T) {
	f := epathCogenPipelineSQL(t, epathCogenPipelineSpec{Parent: "positive", Member: "absent", ExtraOriginal: "Output:Variable,Office,Zone Air System Sensible Heating Energy,Monthly;"})
	db, err := sql.Open("sqlite", f.Files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, query := range []string{
		`UPDATE ReportDataDictionary SET KeyValue='Heating:Electricity' WHERE ReportDataDictionaryIndex=61`,
		`INSERT INTO ReportDataDictionary VALUES(64,0,'Sum','System','HVAC System','Office','Zone Air System Sensible Heating Energy','Monthly',NULL,'J')`,
		`INSERT INTO ReportData(ReportDataIndex,TimeIndex,ReportDataDictionaryIndex,Value) SELECT 1000+TimeIndex,TimeIndex,64,CASE WHEN Month=1 THEN 20.0*3600000.0 ELSE 0.0 END FROM "Time"`,
	} {
		if _, err := tx.Exec(query); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	checkGraph := func(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink, edges []EnergyExplanationEdge, wantLoad bool) {
		t.Helper()
		loadCount := 0
		for _, node := range nodes {
			if epathCogenPipelineContains(node.SourceIDs, "sql-rdd-61") {
				t.Errorf("invalid native meter metadata gained node authority through a legacy alias: %+v", node)
			}
			if node.Level == "load" && epathCogenPipelineContains(node.SourceIDs, "sql-rdd-64") {
				loadCount++
				if wantLoad {
					epathCogenPipelineNear(t, "independent measured heating load", node.Value, 20)
				}
			}
		}
		if wantLoad && loadCount != 1 {
			t.Errorf("independent measured heating load count=%d want 1", loadCount)
		}
		for _, link := range links {
			if epathCogenPipelineContains(link.SourceIDs, "sql-rdd-61") {
				t.Errorf("invalid Cogeneration dictionary gained flow authority: %+v", link)
			}
		}
		for _, edge := range edges {
			if epathCogenPipelineContains(edge.SourceIDs, "sql-rdd-61") {
				t.Errorf("invalid Cogeneration dictionary gained V1 edge authority: %+v", edge)
			}
		}
	}
	for _, policy := range []string{PurposeAllocationPolicyDirectOnly, PurposeAllocationPolicyByServicePathLoadShare} {
		t.Run(policy, func(t *testing.T) {
			plan, request := epathCogenPipelinePlan(f, policy)
			legacy := epathCogenPipelineLegacy(f, plan, true)
			epathCogenPipelineAccounting(t, legacy.Reconciliation, "electricity", "annual", 100, 90)
			checkGraph(t, legacy.Nodes, nil, legacy.Edges, true)
			bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: f.InputPath, Files: f.Files, PurposeRunPlan: &plan}, request)
			for pass := 0; pass < 3; pass++ {
				result := bundle.EnergyExplanation
				epathCogenPipelineAccounting(t, result.Reconciliation, "electricity", "annual", 100, 90)
				checkGraph(t, result.Nodes, result.Links, nil, true)
				source := energyExplanationSourceByID(result.Sources, "sql-rdd-61")
				if source == nil {
					t.Fatal("invalid but actually present native dictionary identity dropped")
				}
				if source.Name != "Cogeneration:Electricity" || source.KeyValue != "Heating:Electricity" || source.SourceType != "sql_report_data" || !source.IsMeter || source.SourceUnit != "J" || source.ReportingFrequency != "Monthly" {
					t.Errorf("actual raw dictionary identity was rewritten: %+v", source)
				}
				if energyDataSourceValueKnown(*source, energySourceObservedRaw) || energyDataSourceValueKnown(*source, energySourceObservedEffective) {
					t.Error("invalid native meter key became a known raw/effective observation")
				}
				wire := epathCogenIdentityWireSource(t, result, source.ID)
				for _, field := range []string{"rawValue", "effectiveValue"} {
					if _, present := wire[field]; present {
						t.Errorf("unknown %s was serialized as a numeric observation", field)
					}
				}
				loadSource := energyExplanationSourceByID(result.Sources, "sql-rdd-64")
				if loadSource == nil || !energyDataSourceValueKnown(*loadSource, energySourceObservedRaw) {
					t.Fatal("legitimate native heating load observation removed")
				}
				epathCogenPipelineNear(t, "legitimate native load raw", loadSource.RawValue, 20)
				foundJanuary := false
				for _, period := range result.Periods {
					checkGraph(t, period.Nodes, period.Links, nil, period.ID == "M1")
					if period.ID == "M1" {
						foundJanuary = true
						epathCogenPipelineAccounting(t, period.Reconciliation, "electricity", "M1", 100, 90)
					}
				}
				if !foundJanuary {
					t.Fatal("actual January period missing")
				}
				for _, zone := range result.ZoneResults {
					checkGraph(t, zone.Nodes, zone.Links, nil, false)
					for _, period := range zone.Periods {
						checkGraph(t, period.Nodes, period.Links, nil, false)
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
}

func TestEnergyPathCogenerationNativeTabularExactProvenanceTwoReloads(t *testing.T) {
	for _, mode := range []string{"positive", "zero", "null"} {
		t.Run(mode, func(t *testing.T) {
			f := epathCogenPipelineSQL(t, epathCogenPipelineSpec{Parent: "absent", Member: "absent", TableMode: mode, TableColumn: "Electricity", TableValue: 4})
			plan, request := epathCogenPipelinePlan(f, PurposeAllocationPolicyDirectOnly)
			bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: f.InputPath, Files: f.Files, PurposeRunPlan: &plan}, request)
			for pass := 0; pass < 3; pass++ {
				result := bundle.EnergyExplanation
				wire := epathCogenIdentityWireSource(t, result, "sql-tabular-cogeneration-101")
				for field, want := range map[string]string{
					"reportName": "AnnualBuildingUtilityPerformanceSummary", "reportForString": "Entire Facility",
					"tableName": "End Uses", "rowName": "Generators", "columnName": "Electricity",
					"sourceUnit": "kWh", "reportingFrequency": "Annual",
				} {
					var actual string
					if err := json.Unmarshal(wire[field], &actual); err != nil {
						t.Fatalf("missing/invalid native %s: %v", field, err)
					}
					if actual != want {
						t.Errorf("native %s=%q want %q", field, actual, want)
					}
				}
				for _, field := range []string{"rawValue", "effectiveValue"} {
					raw, present := wire[field]
					if mode == "null" {
						if present {
							t.Errorf("native NULL %s acquired a value", field)
						}
						continue
					}
					if !present || string(raw) == "null" {
						t.Fatalf("native measured %s lost scalar presence", mode)
					}
					var actual float64
					if err := json.Unmarshal(raw, &actual); err != nil {
						t.Fatal(err)
					}
					want := 4.0
					if mode == "zero" {
						want = 0
					}
					epathCogenPipelineNear(t, "native table "+field, actual, want)
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
