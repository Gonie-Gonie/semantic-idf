package simulation

import (
	"reflect"
	"testing"
)

func vrfHVACConsumptionReservationNative() energyPathVRFAllocationPlan {
	return energyPathVRFAllocationPlan{
		Services: []energyPathVRFServiceAllocation{{SystemID: "vrf-outdoor", PeriodID: "M1", ServiceKind: "heating", Carrier: "electricity",
			DirectValue: 5, SharedValue: 15, AllocatedValue: 15, DirectKnown: true, SharedKnown: true, AllocatedKnown: true,
			ConsumptionSourceIDs: []string{"sql.vrf.local", "sql.vrf.outdoor"}, LoadSourceIDs: []string{"sql.load.SPACE2-1", "sql.load.SPACE4-1"}}},
		Zones: []energyPathVRFZoneAllocation{
			{SystemID: "vrf-outdoor", PeriodID: "M1", ServiceKind: "heating", Carrier: "electricity", ZoneName: "SPACE2-1", DirectValue: 2, AllocatedValue: 7.5},
			{SystemID: "vrf-outdoor", PeriodID: "M1", ServiceKind: "heating", Carrier: "electricity", ZoneName: "SPACE4-1", DirectValue: 3, AllocatedValue: 7.5},
		},
	}
}

func TestEnergyPathVRFReservationPreservesIndependentHVACSourceBudgets(t *testing.T) {
	for _, scenario := range []string{"complete", "kind-only end use", "incomplete VRF", "unallocated VRF", "unknown boiler", "unserved boiler", "invalid boiler pool", "overmapped", "foreign carrier"} {
		t.Run(scenario, func(t *testing.T) {
			nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
			nodes[0].Value, nodes[0].EffectiveValue = 100, 100
			native := vrfHVACConsumptionReservationNative()
			wantDirect, wantAllocated, wantUnassigned, wantOvermapped := 45.0, 25.0, 30.0, 0.0
			wantEdges := 5
			switch scenario {
			case "kind-only end use":
				nodes[0].EndUse = ""
			case "incomplete VRF":
				// A missing sibling does not erase another independently
				// measured source's budget or authorize generic redistribution.
				native.Services[0].SharedKnown = false
				native.Services[0].AllocatedValue = 0
				wantAllocated, wantUnassigned = 10, 45
			case "unallocated VRF":
				native.Services[0].AllocatedValue = 0
				wantAllocated, wantUnassigned = 10, 45
			case "unknown boiler":
				delete(pool.Members[2].Series.Monthly, 1)
				wantAllocated, wantUnassigned, wantEdges = 15, 40, 0
			case "unserved boiler":
				pool.Members[2].RelatedPathIDs = nil
				wantAllocated, wantUnassigned, wantEdges = 15, 40, 0
			case "invalid boiler pool":
				pool.Valid = false
				wantAllocated, wantUnassigned, wantEdges = 15, 40, 0
			case "overmapped":
				nodes[0].Value, nodes[0].EffectiveValue = 50, 50
				wantUnassigned, wantOvermapped = 0, 20
			case "foreign carrier":
				// The boiler has its own typed service paths; a native VRF
				// carrier cannot invalidate those proved recipients.
				native.Services[0].Carrier = "natural_gas"
				for i := range native.Zones {
					native.Zones[i].Carrier = "natural_gas"
				}
				wantDirect, wantAllocated, wantUnassigned = 40, 10, 50
			}
			generic := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology)
			reserved := reserveEnergyPathHVACConsumptionPools(generic, nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
			before := epathHVACConsumptionPoolPlanSnapshot(reserved)
			got := reserveEnergyPathVRFAllocation(reserved, nodes, native, "M1")
			if len(got.Records) != 1 || len(got.Edges) != wantEdges {
				t.Fatalf("source-budget reservation changed edge/ledger roster: %#v", got)
			}
			row := got.Records[0]
			if row.DirectValue != wantDirect || row.AllocatedValue != wantAllocated || row.UnassignedValue != wantUnassigned || row.OvermappedValue != wantOvermapped {
				t.Fatalf("ledger direct/allocated/unassigned/overmapped=%g/%g/%g/%g, want %g/%g/%g/%g", row.DirectValue, row.AllocatedValue, row.UnassignedValue, row.OvermappedValue, wantDirect, wantAllocated, wantUnassigned, wantOvermapped)
			}
			if !reflect.DeepEqual(got.Edges, reserved.Edges) {
				t.Fatalf("VRF changed independently observed boiler amounts, recipients, formula, or source/path IDs: got=%#v want=%#v", got.Edges, reserved.Edges)
			}
			if !reflect.DeepEqual(got.ConsumptionSourceAllocations, reserved.ConsumptionSourceAllocations) || !reflect.DeepEqual(got.ConsumptionPoolGroups, reserved.ConsumptionPoolGroups) {
				t.Fatal("VRF changed independently measured source allocation traces or reopened a source-local boundary")
			}
			if !reflect.DeepEqual(before, epathHVACConsumptionPoolPlanSnapshot(reserved)) {
				t.Fatal("VRF reservation mutated the input source budgets")
			}
		})
	}
}

func TestEnergyPathVRFReservationConflictingHVACSourceStaysUnassigned(t *testing.T) {
	for _, amount := range []float64{10, 0} {
		t.Run(map[float64]string{10: "positive source", 0: "zero source has no edge"}[amount], func(t *testing.T) {
			nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
			nodes[0].Value, nodes[0].EffectiveValue = 100, 100
			pool.Members[2].Series.Monthly[1] = amount
			generic := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology)
			reserved := reserveEnergyPathHVACConsumptionPools(generic, nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
			native := vrfHVACConsumptionReservationNative()
			// The two private cohorts cannot both own the same original consumption
			// observation. The merged boiler edge has no safe constituent split here.
			native.Services[0].ConsumptionSourceIDs = append(native.Services[0].ConsumptionSourceIDs, "sql.boiler.ancillary")
			got := reserveEnergyPathVRFAllocation(reserved, nodes, native, "M1")
			if len(got.Edges) != 0 || len(got.Records) != 1 || len(got.ConsumptionSourceAllocations) != 0 {
				t.Fatalf("conflicting source entered both source-budget allocations: %#v", got)
			}
			row := got.Records[0]
			if row.DirectValue != 45 || row.AllocatedValue != 15 || row.UnassignedValue != 40 || row.OvermappedValue != 0 {
				t.Fatalf("conflicting observed source was scaled into an invented budget: %#v", row)
			}
			if !stringSliceContains(row.SourceIDs, "sql.boiler.ancillary") || !stringSliceContains(row.SourceIDs, "sql.vrf.outdoor") {
				t.Fatal("fail-closed accounting erased the conflicting original source evidence")
			}
		})
	}
}

func TestEnergyPathVRFReservationDoesNotReopenSourceLocalRemainder(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(map[bool]string{true: "known budget", false: "unknown budget has no edges"}[known], func(t *testing.T) {
			nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
			nodes[0].Value, nodes[0].EffectiveValue = 100, 100
			if !known {
				delete(pool.Members[2].Series.Monthly, 1)
			}
			generic := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology)
			reserved := reserveEnergyPathHVACConsumptionPools(generic, nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
			// Even if a future adapter retains an old generic sibling, the existing
			// source-local boundary cannot authorize reusing its unidentified residue.
			reserved.Edges = append(reserved.Edges, EnergyExplanationEdge{ID: "stale-generic", FromID: nodes[0].ID, ToID: "another-load", ZoneName: "Elsewhere", Value: 50, Basis: "service_path_allocation"})
			got := reserveEnergyPathVRFAllocation(reserved, nodes, vrfHVACConsumptionReservationNative(), "M1")
			wantEdges, wantAllocated, wantUnassigned := 5, 25.0, 30.0
			if !known {
				wantEdges, wantAllocated, wantUnassigned = 0, 15, 40
			}
			if len(got.Edges) != wantEdges || got.Records[0].AllocatedValue != wantAllocated || got.Records[0].UnassignedValue != wantUnassigned {
				t.Fatalf("source-local unassigned consumption became a generic share: %#v", got)
			}
			for _, edge := range got.Edges {
				if edge.ID == "stale-generic" || edge.RuleID != energyRelationshipRuleAllocatedHVACConsumptionPool || edge.Value != 2 {
					t.Fatalf("original independent 10-kWh boiler pool was redistributed: %#v", edge)
				}
			}
		})
	}
}
