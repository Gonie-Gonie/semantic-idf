package simulation

import (
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// A material signed remainder from an explicitly sourced interzone pair is a
// Building driver, not merely inspector context. The v1 graph cannot attach an
// unscoped derived pair remainder to either zone-specific load, so the v2
// Building projection must create this link after the zone loads are merged.
// The folded cancelling portion and the retained pair remainder must then close
// each Building load exactly.
func TestEPATH064AcceptanceMaterialPairNetHasBuildingFlowAndClosesLoad(t *testing.T) {
	report := idf.GeometryReport{Topology: idf.ThermalTopologyReport{
		Nodes: []idf.ThermalTopologyNode{
			{ID: "zone.office", ZoneName: "Office"},
			{ID: "zone.lab", ZoneName: "Lab"},
		},
		AirCouplings: []idf.ThermalAirCoupling{
			{ID: "coupling.office", ObjectType: "ZoneMixing", ObjectName: "Office from Lab", FromNodeID: "zone.lab", ToNodeID: "zone.office", Direction: "directed"},
			{ID: "coupling.lab", ObjectType: "ZoneMixing", ObjectName: "Lab from Office", FromNodeID: "zone.office", ToNodeID: "zone.lab", Direction: "directed"},
		},
	}}
	series := []energyExplanationSeries{
		reviewEnergyLoadSeries("Office", "cooling", 65, "office-load"),
		reviewEnergyLoadSeries("Lab", "heating", 50, "lab-load"),
		energyDriverPairwiseFixtureSeries("Office from Lab", "positive", 60, "office-gain"),
		energyDriverPairwiseFixtureSeries("Lab from Office", "negative", 50, "lab-loss"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(
		series,
		reviewEnergySources(series),
		&PurposeRunPlan{},
		newEnergyDriverBuildContext(report),
	)
	result := UpgradeEnergyExplanationV1(legacy)

	pair := energyPathV2NodeByID(result.Nodes, "driver.interzone.transfer.cooling.building")
	if pair == nil || pair.RawValue != 10 || pair.EffectiveValue != 10 || pair.SignedValue != 10 ||
		pair.Value != 10 || pair.AllocatedValue != 10 || !pair.AllocationApplied {
		t.Fatalf("material Building interzone pair node = %#v, want raw imbalance 10 capped to allocated contribution 10", pair)
	}
	pairLink := energyPathV2LinkByIDs(result.Links, pair.ID, "load.cooling.building")
	if pairLink == nil {
		t.Errorf("material Building interzone pair node is orphaned: node=%#v links=%#v", pair, result.Links)
	} else if pairLink.Relation != "driver_to_load" || pairLink.FromValue != 10 || pairLink.ToValue != 10 {
		t.Errorf("material Building interzone pair link = %#v, want a 10 kWh driver_to_load contribution", pairLink)
	}

	coolingStorage := energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other.cooling.building")
	if coolingStorage == nil || coolingStorage.Value != 55 || coolingStorage.AllocatedValue != 55 || !coolingStorage.AllocationApplied {
		t.Fatalf("Building cooling pair remainder = %#v, want 65 allocated pair contribution - 10 retained net = 55 storage", coolingStorage)
	}
	heatingStorage := energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other.heating.building")
	if heatingStorage == nil || heatingStorage.Value != 50 || heatingStorage.AllocatedValue != 50 || !heatingStorage.AllocationApplied {
		t.Fatalf("Building opposite-direction pair contribution = %#v, want 50 storage", heatingStorage)
	}
	for _, want := range []struct {
		service string
		value   float64
	}{
		{service: "cooling", value: 65},
		{service: "heating", value: 50},
	} {
		load := energyPathV2NodeByID(result.Nodes, "load."+want.service+".building")
		if load == nil || load.Value != want.value {
			t.Fatalf("missing Building %s load %g: %#v", want.service, want.value, result.Nodes)
		}
		incoming := 0.0
		for _, link := range result.Links {
			if link.Relation == "driver_to_load" && link.ToID == load.ID {
				incoming = roundedEnergyNumber(incoming + link.ToValue)
			}
		}
		if incoming != load.Value {
			t.Errorf("Building %s closure = %g kWh incoming vs %g kWh load; retained pair plus storage must equal completed zone-month allocation", want.service, incoming, load.Value)
		}
	}
}
