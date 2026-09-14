package simulation

import (
	"database/sql"
	"encoding/json"
	"math"
	"testing"
)

func TestEnergyPathHVACConsumptionBuildingThermalIdentityMerge(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pairs [][2]string
		want  float64
	}{
		{name: "same load two carriers", pairs: [][2]string{{"electricity", "a"}, {"natural_gas", "a"}}, want: 40},
		{name: "distinct loads share one canonical endpoint", pairs: [][2]string{{"electricity", "a"}, {"natural_gas", "b"}}, want: 100},
		{name: "two distinct loads each shared by carriers", pairs: [][2]string{{"electricity", "a"}, {"natural_gas", "a"}, {"electricity", "b"}, {"natural_gas", "b"}}, want: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacy := EnergyExplanationV1{
				Schema: energyExplanationV1Schema,
				Nodes: []EnergyExplanationNode{
					{ID: "energy.carrier.electricity", Level: "energy", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "total"},
					{ID: "energy.carrier.natural_gas", Level: "energy", Value: 20, Unit: "kWh", Carrier: "natural_gas", EndUse: "total"},
					{ID: "energy.end_use.heating.electricity", Level: "energy", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "heating", SourceIDs: []string{"electricity"}},
					{ID: "energy.end_use.heating.natural_gas", Level: "energy", Value: 20, Unit: "kWh", Carrier: "natural_gas", EndUse: "heating", SourceIDs: []string{"gas"}},
					{ID: "load.heating.a", Level: "load", Value: 40, Unit: "kWh", ZoneName: "A", ServiceKind: "heating", SourceIDs: []string{"load-a"}},
					{ID: "load.heating.b", Level: "load", Value: 60, Unit: "kWh", ZoneName: "B", ServiceKind: "heating", SourceIDs: []string{"load-b"}},
					{ID: "load.heating.unserved", Level: "load", Value: 500, Unit: "kWh", ZoneName: "Unserved", ServiceKind: "heating", SourceIDs: []string{"load-unserved"}},
				},
				Edges: []EnergyExplanationEdge{
					{FromID: "energy.carrier.electricity", ToID: "energy.end_use.heating.electricity", Value: 10, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
					{FromID: "energy.carrier.natural_gas", ToID: "energy.end_use.heating.natural_gas", Value: 20, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
				},
			}
			for _, pair := range tc.pairs {
				value := 40.0
				if pair[1] == "b" {
					value = 60
				}
				legacy.Edges = append(legacy.Edges, EnergyExplanationEdge{FromID: "energy.end_use.heating." + pair[0], ToID: "load.heating." + pair[1], Value: value, Unit: "kWh", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "heating"})
			}
			// This legacy/non-monthly case also guards the site denominator:
			// deduplicating loadTotals alone would double its 30-kWh site side.
			result := UpgradeEnergyExplanationV1(legacy)
			assertMixedHeatingNode(t, result.Nodes, "load", "heating", 600)
			assertMixedHeatingNode(t, result.Nodes, "end_use", "heating", 30)
			count := 0
			for _, link := range result.Links {
				if link.Relation != "load_to_end_use" || link.ServiceKind != "heating" {
					continue
				}
				count++
				if link.FromValue != tc.want || link.ToValue != 30 || link.RatioKind != "load_to_site_energy" {
					t.Errorf("exact legacy-load identity was confused with the canonical total or carrier count: %+v, want %g -> 30", link, tc.want)
				}
				allowedLoads := map[string]bool{}
				for _, pair := range tc.pairs {
					allowedLoads["load-"+pair[1]] = true
				}
				seenLoads := map[string]bool{}
				for _, id := range link.SourceIDs {
					if id == "electricity" || id == "gas" {
						continue
					}
					if !allowedLoads[id] {
						t.Errorf("conversion borrowed unserved canonical load source %s", id)
					}
					seenLoads[id] = true
				}
				if len(seenLoads) != len(allowedLoads) {
					t.Errorf("conversion lost served load source: got %v want %v", seenLoads, allowedLoads)
				}
			}
			if count != 1 {
				t.Errorf("conversion count=%d, want one canonical pair", count)
			}
		})
	}
}

// The purpose bundle applies the v1 service-path preallocator before upgrading.
// Its Building sidecar has one full-meter allocation per carrier, even where
// those carriers share the same physical Zone load. Testing only enrich ->
// Upgrade misses this path. PLENUM remains a measured Building load but is not
// an equipment-served allocation recipient and must not enter the conversion.
func TestEnergyPathHVACConsumptionMixedBuildingConversionSingleThermalEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name      string
		plenum    float64
		zeroGasM2 bool
	}{
		{name: "original synthetic plenum", plenum: 99},
		{name: "unserved load exceeds duplicated served subtotal", plenum: 999},
		{name: "carrier disappears in second month", plenum: 999, zeroGasM2: true},
	} {
		for _, reverse := range []bool{false, true} {
			order := "forward"
			if reverse {
				order = "reversed"
			}
			t.Run(tc.name+"/"+order, func(t *testing.T) {
				path, input, _, plan, context := mixedHeatingConsumptionSQLFixture(t)
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`UPDATE ReportData SET Value=? * TimeIndex * 3600000 WHERE ReportDataDictionaryIndex=106`, tc.plenum); err != nil {
					db.Close()
					t.Fatal(err)
				}
				if tc.zeroGasM2 {
					// Keep complete native observations: this is known zero, not a
					// missing gas alias or month. Electricity remains 100 in M2.
					if _, err := db.Exec(`UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex IN (3,4) AND TimeIndex=2`); err != nil {
						db.Close()
						t.Fatal(err)
					}
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
				if err != nil {
					t.Fatal(err)
				}
				legacy = enrichEnergyExplanationWithServicePaths(legacy, input)
				legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(legacy)
				if len(legacy.buildingHVACAllocationEdges) == 0 || len(legacy.buildingHVACAllocationPeriodEdges["m1"]) == 0 || len(legacy.buildingHVACAllocationPeriodEdges["m2"]) == 0 {
					t.Fatal("fixture did not exercise the actual purpose-pipeline Building preallocation sidecar")
				}
				if reverse {
					reverseEdges := func(edges []EnergyExplanationEdge) {
						for left, right := 0, len(edges)-1; left < right; left, right = left+1, right-1 {
							edges[left], edges[right] = edges[right], edges[left]
						}
					}
					reverseEdges(legacy.buildingHVACAllocationEdges)
					for _, edges := range legacy.buildingHVACAllocationPeriodEdges {
						reverseEdges(edges)
					}
				}
				result := UpgradeEnergyExplanationV1(legacy)
				for reload := 0; reload < 3; reload++ {
					checkMixedBuildingConversionEndpoints(t, result, tc.plenum, tc.zeroGasM2)
					if reload == 2 {
						break
					}
					wire, err := json.Marshal(result)
					if err != nil {
						t.Fatal(err)
					}
					var reopened EnergyExplanationResult
					if err := json.Unmarshal(wire, &reopened); err != nil {
						t.Fatal(err)
					}
					result = reopened
				}
			})
		}
	}
}

func checkMixedBuildingConversionEndpoints(t *testing.T, result EnergyExplanationResult, plenum float64, zeroGasM2 bool) {
	t.Helper()
	periods := append([]EnergyPeriod{{ID: "root annual", Nodes: result.Nodes, Links: result.Links}}, result.Periods...)
	seen := map[string]int{}
	for _, period := range periods {
		var factor, gas float64
		switch period.ID {
		case "M1":
			factor, gas = 1, 100
		case "M2":
			factor, gas = 2, 200
			if zeroGasM2 {
				gas = 0
			}
		case "root annual", "annual":
			factor, gas = 3, 300
			if zeroGasM2 {
				gas = 100
			}
		default:
			continue
		}
		seen[period.ID]++
		wantThermal := 500 * factor // Five distinct 100-kWh Zone loads, once each.
		wantElectric := 50 * factor
		wantSite := wantElectric + gas
		assertMixedHeatingNode(t, period.Nodes, "load", "heating", (500+plenum)*factor)
		assertMixedHeatingNode(t, period.Nodes, "end_use", "heating", wantSite)
		assertMixedHeatingNode(t, period.Nodes, "carrier", "electricity", wantElectric)
		if gas > 0 {
			assertMixedHeatingNode(t, period.Nodes, "carrier", "natural_gas", gas)
		}
		conversionCount := 0
		carrierCount := map[string]int{}
		carrierValue := map[string]float64{}
		for _, link := range period.Links {
			if link.Relation == "load_to_end_use" && link.ServiceKind == "heating" {
				conversionCount++
				servedSources := map[string]bool{"sql-rdd-101": false, "sql-rdd-102": false, "sql-rdd-103": false, "sql-rdd-104": false, "sql-rdd-105": false}
				for _, id := range link.SourceIDs {
					if _, served := servedSources[id]; served {
						servedSources[id] = true
					} else if id != "sql-rdd-2" && id != "sql-rdd-4" {
						t.Errorf("%s conversion borrowed an unserved source %s", period.ID, id)
					}
				}
				for id, present := range servedSources {
					if !present {
						t.Errorf("%s conversion lost served thermal source %s", period.ID, id)
					}
				}
				if link.FromID != "load.heating.building" || link.ToID != "end_use.heating.building" ||
					math.Abs(link.FromValue-wantThermal) > 1e-9 || math.Abs(link.ToValue-wantSite) > 1e-9 ||
					link.FromUnit != "kWh" || link.ToUnit != "kWh" || link.Basis != "service_path_allocation" ||
					link.RatioKind != "load_to_site_energy" || math.Abs(link.Ratio-math.Round(wantThermal/wantSite*1000)/1000) > 1e-9 {
					t.Errorf("%s Building conversion counted a shared load per carrier or borrowed unserved load: %+v; want %g thermal -> %g site", period.ID, link, wantThermal, wantSite)
				}
			}
			if link.FromID != "end_use.heating.building" || (link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier") {
				continue
			}
			carrierCount[link.ToID]++
			carrierValue[link.ToID] += link.ToValue
			if math.Abs(link.FromValue-link.ToValue) > 1e-9 {
				t.Errorf("%s carrier split lost equal site-energy endpoints: %+v", period.ID, link)
			}
			wantSource := "sql-rdd-2"
			if link.ToID == "carrier.natural_gas.building" {
				wantSource = "sql-rdd-4"
			}
			if len(link.SourceIDs) != 1 || link.SourceIDs[0] != wantSource {
				t.Errorf("%s carrier split borrowed another carrier or thermal source: %+v", period.ID, link)
			}
		}
		if conversionCount != 1 {
			t.Errorf("%s heating conversion count=%d, want one physical thermal endpoint", period.ID, conversionCount)
		}
		if carrierCount["carrier.electricity.building"] != 1 || math.Abs(carrierValue["carrier.electricity.building"]-wantElectric) > 1e-9 {
			t.Errorf("%s electricity split changed: count=%d value=%g want=%g", period.ID, carrierCount["carrier.electricity.building"], carrierValue["carrier.electricity.building"], wantElectric)
		}
		wantGasCount := 1
		if gas == 0 {
			wantGasCount = 0
		}
		if carrierCount["carrier.natural_gas.building"] != wantGasCount || math.Abs(carrierValue["carrier.natural_gas.building"]-gas) > 1e-9 {
			t.Errorf("%s gas split changed: count=%d value=%g want count=%d value=%g", period.ID, carrierCount["carrier.natural_gas.building"], carrierValue["carrier.natural_gas.building"], wantGasCount, gas)
		}
	}
	for _, period := range []string{"root annual", "annual", "M1", "M2"} {
		if seen[period] != 1 {
			t.Errorf("%s graph count=%d, want exactly one", period, seen[period])
		}
	}
}
