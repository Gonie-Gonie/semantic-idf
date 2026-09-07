package simulation

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// Exercise the new-result entrypoint, not a hand-written v2 graph or just the
// compatibility adapter. Monthly SQL contains both HVAC services, ten direct
// end-use categories, two zones, and non-flow source correspondence.
func TestEPATH190CanonicalPrimaryDirectionsAcrossEveryScopeAndPeriod(t *testing.T) {
	result := epath190BuildCanonicalSQL(t)
	if result.Schema != energyExplanationSchema || result.Scope.Kind != "building" || len(result.ZoneResults) != 2 || len(result.Periods) < 3 {
		t.Fatalf("fixture did not produce Building, two Zones, and annual/monthly graphs: schema=%q scope=%#v zones=%d periods=%d", result.Schema, result.Scope, len(result.ZoneResults), len(result.Periods))
	}
	type graph struct {
		name     string
		building bool
		nodes    []EnergyExplanationNode
		links    []EnergyPathLink
	}
	graphs := []graph{{"Building/top-level annual", true, result.Nodes, result.Links}}
	for _, period := range result.Periods {
		graphs = append(graphs, graph{"Building/" + period.ID, true, period.Nodes, period.Links})
	}
	for _, zone := range result.ZoneResults {
		if len(zone.Periods) < 2 {
			t.Fatalf("Zone %q lacks monthly graphs", zone.Scope.ZoneName)
		}
		graphs = append(graphs, graph{zone.Scope.ZoneName + "/top-level annual", false, zone.Nodes, zone.Links})
		for _, period := range zone.Periods {
			graphs = append(graphs, graph{zone.Scope.ZoneName + "/" + period.ID, false, period.Nodes, period.Links})
		}
	}
	for _, graph := range graphs {
		t.Run(graph.name, func(t *testing.T) {
			byID := map[string]EnergyExplanationNode{}
			for _, node := range graph.nodes {
				if node.ID == "" {
					t.Fatal("empty canonical node ID")
				}
				if _, exists := byID[node.ID]; exists {
					t.Fatalf("duplicate node ID %q", node.ID)
				}
				byID[node.ID] = node
			}
			counts := map[string]int{}
			conversions := map[string]int{}
			physicalIncoming, physicalOutgoing := map[string]int{}, map[string]int{}
			linkIDs := map[string]bool{}
			for _, link := range graph.links {
				from, fromOK := byID[link.FromID]
				to, toOK := byID[link.ToID]
				if !fromOK || !toOK {
					t.Fatalf("missing endpoint for %#v", link)
				}
				if link.ID == "" || linkIDs[link.ID] {
					t.Fatalf("empty or duplicate canonical link ID %q", link.ID)
				}
				linkIDs[link.ID] = true
				if from.Level == "carrier" && to.Level == "end_use" || from.Level == "load" && to.Level == "driver" {
					t.Fatalf("forbidden reverse path: %#v", link)
				}
				var expected [2]string
				switch link.Relation {
				case "driver_to_load":
					expected = [2]string{"driver", "load"}
				case "load_to_end_use":
					expected = [2]string{"load", "end_use"}
					conversions[link.ServiceKind]++
				case "end_use_to_carrier", "direct_end_use_to_carrier":
					expected = [2]string{"end_use", "carrier"}
				case "source_correspondence":
					// A source relationship may skip the Load stage, but it is not a
					// physical path and must not count as input to a direct end use.
					if from.Level != "driver" || to.Level != "end_use" {
						t.Fatalf("invalid non-flow correspondence: %#v", link)
					}
					counts[link.Relation]++
					continue
				case "support_supply", "residual":
					continue // Context, outside the primary four stages.
				default:
					t.Fatalf("unexpected relation in canonical result: %#v", link)
				}
				if from.Level != expected[0] || to.Level != expected[1] {
					t.Fatalf("%s must be %s → %s, got %s → %s: %#v", link.Relation, expected[0], expected[1], from.Level, to.Level, link)
				}
				counts[link.Relation]++
				physicalOutgoing[from.ID]++
				physicalIncoming[to.ID]++
			}
			for _, relation := range []string{"driver_to_load", "load_to_end_use", "direct_end_use_to_carrier", "source_correspondence"} {
				if counts[relation] == 0 {
					t.Fatalf("fixture made %s assertion vacuous; counts=%v", relation, counts)
				}
			}
			if conversions["cooling"] == 0 || conversions["heating"] == 0 || counts["end_use_to_carrier"] == 0 {
				t.Fatalf("both HVAC services must form complete three-stage primary paths: conversions=%v counts=%v", conversions, counts)
			}
			directCategories := map[string]bool{}
			for _, node := range graph.nodes {
				if node.Level != "end_use" || node.EndUse == "cooling" || node.EndUse == "heating" {
					continue
				}
				directCategories[node.EndUse] = true
				if physicalIncoming[node.ID] != 0 {
					t.Fatalf("direct end use %s has a primary load/driver input", node.ID)
				}
				if physicalOutgoing[node.ID] == 0 {
					t.Fatalf("direct end use %s lacks its end_use → carrier branch", node.ID)
				}
				for _, link := range graph.links {
					if link.FromID == node.ID && link.Relation != "source_correspondence" && link.Relation != "support_supply" && link.Relation != "residual" && link.Relation != "direct_end_use_to_carrier" && link.Relation != "end_use_to_carrier" {
						t.Fatalf("direct end use has a non-carrier primary output: %#v", link)
					}
				}
			}
			wantDirect := []string{"lighting", "equipment"}
			if graph.building {
				wantDirect = append(wantDirect, "fans", "pumps", "heat_rejection", "humidification", "refrigeration", "water_systems", "heat_recovery", "other")
			}
			for _, category := range wantDirect {
				if !directCategories[category] {
					t.Errorf("missing direct category %q; fixture categories=%v", category, directCategories)
				}
			}
		})
	}
}

func epath190BuildCanonicalSQL(t *testing.T) EnergyExplanationResult {
	t.Helper()
	dir := t.TempDir()
	inputPath, sqlPath := filepath.Join(dir, "model.idf"), filepath.Join(dir, "eplusout.sql")
	if err := os.WriteFile(inputPath, []byte(energyPathScopeFixtureIDF), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT, IsMeter INTEGER, ReportingFrequency TEXT, IndexGroup TEXT)`,
		`CREATE TABLE "Time" (TimeIndex INTEGER PRIMARY KEY, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER)`,
		`CREATE TABLE ReportData (ReportDataIndex INTEGER PRIMARY KEY, TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		`INSERT INTO "Time" VALUES (1,1,31,24,0),(2,2,28,24,0)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	type output struct {
		key, name string
		value     float64
		meter     bool
	}
	outputs := []output{
		{"", "Electricity:Facility", 68, true}, {"", "NaturalGas:Facility", 100, true},
		{"", "Cooling:Electricity", 25, true}, {"", "Heating:NaturalGas", 100, true},
		{"", "InteriorLights:Electricity", 10, true}, {"", "InteriorEquipment:Electricity", 8, true},
		{"", "Fans:Electricity", 5, true}, {"", "Pumps:Electricity", 2, true}, {"", "HeatRejection:Electricity", 3, true}, {"", "Humidification:Electricity", 2, true}, {"", "Refrigeration:Electricity", 4, true}, {"", "WaterSystems:Electricity", 6, true},
		{"", "HeatRecovery:Electricity", 1, true}, {"", "Other:Electricity", 2, true},
	}
	for _, zone := range []string{"Office", "Lab"} {
		outputs = append(outputs,
			output{zone, "Zone Air System Sensible Cooling Energy", 50, false}, output{zone, "Zone Air System Sensible Heating Energy", 42.5, false},
			output{zone, "Zone Lights Convective Heating Energy", 2, false}, output{zone, "Zone Electric Equipment Convective Heating Energy", 3, false},
			output{zone, "Zone Lights Electricity Energy", 5, false}, output{zone, "Zone Electric Equipment Electricity Energy", 4, false},
		)
	}
	for i, item := range outputs {
		isMeter, group := 0, "Zone"
		if item.meter {
			isMeter, group = 1, "Meter"
		}
		if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES (?,?,?,?,?,?,?)`, i+1, item.key, item.name, "J", isMeter, "Monthly", group); err != nil {
			db.Close()
			t.Fatal(err)
		}
		for month := 1; month <= 2; month++ {
			if _, err := db.Exec(`INSERT INTO ReportData VALUES (?,?,?,?)`, i*2+month, month, i+1, item.value*float64(month)*3600000); err != nil {
				db.Close()
				t.Fatal(err)
			}
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare, ZoneMode: "all"}
	run := SimulationRunResult{RunID: "direction190", InputPath: inputPath, OutputDirectory: dir, PurposeRunPlan: plan, Files: []SimulationFileInfo{{Name: filepath.Base(sqlPath), Path: sqlPath, Kind: "sqlite", Size: info.Size()}}}
	bundle := BuildPurposeResultBundle(&run, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare})
	if len(bundle.EnergyExplanation.Nodes) == 0 {
		t.Fatalf("canonical SQL builder returned no Energy Path nodes: %+v", bundle.Completeness)
	}
	return bundle.EnergyExplanation
}
