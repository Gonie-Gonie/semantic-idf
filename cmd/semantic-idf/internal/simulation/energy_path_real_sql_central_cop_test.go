package simulation

import "reflect"

// Called once by the existing compiler after its source/pool validation. This
// is a ratio-label qualification, not permission to add another paid source.
func epathSQLCentralCoolingCOP(frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService, selected []epathSQLHVACConsumptionPoolFrame) bool {
	if service.Service != "cooling" || service.RatioKind != "coefficient_of_performance" || service.FallbackRatioKind != "coefficient_of_performance" || !epathSQLCentralSharedOnlyService(frames, model, service, selected) || len(model.HVACConsumptionPools) != 2 || len(frames.HVACConsumptionPools) != 2 {
		return false
	}
	roles := map[string]bool{}
	for _, pool := range frames.HVACConsumptionPools {
		if len(pool.Shared) != 1 {
			return false
		}
		role, err := epathSQLCentralSharedRole(pool.Shared[0].Member)
		if err != nil || roles[role] || pool.Declaration.SiteID != role+".electricity" {
			return false
		}
		roles[role] = true
	}
	if !roles["cooling"] || !roles["heating"] {
		return false
	}
	exactZero := func(q epathSQLQuantity) bool {
		lo, hi := q.bounds()
		return q.valid() && q.Value == 0 && q.Error == 0 && lo == 0 && hi == 0
	}
	// The actual original has source-loop District contexts. They remain
	// separate observed zero meters, never members of either paid system pool.
	for _, site := range model.Site {
		if site.Carrier == "electricity" {
			continue
		}
		name, group := "", ""
		switch {
		case site.Carrier == "district_cooling" && site.EndUse == "cooling" && !site.Facility:
			name = "Cooling:DistrictCooling"
			group = "Facility:DistrictCooling:Cooling"
		case site.Carrier == "district_heating" && site.EndUse == "heating" && !site.Facility:
			name = "Heating:DistrictHeatingWater"
			group = "Facility:DistrictHeatingWater:Heating"
		case site.Carrier == "district_cooling" && site.EndUse == "" && site.Facility:
			name = "DistrictCooling:Facility"
			group = "Facility:DistrictCooling"
		case site.Carrier == "district_heating" && site.EndUse == "" && site.Facility:
			name = "DistrictHeatingWater:Facility"
			group = "Facility:DistrictHeatingWater"
		default:
			return false
		}
		want := epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: name, Unit: "J"}}, Keys: []string{""}, IsMeter: true}
		ids, values := frames.SiteSources[site.ID], frames.Site[site.ID]
		if site.Tabular != nil || !reflect.DeepEqual(site.Source, want) || len(ids) != 1 || len(values) != 12 {
			return false
		}
		if _, exists := frames.SiteAnnual[site.ID]; exists {
			return false
		}
		id := ids[0]
		s, known := frames.SourceIdentities[id]
		if !known || id <= 0 || s.DictionaryIndex != id || s.Name != name || !s.IsMeter || s.KeyValue != "" || s.ReportingFrequency != "Monthly" || s.SourceUnit != "J" || s.IndexGroup != group || s.Rows != 12 || s.MissingRows != 0 || s.RawSum == nil || *s.RawSum != 0 || s.EnergyKWh == nil || *s.EnergyKWh != 0 || len(s.Months) != 12 || len(frames.SourceRaw[id]) != 12 || len(frames.SourceEffective[id]) != 12 || frames.SourceZone[id] != "" {
			return false
		}
		for month, bucket := range s.Months {
			if bucket.Month != month+1 || bucket.Rows != 1 || bucket.MissingRows != 0 || bucket.RawSum == nil || *bucket.RawSum != 0 || bucket.EnergyKWh == nil || *bucket.EnergyKWh != 0 || values[month] == nil || !exactZero(*values[month]) || !exactZero(frames.SourceRaw[id][month]) || !exactZero(frames.SourceEffective[id][month]) {
				return false
			}
		}
	}
	return true
}
