package simulation

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func epathSQLAuxiliaryFlowUnitFixture(t *testing.T) (epathSQLFrames, epathRealSQLModel, PurposeResultBundle, epathSQLModelChecks) {
	t.Helper()
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{"a": {Name: "A", Multiplier: 10}, "b": {Name: "B", Multiplier: 1}, "plenum": {Name: "Plenum", Multiplier: 1}}, Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{}, SourceRaw: map[int][]epathSQLQuantity{}, SourceEffective: map[int][]epathSQLQuantity{}, Loads: map[string]epathSQLQuantity{}, LoadSourceIDs: map[string][]int{}}
	model := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}, Site: []epathRealSQLSite{{ID: "pumps.electricity", EndUse: "pumps", Carrier: "electricity", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Pumps:Electricity", Unit: "J"}}, Keys: []string{""}, IsMeter: true}}}, Auxiliaries: []epathRealSQLAuxiliary{{SiteID: "pumps.electricity", ServedZones: []string{"A", "B"}, Weight: "cooling_plus_heating", AllocationMethod: "plant_loop_load_share", ReconciliationID: "allocation.pumps.annual"}}}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	addSource := func(id int, name, owner string, meter bool, native [12]float64) {
		source := epathRealSQLSource{DictionaryIndex: id, Name: name, KeyValue: owner, IsMeter: meter, ReportingFrequency: "Monthly", SourceUnit: "J", Rows: 12}
		total := 0.0
		for month, value := range native {
			source.Months = append(source.Months, epathRealSQLMonth{Month: month + 1, Rows: 1, RawSum: epathOracleNumber(value * 3600000), EnergyKWh: epathOracleNumber(value)})
			total += value
		}
		source.RawSum, source.EnergyKWh = epathOracleNumber(total*3600000), epathOracleNumber(total)
		frames.SourceIdentities[id] = source
		values, err := epathSQLMonthly(source, model.Precision)
		if err != nil {
			t.Fatal(err)
		}
		frames.SourceRaw[id] = values
		if meter {
			frames.SiteSources["pumps.electricity"] = []int{id}
			for _, value := range values {
				q := value
				frames.Site["pumps.electricity"] = append(frames.Site["pumps.electricity"], &q)
			}
		} else {
			service := "cooling"
			if strings.Contains(name, "Heating") {
				service = "heating"
			}
			for month, value := range values {
				q := value.times(frames.Zones[strings.ToLower(owner)].Multiplier)
				frames.SourceEffective[id] = append(frames.SourceEffective[id], q)
				key := epathSQLKey(owner, service, month+1)
				frames.Loads[key], frames.LoadSourceIDs[key] = q, []int{id}
			}
		}
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), SourceType: "sql_report_data", Name: name, KeyValue: owner, IsMeter: meter, SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"})
	}
	addSource(1, "Pumps:Electricity", "", true, [12]float64{14, 20})
	addSource(10, "Zone Air System Sensible Cooling Energy", "A", false, [12]float64{1, 9})
	addSource(11, "Zone Air System Sensible Heating Energy", "A", false, [12]float64{3})
	addSource(12, "Zone Air System Sensible Cooling Energy", "B", false, [12]float64{90, 10})
	addSource(13, "Zone Air System Sensible Heating Energy", "B", false, [12]float64{10})
	addSource(14, "Zone Air System Sensible Cooling Energy", "Plenum", false, [12]float64{500, 500})
	addSource(15, "Zone Air System Sensible Heating Energy", "Plenum", false, [12]float64{500, 500})
	// Hand arithmetic: M1 A40:B100 weights split14 into4:10; M2 A90:B10
	// split20 into18:2. Annual22:12 is NOT a share of annual pooled weights.
	for _, zone := range []string{"A", "B", "Plenum"} {
		z := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: zone}}
		for _, period := range epathSQLZoneCarrierPeriods() {
			value := 0.0
			if zone == "A" {
				value = map[string]float64{"M1": 4, "M2": 18, "annual": 22}[period]
			} else if zone == "B" {
				value = map[string]float64{"M1": 10, "M2": 2, "annual": 12}[period]
			}
			p := EnergyPeriod{ID: period, Kind: "monthly"}
			if period == "annual" {
				p.Kind = "annual"
			}
			if value > 0 {
				loadIDs := []string{"sql-rdd-10"}
				if zone == "B" {
					loadIDs = []string{"sql-rdd-12"}
				}
				if period != "M2" {
					if zone == "A" {
						loadIDs = append(loadIDs, "sql-rdd-11")
					} else {
						loadIDs = append(loadIDs, "sql-rdd-13")
					}
				}
				p.Nodes = []EnergyExplanationNode{
					{ID: "pump", Level: "end_use", EndUse: "pumps", Value: value, AllocatedValue: value, AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", ZoneName: zone, Period: period, Basis: "service_path_allocation", AggregationBasis: "model_total", SourceIDs: append([]string{"sql-rdd-1"}, loadIDs...)},
					{ID: "carrier", Level: "carrier", Carrier: "electricity", Value: value, Unit: "kWh", ScaleDomain: "site", ZoneName: zone, Period: period, Basis: "service_path_allocation", AggregationBasis: "model_total"},
				}
				p.Links = []EnergyPathLink{{ID: "pump-flow", FromID: "pump", ToID: "carrier", Relation: "direct_end_use_to_carrier", Basis: "service_path_allocation", FromValue: value, ToValue: value, FromUnit: "kWh", ToUnit: "kWh", ZoneName: zone, Period: period, SourceIDs: []string{"sql-rdd-1"}}}
			}
			z.Periods = append(z.Periods, p)
			if period == "annual" {
				z.Nodes, z.Links = append([]EnergyExplanationNode(nil), p.Nodes...), append([]EnergyPathLink(nil), p.Links...)
			}
		}
		bundle.EnergyExplanation.ZoneResults = append(bundle.EnergyExplanation.ZoneResults, z)
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelAuxiliaryZoneChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return frames, model, bundle, checks
}

func TestEnergyPathRealSQLAuxiliaryFlowMonthlyFirstAndConsumptionTrace(t *testing.T) {
	frames, model, bundle, checks := epathSQLAuxiliaryFlowUnitFixture(t)
	if err := epathSQLModelAuxiliaryFlowChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 3*13*6 {
		t.Fatalf("lost exact scalar/branch/paired endpoint contexts: %d", len(checks.Rows))
	}
	for _, check := range checks.Rows {
		if check.AuxiliaryZone != nil {
			if err := epathCheckSQLAuxiliaryZone(bundle, check); err != nil {
				t.Fatalf("%s: %v", check.Item.Key, err)
			}
			continue
		}
		if err := epathCheckSQLSiteFlow(bundle, check); err != nil {
			t.Fatalf("%s: %v", check.Item.Key, err)
		}
		want := 0.0
		if check.Item.Target.Relation == "direct_end_use_to_carrier" {
			if check.Item.Zone == "A" {
				want = map[string]float64{"M1": 4, "M2": 18, "annual": 22}[check.Item.Period]
			} else if check.Item.Zone == "B" {
				want = map[string]float64{"M1": 10, "M2": 2, "annual": 12}[check.Item.Period]
			}
		}
		if check.Quantity == nil || math.Abs(check.Quantity.Value-want) > 1e-12 {
			t.Fatalf("wrong independent monthly-first flow %s: want %g, got %#v", check.Item.Key, want, check.Quantity)
		}
		if len(check.SiteFlow.Sources) != 1 || check.SiteFlow.Sources["sql-rdd-1"].RDD == nil || check.SiteFlow.Basis != "service_path_allocation" {
			t.Fatal("allocated flow invented direct pump metering or consumed its load weight")
		}
	}
}

func TestEnergyPathRealSQLAuxiliaryFlowRejectsMissingAndForgedScalarLedger(t *testing.T) {
	for _, name := range []string{"empty", "missing value", "missing allocated", "duplicate", "nil quantity", "nil proof", "changed quantity", "wrong site", "wrong carrier", "wrong Zone", "wrong month", "wrong weight", "wrong ownership", "missing meter proof", "load on consumption", "missing node weight", "annual redistributed", "extra selector", "unavailable status", "unassigned pool"} {
		t.Run(name, func(t *testing.T) {
			frames, model, _, checks := epathSQLAuxiliaryFlowUnitFixture(t)
			index := -1
			for i, check := range checks.Rows {
				period := "M1"
				if name == "annual redistributed" {
					period = "annual"
				}
				if check.Item.Zone == "A" && check.Item.Period == period && check.Item.Target.Field == "value" {
					index = i
				}
			}
			if index < 0 {
				t.Fatal("missing test scalar")
			}
			c := &checks.Rows[index]
			switch name {
			case "empty":
				checks.Rows = nil
			case "missing value":
				checks.Rows = append(checks.Rows[:index], checks.Rows[index+1:]...)
			case "missing allocated":
				checks.Rows = append(checks.Rows[:index+1], checks.Rows[index+2:]...)
			case "duplicate":
				checks.Rows = append(checks.Rows, *c)
			case "nil quantity":
				c.Quantity = nil
			case "nil proof":
				c.AuxiliaryZone = nil
			case "changed quantity":
				q := *c.Quantity
				q.Value *= 10
				c.Quantity = &q
			case "wrong site":
				c.AuxiliaryZone.SiteID = "fans.electricity"
			case "wrong carrier":
				c.AuxiliaryZone.Carrier = "natural_gas"
			case "wrong Zone":
				c.AuxiliaryZone.ZoneName = "B"
			case "wrong month":
				c.AuxiliaryZone.Period = "M2"
			case "wrong weight":
				c.AuxiliaryZone.Weight = "equal"
			case "wrong ownership":
				c.AuxiliaryZone.Owned = false
			case "missing meter proof":
				c.AuxiliaryZone.Sources = nil
			case "load on consumption":
				c.AuxiliaryZone.Sources["sql-rdd-10"] = epathSQLOriginalRDD(frames.SourceIdentities[10])
			case "missing node weight":
				delete(c.AuxiliaryZone.NodeSources, "sql-rdd-10")
			case "annual redistributed":
				c.AuxiliaryZone.Value.Value = 34 * 130.0 / 240 // Incorrect annual-weight share, not22.
			case "extra selector":
				c.Item.Target.ID = "candidate-chosen-pump"
			case "unavailable status":
				c.Want.Status = "unavailable"
			case "unassigned pool":
				model.Auxiliaries[0].Weight, model.Auxiliaries[0].AllocationMethod, model.Auxiliaries[0].ServedZones = "unassigned", "unassigned", nil
			}
			if err := epathSQLModelAuxiliaryFlowChecks(frames, model, &checks); err == nil {
				t.Fatal("invalid independent scalar ledger produced consumption proof")
			}
		})
	}
}

func TestEnergyPathRealSQLAuxiliaryFlowRejectsPhysicalBranchMutations(t *testing.T) {
	for _, name := range []string{"missing branch", "duplicate split", "wrong carrier", "wrong Zone", "wrong period", "wrong end use", "wrong relation", "false measured basis", "wrong domain", "wrong unit", "unequal endpoints", "negative energy", "fabricated ratio", "borrow weight", "missing source", "missing node weight", "wrong source metadata", "unowned Zone flow", "zero-month positive flow"} {
		t.Run(name, func(t *testing.T) {
			frames, model, bundle, checks := epathSQLAuxiliaryFlowUnitFixture(t)
			if err := epathSQLModelAuxiliaryFlowChecks(frames, model, &checks); err != nil {
				t.Fatal(err)
			}
			zone, period := "A", "M1"
			if name == "unowned Zone flow" {
				zone = "Plenum"
			} else if name == "zero-month positive flow" {
				period = "M3"
			}
			var selected *EnergyPeriod
			for i := range bundle.EnergyExplanation.ZoneResults {
				z := &bundle.EnergyExplanation.ZoneResults[i]
				if z.Scope.ZoneName == zone {
					for j := range z.Periods {
						if z.Periods[j].ID == period {
							selected = &z.Periods[j]
						}
					}
				}
			}
			if selected == nil {
				t.Fatal("missing test context")
			}
			if name == "unowned Zone flow" || name == "zero-month positive flow" {
				selected.Nodes = []EnergyExplanationNode{{ID: "pump", Level: "end_use", EndUse: "pumps", Value: 1, Unit: "kWh", ScaleDomain: "site", ZoneName: zone, Period: period, Basis: "service_path_allocation", AggregationBasis: "model_total", SourceIDs: []string{"sql-rdd-1"}}, {ID: "carrier", Level: "carrier", Carrier: "electricity", Unit: "kWh", ScaleDomain: "site", ZoneName: zone, Period: period, AggregationBasis: "model_total"}}
				selected.Links = []EnergyPathLink{{ID: "fake-flow", FromID: "pump", ToID: "carrier", Relation: "direct_end_use_to_carrier", Basis: "service_path_allocation", FromValue: 1, ToValue: 1, FromUnit: "kWh", ToUnit: "kWh", ZoneName: zone, Period: period, SourceIDs: []string{"sql-rdd-1"}}}
			} else {
				link := &selected.Links[0]
				switch name {
				case "missing branch":
					selected.Links = nil
				case "duplicate split":
					link.FromValue, link.ToValue = 2, 2
					copy := *link
					copy.ID = "extra"
					selected.Links = append(selected.Links, copy)
				case "wrong carrier":
					selected.Nodes[1].Carrier = "natural_gas"
				case "wrong Zone":
					link.ZoneName = "B"
				case "wrong period":
					link.Period = "M2"
				case "wrong end use":
					selected.Nodes[0].EndUse = "fans"
				case "wrong relation":
					link.Relation = "end_use_to_carrier"
				case "false measured basis":
					link.Basis = "direct_zone_energy"
				case "wrong domain":
					selected.Nodes[1].ScaleDomain = "thermal"
				case "wrong unit":
					link.ToUnit = "J"
				case "unequal endpoints":
					link.ToValue += 1e-10
				case "negative energy":
					link.FromValue, link.ToValue = -4, -4
				case "fabricated ratio":
					link.Ratio = 1
				case "borrow weight":
					link.SourceIDs = []string{"sql-rdd-1", "sql-rdd-10"}
				case "missing source":
					link.SourceIDs = nil
				case "missing node weight":
					selected.Nodes[0].SourceIDs = []string{"sql-rdd-1"}
				case "wrong source metadata":
					bundle.EnergyExplanation.Sources[0].Name = "Fans:Electricity"
				}
			}
			found, failed := 0, 0
			for _, check := range checks.Rows {
				if check.SiteFlow != nil && check.Item.Zone == zone && check.Item.Period == period {
					found++
					if epathCheckSQLSiteFlow(bundle, check) != nil {
						failed++
					}
				}
			}
			if found != 4 || failed != 4 {
				t.Fatalf("wrong exact consumption branch escaped paired proof: %d/%d", failed, found)
			}
		})
	}
}

func TestEnergyPathRealSQLAuxiliaryFlowRequiresObservedTwelveMonthInput(t *testing.T) {
	for _, name := range []string{"missing month", "NULL source", "duplicate month", "nonfinite meter", "missing load", "zero is not absence", "unchanged unassigned"} {
		t.Run(name, func(t *testing.T) {
			frames, model, _, checks := epathSQLAuxiliaryFlowUnitFixture(t)
			switch name {
			case "missing month":
				frames.Site["pumps.electricity"][2] = nil
			case "NULL source":
				source := frames.SourceIdentities[1]
				source.Months[2].EnergyKWh = nil
				frames.SourceIdentities[1] = source
			case "duplicate month":
				source := frames.SourceIdentities[1]
				source.Months[2].Rows = 2
				frames.SourceIdentities[1] = source
			case "nonfinite meter":
				frames.Site["pumps.electricity"][2].Value = math.Inf(1)
			case "missing load":
				delete(frames.Loads, epathSQLKey("A", "heating", 3))
			case "unchanged unassigned":
				model.Auxiliaries[0].Weight, model.Auxiliaries[0].AllocationMethod, model.Auxiliaries[0].ServedZones = "unassigned", "unassigned", nil
				checks = epathSQLModelChecks{}
			}
			err := epathSQLModelAuxiliaryFlowChecks(frames, model, &checks)
			valid := name == "zero is not absence" || name == "unchanged unassigned"
			if (err == nil) != valid {
				t.Fatalf("known/unknown input semantics changed: %v", err)
			}
			if name == "unchanged unassigned" && len(checks.Rows) != 0 {
				t.Fatal("unassigned pool acquired invented Zone obligations")
			}
		})
	}
}
