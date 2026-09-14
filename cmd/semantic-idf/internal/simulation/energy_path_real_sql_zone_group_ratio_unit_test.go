package simulation

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// This regression binds the reviewed taxonomy, not candidate measurements.
// Native electric convective baseboards consume Q / efficiency; the existing
// graph calls electric heating Load / site energy, not an inferred heat-pump COP.
func TestEnergyPathRealSQLZoneGroupResistanceRatioRecipe(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "oracles", "zone-group-25-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recipe epathRealOracleRecipe
	if err := json.Unmarshal(content, &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.SQLModel == nil || len(recipe.SQLModel.Services) != 2 {
		t.Fatal("exact cooling and resistance-heating declarations required")
	}
	seen := map[string]bool{}
	for _, service := range recipe.SQLModel.Services {
		want, found := map[string]string{"cooling": "coefficient_of_performance", "heating": "load_to_site_energy"}[service.Service]
		if !found || seen[service.Service] || service.RatioKind != want || service.FallbackRatioKind != want {
			t.Fatalf("reviewed service taxonomy changed: %+v", service)
		}
		seen[service.Service] = true
	}
}

// Hand quantities only. Both source observations and graph endpoints are
// explicitly constructed here; no production classifier or captured candidate
// supplies the expected RatioKind, endpoint interval, or numerical value.
func epathSQLResistanceRatioHandFixture(scope, period, basis string, from, to epathSQLQuantity, displayedFrom, displayedTo float64) (PurposeResultBundle, epathSQLModelCheck) {
	zone := ""
	if scope == "zone" {
		zone = "Office"
	}
	nodes := []EnergyExplanationNode{
		{ID: "resistance.load", Level: "load", ServiceKind: "heating", ZoneName: zone, Period: period, ScaleDomain: "thermal", Unit: "kWh", Value: displayedFrom},
		{ID: "resistance.use", Level: "end_use", EndUse: "heating", ZoneName: zone, Period: period, ScaleDomain: "site", Unit: "kWh", Value: displayedTo},
	}
	link := EnergyPathLink{ID: "resistance.conversion", FromID: nodes[0].ID, ToID: nodes[1].ID, Relation: "load_to_end_use", ServiceKind: "heating", ZoneName: zone, Period: period, Basis: basis, FromValue: displayedFrom, ToValue: displayedTo, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"hand.thermal", "hand.electricity"}}
	if displayedFrom > 0 && displayedTo > 0 {
		link.RatioKind, link.RatioLabel = "load_to_site_energy", "Load / site energy"
		link.Ratio = math.Round((displayedFrom/displayedTo)*1000) / 1000
	}
	links := []EnergyPathLink{link}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{
		Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
		Sources: []EnergyDataSource{
			{ID: "hand.thermal", Name: "Baseboard Total Heating Energy", RawValue: from.Value, EffectiveValue: from.Value},
			{ID: "hand.electricity", Name: "Baseboard Electricity Energy", RawValue: to.Value, EffectiveValue: to.Value},
		},
	}}
	if scope == "zone" {
		result := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: zone, AggregationBasis: "model_total"}}
		if period == "annual" {
			result.Nodes, result.Links = nodes, links
		} else {
			result.Periods = []EnergyPeriod{{ID: period, Nodes: nodes, Links: links}}
		}
		bundle.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{result}
	} else if period == "annual" {
		bundle.EnergyExplanation.Nodes, bundle.EnergyExplanation.Links = nodes, links
	} else {
		bundle.EnergyExplanation.Periods = []EnergyPeriod{{ID: period, Nodes: nodes, Links: links}}
	}
	item := epathRealOracleMetricRecipe{Group: "ratios", Scope: scope, Zone: zone, Period: period, Unit: "ratio", Target: epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: "heating", Basis: basis, FromUnit: "kWh", ToUnit: "kWh", RatioKind: "load_to_site_energy", Aggregate: "sum"}}
	check := epathSQLModelCheck{Item: item, Conversion: &epathSQLConversionProof{From: from, To: to}}
	if from.Value > 0 && to.Value > 0 {
		check.Quantity = &epathSQLQuantity{Value: from.Value / to.Value}
	}
	return bundle, check
}

func epathSQLResistanceRatioHandLinks(bundle *PurposeResultBundle, scope, period string) *[]EnergyPathLink {
	if scope == "zone" {
		if period == "annual" {
			return &bundle.EnergyExplanation.ZoneResults[0].Links
		}
		return &bundle.EnergyExplanation.ZoneResults[0].Periods[0].Links
	}
	if period == "annual" {
		return &bundle.EnergyExplanation.Links
	}
	return &bundle.EnergyExplanation.Periods[0].Links
}

func TestEnergyPathRealSQLZoneGroupResistanceRatioExactKind(t *testing.T) {
	for _, scope := range []string{"building", "zone"} {
		for _, period := range []string{"annual", "M1"} {
			bases := []string{"service_path_allocation", "zone_load_allocation"}
			if scope == "zone" {
				bases = []string{"direct_zone_energy"}
			}
			for _, basis := range bases {
				t.Run(scope+"/"+period+"/"+basis, func(t *testing.T) {
					bundle, check := epathSQLResistanceRatioHandFixture(scope, period, basis, epathSQLQuantity{Value: 95}, epathSQLQuantity{Value: 100}, 95, 100)
					before := *check.Conversion
					if err := epathCheckSQLModelConversion(bundle, check); err != nil {
						t.Fatalf("hand 95 thermal / 100 electricity pair: %v", err)
					}
					wrongDeclaration := check
					wrongDeclaration.Item.Target.RatioKind = "coefficient_of_performance"
					if err := epathCheckSQLModelConversion(bundle, wrongDeclaration); err == nil {
						t.Fatal("same numbers accepted the physically incorrect Heating COP declaration")
					}
					links := epathSQLResistanceRatioHandLinks(&bundle, scope, period)
					(*links)[0].RatioKind = "coefficient_of_performance"
					if err := epathCheckSQLModelConversion(bundle, check); err == nil {
						t.Fatal("correct declaration accepted a mislabeled graph COP")
					}
					if !reflect.DeepEqual(before, *check.Conversion) || bundle.EnergyExplanation.Sources[0].RawValue != 95 || bundle.EnergyExplanation.Sources[1].RawValue != 100 {
						t.Fatal("ratio validation changed independent source quantities")
					}
				})
			}
		}
	}
}

func TestEnergyPathRealSQLZoneGroupResistanceRatioObservedZero(t *testing.T) {
	for _, scope := range []string{"building", "zone"} {
		t.Run(scope, func(t *testing.T) {
			basis := "service_path_allocation"
			if scope == "zone" {
				basis = "direct_zone_energy"
			}
			bundle, check := epathSQLResistanceRatioHandFixture(scope, "M1", basis, epathSQLQuantity{}, epathSQLQuantity{}, 0, 0)
			if err := epathCheckSQLModelConversion(bundle, check); err != nil {
				t.Fatalf("known-zero observations with no ratio metadata: %v", err)
			}
			links := epathSQLResistanceRatioHandLinks(&bundle, scope, "M1")
			original := (*links)[0]
			(*links)[0].RatioKind = "load_to_site_energy"
			if err := epathCheckSQLModelConversion(bundle, check); err == nil {
				t.Fatal("zero pair retained a fabricated ratio kind")
			}
			(*links)[0] = original
			(*links)[0].Ratio = .95
			if err := epathCheckSQLModelConversion(bundle, check); err == nil {
				t.Fatal("zero pair retained a fabricated 0.95 ratio")
			}
			*links = nil
			if err := epathCheckSQLModelConversion(bundle, check); err != nil {
				t.Fatalf("whole observed-zero conversion may be absent: %v", err)
			}
			check.Conversion = nil
			if err := epathCheckSQLModelConversion(bundle, check); err == nil {
				t.Fatal("missing independent evidence became an observed zero")
			}
		})
	}
}

func TestEnergyPathRealSQLZoneGroupResistanceRatioTinyBoundedPair(t *testing.T) {
	// Hand native energy .002 kWh at efficiency .95 gives thermal .0019 kWh.
	// The existing three-decimal endpoint grid displays .002/.002 = 1, not
	// the raw quotient .95. The existing half-step interval is not widened.
	for _, scope := range []string{"building", "zone"} {
		t.Run(scope, func(t *testing.T) {
			basis := "service_path_allocation"
			if scope == "zone" {
				basis = "direct_zone_energy"
			}
			from, to := epathSQLQuantity{Value: .0019, Error: .0005}, epathSQLQuantity{Value: .002, Error: .0005}
			bundle, check := epathSQLResistanceRatioHandFixture(scope, "M1", basis, from, to, .002, .002)
			if err := epathCheckSQLModelConversion(bundle, check); err != nil {
				t.Fatalf("independently bounded positive display pair: %v", err)
			}
			links := epathSQLResistanceRatioHandLinks(&bundle, scope, "M1")
			(*links)[0].Ratio = .95
			if err := epathCheckSQLModelConversion(bundle, check); err == nil {
				t.Fatal("raw efficiency replaced the actual displayed paired quotient")
			}
			(*links)[0].Ratio = 2
			(*links)[0].FromValue = .004
			if err := epathCheckSQLModelConversion(bundle, check); err == nil {
				t.Fatal("changed endpoint escaped the original half-step bound")
			}
			*links = nil
			if err := epathCheckSQLModelConversion(bundle, check); err == nil {
				t.Fatal("two strictly positive bounded endpoints silently disappeared")
			}
			// A different hand observation really can round both endpoints to
			// zero. Absence is allowed without changing the positive raw source.
			bundle, check = epathSQLResistanceRatioHandFixture(scope, "M1", basis, epathSQLBounded(.00038, 0, .00088), epathSQLBounded(.0004, 0, .0009), 0, 0)
			if err := epathCheckSQLModelConversion(bundle, check); err != nil {
				t.Fatalf("bounded zero presentation: %v", err)
			}
			*epathSQLResistanceRatioHandLinks(&bundle, scope, "M1") = nil
			if err := epathCheckSQLModelConversion(bundle, check); err != nil {
				t.Fatalf("whole bounded-zero presentation may be absent: %v", err)
			}
			if bundle.EnergyExplanation.Sources[0].RawValue != .00038 || bundle.EnergyExplanation.Sources[1].RawValue != .0004 || check.Conversion.From.Value != .00038 || check.Conversion.To.Value != .0004 {
				t.Fatal("presentation zero erased a positive native observation")
			}
		})
	}
}
