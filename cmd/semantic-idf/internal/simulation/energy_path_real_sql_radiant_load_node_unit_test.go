package simulation

import (
	"fmt"
	"strings"
	"testing"
)

func epathSQLRadiantConsumerFrames(t *testing.T) (epathSQLFrames, epathRealSQLModel) {
	t.Helper()
	text, binding := epathSQLRadiantHandFixture()
	owners, err := epathSQLRadiantOriginalOwners(text, binding)
	if err != nil {
		t.Fatal(err)
	}
	precision := epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{"office": {Name: "Office", Multiplier: 21}}, Loads: map[string]epathSQLQuantity{}, LoadSourceIDs: map[string][]int{},
		SourceIdentities: map[int]epathRealSQLSource{}, RadiantLoadSourceIdentities: map[int]epathSQLRadiantLoadSourceIdentity{},
		Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceRaw: map[int][]epathSQLQuantity{}}
	model := epathRealSQLModel{Precision: precision}
	for i, service := range []string{"cooling", "heating"} {
		name := "Zone Radiant HVAC Cooling Energy"
		if service == "heating" {
			name = "Zone Radiant HVAC Heating Energy"
		}
		source := epathSQLRadiantUnitSource() // Literal m kWh, except observed M2=0; annual76.
		source.DictionaryIndex, source.Name = 101+i, name
		identity, err := epathSQLRadiantLoadObservation(source, service, owners["radiant"], frames.Zones["office"], precision)
		if err != nil {
			t.Fatal(err)
		}
		frames.SourceIdentities[101+i], frames.RadiantLoadSourceIdentities[101+i] = source, identity
		model.Loads = append(model.Loads, epathRealSQLLoad{Service: service, Component: "combined", NativeRadiant: &binding,
			Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: name, Unit: "J"}}, Keys: []string{"Radiant"}}})
		siteID := service + ".electricity"
		model.Site = append(model.Site, epathRealSQLSite{ID: siteID, EndUse: service, Carrier: "electricity"})
		model.Services = append(model.Services, epathRealSQLService{Service: service, SiteIDs: []string{siteID}, ServedZones: []string{"Office"}, Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "load_to_site_energy", FallbackRatioKind: "load_to_site_energy", ReconciliationID: "allocation." + service + ".annual"})
		frames.SiteSources[siteID] = []int{1 + i}
		frames.SourceIdentities[1+i] = epathRealSQLSource{DictionaryIndex: 1 + i, Name: service + " observed meter", IsMeter: true, ReportingFrequency: "Monthly", SourceUnit: "J"}
		for month := 1; month <= 12; month++ {
			frames.Loads[epathSQLKey("office", service, month)] = identity.Effective[month-1]
			frames.LoadSourceIDs[epathSQLKey("office", service, month)] = []int{101 + i}
			q := epathSQLQuantity{Value: float64(10 * (i + 1))}
			frames.Site[siteID] = append(frames.Site[siteID], &q)
			frames.SourceRaw[1+i] = append(frames.SourceRaw[1+i], q)
		}
	}
	return frames, model
}

// This independent hand candidate is not built from compiled expected values,
// the production selector, multiplier, allocator or graph projection.
func epathSQLRadiantConsumerBundle() PurposeResultBundle {
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}}}
	for i, service := range []string{"cooling", "heating"} {
		name := "Zone Radiant HVAC Cooling Energy"
		if service == "heating" {
			name = "Zone Radiant HVAC Heating Energy"
		}
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources,
			EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", 101+i), Name: name, KeyValue: "RADIANT", ZoneName: "Office", SourceType: "sql_report_data", Units: "J", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", AggregationMethod: "sum_report_data", AggregationBasis: "model_total", EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", RawValue: 76, EffectiveValue: 76},
			EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", 1+i), Name: service + " observed meter", SourceType: "sql_report_data", IsMeter: true, SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"})
	}
	zone := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"}}
	for month := 0; month <= 12; month++ {
		period, load := "annual", 76.0
		if month > 0 {
			period, load = fmt.Sprintf("M%d", month), float64(month)
			if month == 2 {
				load = 0
			}
		}
		buildingNodes, zoneNodes, links := []EnergyExplanationNode{}, []EnergyExplanationNode{}, []EnergyPathLink{}
		carrier := 0.0
		for i, service := range []string{"cooling", "heating"} {
			loadID, siteID, loadSource, siteSource := "load."+service, "use."+service, fmt.Sprintf("sql-rdd-%d", 101+i), fmt.Sprintf("sql-rdd-%d", 1+i)
			node := EnergyExplanationNode{ID: loadID, Level: "load", ServiceKind: service, Value: load, RawValue: load, EffectiveValue: load, Unit: "kWh", ScaleDomain: "thermal", Period: period, Basis: "reported_variable", AggregationBasis: "model_total", ThermalComponent: "combined", ThermalBoundary: "active_surface_source", SourceIDs: []string{loadSource}}
			buildingNodes = append(buildingNodes, node)
			node.ZoneName = "Office"
			zoneNodes = append(zoneNodes, node)
			if month == 2 {
				continue // Known site pool is unassigned when the sole owner load is zero.
			}
			site := float64(10 * (i + 1))
			if month == 0 {
				site *= 11
			}
			carrier += site
			zoneNodes = append(zoneNodes, EnergyExplanationNode{ID: siteID, Level: "end_use", EndUse: service, Value: site, AllocatedValue: site, AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", ZoneName: "Office", Period: period, Basis: "service_path_allocation", SourceIDs: []string{siteSource, loadSource}})
			links = append(links,
				EnergyPathLink{ID: "convert." + service, FromID: loadID, ToID: siteID, Relation: "load_to_end_use", ServiceKind: service, Basis: "service_path_allocation", FromValue: load, ToValue: site, FromUnit: "kWh", ToUnit: "kWh", Ratio: load / site, RatioKind: "load_to_site_energy", SourceIDs: []string{loadSource, siteSource}, ZoneName: "Office", Period: period},
				EnergyPathLink{ID: "site." + service, FromID: siteID, ToID: "electricity", Relation: "end_use_to_carrier", Basis: "service_path_allocation", FromValue: site, ToValue: site, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{siteSource}, ZoneName: "Office", Period: period})
		}
		if carrier > 0 {
			zoneNodes = append(zoneNodes, EnergyExplanationNode{ID: "electricity", Level: "carrier", Carrier: "electricity", Value: carrier, Unit: "kWh", ScaleDomain: "site", ZoneName: "Office", Period: period})
		}
		if month == 0 {
			bundle.EnergyExplanation.Nodes, zone.Nodes, zone.Links = buildingNodes, zoneNodes, links
		} else {
			bundle.EnergyExplanation.Periods = append(bundle.EnergyExplanation.Periods, EnergyPeriod{ID: period, Nodes: buildingNodes})
			zone.Periods = append(zone.Periods, EnergyPeriod{ID: period, Nodes: zoneNodes, Links: links})
		}
	}
	bundle.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{zone}
	return bundle
}

func epathSQLRadiantConsumerChecks(t *testing.T) epathSQLModelChecks {
	t.Helper()
	frames, model := epathSQLRadiantConsumerFrames(t)
	checks := epathSQLModelChecks{}
	if err := epathSQLModelLoadDriverChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return checks
}

func TestEnergyPathRealSQLRadiantConsumersCombinedAndMonthlyService(t *testing.T) {
	checks := epathSQLRadiantConsumerChecks(t)
	if len(checks.Rows) != 260 {
		t.Fatalf("two services x 13 periods lost load/Zone-service obligations: %d", len(checks.Rows))
	}
	boundaries, unknown := 0, 0
	for _, check := range checks.Rows {
		if check.NativeRadiantLoad != nil {
			boundaries++
		}
		if check.Item.Target.Field == "loadBreakdown" {
			unknown++
			if check.Quantity != nil || check.Want.Value != nil || check.Want.Status != "unavailable" || check.OptionalPresentation {
				t.Fatal("combined radiant quantity was assigned to sensible or fabricated as latent zero")
			}
		}
		if check.ZoneService != nil && (len(check.ZoneService.RadiantLoadSources) != 1 || check.Item.Period == "annual" && check.ZoneService.Service == "cooling" && check.Quantity.Value != 110) {
			t.Fatal("exact original native source or monthly-first service sum was lost")
		}
	}
	if boundaries != 52 || unknown != 104 {
		t.Fatalf("boundary/unknown component roster=%d/%d, want52/104", boundaries, unknown)
	}
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, epathSQLRadiantConsumerBundle(), checks); len(failures) != 0 {
		t.Fatalf("hand native radiant consumers failed: %v", failures[:min(5, len(failures))])
	}
}

func TestEnergyPathRealSQLRadiantConsumersRejectNodeAndSourceMutants(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*PurposeResultBundle)
	}{
		{"missing_boundary", func(b *PurposeResultBundle) { b.EnergyExplanation.Periods[0].Nodes[0].ThermalBoundary = "" }},
		{"invented_mixed_boundary", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[0].Nodes[0].ThermalBoundary = "mixed_thermal_boundaries"
		}},
		{"claimed_sensible", func(b *PurposeResultBundle) { b.EnergyExplanation.Periods[0].Nodes[0].ThermalComponent = "sensible" }},
		{"fabricated_sensible_zero", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[0].Nodes[0].LoadBreakdown = []EnergyExplanationLoadComponent{{Component: "sensible", Unit: "kWh"}}
		}},
		{"fabricated_latent_zero", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[0].Nodes[0].LoadBreakdown = []EnergyExplanationLoadComponent{{Component: "latent", Unit: "kWh"}}
		}},
		{"wrong_node_basis", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[0].Nodes[0].AggregationBasis = "zone_equivalent"
		}},
		{"reapplied_zone_factor", func(b *PurposeResultBundle) { b.EnergyExplanation.Periods[0].Nodes[0].Value *= 21 }},
		{"lost_load_source", func(b *PurposeResultBundle) { b.EnergyExplanation.Periods[0].Nodes[0].SourceIDs = nil }},
		{"wrong_service_source", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[0].Nodes[0].SourceIDs = []string{"sql-rdd-102"}
		}},
		{"invented_reference", func(b *PurposeResultBundle) { b.EnergyExplanation.Periods[0].Nodes[0].SourceIDs = []string{"absent"} }},
		{"source_scope_laundered", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].ZoneName = "Other" }},
		{"source_zone_key", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].KeyValue = "Office" }},
		{"source_rescaled", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].EffectiveMultiplier = 21 }},
		{"source_wrong_frequency", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].ReportingFrequency = "Hourly" }},
		{"source_wrapper", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Sources[0].SourceType = "derived"
			b.EnergyExplanation.Sources[0].InputSourceIDs = []string{"sql-rdd-102"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := epathSQLRadiantConsumerBundle()
			test.mutate(&bundle)
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, epathSQLRadiantConsumerChecks(t)); len(failures) == 0 {
				t.Fatal("changed thermal boundary, independent quantity or original provenance passed")
			}
		})
	}
}

func TestEnergyPathRealSQLRadiantConsumersRejectFrameAndProofRebinding(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*epathSQLFrames, *epathRealSQLModel)
	}{
		{"missing_typed_source", func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.RadiantLoadSourceIdentities, 101) }},
		{"missing_month_source", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			delete(f.LoadSourceIDs, epathSQLKey("office", "cooling", 1))
		}},
		{"cross_service_source", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.LoadSourceIDs[epathSQLKey("office", "cooling", 1)] = []int{102}
		}},
		{"quantity_rebound", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.Loads[epathSQLKey("office", "cooling", 1)] = epathSQLQuantity{Value: 21}
		}},
		{"foreign_original_owner", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			p := f.RadiantLoadSourceIdentities[101]
			p.Owner.Owner.ZoneName = "Other"
			f.RadiantLoadSourceIdentities[101] = p
		}},
		{"wrong_original_service", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			p := f.RadiantLoadSourceIdentities[101]
			p.Service = "heating"
			f.RadiantLoadSourceIdentities[101] = p
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			frames, model := epathSQLRadiantConsumerFrames(t)
			test.mutate(&frames, &model)
			if err := epathSQLModelLoadDriverChecks(frames, model, &epathSQLModelChecks{}); err == nil {
				t.Fatal("native load compiler accepted missing/rebound independent source evidence")
			}
			if err := epathSQLModelZoneServiceChecks(frames, model, &epathSQLModelChecks{}); err == nil {
				t.Fatal("Zone service compiler accepted missing/rebound independent native ownership")
			}
		})
	}
	for _, mutation := range []string{"owner", "service", "source", "remove"} {
		t.Run("compiled_"+mutation, func(t *testing.T) {
			checks := epathSQLRadiantConsumerChecks(t)
			for _, check := range checks.Rows {
				if check.ZoneService == nil || check.Item.Period != "M1" || check.ZoneService.Service != "cooling" {
					continue
				}
				p := check.ZoneService.RadiantLoadSources["sql-rdd-101"]
				switch mutation {
				case "owner":
					p.Owner.Owner.ZoneName = "Other"
				case "service":
					p.Service = "heating"
				case "source":
					p.Source.KeyValue = "Office"
				}
				check.ZoneService.RadiantLoadSources["sql-rdd-101"] = p
				if mutation == "remove" {
					check.ZoneService.RadiantLoadSources = nil
				}
				if err := epathCheckSQLZoneServiceEndpoints(epathSQLRadiantConsumerBundle(), check); err == nil {
					t.Fatal("typed service proof mutation accepted without exact original evidence")
				}
				return
			}
			t.Fatal("missing native Zone service check")
		})
	}
}

func TestEnergyPathRealSQLRadiantConsumersDoNotChangeLegacySensibleChecks(t *testing.T) {
	frames, model := epathSQLZoneServiceUnitFrames()
	checks := epathSQLModelChecks{}
	if err := epathSQLModelLoadDriverChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	for _, check := range checks.Rows {
		if check.NativeRadiantLoad != nil || check.Item.Target.AggregationBasis != "" {
			t.Fatal("unmarked legacy load acquired native radiant policy")
		}
		if strings.HasSuffix(check.Item.Key, "/sensible") && check.Quantity == nil {
			t.Fatal("legacy sensible authority was weakened to unknown")
		}
		if strings.HasSuffix(check.Item.Key, "/latent") && check.Quantity != nil {
			t.Fatal("legacy unknown latent component became a fabricated zero")
		}
	}
	if err := epathSQLModelZoneServiceChecks(frames, model, &epathSQLModelChecks{}); err != nil {
		t.Fatalf("existing non-native Zone-key source contract changed: %v", err)
	}
}
