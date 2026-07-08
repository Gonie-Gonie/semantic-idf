package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const energyExplanationSchema = "semantic-idf.energy-explanation/v1"
const energyExplanationSummarySchema = "semantic-idf.energy-explanation-summary/v1"
const maxEnergyExplanationTabularRows = 1200

type EnergyExplanationResult struct {
	Schema            string                   `json:"schema"`
	Purpose           string                   `json:"purpose"`
	Frequency         string                   `json:"frequency"`
	AllocationPolicy  string                   `json:"allocationPolicy,omitempty"`
	RelationshipRules []EnergyRelationshipRule `json:"relationshipRules,omitempty"`
	Periods           []EnergyPeriod           `json:"periods,omitempty"`
	Nodes             []EnergyExplanationNode  `json:"nodes"`
	Edges             []EnergyExplanationEdge  `json:"edges"`
	Reconciliation    []EnergyReconciliation   `json:"reconciliation,omitempty"`
	Sources           []EnergyDataSource       `json:"sources,omitempty"`
	Completeness      EnergyCompleteness       `json:"completeness"`
	Warnings          []EnergyWarning          `json:"warnings,omitempty"`
}

type EnergyPeriod struct {
	ID             string                  `json:"id"`
	Label          string                  `json:"label"`
	Kind           string                  `json:"kind"`
	Nodes          []EnergyExplanationNode `json:"nodes,omitempty"`
	Edges          []EnergyExplanationEdge `json:"edges,omitempty"`
	Reconciliation []EnergyReconciliation  `json:"reconciliation,omitempty"`
	Warnings       []EnergyWarning         `json:"warnings,omitempty"`
}

type EnergyExplanationSummary struct {
	Schema                 string                         `json:"schema,omitempty"`
	Period                 string                         `json:"period,omitempty"`
	AllocationPolicy       string                         `json:"allocationPolicy,omitempty"`
	EnergyByCarrier        []EnergyExplanationSummaryItem `json:"energyByCarrier,omitempty"`
	EnergyByEndUse         []EnergyExplanationSummaryItem `json:"energyByEndUse,omitempty"`
	DeliveredLoadByService []EnergyExplanationSummaryItem `json:"deliveredLoadByService,omitempty"`
	HeatDrivers            []EnergyExplanationSummaryItem `json:"heatDrivers,omitempty"`
	Residuals              []EnergyExplanationSummaryItem `json:"residuals,omitempty"`
	TopHeatDrivers         []EnergyExplanationSummaryItem `json:"topHeatDrivers,omitempty"`
	TopZones               []EnergyExplanationSummaryItem `json:"topZones,omitempty"`
	Completeness           EnergyCompleteness             `json:"completeness,omitempty"`
}

type EnergyExplanationSummaryItem struct {
	ID                  string   `json:"id"`
	Level               string   `json:"level,omitempty"`
	Kind                string   `json:"kind,omitempty"`
	Label               string   `json:"label"`
	Value               float64  `json:"value"`
	Unit                string   `json:"unit,omitempty"`
	ZoneName            string   `json:"zoneName,omitempty"`
	ServiceKind         string   `json:"serviceKind,omitempty"`
	Carrier             string   `json:"carrier,omitempty"`
	EndUse              string   `json:"endUse,omitempty"`
	MeterHierarchyLevel string   `json:"meterHierarchyLevel,omitempty"`
	HeatCategory        string   `json:"heatCategory,omitempty"`
	Basis               string   `json:"basis,omitempty"`
	SourceIDs           []string `json:"sourceIds,omitempty"`
}

type EnergyExplanationNode struct {
	ID                  string   `json:"id"`
	Level               string   `json:"level"`
	Kind                string   `json:"kind"`
	Label               string   `json:"label"`
	Value               float64  `json:"value"`
	SignedValue         float64  `json:"signedValue,omitempty"`
	DisplayValue        float64  `json:"displayValue,omitempty"`
	Unit                string   `json:"unit"`
	Period              string   `json:"period,omitempty"`
	ZoneName            string   `json:"zoneName,omitempty"`
	LoopName            string   `json:"loopName,omitempty"`
	ServiceKind         string   `json:"serviceKind,omitempty"`
	Carrier             string   `json:"carrier,omitempty"`
	EndUse              string   `json:"endUse,omitempty"`
	MeterHierarchyLevel string   `json:"meterHierarchyLevel,omitempty"`
	HeatCategory        string   `json:"heatCategory,omitempty"`
	Sign                string   `json:"sign,omitempty"`
	Basis               string   `json:"basis,omitempty"`
	RelatedPathIDs      []string `json:"relatedPathIds,omitempty"`
	SourceIDs           []string `json:"sourceIds,omitempty"`
}

type EnergyExplanationEdge struct {
	ID             string   `json:"id"`
	FromID         string   `json:"fromId"`
	ToID           string   `json:"toId"`
	Value          float64  `json:"value"`
	SignedValue    float64  `json:"signedValue,omitempty"`
	DisplayValue   float64  `json:"displayValue,omitempty"`
	Unit           string   `json:"unit"`
	Period         string   `json:"period,omitempty"`
	Relation       string   `json:"relation"`
	Basis          string   `json:"basis"`
	Formula        string   `json:"formula,omitempty"`
	RuleID         string   `json:"ruleId,omitempty"`
	SourceIDs      []string `json:"sourceIds,omitempty"`
	ZoneName       string   `json:"zoneName,omitempty"`
	ServiceKind    string   `json:"serviceKind,omitempty"`
	RelatedPathIDs []string `json:"relatedPathIds,omitempty"`
}

type EnergyDataSource struct {
	ID                 string `json:"id"`
	SourceType         string `json:"sourceType"`
	IsMeter            bool   `json:"isMeter,omitempty"`
	KeyValue           string `json:"keyValue,omitempty"`
	Name               string `json:"name,omitempty"`
	Units              string `json:"units,omitempty"`
	ReportingFrequency string `json:"reportingFrequency,omitempty"`
	AggregationMethod  string `json:"aggregationMethod,omitempty"`
	IndexGroup         string `json:"indexGroup,omitempty"`
	TableName          string `json:"tableName,omitempty"`
	RowName            string `json:"rowName,omitempty"`
	ColumnName         string `json:"columnName,omitempty"`
	ObjectIndex        *int   `json:"objectIndex,omitempty"`
}

type EnergyReconciliation struct {
	ID             string   `json:"id"`
	Level          string   `json:"level"`
	Period         string   `json:"period"`
	Label          string   `json:"label"`
	ZoneName       string   `json:"zoneName,omitempty"`
	ServiceKind    string   `json:"serviceKind,omitempty"`
	ExpectedValue  float64  `json:"expectedValue"`
	ExplainedValue float64  `json:"explainedValue"`
	ResidualValue  float64  `json:"residualValue"`
	Unit           string   `json:"unit"`
	Basis          string   `json:"basis"`
	Formula        string   `json:"formula,omitempty"`
	SourceIDs      []string `json:"sourceIds,omitempty"`
}

type EnergyCompleteness struct {
	Status             string                          `json:"status"`
	MappedPercent      float64                         `json:"mappedPercent,omitempty"`
	EnergyUse          EnergyCompletenessLevel         `json:"energyUse"`
	DeliveredLoad      EnergyCompletenessLevel         `json:"deliveredLoad"`
	HeatDrivers        EnergyCompletenessLevel         `json:"heatDrivers"`
	Items              []EnergyCompletenessLevel       `json:"items,omitempty"`
	MissingCategories  []string                        `json:"missingCategories,omitempty"`
	SourceAvailability []EnergySourceAvailabilityEntry `json:"sourceAvailability,omitempty"`
}

type EnergyCompletenessLevel struct {
	Level   string `json:"level"`
	Status  string `json:"status"`
	Found   int    `json:"found"`
	Total   int    `json:"total"`
	Message string `json:"message,omitempty"`
}

type EnergySourceAvailabilityEntry struct {
	Name   string `json:"name"`
	Level  string `json:"level"`
	Status string `json:"status"`
}

type EnergyWarning struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Period   string `json:"period,omitempty"`
}

type EnergyRelationshipRule struct {
	ID             string   `json:"id"`
	FromLevel      string   `json:"fromLevel,omitempty"`
	ToLevel        string   `json:"toLevel,omitempty"`
	FromKind       string   `json:"fromKind,omitempty"`
	ToKind         string   `json:"toKind,omitempty"`
	RequiredSource []string `json:"requiredSource,omitempty"`
	Basis          string   `json:"basis"`
	Formula        string   `json:"formula,omitempty"`
}

const (
	energyRelationshipRuleMeterEndUse       = "meter.end_use"
	energyRelationshipRuleMeasuredLoad      = "load.measured_variable"
	energyRelationshipRuleAllocatedZoneLoad = "allocation.by_zone_load_share"
	energyRelationshipRuleHeatDriverBalance = "heat.driver_balance"
	energyRelationshipRuleOnsiteProduction  = "support.onsite_production"
	energyRelationshipRuleEnergyResidual    = "residual.energy_total"
	energyRelationshipRuleHeatResidual      = "residual.heat_driver_balance"
)

type energyMeterAliasDefinition struct {
	Kind                 string
	Label                string
	Carrier              string
	EndUse               string
	HierarchyLevel       string
	FacilityTotal        bool
	Aliases              []string
	LegacyAliases        []string
	OutputRequestAliases []string
}

type energyLoadAliasDefinition struct {
	Kind        string
	Label       string
	ServiceKind string
	Scope       string
	Aliases     []string
}

type energyHeatAliasDefinition struct {
	Kind         string
	Label        string
	HeatCategory string
	ObjectScoped bool
	Aliases      []string
}

type energyExplanationDictionary struct {
	row                sqlOutputDictionaryRow
	isMeter            bool
	reportingFrequency string
	indexGroup         string
	sourceFile         string
	meter              *energyMeterAliasDefinition
	load               *energyLoadAliasDefinition
	heat               *energyHeatAliasDefinition
}

type energyExplanationSeriesBuilder struct {
	dictionary       energyExplanationDictionary
	unit             string
	total            float64
	monthly          map[int]float64
	daily            map[int]float64
	selectedRange    float64
	hasSelectedRange bool
}

type energyExplanationSeries struct {
	Level               string
	Kind                string
	Label               string
	Unit                string
	Carrier             string
	EndUse              string
	MeterHierarchyLevel string
	ServiceKind         string
	ZoneName            string
	LoopName            string
	HeatCategory        string
	HeatSign            string
	Basis               string
	SourceIDs           []string
	Total               float64
	Monthly             map[int]float64
	Daily               map[int]float64
	SelectedRange       float64
	HasSelectedRange    bool

	sourceKeyValue     string
	sourceName         string
	sourceFrequency    string
	sourceIsRate       bool
	sourcePriority     int
	heatSignMultiplier float64
}

type energyExplanationGraph struct {
	Nodes          []EnergyExplanationNode
	Edges          []EnergyExplanationEdge
	Reconciliation []EnergyReconciliation
	Warnings       []EnergyWarning
	MappedPercent  float64
}

type energyExplanationNodeAccumulator struct {
	node EnergyExplanationNode
}

func buildEnergyExplanationResultFromFiles(files []SimulationFileInfo, dashboard EnergyDashboardResult, plan *PurposeRunPlan) EnergyExplanationResult {
	for _, file := range files {
		if file.Kind != "sqlite" {
			continue
		}
		result, err := parseSimulationEnergyExplanationSQL(file.Path, plan)
		if err == nil && (len(result.Nodes) > 0 || len(result.Periods) > 0 || len(result.Sources) > 0) {
			return result
		}
	}
	return buildEnergyExplanationFromDashboard(dashboard, plan)
}

func parseSimulationEnergyExplanationSQL(path string, plan *PurposeRunPlan) (EnergyExplanationResult, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return EnergyExplanationResult{}, err
	}
	defer db.Close()

	series := []energyExplanationSeries{}
	sources := []EnergyDataSource{}
	ready, err := sqlHasTables(db, "ReportDataDictionary", "ReportData", "Time")
	if err != nil {
		return EnergyExplanationResult{}, err
	}
	if ready {
		dictionaries, err := sqlEnergyExplanationDictionaries(db, filepath.Base(path))
		if err != nil {
			return EnergyExplanationResult{}, err
		}
		if len(dictionaries) > 0 {
			intervalHours, err := sqlTimeIntervalHours(db)
			if err != nil {
				intervalHours = map[int64]float64{}
			}
			selectedStartDay, selectedEndDay, hasSelectedRange := energyExplanationSelectedRangeDays(plan)

			ids := make([]int, 0, len(dictionaries))
			byID := map[int]energyExplanationDictionary{}
			for _, dictionary := range dictionaries {
				ids = append(ids, dictionary.row.index)
				byID[dictionary.row.index] = dictionary
			}
			query, args := sqlReportDataQuery(ids)
			rows, err := db.Query(query, args...)
			if err != nil {
				return EnergyExplanationResult{}, err
			}

			builders := map[int]*energyExplanationSeriesBuilder{}
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
				builder := builders[dictionaryIndex]
				if builder == nil {
					builder = &energyExplanationSeriesBuilder{dictionary: dictionary}
					builders[dictionaryIndex] = builder
				}
				number, unit := energyExplanationSQLValue(value.Float64, dictionary, intervalHours[timeIndex])
				builder.unit = unit
				builder.total += number
				if month.Valid && month.Int64 >= 1 && month.Int64 <= 12 {
					if builder.monthly == nil {
						builder.monthly = map[int]float64{}
					}
					builder.monthly[int(month.Int64)] += number
				}
				if energyExplanationSupportsDailyPeriods(dictionary) {
					if rowDay, ok := energyExplanationSQLDayOfYear(month, day); ok {
						if builder.daily == nil {
							builder.daily = map[int]float64{}
						}
						builder.daily[rowDay] += number
					}
				}
				if hasSelectedRange {
					if rowDay, ok := energyExplanationSQLDayOfYear(month, day); ok && comfortDayInScope(rowDay, selectedStartDay, selectedEndDay) {
						builder.selectedRange += number
						builder.hasSelectedRange = true
					}
				}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return EnergyExplanationResult{}, err
			}
			if err := rows.Close(); err != nil {
				return EnergyExplanationResult{}, err
			}

			for _, dictionary := range dictionaries {
				builder := builders[dictionary.row.index]
				if builder == nil || builder.total == 0 && len(builder.monthly) == 0 {
					continue
				}
				source := energyDataSourceForDictionary(dictionary)
				source.ObjectIndex = energyExplanationObjectIndexForDictionary(dictionary, plan)
				sources = append(sources, source)
				series = append(series, energyExplanationSeriesForBuilder(builder, source.ID))
			}
		}
	}
	tabularSeries, tabularSources, err := parseEnergyExplanationTabularAnnual(db, series)
	if err != nil {
		return EnergyExplanationResult{}, err
	}
	series = append(series, tabularSeries...)
	sources = append(sources, tabularSources...)
	if len(series) == 0 && len(sources) == 0 {
		return emptyEnergyExplanationResult(plan), nil
	}
	return buildEnergyExplanationResult(series, sources, plan), nil
}

func buildEnergyExplanationFromDashboard(dashboard EnergyDashboardResult, plan *PurposeRunPlan) EnergyExplanationResult {
	var series []energyExplanationSeries
	var sources []EnergyDataSource
	addSeries := func(item EnergySeries) {
		def, ok := energyMeterAliasDefinitionForName(item.Name)
		if !ok {
			return
		}
		sourceID := "series-" + metricID(item.Name)
		sources = append(sources, EnergyDataSource{
			ID:                sourceID,
			SourceType:        "series",
			IsMeter:           true,
			Name:              item.Name,
			Units:             item.Unit,
			AggregationMethod: "sum_dashboard_series",
		})
		monthly := map[int]float64{}
		for _, point := range item.Points {
			if month, ok := energyExplanationMonthFromPoint(point); ok {
				monthly[month] += point.Value
			}
		}
		series = append(series, energyExplanationSeries{
			Level:               "energy",
			Kind:                def.Kind,
			Label:               def.Label,
			Unit:                item.Unit,
			Carrier:             def.Carrier,
			EndUse:              def.EndUse,
			MeterHierarchyLevel: def.HierarchyLevel,
			SourceIDs:           []string{sourceID},
			Total:               item.Total,
			Monthly:             monthly,
		})
	}
	for _, item := range dashboard.FacilityMonthly {
		addSeries(item)
	}
	for _, item := range dashboard.EndUseMonthly {
		addSeries(item)
	}
	return buildEnergyExplanationResult(series, sources, plan)
}

func preferredEnergyExplanationSeries(series []energyExplanationSeries) []energyExplanationSeries {
	out := make([]energyExplanationSeries, 0, len(series))
	selected := map[string]int{}
	for _, item := range series {
		key := energyExplanationSeriesSelectionKey(item)
		if key == "" {
			out = append(out, item)
			continue
		}
		index, ok := selected[key]
		if !ok {
			selected[key] = len(out)
			out = append(out, item)
			continue
		}
		if energyExplanationSeriesSourcePreferred(item, out[index]) {
			out[index] = item
		}
	}
	return out
}

func energyExplanationSeriesSelectionKey(item energyExplanationSeries) string {
	switch item.Level {
	case "load":
		scope := firstNonEmpty(item.ZoneName, item.LoopName, item.sourceKeyValue)
		return strings.Join([]string{
			item.Level,
			item.Kind,
			normalizeEnergyOutputName(item.ServiceKind),
			normalizeEnergyOutputName(scope),
		}, "|")
	case "heat":
		scope := item.ZoneName
		if strings.TrimSpace(scope) == "" {
			scope = item.sourceKeyValue
		}
		return strings.Join([]string{
			item.Level,
			item.Kind,
			normalizeEnergyOutputName(item.HeatCategory),
			normalizeEnergyOutputName(item.HeatSign),
			normalizeEnergyOutputName(scope),
		}, "|")
	default:
		return ""
	}
}

func energyExplanationSeriesSourcePreferred(candidate energyExplanationSeries, current energyExplanationSeries) bool {
	candidateRank := energyExplanationSeriesSourceRank(candidate)
	currentRank := energyExplanationSeriesSourceRank(current)
	for index := range candidateRank {
		if candidateRank[index] != currentRank[index] {
			return candidateRank[index] < currentRank[index]
		}
	}
	return strings.ToLower(candidate.sourceName) < strings.ToLower(current.sourceName)
}

func energyExplanationSeriesSourceRank(item energyExplanationSeries) [3]int {
	rateRank := 0
	if item.sourceIsRate {
		rateRank = 1
	}
	return [3]int{rateRank, energyExplanationSourceFrequencyRank(item.sourceFrequency), item.sourcePriority}
}

func energyExplanationSourceFrequencyRank(frequency string) int {
	switch strings.ToLower(canonicalPurposeFrequency(frequency)) {
	case "monthly":
		return 0
	case "runperiod", "annual":
		return 1
	case "daily":
		return 2
	case "hourly":
		return 3
	case "timestep", "detailed":
		return 4
	case "":
		return 5
	default:
		return 6
	}
}

func energyAliasPriority(name string, aliases []string) int {
	key := normalizeEnergyOutputName(name)
	for index, alias := range aliases {
		if normalizeEnergyOutputName(alias) == key {
			return index
		}
	}
	return len(aliases) + 100
}

func energyLoadScopeNames(scope string, keyValue string) (string, string) {
	keyValue = strings.TrimSpace(keyValue)
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "zone":
		return keyValue, ""
	case "plant":
		return "", keyValue
	default:
		return "", ""
	}
}

func emptyEnergyExplanationResult(plan *PurposeRunPlan) EnergyExplanationResult {
	allocationPolicy := energyExplanationAllocationPolicy(plan)
	result := EnergyExplanationResult{
		Schema:            energyExplanationSchema,
		Purpose:           string(SimulationPurposeBasicEnergy),
		Frequency:         "monthly",
		AllocationPolicy:  allocationPolicy,
		RelationshipRules: energyRelationshipRuleCatalog(),
	}
	result.Completeness = buildEnergyExplanationCompleteness(nil, nil, plan, 0)
	return result
}

func buildEnergyExplanationResult(series []energyExplanationSeries, sources []EnergyDataSource, plan *PurposeRunPlan) EnergyExplanationResult {
	allocationPolicy := energyExplanationAllocationPolicy(plan)
	series = preferredEnergyExplanationSeries(series)
	sort.SliceStable(series, func(i, j int) bool {
		if series[i].Level != series[j].Level {
			return series[i].Level < series[j].Level
		}
		return series[i].Kind < series[j].Kind
	})
	sort.SliceStable(sources, func(i, j int) bool {
		return sources[i].ID < sources[j].ID
	})
	annual := buildEnergyExplanationGraphForPeriod("annual", series, allocationPolicy, func(item energyExplanationSeries) float64 {
		return item.Total
	})
	months := energyExplanationMonths(series)
	periods := []EnergyPeriod{
		{
			ID:             "annual",
			Label:          "Annual",
			Kind:           "annual",
			Nodes:          append([]EnergyExplanationNode(nil), annual.Nodes...),
			Edges:          append([]EnergyExplanationEdge(nil), annual.Edges...),
			Reconciliation: append([]EnergyReconciliation(nil), annual.Reconciliation...),
			Warnings:       append([]EnergyWarning(nil), annual.Warnings...),
		},
	}
	if selectedRange, ok := buildEnergyExplanationSelectedRangePeriod(series, allocationPolicy, plan); ok {
		periods = append(periods, selectedRange)
	}
	for _, month := range months {
		periodID := fmt.Sprintf("M%d", month)
		graph := buildEnergyExplanationGraphForPeriod(periodID, series, allocationPolicy, func(item energyExplanationSeries) float64 {
			return item.Monthly[month]
		})
		periods = append(periods, EnergyPeriod{
			ID:             periodID,
			Label:          fmt.Sprintf("M%d", month),
			Kind:           "monthly",
			Nodes:          graph.Nodes,
			Edges:          graph.Edges,
			Reconciliation: graph.Reconciliation,
			Warnings:       graph.Warnings,
		})
	}
	for _, day := range energyExplanationDays(series) {
		periodID := fmt.Sprintf("D%d", day)
		graph := buildEnergyExplanationGraphForPeriod(periodID, series, allocationPolicy, func(item energyExplanationSeries) float64 {
			return item.Daily[day]
		})
		periods = append(periods, EnergyPeriod{
			ID:             periodID,
			Label:          fmt.Sprintf("Day %d", day),
			Kind:           "daily",
			Nodes:          graph.Nodes,
			Edges:          graph.Edges,
			Reconciliation: graph.Reconciliation,
			Warnings:       graph.Warnings,
		})
	}
	result := EnergyExplanationResult{
		Schema:            energyExplanationSchema,
		Purpose:           string(SimulationPurposeBasicEnergy),
		Frequency:         "monthly",
		AllocationPolicy:  allocationPolicy,
		RelationshipRules: energyRelationshipRuleCatalog(),
		Periods:           periods,
		Nodes:             annual.Nodes,
		Edges:             annual.Edges,
		Reconciliation:    annual.Reconciliation,
		Sources:           sources,
		Completeness:      buildEnergyExplanationCompleteness(series, sources, plan, annual.MappedPercent),
		Warnings:          annual.Warnings,
	}
	return result
}

func buildEnergyExplanationSelectedRangePeriod(series []energyExplanationSeries, allocationPolicy string, plan *PurposeRunPlan) (EnergyPeriod, bool) {
	label := energyExplanationSelectedRangeLabel(plan)
	if label == "" {
		return EnergyPeriod{}, false
	}
	hasValue := false
	for _, item := range series {
		if item.HasSelectedRange && item.SelectedRange != 0 {
			hasValue = true
			break
		}
	}
	if !hasValue {
		return EnergyPeriod{}, false
	}
	graph := buildEnergyExplanationGraphForPeriod("selected_range", series, allocationPolicy, func(item energyExplanationSeries) float64 {
		if !item.HasSelectedRange {
			return 0
		}
		return item.SelectedRange
	})
	return EnergyPeriod{
		ID:             "selected_range",
		Label:          label,
		Kind:           "selected_range",
		Nodes:          graph.Nodes,
		Edges:          graph.Edges,
		Reconciliation: graph.Reconciliation,
		Warnings:       graph.Warnings,
	}, true
}

func buildEnergyExplanationSummary(explanation EnergyExplanationResult) EnergyExplanationSummary {
	if explanation.Schema == "" || len(explanation.Nodes) == 0 {
		return EnergyExplanationSummary{}
	}
	summary := EnergyExplanationSummary{
		Schema:           energyExplanationSummarySchema,
		Period:           "annual",
		AllocationPolicy: firstNonEmpty(explanation.AllocationPolicy, PurposeAllocationPolicyDirectOnly),
		Completeness:     explanation.Completeness,
	}
	energyByCarrier := map[string]*EnergyExplanationSummaryItem{}
	energyByEndUse := map[string]*EnergyExplanationSummaryItem{}
	loadByService := map[string]*EnergyExplanationSummaryItem{}
	heatByDriver := map[string]*EnergyExplanationSummaryItem{}
	residuals := map[string]*EnergyExplanationSummaryItem{}
	zones := map[string]*EnergyExplanationSummaryItem{}
	for _, node := range explanation.Nodes {
		value := energyExplanationSummaryValue(node)
		if value == 0 {
			continue
		}
		switch {
		case node.Level == "energy" && strings.Contains(node.ID, ".carrier."):
			addEnergyExplanationSummaryNode(energyByCarrier, node.Carrier, node, value)
		case node.Level == "energy" && node.EndUse != "" && node.EndUse != "total":
			key := node.EndUse + "." + node.Carrier
			addEnergyExplanationSummaryNode(energyByEndUse, key, node, value)
		case node.Level == "load":
			key := firstNonEmpty(node.ServiceKind, node.Kind, node.ID)
			addEnergyExplanationSummaryNode(loadByService, key, node, value)
		case node.Level == "heat":
			key := energyExplanationHeatSummaryKey(node)
			addEnergyExplanationSummaryNode(heatByDriver, key, node, value)
			if node.ZoneName != "" {
				addEnergyExplanationSummaryNode(zones, node.ZoneName, EnergyExplanationNode{
					ID:          "zone." + metricID(node.ZoneName),
					Level:       "zone",
					Kind:        "zone.heat_driver",
					Label:       node.ZoneName,
					Value:       value,
					Unit:        node.Unit,
					ZoneName:    node.ZoneName,
					ServiceKind: node.ServiceKind,
					SourceIDs:   node.SourceIDs,
				}, value)
			}
		case node.Level == "residual":
			addEnergyExplanationSummaryNode(residuals, firstNonEmpty(node.Kind, node.ID), node, value)
		}
	}
	summary.EnergyByCarrier = sortedEnergyExplanationSummaryItems(energyByCarrier)
	summary.EnergyByEndUse = sortedEnergyExplanationSummaryItems(energyByEndUse)
	summary.DeliveredLoadByService = sortedEnergyExplanationSummaryItems(loadByService)
	summary.HeatDrivers = sortedEnergyExplanationSummaryItems(heatByDriver)
	summary.Residuals = sortedEnergyExplanationSummaryItems(residuals)
	summary.TopHeatDrivers = limitEnergyExplanationSummaryItems(summary.HeatDrivers, 5)
	summary.TopZones = limitEnergyExplanationSummaryItems(sortedEnergyExplanationSummaryItems(zones), 5)
	return summary
}

func energyExplanationSummaryValue(node EnergyExplanationNode) float64 {
	if node.DisplayValue != 0 {
		return node.DisplayValue
	}
	return node.Value
}

func addEnergyExplanationSummaryNode(groups map[string]*EnergyExplanationSummaryItem, key string, node EnergyExplanationNode, value float64) {
	key = strings.TrimSpace(key)
	if key == "" {
		key = node.ID
	}
	item := groups[key]
	if item == nil {
		groups[key] = &EnergyExplanationSummaryItem{
			ID:                  energyExplanationSummaryItemID(key, node),
			Level:               node.Level,
			Kind:                node.Kind,
			Label:               firstNonEmpty(node.Label, node.Kind, node.ID, key),
			Unit:                node.Unit,
			ZoneName:            node.ZoneName,
			ServiceKind:         node.ServiceKind,
			Carrier:             node.Carrier,
			EndUse:              node.EndUse,
			MeterHierarchyLevel: node.MeterHierarchyLevel,
			HeatCategory:        node.HeatCategory,
			Basis:               node.Basis,
			SourceIDs:           appendUniqueStrings(nil, node.SourceIDs...),
		}
		item = groups[key]
	}
	item.Value = roundedEnergyNumber(item.Value + value)
	item.SourceIDs = appendUniqueStrings(item.SourceIDs, node.SourceIDs...)
}

func energyExplanationSummaryItemID(key string, node EnergyExplanationNode) string {
	if node.Level == "zone" {
		return firstNonEmpty(node.ID, key)
	}
	if node.Level == "energy" && node.EndUse != "" && node.EndUse != "total" && strings.TrimSpace(key) != "" {
		return key
	}
	if node.Level == "heat" && strings.TrimSpace(key) != "" {
		return key
	}
	if node.Kind != "" {
		return node.Kind
	}
	return firstNonEmpty(node.ID, key)
}

func energyExplanationHeatSummaryKey(node EnergyExplanationNode) string {
	key := firstNonEmpty(node.Kind, node.ID)
	if sign := energyExplanationExplicitHeatNodeSign(node); sign != "" && node.Kind != "" {
		return node.Kind + "." + sign
	}
	return key
}

func energyExplanationExplicitHeatNodeSign(node EnergyExplanationNode) string {
	kind := strings.TrimSpace(node.Kind)
	id := strings.TrimSpace(node.ID)
	if kind == "" || id == "" || !strings.HasPrefix(id, kind+".") {
		return ""
	}
	rest := strings.TrimPrefix(id, kind+".")
	sign, _, _ := strings.Cut(rest, ".")
	switch sign {
	case "positive", "negative":
		return sign
	default:
		return ""
	}
}

func sortedEnergyExplanationSummaryItems(groups map[string]*EnergyExplanationSummaryItem) []EnergyExplanationSummaryItem {
	out := make([]EnergyExplanationSummaryItem, 0, len(groups))
	for _, item := range groups {
		out = append(out, *item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if math.Abs(out[i].Value) != math.Abs(out[j].Value) {
			return math.Abs(out[i].Value) > math.Abs(out[j].Value)
		}
		return strings.ToLower(out[i].Label) < strings.ToLower(out[j].Label)
	})
	return out
}

func limitEnergyExplanationSummaryItems(items []EnergyExplanationSummaryItem, limit int) []EnergyExplanationSummaryItem {
	if limit <= 0 || len(items) <= limit {
		return append([]EnergyExplanationSummaryItem(nil), items...)
	}
	return append([]EnergyExplanationSummaryItem(nil), items[:limit]...)
}

func energyExplanationAllocationPolicy(plan *PurposeRunPlan) string {
	if plan == nil {
		return PurposeAllocationPolicyDirectOnly
	}
	return normalizePurposeAllocationPolicy(plan.AllocationPolicy)
}

func buildEnergyExplanationGraphForPeriod(period string, series []energyExplanationSeries, allocationPolicy string, valueFor func(energyExplanationSeries) float64) energyExplanationGraph {
	nodes := map[string]*energyExplanationNodeAccumulator{}
	facilityByCarrier := map[string]string{}
	facilityValueByCarrier := map[string]float64{}
	facilitySourcesByCarrier := map[string][]string{}
	endUseValueByCarrier := map[string]float64{}
	endUseNodesByCarrier := map[string][]string{}
	productionNodesByCarrier := map[string][]string{}
	loadNodesByService := map[string][]string{}
	loadNodesByZoneService := map[string][]string{}
	zoneLoadNodesByService := map[string][]string{}
	heatValueByService := map[string]float64{}
	heatSourcesByService := map[string][]string{}
	heatValueByZoneService := map[string]float64{}
	heatSourcesByZoneService := map[string][]string{}
	addNode := func(node EnergyExplanationNode) {
		if node.ID == "" || node.Value == 0 {
			return
		}
		node.Value = roundedEnergyNumber(node.Value)
		existing := nodes[node.ID]
		if existing == nil {
			nodes[node.ID] = &energyExplanationNodeAccumulator{node: node}
			return
		}
		existing.node.Value = roundedEnergyNumber(existing.node.Value + node.Value)
		existing.node.SignedValue = roundedEnergyNumber(existing.node.SignedValue + node.SignedValue)
		existing.node.DisplayValue = roundedEnergyNumber(existing.node.DisplayValue + node.DisplayValue)
		existing.node.SourceIDs = appendUniqueStrings(existing.node.SourceIDs, node.SourceIDs...)
		existing.node.RelatedPathIDs = appendUniqueStrings(existing.node.RelatedPathIDs, node.RelatedPathIDs...)
	}
	for _, item := range series {
		value := valueFor(item)
		if value == 0 {
			continue
		}
		switch item.Level {
		case "energy":
			nodeID := energyExplanationEnergyNodeID(item)
			addNode(EnergyExplanationNode{
				ID:                  nodeID,
				Level:               "energy",
				Kind:                item.Kind,
				Label:               item.Label,
				Value:               value,
				Unit:                item.Unit,
				Period:              period,
				Carrier:             item.Carrier,
				EndUse:              item.EndUse,
				MeterHierarchyLevel: item.MeterHierarchyLevel,
				SourceIDs:           item.SourceIDs,
			})
			if strings.HasSuffix(item.Kind, ".total") {
				facilityByCarrier[item.Carrier] = nodeID
				facilityValueByCarrier[item.Carrier] += value
				facilitySourcesByCarrier[item.Carrier] = appendUniqueStrings(facilitySourcesByCarrier[item.Carrier], item.SourceIDs...)
			} else if energyExplanationIsProductionEndUse(item) {
				productionNodesByCarrier[item.Carrier] = appendUniqueStrings(productionNodesByCarrier[item.Carrier], nodeID)
			} else {
				endUseValueByCarrier[item.Carrier] += value
				endUseNodesByCarrier[item.Carrier] = appendUniqueStrings(endUseNodesByCarrier[item.Carrier], nodeID)
			}
		case "load":
			nodeID := energyExplanationLoadNodeID(item)
			addNode(EnergyExplanationNode{
				ID:          nodeID,
				Level:       "load",
				Kind:        item.Kind,
				Label:       item.Label,
				Value:       value,
				Unit:        item.Unit,
				Period:      period,
				ZoneName:    item.ZoneName,
				LoopName:    item.LoopName,
				ServiceKind: item.ServiceKind,
				SourceIDs:   item.SourceIDs,
			})
			loadNodesByService[item.ServiceKind] = appendUniqueStrings(loadNodesByService[item.ServiceKind], nodeID)
			if item.ZoneName != "" && item.ServiceKind != "" {
				key := energyExplanationZoneServiceKey(item.ZoneName, item.ServiceKind)
				loadNodesByZoneService[key] = appendUniqueStrings(loadNodesByZoneService[key], nodeID)
				zoneLoadNodesByService[item.ServiceKind] = appendUniqueStrings(zoneLoadNodesByService[item.ServiceKind], nodeID)
			}
		case "heat":
			nodeID := energyExplanationHeatNodeID(item)
			signMultiplier := item.heatSignMultiplier
			if signMultiplier == 0 {
				signMultiplier = 1
			}
			signedValue := value * signMultiplier
			displayValue := math.Abs(signedValue)
			sign := "positive"
			serviceKind := "cooling"
			if signedValue < 0 {
				sign = "negative"
				serviceKind = "heating"
			}
			addNode(EnergyExplanationNode{
				ID:           nodeID,
				Level:        "heat",
				Kind:         item.Kind,
				Label:        item.Label,
				Value:        displayValue,
				SignedValue:  signedValue,
				DisplayValue: displayValue,
				Unit:         item.Unit,
				Period:       period,
				ZoneName:     item.ZoneName,
				ServiceKind:  serviceKind,
				HeatCategory: item.HeatCategory,
				Sign:         sign,
				Basis:        "derived_balance",
				SourceIDs:    item.SourceIDs,
			})
		}
	}

	edges := []EnergyExplanationEdge{}
	reconciliation := []EnergyReconciliation{}
	warnings := []EnergyWarning{}
	meterEndUseRule := energyRelationshipRuleByID(energyRelationshipRuleMeterEndUse)
	onsiteProductionRule := energyRelationshipRuleByID(energyRelationshipRuleOnsiteProduction)
	measuredLoadRule := energyRelationshipRuleByID(energyRelationshipRuleMeasuredLoad)
	allocatedZoneLoadRule := energyRelationshipRuleByID(energyRelationshipRuleAllocatedZoneLoad)
	heatDriverRule := energyRelationshipRuleByID(energyRelationshipRuleHeatDriverBalance)
	energyResidualRule := energyRelationshipRuleByID(energyRelationshipRuleEnergyResidual)
	heatResidualRule := energyRelationshipRuleByID(energyRelationshipRuleHeatResidual)
	for carrier, facilityID := range facilityByCarrier {
		endUseValue := endUseValueByCarrier[carrier]
		facilityValue := facilityValueByCarrier[carrier]
		energySourceIDs := appendUniqueStrings(nil, facilitySourcesByCarrier[carrier]...)
		for _, endUseID := range endUseNodesByCarrier[carrier] {
			endUseNode := nodes[endUseID]
			if endUseNode == nil {
				continue
			}
			energySourceIDs = appendUniqueStrings(energySourceIDs, endUseNode.node.SourceIDs...)
			edges = append(edges, EnergyExplanationEdge{
				ID:        edgeID("edge", period, facilityID, endUseID),
				FromID:    facilityID,
				ToID:      endUseID,
				Value:     endUseNode.node.Value,
				Unit:      endUseNode.node.Unit,
				Period:    period,
				Relation:  "meter_enduse",
				Basis:     meterEndUseRule.Basis,
				Formula:   meterEndUseRule.Formula,
				RuleID:    meterEndUseRule.ID,
				SourceIDs: endUseNode.node.SourceIDs,
			})
		}
		for _, productionID := range productionNodesByCarrier[carrier] {
			productionNode := nodes[productionID]
			if productionNode == nil {
				continue
			}
			edges = append(edges, EnergyExplanationEdge{
				ID:        edgeID("production", period, facilityID, productionID),
				FromID:    facilityID,
				ToID:      productionID,
				Value:     productionNode.node.Value,
				Unit:      productionNode.node.Unit,
				Period:    period,
				Relation:  "onsite_production",
				Basis:     onsiteProductionRule.Basis,
				Formula:   onsiteProductionRule.Formula,
				RuleID:    onsiteProductionRule.ID,
				SourceIDs: productionNode.node.SourceIDs,
			})
		}
		residual := roundedEnergyNumber(facilityValue - endUseValue)
		residualAbs := math.Abs(residual)
		reconciliation = append(reconciliation, EnergyReconciliation{
			ID:             "reconcile.energy." + carrier + "." + period,
			Level:          "energy",
			Period:         period,
			Label:          energyCarrierLabel(carrier) + " total basis",
			ExpectedValue:  roundedEnergyNumber(facilityValue),
			ExplainedValue: roundedEnergyNumber(endUseValue),
			ResidualValue:  residual,
			Unit:           nodes[facilityID].node.Unit,
			Basis:          "residual",
			Formula:        "facility carrier total - mapped broad end-use meters",
			SourceIDs:      energySourceIDs,
		})
		if residualAbs > energyResidualVisibilityThreshold(facilityValue) {
			residualID := "residual.energy." + carrier
			addNode(EnergyExplanationNode{
				ID:        residualID,
				Level:     "residual",
				Kind:      "energy.residual",
				Label:     energyCarrierLabel(carrier) + " residual / other",
				Value:     residualAbs,
				Unit:      nodes[facilityID].node.Unit,
				Period:    period,
				Carrier:   carrier,
				SourceIDs: energySourceIDs,
			})
			edges = append(edges, EnergyExplanationEdge{
				ID:        edgeID("residual", period, facilityID, residualID),
				FromID:    facilityID,
				ToID:      residualID,
				Value:     residualAbs,
				Unit:      nodes[facilityID].node.Unit,
				Period:    period,
				Relation:  "residual",
				Basis:     energyResidualRule.Basis,
				Formula:   energyResidualRule.Formula,
				RuleID:    energyResidualRule.ID,
				SourceIDs: energySourceIDs,
			})
		}
		if residual < -energyResidualVisibilityThreshold(facilityValue) {
			warnings = append(warnings, EnergyWarning{
				Severity: "warning",
				Code:     "end_use_exceeds_facility_total",
				Message:  energyCarrierLabel(carrier) + " end-use meters exceed the available facility total for this period.",
				Period:   period,
			})
		}
	}
	for serviceKind, loadIDs := range loadNodesByService {
		fromID := ""
		switch serviceKind {
		case "cooling":
			fromID = firstExistingNodeID(nodes, "energy.end_use.cooling.electricity", "energy.end_use.cooling.district_cooling")
		case "heating":
			fromID = firstExistingNodeID(nodes, "energy.end_use.heating.electricity", "energy.end_use.heating.natural_gas", "energy.end_use.heating.district_heating")
		}
		if fromID == "" {
			continue
		}
		if allocationPolicy == PurposeAllocationPolicyByZoneLoadShare && addAllocatedZoneLoadShareEdges(&edges, period, nodes, fromID, loadIDs, allocatedZoneLoadRule) {
			continue
		}
		for _, loadID := range loadIDs {
			loadNode := nodes[loadID]
			if loadNode == nil {
				continue
			}
			edges = append(edges, EnergyExplanationEdge{
				ID:          edgeID("load", period, fromID, loadID),
				FromID:      fromID,
				ToID:        loadID,
				Value:       loadNode.node.Value,
				Unit:        loadNode.node.Unit,
				Period:      period,
				Relation:    "delivered_load",
				Basis:       measuredLoadRule.Basis,
				Formula:     measuredLoadRule.Formula,
				RuleID:      measuredLoadRule.ID,
				SourceIDs:   loadNode.node.SourceIDs,
				ServiceKind: serviceKind,
			})
		}
	}
	for _, node := range nodes {
		if node.node.Level != "heat" {
			continue
		}
		fromID := ""
		switch node.node.ServiceKind {
		case "cooling":
			fromID = firstLoadNodeIDForHeat(nodes, loadNodesByZoneService, node.node.ZoneName, "cooling")
		case "heating":
			fromID = firstLoadNodeIDForHeat(nodes, loadNodesByZoneService, node.node.ZoneName, "heating")
		}
		if fromID == "" {
			continue
		}
		heatValueByService[node.node.ServiceKind] += node.node.DisplayValue
		heatSourcesByService[node.node.ServiceKind] = appendUniqueStrings(heatSourcesByService[node.node.ServiceKind], node.node.SourceIDs...)
		if node.node.ZoneName != "" && node.node.ServiceKind != "" {
			key := energyExplanationZoneServiceKey(node.node.ZoneName, node.node.ServiceKind)
			heatValueByZoneService[key] += node.node.DisplayValue
			heatSourcesByZoneService[key] = appendUniqueStrings(heatSourcesByZoneService[key], node.node.SourceIDs...)
		}
		edges = append(edges, EnergyExplanationEdge{
			ID:           edgeID("heat", period, fromID, node.node.ID),
			FromID:       fromID,
			ToID:         node.node.ID,
			Value:        node.node.Value,
			SignedValue:  node.node.SignedValue,
			DisplayValue: node.node.DisplayValue,
			Unit:         node.node.Unit,
			Period:       period,
			Relation:     "heat_driver",
			Basis:        heatDriverRule.Basis,
			Formula:      heatDriverRule.Formula,
			RuleID:       heatDriverRule.ID,
			SourceIDs:    node.node.SourceIDs,
			ZoneName:     node.node.ZoneName,
			ServiceKind:  node.node.ServiceKind,
		})
	}
	for serviceKind, loadIDs := range loadNodesByService {
		if serviceKind == "" {
			continue
		}
		reconciliationLoadIDs := zoneLoadNodesByService[serviceKind]
		if len(reconciliationLoadIDs) == 0 {
			reconciliationLoadIDs = loadIDs
		}
		loadValue := 0.0
		loadUnit := "kWh"
		loadSources := []string{}
		for _, loadID := range reconciliationLoadIDs {
			loadNode := nodes[loadID]
			if loadNode == nil {
				continue
			}
			loadValue += loadNode.node.Value
			if loadNode.node.Unit != "" {
				loadUnit = loadNode.node.Unit
			}
			loadSources = appendUniqueStrings(loadSources, loadNode.node.SourceIDs...)
		}
		if loadValue == 0 {
			continue
		}
		heatValue := roundedEnergyNumber(heatValueByService[serviceKind])
		residual := roundedEnergyNumber(loadValue - heatValue)
		sourceIDs := appendUniqueStrings(loadSources, heatSourcesByService[serviceKind]...)
		reconciliation = append(reconciliation, EnergyReconciliation{
			ID:             "reconcile.heat." + serviceKind + "." + period,
			Level:          "heat",
			Period:         period,
			Label:          energyServiceLabel(serviceKind) + " heat-driver basis",
			ExpectedValue:  roundedEnergyNumber(loadValue),
			ExplainedValue: heatValue,
			ResidualValue:  residual,
			Unit:           loadUnit,
			Basis:          "residual",
			Formula:        "delivered load - mapped heat drivers",
			ServiceKind:    serviceKind,
			SourceIDs:      sourceIDs,
		})
		if math.Abs(residual) <= energyResidualVisibilityThreshold(loadValue) {
			continue
		}
		loadID := firstExistingNodeID(nodes, reconciliationLoadIDs...)
		if loadID == "" {
			continue
		}
		residualID := "residual.heat." + serviceKind
		addNode(EnergyExplanationNode{
			ID:          residualID,
			Level:       "residual",
			Kind:        "heat.residual",
			Label:       energyServiceLabel(serviceKind) + " heat-driver residual",
			Value:       math.Abs(residual),
			Unit:        loadUnit,
			Period:      period,
			ServiceKind: serviceKind,
			Basis:       "residual",
			SourceIDs:   sourceIDs,
		})
		edges = append(edges, EnergyExplanationEdge{
			ID:          edgeID("heat_residual", period, loadID, residualID),
			FromID:      loadID,
			ToID:        residualID,
			Value:       math.Abs(residual),
			Unit:        loadUnit,
			Period:      period,
			Relation:    "residual",
			Basis:       heatResidualRule.Basis,
			Formula:     heatResidualRule.Formula,
			RuleID:      heatResidualRule.ID,
			SourceIDs:   sourceIDs,
			ServiceKind: serviceKind,
		})
		if heatValue > 0 && residual < -energyResidualVisibilityThreshold(loadValue) {
			warnings = append(warnings, EnergyWarning{
				Severity: "warning",
				Code:     "heat_drivers_exceed_delivered_load",
				Message:  energyServiceLabel(serviceKind) + " heat drivers exceed the delivered load basis for this period.",
				Period:   period,
			})
		}
	}
	zoneServiceKeys := make([]string, 0, len(loadNodesByZoneService))
	for key := range loadNodesByZoneService {
		if strings.Contains(key, "|") {
			zoneServiceKeys = append(zoneServiceKeys, key)
		}
	}
	sort.Strings(zoneServiceKeys)
	for _, key := range zoneServiceKeys {
		zoneID, serviceKind := splitEnergyExplanationZoneServiceKey(key)
		if zoneID == "" || serviceKind == "" {
			continue
		}
		loadValue := 0.0
		loadUnit := "kWh"
		zoneName := zoneID
		loadSources := []string{}
		for _, loadID := range loadNodesByZoneService[key] {
			loadNode := nodes[loadID]
			if loadNode == nil {
				continue
			}
			loadValue += loadNode.node.Value
			if loadNode.node.ZoneName != "" {
				zoneName = loadNode.node.ZoneName
			}
			if loadNode.node.Unit != "" {
				loadUnit = loadNode.node.Unit
			}
			loadSources = appendUniqueStrings(loadSources, loadNode.node.SourceIDs...)
		}
		if loadValue == 0 {
			continue
		}
		heatValue := roundedEnergyNumber(heatValueByZoneService[key])
		residual := roundedEnergyNumber(loadValue - heatValue)
		sourceIDs := appendUniqueStrings(loadSources, heatSourcesByZoneService[key]...)
		reconciliation = append(reconciliation, EnergyReconciliation{
			ID:             "reconcile.heat." + serviceKind + "." + zoneID + "." + period,
			Level:          "heat",
			Period:         period,
			Label:          energyServiceLabel(serviceKind) + " heat-driver basis - " + zoneName,
			ZoneName:       zoneName,
			ServiceKind:    serviceKind,
			ExpectedValue:  roundedEnergyNumber(loadValue),
			ExplainedValue: heatValue,
			ResidualValue:  residual,
			Unit:           loadUnit,
			Basis:          "residual",
			Formula:        "zone delivered load - mapped zone heat drivers",
			SourceIDs:      sourceIDs,
		})
	}

	outNodes := make([]EnergyExplanationNode, 0, len(nodes))
	for _, node := range nodes {
		outNodes = append(outNodes, node.node)
	}
	sortEnergyExplanationNodes(outNodes)
	sortEnergyExplanationEdges(edges)
	mapped := 0.0
	totalFacility := 0.0
	for carrier, value := range facilityValueByCarrier {
		totalFacility += value
		mapped += math.Min(value, endUseValueByCarrier[carrier])
	}
	mappedPercent := 0.0
	if totalFacility > 0 {
		mappedPercent = roundedEnergyNumber((mapped / totalFacility) * 100)
	}
	return energyExplanationGraph{
		Nodes:          outNodes,
		Edges:          edges,
		Reconciliation: reconciliation,
		Warnings:       warnings,
		MappedPercent:  mappedPercent,
	}
}

func addAllocatedZoneLoadShareEdges(edges *[]EnergyExplanationEdge, period string, nodes map[string]*energyExplanationNodeAccumulator, fromID string, loadIDs []string, rule EnergyRelationshipRule) bool {
	fromNode := nodes[fromID]
	if fromNode == nil || fromNode.node.Value == 0 {
		return false
	}
	type zoneLoadTarget struct {
		id    string
		node  EnergyExplanationNode
		value float64
	}
	targets := []zoneLoadTarget{}
	totalLoad := 0.0
	for _, loadID := range loadIDs {
		loadNode := nodes[loadID]
		if loadNode == nil || loadNode.node.ZoneName == "" {
			continue
		}
		value := math.Abs(loadNode.node.Value)
		if value == 0 {
			continue
		}
		totalLoad += value
		targets = append(targets, zoneLoadTarget{id: loadID, node: loadNode.node, value: value})
	}
	if totalLoad == 0 || len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		share := target.value / totalLoad
		value := roundedEnergyNumber(fromNode.node.Value * share)
		if value == 0 {
			continue
		}
		*edges = append(*edges, EnergyExplanationEdge{
			ID:          edgeID("allocation", period, fromID, target.id),
			FromID:      fromID,
			ToID:        target.id,
			Value:       value,
			Unit:        fromNode.node.Unit,
			Period:      period,
			Relation:    "allocation",
			Basis:       rule.Basis,
			Formula:     fmt.Sprintf("%s; zone load share %.6f", rule.Formula, share),
			RuleID:      rule.ID,
			SourceIDs:   appendUniqueStrings(fromNode.node.SourceIDs, target.node.SourceIDs...),
			ZoneName:    target.node.ZoneName,
			ServiceKind: target.node.ServiceKind,
		})
	}
	return true
}

func sqlEnergyExplanationDictionaries(db *sql.DB, sourceFile string) ([]energyExplanationDictionary, error) {
	columns, err := sqlTableColumns(db, "ReportDataDictionary")
	if err != nil {
		return nil, err
	}
	indexExpr := quoteSQLiteIdentifier("ReportDataDictionaryIndex")
	keyExpr := sqlTextColumnExpr(columns, "KeyValue", "''")
	nameExpr := sqlTextColumnExpr(columns, "Name", "''")
	unitsExpr := sqlTextColumnExpr(columns, "Units", "''")
	isMeterExpr := sqlCastTextColumnExpr(columns, "IsMeter", "'0'")
	frequencyExpr := sqlTextColumnExpr(columns, "ReportingFrequency", "''")
	indexGroupExpr := sqlTextColumnExpr(columns, "IndexGroup", "''")
	rows, err := db.Query(fmt.Sprintf(`
SELECT DISTINCT rdd.%s,
       %s,
       %s,
       %s,
       %s,
       %s,
       %s
FROM ReportDataDictionary rdd
JOIN ReportData rd ON rd.ReportDataDictionaryIndex = rdd.ReportDataDictionaryIndex
WHERE TRIM(%s) <> '' OR TRIM(%s) <> ''
ORDER BY rdd.%s`, indexExpr, keyExpr, nameExpr, unitsExpr, isMeterExpr, frequencyExpr, indexGroupExpr, nameExpr, keyExpr, indexExpr))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []energyExplanationDictionary
	for rows.Next() {
		var row sqlOutputDictionaryRow
		var isMeterText string
		var frequency string
		var indexGroup string
		if err := rows.Scan(&row.index, &row.keyValue, &row.name, &row.units, &isMeterText, &frequency, &indexGroup); err != nil {
			continue
		}
		if row.index <= 0 || strings.TrimSpace(row.name+row.keyValue) == "" {
			continue
		}
		dictionary := energyExplanationDictionary{
			row:                row,
			isMeter:            parseSQLBool(isMeterText),
			reportingFrequency: strings.TrimSpace(frequency),
			indexGroup:         strings.TrimSpace(indexGroup),
			sourceFile:         sourceFile,
		}
		if def, ok := energyMeterAliasDefinitionForName(firstNonEmpty(row.keyValue, row.name)); ok {
			copy := def
			dictionary.meter = &copy
			dictionary.isMeter = true
		} else if def, ok := energyLoadAliasDefinitionForName(row.name); ok {
			copy := def
			dictionary.load = &copy
		} else if def, ok := energyHeatAliasDefinitionForName(row.name); ok {
			copy := def
			dictionary.heat = &copy
		} else {
			continue
		}
		out = append(out, dictionary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func energyExplanationSeriesForBuilder(builder *energyExplanationSeriesBuilder, sourceID string) energyExplanationSeries {
	dictionary := builder.dictionary
	if dictionary.meter != nil {
		def := dictionary.meter
		return energyExplanationSeries{
			Level:               "energy",
			Kind:                def.Kind,
			Label:               def.Label,
			Unit:                builder.unit,
			Carrier:             def.Carrier,
			EndUse:              def.EndUse,
			MeterHierarchyLevel: def.HierarchyLevel,
			SourceIDs:           []string{sourceID},
			Total:               roundedEnergyNumber(builder.total),
			Monthly:             roundedEnergyExplanationMonthly(builder.monthly),
			Daily:               roundedEnergyExplanationDaily(builder.daily),
			SelectedRange:       roundedEnergyNumber(builder.selectedRange),
			HasSelectedRange:    builder.hasSelectedRange,
			sourceKeyValue:      strings.TrimSpace(dictionary.row.keyValue),
			sourceName:          strings.TrimSpace(firstNonEmpty(dictionary.row.keyValue, dictionary.row.name)),
			sourceFrequency:     strings.TrimSpace(dictionary.reportingFrequency),
			sourcePriority:      energyAliasPriority(firstNonEmpty(dictionary.row.keyValue, dictionary.row.name), def.Aliases),
		}
	}
	if dictionary.load != nil {
		def := dictionary.load
		zoneName, loopName := energyLoadScopeNames(def.Scope, dictionary.row.keyValue)
		return energyExplanationSeries{
			Level:            "load",
			Kind:             def.Kind,
			Label:            def.Label,
			Unit:             builder.unit,
			ServiceKind:      def.ServiceKind,
			ZoneName:         zoneName,
			LoopName:         loopName,
			Basis:            "measured_variable",
			SourceIDs:        []string{sourceID},
			Total:            roundedEnergyNumber(builder.total),
			Monthly:          roundedEnergyExplanationMonthly(builder.monthly),
			Daily:            roundedEnergyExplanationDaily(builder.daily),
			SelectedRange:    roundedEnergyNumber(builder.selectedRange),
			HasSelectedRange: builder.hasSelectedRange,
			sourceKeyValue:   strings.TrimSpace(dictionary.row.keyValue),
			sourceName:       strings.TrimSpace(dictionary.row.name),
			sourceFrequency:  strings.TrimSpace(dictionary.reportingFrequency),
			sourceIsRate:     energyExplanationIntegratesRate(dictionary),
			sourcePriority:   energyAliasPriority(dictionary.row.name, def.Aliases),
		}
	}
	def := dictionary.heat
	zoneName := strings.TrimSpace(dictionary.row.keyValue)
	if def.ObjectScoped {
		zoneName = ""
	}
	heatSign := energyHeatAliasExplicitSign(dictionary.row.name)
	signMultiplier := energyHeatSignMultiplier(heatSign)
	return energyExplanationSeries{
		Level:              "heat",
		Kind:               def.Kind,
		Label:              energyHeatAliasLabel(def.Label, heatSign),
		Unit:               builder.unit,
		ZoneName:           zoneName,
		HeatCategory:       def.HeatCategory,
		HeatSign:           heatSign,
		Basis:              "derived_balance",
		SourceIDs:          []string{sourceID},
		Total:              roundedEnergyNumber(builder.total),
		Monthly:            roundedEnergyExplanationMonthly(builder.monthly),
		Daily:              roundedEnergyExplanationDaily(builder.daily),
		SelectedRange:      roundedEnergyNumber(builder.selectedRange),
		HasSelectedRange:   builder.hasSelectedRange,
		sourceKeyValue:     strings.TrimSpace(dictionary.row.keyValue),
		sourceName:         strings.TrimSpace(dictionary.row.name),
		sourceFrequency:    strings.TrimSpace(dictionary.reportingFrequency),
		sourceIsRate:       energyExplanationIntegratesRate(dictionary),
		sourcePriority:     energyAliasPriority(dictionary.row.name, def.Aliases),
		heatSignMultiplier: signMultiplier,
	}
}

func energyDataSourceForDictionary(dictionary energyExplanationDictionary) EnergyDataSource {
	row := dictionary.row
	return EnergyDataSource{
		ID:                 fmt.Sprintf("sql-rdd-%d", row.index),
		SourceType:         "sql_report_data",
		IsMeter:            dictionary.isMeter,
		KeyValue:           strings.TrimSpace(row.keyValue),
		Name:               strings.TrimSpace(row.name),
		Units:              strings.TrimSpace(row.units),
		ReportingFrequency: strings.TrimSpace(dictionary.reportingFrequency),
		AggregationMethod:  energyExplanationAggregationMethod(dictionary),
		IndexGroup:         strings.TrimSpace(dictionary.indexGroup),
		TableName:          "ReportData",
		ColumnName:         fmt.Sprintf("ReportDataDictionaryIndex=%d", row.index),
	}
}

func parseEnergyExplanationTabularAnnual(db *sql.DB, existing []energyExplanationSeries) ([]energyExplanationSeries, []EnergyDataSource, error) {
	hasTabular, err := sqlTableExists(db, "TabularDataWithStrings")
	if err != nil || !hasTabular {
		return nil, nil, err
	}
	columns, err := sqlTableColumns(db, "TabularDataWithStrings")
	if err != nil {
		return nil, nil, err
	}
	if !sqlHasColumns(columns, "ReportName", "TableName", "RowName", "ColumnName", "Value") {
		return nil, nil, nil
	}
	query := fmt.Sprintf(`
SELECT %s AS table_name,
       %s AS row_name,
       %s AS column_name,
       %s AS units,
       %s AS value
FROM %s
WHERE LOWER(TRIM(COALESCE(%s, ''))) = 'annualbuildingutilityperformancesummary'
  AND TRIM(COALESCE(%s, '')) <> ''
ORDER BY %s`,
		sqlTextColumnExpr(columns, "TableName", "''"),
		sqlTextColumnExpr(columns, "RowName", "''"),
		sqlTextColumnExpr(columns, "ColumnName", "''"),
		sqlTextColumnExpr(columns, "Units", "''"),
		sqlTextColumnExpr(columns, "Value", "''"),
		quoteSQLiteIdentifier("TabularDataWithStrings"),
		sqlTextColumnExpr(columns, "ReportName", "''"),
		sqlTextColumnExpr(columns, "Value", "''"),
		integrityTabularOrderBy(columns),
	)
	rows, err := db.Query(query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	seen := energyExplanationExistingEnergyGroups(existing)
	series := []energyExplanationSeries{}
	sources := []EnergyDataSource{}
	for rows.Next() {
		if len(series) >= maxEnergyExplanationTabularRows {
			break
		}
		var tableName, rowName, columnName, units, valueText string
		if err := rows.Scan(&tableName, &rowName, &columnName, &units, &valueText); err != nil {
			continue
		}
		alias, ok := energyExplanationTabularEnergyAlias(tableName, rowName, columnName)
		if !ok {
			continue
		}
		def, ok := energyMeterAliasDefinitionForName(alias)
		if !ok {
			continue
		}
		groupKey := expectedEnergyExplanationOutputGroupKey(alias, "energy")
		if groupKey == "" || seen[groupKey] {
			continue
		}
		value, ok := parseSQLTabularNumber(valueText)
		if !ok || value == 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		number, unit := convertEnergySQLValue(value, units)
		if number == 0 {
			continue
		}
		sourceID := energyExplanationTabularSourceID(tableName, rowName, columnName)
		sources = append(sources, EnergyDataSource{
			ID:                 sourceID,
			SourceType:         "sql_tabular",
			IsMeter:            true,
			KeyValue:           alias,
			Name:               alias,
			Units:              strings.TrimSpace(units),
			ReportingFrequency: "Annual",
			AggregationMethod:  "tabular_annual_value",
			TableName:          strings.TrimSpace(tableName),
			RowName:            strings.TrimSpace(rowName),
			ColumnName:         tabularColumnLabel(columnName, units),
		})
		series = append(series, energyExplanationSeries{
			Level:               "energy",
			Kind:                def.Kind,
			Label:               def.Label,
			Unit:                unit,
			Carrier:             def.Carrier,
			EndUse:              def.EndUse,
			MeterHierarchyLevel: def.HierarchyLevel,
			SourceIDs:           []string{sourceID},
			Total:               roundedEnergyNumber(number),
			sourceKeyValue:      alias,
			sourceName:          alias,
			sourceFrequency:     "Annual",
			sourcePriority:      energyAliasPriority(alias, def.Aliases),
		})
		seen[groupKey] = true
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return series, sources, nil
}

func energyExplanationExistingEnergyGroups(series []energyExplanationSeries) map[string]bool {
	seen := map[string]bool{}
	for _, item := range series {
		if item.Level != "energy" {
			continue
		}
		if key := energyExplanationCompletenessGroupKey(item); key != "" {
			seen[key] = true
		}
	}
	return seen
}

func energyExplanationTabularEnergyAlias(tableName string, rowName string, columnName string) (string, bool) {
	if !strings.Contains(normalizeEnergyOutputName(tableName), "end uses") {
		return "", false
	}
	carrier, ok := energyExplanationTabularCarrier(columnName)
	if !ok {
		return "", false
	}
	if energyExplanationTabularTotalRow(rowName) {
		return carrier + ":Facility", true
	}
	endUse, ok := energyExplanationTabularEndUse(rowName)
	if !ok {
		return "", false
	}
	if endUse == "Generators" && carrier == "Electricity" {
		return "Generators:ElectricityProduced", true
	}
	return endUse + ":" + carrier, true
}

func energyExplanationTabularCarrier(columnName string) (string, bool) {
	switch normalizeEnergyOutputName(columnName) {
	case "electricity":
		return "Electricity", true
	case "natural gas", "gas":
		return "NaturalGas", true
	case "district cooling":
		return "DistrictCooling", true
	case "district heating":
		return "DistrictHeating", true
	case "steam":
		return "Steam", true
	case "water":
		return "Water", true
	case "fuel oil no 1", "fuel oil 1", "fuel oil #1":
		return "FuelOilNo1", true
	case "fuel oil no 2", "fuel oil 2", "fuel oil #2":
		return "FuelOilNo2", true
	case "propane":
		return "Propane", true
	default:
		return "", false
	}
}

func energyExplanationTabularTotalRow(rowName string) bool {
	switch normalizeEnergyOutputName(rowName) {
	case "total end uses", "total energy", "total site energy", "net site energy":
		return true
	default:
		return false
	}
}

func energyExplanationTabularEndUse(rowName string) (string, bool) {
	switch normalizeEnergyOutputName(rowName) {
	case "cooling":
		return "Cooling", true
	case "heating":
		return "Heating", true
	case "interior lighting":
		return "InteriorLights", true
	case "interior equipment":
		return "InteriorEquipment", true
	case "fans":
		return "Fans", true
	case "pumps":
		return "Pumps", true
	case "heat rejection":
		return "HeatRejection", true
	case "heat recovery":
		return "HeatRecovery", true
	case "water systems":
		return "WaterSystems", true
	case "exterior lighting":
		return "ExteriorLights", true
	case "refrigeration":
		return "Refrigeration", true
	case "generators", "electricity generation":
		return "Generators", true
	default:
		return "", false
	}
}

func energyExplanationTabularSourceID(tableName string, rowName string, columnName string) string {
	return "sql-tabular-" + metricID(tableName+"."+rowName+"."+columnName)
}

func energyExplanationAggregationMethod(dictionary energyExplanationDictionary) string {
	if energyExplanationIntegratesRate(dictionary) {
		return "integrate_rate_by_time_interval"
	}
	return "sum_report_data"
}

func energyExplanationSupportsDailyPeriods(dictionary energyExplanationDictionary) bool {
	frequency := strings.TrimSpace(dictionary.reportingFrequency)
	if frequency == "" {
		return false
	}
	switch strings.ToLower(canonicalPurposeFrequency(frequency)) {
	case "daily", "hourly", "timestep", "detailed":
		return true
	default:
		return false
	}
}

func energyExplanationObjectIndexForDictionary(dictionary energyExplanationDictionary, plan *PurposeRunPlan) *int {
	if plan == nil {
		return nil
	}
	row := dictionary.row
	name := strings.TrimSpace(row.name)
	keyValue := strings.TrimSpace(row.keyValue)
	if dictionary.isMeter {
		sourceName := strings.TrimSpace(firstNonEmpty(keyValue, name))
		for _, object := range plan.OutputObjects {
			if object.ObjectIndex == nil || !purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) {
				continue
			}
			if !energyExplanationIsMeterObjectType(object.ObjectType) {
				continue
			}
			if normalizeEnergyOutputName(object.KeyValue) == normalizeEnergyOutputName(sourceName) {
				return object.ObjectIndex
			}
		}
		return nil
	}
	if name == "" {
		return nil
	}
	var wildcard *int
	for _, object := range plan.OutputObjects {
		if object.ObjectIndex == nil || !purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(object.ObjectType), "Output:Variable") {
			continue
		}
		if normalizeEnergyOutputName(object.VariableName) != normalizeEnergyOutputName(name) {
			continue
		}
		objectKey := strings.TrimSpace(object.KeyValue)
		if objectKey == "*" || objectKey == "" {
			wildcard = object.ObjectIndex
			continue
		}
		if normalizeEnergyOutputName(objectKey) == normalizeEnergyOutputName(keyValue) {
			return object.ObjectIndex
		}
	}
	return wildcard
}

func energyExplanationIsMeterObjectType(objectType string) bool {
	switch strings.ToLower(strings.TrimSpace(objectType)) {
	case "output:meter", "output:meter:meterfileonly", "output:meter:cumulative", "output:meter:cumulativemeterfileonly":
		return true
	default:
		return false
	}
}

func energyExplanationSQLValue(value float64, dictionary energyExplanationDictionary, intervalHours float64) (float64, string) {
	if energyExplanationIntegratesRate(dictionary) {
		hours := intervalHours
		if hours <= 0 || math.IsNaN(hours) || math.IsInf(hours, 0) {
			hours = 1
		}
		switch normalizeUnitToken(dictionary.row.units) {
		case "w":
			return roundedEnergyNumber(value * hours / 1000), "kWh"
		case "kw":
			return roundedEnergyNumber(value * hours), "kWh"
		}
	}
	number, unit := convertEnergySQLValue(value, dictionary.row.units)
	return roundedEnergyNumber(number), unit
}

func energyExplanationIntegratesRate(dictionary energyExplanationDictionary) bool {
	name := normalizeEnergyOutputName(dictionary.row.name)
	if dictionary.heat != nil {
		return strings.Contains(name, " rate") || normalizeUnitToken(dictionary.row.units) == "w" || normalizeUnitToken(dictionary.row.units) == "kw"
	}
	if dictionary.load != nil {
		return strings.Contains(name, " rate")
	}
	return false
}

type sqlTimeIntervalRow struct {
	index   int64
	minutes int
	valid   bool
}

func sqlTimeIntervalHours(db *sql.DB) (map[int64]float64, error) {
	columns, err := sqlTableColumns(db, "Time")
	if err != nil {
		return nil, err
	}
	if !sqlHasColumns(columns, "TimeIndex") {
		return map[int64]float64{}, nil
	}
	query := fmt.Sprintf(`
SELECT %s AS time_index,
       %s AS month,
       %s AS day,
       %s AS hour,
       %s AS minute
FROM %s
ORDER BY %s`,
		quoteSQLiteIdentifier(columns[normalizeSQLColumnName("TimeIndex")]),
		sqlTextColumnExpr(columns, "Month", "''"),
		sqlTextColumnExpr(columns, "Day", "''"),
		sqlTextColumnExpr(columns, "Hour", "''"),
		sqlTextColumnExpr(columns, "Minute", "''"),
		quoteSQLiteIdentifier("Time"),
		quoteSQLiteIdentifier(columns[normalizeSQLColumnName("TimeIndex")]),
	)
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []sqlTimeIntervalRow
	for rows.Next() {
		var index int64
		var monthText, dayText, hourText, minuteText string
		if err := rows.Scan(&index, &monthText, &dayText, &hourText, &minuteText); err != nil {
			continue
		}
		minutes, ok := sqlTimeOrdinalMinutes(monthText, dayText, hourText, minuteText)
		values = append(values, sqlTimeIntervalRow{index: index, minutes: minutes, valid: ok})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := map[int64]float64{}
	for index, row := range values {
		hours := 1.0
		if row.valid {
			if index > 0 && values[index-1].valid {
				hours = float64(row.minutes-values[index-1].minutes) / 60
			} else if row.minutes > 0 {
				hours = float64(row.minutes) / 60
			} else if index+1 < len(values) && values[index+1].valid {
				hours = float64(values[index+1].minutes-row.minutes) / 60
			}
		}
		if hours <= 0 || math.IsNaN(hours) || math.IsInf(hours, 0) {
			hours = 1
		}
		out[row.index] = hours
	}
	return out, nil
}

func sqlTimeOrdinalMinutes(monthText string, dayText string, hourText string, minuteText string) (int, bool) {
	month, okMonth := parseSQLTimeInt(monthText)
	day, okDay := parseSQLTimeInt(dayText)
	hour, okHour := parseSQLTimeInt(hourText)
	minute, _ := parseSQLTimeInt(minuteText)
	if !okMonth || !okDay || !okHour {
		return 0, false
	}
	monthDays := [...]int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	dayOfYear := 0
	for m := 1; m < month && m < len(monthDays); m++ {
		dayOfYear += monthDays[m]
	}
	dayOfYear += maxInt(day, 1) - 1
	return ((dayOfYear*24)+hour)*60 + minute, true
}

func energyExplanationSQLDayOfYear(month sql.NullInt64, day sql.NullInt64) (int, bool) {
	if !month.Valid || !day.Valid {
		return 0, false
	}
	value := dayOfYear(int(month.Int64), int(day.Int64))
	return value, value > 0
}

func energyExplanationSelectedRangeDays(plan *PurposeRunPlan) (int, int, bool) {
	return comfortPeriodScopeDays(energyExplanationPeriodScope(plan))
}

func energyExplanationSelectedRangeLabel(plan *PurposeRunPlan) string {
	return comfortPeriodScopeLabel(energyExplanationPeriodScope(plan))
}

func energyExplanationPeriodScope(plan *PurposeRunPlan) SimulationPurposeScope {
	if plan == nil {
		return SimulationPurposeScope{}
	}
	return SimulationPurposeScope{
		PeriodMode:  plan.PeriodMode,
		PeriodStart: plan.PeriodStart,
		PeriodEnd:   plan.PeriodEnd,
	}
}

func parseSQLTimeInt(value string) (int, bool) {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return number, true
}

func energyRelationshipRuleCatalog() []EnergyRelationshipRule {
	return []EnergyRelationshipRule{
		{
			ID:             energyRelationshipRuleMeterEndUse,
			FromLevel:      "energy",
			ToLevel:        "energy",
			FromKind:       "facility_total",
			ToKind:         "broad_end_use",
			RequiredSource: []string{"sql_report_data:meter"},
			Basis:          "measured_meter",
			Formula:        "sum(ReportData where IsMeter=1 and Name or KeyValue matches end-use meter)",
		},
		{
			ID:             energyRelationshipRuleMeasuredLoad,
			FromLevel:      "energy",
			ToLevel:        "load",
			FromKind:       "cooling_or_heating_end_use",
			ToKind:         "delivered_load",
			RequiredSource: []string{"sql_report_data:variable"},
			Basis:          "measured_variable",
			Formula:        "reported delivered load; not COP-converted from energy use",
		},
		{
			ID:             energyRelationshipRuleAllocatedZoneLoad,
			FromLevel:      "energy",
			ToLevel:        "load",
			FromKind:       "cooling_or_heating_end_use",
			ToKind:         "zone_delivered_load",
			RequiredSource: []string{"sql_report_data:meter", "sql_report_data:variable"},
			Basis:          "allocated",
			Formula:        "allocate end-use energy by measured zone delivered-load share",
		},
		{
			ID:             energyRelationshipRuleHeatDriverBalance,
			FromLevel:      "load",
			ToLevel:        "heat",
			FromKind:       "zone_delivered_load",
			ToKind:         "zone_heat_driver",
			RequiredSource: []string{"sql_report_data:heat_balance_variable"},
			Basis:          "derived_balance",
			Formula:        "integrate(zone heat-balance rate W * timestep_hours) / 1000",
		},
		{
			ID:             energyRelationshipRuleOnsiteProduction,
			FromLevel:      "energy",
			ToLevel:        "energy",
			FromKind:       "facility_total",
			ToKind:         "onsite_production",
			RequiredSource: []string{"ElectricityProduced:Facility", "Generators:ElectricityProduced"},
			Basis:          "measured_meter",
			Formula:        "onsite production meter shown separately from facility consumption residual",
		},
		{
			ID:             energyRelationshipRuleEnergyResidual,
			FromLevel:      "energy",
			ToLevel:        "residual",
			FromKind:       "facility_total",
			ToKind:         "energy_residual",
			RequiredSource: []string{"sql_report_data:meter"},
			Basis:          "residual",
			Formula:        "abs(facility carrier total - mapped broad end-use meters)",
		},
		{
			ID:             energyRelationshipRuleHeatResidual,
			FromLevel:      "load",
			ToLevel:        "residual",
			FromKind:       "delivered_load",
			ToKind:         "heat_driver_residual",
			RequiredSource: []string{"sql_report_data:variable"},
			Basis:          "residual",
			Formula:        "abs(delivered load - mapped heat drivers)",
		},
	}
}

func energyRelationshipRuleByID(id string) EnergyRelationshipRule {
	for _, rule := range energyRelationshipRuleCatalog() {
		if rule.ID == id {
			return rule
		}
	}
	return EnergyRelationshipRule{ID: id}
}

func energyMeterAliasCatalog() []energyMeterAliasDefinition {
	return []energyMeterAliasDefinition{
		{Kind: "energy.electricity.total", Label: "Electricity total", Carrier: "electricity", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"Electricity:Facility"}},
		{Kind: "energy.gas.total", Label: "Natural gas total", Carrier: "natural_gas", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"NaturalGas:Facility", "Gas:Facility"}},
		{Kind: "energy.district_cooling.total", Label: "District cooling total", Carrier: "district_cooling", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"DistrictCooling:Facility"}},
		{Kind: "energy.district_heating.total", Label: "District heating total", Carrier: "district_heating", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"DistrictHeating:Facility"}},
		{Kind: "energy.fuel_oil_1.total", Label: "Fuel oil #1 total", Carrier: "fuel_oil_1", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"FuelOilNo1:Facility"}},
		{Kind: "energy.fuel_oil_2.total", Label: "Fuel oil #2 total", Carrier: "fuel_oil_2", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"FuelOilNo2:Facility"}},
		{Kind: "energy.propane.total", Label: "Propane total", Carrier: "propane", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"Propane:Facility"}},
		{Kind: "energy.other_fuel_1.total", Label: "Other fuel 1 total", Carrier: "other_fuel_1", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"OtherFuel1:Facility"}},
		{Kind: "energy.other_fuel_2.total", Label: "Other fuel 2 total", Carrier: "other_fuel_2", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"OtherFuel2:Facility"}},
		{Kind: "energy.steam.total", Label: "Steam total", Carrier: "steam", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"Steam:Facility"}},
		{Kind: "energy.water.total", Label: "Water total", Carrier: "water", EndUse: "water", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"Water:Facility"}},
		{Kind: "energy.cooling", Label: "Cooling energy", Carrier: "electricity", EndUse: "cooling", HierarchyLevel: "broad_end_use", Aliases: []string{"Cooling:Electricity", "Electricity:Cooling"}},
		{Kind: "energy.heating", Label: "Heating energy", Carrier: "electricity", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:Electricity", "Electricity:Heating"}},
		{Kind: "energy.interior_lighting", Label: "Interior lighting", Carrier: "electricity", EndUse: "interior_lighting", HierarchyLevel: "broad_end_use", Aliases: []string{"InteriorLights:Electricity", "Electricity:InteriorLights"}},
		{Kind: "energy.interior_equipment", Label: "Interior equipment", Carrier: "electricity", EndUse: "interior_equipment", HierarchyLevel: "broad_end_use", Aliases: []string{"InteriorEquipment:Electricity", "Electricity:InteriorEquipment"}},
		{Kind: "energy.fans", Label: "Fans", Carrier: "electricity", EndUse: "fans", HierarchyLevel: "broad_end_use", Aliases: []string{"Fans:Electricity", "Electricity:Fans"}},
		{Kind: "energy.pumps", Label: "Pumps", Carrier: "electricity", EndUse: "pumps", HierarchyLevel: "broad_end_use", Aliases: []string{"Pumps:Electricity", "Electricity:Pumps"}},
		{Kind: "energy.heat_rejection", Label: "Heat rejection", Carrier: "electricity", EndUse: "heat_rejection", HierarchyLevel: "broad_end_use", Aliases: []string{"HeatRejection:Electricity", "Electricity:HeatRejection"}},
		{Kind: "energy.heat_recovery", Label: "Heat recovery", Carrier: "electricity", EndUse: "heat_recovery", HierarchyLevel: "broad_end_use", Aliases: []string{"HeatRecovery:Electricity", "Electricity:HeatRecovery"}},
		{Kind: "energy.water_systems", Label: "Water systems", Carrier: "electricity", EndUse: "water_systems", HierarchyLevel: "broad_end_use", Aliases: []string{"WaterSystems:Electricity", "Electricity:WaterSystems"}},
		{Kind: "energy.exterior_lighting", Label: "Exterior lighting", Carrier: "electricity", EndUse: "exterior_lighting", HierarchyLevel: "broad_end_use", Aliases: []string{"ExteriorLights:Electricity", "Electricity:ExteriorLights"}},
		{Kind: "energy.refrigeration", Label: "Refrigeration", Carrier: "electricity", EndUse: "refrigeration", HierarchyLevel: "broad_end_use", Aliases: []string{"Refrigeration:Electricity", "Electricity:Refrigeration"}},
		{Kind: "energy.generators", Label: "Generators / onsite production", Carrier: "electricity", EndUse: "generators", HierarchyLevel: "broad_end_use", Aliases: []string{"Generators:ElectricityProduced", "ElectricityProduced:Facility"}},
		{Kind: "energy.cooling", Label: "District cooling", Carrier: "district_cooling", EndUse: "cooling", HierarchyLevel: "broad_end_use", Aliases: []string{"Cooling:DistrictCooling", "DistrictCooling:Cooling"}},
		{Kind: "energy.heating", Label: "Natural gas heating", Carrier: "natural_gas", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:NaturalGas", "NaturalGas:Heating"}},
		{Kind: "energy.water_systems", Label: "Natural gas water systems", Carrier: "natural_gas", EndUse: "water_systems", HierarchyLevel: "broad_end_use", Aliases: []string{"WaterSystems:NaturalGas", "NaturalGas:WaterSystems"}},
		{Kind: "energy.interior_equipment", Label: "Natural gas interior equipment", Carrier: "natural_gas", EndUse: "interior_equipment", HierarchyLevel: "broad_end_use", Aliases: []string{"InteriorEquipment:NaturalGas", "NaturalGas:InteriorEquipment"}},
		{Kind: "energy.heating", Label: "District heating", Carrier: "district_heating", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:DistrictHeating", "DistrictHeating:Heating"}},
	}
}

func energyLoadAliasCatalog() []energyLoadAliasDefinition {
	return []energyLoadAliasDefinition{
		{Kind: "load.zone_cooling", Label: "Zone cooling load", ServiceKind: "cooling", Scope: "zone", Aliases: []string{"Zone Air System Sensible Cooling Energy", "Zone Air System Sensible Cooling Rate", "Zone Ideal Loads Zone Sensible Cooling Energy", "Zone Ideal Loads Supply Air Total Cooling Energy"}},
		{Kind: "load.zone_heating", Label: "Zone heating load", ServiceKind: "heating", Scope: "zone", Aliases: []string{"Zone Air System Sensible Heating Energy", "Zone Air System Sensible Heating Rate", "Zone Ideal Loads Zone Sensible Heating Energy", "Zone Ideal Loads Supply Air Total Heating Energy"}},
		{Kind: "load.zone_radiant_cooling", Label: "Radiant cooling load", ServiceKind: "cooling", Scope: "zone", Aliases: []string{"Zone Radiant HVAC Cooling Energy", "Zone Radiant HVAC Cooling Rate"}},
		{Kind: "load.zone_radiant_heating", Label: "Radiant heating load", ServiceKind: "heating", Scope: "zone", Aliases: []string{"Zone Radiant HVAC Heating Energy", "Zone Radiant HVAC Heating Rate"}},
		{Kind: "load.system_cooling", Label: "System cooling delivered", ServiceKind: "cooling", Scope: "system", Aliases: []string{"Cooling Coil Total Cooling Energy", "Cooling Coil Sensible Cooling Energy", "Cooling Coil Total Cooling Rate"}},
		{Kind: "load.system_heating", Label: "System heating delivered", ServiceKind: "heating", Scope: "system", Aliases: []string{"Heating Coil Heating Energy", "Heating Coil Heating Rate"}},
		{Kind: "load.plant_cooling", Label: "Plant cooling demand", ServiceKind: "cooling", Scope: "plant", Aliases: []string{"Plant Supply Side Cooling Demand Rate", "Plant Loop Cooling Demand Energy"}},
		{Kind: "load.plant_heating", Label: "Plant heating demand", ServiceKind: "heating", Scope: "plant", Aliases: []string{"Plant Supply Side Heating Demand Rate", "Plant Loop Heating Demand Energy"}},
	}
}

func energyHeatAliasCatalog() []energyHeatAliasDefinition {
	return []energyHeatAliasDefinition{
		{Kind: "heat.internal_convective", Label: "Internal convective gains", HeatCategory: "internal_gains", Aliases: []string{"Zone Air Heat Balance Internal Convective Heat Gain Rate"}},
		{Kind: "heat.surface_convection", Label: "Surface convection", HeatCategory: "surface_envelope", Aliases: []string{"Zone Air Heat Balance Surface Convection Rate"}},
		{Kind: "heat.interzone_air", Label: "Interzone air transfer", HeatCategory: "air_exchange", Aliases: []string{"Zone Air Heat Balance Interzone Air Transfer Rate"}},
		{Kind: "heat.ventilation_outdoor_air", Label: "Outdoor air transfer", HeatCategory: "air_exchange", Aliases: []string{"Zone Air Heat Balance Outdoor Air Transfer Rate"}},
		{Kind: "heat.hvac_air_transfer", Label: "HVAC system air transfer", HeatCategory: "hvac_system", Aliases: []string{"Zone Air Heat Balance System Air Transfer Rate"}},
		{Kind: "heat.system_convective", Label: "HVAC/system convective gains", HeatCategory: "hvac_system", Aliases: []string{"Zone Air Heat Balance System Convective Heat Gain Rate"}},
		{Kind: "heat.fan_to_air", Label: "Fan heat to air", HeatCategory: "hvac_system", ObjectScoped: true, Aliases: []string{"Fan Air Heat Gain Energy", "Fan Air Heat Gain Rate"}},
		{Kind: "heat.storage_air", Label: "Air energy storage", HeatCategory: "storage_residual", Aliases: []string{"Zone Air Heat Balance Air Energy Storage Rate"}},
		{Kind: "heat.zone_balance_residual", Label: "Heat balance deviation", HeatCategory: "storage_residual", Aliases: []string{"Zone Air Heat Balance Deviation Rate"}},
		{Kind: "heat.people", Label: "People heat", HeatCategory: "internal_gains", Aliases: []string{"Zone People Total Heating Energy", "Zone People Total Heating Rate"}},
		{Kind: "heat.lighting", Label: "Lighting heat", HeatCategory: "internal_gains", Aliases: []string{"Zone Lights Total Heating Energy", "Zone Lights Total Heating Rate"}},
		{Kind: "heat.equipment", Label: "Equipment heat", HeatCategory: "internal_gains", Aliases: []string{"Zone Electric Equipment Total Heating Energy", "Zone Electric Equipment Total Heating Rate", "Zone Gas Equipment Total Heating Energy", "Zone Gas Equipment Total Heating Rate"}},
		{Kind: "heat.infiltration", Label: "Infiltration heat transfer", HeatCategory: "air_exchange", Aliases: []string{"Zone Infiltration Sensible Heat Loss Energy", "Zone Infiltration Sensible Heat Gain Energy", "Zone Infiltration Sensible Heat Loss Rate", "Zone Infiltration Sensible Heat Gain Rate"}},
		{Kind: "heat.ventilation", Label: "Ventilation heat transfer", HeatCategory: "air_exchange", Aliases: []string{"Zone Ventilation Sensible Heat Loss Energy", "Zone Ventilation Sensible Heat Gain Energy", "Zone Ventilation Sensible Heat Loss Rate", "Zone Ventilation Sensible Heat Gain Rate"}},
		{Kind: "heat.mixing", Label: "Mixing heat transfer", HeatCategory: "air_exchange", Aliases: []string{"Zone Mixing Sensible Heat Loss Energy", "Zone Mixing Sensible Heat Gain Energy", "Zone Mixing Sensible Heat Loss Rate", "Zone Mixing Sensible Heat Gain Rate"}},
	}
}

func energyMeterAliasDefinitionForName(name string) (energyMeterAliasDefinition, bool) {
	key := normalizeEnergyOutputName(name)
	for _, def := range energyMeterAliasCatalog() {
		for _, alias := range def.Aliases {
			if normalizeEnergyOutputName(alias) == key {
				return def, true
			}
		}
	}
	return energyMeterAliasDefinition{}, false
}

func energyLoadAliasDefinitionForName(name string) (energyLoadAliasDefinition, bool) {
	key := normalizeEnergyOutputName(name)
	for _, def := range energyLoadAliasCatalog() {
		for _, alias := range def.Aliases {
			if normalizeEnergyOutputName(alias) == key {
				return def, true
			}
		}
	}
	return energyLoadAliasDefinition{}, false
}

func energyHeatAliasDefinitionForName(name string) (energyHeatAliasDefinition, bool) {
	key := normalizeEnergyOutputName(name)
	for _, def := range energyHeatAliasCatalog() {
		for _, alias := range def.Aliases {
			if normalizeEnergyOutputName(alias) == key {
				return def, true
			}
		}
	}
	return energyHeatAliasDefinition{}, false
}

func energyHeatAliasExplicitSign(name string) string {
	key := normalizeEnergyOutputName(name)
	if strings.Contains(key, " sensible heat loss ") {
		return "negative"
	}
	if strings.Contains(key, " sensible heat gain ") {
		return "positive"
	}
	return ""
}

func energyHeatSignMultiplier(sign string) float64 {
	if sign == "negative" {
		return -1
	}
	return 1
}

func energyHeatAliasLabel(label string, sign string) string {
	label = strings.TrimSpace(label)
	switch sign {
	case "positive":
		return energyHeatAliasSignedLabel(label, "gain")
	case "negative":
		return energyHeatAliasSignedLabel(label, "loss")
	default:
		return label
	}
}

func energyHeatAliasSignedLabel(label string, suffix string) string {
	if label == "" {
		return suffix
	}
	if strings.Contains(label, "heat transfer") {
		return strings.Replace(label, "heat transfer", "heat "+suffix, 1)
	}
	return label + " " + suffix
}

func buildEnergyExplanationCompleteness(series []energyExplanationSeries, sources []EnergyDataSource, plan *PurposeRunPlan, mappedPercent float64) EnergyCompleteness {
	expectedEnergy := expectedEnergyExplanationOutputs(plan, "energy")
	expectedLoad := expectedEnergyExplanationOutputs(plan, "load")
	expectedHeat := expectedEnergyExplanationOutputs(plan, "heat")
	expectedEnergyGroups := expectedEnergyExplanationOutputGroups(expectedEnergy, "energy")
	expectedLoadGroups := expectedEnergyExplanationOutputGroups(expectedLoad, "load")
	expectedHeatGroups := expectedEnergyExplanationOutputGroups(expectedHeat, "heat")
	foundEnergyGroups := map[string]bool{}
	foundLoadGroups := map[string]bool{}
	foundHeatGroups := map[string]bool{}
	for _, item := range series {
		key := energyExplanationCompletenessGroupKey(item)
		if key == "" {
			continue
		}
		switch item.Level {
		case "energy":
			foundEnergyGroups[key] = true
		case "load":
			foundLoadGroups[key] = true
		case "heat":
			foundHeatGroups[key] = true
		}
	}
	foundEnergy := len(foundEnergyGroups)
	foundLoad := len(foundLoadGroups)
	foundHeat := len(foundHeatGroups)
	energyLevel := energyCompletenessLevel("energy", foundEnergy, maxInt(len(expectedEnergyGroups), foundEnergy), "Energy Use")
	loadLevel := energyCompletenessLevel("load", foundLoad, maxInt(len(expectedLoadGroups), foundLoad), "Delivered Load")
	heatLevel := energyCompletenessLevel("heat", foundHeat, maxInt(len(expectedHeatGroups), foundHeat), "Heat Drivers")
	status := "complete"
	if energyLevel.Status != "complete" || loadLevel.Status == "missing" || heatLevel.Status == "missing" {
		status = "partial"
	}
	if foundEnergy == 0 && foundLoad == 0 && foundHeat == 0 {
		status = "missing"
	}
	availability := make([]EnergySourceAvailabilityEntry, 0, len(expectedEnergy)+len(expectedLoad)+len(expectedHeat))
	availability = append(availability, sourceAvailabilityEntries(expectedEnergy, "energy", sources)...)
	availability = append(availability, sourceAvailabilityEntries(expectedLoad, "load", sources)...)
	availability = append(availability, sourceAvailabilityEntries(expectedHeat, "heat", sources)...)
	missingCategories := missingEnergySourceCategories(availability)
	return EnergyCompleteness{
		Status:             status,
		MappedPercent:      mappedPercent,
		EnergyUse:          energyLevel,
		DeliveredLoad:      loadLevel,
		HeatDrivers:        heatLevel,
		Items:              []EnergyCompletenessLevel{energyLevel, loadLevel, heatLevel},
		MissingCategories:  missingCategories,
		SourceAvailability: availability,
	}
}

func expectedEnergyExplanationOutputs(plan *PurposeRunPlan, level string) []string {
	out := []string{}
	if plan != nil {
		for _, object := range plan.OutputObjects {
			if !purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) {
				continue
			}
			switch level {
			case "energy":
				if strings.EqualFold(object.ObjectType, "Output:Meter") {
					out = appendUniquePurposeString(out, object.KeyValue)
				}
			case "load":
				if strings.EqualFold(object.ObjectType, "Output:Variable") {
					if _, ok := energyLoadAliasDefinitionForName(object.VariableName); ok {
						out = appendUniquePurposeString(out, object.VariableName)
					}
				}
			case "heat":
				if strings.EqualFold(object.ObjectType, "Output:Variable") {
					if _, ok := energyHeatAliasDefinitionForName(object.VariableName); ok {
						out = appendUniquePurposeString(out, object.VariableName)
					}
				}
			}
		}
	}
	if len(out) > 0 || level != "energy" {
		return out
	}
	for _, name := range energyFacilityMeterNames() {
		out = appendUniquePurposeString(out, name)
	}
	for _, name := range energyEndUseMeterNames() {
		out = appendUniquePurposeString(out, name)
	}
	return out
}

func expectedEnergyExplanationOutputGroups(names []string, level string) []string {
	out := []string{}
	for _, name := range names {
		if key := expectedEnergyExplanationOutputGroupKey(name, level); key != "" {
			out = appendUniquePurposeString(out, key)
		}
	}
	return out
}

func expectedEnergyExplanationOutputGroupKey(name string, level string) string {
	switch level {
	case "energy":
		if def, ok := energyMeterAliasDefinitionForName(name); ok {
			return strings.Join([]string{
				"energy",
				def.Kind,
				normalizeEnergyOutputName(def.Carrier),
				normalizeEnergyOutputName(def.EndUse),
			}, "|")
		}
	case "load":
		if def, ok := energyLoadAliasDefinitionForName(name); ok {
			return strings.Join([]string{
				"load",
				def.Kind,
				normalizeEnergyOutputName(def.ServiceKind),
			}, "|")
		}
	case "heat":
		if def, ok := energyHeatAliasDefinitionForName(name); ok {
			return strings.Join([]string{
				"heat",
				def.Kind,
				normalizeEnergyOutputName(def.HeatCategory),
			}, "|")
		}
	}
	return strings.Join([]string{level, normalizeEnergyOutputName(name)}, "|")
}

func energyExplanationCompletenessGroupKey(item energyExplanationSeries) string {
	switch item.Level {
	case "energy":
		return strings.Join([]string{
			"energy",
			item.Kind,
			normalizeEnergyOutputName(item.Carrier),
			normalizeEnergyOutputName(item.EndUse),
		}, "|")
	case "load":
		return strings.Join([]string{
			"load",
			item.Kind,
			normalizeEnergyOutputName(item.ServiceKind),
		}, "|")
	case "heat":
		return strings.Join([]string{
			"heat",
			item.Kind,
			normalizeEnergyOutputName(item.HeatCategory),
		}, "|")
	default:
		return ""
	}
}

func energyCompletenessLevel(level string, found int, total int, label string) EnergyCompletenessLevel {
	status := "complete"
	if total == 0 {
		status = "not_applicable"
	} else if found == 0 {
		status = "missing"
	} else if found < total {
		status = "partial"
	}
	message := fmt.Sprintf("%s: %d/%d source group(s) available", label, found, total)
	if level == "heat" && found == 0 {
		message = "Heat Drivers need Zone Heat Flow or explanation heat-balance outputs."
	}
	return EnergyCompletenessLevel{
		Level:   level,
		Status:  status,
		Found:   found,
		Total:   total,
		Message: message,
	}
}

func sourceAvailabilityEntries(expected []string, level string, sources []EnergyDataSource) []EnergySourceAvailabilityEntry {
	out := make([]EnergySourceAvailabilityEntry, 0, len(expected))
	for _, name := range expected {
		status := "missing"
		for _, source := range sources {
			if strings.EqualFold(source.Name, name) || strings.EqualFold(source.KeyValue, name) {
				status = "found"
				break
			}
		}
		out = append(out, EnergySourceAvailabilityEntry{Name: name, Level: level, Status: status})
	}
	return out
}

func missingEnergySourceCategories(availability []EnergySourceAvailabilityEntry) []string {
	out := []string{}
	for _, item := range availability {
		if item.Status != "missing" {
			continue
		}
		out = appendUniquePurposeString(out, strings.TrimSpace(item.Level+": "+item.Name))
	}
	return out
}

func energyExplanationEnergyNodeID(item energyExplanationSeries) string {
	if strings.HasSuffix(item.Kind, ".total") {
		return "energy.carrier." + item.Carrier
	}
	return "energy.end_use." + item.EndUse + "." + item.Carrier
}

func energyExplanationIsProductionEndUse(item energyExplanationSeries) bool {
	return item.Level == "energy" && (item.EndUse == "generators" || item.Kind == "energy.generators")
}

func energyExplanationLoadNodeID(item energyExplanationSeries) string {
	nodeID := "load." + item.ServiceKind
	if item.ServiceKind == "" {
		nodeID = item.Kind
	}
	if suffix := energyExplanationZoneSuffix(item.ZoneName); suffix != "" {
		nodeID += "." + suffix
	} else if suffix := energyExplanationZoneSuffix(item.LoopName); suffix != "" {
		nodeID += "." + suffix
	}
	return nodeID
}

func energyExplanationHeatNodeID(item energyExplanationSeries) string {
	nodeID := item.Kind
	if sign := strings.TrimSpace(item.HeatSign); sign != "" {
		nodeID += "." + metricID(sign)
	}
	if suffix := energyExplanationZoneSuffix(item.ZoneName); suffix != "" {
		nodeID += "." + suffix
	}
	return nodeID
}

func energyExplanationZoneSuffix(zoneName string) string {
	zoneName = strings.TrimSpace(zoneName)
	if zoneName == "" || zoneName == "*" {
		return ""
	}
	return metricID(zoneName)
}

func energyExplanationZoneServiceKey(zoneName string, serviceKind string) string {
	return energyExplanationZoneSuffix(zoneName) + "|" + strings.TrimSpace(serviceKind)
}

func splitEnergyExplanationZoneServiceKey(key string) (string, string) {
	zoneName, serviceKind, ok := strings.Cut(key, "|")
	if !ok {
		return "", ""
	}
	return strings.TrimSpace(zoneName), strings.TrimSpace(serviceKind)
}

func energyCarrierLabel(carrier string) string {
	switch carrier {
	case "electricity":
		return "Electricity"
	case "natural_gas":
		return "Natural gas"
	case "district_cooling":
		return "District cooling"
	case "district_heating":
		return "District heating"
	case "fuel_oil_1":
		return "Fuel oil #1"
	case "fuel_oil_2":
		return "Fuel oil #2"
	case "other_fuel_1":
		return "Other fuel 1"
	case "other_fuel_2":
		return "Other fuel 2"
	default:
		return strings.Title(strings.ReplaceAll(carrier, "_", " "))
	}
}

func energyServiceLabel(serviceKind string) string {
	switch serviceKind {
	case "cooling":
		return "Cooling"
	case "heating":
		return "Heating"
	default:
		return strings.Title(strings.ReplaceAll(serviceKind, "_", " "))
	}
}

func energyResidualVisibilityThreshold(reference float64) float64 {
	return math.Max(0.001, math.Abs(reference)*0.001)
}

func firstExistingNodeID(nodes map[string]*energyExplanationNodeAccumulator, ids ...string) string {
	for _, id := range ids {
		if nodes[id] != nil {
			return id
		}
	}
	return ""
}

func firstLoadNodeIDForHeat(nodes map[string]*energyExplanationNodeAccumulator, loadNodesByZoneService map[string][]string, zoneName string, serviceKind string) string {
	if zoneName != "" {
		if id := firstExistingNodeID(nodes, loadNodesByZoneService[energyExplanationZoneServiceKey(zoneName, serviceKind)]...); id != "" {
			return id
		}
	}
	return firstExistingNodeID(nodes, "load."+serviceKind)
}

func energyExplanationMonths(series []energyExplanationSeries) []int {
	seen := map[int]bool{}
	for _, item := range series {
		for month, value := range item.Monthly {
			if value == 0 {
				continue
			}
			seen[month] = true
		}
	}
	months := make([]int, 0, len(seen))
	for month := range seen {
		months = append(months, month)
	}
	sort.Ints(months)
	return months
}

func energyExplanationDays(series []energyExplanationSeries) []int {
	seen := map[int]bool{}
	for _, item := range series {
		for day, value := range item.Daily {
			if value == 0 {
				continue
			}
			seen[day] = true
		}
	}
	days := make([]int, 0, len(seen))
	for day := range seen {
		days = append(days, day)
	}
	sort.Ints(days)
	return days
}

func roundedEnergyExplanationMonthly(monthly map[int]float64) map[int]float64 {
	if len(monthly) == 0 {
		return nil
	}
	out := map[int]float64{}
	for month, value := range monthly {
		out[month] = roundedEnergyNumber(value)
	}
	return out
}

func roundedEnergyExplanationDaily(daily map[int]float64) map[int]float64 {
	if len(daily) == 0 {
		return nil
	}
	out := map[int]float64{}
	for day, value := range daily {
		out[day] = roundedEnergyNumber(value)
	}
	return out
}

func energyExplanationMonthFromPoint(point SimulationPoint) (int, bool) {
	label := strings.TrimPrefix(strings.TrimSpace(point.Label), "M")
	if label == "" {
		return 0, false
	}
	month, err := strconv.Atoi(label)
	return month, err == nil && month >= 1 && month <= 12
}

func sortEnergyExplanationNodes(nodes []EnergyExplanationNode) {
	levelOrder := map[string]int{"energy": 0, "load": 1, "heat": 2, "residual": 3, "support": 4}
	sort.SliceStable(nodes, func(i, j int) bool {
		if levelOrder[nodes[i].Level] != levelOrder[nodes[j].Level] {
			return levelOrder[nodes[i].Level] < levelOrder[nodes[j].Level]
		}
		return nodes[i].ID < nodes[j].ID
	})
}

func sortEnergyExplanationEdges(edges []EnergyExplanationEdge) {
	sort.SliceStable(edges, func(i, j int) bool {
		return edges[i].ID < edges[j].ID
	})
}

func edgeID(prefix string, period string, fromID string, toID string) string {
	return prefix + "." + period + "." + metricID(fromID) + "." + metricID(toID)
}

func appendUniqueStrings(values []string, next ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values)+len(next))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	for _, value := range next {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func sqlCastTextColumnExpr(columns map[string]string, name string, fallback string) string {
	actual := columns[normalizeSQLColumnName(name)]
	if actual == "" {
		return fallback
	}
	return "COALESCE(CAST(" + quoteSQLiteIdentifier(actual) + " AS TEXT), '')"
}

func parseSQLBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
