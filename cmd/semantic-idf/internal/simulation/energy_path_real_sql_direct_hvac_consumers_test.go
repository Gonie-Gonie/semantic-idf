package simulation

import (
	"fmt"
	"reflect"
)

// A merged service node can contain direct and allocated carriers. Only the
// independently observed direct branch contributes to the carrier's direct
// subtotal; the node's preferred display basis cannot reclassify every branch.
func epathSQLDirectHVACCarrierParts(service *epathSQLZoneServiceProof) (map[string]epathSQLQuantity, error) {
	if service == nil || !service.DirectHVAC || len(service.Carriers) == 0 {
		return nil, fmt.Errorf("direct-first carrier requires a complete independent service proof")
	}
	direct, totals := map[string]epathSQLQuantity{}, map[string]epathSQLQuantity{}
	for _, branch := range service.Branches {
		if _, exists := service.Carriers[branch.Carrier]; !exists || branch.Basis != "direct_zone_energy" && branch.Basis != "service_path_allocation" || !branch.Quantity.valid() || branch.Quantity.Value < 0 {
			return nil, fmt.Errorf("invalid independent direct-first carrier branch")
		}
		totals[branch.Carrier] = totals[branch.Carrier].add(branch.Quantity)
		if branch.Basis == "direct_zone_energy" {
			direct[branch.Carrier] = direct[branch.Carrier].add(branch.Quantity)
		}
	}
	for carrier, total := range service.Carriers {
		if !epathSQLZoneCarrierQuantityEqual(totals[carrier], total) {
			return nil, fmt.Errorf("direct-first branches do not equal the independent carrier subtotal")
		}
	}
	return direct, nil
}

// Zone ratios use only that Zone's independently proved consumption branches.
// A broad meter is allowed only by an actual allocation branch, never simply
// because it exists in the building's reporting dictionary.
func epathSQLDirectHVACQualitySources(quality *epathSQLQualityProof, service *epathSQLZoneServiceProof) error {
	if quality == nil || service == nil || !service.DirectHVAC {
		return fmt.Errorf("direct-first quality requires an independent Zone service proof")
	}
	if _, err := epathSQLDirectHVACCarrierParts(service); err != nil {
		return err
	}
	originals := map[string]map[string]epathSQLOriginalSource{}
	for _, branch := range service.Branches {
		if len(branch.Originals) == 0 && branch.Quantity.Value > 0 {
			return fmt.Errorf("positive direct-first branch lacks its original consumption evidence")
		}
		if len(branch.Originals) > 0 {
			if err := epathSQLValidateOriginalSources(branch.Originals); err != nil {
				return err
			}
		}
		if originals[branch.Carrier] == nil {
			originals[branch.Carrier] = map[string]epathSQLOriginalSource{}
		}
		for key, original := range branch.Originals {
			if branch.Basis == "direct_zone_energy" && (original.DirectHVAC == nil || original.DirectHVAC.Service != service.Service || original.DirectHVAC.Carrier != branch.Carrier) {
				return fmt.Errorf("direct quality branch contains a different service/carrier or unproved component")
			}
			if previous, duplicate := originals[branch.Carrier][key]; duplicate && !reflect.DeepEqual(previous, original) {
				return fmt.Errorf("direct-first quality has contradictory original consumption identities")
			}
			// The same original annual source may support distinct monthly
			// relation branches; exact identities form a union, not extra energy.
			originals[branch.Carrier][key] = original
		}
	}
	quality.SiteSources[service.Service] = map[string][]int{}
	quality.SiteOriginals[service.Service] = originals
	return nil
}
