package simulation

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestHeatFlowExecutedPlanGeometryIsCompactAndOwned(t *testing.T) {
	input := thermalTopologySimulationFixture + `
BuildingSurface:Detailed,
  Office Floor, Floor, , Office, , Ground, , NoSun, NoWind, 0.5, 4,
  0,0,0, 0,5,0, 5,5,0, 5,0,0;
`
	document, err := idf.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	geometry := idf.AnalyzeGeometry(document)
	original, err := json.Marshal(geometry)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "executed.idf")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	result := &SimulationRunResult{InputPath: path}
	for _, detail := range []string{PurposeZoneHeatFlowDetailSurface, PurposeZoneHeatFlowDetailZone} {
		overlay := buildThermalTopologySimulationResultWithGeometry(result, SimulationPurposeRequest{ZoneHeatFlowDetail: detail}, &geometry, nil)
		plan := overlay.PlanGeometry
		if overlay.Available || plan == nil || len(plan.Zones) != 1 || len(plan.Stories) != 1 || len(plan.Surfaces) != 1 || len(plan.Topology.Nodes) == 0 {
			t.Fatalf("unavailable measurement discarded executed floor plan: %+v", overlay)
		}
		sum := sha256.Sum256([]byte(input))
		if plan.SourceModelHash != hex.EncodeToString(sum[:]) || plan.Zones[0].Name != "Office" || !strings.EqualFold(plan.Surfaces[0].SurfaceType, "Floor") || len(plan.Surfaces[0].Vertices) != 4 {
			t.Fatalf("executed plan provenance/geometry = %+v", plan)
		}
		encoded, err := json.Marshal(plan)
		if err != nil {
			t.Fatal(err)
		}
		for _, excluded := range []string{`"fields"`, `"metrics"`, `"windows"`, `"constructions"`, `"worldVertices"`, `"boundaries"`, `"sourceAnchors"`, "South Window", "South Wall"} {
			if strings.Contains(string(encoded), excluded) {
				t.Fatalf("floor plan copied unrelated analysis data %s", excluded)
			}
		}
		current, _ := json.Marshal(geometry)
		if string(current) != string(original) {
			t.Fatal("snapshot construction mutated shared geometry")
		}
		// Mutating the owning report later must not move or rename executed data.
		var copyGeometry idf.GeometryReport
		if err := json.Unmarshal(original, &copyGeometry); err != nil {
			t.Fatal(err)
		}
		owned := buildHeatFlowPlanGeometry(copyGeometry, "executed-hash")
		ownedBefore, _ := json.Marshal(owned)
		copyGeometry.Zones[0].Name = "Edited zone"
		copyGeometry.Topology.Nodes[0].Label = "Edited node"
		for index := range copyGeometry.Surfaces {
			if strings.EqualFold(copyGeometry.Surfaces[index].SurfaceType, "Floor") {
				copyGeometry.Surfaces[index].Vertices[0].X += 500
			}
		}
		ownedAfter, _ := json.Marshal(owned)
		if string(ownedBefore) != string(ownedAfter) {
			t.Fatal("executed plan aliases editable geometry")
		}
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != input {
		t.Fatal("snapshot changed executed input")
	}
}

func TestHeatFlowHourlyBoundaryObservationAlignment(t *testing.T) {
	document, err := idf.Parse(thermalTopologySimulationFixture)
	if err != nil {
		t.Fatal(err)
	}
	geometry := idf.AnalyzeGeometry(document)
	series := func(column string, values []float64) SimulationSeries {
		item := SimulationSeries{File: "eplusout.csv", Column: column, ReportingFrequency: "Hourly"}
		for index, value := range values {
			item.Points = append(item.Points, SimulationPoint{X: index + 1, Label: []string{"01-01 01:00", "01-01 02:00", "01-01 03:00"}[index], Value: value})
		}
		return item
	}
	fixture := &SimulationRunResult{Series: []SimulationSeries{
		series("South Wall:Surface Average Face Conduction Heat Transfer Energy [J]", []float64{3_600_000, 0, -3_600_000}),
		series("South Window:Surface Window Heat Gain Energy [J]", []float64{0, 0, 1_800_000}),
	}}
	request := SimulationPurposeRequest{ZoneHeatFlowDetail: PurposeZoneHeatFlowDetailSurface}
	build := func(result *SimulationRunResult) ThermalTopologySimulationResult {
		return buildThermalTopologySimulationResultWithGeometry(result, request, &geometry, nil)
	}
	original, _ := json.Marshal(fixture)
	baseline := build(fixture)
	hourly := findThermalTopologySimulationPeriod(t, baseline.Periods, "hourly")
	if len(hourly.BoundaryFlows) != 1 || !reflect.DeepEqual(hourly.BoundaryFlows[0].Observed, []bool{true, true, true}) || !reflect.DeepEqual(hourly.BoundaryFlows[0].Values, []float64{1, 0, -.5}) {
		t.Fatalf("aligned native hourly observations, including zero = %+v", hourly)
	}
	for _, period := range baseline.Periods {
		if period.Kind != "hourly" && len(period.BoundaryFlows[0].Observed) != 0 {
			t.Fatalf("non-hourly aggregate claimed instantaneous observations: %+v", period)
		}
	}
	for name, mutate := range map[string]func(*SimulationRunResult){
		"mixed frequency":   func(result *SimulationRunResult) { result.Series[1].ReportingFrequency = "Monthly" },
		"unknown frequency": func(result *SimulationRunResult) { result.Series[1].ReportingFrequency = "" },
		"label mismatch":    func(result *SimulationRunResult) { result.Series[1].Points[1].Label = "02-01 02:00" },
	} {
		t.Run(name, func(t *testing.T) {
			var result SimulationRunResult
			if err := json.Unmarshal(original, &result); err != nil {
				t.Fatal(err)
			}
			mutate(&result)
			overlay := build(&result)
			period := findThermalTopologySimulationPeriod(t, overlay.Periods, "hourly")
			flow := period.BoundaryFlows[0]
			if !reflect.DeepEqual(flow.Observed, []bool{false, false, false}) || !reflect.DeepEqual(flow.Values, hourly.BoundaryFlows[0].Values) {
				t.Fatalf("ambiguous observation was displayed or historical aggregate changed: %+v", flow)
			}
		})
	}
	after, _ := json.Marshal(fixture)
	if string(after) != string(original) {
		t.Fatal("observation validation changed native result")
	}
}

func TestHeatFlowHourlyObservationRejectsMissingAndDuplicateSources(t *testing.T) {
	labels := []string{"01-01 01:00", "01-01 02:00", "01-01 03:00"}
	flow := thermalTopologyRawFlow{labels: labels, values: []float64{1, 0, -1}, sourceIDs: []string{"one", "two"}}
	makeSources := func() map[string][]thermalTopologyRawSeries {
		sources := map[string][]thermalTopologyRawSeries{}
		for _, id := range flow.sourceIDs {
			item := thermalTopologyRawSeries{unit: "J", nativeReportingFrequency: "Hourly"}
			for index, label := range labels {
				item.points = append(item.points, SimulationPoint{X: index + 1, Label: label, Value: float64(index)})
			}
			sources[id] = []thermalTopologyRawSeries{item}
		}
		return sources
	}
	for name, mutate := range map[string]func(map[string][]thermalTopologyRawSeries){
		"missing source": func(sources map[string][]thermalTopologyRawSeries) { delete(sources, "two") },
		"duplicate source": func(sources map[string][]thermalTopologyRawSeries) {
			sources["two"] = append(sources["two"], sources["two"][0])
		},
		"short source": func(sources map[string][]thermalTopologyRawSeries) {
			sources["two"][0].points = sources["two"][0].points[:2]
		},
		"reordered labels": func(sources map[string][]thermalTopologyRawSeries) {
			sources["two"][0].points[0].Label, sources["two"][0].points[1].Label = labels[1], labels[0]
		},
	} {
		t.Run(name, func(t *testing.T) {
			sources := makeSources()
			mutate(sources)
			if flags := thermalTopologyFlowObserved(flow, sources, labels); !reflect.DeepEqual(flags, []bool{false, false, false}) {
				t.Fatalf("ambiguous native source claimed exact hourly values: %v", flags)
			}
		})
	}
	sources := makeSources()
	sources["two"][0].points[1].Value = math.NaN()
	if flags := thermalTopologyFlowObserved(flow, sources, labels); !reflect.DeepEqual(flags, []bool{true, false, true}) {
		t.Fatalf("nonfinite contributing value was accepted: %v", flags)
	}
	wrongGlobal := append([]string(nil), labels...)
	wrongGlobal[0] = "01-02 01:00"
	if flags := thermalTopologyFlowObserved(flow, makeSources(), wrongGlobal); !reflect.DeepEqual(flags, []bool{false, false, false}) {
		t.Fatalf("flow borrowed another boundary's frame sequence: %v", flags)
	}
	definition := thermalTopologyGroupedPeriod("hourly", "Hourly", "hourly", []string{labels[0], labels[0], labels[2]}, func(label string, _ int) string { return label })
	if flags := thermalTopologyPeriodObserved([]bool{true, true, true}, definition); !reflect.DeepEqual(flags, []bool{false, true}) {
		t.Fatalf("duplicate hourly timestamp was treated as one observed interval: %v", flags)
	}
}

func TestHeatFlowNativeFrequencyRequiresExplicitEvidence(t *testing.T) {
	for _, test := range []struct {
		series SimulationSeries
		want   string
	}{
		{SimulationSeries{Column: "Wall:Surface Average Face Conduction Heat Transfer Energy [J]"}, ""},
		{SimulationSeries{Column: "Wall:Surface Average Face Conduction Heat Transfer Energy [J](Hourly)"}, "Hourly"},
		{SimulationSeries{Column: "Wall:Surface Average Face Conduction Heat Transfer Energy [J](Hourly)", ReportingFrequency: "Daily"}, "Daily"},
	} {
		if actual := thermalTopologyNativeSeriesFrequency(test.series); actual != test.want {
			t.Fatalf("native frequency = %q, want %q", actual, test.want)
		}
	}
}

func TestHeatFlowHourlyRateObservationRequiresSingleHourIntegration(t *testing.T) {
	for _, test := range []struct {
		name       string
		labels     []string
		rate       bool
		value      float64
		wantValues []float64
		wantFlags  []bool
	}{
		{"normal hourly rate", []string{"01-01 01:00", "01-01 02:00", "01-01 03:00"}, true, 1000, []float64{1, 1, 1}, []bool{true, true, true}},
		{"shared gap in hourly rates", []string{"01-01 01:00", "01-01 03:00"}, true, 1000, []float64{2, 2}, []bool{false, false}},
		{"reported hourly energy with gap", []string{"01-01 01:00", "01-01 03:00"}, false, 3_600_000, []float64{1, 1}, []bool{true, true}},
		{"rate gap beside an intact hour", []string{"01-01 01:00", "01-01 02:00", "01-01 04:00"}, true, 1000, []float64{1, 2, 2}, []bool{true, false, false}},
	} {
		t.Run(test.name, func(t *testing.T) {
			unit := "J"
			if test.rate {
				unit = "W"
			}
			item := thermalTopologyRawSeries{unit: unit, reportingFrequency: "Hourly", nativeReportingFrequency: "Hourly", rate: test.rate}
			for index, label := range test.labels {
				item.points = append(item.points, SimulationPoint{X: index + 1, Label: label, Value: test.value})
			}
			values, labels := normalizeThermalTopologySeries(item)
			if !reflect.DeepEqual(values, test.wantValues) {
				t.Fatalf("historical integrated values changed: %v, want %v", values, test.wantValues)
			}
			// A zero-valued companion energy source has the same exact labels.
			// It cannot make a gapped rate contribution safe by supplying an
			// otherwise valid native Hourly sequence or a measured zero.
			companion := thermalTopologyRawSeries{unit: "J", nativeReportingFrequency: "Hourly"}
			for index, label := range labels {
				companion.points = append(companion.points, SimulationPoint{X: index + 1, Label: label})
			}
			flow := thermalTopologyRawFlow{labels: labels, values: values, sourceIDs: []string{"rate-or-energy", "zero-energy"}}
			sources := map[string][]thermalTopologyRawSeries{"rate-or-energy": {item}, "zero-energy": {companion}}
			if flags := thermalTopologyFlowObserved(flow, sources, labels); !reflect.DeepEqual(flags, test.wantFlags) {
				t.Fatalf("rate/energy interval evidence confused: flags %v, want %v", flags, test.wantFlags)
			}
			if !reflect.DeepEqual(flow.values, test.wantValues) {
				t.Fatal("arrow availability changed retained interval energy")
			}
		})
	}
}

func TestHeatFlowHourlyObservationRequiresExplicitSupportedUnits(t *testing.T) {
	labels := []string{"01-01 01:00", "01-01 02:00"}
	for _, test := range []struct {
		name  string
		rate  bool
		units []string
		want  bool
	}{
		{"rate", true, []string{"W", "watt", "watts", " kW ", "MW"}, true},
		{"energy", false, []string{"J", "joule", "joules", "kJ", "MJ", "GJ", "Wh", "kWh", "MWh"}, true},
		{"invalid rate", true, []string{"", "  ", "Btu", "C", "W/m2", "J"}, false},
		{"invalid energy", false, []string{"", "  ", "Btu", "C", "W/m2", "W"}, false},
	} {
		for _, unit := range test.units {
			t.Run(test.name+"/"+unit, func(t *testing.T) {
				item := thermalTopologyRawSeries{unit: unit, rate: test.rate, reportingFrequency: "Hourly", nativeReportingFrequency: "Hourly",
					points: []SimulationPoint{{X: 1, Label: labels[0], Value: 1}, {X: 2, Label: labels[1], Value: 0}}}
				values, _ := normalizeThermalTopologySeries(item)
				retained := append([]float64(nil), values...)
				flow := thermalTopologyRawFlow{labels: labels, values: values, sourceIDs: []string{"valid companion", "candidate"}}
				companion := thermalTopologyRawSeries{unit: "J", reportingFrequency: "Hourly", nativeReportingFrequency: "Hourly",
					points: []SimulationPoint{{X: 1, Label: labels[0], Value: 0}, {X: 2, Label: labels[1], Value: 0}}}
				sources := map[string][]thermalTopologyRawSeries{"valid companion": {companion}, "candidate": {item}}
				if flags := thermalTopologyFlowObserved(flow, sources, labels); !reflect.DeepEqual(flags, []bool{test.want, test.want}) {
					t.Fatalf("unit %q rate=%t incorrectly verified, including zero: %v", unit, test.rate, flags)
				}
				if !reflect.DeepEqual(flow.values, retained) || sources["candidate"][0].unit != unit {
					t.Fatal("unit availability check modified historical values or native metadata")
				}
			})
		}
	}
}

func TestHeatFlowSQLHourlyObservationUsesNativeFrequency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	createTestEnergyPlusSQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{
		`ALTER TABLE ReportDataDictionary ADD COLUMN ReportingFrequency TEXT`,
		`INSERT INTO ReportDataDictionary VALUES (20, 'South Wall', 'Surface Average Face Conduction Heat Transfer Energy', 'J', 'Hourly')`,
		`INSERT INTO ReportData VALUES (20, 1, 20, 3600000), (21, 2, 20, -1800000)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	document, err := idf.Parse(thermalTopologySimulationFixture)
	if err != nil {
		t.Fatal(err)
	}
	geometry := idf.AnalyzeGeometry(document)
	result := &SimulationRunResult{Files: []SimulationFileInfo{{Kind: "sqlite", Path: path}}}
	request := SimulationPurposeRequest{ZoneHeatFlowDetail: PurposeZoneHeatFlowDetailSurface}
	for _, frequency := range []string{"Hourly", "Daily", ""} {
		if _, err := db.Exec(`UPDATE ReportDataDictionary SET ReportingFrequency = ? WHERE ReportDataDictionaryIndex = 20`, frequency); err != nil {
			t.Fatal(err)
		}
		overlay := buildThermalTopologySimulationResultWithGeometry(result, request, &geometry, nil)
		period := findThermalTopologySimulationPeriod(t, overlay.Periods, "hourly")
		want := strings.EqualFold(frequency, "Hourly")
		if len(period.BoundaryFlows) != 1 || !reflect.DeepEqual(period.BoundaryFlows[0].Observed, []bool{want, want}) || !reflect.DeepEqual(period.BoundaryFlows[0].Values, []float64{1, -.5}) {
			t.Fatalf("SQL native frequency %q did not gate arrows independently of old values: %+v", frequency, period)
		}
	}
}
