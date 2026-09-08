package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func epathSQLZoneServiceUnitFrames() (epathSQLFrames, epathRealSQLModel) {
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{"a": {Name: "A", Multiplier: 1}, "b": {Name: "B", Multiplier: 1}, "plenum": {Name: "Plenum", Multiplier: 1}}, Loads: map[string]epathSQLQuantity{}, Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{}, LoadSourceIDs: map[string][]int{}, SourceRaw: map[int][]epathSQLQuantity{}}
	model := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}, Site: []epathRealSQLSite{
		{ID: "cool.e", EndUse: "cooling", Carrier: "electricity"},
		{ID: "heat.e", EndUse: "heating", Carrier: "electricity"},
		{ID: "heat.g", EndUse: "heating", Carrier: "natural_gas"},
	}}
	for index, site := range model.Site {
		id := index + 1
		frames.SiteSources[site.ID] = []int{id}
		frames.SourceIdentities[id] = epathRealSQLSource{DictionaryIndex: id, Name: site.ID + " observed meter", IsMeter: true, ReportingFrequency: "Monthly", SourceUnit: "J"}
		frames.Site[site.ID] = make([]*epathSQLQuantity, 12)
		for month := 0; month < 12; month++ {
			frames.Site[site.ID][month] = &epathSQLQuantity{}
		}
	}
	for _, service := range []string{"cooling", "heating"} {
		ids, kind := []string{"cool.e"}, "coefficient_of_performance"
		if service == "heating" {
			ids, kind = []string{"heat.e", "heat.g"}, "load_to_site_energy"
		}
		model.Services = append(model.Services, epathRealSQLService{Service: service, SiteIDs: ids, ServedZones: []string{"A", "B"}, Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: kind, FallbackRatioKind: kind, ReconciliationID: "allocation." + service + ".annual"})
		for month := 1; month <= 12; month++ {
			for zone := range frames.Zones {
				id := epathSQLZoneServiceUnitLoadID(zone, service)
				frames.SourceIdentities[id] = epathRealSQLSource{DictionaryIndex: id, Name: service + " delivered load", KeyValue: frames.Zones[zone].Name, ReportingFrequency: "Monthly", SourceUnit: "J"}
				frames.LoadSourceIDs[epathSQLKey(zone, service, month)] = []int{id}
				q := epathSQLQuantity{}
				if zone == "plenum" {
					q.Value = 500 // Unserved, even when all served loads are zero.
				}
				frames.Loads[epathSQLKey(zone, service, month)] = q
			}
		}
	}
	for key, value := range map[string]float64{
		epathSQLKey("a", "cooling", 1): 10, epathSQLKey("b", "cooling", 1): 90,
		epathSQLKey("a", "cooling", 2): 90, epathSQLKey("b", "cooling", 2): 10,
		epathSQLKey("a", "heating", 1): 30, epathSQLKey("b", "heating", 1): 10,
	} {
		frames.Loads[key] = epathSQLQuantity{Value: value}
	}
	frames.Site["cool.e"][0].Value, frames.Site["cool.e"][1].Value = 100, 200
	frames.Site["heat.e"][0].Value, frames.Site["heat.g"][0].Value = 20, 40
	frames.Site["heat.e"][1].Value, frames.Site["heat.g"][1].Value = 5, 15
	for id, indexes := range frames.SiteSources {
		for _, q := range frames.Site[id] {
			frames.SourceRaw[indexes[0]] = append(frames.SourceRaw[indexes[0]], *q)
		}
	}
	return frames, model
}

func epathSQLZoneServiceUnitLoadID(zone, service string) int {
	index := map[string]int{"a": 10, "b": 12, "plenum": 14}[strings.ToLower(zone)]
	if service == "heating" {
		index++
	}
	return index
}

// This hand-authored candidate uses deliberately nonuniform monthly shares.
// It does not consume compiled checks or a production graph/allocator builder.
func epathSQLZoneServiceUnitBundle() PurposeResultBundle {
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	frames, _ := epathSQLZoneServiceUnitFrames()
	ids := []int{}
	for id := range frames.SourceIdentities {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		original := frames.SourceIdentities[id]
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), Name: original.Name, KeyValue: original.KeyValue, SourceType: "sql_report_data", SourceUnit: original.SourceUnit, NormalizedUnit: "kWh", ReportingFrequency: original.ReportingFrequency, IsMeter: original.IsMeter})
	}
	bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: "sql-rdd-99", Name: "Zone Lights Electricity Energy", KeyValue: "A", SourceType: "sql_report_data", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"})
	type values struct{ coolingLoad, coolingSite, heatingLoad, heatingElectric, heatingGas float64 }
	months := map[string][2]values{
		"A": {{10, 10, 30, 15, 30}, {90, 180, 0, 0, 0}},
		"B": {{90, 90, 10, 5, 10}, {10, 20, 0, 0, 0}},
	}
	for _, zone := range []string{"A", "B", "Plenum"} {
		result := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: zone}}
		for _, period := range []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"} {
			v := values{}
			if pair, ok := months[zone]; ok {
				if period == "annual" {
					v = values{pair[0].coolingLoad + pair[1].coolingLoad, pair[0].coolingSite + pair[1].coolingSite, pair[0].heatingLoad, pair[0].heatingElectric, pair[0].heatingGas}
				} else if period == "M1" {
					v = pair[0]
				} else if period == "M2" {
					v = pair[1]
				}
			}
			nodes, links := []EnergyExplanationNode{}, []EnergyPathLink{}
			for _, carrier := range []string{"electricity", "natural_gas"} {
				value := v.coolingSite + v.heatingElectric
				if carrier == "natural_gas" {
					value = v.heatingGas
				}
				if value > 0 {
					nodes = append(nodes, EnergyExplanationNode{ID: carrier, Level: "carrier", Carrier: carrier, Value: value, Unit: "kWh", ScaleDomain: "site", ZoneName: zone, Period: period})
				}
			}
			for _, service := range []string{"cooling", "heating"} {
				load, energy, kind := v.coolingLoad, v.coolingSite, "coefficient_of_performance"
				if service == "heating" {
					load, energy, kind = v.heatingLoad, v.heatingElectric+v.heatingGas, "load_to_site_energy"
				}
				if energy == 0 {
					continue
				}
				trace := []string{fmt.Sprintf("sql-rdd-%d", epathSQLZoneServiceUnitLoadID(zone, service)), "sql-rdd-1"}
				if service == "heating" {
					trace = []string{trace[0], "sql-rdd-2", "sql-rdd-3"}
				}
				nodes = append(nodes,
					EnergyExplanationNode{ID: "load." + service, Level: "load", ServiceKind: service, Value: load, Unit: "kWh", ScaleDomain: "thermal", ZoneName: zone, Period: period, Basis: "reported_variable"},
					EnergyExplanationNode{ID: "use." + service, Level: "end_use", EndUse: service, Value: energy, AllocatedValue: energy, AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", ZoneName: zone, Period: period, Basis: "service_path_allocation", SourceIDs: trace})
				links = append(links, EnergyPathLink{ID: "convert." + service, FromID: "load." + service, ToID: "use." + service, Relation: "load_to_end_use", ServiceKind: service, Basis: "service_path_allocation", FromValue: load, ToValue: energy, FromUnit: "kWh", ToUnit: "kWh", Ratio: load / energy, RatioKind: kind, SourceIDs: trace, ZoneName: zone, Period: period})
				for _, carrier := range []string{"electricity", "natural_gas"} {
					value, source := v.coolingSite, "sql-rdd-1"
					if service == "heating" {
						value, source = v.heatingElectric, "sql-rdd-2"
						if carrier == "natural_gas" {
							value, source = v.heatingGas, "sql-rdd-3"
						}
					} else if carrier == "natural_gas" {
						continue
					}
					if value > 0 {
						links = append(links, EnergyPathLink{ID: "site." + service + "." + carrier, FromID: "use." + service, ToID: carrier, Relation: "end_use_to_carrier", Basis: "service_path_allocation", FromValue: value, ToValue: value, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{source}, ZoneName: zone, Period: period})
					}
				}
			}
			if period == "annual" {
				result.Nodes, result.Links = nodes, links
			} else {
				result.Periods = append(result.Periods, EnergyPeriod{ID: period, Nodes: nodes, Links: links})
			}
		}
		bundle.EnergyExplanation.ZoneResults = append(bundle.EnergyExplanation.ZoneResults, result)
	}
	return bundle
}

func epathSQLZoneServiceUnitChecks(t *testing.T) epathSQLModelChecks {
	t.Helper()
	frames, model := epathSQLZoneServiceUnitFrames()
	var checks epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return checks
}

func TestEnergyPathRealSQLZoneServiceMonthlyBeforeAnnual(t *testing.T) {
	checks := epathSQLZoneServiceUnitChecks(t)
	if len(checks.Rows) != 3*2*13*4 {
		t.Fatalf("missing Zone/service/month/annual obligations: %d", len(checks.Rows))
	}
	bundle := epathSQLZoneServiceUnitBundle()
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 0 {
		t.Fatalf("hand-authored independent Zone shares failed: %v", failures[:min(5, len(failures))])
	}
	found := false
	for _, check := range checks.Rows {
		if check.Item.Zone == "A" && check.Item.Period == "annual" && strings.HasSuffix(check.Item.Key, "cooling/value") {
			found = true
			if check.Quantity.Value != 190 || check.Quantity.Value == 150 || check.ZoneService.Carriers["electricity"].Value != 190 {
				t.Fatal("annual load-share shortcut replaced monthly 100*.1 +200*.9")
			}
		}
	}
	if !found {
		t.Fatal("annual per-Zone site and carrier proof absent")
	}
}

func TestEnergyPathRealSQLZoneServiceSwapPreservesBuildingButFails(t *testing.T) {
	checks, bundle := epathSQLZoneServiceUnitChecks(t), epathSQLZoneServiceUnitBundle()
	for index, wrong := range []float64{90, 10} {
		period := &bundle.EnergyExplanation.ZoneResults[index].Periods[0]
		delta := wrong - []float64{10, 90}[index]
		for i := range period.Nodes {
			if period.Nodes[i].ID == "use.cooling" {
				period.Nodes[i].Value, period.Nodes[i].AllocatedValue = wrong, wrong
			} else if period.Nodes[i].ID == "electricity" {
				period.Nodes[i].Value += delta // Keep each Zone's outward carrier closure too.
			}
		}
		for i := range period.Links {
			link := &period.Links[i]
			if link.ID == "site.cooling.electricity" {
				link.FromValue, link.ToValue = wrong, wrong
			} else if link.ID == "convert.cooling" {
				link.ToValue, link.Ratio = wrong, link.FromValue/wrong
			}
		}
	}
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) == 0 {
		t.Fatal("swapping A10/B90 into A90/B10 escaped because Building remains100")
	}
}

func TestEnergyPathRealSQLZoneServiceZeroLoadStaysUnassigned(t *testing.T) {
	frames, model := epathSQLZoneServiceUnitFrames()
	var checks epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, check := range checks.Rows {
		if check.Item.Scope == "building" && check.Item.Period == "M2" && strings.Contains(check.Item.Key, "heating/") {
			for field, want := range map[string]float64{"expectedValue": 20, "allocatedValue": 0, "unassignedValue": 20} {
				if check.Item.Target.Field == field {
					seen[field] = true
					if check.Quantity == nil || check.Quantity.Value != want {
						t.Fatalf("positive heating pool lost unassigned ledger: %s %#v", field, check.Quantity)
					}
				}
			}
		}
		if check.ZoneService != nil && check.ZoneService.Service == "heating" && check.Item.Period == "M2" {
			if check.Quantity.Value != 0 {
				t.Fatal("known zero served-load denominator broadened to positive unserved plenum")
			}
		}
	}
	if len(seen) != 3 {
		t.Fatal("zero-load case lacks complete assigned/unassigned proof")
	}
	zoneChecks := epathSQLZoneServiceUnitChecks(t)
	for _, basis := range []string{"service_path_allocation", "zone_load_allocation", "reported_variable"} {
		bundle := epathSQLZoneServiceUnitBundle()
		plenum := &bundle.EnergyExplanation.ZoneResults[2].Periods[1]
		plenum.Nodes = []EnergyExplanationNode{{ID: "escaped", Level: "end_use", EndUse: "heating", Value: 20, AllocatedValue: 20, AllocationApplied: true, Basis: basis, Unit: "kWh", ScaleDomain: "site", ZoneName: "Plenum", Period: "M2"}}
		if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, zoneChecks); len(failures) == 0 {
			t.Fatalf("positive unserved plenum escaped the zero proof using basis %s", basis)
		}
	}
}

func TestEnergyPathRealSQLZoneServiceCarrierAndEndpointGuards(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*EnergyPeriod)
	}{
		{"wrong carrier preserving total", func(p *EnergyPeriod) { p.Links[4].ToID = "electricity" }},
		{"one wrong side", func(p *EnergyPeriod) { p.Links[1].ToValue++ }},
		{"reverse site branch", func(p *EnergyPeriod) { p.Links[1].FromID, p.Links[1].ToID = p.Links[1].ToID, p.Links[1].FromID }},
		{"wrong service", func(p *EnergyPeriod) { p.Links[1].ServiceKind = "heating" }},
		{"wrong basis", func(p *EnergyPeriod) { p.Links[1].Basis = "zone_load_allocation" }},
		{"wrong unit", func(p *EnergyPeriod) { p.Links[1].ToUnit = "MJ" }},
		{"ratio on site branch", func(p *EnergyPeriod) { p.Links[1].RatioKind = "efficiency" }},
		{"missing provenance", func(p *EnergyPeriod) { p.Links[1].SourceIDs = []string{"missing"} }},
		{"nonfinite", func(p *EnergyPeriod) { p.Links[1].FromValue = math.NaN() }},
		{"unreviewed additional carrier", func(p *EnergyPeriod) { p.Nodes[1].Carrier = "district_heating" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle, checks := epathSQLZoneServiceUnitBundle(), epathSQLZoneServiceUnitChecks(t)
			test.mutate(&bundle.EnergyExplanation.ZoneResults[0].Periods[0])
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) == 0 {
				t.Fatal("invalid carrier-specific or endpoint evidence accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLZoneServiceUnknownNeverZero(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*epathSQLFrames, *epathRealSQLModel)
	}{
		{"missing served month", func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.Loads, epathSQLKey("a", "cooling", 1)) }},
		{"missing unserved month", func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.Loads, epathSQLKey("plenum", "cooling", 1)) }},
		{"null site month", func(f *epathSQLFrames, _ *epathRealSQLModel) { f.Site["cool.e"][0] = nil }},
		{"short site series", func(f *epathSQLFrames, _ *epathRealSQLModel) { f.Site["cool.e"] = f.Site["cool.e"][:11] }},
		{"invalid load", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.Loads[epathSQLKey("a", "cooling", 1)] = epathSQLQuantity{Value: -1}
		}},
		{"unknown zero denominator", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.Loads[epathSQLKey("a", "heating", 2)] = epathSQLQuantity{Error: .001}
		}},
		{"duplicate pool", func(_ *epathSQLFrames, m *epathRealSQLModel) {
			m.Services[0].SiteIDs = append(m.Services[0].SiteIDs, "cool.e")
		}},
		{"wrong service pool", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Services[0].SiteIDs = []string{"heat.e"} }},
		{"duplicate Zone", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Services[0].ServedZones = []string{"A", "a"} }},
		{"unknown Zone", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Services[0].ServedZones = []string{"Absent"} }},
		{"nonfinite pool", func(f *epathSQLFrames, _ *epathRealSQLModel) { f.Site["cool.e"][0].Value = math.Inf(1) }},
		{"unsupported precision", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Precision.DecimalPlaces = 2 }},
		{"missing site identity", func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.SiteSources, "cool.e") }},
		{"missing original metadata", func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.SourceIdentities, 1) }},
		{"wrong source month count", func(f *epathSQLFrames, _ *epathRealSQLModel) { f.SourceRaw[1] = f.SourceRaw[1][:11] }},
		{"missing load identity", func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.LoadSourceIDs, epathSQLKey("a", "cooling", 1)) }},
		{"foreign Zone load identity", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.LoadSourceIDs[epathSQLKey("a", "cooling", 1)] = []int{12}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			frames, model := epathSQLZoneServiceUnitFrames()
			test.mutate(&frames, &model)
			if err := epathSQLModelZoneServiceChecks(frames, model, &epathSQLModelChecks{}); err == nil {
				t.Fatal("missing/contradictory evidence became zero or broadened scope")
			}
		})
	}
}

func TestEnergyPathRealSQLZoneServiceExactSQLProvenance(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*PurposeResultBundle)
	}{
		{"carrier uses real lighting source", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Links[1].SourceIDs = []string{"sql-rdd-99"}
		}},
		{"gas uses real electricity source", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Links[4].SourceIDs = []string{"sql-rdd-2"}
		}},
		{"carrier borrows approved load context", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Links[1].SourceIDs = []string{"sql-rdd-10"}
		}},
		{"node uses lighting", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Nodes[3].SourceIDs = []string{"sql-rdd-99"}
		}},
		{"conversion uses other Zone load", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Links[0].SourceIDs = []string{"sql-rdd-12", "sql-rdd-1"}
		}},
		{"conversion lost thermal source", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Links[0].SourceIDs = []string{"sql-rdd-1"}
		}},
		{"node lost positive site source", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Nodes[3].SourceIDs = []string{"sql-rdd-10"}
		}},
		{"duplicate trace ID", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Links[1].SourceIDs = []string{"sql-rdd-1", "sql-rdd-1"}
		}},
		{"source name rebound", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].Name = "Zone Lights Electricity Energy" }},
		{"source meter flag rebound", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].IsMeter = false }},
		{"source frequency rebound", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].ReportingFrequency = "Hourly" }},
		{"source raw unit rebound", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].SourceUnit = "W" }},
		{"source normalized unit rebound", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].NormalizedUnit = "MJ" }},
		{"source type rebound", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].SourceType = "derived" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle, checks := epathSQLZoneServiceUnitBundle(), epathSQLZoneServiceUnitChecks(t)
			test.mutate(&bundle)
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) == 0 {
				t.Fatal("existing but unrelated/rebound source falsely proves Zone service provenance")
			}
		})
	}
	for _, explicitService := range []bool{false, true} {
		bundle, checks := epathSQLZoneServiceUnitBundle(), epathSQLZoneServiceUnitChecks(t)
		period := &bundle.EnergyExplanation.ZoneResults[0].Periods[0]
		period.Nodes[3].SourceIDs = []string{"sql-rdd-1"} // Zone load is optional context on a site node.
		if explicitService {
			period.Links[1].ServiceKind = "cooling"
		}
		if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 0 {
			t.Fatalf("exact EndUse identity with optional noncontradictory service: %v", failures[:min(5, len(failures))])
		}
	}
}

func TestEnergyPathRealSQLZoneServiceStableOrderingAndCoverage(t *testing.T) {
	frames, model := epathSQLZoneServiceUnitFrames()
	var before epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &before); err != nil {
		t.Fatal(err)
	}
	model.Services[1].SiteIDs = []string{"heat.g", "heat.e"}
	model.Services[0].ServedZones = []string{"B", "A"}
	var after epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("input pool/Zone ordering changed independent expectations")
	}
	for _, check := range before.Rows {
		if check.ZoneService == nil || check.Item.Zone != "A" || check.Item.Period != "M1" || check.ZoneService.Service != "cooling" {
			continue
		}
		period := epathSQLZoneServiceUnitBundle().EnergyExplanation.ZoneResults[0].Periods[0]
		count := 0
		for _, link := range period.Links {
			if epathSQLZoneServiceCoveredLink(period.Nodes, link, check.ZoneService) {
				count++
				if link.ID != "site.cooling.electricity" {
					t.Fatal("broad predicate falsely covered heating or conversion")
				}
			}
		}
		if count != 1 {
			t.Fatal("exact carrier record coverage missing")
		}
	}
	encoded, err := json.Marshal(before.Rows[0].Want)
	if err != nil || !strings.Contains(string(encoded), "hvac_zone/") {
		t.Fatalf("nullable independent metric wire: %s %v", encoded, err)
	}
}
