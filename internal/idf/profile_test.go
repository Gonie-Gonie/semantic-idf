package idf

import (
	"math"
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
