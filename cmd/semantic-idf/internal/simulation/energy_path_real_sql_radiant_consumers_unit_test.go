package simulation

import (
	"fmt"
	"reflect"
	"testing"
)

func epathSQLRadiantTraceSource(t *testing.T, bundle *PurposeResultBundle) *EnergyDataSource {
	t.Helper()
	for i := range bundle.EnergyExplanation.Sources {
		if bundle.EnergyExplanation.Sources[i].ID == "sql-rdd-101" {
			return &bundle.EnergyExplanation.Sources[i]
		}
	}
	t.Fatal("independent cooling equipment source absent from hand candidate")
	return nil
}

func epathSQLRadiantQualityTraceFixture(t *testing.T, scope string) (PurposeResultBundle, epathSQLModelCheck) {
	t.Helper()
	identity, source, _ := epathSQLRadiantSourceUnitFixture(t, false)
	bundle, check := epathSQLQualityUnitFixture(t)
	// Original hand SQL has m kWh except known M2=0: sum=76, already model
	// total despite the independently parsed Office factor 7*3. The unrelated
	// site observation remains the existing literal 25 kWh, not 25*21.
	bundle.EnergyExplanation.Sources[0] = source
	bundle.EnergyExplanation.Nodes[0].Value = 76
	bundle.EnergyExplanation.Nodes[0].RawValue = 76
	bundle.EnergyExplanation.Nodes[0].EffectiveValue = 76
	bundle.EnergyExplanation.Nodes[0].AggregationBasis = "model_total"
	bundle.EnergyExplanation.Nodes[0].ThermalComponent = "combined"
	bundle.EnergyExplanation.Nodes[0].ThermalBoundary = "active_surface_source"
	bundle.EnergyExplanation.Nodes[0].SourceIDs = []string{source.ID}
	bundle.EnergyExplanation.Links[0].FromValue = 76
	bundle.EnergyExplanation.Links[0].Ratio = 76.0 / 25
	bundle.EnergyExplanation.Links[0].RatioKind = "load_to_site_energy"
	bundle.EnergyExplanation.Links[0].SourceIDs = []string{source.ID, "source-2"}
	delete(check.Quality.Originals, 1)
	check.Quality.Originals[101] = identity.Source
	check.Quality.LoadSources["cooling"] = []int{101}
	check.Quality.RadiantLoads = map[int]epathSQLRadiantLoadSourceIdentity{101: identity}
	load := epathSQLQuantity{}
	for _, month := range identity.Effective {
		load = load.add(month)
	}
	for i := range check.Quality.Dependencies {
		dependency := &check.Quality.Dependencies[i]
		if dependency.Item.Target.Level == "load" && dependency.Item.Target.Service == "cooling" {
			dependency.Quantity = &load
			dependency.Want.Value = epathOracleNumber(76)
		}
		if dependency.Conversion != nil {
			dependency.Conversion.From = load
			dependency.Item.Target.RatioKind = "load_to_site_energy"
			dependency.Quantity = &epathSQLQuantity{Value: 76.0 / 25}
			dependency.Want.Value = epathOracleNumber(76.0 / 25)
		}
	}
	if scope == "zone" {
		check.Item.Scope, check.Item.Zone = scope, "Office"
		check.Want.Scope, check.Want.Zone = scope, "Office"
		for i := range check.Quality.Dependencies {
			dependency := &check.Quality.Dependencies[i]
			dependency.Item.Scope, dependency.Item.Zone = scope, "Office"
			dependency.Want.Scope, dependency.Want.Zone = scope, "Office"
		}
		for i := range bundle.EnergyExplanation.Nodes {
			bundle.EnergyExplanation.Nodes[i].ZoneName = "Office"
		}
		for i := range bundle.EnergyExplanation.Links {
			bundle.EnergyExplanation.Links[i].ZoneName = "Office"
		}
		bundle.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{{
			Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"},
			Nodes: bundle.EnergyExplanation.Nodes, Links: bundle.EnergyExplanation.Links, Quality: bundle.EnergyExplanation.Quality,
		}}
	}
	return bundle, check
}

// These edits leave the independent physical endpoint numbers unchanged.
// Metadata must not turn an equipment-keyed model total into a Zone variable.
func epathSQLRadiantTraceMetadataMutations() map[string]func(*EnergyDataSource) {
	return map[string]func(*EnergyDataSource){
		"foreign_owner":          func(s *EnergyDataSource) { s.ZoneName = "Other" },
		"equipment_key_as_zone":  func(s *EnergyDataSource) { s.KeyValue = "Office" },
		"wrong_service":          func(s *EnergyDataSource) { s.Name = "Zone Radiant HVAC Heating Energy" },
		"multiplied_again":       func(s *EnergyDataSource) { s.EffectiveMultiplier = 21 },
		"zone_multiplier_policy": func(s *EnergyDataSource) { s.MultiplierApplication = "zone_multiplier" },
		"per_instance_basis":     func(s *EnergyDataSource) { s.AggregationBasis = "per_zone_instance" },
		"wrong_frequency":        func(s *EnergyDataSource) { s.ReportingFrequency = "Hourly" },
		"wrong_unit":             func(s *EnergyDataSource) { s.SourceUnit, s.Units = "W", "W" },
		"wrong_scoped_owner":     func(s *EnergyDataSource) { s.ScopeDetails[0].Scope.ZoneName = "Other" },
		"original_claims_inputs": func(s *EnergyDataSource) {
			s.SourceType, s.InputSourceIDs = "derived", []string{"source-2"}
		},
	}
}

func TestEnergyPathRealSQLRadiantQualityTypedRatioTrace(t *testing.T) {
	for _, scope := range []string{"building", "zone"} {
		t.Run(scope, func(t *testing.T) {
			bundle, check := epathSQLRadiantQualityTraceFixture(t, scope)
			source := epathSQLRadiantTraceSource(t, &bundle)
			if source.RawValue != 76 || source.EffectiveValue != 76 || source.EffectiveMultiplier != 1 || source.KeyValue == source.ZoneName || check.Quality.RadiantLoads[101].Owner.ZoneMultiplier*check.Quality.RadiantLoads[101].Owner.ZoneListMultiplier != 21 {
				t.Fatal("hand original equipment/Zone/model-total boundary was weakened")
			}
			want, err := epathCheckSQLModelQuality(bundle, check)
			if err != nil || want.Status != "complete" || want.Found == nil || *want.Found != 1 || want.Total == nil || *want.Total != 1 {
				t.Fatalf("independent 76/25 native ratio: %#v, %v", want, err)
			}
		})
	}
	for name, mutate := range epathSQLRadiantTraceMetadataMutations() {
		t.Run(name, func(t *testing.T) {
			bundle, check := epathSQLRadiantQualityTraceFixture(t, "zone")
			mutate(epathSQLRadiantTraceSource(t, &bundle))
			if _, err := epathCheckSQLModelQuality(bundle, check); err == nil {
				t.Fatal("ratio count accepted a laundered native original")
			}
		})
	}
	for _, mutation := range []string{"missing_registry", "wrong_registry_service", "wrong_registry_owner", "changed_original_total", "display_multiplied_again"} {
		t.Run(mutation, func(t *testing.T) {
			bundle, check := epathSQLRadiantQualityTraceFixture(t, "zone")
			identity := check.Quality.RadiantLoads[101]
			switch mutation {
			case "missing_registry":
				delete(check.Quality.RadiantLoads, 101)
			case "wrong_registry_service":
				identity.Service = "heating"
				check.Quality.RadiantLoads[101] = identity
			case "wrong_registry_owner":
				identity.Owner.Owner.ZoneName = "Other"
				check.Quality.RadiantLoads[101] = identity
			case "changed_original_total":
				identity.Source.EnergyKWh = epathOracleNumber(77)
				check.Quality.RadiantLoads[101] = identity
			case "display_multiplied_again":
				bundle.EnergyExplanation.ZoneResults[0].Nodes[0].Value = 1596
				bundle.EnergyExplanation.ZoneResults[0].Links[0].FromValue = 1596
				bundle.EnergyExplanation.ZoneResults[0].Links[0].Ratio = 1596.0 / 25
			}
			if _, err := epathCheckSQLModelQuality(bundle, check); err == nil {
				t.Fatal("native ratio accepted missing or contradictory independent proof")
			}
		})
	}
	for _, disguised := range []bool{false, true} {
		t.Run(fmt.Sprintf("nested_wrapper_disguised_%t", disguised), func(t *testing.T) {
			bundle, check := epathSQLRadiantQualityTraceFixture(t, "building")
			wrapper := EnergyDataSource{ID: "derived-load", SourceType: "derived", Name: "Explicit combined load", InputSourceIDs: []string{"sql-rdd-101"}}
			if disguised {
				wrapper = *epathSQLRadiantTraceSource(t, &bundle)
				wrapper.ID, wrapper.SourceType, wrapper.InputSourceIDs = "derived-load", "derived", []string{"sql-rdd-101"}
			}
			bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, wrapper)
			bundle.EnergyExplanation.Nodes[0].SourceIDs = []string{wrapper.ID}
			bundle.EnergyExplanation.Links[0].SourceIDs = []string{wrapper.ID, "source-2"}
			_, err := epathCheckSQLModelQuality(bundle, check)
			if disguised && err == nil || !disguised && err != nil {
				t.Fatalf("legitimate recursive leaf vs original-identity laundering: %v", err)
			}
		})
	}
}

func epathSQLRadiantDriverTraceFixture(t *testing.T) (epathSQLFrames, epathRealSQLModel, epathSQLModelChecks, PurposeResultBundle) {
	t.Helper()
	frames, model := epathSQLRadiantConsumerFrames(t)
	frames.Cells = map[string]*epathSQLCell{}
	bundle := epathSQLRadiantConsumerBundle()
	_, actual, _ := epathSQLRadiantSourceUnitFixture(t, false)
	*epathSQLRadiantTraceSource(t, &bundle) = actual
	for i, service := range []string{"cooling", "heating"} {
		id, family, sign := 200+i, "internal.people", 1.0
		name := "Zone People Convective Heating Energy"
		if service == "heating" {
			family, sign, name = "air.infiltration", -1, "Zone Infiltration Sensible Heat Loss Energy"
		}
		original := epathRealSQLSource{DictionaryIndex: id, Name: name, KeyValue: "Office", SourceUnit: "J", ReportingFrequency: "Monthly"}
		frames.SourceIdentities[id] = original
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), SourceType: "sql_report_data", Name: name, KeyValue: "Office", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"})
		for month := 1; month <= 12; month++ {
			// One independently positive pressure family per service receives the
			// whole native m-kWh load; the observed M2 zero allocates no flow.
			frames.Cells[epathSQLKey("office", family, month)] = &epathSQLCell{Zone: "office", Family: family, Category: family, Component: "sensible", Month: month,
				Raw: epathSQLQuantity{Value: sign}, Effective: epathSQLQuantity{Value: sign * 21}, BuildingVisible: true, SourceIDs: []int{id},
				Allocated: map[string]epathSQLQuantity{service: frames.Loads[epathSQLKey("office", service, month)]}}
		}
	}
	appendGraph := func(period, zone string, nodes *[]EnergyExplanationNode, links *[]EnergyPathLink) {
		value := 76.0
		if period != "annual" {
			var month int
			if _, err := fmt.Sscanf(period, "M%d", &month); err != nil {
				t.Fatal(err)
			}
			value = float64(month)
			if month == 2 {
				return
			}
		}
		for i, service := range []string{"cooling", "heating"} {
			category := "internal.people"
			if service == "heating" {
				category = "air.infiltration"
			}
			sourceID, loadID := fmt.Sprintf("sql-rdd-%d", 200+i), fmt.Sprintf("sql-rdd-%d", 101+i)
			*nodes = append(*nodes, EnergyExplanationNode{ID: "driver." + service, Level: "driver", DriverCategory: category, ServiceKind: service, Basis: "heat_balance_share", Unit: "kWh", ScaleDomain: "thermal", Period: period, ZoneName: zone, Value: value, SourceIDs: []string{sourceID}})
			*links = append(*links, EnergyPathLink{ID: "driver-to-load." + service, FromID: "driver." + service, ToID: "load." + service, Relation: "driver_to_load", ServiceKind: service, Basis: "heat_balance_share", FromUnit: "kWh", ToUnit: "kWh", Period: period, ZoneName: "Office", FromValue: value, ToValue: value, SourceIDs: []string{sourceID, loadID}})
		}
	}
	appendGraph("annual", "", &bundle.EnergyExplanation.Nodes, &bundle.EnergyExplanation.Links)
	zone := &bundle.EnergyExplanation.ZoneResults[0]
	appendGraph("annual", "Office", &zone.Nodes, &zone.Links)
	for i := range bundle.EnergyExplanation.Periods {
		period, zonePeriod := &bundle.EnergyExplanation.Periods[i], &zone.Periods[i]
		appendGraph(period.ID, "", &period.Nodes, &period.Links)
		appendGraph(zonePeriod.ID, "Office", &zonePeriod.Nodes, &zonePeriod.Links)
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelDriverLinkChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return frames, model, checks, bundle
}

func epathSQLRadiantDriverTraceCheck(t *testing.T, checks epathSQLModelChecks) epathSQLModelCheck {
	t.Helper()
	for _, check := range checks.Rows {
		if check.Item.Scope == "zone" && check.Item.Period == "M1" && check.DriverLink.Service == "cooling" && check.DriverLink.Category == "internal.people" && check.Item.Target.Field == "fromValue" {
			return check
		}
	}
	t.Fatal("native monthly owner/category proof was not compiled")
	return epathSQLModelCheck{}
}

func TestEnergyPathRealSQLRadiantDriverTypedFlattenedTrace(t *testing.T) {
	_, _, checks, bundle := epathSQLRadiantDriverTraceFixture(t)
	contexts := map[string]bool{}
	for _, check := range checks.Rows {
		if len(check.DriverLink.RadiantLoadSources) != 2 {
			t.Fatal("driver compiler dropped the independently typed native registry")
		}
		contexts[check.Item.Scope+"/"+check.Item.Period+"/"+check.DriverLink.Service] = true
		if err := epathCheckSQLModelDriverLink(bundle, check); err != nil {
			t.Fatalf("%s: %v", check.Want.Key, err)
		}
	}
	if len(contexts) != 2*13*2 {
		t.Fatalf("missing observed-zero/monthly/annual/scope/service contexts: %d", len(contexts))
	}
	for name, mutate := range epathSQLRadiantTraceMetadataMutations() {
		t.Run(name, func(t *testing.T) {
			_, _, checks, bundle := epathSQLRadiantDriverTraceFixture(t)
			mutate(epathSQLRadiantTraceSource(t, &bundle))
			if epathCheckSQLModelDriverLink(bundle, epathSQLRadiantDriverTraceCheck(t, checks)) == nil {
				t.Fatal("flattened driver source bypassed exact native owner/model-total proof")
			}
		})
	}
	for _, mutation := range []string{"missing_registry", "wrong_registry_service", "wrong_registry_owner", "changed_original_total", "display_multiplied_again", "load_source_on_driver_only"} {
		t.Run(mutation, func(t *testing.T) {
			_, _, checks, bundle := epathSQLRadiantDriverTraceFixture(t)
			check := epathSQLRadiantDriverTraceCheck(t, checks)
			identity := check.DriverLink.RadiantLoadSources[101]
			switch mutation {
			case "missing_registry":
				delete(check.DriverLink.RadiantLoadSources, 101)
			case "wrong_registry_service":
				identity.Service = "heating"
				check.DriverLink.RadiantLoadSources[101] = identity
			case "wrong_registry_owner":
				identity.Owner.Owner.ZoneName = "Other"
				check.DriverLink.RadiantLoadSources[101] = identity
			case "changed_original_total":
				identity.Source.EnergyKWh = epathOracleNumber(77)
				check.DriverLink.RadiantLoadSources[101] = identity
			case "display_multiplied_again":
				bundle.EnergyExplanation.ZoneResults[0].Periods[0].Nodes[0].Value = 21
			case "load_source_on_driver_only":
				period := &bundle.EnergyExplanation.ZoneResults[0].Periods[0]
				period.Nodes[0].SourceIDs = []string{"sql-rdd-200"}
				for i := range period.Nodes {
					if period.Nodes[i].ID == "driver.cooling" {
						period.Nodes[i].SourceIDs = []string{"sql-rdd-200", "sql-rdd-101"}
					}
				}
			}
			if epathCheckSQLModelDriverLink(bundle, check) == nil {
				t.Fatal("native driver accepted contradictory quantity/typed evidence")
			}
		})
	}
	for _, disguised := range []bool{false, true} {
		t.Run(fmt.Sprintf("nested_wrapper_disguised_%t", disguised), func(t *testing.T) {
			_, _, checks, bundle := epathSQLRadiantDriverTraceFixture(t)
			wrapper := EnergyDataSource{ID: "derived-load", SourceType: "derived", Name: "Explicit combined load", InputSourceIDs: []string{"sql-rdd-101"}}
			if disguised {
				wrapper = *epathSQLRadiantTraceSource(t, &bundle)
				wrapper.ID, wrapper.SourceType, wrapper.InputSourceIDs = "derived-load", "derived", []string{"sql-rdd-101"}
			}
			bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, wrapper)
			period := &bundle.EnergyExplanation.ZoneResults[0].Periods[0]
			period.Nodes[0].SourceIDs = []string{wrapper.ID}
			for i := range period.Links {
				if period.Links[i].ID == "driver-to-load.cooling" {
					period.Links[i].SourceIDs = []string{"sql-rdd-200", wrapper.ID}
				}
			}
			err := epathCheckSQLModelDriverLink(bundle, epathSQLRadiantDriverTraceCheck(t, checks))
			if disguised && err == nil || !disguised && err != nil {
				t.Fatalf("recursive driver provenance vs original-identity laundering: %v", err)
			}
		})
	}
}

func TestEnergyPathRealSQLRadiantDriverCompilerKeepsNativeBindings(t *testing.T) {
	for _, mutation := range []string{"missing_registry", "wrong_month_source", "zone_as_equipment", "source_total", "reapplied_factor", "combined_without_native"} {
		t.Run(mutation, func(t *testing.T) {
			frames, model, _, _ := epathSQLRadiantDriverTraceFixture(t)
			switch mutation {
			case "missing_registry":
				delete(frames.RadiantLoadSourceIdentities, 101)
			case "wrong_month_source":
				frames.LoadSourceIDs[epathSQLKey("office", "cooling", 1)] = []int{102}
			case "zone_as_equipment":
				source := frames.SourceIdentities[101]
				source.KeyValue = "Office"
				frames.SourceIdentities[101] = source
			case "source_total":
				source := frames.SourceIdentities[101]
				source.EnergyKWh = epathOracleNumber(77)
				frames.SourceIdentities[101] = source
			case "reapplied_factor":
				key := epathSQLKey("office", "cooling", 1)
				frames.Loads[key] = frames.Loads[key].times(21)
			case "combined_without_native":
				model.Loads[0].NativeRadiant = nil
			}
			if epathSQLModelDriverLinkChecks(frames, model, &epathSQLModelChecks{}) == nil {
				t.Fatal("native driver compilation lost exact original service/month/quantity ownership")
			}
		})
	}
}

func TestEnergyPathRealSQLRadiantTraceRetainsIndependentSourceScalarObligations(t *testing.T) {
	// Ratio/driver trace matchers prove provenance, not annual scalar values.
	// Their matching endpoints cannot discharge the separate source obligations.
	for _, field := range []string{"raw", "effective", "scoped_raw", "scoped_effective"} {
		t.Run(field, func(t *testing.T) {
			_, _, sourceChecks := epathSQLRadiantSourceUnitFixture(t, false)
			bundle, check := epathSQLRadiantQualityTraceFixture(t, "zone")
			if _, err := epathCheckSQLModelQuality(bundle, check); err != nil {
				t.Fatalf("unmodified native ratio: %v", err)
			}
			for _, sourceCheck := range sourceChecks.Rows {
				if err := epathCheckSQLRadiantLoadSource(bundle, sourceCheck); err != nil {
					t.Fatalf("unmodified source obligation: %v", err)
				}
			}
			source := epathSQLRadiantTraceSource(t, &bundle)
			before := append([]EnergyExplanationNode(nil), bundle.EnergyExplanation.ZoneResults[0].Nodes...)
			switch field {
			case "raw":
				source.RawValue = 1596
			case "effective":
				source.EffectiveValue = 1596
			case "scoped_raw":
				source.ScopeDetails[0].RawValue = 1596
			case "scoped_effective":
				source.ScopeDetails[0].EffectiveValue = 1596
			}
			if !reflect.DeepEqual(before, bundle.EnergyExplanation.ZoneResults[0].Nodes) {
				t.Fatal("source-only mutation changed the independent endpoint fixture")
			}
			// Even an unchanged endpoint pair cannot replace these scalar
			// obligations. A future stricter ratio check may also reject it.
			failures := 0
			for _, sourceCheck := range sourceChecks.Rows {
				if epathCheckSQLRadiantLoadSource(bundle, sourceCheck) != nil {
					failures++
				}
			}
			if failures == 0 {
				t.Fatal("valid ratio trace hid a re-scaled original source scalar")
			}
		})
	}
}
