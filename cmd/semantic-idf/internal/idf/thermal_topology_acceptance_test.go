package idf

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type thermalTopologyAcceptanceManifest struct {
	ModelFamily       string   `json:"modelFamily"`
	SupportedVersions []string `json:"supportedVersions"`
	Expected          struct {
		ZoneCount        int      `json:"zoneCount"`
		SurfaceCount     int      `json:"surfaceCount"`
		OpeningCount     int      `json:"openingCount"`
		ConnectionCount  int      `json:"connectionCount"`
		ExteriorArea     float64  `json:"exteriorArea"`
		GroundArea       float64  `json:"groundArea"`
		InterzoneArea    float64  `json:"interzoneArea"`
		UACoverage       float64  `json:"uaCoverage"`
		DiagnosticCodes  []string `json:"diagnosticCodes"`
		ClosedShellCount int      `json:"closedShellCount"`
		AirCouplingCount int      `json:"airCouplingCount"`
	} `json:"expected"`
}

func TestThermalTopologyAcceptanceCorpus(t *testing.T) {
	directory := filepath.Join("testdata", "thermal_topology")
	manifestPaths, err := filepath.Glob(filepath.Join(directory, "*.expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifestPaths) != 10 {
		t.Fatalf("thermal topology acceptance manifest count = %d, want 10", len(manifestPaths))
	}
	requiredFamilies := map[string]bool{
		"RefBldgLargeOffice":       false,
		"Small Office":             false,
		"Hospital":                 false,
		"Basement/Kiva":            false,
		"Space/SpaceList":          false,
		"AirflowNetwork":           false,
		"Construction:AirBoundary": false,
		"Interior window/door":     false,
		"Zone multiplier":          false,
		"Simple geometry legacy":   false,
	}
	for _, manifestPath := range manifestPaths {
		manifest := readThermalTopologyAcceptanceManifest(t, manifestPath)
		if _, required := requiredFamilies[manifest.ModelFamily]; !required {
			t.Fatalf("unexpected acceptance model family %q", manifest.ModelFamily)
		}
		requiredFamilies[manifest.ModelFamily] = true
		baseName := strings.TrimSuffix(filepath.Base(manifestPath), ".expected.json")
		idfPath := filepath.Join(directory, baseName+".idf")
		content, err := os.ReadFile(idfPath)
		if err != nil {
			t.Fatalf("read %s: %v", idfPath, err)
		}
		if len(manifest.SupportedVersions) == 0 || manifest.SupportedVersions[0] != "22.1" {
			t.Fatalf("%s must exercise the 22.1+ adapter boundary", baseName)
		}
		for _, version := range manifest.SupportedVersions {
			version := version
			t.Run(baseName+"/v"+version, func(t *testing.T) {
				document, err := Parse(string(content))
				if err != nil {
					t.Fatal(err)
				}
				setThermalTopologyAcceptanceVersion(t, &document, version)
				geometry := AnalyzeGeometry(document)
				assertThermalTopologyAcceptance(t, geometry, manifest)
			})
		}
	}
	for family, found := range requiredFamilies {
		if !found {
			t.Errorf("acceptance model family %q is missing", family)
		}
	}
}

func readThermalTopologyAcceptanceManifest(t *testing.T, path string) thermalTopologyAcceptanceManifest {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest thermalTopologyAcceptanceManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return manifest
}

func setThermalTopologyAcceptanceVersion(t *testing.T, document *Document, version string) {
	t.Helper()
	for index := range document.Objects {
		if strings.EqualFold(document.Objects[index].Type, "Version") && len(document.Objects[index].Fields) > 0 {
			document.Objects[index].Fields[0].Value = version
			return
		}
	}
	t.Fatal("acceptance fixture has no Version object")
}

func assertThermalTopologyAcceptance(t *testing.T, geometry GeometryReport, manifest thermalTopologyAcceptanceManifest) {
	t.Helper()
	topology := geometry.Topology
	expected := manifest.Expected
	if geometry.ZoneCount != expected.ZoneCount || topology.Stats.BoundaryCount != expected.SurfaceCount || topology.Stats.OpeningCount != expected.OpeningCount || topology.Stats.ConnectionCount != expected.ConnectionCount {
		t.Errorf("counts = zones %d surfaces %d openings %d connections %d, want %d/%d/%d/%d", geometry.ZoneCount, topology.Stats.BoundaryCount, topology.Stats.OpeningCount, topology.Stats.ConnectionCount, expected.ZoneCount, expected.SurfaceCount, expected.OpeningCount, expected.ConnectionCount)
	}
	exteriorArea, groundArea, interzoneArea := thermalTopologyAcceptanceAreas(topology)
	for label, values := range map[string][2]float64{
		"exterior area":  {exteriorArea, expected.ExteriorArea},
		"ground area":    {groundArea, expected.GroundArea},
		"interzone area": {interzoneArea, expected.InterzoneArea},
		"UA coverage":    {thermalTopologyAcceptanceUACoverage(topology), expected.UACoverage},
	} {
		if math.Abs(values[0]-values[1]) > 1e-6 {
			t.Errorf("%s = %v, want %v", label, values[0], values[1])
		}
	}
	closedShells := 0
	for _, enclosure := range topology.ZoneEnclosures {
		if enclosure.ClosedShell {
			closedShells++
		}
	}
	if closedShells != expected.ClosedShellCount || topology.Stats.AirCouplingCount != expected.AirCouplingCount {
		t.Errorf("closed shells/air couplings = %d/%d, want %d/%d", closedShells, topology.Stats.AirCouplingCount, expected.ClosedShellCount, expected.AirCouplingCount)
	}
	codes := sortedUniqueStrings(thermalTopologyAcceptanceDiagnosticCodes(topology))
	wantCodes := append([]string(nil), expected.DiagnosticCodes...)
	sort.Strings(wantCodes)
	if strings.Join(codes, "\x00") != strings.Join(wantCodes, "\x00") {
		t.Errorf("diagnostic codes = %v, want %v", codes, wantCodes)
	}
	resolvedConnections := 0
	for _, connection := range topology.Connections {
		if connection.QAOnly || strings.Contains(connection.FromNodeID, "unresolved") || strings.Contains(connection.ToNodeID, "unresolved") {
			continue
		}
		resolvedConnections++
	}
	if resolvedConnections == 0 {
		t.Fatal("acceptance topology has no resolved connection")
	}
}

func thermalTopologyAcceptanceAreas(topology ThermalTopologyReport) (float64, float64, float64) {
	var exteriorArea float64
	var groundArea float64
	var interzoneArea float64
	for _, connection := range topology.Connections {
		switch connection.RelationKind {
		case "exterior":
			exteriorArea += connection.EffectiveGrossArea
		case "ground", "ground_preprocessor", "foundation":
			groundArea += connection.EffectiveGrossArea
		case "interzone_explicit_surface", "interzone_implicit_zone", "interspace_implicit":
			interzoneArea += connection.EffectiveGrossArea
		}
	}
	return exteriorArea, groundArea, interzoneArea
}

func thermalTopologyAcceptanceUACoverage(topology ThermalTopologyReport) float64 {
	if len(topology.ZoneSignatures) == 0 {
		return 0
	}
	total := 0.0
	for _, signature := range topology.ZoneSignatures {
		total += signature.UACoverage
	}
	return total / float64(len(topology.ZoneSignatures))
}

func thermalTopologyAcceptanceDiagnosticCodes(topology ThermalTopologyReport) []string {
	codes := make([]string, 0, len(topology.IssueLinks))
	for _, issue := range topology.IssueLinks {
		codes = append(codes, issue.Code)
	}
	return codes
}
