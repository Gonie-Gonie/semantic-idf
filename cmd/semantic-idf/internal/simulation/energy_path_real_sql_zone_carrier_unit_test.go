package simulation

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathSQLZoneCarrierUnitInputs(t *testing.T) (epathSQLFrames, epathRealSQLModel, epathSQLModelChecks) {
	t.Helper()
	frames, model := epathSQLZoneServiceUnitFrames()
	sources, directFrames, directModel := epathDirectUseUnitInputs()
	frames.Zones, model.DirectUses = directFrames.Zones, directModel.DirectUses
	for index := range sources {
		sources[index].DictionaryIndex += 100
	}
	model.Site = append(model.Site, epathRealSQLSite{ID: "fans.e", EndUse: "fans", Carrier: "electricity"})
	model.FanPools = []epathRealSQLFanPool{{SiteID: "fans.e", Name: "Air System Fan Electricity Energy", Key: "VAV", Frequency: "Hourly", Unit: "J", ServedZones: []string{"A", "B"}}}
	checks := epathSQLModelChecks{}
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelDirectUseChecks(sources, frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	// Test-owned prior fan proof: M1's pool14 shares by effective C+H weights
	// A40:B100 =>4:10; M2's pool20 by A90:B10 =>18:2. No annual reweighting.
	for _, zone := range []string{"A", "B", "Plenum"} {
		for _, period := range epathSQLZoneCarrierPeriods() {
			value := 0.0
			if zone == "A" {
				value = map[string]float64{"M1": 4, "M2": 18, "annual": 22}[period]
			} else if zone == "B" {
				value = map[string]float64{"M1": 10, "M2": 2, "annual": 12}[period]
			}
			target := epathSQLNodeTarget("end_use", "fans", "", "site")
			target.Basis, target.AllowPrunedZero = "service_path_allocation", true
			if err := checks.add("zoneAllocation", "zone", zone, period, "fan_pools/value", "kWh", &epathSQLQuantity{Value: value}, target, "", nil, nil); err != nil {
				t.Fatal(err)
			}
		}
	}
	return frames, model, checks
}

func epathSQLZoneCarrierUnitChecks(t *testing.T) epathSQLModelChecks {
	t.Helper()
	frames, model, checks := epathSQLZoneCarrierUnitInputs(t)
	start := len(checks.Rows)
	if err := epathSQLModelZoneCarrierChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return epathSQLModelChecks{Rows: checks.Rows[start:]}
}

// Independent hand arithmetic, not a candidate built from compiler output.
func epathSQLZoneCarrierUnitBundle() PurposeResultBundle {
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	for _, zone := range []string{"A", "B", "Plenum"} {
		result := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: zone, AggregationBasis: "model_total"}}
		for _, period := range epathSQLZoneCarrierPeriods() {
			electricity, gas := 0.0, 0.0
			if zone == "A" {
				electricity = 20 // 2 native kWh * Zone multiplier10 exactly once.
				if period == "M1" {
					electricity, gas = 49, 30
				} else if period == "M2" {
					electricity = 218
				} else if period == "annual" {
					electricity, gas = 467, 30
				}
			} else if zone == "B" {
				electricity = 1
				if period == "M1" {
					electricity, gas = 106, 10
				} else if period == "M2" {
					electricity = 23
				} else if period == "annual" {
					electricity, gas = 139, 10
				}
			}
			p := EnergyPeriod{ID: period, Kind: "monthly"}
			if period == "annual" {
				p.Kind = "annual"
			}
			for _, carrier := range []string{"electricity", "natural_gas"} {
				value, basis := electricity, "direct_zone_energy"
				if carrier == "natural_gas" {
					value, basis = gas, "service_path_allocation"
				}
				if value == 0 {
					continue
				}
				id := "carrier." + carrier + "." + strings.ToLower(zone)
				p.Nodes = append(p.Nodes, EnergyExplanationNode{ID: id, Level: "carrier", Carrier: carrier, EndUse: "total", Value: value, AllocatedValue: value, AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", Basis: basis, AggregationBasis: "model_total", ZoneName: zone, Period: period})
				rowID := "reconcile.energy." + carrier + "." + period + "." + strings.ToLower(zone)
				if period == "annual" {
					rowID = "reconcile.energy." + carrier + ".M1." + strings.ToLower(zone) + ".annual"
				}
				p.Reconciliation = append(p.Reconciliation, EnergyReconciliation{ID: rowID, Level: "energy", ZoneName: zone, Period: period, Unit: "kWh", Basis: basis, Status: "partial", ExpectedValue: value, ExplainedValue: value})
			}
			if period == "annual" {
				result.Nodes = append([]EnergyExplanationNode(nil), p.Nodes...)
				result.Reconciliation = append([]EnergyReconciliation(nil), p.Reconciliation...)
			}
			result.Periods = append(result.Periods, p)
		}
		bundle.EnergyExplanation.ZoneResults = append(bundle.EnergyExplanation.ZoneResults, result)
	}
	return bundle
}

func TestEnergyPathRealSQLZoneCarrierMonthlySubtotalAndMultiplier(t *testing.T) {
	checks := epathSQLZoneCarrierUnitChecks(t)
	if len(checks.Rows) != 3*13*2*5 {
		t.Fatalf("missing per-Zone/carrier/month numeric obligations: %d", len(checks.Rows))
	}
	bundle := epathSQLZoneCarrierUnitBundle()
	before := epathSQLZoneCarrierUnitBundle()
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 0 {
		t.Fatalf("independent completed-month subtotals differ: %+v", failures)
	}
	if !reflect.DeepEqual(bundle, before) {
		t.Fatal("Zone subtotal checker changed the original graph")
	}
	for _, check := range checks.Rows {
		if check.ZoneCarrier != nil && check.Item.Zone == "A" && check.Item.Period == "annual" && check.ZoneCarrier.Carrier == "electricity" && check.Quantity.Value != 467 {
			t.Fatalf("annual load reweighting or repeated Zone multiplier changed467: %+v", check)
		}
		if check.Reconciliation != nil && check.Item.Zone == "A" && check.Item.Period == "annual" && check.Reconciliation.ID != "reconcile.energy.electricity.M1.a.annual" && check.Reconciliation.ID != "reconcile.energy.natural_gas.M1.a.annual" {
			t.Fatalf("historical first-month identity was guessed incorrectly: %+v", check.Reconciliation)
		}
	}
}

func TestEnergyPathRealSQLZoneCarrierAdversarialCandidate(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*EnergyExplanationZoneResult)
	}{
		{"carrier swap", func(z *EnergyExplanationZoneResult) { z.Periods[1].Nodes[0].Carrier = "natural_gas" }},
		{"Zone swap", func(z *EnergyExplanationZoneResult) { z.Periods[1].Nodes[0].ZoneName = "B" }},
		{"monthly swap preserves annual", func(z *EnergyExplanationZoneResult) {
			z.Periods[1].Nodes[0].Value, z.Periods[2].Nodes[0].Value = 218, 49
		}},
		{"repeat multiplier", func(z *EnergyExplanationZoneResult) { z.Periods[1].Nodes[0].Value *= 10 }},
		{"facility instead of subtotal", func(z *EnergyExplanationZoneResult) { z.Periods[1].Nodes[0].Basis = "reported_meter" }},
		{"missing positive node", func(z *EnergyExplanationZoneResult) { z.Periods[1].Nodes = nil }},
		{"duplicate zero node", func(z *EnergyExplanationZoneResult) {
			n := z.Periods[1].Nodes[0]
			n.ID, n.Value, n.AllocatedValue = "extra", 0, 0
			z.Periods[1].Nodes = append(z.Periods[1].Nodes, n)
		}},
		{"unknown allocated field", func(z *EnergyExplanationZoneResult) {
			z.Periods[1].Nodes[0].inspectorDecodedFromJSON = true
			z.Periods[1].Nodes[0].inspectorValuePresence = 0
		}},
		{"reconciliation wrong carrier", func(z *EnergyExplanationZoneResult) {
			z.Periods[1].Reconciliation[0].ID = "reconcile.energy.natural_gas.M1.a"
		}},
		{"reconciliation wrong Zone", func(z *EnergyExplanationZoneResult) { z.Periods[1].Reconciliation[0].ZoneName = "B" }},
		{"reconciliation false complete", func(z *EnergyExplanationZoneResult) { z.Periods[1].Reconciliation[0].Status = "balanced" }},
		{"reconciliation missing", func(z *EnergyExplanationZoneResult) { z.Periods[1].Reconciliation = nil }},
		{"reconciliation duplicate", func(z *EnergyExplanationZoneResult) {
			z.Periods[1].Reconciliation = append(z.Periods[1].Reconciliation, z.Periods[1].Reconciliation[0])
		}},
		{"reconciliation corrupt explained", func(z *EnergyExplanationZoneResult) { z.Periods[1].Reconciliation[0].ExplainedValue-- }},
		{"reconciliation corrupt residual", func(z *EnergyExplanationZoneResult) { z.Periods[1].Reconciliation[0].ResidualValue = 1 }},
		{"annual duplicate wrapper changed", func(z *EnergyExplanationZoneResult) { z.Periods[0].Reconciliation[0].ExpectedValue++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := epathSQLZoneCarrierUnitBundle()
			test.change(&bundle.EnergyExplanation.ZoneResults[0])
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, epathSQLZoneCarrierUnitChecks(t)); len(failures) == 0 {
				t.Fatal("corrupt Zone subtotal escaped its independent numeric/context proof")
			}
		})
	}
}

func TestEnergyPathRealSQLZoneCarrierSQLZoneCaseIdentity(t *testing.T) {
	// Actual EnergyPlus SQL stores CORE_MID while the canonical IDF graph
	// preserves Core_mid. Only Zone identity is case insensitive; scalar
	// metadata and distinct Zone ownership must remain exact.
	for _, mutation := range []string{"", "different Zone", "missing Zone", "wrong period", "wrong basis", "wrong unit", "wrong domain", "wrong aggregation"} {
		t.Run(mutation, func(t *testing.T) {
			bundle := epathSQLZoneCarrierUnitBundle()
			zone := &bundle.EnergyExplanation.ZoneResults[0]
			zone.Scope.ZoneName = "Core_mid"
			fixNodes := func(nodes []EnergyExplanationNode) {
				for i := range nodes {
					nodes[i].ZoneName = "Core_mid"
					nodes[i].ID = strings.ReplaceAll(nodes[i].ID, ".a", ".core_mid")
				}
			}
			fixNodes(zone.Nodes)
			for i := range zone.Reconciliation {
				zone.Reconciliation[i].ZoneName = "Core_mid"
			}
			for i := range zone.Periods {
				fixNodes(zone.Periods[i].Nodes)
				for j := range zone.Periods[i].Reconciliation {
					zone.Periods[i].Reconciliation[j].ZoneName = "Core_mid"
				}
			}
			node := &zone.Periods[1].Nodes[0]
			switch mutation {
			case "different Zone":
				node.ZoneName = "Core_top"
			case "missing Zone":
				node.ZoneName = ""
			case "wrong period":
				node.Period = "M2"
			case "wrong basis":
				node.Basis = "reported_meter"
			case "wrong unit":
				node.Unit = "MJ"
			case "wrong domain":
				node.ScaleDomain = "thermal"
			case "wrong aggregation":
				node.AggregationBasis = "native_zone"
			}
			checks := epathSQLModelChecks{}
			for _, check := range epathSQLZoneCarrierUnitChecks(t).Rows {
				if check.ZoneCarrier == nil || check.Item.Zone != "A" {
					continue
				}
				proof := *check.ZoneCarrier
				proof.ZoneName, check.Item.Zone = "CORE_MID", "CORE_MID"
				check.ZoneCarrier = &proof
				checks.Rows = append(checks.Rows, check)
			}
			if len(checks.Rows) != 13*2*2 {
				t.Fatal("missing all-period/all-carrier value and allocation obligations")
			}
			failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks)
			if (len(failures) == 0) != (mutation == "") {
				t.Fatalf("SQL uppercase/canonical preserved-case with mutation %q: %+v", mutation, failures)
			}
		})
	}
}

func TestEnergyPathRealSQLZoneCarrierMissingInputsAreNotZero(t *testing.T) {
	for _, name := range []string{"missing month", "missing proof", "unknown", "nonfinite", "negative", "duplicate", "carrier swap", "multiplier", "annual reweight", "unsupported auxiliary"} {
		t.Run(name, func(t *testing.T) {
			frames, model, checks := epathSQLZoneCarrierUnitInputs(t)
			index := -1
			for i, check := range checks.Rows {
				if check.ZoneService != nil && check.Item.Zone == "A" && check.Item.Period == "M1" {
					index = i
					break
				}
			}
			if index < 0 {
				t.Fatal("missing adversarial fixture")
			}
			switch name {
			case "missing month":
				checks.Rows = append(checks.Rows[:index], checks.Rows[index+1:]...)
			case "missing proof":
				checks.Rows[index].ZoneService = nil
			case "unknown":
				checks.Rows[index].Quantity = nil
			case "nonfinite":
				checks.Rows[index].Quantity = &epathSQLQuantity{Value: math.NaN()}
			case "negative":
				checks.Rows[index].Quantity = &epathSQLQuantity{Value: -1}
			case "duplicate":
				checks.Rows = append(checks.Rows, checks.Rows[index])
			case "carrier swap":
				checks.Rows[index].ZoneService.Carriers = map[string]epathSQLQuantity{"diesel": {Value: 10}}
			case "multiplier":
				frames.Zones["a"] = epathSQLZone{Name: "A", Multiplier: 1}
			case "annual reweight":
				for i := range checks.Rows {
					if strings.HasSuffix(checks.Rows[i].Want.Key, "|fan_pools/value") && checks.Rows[i].Item.Zone == "A" && checks.Rows[i].Item.Period == "annual" {
						checks.Rows[i].Quantity = &epathSQLQuantity{Value: 23}
						checks.Rows[i].Want.Value = epathOracleNumber(23)
					}
				}
			case "unsupported auxiliary":
				model.Auxiliaries = append(model.Auxiliaries, epathRealSQLAuxiliary{SiteID: "unknown", Weight: "cooling_plus_heating"})
			}
			if err := epathSQLModelZoneCarrierChecks(frames, model, &checks); err == nil {
				t.Fatal("unknown/contradictory upstream carrier proof became a known subtotal")
			}
		})
	}
}

func TestEnergyPathRealSQLZoneCarrierAnnualIDPossibilities(t *testing.T) {
	monthly := [12]epathSQLQuantity{}
	monthly[1], monthly[2] = epathSQLBounded(.0002, 0, .0007), epathSQLQuantity{Value: 5}
	ids, err := epathSQLZoneCarrierRowIDs("electricity", "South Office", "annual", monthly)
	if err != nil || !reflect.DeepEqual(ids, []string{"reconcile.energy.electricity.M2.south_office.annual", "reconcile.energy.electricity.M3.south_office.annual"}) {
		t.Fatalf("first retained month possibilities must derive from SQL intervals: %v/%v", ids, err)
	}
	ids, err = epathSQLZoneCarrierRowIDs("electricity", "MidFloor", "annual", monthly)
	if err != nil || !reflect.DeepEqual(ids, []string{"reconcile.energy.electricity.M2.annual", "reconcile.energy.electricity.M3.annual"}) {
		t.Fatalf("historical m-prefixed Zone token compatibility lost: %v/%v", ids, err)
	}
	for _, period := range []string{"M0", "M13", "M1extra"} {
		if _, err := epathSQLZoneCarrierRowIDs("electricity", "A", period, monthly); err == nil {
			t.Fatal("invalid typed period accepted")
		}
	}
}

func TestEnergyPathRealSQLZoneCarrierWholeRowZeroAndAllowedIDs(t *testing.T) {
	monthly := [12]epathSQLQuantity{}
	monthly[1], monthly[2] = epathSQLBounded(.0002, 0, .0007), epathSQLQuantity{Value: 5}
	ids, err := epathSQLZoneCarrierRowIDs("electricity", "South Office", "annual", monthly)
	if err != nil {
		t.Fatal(err)
	}
	q := monthly[1].add(monthly[2])
	proof := &epathSQLReconciliationProof{ID: ids[0], AllowedIDs: ids, Level: "energy", ZoneName: "South Office", Period: "annual", Unit: "kWh", Basis: "direct_zone_energy", Status: "partial", Expected: q, Explained: q}
	check := epathSQLModelCheck{Item: epathRealOracleMetricRecipe{Scope: "zone", Zone: "South Office", Period: "annual", Unit: "kWh", Target: epathRealOracleTarget{Collection: "reconciliation", ID: ids[0], Level: "energy", Field: "expectedValue", Unit: "kWh", Basis: "direct_zone_energy", Status: "partial"}}, Quantity: &q, Reconciliation: proof}
	for _, id := range ids {
		row := EnergyReconciliation{ID: id, Level: "energy", ZoneName: "South Office", Period: "annual", Unit: "kWh", Basis: "direct_zone_energy", Status: "partial", ExpectedValue: 5.0002, ExplainedValue: 5.0002}
		for _, bad := range []string{"", "duplicate allowed rows", "unproved first month", "wrong typed period", "wrong typed Zone"} {
			rows := []EnergyReconciliation{row}
			switch bad {
			case "duplicate allowed rows":
				extra := row
				extra.ID = ids[1]
				rows = append(rows, extra)
			case "unproved first month":
				rows[0].ID = "reconcile.energy.electricity.M4.south_office.annual"
			case "wrong typed period":
				rows[0].Period = "M3"
			case "wrong typed Zone":
				rows[0].ZoneName = "Other Office"
			}
			zone := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "South Office"}, Reconciliation: rows, Periods: []EnergyPeriod{{ID: "annual", Kind: "annual", Reconciliation: rows}}}
			bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, ZoneResults: []EnergyExplanationZoneResult{zone}}}
			if err := epathCheckSQLModelReconciliation(bundle, check); (err == nil) != (bad == "") {
				t.Fatalf("SQL-proved first retained month %s, mutant %q: %v", id, bad, err)
			}
		}
	}
	// Missing whole rows are legal only with a fully observed zero-containing
	// proof for every field. Preserve the tiny positive SQL center unchanged.
	q = epathSQLBounded(.0002, 0, .0007)
	proof.Expected, proof.Explained = q, q
	check.Quantity = &q
	zone := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "South Office"}, Periods: []EnergyPeriod{{ID: "annual", Kind: "annual"}}}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, ZoneResults: []EnergyExplanationZoneResult{zone}}}
	if err := epathCheckSQLModelReconciliation(bundle, check); err != nil || proof.Expected.Value != .0002 {
		t.Fatalf("known tiny/prunable whole row changed raw observation or failed: %v", err)
	}
	q = epathSQLQuantity{Value: 1}
	proof.Expected, proof.Explained = q, q
	check.Quantity = &q
	if err := epathCheckSQLModelReconciliation(bundle, check); err == nil {
		t.Fatal("zero residual alone authorized pruning a positive whole subtotal")
	}
	proof.Expected = epathSQLQuantity{Value: math.NaN()}
	if err := epathCheckSQLModelReconciliation(bundle, check); err == nil {
		t.Fatal("invalid independent subtotal became prunable")
	}
}
