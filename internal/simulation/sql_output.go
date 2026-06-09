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
