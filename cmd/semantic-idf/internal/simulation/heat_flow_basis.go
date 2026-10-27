package simulation

import (
	"database/sql"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/epinput"
)

// HeatFlowRateBasis records corrections to the diagnostic EnergyPlus reports.
// Reported arrays use the matching category's Observed mask, including zeros.
// Generic simulation series and the original output files remain unmodified.
type HeatFlowRateBasis struct {
	EffectiveMultiplier      float64   `json:"effectiveMultiplier"`
	SystemAir                string    `json:"systemAir,omitempty"`
	SystemConvective         string    `json:"systemConvective,omitempty"`
	ReportedSystemAir        []float64 `json:"reportedSystemAir,omitempty"`
	ReportedSystemConvective []float64 `json:"reportedSystemConvective,omitempty"`
}

type heatFlowBasisContext struct {
	multipliers map[string]float64
	verified    bool
	nonAirOnly  bool
}

type heatFlowBasisConversion struct {
	multiplier        float64
	systemAir         bool
	modeledConvective bool
}

// The input provides ownership and factors, never proof of the producing
// engine version: a newer executable can run an older-version input file.
type heatFlowSourceOptions struct {
	InputPath     string
	EngineVersion string
}

var heatFlowEngineVersion = regexp.MustCompile(`(?:Version\s+)?(\d+\.\d+)(?:\.|\b)`)

func heatFlowVerifiedReportVersion(value string) bool {
	match := heatFlowEngineVersion.FindStringSubmatch(value)
	if len(match) != 2 {
		return false
	}
	// Verified against the tagged CalcZoneComponentLoadSums implementation.
	switch match[1] {
	case "22.1", "22.2", "23.1", "23.2", "24.1", "24.2", "25.1", "25.2", "26.1":
		return true
	}
	return false
}

func newHeatFlowBasisContext(db *sql.DB, sources ...heatFlowSourceOptions) heatFlowBasisContext {
	context := heatFlowBasisContext{multipliers: map[string]float64{}}
	for _, source := range sources {
		context.verified = heatFlowVerifiedReportVersion(source.EngineVersion)
		path := source.InputPath
		if strings.TrimSpace(path) == "" {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		model, err := epinput.Parse(path, content)
		if err != nil {
			continue
		}
		doc := epinput.ToIDFDocument(model)
		context.nonAirOnly = true
		for _, object := range doc.Objects {
			// These two per-zone contributions are added to the multiplied
			// NonAirSystemResponse in the same report. Without separate outputs
			// their sum cannot safely be divided by the effective multiplier.
			if strings.EqualFold(object.Type, "ZoneHVAC:HighTemperatureRadiant") || strings.EqualFold(object.Type, "SwimmingPool:Indoor") {
				context.nonAirOnly = false
			}
		}
		for _, record := range buildEnergyEffectiveMultiplierIndex(doc).Zones {
			context.multipliers[normalizeHeatFlowName(record.ZoneName)] = record.effectiveMultiplier()
		}
		break
	}
	if db != nil {
		var version string
		if err := db.QueryRow(`SELECT EnergyPlusVersion FROM Simulations ORDER BY SimulationIndex LIMIT 1`).Scan(&version); err == nil {
			// The producing engine version is authoritative over the IDF version.
			context.verified = heatFlowVerifiedReportVersion(version)
		}
		if rows, err := db.Query(`SELECT ZoneName, Multiplier, ListMultiplier FROM Zones`); err == nil {
			defer rows.Close()
			for rows.Next() {
				var name string
				var zone, list float64
				if rows.Scan(&name, &zone, &list) != nil {
					continue
				}
				if zone > 0 && list > 0 && !math.IsInf(zone*list, 0) && !math.IsNaN(zone*list) {
					context.multipliers[normalizeHeatFlowName(name)] = zone * list
				}
			}
		}
	}
	return context
}

func (context heatFlowBasisContext) value(builder *heatFlowZoneBuilder, category HeatFlowCategory, frame int, value float64) float64 {
	return context.prepare(builder.name, category).value(builder, frame, value)
}

func (context heatFlowBasisContext) prepare(zoneName string, category HeatFlowCategory) heatFlowBasisConversion {
	if category.ID != "systemAir" && category.ID != "systemConvective" {
		return heatFlowBasisConversion{}
	}
	if !context.verified {
		return heatFlowBasisConversion{}
	}
	multiplier, known := context.multipliers[normalizeHeatFlowName(zoneName)]
	if !known || multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return heatFlowBasisConversion{}
	}
	return heatFlowBasisConversion{multiplier: multiplier, systemAir: category.ID == "systemAir", modeledConvective: multiplier == 1 || context.nonAirOnly}
}

func (conversion heatFlowBasisConversion) value(builder *heatFlowZoneBuilder, frame int, value float64) float64 {
	if conversion.multiplier <= 0 {
		return value
	}
	if builder.rateBasis == nil {
		builder.rateBasis = &HeatFlowRateBasis{EffectiveMultiplier: conversion.multiplier}
	}
	basis := builder.rateBasis
	if conversion.systemAir {
		basis.SystemAir = "modeled_zone"
		for len(basis.ReportedSystemAir) <= frame {
			basis.ReportedSystemAir = append(basis.ReportedSystemAir, 0)
		}
		basis.ReportedSystemAir[frame] = value
		return value / conversion.multiplier
	}
	basis.SystemConvective = "energyplus_reported"
	for len(basis.ReportedSystemConvective) <= frame {
		basis.ReportedSystemConvective = append(basis.ReportedSystemConvective, 0)
	}
	basis.ReportedSystemConvective[frame] = value
	if conversion.modeledConvective {
		basis.SystemConvective = "modeled_zone"
		return value / conversion.multiplier
	}
	return value
}

func applyHeatFlowBasisWarnings(dataset *HeatFlowDataset) {
	for _, zone := range dataset.Zones {
		if zone.RateBasis != nil && zone.RateBasis.EffectiveMultiplier != 1 && zone.RateBasis.SystemConvective == "energyplus_reported" {
			// A reported zero can be cancellation of components with different
			// bases; it does not establish a zero normalized transfer.
			dataset.Warnings = append(dataset.Warnings, "System convective heat gain retains the EnergyPlus reported basis: zone multipliers may affect only part of this aggregate; no unsupported division was applied.")
			return
		}
	}
}
