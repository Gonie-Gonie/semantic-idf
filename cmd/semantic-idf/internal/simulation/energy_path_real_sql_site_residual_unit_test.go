package simulation

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathSQLSiteResidualUnitInputs() (epathSQLFrames, epathRealSQLModel) {
	frames := epathSQLFrames{Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{}}
	model := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}}
	for index, item := range []struct {
		id, name, carrier, endUse string
		facility                  bool
		monthly                   [12]float64
	}{
		{"facility.e", "Electricity:Facility", "electricity", "", true, [12]float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100}},
		{"lights.e", "InteriorLights:Electricity", "electricity", "lighting", false, [12]float64{80, 100, 110, 90, 90, 90, 90, 90, 90, 90, 90, 90}},
		{"facility.g", "NaturalGas:Facility", "natural_gas", "", true, [12]float64{50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50}},
		{"heating.g", "Heating:NaturalGas", "natural_gas", "heating", false, [12]float64{50, 50, 50, 50, 50, 60, 40, 50, 50, 50, 50, 50}},
	} {
		id := index + 1
		source := epathRealSQLSource{DictionaryIndex: id, Name: item.name, KeyValue: "Whole Building", IsMeter: true, ReportingFrequency: "Monthly", SourceUnit: "J", Rows: 12}
		annual := 0.0
		for month, value := range item.monthly {
			frames.Site[item.id] = append(frames.Site[item.id], &epathSQLQuantity{Value: value})
			source.Months = append(source.Months, epathRealSQLMonth{Month: month + 1, Rows: 1, RawSum: epathOracleNumber(value * 3600000), EnergyKWh: epathOracleNumber(value)})
			annual += value
		}
		source.RawSum, source.EnergyKWh = epathOracleNumber(annual*3600000), epathOracleNumber(annual)
		frames.SiteSources[item.id], frames.SourceIdentities[id] = []int{id}, source
		model.Site = append(model.Site, epathRealSQLSite{ID: item.id, EndUse: item.endUse, Carrier: item.carrier, Facility: item.facility, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: item.name, Unit: "J"}}, Keys: []string{"Whole Building"}, IsMeter: true}})
	}
	return frames, model
}

func epathSQLSiteResidualUnitSources() []EnergyDataSource {
	frames, _ := epathSQLSiteResidualUnitInputs()
	sources := []EnergyDataSource{}
	for id := 1; id <= 4; id++ {
		original := frames.SourceIdentities[id]
		sources = append(sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), Name: original.Name, KeyValue: original.KeyValue, SourceType: "sql_report_data", IsMeter: true, SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"})
	}
	return sources
}

func epathSQLSiteResidualUnitPeriod(period, carrier string, expected, residual float64, visible bool) EnergyPeriod {
	p := EnergyPeriod{ID: period, Kind: "monthly"}
	if period == "annual" {
		p.Kind = "annual"
	}
	p.Nodes = append(p.Nodes, EnergyExplanationNode{ID: "carrier." + carrier + ".building", Level: "carrier", Carrier: carrier, Value: expected, Period: period, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", AggregationBasis: "model_total"})
	if !visible {
		return p
	}
	ids := []string{"sql-rdd-1", "sql-rdd-2"}
	if carrier == "natural_gas" {
		ids = []string{"sql-rdd-3", "sql-rdd-4"}
	}
	id := "residual.site_" + carrier + ".building"
	p.Nodes = append(p.Nodes, EnergyExplanationNode{ID: id, Level: "residual", Carrier: carrier, Value: residual, SignedValue: residual, Unit: "kWh", ScaleDomain: "site", Basis: "residual", AggregationBasis: "model_total", Period: period, SourceIDs: append([]string(nil), ids...)})
	p.Links = []EnergyPathLink{{ID: "residual.link." + carrier, FromID: id, ToID: "carrier." + carrier + ".building", Relation: "residual", Basis: "residual", FromValue: residual, ToValue: residual, FromUnit: "kWh", ToUnit: "kWh", Period: period, SourceIDs: append([]string(nil), ids...)}}
	return p
}

func epathSQLSiteResidualUnitBundle() PurposeResultBundle {
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Sources: epathSQLSiteResidualUnitSources()}}
	for _, period := range []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"} {
		eExpected, eResidual, gExpected, gResidual := 100.0, 10.0, 50.0, 0.0
		switch period {
		case "annual":
			eExpected, eResidual, gExpected, gResidual = 1200, 100, 600, 0
		case "M1":
			eResidual = 20
		case "M2":
			eResidual = 0
		case "M3":
			eResidual = -10
		case "M6":
			gResidual = -10
		case "M7":
			gResidual = 10
		}
		p := epathSQLSiteResidualUnitPeriod(period, "electricity", eExpected, eResidual, eResidual > 0)
		gas := epathSQLSiteResidualUnitPeriod(period, "natural_gas", gExpected, gResidual, gResidual > 0)
		p.Nodes, p.Links = append(p.Nodes, gas.Nodes...), append(p.Links, gas.Links...)
		if period == "annual" {
			bundle.EnergyExplanation.Nodes = append([]EnergyExplanationNode(nil), p.Nodes...)
			bundle.EnergyExplanation.Links = append([]EnergyPathLink(nil), p.Links...)
		}
		bundle.EnergyExplanation.Periods = append(bundle.EnergyExplanation.Periods, p)
	}
	return bundle
}

func epathSQLSiteResidualUnitChecks(t *testing.T) epathSQLModelChecks {
	t.Helper()
	frames, model := epathSQLSiteResidualUnitInputs()
	var checks epathSQLModelChecks
	if err := epathSQLModelSiteResidualChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return checks
}

func TestEnergyPathRealSQLSiteResidualMonthlyAndAnnualSignedAccounting(t *testing.T) {
	checks, bundle := epathSQLSiteResidualUnitChecks(t), epathSQLSiteResidualUnitBundle()
	before := epathSQLSiteResidualUnitBundle()
	if len(checks.Rows) != 13*2*3 {
		t.Fatalf("missing per-period/carrier/node/paired-endpoint obligations: %d", len(checks.Rows))
	}
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 0 {
		t.Fatal(failures)
	}
	if !reflect.DeepEqual(bundle, before) {
		t.Fatal("residual checks changed the original candidate")
	}
	for _, check := range checks.Rows {
		p := check.SiteResidual
		if p.Carrier == "electricity" && check.Item.Period == "annual" && (p.Expected.Value != 1200 || p.Mapped.Value != 1100 || p.Residual.Value != 100) {
			t.Fatal("annual residual must retain the signed -10 month, not sum110 positive drawings")
		}
		if p.Carrier == "natural_gas" && check.Item.Period == "annual" && p.Residual.Value != 0 {
			t.Fatal("annual +10/-10 accounting must remain0 with no annual residual drawing")
		}
		if p.Carrier == "electricity" && check.Item.Period == "M3" && (p.Residual.Value != -10 || check.Quantity.Value != 0) {
			t.Fatal("negative overmapping was lost or became a fabricated positive flow")
		}
	}
}

func TestEnergyPathRealSQLSiteResidualAdversarialVisibleBranch(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*PurposeResultBundle)
	}{
		{"missing material node", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes[:1], b.EnergyExplanation.Nodes[2:]...)
			b.EnergyExplanation.Links = nil
		}},
		{"missing link", func(b *PurposeResultBundle) { b.EnergyExplanation.Links = nil }},
		{"duplicate residual node", func(b *PurposeResultBundle) {
			n := b.EnergyExplanation.Nodes[1]
			n.ID = "duplicate"
			b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes, n)
		}},
		{"duplicate residual link", func(b *PurposeResultBundle) {
			l := b.EnergyExplanation.Links[0]
			l.ID = "duplicate"
			b.EnergyExplanation.Links = append(b.EnergyExplanation.Links, l)
		}},
		{"wrong node carrier", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].Carrier = "natural_gas" }},
		{"wrong target carrier", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].ToID = "carrier.natural_gas.building" }},
		{"wrong target identity", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes[0].ID = "carrier.electricity.other"
			b.EnergyExplanation.Links[0].ToID = "carrier.electricity.other"
		}},
		{"wrong target level", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[0].Level = "end_use" }},
		{"wrong target unit", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[0].Unit = "MJ" }},
		{"wrong target domain", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[0].ScaleDomain = "thermal" }},
		{"wrong target basis", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[0].Basis = "service_path_allocation" }},
		{"wrong target aggregation", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[0].AggregationBasis = "native_zone" }},
		{"wrong target zone", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[0].ZoneName = "Office" }},
		{"wrong target service", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[0].ServiceKind = "cooling" }},
		{"wrong residual zone", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].ZoneName = "Office" }},
		{"wrong residual domain", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].ScaleDomain = "thermal" }},
		{"wrong residual basis", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].Basis = "reported_meter" }},
		{"wrong residual service", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].ServiceKind = "heating" }},
		{"wrong link zone", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].ZoneName = "Office" }},
		{"wrong signed value", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].SignedValue = -100 }},
		{"wrong residual number", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes[1].Value = 99
			b.EnergyExplanation.Nodes[1].SignedValue = 99
		}},
		{"wrong paired from", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].FromValue-- }},
		{"wrong paired to", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].ToValue-- }},
		{"wrong basis", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].Basis = "reported_meter" }},
		{"invented ratio", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].Ratio = 1 }},
		{"invented service", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].ServiceKind = "cooling" }},
		{"missing required node source", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].SourceIDs = []string{"sql-rdd-1"} }},
		{"missing required link source", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].SourceIDs = []string{"sql-rdd-2"} }},
		{"borrow other carrier source", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Links[0].SourceIDs = []string{"sql-rdd-3", "sql-rdd-4"}
		}},
		{"unknown source", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Links[0].SourceIDs = append(b.EnergyExplanation.Links[0].SourceIDs, "unknown")
		}},
		{"wrong source owner", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].KeyValue = "Other Building" }},
		{"wrong source original unit", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].SourceUnit = "kWh" }},
		{"wrong source normalized unit", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].NormalizedUnit = "J" }},
		{"wrong source frequency", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].ReportingFrequency = "Hourly" }},
		{"wrong source type", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].SourceType = "derived_formula" }},
		{"wrong source meter role", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].IsMeter = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := epathSQLSiteResidualUnitBundle()
			test.edit(&bundle)
			checks := epathSQLSiteResidualUnitChecks(t)
			annual := checks.Rows[:0]
			for _, check := range checks.Rows {
				if check.Item.Period == "annual" {
					annual = append(annual, check)
				}
			}
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, epathSQLModelChecks{Rows: annual}); len(failures) == 0 {
				t.Fatal("invalid original residual branch/source was accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLSiteResidualPositiveThresholdUsesOR(t *testing.T) {
	for _, test := range []struct {
		name               string
		expected, residual float64
		visible            bool
	}{
		{"absolute only", 100, .02, true},
		{"relative only", .1, .003, true},
		{"below absolute threshold", 100, .0099, false},
		{"below relative threshold", .25, .0049, false},
		{"below both", 100, .009, false},
		{"rounds to zero despite relative threshold", .01, .0004, false},
		{"positive representable relative threshold", .01, .0006, true},
		{"zero", 100, 0, false},
		{"negative", 100, -10, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			frames, _ := epathSQLSiteResidualUnitInputs()
			p := &epathSQLSiteResidualProof{Carrier: "electricity", Expected: epathSQLQuantity{Value: test.expected}, Mapped: epathSQLQuantity{Value: test.expected - test.residual}, Sources: map[string]epathRealSQLSource{"sql-rdd-1": frames.SourceIdentities[1], "sql-rdd-2": frames.SourceIdentities[2]}, Required: map[string]bool{"sql-rdd-1": true, "sql-rdd-2": true}}
			p.Residual = p.Expected.add(p.Mapped.times(-1))
			check := epathSQLModelCheck{Item: epathRealOracleMetricRecipe{Scope: "building", Period: "M1", Unit: "kWh", Target: epathRealOracleTarget{Collection: "nodes", Field: "value", ID: "residual.site_electricity.building", Level: "residual", Unit: "kWh", ScaleDomain: "site", Basis: "residual"}}, Quantity: func() *epathSQLQuantity { q := p.Residual.positive(); return &q }(), SiteResidual: p}
			for _, visible := range []bool{false, true} {
				value := test.residual
				if value <= 0 {
					value = .001
				}
				period := epathSQLSiteResidualUnitPeriod("M1", "electricity", test.expected, value, visible)
				bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Sources: epathSQLSiteResidualUnitSources(), Periods: []EnergyPeriod{period}}}
				if err := epathCheckSQLSiteResidual(bundle, check); (err == nil) != (visible == test.visible) {
					t.Fatalf("visible=%v expected=%v: %v", visible, test.visible, err)
				}
			}
		})
	}
}

func TestEnergyPathRealSQLSiteResidualMissingAndUnknownFacility(t *testing.T) {
	for _, name := range []string{"missing facility declaration", "duplicate facility declaration", "missing month", "unknown month", "nonfinite month", "negative meter", "missing source", "foreign dictionary index", "variable instead of meter"} {
		t.Run(name, func(t *testing.T) {
			frames, model := epathSQLSiteResidualUnitInputs()
			switch name {
			case "missing facility declaration":
				model.Site = model.Site[1:]
			case "duplicate facility declaration":
				model.Site = append(model.Site, model.Site[0])
			case "missing month":
				frames.Site["facility.e"] = frames.Site["facility.e"][:11]
			case "unknown month":
				frames.Site["facility.e"][0] = nil
			case "nonfinite month":
				frames.Site["facility.e"][0] = &epathSQLQuantity{Value: math.Inf(1)}
			case "negative meter":
				frames.Site["facility.e"][0] = &epathSQLQuantity{Value: -1}
			case "missing source":
				delete(frames.SourceIdentities, 1)
			case "foreign dictionary index":
				s := frames.SourceIdentities[1]
				s.DictionaryIndex = 99
				frames.SourceIdentities[1] = s
			case "variable instead of meter":
				s := frames.SourceIdentities[1]
				s.IsMeter = false
				frames.SourceIdentities[1] = s
			}
			if err := epathSQLModelSiteResidualChecks(frames, model, &epathSQLModelChecks{}); err == nil {
				t.Fatalf("%s became a known facility residual", name)
			}
		})
	}
}

func TestEnergyPathRealSQLSiteResidualSuppressedMonthlyCannotAppear(t *testing.T) {
	for _, test := range []struct {
		period, carrier string
		expected        float64
	}{{"M2", "electricity", 100}, {"M3", "electricity", 100}, {"M6", "natural_gas", 50}, {"annual", "natural_gas", 600}} {
		t.Run(test.period+"/"+test.carrier, func(t *testing.T) {
			bundle := epathSQLSiteResidualUnitBundle()
			fake := epathSQLSiteResidualUnitPeriod(test.period, test.carrier, test.expected, 1, true)
			if test.period == "annual" {
				bundle.EnergyExplanation.Nodes = append(bundle.EnergyExplanation.Nodes, fake.Nodes[1])
				bundle.EnergyExplanation.Links = append(bundle.EnergyExplanation.Links, fake.Links...)
			} else {
				for i := range bundle.EnergyExplanation.Periods {
					if bundle.EnergyExplanation.Periods[i].ID == test.period {
						bundle.EnergyExplanation.Periods[i].Nodes = append(bundle.EnergyExplanation.Periods[i].Nodes, fake.Nodes[1])
						bundle.EnergyExplanation.Periods[i].Links = append(bundle.EnergyExplanation.Periods[i].Links, fake.Links...)
					}
				}
			}
			failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, epathSQLSiteResidualUnitChecks(t))
			found := false
			for _, failure := range failures {
				found = found || strings.Contains(failure.Key, "|"+test.period+"|site_residual/"+test.carrier+"/")
			}
			if !found {
				t.Fatal("zero/negative accounting gained an unauthorized positive drawing")
			}
		})
	}
}
