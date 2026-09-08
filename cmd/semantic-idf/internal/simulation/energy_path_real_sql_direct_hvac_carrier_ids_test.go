package simulation

import "fmt"

// A Zone with a complete direct observation for every monthly service carrier
// cannot inherit an allocated HVAC energy row. Its fresh monthly carrier rows,
// and their monthly-first annual sum, have the plain carrier/period identity.
// Prove this per Zone from original reporting/ownership classes, not candidate
// IDs, positive values, or the building's overall direct-energy percentage.
func epathSQLZoneCarrierDirectMonthlyIDs(frames epathSQLFrames, model epathRealSQLModel, zone string) (bool, error) {
	if len(model.DirectHVACComponents) == 0 || len(model.Services) == 0 || len(model.FanPools) != 0 {
		return false, nil
	}
	for _, auxiliary := range model.Auxiliaries {
		if auxiliary.Weight != "unassigned" {
			return false, nil
		}
	}
	sites := map[string]epathRealSQLSite{}
	for _, site := range model.Site {
		if site.ID == "" || sites[site.ID].ID != "" {
			return false, fmt.Errorf("direct-only carrier identity requires exact site declarations")
		}
		sites[site.ID] = site
	}
	for _, service := range model.Services {
		if len(service.SiteIDs) == 0 {
			return false, fmt.Errorf("direct-only carrier identity lacks service pools")
		}
		for _, id := range service.SiteIDs {
			site, exists := sites[id]
			if !exists || site.Facility || site.EndUse != service.Service || site.Carrier == "" {
				return false, fmt.Errorf("direct-only carrier identity has a contradictory service pool")
			}
			annual, err := epathSQLSiteIsAnnual(frames, id)
			if err != nil {
				return false, err
			}
			if annual {
				return false, nil
			}
			for month := 1; month <= 12; month++ {
				value, exists := frames.DirectHVAC[epathSQLDirectHVACKey(zone, service.Service, site.Carrier, month)]
				if !exists || !value.Present {
					return false, nil
				}
				if !value.Quantity.valid() || value.Quantity.Value < 0 || len(value.SourceIDs) == 0 {
					return false, fmt.Errorf("direct-only carrier identity has no valid original observation")
				}
			}
		}
	}
	return true, nil
}
