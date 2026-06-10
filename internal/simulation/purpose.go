package simulation

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/idf-analyzer/internal/idf"
)

type SimulationPurposeID string

const (
	SimulationPurposeBasicEnergy   SimulationPurposeID = "basic_energy"
	SimulationPurposeZoneHeatFlow  SimulationPurposeID = "zone_heat_flow"
	SimulationPurposeHVACLoopCheck SimulationPurposeID = "hvac_loop_check"
	SimulationPurposeIntegrity     SimulationPurposeID = "integrity_check"
	SimulationPurposeComfort       SimulationPurposeID = "comfort_check"
	SimulationPurposeCustomOutputs SimulationPurposeID = "custom_outputs"
)

const (
	PurposeFrequencyPolicyDefault           = "purpose_default"
	PurposeFrequencyPolicyPreserve          = "preserve"
	PurposeFrequencyPolicyHighestResolution = "highest_resolution"
	PurposeSQLModeSQLFirst                  = "sql_first"
	PurposeOutputStateExisting              = "existing"
	PurposeOutputStateTemporary             = "temporary"
	PurposeOutputStateWillPersist           = "will_be_persisted"
	PurposeOutputStateConflict              = "conflict"
)

type SimulationPurposeRequest struct {
	Purposes         []SimulationPurposeID  `json:"purposes"`
	Scope            SimulationPurposeScope `json:"scope"`
	FrequencyPolicy  string                 `json:"frequencyPolicy,omitempty"`
	SQLMode          string                 `json:"sqlMode,omitempty"`
	PersistOutputs   bool                   `json:"persistOutputs,omitempty"`
	DiscoveryAllowed bool                   `json:"discoveryAllowed,omitempty"`
}

type SimulationPurposeScope struct {
	ZoneMode           string                `json:"zoneMode,omitempty"`
	ZoneNames          []string              `json:"zoneNames,omitempty"`
	LoopMode           string                `json:"loopMode,omitempty"`
	AirLoopNames       []string              `json:"airLoopNames,omitempty"`
	PlantLoopNames     []string              `json:"plantLoopNames,omitempty"`
	CondenserLoopNames []string              `json:"condenserLoopNames,omitempty"`
	ComponentIDs       []string              `json:"componentIds,omitempty"`
	OutputSignatures   []string              `json:"outputSignatures,omitempty"`
	CustomOutputs      []PurposeCustomOutput `json:"customOutputs,omitempty"`
}

type PurposeCustomOutput struct {
	ObjectType         string                 `json:"objectType"`
	KeyValue           string                 `json:"keyValue,omitempty"`
	VariableName       string                 `json:"variableName,omitempty"`
	MeterName          string                 `json:"meterName,omitempty"`
	ReportingFrequency string                 `json:"reportingFrequency,omitempty"`
	Fields             []idf.OutputFieldValue `json:"fields,omitempty"`
}

type PurposeRunPlan struct {
	Purposes          []SimulationPurposeID `json:"purposes"`
	OutputObjects     []PurposeOutputObject `json:"outputObjects"`
	EstimatedWeight   string                `json:"estimatedWeight"`
	EstimatedSeries   int                   `json:"estimatedSeries"`
	EstimatedFrames   int                   `json:"estimatedFrames"`
	RequiresSQL       bool                  `json:"requiresSQL"`
	RequiresDiscovery bool                  `json:"requiresDiscovery"`
	Warnings          []PurposeRunWarning   `json:"warnings,omitempty"`
}

type PurposeOutputObject struct {
	ObjectType         string                 `json:"objectType"`
	Fields             []idf.OutputFieldValue `json:"fields"`
	PurposeIDs         []SimulationPurposeID  `json:"purposeIds"`
	State              string                 `json:"state"`
	Weight             string                 `json:"weight"`
	Signature          string                 `json:"signature"`
	KeyValue           string                 `json:"keyValue,omitempty"`
	VariableName       string                 `json:"variableName,omitempty"`
	ReportingFrequency string                 `json:"reportingFrequency,omitempty"`
	Description        string                 `json:"description,omitempty"`
	Reason             string                 `json:"reason,omitempty"`
}

type PurposeRunWarning struct {
	Severity        string              `json:"severity"`
	Code            string              `json:"code"`
	Message         string              `json:"message"`
	PurposeID       SimulationPurposeID `json:"purposeId,omitempty"`
	OutputSignature string              `json:"outputSignature,omitempty"`
}

type PurposeResultBundle struct {
	Energy       EnergyDashboardResult     `json:"energy,omitempty"`
	ZoneHeatFlow HeatFlowDataset           `json:"zoneHeatFlow,omitempty"`
	HVACLoops    []HVACLoopRunResult       `json:"hvacLoops,omitempty"`
	Comfort      ComfortResult             `json:"comfort,omitempty"`
	Integrity    IntegrityResult           `json:"integrity,omitempty"`
	Series       []SimulationSeries        `json:"series,omitempty"`
	Completeness []PurposeCompletenessItem `json:"completeness,omitempty"`
}

type EnergyDashboardResult struct {
	FacilityMonthly []EnergySeries            `json:"facilityMonthly,omitempty"`
	EndUseMonthly   []EnergySeries            `json:"endUseMonthly,omitempty"`
	ZoneMonthly     []ZoneEnergySeries        `json:"zoneMonthly,omitempty"`
	Totals          []EnergyTotal             `json:"totals,omitempty"`
	Completeness    []PurposeCompletenessItem `json:"completeness,omitempty"`
}

type EnergySeries struct {
	Name   string            `json:"name"`
	Unit   string            `json:"unit,omitempty"`
	Source string            `json:"source,omitempty"`
	Points []SimulationPoint `json:"points,omitempty"`
	Total  float64           `json:"total"`
}

type ZoneEnergySeries struct {
	ZoneName string            `json:"zoneName"`
	Metric   string            `json:"metric"`
	Unit     string            `json:"unit,omitempty"`
	Source   string            `json:"source,omitempty"`
	Points   []SimulationPoint `json:"points,omitempty"`
	Total    float64           `json:"total"`
}

type EnergyTotal struct {
	Name   string  `json:"name"`
	Unit   string  `json:"unit,omitempty"`
	Source string  `json:"source,omitempty"`
	Value  float64 `json:"value"`
}

type HVACLoopRunResult struct {
	Name         string                    `json:"name"`
	LoopType     string                    `json:"loopType,omitempty"`
	Series       []SimulationSeries        `json:"series,omitempty"`
	Completeness []PurposeCompletenessItem `json:"completeness,omitempty"`
}

type ComfortResult struct {
	Zones        []ComfortZoneResult       `json:"zones,omitempty"`
	Series       []SimulationSeries        `json:"series,omitempty"`
	Completeness []PurposeCompletenessItem `json:"completeness,omitempty"`
}

type ComfortZoneResult struct {
	ZoneName string                `json:"zoneName"`
	Metrics  []ComfortMetricResult `json:"metrics,omitempty"`
}

type ComfortMetricResult struct {
	Name    string            `json:"name"`
	Unit    string            `json:"unit,omitempty"`
	Source  string            `json:"source,omitempty"`
	Min     float64           `json:"min"`
	Max     float64           `json:"max"`
	Average float64           `json:"average"`
	Points  []SimulationPoint `json:"points,omitempty"`
}

type IntegrityResult struct {
	Status         string                   `json:"status"`
	ERR            ERRSummary               `json:"err"`
	Files          []SimulationFileInfo     `json:"files,omitempty"`
	Completed      bool                     `json:"completed"`
	SQLIssues      []IntegritySQLIssue      `json:"sqlIssues,omitempty"`
	TabularReports []IntegrityTabularReport `json:"tabularReports,omitempty"`
}

type IntegritySQLIssue struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Count    int    `json:"count,omitempty"`
	Source   string `json:"source,omitempty"`
}

type IntegrityTabularReport struct {
	ReportName string                `json:"reportName"`
	For        string                `json:"for,omitempty"`
	TableName  string                `json:"tableName"`
	Columns    []string              `json:"columns,omitempty"`
	Rows       []IntegrityTabularRow `json:"rows,omitempty"`
	Source     string                `json:"source,omitempty"`
}

type IntegrityTabularRow struct {
	Name   string            `json:"name"`
	Values map[string]string `json:"values,omitempty"`
}

type PurposeCompletenessItem struct {
	PurposeID      SimulationPurposeID `json:"purposeId"`
	RequiredOutput string              `json:"requiredOutput"`
	Found          bool                `json:"found"`
	Source         string              `json:"source"`
	Message        string              `json:"message,omitempty"`
}

type purposePlanBuilder struct {
	doc              idf.Document
	request          SimulationPurposeRequest
	existing         map[string]PurposeOutputObject
	existingBase     map[string]PurposeOutputObject
	recommendations  map[string]idf.OutputRecommendation
	objects          []PurposeOutputObject
	bySignature      map[string]int
	warnings         []PurposeRunWarning
	runPeriodDays    int
	timestepsPerHour int
}

func NormalizeSimulationPurposeRequest(request *SimulationPurposeRequest) SimulationPurposeRequest {
	var normalized SimulationPurposeRequest
	if request != nil {
		normalized = *request
	}
	normalized.Purposes = normalizePurposeIDs(normalized.Purposes)
	if len(normalized.Purposes) == 0 {
		normalized.Purposes = []SimulationPurposeID{SimulationPurposeBasicEnergy, SimulationPurposeZoneHeatFlow}
	}
	normalized.FrequencyPolicy = strings.TrimSpace(normalized.FrequencyPolicy)
	if normalized.FrequencyPolicy == "" {
		normalized.FrequencyPolicy = PurposeFrequencyPolicyDefault
	}
	normalized.SQLMode = strings.TrimSpace(normalized.SQLMode)
	if normalized.SQLMode == "" {
		normalized.SQLMode = PurposeSQLModeSQLFirst
	}
	normalized.Scope.ZoneMode = strings.TrimSpace(normalized.Scope.ZoneMode)
	if normalized.Scope.ZoneMode == "" {
		normalized.Scope.ZoneMode = "all"
	}
	normalized.Scope.LoopMode = strings.TrimSpace(normalized.Scope.LoopMode)
	if normalized.Scope.LoopMode == "" {
		normalized.Scope.LoopMode = "all"
	}
	normalized.Scope.ZoneNames = normalizePurposeStrings(normalized.Scope.ZoneNames)
	normalized.Scope.AirLoopNames = normalizePurposeStrings(normalized.Scope.AirLoopNames)
	normalized.Scope.PlantLoopNames = normalizePurposeStrings(normalized.Scope.PlantLoopNames)
	normalized.Scope.CondenserLoopNames = normalizePurposeStrings(normalized.Scope.CondenserLoopNames)
	normalized.Scope.ComponentIDs = normalizePurposeStrings(normalized.Scope.ComponentIDs)
	normalized.Scope.OutputSignatures = normalizePurposeStrings(normalized.Scope.OutputSignatures)
	return normalized
}

func BuildPurposeRunPlan(doc idf.Document, request SimulationPurposeRequest) PurposeRunPlan {
	request = NormalizeSimulationPurposeRequest(&request)
	builder := newPurposePlanBuilder(doc, request)
	builder.addSQLBase()
	for _, purposeID := range request.Purposes {
		switch purposeID {
		case SimulationPurposeBasicEnergy:
			builder.addBasicEnergy()
		case SimulationPurposeZoneHeatFlow:
			builder.addZoneHeatFlow()
		case SimulationPurposeHVACLoopCheck:
			builder.addHVACLoopCheck()
		case SimulationPurposeIntegrity:
			builder.addIntegrity()
		case SimulationPurposeComfort:
			builder.addComfort()
		case SimulationPurposeCustomOutputs:
			builder.addCustomOutputs()
		}
	}
	return builder.plan()
}

func PurposeRunPlanApplyRequest(plan PurposeRunPlan) idf.OutputApplyRequest {
	additions := make([]idf.OutputObjectRequest, 0, len(plan.OutputObjects))
	for _, object := range plan.OutputObjects {
		if object.State == PurposeOutputStateExisting {
			continue
		}
		additions = append(additions, idf.OutputObjectRequest{
			ObjectType: object.ObjectType,
			Fields:     object.Fields,
			Reason:     object.Reason,
		})
	}
	return idf.OutputApplyRequest{AddObjects: additions}
}

func PurposeRunPlanTemporaryOutputDiff(plan PurposeRunPlan) string {
	var builder strings.Builder
	added := 0
	for _, object := range plan.OutputObjects {
		if object.State == PurposeOutputStateExisting {
			continue
		}
		if added == 0 {
			builder.WriteString("--- original.idf\n")
			builder.WriteString("+++ purpose-run-copy.idf\n")
		}
		added++
		builder.WriteString(fmt.Sprintf("+%s,\n", object.ObjectType))
		for index, field := range object.Fields {
			terminator := ","
			if index == len(object.Fields)-1 {
				terminator = ";"
			}
			builder.WriteString(fmt.Sprintf("+  %s%s  !- %s\n", field.Value, terminator, field.Name))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func PurposeOutputSignature(objectType string, fields []idf.OutputFieldValue) string {
	parts := []string{normalizePurposeToken(objectType)}
	for _, field := range fields {
		name := normalizePurposeFieldName(field.Name)
		value := strings.TrimSpace(field.Value)
		if name == "reporting frequency" {
			value = canonicalPurposeFrequency(value)
		}
		parts = append(parts, name+"="+normalizePurposeToken(value))
	}
	return strings.Join(parts, "|")
}

func BuildPurposeResultBundle(result *SimulationRunResult, request SimulationPurposeRequest) PurposeResultBundle {
	request = NormalizeSimulationPurposeRequest(&request)
	bundle := PurposeResultBundle{
		Series: append([]SimulationSeries(nil), result.Series...),
	}
	for _, purposeID := range request.Purposes {
		switch purposeID {
		case SimulationPurposeBasicEnergy:
			bundle.Energy = buildEnergyDashboardResultFromFiles(result.Files)
			if len(bundle.Energy.FacilityMonthly)+len(bundle.Energy.EndUseMonthly)+len(bundle.Energy.ZoneMonthly) == 0 {
				bundle.Energy = buildEnergyDashboardResult(result.Series)
			}
			bundle.Completeness = append(bundle.Completeness, purposeCompleteness(
				SimulationPurposeBasicEnergy,
				"monthly energy series",
				len(bundle.Energy.FacilityMonthly)+len(bundle.Energy.EndUseMonthly)+len(bundle.Energy.ZoneMonthly) > 0,
				energyDashboardSource(bundle.Energy, result.Series),
			))
			bundle.Energy.Completeness = append([]PurposeCompletenessItem(nil), bundle.Completeness...)
		case SimulationPurposeZoneHeatFlow:
			bundle.ZoneHeatFlow = result.HeatFlow
			source := "missing"
			if result.HeatFlow.SourceFile != "" {
				source = simulationSourceFromFilename(result.HeatFlow.SourceFile)
			}
			bundle.Completeness = append(bundle.Completeness, purposeCompleteness(
				SimulationPurposeZoneHeatFlow,
				"zone heat-flow ledger",
				len(result.HeatFlow.Zones) > 0,
				source,
			))
		case SimulationPurposeHVACLoopCheck:
			bundle.HVACLoops = buildHVACLoopRunResults(result.Series, request)
			bundle.Completeness = append(bundle.Completeness, purposeCompleteness(
				SimulationPurposeHVACLoopCheck,
				"HVAC node state series",
				len(bundle.HVACLoops) > 0 && len(bundle.HVACLoops[0].Series) > 0,
				hvacLoopResultSource(bundle.HVACLoops),
			))
		case SimulationPurposeComfort:
			bundle.Comfort = buildComfortResult(result.Series)
			bundle.Completeness = append(bundle.Completeness, purposeCompleteness(
				SimulationPurposeComfort,
				"zone comfort series",
				len(bundle.Comfort.Series) > 0,
				seriesSource(bundle.Comfort.Series),
			))
		case SimulationPurposeIntegrity:
			sqlIntegrity := buildIntegritySQLResultFromFiles(result.Files)
			bundle.Integrity = IntegrityResult{
				Status:         result.Status,
				ERR:            result.ERR,
				Files:          append([]SimulationFileInfo(nil), result.Files...),
				Completed:      result.ERR.Completed,
				SQLIssues:      sqlIntegrity.Issues,
				TabularReports: sqlIntegrity.TabularReports,
			}
			bundle.Completeness = append(bundle.Completeness, purposeCompleteness(
				SimulationPurposeIntegrity,
				"ERR summary",
				result.ERR.Path != "",
				"err",
			))
			bundle.Completeness = append(bundle.Completeness, purposeCompleteness(
				SimulationPurposeIntegrity,
				"SQL error table",
				sqlIntegrity.HasErrorsTable,
				sqlIntegritySource(sqlIntegrity, "missing"),
			))
			bundle.Completeness = append(bundle.Completeness, purposeCompleteness(
				SimulationPurposeIntegrity,
				"SQL tabular reports",
				sqlIntegrity.HasTabularData,
				sqlIntegritySource(sqlIntegrity, "missing"),
			))
		}
	}
	return bundle
}

func buildHVACLoopRunResults(series []SimulationSeries, request SimulationPurposeRequest) []HVACLoopRunResult {
	nodeSeries := []SimulationSeries{}
	foundVariables := map[string]bool{}
	for _, item := range series {
		if !purposeSeriesMatchesVariables(item.Column, hvacLoopCheckNodeVariables()) {
			continue
		}
		nodeSeries = append(nodeSeries, item)
		_, variableName := splitPurposeSeriesColumn(item.Column)
		if variableName != "" {
			foundVariables[normalizePurposeToken(variableName)] = true
		}
	}
	if len(nodeSeries) == 0 {
		return nil
	}
	result := HVACLoopRunResult{
		Name:         hvacLoopResultName(request.Scope),
		LoopType:     hvacLoopResultType(request.Scope),
		Series:       nodeSeries,
		Completeness: hvacNodeSeriesCompleteness(foundVariables, seriesSource(nodeSeries)),
	}
	return []HVACLoopRunResult{result}
}

func hvacNodeSeriesCompleteness(found map[string]bool, source string) []PurposeCompletenessItem {
	items := make([]PurposeCompletenessItem, 0, len(hvacLoopCheckNodeVariables()))
	for _, variable := range hvacLoopCheckNodeVariables() {
		items = append(items, purposeCompleteness(
			SimulationPurposeHVACLoopCheck,
			variable,
			found[normalizePurposeToken(variable)],
			source,
		))
	}
	return items
}

func hvacLoopResultName(scope SimulationPurposeScope) string {
	names := append([]string{}, scope.AirLoopNames...)
	names = append(names, scope.PlantLoopNames...)
	names = append(names, scope.CondenserLoopNames...)
	switch len(names) {
	case 0:
		return "HVAC Loop Check"
	case 1:
		return names[0]
	default:
		return fmt.Sprintf("%d selected HVAC loops", len(names))
	}
}

func hvacLoopResultType(scope SimulationPurposeScope) string {
	types := []string{}
	if len(scope.AirLoopNames) > 0 {
		types = append(types, "AirLoopHVAC")
	}
	if len(scope.PlantLoopNames) > 0 {
		types = append(types, "PlantLoop")
	}
	if len(scope.CondenserLoopNames) > 0 {
		types = append(types, "CondenserLoop")
	}
	return strings.Join(types, ", ")
}

func hvacLoopResultSource(results []HVACLoopRunResult) string {
	for _, result := range results {
		if source := seriesSource(result.Series); source != "missing" {
			return source
		}
	}
	return "missing"
}

func buildComfortResult(series []SimulationSeries) ComfortResult {
	result := ComfortResult{}
	foundVariables := map[string]bool{}
	zoneMap := map[string]*ComfortZoneResult{}
	zoneOrder := []string{}
	for _, item := range series {
		if !purposeSeriesMatchesVariables(item.Column, comfortCheckVariables()) {
			continue
		}
		keyValue, variableName := splitPurposeSeriesColumn(item.Column)
		if keyValue == "" {
			keyValue = "Unknown Zone"
		}
		foundVariables[normalizePurposeToken(variableName)] = true
		result.Series = append(result.Series, item)
		zoneKey := normalizePurposeToken(keyValue)
		zone := zoneMap[zoneKey]
		if zone == nil {
			zone = &ComfortZoneResult{ZoneName: keyValue}
			zoneMap[zoneKey] = zone
			zoneOrder = append(zoneOrder, zoneKey)
		}
		zone.Metrics = append(zone.Metrics, ComfortMetricResult{
			Name:    variableName,
			Unit:    unitFromSeriesColumn(item.Column),
			Source:  item.File,
			Min:     item.Min,
			Max:     item.Max,
			Average: item.Average,
			Points:  append([]SimulationPoint(nil), item.Points...),
		})
	}
	sort.SliceStable(zoneOrder, func(i, j int) bool {
		return strings.ToLower(zoneMap[zoneOrder[i]].ZoneName) < strings.ToLower(zoneMap[zoneOrder[j]].ZoneName)
	})
	for _, key := range zoneOrder {
		zone := zoneMap[key]
		sort.SliceStable(zone.Metrics, func(i, j int) bool {
			return strings.ToLower(zone.Metrics[i].Name) < strings.ToLower(zone.Metrics[j].Name)
		})
		result.Zones = append(result.Zones, *zone)
	}
	result.Completeness = comfortSeriesCompleteness(foundVariables, seriesSource(result.Series))
	return result
}

func comfortSeriesCompleteness(found map[string]bool, source string) []PurposeCompletenessItem {
	items := make([]PurposeCompletenessItem, 0, len(comfortCheckVariables()))
	for _, variable := range comfortCheckVariables() {
		items = append(items, purposeCompleteness(
			SimulationPurposeComfort,
			variable,
			found[normalizePurposeToken(variable)],
			source,
		))
	}
	return items
}

func buildEnergyDashboardResultFromFiles(files []SimulationFileInfo) EnergyDashboardResult {
	for _, file := range files {
		if file.Kind != "sqlite" {
			continue
		}
		result, err := parseSimulationEnergySQL(file.Path)
		if err == nil && len(result.FacilityMonthly)+len(result.EndUseMonthly)+len(result.ZoneMonthly) > 0 {
			return result
		}
	}
	return EnergyDashboardResult{}
}

func energyDashboardSource(result EnergyDashboardResult, series []SimulationSeries) string {
	for _, item := range result.FacilityMonthly {
		if item.Source != "" {
			return simulationSourceFromFilename(item.Source)
		}
	}
	for _, item := range result.EndUseMonthly {
		if item.Source != "" {
			return simulationSourceFromFilename(item.Source)
		}
	}
	for _, item := range result.ZoneMonthly {
		if item.Source != "" {
			return simulationSourceFromFilename(item.Source)
		}
	}
	return seriesSource(series)
}

func buildIntegritySQLResultFromFiles(files []SimulationFileInfo) integritySQLParseResult {
	for _, file := range files {
		if file.Kind != "sqlite" {
			continue
		}
		result, err := parseSimulationIntegritySQL(file.Path)
		if err == nil && (result.HasErrorsTable || result.HasTabularData || len(result.Issues)+len(result.TabularReports) > 0) {
			return result
		}
	}
	return integritySQLParseResult{}
}

func sqlIntegritySource(result integritySQLParseResult, fallback string) string {
	if result.Source != "" {
		return simulationSourceFromFilename(result.Source)
	}
	return fallback
}

func newPurposePlanBuilder(doc idf.Document, request SimulationPurposeRequest) *purposePlanBuilder {
	existing := collectExistingPurposeOutputs(doc)
	existingBase := map[string]PurposeOutputObject{}
	for _, object := range existing {
		base := purposeOutputBaseSignature(object.ObjectType, object.Fields)
		if _, ok := existingBase[base]; !ok {
			existingBase[base] = object
		}
	}
	report := idf.AnalyzeOutput(doc)
	recommendations := map[string]idf.OutputRecommendation{}
	for _, item := range report.Recommendations {
		recommendations[item.ID] = item
	}
	return &purposePlanBuilder{
		doc:              doc,
		request:          request,
		existing:         existing,
		existingBase:     existingBase,
		recommendations:  recommendations,
		bySignature:      map[string]int{},
		runPeriodDays:    estimatedRunPeriodDays(doc),
		timestepsPerHour: estimatedTimestepsPerHour(doc),
	}
}

func (builder *purposePlanBuilder) addSQLBase() {
	builder.addObject(PurposeOutputObject{
		ObjectType: "Output:SQLite",
		Fields: []idf.OutputFieldValue{
			{Name: "Option Type", Value: "SimpleAndTabular"},
			{Name: "Unit Conversion for Tabular Data", Value: "JtoKWH"},
		},
		PurposeIDs:  append([]SimulationPurposeID(nil), builder.request.Purposes...),
		Weight:      "light",
		Description: "Primary SQL result source with tabular data.",
		Reason:      "Purpose run base SQL output",
	})
}

func (builder *purposePlanBuilder) addBasicEnergy() {
	for _, id := range []string{
		"standard-meter-electricity-facility",
		"standard-meter-naturalgas-facility",
		"standard-meter-district-cooling-facility",
		"standard-meter-district-heating-facility",
		"standard-meter-water-facility",
		"standard-meter-electricity-cooling",
		"standard-meter-electricity-heating",
		"standard-meter-electricity-interior-lights",
		"standard-meter-electricity-interior-equipment",
		"standard-meter-electricity-fans",
		"standard-meter-electricity-pumps",
		"standard-meter-electricity-heat-rejection",
		"standard-meter-electricity-water-systems",
		"standard-meter-naturalgas-heating",
		"standard-meter-naturalgas-water-systems",
	} {
		builder.addRecommendation(id, SimulationPurposeBasicEnergy)
	}
	if !docHasObject(builder.doc, "Zone") {
		return
	}
	for _, variable := range []string{
		"Zone Lights Electricity Energy",
		"Zone Electric Equipment Electricity Energy",
		"Zone Gas Equipment Gas Energy",
		"Zone Air System Sensible Heating Energy",
		"Zone Air System Sensible Cooling Energy",
	} {
		builder.addVariable(SimulationPurposeBasicEnergy, "*", variable, "Monthly", "medium", "Monthly zone-level reported energy.")
	}
}

func (builder *purposePlanBuilder) addZoneHeatFlow() {
	if !docHasObject(builder.doc, "Zone") {
		builder.warn("warning", "zone_scope_empty", "Zone Heat Flow needs Zone objects, but none were found.", SimulationPurposeZoneHeatFlow, "")
		return
	}
	keys := []string{"*"}
	if strings.EqualFold(builder.request.Scope.ZoneMode, "selected") && len(builder.request.Scope.ZoneNames) > 0 {
		keys = builder.request.Scope.ZoneNames
		builder.warn("info", "zone_scope_selected", fmt.Sprintf("Zone Heat Flow is limited to %d selected zone key(s).", len(keys)), SimulationPurposeZoneHeatFlow, "")
	} else {
		builder.warn("info", "wildcard_zone_heat_flow", "Zone Heat Flow uses wildcard zone keys until selected-zone scoping is requested.", SimulationPurposeZoneHeatFlow, "")
	}
	for _, key := range keys {
		builder.addVariable(SimulationPurposeZoneHeatFlow, key, "Zone Mean Air Temperature", "Hourly", "medium", "Hourly zone temperature for heat-flow overlays.")
		for _, variable := range zoneHeatFlowVariableNames() {
			builder.addVariable(SimulationPurposeZoneHeatFlow, key, variable, "Hourly", "medium", "Hourly zone heat-balance ledger variable.")
		}
	}
}

func (builder *purposePlanBuilder) addHVACLoopCheck() {
	keys := []string{"*"}
	targets := builder.hvacLoopCheckTargets()
	if targets.Selected {
		if len(targets.NodeNames) == 0 {
			builder.warn("warning", "hvac_scope_unresolved", "HVAC Loop Check was limited to selected loops, but no loop nodes were resolved.", SimulationPurposeHVACLoopCheck, "")
			return
		}
		keys = targets.NodeNames
		builder.warn("info", "hvac_scope_selected", fmt.Sprintf("HVAC Loop Check is limited to %d node key(s) across %d selected loop(s) and %d component(s).", len(targets.NodeNames), targets.LoopCount, len(targets.ComponentNames)), SimulationPurposeHVACLoopCheck, "")
	} else {
		builder.warn("warning", "hvac_scope_wildcard", "HVAC Loop Check currently uses wildcard node keys when no loop scope is selected.", SimulationPurposeHVACLoopCheck, "")
	}
	for _, key := range keys {
		for _, variable := range hvacLoopCheckNodeVariables() {
			builder.addVariable(SimulationPurposeHVACLoopCheck, key, variable, "Hourly", "heavy", "Hourly system node state for loop inspection.")
		}
	}
}

type hvacLoopCheckTargets struct {
	Selected       bool
	LoopCount      int
	NodeNames      []string
	ComponentNames []string
}

func (builder *purposePlanBuilder) hvacLoopCheckTargets() hvacLoopCheckTargets {
	scope := builder.request.Scope
	selectedNames := map[string]map[string]bool{
		"AirLoopHVAC":   purposeNameSet(scope.AirLoopNames),
		"PlantLoop":     purposeNameSet(scope.PlantLoopNames),
		"CondenserLoop": purposeNameSet(scope.CondenserLoopNames),
	}
	selected := len(scope.AirLoopNames)+len(scope.PlantLoopNames)+len(scope.CondenserLoopNames) > 0 || strings.EqualFold(scope.LoopMode, "selected")
	targets := hvacLoopCheckTargets{Selected: selected}
	if !selected {
		return targets
	}
	report := idf.AnalyzeHVAC(builder.doc)
	nodeSet := map[string]string{}
	componentSet := map[string]string{}
	for _, loop := range report.Loops {
		if !purposeHVACLoopSelected(loop, selectedNames) {
			continue
		}
		targets.LoopCount++
		for _, node := range purposeHVACLoopNodes(loop) {
			nodeSet[normalizePurposeToken(node)] = strings.TrimSpace(node)
		}
		for _, component := range purposeHVACLoopComponents(loop) {
			if component.ObjectName != "" {
				componentSet[normalizePurposeToken(component.ObjectName)] = strings.TrimSpace(component.ObjectName)
			}
		}
	}
	targets.NodeNames = purposeSortedSet(nodeSet)
	targets.ComponentNames = purposeSortedSet(componentSet)
	return targets
}

func purposeHVACLoopSelected(loop idf.HVACLoop, selectedNames map[string]map[string]bool) bool {
	wanted := selectedNames[loop.Type]
	if len(wanted) == 0 {
		return false
	}
	return wanted[normalizePurposeToken(loop.Name)]
}

func purposeHVACLoopNodes(loop idf.HVACLoop) []string {
	out := []string{}
	add := func(value string) {
		out = appendUniquePurposeString(out, value)
	}
	for _, side := range []idf.HVACLoopSide{loop.SupplySide, loop.DemandSide} {
		add(side.InletNode)
		add(side.OutletNode)
		for _, branch := range side.Branches {
			add(branch.InletNode)
			add(branch.OutletNode)
			for _, component := range branch.Components {
				add(component.InletNode)
				add(component.OutletNode)
				add(component.WaterInletNode)
				add(component.WaterOutletNode)
				for _, usage := range component.NodeUsages {
					add(usage.NodeName)
				}
			}
		}
	}
	if loop.DemandGraph.SupplyPath != nil {
		add(loop.DemandGraph.SupplyPath.InletNode)
		add(loop.DemandGraph.SupplyPath.OutletNode)
		for _, component := range loop.DemandGraph.SupplyPath.Components {
			for _, node := range component.InletNodes {
				add(node)
			}
			for _, node := range component.OutletNodes {
				add(node)
			}
		}
	}
	if loop.DemandGraph.ReturnPath != nil {
		add(loop.DemandGraph.ReturnPath.InletNode)
		add(loop.DemandGraph.ReturnPath.OutletNode)
		for _, component := range loop.DemandGraph.ReturnPath.Components {
			for _, node := range component.InletNodes {
				add(node)
			}
			for _, node := range component.OutletNodes {
				add(node)
			}
		}
	}
	sort.Strings(out)
	return out
}

func purposeHVACLoopComponents(loop idf.HVACLoop) []idf.HVACComponent {
	out := []idf.HVACComponent{}
	for _, side := range []idf.HVACLoopSide{loop.SupplySide, loop.DemandSide} {
		for _, branch := range side.Branches {
			out = append(out, branch.Components...)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].ObjectName) < strings.ToLower(out[j].ObjectName)
	})
	return out
}

func hvacLoopCheckNodeVariables() []string {
	return []string{
		"System Node Temperature",
		"System Node Mass Flow Rate",
		"System Node Setpoint Temperature",
		"System Node Humidity Ratio",
		"System Node Enthalpy",
	}
}

func (builder *purposePlanBuilder) addIntegrity() {
	for _, id := range []string{
		"standard-variable-dictionary",
		"standard-summary-all",
		"standard-table-style-html",
	} {
		builder.addRecommendation(id, SimulationPurposeIntegrity)
	}
	builder.addObject(PurposeOutputObject{
		ObjectType: "Output:Diagnostics",
		Fields: []idf.OutputFieldValue{
			{Name: "Key 1", Value: "DisplayExtraWarnings"},
		},
		PurposeIDs:  []SimulationPurposeID{SimulationPurposeIntegrity},
		Weight:      "light",
		Description: "Additional EnergyPlus diagnostic warnings.",
		Reason:      "Integrity Check diagnostic output",
	})
}

func (builder *purposePlanBuilder) addComfort() {
	if !docHasObject(builder.doc, "Zone") {
		builder.warn("warning", "comfort_scope_empty", "Comfort Check needs Zone objects, but none were found.", SimulationPurposeComfort, "")
		return
	}
	keys := []string{"*"}
	if strings.EqualFold(builder.request.Scope.ZoneMode, "selected") && len(builder.request.Scope.ZoneNames) > 0 {
		keys = builder.request.Scope.ZoneNames
		builder.warn("info", "comfort_scope_selected", fmt.Sprintf("Comfort Check is limited to %d selected zone key(s).", len(keys)), SimulationPurposeComfort, "")
	}
	for _, key := range keys {
		for _, variable := range comfortCheckVariables() {
			builder.addVariable(SimulationPurposeComfort, key, variable, "Hourly", "medium", "Hourly zone comfort and setpoint trend.")
		}
	}
}

func (builder *purposePlanBuilder) addCustomOutputs() {
	if len(builder.request.Scope.CustomOutputs) == 0 {
		builder.warn("warning", "custom_outputs_empty", "Custom Outputs purpose is selected but no custom output objects were provided.", SimulationPurposeCustomOutputs, "")
		return
	}
	for _, custom := range builder.request.Scope.CustomOutputs {
		objectType := strings.TrimSpace(custom.ObjectType)
		if objectType == "" {
			if strings.TrimSpace(custom.MeterName) != "" {
				objectType = "Output:Meter"
			} else {
				objectType = "Output:Variable"
			}
		}
		fields := custom.Fields
		if len(fields) == 0 {
			frequency := canonicalPurposeFrequency(custom.ReportingFrequency)
			if strings.EqualFold(objectType, "Output:Meter") {
				fields = []idf.OutputFieldValue{
					{Name: "Key Name", Value: custom.MeterName},
					{Name: "Reporting Frequency", Value: frequency},
				}
			} else {
				fields = []idf.OutputFieldValue{
					{Name: "Key Value", Value: custom.KeyValue},
					{Name: "Variable Name", Value: custom.VariableName},
					{Name: "Reporting Frequency", Value: frequency},
				}
			}
		}
		builder.addObject(PurposeOutputObject{
			ObjectType:  objectType,
			Fields:      fields,
			PurposeIDs:  []SimulationPurposeID{SimulationPurposeCustomOutputs},
			Weight:      "medium",
			Description: "User-selected custom output.",
			Reason:      "Custom Outputs purpose",
		})
	}
}

func (builder *purposePlanBuilder) addRecommendation(id string, purposeID SimulationPurposeID) {
	item, ok := builder.recommendations[id]
	if !ok {
		return
	}
	builder.addObject(PurposeOutputObject{
		ObjectType:  item.ObjectType,
		Fields:      item.Fields,
		PurposeIDs:  []SimulationPurposeID{purposeID},
		Weight:      purposeWeight(item.ObjectType, item.Fields),
		Description: item.Description,
		Reason:      item.Label,
	})
}

func (builder *purposePlanBuilder) addVariable(purposeID SimulationPurposeID, keyValue string, variableName string, frequency string, weight string, description string) {
	builder.addObject(PurposeOutputObject{
		ObjectType: "Output:Variable",
		Fields: []idf.OutputFieldValue{
			{Name: "Key Value", Value: keyValue},
			{Name: "Variable Name", Value: variableName},
			{Name: "Reporting Frequency", Value: frequency},
		},
		PurposeIDs:  []SimulationPurposeID{purposeID},
		Weight:      weight,
		Description: description,
		Reason:      string(purposeID) + " purpose",
	})
}

func (builder *purposePlanBuilder) addObject(object PurposeOutputObject) {
	object.ObjectType = strings.TrimSpace(object.ObjectType)
	object.Fields = normalizePurposeOutputFields(object.ObjectType, object.Fields)
	object.PurposeIDs = normalizePurposeIDs(object.PurposeIDs)
	object.Signature = PurposeOutputSignature(object.ObjectType, object.Fields)
	object.KeyValue = purposeOutputKeyValue(object.Fields)
	object.VariableName = purposeOutputVariableName(object.Fields)
	object.ReportingFrequency = purposeOutputFrequency(object.ObjectType, object.Fields)
	if object.Weight == "" {
		object.Weight = purposeWeight(object.ObjectType, object.Fields)
	}
	if existing, ok := builder.existing[object.Signature]; ok {
		object.State = PurposeOutputStateExisting
		object.Reason = existing.Reason
	} else if conflict, ok := builder.existingBase[purposeOutputBaseSignature(object.ObjectType, object.Fields)]; ok {
		object = builder.applyFrequencyConflictPolicy(object, conflict)
	} else if builder.request.PersistOutputs {
		object.State = PurposeOutputStateWillPersist
	} else {
		object.State = PurposeOutputStateTemporary
	}
	if index, ok := builder.bySignature[object.Signature]; ok {
		current := &builder.objects[index]
		current.PurposeIDs = normalizePurposeIDs(append(current.PurposeIDs, object.PurposeIDs...))
		if current.Description == "" {
			current.Description = object.Description
		}
		return
	}
	builder.bySignature[object.Signature] = len(builder.objects)
	builder.objects = append(builder.objects, object)
}

func (builder *purposePlanBuilder) applyFrequencyConflictPolicy(requested PurposeOutputObject, existing PurposeOutputObject) PurposeOutputObject {
	policy := strings.ToLower(strings.TrimSpace(builder.request.FrequencyPolicy))
	switch policy {
	case PurposeFrequencyPolicyPreserve:
		preserved := existing
		preserved.PurposeIDs = requested.PurposeIDs
		preserved.Description = requested.Description
		preserved.State = PurposeOutputStateExisting
		preserved.Reason = "Existing output frequency preserved"
		builder.warn("info", "frequency_preserved", fmt.Sprintf("%s is reused with existing %s frequency.", purposeOutputLabel(existing), existing.ReportingFrequency), firstPurposeID(requested.PurposeIDs), existing.Signature)
		return preserved
	case PurposeFrequencyPolicyHighestResolution:
		if purposeFrequencyRank(existing.ReportingFrequency) >= purposeFrequencyRank(requested.ReportingFrequency) {
			preserved := existing
			preserved.PurposeIDs = requested.PurposeIDs
			preserved.Description = requested.Description
			preserved.State = PurposeOutputStateExisting
			preserved.Reason = "Existing higher-resolution output reused"
			builder.warn("info", "frequency_existing_higher_resolution", fmt.Sprintf("%s already exists at %s frequency.", purposeOutputLabel(existing), existing.ReportingFrequency), firstPurposeID(requested.PurposeIDs), existing.Signature)
			return preserved
		}
		requested.State = purposeTemporaryState(builder.request)
		builder.warn("warning", "frequency_promoted", fmt.Sprintf("%s exists at %s frequency; adding %s for the selected purpose.", purposeOutputLabel(existing), existing.ReportingFrequency, requested.ReportingFrequency), firstPurposeID(requested.PurposeIDs), requested.Signature)
		return requested
	default:
		requested.State = PurposeOutputStateConflict
		builder.warn("warning", "frequency_conflict", fmt.Sprintf("%s already exists with %s frequency.", purposeOutputLabel(existing), existing.ReportingFrequency), firstPurposeID(requested.PurposeIDs), requested.Signature)
		return requested
	}
}

func (builder *purposePlanBuilder) warn(severity string, code string, message string, purposeID SimulationPurposeID, signature string) {
	builder.warnings = append(builder.warnings, PurposeRunWarning{
		Severity:        severity,
		Code:            code,
		Message:         message,
		PurposeID:       purposeID,
		OutputSignature: signature,
	})
}

func (builder *purposePlanBuilder) plan() PurposeRunPlan {
	sort.SliceStable(builder.objects, func(i, j int) bool {
		left := builder.objects[i]
		right := builder.objects[j]
		if purposeObjectOrder(left.ObjectType) != purposeObjectOrder(right.ObjectType) {
			return purposeObjectOrder(left.ObjectType) < purposeObjectOrder(right.ObjectType)
		}
		if left.KeyValue != right.KeyValue {
			return strings.ToLower(left.KeyValue) < strings.ToLower(right.KeyValue)
		}
		if left.VariableName != right.VariableName {
			return strings.ToLower(left.VariableName) < strings.ToLower(right.VariableName)
		}
		return strings.ToLower(left.Signature) < strings.ToLower(right.Signature)
	})
	seriesCount := 0
	frameCount := 1
	for _, object := range builder.objects {
		if !purposeObjectIsSeries(object.ObjectType) {
			continue
		}
		seriesCount++
		frameCount = maxInt(frameCount, purposeFrequencyFrames(object.ReportingFrequency, builder.runPeriodDays, builder.timestepsPerHour))
	}
	weight := planWeight(seriesCount, frameCount)
	return PurposeRunPlan{
		Purposes:          append([]SimulationPurposeID(nil), builder.request.Purposes...),
		OutputObjects:     builder.objects,
		EstimatedWeight:   weight,
		EstimatedSeries:   seriesCount,
		EstimatedFrames:   frameCount,
		RequiresSQL:       true,
		RequiresDiscovery: builder.request.DiscoveryAllowed,
		Warnings:          builder.warnings,
	}
}

func collectExistingPurposeOutputs(doc idf.Document) map[string]PurposeOutputObject {
	report := idf.AnalyzeOutput(doc)
	out := map[string]PurposeOutputObject{}
	for _, item := range report.Existing {
		fields := make([]idf.OutputFieldValue, 0, len(item.Fields))
		for _, field := range item.Fields {
			fields = append(fields, idf.OutputFieldValue{Name: field.Name, Value: field.Value})
		}
		signature := PurposeOutputSignature(item.ObjectType, fields)
		if _, ok := out[signature]; ok {
			continue
		}
		out[signature] = PurposeOutputObject{
			ObjectType:         item.ObjectType,
			Fields:             fields,
			State:              PurposeOutputStateExisting,
			Weight:             purposeWeight(item.ObjectType, fields),
			Signature:          signature,
			KeyValue:           item.KeyValue,
			VariableName:       item.VariableName,
			ReportingFrequency: item.ReportingFrequency,
			Reason:             "Already in IDF",
		}
	}
	return out
}

func normalizePurposeOutputFields(objectType string, fields []idf.OutputFieldValue) []idf.OutputFieldValue {
	out := make([]idf.OutputFieldValue, 0, len(fields))
	for index, field := range fields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			name = fmt.Sprintf("Field %d", index+1)
		}
		value := strings.TrimSpace(field.Value)
		if normalizePurposeFieldName(name) == "reporting frequency" {
			value = canonicalPurposeFrequency(value)
		}
		out = append(out, idf.OutputFieldValue{Name: name, Value: value})
	}
	return out
}

func purposeOutputBaseSignature(objectType string, fields []idf.OutputFieldValue) string {
	parts := []string{normalizePurposeToken(objectType)}
	for _, field := range fields {
		name := normalizePurposeFieldName(field.Name)
		if name == "reporting frequency" {
			continue
		}
		parts = append(parts, name+"="+normalizePurposeToken(field.Value))
	}
	return strings.Join(parts, "|")
}

func purposeOutputKeyValue(fields []idf.OutputFieldValue) string {
	if value := purposeFieldValue(fields, "Key Value"); value != "" {
		return value
	}
	return purposeFieldValue(fields, "Key Name")
}

func purposeOutputVariableName(fields []idf.OutputFieldValue) string {
	return purposeFieldValue(fields, "Variable Name")
}

func purposeOutputFrequency(objectType string, fields []idf.OutputFieldValue) string {
	if !purposeObjectIsSeries(objectType) {
		return ""
	}
	return canonicalPurposeFrequency(purposeFieldValue(fields, "Reporting Frequency"))
}

func purposeFieldValue(fields []idf.OutputFieldValue, names ...string) string {
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[normalizePurposeFieldName(name)] = true
	}
	for _, field := range fields {
		if wanted[normalizePurposeFieldName(field.Name)] {
			return strings.TrimSpace(field.Value)
		}
	}
	return ""
}

func canonicalPurposeFrequency(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Hourly"
	}
	for _, candidate := range []string{"Detailed", "Timestep", "Hourly", "Daily", "Monthly", "RunPeriod", "Annual"} {
		if strings.EqualFold(value, candidate) {
			return candidate
		}
	}
	return value
}

func purposeWeight(objectType string, fields []idf.OutputFieldValue) string {
	if !purposeObjectIsSeries(objectType) {
		return "light"
	}
	switch strings.ToLower(canonicalPurposeFrequency(purposeFieldValue(fields, "Reporting Frequency"))) {
	case "detailed", "timestep":
		return "heavy"
	case "hourly":
		return "medium"
	default:
		return "light"
	}
}

func purposeTemporaryState(request SimulationPurposeRequest) string {
	if request.PersistOutputs {
		return PurposeOutputStateWillPersist
	}
	return PurposeOutputStateTemporary
}

func purposeFrequencyRank(frequency string) int {
	switch strings.ToLower(canonicalPurposeFrequency(frequency)) {
	case "detailed":
		return 6
	case "timestep":
		return 5
	case "hourly":
		return 4
	case "daily":
		return 3
	case "monthly":
		return 2
	case "runperiod", "annual":
		return 1
	default:
		return 0
	}
}

func purposeFrequencyFrames(frequency string, runPeriodDays int, timestepsPerHour int) int {
	days := maxInt(runPeriodDays, 1)
	timesteps := maxInt(timestepsPerHour, 1)
	switch strings.ToLower(canonicalPurposeFrequency(frequency)) {
	case "detailed", "timestep":
		return days * 24 * timesteps
	case "hourly":
		return days * 24
	case "daily":
		return days
	case "monthly":
		return maxInt(1, minInt(12, (days+30)/31))
	case "runperiod", "annual":
		return 1
	default:
		return days * 24
	}
}

func purposeObjectIsSeries(objectType string) bool {
	switch strings.ToLower(strings.TrimSpace(objectType)) {
	case "output:variable", "output:meter", "output:meter:meterfileonly", "output:meter:cumulative", "output:meter:cumulativemeterfileonly":
		return true
	default:
		return false
	}
}

func purposeObjectOrder(objectType string) int {
	switch strings.ToLower(strings.TrimSpace(objectType)) {
	case "output:sqlite":
		return 0
	case "output:variabledictionary", "output:table:summaryreports", "outputcontrol:table:style", "output:diagnostics":
		return 1
	case "output:meter", "output:meter:meterfileonly", "output:meter:cumulative", "output:meter:cumulativemeterfileonly":
		return 2
	case "output:variable":
		return 3
	default:
		return 4
	}
}

func planWeight(seriesCount int, frameCount int) string {
	load := seriesCount * maxInt(frameCount, 1)
	switch {
	case load >= 600000:
		return "Very Heavy"
	case load >= 150000:
		return "Heavy"
	case load >= 40000:
		return "Medium"
	default:
		return "Light"
	}
}

func buildEnergyDashboardResult(series []SimulationSeries) EnergyDashboardResult {
	var result EnergyDashboardResult
	for _, item := range series {
		name := item.Column
		total := seriesTotal(item)
		energy := EnergySeries{
			Name:   name,
			Unit:   unitFromSeriesColumn(name),
			Source: item.File,
			Points: append([]SimulationPoint(nil), item.Points...),
			Total:  total,
		}
		zoneName, zoneMetric, isZone := splitZoneEnergySeriesName(name)
		switch {
		case isZone:
			result.ZoneMonthly = append(result.ZoneMonthly, ZoneEnergySeries{
				ZoneName: zoneName,
				Metric:   zoneMetric,
				Unit:     energy.Unit,
				Source:   energy.Source,
				Points:   energy.Points,
				Total:    total,
			})
		case strings.Contains(strings.ToLower(name), ":facility"):
			result.FacilityMonthly = append(result.FacilityMonthly, energy)
		case strings.Contains(strings.ToLower(name), "electricity:") || strings.Contains(strings.ToLower(name), "naturalgas:"):
			result.EndUseMonthly = append(result.EndUseMonthly, energy)
		default:
			continue
		}
		result.Totals = append(result.Totals, EnergyTotal{Name: name, Unit: energy.Unit, Source: energy.Source, Value: total})
	}
	return result
}

func seriesTotal(series SimulationSeries) float64 {
	total := 0.0
	for _, point := range series.Points {
		total += point.Value
	}
	return total
}

func splitZoneEnergySeriesName(value string) (string, string, bool) {
	before, after, ok := strings.Cut(value, ":")
	if !ok {
		return "", "", false
	}
	metric := strings.TrimSpace(after)
	if !strings.Contains(strings.ToLower(metric), "zone ") {
		return "", "", false
	}
	return strings.TrimSpace(before), metric, true
}

func splitPurposeSeriesColumn(value string) (string, string) {
	key, variable, ok := strings.Cut(value, ":")
	if !ok {
		return "", strings.TrimSpace(seriesColumnWithoutUnit(value))
	}
	return strings.TrimSpace(key), strings.TrimSpace(seriesColumnWithoutUnit(variable))
}

func seriesColumnWithoutUnit(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.LastIndex(value, "["); index > 0 {
		value = strings.TrimSpace(value[:index])
	}
	return value
}

func purposeSeriesMatchesVariables(column string, variables []string) bool {
	_, variableName := splitPurposeSeriesColumn(column)
	wanted := normalizePurposeToken(variableName)
	for _, variable := range variables {
		if wanted == normalizePurposeToken(variable) {
			return true
		}
	}
	return false
}

func unitFromSeriesColumn(value string) string {
	start := strings.LastIndex(value, "[")
	end := strings.LastIndex(value, "]")
	if start < 0 || end <= start {
		return ""
	}
	return strings.TrimSpace(value[start+1 : end])
}

func purposeCompleteness(purposeID SimulationPurposeID, requiredOutput string, found bool, source string) PurposeCompletenessItem {
	item := PurposeCompletenessItem{
		PurposeID:      purposeID,
		RequiredOutput: requiredOutput,
		Found:          found,
		Source:         source,
	}
	if !found {
		item.Message = "Required output was not found in the parsed result files."
	}
	return item
}

func seriesSource(series []SimulationSeries) string {
	if len(series) == 0 {
		return "missing"
	}
	return simulationSourceFromFilename(series[0].File)
}

func simulationSourceFromFilename(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".sql":
		return "sql"
	case ".csv":
		return "csv"
	case ".eso":
		return "eso"
	case ".err":
		return "err"
	default:
		return "unknown"
	}
}

func zoneHeatFlowVariableNames() []string {
	return []string{
		"Zone Air Heat Balance Internal Convective Heat Gain Rate",
		"Zone Air Heat Balance Surface Convection Rate",
		"Zone Air Heat Balance Interzone Air Transfer Rate",
		"Zone Air Heat Balance Outdoor Air Transfer Rate",
		"Zone Air Heat Balance System Air Transfer Rate",
		"Zone Air Heat Balance System Convective Heat Gain Rate",
		"Zone Air Heat Balance Air Energy Storage Rate",
		"Zone Air Heat Balance Deviation Rate",
	}
}

func comfortCheckVariables() []string {
	return []string{
		"Zone Mean Air Temperature",
		"Zone Thermostat Heating Setpoint Temperature",
		"Zone Thermostat Cooling Setpoint Temperature",
		"Zone Thermal Comfort Fanger Model PMV",
		"Zone Thermal Comfort Fanger Model PPD",
	}
}

func docHasObject(doc idf.Document, objectType string) bool {
	for _, obj := range doc.Objects {
		if strings.EqualFold(strings.TrimSpace(obj.Type), objectType) {
			return true
		}
	}
	return false
}

func estimatedRunPeriodDays(doc idf.Document) int {
	total := 0
	for _, obj := range doc.Objects {
		if !strings.EqualFold(strings.TrimSpace(obj.Type), "RunPeriod") {
			continue
		}
		beginMonth := purposeIntField(obj, 1, 1)
		beginDay := purposeIntField(obj, 2, 1)
		endMonth := purposeIntField(obj, 4, 12)
		endDay := purposeIntField(obj, 5, 31)
		start := dayOfYear(beginMonth, beginDay)
		end := dayOfYear(endMonth, endDay)
		if start <= 0 || end <= 0 {
			continue
		}
		days := end - start + 1
		if days <= 0 {
			days += 365
		}
		total += days
	}
	if total <= 0 {
		return 365
	}
	return total
}

func estimatedTimestepsPerHour(doc idf.Document) int {
	for _, obj := range doc.Objects {
		if !strings.EqualFold(strings.TrimSpace(obj.Type), "Timestep") {
			continue
		}
		return maxInt(1, purposeIntField(obj, 0, 4))
	}
	return 4
}

func purposeIntField(obj idf.Object, index int, fallback int) int {
	if index < 0 || index >= len(obj.Fields) {
		return fallback
	}
	value := strings.TrimSpace(obj.Fields[index].Value)
	if value == "" {
		return fallback
	}
	number := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return fallback
		}
		number = number*10 + int(r-'0')
	}
	if number <= 0 {
		return fallback
	}
	return number
}

func dayOfYear(month int, day int) int {
	daysByMonth := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if month < 1 || month > len(daysByMonth) {
		return 0
	}
	if day < 1 || day > daysByMonth[month-1] {
		return 0
	}
	total := day
	for index := 0; index < month-1; index++ {
		total += daysByMonth[index]
	}
	return total
}

func normalizePurposeIDs(values []SimulationPurposeID) []SimulationPurposeID {
	order := []SimulationPurposeID{
		SimulationPurposeBasicEnergy,
		SimulationPurposeZoneHeatFlow,
		SimulationPurposeHVACLoopCheck,
		SimulationPurposeIntegrity,
		SimulationPurposeComfort,
		SimulationPurposeCustomOutputs,
	}
	seen := map[SimulationPurposeID]bool{}
	for _, value := range values {
		value = SimulationPurposeID(strings.TrimSpace(string(value)))
		if value != "" {
			seen[value] = true
		}
	}
	out := make([]SimulationPurposeID, 0, len(seen))
	for _, value := range order {
		if seen[value] {
			out = append(out, value)
			delete(seen, value)
		}
	}
	extra := make([]string, 0, len(seen))
	for value := range seen {
		extra = append(extra, string(value))
	}
	sort.Strings(extra)
	for _, value := range extra {
		out = append(out, SimulationPurposeID(value))
	}
	return out
}

func normalizePurposeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func purposeNameSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		key := normalizePurposeToken(value)
		if key != "" {
			out[key] = true
		}
	}
	return out
}

func purposeSortedSet(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func appendUniquePurposeString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	key := normalizePurposeToken(value)
	for _, existing := range values {
		if normalizePurposeToken(existing) == key {
			return values
		}
	}
	return append(values, value)
}

func normalizePurposeFieldName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizePurposeToken(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func firstPurposeID(values []SimulationPurposeID) SimulationPurposeID {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func purposeOutputLabel(object PurposeOutputObject) string {
	if object.VariableName != "" {
		return strings.TrimSpace(fmt.Sprintf("%s %s / %s / %s", object.ObjectType, object.KeyValue, object.VariableName, object.ReportingFrequency))
	}
	if object.KeyValue != "" {
		return strings.TrimSpace(fmt.Sprintf("%s %s / %s", object.ObjectType, object.KeyValue, object.ReportingFrequency))
	}
	return object.ObjectType
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
