package simulation

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathSQLAnnualSiteUnitInputs(t *testing.T) (epathSQLFrames, epathRealSQLModel, PurposeResultBundle) {
	t.Helper()
	path := epathSQLTabularUnitFixture(t)
	epathOracleEditSQL(t, path, `INSERT INTO TabularDataWithStrings VALUES(6,'15.34','AnnualBuildingUtilityPerformanceSummary','Entire Facility','End Uses','Total End Uses','District Cooling','kWh')`)
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{"office": {Name: "Office", Multiplier: 1}}, Loads: map[string]epathSQLQuantity{}, Site: map[string][]*epathSQLQuantity{}, SiteAnnual: map[string]epathSQLTabularObservation{}}
	model := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}, Services: []epathRealSQLService{{Service: "cooling", SiteIDs: []string{"cooling.district"}, ServedZones: []string{"Office"}}}}
	for month := 1; month <= 12; month++ {
		value := 1.0
		if month == 1 {
			value = 0
		}
		frames.Loads[epathSQLKey("office", "cooling", month)] = epathSQLQuantity{Value: value}
	}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	for _, item := range []struct {
		id, row, wireID string
		facility        bool
	}{{"cooling.district", "Cooling", "original.annual.cooling", false}, {"facility.district", "Total End Uses", "original.annual.facility", true}} {
		selector := epathSQLTabularUnitSelector()
		selector.RowName = item.row
		site := epathRealSQLSite{ID: item.id, Carrier: "district_cooling", Facility: item.facility, Tabular: &selector}
		if !item.facility {
			site.EndUse = "cooling"
		}
		observation, err := epathReadSQLModelTabular(path, selector)
		if err != nil || observation == nil {
			t.Fatalf("independent original annual cell unavailable: %v", err)
		}
		frames.SiteAnnual[site.ID] = *observation
		model.Site = append(model.Site, site)
		alias := "Cooling:DistrictCooling"
		if site.Facility {
			alias = "DistrictCooling:Facility"
		}
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: item.wireID, Name: alias, KeyValue: alias, SourceType: "sql_tabular", IsMeter: true, Units: "kWh", SourceUnit: "kWh", NormalizedUnit: "kWh", ReportingFrequency: "Annual", AggregationMethod: "tabular_annual_value", AggregationBasis: "model_total", TableName: "End Uses", RowName: item.row, ColumnName: "District Cooling [kWh]", RawValue: observation.Quantity.Value, EffectiveValue: observation.Quantity.Value, inspectorDecodedFromJSON: true, inspectorValuePresence: 3})
	}
	bundle.EnergyExplanation.Nodes = []EnergyExplanationNode{
		{ID: "end_use.cooling", Level: "end_use", EndUse: "cooling", Value: 12.34, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", AggregationBasis: "model_total", Period: "annual", SourceIDs: []string{"original.annual.cooling"}},
		{ID: "carrier.district_cooling.building", Level: "carrier", Carrier: "district_cooling", Value: 15.34, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", AggregationBasis: "model_total", Period: "annual", SourceIDs: []string{"original.annual.facility"}},
		{ID: "residual.site_district_cooling.building", Level: "residual", Carrier: "district_cooling", Value: 3, SignedValue: 3, Unit: "kWh", ScaleDomain: "site", Basis: "residual", AggregationBasis: "model_total", Period: "annual", SourceIDs: []string{"original.annual.cooling", "original.annual.facility"}},
	}
	bundle.EnergyExplanation.Links = []EnergyPathLink{
		{ID: "consumption", FromID: "end_use.cooling", ToID: "carrier.district_cooling.building", Relation: "end_use_to_carrier", Basis: "reported_meter", FromValue: 12.34, ToValue: 12.34, FromUnit: "kWh", ToUnit: "kWh", Period: "annual", SourceIDs: []string{"original.annual.cooling"}},
		{ID: "residual", FromID: "residual.site_district_cooling.building", ToID: "carrier.district_cooling.building", Relation: "residual", Basis: "residual", FromValue: 3, ToValue: 3, FromUnit: "kWh", ToUnit: "kWh", Period: "annual", SourceIDs: []string{"original.annual.cooling", "original.annual.facility"}},
	}
	bundle.EnergyExplanation.Reconciliation = []EnergyReconciliation{{ID: "reconcile.energy.district_cooling.annual", Level: "energy", Period: "annual", ExpectedValue: 15.34, ExplainedValue: 12.34, ResidualValue: 3, Unit: "kWh", Basis: "residual"}}
	for month := 1; month <= 12; month++ {
		bundle.EnergyExplanation.Periods = append(bundle.EnergyExplanation.Periods, EnergyPeriod{ID: fmt.Sprintf("M%d", month), Kind: "monthly"})
	}
	bundle.EnergyExplanation.Periods = append(bundle.EnergyExplanation.Periods, EnergyPeriod{ID: "annual", Kind: "annual",
		Nodes: append([]EnergyExplanationNode(nil), bundle.EnergyExplanation.Nodes...), Links: append([]EnergyPathLink(nil), bundle.EnergyExplanation.Links...),
		Reconciliation: append([]EnergyReconciliation(nil), bundle.EnergyExplanation.Reconciliation...)})
	return frames, model, bundle
}

func epathSQLAnnualSiteUnitChecks(t *testing.T, frames epathSQLFrames, model epathRealSQLModel) epathSQLModelChecks {
	t.Helper()
	var checks epathSQLModelChecks
	for _, build := range []func() error{
		func() error { return epathSQLModelSiteChecks(frames, model, &checks) },
		func() error { return epathSQLModelSiteFlowChecks(frames, model, &checks) },
		func() error { return epathSQLModelSiteResidualChecks(frames, model, &checks) },
		func() error { return epathSQLModelSourceChecks(frames, nil, model, &checks) },
	} {
		if err := build(); err != nil {
			t.Fatal(err)
		}
	}
	return checks
}

func TestEnergyPathRealSQLAnnualSitePeriodKeepsAnnualAndMonthlyDistinct(t *testing.T) {
	frames, _, _ := epathSQLAnnualSiteUnitInputs(t)
	before := frames.SiteAnnual["cooling.district"]
	q, err := epathSQLSitePeriod(frames, "cooling.district", "annual")
	if err != nil || q == nil || !reflect.DeepEqual(*q, before.Quantity) {
		t.Fatalf("original annual precision changed: %+v %v", q, err)
	}
	q.Bounds[0] = -123
	if !reflect.DeepEqual(frames.SiteAnnual["cooling.district"], before) {
		t.Fatal("returned quantity aliases the original observation")
	}
	for month := 1; month <= 12; month++ {
		q, err := epathSQLSitePeriod(frames, "cooling.district", fmt.Sprintf("M%d", month))
		if err != nil || q != nil {
			t.Fatal("annual source became a monthly zero or divided total")
		}
		frames.Site["monthly"] = append(frames.Site["monthly"], &epathSQLQuantity{Value: float64(month)})
	}
	q, err = epathSQLSitePeriod(frames, "monthly", "annual")
	if err != nil || q == nil || q.Value != 78 || q.Error != 0 {
		t.Fatalf("ordinary monthly source no longer sums twelve months: %+v %v", q, err)
	}
	for _, period := range []string{"", "M0", "M13", "M01", "Annual", "annual ", "D1"} {
		if _, err := epathSQLSitePeriod(frames, "cooling.district", period); err == nil {
			t.Fatalf("invalid exact period %q accepted", period)
		}
	}
	if _, err := epathSQLSitePeriod(frames, "missing", "annual"); err == nil {
		t.Fatal("unbound site became unknown instead of a configuration error")
	}
	frames.Site["cooling.district"] = frames.Site["monthly"]
	if _, err := epathSQLSitePeriod(frames, "cooling.district", "annual"); err == nil {
		t.Fatal("duplicate monthly/annual source class accepted")
	}
	for _, bad := range []*epathSQLQuantity{nil, {Value: -1}, {Value: math.NaN()}} {
		frames.Site["monthly"][4] = bad
		if _, err := epathSQLSitePeriod(frames, "monthly", "annual"); err == nil {
			t.Fatal("unknown/negative/nonfinite monthly source was ignored")
		}
	}
}

func TestEnergyPathRealSQLAnnualSiteValuesFlowsResidualsAndSources(t *testing.T) {
	frames, model, bundle := epathSQLAnnualSiteUnitInputs(t)
	checks := epathSQLAnnualSiteUnitChecks(t, frames, model)
	if len(checks.Rows) != 160 {
		t.Fatalf("annual integration lost required period/field/source obligations: %d", len(checks.Rows))
	}
	unknown, sourceCount := 0, 0
	for _, check := range checks.Rows {
		if check.Item.Period != "annual" {
			if check.Quantity != nil || check.Want.Value != nil || check.Want.Status != "unavailable" {
				t.Fatalf("unreported month acquired a numeric expectation: %s", check.Want.Key)
			}
			unknown++
		}
		if check.OriginalSource != nil {
			sourceCount++
			if !epathSQLZoneCarrierQuantityEqual(*check.Quantity, check.OriginalSource.Tabular.Quantity) {
				t.Fatal("original source inherited graph rounding or a synthesized value")
			}
		}
	}
	if unknown != 144 || sourceCount != 4 {
		t.Fatalf("wrong unknown/source counts: %d/%d", unknown, sourceCount)
	}
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 0 {
		t.Fatal(failures)
	}
	// Annual fallback chooses the whole annual observation using annual served
	// loads, not eleven twelfths merely because January has zero served load.
	for _, check := range checks.Rows {
		if check.Item.Period == "annual" && check.SiteFlow != nil && check.Item.Target.Relation == "end_use_to_carrier" && check.Quantity.Value != 12.34 {
			t.Fatal("annual site consumption was distributed or dropped by month")
		}
	}
}

func TestEnergyPathRealSQLAnnualSiteRejectsFabricationAndWrongOriginal(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*PurposeResultBundle)
	}{
		{"monthly zero end use", func(b *PurposeResultBundle) {
			n := b.EnergyExplanation.Nodes[0]
			n.Period, n.Value = "M1", 0
			b.EnergyExplanation.Periods[0].Nodes = []EnergyExplanationNode{n}
		}},
		{"monthly zero carrier", func(b *PurposeResultBundle) {
			n := b.EnergyExplanation.Nodes[1]
			n.Period, n.Value = "M1", 0
			b.EnergyExplanation.Periods[0].Nodes = []EnergyExplanationNode{n}
		}},
		{"monthly zero residual", func(b *PurposeResultBundle) {
			n := b.EnergyExplanation.Nodes[2]
			n.Period, n.Value, n.SignedValue = "M1", 0, 0
			b.EnergyExplanation.Periods[0].Nodes = []EnergyExplanationNode{n}
		}},
		{"monthly divided carrier", func(b *PurposeResultBundle) {
			n := b.EnergyExplanation.Nodes[1]
			n.Period, n.Value = "M1", 15.34/12
			b.EnergyExplanation.Periods[0].Nodes = []EnergyExplanationNode{n}
		}},
		{"wrong annual total", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[0].Value = 12.34 * 11 / 12 }},
		{"borrow facility on end use", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Links[0].SourceIDs = []string{"original.annual.facility"}
		}},
		{"source energy report table", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Sources[0].TableName = "Source Energy End Use Components Summary"
		}},
		{"other district resource", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Sources[0].ColumnName = "District Heating Water [kWh]"
		}},
		{"source wrong raw unit", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].SourceUnit = "GJ" }},
		{"monthly source relabel", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].ReportingFrequency = "Monthly" }},
		{"fake RDD", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].SourceType = "sql_report_data" }},
		{"missing original raw field", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].inspectorValuePresence &^= 1 }},
		{"missing original effective field", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].inspectorValuePresence &^= 2 }},
		{"numeric outside source precision", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].RawValue += .02 }},
		{"duplicate original tuple", func(b *PurposeResultBundle) {
			s := b.EnergyExplanation.Sources[0]
			s.ID = "same-cell-other-id"
			b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, s)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			frames, model, bundle := epathSQLAnnualSiteUnitInputs(t)
			checks := epathSQLAnnualSiteUnitChecks(t, frames, model)
			test.edit(&bundle)
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) == 0 {
				t.Fatal("annual-only observation accepted fabricated or misbound evidence")
			}
		})
	}
}

func TestEnergyPathRealSQLAnnualSiteKnownZeroAndAbsentAreDistinct(t *testing.T) {
	frames, model, bundle := epathSQLAnnualSiteUnitInputs(t)
	for index, site := range model.Site {
		observation := frames.SiteAnnual[site.ID]
		value, quantity, err := epathSQLTabularRawQuantity(observation.Selector, "0.00")
		if err != nil {
			t.Fatal(err)
		}
		observation.RawText, observation.RawValue, observation.Quantity = "0.00", value, quantity
		frames.SiteAnnual[site.ID] = observation
		bundle.EnergyExplanation.Sources[index].RawValue, bundle.EnergyExplanation.Sources[index].EffectiveValue = 0, 0
	}
	q, err := epathSQLSitePeriod(frames, model.Site[0].ID, "annual")
	if err != nil || q == nil || q.Value != 0 || !q.includesZero() {
		t.Fatal("observed annual0 became absent")
	}
	for _, site := range model.Site {
		for _, period := range []string{"M1", "M12"} {
			q, err := epathSQLSitePeriod(frames, site.ID, period)
			if err != nil || q != nil {
				t.Fatal("annual0 became an observed monthly0")
			}
		}
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelSourceChecks(frames, nil, model, &checks); err != nil {
		t.Fatal(err)
	}
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 0 {
		t.Fatal(failures)
	}
	bundle.EnergyExplanation.Sources[0].inspectorValuePresence = 0
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) == 0 {
		t.Fatal("unknown source zero passed as observed zero")
	}
}

func TestEnergyPathRealSQLAnnualSiteRejectsMixedReportingAndSpoofedPrecision(t *testing.T) {
	frames, model, _ := epathSQLAnnualSiteUnitInputs(t)
	observation := frames.SiteAnnual["cooling.district"]
	observation.Quantity = epathSQLQuantity{Value: 12.34, Error: 1}
	frames.SiteAnnual["cooling.district"] = observation
	if _, err := epathSQLSitePeriod(frames, "cooling.district", "annual"); err == nil {
		t.Fatal("widened source precision accepted")
	}
	frames, model, _ = epathSQLAnnualSiteUnitInputs(t)
	monthly := epathRealSQLSite{ID: "monthly.cooling", EndUse: "cooling", Carrier: "district_cooling"}
	model.Site = append(model.Site, monthly)
	for month := 1; month <= 12; month++ {
		frames.Site[monthly.ID] = append(frames.Site[monthly.ID], &epathSQLQuantity{Value: 1})
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelSiteResidualChecks(frames, model, &checks); err == nil || !strings.Contains(err.Error(), "original") && !strings.Contains(err.Error(), "mix") {
		t.Fatal("mixed reporting classes silently accepted")
	}
}
