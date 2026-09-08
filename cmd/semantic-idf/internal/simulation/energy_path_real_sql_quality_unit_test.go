package simulation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func epathSQLQualityUnitFixture(t *testing.T) (PurposeResultBundle, epathSQLModelCheck) {
	t.Helper()
	originals := map[int]epathRealSQLSource{
		1: {DictionaryIndex: 1, Name: "Zone Air System Sensible Cooling Energy", KeyValue: "Office", SourceUnit: "J", ReportingFrequency: "Monthly"},
		2: {DictionaryIndex: 2, Name: "Cooling:Electricity", IsMeter: true, SourceUnit: "J", ReportingFrequency: "Monthly"},
		3: {DictionaryIndex: 3, Name: "Electricity:Facility", IsMeter: true, SourceUnit: "J", ReportingFrequency: "Monthly"},
		4: {DictionaryIndex: 4, Name: "Zone Air System Sensible Heating Energy", KeyValue: "Office", SourceUnit: "J", ReportingFrequency: "Monthly"},
	}
	result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"},
		Nodes: []EnergyExplanationNode{
			{ID: "cool-load", Level: "load", ServiceKind: "cooling", Value: 100, Unit: "kWh", ScaleDomain: "thermal", Period: "annual", Basis: "reported_variable", SourceIDs: []string{"source-1"}},
			{ID: "cool-use", Level: "end_use", EndUse: "cooling", Value: 25, Unit: "kWh", ScaleDomain: "site", Period: "annual", Basis: "reported_meter", SourceIDs: []string{"source-2"}},
			{ID: "electricity", Level: "carrier", Carrier: "electricity", Value: 25, Unit: "kWh", ScaleDomain: "site", Period: "annual", Basis: "reported_meter", SourceIDs: []string{"source-3"}},
		},
		Links: []EnergyPathLink{
			{ID: "conversion", FromID: "cool-load", ToID: "cool-use", Relation: "load_to_end_use", ServiceKind: "cooling", FromValue: 100, ToValue: 25, FromUnit: "kWh", ToUnit: "kWh", Ratio: 4, RatioKind: "coefficient_of_performance", Basis: "service_path_allocation", Period: "annual", SourceIDs: []string{"source-1", "source-2"}},
			{ID: "carrier-branch", FromID: "cool-use", ToID: "electricity", Relation: "end_use_to_carrier", FromValue: 25, ToValue: 25, FromUnit: "kWh", ToUnit: "kWh", Basis: "reported_meter", Period: "annual", SourceIDs: []string{"source-2"}},
		},
		Quality: &EnergyPathQuality{Ratios: EnergyCompletenessLevel{Level: "ratio", Status: "complete", Found: 1, Total: 1}},
	}
	for _, id := range []int{1, 2, 3, 4} {
		source := originals[id]
		result.Sources = append(result.Sources, EnergyDataSource{ID: fmt.Sprintf("source-%d", id), SourceType: "sql_report_data", Name: source.Name, KeyValue: source.KeyValue, SourceUnit: source.SourceUnit, NormalizedUnit: "kWh", ReportingFrequency: source.ReportingFrequency, IsMeter: source.IsMeter})
	}
	// These are declared independent fixture observations, not graph-read values.
	var prerequisites epathSQLModelChecks
	for _, item := range []struct {
		level, category, service, domain, basis string
		value                                   float64
	}{
		{"load", "", "cooling", "thermal", "reported_variable", 100},
		{"end_use", "cooling", "", "site", "reported_meter", 25},
		{"carrier", "electricity", "", "site", "reported_meter", 25},
		{"load", "", "heating", "thermal", "reported_variable", 0},
		{"end_use", "heating", "", "site", "reported_meter", 0},
	} {
		target := epathSQLNodeTarget(item.level, item.category, item.service, item.domain)
		target.Basis, target.AllowPrunedZero = item.basis, true
		q := epathSQLQuantity{Value: item.value}
		if err := prerequisites.add("ratios", "building", "", "annual", item.level+item.category+item.service, "kWh", &q, target, "", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	pair := &epathSQLConversionProof{From: epathSQLQuantity{Value: 100}, To: epathSQLQuantity{Value: 25}}
	q := epathSQLQuantity{Value: 4}
	target := epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: "cooling", Basis: "service_path_allocation", FromUnit: "kWh", ToUnit: "kWh", RatioKind: "coefficient_of_performance", Aggregate: "sum"}
	if err := prerequisites.add("ratios", "building", "", "annual", "conversion", "ratio", &q, target, "", nil, nil); err != nil {
		t.Fatal(err)
	}
	prerequisites.Rows[len(prerequisites.Rows)-1].Conversion = pair
	proof := &epathSQLQualityProof{Field: "ratios", LoadStatus: "complete", Dependencies: prerequisites.Rows, Originals: originals, LoadSources: map[string][]int{"cooling": {1}, "heating": {4}}, SiteSources: map[string]map[string][]int{"cooling": {"electricity": {2}}}, SiteValues: map[string]map[string]epathSQLQuantity{"cooling": {"electricity": {Value: 25}}}}
	var checks epathSQLModelChecks
	zero := 0
	if err := checks.add("ratios", "building", "", "annual", "ratios", "count", nil, epathRealOracleTarget{Collection: "quality", Field: "ratios"}, "unavailable", &zero, &zero); err != nil {
		t.Fatal(err)
	}
	checks.Rows[0].Quality = proof
	return PurposeResultBundle{EnergyExplanation: result}, checks.Rows[0]
}

func TestEnergyPathRealSQLQualityRatioIndependenceAndFailures(t *testing.T) {
	bundle, check := epathSQLQualityUnitFixture(t)
	want, err := epathCheckSQLModelQuality(bundle, check)
	if err != nil || want.Status != "complete" || want.Found == nil || *want.Found != 1 || want.Total == nil || *want.Total != 1 || want.Value == nil || *want.Value != 1 {
		t.Fatalf("no-heating pair denominator: %#v %v", want, err)
	}
	for _, test := range []struct {
		name string
		edit func(*PurposeResultBundle, *epathSQLModelCheck)
	}{
		{"missing quality", func(b *PurposeResultBundle, _ *epathSQLModelCheck) { b.EnergyExplanation.Quality = nil }},
		{"wrong quality level", func(b *PurposeResultBundle, _ *epathSQLModelCheck) { b.EnergyExplanation.Quality.Ratios.Level = "load" }},
		{"invented second service", func(b *PurposeResultBundle, _ *epathSQLModelCheck) { b.EnergyExplanation.Quality.Ratios.Total = 2 }},
		{"false missing numerator", func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Quality.Ratios.Found = 0
			b.EnergyExplanation.Quality.Ratios.Status = "missing"
		}},
		{"positive pair pruned", func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Links = b.EnergyExplanation.Links[1:]
		}},
		{"missing conversion source", func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Links[0].SourceIDs = []string{"absent"}
		}},
		{"existing different source", func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Links[0].SourceIDs = []string{"source-1", "source-3"}
		}},
		{"facility source substituted on carrier branch", func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Links[1].SourceIDs = []string{"source-3"}
		}},
		{"wrong original source unit", func(b *PurposeResultBundle, _ *epathSQLModelCheck) { b.EnergyExplanation.Sources[1].SourceUnit = "W" }},
		{"wrong physical source key", func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Sources[0].KeyValue = "Other Zone"
		}},
		{"lost endpoint source", func(b *PurposeResultBundle, _ *epathSQLModelCheck) { b.EnergyExplanation.Nodes[0].SourceIDs = nil }},
		{"thermal-site contradiction", func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Nodes[1].ScaleDomain = "thermal"
		}},
		{"service contradiction", func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Nodes[1].ServiceKind = "heating"
		}},
		{"unproved conversion basis", func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Links[0].Basis = "direct_zone_energy"
		}},
		{"recursive quality proof", func(_ *PurposeResultBundle, c *epathSQLModelCheck) { c.Quality.Dependencies[0].Quality = c.Quality }},
		{"foreign context proof", func(_ *PurposeResultBundle, c *epathSQLModelCheck) { c.Quality.Dependencies[0].Item.Period = "M1" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, c := epathSQLQualityUnitFixture(t)
			test.edit(&b, &c)
			if _, err := epathCheckSQLModelQuality(b, c); err == nil {
				t.Fatal("candidate quality concealed invalid independent evidence")
			}
		})
	}
}

func TestEnergyPathRealSQLQualityRatioPresentationAndServiceDedup(t *testing.T) {
	t.Run("positive original SQL may have a zero displayed denominator", func(t *testing.T) {
		b, c := epathSQLQualityUnitFixture(t)
		// The original denominator center remains positive; only its independently
		// counted 3dp interval permits displayed zero. Quality must not claim COP.
		tiny := epathSQLQuantity{Value: .0001, Error: .001}
		for i := range c.Quality.Dependencies {
			d := &c.Quality.Dependencies[i]
			if d.Conversion != nil {
				d.Conversion.To = tiny
			} else if d.Item.Target.Level == "carrier" || d.Item.Target.Level == "end_use" && d.Item.Target.Category == "cooling" {
				d.Quantity = &tiny
			}
		}
		c.Quality.SiteValues["cooling"]["electricity"] = tiny
		b.EnergyExplanation.Nodes[1].Value, b.EnergyExplanation.Nodes[2].Value = 0, 0
		b.EnergyExplanation.Links[0].ToValue, b.EnergyExplanation.Links[0].Ratio, b.EnergyExplanation.Links[0].RatioKind = 0, 0, ""
		b.EnergyExplanation.Links[1].FromValue, b.EnergyExplanation.Links[1].ToValue = 0, 0
		b.EnergyExplanation.Quality.Ratios = EnergyCompletenessLevel{Level: "ratio", Status: "missing", Found: 0, Total: 1}
		if _, err := epathCheckSQLModelQuality(b, c); err != nil {
			t.Fatal(err)
		}
		b.EnergyExplanation.Quality.Ratios = EnergyCompletenessLevel{Level: "ratio", Status: "complete", Found: 1, Total: 1}
		if _, err := epathCheckSQLModelQuality(b, c); err == nil {
			t.Fatal("raw positive SQL was substituted for a usable displayed pair")
		}
	})
	t.Run("different bases do not double count one service pair", func(t *testing.T) {
		b, c := epathSQLQualityUnitFixture(t)
		first := &b.EnergyExplanation.Links[0]
		first.FromValue, first.ToValue = 50, 12.5
		second := *first
		second.ID, second.Basis = "second-basis", "zone_load_allocation"
		b.EnergyExplanation.Links = append(b.EnergyExplanation.Links, second)
		d := c.Quality.Dependencies[len(c.Quality.Dependencies)-1]
		d.Conversion = &epathSQLConversionProof{From: epathSQLQuantity{Value: 50}, To: epathSQLQuantity{Value: 12.5}}
		c.Quality.Dependencies[len(c.Quality.Dependencies)-1] = d
		d.Item.Target.Basis, d.Want.Key = "zone_load_allocation", "independent-second-basis"
		c.Quality.Dependencies = append(c.Quality.Dependencies, d)
		if _, err := epathCheckSQLModelQuality(b, c); err != nil {
			t.Fatal(err)
		}
		b.EnergyExplanation.Quality.Ratios.Found, b.EnergyExplanation.Quality.Ratios.Total = 2, 2
		if _, err := epathCheckSQLModelQuality(b, c); err == nil {
			t.Fatal("basis count replaced distinct Zone/service pair count")
		}
	})
	t.Run("unrequested load stage is not missing", func(t *testing.T) {
		b, c := epathSQLQualityUnitFixture(t)
		c.Quality.LoadStatus = "not_requested"
		b.EnergyExplanation.Quality.Ratios = EnergyCompletenessLevel{Level: "ratio", Status: "not_requested"}
		if _, err := epathCheckSQLModelQuality(b, c); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("load without matching site service still counts in denominator", func(t *testing.T) {
		b, c := epathSQLQualityUnitFixture(t)
		b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes, EnergyExplanationNode{ID: "heat-load", Level: "load", ServiceKind: "heating", Value: 50, Unit: "kWh", ScaleDomain: "thermal", Period: "annual", Basis: "reported_variable", SourceIDs: []string{"source-4"}})
		for i := range c.Quality.Dependencies {
			d := &c.Quality.Dependencies[i]
			if d.Item.Target.Level == "load" && d.Item.Target.Service == "heating" {
				d.Quantity = &epathSQLQuantity{Value: 50}
			}
		}
		b.EnergyExplanation.Quality.Ratios = EnergyCompletenessLevel{Level: "ratio", Status: "partial", Found: 1, Total: 2}
		if _, err := epathCheckSQLModelQuality(b, c); err != nil {
			t.Fatal(err)
		}
		b.EnergyExplanation.Quality.Ratios = EnergyCompletenessLevel{Level: "ratio", Status: "complete", Found: 1, Total: 1}
		if _, err := epathCheckSQLModelQuality(b, c); err == nil {
			t.Fatal("denominator counted only complete pairs")
		}
	})
	t.Run("known zero all services has no candidate pair", func(t *testing.T) {
		b, c := epathSQLQualityUnitFixture(t)
		b.EnergyExplanation.Nodes, b.EnergyExplanation.Links = nil, nil
		for i := range c.Quality.Dependencies {
			d := &c.Quality.Dependencies[i]
			d.Quantity = &epathSQLQuantity{}
			if d.Conversion != nil {
				d.Conversion = &epathSQLConversionProof{}
			}
		}
		c.Quality.SiteValues["cooling"]["electricity"] = epathSQLQuantity{}
		b.EnergyExplanation.Quality.Ratios = EnergyCompletenessLevel{Level: "ratio", Status: "not_applicable"}
		if _, err := epathCheckSQLModelQuality(b, c); err != nil {
			t.Fatal(err)
		}
		c.Quality.Dependencies[0].Quantity = nil
		if _, err := epathCheckSQLModelQuality(b, c); err == nil {
			t.Fatal("unknown load observation was treated as prunable zero")
		}
	})
}

func TestEnergyPathRealSQLQualityAllContextsAndAvailability(t *testing.T) {
	frames, model := epathSQLZoneServiceUnitFrames()
	observed := epathRealOracleEvidence{outputPlan: &PurposeRunPlan{}}
	for _, item := range []struct {
		stage, name  string
		meter, found bool
	}{
		{"drivers", "Surface Exchange", false, true}, {"drivers", "Unreported Driver", false, false},
		{"loads", "Delivered Load", false, true}, {"endUses", "Cooling:Electricity", true, true}, {"carriers", "Electricity:Facility", true, true},
	} {
		model.Availability = append(model.Availability, epathRealSQLAvailability{ID: item.name, Stage: item.stage, RequestedNames: []string{item.name}, Observations: []epathRealSQLAlternative{{Name: item.name, Unit: "J"}}, IsMeter: item.meter})
		observed.outputPlan.OutputObjects = append(observed.outputPlan.OutputObjects, PurposeOutputObject{VariableName: item.name, ReportingFrequency: "Monthly"})
		if item.found {
			observed.Sources = append(observed.Sources, epathRealSQLSource{Name: item.name, SourceUnit: "J", ReportingFrequency: "Monthly", IsMeter: item.meter, Rows: 12})
		}
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelQualityChecks(observed, frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 4*13*9 {
		t.Fatalf("incomplete Building/Zone/annual/month quality roster: %d", len(checks.Rows))
	}
	quality := func(zone bool) *EnergyPathQuality {
		q := &EnergyPathQuality{Drivers: EnergyCompletenessLevel{Level: "driver", Status: "partial", Found: 1, Total: 2}, Loads: EnergyCompletenessLevel{Level: "load", Status: "complete", Found: 1, Total: 1}, EndUses: EnergyCompletenessLevel{Level: "end_use", Status: "complete", Found: 1, Total: 1}, Carriers: EnergyCompletenessLevel{Level: "carrier", Status: "complete", Found: 1, Total: 1}}
		if zone {
			q.EndUses = EnergyCompletenessLevel{Level: "end_use", Status: "partial"}
			q.Carriers = EnergyCompletenessLevel{Level: "carrier", Status: "partial"}
		}
		return q
	}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Quality: quality(false)}}
	for month := 1; month <= 12; month++ {
		bundle.EnergyExplanation.Periods = append(bundle.EnergyExplanation.Periods, EnergyPeriod{ID: fmt.Sprintf("M%d", month), Quality: quality(false)})
	}
	for _, zone := range []string{"A", "B", "Plenum"} {
		z := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: zone}, Quality: quality(true)}
		for month := 1; month <= 12; month++ {
			z.Periods = append(z.Periods, EnergyPeriod{ID: fmt.Sprintf("M%d", month), Quality: quality(true)})
		}
		bundle.EnergyExplanation.ZoneResults = append(bundle.EnergyExplanation.ZoneResults, z)
	}
	seen := map[string]bool{}
	for _, c := range checks.Rows {
		if seen[c.Want.Key] {
			t.Fatal("duplicate annual availability seed")
		}
		seen[c.Want.Key] = true
		if c.Quality.Field == "ratios" || epathSQLQualityAccounting(c.Quality.Field) {
			continue
		}
		found, total, status := 1, 1, "complete"
		if c.Quality.Field == "drivers" {
			total, status = 2, "partial"
		}
		if c.Item.Scope == "zone" && (c.Quality.Field == "endUses" || c.Quality.Field == "carriers") {
			found, total, status = 0, 0, "partial"
			if c.Want.Value != nil {
				t.Fatal("unknown Zone denominator became known zero")
			}
		}
		if c.Want.Found == nil || c.Want.Total == nil || *c.Want.Found != found || *c.Want.Total != total || c.Want.Status != status {
			t.Fatalf("wrong run-level contract: %#v", c.Want)
		}
		if _, err := epathCheckSQLModelQuality(bundle, c); err != nil {
			t.Fatalf("run-level availability was confused with an empty selected-period graph: %s %v", c.Want.Key, err)
		}
	}
	if err := epathSQLModelQualityChecks(observed, frames, model, &checks); err == nil {
		t.Fatal("duplicate quality compilation silently overwrote checks")
	}
	observed.outputPlan = nil
	if err := epathSQLModelQualityChecks(observed, frames, model, &epathSQLModelChecks{}); err == nil {
		t.Fatal("availability accepted no executed plan")
	}
}

func epathSQLQualityAccountingUnitCheck(field, scope, zone, period string, prior []epathSQLModelCheck) epathSQLModelCheck {
	group := "completeness"
	if field == "zoneAllocatedPct" || field == "unassignedPct" {
		group = "zoneAllocation"
	}
	key := strings.Join([]string{group, scope, strings.ToLower(zone), period, field}, "|")
	return epathSQLModelCheck{
		Item:    epathRealOracleMetricRecipe{Group: group, Scope: scope, Zone: zone, Period: period, Unit: "%", Target: epathRealOracleTarget{Collection: "quality", Field: field}},
		Want:    epathRealOracleMetric{Key: key, Group: group, Scope: scope, Zone: zone, Period: period, Unit: "%", Status: "unavailable"},
		Quality: &epathSQLQualityProof{Field: field, RunLevel: EnergyCompletenessLevel{Status: "complete"}, Dependencies: epathSQLQualityAccountingDependencies(prior, field, scope, zone, period)},
	}
}

func TestEnergyPathRealSQLQualityThermalClosureAllContexts(t *testing.T) {
	frames, model, checks, b := epathSQLDriverLinksFixture(t)
	if err := epathSQLModelLoadDriverChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	q := func() *EnergyPathQuality {
		return &EnergyPathQuality{DriverToLoadClosedPct: 100, DriverToLoadStatus: "complete"}
	}
	b.EnergyExplanation.Quality = q()
	b.EnergyExplanation.ZoneResults[0].Quality = q()
	for i := range b.EnergyExplanation.Periods {
		b.EnergyExplanation.Periods[i].Quality = q()
		b.EnergyExplanation.ZoneResults[0].Periods[i].Quality = q()
	}
	for _, zone := range []string{"", "Office"} {
		scope := "building"
		if zone != "" {
			scope = "zone"
		}
		for _, period := range append([]string{"annual"}, func() []string {
			out := []string{}
			for m := 1; m <= 12; m++ {
				out = append(out, fmt.Sprintf("M%d", m))
			}
			return out
		}()...) {
			c := epathSQLQualityAccountingUnitCheck("driverToLoadClosedPct", scope, zone, period, checks.Rows)
			want, err := epathCheckSQLModelQuality(b, c)
			if err != nil || want.Value == nil || *want.Value != 100 || want.Status != "complete" {
				t.Fatalf("%s/%s independent driver closure: %#v %v", zone, period, want, err)
			}
		}
	}
	c := epathSQLQualityAccountingUnitCheck("driverToLoadClosedPct", "building", "", "annual", checks.Rows)
	b.EnergyExplanation.Quality.DriverToLoadClosedPct = 99
	if _, err := epathCheckSQLModelQuality(b, c); err == nil {
		t.Fatal("wrong thermal percentage passed exact proven graph")
	}
	b.EnergyExplanation.Quality.DriverToLoadClosedPct = 100
	b.EnergyExplanation.Links[0].ToID = "people"
	if _, err := epathCheckSQLModelQuality(b, c); err == nil {
		t.Fatal("quality hid a wrong thermal destination")
	}
	b.EnergyExplanation.Links[0].ToID = "cooling"
	b.EnergyExplanation.Nodes[0].SourceIDs = []string{"people-source"}
	b.EnergyExplanation.Links[0].SourceIDs = []string{"people-source", "cooling-source"}
	if _, err := epathCheckSQLModelQuality(b, c); err == nil {
		t.Fatal("unchanged total closure hid source ownership moved to the wrong driver category")
	}
}

func epathSQLQualitySiteClosureFixture(t *testing.T) (PurposeResultBundle, epathSQLModelCheck) {
	t.Helper()
	_, _, b, prior := epathSQLSiteFlowUnitFixture(t)
	// Facility observations are independent literals, not sums read from the
	// candidate links: gas168/electric240 vs explained120/288 respectively.
	for _, item := range []struct {
		carrier string
		value   float64
	}{{"natural_gas", 168}, {"electricity", 240}} {
		q := epathSQLQuantity{Value: item.value}
		target := epathSQLNodeTarget("carrier", item.carrier, "", "site")
		target.Basis = "reported_meter"
		if err := prior.add("carriers", "building", "", "annual", item.carrier, "kWh", &q, target, "", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	b.EnergyExplanation.Nodes[2].Value, b.EnergyExplanation.Nodes[3].Value = 168, 240
	b.EnergyExplanation.Quality = &EnergyPathQuality{EndUseToCarrierClosedPct: 76.471, EndUseToCarrierStatus: "overmapped"}
	return b, epathSQLQualityAccountingUnitCheck("endUseToCarrierClosedPct", "building", "", "annual", prior.Rows)
}

func TestEnergyPathRealSQLQualityClosureRejectsCancellationAndResidualPadding(t *testing.T) {
	b, c := epathSQLQualitySiteClosureFixture(t)
	if want, err := epathCheckSQLModelQuality(b, c); err != nil || want.Value == nil || *want.Value != 76.471 {
		t.Fatalf("endpoint abs errors48+48 must not cancel: %#v %v", want, err)
	}
	// The residual drawing may exist, but contributes neither numerator nor denominator.
	b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes, EnergyExplanationNode{ID: "residual", Level: "residual", Value: 48, ScaleDomain: "site", Unit: "kWh", Period: "annual"})
	b.EnergyExplanation.Links = append(b.EnergyExplanation.Links, EnergyPathLink{ID: "residual-drawing", FromID: "residual", ToID: "gas", Relation: "residual_to_carrier", FromValue: 48, ToValue: 48, FromUnit: "kWh", ToUnit: "kWh", Period: "annual"})
	if _, err := epathCheckSQLModelQuality(b, c); err != nil {
		t.Fatal(err)
	}
	for _, pct := range []float64{100, 88.235} {
		b.EnergyExplanation.Quality.EndUseToCarrierClosedPct = pct
		if _, err := epathCheckSQLModelQuality(b, c); err == nil {
			t.Fatalf("cancelling/padding closure %.3f passed", pct)
		}
	}
	for _, mutate := range []func(*PurposeResultBundle, *epathSQLModelCheck){
		func(b *PurposeResultBundle, _ *epathSQLModelCheck) { b.EnergyExplanation.Links[0].SourceIDs = nil },
		func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Quality.EndUseToCarrierStatus = "complete"
		},
		func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Nodes[2].Basis = "reported_end_use_subtotal"
		},
		func(_ *PurposeResultBundle, c *epathSQLModelCheck) { c.Quality.Dependencies = nil },
		func(_ *PurposeResultBundle, c *epathSQLModelCheck) { c.Quality.Dependencies[0].Quality = c.Quality },
	} {
		b, c := epathSQLQualitySiteClosureFixture(t)
		mutate(&b, &c)
		if _, err := epathCheckSQLModelQuality(b, c); err == nil {
			t.Fatal("invalid proof/status/source passed carrier closure")
		}
	}
}

func TestEnergyPathRealSQLQualityZoneCarrierUnknownDenominator(t *testing.T) {
	var prior epathSQLModelChecks
	target := epathSQLNodeTarget("carrier", "electricity", "", "site")
	target.Basis, target.AllowPrunedZero = "service_path_allocation", true
	q := epathSQLQuantity{Value: 25}
	if err := prior.add("carriers", "zone", "Office", "M1", "electricity", "kWh", &q, target, "", nil, nil); err != nil {
		t.Fatal(err)
	}
	b := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, ZoneResults: []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}, Periods: []EnergyPeriod{{ID: "M1", Nodes: []EnergyExplanationNode{{ID: "zone-subtotal", Level: "carrier", Carrier: "electricity", Value: 25, Basis: "service_path_allocation", Unit: "kWh", ScaleDomain: "site", Period: "M1", ZoneName: "Office"}}, Quality: &EnergyPathQuality{EndUseToCarrierStatus: "partial"}}}}}}}
	c := epathSQLQualityAccountingUnitCheck("endUseToCarrierClosedPct", "zone", "Office", "M1", prior.Rows)
	want, err := epathCheckSQLModelQuality(b, c)
	if err != nil || want.Value != nil || want.Status != "partial" {
		t.Fatalf("Zone subtotal became a facility denominator: %#v %v", want, err)
	}
	b.EnergyExplanation.ZoneResults[0].Periods[0].Quality.EndUseToCarrierStatus = "complete"
	if _, err := epathCheckSQLModelQuality(b, c); err == nil {
		t.Fatal("Zone inherited complete Building closure")
	}
	// Fully observed/prunable zero with no node is unavailable, not reported0%.
	zero := epathSQLQuantity{}
	c.Quality.Dependencies[0].Quantity = &zero
	b.EnergyExplanation.ZoneResults[0].Periods[0].Nodes = nil
	b.EnergyExplanation.ZoneResults[0].Periods[0].Quality.EndUseToCarrierStatus = "unavailable"
	if want, err := epathCheckSQLModelQuality(b, c); err != nil || want.Value != nil {
		t.Fatalf("absent known-zero subtotal: %#v %v", want, err)
	}
	c.Quality.Dependencies[0].Quantity = nil
	if _, err := epathCheckSQLModelQuality(b, c); err == nil {
		t.Fatal("unknown subtotal accepted as known-zero absence")
	}
}

func epathSQLQualityAllocationUnitFixture(t *testing.T) (PurposeResultBundle, epathSQLModelChecks) {
	t.Helper()
	b := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, ZoneResults: []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}}}}}
	var prior epathSQLModelChecks
	for month := 0; month <= 12; month++ {
		period, expected, direct, allocated, unassigned := "annual", 1200.0, 240.0, 940.0, 20.0
		if month > 0 {
			period, expected, direct, allocated, unassigned = fmt.Sprintf("M%d", month), 100, 20, 80, 0
			if month == 1 {
				allocated, unassigned = 60, 20
			}
		}
		id := "reconcile.zone_hvac_allocation.cooling." + period
		start := len(prior.Rows)
		for _, item := range []struct {
			field string
			value float64
		}{{"expectedValue", expected}, {"directValue", direct}, {"allocatedValue", allocated}, {"unassignedValue", unassigned}} {
			q := epathSQLQuantity{Value: item.value}
			if err := prior.add("zoneAllocation", "building", "", period, "cooling/"+item.field, "kWh", &q, epathRealOracleTarget{Collection: "reconciliation", ID: id, Level: "allocation", Field: item.field, Unit: "kWh"}, "", nil, nil); err != nil {
				t.Fatal(err)
			}
		}
		if err := prior.bindAllocationProof(start); err != nil {
			t.Fatal(err)
		}
		row := EnergyReconciliation{ID: id, Level: "allocation", Period: period, Unit: "kWh", ExpectedValue: expected, DirectValue: direct, AllocatedValue: allocated, UnassignedValue: unassigned}
		status, assignedPct, unassignedPct := "complete", 100.0, 0.0
		if month == 0 {
			status, assignedPct, unassignedPct = "partial", 98.333, 1.667
		} else if month == 1 {
			status, assignedPct, unassignedPct = "partial", 80, 20
		}
		quality := EnergyPathQuality{ZoneAllocatedPct: assignedPct, UnassignedPct: unassignedPct, ZoneAllocationStatus: status}
		zoneQuality := quality
		if month == 0 {
			b.EnergyExplanation.Reconciliation, b.EnergyExplanation.Quality = []EnergyReconciliation{row}, &quality
			b.EnergyExplanation.ZoneResults[0].Quality = &zoneQuality
		} else {
			b.EnergyExplanation.Periods = append(b.EnergyExplanation.Periods, EnergyPeriod{ID: period, Reconciliation: []EnergyReconciliation{row}, Quality: &quality})
			b.EnergyExplanation.ZoneResults[0].Periods = append(b.EnergyExplanation.ZoneResults[0].Periods, EnergyPeriod{ID: period, Quality: &zoneQuality})
		}
	}
	return b, prior
}

func TestEnergyPathRealSQLQualityAllocationAllContextsAndFailures(t *testing.T) {
	b, prior := epathSQLQualityAllocationUnitFixture(t)
	for _, zone := range []string{"", "Office"} {
		scope := "building"
		if zone != "" {
			scope = "zone"
		}
		for month := 0; month <= 12; month++ {
			period := "annual"
			if month > 0 {
				period = fmt.Sprintf("M%d", month)
			}
			for _, field := range []string{"zoneAllocatedPct", "unassignedPct"} {
				c := epathSQLQualityAccountingUnitCheck(field, scope, zone, period, prior.Rows)
				if want, err := epathCheckSQLModelQuality(b, c); err != nil || want.Value == nil {
					t.Fatalf("%s %s %s: %#v %v", zone, period, field, want, err)
				}
			}
		}
	}
	for _, mutate := range []func(*PurposeResultBundle, *epathSQLModelCheck){
		func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Quality.ZoneAllocatedPct = 78.333
		}, // allocated-only omits direct240.
		func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Quality.ZoneAllocatedPct = 100
		}, // annual hides January's unassigned20.
		func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Quality.ZoneAllocationStatus = "complete"
		},
		func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Reconciliation[0].ExpectedValue++
		},
		func(b *PurposeResultBundle, _ *epathSQLModelCheck) { b.EnergyExplanation.Reconciliation = nil },
		func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			row := b.EnergyExplanation.Reconciliation[0]
			row.ID += ".extra"
			b.EnergyExplanation.Reconciliation = append(b.EnergyExplanation.Reconciliation, row)
		},
		func(_ *PurposeResultBundle, c *epathSQLModelCheck) { c.Quality.Dependencies[0].Allocation.Direct = nil },
		func(_ *PurposeResultBundle, c *epathSQLModelCheck) { c.Quality.Dependencies[0].Item.Period = "M1" },
	} {
		b, prior := epathSQLQualityAllocationUnitFixture(t)
		c := epathSQLQualityAccountingUnitCheck("zoneAllocatedPct", "building", "", "annual", prior.Rows)
		mutate(&b, &c)
		if _, err := epathCheckSQLModelQuality(b, c); err == nil {
			t.Fatal("wrong allocation value/status/observation passed quality")
		}
	}
	b.EnergyExplanation.ZoneResults[0].Quality.ZoneAllocatedPct = 80
	c := epathSQLQualityAccountingUnitCheck("zoneAllocatedPct", "zone", "Office", "annual", prior.Rows)
	if _, err := epathCheckSQLModelQuality(b, c); err == nil {
		t.Fatal("Zone copied a different period's Building allocation")
	}
}

func TestEnergyPathRealSQLQualityAllocationRoundedOvermappedAndWholePrune(t *testing.T) {
	b, prior := epathSQLQualityAllocationUnitFixture(t)
	c := epathSQLQualityAccountingUnitCheck("zoneAllocatedPct", "building", "", "M2", prior.Rows)
	p := c.Quality.Dependencies[0].Allocation
	*p.Allocated = epathSQLQuantity{Value: 80, Error: .0011}.positive()
	b.EnergyExplanation.Periods[1].Reconciliation[0].AllocatedValue = 80.001
	b.EnergyExplanation.Periods[1].Quality.ZoneAllocatedPct = 100.001
	b.EnergyExplanation.Periods[1].Quality.ZoneAllocationStatus = "overmapped"
	want, err := epathCheckSQLModelQuality(b, c)
	if err != nil || want.Value == nil || *want.Value != 100.001 {
		t.Fatalf("bounded displayed overmapping must not be capped: %#v %v", want, err)
	}
	if err := epathValidateOracleMetricIdentity(want); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"unassignedPct", "driverToLoadClosedPct", "endUseToCarrierClosedPct"} {
		invalid := want
		invalid.Key = strings.TrimSuffix(want.Key, "zoneAllocatedPct") + field
		if err := epathValidateOracleMetricIdentity(invalid); err == nil {
			t.Fatalf("over100 leaked to %s", field)
		}
	}
	b.EnergyExplanation.Periods[1].Quality.ZoneAllocationStatus = "complete"
	if _, err := epathCheckSQLModelQuality(b, c); err == nil {
		t.Fatal("over100 complete accounting accepted")
	}
	// All four independently known intervals include zero: whole-row absence
	// is possible, but an unknown field does not authorize that presentation.
	b, tiny := epathSQLModelAllocationFixture()
	b.EnergyExplanation.Quality = &EnergyPathQuality{ZoneAllocationStatus: "unavailable"}
	c = epathSQLQualityAccountingUnitCheck("zoneAllocatedPct", "building", "", "annual", []epathSQLModelCheck{tiny})
	if want, err := epathCheckSQLModelQuality(b, c); err != nil || want.Value != nil {
		t.Fatalf("whole-row tiny presentation: %#v %v", want, err)
	}
	c.Quality.Dependencies[0].Allocation.Unassigned = nil
	if _, err := epathCheckSQLModelQuality(b, c); err == nil {
		t.Fatal("unknown field was accepted as pruned zero")
	}
}

func TestEnergyPathRealSQLQualityRawNullAndAbsentCounts(t *testing.T) {
	for _, field := range []string{"found", "total"} {
		for _, absent := range []bool{false, true} {
			b := epathOracleWireFixture()
			counts := epathOracleWireFixtureGraph(b, "zone-period")["quality"].(map[string]any)["ratios"].(map[string]any)
			if absent {
				delete(counts, field)
			} else {
				counts[field] = nil
			}
			data, err := json.Marshal(b)
			if err != nil {
				t.Fatal(err)
			}
			// Same order as the actual snapshot loader: raw preflight first,
			// exact decoder second. Typed Go zero must never hide missing wire.
			if err := epathValidateOracleCandidateWire(strings.NewReader(string(data))); err == nil {
				t.Fatalf("%s absent=%v reached typed zero decoding", field, absent)
			}
		}
	}
}
