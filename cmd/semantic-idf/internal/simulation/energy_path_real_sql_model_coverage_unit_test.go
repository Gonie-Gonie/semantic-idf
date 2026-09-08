package simulation

import (
	"reflect"
	"strings"
	"testing"
)

func epathSQLCoverageFixture(t *testing.T) (PurposeResultBundle, epathSQLModelChecks) {
	t.Helper()
	driver := EnergyExplanationNode{ID: "wall", Level: "driver", DriverCategory: "surface.exterior_wall", ServiceKind: "cooling", ScaleDomain: "thermal", Unit: "kWh", Period: "annual", Basis: "heat_balance_share", Value: 10, RawValue: 12, EffectiveValue: 12}
	load := EnergyExplanationNode{ID: "cooling", Level: "load", ServiceKind: "cooling", ScaleDomain: "thermal", Unit: "kWh", Period: "annual", Basis: "reported_variable", Value: 10, LoadBreakdown: []EnergyExplanationLoadComponent{{Component: "sensible", Value: 10, Unit: "kWh"}}}
	link := EnergyPathLink{ID: "wall-to-cooling", FromID: driver.ID, ToID: load.ID, Relation: "driver_to_load", ServiceKind: "cooling", Basis: "heat_balance_share", FromValue: 10, ToValue: 10, FromUnit: "kWh", ToUnit: "kWh", Period: "annual"}
	quality := &EnergyPathQuality{}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Nodes: []EnergyExplanationNode{driver, load}, Links: []EnergyPathLink{link}, Quality: quality}}
	bundle.EnergyExplanation.Periods = []EnergyPeriod{{ID: "annual", Nodes: append([]EnergyExplanationNode(nil), bundle.EnergyExplanation.Nodes...), Links: []EnergyPathLink{link}, Quality: &EnergyPathQuality{}}}
	checks := epathSQLModelChecks{}
	add := func(group, key, unit string, q *epathSQLQuantity, target epathRealOracleTarget, status string, found, total *int) {
		t.Helper()
		if err := checks.add(group, "building", "", "annual", key, unit, q, target, status, found, total); err != nil {
			t.Fatal(err)
		}
	}
	for _, field := range []string{"value", "rawValue", "effectiveValue"} {
		target := epathSQLNodeTarget("driver", driver.DriverCategory, "cooling", "thermal")
		target.Field, target.Basis = field, driver.Basis
		q := epathSQLQuantity{Value: 12}
		if field == "value" {
			q.Value = 10
		}
		add("drivers", "wall/"+field, "kWh", &q, target, "", nil, nil)
	}
	for _, component := range []string{"", "sensible", "latent"} {
		target := epathSQLNodeTarget("load", "", "cooling", "thermal")
		target.Basis = load.Basis
		q := &epathSQLQuantity{Value: 10}
		if component != "" {
			target.Field, target.Component = "loadBreakdown", component
		}
		if component == "latent" {
			q = nil
		}
		add("loads", "cooling/"+component, "kWh", q, target, "", nil, nil)
	}
	for _, field := range []string{"fromValue", "toValue"} {
		target := epathRealOracleTarget{Collection: "links", ID: link.ID, Field: field, Relation: link.Relation, Service: "cooling", Basis: link.Basis, FromUnit: "kWh", ToUnit: "kWh"}
		add("drivers", "link/"+field, "kWh", &epathSQLQuantity{Value: 10}, target, "", nil, nil)
	}
	zero := 0
	for _, field := range []string{"drivers", "loads", "endUses", "carriers", "ratios", "driverToLoadClosedPct", "endUseToCarrierClosedPct", "zoneAllocatedPct", "unassignedPct"} {
		unit, found, total := "%", (*int)(nil), (*int)(nil)
		if !strings.HasSuffix(field, "Pct") {
			unit, found, total = "count", &zero, &zero
		}
		add("completeness", "quality/"+field, unit, nil, epathRealOracleTarget{Collection: "quality", Field: field}, "unavailable", found, total)
	}
	return bundle, checks
}

func epathSQLCoverageHasFailure(out epathSQLModelCoverageReport, text string) bool {
	for _, failure := range out.Failures {
		if strings.Contains(failure.Key+" "+failure.Message, text) {
			return true
		}
	}
	return false
}

func TestEnergyPathRealSQLModelCoverageExactRecordsAndImmutable(t *testing.T) {
	bundle, checks := epathSQLCoverageFixture(t)
	before, beforeChecks := epathSQLCoverageFixture(t)
	// The two annual presentations may order records differently, not change
	// their values, basis or source membership.
	p := &bundle.EnergyExplanation.Periods[0]
	p.Nodes[0], p.Nodes[1] = p.Nodes[1], p.Nodes[0]
	before.EnergyExplanation.Periods[0].Nodes[0], before.EnergyExplanation.Periods[0].Nodes[1] = before.EnergyExplanation.Periods[0].Nodes[1], before.EnergyExplanation.Periods[0].Nodes[0]
	out := epathSQLModelCoverage(bundle, checks)
	if len(out.Failures) != 0 || len(out.Records) != 12 {
		t.Fatalf("exact checked graph was not covered: %+v", out)
	}
	if !reflect.DeepEqual(bundle, before) || !reflect.DeepEqual(checks, beforeChecks) {
		t.Fatal("coverage changed the original candidate or compiled expectations")
	}
	for _, record := range out.Records {
		for _, field := range record.RequiredFields {
			if len(record.Selectors[field]) == 0 {
				t.Fatalf("coverage count concealed an unchecked concrete field: %+v", record)
			}
		}
	}
}

func TestEnergyPathRealSQLModelCoverageRejectsUncheckedRecordsAndSelectors(t *testing.T) {
	for _, test := range []struct {
		name, failure string
		change        func(*PurposeResultBundle, *epathSQLModelChecks)
	}{
		{"extra basis record", "nodes/extra/value", func(b *PurposeResultBundle, c *epathSQLModelChecks) {
			extra := b.EnergyExplanation.Nodes[0]
			extra.ID, extra.Basis = "extra", "reported_variable"
			b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes, extra)
			b.EnergyExplanation.Periods = nil
		}},
		{"removed required selector", "required compiled selector removed", func(b *PurposeResultBundle, c *epathSQLModelChecks) { c.Rows = c.Rows[1:] }},
		{"removed selector and registry", "nodes/wall/value", func(b *PurposeResultBundle, c *epathSQLModelChecks) {
			delete(c.Keys, c.Rows[0].Want.Key)
			c.Rows = c.Rows[1:]
		}},
		{"duplicate selector", "duplicate required selector", func(b *PurposeResultBundle, c *epathSQLModelChecks) { c.Rows = append(c.Rows, c.Rows[0]) }},
		{"wrong target context", "contradictory context", func(b *PurposeResultBundle, c *epathSQLModelChecks) { c.Rows[0].Item.Period = "M1" }},
		{"wrong target unit", "nodes/wall/value", func(b *PurposeResultBundle, c *epathSQLModelChecks) { c.Rows[0].Item.Target.Unit = "J" }},
		{"missing opposite link quantity", "links/wall-to-cooling/toValue", func(b *PurposeResultBundle, c *epathSQLModelChecks) {
			for i := range c.Rows {
				if c.Rows[i].Item.Target.Collection == "links" && c.Rows[i].Item.Target.Field == "toValue" {
					delete(c.Keys, c.Rows[i].Want.Key)
					c.Rows = append(c.Rows[:i], c.Rows[i+1:]...)
					break
				}
			}
		}},
		{"broad link selector is not per-branch proof", "links/wall-to-cooling/fromValue", func(b *PurposeResultBundle, c *epathSQLModelChecks) {
			for i := range c.Rows {
				if c.Rows[i].Item.Target.Collection == "links" {
					c.Rows[i].Item.Target.ID, c.Rows[i].Item.Target.Aggregate = "", "sum"
				}
			}
		}},
		{"quality percentage without counts", "quality/drivers/drivers", func(b *PurposeResultBundle, c *epathSQLModelChecks) {
			for i := range c.Rows {
				if c.Rows[i].Item.Target.Collection == "quality" && c.Rows[i].Item.Target.Field == "drivers" {
					c.Rows[i].Want.Found, c.Rows[i].Want.Total = nil, nil
				}
			}
		}},
		{"missing quality object", "quality/drivers/drivers", func(b *PurposeResultBundle, c *epathSQLModelChecks) {
			b.EnergyExplanation.Quality = nil
			b.EnergyExplanation.Periods = nil
		}},
		{"quality percentage without status", "quality/driverToLoadClosedPct", func(b *PurposeResultBundle, c *epathSQLModelChecks) {
			for i := range c.Rows {
				if c.Rows[i].Item.Target.Field == "driverToLoadClosedPct" {
					c.Rows[i].Want.Status = ""
				}
			}
		}},
		{"unknown support is not a primary escape hatch", "explicit-reviewed-role", func(b *PurposeResultBundle, c *epathSQLModelChecks) {
			b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes, EnergyExplanationNode{ID: "unknown", Level: "support", EndUse: "cooling", ScaleDomain: "site", Unit: "kWh", Period: "annual"})
			b.EnergyExplanation.Periods = nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle, checks := epathSQLCoverageFixture(t)
			test.change(&bundle, &checks)
			if out := epathSQLModelCoverage(bundle, checks); !epathSQLCoverageHasFailure(out, test.failure) {
				t.Fatalf("counterexample was not rejected with %q: %+v", test.failure, out.Failures)
			}
		})
	}
}

func TestEnergyPathRealSQLModelCoverageRejectsWrapperContradictions(t *testing.T) {
	for _, test := range []struct {
		name, failure string
		change        func(*EnergyExplanationResult)
	}{
		{"annual number", "contradict periods.annual", func(r *EnergyExplanationResult) { r.Periods[0].Nodes[0].Value++ }},
		{"annual basis", "contradict periods.annual", func(r *EnergyExplanationResult) { r.Periods[0].Links[0].Basis = "other" }},
		{"annual quality", "contradict periods.annual", func(r *EnergyExplanationResult) { r.Periods[0].Quality.Drivers.Found = 99 }},
		{"duplicate period", "duplicate candidate period", func(r *EnergyExplanationResult) { r.Periods = append(r.Periods, r.Periods[0]) }},
		{"duplicate node", "duplicate candidate node ID", func(r *EnergyExplanationResult) { r.Nodes = append(r.Nodes, r.Nodes[0]) }},
		{"duplicate link", "duplicate candidate link ID", func(r *EnergyExplanationResult) { r.Links = append(r.Links, r.Links[0]) }},
		{"missing endpoint", "missing target endpoint", func(r *EnergyExplanationResult) { r.Links[0].ToID = "missing" }},
		{"invalid monthly scope", "conflicting scope/period", func(r *EnergyExplanationResult) {
			r.Periods = append(r.Periods, EnergyPeriod{ID: "M1", Nodes: r.Nodes})
		}},
		{"advertised missing Zone", "advertised Zone", func(r *EnergyExplanationResult) { r.AvailableZones = []string{"Office"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle, checks := epathSQLCoverageFixture(t)
			test.change(&bundle.EnergyExplanation)
			if out := epathSQLModelCoverage(bundle, checks); !epathSQLCoverageHasFailure(out, test.failure) {
				t.Fatalf("wrapper counterexample was not rejected: %+v", out.Failures)
			}
		})
	}
}

func TestEnergyPathRealSQLModelCoverageContextDoesNotExemptPrimaryEndpoints(t *testing.T) {
	bundle, checks := epathSQLCoverageFixture(t)
	bundle.EnergyExplanation.Periods = nil
	use := EnergyExplanationNode{ID: "lighting", Level: "end_use", EndUse: "interior_lighting", ScaleDomain: "site", Unit: "kWh", Value: 25, Period: "annual", Basis: "direct_zone_energy"}
	bundle.EnergyExplanation.Nodes = append(bundle.EnergyExplanation.Nodes, use)
	bundle.EnergyExplanation.Links = append(bundle.EnergyExplanation.Links, EnergyPathLink{ID: "correspondence", FromID: "wall", ToID: use.ID, Relation: "source_correspondence", FromValue: 10, ToValue: 25, FromUnit: "kWh", ToUnit: "kWh", Period: "annual"})
	out := epathSQLModelCoverage(bundle, checks)
	if !epathSQLCoverageHasFailure(out, "nodes/lighting/value") || epathSQLCoverageHasFailure(out, "links/correspondence") {
		t.Fatalf("non-flow context either escaped primary coverage or became fake conservation: %+v", out.Failures)
	}
	for _, record := range out.Records {
		if record.ID == "correspondence" && (record.Role != "non-flow" || record.Reason == "" || len(record.RequiredFields) != 0) {
			t.Fatalf("non-flow exemption lacks exact record explanation: %+v", record)
		}
	}
}

func TestEnergyPathRealSQLModelCoverageRetainsAbsentRecordObligation(t *testing.T) {
	bundle, checks := epathSQLCoverageFixture(t)
	target := epathSQLNodeTarget("end_use", "heating", "", "site")
	target.Basis, target.AllowPrunedZero = "reported_meter", true
	if err := checks.add("endUses", "building", "", "annual", "reported-zero", "kWh", &epathSQLQuantity{}, target, "", nil, nil); err != nil {
		t.Fatal(err)
	}
	if out := epathSQLModelCoverage(bundle, checks); len(out.Failures) != 0 {
		t.Fatalf("a valid absent-zero obligation must remain a selector, not an invented node: %+v", out.Failures)
	}
	checks.Rows = checks.Rows[:len(checks.Rows)-1]
	if out := epathSQLModelCoverage(bundle, checks); !epathSQLCoverageHasFailure(out, "required compiled selector removed") {
		t.Fatal("no candidate node existed to reveal deletion of the zero-presence assertion")
	}
}

func TestEnergyPathRealSQLModelCoverageBuildingReportedSiteCardinality(t *testing.T) {
	for _, level := range []string{"end_use", "carrier"} {
		for _, split := range []bool{false, true} {
			name := level + "/extra-zero"
			if split {
				name = level + "/split-total"
			}
			t.Run(name, func(t *testing.T) {
				bundle, checks := epathSQLCoverageFixture(t)
				bundle.EnergyExplanation.Periods = nil
				node := EnergyExplanationNode{ID: "reported", Level: level, ScaleDomain: "site", Unit: "kWh", Period: "annual", Basis: "reported_meter", Value: 20}
				group, category := "endUses", "interior_lighting"
				node.EndUse = category
				if level == "carrier" {
					group, category = "carriers", "electricity"
					node.EndUse, node.Carrier = "", category
				}
				target := epathSQLNodeTarget(level, category, "", "site")
				target.Basis = "reported_meter"
				if err := checks.add(group, "building", "", "annual", "reported", "kWh", &epathSQLQuantity{Value: 20}, target, "", nil, nil); err != nil {
					t.Fatal(err)
				}
				bundle.EnergyExplanation.Nodes = append(bundle.EnergyExplanation.Nodes, node)
				if out := epathSQLModelCoverage(bundle, checks); len(out.Failures) != 0 {
					t.Fatalf("one canonical reported record was rejected: %+v", out.Failures)
				}
				extra := node
				extra.ID, extra.Value = "extra-reported", 0
				if split {
					bundle.EnergyExplanation.Nodes[len(bundle.EnergyExplanation.Nodes)-1].Value = 12
					extra.Value = 8
				}
				bundle.EnergyExplanation.Nodes = append(bundle.EnergyExplanation.Nodes, extra)
				check := checks.Rows[len(checks.Rows)-1]
				actual, err := epathReadOracleCandidate(bundle, check.Item, check.Want)
				if err != nil || actual == nil || *actual != 20 {
					t.Fatalf("counterexample must preserve the exact scalar sum: %v / %v", actual, err)
				}
				out := epathSQLModelCoverage(bundle, checks)
				if !epathSQLCoverageHasFailure(out, "duplicate Building reported-meter canonical category") {
					t.Fatalf("a matching sum concealed an additional primary record: %+v", out.Failures)
				}
			})
		}
	}
}

func TestEnergyPathRealSQLModelCoveragePreservesDriverComponentsAndParallelBranches(t *testing.T) {
	bundle, checks := epathSQLCoverageFixture(t)
	bundle.EnergyExplanation.Periods = nil
	driver := &bundle.EnergyExplanation.Nodes[0]
	driver.Value, driver.RawValue, driver.EffectiveValue = 6, 7, 7
	extra := *driver
	extra.ID, extra.ThermalComponent = "wall.other-component", "latent"
	extra.Value, extra.RawValue, extra.EffectiveValue = 4, 5, 5
	bundle.EnergyExplanation.Nodes = append(bundle.EnergyExplanation.Nodes, extra)
	bundle.EnergyExplanation.Links[0].FromValue, bundle.EnergyExplanation.Links[0].ToValue = 4, 4
	for index := range checks.Rows {
		if checks.Rows[index].Item.Target.Collection == "links" {
			checks.Rows[index].Quantity = &epathSQLQuantity{Value: 4}
			checks.Rows[index].Want.Value = epathOracleNumber(4)
		}
	}
	for _, branch := range []struct {
		id, from string
		value    float64
	}{{"parallel", "wall", 2}, {"component", extra.ID, 4}} {
		link := bundle.EnergyExplanation.Links[0]
		link.ID, link.RuleID, link.FromID = branch.id, branch.id, branch.from
		link.FromValue, link.ToValue = branch.value, branch.value
		bundle.EnergyExplanation.Links = append(bundle.EnergyExplanation.Links, link)
		for _, field := range []string{"fromValue", "toValue"} {
			target := epathRealOracleTarget{Collection: "links", ID: link.ID, Field: field, Relation: link.Relation, Service: link.ServiceKind, Basis: link.Basis, FromUnit: "kWh", ToUnit: "kWh"}
			if err := checks.add("drivers", "building", "", "annual", branch.id+"/"+field, "kWh", &epathSQLQuantity{Value: branch.value}, target, "", nil, nil); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, id := range []string{"support.first", "support.second"} {
		bundle.EnergyExplanation.Nodes = append(bundle.EnergyExplanation.Nodes, EnergyExplanationNode{ID: id, Level: "support", EndUse: "electricity_purchased", Carrier: "electricity", ScaleDomain: "site", Unit: "kWh", Period: "annual", Basis: "reported_meter"})
	}
	if out := epathSQLModelCoverage(bundle, checks); len(out.Failures) != 0 {
		t.Fatalf("site cardinality rejected legitimate driver components, parallel branches or support context: %+v", out.Failures)
	}
}

func TestEnergyPathRealSQLModelCoverageCustomZoneProofOwnsExactBranches(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		bundle, checks := epathSQLZoneServiceUnitBundle(), epathSQLZoneServiceUnitChecks(t)
		if invalid {
			period := &bundle.EnergyExplanation.ZoneResults[0].Periods[0]
			for index := range period.Links {
				if period.Links[index].ID == "site.cooling.electricity" {
					period.Links[index].ToValue++
				}
			}
		}
		out := epathSQLModelCoverage(bundle, checks)
		found := false
		for _, record := range out.Records {
			if record.Scope != "zone" || record.Zone != "A" || record.Period != "M1" || record.Collection != "links" || record.ID != "site.cooling.electricity" {
				continue
			}
			found = true
			for _, field := range []string{"fromValue", "toValue"} {
				if (len(record.Selectors[field]) > 0) == invalid {
					t.Fatalf("custom proof validity did not control exact matched branch coverage, invalid=%v: %+v", invalid, record)
				}
				for _, key := range record.Selectors[field] {
					if !strings.Contains(key, "cooling/value") || strings.Contains(key, "heating") {
						t.Fatalf("different service/scalar claimed this carrier branch: %+v", record)
					}
				}
			}
		}
		if !found {
			t.Fatal("exact Zone/service/period carrier branch absent from ledger")
		}
		// This intentionally small service-only fixture must not become full
		// acceptance: it has no independent driver/components/quality checks.
		if len(out.Failures) == 0 {
			t.Fatal("custom service proof concealed unrelated primary coverage gaps")
		}
	}
}

func TestEnergyPathRealSQLModelCoverageCopiedAccountingRequiresExactCheckedOrigin(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		row := EnergyReconciliation{ID: "allocation.cooling", Level: "allocation", Period: "annual", Unit: "kWh", ExpectedValue: 10, AllocatedValue: 9, UnassignedValue: 1}
		bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Reconciliation: []EnergyReconciliation{row}}}
		var checks epathSQLModelChecks
		for field, value := range map[string]float64{"expectedValue": 10, "directValue": 0, "allocatedValue": 9, "unassignedValue": 1} {
			target := epathRealOracleTarget{Collection: "reconciliation", ID: row.ID, Level: row.Level, Unit: "kWh", Field: field}
			if err := checks.add("zoneAllocation", "building", "", "annual", field, "kWh", &epathSQLQuantity{Value: value}, target, "", nil, nil); err != nil {
				t.Fatal(err)
			}
		}
		copy := row
		if corrupt {
			copy.AllocatedValue++
		}
		context := epathSQLCoverageContext{scope: "zone", zone: "Office", period: "annual", rows: []EnergyReconciliation{copy}}
		out := epathSQLModelCoverageReport{}
		epathSQLCoverageRecords(bundle, context, nil, map[string][]epathSQLModelCheck{epathSQLCoverageContextKey("building", "", "annual"): checks.Rows}, &out)
		found := false
		for _, record := range out.Records {
			if record.Collection != "reconciliation" {
				continue
			}
			found = true
			if (record.Role == "referenced-accounting") == corrupt {
				t.Fatalf("copied allocation context identity was not checked: %+v", record)
			}
			for _, field := range record.RequiredFields {
				if (len(record.Selectors[field]) > 0) == corrupt {
					t.Fatalf("corrupt=%v: copied context improperly inherited %s: %+v", corrupt, field, record)
				}
			}
		}
		if !found {
			t.Fatal("copied accounting missing from exact record ledger")
		}
	}
}
