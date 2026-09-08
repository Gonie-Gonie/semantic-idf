package simulation

import (
	"fmt"
	"strings"
	"testing"
)

func epathSQLSiteFlowUnitFixture(t *testing.T) (epathSQLFrames, epathRealSQLModel, PurposeResultBundle, epathSQLModelChecks) {
	t.Helper()
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{"office": {Name: "Office", Multiplier: 1}}, Loads: map[string]epathSQLQuantity{}, Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{}}
	model := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}, Services: []epathRealSQLService{{Service: "heating", SiteIDs: []string{"gas", "electric"}, ServedZones: []string{"Office"}}}}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	for month := 1; month <= 12; month++ {
		value := 50.0
		if month == 7 || month == 8 {
			value = 0
		}
		frames.Loads[epathSQLKey("office", "heating", month)] = epathSQLQuantity{Value: value}
	}
	for index, item := range []struct {
		id, endUse, carrier string
		monthly             float64
	}{{"gas", "heating", "natural_gas", 10}, {"electric", "heating", "electricity", 4}, {"lights", "lighting", "electricity", 20}} {
		id := index + 1
		source := epathRealSQLSource{DictionaryIndex: id, Name: item.id + ":Energy", KeyValue: "Whole Building", SourceUnit: "J", ReportingFrequency: "Monthly", IsMeter: true}
		for month := 1; month <= 12; month++ {
			source.Months = append(source.Months, epathRealSQLMonth{Month: month, Rows: 1, RawSum: epathOracleNumber(item.monthly * 3600000), EnergyKWh: epathOracleNumber(item.monthly)})
			frames.Site[item.id] = append(frames.Site[item.id], &epathSQLQuantity{Value: item.monthly})
		}
		frames.SiteSources[item.id], frames.SourceIdentities[id] = []int{id}, source
		model.Site = append(model.Site, epathRealSQLSite{ID: item.id, EndUse: item.endUse, Carrier: item.carrier})
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), Name: source.Name, KeyValue: source.KeyValue, SourceType: "sql_report_data", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", IsMeter: true})
	}
	bundle.EnergyExplanation.Nodes = []EnergyExplanationNode{
		{ID: "heating", Level: "end_use", EndUse: "heating", Value: 168, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", AggregationBasis: "model_total", Period: "annual", SourceIDs: []string{"sql-rdd-1", "sql-rdd-2"}},
		{ID: "lighting", Level: "end_use", EndUse: "lighting", Value: 240, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", AggregationBasis: "model_total", Period: "annual", SourceIDs: []string{"sql-rdd-3"}},
		{ID: "gas", Level: "carrier", Carrier: "natural_gas", Value: 120, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", AggregationBasis: "model_total", Period: "annual"},
		{ID: "electricity", Level: "carrier", Carrier: "electricity", Value: 288, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", AggregationBasis: "model_total", Period: "annual"},
	}
	for _, item := range []struct {
		id, from, to, relation, source string
		value                          float64
	}{
		{"gas.paired", "heating", "gas", "end_use_to_carrier", "sql-rdd-1", 100},
		{"gas.direct", "heating", "gas", "direct_end_use_to_carrier", "sql-rdd-1", 20},
		{"electric.paired", "heating", "electricity", "end_use_to_carrier", "sql-rdd-2", 40},
		{"electric.direct", "heating", "electricity", "direct_end_use_to_carrier", "sql-rdd-2", 8},
		{"lights", "lighting", "electricity", "direct_end_use_to_carrier", "sql-rdd-3", 240},
	} {
		bundle.EnergyExplanation.Links = append(bundle.EnergyExplanation.Links, EnergyPathLink{ID: item.id, FromID: item.from, ToID: item.to, Relation: item.relation, Basis: "reported_meter", FromValue: item.value, ToValue: item.value, FromUnit: "kWh", ToUnit: "kWh", Period: "annual", SourceIDs: []string{item.source}})
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelSiteFlowChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return frames, model, bundle, checks
}

func epathSQLSiteFlowUnitFailures(bundle PurposeResultBundle, checks epathSQLModelChecks) []error {
	var failures []error
	for _, check := range checks.Rows {
		if check.Item.Period == "annual" {
			if err := epathCheckSQLSiteFlow(bundle, check); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return failures
}

func TestEnergyPathRealSQLSiteFlowMonthlyFirstMixedCarriers(t *testing.T) {
	_, _, bundle, checks := epathSQLSiteFlowUnitFixture(t)
	if len(checks.Rows) != 156 {
		t.Fatalf("lost mandatory period/carrier/branch/endpoint obligation: %d", len(checks.Rows))
	}
	if failures := epathSQLSiteFlowUnitFailures(bundle, checks); len(failures) > 0 {
		t.Fatal(failures)
	}
	for _, check := range checks.Rows {
		if check.Item.Period == "annual" && strings.Contains(check.Want.Key, "heating/natural_gas/end_use_to_carrier/fromValue") && check.Quantity.Value != 100 {
			t.Fatal("annual must sum ten paired months, leaving July/August directly unassigned")
		}
	}
}

func TestEnergyPathRealSQLSiteFlowRejectsConservedButWrongBranches(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*PurposeResultBundle)
	}{
		{"move between temporal branches", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Links[0].FromValue++
			b.EnergyExplanation.Links[0].ToValue++
			b.EnergyExplanation.Links[1].FromValue--
			b.EnergyExplanation.Links[1].ToValue--
		}},
		{"swap carrier", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].ToID = "electricity" }},
		{"borrow lighting source", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].SourceIDs = []string{"sql-rdd-3"} }},
		{"borrow other carrier source", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].SourceIDs = []string{"sql-rdd-2"} }},
		{"missing active source", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].SourceIDs = nil }},
		{"missing node source", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[0].SourceIDs = []string{"sql-rdd-1"} }},
		{"wrong source metadata", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].KeyValue = "Elsewhere" }},
		{"missing branch", func(b *PurposeResultBundle) { b.EnergyExplanation.Links = b.EnergyExplanation.Links[1:] }},
		{"duplicate split", func(b *PurposeResultBundle) {
			copy := b.EnergyExplanation.Links[0]
			copy.ID = "duplicate"
			copy.FromValue /= 2
			copy.ToValue /= 2
			b.EnergyExplanation.Links[0].FromValue /= 2
			b.EnergyExplanation.Links[0].ToValue /= 2
			b.EnergyExplanation.Links = append(b.EnergyExplanation.Links, copy)
		}},
		{"unequal endpoints", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].ToValue += 1e-10 }},
		{"fabricated ratio", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].Ratio = 1 }},
		{"fabricated service", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].ServiceKind = "cooling" }},
		{"wrong basis", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].Basis = "residual" }},
		{"duplicate zero node", func(b *PurposeResultBundle) {
			node := b.EnergyExplanation.Nodes[0]
			node.ID = "extra"
			node.Value = 0
			b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes, node)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, bundle, checks := epathSQLSiteFlowUnitFixture(t)
			test.mutate(&bundle)
			if len(epathSQLSiteFlowUnitFailures(bundle, checks)) == 0 {
				t.Fatal("invalid physical branch accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLSiteFlowUnknownAndBoundedPresence(t *testing.T) {
	frames, model, _, _ := epathSQLSiteFlowUnitFixture(t)
	frames.Site["gas"][0] = nil
	if _, err := epathSQLSiteFlowMonth(frames, model, model.Site[0], 1); err == nil {
		t.Fatal("unknown meter became zero")
	}
	frames.Site["gas"][0] = &epathSQLQuantity{Value: 10}
	frames.Loads[epathSQLKey("office", "heating", 1)] = epathSQLQuantity{Value: .0001, Error: .001}
	split, err := epathSQLSiteFlowMonth(frames, model, model.Site[0], 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, relation := range []string{"end_use_to_carrier", "direct_end_use_to_carrier"} {
		low, high := split[relation].bounds()
		if low != 0 || high != 10 {
			t.Fatal("uncertain paired presentation must remain bounded by the same original meter")
		}
	}
	frames.Loads[epathSQLKey("office", "heating", 1)] = epathSQLQuantity{Value: .0011, Error: .001}
	split, err = epathSQLSiteFlowMonth(frames, model, model.Site[0], 1)
	if err != nil || split["end_use_to_carrier"].Value != 10 || split["direct_end_use_to_carrier"].Value != 0 {
		t.Fatal("independently positive load lost its paired branch")
	}
}

func TestEnergyPathRealSQLSiteFlowAnnualRetainsDiscreteMonthlyChoices(t *testing.T) {
	frames, model, bundle, _ := epathSQLSiteFlowUnitFixture(t)
	for month := 1; month <= 12; month++ {
		frames.Loads[epathSQLKey("office", "heating", month)] = epathSQLQuantity{}
	}
	frames.Loads[epathSQLKey("office", "heating", 1)] = epathSQLQuantity{Value: .0001, Error: .001}
	var checks epathSQLModelChecks
	if err := epathSQLModelSiteFlowChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		paired, direct float64
		valid          bool
	}{{0, 120, true}, {10, 110, true}, {4, 116, false}} {
		links := []EnergyPathLink{}
		for i, original := range bundle.EnergyExplanation.Links {
			link := original
			switch i {
			case 0:
				link.FromValue, link.ToValue = test.paired, test.paired
			case 1:
				link.FromValue, link.ToValue = test.direct, test.direct
			case 2:
				link.FromValue, link.ToValue = 4, 4
			case 3:
				link.FromValue, link.ToValue = 44, 44
			}
			if link.FromValue > 0 {
				links = append(links, link)
			}
		}
		candidate := bundle
		candidate.EnergyExplanation.Links = links
		if failures := epathSQLSiteFlowUnitFailures(candidate, checks); (len(failures) == 0) != test.valid {
			t.Fatalf("annual choices %+v: %v", test, failures)
		}
	}
}

func TestEnergyPathRealSQLSiteFlowMonthlyChoiceIsNotFractionalAllocation(t *testing.T) {
	frames, model, bundle, _ := epathSQLSiteFlowUnitFixture(t)
	frames.Loads[epathSQLKey("office", "heating", 1)] = epathSQLQuantity{Value: .0001, Error: .001}
	var checks epathSQLModelChecks
	if err := epathSQLModelSiteFlowChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	for i := range bundle.EnergyExplanation.Nodes {
		bundle.EnergyExplanation.Nodes[i].Period = "M1"
	}
	bundle.EnergyExplanation.Nodes[0].Value = 14
	bundle.EnergyExplanation.Nodes[1].Value = 20
	bundle.EnergyExplanation.Nodes[2].Value = 10
	bundle.EnergyExplanation.Nodes[3].Value = 24
	for i := range bundle.EnergyExplanation.Links {
		bundle.EnergyExplanation.Links[i].Period = "M1"
	}
	gasPaired, gasDirect := bundle.EnergyExplanation.Links[0], bundle.EnergyExplanation.Links[1]
	for _, pair := range []struct {
		paired, direct float64
		valid          bool
	}{{10, 0, true}, {0, 10, true}, {4, 6, false}} {
		gasPaired.FromValue, gasPaired.ToValue = pair.paired, pair.paired
		gasDirect.FromValue, gasDirect.ToValue = pair.direct, pair.direct
		links := []EnergyPathLink{}
		if pair.paired > 0 {
			links = append(links, gasPaired)
		}
		if pair.direct > 0 {
			links = append(links, gasDirect)
		}
		bundle.EnergyExplanation.Periods = []EnergyPeriod{{ID: "M1", Nodes: bundle.EnergyExplanation.Nodes, Links: links}}
		for _, check := range checks.Rows {
			if check.Item.Period == "M1" && check.SiteFlow.Carrier == "natural_gas" {
				if err := epathCheckSQLSiteFlow(bundle, check); (err == nil) != pair.valid {
					t.Fatalf("monthly choice %+v: %v", pair, err)
				}
			}
		}
	}
}

func TestEnergyPathRealSQLFanFlowExactPoolAndCarrier(t *testing.T) {
	observed, frames, pools, precision := epathFanPoolUnitInputs(t, epathFanPoolUnitSQL(t))
	frames.SourceIdentities = map[int]epathRealSQLSource{}
	frames.SiteSources = map[string][]int{"fans": {303}}
	frames.LoadSourceIDs = map[string][]int{}
	for _, source := range observed.Sources {
		frames.SourceIdentities[source.DictionaryIndex] = source
	}
	for index, zone := range []string{"A", "B", "C", "D", "Unserved"} {
		id := 410 + index
		source := epathRealSQLSource{DictionaryIndex: id, Name: "Zone Air System Sensible Cooling Energy", KeyValue: zone, SourceUnit: "J", ReportingFrequency: "Monthly"}
		for month := 1; month <= 12; month++ {
			key := epathSQLKey(zone, "cooling", month)
			value := frames.Loads[key].Value
			frames.LoadSourceIDs[key] = []int{id}
			source.Months = append(source.Months, epathRealSQLMonth{Month: month, Rows: 1, RawSum: epathOracleNumber(value * 3600000), EnergyKWh: epathOracleNumber(value)})
		}
		frames.SourceIdentities[id] = source
		observed.Sources = append(observed.Sources, source)
	}
	model := epathRealSQLModel{FanPools: pools, Precision: precision, Site: []epathRealSQLSite{{ID: "fans", EndUse: "fans", Carrier: "electricity"}}}
	var checks epathSQLModelChecks
	if err := epathSQLModelFanPoolChecks(observed, frames, pools, precision, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelFanFlowChecks(observed, frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 277 {
		t.Fatalf("lost required fan branch obligations: %d", len(checks.Rows))
	}
	makeBundle := func(sourceID string, value float64, carrierName string) PurposeResultBundle {
		sources := []EnergyDataSource{}
		for _, source := range observed.Sources {
			sources = append(sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", source.DictionaryIndex), Name: source.Name, KeyValue: source.KeyValue, SourceType: "sql_report_data", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: source.ReportingFrequency, IsMeter: source.IsMeter})
		}
		fan := EnergyExplanationNode{ID: "fan", Level: "end_use", EndUse: "fans", Value: value, Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", AggregationBasis: "model_total", ZoneName: "A", Period: "M1", SourceIDs: []string{sourceID, "sql-rdd-303", "sql-rdd-410"}}
		carrier := EnergyExplanationNode{ID: "carrier", Level: "carrier", Carrier: carrierName, Value: value, Unit: "kWh", ScaleDomain: "site", AggregationBasis: "model_total", ZoneName: "A", Period: "M1"}
		link := EnergyPathLink{ID: "fan-carrier", FromID: "fan", ToID: "carrier", Relation: "direct_end_use_to_carrier", Basis: "service_path_allocation", FromValue: value, ToValue: value, FromUnit: "kWh", ToUnit: "kWh", ZoneName: "A", Period: "M1", SourceIDs: []string{sourceID, "sql-rdd-303"}}
		return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Sources: sources, ZoneResults: []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "A"}, Periods: []EnergyPeriod{{ID: "M1", Nodes: []EnergyExplanationNode{fan, carrier}, Links: []EnergyPathLink{link}}}}}}}
	}
	for _, test := range []struct {
		source  string
		value   float64
		carrier string
		valid   bool
	}{{"sql-rdd-101", 25, "electricity", true}, {"sql-rdd-202", 25, "electricity", false}, {"sql-rdd-101", 50, "electricity", false}, {"sql-rdd-101", 25, "natural_gas", false}} {
		for _, check := range checks.Rows {
			if check.SiteFlow != nil && check.Item.Zone == "A" && check.Item.Period == "M1" {
				if err := epathCheckSQLSiteFlow(makeBundle(test.source, test.value, test.carrier), check); (err == nil) != test.valid {
					t.Fatalf("exact fan source/value/carrier %+v: %v", test, err)
				}
			}
		}
	}
	for _, test := range []struct {
		name   string
		mutate func(*PurposeResultBundle)
	}{
		{"missing branch pool", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Links[0].SourceIDs = []string{"sql-rdd-303"}
		}},
		{"borrow another Zone weight", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Nodes[0].SourceIDs[2] = "sql-rdd-411"
		}},
		{"weight is not fan consumption", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Links[0].SourceIDs = append(b.EnergyExplanation.ZoneResults[0].Periods[0].Links[0].SourceIDs, "sql-rdd-410")
		}},
		{"missing weight", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Nodes[0].SourceIDs = []string{"sql-rdd-101", "sql-rdd-303"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := makeBundle("sql-rdd-101", 25, "electricity")
			test.mutate(&bundle)
			for _, check := range checks.Rows {
				if check.SiteFlow != nil && check.Item.Zone == "A" && check.Item.Period == "M1" && epathCheckSQLSiteFlow(bundle, check) == nil {
					t.Fatal("invalid fan provenance accepted")
				}
			}
		})
	}
}
