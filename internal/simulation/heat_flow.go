package simulation

import (
	"bufio"
	"encoding/csv"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const maxHeatFlowFrames = 720

type HeatFlowDataset struct {
	SourceFile         string                    `json:"sourceFile,omitempty"`
	Unit               string                    `json:"unit,omitempty"`
	TemperatureUnit    string                    `json:"temperatureUnit,omitempty"`
	FrameCount         int                       `json:"frameCount"`
	OriginalFrameCount int                       `json:"originalFrameCount"`
	Labels             []string                  `json:"labels,omitempty"`
	Categories         []HeatFlowCategory        `json:"categories,omitempty"`
	Zones              []HeatFlowZoneSeries      `json:"zones,omitempty"`
	Completeness       []PurposeCompletenessItem `json:"completeness,omitempty"`
	MaxAbs             float64                   `json:"maxAbs,omitempty"`
	MinTemperature     float64                   `json:"minTemperature,omitempty"`
	MaxTemperature     float64                   `json:"maxTemperature,omitempty"`
	Warnings           []string                  `json:"warnings,omitempty"`
}

type HeatFlowCategory struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	VariableName string `json:"variableName"`
	Unit         string `json:"unit,omitempty"`
	Color        string `json:"color,omitempty"`
}

type HeatFlowZoneSeries struct {
	Name        string      `json:"name"`
	Values      [][]float64 `json:"values"`
	Temperature []float64   `json:"temperature,omitempty"`
}

type heatFlowCategoryDefinition struct {
	id        string
	label     string
	variable  string
	color     string
	aliases   []string
	available bool
}

type heatFlowColumn struct {
	index         int
	zoneName      string
	categoryIndex int
	temperature   bool
}

type heatFlowZoneBuilder struct {
	name            string
	values          [][]float64
	temperature     []float64
	hasTemperature  bool
	hasHeatFlowData bool
}

func heatFlowCategoryDefinitions() []heatFlowCategoryDefinition {
	return []heatFlowCategoryDefinition{
		{id: "internalConvective", label: "Internal convective gains", variable: "Zone Air Heat Balance Internal Convective Heat Gain Rate", color: "#f59e0b"},
		{id: "surfaceConvection", label: "Surface convection", variable: "Zone Air Heat Balance Surface Convection Rate", color: "#ef4444"},
		{id: "interzoneAir", label: "Interzone air transfer", variable: "Zone Air Heat Balance Interzone Air Transfer Rate", color: "#a855f7"},
		{id: "outdoorAir", label: "Outdoor air transfer", variable: "Zone Air Heat Balance Outdoor Air Transfer Rate", color: "#14b8a6"},
		{id: "systemAir", label: "HVAC system air transfer", variable: "Zone Air Heat Balance System Air Transfer Rate", color: "#3b82f6"},
		{id: "systemConvective", label: "HVAC/system convective gains", variable: "Zone Air Heat Balance System Convective Heat Gain Rate", color: "#64748b"},
		{id: "airStorage", label: "Air energy storage", variable: "Zone Air Heat Balance Air Energy Storage Rate", color: "#e5e7eb"},
		{id: "deviation", label: "Heat balance deviation", variable: "Zone Air Heat Balance Deviation Rate", color: "#94a3b8"},
	}
}

func parseSimulationHeatFlowCSV(path string) (HeatFlowDataset, error) {
	header, rowCount, err := heatFlowCSVHeaderAndRowCount(path)
	if err != nil {
		return HeatFlowDataset{}, err
	}
	categories, columns := heatFlowColumnsFromHeader(header)
	if len(categories) == 0 || len(columns) == 0 {
		return HeatFlowDataset{}, nil
	}

	stride := 1
	if rowCount > maxHeatFlowFrames {
		stride = int(math.Ceil(float64(rowCount) / float64(maxHeatFlowFrames)))
	}

	file, err := os.Open(path)
	if err != nil {
		return HeatFlowDataset{}, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	if _, err := reader.Read(); err != nil {
		return HeatFlowDataset{}, err
	}

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
	frameIndex := 0
	keptFrames := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			frameIndex++
			continue
		}
		if stride > 1 && frameIndex%stride != 0 && frameIndex != rowCount-1 {
			frameIndex++
			continue
		}
		label := ""
		if len(record) > 0 {
			label = strings.TrimSpace(record[0])
		}
		dataset.Labels = append(dataset.Labels, label)
		for _, column := range columns {
			if column.index >= len(record) {
				continue
			}
			value, ok := parseCSVFloat(record[column.index])
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
			builder.ensureFrame(keptFrames, len(categories))
			if column.temperature {
				builder.temperature[keptFrames] = roundedHeatFlowNumber(value)
				builder.hasTemperature = true
				dataset.MinTemperature = math.Min(dataset.MinTemperature, value)
				dataset.MaxTemperature = math.Max(dataset.MaxTemperature, value)
				continue
			}
			builder.values[column.categoryIndex][keptFrames] = roundedHeatFlowNumber(value)
			builder.hasHeatFlowData = true
			dataset.MaxAbs = math.Max(dataset.MaxAbs, math.Abs(value))
		}
		keptFrames++
		frameIndex++
	}
	dataset.FrameCount = keptFrames
	if rowCount > keptFrames {
		dataset.Warnings = append(dataset.Warnings, "Heat-flow frames were sampled for interactive rendering.")
	}
	if math.IsInf(dataset.MinTemperature, 0) {
		dataset.MinTemperature = 0
		dataset.MaxTemperature = 0
	} else {
		dataset.MinTemperature = roundedHeatFlowNumber(dataset.MinTemperature)
		dataset.MaxTemperature = roundedHeatFlowNumber(dataset.MaxTemperature)
	}
	dataset.MaxAbs = roundedHeatFlowNumber(dataset.MaxAbs)

	sort.SliceStable(zoneOrder, func(i, j int) bool {
		return strings.ToLower(zoneBuilders[zoneOrder[i]].name) < strings.ToLower(zoneBuilders[zoneOrder[j]].name)
	})
	for _, key := range zoneOrder {
		builder := zoneBuilders[key]
		if !builder.hasHeatFlowData {
			continue
		}
		builder.ensureFrame(keptFrames-1, len(categories))
		zone := HeatFlowZoneSeries{Name: builder.name, Values: builder.values}
		if builder.hasTemperature {
			zone.Temperature = builder.temperature
		}
		dataset.Zones = append(dataset.Zones, zone)
	}
	if len(dataset.Zones) == 0 {
		return HeatFlowDataset{}, nil
	}
	return dataset, nil
}

func parseSimulationHeatFlowESO(path string) (HeatFlowDataset, error) {
	categories, columns, rowCount, err := heatFlowESOColumnsAndFrameCount(path)
	if err != nil {
		return HeatFlowDataset{}, err
	}
	if len(categories) == 0 || len(columns) == 0 || rowCount == 0 {
		return HeatFlowDataset{}, nil
	}

	stride := 1
	if rowCount > maxHeatFlowFrames {
		stride = int(math.Ceil(float64(rowCount) / float64(maxHeatFlowFrames)))
	}

	file, err := os.Open(path)
	if err != nil {
		return HeatFlowDataset{}, err
	}
	defer file.Close()

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
	afterDictionary := false
	frameIndex := -1
	keptFrameIndex := -1
	keepFrame := false

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !afterDictionary {
			if strings.EqualFold(line, "End of Data Dictionary") {
				afterDictionary = true
			}
			continue
		}
		record, err := parseHeatFlowCSVLine(line)
		if err != nil || len(record) == 0 {
			continue
		}
		recordID, err := strconv.Atoi(strings.TrimSpace(record[0]))
		if err != nil {
			continue
		}
		if recordID == 2 {
			frameIndex++
			keepFrame = stride <= 1 || frameIndex%stride == 0 || frameIndex == rowCount-1
			if keepFrame {
				keptFrameIndex++
				dataset.Labels = append(dataset.Labels, heatFlowESOFrameLabel(record))
			}
			continue
		}
		if !keepFrame || keptFrameIndex < 0 {
			continue
		}
		column, ok := columns[recordID]
		if !ok || len(record) < 2 {
			continue
		}
		value, ok := parseCSVFloat(record[1])
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
		if column.temperature {
			builder.temperature[keptFrameIndex] = roundedHeatFlowNumber(value)
			builder.hasTemperature = true
			dataset.MinTemperature = math.Min(dataset.MinTemperature, value)
			dataset.MaxTemperature = math.Max(dataset.MaxTemperature, value)
			continue
		}
		builder.values[column.categoryIndex][keptFrameIndex] = roundedHeatFlowNumber(value)
		builder.hasHeatFlowData = true
		dataset.MaxAbs = math.Max(dataset.MaxAbs, math.Abs(value))
	}
	if err := scanner.Err(); err != nil {
		return HeatFlowDataset{}, err
	}
	dataset.FrameCount = keptFrameIndex + 1
	if rowCount > dataset.FrameCount {
		dataset.Warnings = append(dataset.Warnings, "Heat-flow frames were sampled for interactive rendering.")
	}
	return finalizeHeatFlowDataset(dataset, zoneBuilders, zoneOrder, len(categories))
}

func heatFlowCSVHeaderAndRowCount(path string) ([]string, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, 0, err
	}
	rowCount := 0
	for {
		_, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		rowCount++
	}
	return header, rowCount, nil
}

func heatFlowColumnsFromHeader(header []string) ([]HeatFlowCategory, []heatFlowColumn) {
	definitions := heatFlowCategoryDefinitions()
	var columns []heatFlowColumn
	for index, columnName := range header {
		if index == 0 {
			continue
		}
		if zoneName, ok := heatFlowZoneAndVariable(columnName, heatFlowTemperatureVariables()); ok {
			columns = append(columns, heatFlowColumn{index: index, zoneName: zoneName, temperature: true})
			continue
		}
		for defIndex, definition := range definitions {
			names := append([]string{definition.variable}, definition.aliases...)
			if zoneName, ok := heatFlowZoneAndVariable(columnName, names); ok {
				definition.available = true
				definitions[defIndex] = definition
				columns = append(columns, heatFlowColumn{index: index, zoneName: zoneName, categoryIndex: defIndex})
				break
			}
		}
	}

	categories := []HeatFlowCategory{}
	categoryIndexMap := map[int]int{}
	for index, definition := range definitions {
		if !definition.available {
			continue
		}
		categoryIndexMap[index] = len(categories)
		categories = append(categories, HeatFlowCategory{
			ID:           definition.id,
			Label:        definition.label,
			VariableName: definition.variable,
			Unit:         "W",
			Color:        definition.color,
		})
	}
	for index := range columns {
		if columns[index].temperature {
			continue
		}
		columns[index].categoryIndex = categoryIndexMap[columns[index].categoryIndex]
	}
	return categories, columns
}

func heatFlowESOColumnsAndFrameCount(path string) ([]HeatFlowCategory, map[int]heatFlowColumn, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, err
	}
	defer file.Close()

	definitions := heatFlowCategoryDefinitions()
	columns := map[int]heatFlowColumn{}
	afterDictionary := false
	rowCount := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !afterDictionary {
			if strings.EqualFold(line, "End of Data Dictionary") {
				afterDictionary = true
				continue
			}
			record, err := parseHeatFlowCSVLine(line)
			if err != nil || len(record) < 4 {
				continue
			}
			reportID, err := strconv.Atoi(strings.TrimSpace(record[0]))
			if err != nil {
				continue
			}
			zoneName := strings.TrimSpace(record[2])
			variableName := heatFlowColumnMainName(record[3])
			if zoneName == "" || variableName == "" {
				continue
			}
			if heatFlowVariableMatches(variableName, heatFlowTemperatureVariables()) {
				columns[reportID] = heatFlowColumn{zoneName: zoneName, temperature: true}
				continue
			}
			for defIndex, definition := range definitions {
				names := append([]string{definition.variable}, definition.aliases...)
				if heatFlowVariableMatches(variableName, names) {
					definition.available = true
					definitions[defIndex] = definition
					columns[reportID] = heatFlowColumn{zoneName: zoneName, categoryIndex: defIndex}
					break
				}
			}
			continue
		}
		record, err := parseHeatFlowCSVLine(line)
		if err != nil || len(record) == 0 {
			continue
		}
		reportID, err := strconv.Atoi(strings.TrimSpace(record[0]))
		if err == nil && reportID == 2 {
			rowCount++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, 0, err
	}

	categories, categoryIndexMap := heatFlowCategoriesFromDefinitions(definitions)
	for reportID, column := range columns {
		if column.temperature {
			continue
		}
		categoryIndex, ok := categoryIndexMap[column.categoryIndex]
		if !ok {
			delete(columns, reportID)
			continue
		}
		column.categoryIndex = categoryIndex
		columns[reportID] = column
	}
	return categories, columns, rowCount, nil
}

func heatFlowCategoriesFromDefinitions(definitions []heatFlowCategoryDefinition) ([]HeatFlowCategory, map[int]int) {
	categories := []HeatFlowCategory{}
	categoryIndexMap := map[int]int{}
	for index, definition := range definitions {
		if !definition.available {
			continue
		}
		categoryIndexMap[index] = len(categories)
		categories = append(categories, HeatFlowCategory{
			ID:           definition.id,
			Label:        definition.label,
			VariableName: definition.variable,
			Unit:         "W",
			Color:        definition.color,
		})
	}
	return categories, categoryIndexMap
}

func heatFlowTemperatureVariables() []string {
	return []string{"Zone Mean Air Temperature", "Zone Air Temperature", "Space Mean Air Temperature"}
}

func heatFlowVariableMatches(variableName string, variableNames []string) bool {
	normalizedName := normalizeHeatFlowName(heatFlowColumnMainName(variableName))
	for _, variable := range variableNames {
		if normalizedName == normalizeHeatFlowName(variable) {
			return true
		}
	}
	return false
}

func heatFlowZoneAndVariable(columnName string, variableNames []string) (string, bool) {
	main := heatFlowColumnMainName(columnName)
	normalizedMain := normalizeHeatFlowName(main)
	for _, variable := range variableNames {
		normalizedVariable := normalizeHeatFlowName(variable)
		if normalizedVariable == "" || !strings.HasSuffix(normalizedMain, normalizedVariable) {
			continue
		}
		rawPrefixLen := len(main) - len(variable)
		if rawPrefixLen < 0 {
			rawPrefixLen = 0
		}
		prefix := strings.TrimSpace(main[:rawPrefixLen])
		prefix = strings.TrimSpace(strings.TrimSuffix(prefix, ":"))
		if prefix == "" {
			continue
		}
		return prefix, true
	}
	return "", false
}

func heatFlowColumnMainName(columnName string) string {
	value := strings.TrimSpace(columnName)
	if bang := strings.LastIndex(value, "!"); bang > 0 {
		value = strings.TrimSpace(value[:bang])
	}
	if bracket := strings.LastIndex(value, "["); bracket > 0 {
		value = strings.TrimSpace(value[:bracket])
	}
	return value
}

func parseHeatFlowCSVLine(line string) ([]string, error) {
	reader := csv.NewReader(strings.NewReader(line))
	reader.FieldsPerRecord = -1
	return reader.Read()
}

func heatFlowESOFrameLabel(record []string) string {
	if len(record) < 6 {
		return ""
	}
	month := strings.TrimSpace(record[2])
	day := strings.TrimSpace(record[3])
	hour := strings.TrimSpace(record[5])
	if monthInt, err := strconv.Atoi(month); err == nil {
		month = fmtTwoDigits(monthInt)
	}
	if dayInt, err := strconv.Atoi(day); err == nil {
		day = fmtTwoDigits(dayInt)
	}
	if hourInt, err := strconv.Atoi(hour); err == nil {
		hour = fmtTwoDigits(hourInt)
	}
	return strings.TrimSpace(month + "-" + day + " " + hour + ":00")
}

func fmtTwoDigits(value int) string {
	if value < 0 {
		value = 0
	}
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func finalizeHeatFlowDataset(dataset HeatFlowDataset, zoneBuilders map[string]*heatFlowZoneBuilder, zoneOrder []string, categoryCount int) (HeatFlowDataset, error) {
	if math.IsInf(dataset.MinTemperature, 0) {
		dataset.MinTemperature = 0
		dataset.MaxTemperature = 0
	} else {
		dataset.MinTemperature = roundedHeatFlowNumber(dataset.MinTemperature)
		dataset.MaxTemperature = roundedHeatFlowNumber(dataset.MaxTemperature)
	}
	dataset.MaxAbs = roundedHeatFlowNumber(dataset.MaxAbs)

	sort.SliceStable(zoneOrder, func(i, j int) bool {
		return strings.ToLower(zoneBuilders[zoneOrder[i]].name) < strings.ToLower(zoneBuilders[zoneOrder[j]].name)
	})
	for _, key := range zoneOrder {
		builder := zoneBuilders[key]
		if !builder.hasHeatFlowData {
			continue
		}
		builder.ensureFrame(dataset.FrameCount-1, categoryCount)
		zone := HeatFlowZoneSeries{Name: builder.name, Values: builder.values}
		if builder.hasTemperature {
			zone.Temperature = builder.temperature
		}
		dataset.Zones = append(dataset.Zones, zone)
	}
	if len(dataset.Zones) == 0 {
		return HeatFlowDataset{}, nil
	}
	return dataset, nil
}

func (builder *heatFlowZoneBuilder) ensureFrame(frameIndex int, categoryCount int) {
	if frameIndex < 0 {
		return
	}
	for len(builder.temperature) <= frameIndex {
		builder.temperature = append(builder.temperature, 0)
	}
	for len(builder.values) < categoryCount {
		builder.values = append(builder.values, nil)
	}
	for index := 0; index < categoryCount; index++ {
		for len(builder.values[index]) <= frameIndex {
			builder.values[index] = append(builder.values[index], 0)
		}
	}
}

func normalizeHeatFlowName(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func roundedHeatFlowNumber(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	scale := 1000.0
	if math.Abs(value) >= 10000 {
		scale = 1
	} else if math.Abs(value) >= 1000 {
		scale = 10
	}
	return math.Round(value*scale) / scale
}
