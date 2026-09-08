package simulation

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Original EAST ZONE / April observations from the preserved 25.1
// RadLoTempCFloHeatCool capture: Monthly Outdoor Air Transfer [W] (RDD402),
// Infiltration Sensible Heat Loss [J] (418), and Heat Gain [J] (419).
// The native observations, not a candidate graph, define this cancellation.
func outdoorPrecisionOriginalValues() (outdoor, loss, gain float64) {
	return -162.46568234169152 * 720 / 1000,
		425943048.9830728 / 3600000,
		4832000.353409533 / 3600000
}

func outdoorPrecisionFixture(t *testing.T, outdoorDelta float64) ([]energyExplanationSeries, []EnergyDataSource) {
	t.Helper()
	outdoor, loss, gain := outdoorPrecisionOriginalValues()
	values := []float64{outdoor + outdoorDelta, loss, gain}
	names := []string{
		"Zone Air Heat Balance Outdoor Air Transfer Rate",
		"Zone Infiltration Sensible Heat Loss Energy",
		"Zone Infiltration Sensible Heat Gain Energy",
	}
	ids := []string{"sql-rdd-402", "sql-rdd-418", "sql-rdd-419"}
	series := make([]energyExplanationSeries, 0, len(values))
	sources := make([]EnergyDataSource, 0, len(values))
	for i, value := range values {
		item := reviewEnergyHeatSeries(t, "EAST ZONE", names[i], roundedEnergyNumber(value), ids[i])
		item.Monthly = map[int]float64{4: roundedEnergyNumber(value)}
		item.RawMonthly = cloneEnergyExplanationPeriodValues(item.Monthly)
		item.RawTotal = item.Total
		item.EffectiveMultiplier = 1
		item.MultiplierApplication = energyMultiplierAlreadyModelTotal
		item.multiplierApplied = true
		item.driverMonthlyShadow = &energyDriverMonthlyShadow{values: map[int]float64{4: value}, effective: true}
		series = append(series, item)
		unit := "J"
		if i == 0 {
			unit = "W"
		}
		sources = append(sources, EnergyDataSource{
			ID: ids[i], Name: names[i], KeyValue: "EAST ZONE", SourceType: "sql_report_data",
			ReportingFrequency: "Monthly", SourceUnit: unit, Units: "kWh", NormalizedUnit: "kWh",
			RawValue: roundedEnergyNumber(value), EffectiveValue: roundedEnergyNumber(value),
			EffectiveMultiplier: 1, MultiplierApplication: energyMultiplierAlreadyModelTotal,
		})
	}
	prepared, sources, _ := prepareEnergyDriverSeries(series, sources, energyDriverBuildContext{Enabled: true})
	return prepared, sources
}

func TestEnergyDriverOutdoorPrecisionOriginalEastDoesNotInventMechanicalVentilation(t *testing.T) {
	outdoor, loss, gain := outdoorPrecisionOriginalValues()
	exact := outdoor - (gain - loss)
	rounded := roundedEnergyNumber(outdoor) - (roundedEnergyNumber(gain) - roundedEnergyNumber(loss))
	if math.Abs(exact) > 1e-10 || math.Abs(rounded-.001) > 1e-10 {
		t.Fatalf("literal source regression changed: exact %.17g / rounded %.17g", exact, rounded)
	}
	series, sources := outdoorPrecisionFixture(t, 0)
	beforeSeries, beforeSources := outdoorPrecisionFixture(t, 0)
	out, after, warnings := appendEnergyDriverVentilationFallbacks(series, sources, nil)
	if len(out) != len(series) || len(after) != len(sources) || len(warnings) != 0 {
		t.Fatalf("separately rounded source totals invented ventilation: series=%d/%d sources=%d/%d warnings=%+v", len(out), len(series), len(after), len(sources), warnings)
	}
	if !reflect.DeepEqual(series, beforeSeries) || !reflect.DeepEqual(sources, beforeSources) {
		t.Fatal("fallback calculation mutated original source values or private monthly evidence")
	}
	// Both gains and losses remain real observations; cancellation only governs
	// the derived aggregate residual, never the independent pressure sources.
	if out[1].driverMonthlyShadow.values[4] != loss || out[2].driverMonthlyShadow.values[4] != gain {
		t.Fatal("source-level gain/loss observations were netted or snapped")
	}

	load := reviewEnergyLoadSeries("EAST ZONE", "cooling", 10, "load-east")
	load.Monthly = map[int]float64{4: 10}
	series = append(series, load)
	sources = append(sources, EnergyDataSource{ID: "load-east", Name: load.sourceName, KeyValue: "EAST ZONE"})
	doc := parsePurposePlanFixture(t, "Zone,EAST ZONE,0,0,0,0,1,1;")
	result := UpgradeEnergyExplanationV1(buildEnergyExplanationResultWithDriverContext(series, sources, &PurposeRunPlan{}, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)))
	for _, period := range []string{"M4", "annual"} {
		nodes, _ := epath080Graph(t, result, "EAST ZONE", period)
		if node := epath080NodeByID(nodes, epath080DriverNodeID(energyDriverCategoryMechanicalVentilation, "cooling", "EAST ZONE")); node != nil {
			t.Fatalf("%s Zone acquired a mechanical-ventilation branch: %+v", period, node)
		}
		building := result.Nodes
		if period != "annual" {
			p := energyExplanationPeriodByID(result.Periods, period)
			if p == nil {
				t.Fatal("original April observations lost their Building period")
			}
			building = p.Nodes
		}
		if node := epath080NodeByID(building, epath080DriverNodeID(energyDriverCategoryMechanicalVentilation, "cooling", "building")); node != nil {
			t.Fatalf("%s Building acquired a mechanical-ventilation branch: %+v", period, node)
		}
	}
}

func TestEnergyDriverOutdoorPrecisionPreservesGenuineResidualAndProvenance(t *testing.T) {
	for _, delta := range []float64{1.25, -.75, .0006} {
		t.Run(fmt.Sprintf("delta_%g", delta), func(t *testing.T) {
			series, sources := outdoorPrecisionFixture(t, delta)
			expected := series[0].driverMonthlyShadow.values[4] - (-series[1].driverMonthlyShadow.values[4] + series[2].driverMonthlyShadow.values[4])
			out, after, warnings := appendEnergyDriverVentilationFallbacks(series, sources, nil)
			if len(out) != len(series)+1 || len(after) != len(sources)+1 || !reviewHasWarning(warnings, "energy_driver_ventilation_fallback") {
				t.Fatal("genuine outdoor-minus-infiltration fallback disappeared")
			}
			item := out[len(out)-1]
			if item.DriverCategory != energyDriverCategoryMechanicalVentilation || item.Monthly[4] != roundedEnergyNumber(expected) || item.Total != roundedEnergyNumber(expected) || item.driverMonthlyShadow.values[4] != expected {
				t.Fatalf("genuine residual changed: expected %.17g / %+v", expected, item)
			}
			source := after[len(after)-1]
			if len(source.InputSourceIDs) != 3 {
				t.Fatalf("genuine fallback source union is incomplete or duplicated: %+v", source.InputSourceIDs)
			}
			required := map[string]bool{"sql-rdd-402": true, "sql-rdd-418": true, "sql-rdd-419": true}
			for _, id := range source.InputSourceIDs {
				if !required[id] {
					t.Fatalf("genuine fallback has duplicate or foreign source %s", id)
				}
				delete(required, id)
			}
			if len(required) != 0 || source.Formula != "signed outdoor-air aggregate - signed infiltration" || source.RawValue != roundedEnergyNumber(expected) || source.EffectiveValue != roundedEnergyNumber(expected) {
				t.Fatalf("genuine fallback lost exact source union or scalar contract: %+v", source)
			}
		})
	}
}

func TestEnergyDriverOutdoorPrecisionDoesNotHealMissingOrInvalidMonthlyEvidence(t *testing.T) {
	for _, mutation := range []string{"missing month", "invalid source", "one untracked source", "nonmatching month"} {
		t.Run(mutation, func(t *testing.T) {
			series, sources := outdoorPrecisionFixture(t, 1.25)
			switch mutation {
			case "missing month":
				delete(series[1].driverMonthlyShadow.values, 4)
			case "invalid source":
				series[1].driverMonthlyShadow.invalid = true
			case "one untracked source":
				series[1].driverMonthlyShadow = nil
			case "nonmatching month":
				series[1].driverMonthlyShadow.values = map[int]float64{5: 1}
			}
			out, after, warnings := appendEnergyDriverVentilationFallbacks(series, sources, nil)
			if len(out) != len(series) || len(after) != len(sources) || len(warnings) != 0 {
				t.Fatal("unknown monthly subtraction was replaced with rounded data or observed zero")
			}
			if !reflect.DeepEqual(after, sources) {
				t.Fatal("incomplete source identity disappeared")
			}
		})
	}
}

func TestEnergyDriverOutdoorPrecisionKeepsUntrackedLegacyFallback(t *testing.T) {
	series, sources := outdoorPrecisionFixture(t, 1.25)
	for i := range series {
		series[i].driverMonthlyShadow = nil
	}
	expected := series[0].Monthly[4] - (-series[1].Monthly[4] + series[2].Monthly[4])
	out, _, _ := appendEnergyDriverVentilationFallbacks(series, sources, nil)
	if len(out) != len(series)+1 || out[len(out)-1].Monthly[4] != roundedEnergyNumber(expected) || out[len(out)-1].driverMonthlyShadow != nil {
		t.Fatal("legacy without tracked Monthly precision changed numeric authority")
	}
}
