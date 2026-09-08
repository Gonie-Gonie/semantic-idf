package simulation

import (
	"strings"
	"testing"
)

func TestEnergyPathRadiantModelTotalAndOriginalOwner(t *testing.T) {
	doc := energyPathRadiantHandDocument(t)
	doc = parsePurposePlanFixture(t, doc.String()+"\nZoneList,Repeated,Office; ZoneGroup,Repeated Group,Repeated,3;\n")
	context := energyDriverBuildContext{Enabled: true, RadiantLoads: energyPathRadiantLoadTargets(doc), Multipliers: buildEnergyEffectiveMultiplierIndex(doc)}
	record, ok := context.Multipliers.resolve("Office")
	if !ok || record.effectiveMultiplier() != 21 {
		t.Fatalf("fixture requires Zone 7 × Group 3, got %#v", record)
	}
	for _, name := range []string{"Zone Radiant HVAC Cooling Energy", "Zone Radiant HVAC Heating Energy", "Zone Radiant HVAC Cooling Rate", "Zone Radiant HVAC Heating Rate"} {
		t.Run(name, func(t *testing.T) {
			service := "cooling"
			if strings.Contains(name, "Heating") {
				service = "heating"
			}
			item := canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "load", Kind: "load.radiant_" + service, ServiceKind: service, PathType: "zone", Unit: "kWh", ZoneName: "Wrong Plan Owner", SourceName: name, sourceName: name, SourceKey: "RADIANT", sourceKeyValue: "RADIANT", SourceIDs: []string{"rad"}, sourceFrequency: "Monthly", Total: 210, Monthly: map[int]float64{1: 210}})
			sources := []EnergyDataSource{{ID: "rad", Name: name, KeyValue: "RADIANT"}}
			bound, sources, warnings := bindEnergyPathRadiantLoadSeries([]energyExplanationSeries{item}, sources, context)
			if len(warnings) != 0 || len(bound) != 1 || bound[0].ZoneName != "Office" || sources[0].ZoneName != "Office" || sources[0].KeyValue != "RADIANT" || bound[0].ThermalBoundary != energyPathThermalBoundaryActiveSurfaceSource {
				t.Fatalf("original owner/boundary not retained: %#v / %#v / %#v", bound, sources, warnings)
			}
			got, sources, warnings := applyEnergyExplanationMultipliers(bound, sources, context.Multipliers)
			if len(warnings) != 0 || got[0].Total != 210 || got[0].Monthly[1] != 210 || got[0].RawTotal != 210 || got[0].EffectiveMultiplier != 1 || got[0].MultiplierApplication != energyMultiplierAlreadyModelTotal || sources[0].EffectiveValue != 210 {
				t.Fatalf("model-total radiant output was multiplied again: %#v / %#v", got, sources)
			}
			twice, _, _ := applyEnergyExplanationMultipliers(got, sources, context.Multipliers)
			if twice[0].Total != 210 {
				t.Fatal("repeat multiplier pass changed radiant total")
			}
			air := canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "load", Kind: "load.zone_" + service, ServiceKind: service, PathType: "zone", Unit: "kWh", ZoneName: "Office", SourceName: "Zone Air System Sensible " + strings.Title(service) + " Energy", SourceIDs: []string{"air"}, sourceFrequency: "Monthly", Monthly: map[int]float64{1: 0}})
			selected := selectCanonicalEnergyExplanationLoads(append(twice, air))
			if len(selected) != 1 || selected[0].Total != 210 || selected[0].ThermalBoundary != energyPathThermalBoundaryActiveSurfaceSource || selected[0].ZoneName != "Office" {
				t.Fatalf("observed zero air source must not veto positive owned surface source: %#v", selected)
			}
		})
	}
}

func TestEnergyPathRadiantUnownedOutputCannotInventZone(t *testing.T) {
	doc := energyPathRadiantHandDocument(t)
	context := energyDriverBuildContext{Enabled: true, RadiantLoads: energyPathRadiantLoadTargets(doc)}
	for _, service := range []string{"cooling", "heating"} {
		for _, measure := range []string{"Energy", "Rate"} {
			for _, key := range []string{"Office", "Other", "Unknown Radiant Floor", ""} {
				t.Run(service+"/"+measure+"/"+key, func(t *testing.T) {
					name := "Zone Radiant HVAC " + strings.Title(service) + " " + measure
					item := energyExplanationSeries{Level: "load", Kind: "load.radiant_" + service, SourceName: name, SourceKey: key, ZoneName: "Office", SourceIDs: []string{"rad"}, Total: 45, Monthly: map[int]float64{1: 45}}
					got, sources, warnings := bindEnergyPathRadiantLoadSeries([]energyExplanationSeries{item}, []EnergyDataSource{{ID: "rad", Name: name, KeyValue: key, ZoneName: "Office", RawValue: 45, EffectiveValue: 45, DriverRole: energyDriverSourceRoleMainFlow}}, context)
					if len(got) != 0 || len(warnings) != 1 || warnings[0].Code != "radiant_load_owner_unresolved" || len(sources) != 1 {
						t.Fatalf("unowned source not retained exclusively as context: %#v / %#v / %#v", got, sources, warnings)
					}
					source := sources[0]
					if source.RawValue != 45 || source.EffectiveValue != 45 || source.Name != name || source.KeyValue != key || source.ZoneName != "" || source.DriverRole != energyDriverSourceRoleContext || source.DriverCategory != "load."+service || source.DriverComponent != "load.active_surface_source.combined" || source.HeatDirection != service || source.InspectorSection != energyDriverInspectorSectionContext || source.AllocationApplied || len(source.InputSourceIDs) != 0 || source.Formula != "" {
						t.Fatalf("unknown owner lost known non-additive thermal source semantics: %#v", source)
					}
				})
			}
		}
	}
}

func TestEnergyPathRadiantUnselectedBoundaryIsNotCombinedDelivery(t *testing.T) {
	radiant := canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "load", Kind: "load.radiant_cooling", ServiceKind: "cooling", ZoneName: "Office", ThermalBoundary: energyPathThermalBoundaryActiveSurfaceSource, Total: 30})
	air := canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "load", Kind: "load.zone_cooling", ServiceKind: "cooling", ZoneName: "Office", Total: 20})
	if got := energyPathRadiantSelectionWarnings([]energyExplanationSeries{radiant, air}, []energyExplanationSeries{air}); len(got) != 1 || got[0].Code != "radiant_surface_boundary_not_selected" {
		t.Fatalf("different unselected measurement boundary was hidden: %#v", got)
	}
	for _, selected := range [][]energyExplanationSeries{{radiant}, nil} {
		if got := energyPathRadiantSelectionWarnings([]energyExplanationSeries{radiant, air}, selected); len(got) != 0 {
			t.Fatalf("unexpected unselected boundary warning: %#v", got)
		}
	}
}
