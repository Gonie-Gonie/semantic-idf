package idf

import "testing"

func TestAnalyzeGeometryBuildsZonesSurfacesWindowsAndStories(t *testing.T) {
	doc, err := Parse(summaryFixtureIDF)
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
	if southWindow.Orientation != "south" {
		t.Fatalf("south window orientation = %q, want south", southWindow.Orientation)
	}
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
