package simulation

// Only the validated parent's original direct surface name establishes this
// boundary. A Floor/Ground category, equipment-like name, or neighbouring
// surface is not evidence that another surface is an active emitter.
func energyPathRadiantSurfaceTarget(context energyDriverBuildContext, key string) (energyPathRadiantLoadTarget, bool) {
	if context.Enabled && normalizeEnergySurfaceKey(key) != "" {
		for _, target := range context.RadiantLoads {
			if target.SurfaceName != "" && target.ZoneName != "" && normalizeEnergySurfaceKey(target.SurfaceName) == normalizeEnergySurfaceKey(key) {
				return target, true
			}
		}
	}
	return energyPathRadiantLoadTarget{}, false
}

// The streaming reader uses the same exact ownership boundary as preparation,
// before a passive surface-category aggregate can absorb an active surface.
func energyPathRadiantSurfaceIsContext(context energyDriverBuildContext, key string) bool {
	_, found := energyPathRadiantSurfaceTarget(context, key)
	return found
}

func applyEnergyPathRadiantSurfaceContext(item *energyExplanationSeries, policy *energyDriverSourcePolicy, context energyDriverBuildContext) {
	if !context.Enabled || len(context.RadiantLoads) == 0 {
		return
	}
	if item.SurfaceScoped && item.Kind == "heat.surface_inside_face_convection" {
		if target, found := energyPathRadiantSurfaceTarget(context, item.sourceKeyValue); found {
			item.ZoneName = target.ZoneName
			policy.Role = energyDriverSourceRoleContext
			policy.Explanation = "Inside-face convection of a validated active radiant surface is thermal context, not an independently measured passive envelope driver. The original observation is retained without estimating a passive or storage split."
		}
		return
	}
	if item.Kind != "heat.surface_convection" || policy.Role != energyDriverSourceRoleReconciliation || item.ZoneName == "" {
		return
	}
	for _, target := range context.RadiantLoads {
		if target.SurfaceName != "" && target.ZoneName != "" && normalizeEnergySurfaceKey(target.ZoneName) == normalizeEnergySurfaceKey(item.ZoneName) {
			// The aggregate includes active emission. Subtracting only passive
			// surfaces would reintroduce the excluded emission as inferred storage.
			// Preserve its observed value but do not perform that decomposition.
			policy.Role = energyDriverSourceRoleContext
			policy.Explanation = "Zone surface-convection aggregate includes a validated active radiant surface. It is retained as thermal context; subtracting passive surface details cannot establish a passive or storage contribution."
			return
		}
	}
}
