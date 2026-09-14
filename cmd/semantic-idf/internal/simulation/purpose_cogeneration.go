package simulation

import "sort"

// This parent meter is separate from the twenty PV reporting identities. The
// native inverter's ancillary input belongs to it, not to produced electricity.
// Requesting it proves neither that a dictionary exists nor a numeric zero.
func (builder *purposePlanBuilder) addEnergyPathCogenerationOutputs() {
	inventory := energyPathBuildCogenerationInventory(builder.doc)
	if !inventory.ReviewedVersion || inventory.CustomMeterNames[energyPathCogenerationKey("Cogeneration:Electricity")] {
		return
	}
	hasNativeInverter := false
	for _, target := range energyPathBuildPVElectricalInventory(builder.doc).Targets {
		if target.IdentityValid && target.Definition.ID == "inverter.ancillary_ac" {
			hasNativeInverter = true
		}
	}
	if !hasNativeInverter {
		return
	}
	existing := make([]PurposeOutputObject, 0, len(builder.existing))
	for _, output := range builder.existing {
		existing = append(existing, output)
	}
	sort.SliceStable(existing, func(i, j int) bool { return existing[i].Signature < existing[j].Signature })
	for _, frequency := range []string{"Monthly", "Hourly"} {
		intent := energyPathPVElectricalIntent{Frequency: frequency, Target: energyPathPVElectricalTarget{IdentityValid: true, Definition: energyPathPVElectricalDefinition{ID: "cogeneration.electricity", Name: "Cogeneration:Electricity", IsMeter: true, Role: "consumed_input"}}}
		request := intent.request()
		request.Description = "Native cogeneration electricity consumed input, including inverter ancillary AC; parent budget counted once, not production."
		covered := false
		for _, candidates := range [][]PurposeOutputObject{existing, builder.objects} {
			for _, candidate := range candidates {
				if !energyPathPVElectricalRequestCovers(candidate, intent) {
					continue
				}
				candidate.PurposeIDs = appendUniquePurposeIDsForPVElectrical(candidate.PurposeIDs, SimulationPurposeBasicEnergy)
				candidate.Reason, candidate.Description = request.Reason, request.Description
				builder.addObject(candidate)
				covered = true
				break
			}
			if covered {
				break
			}
		}
		if !covered {
			builder.addObject(request)
		}
	}
}
