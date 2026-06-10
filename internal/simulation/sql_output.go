package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type sqlOutputDictionaryRow struct {
	index    int
	keyValue string
	name     string
	units    string
}

type sqlEnergyDictionaryRow struct {
	sqlOutputDictionaryRow
	category    string
	displayName string
	zoneName    string
}

type energySeriesBuilder struct {
	dictionary sqlEnergyDictionaryRow
	unit       string
	points     []SimulationPoint
	total      float64
}

func parseSimulationSQLSeries(path string) ([]SimulationSeries, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	dictionaries, err := sqlOutputSeriesDictionaries(db)
	if err != nil {
		return nil, err
	}
	if len(dictionaries) == 0 {
		return nil, nil
	}

	ids := make([]int, 0, len(dictionaries))
	accumulators := map[int]*columnAccumulator{}
	for position, dictionary := range dictionaries {
		ids = append(ids, dictionary.index)
		accumulators[dictionary.index] = &columnAccumulator{
			index: position + 1,
			name:  sqlOutputSeriesName(dictionary),
			min:   math.Inf(1),
			max:   math.Inf(-1),
		}
	}

	query, args := sqlReportDataQuery(ids)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seriesPoints := map[int][]SimulationPoint{}
	timeOrdinal := map[int64]int{}
	timeLabels := map[int64]string{}
	rowCount := 0
	for rows.Next() {
		var timeIndex int64
		var month, day, hour, minute sql.NullInt64
		var dictionaryIndex int
		var value sql.NullFloat64
		if err := rows.Scan(&timeIndex, &month, &day, &hour, &minute, &dictionaryIndex, &value); err != nil {
			continue
		}
		if !value.Valid || math.IsNaN(value.Float64) || math.IsInf(value.Float64, 0) {
			continue
		}
		ordinal := timeOrdinal[timeIndex]
		if ordinal == 0 {
			rowCount++
			ordinal = rowCount
			timeOrdinal[timeIndex] = ordinal
			timeLabels[timeIndex] = sqlFrameLabel(month, day, hour, minute)
		}
		acc := accumulators[dictionaryIndex]
		if acc == nil {
			continue
		}
		number := value.Float64
		acc.numericCount++
		acc.sum += number
		acc.last = number
		acc.min = math.Min(acc.min, number)
		acc.max = math.Max(acc.max, number)
		seriesPoints[dictionaryIndex] = append(seriesPoints[dictionaryIndex], SimulationPoint{
			X:     ordinal,
			Label: timeLabels[timeIndex],
			Value: number,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	series := []SimulationSeries{}
	for _, dictionary := range dictionaries {
		acc := accumulators[dictionary.index]
		if acc == nil || acc.numericCount == 0 {
			continue
		}
		average := acc.sum / float64(acc.numericCount)
		series = append(series, SimulationSeries{
			File:     filepath.Base(path),
			Column:   acc.name,
			Min:      acc.min,
			Max:      acc.max,
			Average:  average,
			Points:   downsamplePoints(seriesPoints[dictionary.index], maxCSVSeriesPoints),
			RowCount: rowCount,
		})
	}
	return series, nil
}

func parseSimulationEnergySQL(path string) (EnergyDashboardResult, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return EnergyDashboardResult{}, err
	}
	defer db.Close()

	dictionaries, err := sqlOutputEnergyDictionaries(db)
	if err != nil {
		return EnergyDashboardResult{}, err
	}
	if len(dictionaries) == 0 {
		return EnergyDashboardResult{}, nil
	}
	ids := make([]int, 0, len(dictionaries))
	byID := map[int]sqlEnergyDictionaryRow{}
	for _, dictionary := range dictionaries {
		ids = append(ids, dictionary.index)
		byID[dictionary.index] = dictionary
	}

	query, args := sqlReportDataQuery(ids)
	rows, err := db.Query(query, args...)
	if err != nil {
		return EnergyDashboardResult{}, err
	}
	defer rows.Close()

	builders := map[int]*energySeriesBuilder{}
	timeOrdinal := map[int64]int{}
	timeLabels := map[int64]string{}
	rowCount := 0
	for rows.Next() {
		var timeIndex int64
		var month, day, hour, minute sql.NullInt64
		var dictionaryIndex int
		var value sql.NullFloat64
		if err := rows.Scan(&timeIndex, &month, &day, &hour, &minute, &dictionaryIndex, &value); err != nil {
			continue
		}
		if !value.Valid || math.IsNaN(value.Float64) || math.IsInf(value.Float64, 0) {
			continue
		}
		dictionary, ok := byID[dictionaryIndex]
		if !ok {
			continue
		}
		ordinal := timeOrdinal[timeIndex]
		if ordinal == 0 {
			rowCount++
			ordinal = rowCount
			timeOrdinal[timeIndex] = ordinal
			timeLabels[timeIndex] = sqlFrameLabel(month, day, hour, minute)
		}
		builder := builders[dictionaryIndex]
		if builder == nil {
			builder = &energySeriesBuilder{dictionary: dictionary}
			builders[dictionaryIndex] = builder
		}
		number, unit := convertEnergySQLValue(value.Float64, dictionary.units)
		builder.unit = unit
		builder.total += number
		builder.points = append(builder.points, SimulationPoint{X: ordinal, Label: timeLabels[timeIndex], Value: roundedEnergyNumber(number)})
	}
	if err := rows.Err(); err != nil {
		return EnergyDashboardResult{}, err
	}

	result := EnergyDashboardResult{}
	for _, dictionary := range dictionaries {
		builder := builders[dictionary.index]
		if builder == nil || len(builder.points) == 0 {
			continue
		}
		series := EnergySeries{
			Name:   dictionary.displayName,
			Unit:   builder.unit,
			Source: filepath.Base(path),
			Points: builder.points,
			Total:  roundedEnergyNumber(builder.total),
		}
		switch dictionary.category {
		case "facility":
			result.FacilityMonthly = append(result.FacilityMonthly, series)
		case "end_use":
			result.EndUseMonthly = append(result.EndUseMonthly, series)
		case "zone":
			result.ZoneMonthly = append(result.ZoneMonthly, ZoneEnergySeries{
				ZoneName: dictionary.zoneName,
				Metric:   dictionary.displayName,
				Unit:     series.Unit,
				Source:   series.Source,
				Points:   series.Points,
				Total:    series.Total,
			})
		}
		result.Totals = append(result.Totals, EnergyTotal{
			Name:   series.Name,
			Unit:   series.Unit,
			Source: series.Source,
			Value:  series.Total,
		})
	}
	sortEnergyDashboardResult(&result)
	return result, nil
}

func parseSimulationHeatFlowSQL(path string) (HeatFlowDataset, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return HeatFlowDataset{}, err
	}
	defer db.Close()

	dictionaries, err := sqlOutputHeatFlowDictionaries(db)
	if err != nil {
		return HeatFlowDataset{}, err
	}
	if len(dictionaries) == 0 {
		return HeatFlowDataset{}, nil
	}

	definitions := heatFlowCategoryDefinitions()
	columns := map[int]heatFlowColumn{}
	ids := make([]int, 0, len(dictionaries))
	for _, dictionary := range dictionaries {
		zoneName := strings.TrimSpace(dictionary.keyValue)
		if zoneName == "" {
			continue
		}
		if heatFlowVariableMatches(dictionary.name, heatFlowTemperatureVariables()) {
			columns[dictionary.index] = heatFlowColumn{zoneName: zoneName, temperature: true}
			ids = append(ids, dictionary.index)
			continue
		}
		for defIndex, definition := range definitions {
			names := append([]string{definition.variable}, definition.aliases...)
			if heatFlowVariableMatches(dictionary.name, names) {
				definition.available = true
				definitions[defIndex] = definition
				columns[dictionary.index] = heatFlowColumn{zoneName: zoneName, categoryIndex: defIndex}
				ids = append(ids, dictionary.index)
				break
			}
		}
	}

	categories, categoryIndexMap := heatFlowCategoriesFromDefinitions(definitions)
	for dictionaryIndex, column := range columns {
		if column.temperature {
			continue
		}
		categoryIndex, ok := categoryIndexMap[column.categoryIndex]
		if !ok {
			delete(columns, dictionaryIndex)
			continue
		}
		column.categoryIndex = categoryIndex
		columns[dictionaryIndex] = column
	}
	if len(categories) == 0 || len(columns) == 0 {
		return HeatFlowDataset{}, nil
	}

	rowCount, err := sqlDistinctTimeCount(db, ids)
	if err != nil {
		return HeatFlowDataset{}, err
	}
	if rowCount == 0 {
		return HeatFlowDataset{}, nil
	}
	stride := 1
	if rowCount > maxHeatFlowFrames {
		stride = int(math.Ceil(float64(rowCount) / float64(maxHeatFlowFrames)))
	}

	query, args := sqlReportDataQuery(ids)
	rows, err := db.Query(query, args...)
	if err != nil {
		return HeatFlowDataset{}, err
	}
	defer rows.Close()

	dataset := HeatFlowDataset{
		SourceFile:         filepath.Base(path),
		Unit:               "W",
		TemperatureUnit:    "C",
		OriginalFrameCount: rowCount,
		Categories:         categories,
		MinTemperature:     math.Inf(1),
		MaxTemperature:     math.Inf(-1),
	}
	zoneBuilders := map[string]*heatFlowZoneBuilder{}
	zoneOrder := []string{}
	timeFrame := map[int64]int{}
	keptFrame := map[int64]int{}
	frameIndex := -1

	for rows.Next() {
		var timeIndex int64
		var month, day, hour, minute sql.NullInt64
		var dictionaryIndex int
		var value sql.NullFloat64
		if err := rows.Scan(&timeIndex, &month, &day, &hour, &minute, &dictionaryIndex, &value); err != nil {
			continue
		}
		if !value.Valid || math.IsNaN(value.Float64) || math.IsInf(value.Float64, 0) {
			continue
		}
		currentFrame, known := timeFrame[timeIndex]
		if !known {
			frameIndex++
			currentFrame = frameIndex
			timeFrame[timeIndex] = currentFrame
			if stride <= 1 || currentFrame%stride == 0 || currentFrame == rowCount-1 {
				keptFrame[timeIndex] = dataset.FrameCount
				dataset.Labels = append(dataset.Labels, sqlFrameLabel(month, day, hour, minute))
				dataset.FrameCount++
			}
		}
		keptFrameIndex, keep := keptFrame[timeIndex]
		if !keep {
			continue
		}
		column, ok := columns[dictionaryIndex]
		if !ok {
			continue
		}
		key := normalizeHeatFlowName(column.zoneName)
		builder := zoneBuilders[key]
		if builder == nil {
			builder = &heatFlowZoneBuilder{
				name:   strings.TrimSpace(column.zoneName),
				values: make([][]float64, len(categories)),
			}
			zoneBuilders[key] = builder
			zoneOrder = append(zoneOrder, key)
		}
		builder.ensureFrame(keptFrameIndex, len(categories))
		number := value.Float64
		if column.temperature {
			builder.temperature[keptFrameIndex] = roundedHeatFlowNumber(number)
			builder.hasTemperature = true
			dataset.MinTemperature = math.Min(dataset.MinTemperature, number)
			dataset.MaxTemperature = math.Max(dataset.MaxTemperature, number)
			continue
		}
		builder.values[column.categoryIndex][keptFrameIndex] = roundedHeatFlowNumber(number)
		builder.hasHeatFlowData = true
		dataset.MaxAbs = math.Max(dataset.MaxAbs, math.Abs(number))
	}
	if err := rows.Err(); err != nil {
		return HeatFlowDataset{}, err
	}
	if rowCount > dataset.FrameCount {
		dataset.Warnings = append(dataset.Warnings, "Heat-flow frames were sampled for interactive rendering.")
	}
	return finalizeHeatFlowDataset(dataset, zoneBuilders, zoneOrder, len(categories))
}

func sqlOutputSeriesDictionaries(db *sql.DB) ([]sqlOutputDictionaryRow, error) {
	rows, err := db.Query(`
SELECT DISTINCT rdd.ReportDataDictionaryIndex, COALESCE(rdd.KeyValue, ''), COALESCE(rdd.Name, ''), COALESCE(rdd.Units, '')
FROM ReportDataDictionary rdd
JOIN ReportData rd ON rd.ReportDataDictionaryIndex = rdd.ReportDataDictionaryIndex
WHERE TRIM(COALESCE(rdd.Name, '')) <> ''
ORDER BY rdd.ReportDataDictionaryIndex
LIMIT ?`, maxCSVSeriesColumns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLDictionaryRows(rows)
}

func sqlOutputEnergyDictionaries(db *sql.DB) ([]sqlEnergyDictionaryRow, error) {
	rows, err := db.Query(`
SELECT DISTINCT rdd.ReportDataDictionaryIndex, COALESCE(rdd.KeyValue, ''), COALESCE(rdd.Name, ''), COALESCE(rdd.Units, '')
FROM ReportDataDictionary rdd
JOIN ReportData rd ON rd.ReportDataDictionaryIndex = rdd.ReportDataDictionaryIndex
WHERE TRIM(COALESCE(rdd.Name, '')) <> ''
ORDER BY rdd.ReportDataDictionaryIndex`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dictionaries, err := scanSQLDictionaryRows(rows)
	if err != nil {
		return nil, err
	}
	out := make([]sqlEnergyDictionaryRow, 0, len(dictionaries))
	for _, dictionary := range dictionaries {
		if row, ok := classifySQLEnergyDictionary(dictionary); ok {
			out = append(out, row)
		}
	}
	return out, nil
}

func sqlOutputHeatFlowDictionaries(db *sql.DB) ([]sqlOutputDictionaryRow, error) {
	rows, err := db.Query(`
SELECT ReportDataDictionaryIndex, COALESCE(KeyValue, ''), COALESCE(Name, ''), COALESCE(Units, '')
FROM ReportDataDictionary
WHERE TRIM(COALESCE(Name, '')) <> ''
ORDER BY ReportDataDictionaryIndex`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dictionaries, err := scanSQLDictionaryRows(rows)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, name := range heatFlowSQLVariableNames() {
		names[name] = true
	}
	out := make([]sqlOutputDictionaryRow, 0, len(dictionaries))
	for _, dictionary := range dictionaries {
		if names[normalizeHeatFlowName(dictionary.name)] {
			out = append(out, dictionary)
		}
	}
	return out, nil
}

func classifySQLEnergyDictionary(row sqlOutputDictionaryRow) (sqlEnergyDictionaryRow, bool) {
	name := strings.TrimSpace(row.name)
	keyValue := strings.TrimSpace(row.keyValue)
	normalizedName := normalizeEnergyOutputName(name)
	normalizedKey := normalizeEnergyOutputName(keyValue)
	metricName := name
	if metricName == "" {
		metricName = keyValue
	}
	energy := sqlEnergyDictionaryRow{
		sqlOutputDictionaryRow: row,
		displayName:            metricName,
		zoneName:               keyValue,
	}
	if energyFacilityMeters()[normalizedName] || energyFacilityMeters()[normalizedKey] {
		energy.category = "facility"
		if energyFacilityMeters()[normalizedKey] {
			energy.displayName = keyValue
		}
		return energy, true
	}
	if energyEndUseMeters()[normalizedName] || energyEndUseMeters()[normalizedKey] {
		energy.category = "end_use"
		if energyEndUseMeters()[normalizedKey] {
			energy.displayName = keyValue
		}
		return energy, true
	}
	if energyZoneVariables()[normalizedName] {
		energy.category = "zone"
		energy.displayName = name
		if energy.zoneName == "" {
			energy.zoneName = "Unknown Zone"
		}
		return energy, true
	}
	return sqlEnergyDictionaryRow{}, false
}

func energyFacilityMeters() map[string]bool {
	return normalizedEnergySet(
		"Electricity:Facility",
		"NaturalGas:Facility",
		"DistrictCooling:Facility",
		"DistrictHeating:Facility",
		"Water:Facility",
	)
}

func energyEndUseMeters() map[string]bool {
	return normalizedEnergySet(
		"Electricity:Cooling",
		"Electricity:Heating",
		"Electricity:InteriorLights",
		"Electricity:InteriorEquipment",
		"Electricity:Fans",
		"Electricity:Pumps",
		"Electricity:HeatRejection",
		"Electricity:WaterSystems",
		"NaturalGas:Heating",
		"NaturalGas:WaterSystems",
	)
}

func energyZoneVariables() map[string]bool {
	return normalizedEnergySet(
		"Zone Lights Electricity Energy",
		"Zone Electric Equipment Electricity Energy",
		"Zone Gas Equipment Gas Energy",
		"Zone Air System Sensible Heating Energy",
		"Zone Air System Sensible Cooling Energy",
	)
}

func normalizedEnergySet(values ...string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[normalizeEnergyOutputName(value)] = true
	}
	return out
}

func normalizeEnergyOutputName(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func convertEnergySQLValue(value float64, unit string) (float64, string) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "j":
		return value / 3600000, "kWh"
	case "kj":
		return value / 3600, "kWh"
	case "mj":
		return value / 3.6, "kWh"
	case "gj":
		return value * 277.7777777778, "kWh"
	case "wh":
		return value / 1000, "kWh"
	case "kwh":
		return value, "kWh"
	case "w":
		return value / 1000, "kW"
	default:
		return value, strings.TrimSpace(unit)
	}
}

func roundedEnergyNumber(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func sortEnergyDashboardResult(result *EnergyDashboardResult) {
	sort.SliceStable(result.FacilityMonthly, func(i, j int) bool {
		return strings.ToLower(result.FacilityMonthly[i].Name) < strings.ToLower(result.FacilityMonthly[j].Name)
	})
	sort.SliceStable(result.EndUseMonthly, func(i, j int) bool {
		return strings.ToLower(result.EndUseMonthly[i].Name) < strings.ToLower(result.EndUseMonthly[j].Name)
	})
	sort.SliceStable(result.ZoneMonthly, func(i, j int) bool {
		if result.ZoneMonthly[i].ZoneName != result.ZoneMonthly[j].ZoneName {
			return strings.ToLower(result.ZoneMonthly[i].ZoneName) < strings.ToLower(result.ZoneMonthly[j].ZoneName)
		}
		return strings.ToLower(result.ZoneMonthly[i].Metric) < strings.ToLower(result.ZoneMonthly[j].Metric)
	})
	sort.SliceStable(result.Totals, func(i, j int) bool {
		return strings.ToLower(result.Totals[i].Name) < strings.ToLower(result.Totals[j].Name)
	})
}

func scanSQLDictionaryRows(rows *sql.Rows) ([]sqlOutputDictionaryRow, error) {
	var dictionaries []sqlOutputDictionaryRow
	for rows.Next() {
		var row sqlOutputDictionaryRow
		if err := rows.Scan(&row.index, &row.keyValue, &row.name, &row.units); err != nil {
			continue
		}
		if row.index <= 0 || strings.TrimSpace(row.name) == "" {
			continue
		}
		dictionaries = append(dictionaries, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return dictionaries, nil
}

func heatFlowSQLVariableNames() []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(value string) {
		key := normalizeHeatFlowName(value)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, key)
	}
	for _, value := range heatFlowTemperatureVariables() {
		add(value)
	}
	for _, definition := range heatFlowCategoryDefinitions() {
		add(definition.variable)
		for _, alias := range definition.aliases {
			add(alias)
		}
	}
	sort.Strings(out)
	return out
}

func sqlReportDataQuery(dictionaryIDs []int) (string, []any) {
	placeholders := sqlPlaceholders(len(dictionaryIDs))
	args := make([]any, 0, len(dictionaryIDs))
	for _, id := range dictionaryIDs {
		args = append(args, id)
	}
	return fmt.Sprintf(`
SELECT rd.TimeIndex,
       t.Month,
       t.Day,
       t.Hour,
       t.Minute,
       rd.ReportDataDictionaryIndex,
       rd.Value
FROM ReportData rd
LEFT JOIN "Time" t ON t.TimeIndex = rd.TimeIndex
WHERE rd.ReportDataDictionaryIndex IN (%s)
ORDER BY rd.TimeIndex, rd.ReportDataDictionaryIndex`, placeholders), args
}

func sqlDistinctTimeCount(db *sql.DB, dictionaryIDs []int) (int, error) {
	if len(dictionaryIDs) == 0 {
		return 0, nil
	}
	placeholders := sqlPlaceholders(len(dictionaryIDs))
	args := make([]any, 0, len(dictionaryIDs))
	for _, id := range dictionaryIDs {
		args = append(args, id)
	}
	query := fmt.Sprintf(`SELECT COUNT(DISTINCT TimeIndex) FROM ReportData WHERE ReportDataDictionaryIndex IN (%s)`, placeholders)
	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func sqlPlaceholders(count int) string {
	if count <= 0 {
		return "NULL"
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func sqlOutputSeriesName(row sqlOutputDictionaryRow) string {
	name := strings.TrimSpace(row.name)
	if key := strings.TrimSpace(row.keyValue); key != "" {
		name = key + ":" + name
	}
	if units := strings.TrimSpace(row.units); units != "" {
		name += " [" + units + "]"
	}
	return name
}

func sqlFrameLabel(month sql.NullInt64, day sql.NullInt64, hour sql.NullInt64, minute sql.NullInt64) string {
	if !month.Valid || !day.Valid || !hour.Valid {
		return ""
	}
	minuteValue := int64(0)
	if minute.Valid {
		minuteValue = minute.Int64
	}
	if minuteValue >= 60 {
		minuteValue = 0
	}
	return fmt.Sprintf("%02d-%02d %02d:%02d", month.Int64, day.Int64, hour.Int64, minuteValue)
}
