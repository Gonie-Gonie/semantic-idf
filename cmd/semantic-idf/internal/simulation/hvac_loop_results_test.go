package simulation

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const hvacTwoLoopInput = `Version,25.1;
AirLoopHVAC,First Loop,,,Autosize,First Branches,,First Inlet,First Return,First Demand,First Outlet;
BranchList,First Branches,First Branch;
Branch,First Branch,,Fan:ConstantVolume,First Fan,First Inlet,Fan Outlet,Coil:Cooling:Water,First Coil,Fan Outlet,First Outlet;
Fan:ConstantVolume,First Fan,,0.7,600,Autosize,0.9,1,First Inlet,Fan Outlet;
AirLoopHVAC,Second Loop,,,Autosize,Second Branches,,Second Inlet,Second Return,Second Demand,Second Outlet;
BranchList,Second Branches,Second Branch;
Branch,Second Branch,,Fan:ConstantVolume,Second Fan,Second Inlet,Second Outlet;
Fan:ConstantVolume,Second Fan,,0.7,600,Autosize,0.9,1,Second Inlet,Second Outlet;
`

func hvacResultTestSeries(key, variable, unit string) SimulationSeries {
	return SimulationSeries{File: "eplusout.sql", KeyValue: key, Name: variable, ReportingFrequency: "Hourly",
		Column: key + ":" + variable + " [" + unit + "](Hourly)", Points: []SimulationPoint{{X: 7, Label: "01-01 01:00", Value: 1}}}
}

func TestHVACLoopResultsUseExecutedTopologyAndExactMembership(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "executed.idf")
	if err := os.WriteFile(inputPath, []byte(hvacTwoLoopInput), 0o644); err != nil {
		t.Fatal(err)
	}
	series := []SimulationSeries{
		hvacResultTestSeries("FIRST INLET", "System Node Temperature", "C"),
		hvacResultTestSeries("Fan Outlet", "System Node Mass Flow Rate", "kg/s"),
		hvacResultTestSeries("First Fan", "Fan Electricity Rate", "W"),
		hvacResultTestSeries("First Coil", "Cooling Coil Total Cooling Rate", "W"),
		hvacResultTestSeries("Second Inlet", "System Node Relative Humidity", "%"),
		hvacResultTestSeries("Second Fan", "Fan Electricity Rate", "W"),
		hvacResultTestSeries("First Fan", "Pump Electricity Rate", "W"),
		hvacResultTestSeries("First Inlet EXTRA", "System Node Temperature", "C"),
	}
	daily := hvacResultTestSeries("First Inlet", "System Node Temperature", "C")
	daily.ReportingFrequency = "Daily"
	series = append(series, daily)
	request := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}}
	bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: inputPath, Series: series}, request)
	if len(bundle.HVACLoops) != 2 {
		t.Fatalf("one result per executed loop required, got %+v", bundle.HVACLoops)
	}
	first, second := bundle.HVACLoops[0], bundle.HVACLoops[1]
	if first.Name != "First Loop" || first.Topology == nil || first.Topology.Name != first.Name || first.Topology.SupplySide.InletNode != "First Inlet" {
		t.Fatalf("executed topology not attached: %+v", first)
	}
	if len(first.Series) != 2 || len(first.Components) != 2 || len(second.Series) != 1 || len(second.Components) != 1 {
		t.Fatalf("loop series were mixed or non-hourly data survived: first=%+v second=%+v", first, second)
	}
	for _, component := range first.Components {
		if component.ComponentName == "First Fan" && (component.ComponentType != "Fan:ConstantVolume" || len(component.Series) != 1) {
			t.Fatalf("component type must come from exact input ownership: %+v", component)
		}
	}
	request.Scope = SimulationPurposeScope{LoopMode: "selected", AirLoopNames: []string{"First Loop"}, ComponentIDs: []string{"Coil:Cooling:Water:First Coil"}}
	bundle = BuildPurposeResultBundle(&SimulationRunResult{InputPath: inputPath, Series: series}, request)
	if len(bundle.HVACLoops) != 1 || len(bundle.HVACLoops[0].Series) != 1 || bundle.HVACLoops[0].Series[0].KeyValue != "Fan Outlet" || len(bundle.HVACLoops[0].Components) != 1 || bundle.HVACLoops[0].Components[0].ComponentName != "First Coil" {
		t.Fatalf("selected component scope broadened: %+v", bundle.HVACLoops)
	}
	if bundle.HVACLoops[0].Topology == nil || len(bundle.HVACLoops[0].Topology.SupplySide.Branches[0].Components) != 2 {
		t.Fatal("selected measurement scope should retain its full loop topology context")
	}
}

func TestHVACComponentOnlyScopeDoesNotRequestOtherEquipment(t *testing.T) {
	doc := parsePurposePlanFixture(t, hvacTwoLoopInput)
	request := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}, Scope: SimulationPurposeScope{ComponentIDs: []string{"Fan:ConstantVolume:Second Fan"}}}
	plan := BuildPurposeRunPlan(doc, request)
	if findPurposeOutput(plan, "Output:Variable", "Second Fan", "Fan Electricity Rate") == nil || findPurposeOutput(plan, "Output:Variable", "*", "Fan Electricity Rate") != nil || findPurposeOutput(plan, "Output:Variable", "First Fan", "Fan Electricity Rate") != nil {
		t.Fatalf("component-only scope must stay exact: %+v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "Second Inlet", "System Node Relative Humidity") == nil {
		t.Fatal("node relative humidity was not requested")
	}
}

func TestHVACDefaultSampleAllLoopScopeIncludesWaterLoops(t *testing.T) {
	inputPath := filepath.Join("..", "..", "frontend", "src", "samples", "RefBldgLargeOfficeNew2004_Chicago.idf")
	doc, err := simulationDocumentFromInput(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"VAV_1": "AirLoopHVAC", "VAV_2": "AirLoopHVAC", "VAV_3": "AirLoopHVAC", "VAV_5": "AirLoopHVAC",
		"CoolSys1": "PlantLoop", "HeatSys1": "PlantLoop", "SWHSys1": "PlantLoop", "TowerWaterSys": "CondenserLoop",
	}
	report := idf.AnalyzeHVAC(doc)
	if len(report.Loops) != len(want) {
		t.Fatalf("default sample must exercise all eight air/water loops, got %d", len(report.Loops))
	}
	request := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}, Scope: SimulationPurposeScope{LoopMode: "all"}}
	plan := BuildPurposeRunPlan(doc, request)
	for _, variable := range []string{"System Node Temperature", "Pump Electricity Rate", "Chiller COP", "Boiler Heating Rate", "Cooling Tower Fan Electricity Rate"} {
		if findPurposeOutput(plan, "Output:Variable", "*", variable) == nil {
			t.Fatalf("all-loop plan omitted air/water loop observations: %s", variable)
		}
	}
	series := []SimulationSeries{}
	for _, loop := range report.Loops {
		if loop.Type != want[loop.Name] || loop.SupplySide.InletNode == "" {
			t.Fatalf("unexpected default sample loop identity/ports: %+v", loop)
		}
		series = append(series, hvacResultTestSeries(loop.SupplySide.InletNode, "System Node Temperature", "C"))
	}
	bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: inputPath, PurposeRunPlan: &plan, Series: series}, request)
	if len(bundle.HVACLoops) != len(want) {
		t.Fatalf("all-loop result lost default sample loops: got %d, want %d", len(bundle.HVACLoops), len(want))
	}
	for _, loop := range bundle.HVACLoops {
		if loop.LoopType != want[loop.Name] || loop.Topology == nil || loop.Topology.Name != loop.Name || len(loop.Series) == 0 {
			t.Fatalf("default sample loop lacks inspectable executed topology or measurements: %+v", loop)
		}
		delete(want, loop.Name)
		t.Logf("%s %s: executed topology and node observations available", loop.LoopType, loop.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing default sample loops: %v", want)
	}
}

func TestHVACNodeDeltaPairsActualFrames(t *testing.T) {
	left := []SimulationPoint{{X: 1, Value: 10}, {X: 2, Value: 20}, {X: 3, Value: 30}}
	right := []SimulationPoint{{X: 1, Value: 11}, {X: 3, Value: 32}}
	if value, count := hvacAverageAbsoluteDelta(left, right); value != 1.5 || count != 2 {
		t.Fatalf("missing observation shifted the pairing: %v (%d)", value, count)
	}
}

func TestHVACNestedMeasurementsKeepTypedOwnershipAndActualPorts(t *testing.T) {
	loop := idf.HVACLoop{Type: "AirLoopHVAC", Name: "Main", SupplySide: idf.HVACLoopSide{Branches: []idf.HVACBranch{{Components: []idf.HVACComponent{{ObjectName: "Unit", ObjectType: "AirLoopHVAC:UnitarySystem", ObjectIndex: 1}}}}}}
	report := idf.HVACReport{
		ComponentReferences: []idf.HVACComponentReference{
			{FromObjectType: "AirLoopHVAC:UnitarySystem", FromObjectName: "Unit", TargetObjectType: "Fan:ConstantVolume", TargetObjectName: "Inner Fan", TargetObjectIndex: 2, TargetExists: true},
			{FromObjectType: "AirLoopHVAC:UnitarySystem", FromObjectName: "Unit", TargetObjectType: "Coil:Cooling:Water", TargetObjectName: "Inner Coil", TargetObjectIndex: 3, TargetExists: true},
			{FromObjectType: "AirLoopHVAC:UnitarySystem", FromObjectName: "Other Unit", TargetObjectType: "Fan:ConstantVolume", TargetObjectName: "Other Fan", TargetExists: true},
			{FromObjectType: "AirLoopHVAC:UnitarySystem", FromObjectName: "Unit", TargetObjectType: "Fan:ConstantVolume", TargetObjectName: "Missing Fan", TargetExists: false},
		},
		NodeUsages: []idf.HVACNodeUsage{
			{ObjectType: "Fan:ConstantVolume", ObjectName: "Inner Fan", NodeName: "Outer Inlet", Role: "air_inlet"},
			{ObjectType: "Fan:ConstantVolume", ObjectName: "Inner Fan", NodeName: "Between Fan Coil", Role: "air_outlet"},
			{ObjectType: "Coil:Cooling:Water", ObjectName: "Inner Coil", NodeName: "Between Fan Coil", Role: "air_inlet"},
			{ObjectType: "Coil:Cooling:Water", ObjectName: "Inner Coil", NodeName: "Outer Outlet", Role: "air_outlet"},
			{ObjectType: "Coil:Cooling:Water", ObjectName: "Inner Coil", NodeName: "Water Inlet", Role: "water_inlet"},
			{ObjectType: "Coil:Cooling:Water", ObjectName: "Inner Coil", NodeName: "Water Outlet", Role: "water_outlet"},
		},
	}
	components := hvacLoopScopedMeasurementComponents(loop, report, purposeComponentIDSet([]string{"AirLoopHVAC:UnitarySystem:Unit"}))
	if len(components) != 3 {
		t.Fatalf("typed descendants missing or unrelated/unresolved components included: %+v", components)
	}
	owners := map[string][]idf.HVACComponent{}
	for _, component := range components {
		owners[normalizePurposeToken(component.ObjectName)] = append(owners[normalizePurposeToken(component.ObjectName)], component)
	}
	series := []SimulationSeries{hvacResultTestSeries("Inner Fan", "Fan Electricity Rate", "W"), hvacResultTestSeries("Inner Coil", "Cooling Coil Total Cooling Rate", "W")}
	summaries := hvacTypedComponentSummaries(series, owners)
	if len(summaries) != 2 {
		t.Fatalf("measured children not exposed: %+v", summaries)
	}
	coil := summaries[0]
	if coil.ComponentName != "Inner Coil" || coil.ParentComponentName != "Unit" || coil.ParentComponentType != "AirLoopHVAC:UnitarySystem" || len(coil.InletNodes) != 2 || len(coil.OutletNodes) != 2 || len(coil.NodePorts) != 4 {
		t.Fatalf("typed air/water ports or wrapper ownership lost: %+v", coil)
	}
	components = hvacLoopScopedMeasurementComponents(loop, report, purposeComponentIDSet([]string{"Fan:ConstantVolume:Inner Fan"}))
	if len(components) != 1 || components[0].ObjectName != "Inner Fan" {
		t.Fatalf("selecting one nested child broadened to wrapper or siblings: %+v", components)
	}
}

func TestHVACOperationOutputsAreScopedToReportingEquipmentTypes(t *testing.T) {
	for _, pair := range [][2]string{{"Chiller:Electric:EIR", "Chiller COP"}, {"CoolingTower:SingleSpeed", "Cooling Tower Fan Electricity Rate"}, {"Coil:Cooling:DX:SingleSpeed", "Cooling Coil Runtime Fraction"}, {"Coil:Heating:DX:SingleSpeed", "Heating Coil Electricity Rate"}} {
		if !hvacComponentTypeReportsVariable(pair[0], pair[1]) {
			t.Fatalf("reported state not requested for %s: %s", pair[0], pair[1])
		}
	}
	if hvacComponentTypeReportsVariable("Coil:Cooling:Water", "Cooling Coil Electricity Rate") || hvacComponentTypeReportsVariable("Fan:ConstantVolume", "Chiller COP") {
		t.Fatal("equipment plan invented unavailable electric power/COP")
	}
}

func TestHVACSQLKeepsFullAlignedWeatherHourlyObservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT, ReportingFrequency TEXT, IsMeter INTEGER)`,
		`CREATE TABLE "Time" (TimeIndex INTEGER PRIMARY KEY, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER, EnvironmentPeriodIndex INTEGER, IntervalType INTEGER, WarmupFlag INTEGER)`,
		`CREATE TABLE EnvironmentPeriods (EnvironmentPeriodIndex INTEGER, EnvironmentType INTEGER)`,
		`CREATE TABLE ReportData (TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		`INSERT INTO EnvironmentPeriods VALUES (1,1),(2,3)`,
		`INSERT INTO ReportDataDictionary VALUES (1,'A','System Node Temperature','C','Hourly',0),(2,'A','System Node Mass Flow Rate','kg/s','Hourly',0),(3,'OTHER','Unrelated','W','Hourly',0)`,
		`INSERT INTO "Time" VALUES (1,1,1,1,0,1,1,0),(2,1,1,1,0,2,1,1)`,
		`INSERT INTO ReportData VALUES (1,1,999),(1,2,999),(2,1,999),(2,2,999)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	const frames = 1301
	for index := 1; index <= frames; index++ {
		date := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(index-1) * time.Hour)
		if _, err := tx.Exec(`INSERT INTO "Time" VALUES (?,?,?,?,0,2,1,0)`, index+2, int(date.Month()), date.Day(), date.Hour()+1); err != nil {
			t.Fatal(err)
		}
		for dictionary := 1; dictionary <= 3; dictionary++ {
			if index == 650 && dictionary == 2 {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO ReportData VALUES (?,?,?)`, index+2, dictionary, index); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	series, err := parseSimulationSQLSeriesForPlan(path, PurposeRunPlan{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 3 || len(series[0].Points) != frames || len(series[1].Points) != frames-1 || len(series[2].Points) != maxCSVSeriesPoints {
		t.Fatalf("HVAC observations were downsampled, or generic preview unbounded: %v", hvacTestPointCounts(series))
	}
	if series[0].Points[0].Value != 1 || series[1].Points[649].Value != 651 || series[1].Points[649].X != series[0].Points[650].X {
		t.Fatal("design/warmup data remained or missing observation shifted shared x coordinates")
	}
}

func TestHVACCSVKeepsFullSharedRowCoordinates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.csv")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"Date/Time", "A:System Node Temperature [C](Hourly)", "A:System Node Mass Flow Rate [kg/s](Hourly)", "OTHER:Unrelated [W](Hourly)"}); err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 1301; index++ {
		value := strconv.Itoa(index)
		mass := value
		if index == 650 {
			mass = ""
		}
		date := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(index-1) * time.Hour)
		if err := writer.Write([]string{fmt.Sprintf("%02d/%02d %02d:00:00", int(date.Month()), date.Day(), date.Hour()+1), value, mass, value}); err != nil {
			t.Fatal(err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, series, err := parseSimulationCSVForPlan(path, PurposeRunPlan{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 3 || len(series[0].Points) != 1301 || len(series[1].Points) != 1300 || len(series[2].Points) != maxCSVSeriesPoints || series[1].Points[649].X != 651 {
		t.Fatalf("CSV HVAC full observations / row alignment lost: %v", hvacTestPointCounts(series))
	}
}

func hvacTestPointCounts(series []SimulationSeries) []int {
	counts := []int{}
	for _, item := range series {
		counts = append(counts, len(item.Points))
	}
	return counts
}

func TestHVACSavedResultReplay(t *testing.T) {
	input := os.Getenv("SEMANTIC_IDF_HVAC_REPLAY_INPUT")
	if input == "" {
		t.Skip("set SEMANTIC_IDF_HVAC_REPLAY_INPUT to replay an executed input and sibling eplusout.sql")
	}
	doc, err := simulationDocumentFromInput(input)
	if err != nil {
		t.Fatal(err)
	}
	request := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}}
	plan := BuildPurposeRunPlan(doc, request)
	series, err := parseSimulationSQLSeriesForPlan(filepath.Join(filepath.Dir(input), "eplusout.sql"), plan)
	if err != nil {
		t.Fatal(err)
	}
	bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: input, Series: series}, request)
	if len(bundle.HVACLoops) == 0 {
		t.Fatal("executed input has no inspectable HVAC loops")
	}
	for _, loop := range bundle.HVACLoops {
		points := 0
		for _, item := range loop.Series {
			if len(item.Points) > points {
				points = len(item.Points)
			}
		}
		t.Logf("%s %s: node series=%d components=%d node frames=%d", loop.LoopType, loop.Name, len(loop.Series), len(loop.Components), points)
	}
	if output := os.Getenv("SEMANTIC_IDF_HVAC_REPLAY_OUTPUT"); output != "" {
		file, err := os.Create(output)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(file).Encode(bundle); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
