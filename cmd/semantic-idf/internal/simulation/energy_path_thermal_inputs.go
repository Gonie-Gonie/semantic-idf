package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Boundary inputs are measured context, not another heat-driver pressure or a
// conserved extension of the allocated driver -> delivered-load diagram.
type energyPathThermalInputDefinition struct {
	kind, label, variable string
	incident, internal    bool
}

var energyPathThermalInputDefinitions = []energyPathThermalInputDefinition{
	{kind: "input.solar_incident", label: "Incident solar at exterior surfaces", variable: "Surface Outside Face Incident Solar Radiation Rate per Area", incident: true},
	{kind: "input.exterior_convection", label: "Exterior surface convection", variable: "Surface Outside Face Convection Heat Gain Energy"},
	{kind: "input.exterior_longwave", label: "Exterior net longwave exchange", variable: "Surface Outside Face Net Thermal Radiation Heat Gain Energy"},
	{kind: "input.solar_absorbed", label: "Solar absorbed at exterior surfaces", variable: "Surface Outside Face Solar Radiation Heat Gain Energy"},
	{kind: "input.surface_storage", label: "Envelope surface heat storage (context)", variable: "Surface Heat Storage Energy"},
	{kind: "input.internal_gains", label: "Reported total internal gains", variable: "Zone Total Internal Total Heating Energy", internal: true},
}

func energyPathThermalInputVariable(name string) bool {
	for _, definition := range energyPathThermalInputDefinitions {
		if strings.EqualFold(strings.TrimSpace(name), definition.variable) {
			return true
		}
	}
	return false
}

// Output:Diagnostics is unique. Combine requested flags before ApplyOutput,
// which then merges this one request with any user-owned diagnostics object.
func mergeEnergyPathDiagnosticsAdditions(additions []idf.OutputObjectRequest) []idf.OutputObjectRequest {
	out := make([]idf.OutputObjectRequest, 0, len(additions))
	index := -1
	seen := map[string]bool{}
	for _, addition := range additions {
		if !strings.EqualFold(addition.ObjectType, "Output:Diagnostics") {
			out = append(out, addition)
			continue
		}
		if index < 0 {
			index = len(out)
			out = append(out, addition)
			out[index].Fields = nil
		}
		for _, field := range addition.Fields {
			key := normalizePurposeToken(field.Value)
			if key != "" && !seen[key] {
				out[index].Fields = append(out[index].Fields, idf.OutputFieldValue{Name: fmt.Sprintf("Key %d", len(out[index].Fields)+1), Value: field.Value})
				seen[key] = true
			}
		}
	}
	return out
}

func (builder *purposePlanBuilder) addEnergyPathThermalInputOutputs(zoneKeys []string) {
	selected := map[string]bool{}
	for _, zone := range zoneKeys {
		selected[normalizePurposeToken(zone)] = true
	}
	surfaces := energyPathThermalInputSurfaceRoster(&builder.geometry)
	surfaceKeys := make([]string, 0, len(surfaces))
	for key := range surfaces {
		surfaceKeys = append(surfaceKeys, key)
	}
	sort.Strings(surfaceKeys)
	for _, definition := range energyPathThermalInputDefinitions {
		frequency := "Monthly"
		if definition.incident {
			// Native monthly averages carry full-calendar-month Interval in some
			// EnergyPlus versions, including partial-month runs. Daily averages
			// have an explicit full-day duration and need 24x fewer rows than Hourly.
			frequency = "Daily"
		}
		if definition.internal {
			for _, zone := range zoneKeys {
				builder.addVariableWithReason(SimulationPurposeBasicEnergy, zone, definition.variable, frequency, "medium", "Measured thermal input context; never added to allocated heat-driver pressure.", "Basic Energy Path")
			}
			continue
		}
		for _, key := range surfaceKeys {
			surface := surfaces[key]
			if !selected[normalizePurposeToken(surface.zone)] || !definition.incident && surface.opening {
				continue
			}
			builder.addVariableWithReason(SimulationPurposeBasicEnergy, surface.name, definition.variable, frequency, "medium", "Measured exterior/envelope boundary context; not a conserved driver allocation.", "Basic Energy Path")
		}
	}
	builder.addObject(PurposeOutputObject{ObjectType: "Output:Diagnostics", Fields: []idf.OutputFieldValue{{Name: "Key 1", Value: "DisplayAdvancedReportVariables"}},
		PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Weight: "light", Description: "Enables native envelope surface heat-storage context.", Reason: "Basic Energy Path"})
}

type energyPathThermalInputSurface struct {
	name, zone, category string
	opening              bool
	area, multiplier     float64
	verified             bool
	ambiguous            bool
}

func energyPathThermalInputSurfaceRoster(geometry *idf.GeometryReport) map[string]energyPathThermalInputSurface {
	out := map[string]energyPathThermalInputSurface{}
	if geometry == nil {
		return out
	}
	base := map[string]idf.GeometrySurface{}
	add := func(surface energyPathThermalInputSurface) {
		key := normalizePurposeToken(surface.name)
		if key == "" {
			return
		}
		if _, exists := out[key]; exists {
			// An ambiguous identity must not be rescued by whichever geometry
			// happened to be enumerated last.
			surface = out[key]
			surface.ambiguous = true
		}
		out[key] = surface
	}
	for _, surface := range geometry.Surfaces {
		base[normalizePurposeToken(surface.ID)] = surface
		if surface.IsShading || !strings.EqualFold(strings.TrimSpace(surface.OutsideBoundary), "Outdoors") {
			continue
		}
		add(energyPathThermalInputSurface{name: surface.Name, zone: surface.ZoneName,
			category: energySurfaceDriverCategory(surface.SurfaceType, surface.Type, "outdoors", "Outdoors")})
	}
	for _, opening := range geometry.Windows {
		parent, exists := base[normalizePurposeToken(opening.BaseSurfaceID)]
		if !exists || !strings.EqualFold(strings.TrimSpace(parent.OutsideBoundary), "Outdoors") {
			continue
		}
		add(energyPathThermalInputSurface{name: opening.Name, zone: firstNonEmpty(opening.ZoneName, parent.ZoneName),
			category: energyDriverCategoryWindowsDoors, opening: true})
	}
	return out
}

type energyPathThermalInputTime struct {
	id    int64
	month int
	end   time.Time
	hours float64
	valid bool
}

type energyPathThermalInputAxes struct {
	monthly    map[int]energyPathThermalInputTime
	daily      map[int][]energyPathThermalInputTime
	hourly     map[int][]energyPathThermalInputTime
	weatherIDs map[int64]bool
}

// Each month is a native SQL report period, rather than a calendar-duration
// estimate. Duplicate month/year aliases and invalid native timestamps fail shut.
func readEnergyPathThermalInputAxes(db *sql.DB) (energyPathThermalInputAxes, error) {
	axes := energyPathThermalInputAxes{monthly: map[int]energyPathThermalInputTime{}, daily: map[int][]energyPathThermalInputTime{}, hourly: map[int][]energyPathThermalInputTime{}, weatherIDs: map[int64]bool{}}
	var count, environment int
	if err := db.QueryRow(`SELECT COUNT(*),COALESCE(MIN(EnvironmentPeriodIndex),0) FROM EnvironmentPeriods WHERE EnvironmentType=3`).Scan(&count, &environment); err != nil || count != 1 {
		return axes, fmt.Errorf("thermal input context requires one weather-run environment")
	}
	rows, err := db.Query(`SELECT TimeIndex,Year,Month,Day,Hour,Minute,"Interval",IntervalType FROM "Time"
WHERE EnvironmentPeriodIndex=? AND (WarmupFlag=0 OR WarmupFlag IS NULL) AND IntervalType IN (1,2,3) ORDER BY TimeIndex`, environment)
	if err != nil {
		return axes, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var year, month, day, hour, minute, intervalType sql.NullInt64
		var interval sql.NullFloat64
		if err := rows.Scan(&id, &year, &month, &day, &hour, &minute, &interval, &intervalType); err != nil {
			return axes, err
		}
		if !month.Valid || month.Int64 < 1 || month.Int64 > 12 {
			return axes, fmt.Errorf("invalid thermal input report month")
		}
		m := int(month.Int64)
		axes.weatherIDs[id] = true
		t := energyPathThermalInputTime{id: id, month: m}
		t.valid = year.Valid && year.Int64 > 0 && day.Valid && hour.Valid && minute.Valid && minute.Int64 == 0 && hour.Int64 >= 1 && hour.Int64 <= 24 &&
			day.Int64 >= 1 && day.Int64 <= int64(time.Date(int(year.Int64), time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day())
		if t.valid {
			t.end = time.Date(int(year.Int64), time.Month(m), int(day.Int64), int(hour.Int64), 0, 0, 0, time.UTC)
		}
		switch intervalType.Int64 {
		case 3:
			if _, duplicate := axes.monthly[m]; duplicate {
				t.valid = false
			}
			axes.monthly[m] = t
		case 2:
			t.hours = 24
			t.valid = t.valid && interval.Valid && interval.Float64 == 1440 && hour.Int64 == 24
			axes.daily[m] = append(axes.daily[m], t)
		case 1:
			t.hours = 1
			t.valid = t.valid && interval.Valid && interval.Float64 == 60
			axes.hourly[m] = append(axes.hourly[m], t)
		}
	}
	if err := rows.Err(); err != nil {
		return axes, err
	}
	// Global native axes detect a missing report row; individual dictionaries
	// must match every retained native row in their selected frequency/month.
	for _, axis := range []map[int][]energyPathThermalInputTime{axes.daily, axes.hourly} {
		for month, times := range axis {
			if _, present := axes.monthly[month]; !present {
				return axes, fmt.Errorf("missing native Monthly weather-run period")
			}
			valid := len(times) > 0
			for i, t := range times {
				valid = valid && t.valid
				if i > 0 && t.end.Sub(times[i-1].end) != time.Duration(t.hours)*time.Hour {
					valid = false
				}
			}
			period, present := axes.monthly[month]
			// EnergyPlus Monthly Time.Day, as well as Interval, can name the
			// full calendar month even when a weather run stops early. It must
			// not be used as the observed Daily/Hourly coverage endpoint.
			valid = valid && present && period.valid
			if len(times) > 0 && times[0].hours == 1 {
				valid = valid && times[0].end.Hour() == 1 && times[len(times)-1].end.Hour() == 0
			}
			if !valid {
				for i := range times {
					times[i].valid = false
				}
				axis[month] = times
			}
		}
	}
	for month, daily := range axes.daily {
		hourly := axes.hourly[month]
		if len(hourly) > 0 && (len(hourly) != 24*len(daily) || !hourly[0].valid || !hourly[len(hourly)-1].valid || !daily[0].end.Add(-23*time.Hour).Equal(hourly[0].end)) {
			for i := range daily {
				daily[i].valid = false
			}
			axes.daily[month] = daily
		}
	}
	return axes, nil
}

type energyPathThermalInputSeries struct {
	definition                         energyPathThermalInputDefinition
	id, key, zone, category, frequency string
	dictionary                         int
	area, multiplier                   float64
	values                             map[int]float64
	observed                           map[int]bool
}

type energyPathThermalInputValue struct {
	kind, label, zone, category string
	month                       int
	raw, effective              float64
	sourceIDs                   []string
	observed                    bool
}

func attachEnergyPathThermalInputs(result *EnergyExplanationResult, files []SimulationFileInfo, plan *PurposeRunPlan, context energyDriverBuildContext, geometry *idf.GeometryReport, document *idf.Document) {
	if result == nil || result.Schema != energyExplanationSchema {
		return
	}
	for _, file := range files {
		if file.Kind != "sqlite" && !strings.EqualFold(filepath.Ext(file.Name), ".sql") {
			continue
		}
		values, sources, months, err := readEnergyPathThermalInputs(file.Path, plan, context, geometry)
		if err != nil {
			result.Warnings = appendEnergyDriverWarning(result.Warnings, EnergyWarning{Severity: "warning", Code: "energy_path_thermal_inputs_unavailable", Message: "Measured thermal input context is unavailable: " + err.Error()})
			continue
		}
		result.Sources = append(result.Sources, sources...)
		appendEnergyPathThermalInputGraphs(result, values, months)
		missing := false
		for _, value := range values {
			missing = missing || !value.observed
		}
		if missing || len(values) == 0 {
			result.Warnings = appendEnergyDriverWarning(result.Warnings, EnergyWarning{Severity: "warning", Code: "energy_path_thermal_inputs_incomplete", Message: "Some measured thermal input context is unavailable. Missing native observations, unverified surface ownership/area, or incomplete periods are not replaced with estimates or zero; a new run may be required."})
		}
		return
	}
}

func readEnergyPathThermalInputs(path string, plan *PurposeRunPlan, context energyDriverBuildContext, geometry *idf.GeometryReport) ([]energyPathThermalInputValue, []EnergyDataSource, []int, error) {
	db, err := openSimulationSQLiteReadOnly(path)
	if err != nil {
		return nil, nil, nil, err
	}
	defer db.Close()
	axes, err := readEnergyPathThermalInputAxes(db)
	if err != nil {
		return nil, nil, nil, err
	}
	months := make([]int, 0, len(axes.monthly))
	for month := range axes.monthly {
		months = append(months, month)
	}
	sort.Ints(months)
	if len(months) == 0 {
		return nil, nil, nil, fmt.Errorf("no native Monthly weather-run periods")
	}
	roster := energyPathThermalInputSurfaceRoster(geometry)
	verifyEnergyPathThermalInputSurfaces(db, roster, context)
	selected := map[string]bool{}
	if plan != nil {
		selected, _ = purposeSelectedZoneSet(SimulationPurposeScope{ZoneMode: plan.ZoneMode, ZoneNames: plan.ZoneNames})
	}
	zoneNames := map[string]string{}
	if geometry != nil {
		for _, zone := range geometry.Zones {
			key := normalizePurposeToken(zone.Name)
			if len(selected) == 0 || selected[key] {
				zoneNames[key] = zone.Name
			}
		}
	}
	verifiedZones := verifiedEnergyPathThermalInputZones(db, zoneNames, context)
	series, err := readEnergyPathThermalInputDictionaries(db, axes, roster, zoneNames, verifiedZones)
	if err != nil {
		return nil, nil, nil, err
	}
	values, sources := collectEnergyPathThermalInputValues(series, roster, zoneNames, months)
	return values, sources, months, nil
}

func verifiedEnergyPathThermalInputZones(db *sql.DB, zones map[string]string, context energyDriverBuildContext) map[string]float64 {
	verified := map[string]float64{}
	rows, err := db.Query(`SELECT ZoneName,Multiplier,ListMultiplier FROM Zones`)
	if err != nil {
		return verified
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var name sql.NullString
		var multiplier, group sql.NullFloat64
		if rows.Scan(&name, &multiplier, &group) != nil {
			return map[string]float64{}
		}
		key := normalizePurposeToken(name.String)
		record, known := context.Multipliers.resolve(zones[key])
		factor := multiplier.Float64 * group.Float64
		if seen[key] || !known || !multiplier.Valid || !group.Valid || validEnergyFloorArea(multiplier.Float64) == 0 || validEnergyFloorArea(group.Float64) == 0 || factor != record.effectiveMultiplier() {
			delete(verified, key)
		} else {
			verified[key] = factor
		}
		seen[key] = true
	}
	if rows.Err() != nil {
		return map[string]float64{}
	}
	return verified
}

func verifyEnergyPathThermalInputSurfaces(db *sql.DB, roster map[string]energyPathThermalInputSurface, context energyDriverBuildContext) {
	rows, err := db.Query(`SELECT s.SurfaceName,s.Area,s.ExtBoundCond,s.HeatTransferSurf,z.ZoneName,z.Multiplier,z.ListMultiplier
FROM Surfaces s JOIN Zones z ON z.ZoneIndex=s.ZoneIndex`)
	if err != nil {
		return
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var name, zone sql.NullString
		var area, multiplier, group sql.NullFloat64
		var boundary, heatTransfer sql.NullInt64
		if rows.Scan(&name, &area, &boundary, &heatTransfer, &zone, &multiplier, &group) != nil {
			for key, surface := range roster {
				surface.verified = false
				roster[key] = surface
			}
			return
		}
		key := normalizePurposeToken(name.String)
		surface, exists := roster[key]
		if !exists {
			continue
		}
		record, proven := context.Multipliers.resolve(surface.zone)
		owner, ownerKnown := context.SurfaceCategories.BySurfaceKey[normalizeEnergySurfaceKey(name.String)]
		factor := multiplier.Float64 * group.Float64
		surface.verified = !seen[key] && !surface.ambiguous && surface.zone != "" && normalizePurposeToken(zone.String) == normalizePurposeToken(surface.zone) &&
			ownerKnown && normalizePurposeToken(owner.ZoneName) == normalizePurposeToken(surface.zone) && proven &&
			boundary.Valid && boundary.Int64 == 0 && heatTransfer.Valid && heatTransfer.Int64 == 1 &&
			area.Valid && validEnergyFloorArea(area.Float64) > 0 && multiplier.Valid && group.Valid &&
			validEnergyFloorArea(multiplier.Float64) > 0 && validEnergyFloorArea(group.Float64) > 0 && factor == record.effectiveMultiplier()
		surface.area, surface.multiplier = area.Float64, factor
		roster[key], seen[key] = surface, true
	}
	if rows.Err() != nil {
		for key, surface := range roster {
			surface.verified = false
			roster[key] = surface
		}
	}
}

func readEnergyPathThermalInputDictionaries(db *sql.DB, axes energyPathThermalInputAxes, roster map[string]energyPathThermalInputSurface, zoneNames map[string]string, verifiedZones map[string]float64) ([]energyPathThermalInputSeries, error) {
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,KeyValue,Name,Units,ReportingFrequency FROM ReportDataDictionary WHERE IsMeter=0 ORDER BY ReportDataDictionaryIndex`)
	if err != nil {
		return nil, err
	}
	var series []energyPathThermalInputSeries
	for rows.Next() {
		var index int
		var key, name, units, frequency string
		if err := rows.Scan(&index, &key, &name, &units, &frequency); err != nil {
			rows.Close()
			return nil, err
		}
		for _, definition := range energyPathThermalInputDefinitions {
			if !strings.EqualFold(strings.TrimSpace(name), definition.variable) {
				continue
			}
			item := energyPathThermalInputSeries{definition: definition, dictionary: index, id: fmt.Sprintf("sql-input-rdd-%d", index), key: key,
				frequency: frequency, values: map[int]float64{}, observed: map[int]bool{}}
			if definition.internal {
				item.zone = zoneNames[normalizePurposeToken(key)]
				factor, proven := verifiedZones[normalizePurposeToken(item.zone)]
				if item.zone == "" || !proven {
					continue
				}
				item.multiplier = factor
			} else {
				surface := roster[normalizePurposeToken(key)]
				if !surface.verified || zoneNames[normalizePurposeToken(surface.zone)] == "" || !definition.incident && surface.opening {
					continue
				}
				// Executed SQL Area already includes opening multiplicity and
				// removes openings from the opaque surface. Native 22.1 smoke:
				// window multiplier=2 => Area=20, GrossArea=10; wall net area
				// =49.67728, gross=69.67728. Never apply that factor again.
				item.zone, item.category, item.area, item.multiplier = surface.zone, surface.category, surface.area, surface.multiplier
			}
			if definition.incident {
				if !strings.EqualFold(units, "W/m2") && !strings.EqualFold(units, "W/m²") || !strings.EqualFold(frequency, "Daily") && !strings.EqualFold(frequency, "Hourly") {
					continue
				}
			} else if !strings.EqualFold(units, "J") || !strings.EqualFold(frequency, "Monthly") {
				continue
			}
			series = append(series, item)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := readEnergyPathThermalInputSeries(db, series, axes); err != nil {
		return nil, err
	}
	return series, nil
}

func readEnergyPathThermalInputSeries(db *sql.DB, series []energyPathThermalInputSeries, axes energyPathThermalInputAxes) error {
	if len(series) == 0 {
		return nil
	}
	expected := map[string]map[int64]energyPathThermalInputTime{"monthly": {}, "daily": {}, "hourly": {}}
	counts := map[string]map[int]int{"monthly": {}, "daily": {}, "hourly": {}}
	valid := map[string]map[int]bool{"monthly": {}, "daily": {}, "hourly": {}}
	for month, period := range axes.monthly {
		for frequency, times := range map[string][]energyPathThermalInputTime{"monthly": {period}, "daily": axes.daily[month], "hourly": axes.hourly[month]} {
			valid[frequency][month] = period.valid && len(times) > 0
			for _, t := range times {
				expected[frequency][t.id] = t
				counts[frequency][month]++
				valid[frequency][month] = valid[frequency][month] && t.valid
			}
		}
	}
	type state struct {
		item    *energyPathThermalInputSeries
		seen    map[int]int
		last    int64
		hasLast bool
	}
	byID := map[int]*state{}
	ids := make([]int, 0, len(series))
	for i := range series {
		item := &series[i]
		if _, duplicate := byID[item.dictionary]; duplicate {
			return fmt.Errorf("ambiguous thermal input dictionary index %d", item.dictionary)
		}
		frequency := strings.ToLower(item.frequency)
		for month := range axes.monthly {
			item.observed[month] = valid[frequency][month]
		}
		byID[item.dictionary] = &state{item: item, seen: map[int]int{}}
		ids = append(ids, item.dictionary)
	}
	// Native EnergyPlus ReportData has no dictionary index. Stream the entire
	// selected roster in ONE filtered scan; N per-source queries would rescan a
	// potentially multi-gigabyte annual table N times. The shared walker does
	// not create an index or modify the original simulation database.
	err := walkReportDataCompact(db, SQLSeriesQuery{DictionaryIndexes: ids}, func(row SQLSeriesRow) error {
		if !axes.weatherIDs[row.TimeIndex] {
			return nil
		}
		state := byID[row.DictionaryIndex]
		if state == nil {
			return nil
		}
		item := state.item
		frequency := strings.ToLower(item.frequency)
		t, exists := expected[frequency][row.TimeIndex]
		if !exists {
			for month := range item.observed {
				item.observed[month] = false
			}
			return nil
		}
		// A malformed BLOB/TEXT ReportData.TimeIndex can scan as an integer
		// while failing the native SQL Time join. The shared walker preserves
		// that distinction as absent joined metadata; never reinterpret it as
		// the valid weather observation having the same decoded integer.
		hour := t.end.Hour()
		if hour == 0 {
			hour = 24
		}
		if !row.Month.Valid || !row.Day.Valid || !row.Hour.Valid || !row.Minute.Valid ||
			int(row.Month.Int64) != t.month || int(row.Day.Int64) != t.end.Add(-time.Nanosecond).Day() || int(row.Hour.Int64) != hour || row.Minute.Int64 != 0 ||
			strings.TrimSpace(row.IntervalType) != map[string]string{"monthly": "3", "daily": "2", "hourly": "1"}[frequency] {
			item.observed[t.month] = false
			return nil
		}
		duplicate := state.hasLast && state.last == row.TimeIndex
		state.hasLast, state.last = true, row.TimeIndex
		value := row.Value
		if duplicate || !value.Valid || !energyPathFinite(value.Float64) {
			item.observed[t.month] = false
			return nil
		}
		converted := value.Float64 / 3.6e6
		if item.definition.incident {
			if value.Float64 < 0 {
				item.observed[t.month] = false
				return nil
			}
			converted = value.Float64 * item.area * t.hours / 1000
		}
		item.values[t.month] += converted
		state.seen[t.month]++
		return nil
	})
	if err != nil {
		return err
	}
	for _, state := range byID {
		item := state.item
		frequency := strings.ToLower(item.frequency)
		for month, observed := range item.observed {
			item.observed[month] = observed && counts[frequency][month] > 0 && counts[frequency][month] == state.seen[month] && energyPathFinite(item.values[month])
		}
	}
	return nil
}

func collectEnergyPathThermalInputValues(series []energyPathThermalInputSeries, roster map[string]energyPathThermalInputSurface, zones map[string]string, months []int) ([]energyPathThermalInputValue, []EnergyDataSource) {
	byIdentity := map[string][]energyPathThermalInputSeries{}
	for _, item := range series {
		key := item.definition.kind + "|" + normalizePurposeToken(item.key)
		byIdentity[key] = append(byIdentity[key], item)
	}
	var values []energyPathThermalInputValue
	used := map[string]energyPathThermalInputSeries{}
	for _, definition := range energyPathThermalInputDefinitions {
		groups := map[string][]string{}
		if definition.internal {
			for key, name := range zones {
				groups[name+"|"] = []string{key}
			}
		} else {
			for key, surface := range roster {
				if zones[normalizePurposeToken(surface.zone)] == "" || !definition.incident && surface.opening {
					continue
				}
				group := surface.zone + "|" + surface.category
				groups[group] = append(groups[group], key)
			}
		}
		for group, keys := range groups {
			parts := strings.SplitN(group, "|", 2)
			for _, month := range months {
				value := energyPathThermalInputValue{kind: definition.kind, label: definition.label, zone: parts[0], category: parts[1], month: month}
				complete := len(keys) > 0
				for _, key := range keys {
					candidates := byIdentity[definition.kind+"|"+key]
					if definition.incident {
						var daily []energyPathThermalInputSeries
						for _, candidate := range candidates {
							if strings.EqualFold(candidate.frequency, "Daily") {
								daily = append(daily, candidate)
							}
						}
						if len(daily) > 0 {
							candidates = daily
						}
					}
					if len(candidates) != 1 || !candidates[0].observed[month] {
						complete = false
						break
					}
					item := candidates[0]
					value.raw += item.values[month]
					value.effective += item.values[month] * item.multiplier
					value.sourceIDs = append(value.sourceIDs, item.id)
					used[item.id] = item
				}
				value.observed = complete
				if complete {
					sort.Strings(value.sourceIDs)
				} else {
					value.raw, value.effective, value.sourceIDs = 0, 0, nil
				}
				values = append(values, value)
			}
		}
	}
	var sources []EnergyDataSource
	for _, item := range used {
		var total float64
		for month, value := range item.values {
			if item.observed[month] {
				total += value
			}
		}
		formula := "native Monthly energy [J] / 3600000; effective = raw × Zone multiplier × ZoneGroup multiplier"
		if item.definition.incident {
			formula = "sum(native full-interval solar [W/m2] × executed SQL Surfaces.Area [m2] × observed hours / 1000); effective = raw × Zone multiplier × ZoneGroup multiplier"
		}
		sources = append(sources, EnergyDataSource{ID: item.id, SourceType: "sql_report_data", KeyValue: item.key, Name: item.definition.variable,
			Units: "kWh", SourceUnit: map[bool]string{true: "W/m2", false: "J"}[item.definition.incident], NormalizedUnit: "kWh", ReportingFrequency: item.frequency,
			ZoneName: item.zone, RawValue: total, EffectiveValue: total * item.multiplier, EffectiveMultiplier: item.multiplier,
			MultiplierApplication: "zone_and_group_once", AggregationMethod: "sum_observed_native_intervals", AggregationBasis: "reported_boundary",
			DriverRole: "context", DriverCategory: item.category, InspectorSection: "thermal_inputs", Formula: formula,
			Explanation: energyPathThermalInputExplanation(item.definition.kind)})
		sources[len(sources)-1].observedValuePresence = energySourceObservedRaw | energySourceObservedEffective
	}
	sort.Slice(values, func(i, j int) bool {
		a, b := values[i], values[j]
		return fmt.Sprintf("%s|%s|%s|%02d", a.zone, a.kind, a.category, a.month) < fmt.Sprintf("%s|%s|%s|%02d", b.zone, b.kind, b.category, b.month)
	})
	sort.Slice(sources, func(i, j int) bool { return sources[i].ID < sources[j].ID })
	return values, sources
}

func energyPathThermalInputExplanation(kind string) string {
	switch kind {
	case "input.surface_storage":
		return "Positive reported storage enters envelope mass; negative storage leaves it. Internal envelope storage context is not an independent exterior input or an insulation saving."
	case "input.solar_incident":
		return "Solar arriving at exterior surfaces, integrated from observed native Daily/Hourly density and executed net surface area. Incident and absorbed solar are distinct, overlapping context quantities and must not be added."
	case "input.solar_absorbed":
		return "Reported exterior absorbed solar, distinct from incident solar. This context is not a conserved allocation into the room or an insulation saving."
	case "input.internal_gains":
		return "Native total internal heat gain, before convective/radiant/latent paths and time shifts. Association with allocated internal drivers is contextual, not a causal allocation."
	default:
		return "Native signed exterior surface exchange; positive heat enters the outside face. Boundary context and allocated room heat drivers have different physical boundaries and need not balance."
	}
}

func appendEnergyPathThermalInputGraphs(result *EnergyExplanationResult, values []energyPathThermalInputValue, months []int) {
	appendGraph := func(nodes *[]EnergyExplanationNode, links *[]EnergyPathLink, zone, period string) {
		selected := energyPathThermalInputGraphValues(values, months, zone, period)
		for _, value := range selected {
			// Period lives in metadata, not identity: an Annual selection must
			// find the same physical input in every native Monthly graph.
			id := "input:" + value.kind + ":" + value.category + ":" + normalizePurposeToken(zone)
			node := EnergyExplanationNode{ID: id, Level: "input", Kind: value.kind, Label: value.label, Value: math.Abs(value.effective),
				SignedValue: value.effective, RawValue: value.raw, EffectiveValue: value.effective, Unit: "kWh", ScaleDomain: "boundary", Period: period,
				ZoneName: zone, DriverCategory: value.category, Basis: "reported_boundary", AggregationBasis: "sum_complete_zone_month_boundary_inputs", SourceIDs: value.sourceIDs,
				AllocationExplanation: energyPathThermalInputExplanation(value.kind)}
			var associated []EnergyPathLink
			for _, driver := range *nodes {
				if driver.Level != "driver" || !energyPathThermalInputMatchesDriver(value, driver) {
					continue
				}
				associated = append(associated, EnergyPathLink{ID: "input-context:" + id + ":" + driver.ID, FromID: id, ToID: driver.ID,
					Relation: "input_to_driver", Basis: "boundary_context", Explanation: energyPathThermalInputExplanation(value.kind), FromValue: value.effective,
					FromUnit: "kWh", ToValue: driver.Value, ToUnit: driver.Unit, Period: period, ZoneName: zone,
					SourceIDs: appendUniqueStrings(append([]string(nil), value.sourceIDs...), driver.SourceIDs...)})
			}
			*nodes = append(*nodes, node)
			*links = append(*links, associated...)
		}
	}
	zone := ""
	if result.Scope.Kind == "zone" {
		zone = result.Scope.ZoneName
	}
	appendGraph(&result.Nodes, &result.Links, zone, "Annual")
	for i := range result.Periods {
		period := &result.Periods[i]
		appendGraph(&period.Nodes, &period.Links, zone, period.ID)
	}
	for i := range result.ZoneResults {
		zoneResult := &result.ZoneResults[i]
		appendGraph(&zoneResult.Nodes, &zoneResult.Links, zoneResult.Scope.ZoneName, "Annual")
		for j := range zoneResult.Periods {
			period := &zoneResult.Periods[j]
			appendGraph(&period.Nodes, &period.Links, zoneResult.Scope.ZoneName, period.ID)
		}
	}
}

func energyPathThermalInputMatchesDriver(value energyPathThermalInputValue, driver EnergyExplanationNode) bool {
	category := canonicalEnergyDriverCategory(firstNonEmpty(driver.DriverCategory, driver.Kind))
	if value.kind == "input.internal_gains" {
		return strings.HasPrefix(category, "internal.")
	}
	return category == value.category
}

// Stored boundary context cannot acquire conversion authority, source evidence,
// a different period, or another Zone merely by naming existing endpoints.
func filterEnergyPathThermalInputLinks(nodes []EnergyExplanationNode, links []EnergyPathLink, sources []EnergyDataSource, scope EnergyExplanationScope, period string) []EnergyPathLink {
	byNode, nodeCounts := map[string]EnergyExplanationNode{}, map[string]int{}
	for _, node := range nodes {
		byNode[node.ID] = node
		nodeCounts[node.ID]++
	}
	bySource, sourceCounts := map[string]EnergyDataSource{}, map[string]int{}
	for _, source := range sources {
		bySource[source.ID] = source
		sourceCounts[source.ID]++
	}
	linkCounts := map[string]int{}
	for _, link := range links {
		if link.Relation == "input_to_driver" {
			linkCounts[link.ID]++
		}
	}
	out := make([]EnergyPathLink, 0, len(links))
	for _, link := range links {
		if link.Relation != "input_to_driver" {
			out = append(out, link)
			continue
		}
		input, driver := byNode[link.FromID], byNode[link.ToID]
		if link.ID == "" || linkCounts[link.ID] != 1 || nodeCounts[input.ID] != 1 || nodeCounts[driver.ID] != 1 ||
			input.Level != "input" || input.ScaleDomain != "boundary" || input.Basis != "reported_boundary" ||
			driver.Level != "driver" || driver.ScaleDomain != "thermal" || link.Basis != "boundary_context" ||
			link.Ratio != 0 || link.RatioKind != "" || link.RatioLabel != "" || link.RuleID != "" ||
			!energyPathFinite(input.SignedValue) || !energyPathFinite(input.Value) || input.Value != math.Abs(input.SignedValue) ||
			!energyPathFinite(link.FromValue) || !energyPathFinite(link.ToValue) || link.ToValue < 0 ||
			link.FromValue != input.SignedValue || link.ToValue != driver.Value ||
			input.Unit != "kWh" || driver.Unit != "kWh" || link.FromUnit != input.Unit || link.ToUnit != driver.Unit ||
			!strings.EqualFold(link.Period, period) || !strings.EqualFold(input.Period, period) || !strings.EqualFold(driver.Period, period) ||
			!strings.EqualFold(input.ZoneName, link.ZoneName) || !strings.EqualFold(driver.ZoneName, link.ZoneName) ||
			scope.Kind == "zone" && !strings.EqualFold(scope.ZoneName, link.ZoneName) || scope.Kind != "zone" && link.ZoneName != "" ||
			input.inspectorDecodedFromJSON && input.inspectorValuePresence&8 == 0 ||
			!energyPathThermalInputMatchesDriver(energyPathThermalInputValue{kind: input.Kind, category: input.DriverCategory}, driver) ||
			len(input.SourceIDs) == 0 || len(driver.SourceIDs) == 0 {
			continue
		}
		variable := ""
		for _, definition := range energyPathThermalInputDefinitions {
			if input.Kind == definition.kind {
				variable = definition.variable
			}
		}
		valid := variable != ""
		union := map[string]bool{}
		for _, id := range input.SourceIDs {
			source := bySource[id]
			valid = valid && sourceCounts[id] == 1 && strings.HasPrefix(id, "sql-input-rdd-") && source.DriverRole == "context" &&
				source.AggregationBasis == "reported_boundary" && strings.EqualFold(source.Name, variable) && source.NormalizedUnit == "kWh" &&
				energyDataSourceValueKnown(source, energySourceObservedRaw) && energyDataSourceValueKnown(source, energySourceObservedEffective) &&
				energyPathFinite(source.RawValue) && energyPathFinite(source.EffectiveValue) && energyPathPositiveFinite(source.EffectiveMultiplier) &&
				(scope.Kind != "zone" || strings.EqualFold(source.ZoneName, scope.ZoneName))
			union[id] = true
		}
		for _, id := range driver.SourceIDs {
			valid = valid && sourceCounts[id] == 1
			union[id] = true
		}
		seen := map[string]bool{}
		for _, id := range link.SourceIDs {
			valid = valid && union[id] && !seen[id]
			seen[id] = true
		}
		valid = valid && len(seen) == len(union)
		if valid {
			out = append(out, link)
		}
	}
	return out
}

func energyPathThermalInputGraphValues(values []energyPathThermalInputValue, months []int, zone, period string) []energyPathThermalInputValue {
	month := 0
	if !strings.EqualFold(period, "Annual") {
		if _, err := fmt.Sscanf(period, "M%d", &month); err != nil || month < 1 || month > 12 {
			return nil
		}
	}
	// Missing Zone-month inputs poison their matching scope/category, rather
	// than silently becoming zero in Building or Annual aggregation.
	groups := map[string][]energyPathThermalInputValue{}
	for _, value := range values {
		if zone != "" && !strings.EqualFold(zone, value.zone) || month != 0 && month != value.month {
			continue
		}
		key := value.kind + "|" + value.category
		groups[key] = append(groups[key], value)
	}
	completed := []energyPathThermalInputValue{}
	for _, group := range groups {
		complete := true
		zoneMonths := map[string]map[int]bool{}
		for _, item := range group {
			complete = complete && item.observed
			key := normalizePurposeToken(item.zone)
			if zoneMonths[key] == nil {
				zoneMonths[key] = map[int]bool{}
			}
			if zoneMonths[key][item.month] {
				complete = false
			}
			zoneMonths[key][item.month] = true
		}
		for _, seen := range zoneMonths {
			if month == 0 && len(seen) != len(months) {
				complete = false
			}
		}
		if !complete {
			continue
		}
		value := group[0]
		value.raw, value.effective, value.sourceIDs = 0, 0, nil
		for _, item := range group {
			value.raw += item.raw
			value.effective += item.effective
			value.sourceIDs = appendUniqueStrings(value.sourceIDs, item.sourceIDs...)
		}
		completed = append(completed, value)
	}
	merged := map[string]energyPathThermalInputValue{}
	for _, value := range completed {
		key := value.kind + "|" + value.category
		current, exists := merged[key]
		if !exists {
			current = value
			current.raw, current.effective, current.sourceIDs = 0, 0, nil
		}
		current.raw += value.raw
		current.effective += value.effective
		current.sourceIDs = appendUniqueStrings(current.sourceIDs, value.sourceIDs...)
		merged[key] = current
	}
	result := make([]energyPathThermalInputValue, 0, len(merged))
	for _, value := range merged {
		sort.Strings(value.sourceIDs)
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].kind+result[i].category < result[j].kind+result[j].category })
	return result
}
