package simulation

import "testing"

func TestEPATH071ReviewPeriodLocalLatentShareAndBuildingClosure(t *testing.T) {
	series := []energyExplanationSeries{
		epath071Load("office-sensible", "Office", "cooling", "sensible", 100, map[int]float64{1: 91, 2: 9}),
		epath071Load("office-latent", "Office", "cooling", "latent", 20, map[int]float64{1: 9, 2: 11}),
		epath071Load("lab-sensible", "Lab", "cooling", "sensible", 80, map[int]float64{1: 40, 2: 40}),
	}
	multipliers := energyEffectiveMultiplierIndex{
		Enabled: true,
		Zones: map[string]energyZoneMultiplierRecord{
			normalizePurposeToken("Office"): {ZoneName: "Office", ZoneMultiplier: 2, GroupMultiplier: 1},
			normalizePurposeToken("Lab"):    {ZoneName: "Lab", ZoneMultiplier: 1, GroupMultiplier: 1},
		},
		SpaceZones:       map[string]string{},
		OutputKeyZones:   map[string]string{},
		GroupMultipliers: map[string]float64{},
	}
	result := epath071BuildCanonicalResult(series, multipliers)

	tests := []struct {
		period       string
		wantTotal    float64
		wantSensible float64
		wantLatent   float64
		wantShare    float64
		wantBadge    bool
	}{
		{period: "annual", wantTotal: 320, wantSensible: 280, wantLatent: 40, wantShare: 0.125, wantBadge: true},
		{period: "M1", wantTotal: 240, wantSensible: 222, wantLatent: 18, wantShare: 0.075, wantBadge: false},
		{period: "M2", wantTotal: 80, wantSensible: 58, wantLatent: 22, wantShare: 0.275, wantBadge: true},
	}
	for _, test := range tests {
		node := epath071LoadNode(result, test.period, "cooling")
		if node == nil {
			t.Errorf("%s Building cooling load is missing", test.period)
			continue
		}
		if node.Value != test.wantTotal || node.LatentShare != test.wantShare {
			t.Errorf("%s Building cooling metadata = value %g latentShare %g, want %g and %g", test.period, node.Value, node.LatentShare, test.wantTotal, test.wantShare)
		}
		if got := epath071Contains(node.Badges, "dehumidification_significant"); got != test.wantBadge {
			t.Errorf("%s Building cooling badge = %v (%#v), want %v", test.period, got, node.Badges, test.wantBadge)
		}
		sensible := review071LoadComponent(node.LoadBreakdown, "sensible")
		latent := review071LoadComponent(node.LoadBreakdown, "latent")
		if sensible == nil || sensible.Value != test.wantSensible || latent == nil || latent.Value != test.wantLatent || latent.Share != test.wantShare {
			t.Errorf("%s Building cooling breakdown = %#v, want sensible=%g latent=%g share=%g", test.period, node.LoadBreakdown, test.wantSensible, test.wantLatent, test.wantShare)
		}
		epath071AssertZoneClosure(t, result, test.period, "cooling")
	}

	office := review071ZoneResult(result.ZoneResults, "Office")
	if office == nil {
		t.Fatal("Office zone result is missing")
	}
	officeAnnual := epath071NodeByID(office.Nodes, "load.cooling.office")
	officeM1 := review071ZonePeriodLoad(office, "M1", "load.cooling.office")
	officeM2 := review071ZonePeriodLoad(office, "M2", "load.cooling.office")
	for _, check := range []struct {
		label     string
		node      *EnergyExplanationNode
		value     float64
		share     float64
		wantBadge bool
	}{
		{label: "annual", node: officeAnnual, value: 240, share: 1.0 / 6.0, wantBadge: true},
		{label: "M1", node: officeM1, value: 200, share: 0.09, wantBadge: false},
		{label: "M2", node: officeM2, value: 40, share: 0.55, wantBadge: true},
	} {
		if check.node == nil || check.node.Value != check.value || !review071Near(check.node.LatentShare, check.share) {
			t.Errorf("Office %s load = %#v, want value=%g latentShare=%g", check.label, check.node, check.value, check.share)
			continue
		}
		if got := epath071Contains(check.node.Badges, "dehumidification_significant"); got != check.wantBadge {
			t.Errorf("Office %s badge = %v (%#v), want %v", check.label, got, check.node.Badges, check.wantBadge)
		}
	}
}

func TestEPATH071ReviewStoredV1HumidityDetailDoesNotInflatePrimaryLoads(t *testing.T) {
	loadNode := func(id string, service string, value float64, sourceIDs ...string) EnergyExplanationNode {
		return EnergyExplanationNode{
			ID: id, Level: "load", Kind: "load.zone_" + service, Label: service + " load",
			Value: value, RawValue: value, EffectiveValue: value, AllocatedValue: value,
			Unit: "kWh", ZoneName: "Office", ServiceKind: service, PathType: "zone",
			ThermalComponent: "combined", SourceIDs: append([]string(nil), sourceIDs...),
		}
	}
	cooling := loadNode("load.cooling.office", "cooling", 100, "cooling-total", "cooling-latent")
	cooling.LoadBreakdown = []EnergyExplanationLoadComponent{
		{Component: "sensible", Value: 90, Share: 0.9, Unit: "kWh", SourceIDs: []string{"cooling-sensible"}},
		{Component: "latent", Value: 10, Share: 0.1, Unit: "kWh", SourceIDs: []string{"cooling-latent"}},
	}
	heating := loadNode("load.heating.office", "heating", 80, "heating-total", "heating-latent")
	heating.LoadBreakdown = []EnergyExplanationLoadComponent{
		{Component: "sensible", Value: 72, Share: 0.9, Unit: "kWh", SourceIDs: []string{"heating-sensible"}},
		{Component: "latent", Value: 8, Share: 0.1, Unit: "kWh", SourceIDs: []string{"heating-latent"}},
	}
	dehumidification := loadNode("load.dehumidification.office", "dehumidification", 10, "dehumidification-detail")
	dehumidification.ThermalComponent = "latent"
	humidification := loadNode("load.humidification.office", "humidification", 8, "humidification-detail")
	humidification.ThermalComponent = "latent"
	legacyNodes := []EnergyExplanationNode{cooling, dehumidification, heating, humidification}
	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{
		Schema:    energyExplanationV1Schema,
		Frequency: "Monthly",
		Nodes:     legacyNodes,
		Periods: []EnergyPeriod{{
			ID: "M1", Label: "Jan", Kind: "monthly", Nodes: legacyNodes,
		}},
		Sources: []EnergyDataSource{
			{ID: "cooling-total", SourceType: "stored-v1", ZoneName: "Office"},
			{ID: "cooling-sensible", SourceType: "stored-v1", ZoneName: "Office"},
			{ID: "cooling-latent", SourceType: "stored-v1", ZoneName: "Office"},
			{ID: "dehumidification-detail", SourceType: "stored-v1", ZoneName: "Office"},
			{ID: "heating-total", SourceType: "stored-v1", ZoneName: "Office"},
			{ID: "heating-sensible", SourceType: "stored-v1", ZoneName: "Office"},
			{ID: "heating-latent", SourceType: "stored-v1", ZoneName: "Office"},
			{ID: "humidification-detail", SourceType: "stored-v1", ZoneName: "Office"},
		},
		scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
	})

	for _, period := range []string{"annual", "M1"} {
		coolingNode := epath071LoadNode(result, period, "cooling")
		heatingNode := epath071LoadNode(result, period, "heating")
		if coolingNode == nil || coolingNode.Value != 100 {
			t.Errorf("stored-v1 %s cooling = %#v, want authoritative total 100 without additive dehumidification", period, coolingNode)
		}
		if heatingNode == nil || heatingNode.Value != 80 {
			t.Errorf("stored-v1 %s heating = %#v, want authoritative total 80 without additive humidification", period, heatingNode)
		}
		for _, unwanted := range []string{"load.dehumidification.building", "load.humidification.building"} {
			var node *EnergyExplanationNode
			if period == "annual" {
				node = epath071NodeByID(result.Nodes, unwanted)
			} else if selected := epath071PeriodByID(result.Periods, period); selected != nil {
				node = epath071NodeByID(selected.Nodes, unwanted)
			}
			if node != nil {
				t.Errorf("stored-v1 %s retained humidity detail as primary node: %#v", period, node)
			}
		}
	}
}

func review071LoadComponent(components []EnergyExplanationLoadComponent, wanted string) *EnergyExplanationLoadComponent {
	for index := range components {
		if components[index].Component == wanted {
			return &components[index]
		}
	}
	return nil
}

func review071ZoneResult(results []EnergyExplanationZoneResult, wanted string) *EnergyExplanationZoneResult {
	for index := range results {
		if results[index].Scope.ZoneName == wanted {
			return &results[index]
		}
	}
	return nil
}

func review071ZonePeriodLoad(result *EnergyExplanationZoneResult, periodID string, nodeID string) *EnergyExplanationNode {
	if result == nil {
		return nil
	}
	period := epath071PeriodByID(result.Periods, periodID)
	if period == nil {
		return nil
	}
	return epath071NodeByID(period.Nodes, nodeID)
}

func review071Near(left float64, right float64) bool {
	delta := left - right
	if delta < 0 {
		delta = -delta
	}
	return delta < 1e-6
}
