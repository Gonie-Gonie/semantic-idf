package simulation

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"sort"
	"testing"
)

type epath092AuditCarrierValue struct {
	carrier string
	value   float64
}

func TestEPATH092AuditOnlyMatchedHVACServicesCrossThermalBoundary(t *testing.T) {
	legacy := epath092AuditTopologyFixture()
	before, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}

	result := UpgradeEnergyExplanationV1(legacy)
	after, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("v1 input was mutated during v2 conversion\nbefore=%s\nafter=%s", before, after)
	}

	conversionLinks := epath092AuditLinksByRelation(result.Links, "load_to_end_use")
	if len(conversionLinks) != 2 {
		t.Fatalf("only Cooling and Heating may cross the thermal/site boundary; got %#v", conversionLinks)
	}

	cooling := energyPathV2LinkByIDs(result.Links, "load.cooling.building", "end_use.cooling.building")
	epath092AuditAssertRatio(t, cooling, 120, "kWh thermal", 30, "kWh site", 4, "coefficient_of_performance", "COP")
	heating := energyPathV2LinkByIDs(result.Links, "load.heating.building", "end_use.heating.building")
	epath092AuditAssertRatio(t, heating, 80, "kWh thermal", 100, "kWh site", .8, "efficiency", "Efficiency")

	for _, pair := range [][2]string{
		{"load.heating.building", "end_use.cooling.building"},
		{"load.cooling.building", "end_use.fans.building"},
		{"load.heating.building", "end_use.pumps.building"},
		{"load.cooling.building", "end_use.heat_rejection.building"},
		{"load.heating.building", "end_use.humidification.building"},
	} {
		if link := energyPathV2LinkByIDs(result.Links, pair[0], pair[1]); link != nil {
			t.Errorf("unrelated/auxiliary conversion link must be suppressed: %#v", link)
		}
	}

	for _, endUse := range []string{"fans", "pumps", "heat_rejection", "humidification"} {
		endUseID := "end_use." + endUse + ".building"
		link := energyPathV2LinkByIDs(result.Links, endUseID, "carrier.electricity.building")
		if link == nil || link.Relation != "direct_end_use_to_carrier" || link.FromValue != link.ToValue || link.FromValue <= 0 {
			t.Errorf("%s must remain in the lower direct/auxiliary lane: %#v", endUse, link)
		}
		if node := energyPathV2NodeByID(result.Nodes, endUseID); node == nil || epath092AuditOutgoingValue(result.Links, endUseID) != node.Value {
			t.Errorf("%s direct carrier branch does not close: node=%#v links=%#v", endUse, node, result.Links)
		}
	}

	for _, detail := range []struct {
		standaloneID string
		loadID       string
		sourceID     string
		component    string
	}{
		{standaloneID: "load.dehumidification.building", loadID: "load.cooling.building", sourceID: "source.dehumidification", component: "load.dehumidification"},
		{standaloneID: "load.humidification.building", loadID: "load.heating.building", sourceID: "source.humidification", component: "load.humidification"},
	} {
		if node := energyPathV2NodeByID(result.Nodes, detail.standaloneID); node != nil {
			t.Errorf("humidity detail escaped as a primary load node: %#v", node)
		}
		load := energyPathV2NodeByID(result.Nodes, detail.loadID)
		if load == nil || !stringSliceContains(load.SourceIDs, detail.sourceID) {
			t.Errorf("humidity detail is not traceable from %s: %#v", detail.loadID, load)
		}
		source := epath092AuditSourceByID(result.Sources, detail.sourceID)
		if source == nil || source.DriverRole != energyDriverSourceRoleContext || source.DriverComponent != detail.component || source.InspectorSection != energyDriverInspectorSectionBreakdown {
			t.Errorf("humidity detail source metadata = %#v", source)
		}
	}
	if !stringSliceContains(cooling.SourceIDs, "source.dehumidification") || !stringSliceContains(heating.SourceIDs, "source.humidification") {
		t.Errorf("conversion links lost humidity detail trace: cooling=%#v heating=%#v", cooling, heating)
	}

	summary := buildEnergyExplanationSummary(result)
	if len(summary.Ratios) != 2 {
		t.Fatalf("summary should expose only Cooling/Heating conversion ratios: %#v", summary.Ratios)
	}
}

func TestEPATH092AuditRatioClassificationMatrix(t *testing.T) {
	tests := []struct {
		name      string
		service   string
		carriers  []epath092AuditCarrierValue
		load      float64
		wantRatio float64
		wantKind  string
		wantLabel string
	}{
		{name: "electric cooling only is COP", service: "cooling", carriers: []epath092AuditCarrierValue{{"electricity", 25}}, load: 100, wantRatio: 4, wantKind: "coefficient_of_performance", wantLabel: "COP"},
		{name: "combustion heating at or below unity is efficiency", service: "heating", carriers: []epath092AuditCarrierValue{{"natural_gas", 100}}, load: 85, wantRatio: .85, wantKind: "efficiency", wantLabel: "Efficiency"},
		{name: "combustion heating above unity is load per fuel", service: "heating", carriers: []epath092AuditCarrierValue{{"fuel_oil_2", 100}}, load: 120, wantRatio: 1.2, wantKind: "load_to_fuel", wantLabel: "Load / fuel"},
		{name: "mixed carrier heating is load per site energy", service: "heating", carriers: []epath092AuditCarrierValue{{"electricity", 10}, {"natural_gas", 20}}, load: 90, wantRatio: 3, wantKind: "load_to_site_energy", wantLabel: "Load / site energy"},
		{name: "district cooling is purchased energy", service: "cooling", carriers: []epath092AuditCarrierValue{{"district_cooling", 25}}, load: 75, wantRatio: 3, wantKind: "load_to_purchased_energy", wantLabel: "Load / purchased energy"},
		{name: "district heating is purchased energy", service: "heating", carriers: []epath092AuditCarrierValue{{"district_heating", 40}}, load: 80, wantRatio: 2, wantKind: "load_to_purchased_energy", wantLabel: "Load / purchased energy"},
		{name: "steam heating is purchased energy", service: "heating", carriers: []epath092AuditCarrierValue{{"steam", 25}}, load: 50, wantRatio: 2, wantKind: "load_to_purchased_energy", wantLabel: "Load / purchased energy"},
		{name: "electric heating is not inferred as COP", service: "heating", carriers: []epath092AuditCarrierValue{{"electricity", 20}}, load: 60, wantRatio: 3, wantKind: "load_to_site_energy", wantLabel: "Load / site energy"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := UpgradeEnergyExplanationV1(epath092AuditConversionFixture(test.service, test.carriers, test.load, "kWh thermal", "kWh site"))
			link := energyPathV2LinkByIDs(result.Links, "load."+test.service+".building", "end_use."+test.service+".building")
			epath092AuditAssertRatio(t, link, test.load, "kWh thermal", epath092AuditCarrierTotal(test.carriers), "kWh site", test.wantRatio, test.wantKind, test.wantLabel)
			if len(buildEnergyExplanationSummary(result).Ratios) != 1 {
				t.Fatalf("ratio summary should contain exactly the classified conversion: %#v", buildEnergyExplanationSummary(result).Ratios)
			}
		})
	}
}

func TestEPATH092AuditInvalidRatiosNeverAcquireLabels(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EnergyExplanationV1)
	}{
		{
			name: "mismatched service",
			mutate: func(input *EnergyExplanationV1) {
				input.Nodes[len(input.Nodes)-1].ServiceKind = "heating"
				input.Nodes[len(input.Nodes)-1].Kind = "load.zone_heating"
				input.Edges[len(input.Edges)-1].ServiceKind = "heating"
			},
		},
		{
			name: "missing carrier",
			mutate: func(input *EnergyExplanationV1) {
				input.Nodes[0].Carrier = ""
				input.Nodes[1].Carrier = ""
			},
		},
		{
			name: "incompatible units",
			mutate: func(input *EnergyExplanationV1) {
				input.Nodes[1].Unit = "MJ site"
			},
		},
		{
			name: "matching non-energy units",
			mutate: func(input *EnergyExplanationV1) {
				input.Nodes[0].Unit = "widgets"
				input.Nodes[1].Unit = "widgets"
				input.Nodes[len(input.Nodes)-1].Unit = "widgets"
				for index := range input.Edges {
					input.Edges[index].Unit = "widgets"
				}
			},
		},
		{
			name: "zero thermal numerator",
			mutate: func(input *EnergyExplanationV1) {
				input.Nodes[len(input.Nodes)-1].Value = 0
				input.Edges[len(input.Edges)-1].Value = 0
			},
		},
		{
			name: "zero site denominator",
			mutate: func(input *EnergyExplanationV1) {
				input.Nodes[1].Value = 0
				input.Edges[0].Value = 0
			},
		},
		{
			name: "negative site denominator",
			mutate: func(input *EnergyExplanationV1) {
				input.Nodes[1].Value = -25
				input.Edges[0].Value = -25
			},
		},
		{
			name: "NaN site denominator",
			mutate: func(input *EnergyExplanationV1) {
				input.Nodes[1].Value = math.NaN()
				input.Edges[0].Value = math.NaN()
			},
		},
		{
			name: "infinite thermal numerator",
			mutate: func(input *EnergyExplanationV1) {
				input.Nodes[len(input.Nodes)-1].Value = math.Inf(1)
				input.Edges[len(input.Edges)-1].Value = math.Inf(1)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := epath092AuditConversionFixture("cooling", []epath092AuditCarrierValue{{"electricity", 25}}, 100, "kWh thermal", "kWh site")
			test.mutate(&input)
			result := UpgradeEnergyExplanationV1(input)
			link := epath092AuditLinksByRelation(result.Links, "load_to_end_use")
			if test.name == "mismatched service" && len(link) != 0 {
				t.Fatalf("service/end-use mismatch created a conversion: %#v", link)
			}
			for _, current := range link {
				if current.Ratio != 0 || current.RatioKind != "" || current.RatioLabel != "" || math.IsNaN(current.Ratio) || math.IsInf(current.Ratio, 0) {
					t.Errorf("invalid conversion exposed ratio metadata: %#v", current)
				}
			}
			if ratios := buildEnergyExplanationSummary(result).Ratios; len(ratios) != 0 {
				t.Errorf("invalid conversion leaked into ratio summary: %#v", ratios)
			}
		})
	}
}

func TestEPATH092AuditMonthlyAggregationIsOrderInvariantAndReclassifiesMixedCarrier(t *testing.T) {
	fixture := epath092AuditMonthlyFixture()
	before, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	forward := UpgradeEnergyExplanationV1(fixture)
	after, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("monthly v1 input was mutated\nbefore=%s\nafter=%s", before, after)
	}

	reversedInput := epath092AuditMonthlyFixture()
	epath092AuditReversePeriodsAndContents(reversedInput.Periods)
	reversed := UpgradeEnergyExplanationV1(reversedInput)
	if got, want := epath092AuditAnnualSnapshot(reversed), epath092AuditAnnualSnapshot(forward); !reflect.DeepEqual(got, want) {
		t.Fatalf("annual conversion depends on monthly/node/edge order\nreversed=%#v\nforward=%#v", got, want)
	}

	conversion := energyPathV2LinkByIDs(forward.Links, "load.cooling.building", "end_use.cooling.building")
	epath092AuditAssertRatio(t, conversion, 160, "kWh thermal", 45, "kWh site", 3.556, "load_to_site_energy", "Load / site energy")
	if node := energyPathV2NodeByID(forward.Nodes, "load.cooling.building"); node == nil || epath092AuditIncomingValue(forward.Links, node.ID, "driver_to_load") != node.Value {
		t.Fatalf("annual driver-to-load closure failed: node=%#v links=%#v", node, forward.Links)
	}
	if node := energyPathV2NodeByID(forward.Nodes, "end_use.cooling.building"); node == nil || epath092AuditOutgoingValueByRelation(forward.Links, node.ID, "end_use_to_carrier") != node.Value {
		t.Fatalf("annual cooling carrier split closure failed: node=%#v links=%#v", node, forward.Links)
	}
	for _, endUse := range []string{"fans", "pumps"} {
		if epath092AuditHasConversionTo(forward.Links, "end_use."+endUse+".building") {
			t.Errorf("%s auxiliary energy leaked into annual conversion", endUse)
		}
	}

	for _, periodID := range []string{"M1", "M2"} {
		period := energyExplanationPeriodByID(forward.Periods, periodID)
		if period == nil {
			t.Fatalf("missing %s graph", periodID)
		}
		load := energyPathV2NodeByID(period.Nodes, "load.cooling.building")
		if load == nil || epath092AuditIncomingValue(period.Links, load.ID, "driver_to_load") != load.Value {
			t.Errorf("%s driver-to-load closure failed: node=%#v links=%#v", periodID, load, period.Links)
		}
		endUse := energyPathV2NodeByID(period.Nodes, "end_use.cooling.building")
		if endUse == nil || epath092AuditOutgoingValueByRelation(period.Links, endUse.ID, "end_use_to_carrier") != endUse.Value {
			t.Errorf("%s end-use carrier closure failed: node=%#v links=%#v", periodID, endUse, period.Links)
		}
	}

	m1 := energyExplanationPeriodByID(forward.Periods, "M1")
	m2 := energyExplanationPeriodByID(forward.Periods, "M2")
	epath092AuditAssertRatio(t, energyPathV2LinkByIDs(m1.Links, "load.cooling.building", "end_use.cooling.building"), 100, "kWh thermal", 25, "kWh site", 4, "coefficient_of_performance", "COP")
	epath092AuditAssertRatio(t, energyPathV2LinkByIDs(m2.Links, "load.cooling.building", "end_use.cooling.building"), 60, "kWh thermal", 20, "kWh site", 3, "load_to_purchased_energy", "Load / purchased energy")
}

func epath092AuditTopologyFixture() EnergyExplanationV1 {
	nodes := []EnergyExplanationNode{
		{ID: "energy.carrier.electricity", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Value: 49, Unit: "kWh site", Carrier: "electricity", EndUse: "total", SourceIDs: []string{"meter.facility.electricity"}},
		{ID: "energy.carrier.natural_gas", Level: "energy", Kind: "energy.natural_gas.total", Label: "Natural gas", Value: 100, Unit: "kWh site", Carrier: "natural_gas", EndUse: "total", SourceIDs: []string{"meter.facility.gas"}},
		{ID: "energy.end_use.cooling.electricity", Level: "energy", Kind: "energy.cooling", Label: "Cooling", Value: 30, Unit: "kWh site", Carrier: "electricity", EndUse: "cooling", SourceIDs: []string{"meter.cooling"}},
		{ID: "energy.end_use.heating.natural_gas", Level: "energy", Kind: "energy.heating", Label: "Heating", Value: 100, Unit: "kWh site", Carrier: "natural_gas", EndUse: "heating", SourceIDs: []string{"meter.heating"}},
		{ID: "energy.end_use.fans.electricity", Level: "energy", Kind: "energy.fans", Label: "Fans", Value: 8, Unit: "kWh site", Carrier: "electricity", EndUse: "fans", SourceIDs: []string{"meter.fans"}},
		{ID: "energy.end_use.pumps.electricity", Level: "energy", Kind: "energy.pumps", Label: "Pumps", Value: 5, Unit: "kWh site", Carrier: "electricity", EndUse: "pumps", SourceIDs: []string{"meter.pumps"}},
		{ID: "energy.end_use.heat_rejection.electricity", Level: "energy", Kind: "energy.heat_rejection", Label: "Heat rejection", Value: 4, Unit: "kWh site", Carrier: "electricity", EndUse: "heat_rejection", SourceIDs: []string{"meter.heat-rejection"}},
		{ID: "energy.end_use.humidification.electricity", Level: "energy", Kind: "energy.humidification", Label: "Humidification", Value: 2, Unit: "kWh site", Carrier: "electricity", EndUse: "humidification", SourceIDs: []string{"meter.humidification"}},
		{ID: "load.cooling.office", Level: "load", Kind: "load.zone_cooling", Label: "Cooling load", Value: 120, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"source.cooling-load"}},
		{ID: "load.dehumidification.office", Level: "load", Kind: "load.zone_dehumidification", Label: "Dehumidification detail", Value: 12, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "dehumidification", SourceIDs: []string{"source.dehumidification"}},
		{ID: "load.heating.office", Level: "load", Kind: "load.zone_heating", Label: "Heating load", Value: 80, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "heating", SourceIDs: []string{"source.heating-load"}},
		{ID: "load.humidification.office", Level: "load", Kind: "load.zone_humidification", Label: "Humidification detail", Value: 8, Unit: "kWh thermal", ZoneName: "Office", ServiceKind: "humidification", SourceIDs: []string{"source.humidification"}},
	}
	edges := []EnergyExplanationEdge{
		{ID: "meter.cooling", FromID: nodes[0].ID, ToID: nodes[2].ID, Value: 30, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.cooling"}},
		{ID: "meter.heating", FromID: nodes[1].ID, ToID: nodes[3].ID, Value: 100, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.heating"}},
		{ID: "meter.fans", FromID: nodes[0].ID, ToID: nodes[4].ID, Value: 8, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.fans"}},
		{ID: "meter.pumps", FromID: nodes[0].ID, ToID: nodes[5].ID, Value: 5, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.pumps"}},
		{ID: "meter.heat-rejection", FromID: nodes[0].ID, ToID: nodes[6].ID, Value: 4, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.heat-rejection"}},
		{ID: "meter.humidification", FromID: nodes[0].ID, ToID: nodes[7].ID, Value: 2, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.humidification"}},
		{ID: "conversion.cooling", FromID: nodes[2].ID, ToID: nodes[8].ID, Value: 120, Unit: "kWh thermal", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "cooling", SourceIDs: []string{"source.cooling-load"}},
		{ID: "conversion.heating", FromID: nodes[3].ID, ToID: nodes[10].ID, Value: 80, Unit: "kWh thermal", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "heating", SourceIDs: []string{"source.heating-load"}},
		// Malformed historical edges deliberately try to force unrelated energy
		// through a thermal load. V2 must recognize endpoints, not trust relation text.
		{ID: "wrong.cross-service", FromID: nodes[2].ID, ToID: nodes[10].ID, Value: 80, Unit: "kWh thermal", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "heating"},
		{ID: "wrong.fans", FromID: nodes[4].ID, ToID: nodes[8].ID, Value: 120, Unit: "kWh thermal", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "cooling"},
		{ID: "wrong.pumps", FromID: nodes[5].ID, ToID: nodes[10].ID, Value: 80, Unit: "kWh thermal", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "heating"},
		{ID: "wrong.heat-rejection", FromID: nodes[6].ID, ToID: nodes[8].ID, Value: 120, Unit: "kWh thermal", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "cooling"},
		{ID: "wrong.humidification", FromID: nodes[7].ID, ToID: nodes[10].ID, Value: 80, Unit: "kWh thermal", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "heating"},
	}
	return EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes:            nodes,
		Edges:            edges,
		Sources: []EnergyDataSource{
			{ID: "source.cooling-load", SourceType: "sql_variable", Name: "Zone Ideal Loads Supply Air Total Cooling Energy", KeyValue: "Office"},
			{ID: "source.dehumidification", SourceType: "sql_variable", Name: "Zone Ideal Loads Supply Air Latent Cooling Energy", KeyValue: "Office"},
			{ID: "source.heating-load", SourceType: "sql_variable", Name: "Zone Ideal Loads Supply Air Total Heating Energy", KeyValue: "Office"},
			{ID: "source.humidification", SourceType: "sql_variable", Name: "Zone Ideal Loads Supply Air Latent Heating Energy", KeyValue: "Office"},
		},
	}
}

func epath092AuditConversionFixture(service string, carriers []epath092AuditCarrierValue, loadValue float64, loadUnit string, endUseUnit string) EnergyExplanationV1 {
	nodes := make([]EnergyExplanationNode, 0, len(carriers)*2+1)
	edges := make([]EnergyExplanationEdge, 0, len(carriers)+1)
	for index, item := range carriers {
		carrierID := "energy.carrier." + item.carrier
		endUseID := "energy.end_use." + service + "." + item.carrier
		nodes = append(nodes,
			EnergyExplanationNode{ID: carrierID, Level: "energy", Kind: "energy." + item.carrier + ".total", Label: item.carrier, Value: item.value, Unit: endUseUnit, Carrier: item.carrier, EndUse: "total", SourceIDs: []string{"facility." + item.carrier}},
			EnergyExplanationNode{ID: endUseID, Level: "energy", Kind: "energy." + service, Label: service, Value: item.value, Unit: endUseUnit, Carrier: item.carrier, EndUse: service, SourceIDs: []string{"meter." + service + "." + item.carrier}},
		)
		edges = append(edges, EnergyExplanationEdge{ID: "meter." + service + "." + item.carrier, FromID: carrierID, ToID: endUseID, Value: item.value, Unit: endUseUnit, Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter." + service + "." + item.carrier}})
		if index == 0 {
			// One physical service link is enough; the canonical end-use node owns
			// the complete carrier sum used as the conversion denominator.
			edges = append(edges, EnergyExplanationEdge{ID: "conversion." + service, FromID: endUseID, ToID: "load." + service, Value: loadValue, Unit: loadUnit, Relation: "delivered_load", Basis: "measured_variable", ServiceKind: service, SourceIDs: []string{"load." + service}})
		}
	}
	nodes = append(nodes, EnergyExplanationNode{ID: "load." + service, Level: "load", Kind: "load.zone_" + service, Label: service + " load", Value: loadValue, Unit: loadUnit, ServiceKind: service, SourceIDs: []string{"load." + service}})
	return EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes:            nodes,
		Edges:            edges,
	}
}

func epath092AuditMonthlyFixture() EnergyExplanationV1 {
	return EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "monthly",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Periods: []EnergyPeriod{
			epath092AuditCoolingPeriod("M1", "electricity", 25, 100, "fans", 5),
			epath092AuditCoolingPeriod("M2", "district_cooling", 20, 60, "pumps", 7),
		},
		canonicalMonthlyBasis: true,
	}
}

func epath092AuditCoolingPeriod(period string, serviceCarrier string, serviceEnergy float64, loadValue float64, auxiliary string, auxiliaryValue float64) EnergyPeriod {
	serviceCarrierID := "energy.carrier." + serviceCarrier
	electricityCarrierID := "energy.carrier.electricity"
	serviceEndUseID := "energy.end_use.cooling." + serviceCarrier
	auxiliaryEndUseID := "energy.end_use." + auxiliary + ".electricity"
	carrierTotals := map[string]float64{serviceCarrierID: serviceEnergy}
	carrierTotals[electricityCarrierID] += auxiliaryValue
	nodes := make([]EnergyExplanationNode, 0, len(carrierTotals)+4)
	for carrierID, value := range carrierTotals {
		carrier := carrierID[len("energy.carrier."):]
		nodes = append(nodes, EnergyExplanationNode{ID: carrierID, Level: "energy", Kind: "energy." + carrier + ".total", Label: carrier, Value: value, Unit: "kWh site", Carrier: carrier, EndUse: "total", Period: period, SourceIDs: []string{"facility." + carrier + "." + period}})
	}
	nodes = append(nodes,
		EnergyExplanationNode{ID: serviceEndUseID, Level: "energy", Kind: "energy.cooling", Label: "Cooling", Value: serviceEnergy, Unit: "kWh site", Carrier: serviceCarrier, EndUse: "cooling", Period: period, SourceIDs: []string{"meter.cooling." + serviceCarrier + "." + period}},
		EnergyExplanationNode{ID: auxiliaryEndUseID, Level: "energy", Kind: "energy." + auxiliary, Label: auxiliary, Value: auxiliaryValue, Unit: "kWh site", Carrier: "electricity", EndUse: auxiliary, Period: period, SourceIDs: []string{"meter." + auxiliary + "." + period}},
		EnergyExplanationNode{ID: "load.cooling", Level: "load", Kind: "load.zone_cooling", Label: "Cooling load", Value: loadValue, Unit: "kWh thermal", ServiceKind: "cooling", Period: period, SourceIDs: []string{"load.cooling." + period}},
		EnergyExplanationNode{ID: "heat.storage.cooling", Level: "heat", Kind: "heat.storage", Label: "Other / storage", Value: loadValue, SignedValue: loadValue, DisplayValue: loadValue, Unit: "kWh thermal", ServiceKind: "cooling", Period: period, HeatCategory: "storage_other", SourceIDs: []string{"driver.cooling." + period}},
	)
	edges := []EnergyExplanationEdge{
		{ID: "meter.cooling." + period, FromID: serviceCarrierID, ToID: serviceEndUseID, Value: serviceEnergy, Unit: "kWh site", Period: period, Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.cooling." + serviceCarrier + "." + period}},
		{ID: "meter." + auxiliary + "." + period, FromID: electricityCarrierID, ToID: auxiliaryEndUseID, Value: auxiliaryValue, Unit: "kWh site", Period: period, Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter." + auxiliary + "." + period}},
		{ID: "conversion.cooling." + period, FromID: serviceEndUseID, ToID: "load.cooling", Value: loadValue, Unit: "kWh thermal", Period: period, Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "cooling", SourceIDs: []string{"load.cooling." + period}},
		{ID: "driver.cooling." + period, FromID: "load.cooling", ToID: "heat.storage.cooling", Value: loadValue, Unit: "kWh thermal", Period: period, Relation: "heat_driver", Basis: "derived_balance", ServiceKind: "cooling", SourceIDs: []string{"driver.cooling." + period}},
	}
	return EnergyPeriod{ID: period, Label: period, Kind: "monthly", Nodes: nodes, Edges: edges}
}

type epath092AuditSnapshot struct {
	FromValue       float64
	ToValue         float64
	Ratio           float64
	RatioKind       string
	RatioLabel      string
	CoolingValue    float64
	CarrierBranches map[string]float64
	AuxiliaryLinks  map[string]string
}

func epath092AuditAnnualSnapshot(result EnergyExplanationResult) epath092AuditSnapshot {
	link := energyPathV2LinkByIDs(result.Links, "load.cooling.building", "end_use.cooling.building")
	out := epath092AuditSnapshot{CarrierBranches: map[string]float64{}, AuxiliaryLinks: map[string]string{}}
	if link != nil {
		out.FromValue = link.FromValue
		out.ToValue = link.ToValue
		out.Ratio = link.Ratio
		out.RatioKind = link.RatioKind
		out.RatioLabel = link.RatioLabel
	}
	if node := energyPathV2NodeByID(result.Nodes, "end_use.cooling.building"); node != nil {
		out.CoolingValue = node.Value
	}
	for _, current := range result.Links {
		if current.FromID == "end_use.cooling.building" && current.Relation == "end_use_to_carrier" {
			out.CarrierBranches[current.ToID] = current.FromValue
		}
		if current.FromID == "end_use.fans.building" || current.FromID == "end_use.pumps.building" {
			out.AuxiliaryLinks[current.FromID] = current.Relation
		}
	}
	return out
}

func epath092AuditReversePeriodsAndContents(periods []EnergyPeriod) {
	for left, right := 0, len(periods)-1; left < right; left, right = left+1, right-1 {
		periods[left], periods[right] = periods[right], periods[left]
	}
	for index := range periods {
		for left, right := 0, len(periods[index].Nodes)-1; left < right; left, right = left+1, right-1 {
			periods[index].Nodes[left], periods[index].Nodes[right] = periods[index].Nodes[right], periods[index].Nodes[left]
		}
		for left, right := 0, len(periods[index].Edges)-1; left < right; left, right = left+1, right-1 {
			periods[index].Edges[left], periods[index].Edges[right] = periods[index].Edges[right], periods[index].Edges[left]
		}
	}
}

func epath092AuditAssertRatio(t *testing.T, link *EnergyPathLink, fromValue float64, fromUnit string, toValue float64, toUnit string, ratio float64, kind string, label string) {
	t.Helper()
	if link == nil {
		t.Fatalf("missing conversion link")
	}
	if link.Relation != "load_to_end_use" || link.FromValue != fromValue || link.FromUnit != fromUnit || link.ToValue != toValue || link.ToUnit != toUnit || link.Ratio != ratio || link.RatioKind != kind || link.RatioLabel != label {
		t.Fatalf("conversion link = %#v; want values %g %q -> %g %q, ratio=%g kind=%q label=%q", link, fromValue, fromUnit, toValue, toUnit, ratio, kind, label)
	}
}

func epath092AuditLinksByRelation(links []EnergyPathLink, relation string) []*EnergyPathLink {
	out := make([]*EnergyPathLink, 0)
	for index := range links {
		if links[index].Relation == relation {
			out = append(out, &links[index])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func epath092AuditCarrierTotal(items []epath092AuditCarrierValue) float64 {
	total := 0.0
	for _, item := range items {
		total += item.value
	}
	return total
}

func epath092AuditSourceByID(sources []EnergyDataSource, id string) *EnergyDataSource {
	for index := range sources {
		if sources[index].ID == id {
			return &sources[index]
		}
	}
	return nil
}

func epath092AuditOutgoingValue(links []EnergyPathLink, fromID string) float64 {
	total := 0.0
	for _, link := range links {
		if link.FromID == fromID && (link.Relation == "end_use_to_carrier" || link.Relation == "direct_end_use_to_carrier") {
			total += link.FromValue
		}
	}
	return roundedEnergyNumber(total)
}

func epath092AuditOutgoingValueByRelation(links []EnergyPathLink, fromID string, relation string) float64 {
	total := 0.0
	for _, link := range links {
		if link.FromID == fromID && link.Relation == relation {
			total += link.FromValue
		}
	}
	return roundedEnergyNumber(total)
}

func epath092AuditIncomingValue(links []EnergyPathLink, toID string, relation string) float64 {
	total := 0.0
	for _, link := range links {
		if link.ToID == toID && link.Relation == relation {
			total += link.ToValue
		}
	}
	return roundedEnergyNumber(total)
}

func epath092AuditHasConversionTo(links []EnergyPathLink, endUseID string) bool {
	for _, link := range links {
		if link.Relation == "load_to_end_use" && link.ToID == endUseID {
			return true
		}
	}
	return false
}
