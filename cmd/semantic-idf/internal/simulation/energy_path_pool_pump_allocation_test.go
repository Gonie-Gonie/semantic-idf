package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

// Projection hand math. The original water-side inventory is independently
// hash-bound; air-route proof is an explicit hand input here, exercised against
// the complete original OA/terminal topology in its separate native tests.
func energyPathPoolPumpHand(t *testing.T) (energyPathZoneAuxiliaryAllocationPlan, []EnergyExplanationNode, energyServicePathIndex, energyPathPoolEvidence) {
	t.Helper()
	inventory := energyPathNativePoolInventory(energyPathPoolOriginal(t))
	var paths []energyPathAuxiliaryServicePath
	var nodes []EnergyExplanationNode
	var loads []energyExplanationSeries
	byZone := map[string][]string{}
	for i, weight := range []float64{1, 2, 3, 4, 10} {
		zone := fmt.Sprintf("SPACE%d-1", i+1)
		for _, service := range []string{"cooling", "heating"} {
			path := service + "." + strings.ToLower(zone)
			plant := "Chilled Water Loop"
			if service == "heating" {
				plant = "Hot Water Loop"
			}
			paths = append(paths, energyPathAuxiliaryServicePath{ID: path, ZoneName: zone, ServiceKind: service, PlantLoopName: plant, AirLoopName: "VAV Sys 1"})
			byZone[strings.ToLower(zone)+"|"+service] = []string{path}
			sourceID := fmt.Sprintf("sql-rdd-%d", 110+i)
			if service == "heating" {
				sourceID = fmt.Sprintf("sql-rdd-%d", 120+i)
			}
			nodes = append(nodes, epath101AuditLoad(zone, service, weight, sourceID, []string{path}))
			item := energyExplanationSeries{Stage: "load", Level: "load", Unit: "kWh", ZoneName: zone, ServiceKind: service, SourceIDs: []string{sourceID}, MonthlySourceIDs: []string{sourceID}, Monthly: map[int]float64{1: weight}, RawMonthly: map[int]float64{1: weight}, Total: weight, RawTotal: weight, EffectiveMultiplier: 1}
			for month := 2; month <= 12; month++ {
				item.Monthly[month], item.RawMonthly[month] = 0, 0
			}
			loads = append(loads, item)
		}
	}
	// Positive PLENUM observation is not a served owner. Neither load magnitude
	// nor the original multiplier gives this return plenum a delivery path.
	nodes = append(nodes, epath101AuditLoad("PLENUM-1", "cooling", 999, "sql-rdd-199", nil))
	var coolingPaths []string
	for _, path := range paths {
		if path.ServiceKind == "cooling" {
			coolingPaths = append(coolingPaths, path.ID)
		}
	}
	for at := range inventory.Loops {
		loop := &inventory.Loops[at]
		if loop.Component.ObjectName != "Chilled Water Loop" {
			continue
		}
		loop.AirRoutesComplete = true
		for demand := range loop.Demands {
			loop.Demands[demand].AirRouteComplete = true
			loop.Demands[demand].RelatedPathIDs = append([]string(nil), coolingPaths...)
		}
	}
	evidence := energyPathPoolEvidence{Inventory: inventory, LoadSeries: loads}
	definition, _, found := energyPathPoolOutputDefinitionForName("Pump Electricity Energy")
	if !found {
		t.Fatal("missing native pump definition")
	}
	for _, source := range inventory.Sources {
		if source.Kind != "pump" {
			continue
		}
		value, id := 100.0, "sql-rdd-71"
		if source.Component.ObjectName == "CW Circ Pump" {
			value, id = 20, "sql-rdd-72"
		}
		item := energyExplanationSeries{Stage: "context", Level: "context", Unit: "kWh", SourceIDs: []string{id}, MonthlySourceIDs: []string{id}, Monthly: map[int]float64{1: value}, RawMonthly: map[int]float64{1: value}, Total: value, RawTotal: value, EffectiveMultiplier: 1, sourceFrequency: "Monthly"}
		for month := 2; month <= 12; month++ {
			item.Monthly[month], item.RawMonthly[month] = 0, 0
		}
		evidence.Observations = append(evidence.Observations, energyPathPoolObservation{Definition: definition, Component: source.Component, Loop: source.Loop, Series: item, Valid: true})
	}
	owner := epath101AuditAuxiliary("pumps", 120, "sql-rdd-2", epath101AuditPathIDs(paths))
	nodes = append([]EnergyExplanationNode{owner}, nodes...)
	plan := energyPathZoneAuxiliaryAllocationPlan{
		CentralEndUseNodeIDs: map[string]bool{owner.ID: true}, SourceExpectedByNode: map[string]float64{owner.ID: 120},
		Records: []energyPathZoneAuxiliaryAllocationRecord{{Period: "M1", EndUse: "pumps", Carrier: "electricity", Unit: "kWh", ExpectedValue: 120, AllocatedValue: 120, Method: "service_load_share", SourceIDs: []string{"sql-rdd-2"}}},
		Edges: []EnergyExplanationEdge{
			{ID: "stale.broad.pump.allocation", FromID: owner.ID, ToID: nodes[1].ID, Value: 120, Unit: "kWh", Relation: energyPathAuxiliaryAllocationRelation, SourceIDs: []string{"sql-rdd-2"}},
			{ID: "unrelated.fan", FromID: "fan.owner", ToID: "fan.target", Value: 7, Unit: "kWh"},
		},
	}
	topology := epath101AuditTopology(paths)
	topology.byZoneService = byZone
	return plan, nodes, topology, evidence
}

func energyPathPoolPumpAssert(t *testing.T, plan energyPathZoneAuxiliaryAllocationPlan, allocated, unassigned float64, shares map[string]float64) {
	t.Helper()
	if len(plan.Records) != 1 {
		t.Fatalf("pump record count=%d", len(plan.Records))
	}
	record := plan.Records[0]
	if record.ExpectedValue != 120 || record.DirectValue != 0 || record.AllocatedValue != allocated || record.UnassignedValue != unassigned || record.OvermappedValue != 0 || record.AllocatedValue+record.UnassignedValue != 120 {
		t.Fatalf("source-local pump ledger: %+v", record)
	}
	got := map[string]float64{}
	unrelated := 0
	for _, edge := range plan.Edges {
		if edge.ID == "unrelated.fan" {
			unrelated++
			if edge.Value != 7 {
				t.Fatal("unrelated fan changed")
			}
			continue
		}
		if edge.ID == "stale.broad.pump.allocation" || edge.ServiceKind != "cooling" || !edge.serviceBoundaryExactConsumers || !reflect.DeepEqual(edge.serviceBoundaryConsumerSourceIDs, []string{"sql-rdd-72"}) || !stringSliceContains(edge.SourceIDs, "sql-rdd-2") || !stringSliceContains(edge.SourceIDs, "sql-rdd-72") || stringSliceContains(edge.SourceIDs, "sql-rdd-71") || edge.ZoneName == "PLENUM-1" {
			t.Fatalf("wrong native owner/consumer boundary on pump edge: %+v", edge)
		}
		got[edge.ZoneName] += edge.Value
	}
	if unrelated != 1 || !reflect.DeepEqual(got, shares) {
		t.Fatalf("exact source-local shares=%v want%v unrelated=%d", got, shares, unrelated)
	}
}

func TestEnergyPathPoolPumpNativeCWBudgetDoesNotAllocateSharedHW(t *testing.T) {
	plan, nodes, topology, evidence := energyPathPoolPumpHand(t)
	before, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	got := reserveEnergyPathPoolPumpAllocation(plan, nodes, topology, evidence, "M1", "monthly", true)
	energyPathPoolPumpAssert(t, got, 20, 100, map[string]float64{"SPACE1-1": 1, "SPACE2-1": 2, "SPACE3-1": 3, "SPACE4-1": 4, "SPACE5-1": 10})
	if len(got.PoolPumpSourceAllocations) != 5 {
		t.Fatalf("native CW allocation source traces=%d want5", len(got.PoolPumpSourceAllocations))
	}
	for _, row := range got.PoolPumpSourceAllocations {
		if row.SourceID != "sql-rdd-72" || row.ServiceKind != "cooling" || len(row.LoadSourceIDs) != 1 || len(row.RelatedPathIDs) != 1 {
			t.Fatalf("source-local trace borrowed a sibling consumer: %+v", row)
		}
	}
	after, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("reservation mutated caller plan slices")
	}
}

func TestEnergyPathPoolPumpMissingOwnerEvidenceRejectsShrunkenDenominator(t *testing.T) {
	for _, scenario := range []string{"missing path", "missing positive node", "missing selected load", "NULL selected load", "wrong load unit", "no OA demand proof", "unresolved original pool", "conflicting path owner"} {
		t.Run(scenario, func(t *testing.T) {
			plan, nodes, topology, evidence := energyPathPoolPumpHand(t)
			switch scenario {
			case "missing path":
				for i, path := range topology.auxiliaryPaths {
					if path.ID == "cooling.space5-1" {
						topology.auxiliaryPaths = append(topology.auxiliaryPaths[:i], topology.auxiliaryPaths[i+1:]...)
						break
					}
				}
			case "conflicting path owner":
				topology.auxiliaryPaths = append(topology.auxiliaryPaths, energyPathAuxiliaryServicePath{ID: "cooling.space5-1", ZoneName: "PLENUM-1", ServiceKind: "cooling"})
			case "missing positive node":
				for i, node := range nodes {
					if node.ZoneName == "SPACE5-1" && node.ServiceKind == "cooling" {
						nodes = append(nodes[:i], nodes[i+1:]...)
						break
					}
				}
			case "missing selected load", "NULL selected load", "wrong load unit":
				for i := range evidence.LoadSeries {
					item := &evidence.LoadSeries[i]
					if item.ZoneName != "SPACE5-1" || item.ServiceKind != "cooling" {
						continue
					}
					if scenario == "missing selected load" {
						evidence.LoadSeries = append(evidence.LoadSeries[:i], evidence.LoadSeries[i+1:]...)
					} else if scenario == "NULL selected load" {
						delete(item.Monthly, 1)
					} else {
						item.Unit = "J"
					}
					break
				}
			case "no OA demand proof":
				for i := range evidence.Inventory.Loops {
					if evidence.Inventory.Loops[i].Component.ObjectName == "Chilled Water Loop" {
						evidence.Inventory.Loops[i].Demands[1].AirRouteComplete = false
					}
				}
			case "unresolved original pool":
				evidence.Inventory.UnresolvedPoolIndices = []int{301}
			}
			got := reserveEnergyPathPoolPumpAllocation(plan, nodes, topology, evidence, "M1", "monthly", true)
			energyPathPoolPumpAssert(t, got, 0, 120, map[string]float64{})
			if len(got.PoolPumpSourceAllocations) != 0 {
				t.Fatalf("unproved denominator created source allocation: %+v", got.PoolPumpSourceAllocations)
			}
		})
	}
}

func TestEnergyPathPoolPumpBroadOnlyZeroAndOvermappingCannotBorrowBudget(t *testing.T) {
	for _, scenario := range []string{"broad only", "missing CW Energy", "duplicate native source", "native sum exceeds meter", "native source overlaps meter", "CW native zero", "heating load zero is not permission", "HW missing leaves CW independent"} {
		t.Run(scenario, func(t *testing.T) {
			plan, nodes, topology, evidence := energyPathPoolPumpHand(t)
			allocated, unassigned := 0.0, 120.0
			shares := map[string]float64{}
			if scenario == "heating load zero is not permission" || scenario == "HW missing leaves CW independent" {
				allocated, unassigned = 20, 100
				shares = map[string]float64{"SPACE1-1": 1, "SPACE2-1": 2, "SPACE3-1": 3, "SPACE4-1": 4, "SPACE5-1": 10}
			}
			switch scenario {
			case "broad only":
				evidence.Observations = nil
			case "missing CW Energy", "CW native zero", "native sum exceeds meter", "HW missing leaves CW independent":
				for i := range evidence.Observations {
					observation := &evidence.Observations[i]
					if scenario == "HW missing leaves CW independent" {
						if observation.Component.ObjectName == "HW Circ Pump" {
							observation.Series.Monthly = nil
						}
						continue
					}
					if observation.Component.ObjectName != "CW Circ Pump" {
						continue
					}
					if scenario == "missing CW Energy" {
						observation.Series.Monthly = nil
					} else if scenario == "CW native zero" {
						observation.Series.Monthly[1] = 0
					} else {
						observation.Series.Monthly[1] = 21
					}
				}
			case "duplicate native source":
				evidence.Observations = append(evidence.Observations, evidence.Observations[1])
			case "native source overlaps meter":
				evidence.Observations[1].Series.SourceIDs, evidence.Observations[1].Series.MonthlySourceIDs = []string{"sql-rdd-2"}, []string{"sql-rdd-2"}
			case "heating load zero is not permission":
				for i := range nodes {
					if nodes[i].ServiceKind == "heating" {
						nodes[i].Value = 0
					}
				}
				for i := range evidence.LoadSeries {
					if evidence.LoadSeries[i].ServiceKind == "heating" {
						evidence.LoadSeries[i].Monthly[1] = 0
					}
				}
			}
			got := reserveEnergyPathPoolPumpAllocation(plan, nodes, topology, evidence, "M1", "monthly", true)
			energyPathPoolPumpAssert(t, got, allocated, unassigned, shares)
			if scenario == "CW native zero" {
				if len(got.PoolPumpSourceAllocations) != 5 {
					t.Fatal("measured native zero should retain five exact zero allocation traces")
				}
				for _, row := range got.PoolPumpSourceAllocations {
					if row.Value != 0 || row.SourceID != "sql-rdd-72" {
						t.Fatalf("native zero trace fabricated value: %+v", row)
					}
				}
			}
		})
	}
}

func TestEnergyPathPoolPumpKnownZeroOwnerDoesNotInvalidateDenominator(t *testing.T) {
	plan, nodes, topology, evidence := energyPathPoolPumpHand(t)
	for i := range nodes {
		if nodes[i].ZoneName == "SPACE5-1" && nodes[i].ServiceKind == "cooling" {
			nodes[i].Value = 0
		}
	}
	for i := range evidence.LoadSeries {
		if evidence.LoadSeries[i].ZoneName == "SPACE5-1" && evidence.LoadSeries[i].ServiceKind == "cooling" {
			evidence.LoadSeries[i].Monthly[1] = 0
		}
	}
	got := reserveEnergyPathPoolPumpAllocation(plan, nodes, topology, evidence, "M1", "monthly", true)
	energyPathPoolPumpAssert(t, got, 20, 100, map[string]float64{"SPACE1-1": 2, "SPACE2-1": 4, "SPACE3-1": 6, "SPACE4-1": 8})
	if math.IsNaN(got.Records[0].AllocatedValue) {
		t.Fatal("known-zero load produced invalid ratio")
	}
}

func TestEnergyPathPoolPumpPositiveHumidityDetailDoesNotReplaceKnownZeroPrimary(t *testing.T) {
	plan, nodes, topology, evidence := energyPathPoolPumpHand(t)
	for i := range nodes {
		if nodes[i].ZoneName == "SPACE5-1" && nodes[i].ServiceKind == "cooling" {
			nodes[i].Value = 0
		}
	}
	for i := range evidence.LoadSeries {
		item := &evidence.LoadSeries[i]
		if item.ZoneName == "SPACE5-1" && item.ServiceKind == "cooling" {
			item.Monthly[1], item.RawMonthly[1], item.Total, item.RawTotal = 0, 0, 0, 0
		}
	}
	// This is the actual canonical humidity-detail name, not a made-up alias.
	// Its magnitude must neither enter the primary denominator nor make the
	// known-zero primary owner look like a missing positive target.
	humidity := energyExplanationSeries{Stage: "load", Level: "load", Unit: "kWh", ZoneName: "SPACE5-1", ServiceKind: "dehumidification",
		SourceIDs: []string{"sql-rdd-251"}, MonthlySourceIDs: []string{"sql-rdd-251"}, Monthly: map[int]float64{1: 999}, RawMonthly: map[int]float64{1: 999}}
	evidence.LoadSeries = append(evidence.LoadSeries, humidity)
	nodes = append(nodes, epath101AuditLoad("SPACE5-1", "dehumidification", 999, "sql-rdd-251", []string{"cooling.space5-1"}))
	var paths []string
	for _, path := range topology.auxiliaryPaths {
		if path.ServiceKind == "cooling" {
			paths = append(paths, path.ID)
		}
	}
	targets := energyPathHVACConsumptionTargets(nodes, topology, "cooling", paths)
	if !energyPathPoolLoadRosterComplete(evidence.LoadSeries, topology, paths, "cooling", "M1", "monthly", true) ||
		!energyPathPoolPositiveLoadTargetsComplete(targets, evidence.LoadSeries, topology, paths, "cooling", "M1", "monthly", true) || len(targets) != 4 {
		t.Fatal("optional positive humidity detail invalidated the independently complete primary roster")
	}
	got := reserveEnergyPathPoolPumpAllocation(plan, nodes, topology, evidence, "M1", "monthly", true)
	energyPathPoolPumpAssert(t, got, 20, 100, map[string]float64{"SPACE1-1": 2, "SPACE2-1": 4, "SPACE3-1": 6, "SPACE4-1": 8})
	for _, edge := range got.Edges {
		if stringSliceContains(edge.SourceIDs, "sql-rdd-251") {
			t.Fatalf("humidity detail became a primary allocation weight: %+v", edge)
		}
	}
}

func TestEnergyPathPoolPumpHumidityCannotEstablishAbsentOrUnknownPrimary(t *testing.T) {
	for _, scenario := range []string{"primary absent", "primary unknown"} {
		t.Run(scenario, func(t *testing.T) {
			plan, nodes, topology, evidence := energyPathPoolPumpHand(t)
			for i := range evidence.LoadSeries {
				item := &evidence.LoadSeries[i]
				if item.ZoneName != "SPACE5-1" || item.ServiceKind != "cooling" {
					continue
				}
				if scenario == "primary absent" {
					evidence.LoadSeries = append(evidence.LoadSeries[:i], evidence.LoadSeries[i+1:]...)
				} else {
					delete(item.Monthly, 1)
					delete(item.RawMonthly, 1)
				}
				break
			}
			evidence.LoadSeries = append(evidence.LoadSeries, energyExplanationSeries{Stage: "load", Level: "load", Unit: "kWh", ZoneName: "SPACE5-1", ServiceKind: "dehumidification",
				SourceIDs: []string{"sql-rdd-251"}, MonthlySourceIDs: []string{"sql-rdd-251"}, Monthly: map[int]float64{1: 10}, RawMonthly: map[int]float64{1: 10}})
			nodes = append(nodes, epath101AuditLoad("SPACE5-1", "dehumidification", 10, "sql-rdd-251", []string{"cooling.space5-1"}))
			var paths []string
			for _, path := range topology.auxiliaryPaths {
				if path.ServiceKind == "cooling" {
					paths = append(paths, path.ID)
				}
			}
			if energyPathPoolLoadRosterComplete(evidence.LoadSeries, topology, paths, "cooling", "M1", "monthly", true) {
				t.Fatal("humidity detail substituted for a missing/unknown selected primary")
			}
			got := reserveEnergyPathPoolPumpAllocation(plan, nodes, topology, evidence, "M1", "monthly", true)
			energyPathPoolPumpAssert(t, got, 0, 120, map[string]float64{})
			if len(got.PoolPumpSourceAllocations) != 0 {
				t.Fatal("unproved primary produced native source-allocation traces")
			}
		})
	}
}

func TestEnergyPathPoolPumpRejectsStalePrimaryNodeValueOrSources(t *testing.T) {
	for _, scenario := range []string{"different positive value", "different primary source", "sourceless primary"} {
		t.Run(scenario, func(t *testing.T) {
			plan, nodes, topology, evidence := energyPathPoolPumpHand(t)
			for i := range nodes {
				node := &nodes[i]
				if node.ZoneName != "SPACE5-1" || node.ServiceKind != "cooling" {
					continue
				}
				switch scenario {
				case "different positive value":
					node.Value = 100
				case "different primary source":
					node.SourceIDs = []string{"sql-rdd-999"}
				case "sourceless primary":
					node.SourceIDs = nil
				}
				break
			}
			var paths []string
			for _, path := range topology.auxiliaryPaths {
				if path.ServiceKind == "cooling" {
					paths = append(paths, path.ID)
				}
			}
			if !energyPathPoolLoadRosterComplete(evidence.LoadSeries, topology, paths, "cooling", "M1", "monthly", true) {
				t.Fatal("control must retain complete, known, original selected primary evidence")
			}
			targets := energyPathHVACConsumptionTargets(nodes, topology, "cooling", paths)
			if energyPathPoolPositiveLoadTargetsComplete(targets, evidence.LoadSeries, topology, paths, "cooling", "M1", "monthly", true) {
				t.Fatal("stale node escaped value/source binding to the selected primary")
			}
			got := reserveEnergyPathPoolPumpAllocation(plan, nodes, topology, evidence, "M1", "monthly", true)
			energyPathPoolPumpAssert(t, got, 0, 120, map[string]float64{})
			if len(got.PoolPumpSourceAllocations) != 0 {
				t.Fatal("stale primary data created an allocation trace")
			}
		})
	}
}
