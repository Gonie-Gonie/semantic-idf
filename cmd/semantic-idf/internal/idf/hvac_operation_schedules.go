package idf

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// HVACOperationSchedule preserves configured schedule values, not operating
// results. Dates use the leap-year calendar (1..366) used by EnergyPlus. Unknown
// schedules deliberately contain no fallback value or executable intervals.
type HVACOperationSchedule struct {
	Name          string                          `json:"name"`
	ObjectIndex   int                             `json:"objectIndex"`
	ObjectType    string                          `json:"objectType"`
	Kind          string                          `json:"kind"`
	Value         *float64                        `json:"value,omitempty"`
	Intervals     []HVACOperationScheduleInterval `json:"intervals,omitempty"`
	Rules         []HVACOperationScheduleRule     `json:"rules,omitempty"`
	WeekDays      map[string]string               `json:"weekDays,omitempty"`
	Periods       []HVACOperationSchedulePeriod   `json:"periods,omitempty"`
	Interpolation string                          `json:"interpolation,omitempty"`
	Status        string                          `json:"status"`
	Reason        string                          `json:"reason,omitempty"`
}

type HVACOperationScheduleInterval struct {
	UntilMinute int     `json:"untilMinute"`
	Value       float64 `json:"value"`
}

type HVACOperationScheduleRule struct {
	StartDay     int                             `json:"startDay"`
	EndDay       int                             `json:"endDay"`
	Selector     string                          `json:"selector"`
	Intervals    []HVACOperationScheduleInterval `json:"intervals,omitempty"`
	ScheduleName string                          `json:"scheduleName,omitempty"`
}

type HVACOperationSchedulePeriod struct {
	StartDay     int    `json:"startDay"`
	EndDay       int    `json:"endDay"`
	ScheduleName string `json:"scheduleName"`
}

var hvacOperationScheduleDayTypes = []string{
	"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday",
	"holiday", "summerdesignday", "winterdesignday", "customday1", "customday2",
}

// The field positions below are the canonical IDD positions, independent of
// comments. The shared manual catalog currently has no Schedule:* definitions.
// Reference: EnergyPlus InputOutputReference, Group Schedules, and v25.1.0
// src/EnergyPlus/ScheduleManager.cc. Unlike the profile overview, this compiler
// must not turn malformed, externally actuated or interpolated data into 1.0.
func buildHVACOperationSchedules(doc Document) []HVACOperationSchedule {
	actuated := hvacOperationActuatedSchedules(doc)
	result := make([]HVACOperationSchedule, 0)
	byKey := map[string][]int{}
	for _, obj := range doc.Objects {
		typ := normalizeName(obj.Type)
		if !strings.HasPrefix(typ, "schedule:") && !(strings.HasPrefix(typ, "externalinterface:") && strings.HasSuffix(typ, ":schedule")) {
			continue
		}
		schedule := compileHVACOperationSchedule(obj)
		if actuated[normalizeName(schedule.Name)] && hvacOperationScheduleNamespace(schedule) == "annual" {
			schedule = hvacOperationUnknownSchedule(schedule, "Schedule value can be overridden by an EMS or external actuator; runtime data is required.")
		}
		key := hvacOperationScheduleNamespace(schedule) + "|" + normalizeName(schedule.Name)
		byKey[key] = append(byKey[key], len(result))
		result = append(result, schedule)
	}
	for _, indexes := range byKey {
		if len(indexes) > 1 {
			for _, index := range indexes {
				result[index] = hvacOperationUnknownSchedule(result[index], "Duplicate schedule name in the same schedule namespace.")
			}
		}
	}
	// Resolve dependencies in namespace order: weeks depend on days, years on
	// weeks. Names are only unique within their annual/week/day namespace.
	for _, namespace := range []string{"week", "annual"} {
		for index := range result {
			schedule := &result[index]
			if schedule.Status != "supported" || hvacOperationScheduleNamespace(*schedule) != namespace {
				continue
			}
			refs := make([]string, 0)
			for _, dayType := range hvacOperationScheduleDayTypes {
				if name := schedule.WeekDays[dayType]; name != "" {
					refs = append(refs, name)
				}
			}
			for _, rule := range schedule.Rules {
				if rule.ScheduleName != "" {
					refs = append(refs, rule.ScheduleName)
				}
			}
			for _, period := range schedule.Periods {
				refs = append(refs, period.ScheduleName)
			}
			targetNamespace := "day"
			if namespace == "annual" {
				targetNamespace = "week"
			}
			for _, name := range refs {
				indexes := byKey[targetNamespace+"|"+normalizeName(name)]
				if len(indexes) != 1 {
					*schedule = hvacOperationUnknownSchedule(*schedule, fmt.Sprintf("Referenced %s schedule %q is missing or ambiguous.", targetNamespace, name))
					break
				}
				if target := result[indexes[0]]; target.Status != "supported" {
					*schedule = hvacOperationUnknownSchedule(*schedule, fmt.Sprintf("Referenced %s schedule %q cannot be evaluated: %s", targetNamespace, name, target.Reason))
					break
				}
			}
		}
	}
	return result
}

func hvacOperationScheduleNamespace(schedule HVACOperationSchedule) string {
	typ := normalizeName(schedule.ObjectType)
	if strings.HasPrefix(typ, "schedule:day:") {
		return "day"
	}
	if strings.HasPrefix(typ, "schedule:week:") {
		return "week"
	}
	return "annual"
}

func hvacOperationActuatedSchedules(doc Document) map[string]bool {
	result := map[string]bool{}
	for _, obj := range doc.Objects {
		typ := normalizeName(obj.Type)
		if typ != "energymanagementsystem:actuator" && !(strings.HasPrefix(typ, "externalinterface:") && strings.HasSuffix(typ, ":actuator")) {
			continue
		}
		// All EMS/external actuator variants share these first four fields.
		if strings.HasPrefix(normalizeName(hvacFieldValue(obj, 2)), "schedule:") && strings.EqualFold(hvacFieldValue(obj, 3), "Schedule Value") {
			result[normalizeName(hvacFieldValue(obj, 1))] = true
		}
	}
	return result
}

func hvacOperationUnknownSchedule(schedule HVACOperationSchedule, reason string) HVACOperationSchedule {
	schedule.Status, schedule.Reason = "unknown", reason
	schedule.Value, schedule.Intervals, schedule.Rules, schedule.WeekDays, schedule.Periods = nil, nil, nil, nil, nil
	return schedule
}

func compileHVACOperationSchedule(obj Object) HVACOperationSchedule {
	schedule := HVACOperationSchedule{Name: objectName(obj), ObjectIndex: obj.Index, ObjectType: obj.Type, Kind: "unsupported", Status: "supported"}
	unknown := func(reason string) HVACOperationSchedule { return hvacOperationUnknownSchedule(schedule, reason) }
	if schedule.Name == "" {
		return unknown("Schedule name is missing.")
	}
	typ := normalizeName(obj.Type)
	switch typ {
	case "schedule:constant", "schedule:day:constant":
		schedule.Kind = "constant"
		if typ == "schedule:day:constant" {
			schedule.Kind = "day"
		}
		value, ok := hvacOperationScheduleNumber(hvacFieldValue(obj, 2))
		if !ok || len(obj.Fields) != 3 {
			return unknown("Constant schedule requires one finite numeric value.")
		}
		schedule.Value = &value
		if schedule.Kind == "day" {
			schedule.Intervals = []HVACOperationScheduleInterval{{UntilMinute: 1440, Value: value}}
		}
	case "schedule:day:hourly":
		schedule.Kind = "day"
		if len(obj.Fields) != 26 {
			return unknown("Hourly day schedule requires exactly 24 numeric values.")
		}
		for index := 2; index < len(obj.Fields); index++ {
			value, ok := hvacOperationScheduleNumber(obj.Fields[index].Value)
			if !ok {
				return unknown("Hourly day schedule contains a missing or invalid value.")
			}
			schedule.Intervals = append(schedule.Intervals, HVACOperationScheduleInterval{UntilMinute: (index - 1) * 60, Value: value})
		}
	case "schedule:day:interval":
		schedule.Kind = "day"
		if !hvacOperationScheduleInterpolation(&schedule, hvacFieldValue(obj, 2)) {
			return unknown("Interpolated schedule requires timestep evaluation; configured step values are not substituted.")
		}
		intervals, ok := hvacOperationScheduleIntervals(obj.Fields, 3, false)
		if !ok {
			return unknown("Day intervals must increase strictly and cover the full day through 24:00.")
		}
		schedule.Intervals = intervals
	case "schedule:day:list":
		schedule.Kind = "day"
		if !hvacOperationScheduleInterpolation(&schedule, hvacFieldValue(obj, 2)) {
			return unknown("Interpolated schedule requires timestep evaluation; configured step values are not substituted.")
		}
		minutes, err := strconv.Atoi(hvacFieldValue(obj, 3))
		if err != nil || minutes <= 0 || minutes > 60 || 60%minutes != 0 || len(obj.Fields)-4 != 1440/minutes {
			return unknown("Day list must contain exactly 24 hours of values at a minute interval that divides 60.")
		}
		for index := 4; index < len(obj.Fields); index++ {
			value, ok := hvacOperationScheduleNumber(obj.Fields[index].Value)
			if !ok {
				return unknown("Day list contains a missing or invalid value.")
			}
			schedule.Intervals = append(schedule.Intervals, HVACOperationScheduleInterval{UntilMinute: (index - 3) * minutes, Value: value})
		}
	case "schedule:compact":
		schedule.Kind = "compact"
		if reason := hvacOperationCompileCompact(obj, &schedule); reason != "" {
			return unknown(reason)
		}
	case "schedule:week:daily":
		schedule.Kind = "week"
		if len(obj.Fields) != 13 {
			return unknown("Daily week schedule requires all 12 day-type schedule references.")
		}
		schedule.WeekDays = map[string]string{}
		for index, dayType := range hvacOperationScheduleDayTypes {
			name := hvacFieldValue(obj, index+1)
			if name == "" {
				return unknown("Daily week schedule is missing a day-type schedule reference.")
			}
			schedule.WeekDays[dayType] = name
		}
	case "schedule:week:compact":
		schedule.Kind = "week"
		if len(obj.Fields) < 3 || (len(obj.Fields)-1)%2 != 0 {
			return unknown("Compact week schedule requires complete day-selector and day-schedule pairs.")
		}
		assigned := map[string]bool{}
		for index := 1; index < len(obj.Fields); index += 2 {
			selector := hvacFieldValue(obj, index)
			if body, ok := hvacOperationScheduleDirective(selector, "for"); ok {
				selector = body
			}
			days, ok := hvacOperationScheduleSelector(selector, assigned)
			name := hvacFieldValue(obj, index+1)
			if !ok || name == "" {
				return unknown("Compact week schedule contains an invalid/overlapping day selector or missing day schedule.")
			}
			schedule.Rules = append(schedule.Rules, HVACOperationScheduleRule{StartDay: 1, EndDay: 366, Selector: strings.Join(days, " "), ScheduleName: name})
		}
		if len(assigned) != len(hvacOperationScheduleDayTypes) {
			return unknown("Compact week schedule does not define all 12 day types.")
		}
	case "schedule:year":
		schedule.Kind = "year"
		if reason := hvacOperationCompileYear(obj, &schedule); reason != "" {
			return unknown(reason)
		}
	case "schedule:file":
		return unknown("External file schedule requires simulation data or an evaluated external file.")
	default:
		if strings.HasPrefix(typ, "externalinterface:") {
			return unknown("External interface sets schedule values at runtime; its initial value is not an operating schedule.")
		}
		return unknown("This schedule type is not evaluated for operating availability.")
	}
	return schedule
}

func hvacOperationScheduleNumber(value string) (float64, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func hvacOperationScheduleInterpolation(schedule *HVACOperationSchedule, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "no") {
		schedule.Interpolation = "No"
		return true
	}
	schedule.Interpolation = value
	return false
}

func hvacOperationScheduleDirective(value, directive string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) <= len(directive) || !strings.EqualFold(value[:len(directive)], directive) {
		return "", false
	}
	suffix := value[len(directive):]
	if suffix[0] != ':' && suffix[0] != ' ' && suffix[0] != '\t' {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(suffix), ":")), true
}

func hvacOperationScheduleMinute(value string) (int, bool) {
	if body, ok := hvacOperationScheduleDirective(value, "until"); ok {
		value = body
	}
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, false
	}
	hour, hourErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	minute, minuteErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 24 || minute < 0 || minute > 59 || hour == 24 && minute != 0 {
		return 0, false
	}
	return hour*60 + minute, true
}

func hvacOperationScheduleIntervals(fields []Field, start int, titled bool) ([]HVACOperationScheduleInterval, bool) {
	if start >= len(fields) || (len(fields)-start)%2 != 0 {
		return nil, false
	}
	intervals := make([]HVACOperationScheduleInterval, 0, (len(fields)-start)/2)
	previous := 0
	for index := start; index < len(fields); index += 2 {
		if titled {
			if _, ok := hvacOperationScheduleDirective(fields[index].Value, "until"); !ok {
				return nil, false
			}
		}
		minute, timeOK := hvacOperationScheduleMinute(fields[index].Value)
		value, valueOK := hvacOperationScheduleNumber(fields[index+1].Value)
		if !timeOK || !valueOK || minute <= previous {
			return nil, false
		}
		intervals = append(intervals, HVACOperationScheduleInterval{UntilMinute: minute, Value: value})
		previous = minute
	}
	return intervals, previous == 1440
}

// AllOtherDays is resolved only against the day types already assigned within
// this Through period/week. Emitting explicit selectors avoids client ambiguity.
func hvacOperationScheduleSelector(selector string, assigned map[string]bool) ([]string, bool) {
	selected := map[string]bool{}
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(selector)))
	if len(tokens) == 0 {
		return nil, false
	}
	for _, token := range tokens {
		days := []string{}
		switch token {
		case "alldays":
			days = hvacOperationScheduleDayTypes
		case "allotherdays":
			for _, day := range hvacOperationScheduleDayTypes {
				if !assigned[day] {
					days = append(days, day)
				}
			}
		case "weekdays":
			days = hvacOperationScheduleDayTypes[1:6]
		case "weekends":
			days = []string{"sunday", "saturday"}
		case "holidays":
			days = []string{"holiday"}
		default:
			for _, day := range hvacOperationScheduleDayTypes {
				if token == day {
					days = []string{day}
					break
				}
			}
			if len(days) == 0 {
				return nil, false
			}
		}
		for _, day := range days {
			if assigned[day] {
				return nil, false
			}
			selected[day] = true
		}
	}
	days := make([]string, 0, len(selected))
	for _, day := range hvacOperationScheduleDayTypes {
		if selected[day] {
			days = append(days, day)
			assigned[day] = true
		}
	}
	return days, len(days) > 0
}

func hvacOperationScheduleDay(month, day int) (int, bool) {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return 0, false
	}
	date := time.Date(2000, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return date.YearDay(), int(date.Month()) == month && date.Day() == day
}

func hvacOperationScheduleMonthDay(value string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) != 2 {
		return 0, false
	}
	month, monthErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	day, dayErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if monthErr != nil || dayErr != nil {
		return 0, false
	}
	return hvacOperationScheduleDay(month, day)
}

func hvacOperationCompileCompact(obj Object, schedule *HVACOperationSchedule) string {
	previousEnd, periodStart, periodEnd := 0, 0, 0
	assigned := map[string]bool{}
	for index := 2; index < len(obj.Fields); {
		value := hvacFieldValue(obj, index)
		if through, ok := hvacOperationScheduleDirective(value, "through"); ok {
			if periodEnd > 0 && len(assigned) != len(hvacOperationScheduleDayTypes) {
				return "Compact schedule does not define all 12 day types in a Through period."
			}
			endDay, dateOK := hvacOperationScheduleMonthDay(through)
			if !dateOK || endDay <= previousEnd {
				return "Compact Through dates must be valid and strictly increasing."
			}
			periodStart, periodEnd, previousEnd = previousEnd+1, endDay, endDay
			assigned = map[string]bool{}
			index++
			continue
		}
		selector, selectorOK := hvacOperationScheduleDirective(value, "for")
		if !selectorOK || periodEnd == 0 {
			return "Compact schedule requires Through, For, optional Interpolate, and complete Until/value pairs."
		}
		days, ok := hvacOperationScheduleSelector(selector, assigned)
		if !ok {
			return "Compact schedule contains an invalid or overlapping day selector."
		}
		index++
		if index < len(obj.Fields) {
			if interpolation, ok := hvacOperationScheduleDirective(hvacFieldValue(obj, index), "interpolate"); ok {
				if !hvacOperationScheduleInterpolation(schedule, interpolation) {
					return "Interpolated schedule requires timestep evaluation; configured step values are not substituted."
				}
				index++
			}
		}
		start := index
		for index < len(obj.Fields) {
			if _, ok := hvacOperationScheduleDirective(hvacFieldValue(obj, index), "through"); ok {
				break
			}
			if _, ok := hvacOperationScheduleDirective(hvacFieldValue(obj, index), "for"); ok {
				break
			}
			index++
		}
		intervals, ok := hvacOperationScheduleIntervals(obj.Fields[start:index], 0, true)
		if !ok {
			return "Compact Until intervals must increase strictly and cover the full day through 24:00."
		}
		schedule.Rules = append(schedule.Rules, HVACOperationScheduleRule{StartDay: periodStart, EndDay: periodEnd, Selector: strings.Join(days, " "), Intervals: intervals})
	}
	if periodEnd != 366 || len(assigned) != len(hvacOperationScheduleDayTypes) || len(schedule.Rules) == 0 {
		return "Compact schedule must cover the entire year and all 12 day types."
	}
	if schedule.Interpolation == "" {
		schedule.Interpolation = "No"
	}
	return ""
}

func hvacOperationCompileYear(obj Object, schedule *HVACOperationSchedule) string {
	if len(obj.Fields) < 7 || (len(obj.Fields)-2)%5 != 0 {
		return "Year schedule requires complete week schedule and start/end date groups."
	}
	coverage := make([]int, 367)
	for index := 2; index < len(obj.Fields); index += 5 {
		name := hvacFieldValue(obj, index)
		start, startOK := hvacOperationScheduleMonthDay(hvacFieldValue(obj, index+1) + "/" + hvacFieldValue(obj, index+2))
		end, endOK := hvacOperationScheduleMonthDay(hvacFieldValue(obj, index+3) + "/" + hvacFieldValue(obj, index+4))
		if name == "" || !startOK || !endOK {
			return "Year schedule contains an invalid date or missing week schedule."
		}
		ranges := [][2]int{{start, end}}
		if start > end {
			ranges = [][2]int{{start, 366}, {1, end}}
		}
		for _, dates := range ranges {
			schedule.Periods = append(schedule.Periods, HVACOperationSchedulePeriod{StartDay: dates[0], EndDay: dates[1], ScheduleName: name})
			for day := dates[0]; day <= dates[1]; day++ {
				coverage[day]++
			}
		}
	}
	// EnergyPlus fills an otherwise unassigned February 29 from February 28.
	if coverage[60] == 0 && coverage[59] == 1 {
		for _, period := range schedule.Periods {
			if period.StartDay <= 59 && period.EndDay >= 59 {
				schedule.Periods = append(schedule.Periods, HVACOperationSchedulePeriod{StartDay: 60, EndDay: 60, ScheduleName: period.ScheduleName})
				coverage[60] = 1
				break
			}
		}
	}
	for day := 1; day <= 366; day++ {
		if coverage[day] != 1 {
			return "Year schedule has missing or overlapping dates."
		}
	}
	sort.Slice(schedule.Periods, func(i, j int) bool { return schedule.Periods[i].StartDay < schedule.Periods[j].StartDay })
	return ""
}
