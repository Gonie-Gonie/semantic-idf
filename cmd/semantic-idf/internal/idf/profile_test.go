package idf

import (
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

const profileFixtureIDF = `
Version,
  24.1;                    !- Version Identifier

Schedule:Compact,
  OfficeSched,              !- Name
  Fraction,                 !- Schedule Type Limits Name
  Through: 12/31,           !- Field 1
  For: Weekdays,            !- Field 2
  Until: 09:00,             !- Field 3
  0.05,                     !- Field 4
  Until: 18:00,             !- Field 5
  1,                        !- Field 6
  Until: 24:00,             !- Field 7
  0.05,                     !- Field 8
  For: Saturday,            !- Field 9
  Until: 09:00,             !- Field 10
  0,                        !- Field 11
  Until: 15:00,             !- Field 12
  0.5,                      !- Field 13
  Until: 24:00,             !- Field 14
  0,                        !- Field 15
  For: Sunday,              !- Field 16
  Until: 24:00,             !- Field 17
  0;                        !- Field 18

Zone,
  Office A,                 !- Name
  0,                        !- Direction of Relative North
  0,                        !- X Origin
  0,                        !- Y Origin
  0,                        !- Z Origin
  1,                        !- Type
  1,                        !- Multiplier
  3,                        !- Ceiling Height
  300;                      !- Volume

Zone,
  Office B,                 !- Name
  0,                        !- Direction of Relative North
  0,                        !- X Origin
  0,                        !- Y Origin
  0,                        !- Z Origin
  1,                        !- Type
  1,                        !- Multiplier
  3,                        !- Ceiling Height
  300;                      !- Volume

ZoneList,
  Offices,                  !- Name
  Office A,                 !- Zone 1 Name
  Office B;                 !- Zone 2 Name

BuildingSurface:Detailed,
  Office A Floor,           !- Name
  Floor,                    !- Surface Type
  Floor Construction,       !- Construction Name
  Office A,                 !- Zone Name
  Ground,                   !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  NoSun,                    !- Sun Exposure
  NoWind,                   !- Wind Exposure
  0.5,                      !- View Factor to Ground
  4,                        !- Number of Vertices
  0, 0, 0,
  10, 0, 0,
  10, 10, 0,
  0, 10, 0;

BuildingSurface:Detailed,
  Office B Floor,           !- Name
  Floor,                    !- Surface Type
  Floor Construction,       !- Construction Name
  Office B,                 !- Zone Name
  Ground,                   !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  NoSun,                    !- Sun Exposure
  NoWind,                   !- Wind Exposure
  0.5,                      !- View Factor to Ground
  4,                        !- Number of Vertices
  0, 0, 0,
  10, 0, 0,
  10, 10, 0,
  0, 10, 0;

People,
  Office People,            !- Name
  Offices,                  !- Zone or ZoneList Name
  OfficeSched,              !- Number of People Schedule Name
  People/Area,              !- Number of People Calculation Method
  ,                         !- Number of People
  0.08;                     !- People per Zone Floor Area

Lights,
  Office Lights,            !- Name
  Offices,                  !- Zone or ZoneList Name
  OfficeSched,              !- Schedule Name
  Watts/Area,               !- Design Level Calculation Method
  ,                         !- Lighting Level
  10.5;                     !- Watts per Zone Floor Area

ZoneInfiltration:DesignFlowRate,
  Office Infiltration,      !- Name
  Offices,                  !- Zone or ZoneList Name
  OfficeSched,              !- Schedule Name
  AirChanges/Hour,          !- Design Flow Rate Calculation Method
  ,                         !- Design Flow Rate
  ,                         !- Flow per Zone Floor Area
  ,                         !- Flow per Exterior Surface Area
  0.3;                      !- Air Changes per Hour
`

func TestAnalyzeProfileNormalizesZoneProfiles(t *testing.T) {
	doc, err := Parse(profileFixtureIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := AnalyzeProfile(doc)

	if profile.ZoneCount != 2 {
		t.Fatalf("zone count = %d, want 2", profile.ZoneCount)
	}
	if profile.ItemCount != 6 {
		t.Fatalf("item count = %d, want 6", profile.ItemCount)
	}
	if profile.GroupCount != 1 {
		t.Fatalf("group count = %d, want 1", profile.GroupCount)
	}
	if len(profile.Schedules) != 1 || profile.Schedules[0].DetectedPattern != "weekday_9_to_6_sat_9_to_3" {
		t.Fatalf("schedule summary = %#v, want weekday/saturday pattern", profile.Schedules)
	}
	if !profile.Schedules[0].Resolved {
		t.Fatalf("schedule should be resolved: %#v", profile.Schedules[0])
	}
	if got := len(profile.Schedules[0].WeeklyProfile); got != 168 {
		t.Fatalf("weekly profile length = %d, want 168", got)
	}
	if got := len(profile.Schedules[0].Rules); got != 3 {
		t.Fatalf("schedule rules = %d, want 3", got)
	}
	if got := profile.Schedules[0].WeekdayProfile[9]; got != 1 {
		t.Fatalf("weekday 09:00 profile = %v, want 1", got)
	}
	if got := profile.Schedules[0].SaturdayProfile[15]; got != 0 {
		t.Fatalf("saturday 15:00 profile = %v, want 0", got)
	}

	zone := profile.ZoneProfiles[0]
	assertProfileDimension(t, zone, ProfileDimensionOccupancy, 0.08, 0.0001)
	assertProfileDimension(t, zone, ProfileDimensionLighting, 10.5, 0.0001)
	assertProfileDimension(t, zone, ProfileDimensionInfiltration, 0.3, 0.0001)
}

func TestProfileGroupingSeparatesScheduleEngineeringContributionAllocation(t *testing.T) {
	doc, err := Parse(`
Schedule:Constant, Schedule A, , 1;
Schedule:Constant, Schedule B, , 0.5;

Zone, Zone A, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Zone, Zone B, 0, 0, 0, 0, 1, 1, 3, 300, 100;

Lights, Zone A Schedule A Lights, Zone A, Schedule A, LightingLevel, 10;
Lights, Zone A Schedule B Lights, Zone A, Schedule B, LightingLevel, 20;

Lights, Zone B Schedule A Lights, Zone B, Schedule A, LightingLevel, 20;
Lights, Zone B Schedule B Lights, Zone B, Schedule B, LightingLevel, 10;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report := AnalyzeProfile(doc)
	if report.GroupCount != 2 {
		t.Fatalf("group count = %d, want 2 for swapped per-schedule engineering contributions", report.GroupCount)
	}

	zoneA := findZoneProfile(report.ZoneProfiles, "Zone A")
	zoneB := findZoneProfile(report.ZoneProfiles, "Zone B")
	if zoneA == nil || zoneB == nil {
		t.Fatalf("zone profiles = %#v, want Zone A and Zone B", report.ZoneProfiles)
	}
	lightingA := profileDimensionSummary(zoneA.Dimensions, ProfileDimensionLighting)
	lightingB := profileDimensionSummary(zoneB.Dimensions, ProfileDimensionLighting)
	if lightingA == nil || lightingB == nil {
		t.Fatalf("lighting summaries = %#v / %#v", lightingA, lightingB)
	}
	if lightingA.Value != lightingB.Value || lightingA.ScheduleName != lightingB.ScheduleName {
		t.Fatalf("aggregate basis differs: A=%#v B=%#v; fixture must have equal total and schedule set", lightingA, lightingB)
	}
	if lightingA.ContributionSignature == "" || lightingB.ContributionSignature == "" || lightingA.ContributionSignature == lightingB.ContributionSignature {
		t.Fatalf("contribution signatures = %q / %q, want distinct stable allocations", lightingA.ContributionSignature, lightingB.ContributionSignature)
	}
	if lightingA.ContributionSignature != "Schedule A:0.333000+Schedule B:0.667000" {
		t.Fatalf("Zone A contribution signature = %q", lightingA.ContributionSignature)
	}
	if lightingB.ContributionSignature != "Schedule A:0.667000+Schedule B:0.333000" {
		t.Fatalf("Zone B contribution signature = %q", lightingB.ContributionSignature)
	}
}

func TestProfileGroupingNormalizesScheduleContributionsAcrossZoneAreas(t *testing.T) {
	doc, err := Parse(`
Schedule:Constant, Schedule A, , 1;
Zone, Small Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Zone, Large Zone, 0, 0, 0, 0, 1, 1, 3, 600, 200;
Lights, Small Lights, Small Zone, Schedule A, Watts/Area, , 10;
Lights, Large Lights, Large Zone, Schedule A, Watts/Area, , 10;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report := AnalyzeProfile(doc)
	if report.GroupCount != 1 {
		t.Fatalf("group count = %d, want one Profile for equal density and schedule shape across different Zone areas", report.GroupCount)
	}
	for _, zoneName := range []string{"Small Zone", "Large Zone"} {
		zone := findZoneProfile(report.ZoneProfiles, zoneName)
		if zone == nil {
			t.Fatalf("zone %q not found", zoneName)
		}
		lighting := profileDimensionSummary(zone.Dimensions, ProfileDimensionLighting)
		if lighting == nil || lighting.MetricID != "power_per_area" || lighting.Value != 10 {
			t.Fatalf("%s lighting = %#v, want 10 W/m2", zoneName, lighting)
		}
		if lighting.ContributionSignature != "Schedule A:1.000000" {
			t.Fatalf("%s contribution signature = %q, want area-invariant Schedule A fraction", zoneName, lighting.ContributionSignature)
		}
	}
}

func TestProfileGroupingKeepsPreferredAndFallbackMetricIdentityDistinct(t *testing.T) {
	doc, err := Parse(`
Schedule:Constant, Schedule A, , 1;
Zone, Density Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Zone, Count Zone, 0, 0, 0, 0, 1, 1, 3, 300;
People, Density People, Density Zone, Schedule A, People/Area, , 10;
People, Count People, Count Zone, Schedule A, People, 10;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report := AnalyzeProfile(doc)
	if report.GroupCount != 2 {
		t.Fatalf("group count = %d, want preferred density and count fallback kept in separate Profiles", report.GroupCount)
	}

	densityZone := findZoneProfile(report.ZoneProfiles, "Density Zone")
	countZone := findZoneProfile(report.ZoneProfiles, "Count Zone")
	if densityZone == nil || countZone == nil {
		t.Fatalf("zone profiles = %#v, want Density Zone and Count Zone", report.ZoneProfiles)
	}
	density := profileDimensionSummary(densityZone.Dimensions, ProfileDimensionOccupancy)
	count := profileDimensionSummary(countZone.Dimensions, ProfileDimensionOccupancy)
	if density == nil || density.Value != 10 || density.MetricID != "people_per_area" || density.FallbackMetric {
		t.Fatalf("preferred density summary = %#v, want preferred 10 people/m2", density)
	}
	if count == nil || count.Value != 10 || count.MetricID != "count" || !count.FallbackMetric {
		t.Fatalf("fallback count summary = %#v, want fallback 10 people", count)
	}
	groupKeyByZone := map[string]string{}
	for _, group := range report.Groups {
		for _, zoneName := range group.ZoneNames {
			groupKeyByZone[zoneName] = group.Key
		}
	}
	if !strings.Contains(groupKeyByZone["Density Zone"], "people_per_area:preferred") ||
		!strings.Contains(groupKeyByZone["Count Zone"], "count:fallback") ||
		groupKeyByZone["Density Zone"] == groupKeyByZone["Count Zone"] {
		t.Fatalf("group keys = %#v, want actual selected metric and fallback identity", groupKeyByZone)
	}
}

func TestProfileSourceIntensitiesResolveWithoutGeometry(t *testing.T) {
	doc, err := Parse(`
Zone, No Geometry, 0, 0, 0, 0, 1, 1, 3, 300;
People, Density People, No Geometry, Always, Area/Person, , , 20, 0.3, Autocalculate, Always;
Lights, Density Lights, No Geometry, Always, Watts/Area, , 11, , 0, 0, 0;
GasEquipment, Density Gas, No Geometry, Always, Power/Area, , 4, , 0, 0, 0;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := AnalyzeProfile(doc)
	zone := findZoneProfile(profile.ZoneProfiles, "No Geometry")
	if zone == nil {
		t.Fatal("No Geometry profile not found")
	}
	occupancy := profileDimensionSummary(zone.Dimensions, ProfileDimensionOccupancy)
	if occupancy == nil || occupancy.Status != metricStatusOK || math.Abs(occupancy.Value-0.05) > 1e-9 {
		t.Fatalf("occupancy = %#v, want source density 0.05 people/m2", occupancy)
	}
	lighting := profileDimensionSummary(zone.Dimensions, ProfileDimensionLighting)
	if lighting == nil || lighting.Status != metricStatusOK || lighting.Value != 11 {
		t.Fatalf("lighting = %#v, want source density 11 W/m2", lighting)
	}
	equipment := profileDimensionSummary(zone.Dimensions, ProfileDimensionEquipment)
	if equipment == nil || equipment.Status != metricStatusOK || equipment.Value != 4 {
		t.Fatalf("equipment = %#v, want source density 4 W/m2", equipment)
	}
}

func TestProfileZoneMultiplierKeepsRepresentativeIntensitiesInvariant(t *testing.T) {
	const common = `
BuildingSurface:Detailed,
  Floor, Floor, Floor Construction, Test Zone, , Ground, , NoSun, NoWind, 0.5, 4,
  0,0,0, 10,0,0, 10,10,0, 0,10,0;
People, People, Test Zone, Always, People, 10, , , 0.3, Autocalculate, Always;
Lights, Lights, Test Zone, Always, LightingLevel, 1000, , , 0, 0, 0;
ElectricEquipment, Plug, Test Zone, Always, Watts/Area, , 5, , 0, 0, 0;
`
	analyze := func(multiplier int) ZoneProfile {
		doc, err := Parse("Zone, Test Zone, 0, 0, 0, 0, 1, " + strconv.Itoa(multiplier) + ", 3, 300;\n" + common)
		if err != nil {
			t.Fatalf("Parse(%d) error = %v", multiplier, err)
		}
		zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Test Zone")
		if zone == nil {
			t.Fatalf("Test Zone profile missing for multiplier %d", multiplier)
		}
		return *zone
	}
	one := analyze(1)
	ten := analyze(10)
	for dimension, want := range map[string]float64{
		ProfileDimensionOccupancy: 0.1,
		ProfileDimensionLighting:  10,
		ProfileDimensionEquipment: 5,
	} {
		a := profileDimensionSummary(one.Dimensions, dimension)
		b := profileDimensionSummary(ten.Dimensions, dimension)
		if a == nil || b == nil || a.Status != metricStatusOK || b.Status != metricStatusOK || math.Abs(a.Value-want) > 1e-9 || math.Abs(b.Value-want) > 1e-9 {
			t.Fatalf("%s multiplier invariant = %#v vs %#v, want %v", dimension, a, b, want)
		}
	}
}

func TestProfileSpaceListDirectLevelsRepeatPerSpaceWithoutDuplicateZoneItems(t *testing.T) {
	doc, err := Parse(`
Zone, Shared Zone, 0, 0, 0, 1, 1, 3, 300, 100;
Space, Space A, Shared Zone, , , 30;
Space, Space B, Shared Zone, , , 70;
SpaceList, Shared Spaces, Space A, Space A, Space B;
People, Shared People, Shared Spaces, Always, People, 2, , , 0.3, Autocalculate, Always;
Lights, Shared Lights, Shared Spaces, Always, LightingLevel, 100, , , 0, 0, 0;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Shared Zone")
	if zone == nil {
		t.Fatal("Shared Zone profile not found")
	}
	occupancy := profileDimensionSummary(zone.Dimensions, ProfileDimensionOccupancy)
	lighting := profileDimensionSummary(zone.Dimensions, ProfileDimensionLighting)
	if occupancy == nil || math.Abs(occupancy.Value-0.04) > 1e-9 || occupancy.ItemCount != 1 {
		t.Fatalf("occupancy = %#v, want 4 people / 100 m2 and one zone item", occupancy)
	}
	if lighting == nil || lighting.Value != 2 || lighting.ItemCount != 1 {
		t.Fatalf("lighting = %#v, want 200 W / 100 m2 and one zone item", lighting)
	}
}

func TestProfileSpaceTargetsUseTheirOwnEngineeringBasis(t *testing.T) {
	doc, err := Parse(`
Zone, Shared Zone, 0, 0, 0, 1, 1, 3, 300, 100;
Space, Space A, Shared Zone, , , 25;
Space, Space B, Shared Zone, , , 75;
People, Space People, Space A, Always, People, 5, , , 0.3, Autocalculate, Always;
Lights, Space Lights, Space A, Always, LightingLevel, 100, , , 0, 0, 0;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Shared Zone")
	if zone == nil {
		t.Fatal("Shared Zone profile not found")
	}
	people := selectProfileMetric(profileItemByName(t, *zone, "Space People").Normalized, "people_per_area")
	if people.Status != metricStatusOK || math.Abs(people.Value-0.2) > 1e-9 {
		t.Fatalf("Space people density = %#v, want 5 people / 25 m2", people)
	}
	lights := selectProfileMetric(profileItemByName(t, *zone, "Space Lights").Normalized, "power_per_area")
	if lights.Status != metricStatusOK || math.Abs(lights.Value-4) > 1e-9 {
		t.Fatalf("Space lighting density = %#v, want 100 W / 25 m2", lights)
	}
	occupancySummary := profileDimensionSummary(zone.Dimensions, ProfileDimensionOccupancy)
	lightingSummary := profileDimensionSummary(zone.Dimensions, ProfileDimensionLighting)
	if occupancySummary == nil || math.Abs(occupancySummary.Value-0.05) > 1e-9 {
		t.Fatalf("Zone occupancy summary = %#v, want 5 people / 100 m2", occupancySummary)
	}
	if lightingSummary == nil || math.Abs(lightingSummary.Value-1) > 1e-9 {
		t.Fatalf("Zone lighting summary = %#v, want 100 W / 100 m2", lightingSummary)
	}
}

func TestProfileSpaceAutocalculatesZeroAreaAndVolumeInputs(t *testing.T) {
	doc, err := Parse(`
Zone, Shared Zone, 0, 0, 0, 1, 1, 3, 0, 0;
Space, Space A, Shared Zone, 4, 0, 0;
BuildingSurface:Detailed,
  Space Floor, Floor, Floor Construction, Shared Zone, Space A, Ground, , NoSun, NoWind, 0.5, 4,
  0,0,0, 10,0,0, 10,5,0, 0,5,0;
ZoneVentilation:DesignFlowRate,
  Space Ventilation, Space A, Always, AirChanges/Hour, , , , 0.5;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Shared Zone")
	if zone == nil {
		t.Fatal("Shared Zone profile not found")
	}
	item := profileItemByName(t, *zone, "Space Ventilation")
	flow := selectProfileMetric(item.Normalized, "flow")
	if flow.Status != metricStatusOK || math.Abs(flow.Value-(0.5*200/3600)) > 1e-9 {
		t.Fatalf("Space ventilation flow = %#v, want ACH * (50 m2 * 4 m) / 3600", flow)
	}
}

func TestProfileZoneCeilingHeightOverridesGeometryVolumeAutocalculation(t *testing.T) {
	doc, err := Parse(`
Zone, Tall Zone, 0, 0, 0, 0, 1, 1, 4, , 100;
BuildingSurface:Detailed,
  Floor, Floor, Floor Construction, Tall Zone, , Ground, , NoSun, NoWind, 0.5, 4,
  0,0,0, 10,0,0, 10,10,0, 0,10,0;
BuildingSurface:Detailed,
  Roof, Roof, Roof Construction, Tall Zone, , Outdoors, , SunExposed, WindExposed, 0.5, 4,
  0,0,3, 0,10,3, 10,10,3, 10,0,3;
ZoneVentilation:DesignFlowRate, Tall Zone Air, Tall Zone, , AirChanges/Hour, , , , 1;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Tall Zone")
	if zone == nil {
		t.Fatal("Tall Zone profile missing")
	}
	flow := selectProfileMetric(profileItemByName(t, *zone, "Tall Zone Air").Normalized, "flow")
	if zone.Volume != 400 || flow.Status != metricStatusOK || math.Abs(flow.Value-(400.0/3600)) > 1e-12 {
		t.Fatalf("Zone ceiling-height volume = %.6g, flow %#v; want 400 m3 and 1 ACH", zone.Volume, flow)
	}
}

func TestProfileMissingMetricsUseEmDash(t *testing.T) {
	doc, err := Parse(`
Zone, Test Zone, 0, 0, 0, 1, 1, 3, 300, 100;
People, Incomplete People, Test Zone, Always, People, , , , 0.3, Autocalculate, Always;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Test Zone")
	if zone == nil {
		t.Fatal("Test Zone profile not found")
	}
	item := profileItemByName(t, *zone, "Incomplete People")
	for _, metric := range item.Normalized {
		if metric.Status == metricStatusMissing && metric.DisplayValue != "—" {
			t.Fatalf("missing metric %q display = %q, want em dash", metric.ID, metric.DisplayValue)
		}
	}
	occupancy := profileDimensionSummary(zone.Dimensions, ProfileDimensionOccupancy)
	if occupancy == nil || occupancy.Status != metricStatusMissing || occupancy.DisplayValue != "—" {
		t.Fatalf("missing occupancy summary = %#v, want em dash", occupancy)
	}
}

func TestProfilePreservesInvalidTargetWarningAtReportLevel(t *testing.T) {
	doc, err := Parse(`
Zone, Valid Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
People, Orphan People, Missing Space, , People, 5;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report := AnalyzeProfile(doc)
	if !profileWarningCode(report.Warnings, "missing_profile_zone") {
		t.Fatalf("report warnings = %#v, want missing_profile_zone", report.Warnings)
	}
}

func TestProfileMarksMixedZoneListTargetPartial(t *testing.T) {
	doc, err := Parse(`
Zone, Valid Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
ZoneList, Mixed Zones, Valid Zone, Missing Zone;
People, Mixed People, Mixed Zones, , People, 5;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report := AnalyzeProfile(doc)
	zone := findZoneProfile(report.ZoneProfiles, "Valid Zone")
	if zone == nil {
		t.Fatal("Valid Zone profile not found")
	}
	occupancy := profileDimensionSummary(zone.Dimensions, ProfileDimensionOccupancy)
	if occupancy == nil || occupancy.Status != metricStatusPartial || math.Abs(occupancy.Value-0.05) > 1e-9 {
		t.Fatalf("mixed ZoneList occupancy = %#v, want known value marked partial", occupancy)
	}
	item := profileItemByName(t, *zone, "Mixed People")
	if !profileWarningCode(item.Warnings, "unresolved_profile_target_member") {
		t.Fatalf("item warnings = %#v, want unresolved target member", item.Warnings)
	}
	if !profileWarningCode(report.Warnings, "unresolved_profile_target_member") {
		t.Fatalf("report warnings = %#v, want unresolved target member", report.Warnings)
	}
}

func TestProfileKeepsAllMissingSpaceListTargetUnavailable(t *testing.T) {
	doc, err := Parse(`
Zone, Valid Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
SpaceList, Broken Spaces, Missing Space A, Missing Space B;
Lights, Orphan Lights, Broken Spaces, , LightingLevel, 100;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	ctx := newProfileContextWithGeometry(doc, AnalyzeGeometry(doc))
	items := ctx.collectProfileItems()
	if len(items) != 1 || items[0].DisplayMetric.Status != metricStatusMissing {
		t.Fatalf("unresolved SpaceList items = %#v, want one unavailable item", items)
	}
	if !profileWarningCode(items[0].Warnings, "unresolved_profile_target_member") {
		t.Fatalf("item warnings = %#v, want unresolved target member", items[0].Warnings)
	}
	report := AnalyzeProfile(doc)
	if !profileWarningCode(report.Warnings, "unresolved_profile_target_member") {
		t.Fatalf("report warnings = %#v, want unresolved target member", report.Warnings)
	}
	zone := findZoneProfile(report.ZoneProfiles, "Valid Zone")
	if zone == nil {
		t.Fatal("Valid Zone profile not found")
	}
	if got := profileDimensionSummary(zone.Dimensions, ProfileDimensionLighting); got != nil {
		t.Fatalf("valid zone lighting = %#v, want no value from unresolved SpaceList", got)
	}
}

func TestProfileResolvedZeroPeopleProducesZeroPersonBasedLoads(t *testing.T) {
	doc, err := Parse(`
Zone, Test Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
People, Empty People, Test Zone, , People, 0;
Lights, Person Lights, Test Zone, , Watts/Person, , , 12;
ZoneVentilation:DesignFlowRate, Person Air, Test Zone, , Flow/Person, , , 0.01;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Test Zone")
	if zone == nil {
		t.Fatal("Test Zone profile missing")
	}
	for objectName, metricID := range map[string]string{"Person Lights": "total_power", "Person Air": "flow"} {
		metric := selectProfileMetric(profileItemByName(t, *zone, objectName).Normalized, metricID)
		if metric.Status != metricStatusOK || metric.Value != 0 {
			t.Fatalf("%s %s = %#v, want resolved zero", objectName, metricID, metric)
		}
	}
}

func TestProfileRejectsPartialPeopleBasisForPersonBasedLoads(t *testing.T) {
	doc, err := Parse(`
Zone, Test Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
People, Valid People, Test Zone, , People, 10;
People, Broken People, Test Zone, , People/Area, , ;
Lights, Person Lights, Test Zone, , Watts/Person, , , 12;
ZoneVentilation:DesignFlowRate, Person Air, Test Zone, , Flow/Person, , , 0.01;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Test Zone")
	if zone == nil {
		t.Fatal("Test Zone profile missing")
	}
	for objectName, metricID := range map[string]string{"Person Lights": "total_power", "Person Air": "flow"} {
		item := profileItemByName(t, *zone, objectName)
		metric := selectProfileMetric(item.Normalized, metricID)
		if metric.Status != metricStatusMissing || !profileWarningCode(item.Warnings, "missing_people_reference") {
			t.Fatalf("%s %s = %#v warnings %#v, want unresolved partial People basis", objectName, metricID, metric, item.Warnings)
		}
	}
}

func TestProfileDoesNotInventSpacePeopleAllocationWithoutAreas(t *testing.T) {
	doc, err := Parse(`
Zone, Test Zone, 0, 0, 0, 0, 1, 1, 0, 0, 100;
Space, Space A, Test Zone, 0, 0, 0;
Space, Space B, Test Zone, 0, 0, 0;
People, Zone People, Test Zone, , People, 10;
Lights, Space Lights, Space A, , Watts/Person, , , 12;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Test Zone")
	if zone == nil {
		t.Fatal("Test Zone profile missing")
	}
	item := profileItemByName(t, *zone, "Space Lights")
	if metric := selectProfileMetric(item.Normalized, "total_power"); metric.Status != metricStatusMissing {
		t.Fatalf("Space Watts/Person = %#v, want unresolved without Space allocation basis", metric)
	}
}

func TestProfilePreservesImplicitZoneRemainderArea(t *testing.T) {
	doc, err := Parse(`
Zone, Mixed Zone, 0, 0, 0, 0, 1, 1, 3, 0, 0;
Space, Explicit Space, Mixed Zone, 3, 0, 0;
BuildingSurface:Detailed,
  Explicit Floor, Floor, Floor Construction, Mixed Zone, Explicit Space, Ground, , NoSun, NoWind, 0.5, 4,
  0,0,0, 5,0,0, 5,5,0, 0,5,0;
BuildingSurface:Detailed,
  Remainder Floor, Floor, Floor Construction, Mixed Zone, , Ground, , NoSun, NoWind, 0.5, 4,
  5,0,0, 20,0,0, 20,5,0, 5,5,0;
People, Zone People, Mixed Zone, , People, 10;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Mixed Zone")
	if zone == nil {
		t.Fatal("Mixed Zone profile missing")
	}
	occupancy := profileDimensionSummary(zone.Dimensions, ProfileDimensionOccupancy)
	if zone.FloorArea != 100 || occupancy == nil || math.Abs(occupancy.Value-0.1) > 1e-9 {
		t.Fatalf("mixed-space profile = floor %.6g occupancy %#v, want 100 m2 and 0.1 people/m2", zone.FloorArea, occupancy)
	}
}

func TestProfileDeclaredZoneAreaScalesExplicitAndImplicitSpacesProportionally(t *testing.T) {
	doc, err := Parse(`
Zone, Mixed Zone, 0, 0, 0, 0, 1, 1, 3, 0, 200;
Space, Explicit Space, Mixed Zone, 3, 0, 0;
BuildingSurface:Detailed,
  Explicit Floor, Floor, Floor Construction, Mixed Zone, Explicit Space, Ground, , NoSun, NoWind, 0.5, 4,
  0,0,0, 5,0,0, 5,5,0, 0,5,0;
BuildingSurface:Detailed,
  Remainder Floor, Floor, Floor Construction, Mixed Zone, , Ground, , NoSun, NoWind, 0.5, 4,
  5,0,0, 20,0,0, 20,5,0, 5,5,0;
People, Space People, Explicit Space, , People/Area, , 0.1;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Mixed Zone")
	if zone == nil {
		t.Fatal("Mixed Zone profile missing")
	}
	item := profileItemByName(t, *zone, "Space People")
	count := selectProfileMetric(item.Normalized, "count")
	if zone.FloorArea != 200 || math.Abs(count.Value-5) > 1e-9 {
		t.Fatalf("scaled mixed Spaces = zone %.6g count %#v, want explicit Space 50/200 m2 and 5 people", zone.FloorArea, count)
	}
}

func TestProfileDeclaredSpaceAreaOverridesSurfaceBeforeZoneScaling(t *testing.T) {
	doc, err := Parse(`
Zone, Mixed Zone, 0, 0, 0, 0, 1, 1, 3, 0, 200;
Space, Explicit Space, Mixed Zone, 3, 0, 50;
BuildingSurface:Detailed,
  Explicit Floor, Floor, Floor Construction, Mixed Zone, Explicit Space, Ground, , NoSun, NoWind, 0.5, 4,
  0,0,0, 5,0,0, 5,5,0, 0,5,0;
BuildingSurface:Detailed,
  Remainder Floor, Floor, Floor Construction, Mixed Zone, , Ground, , NoSun, NoWind, 0.5, 4,
  5,0,0, 20,0,0, 20,5,0, 5,5,0;
People, Space People, Explicit Space, , People/Area, , 0.1;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Mixed Zone")
	if zone == nil {
		t.Fatal("Mixed Zone profile missing")
	}
	count := selectProfileMetric(profileItemByName(t, *zone, "Space People").Normalized, "count")
	if math.Abs(count.Value-8) > 1e-9 {
		t.Fatalf("declared Space override count = %#v, want 50/(50+75)*200*0.1 = 8", count)
	}
}

func TestProfileDeclaredSpaceAreaOverridesSurfaceInAutomaticZoneArea(t *testing.T) {
	doc, err := Parse(`
Zone, Mixed Zone, 0, 0, 0, 0, 1, 1, 3, 0, 0;
Space, Explicit Space, Mixed Zone, 3, 0, 50;
BuildingSurface:Detailed,
  Explicit Floor, Floor, Floor Construction, Mixed Zone, Explicit Space, Ground, , NoSun, NoWind, 0.5, 4,
  0,0,0, 5,0,0, 5,5,0, 0,5,0;
BuildingSurface:Detailed,
  Remainder Floor, Floor, Floor Construction, Mixed Zone, , Ground, , NoSun, NoWind, 0.5, 4,
  5,0,0, 20,0,0, 20,5,0, 5,5,0;
People, Space People, Explicit Space, , People/Area, , 0.1;
People, Zone People, Mixed Zone, , People, 12.5;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Mixed Zone")
	if zone == nil {
		t.Fatal("Mixed Zone profile missing")
	}
	count := selectProfileMetric(profileItemByName(t, *zone, "Space People").Normalized, "count")
	occupancy := profileDimensionSummary(zone.Dimensions, ProfileDimensionOccupancy)
	if zone.FloorArea != 125 || math.Abs(count.Value-5) > 1e-9 || occupancy == nil || math.Abs(occupancy.Value-0.14) > 1e-9 {
		t.Fatalf("automatic mixed Space area = zone %.6g count %#v occupancy %#v, want 125 m2, 5 people, 0.14 people/m2", zone.FloorArea, count, occupancy)
	}
}

func TestProfilePreservesTinyNonzeroEngineeringValues(t *testing.T) {
	doc, err := Parse(`
Zone, Test Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
ZoneVentilation:DesignFlowRate, Tiny Air, Test Zone, , Flow/Zone, 0.00000001;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Test Zone")
	if zone == nil {
		t.Fatal("Test Zone profile missing")
	}
	dimension := profileDimensionSummary(zone.Dimensions, ProfileDimensionVentilation)
	if dimension == nil || dimension.Value != 1e-8 || dimension.DisplayValue == "0.0 m3/s" || dimension.DisplayValue == "0.00 m3/s" {
		t.Fatalf("tiny ventilation summary = %#v, want nonzero scientific display", dimension)
	}
}

func TestProfileWarnsWhenScheduleUsesDesignLevelFallback(t *testing.T) {
	doc, err := Parse(`
Zone, Test Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Schedule:File, External Schedule, Fraction, schedule.csv, 1, 1, 8760, Comma, No, 1, 0;
Lights, Test Lights, Test Zone, External Schedule, Watts/Area, , 10;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Test Zone")
	if zone == nil {
		t.Fatal("Test Zone profile missing")
	}
	item := profileItemByName(t, *zone, "Test Lights")
	if !profileWarningCode(item.Warnings, "schedule_profile_fallback") {
		t.Fatalf("schedule warnings = %#v, want explicit design-level fallback warning", item.Warnings)
	}
}

func TestProfileSupportsAllStandardEquipmentFamiliesAndAliases(t *testing.T) {
	doc, err := Parse(`
Zone, Test Zone, 0, 0, 0, 1, 1, 3, 300, 100;
People, People, Test Zone, Always, People, 10, , , 0.3, Autocalculate, Always;
ElectricEquipment, Electric, Test Zone, Always, EquipmentLevel, 100, , , 0, 0, 0;
GasEquipment, Gas, Test Zone, Always, Power/Area, , 2, , 0, 0, 0;
HotWaterEquipment, Hot Water, Test Zone, Always, Watts/Person, , , 10, 0, 0, 0;
SteamEquipment, Steam, Test Zone, Always, EquipmentLevel, 50, , , 0, 0, 0;
OtherEquipment, Other, None, Test Zone, Always, Power/Person, , , 5, 0, 0, 0;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := findZoneProfile(AnalyzeProfile(doc).ZoneProfiles, "Test Zone")
	if zone == nil {
		t.Fatal("Test Zone profile not found")
	}
	items := make([]ProfileItem, 0, 5)
	for _, item := range zone.Items {
		if item.Dimension == ProfileDimensionEquipment {
			items = append(items, item)
		}
	}
	if len(items) != 5 {
		t.Fatalf("equipment items = %d, want five object families: %#v", len(items), items)
	}
	resolvedSource := map[string]bool{}
	for _, item := range items {
		for _, metric := range item.Normalized {
			if metric.Status != metricStatusMissing {
				resolvedSource[item.ObjectType] = true
			}
		}
	}
	if len(resolvedSource) != 5 {
		t.Fatalf("resolved equipment families = %#v, want all five", resolvedSource)
	}
}

func TestAnalyzeProfileLargeOfficeAreaPerPersonAndOutdoorAir(t *testing.T) {
	content, err := os.ReadFile("../../frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	doc, err := Parse(string(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := AnalyzeProfile(doc)
	zone := findZoneProfile(profile.ZoneProfiles, "Perimeter_bot_ZN_1")
	if zone == nil {
		t.Fatal("Perimeter_bot_ZN_1 profile not found")
	}
	occupancy := profileDimensionSummary(zone.Dimensions, ProfileDimensionOccupancy)
	if occupancy == nil || occupancy.Status != metricStatusOK {
		t.Fatalf("occupancy = %#v, want resolved people/area", occupancy)
	}
	if math.Abs(occupancy.Value-1.0/18.58) > 0.00001 {
		t.Fatalf("occupancy density = %v, want %v", occupancy.Value, 1.0/18.58)
	}
	outdoorAir := profileDimensionSummary(zone.Dimensions, ProfileDimensionOutdoorAir)
	if outdoorAir == nil || outdoorAir.Status != metricStatusOK {
		t.Fatalf("outdoor air = %#v, want resolved flow/person", outdoorAir)
	}
	if math.Abs(outdoorAir.Value-0.0125) > 0.000001 {
		t.Fatalf("outdoor air per person = %v, want 0.0125", outdoorAir.Value)
	}
}

func TestAnalyzeProfileParsesCompositeScheduleSelectors(t *testing.T) {
	doc, err := Parse(`
Schedule:Compact,
  ComboSched,               !- Name
  Fraction,                 !- Schedule Type Limits Name
  Through: 12/31,           !- Field 1
  For: SummerDesignDay,     !- Field 2
  Until: 24:00,             !- Field 3
  1,                        !- Field 4
  For: Weekdays SummerDesignDay, !- Field 5
  Until: 08:00,             !- Field 6
  0,                        !- Field 7
  Until: 18:00,             !- Field 8
  1,                        !- Field 9
  Until: 24:00,             !- Field 10
  0,                        !- Field 11
  For: Saturday WinterDesignDay, !- Field 12
  Until: 12:00,             !- Field 13
  0.5,                      !- Field 14
  Until: 24:00,             !- Field 15
  0,                        !- Field 16
  For: Sunday Holidays AllOtherDays, !- Field 17
  Until: 24:00,             !- Field 18
  0;                        !- Field 19
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := AnalyzeProfile(doc)
	if len(profile.Schedules) != 1 {
		t.Fatalf("schedule count = %d, want 1", len(profile.Schedules))
	}
	schedule := profile.Schedules[0]
	if !schedule.Resolved {
		t.Fatalf("schedule should resolve composite selectors: %#v", schedule)
	}
	if got := schedule.WeekdayProfile[9]; got != 1 {
		t.Fatalf("weekday 09:00 profile = %v, want 1", got)
	}
	if got := schedule.SaturdayProfile[10]; got != 0.5 {
		t.Fatalf("saturday 10:00 profile = %v, want 0.5", got)
	}
	if got := schedule.SundayProfile[10]; got != 0 {
		t.Fatalf("sunday 10:00 profile = %v, want 0", got)
	}
}

func TestProfileGraphDatasetBuildsDeckSeries(t *testing.T) {
	doc, err := Parse(profileFixtureIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := AnalyzeProfile(doc)
	lighting := findProfileSeries(profile.GraphDataset.Series, "zone", "Office A", ProfileDimensionLighting)
	if lighting == nil {
		t.Fatalf("lighting series for Office A not found")
	}
	if len(lighting.AnnualMultiplierProfile) != 8760 {
		t.Fatalf("annual multiplier length = %d, want 8760", len(lighting.AnnualMultiplierProfile))
	}
	if len(lighting.WeekMultiplierProfile) != 168 {
		t.Fatalf("week multiplier length = %d, want 168", len(lighting.WeekMultiplierProfile))
	}
	if len(lighting.DayMultiplierProfile) != 72 {
		t.Fatalf("day multiplier length = %d, want 72", len(lighting.DayMultiplierProfile))
	}
	if lighting.DesignValue != 10.5 || lighting.AnnualContribution <= 0 {
		t.Fatalf("lighting series design/annual = %v/%v, want design 10.5 and annual > 0", lighting.DesignValue, lighting.AnnualContribution)
	}
}

func TestProfileGraphSchedulesAdditiveContributionsBeforeZoneNormalization(t *testing.T) {
	analyze := func(secondScheduleValue string) (*ProfileDimensionSummary, *ProfileGraphSeries) {
		doc, err := Parse(`
Schedule:Constant, Full, , 1;
Schedule:Constant, Second, , ` + secondScheduleValue + `;
Zone, Shared Zone, 0, 0, 0, 1, 1, 3, 300, 100;
Space, Space A, Shared Zone, , , 25;
Space, Space B, Shared Zone, , , 75;
Lights, Space A Lights, Space A, Full, LightingLevel, 100, , , 0, 0, 0;
Lights, Space B Lights, Space B, Second, LightingLevel, 300, , , 0, 0, 0;
`)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		profile := AnalyzeProfile(doc)
		zone := findZoneProfile(profile.ZoneProfiles, "Shared Zone")
		if zone == nil {
			t.Fatal("Shared Zone profile not found")
		}
		dimension := profileDimensionSummary(zone.Dimensions, ProfileDimensionLighting)
		series := findProfileSeries(profile.GraphDataset.Series, "zone", "Shared Zone", ProfileDimensionLighting)
		if dimension == nil || series == nil {
			t.Fatalf("lighting dimension/series missing: %#v / %#v", dimension, series)
		}
		return dimension, series
	}

	dimension, mixed := analyze("0.5")
	if math.Abs(dimension.Value-4) > 1e-12 {
		t.Fatalf("lighting design value = %v, want (100 W + 300 W) / 100 m2 = 4 W/m2", dimension.Value)
	}
	if got := mixed.Values[0]; math.Abs(got-2.5) > 1e-12 {
		t.Fatalf("mixed-schedule graph value = %v, want (100 W*1 + 300 W*0.5) / 100 m2 = 2.5 W/m2", got)
	}
	if got := mixed.AnnualMultiplierProfile[0]; math.Abs(got-0.625) > 1e-12 {
		t.Fatalf("mixed-schedule multiplier = %v, want 2.5/4 = 0.625", got)
	}
	if math.Abs(mixed.Peak-2.5) > 1e-12 {
		t.Fatalf("mixed-schedule peak = %v, want 2.5 W/m2", mixed.Peak)
	}
	if !strings.Contains(mixed.ScheduleName, "Full") || !strings.Contains(mixed.ScheduleName, "Second") {
		t.Fatalf("mixed-schedule label = %q, want both contributing schedules", mixed.ScheduleName)
	}

	fullDimension, full := analyze("1")
	if math.Abs(full.Values[0]-fullDimension.Value) > 1e-12 || math.Abs(full.Peak-fullDimension.Value) > 1e-12 {
		t.Fatalf("all-on graph value/peak = %v/%v, want displayed design value %v", full.Values[0], full.Peak, fullDimension.Value)
	}
}

func TestProfileGraphPreservesTinyValidEngineeringValues(t *testing.T) {
	doc, err := Parse(`
Schedule:Constant, Full, , 1;
Zone, Tiny Zone, 0, 0, 0, 1, 1, 3, 300, 100;
ZoneVentilation:DesignFlowRate,
  Tiny Ventilation, Tiny Zone, Full, Flow/Zone, 0.0000004, , , ;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := AnalyzeProfile(doc)
	zone := findZoneProfile(profile.ZoneProfiles, "Tiny Zone")
	if zone == nil {
		t.Fatal("Tiny Zone profile not found")
	}
	dimension := profileDimensionSummary(zone.Dimensions, ProfileDimensionVentilation)
	series := findProfileSeries(profile.GraphDataset.Series, "zone", "Tiny Zone", ProfileDimensionVentilation)
	if dimension == nil || series == nil {
		t.Fatalf("ventilation dimension/series missing: %#v / %#v", dimension, series)
	}
	if dimension.MetricID != "flow" || math.Abs(dimension.Value-4e-7) > 1e-15 {
		t.Fatalf("tiny ventilation dimension = %#v, want 4e-7 m3/s source fallback", dimension)
	}
	if len(series.Values) != 8760 || math.Abs(series.Values[0]-4e-7) > 1e-15 || math.Abs(series.Peak-4e-7) > 1e-15 {
		t.Fatalf("tiny ventilation graph value/peak = %v/%v, want nonzero 4e-7", series.Values[0], series.Peak)
	}
	if series.AnnualContribution <= 0 {
		t.Fatalf("tiny ventilation annual contribution = %v, want > 0", series.AnnualContribution)
	}
}

func TestProfileGraphPreservesTinyScheduleFractions(t *testing.T) {
	cases := map[string]string{
		"constant": `Schedule:Constant, Tiny Schedule, , 0.00001;`,
		"compact": `
Schedule:Compact,
  Tiny Schedule, ,
  Through: 12/31,
  For: AllDays,
  Until: 24:00,
  0.00001;`,
	}
	for name, scheduleObject := range cases {
		t.Run(name, func(t *testing.T) {
			doc, err := Parse(scheduleObject + `
Zone, Test Zone, 0, 0, 0, 1, 1, 3, 300, 100;
Lights, Lights, Test Zone, Tiny Schedule, Watts/Area, , 5, , 0, 0, 0;
`)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			series := findProfileSeries(AnalyzeProfile(doc).GraphDataset.Series, "zone", "Test Zone", ProfileDimensionLighting)
			if series == nil {
				t.Fatal("lighting graph series not found")
			}
			if len(series.AnnualMultiplierProfile) != 8760 || math.Abs(series.AnnualMultiplierProfile[0]-1e-5) > 1e-15 {
				t.Fatalf("tiny schedule multiplier = %v, want 1e-5", series.AnnualMultiplierProfile[0])
			}
			if len(series.Values) != 8760 || math.Abs(series.Values[0]-5e-5) > 1e-15 || math.Abs(series.Peak-5e-5) > 1e-15 {
				t.Fatalf("tiny schedule graph value/peak = %v/%v, want 5e-5", series.Values[0], series.Peak)
			}
			if len(series.MonthMultiplierProfile) != 12 || math.Abs(series.MonthMultiplierProfile[0]-1e-5) > 1e-15 {
				t.Fatalf("tiny schedule monthly multiplier = %v, want 1e-5", series.MonthMultiplierProfile[0])
			}
		})
	}
}

func TestProfileGraphSurfacesUnsupportedScheduleFallback(t *testing.T) {
	doc, err := Parse(`
Schedule:File,
  Imported Schedule, , values.csv, 1, 1, 8760, Comma, No, 60;
Zone, Test Zone, 0, 0, 0, 1, 1, 3, 300, 100;
Lights, Lights, Test Zone, Imported Schedule, Watts/Area, , 5, , 0, 0, 0;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	series := findProfileSeries(AnalyzeProfile(doc).GraphDataset.Series, "zone", "Test Zone", ProfileDimensionLighting)
	if series == nil {
		t.Fatal("lighting graph series not found")
	}
	if series.Status != metricStatusPartial {
		t.Fatalf("unsupported-schedule graph status = %q, want partial", series.Status)
	}
	found := false
	for _, warning := range series.Warnings {
		if warning.Code == "schedule_profile_fallback" && strings.Contains(warning.Message, "not yet parsed") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("unsupported schedule fallback warning missing: %#v", series.Warnings)
	}
	if len(series.Values) != 8760 || math.Abs(series.Values[0]-series.DesignValue) > 1e-12 {
		t.Fatalf("unsupported schedule fallback values/design = %v/%v, want explicit all-on fallback", series.Values[0], series.DesignValue)
	}
}

func TestProfileScheduleSimilarityClustersSameContentNames(t *testing.T) {
	doc, err := Parse(profileFixtureIDF + `
Schedule:Compact,
  OfficeSchedCopy,          !- Name
  Fraction,                 !- Schedule Type Limits Name
  Through: 12/31,           !- Field 1
  For: Weekdays,            !- Field 2
  Until: 09:00,             !- Field 3
  0.05,                     !- Field 4
  Until: 18:00,             !- Field 5
  1,                        !- Field 6
  Until: 24:00,             !- Field 7
  0.05,                     !- Field 8
  For: Saturday,            !- Field 9
  Until: 09:00,             !- Field 10
  0,                        !- Field 11
  Until: 15:00,             !- Field 12
  0.5,                      !- Field 13
  Until: 24:00,             !- Field 14
  0,                        !- Field 15
  For: Sunday,              !- Field 16
  Until: 24:00,             !- Field 17
  0;                        !- Field 18

ElectricEquipment,
  Office B Plug Load,       !- Name
  Office B,                 !- Zone or ZoneList Name
  OfficeSchedCopy,          !- Schedule Name
  Watts/Area,               !- Design Level Calculation Method
  ,                         !- Design Level
  8;                        !- Watts per Zone Floor Area
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := AnalyzeProfile(doc)
	foundCluster := false
	foundHint := false
	for _, cluster := range profile.ScheduleClusters {
		if cluster.SameContentDifferentNames && containsString(cluster.ScheduleNames, "OfficeSched") && containsString(cluster.ScheduleNames, "OfficeSchedCopy") {
			foundCluster = true
			break
		}
	}
	for _, hint := range profile.Outliers {
		if hint.RuleID == "different_name_same_schedule_hash" {
			foundHint = true
			break
		}
	}
	if !foundCluster || !foundHint {
		t.Fatalf("same-content schedule names cluster=%v hint=%v clusters=%#v hints=%#v", foundCluster, foundHint, profile.ScheduleClusters, profile.Outliers)
	}
}

func TestProfileOutliersAndParameterCandidates(t *testing.T) {
	doc, err := Parse(profileFixtureIDF + `
Zone,
  Office C,                 !- Name
  0,                        !- Direction of Relative North
  0,                        !- X Origin
  0,                        !- Y Origin
  0,                        !- Z Origin
  1,                        !- Type
  1,                        !- Multiplier
  3,                        !- Ceiling Height
  300;                      !- Volume

BuildingSurface:Detailed,
  Office C Floor,           !- Name
  Floor,                    !- Surface Type
  Floor Construction,       !- Construction Name
  Office C,                 !- Zone Name
  Ground,                   !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  NoSun,                    !- Sun Exposure
  NoWind,                   !- Wind Exposure
  0.5,                      !- View Factor to Ground
  4,                        !- Number of Vertices
  0, 0, 0,
  10, 0, 0,
  10, 10, 0,
  0, 10, 0;

Lights,
  Office C Lights,          !- Name
  Office C,                 !- Zone or ZoneList Name
  OfficeSched,              !- Schedule Name
  Watts/Area,               !- Design Level Calculation Method
  ,                         !- Lighting Level
  90;                       !- Watts per Zone Floor Area
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := AnalyzeProfile(doc)
	foundOutlier := false
	for _, hint := range profile.Outliers {
		if hint.RuleID == "robust_value_outlier" && hint.ZoneName == "Office C" && hint.Dimension == ProfileDimensionLighting {
			foundOutlier = true
			break
		}
	}
	foundCandidate := false
	for _, candidate := range profile.ParameterCandidates {
		if candidate.Dimension == ProfileDimensionLighting && candidate.CurrentMax >= 90 && candidate.ApplyRequest != nil {
			foundCandidate = true
			break
		}
	}
	if !foundOutlier || !foundCandidate {
		t.Fatalf("lighting outlier=%v candidate=%v outliers=%#v candidates=%#v", foundOutlier, foundCandidate, profile.Outliers, profile.ParameterCandidates)
	}
}

func TestApplyProfileClonesSourceObjectsToTargetZone(t *testing.T) {
	doc, err := Parse(profileFixtureIDF + `
Zone,
  Copy Target,              !- Name
  0,                        !- Direction of Relative North
  0,                        !- X Origin
  0,                        !- Y Origin
  0,                        !- Z Origin
  1,                        !- Type
  1,                        !- Multiplier
  3,                        !- Ceiling Height
  300;                      !- Volume
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	preview := PreviewApplyProfile(doc, ProfileApplyRequest{
		SourceObjectIndexes:   []int{7, 8},
		TargetZoneNames:       []string{"Copy Target"},
		Mode:                  "clone",
		ReplaceExistingPolicy: "replace",
	})
	if !preview.CanApply {
		t.Fatalf("preview cannot apply: %#v", preview.Warnings)
	}
	if len(preview.Changes) != 2 {
		t.Fatalf("preview changes = %d, want 2: %#v", len(preview.Changes), preview.Changes)
	}

	updated, applyPreview := ApplyProfile(doc, ProfileApplyRequest{
		SourceObjectIndexes:   []int{7, 8},
		TargetZoneNames:       []string{"Copy Target"},
		Mode:                  "clone",
		ReplaceExistingPolicy: "replace",
	})
	if !applyPreview.CanApply {
		t.Fatalf("apply cannot apply: %#v", applyPreview.Warnings)
	}
	foundPeople := false
	foundLights := false
	for _, obj := range updated.Objects {
		if strings.EqualFold(obj.Type, "People") && strings.EqualFold(profileTargetName(obj), "Copy Target") {
			foundPeople = true
		}
		if strings.EqualFold(obj.Type, "Lights") && strings.EqualFold(profileTargetName(obj), "Copy Target") {
			foundLights = true
		}
	}
	if !foundPeople || !foundLights {
		t.Fatalf("updated document missing cloned profile objects: people=%v lights=%v", foundPeople, foundLights)
	}
}

func findProfileSeries(series []ProfileGraphSeries, scopeType string, zoneName string, dimension string) *ProfileGraphSeries {
	for index := range series {
		if series[index].ScopeType == scopeType && series[index].ZoneName == zoneName && series[index].Dimension == dimension {
			return &series[index]
		}
	}
	return nil
}

func findZoneProfile(profiles []ZoneProfile, zoneName string) *ZoneProfile {
	for index := range profiles {
		if strings.EqualFold(profiles[index].ZoneName, zoneName) {
			return &profiles[index]
		}
	}
	return nil
}

func profileDimensionSummary(dimensions []ProfileDimensionSummary, dimension string) *ProfileDimensionSummary {
	for index := range dimensions {
		if dimensions[index].Dimension == dimension {
			return &dimensions[index]
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertProfileDimension(t *testing.T, zone ZoneProfile, dimension string, want float64, tolerance float64) {
	t.Helper()
	for _, item := range zone.Dimensions {
		if item.Dimension != dimension {
			continue
		}
		if math.Abs(item.Value-want) > tolerance {
			t.Fatalf("%s profile = %v, want %v +/- %v", dimension, item.Value, want, tolerance)
		}
		return
	}
	t.Fatalf("dimension %q not found in zone profile: %#v", dimension, zone.Dimensions)
}
