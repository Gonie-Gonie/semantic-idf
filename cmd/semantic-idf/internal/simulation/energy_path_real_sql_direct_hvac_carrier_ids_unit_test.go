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
