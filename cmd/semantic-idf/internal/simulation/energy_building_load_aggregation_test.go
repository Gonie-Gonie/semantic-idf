package simulation

import "testing"

func TestEPATH071StoredV1HumidityLoadsRemainNonAdditiveDetail(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{ID: "load.cooling.office", Level: "load", Kind: "load.zone_cooling", Label: "Cooling", Value: 10, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"cooling"}},
			{ID: "load.heating.office", Level: "load", Kind: "load.zone_heating", Label: "Heating", Value: 7, Unit: "kWh", ZoneName: "Office", ServiceKind: "heating", SourceIDs: []string{"heating"}},
			{ID: "load.dehumidification.office", Level: "load", Kind: "load.zone_dehumidification", Label: "Dehumidification", Value: 3, Unit: "kWh", ZoneName: "Office", ServiceKind: "dehumidification", SourceIDs: []string{"dehumidification"}},
			{ID: "load.humidification.office", Level: "load", Kind: "load.zone_humidification", Label: "Humidification", Value: 2, Unit: "kWh", ZoneName: "Office", ServiceKind: "humidification", SourceIDs: []string{"humidification"}},
		},
		Sources: []EnergyDataSource{
			{ID: "cooling", Name: "Zone Air System Sensible Cooling Energy"},
			{ID: "heating", Name: "Zone Air System Sensible Heating Energy"},
			{ID: "dehumidification", Name: "Zone Ideal Loads Supply Air Latent Cooling Energy"},
			{ID: "humidification", Name: "Zone Ideal Loads Supply Air Latent Heating Energy"},
		},
	}

	result := UpgradeEnergyExplanationV1(legacy)
	cooling := epath071NodeByID(result.Nodes, "load.cooling.building")
	heating := epath071NodeByID(result.Nodes, "load.heating.building")
	if cooling == nil || cooling.Value != 10 || !epath071Contains(cooling.SourceIDs, "dehumidification") {
		t.Fatalf("stored-v1 cooling projection = %#v, want value 10 with non-additive dehumidification provenance", cooling)
	}
	if heating == nil || heating.Value != 7 || !epath071Contains(heating.SourceIDs, "humidification") {
		t.Fatalf("stored-v1 heating projection = %#v, want value 7 with non-additive humidification provenance", heating)
	}
	for _, id := range []string{"load.dehumidification.building", "load.humidification.building"} {
		if node := epath071NodeByID(result.Nodes, id); node != nil {
			t.Errorf("stored-v1 detail became a primary v2 load: %#v", node)
		}
	}
	for id, component := range map[string]string{
		"dehumidification": "load.dehumidification",
		"humidification":   "load.humidification",
	} {
		source := epath071SourceByID(result.Sources, id)
		if source == nil || source.DriverRole != energyDriverSourceRoleContext || source.InspectorSection != energyDriverInspectorSectionBreakdown || source.DriverComponent != component {
			t.Errorf("stored-v1 humidity detail %q = %#v", id, source)
		}
	}
}
