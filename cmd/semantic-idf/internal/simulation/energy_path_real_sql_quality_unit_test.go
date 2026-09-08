package simulation

import (
	"encoding/json"
	"fmt"
	"reflect"
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

func epathSQLQualityTabularUnitFixture(t *testing.T) (PurposeResultBundle, epathSQLModelCheck) {
	t.Helper()
	_, observation, original, source := epathSQLOriginalUnitFixture(t, "25.00")
	bundle, check := epathSQLQualityUnitFixture(t)
	key, err := epathSQLOriginalKey(original)
	if err != nil {
		t.Fatal(err)
	}
	// The load is the literal 100 kWh RDD observation above; purchased energy
	// is the independently read original 25.00 kWh annual table cell.
	source.ID = "source-2"
	bundle.EnergyExplanation.Sources[1] = source
	bundle.EnergyExplanation.Nodes[2].Carrier = "district_cooling"
	bundle.EnergyExplanation.Nodes[2].SourceIDs = []string{source.ID}
	bundle.EnergyExplanation.Links[0].RatioKind = "load_to_site_energy"
	for i := range check.Quality.Dependencies {
		dependency := &check.Quality.Dependencies[i]
		if dependency.Item.Target.Level == "carrier" {
			dependency.Item.Target.Category = "district_cooling"
		}
		if dependency.Item.Target.Level == "carrier" || dependency.Item.Target.Level == "end_use" && dependency.Item.Target.Category == "cooling" {
			q := observation.Quantity
			dependency.Quantity = &q
		}
		if dependency.Conversion != nil {
			dependency.Conversion.To = observation.Quantity
			dependency.Item.Target.RatioKind = "load_to_site_energy"
		}
	}
	delete(check.Quality.Originals, 2)
	check.Quality.SiteSources = nil
	check.Quality.SiteOriginals = map[string]map[string]map[string]epathSQLOriginalSource{"cooling": {"district_cooling": {key: original}}}
	check.Quality.SiteValues = map[string]map[string]epathSQLQuantity{"cooling": {"district_cooling": observation.Quantity}}
	return bundle, check
}

func TestEnergyPathRealSQLQualityAnnualTabularOriginalPair(t *testing.T) {
	bundle, check := epathSQLQualityTabularUnitFixture(t)
	want, err := epathCheckSQLModelQuality(bundle, check)
	if err != nil || want.Status != "complete" || want.Found == nil || *want.Found != 1 || want.Total == nil || *want.Total != 1 {
		t.Fatalf("independent annual mixed RDD/Tabular pair: %#v %v", want, err)
	}
	for name, edit := range map[string]func(*PurposeResultBundle, *epathSQLModelCheck){
		"unselected latent load source": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, EnergyDataSource{ID: "unselected-latent", SourceType: "sql_report_data", Name: "Zone Ideal Loads Supply Air Latent Cooling Energy", KeyValue: "Ideal Unit", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"})
			b.EnergyExplanation.Nodes[0].SourceIDs = append(b.EnergyExplanation.Nodes[0].SourceIDs, "unselected-latent")
		},
		"facility is not consumption cell": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Sources[1].RowName = "Total End Uses"
		},
		"site source absent on consumption endpoint": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Nodes[1].SourceIDs = []string{"source-1"}
		},
		"site source absent on conversion": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Links[0].SourceIDs = []string{"source-1"}
		},
		"changed observed annual quantity": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Nodes[1].Value, b.EnergyExplanation.Nodes[2].Value = 26, 26
			b.EnergyExplanation.Links[0].ToValue, b.EnergyExplanation.Links[0].Ratio = 26, 100.0/26
			b.EnergyExplanation.Links[1].FromValue, b.EnergyExplanation.Links[1].ToValue = 26, 26
		},
		"unknown annual site cannot become zero": func(_ *PurposeResultBundle, c *epathSQLModelCheck) {
			delete(c.Quality.SiteValues["cooling"], "district_cooling")
		},
	} {
		t.Run(name, func(t *testing.T) {
			b, c := epathSQLQualityTabularUnitFixture(t)
			edit(&b, &c)
			if _, err := epathCheckSQLModelQuality(b, c); err == nil {
				t.Fatal("annual pair accepted missing/wrong independently owned evidence")
			}
		})
	}
	for month := 1; month <= 12; month++ {
		period := fmt.Sprintf("M%d", month)
		// Deliberately retain the same literal pair quantities in this mutant:
		// matching arithmetic must not legitimize an annual cell in any month.
		b, c := epathSQLQualityTabularUnitFixture(t)
		c.Item.Period = period
		for i := range c.Quality.Dependencies {
			c.Quality.Dependencies[i].Item.Period = period
		}
		for i := range b.EnergyExplanation.Nodes {
			b.EnergyExplanation.Nodes[i].Period = period
		}
		for i := range b.EnergyExplanation.Links {
			b.EnergyExplanation.Links[i].Period = period
		}
		b.EnergyExplanation.Periods = []EnergyPeriod{{ID: period, Nodes: b.EnergyExplanation.Nodes, Links: b.EnergyExplanation.Links, Quality: b.EnergyExplanation.Quality}}
		if _, err := epathCheckSQLModelQuality(b, c); err == nil {
			t.Fatalf("annual Tabular pair was fabricated in %s", period)
		}
	}
}

func TestEnergyPathRealSQLQualityNonAdditiveDetailNeverProvesLoadQuantity(t *testing.T) {
	fixture := func() (PurposeResultBundle, epathSQLModelCheck) {
		b, c := epathSQLQualityUnitFixture(t)
		detail, source := epathSQLOriginalLoadDetailUnitFixture()
		original, err := epathSQLOriginalLoadDetail(detail)
		if err != nil {
			t.Fatal(err)
		}
		c.Quality.LoadDetails = map[string]map[string]epathSQLOriginalSource{"cooling": {source.ID: original}}
		b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, source)
		b.EnergyExplanation.Nodes[0].SourceIDs = append(b.EnergyExplanation.Nodes[0].SourceIDs, source.ID)
		b.EnergyExplanation.Links[0].SourceIDs = append(b.EnergyExplanation.Links[0].SourceIDs, source.ID)
		return b, c
	}
	b, c := fixture()
	if _, err := epathCheckSQLModelQuality(b, c); err != nil {
		t.Fatalf("exact context changed otherwise proven pair: %v", err)
	}
	for name, edit := range map[string]func(*PurposeResultBundle, *epathSQLModelCheck){
		"context replaces load endpoint": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Nodes[0].SourceIDs = []string{"sql-rdd-7"}
		},
		"context replaces load link": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Links[0].SourceIDs = []string{"source-2", "sql-rdd-7"}
		},
		"context added to numeric load": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Nodes[0].Value = 136
			b.EnergyExplanation.Links[0].FromValue = 136
			b.EnergyExplanation.Links[0].Ratio = 136.0 / 25
		},
		"context replaces carrier proof": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Links[1].SourceIDs = []string{"sql-rdd-7"}
		},
		"context disguised as ordinary source": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Sources[len(b.EnergyExplanation.Sources)-1].DriverRole = ""
		},
		"context disguised as derived wrapper": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Sources[len(b.EnergyExplanation.Sources)-1].InputSourceIDs = []string{"source-1"}
		},
		"context proof drops typed role": func(_ *PurposeResultBundle, c *epathSQLModelCheck) {
			p := c.Quality.LoadDetails["cooling"]["sql-rdd-7"]
			p.LoadDetail = nil
			c.Quality.LoadDetails["cooling"]["sql-rdd-7"] = p
		},
	} {
		t.Run(name, func(t *testing.T) {
			b, c := fixture()
			edit(&b, &c)
			if _, err := epathCheckSQLModelQuality(b, c); err == nil {
				t.Fatal("non-additive context concealed missing numeric ownership or changed quantity")
			}
		})
	}
}

func TestEnergyPathRealSQLQualityAnnualTabularCompilerTemporalBoundary(t *testing.T) {
	site, observation, original, _ := epathSQLOriginalUnitFixture(t, "25.00")
	frames, model := epathSQLZoneServiceUnitFrames()
	oldID := model.Site[0].ID
	model.Site[0] = site
	delete(frames.Site, oldID)
	delete(frames.SiteSources, oldID)
	frames.SiteAnnual = map[string]epathSQLTabularObservation{site.ID: observation}
	detail, _ := epathSQLOriginalLoadDetailUnitFixture()
	detail.ZoneName, detail.OwnerName, detail.Source.KeyValue = "A", "Ideal A", "Ideal A"
	frames.LoadDetailIdentities = map[int]epathSQLLoadDetailIdentity{7: detail}
	frames.LoadDetailSourceIDs = map[string][]int{}
	frames.SourceIdentities[7] = detail.Source
	for month := 1; month <= 12; month++ {
		frames.LoadDetailSourceIDs[epathSQLKey("a", "cooling", month)] = []int{7}
	}
	observed := epathRealOracleEvidence{outputPlan: &PurposeRunPlan{}}
	for _, stage := range []string{"drivers", "loads", "endUses", "carriers"} {
		name, meter := "original requested "+stage, stage == "endUses" || stage == "carriers"
		model.Availability = append(model.Availability, epathRealSQLAvailability{ID: stage, Stage: stage, RequestedNames: []string{name}, Observations: []epathRealSQLAlternative{{Name: name, Unit: "J"}}, IsMeter: meter})
		observed.outputPlan.OutputObjects = append(observed.outputPlan.OutputObjects, PurposeOutputObject{VariableName: name, ReportingFrequency: "Monthly"})
		observed.Sources = append(observed.Sources, epathRealSQLSource{Name: name, SourceUnit: "J", ReportingFrequency: "Monthly", IsMeter: meter, Rows: 12})
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelQualityChecks(observed, frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	key, _ := epathSQLOriginalKey(original)
	annual, monthly, availability := 0, 0, 0
	for _, check := range checks.Rows {
		proof := check.Quality
		if proof.Field == "ratios" {
			wantDetail := check.Item.Scope == "building" || check.Item.Zone == "A"
			_, hasDetail := proof.LoadDetails["cooling"]["sql-rdd-7"]
			if hasDetail != wantDetail || len(proof.LoadDetails["heating"]) != 0 {
				t.Fatal("non-additive detail escaped exact scope/service/period membership")
			}
			for _, id := range proof.LoadSources["cooling"] {
				if id == 7 {
					t.Fatal("non-additive detail became delivered-load numeric authority")
				}
			}
			originals, err := epathSQLQualitySiteOriginals(proof, "cooling", "district_cooling")
			if err != nil {
				t.Fatal(err)
			}
			q, known := proof.SiteValues["cooling"]["district_cooling"]
			if check.Item.Period == "annual" {
				annual++
				if len(originals) != 1 || originals[key].Tabular == nil {
					t.Fatal("annual original source identity lost")
				}
				if check.Item.Scope == "building" && (!known || !reflect.DeepEqual(q, observation.Quantity)) {
					t.Fatal("original annual value/precision was not retained")
				}
				if check.Item.Scope == "zone" && known {
					t.Fatal("annual Building pool was invented as a measured Zone amount without a Zone service proof")
				}
			} else {
				monthly++
				if len(originals) != 0 || known {
					t.Fatal("annual site became monthly source/zero observation")
				}
			}
		} else if proof.Field == "drivers" || proof.Field == "loads" || proof.Field == "endUses" || proof.Field == "carriers" {
			availability++
			if proof.RunLevel.Found != 1 || proof.RunLevel.Total != 1 {
				t.Fatal("annual table cell changed requested Monthly availability counts")
			}
		}
	}
	if annual != 4 || monthly != 48 || availability != 4*13*4 {
		t.Fatalf("incomplete annual/month/Zone proof roster: %d/%d/%d", annual, monthly, availability)
	}
	wrongOwner := detail
	wrongOwner.ZoneName = "B"
	frames.LoadDetailIdentities[7] = wrongOwner
	if err := epathSQLModelQualityChecks(observed, frames, model, &epathSQLModelChecks{}); err == nil {
		t.Fatal("misbound non-additive equipment owner entered ratio proof")
	}
	frames.LoadDetailIdentities[7] = detail
	// Annual access to a true monthly meter still requires all twelve values.
	frames.Site["heat.e"][11] = nil
	if err := epathSQLModelQualityChecks(observed, frames, model, &epathSQLModelChecks{}); err == nil {
		t.Fatal("Tabular support relaxed the strict twelve-month RDD sum")
	}
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

func epathSQLQualityMixedTemporalClosureFixture(t *testing.T) (PurposeResultBundle, epathSQLFrames, epathRealSQLModel, epathSQLModelChecks) {
	t.Helper()
	frames, model, bundle := epathSQLAnnualSiteUnitInputs(t)
	frames.SiteSources, frames.SourceIdentities = map[string][]int{}, map[int]epathRealSQLSource{}
	for index, item := range []struct {
		id, name string
		facility bool
	}{{"light.e", "InteriorLights:Electricity", false}, {"facility.e", "Electricity:Facility", true}} {
		id := 90 + index
		source := epathRealSQLSource{DictionaryIndex: id, Name: item.name, IsMeter: true, SourceUnit: "J", ReportingFrequency: "Monthly", Rows: 12}
		for month := 1; month <= 12; month++ {
			source.Months = append(source.Months, epathRealSQLMonth{Month: month, Rows: 1, RawSum: epathOracleNumber(20 * 3600000), EnergyKWh: epathOracleNumber(20)})
			frames.Site[item.id] = append(frames.Site[item.id], &epathSQLQuantity{Value: 20})
		}
		frames.SourceIdentities[id], frames.SiteSources[item.id] = source, []int{id}
		site := epathRealSQLSite{ID: item.id, Carrier: "electricity", Facility: item.facility}
		if !item.facility {
			site.EndUse = "lighting"
		}
		model.Site = append(model.Site, site)
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), SourceType: "sql_report_data", Name: item.name, IsMeter: true, SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"})
	}
	addElectric := func(nodes *[]EnergyExplanationNode, links *[]EnergyPathLink, period string, value float64) {
		*nodes = append(*nodes, EnergyExplanationNode{ID: "lighting.e", Level: "end_use", EndUse: "lighting", Value: value, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", AggregationBasis: "model_total", Period: period, SourceIDs: []string{"sql-rdd-90"}}, EnergyExplanationNode{ID: "electricity", Level: "carrier", Carrier: "electricity", Value: value, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", AggregationBasis: "model_total", Period: period, SourceIDs: []string{"sql-rdd-91"}})
		*links = append(*links, EnergyPathLink{ID: "lighting.flow", FromID: "lighting.e", ToID: "electricity", Relation: "direct_end_use_to_carrier", Basis: "reported_meter", Period: period, FromValue: value, ToValue: value, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-90"}})
	}
	addElectric(&bundle.EnergyExplanation.Nodes, &bundle.EnergyExplanation.Links, "annual", 240)
	// The original annual district has 15.34 facility,12.34 consumption. The
	// independently observed electric carrier has12*20=240 and no residual.
	bundle.EnergyExplanation.Quality = &EnergyPathQuality{EndUseToCarrierClosedPct: 98.825, EndUseToCarrierStatus: "partial"}
	for i := range bundle.EnergyExplanation.Periods {
		p := &bundle.EnergyExplanation.Periods[i]
		if p.ID == "annual" {
			addElectric(&p.Nodes, &p.Links, p.ID, 240)
			p.Quality = &EnergyPathQuality{EndUseToCarrierClosedPct: 98.825, EndUseToCarrierStatus: "partial"}
			continue
		}
		addElectric(&p.Nodes, &p.Links, p.ID, 20)
		p.Quality = &EnergyPathQuality{EndUseToCarrierClosedPct: 100, EndUseToCarrierStatus: "complete"}
	}
	var prior epathSQLModelChecks
	if err := epathSQLModelSiteChecks(frames, model, &prior); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelSiteFlowChecks(frames, model, &prior); err != nil {
		t.Fatal(err)
	}
	return bundle, frames, model, prior
}

func TestEnergyPathRealSQLQualityClosureAnnualAbsenceIsNotUnknownMonthly(t *testing.T) {
	bundle, frames, model, prior := epathSQLQualityMixedTemporalClosureFixture(t)
	makeCheck := func(period string) epathSQLModelCheck {
		c := epathSQLQualityAccountingUnitCheck("endUseToCarrierClosedPct", "building", "", period, prior.Rows)
		var err error
		c.Quality.AbsentSites, err = epathSQLQualityAbsentSites(frames, model, period)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	for _, period := range epathSQLZoneCarrierPeriods() {
		c := makeCheck(period)
		want, err := epathCheckSQLModelQuality(bundle, c)
		expected := 100.0
		if period == "annual" {
			expected = 98.825
		}
		if err != nil || want.Value == nil || *want.Value != expected {
			t.Fatalf("mixed original annual/monthly %s: %+v %v", period, want, err)
		}
	}
	for _, name := range []string{"unproved nil", "lost flow proof", "different original", "fabricated zero end use", "fabricated zero carrier", "missing monthly electricity", "wrong monthly electricity", "borrow wrong month", "invented monthly accounting"} {
		t.Run(name, func(t *testing.T) {
			b := bundle
			b.EnergyExplanation.Periods = append([]EnergyPeriod(nil), bundle.EnergyExplanation.Periods...)
			p := &b.EnergyExplanation.Periods[0]
			p.Nodes = append([]EnergyExplanationNode(nil), p.Nodes...)
			c := makeCheck("M1")
			c.Quality.Dependencies = append([]epathSQLModelCheck(nil), c.Quality.Dependencies...)
			switch name {
			case "unproved nil":
				c.Quality.AbsentSites = nil
			case "lost flow proof":
				for i := range c.Quality.Dependencies {
					if c.Quality.Dependencies[i].SiteFlow != nil && c.Quality.Dependencies[i].SiteFlow.Unreported {
						c.Quality.Dependencies[i].SiteFlow = nil
					}
				}
			case "different original":
				c.Quality.AbsentSites["carrier/district_cooling"] = c.Quality.AbsentSites["end_use/cooling"]
			case "fabricated zero end use":
				p.Nodes = append(p.Nodes, EnergyExplanationNode{ID: "fake-cooling", Level: "end_use", EndUse: "cooling", Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", Period: "M1"})
			case "fabricated zero carrier":
				p.Nodes = append(p.Nodes, EnergyExplanationNode{ID: "fake-carrier", Level: "carrier", Carrier: "district_cooling", Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", Period: "M1"})
			case "missing monthly electricity":
				p.Nodes = p.Nodes[:1]
			case "wrong monthly electricity":
				p.Nodes[1].Value = 0
			case "borrow wrong month":
				for i := range c.Quality.Dependencies {
					if c.Quality.Dependencies[i].SiteFlow != nil && c.Quality.Dependencies[i].SiteFlow.Unreported {
						c.Quality.Dependencies[i].Item.Period = "M2"
					}
				}
			case "invented monthly accounting":
				p.Reconciliation = []EnergyReconciliation{{ID: "reconcile.energy.district_cooling.M1", Level: "energy", Unit: "kWh", Period: "M1"}}
			}
			if _, err := epathCheckSQLModelQuality(b, c); err == nil {
				t.Fatal("absence-aware closure accepted unknown or fabricated monthly evidence")
			}
		})
	}
	frames.Site["facility.e"][0] = nil
	if _, err := epathSQLQualityAbsentSites(frames, model, "M1"); err == nil {
		t.Fatal("missing original monthly facility converted to annual absence")
	}
}

func TestEnergyPathRealSQLQualityClosureCallsStrictZoneAbsenceProofs(t *testing.T) {
	prior := epathSQLAnnualZoneUnitChecks(t)
	b := epathSQLAnnualZoneUnitBundle()
	checked := 0
	for _, dependency := range prior.Rows {
		if dependency.Item.Period == "annual" || dependency.Item.Target.Collection != "nodes" || dependency.Item.Target.Field != "value" {
			continue
		}
		c := epathSQLQualityAccountingUnitCheck("endUseToCarrierClosedPct", "zone", dependency.Item.Zone, dependency.Item.Period, prior.Rows)
		handled, err := epathSQLQualityCheckAbsentSiteNode(b, c, dependency)
		if !handled || err != nil {
			t.Fatalf("strict Zone absence was not invoked: %s %v", dependency.Want.Key, err)
		}
		checked++
	}
	if checked != 3*12*4 {
		t.Fatalf("lost selected Zone/month/service+carrier absence proofs: %d", checked)
	}
	for _, level := range []string{"end_use", "carrier"} {
		for _, dependency := range prior.Rows {
			if dependency.Item.Zone != "A" || dependency.Item.Period != "M1" || dependency.Item.Target.Level != level || dependency.Item.Target.Field != "value" {
				continue
			}
			mutant := epathSQLAnnualZoneUnitBundle()
			node := EnergyExplanationNode{ID: "invented-zero", Level: level, Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", Period: "M1", ZoneName: "A"}
			if level == "carrier" {
				node.Carrier = dependency.Item.Target.Category
			} else {
				node.EndUse = dependency.Item.Target.Category
			}
			mutant.EnergyExplanation.ZoneResults[0].Periods[1].Nodes = append(mutant.EnergyExplanation.ZoneResults[0].Periods[1].Nodes, node)
			c := epathSQLQualityAccountingUnitCheck("endUseToCarrierClosedPct", "zone", "A", "M1", prior.Rows)
			if handled, err := epathSQLQualityCheckAbsentSiteNode(mutant, c, dependency); !handled || err == nil {
				t.Fatal("Zone unknown numeric became a fabricated zero endpoint")
			}
			break
		}
	}
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
