package simulation

import (
	"reflect"
	"testing"
)

func vrfReservationFixture() (energyPathZoneHVACAllocationPlan, []EnergyExplanationNode, energyPathVRFAllocationPlan) {
	nodes := []EnergyExplanationNode{{ID: "cooling", Level: "energy", EndUse: "cooling", Carrier: "electricity"},
		{ID: "gas", Level: "energy", EndUse: "heating", Carrier: "natural_gas"}}
	plan := energyPathZoneHVACAllocationPlan{CentralEndUseNodeIDs: map[string]bool{"cooling": true, "gas": true},
		Records: []energyPathZoneHVACAllocationRecord{{Period: "M1", ServiceKind: "cooling", Carrier: "electricity", ExpectedValue: 1000, DirectValue: 100, AllocatedValue: 900, UsedServicePath: true},
			{Period: "M1", ServiceKind: "heating", Carrier: "natural_gas", ExpectedValue: 50, AllocatedValue: 50}},
		Edges: []EnergyExplanationEdge{{ID: "vrf", FromID: "cooling", ToID: "load-vrf", ZoneName: "VRF", Value: 300, Basis: "service_path_allocation"},
			{ID: "other-one", FromID: "cooling", ToID: "load-one", ZoneName: "Other one", Value: 400, Basis: "service_path_allocation", SourceIDs: []string{"broad", "load-one"}},
			{ID: "other-two", FromID: "cooling", ToID: "load-two", ZoneName: "Other two", Value: 200, Basis: "service_path_allocation"},
			{ID: "gas-edge", FromID: "gas", ToID: "load-gas", ZoneName: "Elsewhere", Value: 50, Basis: "service_path_allocation"}}}
	native := energyPathVRFAllocationPlan{Services: []energyPathVRFServiceAllocation{{PeriodID: "M1", ServiceKind: "cooling", Carrier: "electricity",
		DirectValue: 50, SharedValue: 150, AllocatedValue: 120, DirectKnown: true, SharedKnown: true, ConsumptionSourceIDs: []string{"local", "outdoor"}}},
		Zones: []energyPathVRFZoneAllocation{{PeriodID: "M1", ServiceKind: "cooling", Carrier: "electricity", ZoneName: "VRF"}}}
	return plan, nodes, native
}

func TestEnergyPathVRFReservationPreservesNonVRFAndSharedUnassigned(t *testing.T) {
	plan, nodes, native := vrfReservationFixture()
	got := reserveEnergyPathVRFAllocation(plan, nodes, native, "M1")
	if len(got.Edges) != 3 || len(got.Records) != 2 {
		t.Fatalf("unexpected reserved roster: %#v", got)
	}
	values := map[string]float64{}
	for _, edge := range got.Edges {
		values[edge.ID] = edge.Value
		for _, source := range edge.SourceIDs {
			if source == "local" || source == "outdoor" {
				t.Fatal("reservation context became another system's consumption ribbon source")
			}
		}
	}
	if values["other-one"] != 466.667 || values["other-two"] != 233.333 || values["gas-edge"] != 50 {
		t.Fatalf("non-VRF residual shares = %#v, want the remaining700 with exact original eligibility", values)
	}
	row := got.Records[0]
	if row.DirectValue != 150 || row.AllocatedValue != 820 || row.UnassignedValue != 30 || row.OvermappedValue != 0 {
		t.Fatalf("shared owned-unassigned consumption leaked: %#v", row)
	}
	if !reflect.DeepEqual(plan.Records[1], got.Records[1]) || plan.Records[0].DirectValue != 100 || plan.Edges[1].Value != 400 {
		t.Fatal("reservation mutated its input or an unrelated carrier")
	}
}

func TestEnergyPathVRFReservationDoesNotGuessMissingPoolsOrRecipients(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*energyPathZoneHVACAllocationPlan, *energyPathVRFAllocationPlan)
	}{
		{"unknown outdoor", func(_ *energyPathZoneHVACAllocationPlan, v *energyPathVRFAllocationPlan) {
			v.Services[0].SharedKnown = false
		}},
		{"unknown terminal", func(_ *energyPathZoneHVACAllocationPlan, v *energyPathVRFAllocationPlan) {
			v.Services[0].DirectKnown = false
		}},
		{"no other eligible edge", func(p *energyPathZoneHVACAllocationPlan, _ *energyPathVRFAllocationPlan) {
			p.Edges = []EnergyExplanationEdge{p.Edges[0], p.Edges[3]}
		}},
		{"other edges known zero", func(p *energyPathZoneHVACAllocationPlan, _ *energyPathVRFAllocationPlan) {
			p.Edges[1].Value, p.Edges[2].Value = 0, 0
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, nodes, native := vrfReservationFixture()
			test.mutate(&plan, &native)
			got := reserveEnergyPathVRFAllocation(plan, nodes, native, "M1")
			if len(got.Edges) != 1 || got.Edges[0].ID != "gas-edge" {
				t.Fatalf("unknown VRF meter residue was assigned to another recipient: %#v", got.Edges)
			}
			if got.Records[0].DirectValue != 150 || got.Records[0].AllocatedValue != 120 || got.Records[0].UnassignedValue != 730 {
				t.Fatalf("observed direct/allocation values lost: %#v", got.Records[0])
			}
		})
	}
}

func TestEnergyPathVRFReservationUnaffectedPeriodsAndModelsStayUnchanged(t *testing.T) {
	plan, nodes, native := vrfReservationFixture()
	for _, got := range []energyPathZoneHVACAllocationPlan{
		reserveEnergyPathVRFAllocation(plan, nodes, native, "M2"),
		reserveEnergyPathVRFAllocation(plan, nodes, energyPathVRFAllocationPlan{}, "M1"),
	} {
		if !reflect.DeepEqual(plan, got) {
			t.Fatal("a model or period without native VRF observations changed")
		}
	}
}

func TestEnergyPathVRFReservationDoesNotInferAnotherCarrierFromElectricPath(t *testing.T) {
	plan, nodes, native := vrfReservationFixture()
	nodes = append(nodes, EnergyExplanationNode{ID: "other-carrier", Level: "energy", EndUse: "cooling", Carrier: "district_cooling"})
	plan.Edges = append(plan.Edges,
		EnergyExplanationEdge{ID: "other-carrier-vrf", FromID: "other-carrier", ZoneName: "VRF", Value: 40, Basis: "service_path_allocation"},
		EnergyExplanationEdge{ID: "other-carrier-elsewhere", FromID: "other-carrier", ZoneName: "Elsewhere", Value: 60, Basis: "service_path_allocation"})
	plan.Records = append(plan.Records, energyPathZoneHVACAllocationRecord{Period: "M1", ServiceKind: "cooling", Carrier: "district_cooling", ExpectedValue: 100, AllocatedValue: 100})
	got := reserveEnergyPathVRFAllocation(plan, nodes, native, "M1")
	for _, edge := range got.Edges {
		if edge.ID == "other-carrier-vrf" || edge.ID == "other-carrier-elsewhere" && edge.Value != 60 {
			t.Fatalf("unproved VRF sibling carrier ownership or recipient inflation: %#v", edge)
		}
	}
	if row := got.Records[2]; row.DirectValue != 0 || row.AllocatedValue != 60 || row.UnassignedValue != 40 {
		t.Fatalf("unknown carrier owner did not stay unassigned: %#v", row)
	}
	// This guard is about a broad carrier fallback. A separate, explicitly
	// identified non-VRF loop must retain its original service-path allocation.
	nodes[len(nodes)-1].LoopName = "Explicit chilled water loop"
	got = reserveEnergyPathVRFAllocation(plan, nodes, native, "M1")
	found := false
	for _, edge := range got.Edges {
		if edge.ID == "other-carrier-vrf" {
			found = edge.Value == 40
		}
	}
	if !found || got.Records[2].AllocatedValue != 100 || got.Records[2].UnassignedValue != 0 {
		t.Fatal("an explicitly identified separate loop was suppressed")
	}
}

func TestEnergyPathVRFReservationPublishesCompletedDisplayBudgets(t *testing.T) {
	plan, nodes, native := vrfReservationFixture()
	plan.Records[0].ExpectedValue, plan.Records[0].DirectValue = 124, 0
	native.Services[0].DirectValue = 14.0012
	native.Services[0].SharedValue, native.Services[0].AllocatedValue = 109.9988, 109.9988
	display := native
	display.Services = append([]energyPathVRFServiceAllocation(nil), native.Services...)
	display.Services[0].DirectValue = 14.002
	display.Services[0].SharedValue, display.Services[0].AllocatedValue = 109.998, 109.998
	got := reserveEnergyPathVRFAllocation(plan, nodes, native, "M1", display)
	row := got.Records[0]
	if row.DirectValue != 14.002 || row.AllocatedValue != 109.998 || row.UnassignedValue != 0 || row.OvermappedValue != 0 {
		t.Fatalf("published ledger disagrees with completed source display budgets: %#v", row)
	}
	if native.Services[0].DirectValue != 14.0012 || native.Services[0].SharedValue != 109.9988 {
		t.Fatal("display ledger changed original observed reservation quantities")
	}
	for _, edge := range got.Edges {
		if edge.FromID == "cooling" {
			t.Fatal("a purely internal display split invented another recipient's consumption")
		}
	}
}
