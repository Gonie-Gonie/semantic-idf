package simulation

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathSQLAnnualBuildingUnitFrames() (epathSQLFrames, epathRealSQLModel) {
	frames := epathSQLFrames{
		Zones: map[string]epathSQLZone{"a": {Name: "A", Multiplier: 1}, "b": {Name: "B", Multiplier: 1}, "plenum": {Name: "Plenum", Multiplier: 1}},
		Loads: map[string]epathSQLQuantity{}, Site: map[string][]*epathSQLQuantity{},
		SiteAnnual: map[string]epathSQLTabularObservation{
			"district.c": {Selector: epathSQLTabularUnitSelector(), TabularDataIndex: 1, RawText: "300.00", RawValue: 300, Quantity: epathSQLBounded(300, 299.995, 300.005), Weather: epathRealSQLWeather{Year: 2017, EnvironmentIndex: 1, EnvironmentName: "WEATHER", TimeRows: 12, FirstMonth: 1, LastMonth: 12, Months: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, CoverageBasis: "monthly_intervals_and_cumulative_simulation_days"}},
			"district.h": {Selector: epathRealSQLTabularSelector{ReportName: "AnnualBuildingUtilityPerformanceSummary", ReportForString: "Entire Facility", TableName: "End Uses", RowName: "Heating", ColumnName: "District Heating Water", Unit: "kWh", DecimalPlaces: 2}, TabularDataIndex: 2, RawText: "120.00", RawValue: 120, Quantity: epathSQLBounded(120, 119.995, 120.005), Weather: epathRealSQLWeather{Year: 2017, EnvironmentIndex: 1, EnvironmentName: "WEATHER", TimeRows: 12, FirstMonth: 1, LastMonth: 12, Months: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, CoverageBasis: "monthly_intervals_and_cumulative_simulation_days"}},
		},
	}
	model := epathRealSQLModel{}
	for _, service := range []string{"cooling", "heating"} {
		id := "district.c"
		if service == "heating" {
			id = "district.h"
		}
		model.Services = append(model.Services, epathRealSQLService{Service: service, SiteIDs: []string{id}, ServedZones: []string{"A", "B"}, Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "load_to_purchased_energy", FallbackRatioKind: "load_to_purchased_energy", ReconciliationID: "allocation." + service + ".annual"})
		for zone := range frames.Zones {
			for month := 1; month <= 12; month++ {
				value := 0.0
				if zone == "plenum" {
					value = 500 // Real but unserved; never an allocation denominator.
				}
				frames.Loads[epathSQLKey(zone, service, month)] = epathSQLQuantity{Value: value}
			}
		}
	}
	for key, value := range map[string]float64{
		epathSQLKey("a", "cooling", 1): 10, epathSQLKey("a", "cooling", 2): 90,
		epathSQLKey("b", "cooling", 1): 90, epathSQLKey("b", "cooling", 2): 10,
		epathSQLKey("a", "heating", 1): 30, epathSQLKey("b", "heating", 1): 10,
	} {
		frames.Loads[key] = epathSQLQuantity{Value: value}
	}
	return frames, model
}

// Hand-authored annual observations and candidate endpoints. No compiled
// expected quantities or production allocator is used to create this candidate.
func epathSQLAnnualBuildingUnitBundle() PurposeResultBundle {
	result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}
	for _, item := range []struct {
		service    string
		load, site float64
	}{{"cooling", 200, 300}, {"heating", 40, 120}} {
		loadID, useID := "load."+item.service, "use."+item.service
		result.Nodes = append(result.Nodes,
			EnergyExplanationNode{ID: loadID, Level: "load", ServiceKind: item.service, ScaleDomain: "thermal", Unit: "kWh", Period: "annual", Value: item.load},
			EnergyExplanationNode{ID: useID, Level: "end_use", EndUse: item.service, ScaleDomain: "site", Unit: "kWh", Period: "annual", Value: item.site})
		result.Sources = append(result.Sources, EnergyDataSource{ID: "original." + item.service, SourceType: "sql_tabular", IsMeter: true, SourceUnit: "kWh", NormalizedUnit: "kWh", ReportingFrequency: "Annual", RawValue: item.site, EffectiveValue: item.site})
		result.Links = append(result.Links, EnergyPathLink{ID: "convert." + item.service, FromID: loadID, ToID: useID, Relation: "load_to_end_use", ServiceKind: item.service, Basis: "service_path_allocation", Period: "annual", FromValue: item.load, ToValue: item.site, FromUnit: "kWh", ToUnit: "kWh", Ratio: item.load / item.site, RatioKind: "load_to_purchased_energy", SourceIDs: []string{"original." + item.service}})
		result.Reconciliation = append(result.Reconciliation, EnergyReconciliation{ID: "allocation." + item.service + ".annual", Level: "allocation", Period: "annual", Unit: "kWh", ExpectedValue: item.site, AllocatedValue: item.site})
	}
	for month := 1; month <= 12; month++ {
		result.Periods = append(result.Periods, EnergyPeriod{ID: fmt.Sprintf("M%d", month), Kind: "month"})
	}
	return PurposeResultBundle{EnergyExplanation: result}
}

func TestEnergyPathRealSQLAnnualBuildingServiceExactTemporalProof(t *testing.T) {
	frames, model := epathSQLAnnualBuildingUnitFrames()
	original := frames.SiteAnnual["district.c"]
	var checks epathSQLModelChecks
	if err := epathSQLModelServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 156 {
		t.Fatalf("two services require all13 periods, both branches and all4 accounting fields: %d", len(checks.Rows))
	}
	monthly := 0
	for _, check := range checks.Rows {
		if check.Item.Period != "annual" {
			monthly++
			if check.Quantity != nil || check.Want.Value != nil || check.Want.Status != "unavailable" || !check.AnnualServiceAbsent || check.Conversion != nil || check.Allocation != nil {
				t.Fatalf("annual-only consumption invented monthly evidence: %+v", check.Want)
			}
		}
	}
	if monthly != 144 || len(frames.Site) != 0 || !reflect.DeepEqual(original, frames.SiteAnnual["district.c"]) {
		t.Fatal("temporal proof distributed or mutated the original annual cell")
	}
	bundle := epathSQLAnnualBuildingUnitBundle()
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 0 {
		t.Fatalf("independent annual values and explicit monthly absence rejected: %+v", failures)
	}
	for _, check := range checks.Rows {
		if check.Conversion != nil && check.Item.Target.Service == "cooling" && check.Item.Target.Basis == "service_path_allocation" {
			low, high := check.Conversion.To.bounds()
			if check.Conversion.From.Value != 200 || math.Abs(low-299.995) > 1e-10 || math.Abs(high-300.005) > 1e-10 {
				t.Fatal("served load or annual cell's0.01 kWh display precision changed")
			}
		}
	}
}

func TestEnergyPathRealSQLAnnualBuildingServiceRejectFabrication(t *testing.T) {
	frames, model := epathSQLAnnualBuildingUnitFrames()
	var checks epathSQLModelChecks
	if err := epathSQLModelServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PurposeResultBundle){
		"monthly positive conversion": func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[0].Nodes = []EnergyExplanationNode{{ID: "load", Level: "load", ServiceKind: "cooling", Unit: "kWh", ScaleDomain: "thermal", Period: "M1"}, {ID: "use", Level: "end_use", EndUse: "cooling", Unit: "kWh", ScaleDomain: "site", Period: "M1"}}
			b.EnergyExplanation.Periods[0].Links = []EnergyPathLink{{ID: "invented", FromID: "load", ToID: "use", Relation: "load_to_end_use", ServiceKind: "cooling", Basis: "service_path_allocation", FromValue: 10, ToValue: 25, Ratio: .4, RatioKind: "load_to_purchased_energy", FromUnit: "kWh", ToUnit: "kWh", Period: "M1"}}
		},
		"monthly zero allocation": func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[0].Reconciliation = []EnergyReconciliation{{ID: "allocation.cooling.m1", Level: "allocation", Period: "M1", Unit: "kWh"}}
		},
		"annual unserved plenum": func(b *PurposeResultBundle) {
			b.EnergyExplanation.Links[0].FromValue = 6200
			b.EnergyExplanation.Links[0].Ratio = 6200.0 / 300
		},
		"annual missing allocation": func(b *PurposeResultBundle) { b.EnergyExplanation.Reconciliation = nil },
		"annual false COP":          func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].RatioKind = "coefficient_of_performance" },
		"annual divided site": func(b *PurposeResultBundle) {
			b.EnergyExplanation.Links[0].ToValue = 25
			b.EnergyExplanation.Links[0].Ratio = 8
		},
	} {
		t.Run(name, func(t *testing.T) {
			bundle := epathSQLAnnualBuildingUnitBundle()
			mutate(&bundle)
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) == 0 {
				t.Fatal("fabricated temporal/served-source evidence accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLAnnualBuildingServiceRejectUnknownTemporalInputs(t *testing.T) {
	for name, mutate := range map[string]func(*epathSQLFrames, *epathRealSQLModel){
		"missing load month": func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.Loads, epathSQLKey("a", "cooling", 1)) },
		"duplicate site": func(_ *epathSQLFrames, m *epathRealSQLModel) {
			m.Services[0].SiteIDs = []string{"district.c", "district.c"}
		},
		"unknown site": func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Services[0].SiteIDs = []string{"absent"} },
		"mixed temporal service": func(f *epathSQLFrames, m *epathRealSQLModel) {
			for month := 1; month <= 12; month++ {
				f.Site["monthly"] = append(f.Site["monthly"], &epathSQLQuantity{Value: 1})
			}
			m.Services[0].SiteIDs = append(m.Services[0].SiteIDs, "monthly")
		},
	} {
		t.Run(name, func(t *testing.T) {
			frames, model := epathSQLAnnualBuildingUnitFrames()
			mutate(&frames, &model)
			if err := epathSQLModelServiceChecks(frames, model, &epathSQLModelChecks{}); err == nil {
				t.Fatal("unknown/mixed annual evidence became a fabricated observation")
			}
		})
	}
	frames, model := epathSQLAnnualBuildingUnitFrames()
	for zone := range frames.Zones {
		if zone != "plenum" {
			for month := 1; month <= 12; month++ {
				frames.Loads[epathSQLKey(zone, "heating", month)] = epathSQLQuantity{}
			}
		}
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	for _, check := range checks.Rows {
		if check.Item.Period == "annual" && strings.HasSuffix(check.Want.Key, "heating/unassignedValue") && (check.Quantity == nil || check.Quantity.Value != 120) {
			t.Fatal("passive plenum load expanded annual served membership")
		}
	}
	frames.Loads[epathSQLKey("a", "heating", 1)] = epathSQLBounded(0, 0, .001)
	if err := epathSQLModelServiceChecks(frames, model, &epathSQLModelChecks{}); err == nil || !strings.Contains(err.Error(), "uncertain annual served denominator") {
		t.Fatalf("uncertain zero denominator was treated as a known unassigned allocation: %v", err)
	}
}

func TestEnergyPathRealSQLAnnualBuildingServiceAbsenceRejectsZeroRecords(t *testing.T) {
	frames, model := epathSQLAnnualBuildingUnitFrames()
	var checks epathSQLModelChecks
	if err := epathSQLModelServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	for _, collection := range []string{"links", "reconciliation"} {
		t.Run(collection, func(t *testing.T) {
			var selected *epathSQLModelCheck
			for index := range checks.Rows {
				check := &checks.Rows[index]
				if check.AnnualServiceAbsent && check.Item.Period == "M1" && check.Item.Target.Collection == collection {
					selected = check
					break
				}
			}
			if selected == nil {
				t.Fatal("required temporal absence selector missing")
			}
			bundle := epathSQLAnnualBuildingUnitBundle()
			if err := epathCheckSQLAnnualServiceAbsent(bundle, *selected); err != nil {
				t.Fatal(err)
			}
			period := &bundle.EnergyExplanation.Periods[0]
			if collection == "links" {
				service := selected.Item.Target.Service
				period.Nodes = []EnergyExplanationNode{{ID: "load", Level: "load", ServiceKind: service, Unit: "kWh", ScaleDomain: "thermal", Period: "M1"}, {ID: "use", Level: "end_use", EndUse: service, Unit: "kWh", ScaleDomain: "site", Period: "M1"}}
				period.Links = []EnergyPathLink{{ID: "fabricated.zero", FromID: "load", ToID: "use", Relation: "load_to_end_use", ServiceKind: service, Basis: "service_path_allocation", FromUnit: "kWh", ToUnit: "kWh", Period: "M1", SourceIDs: []string{"original." + service}}}
			} else {
				period.Reconciliation = []EnergyReconciliation{{ID: selected.Item.Target.ID, Level: "allocation", Unit: "kWh", Period: "M1"}}
			}
			if err := epathCheckSQLAnnualServiceAbsent(bundle, *selected); err == nil || !strings.Contains(err.Error(), "invented a monthly") {
				t.Fatalf("zero record escaped explicit temporal absence guard: %v", err)
			}
			var coverage epathSQLModelCoverageReport
			context := epathSQLCoverageContext{scope: "building", period: "M1", nodes: period.Nodes, links: period.Links, rows: period.Reconciliation}
			epathSQLCoverageRecords(bundle, context, []epathSQLModelCheck{*selected}, nil, &coverage)
			rejected := false
			for _, failure := range coverage.Failures {
				if strings.Contains(failure.Message, "custom proof cannot authorize record coverage") && strings.Contains(failure.Message, "invented a monthly") {
					rejected = true
				}
			}
			if !rejected {
				t.Fatal("coverage trusted an invalid annual-only absence marker")
			}
		})
	}
}
