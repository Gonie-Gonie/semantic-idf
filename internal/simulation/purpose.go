package simulation

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/Gonie-Gonie/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
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
	PurposeAllocationPolicyDirectOnly       = "direct_only"
	PurposeAllocationPolicyByZoneLoadShare  = "by_zone_load_share"
	PurposeOutputStateExisting              = "existing"
	PurposeOutputStateTemporary             = "temporary"
	PurposeOutputStateWillPersist           = "will_be_persisted"
	PurposeOutputStateConflict              = "conflict"
	PurposeOutputApplyModeAddMissingOnly    = "add_missing_only"
	PurposeOutputApplyModeReplaceConflicts  = "replace_conflicting"
	PurposeOutputApplyModeKeepExistingAdd   = "keep_existing_and_add"
	PurposeOutputApplyModeRemovePurpose     = "remove_purpose_outputs"
)

type SimulationPurposeRequest struct {
	Purposes         []SimulationPurposeID  `json:"purposes"`
	Scope            SimulationPurposeScope `json:"scope"`
	FrequencyPolicy  string                 `json:"frequencyPolicy,omitempty"`
	SQLMode          string                 `json:"sqlMode,omitempty"`
	AllocationPolicy string                 `json:"allocationPolicy,omitempty"`
	PersistOutputs   bool                   `json:"persistOutputs,omitempty"`
	DiscoveryAllowed bool                   `json:"discoveryAllowed,omitempty"`
	OutputApplyMode  string                 `json:"outputApplyMode,omitempty"`
}

type SimulationPurposeScope struct {
	ZoneMode           string                `json:"zoneMode,omitempty"`
	ZoneNames          []string              `json:"zoneNames,omitempty"`
	PeriodMode         string                `json:"periodMode,omitempty"`
	PeriodStart        string                `json:"periodStart,omitempty"`
	PeriodEnd          string                `json:"periodEnd,omitempty"`
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
	AllocationPolicy  string                `json:"allocationPolicy,omitempty"`
	PeriodMode        string                `json:"periodMode,omitempty"`
	PeriodStart       string                `json:"periodStart,omitempty"`
	PeriodEnd         string                `json:"periodEnd,omitempty"`
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
	ObjectIndex        *int                   `json:"objectIndex,omitempty"`
}

type PurposeRunWarning struct {
	Severity        string              `json:"severity"`
	Code            string              `json:"code"`
	Message         string              `json:"message"`
	PurposeID       SimulationPurposeID `json:"purposeId,omitempty"`
	OutputSignature string              `json:"outputSignature,omitempty"`
}

type PurposeResultBundle struct {
	Energy                   EnergyDashboardResult     `json:"energy,omitempty"`
	EnergyExplanation        EnergyExplanationResult   `json:"energyExplanation,omitempty"`
	EnergyExplanationSummary EnergyExplanationSummary  `json:"energyExplanationSummary,omitempty"`
	ZoneHeatFlow             HeatFlowDataset           `json:"zoneHeatFlow,omitempty"`
	HVACLoops                []HVACLoopRunResult       `json:"hvacLoops,omitempty"`
	Comfort                  ComfortResult             `json:"comfort,omitempty"`
	Integrity                IntegrityResult           `json:"integrity,omitempty"`
	Series                   []SimulationSeries        `json:"series,omitempty"`
	Completeness             []PurposeCompletenessItem `json:"completeness,omitempty"`
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
	Name           string                    `json:"name"`
	LoopType       string                    `json:"loopType,omitempty"`
	Status         string                    `json:"status,omitempty"`
	StatusMessage  string                    `json:"statusMessage,omitempty"`
	Series         []SimulationSeries        `json:"series,omitempty"`
	NodeSummaries  []HVACNodeRunSummary      `json:"nodeSummaries,omitempty"`
	Components     []HVACComponentRunSummary `json:"components,omitempty"`
	DerivedMetrics []HVACLoopDerivedMetric   `json:"derivedMetrics,omitempty"`
	Alerts         []HVACLoopAlert           `json:"alerts,omitempty"`
	Completeness   []PurposeCompletenessItem `json:"completeness,omitempty"`
}

type HVACNodeRunSummary struct {
	NodeName                   string  `json:"nodeName"`
	Source                     string  `json:"source,omitempty"`
	SeriesCount                int     `json:"seriesCount"`
	HasTemperature             bool    `json:"hasTemperature"`
	TemperatureAverage         float64 `json:"temperatureAverage"`
	TemperatureUnit            string  `json:"temperatureUnit,omitempty"`
	HasMassFlow                bool    `json:"hasMassFlow"`
	MassFlowAverage            float64 `json:"massFlowAverage"`
	MassFlowMax                float64 `json:"massFlowMax"`
	MassFlowUnit               string  `json:"massFlowUnit,omitempty"`
	HasSetpoint                bool    `json:"hasSetpoint"`
	SetpointAverage            float64 `json:"setpointAverage"`
	SetpointUnit               string  `json:"setpointUnit,omitempty"`
	TemperatureSetpointDelta   float64 `json:"temperatureSetpointDelta"`
	ActiveMassFlowFraction     float64 `json:"activeMassFlowFraction"`
	TemperatureSetpointSamples int     `json:"temperatureSetpointSamples,omitempty"`
}

type HVACLoopDerivedMetric struct {
	Name    string  `json:"name"`
	Unit    string  `json:"unit,omitempty"`
	Value   float64 `json:"value"`
	Source  string  `json:"source,omitempty"`
	Status  string  `json:"status,omitempty"`
	Message string  `json:"message,omitempty"`
}

type HVACLoopAlert struct {
	Severity string   `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	NodeName string   `json:"nodeName,omitempty"`
	Source   string   `json:"source,omitempty"`
	Value    *float64 `json:"value,omitempty"`
	Unit     string   `json:"unit,omitempty"`
}

type HVACComponentRunSummary struct {
	ComponentName string                         `json:"componentName"`
	ComponentType string                         `json:"componentType,omitempty"`
	Source        string                         `json:"source,omitempty"`
	SeriesCount   int                            `json:"seriesCount"`
	Metrics       []HVACComponentOperationMetric `json:"metrics,omitempty"`
	Series        []SimulationSeries             `json:"series,omitempty"`
}

type HVACComponentOperationMetric struct {
	Name       string  `json:"name"`
	Unit       string  `json:"unit,omitempty"`
	Source     string  `json:"source,omitempty"`
	Min        float64 `json:"min"`
	Max        float64 `json:"max"`
	Average    float64 `json:"average"`
	Total      float64 `json:"total"`
	PointCount int     `json:"pointCount"`
}

type ComfortResult struct {
	PeriodScope  string                    `json:"periodScope,omitempty"`
	Zones        []ComfortZoneResult       `json:"zones,omitempty"`
	Series       []SimulationSeries        `json:"series,omitempty"`
	Issues       []ComfortIssueRank        `json:"issues,omitempty"`
	UnmetHours   []ComfortUnmetSummary     `json:"unmetHours,omitempty"`
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

type ComfortIssueRank struct {
	ZoneName         string  `json:"zoneName"`
	Source           string  `json:"source,omitempty"`
	UnmetSamples     int     `json:"unmetSamples"`
	HeatingSamples   int     `json:"heatingSamples,omitempty"`
	CoolingSamples   int     `json:"coolingSamples,omitempty"`
	TotalSamples     int     `json:"totalSamples,omitempty"`
	MaxDeviation     float64 `json:"maxDeviation"`
	AverageDeviation float64 `json:"averageDeviation"`
	Unit             string  `json:"unit,omitempty"`
	PeakLabel        string  `json:"peakLabel,omitempty"`
}

type ComfortUnmetSummary struct {
	ZoneName  string  `json:"zoneName"`
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	ValueText string  `json:"valueText,omitempty"`
	Unit      string  `json:"unit,omitempty"`
	Report    string  `json:"report,omitempty"`
	Table     string  `json:"table,omitempty"`
	Source    string  `json:"source,omitempty"`
}

type IntegrityResult struct {
	Status            string                   `json:"status"`
	ERR               ERRSummary               `json:"err"`
	Files             []SimulationFileInfo     `json:"files,omitempty"`
	Completed         bool                     `json:"completed"`
	StaticDiagnostics []idf.Diagnostic         `json:"staticDiagnostics,omitempty"`
	CrossChecks       []IntegrityCrossCheck    `json:"crossChecks,omitempty"`
	SQLIssues         []IntegritySQLIssue      `json:"sqlIssues,omitempty"`
	TabularReports    []IntegrityTabularReport `json:"tabularReports,omitempty"`
}

type IntegritySQLIssue struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Count    int    `json:"count,omitempty"`
	Source   string `json:"source,omitempty"`
}

type IntegrityCrossCheck struct {
	Category     string            `json:"category"`
	Name         string            `json:"name"`
	Status       string            `json:"status"`
	StaticSource string            `json:"staticSource,omitempty"`
	SQLSource    string            `json:"sqlSource,omitempty"`
	SQLReport    string            `json:"sqlReport,omitempty"`
	SQLTable     string            `json:"sqlTable,omitempty"`
	Message      string            `json:"message,omitempty"`
	Values       map[string]string `json:"values,omitempty"`
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
	Status         string              `json:"status,omitempty"`
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
	normalized.AllocationPolicy = normalizePurposeAllocationPolicy(normalized.AllocationPolicy)
	normalized.OutputApplyMode = normalizePurposeOutputApplyMode(normalized.OutputApplyMode)
	normalized.Scope.ZoneMode = strings.TrimSpace(normalized.Scope.ZoneMode)
	if normalized.Scope.ZoneMode == "" {
		normalized.Scope.ZoneMode = "all"
	}
	normalized.Scope.LoopMode = strings.TrimSpace(normalized.Scope.LoopMode)
	if normalized.Scope.LoopMode == "" {
		normalized.Scope.LoopMode = "all"
	}
	normalized.Scope.ZoneNames = normalizePurposeStrings(normalized.Scope.ZoneNames)
	normalized.Scope.PeriodMode = strings.TrimSpace(normalized.Scope.PeriodMode)
	if normalized.Scope.PeriodMode == "" {
		normalized.Scope.PeriodMode = "full"
	}
	normalized.Scope.PeriodStart = strings.TrimSpace(normalized.Scope.PeriodStart)
	normalized.Scope.PeriodEnd = strings.TrimSpace(normalized.Scope.PeriodEnd)
	normalized.Scope.AirLoopNames = normalizePurposeStrings(normalized.Scope.AirLoopNames)
	normalized.Scope.PlantLoopNames = normalizePurposeStrings(normalized.Scope.PlantLoopNames)
	normalized.Scope.CondenserLoopNames = normalizePurposeStrings(normalized.Scope.CondenserLoopNames)
	normalized.Scope.ComponentIDs = normalizePurposeStrings(normalized.Scope.ComponentIDs)
	normalized.Scope.OutputSignatures = normalizePurposeStrings(normalized.Scope.OutputSignatures)
	return normalized
}

func normalizePurposeAllocationPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case PurposeAllocationPolicyDirectOnly, "", "none":
		return PurposeAllocationPolicyDirectOnly
	case PurposeAllocationPolicyByZoneLoadShare, "zone_load_share":
		return PurposeAllocationPolicyByZoneLoadShare
	default:
		return PurposeAllocationPolicyDirectOnly
	}
}

func normalizePurposeOutputApplyMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case PurposeOutputApplyModeReplaceConflicts:
		return PurposeOutputApplyModeReplaceConflicts
	case PurposeOutputApplyModeKeepExistingAdd:
		return PurposeOutputApplyModeKeepExistingAdd
	case PurposeOutputApplyModeRemovePurpose:
		return PurposeOutputApplyModeRemovePurpose
	default:
		return PurposeOutputApplyModeAddMissingOnly
	}
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
	if request.DiscoveryAllowed {
		builder.addDiscoveryDictionaryOutputs()
	}
	return builder.plan()
}

func PurposeRunPlanApplyRequest(plan PurposeRunPlan, applyModes ...string) idf.OutputApplyRequest {
	mode := PurposeOutputApplyModeAddMissingOnly
	if len(applyModes) > 0 {
		mode = normalizePurposeOutputApplyMode(applyModes[0])
	}
	additions := make([]idf.OutputObjectRequest, 0, len(plan.OutputObjects))
	updates := []idf.OutputFieldUpdate{}
	removeIndexes := []int{}
	removed := map[int]bool{}
	for _, object := range plan.OutputObjects {
		switch mode {
		case PurposeOutputApplyModeRemovePurpose:
			if object.ObjectIndex != nil && !removed[*object.ObjectIndex] {
				removeIndexes = append(removeIndexes, *object.ObjectIndex)
				removed[*object.ObjectIndex] = true
			}
			continue
		case PurposeOutputApplyModeReplaceConflicts:
			if object.State == PurposeOutputStateConflict && object.ObjectIndex != nil {
				if fieldIndex, ok := purposeReportingFrequencyFieldIndex(object.Fields); ok {
					updates = append(updates, idf.OutputFieldUpdate{
						ObjectIndex: *object.ObjectIndex,
						FieldIndex:  fieldIndex,
						Value:       object.ReportingFrequency,
					})
				}
				continue
			}
			if object.State == PurposeOutputStateExisting || object.State == PurposeOutputStateConflict {
				continue
			}
		case PurposeOutputApplyModeKeepExistingAdd:
			if object.State == PurposeOutputStateExisting {
				continue
			}
		default:
			if object.State == PurposeOutputStateExisting || object.State == PurposeOutputStateConflict {
				continue
			}
		}
		if object.State == PurposeOutputStateExisting {
			continue
		}
		additions = append(additions, idf.OutputObjectRequest{
			ObjectType: object.ObjectType,
			Fields:     object.Fields,
			Reason:     object.Reason,
		})
	}
	sort.Ints(removeIndexes)
	return idf.OutputApplyRequest{AddObjects: additions, Updates: updates, RemoveObjectIndexes: removeIndexes}
}

func purposeReportingFrequencyFieldIndex(fields []idf.OutputFieldValue) (int, bool) {
	for index, field := range fields {
		if normalizePurposeFieldName(field.Name) == "reporting frequency" {
			return index, true
		}
	}
	return -1, false
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
			plan := purposeRunPlanWithRequestScope(result.PurposeRunPlan, request)
			bundle.Energy = buildEnergyDashboardResultFromFiles(result.Files)
			if len(bundle.Energy.FacilityMonthly)+len(bundle.Energy.EndUseMonthly)+len(bundle.Energy.ZoneMonthly) == 0 {
				bundle.Energy = buildEnergyDashboardResult(result.Series)
			}
			bundle.Energy.Completeness = energyDashboardCompleteness(bundle.Energy, result.PurposeRunPlan, result.Series)
			bundle.EnergyExplanation = buildEnergyExplanationResultFromFiles(result.Files, bundle.Energy, plan)
			bundle.EnergyExplanation = enrichEnergyExplanationWithServicePaths(bundle.EnergyExplanation, result.InputPath)
			bundle.EnergyExplanationSummary = buildEnergyExplanationSummary(bundle.EnergyExplanation)
			bundle.Completeness = append(bundle.Completeness, bundle.Energy.Completeness...)
		case SimulationPurposeZoneHeatFlow:
			bundle.ZoneHeatFlow = result.HeatFlow
			bundle.ZoneHeatFlow.Completeness = zoneHeatFlowCompleteness(result.HeatFlow)
			bundle.Completeness = append(bundle.Completeness, bundle.ZoneHeatFlow.Completeness...)
		case SimulationPurposeHVACLoopCheck:
			bundle.HVACLoops = buildHVACLoopRunResults(result.Series, request)
			hasNodeSeries := len(bundle.HVACLoops) > 0 && len(bundle.HVACLoops[0].Series) > 0
			hasComponentSeries := len(bundle.HVACLoops) > 0 && hvacComponentSeriesCount(bundle.HVACLoops[0].Components) > 0
			bundle.Completeness = append(bundle.Completeness, purposeCompleteness(
				SimulationPurposeHVACLoopCheck,
				"HVAC node state series",
				hasNodeSeries,
				hvacLoopResultSource(bundle.HVACLoops),
			))
			bundle.Completeness = append(bundle.Completeness, purposeCompleteness(
				SimulationPurposeHVACLoopCheck,
				"HVAC component operation series",
				hasComponentSeries,
				hvacLoopResultSource(bundle.HVACLoops),
			))
		case SimulationPurposeComfort:
			bundle.Comfort = buildComfortResult(result.Series, request.Scope)
			bundle.Comfort.UnmetHours = buildComfortUnmetSummariesFromFiles(result.Files)
			bundle.Completeness = append(bundle.Completeness, bundle.Comfort.Completeness...)
		case SimulationPurposeIntegrity:
			sqlIntegrity := buildIntegritySQLResultFromFiles(result.Files)
			bundle.Integrity = IntegrityResult{
				Status:            result.Status,
				ERR:               result.ERR,
				Files:             append([]SimulationFileInfo(nil), result.Files...),
				Completed:         result.ERR.Completed,
				StaticDiagnostics: buildIntegrityStaticDiagnostics(result.InputPath),
				CrossChecks:       buildIntegrityCrossChecks(result.InputPath, sqlIntegrity.TabularReports),
				SQLIssues:         sqlIntegrity.Issues,
				TabularReports:    sqlIntegrity.TabularReports,
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

func purposeRunPlanWithRequestScope(plan *PurposeRunPlan, request SimulationPurposeRequest) *PurposeRunPlan {
	var copy PurposeRunPlan
	if plan != nil {
		copy = *plan
	}
	copy.AllocationPolicy = request.AllocationPolicy
	copy.PeriodMode = request.Scope.PeriodMode
	copy.PeriodStart = request.Scope.PeriodStart
	copy.PeriodEnd = request.Scope.PeriodEnd
	return &copy
}

type energyServicePathIndex struct {
	byService     map[string][]string
	byZone        map[string][]string
	byZoneService map[string][]string
	byLoopService map[string][]string
}

func enrichEnergyExplanationWithServicePaths(explanation EnergyExplanationResult, inputPath string) EnergyExplanationResult {
	index := buildEnergyServicePathIndex(inputPath)
	if len(index.byService) == 0 && len(index.byZone) == 0 && len(index.byLoopService) == 0 {
		return explanation
	}
	explanation.Nodes = enrichEnergyExplanationNodesWithServicePaths(explanation.Nodes, index)
	nodePaths := energyExplanationNodePathMap(explanation.Nodes)
	explanation.Edges = enrichEnergyExplanationEdgesWithServicePaths(explanation.Edges, nodePaths, index)
	for periodIndex := range explanation.Periods {
		explanation.Periods[periodIndex].Nodes = enrichEnergyExplanationNodesWithServicePaths(explanation.Periods[periodIndex].Nodes, index)
		periodNodePaths := energyExplanationNodePathMap(explanation.Periods[periodIndex].Nodes)
		explanation.Periods[periodIndex].Edges = enrichEnergyExplanationEdgesWithServicePaths(explanation.Periods[periodIndex].Edges, periodNodePaths, index)
	}
	return explanation
}

func buildEnergyServicePathIndex(inputPath string) energyServicePathIndex {
	index := energyServicePathIndex{
		byService:     map[string][]string{},
		byZone:        map[string][]string{},
		byZoneService: map[string][]string{},
		byLoopService: map[string][]string{},
	}
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		return index
	}
	content, err := os.ReadFile(inputPath)
	if err != nil {
		return index
	}
	model, err := epinput.Parse(inputPath, content)
	if err != nil {
		return index
	}
	report := idf.AnalyzeHVAC(epinput.ToIDFDocument(model))
	for _, summary := range report.ServiceModel.ZoneServices {
		for _, path := range summary.Paths {
			pathID := strings.TrimSpace(path.ID)
			service := energyCanonicalServiceKind(path.ServiceKind)
			if pathID == "" {
				continue
			}
			if service != "" {
				index.byService[service] = appendUniqueStrings(index.byService[service], pathID)
			}
			zoneKey := normalizePurposeToken(firstNonEmpty(path.ZoneName, path.ServedSubject.ZoneName, summary.ZoneName))
			if zoneKey != "" {
				index.byZone[zoneKey] = appendUniqueStrings(index.byZone[zoneKey], pathID)
				if service != "" {
					index.byZoneService[zoneKey+"|"+service] = appendUniqueStrings(index.byZoneService[zoneKey+"|"+service], pathID)
				}
			}
			for _, loopName := range energyServicePathLoopNames(path) {
				loopKey := normalizePurposeToken(loopName)
				if loopKey != "" && service != "" {
					index.byLoopService[loopKey+"|"+service] = appendUniqueStrings(index.byLoopService[loopKey+"|"+service], pathID)
				}
			}
		}
	}
	return index
}

func energyServicePathLoopNames(path idf.ZoneServicePath) []string {
	out := []string{}
	if path.PlantLoop != nil {
		out = appendUniqueStrings(out, path.PlantLoop.Name)
	}
	if path.AirLoop != nil {
		out = appendUniqueStrings(out, path.AirLoop.Name)
	}
	if path.CondenserLoop != nil {
		out = appendUniqueStrings(out, path.CondenserLoop.Name)
	}
	if path.SourceSystem != nil {
		out = appendUniqueStrings(out, path.SourceSystem.Name)
	}
	if path.RefrigerantSystem != nil {
		out = appendUniqueStrings(out, path.RefrigerantSystem.Name)
	}
	return out
}

func enrichEnergyExplanationNodesWithServicePaths(nodes []EnergyExplanationNode, index energyServicePathIndex) []EnergyExplanationNode {
	out := append([]EnergyExplanationNode(nil), nodes...)
	for nodeIndex := range out {
		out[nodeIndex].RelatedPathIDs = appendUniqueStrings(out[nodeIndex].RelatedPathIDs, relatedEnergyServicePathIDs(out[nodeIndex], index)...)
	}
	return out
}

func enrichEnergyExplanationEdgesWithServicePaths(edges []EnergyExplanationEdge, nodePaths map[string][]string, index energyServicePathIndex) []EnergyExplanationEdge {
	out := append([]EnergyExplanationEdge(nil), edges...)
	for edgeIndex := range out {
		out[edgeIndex].RelatedPathIDs = appendUniqueStrings(out[edgeIndex].RelatedPathIDs, nodePaths[out[edgeIndex].FromID]...)
		out[edgeIndex].RelatedPathIDs = appendUniqueStrings(out[edgeIndex].RelatedPathIDs, nodePaths[out[edgeIndex].ToID]...)
		if out[edgeIndex].ZoneName != "" || out[edgeIndex].ServiceKind != "" {
			out[edgeIndex].RelatedPathIDs = appendUniqueStrings(out[edgeIndex].RelatedPathIDs, relatedEnergyServicePathIDs(EnergyExplanationNode{
				ZoneName:    out[edgeIndex].ZoneName,
				ServiceKind: out[edgeIndex].ServiceKind,
			}, index)...)
		}
	}
	return out
}

func energyExplanationNodePathMap(nodes []EnergyExplanationNode) map[string][]string {
	out := map[string][]string{}
	for _, node := range nodes {
		if node.ID == "" || len(node.RelatedPathIDs) == 0 {
			continue
		}
		out[node.ID] = appendUniqueStrings(out[node.ID], node.RelatedPathIDs...)
	}
	return out
}

func relatedEnergyServicePathIDs(node EnergyExplanationNode, index energyServicePathIndex) []string {
	service := energyCanonicalServiceKind(firstNonEmpty(node.ServiceKind, node.EndUse))
	zoneKey := normalizePurposeToken(node.ZoneName)
	loopKey := normalizePurposeToken(node.LoopName)
	out := []string{}
	if zoneKey != "" && service != "" {
		out = appendUniqueStrings(out, index.byZoneService[zoneKey+"|"+service]...)
	}
	if loopKey != "" && service != "" {
		out = appendUniqueStrings(out, index.byLoopService[loopKey+"|"+service]...)
	}
	if len(out) == 0 && zoneKey != "" {
		out = appendUniqueStrings(out, index.byZone[zoneKey]...)
	}
	if len(out) == 0 && zoneKey == "" && loopKey == "" && service != "" && (node.Level == "load" || node.Level == "heat") {
		out = appendUniqueStrings(out, index.byService[service]...)
	}
	return out
}

func energyCanonicalServiceKind(serviceKind string) string {
	value := strings.ToLower(strings.TrimSpace(serviceKind))
	switch {
	case strings.Contains(value, "cooling") || value == "dehumidification":
		return "cooling"
	case strings.Contains(value, "heating") || value == "humidification":
		return "heating"
	case strings.Contains(value, "ventilation"):
		return "ventilation"
	default:
		return value
	}
}

func buildHVACLoopRunResults(series []SimulationSeries, request SimulationPurposeRequest) []HVACLoopRunResult {
	nodeSeries := []SimulationSeries{}
	componentSeries := []SimulationSeries{}
	foundVariables := map[string]bool{}
	for _, item := range series {
		switch {
		case purposeSeriesMatchesVariables(item.Column, hvacLoopCheckNodeVariables()):
			nodeSeries = append(nodeSeries, item)
			_, variableName := splitPurposeSeriesColumn(item.Column)
			if variableName != "" {
				foundVariables[normalizePurposeToken(variableName)] = true
			}
		case purposeSeriesMatchesVariables(item.Column, hvacLoopCheckComponentVariableNames()):
			componentSeries = append(componentSeries, item)
		}
	}
	if len(nodeSeries)+len(componentSeries) == 0 {
		return nil
	}
	result := HVACLoopRunResult{
		Name:          hvacLoopResultName(request.Scope),
		LoopType:      hvacLoopResultType(request.Scope),
		Series:        nodeSeries,
		NodeSummaries: buildHVACNodeRunSummaries(nodeSeries),
		Components:    buildHVACComponentRunSummaries(componentSeries),
	}
	result.Completeness = append(result.Completeness, hvacNodeSeriesCompleteness(foundVariables, seriesSource(nodeSeries))...)
	result.Completeness = append(result.Completeness, purposeCompleteness(
		SimulationPurposeHVACLoopCheck,
		"HVAC component operation series",
		len(componentSeries) > 0,
		seriesSource(componentSeries),
	))
	result.DerivedMetrics = buildHVACLoopDerivedMetrics(result.NodeSummaries, result.LoopType)
	result.Alerts = buildHVACLoopAlerts(result.NodeSummaries)
	result.Alerts = append(result.Alerts, buildHVACLoopDerivedAlerts(result.NodeSummaries)...)
	result.Status, result.StatusMessage = classifyHVACLoopStatus(result.NodeSummaries, result.DerivedMetrics, result.Alerts)
	return []HVACLoopRunResult{result}
}

type hvacNodeSeriesBucket struct {
	nodeName    string
	series      []SimulationSeries
	temperature *SimulationSeries
	massFlow    *SimulationSeries
	setpoint    *SimulationSeries
}

func buildHVACNodeRunSummaries(series []SimulationSeries) []HVACNodeRunSummary {
	buckets := map[string]*hvacNodeSeriesBucket{}
	order := []string{}
	for _, item := range series {
		nodeName, variableName := splitPurposeSeriesColumn(item.Column)
		if nodeName == "" {
			nodeName = "Unknown Node"
		}
		key := normalizePurposeToken(nodeName)
		bucket := buckets[key]
		if bucket == nil {
			bucket = &hvacNodeSeriesBucket{nodeName: nodeName}
			buckets[key] = bucket
			order = append(order, key)
		}
		bucket.series = append(bucket.series, item)
		itemCopy := item
		switch normalizePurposeToken(variableName) {
		case "system node temperature":
			bucket.temperature = &itemCopy
		case "system node mass flow rate":
			bucket.massFlow = &itemCopy
		case "system node setpoint temperature":
			bucket.setpoint = &itemCopy
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		return strings.ToLower(buckets[order[i]].nodeName) < strings.ToLower(buckets[order[j]].nodeName)
	})
	out := make([]HVACNodeRunSummary, 0, len(order))
	for _, key := range order {
		out = append(out, hvacNodeSummaryFromBucket(buckets[key]))
	}
	return out
}

func hvacNodeSummaryFromBucket(bucket *hvacNodeSeriesBucket) HVACNodeRunSummary {
	summary := HVACNodeRunSummary{
		NodeName:    bucket.nodeName,
		Source:      seriesSource(bucket.series),
		SeriesCount: len(bucket.series),
	}
	if bucket.temperature != nil {
		summary.HasTemperature = true
		summary.TemperatureAverage = roundedPurposeNumber(seriesDisplayAverage(*bucket.temperature))
		summary.TemperatureUnit = seriesDisplayUnit(*bucket.temperature)
	}
	if bucket.massFlow != nil {
		summary.HasMassFlow = true
		summary.MassFlowAverage = roundedPurposeNumber(seriesDisplayAverage(*bucket.massFlow))
		summary.MassFlowMax = roundedPurposeNumber(seriesDisplayMax(*bucket.massFlow))
		summary.MassFlowUnit = seriesDisplayUnit(*bucket.massFlow)
		summary.ActiveMassFlowFraction = roundedPurposeNumber(hvacActiveMassFlowFraction(bucket.massFlow.Points))
	}
	if bucket.setpoint != nil {
		summary.HasSetpoint = true
		summary.SetpointAverage = roundedPurposeNumber(seriesDisplayAverage(*bucket.setpoint))
		summary.SetpointUnit = seriesDisplayUnit(*bucket.setpoint)
	}
	if bucket.temperature != nil && bucket.setpoint != nil {
		delta, samples := hvacAverageAbsoluteDelta(bucket.temperature.Points, bucket.setpoint.Points)
		summary.TemperatureSetpointDelta = roundedPurposeNumber(delta)
		summary.TemperatureSetpointSamples = samples
	}
	return summary
}

type hvacComponentSeriesBucket struct {
	componentName string
	componentType string
	series        []SimulationSeries
}

func buildHVACComponentRunSummaries(series []SimulationSeries) []HVACComponentRunSummary {
	buckets := map[string]*hvacComponentSeriesBucket{}
	order := []string{}
	for _, item := range series {
		componentName, variableName := splitPurposeSeriesColumn(item.Column)
		if componentName == "" {
			componentName = "Unknown Component"
		}
		key := normalizePurposeToken(componentName)
		bucket := buckets[key]
		if bucket == nil {
			bucket = &hvacComponentSeriesBucket{componentName: componentName}
			buckets[key] = bucket
			order = append(order, key)
		}
		if componentType := hvacComponentOperationCategory(variableName); componentType != "" && bucket.componentType == "" {
			bucket.componentType = componentType
		}
		bucket.series = append(bucket.series, item)
	}
	sort.SliceStable(order, func(i, j int) bool {
		return strings.ToLower(buckets[order[i]].componentName) < strings.ToLower(buckets[order[j]].componentName)
	})
	out := make([]HVACComponentRunSummary, 0, len(order))
	for _, key := range order {
		bucket := buckets[key]
		metrics := make([]HVACComponentOperationMetric, 0, len(bucket.series))
		for _, item := range bucket.series {
			_, variableName := splitPurposeSeriesColumn(item.Column)
			metrics = append(metrics, HVACComponentOperationMetric{
				Name:       variableName,
				Unit:       seriesDisplayUnit(item),
				Source:     item.File,
				Min:        roundedPurposeNumber(seriesDisplayMin(item)),
				Max:        roundedPurposeNumber(seriesDisplayMax(item)),
				Average:    roundedPurposeNumber(seriesDisplayAverage(item)),
				Total:      roundedPurposeNumber(seriesDisplayTotal(item)),
				PointCount: len(seriesDisplayPoints(item)),
			})
		}
		sort.SliceStable(metrics, func(i, j int) bool {
			return strings.ToLower(metrics[i].Name) < strings.ToLower(metrics[j].Name)
		})
		out = append(out, HVACComponentRunSummary{
			ComponentName: bucket.componentName,
			ComponentType: bucket.componentType,
			Source:        seriesSource(bucket.series),
			SeriesCount:   len(bucket.series),
			Metrics:       metrics,
			Series:        append([]SimulationSeries(nil), bucket.series...),
		})
	}
	return out
}

func hvacComponentSeriesCount(components []HVACComponentRunSummary) int {
	total := 0
	for _, component := range components {
		total += len(component.Series)
	}
	return total
}

func buildHVACLoopDerivedMetrics(nodes []HVACNodeRunSummary, loopType string) []HVACLoopDerivedMetric {
	if len(nodes) == 0 {
		return nil
	}
	var metrics []HVACLoopDerivedMetric
	if average, ok := averageHVACNodeValue(nodes, func(node HVACNodeRunSummary) (float64, bool) {
		return node.TemperatureAverage, node.HasTemperature
	}); ok {
		metrics = append(metrics, HVACLoopDerivedMetric{Name: "Average node temperature", Unit: "C", Value: roundedPurposeNumber(average), Source: "series", Status: "info"})
	}
	if peak, ok := maxHVACNodeValue(nodes, func(node HVACNodeRunSummary) (float64, bool) {
		return node.MassFlowMax, node.HasMassFlow
	}); ok {
		metrics = append(metrics, HVACLoopDerivedMetric{Name: "Peak node mass flow", Unit: "kg/s", Value: roundedPurposeNumber(peak), Source: "series", Status: hvacFlowStatus(peak)})
	}
	if average, ok := averageHVACNodeValue(nodes, func(node HVACNodeRunSummary) (float64, bool) {
		return node.TemperatureSetpointDelta, node.TemperatureSetpointSamples > 0
	}); ok {
		metrics = append(metrics, HVACLoopDerivedMetric{Name: "Average temperature-setpoint delta", Unit: "C", Value: roundedPurposeNumber(average), Source: "series", Status: hvacSetpointDeltaStatus(average)})
	}
	if average, ok := averageHVACNodeValue(nodes, func(node HVACNodeRunSummary) (float64, bool) {
		return node.ActiveMassFlowFraction * 100, node.HasMassFlow
	}); ok {
		metrics = append(metrics, HVACLoopDerivedMetric{Name: "Average active-flow fraction", Unit: "%", Value: roundedPurposeNumber(average), Source: "series", Status: hvacActiveFlowStatus(average)})
	}
	if spread, ok := hvacNodeTemperatureSpread(nodes); ok {
		metrics = append(metrics, HVACLoopDerivedMetric{
			Name:    "Average node temperature spread",
			Unit:    "C",
			Value:   roundedPurposeNumber(spread),
			Source:  "derived_from_node_state",
			Status:  hvacTemperatureSpreadStatus(spread),
			Message: "Difference between warmest and coolest reported node average.",
		})
		if peakFlow, flowOK := maxHVACNodeValue(nodes, func(node HVACNodeRunSummary) (float64, bool) {
			return node.MassFlowMax, node.HasMassFlow
		}); flowOK {
			metricName, cp := hvacDerivedHeatTransferConfig(loopType)
			metrics = append(metrics, HVACLoopDerivedMetric{
				Name:    metricName,
				Unit:    "kW",
				Value:   roundedPurposeNumber(math.Abs(peakFlow * cp * spread)),
				Source:  "derived_from_node_state",
				Status:  "derived",
				Message: "Estimated from peak node mass flow, fluid heat capacity, and average node temperature spread.",
			})
		}
	}
	return metrics
}

func hvacDerivedHeatTransferConfig(loopType string) (string, float64) {
	normalized := strings.ToLower(loopType)
	if strings.Contains(normalized, "plant") || strings.Contains(normalized, "condenser") {
		return "Estimated water-side heat transfer", 4.186
	}
	return "Estimated air-side heat transfer", 1.006
}

func hvacNodeTemperatureSpread(nodes []HVACNodeRunSummary) (float64, bool) {
	minimum := math.Inf(1)
	maximum := math.Inf(-1)
	count := 0
	for _, node := range nodes {
		if !node.HasTemperature {
			continue
		}
		count++
		minimum = math.Min(minimum, node.TemperatureAverage)
		maximum = math.Max(maximum, node.TemperatureAverage)
	}
	if count < 2 || math.IsInf(minimum, 0) || math.IsInf(maximum, 0) {
		return 0, false
	}
	return math.Abs(maximum - minimum), true
}

func hvacTemperatureSpreadStatus(value float64) string {
	if value < 0.2 {
		return "warning"
	}
	return "ok"
}

func buildHVACLoopAlerts(nodes []HVACNodeRunSummary) []HVACLoopAlert {
	var alerts []HVACLoopAlert
	for _, node := range nodes {
		if node.HasMassFlow && node.MassFlowMax <= 0.001 {
			alerts = append(alerts, HVACLoopAlert{
				Severity: "warning",
				Code:     "no_detected_mass_flow",
				Message:  "Node mass flow stayed near zero during the parsed run.",
				NodeName: node.NodeName,
				Source:   node.Source,
				Value:    purposeFloat64(node.MassFlowMax),
				Unit:     node.MassFlowUnit,
			})
		}
		if node.HasTemperature && !node.HasSetpoint {
			alerts = append(alerts, HVACLoopAlert{
				Severity: "info",
				Code:     "missing_temperature_setpoint",
				Message:  "Temperature was reported without a matching setpoint series for this node.",
				NodeName: node.NodeName,
				Source:   node.Source,
			})
		}
		if node.TemperatureSetpointSamples > 0 && node.TemperatureSetpointDelta > 5 {
			alerts = append(alerts, HVACLoopAlert{
				Severity: "warning",
				Code:     "temperature_setpoint_delta",
				Message:  "Average node temperature drifted more than 5 C from setpoint.",
				NodeName: node.NodeName,
				Source:   node.Source,
				Value:    purposeFloat64(node.TemperatureSetpointDelta),
				Unit:     "C",
			})
		}
	}
	return alerts
}

func buildHVACLoopDerivedAlerts(nodes []HVACNodeRunSummary) []HVACLoopAlert {
	peakFlow, flowOK := maxHVACNodeValue(nodes, func(node HVACNodeRunSummary) (float64, bool) {
		return node.MassFlowMax, node.HasMassFlow
	})
	spread, spreadOK := hvacNodeTemperatureSpread(nodes)
	if !flowOK || !spreadOK || peakFlow <= 0.001 || spread >= 0.2 {
		return nil
	}
	return []HVACLoopAlert{{
		Severity: "warning",
		Code:     "flow_without_temperature_spread",
		Message:  "Node mass flow was detected, but reported node temperature spread stayed near zero.",
		Source:   "derived_from_node_state",
		Value:    purposeFloat64(roundedPurposeNumber(spread)),
		Unit:     "C",
	}}
}

func classifyHVACLoopStatus(nodes []HVACNodeRunSummary, metrics []HVACLoopDerivedMetric, alerts []HVACLoopAlert) (string, string) {
	peakFlow, flowOK := maxHVACNodeValue(nodes, func(node HVACNodeRunSummary) (float64, bool) {
		return node.MassFlowMax, node.HasMassFlow
	})
	spread, spreadOK := hvacNodeTemperatureSpread(nodes)
	setpointDelta, setpointOK := averageHVACNodeValue(nodes, func(node HVACNodeRunSummary) (float64, bool) {
		return node.TemperatureSetpointDelta, node.TemperatureSetpointSamples > 0
	})
	if hasHVACAlert(alerts, "temperature_setpoint_delta") || (setpointOK && setpointDelta > 5) {
		return "setpoint_not_met", "Average node temperature drifted more than 5 C from setpoint."
	}
	if !flowOK {
		return "unknown", "Mass-flow series were not available for loop status classification."
	}
	if peakFlow <= 0.001 {
		return "off", "No meaningful node mass flow was detected."
	}
	if spreadOK && spread < 0.2 {
		return "flow_no_load", "Mass flow was present, but node temperature spread stayed near zero."
	}
	if spreadOK && spread >= 0.2 {
		if hvacNodeNamesSuggest(nodes, "cool", "cold", "chw", "chilled") {
			return "active_cooling", "Flow and node temperature spread indicate active cooling transfer."
		}
		if hvacNodeNamesSuggest(nodes, "heat", "hot", "hw") {
			return "active_heating", "Flow and node temperature spread indicate active heat transfer."
		}
		if setpointOK && setpointDelta <= 2 {
			return "setpoint_tracking", "Flow and temperature spread are present while setpoints remain close."
		}
		return "suspicious", "Flow and temperature spread were detected, but heating/cooling role could not be inferred from node names."
	}
	if hasHVACDerivedMetric(metrics, "Estimated air-side heat transfer") || hasHVACDerivedMetric(metrics, "Estimated water-side heat transfer") {
		return "setpoint_tracking", "Derived heat-transfer metrics are available."
	}
	return "suspicious", "Loop status could not be classified from the available node summaries."
}

func hasHVACAlert(alerts []HVACLoopAlert, code string) bool {
	for _, alert := range alerts {
		if alert.Code == code {
			return true
		}
	}
	return false
}

func hasHVACDerivedMetric(metrics []HVACLoopDerivedMetric, name string) bool {
	for _, metric := range metrics {
		if metric.Name == name {
			return true
		}
	}
	return false
}

func hvacNodeNamesSuggest(nodes []HVACNodeRunSummary, tokens ...string) bool {
	for _, node := range nodes {
		name := normalizePurposeToken(node.NodeName)
		for _, token := range tokens {
			if strings.Contains(name, token) {
				return true
			}
		}
	}
	return false
}

func purposeFloat64(value float64) *float64 {
	return &value
}

func hvacActiveMassFlowFraction(points []SimulationPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	active := 0
	for _, point := range points {
		if math.Abs(point.Value) > 0.001 {
			active++
		}
	}
	return float64(active) / float64(len(points))
}

func hvacAverageAbsoluteDelta(left []SimulationPoint, right []SimulationPoint) (float64, int) {
	limit := minInt(len(left), len(right))
	if limit == 0 {
		return 0, 0
	}
	total := 0.0
	for index := 0; index < limit; index++ {
		total += math.Abs(left[index].Value - right[index].Value)
	}
	return total / float64(limit), limit
}

func averageHVACNodeValue(nodes []HVACNodeRunSummary, value func(HVACNodeRunSummary) (float64, bool)) (float64, bool) {
	total := 0.0
	count := 0
	for _, node := range nodes {
		if item, ok := value(node); ok {
			total += item
			count++
		}
	}
	if count == 0 {
		return 0, false
	}
	return total / float64(count), true
}

func maxHVACNodeValue(nodes []HVACNodeRunSummary, value func(HVACNodeRunSummary) (float64, bool)) (float64, bool) {
	maximum := math.Inf(-1)
	found := false
	for _, node := range nodes {
		if item, ok := value(node); ok {
			maximum = math.Max(maximum, item)
			found = true
		}
	}
	if !found {
		return 0, false
	}
	return maximum, true
}

func hvacFlowStatus(value float64) string {
	if value <= 0.001 {
		return "warning"
	}
	return "ok"
}

func hvacSetpointDeltaStatus(value float64) string {
	if value > 5 {
		return "warning"
	}
	return "ok"
}

func hvacActiveFlowStatus(value float64) string {
	if value < 5 {
		return "warning"
	}
	return "ok"
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
		for _, component := range result.Components {
			if source := seriesSource(component.Series); source != "missing" {
				return source
			}
		}
	}
	return "missing"
}

func buildComfortResult(series []SimulationSeries, scope SimulationPurposeScope) ComfortResult {
	result := ComfortResult{}
	foundVariables := map[string]bool{}
	zoneMap := map[string]*ComfortZoneResult{}
	zoneOrder := []string{}
	periodLabel := comfortPeriodScopeLabel(scope)
	if periodLabel != "" {
		result.PeriodScope = periodLabel
	}
	for _, item := range series {
		if !purposeSeriesMatchesVariables(item.Column, comfortCheckVariables()) {
			continue
		}
		filteredItem, ok := filterComfortSeriesByPeriod(item, scope)
		if !ok {
			continue
		}
		keyValue, variableName := splitPurposeSeriesColumn(item.Column)
		if keyValue == "" {
			keyValue = "Unknown Zone"
		}
		foundVariables[normalizePurposeToken(variableName)] = true
		result.Series = append(result.Series, filteredItem)
		zoneKey := normalizePurposeToken(keyValue)
		zone := zoneMap[zoneKey]
		if zone == nil {
			zone = &ComfortZoneResult{ZoneName: keyValue}
			zoneMap[zoneKey] = zone
			zoneOrder = append(zoneOrder, zoneKey)
		}
		zone.Metrics = append(zone.Metrics, ComfortMetricResult{
			Name:    variableName,
			Unit:    seriesDisplayUnit(filteredItem),
			Source:  filteredItem.File,
			Min:     seriesDisplayMin(filteredItem),
			Max:     seriesDisplayMax(filteredItem),
			Average: seriesDisplayAverage(filteredItem),
			Points:  append([]SimulationPoint(nil), seriesDisplayPoints(filteredItem)...),
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
	result.Issues = buildComfortIssueRanking(result.Zones)
	result.Completeness = comfortSeriesCompleteness(foundVariables, seriesSource(result.Series))
	return result
}

func filterComfortSeriesByPeriod(item SimulationSeries, scope SimulationPurposeScope) (SimulationSeries, bool) {
	startDay, endDay, ok := comfortPeriodScopeDays(scope)
	if !ok {
		return item, true
	}
	filtered := item
	filtered.Points = []SimulationPoint{}
	for _, point := range item.Points {
		day, dayOK := comfortPointDay(point)
		if !dayOK || !comfortDayInScope(day, startDay, endDay) {
			continue
		}
		filtered.Points = append(filtered.Points, point)
	}
	if len(filtered.Points) == 0 {
		return filtered, false
	}
	filtered.Min, filtered.Max, filtered.Average = simulationPointStats(filtered.Points)
	return normalizeSimulationSeriesDisplay(filtered), true
}

func comfortPeriodScopeDays(scope SimulationPurposeScope) (int, int, bool) {
	mode := strings.ToLower(strings.TrimSpace(scope.PeriodMode))
	if mode == "" || mode == "full" || mode == "run_period" {
		return 0, 0, false
	}
	if mode != "custom" && strings.TrimSpace(scope.PeriodStart) == "" && strings.TrimSpace(scope.PeriodEnd) == "" {
		return 0, 0, false
	}
	startDay := 1
	endDay := 365
	if value, ok := parseComfortPeriodDay(scope.PeriodStart); ok {
		startDay = value
	}
	if value, ok := parseComfortPeriodDay(scope.PeriodEnd); ok {
		endDay = value
	}
	return startDay, endDay, true
}

func comfortPeriodScopeLabel(scope SimulationPurposeScope) string {
	_, _, ok := comfortPeriodScopeDays(scope)
	if !ok {
		return ""
	}
	start := strings.TrimSpace(scope.PeriodStart)
	if start == "" {
		start = "year start"
	}
	end := strings.TrimSpace(scope.PeriodEnd)
	if end == "" {
		end = "year end"
	}
	return start + " to " + end
}

func parseComfortPeriodDay(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	token := strings.Fields(value)[0]
	token = strings.ReplaceAll(token, "/", "-")
	token = strings.ReplaceAll(token, ".", "-")
	parts := strings.Split(token, "-")
	if len(parts) >= 3 && len(parts[0]) == 4 {
		parts = parts[1:]
	}
	if len(parts) < 2 {
		return 0, false
	}
	month, err := strconv.Atoi(strings.TrimLeft(parts[0], "0"))
	if err != nil {
		return 0, false
	}
	day, err := strconv.Atoi(strings.TrimLeft(parts[1], "0"))
	if err != nil {
		return 0, false
	}
	result := dayOfYear(month, day)
	return result, result > 0
}

func comfortPointDay(point SimulationPoint) (int, bool) {
	if day, ok := parseComfortPeriodDay(point.Label); ok {
		return day, true
	}
	x := point.X
	if x <= 0 {
		return 0, false
	}
	if x <= 366 {
		return x, true
	}
	return ((x - 1) / 24) + 1, true
}

func comfortDayInScope(day int, startDay int, endDay int) bool {
	if startDay <= endDay {
		return day >= startDay && day <= endDay
	}
	return day >= startDay || day <= endDay
}

func simulationPointStats(points []SimulationPoint) (float64, float64, float64) {
	if len(points) == 0 {
		return 0, 0, 0
	}
	minValue := points[0].Value
	maxValue := points[0].Value
	total := 0.0
	for _, point := range points {
		minValue = math.Min(minValue, point.Value)
		maxValue = math.Max(maxValue, point.Value)
		total += point.Value
	}
	return roundedPurposeNumber(minValue), roundedPurposeNumber(maxValue), roundedPurposeNumber(total / float64(len(points)))
}

func buildComfortIssueRanking(zones []ComfortZoneResult) []ComfortIssueRank {
	issues := []ComfortIssueRank{}
	for _, zone := range zones {
		metrics := comfortMetricMap(zone.Metrics)
		temperature := metrics[normalizePurposeToken("Zone Mean Air Temperature")]
		if temperature == nil {
			continue
		}
		heating := metrics[normalizePurposeToken("Zone Thermostat Heating Setpoint Temperature")]
		cooling := metrics[normalizePurposeToken("Zone Thermostat Cooling Setpoint Temperature")]
		if heating == nil && cooling == nil {
			continue
		}
		issue := ComfortIssueRank{
			ZoneName:     zone.ZoneName,
			Source:       temperature.Source,
			TotalSamples: len(temperature.Points),
			Unit:         temperature.Unit,
		}
		totalDeviation := 0.0
		for index, point := range temperature.Points {
			deviation := 0.0
			if heating != nil && index < len(heating.Points) && point.Value < heating.Points[index].Value {
				deviation = heating.Points[index].Value - point.Value
				issue.HeatingSamples++
			}
			if cooling != nil && index < len(cooling.Points) && point.Value > cooling.Points[index].Value {
				coolingDeviation := point.Value - cooling.Points[index].Value
				if coolingDeviation > deviation {
					deviation = coolingDeviation
				}
				issue.CoolingSamples++
			}
			if deviation <= 0 {
				continue
			}
			issue.UnmetSamples++
			totalDeviation += deviation
			if deviation > issue.MaxDeviation {
				issue.MaxDeviation = deviation
				issue.PeakLabel = point.Label
			}
		}
		if issue.UnmetSamples == 0 {
			continue
		}
		issue.MaxDeviation = roundedPurposeNumber(issue.MaxDeviation)
		issue.AverageDeviation = roundedPurposeNumber(totalDeviation / float64(issue.UnmetSamples))
		issues = append(issues, issue)
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].UnmetSamples != issues[j].UnmetSamples {
			return issues[i].UnmetSamples > issues[j].UnmetSamples
		}
		if issues[i].MaxDeviation != issues[j].MaxDeviation {
			return issues[i].MaxDeviation > issues[j].MaxDeviation
		}
		return strings.ToLower(issues[i].ZoneName) < strings.ToLower(issues[j].ZoneName)
	})
	return issues
}

func comfortMetricMap(metrics []ComfortMetricResult) map[string]*ComfortMetricResult {
	out := map[string]*ComfortMetricResult{}
	for index := range metrics {
		metric := &metrics[index]
		out[normalizePurposeToken(metric.Name)] = metric
	}
	return out
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

func zoneHeatFlowCompleteness(dataset HeatFlowDataset) []PurposeCompletenessItem {
	source := "missing"
	if dataset.SourceFile != "" {
		source = simulationSourceFromFilename(dataset.SourceFile)
	}
	foundTemperature := false
	for _, zone := range dataset.Zones {
		if len(zone.Temperature) > 0 {
			foundTemperature = true
			break
		}
	}
	items := []PurposeCompletenessItem{
		purposeCompleteness(
			SimulationPurposeZoneHeatFlow,
			"Zone Mean Air Temperature",
			foundTemperature,
			source,
		),
	}
	foundCategories := map[string]bool{}
	for _, category := range dataset.Categories {
		foundCategories[normalizePurposeToken(category.VariableName)] = true
	}
	for _, variable := range zoneHeatFlowVariableNames() {
		items = append(items, purposeCompleteness(
			SimulationPurposeZoneHeatFlow,
			variable,
			foundCategories[normalizePurposeToken(variable)],
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

func energyDashboardCompleteness(result EnergyDashboardResult, plan *PurposeRunPlan, fallbackSeries []SimulationSeries) []PurposeCompletenessItem {
	expected := basicEnergyCompletenessOutputs(plan)
	foundSources := map[string]string{}
	for _, item := range result.FacilityMonthly {
		if item.Name != "" {
			foundSources[normalizeEnergyOutputName(item.Name)] = simulationSourceFromFilename(item.Source)
		}
	}
	for _, item := range result.EndUseMonthly {
		if item.Name != "" {
			foundSources[normalizeEnergyOutputName(item.Name)] = simulationSourceFromFilename(item.Source)
		}
	}
	for _, item := range result.ZoneMonthly {
		if item.Metric != "" {
			foundSources[normalizeEnergyOutputName(item.Metric)] = simulationSourceFromFilename(item.Source)
		}
	}
	fallbackSource := energyDashboardSource(result, fallbackSeries)
	items := make([]PurposeCompletenessItem, 0, len(expected))
	for _, name := range expected {
		source, found := foundSources[normalizeEnergyOutputName(name)]
		if source == "" && found {
			source = fallbackSource
		}
		if !found {
			source = "missing"
		}
		items = append(items, purposeCompleteness(SimulationPurposeBasicEnergy, name, found, source))
	}
	return items
}

func basicEnergyCompletenessOutputs(plan *PurposeRunPlan) []string {
	out := []string{}
	if plan != nil {
		for _, object := range plan.OutputObjects {
			if !purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(object.ObjectType)) {
			case "output:meter", "output:meter:meterfileonly", "output:meter:cumulative", "output:meter:cumulativemeterfileonly":
				out = appendUniquePurposeString(out, object.KeyValue)
			case "output:variable":
				if energyZoneVariables()[normalizeEnergyOutputName(object.VariableName)] {
					out = appendUniquePurposeString(out, object.VariableName)
				}
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, name := range energyFacilityMeterNames() {
		out = appendUniquePurposeString(out, name)
	}
	for _, name := range energyEndUseMeterNames() {
		out = appendUniquePurposeString(out, name)
	}
	for _, name := range energyZoneVariableNames() {
		out = appendUniquePurposeString(out, name)
	}
	return out
}

func purposeIDsContain(values []SimulationPurposeID, target SimulationPurposeID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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

func buildIntegrityStaticDiagnostics(path string) []idf.Diagnostic {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return []idf.Diagnostic{integrityStaticDiagnosticUnavailable(fmt.Sprintf("Static Diagnose could not read the run input: %v", err))}
	}
	doc, err := idf.Parse(string(content))
	if err != nil {
		return []idf.Diagnostic{integrityStaticDiagnosticUnavailable(fmt.Sprintf("Static Diagnose could not parse the run input: %v", err))}
	}
	return idf.AnalyzeDiagnostics(doc)
}

type integrityNamedEntity struct {
	Category   string
	Name       string
	Source     string
	ReportName string
	TableName  string
	Values     map[string]string
}

func buildIntegrityCrossChecks(path string, reports []IntegrityTabularReport) []IntegrityCrossCheck {
	staticEntities := buildIntegrityStaticEntities(path)
	sqlEntities, nominalRows := buildIntegritySQLTabularEntities(reports)
	var checks []IntegrityCrossCheck
	for _, category := range []string{"zone", "surface", "construction"} {
		checks = append(checks, matchIntegrityCrossCheckCategory(
			category,
			staticEntities[category],
			sqlEntities[category],
		)...)
	}
	checks = append(checks, nominalRows...)
	sort.SliceStable(checks, func(i, j int) bool {
		left := checks[i]
		right := checks[j]
		if left.Category != right.Category {
			return left.Category < right.Category
		}
		if integrityCrossCheckStatusRank(left.Status) != integrityCrossCheckStatusRank(right.Status) {
			return integrityCrossCheckStatusRank(left.Status) < integrityCrossCheckStatusRank(right.Status)
		}
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	})
	if len(checks) > 240 {
		return checks[:240]
	}
	return checks
}

func buildIntegrityStaticEntities(path string) map[string][]integrityNamedEntity {
	out := map[string][]integrityNamedEntity{}
	path = strings.TrimSpace(path)
	if path == "" {
		return out
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	doc, err := idf.Parse(string(content))
	if err != nil {
		return out
	}
	seen := map[string]bool{}
	for _, obj := range doc.Objects {
		category := integrityStaticEntityCategory(obj.Type)
		if category == "" {
			continue
		}
		name := purposeObjectName(obj)
		if !integrityCandidateNameValid(name) {
			continue
		}
		key := category + "\x00" + integrityAliasKey(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out[category] = append(out[category], integrityNamedEntity{
			Category: category,
			Name:     strings.TrimSpace(name),
			Source:   obj.Type,
		})
	}
	for category := range out {
		sort.SliceStable(out[category], func(i, j int) bool {
			return strings.ToLower(out[category][i].Name) < strings.ToLower(out[category][j].Name)
		})
	}
	return out
}

func integrityStaticEntityCategory(objectType string) string {
	objectType = strings.ToLower(strings.TrimSpace(objectType))
	switch objectType {
	case "zone":
		return "zone"
	case "buildingsurface:detailed", "fenestrationsurface:detailed", "wall:detailed", "roofceiling:detailed", "floor:detailed", "window", "door", "glazeddoor", "internalmass":
		return "surface"
	}
	if strings.HasPrefix(objectType, "construction") {
		return "construction"
	}
	if strings.Contains(objectType, "surface") && !strings.Contains(objectType, "control") {
		return "surface"
	}
	return ""
}

func buildIntegritySQLTabularEntities(reports []IntegrityTabularReport) (map[string][]integrityNamedEntity, []IntegrityCrossCheck) {
	entities := map[string][]integrityNamedEntity{}
	var nominalRows []IntegrityCrossCheck
	seen := map[string]bool{}
	for _, report := range reports {
		categories := integrityTabularReportCategories(report)
		nominal := integrityTabularReportHasNominalLoad(report)
		for _, row := range report.Rows {
			if nominal && integrityCandidateNameValid(row.Name) {
				nominalRows = append(nominalRows, IntegrityCrossCheck{
					Category:  "nominal_load",
					Name:      strings.TrimSpace(row.Name),
					Status:    "info",
					SQLSource: report.Source,
					SQLReport: report.ReportName,
					SQLTable:  report.TableName,
					Message:   "Nominal load row captured from SQL tabular results for sizing/load review.",
					Values:    limitedIntegrityValues(row.Values, 6),
				})
			}
			for _, category := range categories {
				candidates := integritySQLRowNameCandidates(report, row, category)
				for _, name := range candidates {
					key := category + "\x00" + integrityAliasKey(name)
					if seen[key] {
						continue
					}
					seen[key] = true
					entities[category] = append(entities[category], integrityNamedEntity{
						Category:   category,
						Name:       strings.TrimSpace(name),
						Source:     report.Source,
						ReportName: report.ReportName,
						TableName:  report.TableName,
						Values:     limitedIntegrityValues(row.Values, 6),
					})
				}
			}
		}
	}
	for category := range entities {
		sort.SliceStable(entities[category], func(i, j int) bool {
			return strings.ToLower(entities[category][i].Name) < strings.ToLower(entities[category][j].Name)
		})
	}
	return entities, nominalRows
}

func integrityTabularReportCategories(report IntegrityTabularReport) []string {
	text := integrityReportMatchText(report)
	var categories []string
	if integrityTextHasWord(text, "zone") {
		categories = append(categories, "zone")
	}
	if strings.Contains(text, "surface") || strings.Contains(text, "fenestration") || strings.Contains(text, "opaque exterior") || strings.Contains(text, "window") || strings.Contains(text, "door") {
		categories = append(categories, "surface")
	}
	if strings.Contains(text, "construction") {
		categories = append(categories, "construction")
	}
	return categories
}

func integrityTabularReportHasNominalLoad(report IntegrityTabularReport) bool {
	text := integrityReportMatchText(report)
	if strings.Contains(text, "nominal") && strings.Contains(text, "load") {
		return true
	}
	if strings.Contains(text, "sizing") && strings.Contains(text, "load") {
		return true
	}
	for _, column := range report.Columns {
		columnText := normalizePurposeToken(column)
		if strings.Contains(columnText, "nominal") && strings.Contains(columnText, "load") {
			return true
		}
	}
	return false
}

func integrityReportMatchText(report IntegrityTabularReport) string {
	return normalizePurposeToken(strings.Join([]string{report.ReportName, report.For, report.TableName}, " "))
}

func integrityTextHasWord(text string, word string) bool {
	for _, field := range strings.Fields(text) {
		if field == word {
			return true
		}
	}
	return false
}

func integritySQLRowNameCandidates(report IntegrityTabularReport, row IntegrityTabularRow, category string) []string {
	seen := map[string]bool{}
	var candidates []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if !integrityCandidateNameValid(value) {
			return
		}
		key := integrityAliasKey(value)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, value)
	}
	if integrityReportRowNameLikelyCategory(report, category) {
		add(row.Name)
	}
	for column, value := range row.Values {
		if integrityColumnLikelyCategory(column, category) {
			add(value)
		}
	}
	return candidates
}

func integrityReportRowNameLikelyCategory(report IntegrityTabularReport, category string) bool {
	text := integrityReportMatchText(report)
	switch category {
	case "zone":
		return integrityTextHasWord(text, "zone")
	case "surface":
		return strings.Contains(text, "surface") || strings.Contains(text, "fenestration") || strings.Contains(text, "opaque exterior") || strings.Contains(text, "window") || strings.Contains(text, "door")
	case "construction":
		return strings.Contains(text, "construction")
	default:
		return false
	}
}

func integrityColumnLikelyCategory(column string, category string) bool {
	text := normalizePurposeToken(column)
	switch category {
	case "zone":
		return strings.Contains(text, "zone name") || text == "zone"
	case "surface":
		return strings.Contains(text, "surface name") || text == "surface" || strings.Contains(text, "base surface")
	case "construction":
		return strings.Contains(text, "construction name") || text == "construction"
	default:
		return false
	}
}

func matchIntegrityCrossCheckCategory(category string, staticEntities []integrityNamedEntity, sqlEntities []integrityNamedEntity) []IntegrityCrossCheck {
	usedSQL := map[int]bool{}
	var checks []IntegrityCrossCheck
	for _, staticEntity := range staticEntities {
		index, status := findIntegritySQLEntityMatch(staticEntity.Name, sqlEntities, usedSQL)
		if index >= 0 {
			usedSQL[index] = true
			sqlEntity := sqlEntities[index]
			checks = append(checks, IntegrityCrossCheck{
				Category:     category,
				Name:         staticEntity.Name,
				Status:       status,
				StaticSource: staticEntity.Source,
				SQLSource:    sqlEntity.Source,
				SQLReport:    sqlEntity.ReportName,
				SQLTable:     sqlEntity.TableName,
				Message:      integrityCrossCheckMessage(status),
				Values:       limitedIntegrityValues(sqlEntity.Values, 6),
			})
			continue
		}
		checks = append(checks, IntegrityCrossCheck{
			Category:     category,
			Name:         staticEntity.Name,
			Status:       "static_only",
			StaticSource: staticEntity.Source,
			Message:      integrityCrossCheckMessage("static_only"),
		})
	}
	for index, sqlEntity := range sqlEntities {
		if usedSQL[index] {
			continue
		}
		checks = append(checks, IntegrityCrossCheck{
			Category:  category,
			Name:      sqlEntity.Name,
			Status:    "sql_only",
			SQLSource: sqlEntity.Source,
			SQLReport: sqlEntity.ReportName,
			SQLTable:  sqlEntity.TableName,
			Message:   integrityCrossCheckMessage("sql_only"),
			Values:    limitedIntegrityValues(sqlEntity.Values, 6),
		})
	}
	return checks
}

func findIntegritySQLEntityMatch(staticName string, sqlEntities []integrityNamedEntity, used map[int]bool) (int, string) {
	for index, sqlEntity := range sqlEntities {
		if used[index] {
			continue
		}
		if staticName == sqlEntity.Name {
			return index, "exact"
		}
	}
	staticNormalized := normalizePurposeToken(staticName)
	for index, sqlEntity := range sqlEntities {
		if used[index] {
			continue
		}
		if staticNormalized != "" && staticNormalized == normalizePurposeToken(sqlEntity.Name) {
			return index, "normalized"
		}
	}
	staticAlias := integrityAliasKey(staticName)
	for index, sqlEntity := range sqlEntities {
		if used[index] {
			continue
		}
		if staticAlias != "" && staticAlias == integrityAliasKey(sqlEntity.Name) {
			return index, "alias"
		}
	}
	return -1, ""
}

func integrityCrossCheckMessage(status string) string {
	switch status {
	case "exact":
		return "Static IDF name matches SQL tabular row exactly."
	case "normalized":
		return "Matched after case or whitespace normalization."
	case "alias":
		return "Matched after compact alias normalization."
	case "static_only":
		return "Name exists in the IDF but was not found in parsed SQL tabular rows."
	case "sql_only":
		return "Name appears in SQL tabular rows but was not found in the static IDF model."
	case "info":
		return "SQL tabular row captured for review."
	default:
		return ""
	}
}

func integrityCrossCheckStatusRank(status string) int {
	switch status {
	case "static_only", "sql_only":
		return 0
	case "alias", "normalized":
		return 1
	case "exact":
		return 2
	case "info":
		return 3
	default:
		return 4
	}
}

func integrityCandidateNameValid(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return false
	}
	switch normalizePurposeToken(value) {
	case "total", "subtotal", "grand total", "not part of total", "not applicable", "n/a", "na", "none", "unknown":
		return false
	}
	if _, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", ""), 64); err == nil {
		return false
	}
	return integrityAliasKey(value) != ""
}

func integrityAliasKey(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func purposeObjectName(obj idf.Object) string {
	if len(obj.Fields) == 0 {
		return ""
	}
	return strings.TrimSpace(obj.Fields[0].Value)
}

func limitedIntegrityValues(values map[string]string, limit int) map[string]string {
	if len(values) == 0 || limit <= 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, key := range keys {
		if len(out) >= limit {
			break
		}
		out[key] = strings.TrimSpace(values[key])
	}
	return out
}

func integrityStaticDiagnosticUnavailable(message string) idf.Diagnostic {
	return idf.Diagnostic{
		Severity:   idf.DiagnosticWarning,
		Category:   "Static Diagnose",
		Code:       "static_diagnostics_unavailable",
		Source:     "semantic_idf",
		Confidence: "high",
		Message:    message,
	}
}

func buildComfortUnmetSummariesFromFiles(files []SimulationFileInfo) []ComfortUnmetSummary {
	for _, file := range files {
		if file.Kind != "sqlite" {
			continue
		}
		items, err := parseComfortUnmetSQL(file.Path)
		if err == nil && len(items) > 0 {
			return items
		}
	}
	return nil
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

func (builder *purposePlanBuilder) addDiscoveryDictionaryOutputs() {
	purposeIDs := append([]SimulationPurposeID(nil), builder.request.Purposes...)
	if len(purposeIDs) == 0 {
		purposeIDs = []SimulationPurposeID{SimulationPurposeCustomOutputs}
	}
	builder.addObject(PurposeOutputObject{
		ObjectType: "Output:VariableDictionary",
		Fields: []idf.OutputFieldValue{
			{Name: "Key Field", Value: "Regular"},
		},
		PurposeIDs:  purposeIDs,
		Weight:      "light",
		Description: "Requests EnergyPlus report-variable and meter dictionaries for output discovery.",
		Reason:      "Discovery catalog dictionary output",
	})
	builder.warn("info", "discovery_dictionary_requested", "Discovery is enabled, so the run plan requests EnergyPlus output dictionaries for the catalog assistant.", firstPurposeID(purposeIDs), "")
}

func (builder *purposePlanBuilder) addBasicEnergy() {
	for _, id := range []string{
		"standard-meter-electricity-facility",
		"standard-meter-naturalgas-facility",
		"standard-meter-district-cooling-facility",
		"standard-meter-district-heating-facility",
		"standard-meter-fuel-oil-no1-facility",
		"standard-meter-fuel-oil-no2-facility",
		"standard-meter-propane-facility",
		"standard-meter-other-fuel-1-facility",
		"standard-meter-other-fuel-2-facility",
		"standard-meter-steam-facility",
		"standard-meter-water-facility",
		"standard-meter-electricity-cooling",
		"standard-meter-electricity-heating",
		"standard-meter-electricity-interior-lights",
		"standard-meter-electricity-interior-equipment",
		"standard-meter-electricity-fans",
		"standard-meter-electricity-pumps",
		"standard-meter-electricity-heat-rejection",
		"standard-meter-electricity-heat-recovery",
		"standard-meter-electricity-water-systems",
		"standard-meter-electricity-exterior-lights",
		"standard-meter-electricity-refrigeration",
		"standard-meter-electricity-produced-facility",
		"standard-meter-district-cooling-cooling",
		"standard-meter-district-heating-heating",
		"standard-meter-naturalgas-heating",
		"standard-meter-naturalgas-water-systems",
		"standard-meter-naturalgas-interior-equipment",
	} {
		builder.addRecommendation(id, SimulationPurposeBasicEnergy)
	}
	if !docHasObject(builder.doc, "Zone") {
		return
	}
	for _, variable := range basicEnergyZoneReportedEnergyVariableNames() {
		builder.addVariableWithReason(SimulationPurposeBasicEnergy, "*", variable, "Monthly", "medium", "Basic Energy Explain: monthly zone-level reported energy.", "Basic Energy Explain output")
	}
	for _, variable := range basicEnergyDeliveredLoadVariableNames() {
		builder.addVariableWithReason(SimulationPurposeBasicEnergy, "*", variable, "Monthly", "medium", "Basic Energy Explain: monthly delivered-load energy or rate fallback.", "Basic Energy Explain output")
	}
	for _, variable := range basicEnergyObjectHeatDriverVariableNames() {
		builder.addVariableWithReason(SimulationPurposeBasicEnergy, "*", variable, "Monthly", "medium", "Basic Energy Heat Drivers: monthly object-level heat-driver explanation output.", "Basic Energy Heat Drivers output")
	}
	for _, variable := range basicEnergyDetailedHeatDriverVariableNames() {
		builder.addVariableWithReason(SimulationPurposeBasicEnergy, "*", variable, "Monthly", "medium", "Basic Energy Heat Drivers: monthly detailed zone heat-driver explanation output.", "Basic Energy Heat Drivers output")
	}
	if purposeIDsContain(builder.request.Purposes, SimulationPurposeZoneHeatFlow) {
		return
	}
	for _, variable := range zoneHeatFlowVariableNames() {
		builder.addVariableWithReason(SimulationPurposeBasicEnergy, "*", variable, "Monthly", "medium", "Basic Energy Heat Drivers: monthly zone heat-balance explanation output.", "Basic Energy Heat Drivers output")
	}
}

func basicEnergyZoneReportedEnergyVariableNames() []string {
	return []string{
		"Zone Lights Electricity Energy",
		"Zone Electric Equipment Electricity Energy",
		"Zone Gas Equipment Gas Energy",
	}
}

func basicEnergyDeliveredLoadVariableNames() []string {
	out := []string{}
	for _, def := range energyLoadAliasCatalog() {
		for _, alias := range def.Aliases {
			out = appendUniquePurposeString(out, alias)
		}
	}
	return out
}

func (builder *purposePlanBuilder) addZoneHeatFlow() {
	if !docHasObject(builder.doc, "Zone") {
		builder.warn("warning", "zone_scope_empty", "Zone Heat Flow needs Zone objects, but none were found.", SimulationPurposeZoneHeatFlow, "")
		return
	}
	keys := []string{"*"}
	if scopedKeys, ok, mode := purposeZoneKeysForScope(builder.request.Scope); ok {
		keys = scopedKeys
		builder.warn("info", "zone_scope_"+mode, fmt.Sprintf("Zone Heat Flow is limited to %d %s zone key(s).", len(keys), mode), SimulationPurposeZoneHeatFlow, "")
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

func purposeZoneKeysForScope(scope SimulationPurposeScope) ([]string, bool, string) {
	mode := strings.ToLower(strings.TrimSpace(scope.ZoneMode))
	switch mode {
	case "selected", "visible", "filtered":
		if len(scope.ZoneNames) == 0 {
			return nil, false, mode
		}
		return append([]string(nil), scope.ZoneNames...), true, mode
	default:
		return nil, false, "all"
	}
}

func (builder *purposePlanBuilder) addHVACLoopCheck() {
	keys := []string{"*"}
	componentTargets := []hvacLoopCheckComponentTarget{}
	targets := builder.hvacLoopCheckTargets()
	if targets.Selected {
		if len(targets.NodeNames) == 0 {
			builder.warn("warning", "hvac_scope_unresolved", "HVAC Loop Check was limited to selected loops, but no loop nodes were resolved.", SimulationPurposeHVACLoopCheck, "")
			return
		}
		keys = targets.NodeNames
		componentTargets = targets.Components
		builder.warn("info", "hvac_scope_selected", fmt.Sprintf("HVAC Loop Check is limited to %d node key(s) across %d selected loop(s) and %d component(s).", len(targets.NodeNames), targets.LoopCount, len(targets.ComponentNames)), SimulationPurposeHVACLoopCheck, "")
	} else {
		builder.warn("warning", "hvac_scope_wildcard", "HVAC Loop Check currently uses wildcard node keys when no loop scope is selected.", SimulationPurposeHVACLoopCheck, "")
	}
	for _, key := range keys {
		for _, variable := range hvacLoopCheckNodeVariables() {
			builder.addVariable(SimulationPurposeHVACLoopCheck, key, variable, "Hourly", "heavy", "Hourly system node state for loop inspection.")
		}
	}
	if targets.Selected {
		for _, component := range componentTargets {
			for _, output := range hvacLoopCheckComponentOutputsForType(component.ObjectType) {
				builder.addVariable(SimulationPurposeHVACLoopCheck, component.Name, output.VariableName, "Hourly", "heavy", output.Description)
			}
		}
		return
	}
	for _, output := range hvacLoopCheckComponentOutputs() {
		builder.addVariable(SimulationPurposeHVACLoopCheck, "*", output.VariableName, "Hourly", "heavy", output.Description)
	}
}

type hvacLoopCheckTargets struct {
	Selected       bool
	LoopCount      int
	NodeNames      []string
	ComponentNames []string
	Components     []hvacLoopCheckComponentTarget
}

type hvacLoopCheckComponentTarget struct {
	Name       string
	ObjectType string
}

func (builder *purposePlanBuilder) hvacLoopCheckTargets() hvacLoopCheckTargets {
	scope := builder.request.Scope
	selectedNames := map[string]map[string]bool{
		"AirLoopHVAC":   purposeNameSet(scope.AirLoopNames),
		"PlantLoop":     purposeNameSet(scope.PlantLoopNames),
		"CondenserLoop": purposeNameSet(scope.CondenserLoopNames),
	}
	selectedComponentIDs := purposeComponentIDSet(scope.ComponentIDs)
	componentScoped := len(selectedComponentIDs) > 0
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
		if !componentScoped {
			for _, node := range purposeHVACLoopNodes(loop) {
				nodeSet[normalizePurposeToken(node)] = strings.TrimSpace(node)
			}
		}
		for _, component := range purposeHVACLoopComponents(loop) {
			if componentScoped && !purposeHVACComponentSelected(component, selectedComponentIDs) {
				continue
			}
			if componentScoped {
				for _, node := range purposeHVACComponentNodes(component) {
					nodeSet[normalizePurposeToken(node)] = strings.TrimSpace(node)
				}
			}
			if component.ObjectName != "" {
				componentSet[normalizePurposeToken(component.ObjectName)] = strings.TrimSpace(component.ObjectName)
				targets.Components = appendUniqueHVACComponentTarget(targets.Components, hvacLoopCheckComponentTarget{
					Name:       strings.TrimSpace(component.ObjectName),
					ObjectType: strings.TrimSpace(component.ObjectType),
				})
			}
		}
	}
	targets.NodeNames = purposeSortedSet(nodeSet)
	targets.ComponentNames = purposeSortedSet(componentSet)
	sort.SliceStable(targets.Components, func(i, j int) bool {
		if !strings.EqualFold(targets.Components[i].Name, targets.Components[j].Name) {
			return strings.ToLower(targets.Components[i].Name) < strings.ToLower(targets.Components[j].Name)
		}
		return strings.ToLower(targets.Components[i].ObjectType) < strings.ToLower(targets.Components[j].ObjectType)
	})
	return targets
}

func purposeComponentIDSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		key := normalizePurposeToken(value)
		if key != "" {
			out[key] = true
		}
	}
	return out
}

func purposeHVACComponentSelected(component idf.HVACComponent, selected map[string]bool) bool {
	for _, id := range purposeHVACComponentIDs(component) {
		if selected[normalizePurposeToken(id)] {
			return true
		}
	}
	return false
}

func purposeHVACComponentIDs(component idf.HVACComponent) []string {
	out := []string{}
	add := func(value string) {
		out = appendUniquePurposeString(out, value)
	}
	if component.ObjectIndex >= 0 {
		for _, prefix := range []string{"component", "source", "terminal"} {
			add(fmt.Sprintf("%s:%d", prefix, component.ObjectIndex))
		}
	}
	if component.ObjectType != "" && component.ObjectName != "" {
		for _, prefix := range []string{"component", "source", "terminal"} {
			add(prefix + ":" + component.ObjectType + ":" + component.ObjectName)
		}
		add(component.ObjectType + ":" + component.ObjectName)
	}
	add(component.ObjectName)
	return out
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

func purposeHVACComponentNodes(component idf.HVACComponent) []string {
	out := []string{}
	add := func(value string) {
		out = appendUniquePurposeString(out, value)
	}
	add(component.InletNode)
	add(component.OutletNode)
	add(component.WaterInletNode)
	add(component.WaterOutletNode)
	for _, usage := range component.NodeUsages {
		add(usage.NodeName)
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

func appendUniqueHVACComponentTarget(targets []hvacLoopCheckComponentTarget, target hvacLoopCheckComponentTarget) []hvacLoopCheckComponentTarget {
	if strings.TrimSpace(target.Name) == "" {
		return targets
	}
	targetKey := normalizePurposeToken(target.ObjectType + ":" + target.Name)
	for _, existing := range targets {
		if normalizePurposeToken(existing.ObjectType+":"+existing.Name) == targetKey {
			return targets
		}
	}
	return append(targets, target)
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

type hvacComponentOutputCatalogItem struct {
	VariableName string
	Category     string
	Description  string
	MatchTokens  []string
}

func hvacLoopCheckComponentOutputs() []hvacComponentOutputCatalogItem {
	return []hvacComponentOutputCatalogItem{
		{VariableName: "Fan Electricity Rate", Category: "fan", Description: "Hourly fan electric power for loop operation review.", MatchTokens: []string{"fan"}},
		{VariableName: "Fan Electricity Energy", Category: "fan", Description: "Hourly fan electric energy for loop operation review.", MatchTokens: []string{"fan"}},
		{VariableName: "Pump Electricity Rate", Category: "pump", Description: "Hourly pump electric power for loop operation review.", MatchTokens: []string{"pump"}},
		{VariableName: "Pump Electricity Energy", Category: "pump", Description: "Hourly pump electric energy for loop operation review.", MatchTokens: []string{"pump"}},
		{VariableName: "Cooling Coil Total Cooling Rate", Category: "cooling coil", Description: "Hourly cooling coil load for loop operation review.", MatchTokens: []string{"coil:cooling", "coolingcoil", "cooling coil"}},
		{VariableName: "Cooling Coil Total Cooling Energy", Category: "cooling coil", Description: "Hourly cooling coil energy for loop operation review.", MatchTokens: []string{"coil:cooling", "coolingcoil", "cooling coil"}},
		{VariableName: "Heating Coil Heating Rate", Category: "heating coil", Description: "Hourly heating coil load for loop operation review.", MatchTokens: []string{"coil:heating", "heatingcoil", "heating coil"}},
		{VariableName: "Heating Coil Heating Energy", Category: "heating coil", Description: "Hourly heating coil energy for loop operation review.", MatchTokens: []string{"coil:heating", "heatingcoil", "heating coil"}},
		{VariableName: "Chiller Evaporator Cooling Rate", Category: "chiller", Description: "Hourly chiller evaporator cooling load for plant-loop review.", MatchTokens: []string{"chiller"}},
		{VariableName: "Chiller Evaporator Cooling Energy", Category: "chiller", Description: "Hourly chiller evaporator cooling energy for plant-loop review.", MatchTokens: []string{"chiller"}},
		{VariableName: "Chiller Electricity Rate", Category: "chiller", Description: "Hourly chiller electric power for plant-loop review.", MatchTokens: []string{"chiller"}},
		{VariableName: "Chiller Electricity Energy", Category: "chiller", Description: "Hourly chiller electric energy for plant-loop review.", MatchTokens: []string{"chiller"}},
		{VariableName: "Boiler Heating Rate", Category: "boiler", Description: "Hourly boiler heating load for plant-loop review.", MatchTokens: []string{"boiler"}},
		{VariableName: "Boiler Heating Energy", Category: "boiler", Description: "Hourly boiler heating energy for plant-loop review.", MatchTokens: []string{"boiler"}},
		{VariableName: "Cooling Tower Heat Transfer Rate", Category: "cooling tower", Description: "Hourly cooling tower heat rejection rate for condenser-loop review.", MatchTokens: []string{"coolingtower", "cooling tower"}},
		{VariableName: "Cooling Tower Heat Transfer Energy", Category: "cooling tower", Description: "Hourly cooling tower heat rejection energy for condenser-loop review.", MatchTokens: []string{"coolingtower", "cooling tower"}},
	}
}

func hvacLoopCheckComponentOutputsForType(objectType string) []hvacComponentOutputCatalogItem {
	objectKey := normalizePurposeToken(objectType)
	out := []hvacComponentOutputCatalogItem{}
	for _, item := range hvacLoopCheckComponentOutputs() {
		for _, token := range item.MatchTokens {
			if strings.Contains(objectKey, normalizePurposeToken(token)) {
				out = append(out, item)
				break
			}
		}
	}
	return out
}

func hvacLoopCheckComponentVariableNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range hvacLoopCheckComponentOutputs() {
		key := normalizePurposeToken(item.VariableName)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item.VariableName)
	}
	return out
}

func hvacComponentOperationCategory(variableName string) string {
	wanted := normalizePurposeToken(variableName)
	for _, item := range hvacLoopCheckComponentOutputs() {
		if normalizePurposeToken(item.VariableName) == wanted {
			return item.Category
		}
	}
	return ""
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
	builder.addRecommendation("standard-summary-all", SimulationPurposeComfort)
	keys := []string{"*"}
	if scopedKeys, ok, mode := purposeZoneKeysForScope(builder.request.Scope); ok {
		keys = scopedKeys
		builder.warn("info", "comfort_scope_"+mode, fmt.Sprintf("Comfort Check is limited to %d %s zone key(s).", len(keys), mode), SimulationPurposeComfort, "")
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
	builder.addVariableWithReason(purposeID, keyValue, variableName, frequency, weight, description, string(purposeID)+" purpose")
}

func (builder *purposePlanBuilder) addVariableWithReason(purposeID SimulationPurposeID, keyValue string, variableName string, frequency string, weight string, description string, reason string) {
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
		Reason:      reason,
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
		object.ObjectIndex = existing.ObjectIndex
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
		requested.ObjectIndex = existing.ObjectIndex
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
	if warning := builder.outputWeightWarning(weight, seriesCount, frameCount); warning.Code != "" {
		builder.warnings = append(builder.warnings, warning)
	}
	return PurposeRunPlan{
		Purposes:          append([]SimulationPurposeID(nil), builder.request.Purposes...),
		OutputObjects:     builder.objects,
		EstimatedWeight:   weight,
		EstimatedSeries:   seriesCount,
		EstimatedFrames:   frameCount,
		RequiresSQL:       true,
		RequiresDiscovery: builder.request.DiscoveryAllowed,
		AllocationPolicy:  builder.request.AllocationPolicy,
		PeriodMode:        builder.request.Scope.PeriodMode,
		PeriodStart:       builder.request.Scope.PeriodStart,
		PeriodEnd:         builder.request.Scope.PeriodEnd,
		Warnings:          builder.warnings,
	}
}

func (builder *purposePlanBuilder) outputWeightWarning(weight string, seriesCount int, frameCount int) PurposeRunWarning {
	normalized := strings.ToLower(strings.TrimSpace(weight))
	if normalized != "heavy" && normalized != "very heavy" {
		return PurposeRunWarning{}
	}
	code := "output_weight_heavy"
	if normalized == "very heavy" {
		code = "output_weight_very_heavy"
	}
	return PurposeRunWarning{
		Severity:  "warning",
		Code:      code,
		PurposeID: firstPurposeID(builder.request.Purposes),
		Message:   fmt.Sprintf("%s output estimate: %d series x %d frames. Consider selected scope or lower frequency if runtime becomes too high.", weight, seriesCount, frameCount),
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
		objectIndex := item.ObjectIndex
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
			ObjectIndex:        &objectIndex,
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
		name := simulationSeriesDisplayName(item)
		total := seriesDisplayTotal(item)
		energy := EnergySeries{
			Name:   name,
			Unit:   seriesDisplayUnit(item),
			Source: item.File,
			Points: append([]SimulationPoint(nil), seriesDisplayPoints(item)...),
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

func simulationSeriesDisplayName(series SimulationSeries) string {
	if strings.TrimSpace(series.DisplayColumn) != "" {
		return strings.TrimSpace(series.DisplayColumn)
	}
	return series.Column
}

func seriesDisplayUnit(series SimulationSeries) string {
	if strings.TrimSpace(series.DisplayUnit) != "" {
		return strings.TrimSpace(series.DisplayUnit)
	}
	return unitFromSeriesColumn(series.Column)
}

func seriesDisplayMin(series SimulationSeries) float64 {
	if strings.TrimSpace(series.DisplayUnit) != "" {
		return series.DisplayMin
	}
	return series.Min
}

func seriesDisplayMax(series SimulationSeries) float64 {
	if strings.TrimSpace(series.DisplayUnit) != "" {
		return series.DisplayMax
	}
	return series.Max
}

func seriesDisplayAverage(series SimulationSeries) float64 {
	if strings.TrimSpace(series.DisplayUnit) != "" {
		return series.DisplayAverage
	}
	return series.Average
}

func seriesDisplayPoints(series SimulationSeries) []SimulationPoint {
	if len(series.DisplayPoints) == len(series.Points) && len(series.DisplayPoints) > 0 {
		return series.DisplayPoints
	}
	return series.Points
}

func seriesDisplayTotal(series SimulationSeries) float64 {
	total := 0.0
	for _, point := range seriesDisplayPoints(series) {
		total += point.Value
	}
	return total
}

func roundedPurposeNumber(value float64) float64 {
	return math.Round(value*1000) / 1000
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
	status := "missing"
	if found {
		status = "found"
	}
	item := PurposeCompletenessItem{
		PurposeID:      purposeID,
		RequiredOutput: requiredOutput,
		Found:          found,
		Source:         source,
		Status:         status,
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

func basicEnergyObjectHeatDriverVariableNames() []string {
	return []string{
		"Fan Air Heat Gain Energy",
		"Fan Air Heat Gain Rate",
	}
}

func basicEnergyDetailedHeatDriverVariableNames() []string {
	excluded := map[string]bool{}
	for _, variable := range zoneHeatFlowVariableNames() {
		excluded[normalizeEnergyOutputName(variable)] = true
	}
	for _, variable := range basicEnergyObjectHeatDriverVariableNames() {
		excluded[normalizeEnergyOutputName(variable)] = true
	}
	out := []string{}
	for _, def := range energyHeatAliasCatalog() {
		for _, alias := range def.Aliases {
			if excluded[normalizeEnergyOutputName(alias)] {
				continue
			}
			out = appendUniquePurposeString(out, alias)
		}
	}
	return out
}

func comfortCheckVariables() []string {
	return []string{
		"Zone Mean Air Temperature",
		"Zone Air Relative Humidity",
		"Zone Thermostat Heating Setpoint Temperature",
		"Zone Thermostat Cooling Setpoint Temperature",
		"Zone Air System Sensible Heating Rate",
		"Zone Air System Sensible Cooling Rate",
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
