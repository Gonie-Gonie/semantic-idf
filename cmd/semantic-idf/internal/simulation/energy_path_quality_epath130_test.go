package simulation

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestEPATH130StageAvailabilitySeparatesThermalSiteAndAliasGroups(t *testing.T) {
	result := epath130WeightedGraph([]float64{100, 300}, []float64{110, 240})
	result.Completeness = EnergyCompleteness{
		MappedPercent: 100,
		HeatDrivers:   EnergyCompletenessLevel{Level: "heat", Status: "partial", Found: 1, Total: 2},
		DeliveredLoad: EnergyCompletenessLevel{Level: "load", Status: "complete", Found: 2, Total: 2},
		SourceAvailability: []EnergySourceAvailabilityEntry{
			{Name: "Electricity:Facility", Level: "energy", Status: "found"},
			{Name: "NaturalGas:Facility", Level: "energy", Status: "found"},
			{Name: "Cooling:Electricity", Level: "energy", Status: "found"},
			{Name: "Electricity:Cooling", Level: "energy", Status: "found"},
			{Name: "Heating:NaturalGas", Level: "energy", Status: "found"},
			{Name: "ElectricityProduced:Facility", Level: "context", Status: "missing"},
			{Name: "Water:Facility", Level: "context", Status: "missing"},
		},
	}
	quality := BuildEnergyPathQuality(result, "annual")
	if quality.Drivers.Status != "partial" || quality.Drivers.Found != 1 || quality.Drivers.Total != 2 || quality.Loads.Status != "complete" || quality.Carriers.Status != "complete" || quality.Carriers.Found != 2 || quality.Carriers.Total != 2 {
		t.Errorf("carrier-complete run must still expose partial drivers independently of stale mapped100: %#v", quality)
	}
	if quality.EndUses.Status != "complete" || quality.EndUses.Found != 2 || quality.EndUses.Total != 2 {
		t.Errorf("carrier-order aliases were counted twice or context outputs treated as end uses: %#v", quality.EndUses)
	}
}

func TestEPATH130RequestedMissingAndInapplicableRemainDistinctPerStage(t *testing.T) {
	for _, status := range []string{"missing", "not_requested", "not_applicable"} {
		t.Run(status, func(t *testing.T) {
			result := EnergyExplanationResult{Schema: energyExplanationSchema, Completeness: EnergyCompleteness{
				HeatDrivers: EnergyCompletenessLevel{Status: status}, DeliveredLoad: EnergyCompletenessLevel{Status: status},
				SourceAvailability: []EnergySourceAvailabilityEntry{{Name: "Cooling:Electricity", Level: "energy", Status: status}, {Name: "Electricity:Facility", Level: "energy", Status: status}},
			}}
			quality := BuildEnergyPathQuality(result, "annual")
			for name, level := range map[string]EnergyCompletenessLevel{"drivers": quality.Drivers, "loads": quality.Loads, "end uses": quality.EndUses, "carriers": quality.Carriers} {
				if level.Status != status {
					t.Errorf("%s became %s, want explicit %s", name, level.Status, status)
				}
			}
			if status == "not_requested" && quality.Ratios.Status != "not_requested" {
				t.Errorf("Light output plan ratios were reported missing: %#v", quality.Ratios)
			}
		})
	}
	result := EnergyExplanationResult{Completeness: EnergyCompleteness{SourceAvailability: []EnergySourceAvailabilityEntry{
		{Name: "Electricity:Facility", Level: "energy", Status: "not_applicable"},
		{Name: "Cooling:Electricity", Level: "energy", Status: "not_requested"},
	}}}
	quality := BuildEnergyPathQuality(result, "annual")
	if quality.Carriers.Status != "not_applicable" || quality.EndUses.Status != "not_requested" {
		t.Errorf("one site's availability status leaked across stage boundary: %#v", quality)
	}
}

func TestEPATH130ClosureIsWeightedAndCannotCancelOppositeResiduals(t *testing.T) {
	result := epath130WeightedGraph([]float64{100, 300}, []float64{110, 240})
	quality := BuildEnergyPathQuality(result, "annual")
	// Absolute errors10+60 over total400 ->82.5%; neither aggregate net87.5%
	// nor average per-endpoint closure is a valid replacement.
	if quality.DriverToLoadClosedPct != 82.5 || quality.EndUseToCarrierClosedPct != 82.5 {
		t.Errorf("closure canceled opposing discrepancies or ignored endpoint weights: %#v", quality)
	}
	if quality.DriverToLoadStatus != "overmapped" || quality.EndUseToCarrierStatus != "overmapped" {
		t.Errorf("positive and negative discrepancies hid an overmapped endpoint: %#v", quality)
	}
	result = epath130WeightedGraph([]float64{100, 100}, []float64{110, 90})
	quality = BuildEnergyPathQuality(result, "annual")
	if quality.DriverToLoadClosedPct != 90 || quality.EndUseToCarrierClosedPct != 90 {
		t.Errorf("equal positive/negative residuals falsely claimed100%%closure: %#v", quality)
	}
}

func TestEPATH130RatioAvailabilityDoesNotDemandThermalSiteConservation(t *testing.T) {
	result := epath121GeneratedConversion(t, "cooling", "electricity", 100, 25)
	result.Completeness.DeliveredLoad = EnergyCompletenessLevel{Status: "complete", Found: 1, Total: 1}
	carrier := energyPathV2NodeByID(result.Nodes, "carrier.electricity.building")
	carrier.Basis, carrier.MeterHierarchyLevel = "reported_meter", "facility_total"
	quality := BuildEnergyPathQuality(result, "annual")
	if quality.Ratios.Status != "complete" || quality.Ratios.Found != 1 || quality.Ratios.Total != 1 || quality.EndUseToCarrierClosedPct != 100 {
		t.Errorf("100thermal/25site COP4 should have full ratio availability and site closure: %#v", quality)
	}
	for index := range result.Links {
		if result.Links[index].Relation == "load_to_end_use" {
			result.Links[index].SourceIDs = nil
		}
	}
	quality = BuildEnergyPathQuality(result, "annual")
	if quality.Ratios.Found != 0 || quality.Ratios.Total != 1 || quality.Ratios.Status != "missing" {
		t.Errorf("untraceable displayed ratio was marked available: %#v", quality.Ratios)
	}
	if quality.EndUseToCarrierClosedPct != 100 {
		t.Errorf("missing ratio trace changed independent site conservation: %#v", quality)
	}
}

func TestEPATH130PeriodMetricsUseOnlyTheSelectedGraph(t *testing.T) {
	result := epath130WeightedGraph([]float64{100}, []float64{80})
	month := epath130WeightedGraph([]float64{10}, []float64{10})
	result.Periods = []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: month.Nodes, Links: month.Links}}
	annual := BuildEnergyPathQuality(result, "annual")
	january := BuildEnergyPathQuality(result, "M1")
	missing := BuildEnergyPathQuality(result, "M2")
	if annual.DriverToLoadClosedPct != 80 || annual.EndUseToCarrierClosedPct != 80 || january.DriverToLoadClosedPct != 100 || january.EndUseToCarrierClosedPct != 100 {
		t.Errorf("period metrics mixed annual/month data: annual=%#v M1=%#v", annual, january)
	}
	if missing.DriverToLoadClosedPct != 0 || missing.EndUseToCarrierClosedPct != 0 || missing.DriverToLoadStatus == "complete" || missing.EndUseToCarrierStatus == "complete" {
		t.Errorf("absent month borrowed annual graph coverage: %#v", missing)
	}
}

func TestEPATH130ZoneCoverageUsesDirectAllocatedAndExplicitUnassigned(t *testing.T) {
	row := func(id, period string, expected, direct, allocated, unassigned float64) EnergyReconciliation {
		return EnergyReconciliation{ID: id, Level: "allocation", Period: period, ExpectedValue: expected, DirectValue: direct, AllocatedValue: allocated, UnassignedValue: unassigned, Unit: "kWh", Status: "partial"}
	}
	hvac := row("reconcile.zone_hvac_allocation.cooling.electricity.annual", "annual", 100, 20, 60, 20)
	aux := row("reconcile.zone_auxiliary_allocation.fans.electricity.annual", "annual", 50, 5, 35, 10)
	m1 := row("reconcile.zone_hvac_allocation.cooling.electricity.m1", "M1", 10, 3, 7, 0)
	m1.Status = "complete"
	result := EnergyExplanationResult{Reconciliation: []EnergyReconciliation{hvac, aux, hvac, m1, row("unrelated", "annual", 1000, 0, 0, 1000)}, Periods: []EnergyPeriod{{ID: "M1", Reconciliation: []EnergyReconciliation{m1, hvac}}}}
	annual := BuildEnergyPathQuality(result, "annual")
	january := BuildEnergyPathQuality(result, "M1")
	if annual.ZoneAllocatedPct != 80 || annual.UnassignedPct != 20 || annual.ZoneAllocationStatus != "partial" {
		t.Errorf("direct+allocated/explicit unassigned coverage counted duplicate/unrelated/wrongperiod rows: %#v", annual)
	}
	if january.ZoneAllocatedPct != 100 || january.UnassignedPct != 0 || january.ZoneAllocationStatus != "complete" {
		t.Errorf("selected month's direct+allocated coverage included annual gaps: %#v", january)
	}
}

func TestEPATH130SparseStoredPayloadRemainsPartialAndSubtotalsDoNotProveClosure(t *testing.T) {
	result := epath130WeightedGraph([]float64{100}, []float64{100})
	quality := BuildEnergyPathQuality(result, "annual")
	for name, level := range map[string]EnergyCompletenessLevel{"drivers": quality.Drivers, "loads": quality.Loads, "end uses": quality.EndUses, "carriers": quality.Carriers} {
		if level.Status != "partial" {
			t.Errorf("sparse stored %s claimed requested-output coverage: %#v", name, level)
		}
	}
	for index := range result.Nodes {
		if result.Nodes[index].Level == "carrier" {
			result.Nodes[index].Basis = "reported_end_use_subtotal"
			result.Nodes[index].MeterHierarchyLevel = "observed_end_use_subtotal"
		}
	}
	quality = BuildEnergyPathQuality(result, "annual")
	if quality.Carriers.Status != "partial" || quality.EndUseToCarrierClosedPct != 0 || quality.EndUseToCarrierStatus != "partial" {
		t.Errorf("observed subtotal masqueraded as complete facility total: %#v", quality)
	}
}

func TestEPATH130V1JSONStaysFrozenAndV2QualitySurvivesRoundtrip(t *testing.T) {
	legacy := epath090ReviewFixture(false)
	frozen, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(frozen, []byte("\"quality\"")) {
		t.Fatal("new quality field leaked into frozen V1 write fixture")
	}
	var legacyReload EnergyExplanationV1
	if err := json.Unmarshal(frozen, &legacyReload); err != nil {
		t.Fatal(err)
	}
	frozenAgain, err := json.Marshal(legacyReload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frozen, frozenAgain) {
		t.Fatal("frozen V1 JSON changed across roundtrip")
	}
	result := UpgradeEnergyExplanationV1(legacy)
	for pass := 0; pass < 3; pass++ {
		if pass > 0 {
			result = epath120Reload(t, result)
		}
		if result.Quality == nil {
			t.Fatalf("pass%d V2 lacks canonical quality", pass)
		}
		if result.Quality.EndUseToCarrierClosedPct != 71.429 || result.Quality.EndUseToCarrierStatus != "overmapped" {
			t.Errorf("pass%d V2 should preserve weighted site closure for Electricity100/70 and Gas40/50: %#v", pass, result.Quality)
		}
		for _, period := range result.Periods {
			if period.Quality == nil {
				t.Errorf("pass%d period%s lacks period-local quality", pass, period.ID)
			}
		}
		if summary := buildEnergyExplanationSummaryV2(result); summary.Quality == nil || summary.Quality.EndUseToCarrierClosedPct != result.Quality.EndUseToCarrierClosedPct {
			t.Errorf("pass%d summary did not carry same quality", pass)
		}
	}
}

func epath130WeightedGraph(expected, incoming []float64) EnergyExplanationResult {
	result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}}
	services := []string{"cooling", "heating"}
	carriers := []string{"electricity", "natural_gas"}
	for i, want := range expected {
		service, carrier := services[i], carriers[i]
		loadID, driverID, endUseID, carrierID := "load."+service+".building", "driver.internal.people."+service+".building", "end_use."+service+".building", "carrier."+carrier+".building"
		result.Nodes = append(result.Nodes,
			EnergyExplanationNode{ID: loadID, Level: "load", ServiceKind: service, Value: want, Unit: "kWh", ScaleDomain: "thermal"},
			EnergyExplanationNode{ID: driverID, Level: "driver", DriverCategory: "internal.people", ServiceKind: service, Value: incoming[i], AllocatedValue: incoming[i], AllocationApplied: true, Unit: "kWh", ScaleDomain: "thermal"},
			EnergyExplanationNode{ID: endUseID, Level: "end_use", EndUse: service, Value: incoming[i], Unit: "kWh", ScaleDomain: "site"},
			EnergyExplanationNode{ID: carrierID, Level: "carrier", Carrier: carrier, Value: want, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", MeterHierarchyLevel: "facility_total"},
		)
		result.Links = append(result.Links,
			EnergyPathLink{ID: "driver." + service, FromID: driverID, ToID: loadID, Relation: "driver_to_load", ServiceKind: service, FromValue: incoming[i], ToValue: incoming[i], FromUnit: "kWh", ToUnit: "kWh"},
			EnergyPathLink{ID: "carrier." + carrier, FromID: endUseID, ToID: carrierID, Relation: "end_use_to_carrier", FromValue: incoming[i], ToValue: incoming[i], FromUnit: "kWh", ToUnit: "kWh"},
		)
	}
	return result
}
