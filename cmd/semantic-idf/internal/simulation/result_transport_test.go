package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestSimulationResultTransportLosslessSharedColumns(t *testing.T) {
	points := make([]SimulationPoint, 160)
	for i := range points {
		points[i] = SimulationPoint{X: i / 2, Label: fmt.Sprintf("hour %d", i%7), Value: float64(i) / 3}
	}
	points[0].Value = math.Copysign(0, -1)
	points[1].Value = 0
	points[2].Value = 0.000000000000000001
	points[3].Label = ""
	display := copyResultSlice(points)
	display[5].Value = 1.234567
	otherTimeline := copyResultSlice(points)
	otherTimeline[6].X += 42
	otherLabels := copyResultSlice(points)
	otherLabels[8].Label = "different"
	series := SimulationSeries{File: "eplusout.sql", Column: "Temperature", Points: points, DisplayPoints: display, RowCount: 8760}
	result := &SimulationRunResult{RunID: "exact", Series: []SimulationSeries{
		series,
		{Points: otherTimeline}, {Points: otherLabels},
		{Points: nil}, {Points: []SimulationPoint{}},
	}, PurposeResults: &PurposeResultBundle{
		HVACLoops: []HVACLoopRunResult{{Name: "Air", Series: []SimulationSeries{series},
			Components: []HVACComponentRunSummary{{ComponentName: "Coil", Series: []SimulationSeries{series}}}},
			{Name: "Water", Series: []SimulationSeries{{Points: copyResultSlice(points)}}}},
		Comfort: ComfortResult{Series: []SimulationSeries{series},
			Zones:           []ComfortZoneResult{{ZoneName: "Office", Metrics: []ComfortMetricResult{{Name: "T", Points: copyResultSlice(points)}}}},
			BuildingMetrics: []ComfortMetricResult{{Name: "Facility", Points: display}}},
	}}
	want, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	wire, ok := CompactResultTransport(result).(*ResultTransport)
	if !ok || len(wire.Timelines) != 3 || len(wire.PointSets) != 4 {
		t.Fatalf("equal content must share columns while distinct values/timestamps remain separate: %#v", wire)
	}
	validateResultTransportBindings(t, result, wire)
	if math.Float64bits(wire.PointSets[0].Values[0]) != math.Float64bits(points[0].Value) {
		t.Fatal("negative zero changed")
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var wantObject, envelope map[string]any
	if err := json.Unmarshal(want, &wantObject); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	got := envelope["result"].(map[string]any)
	sets := envelope["pointSets"].([]any)
	timelines := envelope["timelines"].([]any)
	for _, bindingValue := range envelope["pointBindings"].([]any) {
		binding := bindingValue.(map[string]any)
		set := sets[int(binding["data"].(float64))].(map[string]any)
		timeline := timelines[int(set["timeline"].(float64))].(map[string]any)
		x, labels, values := timeline["x"].([]any), timeline["labels"].([]any), set["values"].([]any)
		points := make([]any, len(values))
		for i, value := range values {
			point := map[string]any{"x": x[i], "value": value}
			if labels[i] != "" {
				point["label"] = labels[i]
			}
			points[i] = point
		}
		path := binding["path"].([]any)
		var parent any = got
		for _, segment := range path[:len(path)-1] {
			switch index := segment.(type) {
			case string:
				parent = parent.(map[string]any)[index]
			case float64:
				parent = parent.([]any)[int(index)]
			}
		}
		parent.(map[string]any)[path[len(path)-1].(string)] = points
	}
	if !reflect.DeepEqual(got, wantObject) {
		t.Fatal("transport changed observations, metadata, null/empty collections or their order")
	}
	after, err := json.Marshal(result)
	if err != nil || string(after) != string(want) {
		t.Fatal("compact transport mutated the original result")
	}
	if len(encoded) >= len(want)/2 {
		t.Fatalf("shared full-resolution fixture did not shrink: %d >= %d / 2", len(encoded), len(want))
	}
}

func TestSimulationResultTransportSmallAndInvalidSamples(t *testing.T) {
	if got := CompactResultTransport(nil); !reflect.ValueOf(got).IsNil() {
		t.Fatal("nil result changed")
	}
	result := &SimulationRunResult{Series: []SimulationSeries{{Points: []SimulationPoint{{X: 4, Value: 0}}}}}
	if CompactResultTransport(result) != result {
		t.Fatal("small results should keep their ordinary response")
	}
	result.Series[0].Points = make([]SimulationPoint, 128)
	result.Series[0].Points[127].Value = math.Inf(1)
	if _, err := json.Marshal(CompactResultTransport(result)); err == nil {
		t.Fatal("nonfinite value was silently converted or dropped")
	}
}

// Verify every binding directly against the independently resolved source
// path. This also scales to a saved full-year result without re-marshaling or
// materializing a second copy of its millions of point objects.
func validateResultTransportBindings(t *testing.T, original *SimulationRunResult, wire *ResultTransport) int {
	t.Helper()
	seen := make(map[string]bool, len(wire.PointBindings))
	samples := 0
	for _, binding := range wire.PointBindings {
		path := fmt.Sprintf("%#v", binding.Path)
		if seen[path] {
			t.Fatalf("duplicate transport binding: %s", path)
		}
		seen[path] = true
		if binding.Data < 0 || binding.Data >= len(wire.PointSets) {
			t.Fatalf("invalid point set at %s: %d", path, binding.Data)
		}
		set := wire.PointSets[binding.Data]
		if set.Timeline < 0 || set.Timeline >= len(wire.Timelines) {
			t.Fatalf("invalid timeline at %s: %d", path, set.Timeline)
		}
		timeline := wire.Timelines[set.Timeline]
		points := resultTransportTestPointPath(t, original, binding.Path)
		if len(points) == 0 || len(points) != len(set.Values) || len(points) != len(timeline.X) || len(points) != len(timeline.Labels) {
			t.Fatalf("sample count changed at %s: original=%d values=%d x=%d labels=%d", path, len(points), len(set.Values), len(timeline.X), len(timeline.Labels))
		}
		if remaining := resultTransportTestPointPath(t, wire.Result, binding.Path); remaining != nil {
			t.Fatalf("bound points remain inline at %s", path)
		}
		for index, point := range points {
			if point.X != timeline.X[index] || point.Label != timeline.Labels[index] || math.Float64bits(point.Value) != math.Float64bits(set.Values[index]) {
				t.Fatalf("sample changed at %s[%d]: original=%+v x=%d label=%q value=%v", path, index, point, timeline.X[index], timeline.Labels[index], set.Values[index])
			}
		}
		samples += len(points)
	}
	return samples
}

func resultTransportTestPointPath(t *testing.T, result *SimulationRunResult, path []any) []SimulationPoint {
	t.Helper()
	points, ok := resultTransportTestPath(t, result, path).Interface().([]SimulationPoint)
	if !ok {
		t.Fatalf("transport path does not address points: %v", path)
	}
	return points
}

func resultTransportTestPath(t *testing.T, result *SimulationRunResult, path []any) reflect.Value {
	t.Helper()
	value := reflect.ValueOf(result)
	for _, segment := range path {
		for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
			value = value.Elem()
		}
		if !value.IsValid() {
			t.Fatalf("nil ancestor in transport path %v", path)
		}
		switch segment := segment.(type) {
		case string:
			if value.Kind() != reflect.Struct {
				t.Fatalf("non-object ancestor in transport path %v", path)
			}
			field := -1
			for index := 0; index < value.NumField(); index++ {
				if strings.Split(value.Type().Field(index).Tag.Get("json"), ",")[0] == segment {
					field = index
					break
				}
			}
			if field < 0 {
				t.Fatalf("unknown field %q in transport path %v", segment, path)
			}
			value = value.Field(field)
		case int:
			if value.Kind() != reflect.Slice || segment < 0 || segment >= value.Len() {
				t.Fatalf("invalid index %d in transport path %v", segment, path)
			}
			value = value.Index(segment)
		default:
			t.Fatalf("invalid segment type %T in transport path %v", segment, path)
		}
	}
	return value
}

func validateResultTransportColumnBindings(t *testing.T, original *SimulationRunResult, wire *ResultTransport) int {
	t.Helper()
	seen := make(map[string]bool, len(wire.PointBindings)+len(wire.ColumnBindings))
	for _, binding := range wire.PointBindings {
		seen[fmt.Sprintf("%#v", binding.Path)] = true
	}
	samples := 0
	for _, binding := range wire.ColumnBindings {
		path := fmt.Sprintf("%#v", binding.Path)
		if seen[path] {
			t.Fatalf("duplicate column binding: %s", path)
		}
		seen[path] = true
		if binding.Data < 0 || binding.Data >= len(wire.ColumnSets) {
			t.Fatalf("invalid column at %s", path)
		}
		column := wire.ColumnSets[binding.Data]
		originalValues := resultTransportTestPath(t, original, binding.Path)
		remaining := resultTransportTestPath(t, wire.Result, binding.Path)
		if originalValues.Kind() != reflect.Slice || originalValues.Len() < 128 || !remaining.IsNil() {
			t.Fatalf("invalid original or inline column at %s", path)
		}
		equal := false
		switch values := originalValues.Interface().(type) {
		case []float64:
			got, ok := column.Values.([]float64)
			equal = ok && column.Kind == "number" && resultPrimitiveEqual(values, got)
		case []bool:
			got, ok := column.Values.([]bool)
			equal = ok && column.Kind == "boolean" && resultPrimitiveEqual(values, got)
		case []string:
			got, ok := column.Values.([]string)
			equal = ok && column.Kind == "string" && resultPrimitiveEqual(values, got)
		}
		if !equal {
			t.Fatalf("column length/order/value bits changed at %s", path)
		}
		samples += originalValues.Len()
	}
	return samples
}

func TestSimulationResultTransportLosslessHeatFlowColumns(t *testing.T) {
	values, observed, labels := make([]float64, 160), make([]bool, 160), make([]string, 160)
	for i := range values {
		values[i], observed[i], labels[i] = float64(i)/3, i != 7, fmt.Sprintf("시간 %d", i)
	}
	values[0] = math.Copysign(0, -1)
	positiveZero := copyResultSlice(values)
	positiveZero[0] = 0
	dataset := HeatFlowDataset{Labels: labels, Zones: []HeatFlowZoneSeries{
		{Name: "Office", Values: [][]float64{values, copyResultSlice(values), positiveZero, nil, {}},
			Observed:    [][]bool{observed, copyResultSlice(observed), nil, {}},
			Temperature: copyResultSlice(values), TemperatureObserved: observed},
	}}
	result := &SimulationRunResult{HeatFlow: dataset, PurposeResults: &PurposeResultBundle{
		ZoneHeatFlow: dataset, ThermalTopology: ThermalTopologySimulationResult{Periods: []ThermalTopologySimulationPeriod{{
			ID: "hourly", Labels: copyResultSlice(labels),
			BoundaryFlows:   []ThermalTopologyBoundaryFlow{{Values: values, Observed: observed, Traces: []ThermalTopologyFlowTrace{{Values: copyResultSlice(values)}}}},
			ConnectionFlows: []ThermalTopologyConnectionFlow{{Values: values}},
		}}},
	}}
	before, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	wire, ok := CompactResultTransport(result).(*ResultTransport)
	if !ok || len(wire.ColumnSets) != 4 || len(wire.ColumnBindings) != 21 {
		t.Fatalf("typed repeated columns were not interned exactly: %#v", wire)
	}
	if wire.Timelines == nil || wire.PointSets == nil || wire.PointBindings == nil {
		t.Fatal("column-only transfer must retain empty point arrays")
	}
	if samples := validateResultTransportColumnBindings(t, result, wire); samples != 21*160 {
		t.Fatalf("validated %d samples", samples)
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var want, envelope map[string]any
	if err := json.Unmarshal(before, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	got := envelope["result"].(map[string]any)
	columns := envelope["columnSets"].([]any)
	for _, entry := range envelope["columnBindings"].([]any) {
		binding := entry.(map[string]any)
		path := binding["path"].([]any)
		var owner any = got
		for _, key := range path[:len(path)-1] {
			switch key := key.(type) {
			case string:
				owner = owner.(map[string]any)[key]
			case float64:
				owner = owner.([]any)[int(key)]
			}
		}
		values := columns[int(binding["data"].(float64))].(map[string]any)["values"]
		switch key := path[len(path)-1].(type) {
		case string:
			owner.(map[string]any)[key] = values
		case float64:
			owner.([]any)[int(key)] = values
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("column restore changed original JSON values, metadata, absent fields or nil/empty arrays")
	}
	after, err := json.Marshal(result)
	if err != nil || string(before) != string(after) {
		t.Fatal("column transport mutated original result")
	}
	if len(encoded) >= len(before)/2 {
		t.Fatalf("repeated column fixture failed to shrink: %d >= %d / 2", len(encoded), len(before))
	}
	values[1], observed[1], labels[1] = 999, false, "changed"
	for _, column := range wire.ColumnSets {
		switch values := column.Values.(type) {
		case []float64:
			if values[1] != 1.0/3 {
				t.Fatal("numeric transport column aliases caller")
			}
		case []bool:
			if !values[1] {
				t.Fatal("mask transport column aliases caller")
			}
		case []string:
			if values[1] != "시간 1" {
				t.Fatal("label transport column aliases caller")
			}
		}
	}
}

func TestSimulationResultTransportColumnCollisionAndInvalidNumbers(t *testing.T) {
	builder := resultTransportBuilder{columnIndex: map[resultValueKey][]int{}, columnSlices: map[resultValueSliceKey]int{}}
	numbers := make([]float64, 128)
	kind, hash := resultPrimitiveHash(numbers)
	other := copyResultSlice(numbers)
	other[127] = 42
	builder.wire.ColumnSets = []resultValueColumn{{Kind: kind, Values: other}}
	builder.columnIndex[resultValueKey{Kind: kind, Hash: hash, Length: 128}] = []int{0}
	resultBindColumn(&builder, &numbers, []any{"test"})
	if len(builder.wire.ColumnSets) != 2 || builder.wire.ColumnBindings[0].Data != 1 {
		t.Fatal("hash collision replaced distinct observations")
	}
	for _, nonfinite := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		values := make([]float64, 128)
		values[127] = nonfinite
		result := &SimulationRunResult{HeatFlow: HeatFlowDataset{Zones: []HeatFlowZoneSeries{{Values: [][]float64{values}}}}}
		if _, err := json.Marshal(CompactResultTransport(result)); err == nil {
			t.Fatal("nonfinite column was silently changed or removed")
		}
	}
}
