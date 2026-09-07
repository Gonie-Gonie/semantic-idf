package simulation

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const (
	energyMultiplierAlreadyModelTotal = "already_model_total"
	energyMultiplierRequiresZone      = "requires_zone_multiplier"
	energyMultiplierRequiresGroup     = "requires_group_multiplier"
	energyMultiplierUnknown           = "unknown"
)

type energyZoneMultiplierRecord struct {
	ZoneName        string
	ZoneMultiplier  float64
	GroupMultiplier float64
}

func (record energyZoneMultiplierRecord) effectiveMultiplier() float64 {
	zone := positiveEnergyMultiplier(record.ZoneMultiplier)
	group := positiveEnergyMultiplier(record.GroupMultiplier)
	return zone * group
}

type energyEffectiveMultiplierIndex struct {
	Enabled          bool
	Zones            map[string]energyZoneMultiplierRecord
	SpaceZones       map[string]string
	OutputKeyZones   map[string]string
	GroupMultipliers map[string]float64
}

func buildEnergyEffectiveMultiplierIndex(doc idf.Document) energyEffectiveMultiplierIndex {
	index := energyEffectiveMultiplierIndex{
		Enabled:          true,
		Zones:            map[string]energyZoneMultiplierRecord{},
		SpaceZones:       map[string]string{},
		OutputKeyZones:   map[string]string{},
		GroupMultipliers: map[string]float64{},
	}
	zoneLists := map[string][]string{}
	groupByZoneList := map[string]float64{}

	for _, object := range doc.Objects {
		name := purposeObjectName(object)
		key := normalizePurposeToken(name)
		switch {
		case strings.EqualFold(strings.TrimSpace(object.Type), "Zone"):
			if key == "" {
				continue
			}
			index.Zones[key] = energyZoneMultiplierRecord{
				ZoneName:        name,
				ZoneMultiplier:  energyObjectNumericField(object, 6, 1, "multiplier"),
				GroupMultiplier: 1,
			}
		case strings.EqualFold(strings.TrimSpace(object.Type), "Space"):
			zoneName := energyObjectStringField(object, 1, "zone name")
			if key != "" && zoneName != "" {
				index.SpaceZones[key] = zoneName
			}
		case strings.EqualFold(strings.TrimSpace(object.Type), "ZoneList"):
			for _, field := range object.Fields[1:] {
				if value := strings.TrimSpace(field.Value); value != "" {
					zoneLists[key] = appendUniqueStrings(zoneLists[key], value)
				}
			}
		case strings.EqualFold(strings.TrimSpace(object.Type), "ZoneGroup"):
			listName := energyObjectStringField(object, 1, "zone list name")
			if listName == "" {
				continue
			}
			listKey := normalizePurposeToken(listName)
			current := positiveEnergyMultiplier(groupByZoneList[listKey])
			groupByZoneList[listKey] = current * energyObjectNumericField(object, 2, 1, "zone list multiplier")
		}
	}

	for listKey, multiplier := range groupByZoneList {
		index.GroupMultipliers[listKey] = positiveEnergyMultiplier(multiplier)
		for _, zoneName := range zoneLists[listKey] {
			zoneKey := normalizePurposeToken(zoneName)
			record, ok := index.Zones[zoneKey]
			if !ok {
				continue
			}
			record.GroupMultiplier *= positiveEnergyMultiplier(multiplier)
			index.Zones[zoneKey] = record
		}
	}

	// Ideal Loads variables use the equipment object as their RDD key. Resolve
	// that key through the IDF HVAC ownership graph instead of guessing from
	// object names.
	for _, relation := range idf.AnalyzeHVAC(doc).ZoneRelations {
		for _, component := range relation.ZoneEquipment {
			if !strings.EqualFold(strings.TrimSpace(component.ObjectType), "ZoneHVAC:IdealLoadsAirSystem") {
				continue
			}
			if key := normalizePurposeToken(component.ObjectName); key != "" {
				index.OutputKeyZones[key] = relation.ZoneName
			}
		}
	}
	return index
}

func (index energyEffectiveMultiplierIndex) resolve(name string) (energyZoneMultiplierRecord, bool) {
	if !index.Enabled {
		return energyZoneMultiplierRecord{}, false
	}
	key := normalizePurposeToken(name)
	if record, ok := index.Zones[key]; ok {
		return record, true
	}
	if zoneName := index.SpaceZones[key]; zoneName != "" {
		if record, ok := index.Zones[normalizePurposeToken(zoneName)]; ok {
			return record, true
		}
	}
	if zoneName := index.OutputKeyZones[key]; zoneName != "" {
		if record, ok := index.Zones[normalizePurposeToken(zoneName)]; ok {
			return record, true
		}
	}
	return energyZoneMultiplierRecord{}, false
}

func energyObjectStringField(object idf.Object, fallbackIndex int, commentToken string) string {
	token := normalizePurposeToken(commentToken)
	for _, field := range object.Fields {
		if normalizePurposeToken(field.Comment) == token {
			return strings.TrimSpace(field.Value)
		}
	}
	if fallbackIndex >= 0 && fallbackIndex < len(object.Fields) {
		return strings.TrimSpace(object.Fields[fallbackIndex].Value)
	}
	return ""
}

func energyObjectNumericField(object idf.Object, fallbackIndex int, fallback float64, commentToken string) float64 {
	value := energyObjectStringField(object, fallbackIndex, commentToken)
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || number <= 0 {
		return fallback
	}
	return number
}

func positiveEnergyMultiplier(value float64) float64 {
	if value > 0 {
		return value
	}
	return 1
}

func energyExplanationMultiplierRequirement(item energyExplanationSeries) string {
	name := normalizeEnergyOutputName(firstNonEmpty(item.SourceName, item.sourceName))
	if strings.TrimSpace(item.ZoneName) != "" &&
		(strings.EqualFold(strings.TrimSpace(item.MeterHierarchyLevel), "zone_direct_use") || canonicalEnergyPathBasis(item.Basis, "") == "direct_zone_energy") {
		// Zone direct-use report variables are zone/object contributions, not
		// facility meters. Apply the owning Zone and ZoneList multipliers before
		// exposing their scoped site-energy subtotal. Canonical/custom inputs may
		// carry only the direct-zone basis, so the hierarchy marker is not the
		// sole discriminator.
		return energyMultiplierRequiresZone
	}
	if item.Stage == "carrier" || item.Stage == "end_use" || item.Stage == "support" || item.Level == "energy" {
		return energyMultiplierAlreadyModelTotal
	}
	if strings.Contains(name, "zone ideal loads ") || strings.Contains(name, "zone system predicted sensible load") {
		return energyMultiplierAlreadyModelTotal
	}
	if strings.HasPrefix(name, "zone list ") {
		return energyMultiplierRequiresGroup
	}
	if item.Stage == "driver" || item.Level == "heat" {
		return energyMultiplierRequiresZone
	}
	if item.Stage == "load" || item.Level == "load" {
		if strings.EqualFold(strings.TrimSpace(item.PathType), "system") || strings.EqualFold(strings.TrimSpace(item.PathType), "plant") {
			return energyMultiplierAlreadyModelTotal
		}
		if strings.Contains(name, "zone air system ") || strings.Contains(name, "zone predicted sensible load") || strings.EqualFold(strings.TrimSpace(item.PathType), "zone") {
			return energyMultiplierRequiresZone
		}
	}
	return energyMultiplierUnknown
}

func energyExplanationMultiplierForSeries(item energyExplanationSeries, index energyEffectiveMultiplierIndex) (float64, string, bool) {
	requirement := energyExplanationMultiplierRequirement(item)
	if requirement == energyMultiplierAlreadyModelTotal {
		return 1, requirement, true
	}
	if requirement == energyMultiplierUnknown {
		return 1, requirement, false
	}
	lookup := firstNonEmpty(item.ZoneName, item.SurfaceName, item.sourceKeyValue)
	if requirement == energyMultiplierRequiresGroup {
		if factor, ok := index.GroupMultipliers[normalizePurposeToken(lookup)]; ok {
			return positiveEnergyMultiplier(factor), requirement, true
		}
	}
	record, ok := index.resolve(lookup)
	if !ok {
		return 1, energyMultiplierUnknown, false
	}
	if requirement == energyMultiplierRequiresGroup {
		return positiveEnergyMultiplier(record.GroupMultiplier), requirement, true
	}
	return record.effectiveMultiplier(), requirement, true
}

func applyEnergyExplanationMultipliers(series []energyExplanationSeries, sources []EnergyDataSource, index energyEffectiveMultiplierIndex) ([]energyExplanationSeries, []EnergyDataSource, []EnergyWarning) {
	sourceIndex := make(map[string]int, len(sources))
	for i := range sources {
		sourceIndex[sources[i].ID] = i
	}
	warnings := []EnergyWarning{}
	for i := range series {
		item := canonicalEnergyExplanationSeries(series[i])
		if item.multiplierApplied {
			series[i] = item
			continue
		}
		// Some RDD families, notably ZoneHVAC:IdealLoadsAirSystem, use the
		// equipment object rather than the owning zone as their output key.
		// Canonicalize ownership before grouping or exposing scoped metadata.
		if record, ok := index.resolve(firstNonEmpty(item.ZoneName, item.SourceKey, item.sourceKeyValue)); ok &&
			(item.Stage == "load" || item.Stage == "driver") {
			item.ZoneName = record.ZoneName
			item.SourceFamily = energyExplanationCanonicalSourceFamily(item)
			item.CanonicalFamily = energyExplanationCanonicalIdentity(item)
		}
		factor, application, resolved := energyExplanationMultiplierForSeries(item, index)
		item.EffectiveMultiplier = factor
		item.MultiplierApplication = application
		item.Total = roundedEnergyNumber(item.RawTotal * factor)
		item.Monthly = scaledEnergyExplanationPeriodValues(item.RawMonthly, factor)
		item.Daily = scaledEnergyExplanationPeriodValues(item.RawDaily, factor)
		item.Hourly = scaledEnergyExplanationPeriodValues(item.RawHourly, factor)
		item.SelectedRange = roundedEnergyNumber(item.RawSelectedRange * factor)
		item.multiplierApplied = true
		series[i] = item

		if !resolved && application == energyMultiplierUnknown && (item.Stage == "driver" || item.Stage == "load") {
			warnings = appendEnergyDriverWarning(warnings, EnergyWarning{
				Severity: "warning",
				Code:     "energy_multiplier_unknown",
				Message:  fmt.Sprintf("Multiplier semantics for %q could not be resolved; the reported value is retained with factor 1.", firstNonEmpty(item.SourceName, item.CanonicalKind)),
			})
		}
		if item.parseCategoryAggregate {
			continue
		}
		for _, sourceID := range item.SourceIDs {
			position, ok := sourceIndex[sourceID]
			if !ok {
				continue
			}
			source := &sources[position]
			source.RawValue = roundedEnergyNumber(item.RawTotal)
			source.EffectiveValue = roundedEnergyNumber(item.Total)
			source.EffectiveMultiplier = roundedEnergyNumber(factor)
			source.MultiplierApplication = application
			if resolved && factor > 0 && !math.IsNaN(factor) && !math.IsInf(factor, 0) && energyDataSourceValueKnown(*source, energySourceObservedRaw) {
				source.observedValuePresence |= energySourceObservedEffective
			}
			if source.ZoneName == "" {
				source.ZoneName = item.ZoneName
			}
		}
	}

	// Detailed surface rows are intentionally removed after their streaming
	// category aggregate is built, but they remain first-class inspector
	// sources. Apply only the owning zone/group multiplier here; never reuse the
	// geometry surface multiplier.
	for i := range sources {
		source := &sources[i]
		if source.MultiplierApplication != "" {
			continue
		}
		item := canonicalEnergyExplanationSeries(energyExplanationSeries{
			Level:          energyExplanationLevelForSourceName(source.Name),
			Kind:           source.DriverCategory,
			ZoneName:       source.ZoneName,
			SourceName:     source.Name,
			sourceName:     source.Name,
			sourceKeyValue: source.KeyValue,
		})
		factor, application, resolved := energyExplanationMultiplierForSeries(item, index)
		source.EffectiveMultiplier = roundedEnergyNumber(factor)
		source.MultiplierApplication = application
		source.EffectiveValue = roundedEnergyNumber(source.RawValue * factor)
		if resolved && factor > 0 && !math.IsNaN(factor) && !math.IsInf(factor, 0) && energyDataSourceValueKnown(*source, energySourceObservedRaw) {
			source.observedValuePresence |= energySourceObservedEffective
		}
	}
	return series, sources, warnings
}

func energyExplanationLevelForSourceName(name string) string {
	if _, ok := energyHeatAliasDefinitionForName(name); ok {
		return "heat"
	}
	if _, ok := energyLoadAliasDefinitionForName(name); ok {
		return "load"
	}
	return "energy"
}

func scaledEnergyExplanationPeriodValues(values map[int]float64, factor float64) map[int]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[int]float64, len(values))
	for period, value := range values {
		out[period] = roundedEnergyNumber(value * factor)
	}
	return out
}
