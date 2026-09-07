package simulation

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestEnergyPathRealSQLModelDirectionalIntervals(t *testing.T) {
	q := epathSQLQuantity{Value: -.0002, Error: .001}.positive()
	low, high := q.bounds()
	if q.Value != 0 || low != 0 || math.Abs(high-.0008) > 1e-15 {
		t.Fatalf("negative center discarded possible positive pressure: %#v", q)
	}
	zero := epathSQLQuantity{Value: -5, Error: .001}.positive()
	if zero.Value != 0 || zero.Error != 0 {
		t.Fatal("strictly negative pressure invented positive uncertainty")
	}
	optional := epathSQLQuantity{Value: 5, Error: .01}.optionalPresentation()
	total := epathSQLQuantity{Value: 10, Error: .01}.add(optional).add(q)
	low, high = total.bounds()
	if math.Abs(low-9.99) > 1e-12 || math.Abs(high-15.0208) > 1e-12 || total.Value != 15 {
		t.Fatalf("optional raw pressure range must not double: center=%g interval=[%g,%g]", total.Value, low, high)
	}
	if err := epathCheckSQLModelQuantity(epathOracleNumber(20), &total); err == nil {
		t.Fatal("symmetric optional pad accepted twice the actual pressure")
	}
	precision := epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}
	share, err := epathSQLShare(epathSQLQuantity{Value: 100}, q, epathSQLQuantity{Value: 10}, precision)
	if err != nil {
		t.Fatal(err)
	}
	_, high = share.bounds()
	if share.Value != 0 || high <= 0 {
		t.Fatal("zero-centered uncertain pressure vanished from contribution bounds")
	}
	share, err = epathSQLShare(epathSQLQuantity{Value: 100}, zero, epathSQLQuantity{Value: 10}, precision)
	if err != nil || share.Value != 0 || share.Error != 0 {
		t.Fatal("actual absent directional pressure gained rounding energy")
	}
	if _, err := epathSQLShare(epathSQLQuantity{}, epathSQLQuantity{}, epathSQLQuantity{}, epathRealSQLPrecision{}); err == nil {
		t.Fatal("zero values bypassed invalid precision configuration")
	}
	if _, err := epathSQLShare(epathSQLQuantity{Value: 1}, epathSQLQuantity{}, epathSQLQuantity{}, precision); err == nil {
		t.Fatal("zero pressure concealed missing denominator for positive delivered load")
	}
}

func epathSQLModelAllocationFixture() (PurposeResultBundle, epathSQLModelCheck) {
	b, _, _ := epathOracleCandidateFixture()
	b.EnergyExplanation.Reconciliation = nil
	expected := epathSQLQuantity{Value: .0004, Error: .00051}.positive()
	allocated := expected
	direct, unassigned := epathSQLQuantity{}, epathSQLQuantity{}
	proof := &epathSQLAllocationProof{&expected, &direct, &allocated, &unassigned}
	item := epathRealOracleMetricRecipe{Group: "zoneAllocation", Scope: "building", Period: "annual", Unit: "kWh", Target: epathRealOracleTarget{Collection: "reconciliation", ID: "allocation.cooling.annual", Level: "allocation", Field: "expectedValue", Unit: "kWh"}}
	return b, epathSQLModelCheck{Item: item, Want: epathRealOracleMetric{Group: "zoneAllocation", Scope: "building", Period: "annual", Unit: "kWh", Value: epathOracleNumber(expected.Value)}, Quantity: &expected, Allocation: proof}
}

func TestEnergyPathRealSQLModelWholeAllocationPruningRequiresCompleteProof(t *testing.T) {
	b, check := epathSQLModelAllocationFixture()
	if err := epathCheckSQLModelAllocation(b, check); err != nil {
		t.Fatalf("complete tiny whole-row proof: %v", err)
	}
	if check.Allocation.Expected.Value != .0004 {
		t.Fatal("raw SQL observation rewritten to presentation zero")
	}
	for _, test := range []struct {
		name   string
		change func(*epathSQLModelCheck)
	}{
		{"unknown_expected", func(c *epathSQLModelCheck) { c.Allocation.Expected = nil }},
		{"unknown_direct", func(c *epathSQLModelCheck) { c.Allocation.Direct = nil }},
		{"unknown_allocated", func(c *epathSQLModelCheck) { c.Allocation.Allocated = nil }},
		{"unknown_unassigned", func(c *epathSQLModelCheck) { c.Allocation.Unassigned = nil }},
		{"invalid_tolerance", func(c *epathSQLModelCheck) { c.Allocation.Direct.Error = math.NaN() }},
		{"negative_quantity", func(c *epathSQLModelCheck) { c.Allocation.Direct.Value = -.0001; c.Allocation.Direct.Error = .001 }},
		{"negative_interval", func(c *epathSQLModelCheck) { c.Allocation.Direct.Error = .001 }},
		{"large_missing", func(c *epathSQLModelCheck) {
			*c.Allocation.Expected = epathSQLQuantity{Value: 1, Error: .001}
			*c.Allocation.Allocated = *c.Allocation.Expected
		}},
		{"nonzero_direct", func(c *epathSQLModelCheck) {
			*c.Allocation.Expected = epathSQLQuantity{Value: 1, Error: .001}
			*c.Allocation.Direct = *c.Allocation.Expected
			*c.Allocation.Allocated = epathSQLQuantity{}
		}},
		{"does_not_conserve", func(c *epathSQLModelCheck) {
			*c.Allocation.Allocated = epathSQLQuantity{Value: .0002, Error: .0003}.positive()
		}},
		{"missing_exact_id", func(c *epathSQLModelCheck) { c.Item.Target.ID = "" }},
		{"not_allocation", func(c *epathSQLModelCheck) { c.Item.Target.Level = "carrier" }},
		{"not_reconciliation", func(c *epathSQLModelCheck) { c.Item.Target.Collection = "sources" }},
		{"wrong_field", func(c *epathSQLModelCheck) { c.Item.Target.Field = "residualValue" }},
		{"unknown_checked_quantity", func(c *epathSQLModelCheck) { c.Quantity = nil }},
		{"contradictory_checked_quantity", func(c *epathSQLModelCheck) { c.Quantity = &epathSQLQuantity{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, c := epathSQLModelAllocationFixture()
			test.change(&c)
			if err := epathCheckSQLModelAllocation(b, c); err == nil {
				t.Fatal("invalid whole-row absence accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLModelPresentAllocationNeverTreatedAsPruned(t *testing.T) {
	for _, test := range []struct {
		name      string
		change    func(*EnergyReconciliation)
		duplicate bool
	}{
		{"valid_zero", func(*EnergyReconciliation) {}, false},
		{"wrong_unit", func(r *EnergyReconciliation) { r.Unit = "J" }, false},
		{"wrong_level", func(r *EnergyReconciliation) { r.Level = "carrier" }, false},
		{"wrong_period", func(r *EnergyReconciliation) { r.Period = "M1" }, false},
		{"negative", func(r *EnergyReconciliation) { r.DirectValue = -.0001 }, false},
		{"nonfinite", func(r *EnergyReconciliation) { r.UnassignedValue = math.NaN() }, false},
		{"out_of_bounds", func(r *EnergyReconciliation) { r.AllocatedValue = .01 }, false},
		{"duplicate_exact_id", func(*EnergyReconciliation) {}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, c := epathSQLModelAllocationFixture()
			row := EnergyReconciliation{ID: c.Item.Target.ID, Level: "allocation", Period: "annual", Unit: "kWh"}
			test.change(&row)
			b.EnergyExplanation.Reconciliation = []EnergyReconciliation{row}
			if test.duplicate {
				b.EnergyExplanation.Reconciliation = append(b.EnergyExplanation.Reconciliation, row)
			}
			err := epathCheckSQLModelAllocation(b, c)
			if (err == nil) != (test.name == "valid_zero") {
				t.Fatalf("present row %s: %v", test.name, err)
			}
		})
	}
}

func TestEnergyPathRealSQLModelPruningIsNotUnknownSource(t *testing.T) {
	b, item, want := epathOracleCandidateFixture()
	item.Target.AllowPrunedZero = true
	check := epathSQLModelCheck{Item: item, Want: want, Quantity: &epathSQLQuantity{Value: .0004, Error: .00051}}
	b.EnergyExplanation.Nodes = nil
	if err := epathCheckSQLModelPresentation(b, check, nil); err != nil {
		t.Fatalf("fully observed prunable node: %v", err)
	}
	if check.Quantity.Value != .0004 {
		t.Fatal("source-backed center was rewritten to zero")
	}
	check.Quantity = &epathSQLQuantity{Value: 1, Error: .00051}
	if err := epathCheckSQLModelPresentation(b, check, nil); err == nil {
		t.Fatal("large missing contribution accepted as pruned")
	}
	check.Quantity = nil
	if err := epathCheckSQLModelPresentation(b, check, nil); err == nil {
		t.Fatal("unknown SQL authorized pruning")
	}
	check.Quantity = &epathSQLQuantity{Value: 0, Error: math.NaN()}
	if err := epathCheckSQLModelPresentation(b, check, nil); err == nil {
		t.Fatal("unknown actual bypassed invalid interval")
	}
	check.Quantity = &epathSQLQuantity{Value: .0004, Error: .00051}
	if err := epathCheckSQLModelPresentation(b, check, epathOracleNumber(-.0001)); err == nil {
		t.Fatal("negative directional node accepted within zero-crossing interval")
	}
	check.OptionalPresentation = true
	check.Item.Target.Collection = "sources"
	if err := epathCheckSQLModelPresentation(b, check, nil); err == nil {
		t.Fatal("pruned graph granted unknown root source value")
	}
	check.Item.Target.Collection = "nodes"
	check.Item.Target.Field = "rawValue"
	check.Item.Target.AllowPrunedZero = false
	optional := epathSQLQuantity{Value: 5, Error: .001}.optionalPresentation()
	check.Quantity = &optional
	if err := epathCheckSQLModelPresentation(b, check, nil); err != nil {
		t.Fatalf("absent optional driver raw presentation: %v", err)
	}
	b, _, _ = epathOracleCandidateFixture()
	if err := json.Unmarshal([]byte(`{"id":"wall","level":"driver","kind":"heat.surface","driverCategory":"surface.exterior_walls","serviceKind":"cooling","value":0.0004,"rawValue":null,"unit":"kWh","scaleDomain":"thermal","period":"annual","basis":"heat_balance_share","allocationApplied":true}`), &b.EnergyExplanation.Nodes[0]); err != nil {
		t.Fatal(err)
	}
	if err := epathCheckSQLModelPresentation(b, check, nil); err == nil {
		t.Fatal("present node's null field mistaken for absent node")
	}
	if err := epathCheckSQLModelPresentation(b, check, epathOracleNumber(5.01)); err == nil {
		t.Fatal("optional raw field escaped its proven upper bound")
	}
}

func TestEnergyPathRealSQLModelVisibleRawKeepsUncertainCells(t *testing.T) {
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{"office": {Name: "Office", Multiplier: 1}}, Loads: map[string]epathSQLQuantity{}, Cells: map[string]*epathSQLCell{}}
	frames.Loads[epathSQLKey("Office", "cooling", 1)] = epathSQLQuantity{Value: 100}
	for key, cell := range map[string]*epathSQLCell{
		"certain":            {Raw: epathSQLQuantity{Value: 10, Error: .01}, Allocated: map[string]epathSQLQuantity{"cooling": {Value: 1, Error: .01}}},
		"uncertain-positive": {Raw: epathSQLQuantity{Value: 5, Error: .01}, Allocated: map[string]epathSQLQuantity{"cooling": {Value: .0002, Error: .0005}}},
		"uncertain-negative": {Raw: epathSQLQuantity{Value: -.0002, Error: .001}, Allocated: map[string]epathSQLQuantity{"cooling": {Value: 0, Error: .001}}},
	} {
		cell.Zone = "office"
		cell.Month = 1
		cell.Category = "surface.exterior_walls"
		cell.Effective = cell.Raw
		cell.BuildingVisible = true
		frames.Cells[key] = cell
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelLoadDriverChecks(frames, epathRealSQLModel{}, &checks); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, check := range checks.Rows {
		if check.Want.Scope != "zone" || check.Want.Period != "M1" || !strings.HasSuffix(check.Want.Key, "surface.exterior_walls/cooling/rawValue") {
			continue
		}
		found = true
		low, high := check.Quantity.bounds()
		if check.OptionalPresentation || math.Abs(low-9.99) > 1e-12 || math.Abs(high-15.0208) > 1e-12 {
			t.Fatalf("certain+uncertain raw union: optional=%v [%g,%g]", check.OptionalPresentation, low, high)
		}
	}
	if !found {
		t.Fatal("required visible raw check missing")
	}
}

func epathSQLModelConversionFixture() (PurposeResultBundle, epathSQLModelCheck) {
	b, _, _ := epathOracleCandidateFixture()
	b.EnergyExplanation.Nodes = []EnergyExplanationNode{{ID: "load", Level: "load", ServiceKind: "cooling", ScaleDomain: "thermal", Unit: "kWh", Period: "annual", Value: 100}, {ID: "use", Level: "end_use", ServiceKind: "cooling", ScaleDomain: "site", Unit: "kWh", Period: "annual"}}
	b.EnergyExplanation.Sources = []EnergyDataSource{{ID: "exact-sql", RawValue: .0004987871280347543}}
	b.EnergyExplanation.Links = []EnergyPathLink{{ID: "conversion", FromID: "load", ToID: "use", Relation: "load_to_end_use", Basis: "service_path_allocation", ServiceKind: "cooling", Period: "annual", FromValue: 100, ToValue: 0, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"exact-sql"}}}
	item := epathRealOracleMetricRecipe{Scope: "building", Period: "annual", Unit: "ratio", Target: epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Basis: "service_path_allocation", Service: "cooling", FromUnit: "kWh", ToUnit: "kWh", RatioKind: "coefficient_of_performance", Aggregate: "sum"}}
	proof := &epathSQLConversionProof{From: epathSQLQuantity{Value: 100, Error: .001}, To: epathSQLQuantity{Value: .0004987871280347543, Error: .0005}}
	return b, epathSQLModelCheck{Item: item, Quantity: &epathSQLQuantity{Value: 100 / proof.To.Value}, Conversion: proof}
}

func TestEnergyPathRealSQLModelTinyDenominatorPresentation(t *testing.T) {
	b, check := epathSQLModelConversionFixture()
	raw := b.EnergyExplanation.Sources[0].RawValue
	if err := epathCheckSQLModelConversion(b, check); err != nil {
		t.Fatalf("normalized zero denominator honestly unavailable: %v", err)
	}
	b.EnergyExplanation.Links = nil
	if err := epathCheckSQLModelConversion(b, check); err != nil {
		t.Fatalf("whole conversion pruned by bounded site endpoint: %v", err)
	}
	if b.EnergyExplanation.Sources[0].RawValue != raw || check.Conversion.To.Value != raw || raw <= 0 {
		t.Fatal("raw tiny positive source became zero")
	}
	check.Conversion.To = epathSQLQuantity{Value: 10, Error: .001}
	if err := epathCheckSQLModelConversion(b, check); err == nil {
		t.Fatal("two positive required endpoints permitted missing conversion")
	}
	for _, test := range []struct {
		name   string
		mutate func(*PurposeResultBundle, *epathSQLModelCheck)
	}{
		{"zero denominator positive ratio", func(b *PurposeResultBundle, _ *epathSQLModelCheck) { b.EnergyExplanation.Links[0].Ratio = 1 }},
		{"zero denominator kind retained", func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Links[0].RatioKind = "coefficient_of_performance"
		}},
		{"negative endpoint", func(b *PurposeResultBundle, _ *epathSQLModelCheck) { b.EnergyExplanation.Links[0].ToValue = -.0001 }},
		{"thermal amount escaped", func(b *PurposeResultBundle, _ *epathSQLModelCheck) { b.EnergyExplanation.Links[0].FromValue = 200 }},
		{"wrong unit", func(b *PurposeResultBundle, _ *epathSQLModelCheck) { b.EnergyExplanation.Links[0].ToUnit = "MJ" }},
		{"unknown interval", func(_ *PurposeResultBundle, c *epathSQLModelCheck) { c.Conversion = nil }},
		{"invalid tolerance", func(b *PurposeResultBundle, c *epathSQLModelCheck) {
			b.EnergyExplanation.Links = nil
			c.Conversion.From.Error = math.NaN()
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, c := epathSQLModelConversionFixture()
			test.mutate(&b, &c)
			if err := epathCheckSQLModelConversion(b, c); err == nil {
				t.Fatal("invalid paired evidence accepted")
			}
		})
	}
	b, check = epathSQLModelConversionFixture()
	link := &b.EnergyExplanation.Links[0]
	link.ToValue = .0005
	link.Ratio = link.FromValue / link.ToValue
	link.RatioKind = "coefficient_of_performance"
	if err := epathCheckSQLModelConversion(b, check); err != nil {
		t.Fatalf("positive normalized pair must use its exact quotient: %v", err)
	}
	link.Ratio = check.Quantity.Value
	if err := epathCheckSQLModelConversion(b, check); err == nil {
		t.Fatal("raw SQL quotient replaced actual paired quotient")
	}
	link.Ratio = link.FromValue / link.ToValue
	link.RatioKind = "efficiency"
	if err := epathCheckSQLModelConversion(b, check); err == nil {
		t.Fatal("positive pair lost exact ratio kind")
	}
	link.FromValue = 1e-7
	link.ToValue = 1000
	link.Ratio = 1e-10
	link.RatioKind = "coefficient_of_performance"
	check.Conversion = &epathSQLConversionProof{From: epathSQLQuantity{Value: 1e-7}, To: epathSQLQuantity{Value: 1000}}
	if err := epathCheckSQLModelConversion(b, check); err != nil {
		t.Fatalf("exact tiny positive pair: %v", err)
	}
	link.Ratio = 1e-9
	if err := epathCheckSQLModelConversion(b, check); err == nil {
		t.Fatal("small positive ratio used an absolute tolerance larger than the quantity")
	}
}

func TestEnergyPathRealSQLModelEndUseTypedServiceIdentity(t *testing.T) {
	b, check := epathSQLModelConversionFixture()
	b.EnergyExplanation.Nodes[1].ServiceKind = ""
	b.EnergyExplanation.Nodes[1].EndUse = "cooling"
	if err := epathCheckSQLModelConversion(b, check); err != nil {
		t.Fatalf("actual typed EndUse identity with omitted redundant service: %v", err)
	}
	b.EnergyExplanation.Nodes[1].EndUse = "heating"
	if err := epathCheckSQLModelConversion(b, check); err == nil {
		t.Fatal("wrong typed EndUse accepted")
	}
	b.EnergyExplanation.Nodes[1].EndUse = "cooling"
	b.EnergyExplanation.Nodes[1].ServiceKind = "heating"
	if err := epathCheckSQLModelConversion(b, check); err == nil {
		t.Fatal("explicit contradictory service overridden by EndUse")
	}
	b.EnergyExplanation.Nodes[1].EndUse = "equipment"
	b.EnergyExplanation.Nodes[1].ServiceKind = "cooling"
	if err := epathCheckSQLModelConversion(b, check); err == nil {
		t.Fatal("explicit contradictory EndUse ignored")
	}
}

func TestEnergyPathRealSQLModelInvalidIntervalBeforeUnknown(t *testing.T) {
	for _, q := range []epathSQLQuantity{{Value: 0, Error: math.NaN()}, {Value: 0, Error: -1}, {Value: 0, Error: 1, Bounds: &[2]float64{1, 0}}, {Value: 0, Error: 1, Bounds: &[2]float64{0, math.Inf(1)}}} {
		if err := epathCheckSQLModelQuantity(nil, &q); err == nil {
			t.Fatal("unknown actual bypassed invalid interval configuration")
		}
		if _, err := epathSQLRatio(q, epathSQLQuantity{}); err == nil {
			t.Fatal("zero denominator bypassed invalid numerator interval")
		}
	}
}

func TestEnergyPathRealSQLModelKnownServedScopeNeverExpandsToPlenum(t *testing.T) {
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{"served": {Name: "Served", Multiplier: 1}, "plenum": {Name: "Plenum", Multiplier: 1}}, Loads: map[string]epathSQLQuantity{}, Site: map[string][]*epathSQLQuantity{"cooling": make([]*epathSQLQuantity, 12), "heating": make([]*epathSQLQuantity, 12)}}
	for month := 1; month <= 12; month++ {
		frames.Loads[epathSQLKey("served", "cooling", month)] = epathSQLQuantity{Value: 10}
		frames.Loads[epathSQLKey("plenum", "heating", month)] = epathSQLQuantity{Value: 100}
		frames.Site["cooling"][month-1] = &epathSQLQuantity{Value: 1}
		frames.Site["heating"][month-1] = &epathSQLQuantity{Value: 2}
	}
	model := epathRealSQLModel{}
	for _, service := range []string{"cooling", "heating"} {
		model.Services = append(model.Services, epathRealSQLService{Service: service, SiteIDs: []string{service}, ServedZones: []string{"Served"}, Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "efficiency", FallbackRatioKind: "efficiency", ReconciliationID: "reconcile." + service + ".annual"})
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, check := range checks.Rows {
		if check.Want.Period != "annual" {
			continue
		}
		for suffix, want := range map[string]float64{"heating/expectedValue": 24, "heating/allocatedValue": 0, "heating/unassignedValue": 24} {
			if strings.HasSuffix(check.Want.Key, suffix) {
				seen[suffix] = true
				if check.Quantity == nil || check.Quantity.Value != want {
					t.Fatalf("known topology scope changed %s: %#v", suffix, check.Quantity)
				}
			}
		}
		if strings.HasSuffix(check.Want.Key, "heating/zone_load_allocation") {
			seen["fallback"] = true
			if check.Quantity != nil || check.Conversion.From.Value != 0 || check.Conversion.To.Value != 0 {
				t.Fatal("passive plenum load invented fallback conversion")
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("required known-scope checks absent: %v", seen)
	}
}
