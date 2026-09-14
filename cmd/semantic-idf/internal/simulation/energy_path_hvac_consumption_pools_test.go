package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"
)

func TestEnergyPathHVACConsumptionPoolsLocalAndCentralShare(t *testing.T) {
	nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
	baseline := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology)
	if baseline.Records[0].DirectValue != 40 || len(baseline.Edges) != 3 {
		t.Fatalf("control no longer exercises whole-Zone direct exclusion: %#v", baseline)
	}
	before := epathHVACConsumptionPoolPlanSnapshot(baseline)
	got := reserveEnergyPathHVACConsumptionPools(baseline, nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
	epathHVACConsumptionPoolAssert(t, got, 50, 40, 10, 0, 0, map[string]float64{"SPACE1-1": 2, "SPACE2-1": 2, "SPACE3-1": 2, "SPACE4-1": 2, "SPACE5-1": 2})
	if !reflect.DeepEqual(before, epathHVACConsumptionPoolPlanSnapshot(baseline)) {
		t.Fatal("reservation mutated the original generic plan")
	}
	totals := map[string]float64{"SPACE2-1": 10, "SPACE4-1": 30}
	for _, edge := range got.Edges {
		totals[edge.ZoneName] += edge.Value
		if edge.RuleID != energyRelationshipRuleAllocatedHVACConsumptionPool || edge.Basis != "service_path_allocation" || !stringSliceContains(edge.SourceIDs, "sql.boiler.ancillary") || !stringSliceContains(edge.SourceIDs, "sql.meter.heating.electricity") || !stringSliceContains(edge.SourceIDs, "sql.load."+edge.ZoneName) {
			t.Errorf("shared edge lacks exact constituent/meter/load evidence: %#v", edge)
		}
		if stringSliceContains(edge.SourceIDs, "sql.baseboard.2") || stringSliceContains(edge.SourceIDs, "sql.baseboard.4") || stringSliceContains(edge.SourceIDs, "sql.load.PLENUM-1") {
			t.Errorf("shared edge inherited another consumer's electricity or an unserved load: %#v", edge)
		}
	}
	for i, want := range []float64{2, 12, 2, 32, 2} {
		if got := totals[fmt.Sprintf("SPACE%d-1", i+1)]; got != want {
			t.Errorf("Zone%d direct + source-local share = %g, want %g", i+1, got, want)
		}
	}
}

// BuildPurposeResultBundle first publishes a generic v1 allocation and only
// then upgrades it. A constituent reservation must replace that intermediate
// graph just as it replaces the initial delivered-load graph. In particular,
// no electric edge is evidence of no allocatable observed electric constituent,
// not permission to reconstruct an electric budget from a sibling gas share.
func TestEnergyPathHVACConsumptionPoolsSurviveV1Preallocation(t *testing.T) {
	for _, scenario := range []string{"positive shared", "observed zero shared", "absent shared"} {
		t.Run(scenario, func(t *testing.T) {
			nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
			gas := epath100AuditEndUseNode("heating", "natural_gas", 100, "M1", "sql.meter.heating.gas", pool.Members[2].RelatedPathIDs)
			nodes = append(nodes, gas)
			wantShared := 10.0
			wantSourceRows := 5
			switch scenario {
			case "observed zero shared":
				pool.Members[2].Series.Monthly[1] = 0
				wantShared = 0
			case "absent shared":
				delete(pool.Members[2].Series.Monthly, 1)
				wantShared, wantSourceRows = 0, 0
			}
			legacy := EnergyExplanationV1{
				Schema: energyExplanationV1Schema, AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
				Nodes: nodes, Periods: []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: nodes}},
				canonicalMonthlyBasis: true, zoneDirectUseSeries: direct, servicePathIndex: topology,
				hvacConsumptionPools: []energyPathHVACConsumptionPool{pool},
			}
			legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(legacy)
			period := legacy.Periods[0]
			preallocatedElectric, preallocatedGas := 0.0, 0.0
			for _, edge := range period.Edges {
				switch edge.FromID {
				case nodes[0].ID:
					preallocatedElectric += edge.Value
				case gas.ID:
					preallocatedGas += edge.Value
				}
			}
			if roundedEnergyNumber(preallocatedElectric) != 10 || roundedEnergyNumber(preallocatedGas) != 100 {
				t.Fatalf("control skipped the real v1 generic preallocation: electric=%g gas=%g", preallocatedElectric, preallocatedGas)
			}
			plan := buildEnergyPathZoneHVACAllocationPlan(period.Nodes, period.Edges, legacy.zoneDirectUseSeries, period.ID, period.Kind, legacy.canonicalMonthlyBasis, legacy.servicePathIndex)
			plan = reserveEnergyPathHVACConsumptionPools(plan, period.Nodes, legacy.servicePathIndex, legacy.hvacConsumptionPools, period.ID, period.Kind, legacy.canonicalMonthlyBasis)
			if !plan.ConsumptionPoolGroups[energyPathZoneHVACAllocationGroupKey("heating", "electricity")] || len(plan.ConsumptionSourceAllocations) != wantSourceRows {
				t.Fatalf("v1 preallocation lost the bound pool or observed-zero presence: %#v", plan)
			}
			for _, row := range plan.ConsumptionSourceAllocations {
				if row.ObservedValue != wantShared || row.AllocatedValue != wantShared/5 {
					t.Errorf("source-local row changed its actual observation or exact five-Zone allocation: %#v", row)
				}
			}
			var electricRecord *energyPathZoneHVACAllocationRecord
			for i := range plan.Records {
				if plan.Records[i].Carrier == "electricity" {
					electricRecord = &plan.Records[i]
				}
			}
			if electricRecord == nil || electricRecord.ExpectedValue != 50 || electricRecord.DirectValue != 40 || electricRecord.AllocatedValue != wantShared || electricRecord.UnassignedValue != 10-wantShared {
				t.Fatalf("intermediate broad allocation replaced the native constituent ledger: %#v", electricRecord)
			}
			applied := applyEnergyPathZoneHVACAllocationPlan(period.Edges, period.Nodes, plan)
			electricEdges, gasEdges := 0, 0
			for _, edge := range applied {
				switch edge.FromID {
				case nodes[0].ID:
					electricEdges++
					if edge.RuleID != energyRelationshipRuleAllocatedHVACConsumptionPool || edge.Value != wantShared/5 || edge.Value <= 0 {
						t.Errorf("stale generic electric allocation survived native reservation: %#v", edge)
					}
				case gas.ID:
					gasEdges++
					if edge.Value != 20 || edge.RuleID == energyRelationshipRuleAllocatedHVACConsumptionPool || edge.ZoneName == "PLENUM-1" {
						t.Errorf("electric reservation changed the independent original five-Zone gas allocation: %#v", edge)
					}
				}
			}
			wantElectricEdges := 0
			if wantShared > 0 {
				wantElectricEdges = 5
			}
			if electricEdges != wantElectricEdges || gasEdges != 5 {
				t.Errorf("electric/gas edge counts=%d/%d, want %d/5", electricEdges, gasEdges, wantElectricEdges)
			}
		})
	}
}

func TestEnergyPathHVACConsumptionPoolsV1UpgradeCannotReconstructSiblingCarrier(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		shared      float64
		known       bool
	}{
		{name: "positive shared", shared: 10, known: true},
		{name: "observed zero shared", query: `UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=52`, known: true},
		{name: "NULL shared", query: `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=52`},
		{name: "absent shared", query: `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=52`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, input, _, plan, context := mixedHeatingConsumptionSQLFixture(t)
			if tc.query != "" {
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(tc.query); err != nil {
					db.Close()
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
			legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(enrichEnergyExplanationWithServicePaths(legacy, input))
			if len(legacy.buildingHVACAllocationPeriodEdges["m1"]) == 0 {
				t.Fatal("fixture omitted actual purpose-pipeline v1 preallocation")
			}
			result := UpgradeEnergyExplanationV1(legacy)
			check := func(result EnergyExplanationResult) {
				t.Helper()
				zoneCount := 0
				for _, zone := range result.ZoneResults {
					zoneCount++
					owner := 0
					fmt.Sscanf(zone.Scope.ZoneName, "SPACE%d-1", &owner)
					served := owner >= 1 && owner <= 5
					if !served && zone.Scope.ZoneName != "PLENUM-1" {
						t.Fatalf("native component became a fictitious Zone: %s", zone.Scope.ZoneName)
					}
					periods := append([]EnergyPeriod{{ID: "annual", Nodes: zone.Nodes, Links: zone.Links}}, zone.Periods...)
					for _, period := range periods {
						factor := 0.0
						switch period.ID {
						case "annual":
							factor = 3
						case "M1":
							factor = 1
						case "M2":
							factor = 2
						default:
							continue
						}
						wantLocal, wantShared, wantGas := 0.0, 0.0, 0.0
						if served {
							wantLocal = []float64{0, 10, 0, 30, 0}[owner-1] * factor
							wantShared, wantGas = tc.shared*factor/5, 20*factor
						}
						carrierByID := map[string]string{}
						values := map[string]float64{}
						counts := map[string]int{}
						for _, node := range period.Nodes {
							if node.Level == "carrier" {
								carrierByID[node.ID] = node.Carrier
								values[node.Carrier] += node.Value
								counts[node.Carrier]++
							}
						}
						for carrier, want := range map[string]float64{"electricity": wantLocal + wantShared, "natural_gas": wantGas} {
							if math.Abs(values[carrier]-want) > 1e-9 || counts[carrier] > 1 || want > 0 && counts[carrier] != 1 {
								t.Errorf("%s/%s %s=%g (%d nodes), want %g; absent constituent must not borrow a sibling carrier's share", zone.Scope.ZoneName, period.ID, carrier, values[carrier], counts[carrier], want)
							}
						}
						var local, shared, gas float64
						for _, link := range period.Links {
							if link.Relation != "end_use_to_carrier" {
								continue
							}
							switch carrierByID[link.ToID] {
							case "electricity":
								if link.Basis == "direct_zone_energy" {
									local += link.ToValue
								} else {
									shared += link.ToValue
								}
							case "natural_gas":
								gas += link.ToValue
							}
						}
						if math.Abs(local-wantLocal) > 1e-9 || math.Abs(shared-wantShared) > 1e-9 || math.Abs(gas-wantGas) > 1e-9 {
							t.Errorf("%s/%s direct/shared/gas branches=%g/%g/%g, want %g/%g/%g", zone.Scope.ZoneName, period.ID, local, shared, gas, wantLocal, wantShared, wantGas)
						}
					}
				}
				if zoneCount != 6 {
					t.Errorf("Zone count=%d, want original five served plus return plenum", zoneCount)
				}
				seenSource := false
				for _, source := range result.Sources {
					if source.ID != "sql-rdd-52" {
						continue
					}
					seenSource = true
					if energyDataSourceValueKnown(source, energySourceObservedRaw) != tc.known || tc.known && source.RawValue != tc.shared*3 {
						t.Errorf("native Boiler source conflated absent with observed zero or replaced its raw quantity: %+v", source)
					}
					if !tc.known && (source.AllocationApplied || len(source.ScopeDetails) != 0) {
						t.Errorf("absent Boiler observation acquired a measured/allocation Zone row: %+v", source)
					}
					if tc.known && len(source.ScopeDetails) != 5 {
						t.Errorf("known Boiler observation lost its exact five-Zone allocation presence: %+v", source)
					}
					for _, detail := range source.ScopeDetails {
						exactPresence := detail.inspectorDecodedFromJSON || detail.inspectorScopedValuePresence
						if !detail.AllocationApplied || detail.AllocatedValue != tc.shared*3/5 || detail.RawValue != 0 || detail.EffectiveValue != 0 || detail.inspectorValuePresence&3 != 0 || !exactPresence {
							t.Errorf("allocated-only source row became measured Zone consumption: %+v", detail)
						}
					}
				}
				if tc.known && !seenSource {
					t.Error("actual native Boiler observation disappeared, including an observed zero")
				}
			}
			check(result)
			wire, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var reopened EnergyExplanationResult
			if err := json.Unmarshal(wire, &reopened); err != nil {
				t.Fatal(err)
			}
			check(reopened)
		})
	}
}

func TestEnergyPathHVACConsumptionPoolsUnknownIsNotCentralRemainder(t *testing.T) {
	for _, mutation := range []string{"missing shared month", "known zero shared", "negative shared", "tiny negative shared", "nonfinite shared", "wrong unit", "no source", "ambiguous source", "no source paths", "foreign source paths", "invalid original membership", "duplicate member", "duplicate source", "wrong carrier", "wrong component", "wrong output", "duplicate pool"} {
		t.Run(mutation, func(t *testing.T) {
			nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
			switch mutation {
			case "missing shared month":
				delete(pool.Members[2].Series.Monthly, 1)
			case "known zero shared":
				pool.Members[2].Series.Monthly[1] = 0
			case "negative shared":
				pool.Members[2].Series.Monthly[1] = -10
			case "tiny negative shared":
				pool.Members[2].Series.Monthly[1] = -0.0001
			case "nonfinite shared":
				pool.Members[2].Series.Monthly[1] = math.NaN()
			case "wrong unit":
				pool.Members[2].Series.Unit = "kWh thermal"
			case "no source":
				pool.Members[2].Series.SourceIDs, pool.Members[2].Series.MonthlySourceIDs = nil, nil
			case "ambiguous source":
				pool.Members[2].Series.MonthlySourceIDs = []string{"sql.boiler.ancillary", "sql.duplicate.ancillary"}
			case "no source paths":
				pool.Members[2].RelatedPathIDs = nil
			case "foreign source paths":
				pool.Members[2].RelatedPathIDs = []string{"unrelated.heating"}
			case "invalid original membership":
				pool.Valid = false
			case "duplicate member":
				pool.Members = append(pool.Members, pool.Members[2])
			case "duplicate source":
				pool.Members[2].Series.SourceIDs = []string{"sql.baseboard.2"}
				pool.Members[2].Series.MonthlySourceIDs = []string{"sql.baseboard.2"}
			case "wrong carrier":
				pool.Members[2].Series.Carrier = "natural_gas"
			case "wrong component":
				pool.Members[2].Series.SourceKey = "Unrelated Boiler"
			case "wrong output":
				pool.Members[2].Series.SourceName = "Boiler Ancillary NaturalGas Energy"
			}
			pools := []energyPathHVACConsumptionPool{pool}
			if mutation == "duplicate pool" {
				pools = append(pools, pool)
			}
			baseline := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology)
			got := reserveEnergyPathHVACConsumptionPools(baseline, nodes, topology, pools, "M1", "monthly", true)
			epathHVACConsumptionPoolAssert(t, got, 50, 40, 0, 10, 0, nil)
		})
	}
}

func TestEnergyPathHVACConsumptionPoolsMissingLocalKeepsKnownIndependentShared(t *testing.T) {
	nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
	delete(pool.Members[1].Series.Monthly, 1)
	direct = direct[:1]
	baseline := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology)
	got := reserveEnergyPathHVACConsumptionPools(baseline, nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
	epathHVACConsumptionPoolAssert(t, got, 50, 10, 10, 30, 0, map[string]float64{"SPACE1-1": 2, "SPACE2-1": 2, "SPACE3-1": 2, "SPACE4-1": 2, "SPACE5-1": 2})
}

func TestEnergyPathHVACConsumptionPoolsNilAndForeignEvidencePreserveLegacy(t *testing.T) {
	nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
	baseline := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology)
	for _, foreign := range []bool{false, true} {
		var pools []energyPathHVACConsumptionPool
		if foreign {
			pool.MeterSourceIDs = []string{"sql.some.other.meter"}
			pools = []energyPathHVACConsumptionPool{pool}
		}
		got := reserveEnergyPathHVACConsumptionPools(baseline, nodes, topology, pools, "M1", "monthly", true)
		if !reflect.DeepEqual(baseline, got) {
			t.Fatalf("nil/foreign source evidence changed existing allocation: foreign=%v", foreign)
		}
	}
}

func TestEnergyPathHVACConsumptionPoolsKnownZeroLocalDoesNotExcludeCentralRecipient(t *testing.T) {
	nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
	direct[0].Monthly[1], pool.Members[0].Series.Monthly[1] = 0, 0
	nodes[0].Value, nodes[0].EffectiveValue = 40, 40
	baseline := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology)
	got := reserveEnergyPathHVACConsumptionPools(baseline, nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
	epathHVACConsumptionPoolAssert(t, got, 40, 30, 10, 0, 0, map[string]float64{"SPACE1-1": 2, "SPACE2-1": 2, "SPACE3-1": 2, "SPACE4-1": 2, "SPACE5-1": 2})
}

func TestEnergyPathHVACConsumptionPoolsExactRecipientsAndEffectiveLoads(t *testing.T) {
	for _, scenario := range []string{"source subset", "zero local load", "effective load multiplier", "known recipients all zero", "missing topology"} {
		t.Run(scenario, func(t *testing.T) {
			nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
			want := map[string]float64{}
			allocated := 10.0
			switch scenario {
			case "source subset":
				pool.Members[2].RelatedPathIDs = []string{"heating.SPACE2-1", "heating.SPACE4-1"}
				want = map[string]float64{"SPACE2-1": 5, "SPACE4-1": 5}
			case "zero local load":
				nodes[2].Value, nodes[2].RawValue, nodes[2].EffectiveValue = 0, 0, 0
				want = map[string]float64{"SPACE1-1": 2.5, "SPACE3-1": 2.5, "SPACE4-1": 2.5, "SPACE5-1": 2.5}
			case "effective load multiplier":
				nodes[2].Value, nodes[2].EffectiveValue, nodes[2].Multiplier, nodes[2].RawValue = 6, 6, 6, 1
				want = map[string]float64{"SPACE1-1": 1, "SPACE2-1": 6, "SPACE3-1": 1, "SPACE4-1": 1, "SPACE5-1": 1}
			case "known recipients all zero":
				for i := 1; i <= 5; i++ {
					nodes[i].Value, nodes[i].RawValue, nodes[i].EffectiveValue = 0, 0, 0
				}
				allocated = 0
			case "missing topology":
				topology = energyServicePathIndex{}
				allocated = 0
			}
			baseline := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology)
			got := reserveEnergyPathHVACConsumptionPools(baseline, nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
			epathHVACConsumptionPoolAssert(t, got, 50, 40, allocated, 10-allocated, 0, want)
		})
	}
}

func TestEnergyPathHVACConsumptionPoolsMergeIndependentSharedSources(t *testing.T) {
	nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
	second := energyPathHVACConsumptionMember{ID: "boiler2.ancillary", ObjectType: "Boiler:HotWater", ObjectName: "Second Boiler", OutputName: "Boiler Ancillary Electricity Energy", RelatedPathIDs: []string{"heating.SPACE2-1"}, Series: epathHVACConsumptionPoolSeries("", "sql.boiler2.ancillary", 5)}
	pool.Members = append(pool.Members, second)
	nodes[0].Value, nodes[0].EffectiveValue = 55, 55
	baseline := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology)
	got := reserveEnergyPathHVACConsumptionPools(baseline, nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
	epathHVACConsumptionPoolAssert(t, got, 55, 40, 15, 0, 0, map[string]float64{"SPACE1-1": 2, "SPACE2-1": 7, "SPACE3-1": 2, "SPACE4-1": 2, "SPACE5-1": 2})
	for _, edge := range got.Edges {
		if stringSliceContains(edge.SourceIDs, "sql.boiler2.ancillary") != (edge.ZoneName == "SPACE2-1") {
			t.Errorf("source-local provenance escaped its recipient set: %#v", edge)
		}
	}
}

func TestEnergyPathHVACConsumptionPoolsConstituentBudgetsAndOvermapping(t *testing.T) {
	for _, amount := range []float64{0.003, 20} {
		nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
		pool.Members[2].Series.Monthly[1] = amount
		baseline := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology)
		got := reserveEnergyPathHVACConsumptionPools(baseline, nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
		want := map[string]float64{"SPACE1-1": 0.001, "SPACE2-1": 0.001, "SPACE3-1": 0.001}
		unassigned, overmapped := 9.997, 0.0
		if amount == 20 {
			want = map[string]float64{"SPACE1-1": 4, "SPACE2-1": 4, "SPACE3-1": 4, "SPACE4-1": 4, "SPACE5-1": 4}
			unassigned, overmapped = 0, 10
		}
		epathHVACConsumptionPoolAssert(t, got, 50, 40, amount, unassigned, overmapped, want)
	}
}

func TestEnergyPathHVACConsumptionPoolsAnnualIsCompletedMonthlySum(t *testing.T) {
	nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
	first := reserveEnergyPathHVACConsumptionPools(buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology), nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
	for i := range direct {
		direct[i].Monthly[2] = direct[i].Monthly[1]
	}
	for i := range pool.Members {
		pool.Members[i].Series.Monthly[2] = pool.Members[i].Series.Monthly[1]
	}
	for i := range nodes {
		nodes[i].Period = "M2"
	}
	nodes[1].Value, nodes[1].RawValue, nodes[1].EffectiveValue = 6, 6, 6
	second := reserveEnergyPathHVACConsumptionPools(buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M2", "monthly", true, topology), nodes, topology, []energyPathHVACConsumptionPool{pool}, "M2", "monthly", true)
	annual := aggregateEnergyPathZoneHVACAllocationPlans([]energyPathZoneHVACAllocationPlan{first, second})
	epathHVACConsumptionPoolAssert(t, annual, 100, 80, 20, 0, 0, map[string]float64{"SPACE1-1": 8, "SPACE2-1": 3, "SPACE3-1": 3, "SPACE4-1": 3, "SPACE5-1": 3})
	for _, edge := range annual.Edges {
		if edge.RuleID != energyRelationshipRuleAllocatedHVACConsumptionPool {
			t.Errorf("monthly aggregation lost the exact source-local projection guard: %#v", edge)
		}
	}
	if !annual.ConsumptionPoolGroups[energyPathZoneHVACAllocationGroupKey("heating", "electricity")] || len(annual.ConsumptionSourceAllocations) != 5 {
		t.Fatalf("annual aggregation dropped the private constituent boundary: %#v", annual)
	}
	for _, row := range annual.ConsumptionSourceAllocations {
		want := 3.0
		if row.ZoneName == "SPACE1-1" {
			want = 8
		}
		if row.Period != "annual" || row.ObservedValue != 20 || row.AllocatedValue != want {
			t.Errorf("annual source trace is not a completed monthly sum: %#v", row)
		}
	}
	// The normal monthly-authoritative path and the existing annual-only direct
	// override must select the matching source ledger, not concatenate both.
	for _, override := range []bool{false, true} {
		annualOnly := epathHVACConsumptionPoolPlanSnapshot(annual)
		annualOnly.ConsumptionSourceAllocations = append([]energyPathHVACConsumptionSourceAllocation(nil), annual.ConsumptionSourceAllocations...)
		if override {
			annualOnly.Records[0].DirectValue++
			annualOnly.ConsumptionSourceAllocations[0].AllocatedValue = 99
		}
		chosen := energyPathZoneHVACAllocationPlanWithAnnualFallback(annual, annualOnly, nodes)
		if len(chosen.ConsumptionSourceAllocations) != 5 || !chosen.ConsumptionPoolGroups[energyPathZoneHVACAllocationGroupKey("heating", "electricity")] {
			t.Fatalf("annual fallback lost or duplicated source evidence: override=%v", override)
		}
		if (chosen.ConsumptionSourceAllocations[0].AllocatedValue == 99) != override {
			t.Errorf("annual fallback source evidence disagrees with selected direct-first ledger: override=%v", override)
		}
	}
}

func epathHVACConsumptionPoolFixture() ([]EnergyExplanationNode, []energyExplanationSeries, energyServicePathIndex, energyPathHVACConsumptionPool) {
	nodes := []EnergyExplanationNode{epath100AuditEndUseNode("heating", "electricity", 50, "M1", "sql.meter.heating.electricity", nil)}
	topology := energyServicePathIndex{byZoneService: map[string][]string{}}
	paths := []string{}
	for i := 1; i <= 5; i++ {
		zone := fmt.Sprintf("SPACE%d-1", i)
		path := "heating." + zone
		paths = append(paths, path)
		topology.byZoneService[normalizePurposeToken(zone)+"|heating"] = []string{path}
		nodes = append(nodes, epath100AuditLoadNode(zone, "heating", 1, "M1", "sql.load."+zone, []string{path}))
	}
	// A positive actual plenum response is not conditioned equipment ownership.
	nodes = append(nodes, epath100AuditLoadNode("PLENUM-1", "heating", 1000, "M1", "sql.load.PLENUM-1", nil))
	direct := []energyExplanationSeries{epathHVACConsumptionPoolSeries("SPACE2-1", "sql.baseboard.2", 10), epathHVACConsumptionPoolSeries("SPACE4-1", "sql.baseboard.4", 30)}
	pool := energyPathHVACConsumptionPool{ID: "heating.electricity", ServiceKind: "heating", Carrier: "electricity", MeterSourceIDs: []string{"sql.meter.heating.electricity"}, Valid: true}
	for i, item := range direct {
		pool.Members = append(pool.Members, energyPathHVACConsumptionMember{ID: fmt.Sprintf("baseboard.%d", i), ObjectType: "ZoneHVAC:Baseboard:RadiantConvective:Electric", ObjectName: item.ZoneName + " Baseboard", OutputName: "Baseboard Electricity Energy", ZoneName: item.ZoneName, Series: item})
	}
	pool.Members = append(pool.Members, energyPathHVACConsumptionMember{ID: "boiler.ancillary", ObjectType: "Boiler:HotWater", ObjectName: "Central Boiler", OutputName: "Boiler Ancillary Electricity Energy", RelatedPathIDs: paths, Series: epathHVACConsumptionPoolSeries("", "sql.boiler.ancillary", 10)})
	return nodes, direct, topology, pool
}

func epathHVACConsumptionPoolSeries(zone, source string, value float64) energyExplanationSeries {
	return energyExplanationSeries{Stage: "end_use", Level: "energy", Kind: "energy.heating", Unit: "kWh", ZoneName: zone, ServiceKind: "heating", EndUse: "heating", Carrier: "electricity", SourceIDs: []string{source}, MonthlySourceIDs: []string{source}, Monthly: map[int]float64{1: value}, RawMonthly: map[int]float64{1: value}, Total: value, RawTotal: value, EffectiveMultiplier: 1, sourceFrequency: "Monthly", Basis: "direct_zone_energy"}
}

func epathHVACConsumptionPoolAssert(t *testing.T, plan energyPathZoneHVACAllocationPlan, expected, direct, allocated, unassigned, overmapped float64, zones map[string]float64) {
	t.Helper()
	if len(plan.Records) != 1 {
		t.Fatalf("record count=%d, want1: %#v", len(plan.Records), plan.Records)
	}
	row := plan.Records[0]
	if row.ExpectedValue != expected || row.DirectValue != direct || row.AllocatedValue != allocated || row.UnassignedValue != unassigned || row.OvermappedValue != overmapped {
		t.Errorf("ledger=%#v, want expected/direct/allocated/unassigned/overmapped=%g/%g/%g/%g/%g", row, expected, direct, allocated, unassigned, overmapped)
	}
	actual := map[string]float64{}
	seen := map[string]bool{}
	for _, edge := range plan.Edges {
		pair := edge.FromID + "\x00" + edge.ToID
		if seen[pair] || edge.Value <= 0 || !energyPathFinite(edge.Value) {
			t.Errorf("duplicate, negative or invalid source-local edge: %#v", edge)
		}
		seen[pair] = true
		actual[edge.ZoneName] = roundedEnergyNumber(actual[edge.ZoneName] + edge.Value)
	}
	if len(actual) != len(zones) {
		t.Errorf("Zone count differs: got %v, want %v", actual, zones)
	}
	for zone, value := range zones {
		if actual[zone] != value {
			t.Errorf("Zone %s allocated=%g, want %g", zone, actual[zone], value)
		}
	}
}

func epathHVACConsumptionPoolPlanSnapshot(plan energyPathZoneHVACAllocationPlan) energyPathZoneHVACAllocationPlan {
	copy := plan
	copy.Edges = append([]EnergyExplanationEdge(nil), plan.Edges...)
	copy.Records = append([]energyPathZoneHVACAllocationRecord(nil), plan.Records...)
	for i := range copy.Edges {
		copy.Edges[i].SourceIDs = append([]string(nil), plan.Edges[i].SourceIDs...)
		copy.Edges[i].RelatedPathIDs = append([]string(nil), plan.Edges[i].RelatedPathIDs...)
	}
	for i := range copy.Records {
		copy.Records[i].SourceIDs = append([]string(nil), plan.Records[i].SourceIDs...)
		sort.Strings(copy.Records[i].SourceIDs)
	}
	return copy
}
