package idf

import (
	"fmt"
	"math"
	"strings"
)

type ThermalTopologyBatchMetricDefinition struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Unit           string `json:"unit,omitempty"`
	BasisSensitive bool   `json:"basisSensitive,omitempty"`
}

type ThermalTopologyBatchMetricValue struct {
	Value          float64 `json:"value"`
	DisplayValue   string  `json:"displayValue"`
	Status         string  `json:"status"`
	AreaBasis      string  `json:"areaBasis,omitempty"`
	Coverage       float64 `json:"coverage,omitempty"`
	HasCoverage    bool    `json:"hasCoverage,omitempty"`
	BasisSensitive bool    `json:"basisSensitive,omitempty"`
}

type ThermalTopologyBatchSummary struct {
	AreaBasis  string                                     `json:"areaBasis"`
	UACoverage float64                                    `json:"uaCoverage"`
	Metrics    map[string]ThermalTopologyBatchMetricValue `json:"metrics"`
}

type thermalTopologyBatchTotals struct {
	area        float64
	opening     float64
	ua          float64
	coveredArea float64
}

var thermalTopologyBatchMetricDefinitions = []ThermalTopologyBatchMetricDefinition{
	{ID: "topology_zone_count", Name: "Zone count", Unit: "count"},
	{ID: "topology_space_count", Name: "Space count", Unit: "count"},
	{ID: "topology_connection_count", Name: "Thermal connection count", Unit: "count"},
	{ID: "topology_exterior_area", Name: "Exterior area", Unit: "m2", BasisSensitive: true},
	{ID: "topology_ground_area", Name: "Ground area", Unit: "m2", BasisSensitive: true},
	{ID: "topology_interzone_area", Name: "Interzone interface area", Unit: "m2", BasisSensitive: true},
	{ID: "topology_adiabatic_area", Name: "Adiabatic area", Unit: "m2", BasisSensitive: true},
	{ID: "topology_exterior_opening_area", Name: "Exterior opening area", Unit: "m2", BasisSensitive: true},
	{ID: "topology_exterior_wwr", Name: "Exterior WWR", Unit: "%", BasisSensitive: true},
	{ID: "topology_exterior_ua", Name: "Exterior UA", Unit: "W/K", BasisSensitive: true},
	{ID: "topology_ground_ua", Name: "Ground UA", Unit: "W/K", BasisSensitive: true},
	{ID: "topology_interzone_ua", Name: "Interzone UA", Unit: "W/K", BasisSensitive: true},
	{ID: "topology_closed_zone_count", Name: "Closed zone count", Unit: "count"},
	{ID: "topology_open_shell_zone_count", Name: "Open-shell zone count", Unit: "count"},
	{ID: "topology_boundary_diagnostic_count", Name: "Boundary diagnostic count", Unit: "count"},
	{ID: "topology_air_coupling_count", Name: "Air coupling count", Unit: "count"},
	{ID: "topology_ua_coverage", Name: "U-value coverage", Unit: "%", BasisSensitive: true},
}

func ThermalTopologyBatchMetricDefinitions() []ThermalTopologyBatchMetricDefinition {
	return append([]ThermalTopologyBatchMetricDefinition(nil), thermalTopologyBatchMetricDefinitions...)
}

func SummarizeThermalTopologyForBatch(report ThermalTopologyReport, areaBasis string) ThermalTopologyBatchSummary {
	areaBasis = strings.ToLower(strings.TrimSpace(areaBasis))
	if areaBasis != "physical" {
		areaBasis = "effective"
	}
	groups := map[string]*thermalTopologyBatchTotals{
		"exterior": {}, "ground": {}, "interzone": {}, "adiabatic": {}, "all": {},
	}
	connectionCount := 0
	for _, connection := range report.Connections {
		if len(connection.BoundaryIDs) == 0 || connection.RelationKind == "air_coupling" {
			continue
		}
		connectionCount++
		group := thermalTopologyBatchRelationGroup(connection.RelationKind)
		if group == "" {
			continue
		}
		area := connection.EffectiveGrossArea
		opening := connection.EffectiveOpeningArea
		ua := connection.TotalUA
		hasUA := connection.HasUA
		if areaBasis == "physical" {
			area = connection.PhysicalGrossArea
			opening = connection.PhysicalOpeningArea
			ua = connection.PhysicalTotalUA
			hasUA = connection.HasPhysicalUA
		}
		for _, key := range []string{group, "all"} {
			groups[key].area += area
			groups[key].opening += opening
			if hasUA {
				groups[key].ua += ua
				groups[key].coveredArea += area
			}
		}
	}
	zoneCount, spaceCount := 0, 0
	for _, node := range report.Nodes {
		switch node.Kind {
		case "zone":
			zoneCount++
		case "space":
			spaceCount++
		}
	}
	closedZones := 0
	for _, signature := range report.ZoneSignatures {
		if signature.ClosedShell {
			closedZones++
		}
	}
	openZones := maxThermalTopologyInt(0, zoneCount-closedZones)
	overallCoverage := thermalTopologyBatchCoverage(groups["all"])
	values := map[string]ThermalTopologyBatchMetricValue{}
	addCount := func(id string, value int) {
		values[id] = thermalTopologyBatchValue(float64(value), "count", "ok", areaBasis, false, 0, false)
	}
	addArea := func(id string, value float64) {
		values[id] = thermalTopologyBatchValue(value, "m2", "ok", areaBasis, true, 0, false)
	}
	addUA := func(id string, group *thermalTopologyBatchTotals) {
		coverage := thermalTopologyBatchCoverage(group)
		status := thermalTopologyBatchCoverageStatus(group.area, coverage)
		values[id] = thermalTopologyBatchValue(group.ua, "W/K", status, areaBasis, true, coverage, true)
	}
	addCount("topology_zone_count", zoneCount)
	addCount("topology_space_count", spaceCount)
	addCount("topology_connection_count", connectionCount)
	addArea("topology_exterior_area", groups["exterior"].area)
	addArea("topology_ground_area", groups["ground"].area)
	addArea("topology_interzone_area", groups["interzone"].area)
	addArea("topology_adiabatic_area", groups["adiabatic"].area)
	addArea("topology_exterior_opening_area", groups["exterior"].opening)
	wwr := 0.0
	if groups["exterior"].area > 0 {
		wwr = (groups["exterior"].opening / groups["exterior"].area) * 100
	}
	values["topology_exterior_wwr"] = thermalTopologyBatchValue(wwr, "%", "ok", areaBasis, true, 0, false)
	addUA("topology_exterior_ua", groups["exterior"])
	addUA("topology_ground_ua", groups["ground"])
	addUA("topology_interzone_ua", groups["interzone"])
	addCount("topology_closed_zone_count", closedZones)
	addCount("topology_open_shell_zone_count", openZones)
	addCount("topology_boundary_diagnostic_count", len(report.IssueLinks))
	addCount("topology_air_coupling_count", len(report.AirCouplings))
	values["topology_ua_coverage"] = thermalTopologyBatchValue(overallCoverage*100, "%", thermalTopologyBatchCoverageStatus(groups["all"].area, overallCoverage), areaBasis, true, overallCoverage, true)
	return ThermalTopologyBatchSummary{AreaBasis: areaBasis, UACoverage: overallCoverage, Metrics: values}
}

func thermalTopologyBatchRelationGroup(relation string) string {
	relation = strings.ToLower(strings.TrimSpace(relation))
	switch {
	case relation == "exterior":
		return "exterior"
	case relation == "ground" || strings.HasPrefix(relation, "ground_") || relation == "foundation":
		return "ground"
	case strings.HasPrefix(relation, "interzone_") || strings.HasPrefix(relation, "interspace_"):
		return "interzone"
	case strings.HasPrefix(relation, "adiabatic_"):
		return "adiabatic"
	default:
		return ""
	}
}

func thermalTopologyBatchCoverage(totals *thermalTopologyBatchTotals) float64 {
	if totals == nil || totals.area <= 0 {
		return 0
	}
	return math.Min(1, totals.coveredArea/totals.area)
}

func thermalTopologyBatchCoverageStatus(area float64, coverage float64) string {
	if area <= 0 {
		return "not_applicable"
	}
	if coverage <= 0 {
		return "missing"
	}
	if coverage < 0.999999 {
		return "partial"
	}
	return "ok"
}

func thermalTopologyBatchValue(value float64, unit string, status string, areaBasis string, basisSensitive bool, coverage float64, hasCoverage bool) ThermalTopologyBatchMetricValue {
	return ThermalTopologyBatchMetricValue{
		Value:          roundedNumber(value, 4),
		DisplayValue:   fmt.Sprintf("%g", roundedNumber(value, 4)),
		Status:         status,
		AreaBasis:      areaBasis,
		Coverage:       roundedNumber(coverage, 4),
		HasCoverage:    hasCoverage,
		BasisSensitive: basisSensitive,
	}
}
