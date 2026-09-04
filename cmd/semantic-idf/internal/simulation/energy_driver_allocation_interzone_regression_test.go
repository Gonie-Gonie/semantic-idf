package simulation

import (
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// A raw pair imbalance and an allocated zone contribution are different
// quantities. Building projection must aggregate the completed zone-month
// contributions without reintroducing the raw pair remainder as extra width.
func TestEPATH080BuildingInterzoneProjectionClosesAfterZoneMonthAllocation(t *testing.T) {
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
		reviewEnergyLoadSeries("Office", "cooling", 100, "office-load"),
		reviewEnergyLoadSeries("Lab", "heating", 50, "lab-load"),
		energyDriverPairwiseFixtureSeries("Office from Lab", "positive", 60, "office-pair-gain"),
		energyDriverPairwiseFixtureSeries("Lab from Office", "negative", 50, "lab-pair-loss"),
		reviewEnergyHeatSeries(t, "Office", "Zone People Convective Heating Energy", 1140, "office-people"),
	}
	legacy := buildEnergyExplanationResultWithDriverContext(
		series,
		reviewEnergySources(series),
		&PurposeRunPlan{},
		newEnergyDriverBuildContext(report),
	)
	result := UpgradeEnergyExplanationV1(legacy)

	var month *EnergyPeriod
	for index := range result.Periods {
		if result.Periods[index].ID == "M1" {
			month = &result.Periods[index]
			break
		}
	}
	if month == nil {
		t.Fatalf("missing M1 Building period: %#v", result.Periods)
	}
	load := energyPathV2NodeByID(month.Nodes, "load.cooling.building")
	if load == nil || load.Value != 100 {
		t.Fatalf("M1 Building cooling load = %#v, want 100", load)
	}
	pair := energyPathV2NodeByID(month.Nodes, "driver.interzone.transfer.cooling.building")
	if pair == nil || pair.RawValue != 10 || pair.EffectiveValue != 10 || pair.AllocatedValue != 5 || pair.Value != 5 || !pair.AllocationApplied {
		t.Fatalf("M1 Building pair node = %#v, want raw/effective imbalance 10/10 and allocated zone-month contribution 5", pair)
	}
	people := energyPathV2NodeByID(month.Nodes, "driver.internal.people.cooling.building")
	if people == nil || people.RawValue != 1140 || people.AllocatedValue != 95 || people.Value != 95 {
		t.Fatalf("M1 Building People node = %#v, want raw pressure 1140 and allocated contribution 95", people)
	}
	incoming := 0.0
	for _, link := range month.Links {
		if link.Relation == "driver_to_load" && link.ToID == load.ID {
			incoming = roundedEnergyNumber(incoming + link.ToValue)
			if link.FromID == pair.ID && (link.FromValue != 5 || link.ToValue != 5) {
				t.Errorf("M1 Building pair ribbon = %#v, want allocated width 5", link)
			}
		}
	}
	if incoming != load.Value {
		t.Fatalf("M1 Building cooling drivers = %g, want exact zone-month contribution closure %g; nodes=%#v links=%#v", incoming, load.Value, month.Nodes, month.Links)
	}
	if storage := energyPathV2NodeByID(month.Nodes, "driver.balance.storage_other.cooling.building"); storage != nil && storage.Value > 0 {
		t.Fatalf("M1 cooling added compensating storage after pair5 + People95 already closed the load: %#v", storage)
	}
	heatingLoad := energyPathV2NodeByID(month.Nodes, "load.heating.building")
	if heatingLoad == nil || heatingLoad.Value != 50 {
		t.Fatalf("M1 Building heating load = %#v, want 50", heatingLoad)
	}
	heatingStorage := energyPathV2NodeByID(month.Nodes, "driver.balance.storage_other.heating.building")
	if heatingStorage == nil || heatingStorage.Value != 50 || heatingStorage.AllocatedValue != 50 || !heatingStorage.AllocationApplied {
		t.Fatalf("M1 opposite-service pair contribution = %#v, want Other/storage allocated width 50", heatingStorage)
	}
	heatingIncoming := 0.0
	for _, link := range month.Links {
		if link.Relation == "driver_to_load" && link.ToID == heatingLoad.ID {
			heatingIncoming = roundedEnergyNumber(heatingIncoming + link.ToValue)
			if link.FromID == heatingStorage.ID && (link.FromValue != 50 || link.ToValue != 50) {
				t.Errorf("M1 heating Other/storage ribbon = %#v, want allocated width 50", link)
			}
		}
	}
	if heatingIncoming != heatingLoad.Value {
		t.Fatalf("M1 Building heating drivers = %g, want exact zone-month contribution closure %g", heatingIncoming, heatingLoad.Value)
	}

	pairSource := reviewEnergySourceWithFormula(result.Sources, "signed pairwise interzone receiving-zone contributions summed at Building scope")
	if pairSource == nil || pairSource.RawValue != 10 || pairSource.EffectiveValue != 10 || pairSource.AllocatedValue != 5 || !pairSource.AllocationApplied ||
		!stringSliceContains(pairSource.InputSourceIDs, "office-pair-gain") ||
		!stringSliceContains(pairSource.InputSourceIDs, "lab-pair-loss") {
		t.Fatalf("raw +10 pair imbalance and allocated contribution 5 must remain separately inspectable as source/balance provenance: %#v", pairSource)
	}
}
