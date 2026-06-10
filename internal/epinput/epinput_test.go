package epinput

import (
	"strings"
	"testing"
)

const idfV22 = `
Version,
  22.2;                    !- Version Identifier

Zone,
  Office;                  !- Name
`

const epjsonV22 = `{
  "Version": {
    "Version 1": {
      "version_identifier": "22.2",
      "idf_order": 1
    }
  },
  "Zone": {
    "Office": {
      "direction_of_relative_north": 0,
      "idf_order": 2
    }
  },
  "Fan:ConstantVolume": {
    "Supply Fan": {
      "air_inlet_node_name": "Air Inlet Node",
      "air_outlet_node_name": "Air Outlet Node",
      "idf_order": 3
    }
  }
}`

func TestDetectFormat(t *testing.T) {
	if got := DetectFormat("model.idf", []byte("{}")); got != FormatIDF {
		t.Fatalf("idf extension format = %s, want idf", got)
	}
	if got := DetectFormat("model.epJSON", nil); got != FormatEPJSON {
		t.Fatalf("epJSON extension format = %s, want epjson", got)
	}
	if got := DetectFormat("", []byte("  {")); got != FormatEPJSON {
		t.Fatalf("json content format = %s, want epjson", got)
	}
}

func TestParseIDFDetectsSupportedVersionAndWritesEPJSON(t *testing.T) {
	model, err := Parse("model.idf", []byte(idfV22))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if model.Format != FormatIDF {
		t.Fatalf("format = %s, want idf", model.Format)
	}
	if !model.Version.Supported || model.Version.Major != 22 {
		t.Fatalf("version = %#v, want supported 22.x", model.Version)
	}

	epjson, err := Write(model, FormatEPJSON)
	if err != nil {
		t.Fatalf("Write(epjson) error = %v", err)
	}
	for _, want := range []string{`"Version"`, `"version_identifier": "22.2"`, `"Zone"`, `"Office"`} {
		if !strings.Contains(epjson, want) {
			t.Fatalf("epJSON output missing %q:\n%s", want, epjson)
		}
	}
}

func TestParseEPJSONDetectsVersionAndWritesIDF(t *testing.T) {
	model, err := Parse("model.epJSON", []byte(epjsonV22))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if model.Format != FormatEPJSON {
		t.Fatalf("format = %s, want epjson", model.Format)
	}
	if !model.Version.Supported || model.Version.Raw != "22.2" {
		t.Fatalf("version = %#v, want supported 22.2", model.Version)
	}

	idfText, err := Write(model, FormatIDF)
	if err != nil {
		t.Fatalf("Write(idf) error = %v", err)
	}
	for _, want := range []string{"Version,", "22.2", "Zone,", "Office", "Fan:ConstantVolume", "Air Inlet Node"} {
		if !strings.Contains(idfText, want) {
			t.Fatalf("IDF output missing %q:\n%s", want, idfText)
		}
	}
}

func TestOutputControlObjectsRoundTripWithoutSyntheticName(t *testing.T) {
	model, err := Parse("model.idf", []byte(`Version,
  24.2;                    !- Version Identifier

OutputControl:Table:Style,
  Comma,                   !- Column Separator
  JtoKWH;                  !- Unit Conversion
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	idfText, err := Write(model, FormatIDF)
	if err != nil {
		t.Fatalf("Write(idf) error = %v", err)
	}
	if strings.Contains(idfText, "OutputControl:Table:Style 1") {
		t.Fatalf("IDF output contains a synthetic OutputControl name:\n%s", idfText)
	}
	for _, want := range []string{"OutputControl:Table:Style,", "Comma,", "JtoKWH;"} {
		if !strings.Contains(idfText, want) {
			t.Fatalf("IDF output missing %q:\n%s", want, idfText)
		}
	}
}

func TestRejectsPre22VersionWhenKnown(t *testing.T) {
	_, err := Parse("old.idf", []byte("Version,\n  9.6; !- Version Identifier\n"))
	if err == nil {
		t.Fatalf("Parse() error = nil, want unsupported version error")
	}
	if !strings.Contains(err.Error(), "version 22 or newer") {
		t.Fatalf("error = %q, want supported range message", err)
	}
}

func TestPatchFieldValueUpdatesRootAndNestedValues(t *testing.T) {
	model, err := Parse("model.epJSON", []byte(`{
  "Version": {
    "Version 1": {
      "version_identifier": "22.2",
      "idf_order": 1
    }
  },
  "Zone": {
    "Office": {
      "direction_of_relative_north": 0,
      "metadata": {"tags": ["old"]},
      "idf_order": 2
    }
  }
}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if err := PatchFieldValue(model, 1, 0, nil, "15"); err != nil {
		t.Fatalf("PatchFieldValue(root) error = %v", err)
	}
	if err := PatchFieldValue(model, 1, 1, []string{"tags", "0"}, `"new"`); err != nil {
		t.Fatalf("PatchFieldValue(nested) error = %v", err)
	}

	epjson, err := Write(model, FormatEPJSON)
	if err != nil {
		t.Fatalf("Write(epjson) error = %v", err)
	}
	for _, want := range []string{`"direction_of_relative_north": 15`, `"metadata": {"tags": ["new"]}`} {
		if !strings.Contains(epjson, want) {
			t.Fatalf("patched epJSON missing %q:\n%s", want, epjson)
		}
	}
}

func TestDetailedSurfaceVerticesUseEPJSONArray(t *testing.T) {
	model, err := Parse("surface.idf", []byte(`Version,
  22.2;                    !- Version Identifier

BuildingSurface:Detailed,
  Wall 1,                   !- Name
  Wall,                     !- Surface Type
  Basic Wall,               !- Construction Name
  Zone 1,                   !- Zone Name
  Outdoors,                 !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  SunExposed,               !- Sun Exposure
  WindExposed,              !- Wind Exposure
  0.5,                      !- View Factor to Ground
  4,                        !- Number of Vertices
  0,                        !- Vertex 1 X-coordinate
  0,                        !- Vertex 1 Y-coordinate
  0,                        !- Vertex 1 Z-coordinate
  10,                       !- Vertex 2 X-coordinate
  0,                        !- Vertex 2 Y-coordinate
  0,                        !- Vertex 2 Z-coordinate
  10,                       !- Vertex 3 X-coordinate
  3,                        !- Vertex 3 Y-coordinate
  0,                        !- Vertex 3 Z-coordinate
  0,                        !- Vertex 4 X-coordinate
  3,                        !- Vertex 4 Y-coordinate
  0;                        !- Vertex 4 Z-coordinate
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	epjson, err := Write(model, FormatEPJSON)
	if err != nil {
		t.Fatalf("Write(epjson) error = %v", err)
	}
	for _, want := range []string{`"vertices": [`, `"vertex_x_coordinate": 0`, `"vertex_y_coordinate": 3`} {
		if !strings.Contains(epjson, want) {
			t.Fatalf("epJSON output missing %q:\n%s", want, epjson)
		}
	}
	if strings.Contains(epjson, "vertex_1_x_coordinate") {
		t.Fatalf("epJSON output kept flat vertex fields:\n%s", epjson)
	}
}

func TestEPJSONVerticesArrayWritesIDFVertexFields(t *testing.T) {
	model, err := Parse("surface.epJSON", []byte(`{
  "Version": {
    "Version 1": {
      "version_identifier": "22.2",
      "idf_order": 1
    }
  },
  "BuildingSurface:Detailed": {
    "Wall 1": {
      "surface_type": "Wall",
      "construction_name": "Basic Wall",
      "zone_name": "Zone 1",
      "number_of_vertices": 2,
      "vertices": [
        {"vertex_x_coordinate": 0, "vertex_y_coordinate": 0, "vertex_z_coordinate": 0},
        {"vertex_x_coordinate": 10, "vertex_y_coordinate": 0, "vertex_z_coordinate": 0}
      ],
      "idf_order": 2
    }
  }
}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	idfText, err := Write(model, FormatIDF)
	if err != nil {
		t.Fatalf("Write(idf) error = %v", err)
	}
	for _, want := range []string{"Wall 1", "Vertex 1 X-coordinate", "Vertex 2 Z-coordinate"} {
		if !strings.Contains(idfText, want) {
			t.Fatalf("IDF output missing %q:\n%s", want, idfText)
		}
	}
}
