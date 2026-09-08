package simulation

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func epathSQLDirectServiceUnitFrames() (epathSQLFrames, epathRealSQLModel) {
	frames, model := epathSQLZoneServiceUnitFrames()
	frames.DirectHVAC = map[string]epathSQLDirectHVACMonth{}
	frames.DirectHVACSourceIdentities = map[int]epathSQLDirectHVACSourceIdentity{}
	items := []struct {
		id, service, carrier, site, key, component, name string
		value                                            float64
	}{
		{"cooling.coil.electricity", "cooling", "electricity", "cool.e", "A DX", "Coil:Cooling:DX:SingleSpeed", "Cooling Coil Electricity Energy", 25},
		{"cooling.coil.crankcase_electricity", "cooling", "electricity", "cool.e", "A DX", "Coil:Cooling:DX:SingleSpeed", "Cooling Coil Crankcase Heater Electricity Energy", 5},
		{"heating.coil.natural_gas", "heating", "natural_gas", "heat.g", "A fuel", "Coil:Heating:Fuel", "Heating Coil NaturalGas Energy", 10},
		{"heating.coil.ancillary_natural_gas", "heating", "natural_gas", "heat.g", "A fuel", "Coil:Heating:Fuel", "Heating Coil Ancillary NaturalGas Energy", 2},
		{"heating.coil.electricity", "heating", "electricity", "heat.e", "A fuel", "Coil:Heating:Fuel", "Heating Coil Electricity Energy", 6},
	}
	for offset, item := range items {
		index := 40 + offset
		owner := epathRealSQLDirectHVACOwner{KeyValue: item.key, ZoneName: "A", EquipmentType: "ZoneHVAC:PackagedTerminalAirConditioner", EquipmentName: "A package", ComponentType: item.component}
		model.DirectHVACComponents = append(model.DirectHVACComponents, epathRealSQLDirectHVACComponent{ID: item.id, Service: item.service, Carrier: item.carrier, SiteID: item.site, Frequency: "Monthly", AggregationBasis: "model_total", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: item.name, Unit: "J"}}, Keys: []string{item.key}}, Owners: []epathRealSQLDirectHVACOwner{owner}})
		source := epathRealSQLSource{DictionaryIndex: index, Name: item.name, KeyValue: item.key, SourceUnit: "J", ReportingFrequency: "Monthly", Rows: 12}
		for month := 1; month <= 12; month++ {
			value := 0.0
			if month == 1 {
				value = item.value
			}
			raw := value * 3600000
			source.Months = append(source.Months, epathRealSQLMonth{Month: month, Rows: 1, RawSum: &raw, EnergyKWh: &value})
			key := epathSQLDirectHVACKey("A", item.service, item.carrier, month)
			q := frames.DirectHVAC[key]
			q.Present = true
			budget := 0.0
			if value > 0 {
				budget = .001
			}
			q.Quantity = q.Quantity.add(epathSQLQuantity{Value: value, Error: budget}.positive())
			q.SourceIDs = append(q.SourceIDs, index)
			frames.DirectHVAC[key] = q
		}
		frames.DirectHVACSourceIdentities[index] = epathSQLDirectHVACSourceIdentity{FamilyID: item.id, Service: item.service, Carrier: item.carrier, SiteID: item.site, AggregationBasis: "model_total", Owner: owner, Source: source, Precision: model.Precision}
	}
	model.Services[1].ReconciliationID = ""
	model.Services[1].CarrierReconciliationIDs = map[string]string{
		"electricity": "reconcile.zone_hvac_allocation.heating.electricity.annual",
		"natural_gas": "reconcile.zone_hvac_allocation.heating.natural_gas.annual",
	}
	return frames, model
}

func TestEnergyPathRealSQLDirectHVACServiceCarrierRemainderAndKnownZero(t *testing.T) {
	frames, model := epathSQLDirectServiceUnitFrames()
	before := frames.Loads[epathSQLKey("a", "cooling", 2)]
	cooling, err := epathSQLCompileDirectHVACService(frames, model, model.Services[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		month                            int
		direct, allocated, unassigned, b float64
	}{{1, 30, 70, 0, 70}, {2, 0, 200, 0, 200}} {
		row := cooling.Monthly[test.month-1]
		if row.Direct.Value != test.direct || row.Allocated.Value != test.allocated || row.Unassigned.Value != test.unassigned || row.Zones["b"]["electricity"].Allocated.Value != test.b || !row.Zones["a"]["electricity"].ObservedDirect || row.Zones["a"]["electricity"].Allocated.Value != 0 {
			t.Fatalf("carrier-qualified direct-first month%d = %#v", test.month, row)
		}
	}
	if !reflect.DeepEqual(before, frames.Loads[epathSQLKey("a", "cooling", 2)]) {
		t.Fatal("direct source altered thermal authority")
	}
	heating, err := epathSQLCompileDirectHVACService(frames, model, model.Services[1])
	if err != nil {
		t.Fatal(err)
	}
	first := heating.Monthly[0]
	if first.Site.Value != 60 || first.Direct.Value != 18 || first.Allocated.Value != 42 || first.Zones["b"]["electricity"].Allocated.Value != 14 || first.Zones["b"]["natural_gas"].Allocated.Value != 28 {
		t.Fatalf("two carriers were pooled/renormalized: %#v", first)
	}
	if heating.Monthly[1].Unassigned.Value != 20 || heating.Monthly[1].Allocated.Value != 0 || heating.Monthly[1].Zones["plenum"]["natural_gas"].Allocated.Value != 0 {
		t.Fatal("known zero served loads allocated to the positive passive plenum")
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	seen, heatingSeen := 0, 0
	for _, check := range checks.Rows {
		if check.Item.Period == "annual" && strings.Contains(check.Item.Key, "cooling/") && check.Allocation != nil {
			seen++
			if check.Allocation.Expected.Value != 300 || check.Allocation.Direct.Value != 30 || check.Allocation.Allocated.Value != 270 || check.Allocation.Unassigned.Value != 0 {
				t.Fatal("Building annual ledger did not sum the same completed monthly frames")
			}
		}
		if check.Item.Period == "annual" && strings.Contains(check.Item.Key, "heating/") && check.Allocation != nil {
			heatingSeen++
			expected, direct, allocated, unassigned := 25.0, 6.0, 14.0, 5.0
			if check.Item.Target.ID == "reconcile.zone_hvac_allocation.heating.natural_gas.annual" {
				expected, direct, allocated, unassigned = 55, 12, 28, 15
			}
			if check.Allocation.Expected.Value != expected || check.Allocation.Direct.Value != direct || check.Allocation.Allocated.Value != allocated || check.Allocation.Unassigned.Value != unassigned {
				t.Fatal("mixed-fuel Building ledger lost an independently computed carrier partition")
			}
		}
	}
	if seen != 4 {
		t.Fatalf("missing direct Building ledger: %d", seen)
	}
	if heatingSeen != 8 {
		t.Fatalf("missing per-carrier direct Building ledger: %d", heatingSeen)
	}
}

func TestEnergyPathRealSQLDirectHVACServiceRejectsUnobservedOrContradictory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*epathSQLFrames, *epathRealSQLModel)
	}{
		{"missing month", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			delete(f.DirectHVAC, epathSQLDirectHVACKey("a", "cooling", "electricity", 2))
		}},
		{"not observed zero", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			key := epathSQLDirectHVACKey("a", "cooling", "electricity", 2)
			q := f.DirectHVAC[key]
			q.Present = false
			f.DirectHVAC[key] = q
		}},
		{"unknown interval", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			key := epathSQLDirectHVACKey("a", "cooling", "electricity", 1)
			q := f.DirectHVAC[key]
			q.Quantity.Error = -1
			f.DirectHVAC[key] = q
		}},
		{"foreign component owner", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			q := f.DirectHVACSourceIdentities[40]
			q.Owner.ZoneName = "B"
			f.DirectHVACSourceIdentities[40] = q
		}},
		{"wrong carrier", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			q := f.DirectHVACSourceIdentities[40]
			q.Carrier = "natural_gas"
			f.DirectHVACSourceIdentities[40] = q
		}},
		{"duplicate original", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			key := epathSQLDirectHVACKey("a", "cooling", "electricity", 1)
			q := f.DirectHVAC[key]
			q.SourceIDs = append(q.SourceIDs, 40)
			f.DirectHVAC[key] = q
		}},
		{"missing source proof", func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.DirectHVACSourceIdentities, 40) }},
		{"changed source quantity", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			key := epathSQLDirectHVACKey("a", "cooling", "electricity", 1)
			q := f.DirectHVAC[key]
			q.Quantity = epathSQLQuantity{Value: 31, Error: .002}
			f.DirectHVAC[key] = q
		}},
		{"missing main constituent", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			key := epathSQLDirectHVACKey("a", "cooling", "electricity", 1)
			q := f.DirectHVAC[key]
			q.SourceIDs = []int{41}
			q.Quantity = epathSQLQuantity{Value: 5, Error: .001}
			f.DirectHVAC[key] = q
		}},
		{"unowned served scope", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Services[0].ServedZones = []string{"B"} }},
		{"overmapped direct", func(f *epathSQLFrames, _ *epathRealSQLModel) { f.Site["cool.e"][0] = &epathSQLQuantity{Value: 20} }},
		{"missing load", func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.Loads, epathSQLKey("b", "cooling", 1)) }},
		{"undeclared direct", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.DirectHVACComponents = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, m := epathSQLDirectServiceUnitFrames()
			tc.change(&f, &m)
			if _, err := epathSQLCompileDirectHVACService(f, m, m.Services[0]); err == nil {
				t.Fatal("invalid direct-first evidence accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLDirectHVACServiceZeroLoadRelationIsSeparate(t *testing.T) {
	frames, model := epathSQLDirectServiceUnitFrames()
	frames.Loads[epathSQLKey("a", "cooling", 1)] = epathSQLQuantity{}
	// The same measured coil has a small second-month observation while its
	// first-month site consumption occurs with exactly zero delivered load.
	identity := frames.DirectHVACSourceIdentities[41]
	identity.Source.Months[1].RawSum = epathOracleNumber(3600000)
	identity.Source.Months[1].EnergyKWh = epathOracleNumber(1)
	frames.DirectHVACSourceIdentities[41] = identity
	key := epathSQLDirectHVACKey("a", "cooling", "electricity", 2)
	observation := frames.DirectHVAC[key]
	observation.Quantity = epathSQLQuantity{Value: 1, Error: .001}
	frames.DirectHVAC[key] = observation
	var checks epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	var first epathSQLModelCheck
	annualFound := false
	for _, check := range checks.Rows {
		if check.ZoneService == nil || check.Item.Zone != "A" || check.ZoneService.Service != "cooling" {
			continue
		}
		if check.Item.Period == "M1" {
			first = check
		}
		if check.Item.Period == "annual" {
			annualFound = true
			totals := map[string]float64{}
			for _, branch := range check.ZoneService.Branches {
				totals[branch.Relation] += branch.Quantity.Value
			}
			if totals["direct_end_use_to_carrier"] != 30 || totals["end_use_to_carrier"] != 1 {
				t.Fatalf("annual relation-specific quantities were merged: %v", totals)
			}
		}
	}
	if !annualFound || first.ZoneService == nil {
		t.Fatal("missing zero-load direct obligations")
	}
	bundle := epathSQLDirectServiceUnitBundle()
	row := &bundle.EnergyExplanation.ZoneResults[0].Periods[0]
	for i := range row.Nodes {
		if row.Nodes[i].ID == "load.cooling" {
			row.Nodes[i].Value = 0
		}
	}
	filtered := []EnergyPathLink{}
	for _, link := range row.Links {
		if link.ID == "conversion.cooling" {
			continue
		}
		if link.ID == "cooling.electricity" {
			link.Relation = "direct_end_use_to_carrier"
		}
		filtered = append(filtered, link)
	}
	row.Links = filtered
	// Source41's immutable monthly observation changed only in this hand fixture.
	for i := range bundle.EnergyExplanation.Sources {
		if bundle.EnergyExplanation.Sources[i].ID == "sql-rdd-41" {
			bundle.EnergyExplanation.Sources[i] = epathSQLDirectHVACSourceUnitCandidate(identity)
		}
	}
	if err := epathCheckSQLZoneServiceEndpoints(bundle, first); err != nil {
		t.Fatalf("positive crankcase/main consumption with no load was rejected: %v", err)
	}
	for i := range row.Links {
		if row.Links[i].ID == "cooling.electricity" {
			row.Links[i].Relation = "end_use_to_carrier"
		}
	}
	if err := epathCheckSQLZoneServiceEndpoints(bundle, first); err == nil {
		t.Fatal("same direct quantity under the wrong causal relation passed")
	}
}

// These candidate branches use independent hand values, never compiler output.
func epathSQLDirectServiceUnitBundle() PurposeResultBundle {
	frames, _ := epathSQLDirectServiceUnitFrames()
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	for id, source := range frames.SourceIdentities {
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), SourceType: "sql_report_data", Name: source.Name, KeyValue: source.KeyValue, SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", IsMeter: source.IsMeter})
	}
	for _, identity := range frames.DirectHVACSourceIdentities {
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, epathSQLDirectHVACSourceUnitCandidate(identity))
	}
	for _, zone := range []string{"A", "B"} {
		basis, c, h, e, g, cl, hl := "direct_zone_energy", 30.0, 18.0, 36.0, 12.0, 10.0, 30.0
		if zone == "B" {
			basis, c, h, e, g, cl, hl = "service_path_allocation", 70, 42, 84, 28, 90, 10
		}
		row := EnergyPeriod{ID: "M1", Kind: "monthly"}
		for _, carrier := range []string{"electricity", "natural_gas"} {
			value := e
			if carrier == "natural_gas" {
				value = g
			}
			row.Nodes = append(row.Nodes, EnergyExplanationNode{ID: carrier, Level: "carrier", Carrier: carrier, Value: value, Unit: "kWh", ScaleDomain: "site", ZoneName: zone, Period: "M1"})
		}
		for _, service := range []string{"cooling", "heating"} {
			value, load := c, cl
			if service == "heating" {
				value, load = h, hl
			}
			loadID := fmt.Sprintf("sql-rdd-%d", epathSQLZoneServiceUnitLoadID(zone, service))
			trace := []string{"sql-rdd-40", "sql-rdd-41"}
			if service == "heating" {
				trace = []string{"sql-rdd-42", "sql-rdd-43", "sql-rdd-44"}
			}
			if zone == "B" {
				if service == "cooling" {
					trace = append(trace, "sql-rdd-1")
				} else {
					trace = append(trace, "sql-rdd-2", "sql-rdd-3")
				}
				trace = append(trace, loadID)
			}
			row.Nodes = append(row.Nodes, EnergyExplanationNode{ID: "load." + service, Level: "load", ServiceKind: service, Value: load, Unit: "kWh", ScaleDomain: "thermal", ZoneName: zone, Period: "M1", SourceIDs: []string{loadID}}, EnergyExplanationNode{ID: "use." + service, Level: "end_use", EndUse: service, Value: value, AllocatedValue: value, Unit: "kWh", ScaleDomain: "site", ZoneName: zone, Period: "M1", Basis: basis, SourceIDs: trace})
			conversionTrace := append(append([]string(nil), trace...), loadID)
			if zone == "B" {
				conversionTrace = trace
			}
			row.Links = append(row.Links, EnergyPathLink{ID: "conversion." + service, FromID: "load." + service, ToID: "use." + service, Relation: "load_to_end_use", Basis: basis, ServiceKind: service, ZoneName: zone, Period: "M1", FromValue: load, ToValue: value, FromUnit: "kWh", ToUnit: "kWh", Ratio: load / value, RatioKind: "load_to_site_energy", SourceIDs: conversionTrace})
			for _, carrier := range []string{"electricity", "natural_gas"} {
				if service == "cooling" && carrier == "natural_gas" {
					continue
				}
				amount := 30.0
				ids := []string{"sql-rdd-40", "sql-rdd-41"}
				if service == "heating" {
					amount = 6
					ids = []string{"sql-rdd-44"}
					if carrier == "natural_gas" {
						amount = 12
						ids = []string{"sql-rdd-42", "sql-rdd-43"}
					}
				}
				if zone == "B" {
					if service == "cooling" {
						amount = 70
						ids = append(ids, "sql-rdd-1")
					} else if carrier == "electricity" {
						amount = 14
						ids = append(ids, "sql-rdd-2")
					} else {
						amount = 28
						ids = append(ids, "sql-rdd-3")
					}
				}
				row.Links = append(row.Links, EnergyPathLink{ID: service + "." + carrier, FromID: "use." + service, ToID: carrier, Relation: "end_use_to_carrier", Basis: basis, ZoneName: zone, Period: "M1", FromValue: amount, ToValue: amount, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: ids})
			}
		}
		bundle.EnergyExplanation.ZoneResults = append(bundle.EnergyExplanation.ZoneResults, EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: zone}, Periods: []EnergyPeriod{row}})
	}
	return bundle
}

func TestEnergyPathRealSQLDirectHVACServiceStrictBranchSources(t *testing.T) {
	frames, model := epathSQLDirectServiceUnitFrames()
	var checks epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	selected := []epathSQLModelCheck{}
	for _, check := range checks.Rows {
		if check.ZoneService != nil && check.Item.Period == "M1" && check.Item.Zone != "Plenum" {
			selected = append(selected, check)
		}
	}
	if len(selected) != 4 {
		t.Fatalf("missing exact Zone proofs: %d", len(selected))
	}
	bundle := epathSQLDirectServiceUnitBundle()
	for _, check := range selected {
		if err := epathCheckSQLZoneServiceEndpoints(bundle, check); err != nil {
			t.Fatalf("hand direct/remainder source proof: %v", err)
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*PurposeResultBundle)
	}{
		{"direct branch broad source", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Links[1].SourceIDs = append(b.EnergyExplanation.ZoneResults[0].Periods[0].Links[1].SourceIDs, "sql-rdd-1")
		}},
		{"allocated branch thermal source", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[1].Periods[0].Links[1].SourceIDs = append(b.EnergyExplanation.ZoneResults[1].Periods[0].Links[1].SourceIDs, "sql-rdd-12")
		}},
		{"wrong carrier", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Links[1].ToID = "natural_gas"
		}},
		{"wrong relation", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Links[1].Relation = "direct_end_use_to_carrier"
		}},
		{"wrong basis", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Links[1].Basis = "service_path_allocation"
		}},
		{"duplicate zero branch", func(b *PurposeResultBundle) {
			row := &b.EnergyExplanation.ZoneResults[0].Periods[0]
			link := row.Links[1]
			link.ID = "extra"
			link.FromValue, link.ToValue = 0, 0
			row.Links = append(row.Links, link)
		}},
		{"wrong signed branch", func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Periods[0].Links[1].ToValue = -30 }},
		{"missing original", func(b *PurposeResultBundle) {
			for i, s := range b.EnergyExplanation.Sources {
				if s.ID == "sql-rdd-40" {
					b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources[:i], b.EnergyExplanation.Sources[i+1:]...)
					break
				}
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := epathSQLDirectServiceUnitBundle()
			tc.change(&b)
			failed := false
			for _, check := range selected {
				if epathCheckSQLZoneServiceEndpoints(b, check) != nil {
					failed = true
				}
			}
			if !failed {
				t.Fatal("wrong direct-first carrier proof passed")
			}
		})
	}
}
