package simulation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func energyPathQualityAllocationContextFixture() EnergyExplanationResult {
	result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}
	for _, name := range []string{"Office", "Lab", "Plenum"} {
		result.ZoneResults = append(result.ZoneResults, EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: name}})
	}
	for month := 0; month <= 12; month++ {
		period, fan, pump, cooling, heating := "annual", 100.0, 100.0, 400.0, 400.0
		if month > 0 {
			period, fan, pump, cooling, heating = fmt.Sprintf("M%d", month), 10, 0, 30, 60
			if month == 10 {
				fan, pump, cooling, heating = 40, 60, 100, 100
			}
			if month == 11 {
				fan, pump, cooling, heating = 80, 20, 50, 50
			}
		}
		rows := []EnergyReconciliation{}
		for _, item := range []struct {
			family                          string
			expected, allocated, unassigned float64
		}{
			{"zone_auxiliary_allocation.fans.electricity", fan, fan, 0},
			{"zone_auxiliary_allocation.pumps.electricity", pump, 0, pump},
			{"zone_hvac_allocation.cooling.electricity", cooling, cooling, 0},
			{"zone_hvac_allocation.heating.natural_gas", heating, heating, 0},
		} {
			rows = append(rows, EnergyReconciliation{ID: "reconcile." + item.family + "." + strings.ToLower(period), Level: "allocation", Period: period, Unit: "kWh", ExpectedValue: item.expected, AllocatedValue: item.allocated, UnassignedValue: item.unassigned})
		}
		if month == 0 {
			result.Reconciliation = append([]EnergyReconciliation(nil), rows...)
		}
		result.Periods = append(result.Periods, EnergyPeriod{ID: period, Reconciliation: append([]EnergyReconciliation(nil), rows...), Summary: &EnergyExplanationSummary{}})
		for index := range result.ZoneResults {
			zone := &result.ZoneResults[index]
			// The actual bug's shape: auxiliary records repeated in the Zone,
			// but Building HVAC allocation records intentionally are not.
			localRows := append([]EnergyReconciliation(nil), rows[:2]...)
			nodes := []EnergyExplanationNode{{ID: "local-carrier", Level: "carrier", Carrier: "electricity", Value: 5, Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", ZoneName: zone.Scope.ZoneName, Period: period}}
			if month == 0 {
				zone.Reconciliation, zone.Nodes = localRows, nodes
			}
			zone.Periods = append(zone.Periods, EnergyPeriod{ID: period, Nodes: nodes, Reconciliation: localRows, Summary: &EnergyExplanationSummary{}})
		}
	}
	return result
}

func energyPathQualityAllocationContextGraphBytes(t *testing.T, result EnergyExplanationResult) []byte {
	t.Helper()
	type graph struct {
		Nodes []EnergyExplanationNode
		Links []EnergyPathLink
		Rows  []EnergyReconciliation
	}
	graphs := []graph{{result.Nodes, result.Links, result.Reconciliation}}
	for _, period := range result.Periods {
		graphs = append(graphs, graph{period.Nodes, period.Links, period.Reconciliation})
	}
	for _, zone := range result.ZoneResults {
		graphs = append(graphs, graph{zone.Nodes, zone.Links, zone.Reconciliation})
		for _, period := range zone.Periods {
			graphs = append(graphs, graph{period.Nodes, period.Links, period.Reconciliation})
		}
	}
	data, err := json.Marshal(graphs)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestEnergyPathQualityZoneAllocationUsesExactBuildingPeriod(t *testing.T) {
	result := energyPathQualityAllocationContextFixture()
	before := energyPathQualityAllocationContextGraphBytes(t, result)
	if !refreshEnergyPathQuality(&result) {
		t.Fatal("first quality refresh did not initialize metrics")
	}
	assert := func(q *EnergyPathQuality, period string, zone bool) {
		t.Helper()
		assigned, unassigned, status := 100.0, 0.0, "complete"
		switch period {
		case "annual":
			assigned, unassigned, status = 90, 10, "partial" // (100+400+400)/1000, not aux100/200.
		case "M10":
			assigned, unassigned, status = 80, 20, "partial" // (40+100+100)/300, not aux40/100.
		case "M11":
			assigned, unassigned, status = 90, 10, "partial" // (80+50+50)/200, not aux80/100.
		}
		if q == nil || q.ZoneAllocatedPct != assigned || q.UnassignedPct != unassigned || q.ZoneAllocationStatus != status {
			t.Fatalf("%s zone=%v independent whole-Building allocation expected %g/%g %s, got %#v", period, zone, assigned, unassigned, status, q)
		}
		if zone && (q.EndUseToCarrierStatus != "partial" || q.EndUseToCarrierClosedPct != 0 || q.Ratios.Status != "not_applicable") {
			t.Fatalf("Building allocation copy replaced Zone-specific quality: %#v", q)
		}
	}
	assert(result.Quality, "annual", false)
	for _, period := range result.Periods {
		assert(period.Quality, period.ID, false)
		assert(period.Summary.Quality, period.ID, false)
	}
	for _, zone := range result.ZoneResults {
		assert(zone.Quality, "annual", true)
		assert(zone.Summary.Quality, "annual", true)
		for _, period := range zone.Periods {
			assert(period.Quality, period.ID, true)
			assert(period.Summary.Quality, period.ID, true)
		}
	}
	if after := energyPathQualityAllocationContextGraphBytes(t, result); string(before) != string(after) {
		t.Fatal("quality refresh changed physical graphs or allocation observations")
	}
	if refreshEnergyPathQuality(&result) {
		t.Fatal("same-context refresh is not idempotent")
	}
}

func TestEnergyPathQualityZoneAllocationCannotBorrowAnnualForAbsentMonth(t *testing.T) {
	result := energyPathQualityAllocationContextFixture()
	periods := []EnergyPeriod{}
	for _, period := range result.Periods {
		if period.ID != "M10" {
			periods = append(periods, period)
		}
	}
	result.Periods = periods
	refreshEnergyPathQuality(&result)
	for _, zone := range result.ZoneResults {
		for _, period := range zone.Periods {
			if period.ID == "M10" {
				if period.Quality.ZoneAllocationStatus != "unavailable" || period.Quality.ZoneAllocatedPct != 0 || period.Quality.UnassignedPct != 0 || !reflect.DeepEqual(period.Quality, period.Summary.Quality) {
					t.Fatal("missing Building month borrowed Annual or Zone auxiliary denominator")
				}
			} else if period.ID == "M11" && period.Quality.ZoneAllocatedPct != 90 {
				t.Fatal("missing sibling month changed a valid Building period")
			}
		}
	}
}

func TestEnergyPathQualityZoneAllocationZeroAndOvermappedStatus(t *testing.T) {
	result := energyPathQualityAllocationContextFixture()
	for index := range result.Periods {
		period := &result.Periods[index]
		if period.ID == "M1" {
			period.Reconciliation = nil
		}
		if period.ID == "M2" {
			period.Reconciliation = []EnergyReconciliation{{ID: "reconcile.zone_hvac_allocation.cooling.electricity.m2", Level: "allocation", Period: "M2", Unit: "kWh", ExpectedValue: 100, DirectValue: 20, AllocatedValue: 80.001}}
		}
	}
	refreshEnergyPathQuality(&result)
	for _, zone := range result.ZoneResults {
		for _, period := range zone.Periods {
			if period.ID == "M1" && (period.Quality.ZoneAllocationStatus != "unavailable" || period.Quality.ZoneAllocatedPct != 0 || period.Quality.UnassignedPct != 0) {
				t.Fatal("known-empty Building allocation inherited Zone subset")
			}
			if period.ID == "M2" && (period.Quality.ZoneAllocationStatus != "overmapped" || period.Quality.ZoneAllocatedPct != 100.001 || period.Quality.UnassignedPct != 0) {
				t.Fatal("Building overmapped evidence was capped or recomputed from Zone subset")
			}
		}
	}
}
