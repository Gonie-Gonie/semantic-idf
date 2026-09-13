package simulation

import "math"

// ResultTransport is an internal HTTP representation. It retains every sample
// and its order, sharing identical columns and timelines without changing the
// public result or the numerical/export models.
type ResultTransport struct {
	Schema        string               `json:"schema"`
	Result        *SimulationRunResult `json:"result"`
	Timelines     []resultTimeline     `json:"timelines"`
	PointSets     []resultPointSet     `json:"pointSets"`
	PointBindings []resultPointBinding `json:"pointBindings"`
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
		wire:          ResultTransport{Schema: "semantic-idf.simulation-transfer/v1", Result: &copyResult},
		timelineIndex: make(map[resultColumnKey][]int), pointIndex: make(map[resultColumnKey][]int),
		slices: make(map[resultSliceKey]int),
	}
	copyResult.Series = builder.series(result.Series, []any{"series"})
	if result.PurposeResults != nil {
		bundle := *result.PurposeResults
		copyResult.PurposeResults = &bundle
		bundle.HVACLoops = copyResultSlice(bundle.HVACLoops)
		for i := range bundle.HVACLoops {
			loop := &bundle.HVACLoops[i]
			path := []any{"purposeResults", "hvacLoops", i}
			loop.Series = builder.series(loop.Series, resultPath(path, "series"))
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
	if len(builder.wire.PointBindings) == 0 {
		return result
	}
	return &builder.wire
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
	}
	return items
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
