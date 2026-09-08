package simulation

import "testing"

func TestEnergyPathRealSQLRadiantReconciliationRetainsContextWithoutThermalCell(t *testing.T) {
	path, model, original := epathSQLRadiantSurfaceContextUnitFixture(t)
	frames := epathSQLRadiantSurfaceContextUnitFrames(t, path, model, original)
	var checks epathSQLModelChecks
	if err := epathSQLModelThermalReconciliationChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) == 0 {
		t.Fatal("context-only active surface lost ordinary thermal reconciliation")
	}
	for _, check := range checks.Rows {
		if check.Reconciliation == nil || check.Quantity == nil {
			t.Fatal("context exclusion bypassed a numeric reconciliation obligation")
		}
	}
}

func TestEnergyPathRealSQLRadiantReconciliationRejectsUnprovedContext(t *testing.T) {
	for _, mutation := range []string{"missing typed context", "changed source", "pressure reuse", "fabricated cell", "ordinary missing context"} {
		t.Run(mutation, func(t *testing.T) {
			path, model, original := epathSQLRadiantSurfaceContextUnitFixture(t)
			frames := epathSQLRadiantSurfaceContextUnitFrames(t, path, model, original)
			index := len(model.Families) - 1
			switch mutation {
			case "missing typed context":
				delete(frames.RadiantSurfaceContextIdentities, 16)
			case "changed source":
				source := frames.SourceIdentities[16]
				source.KeyValue = "Other Zone"
				frames.SourceIdentities[16] = source
			case "pressure reuse":
				model.Families[index].Role = "pressure"
			case "fabricated cell":
				copy := *frames.Cells[epathSQLKey("Office", "surface:surface.ground_floors", 1)]
				copy.Family = model.Families[index].ID
				copy.SourceIDs = nil
				frames.Cells[epathSQLKey("Office", copy.Family, 1)] = &copy
			case "ordinary missing context":
				family := model.Families[index]
				family.ID = "ordinary.context"
				family.Terms = []epathRealSQLTerm{{Sign: 1, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Unobserved Context", Unit: "W"}}}}}
				model.Families = append(model.Families, family)
			}
			var checks epathSQLModelChecks
			if err := epathSQLModelThermalReconciliationChecks(frames, model, &checks); err == nil {
				t.Fatal("unproved/misclassified context replaced a required thermal family")
			}
		})
	}
}
