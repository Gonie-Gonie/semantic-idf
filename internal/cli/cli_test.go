package cli

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cliFixtureIDF = `
Version,
  24.1;

Building,
  CLI Building;

Zone,
  Office;

Zone,
  Office;
`

const cliProfileFixtureIDF = `
Version,
  24.1;

Schedule:Constant,
  AlwaysOn,                 !- Name
  Fraction,                 !- Schedule Type Limits Name
  1;                        !- Hourly Value

Zone,
  Office;

BuildingSurface:Detailed,
  Office Floor,             !- Name
  Floor,                    !- Surface Type
  Floor Construction,       !- Construction Name
  Office,                   !- Zone Name
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
  Office Lights,            !- Name
  Office,                   !- Zone or ZoneList Name
  AlwaysOn,                 !- Schedule Name
  Watts/Area,               !- Design Level Calculation Method
  ,                         !- Lighting Level
  12;                       !- Watts per Zone Floor Area
`

const cliHVACFixtureIDF = `
Version,
  24.1;

Zone,
  Office;

ZoneHVAC:EquipmentConnections,
  Office,
  Office Equipment,
  Office Supply Inlet,
  ,
  Office Zone Air Node,
  ;

ZoneHVAC:EquipmentList,
  Office Equipment,
  ZoneHVAC:PackagedTerminalHeatPump,
  Office PTHP,
  1,
  1;

ZoneHVAC:PackagedTerminalHeatPump,
  Office PTHP,
  ,
  Autosize,
  Office Supply Inlet,
  Office Zone Air Node;
`

func TestCLISummaryWritesText(t *testing.T) {
	input := writeCLITestInput(t, cliFixtureIDF)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runCLI([]string{"summary", "-format", "text", input}, strings.NewReader(""), &stdout, &stderr, "0.4.1")
	if code != 0 {
		t.Fatalf("runCLI summary exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Building name: CLI Building") {
		t.Fatalf("summary output missing building name:\n%s", stdout.String())
	}
}

func TestCLIConvertYAML(t *testing.T) {
	input := writeCLITestInput(t, cliFixtureIDF)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runCLI([]string{"convert", "-to", "yaml", input}, strings.NewReader(""), &stdout, &stderr, "0.4.1")
	if code != 0 {
		t.Fatalf("runCLI convert yaml exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "semantic_energyplus_model:") || !strings.Contains(stdout.String(), "source_name_conflicts:") {
		t.Fatalf("semantic YAML output missing expected content:\n%s", stdout.String())
	}
}

func TestCLICleanSemanticDuplicates(t *testing.T) {
	input := writeCLITestInput(t, cliFixtureIDF)
	output := filepath.Join(t.TempDir(), "cleaned.idf")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runCLI([]string{"clean", "-rules", "none", "-semantic-duplicates", "-o", output, input}, strings.NewReader(""), &stdout, &stderr, "0.4.1")
	if code != 0 {
		t.Fatalf("runCLI clean exit = %d, stderr = %s", code, stderr.String())
	}
	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read cleaned output: %v", err)
	}
	if !strings.Contains(string(content), "Office 2") {
		t.Fatalf("cleaned output did not rename duplicate zone:\n%s", string(content))
	}
}

func TestCLIProfileExports(t *testing.T) {
	input := writeCLITestInput(t, cliProfileFixtureIDF)

	var graphOut bytes.Buffer
	var graphErr bytes.Buffer
	code := runCLI([]string{"profile-graph", "-format", "json", input}, strings.NewReader(""), &graphOut, &graphErr, "0.4.1")
	if code != 0 {
		t.Fatalf("runCLI profile-graph exit = %d, stderr = %s", code, graphErr.String())
	}
	if !strings.Contains(graphOut.String(), `"series"`) || !strings.Contains(graphOut.String(), `"metricModes"`) {
		t.Fatalf("profile-graph output missing graph fields:\n%s", graphOut.String())
	}

	var qaOut bytes.Buffer
	var qaErr bytes.Buffer
	code = runCLI([]string{"profile-qa", "-format", "text", input}, strings.NewReader(""), &qaOut, &qaErr, "0.4.1")
	if code != 0 {
		t.Fatalf("runCLI profile-qa exit = %d, stderr = %s", code, qaErr.String())
	}
	if !strings.Contains(qaOut.String(), "Profile QA outliers") {
		t.Fatalf("profile-qa text missing heading:\n%s", qaOut.String())
	}

	var schedulesOut bytes.Buffer
	var schedulesErr bytes.Buffer
	code = runCLI([]string{"profile-schedules", "-format", "csv", input}, strings.NewReader(""), &schedulesOut, &schedulesErr, "0.4.1")
	if code != 0 {
		t.Fatalf("runCLI profile-schedules exit = %d, stderr = %s", code, schedulesErr.String())
	}
	if !strings.Contains(schedulesOut.String(), "AlwaysOn") || !strings.Contains(schedulesOut.String(), "operating_hours") {
		t.Fatalf("profile-schedules csv missing schedule:\n%s", schedulesOut.String())
	}
}

func TestCLIHVACGraphExports(t *testing.T) {
	input := writeCLITestInput(t, cliHVACFixtureIDF)

	var serviceOut bytes.Buffer
	var serviceErr bytes.Buffer
	code := runCLI([]string{"hvac-graph", "--graph", "service", "-format", "json", input}, strings.NewReader(""), &serviceOut, &serviceErr, "0.4.1")
	if code != 0 {
		t.Fatalf("runCLI hvac-graph service exit = %d, stderr = %s", code, serviceErr.String())
	}
	for _, want := range []string{
		`"schema": "semantic-idf.hvac.graph.v1"`,
		`"graph": "service"`,
		`"serviceModel"`,
		`"navigation"`,
		`"zoneServices"`,
	} {
		if !strings.Contains(serviceOut.String(), want) {
			t.Fatalf("hvac-graph service JSON missing %q:\n%s", want, serviceOut.String())
		}
	}

	var ruleOut bytes.Buffer
	var ruleErr bytes.Buffer
	code = runCLI([]string{"hvac-graph", "--graph", "rule", "-format", "json", input}, strings.NewReader(""), &ruleOut, &ruleErr, "0.4.1")
	if code != 0 {
		t.Fatalf("runCLI hvac-graph rule exit = %d, stderr = %s", code, ruleErr.String())
	}
	if !strings.Contains(ruleOut.String(), `"graph": "rule"`) || !strings.Contains(ruleOut.String(), `"ruleGraph"`) {
		t.Fatalf("hvac-graph rule JSON missing graph payload:\n%s", ruleOut.String())
	}

	var couplingOut bytes.Buffer
	var couplingErr bytes.Buffer
	code = runCLI([]string{"hvac-graph", "--graph", "coupling", "-format", "text", input}, strings.NewReader(""), &couplingOut, &couplingErr, "0.4.1")
	if code != 0 {
		t.Fatalf("runCLI hvac-graph coupling exit = %d, stderr = %s", code, couplingErr.String())
	}
	if !strings.Contains(couplingOut.String(), "HVAC graph: coupling") || !strings.Contains(couplingOut.String(), "Navigation entities:") {
		t.Fatalf("hvac-graph coupling text missing summary:\n%s", couplingOut.String())
	}
}

func TestCLIConvertTableXLSX(t *testing.T) {
	input := writeCLITestInput(t, cliFixtureIDF)
	output := filepath.Join(t.TempDir(), "tables.xlsx")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runCLI([]string{"convert", "-to", "table", "-o", output, input}, strings.NewReader(""), &stdout, &stderr, "0.4.1")
	if code != 0 {
		t.Fatalf("runCLI convert table exit = %d, stderr = %s", code, stderr.String())
	}
	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("open xlsx zip: %v", err)
	}
	defer archive.Close()
	foundSheet := false
	foundStyles := false
	for _, file := range archive.File {
		if file.Name == "xl/worksheets/sheet1.xml" {
			foundSheet = true
			text := readZipFileText(t, file)
			if !strings.Contains(text, "[Zone]") || !strings.Contains(text, "object_index") {
				t.Fatalf("sheet XML missing table markers/header:\n%s", text)
			}
		}
		if file.Name == "xl/styles.xml" {
			foundStyles = true
			text := readZipFileText(t, file)
			if !strings.Contains(text, `<b/>`) || !strings.Contains(text, `patternType="solid"`) {
				t.Fatalf("styles XML missing bold/fill styling:\n%s", text)
			}
		}
	}
	if !foundSheet || !foundStyles {
		t.Fatalf("xlsx entries found sheet=%t styles=%t", foundSheet, foundStyles)
	}
}

func writeCLITestInput(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model.idf")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func readZipFileText(t *testing.T, file *zip.File) string {
	t.Helper()
	reader, err := file.Open()
	if err != nil {
		t.Fatalf("open zip entry %s: %v", file.Name, err)
	}
	defer reader.Close()
	var b bytes.Buffer
	if _, err := b.ReadFrom(reader); err != nil {
		t.Fatalf("read zip entry %s: %v", file.Name, err)
	}
	return b.String()
}
