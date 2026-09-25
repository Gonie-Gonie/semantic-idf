package idf

import (
	"fmt"
	"reflect"
	"testing"
)

func TestDetailedVerticesAutocalculateCatalogTail(t *testing.T) {
	want := []point3{{x: 0, y: 0, z: 0}, {x: 4, y: 0, z: 0}, {x: 0, y: 3, z: 0}}
	for _, tc := range []struct {
		objectType string
		countIndex int
	}{
		{"BuildingSurface:Detailed", 10},
		{"Wall:Detailed", 9},
		{"RoofCeiling:Detailed", 9},
		{"Floor:Detailed", 9},
		{"FenestrationSurface:Detailed", 8},
		{"Shading:Site:Detailed", 2},
		{"Shading:Building:Detailed", 2},
		{"Shading:Zone:Detailed", 3},
	} {
		t.Run(tc.objectType, func(t *testing.T) {
			object := Object{Type: tc.objectType, Fields: make([]Field, tc.countIndex+1)}
			object.Fields[0].Value = "Literal"
			object.Fields[tc.countIndex].Value = "  AutoCalculate "
			for _, vertex := range want {
				for _, value := range []float64{vertex.x, vertex.y, vertex.z} {
					object.Fields = append(object.Fields, Field{Value: fmt.Sprint(value)})
				}
			}
			got, ok := detailedVertices(object)
			if !ok || !reflect.DeepEqual(got, want) {
				t.Fatalf("catalog-bound tail = %#v/%v, want %#v", got, ok, want)
			}
			if area, ok := objectArea(object); !ok || area != 6 {
				t.Fatalf("literal triangle area = %v/%v, want 6", area, ok)
			}
		})
	}
}

func TestDetailedVerticesAutocalculateRejectsIncompleteOrNonfiniteTail(t *testing.T) {
	base := Object{Type: "BuildingSurface:Detailed", Fields: make([]Field, 11)}
	base.Fields[0].Value = "Literal"
	base.Fields[10].Value = "autocalculate"
	for _, value := range []string{"0", "0", "0", "4", "0", "0", "0", "3", "0"} {
		base.Fields = append(base.Fields, Field{Value: value})
	}
	for _, tc := range []struct {
		name string
		edit func(*Object)
	}{
		{"no_vertices", func(o *Object) { o.Fields = o.Fields[:11] }},
		{"two_vertices", func(o *Object) { o.Fields = o.Fields[:17] }},
		{"incomplete_triple", func(o *Object) { o.Fields = o.Fields[:19] }},
		{"trailing_field", func(o *Object) { o.Fields = append(o.Fields, Field{Value: "1"}) }},
		{"blank_coordinate", func(o *Object) { o.Fields[15].Value = "" }},
		{"nan_coordinate", func(o *Object) { o.Fields[15].Value = "NaN" }},
		{"positive_infinity", func(o *Object) { o.Fields[15].Value = "+Inf" }},
		{"negative_infinity", func(o *Object) { o.Fields[15].Value = "-Inf" }},
		{"overflow_coordinate", func(o *Object) { o.Fields[15].Value = "1e999" }},
		{"autosize_coordinate", func(o *Object) { o.Fields[15].Value = "autosize" }},
		{"unknown_type_comment", func(o *Object) { o.Type = "NotASurface"; o.Fields[10].Comment = "Number of Vertices" }},
		{"wrong_field_autocalculate", func(o *Object) {
			o.Fields[9].Value = "autocalculate"
			o.Fields[9].Comment = "Number of Vertices"
			o.Fields[10].Value = "invalid"
		}},
		{"comment_subset_cannot_hide_tail", func(o *Object) {
			for i := 0; i < 9; i++ {
				o.Fields[11+i].Comment = fmt.Sprintf("Vertex %d %c-coordinate", i/3+1, "XYZ"[i%3])
			}
			o.Fields = append(o.Fields, Field{Value: "unexpected"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			object := base
			object.Fields = append([]Field(nil), base.Fields...)
			tc.edit(&object)
			if got, ok := detailedVertices(object); ok {
				t.Fatalf("invalid autocalculate tail accepted: %#v", got)
			}
		})
	}
}

func TestDetailedVerticesExplicitCountUnchanged(t *testing.T) {
	doc, err := Parse("BuildingSurface:Detailed, Literal, Floor, C, Z, , Ground, , NoSun, NoWind, autocalculate, 3, 0,0,0, 4,0,0, 0,3,0;")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := detailedVertices(doc.Objects[0])
	if !ok || len(got) != 3 {
		t.Fatalf("numeric count with unrelated autocalculate view factor = %#v/%v", got, ok)
	}
}
