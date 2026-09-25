package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Literal native SQL enters the real parser. Recipients and service paths come
// from the pinned original, not fabricated path IDs. The explicit facility
// meter forces the real carrier-ribbon projection that used to lose the paid
// source even while the allocated node, conversion and source cache retained it.
func TestEnergyPathCentralHeatPumpNativeConstituentsReachCarrierRibbons(t *testing.T) {
	path, db, plan, _, _ := centralHeatPumpMonthlyFixture(t)
	doc := energyPathCentralHeatPumpOriginal(t)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	plan.AllocationPolicy = PurposeAllocationPolicyByServicePathLoadShare
	if len(context.CentralHeatPumpPaths) != 2 {
		t.Fatalf("original paid cohorts missing: %#v", context.CentralHeatPumpPaths)
	}
	for _, paths := range context.CentralHeatPumpPaths {
		if len(paths) != 5 {
			t.Fatalf("partial original recipient cohort: %#v", paths)
		}
	}
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
	exec(`INSERT INTO ReportDataDictionary VALUES(99,'','Electricity:Facility','J','Monthly',1,'Sum','Zone','Facility',NULL)`)
	for month := 1; month <= 12; month++ {
		joules := 14 * 3600000
		if month == 1 {
			joules = 4 * 3600000
		}
		exec(`INSERT INTO ReportData VALUES(?,?,99,?)`, 9900+month, month, joules)
	}
	for serviceIndex, service := range []string{"Cooling", "Heating"} {
		for zoneIndex := 1; zoneIndex <= 6; zoneIndex++ {
			zone, joules := fmt.Sprintf("SPACE%d-1", zoneIndex), 10*3600000
			if zoneIndex == 6 {
				zone, joules = "PLENUM-1", 1000*3600000
			}
			dictionary := 200 + serviceIndex*10 + zoneIndex
			exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,'J','Monthly',0,'Sum','Zone','Zone',NULL)`, dictionary, zone, "Zone Air System Sensible "+service+" Energy")
			for month := 1; month <= 12; month++ {
				exec(`INSERT INTO ReportData VALUES(?,?,?,?)`, dictionary*100+month, month, dictionary, joules)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.hvacConsumptionPools) != 2 {
		t.Fatalf("parser omitted native paid pools: %#v", legacy.hvacConsumptionPools)
	}
	originalPath := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "CentralChillerHeaterSystem_Simultaneous_Cooling_Heating.idf")
	legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(enrichEnergyExplanationWithServicePaths(legacy, originalPath))
	result := UpgradeEnergyExplanationV1(legacy)
	check := func(result EnergyExplanationResult) {
		t.Helper()
		branches, served := 0, 0
		for _, zone := range result.ZoneResults {
			isServed := strings.HasPrefix(zone.Scope.ZoneName, "SPACE")
			if isServed {
				served++
			} else if zone.Scope.ZoneName != "PLENUM-1" {
				t.Fatalf("unexpected recipient %q", zone.Scope.ZoneName)
			}
			periods := append([]EnergyPeriod{{ID: "annual", Nodes: zone.Nodes, Links: zone.Links}}, zone.Periods...)
			for _, period := range periods {
				if period.ID != "annual" && !strings.HasPrefix(period.ID, "M") {
					continue
				}
				nodeByID := map[string]EnergyExplanationNode{}
				for _, node := range period.Nodes {
					nodeByID[node.ID] = node
				}
				seen := map[string]bool{}
				for _, link := range period.Links {
					if link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier" {
						continue
					}
					endUse := nodeByID[link.FromID]
					if endUse.EndUse != "cooling" && endUse.EndUse != "heating" {
						continue
					}
					parent, paid, want := "sql-rdd-30", "sql-rdd-10", 2.0
					if endUse.EndUse == "heating" {
						parent, paid, want = "sql-rdd-31", "sql-rdd-11", 0.8
					}
					if period.ID == "annual" {
						want = 22
						if endUse.EndUse == "heating" {
							want = 9.6
						}
					}
					if !isServed || endUse.EndUse == "cooling" && period.ID == "M1" || seen[endUse.EndUse] {
						t.Fatalf("unserved/known-zero/duplicate branch: %s/%s %#v", zone.Scope.ZoneName, period.ID, link)
					}
					seen[endUse.EndUse] = true
					branches++
					if link.Basis != "service_path_allocation" || link.RuleID != "meter.end_use" || link.FromValue != want || link.ToValue != want || len(link.SourceIDs) != 2 || !stringSliceContains(link.SourceIDs, parent) || !stringSliceContains(link.SourceIDs, paid) {
						t.Fatalf("native allocated ribbon lost exact paid lineage or borrowed facility/thermal/sibling evidence: %s/%s %#v", zone.Scope.ZoneName, period.ID, link)
					}
				}
				wantCount := 0
				if isServed {
					wantCount = 2
					if period.ID == "M1" {
						wantCount = 1
					}
				}
				if len(seen) != wantCount {
					t.Fatalf("missing actual service branches: %s/%s got%d want%d", zone.Scope.ZoneName, period.ID, len(seen), wantCount)
				}
			}
		}
		// Root annual and cached annual are both checked, plus all 12 months.
		if served != 5 || branches != 135 {
			t.Fatalf("whole native five-recipient/month/annual census changed: served%d branches%d", served, branches)
		}
		for _, link := range result.Links {
			if link.Relation == "end_use_to_carrier" && (stringSliceContains(link.SourceIDs, "sql-rdd-10") || stringSliceContains(link.SourceIDs, "sql-rdd-11")) {
				t.Fatalf("Building broad meter became an extra paid-source allocation: %#v", link)
			}
		}
	}
	check(result)
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var reopened EnergyExplanationResult
	if err := json.Unmarshal(wire, &reopened); err != nil {
		t.Fatal(err)
	}
	check(reopened)
}

func TestEnergyPathHVACConsumptionCarrierTraceKeepsPositiveSiblingZonesDisjoint(t *testing.T) {
	carrier := epath100AuditCarrierNode("electricity", 15, "M1", "facility")
	endUse := epath100AuditEndUseNode("heating", "electricity", 15, "M1", "meter", []string{"heating.Office", "heating.Lab"})
	nodes := []EnergyExplanationNode{carrier, endUse}
	topology := energyServicePathIndex{byZoneService: map[string][]string{}}
	pool := energyPathHVACConsumptionPool{ID: "heating.electricity", ServiceKind: "heating", Carrier: "electricity", MeterSourceIDs: []string{"meter"}, Valid: true}
	for i, zone := range []string{"Office", "Lab"} {
		path, paid := "heating."+zone, "paid."+zone
		topology.byZoneService[normalizePurposeToken(zone)+"|heating"] = []string{path}
		nodes = append(nodes, epath100AuditLoadNode(zone, "heating", 1, "M1", "load."+zone, []string{path}))
		pool.Members = append(pool.Members, energyPathHVACConsumptionMember{ID: paid, ObjectType: "Boiler:HotWater", ObjectName: zone + " Boiler", OutputName: "Boiler Ancillary Electricity Energy", RelatedPathIDs: []string{path}, Series: epathHVACConsumptionPoolSeries("", paid, float64(5*(i+1)))})
	}
	edges := []EnergyExplanationEdge{epath100AuditMeterEdge("M1", carrier, endUse)}
	plan := buildEnergyPathZoneHVACAllocationPlan(nodes, edges, nil, "M1", "monthly", true, topology)
	plan = reserveEnergyPathHVACConsumptionPools(plan, nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
	index := energyPathHVACConsumptionSourceIndex(plan)
	key := energyPathZoneHVACAllocationGroupKey("heating", "electricity") + "\x00"
	if len(plan.ConsumptionSourceAllocations) != 2 || len(index) != 2 || !index[key+"paid.Office"] || !index[key+"paid.Lab"] {
		t.Fatalf("fixture did not retain both positive same-service/carrier sources: %#v %#v", plan.ConsumptionSourceAllocations, index)
	}
	nodes = qualifyEnergyPathHVACConsumptionAllocationNodes(nodes, plan)
	edges = applyEnergyPathZoneHVACAllocationPlan(edges, nodes, plan)
	for i, zone := range []string{"Office", "Lab"} {
		t.Run(zone, func(t *testing.T) {
			_, links := upgradeEnergyExplanationGraph(nodes, edges, nil, EnergyExplanationScope{Kind: "zone", ZoneName: zone}, PurposeAllocationPolicyByServicePathLoadShare, true, nil, index)
			branches := 0
			for _, link := range links {
				if link.Relation != "end_use_to_carrier" {
					continue
				}
				branches++
				want := float64(5 * (i + 1))
				if link.Basis != "service_path_allocation" || link.FromValue != want || link.ToValue != want || len(link.SourceIDs) != 2 || !stringSliceContains(link.SourceIDs, "meter") || !stringSliceContains(link.SourceIDs, "paid."+zone) {
					t.Fatalf("selected ribbon borrowed its positive sibling or facility/load evidence: %#v", link)
				}
			}
			if branches != 1 {
				t.Fatalf("got%d carrier ribbons, want one actual selected source", branches)
			}
		})
	}
}

func TestEnergyPathHVACConsumptionCarrierTraceRequiresExactPositiveReservation(t *testing.T) {
	row := energyPathHVACConsumptionSourceAllocation{ServiceKind: "heating", Carrier: "electricity", ZoneName: "Office", SourceIDs: []string{"paid"}, ObservedValue: 10, AllocatedValue: 5}
	endpoint := EnergyExplanationNode{EndUse: "heating", Carrier: "electricity", Basis: "service_path_allocation", AllocationApplied: true, hvacConsumptionPoolBound: true, allocationSourceIDs: []string{"meter", "paid", "load", "context", "foreign-paid"}}
	for _, scenario := range []string{"positive", "missing row", "zero row", "NaN row", "positive infinity row", "negative infinity row", "other carrier", "other service", "not bound", "not allocated", "direct endpoint", "no selected source", "two sources"} {
		t.Run(scenario, func(t *testing.T) {
			allocation, node := row, endpoint
			rows := []energyPathHVACConsumptionSourceAllocation{allocation}
			switch scenario {
			case "missing row":
				rows = nil
			case "zero row":
				rows[0].AllocatedValue = 0
			case "NaN row":
				rows[0].AllocatedValue = math.NaN()
			case "positive infinity row":
				rows[0].AllocatedValue = math.Inf(1)
			case "negative infinity row":
				rows[0].AllocatedValue = math.Inf(-1)
			case "other carrier":
				rows[0].Carrier = "natural_gas"
			case "other service":
				rows[0].ServiceKind = "cooling"
			case "not bound":
				node.hvacConsumptionPoolBound = false
			case "not allocated":
				node.AllocationApplied = false
			case "direct endpoint":
				node.Basis = "direct_zone_energy"
			case "no selected source":
				node.allocationSourceIDs = []string{"meter", "load"}
			case "two sources":
				rows[0].SourceIDs = []string{"paid", "context"}
			}
			got := appendEnergyPathHVACConsumptionSources([]string{"meter"}, node, energyPathHVACConsumptionSourceIndex(energyPathZoneHVACAllocationPlan{ConsumptionSourceAllocations: rows}))
			want := []string{"meter"}
			if scenario == "positive" {
				want = append(want, "paid")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("carrier-qualified source trace got%v want%v", got, want)
			}
		})
	}
}
