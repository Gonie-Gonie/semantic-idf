package simulation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// These requested names and observed alias families were independently checked
// against the official 25.1 LargeOffice run's output plan, RDD and Monthly SQL
// dictionary (20260907T162304). This is a closed availability-shape regression,
// NOT an EnergyPlus numeric golden: values below are small deterministic inputs.
// Grouped sensible/latent or context names are availability families only; they
// do not imply interchangeable or additive physical measurements.
var epathRealReportedAvailabilityGroups = []struct {
	level, kind         string
	requested, observed []string
}{
	{level: "load", kind: "load.plant_unmet_or_residual", requested: []string{
		"Cond Loop Demand Not Distributed",
		"Plant Supply Side Not Distributed Demand Rate",
		"Plant Supply Side Unmet Demand Rate",
	}, observed: []string{"Plant Supply Side Not Distributed Demand Rate", "Plant Supply Side Unmet Demand Rate"}},
	{level: "load", kind: "load.system_cooling", requested: []string{
		"Cooling Coil Sensible Cooling Energy",
		"Cooling Coil Total Cooling Energy",
		"Cooling Coil Total Cooling Rate",
	}, observed: []string{"Cooling Coil Total Cooling Energy", "Cooling Coil Sensible Cooling Energy", "Cooling Coil Total Cooling Rate"}},
	{level: "load", kind: "load.system_heating", requested: []string{
		"Heating Coil Heating Energy",
		"Heating Coil Heating Rate",
	}, observed: []string{"Heating Coil Heating Energy", "Heating Coil Heating Rate"}},
	{level: "load", kind: "load.plant_cooling", requested: []string{
		"Plant Loop Cooling Demand Energy",
		"Plant Supply Side Cooling Demand Rate",
	}, observed: []string{"Plant Supply Side Cooling Demand Rate"}},
	{level: "load", kind: "load.plant_heating", requested: []string{
		"Plant Loop Heating Demand Energy",
		"Plant Supply Side Heating Demand Rate",
	}, observed: []string{"Plant Supply Side Heating Demand Rate"}},
	{level: "heat", kind: "heat.infiltration", requested: []string{
		"AFN Zone Infiltration Latent Heat Gain Energy",
		"AFN Zone Infiltration Latent Heat Gain Rate",
		"AFN Zone Infiltration Latent Heat Loss Energy",
		"AFN Zone Infiltration Latent Heat Loss Rate",
		"AFN Zone Infiltration Sensible Heat Gain Energy",
		"AFN Zone Infiltration Sensible Heat Gain Rate",
		"AFN Zone Infiltration Sensible Heat Loss Energy",
		"AFN Zone Infiltration Sensible Heat Loss Rate",
		"Zone Infiltration Latent Heat Gain Energy",
		"Zone Infiltration Latent Heat Gain Rate",
		"Zone Infiltration Latent Heat Loss Energy",
		"Zone Infiltration Latent Heat Loss Rate",
		"Zone Infiltration Sensible Heat Gain Energy",
		"Zone Infiltration Sensible Heat Gain Rate",
		"Zone Infiltration Sensible Heat Loss Energy",
		"Zone Infiltration Sensible Heat Loss Rate",
	}, observed: []string{"Zone Infiltration Sensible Heat Loss Energy", "Zone Infiltration Sensible Heat Gain Energy", "Zone Infiltration Latent Heat Loss Energy", "Zone Infiltration Latent Heat Gain Energy"}},
	{level: "heat", kind: "heat.mixing", requested: []string{
		"AFN Zone Mixing Latent Heat Gain Energy",
		"AFN Zone Mixing Latent Heat Gain Rate",
		"AFN Zone Mixing Latent Heat Loss Energy",
		"AFN Zone Mixing Latent Heat Loss Rate",
		"AFN Zone Mixing Sensible Heat Gain Energy",
		"AFN Zone Mixing Sensible Heat Gain Rate",
		"AFN Zone Mixing Sensible Heat Loss Energy",
		"AFN Zone Mixing Sensible Heat Loss Rate",
		"Zone Mixing Latent Heat Gain Energy",
		"Zone Mixing Latent Heat Gain Rate",
		"Zone Mixing Latent Heat Loss Energy",
		"Zone Mixing Latent Heat Loss Rate",
		"Zone Mixing Sensible Heat Gain Energy",
		"Zone Mixing Sensible Heat Gain Rate",
		"Zone Mixing Sensible Heat Loss Energy",
		"Zone Mixing Sensible Heat Loss Rate",
	}, observed: []string{}},
	{level: "heat", kind: "heat.ventilation", requested: []string{
		"AFN Zone Ventilation Latent Heat Gain Energy",
		"AFN Zone Ventilation Latent Heat Gain Rate",
		"AFN Zone Ventilation Latent Heat Loss Energy",
		"AFN Zone Ventilation Latent Heat Loss Rate",
		"AFN Zone Ventilation Sensible Heat Gain Energy",
		"AFN Zone Ventilation Sensible Heat Gain Rate",
		"AFN Zone Ventilation Sensible Heat Loss Energy",
		"AFN Zone Ventilation Sensible Heat Loss Rate",
		"Zone Ventilation Latent Heat Gain Energy",
		"Zone Ventilation Latent Heat Gain Rate",
		"Zone Ventilation Latent Heat Loss Energy",
		"Zone Ventilation Latent Heat Loss Rate",
		"Zone Ventilation Sensible Heat Gain Energy",
		"Zone Ventilation Sensible Heat Gain Rate",
		"Zone Ventilation Sensible Heat Loss Energy",
		"Zone Ventilation Sensible Heat Loss Rate",
	}, observed: []string{}},
	{level: "heat", kind: "heat.storage_air", requested: []string{
		"Zone Air Heat Balance Air Energy Storage Rate",
	}, observed: []string{"Zone Air Heat Balance Air Energy Storage Rate"}},
	{level: "heat", kind: "heat.zone_balance_residual", requested: []string{
		"Zone Air Heat Balance Deviation Rate",
	}, observed: []string{}},
	{level: "heat", kind: "heat.internal_convective", requested: []string{
		"Zone Air Heat Balance Internal Convective Heat Gain Rate",
		"Zone Total Internal Convective Heating Energy",
		"Zone Total Internal Convective Heating Rate",
		"Zone Total Internal Latent Gain Energy",
		"Zone Total Internal Latent Gain Rate",
	}, observed: []string{"Zone Total Internal Convective Heating Energy", "Zone Total Internal Convective Heating Rate", "Zone Total Internal Latent Gain Energy", "Zone Total Internal Latent Gain Rate", "Zone Air Heat Balance Internal Convective Heat Gain Rate"}},
	{level: "heat", kind: "heat.interzone_air", requested: []string{
		"Zone Air Heat Balance Interzone Air Transfer Rate",
	}, observed: []string{"Zone Air Heat Balance Interzone Air Transfer Rate"}},
	{level: "heat", kind: "heat.ventilation_outdoor_air", requested: []string{
		"Zone Air Heat Balance Outdoor Air Transfer Rate",
	}, observed: []string{"Zone Air Heat Balance Outdoor Air Transfer Rate"}},
	{level: "heat", kind: "heat.surface_convection", requested: []string{
		"Zone Air Heat Balance Surface Convection Rate",
	}, observed: []string{"Zone Air Heat Balance Surface Convection Rate"}},
	{level: "heat", kind: "heat.hvac_air_transfer", requested: []string{
		"Zone Air Heat Balance System Air Transfer Rate",
	}, observed: []string{"Zone Air Heat Balance System Air Transfer Rate"}},
	{level: "heat", kind: "heat.system_convective", requested: []string{
		"Zone Air Heat Balance System Convective Heat Gain Rate",
	}, observed: []string{"Zone Air Heat Balance System Convective Heat Gain Rate"}},
	{level: "load", kind: "load.zone_latent_cooling", requested: []string{
		"Zone Air System Latent Cooling Energy",
		"Zone Air System Latent Cooling Rate",
	}, observed: []string{}},
	{level: "load", kind: "load.zone_latent_heating", requested: []string{
		"Zone Air System Latent Heating Energy",
		"Zone Air System Latent Heating Rate",
	}, observed: []string{}},
	{level: "load", kind: "load.zone_cooling", requested: []string{
		"Zone Air System Sensible Cooling Energy",
		"Zone Air System Sensible Cooling Rate",
	}, observed: []string{"Zone Air System Sensible Cooling Energy", "Zone Air System Sensible Cooling Rate"}},
	{level: "load", kind: "load.zone_heating", requested: []string{
		"Zone Air System Sensible Heating Energy",
		"Zone Air System Sensible Heating Rate",
	}, observed: []string{"Zone Air System Sensible Heating Energy", "Zone Air System Sensible Heating Rate"}},
	{level: "load", kind: "load.zone_equipment_heating", requested: []string{
		"Zone Baseboard Total Heating Energy",
		"Zone Baseboard Total Heating Rate",
	}, observed: []string{}},
	{level: "heat", kind: "heat.combined_outdoor_air", requested: []string{
		"Zone Combined Outdoor Air Latent Heat Gain Energy",
		"Zone Combined Outdoor Air Latent Heat Gain Rate",
		"Zone Combined Outdoor Air Latent Heat Loss Energy",
		"Zone Combined Outdoor Air Latent Heat Loss Rate",
		"Zone Combined Outdoor Air Sensible Heat Gain Energy",
		"Zone Combined Outdoor Air Sensible Heat Gain Rate",
		"Zone Combined Outdoor Air Sensible Heat Loss Energy",
		"Zone Combined Outdoor Air Sensible Heat Loss Rate",
	}, observed: []string{}},
	{level: "heat", kind: "heat.equipment", requested: []string{
		"Zone Electric Equipment Convective Heating Energy",
		"Zone Electric Equipment Convective Heating Rate",
		"Zone Electric Equipment Latent Gain Energy",
		"Zone Electric Equipment Latent Gain Rate",
		"Zone Gas Equipment Convective Heating Energy",
		"Zone Gas Equipment Convective Heating Rate",
		"Zone Gas Equipment Latent Gain Energy",
		"Zone Gas Equipment Latent Gain Rate",
		"Zone Hot Water Equipment Convective Heating Energy",
		"Zone Hot Water Equipment Convective Heating Rate",
		"Zone Hot Water Equipment Latent Gain Energy",
		"Zone Hot Water Equipment Latent Gain Rate",
		"Zone Other Equipment Convective Heating Energy",
		"Zone Other Equipment Convective Heating Rate",
		"Zone Other Equipment Latent Gain Energy",
		"Zone Other Equipment Latent Gain Rate",
		"Zone Steam Equipment Convective Heating Energy",
		"Zone Steam Equipment Convective Heating Rate",
		"Zone Steam Equipment Latent Gain Energy",
		"Zone Steam Equipment Latent Gain Rate",
	}, observed: []string{"Zone Electric Equipment Convective Heating Energy", "Zone Electric Equipment Convective Heating Rate", "Zone Electric Equipment Latent Gain Energy", "Zone Electric Equipment Latent Gain Rate"}},
	{level: "heat", kind: "heat.internal_other", requested: []string{
		"Zone IT Equipment Convective Heating Energy",
		"Zone IT Equipment Convective Heating Rate",
		"Zone IT Equipment Latent Gain Energy",
		"Zone IT Equipment Latent Gain Rate",
		"Zone Other Internal Convective Heating Energy",
		"Zone Other Internal Convective Heating Rate",
		"Zone Other Internal Latent Gain Energy",
		"Zone Other Internal Latent Gain Rate",
	}, observed: []string{}},
	{level: "heat", kind: "heat.lighting", requested: []string{
		"Zone Lights Convective Heating Energy",
		"Zone Lights Convective Heating Rate",
	}, observed: []string{"Zone Lights Convective Heating Energy", "Zone Lights Convective Heating Rate"}},
	{level: "heat", kind: "heat.people", requested: []string{
		"Zone People Convective Heating Energy",
		"Zone People Convective Heating Rate",
		"Zone People Latent Gain Energy",
		"Zone People Latent Gain Rate",
	}, observed: []string{"Zone People Convective Heating Energy", "Zone People Convective Heating Rate", "Zone People Latent Gain Energy", "Zone People Latent Gain Rate"}},
	{level: "load", kind: "load.zone_predicted_cooling", requested: []string{
		"Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate",
		"Zone System Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate",
	}, observed: []string{"Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate", "Zone System Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate"}},
	{level: "load", kind: "load.zone_predicted_heating", requested: []string{
		"Zone Predicted Sensible Load to Heating Setpoint Heat Transfer Rate",
		"Zone System Predicted Sensible Load to Heating Setpoint Heat Transfer Rate",
	}, observed: []string{"Zone Predicted Sensible Load to Heating Setpoint Heat Transfer Rate", "Zone System Predicted Sensible Load to Heating Setpoint Heat Transfer Rate"}},
	{level: "load", kind: "load.zone_radiant_cooling", requested: []string{
		"Zone Radiant HVAC Cooling Energy",
		"Zone Radiant HVAC Cooling Rate",
	}, observed: []string{}},
	{level: "load", kind: "load.zone_radiant_heating", requested: []string{
		"Zone Radiant HVAC Heating Energy",
		"Zone Radiant HVAC Heating Rate",
	}, observed: []string{}},
	{level: "heat", kind: "heat.surface_inside_face_convection", requested: []string{
		"Surface Inside Face Convection Heat Gain Energy",
		"Surface Inside Face Convection Heat Gain Rate",
		"Surface Inside Face Convection Heat Transfer Energy",
		"Surface Inside Face Convection Heat Transfer Rate",
	}, observed: []string{"Surface Inside Face Convection Heat Gain Rate", "Surface Inside Face Convection Heat Gain Energy"}},
	{level: "heat", kind: "heat.ventilation_system_oa", requested: []string{
		"Air System Outdoor Air Latent Cooling Energy",
		"Air System Outdoor Air Latent Cooling Rate",
		"Air System Outdoor Air Latent Heating Energy",
		"Air System Outdoor Air Latent Heating Rate",
		"Air System Outdoor Air Sensible Cooling Energy",
		"Air System Outdoor Air Sensible Cooling Rate",
		"Air System Outdoor Air Sensible Heating Energy",
		"Air System Outdoor Air Sensible Heating Rate",
		"Air System Outdoor Air Total Cooling Energy",
		"Air System Outdoor Air Total Cooling Rate",
		"Air System Outdoor Air Total Heating Energy",
		"Air System Outdoor Air Total Heating Rate",
	}, observed: []string{}},
}

func TestEnergyPathRealReportedAvailabilityPrecedesGraphSelection(t *testing.T) {
	series, sources, plan, context := epathRealReportedAvailabilityFixture(t)
	got := buildEnergyExplanationResultWithDriverContext(series, sources, plan, context)
	epathRealAssertReportedAvailability(t, got.Completeness, 12, 18, 9, 14)

	// All reported control/system/plant sources remain provenance, while exactly
	// two authoritative delivered-load nodes are used for the physical graph.
	loads := 0
	for _, node := range got.Nodes {
		if node.Level == "load" {
			loads++
			if node.Kind != "load.zone_cooling" && node.Kind != "load.zone_heating" {
				t.Errorf("context load became a physical node: %#v", node)
			}
		}
	}
	if loads != 2 {
		t.Fatalf("physical load count = %d, want 2", loads)
	}
	sourceNames := map[string]bool{}
	derived := 0
	for _, source := range got.Sources {
		sourceNames[source.Name] = true
		if source.SourceType == "derived_formula" {
			derived++
		}
	}
	for _, group := range epathRealReportedAvailabilityGroups {
		for _, name := range group.observed {
			if !sourceNames[name] {
				t.Errorf("reported source lost: %s", name)
			}
		}
	}
	if derived == 0 {
		t.Fatal("fixture failed to exercise derived driver provenance")
	}

	// The fix changes availability input only. The same runtime builder with its
	// legacy completeness mode has identical graph/source payloads. Frozen v1
	// reconciliation rows/source-ID sets originate from maps, so sort those sets.
	series, sources, legacyPlan, context := epathRealReportedAvailabilityFixture(t)
	legacyPlan.BasicEnergyDetail = ""
	prior := buildEnergyExplanationResultWithDriverContext(series, sources, legacyPlan, context)
	for _, periods := range [][]EnergyPeriod{got.Periods, prior.Periods} {
		for i := range periods {
			periods[i].Reconciliation = append([]EnergyReconciliation(nil), periods[i].Reconciliation...)
			for j := range periods[i].Reconciliation {
				row := &periods[i].Reconciliation[j]
				row.SourceIDs = append([]string(nil), row.SourceIDs...)
				sort.Strings(row.SourceIDs)
			}
			sort.Slice(periods[i].Reconciliation, func(a, b int) bool { return periods[i].Reconciliation[a].ID < periods[i].Reconciliation[b].ID })
		}
	}
	if !reflect.DeepEqual(got.Nodes, prior.Nodes) || !reflect.DeepEqual(got.Edges, prior.Edges) ||
		!reflect.DeepEqual(got.Periods, prior.Periods) || !reflect.DeepEqual(got.Sources, prior.Sources) {
		t.Fatalf("requested-output availability changed graph or provenance: nodes=%t edges=%t periods=%t sources=%t", reflect.DeepEqual(got.Nodes, prior.Nodes), reflect.DeepEqual(got.Edges, prior.Edges), reflect.DeepEqual(got.Periods, prior.Periods), reflect.DeepEqual(got.Sources, prior.Sources))
	}
}

func TestEnergyPathRealReportedAvailabilityExcludesUnrequestedExtras(t *testing.T) {
	series, sources, plan, context := epathRealReportedAvailabilityFixture(t)
	kept := make([]PurposeOutputObject, 0, len(plan.OutputObjects))
	for _, object := range plan.OutputObjects {
		heat, isHeat := energyHeatAliasDefinitionForName(object.VariableName)
		load, isLoad := energyLoadAliasDefinitionForName(object.VariableName)
		if isHeat && heat.Kind == "heat.people" || isLoad && load.Kind == "load.system_cooling" {
			continue
		}
		kept = append(kept, object)
	}
	plan.OutputObjects = kept
	got := buildEnergyExplanationResultWithDriverContext(series, sources, plan, context)
	epathRealAssertReportedAvailability(t, got.Completeness, 11, 17, 8, 13)
	// Removing a request cannot delete already observed source provenance.
	hasPeople, hasCoil := false, false
	for _, source := range got.Sources {
		hasPeople = hasPeople || source.Name == "Zone People Convective Heating Energy"
		hasCoil = hasCoil || source.Name == "Cooling Coil Total Cooling Energy"
	}
	if !hasPeople || !hasCoil {
		t.Fatal("unrequested observed extras must remain source evidence")
	}
}

func TestEnergyPathRealReportedAvailabilityDoesNotCountFiveDerivedGroups(t *testing.T) {
	series, sources, plan, _ := epathRealReportedAvailabilityFixture(t)
	for i := range series {
		series[i] = canonicalEnergyExplanationSeries(series[i])
	}
	graphSeries := selectCanonicalEnergyExplanationLoads(series)
	// These are the five derived (not requested/reported) group kinds observed
	// in the full real LargeOffice build. The old graph-series count was 17/18.
	for _, kind := range []string{"heat.outdoor_air_unsplit", "heat.ventilation_fallback", "heat.surface_reconciliation", "heat.internal_other_reconciliation", "heat.unmapped_zone_balance"} {
		graphSeries = append(graphSeries, energyExplanationSeries{Level: "heat", Kind: kind})
	}
	old := buildEnergyExplanationCompleteness(graphSeries, sources, plan, 0)
	epathRealAssertReportedAvailability(t, old, 17, 18, 2, 14)
	reported := energyExplanationRequestedThermalAvailability(series, plan)
	got := buildEnergyExplanationCompleteness(reported, sources, plan, 0)
	epathRealAssertReportedAvailability(t, got, 12, 18, 9, 14)
	// Group identity capture must not retain or edit numeric observations/maps.
	for _, group := range reported {
		if group.Monthly != nil || group.Total != 0 || group.SourceIDs != nil {
			t.Fatalf("availability capture retained numeric/provenance payload: %#v", group)
		}
	}
}

func epathRealAssertReportedAvailability(t *testing.T, got EnergyCompleteness, driversFound, driversTotal, loadsFound, loadsTotal int) {
	t.Helper()
	if got.HeatDrivers.Found != driversFound || got.HeatDrivers.Total != driversTotal ||
		got.DeliveredLoad.Found != loadsFound || got.DeliveredLoad.Total != loadsTotal {
		t.Fatalf("reported availability Drivers %d/%d, Loads %d/%d; want %d/%d, %d/%d",
			got.HeatDrivers.Found, got.HeatDrivers.Total, got.DeliveredLoad.Found, got.DeliveredLoad.Total,
			driversFound, driversTotal, loadsFound, loadsTotal)
	}
}

func epathRealReportedAvailabilityFixture(t *testing.T) ([]energyExplanationSeries, []EnergyDataSource, *PurposeRunPlan, energyDriverBuildContext) {
	t.Helper()
	doc, err := idf.Parse(`Version,25.1;
Zone,Office,0,0,0,0,1,1;
Material:NoMass,Insulation,Rough,2; Construction,Wall Construction,Insulation;
BuildingSurface:Detailed,Office Wall,Wall,Wall Construction,Office,,Outdoors,,SunExposed,WindExposed,0.5,4,
0,0,0,0,0,3,4,0,3,4,0,0;`)
	if err != nil {
		t.Fatal(err)
	}
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
	series := []energyExplanationSeries{}
	sources := []EnergyDataSource{}
	for _, group := range epathRealReportedAvailabilityGroups {
		for _, name := range group.requested {
			plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{
				ObjectType: "Output:Variable", KeyValue: "*", VariableName: name,
				ReportingFrequency: "Monthly", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy},
			})
			kind := ""
			if group.level == "heat" {
				if def, ok := energyHeatAliasDefinitionForName(name); ok {
					kind = def.Kind
				}
			} else if def, ok := energyLoadAliasDefinitionForName(name); ok {
				kind = def.Kind
			}
			if kind != group.kind {
				t.Fatalf("reviewed requested group %s name %q classified as %s", group.kind, name, kind)
			}
		}
		for _, name := range group.observed {
			id := fmt.Sprintf("reported-%d", len(sources))
			key, sourceUnit := "Office", "J"
			if strings.HasSuffix(name, "Rate") {
				sourceUnit = "W"
			}
			item := energyExplanationSeries{
				Level: group.level, Kind: group.kind, Label: name, Unit: "kWh", ZoneName: "Office",
				SourceName: name, sourceName: name, sourceKeyValue: key, sourceFrequency: "Monthly",
				sourceIsRate: sourceUnit == "W", SourceIDs: []string{id},
				Total: 120, Monthly: map[int]float64{1: 10, 2: 10, 3: 10, 4: 10, 5: 10, 6: 10, 7: 10, 8: 10, 9: 10, 10: 10, 11: 10, 12: 10},
			}
			if group.level == "heat" {
				def, ok := energyHeatAliasDefinitionForName(name)
				if !ok || def.Kind != group.kind {
					t.Fatalf("observed heat group mismatch: %s", name)
				}
				item.HeatCategory, item.SurfaceScoped = def.HeatCategory, def.SurfaceScoped
				if item.SurfaceScoped {
					key, item.sourceKeyValue = "Office Wall", "Office Wall"
				}
			} else {
				def, ok := energyLoadAliasDefinitionForName(name)
				if !ok || def.Kind != group.kind {
					t.Fatalf("observed load group mismatch: %s", name)
				}
				item.ServiceKind, item.PathType = def.ServiceKind, def.Scope
				if def.Scope != "zone" {
					item.ZoneName = ""
					item.LoopName = key
				}
			}
			// Explicit reported zero is found, not missing.
			if group.kind == "heat.interzone_air" || group.kind == "heat.system_convective" {
				item.Total = 0
				for month := range item.Monthly {
					item.Monthly[month] = 0
				}
			}
			sources = append(sources, EnergyDataSource{
				ID: id, SourceType: "output_variable", Name: name, KeyValue: key, Units: sourceUnit,
				NormalizedUnit: "kWh", ReportingFrequency: "Monthly", ZoneName: item.ZoneName,
			})
			series = append(series, item)
		}
	}
	return series, sources, plan, context
}
