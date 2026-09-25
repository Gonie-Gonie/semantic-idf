package simulation

import (
	"reflect"
	"strings"
	"testing"
)

func TestCogenerationQualityConsumedResourcesAndContextRemainSeparate(t *testing.T) {
	result := EnergyExplanationResult{Completeness: EnergyCompleteness{SourceAvailability: []EnergySourceAvailabilityEntry{
		{Name: "Cooling:Electricity", Level: "energy", Status: "found"},
		{Name: "Electricity:Facility", Level: "energy", Status: "found"},
	}}}
	for _, tc := range []struct{ resource, carrier string }{
		{"Electricity", "electricity"}, {"NaturalGas", "natural_gas"},
		{"Propane", "propane"}, {"Diesel", "diesel"}, {"Gasoline", "gasoline"},
		{"Coal", "coal"}, {"FuelOilNo1", "fuel_oil_1"}, {"FuelOilNo2", "fuel_oil_2"},
		{"OtherFuel1", "other_fuel_1"}, {"OtherFuel2", "other_fuel_2"},
	} {
		name := "Cogeneration:" + tc.resource
		level, group := energyPathQualityEnergyGroup(name)
		if level != "end_use" || group != "energy.cogeneration_input|"+tc.carrier+"|cogeneration_input" {
			t.Fatalf("consumed resource %s inherited generator supply semantics: %q/%q", name, level, group)
		}
		if energyExplanationNameIsSupplyContext(name) {
			t.Fatalf("site-quality prefilter discarded consumed request %s", name)
		}
		result.Completeness.SourceAvailability = append(result.Completeness.SourceAvailability,
			EnergySourceAvailabilityEntry{Name: name, Level: "energy", Status: "found"},
			EnergySourceAvailabilityEntry{Name: strings.ToUpper(name), Level: "energy", Status: "found"})
	}
	for _, name := range []string{
		"Cogeneration:ElectricityProduced", "Cogeneration:ElectricityPurchased",
		"Cogeneration:ElectricitySurplusSold", "Cogeneration:ElectricityNet",
		"Cogeneration:EnergyTransfer", "Cogeneration:Water", "Cogeneration:Steam",
		"Cogeneration:Gas", "Cogeneration:UnreviewedResource", "Cogeneration:*",
		"Electricity:Cogeneration", "Cogeneration:Electricity:Zone:Z",
		"ElectricityProduced:Facility", "ElectricityPurchased:Facility", "ElectricitySurplusSold:Facility",
		"Electric Storage Discharge Energy", "Inverter Ancillary AC Electricity Energy",
	} {
		if level, group := energyPathQualityEnergyGroup(name); level != "" || group != "" {
			t.Fatalf("supply/context/ancillary name counted as another end use: %s -> %s/%s", name, level, group)
		}
		result.Completeness.SourceAvailability = append(result.Completeness.SourceAvailability,
			EnergySourceAvailabilityEntry{Name: name, Level: "energy", Status: "found"})
	}
	// Context-level entries and duplicated source IDs never add groups.
	result.Completeness.SourceAvailability = append(result.Completeness.SourceAvailability,
		EnergySourceAvailabilityEntry{Name: "Cogeneration:Electricity", Level: "context", Status: "found", SourceIDs: []string{"parent", "ancillary"}})
	before := append([]EnergySourceAvailabilityEntry(nil), result.Completeness.SourceAvailability...)
	quality := BuildEnergyPathQuality(result, "annual")
	if quality.EndUses.Found != 11 || quality.EndUses.Total != 11 || quality.EndUses.Status != "complete" || quality.Carriers.Found != 1 || quality.Carriers.Total != 1 {
		t.Fatalf("consumed10 plus Cooling group or separate Facility census changed: %+v / %+v", quality.EndUses, quality.Carriers)
	}
	if !reflect.DeepEqual(before, result.Completeness.SourceAvailability) {
		t.Fatal("quality rewrote availability provenance")
	}
}

func TestCogenerationQualityUsesExactRequestedParentAvailability(t *testing.T) {
	name := "Cogeneration:Electricity"
	for _, mode := range []string{"known zero M/H", "positive M/H", "monthly only requested", "missing monthly", "missing hourly", "unknown monthly", "duplicate monthly", "unrequested", "annual only", "ancillary only"} {
		t.Run(mode, func(t *testing.T) {
			monthly := cogenCoverageReviewSource("sql-rdd-61", name, "Monthly", 0, true)
			hourly := cogenCoverageReviewSource("sql-rdd-62", name, "Hourly", 0, true)
			sources := []EnergyDataSource{monthly, hourly}
			plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, OutputObjects: []PurposeOutputObject{
				cogenCoverageReviewRequest(name, "Monthly"), cogenCoverageReviewRequest(name, "Hourly"),
			}}
			wantFound, wantTotal := 0, 1
			switch mode {
			case "known zero M/H":
				wantFound = 1
			case "positive M/H":
				wantFound = 1
				sources[0].RawValue, sources[0].EffectiveValue = 2, 2
				sources[1].RawValue, sources[1].EffectiveValue = 2, 2
			case "monthly only requested":
				wantFound = 1
				plan.OutputObjects, sources = plan.OutputObjects[:1], sources[:1]
			case "missing monthly":
				sources = sources[1:]
			case "missing hourly":
				sources = sources[:1]
			case "unknown monthly":
				sources[0].observedValuePresence = 0
			case "duplicate monthly":
				duplicate := monthly
				duplicate.ID = "sql-rdd-63"
				sources = append(sources, duplicate)
			case "unrequested":
				wantTotal = 0
				plan.OutputObjects = []PurposeOutputObject{cogenCoverageReviewRequest("Electricity:Facility", "Monthly")}
			case "annual only":
				sources = []EnergyDataSource{{ID: "table", SourceType: "sql_tabular", Name: name, ReportingFrequency: "Annual", SourceUnit: "kWh", observedValuePresence: energySourceObservedRaw | energySourceObservedEffective}}
			case "ancillary only":
				sources = []EnergyDataSource{{ID: "ancillary", SourceType: "sql_report_data", Name: "Inverter Ancillary AC Electricity Energy", KeyValue: "INV", SourceUnit: "J", ReportingFrequency: "Monthly", observedValuePresence: energySourceObservedRaw | energySourceObservedEffective}}
			}
			before := append([]EnergyDataSource(nil), sources...)
			completeness := buildEnergyExplanationCompleteness(nil, sources, plan, 0)
			applyEnergyPathCogenerationAvailability(&completeness, plan, energyPathCogenerationReadResult{Enabled: true, SourceSnapshots: sources})
			quality := BuildEnergyPathQuality(EnergyExplanationResult{Sources: sources, Completeness: completeness}, "annual")
			if quality.EndUses.Found != wantFound || quality.EndUses.Total != wantTotal {
				t.Fatalf("quality substituted another frequency/source for requested native parent: %+v", quality.EndUses)
			}
			if wantTotal == 1 && (quality.EndUses.Status == "complete") != (wantFound == 1) {
				t.Fatalf("requested missing and known zero were conflated: %+v", quality.EndUses)
			}
			if !reflect.DeepEqual(before, sources) {
				t.Fatal("quality mutated original native knownness/scalars")
			}
			if wantFound == 1 {
				for _, entry := range completeness.SourceAvailability {
					if entry.Name == name && (len(entry.SourceIDs) != len(sources) || entry.SourceIDs[0] != sources[0].ID) {
						t.Fatal("found parent group lost exact source trace")
					}
				}
			}
		})
	}
}
