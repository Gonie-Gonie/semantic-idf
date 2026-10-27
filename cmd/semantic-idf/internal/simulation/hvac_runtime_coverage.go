package simulation

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Channel acquisition and interval validity are distinct. Keep collected raw
// channels usable at valid frames, while completeness also accounts for gaps,
// unset targets and values outside a native status domain.
func validateHVACRuntimeObservations(result *HVACLoopRunResult, coverage *HVACControlCoverage) {
	if len(result.ControlSeries) == 0 {
		return
	}
	fileKey := func(value string) string {
		return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"))
	}
	label := func(point SimulationPoint) string {
		if point.Label != "" {
			return point.Label
		}
		return strconv.Itoa(point.X)
	}
	files := map[string]int{}
	byID := map[string]SimulationSeries{}
	for _, series := range result.ControlSeries {
		files[fileKey(series.File)]++
		byID[series.SourceID] = series
	}
	orderedFiles := make([]string, 0, len(files))
	for file := range files {
		orderedFiles = append(orderedFiles, file)
	}
	sort.Slice(orderedFiles, func(i, j int) bool {
		a, b := orderedFiles[i], orderedFiles[j]
		if strings.HasSuffix(a, ".sql") != strings.HasSuffix(b, ".sql") {
			return strings.HasSuffix(a, ".sql")
		}
		if files[a] != files[b] {
			return files[a] > files[b]
		}
		return a < b
	})
	primary := orderedFiles[0]
	axes := map[string]map[int]string{}
	axisConflicts := map[string]map[int]bool{}
	for _, series := range result.ControlSeries {
		if fileKey(series.File) != primary {
			continue
		}
		cadence := purposeControlFrequency(purposeSeriesFrequency(series))
		if axes[cadence] == nil {
			axes[cadence], axisConflicts[cadence] = map[int]string{}, map[int]bool{}
		}
		axis, conflicts := axes[cadence], axisConflicts[cadence]
		for _, point := range series.Points {
			name := label(point)
			if previous, exists := axis[point.X]; exists && previous != name {
				conflicts[point.X] = true
			}
			axis[point.X] = name
		}
	}
	axisRows := map[string][]int{}
	axisLabelCounts := map[string]map[string]int{}
	for cadence, axis := range axes {
		rows, counts := make([]int, 0, len(axis)), map[string]int{}
		for row, name := range axis {
			rows = append(rows, row)
			counts[name]++
		}
		sort.Ints(rows)
		axisRows[cadence], axisLabelCounts[cadence] = rows, counts
	}
	type observationIndex struct {
		ordered bool
		rows    map[int]int
		labels  map[string]int
	}
	indexes := map[string]observationIndex{}
	for id, series := range byID {
		index := observationIndex{ordered: true}
		for i := 1; i < len(series.Points); i++ {
			index.ordered = index.ordered && series.Points[i].X >= series.Points[i-1].X
		}
		if fileKey(series.File) != primary {
			index.labels = map[string]int{}
			for i, point := range series.Points {
				name := label(point)
				if _, exists := index.labels[name]; exists {
					index.labels[name] = -1
				} else {
					index.labels[name] = i
				}
			}
		} else if !index.ordered {
			index.rows = map[int]int{}
			for i, point := range series.Points {
				if _, exists := index.rows[point.X]; exists {
					index.rows[point.X] = -1
				} else {
					index.rows[point.X] = i
				}
			}
		}
		indexes[id] = index
	}
	validation := map[string]string{}
	timestepEnds := hvacRuntimeNativeTimestepEnds(result.ControlSeries)
	for i := range result.ControlSignals {
		binding := &result.ControlSignals[i]
		if !binding.Required || binding.Status != "collected" {
			continue
		}
		identity := binding.SeriesID + "\x00" + binding.Role + "\x00" + binding.Domain
		message, checked := validation[identity]
		if !checked {
			series, index := byID[binding.SeriesID], indexes[binding.SeriesID]
			cadence := purposeControlFrequency(purposeSeriesFrequency(series))
			axis, conflicts := axes[cadence], axisConflicts[cadence]
			rows, labelCount := axisRows[cadence], axisLabelCounts[cadence]
			invalid, first, cursor := 0, "", 0
			for _, row := range rows {
				offset, exists, duplicate := 0, false, false
				if fileKey(series.File) != primary {
					offset, exists = index.labels[axis[row]]
					if labelCount[axis[row]] != 1 {
						exists = false
					}
				} else if index.ordered {
					for cursor < len(series.Points) && series.Points[cursor].X < row {
						cursor++
					}
					offset, exists = cursor, cursor < len(series.Points) && series.Points[cursor].X == row
					duplicate = exists && cursor+1 < len(series.Points) && series.Points[cursor+1].X == row
				} else {
					offset, exists = index.rows[row]
				}
				reason := ""
				if conflicts[row] || duplicate || exists && offset == -1 {
					reason = "ambiguous observation"
				} else if !exists {
					reason = "missing observation"
				} else if label(series.Points[offset]) != axis[row] {
					reason = "conflicting native row label"
				} else {
					value := series.Points[offset].Value
					switch {
					case math.IsNaN(value) || math.IsInf(value, 0):
						reason = "non-finite observation"
					case binding.Role == "setpoint" && !hvacHasTemperatureSetpoint(value):
						reason = "unset native setpoint"
					case binding.Domain == "manager_status" && (value < 0 || value > 3 || math.Trunc(value) != value), binding.Domain == "hybrid_ventilation_status" && (value < 0 || value > 2 || math.Trunc(value) != value):
						reason = "invalid native status domain"
					}
				}
				if reason != "" {
					invalid++
					if first == "" {
						first = axis[row] + ": " + reason
					}
				}
			}
			if invalid > 0 {
				message = fmt.Sprintf("%d of %d required %s intervals are missing, ambiguous or invalid (%s).", invalid, len(rows), binding.ReportingFrequency, first)
			}
			if message == "" && purposeDetailedFrequency(purposeSeriesFrequency(series)) {
				message = validateHVACDetailedIntervals(series, timestepEnds)
			}
			validation[identity] = message
		}
		if message != "" {
			binding.Message = message
			coverage.Collected--
			coverage.Missing++
		}
	}
}

type hvacNativeEndpoint struct {
	environment int64
	year        int64
	minute      int64
}

type hvacNativeFrame struct {
	label    string
	duration float64
}

// SQL labels and Interval are already rounded to minutes by EnergyPlus. Use
// those reported boundaries exactly, and fail acquisition when the source no
// longer distinguishes intervals; never interpolate or round a status code.
func hvacRuntimeNativeEndpoint(point SimulationPoint, interval SimulationNativeInterval) (hvacNativeEndpoint, bool) {
	fields := strings.Fields(point.Label)
	if len(fields) != 2 || interval.EnvironmentPeriodIndex <= 0 || interval.SimulationDays <= 0 {
		return hvacNativeEndpoint{}, false
	}
	clock := strings.Split(fields[1], ":")
	if len(clock) != 2 {
		return hvacNativeEndpoint{}, false
	}
	hour, hourErr := strconv.ParseInt(clock[0], 10, 64)
	minute, minuteErr := strconv.ParseInt(clock[1], 10, 64)
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 24 || minute < 0 || minute > 59 {
		return hvacNativeEndpoint{}, false
	}
	return hvacNativeEndpoint{environment: interval.EnvironmentPeriodIndex, year: interval.Year, minute: (interval.SimulationDays-1)*24*60 + hour*60 + minute}, true
}

func hvacRuntimeNativeTimestepEnds(series []SimulationSeries) map[hvacNativeEndpoint]hvacNativeFrame {
	ends := map[hvacNativeEndpoint]hvacNativeFrame{}
	seen := map[*SimulationNativeInterval]bool{}
	for _, item := range series {
		if !purposeTimestepFrequency(purposeSeriesFrequency(item)) || len(item.NativeIntervals) == 0 || len(item.NativeIntervals) != len(item.Points) || seen[&item.NativeIntervals[0]] {
			continue
		}
		seen[&item.NativeIntervals[0]] = true
		for i, point := range item.Points {
			if endpoint, valid := hvacRuntimeNativeEndpoint(point, item.NativeIntervals[i]); valid {
				ends[endpoint] = hvacNativeFrame{label: point.Label, duration: item.NativeIntervals[i].IntervalMinutes}
			}
		}
	}
	return ends
}

func validateHVACDetailedIntervals(series SimulationSeries, timestepEnds map[hvacNativeEndpoint]hvacNativeFrame) string {
	failure := func(reason string) string {
		return "Required Detailed availability observations have invalid native interval coverage (" + reason + ")."
	}
	if len(series.NativeIntervals) != len(series.Points) {
		return failure("native SQL interval metadata is missing")
	}
	ends := map[hvacNativeEndpoint]bool{}
	type environment struct{ index, year int64 }
	type bounds struct{ start, end float64 }
	zoneBounds := map[environment]bounds{}
	for endpoint, frame := range timestepEnds {
		if frame.duration <= 0 || math.IsNaN(frame.duration) || math.IsInf(frame.duration, 0) {
			return failure(frame.label + ": invalid native Timestep duration")
		}
		key := environment{endpoint.environment, endpoint.year}
		start, end := float64(endpoint.minute)-frame.duration, float64(endpoint.minute)
		bound, exists := zoneBounds[key]
		if !exists {
			bound = bounds{start, end}
		} else {
			bound.start = math.Min(bound.start, start)
			bound.end = math.Max(bound.end, end)
		}
		zoneBounds[key] = bound
	}
	detailedBounds := map[environment]bounds{}
	var previous hvacNativeEndpoint
	var previousTime int64
	for i, interval := range series.NativeIntervals {
		point := series.Points[i]
		endpoint, valid := hvacRuntimeNativeEndpoint(point, interval)
		if !valid || interval.TimeIndex <= 0 || interval.IntervalMinutes <= 0 || math.IsNaN(interval.IntervalMinutes) || math.IsInf(interval.IntervalMinutes, 0) {
			return failure(point.Label + ": missing or nonpositive native duration")
		}
		if ends[endpoint] || interval.TimeIndex <= previousTime {
			return failure(point.Label + ": duplicate or unordered native interval")
		}
		key := environment{endpoint.environment, endpoint.year}
		bound, exists := detailedBounds[key]
		if !exists {
			bound.start = float64(endpoint.minute) - interval.IntervalMinutes
		}
		bound.end = float64(endpoint.minute)
		detailedBounds[key] = bound
		if i > 0 && endpoint.environment == previous.environment && endpoint.year == previous.year && float64(endpoint.minute)-interval.IntervalMinutes != float64(previous.minute) {
			return failure(point.Label + ": gap or overlap between native intervals")
		}
		ends[endpoint] = true
		previous, previousTime = endpoint, interval.TimeIndex
	}
	for key, expected := range zoneBounds {
		observed, exists := detailedBounds[key]
		if !exists || observed != expected {
			return failure("Detailed boundaries do not cover the exact native Timestep reporting period")
		}
	}
	for key := range detailedBounds {
		if _, exists := zoneBounds[key]; len(zoneBounds) > 0 && !exists {
			return failure("Detailed observations include an unrelated native environment")
		}
	}
	for endpoint, frame := range timestepEnds {
		if !ends[endpoint] {
			return failure(frame.label + ": no exact native status at the Timestep endpoint")
		}
	}
	return ""
}
