package idf

import "testing"

const sampleIDF = `
Version,
  24.1;                    !- Version Identifier

ScheduleTypeLimits,
  Fraction;                 !- Name

Schedule:Compact,
  AlwaysOn,                 !- Name
  Fraction,                 !- Schedule Type Limits Name
  Through: 12/31,           !- Field 1
  For: AllDays,             !- Field 2
  Until: 24:00,             !- Field 3
  1;                        !- Field 4

Schedule:Compact,
  UnusedSchedule,           !- Name
  Fraction,                 !- Schedule Type Limits Name
  Through: 12/31,           !- Field 1
  For: AllDays,             !- Field 2
  Until: 24:00,             !- Field 3
  0;                        !- Field 4

Zone,
  Office;                   !- Name

Lights,
  Office Lights,            !- Name
  Office,                   !- Zone or ZoneList Name
  AlwaysOn,                 !- Schedule Name
  LightingLevel,            !- Design Level Calculation Method
  500;                      !- Lighting Level

Fan:ConstantVolume,
  Supply Fan,               !- Name
  AlwaysOn,                 !- Availability Schedule Name
  0.7,                      !- Fan Total Efficiency
  500,                      !- Pressure Rise
  1.0,                      !- Maximum Flow Rate
  0.9,                      !- Motor Efficiency
  1.0,                      !- Motor In Airstream Fraction
  Air Inlet Node,           !- Air Inlet Node Name
  Air Outlet Node;          !- Air Outlet Node Name
`

const zoneDetailsIDF = `
Zone,
  Office;                   !- Name

BuildingSurface:Detailed,
  Office Floor,             !- Name
  Floor,                    !- Surface Type
  Office,                   !- Zone Name
  ,                         !- Outside Boundary Condition
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
  Office Lights,            !- Name
  Office,                   !- Zone or ZoneList Name
  AlwaysOn,                 !- Schedule Name
  LightingLevel,            !- Design Level Calculation Method
  500;                      !- Lighting Level
`

func TestParseKeepsFieldComments(t *testing.T) {
	doc, err := Parse(sampleIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(doc.Objects) != 7 {
		t.Fatalf("object count = %d, want 7", len(doc.Objects))
	}
	if got := doc.Objects[2].Fields[0].Value; got != "AlwaysOn" {
		t.Fatalf("schedule name = %q, want AlwaysOn", got)
	}
	if got := doc.Objects[2].Fields[0].Comment; got != "Name" {
		t.Fatalf("schedule comment = %q, want Name", got)
	}
}

func TestAnalyzeFindsSchedulesConnectionsAndUnusedObjects(t *testing.T) {
	doc, err := Parse(sampleIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report := Analyze(doc)

	if report.ObjectCount != 7 {
		t.Fatalf("object count = %d, want 7", report.ObjectCount)
	}
	if len(report.Schedules) != 2 {
		t.Fatalf("schedule count = %d, want 2", len(report.Schedules))
	}
	if len(report.HVACConnections) != 1 {
		t.Fatalf("connection count = %d, want 1", len(report.HVACConnections))
	}
	connection := report.HVACConnections[0]
	if connection.FromNode != "Air Inlet Node" || connection.ToNode != "Air Outlet Node" {
		t.Fatalf("connection = %#v, want inlet to outlet", connection)
	}
	if len(report.UnusedObjects) != 1 {
		t.Fatalf("unused count = %d, want 1: %#v", len(report.UnusedObjects), report.UnusedObjects)
	}
	if report.UnusedObjects[0].Name != "UnusedSchedule" {
		t.Fatalf("unused name = %q, want UnusedSchedule", report.UnusedObjects[0].Name)
	}
}

func TestAnalyzeBuildsZoneDetails(t *testing.T) {
	doc, err := Parse(zoneDetailsIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report := Analyze(doc)

	if len(report.Zones) != 1 {
		t.Fatalf("zone count = %d, want 1", len(report.Zones))
	}
	zone := report.Zones[0]
	if zone.SurfaceCount != 1 || len(zone.Surfaces) != 1 {
		t.Fatalf("zone surfaces = %#v, want one surface", zone.Surfaces)
	}
	if zone.Surfaces[0].Name != "Office Floor" {
		t.Fatalf("surface name = %q, want Office Floor", zone.Surfaces[0].Name)
	}
	if len(zone.RelatedObjects) != 1 || zone.RelatedObjects[0].Name != "Office Lights" {
		t.Fatalf("related objects = %#v, want Office Lights", zone.RelatedObjects)
	}
}

func TestUpdateFieldAndRemoveUnusedObjects(t *testing.T) {
	doc, err := Parse(sampleIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	updated, err := UpdateField(doc, 4, 0, "Core Office")
	if err != nil {
		t.Fatalf("UpdateField() error = %v", err)
	}
	if got := updated.Objects[4].Fields[0].Value; got != "Core Office" {
		t.Fatalf("updated zone = %q, want Core Office", got)
	}
	if got := doc.Objects[4].Fields[0].Value; got != "Office" {
		t.Fatalf("original zone mutated = %q", got)
	}

	trimmed, removed := RemoveUnusedObjects(doc)
	if len(removed) != 1 {
		t.Fatalf("removed count = %d, want 1", len(removed))
	}
	if len(trimmed.Objects) != 6 {
		t.Fatalf("trimmed count = %d, want 6", len(trimmed.Objects))
	}
	roundTrip, err := Parse(trimmed.String())
	if err != nil {
		t.Fatalf("round trip parse error = %v", err)
	}
	if len(roundTrip.Objects) != 6 {
		t.Fatalf("round trip count = %d, want 6", len(roundTrip.Objects))
	}
}

func TestAnalyzeDiagnosticsFindsCommonIssues(t *testing.T) {
	doc, err := Parse(`
Version, 24.1;

Zone,
  Office;                   !- Name

Zone,
  Office;                   !- Name

Lights,
  Bad Lights,               !- Name
  Missing Zone,             !- Zone or ZoneList Name
  Missing Schedule,         !- Schedule Name
  LightingLevel,            !- Design Level Calculation Method
  100;                      !- Lighting Level

BuildingSurface:Detailed,
  Bad Surface,              !- Name
  Wall,                     !- Surface Type
  Missing Zone,             !- Zone Name
  Outdoors,                 !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  SunExposed,               !- Sun Exposure
  WindExposed,              !- Wind Exposure
  0.5,                      !- View Factor to Ground
  3,                        !- Number of Vertices
  0, 0, 0,
  0, 0, 0,
  0, 0, 0;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	diagnostics := AnalyzeDiagnostics(doc)
	assertDiagnosticCode(t, diagnostics, "missing_required_object")
	assertDiagnosticCode(t, diagnostics, "duplicate_name")
	assertDiagnosticCode(t, diagnostics, "missing_schedule_reference")
	assertDiagnosticCode(t, diagnostics, "missing_zone_reference")
	assertDiagnosticCode(t, diagnostics, "zero_area")
}

func TestCleanupPreviewAppliesSelectedRules(t *testing.T) {
	doc, err := Parse(`
Version, 24.1;

Schedule:Compact,
  Used,                     !- Name
  ,                         !- Schedule Type Limits Name
  Through: 12/31,           !- Field 1
  For: AllDays,             !- Field 2
  Until: 24:00,             !- Field 3
  1;                        !- Field 4

Schedule:Compact,
  Unused,                   !- Name
  ,                         !- Schedule Type Limits Name
  Through: 12/31,           !- Field 1
  For: AllDays,             !- Field 2
  Until: 24:00,             !- Field 3
  0;                        !- Field 4

Lights,
  Lights,                   !- Name
  Zone A,                   !- Zone or ZoneList Name
  Used,                     !- Schedule Name
  LightingLevel,            !- Design Level Calculation Method
  100;                      !- Lighting Level

Output:Variable,
  *,                        !- Key Value
  Zone Mean Air Temperature,!- Variable Name
  Hourly;                   !- Reporting Frequency

Output:Variable,
  *,                        !- Key Value
  Zone Mean Air Temperature,!- Variable Name
  Hourly;                   !- Reporting Frequency
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	scan := ScanCleanup(doc)
	if len(scan.Candidates) != 2 {
		t.Fatalf("cleanup candidates = %d, want 2: %#v", len(scan.Candidates), scan.Candidates)
	}
	if scan.Candidates[0].Key == "" {
		t.Fatalf("cleanup candidate key was empty: %#v", scan.Candidates[0])
	}
	if len(scan.Candidates[0].RelatedCodes) == 0 || scan.Candidates[0].Risk == "" || scan.Candidates[0].Source == "" {
		t.Fatalf("cleanup candidate metadata missing: %#v", scan.Candidates[0])
	}
	excludedPreview := PreviewCleanup(doc, []string{CleanupRuleUnusedSchedules, CleanupRuleDuplicateOutputVars}, []string{scan.Candidates[0].Key})
	if excludedPreview.RemovedCount != 1 {
		t.Fatalf("removed count with excluded candidate = %d, want 1", excludedPreview.RemovedCount)
	}
	updated, preview := ApplyCleanup(doc, []string{CleanupRuleUnusedSchedules, CleanupRuleDuplicateOutputVars})
	if preview.RemovedCount != 2 {
		t.Fatalf("removed count = %d, want 2", preview.RemovedCount)
	}
	if len(updated.Objects) != len(doc.Objects)-2 {
		t.Fatalf("updated object count = %d, want %d", len(updated.Objects), len(doc.Objects)-2)
	}
}

func TestCleanupFindsDuplicateOutputManagementObjects(t *testing.T) {
	doc, err := Parse(`
Version, 24.1;

Output:Meter,
  Electricity:Facility,
  Hourly;

Output:Meter,
  Electricity:Facility,
  Hourly;

Output:SQLite,
  SimpleAndTabular;

Output:SQLite,
  SimpleAndTabular;

OutputControl:Table:Style,
  HTML,
  JtoKWH;

OutputControl:Table:Style,
  HTML,
  JtoKWH;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	scan := ScanCleanup(doc)
	if len(scan.Candidates) != 3 {
		t.Fatalf("cleanup candidates = %d, want 3: %#v", len(scan.Candidates), scan.Candidates)
	}
	for _, candidate := range scan.Candidates {
		if candidate.RuleID != CleanupRuleDuplicateOutputVars || candidate.Source != "output" || candidate.Risk != "safe" {
			t.Fatalf("duplicate output candidate metadata = %#v", candidate)
		}
		assertStringSliceContains(t, candidate.RelatedCodes, "duplicate_output_request")
	}
	var duplicateRule *CleanupRule
	for index := range scan.Rules {
		if scan.Rules[index].ID == CleanupRuleDuplicateOutputVars {
			duplicateRule = &scan.Rules[index]
			break
		}
	}
	if duplicateRule == nil || duplicateRule.Group != "output" || !duplicateRule.Default {
		t.Fatalf("duplicate rule = %#v", duplicateRule)
	}
}

func assertStringSliceContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("%q not found in %#v", want, values)
}

func assertDiagnosticCode(t *testing.T, diagnostics []Diagnostic, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("diagnostic code %q not found in %#v", code, diagnostics)
}
