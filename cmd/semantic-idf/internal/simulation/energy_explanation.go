package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const energyExplanationV1Schema = "semantic-idf.energy-explanation/v1"
const energyExplanationSchema = "semantic-idf.energy-explanation/v2"
const energyExplanationSummarySchema = "semantic-idf.energy-explanation-summary/v2"
const maxEnergyExplanationTabularRows = 1200

// EnergyExplanationV1 is the read-only compatibility shape used by stored
// manifests and by the legacy SQL graph assembler. Runtime responses are
// upgraded to EnergyExplanationResult before they leave this package.
type EnergyExplanationV1 struct {
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

	scope                             EnergyExplanationScope
	canonicalMonthlyBasis             bool
	zoneDirectUseSeries               []energyExplanationSeries
	buildingHVACAllocationEdges       []EnergyExplanationEdge
	buildingHVACAllocationPeriodEdges map[string][]EnergyExplanationEdge
	servicePathIndex                  energyServicePathIndex
}

type EnergyExplanationScope struct {
	Kind             string `json:"kind"`
	ZoneName         string `json:"zoneName,omitempty"`
	AggregationBasis string `json:"aggregationBasis"`
}

type EnergyExplanationResult struct {
	Schema            string                         `json:"schema"`
	Purpose           string                         `json:"purpose"`
	Scope             EnergyExplanationScope         `json:"scope"`
	Frequency         string                         `json:"frequency"`
	AllocationPolicy  string                         `json:"allocationPolicy,omitempty"`
	RelationshipRules []EnergyRelationshipRule       `json:"relationshipRules,omitempty"`
	Periods           []EnergyPeriod                 `json:"periods,omitempty"`
	Nodes             []EnergyExplanationNode        `json:"nodes"`
	Links             []EnergyPathLink               `json:"links"`
	Reconciliation    []EnergyReconciliation         `json:"reconciliation,omitempty"`
	Sources           []EnergyDataSource             `json:"sources,omitempty"`
	Completeness      EnergyCompleteness             `json:"completeness"`
	Quality           *EnergyPathQuality             `json:"quality,omitempty"`
	Warnings          []EnergyWarning                `json:"warnings,omitempty"`
	ZoneContributions []EnergyExplanationSummaryItem `json:"zoneContributions,omitempty"`
	AvailableZones    []string                       `json:"availableZones,omitempty"`
	ZoneResults       []EnergyExplanationZoneResult  `json:"zoneResults,omitempty"`

	// Edges is retained for one Go-level compatibility window. It is accepted
	// on read through UnmarshalJSON, but it is never emitted in v2 JSON.
	Edges []EnergyExplanationEdge `json:"-"`

	legacyNodes     []EnergyExplanationNode
	upgradedFromV1  bool
	sanitizedOnRead bool
}

type EnergyExplanationZoneResult struct {
	Scope             EnergyExplanationScope         `json:"scope"`
	Summary           EnergyExplanationSummary       `json:"summary"`
	Completeness      EnergyCompleteness             `json:"completeness"`
	Quality           *EnergyPathQuality             `json:"quality,omitempty"`
	Periods           []EnergyPeriod                 `json:"periods,omitempty"`
	Nodes             []EnergyExplanationNode        `json:"nodes"`
	Links             []EnergyPathLink               `json:"links"`
	Reconciliation    []EnergyReconciliation         `json:"reconciliation,omitempty"`
	Warnings          []EnergyWarning                `json:"warnings,omitempty"`
	ZoneContributions []EnergyExplanationSummaryItem `json:"zoneContributions,omitempty"`
}

type EnergyPeriod struct {
	ID                string                         `json:"id"`
	Label             string                         `json:"label"`
	Kind              string                         `json:"kind"`
	Summary           *EnergyExplanationSummary      `json:"summary,omitempty"`
	Quality           *EnergyPathQuality             `json:"quality,omitempty"`
	Nodes             []EnergyExplanationNode        `json:"nodes,omitempty"`
	Links             []EnergyPathLink               `json:"links,omitempty"`
	Reconciliation    []EnergyReconciliation         `json:"reconciliation,omitempty"`
	Warnings          []EnergyWarning                `json:"warnings,omitempty"`
	ZoneContributions []EnergyExplanationSummaryItem `json:"zoneContributions,omitempty"`

	// Deprecated read-only compatibility collection. MarshalJSON deliberately
	// omits it so new manifests cannot regress to the v1 edge contract.
	Edges []EnergyExplanationEdge `json:"-"`
}

type EnergyExplanationSummary struct {
	Schema       string                         `json:"schema,omitempty"`
	Period       string                         `json:"period,omitempty"`
	Scope        EnergyExplanationScope         `json:"scope"`
	Drivers      []EnergyExplanationSummaryItem `json:"drivers,omitempty"`
	Loads        []EnergyExplanationSummaryItem `json:"loads,omitempty"`
	EndUses      []EnergyExplanationSummaryItem `json:"endUses,omitempty"`
	Carriers     []EnergyExplanationSummaryItem `json:"carriers,omitempty"`
	Ratios       []EnergyExplanationSummaryItem `json:"ratios,omitempty"`
	Residuals    []EnergyExplanationSummaryItem `json:"residuals,omitempty"`
	TopZones     []EnergyExplanationSummaryItem `json:"topZones,omitempty"`
	Completeness EnergyCompleteness             `json:"completeness,omitempty"`
	Quality      *EnergyPathQuality             `json:"quality,omitempty"`

	// Deprecated Go aliases keep older callers source-compatible without
	// serializing the removed v1 summary keys.
	AllocationPolicy       string                         `json:"-"`
	EnergyByCarrier        []EnergyExplanationSummaryItem `json:"-"`
	EnergyByEndUse         []EnergyExplanationSummaryItem `json:"-"`
	DeliveredLoadByService []EnergyExplanationSummaryItem `json:"-"`
	DerivedKPIs            []EnergyExplanationSummaryItem `json:"-"`
	HeatDrivers            []EnergyExplanationSummaryItem `json:"-"`
	TopHeatDrivers         []EnergyExplanationSummaryItem `json:"-"`
}

// EnergyPathQuality separates run-level output availability from period-local
// accounting. Conversion ratios describe availability, never thermal/site
// conservation. Status fields distinguish a zero result from no denominator.
type EnergyPathQuality struct {
	Drivers                  EnergyCompletenessLevel `json:"drivers"`
	Loads                    EnergyCompletenessLevel `json:"loads"`
	EndUses                  EnergyCompletenessLevel `json:"endUses"`
	Carriers                 EnergyCompletenessLevel `json:"carriers"`
	Ratios                   EnergyCompletenessLevel `json:"ratios"`
	DriverToLoadClosedPct    float64                 `json:"driverToLoadClosedPct"`
	EndUseToCarrierClosedPct float64                 `json:"endUseToCarrierClosedPct"`
	ZoneAllocatedPct         float64                 `json:"zoneAllocatedPct"`
	UnassignedPct            float64                 `json:"unassignedPct"`
	DriverToLoadStatus       string                  `json:"driverToLoadStatus"`
	EndUseToCarrierStatus    string                  `json:"endUseToCarrierStatus"`
	ZoneAllocationStatus     string                  `json:"zoneAllocationStatus"`
}

type EnergyExplanationSummaryItem struct {
	ID                  string   `json:"id"`
	Level               string   `json:"level,omitempty"`
	Kind                string   `json:"kind,omitempty"`
	Label               string   `json:"label"`
	Value               float64  `json:"value"`
	RawValue            float64  `json:"rawValue,omitempty"`
	AllocatedValue      float64  `json:"allocatedValue,omitempty"`
	Unit                string   `json:"unit,omitempty"`
	ScaleDomain         string   `json:"scaleDomain,omitempty"`
	AggregationBasis    string   `json:"aggregationBasis,omitempty"`
	ZoneName            string   `json:"zoneName,omitempty"`
	ServiceKind         string   `json:"serviceKind,omitempty"`
	PathType            string   `json:"pathType,omitempty"`
	Carrier             string   `json:"carrier,omitempty"`
	EndUse              string   `json:"endUse,omitempty"`
	MeterHierarchyLevel string   `json:"meterHierarchyLevel,omitempty"`
	HeatCategory        string   `json:"heatCategory,omitempty"`
	Sign                string   `json:"sign,omitempty"`
	Basis               string   `json:"basis,omitempty"`
	Formula             string   `json:"formula,omitempty"`
	NumeratorLabel      string   `json:"numeratorLabel,omitempty"`
	NumeratorValue      float64  `json:"numeratorValue,omitempty"`
	NumeratorUnit       string   `json:"numeratorUnit,omitempty"`
	DenominatorLabel    string   `json:"denominatorLabel,omitempty"`
	DenominatorValue    float64  `json:"denominatorValue,omitempty"`
	DenominatorUnit     string   `json:"denominatorUnit,omitempty"`
	SourceIDs           []string `json:"sourceIds,omitempty"`
}

type EnergyExplanationNode struct {
	ID                    string                             `json:"id"`
	Level                 string                             `json:"level"`
	Kind                  string                             `json:"kind"`
	Label                 string                             `json:"label"`
	Value                 float64                            `json:"value"`
	SignedValue           float64                            `json:"signedValue,omitempty"`
	RawValue              float64                            `json:"rawValue,omitempty"`
	EffectiveValue        float64                            `json:"effectiveValue,omitempty"`
	AllocatedValue        float64                            `json:"allocatedValue,omitempty"`
	AllocationApplied     bool                               `json:"allocationApplied,omitempty"`
	AllocationExplanation string                             `json:"allocationExplanation,omitempty"`
	DisplayValue          float64                            `json:"displayValue,omitempty"`
	Unit                  string                             `json:"unit"`
	ScaleDomain           string                             `json:"scaleDomain,omitempty"`
	Period                string                             `json:"period,omitempty"`
	ZoneName              string                             `json:"zoneName,omitempty"`
	ServiceKind           string                             `json:"serviceKind,omitempty"`
	Carrier               string                             `json:"carrier,omitempty"`
	EndUse                string                             `json:"endUse,omitempty"`
	DriverCategory        string                             `json:"driverCategory,omitempty"`
	ThermalComponent      string                             `json:"thermalComponent,omitempty"`
	LoadBreakdown         []EnergyExplanationLoadComponent   `json:"loadBreakdown,omitempty"`
	OffsetEffects         []EnergyExplanationOffsetEffect    `json:"offsetEffects,omitempty"`
	SimultaneousLoad      *EnergyExplanationSimultaneousLoad `json:"simultaneousLoad,omitempty"`
	LatentShare           float64                            `json:"latentShare,omitempty"`
	Badges                []string                           `json:"badges,omitempty"`
	Basis                 string                             `json:"basis,omitempty"`
	AggregationBasis      string                             `json:"aggregationBasis,omitempty"`
	Multiplier            float64                            `json:"multiplier,omitempty"`
	RelatedPathIDs        []string                           `json:"relatedPathIds,omitempty"`
	RelatedEntityIDs      []string                           `json:"relatedEntityIds,omitempty"`
	SourceIDs             []string                           `json:"sourceIds,omitempty"`

	// Legacy metadata remains readable while v1 payloads are upgraded.
	LoopName            string `json:"loopName,omitempty"`
	PathType            string `json:"pathType,omitempty"`
	MeterHierarchyLevel string `json:"meterHierarchyLevel,omitempty"`
	HeatCategory        string `json:"heatCategory,omitempty"`
	Sign                string `json:"sign,omitempty"`

	// driverZoneOnly is an internal projection guard. Zone aggregate air
	// transfer variables are useful in a Zone inspector, but must not be
	// promoted to a Building main-flow ribbon without pairwise provenance.
	driverZoneOnly                bool
	driverBuildingOnly            bool
	allocationSourceIDs           []string
	simultaneousLoadContributions []energyExplanationSimultaneousLoadContribution
	endUseCarriers                []string
}

// EnergyExplanationLoadComponent keeps sensible and latent delivery inside a
// single primary Cooling or Heating node. Values are period-local effective
// contributions, so a monthly badge never depends on annual source metadata.
type EnergyExplanationLoadComponent struct {
	Component string   `json:"component"`
	Value     float64  `json:"value"`
	Share     float64  `json:"share,omitempty"`
	Unit      string   `json:"unit,omitempty"`
	SourceIDs []string `json:"sourceIds,omitempty"`
}

// EnergyExplanationOffsetEffect is diagnostic heat-balance context. It
// describes pressure that offsets the opposite service and is never promoted
// to a reverse main-flow ribbon.
type EnergyExplanationOffsetEffect struct {
	EffectKind     string   `json:"effectKind"`
	TargetService  string   `json:"targetService"`
	DriverCategory string   `json:"driverCategory,omitempty"`
	Label          string   `json:"label,omitempty"`
	HeatDirection  string   `json:"heatDirection"`
	RawValue       float64  `json:"rawValue"`
	EffectiveValue float64  `json:"effectiveValue"`
	Unit           string   `json:"unit,omitempty"`
	Basis          string   `json:"basis"`
	Explanation    string   `json:"explanation"`
	SourceIDs      []string `json:"sourceIds,omitempty"`
}

// EnergyExplanationSimultaneousLoad is a bounded scope-period diagnostic.
// Numerator and denominator are sums of already completed zone-month pairs,
// so annual and Building views cannot manufacture overlap by netting zones.
type EnergyExplanationSimultaneousLoad struct {
	Available   bool     `json:"available"`
	Numerator   float64  `json:"numerator"`
	Denominator float64  `json:"denominator"`
	Ratio       float64  `json:"ratio"`
	Unit        string   `json:"unit,omitempty"`
	Basis       string   `json:"basis"`
	SourceIDs   []string `json:"sourceIds,omitempty"`
}

type energyExplanationSimultaneousLoadContribution struct {
	Key         string
	Numerator   float64
	Denominator float64
	Unit        string
	SourceIDs   []string
}

type EnergyPathLink struct {
	ID             string   `json:"id"`
	FromID         string   `json:"fromId"`
	ToID           string   `json:"toId"`
	Relation       string   `json:"relation"`
	Basis          string   `json:"basis"`
	RuleID         string   `json:"ruleId,omitempty"`
	Explanation    string   `json:"explanation,omitempty"`
	FromValue      float64  `json:"fromValue"`
	FromUnit       string   `json:"fromUnit"`
	ToValue        float64  `json:"toValue"`
	ToUnit         string   `json:"toUnit"`
	Ratio          float64  `json:"ratio,omitempty"`
	RatioKind      string   `json:"ratioKind,omitempty"`
	RatioLabel     string   `json:"ratioLabel,omitempty"`
	Period         string   `json:"period,omitempty"`
	ZoneName       string   `json:"zoneName,omitempty"`
	ServiceKind    string   `json:"serviceKind,omitempty"`
	RelatedPathIDs []string `json:"relatedPathIds,omitempty"`
	SourceIDs      []string `json:"sourceIds,omitempty"`
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
	ID                    string                        `json:"id"`
	SourceType            string                        `json:"sourceType"`
	IsMeter               bool                          `json:"isMeter,omitempty"`
	KeyValue              string                        `json:"keyValue,omitempty"`
	Name                  string                        `json:"name,omitempty"`
	Units                 string                        `json:"units,omitempty"`
	SourceUnit            string                        `json:"sourceUnit,omitempty"`
	NormalizedUnit        string                        `json:"normalizedUnit,omitempty"`
	ReportingFrequency    string                        `json:"reportingFrequency,omitempty"`
	AggregationMethod     string                        `json:"aggregationMethod,omitempty"`
	IndexGroup            string                        `json:"indexGroup,omitempty"`
	TableName             string                        `json:"tableName,omitempty"`
	RowName               string                        `json:"rowName,omitempty"`
	ColumnName            string                        `json:"columnName,omitempty"`
	ZoneName              string                        `json:"zoneName,omitempty"`
	ObjectIndex           *int                          `json:"objectIndex,omitempty"`
	RawValue              float64                       `json:"rawValue,omitempty"`
	EffectiveValue        float64                       `json:"effectiveValue,omitempty"`
	EffectiveMultiplier   float64                       `json:"effectiveMultiplier,omitempty"`
	MultiplierApplication string                        `json:"multiplierApplication,omitempty"`
	AllocationFactor      float64                       `json:"allocationFactor,omitempty"`
	AllocatedValue        float64                       `json:"allocatedValue,omitempty"`
	AllocationApplied     bool                          `json:"allocationApplied,omitempty"`
	AllocationExplanation string                        `json:"allocationExplanation,omitempty"`
	AllocationFormula     string                        `json:"allocationFormula,omitempty"`
	AggregationBasis      string                        `json:"aggregationBasis,omitempty"`
	DriverRole            string                        `json:"driverRole,omitempty"`
	DriverCategory        string                        `json:"driverCategory,omitempty"`
	DriverComponent       string                        `json:"driverComponent,omitempty"`
	HeatDirection         string                        `json:"heatDirection,omitempty"`
	InspectorSection      string                        `json:"inspectorSection,omitempty"`
	Explanation           string                        `json:"explanation,omitempty"`
	Formula               string                        `json:"formula,omitempty"`
	InputSourceIDs        []string                      `json:"inputSourceIds,omitempty"`
	RelatedEntityIDs      []string                      `json:"relatedEntityIds,omitempty"`
	ScopeDetails          []EnergyDataSourceScopeDetail `json:"scopeDetails,omitempty"`
}

type EnergyDataSourceScopeDetail struct {
	Scope                 EnergyExplanationScope `json:"scope"`
	RawValue              float64                `json:"rawValue,omitempty"`
	EffectiveValue        float64                `json:"effectiveValue,omitempty"`
	EffectiveMultiplier   float64                `json:"effectiveMultiplier,omitempty"`
	MultiplierApplication string                 `json:"multiplierApplication,omitempty"`
	AllocationFactor      float64                `json:"allocationFactor,omitempty"`
	AllocatedValue        float64                `json:"allocatedValue,omitempty"`
	AllocationApplied     bool                   `json:"allocationApplied,omitempty"`
	AggregationBasis      string                 `json:"aggregationBasis,omitempty"`
}

type EnergyReconciliation struct {
	ID               string   `json:"id"`
	Level            string   `json:"level"`
	Period           string   `json:"period"`
	Label            string   `json:"label"`
	Status           string   `json:"status,omitempty"`
	ZoneName         string   `json:"zoneName,omitempty"`
	ServiceKind      string   `json:"serviceKind,omitempty"`
	ExpectedValue    float64  `json:"expectedValue"`
	ExplainedValue   float64  `json:"explainedValue"`
	ResidualValue    float64  `json:"residualValue"`
	DirectValue      float64  `json:"directValue,omitempty"`
	AllocatedValue   float64  `json:"allocatedValue,omitempty"`
	UnassignedValue  float64  `json:"unassignedValue,omitempty"`
	OvermappedValue  float64  `json:"overmappedValue,omitempty"`
	AllocationMethod string   `json:"allocationMethod,omitempty"`
	Unit             string   `json:"unit"`
	Basis            string   `json:"basis"`
	Formula          string   `json:"formula,omitempty"`
	SourceIDs        []string `json:"sourceIds,omitempty"`
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
	Name      string   `json:"name"`
	Level     string   `json:"level"`
	Status    string   `json:"status"`
	SourceIDs []string `json:"sourceIds,omitempty"`
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
	energyRelationshipRuleMeterEndUse                   = "meter.end_use"
	energyRelationshipRuleMeasuredEnergyVariable        = "energy.measured_variable"
	energyRelationshipRuleMeasuredLoad                  = "load.measured_variable"
	energyRelationshipRuleAllocatedZoneLoad             = "allocation.by_zone_load_share"
	energyRelationshipRuleAllocatedServicePathLoad      = "allocation.by_service_path_load_share"
	energyRelationshipRuleAllocatedAuxiliaryServicePath = "allocation.by_auxiliary_service_path_share"
	energyRelationshipRuleHeatDriverBalance             = "heat.driver_balance"
	energyRelationshipRuleInternalGainHeat              = "heat.internal_gain_energy"
	energyRelationshipRulePurchasedElectricity          = "support.purchased_electricity"
	energyRelationshipRuleOnsiteProduction              = "support.onsite_production"
	energyRelationshipRuleStorageDischarge              = "support.storage_discharge"
	energyRelationshipRuleSoldElectricity               = "support.sold_electricity"
	energyRelationshipRuleEnergyResidual                = "residual.energy_total"
	energyRelationshipRuleHeatResidual                  = "residual.heat_driver_balance"
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

// energyCarrierTaxonomyDefinition is the single presentation and identity
// contract for recognized Energy Path carriers. Water is intentionally part
// of the resource taxonomy, but its native unit belongs to the context domain;
// it is not site energy unless an explicit conversion proves otherwise.
type energyCarrierTaxonomyDefinition struct {
	Token       string
	Label       string
	Unit        string
	ScaleDomain string
}

func energyCarrierTaxonomy() []energyCarrierTaxonomyDefinition {
	return []energyCarrierTaxonomyDefinition{
		{Token: "electricity", Label: "Electricity", Unit: "kWh", ScaleDomain: "site"},
		{Token: "natural_gas", Label: "Natural gas", Unit: "kWh", ScaleDomain: "site"},
		{Token: "district_cooling", Label: "District cooling", Unit: "kWh", ScaleDomain: "site"},
		{Token: "district_heating", Label: "District heating", Unit: "kWh", ScaleDomain: "site"},
		{Token: "steam", Label: "Steam", Unit: "kWh", ScaleDomain: "site"},
		{Token: "propane", Label: "Propane", Unit: "kWh", ScaleDomain: "site"},
		{Token: "fuel_oil_1", Label: "Fuel oil #1", Unit: "kWh", ScaleDomain: "site"},
		{Token: "fuel_oil_2", Label: "Fuel oil #2", Unit: "kWh", ScaleDomain: "site"},
		{Token: "coal", Label: "Coal", Unit: "kWh", ScaleDomain: "site"},
		{Token: "diesel", Label: "Diesel", Unit: "kWh", ScaleDomain: "site"},
		{Token: "gasoline", Label: "Gasoline", Unit: "kWh", ScaleDomain: "site"},
		{Token: "other_fuel_1", Label: "Other fuel 1", Unit: "kWh", ScaleDomain: "site"},
		{Token: "other_fuel_2", Label: "Other fuel 2", Unit: "kWh", ScaleDomain: "site"},
		{Token: "water", Label: "Water", Unit: "m3", ScaleDomain: "context"},
	}
}

func energyCarrierTaxonomyDefinitionFor(value string) (energyCarrierTaxonomyDefinition, bool) {
	compact := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return -1
	}, strings.TrimSpace(value))
	aliases := map[string]string{
		"electricity":          "electricity",
		"naturalgas":           "natural_gas",
		"gas":                  "natural_gas",
		"districtcooling":      "district_cooling",
		"districtheating":      "district_heating",
		"districtheatingwater": "district_heating",
		"steam":                "steam",
		"districtheatingsteam": "steam",
		"propane":              "propane",
		"fueloil1":             "fuel_oil_1",
		"fueloilno1":           "fuel_oil_1",
		"fueloil2":             "fuel_oil_2",
		"fueloilno2":           "fuel_oil_2",
		"coal":                 "coal",
		"diesel":               "diesel",
		"gasoline":             "gasoline",
		"otherfuel1":           "other_fuel_1",
		"otherfuel2":           "other_fuel_2",
		"water":                "water",
	}
	token, ok := aliases[compact]
	if !ok {
		return energyCarrierTaxonomyDefinition{}, false
	}
	for _, definition := range energyCarrierTaxonomy() {
		if definition.Token == token {
			return definition, true
		}
	}
	return energyCarrierTaxonomyDefinition{}, false
}

type energyLoadAliasDefinition struct {
	Kind           string
	Label          string
	ServiceKind    string
	Scope          string
	EnergyPathOnly bool
	Aliases        []string
	LegacyAliases  []string
}

type energyHeatAliasDefinition struct {
	Kind                 string
	Label                string
	HeatCategory         string
	ObjectScoped         bool
	SurfaceScoped        bool
	Aliases              []string
	OutputRequestAliases []string
}

type energyExplanationDictionary struct {
	row                sqlOutputDictionaryRow
	isMeter            bool
	reportingFrequency string
	indexGroup         string
	sourceFile         string
	meter              *energyMeterAliasDefinition
	energy             *energyMeterAliasDefinition
	load               *energyLoadAliasDefinition
	heat               *energyHeatAliasDefinition
}

type energyExplanationSeriesBuilder struct {
	dictionary       energyExplanationDictionary
	unit             string
	total            float64
	monthly          map[int]float64
	daily            map[int]float64
	hourly           map[int]float64
	selectedRange    float64
	hasSelectedRange bool
}

type energyExplanationCategorySeriesBuilder struct {
	seriesBuilder          energyExplanationSeriesBuilder
	category               energySurfaceCategory
	sourceIDs              []string
	annualSourceIDs        []string
	monthlySourceIDs       []string
	dailySourceIDs         []string
	hourlySourceIDs        []string
	selectedRangeSourceIDs []string
}

type energySurfacePeriodSelection struct {
	category      energySurfaceCategory
	annual        bool
	monthly       bool
	daily         bool
	hourly        bool
	selectedRange bool
}

type energyExplanationSeries struct {
	// Canonical identity is populated at the SQL/Tabular boundary. The legacy
	// fields below remain while the v1 assembler is kept as an adapter.
	Stage                  string
	CanonicalKind          string
	Level                  string
	Kind                   string
	Label                  string
	Unit                   string
	Carrier                string
	EndUse                 string
	MeterHierarchyLevel    string
	ServiceKind            string
	PathType               string
	ZoneName               string
	SurfaceName            string
	LoopName               string
	HeatCategory           string
	ThermalComponent       string
	DriverCategory         string
	DriverSourceRole       string
	DriverExplanation      string
	DriverComponent        string
	DriverFormula          string
	DriverInputSourceIDs   []string
	SurfaceScoped          bool
	Sign                   string
	HeatSign               string
	Basis                  string
	SourceKey              string
	SourceName             string
	SourceClass            string
	CanonicalFamily        string
	SourceFamily           string
	PeriodBasis            string
	SourceIDs              []string
	AnnualSourceIDs        []string
	MonthlySourceIDs       []string
	DailySourceIDs         []string
	HourlySourceIDs        []string
	SelectedRangeSourceIDs []string
	RelatedEntityIDs       []string
	RawTotal               float64
	RawMonthly             map[int]float64
	RawDaily               map[int]float64
	RawHourly              map[int]float64
	RawSelectedRange       float64
	Total                  float64
	Monthly                map[int]float64
	Daily                  map[int]float64
	Hourly                 map[int]float64
	SelectedRange          float64
	HasSelectedRange       bool
	EffectiveMultiplier    float64
	MultiplierApplication  string
	multiplierApplied      bool

	sourceKeyValue         string
	sourceName             string
	sourceFrequency        string
	sourceIsRate           bool
	sourcePriority         int
	heatSignMultiplier     float64
	parseCategorySource    bool
	parseCategoryAggregate bool
	driverZoneOnly         bool
	driverBuildingOnly     bool
	interzonePairID        string
	canonicalLoadMetadata  bool
	loadBreakdown          []energyLoadBreakdownSeries
}

// energyExplanationParseResult is deliberately graph-free. SQL and tabular
// readers stop at canonical series; node/link direction belongs to the graph
// builder that consumes this value.
type energyExplanationParseResult struct {
	Series  []energyExplanationSeries
	Sources []EnergyDataSource
}

type energyExplanationInternalGainTarget struct {
	endUse  string
	carrier string
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

func buildEnergyExplanationResultFromFiles(files []SimulationFileInfo, dashboard EnergyDashboardResult, plan *PurposeRunPlan) EnergyExplanationV1 {
	return buildEnergyExplanationResultFromFilesWithDriverContext(files, dashboard, plan, energyDriverBuildContext{})
}

func buildEnergyExplanationResultFromFilesWithDriverContext(files []SimulationFileInfo, dashboard EnergyDashboardResult, plan *PurposeRunPlan, driverContext energyDriverBuildContext) EnergyExplanationV1 {
	for _, file := range files {
		if file.Kind != "sqlite" {
			continue
		}
		result, err := parseSimulationEnergyExplanationSQLWithDriverContext(file.Path, plan, driverContext)
		if err == nil && (len(result.Nodes) > 0 || len(result.Periods) > 0 || len(result.Sources) > 0) {
			return result
		}
	}
	return buildEnergyExplanationFromDashboardWithDriverContext(dashboard, plan, driverContext)
}

func parseSimulationEnergyExplanationSQL(path string, plan *PurposeRunPlan) (EnergyExplanationV1, error) {
	return parseSimulationEnergyExplanationSQLWithDriverContext(path, plan, energyDriverBuildContext{})
}

func parseSimulationEnergyExplanationSQLWithDriverContext(path string, plan *PurposeRunPlan, driverContext energyDriverBuildContext) (EnergyExplanationV1, error) {
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, plan, driverContext)
	if err != nil {
		return EnergyExplanationV1{}, err
	}
	if len(parsed.Series) == 0 && len(parsed.Sources) == 0 {
		return emptyEnergyExplanationResult(plan), nil
	}
	return buildEnergyExplanationResultWithDriverContext(parsed.Series, parsed.Sources, plan, driverContext), nil
}

func parseSimulationEnergyExplanationCanonicalSQL(path string, plan *PurposeRunPlan, driverContext energyDriverBuildContext) (energyExplanationParseResult, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return energyExplanationParseResult{}, err
	}
	defer db.Close()

	series := []energyExplanationSeries{}
	sources := []EnergyDataSource{}
	ready, err := sqlHasTables(db, "ReportDataDictionary", "ReportData", "Time")
	if err != nil {
		return energyExplanationParseResult{}, err
	}
	if ready {
		dictionaries, err := sqlEnergyExplanationDictionaries(db, filepath.Base(path), plan)
		if err != nil {
			return energyExplanationParseResult{}, err
		}
		if len(dictionaries) > 0 {
			intervalDetails, err := sqlTimeIntervalDetailsForDatabase(db)
			if err != nil {
				intervalDetails = sqlTimeIntervalDetails{
					hours:    map[int64]float64{},
					explicit: map[int64]bool{},
				}
			}
			selectedStartDay, selectedEndDay, hasSelectedRange := energyExplanationSelectedRangeDays(plan)

			ids := make([]int, 0, len(dictionaries))
			byID := map[int]energyExplanationDictionary{}
			for _, dictionary := range dictionaries {
				ids = append(ids, dictionary.row.index)
				byID[dictionary.row.index] = dictionary
			}

			builders := map[int]*energyExplanationSeriesBuilder{}
			surfaceCategories, surfaceCategoryEligible := energyExplanationSurfaceCategoriesForDictionaries(dictionaries, driverContext)
			categoryBuilders := map[string]*energyExplanationCategorySeriesBuilder{}
			if err := walkReportData(db, SQLSeriesQuery{DictionaryIndexes: ids}, func(row SQLSeriesRow) error {
				timeIndex := row.TimeIndex
				dictionaryIndex := row.DictionaryIndex
				value := row.Value
				if !value.Valid || math.IsNaN(value.Float64) || math.IsInf(value.Float64, 0) {
					return nil
				}
				dictionary, ok := byID[dictionaryIndex]
				if !ok {
					return nil
				}
				builder := builders[dictionaryIndex]
				if builder == nil {
					builder = &energyExplanationSeriesBuilder{dictionary: dictionary}
					builders[dictionaryIndex] = builder
				}
				intervalHours := energyExplanationRateIntervalHours(dictionary, row, intervalDetails.hours[timeIndex], intervalDetails.explicit[timeIndex])
				number, unit := energyExplanationSQLValue(value.Float64, dictionary, intervalHours)
				accumulateEnergyExplanationSeriesBuilder(builder, row, number, unit, dictionary, selectedStartDay, selectedEndDay, hasSelectedRange)
				if selection, ok := surfaceCategories[dictionaryIndex]; ok {
					category := selection.category
					categoryKey := energyExplanationCategoryBuilderKey(dictionary, category)
					categoryBuilder := categoryBuilders[categoryKey]
					if categoryBuilder == nil {
						categoryBuilder = &energyExplanationCategorySeriesBuilder{
							seriesBuilder: energyExplanationSeriesBuilder{dictionary: dictionary},
							category:      category,
						}
						categoryBuilders[categoryKey] = categoryBuilder
					}
					sourceID := fmt.Sprintf("sql-rdd-%d", dictionaryIndex)
					categoryBuilder.sourceIDs = appendUniqueStrings(categoryBuilder.sourceIDs, sourceID)
					if selection.annual {
						categoryBuilder.annualSourceIDs = appendUniqueStrings(categoryBuilder.annualSourceIDs, sourceID)
					}
					if selection.monthly {
						categoryBuilder.monthlySourceIDs = appendUniqueStrings(categoryBuilder.monthlySourceIDs, sourceID)
					}
					if selection.daily {
						categoryBuilder.dailySourceIDs = appendUniqueStrings(categoryBuilder.dailySourceIDs, sourceID)
					}
					if selection.hourly {
						categoryBuilder.hourlySourceIDs = appendUniqueStrings(categoryBuilder.hourlySourceIDs, sourceID)
					}
					if selection.selectedRange {
						categoryBuilder.selectedRangeSourceIDs = appendUniqueStrings(categoryBuilder.selectedRangeSourceIDs, sourceID)
					}
					categoryBuilder.category.RelatedEntityIDs = appendUniqueStrings(categoryBuilder.category.RelatedEntityIDs, category.RelatedEntityIDs...)
					accumulateEnergyExplanationSurfaceCategoryBuilder(&categoryBuilder.seriesBuilder, row, number, unit, dictionary, selection, selectedStartDay, selectedEndDay, hasSelectedRange)
				}
				return nil
			}); err != nil {
				return energyExplanationParseResult{}, err
			}

			for _, dictionary := range dictionaries {
				builder := builders[dictionary.row.index]
				if builder == nil || builder.total == 0 && len(builder.monthly) == 0 {
					continue
				}
				source := energyDataSourceForDictionary(dictionary)
				source.ObjectIndex = energyExplanationObjectIndexForDictionary(dictionary, plan)
				source.NormalizedUnit = builder.unit
				if dictionary.energy != nil && dictionary.energy.HierarchyLevel == "zone_direct_use" {
					source.ZoneName = strings.TrimSpace(dictionary.row.keyValue)
				}
				scopeZoneName := energyExplanationScopeZoneForDictionary(dictionary, plan)
				if scopeZoneName != "" {
					source.ZoneName = scopeZoneName
				}
				sources = append(sources, source)
				item := energyExplanationSeriesForBuilder(builder, source.ID)
				item.parseCategorySource = surfaceCategoryEligible[dictionary.row.index]
				if scopeZoneName != "" {
					item.ZoneName = scopeZoneName
				}
				series = append(series, canonicalEnergyExplanationSeries(item))
			}
			series = append(series, energyExplanationCategorySeries(categoryBuilders)...)
		}
	}
	tabularSeries, tabularSources, err := parseEnergyExplanationTabularAnnual(db, series)
	if err != nil {
		return energyExplanationParseResult{}, err
	}
	series = append(series, tabularSeries...)
	sources = append(sources, tabularSources...)
	return energyExplanationParseResult{Series: series, Sources: sources}, nil
}

func accumulateEnergyExplanationSeriesBuilder(builder *energyExplanationSeriesBuilder, row SQLSeriesRow, number float64, unit string, dictionary energyExplanationDictionary, selectedStartDay int, selectedEndDay int, hasSelectedRange bool) {
	if builder == nil {
		return
	}
	builder.unit = unit
	builder.total += number
	if energyExplanationSupportsMonthlyPeriods(dictionary) && row.Month.Valid && row.Month.Int64 >= 1 && row.Month.Int64 <= 12 {
		if builder.monthly == nil {
			builder.monthly = map[int]float64{}
		}
		builder.monthly[int(row.Month.Int64)] += number
	}
	if energyExplanationSupportsDailyPeriods(dictionary) {
		if rowDay, ok := energyExplanationSQLDayOfYear(row.Month, row.Day); ok {
			if builder.daily == nil {
				builder.daily = map[int]float64{}
			}
			builder.daily[rowDay] += number
		}
	}
	if energyExplanationSupportsHourlyPeriods(dictionary) {
		if rowHour, ok := energyExplanationSQLHourOfYear(row.Month, row.Day, row.Hour); ok {
			if builder.hourly == nil {
				builder.hourly = map[int]float64{}
			}
			builder.hourly[rowHour] += number
		}
	}
	if hasSelectedRange {
		if rowDay, ok := energyExplanationSQLDayOfYear(row.Month, row.Day); ok && comfortDayInScope(rowDay, selectedStartDay, selectedEndDay) {
			builder.selectedRange += number
			builder.hasSelectedRange = true
		}
	}
}

func energyExplanationSurfaceCategoriesForDictionaries(dictionaries []energyExplanationDictionary, context energyDriverBuildContext) (map[int]energySurfacePeriodSelection, map[int]bool) {
	selected := map[int]energySurfacePeriodSelection{}
	eligible := map[int]bool{}
	if !context.Enabled {
		return selected, eligible
	}
	type selectedDictionary struct {
		dictionary energyExplanationDictionary
		category   energySurfaceCategory
		series     energyExplanationSeries
	}
	bySource := map[string][]selectedDictionary{}
	for _, dictionary := range dictionaries {
		if dictionary.heat == nil || !dictionary.heat.SurfaceScoped {
			continue
		}
		policy := energyDriverSourcePolicyFor(dictionary.row.name, dictionary.heat.Kind)
		if policy.Role != energyDriverSourceRoleMainFlow {
			continue
		}
		category, _ := context.SurfaceCategories.resolve(dictionary.row.keyValue)
		eligible[dictionary.row.index] = true
		candidate := energyExplanationSeriesForBuilder(&energyExplanationSeriesBuilder{dictionary: dictionary}, fmt.Sprintf("sql-rdd-%d", dictionary.row.index))
		key := energyExplanationSeriesSelectionKey(candidate)
		bySource[key] = append(bySource[key], selectedDictionary{dictionary: dictionary, category: category, series: candidate})
	}
	for _, candidates := range bySource {
		selectCandidate := func(include func(energyExplanationDictionary) bool, mark func(*energySurfacePeriodSelection)) {
			var best selectedDictionary
			found := false
			for _, candidate := range candidates {
				if include != nil && !include(candidate.dictionary) {
					continue
				}
				if !found || energyExplanationSeriesSourcePreferred(candidate.series, best.series) {
					best = candidate
					found = true
				}
			}
			if found {
				selection := selected[best.dictionary.row.index]
				selection.category = best.category
				mark(&selection)
				selected[best.dictionary.row.index] = selection
			}
		}
		selectCandidate(nil, func(selection *energySurfacePeriodSelection) { selection.annual = true })
		selectCandidate(energyExplanationSupportsMonthlyPeriods, func(selection *energySurfacePeriodSelection) { selection.monthly = true })
		selectCandidate(energyExplanationSupportsDailyPeriods, func(selection *energySurfacePeriodSelection) {
			selection.daily = true
			selection.selectedRange = true
		})
		selectCandidate(energyExplanationSupportsHourlyPeriods, func(selection *energySurfacePeriodSelection) { selection.hourly = true })
	}
	return selected, eligible
}

func accumulateEnergyExplanationSurfaceCategoryBuilder(builder *energyExplanationSeriesBuilder, row SQLSeriesRow, number float64, unit string, dictionary energyExplanationDictionary, selection energySurfacePeriodSelection, selectedStartDay int, selectedEndDay int, hasSelectedRange bool) {
	if builder == nil {
		return
	}
	builder.unit = unit
	if selection.annual {
		builder.total += number
	}
	if selection.monthly && row.Month.Valid && row.Month.Int64 >= 1 && row.Month.Int64 <= 12 {
		if builder.monthly == nil {
			builder.monthly = map[int]float64{}
		}
		builder.monthly[int(row.Month.Int64)] += number
	}
	rowDay, hasDay := energyExplanationSQLDayOfYear(row.Month, row.Day)
	if selection.daily && hasDay {
		if builder.daily == nil {
			builder.daily = map[int]float64{}
		}
		builder.daily[rowDay] += number
	}
	if selection.hourly {
		if rowHour, ok := energyExplanationSQLHourOfYear(row.Month, row.Day, row.Hour); ok {
			if builder.hourly == nil {
				builder.hourly = map[int]float64{}
			}
			builder.hourly[rowHour] += number
		}
	}
	if selection.selectedRange && hasSelectedRange && hasDay && rowDay >= selectedStartDay && rowDay <= selectedEndDay {
		builder.selectedRange += number
		builder.hasSelectedRange = true
	}
}

func energyExplanationCategoryBuilderKey(dictionary energyExplanationDictionary, category energySurfaceCategory) string {
	item := energyExplanationSeriesForBuilder(&energyExplanationSeriesBuilder{dictionary: dictionary}, "")
	return strings.Join([]string{
		normalizeEnergyOutputName(category.ZoneName),
		normalizeEnergyOutputName(category.Category),
		normalizeEnergyOutputName(item.Kind),
		normalizeEnergyOutputName(item.ThermalComponent),
		normalizeEnergyOutputName(item.HeatSign),
		fmt.Sprintf("%.6f", item.heatSignMultiplier),
	}, "|")
}

func energyExplanationCategorySeries(builders map[string]*energyExplanationCategorySeriesBuilder) []energyExplanationSeries {
	if len(builders) == 0 {
		return nil
	}
	keys := make([]string, 0, len(builders))
	for key := range builders {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]energyExplanationSeries, 0, len(keys))
	for _, key := range keys {
		aggregate := builders[key]
		item := energyExplanationSeriesForBuilder(&aggregate.seriesBuilder, "")
		item.ZoneName = aggregate.category.ZoneName
		item.DriverCategory = aggregate.category.Category
		item.DriverSourceRole = energyDriverSourceRoleMainFlow
		item.DriverExplanation = energyDriverSurfaceExplanation
		item.Label = energyDriverCategoryLabel(aggregate.category.Category)
		item.SurfaceScoped = false
		item.SourceIDs = append([]string(nil), aggregate.sourceIDs...)
		item.AnnualSourceIDs = append([]string(nil), aggregate.annualSourceIDs...)
		item.MonthlySourceIDs = append([]string(nil), aggregate.monthlySourceIDs...)
		item.DailySourceIDs = append([]string(nil), aggregate.dailySourceIDs...)
		item.HourlySourceIDs = append([]string(nil), aggregate.hourlySourceIDs...)
		item.SelectedRangeSourceIDs = append([]string(nil), aggregate.selectedRangeSourceIDs...)
		item.RelatedEntityIDs = append([]string(nil), aggregate.category.RelatedEntityIDs...)
		// Energy-vs-Rate preference is resolved for every physical surface before
		// rows enter this builder. The resulting category series may legitimately
		// contain Energy rows for one surface and a Rate fallback for another, so
		// it must not be selected again as one whole-source alias family.
		item.sourceName = "Selected surface inside-face convection by surface"
		item.SourceName = item.sourceName
		item.sourceIsRate = false
		item.sourceKeyValue = ""
		item.parseCategoryAggregate = true
		out = append(out, canonicalEnergyExplanationSeries(item))
	}
	return out
}

func energyExplanationScopeZoneForDictionary(dictionary energyExplanationDictionary, plan *PurposeRunPlan) string {
	if plan == nil || dictionary.load == nil && dictionary.heat == nil {
		return ""
	}
	for _, object := range plan.OutputObjects {
		if object.ScopeZoneName == "" || !purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) || !strings.EqualFold(strings.TrimSpace(object.ObjectType), "Output:Variable") {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(object.KeyValue), strings.TrimSpace(dictionary.row.keyValue)) {
			continue
		}
		sameAliasFamily := dictionary.load != nil && energyNamesShareLoadAliasGroup(object.VariableName, dictionary.row.name) ||
			dictionary.heat != nil && energyNamesShareHeatAliasGroup(object.VariableName, dictionary.row.name)
		if !sameAliasFamily {
			continue
		}
		return object.ScopeZoneName
	}
	return ""
}

func buildEnergyExplanationFromDashboard(dashboard EnergyDashboardResult, plan *PurposeRunPlan) EnergyExplanationV1 {
	return buildEnergyExplanationFromDashboardWithDriverContext(dashboard, plan, energyDriverBuildContext{})
}

func buildEnergyExplanationFromDashboardWithDriverContext(dashboard EnergyDashboardResult, plan *PurposeRunPlan, driverContext energyDriverBuildContext) EnergyExplanationV1 {
	var series []energyExplanationSeries
	var sources []EnergyDataSource
	addSeries := func(item EnergySeries) {
		def, ok := energyMeterAliasOrOtherDefinitionForName(item.Name)
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
			SourceUnit:        item.Unit,
			NormalizedUnit:    item.Unit,
			AggregationMethod: "sum_dashboard_series",
		})
		monthly := map[int]float64{}
		for _, point := range item.Points {
			if month, ok := energyExplanationMonthFromPoint(point); ok {
				monthly[month] += point.Value
			}
		}
		series = append(series, canonicalEnergyExplanationSeries(energyExplanationSeries{
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
		}))
	}
	for _, item := range dashboard.FacilityMonthly {
		addSeries(item)
	}
	for _, item := range dashboard.EndUseMonthly {
		addSeries(item)
	}
	return buildEnergyExplanationResultWithDriverContext(series, sources, plan, driverContext)
}

func preferredEnergyExplanationSeries(series []energyExplanationSeries) []energyExplanationSeries {
	out := make([]energyExplanationSeries, 0, len(series))
	grouped := map[string][]energyExplanationSeries{}
	order := []string{}
	for _, item := range series {
		key := energyExplanationSeriesSelectionKey(item)
		if key == "" {
			out = append(out, item)
			continue
		}
		if _, ok := grouped[key]; !ok {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], item)
	}
	for _, key := range order {
		items := preferredEnergyExplanationSourceClass(grouped[key])
		selected, ok := preferredEnergyExplanationSeriesCandidate(items, func(energyExplanationSeries) bool { return true })
		if !ok {
			continue
		}
		if periodItem, ok := preferredEnergyExplanationPeriodSeries(items, func(item energyExplanationSeries) map[int]float64 { return item.Monthly }); ok {
			selected.Monthly = cloneEnergyExplanationPeriodValues(periodItem.Monthly)
			selected.RawMonthly = cloneEnergyExplanationPeriodValues(periodItem.RawMonthly)
			selected.MonthlySourceIDs = energyExplanationPeriodSourceIDs(periodItem.MonthlySourceIDs, periodItem.SourceIDs)
			selected.SourceIDs = appendUniqueStrings(selected.SourceIDs, periodItem.SourceIDs...)
		} else {
			selected.Monthly = nil
			selected.RawMonthly = nil
		}
		if periodItem, ok := preferredEnergyExplanationPeriodSeries(items, func(item energyExplanationSeries) map[int]float64 { return item.Daily }); ok {
			selected.Daily = cloneEnergyExplanationPeriodValues(periodItem.Daily)
			selected.RawDaily = cloneEnergyExplanationPeriodValues(periodItem.RawDaily)
			selected.DailySourceIDs = energyExplanationPeriodSourceIDs(periodItem.DailySourceIDs, periodItem.SourceIDs)
			selected.SourceIDs = appendUniqueStrings(selected.SourceIDs, periodItem.SourceIDs...)
		} else {
			selected.Daily = nil
			selected.RawDaily = nil
		}
		if periodItem, ok := preferredEnergyExplanationPeriodSeries(items, func(item energyExplanationSeries) map[int]float64 { return item.Hourly }); ok {
			selected.Hourly = cloneEnergyExplanationPeriodValues(periodItem.Hourly)
			selected.RawHourly = cloneEnergyExplanationPeriodValues(periodItem.RawHourly)
			selected.HourlySourceIDs = energyExplanationPeriodSourceIDs(periodItem.HourlySourceIDs, periodItem.SourceIDs)
			selected.SourceIDs = appendUniqueStrings(selected.SourceIDs, periodItem.SourceIDs...)
		} else {
			selected.Hourly = nil
			selected.RawHourly = nil
		}
		if rangeItem, ok := preferredEnergyExplanationSeriesCandidate(items, func(item energyExplanationSeries) bool { return item.HasSelectedRange }); ok {
			selected.SelectedRange = rangeItem.SelectedRange
			selected.RawSelectedRange = rangeItem.RawSelectedRange
			selected.SelectedRangeSourceIDs = energyExplanationPeriodSourceIDs(rangeItem.SelectedRangeSourceIDs, rangeItem.SourceIDs)
			selected.HasSelectedRange = true
			selected.SourceIDs = appendUniqueStrings(selected.SourceIDs, rangeItem.SourceIDs...)
		}
		out = append(out, selected)
	}
	return out
}

func canonicalEnergyExplanationSeries(item energyExplanationSeries) energyExplanationSeries {
	if strings.TrimSpace(item.Carrier) != "" {
		item.Carrier = canonicalEnergyPathCarrier(item.Carrier)
	}
	item.Unit = canonicalEnergyPathEnergyUnit(item.Unit)
	if item.Stage == "" {
		switch item.Level {
		case "heat", "driver":
			item.Stage = "driver"
		case "load":
			item.Stage = "load"
		case "energy":
			switch {
			case energyExplanationIsSupportEndUse(item):
				item.Stage = "support"
			case item.MeterHierarchyLevel == "facility_total" || strings.HasSuffix(item.Kind, ".total"):
				item.Stage = "carrier"
			default:
				item.Stage = "end_use"
			}
		default:
			item.Stage = strings.TrimSpace(item.Level)
		}
	}
	if item.CanonicalKind == "" {
		item.CanonicalKind = strings.TrimSpace(item.Kind)
	}
	if item.SurfaceName == "" && item.SurfaceScoped {
		item.SurfaceName = strings.TrimSpace(item.sourceKeyValue)
	}
	if item.Sign == "" {
		item.Sign = strings.TrimSpace(item.HeatSign)
		if item.Sign == "" && item.Stage == "driver" {
			item.Sign = "signed"
		}
	}
	if item.SourceName == "" {
		item.SourceName = strings.TrimSpace(item.sourceName)
	}
	if item.SourceKey == "" {
		item.SourceKey = strings.TrimSpace(item.sourceKeyValue)
	}
	if item.SourceClass == "" {
		item.SourceClass = energyExplanationPhysicalSourceClass(item)
	}
	if item.PeriodBasis == "" {
		item.PeriodBasis = energyExplanationPeriodBasis(item.sourceFrequency, item.Monthly)
	}
	if len(item.AnnualSourceIDs) == 0 {
		item.AnnualSourceIDs = appendUniqueStrings(nil, item.SourceIDs...)
	}
	if len(item.Monthly) > 0 && len(item.MonthlySourceIDs) == 0 {
		item.MonthlySourceIDs = appendUniqueStrings(nil, item.SourceIDs...)
	}
	if len(item.Daily) > 0 && len(item.DailySourceIDs) == 0 {
		item.DailySourceIDs = appendUniqueStrings(nil, item.SourceIDs...)
	}
	if len(item.Hourly) > 0 && len(item.HourlySourceIDs) == 0 {
		item.HourlySourceIDs = appendUniqueStrings(nil, item.SourceIDs...)
	}
	if item.HasSelectedRange && len(item.SelectedRangeSourceIDs) == 0 {
		item.SelectedRangeSourceIDs = appendUniqueStrings(nil, item.SourceIDs...)
	}
	if !item.multiplierApplied {
		item.RawTotal = item.Total
		item.RawMonthly = cloneEnergyExplanationPeriodValues(item.Monthly)
		item.RawDaily = cloneEnergyExplanationPeriodValues(item.Daily)
		item.RawHourly = cloneEnergyExplanationPeriodValues(item.Hourly)
		item.RawSelectedRange = item.SelectedRange
	}
	item.SourceFamily = energyExplanationCanonicalSourceFamily(item)
	item.CanonicalFamily = energyExplanationCanonicalIdentity(item)
	return item
}

func energyExplanationPeriodBasis(frequency string, monthly map[int]float64) string {
	switch strings.ToLower(canonicalPurposeFrequency(frequency)) {
	case "runperiod", "annual":
		return "annual_only"
	case "monthly":
		return "monthly"
	case "daily", "hourly", "timestep", "detailed":
		return "streamed_monthly"
	case "":
		if len(monthly) > 0 {
			return "monthly"
		}
		return "unknown"
	default:
		return "unknown"
	}
}

func energyExplanationCanonicalSourceFamily(item energyExplanationSeries) string {
	return energyExplanationCanonicalIdentityParts(item, true)
}

func energyExplanationCanonicalIdentity(item energyExplanationSeries) string {
	return energyExplanationCanonicalIdentityParts(item, false)
}

func energyExplanationCanonicalIdentityParts(item energyExplanationSeries, includeSourceClass bool) string {
	sourceKey := ""
	if (item.Stage == "load" || item.Stage == "driver") && item.ZoneName == "" && item.SurfaceName == "" {
		sourceKey = item.SourceKey
	}
	parts := []string{
		normalizeEnergyOutputName(item.Stage),
		normalizeEnergyOutputName(item.CanonicalKind),
		normalizeEnergyOutputName(item.ZoneName),
		normalizeEnergyOutputName(item.SurfaceName),
		normalizeEnergyOutputName(item.ServiceKind),
		normalizeEnergyOutputName(item.Carrier),
		normalizeEnergyOutputName(item.EndUse),
		normalizeEnergyOutputName(item.ThermalComponent),
		normalizeEnergyOutputName(item.DriverCategory),
		normalizeEnergyOutputName(item.HeatCategory),
		normalizeEnergyOutputName(item.Sign),
	}
	if includeSourceClass {
		parts = append(parts, normalizeEnergyOutputName(item.SourceClass))
	}
	parts = append(parts, normalizeEnergyOutputName(sourceKey))
	return strings.Join(parts, "|")
}

func energyExplanationPhysicalSourceClass(item energyExplanationSeries) string {
	if item.Stage == "carrier" || item.Stage == "end_use" || item.Stage == "support" {
		return "meter"
	}
	name := normalizeEnergyOutputName(firstNonEmpty(item.SourceName, item.sourceName))
	switch {
	case strings.Contains(name, "zone ideal loads"):
		return "zone_ideal_loads"
	case strings.Contains(name, "zone system predicted sensible load"):
		return "zone_system_predicted"
	case strings.Contains(name, "zone predicted sensible load"):
		return "zone_predicted"
	case strings.Contains(name, "zone air system"):
		return "zone_air_system"
	case strings.Contains(name, "surface inside face convection"):
		return "surface_inside_face_convection"
	case strings.Contains(name, "zone total internal") || strings.Contains(name, "zone air heat balance internal convective"):
		return "zone_internal_gain_aggregate"
	case strings.Contains(name, "zone combined outdoor air"):
		return "zone_combined_outdoor_air"
	case strings.Contains(name, "zone air heat balance outdoor air"):
		return "zone_outdoor_air_aggregate"
	case strings.Contains(name, "zone air heat balance"):
		return "zone_air_heat_balance"
	}
	name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(name, " energy"), " rate"))
	if name != "" {
		return name
	}
	return normalizeEnergyOutputName(item.CanonicalKind)
}

func preferredEnergyExplanationSeriesCandidate(items []energyExplanationSeries, include func(energyExplanationSeries) bool) (energyExplanationSeries, bool) {
	var selected energyExplanationSeries
	found := false
	for _, item := range items {
		if include != nil && !include(item) {
			continue
		}
		if !found || energyExplanationSeriesSourcePreferred(item, selected) {
			selected = item
			found = true
		}
	}
	return selected, found
}

func preferredEnergyExplanationSourceClass(items []energyExplanationSeries) []energyExplanationSeries {
	if len(items) < 2 {
		return items
	}
	byClass := map[string][]energyExplanationSeries{}
	order := []string{}
	for _, item := range items {
		key := normalizeEnergyOutputName(item.SourceClass)
		if _, exists := byClass[key]; !exists {
			order = append(order, key)
		}
		byClass[key] = append(byClass[key], item)
	}
	if len(byClass) < 2 {
		return items
	}
	selectedClass := ""
	selectedCoverage := -1
	selectedRank := int(^uint(0) >> 1)
	for _, class := range order {
		coverage := 0
		for _, item := range byClass[class] {
			if len(item.Monthly) > coverage {
				coverage = len(item.Monthly)
			}
		}
		rank := energyExplanationPhysicalSourceClassRank(class)
		if selectedClass == "" || coverage > selectedCoverage || coverage == selectedCoverage && rank < selectedRank {
			selectedClass = class
			selectedCoverage = coverage
			selectedRank = rank
		}
	}
	return byClass[selectedClass]
}

func energyExplanationPhysicalSourceClassRank(class string) int {
	switch normalizeEnergyOutputName(class) {
	case "zone_air_system":
		return 0
	case "zone_ideal_loads":
		return 1
	case "zone_predicted":
		return 2
	case "zone_system_predicted":
		return 3
	default:
		return 10
	}
}

func preferredEnergyExplanationPeriodSeries(items []energyExplanationSeries, values func(energyExplanationSeries) map[int]float64) (energyExplanationSeries, bool) {
	var selected energyExplanationSeries
	selectedCoverage := -1
	found := false
	for _, item := range items {
		coverage := len(values(item))
		if coverage == 0 {
			continue
		}
		candidateRateRank := 0
		if item.sourceIsRate {
			candidateRateRank = 1
		}
		selectedRateRank := 0
		if selected.sourceIsRate {
			selectedRateRank = 1
		}
		if !found || candidateRateRank < selectedRateRank || candidateRateRank == selectedRateRank && (coverage > selectedCoverage || coverage == selectedCoverage && energyExplanationSeriesSourcePreferred(item, selected)) {
			selected = item
			selectedCoverage = coverage
			found = true
		}
	}
	return selected, found
}

func cloneEnergyExplanationPeriodValues(values map[int]float64) map[int]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[int]float64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func energyExplanationPeriodSourceIDs(periodIDs []string, fallback []string) []string {
	if len(periodIDs) > 0 {
		return appendUniqueStrings(nil, periodIDs...)
	}
	return appendUniqueStrings(nil, fallback...)
}

func energyExplanationSeriesForGraphPeriod(series []energyExplanationSeries, period string) []energyExplanationSeries {
	out := make([]energyExplanationSeries, len(series))
	for index, item := range series {
		switch period {
		case "monthly":
			item.SourceIDs = energyExplanationPeriodSourceIDs(item.MonthlySourceIDs, item.SourceIDs)
		case "daily":
			item.SourceIDs = energyExplanationPeriodSourceIDs(item.DailySourceIDs, item.SourceIDs)
		case "hourly":
			item.SourceIDs = energyExplanationPeriodSourceIDs(item.HourlySourceIDs, item.SourceIDs)
		case "selected_range":
			item.SourceIDs = energyExplanationPeriodSourceIDs(item.SelectedRangeSourceIDs, item.SourceIDs)
		default:
			item.SourceIDs = energyExplanationPeriodSourceIDs(item.AnnualSourceIDs, item.SourceIDs)
		}
		out[index] = item
	}
	return out
}

func energyExplanationSeriesSelectionKey(item energyExplanationSeries) string {
	item = canonicalEnergyExplanationSeries(item)
	if item.CanonicalFamily != "" && (item.Stage == "carrier" || item.Stage == "end_use" || item.Stage == "support") {
		return item.CanonicalFamily
	}
	switch item.Level {
	case "load":
		return item.CanonicalFamily
	case "heat":
		// Driver preparation classifies additive, context, and reconciliation
		// sources before preference selection. Keep those roles in separate
		// preference families so a broader context quantity (for example People
		// Sensible) cannot displace the narrower convective main-flow source.
		if role := strings.TrimSpace(item.DriverSourceRole); role != "" {
			if role == energyDriverSourceRoleMainFlow && strings.HasPrefix(canonicalEnergyDriverCategory(item.DriverCategory), "internal.") {
				// Electric, gas, hot-water, steam, IT, and other internal-gain
				// outputs are independent additive families. SourceFamily retains
				// that physical subtype while collapsing only its Energy/Rate pair.
				return item.SourceFamily + "|driver-role:" + normalizeEnergyOutputName(role)
			}
			return item.CanonicalFamily + "|driver-role:" + normalizeEnergyOutputName(role)
		}
		return item.CanonicalFamily
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

func emptyEnergyExplanationResult(plan *PurposeRunPlan) EnergyExplanationV1 {
	allocationPolicy := energyExplanationAllocationPolicy(plan)
	result := EnergyExplanationV1{
		Schema:            energyExplanationV1Schema,
		Purpose:           string(SimulationPurposeBasicEnergy),
		Frequency:         "monthly",
		AllocationPolicy:  allocationPolicy,
		RelationshipRules: energyRelationshipRuleCatalog(),
		scope:             energyExplanationScopeForPlan(plan),
	}
	result.Completeness = buildEnergyExplanationCompleteness(nil, nil, plan, 0)
	return result
}

func buildEnergyExplanationResult(series []energyExplanationSeries, sources []EnergyDataSource, plan *PurposeRunPlan) EnergyExplanationV1 {
	return buildEnergyExplanationResultWithDriverContext(series, sources, plan, energyDriverBuildContext{})
}

func buildEnergyExplanationResultWithDriverContext(series []energyExplanationSeries, sources []EnergyDataSource, plan *PurposeRunPlan, driverContext energyDriverBuildContext) EnergyExplanationV1 {
	allocationPolicy := energyExplanationAllocationPolicy(plan)
	for index := range series {
		series[index] = canonicalEnergyExplanationSeries(series[index])
	}
	series, sources = filterEnergyExplanationSeriesForBasicDetail(series, sources, plan)
	driverWarnings := []EnergyWarning{}
	if driverContext.Enabled {
		series, sources, driverWarnings = prepareEnergyDriverSeries(series, sources, driverContext)
	}
	for index := range series {
		series[index] = canonicalEnergyExplanationSeries(series[index])
	}
	var multiplierWarnings []EnergyWarning
	if driverContext.Enabled {
		series, sources, multiplierWarnings = applyEnergyExplanationMultipliers(series, sources, driverContext.Multipliers)
	}
	driverWarnings = append(driverWarnings, multiplierWarnings...)
	loadCandidates := append([]energyExplanationSeries(nil), series...)
	if driverContext.Enabled {
		series = selectCanonicalEnergyExplanationLoads(series)
	}
	series = preferredEnergyExplanationSeries(series)
	if driverContext.Enabled {
		for index := range series {
			if _, ok := energyLoadCanonicalSelectionService(series[index].ServiceKind); series[index].Stage == "load" && ok {
				series[index].canonicalLoadMetadata = true
			}
		}
		sources = annotateEnergyExplanationLoadSources(loadCandidates, series, sources)
		series, sources, multiplierWarnings = finalizeEnergyDriverMappings(series, sources, driverContext)
		driverWarnings = append(driverWarnings, multiplierWarnings...)
	}
	zoneDirectUseSeries := []energyExplanationSeries(nil)
	if energyExplanationPlanUsesEnergyPath(plan) {
		zoneDirectUseSeries = energyExplanationDirectZoneSeries(series)
	}
	series = excludeEnergyExplanationDirectUseSeries(series)
	series, sources = filterEnergyExplanationWaterContextSeries(series, sources)
	sort.SliceStable(series, func(i, j int) bool {
		if series[i].Level != series[j].Level {
			return series[i].Level < series[j].Level
		}
		return series[i].Kind < series[j].Kind
	})
	sort.SliceStable(sources, func(i, j int) bool {
		return sources[i].ID < sources[j].ID
	})
	months := energyExplanationMonths(series)
	monthlySeries := energyExplanationSeriesForGraphPeriod(series, "monthly")
	monthlyGraphs := make(map[int]energyExplanationGraph, len(months))
	for _, month := range months {
		periodID := fmt.Sprintf("M%d", month)
		graph := buildEnergyExplanationGraphForPeriod(periodID, monthlySeries, allocationPolicy, func(item energyExplanationSeries) float64 {
			return item.Monthly[month]
		}, sources)
		graph.Warnings = appendEnergyDriverWarningsForPeriod(graph.Warnings, driverWarnings, periodID)
		monthlyGraphs[month] = graph
	}
	annualSeries := series
	// The legacy parser/builder remains the frozen v1 adapter. Runtime builds
	// carry an IDF-derived driver context and use the canonical monthly
	// contribution contract; direct v1 reads retain their historical totals and
	// identifiers until they cross the v1->v2 upgrade boundary.
	if driverContext.Enabled {
		annualSeries = energyExplanationAnnualContributionSeries(series)
	}
	annualSeed := buildEnergyExplanationGraphForPeriod("annual", annualSeries, allocationPolicy, func(item energyExplanationSeries) float64 {
		return item.Total
	}, sources)
	annual := annualSeed
	if driverContext.Enabled {
		annual = buildEnergyExplanationAnnualGraphFromMonthly(annualSeed, monthlyGraphs, series)
	}
	annual.Warnings = appendEnergyDriverWarningsForPeriod(annual.Warnings, driverWarnings, "annual")
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
	if selectedRange, ok := buildEnergyExplanationSelectedRangePeriod(series, sources, allocationPolicy, plan); ok {
		periods = append(periods, selectedRange)
	}
	for _, month := range months {
		periodID := fmt.Sprintf("M%d", month)
		graph := monthlyGraphs[month]
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
		graph := buildEnergyExplanationGraphForPeriod(periodID, energyExplanationSeriesForGraphPeriod(series, "daily"), allocationPolicy, func(item energyExplanationSeries) float64 {
			return item.Daily[day]
		}, sources)
		graph.Warnings = appendEnergyDriverWarningsForPeriod(graph.Warnings, driverWarnings, periodID)
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
	for _, hour := range energyExplanationHours(series) {
		periodID := fmt.Sprintf("H%d", hour)
		graph := buildEnergyExplanationGraphForPeriod(periodID, energyExplanationSeriesForGraphPeriod(series, "hourly"), allocationPolicy, func(item energyExplanationSeries) float64 {
			return item.Hourly[hour]
		}, sources)
		graph.Warnings = appendEnergyDriverWarningsForPeriod(graph.Warnings, driverWarnings, periodID)
		periods = append(periods, EnergyPeriod{
			ID:             periodID,
			Label:          fmt.Sprintf("Hour %d", hour),
			Kind:           "hourly",
			Nodes:          graph.Nodes,
			Edges:          graph.Edges,
			Reconciliation: graph.Reconciliation,
			Warnings:       graph.Warnings,
		})
	}
	result := EnergyExplanationV1{
		Schema:                energyExplanationV1Schema,
		Purpose:               string(SimulationPurposeBasicEnergy),
		Frequency:             "monthly",
		AllocationPolicy:      allocationPolicy,
		RelationshipRules:     energyRelationshipRuleCatalog(),
		Periods:               periods,
		Nodes:                 annual.Nodes,
		Edges:                 annual.Edges,
		Reconciliation:        annual.Reconciliation,
		Sources:               sources,
		Completeness:          buildEnergyExplanationCompleteness(series, sources, plan, annual.MappedPercent),
		Warnings:              annual.Warnings,
		scope:                 energyExplanationScopeForPlan(plan),
		canonicalMonthlyBasis: driverContext.Enabled,
		zoneDirectUseSeries:   zoneDirectUseSeries,
	}
	return result
}

func energyExplanationAnnualContributionSeries(series []energyExplanationSeries) []energyExplanationSeries {
	out := make([]energyExplanationSeries, 0, len(series)+2)
	for _, original := range series {
		item := canonicalEnergyExplanationSeries(original)
		if len(item.Monthly) == 0 {
			item.SourceIDs = energyExplanationPeriodSourceIDs(item.AnnualSourceIDs, item.SourceIDs)
			// Annual-only fallback is a measured site-energy convenience. It must
			// never fabricate driver or delivered-load contributions.
			if item.Stage != "carrier" && item.Stage != "end_use" {
				continue
			}
			out = append(out, item)
			continue
		}
		if item.Stage != "driver" {
			item.SourceIDs = energyExplanationPeriodSourceIDs(item.MonthlySourceIDs, item.SourceIDs)
			item.Total = roundedEnergyNumber(sumEnergyExplanationPeriodValues(item.Monthly))
			item.RawTotal = roundedEnergyNumber(sumEnergyExplanationPeriodValues(item.RawMonthly))
			if len(item.RawMonthly) == 0 {
				item.RawTotal = energyExplanationRawValue(item.Total, item.EffectiveMultiplier)
			}
			out = append(out, item)
			continue
		}
		if strings.TrimSpace(item.HeatSign) != "" {
			item.SourceIDs = energyExplanationPeriodSourceIDs(item.MonthlySourceIDs, item.SourceIDs)
			item.Total = roundedEnergyNumber(sumEnergyExplanationPeriodValues(item.Monthly))
			item.RawTotal = roundedEnergyNumber(sumEnergyExplanationPeriodValues(item.RawMonthly))
			if len(item.RawMonthly) == 0 {
				item.RawTotal = energyExplanationRawValue(item.Total, item.EffectiveMultiplier)
			}
			out = append(out, item)
			continue
		}

		positive := item
		negative := item
		positive.SourceIDs = energyExplanationPeriodSourceIDs(item.MonthlySourceIDs, item.SourceIDs)
		negative.SourceIDs = energyExplanationPeriodSourceIDs(item.MonthlySourceIDs, item.SourceIDs)
		positive.Total, positive.RawTotal = 0, 0
		negative.Total, negative.RawTotal = 0, 0
		positive.HeatSign, positive.Sign, positive.heatSignMultiplier = "", "positive", 1
		negative.HeatSign, negative.Sign, negative.heatSignMultiplier = "", "negative", -1
		for month := 1; month <= 12; month++ {
			effectiveSigned := item.Monthly[month] * energyExplanationHeatSeriesSignMultiplier(item)
			rawSigned := item.RawMonthly[month] * energyExplanationHeatSeriesSignMultiplier(item)
			if len(item.RawMonthly) == 0 {
				rawSigned = energyExplanationRawValue(effectiveSigned, item.EffectiveMultiplier)
			}
			if effectiveSigned > 0 {
				positive.Total += effectiveSigned
				positive.RawTotal += math.Abs(rawSigned)
			} else if effectiveSigned < 0 {
				negative.Total += math.Abs(effectiveSigned)
				negative.RawTotal += math.Abs(rawSigned)
			}
		}
		if positive.Total != 0 {
			positive.Total = roundedEnergyNumber(positive.Total)
			positive.RawTotal = roundedEnergyNumber(positive.RawTotal)
			positive.SourceFamily = energyExplanationCanonicalSourceFamily(positive)
			out = append(out, positive)
		}
		if negative.Total != 0 {
			negative.Total = roundedEnergyNumber(negative.Total)
			negative.RawTotal = roundedEnergyNumber(negative.RawTotal)
			negative.SourceFamily = energyExplanationCanonicalSourceFamily(negative)
			out = append(out, negative)
		}
	}
	return out
}

func buildEnergyExplanationAnnualGraphFromMonthly(seed energyExplanationGraph, monthly map[int]energyExplanationGraph, series []energyExplanationSeries) energyExplanationGraph {
	if len(monthly) == 0 {
		return seed
	}
	annual := aggregateEnergyExplanationMonthlyGraphs(monthly)
	fallbackSources := map[string]bool{}
	for _, original := range series {
		item := canonicalEnergyExplanationSeries(original)
		if len(item.Monthly) != 0 {
			continue
		}
		allowAnnualOnly := item.Stage == "carrier" || item.Stage == "end_use"
		if !allowAnnualOnly {
			continue
		}
		for _, sourceID := range item.SourceIDs {
			fallbackSources[sourceID] = true
		}
	}

	nodeIndex := make(map[string]int, len(annual.Nodes)+len(seed.Nodes))
	for index, node := range annual.Nodes {
		nodeIndex[node.ID] = index
	}
	fallbackNode := map[string]bool{}
	for _, node := range seed.Nodes {
		if !energyExplanationSourcesIntersect(node.SourceIDs, fallbackSources) {
			continue
		}
		fallbackNode[node.ID] = true
		if _, exists := nodeIndex[node.ID]; exists {
			continue
		}
		node.Period = "annual"
		nodeIndex[node.ID] = len(annual.Nodes)
		annual.Nodes = append(annual.Nodes, node)
	}

	edgeIndex := make(map[string]int, len(annual.Edges)+len(seed.Edges))
	for index, edge := range annual.Edges {
		edgeIndex[energyExplanationEdgeAggregationKey(edge)] = index
	}
	for _, edge := range seed.Edges {
		key := energyExplanationEdgeAggregationKey(edge)
		if _, exists := edgeIndex[key]; exists {
			continue
		}
		if !fallbackNode[edge.FromID] && !fallbackNode[edge.ToID] && !energyExplanationSourcesIntersect(edge.SourceIDs, fallbackSources) {
			continue
		}
		if _, ok := nodeIndex[edge.FromID]; !ok {
			continue
		}
		if _, ok := nodeIndex[edge.ToID]; !ok {
			continue
		}
		edge.Period = "annual"
		edge.ID = energyExplanationAnnualEdgeID(edge)
		edgeIndex[key] = len(annual.Edges)
		annual.Edges = append(annual.Edges, edge)
	}

	reconciliationIndex := make(map[string]int, len(annual.Reconciliation)+len(seed.Reconciliation))
	for index, item := range annual.Reconciliation {
		reconciliationIndex[energyExplanationReconciliationAggregationKey(item)] = index
	}
	for _, item := range seed.Reconciliation {
		key := energyExplanationReconciliationAggregationKey(item)
		if _, exists := reconciliationIndex[key]; exists || !energyExplanationSourcesIntersect(item.SourceIDs, fallbackSources) {
			continue
		}
		item.Period = "annual"
		item.ID = energyExplanationAnnualReconciliationID(item.ID)
		reconciliationIndex[key] = len(annual.Reconciliation)
		annual.Reconciliation = append(annual.Reconciliation, item)
	}
	for _, warning := range seed.Warnings {
		warning.Period = "annual"
		annual.Warnings = appendEnergyDriverWarning(annual.Warnings, warning)
	}
	annual.MappedPercent = energyExplanationMappedPercentFromReconciliation(annual.Reconciliation)
	sortEnergyExplanationNodes(annual.Nodes)
	sortEnergyExplanationEdges(annual.Edges)
	return annual
}

func aggregateEnergyExplanationMonthlyGraphs(monthly map[int]energyExplanationGraph) energyExplanationGraph {
	annual := energyExplanationGraph{}
	nodeIndex := map[string]int{}
	edgeIndex := map[string]int{}
	reconciliationIndex := map[string]int{}
	months := make([]int, 0, len(monthly))
	for month := range monthly {
		months = append(months, month)
	}
	sort.Ints(months)
	for _, month := range months {
		graph := monthly[month]
		for _, node := range graph.Nodes {
			index, exists := nodeIndex[node.ID]
			if !exists {
				node.Period = "annual"
				node.LoadBreakdown = cloneEnergyExplanationLoadComponents(node.LoadBreakdown)
				node.OffsetEffects = cloneEnergyExplanationOffsetEffects(node.OffsetEffects)
				node.SimultaneousLoad = cloneEnergyExplanationSimultaneousLoad(node.SimultaneousLoad)
				node.allocationSourceIDs = appendUniqueStrings(nil, node.allocationSourceIDs...)
				node.simultaneousLoadContributions = cloneEnergyExplanationSimultaneousLoadContributions(node.simultaneousLoadContributions)
				nodeIndex[node.ID] = len(annual.Nodes)
				annual.Nodes = append(annual.Nodes, node)
				continue
			}
			current := &annual.Nodes[index]
			current.Value = roundedEnergyNumber(current.Value + node.Value)
			current.SignedValue = roundedEnergyNumber(current.SignedValue + node.SignedValue)
			current.RawValue = roundedEnergyNumber(current.RawValue + node.RawValue)
			current.EffectiveValue = roundedEnergyNumber(current.EffectiveValue + node.EffectiveValue)
			current.AllocatedValue = roundedEnergyNumber(current.AllocatedValue + node.AllocatedValue)
			current.AllocationApplied = current.AllocationApplied || node.AllocationApplied
			current.AllocationExplanation = firstNonEmpty(current.AllocationExplanation, node.AllocationExplanation)
			current.DisplayValue = roundedEnergyNumber(current.DisplayValue + node.DisplayValue)
			if current.RawValue != 0 && current.EffectiveValue != 0 {
				current.Multiplier = roundedEnergyNumber(current.EffectiveValue / current.RawValue)
			}
			current.SourceIDs = appendUniqueStrings(current.SourceIDs, node.SourceIDs...)
			current.allocationSourceIDs = appendUniqueStrings(current.allocationSourceIDs, node.allocationSourceIDs...)
			current.RelatedEntityIDs = appendUniqueStrings(current.RelatedEntityIDs, node.RelatedEntityIDs...)
			current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, node.RelatedPathIDs...)
			current.LoadBreakdown = mergeEnergyExplanationLoadComponents(current.LoadBreakdown, node.LoadBreakdown)
			current.OffsetEffects = mergeEnergyExplanationOffsetEffects(current.OffsetEffects, node.OffsetEffects)
			current.simultaneousLoadContributions = mergeEnergyExplanationSimultaneousLoadContributions(current.simultaneousLoadContributions, node.simultaneousLoadContributions)
			finalizeEnergyExplanationLoadNode(current)
			finalizeEnergyExplanationSimultaneousLoad(current)
		}
		for _, edge := range graph.Edges {
			key := energyExplanationEdgeAggregationKey(edge)
			index, exists := edgeIndex[key]
			if !exists {
				edge.Period = "annual"
				edge.ID = energyExplanationAnnualEdgeID(edge)
				edgeIndex[key] = len(annual.Edges)
				annual.Edges = append(annual.Edges, edge)
				continue
			}
			current := &annual.Edges[index]
			current.Value = roundedEnergyNumber(current.Value + edge.Value)
			current.SignedValue = roundedEnergyNumber(current.SignedValue + edge.SignedValue)
			current.DisplayValue = roundedEnergyNumber(current.DisplayValue + edge.DisplayValue)
			current.SourceIDs = appendUniqueStrings(current.SourceIDs, edge.SourceIDs...)
			current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, edge.RelatedPathIDs...)
		}
		for _, item := range graph.Reconciliation {
			key := energyExplanationReconciliationAggregationKey(item)
			index, exists := reconciliationIndex[key]
			if !exists {
				item.ID = energyExplanationAnnualReconciliationID(item.ID)
				item.Period = "annual"
				reconciliationIndex[key] = len(annual.Reconciliation)
				annual.Reconciliation = append(annual.Reconciliation, item)
				continue
			}
			current := &annual.Reconciliation[index]
			current.ExpectedValue = roundedEnergyNumber(current.ExpectedValue + item.ExpectedValue)
			current.ExplainedValue = roundedEnergyNumber(current.ExplainedValue + item.ExplainedValue)
			current.ResidualValue = roundedEnergyNumber(current.ResidualValue + item.ResidualValue)
			current.Status = energyReconciliationStatus(current.ExpectedValue, current.ResidualValue)
			current.SourceIDs = appendUniqueStrings(current.SourceIDs, item.SourceIDs...)
		}
		for _, warning := range graph.Warnings {
			warning.Period = "annual"
			annual.Warnings = appendEnergyDriverWarning(annual.Warnings, warning)
		}
	}
	annual.MappedPercent = energyExplanationMappedPercentFromReconciliation(annual.Reconciliation)
	sortEnergyExplanationNodes(annual.Nodes)
	sortEnergyExplanationEdges(annual.Edges)
	return annual
}

func energyExplanationSourcesIntersect(sourceIDs []string, selected map[string]bool) bool {
	for _, sourceID := range sourceIDs {
		if selected[sourceID] {
			return true
		}
	}
	return false
}

func energyExplanationEdgeAggregationKey(edge EnergyExplanationEdge) string {
	return strings.Join([]string{
		edge.FromID,
		edge.ToID,
		normalizeEnergyOutputName(edge.Relation),
		normalizeEnergyOutputName(edge.RuleID),
		normalizeEnergyOutputName(edge.ServiceKind),
		normalizeEnergyOutputName(edge.ZoneName),
		normalizeEnergyOutputName(edge.Basis),
	}, "|")
}

func energyExplanationAnnualEdgeID(edge EnergyExplanationEdge) string {
	prefix := "edge"
	if parts := strings.SplitN(strings.TrimSpace(edge.ID), ".", 2); len(parts) > 0 && parts[0] != "" {
		prefix = parts[0]
	}
	return edgeID(prefix, "annual", edge.FromID, edge.ToID)
}

func energyExplanationReconciliationAggregationKey(item EnergyReconciliation) string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(item.Level)),
		normalizeEnergyOutputName(item.ZoneName),
		normalizeEnergyOutputName(item.ServiceKind),
		normalizeEnergyOutputName(energyExplanationAnnualReconciliationID(item.ID)),
	}, "|")
}

func energyExplanationAnnualReconciliationID(id string) string {
	parts := strings.Split(strings.TrimSpace(id), ".")
	if len(parts) > 0 {
		last := strings.ToLower(parts[len(parts)-1])
		if last == "annual" || strings.HasPrefix(last, "m") {
			parts[len(parts)-1] = "annual"
			return strings.Join(parts, ".")
		}
	}
	return strings.TrimSuffix(id, ".") + ".annual"
}

func energyExplanationMappedPercentFromReconciliation(items []EnergyReconciliation) float64 {
	expected := 0.0
	explained := 0.0
	for _, item := range items {
		if !strings.EqualFold(item.Level, "energy") || !energyExplanationUnitIsSiteEnergy(item.Unit) {
			continue
		}
		expected += item.ExpectedValue
		explained += math.Min(item.ExpectedValue, item.ExplainedValue)
	}
	if expected <= 0 {
		return 0
	}
	return roundedEnergyNumber(explained / expected * 100)
}

func filterEnergyExplanationWaterContextSeries(series []energyExplanationSeries, sources []EnergyDataSource) ([]energyExplanationSeries, []EnergyDataSource) {
	sourceByID := make(map[string]EnergyDataSource, len(sources))
	for _, source := range sources {
		sourceByID[source.ID] = source
	}
	waterSourceIDs := map[string]bool{}
	outSeries := make([]energyExplanationSeries, 0, len(series))
	for _, item := range series {
		if canonicalEnergyPathCarrier(item.Carrier) != "water" {
			outSeries = append(outSeries, item)
			continue
		}
		for _, sourceID := range item.SourceIDs {
			waterSourceIDs[sourceID] = true
		}
		explicitConversion := strings.EqualFold(strings.TrimSpace(item.Basis), "derived_ratio") &&
			energyExplanationUnitIsSiteEnergy(item.Unit) && len(item.SourceIDs) > 0
		if explicitConversion {
			for _, sourceID := range item.SourceIDs {
				if source, ok := sourceByID[sourceID]; !ok || !energyPathSourceHasExplicitWaterConversion(source) {
					explicitConversion = false
					break
				}
			}
		}
		if explicitConversion {
			outSeries = append(outSeries, item)
		}
	}

	outSources := append([]EnergyDataSource(nil), sources...)
	for index := range outSources {
		source := &outSources[index]
		if !waterSourceIDs[source.ID] && !energyExplanationNameIsWaterMeter(firstNonEmpty(source.Name, source.KeyValue)) {
			continue
		}
		source.InspectorSection = "context"
		if energyPathSourceHasExplicitWaterConversion(*source) {
			source.Explanation = firstNonEmpty(source.Explanation, "Water utility source with an explicit site-energy conversion.")
		} else {
			source.Explanation = firstNonEmpty(source.Explanation, "Water utility use is context only and is excluded from the site-energy flow unless an explicit site-energy conversion is provided.")
		}
	}
	return outSeries, outSources
}

func energyExplanationHeatSeriesSignMultiplier(item energyExplanationSeries) float64 {
	if item.heatSignMultiplier != 0 {
		return item.heatSignMultiplier
	}
	if strings.EqualFold(item.HeatSign, "negative") || strings.EqualFold(item.Sign, "negative") {
		return -1
	}
	return 1
}

func sumEnergyExplanationPeriodValues(values map[int]float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total
}

func energyExplanationRawValue(effective float64, multiplier float64) float64 {
	if multiplier == 0 {
		multiplier = 1
	}
	return roundedEnergyNumber(effective / multiplier)
}

func filterEnergyExplanationSeriesForBasicDetail(series []energyExplanationSeries, sources []EnergyDataSource, plan *PurposeRunPlan) ([]energyExplanationSeries, []EnergyDataSource) {
	if plan == nil || !strings.EqualFold(strings.TrimSpace(plan.BasicEnergyDetail), PurposeBasicEnergyDetailLight) {
		return series, sources
	}
	filteredSeries := make([]energyExplanationSeries, 0, len(series))
	referencedSources := map[string]bool{}
	for _, item := range series {
		if item.Level != "energy" {
			continue
		}
		filteredSeries = append(filteredSeries, item)
		for _, sourceID := range item.SourceIDs {
			referencedSources[sourceID] = true
		}
	}
	filteredSources := make([]EnergyDataSource, 0, len(sources))
	for _, source := range sources {
		if referencedSources[source.ID] {
			filteredSources = append(filteredSources, source)
		}
	}
	return filteredSeries, filteredSources
}

func energyExplanationPlanUsesEnergyPath(plan *PurposeRunPlan) bool {
	return plan != nil && strings.EqualFold(strings.TrimSpace(plan.BasicEnergyDetail), PurposeBasicEnergyDetailEnergyPath)
}

// Zone direct-use variables are captured as source records for the scoped v2
// work, but the frozen v1 graph must not add them on top of broad meters. The
// zone-native accounting policy is introduced in the later allocation phase.
func excludeEnergyExplanationDirectUseSeries(series []energyExplanationSeries) []energyExplanationSeries {
	out := make([]energyExplanationSeries, 0, len(series))
	for _, item := range series {
		if strings.EqualFold(strings.TrimSpace(item.Level), "energy") && strings.EqualFold(strings.TrimSpace(item.MeterHierarchyLevel), "zone_direct_use") {
			continue
		}
		out = append(out, item)
	}
	return out
}

func energyExplanationDirectZoneSeries(series []energyExplanationSeries) []energyExplanationSeries {
	out := make([]energyExplanationSeries, 0)
	for _, item := range series {
		item = canonicalEnergyExplanationSeries(item)
		if item.Stage != "end_use" || strings.TrimSpace(item.ZoneName) == "" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(item.MeterHierarchyLevel), "zone_direct_use") && canonicalEnergyPathBasis(item.Basis, "") != "direct_zone_energy" {
			continue
		}
		item.Basis = "direct_zone_energy"
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := strings.Join([]string{normalizeEnergyOutputName(out[i].ZoneName), normalizeEnergyOutputName(out[i].EndUse), normalizeEnergyOutputName(out[i].Carrier), normalizeEnergyOutputName(out[i].CanonicalKind)}, "|")
		right := strings.Join([]string{normalizeEnergyOutputName(out[j].ZoneName), normalizeEnergyOutputName(out[j].EndUse), normalizeEnergyOutputName(out[j].Carrier), normalizeEnergyOutputName(out[j].CanonicalKind)}, "|")
		return left < right
	})
	return out
}

func buildEnergyExplanationSelectedRangePeriod(series []energyExplanationSeries, sources []EnergyDataSource, allocationPolicy string, plan *PurposeRunPlan) (EnergyPeriod, bool) {
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
	graph := buildEnergyExplanationGraphForPeriod("selected_range", energyExplanationSeriesForGraphPeriod(series, "selected_range"), allocationPolicy, func(item energyExplanationSeries) float64 {
		if !item.HasSelectedRange {
			return 0
		}
		return item.SelectedRange
	}, sources)
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

func buildEnergyExplanationSummaryV1(explanation EnergyExplanationV1) EnergyExplanationSummary {
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
			key := energyExplanationLoadSummaryKey(node)
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
	summary.DerivedKPIs = buildEnergyExplanationDerivedKPIs(summary.EnergyByEndUse, summary.DeliveredLoadByService)
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
			ScaleDomain:         node.ScaleDomain,
			AggregationBasis:    node.AggregationBasis,
			ZoneName:            node.ZoneName,
			ServiceKind:         node.ServiceKind,
			PathType:            node.PathType,
			Carrier:             node.Carrier,
			EndUse:              node.EndUse,
			MeterHierarchyLevel: node.MeterHierarchyLevel,
			HeatCategory:        node.HeatCategory,
			Sign:                node.Sign,
			Basis:               node.Basis,
			SourceIDs:           appendUniqueStrings(nil, node.SourceIDs...),
		}
		item = groups[key]
	}
	item.Value = roundedEnergyNumber(item.Value + value)
	item.RawValue = roundedEnergyNumber(item.RawValue + node.RawValue)
	item.AllocatedValue = roundedEnergyNumber(item.AllocatedValue + node.AllocatedValue)
	item.Sign = mergeEnergyExplanationSummarySign(item.Sign, node.Sign)
	item.SourceIDs = appendUniqueStrings(item.SourceIDs, node.SourceIDs...)
}

func mergeEnergyExplanationSummarySign(current string, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if next == "" {
		return current
	}
	if current == "" {
		return next
	}
	if current == next {
		return current
	}
	return "mixed"
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

func energyExplanationLoadSummaryKey(node EnergyExplanationNode) string {
	service := strings.TrimSpace(node.ServiceKind)
	pathType := strings.TrimSpace(node.PathType)
	if service != "" && pathType != "" {
		return service + "." + pathType
	}
	return firstNonEmpty(service, node.Kind, node.ID)
}

func buildEnergyExplanationDerivedKPIs(energyItems, loadItems []EnergyExplanationSummaryItem) []EnergyExplanationSummaryItem {
	definitions := []struct {
		service string
		label   string
		kind    string
	}{
		{service: "cooling", label: "Cooling COP", kind: "kpi.cooling_cop"},
		{service: "heating", label: "Heating COP", kind: "kpi.heating_cop"},
	}
	out := []EnergyExplanationSummaryItem{}
	for _, def := range definitions {
		energy := energyExplanationElectricEndUseForService(energyItems, def.service)
		load := energyExplanationPreferredLoadForService(loadItems, def.service)
		if energy == nil || load == nil || energy.Value <= 0 || load.Value <= 0 {
			continue
		}
		out = append(out, EnergyExplanationSummaryItem{
			ID:               def.kind,
			Level:            "derived_kpi",
			Kind:             def.kind,
			Label:            def.label,
			Value:            roundedEnergyNumber(load.Value / energy.Value),
			ServiceKind:      def.service,
			PathType:         load.PathType,
			Basis:            "derived_kpi",
			Formula:          "delivered_load / electric_end_use_energy",
			NumeratorLabel:   firstNonEmpty(load.Label, load.Kind, load.ID),
			NumeratorValue:   load.Value,
			NumeratorUnit:    load.Unit,
			DenominatorLabel: firstNonEmpty(energy.Label, energy.Kind, energy.ID),
			DenominatorValue: energy.Value,
			DenominatorUnit:  energy.Unit,
			SourceIDs:        appendUniqueStrings(appendUniqueStrings(nil, energy.SourceIDs...), load.SourceIDs...),
		})
	}
	return out
}

func energyExplanationElectricEndUseForService(items []EnergyExplanationSummaryItem, service string) *EnergyExplanationSummaryItem {
	for i := range items {
		item := &items[i]
		if strings.EqualFold(item.EndUse, service) && strings.EqualFold(item.Carrier, "electricity") && item.Value > 0 {
			return item
		}
	}
	return nil
}

func energyExplanationPreferredLoadForService(items []EnergyExplanationSummaryItem, service string) *EnergyExplanationSummaryItem {
	var best *EnergyExplanationSummaryItem
	bestRank := math.MaxInt
	for i := range items {
		item := &items[i]
		if !strings.EqualFold(item.ServiceKind, service) || item.Value <= 0 {
			continue
		}
		rank := energyExplanationLoadPathPriority(item.PathType)
		if best == nil || rank < bestRank || rank == bestRank && math.Abs(item.Value) > math.Abs(best.Value) {
			best = item
			bestRank = rank
		}
	}
	return best
}

func energyExplanationLoadPathPriority(pathType string) int {
	switch strings.ToLower(strings.TrimSpace(pathType)) {
	case "zone":
		return 0
	case "system":
		return 1
	case "plant":
		return 2
	default:
		return 3
	}
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

func buildEnergyExplanationGraphForPeriod(period string, series []energyExplanationSeries, allocationPolicy string, valueFor func(energyExplanationSeries) float64, sourceSets ...[]EnergyDataSource) energyExplanationGraph {
	if len(sourceSets) > 0 {
		series, _ = filterEnergyExplanationWaterContextSeries(series, sourceSets[0])
	}
	canonicalDriverAllocationEnabled := false
	for _, original := range series {
		item := canonicalEnergyExplanationSeries(original)
		if item.Stage == "load" && item.canonicalLoadMetadata && strings.TrimSpace(item.ZoneName) != "" {
			if service := energyCanonicalServiceKind(item.ServiceKind); service == "cooling" || service == "heating" {
				canonicalDriverAllocationEnabled = true
				break
			}
		}
	}
	nodes := map[string]*energyExplanationNodeAccumulator{}
	facilityByCarrier := map[string]string{}
	facilityValueByCarrier := map[string]float64{}
	facilitySourcesByCarrier := map[string][]string{}
	endUseValueByCarrier := map[string]float64{}
	endUseNodesByCarrier := map[string][]string{}
	endUseNodeByTarget := map[string]string{}
	supportNodesByCarrier := map[string][]string{}
	loadNodesByService := map[string][]string{}
	loadNodesByZoneService := map[string][]string{}
	zoneLoadNodesByService := map[string][]string{}
	heatValueByService := map[string]float64{}
	heatSourcesByService := map[string][]string{}
	heatValueByZoneService := map[string]float64{}
	heatSourcesByZoneService := map[string][]string{}
	heatDeviationByZoneService := map[string]float64{}
	internalGainTargetByHeatNode := map[string]energyExplanationInternalGainTarget{}
	addNode := func(node EnergyExplanationNode) {
		if node.ID == "" || node.Value == 0 {
			return
		}
		node.Value = roundedEnergyNumber(node.Value)
		finalizeEnergyExplanationLoadNode(&node)
		finalizeEnergyExplanationSimultaneousLoad(&node)
		existing := nodes[node.ID]
		if existing == nil {
			node.LoadBreakdown = cloneEnergyExplanationLoadComponents(node.LoadBreakdown)
			node.OffsetEffects = cloneEnergyExplanationOffsetEffects(node.OffsetEffects)
			node.SimultaneousLoad = cloneEnergyExplanationSimultaneousLoad(node.SimultaneousLoad)
			node.allocationSourceIDs = appendUniqueStrings(nil, node.allocationSourceIDs...)
			node.simultaneousLoadContributions = cloneEnergyExplanationSimultaneousLoadContributions(node.simultaneousLoadContributions)
			nodes[node.ID] = &energyExplanationNodeAccumulator{node: node}
			return
		}
		existing.node.Value = roundedEnergyNumber(existing.node.Value + node.Value)
		existing.node.SignedValue = roundedEnergyNumber(existing.node.SignedValue + node.SignedValue)
		existing.node.RawValue = roundedEnergyNumber(existing.node.RawValue + node.RawValue)
		existing.node.EffectiveValue = roundedEnergyNumber(existing.node.EffectiveValue + node.EffectiveValue)
		if existing.node.RawValue != 0 && existing.node.EffectiveValue != 0 {
			existing.node.Multiplier = roundedEnergyNumber(existing.node.EffectiveValue / existing.node.RawValue)
		}
		existing.node.DisplayValue = roundedEnergyNumber(existing.node.DisplayValue + node.DisplayValue)
		existing.node.SourceIDs = appendUniqueStrings(existing.node.SourceIDs, node.SourceIDs...)
		existing.node.allocationSourceIDs = appendUniqueStrings(existing.node.allocationSourceIDs, node.allocationSourceIDs...)
		existing.node.RelatedEntityIDs = appendUniqueStrings(existing.node.RelatedEntityIDs, node.RelatedEntityIDs...)
		existing.node.RelatedPathIDs = appendUniqueStrings(existing.node.RelatedPathIDs, node.RelatedPathIDs...)
		existing.node.LoadBreakdown = mergeEnergyExplanationLoadComponents(existing.node.LoadBreakdown, node.LoadBreakdown)
		existing.node.OffsetEffects = mergeEnergyExplanationOffsetEffects(existing.node.OffsetEffects, node.OffsetEffects)
		existing.node.simultaneousLoadContributions = mergeEnergyExplanationSimultaneousLoadContributions(existing.node.simultaneousLoadContributions, node.simultaneousLoadContributions)
		if existing.node.ThermalComponent != node.ThermalComponent {
			existing.node.ThermalComponent = "combined"
		}
		if existing.node.PathType == "" {
			existing.node.PathType = node.PathType
		}
		finalizeEnergyExplanationLoadNode(&existing.node)
		finalizeEnergyExplanationSimultaneousLoad(&existing.node)
	}
	for _, item := range series {
		item = canonicalEnergyExplanationSeries(item)
		value := valueFor(item)
		if value == 0 {
			continue
		}
		rawValue, effectiveValue, effectiveMultiplier := energyExplanationNodeAccounting(item, value)
		switch item.Stage {
		case "carrier", "end_use", "support":
			nodeID := energyExplanationEnergyNodeID(item)
			addNode(EnergyExplanationNode{
				ID:                  nodeID,
				Level:               "energy",
				Kind:                item.Kind,
				Label:               item.Label,
				Value:               value,
				RawValue:            rawValue,
				EffectiveValue:      effectiveValue,
				Multiplier:          effectiveMultiplier,
				Unit:                item.Unit,
				Period:              period,
				Carrier:             item.Carrier,
				EndUse:              item.EndUse,
				ZoneName:            item.ZoneName,
				MeterHierarchyLevel: item.MeterHierarchyLevel,
				Basis:               item.Basis,
				SourceIDs:           appendUniqueStrings(item.SourceIDs, item.DriverInputSourceIDs...),
			})
			if item.Stage == "carrier" {
				facilityByCarrier[item.Carrier] = nodeID
				facilityValueByCarrier[item.Carrier] += value
				facilitySourcesByCarrier[item.Carrier] = appendUniqueStrings(facilitySourcesByCarrier[item.Carrier], item.SourceIDs...)
			} else if item.Stage == "support" {
				supportNodesByCarrier[item.Carrier] = appendUniqueStrings(supportNodesByCarrier[item.Carrier], nodeID)
			} else {
				endUseValueByCarrier[item.Carrier] += value
				endUseNodesByCarrier[item.Carrier] = appendUniqueStrings(endUseNodesByCarrier[item.Carrier], nodeID)
				endUseNodeByTarget[energyExplanationEndUseCarrierKey(item.EndUse, item.Carrier)] = nodeID
			}
		case "load":
			nodeID := energyExplanationLoadNodeID(item)
			thermalComponent := ""
			if item.canonicalLoadMetadata || strings.EqualFold(strings.TrimSpace(item.ThermalComponent), "combined") {
				// Frozen v1 nodes did not expose a component for their historical
				// sensible-only load. Canonical Energy Path builds opt in through
				// the private marker; an explicit combined total is safe in either
				// path because it did not exist in the frozen alias catalog.
				thermalComponent = strings.ToLower(strings.TrimSpace(item.ThermalComponent))
			}
			addNode(EnergyExplanationNode{
				ID:               nodeID,
				Level:            "load",
				Kind:             item.Kind,
				Label:            item.Label,
				Value:            value,
				RawValue:         rawValue,
				EffectiveValue:   effectiveValue,
				Multiplier:       effectiveMultiplier,
				Unit:             item.Unit,
				Period:           period,
				ZoneName:         item.ZoneName,
				LoopName:         item.LoopName,
				ServiceKind:      item.ServiceKind,
				PathType:         item.PathType,
				DriverCategory:   item.DriverCategory,
				ThermalComponent: thermalComponent,
				LoadBreakdown:    energyExplanationLoadComponentsForPeriod(item, period, value),
				Basis:            item.Basis,
				SourceIDs:        item.SourceIDs,
			})
			loadNodesByService[item.ServiceKind] = appendUniqueStrings(loadNodesByService[item.ServiceKind], nodeID)
			if item.ZoneName != "" && item.ServiceKind != "" {
				key := energyExplanationZoneServiceKey(item.ZoneName, item.ServiceKind)
				loadNodesByZoneService[key] = appendUniqueStrings(loadNodesByZoneService[key], nodeID)
				zoneLoadNodesByService[item.ServiceKind] = appendUniqueStrings(zoneLoadNodesByService[item.ServiceKind], nodeID)
			}
		case "driver":
			if item.DriverSourceRole != "" && item.DriverSourceRole != energyDriverSourceRoleMainFlow {
				// Reconciliation and context series remain available to the
				// period accounting pass below, but never become additive graph
				// nodes or driver-to-load links.
				continue
			}
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
			nodeID := energyExplanationHeatNodeID(item)
			if energyDriverIsSyntheticPreAllocationClosure(EnergyExplanationNode{Kind: item.Kind}) {
				// Keep the tautological pre-allocation closure separate from other
				// balance.storage_other components. Otherwise addNode could merge it
				// into a physical/reconciliation pressure and make allocation depend
				// on input order and whichever Kind happened to be retained.
				nodeID += ".preallocation_closure"
			}
			if item.DriverCategory != "" && item.HeatSign == "" {
				nodeID += "." + sign
			}
			addNode(EnergyExplanationNode{
				ID:                  nodeID,
				Level:               "heat",
				Kind:                item.Kind,
				Label:               item.Label,
				Value:               displayValue,
				SignedValue:         signedValue,
				RawValue:            math.Abs(rawValue),
				EffectiveValue:      displayValue,
				Multiplier:          effectiveMultiplier,
				DisplayValue:        displayValue,
				Unit:                item.Unit,
				Period:              period,
				ZoneName:            item.ZoneName,
				ServiceKind:         serviceKind,
				DriverCategory:      item.DriverCategory,
				ThermalComponent:    item.ThermalComponent,
				HeatCategory:        item.HeatCategory,
				Sign:                sign,
				Basis:               "derived_balance",
				RelatedEntityIDs:    appendUniqueStrings(nil, item.RelatedEntityIDs...),
				SourceIDs:           appendUniqueStrings(item.SourceIDs, item.DriverInputSourceIDs...),
				allocationSourceIDs: appendUniqueStrings(nil, item.SourceIDs...),
				driverZoneOnly:      item.driverZoneOnly,
				driverBuildingOnly:  item.driverBuildingOnly,
			})
			if target, ok := energyExplanationInternalGainEnergyTarget(item); ok {
				internalGainTargetByHeatNode[nodeID] = target
			}
		}
	}
	canonicalDriverAllocation := allocateCanonicalEnergyDriverNodes(nodes, loadNodesByZoneService, canonicalDriverAllocationEnabled)
	for carrier, endUseValue := range endUseValueByCarrier {
		if facilityByCarrier[carrier] != "" || endUseValue == 0 {
			continue
		}
		sourceIDs := []string{}
		unit := ""
		for _, endUseID := range endUseNodesByCarrier[carrier] {
			if endUseNode := nodes[endUseID]; endUseNode != nil {
				sourceIDs = appendUniqueStrings(sourceIDs, endUseNode.node.SourceIDs...)
				unit = firstNonEmpty(unit, endUseNode.node.Unit)
			}
		}
		nodeID := "energy.carrier." + carrier
		addNode(EnergyExplanationNode{
			ID:                  nodeID,
			Level:               "energy",
			Kind:                "energy." + carrier + ".observed_end_use_subtotal",
			Label:               energyCarrierLabel(carrier) + " observed end-use subtotal",
			Value:               endUseValue,
			RawValue:            endUseValue,
			EffectiveValue:      endUseValue,
			Multiplier:          1,
			Unit:                unit,
			Period:              period,
			Carrier:             carrier,
			EndUse:              "total",
			MeterHierarchyLevel: "observed_end_use_subtotal",
			Badges:              []string{"partial", "observed_end_use_subtotal"},
			Basis:               "reported_end_use_subtotal",
			SourceIDs:           sourceIDs,
		})
		facilityByCarrier[carrier] = nodeID
		facilityValueByCarrier[carrier] = endUseValue
		facilitySourcesByCarrier[carrier] = appendUniqueStrings(facilitySourcesByCarrier[carrier], sourceIDs...)
	}

	edges := []EnergyExplanationEdge{}
	reconciliation := []EnergyReconciliation{}
	warnings := []EnergyWarning{}
	meterEndUseRule := energyRelationshipRuleByID(energyRelationshipRuleMeterEndUse)
	measuredEnergyVariableRule := energyRelationshipRuleByID(energyRelationshipRuleMeasuredEnergyVariable)
	purchasedElectricityRule := energyRelationshipRuleByID(energyRelationshipRulePurchasedElectricity)
	onsiteProductionRule := energyRelationshipRuleByID(energyRelationshipRuleOnsiteProduction)
	storageDischargeRule := energyRelationshipRuleByID(energyRelationshipRuleStorageDischarge)
	soldElectricityRule := energyRelationshipRuleByID(energyRelationshipRuleSoldElectricity)
	measuredLoadRule := energyRelationshipRuleByID(energyRelationshipRuleMeasuredLoad)
	allocatedZoneLoadRule := energyRelationshipRuleByID(energyRelationshipRuleAllocatedZoneLoad)
	heatDriverRule := energyRelationshipRuleByID(energyRelationshipRuleHeatDriverBalance)
	internalGainHeatRule := energyRelationshipRuleByID(energyRelationshipRuleInternalGainHeat)
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
			rule := meterEndUseRule
			relation := "meter_enduse"
			edgeBasis := rule.Basis
			edgeFormula := rule.Formula
			edgeRuleID := rule.ID
			if endUseNode.node.Basis == measuredEnergyVariableRule.Basis {
				rule = measuredEnergyVariableRule
				relation = "energy_variable"
				edgeBasis = rule.Basis
				edgeFormula = rule.Formula
				edgeRuleID = rule.ID
			} else if strings.EqualFold(strings.TrimSpace(endUseNode.node.Basis), "derived_ratio") {
				edgeBasis = "derived_ratio"
				edgeFormula = "reported source quantity multiplied by an explicit site-energy conversion factor"
				edgeRuleID = ""
			}
			energySourceIDs = appendUniqueStrings(energySourceIDs, endUseNode.node.SourceIDs...)
			edges = append(edges, EnergyExplanationEdge{
				ID:        edgeID("edge", period, facilityID, endUseID),
				FromID:    facilityID,
				ToID:      endUseID,
				Value:     endUseNode.node.Value,
				Unit:      endUseNode.node.Unit,
				Period:    period,
				Relation:  relation,
				Basis:     edgeBasis,
				Formula:   edgeFormula,
				RuleID:    edgeRuleID,
				SourceIDs: endUseNode.node.SourceIDs,
			})
		}
		for _, supportID := range supportNodesByCarrier[carrier] {
			supportNode := nodes[supportID]
			if supportNode == nil {
				continue
			}
			rule := onsiteProductionRule
			relation := "onsite_production"
			edgePrefix := "production"
			switch supportNode.node.EndUse {
			case "electricity_purchased":
				rule = purchasedElectricityRule
				relation = "purchased_electricity"
				edgePrefix = "supply"
			case "storage_discharge":
				rule = storageDischargeRule
				relation = "storage_discharge"
				edgePrefix = "support"
			case "electricity_sold":
				rule = soldElectricityRule
				relation = "sold_electricity"
				edgePrefix = "export"
			}
			edges = append(edges, EnergyExplanationEdge{
				ID:        edgeID(edgePrefix, period, facilityID, supportID),
				FromID:    facilityID,
				ToID:      supportID,
				Value:     supportNode.node.Value,
				Unit:      supportNode.node.Unit,
				Period:    period,
				Relation:  relation,
				Basis:     rule.Basis,
				Formula:   rule.Formula,
				RuleID:    rule.ID,
				SourceIDs: supportNode.node.SourceIDs,
			})
		}
		residual := roundedEnergyNumber(facilityValue - endUseValue)
		residualAbs := math.Abs(residual)
		reconciliation = append(reconciliation, EnergyReconciliation{
			ID:             "reconcile.energy." + carrier + "." + period,
			Level:          "energy",
			Period:         period,
			Label:          energyCarrierLabel(carrier) + " total basis",
			Status:         energyReconciliationStatus(facilityValue, residual),
			ExpectedValue:  roundedEnergyNumber(facilityValue),
			ExplainedValue: roundedEnergyNumber(endUseValue),
			ResidualValue:  residual,
			Unit:           nodes[facilityID].node.Unit,
			Basis:          "residual",
			Formula:        "facility carrier total - mapped broad end-use meters",
			SourceIDs:      energySourceIDs,
		})
		if energyCarrierResidualVisibleInGraph(facilityValue, residual) {
			residualID := "residual.energy." + carrier
			addNode(EnergyExplanationNode{
				ID:        residualID,
				Level:     "residual",
				Kind:      "energy.residual",
				Label:     "Unclassified energy",
				Value:     residualAbs,
				Unit:      nodes[facilityID].node.Unit,
				Period:    period,
				Carrier:   carrier,
				Badges:    []string{"unclassified_energy"},
				Basis:     energyResidualRule.Basis,
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
		if node.node.AllocationApplied && node.node.Value == 0 {
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
			if node.node.Kind == "heat.zone_balance_residual" {
				heatDeviationByZoneService[key] += node.node.DisplayValue
			}
		}
		edgeBasis := heatDriverRule.Basis
		edgeFormula := heatDriverRule.Formula
		if node.node.AllocationApplied {
			edgeBasis = "heat_balance_share"
			edgeFormula = energyDriverAllocationExplanation
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
			Basis:        edgeBasis,
			Formula:      edgeFormula,
			RuleID:       heatDriverRule.ID,
			SourceIDs:    node.node.SourceIDs,
			ZoneName:     node.node.ZoneName,
			ServiceKind:  node.node.ServiceKind,
		})
	}
	for heatID, target := range internalGainTargetByHeatNode {
		heatNode := nodes[heatID]
		if heatNode == nil || canonicalDriverAllocation && heatNode.node.Value == 0 {
			continue
		}
		fromID := endUseNodeByTarget[energyExplanationEndUseCarrierKey(target.endUse, target.carrier)]
		fromNode := nodes[fromID]
		if fromNode == nil {
			continue
		}
		edges = append(edges, EnergyExplanationEdge{
			ID:           edgeID("internal_gain", period, fromID, heatID),
			FromID:       fromID,
			ToID:         heatID,
			Value:        heatNode.node.Value,
			SignedValue:  heatNode.node.SignedValue,
			DisplayValue: heatNode.node.DisplayValue,
			Unit:         heatNode.node.Unit,
			Period:       period,
			Relation:     "internal_gain_heat",
			Basis:        internalGainHeatRule.Basis,
			Formula:      internalGainHeatRule.Formula,
			RuleID:       internalGainHeatRule.ID,
			SourceIDs:    appendUniqueStrings(fromNode.node.SourceIDs, heatNode.node.SourceIDs...),
			ZoneName:     heatNode.node.ZoneName,
			ServiceKind:  heatNode.node.ServiceKind,
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
			Status:         energyReconciliationStatus(loadValue, residual),
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
		status := energyReconciliationStatus(loadValue, residual)
		sourceIDs := appendUniqueStrings(loadSources, heatSourcesByZoneService[key]...)
		reconciliation = append(reconciliation, EnergyReconciliation{
			ID:             "reconcile.heat." + serviceKind + "." + zoneID + "." + period,
			Level:          "heat",
			Period:         period,
			Label:          energyServiceLabel(serviceKind) + " heat-driver basis - " + zoneName,
			Status:         status,
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
		if status != "balanced" {
			warnings = append(warnings, EnergyWarning{
				Severity: "warning",
				Code:     "zone_heat_residual_gap",
				Message:  fmt.Sprintf("%s %s heat-driver reconciliation has %s residual %g %s.", zoneName, energyServiceLabel(serviceKind), status, residual, loadUnit),
				Period:   period,
			})
		}
		deviationValue := roundedEnergyNumber(heatDeviationByZoneService[key])
		if deviationValue > energyResidualVisibilityThreshold(loadValue) {
			warnings = append(warnings, EnergyWarning{
				Severity: "warning",
				Code:     "heat_balance_deviation_large",
				Message:  fmt.Sprintf("%s %s heat-balance deviation is %g %s for this period.", zoneName, energyServiceLabel(serviceKind), deviationValue, loadUnit),
				Period:   period,
			})
		}
	}
	reconciliation, warnings = appendEnergyDriverPeriodAccounting(period, series, valueFor, reconciliation, warnings)

	outNodes := make([]EnergyExplanationNode, 0, len(nodes))
	for _, node := range nodes {
		outNodes = append(outNodes, node.node)
	}
	sortEnergyExplanationNodes(outNodes)
	sortEnergyExplanationEdges(edges)
	mapped := 0.0
	totalFacility := 0.0
	for carrier, value := range facilityValueByCarrier {
		facilityNode := nodes[facilityByCarrier[carrier]]
		if facilityNode == nil || !energyExplanationUnitIsSiteEnergy(facilityNode.node.Unit) {
			continue
		}
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

func energyExplanationNodeAccounting(item energyExplanationSeries, effectiveValue float64) (float64, float64, float64) {
	if !item.multiplierApplied {
		return 0, 0, 0
	}
	multiplier := positiveEnergyMultiplier(item.EffectiveMultiplier)
	return energyExplanationRawValue(effectiveValue, multiplier), effectiveValue, multiplier
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

func applyEnergyExplanationServicePathLoadShareAllocation(explanation EnergyExplanationResult) EnergyExplanationResult {
	if explanation.AllocationPolicy != PurposeAllocationPolicyByServicePathLoadShare {
		return explanation
	}
	rule := energyRelationshipRuleByID(energyRelationshipRuleAllocatedServicePathLoad)
	explanation.Edges = servicePathLoadShareAllocatedEdges(explanation.Nodes, explanation.Edges, rule)
	for periodIndex := range explanation.Periods {
		explanation.Periods[periodIndex].Edges = servicePathLoadShareAllocatedEdges(explanation.Periods[periodIndex].Nodes, explanation.Periods[periodIndex].Edges, rule)
	}
	return explanation
}

func applyEnergyExplanationV1ServicePathLoadShareAllocation(explanation EnergyExplanationV1) EnergyExplanationV1 {
	if explanation.AllocationPolicy != PurposeAllocationPolicyByServicePathLoadShare {
		return explanation
	}
	annualOriginalEdges := append([]EnergyExplanationEdge(nil), explanation.Edges...)
	// Preserve the historical full-meter Building conversion privately. The
	// public v1-compatible edge collection now carries the direct-first Zone
	// allocation ledger, while UpgradeEnergyExplanationV1 uses this sidecar only
	// for the Building graph so direct zone observations are not counted twice.
	annualBuildingPlan := buildEnergyPathZoneHVACAllocationPlan(explanation.Nodes, annualOriginalEdges, nil, "annual", "annual", explanation.canonicalMonthlyBasis)
	explanation.buildingHVACAllocationEdges = applyEnergyPathZoneHVACAllocationPlan(annualOriginalEdges, explanation.Nodes, annualBuildingPlan)
	explanation.buildingHVACAllocationPeriodEdges = map[string][]EnergyExplanationEdge{}
	annualPlan := buildEnergyPathZoneHVACAllocationPlan(explanation.Nodes, annualOriginalEdges, explanation.zoneDirectUseSeries, "annual", "annual", explanation.canonicalMonthlyBasis)
	explanation.Edges = applyEnergyPathZoneHVACAllocationPlan(annualOriginalEdges, explanation.Nodes, annualPlan)
	monthlyPlans := []energyPathZoneHVACAllocationPlan{}
	monthlyBuildingPlans := []energyPathZoneHVACAllocationPlan{}
	for periodIndex := range explanation.Periods {
		period := &explanation.Periods[periodIndex]
		originalEdges := append([]EnergyExplanationEdge(nil), period.Edges...)
		buildingPlan := buildEnergyPathZoneHVACAllocationPlan(period.Nodes, originalEdges, nil, period.ID, period.Kind, explanation.canonicalMonthlyBasis)
		explanation.buildingHVACAllocationPeriodEdges[strings.ToLower(strings.TrimSpace(period.ID))] = applyEnergyPathZoneHVACAllocationPlan(originalEdges, period.Nodes, buildingPlan)
		plan := buildEnergyPathZoneHVACAllocationPlan(period.Nodes, originalEdges, explanation.zoneDirectUseSeries, period.ID, period.Kind, explanation.canonicalMonthlyBasis)
		period.Edges = applyEnergyPathZoneHVACAllocationPlan(originalEdges, period.Nodes, plan)
		if strings.EqualFold(strings.TrimSpace(period.Kind), "monthly") {
			monthlyPlans = append(monthlyPlans, plan)
			monthlyBuildingPlans = append(monthlyBuildingPlans, buildingPlan)
		}
	}
	if explanation.canonicalMonthlyBasis {
		if len(monthlyPlans) > 0 {
			monthlyPlan := aggregateEnergyPathZoneHVACAllocationPlans(monthlyPlans)
			annualPlan = energyPathZoneHVACAllocationPlanWithAnnualFallback(monthlyPlan, annualPlan, explanation.Nodes)
			explanation.Edges = applyEnergyPathZoneHVACAllocationPlan(annualOriginalEdges, explanation.Nodes, annualPlan)
		}
		if len(monthlyBuildingPlans) > 0 {
			monthlyBuildingPlan := aggregateEnergyPathZoneHVACAllocationPlans(monthlyBuildingPlans)
			annualBuildingPlan = energyPathZoneHVACAllocationPlanWithAnnualFallback(monthlyBuildingPlan, annualBuildingPlan, explanation.Nodes)
			explanation.buildingHVACAllocationEdges = applyEnergyPathZoneHVACAllocationPlan(annualOriginalEdges, explanation.Nodes, annualBuildingPlan)
		}
		for periodIndex := range explanation.Periods {
			if strings.EqualFold(explanation.Periods[periodIndex].Kind, "annual") || strings.EqualFold(explanation.Periods[periodIndex].ID, "annual") {
				explanation.Periods[periodIndex].Edges = append([]EnergyExplanationEdge(nil), explanation.Edges...)
				explanation.buildingHVACAllocationPeriodEdges[strings.ToLower(strings.TrimSpace(explanation.Periods[periodIndex].ID))] = append([]EnergyExplanationEdge(nil), explanation.buildingHVACAllocationEdges...)
			}
		}
	}
	return explanation
}

func annualServicePathAllocationEdgesFromMonthly(annual []EnergyExplanationEdge, periods []EnergyPeriod, ruleID string) []EnergyExplanationEdge {
	rule := energyRelationshipRuleByID(ruleID)
	annualFormula := strings.TrimSpace(rule.Formula + "; annual sum of monthly service path allocations")
	monthlyByKey := map[string]EnergyExplanationEdge{}
	monthlyOrder := []string{}
	for _, period := range periods {
		if !strings.EqualFold(period.Kind, "monthly") {
			continue
		}
		for _, edge := range period.Edges {
			if edge.RuleID != ruleID || !strings.EqualFold(edge.Relation, "allocation") {
				continue
			}
			key := energyExplanationEdgeAggregationKey(edge)
			current, exists := monthlyByKey[key]
			if !exists {
				edge.Period = "annual"
				edge.ID = energyExplanationAnnualEdgeID(edge)
				edge.Formula = annualFormula
				monthlyByKey[key] = edge
				monthlyOrder = append(monthlyOrder, key)
				continue
			}
			current.Value = roundedEnergyNumber(current.Value + edge.Value)
			current.SignedValue = roundedEnergyNumber(current.SignedValue + edge.SignedValue)
			current.DisplayValue = roundedEnergyNumber(current.DisplayValue + edge.DisplayValue)
			current.SourceIDs = appendUniqueStrings(current.SourceIDs, edge.SourceIDs...)
			current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, edge.RelatedPathIDs...)
			current.Formula = annualFormula
			monthlyByKey[key] = current
		}
	}
	if len(monthlyByKey) == 0 {
		return annual
	}
	out := make([]EnergyExplanationEdge, 0, len(annual)+len(monthlyByKey))
	emitted := map[string]bool{}
	for _, edge := range annual {
		if edge.RuleID != ruleID || !strings.EqualFold(edge.Relation, "allocation") {
			out = append(out, edge)
			continue
		}
		key := energyExplanationEdgeAggregationKey(edge)
		monthly, exists := monthlyByKey[key]
		if !exists {
			out = append(out, edge)
			continue
		}
		if !emitted[key] {
			out = append(out, monthly)
			emitted[key] = true
		}
	}
	for _, key := range monthlyOrder {
		if !emitted[key] {
			out = append(out, monthlyByKey[key])
		}
	}
	sortEnergyExplanationEdges(out)
	return out
}

func servicePathLoadShareAllocatedEdges(nodes []EnergyExplanationNode, edges []EnergyExplanationEdge, rule EnergyRelationshipRule) []EnergyExplanationEdge {
	nodeByID := map[string]EnergyExplanationNode{}
	for _, node := range nodes {
		if node.ID != "" {
			nodeByID[node.ID] = node
		}
	}
	groups := map[string][]EnergyExplanationEdge{}
	groupOrder := []string{}
	edgeGroup := map[string]string{}
	for _, edge := range edges {
		if !isServicePathLoadShareCandidate(edge, nodeByID) {
			continue
		}
		key := strings.Join([]string{edge.Period, edge.FromID, servicePathLoadShareEdgeServiceKind(edge, nodeByID)}, "\x00")
		if _, ok := groups[key]; !ok {
			groupOrder = append(groupOrder, key)
		}
		groups[key] = append(groups[key], edge)
		edgeGroup[edge.ID] = key
	}
	if len(groups) == 0 {
		return edges
	}
	allocatedGroups := map[string][]EnergyExplanationEdge{}
	for _, key := range groupOrder {
		allocatedGroups[key] = servicePathLoadShareAllocatedGroup(groups[key], nodeByID, rule)
	}
	out := make([]EnergyExplanationEdge, 0, len(edges))
	emittedGroups := map[string]bool{}
	for _, edge := range edges {
		key := edgeGroup[edge.ID]
		if key == "" {
			out = append(out, edge)
			continue
		}
		if emittedGroups[key] {
			continue
		}
		out = append(out, allocatedGroups[key]...)
		emittedGroups[key] = true
	}
	return out
}

func isServicePathLoadShareCandidate(edge EnergyExplanationEdge, nodeByID map[string]EnergyExplanationNode) bool {
	if edge.Relation != "delivered_load" || edge.Basis != "measured_variable" || edge.FromID == "" || edge.ToID == "" {
		return false
	}
	fromNode, fromOK := nodeByID[edge.FromID]
	toNode, toOK := nodeByID[edge.ToID]
	if !fromOK || !toOK || fromNode.Level != "energy" || toNode.Level != "load" {
		return false
	}
	return energyCanonicalServiceKind(firstNonEmpty(edge.ServiceKind, toNode.ServiceKind, fromNode.EndUse)) != ""
}

func servicePathLoadShareEdgeServiceKind(edge EnergyExplanationEdge, nodeByID map[string]EnergyExplanationNode) string {
	fromNode := nodeByID[edge.FromID]
	toNode := nodeByID[edge.ToID]
	return energyCanonicalServiceKind(firstNonEmpty(edge.ServiceKind, toNode.ServiceKind, fromNode.EndUse))
}

func servicePathLoadShareAllocatedGroup(group []EnergyExplanationEdge, nodeByID map[string]EnergyExplanationNode, rule EnergyRelationshipRule) []EnergyExplanationEdge {
	if len(group) == 0 {
		return group
	}
	fromNode := nodeByID[group[0].FromID]
	if fromNode.ID == "" || fromNode.Value == 0 {
		return append([]EnergyExplanationEdge(nil), group...)
	}
	type servicePathTarget struct {
		edge  EnergyExplanationEdge
		node  EnergyExplanationNode
		paths []string
		value float64
	}
	targets := []servicePathTarget{}
	totalLoad := 0.0
	for _, edge := range group {
		targetNode := nodeByID[edge.ToID]
		paths := appendUniqueStrings(targetNode.RelatedPathIDs, edge.RelatedPathIDs...)
		if len(paths) == 0 {
			return append([]EnergyExplanationEdge(nil), group...)
		}
		value := math.Abs(edge.Value)
		if value == 0 {
			value = math.Abs(targetNode.Value)
		}
		if value == 0 {
			return append([]EnergyExplanationEdge(nil), group...)
		}
		totalLoad += value
		targets = append(targets, servicePathTarget{edge: edge, node: targetNode, paths: paths, value: value})
	}
	if totalLoad == 0 {
		return append([]EnergyExplanationEdge(nil), group...)
	}
	allocated := make([]EnergyExplanationEdge, 0, len(targets))
	for _, target := range targets {
		share := target.value / totalLoad
		value := roundedEnergyNumber(fromNode.Value * share)
		if value == 0 {
			continue
		}
		serviceKind := energyCanonicalServiceKind(firstNonEmpty(target.edge.ServiceKind, target.node.ServiceKind, fromNode.EndUse))
		sourceIDs := appendUniqueStrings(fromNode.SourceIDs, target.edge.SourceIDs...)
		sourceIDs = appendUniqueStrings(sourceIDs, target.node.SourceIDs...)
		allocated = append(allocated, EnergyExplanationEdge{
			ID:             edgeID("allocation_service_path", target.edge.Period, target.edge.FromID, target.edge.ToID),
			FromID:         target.edge.FromID,
			ToID:           target.edge.ToID,
			Value:          value,
			Unit:           firstNonEmpty(fromNode.Unit, target.edge.Unit, target.node.Unit),
			Period:         target.edge.Period,
			Relation:       "allocation",
			Basis:          rule.Basis,
			Formula:        fmt.Sprintf("%s; service path load share %.6f; service paths %s", rule.Formula, share, strings.Join(target.paths, ", ")),
			RuleID:         rule.ID,
			SourceIDs:      sourceIDs,
			ZoneName:       target.node.ZoneName,
			ServiceKind:    serviceKind,
			RelatedPathIDs: target.paths,
		})
	}
	if len(allocated) == 0 {
		return append([]EnergyExplanationEdge(nil), group...)
	}
	return allocated
}

func sqlEnergyExplanationDictionaries(db *sql.DB, sourceFile string, plan *PurposeRunPlan) ([]energyExplanationDictionary, error) {
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
		if def, ok := energyMeterAliasOrOtherDefinitionForName(firstNonEmpty(row.keyValue, row.name)); ok {
			copy := def
			dictionary.meter = &copy
			dictionary.isMeter = true
		} else if def, ok := energyVariableAliasDefinitionForName(row.name); ok {
			// Zone direct-use variables are part of the new Energy Path capture
			// contract. Keep legacy parser results stable unless the run plan
			// explicitly selected that contract.
			if def.HierarchyLevel == "zone_direct_use" && !energyExplanationPlanUsesEnergyPath(plan) {
				continue
			}
			copy := def
			dictionary.energy = &copy
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
			Basis:               "measured_meter",
			SourceIDs:           []string{sourceID},
			Total:               roundedEnergyNumber(builder.total),
			Monthly:             roundedEnergyExplanationMonthly(builder.monthly),
			Daily:               roundedEnergyExplanationDaily(builder.daily),
			Hourly:              roundedEnergyExplanationHourly(builder.hourly),
			SelectedRange:       roundedEnergyNumber(builder.selectedRange),
			HasSelectedRange:    builder.hasSelectedRange,
			sourceKeyValue:      strings.TrimSpace(dictionary.row.keyValue),
			sourceName:          strings.TrimSpace(firstNonEmpty(dictionary.row.keyValue, dictionary.row.name)),
			sourceFrequency:     strings.TrimSpace(dictionary.reportingFrequency),
			sourcePriority:      energyAliasPriority(firstNonEmpty(dictionary.row.keyValue, dictionary.row.name), def.Aliases),
		}
	}
	if dictionary.energy != nil {
		def := dictionary.energy
		zoneName := ""
		if def.HierarchyLevel == "zone_direct_use" {
			zoneName = strings.TrimSpace(dictionary.row.keyValue)
		}
		return energyExplanationSeries{
			Level:               "energy",
			Kind:                def.Kind,
			Label:               def.Label,
			Unit:                builder.unit,
			Carrier:             def.Carrier,
			EndUse:              def.EndUse,
			MeterHierarchyLevel: def.HierarchyLevel,
			ZoneName:            zoneName,
			Basis:               "measured_energy_variable",
			SourceIDs:           []string{sourceID},
			Total:               roundedEnergyNumber(builder.total),
			Monthly:             roundedEnergyExplanationMonthly(builder.monthly),
			Daily:               roundedEnergyExplanationDaily(builder.daily),
			Hourly:              roundedEnergyExplanationHourly(builder.hourly),
			SelectedRange:       roundedEnergyNumber(builder.selectedRange),
			HasSelectedRange:    builder.hasSelectedRange,
			sourceKeyValue:      strings.TrimSpace(dictionary.row.keyValue),
			sourceName:          strings.TrimSpace(dictionary.row.name),
			sourceFrequency:     strings.TrimSpace(dictionary.reportingFrequency),
			sourcePriority:      energyAliasPriority(dictionary.row.name, def.Aliases),
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
			PathType:         def.Scope,
			ZoneName:         zoneName,
			LoopName:         loopName,
			ThermalComponent: energyExplanationLoadThermalComponent(dictionary.row.name, def.Kind),
			Basis:            energyExplanationLoadSourceBasis(dictionary),
			SourceIDs:        []string{sourceID},
			Total:            roundedEnergyNumber(builder.total),
			Monthly:          roundedEnergyExplanationMonthly(builder.monthly),
			Daily:            roundedEnergyExplanationDaily(builder.daily),
			Hourly:           roundedEnergyExplanationHourly(builder.hourly),
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
	if def.ObjectScoped || def.SurfaceScoped {
		zoneName = ""
	}
	heatSign := energyHeatAliasExplicitSign(dictionary.row.name)
	signMultiplier := energyHeatSignMultiplier(heatSign)
	heatName := normalizeEnergyOutputName(dictionary.row.name)
	if strings.Contains(heatName, "surface inside face convection") {
		// EnergyPlus reports this from the surface-face perspective: positive
		// means zone air gives heat to the surface. Main drivers use the
		// opposite, surface-to-zone-air sign convention.
		signMultiplier *= -1
	}
	if strings.Contains(heatName, "zone air heat balance air energy storage") {
		// Zone heat balance reports storage on the equation's storage side.
		// Driver pressure is the opposite remaining contribution: -storage.
		signMultiplier *= -1
	}
	return energyExplanationSeries{
		Level:              "heat",
		Kind:               def.Kind,
		Label:              energyHeatAliasLabel(def.Label, heatSign),
		Unit:               builder.unit,
		ZoneName:           zoneName,
		HeatCategory:       def.HeatCategory,
		ThermalComponent:   energyHeatThermalComponent(dictionary.row.name),
		SurfaceScoped:      def.SurfaceScoped,
		HeatSign:           heatSign,
		Basis:              "derived_balance",
		SourceIDs:          []string{sourceID},
		Total:              roundedEnergyNumber(builder.total),
		Monthly:            roundedEnergyExplanationMonthly(builder.monthly),
		Daily:              roundedEnergyExplanationDaily(builder.daily),
		Hourly:             roundedEnergyExplanationHourly(builder.hourly),
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
		SourceUnit:         strings.TrimSpace(row.units),
		ReportingFrequency: strings.TrimSpace(dictionary.reportingFrequency),
		AggregationMethod:  energyExplanationAggregationMethod(dictionary),
		IndexGroup:         strings.TrimSpace(dictionary.indexGroup),
		TableName:          "ReportData",
		RowName:            reportDataSourceRowName(row),
		ColumnName:         reportDataSourceColumnName(row.units),
	}
}

func reportDataSourceRowName(row sqlOutputDictionaryRow) string {
	keyValue := strings.TrimSpace(row.keyValue)
	name := strings.TrimSpace(row.name)
	if keyValue != "" && name != "" {
		return keyValue + " / " + name
	}
	return firstNonEmpty(keyValue, name)
}

func reportDataSourceColumnName(units string) string {
	unit := strings.TrimSpace(units)
	if unit == "" {
		return "Value"
	}
	return "Value [" + unit + "]"
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
		item := canonicalEnergyExplanationSeries(energyExplanationSeries{
			Level:               "energy",
			Kind:                def.Kind,
			Label:               def.Label,
			Unit:                unit,
			Carrier:             def.Carrier,
			EndUse:              def.EndUse,
			MeterHierarchyLevel: def.HierarchyLevel,
			Basis:               "sql_tabular",
			SourceIDs:           []string{sourceID},
			Total:               roundedEnergyNumber(number),
			sourceKeyValue:      alias,
			sourceName:          alias,
			sourceFrequency:     "Annual",
			sourcePriority:      energyAliasPriority(alias, def.Aliases),
		})
		// The tabular fallback is deliberately narrower than meter parsing:
		// annual tables can supply only carrier and end-use totals. Production,
		// storage, driver, and load series need a time-series source so they are
		// never synthesized at the canonical parser boundary.
		if item.Stage != "carrier" && item.Stage != "end_use" {
			continue
		}
		sources = append(sources, EnergyDataSource{
			ID:                 sourceID,
			SourceType:         "sql_tabular",
			IsMeter:            true,
			KeyValue:           alias,
			Name:               alias,
			Units:              strings.TrimSpace(units),
			SourceUnit:         strings.TrimSpace(units),
			NormalizedUnit:     unit,
			ReportingFrequency: "Annual",
			AggregationMethod:  "tabular_annual_value",
			TableName:          strings.TrimSpace(tableName),
			RowName:            strings.TrimSpace(rowName),
			ColumnName:         tabularColumnLabel(columnName, units),
		})
		series = append(series, item)
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
	case "gasoline":
		return "Gasoline", true
	case "diesel":
		return "Diesel", true
	case "coal":
		return "Coal", true
	case "district cooling":
		return "DistrictCooling", true
	case "district heating", "district heating water":
		return "DistrictHeating", true
	case "steam", "district heating steam":
		return "Steam", true
	case "water":
		return "Water", true
	case "fuel oil no 1", "fuel oil 1", "fuel oil #1":
		return "FuelOilNo1", true
	case "fuel oil no 2", "fuel oil 2", "fuel oil #2":
		return "FuelOilNo2", true
	case "propane":
		return "Propane", true
	case "other fuel 1":
		return "OtherFuel1", true
	case "other fuel 2":
		return "OtherFuel2", true
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

func energyExplanationLoadSourceBasis(dictionary energyExplanationDictionary) string {
	if energyExplanationIntegratesRate(dictionary) {
		return "integrated_rate_variable"
	}
	return "measured_energy_variable"
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

func energyExplanationSupportsMonthlyPeriods(dictionary energyExplanationDictionary) bool {
	return energyExplanationReportingFrequencySupportsMonthly(dictionary.reportingFrequency)
}

func energyExplanationReportingFrequencySupportsMonthly(frequency string) bool {
	switch strings.ToLower(canonicalPurposeFrequency(frequency)) {
	case "runperiod", "annual":
		return false
	default:
		return true
	}
}

func energyExplanationSupportsHourlyPeriods(dictionary energyExplanationDictionary) bool {
	frequency := strings.TrimSpace(dictionary.reportingFrequency)
	if frequency == "" {
		return false
	}
	switch strings.ToLower(canonicalPurposeFrequency(frequency)) {
	case "hourly", "timestep", "detailed":
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
			if normalizeEnergyOutputName(object.KeyValue) == normalizeEnergyOutputName(sourceName) || energyNamesShareEnergyAliasGroup(object.KeyValue, sourceName) {
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
		if !energyExplanationVariableObjectMatchesDictionaryName(object.VariableName, name, dictionary) {
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

func energyExplanationVariableObjectMatchesDictionaryName(objectName string, sourceName string, dictionary energyExplanationDictionary) bool {
	if normalizeEnergyOutputName(objectName) == normalizeEnergyOutputName(sourceName) {
		return true
	}
	switch {
	case dictionary.energy != nil:
		return energyNamesShareEnergyAliasGroup(objectName, sourceName)
	case dictionary.load != nil:
		return energyNamesShareLoadAliasGroup(objectName, sourceName)
	case dictionary.heat != nil:
		return energyNamesShareHeatAliasGroup(objectName, sourceName)
	default:
		return false
	}
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
	index         int64
	minutes       int
	month         int
	intervalType  string
	hours         float64
	valid         bool
	explicitHours bool
}

type sqlTimeIntervalDetails struct {
	hours    map[int64]float64
	explicit map[int64]bool
}

func sqlTimeIntervalHours(db *sql.DB) (map[int64]float64, error) {
	details, err := sqlTimeIntervalDetailsForDatabase(db)
	return details.hours, err
}

func sqlTimeIntervalDetailsForDatabase(db *sql.DB) (sqlTimeIntervalDetails, error) {
	columns, err := sqlTableColumns(db, "Time")
	if err != nil {
		return sqlTimeIntervalDetails{}, err
	}
	if !sqlHasColumns(columns, "TimeIndex") {
		return sqlTimeIntervalDetails{hours: map[int64]float64{}, explicit: map[int64]bool{}}, nil
	}
	query := fmt.Sprintf(`
SELECT %s AS time_index,
       %s AS month,
       %s AS day,
       %s AS hour,
       %s AS minute,
       %s AS interval_minutes,
       %s AS interval_type
FROM %s
ORDER BY %s`,
		quoteSQLiteIdentifier(columns[normalizeSQLColumnName("TimeIndex")]),
		sqlTextColumnExpr(columns, "Month", "''"),
		sqlTextColumnExpr(columns, "Day", "''"),
		sqlTextColumnExpr(columns, "Hour", "''"),
		sqlTextColumnExpr(columns, "Minute", "''"),
		sqlCastTextColumnExpr(columns, "Interval", "''"),
		sqlCastTextColumnExpr(columns, "IntervalType", "''"),
		quoteSQLiteIdentifier("Time"),
		quoteSQLiteIdentifier(columns[normalizeSQLColumnName("TimeIndex")]),
	)
	rows, err := db.Query(query)
	if err != nil {
		return sqlTimeIntervalDetails{}, err
	}
	defer rows.Close()
	var values []sqlTimeIntervalRow
	for rows.Next() {
		var index int64
		var monthText, dayText, hourText, minuteText, intervalText, intervalType string
		if err := rows.Scan(&index, &monthText, &dayText, &hourText, &minuteText, &intervalText, &intervalType); err != nil {
			continue
		}
		minutes, ok := sqlTimeOrdinalMinutes(monthText, dayText, hourText, minuteText)
		month, _ := parseSQLTimeInt(monthText)
		intervalMinutes, intervalErr := strconv.ParseFloat(strings.TrimSpace(intervalText), 64)
		explicitHours := intervalErr == nil && intervalMinutes > 0 && !math.IsNaN(intervalMinutes) && !math.IsInf(intervalMinutes, 0)
		values = append(values, sqlTimeIntervalRow{
			index:         index,
			minutes:       minutes,
			month:         month,
			intervalType:  strings.TrimSpace(intervalType),
			hours:         intervalMinutes / 60,
			valid:         ok,
			explicitHours: explicitHours,
		})
	}
	if err := rows.Err(); err != nil {
		return sqlTimeIntervalDetails{}, err
	}
	out := sqlTimeIntervalDetails{hours: map[int64]float64{}, explicit: map[int64]bool{}}
	for index, row := range values {
		hours := 1.0
		if row.explicitHours {
			hours = row.hours
			out.explicit[row.index] = true
		} else if intervalTypeHours, ok := sqlTimeIntervalTypeHours(row.intervalType, row.month); ok {
			hours = intervalTypeHours
		} else if row.valid {
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
		out.hours[row.index] = hours
	}
	return out, nil
}

func sqlTimeIntervalTypeHours(intervalType string, month int) (float64, bool) {
	switch normalizeEnergyOutputName(intervalType) {
	case "month", "monthly":
		return energyExplanationMonthHours(month), true
	case "day", "daily":
		return 24, true
	case "hour", "hourly":
		return 1, true
	default:
		return 0, false
	}
}

func energyExplanationRateIntervalHours(dictionary energyExplanationDictionary, row SQLSeriesRow, derivedHours float64, explicit bool) float64 {
	if explicit && derivedHours > 0 && !math.IsNaN(derivedHours) && !math.IsInf(derivedHours, 0) {
		return derivedHours
	}
	frequency := normalizeEnergyOutputName(dictionary.reportingFrequency)
	if frequency == "" {
		frequency = normalizeEnergyOutputName(row.IntervalType)
	}
	switch frequency {
	case "month", "monthly":
		month := 0
		if row.Month.Valid {
			month = int(row.Month.Int64)
		}
		return energyExplanationMonthHours(month)
	case "day", "daily":
		return 24
	case "hour", "hourly":
		return 1
	case "annual", "runperiod", "run period":
		return 8760
	default:
		return derivedHours
	}
}

func energyExplanationMonthHours(month int) float64 {
	monthDays := [...]int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if month >= 1 && month < len(monthDays) {
		return float64(monthDays[month] * 24)
	}
	return 730
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

func energyExplanationSQLHourOfYear(month sql.NullInt64, day sql.NullInt64, hour sql.NullInt64) (int, bool) {
	rowDay, ok := energyExplanationSQLDayOfYear(month, day)
	if !ok || !hour.Valid {
		return 0, false
	}
	rowHour := int(hour.Int64)
	if rowHour < 1 || rowHour > 24 {
		return 0, false
	}
	return ((rowDay - 1) * 24) + rowHour, true
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
			ID:             energyRelationshipRuleMeasuredEnergyVariable,
			FromLevel:      "energy",
			ToLevel:        "energy",
			FromKind:       "facility_total",
			ToKind:         "energy_variable_end_use",
			RequiredSource: []string{"sql_report_data:variable"},
			Basis:          "measured_energy_variable",
			Formula:        "sum(ReportData where Name matches measured energy variable)",
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
			ID:             energyRelationshipRuleAllocatedServicePathLoad,
			FromLevel:      "energy",
			ToLevel:        "load",
			FromKind:       "cooling_or_heating_end_use",
			ToKind:         "service_path_delivered_load",
			RequiredSource: []string{"sql_report_data:meter", "sql_report_data:variable", "idf_hvac_service_model"},
			Basis:          "allocated",
			Formula:        "allocate end-use energy by measured delivered-load share for matched HVAC service paths",
		},
		{
			ID:             energyRelationshipRuleAllocatedAuxiliaryServicePath,
			FromLevel:      "energy",
			ToLevel:        "load",
			FromKind:       "hvac_auxiliary_end_use",
			ToKind:         "related_service_path_evidence",
			RequiredSource: []string{"sql_report_data:meter", "sql_report_data:variable", "idf_hvac_service_model"},
			Basis:          "allocated",
			Formula:        "allocate HVAC auxiliary end-use energy only across related AirLoop, PlantLoop, or CondenserLoop service paths",
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
			ID:             energyRelationshipRuleInternalGainHeat,
			FromLevel:      "energy",
			ToLevel:        "heat",
			FromKind:       "internal_gain_end_use",
			ToKind:         "internal_gain_heat_driver",
			RequiredSource: []string{"sql_report_data:meter", "sql_report_data:heat_gain_variable"},
			Basis:          "measured_meter_plus_zone_gain_variable",
			Formula:        "link matching internal-gain end-use energy with reported zone heat-gain variable; values are independently measured",
		},
		{
			ID:             energyRelationshipRulePurchasedElectricity,
			FromLevel:      "energy",
			ToLevel:        "energy",
			FromKind:       "facility_total",
			ToKind:         "purchased_electricity",
			RequiredSource: []string{"ElectricityPurchased:Facility"},
			Basis:          "measured_meter",
			Formula:        "purchased electricity shown in supply context and excluded from end-use consumption reconciliation",
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
			ID:             energyRelationshipRuleStorageDischarge,
			FromLevel:      "energy",
			ToLevel:        "energy",
			FromKind:       "facility_total",
			ToKind:         "storage_discharge",
			RequiredSource: []string{"Electric Storage Discharge Energy"},
			Basis:          "measured_energy_variable",
			Formula:        "storage discharge shown separately from facility consumption residual",
		},
		{
			ID:             energyRelationshipRuleSoldElectricity,
			FromLevel:      "energy",
			ToLevel:        "energy",
			FromKind:       "facility_total",
			ToKind:         "sold_electricity",
			RequiredSource: []string{"ElectricitySurplusSold:Facility"},
			Basis:          "measured_meter",
			Formula:        "surplus electricity sold shown in supply context and excluded from end-use consumption reconciliation",
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
		{Kind: "energy.gas.total", Label: "Natural gas total", Carrier: "natural_gas", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"NaturalGas:Facility", "Gas:Facility"}, OutputRequestAliases: []string{"NaturalGas:Facility", "Gas:Facility"}},
		{Kind: "energy.gasoline.total", Label: "Gasoline total", Carrier: "gasoline", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"Gasoline:Facility"}},
		{Kind: "energy.diesel.total", Label: "Diesel total", Carrier: "diesel", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"Diesel:Facility"}},
		{Kind: "energy.coal.total", Label: "Coal total", Carrier: "coal", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"Coal:Facility"}},
		{Kind: "energy.district_cooling.total", Label: "District cooling total", Carrier: "district_cooling", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"DistrictCooling:Facility"}},
		{Kind: "energy.district_heating.total", Label: "District heating total", Carrier: "district_heating", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"DistrictHeatingWater:Facility", "DistrictHeating:Facility"}, LegacyAliases: []string{"DistrictHeating:Facility"}, OutputRequestAliases: []string{"DistrictHeatingWater:Facility", "DistrictHeating:Facility"}},
		{Kind: "energy.fuel_oil_1.total", Label: "Fuel oil #1 total", Carrier: "fuel_oil_1", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"FuelOilNo1:Facility"}},
		{Kind: "energy.fuel_oil_2.total", Label: "Fuel oil #2 total", Carrier: "fuel_oil_2", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"FuelOilNo2:Facility"}},
		{Kind: "energy.propane.total", Label: "Propane total", Carrier: "propane", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"Propane:Facility"}},
		{Kind: "energy.other_fuel_1.total", Label: "Other fuel 1 total", Carrier: "other_fuel_1", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"OtherFuel1:Facility"}},
		{Kind: "energy.other_fuel_2.total", Label: "Other fuel 2 total", Carrier: "other_fuel_2", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"OtherFuel2:Facility"}},
		{Kind: "energy.steam.total", Label: "Steam total", Carrier: "steam", EndUse: "total", HierarchyLevel: "facility_total", FacilityTotal: true, Aliases: []string{"DistrictHeatingSteam:Facility", "Steam:Facility"}, LegacyAliases: []string{"Steam:Facility"}, OutputRequestAliases: []string{"DistrictHeatingSteam:Facility", "Steam:Facility"}},
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
		{Kind: "energy.electricity_purchased", Label: "Purchased electricity", Carrier: "electricity", EndUse: "electricity_purchased", HierarchyLevel: "supply_context", Aliases: []string{"ElectricityPurchased:Facility"}},
		{Kind: "energy.generators", Label: "Generators / onsite production", Carrier: "electricity", EndUse: "generators", HierarchyLevel: "broad_end_use", Aliases: []string{"Generators:ElectricityProduced", "ElectricityProduced:Facility"}},
		{Kind: "energy.electricity_sold", Label: "Electricity sold", Carrier: "electricity", EndUse: "electricity_sold", HierarchyLevel: "supply_context", Aliases: []string{"ElectricitySurplusSold:Facility"}},
		{Kind: "energy.cooling", Label: "District cooling", Carrier: "district_cooling", EndUse: "cooling", HierarchyLevel: "broad_end_use", Aliases: []string{"Cooling:DistrictCooling", "DistrictCooling:Cooling"}},
		{Kind: "energy.heating", Label: "Natural gas heating", Carrier: "natural_gas", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:NaturalGas", "Heating:Gas", "NaturalGas:Heating", "Gas:Heating"}, LegacyAliases: []string{"NaturalGas:Heating", "Gas:Heating"}, OutputRequestAliases: []string{"Heating:NaturalGas", "Heating:Gas"}},
		{Kind: "energy.heating", Label: "Gasoline heating", Carrier: "gasoline", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:Gasoline", "Gasoline:Heating"}, LegacyAliases: []string{"Gasoline:Heating"}, OutputRequestAliases: []string{"Heating:Gasoline"}},
		{Kind: "energy.heating", Label: "Diesel heating", Carrier: "diesel", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:Diesel", "Diesel:Heating"}, LegacyAliases: []string{"Diesel:Heating"}, OutputRequestAliases: []string{"Heating:Diesel"}},
		{Kind: "energy.heating", Label: "Coal heating", Carrier: "coal", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:Coal", "Coal:Heating"}, LegacyAliases: []string{"Coal:Heating"}, OutputRequestAliases: []string{"Heating:Coal"}},
		{Kind: "energy.heating", Label: "Fuel oil #1 heating", Carrier: "fuel_oil_1", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:FuelOilNo1", "FuelOilNo1:Heating"}, LegacyAliases: []string{"FuelOilNo1:Heating"}, OutputRequestAliases: []string{"Heating:FuelOilNo1"}},
		{Kind: "energy.heating", Label: "Fuel oil #2 heating", Carrier: "fuel_oil_2", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:FuelOilNo2", "FuelOilNo2:Heating"}, LegacyAliases: []string{"FuelOilNo2:Heating"}, OutputRequestAliases: []string{"Heating:FuelOilNo2"}},
		{Kind: "energy.heating", Label: "Propane heating", Carrier: "propane", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:Propane", "Propane:Heating"}, LegacyAliases: []string{"Propane:Heating"}, OutputRequestAliases: []string{"Heating:Propane"}},
		{Kind: "energy.heating", Label: "Other fuel 1 heating", Carrier: "other_fuel_1", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:OtherFuel1", "OtherFuel1:Heating"}, LegacyAliases: []string{"OtherFuel1:Heating"}, OutputRequestAliases: []string{"Heating:OtherFuel1"}},
		{Kind: "energy.heating", Label: "Other fuel 2 heating", Carrier: "other_fuel_2", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:OtherFuel2", "OtherFuel2:Heating"}, LegacyAliases: []string{"OtherFuel2:Heating"}, OutputRequestAliases: []string{"Heating:OtherFuel2"}},
		{Kind: "energy.water_systems", Label: "Natural gas water systems", Carrier: "natural_gas", EndUse: "water_systems", HierarchyLevel: "broad_end_use", Aliases: []string{"WaterSystems:NaturalGas", "WaterSystems:Gas", "NaturalGas:WaterSystems", "Gas:WaterSystems"}, LegacyAliases: []string{"NaturalGas:WaterSystems", "Gas:WaterSystems"}, OutputRequestAliases: []string{"WaterSystems:NaturalGas", "WaterSystems:Gas"}},
		{Kind: "energy.interior_equipment", Label: "Natural gas interior equipment", Carrier: "natural_gas", EndUse: "interior_equipment", HierarchyLevel: "broad_end_use", Aliases: []string{"InteriorEquipment:NaturalGas", "InteriorEquipment:Gas", "NaturalGas:InteriorEquipment", "Gas:InteriorEquipment"}, LegacyAliases: []string{"NaturalGas:InteriorEquipment", "Gas:InteriorEquipment"}, OutputRequestAliases: []string{"InteriorEquipment:NaturalGas", "InteriorEquipment:Gas"}},
		{Kind: "energy.heating", Label: "District heating", Carrier: "district_heating", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:DistrictHeatingWater", "Heating:DistrictHeating", "DistrictHeating:Heating"}, LegacyAliases: []string{"Heating:DistrictHeating", "DistrictHeating:Heating"}, OutputRequestAliases: []string{"Heating:DistrictHeatingWater", "Heating:DistrictHeating", "DistrictHeating:Heating"}},
		{Kind: "energy.heating", Label: "District steam heating", Carrier: "steam", EndUse: "heating", HierarchyLevel: "broad_end_use", Aliases: []string{"Heating:DistrictHeatingSteam", "Heating:Steam", "Steam:Heating"}, LegacyAliases: []string{"Heating:Steam", "Steam:Heating"}, OutputRequestAliases: []string{"Heating:DistrictHeatingSteam", "Heating:Steam", "Steam:Heating"}},
	}
}

func energyLoadAliasCatalog() []energyLoadAliasDefinition {
	return []energyLoadAliasDefinition{
		{Kind: "load.zone_cooling", Label: "Zone cooling load", ServiceKind: "cooling", Scope: "zone", Aliases: []string{"Zone Air System Sensible Cooling Energy", "Zone Air System Sensible Cooling Rate", "Zone Ideal Loads Zone Total Cooling Energy", "Zone Ideal Loads Zone Total Cooling Rate", "Zone Ideal Loads Zone Sensible Cooling Energy", "Zone Ideal Loads Zone Sensible Cooling Rate", "Zone Ideal Loads Supply Air Total Cooling Energy", "Zone Ideal Loads Supply Air Total Cooling Rate"}, LegacyAliases: []string{"Zone Air System Sensible Cooling Energy", "Zone Air System Sensible Cooling Rate", "Zone Ideal Loads Zone Sensible Cooling Energy", "Zone Ideal Loads Supply Air Total Cooling Energy"}},
		{Kind: "load.zone_heating", Label: "Zone heating load", ServiceKind: "heating", Scope: "zone", Aliases: []string{"Zone Air System Sensible Heating Energy", "Zone Air System Sensible Heating Rate", "Zone Ideal Loads Zone Total Heating Energy", "Zone Ideal Loads Zone Total Heating Rate", "Zone Ideal Loads Zone Sensible Heating Energy", "Zone Ideal Loads Zone Sensible Heating Rate", "Zone Ideal Loads Supply Air Total Heating Energy", "Zone Ideal Loads Supply Air Total Heating Rate"}, LegacyAliases: []string{"Zone Air System Sensible Heating Energy", "Zone Air System Sensible Heating Rate", "Zone Ideal Loads Zone Sensible Heating Energy", "Zone Ideal Loads Supply Air Total Heating Energy"}},
		{Kind: "load.zone_latent_cooling", Label: "Zone latent cooling load", ServiceKind: "cooling", Scope: "zone", EnergyPathOnly: true, Aliases: []string{"Zone Air System Latent Cooling Energy", "Zone Air System Latent Cooling Rate", "Zone Ideal Loads Zone Latent Cooling Energy", "Zone Ideal Loads Zone Latent Cooling Rate"}},
		{Kind: "load.zone_latent_heating", Label: "Zone latent heating load", ServiceKind: "heating", Scope: "zone", EnergyPathOnly: true, Aliases: []string{"Zone Air System Latent Heating Energy", "Zone Air System Latent Heating Rate", "Zone Ideal Loads Zone Latent Heating Energy", "Zone Ideal Loads Zone Latent Heating Rate"}},
		{Kind: "load.zone_radiant_cooling", Label: "Radiant cooling load", ServiceKind: "cooling", Scope: "zone", Aliases: []string{"Zone Radiant HVAC Cooling Energy", "Zone Radiant HVAC Cooling Rate"}},
		{Kind: "load.zone_radiant_heating", Label: "Radiant heating load", ServiceKind: "heating", Scope: "zone", Aliases: []string{"Zone Radiant HVAC Heating Energy", "Zone Radiant HVAC Heating Rate"}},
		{Kind: "load.zone_equipment_heating", Label: "Zone equipment heating load", ServiceKind: "heating", Scope: "zone", EnergyPathOnly: true, Aliases: []string{"Zone Baseboard Total Heating Energy", "Zone Baseboard Total Heating Rate"}},
		{Kind: "load.system_cooling", Label: "System cooling delivered", ServiceKind: "cooling", Scope: "system", Aliases: []string{"Cooling Coil Total Cooling Energy", "Cooling Coil Sensible Cooling Energy", "Cooling Coil Total Cooling Rate"}},
		{Kind: "load.system_heating", Label: "System heating delivered", ServiceKind: "heating", Scope: "system", Aliases: []string{"Heating Coil Heating Energy", "Heating Coil Heating Rate"}},
		{Kind: "load.plant_cooling", Label: "Plant cooling demand", ServiceKind: "cooling", Scope: "plant", Aliases: []string{"Plant Supply Side Cooling Demand Rate", "Plant Loop Cooling Demand Energy"}},
		{Kind: "load.plant_heating", Label: "Plant heating demand", ServiceKind: "heating", Scope: "plant", Aliases: []string{"Plant Supply Side Heating Demand Rate", "Plant Loop Heating Demand Energy"}},
		{Kind: "load.plant_unmet_or_residual", Label: "Plant unmet/residual demand", ServiceKind: "unmet_or_residual", Scope: "plant", Aliases: []string{"Plant Supply Side Unmet Demand Rate", "Plant Supply Side Not Distributed Demand Rate", "Cond Loop Demand Not Distributed"}},
		{Kind: "load.zone_predicted_cooling", Label: "Predicted cooling control load", ServiceKind: "cooling", Scope: "zone", EnergyPathOnly: true, Aliases: []string{"Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate", "Zone System Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate"}},
		{Kind: "load.zone_predicted_heating", Label: "Predicted heating control load", ServiceKind: "heating", Scope: "zone", EnergyPathOnly: true, Aliases: []string{"Zone Predicted Sensible Load to Heating Setpoint Heat Transfer Rate", "Zone System Predicted Sensible Load to Heating Setpoint Heat Transfer Rate"}},
		{Kind: "load.zone_humidification", Label: "Zone humidification load", ServiceKind: "humidification", Scope: "zone", Aliases: []string{"Zone Ideal Loads Supply Air Latent Heating Energy", "Zone Ideal Loads Supply Air Latent Heating Rate", "Zone Ideal Loads Zone Latent Heating Energy", "Zone Ideal Loads Zone Latent Heating Rate"}},
		{Kind: "load.zone_dehumidification", Label: "Zone dehumidification load", ServiceKind: "dehumidification", Scope: "zone", Aliases: []string{"Zone Ideal Loads Supply Air Latent Cooling Energy", "Zone Ideal Loads Supply Air Latent Cooling Rate", "Zone Ideal Loads Zone Latent Cooling Energy", "Zone Ideal Loads Zone Latent Cooling Rate"}},
	}
}

func energyHeatAliasCatalog() []energyHeatAliasDefinition {
	catalog := []energyHeatAliasDefinition{
		{Kind: "heat.surface_inside_face_convection", Label: "Surface-to-zone-air heat exchange", HeatCategory: "surface_envelope", SurfaceScoped: true, Aliases: []string{"Surface Inside Face Convection Heat Transfer Energy", "Surface Inside Face Convection Heat Gain Energy", "Surface Inside Face Convection Heat Transfer Rate", "Surface Inside Face Convection Heat Gain Rate"}, OutputRequestAliases: []string{}},
		{Kind: "heat.surface_conduction", Label: "Surface conduction reference", HeatCategory: "surface_envelope", SurfaceScoped: true, Aliases: []string{"Surface Inside Face Conduction Heat Transfer Energy", "Surface Inside Face Conduction Heat Transfer Rate", "Surface Outside Face Conduction Heat Transfer Energy", "Surface Outside Face Conduction Heat Transfer Rate"}, OutputRequestAliases: []string{}},
		{Kind: "heat.surface_absorbed_solar", Label: "Surface absorbed solar reference", HeatCategory: "surface_envelope", SurfaceScoped: true, Aliases: []string{"Surface Inside Face Solar Radiation Heat Gain Energy", "Surface Inside Face Solar Radiation Heat Gain Rate", "Surface Outside Face Incident Solar Radiation Heat Gain Energy", "Surface Outside Face Incident Solar Radiation Heat Gain Rate"}, OutputRequestAliases: []string{}},
		{Kind: "heat.surface_radiant", Label: "Surface radiant reference", HeatCategory: "surface_envelope", SurfaceScoped: true, Aliases: []string{"Surface Inside Face Lights Radiation Heat Gain Energy", "Surface Inside Face Lights Radiation Heat Gain Rate", "Surface Inside Face Internal Gains Radiation Heat Gain Energy", "Surface Inside Face Internal Gains Radiation Heat Gain Rate"}, OutputRequestAliases: []string{}},
		{Kind: "heat.internal_convective", Label: "Internal convective reconciliation", HeatCategory: "internal_gains", Aliases: []string{"Zone Air Heat Balance Internal Convective Heat Gain Rate", "Zone Total Internal Convective Heating Energy", "Zone Total Internal Convective Heating Rate", "Zone Total Internal Latent Gain Energy", "Zone Total Internal Latent Gain Rate"}},
		{Kind: "heat.surface_convection", Label: "Surface convection", HeatCategory: "surface_envelope", Aliases: []string{"Zone Air Heat Balance Surface Convection Rate"}},
		{Kind: "heat.interzone_air", Label: "Interzone air transfer", HeatCategory: "air_exchange", Aliases: []string{"Zone Air Heat Balance Interzone Air Transfer Rate"}},
		{Kind: "heat.ventilation_outdoor_air", Label: "Outdoor air transfer", HeatCategory: "air_exchange", Aliases: []string{"Zone Air Heat Balance Outdoor Air Transfer Rate"}},
		{Kind: "heat.hvac_air_transfer", Label: "HVAC system air transfer", HeatCategory: "hvac_system", Aliases: []string{"Zone Air Heat Balance System Air Transfer Rate"}},
		{Kind: "heat.system_convective", Label: "HVAC/system convective gains", HeatCategory: "hvac_system", Aliases: []string{"Zone Air Heat Balance System Convective Heat Gain Rate"}},
		{Kind: "heat.fan_to_air", Label: "Fan heat to air", HeatCategory: "hvac_system", ObjectScoped: true, Aliases: []string{"Fan Air Heat Gain Energy", "Fan Air Heat Gain Rate"}},
		{Kind: "heat.solar_window", Label: "Window transmitted solar", HeatCategory: "surface_envelope", Aliases: []string{"Zone Windows Total Transmitted Solar Radiation Energy", "Zone Windows Total Transmitted Solar Radiation Rate", "Zone Transmitted Solar Energy"}},
		{Kind: "heat.window_heat_transfer", Label: "Window heat transfer", HeatCategory: "surface_envelope", Aliases: []string{"Zone Windows Total Heat Gain Energy", "Zone Windows Total Heat Gain Rate", "Zone Windows Total Heat Loss Energy", "Zone Windows Total Heat Loss Rate"}},
		{Kind: "heat.storage_air", Label: "Air energy storage", HeatCategory: "storage_residual", Aliases: []string{"Zone Air Heat Balance Air Energy Storage Rate"}},
		{Kind: "heat.zone_balance_residual", Label: "Heat balance deviation", HeatCategory: "storage_residual", Aliases: []string{"Zone Air Heat Balance Deviation Rate"}},
		{Kind: "heat.people", Label: "People heat", HeatCategory: "internal_gains", Aliases: []string{"Zone People Convective Heating Energy", "Zone People Convective Heating Rate", "Zone People Latent Gain Energy", "Zone People Latent Gain Rate", "Zone People Sensible Heating Energy", "Zone People Sensible Heating Rate", "Zone People Total Heating Energy", "Zone People Total Heating Rate", "Zone People Radiant Heating Energy", "Zone People Radiant Heating Rate"}},
		{Kind: "heat.lighting", Label: "Lighting heat", HeatCategory: "internal_gains", Aliases: []string{"Zone Lights Convective Heating Energy", "Zone Lights Convective Heating Rate", "Zone Lights Total Heating Energy", "Zone Lights Total Heating Rate", "Zone Lights Radiant Heating Energy", "Zone Lights Radiant Heating Rate", "Zone Lights Visible Radiation Heating Energy", "Zone Lights Visible Radiation Heating Rate", "Zone Lights Return Air Heating Energy", "Zone Lights Return Air Heating Rate"}},
		{Kind: "heat.equipment", Label: "Equipment heat", HeatCategory: "internal_gains", Aliases: []string{"Zone Electric Equipment Convective Heating Energy", "Zone Electric Equipment Convective Heating Rate", "Zone Electric Equipment Latent Gain Energy", "Zone Electric Equipment Latent Gain Rate", "Zone Gas Equipment Convective Heating Energy", "Zone Gas Equipment Convective Heating Rate", "Zone Gas Equipment Latent Gain Energy", "Zone Gas Equipment Latent Gain Rate", "Zone Other Equipment Convective Heating Energy", "Zone Other Equipment Convective Heating Rate", "Zone Other Equipment Latent Gain Energy", "Zone Other Equipment Latent Gain Rate", "Zone Hot Water Equipment Convective Heating Energy", "Zone Hot Water Equipment Convective Heating Rate", "Zone Hot Water Equipment Latent Gain Energy", "Zone Hot Water Equipment Latent Gain Rate", "Zone Steam Equipment Convective Heating Energy", "Zone Steam Equipment Convective Heating Rate", "Zone Steam Equipment Latent Gain Energy", "Zone Steam Equipment Latent Gain Rate", "Zone Electric Equipment Total Heating Energy", "Zone Electric Equipment Total Heating Rate", "Zone Gas Equipment Total Heating Energy", "Zone Gas Equipment Total Heating Rate", "Zone Other Equipment Total Heating Energy", "Zone Other Equipment Total Heating Rate", "Zone Electric Equipment Radiant Heating Energy", "Zone Electric Equipment Radiant Heating Rate", "Zone Gas Equipment Radiant Heating Energy", "Zone Gas Equipment Radiant Heating Rate", "Zone Other Equipment Radiant Heating Energy", "Zone Other Equipment Radiant Heating Rate", "Zone Electric Equipment Lost Heat Energy", "Zone Electric Equipment Lost Heat Rate", "Zone Gas Equipment Lost Heat Energy", "Zone Gas Equipment Lost Heat Rate"}},
		{Kind: "heat.internal_other", Label: "Other internal heat", HeatCategory: "internal_gains", Aliases: []string{"Zone Other Internal Convective Heating Energy", "Zone Other Internal Convective Heating Rate", "Zone Other Internal Latent Gain Energy", "Zone Other Internal Latent Gain Rate", "Zone IT Equipment Convective Heating Energy", "Zone IT Equipment Convective Heating Rate", "Zone IT Equipment Latent Gain Energy", "Zone IT Equipment Latent Gain Rate"}},
		{Kind: "heat.infiltration", Label: "Infiltration heat transfer", HeatCategory: "air_exchange", Aliases: []string{"Zone Infiltration Sensible Heat Loss Energy", "Zone Infiltration Sensible Heat Gain Energy", "Zone Infiltration Sensible Heat Loss Rate", "Zone Infiltration Sensible Heat Gain Rate", "AFN Zone Infiltration Sensible Heat Loss Energy", "AFN Zone Infiltration Sensible Heat Gain Energy", "AFN Zone Infiltration Sensible Heat Loss Rate", "AFN Zone Infiltration Sensible Heat Gain Rate", "Zone Infiltration Latent Heat Loss Energy", "Zone Infiltration Latent Heat Gain Energy", "Zone Infiltration Latent Heat Loss Rate", "Zone Infiltration Latent Heat Gain Rate", "AFN Zone Infiltration Latent Heat Loss Energy", "AFN Zone Infiltration Latent Heat Gain Energy", "AFN Zone Infiltration Latent Heat Loss Rate", "AFN Zone Infiltration Latent Heat Gain Rate"}},
		{Kind: "heat.ventilation", Label: "Ventilation heat transfer", HeatCategory: "air_exchange", Aliases: []string{"Zone Ventilation Sensible Heat Loss Energy", "Zone Ventilation Sensible Heat Gain Energy", "Zone Ventilation Sensible Heat Loss Rate", "Zone Ventilation Sensible Heat Gain Rate", "AFN Zone Ventilation Sensible Heat Loss Energy", "AFN Zone Ventilation Sensible Heat Gain Energy", "AFN Zone Ventilation Sensible Heat Loss Rate", "AFN Zone Ventilation Sensible Heat Gain Rate", "Zone Ventilation Latent Heat Loss Energy", "Zone Ventilation Latent Heat Gain Energy", "Zone Ventilation Latent Heat Loss Rate", "Zone Ventilation Latent Heat Gain Rate", "AFN Zone Ventilation Latent Heat Loss Energy", "AFN Zone Ventilation Latent Heat Gain Energy", "AFN Zone Ventilation Latent Heat Loss Rate", "AFN Zone Ventilation Latent Heat Gain Rate"}},
		{Kind: "heat.ventilation_ideal_oa", Label: "Ideal Loads outdoor-air conditioning context", HeatCategory: "air_exchange", ObjectScoped: true, Aliases: []string{"Zone Ideal Loads Outdoor Air Sensible Heating Energy", "Zone Ideal Loads Outdoor Air Sensible Heating Rate", "Zone Ideal Loads Outdoor Air Latent Heating Energy", "Zone Ideal Loads Outdoor Air Latent Heating Rate", "Zone Ideal Loads Outdoor Air Total Heating Energy", "Zone Ideal Loads Outdoor Air Total Heating Rate", "Zone Ideal Loads Outdoor Air Sensible Cooling Energy", "Zone Ideal Loads Outdoor Air Sensible Cooling Rate", "Zone Ideal Loads Outdoor Air Latent Cooling Energy", "Zone Ideal Loads Outdoor Air Latent Cooling Rate", "Zone Ideal Loads Outdoor Air Total Cooling Energy", "Zone Ideal Loads Outdoor Air Total Cooling Rate"}},
		{Kind: "heat.ventilation_ideal_heat_recovery", Label: "Ideal Loads heat-recovery context", HeatCategory: "air_exchange", ObjectScoped: true, Aliases: []string{"Zone Ideal Loads Heat Recovery Sensible Heating Energy", "Zone Ideal Loads Heat Recovery Sensible Heating Rate", "Zone Ideal Loads Heat Recovery Latent Heating Energy", "Zone Ideal Loads Heat Recovery Latent Heating Rate", "Zone Ideal Loads Heat Recovery Total Heating Energy", "Zone Ideal Loads Heat Recovery Total Heating Rate", "Zone Ideal Loads Heat Recovery Sensible Cooling Energy", "Zone Ideal Loads Heat Recovery Sensible Cooling Rate", "Zone Ideal Loads Heat Recovery Latent Cooling Energy", "Zone Ideal Loads Heat Recovery Latent Cooling Rate", "Zone Ideal Loads Heat Recovery Total Cooling Energy", "Zone Ideal Loads Heat Recovery Total Cooling Rate"}},
		{Kind: "heat.ventilation_system_oa", Label: "System outdoor-air conditioning context", HeatCategory: "air_exchange", ObjectScoped: true, Aliases: []string{"Air System Outdoor Air Sensible Heating Energy", "Air System Outdoor Air Sensible Heating Rate", "Air System Outdoor Air Latent Heating Energy", "Air System Outdoor Air Latent Heating Rate", "Air System Outdoor Air Total Heating Energy", "Air System Outdoor Air Total Heating Rate", "Air System Outdoor Air Sensible Cooling Energy", "Air System Outdoor Air Sensible Cooling Rate", "Air System Outdoor Air Latent Cooling Energy", "Air System Outdoor Air Latent Cooling Rate", "Air System Outdoor Air Total Cooling Energy", "Air System Outdoor Air Total Cooling Rate"}},
		{Kind: "heat.ventilation_heat_recovery", Label: "Ventilation heat-recovery context", HeatCategory: "air_exchange", ObjectScoped: true, Aliases: []string{"Heat Exchanger Sensible Heating Energy", "Heat Exchanger Sensible Heating Rate", "Heat Exchanger Latent Heating Energy", "Heat Exchanger Latent Heating Rate", "Heat Exchanger Total Heating Energy", "Heat Exchanger Total Heating Rate", "Heat Exchanger Sensible Cooling Energy", "Heat Exchanger Sensible Cooling Rate", "Heat Exchanger Latent Cooling Energy", "Heat Exchanger Latent Cooling Rate", "Heat Exchanger Total Cooling Energy", "Heat Exchanger Total Cooling Rate"}},
		{Kind: "heat.combined_outdoor_air", Label: "Combined outdoor-air transfer", HeatCategory: "air_exchange", Aliases: []string{"Zone Combined Outdoor Air Sensible Heat Loss Energy", "Zone Combined Outdoor Air Sensible Heat Gain Energy", "Zone Combined Outdoor Air Sensible Heat Loss Rate", "Zone Combined Outdoor Air Sensible Heat Gain Rate", "Zone Combined Outdoor Air Latent Heat Loss Energy", "Zone Combined Outdoor Air Latent Heat Gain Energy", "Zone Combined Outdoor Air Latent Heat Loss Rate", "Zone Combined Outdoor Air Latent Heat Gain Rate"}},
		{Kind: "heat.mixing", Label: "Mixing heat transfer", HeatCategory: "air_exchange", Aliases: []string{"Zone Mixing Sensible Heat Loss Energy", "Zone Mixing Sensible Heat Gain Energy", "Zone Mixing Sensible Heat Loss Rate", "Zone Mixing Sensible Heat Gain Rate", "Zone Mixing Latent Heat Loss Energy", "Zone Mixing Latent Heat Gain Energy", "Zone Mixing Latent Heat Loss Rate", "Zone Mixing Latent Heat Gain Rate", "Zone Cross Mixing Sensible Heat Loss Energy", "Zone Cross Mixing Sensible Heat Gain Energy", "Zone Cross Mixing Sensible Heat Loss Rate", "Zone Cross Mixing Sensible Heat Gain Rate", "Zone Cross Mixing Latent Heat Loss Energy", "Zone Cross Mixing Latent Heat Gain Energy", "Zone Cross Mixing Latent Heat Loss Rate", "Zone Cross Mixing Latent Heat Gain Rate", "Zone CrossMixing Sensible Heat Loss Energy", "Zone CrossMixing Sensible Heat Gain Energy", "Zone CrossMixing Sensible Heat Loss Rate", "Zone CrossMixing Sensible Heat Gain Rate", "Zone CrossMixing Latent Heat Loss Energy", "Zone CrossMixing Latent Heat Gain Energy", "Zone CrossMixing Latent Heat Loss Rate", "Zone CrossMixing Latent Heat Gain Rate", "Zone Refrigeration Door Mixing Sensible Heat Loss Energy", "Zone Refrigeration Door Mixing Sensible Heat Gain Energy", "Zone Refrigeration Door Mixing Sensible Heat Loss Rate", "Zone Refrigeration Door Mixing Sensible Heat Gain Rate", "Zone Refrigeration Door Mixing Latent Heat Loss Energy", "Zone Refrigeration Door Mixing Latent Heat Gain Energy", "Zone Refrigeration Door Mixing Latent Heat Loss Rate", "Zone Refrigeration Door Mixing Latent Heat Gain Rate", "AFN Zone Mixing Sensible Heat Loss Energy", "AFN Zone Mixing Sensible Heat Gain Energy", "AFN Zone Mixing Sensible Heat Loss Rate", "AFN Zone Mixing Sensible Heat Gain Rate", "AFN Zone Mixing Latent Heat Loss Energy", "AFN Zone Mixing Latent Heat Gain Energy", "AFN Zone Mixing Latent Heat Loss Rate", "AFN Zone Mixing Latent Heat Gain Rate"}},
	}
	for index := range catalog {
		if catalog[index].Kind != "heat.mixing" {
			continue
		}
		catalog[index].OutputRequestAliases = []string{
			"Zone Mixing Sensible Heat Loss Energy", "Zone Mixing Sensible Heat Gain Energy",
			"Zone Mixing Sensible Heat Loss Rate", "Zone Mixing Sensible Heat Gain Rate",
			"Zone Mixing Latent Heat Loss Energy", "Zone Mixing Latent Heat Gain Energy",
			"Zone Mixing Latent Heat Loss Rate", "Zone Mixing Latent Heat Gain Rate",
			"AFN Zone Mixing Sensible Heat Loss Energy", "AFN Zone Mixing Sensible Heat Gain Energy",
			"AFN Zone Mixing Sensible Heat Loss Rate", "AFN Zone Mixing Sensible Heat Gain Rate",
			"AFN Zone Mixing Latent Heat Loss Energy", "AFN Zone Mixing Latent Heat Gain Energy",
			"AFN Zone Mixing Latent Heat Loss Rate", "AFN Zone Mixing Latent Heat Gain Rate",
		}
	}
	return catalog
}

func energyVariableAliasCatalog() []energyMeterAliasDefinition {
	return []energyMeterAliasDefinition{
		{Kind: "energy.storage_charge", Label: "Storage charge", Carrier: "electricity", EndUse: "storage_charge", HierarchyLevel: "broad_end_use", Aliases: []string{"Electric Storage Charge Energy"}},
		{Kind: "energy.storage_discharge", Label: "Storage discharge", Carrier: "electricity", EndUse: "storage_discharge", HierarchyLevel: "broad_end_use", Aliases: []string{"Electric Storage Discharge Energy"}},
	}
}

func energyPathDirectUseVariableAliasCatalog() []energyMeterAliasDefinition {
	return []energyMeterAliasDefinition{
		{Kind: "energy.interior_lighting", Label: "Zone interior lighting", Carrier: "electricity", EndUse: "interior_lighting", HierarchyLevel: "zone_direct_use", Aliases: []string{"Zone Lights Electricity Energy", "Zone Lights Electric Energy"}, LegacyAliases: []string{"Zone Lights Electric Energy"}, OutputRequestAliases: []string{"Zone Lights Electricity Energy"}},
		{Kind: "energy.interior_equipment", Label: "Zone electric equipment", Carrier: "electricity", EndUse: "interior_equipment", HierarchyLevel: "zone_direct_use", Aliases: []string{"Zone Electric Equipment Electricity Energy", "Zone Electric Equipment Electric Energy"}, LegacyAliases: []string{"Zone Electric Equipment Electric Energy"}, OutputRequestAliases: []string{"Zone Electric Equipment Electricity Energy"}},
		{Kind: "energy.interior_equipment", Label: "Zone gas equipment", Carrier: "natural_gas", EndUse: "interior_equipment", HierarchyLevel: "zone_direct_use", Aliases: []string{"Zone Gas Equipment NaturalGas Energy", "Zone Gas Equipment Gas Energy"}, LegacyAliases: []string{"Zone Gas Equipment Gas Energy"}, OutputRequestAliases: []string{"Zone Gas Equipment NaturalGas Energy", "Zone Gas Equipment Gas Energy"}},
		// The OtherEquipment output deliberately does not encode its configured
		// fuel type. Keep the carrier unknown-safe instead of guessing a resource
		// from the object name; exact carrier evidence can still arrive through a
		// canonical/custom zone-keyed source.
		{Kind: "energy.interior_equipment", Label: "Zone other equipment", Carrier: "other", EndUse: "interior_equipment", HierarchyLevel: "zone_direct_use", Aliases: []string{"Zone Other Equipment Fuel Energy", "Other Equipment Fuel Energy"}, LegacyAliases: []string{"Other Equipment Fuel Energy"}, OutputRequestAliases: []string{"Zone Other Equipment Fuel Energy"}},
		{Kind: "energy.interior_equipment", Label: "Zone hot water equipment", Carrier: "district_heating", EndUse: "interior_equipment", HierarchyLevel: "zone_direct_use", Aliases: []string{"Zone Hot Water Equipment District Heating Energy"}, OutputRequestAliases: []string{"Zone Hot Water Equipment District Heating Energy"}},
		{Kind: "energy.interior_equipment", Label: "Zone steam equipment", Carrier: "district_heating", EndUse: "interior_equipment", HierarchyLevel: "zone_direct_use", Aliases: []string{"Zone Steam Equipment District Heating Energy"}, OutputRequestAliases: []string{"Zone Steam Equipment District Heating Energy"}},
	}
}

func energyVariableAliasDefinitionForName(name string) (energyMeterAliasDefinition, bool) {
	key := normalizeEnergyOutputName(name)
	definitions := append(energyVariableAliasCatalog(), energyPathDirectUseVariableAliasCatalog()...)
	for _, def := range definitions {
		for _, alias := range def.Aliases {
			if normalizeEnergyOutputName(alias) == key {
				return def, true
			}
		}
	}
	return energyMeterAliasDefinition{}, false
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

func energyMeterAliasOrOtherDefinitionForName(name string) (energyMeterAliasDefinition, bool) {
	if def, ok := energyMeterAliasDefinitionForName(name); ok {
		return def, true
	}
	if def, ok := energyMeterEndUseCarrierDefinitionForName(name); ok {
		return def, true
	}
	carrier, ok := energyMeterCarrierFromUnknownMeter(name)
	if !ok {
		return energyMeterAliasDefinition{}, false
	}
	return energyMeterAliasDefinition{
		Kind:           "energy.other",
		Label:          "Other " + strings.ToLower(energyCarrierLabel(carrier)) + " use",
		Carrier:        carrier,
		EndUse:         "other",
		HierarchyLevel: "broad_end_use",
		Aliases:        []string{name},
	}, true
}

func energyMeterEndUseCarrierDefinitionForName(name string) (energyMeterAliasDefinition, bool) {
	parts := strings.Split(strings.TrimSpace(name), ":")
	if len(parts) != 2 {
		return energyMeterAliasDefinition{}, false
	}
	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])
	if strings.EqualFold(left, "facility") || strings.EqualFold(right, "facility") {
		return energyMeterAliasDefinition{}, false
	}
	if carrier, ok := energyCarrierToken(left); ok {
		if endUse, ok := energyEndUseToken(right); ok {
			return energyMeterEndUseCarrierDefinition(name, carrier, endUse), true
		}
	}
	if endUse, ok := energyEndUseToken(left); ok {
		if carrier, ok := energyCarrierToken(right); ok {
			return energyMeterEndUseCarrierDefinition(name, carrier, endUse), true
		}
	}
	return energyMeterAliasDefinition{}, false
}

func energyMeterEndUseCarrierDefinition(name string, carrier string, endUse string) energyMeterAliasDefinition {
	return energyMeterAliasDefinition{
		Kind:           "energy." + endUse,
		Label:          energyEndUseLabel(endUse, carrier),
		Carrier:        carrier,
		EndUse:         endUse,
		HierarchyLevel: "broad_end_use",
		Aliases:        []string{name},
	}
}

func energyEndUseToken(value string) (string, bool) {
	switch normalizeEnergyOutputName(value) {
	case "cooling":
		return "cooling", true
	case "heating":
		return "heating", true
	case "interiorlights":
		return "interior_lighting", true
	case "interiorequipment":
		return "interior_equipment", true
	case "exteriorlights":
		return "exterior_lighting", true
	case "exteriorequipment":
		return "exterior_equipment", true
	case "fans":
		return "fans", true
	case "pumps":
		return "pumps", true
	case "heatrejection":
		return "heat_rejection", true
	case "heatrecovery":
		return "heat_recovery", true
	case "watersystems", "dhw":
		return "water_systems", true
	case "refrigeration":
		return "refrigeration", true
	case "humidifier", "humidification":
		return "humidification", true
	case "cogeneration":
		return "generators", true
	case "miscellaneous":
		return "other", true
	default:
		return "", false
	}
}

func energyEndUseLabel(endUse string, carrier string) string {
	switch endUse {
	case "cooling":
		return energyCarrierLabel(carrier) + " cooling"
	case "heating":
		return energyCarrierLabel(carrier) + " heating"
	case "interior_lighting":
		return energyCarrierLabel(carrier) + " interior lighting"
	case "interior_equipment":
		return energyCarrierLabel(carrier) + " interior equipment"
	case "exterior_lighting":
		return energyCarrierLabel(carrier) + " exterior lighting"
	case "exterior_equipment":
		return energyCarrierLabel(carrier) + " exterior equipment"
	case "fans":
		return energyCarrierLabel(carrier) + " fans"
	case "pumps":
		return energyCarrierLabel(carrier) + " pumps"
	case "heat_rejection":
		return energyCarrierLabel(carrier) + " heat rejection"
	case "heat_recovery":
		return energyCarrierLabel(carrier) + " heat recovery"
	case "water_systems":
		return energyCarrierLabel(carrier) + " water systems"
	case "refrigeration":
		return energyCarrierLabel(carrier) + " refrigeration"
	case "humidification":
		return energyCarrierLabel(carrier) + " humidification"
	case "generators":
		return energyCarrierLabel(carrier) + " cogeneration / onsite production"
	case "other":
		return "Other " + strings.ToLower(energyCarrierLabel(carrier)) + " use"
	default:
		return energyCarrierLabel(carrier) + " " + strings.ReplaceAll(endUse, "_", " ")
	}
}

func energyMeterCarrierFromUnknownMeter(name string) (string, bool) {
	parts := strings.Split(strings.TrimSpace(name), ":")
	if len(parts) != 2 {
		return "", false
	}
	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])
	if strings.EqualFold(left, "facility") || strings.EqualFold(right, "facility") {
		return "", false
	}
	if carrier, ok := energyCarrierToken(left); ok {
		return carrier, true
	}
	if carrier, ok := energyCarrierToken(right); ok {
		return carrier, true
	}
	return "", false
}

func energyCarrierToken(value string) (string, bool) {
	definition, ok := energyCarrierTaxonomyDefinitionFor(value)
	return definition.Token, ok
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

func energyExplanationVariableAliasCandidates(name string) []string {
	if definition, ok := energyVariableAliasDefinitionForName(name); ok {
		return preferredEnergyExplanationAliases(name, definition.Aliases, nil)
	}
	if definition, ok := energyLoadAliasDefinitionForName(name); ok {
		return preferredEnergyExplanationAliases(name, definition.Aliases, nil)
	}
	if definition, ok := energyHeatAliasDefinitionForName(name); ok {
		role := energyDriverSourcePolicyFor(name, definition.Kind).Role
		component := energyHeatThermalComponent(name)
		sign := energyHeatAliasExplicitSign(name)
		return preferredEnergyExplanationAliases(name, definition.Aliases, func(alias string) bool {
			return energyDriverSourcePolicyFor(alias, definition.Kind).Role == role &&
				energyHeatThermalComponent(alias) == component &&
				energyHeatAliasExplicitSign(alias) == sign
		})
	}
	return nil
}

func preferredEnergyExplanationAliases(name string, aliases []string, include func(string) bool) []string {
	wanted := normalizeEnergyOutputName(name)
	out := []string{}
	for _, rate := range []bool{false, true} {
		for _, alias := range aliases {
			if normalizeEnergyOutputName(alias) == wanted || energyExplanationOutputIsRate(alias) != rate || include != nil && !include(alias) {
				continue
			}
			out = appendUniquePurposeString(out, alias)
		}
	}
	return out
}

func energyExplanationOutputIsRate(name string) bool {
	return strings.HasSuffix(normalizeEnergyOutputName(name), " rate")
}

func energyHeatAliasExplicitSign(name string) string {
	key := normalizeEnergyOutputName(name)
	internalSource := strings.Contains(key, "zone people ") || strings.Contains(key, "zone lights ") ||
		strings.Contains(key, "zone electric equipment ") || strings.Contains(key, "zone gas equipment ") ||
		strings.Contains(key, "zone other equipment ") || strings.Contains(key, "zone hot water equipment ") ||
		strings.Contains(key, "zone steam equipment ") || strings.Contains(key, "zone it equipment ") ||
		strings.Contains(key, "zone other internal ") || strings.Contains(key, "zone total internal ") ||
		strings.Contains(key, "zone air heat balance internal convective")
	if internalSource && (strings.Contains(key, " convective heating ") || strings.Contains(key, " convective heat gain ") || strings.Contains(key, " latent gain ")) {
		return "positive"
	}
	if strings.Contains(key, " sensible heat loss ") || strings.Contains(key, " latent heat loss ") || strings.Contains(key, "zone windows total heat loss") {
		return "negative"
	}
	if strings.Contains(key, " sensible heat gain ") || strings.Contains(key, " latent heat gain ") || strings.Contains(key, "zone windows total heat gain") {
		return "positive"
	}
	if strings.Contains(key, "outdoor air") && strings.Contains(key, " heating ") {
		return "negative"
	}
	if strings.Contains(key, "outdoor air") && strings.Contains(key, " cooling ") {
		return "positive"
	}
	return ""
}

func energyHeatThermalComponent(name string) string {
	key := normalizeEnergyOutputName(name)
	if strings.Contains(key, "latent") {
		return "latent"
	}
	if strings.Contains(key, "sensible") {
		return "sensible"
	}
	if strings.Contains(key, "convective") || strings.Contains(key, "convection") {
		return "sensible"
	}
	if strings.Contains(key, "zone air heat balance") {
		return "sensible"
	}
	return "combined"
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
	expectedEnergy, expectedContext := partitionEnergyExplanationContextOutputs(expectedEnergyExplanationOutputs(plan, "energy"))
	expectedLoad := expectedEnergyExplanationOutputs(plan, "load")
	expectedHeat := expectedEnergyExplanationOutputs(plan, "heat")
	expectedEnergyGroups := expectedEnergyExplanationOutputGroups(expectedEnergy, "energy")
	expectedLoadGroups := expectedEnergyExplanationOutputGroups(expectedLoad, "load")
	expectedHeatGroups := expectedEnergyExplanationOutputGroups(expectedHeat, "heat")
	foundEnergyGroups := map[string]bool{}
	foundLoadGroups := map[string]bool{}
	foundHeatGroups := map[string]bool{}
	for _, item := range series {
		if item.Level == "energy" && canonicalEnergyPathCarrier(item.Carrier) == "water" && !energyExplanationUnitIsSiteEnergy(item.Unit) {
			continue
		}
		if energyExplanationIsSupportEndUse(item) {
			continue
		}
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
	if energyExplanationLevelNotRequested(plan, "load") {
		loadLevel = energyCompletenessNotRequestedLevel("load", "Delivered Load")
	}
	if energyExplanationLevelNotRequested(plan, "heat") {
		heatLevel = energyCompletenessNotRequestedLevel("heat", "Heat Drivers")
	}
	status := "complete"
	if energyLevel.Status != "complete" || loadLevel.Status == "missing" || heatLevel.Status == "missing" {
		status = "partial"
	}
	if foundEnergy == 0 && foundLoad == 0 && foundHeat == 0 {
		status = "missing"
	}
	availability := make([]EnergySourceAvailabilityEntry, 0, len(expectedEnergy)+len(expectedLoad)+len(expectedHeat))
	availability = append(availability, sourceAvailabilityEntriesForLevel(expectedEnergy, "energy", sources, false)...)
	availability = append(availability, sourceAvailabilityEntriesForLevel(expectedLoad, "load", sources, energyExplanationLevelNotRequested(plan, "load"))...)
	availability = append(availability, sourceAvailabilityEntriesForLevel(expectedHeat, "heat", sources, energyExplanationLevelNotRequested(plan, "heat"))...)
	availability = append(availability, sourceAvailabilityEntries(expectedContext, "context", sources)...)
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

func partitionEnergyExplanationContextOutputs(input []string) ([]string, []string) {
	energy := make([]string, 0, len(input))
	context := make([]string, 0)
	for _, name := range input {
		if energyExplanationNameIsWaterMeter(name) || energyExplanationNameIsSupplyContext(name) {
			context = appendUniquePurposeString(context, name)
			continue
		}
		energy = appendUniquePurposeString(energy, name)
	}
	return energy, context
}

func energyExplanationNameIsSupplyContext(name string) bool {
	definition, ok := energyMeterAliasDefinitionForName(name)
	if !ok {
		definition, ok = energyVariableAliasDefinitionForName(name)
	}
	if !ok {
		return false
	}
	return energyExplanationIsSupportEndUse(energyExplanationSeries{
		Level:   "energy",
		Kind:    definition.Kind,
		Carrier: definition.Carrier,
		EndUse:  definition.EndUse,
	})
}

func energyExplanationNameIsWaterMeter(name string) bool {
	definition, ok := energyMeterAliasDefinitionForName(name)
	if !ok {
		definition, ok = energyMeterEndUseCarrierDefinitionForName(name)
	}
	return ok && canonicalEnergyPathCarrier(definition.Carrier) == "water"
}

func energyExplanationUnitIsSiteEnergy(unit string) bool {
	if energyPathUnitIsCanonicalSiteEnergy(unit) {
		return true
	}
	_, ok := energyPathEnergyUnitNormalization(unit)
	return ok
}

func energyExplanationLevelNotRequested(plan *PurposeRunPlan, level string) bool {
	return plan != nil && strings.EqualFold(strings.TrimSpace(plan.BasicEnergyDetail), PurposeBasicEnergyDetailLight) && (level == "load" || level == "heat")
}

func energyCompletenessNotRequestedLevel(level string, label string) EnergyCompletenessLevel {
	return EnergyCompletenessLevel{
		Level:   level,
		Status:  "not_requested",
		Message: label + ": not requested by the current output plan",
	}
}

func expectedEnergyExplanationOutputs(plan *PurposeRunPlan, level string) []string {
	out := []string{}
	planRequestsBasicEnergy := false
	if plan != nil {
		for _, object := range plan.OutputObjects {
			if !purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) {
				continue
			}
			planRequestsBasicEnergy = true
			switch level {
			case "energy":
				if strings.EqualFold(object.ObjectType, "Output:Meter") {
					out = appendUniquePurposeString(out, object.KeyValue)
				} else if strings.EqualFold(object.ObjectType, "Output:Variable") {
					if definition, ok := energyVariableAliasDefinitionForName(object.VariableName); ok && definition.HierarchyLevel != "zone_direct_use" {
						out = appendUniquePurposeString(out, object.VariableName)
					}
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
	if plan != nil && planRequestsBasicEnergy {
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
		if def, ok := energyVariableAliasDefinitionForName(name); ok {
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
	if total == 0 {
		message = label + ": not requested by the current output plan"
	} else if level == "heat" && found == 0 {
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
		sourceIDs := []string{}
		for _, source := range sources {
			if energySourceMatchesAvailabilityName(source, name, level) {
				status = "found"
				if source.ID != "" {
					sourceIDs = appendUniquePurposeString(sourceIDs, source.ID)
				}
			}
		}
		out = append(out, EnergySourceAvailabilityEntry{Name: name, Level: level, Status: status, SourceIDs: sourceIDs})
	}
	return out
}

func energySourceMatchesAvailabilityName(source EnergyDataSource, name string, level string) bool {
	if strings.EqualFold(source.Name, name) || strings.EqualFold(source.KeyValue, name) {
		return true
	}
	if level == "energy" || level == "context" {
		return energyNamesShareEnergyAliasGroup(source.Name, name) || energyNamesShareEnergyAliasGroup(source.KeyValue, name)
	}
	if level == "load" {
		return energyNamesShareLoadAliasGroup(source.Name, name) || energyNamesShareLoadAliasGroup(source.KeyValue, name)
	}
	if level == "heat" {
		return energyNamesShareHeatAliasGroup(source.Name, name) || energyNamesShareHeatAliasGroup(source.KeyValue, name)
	}
	return false
}

func energyNamesShareEnergyAliasGroup(left string, right string) bool {
	leftKey := energyKnownAliasGroupKey(left)
	rightKey := energyKnownAliasGroupKey(right)
	return leftKey != "" && leftKey == rightKey
}

func energyNamesShareLoadAliasGroup(left string, right string) bool {
	leftDef, leftOK := energyLoadAliasDefinitionForName(left)
	rightDef, rightOK := energyLoadAliasDefinitionForName(right)
	if !leftOK || !rightOK {
		return false
	}
	return leftDef.Kind == rightDef.Kind && leftDef.ServiceKind == rightDef.ServiceKind && leftDef.Scope == rightDef.Scope
}

func energyNamesShareHeatAliasGroup(left string, right string) bool {
	leftDef, leftOK := energyHeatAliasDefinitionForName(left)
	rightDef, rightOK := energyHeatAliasDefinitionForName(right)
	if !leftOK || !rightOK {
		return false
	}
	return leftDef.Kind == rightDef.Kind &&
		leftDef.HeatCategory == rightDef.HeatCategory &&
		leftDef.ObjectScoped == rightDef.ObjectScoped &&
		energyHeatAliasExplicitSign(left) == energyHeatAliasExplicitSign(right)
}

func energyKnownAliasGroupKey(name string) string {
	if def, ok := energyMeterAliasDefinitionForName(name); ok {
		return energyMeterDefinitionGroupKey(def)
	}
	if def, ok := energyVariableAliasDefinitionForName(name); ok {
		return energyMeterDefinitionGroupKey(def)
	}
	if def, ok := energyMeterEndUseCarrierDefinitionForName(name); ok {
		return energyMeterDefinitionGroupKey(def)
	}
	return ""
}

func energyMeterDefinitionGroupKey(def energyMeterAliasDefinition) string {
	return strings.Join([]string{
		def.Kind,
		normalizeEnergyOutputName(def.Carrier),
		normalizeEnergyOutputName(def.EndUse),
	}, "|")
}

func sourceAvailabilityEntriesForLevel(expected []string, level string, sources []EnergyDataSource, notRequested bool) []EnergySourceAvailabilityEntry {
	if len(expected) == 0 {
		status := "not_applicable"
		if notRequested {
			status = "not_requested"
		}
		return []EnergySourceAvailabilityEntry{{
			Name:   "not requested by current output plan",
			Level:  level,
			Status: status,
		}}
	}
	return sourceAvailabilityEntries(expected, level, sources)
}

func missingEnergySourceCategories(availability []EnergySourceAvailabilityEntry) []string {
	out := []string{}
	for _, item := range availability {
		if item.Status != "missing" || strings.EqualFold(item.Level, "context") {
			continue
		}
		out = appendUniquePurposeString(out, strings.TrimSpace(item.Level+": "+item.Name))
	}
	return out
}

func energyExplanationEnergyNodeID(item energyExplanationSeries) string {
	if item.Stage == "carrier" || strings.HasSuffix(item.Kind, ".total") {
		return "energy.carrier." + item.Carrier
	}
	nodeID := "energy.end_use." + item.EndUse + "." + item.Carrier
	if item.MeterHierarchyLevel == "zone_direct_use" {
		nodeID += "." + energyExplanationZoneSuffix(item.ZoneName)
	}
	return strings.TrimSuffix(nodeID, ".")
}

func energyExplanationIsSupportEndUse(item energyExplanationSeries) bool {
	if item.Stage != "support" && item.Level != "energy" {
		return false
	}
	switch canonicalEnergyPathPart(firstNonEmpty(item.EndUse, energyExplanationKindSuffix(item.Kind))) {
	case "generators", "onsite_production", "production", "storage_discharge", "electricity_purchased", "purchased_electricity", "electricity_sold", "sold_electricity", "surplus_sold":
		return true
	default:
		return item.Kind == "energy.generators"
	}
}

func energyExplanationEndUseCarrierKey(endUse string, carrier string) string {
	return normalizeEnergyOutputName(endUse) + "|" + normalizeEnergyOutputName(carrier)
}

func energyExplanationInternalGainEnergyTarget(item energyExplanationSeries) (energyExplanationInternalGainTarget, bool) {
	switch item.Kind {
	case "heat.lighting":
		return energyExplanationInternalGainTarget{endUse: "interior_lighting", carrier: "electricity"}, true
	case "heat.equipment":
		nameKey := normalizeEnergyOutputName(item.sourceName)
		if strings.Contains(nameKey, "gasequipment") {
			return energyExplanationInternalGainTarget{endUse: "interior_equipment", carrier: "natural_gas"}, true
		}
		if strings.Contains(nameKey, "electricequipment") {
			return energyExplanationInternalGainTarget{endUse: "interior_equipment", carrier: "electricity"}, true
		}
	}
	return energyExplanationInternalGainTarget{}, false
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
	if strings.TrimSpace(item.DriverCategory) != "" {
		nodeID = "heat.driver." + canonicalEnergyPathCategory(item.DriverCategory)
	}
	if sign := strings.TrimSpace(item.HeatSign); sign != "" && energyExplanationHeatNodeUsesSignSuffix(item) {
		nodeID += "." + metricID(sign)
	}
	if suffix := energyExplanationZoneSuffix(item.ZoneName); suffix != "" {
		nodeID += "." + suffix
	}
	return nodeID
}

func energyExplanationHeatNodeUsesSignSuffix(item energyExplanationSeries) bool {
	// The frozen v1 contract represented internal-gain families with one stable
	// node ID. Canonical driver preparation assigns a category/role before v2
	// construction, where an explicit sign suffix is required to keep gain and
	// loss contributions distinct. Preserve the legacy ID only for unprepared
	// internal-gain series read through the v1 compatibility boundary.
	if strings.TrimSpace(item.DriverCategory) != "" || strings.TrimSpace(item.DriverSourceRole) != "" {
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(item.HeatCategory), "internal_gains")
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
	if definition, ok := energyCarrierTaxonomyDefinitionFor(carrier); ok {
		return definition.Label
	}
	return cases.Title(language.Und, cases.NoLower).String(strings.ReplaceAll(carrier, "_", " "))
}

func energyServiceLabel(serviceKind string) string {
	switch serviceKind {
	case "cooling":
		return "Cooling"
	case "heating":
		return "Heating"
	case "unmet_or_residual":
		return "Unmet / residual"
	default:
		return cases.Title(language.Und, cases.NoLower).String(strings.ReplaceAll(serviceKind, "_", " "))
	}
}

func energyResidualVisibilityThreshold(reference float64) float64 {
	return math.Max(0.001, math.Abs(reference)*0.001)
}

const energyCarrierResidualAbsoluteGraphThresholdKWh = 0.01

func energyCarrierResidualGraphThreshold(reference float64) float64 {
	// Either a material share or an absolute amount warrants a visible branch.
	return math.Min(energyCarrierResidualAbsoluteGraphThresholdKWh, math.Abs(reference)*0.02)
}

func energyCarrierResidualVisibleInGraph(expected float64, residual float64) bool {
	return residual > 0 && residual > energyCarrierResidualGraphThreshold(expected)
}

func energyReconciliationStatus(expected float64, residual float64) string {
	if math.Abs(residual) <= energyResidualVisibilityThreshold(expected) {
		return "balanced"
	}
	if residual < 0 {
		return "overmapped"
	}
	return "residual"
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
	if zoneName == "" {
		keys := make([]string, 0, len(loadNodesByZoneService))
		for key := range loadNodesByZoneService {
			_, candidateService := splitEnergyExplanationZoneServiceKey(key)
			if candidateService == serviceKind {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			if id := firstExistingNodeID(nodes, loadNodesByZoneService[key]...); id != "" {
				return id
			}
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

func energyExplanationHours(series []energyExplanationSeries) []int {
	seen := map[int]bool{}
	for _, item := range series {
		for hour, value := range item.Hourly {
			if value == 0 {
				continue
			}
			seen[hour] = true
		}
	}
	hours := make([]int, 0, len(seen))
	for hour := range seen {
		hours = append(hours, hour)
	}
	sort.Ints(hours)
	return hours
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

func roundedEnergyExplanationHourly(hourly map[int]float64) map[int]float64 {
	if len(hourly) == 0 {
		return nil
	}
	out := map[int]float64{}
	for hour, value := range hourly {
		out[hour] = roundedEnergyNumber(value)
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
	levelOrder := map[string]int{
		"driver":   0,
		"heat":     0,
		"load":     1,
		"end_use":  2,
		"energy":   2,
		"carrier":  3,
		"support":  4,
		"residual": 5,
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if levelOrder[nodes[i].Level] != levelOrder[nodes[j].Level] {
			return levelOrder[nodes[i].Level] < levelOrder[nodes[j].Level]
		}
		if levelOrder[nodes[i].Level] == 0 {
			leftOrder := energyDriverCategoryOrder(firstNonEmpty(nodes[i].DriverCategory, nodes[i].Kind))
			rightOrder := energyDriverCategoryOrder(firstNonEmpty(nodes[j].DriverCategory, nodes[j].Kind))
			if leftOrder != rightOrder {
				return leftOrder < rightOrder
			}
		}
		return nodes[i].ID < nodes[j].ID
	})
}

func sortEnergyExplanationEdges(edges []EnergyExplanationEdge) {
	sort.SliceStable(edges, func(i, j int) bool {
		return edges[i].ID < edges[j].ID
	})
}

func edgeID(prefix string, _ string, fromID string, toID string) string {
	return prefix + "." + metricID(fromID) + "." + metricID(toID)
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
