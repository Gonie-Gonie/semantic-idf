package idf

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

func profileIsSetpointDimension(dimension string) bool {
	return dimension == ProfileDimensionHeatingSetpoint || dimension == ProfileDimensionCoolingSetpoint
}

type profileThermostatBinding struct {
	object        Object
	zones         []string
	control       string
	schedules     map[int]map[string]string
	warnings      []string
	invalidTarget bool
}

// Temperature schedules are engineering values, not load multipliers. Resolve
// the thermostat's exact typed references and control schedule before creating
// one item per zone/dimension. Unbound definitions never become zone profiles.
func (ctx *profileContext) collectProfileSetpointItems() []ProfileItem {
	objects := map[string][]Object{}
	modifiers := map[string][]string{}
	for _, obj := range ctx.doc.Objects {
		key := normalizeName(obj.Type) + "|" + normalizeName(objectName(obj))
		objects[key] = append(objects[key], obj)
		typ := normalizeName(obj.Type)
		if typ == "zonecontrol:thermostat:operativetemperature" || typ == "zonecontrol:thermostat:temperatureandhumidity" {
			target := normalizeName(hvacFieldValue(obj, 0))
			modifiers[target] = append(modifiers[target], obj.Type)
		}
	}
	bindings := []profileThermostatBinding{}
	owners := map[string]int{}
	for _, obj := range ctx.doc.Objects {
		typ := normalizeName(obj.Type)
		if typ != "zonecontrol:thermostat" && typ != "zonecontrol:thermostat:stageddualsetpoint" && typ != "zonecontrol:thermostat:thermalcomfort" {
			continue
		}
		binding := profileThermostatBinding{object: obj, schedules: map[int]map[string]string{}}
		target := hvacFieldValue(obj, 1)
		if len(objects["zone|"+normalizeName(target)])+len(objects["zonelist|"+normalizeName(target)]) != 1 {
			binding.invalidTarget = true
			binding.warnings = append(binding.warnings, "Thermostat zone or ZoneList name is missing or ambiguous; no temperature is assigned.")
		}
		if _, ok := ctx.zoneByKey[normalizeName(target)]; ok {
			binding.zones = []string{target}
		} else if members, ok := ctx.zoneLists[normalizeName(target)]; ok {
			for _, name := range cleanProfileNames(members) {
				if _, found := ctx.zoneByKey[normalizeName(name)]; found {
					binding.zones = append(binding.zones, name)
					if len(objects["zone|"+normalizeName(name)]) != 1 {
						binding.invalidTarget = true
						binding.warnings = append(binding.warnings, "Thermostat ZoneList contains an ambiguous zone: "+name+".")
					}
				} else {
					binding.warnings = append(binding.warnings, "Thermostat ZoneList contains an unresolved member: "+name+".")
				}
			}
		} else {
			ctx.warnings = append(ctx.warnings, profileWarning("warning", "unresolved_setpoint_zone", "Thermostat zone or ZoneList reference is missing or ambiguous: "+target+".", "", "", obj))
			continue
		}
		switch typ {
		case "zonecontrol:thermostat":
			binding.control = hvacFieldValue(obj, 2)
			for index := 3; index <= 9 && index < len(obj.Fields); index += 2 {
				setpointType, name := hvacFieldValue(obj, index), hvacFieldValue(obj, index+1)
				if setpointType == "" && name == "" {
					continue
				}
				mode := profileThermostatMode(setpointType)
				matches := objects[normalizeName(setpointType)+"|"+normalizeName(name)]
				if mode == 0 || len(matches) != 1 {
					binding.warnings = append(binding.warnings, "Thermostat setpoint reference is unsupported, missing or ambiguous: "+setpointType+" / "+name+".")
					continue
				}
				if _, exists := binding.schedules[mode]; exists {
					binding.schedules[mode] = nil
					binding.warnings = append(binding.warnings, "Thermostat repeats a control-type definition; its temperature is unresolved.")
					continue
				}
				setpoint := matches[0]
				switch mode {
				case 1:
					binding.schedules[mode] = map[string]string{ProfileDimensionHeatingSetpoint: hvacFieldValue(setpoint, 1)}
				case 2:
					binding.schedules[mode] = map[string]string{ProfileDimensionCoolingSetpoint: hvacFieldValue(setpoint, 1)}
				case 3:
					binding.schedules[mode] = map[string]string{ProfileDimensionHeatingSetpoint: hvacFieldValue(setpoint, 1), ProfileDimensionCoolingSetpoint: hvacFieldValue(setpoint, 1)}
				case 4:
					binding.schedules[mode] = map[string]string{ProfileDimensionHeatingSetpoint: hvacFieldValue(setpoint, 1), ProfileDimensionCoolingSetpoint: hvacFieldValue(setpoint, 2)}
				}
			}
		case "zonecontrol:thermostat:stageddualsetpoint":
			// Canonical IDD positions: number of heating stages precedes the
			// heating schedule; all four heating offsets precede cooling stages.
			binding.schedules[4] = map[string]string{ProfileDimensionHeatingSetpoint: hvacFieldValue(obj, 3), ProfileDimensionCoolingSetpoint: hvacFieldValue(obj, 10)}
		case "zonecontrol:thermostat:thermalcomfort":
			binding.warnings = append(binding.warnings, "Thermal-comfort control does not directly define a dry-bulb temperature schedule; runtime temperature conversion is not substituted.")
		}
		for _, zone := range binding.zones {
			owners[normalizeName(zone)]++
		}
		bindings = append(bindings, binding)
	}
	if len(bindings) == 0 {
		return nil
	}
	for index := range bindings {
		binding := &bindings[index]
		targets := []string{objectName(binding.object)}
		for _, zone := range binding.zones {
			targets = append(targets, zone+" "+objectName(binding.object))
		}
		for _, target := range targets {
			for _, typ := range modifiers[normalizeName(target)] {
				binding.warnings = append(binding.warnings, "Configured base temperature schedules are shown; "+typ+" can modify the effective thermostat target during simulation.")
			}
		}
	}
	compiler := newProfileSetpointSchedules(ctx.doc)
	items := []ProfileItem{}
	for _, binding := range bindings {
		for _, dimension := range []string{ProfileDimensionHeatingSetpoint, ProfileDimensionCoolingSetpoint} {
			values, names, reasons := compiler.thermostatValues(binding, dimension)
			for _, zoneName := range binding.zones {
				zone := ctx.zoneByKey[normalizeName(zoneName)]
				actual := values
				warnings := append(append([]string(nil), binding.warnings...), reasons...)
				if binding.invalidTarget || owners[normalizeName(zone.name)] > 1 {
					actual = make([]*float64, 8760)
					if owners[normalizeName(zone.name)] > 1 {
						warnings = append(warnings, "Multiple thermostat controllers target this zone; their temperatures are not added or arbitrarily selected.")
					}
				}
				item := ProfileItem{
					ID:       fmt.Sprintf("profile-item-%d-%s-%s", binding.object.Index, safeID(zone.name), dimension),
					ZoneName: zone.name, Dimension: dimension, ObjectIndex: binding.object.Index,
					ObjectType: binding.object.Type, ObjectName: objectName(binding.object),
					SourceTarget: hvacFieldValue(binding.object, 1), SourceTargetKind: "Zone",
					ScheduleName: strings.Join(names, " + "), SchedulePattern: "temperature", RawMethod: "Thermostat schedule",
					CloneEligible: false, setpointValues: actual,
				}
				if _, exists := ctx.zoneLists[normalizeName(item.SourceTarget)]; exists {
					item.SourceTargetKind = "ZoneList"
				}
				item.ScheduleHash = profileSetpointHash(actual, "")
				item.AggregationSignature = item.ScheduleHash
				mean, known := profileKnownTemperatureMean(actual)
				metric := profileMetric("temperature", "Setpoint temperature", "degC", mean, known > 0, 2)
				if len(warnings) > 0 && known > 0 {
					metric.Status = metricStatusPartial
				}
				item.Normalized, item.DisplayMetric = []ProfileMetric{metric}, metric
				for _, warning := range cleanProfileNames(warnings) {
					item.Warnings = append(item.Warnings, profileWarning("warning", "unresolved_setpoint_profile", warning, zone.name, dimension, binding.object))
				}
				items = append(items, item)
			}
		}
	}
	return items
}

func profileThermostatMode(objectType string) int {
	switch normalizeName(objectType) {
	case "thermostatsetpoint:singleheating":
		return 1
	case "thermostatsetpoint:singlecooling":
		return 2
	case "thermostatsetpoint:singleheatingorcooling":
		return 3
	case "thermostatsetpoint:dualsetpoint":
		return 4
	}
	return 0
}

type profileSetpointSchedules struct {
	byKey    map[string][]HVACOperationSchedule
	dayCache map[string][]HVACOperationScheduleInterval
}

func newProfileSetpointSchedules(doc Document) *profileSetpointSchedules {
	compiler := &profileSetpointSchedules{byKey: map[string][]HVACOperationSchedule{}, dayCache: map[string][]HVACOperationScheduleInterval{}}
	for _, schedule := range buildHVACOperationSchedules(doc) {
		key := hvacOperationScheduleNamespace(schedule) + "|" + normalizeName(schedule.Name)
		compiler.byKey[key] = append(compiler.byKey[key], schedule)
	}
	return compiler
}

// Day/week/year dependencies have at most three levels. Cache immutable daily
// intervals shared by every thermostat and zone to bound calendar work.
func (compiler *profileSetpointSchedules) day(name, namespace string, date time.Time, depth int) []HVACOperationScheduleInterval {
	key := namespace + "|" + normalizeName(name) + "|" + strconv.Itoa(date.YearDay())
	if values, exists := compiler.dayCache[key]; exists {
		return values
	}
	if depth > 3 {
		return nil
	}
	matches := compiler.byKey[namespace+"|"+normalizeName(name)]
	if len(matches) != 1 || matches[0].Status != "supported" {
		return nil
	}
	schedule := matches[0]
	var intervals []HVACOperationScheduleInterval
	day, _ := hvacOperationScheduleDay(int(date.Month()), date.Day())
	dayType := strings.ToLower(date.Weekday().String())
	switch schedule.Kind {
	case "constant":
		if schedule.Value != nil {
			intervals = []HVACOperationScheduleInterval{{UntilMinute: 1440, Value: *schedule.Value}}
		}
	case "day":
		intervals = schedule.Intervals
	case "compact":
		for _, rule := range schedule.Rules {
			if day >= rule.StartDay && day <= rule.EndDay && profileSetpointSelectorHas(rule.Selector, dayType) {
				intervals = rule.Intervals
				break
			}
		}
	case "week":
		child := schedule.WeekDays[dayType]
		if child == "" {
			for _, rule := range schedule.Rules {
				if profileSetpointSelectorHas(rule.Selector, dayType) {
					child = rule.ScheduleName
					break
				}
			}
		}
		intervals = compiler.day(child, "day", date, depth+1)
	case "year":
		for _, period := range schedule.Periods {
			if day >= period.StartDay && day <= period.EndDay {
				intervals = compiler.day(period.ScheduleName, "week", date, depth+1)
				break
			}
		}
	}
	compiler.dayCache[key] = intervals
	return intervals
}

func profileSetpointSelectorHas(selector, dayType string) bool {
	for _, day := range strings.Fields(selector) {
		if day == dayType {
			return true
		}
	}
	return false
}

func (compiler *profileSetpointSchedules) thermostatValues(binding profileThermostatBinding, dimension string) ([]*float64, []string, []string) {
	values := make([]*float64, 8760)
	names := []string{}
	reasons := map[string]bool{}
	if binding.control != "" {
		names = append(names, binding.control)
	}
	for mode := 1; mode <= 4; mode++ {
		if name := binding.schedules[mode][dimension]; name != "" {
			names = append(names, name)
		}
	}
	names = cleanProfileNames(names)
	if len(binding.schedules) == 0 {
		return values, names, []string{"No supported thermostat temperature definition is available."}
	}
	for dayIndex := 0; dayIndex < 365; dayIndex++ {
		date := time.Date(2001, 1, 1+dayIndex, 0, 0, 0, 0, time.UTC)
		control := []HVACOperationScheduleInterval{{UntilMinute: 1440, Value: 4}}
		if normalizeName(binding.object.Type) == "zonecontrol:thermostat" {
			control = compiler.day(binding.control, "annual", date, 0)
		}
		if len(control) == 0 {
			reasons["Thermostat control-type schedule is missing, ambiguous or cannot be evaluated; no active temperature is assumed."] = true
			continue
		}
		targets := map[int][]HVACOperationScheduleInterval{}
		for mode, refs := range binding.schedules {
			if refs[dimension] != "" {
				targets[mode] = compiler.day(refs[dimension], "annual", date, 0)
			}
		}
		for hour := 0; hour < 24; hour++ {
			start, end := hour*60, (hour+1)*60
			bounds := []int{start, end}
			for _, intervals := range append([][]HVACOperationScheduleInterval{control}, profileSetpointIntervalLists(targets)...) {
				for _, interval := range intervals {
					if interval.UntilMinute > start && interval.UntilMinute < end {
						bounds = append(bounds, interval.UntilMinute)
					}
				}
			}
			sort.Ints(bounds)
			total, minutes, unknown := 0.0, 0, false
			for index := 1; index < len(bounds); index++ {
				if bounds[index] == bounds[index-1] {
					continue
				}
				minute := bounds[index]
				modeValue, ok := profileSetpointAt(control, minute)
				mode := int(modeValue)
				if !ok || modeValue != float64(mode) || mode < 0 || mode > 4 {
					unknown = true
					reasons["Thermostat control-type schedule contains an invalid mode; affected hours are unresolved."] = true
					continue
				}
				if mode == 0 || mode == 1 && dimension == ProfileDimensionCoolingSetpoint || mode == 2 && dimension == ProfileDimensionHeatingSetpoint {
					continue
				}
				value, known := profileSetpointAt(targets[mode], minute)
				if !known {
					unknown = true
					reasons["Active thermostat temperature schedule or control-type definition is missing, ambiguous or unsupported; affected hours are unresolved."] = true
					continue
				}
				length := bounds[index] - bounds[index-1]
				total += value * float64(length)
				minutes += length
			}
			if minutes > 0 && !unknown {
				value := total / float64(minutes)
				values[dayIndex*24+hour] = &value
			}
		}
	}
	return values, names, sortedStringSet(reasons)
}

func profileSetpointIntervalLists(targets map[int][]HVACOperationScheduleInterval) [][]HVACOperationScheduleInterval {
	lists := make([][]HVACOperationScheduleInterval, 0, len(targets))
	for _, values := range targets {
		lists = append(lists, values)
	}
	return lists
}

func profileSetpointAt(intervals []HVACOperationScheduleInterval, minute int) (float64, bool) {
	for _, interval := range intervals {
		if minute <= interval.UntilMinute {
			return interval.Value, true
		}
	}
	return 0, false
}

func profileKnownTemperatureMean(values []*float64) (float64, int) {
	total, count := 0.0, 0
	for _, value := range values {
		if value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) {
			total += *value
			count++
		}
	}
	if count == 0 {
		return 0, 0
	}
	return total / float64(count), count
}

func profileSetpointHash(values []*float64, identity string) string {
	hash := sha1.New()
	_, _ = hash.Write([]byte(identity + "|"))
	for _, value := range values {
		text := "null"
		if value != nil {
			text = strconv.FormatFloat(*value, 'g', -1, 64)
		}
		_, _ = hash.Write([]byte(text + ","))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func profileSetpointMetricAggregate(items []ProfileItem, metricID string) profileMetricAggregate {
	candidate := profileMetricAggregate{id: metricID, label: "Setpoint temperature", unit: "degC"}
	// Two thermostats cannot be added, even if their scalar averages happen to
	// agree. The binding pass marks every conflicting source as missing.
	if len(items) != 1 {
		return candidate
	}
	metric := selectProfileMetric(items[0].Normalized, metricID)
	if metric.Status == metricStatusMissing {
		return candidate
	}
	candidate.value, candidate.resolvedCount = metric.Value, 1
	if metric.Status == metricStatusOK {
		candidate.completeCount = 1
	}
	return candidate
}

func profileSetpointGraphSeries(scopeType string, group ProfileGroup, zoneName string, dimension ProfileDimensionSummary, itemMap map[string]ProfileItem) ProfileGraphSeries {
	values := make([]*float64, 8760)
	indexes := []int{}
	warnings := []ProfileWarning{}
	if len(dimension.ItemIDs) == 1 {
		item := itemMap[dimension.ItemIDs[0]]
		if len(item.setpointValues) == 8760 {
			values = append([]*float64(nil), item.setpointValues...)
		}
	}
	for _, id := range dimension.ItemIDs {
		item := itemMap[id]
		indexes = append(indexes, item.ObjectIndex)
		warnings = append(warnings, item.Warnings...)
	}
	days := append(append(append([]*float64(nil), values[:24]...), values[120:144]...), values[144:168]...)
	months := make([]*float64, 12)
	offset := 0
	for month := 1; month <= 12; month++ {
		hours := time.Date(2001, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day() * 24
		if mean, count := profileKnownTemperatureMean(values[offset : offset+hours]); count > 0 {
			months[month-1] = &mean
		}
		offset += hours
	}
	duration := make([]*float64, 0, len(values))
	for _, value := range values {
		if value != nil {
			duration = append(duration, value)
		}
	}
	sort.Slice(duration, func(i, j int) bool { return *duration[i] > *duration[j] })
	for len(duration) < len(values) {
		duration = append(duration, nil)
	}
	peak := 0.0
	if duration[0] != nil {
		peak = *duration[0]
	}
	scopeID, label := safeID(zoneName), zoneName
	if scopeType == "group" {
		scopeID, label = group.ID, group.Name
	}
	return ProfileGraphSeries{
		ID: fmt.Sprintf("profile-series-%s-%s-%s", scopeType, safeID(scopeID), safeID(dimension.Dimension)), Label: fmt.Sprintf("%s / %s", label, dimension.Label),
		ScopeType: scopeType, GroupID: group.ID, GroupName: group.Name, ZoneName: zoneName, Dimension: dimension.Dimension, DimensionLabel: dimension.Label,
		MetricID: dimension.MetricID, MetricLabel: dimension.MetricLabel, Unit: dimension.Unit, DesignValue: dimension.Value, DisplayValue: dimension.DisplayValue,
		ScheduleName: dimension.ScheduleName, ScheduleHash: dimension.ScheduleHash, SchedulePattern: dimension.SchedulePattern,
		DayValueProfile: days, WeekValueProfile: append([]*float64(nil), values[:168]...), MonthValueProfile: months, AnnualValueProfile: values, DurationValueProfile: duration,
		SourceItemIDs: append([]string(nil), dimension.ItemIDs...), SourceObjectIndexes: uniqueInts(indexes), Peak: peak, Status: dimension.Status, Warnings: warnings,
	}
}
