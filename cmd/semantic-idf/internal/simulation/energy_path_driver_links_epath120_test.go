package simulation

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestEPATH120GeneratedDriverLinksCloseEachServiceBeforeAndAfterReload(t *testing.T) {
	series := []energyExplanationSeries{
		review080Load("office-cooling", "Office", "cooling", map[int]float64{1: 10, 2: 20}),
		review080Load("office-heating", "Office", "heating", map[int]float64{1: 5, 2: 7}),
		review080Heat(t, "people", "Office", "Zone People Convective Heating Energy", map[int]float64{1: 8, 2: 2}),
		review080Heat(t, "infiltration-loss", "Office", "Zone Infiltration Sensible Heat Loss Energy", map[int]float64{1: 4, 2: 3}),
		// A real load with no matching pressure is explicitly allocated to Other/storage.
		review080Load("lab-cooling", "Lab", "cooling", map[int]float64{1: 3, 2: 4}),
		// A zero-load zone retains raw observations, but contributes no driver ribbon.
		review080Load("zero-cooling", "Empty", "cooling", map[int]float64{1: 0, 2: 0}),
		review080Heat(t, "zero-pressure", "Empty", "Zone People Convective Heating Energy", map[int]float64{1: 5, 2: 6}),
	}
	generated := review080BuildResult(series, map[string]float64{"Office": 2, "Lab": 1, "Empty": 2})
	wantSources := epath120SourceAccounting(generated.Sources)
	for _, id := range []string{"people", "infiltration-loss", "zero-pressure"} {
		source := review080Source(t, generated.Sources, id)
		if !source.AllocationApplied || source.AllocationFormula == "" || source.AllocationExplanation == "" {
			t.Fatalf("generated driver source %s lacks inspectable allocation provenance: %#v", id, source)
		}
	}
	for _, stage := range []string{"generated", "first reload", "second reload"} {
		if stage != "generated" {
			generated = epath120Reload(t, generated)
		}
		t.Run(stage, func(t *testing.T) {
			epath120AssertDriverClosure(t, generated.Nodes, generated.Links, map[string]float64{"cooling": 67, "heating": 24})
			for periodID, expected := range map[string]map[string]float64{"M1": {"cooling": 23, "heating": 10}, "M2": {"cooling": 44, "heating": 14}} {
				period := review080Period(t, generated.Periods, periodID)
				epath120AssertDriverClosure(t, period.Nodes, period.Links, expected)
			}
			people := review080DriverNode(generated.Nodes, energyDriverCategoryPeople, "cooling")
			if people == nil || people.RawValue != 10 || people.EffectiveValue != 20 || people.AllocatedValue != 60 || people.Value != 60 {
				t.Errorf("monthly-first People contribution lost raw/effective/allocated distinction: %#v", people)
			}
			storage := review080DriverNode(generated.Nodes, energyDriverCategoryStorageOther, "cooling")
			if storage == nil || storage.RawValue != 0 || storage.EffectiveValue != 0 || storage.AllocatedValue != 7 || storage.Value != 7 {
				t.Errorf("Other/storage contribution failed to retain 7 kWh closure: %#v", storage)
			}
			zero := review080Source(t, generated.Sources, "zero-pressure")
			if zero.RawValue != 11 || zero.EffectiveValue != 22 || zero.AllocatedValue != 0 || !zero.AllocationApplied {
				t.Errorf("explicit zero allocation was replaced by raw pressure: %#v", zero)
			}
			if got := epath120SourceAccounting(generated.Sources); !reflect.DeepEqual(got, wantSources) {
				t.Errorf("reload changed inspector source accounting or allocation formula\ngot=%#v\nwant=%#v", got, wantSources)
			}
		})
	}
}

func TestEPATH120StoredDriverWidthsUseExplicitAllocationIncludingZero(t *testing.T) {
	for _, test := range []struct {
		name                      string
		raw, effective, allocated float64
		storedWidth, wantWidth    float64
	}{
		{name: "explicit allocated zero", raw: 5, effective: 10, allocated: 0, storedWidth: 10, wantWidth: 0},
		{name: "stale raw pressure width capped", raw: 15, effective: 30, allocated: 6, storedWidth: 30, wantWidth: 6},
		{name: "underwidth driver link restored to allocation", raw: 10, effective: 20, allocated: 10, storedWidth: 4, wantWidth: 10},
		{name: "zero raw Other storage allocation", raw: 0, effective: 0, allocated: 7, storedWidth: 7, wantWidth: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			category := energyDriverCategoryPeople
			if test.raw == 0 {
				category = energyDriverCategoryStorageOther
			}
			result := epath120StoredDriverFixture(category, test.raw, test.effective, test.allocated, test.storedWidth)
			wantSource := epath120SourceAccounting(result.Sources)
			for pass := 1; pass <= 2; pass++ {
				result = epath120Reload(t, result)
				driver := review080DriverNode(result.Nodes, category, "cooling")
				if driver == nil && test.allocated > 0 {
					t.Fatalf("reload %d dropped allocated driver", pass)
				}
				if driver != nil && (driver.RawValue != test.raw || driver.EffectiveValue != test.effective || driver.AllocatedValue != test.allocated || driver.Value != test.allocated) {
					t.Errorf("reload %d conflated raw/effective/allocated driver values: %#v", pass, driver)
				}
				width := 0.0
				for _, link := range result.Links {
					if link.Relation != "driver_to_load" {
						continue
					}
					width += link.FromValue
					if link.FromValue != link.ToValue || link.FromUnit != "kWh thermal" || link.ToUnit != "kWh thermal" || link.ServiceKind != "cooling" {
						t.Errorf("reload %d lost equal thermal endpoints/service: %#v", pass, link)
					}
				}
				if width != test.wantWidth {
					t.Errorf("reload %d driver ribbon = %g, want explicit allocated contribution %g", pass, width, test.wantWidth)
				}
				if got := epath120SourceAccounting(result.Sources); !reflect.DeepEqual(got, wantSource) {
					t.Errorf("reload %d changed raw pressure or inspector formula: got=%#v want=%#v", pass, got, wantSource)
				}
			}
		})
	}
}

func TestEPATH120RefreshingDriverLinkPreservesPartialTemporalConversion(t *testing.T) {
	result := epath120StoredDriverFixture(energyDriverCategoryPeople, 10, 20, 10, 4)
	result.Nodes = append(result.Nodes,
		EnergyExplanationNode{ID: "end_use.cooling.building", Level: "end_use", Kind: "end_use.cooling", EndUse: "cooling", ServiceKind: "cooling", Value: 2, Unit: "kWh", ScaleDomain: "site", SourceIDs: []string{"observed-energy"}},
		EnergyExplanationNode{ID: "carrier.electricity.building", Level: "carrier", Kind: "carrier.electricity", Carrier: "electricity", Value: 2, Unit: "kWh", ScaleDomain: "site"},
	)
	conversion := EnergyPathLink{
		ID: "observed-conversion", FromID: "load.cooling.building", ToID: "end_use.cooling.building", Relation: "load_to_end_use", Basis: "direct_zone_energy",
		FromValue: 4, FromUnit: "kWh thermal", ToValue: 2, ToUnit: "kWh", Ratio: 2, RatioKind: "coefficient_of_performance", ServiceKind: "cooling",
		Explanation: "Only observed coincident months; partial temporal coverage", SourceIDs: []string{"observed-energy", "observed-load-month"},
	}
	result.Links = append(result.Links, conversion, EnergyPathLink{
		ID: "consumption", FromID: "end_use.cooling.building", ToID: "carrier.electricity.building", Relation: "end_use_to_carrier", Basis: "reported_meter", FromValue: 2, FromUnit: "kWh", ToValue: 2, ToUnit: "kWh",
	})
	for pass := 1; pass <= 2; pass++ {
		result = epath120Reload(t, result)
		found := false
		for _, link := range result.Links {
			if link.Relation == "driver_to_load" && (link.FromValue != 10 || link.ToValue != 10) {
				t.Errorf("reload %d did not repair full allocated driver width: %#v", pass, link)
			}
			if link.ID != conversion.ID {
				continue
			}
			found = true
			if link.FromValue != 4 || link.ToValue != 2 || link.Explanation != conversion.Explanation || !reflect.DeepEqual(link.SourceIDs, conversion.SourceIDs) {
				t.Errorf("reload %d fabricated unobserved conversion coverage from annual load: %#v", pass, link)
			}
		}
		if !found {
			t.Errorf("reload %d dropped valid partial-temporal conversion", pass)
		}
	}
}

func TestEPATH120ReconnectsOnlyUnambiguousMatchingThermalLoad(t *testing.T) {
	for _, test := range []struct {
		name      string
		mutate    func(*EnergyExplanationResult)
		wantLinks int
	}{
		{name: "missing Other storage connection", mutate: func(result *EnergyExplanationResult) { result.Links = nil }, wantLinks: 1},
		{name: "wrong service rejected", mutate: func(result *EnergyExplanationResult) { result.Nodes[1].ServiceKind = "heating" }, wantLinks: 0},
		{name: "wrong domain rejected", mutate: func(result *EnergyExplanationResult) { result.Nodes[1].ScaleDomain = "site" }, wantLinks: 0},
		{name: "ambiguous missing target not invented", mutate: func(result *EnergyExplanationResult) {
			result.Links = nil
			second := result.Nodes[1]
			second.ID = "load.cooling.second"
			result.Nodes = append(result.Nodes, second)
		}, wantLinks: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := epath120StoredDriverFixture(energyDriverCategoryStorageOther, 0, 0, 7, 7)
			test.mutate(&result)
			wantSource := epath120SourceAccounting(result.Sources)
			for pass := 1; pass <= 2; pass++ {
				result = epath120Reload(t, result)
				count := 0
				for _, link := range result.Links {
					if link.Relation != "driver_to_load" {
						continue
					}
					count++
					if link.FromValue != 7 || link.ToValue != 7 || link.ServiceKind != "cooling" {
						t.Errorf("reload %d did not reuse explicit Other/storage allocation: %#v", pass, link)
					}
				}
				if count != test.wantLinks {
					t.Errorf("reload %d valid driver links = %d, want %d", pass, count, test.wantLinks)
				}
				if got := epath120SourceAccounting(result.Sources); !reflect.DeepEqual(got, wantSource) {
					t.Errorf("reload %d changed source evidence while validating topology: %#v", pass, got)
				}
			}
		})
	}
}

func epath120StoredDriverFixture(category string, raw, effective, allocated, width float64) EnergyExplanationResult {
	return EnergyExplanationResult{
		Schema: energyExplanationSchema,
		Scope:  EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
		Nodes: []EnergyExplanationNode{
			{ID: "driver." + category + ".cooling.building", Level: "driver", DriverCategory: category, Kind: "driver." + category, Value: allocated, RawValue: raw, EffectiveValue: effective, AllocatedValue: allocated, AllocationApplied: true, Unit: "kWh thermal", ScaleDomain: "thermal", ServiceKind: "cooling", Basis: "heat_balance_share", SourceIDs: []string{"pressure"}},
			{ID: "load.cooling.building", Level: "load", Kind: "load.cooling", Value: allocated, RawValue: allocated, EffectiveValue: allocated, AllocatedValue: allocated, Unit: "kWh thermal", ScaleDomain: "thermal", ServiceKind: "cooling", SourceIDs: []string{"load"}},
		},
		Links:   []EnergyPathLink{{ID: "driver-load", FromID: "driver." + category + ".cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", FromValue: width, ToValue: width, FromUnit: "kWh thermal", ToUnit: "kWh thermal", ServiceKind: "cooling", SourceIDs: []string{"pressure", "load"}}},
		Sources: []EnergyDataSource{{ID: "pressure", SourceType: "test", RawValue: raw, EffectiveValue: effective, AllocatedValue: allocated, AllocationApplied: true, SourceUnit: "kWh thermal", NormalizedUnit: "kWh thermal", AllocationFormula: "actual service load * matching pressure / total matching pressure", AllocationExplanation: "Monthly signed heat-balance share; raw pressure remains independent."}},
	}
}

func epath120AssertDriverClosure(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink, loads map[string]float64) {
	t.Helper()
	byID := map[string]EnergyExplanationNode{}
	for _, node := range nodes {
		byID[node.ID] = node
	}
	for service, want := range loads {
		total := 0.0
		loadTotal := 0.0
		for _, node := range nodes {
			if node.Level == "load" && node.ServiceKind == service {
				loadTotal += node.Value
			}
		}
		for _, link := range links {
			if link.Relation != "driver_to_load" || link.ServiceKind != service {
				continue
			}
			driver, load := byID[link.FromID], byID[link.ToID]
			if driver.Level != "driver" || load.Level != "load" || driver.ScaleDomain != "thermal" || load.ScaleDomain != "thermal" || driver.ServiceKind != service || load.ServiceKind != service || link.FromUnit != driver.Unit || link.ToUnit != load.Unit || (link.FromUnit != "kWh" && link.FromUnit != "kWh thermal") || link.FromUnit != link.ToUnit {
				t.Errorf("driver link does not join matching thermal/service endpoints: %#v", link)
			}
			if link.FromValue != link.ToValue || link.FromValue != driver.AllocatedValue {
				t.Errorf("driver ribbon width must equal allocated contribution, not raw pressure: %#v driver=%#v", link, driver)
			}
			total += link.ToValue
		}
		if math.Abs(total-want) > 1e-9 || math.Abs(loadTotal-want) > 1e-9 {
			t.Errorf("%s incoming closure/load = %g/%g, want %g", service, total, loadTotal, want)
		}
	}
}

func epath120Reload(t *testing.T, result EnergyExplanationResult) EnergyExplanationResult {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded EnergyExplanationResult
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	return reloaded
}

func epath120SourceAccounting(sources []EnergyDataSource) map[string]any {
	result := map[string]any{}
	for _, source := range sources {
		result[source.ID] = []any{source.RawValue, source.EffectiveValue, source.AllocatedValue, source.AllocationApplied, source.Formula, source.AllocationFormula, source.AllocationExplanation, append([]string{}, source.InputSourceIDs...)}
	}
	return result
}
