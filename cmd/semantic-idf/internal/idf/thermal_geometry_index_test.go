package idf

import (
	"reflect"
	"strings"
	"testing"
)

func TestThermalOwnerNameKeysPreserveEqualFoldMatching(t *testing.T) {
	names := []string{"", "Zone", "ZONE", " zone", "zone ", "K", "k", "K", "s", "ſ", "Σ", "σ", "ς", "존", "ß", "ss"}
	for _, left := range names {
		for _, right := range names {
			if got, want := thermalOwnerNameKey(left) == thermalOwnerNameKey(right), strings.EqualFold(left, right); got != want {
				t.Errorf("owner keys for %q and %q match = %v, want %v", left, right, got, want)
			}
		}
	}
}

func TestThermalOwnershipIndexPreservesSurfaceOrderAndCentroids(t *testing.T) {
	geometry := GeometryReport{Surfaces: []GeometrySurface{
		{ZoneName: "k", SpaceName: "ſ", WorldVertices: []GeometryPoint{{X: 1e12}, {X: 0.0023, Z: 3}}},
		{ZoneName: " k", SpaceName: "s ", WorldVertices: []GeometryPoint{{X: 100}}},
		{ZoneName: "K", SpaceName: "S", WorldVertices: []GeometryPoint{{X: -1e12}, {Y: 3.1234}}},
		{ZoneName: "K", SpaceName: "s", IsShading: true, WorldVertices: []GeometryPoint{{X: 999}}},
		{ZoneName: "", SpaceName: "", WorldVertices: []GeometryPoint{{X: 3.5}}},
	}}
	index := newThermalGeometryOwnershipIndex(geometry)
	for _, name := range []string{"k", "K", " k", "missing", ""} {
		var wantPositions []int
		var wantPoints []GeometryPoint
		for position, surface := range geometry.Surfaces {
			if !surface.IsShading && strings.EqualFold(surface.ZoneName, name) {
				wantPositions = append(wantPositions, position)
				wantPoints = append(wantPoints, surface.WorldVertices...)
			}
		}
		positions := index.surfacesByZone[thermalOwnerNameKey(name)]
		if !reflect.DeepEqual(positions, wantPositions) {
			t.Errorf("zone %q positions = %v, want %v", name, positions, wantPositions)
		}
		if got, want := thermalIndexedCentroid(geometry.Surfaces, positions), thermalCentroid(wantPoints); got != want {
			t.Errorf("zone %q centroid = %#v, want %#v", name, got, want)
		}
	}
	if got := index.surfacesBySpace[thermalOwnerNameKey("s")]; !reflect.DeepEqual(got, []int{0, 2}) {
		t.Fatalf("space simple-fold positions = %v, want [0 2]", got)
	}
}
