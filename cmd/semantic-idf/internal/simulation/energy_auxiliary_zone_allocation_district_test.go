package simulation

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const epathDistrictCoolingLoop = "Chilled Water Loop Chilled Water Loop"
const epathDistrictHeatingLoop = "Hot Water Loop Hot Water Loop"

func epathDistrictOriginal(t *testing.T) (energyServicePathIndex, idf.HVACReport) {
	t.Helper()
	path := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "5ZoneFanCoilDOAS_ERVOnAirLoopMainBranch.idf")
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(input)); got != "3b7712676a8ea7cb034cec7eb0b843731d12ba79cf72d3d54f3b3e6481b765da" {
		t.Fatalf("original District model changed: %s", got)
	}
	t.Cleanup(func() {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(input, after) {
			t.Error("auxiliary regression changed the immutable District input")
		}
	})
	return buildEnergyServicePathIndex(path), idf.AnalyzeHVAC(parsePurposePlanFixture(t, string(input)))
}

func epathDistrictZones() []string {
	return []string{"SPACE1-1", "SPACE2-1", "SPACE3-1", "SPACE4-1", "SPACE5-1"}
}

func epathDistrictAssertAuxiliaryPaths(t *testing.T, topology energyServicePathIndex) {
	t.Helper()
	eligible := energyPathZoneAuxiliaryEligiblePaths("pumps", nil, topology.auxiliaryPaths)
	owners, seenIDs := map[string]bool{}, map[string]bool{}
	for _, path := range topology.auxiliaryPaths {
		if path.ID == "" || seenIDs[path.ID] || path.ZoneName == "PLENUM-1" || path.CondenserLoopName != "" ||
			path.AirLoopName != "" && path.AirLoopName != "DOAS" {
			t.Fatalf("original service index contains duplicate/foreign/invented path: %#v", path)
		}
		seenIDs[path.ID] = true
		if path.PlantLoopName != "" {
			want := map[string]string{"cooling": epathDistrictCoolingLoop, "heating": epathDistrictHeatingLoop}[path.ServiceKind]
			if want == "" || path.PlantLoopName != want {
				t.Fatalf("district plant contains crossed or unsupported service: %#v", path)
			}
		}
	}
	for _, path := range eligible {
		owners[path.ZoneName+"|"+path.ServiceKind] = true
	}
	if len(owners) != 10 || !topology.auxiliaryResolvable["pumps"] || topology.auxiliaryResolvable["fans"] || topology.auxiliaryResolvable["heat_rejection"] {
		t.Fatalf("want five cooling/heating pump owners, mixed fans unresolved and no tower: owners=%v resolvable=%v", owners, topology.auxiliaryResolvable)
	}
	for _, zone := range epathDistrictZones() {
		for _, service := range []string{"cooling", "heating"} {
			if !owners[zone+"|"+service] {
				t.Fatalf("missing original district owner %s/%s", zone, service)
			}
		}
	}
}

func TestEPATH101DistrictOriginalPumpInventoryAndMixedFans(t *testing.T) {
	topology, report := epathDistrictOriginal(t)
	epathDistrictAssertAuxiliaryPaths(t, topology)
	pumpPaths := energyPathZoneAuxiliaryEligiblePaths("pumps", nil, topology.auxiliaryPaths)
	fanPaths := energyPathZoneAuxiliaryEligiblePaths("fans", nil, topology.auxiliaryPaths)
	doasOwners := map[string]bool{}
	for _, path := range fanPaths {
		if path.AirLoopName != "DOAS" || path.ZoneName == "PLENUM-1" {
			t.Fatalf("DOAS fan path escaped original served owners: %#v", path)
		}
		doasOwners[path.ZoneName] = true
	}
	if len(doasOwners) != 5 {
		t.Fatalf("shared DOAS must retain all five owners: %v", doasOwners)
	}
	localFans, pumps := map[string]bool{}, map[string]bool{}
	central := 0
	for _, item := range report.ServiceModel.Components {
		switch item.Component.ObjectType {
		case "ZoneHVAC:FourPipeFanCoil":
			for _, ref := range item.InternalRefs {
				if ref.ObjectType == "Fan:OnOff" {
					localFans[ref.ObjectName] = true
					if energyPathAuxiliaryComponentResolved("fans", item, fanPaths) {
						t.Fatalf("local recirculation fan was relabelled as a DOAS fan: %#v", item)
					}
				}
			}
		case "Fan:VariableVolume":
			if item.Component.ObjectName != "DOAS Supply Fan" || !energyPathAuxiliaryComponentResolved("fans", item, fanPaths) {
				t.Fatalf("actual DOAS supply fan lost its AirLoop owner: %#v", item)
			}
			central++
		case "Pump:ConstantSpeed":
			pumps[item.Component.ObjectName] = true
			if !energyPathAuxiliaryComponentResolved("pumps", item, pumpPaths) {
				t.Fatalf("original single-service plant pump lost ownership: %#v", item)
			}
		}
	}
	if len(localFans) != 5 || central != 1 || len(pumps) != 2 || !pumps["Hot Water Loop HW Supply Pump"] || !pumps["Chilled Water Loop ChW Supply Pump"] {
		t.Fatalf("original broad-meter membership changed: local=%v central=%d pumps=%v", localFans, central, pumps)
	}
	for _, zone := range epathDistrictZones() {
		if !localFans[zone+" Supply Fan"] || !doasOwners[zone] {
			t.Fatalf("missing exact local fan or DOAS owner %s", zone)
		}
	}
}

// These are independent hand quantities, not saved-SQL acceptance values.
// One load node per service/Zone carries the union of local and shared paths.
func epathDistrictGraph(topology energyServicePathIndex, period string, pumps, fans float64, cooling, heating []float64) ([]EnergyExplanationNode, []EnergyExplanationEdge, []EnergyDataSource) {
	nodes := []EnergyExplanationNode{epath101AuditCarrierFor("electricity", pumps+fans, "meter.facility."+period),
		epath101AuditAuxiliary("pumps", pumps, "meter.pumps."+period, nil), epath101AuditAuxiliary("fans", fans, "meter.fans."+period, nil)}
	sources := []EnergyDataSource{{ID: "meter.facility." + period, Name: "Electricity:Facility", SourceType: "sql_meter", IsMeter: true},
		{ID: "meter.pumps." + period, Name: "Pumps:Electricity", SourceType: "sql_meter", IsMeter: true},
		{ID: "meter.fans." + period, Name: "Fans:Electricity", SourceType: "sql_meter", IsMeter: true}}
	for i, zone := range epathDistrictZones() {
		for _, service := range []string{"cooling", "heating"} {
			value := cooling[i]
			if service == "heating" {
				value = heating[i]
			}
			source := "load." + service + "." + zone + "." + period
			nodes = append(nodes, epath101AuditLoad(zone, service, value, source, topology.byZoneService[normalizePurposeToken(zone)+"|"+service]))
			sources = append(sources, EnergyDataSource{ID: source, SourceType: "sql_variable", ZoneName: zone})
		}
	}
	for i := range nodes {
		nodes[i].Period = period
	}
	edges := []EnergyExplanationEdge{}
	for _, endUse := range []string{"pumps", "fans"} {
		value := pumps
		if endUse == "fans" {
			value = fans
		}
		edges = append(edges, EnergyExplanationEdge{ID: "facility-" + endUse + "." + period, FromID: "energy.carrier.electricity", ToID: "energy.end_use." + endUse + ".electricity",
			Value: value, Unit: "kWh site", Period: period, Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter." + endUse + "." + period}})
	}
	return nodes, edges, sources
}

func epathDistrictZonePumpPaths(topology energyServicePathIndex, zone string, cooling bool) []string {
	paths := []string{}
	for _, path := range energyPathZoneAuxiliaryEligiblePaths("pumps", nil, topology.auxiliaryPaths) {
		if path.ZoneName == zone && (cooling || path.ServiceKind == "heating") {
			paths = append(paths, path.ID)
		}
	}
	sort.Strings(paths)
	return paths
}

func TestEPATH101DistrictPumpSharesCountLoadsNotDeliveryPaths(t *testing.T) {
	original, _ := epathDistrictOriginal(t)
	epathDistrictAssertAuxiliaryPaths(t, original)
	for _, extraRepresentation := range []bool{false, true} {
		t.Run(fmt.Sprintf("extra_path_%v", extraRepresentation), func(t *testing.T) {
			topology := original
			if extraRepresentation {
				// Same validated physical route, an extra distinct trace ID for
				// SPACE1 cooling only. It is not another load, fan or pump.
				topology.auxiliaryPaths = append([]energyPathAuxiliaryServicePath(nil), original.auxiliaryPaths...)
				topology.byZoneService = make(map[string][]string, len(original.byZoneService))
				for key, ids := range original.byZoneService {
					topology.byZoneService[key] = append([]string(nil), ids...)
				}
				added := false
				for _, path := range original.auxiliaryPaths {
					if path.ZoneName == "SPACE1-1" && path.ServiceKind == "cooling" && path.PlantLoopName == epathDistrictCoolingLoop {
						path.ID = "test.additional_trace." + path.ID
						topology.auxiliaryPaths = append(topology.auxiliaryPaths, path)
						key := normalizePurposeToken(path.ZoneName) + "|cooling"
						topology.byZoneService[key] = append(topology.byZoneService[key], path.ID)
						added = true
						break
					}
				}
				if !added {
					t.Fatal("missing exact original cooling route for trace multiplicity counterexample")
				}
			}
			nodes, _, _ := epathDistrictGraph(topology, "M1", 100, 30, []float64{50, 20, 10, 10, 10}, []float64{10, 20, 30, 20, 20})
			plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "M1", "monthly", true)
			epath101AuditAssertRecord(t, plan.Records, "pumps", 100, 0, 100, 0)
			epath101AuditAssertRecord(t, plan.Records, "fans", 30, 0, 0, 30)
			if len(plan.Edges) != 5 || len(plan.Records) != 2 || len(plan.FanSourceAllocations) != 0 || epath101AuditRecord(plan.Records, "pumps").Method != "plant_loop_load_share" {
				t.Fatalf("invented direct component/pool or changed broad allocation policy: %#v", plan)
			}
			for i, zone := range epathDistrictZones() {
				edge := epath101AuditAllocationEdge(plan.Edges, zone)
				if edge == nil || edge.Value != []float64{30, 20, 20, 15, 15}[i] || edge.Period != "M1" || edge.ServiceKind != "hvac" ||
					edge.FromID != "energy.end_use.pumps.electricity" || edge.Basis != "service_path_allocation" || edge.RuleID != energyRelationshipRuleAllocatedAuxiliaryServicePath {
					t.Fatalf("%s pump share counted routes rather than one C/H load: %#v", zone, edge)
				}
				epathFanCoilAssertSet(t, edge.RelatedPathIDs, epathDistrictZonePumpPaths(topology, zone, true))
				epathFanCoilAssertSet(t, edge.SourceIDs, []string{"meter.pumps.M1", "load.cooling." + zone + ".M1", "load.heating." + zone + ".M1"})
			}
		})
	}
}

func TestEPATH101DistrictAnnualPumpSharesAreCompletedMonthlySum(t *testing.T) {
	topology, _ := epathDistrictOriginal(t)
	epathDistrictAssertAuxiliaryPaths(t, topology)
	annualNodes, annualEdges, sources := epathDistrictGraph(topology, "annual", 300, 70, []float64{50, 20, 10, 10, 10}, []float64{20, 30, 40, 30, 80})
	m1Nodes, m1Edges, m1Sources := epathDistrictGraph(topology, "M1", 100, 30, []float64{50, 20, 10, 10, 10}, []float64{10, 20, 30, 20, 20})
	m2Nodes, m2Edges, m2Sources := epathDistrictGraph(topology, "M2", 200, 40, []float64{0, 0, 0, 0, 0}, []float64{10, 10, 10, 10, 60})
	sources = append(sources, m1Sources...)
	sources = append(sources, m2Sources...)
	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{Schema: energyExplanationV1Schema, AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes: annualNodes, Edges: annualEdges, Sources: sources,
		Periods:               []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: m1Nodes, Edges: m1Edges}, {ID: "M2", Kind: "monthly", Nodes: m2Nodes, Edges: m2Edges}},
		canonicalMonthlyBasis: true, servicePathIndex: topology})
	if len(result.ZoneResults) != 5 {
		t.Fatalf("expected exactly five conditioned Zone projections, got %d", len(result.ZoneResults))
	}
	for i, name := range epathDistrictZones() {
		zone := epath101AuditZoneResult(result.ZoneResults, name)
		if zone == nil {
			t.Fatalf("missing original owner %s", name)
		}
		id := "end_use.pumps." + metricID(name)
		annual := epath101AuditNode(zone.Nodes, id)
		want := []float64{50, 40, 40, 35, 135}[i]
		if annual == nil || annual.Value != want || annual.Basis != "service_path_allocation" || !annual.AllocationApplied || epath101AuditMonthlyNodeSum(zone.Periods, id) != want {
			t.Fatalf("%s annual pump share must sum months, not redistribute annual C/H loads: %#v", name, annual)
		}
		if epath101AuditNode(zone.Nodes, "end_use.fans."+metricID(name)) != nil {
			t.Fatal("unassigned mixed fans became a Zone energy observation")
		}
		for _, period := range []struct {
			id     string
			values []float64
		}{{"M1", []float64{30, 20, 20, 15, 15}}, {"M2", []float64{20, 20, 20, 20, 120}}} {
			actual := epath101AuditPeriod(zone.Periods, period.id)
			if actual == nil {
				t.Fatalf("%s lost actual monthly period %s", name, period.id)
			}
			node := epath101AuditNode(actual.Nodes, id)
			if node == nil || node.Value != period.values[i] || !node.AllocationApplied || node.Basis != "service_path_allocation" {
				t.Fatalf("%s %s allocation changed: %#v", name, period.id, node)
			}
			if stringSliceContains(node.SourceIDs, "meter.pumps.annual") || !stringSliceContains(node.SourceIDs, "meter.pumps."+period.id) {
				t.Fatalf("monthly pump trace borrowed annual authority: %#v", node)
			}
		}
	}
	for _, endUse := range []string{"pumps", "fans"} {
		row := epath101AuditAuxiliaryReconciliation(result.Reconciliation, endUse, "electricity", "annual")
		want, allocated, unassigned := 300.0, 300.0, 0.0
		if endUse == "fans" {
			want, allocated, unassigned = 70, 0, 70
		}
		if row == nil || row.ExpectedValue != want || row.DirectValue != 0 || row.AllocatedValue != allocated || row.UnassignedValue != unassigned || row.ResidualValue != unassigned {
			t.Fatalf("annual broad accounting lost its observed/allocated/unassigned boundary: %#v", row)
		}
	}
}

func TestEPATH101DistrictAmbiguousPumpInventoryCannotBeLaunderedByScope(t *testing.T) {
	original, report := epathDistrictOriginal(t)
	epathDistrictAssertAuxiliaryPaths(t, original)
	for _, mutation := range []string{"mixed heating", "ventilation", "service_water_heating", "unknown", "missing original pump owner", "extra unresolved pump"} {
		t.Run(mutation, func(t *testing.T) {
			topology := original
			topology.auxiliaryPaths = append([]energyPathAuxiliaryServicePath(nil), original.auxiliaryPaths...)
			components := append([]idf.ComponentIndexItem(nil), report.ServiceModel.Components...)
			if mutation == "missing original pump owner" || mutation == "extra unresolved pump" {
				found := false
				for i := range components {
					if components[i].Component.ObjectType != "Pump:ConstantSpeed" || components[i].Component.ObjectName != "Hot Water Loop HW Supply Pump" {
						continue
					}
					item := components[i]
					item.RelatedPathIDs, item.RelatedLoopRefs, item.Occurrences = nil, nil, nil
					if mutation == "extra unresolved pump" {
						item.Component.ID, item.Component.ObjectName = "test.unresolved.pump", "Unresolved Extra Pump"
						components = append(components, item)
					} else {
						components[i] = item
					}
					found = true
					break
				}
				if !found {
					t.Fatal("missing exact original HW pump for inventory counterexample")
				}
			} else {
				for i := range topology.auxiliaryPaths {
					path := &topology.auxiliaryPaths[i]
					if path.PlantLoopName == epathDistrictHeatingLoop {
						path.PlantLoopName = epathDistrictCoolingLoop
						path.ServiceKind = mutation
						if mutation == "mixed heating" {
							path.ServiceKind = "heating"
						}
					}
				}
			}
			topology.auxiliaryResolvable = energyPathAuxiliaryResolvableInventory(components, topology.auxiliaryPaths)
			if topology.auxiliaryResolvable["pumps"] {
				t.Fatal("global malformed/unresolved pump pool was certified using a surviving subset")
			}
			coolingPath := ""
			for _, path := range original.auxiliaryPaths {
				if path.ZoneName == "SPACE1-1" && path.ServiceKind == "cooling" && path.PlantLoopName == epathDistrictCoolingLoop {
					coolingPath = path.ID
					break
				}
			}
			if coolingPath == "" {
				t.Fatal("missing original selected-zone cooling path")
			}
			for _, restricted := range [][]string{nil, {coolingPath}} {
				nodes, _, _ := epathDistrictGraph(original, "M1", 100, 30, []float64{50, 20, 10, 10, 10}, []float64{10, 20, 30, 20, 20})
				nodes[1].RelatedPathIDs = restricted
				plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "M1", "monthly", true)
				epath101AuditAssertRecord(t, plan.Records, "pumps", 100, 0, 0, 100)
				if len(plan.Edges) != 0 || epath101AuditRecord(plan.Records, "pumps").Method != "unassigned" {
					t.Fatalf("restricted path %v laundered unresolved broad pool: %#v", restricted, plan)
				}
				epathFanCoilAssertSet(t, epath101AuditRecord(plan.Records, "pumps").SourceIDs, []string{"meter.pumps.M1"})
			}
		})
	}
}

func TestEPATH101DistrictDOASVolumeCannotBoundSixFanPool(t *testing.T) {
	topology, _ := epathDistrictOriginal(t)
	epathDistrictAssertAuxiliaryPaths(t, topology)
	doasPaths := []string{}
	for _, path := range energyPathZoneAuxiliaryEligiblePaths("fans", nil, topology.auxiliaryPaths) {
		doasPaths = append(doasPaths, path.ID)
	}
	if len(doasPaths) == 0 {
		t.Fatal("original DOAS paths must exist even though the broad fan pool is unresolved")
	}
	for _, restricted := range [][]string{nil, doasPaths} {
		nodes, _, _ := epathDistrictGraph(topology, "M1", 100, 30, []float64{50, 20, 10, 10, 10}, []float64{10, 20, 30, 20, 20})
		nodes[2].RelatedPathIDs = restricted
		// The manual source is a shared DOAS outlet, not five individual
		// Zone air volumes or six measured fan electricity constituents.
		nodes = append(nodes, EnergyExplanationNode{ID: "test.doas.volume", Level: "airflow", Kind: "airflow.supply_air_volume", Value: 1000, Unit: "m3",
			SourceIDs: []string{"manual.doas.supply.volume"}, RelatedPathIDs: doasPaths})
		plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "M1", "monthly", true)
		epath101AuditAssertRecord(t, plan.Records, "fans", 30, 0, 0, 30)
		fan := epath101AuditRecord(plan.Records, "fans")
		if fan.Method != "unassigned" || len(plan.FanSourceAllocations) != 0 {
			t.Fatalf("one shared airflow licensed the entire mixed fan pool: %#v", plan)
		}
		epathFanCoilAssertSet(t, fan.SourceIDs, []string{"meter.fans.M1"})
		for _, edge := range plan.Edges {
			if edge.FromID == "energy.end_use.fans.electricity" || stringSliceContains(edge.SourceIDs, "manual.doas.supply.volume") {
				t.Fatalf("shared DOAS volume invented a fan branch or contaminated pump weights: %#v", edge)
			}
		}
	}
}
