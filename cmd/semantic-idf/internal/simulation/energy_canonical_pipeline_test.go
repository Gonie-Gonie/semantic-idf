package simulation

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const energyMultiplierFixtureIDF = `
Version, 24.1;

Zone,
  Office,
  0,
  0,
  0,
  0,
  1,
  2;

ZoneList,
  Office Group Zones,
  OFFICE;

ZoneGroup,
  Repeated Offices,
  office group zones,
  5;

Space,
  Office Space,
  office;

ZoneHVAC:EquipmentConnections,
  Office,
  Office Equipment List,
  Office Inlet,
  ,
  Office Air Node,
  Office Return;

ZoneHVAC:EquipmentList,
  Office Equipment List,
  SequentialLoad,
  ZoneHVAC:IdealLoadsAirSystem,
  Office Ideal Loads,
  1,
  1;

ZoneHVAC:IdealLoadsAirSystem,
  Office Ideal Loads;
`

func TestEPATH050CanonicalSQLBoundaryIsGraphFreeAndExplicit(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyPathScopeFixtureIDF)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	createEnergyPathContractSQL(t, path)

	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Series) == 0 || len(parsed.Sources) == 0 {
		t.Fatalf("canonical parse result = %#v", parsed)
	}
	stages := map[string]bool{}
	foundSurface := false
	foundLoad := false
	for _, item := range parsed.Series {
		stages[item.Stage] = true
		if item.Stage == "" || item.CanonicalKind == "" || item.SourceClass == "" || item.SourceFamily == "" || item.CanonicalFamily == "" || item.PeriodBasis == "" || item.SourceName == "" {
			t.Errorf("incomplete canonical identity: %#v", item)
		}
		if item.Stage == "driver" && item.SurfaceName == "Office Wall" {
			foundSurface = item.ThermalComponent != "" && item.Sign != "" && item.SourceKey == "Office Wall" && len(item.Monthly) == 1
		}
		if item.Stage == "load" && item.ServiceKind == "cooling" && item.SourceClass == "zone_air_system" {
			foundLoad = true
		}
	}
	if !foundSurface || !foundLoad || !stages["carrier"] || !stages["end_use"] || !stages["driver"] || !stages["load"] {
		t.Fatalf("canonical identities: surface=%t load=%t stages=%#v", foundSurface, foundLoad, stages)
	}
}

func TestEPATH050TabularAnnualCreatesOnlyCarrierAndEndUseSeries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	createTestEnergyTabularSQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO TabularDataWithStrings VALUES
		('AnnualBuildingUtilityPerformanceSummary', 'Entire Facility', 'End Uses', 'Generators', 'Electricity', 'GJ', 5, 1, '0.0020')`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}, energyDriverBuildContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Series) == 0 {
		t.Fatal("tabular annual fixture produced no canonical series")
	}
	for _, item := range parsed.Series {
		if item.Stage != "carrier" && item.Stage != "end_use" {
			t.Fatalf("tabular annual leaked into %q stage: %#v", item.Stage, item)
		}
		if item.PeriodBasis != "annual_only" || len(item.Monthly) != 0 || len(item.RawMonthly) != 0 {
			t.Fatalf("tabular annual fabricated period values: %#v", item)
		}
	}
	for _, source := range parsed.Sources {
		if strings.Contains(strings.ToLower(source.Name), "generator") || strings.Contains(strings.ToLower(source.KeyValue), "generator") {
			t.Fatalf("tabular production source crossed the canonical boundary: %#v", source)
		}
	}
}

func TestEPATH050SyntheticCanonicalBuilderOwnsGraphDirection(t *testing.T) {
	series := []energyExplanationSeries{
		{Stage: "driver", CanonicalKind: "heat.infiltration", Kind: "heat.infiltration", Label: "Infiltration", Unit: "kWh", ZoneName: "Office", DriverCategory: energyDriverCategoryInfiltration, Sign: "positive", HeatSign: "positive", SourceClass: "fixture_driver", SourceIDs: []string{"driver"}, Total: 10},
		{Stage: "load", CanonicalKind: "load.zone_cooling", Kind: "load.zone_cooling", Label: "Cooling load", Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", PathType: "zone", SourceClass: "fixture_load", SourceIDs: []string{"load"}, Total: 10},
		{Stage: "end_use", CanonicalKind: "energy.cooling", Kind: "energy.cooling", Label: "Cooling", Unit: "kWh", Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", SourceClass: "meter", SourceIDs: []string{"end-use"}, Total: 4},
		{Stage: "carrier", CanonicalKind: "energy.electricity.total", Kind: "energy.electricity.total", Label: "Electricity", Unit: "kWh", Carrier: "electricity", MeterHierarchyLevel: "facility_total", SourceClass: "meter", SourceIDs: []string{"carrier"}, Total: 5},
	}
	graph := buildEnergyExplanationGraphForPeriod("annual", series, PurposeAllocationPolicyDirectOnly, func(item energyExplanationSeries) float64 { return item.Total })
	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{Schema: energyExplanationV1Schema, Nodes: graph.Nodes, Edges: graph.Edges})
	for _, pair := range [][2]string{
		{"driver.air.infiltration.cooling.building", "load.cooling.building"},
		{"load.cooling.building", "end_use.cooling.building"},
		{"end_use.cooling.building", "carrier.electricity.building"},
	} {
		if energyPathV2LinkByIDs(result.Links, pair[0], pair[1]) == nil {
			t.Fatalf("missing canonical direction %s -> %s in %#v", pair[0], pair[1], result.Links)
		}
		if energyPathV2LinkByIDs(result.Links, pair[1], pair[0]) != nil {
			t.Fatalf("reverse link leaked: %s -> %s", pair[1], pair[0])
		}
	}
}

func TestEPATH051EffectiveMultiplierIndexAndOfficialSourceSemantics(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyMultiplierFixtureIDF)
	index := buildEnergyEffectiveMultiplierIndex(doc)
	for _, key := range []string{"office", "OFFICE SPACE", "office ideal loads"} {
		record, ok := index.resolve(key)
		if !ok || record.ZoneName != "Office" || record.ZoneMultiplier != 2 || record.GroupMultiplier != 5 || record.effectiveMultiplier() != 10 {
			t.Fatalf("multiplier index %q = %#v, %t", key, record, ok)
		}
	}

	tests := []struct {
		name        string
		stage       string
		level       string
		pathType    string
		zone        string
		application string
		factor      float64
	}{
		{name: "Zone Air System Sensible Heating Energy", stage: "load", level: "load", pathType: "zone", zone: "OFFICE SPACE", application: energyMultiplierRequiresZone, factor: 10},
		{name: "Zone Air System Sensible Cooling Energy", stage: "load", level: "load", pathType: "zone", zone: "Office", application: energyMultiplierRequiresZone, factor: 10},
		{name: "Zone Predicted Sensible Load to Heating Setpoint Heat Transfer Rate", stage: "load", level: "load", pathType: "zone", zone: "Office", application: energyMultiplierRequiresZone, factor: 10},
		{name: "Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate", stage: "load", level: "load", pathType: "zone", zone: "Office", application: energyMultiplierRequiresZone, factor: 10},
		{name: "Zone Air Heat Balance Surface Convection Rate", stage: "driver", level: "heat", zone: "Office", application: energyMultiplierRequiresZone, factor: 10},
		{name: "Zone System Predicted Sensible Load to Heating Setpoint Heat Transfer Rate", stage: "load", level: "load", pathType: "zone", zone: "Office", application: energyMultiplierAlreadyModelTotal, factor: 1},
		{name: "Zone System Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate", stage: "load", level: "load", pathType: "zone", zone: "Office", application: energyMultiplierAlreadyModelTotal, factor: 1},
		{name: "Zone Ideal Loads Zone Sensible Heating Energy", stage: "load", level: "load", pathType: "zone", zone: "Office Ideal Loads", application: energyMultiplierAlreadyModelTotal, factor: 1},
		{name: "Zone List Sensible Heating Energy", stage: "load", level: "load", pathType: "zone", zone: "Office Group Zones", application: energyMultiplierRequiresGroup, factor: 5},
		{name: "Electricity:Facility", stage: "carrier", level: "energy", application: energyMultiplierAlreadyModelTotal, factor: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := canonicalEnergyExplanationSeries(energyExplanationSeries{
				Stage: test.stage, Level: test.level, Kind: "fixture", CanonicalKind: "fixture", PathType: test.pathType,
				ZoneName: test.zone, SourceName: test.name, sourceName: test.name, SourceIDs: []string{"source"}, Total: 10, Monthly: map[int]float64{1: 10},
			})
			got, gotSources, warnings := applyEnergyExplanationMultipliers([]energyExplanationSeries{item}, []EnergyDataSource{{ID: "source", Name: test.name}}, index)
			if len(got) != 1 || got[0].MultiplierApplication != test.application || got[0].EffectiveMultiplier != test.factor || got[0].Total != 10*test.factor || got[0].Monthly[1] != 10*test.factor {
				t.Fatalf("multiplier result = %#v", got)
			}
			if len(gotSources) != 1 || gotSources[0].RawValue != 10 || gotSources[0].EffectiveValue != 10*test.factor || gotSources[0].EffectiveMultiplier != test.factor || gotSources[0].MultiplierApplication != test.application {
				t.Fatalf("source accounting = %#v", gotSources)
			}
			if strings.Contains(test.name, "Ideal Loads") && (got[0].ZoneName != "Office" || gotSources[0].ZoneName != "Office") {
				t.Fatalf("Ideal Loads ownership = series %#v / source %#v", got[0], gotSources[0])
			}
			if len(warnings) != 0 {
				t.Fatalf("unexpected warnings = %#v", warnings)
			}
			twice, twiceSources, twiceWarnings := applyEnergyExplanationMultipliers(got, gotSources, index)
			if len(twice) != 1 || twice[0].Total != got[0].Total || twice[0].Monthly[1] != got[0].Monthly[1] || twice[0].EffectiveMultiplier != got[0].EffectiveMultiplier {
				t.Fatalf("multiplier was applied more than once: first %#v / second %#v", got, twice)
			}
			if len(twiceSources) != 1 || twiceSources[0].EffectiveValue != gotSources[0].EffectiveValue || len(twiceWarnings) != 0 {
				t.Fatalf("source multiplier was applied more than once: first %#v / second %#v / warnings %#v", gotSources, twiceSources, twiceWarnings)
			}
		})
	}
}

func TestEPATH051BuildingAndZoneInspectorKeepRawEffectiveAndApplication(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyMultiplierFixtureIDF)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	series := []energyExplanationSeries{{
		Level: "load", Kind: "load.zone_cooling", Label: "Cooling", Unit: "kWh", ServiceKind: "cooling", PathType: "zone", ZoneName: "Office Space",
		SourceIDs: []string{"load"}, sourceName: "Zone Air System Sensible Cooling Energy", SourceName: "Zone Air System Sensible Cooling Energy", sourceFrequency: "Monthly", Total: 10, Monthly: map[int]float64{1: 10},
	}}
	legacy := buildEnergyExplanationResultWithDriverContext(series, []EnergyDataSource{{ID: "load", Name: "Zone Air System Sensible Cooling Energy"}}, &PurposeRunPlan{}, context)
	result := UpgradeEnergyExplanationV1(legacy)
	node := energyPathV2NodeByID(result.Nodes, "load.cooling.building")
	if node == nil || node.Value != 100 || node.RawValue != 10 || node.EffectiveValue != 100 || node.AllocatedValue != 100 || node.Multiplier != 10 {
		t.Fatalf("building effective node = %#v", node)
	}
	source := energyExplanationSourceByID(result.Sources, "load")
	if source == nil || source.RawValue != 10 || source.EffectiveValue != 100 || source.EffectiveMultiplier != 10 || source.MultiplierApplication != energyMultiplierRequiresZone {
		t.Fatalf("building source detail = %#v", source)
	}
	if len(source.ScopeDetails) != 1 || source.ScopeDetails[0].Scope.ZoneName != "Office" || source.ScopeDetails[0].RawValue != 10 || source.ScopeDetails[0].EffectiveValue != 100 || source.ScopeDetails[0].EffectiveMultiplier != 10 || source.ScopeDetails[0].MultiplierApplication != energyMultiplierRequiresZone {
		t.Fatalf("zone inspector detail = %#v", source.ScopeDetails)
	}
}

func TestEPATH051SurfaceUsesZoneGroupButNeverGeometrySurfaceMultiplier(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyMultiplierFixtureIDF)
	geometry := idf.GeometryReport{Surfaces: []idf.GeometrySurface{{
		ID:                "surface.office-wall",
		Name:              "Office Wall",
		Type:              "BuildingSurface:Detailed",
		SurfaceType:       "Wall",
		ZoneName:          "Office",
		OutsideBoundary:   "Outdoors",
		ZoneMultiplier:    2,
		SurfaceMultiplier: 3,
	}}}
	context := newEnergyDriverBuildContext(geometry, doc)
	category, warning := context.SurfaceCategories.resolve("Office Wall")
	if warning != nil || category.EffectiveMultiplier != 6 {
		t.Fatalf("surface fixture did not carry its geometry multiplier: category %#v / warning %#v", category, warning)
	}
	item := canonicalEnergyExplanationSeries(energyExplanationSeries{
		Stage: "driver", Level: "heat", Kind: "heat.surface_inside_face_convection", CanonicalKind: "heat.surface_inside_face_convection",
		ZoneName: "Office", SurfaceName: "Office Wall", SourceName: "Surface Inside Face Convection Heat Gain Energy", sourceName: "Surface Inside Face Convection Heat Gain Energy",
		SourceIDs: []string{"surface"}, Total: 10, Monthly: map[int]float64{1: 10},
	})
	got, gotSources, warnings := applyEnergyExplanationMultipliers([]energyExplanationSeries{item}, []EnergyDataSource{{ID: "surface", Name: item.SourceName, ZoneName: "Office"}}, context.Multipliers)
	if len(got) != 1 || got[0].EffectiveMultiplier != 10 || got[0].Total != 100 || got[0].Monthly[1] != 100 {
		t.Fatalf("surface value reused the geometry surface multiplier; want raw 10 x zone/group 10 = 100, got %#v", got)
	}
	if len(gotSources) != 1 || gotSources[0].RawValue != 10 || gotSources[0].EffectiveValue != 100 || gotSources[0].EffectiveMultiplier != 10 || gotSources[0].MultiplierApplication != energyMultiplierRequiresZone || len(warnings) != 0 {
		t.Fatalf("surface source multiplier accounting = %#v / warnings %#v", gotSources, warnings)
	}
}

func TestEPATH051UnknownMultiplierIsConservativeButMetersRemainKnown(t *testing.T) {
	series := []energyExplanationSeries{
		canonicalEnergyExplanationSeries(energyExplanationSeries{Stage: "load", Level: "load", Kind: "load.zone_cooling", ZoneName: "Missing", SourceName: "Mystery Zone Load", SourceIDs: []string{"load"}, Total: 10}),
		canonicalEnergyExplanationSeries(energyExplanationSeries{Stage: "carrier", Level: "energy", Kind: "energy.electricity.total", Carrier: "electricity", SourceName: "Electricity:Facility", SourceIDs: []string{"meter"}, Total: 10}),
	}
	got, _, warnings := applyEnergyExplanationMultipliers(series, []EnergyDataSource{{ID: "load"}, {ID: "meter"}}, energyEffectiveMultiplierIndex{})
	if got[0].MultiplierApplication != energyMultiplierUnknown || got[0].EffectiveMultiplier != 1 || got[0].Total != 10 || len(warnings) != 1 || warnings[0].Code != "energy_multiplier_unknown" {
		t.Fatalf("unknown multiplier fallback = %#v / %#v", got[0], warnings)
	}
	if got[1].MultiplierApplication != energyMultiplierAlreadyModelTotal || got[1].EffectiveMultiplier != 1 || got[1].Total != 10 {
		t.Fatalf("known meter semantics = %#v", got[1])
	}
}

func TestEPATH052AnnualPreservesSeasonalSignedContributions(t *testing.T) {
	series := []energyExplanationSeries{
		{Level: "heat", Kind: "heat.infiltration", Label: "Infiltration", Unit: "kWh", ZoneName: "Office", ThermalComponent: "sensible", SourceIDs: []string{"driver"}, sourceName: "Zone Infiltration Sensible Heat Gain Energy", sourceFrequency: "Monthly", Monthly: map[int]float64{1: -10, 7: 10}},
		{Level: "load", Kind: "load.zone_heating", Label: "Heating", Unit: "kWh", ZoneName: "Office", ServiceKind: "heating", PathType: "zone", SourceIDs: []string{"heating-load"}, sourceName: "Zone Air System Sensible Heating Energy", sourceFrequency: "Monthly", Monthly: map[int]float64{1: 10, 7: 0}},
		{Level: "load", Kind: "load.zone_cooling", Label: "Cooling", Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", PathType: "zone", SourceIDs: []string{"cooling-load"}, sourceName: "Zone Air System Sensible Cooling Energy", sourceFrequency: "Monthly", Monthly: map[int]float64{1: 0, 7: 10}},
	}
	sources := []EnergyDataSource{{ID: "driver"}, {ID: "heating-load"}, {ID: "cooling-load"}}
	context := energyDriverBuildContext{Enabled: true, Multipliers: unitEnergyMultiplierIndex("Office")}
	legacy := buildEnergyExplanationResultWithDriverContext(series, sources, &PurposeRunPlan{}, context)
	result := UpgradeEnergyExplanationV1(legacy)

	heatingID := "driver.air.infiltration.heating.building"
	coolingID := "driver.air.infiltration.cooling.building"
	heating := energyPathV2NodeByID(result.Nodes, heatingID)
	cooling := energyPathV2NodeByID(result.Nodes, coolingID)
	if heating == nil || heating.Value != 10 || cooling == nil || cooling.Value != 10 {
		t.Fatalf("annual seasonal drivers = heating %#v / cooling %#v", heating, cooling)
	}
	if got := sumEnergyPathPeriodNodeValues(result.Periods, heatingID); got != heating.Value {
		t.Fatalf("annual heating %v != monthly graph sum %v", heating.Value, got)
	}
	if got := sumEnergyPathPeriodNodeValues(result.Periods, coolingID); got != cooling.Value {
		t.Fatalf("annual cooling %v != monthly graph sum %v", cooling.Value, got)
	}
	for _, pair := range [][2]string{{heatingID, "load.heating.building"}, {coolingID, "load.cooling.building"}} {
		annualLink := energyPathV2LinkByIDs(result.Links, pair[0], pair[1])
		if annualLink == nil || annualLink.FromValue != sumEnergyPathPeriodLinkValues(result.Periods, pair[0], pair[1]) {
			t.Fatalf("annual link %v does not equal monthly contributions: %#v", pair, annualLink)
		}
	}
}

func TestEPATH052AnnualSumsMonthlyAllocationLinks(t *testing.T) {
	monthly := func(first float64, second float64) map[int]float64 { return map[int]float64{1: first, 2: second} }
	series := []energyExplanationSeries{
		{Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Unit: "kWh", Carrier: "electricity", MeterHierarchyLevel: "facility_total", SourceIDs: []string{"facility"}, sourceName: "Electricity:Facility", sourceFrequency: "Monthly", Monthly: monthly(100, 300)},
		{Level: "energy", Kind: "energy.cooling", Label: "Cooling", Unit: "kWh", Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", SourceIDs: []string{"end-use"}, sourceName: "Cooling:Electricity", sourceFrequency: "Monthly", Monthly: monthly(100, 300)},
		{Level: "load", Kind: "load.zone_cooling", Label: "Office cooling", Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", PathType: "zone", SourceIDs: []string{"office-load"}, sourceName: "Zone Air System Sensible Cooling Energy", sourceFrequency: "Monthly", Monthly: monthly(90, 10)},
		{Level: "load", Kind: "load.zone_cooling", Label: "Lab cooling", Unit: "kWh", ZoneName: "Lab", ServiceKind: "cooling", PathType: "zone", SourceIDs: []string{"lab-load"}, sourceName: "Zone Air System Sensible Cooling Energy", sourceFrequency: "Monthly", Monthly: monthly(10, 90)},
	}
	context := energyDriverBuildContext{Enabled: true, Multipliers: unitEnergyMultiplierIndex("Office", "Lab")}
	legacy := buildEnergyExplanationResultWithDriverContext(series, []EnergyDataSource{{ID: "facility"}, {ID: "end-use"}, {ID: "office-load"}, {ID: "lab-load"}}, &PurposeRunPlan{AllocationPolicy: PurposeAllocationPolicyByZoneLoadShare}, context)
	annualOffice := energyExplanationEdgeByIDs(legacy.Edges, "energy.end_use.cooling.electricity", "load.cooling.office")
	annualLab := energyExplanationEdgeByIDs(legacy.Edges, "energy.end_use.cooling.electricity", "load.cooling.lab")
	if annualOffice == nil || annualOffice.Value != 120 || annualLab == nil || annualLab.Value != 280 {
		t.Fatalf("annual allocation links = office %#v / lab %#v", annualOffice, annualLab)
	}
	for _, pair := range [][2]string{{"energy.end_use.cooling.electricity", "load.cooling.office"}, {"energy.end_use.cooling.electricity", "load.cooling.lab"}} {
		annual := energyExplanationEdgeByIDs(legacy.Edges, pair[0], pair[1])
		sum := 0.0
		count := 0
		for _, period := range legacy.Periods {
			if period.Kind != "monthly" {
				continue
			}
			if edge := energyExplanationEdgeByIDs(period.Edges, pair[0], pair[1]); edge != nil {
				sum += edge.Value
				count++
			}
		}
		if annual == nil || annual.Value != sum || count != 2 || strings.Contains(strings.ToLower(annual.ID), "m1") || strings.Contains(strings.ToLower(annual.ID), "m2") {
			t.Fatalf("annual edge %v = %#v, monthly sum=%v count=%d", pair, annual, sum, count)
		}
	}

	result := UpgradeEnergyExplanationV1(legacy)
	rootLink := energyPathV2LinkByIDs(result.Links, "load.cooling.building", "end_use.cooling.building")
	if rootLink == nil || rootLink.FromValue != 200 || rootLink.ToValue != 400 {
		t.Fatalf("v2 annual root conversion = %#v", rootLink)
	}
	if rootLink.FromValue != sumEnergyPathPeriodLinkValues(result.Periods, rootLink.FromID, rootLink.ToID) || rootLink.ToValue != sumEnergyPathPeriodLinkToValues(result.Periods, rootLink.FromID, rootLink.ToID) {
		t.Fatalf("v2 root link is not the sum of monthly links: %#v", rootLink)
	}
	var office *EnergyExplanationZoneResult
	for index := range result.ZoneResults {
		if strings.EqualFold(result.ZoneResults[index].Scope.ZoneName, "Office") {
			office = &result.ZoneResults[index]
			break
		}
	}
	if office == nil {
		t.Fatalf("missing Office projection: %#v", result.ZoneResults)
	}
	officeEndUse := energyPathV2NodeByID(office.Nodes, "end_use.cooling.office")
	officeCarrier := energyPathV2NodeByID(office.Nodes, "carrier.electricity.office")
	officeLink := energyPathV2LinkByIDs(office.Links, "load.cooling.office", "end_use.cooling.office")
	if officeEndUse == nil || officeEndUse.Value != 120 || officeCarrier == nil || officeCarrier.Value != 120 || officeLink == nil || officeLink.FromValue != 100 || officeLink.ToValue != 120 {
		t.Fatalf("Office annual projection = end use %#v / carrier %#v / link %#v", officeEndUse, officeCarrier, officeLink)
	}
	if officeEndUse.Value != sumEnergyPathPeriodNodeValues(office.Periods, officeEndUse.ID) || officeCarrier.Value != sumEnergyPathPeriodNodeValues(office.Periods, officeCarrier.ID) || officeLink.FromValue != sumEnergyPathPeriodLinkValues(office.Periods, officeLink.FromID, officeLink.ToID) || officeLink.ToValue != sumEnergyPathPeriodLinkToValues(office.Periods, officeLink.FromID, officeLink.ToID) {
		t.Fatalf("Office annual graph is not the sum of scoped months: nodes=%#v link=%#v", office.Nodes, officeLink)
	}
	endUseSource := energyExplanationSourceByID(result.Sources, "end-use")
	if endUseSource == nil {
		t.Fatal("missing end-use source dictionary entry")
	}
	foundOfficeDetail := false
	for _, detail := range endUseSource.ScopeDetails {
		if strings.EqualFold(detail.Scope.ZoneName, "Office") {
			foundOfficeDetail = detail.RawValue == 400 && detail.EffectiveValue == 400 && detail.AllocatedValue == 120 && detail.AllocationFactor == 0.3 && detail.MultiplierApplication == energyMultiplierAlreadyModelTotal
		}
	}
	if !foundOfficeDetail {
		t.Fatalf("Office source accounting does not match annual node contribution: %#v", endUseSource.ScopeDetails)
	}
}

func TestEPATH052ServicePathAnnualAllocationAndSourceDetailsSumMonths(t *testing.T) {
	monthly := func(first float64, second float64) map[int]float64 { return map[int]float64{1: first, 2: second} }
	series := []energyExplanationSeries{
		{Level: "energy", Kind: "energy.electricity.total", Label: "Electricity", Unit: "kWh", Carrier: "electricity", MeterHierarchyLevel: "facility_total", SourceIDs: []string{"facility"}, sourceName: "Electricity:Facility", sourceFrequency: "Monthly", Monthly: monthly(100, 300)},
		{Level: "energy", Kind: "energy.cooling", Label: "Cooling", Unit: "kWh", Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", SourceIDs: []string{"end-use"}, sourceName: "Cooling:Electricity", sourceFrequency: "Monthly", Monthly: monthly(100, 300)},
		{Level: "load", Kind: "load.zone_cooling", Label: "Office cooling", Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", PathType: "zone", SourceIDs: []string{"office-load"}, sourceName: "Zone Air System Sensible Cooling Energy", sourceFrequency: "Monthly", Monthly: monthly(90, 10)},
		{Level: "load", Kind: "load.zone_cooling", Label: "Lab cooling", Unit: "kWh", ZoneName: "Lab", ServiceKind: "cooling", PathType: "zone", SourceIDs: []string{"lab-load"}, sourceName: "Zone Air System Sensible Cooling Energy", sourceFrequency: "Monthly", Monthly: monthly(10, 90)},
	}
	context := energyDriverBuildContext{Enabled: true, Multipliers: unitEnergyMultiplierIndex("Office", "Lab")}
	legacy := buildEnergyExplanationResultWithDriverContext(series, []EnergyDataSource{{ID: "facility"}, {ID: "end-use"}, {ID: "office-load"}, {ID: "lab-load"}}, &PurposeRunPlan{AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare}, context)
	addPaths := func(nodes []EnergyExplanationNode) {
		for index := range nodes {
			switch nodes[index].ID {
			case "load.cooling.office":
				nodes[index].RelatedPathIDs = []string{"path.office.cooling"}
			case "load.cooling.lab":
				nodes[index].RelatedPathIDs = []string{"path.lab.cooling"}
			}
		}
	}
	addPaths(legacy.Nodes)
	for index := range legacy.Periods {
		addPaths(legacy.Periods[index].Nodes)
	}
	legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(legacy)
	officeEdge := energyExplanationEdgeByIDs(legacy.Edges, "energy.end_use.cooling.electricity", "load.cooling.office")
	labEdge := energyExplanationEdgeByIDs(legacy.Edges, "energy.end_use.cooling.electricity", "load.cooling.lab")
	if officeEdge == nil || officeEdge.RuleID != energyRelationshipRuleAllocatedServicePathLoad || officeEdge.Value != 120 || !stringSliceContains(officeEdge.RelatedPathIDs, "path.office.cooling") || labEdge == nil || labEdge.Value != 280 {
		t.Fatalf("service-path annual allocation did not sum months: office %#v / lab %#v", officeEdge, labEdge)
	}
	if !strings.Contains(officeEdge.Formula, "annual sum of monthly service path allocations") || strings.Contains(officeEdge.Formula, "zone load share 0.900000") {
		t.Fatalf("service-path annual formula retained a single-month share: %q", officeEdge.Formula)
	}
	for _, edge := range []*EnergyExplanationEdge{officeEdge, labEdge} {
		if edge.Value != sumEnergyExplanationPeriodEdgeValues(legacy.Periods, edge.FromID, edge.ToID) {
			t.Fatalf("service-path annual edge %#v does not equal its monthly contributions", edge)
		}
	}

	result := UpgradeEnergyExplanationV1(legacy)
	var office *EnergyExplanationZoneResult
	for index := range result.ZoneResults {
		if strings.EqualFold(result.ZoneResults[index].Scope.ZoneName, "Office") {
			office = &result.ZoneResults[index]
			break
		}
	}
	if office == nil {
		t.Fatalf("missing Office service-path projection: %#v", result.ZoneResults)
	}
	endUse := energyPathV2NodeByID(office.Nodes, "end_use.cooling.office")
	link := energyPathV2LinkByIDs(office.Links, "load.cooling.office", "end_use.cooling.office")
	if endUse == nil || endUse.Value != 120 || link == nil || link.FromValue != 100 || link.ToValue != 120 || !stringSliceContains(link.RelatedPathIDs, "path.office.cooling") {
		t.Fatalf("service-path Office annual graph = end use %#v / link %#v", endUse, link)
	}
	if endUse.Value != sumEnergyPathPeriodNodeValues(office.Periods, endUse.ID) || link.ToValue != sumEnergyPathPeriodLinkToValues(office.Periods, link.FromID, link.ToID) {
		t.Fatalf("service-path Office annual graph is not the sum of months: node %#v / link %#v", endUse, link)
	}
	source := energyExplanationSourceByID(result.Sources, "end-use")
	if source == nil {
		t.Fatal("missing service-path end-use source")
	}
	foundOffice := false
	for _, detail := range source.ScopeDetails {
		if strings.EqualFold(detail.Scope.ZoneName, "Office") {
			foundOffice = detail.RawValue == 400 && detail.EffectiveValue == 400 && detail.AllocationFactor == 0.3 && detail.AllocatedValue == 120
		}
	}
	if !foundOffice {
		t.Fatalf("service-path source detail does not match the final annual Zone contribution: %#v", source.ScopeDetails)
	}
}

func TestEPATH052SelectionIsFixedByPhysicalFamilyAndEnergyPriority(t *testing.T) {
	energy := canonicalEnergyExplanationSeries(energyExplanationSeries{
		Level: "load", Kind: "load.zone_cooling", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"energy"}, sourceName: "Zone Air System Sensible Cooling Energy", sourceFrequency: "Monthly", Monthly: map[int]float64{1: 0, 2: 2},
	})
	rate := canonicalEnergyExplanationSeries(energyExplanationSeries{
		Level: "load", Kind: "load.zone_cooling", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"rate"}, sourceName: "Zone Air System Sensible Cooling Rate", sourceFrequency: "Monthly", sourceIsRate: true, Monthly: map[int]float64{1: 9, 2: 9},
	})
	selected := preferredEnergyExplanationSeries([]energyExplanationSeries{rate, energy})
	if len(selected) != 1 || selected[0].Monthly[1] != 0 || selected[0].Monthly[2] != 2 || !stringSliceContains(selected[0].MonthlySourceIDs, "energy") || stringSliceContains(selected[0].MonthlySourceIDs, "rate") {
		t.Fatalf("Energy source switched to Rate for a zero month: %#v", selected)
	}

	annualZoneAir := canonicalEnergyExplanationSeries(energyExplanationSeries{
		Level: "load", Kind: "load.zone_heating", ZoneName: "Office", ServiceKind: "heating", SourceIDs: []string{"zone-air"}, sourceName: "Zone Air System Sensible Heating Energy", sourceFrequency: "Annual", Total: 100,
	})
	monthlyIdeal := canonicalEnergyExplanationSeries(energyExplanationSeries{
		Level: "load", Kind: "load.zone_heating", ZoneName: "Office", ServiceKind: "heating", SourceIDs: []string{"ideal"}, sourceName: "Zone Ideal Loads Zone Sensible Heating Rate", sourceFrequency: "Monthly", sourceIsRate: true, Monthly: map[int]float64{1: 7},
	})
	selected = preferredEnergyExplanationSeries([]energyExplanationSeries{annualZoneAir, monthlyIdeal})
	if len(selected) != 1 || selected[0].SourceClass != "zone_ideal_loads" || selected[0].Monthly[1] != 7 || stringSliceContains(selected[0].SourceIDs, "zone-air") {
		t.Fatalf("physical source family was hybridized across the year: %#v", selected)
	}

	monthlyZoneAir := canonicalEnergyExplanationSeries(energyExplanationSeries{
		Level: "load", Kind: "load.zone_heating", ZoneName: "Office", ServiceKind: "heating", SourceIDs: []string{"zone-air-monthly"}, sourceName: "Zone Air System Sensible Heating Energy", sourceFrequency: "Monthly", Monthly: map[int]float64{1: 3},
	})
	monthlyIdeal = canonicalEnergyExplanationSeries(energyExplanationSeries{
		Level: "load", Kind: "load.zone_heating", ZoneName: "Office", ServiceKind: "heating", SourceIDs: []string{"ideal-monthly"}, sourceName: "Zone Ideal Loads Zone Sensible Heating Energy", sourceFrequency: "Monthly", Monthly: map[int]float64{1: 7},
	})
	selected = preferredEnergyExplanationSeries([]energyExplanationSeries{monthlyIdeal, monthlyZoneAir})
	if len(selected) != 1 || selected[0].SourceClass != "zone_air_system" || selected[0].Monthly[1] != 3 || !stringSliceContains(selected[0].MonthlySourceIDs, "zone-air-monthly") || stringSliceContains(selected[0].SourceIDs, "ideal-monthly") {
		t.Fatalf("simultaneous monthly physical families were double counted: %#v", selected)
	}
}

func TestEPATH052AnnualOnlyFallbackCannotCreateLoadDriverOrSupport(t *testing.T) {
	series := []energyExplanationSeries{
		{Level: "energy", Kind: "energy.electricity.total", Carrier: "electricity", MeterHierarchyLevel: "facility_total", Unit: "kWh", SourceIDs: []string{"carrier"}, sourceName: "Electricity:Facility", sourceFrequency: "Annual", Total: 50},
		{Level: "energy", Kind: "energy.cooling", Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", Unit: "kWh", SourceIDs: []string{"end-use"}, sourceName: "Cooling:Electricity", sourceFrequency: "Annual", Total: 40},
		{Level: "energy", Kind: "energy.generators", Carrier: "electricity", EndUse: "generators", Unit: "kWh", SourceIDs: []string{"support"}, sourceName: "ElectricityProduced:Facility", sourceFrequency: "Annual", Total: 5},
		{Level: "load", Kind: "load.zone_cooling", ZoneName: "Office", ServiceKind: "cooling", Unit: "kWh", SourceIDs: []string{"load"}, sourceName: "Zone Air System Sensible Cooling Energy", sourceFrequency: "Annual", Total: 20},
		{Level: "heat", Kind: "heat.infiltration", ZoneName: "Office", Unit: "kWh", SourceIDs: []string{"driver"}, sourceName: "Zone Infiltration Sensible Heat Gain Energy", sourceFrequency: "Annual", Total: 10},
	}
	legacy := buildEnergyExplanationResultWithDriverContext(series, nil, &PurposeRunPlan{}, energyDriverBuildContext{Enabled: true, Multipliers: unitEnergyMultiplierIndex("Office")})
	for _, node := range legacy.Nodes {
		if node.Level == "load" || node.Level == "heat" || node.EndUse == "generators" {
			t.Fatalf("annual-only source fabricated non-energy contribution: %#v", node)
		}
	}
	if energyExplanationNodeByID(legacy.Nodes, "energy.carrier.electricity") == nil || energyExplanationNodeByID(legacy.Nodes, "energy.end_use.cooling.electricity") == nil {
		t.Fatalf("annual carrier/end-use fallback was lost: %#v", legacy.Nodes)
	}
	for _, period := range legacy.Periods {
		if period.Kind == "monthly" {
			t.Fatalf("annual-only fallback fabricated a month: %#v", period)
		}
	}
}

func unitEnergyMultiplierIndex(zoneNames ...string) energyEffectiveMultiplierIndex {
	index := energyEffectiveMultiplierIndex{Enabled: true, Zones: map[string]energyZoneMultiplierRecord{}, SpaceZones: map[string]string{}, OutputKeyZones: map[string]string{}, GroupMultipliers: map[string]float64{}}
	for _, zoneName := range zoneNames {
		index.Zones[normalizePurposeToken(zoneName)] = energyZoneMultiplierRecord{ZoneName: zoneName, ZoneMultiplier: 1, GroupMultiplier: 1}
	}
	return index
}

func sumEnergyPathPeriodNodeValues(periods []EnergyPeriod, id string) float64 {
	total := 0.0
	for _, period := range periods {
		if period.Kind != "monthly" {
			continue
		}
		if node := energyPathV2NodeByID(period.Nodes, id); node != nil {
			total += node.Value
		}
	}
	return roundedEnergyNumber(total)
}

func sumEnergyPathPeriodLinkValues(periods []EnergyPeriod, fromID string, toID string) float64 {
	total := 0.0
	for _, period := range periods {
		if period.Kind != "monthly" {
			continue
		}
		if link := energyPathV2LinkByIDs(period.Links, fromID, toID); link != nil {
			total += link.FromValue
		}
	}
	return roundedEnergyNumber(total)
}

func sumEnergyPathPeriodLinkToValues(periods []EnergyPeriod, fromID string, toID string) float64 {
	total := 0.0
	for _, period := range periods {
		if period.Kind != "monthly" {
			continue
		}
		if link := energyPathV2LinkByIDs(period.Links, fromID, toID); link != nil {
			total += link.ToValue
		}
	}
	return roundedEnergyNumber(total)
}

func sumEnergyExplanationPeriodEdgeValues(periods []EnergyPeriod, fromID string, toID string) float64 {
	total := 0.0
	for _, period := range periods {
		if period.Kind != "monthly" {
			continue
		}
		if edge := energyExplanationEdgeByIDs(period.Edges, fromID, toID); edge != nil {
			total += edge.Value
		}
	}
	return roundedEnergyNumber(total)
}
