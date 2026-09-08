package simulation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func vrfProjectionLoad(zone, service, period string, value float64) EnergyExplanationNode {
	return EnergyExplanationNode{ID: "load." + service + "." + metricID(zone), Level: "load", Kind: "load." + service, ServiceKind: service,
		Value: value, EffectiveValue: value, Unit: "kWh", ScaleDomain: "thermal", ZoneName: zone, Period: period, SourceIDs: []string{"load/" + zone + "/" + service}}
}

func vrfProjectionFind(nodes []EnergyExplanationNode, id string) *EnergyExplanationNode {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
	}
	return nil
}

func vrfProjectionLinks(links []EnergyPathLink, relation, endUseID string) []EnergyPathLink {
	out := []EnergyPathLink{}
	for _, link := range links {
		if link.Relation == relation && (link.FromID == endUseID || link.ToID == endUseID) {
			out = append(out, link)
		}
	}
	return out
}

func TestEnergyPathVRFProjectionSourceBudgetsAndSingleConversion(t *testing.T) {
	cohort, loads := vrfAllocationFixture(t, 3)
	native := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads)
	before, _ := json.Marshal(native)
	display := energyPathVRFDisplayPlan(native)
	after, _ := json.Marshal(native)
	if string(before) != string(after) {
		t.Fatal("display plan changed original unrounded observations")
	}
	for _, role := range []string{"cooling.vrf.outdoor_electricity", "cooling.vrf.crankcase_electricity", "heating.vrf.outdoor_electricity", "heating.vrf.defrost_electricity"} {
		for month := 1; month <= 3; month++ {
			total, observed := 0.0, -1.0
			for _, source := range display.Sources {
				if source.Month == month && source.Target.Definition.ID == role {
					total += source.AllocatedValue
					observed = source.ObservedValue
				}
			}
			vrfAllocationNear(t, total, roundedEnergyNumber(observed))
		}
	}
	for month := 1; month <= 3; month++ {
		for zone := 1; zone <= 5; zone++ {
			name, period := fmt.Sprintf("SPACE%d-1", zone), fmt.Sprintf("M%d", month)
			scope := normalizeEnergyExplanationScope(EnergyExplanationScope{Kind: "zone", ZoneName: name})
			load := vrfProjectionLoad(name, "cooling", period, float64(10*zone*month))
			nodes, links := projectEnergyPathVRFZoneGraph([]EnergyExplanationNode{load}, nil, display, scope, period, true)
			endID := "end_use.cooling." + metricID(name)
			endUse := vrfProjectionFind(nodes, endID)
			want := vrfAllocationZone(t, display, "cooling", name, month)
			if endUse == nil || endUse.Basis != "service_path_allocation" || endUse.ZoneName != name || endUse.Period != period {
				t.Fatalf("missing allocated VRF subtotal/context: %#v", endUse)
			}
			vrfAllocationNear(t, endUse.Value, want.TotalValue)
			conversions := vrfProjectionLinks(links, "load_to_end_use", endID)
			if len(conversions) != 1 {
				t.Fatalf("service has %d conversions instead of one", len(conversions))
			}
			vrfAllocationNear(t, conversions[0].FromValue, load.Value)
			vrfAllocationNear(t, conversions[0].ToValue, endUse.Value)
			carrier := vrfProjectionLinks(links, "end_use_to_carrier", endID)
			if len(carrier) != 1 || len(carrier[0].SourceIDs) != 3 {
				t.Fatalf("consumption-only source trace lost: %#v", carrier)
			}
			for _, sourceID := range carrier[0].SourceIDs {
				if strings.HasPrefix(sourceID, "load/") {
					t.Fatal("load weights claimed to measure carrier consumption")
				}
			}
		}
	}
}

func TestEnergyPathVRFProjectionPreservesExistingDirectCarrierBasis(t *testing.T) {
	cohort, loads := vrfAllocationFixture(t, 1)
	display := energyPathVRFDisplayPlan(buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads))
	scope := normalizeEnergyExplanationScope(EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE1-1"})
	for _, basis := range []string{"direct_zone_energy", "service_path_allocation"} {
		t.Run(basis, func(t *testing.T) {
			carrier := EnergyExplanationNode{ID: "carrier.electricity.space1_1", Level: "carrier", Carrier: "electricity", Value: 5,
				EffectiveValue: 5, AllocatedValue: 5, Unit: "kWh", ScaleDomain: "site", Period: "M1", ZoneName: scope.ZoneName,
				Basis: basis, AggregationBasis: "model_total", SourceIDs: []string{"existing.lighting.source"}}
			nodes, _ := projectEnergyPathVRFZoneGraph([]EnergyExplanationNode{carrier}, nil, display, scope, "M1", true)
			got := vrfProjectionFind(nodes, carrier.ID)
			if got == nil || got.Basis != basis {
				t.Fatalf("VRF changed existing carrier evidence precedence: got %#v; want basis %s", got, basis)
			}
			cooling := vrfAllocationZone(t, display, "cooling", scope.ZoneName, 1)
			heating := vrfAllocationZone(t, display, "heating", scope.ZoneName, 1)
			vrfAllocationNear(t, got.Value, 5+cooling.TotalValue+heating.TotalValue)
			found := false
			for _, id := range got.SourceIDs {
				found = found || id == "existing.lighting.source"
			}
			if !found {
				t.Fatal("existing measured carrier contribution lost its source")
			}
		})
	}
}

func TestEnergyPathVRFProjectionPreservesOriginalNavigation(t *testing.T) {
	cohort, loads := vrfAllocationFixture(t, 1)
	display := energyPathVRFDisplayPlan(buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads))
	scope := normalizeEnergyExplanationScope(EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE1-1"})
	load := vrfProjectionLoad(scope.ZoneName, "cooling", "M1", 10)
	load.RelatedPathIDs = []string{"original.cooling.space1"}
	nodes, links := projectEnergyPathVRFZoneGraph([]EnergyExplanationNode{load}, nil, display, scope, "M1", true)
	endUse := vrfProjectionFind(nodes, "end_use.cooling.space1_1")
	if endUse == nil || !reflect.DeepEqual(endUse.RelatedPathIDs, load.RelatedPathIDs) {
		t.Fatalf("VRF consumption node lost selected original service path: %#v", endUse)
	}
	expectedEntities := []string{cohort.System.OutdoorUnit.ID}
	for _, terminal := range cohort.System.Terminals {
		if terminal.ZoneName == scope.ZoneName {
			expectedEntities = append(expectedEntities, terminal.Terminal.ID)
		}
	}
	for _, id := range expectedEntities {
		found := false
		for _, actual := range endUse.RelatedEntityIDs {
			found = found || actual == id
		}
		if !found {
			t.Fatalf("missing original physical component %s in node trace %#v", id, endUse.RelatedEntityIDs)
		}
	}
	for _, link := range vrfProjectionLinks(links, "end_use_to_carrier", endUse.ID) {
		if !reflect.DeepEqual(link.RelatedPathIDs, load.RelatedPathIDs) {
			t.Fatal("consumption ribbon lost exact original service path")
		}
	}
}

func TestEnergyPathVRFProjectionDisplayLedgerMatchesSourceBudgets(t *testing.T) {
	cohort, loads := vrfAllocationFixture(t, 1)
	vrfAllocationObservationFor(t, &cohort, "cooling.vrf.terminal_electricity", "SPACE2-1").Monthly[1] = 2.0006
	vrfAllocationObservationFor(t, &cohort, "cooling.vrf.terminal_electricity", "SPACE3-1").Monthly[1] = 3.0006
	vrfAllocationObservationFor(t, &cohort, "cooling.vrf.outdoor_electricity", "").Monthly[1] = 99.9994
	vrfAllocationObservationFor(t, &cohort, "cooling.vrf.crankcase_electricity", "").Monthly[1] = 9.9994
	native := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads)
	display := energyPathVRFDisplayPlan(native)
	for _, month := range []int{1, 0} {
		raw := vrfAllocationService(t, native, "cooling", month)
		ledger := vrfAllocationService(t, display, "cooling", month)
		vrfAllocationNear(t, raw.DirectValue, 14.0012)
		vrfAllocationNear(t, raw.SharedValue, 109.9988)
		vrfAllocationNear(t, ledger.DirectValue, 14.002)
		vrfAllocationNear(t, ledger.SharedValue, 109.998)
		vrfAllocationNear(t, ledger.AllocatedValue, 109.998)
		vrfAllocationNear(t, ledger.UnassignedValue, 0)
		// Both independently exact source sums and their completed 3dp budgets
		// close to this 124 kWh meter; do not create a bookkeeping-only warning.
		vrfAllocationNear(t, ledger.DirectValue+ledger.AllocatedValue, 124)
		zoneTotal := 0.0
		for _, row := range display.Zones {
			if row.ServiceKind == "cooling" && row.Month == month {
				zoneTotal += row.TotalValue
			}
		}
		vrfAllocationNear(t, ledger.DirectValue+ledger.AllocatedValue, zoneTotal)
	}
}

func TestEnergyPathVRFProjectionPartialDirectOnlyAndZero(t *testing.T) {
	for _, kind := range []string{"direct_only", "missing_local", "zero_total"} {
		t.Run(kind, func(t *testing.T) {
			cohort, loads := vrfAllocationFixture(t, 1)
			zone := "SPACE2-1"
			enabled := kind != "direct_only"
			if kind == "missing_local" {
				delete(vrfAllocationObservationFor(t, &cohort, "cooling.vrf.terminal_electricity", zone).Monthly, 1)
			}
			if kind == "zero_total" {
				vrfAllocationObservationFor(t, &cohort, "cooling.vrf.terminal_electricity", zone).Monthly[1] = 0
				vrfAllocationObservationFor(t, &cohort, "cooling.vrf.outdoor_electricity", "").Monthly[1] = 0
				vrfAllocationObservationFor(t, &cohort, "cooling.vrf.crankcase_electricity", "").Monthly[1] = 0
			}
			plan := energyPathVRFDisplayPlan(buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads))
			load := vrfProjectionLoad(zone, "cooling", "M1", 20)
			nodes, links := projectEnergyPathVRFZoneGraph([]EnergyExplanationNode{load}, nil, plan,
				normalizeEnergyExplanationScope(EnergyExplanationScope{Kind: "zone", ZoneName: zone}), "M1", enabled)
			endID := "end_use.cooling." + metricID(zone)
			if len(vrfProjectionLinks(links, "load_to_end_use", endID)) != 0 {
				t.Fatal("partial or known-zero service manufactured a conversion/ratio")
			}
			endUse := vrfProjectionFind(nodes, endID)
			if kind == "zero_total" {
				if endUse != nil {
					t.Fatal("known-zero consumption manufactured positive site data")
				}
			} else if endUse == nil || !reflect.DeepEqual(endUse.Badges, []string{"partial"}) {
				t.Fatalf("partial positive subtotal disappeared or lost badge: %#v", endUse)
			} else if kind == "direct_only" {
				vrfAllocationNear(t, endUse.Value, 2)
				if endUse.Basis != "direct_zone_energy" || endUse.AllocationApplied {
					t.Fatal("DirectOnly policy included a shared allocation")
				}
			}
		})
	}
}

func TestEnergyPathVRFProjectionScopeAndSiblingCarrierGuards(t *testing.T) {
	cohort, loads := vrfAllocationFixture(t, 1)
	plan := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads)
	load := vrfProjectionLoad("SPACE1-1", "heating", "M1", 50)
	base := []EnergyExplanationNode{load,
		{ID: "broadgas", Level: "energy", EndUse: "heating", Carrier: "natural_gas", Value: 100},
		{ID: "explicitloop", Level: "energy", EndUse: "heating", Carrier: "district_heating", LoopName: "Independent HW", Value: 100},
		{ID: "exactgas", Level: "energy", EndUse: "heating", Carrier: "natural_gas", ZoneName: "SPACE1-1", Value: 2},
	}
	edges := []EnergyExplanationEdge{{FromID: "broadgas", ToID: load.ID}, {FromID: "explicitloop", ToID: load.ID}, {FromID: "exactgas", ToID: load.ID}}
	for _, scope := range []EnergyExplanationScope{{Kind: "building"}, {Kind: "zone", ZoneName: "PLENUM-1"}, {Kind: "zone", ZoneName: "NON-VRF"}} {
		nodes, actual := energyPathVRFZoneLegacyInputs(base, edges, plan, scope)
		if !reflect.DeepEqual(nodes, base) || !reflect.DeepEqual(actual, edges) {
			t.Fatal("unowned/Building context changed")
		}
		projected, links := projectEnergyPathVRFZoneGraph(base, nil, plan, scope, "M1", true)
		if !reflect.DeepEqual(projected, base) || len(links) != 0 {
			t.Fatal("unowned context acquired VRF data")
		}
	}
	nodes, actual := energyPathVRFZoneLegacyInputs(base, edges, plan, EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE1-1"})
	if len(nodes) != 3 || len(actual) != 2 || vrfProjectionFind(nodes, "broadgas") != nil || vrfProjectionFind(nodes, "exactgas") == nil || vrfProjectionFind(nodes, "explicitloop") == nil {
		t.Fatalf("broad fallback leaked or explicit independent energy hidden: %#v %#v", nodes, actual)
	}
}

func TestEnergyPathVRFProjectionSQLAndSourceWireBoundary(t *testing.T) {
	doc := vrfSQLDocument(t)
	plan := vrfSQLPlan(doc)
	plan.AllocationPolicy = PurposeAllocationPolicyByServicePathLoadShare
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(vrfSQLFixture(t), &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
	if err != nil {
		t.Fatal(err)
	}
	legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(legacy)
	before, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(legacy)
	after, _ := json.Marshal(legacy)
	if string(before) != string(after) {
		t.Fatal("V2 projection mutated native/frozen V1 wire")
	}
	building := vrfProjectionFind(result.Nodes, "end_use.cooling.building")
	if building == nil {
		t.Fatal("missing original Building cooling meter")
	}
	vrfAllocationNear(t, building.Value, 124*78)
	for _, zone := range result.ZoneResults {
		name := zone.Scope.ZoneName
		if name == "PLENUM-1" {
			if vrfProjectionFind(zone.Nodes, "end_use.cooling."+metricID(name)) != nil {
				t.Fatal("unserved PLENUM inherited shared consumption")
			}
			continue
		}
		annual := vrfProjectionFind(zone.Nodes, "end_use.cooling."+metricID(name))
		if annual == nil {
			t.Fatalf("served VRF Zone %s has no cooling subtotal", name)
		}
		sum := 0.0
		months := 0
		for _, period := range zone.Periods {
			if period.Kind != "monthly" {
				continue
			}
			months++
			endUse := vrfProjectionFind(period.Nodes, annual.ID)
			if endUse == nil || len(vrfProjectionLinks(period.Links, "load_to_end_use", annual.ID)) != 1 {
				t.Fatalf("%s/%s missing subtotal or single conversion", name, period.ID)
			}
			sum += endUse.Value
		}
		if months != 12 {
			t.Fatalf("%s actual months=%d, want12", name, months)
		}
		vrfAllocationNear(t, annual.Value, sum)
	}
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Sources []map[string]json.RawMessage `json:"sources"`
	}
	if err := json.Unmarshal(wire, &raw); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, source := range raw.Sources {
		var id string
		_ = json.Unmarshal(source["id"], &id)
		if id != "sql-rdd-102" && id != "sql-rdd-201" {
			continue
		}
		seen[id] = true
		if source["rawValue"] == nil || source["effectiveValue"] == nil {
			t.Fatalf("observed Building original source %s lost known scalar", id)
		}
		var details []map[string]json.RawMessage
		_ = json.Unmarshal(source["scopeDetails"], &details)
		found := 0
		for _, detail := range details {
			var scope EnergyExplanationScope
			_ = json.Unmarshal(detail["scope"], &scope)
			if scope.Kind != "zone" || scope.ZoneName != "SPACE1-1" {
				continue
			}
			found++
			if id == "sql-rdd-201" {
				if detail["rawValue"] != nil || detail["effectiveValue"] != nil || detail["allocatedValue"] == nil {
					t.Fatalf("shared pool claimed measured Zone energy: %s", detail)
				}
			} else if string(detail["rawValue"]) != "0" || string(detail["effectiveValue"]) != "0" {
				t.Fatalf("actual local known zero became unknown: %s", detail)
			}
		}
		if found != 1 {
			t.Fatalf("%s owning scope traces=%d, want1", id, found)
		}
	}
	if len(seen) != 2 {
		t.Fatal("original consumption sources disappeared")
	}
}
