package idf

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestGeometryMetricUsesEmDashForMissingDisplayValue(t *testing.T) {
	metric := geometryMetric("Missing", "", "", 2)
	if metric.DisplayValue != "—" {
		t.Fatalf("missing geometry metric display = %q, want em dash", metric.DisplayValue)
	}
}

func TestAnalyzeGeometryBuildsZonesSurfacesWindowsAndStories(t *testing.T) {
	doc, err := Parse(metricsFixtureIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	geometry := AnalyzeGeometry(doc)
	if geometry.ZoneCount != 1 {
		t.Fatalf("zone count = %d, want 1", geometry.ZoneCount)
	}
	if geometry.SurfaceCount != 6 {
		t.Fatalf("surface count = %d, want 6", geometry.SurfaceCount)
	}
	if geometry.WindowCount != 2 {
		t.Fatalf("window count = %d, want 2", geometry.WindowCount)
	}
	if len(geometry.Stories) != 1 {
		t.Fatalf("story count = %d, want 1", len(geometry.Stories))
	}
	if !geometry.Bounds.OK {
		t.Fatalf("geometry bounds were not populated")
	}

	zone := geometry.Zones[0]
	if zone.FloorArea != 200 {
		t.Fatalf("zone floor area = %v, want 200", zone.FloorArea)
	}
	if len(zone.SurfaceIDs) != 6 {
		t.Fatalf("zone surface ids = %d, want 6", len(zone.SurfaceIDs))
	}
	if len(zone.WindowIDs) != 2 {
		t.Fatalf("zone window ids = %d, want 2", len(zone.WindowIDs))
	}

	southWindow := findGeometryWindow(t, geometry, "South Window")
	if southWindow.BaseSurfaceName != "South Wall" {
		t.Fatalf("south window base = %q, want South Wall", southWindow.BaseSurfaceName)
	}
	if southWindow.Area != 2 {
		t.Fatalf("south window area = %v, want 2", southWindow.Area)
	}
	if southWindow.PhysicalArea != 2 || southWindow.EffectiveArea != 2 || southWindow.AreaBasis != "effective" {
		t.Fatalf("south window area basis = physical %v effective %v basis %q", southWindow.PhysicalArea, southWindow.EffectiveArea, southWindow.AreaBasis)
	}
	if southWindow.Orientation != "south" {
		t.Fatalf("south window orientation = %q, want south", southWindow.Orientation)
	}
}

func TestAnalyzeGeometrySeparatesPhysicalAndEffectiveArea(t *testing.T) {
	multipliedIDF := strings.Replace(
		metricsFixtureIDF,
		"  1,                       !- Multiplier\n  3,                       !- Ceiling Height",
		"  10,                      !- Multiplier\n  3,                       !- Ceiling Height",
		1,
	)
	multipliedIDF = strings.Replace(
		multipliedIDF,
		"  ,                        !- Frame and Divider Name\n  1,                       !- Multiplier",
		"  ,                        !- Frame and Divider Name\n  2,                       !- Multiplier",
		1,
	)
	doc, err := Parse(multipliedIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	geometry := AnalyzeGeometry(doc)
	southWall := findGeometrySurface(t, geometry, "South Wall")
	if southWall.PhysicalArea != 30 || southWall.EffectiveArea != 300 || southWall.Area != southWall.EffectiveArea {
		t.Fatalf("south wall areas = physical %v effective %v legacy %v", southWall.PhysicalArea, southWall.EffectiveArea, southWall.Area)
	}
	if southWall.ZoneMultiplier != 10 || southWall.SurfaceMultiplier != 1 || southWall.AreaBasis != "effective" {
		t.Fatalf("south wall area metadata = zone %v surface %v basis %q", southWall.ZoneMultiplier, southWall.SurfaceMultiplier, southWall.AreaBasis)
	}

	southWindow := findGeometryWindow(t, geometry, "South Window")
	if southWindow.PhysicalArea != 4 || southWindow.EffectiveArea != 40 || southWindow.Area != southWindow.EffectiveArea {
		t.Fatalf("south window areas = physical %v effective %v legacy %v", southWindow.PhysicalArea, southWindow.EffectiveArea, southWindow.Area)
	}
	if southWindow.ZoneMultiplier != 10 || southWindow.SurfaceMultiplier != 2 || southWindow.Multiplier != 2 {
		t.Fatalf("south window multiplier metadata = zone %v surface %v legacy %v", southWindow.ZoneMultiplier, southWindow.SurfaceMultiplier, southWindow.Multiplier)
	}
}

func TestAnalyzeGeometryTransformsRelativeVerticesToWorldCoordinates(t *testing.T) {
	relativeDocument, err := Parse(geometryCoordinateFixture(
		"Relative",
		"0,0,0, 2,0,0, 2,0,2, 0,0,2",
		"0.5,0,0.5, 1.5,0,0.5, 1.5,0,1.5, 0.5,0,1.5",
	))
	if err != nil {
		t.Fatalf("Parse(relative) error = %v", err)
	}
	worldDocument, err := Parse(geometryCoordinateFixture(
		"World",
		"-20,10,3, -20,12,3, -20,12,5, -20,10,5",
		"-20,10.5,3.5, -20,11.5,3.5, -20,11.5,4.5, -20,10.5,4.5",
	))
	if err != nil {
		t.Fatalf("Parse(world) error = %v", err)
	}

	relative := AnalyzeGeometry(relativeDocument)
	world := AnalyzeGeometry(worldDocument)
	relativeSurface := findGeometrySurface(t, relative, "Space Wall")
	worldSurface := findGeometrySurface(t, world, "Space Wall")
	if relativeSurface.ZoneName != "Zone A" || relativeSurface.SpaceName != "Space A" {
		t.Fatalf("space-owned surface resolved to zone %q space %q", relativeSurface.ZoneName, relativeSurface.SpaceName)
	}
	if reflect.DeepEqual(relativeSurface.RawVertices, relativeSurface.WorldVertices) {
		t.Fatal("relative raw vertices must remain distinct from transformed world vertices")
	}
	if !reflect.DeepEqual(relativeSurface.WorldVertices, worldSurface.WorldVertices) {
		t.Fatalf("relative/world surface mismatch:\nrelative %#v\nworld %#v", relativeSurface.WorldVertices, worldSurface.WorldVertices)
	}
	if !reflect.DeepEqual(relativeSurface.Vertices, relativeSurface.WorldVertices) {
		t.Fatal("legacy surface vertices must alias world vertices")
	}

	relativeWindow := findGeometryWindow(t, relative, "Space Window")
	worldWindow := findGeometryWindow(t, world, "Space Window")
	if !reflect.DeepEqual(relativeWindow.WorldVertices, worldWindow.WorldVertices) {
		t.Fatalf("relative/world window mismatch:\nrelative %#v\nworld %#v", relativeWindow.WorldVertices, worldWindow.WorldVertices)
	}
	if !reflect.DeepEqual(relativeWindow.Vertices, relativeWindow.WorldVertices) {
		t.Fatal("legacy window vertices must alias world vertices")
	}
	if !reflect.DeepEqual(relative.Bounds, world.Bounds) {
		t.Fatalf("relative/world bounds mismatch: relative %#v world %#v", relative.Bounds, world.Bounds)
	}
	if relativeSurface.PhysicalArea != worldSurface.PhysicalArea || relativeWindow.PhysicalArea != worldWindow.PhysicalArea {
		t.Fatalf("relative/world physical area mismatch: surface %v/%v window %v/%v", relativeSurface.PhysicalArea, worldSurface.PhysicalArea, relativeWindow.PhysicalArea, worldWindow.PhysicalArea)
	}
}

func TestHeatTransferSurfaceAdapterCoversEnergyPlusSurfaceFamilies(t *testing.T) {
	required := []string{
		"BuildingSurface:Detailed",
		"Wall:Detailed",
		"RoofCeiling:Detailed",
		"Floor:Detailed",
		"Wall:Exterior",
		"Wall:Adiabatic",
		"Wall:Underground",
		"Ceiling:Adiabatic",
		"Ceiling:Interzone",
		"Floor:GroundContact",
		"Floor:Adiabatic",
		"Floor:Interzone",
		"Roof",
	}
	supported := heatTransferSurfaceObjectTypes()
	for _, objectType := range required {
		if !containsStringFold(supported, objectType) {
			t.Errorf("heat-transfer adapter does not cover %q; supported = %v", objectType, supported)
		}
	}
}

func TestAnalyzeGeometryConvertsEquivalentRectangularAndDetailedSurfaces(t *testing.T) {
	doc, err := Parse(rectangularGeometryFixture)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	geometry := AnalyzeGeometry(doc)
	simple := findGeometrySurface(t, geometry, "Simple Wall")
	detailed := findGeometrySurface(t, geometry, "Detailed Wall")
	if simple.VerticesSource != "generated_rectangular_geometry" {
		t.Fatalf("simple vertices source = %q", simple.VerticesSource)
	}
	if simple.PhysicalArea != 12 || detailed.PhysicalArea != 12 {
		t.Fatalf("equivalent wall physical areas = simple %v detailed %v", simple.PhysicalArea, detailed.PhysicalArea)
	}
	if !reflect.DeepEqual(simple.WorldVertices, detailed.WorldVertices) {
		t.Fatalf("equivalent simple/detailed polygons differ:\nsimple %#v\ndetailed %#v", simple.WorldVertices, detailed.WorldVertices)
	}
	if simple.OutsideBoundary != "Outdoors" || detailed.OutsideBoundary != "Outdoors" {
		t.Fatalf("equivalent wall boundaries = simple %q detailed %q", simple.OutsideBoundary, detailed.OutsideBoundary)
	}

	window := findGeometryWindow(t, geometry, "Simple Window")
	if window.VerticesSource != "generated_rectangular_opening" || window.PhysicalArea != 2 {
		t.Fatalf("simple window = source %q physical area %v", window.VerticesSource, window.PhysicalArea)
	}
}

func TestAnalyzeGeometryKeepsShadingForSpatialViews(t *testing.T) {
	doc, err := Parse(rectangularGeometryFixture)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	geometry := AnalyzeGeometry(doc)
	for _, name := range []string{"Site Shade", "Attached Shade"} {
		shade := findGeometrySurface(t, geometry, name)
		if !shade.IsShading || shade.SurfaceType != "Shading" {
			t.Fatalf("shade %q was not marked as shading: %#v", name, shade)
		}
		if len(shade.WorldVertices) != 4 || shade.PhysicalArea <= 0 {
			t.Fatalf("shade %q has invalid spatial polygon: %#v", name, shade.WorldVertices)
		}
	}
	if attached := findGeometrySurface(t, geometry, "Attached Shade"); attached.ZoneName != "Zone A" {
		t.Fatalf("attached shade zone = %q, want Zone A", attached.ZoneName)
	}
}

func containsStringFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

const rectangularGeometryFixture = `
Version,22.1;
GlobalGeometryRules,
  UpperLeftCorner,
  CounterClockWise,
  World,
  World,
  World;
Zone,
  Zone A,
  0,
  0,
  0,
  0,
  1,
  1;
Wall:Exterior,
  Simple Wall,
  Wall Construction,
  Zone A,
  ,
  180,
  90,
  4,
  0,
  0,
  4,
  3;
BuildingSurface:Detailed,
  Detailed Wall,
  Wall,
  Wall Construction,
  Zone A,
  ,
  Outdoors,
  ,
  SunExposed,
  WindExposed,
  0.5,
  4,
  4,0,0,
  4,0,3,
  0,0,3,
  0,0,0;
Window,
  Simple Window,
  Window Construction,
  Simple Wall,
  ,
  1,
  1,
  1,
  2,
  1;
Shading:Site:Detailed,
  Site Shade,
  ,
  4,
  -2,-2,3,
  -2,-2,0,
  2,-2,0,
  2,-2,3;
Shading:Zone:Detailed,
  Attached Shade,
  Simple Wall,
  ,
  4,
  4,0,2.5,
  4,-1,2.5,
  0,-1,2.5,
  0,0,2.5;
`

func geometryCoordinateFixture(coordinateSystem string, surfaceVertices string, windowVertices string) string {
	return fmt.Sprintf(`
GlobalGeometryRules,
  UpperLeftCorner,         !- Starting Vertex Position
  CounterClockWise,        !- Vertex Entry Direction
  %s;                      !- Coordinate System
Building,
  Transform Test,          !- Name
  90;                      !- North Axis
Zone,
  Zone A,                  !- Name
  0,                       !- Direction of Relative North
  10,                      !- X Origin
  20,                      !- Y Origin
  3,                       !- Z Origin
  1,                       !- Type
  1;                       !- Multiplier
Space,
  Space A,                 !- Name
  Zone A;                  !- Zone Name
BuildingSurface:Detailed,
  Space Wall,              !- Name
  Wall,                    !- Surface Type
  Wall Construction,       !- Construction Name
  ,                        !- Zone Name
  Space A,                 !- Space Name
  Outdoors,                !- Outside Boundary Condition
  ,                        !- Outside Boundary Condition Object
  SunExposed,              !- Sun Exposure
  WindExposed,             !- Wind Exposure
  0.5,                     !- View Factor to Ground
  4,                       !- Number of Vertices
  %s;                      !- Vertex 1 X-coordinate
FenestrationSurface:Detailed,
  Space Window,            !- Name
  Window,                  !- Surface Type
  Window Construction,     !- Construction Name
  Space Wall,              !- Building Surface Name
  0.5,                     !- View Factor to Ground
  ,                        !- Frame and Divider Name
  1,                       !- Multiplier
  4,                       !- Number of Vertices
  %s;                      !- Vertex 1 X-coordinate
`, coordinateSystem, surfaceVertices, windowVertices)
}

func findGeometryWindow(t *testing.T, geometry GeometryReport, name string) GeometryWindow {
	t.Helper()
	for _, window := range geometry.Windows {
		if window.Name == name {
			return window
		}
	}
	t.Fatalf("window %q not found", name)
	return GeometryWindow{}
}

func findGeometrySurface(t *testing.T, geometry GeometryReport, name string) GeometrySurface {
	t.Helper()
	for _, surface := range geometry.Surfaces {
		if surface.Name == name {
			return surface
		}
	}
	t.Fatalf("surface %q not found", name)
	return GeometrySurface{}
}
