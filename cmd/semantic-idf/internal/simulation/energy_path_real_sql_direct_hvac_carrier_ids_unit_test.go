package simulation

import "testing"

func TestEnergyPathSQLDirectHVACCarrierIdentityIsPerOwner(t *testing.T) {
	path, model, _ := epathSQLDirectHVACSourceUnitFixture(t)
	model.Services = []epathRealSQLService{
		{Service: "cooling", SiteIDs: []string{"cooling.electricity"}},
		{Service: "heating", SiteIDs: []string{"heating.electricity", "heating.natural_gas"}},
	}
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	if plain, err := epathSQLZoneCarrierDirectMonthlyIDs(frames, model, "Office"); err != nil || !plain {
		t.Fatalf("complete direct monthly observations: %v/%v", plain, err)
	}
	if plain, err := epathSQLZoneCarrierDirectMonthlyIDs(frames, model, "Unserved"); err != nil || plain {
		t.Fatalf("one owner's proof leaked to another Zone: %v/%v", plain, err)
	}
	model.Auxiliaries = []epathRealSQLAuxiliary{{Weight: "cooling_plus_heating"}}
	if plain, err := epathSQLZoneCarrierDirectMonthlyIDs(frames, model, "Office"); err != nil || plain {
		t.Fatalf("HVAC direct proof concealed an allocated auxiliary: %v/%v", plain, err)
	}
	model.Auxiliaries = nil
	delete(frames.DirectHVAC, epathSQLDirectHVACKey("Office", "heating", "electricity", 2))
	if plain, err := epathSQLZoneCarrierDirectMonthlyIDs(frames, model, "Office"); err != nil || plain {
		t.Fatalf("missing zero month masqueraded as observed direct zero: %v/%v", plain, err)
	}
}

func TestEnergyPathSQLNativeFanDirectCarrierIdentityRequiresCompleteOwnerCohort(t *testing.T) {
	path, model, _ := epathSQLNativeZoneEquipmentUnit(t)
	model.Services = []epathRealSQLService{
		{Service: "cooling", SiteIDs: []string{"cooling.electricity"}},
		{Service: "heating", SiteIDs: []string{"heating.electricity"}},
	}
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	for _, zone := range []string{"Office", "office"} {
		if plain, err := epathSQLZoneCarrierDirectMonthlyIDs(frames, model, zone); err != nil || !plain {
			t.Fatalf("native direct fan and complete direct HVAC, including eleven measured zero months: %v/%v", plain, err)
		}
	}
	if plain, err := epathSQLZoneCarrierDirectMonthlyIDs(frames, model, "Unserved"); err != nil || plain {
		t.Fatalf("native fan ownership must not grant another Zone a plain identity: %v/%v", plain, err)
	}
	for _, mutation := range []string{"missing fan zero month", "unknown fan zero month", "missing native source", "incomplete native source", "duplicate fan source", "broad meter mismatch", "shared fan pool", "shared auxiliary", "foreign native policy", "missing HVAC zero month"} {
		t.Run(mutation, func(t *testing.T) {
			changed := frames
			changed.DirectHVAC = map[string]epathSQLDirectHVACMonth{}
			for key, value := range frames.DirectHVAC {
				value.SourceIDs = append([]int(nil), value.SourceIDs...)
				changed.DirectHVAC[key] = value
			}
			changed.DirectHVACSourceIdentities = map[int]epathSQLDirectHVACSourceIdentity{}
			for key, value := range frames.DirectHVACSourceIdentities {
				changed.DirectHVACSourceIdentities[key] = value
			}
			changed.Site = map[string][]*epathSQLQuantity{}
			for key, values := range frames.Site {
				changed.Site[key] = append([]*epathSQLQuantity(nil), values...)
			}
			changedModel := model
			changedModel.Auxiliaries = append([]epathRealSQLAuxiliary(nil), model.Auxiliaries...)
			key := epathSQLDirectHVACKey("Office", "fans", "electricity", 2)
			switch mutation {
			case "missing fan zero month":
				delete(changed.DirectHVAC, key)
			case "unknown fan zero month":
				value := changed.DirectHVAC[key]
				value.Present = false
				changed.DirectHVAC[key] = value
			case "missing native source":
				delete(changed.DirectHVACSourceIdentities, 60)
			case "incomplete native source":
				identity := changed.DirectHVACSourceIdentities[60]
				identity.Source.MissingRows = 1
				changed.DirectHVACSourceIdentities[60] = identity
			case "duplicate fan source":
				value := changed.DirectHVAC[key]
				value.SourceIDs = append(value.SourceIDs, 60)
				changed.DirectHVAC[key] = value
			case "broad meter mismatch":
				value := *changed.Site["fans.electricity"][1]
				value.Value++
				changed.Site["fans.electricity"][1] = &value
			case "shared fan pool":
				changedModel.FanPools = []epathRealSQLFanPool{{SiteID: "fans.electricity"}}
			case "shared auxiliary":
				changedModel.Auxiliaries[0].Weight = "cooling_plus_heating"
			case "foreign native policy":
				changedModel.Auxiliaries[0].SiteID = "heating.electricity"
			case "missing HVAC zero month":
				delete(changed.DirectHVAC, epathSQLDirectHVACKey("Office", "heating", "electricity", 2))
			}
			if plain, err := epathSQLZoneCarrierDirectMonthlyIDs(changed, changedModel, "Office"); plain {
				t.Fatalf("%s cannot authorize a native direct carrier identity: %v", mutation, err)
			}
		})
	}
}
