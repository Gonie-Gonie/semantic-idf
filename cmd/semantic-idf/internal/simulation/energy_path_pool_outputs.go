package simulation

import "github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"

// A native reporting identity is enough to request observation; it does not
// prove a pool/plant recipient, complete water topology, or a resource split.
// Keep these separate gates so an unresolved pool cannot erase the measured
// source that the reader must retain as explicitly unassigned.
func energyPathPoolOutputRequests(doc idf.Document) []PurposeOutputObject {
	inventory := energyPathNativePoolInventory(doc)
	if !inventory.HasNativePool || !inventory.SchemaReviewed {
		return nil
	}
	var out []PurposeOutputObject
	add := func(component idf.ComponentRef, fuelType, zoneName string) {
		for _, definition := range energyPathPoolOutputDefinitions() {
			if !definition.matchesOriginal(component.ObjectType, fuelType) {
				continue
			}
			for _, frequency := range []string{"Monthly", "Hourly"} {
				for _, isRate := range []bool{false, true} {
					name := definition.EnergyName
					if isRate {
						name = definition.RateName
					}
					description := "Native component thermal-transfer context, non-additive to purchased energy and Zone-air delivered load; model-total observation, not a representative Zone quantity."
					if definition.Role == energyPathPoolPurchasedConstituent {
						description = "Native purchased-consumption constituent. Monthly Energy is the source-local budget authority; topology and recipient completeness are validated separately, otherwise the observed budget remains unassigned."
					}
					weight := "medium"
					if frequency == "Hourly" {
						weight = "heavy"
						description += " Hourly source-chart companion, never a second additive budget."
					}
					if isRate {
						description += " Native Rate companion, not a replacement for an unavailable Monthly Energy observation."
					}
					out = append(out, PurposeOutputObject{
						ObjectType: "Output:Variable",
						Fields: []idf.OutputFieldValue{
							{Name: "Key Value", Value: component.ObjectName},
							{Name: "Variable Name", Value: name},
							{Name: "Reporting Frequency", Value: frequency},
						},
						PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Weight: weight,
						Description: description, Reason: "Basic Energy Path", ScopeZoneName: zoneName,
					})
				}
			}
		}
	}
	for _, pool := range inventory.Pools {
		if pool.IdentityValid {
			add(pool.Component, "", pool.ZoneName)
		}
	}
	for _, source := range inventory.Sources {
		if source.IdentityValid {
			add(source.Component, source.FuelType, "")
		}
	}
	return out
}

func (builder *purposePlanBuilder) addEnergyPathPoolOutputs() {
	for _, output := range energyPathPoolOutputRequests(builder.doc) {
		builder.addObject(output)
	}
}
