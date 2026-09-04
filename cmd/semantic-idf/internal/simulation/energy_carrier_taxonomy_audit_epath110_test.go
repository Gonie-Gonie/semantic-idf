package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type epath110AuditCarrierCase struct {
	Carrier           string
	Label             string
	FacilityAliases   []string
	MeterTokens       []string
	Combustion        bool
	PurchasedDistrict bool
}

var epath110AuditCarrierCases = []epath110AuditCarrierCase{
	{Carrier: "electricity", Label: "Electricity", FacilityAliases: []string{"Electricity:Facility"}, MeterTokens: []string{"Electricity"}},
	{Carrier: "natural_gas", Label: "Natural gas", FacilityAliases: []string{"NaturalGas:Facility", "Gas:Facility"}, MeterTokens: []string{"NaturalGas", "Gas"}, Combustion: true},
	{Carrier: "district_cooling", Label: "District cooling", FacilityAliases: []string{"DistrictCooling:Facility"}, MeterTokens: []string{"DistrictCooling"}, PurchasedDistrict: true},
	{Carrier: "district_heating", Label: "District heating", FacilityAliases: []string{"DistrictHeatingWater:Facility", "DistrictHeating:Facility"}, MeterTokens: []string{"DistrictHeatingWater", "DistrictHeating"}, PurchasedDistrict: true},
	{Carrier: "steam", Label: "Steam", FacilityAliases: []string{"DistrictHeatingSteam:Facility", "Steam:Facility"}, MeterTokens: []string{"DistrictHeatingSteam", "Steam"}, PurchasedDistrict: true},
	{Carrier: "propane", Label: "Propane", FacilityAliases: []string{"Propane:Facility"}, MeterTokens: []string{"Propane"}, Combustion: true},
	{Carrier: "fuel_oil_1", Label: "Fuel oil #1", FacilityAliases: []string{"FuelOilNo1:Facility"}, MeterTokens: []string{"FuelOilNo1"}, Combustion: true},
	{Carrier: "fuel_oil_2", Label: "Fuel oil #2", FacilityAliases: []string{"FuelOilNo2:Facility"}, MeterTokens: []string{"FuelOilNo2"}, Combustion: true},
	{Carrier: "coal", Label: "Coal", FacilityAliases: []string{"Coal:Facility"}, MeterTokens: []string{"Coal"}, Combustion: true},
	{Carrier: "diesel", Label: "Diesel", FacilityAliases: []string{"Diesel:Facility"}, MeterTokens: []string{"Diesel"}, Combustion: true},
	{Carrier: "gasoline", Label: "Gasoline", FacilityAliases: []string{"Gasoline:Facility"}, MeterTokens: []string{"Gasoline"}, Combustion: true},
	{Carrier: "other_fuel_1", Label: "Other fuel 1", FacilityAliases: []string{"OtherFuel1:Facility"}, MeterTokens: []string{"OtherFuel1"}, Combustion: true},
	{Carrier: "other_fuel_2", Label: "Other fuel 2", FacilityAliases: []string{"OtherFuel2:Facility"}, MeterTokens: []string{"OtherFuel2"}, Combustion: true},
	{Carrier: "water", Label: "Water", FacilityAliases: []string{"Water:Facility"}, MeterTokens: []string{"Water"}},
}

type epath110AuditEndUseCase struct {
	MeterToken string
	EndUse     string
	Kind       string
}

var epath110AuditEndUseCases = []epath110AuditEndUseCase{
	{MeterToken: "Cooling", EndUse: "cooling", Kind: "energy.cooling"},
	{MeterToken: "Heating", EndUse: "heating", Kind: "energy.heating"},
	{MeterToken: "InteriorLights", EndUse: "interior_lighting", Kind: "energy.interior_lighting"},
	{MeterToken: "InteriorEquipment", EndUse: "interior_equipment", Kind: "energy.interior_equipment"},
	{MeterToken: "ExteriorLights", EndUse: "exterior_lighting", Kind: "energy.exterior_lighting"},
	{MeterToken: "ExteriorEquipment", EndUse: "exterior_equipment", Kind: "energy.exterior_equipment"},
	{MeterToken: "Fans", EndUse: "fans", Kind: "energy.fans"},
	{MeterToken: "Pumps", EndUse: "pumps", Kind: "energy.pumps"},
	{MeterToken: "HeatRejection", EndUse: "heat_rejection", Kind: "energy.heat_rejection"},
	{MeterToken: "HeatRecovery", EndUse: "heat_recovery", Kind: "energy.heat_recovery"},
	{MeterToken: "WaterSystems", EndUse: "water_systems", Kind: "energy.water_systems"},
	{MeterToken: "Refrigeration", EndUse: "refrigeration", Kind: "energy.refrigeration"},
	{MeterToken: "Humidifier", EndUse: "humidification", Kind: "energy.humidification"},
	{MeterToken: "Cogeneration", EndUse: "generators", Kind: "energy.generators"},
	{MeterToken: "Miscellaneous", EndUse: "other", Kind: "energy.other"},
}

func TestEPATH110AuditFixedCarrierTaxonomyLabelsAndEnergyUnits(t *testing.T) {
	wantTaxonomy := epath110AuditCarrierNames(epath110AuditCarrierCases)
	gotSet := map[string]bool{}
	for _, definition := range energyMeterAliasCatalog() {
		if definition.FacilityTotal {
			gotSet[definition.Carrier] = true
		}
	}
	gotTaxonomy := make([]string, 0, len(gotSet))
	for carrier := range gotSet {
		gotTaxonomy = append(gotTaxonomy, carrier)
	}
	sort.Strings(gotTaxonomy)
	sort.Strings(wantTaxonomy)
	if !reflect.DeepEqual(gotTaxonomy, wantTaxonomy) {
		t.Fatalf("facility carrier taxonomy = %#v, want the fixed EPATH-110 set %#v", gotTaxonomy, wantTaxonomy)
	}

	for _, test := range epath110AuditCarrierCases {
		t.Run(test.Carrier, func(t *testing.T) {
			if got := energyCarrierLabel(test.Carrier); got != test.Label {
				t.Errorf("canonical carrier label = %q, want %q", got, test.Label)
			}
			if got := energyPathCarrierUsesCombustionEfficiency(test.Carrier); got != test.Combustion {
				t.Errorf("combustion classification = %t, want %t", got, test.Combustion)
			}
			if got := energyPathCarrierIsPurchasedDistrictEnergy(test.Carrier); got != test.PurchasedDistrict {
				t.Errorf("purchased-district classification = %t, want %t", got, test.PurchasedDistrict)
			}
			for _, alias := range test.FacilityAliases {
				definition, ok := energyMeterAliasDefinitionForName(alias)
				wantEndUse := "total"
				if test.Carrier == "water" {
					// Water:Facility is recognized so it can be retained as context,
					// but it is deliberately not an energy total.
					wantEndUse = "water"
				}
				if !ok || !definition.FacilityTotal || definition.Carrier != test.Carrier || definition.EndUse != wantEndUse {
					t.Errorf("facility alias %q = %#v, ok=%t; want exact carrier %q", alias, definition, ok, test.Carrier)
				}
			}
		})
	}

	if _, ok := energyCarrierToken("Fuel"); ok {
		t.Fatal("generic Fuel must not be accepted as a canonical carrier")
	}
	for _, carrier := range []string{"electricity", "district_cooling", "district_heating", "steam"} {
		if energyPathCarrierUsesCombustionEfficiency(carrier) {
			t.Errorf("%q was misclassified as generic combustion fuel", carrier)
		}
	}

	unitCases := []struct {
		value float64
		unit  string
		want  float64
	}{
		{value: 3_600_000, unit: "J", want: 1},
		{value: 3_600, unit: "kJ", want: 1},
		{value: 3.6, unit: "MJ", want: 1},
		{value: 0.0036, unit: "GJ", want: 1},
		{value: 1_000, unit: "Wh", want: 1},
		{value: 1, unit: " KWH ", want: 1},
	}
	for _, test := range unitCases {
		t.Run("unit_"+strings.TrimSpace(test.unit), func(t *testing.T) {
			value, unit := convertEnergySQLValue(test.value, test.unit)
			if unit != "kWh" || math.Abs(value-test.want) > 1e-9 {
				t.Errorf("convertEnergySQLValue(%g, %q) = %g %q, want %g kWh", test.value, test.unit, value, unit, test.want)
			}
		})
	}
	if value, unit := convertEnergySQLValue(2.5, "m3"); value != 2.5 || unit != "m3" {
		t.Fatalf("water volume was silently converted to site energy: got %g %q", value, unit)
	}
	for _, unit := range []string{"kWh thermal", "kWh site"} {
		if got := canonicalEnergyPathEnergyUnit(unit); got != unit {
			t.Errorf("established domain-qualified unit %q was rewritten as %q", unit, got)
		}
	}
}

func TestEPATH110AuditStoredV1EnergyUnitsNormalizeValuesAndLabelsTogether(t *testing.T) {
	tests := []struct {
		unit  string
		value float64
	}{
		{unit: "J", value: 3_600_000},
		{unit: "MJ", value: 3.6},
		{unit: "Wh", value: 1_000},
	}

	for _, test := range tests {
		t.Run(test.unit, func(t *testing.T) {
			legacy := EnergyExplanationV1{
				Schema:           energyExplanationV1Schema,
				Purpose:          string(SimulationPurposeBasicEnergy),
				Frequency:        "annual",
				AllocationPolicy: PurposeAllocationPolicyDirectOnly,
				Nodes: []EnergyExplanationNode{
					{ID: "energy.carrier.electricity", Level: "energy", Kind: "energy.electricity.total", Label: "legacy electricity", Value: test.value, Unit: test.unit, Carrier: "electricity", EndUse: "total", MeterHierarchyLevel: "facility_total", Basis: "reported_meter", SourceIDs: []string{"meter.facility.electricity"}},
					{ID: "energy.end_use.cooling.electricity", Level: "energy", Kind: "energy.cooling", Label: "legacy cooling electricity", Value: test.value, Unit: test.unit, Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", Basis: "reported_meter", SourceIDs: []string{"meter.cooling.electricity"}},
				},
				Edges: []EnergyExplanationEdge{
					{ID: "meter-end-use", FromID: "energy.carrier.electricity", ToID: "energy.end_use.cooling.electricity", Value: test.value, Unit: test.unit, Relation: "meter_enduse", Basis: "reported_meter", SourceIDs: []string{"meter.cooling.electricity"}},
				},
				Sources: []EnergyDataSource{
					{ID: "meter.facility.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Electricity:Facility", SourceUnit: test.unit, NormalizedUnit: test.unit},
					{ID: "meter.cooling.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Cooling:Electricity", SourceUnit: test.unit, NormalizedUnit: test.unit},
				},
				scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
			}

			result := UpgradeEnergyExplanationV1(legacy)
			for _, id := range []string{"carrier.electricity.building", "end_use.cooling.building"} {
				node := epath110AuditNodeByID(result.Nodes, id)
				if node == nil {
					t.Errorf("missing upgraded node %q", id)
					continue
				}
				if node.Unit != "kWh" || math.Abs(node.Value-1) > 1e-9 || math.Abs(node.RawValue-1) > 1e-9 || math.Abs(node.EffectiveValue-1) > 1e-9 || math.Abs(node.AllocatedValue-1) > 1e-9 {
					t.Errorf("stored-v1 %g %s node %q = value/raw/effective/allocated %g/%g/%g/%g %s; want coherent 1 kWh fields", test.value, test.unit, id, node.Value, node.RawValue, node.EffectiveValue, node.AllocatedValue, node.Unit)
				}
			}
			link := epath110AuditLinkByIDs(result.Links, "end_use.cooling.building", "carrier.electricity.building")
			if link == nil || link.FromUnit != "kWh" || link.ToUnit != "kWh" || math.Abs(link.FromValue-1) > 1e-9 || math.Abs(link.ToValue-1) > 1e-9 {
				t.Errorf("stored-v1 %g %s branch was relabeled without value conversion: %#v", test.value, test.unit, link)
			}
			if legacy.Nodes[0].Value != test.value || legacy.Nodes[0].Unit != test.unit || legacy.Edges[0].Value != test.value || legacy.Edges[0].Unit != test.unit {
				t.Error("stored-v1 normalization mutated its input payload")
			}
		})
	}
}

func TestEPATH110AuditMixedNestedAndSourceUnitNormalizationIsValueAware(t *testing.T) {
	input := EnergyExplanationV1{
		Nodes: []EnergyExplanationNode{{
			ID: "load.cooling", Level: "load", Kind: "load.zone_cooling", Label: "Cooling load",
			Value: 3_600_000, RawValue: 3_600_000, EffectiveValue: 3_600_000, AllocatedValue: 3_600_000,
			Unit: "J", ServiceKind: "cooling", SourceIDs: []string{"load.source"},
			LoadBreakdown: []EnergyExplanationLoadComponent{
				{Component: "sensible", Value: 0.4, Share: 0.4, Unit: "kWh", SourceIDs: []string{"sensible.source"}},
				{Component: "latent", Value: 1.8, Share: 0.5, Unit: "MJ", SourceIDs: []string{"latent.source"}},
				{Component: "other", Value: 360_000, Share: 0.1, SourceIDs: []string{"other.source"}},
			},
		}},
		Sources: []EnergyDataSource{{
			ID: "load.source", SourceType: "sql_variable", Name: "Cooling load", SourceUnit: "J", NormalizedUnit: "J",
			RawValue: 3_600_000, EffectiveValue: 7_200_000, AllocatedValue: 1_800_000,
			EffectiveMultiplier: 2, AllocationFactor: 0.25, AllocationApplied: true,
			ScopeDetails: []EnergyDataSourceScopeDetail{{
				Scope:    EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"},
				RawValue: 720_000, EffectiveValue: 1_440_000, AllocatedValue: 360_000,
				EffectiveMultiplier: 2, AllocationFactor: 0.25, AllocationApplied: true,
			}},
		}},
		Reconciliation: []EnergyReconciliation{{
			ID: "reconcile.energy.electricity.annual", Level: "energy", Period: "annual", Label: "Electricity total basis", Unit: "J", Basis: "residual",
			ExpectedValue: 3_600_000, ExplainedValue: 7_200_000, ResidualValue: -3_600_000,
			DirectValue: 1_800_000, AllocatedValue: 900_000, UnassignedValue: 360_000, OvermappedValue: 720_000,
		}},
	}

	normalized := normalizeEnergyExplanationV1Units(input)
	node := normalized.Nodes[0]
	if node.Unit != "kWh" || math.Abs(node.Value-1) > 1e-9 || math.Abs(node.RawValue-1) > 1e-9 || math.Abs(node.EffectiveValue-1) > 1e-9 || math.Abs(node.AllocatedValue-1) > 1e-9 {
		t.Errorf("J parent node normalization = value/raw/effective/allocated %g/%g/%g/%g %s; want 1/1/1/1 kWh", node.Value, node.RawValue, node.EffectiveValue, node.AllocatedValue, node.Unit)
	}
	wantComponents := []struct {
		value float64
		share float64
	}{
		{value: 0.4, share: 0.4}, // Already kWh: never apply the parent J factor.
		{value: 0.5, share: 0.5}, // 1.8 MJ -> 0.5 kWh.
		{value: 0.1, share: 0.1}, // Missing component unit inherits parent J.
	}
	if len(node.LoadBreakdown) != len(wantComponents) {
		t.Fatalf("mixed-unit load breakdown length = %d, want %d", len(node.LoadBreakdown), len(wantComponents))
	}
	for index, want := range wantComponents {
		component := node.LoadBreakdown[index]
		if component.Unit != "kWh" || math.Abs(component.Value-want.value) > 1e-9 || math.Abs(component.Share-want.share) > 1e-9 {
			t.Errorf("mixed-unit component %d = value/share %g/%g %s; want %g/%g kWh", index, component.Value, component.Share, component.Unit, want.value, want.share)
		}
	}

	source := normalized.Sources[0]
	if source.SourceUnit != "J" || source.NormalizedUnit != "kWh" || math.Abs(source.RawValue-1) > 1e-9 || math.Abs(source.EffectiveValue-2) > 1e-9 || math.Abs(source.AllocatedValue-0.5) > 1e-9 {
		t.Errorf("J source normalization = source/normalized %q/%q, raw/effective/allocated %g/%g/%g; want J/kWh and 1/2/0.5", source.SourceUnit, source.NormalizedUnit, source.RawValue, source.EffectiveValue, source.AllocatedValue)
	}
	if source.EffectiveMultiplier != 2 || source.AllocationFactor != 0.25 || !source.AllocationApplied {
		t.Errorf("dimensionless source accounting was changed: multiplier=%g factor=%g applied=%t", source.EffectiveMultiplier, source.AllocationFactor, source.AllocationApplied)
	}
	if len(source.ScopeDetails) != 1 {
		t.Fatalf("source scope details length = %d, want 1", len(source.ScopeDetails))
	}
	detail := source.ScopeDetails[0]
	if math.Abs(detail.RawValue-0.2) > 1e-9 || math.Abs(detail.EffectiveValue-0.4) > 1e-9 || math.Abs(detail.AllocatedValue-0.1) > 1e-9 {
		t.Errorf("J source scope detail raw/effective/allocated = %g/%g/%g; want 0.2/0.4/0.1 kWh", detail.RawValue, detail.EffectiveValue, detail.AllocatedValue)
	}
	if detail.EffectiveMultiplier != 2 || detail.AllocationFactor != 0.25 || !detail.AllocationApplied {
		t.Errorf("dimensionless scope-detail accounting was changed: multiplier=%g factor=%g applied=%t", detail.EffectiveMultiplier, detail.AllocationFactor, detail.AllocationApplied)
	}
	reconciliation := normalized.Reconciliation[0]
	if reconciliation.Unit != "kWh" || math.Abs(reconciliation.ExpectedValue-1) > 1e-9 || math.Abs(reconciliation.ExplainedValue-2) > 1e-9 || math.Abs(reconciliation.ResidualValue+1) > 1e-9 || math.Abs(reconciliation.DirectValue-0.5) > 1e-9 || math.Abs(reconciliation.AllocatedValue-0.25) > 1e-9 || math.Abs(reconciliation.UnassignedValue-0.1) > 1e-9 || math.Abs(reconciliation.OvermappedValue-0.2) > 1e-9 {
		t.Errorf("J reconciliation normalization = %#v; every dimensional accounting field must use the same kWh factor", reconciliation)
	}

	// The compatibility boundary is copy-on-normalize: stored payloads and
	// nested slices remain byte-for-byte meaningful to callers that cache them.
	if input.Nodes[0].Unit != "J" || input.Nodes[0].LoadBreakdown[0].Unit != "kWh" || input.Nodes[0].LoadBreakdown[0].Value != 0.4 || input.Nodes[0].LoadBreakdown[1].Unit != "MJ" || input.Nodes[0].LoadBreakdown[1].Value != 1.8 || input.Sources[0].NormalizedUnit != "J" || input.Sources[0].RawValue != 3_600_000 || input.Sources[0].ScopeDetails[0].RawValue != 720_000 || input.Reconciliation[0].OvermappedValue != 720_000 {
		t.Error("mixed-unit normalization mutated the stored-v1 input or its nested slices")
	}
}

func TestEPATH110AuditStoredV1WaterVolumeNormalizationIsValueAware(t *testing.T) {
	tests := []struct {
		name        string
		unit        string
		sourceValue float64
	}{
		{name: "litres", unit: "L", sourceValue: 1_000},
		{name: "cubic_feet", unit: "ft3", sourceValue: 35.3146667},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := EnergyExplanationV1{
				Schema:           energyExplanationV1Schema,
				Purpose:          string(SimulationPurposeBasicEnergy),
				Frequency:        "annual",
				AllocationPolicy: PurposeAllocationPolicyDirectOnly,
				Nodes: []EnergyExplanationNode{{
					ID: "energy.carrier.water", Level: "energy", Kind: "energy.water.total", Label: "Water total",
					Value: test.sourceValue, SignedValue: test.sourceValue, RawValue: test.sourceValue,
					EffectiveValue: test.sourceValue * 2, AllocatedValue: test.sourceValue / 2, DisplayValue: test.sourceValue,
					AllocationApplied: true, Unit: test.unit, Carrier: "water", EndUse: "water",
					MeterHierarchyLevel: "facility_total", Basis: "reported_meter", SourceIDs: []string{"water.source"},
				}},
				Sources: []EnergyDataSource{{
					ID: "water.source", SourceType: "sql_meter", IsMeter: true, Name: "Water:Facility",
					Units: test.unit, SourceUnit: test.unit, NormalizedUnit: test.unit,
					RawValue: test.sourceValue, EffectiveValue: test.sourceValue * 2, AllocatedValue: test.sourceValue / 2,
					ScopeDetails: []EnergyDataSourceScopeDetail{{
						Scope:    EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"},
						RawValue: test.sourceValue, EffectiveValue: test.sourceValue * 2, AllocatedValue: test.sourceValue / 2,
					}},
				}},
				scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
			}

			normalized := normalizeEnergyExplanationV1Units(input)
			node := normalized.Nodes[0]
			if node.Unit != "m3" || math.Abs(node.Value-1) > 0.001 || math.Abs(node.SignedValue-1) > 0.001 || math.Abs(node.RawValue-1) > 0.001 || math.Abs(node.EffectiveValue-2) > 0.001 || math.Abs(node.AllocatedValue-0.5) > 0.001 || math.Abs(node.DisplayValue-1) > 0.001 {
				t.Errorf("stored-v1 %g %s water node = value/signed/raw/effective/allocated/display %g/%g/%g/%g/%g/%g %s; want 1/1/1/2/0.5/1 m3", test.sourceValue, test.unit, node.Value, node.SignedValue, node.RawValue, node.EffectiveValue, node.AllocatedValue, node.DisplayValue, node.Unit)
			}
			source := normalized.Sources[0]
			if source.Units != test.unit || source.SourceUnit != test.unit || source.NormalizedUnit != "m3" || math.Abs(source.RawValue-1) > 0.001 || math.Abs(source.EffectiveValue-2) > 0.001 || math.Abs(source.AllocatedValue-0.5) > 0.001 {
				t.Errorf("stored-v1 %s water source = %#v; original units must remain and values must normalize to m3", test.unit, source)
			}
			if len(source.ScopeDetails) != 1 || math.Abs(source.ScopeDetails[0].RawValue-1) > 0.001 || math.Abs(source.ScopeDetails[0].EffectiveValue-2) > 0.001 || math.Abs(source.ScopeDetails[0].AllocatedValue-0.5) > 0.001 {
				t.Errorf("stored-v1 %s water source scope normalization = %#v", test.unit, source.ScopeDetails)
			}

			zeroSourceInput := EnergyExplanationV1{
				Schema:           energyExplanationV1Schema,
				Purpose:          string(SimulationPurposeBasicEnergy),
				Frequency:        "annual",
				AllocationPolicy: PurposeAllocationPolicyDirectOnly,
				Nodes: []EnergyExplanationNode{{
					ID: "energy.carrier.water.zero", Level: "energy", Kind: "energy.water.total", Label: "Water total",
					Value: test.sourceValue, Unit: test.unit, Carrier: "water", EndUse: "water",
					MeterHierarchyLevel: "facility_total", Basis: "reported_meter", SourceIDs: []string{"water.zero.source"},
				}},
				Sources: []EnergyDataSource{{
					ID: "water.zero.source", SourceType: "sql_meter", IsMeter: true, Name: "Water:Facility",
					Units: test.unit, SourceUnit: test.unit, NormalizedUnit: test.unit,
				}},
				scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
			}
			result := UpgradeEnergyExplanationV1(zeroSourceInput)
			if len(result.Nodes) != 0 || len(result.Links) != 0 || len(result.Reconciliation) != 0 {
				t.Errorf("raw %s water entered the v2 energy graph: nodes=%#v links=%#v reconciliation=%#v", test.unit, result.Nodes, result.Links, result.Reconciliation)
			}
			refilled := epath110AuditSourceByID(result.Sources, "water.zero.source")
			if refilled == nil || refilled.Units != test.unit || refilled.SourceUnit != test.unit || refilled.NormalizedUnit != "m3" || math.Abs(refilled.RawValue-1) > 0.001 || math.Abs(refilled.EffectiveValue-1) > 0.001 || math.Abs(refilled.AllocatedValue-1) > 0.001 || !strings.EqualFold(refilled.InspectorSection, "context") {
				t.Errorf("zero-valued %s source was not backfilled coherently from its normalized node: %#v", test.unit, refilled)
			}

			if input.Nodes[0].Unit != test.unit || input.Nodes[0].Value != test.sourceValue || input.Sources[0].Units != test.unit || input.Sources[0].SourceUnit != test.unit || input.Sources[0].NormalizedUnit != test.unit || input.Sources[0].RawValue != test.sourceValue || input.Sources[0].ScopeDetails[0].RawValue != test.sourceValue || zeroSourceInput.Nodes[0].Unit != test.unit || zeroSourceInput.Sources[0].NormalizedUnit != test.unit || zeroSourceInput.Sources[0].RawValue != 0 {
				t.Error("water volume normalization mutated a stored-v1 input or nested source details")
			}
		})
	}
}

func TestEPATH110AuditRecognizedEnergyCarrierRejectsVolumeUnits(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.electricity.invalid", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Value: 7, Unit: "m3", Carrier: "electricity", EndUse: "total", MeterHierarchyLevel: "facility_total", Basis: "reported_meter", SourceIDs: []string{"invalid.facility.electricity"}},
			{ID: "energy.end_use.cooling.electricity.invalid", Level: "energy", Kind: "energy.cooling", Label: "Cooling electricity", Value: 7, Unit: "m3", Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", Basis: "reported_meter", SourceIDs: []string{"invalid.cooling.electricity"}},
		},
		Edges: []EnergyExplanationEdge{
			{ID: "invalid-electricity-volume", FromID: "energy.carrier.electricity.invalid", ToID: "energy.end_use.cooling.electricity.invalid", Value: 7, Unit: "m3", Relation: "meter_enduse", Basis: "reported_meter", SourceIDs: []string{"invalid.cooling.electricity"}},
		},
		Reconciliation: []EnergyReconciliation{
			{ID: "reconcile.energy.electricity.invalid.annual", Level: "energy", Period: "annual", Label: "Electricity total basis", Status: "balanced", ExpectedValue: 7, ExplainedValue: 7, Unit: "m3", Basis: "residual", SourceIDs: []string{"invalid.facility.electricity", "invalid.cooling.electricity"}},
		},
		Sources: []EnergyDataSource{
			{ID: "invalid.facility.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Electricity:Facility", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"},
			{ID: "invalid.cooling.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Cooling:Electricity", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"},
		},
		Completeness: EnergyCompleteness{
			Status: "complete", MappedPercent: 100,
			EnergyUse: EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 2, Total: 2},
		},
		scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
	}

	result := UpgradeEnergyExplanationV1(legacy)
	if len(result.Nodes) != 0 || len(result.Links) != 0 || len(result.Reconciliation) != 0 {
		t.Errorf("recognized Electricity expressed in m3 survived V1->V2: nodes=%#v links=%#v reconciliation=%#v", result.Nodes, result.Links, result.Reconciliation)
	}
	if result.Completeness.MappedPercent != 0 {
		t.Errorf("invalid m3 Electricity retained mappedPercent %g, want 0", result.Completeness.MappedPercent)
	}
	for _, sourceID := range []string{"invalid.facility.electricity", "invalid.cooling.electricity"} {
		source := epath110AuditSourceByID(result.Sources, sourceID)
		if source == nil {
			t.Errorf("invalid-unit source %q was dropped instead of retained for audit", sourceID)
			continue
		}
		if source.Units != "m3" || source.SourceUnit != "m3" || source.NormalizedUnit != "m3" {
			t.Errorf("invalid-unit source %q was silently relabeled as energy: %#v", sourceID, source)
		}
	}
	if legacy.Nodes[0].Value != 7 || legacy.Nodes[0].Unit != "m3" || legacy.Edges[0].Value != 7 || legacy.Edges[0].Unit != "m3" || legacy.Reconciliation[0].ExpectedValue != 7 || legacy.Sources[0].NormalizedUnit != "m3" || legacy.Completeness.MappedPercent != 100 {
		t.Error("invalid-unit V1->V2 rejection mutated the stored-v1 payload")
	}
}

func TestEPATH110AuditEveryEndUseCarrierAliasOrderMapsExactly(t *testing.T) {
	for _, carrier := range epath110AuditCarrierCases {
		for _, meterToken := range carrier.MeterTokens {
			for _, endUse := range epath110AuditEndUseCases {
				names := []string{
					endUse.MeterToken + ":" + meterToken,
					meterToken + ":" + endUse.MeterToken,
					"  " + strings.ToLower(endUse.MeterToken) + " : " + strings.ToUpper(meterToken) + "  ",
				}
				for _, name := range names {
					t.Run(carrier.Carrier+"/"+endUse.EndUse+"/"+metricID(name), func(t *testing.T) {
						definition, ok := energyMeterAliasOrOtherDefinitionForName(name)
						if !ok {
							t.Fatalf("recognized carrier/end-use meter %q was not mapped", name)
						}
						if definition.Carrier != carrier.Carrier || definition.EndUse != endUse.EndUse || definition.Kind != endUse.Kind || definition.HierarchyLevel != "broad_end_use" {
							t.Errorf("meter %q = carrier %q, end use %q, kind %q, hierarchy %q; want %q/%q/%q/broad_end_use", name, definition.Carrier, definition.EndUse, definition.Kind, definition.HierarchyLevel, carrier.Carrier, endUse.EndUse, endUse.Kind)
						}
						if definition.Carrier == "fuel" || definition.Carrier == "other" {
							t.Errorf("recognized meter %q degraded to generic carrier %q", name, definition.Carrier)
						}
					})
				}
			}
		}
	}

	left, leftOK := energyMeterAliasOrOtherDefinitionForName("Cooling:Electricity")
	right, rightOK := energyMeterAliasOrOtherDefinitionForName("Electricity:Cooling")
	if !leftOK || !rightOK || left.Carrier != "electricity" || left.EndUse != "cooling" || left.Kind != "energy.cooling" ||
		right.Carrier != left.Carrier || right.EndUse != left.EndUse || right.Kind != left.Kind || right.HierarchyLevel != left.HierarchyLevel {
		t.Fatalf("Cooling:Electricity and Electricity:Cooling are not the same semantic relation: left=%#v/%t right=%#v/%t", left, leftOK, right, rightOK)
	}
}

func TestEPATH110AuditRecognizedCarrierEndUsesReachOnlyTheirExactCarrier(t *testing.T) {
	series := make([]energyExplanationSeries, 0, (len(epath110AuditCarrierCases)-1)*2)
	sources := make([]EnergyDataSource, 0, (len(epath110AuditCarrierCases)-1)*2)
	wantValues := map[string]float64{}
	wantEndUseSources := map[string]string{}
	value := 1.0
	for _, carrier := range epath110AuditCarrierCases {
		if carrier.Carrier == "water" {
			continue
		}
		facilitySource := "meter.facility." + carrier.Carrier
		endUseSource := "meter.heating." + carrier.Carrier
		series = append(series,
			energyExplanationSeries{
				Stage: "carrier", CanonicalKind: "energy." + carrier.Carrier + ".total", Level: "energy", Kind: "energy." + carrier.Carrier + ".total",
				Label: "legacy " + strings.ToUpper(carrier.Carrier) + " total", Unit: "kWh", Carrier: carrier.Carrier, EndUse: "total",
				MeterHierarchyLevel: "facility_total", Basis: "reported_meter", SourceIDs: []string{facilitySource}, Total: value,
			},
			energyExplanationSeries{
				Stage: "end_use", CanonicalKind: "energy.heating", Level: "energy", Kind: "energy.heating",
				Label: "legacy " + carrier.Carrier + " heating", Unit: "kWh", Carrier: carrier.Carrier, EndUse: "heating",
				MeterHierarchyLevel: "broad_end_use", Basis: "reported_meter", SourceIDs: []string{endUseSource}, Total: value,
			},
		)
		sources = append(sources,
			EnergyDataSource{ID: facilitySource, SourceType: "sql_meter", IsMeter: true, Name: carrier.FacilityAliases[0], SourceUnit: "J", NormalizedUnit: "kWh"},
			EnergyDataSource{ID: endUseSource, SourceType: "sql_meter", IsMeter: true, Name: "Heating:" + carrier.MeterTokens[0], SourceUnit: "J", NormalizedUnit: "kWh"},
		)
		wantValues[carrier.Carrier] = value
		wantEndUseSources[carrier.Carrier] = endUseSource
		value++
	}
	wantHeating := 0.0
	for _, branchValue := range wantValues {
		wantHeating += branchValue
	}
	series = append(series, energyExplanationSeries{
		Stage: "load", CanonicalKind: "load.zone_heating", Level: "load", Kind: "load.zone_heating",
		Label: "Heating load", Unit: "kWh", ServiceKind: "heating", PathType: "zone", ZoneName: "Office",
		Basis: "reported_variable", SourceIDs: []string{"variable.heating.load"}, Total: wantHeating,
	})
	sources = append(sources, EnergyDataSource{ID: "variable.heating.load", SourceType: "sql_variable", Name: "Zone Air System Sensible Heating Energy", SourceUnit: "J", NormalizedUnit: "kWh", ZoneName: "Office"})

	result := epath110AuditResultFromSeries(series, sources)
	heating := epath110AuditNodeByID(result.Nodes, "end_use.heating.building")
	if heating == nil {
		t.Fatalf("missing carrier-neutral Heating node: %#v", result.Nodes)
	}
	if heating.Value != wantHeating || heating.Carrier != "" || heating.Unit != "kWh" || heating.ScaleDomain != "site" {
		t.Errorf("carrier-neutral Heating = %#v, want total %g kWh on site scale", heating, wantHeating)
	}

	carrierCount := 0
	for _, node := range result.Nodes {
		if node.Level == "carrier" {
			carrierCount++
			if node.Carrier == "fuel" {
				t.Errorf("generic fuel carrier node leaked into v2 graph: %#v", node)
			}
		}
	}
	if carrierCount != len(wantValues) {
		t.Errorf("carrier node count = %d, want %d exact energy carriers", carrierCount, len(wantValues))
	}

	for _, carrier := range epath110AuditCarrierCases {
		branchValue, wanted := wantValues[carrier.Carrier]
		if !wanted {
			continue
		}
		carrierID := "carrier." + carrier.Carrier + ".building"
		node := epath110AuditNodeByID(result.Nodes, carrierID)
		if node == nil {
			t.Errorf("missing exact carrier node %q", carrierID)
			continue
		}
		if node.Carrier != carrier.Carrier || node.Label != carrier.Label || node.Value != branchValue || node.Unit != "kWh" || node.ScaleDomain != "site" {
			t.Errorf("carrier node %q = %#v; want %q, %g kWh, site", carrierID, node, carrier.Label, branchValue)
		}
		link := epath110AuditLinkByIDs(result.Links, heating.ID, carrierID)
		if link == nil || link.Relation != "end_use_to_carrier" || link.FromValue != branchValue || link.ToValue != branchValue || link.FromUnit != "kWh" || link.ToUnit != "kWh" {
			t.Errorf("exact Heating -> %s branch = %#v", carrier.Carrier, link)
			continue
		}
		if !reflect.DeepEqual(link.SourceIDs, []string{wantEndUseSources[carrier.Carrier]}) {
			t.Errorf("%s branch source IDs = %#v, want only %#v", carrier.Carrier, link.SourceIDs, []string{wantEndUseSources[carrier.Carrier]})
		}
		if epath110AuditLinkByIDs(result.Links, carrierID, heating.ID) != nil {
			t.Errorf("reverse carrier -> end-use link leaked for %s", carrier.Carrier)
		}
	}
}

func TestEPATH110AuditEndUseWithoutFacilityTotalStillGetsExactCarrierBranch(t *testing.T) {
	result := epath110AuditResultFromSeries([]energyExplanationSeries{
		{
			Stage: "end_use", CanonicalKind: "energy.cooling", Level: "energy", Kind: "energy.cooling", Label: "Cooling electricity", Unit: "kWh",
			Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", Basis: "reported_meter", SourceIDs: []string{"meter.cooling.electricity"}, Total: 7,
		},
	}, []EnergyDataSource{
		{ID: "meter.cooling.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Cooling:Electricity", SourceUnit: "J", NormalizedUnit: "kWh"},
	})

	endUse := epath110AuditNodeByID(result.Nodes, "end_use.cooling.building")
	carrier := epath110AuditNodeByID(result.Nodes, "carrier.electricity.building")
	link := epath110AuditLinkByIDs(result.Links, "end_use.cooling.building", "carrier.electricity.building")
	if endUse == nil || endUse.Value != 7 || endUse.Unit != "kWh" {
		t.Errorf("end use without facility meter = %#v", endUse)
	}
	if carrier == nil || carrier.Carrier != "electricity" || carrier.Label != "Electricity" || carrier.Value != 7 || carrier.Unit != "kWh" || carrier.ScaleDomain != "site" {
		t.Errorf("observed exact-carrier subtotal = %#v", carrier)
	}
	if link == nil || (link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier") || link.FromValue != 7 || link.ToValue != 7 || link.FromUnit != "kWh" || link.ToUnit != "kWh" || !reflect.DeepEqual(link.SourceIDs, []string{"meter.cooling.electricity"}) {
		t.Errorf("end use without facility total did not retain its exact electricity branch: %#v", link)
	}
	for _, item := range result.Reconciliation {
		if strings.Contains(item.ID, ".electricity.") && item.Status == "balanced" && item.Basis == "residual" {
			t.Errorf("observed end-use subtotal was falsely presented as a balanced facility total: %#v", item)
		}
	}
}

func TestEPATH110AuditRejectsCrossWiredLegacyCarrierEdge(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.electricity", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Value: 9, Unit: "kWh", Carrier: "electricity", EndUse: "total", MeterHierarchyLevel: "facility_total", SourceIDs: []string{"meter.facility.electricity"}},
			{ID: "energy.carrier.natural_gas", Level: "energy", Kind: "energy.natural_gas.total", Label: "Natural gas", Value: 9, Unit: "kWh", Carrier: "natural_gas", EndUse: "total", MeterHierarchyLevel: "facility_total", SourceIDs: []string{"meter.facility.natural_gas"}},
			{ID: "energy.end_use.cooling.electricity", Level: "energy", Kind: "energy.cooling", Label: "Cooling electricity", Value: 9, Unit: "kWh", Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", SourceIDs: []string{"meter.cooling.electricity"}},
		},
		Edges: []EnergyExplanationEdge{
			// Deliberately malformed stored v1 data: edge endpoint says gas while
			// the meter-derived end-use node says electricity.
			{ID: "cross-wired", FromID: "energy.carrier.natural_gas", ToID: "energy.end_use.cooling.electricity", Value: 9, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.cooling.electricity"}},
		},
		Sources: []EnergyDataSource{
			{ID: "meter.facility.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Electricity:Facility", SourceUnit: "J", NormalizedUnit: "kWh"},
			{ID: "meter.facility.natural_gas", SourceType: "sql_meter", IsMeter: true, Name: "NaturalGas:Facility", SourceUnit: "J", NormalizedUnit: "kWh"},
			{ID: "meter.cooling.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Cooling:Electricity", SourceUnit: "J", NormalizedUnit: "kWh"},
		},
		scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
	}
	result := UpgradeEnergyExplanationV1(legacy)

	wrong := epath110AuditLinkByIDs(result.Links, "end_use.cooling.building", "carrier.natural_gas.building")
	if wrong != nil {
		t.Errorf("cross-wired legacy edge overrode exact meter carrier evidence: %#v", wrong)
	}
	exact := epath110AuditLinkByIDs(result.Links, "end_use.cooling.building", "carrier.electricity.building")
	if exact == nil || exact.FromValue != 9 || exact.ToValue != 9 || !reflect.DeepEqual(exact.SourceIDs, []string{"meter.cooling.electricity"}) {
		t.Errorf("exact electricity branch was not recovered from end-use carrier evidence: %#v", exact)
	}
}

func TestEPATH110AuditWaterSystemsEnergyRemainsDistinctFromWaterVolume(t *testing.T) {
	series := []energyExplanationSeries{
		{Stage: "carrier", CanonicalKind: "energy.electricity.total", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Unit: "kWh", Carrier: "electricity", EndUse: "total", MeterHierarchyLevel: "facility_total", SourceIDs: []string{"facility.electricity"}, Total: 4},
		{Stage: "carrier", CanonicalKind: "energy.natural_gas.total", Level: "energy", Kind: "energy.natural_gas.total", Label: "Natural gas", Unit: "kWh", Carrier: "natural_gas", EndUse: "total", MeterHierarchyLevel: "facility_total", SourceIDs: []string{"facility.gas"}, Total: 6},
		{Stage: "end_use", CanonicalKind: "energy.water_systems", Level: "energy", Kind: "energy.water_systems", Label: "Electric water systems", Unit: "kWh", Carrier: "electricity", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", SourceIDs: []string{"water_systems.electricity"}, Total: 4},
		{Stage: "end_use", CanonicalKind: "energy.water_systems", Level: "energy", Kind: "energy.water_systems", Label: "Gas water systems", Unit: "kWh", Carrier: "natural_gas", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", SourceIDs: []string{"water_systems.gas"}, Total: 6},
	}
	result := epath110AuditResultFromSeries(series, []EnergyDataSource{
		{ID: "facility.electricity"}, {ID: "facility.gas"}, {ID: "water_systems.electricity"}, {ID: "water_systems.gas"},
	})
	endUse := epath110AuditNodeByID(result.Nodes, "end_use.water_systems.building")
	if endUse == nil || endUse.Value != 10 || endUse.Unit != "kWh" {
		t.Fatalf("water-systems energy was confused with water volume: %#v", endUse)
	}
	for carrier, value := range map[string]float64{"electricity": 4, "natural_gas": 6} {
		link := epath110AuditLinkByIDs(result.Links, endUse.ID, "carrier."+carrier+".building")
		if link == nil || link.FromValue != value || link.ToValue != value || link.FromUnit != "kWh" || link.ToUnit != "kWh" {
			t.Errorf("water-systems %s energy branch = %#v", carrier, link)
		}
	}
	if epath110AuditNodeByID(result.Nodes, "carrier.water.building") != nil {
		t.Error("water-systems energy fabricated a Water volume carrier")
	}
}

func TestEPATH110AuditWaterVolumeIsContextNotSiteEnergy(t *testing.T) {
	series := []energyExplanationSeries{
		{Stage: "carrier", CanonicalKind: "energy.electricity.total", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Unit: "kWh", Carrier: "electricity", EndUse: "total", MeterHierarchyLevel: "facility_total", Basis: "reported_meter", SourceIDs: []string{"meter.facility.electricity"}, Total: 10},
		{Stage: "end_use", CanonicalKind: "energy.cooling", Level: "energy", Kind: "energy.cooling", Label: "Cooling", Unit: "kWh", Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", Basis: "reported_meter", SourceIDs: []string{"meter.cooling.electricity"}, Total: 10},
		{Stage: "carrier", CanonicalKind: "energy.water.total", Level: "energy", Kind: "energy.water.total", Label: "Water total", Unit: "m3", Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: "reported_meter", SourceIDs: []string{"meter.facility.water"}, Total: 250},
		{Stage: "end_use", CanonicalKind: "energy.water_systems", Level: "energy", Kind: "energy.water_systems", Label: "Water systems water", Unit: "m3", Carrier: "water", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", Basis: "reported_meter", SourceIDs: []string{"meter.water_systems.water"}, Total: 250},
	}
	sources := []EnergyDataSource{
		{ID: "meter.facility.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Electricity:Facility", SourceUnit: "J", NormalizedUnit: "kWh"},
		{ID: "meter.cooling.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Cooling:Electricity", SourceUnit: "J", NormalizedUnit: "kWh"},
		{ID: "meter.facility.water", SourceType: "sql_meter", IsMeter: true, Name: "Water:Facility", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"},
		{ID: "meter.water_systems.water", SourceType: "sql_meter", IsMeter: true, Name: "WaterSystems:Water", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"},
	}

	result := epath110AuditResultFromSeries(series, sources)
	for _, node := range result.Nodes {
		if node.Carrier == "water" || strings.Contains(node.ID, ".water") || epath110AuditContainsAny(node.SourceIDs, "meter.facility.water", "meter.water_systems.water") {
			t.Errorf("unconverted water volume entered the site-energy node graph: %#v", node)
		}
	}
	for _, link := range result.Links {
		if strings.Contains(link.FromID, ".water") || strings.Contains(link.ToID, ".water") || epath110AuditContainsAny(link.SourceIDs, "meter.facility.water", "meter.water_systems.water") {
			t.Errorf("unconverted water volume entered a site-energy link: %#v", link)
		}
	}
	for _, item := range result.Reconciliation {
		if strings.Contains(item.ID, ".water.") || epath110AuditContainsAny(item.SourceIDs, "meter.facility.water", "meter.water_systems.water") {
			t.Errorf("unconverted water volume entered site-energy reconciliation: %#v", item)
		}
	}

	summary := buildEnergyExplanationSummary(result)
	carrierTotal := 0.0
	for _, item := range summary.Carriers {
		if item.Carrier == "water" || strings.Contains(item.ID, ".water") {
			t.Errorf("unconverted water volume entered carrier summary: %#v", item)
		}
		carrierTotal += item.Value
	}
	if carrierTotal != 10 {
		t.Errorf("site-energy carrier total = %g, want electricity-only 10 kWh", carrierTotal)
	}
	for _, sourceID := range []string{"meter.facility.water", "meter.water_systems.water"} {
		source := epath110AuditSourceByID(result.Sources, sourceID)
		if source == nil {
			t.Errorf("water context source %q was dropped", sourceID)
			continue
		}
		if source.SourceUnit != "m3" || source.NormalizedUnit != "m3" || !strings.EqualFold(source.InspectorSection, "context") {
			t.Errorf("water context source %q = %#v; want preserved m3 source marked context", sourceID, source)
		}
	}
}

func TestEPATH110AuditWaterContextDoesNotDiluteEnergyCompleteness(t *testing.T) {
	electricityNodes := []EnergyExplanationNode{
		{ID: "energy.carrier.electricity", Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "total", MeterHierarchyLevel: "facility_total", Basis: "reported_meter", SourceIDs: []string{"meter.facility.electricity"}},
		{ID: "energy.end_use.cooling.electricity", Level: "energy", Kind: "energy.cooling", Label: "Cooling electricity", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", Basis: "reported_meter", SourceIDs: []string{"meter.cooling.electricity"}},
	}
	electricityEdges := []EnergyExplanationEdge{
		{ID: "electricity-end-use", FromID: "energy.carrier.electricity", ToID: "energy.end_use.cooling.electricity", Value: 10, Unit: "kWh", Relation: "meter_enduse", Basis: "reported_meter", SourceIDs: []string{"meter.cooling.electricity"}},
	}
	electricityReconciliation := []EnergyReconciliation{
		{ID: "reconcile.energy.electricity.annual", Level: "energy", Period: "annual", Label: "Electricity total basis", Status: "balanced", ExpectedValue: 10, ExplainedValue: 10, Unit: "kWh", Basis: "residual", SourceIDs: []string{"meter.facility.electricity", "meter.cooling.electricity"}},
	}
	electricitySources := []EnergyDataSource{
		{ID: "meter.facility.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Electricity:Facility", SourceUnit: "J", NormalizedUnit: "kWh"},
		{ID: "meter.cooling.electricity", SourceType: "sql_meter", IsMeter: true, Name: "Cooling:Electricity", SourceUnit: "J", NormalizedUnit: "kWh"},
	}
	electricityAvailability := []EnergySourceAvailabilityEntry{
		{Name: "Electricity:Facility", Level: "energy", Status: "found", SourceIDs: []string{"meter.facility.electricity"}},
		{Name: "Cooling:Electricity", Level: "energy", Status: "found", SourceIDs: []string{"meter.cooling.electricity"}},
	}
	electricityCompleteness := EnergyCompleteness{
		Status:        "complete",
		MappedPercent: 100,
		EnergyUse:     EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 2, Total: 2},
		Items:         []EnergyCompletenessLevel{{Level: "energy", Status: "complete", Found: 2, Total: 2}},
		SourceAvailability: append([]EnergySourceAvailabilityEntry(nil),
			electricityAvailability...),
	}
	baseline := UpgradeEnergyExplanationV1(EnergyExplanationV1{
		Schema: energyExplanationV1Schema, Purpose: string(SimulationPurposeBasicEnergy), Frequency: "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly, Nodes: electricityNodes, Edges: electricityEdges,
		Reconciliation: electricityReconciliation, Sources: electricitySources, Completeness: electricityCompleteness,
		scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
	})

	withWaterCompleteness := electricityCompleteness
	withWaterCompleteness.MappedPercent = roundedEnergyNumber(10.0 / 260.0 * 100)
	withWaterCompleteness.EnergyUse = EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 3, Total: 3}
	withWaterCompleteness.Items = []EnergyCompletenessLevel{{Level: "energy", Status: "complete", Found: 3, Total: 3}}
	withWaterCompleteness.SourceAvailability = append(append([]EnergySourceAvailabilityEntry(nil), electricityAvailability...),
		EnergySourceAvailabilityEntry{Name: "Water:Facility", Level: "energy", Status: "found", SourceIDs: []string{"meter.facility.water"}})
	withWater := EnergyExplanationV1{
		Schema: energyExplanationV1Schema, Purpose: string(SimulationPurposeBasicEnergy), Frequency: "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes: append(append([]EnergyExplanationNode(nil), electricityNodes...),
			EnergyExplanationNode{ID: "energy.carrier.water", Level: "energy", Kind: "energy.water.total", Label: "Water total", Value: 250, Unit: "m3", Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: "reported_meter", SourceIDs: []string{"meter.facility.water"}}),
		Edges: append([]EnergyExplanationEdge(nil), electricityEdges...),
		Reconciliation: append(append([]EnergyReconciliation(nil), electricityReconciliation...),
			EnergyReconciliation{ID: "reconcile.energy.water.annual", Level: "energy", Period: "annual", Label: "Water total basis", Status: "residual", ExpectedValue: 250, ExplainedValue: 0, ResidualValue: 250, Unit: "m3", Basis: "residual", SourceIDs: []string{"meter.facility.water"}}),
		Sources: append(append([]EnergyDataSource(nil), electricitySources...),
			EnergyDataSource{ID: "meter.facility.water", SourceType: "sql_meter", IsMeter: true, Name: "Water:Facility", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"}),
		Completeness: withWaterCompleteness,
		scope:        EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
	}
	result := UpgradeEnergyExplanationV1(withWater)

	if result.Completeness.Status != baseline.Completeness.Status || result.Completeness.Status != "complete" || result.Completeness.MappedPercent != baseline.Completeness.MappedPercent || result.Completeness.MappedPercent != 100 {
		t.Errorf("raw water changed complete electricity accounting: baseline=%#v with-water=%#v", baseline.Completeness, result.Completeness)
	}
	if result.Completeness.EnergyUse.Status != baseline.Completeness.EnergyUse.Status || result.Completeness.EnergyUse.Found != baseline.Completeness.EnergyUse.Found || result.Completeness.EnergyUse.Total != baseline.Completeness.EnergyUse.Total {
		t.Errorf("raw water changed electricity-only EnergyUse completeness: baseline=%#v with-water=%#v", baseline.Completeness.EnergyUse, result.Completeness.EnergyUse)
	}
	for _, item := range result.Completeness.Items {
		if strings.EqualFold(item.Level, "energy") && (item.Found != baseline.Completeness.EnergyUse.Found || item.Total != baseline.Completeness.EnergyUse.Total || item.Status != baseline.Completeness.EnergyUse.Status) {
			t.Errorf("raw water remained in energy completeness item: %#v", item)
		}
	}
	availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Water:Facility")
	if availability == nil || availability.Level != "context" || availability.Status != "found" || !reflect.DeepEqual(availability.SourceIDs, []string{"meter.facility.water"}) {
		t.Errorf("Water:Facility context availability = %#v; want found context with exact source", availability)
	}
	for _, missing := range result.Completeness.MissingCategories {
		if strings.Contains(strings.ToLower(missing), "water") {
			t.Errorf("available water context was treated as missing energy: %q", missing)
		}
	}
	waterSource := epath110AuditSourceByID(result.Sources, "meter.facility.water")
	if waterSource == nil || !strings.EqualFold(waterSource.InspectorSection, "context") || waterSource.SourceUnit != "m3" || waterSource.NormalizedUnit != "m3" {
		t.Errorf("raw water context source = %#v", waterSource)
	}
	if withWater.Completeness.MappedPercent == 100 || withWater.Completeness.EnergyUse.Found != 3 || withWater.Completeness.SourceAvailability[2].Level != "energy" {
		t.Error("v2 completeness adaptation mutated the stored-v1 input")
	}
}

func TestEPATH110AuditRawWaterOnlyClearsStaleMappedPercent(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.water", Level: "energy", Kind: "energy.water.total", Label: "Water total", Value: 250, Unit: "m3", Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: "reported_meter", SourceIDs: []string{"meter.facility.water"}},
			{ID: "energy.end_use.water", Level: "energy", Kind: "energy.water_systems", Label: "Water systems", Value: 250, Unit: "m3", Carrier: "water", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", Basis: "reported_meter", SourceIDs: []string{"meter.water_systems.water"}},
		},
		Edges: []EnergyExplanationEdge{
			{ID: "raw-water-end-use", FromID: "energy.carrier.water", ToID: "energy.end_use.water", Value: 250, Unit: "m3", Relation: "meter_enduse", Basis: "reported_meter", SourceIDs: []string{"meter.water_systems.water"}},
		},
		Reconciliation: []EnergyReconciliation{
			{ID: "reconcile.energy.water.annual", Level: "energy", Period: "annual", Label: "Water total basis", Status: "balanced", ExpectedValue: 250, ExplainedValue: 250, Unit: "m3", Basis: "residual", SourceIDs: []string{"meter.facility.water", "meter.water_systems.water"}},
		},
		Sources: []EnergyDataSource{
			{ID: "meter.facility.water", SourceType: "sql_meter", IsMeter: true, Name: "Water:Facility", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"},
			{ID: "meter.water_systems.water", SourceType: "sql_meter", IsMeter: true, Name: "WaterSystems:Water", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"},
		},
		Completeness: EnergyCompleteness{
			Status:        "complete",
			MappedPercent: 100, // Deliberately stale v1 accounting that treated m3 as energy.
			EnergyUse:     EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 2, Total: 2},
			Items:         []EnergyCompletenessLevel{{Level: "energy", Status: "complete", Found: 2, Total: 2}},
			SourceAvailability: []EnergySourceAvailabilityEntry{
				{Name: "Water:Facility", Level: "energy", Status: "found", SourceIDs: []string{"meter.facility.water"}},
				{Name: "WaterSystems:Water", Level: "energy", Status: "found", SourceIDs: []string{"meter.water_systems.water"}},
			},
		},
		scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
	}

	result := UpgradeEnergyExplanationV1(legacy)
	if result.Completeness.MappedPercent != 0 {
		t.Errorf("raw-water-only stale mappedPercent survived upgrade: got %g, want 0", result.Completeness.MappedPercent)
	}
	if len(result.Nodes) != 0 || len(result.Links) != 0 || len(result.Reconciliation) != 0 {
		t.Errorf("raw-water-only payload entered v2 energy accounting: nodes=%#v links=%#v reconciliation=%#v", result.Nodes, result.Links, result.Reconciliation)
	}
	for _, name := range []string{"Water:Facility", "WaterSystems:Water"} {
		availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, name)
		if availability == nil || availability.Level != "context" || availability.Status != "found" {
			t.Errorf("raw-water-only availability %q = %#v; want found context", name, availability)
		}
	}
	if legacy.Completeness.MappedPercent != 100 || legacy.Completeness.SourceAvailability[0].Level != "energy" {
		t.Error("raw-water-only completeness adaptation mutated the stored-v1 input")
	}
}

func TestEPATH110AuditConvertedWaterRemainsInEnergyCompleteness(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.water.converted", Level: "energy", Kind: "energy.water.total", Label: "Converted water total", Value: 25, Unit: "kWh", Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: "derived_ratio", SourceIDs: []string{"derived.water.facility"}},
			{ID: "energy.end_use.water.converted", Level: "energy", Kind: "energy.water_systems", Label: "Converted water systems", Value: 25, Unit: "kWh", Carrier: "water", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", Basis: "derived_ratio", SourceIDs: []string{"derived.water.end_use"}},
		},
		Edges: []EnergyExplanationEdge{
			{ID: "converted-water-end-use", FromID: "energy.carrier.water.converted", ToID: "energy.end_use.water.converted", Value: 25, Unit: "kWh", Relation: "meter_enduse", Basis: "derived_ratio", SourceIDs: []string{"derived.water.end_use"}},
		},
		Reconciliation: []EnergyReconciliation{
			{ID: "reconcile.energy.water.converted.annual", Level: "energy", Period: "annual", Label: "Converted water energy total basis", Status: "balanced", ExpectedValue: 25, ExplainedValue: 25, Unit: "kWh", Basis: "derived_ratio", SourceIDs: []string{"derived.water.facility", "derived.water.end_use"}},
		},
		Sources: []EnergyDataSource{
			{ID: "derived.water.facility", SourceType: "derived_formula", IsMeter: true, Name: "Water:Facility", Units: "m3", SourceUnit: "m3", NormalizedUnit: "kWh", Formula: "250 m3 * explicit 0.1 kWh/m3 site-energy conversion"},
			{ID: "derived.water.end_use", SourceType: "derived_formula", IsMeter: true, Name: "WaterSystems:Water", Units: "m3", SourceUnit: "m3", NormalizedUnit: "kWh", Formula: "250 m3 * explicit 0.1 kWh/m3 site-energy conversion"},
		},
		Completeness: EnergyCompleteness{
			Status:        "complete",
			MappedPercent: 0, // Deliberately stale: canonical converted reconciliation is authoritative.
			EnergyUse:     EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 2, Total: 2},
			Items:         []EnergyCompletenessLevel{{Level: "energy", Status: "complete", Found: 2, Total: 2}},
			SourceAvailability: []EnergySourceAvailabilityEntry{
				{Name: "Water:Facility", Level: "energy", Status: "found", SourceIDs: []string{"derived.water.facility"}},
				{Name: "WaterSystems:Water", Level: "energy", Status: "found", SourceIDs: []string{"derived.water.end_use"}},
			},
		},
		scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
	}

	result := UpgradeEnergyExplanationV1(legacy)
	if result.Completeness.Status != "complete" || result.Completeness.MappedPercent != 100 || result.Completeness.EnergyUse.Status != "complete" || result.Completeness.EnergyUse.Found != 2 || result.Completeness.EnergyUse.Total != 2 {
		t.Errorf("explicitly converted water was removed from energy completeness: %#v", result.Completeness)
	}
	for _, name := range []string{"Water:Facility", "WaterSystems:Water"} {
		availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, name)
		if availability == nil || availability.Level != "energy" || availability.Status != "found" {
			t.Errorf("converted-water availability %q = %#v; want found energy", name, availability)
		}
	}
	carrier := epath110AuditNodeByID(result.Nodes, "carrier.water.building")
	link := epath110AuditLinkByIDs(result.Links, "end_use.water_systems.building", "carrier.water.building")
	if carrier == nil || carrier.Value != 25 || carrier.Unit != "kWh" || link == nil || link.ToValue != 25 || link.ToUnit != "kWh" {
		t.Errorf("converted-water energy path was lost while adapting completeness: carrier=%#v link=%#v", carrier, link)
	}
	if legacy.Completeness.MappedPercent != 0 || legacy.Completeness.EnergyUse.Found != 2 || legacy.Completeness.SourceAvailability[0].Level != "energy" {
		t.Error("converted-water completeness adaptation mutated the stored-v1 input")
	}
}

func TestEPATH110AuditStoredV2JSONSanitizesRawWaterEverywhere(t *testing.T) {
	stored := []byte(`{
  "energyExplanation": {
    "schema": "semantic-idf.energy-explanation/v2",
    "purpose": "basic_energy",
    "scope": { "kind": "building", "aggregationBasis": "model_total" },
    "frequency": "annual",
    "nodes": [
      { "id": "carrier.electricity.building", "level": "carrier", "kind": "carrier.electricity", "label": "Electricity", "value": 10, "unit": "kWh", "scaleDomain": "site", "carrier": "electricity", "sourceIds": ["meter.facility.electricity"] },
      { "id": "end_use.cooling.building", "level": "end_use", "kind": "energy.cooling", "label": "Cooling", "value": 10, "unit": "kWh", "scaleDomain": "site", "endUse": "cooling", "sourceIds": ["meter.cooling.electricity"] },
      { "id": "carrier.water.building", "level": "carrier", "kind": "carrier.water", "label": "Water total", "value": 250, "unit": "m3", "scaleDomain": "site", "carrier": "water", "basis": "reported_meter", "sourceIds": ["meter.facility.water"] },
      { "id": "end_use.water_systems.building", "level": "end_use", "kind": "energy.water_systems", "label": "Water systems", "value": 250, "unit": "m3", "scaleDomain": "site", "endUse": "water_systems", "basis": "reported_meter", "sourceIds": ["meter.water_systems.water"] }
    ],
    "links": [
      { "id": "cooling-electricity", "fromId": "end_use.cooling.building", "toId": "carrier.electricity.building", "relation": "end_use_to_carrier", "basis": "reported_meter", "fromValue": 10, "toValue": 10, "fromUnit": "kWh", "toUnit": "kWh", "sourceIds": ["meter.cooling.electricity"] },
      { "id": "water-systems-water", "fromId": "end_use.water_systems.building", "toId": "carrier.water.building", "relation": "end_use_to_carrier", "basis": "reported_meter", "fromValue": 250, "toValue": 250, "fromUnit": "m3", "toUnit": "m3", "sourceIds": ["meter.water_systems.water"] }
    ],
    "reconciliation": [
      { "id": "reconcile.energy.electricity.annual", "level": "energy", "period": "annual", "label": "Electricity total basis", "status": "balanced", "expectedValue": 10, "explainedValue": 10, "residualValue": 0, "unit": "kWh", "basis": "residual", "sourceIds": ["meter.facility.electricity", "meter.cooling.electricity"] },
      { "id": "reconcile.energy.water.annual", "level": "energy", "period": "annual", "label": "Water total basis", "status": "balanced", "expectedValue": 250, "explainedValue": 250, "residualValue": 0, "unit": "m3", "basis": "residual", "sourceIds": ["meter.facility.water", "meter.water_systems.water"] }
    ],
    "sources": [
      { "id": "meter.facility.electricity", "sourceType": "sql_meter", "isMeter": true, "name": "Electricity:Facility", "sourceUnit": "J", "normalizedUnit": "kWh" },
      { "id": "meter.cooling.electricity", "sourceType": "sql_meter", "isMeter": true, "name": "Cooling:Electricity", "sourceUnit": "J", "normalizedUnit": "kWh" },
      { "id": "meter.facility.water", "sourceType": "sql_meter", "isMeter": true, "name": "Water:Facility", "units": "m3", "sourceUnit": "m3", "normalizedUnit": "m3" },
      { "id": "meter.water_systems.water", "sourceType": "sql_meter", "isMeter": true, "name": "WaterSystems:Water", "units": "m3", "sourceUnit": "m3", "normalizedUnit": "m3" }
    ],
    "completeness": {
      "status": "complete", "mappedPercent": 100,
      "energyUse": { "level": "energy", "status": "complete", "found": 4, "total": 4 },
      "items": [{ "level": "energy", "status": "complete", "found": 4, "total": 4 }],
      "sourceAvailability": [
        { "name": "Electricity:Facility", "level": "energy", "status": "found", "sourceIds": ["meter.facility.electricity"] },
        { "name": "Cooling:Electricity", "level": "energy", "status": "found", "sourceIds": ["meter.cooling.electricity"] },
        { "name": "Water:Facility", "level": "energy", "status": "found", "sourceIds": ["meter.facility.water"] },
        { "name": "WaterSystems:Water", "level": "energy", "status": "found", "sourceIds": ["meter.water_systems.water"] }
      ]
    }
  },
  "energyExplanationSummary": {
    "schema": "semantic-idf.energy-explanation-summary/v2",
    "period": "annual",
    "scope": { "kind": "building", "aggregationBasis": "model_total" },
    "endUses": [
      { "id": "end_use.cooling.building", "level": "end_use", "kind": "energy.cooling", "label": "Cooling", "value": 10, "unit": "kWh", "scaleDomain": "site", "endUse": "cooling" },
      { "id": "end_use.water_systems.building", "level": "end_use", "kind": "energy.water_systems", "label": "Water systems", "value": 250, "unit": "m3", "scaleDomain": "site", "endUse": "water_systems" }
    ],
    "carriers": [
      { "id": "carrier.electricity.building", "level": "carrier", "kind": "carrier.electricity", "label": "Electricity", "value": 10, "unit": "kWh", "scaleDomain": "site", "carrier": "electricity" },
      { "id": "carrier.water.building", "level": "carrier", "kind": "carrier.water", "label": "Water total", "value": 250, "unit": "m3", "scaleDomain": "site", "carrier": "water" }
    ],
    "completeness": {
      "status": "complete", "mappedPercent": 100,
      "energyUse": { "level": "energy", "status": "complete", "found": 4, "total": 4 }
    }
  }
}`)

	assertSanitized := func(t *testing.T, label string, bundle PurposeResultBundle) {
		t.Helper()
		result := bundle.EnergyExplanation
		if result.Schema != energyExplanationSchema {
			t.Errorf("%s schema = %q, want v2", label, result.Schema)
		}
		for _, node := range result.Nodes {
			if node.Carrier == "water" || strings.Contains(node.ID, ".water") || node.Unit == "m3" || epath110AuditContainsAny(node.SourceIDs, "meter.facility.water", "meter.water_systems.water") {
				t.Errorf("%s raw water node survived v2 read sanitation: %#v", label, node)
			}
		}
		for _, link := range result.Links {
			if strings.Contains(link.FromID, ".water") || strings.Contains(link.ToID, ".water") || link.FromUnit == "m3" || link.ToUnit == "m3" || epath110AuditContainsAny(link.SourceIDs, "meter.facility.water", "meter.water_systems.water") {
				t.Errorf("%s raw water link survived v2 read sanitation: %#v", label, link)
			}
		}
		for _, item := range result.Reconciliation {
			if energyPathReconciliationIsWater(item) || item.Unit == "m3" || epath110AuditContainsAny(item.SourceIDs, "meter.facility.water", "meter.water_systems.water") {
				t.Errorf("%s raw water reconciliation survived v2 read sanitation: %#v", label, item)
			}
		}
		if result.Completeness.Status != "complete" || result.Completeness.MappedPercent != 100 || result.Completeness.EnergyUse.Status != "complete" || result.Completeness.EnergyUse.Found != 2 || result.Completeness.EnergyUse.Total != 2 {
			t.Errorf("%s v2 water sanitation completeness = %#v; want electricity-only 2/2 and 100%%", label, result.Completeness)
		}
		for _, name := range []string{"Water:Facility", "WaterSystems:Water"} {
			availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, name)
			if availability == nil || availability.Level != "context" || availability.Status != "found" {
				t.Errorf("%s water availability %q = %#v; want found context", label, name, availability)
			}
		}
		for _, sourceID := range []string{"meter.facility.water", "meter.water_systems.water"} {
			source := epath110AuditSourceByID(result.Sources, sourceID)
			if source == nil || source.SourceUnit != "m3" || source.NormalizedUnit != "m3" || !strings.EqualFold(source.InspectorSection, "context") {
				t.Errorf("%s retained water context source %q = %#v", label, sourceID, source)
			}
		}

		summary := bundle.EnergyExplanationSummary
		if summary.Schema != energyExplanationSummarySchema || summary.Completeness.MappedPercent != 100 || summary.Completeness.EnergyUse.Found != 2 || summary.Completeness.EnergyUse.Total != 2 {
			t.Errorf("%s stored summary completeness was not rebuilt from sanitized v2 result: %#v", label, summary)
		}
		siteTotal := 0.0
		for _, item := range summary.Carriers {
			if item.Carrier == "water" || strings.Contains(item.ID, ".water") || item.Unit == "m3" {
				t.Errorf("%s raw water contaminated stored v2 carrier summary: %#v", label, item)
			}
			siteTotal += item.Value
		}
		if len(summary.Carriers) != 1 || summary.Carriers[0].Carrier != "electricity" || summary.Carriers[0].Unit != "kWh" || siteTotal != 10 {
			t.Errorf("%s site-energy summary total = %g from %#v; want electricity-only 10 kWh", label, siteTotal, summary.Carriers)
		}
		for _, item := range summary.EndUses {
			if strings.Contains(item.ID, "water_systems") || item.Unit == "m3" {
				t.Errorf("%s raw water contaminated stored v2 end-use summary: %#v", label, item)
			}
		}
		canonicalSummary := buildEnergyExplanationSummary(result)
		if len(canonicalSummary.Carriers) != 1 || canonicalSummary.Carriers[0].Carrier != "electricity" || canonicalSummary.Carriers[0].Value != 10 || canonicalSummary.Carriers[0].Unit != "kWh" {
			t.Errorf("%s backend site-energy summary remained contaminated: %#v", label, canonicalSummary.Carriers)
		}
	}

	var first PurposeResultBundle
	if err := json.Unmarshal(stored, &first); err != nil {
		t.Fatalf("unmarshal adversarial stored-v2 bundle: %v", err)
	}
	assertSanitized(t, "first read", first)
	reencoded, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal sanitized stored-v2 bundle: %v", err)
	}
	var second PurposeResultBundle
	if err := json.Unmarshal(reencoded, &second); err != nil {
		t.Fatalf("unmarshal sanitized stored-v2 round trip: %v", err)
	}
	assertSanitized(t, "second read", second)
}

func TestEPATH110AuditWaterMayEnterOnlyWithExplicitSiteEnergyConversion(t *testing.T) {
	series := []energyExplanationSeries{
		{Stage: "carrier", CanonicalKind: "energy.water.total", Level: "energy", Kind: "energy.water.total", Label: "legacy converted water", Unit: "kWh", Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: "derived_ratio", SourceIDs: []string{"derived.water.facility"}, Total: 25},
		{Stage: "end_use", CanonicalKind: "energy.water_systems", Level: "energy", Kind: "energy.water_systems", Label: "legacy converted water systems", Unit: "kWh", Carrier: "water", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", Basis: "derived_ratio", SourceIDs: []string{"derived.water.end_use"}, Total: 25},
	}
	sources := []EnergyDataSource{
		{ID: "derived.water.facility", SourceType: "derived_formula", IsMeter: true, Name: "Water:Facility", Units: "m3", SourceUnit: "m3", NormalizedUnit: "kWh", Formula: "250 m3 * explicit 0.1 kWh/m3 site-energy conversion"},
		{ID: "derived.water.end_use", SourceType: "derived_formula", IsMeter: true, Name: "WaterSystems:Water", Units: "m3", SourceUnit: "m3", NormalizedUnit: "kWh", Formula: "250 m3 * explicit 0.1 kWh/m3 site-energy conversion"},
	}

	result := epath110AuditResultFromSeries(series, sources)
	carrier := epath110AuditNodeByID(result.Nodes, "carrier.water.building")
	endUse := epath110AuditNodeByID(result.Nodes, "end_use.water_systems.building")
	link := epath110AuditLinkByIDs(result.Links, "end_use.water_systems.building", "carrier.water.building")
	if carrier == nil || carrier.Carrier != "water" || carrier.Label != "Water" || carrier.Unit != "kWh" || carrier.ScaleDomain != "site" || carrier.Basis != "derived_ratio" {
		t.Errorf("explicitly converted water carrier = %#v", carrier)
	}
	if endUse == nil || endUse.Value != 25 || endUse.Unit != "kWh" || endUse.ScaleDomain != "site" {
		t.Errorf("explicitly converted water end use = %#v", endUse)
	}
	if link == nil || link.FromValue != 25 || link.ToValue != 25 || link.FromUnit != "kWh" || link.ToUnit != "kWh" {
		t.Errorf("explicitly converted water carrier link = %#v", link)
	}
	for _, sourceID := range []string{"derived.water.facility", "derived.water.end_use"} {
		source := epath110AuditSourceByID(result.Sources, sourceID)
		if source == nil || source.SourceUnit != "m3" || source.NormalizedUnit != "kWh" || source.Formula == "" || !strings.EqualFold(source.InspectorSection, "context") {
			t.Errorf("explicit water conversion provenance %q = %#v", sourceID, source)
		}
	}
}

func TestEPATH110AuditMixedRawAndConvertedWaterKeepsOnlyEnergyReconciliation(t *testing.T) {
	legacy := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.water.raw", Level: "energy", Kind: "energy.water.total", Label: "Raw water total", Value: 250, Unit: "m3", Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: "reported_meter", SourceIDs: []string{"raw.water.facility", "shared.water.input"}},
			{ID: "energy.end_use.water.raw", Level: "energy", Kind: "energy.water_systems", Label: "Raw water systems", Value: 250, Unit: "m3", Carrier: "water", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", Basis: "reported_meter", SourceIDs: []string{"raw.water.end_use", "shared.water.input"}},
			{ID: "energy.carrier.water.converted", Level: "energy", Kind: "energy.water.total", Label: "Converted water total", Value: 25, Unit: "kWh", Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: "derived_ratio", SourceIDs: []string{"derived.water.facility"}},
			{ID: "energy.end_use.water.converted", Level: "energy", Kind: "energy.water_systems", Label: "Converted water systems", Value: 25, Unit: "kWh", Carrier: "water", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", Basis: "derived_ratio", SourceIDs: []string{"derived.water.end_use"}},
		},
		Edges: []EnergyExplanationEdge{
			{ID: "raw-water-end-use", FromID: "energy.carrier.water.raw", ToID: "energy.end_use.water.raw", Value: 250, Unit: "m3", Relation: "meter_enduse", Basis: "reported_meter", SourceIDs: []string{"raw.water.end_use", "shared.water.input"}},
			{ID: "converted-water-end-use", FromID: "energy.carrier.water.converted", ToID: "energy.end_use.water.converted", Value: 25, Unit: "kWh", Relation: "meter_enduse", Basis: "derived_ratio", SourceIDs: []string{"derived.water.end_use"}},
		},
		Reconciliation: []EnergyReconciliation{
			{ID: "reconcile.energy.water.raw.annual", Level: "energy", Period: "annual", Label: "Raw water total basis", Status: "balanced", ExpectedValue: 250, ExplainedValue: 250, Unit: "m3", Basis: "residual", SourceIDs: []string{"raw.water.facility", "raw.water.end_use", "shared.water.input"}},
			{ID: "reconcile.energy.water.converted.annual", Level: "energy", Period: "annual", Label: "Converted water energy total basis", Status: "balanced", ExpectedValue: 25, ExplainedValue: 25, Unit: "kWh", Basis: "derived_ratio", SourceIDs: []string{"derived.water.facility", "derived.water.end_use", "shared.water.input"}},
		},
		Sources: []EnergyDataSource{
			{ID: "shared.water.input", SourceType: "sql_meter", IsMeter: true, Name: "Water:Facility shared input", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"},
			{ID: "raw.water.facility", SourceType: "sql_meter", IsMeter: true, Name: "Water:Facility", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"},
			{ID: "raw.water.end_use", SourceType: "sql_meter", IsMeter: true, Name: "WaterSystems:Water", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"},
			{ID: "derived.water.facility", SourceType: "derived_formula", IsMeter: true, Name: "Water:Facility converted", Units: "m3", SourceUnit: "m3", NormalizedUnit: "kWh", Formula: "250 m3 * explicit 0.1 kWh/m3 site-energy conversion", InputSourceIDs: []string{"shared.water.input"}},
			{ID: "derived.water.end_use", SourceType: "derived_formula", IsMeter: true, Name: "WaterSystems:Water converted", Units: "m3", SourceUnit: "m3", NormalizedUnit: "kWh", Formula: "250 m3 * explicit 0.1 kWh/m3 site-energy conversion", InputSourceIDs: []string{"shared.water.input"}},
		},
		Completeness: EnergyCompleteness{
			Status:        "complete",
			MappedPercent: 0,
			EnergyUse:     EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 4, Total: 4},
			Items:         []EnergyCompletenessLevel{{Level: "energy", Status: "complete", Found: 4, Total: 4}},
			SourceAvailability: []EnergySourceAvailabilityEntry{
				// Raw water deliberately comes first. Scanning it must not stop
				// provenance indexing for the converted entries that follow.
				{Name: "Water:Facility", Level: "energy", Status: "found", SourceIDs: []string{"raw.water.facility", "shared.water.input"}},
				{Name: "WaterSystems:Water", Level: "energy", Status: "found", SourceIDs: []string{"raw.water.end_use", "shared.water.input"}},
				{Name: "Water:Facility", Level: "energy", Status: "found", SourceIDs: []string{"derived.water.facility"}},
				{Name: "WaterSystems:Water", Level: "energy", Status: "found", SourceIDs: []string{"derived.water.end_use"}},
			},
		},
		scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
	}

	result := UpgradeEnergyExplanationV1(legacy)
	waterRows := make([]EnergyReconciliation, 0)
	for _, item := range result.Reconciliation {
		identity := strings.ToLower(strings.Join(append([]string{item.ID, item.Label}, item.SourceIDs...), " "))
		if strings.Contains(identity, "water") {
			waterRows = append(waterRows, item)
		}
	}
	if len(waterRows) != 1 {
		t.Fatalf("mixed raw/converted water reconciliation rows = %#v; want only one converted-energy row", waterRows)
	}
	row := waterRows[0]
	if row.Unit != "kWh" || row.ExpectedValue != 25 || row.ExplainedValue != 25 || row.ResidualValue != 0 || row.Status != "balanced" {
		t.Errorf("retained converted-water reconciliation = %#v; want balanced 25 kWh", row)
	}
	if epath110AuditContainsAny(row.SourceIDs, "raw.water.facility", "raw.water.end_use") || !epath110AuditContainsAny(row.SourceIDs, "derived.water.facility", "derived.water.end_use") {
		t.Errorf("converted-water reconciliation provenance = %#v; raw water must be absent and derived conversion present", row.SourceIDs)
	}
	if result.Completeness.Status != "complete" || result.Completeness.MappedPercent != 100 || result.Completeness.EnergyUse.Status != "complete" || result.Completeness.EnergyUse.Found != 2 || result.Completeness.EnergyUse.Total != 2 {
		t.Errorf("mixed raw/converted water completeness = %#v; want converted-energy-only 2/2 and 100%%", result.Completeness)
	}
	for _, availability := range result.Completeness.SourceAvailability {
		if epath110AuditContainsAny(availability.SourceIDs, "raw.water.facility", "raw.water.end_use") && availability.Level != "context" {
			t.Errorf("raw-first water availability stayed in energy: %#v", availability)
		}
		if epath110AuditContainsAny(availability.SourceIDs, "derived.water.facility", "derived.water.end_use") && availability.Level != "energy" {
			t.Errorf("later derived-water availability was misclassified after raw source: %#v", availability)
		}
	}
	if legacy.Reconciliation[0].Unit != "m3" || legacy.Reconciliation[0].ExpectedValue != 250 || !epath110AuditContainsAny(legacy.Reconciliation[0].SourceIDs, "raw.water.facility") {
		t.Error("mixed-water reconciliation filtering mutated the stored-v1 input")
	}
}

func TestEPATH110AuditSeriesNeverCoalescesRawWaterWithConvertedEnergy(t *testing.T) {
	raw := []energyExplanationSeries{
		{Stage: "carrier", CanonicalKind: "energy.water.total", Level: "energy", Kind: "energy.water.total", Label: "Raw water total", Unit: "m3", Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: "reported_meter", SourceIDs: []string{"raw.water.facility"}, Total: 250},
		{Stage: "end_use", CanonicalKind: "energy.water_systems", Level: "energy", Kind: "energy.water_systems", Label: "Raw water systems", Unit: "m3", Carrier: "water", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", Basis: "reported_meter", SourceIDs: []string{"raw.water.end_use"}, Total: 250},
	}
	converted := []energyExplanationSeries{
		{Stage: "carrier", CanonicalKind: "energy.water.total", Level: "energy", Kind: "energy.water.total", Label: "Converted water total", Unit: "kWh", Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: "derived_ratio", SourceIDs: []string{"derived.water.facility"}, Total: 25},
		{Stage: "end_use", CanonicalKind: "energy.water_systems", Level: "energy", Kind: "energy.water_systems", Label: "Converted water systems", Unit: "kWh", Carrier: "water", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", Basis: "derived_ratio", SourceIDs: []string{"derived.water.end_use"}, Total: 25},
	}
	sources := []EnergyDataSource{
		{ID: "raw.water.facility", SourceType: "sql_meter", IsMeter: true, Name: "Water:Facility", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"},
		{ID: "raw.water.end_use", SourceType: "sql_meter", IsMeter: true, Name: "WaterSystems:Water", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"},
		{ID: "derived.water.facility", SourceType: "derived_formula", IsMeter: true, Name: "Water:Facility converted", Units: "m3", SourceUnit: "m3", NormalizedUnit: "kWh", Formula: "250 m3 * explicit 0.1 kWh/m3 site-energy conversion"},
		{ID: "derived.water.end_use", SourceType: "derived_formula", IsMeter: true, Name: "WaterSystems:Water converted", Units: "m3", SourceUnit: "m3", NormalizedUnit: "kWh", Formula: "250 m3 * explicit 0.1 kWh/m3 site-energy conversion"},
	}

	orders := []struct {
		name   string
		series []energyExplanationSeries
	}{
		{name: "raw_before_converted", series: append(append([]energyExplanationSeries(nil), raw...), converted...)},
		{name: "converted_before_raw", series: append(append([]energyExplanationSeries(nil), converted...), raw...)},
	}
	for _, order := range orders {
		t.Run(order.name, func(t *testing.T) {
			result := epath110AuditResultFromSeries(order.series, sources)
			if result.Completeness.MappedPercent != 100 {
				t.Errorf("raw water diluted series-level converted-energy mappedPercent to %g", result.Completeness.MappedPercent)
			}
			carrier := epath110AuditNodeByID(result.Nodes, "carrier.water.building")
			endUse := epath110AuditNodeByID(result.Nodes, "end_use.water_systems.building")
			link := epath110AuditLinkByIDs(result.Links, "end_use.water_systems.building", "carrier.water.building")
			if carrier == nil || carrier.Value != 25 || carrier.Unit != "kWh" || carrier.Basis != "derived_ratio" {
				t.Errorf("raw and converted water series coalesced in carrier: %#v", carrier)
			}
			if endUse == nil || endUse.Value != 25 || endUse.Unit != "kWh" || endUse.Basis != "derived_ratio" {
				t.Errorf("raw and converted water series coalesced in end use: %#v", endUse)
			}
			if link == nil || link.FromValue != 25 || link.ToValue != 25 || link.FromUnit != "kWh" || link.ToUnit != "kWh" || link.Basis != "derived_ratio" {
				t.Errorf("raw and converted water series coalesced in branch: %#v", link)
			}
			if carrier != nil && (epath110AuditContainsAny(carrier.SourceIDs, "raw.water.facility", "raw.water.end_use") || !epath110AuditContainsAny(carrier.SourceIDs, "derived.water.facility")) {
				t.Errorf("converted water carrier provenance = %#v", carrier.SourceIDs)
			}
			if endUse != nil && (epath110AuditContainsAny(endUse.SourceIDs, "raw.water.facility", "raw.water.end_use") || !epath110AuditContainsAny(endUse.SourceIDs, "derived.water.end_use")) {
				t.Errorf("converted water end-use provenance = %#v", endUse.SourceIDs)
			}
			if link != nil && (epath110AuditContainsAny(link.SourceIDs, "raw.water.facility", "raw.water.end_use") || !epath110AuditContainsAny(link.SourceIDs, "derived.water.end_use")) {
				t.Errorf("converted water branch provenance = %#v", link.SourceIDs)
			}
			waterRows := make([]EnergyReconciliation, 0)
			for _, item := range result.Reconciliation {
				if strings.Contains(strings.ToLower(item.ID+" "+item.Label), "water") {
					waterRows = append(waterRows, item)
				}
			}
			if len(waterRows) != 1 || waterRows[0].ExpectedValue != 25 || waterRows[0].ExplainedValue != 25 || waterRows[0].Unit != "kWh" || epath110AuditContainsAny(waterRows[0].SourceIDs, "raw.water.facility", "raw.water.end_use") {
				t.Errorf("series-level converted-water reconciliation = %#v", waterRows)
			}
			for _, sourceID := range []string{"raw.water.facility", "raw.water.end_use"} {
				source := epath110AuditSourceByID(result.Sources, sourceID)
				if source == nil || !strings.EqualFold(source.InspectorSection, "context") || source.SourceUnit != "m3" || source.NormalizedUnit != "m3" {
					t.Errorf("raw water context source %q = %#v", sourceID, source)
				}
			}
		})
	}
}

func TestEPATH110AuditWaterConversionRequiresExplicitUnitAndFormulaEvidence(t *testing.T) {
	tests := []struct {
		name       string
		basis      string
		nodeUnit   string
		sourceID   string
		sourceUnit string
		formula    string
	}{
		{name: "missing formula", basis: "derived_ratio", nodeUnit: "kWh", sourceID: "derived.water", sourceUnit: "m3"},
		{name: "source is already energy", basis: "derived_ratio", nodeUnit: "kWh", sourceID: "derived.water", sourceUnit: "kWh", formula: "identity is not a volume-to-energy conversion"},
		{name: "node is reported meter", basis: "reported_meter", nodeUnit: "kWh", sourceID: "derived.water", sourceUnit: "m3", formula: "volume * site-energy conversion factor"},
		{name: "node remains volume", basis: "derived_ratio", nodeUnit: "m3", sourceID: "derived.water", sourceUnit: "m3", formula: "volume * site-energy conversion factor"},
		{name: "conversion provenance is missing", basis: "derived_ratio", nodeUnit: "kWh", sourceID: "missing.water", sourceUnit: "m3", formula: "volume * site-energy conversion factor"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			series := []energyExplanationSeries{
				{Stage: "carrier", CanonicalKind: "energy.water.total", Level: "energy", Kind: "energy.water.total", Label: "Water", Unit: test.nodeUnit, Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: test.basis, SourceIDs: []string{test.sourceID}, Total: 25},
				{Stage: "end_use", CanonicalKind: "energy.water_systems", Level: "energy", Kind: "energy.water_systems", Label: "Water systems", Unit: test.nodeUnit, Carrier: "water", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", Basis: test.basis, SourceIDs: []string{test.sourceID}, Total: 25},
			}
			sources := []EnergyDataSource(nil)
			if test.sourceID != "missing.water" {
				sources = []EnergyDataSource{{
					ID:             test.sourceID,
					SourceType:     "derived_formula",
					IsMeter:        true,
					Name:           "Water:Facility",
					Units:          test.sourceUnit,
					SourceUnit:     test.sourceUnit,
					NormalizedUnit: test.nodeUnit,
					Formula:        test.formula,
				}}
			}

			result := epath110AuditResultFromSeries(series, sources)
			for _, node := range result.Nodes {
				if node.Carrier == "water" || strings.Contains(node.ID, ".water") || epath110AuditContainsAny(node.SourceIDs, test.sourceID) {
					t.Errorf("water without explicit conversion evidence entered site energy: %#v", node)
				}
			}
			for _, link := range result.Links {
				if strings.Contains(link.FromID, ".water") || strings.Contains(link.ToID, ".water") || epath110AuditContainsAny(link.SourceIDs, test.sourceID) {
					t.Errorf("water without explicit conversion evidence entered a site-energy link: %#v", link)
				}
			}
			if test.sourceID != "missing.water" {
				source := epath110AuditSourceByID(result.Sources, test.sourceID)
				if source == nil {
					t.Fatal("rejected water evidence was dropped instead of retained as context")
				}
				if !strings.EqualFold(source.InspectorSection, "context") {
					t.Errorf("rejected water evidence section = %q, want context", source.InspectorSection)
				}
			}
		})
	}
}

func TestEPATH110AuditCoalescedWaterRequiresEveryDirectSourceToProveConversion(t *testing.T) {
	sources := []EnergyDataSource{
		{ID: "raw.water", SourceType: "sql_meter", IsMeter: true, Name: "Water:Facility", Units: "m3", SourceUnit: "m3", NormalizedUnit: "m3"},
		{ID: "derived.water", SourceType: "derived_formula", Name: "Converted Water:Facility", Units: "m3", SourceUnit: "m3", NormalizedUnit: "kWh", Formula: "250 m3 * explicit 0.1 kWh/m3 site-energy conversion", InputSourceIDs: []string{"raw.water"}},
	}
	mixedSourceIDs := []string{"raw.water", "derived.water"}
	legacy := EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes: []EnergyExplanationNode{
			{ID: "energy.carrier.water.coalesced", Level: "energy", Kind: "energy.water.total", Label: "Coalesced water", Value: 275, Unit: "kWh", Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: "derived_ratio", SourceIDs: mixedSourceIDs},
			{ID: "energy.end_use.water.coalesced", Level: "energy", Kind: "energy.water_systems", Label: "Coalesced water systems", Value: 275, Unit: "kWh", Carrier: "water", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", Basis: "derived_ratio", SourceIDs: mixedSourceIDs},
		},
		Edges: []EnergyExplanationEdge{
			{ID: "coalesced-water-end-use", FromID: "energy.carrier.water.coalesced", ToID: "energy.end_use.water.coalesced", Value: 275, Unit: "kWh", Relation: "meter_enduse", Basis: "derived_ratio", SourceIDs: mixedSourceIDs},
		},
		Reconciliation: []EnergyReconciliation{
			{ID: "reconcile.energy.water.coalesced.annual", Level: "energy", Period: "annual", Label: "Coalesced water total basis", Status: "balanced", ExpectedValue: 275, ExplainedValue: 275, Unit: "kWh", Basis: "derived_ratio", SourceIDs: mixedSourceIDs},
		},
		Sources: sources,
		Completeness: EnergyCompleteness{
			Status: "complete", MappedPercent: 100,
			EnergyUse: EnergyCompletenessLevel{Level: "energy", Status: "complete", Found: 1, Total: 1},
		},
		scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
	}

	result := UpgradeEnergyExplanationV1(legacy)
	if len(result.Nodes) != 0 || len(result.Links) != 0 || len(result.Reconciliation) != 0 || result.Completeness.MappedPercent != 0 {
		t.Errorf("coalesced stored-v1 water with mixed direct provenance entered energy accounting: nodes=%#v links=%#v reconciliation=%#v completeness=%#v", result.Nodes, result.Links, result.Reconciliation, result.Completeness)
	}
	for _, sourceID := range mixedSourceIDs {
		source := epath110AuditSourceByID(result.Sources, sourceID)
		if source == nil || !strings.EqualFold(source.InspectorSection, "context") {
			t.Errorf("rejected coalesced-water source %q was not retained as context: %#v", sourceID, source)
		}
	}
	if legacy.Nodes[0].Value != 275 || len(legacy.Nodes[0].SourceIDs) != 2 || legacy.Sources[1].InputSourceIDs[0] != "raw.water" {
		t.Error("coalesced-water rejection mutated its stored-v1 input")
	}

	mixedSeries := []energyExplanationSeries{
		{Stage: "carrier", CanonicalKind: "energy.water.total", Level: "energy", Kind: "energy.water.total", Label: "Mixed direct-source water", Unit: "kWh", Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: "derived_ratio", SourceIDs: mixedSourceIDs, Total: 275},
		{Stage: "end_use", CanonicalKind: "energy.water_systems", Level: "energy", Kind: "energy.water_systems", Label: "Mixed direct-source water systems", Unit: "kWh", Carrier: "water", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", Basis: "derived_ratio", SourceIDs: mixedSourceIDs, Total: 275},
	}
	seriesResult := epath110AuditResultFromSeries(mixedSeries, sources)
	if len(seriesResult.Nodes) != 0 || len(seriesResult.Links) != 0 || len(seriesResult.Reconciliation) != 0 || seriesResult.Completeness.MappedPercent != 0 {
		t.Errorf("mixed direct-source water series entered energy accounting: nodes=%#v links=%#v reconciliation=%#v completeness=%#v", seriesResult.Nodes, seriesResult.Links, seriesResult.Reconciliation, seriesResult.Completeness)
	}

	validDerivedOnly := legacy
	validDerivedOnly.Nodes = append([]EnergyExplanationNode(nil), legacy.Nodes...)
	validDerivedOnly.Edges = append([]EnergyExplanationEdge(nil), legacy.Edges...)
	validDerivedOnly.Reconciliation = append([]EnergyReconciliation(nil), legacy.Reconciliation...)
	for index := range validDerivedOnly.Nodes {
		validDerivedOnly.Nodes[index].Value = 25
		validDerivedOnly.Nodes[index].SourceIDs = []string{"derived.water"}
	}
	validDerivedOnly.Edges[0].Value = 25
	validDerivedOnly.Edges[0].SourceIDs = []string{"derived.water"}
	validDerivedOnly.Reconciliation[0].ExpectedValue = 25
	validDerivedOnly.Reconciliation[0].ExplainedValue = 25
	validDerivedOnly.Reconciliation[0].SourceIDs = []string{"derived.water"}
	valid := UpgradeEnergyExplanationV1(validDerivedOnly)
	carrier := epath110AuditNodeByID(valid.Nodes, "carrier.water.building")
	link := epath110AuditLinkByIDs(valid.Links, "end_use.water_systems.building", "carrier.water.building")
	if carrier == nil || carrier.Value != 25 || !reflect.DeepEqual(carrier.SourceIDs, []string{"derived.water"}) || link == nil || link.ToValue != 25 || !reflect.DeepEqual(link.SourceIDs, []string{"derived.water"}) {
		t.Errorf("valid derived-only water path was lost or broadened to raw direct provenance: carrier=%#v link=%#v", carrier, link)
	}
	derived := epath110AuditSourceByID(valid.Sources, "derived.water")
	if derived == nil || !reflect.DeepEqual(derived.InputSourceIDs, []string{"raw.water"}) {
		t.Errorf("valid derived water source lost raw input provenance: %#v", derived)
	}
}

func TestEPATH110AuditWaterConversionAcceptsOnlyVolumetricSourceUnits(t *testing.T) {
	tests := []struct {
		sourceUnit string
		accepted   bool
	}{
		{sourceUnit: "m3", accepted: true},
		{sourceUnit: " M3 ", accepted: true},
		{sourceUnit: "L", accepted: true},
		{sourceUnit: " l ", accepted: true},
		{sourceUnit: "kg"},
		{sourceUnit: "C"},
		{sourceUnit: "arbitrary-widget-unit"},
	}

	for _, test := range tests {
		t.Run(metricID(test.sourceUnit), func(t *testing.T) {
			series := []energyExplanationSeries{
				{Stage: "carrier", CanonicalKind: "energy.water.total", Level: "energy", Kind: "energy.water.total", Label: "Converted water total", Unit: "kWh", Carrier: "water", EndUse: "water", MeterHierarchyLevel: "facility_total", Basis: "derived_ratio", SourceIDs: []string{"derived.water"}, Total: 1},
				{Stage: "end_use", CanonicalKind: "energy.water_systems", Level: "energy", Kind: "energy.water_systems", Label: "Converted water systems", Unit: "kWh", Carrier: "water", EndUse: "water_systems", MeterHierarchyLevel: "broad_end_use", Basis: "derived_ratio", SourceIDs: []string{"derived.water"}, Total: 1},
			}
			sources := []EnergyDataSource{{
				ID: "derived.water", SourceType: "derived_formula", IsMeter: true, Name: "Water:Facility",
				Units: test.sourceUnit, SourceUnit: test.sourceUnit, NormalizedUnit: "kWh",
				Formula: "source volume * explicit site-energy conversion factor",
			}}

			result := epath110AuditResultFromSeries(series, sources)
			carrier := epath110AuditNodeByID(result.Nodes, "carrier.water.building")
			endUse := epath110AuditNodeByID(result.Nodes, "end_use.water_systems.building")
			link := epath110AuditLinkByIDs(result.Links, "end_use.water_systems.building", "carrier.water.building")
			if test.accepted {
				if carrier == nil || carrier.Value != 1 || carrier.Unit != "kWh" || endUse == nil || endUse.Value != 1 || endUse.Unit != "kWh" || link == nil || link.FromValue != 1 || link.ToValue != 1 {
					t.Errorf("volumetric source unit %q did not produce a coherent converted-water energy path: carrier=%#v endUse=%#v link=%#v", test.sourceUnit, carrier, endUse, link)
				}
				return
			}
			if carrier != nil || endUse != nil || link != nil {
				t.Errorf("non-volume source unit %q was trusted as explicit water conversion: carrier=%#v endUse=%#v link=%#v", test.sourceUnit, carrier, endUse, link)
			}
			source := epath110AuditSourceByID(result.Sources, "derived.water")
			if source == nil || !strings.EqualFold(source.InspectorSection, "context") || source.SourceUnit != test.sourceUnit {
				t.Errorf("rejected non-volume water source %q was not retained faithfully as context: %#v", test.sourceUnit, source)
			}
		})
	}
}

func TestEPATH110AuditAmbiguousDirectFuelEvidenceNeverClaimsGenericFuel(t *testing.T) {
	want := map[string]string{
		"Zone Lights Electricity Energy":                   "electricity",
		"Zone Gas Equipment NaturalGas Energy":             "natural_gas",
		"Zone Hot Water Equipment District Heating Energy": "district_heating",
		"Zone Steam Equipment District Heating Energy":     "district_heating",
	}
	for name, carrier := range want {
		definition, ok := energyVariableAliasDefinitionForName(name)
		if !ok || definition.Carrier != carrier || definition.Carrier == "fuel" {
			t.Errorf("direct-use variable %q = %#v, ok=%t; want exact carrier %q", name, definition, ok, carrier)
		}
	}
	ambiguous, ok := energyVariableAliasDefinitionForName("Zone Other Equipment Fuel Energy")
	if !ok || ambiguous.Carrier == "fuel" || ambiguous.Carrier != "other" {
		t.Fatalf("ambiguous OtherEquipment evidence must remain explicitly unknown-safe, not generic fuel: %#v, ok=%t", ambiguous, ok)
	}
}

func epath110AuditCarrierNames(cases []epath110AuditCarrierCase) []string {
	out := make([]string, 0, len(cases))
	for _, item := range cases {
		out = append(out, item.Carrier)
	}
	return out
}

func epath110AuditResultFromSeries(series []energyExplanationSeries, sources []EnergyDataSource) EnergyExplanationResult {
	_, annotatedSources := filterEnergyExplanationWaterContextSeries(series, sources)
	graph := buildEnergyExplanationGraphForPeriod("annual", series, PurposeAllocationPolicyDirectOnly, func(item energyExplanationSeries) float64 {
		return item.Total
	}, sources)
	return UpgradeEnergyExplanationV1(EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		Purpose:          string(SimulationPurposeBasicEnergy),
		Frequency:        "annual",
		AllocationPolicy: PurposeAllocationPolicyDirectOnly,
		Nodes:            graph.Nodes,
		Edges:            graph.Edges,
		Reconciliation:   graph.Reconciliation,
		Sources:          annotatedSources,
		Completeness:     EnergyCompleteness{MappedPercent: graph.MappedPercent},
		scope:            EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
	})
}

func epath110AuditNodeByID(nodes []EnergyExplanationNode, id string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].ID == id {
			return &nodes[index]
		}
	}
	return nil
}

func epath110AuditLinkByIDs(links []EnergyPathLink, fromID string, toID string) *EnergyPathLink {
	for index := range links {
		if links[index].FromID == fromID && links[index].ToID == toID {
			return &links[index]
		}
	}
	return nil
}

func epath110AuditSourceByID(sources []EnergyDataSource, id string) *EnergyDataSource {
	for index := range sources {
		if sources[index].ID == id {
			return &sources[index]
		}
	}
	return nil
}

func epath110AuditContainsAny(values []string, candidates ...string) bool {
	for _, value := range values {
		for _, candidate := range candidates {
			if value == candidate {
				return true
			}
		}
	}
	return false
}

func TestEPATH110AuditFixtureIdentityIsUnique(t *testing.T) {
	// Keep accidental duplicate carrier/test fixture entries from weakening the
	// matrix above. This is intentionally separate so a duplicate reports a
	// compact error instead of hundreds of ambiguous subtest failures.
	seenCarriers := map[string]bool{}
	seenAliases := map[string]string{}
	for _, carrier := range epath110AuditCarrierCases {
		if seenCarriers[carrier.Carrier] {
			t.Errorf("duplicate carrier fixture %q", carrier.Carrier)
		}
		seenCarriers[carrier.Carrier] = true
		for _, alias := range append(append([]string(nil), carrier.FacilityAliases...), carrier.MeterTokens...) {
			key := strings.ToLower(strings.TrimSpace(alias))
			if previous := seenAliases[key]; previous != "" && previous != carrier.Carrier {
				t.Errorf("alias %q is shared by %q and %q", alias, previous, carrier.Carrier)
			}
			seenAliases[key] = carrier.Carrier
		}
	}
	if len(seenCarriers) != 14 {
		t.Fatalf("EPATH-110 fixture has %d carriers, want 14: %s", len(seenCarriers), fmt.Sprint(epath110AuditCarrierNames(epath110AuditCarrierCases)))
	}
}
