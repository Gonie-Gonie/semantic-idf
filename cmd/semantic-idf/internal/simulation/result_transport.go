package simulation

import "math"

// ResultTransport is an internal HTTP representation. It retains every sample
// and its order, sharing identical columns and timelines without changing the
// public result or the numerical/export models.
type ResultTransport struct {
	Schema         string               `json:"schema"`
	Result         *SimulationRunResult `json:"result"`
	Timelines      []resultTimeline     `json:"timelines"`
	PointSets      []resultPointSet     `json:"pointSets"`
	PointBindings  []resultPointBinding `json:"pointBindings"`
	ColumnSets     []resultValueColumn  `json:"columnSets,omitempty"`
	ColumnBindings []resultPointBinding `json:"columnBindings,omitempty"`
}

type resultValueColumn struct {
	Kind   string `json:"kind"`
	Values any    `json:"values"`
}

type resultValueKey struct {
	Hash   uint64
	Length int
	Kind   string
}

type resultValueSliceKey struct {
	First  any
	Length int
}

type resultTimeline struct {
	X      []int    `json:"x"`
	Labels []string `json:"labels"`
}

type resultPointSet struct {
	Timeline int       `json:"timeline"`
	Values   []float64 `json:"values"`
}

type resultPointBinding struct {
	Path []any `json:"path"`
	Data int   `json:"data"`
}

type resultColumnKey struct {
	Hash     uint64
	Length   int
	Timeline int
}

type resultSliceKey struct {
	First  *SimulationPoint
	Length int
}

type resultTransportBuilder struct {
	wire          ResultTransport
	timelineIndex map[resultColumnKey][]int
	pointIndex    map[resultColumnKey][]int
	slices        map[resultSliceKey]int
	columnIndex   map[resultValueKey][]int
	columnSlices  map[resultValueSliceKey]int
}

// CompactResultTransport makes a shallow structural copy; the caller's result,
// point arrays, and metadata are never mutated. Small results retain their
// ordinary wire shape. Large arrays use exact, collision-checked interning.
func CompactResultTransport(result *SimulationRunResult) any {
	if result == nil {
		return result
	}
	copyResult := *result
	builder := resultTransportBuilder{
		wire: ResultTransport{Schema: "semantic-idf.simulation-transfer/v1", Result: &copyResult,
			Timelines: []resultTimeline{}, PointSets: []resultPointSet{}, PointBindings: []resultPointBinding{}},
		timelineIndex: make(map[resultColumnKey][]int), pointIndex: make(map[resultColumnKey][]int),
		slices:      make(map[resultSliceKey]int),
		columnIndex: make(map[resultValueKey][]int), columnSlices: make(map[resultValueSliceKey]int),
	}
	copyResult.Series = builder.series(result.Series, []any{"series"})
	copyResult.HeatFlow = builder.heatFlow(result.HeatFlow, []any{"heatFlow"})
	if result.PurposeResults != nil {
		bundle := *result.PurposeResults
		copyResult.PurposeResults = &bundle
		bundle.ZoneHeatFlow = builder.heatFlow(bundle.ZoneHeatFlow, []any{"purposeResults", "zoneHeatFlow"})
		bundle.ThermalTopology = builder.thermalTopology(bundle.ThermalTopology, []any{"purposeResults", "thermalTopology"})
		bundle.HVACLoops = copyResultSlice(bundle.HVACLoops)
		for i := range bundle.HVACLoops {
			loop := &bundle.HVACLoops[i]
			path := []any{"purposeResults", "hvacLoops", i}
			loop.Series = builder.series(loop.Series, resultPath(path, "series"))
			loop.ControlSeries = builder.series(loop.ControlSeries, resultPath(path, "controlSeries"))
			loop.Components = copyResultSlice(loop.Components)
			for j := range loop.Components {
				component := &loop.Components[j]
				component.Series = builder.series(component.Series, resultPath(path, "components", j, "series"))
			}
		}
		comfort := &bundle.Comfort
		comfort.Series = builder.series(comfort.Series, []any{"purposeResults", "comfort", "series"})
		comfort.Zones = copyResultSlice(comfort.Zones)
		for i := range comfort.Zones {
			zone := &comfort.Zones[i]
			zone.Metrics = builder.metrics(zone.Metrics, []any{"purposeResults", "comfort", "zones", i, "metrics"})
		}
		comfort.BuildingMetrics = builder.metrics(comfort.BuildingMetrics, []any{"purposeResults", "comfort", "buildingMetrics"})
	}
	if len(builder.wire.PointBindings)+len(builder.wire.ColumnBindings) == 0 {
		return result
	}
	return &builder.wire
}

// Heat-flow columns occur both in the ordinary result and its purpose bundle.
// Surface flows also repeat their exact observations across boundaries,
// connections and traces. Intern these typed columns without changing their
// values, masks, ordering, or the independently owned original containers.
func (builder *resultTransportBuilder) heatFlow(dataset HeatFlowDataset, path []any) HeatFlowDataset {
	resultBindColumn(builder, &dataset.Labels, resultPath(path, "labels"))
	dataset.Zones = copyResultSlice(dataset.Zones)
	for i := range dataset.Zones {
		zone := &dataset.Zones[i]
		zonePath := resultPath(path, "zones", i)
		zone.Values = copyResultSlice(zone.Values)
		for j := range zone.Values {
			resultBindColumn(builder, &zone.Values[j], resultPath(zonePath, "values", j))
		}
		zone.Observed = copyResultSlice(zone.Observed)
		for j := range zone.Observed {
			resultBindColumn(builder, &zone.Observed[j], resultPath(zonePath, "observed", j))
		}
		resultBindColumn(builder, &zone.Temperature, resultPath(zonePath, "temperature"))
		resultBindColumn(builder, &zone.TemperatureObserved, resultPath(zonePath, "temperatureObserved"))
	}
	return dataset
}

func (builder *resultTransportBuilder) thermalTopology(topology ThermalTopologySimulationResult, path []any) ThermalTopologySimulationResult {
	topology.Periods = copyResultSlice(topology.Periods)
	for i := range topology.Periods {
		period := &topology.Periods[i]
		periodPath := resultPath(path, "periods", i)
		resultBindColumn(builder, &period.Labels, resultPath(periodPath, "labels"))
		period.BoundaryFlows = copyResultSlice(period.BoundaryFlows)
		for j := range period.BoundaryFlows {
			flow := &period.BoundaryFlows[j]
			flowPath := resultPath(periodPath, "boundaryFlows", j)
			resultBindColumn(builder, &flow.Values, resultPath(flowPath, "values"))
			resultBindColumn(builder, &flow.Observed, resultPath(flowPath, "observed"))
			flow.Traces = copyResultSlice(flow.Traces)
			for k := range flow.Traces {
				resultBindColumn(builder, &flow.Traces[k].Values, resultPath(flowPath, "traces", k, "values"))
			}
		}
		period.ConnectionFlows = copyResultSlice(period.ConnectionFlows)
		for j := range period.ConnectionFlows {
			resultBindColumn(builder, &period.ConnectionFlows[j].Values, resultPath(periodPath, "connectionFlows", j, "values"))
		}
	}
	return topology
}

type resultPrimitive interface{ float64 | bool | string }

func resultBindColumn[T resultPrimitive](builder *resultTransportBuilder, target *[]T, path []any) {
	values := *target
	if len(values) < 128 {
		return
	}
	sliceKey := resultValueSliceKey{First: &values[0], Length: len(values)}
	id, exists := builder.columnSlices[sliceKey]
	if !exists {
		kind, hash := resultPrimitiveHash(values)
		key := resultValueKey{Hash: hash, Length: len(values), Kind: kind}
		for _, candidate := range builder.columnIndex[key] {
			if resultPrimitiveEqual(builder.wire.ColumnSets[candidate].Values.([]T), values) {
				id, exists = candidate, true
				break
			}
		}
		if !exists {
			id = len(builder.wire.ColumnSets)
			builder.wire.ColumnSets = append(builder.wire.ColumnSets, resultValueColumn{Kind: kind, Values: copyResultSlice(values)})
			builder.columnIndex[key] = append(builder.columnIndex[key], id)
		}
		builder.columnSlices[sliceKey] = id
	}
	builder.wire.ColumnBindings = append(builder.wire.ColumnBindings, resultPointBinding{Path: path, Data: id})
	*target = nil
}

func resultPrimitiveHash[T resultPrimitive](values []T) (string, uint64) {
	hash := uint64(14695981039346656037)
	switch values := any(values).(type) {
	case []float64:
		for _, value := range values {
			hash = (hash ^ math.Float64bits(value)) * 1099511628211
		}
		return "number", hash
	case []bool:
		for _, value := range values {
			bit := uint64(0)
			if value {
				bit = 1
			}
			hash = (hash ^ bit) * 1099511628211
		}
		return "boolean", hash
	case []string:
		for _, value := range values {
			hash = (hash ^ uint64(len(value))) * 1099511628211
			for i := 0; i < len(value); i++ {
				hash = (hash ^ uint64(value[i])) * 1099511628211
			}
		}
		return "string", hash
	}
	panic("unsupported result column")
}

func resultPrimitiveEqual[T resultPrimitive](left, right []T) bool {
	if len(left) != len(right) {
		return false
	}
	if numbers, ok := any(left).([]float64); ok {
		other := any(right).([]float64)
		for i, value := range numbers {
			if math.Float64bits(value) != math.Float64bits(other[i]) {
				return false
			}
		}
		return true
	}
	for i, value := range left {
		if value != right[i] {
			return false
		}
	}
	return true
}

func copyResultSlice[T any](items []T) []T {
	if items == nil {
		return nil
	}
	copyItems := make([]T, len(items))
	copy(copyItems, items)
	return copyItems
}

func resultPath(base []any, suffix ...any) []any {
	path := make([]any, 0, len(base)+len(suffix))
	path = append(path, base...)
	return append(path, suffix...)
}

func (builder *resultTransportBuilder) series(items []SimulationSeries, path []any) []SimulationSeries {
	items = copyResultSlice(items)
	for i := range items {
		builder.points(&items[i].Points, resultPath(path, i, "points"))
		builder.points(&items[i].DisplayPoints, resultPath(path, i, "displayPoints"))
		builder.nativeIntervals(&items[i].NativeIntervals, resultPath(path, i, "nativeIntervals"))
	}
	return items
}

// Native timing repeats across raw results and loop resources. Keep it in the
// existing numeric column pool so the initial WebView message does not contain
// annual metadata arrays. The decoder restores the canonical objects lazily.
func (builder *resultTransportBuilder) nativeIntervals(target *[]SimulationNativeInterval, path []any) {
	intervals := *target
	if len(intervals) < 128 {
		return
	}
	sliceKey := resultValueSliceKey{First: &intervals[0], Length: len(intervals)}
	id, exists := builder.columnSlices[sliceKey]
	if !exists {
		values := make([]float64, 0, len(intervals)*5)
		const maxSafeInteger = int64(1<<53 - 1)
		for _, interval := range intervals {
			for _, value := range []int64{interval.TimeIndex, interval.EnvironmentPeriodIndex, interval.SimulationDays, interval.Year} {
				if value < -maxSafeInteger || value > maxSafeInteger {
					return // Do not round an int64 while compacting source evidence.
				}
			}
			values = append(values, float64(interval.TimeIndex), float64(interval.EnvironmentPeriodIndex), interval.IntervalMinutes, float64(interval.SimulationDays), float64(interval.Year))
		}
		_, hash := resultPrimitiveHash(values)
		key := resultValueKey{Hash: hash, Length: len(values), Kind: "nativeIntervals"}
		for _, candidate := range builder.columnIndex[key] {
			if resultPrimitiveEqual(builder.wire.ColumnSets[candidate].Values.([]float64), values) {
				id, exists = candidate, true
				break
			}
		}
		if !exists {
			id = len(builder.wire.ColumnSets)
			builder.wire.ColumnSets = append(builder.wire.ColumnSets, resultValueColumn{Kind: key.Kind, Values: values})
			builder.columnIndex[key] = append(builder.columnIndex[key], id)
		}
		builder.columnSlices[sliceKey] = id
	}
	builder.wire.ColumnBindings = append(builder.wire.ColumnBindings, resultPointBinding{Path: path, Data: id})
	*target = nil
}

func (builder *resultTransportBuilder) metrics(items []ComfortMetricResult, path []any) []ComfortMetricResult {
	items = copyResultSlice(items)
	for i := range items {
		builder.points(&items[i].Points, resultPath(path, i, "points"))
	}
	return items
}

func (builder *resultTransportBuilder) points(target *[]SimulationPoint, path []any) {
	points := *target
	if len(points) < 128 {
		return
	}
	key := resultSliceKey{First: &points[0], Length: len(points)}
	id, exists := builder.slices[key]
	if !exists {
		timeline := builder.timeline(points)
		hash := uint64(14695981039346656037)
		for _, point := range points {
			hash = (hash ^ math.Float64bits(point.Value)) * 1099511628211
		}
		column := resultColumnKey{Hash: hash, Length: len(points), Timeline: timeline}
		for _, candidate := range builder.pointIndex[column] {
			if resultValuesEqual(builder.wire.PointSets[candidate].Values, points) {
				id, exists = candidate, true
				break
			}
		}
		if !exists {
			values := make([]float64, len(points))
			for i, point := range points {
				values[i] = point.Value
			}
			id = len(builder.wire.PointSets)
			builder.wire.PointSets = append(builder.wire.PointSets, resultPointSet{Timeline: timeline, Values: values})
			builder.pointIndex[column] = append(builder.pointIndex[column], id)
		}
		builder.slices[key] = id
	}
	builder.wire.PointBindings = append(builder.wire.PointBindings, resultPointBinding{Path: path, Data: id})
	*target = nil
}

func resultValuesEqual(values []float64, points []SimulationPoint) bool {
	for i, point := range points {
		if math.Float64bits(values[i]) != math.Float64bits(point.Value) {
			return false
		}
	}
	return true
}

func (builder *resultTransportBuilder) timeline(points []SimulationPoint) int {
	hash := uint64(14695981039346656037)
	for _, point := range points {
		hash = (hash ^ uint64(point.X)) * 1099511628211
		hash = (hash ^ uint64(len(point.Label))) * 1099511628211
		for i := 0; i < len(point.Label); i++ {
			hash = (hash ^ uint64(point.Label[i])) * 1099511628211
		}
	}
	key := resultColumnKey{Hash: hash, Length: len(points)}
	for _, id := range builder.timelineIndex[key] {
		timeline := builder.wire.Timelines[id]
		equal := true
		for i, point := range points {
			if timeline.X[i] != point.X || timeline.Labels[i] != point.Label {
				equal = false
				break
			}
		}
		if equal {
			return id
		}
	}
	timeline := resultTimeline{X: make([]int, len(points)), Labels: make([]string, len(points))}
	for i, point := range points {
		timeline.X[i], timeline.Labels[i] = point.X, point.Label
	}
	id := len(builder.wire.Timelines)
	builder.wire.Timelines = append(builder.wire.Timelines, timeline)
	builder.timelineIndex[key] = append(builder.timelineIndex[key], id)
	return id
}
