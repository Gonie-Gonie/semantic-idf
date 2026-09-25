package simulation

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const energyPathSimpleVentilationFanID = "fans.simple_ventilation.electricity"
const energyPathSimpleVentilationFanName = "Zone Ventilation Fan Electricity Energy"

// This is a reporting-Zone aggregate, not one synthetic Fan:* component and
// not one source per ventilation object. Both native ventilation families
// share one Zone reporting variable. Members retain original input identity.
type energyPathSimpleVentilationTarget struct {
	ZoneName string
	Zone     idf.ComponentRef
	Members  []idf.ComponentRef
}

func energyPathSimpleVentilationFanDefinition() energyPathDirectHVACComponentDefinition {
	return energyPathDirectHVACComponentDefinition{
		ID:         energyPathSimpleVentilationFanID,
		ObjectType: "Zone",
		Energy:     energyMeterAliasDefinition{Kind: "energy.fans", Label: "Simple ventilation fan electricity", Carrier: "electricity", EndUse: "fans", HierarchyLevel: "zone_direct_use", Aliases: []string{energyPathSimpleVentilationFanName}},
	}
}

func energyPathSimpleVentilationObject(objectType string) bool {
	return strings.EqualFold(strings.TrimSpace(objectType), "ZoneVentilation:DesignFlowRate") || strings.EqualFold(strings.TrimSpace(objectType), "ZoneVentilation:WindandStackOpenArea")
}

func energyPathSimpleVentilationField(object idf.Object, index int) string {
	if index < 0 || index >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[index].Value)
}

func energyPathSimpleVentilationRef(object idf.Object, index int) idf.ComponentRef {
	return idf.ComponentRef{ID: fmt.Sprintf("component:%d", index), ObjectType: object.Type, ObjectName: energyPathSimpleVentilationField(object, 0), ObjectIndex: index}
}

// Bounded native 25.1 contract: exact Zone selectors, with the entire original
// ventilation namespace censused before selection. ZoneList/Space expansion is
// not guessed; any such selector declines this aggregate cohort rather than
// assigning only its convenient direct-Zone subset. This is not an engine input
// validator. Existing physics/controls/schedules are never rewritten.
func energyPathSimpleVentilationTargets(doc idf.Document) []energyPathSimpleVentilationTarget {
	found := false
	for _, object := range doc.Objects {
		found = found || energyPathSimpleVentilationObject(object.Type)
	}
	if !found {
		return nil
	}
	objects := map[string][]int{}
	versions := 0
	for index, object := range doc.Objects {
		if strings.EqualFold(strings.TrimSpace(object.Type), "Version") {
			versions++
			if energyPathSimpleVentilationField(object, 0) != "25.1" {
				return nil
			}
		}
		key := energyPathDirectHVACComponentKey(object.Type, energyPathSimpleVentilationField(object, 0))
		objects[key] = append(objects[key], index)
	}
	if versions != 1 {
		return nil
	}
	byZone := map[string]*energyPathSimpleVentilationTarget{}
	invalid := map[string]bool{}
	names := map[string][]string{}
	for index, object := range doc.Objects {
		if !energyPathSimpleVentilationObject(object.Type) {
			continue
		}
		owner := energyPathSimpleVentilationField(object, 1)
		key := energyPathDirectHVACComponentKey("Zone", owner)
		zoneIndices := objects[key]
		// An ambiguous selector cannot prove which Zone aggregate it affects.
		if owner == "" || len(zoneIndices) != 1 {
			return nil
		}
		for _, other := range []string{"ZoneList", "Space", "SpaceList"} {
			if len(objects[energyPathDirectHVACComponentKey(other, owner)]) > 0 {
				return nil
			}
		}
		zone := doc.Objects[zoneIndices[0]]
		canonical := strings.ToLower(energyPathSimpleVentilationField(zone, 0))
		if byZone[canonical] == nil {
			byZone[canonical] = &energyPathSimpleVentilationTarget{ZoneName: energyPathSimpleVentilationField(zone, 0), Zone: energyPathSimpleVentilationRef(zone, zoneIndices[0])}
		}
		name := strings.ToLower(energyPathSimpleVentilationField(object, 0))
		if name == "" {
			invalid[canonical] = true
		}
		names[name] = append(names[name], canonical)
		byZone[canonical].Members = append(byZone[canonical].Members, energyPathSimpleVentilationRef(object, index))
		if strings.EqualFold(strings.TrimSpace(object.Type), "ZoneVentilation:DesignFlowRate") {
			// Blank fields have native defaults. Natural and zero-pressure
			// inputs still get a target; an input zero is NOT a SQL observation.
			kind := strings.ToLower(energyPathSimpleVentilationField(object, 8))
			if kind != "" && kind != "natural" && kind != "intake" && kind != "exhaust" && kind != "balanced" {
				invalid[canonical] = true
			}
			for _, spec := range []struct {
				index            int
				fallback         float64
				strictlyPositive bool
			}{{9, 0, false}, {10, 1, true}} {
				value := spec.fallback
				text := energyPathSimpleVentilationField(object, spec.index)
				if text != "" {
					parsed, err := strconv.ParseFloat(text, 64)
					if err != nil {
						invalid[canonical] = true
						continue
					}
					value = parsed
				}
				if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || spec.strictlyPositive && value == 0 {
					invalid[canonical] = true
				}
			}
		}
	}
	for _, owners := range names {
		if len(owners) > 1 {
			for _, owner := range owners {
				invalid[owner] = true
			}
		}
	}
	balances := map[string]int{}
	for _, object := range doc.Objects {
		if !strings.EqualFold(strings.TrimSpace(object.Type), "ZoneAirBalance:OutdoorAir") {
			continue
		}
		owner := strings.ToLower(energyPathSimpleVentilationField(object, 1))
		if byZone[owner] == nil {
			continue
		}
		balances[owner]++
		method := strings.ToLower(energyPathSimpleVentilationField(object, 2))
		// Quadrature uses the combined outdoor-air reporting family instead.
		// An invalid native choice cannot serve as identity evidence either.
		if method != "" && method != "none" {
			invalid[owner] = true
		}
	}
	var out []energyPathSimpleVentilationTarget
	for owner, target := range byZone {
		if invalid[owner] || balances[owner] > 1 {
			continue
		}
		out = append(out, *target)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].ZoneName) < strings.ToLower(out[j].ZoneName) })
	return out
}

func energyPathSimpleVentilationDirectTargets(targets []energyPathSimpleVentilationTarget) []energyPathDirectHVACComponentTarget {
	var out []energyPathDirectHVACComponentTarget
	for _, target := range targets {
		out = append(out, energyPathDirectHVACComponentTarget{Definition: energyPathSimpleVentilationFanDefinition(), KeyValue: target.ZoneName, ZoneName: target.ZoneName})
	}
	return out
}

func (builder *purposePlanBuilder) addEnergyPathSimpleVentilationOutputs() {
	selected, scoped := purposeSelectedZoneSet(builder.request.Scope)
	for _, target := range energyPathSimpleVentilationTargets(builder.doc) {
		if scoped && !selected[normalizePurposeToken(target.ZoneName)] {
			continue
		}
		builder.addVariableWithReasonAndScopeZone(SimulationPurposeBasicEnergy, target.ZoneName, energyPathSimpleVentilationFanName, "Monthly", "medium",
			"Native Zone-aggregate simple-ventilation fan electricity; one source per Zone, distinct from thermal ventilation transfer and other fans. Native meter contribution is preserved with factor 1.", "Basic Energy Path", target.ZoneName)
	}
}

// The original native Hourly wildcard can cover each new scoped Monthly
// owner. Reuse its actual opener; a scheduled/foreign-key observation cannot
// cover the unfiltered chart. Other reporting families keep their own policy.
func (builder *purposePlanBuilder) addEnergyPathSimpleVentilationHourlyOutput(monthly PurposeOutputObject) bool {
	if !strings.EqualFold(monthly.ObjectType, "Output:Variable") ||
		!strings.EqualFold(monthly.VariableName, energyPathSimpleVentilationFanName) ||
		!strings.EqualFold(monthly.ReportingFrequency, "Monthly") ||
		strings.TrimSpace(purposeFieldValue(monthly.Fields, "Schedule Name")) != "" {
		return false
	}
	owner := ""
	for _, target := range energyPathSimpleVentilationTargets(builder.doc) {
		if strings.EqualFold(monthly.KeyValue, target.ZoneName) && strings.EqualFold(monthly.ScopeZoneName, target.ZoneName) {
			owner = target.ZoneName
			break
		}
	}
	if owner == "" {
		return false
	}
	existing := make([]PurposeOutputObject, 0, len(builder.existing))
	for _, output := range builder.existing {
		existing = append(existing, output)
	}
	sort.SliceStable(existing, func(i, j int) bool { return existing[i].Signature < existing[j].Signature })
	for _, candidates := range [][]PurposeOutputObject{existing, builder.objects} {
		for _, output := range candidates {
			key := strings.TrimSpace(output.KeyValue)
			if !strings.EqualFold(output.ObjectType, "Output:Variable") ||
				!strings.EqualFold(output.VariableName, energyPathSimpleVentilationFanName) ||
				!strings.EqualFold(output.ReportingFrequency, "Hourly") ||
				strings.TrimSpace(purposeFieldValue(output.Fields, "Schedule Name")) != "" ||
				(key != "" && key != "*" && !strings.EqualFold(key, owner)) ||
				(output.ScopeZoneName != "" && !strings.EqualFold(output.ScopeZoneName, owner)) {
				continue
			}
			output.PurposeIDs = normalizePurposeIDs(append(output.PurposeIDs, SimulationPurposeBasicEnergy))
			output.Reason = "Basic Energy Path"
			builder.addObject(output)
			return true
		}
	}
	builder.addVariableWithReasonAndScopeZone(SimulationPurposeBasicEnergy, owner, energyPathSimpleVentilationFanName, "Hourly", "heavy",
		"Hourly native Zone-aggregate ventilation fan electricity for the selected source chart; Monthly remains the accounting authority.", "Basic Energy Path", owner)
	return true
}

// The reusable direct reader already keeps Monthly J observations separate
// from Hourly charts and fails closed on incomplete/negative/duplicate rows.
// Its factor-one branch is correct here for a DIFFERENT reason than HVAC coil
// sizing: native 25.1 registration passes zoneMult=zoneListMult=1. Do not claim
// the report variable has been scaled to the physical repeated-Zone building.
func applyEnergyPathSimpleVentilationSourceContext(sources []EnergyDataSource, targets []energyPathSimpleVentilationTarget) {
	owners := map[string]energyPathSimpleVentilationTarget{}
	for _, target := range targets {
		owners[strings.ToLower(target.ZoneName)] = target
	}
	for index := range sources {
		source := &sources[index]
		owner, ok := owners[strings.ToLower(strings.TrimSpace(source.KeyValue))]
		if !ok || source.IsMeter || source.SourceType != "sql_report_data" || !strings.EqualFold(source.Name, energyPathSimpleVentilationFanName) || !strings.EqualFold(source.ZoneName, owner.ZoneName) {
			continue
		}
		members := make([]string, 0, len(owner.Members))
		for _, member := range owner.Members {
			members = append(members, fmt.Sprintf("%s %q (original index %d)", member.ObjectType, member.ObjectName, member.ObjectIndex))
		}
		source.Explanation = "Native Zone-aggregate ventilation fan electricity, not one measurement per ventilation object. EnergyPlus 25.1 registers this native meter contribution with Zone/ZoneList multiplier 1; raw and effective site energy are not expanded again. No thermal service or AirLoop path is implied. Original members: " + strings.Join(members, "; ") + "."
	}
}

// Conservative path qualification for a pure native ventilation node. Caller
// may clear inherited thermal/AirLoop paths when true, retaining every numeric
// and source field. Mixed/foreign source nodes return false, not a partial
// truncation. This predicate is not an ownership authority: SQL binding above
// has already proved the original Zone aggregate before such a node exists.
func energyPathNodeHasOnlySimpleVentilationSources(node EnergyExplanationNode, sources []EnergyDataSource, scope EnergyExplanationScope) bool {
	if scope.Kind != "zone" || scope.ZoneName == "" || !strings.EqualFold(strings.TrimSpace(node.ZoneName), strings.TrimSpace(scope.ZoneName)) || !energyPathLegacyNodeIsDirectZoneEnergy(node) || legacyEnergyNodeIsCarrier(node) || node.EndUse != "fans" || len(node.SourceIDs) == 0 {
		return false
	}
	if canonicalEnergyPathBasis(node.Basis, "") != "direct_zone_energy" && !strings.EqualFold(node.MeterHierarchyLevel, "zone_direct_use") {
		return false
	}
	byID := map[string]EnergyDataSource{}
	counts := map[string]int{}
	for _, source := range sources {
		byID[source.ID] = source
		counts[source.ID]++
	}
	for _, id := range node.SourceIDs {
		source := byID[id]
		if id == "" || counts[id] != 1 || source.SourceType != "sql_report_data" || source.IsMeter || !strings.EqualFold(source.Name, energyPathSimpleVentilationFanName) ||
			!strings.EqualFold(source.ZoneName, scope.ZoneName) || !strings.EqualFold(source.KeyValue, scope.ZoneName) || !strings.EqualFold(source.ReportingFrequency, "Monthly") || source.SourceUnit != "J" || source.NormalizedUnit != "kWh" {
			return false
		}
	}
	return true
}
