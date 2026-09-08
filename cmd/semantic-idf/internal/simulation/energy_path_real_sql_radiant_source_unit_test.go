package simulation

import (
	"fmt"
	"testing"
)

func epathSQLRadiantSourceUnitFixture(t *testing.T, zero bool) (epathSQLRadiantLoadSourceIdentity, EnergyDataSource, epathSQLModelChecks) {
	t.Helper()
	text, binding := epathSQLRadiantHandFixture()
	owners, err := epathSQLRadiantOriginalOwners(text, binding)
	if err != nil {
		t.Fatal(err)
	}
	source := epathSQLRadiantUnitSource()
	if zero {
		source.RawSum, source.EnergyKWh = epathOracleNumber(0), epathOracleNumber(0)
		for i := range source.Months {
			source.Months[i].RawSum, source.Months[i].EnergyKWh = epathOracleNumber(0), epathOracleNumber(0)
		}
	}
	identity, err := epathSQLRadiantLoadObservation(source, "cooling", owners["radiant"], epathSQLZone{Name: "Office", Multiplier: 21}, epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2})
	if err != nil {
		t.Fatal(err)
	}
	value := *source.EnergyKWh
	actual := EnergyDataSource{ID: "sql-rdd-101", SourceType: "sql_report_data", Name: source.Name, KeyValue: source.KeyValue, ZoneName: "Office", SourceUnit: "J", Units: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", AggregationMethod: "sum_report_data", AggregationBasis: "model_total", RawValue: value, EffectiveValue: value, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", inspectorDecodedFromJSON: true, inspectorValuePresence: 3,
		ScopeDetails: []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"}, RawValue: value, EffectiveValue: value, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", AggregationBasis: "model_total", inspectorDecodedFromJSON: true, inspectorValuePresence: 3}}}
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{"office": {Name: "Office", Multiplier: 21}}, SourceIdentities: map[int]epathRealSQLSource{101: source}, SourceRaw: map[int][]epathSQLQuantity{101: identity.Raw[:]}, SourceEffective: map[int][]epathSQLQuantity{101: identity.Effective[:]}, SourceZone: map[int]string{101: "office"}, RadiantLoadSourceIdentities: map[int]epathSQLRadiantLoadSourceIdentity{101: identity}}
	checks := epathSQLModelChecks{}
	if err := epathSQLModelSourceChecks(frames, []epathRealSQLSource{source}, epathRealSQLModel{}, &checks); err != nil {
		t.Fatal(err)
	}
	return identity, actual, checks
}

func TestEnergyPathRealSQLRadiantSourceScalarsAndKnownZero(t *testing.T) {
	for _, zero := range []bool{false, true} {
		identity, actual, checks := epathSQLRadiantSourceUnitFixture(t, zero)
		if len(checks.Rows) != 4 {
			t.Fatal("raw/effective Building+owned Zone metric roster changed")
		}
		for _, check := range checks.Rows {
			if check.NativeRadiantSource == nil || check.Want.Value == nil || *check.Want.Value != *identity.Source.EnergyKWh {
				t.Fatalf("source metric lost original quantity/known zero: %#v", check)
			}
			bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Sources: []EnergyDataSource{actual}}}
			if err := epathCheckSQLRadiantLoadSource(bundle, check); err != nil {
				t.Fatal(err)
			}
		}
		original, err := epathSQLOriginalRadiant(identity)
		if err != nil {
			t.Fatal(err)
		}
		allowed := map[string]epathSQLOriginalSource{"sql-rdd-101": original}
		for _, period := range []string{"annual", "M1", "M2"} {
			if err := epathSQLVerifyOriginalSources([]string{actual.ID}, map[string]EnergyDataSource{actual.ID: actual}, allowed, map[string]bool{actual.ID: true}, period); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestEnergyPathRealSQLRadiantSourceRejectsMetadataAndNumericMutants(t *testing.T) {
	for index, mutate := range []func(*EnergyDataSource){
		func(s *EnergyDataSource) { s.ID = "sql-rdd-999" },
		func(s *EnergyDataSource) { s.KeyValue = "Office" },
		func(s *EnergyDataSource) { s.ZoneName = "Other" },
		func(s *EnergyDataSource) { s.EffectiveMultiplier = 21 },
		func(s *EnergyDataSource) { s.MultiplierApplication = "zone_multiplier" },
		func(s *EnergyDataSource) { s.AggregationBasis = "per_zone_instance" },
		func(s *EnergyDataSource) { s.ReportingFrequency = "Hourly" },
		func(s *EnergyDataSource) { s.AggregationMethod = "" },
		func(s *EnergyDataSource) { s.AggregationMethod = "sum" },
		func(s *EnergyDataSource) { s.AggregationMethod = "average" },
		func(s *EnergyDataSource) { s.AggregationMethod = "integrate_rate_by_time_interval" },
		func(s *EnergyDataSource) { s.Units = "W" },
		func(s *EnergyDataSource) { s.IsMeter = true },
		func(s *EnergyDataSource) { s.InputSourceIDs = []string{"sql-rdd-101"} },
		func(s *EnergyDataSource) { s.ScopeDetails[0].Scope.ZoneName = "Other" },
		func(s *EnergyDataSource) { s.ScopeDetails[0].Scope.Kind = "building" },
		func(s *EnergyDataSource) { s.ScopeDetails = append(s.ScopeDetails, s.ScopeDetails[0]) },
		func(s *EnergyDataSource) { s.ScopeDetails[0].EffectiveMultiplier = 21 },
		func(s *EnergyDataSource) { s.RawValue = 1596 },
		func(s *EnergyDataSource) { s.EffectiveValue = 1596 },
		func(s *EnergyDataSource) { s.ScopeDetails[0].RawValue = 1596 },
		func(s *EnergyDataSource) { s.ScopeDetails[0].EffectiveValue = 1596 },
		func(s *EnergyDataSource) { s.inspectorValuePresence = 0 },
		func(s *EnergyDataSource) { s.ScopeDetails[0].inspectorValuePresence = 0 },
	} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			_, actual, checks := epathSQLRadiantSourceUnitFixture(t, false)
			mutate(&actual)
			failures := 0
			for _, check := range checks.Rows {
				if epathCheckSQLRadiantLoadSource(PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Sources: []EnergyDataSource{actual}}}, check) != nil {
					failures++
				}
			}
			if failures == 0 {
				t.Fatal("source metadata/numeric mutant escaped every original scalar obligation")
			}
		})
	}
	_, actual, checks := epathSQLRadiantSourceUnitFixture(t, true)
	actual.ScopeDetails = nil
	for _, check := range checks.Rows {
		if check.Item.Scope == "zone" && epathCheckSQLRadiantLoadSource(PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Sources: []EnergyDataSource{actual}}}, check) == nil {
			t.Fatal("missing scoped zero was filled from a Building scalar")
		}
	}
}

func TestEnergyPathRealSQLRadiantSourceProofAndWrapperCannotBeForged(t *testing.T) {
	identity, actual, checks := epathSQLRadiantSourceUnitFixture(t, false)
	bad := identity
	bad.Effective[0] = bad.Effective[0].times(21)
	if epathSQLValidateRadiantLoadSourceIdentity(bad) == nil {
		t.Fatal("forged native effective quantity accepted")
	}
	bad = identity
	bad.ThermalBoundary = ""
	if _, err := epathSQLOriginalRadiant(bad); err == nil {
		t.Fatal("lost active thermal boundary accepted")
	}
	original, err := epathSQLOriginalRadiant(identity)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]epathSQLOriginalSource{actual.ID: original}
	wrapper := actual
	wrapper.ID, wrapper.SourceType, wrapper.InputSourceIDs = "derived-spoof", "derived", []string{actual.ID}
	if _, err := epathSQLOriginalSourceLeaves([]string{wrapper.ID}, map[string]EnergyDataSource{actual.ID: actual, wrapper.ID: wrapper}, allowed, "M1"); err == nil {
		t.Fatal("same-name/key original source masqueraded as a derived wrapper")
	}
	for _, mutate := range []func(*epathSQLModelCheck){
		func(c *epathSQLModelCheck) { c.Item.Period = "M1" },
		func(c *epathSQLModelCheck) { c.Item.Scope, c.Item.Zone = "zone", "Other" },
		func(c *epathSQLModelCheck) { c.Item.Target.SourceKey = "Office" },
		func(c *epathSQLModelCheck) { c.Quantity = &epathSQLQuantity{Value: 1596} },
	} {
		check := checks.Rows[0]
		mutate(&check)
		if epathCheckSQLRadiantLoadSource(PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Sources: []EnergyDataSource{actual}}}, check) == nil {
			t.Fatal("typed source check escaped exact original context/quantity")
		}
	}
}

func TestEnergyPathRealSQLRadiantSourceFrameMapsCannotDropOrRescaleProof(t *testing.T) {
	identity, _, _ := epathSQLRadiantSourceUnitFixture(t, false)
	_, binding := epathSQLRadiantHandFixture()
	model := epathRealSQLModel{Loads: []epathRealSQLLoad{{Service: "cooling", Component: "combined", NativeRadiant: &binding, Source: epathRealSQLSelector{Keys: []string{"Radiant"}, Alternatives: []epathRealSQLAlternative{{Name: "Zone Radiant HVAC Cooling Energy", Unit: "J"}}}}}}
	for index, mutate := range []func(*epathSQLFrames, *[]epathRealSQLSource){
		func(f *epathSQLFrames, _ *[]epathRealSQLSource) { delete(f.SourceRaw, 101) },
		func(f *epathSQLFrames, _ *[]epathRealSQLSource) { delete(f.SourceEffective, 101) },
		func(f *epathSQLFrames, _ *[]epathRealSQLSource) { delete(f.SourceZone, 101) },
		func(f *epathSQLFrames, _ *[]epathRealSQLSource) { delete(f.SourceIdentities, 101) },
		func(f *epathSQLFrames, _ *[]epathRealSQLSource) { delete(f.RadiantLoadSourceIdentities, 101) },
		func(f *epathSQLFrames, _ *[]epathRealSQLSource) { f.SourceRaw[101][0] = f.SourceRaw[101][0].times(21) },
		func(f *epathSQLFrames, _ *[]epathRealSQLSource) {
			f.SourceEffective[101][0] = f.SourceEffective[101][0].times(21)
		},
		func(f *epathSQLFrames, _ *[]epathRealSQLSource) { f.SourceZone[101] = "other" },
		func(f *epathSQLFrames, _ *[]epathRealSQLSource) {
			f.Zones["office"] = epathSQLZone{Name: "Other", Multiplier: 21}
		},
		func(_ *epathSQLFrames, sources *[]epathRealSQLSource) {
			(*sources)[0].Name = "Zone Air System Sensible Cooling Energy"
		},
		func(_ *epathSQLFrames, sources *[]epathRealSQLSource) { *sources = append(*sources, (*sources)[0]) },
	} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			frames := epathSQLFrames{Zones: map[string]epathSQLZone{"office": {Name: "Office", Multiplier: 21}}, SourceIdentities: map[int]epathRealSQLSource{101: identity.Source}, SourceRaw: map[int][]epathSQLQuantity{101: append([]epathSQLQuantity(nil), identity.Raw[:]...)}, SourceEffective: map[int][]epathSQLQuantity{101: append([]epathSQLQuantity(nil), identity.Effective[:]...)}, SourceZone: map[int]string{101: "office"}, RadiantLoadSourceIdentities: map[int]epathSQLRadiantLoadSourceIdentity{101: identity}}
			observed := []epathRealSQLSource{identity.Source}
			mutate(&frames, &observed)
			checks := epathSQLModelChecks{}
			if epathSQLModelSourceChecks(frames, observed, model, &checks) == nil {
				t.Fatal("altered source map removed or rescaled original typed source obligations")
			}
		})
	}
}
