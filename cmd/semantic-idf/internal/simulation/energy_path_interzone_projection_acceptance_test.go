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
	if pair == nil || pair.Value != 10 {
		t.Fatalf("material Building interzone pair node = %#v, want 10 kWh cooling remainder", pair)
	}
	pairLink := energyPathV2LinkByIDs(result.Links, pair.ID, "load.cooling.building")
	if pairLink == nil {
		t.Errorf("material Building interzone pair node is orphaned: node=%#v links=%#v", pair, result.Links)
	} else if pairLink.Relation != "driver_to_load" || pairLink.FromValue != 10 || pairLink.ToValue != 10 {
		t.Errorf("material Building interzone pair link = %#v, want a 10 kWh driver_to_load contribution", pairLink)
	}

	load := energyPathV2NodeByID(result.Nodes, "load.cooling.building")
	if load == nil {
		t.Fatalf("missing Building cooling load: %#v", result.Nodes)
	}
	incoming := 0.0
	for _, link := range result.Links {
		if link.Relation == "driver_to_load" && link.ToID == load.ID {
			incoming += link.ToValue
		}
	}
	if incoming != load.Value {
		t.Errorf("Building cooling closure = %g kWh incoming vs %g kWh load; retained material pair net must participate in the flow", incoming, load.Value)
	}
}
