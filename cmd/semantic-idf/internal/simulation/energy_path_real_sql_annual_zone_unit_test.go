package simulation

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func epathSQLAnnualZoneUnitFrames(t *testing.T) (epathSQLFrames, epathRealSQLModel) {
	t.Helper()
	frames, model := epathSQLZoneServiceUnitFrames()
	frames.Site = map[string][]*epathSQLQuantity{}
	frames.SiteSources = map[string][]int{}
	frames.SiteAnnual = map[string]epathSQLTabularObservation{}
	model.Site = nil
	path := epathSQLTabularUnitFixture(t)
	epathOracleEditSQL(t, path, `UPDATE TabularDataWithStrings SET Value='120.00' WHERE TabularDataIndex=1;
INSERT INTO TabularDataWithStrings VALUES(6,'60.00','AnnualBuildingUtilityPerformanceSummary','Entire Facility','End Uses','Heating','District Heating Water','kWh');`)
	for i, service := range []string{"cooling", "heating"} {
		selector := epathSQLTabularUnitSelector()
		carrier := "district_cooling"
		if service == "heating" {
			selector.RowName = "Heating"
			selector.ColumnName = "District Heating Water"
			carrier = "district_heating"
		}
		observation, err := epathReadSQLModelTabular(path, selector)
		if err != nil || observation == nil {
			t.Fatal(err)
		}
		id := service + ".district"
		model.Site = append(model.Site, epathRealSQLSite{ID: id, EndUse: service, Carrier: carrier, Tabular: &selector})
		frames.SiteAnnual[id] = *observation
		model.Services[i].SiteIDs = []string{id}
		model.Services[i].RatioKind = "load_to_purchased_energy"
		model.Services[i].FallbackRatioKind = "load_to_purchased_energy"
	}
	return frames, model
}

// Deliberately hand-authored: annual cooling120 allocates60/60 from annual
// loads100/100, not the old monthly fixture's190/110 allocation. Heating60
// allocates45/15. Neither annual site source is distributed into any month.
func epathSQLAnnualZoneUnitBundle() PurposeResultBundle {
	frames, _ := epathSQLZoneServiceUnitFrames()
	b := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	for id, s := range frames.SourceIdentities {
		if id >= 10 {
			b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), SourceType: "sql_report_data", Name: s.Name, KeyValue: s.KeyValue, SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"})
		}
	}
	for _, service := range []string{"cooling", "heating"} {
		name, row, column, value := "Cooling:DistrictCooling", "Cooling", "District Cooling", 120.0
		if service == "heating" {
			name, row, column, value = "Heating:DistrictHeating", "Heating", "District Heating Water", 60
		}
		b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, EnergyDataSource{ID: "tabular-" + service, SourceType: "sql_tabular", IsMeter: true, Name: name, KeyValue: name, Units: "kWh", SourceUnit: "kWh", NormalizedUnit: "kWh", ReportingFrequency: "Annual", AggregationMethod: "tabular_annual_value", AggregationBasis: "model_total", TableName: "End Uses", RowName: row, ColumnName: column + " [kWh]", RawValue: value, EffectiveValue: value})
	}
	for _, zone := range []string{"A", "B", "Plenum"} {
		z := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: zone, AggregationBasis: "model_total"}}
		for _, period := range epathSQLZoneCarrierPeriods() {
			p := EnergyPeriod{ID: period, Kind: "monthly"}
			if period == "annual" {
				p.Kind = "annual"
			}
			if zone != "Plenum" && period == "annual" {
				for _, service := range []string{"cooling", "heating"} {
					load, site, carrier := 100.0, 60.0, "district_cooling"
					if service == "heating" {
						load, site, carrier = 30, 45, "district_heating"
						if zone == "B" {
							load, site = 10, 15
						}
					}
					loadID, useID, carrierID := "load."+service, "use."+service, "carrier."+carrier
					trace := []string{fmt.Sprintf("sql-rdd-%d", epathSQLZoneServiceUnitLoadID(zone, service)), "tabular-" + service}
					p.Nodes = append(p.Nodes, EnergyExplanationNode{ID: loadID, Level: "load", ServiceKind: service, Value: load, Unit: "kWh", ScaleDomain: "thermal", Basis: "reported_variable", ZoneName: zone, Period: period}, EnergyExplanationNode{ID: useID, Level: "end_use", EndUse: service, Value: site, AllocatedValue: site, AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", ZoneName: zone, Period: period, SourceIDs: trace}, EnergyExplanationNode{ID: carrierID, Level: "carrier", Carrier: carrier, EndUse: "total", Value: site, AllocatedValue: site, AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", AggregationBasis: "model_total", ZoneName: zone, Period: period})
					p.Links = append(p.Links, EnergyPathLink{ID: "conversion." + service, FromID: loadID, ToID: useID, Relation: "load_to_end_use", ServiceKind: service, Basis: "service_path_allocation", FromValue: load, ToValue: site, FromUnit: "kWh", ToUnit: "kWh", Ratio: load / site, RatioKind: "load_to_purchased_energy", SourceIDs: trace, ZoneName: zone, Period: period}, EnergyPathLink{ID: "site." + service, FromID: useID, ToID: carrierID, Relation: "end_use_to_carrier", Basis: "service_path_allocation", FromValue: site, ToValue: site, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"tabular-" + service}, ZoneName: zone, Period: period})
					p.Reconciliation = append(p.Reconciliation, EnergyReconciliation{ID: "reconcile.energy." + carrier + ".annual." + strings.ToLower(zone) + ".annual", Level: "energy", ZoneName: zone, Period: period, Unit: "kWh", Basis: "service_path_allocation", Status: "partial", ExpectedValue: site, ExplainedValue: site})
				}
			}
			if period == "annual" {
				z.Nodes = append([]EnergyExplanationNode(nil), p.Nodes...)
				z.Links = append([]EnergyPathLink(nil), p.Links...)
				z.Reconciliation = append([]EnergyReconciliation(nil), p.Reconciliation...)
			}
			z.Periods = append(z.Periods, p)
		}
		b.EnergyExplanation.ZoneResults = append(b.EnergyExplanation.ZoneResults, z)
	}
	return b
}

func epathSQLAnnualZoneUnitChecks(t *testing.T) epathSQLModelChecks {
	t.Helper()
	frames, model := epathSQLAnnualZoneUnitFrames(t)
	var checks epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelZoneCarrierChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return checks
}

func TestEnergyPathRealSQLAnnualZoneTemporalAllocation(t *testing.T) {
	frames, model := epathSQLAnnualZoneUnitFrames(t)
	before := fmt.Sprintf("%#v", frames.SiteAnnual)
	var checks epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelZoneCarrierChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, epathSQLAnnualZoneUnitBundle(), checks); len(failures) > 0 {
		t.Fatalf("independent annual hand fixture: %v", failures[:min(8, len(failures))])
	}
	if len(checks.Rows) != 3*2*13*(4+5) {
		t.Fatalf("lost required temporal obligations: %d", len(checks.Rows))
	}
	for _, c := range checks.Rows {
		if c.Item.Period != "annual" && ((c.ZoneService != nil) || (c.ZoneCarrier != nil)) {
			if c.Quantity != nil || c.Want.Value != nil || c.Want.Status != "unavailable" {
				t.Fatal("monthly observation fabricated")
			}
		}
		if c.ZoneService != nil && c.Item.Zone == "A" && c.Item.Period == "annual" && strings.HasSuffix(c.Want.Key, "cooling/value") {
			if c.Quantity.Value != 60 || !c.ZoneService.AnnualTabular {
				t.Fatal("annual allocation reused monthly weights")
			}
			low, high := c.Quantity.bounds()
			if !(low < 60 && high > 60) {
				t.Fatal("Tabular display uncertainty lost")
			}
		}
	}
	if before != fmt.Sprintf("%#v", frames.SiteAnnual) || len(frames.Site) != 0 {
		t.Fatal("annual inputs were modified/distributed")
	}
}

func TestEnergyPathRealSQLAnnualZoneRejectFabrication(t *testing.T) {
	checks := epathSQLAnnualZoneUnitChecks(t)
	for name, mutate := range map[string]func(*PurposeResultBundle){
		"monthly zero end use": func(b *PurposeResultBundle) {
			p := &b.EnergyExplanation.ZoneResults[0].Periods[1]
			p.Nodes = append(p.Nodes, EnergyExplanationNode{ID: "invented", Level: "end_use", EndUse: "cooling", Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", Period: "M1", ZoneName: "A"})
		},
		"monthly zero carrier": func(b *PurposeResultBundle) {
			p := &b.EnergyExplanation.ZoneResults[0].Periods[1]
			p.Nodes = append(p.Nodes, EnergyExplanationNode{ID: "invented", Level: "carrier", Carrier: "district_cooling", Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", AggregationBasis: "model_total", Period: "M1", ZoneName: "A"})
		},
		"monthly zero reconciliation": func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[1].Reconciliation = []EnergyReconciliation{{ID: "reconcile.energy.district_cooling.M1.a", Level: "energy", Period: "M1", ZoneName: "A", Unit: "kWh", Basis: "service_path_allocation"}}
		},
		"unowned zero consumption": func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[2].Nodes = []EnergyExplanationNode{{ID: "unowned", Level: "end_use", EndUse: "cooling", Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", Period: "annual", ZoneName: "Plenum"}}
		},
		"annual source monthly frequency": func(b *PurposeResultBundle) {
			for i := range b.EnergyExplanation.Sources {
				if b.EnergyExplanation.Sources[i].ID == "tabular-cooling" {
					b.EnergyExplanation.Sources[i].ReportingFrequency = "Monthly"
				}
			}
		},
		"annual source wrong column": func(b *PurposeResultBundle) {
			for i := range b.EnergyExplanation.Sources {
				if b.EnergyExplanation.Sources[i].ID == "tabular-cooling" {
					b.EnergyExplanation.Sources[i].ColumnName = "District Heating Water [kWh]"
				}
			}
		},
		"annual source wrong unit": func(b *PurposeResultBundle) {
			for i := range b.EnergyExplanation.Sources {
				if b.EnergyExplanation.Sources[i].ID == "tabular-cooling" {
					b.EnergyExplanation.Sources[i].SourceUnit = "J"
				}
			}
		},
		"missing annual source": func(b *PurposeResultBundle) {
			for i, s := range b.EnergyExplanation.Sources {
				if s.ID == "tabular-cooling" {
					b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources[:i], b.EnergyExplanation.Sources[i+1:]...)
					break
				}
			}
		},
		"swapped annual carrier": func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Nodes[2].Carrier = "district_heating" },
		"annual monthly-reweighted value": func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Nodes[1].Value = 76
			b.EnergyExplanation.ZoneResults[0].Nodes[1].AllocatedValue = 76
		},
		"annual wrong typed Zone": func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Nodes[1].ZoneName = "B" },
		"annual duplicate branch": func(b *PurposeResultBundle) {
			z := &b.EnergyExplanation.ZoneResults[0]
			l := z.Links[1]
			l.ID += ".extra"
			z.Links = append(z.Links, l)
		},
		"annual zero duplicate end use": func(b *PurposeResultBundle) {
			z := &b.EnergyExplanation.ZoneResults[0]
			n := z.Nodes[1]
			n.ID += ".duplicate"
			n.Value, n.AllocatedValue = 0, 0
			z.Nodes = append(z.Nodes, n)
		},
	} {
		t.Run(name, func(t *testing.T) {
			b := epathSQLAnnualZoneUnitBundle()
			mutate(&b)
			if len(epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, b, checks)) == 0 {
				t.Fatal("invalid annual/monthly data accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLAnnualZoneRejectIncompleteProof(t *testing.T) {
	for name, mutate := range map[string]func(*epathSQLFrames, *epathRealSQLModel){
		"missing monthly load": func(f *epathSQLFrames, m *epathRealSQLModel) { delete(f.Loads, epathSQLKey("a", "cooling", 7)) },
		"missing source":       func(f *epathSQLFrames, m *epathRealSQLModel) { delete(f.SiteAnnual, "cooling.district") },
		"duplicate pool": func(f *epathSQLFrames, m *epathRealSQLModel) {
			m.Services[0].SiteIDs = append(m.Services[0].SiteIDs, "cooling.district")
		},
		"carrier swap":  func(f *epathSQLFrames, m *epathRealSQLModel) { m.Site[0].Carrier = "district_heating" },
		"unknown owner": func(f *epathSQLFrames, m *epathRealSQLModel) { m.Services[0].ServedZones = []string{"Unknown"} },
		"dual temporal source": func(f *epathSQLFrames, m *epathRealSQLModel) {
			f.Site["cooling.district"] = make([]*epathSQLQuantity, 12)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f, m := epathSQLAnnualZoneUnitFrames(t)
			mutate(&f, &m)
			if err := epathSQLModelZoneServiceChecks(f, m, &epathSQLModelChecks{}); err == nil {
				t.Fatal("invalid temporal/source model accepted")
			}
		})
	}
	f, m := epathSQLAnnualZoneUnitFrames(t)
	var c epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(f, m, &c); err != nil {
		t.Fatal(err)
	}
	for i := range c.Rows {
		if c.Rows[i].ZoneService != nil && c.Rows[i].Item.Period == "M1" {
			c.Rows = append(c.Rows[:i], c.Rows[i+1:]...)
			break
		}
	}
	if err := epathSQLModelZoneCarrierChecks(f, m, &c); err == nil {
		t.Fatal("missing mandatory monthly absence proof accepted")
	}
	if !reflect.DeepEqual(m.Services[0].ServedZones, []string{"A", "B"}) {
		t.Fatal("ownership fixture changed")
	}
}
