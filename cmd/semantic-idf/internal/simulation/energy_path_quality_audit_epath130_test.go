package simulation

import "testing"

func TestEPATH130StagePlanEvidencePrecedesLegacyAggregateFallback(t *testing.T) {
	for _, aggregate := range []string{"not_requested", "not_applicable"} {
		stage := "not_applicable"
		if aggregate == stage {
			stage = "not_requested"
		}
		result := EnergyExplanationResult{Completeness: EnergyCompleteness{
			EnergyUse:          EnergyCompletenessLevel{Status: aggregate},
			SourceAvailability: []EnergySourceAvailabilityEntry{{Name: "Electricity:Facility", Level: "energy", Status: stage}},
		}}
		quality := BuildEnergyPathQuality(result, "annual")
		if quality.Carriers.Status != stage || quality.EndUses.Status != aggregate {
			t.Errorf("stage=%s aggregate=%s: stage-specific plan evidence was lost: %#v", stage, aggregate, quality)
		}
	}
}

func TestEPATH130ContextPlanEntriesCannotDisableSiteStages(t *testing.T) {
	for _, name := range []string{"Water:Facility", "ElectricityProduced:Facility", "ElectricityPurchased:Facility", "Unknown context resource"} {
		for _, status := range []string{"not_requested", "not_applicable"} {
			result := EnergyExplanationResult{Completeness: EnergyCompleteness{SourceAvailability: []EnergySourceAvailabilityEntry{{Name: name, Level: "energy", Status: status}}}}
			quality := BuildEnergyPathQuality(result, "annual")
			if quality.Carriers.Status != "unavailable" || quality.EndUses.Status != "unavailable" {
				t.Errorf("%s=%s disabled an unrelated site stage: %#v", name, status, quality)
			}
		}
	}
	for _, name := range []string{"", "not requested by current output plan"} {
		result := EnergyExplanationResult{Completeness: EnergyCompleteness{SourceAvailability: []EnergySourceAvailabilityEntry{{Name: name, Level: "energy", Status: "not_requested"}}}}
		quality := BuildEnergyPathQuality(result, "annual")
		if quality.Carriers.Status != "not_requested" || quality.EndUses.Status != "not_requested" {
			t.Errorf("whole-stage sentinel %q no longer applies to both site stages: %#v", name, quality)
		}
	}
}
