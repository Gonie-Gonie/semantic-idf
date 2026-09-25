package simulation

import (
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Requires HEAT-ONLY-PRODUCTION-INSTALL-20260925.patch. The request gate
// consumes exported resolved child relations and actual heating service paths;
// it does not parse native positional fields or turn a thermostat into a sole
// allocation recipient. The IDF resolver owns the wrapper/fan/coil port proof.
func energyPathHeatOnlyFanOutputOwner(report idf.HVACReport, loop idf.HVACLoop, wrapper idf.HVACComponent) (string, bool) {
	const heatOnly = "AirLoopHVAC:Unitary:Furnace:HeatOnly"
	if !strings.EqualFold(wrapper.ObjectType, heatOnly) || !wrapper.Exists || !strings.EqualFold(loop.Type, "AirLoopHVAC") {
		return "", false
	}
	var fan idf.HVACComponentReference
	children := 0
	for _, ref := range report.ComponentReferences {
		if ref.FromObjectIndex == wrapper.ObjectIndex && strings.EqualFold(ref.FromObjectType, heatOnly) && strings.EqualFold(ref.FromObjectName, wrapper.ObjectName) && strings.EqualFold(ref.TargetObjectType, "Fan:OnOff") {
			children++
			fan = ref
		}
	}
	if children != 1 || !fan.TargetExists || fan.TargetObjectName == "" || fan.TypeFieldIndex != 8 || fan.NameFieldIndex != 9 {
		return "", false
	}
	// All typed references, including another wrapper or a direct branch fan,
	// must agree that this is the one native child occurrence.
	uses := 0
	for _, ref := range report.ComponentReferences {
		if strings.EqualFold(ref.TargetObjectType, "Fan:OnOff") && strings.EqualFold(ref.TargetObjectName, fan.TargetObjectName) {
			uses++
		}
	}
	if uses != 1 {
		return "", false
	}
	for _, zone := range report.ServiceModel.ZoneServices {
		for _, path := range zone.Paths {
			if path.ServiceKind != "heating" || path.PathType != "central_air" || path.ZoneName == "" || path.SpaceName != "" || path.AirLoop == nil || path.AirLoop.ObjectIndex != loop.ObjectIndex || !strings.EqualFold(path.AirLoop.Type, loop.Type) || !strings.EqualFold(path.AirLoop.Name, loop.Name) {
				continue
			}
			// This exact conditioning identity exists only after the native
			// HeatOnly resolver has checked its control Zone, branch ownership,
			// OnOff/fuel children, placement and exact supplied-terminal owner.
			for _, component := range path.Conditioning {
				if component.ObjectIndex == wrapper.ObjectIndex && strings.EqualFold(component.ObjectType, heatOnly) && strings.EqualFold(component.ObjectName, wrapper.ObjectName) {
					return fan.TargetObjectName, true
				}
			}
		}
	}
	return "", false
}
