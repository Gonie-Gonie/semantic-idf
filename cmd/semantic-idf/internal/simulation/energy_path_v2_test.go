package simulation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpgradeEnergyExplanationV1BuildsCanonicalForwardLinks(t *testing.T) {
	legacy := energyExplanationV1ConversionFixture()
	result := UpgradeEnergyExplanationV1(legacy)

	if result.Schema != energyExplanationSchema {
		t.Fatalf("schema = %q", result.Schema)
	}
	if result.Scope.Kind != "building" || result.Scope.AggregationBasis != "model_total" {
		t.Fatalf("scope = %#v", result.Scope)
	}
	wantLevels := map[string]string{
		"driver.balance.storage_other.cooling.building": "driver",
		"load.cooling.building":                         "load",
		"end_use.cooling.building":                      "end_use",
		"carrier.electricity.building":                  "carrier",
	}
	for id, level := range wantLevels {
		node := energyPathV2NodeByID(result.Nodes, id)
		if node == nil || node.Level != level || node.AggregationBasis != "model_total" || node.RawValue == 0 || node.AllocatedValue == 0 || node.Multiplier != 1 {
			t.Fatalf("node %q = %#v", id, node)
		}
		if level == "driver" || level == "load" {
			if node.ScaleDomain != "thermal" {
				t.Fatalf("thermal node %q = %#v", id, node)
			}
		} else if node.ScaleDomain != "site" {
			t.Fatalf("site node %q = %#v", id, node)
		}
	}

	driverLink := energyPathV2LinkByRelation(result.Links, "driver_to_load")
	if driverLink == nil || driverLink.FromID != "driver.balance.storage_other.cooling.building" || driverLink.ToID != "load.cooling.building" || driverLink.FromValue != 100 || driverLink.ToValue != 100 || driverLink.Basis != "heat_balance_share" {
		t.Fatalf("driver link = %#v", driverLink)
	}
	conversion := energyPathV2LinkByRelation(result.Links, "load_to_end_use")
	if conversion == nil || conversion.FromID != "load.cooling.building" || conversion.ToID != "end_use.cooling.building" || conversion.FromValue != 100 || conversion.ToValue != 25 || conversion.Ratio != 4 || conversion.RatioKind != "coefficient_of_performance" || conversion.Basis != "direct_zone_energy" {
		t.Fatalf("load conversion link = %#v", conversion)
	}
	carrierLink := energyPathV2LinkByRelation(result.Links, "end_use_to_carrier")
	if carrierLink == nil || carrierLink.FromID != "end_use.cooling.building" || carrierLink.ToID != "carrier.electricity.building" || carrierLink.FromValue != 25 || carrierLink.ToValue != 25 || carrierLink.Basis != "reported_meter" {
		t.Fatalf("carrier link = %#v", carrierLink)
	}
}

func TestEnergyExplanationResultUnmarshalUpgradesStoredV1Bundle(t *testing.T) {
	legacy := energyExplanationV1ConversionFixture()
	payload, err := json.Marshal(struct {
		EnergyExplanation EnergyExplanationV1 `json:"energyExplanation"`
	}{EnergyExplanation: legacy})
	if err != nil {
		t.Fatal(err)
	}
	var bundle PurposeResultBundle
	if err := json.Unmarshal(payload, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.EnergyExplanation.Schema != energyExplanationSchema || len(bundle.EnergyExplanation.Links) != 3 {
		t.Fatalf("upgraded explanation = %#v", bundle.EnergyExplanation)
	}
	if bundle.EnergyExplanationSummary.Schema != energyExplanationSummarySchema || len(bundle.EnergyExplanationSummary.Drivers) != 1 || len(bundle.EnergyExplanationSummary.Ratios) != 1 {
		t.Fatalf("upgraded summary = %#v", bundle.EnergyExplanationSummary)
	}
}

func TestPurposeResultBundleRebuildsStoredV1SummaryWithCanonicalIDs(t *testing.T) {
	payload, err := json.Marshal(struct {
		EnergyExplanation EnergyExplanationV1 `json:"energyExplanation"`
		Summary           struct {
			Schema          string                         `json:"schema"`
			EnergyByCarrier []EnergyExplanationSummaryItem `json:"energyByCarrier"`
		} `json:"energyExplanationSummary"`
	}{
		EnergyExplanation: energyExplanationV1ConversionFixture(),
		Summary: struct {
			Schema          string                         `json:"schema"`
			EnergyByCarrier []EnergyExplanationSummaryItem `json:"energyByCarrier"`
		}{
			Schema:          "semantic-idf.energy-explanation-summary/v1",
			EnergyByCarrier: []EnergyExplanationSummaryItem{{ID: "energy.electricity.total", Label: "Legacy electricity", Value: 25}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var bundle PurposeResultBundle
	if err := json.Unmarshal(payload, &bundle); err != nil {
		t.Fatal(err)
	}
	if len(bundle.EnergyExplanationSummary.Carriers) != 1 || bundle.EnergyExplanationSummary.Carriers[0].ID != "carrier.electricity.building" {
		t.Fatalf("canonical rebuilt summary = %#v", bundle.EnergyExplanationSummary)
	}
}

func TestEnergyExplanationV2MarshalWritesLinksWithoutEdges(t *testing.T) {
	result := UpgradeEnergyExplanationV1(energyExplanationV1ConversionFixture())
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(payload, &root); err != nil {
		t.Fatal(err)
	}
	if _, ok := root["edges"]; ok {
		t.Fatalf("v2 payload contains deprecated edges: %s", payload)
	}
	if _, ok := root["links"]; !ok {
		t.Fatalf("v2 payload has no links: %s", payload)
	}
	var periods []map[string]json.RawMessage
	if err := json.Unmarshal(root["periods"], &periods); err != nil {
		t.Fatal(err)
	}
	for _, period := range periods {
		if _, ok := period["edges"]; ok {
			t.Fatalf("v2 period contains deprecated edges: %s", payload)
		}
		if _, ok := period["links"]; !ok {
			t.Fatalf("v2 period has no links: %s", payload)
		}
	}
}

func TestEnergyExplanationZeroValueDoesNotClaimV2Schema(t *testing.T) {
	payload, err := json.Marshal(EnergyExplanationResult{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), energyExplanationSchema) {
		t.Fatalf("empty result claimed a v2 schema: %s", payload)
	}
	var result EnergyExplanationResult
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	if result.Schema != "" {
		t.Fatalf("empty result schema = %q", result.Schema)
	}
}

func TestPurposeResultBundleZeroValueDoesNotAdvertiseV2(t *testing.T) {
	payload, err := json.Marshal(PurposeResultBundle{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), energyExplanationSchema) || strings.Contains(string(payload), energyExplanationSummarySchema) {
		t.Fatalf("empty purpose bundle advertised an energy-path schema: %s", payload)
	}
	var bundle PurposeResultBundle
	if err := json.Unmarshal(payload, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.EnergyExplanation.Schema != "" || bundle.EnergyExplanationSummary.Schema != "" {
		t.Fatalf("empty purpose bundle acquired schemas: %#v", bundle)
	}
	roundTrip, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(roundTrip), energyExplanationSchema) || strings.Contains(string(roundTrip), energyExplanationSummarySchema) {
		t.Fatalf("empty purpose bundle round trip advertised an energy-path schema: %s", roundTrip)
	}
}

func TestStoredEnergyExplanationV1PeriodEdgesRoundTripAsV2Links(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("testdata", "energy_path", "v1_energy_explanation.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result EnergyExplanationResult
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	annual := energyExplanationPeriodByID(result.Periods, "annual")
	if result.Schema != energyExplanationSchema || annual == nil || len(annual.Links) == 0 || len(annual.Edges) != 0 {
		t.Fatalf("upgraded stored period = %#v", annual)
	}
	link := energyPathV2LinkByIDs(annual.Links, "load.cooling.building", "end_use.cooling.building")
	if link == nil || link.Relation != "load_to_end_use" || link.ZoneName != "ZONE ONE" || !stringSliceContains(link.SourceIDs, "sql-rdd-21") || !stringSliceContains(link.SourceIDs, "sql-rdd-23") || len(link.RelatedPathIDs) == 0 {
		t.Fatalf("upgraded stored load link = %#v", link)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"edges"`) || !strings.Contains(string(encoded), `"links"`) {
		t.Fatalf("v2 round trip contract = %s", encoded)
	}
	var roundTrip EnergyExplanationResult
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	roundTripAnnual := energyExplanationPeriodByID(roundTrip.Periods, "annual")
	if roundTripAnnual == nil || energyPathV2LinkByIDs(roundTripAnnual.Links, link.FromID, link.ToID) == nil {
		t.Fatalf("round-trip period links = %#v", roundTripAnnual)
	}
}

func TestPersistedEnergyExplanationV2RebuildsTopZonesWithoutSummary(t *testing.T) {
	result := UpgradeEnergyExplanationV1(energyExplanationV1ConversionFixture())
	if len(result.ZoneContributions) == 0 {
		t.Fatalf("zone contributions = %#v", result.ZoneContributions)
	}
	payload, err := json.Marshal(PurposeResultBundle{EnergyExplanation: result})
	if err != nil {
		t.Fatal(err)
	}
	var bundle PurposeResultBundle
	if err := json.Unmarshal(payload, &bundle); err != nil {
		t.Fatal(err)
	}
	if len(bundle.EnergyExplanationSummary.TopZones) != 1 || !strings.EqualFold(bundle.EnergyExplanationSummary.TopZones[0].ZoneName, "Office") {
		t.Fatalf("rebuilt top zones = %#v", bundle.EnergyExplanationSummary.TopZones)
	}
}

func TestUpgradeEnergyExplanationV1DirectZoneDoesNotAllocateFacilityMeters(t *testing.T) {
	legacy := energyExplanationV1ConversionFixture()
	legacy.Reconciliation = []EnergyReconciliation{{ID: "reconcile.energy.electricity.annual", Level: "energy", Period: "annual", ExpectedValue: 25, ExplainedValue: 25, Unit: "kWh", Basis: "residual"}}
	legacy.scope = EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}
	result := UpgradeEnergyExplanationV1(legacy)
	if energyPathV2NodeByID(result.Nodes, "carrier.electricity.office") != nil || energyPathV2NodeByID(result.Nodes, "end_use.cooling.office") != nil {
		t.Fatalf("direct-only zone fabricated site allocation: %#v", result.Nodes)
	}
	if energyPathV2NodeByID(result.Nodes, "load.cooling.office") == nil || energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other.cooling.office") == nil {
		t.Fatalf("direct zone contributions missing: %#v", result.Nodes)
	}
	if energyPathV2LinkByRelation(result.Links, "load_to_end_use") != nil || energyPathV2LinkByRelation(result.Links, "end_use_to_carrier") != nil {
		t.Fatalf("direct-only zone fabricated conversion links: %#v", result.Links)
	}
	if len(result.Reconciliation) != 0 {
		t.Fatalf("direct-only zone fabricated facility reconciliation: %#v", result.Reconciliation)
	}
}

func TestUpgradeEnergyExplanationV1PrecomputesBuildingZoneAndMonthGraphs(t *testing.T) {
	legacy := energyExplanationV1ConversionFixture()
	monthlyNodes := append([]EnergyExplanationNode(nil), legacy.Nodes...)
	for index := range monthlyNodes {
		monthlyNodes[index].Period = "M1"
	}
	monthlyEdges := append([]EnergyExplanationEdge(nil), legacy.Edges...)
	for index := range monthlyEdges {
		monthlyEdges[index].Period = "M1"
	}
	legacy.Periods = append(legacy.Periods, EnergyPeriod{ID: "M1", Label: "January", Kind: "monthly", Nodes: monthlyNodes, Edges: monthlyEdges})
	result := UpgradeEnergyExplanationV1(legacy)
	if len(result.AvailableZones) != 1 || result.AvailableZones[0] != "Office" || len(result.ZoneResults) != 1 {
		t.Fatalf("zone inventory/results = %#v / %#v", result.AvailableZones, result.ZoneResults)
	}
	zone := result.ZoneResults[0]
	if zone.Scope.Kind != "zone" || zone.Scope.ZoneName != "Office" || zone.Scope.AggregationBasis != "model_total" {
		t.Fatalf("zone result scope = %#v", zone.Scope)
	}
	if energyPathV2NodeByID(zone.Nodes, "load.cooling.office") == nil || energyPathV2NodeByID(zone.Nodes, "driver.balance.storage_other.cooling.office") == nil {
		t.Fatalf("zone annual graph = %#v", zone.Nodes)
	}
	month := energyExplanationPeriodByID(zone.Periods, "M1")
	if month == nil || month.Summary == nil || month.Summary.Period != "M1" || month.Summary.Scope.Kind != "zone" || month.Summary.Scope.ZoneName != "Office" || len(month.Summary.Loads) != 1 || energyPathV2NodeByID(month.Nodes, "load.cooling.office") == nil || energyPathV2LinkByIDs(month.Links, "driver.balance.storage_other.cooling.office", "load.cooling.office") == nil {
		t.Fatalf("zone monthly graph = %#v", month)
	}
	if zone.Summary.Period != "annual" || zone.Summary.Scope.Kind != "zone" || zone.Summary.Scope.ZoneName != "Office" || len(zone.Summary.Loads) != 1 {
		t.Fatalf("zone annual summary = %#v", zone.Summary)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"availableZones"`) || !strings.Contains(string(payload), `"zoneResults"`) || strings.Contains(string(payload), `"edges"`) {
		t.Fatalf("zone projection wire contract = %s", payload)
	}
	var roundTrip EnergyExplanationResult
	if err := json.Unmarshal(payload, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if len(roundTrip.ZoneResults) != 1 || energyExplanationPeriodByID(roundTrip.ZoneResults[0].Periods, "M1") == nil {
		t.Fatalf("zone projection round trip = %#v", roundTrip.ZoneResults)
	}
}

func TestUpgradeEnergyExplanationV1SeparatesEndUseAndCarrierBranches(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema:  energyExplanationV1Schema,
		Purpose: string(SimulationPurposeBasicEnergy),
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.electricity", Level: "energy", Kind: "energy.total", Label: "Electricity", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "total", Basis: "measured_meter"},
			{ID: "energy.carrier.natural_gas", Level: "energy", Kind: "energy.total", Label: "Natural gas", Value: 20, Unit: "kWh", Carrier: "natural_gas", EndUse: "total", Basis: "measured_meter"},
			{ID: "energy.end_use.heating.electricity", Level: "energy", Kind: "energy.heating", Label: "Heating", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "heating", Basis: "measured_meter"},
			{ID: "energy.end_use.heating.natural_gas", Level: "energy", Kind: "energy.heating", Label: "Heating", Value: 20, Unit: "kWh", Carrier: "natural_gas", EndUse: "heating", Basis: "measured_meter"},
		},
		Edges: []EnergyExplanationEdge{
			{ID: "electric-heating", FromID: "energy.carrier.electricity", ToID: "energy.end_use.heating.electricity", Value: 10, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
			{ID: "gas-heating", FromID: "energy.carrier.natural_gas", ToID: "energy.end_use.heating.natural_gas", Value: 20, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
		},
	}
	result := UpgradeEnergyExplanationV1(legacy)
	endUse := energyPathV2NodeByID(result.Nodes, "end_use.heating.building")
	if endUse == nil || endUse.Value != 30 {
		t.Fatalf("merged heating end use = %#v", endUse)
	}
	linkedCarriers := map[string]bool{}
	for _, link := range result.Links {
		if link.FromID == endUse.ID {
			linkedCarriers[link.ToID] = true
		}
	}
	if !linkedCarriers["carrier.electricity.building"] || !linkedCarriers["carrier.natural_gas.building"] {
		t.Fatalf("carrier branches = %#v; links = %#v", linkedCarriers, result.Links)
	}
}

func TestEPATH090UpgradeBuildsCarrierReconciliationFromCanonicalSplits(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.electricity", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Value: 100, Unit: "kWh", Carrier: "electricity", EndUse: "total", SourceIDs: []string{"facility-electricity"}},
			{ID: "energy.carrier.natural_gas", Level: "energy", Kind: "energy.natural_gas.total", Label: "Natural gas", Value: 50, Unit: "kWh", Carrier: "natural_gas", EndUse: "total", SourceIDs: []string{"facility-gas"}},
			{ID: "energy.end_use.heating.electricity", Level: "energy", Kind: "energy.heating", Label: "Electricity heating", Value: 70, Unit: "kWh", Carrier: "electricity", EndUse: "heating", SourceIDs: []string{"node-electricity-heating"}},
			{ID: "energy.end_use.heating.natural_gas", Level: "energy", Kind: "energy.heating", Label: "Natural gas heating", Value: 60, Unit: "kWh", Carrier: "natural_gas", EndUse: "heating", SourceIDs: []string{"node-gas-heating"}},
			{ID: "load.heating.office", Level: "load", Kind: "load.zone_heating", Label: "Heating load", Value: 120, Unit: "kWh", ZoneName: "Office", ServiceKind: "heating"},
		},
		Edges: []EnergyExplanationEdge{
			{FromID: "energy.carrier.electricity", ToID: "energy.end_use.heating.electricity", Value: 70, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"edge-electricity-heating"}},
			{FromID: "energy.carrier.natural_gas", ToID: "energy.end_use.heating.natural_gas", Value: 60, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
			{FromID: "energy.end_use.heating.electricity", ToID: "load.heating.office", Value: 120, Unit: "kWh", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "heating"},
		},
	}

	result := UpgradeEnergyExplanationV1(legacy)
	heating := energyPathV2NodeByID(result.Nodes, "end_use.heating.building")
	if heating == nil || heating.Value != 130 || heating.Label != "Heating" || heating.Kind != "energy.heating" || heating.Carrier != "" {
		t.Fatalf("carrier-neutral Heating = %#v", heating)
	}
	electricity := energyPathV2LinkByIDs(result.Links, heating.ID, "carrier.electricity.building")
	if electricity == nil || electricity.Relation != "end_use_to_carrier" || electricity.FromValue != 70 || electricity.ToValue != 70 || len(electricity.SourceIDs) != 1 || electricity.SourceIDs[0] != "edge-electricity-heating" {
		t.Fatalf("electricity split provenance = %#v", electricity)
	}
	gas := energyPathV2LinkByIDs(result.Links, heating.ID, "carrier.natural_gas.building")
	if gas == nil || gas.Relation != "end_use_to_carrier" || gas.FromValue != 60 || gas.ToValue != 60 || len(gas.SourceIDs) != 1 || gas.SourceIDs[0] != "node-gas-heating" {
		t.Fatalf("natural-gas split provenance = %#v", gas)
	}
	electricityReconciliation := energyExplanationReconciliationByID(result.Reconciliation, "reconcile.energy.electricity.annual")
	if electricityReconciliation == nil || electricityReconciliation.ExpectedValue != 100 || electricityReconciliation.ExplainedValue != 70 || electricityReconciliation.ResidualValue != 30 || electricityReconciliation.Status != "residual" {
		t.Fatalf("electricity reconciliation = %#v", electricityReconciliation)
	}
	gasReconciliation := energyExplanationReconciliationByID(result.Reconciliation, "reconcile.energy.natural_gas.annual")
	if gasReconciliation == nil || gasReconciliation.ExpectedValue != 50 || gasReconciliation.ExplainedValue != 60 || gasReconciliation.ResidualValue != -10 || gasReconciliation.Status != "overmapped" {
		t.Fatalf("natural-gas reconciliation = %#v", gasReconciliation)
	}
}

func TestUpgradeEnergyExplanationV1UsesMergedMultiCarrierEndUseForRatio(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.electricity", Level: "energy", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "total"},
			{ID: "energy.carrier.natural_gas", Level: "energy", Value: 20, Unit: "kWh", Carrier: "natural_gas", EndUse: "total"},
			{ID: "energy.end_use.heating.electricity", Level: "energy", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "heating", SourceIDs: []string{"electric-heating"}},
			{ID: "energy.end_use.heating.natural_gas", Level: "energy", Value: 20, Unit: "kWh", Carrier: "natural_gas", EndUse: "heating", SourceIDs: []string{"gas-heating"}},
			{ID: "load.heating.office", Level: "load", Value: 90, Unit: "kWh", ZoneName: "Office", ServiceKind: "heating", SourceIDs: []string{"heating-load"}},
		},
		Edges: []EnergyExplanationEdge{
			{FromID: "energy.carrier.electricity", ToID: "energy.end_use.heating.electricity", Value: 10, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
			{FromID: "energy.carrier.natural_gas", ToID: "energy.end_use.heating.natural_gas", Value: 20, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
			{FromID: "energy.end_use.heating.electricity", ToID: "load.heating.office", Value: 90, Unit: "kWh", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "heating"},
		},
	}
	result := UpgradeEnergyExplanationV1(legacy)
	link := energyPathV2LinkByRelation(result.Links, "load_to_end_use")
	if link == nil || link.FromValue != 90 || link.ToValue != 30 || link.Ratio != 3 || link.RatioKind != "load_to_site_energy" || !stringSliceContains(link.SourceIDs, "electric-heating") || !stringSliceContains(link.SourceIDs, "gas-heating") {
		t.Fatalf("multi-carrier load conversion = %#v", link)
	}
}

func TestUpgradeEnergyExplanationV1UsesEfficiencyForCombustionHeating(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.natural_gas", Level: "energy", Value: 100, Unit: "kWh", Carrier: "natural_gas", EndUse: "total"},
			{ID: "energy.end_use.heating.natural_gas", Level: "energy", Value: 100, Unit: "kWh", Carrier: "natural_gas", EndUse: "heating"},
			{ID: "load.heating.office", Level: "load", Value: 85, Unit: "kWh", ZoneName: "Office", ServiceKind: "heating"},
		},
		Edges: []EnergyExplanationEdge{
			{FromID: "energy.carrier.natural_gas", ToID: "energy.end_use.heating.natural_gas", Value: 100, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
			{FromID: "energy.end_use.heating.natural_gas", ToID: "load.heating.office", Value: 85, Unit: "kWh", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "heating"},
		},
	}
	result := UpgradeEnergyExplanationV1(legacy)
	link := energyPathV2LinkByRelation(result.Links, "load_to_end_use")
	if link == nil || link.FromValue != 85 || link.ToValue != 100 || link.Ratio != 0.85 || link.RatioKind != "efficiency" || link.RatioLabel != "Efficiency" {
		t.Fatalf("gas heating conversion = %#v", link)
	}
}

func TestUpgradeEnergyExplanationV1UsesMergedMultiCarrierEndUseForAllocationRatio(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		AllocationPolicy: PurposeAllocationPolicyByZoneLoadShare,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.electricity", Level: "energy", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "total"},
			{ID: "energy.carrier.natural_gas", Level: "energy", Value: 20, Unit: "kWh", Carrier: "natural_gas", EndUse: "total"},
			{ID: "energy.end_use.heating.electricity", Level: "energy", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "heating", SourceIDs: []string{"electric-heating"}},
			{ID: "energy.end_use.heating.natural_gas", Level: "energy", Value: 20, Unit: "kWh", Carrier: "natural_gas", EndUse: "heating", SourceIDs: []string{"gas-heating"}},
			{ID: "load.heating.office", Level: "load", Value: 90, Unit: "kWh", ZoneName: "Office", ServiceKind: "heating"},
		},
		Edges: []EnergyExplanationEdge{
			{FromID: "energy.carrier.electricity", ToID: "energy.end_use.heating.electricity", Value: 10, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
			{FromID: "energy.carrier.natural_gas", ToID: "energy.end_use.heating.natural_gas", Value: 20, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
			{FromID: "energy.end_use.heating.electricity", ToID: "load.heating.office", Value: 10, Unit: "kWh", Relation: "allocation", Basis: "allocated", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "heating"},
		},
	}
	result := UpgradeEnergyExplanationV1(legacy)
	link := energyPathV2LinkByRelation(result.Links, "load_to_end_use")
	if link == nil || link.FromValue != 90 || link.ToValue != 30 || link.Ratio != 3 || link.RatioKind != "load_to_site_energy" || link.Basis != "zone_load_allocation" || !stringSliceContains(link.SourceIDs, "electric-heating") || !stringSliceContains(link.SourceIDs, "gas-heating") {
		t.Fatalf("multi-carrier allocated conversion = %#v", link)
	}
}

func TestUpgradeEnergyExplanationV1AllocatesMixedCarriersByServiceShare(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		AllocationPolicy: PurposeAllocationPolicyByZoneLoadShare,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.electricity", Level: "energy", Value: 80, Unit: "kWh", Carrier: "electricity", EndUse: "total"},
			{ID: "energy.carrier.natural_gas", Level: "energy", Value: 20, Unit: "kWh", Carrier: "natural_gas", EndUse: "total"},
			{ID: "energy.end_use.heating.electricity", Level: "energy", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "heating"},
			{ID: "energy.end_use.heating.natural_gas", Level: "energy", Value: 20, Unit: "kWh", Carrier: "natural_gas", EndUse: "heating"},
			{ID: "energy.end_use.cooling.electricity", Level: "energy", Value: 70, Unit: "kWh", Carrier: "electricity", EndUse: "cooling"},
			{ID: "load.heating.office", Level: "load", Value: 90, Unit: "kWh", ZoneName: "Office", ServiceKind: "heating"},
			{ID: "load.heating.lab", Level: "load", Value: 10, Unit: "kWh", ZoneName: "Lab", ServiceKind: "heating"},
			{ID: "load.cooling.office", Level: "load", Value: 10, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling"},
			{ID: "load.cooling.lab", Level: "load", Value: 90, Unit: "kWh", ZoneName: "Lab", ServiceKind: "cooling"},
		},
		Edges: []EnergyExplanationEdge{
			{FromID: "energy.carrier.electricity", ToID: "energy.end_use.heating.electricity", Value: 10, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
			{FromID: "energy.carrier.natural_gas", ToID: "energy.end_use.heating.natural_gas", Value: 20, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
			{FromID: "energy.carrier.electricity", ToID: "energy.end_use.cooling.electricity", Value: 70, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
			{FromID: "energy.end_use.heating.electricity", ToID: "load.heating.office", Value: 9, Unit: "kWh", Relation: "allocation", Basis: "allocated", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "heating"},
			{FromID: "energy.end_use.heating.electricity", ToID: "load.heating.lab", Value: 1, Unit: "kWh", Relation: "allocation", Basis: "allocated", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "heating"},
			{FromID: "energy.end_use.cooling.electricity", ToID: "load.cooling.office", Value: 7, Unit: "kWh", Relation: "allocation", Basis: "allocated", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "cooling"},
			{FromID: "energy.end_use.cooling.electricity", ToID: "load.cooling.lab", Value: 63, Unit: "kWh", Relation: "allocation", Basis: "allocated", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "cooling"},
		},
	}
	zoneResult := func(zone string) EnergyExplanationResult {
		copy := legacy
		copy.scope = EnergyExplanationScope{Kind: "zone", ZoneName: zone}
		return UpgradeEnergyExplanationV1(copy)
	}
	office := zoneResult("Office")
	if node := energyPathV2NodeByID(office.Nodes, "end_use.heating.office"); node == nil || node.Value != 27 {
		t.Fatalf("office heating end use = %#v", node)
	}
	if node := energyPathV2NodeByID(office.Nodes, "end_use.cooling.office"); node == nil || node.Value != 7 {
		t.Fatalf("office cooling end use = %#v", node)
	}
	if node := energyPathV2NodeByID(office.Nodes, "carrier.electricity.office"); node == nil || node.Value != 16 {
		t.Fatalf("office electricity carrier = %#v", node)
	}
	if node := energyPathV2NodeByID(office.Nodes, "carrier.natural_gas.office"); node == nil || node.Value != 18 {
		t.Fatalf("office gas carrier = %#v", node)
	}
	heatingLink := energyPathV2LinkByIDs(office.Links, "load.heating.office", "end_use.heating.office")
	if heatingLink == nil || heatingLink.FromValue != 90 || heatingLink.ToValue != 27 || heatingLink.RatioKind != "load_to_site_energy" {
		t.Fatalf("office heating conversion = %#v", heatingLink)
	}
	lab := zoneResult("Lab")
	for _, stage := range []struct {
		officeID string
		labID    string
		building string
	}{
		{"end_use.heating.office", "end_use.heating.lab", "end_use.heating.building"},
		{"end_use.cooling.office", "end_use.cooling.lab", "end_use.cooling.building"},
		{"carrier.electricity.office", "carrier.electricity.lab", "carrier.electricity.building"},
		{"carrier.natural_gas.office", "carrier.natural_gas.lab", "carrier.natural_gas.building"},
	} {
		buildingNode := energyPathV2NodeByID(UpgradeEnergyExplanationV1(legacy).Nodes, stage.building)
		officeNode := energyPathV2NodeByID(office.Nodes, stage.officeID)
		labNode := energyPathV2NodeByID(lab.Nodes, stage.labID)
		if buildingNode == nil || officeNode == nil || labNode == nil || officeNode.Value+labNode.Value != buildingNode.Value {
			t.Fatalf("zone sum for %s = %#v + %#v, building %#v", stage.building, officeNode, labNode, buildingNode)
		}
	}
}

func TestUpgradeEnergyExplanationV1UnionsLinkTraceAndZoneMetadata(t *testing.T) {
	result := UpgradeEnergyExplanationV1(energyExplanationV1ConversionFixture())
	loadLink := energyPathV2LinkByRelation(result.Links, "load_to_end_use")
	if loadLink == nil || loadLink.ZoneName != "Office" || !stringSliceContains(loadLink.SourceIDs, "cooling") || !stringSliceContains(loadLink.SourceIDs, "load") || !stringSliceContains(loadLink.RelatedPathIDs, "path.office.cooling") {
		t.Fatalf("load-link trace metadata = %#v", loadLink)
	}
	carrierLink := energyPathV2LinkByRelation(result.Links, "end_use_to_carrier")
	if carrierLink == nil || stringSliceContains(carrierLink.SourceIDs, "facility") || !stringSliceContains(carrierLink.SourceIDs, "cooling") {
		t.Fatalf("carrier-link trace metadata = %#v", carrierLink)
	}
}

func TestUpgradeEnergyExplanationV1PreservesPerSourceMultiplierAccounting(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{ID: "heat.internal.office", Level: "heat", Kind: "heat.internal", Value: 10, RawValue: 10, Multiplier: 3, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"office-driver"}},
			{ID: "heat.internal.lab", Level: "heat", Kind: "heat.internal", Value: 5, RawValue: 5, Multiplier: 2, Unit: "kWh", ZoneName: "Lab", ServiceKind: "cooling", SourceIDs: []string{"lab-driver"}},
		},
		Sources: []EnergyDataSource{
			{ID: "office-driver", SourceType: "sql_variable"},
			{ID: "lab-driver", SourceType: "sql_variable"},
		},
	}
	building := UpgradeEnergyExplanationV1(legacy)
	office := energyExplanationSourceByID(building.Sources, "office-driver")
	lab := energyExplanationSourceByID(building.Sources, "lab-driver")
	if office == nil || office.RawValue != 10 || office.EffectiveValue != 30 || office.EffectiveMultiplier != 3 || office.AllocationFactor != 1 || office.AllocatedValue != 30 {
		t.Fatalf("office source accounting = %#v", office)
	}
	if lab == nil || lab.RawValue != 5 || lab.EffectiveValue != 10 || lab.EffectiveMultiplier != 2 || lab.AllocationFactor != 1 || lab.AllocatedValue != 10 {
		t.Fatalf("lab source accounting = %#v", lab)
	}
	legacy.scope = EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}
	zone := UpgradeEnergyExplanationV1(legacy)
	if energyExplanationSourceByID(zone.Sources, "office-driver") == nil || energyExplanationSourceByID(zone.Sources, "lab-driver") != nil {
		t.Fatalf("zone source filtering = %#v", zone.Sources)
	}
}

func TestUpgradeEnergyExplanationV1KeepsAllocationSeparateFromPhysicalMultiplier(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		AllocationPolicy: PurposeAllocationPolicyByZoneLoadShare,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.electricity", Level: "energy", Value: 100, Unit: "kWh", Carrier: "electricity", EndUse: "total", SourceIDs: []string{"facility"}},
			{ID: "energy.end_use.cooling.electricity", Level: "energy", Value: 100, Unit: "kWh", Carrier: "electricity", EndUse: "cooling", SourceIDs: []string{"cooling"}},
			{ID: "load.cooling.office", Level: "load", Value: 25, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"office-load"}},
			{ID: "load.cooling.lab", Level: "load", Value: 75, Unit: "kWh", ZoneName: "Lab", ServiceKind: "cooling", SourceIDs: []string{"lab-load"}},
		},
		Edges: []EnergyExplanationEdge{
			{FromID: "energy.carrier.electricity", ToID: "energy.end_use.cooling.electricity", Value: 100, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
			{FromID: "energy.end_use.cooling.electricity", ToID: "load.cooling.office", Value: 25, Unit: "kWh", Relation: "allocation", Basis: "allocated", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "cooling"},
			{FromID: "energy.end_use.cooling.electricity", ToID: "load.cooling.lab", Value: 75, Unit: "kWh", Relation: "allocation", Basis: "allocated", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "cooling"},
		},
		Sources: []EnergyDataSource{
			{ID: "facility", SourceType: "sql_meter"},
			{ID: "cooling", SourceType: "sql_meter"},
			{ID: "office-load", SourceType: "sql_variable"},
			{ID: "lab-load", SourceType: "sql_variable"},
		},
		scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"},
	}
	result := UpgradeEnergyExplanationV1(legacy)
	facility := energyExplanationSourceByID(result.Sources, "facility")
	if facility == nil || facility.RawValue != 100 || facility.EffectiveValue != 100 || facility.EffectiveMultiplier != 1 || facility.AllocationFactor != 0.25 || facility.AllocatedValue != 25 {
		t.Fatalf("allocated facility source = %#v", facility)
	}
	load := energyExplanationSourceByID(result.Sources, "office-load")
	if load == nil || load.RawValue != 25 || load.EffectiveValue != 25 || load.EffectiveMultiplier != 1 || load.AllocationFactor != 1 || load.AllocatedValue != 25 {
		t.Fatalf("direct load source = %#v", load)
	}
}

func TestUpgradeEnergyExplanationV1RebuildsDirectZoneResidualAndPreservesSupportLinks(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.electricity.office", Level: "energy", Value: 25, Unit: "kWh", Carrier: "electricity", EndUse: "total", ZoneName: "Office"},
			{ID: "residual.energy.electricity.office", Level: "residual", Value: 5, Unit: "kWh", Carrier: "electricity", ZoneName: "Office"},
			{ID: "energy.end_use.generators.electricity.office", Level: "energy", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "generators", ZoneName: "Office"},
		},
		Edges: []EnergyExplanationEdge{
			{FromID: "energy.carrier.electricity.office", ToID: "residual.energy.electricity.office", Value: 5, Unit: "kWh", Relation: "residual", Basis: "residual"},
			{FromID: "energy.carrier.electricity.office", ToID: "energy.end_use.generators.electricity.office", Value: 10, Unit: "kWh", Relation: "onsite_production", Basis: "measured_meter"},
		},
		scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"},
	}
	result := UpgradeEnergyExplanationV1(legacy)
	residual := energyPathV2NodeByID(result.Nodes, "residual.site_electricity.office")
	residualLink := energyPathV2LinkByRelation(result.Links, "residual")
	if residual == nil || residual.Value != 25 || residual.Basis != "residual" || residualLink == nil || residualLink.FromValue != 25 || residualLink.ToValue != 25 || residualLink.ZoneName != "Office" {
		t.Fatalf("zone residual node/link = %#v / %#v", residual, residualLink)
	}
	support := energyPathV2NodeByID(result.Nodes, "support.generators.office")
	supportLink := energyPathV2LinkByRelation(result.Links, "support_supply")
	if support == nil || support.Value != 10 || supportLink == nil || supportLink.FromValue != 10 || supportLink.ToValue != 10 || supportLink.ZoneName != "Office" {
		t.Fatalf("zone support node/link = %#v / %#v", support, supportLink)
	}
}

func TestUpgradeEnergyExplanationV1ZoneAllocationsSumToBuilding(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		AllocationPolicy: PurposeAllocationPolicyByZoneLoadShare,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.electricity", Level: "energy", Value: 100, Unit: "kWh", Carrier: "electricity", EndUse: "total", SourceIDs: []string{"facility"}},
			{ID: "energy.end_use.cooling.electricity", Level: "energy", Value: 100, Unit: "kWh", Carrier: "electricity", EndUse: "cooling", SourceIDs: []string{"cooling"}},
			{ID: "load.cooling.office", Level: "load", Value: 25, RawValue: 25, Multiplier: 1, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling"},
			{ID: "load.cooling.lab", Level: "load", Value: 25, RawValue: 25, Multiplier: 3, Unit: "kWh", ZoneName: "Lab", ServiceKind: "cooling"},
		},
		Edges: []EnergyExplanationEdge{
			{FromID: "energy.carrier.electricity", ToID: "energy.end_use.cooling.electricity", Value: 100, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
			{FromID: "energy.end_use.cooling.electricity", ToID: "load.cooling.office", Value: 25, Unit: "kWh", Relation: "allocation", Basis: "allocated", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "cooling"},
			{FromID: "energy.end_use.cooling.electricity", ToID: "load.cooling.lab", Value: 75, Unit: "kWh", Relation: "allocation", Basis: "allocated", RuleID: energyRelationshipRuleAllocatedZoneLoad, ServiceKind: "cooling"},
		},
		Sources: []EnergyDataSource{
			{ID: "facility", SourceType: "sql_meter"},
			{ID: "cooling", SourceType: "sql_meter"},
		},
	}
	building := UpgradeEnergyExplanationV1(legacy)
	buildingEndUse := energyPathV2NodeByID(building.Nodes, "end_use.cooling.building")
	buildingCarrier := energyPathV2NodeByID(building.Nodes, "carrier.electricity.building")
	if buildingEndUse == nil || buildingCarrier == nil {
		t.Fatalf("building nodes = %#v", building.Nodes)
	}
	zoneValue := func(zone string, nodePrefix string) float64 {
		copy := legacy
		copy.scope = EnergyExplanationScope{Kind: "zone", ZoneName: zone}
		result := UpgradeEnergyExplanationV1(copy)
		node := energyPathV2NodeByID(result.Nodes, nodePrefix+"."+strings.ToLower(zone))
		if node == nil {
			t.Fatalf("%s node %s = %#v", zone, nodePrefix, result.Nodes)
		}
		return node.Value
	}
	endUseSum := zoneValue("Office", "end_use.cooling") + zoneValue("Lab", "end_use.cooling")
	carrierSum := zoneValue("Office", "carrier.electricity") + zoneValue("Lab", "carrier.electricity")
	if endUseSum != buildingEndUse.Value || carrierSum != buildingCarrier.Value {
		t.Fatalf("zone allocations end use/carrier = %g/%g; building = %g/%g", endUseSum, carrierSum, buildingEndUse.Value, buildingCarrier.Value)
	}
	facilitySource := energyExplanationSourceByID(building.Sources, "facility")
	if facilitySource == nil || len(facilitySource.ScopeDetails) != 2 || facilitySource.ScopeDetails[0].Scope.ZoneName != "Lab" || facilitySource.ScopeDetails[0].AllocatedValue != 75 || facilitySource.ScopeDetails[1].Scope.ZoneName != "Office" || facilitySource.ScopeDetails[1].AllocatedValue != 25 {
		t.Fatalf("facility zone scope details = %#v", facilitySource)
	}
}

func TestUpgradeEnergyExplanationV1PreservesZoneScopeAndEffectiveValue(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema: energyExplanationV1Schema,
		Nodes: []EnergyExplanationNode{
			{
				ID:          "heat.internal_convective.office",
				Level:       "heat",
				Kind:        "heat.internal_convective",
				Label:       "Internal gains",
				Value:       10,
				RawValue:    10,
				Multiplier:  3,
				Unit:        "kWh",
				ZoneName:    "Office",
				ServiceKind: "cooling",
			},
			{
				ID:          "heat.internal_convective.lab",
				Level:       "heat",
				Kind:        "heat.internal_convective",
				Label:       "Internal gains",
				Value:       5,
				RawValue:    5,
				Multiplier:  2,
				Unit:        "kWh",
				ZoneName:    "Lab",
				ServiceKind: "cooling",
			},
		},
		scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"},
	}
	result := UpgradeEnergyExplanationV1(legacy)
	if result.Scope.Kind != "zone" || result.Scope.ZoneName != "Office" || result.Scope.AggregationBasis != "model_total" {
		t.Fatalf("zone scope = %#v", result.Scope)
	}
	node := energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other.cooling.office")
	if node == nil || node.ZoneName != "Office" || node.RawValue != 10 || node.AllocatedValue != 30 || node.Value != 30 || node.Multiplier != 3 {
		t.Fatalf("zone effective node = %#v", node)
	}

	legacy.scope = EnergyExplanationScope{Kind: "zone", ZoneName: "Lab"}
	lab := UpgradeEnergyExplanationV1(legacy)
	labNode := energyPathV2NodeByID(lab.Nodes, "driver.balance.storage_other.cooling.lab")
	if labNode == nil || labNode.Value != 10 {
		t.Fatalf("lab effective node = %#v", labNode)
	}
	legacy.scope = EnergyExplanationScope{}
	building := UpgradeEnergyExplanationV1(legacy)
	buildingNode := energyPathV2NodeByID(building.Nodes, "driver.balance.storage_other.cooling.building")
	if buildingNode == nil || buildingNode.Value != node.Value+labNode.Value {
		t.Fatalf("building contribution = %#v; zones = %g + %g", buildingNode, node.Value, labNode.Value)
	}
}

func TestBuildEnergyExplanationSummaryV2UsesFourStagesAndAccountingFields(t *testing.T) {
	result := UpgradeEnergyExplanationV1(energyExplanationV1ConversionFixture())
	summary := buildEnergyExplanationSummary(result)
	if summary.Schema != energyExplanationSummarySchema || summary.Scope.Kind != "building" || summary.Scope.AggregationBasis != "model_total" {
		t.Fatalf("summary identity = %#v", summary)
	}
	for name, items := range map[string][]EnergyExplanationSummaryItem{
		"drivers":  summary.Drivers,
		"loads":    summary.Loads,
		"endUses":  summary.EndUses,
		"carriers": summary.Carriers,
	} {
		if len(items) != 1 || items[0].RawValue == 0 || items[0].AllocatedValue == 0 || items[0].Basis == "" || items[0].AggregationBasis != "model_total" {
			t.Fatalf("%s summary = %#v", name, items)
		}
	}
	if len(summary.Ratios) != 1 || summary.Ratios[0].Value != 4 || summary.Ratios[0].RawValue != 100 || summary.Ratios[0].AllocatedValue != 25 {
		t.Fatalf("ratios = %#v", summary.Ratios)
	}
	if len(summary.TopZones) != 1 || !strings.EqualFold(summary.TopZones[0].ZoneName, "Office") {
		t.Fatalf("top zones = %#v", summary.TopZones)
	}
}

func TestBuildEnergyExplanationSummaryV2SelectsRequestedPeriod(t *testing.T) {
	legacy := energyExplanationV1ConversionFixture()
	monthlyNodes := append([]EnergyExplanationNode(nil), legacy.Nodes...)
	for index := range monthlyNodes {
		monthlyNodes[index].Value /= 2
		monthlyNodes[index].SignedValue /= 2
		monthlyNodes[index].DisplayValue /= 2
		monthlyNodes[index].Period = "M1"
	}
	monthlyEdges := append([]EnergyExplanationEdge(nil), legacy.Edges...)
	for index := range monthlyEdges {
		monthlyEdges[index].Value /= 2
		monthlyEdges[index].SignedValue /= 2
		monthlyEdges[index].DisplayValue /= 2
		monthlyEdges[index].Period = "M1"
	}
	legacy.Periods = append(legacy.Periods, EnergyPeriod{ID: "M1", Label: "January", Kind: "monthly", Nodes: monthlyNodes, Edges: monthlyEdges})
	result := UpgradeEnergyExplanationV1(legacy)
	summary := buildEnergyExplanationSummaryForPeriod(result, "M1")
	if summary.Period != "M1" || len(summary.Drivers) != 1 || summary.Drivers[0].Value != 50 || len(summary.Loads) != 1 || summary.Loads[0].Value != 50 || len(summary.EndUses) != 1 || summary.EndUses[0].Value != 12.5 || len(summary.Carriers) != 1 || summary.Carriers[0].Value != 12.5 || len(summary.Ratios) != 1 || summary.Ratios[0].Value != 4 {
		t.Fatalf("M1 summary = %#v", summary)
	}
	period := energyExplanationPeriodByID(result.Periods, "M1")
	if period == nil || period.Summary == nil || period.Summary.Period != "M1" || len(period.Summary.Carriers) != 1 || period.Summary.Carriers[0].Value != 12.5 {
		t.Fatalf("serialized M1 summary = %#v", period)
	}
}

func energyExplanationV1ConversionFixture() EnergyExplanationV1 {
	nodes := []EnergyExplanationNode{
		{ID: "energy.carrier.electricity", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Value: 25, Unit: "kWh", Carrier: "electricity", EndUse: "total", Basis: "measured_meter", SourceIDs: []string{"facility"}},
		{ID: "energy.end_use.cooling.electricity", Level: "energy", Kind: "energy.cooling", Label: "Cooling energy", Value: 25, Unit: "kWh", Carrier: "electricity", EndUse: "cooling", Basis: "measured_meter", SourceIDs: []string{"cooling"}},
		{ID: "load.cooling.office", Level: "load", Kind: "load.zone_cooling", Label: "Cooling load", Value: 100, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", Basis: "measured_variable", SourceIDs: []string{"load"}, RelatedPathIDs: []string{"path.office.cooling"}},
		{ID: "heat.internal_convective.office", Level: "heat", Kind: "heat.internal_convective", Label: "Internal gains", Value: 100, SignedValue: 100, DisplayValue: 100, Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", HeatCategory: "internal_gains", Basis: "derived_balance", SourceIDs: []string{"driver"}, RelatedPathIDs: []string{"path.office.cooling"}},
	}
	edges := []EnergyExplanationEdge{
		{ID: "carrier-end-use", FromID: nodes[0].ID, ToID: nodes[1].ID, Value: 25, Unit: "kWh", Period: "annual", Relation: "meter_enduse", Basis: "measured_meter", RuleID: energyRelationshipRuleMeterEndUse, SourceIDs: []string{"cooling"}},
		{ID: "end-use-load", FromID: nodes[1].ID, ToID: nodes[2].ID, Value: 100, Unit: "kWh", Period: "annual", Relation: "delivered_load", Basis: "measured_variable", RuleID: energyRelationshipRuleMeasuredLoad, ServiceKind: "cooling", SourceIDs: []string{"load"}, RelatedPathIDs: []string{"path.office.cooling"}},
		{ID: "load-driver", FromID: nodes[2].ID, ToID: nodes[3].ID, Value: 100, Unit: "kWh", Period: "annual", Relation: "heat_driver", Basis: "derived_balance", RuleID: energyRelationshipRuleHeatDriverBalance, ServiceKind: "cooling", ZoneName: "Office", SourceIDs: []string{"driver"}, RelatedPathIDs: []string{"path.office.cooling"}},
	}
	return EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "monthly",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Periods: []EnergyPeriod{{
			ID:    "annual",
			Label: "Annual",
			Kind:  "annual",
			Nodes: append([]EnergyExplanationNode(nil), nodes...),
			Edges: append([]EnergyExplanationEdge(nil), edges...),
		}},
		Nodes: nodes,
		Edges: edges,
		Completeness: EnergyCompleteness{
			Status: "complete",
		},
	}
}

func energyPathV2NodeByID(nodes []EnergyExplanationNode, id string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].ID == id {
			return &nodes[index]
		}
	}
	return nil
}

func energyPathV2LinkByRelation(links []EnergyPathLink, relation string) *EnergyPathLink {
	for index := range links {
		if links[index].Relation == relation {
			return &links[index]
		}
	}
	return nil
}

func energyPathV2LinkByIDs(links []EnergyPathLink, fromID string, toID string) *EnergyPathLink {
	for index := range links {
		if links[index].FromID == fromID && links[index].ToID == toID {
			return &links[index]
		}
	}
	return nil
}
