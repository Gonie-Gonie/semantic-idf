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
	axis := map[int]string{}
	conflicts := map[int]bool{}
	for _, series := range result.ControlSeries {
		if fileKey(series.File) != primary {
			continue
		}
		for _, point := range series.Points {
			name := label(point)
			if previous, exists := axis[point.X]; exists && previous != name {
				conflicts[point.X] = true
			}
			axis[point.X] = name
		}
	}
	rows := make([]int, 0, len(axis))
	labelCount := map[string]int{}
	for row, name := range axis {
		rows = append(rows, row)
		labelCount[name]++
	}
	sort.Ints(rows)
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
	for i := range result.ControlSignals {
		binding := &result.ControlSignals[i]
		if !binding.Required || binding.Status != "collected" {
			continue
		}
		identity := binding.SeriesID + "\x00" + binding.Role + "\x00" + binding.Domain
		message, checked := validation[identity]
		if !checked {
			series, index := byID[binding.SeriesID], indexes[binding.SeriesID]
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
					case binding.Domain == "manager_status" && (value < 0 || value > 3), binding.Domain == "hybrid_ventilation_status" && (value < 0 || value > 2):
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
				message = fmt.Sprintf("%d of %d required Timestep intervals are missing, ambiguous or invalid (%s). Run HVAC Loop Check again and inspect this exact source output.", invalid, len(rows), first)
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
