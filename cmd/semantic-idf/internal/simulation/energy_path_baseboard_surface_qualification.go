package simulation

import (
	"fmt"
	"strings"
)

const energyPathBaseboardRecipientExplanation = "This surface-convection observation includes a potential response to a configured radiant-convective baseboard together with other surface heat-balance effects. It is not isolated passive conduction or a measured baseboard-only gain. Declared fractions establish coupling, not operation or an observed energy split; no heat is added or subtracted."

const energyPathBaseboardRecipientAggregateExplanation = "This surface-convection aggregate includes declared radiant-convective baseboard recipients and can contain both environmental and HVAC responses. It is not an isolated passive or baseboard-only contribution. The original total and heat-balance allocation are retained without estimating or adding a radiant split."

type energyPathBaseboardSurfaceReference struct {
	target    energyPathBaseboardTarget
	recipient energyPathBaseboardRecipient
}

// Only the validated original recipient roster supplies this qualification.
// Unlike an embedded radiant emitter, a receiving building surface remains in
// the existing surface exchange and allocation denominator. Configuration is
// non-additive provenance, never evidence that equipment operated in a period.
func applyEnergyPathBaseboardRecipientQualification(series []energyExplanationSeries, sources []EnergyDataSource, context energyDriverBuildContext) {
	if !context.Enabled || len(context.BaseboardTargets) == 0 {
		return
	}
	qualifiedSourceIDs := map[string]bool{}
	for index := range sources {
		source := &sources[index]
		if source.IsMeter || source.SourceType == "derived_formula" {
			continue
		}
		var references []energyPathBaseboardSurfaceReference
		explanation := energyPathBaseboardRecipientExplanation
		switch normalizeEnergyOutputName(source.Name) {
		case "surface inside face convection heat gain energy", "surface inside face convection heat gain rate":
			references = energyPathBaseboardSurfaceReferences(context, source.KeyValue, source.ZoneName)
		case "zone air heat balance surface convection rate":
			if source.ZoneName == "" || normalizeEnergySurfaceKey(source.ZoneName) == normalizeEnergySurfaceKey(source.KeyValue) {
				references = energyPathBaseboardSurfaceReferences(context, "", source.KeyValue)
			}
			explanation = energyPathBaseboardRecipientAggregateExplanation
		}
		if len(references) == 0 {
			continue
		}
		qualifiedSourceIDs[source.ID] = true
		source.Explanation = appendEnergyPathBaseboardExplanation(source.Explanation, explanation)
		for _, reference := range references {
			target, recipient := reference.target, reference.recipient
			trace := fmt.Sprintf("Non-additive recipient configuration: %s %q [%s, object %d] -> surface %q [object %d], Zone %q; radiant fraction=%g; recipient share of radiant output=%g.",
				target.Component.ObjectType, target.Component.ObjectName, target.Component.ID, target.Component.ObjectIndex,
				recipient.SurfaceName, recipient.SurfaceObjectIndex, recipient.ZoneName, target.RadiantFraction, recipient.Fraction)
			source.Explanation = appendEnergyPathBaseboardExplanation(source.Explanation, trace)
			// Do not put the parent on a category/node's RelatedEntityIDs:
			// legacy projection propagates those IDs to nonrecipient peers.
			source.RelatedEntityIDs = appendUniqueStrings(source.RelatedEntityIDs, target.Component.ID)
		}
	}
	for index := range series {
		item := &series[index]
		if item.Level != "heat" {
			continue
		}
		if item.SurfaceScoped && item.Kind == "heat.surface_inside_face_convection" {
			if len(energyPathBaseboardSurfaceReferences(context, item.sourceKeyValue, item.ZoneName)) > 0 {
				item.DriverExplanation = appendEnergyPathBaseboardExplanation(item.DriverExplanation, energyPathBaseboardRecipientExplanation)
			}
			continue
		}
		if item.Kind == "heat.surface_convection" && len(energyPathBaseboardSurfaceReferences(context, "", item.ZoneName)) > 0 {
			item.DriverExplanation = appendEnergyPathBaseboardExplanation(item.DriverExplanation, energyPathBaseboardRecipientAggregateExplanation)
			continue
		}
		if item.parseCategoryAggregate {
			for _, sourceID := range item.SourceIDs {
				if qualifiedSourceIDs[sourceID] {
					item.DriverExplanation = appendEnergyPathBaseboardExplanation(item.DriverExplanation, energyPathBaseboardRecipientAggregateExplanation)
					break
				}
			}
		}
	}
}

func energyPathBaseboardSurfaceReferences(context energyDriverBuildContext, surfaceKey, zoneName string) []energyPathBaseboardSurfaceReference {
	surfaceKey, zoneName = normalizeEnergySurfaceKey(surfaceKey), normalizeEnergySurfaceKey(zoneName)
	if !context.Enabled || surfaceKey == "" && zoneName == "" {
		return nil
	}
	var out []energyPathBaseboardSurfaceReference
	for _, target := range context.BaseboardTargets {
		if target.Component.ID == "" || !strings.EqualFold(target.Component.ObjectType, energyPathBaseboardElectricType) ||
			!(target.RadiantFraction > 0 && target.RadiantFraction <= 1) || target.ZoneName == "" ||
			zoneName != "" && normalizeEnergySurfaceKey(target.ZoneName) != zoneName {
			continue
		}
		for _, recipient := range target.Recipients {
			if normalizeEnergySurfaceKey(recipient.ZoneName) != normalizeEnergySurfaceKey(target.ZoneName) ||
				!(recipient.Fraction > 0 && recipient.Fraction <= 1) || normalizeEnergySurfaceKey(recipient.SurfaceName) == "" ||
				surfaceKey != "" && normalizeEnergySurfaceKey(recipient.SurfaceName) != surfaceKey {
				continue
			}
			out = append(out, energyPathBaseboardSurfaceReference{target: target, recipient: recipient})
		}
	}
	return out
}

func appendEnergyPathBaseboardExplanation(existing, extra string) string {
	if strings.Contains(existing, extra) {
		return existing
	}
	return strings.TrimSpace(existing + " " + extra)
}
