package simulation

import (
	"sort"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEPATH080ReviewAllocatesPerZoneMonthBeforeBuildingAggregation(t *testing.T) {
	series := []energyExplanationSeries{
		// Each zone's positive pressure exceeds its actual cooling load. This
		// makes zone-first allocation observably different from taking one
		// building-wide pressure share after multipliers are applied.
		review080Load("load-office", "Office", "cooling", map[int]float64{1: 9, 2: 2}),
		review080Load("load-lab", "Lab", "cooling", map[int]float64{1: 1, 2: 8}),
		review080Heat(t, "people-office", "Office", "Zone People Convective Heating Energy", map[int]float64{1: 9, 2: 9}),
		review080Heat(t, "infiltration-office", "Office", "Zone Infiltration Sensible Heat Gain Energy", map[int]float64{1: 1, 2: 1}),
		review080Heat(t, "people-lab", "Lab", "Zone People Convective Heating Energy", map[int]float64{1: 1, 2: 1}),
		review080Heat(t, "infiltration-lab", "Lab", "Zone Infiltration Sensible Heat Gain Energy", map[int]float64{1: 9, 2: 9}),
	}
	result := review080BuildResult(series, map[string]float64{"Office": 2, "Lab": 3})

	checks := []struct {
		name         string
		nodes        []EnergyExplanationNode
		links        []EnergyPathLink
		people       float64
		infiltration float64
		load         float64
	}{
		{name: "building M1", nodes: review080Period(t, result.Periods, "M1").Nodes, links: review080Period(t, result.Periods, "M1").Links, people: 16.5, infiltration: 4.5, load: 21},
		{name: "building M2", nodes: review080Period(t, result.Periods, "M2").Nodes, links: review080Period(t, result.Periods, "M2").Links, people: 6, infiltration: 22, load: 28},
		{name: "building annual", nodes: result.Nodes, links: result.Links, people: 22.5, infiltration: 26.5, load: 49},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if got := review080NodeValue(check.nodes, "load", "", "cooling"); got != check.load {
				t.Fatalf("cooling load = %g, want %g", got, check.load)
			}
			review080AssertAllocatedDriver(t, check.nodes, check.links, energyDriverCategoryPeople, check.people)
			review080AssertAllocatedDriver(t, check.nodes, check.links, energyDriverCategoryInfiltration, check.infiltration)
			if got := review080DriverTotal(check.nodes, "cooling"); got != check.load {
				t.Fatalf("allocated driver closure = %g, load = %g", got, check.load)
			}
		})
	}

	annualPeople := review080DriverNode(result.Nodes, energyDriverCategoryPeople, "cooling")
	if annualPeople == nil || annualPeople.RawValue != 20 || annualPeople.EffectiveValue != 42 || annualPeople.AllocatedValue != 22.5 || annualPeople.Value != 22.5 {
		t.Fatalf("annual people raw/effective/allocated metadata = %#v, want 20/42/22.5", annualPeople)
	}
	if !review080HasAll(annualPeople.SourceIDs, "people-office", "people-lab") || review080HasAny(annualPeople.SourceIDs, "infiltration-office", "infiltration-lab") {
		t.Fatalf("people category provenance leaked across categories: %#v", annualPeople.SourceIDs)
	}
	annualInfiltration := review080DriverNode(result.Nodes, energyDriverCategoryInfiltration, "cooling")
	if annualInfiltration == nil || annualInfiltration.RawValue != 20 || annualInfiltration.EffectiveValue != 58 || annualInfiltration.AllocatedValue != 26.5 || annualInfiltration.Value != 26.5 {
		t.Fatalf("annual infiltration raw/effective/allocated metadata = %#v, want 20/58/26.5", annualInfiltration)
	}
	if !review080HasAll(annualInfiltration.SourceIDs, "infiltration-office", "infiltration-lab") || review080HasAny(annualInfiltration.SourceIDs, "people-office", "people-lab") {
		t.Fatalf("infiltration category provenance leaked across categories: %#v", annualInfiltration.SourceIDs)
	}

	office := review080ZoneResult(t, result.ZoneResults, "Office")
	lab := review080ZoneResult(t, result.ZoneResults, "Lab")
	for _, check := range []struct {
		name         string
		zone         *EnergyExplanationZoneResult
		people       float64
		infiltration float64
		load         float64
	}{
		{name: "Office", zone: office, people: 19.8, infiltration: 2.2, load: 22},
		{name: "Lab", zone: lab, people: 2.7, infiltration: 24.3, load: 27},
	} {
		t.Run(check.name+" annual isolation", func(t *testing.T) {
			review080AssertAllocatedDriver(t, check.zone.Nodes, check.zone.Links, energyDriverCategoryPeople, check.people)
			review080AssertAllocatedDriver(t, check.zone.Nodes, check.zone.Links, energyDriverCategoryInfiltration, check.infiltration)
			if got := review080DriverTotal(check.zone.Nodes, "cooling"); got != check.load {
				t.Fatalf("zone allocated driver closure = %g, want %g", got, check.load)
			}
		})
	}
}

func TestEPATH080ReviewLastRemainderClosesAwkwardDecimalsDeterministically(t *testing.T) {
	forward := []energyExplanationSeries{
		review080Load("load", "Office", "cooling", map[int]float64{1: 1, 2: 1}),
		review080Heat(t, "people", "Office", "Zone People Convective Heating Energy", map[int]float64{1: 1, 2: 1}),
		review080Heat(t, "lighting", "Office", "Zone Lights Convective Heating Energy", map[int]float64{1: 1, 2: 1}),
		review080Heat(t, "equipment", "Office", "Zone Electric Equipment Convective Heating Energy", map[int]float64{1: 1, 2: 1}),
	}
	reverse := append([]energyExplanationSeries(nil), forward...)
	for left, right := 1, len(reverse)-1; left < right; left, right = left+1, right-1 {
		reverse[left], reverse[right] = reverse[right], reverse[left]
	}

	forwardResult := review080BuildResult(forward, map[string]float64{"Office": 1})
	reverseResult := review080BuildResult(reverse, map[string]float64{"Office": 1})
	for _, periodID := range []string{"M1", "M2"} {
		forwardValues := review080DriverValues(review080Period(t, forwardResult.Periods, periodID).Nodes, "cooling")
		reverseValues := review080DriverValues(review080Period(t, reverseResult.Periods, periodID).Nodes, "cooling")
		if !review080EqualCategoryValues(forwardValues, reverseValues) {
			t.Fatalf("%s allocation depends on input order: forward=%#v reverse=%#v", periodID, forwardValues, reverseValues)
		}
		if got := review080SumValues(forwardValues); got != 1 {
			t.Fatalf("%s last-remainder closure = %g, want exactly 1", periodID, got)
		}
		values := review080SortedValues(forwardValues)
		if len(values) != 3 || values[0] != 0.333 || values[1] != 0.333 || values[2] != 0.334 {
			t.Fatalf("%s thirds = %#v, want 0.333/0.333/0.334", periodID, values)
		}
	}
	annualValues := review080DriverValues(forwardResult.Nodes, "cooling")
	if got := review080SumValues(annualValues); got != 2 {
		t.Fatalf("annual sum of monthly last remainders = %g, want exactly 2", got)
	}
	values := review080SortedValues(annualValues)
	if len(values) != 3 || values[0] != 0.666 || values[1] != 0.666 || values[2] != 0.668 {
		t.Fatalf("annual thirds = %#v, want monthly-summed 0.666/0.666/0.668", values)
	}
}

func TestEPATH080ReviewExplicitZeroAndZeroPressureRemainDistinct(t *testing.T) {
	series := []energyExplanationSeries{
		// A reported positive pressure with no actual load must remain inspectable
		// as raw provenance but must not be resurrected as a main ribbon.
		review080Load("office-load-zero", "Office", "cooling", map[int]float64{1: 0}),
		review080Heat(t, "office-people-pressure", "Office", "Zone People Convective Heating Energy", map[int]float64{1: 5}),
		// Lab has an actual cooling load but only a heating-direction pressure.
		// Its cooling allocation therefore belongs wholly to Other / storage.
		review080Load("lab-load", "Lab", "cooling", map[int]float64{1: 7}),
		review080Heat(t, "lab-infiltration-loss", "Lab", "Zone Infiltration Sensible Heat Loss Energy", map[int]float64{1: 3}),
	}
	result := review080BuildResult(series, map[string]float64{"Office": 2, "Lab": 1})

	if node := review080DriverNode(result.Nodes, energyDriverCategoryPeople, "cooling"); node != nil {
		t.Fatalf("explicitly allocated-zero Office pressure survived as a main annual node: %#v", node)
	}
	for _, link := range result.Links {
		if link.Relation == "driver_to_load" && review080HasAny(link.SourceIDs, "office-people-pressure") {
			t.Fatalf("explicitly allocated-zero Office pressure survived as a main annual link: %#v", link)
		}
	}
	pressure := review080Source(t, result.Sources, "office-people-pressure")
	if pressure.RawValue != 5 || pressure.EffectiveValue != 10 || pressure.AllocatedValue != 0 || !pressure.AllocationApplied || pressure.DriverCategory != energyDriverCategoryPeople {
		t.Fatalf("allocated-zero raw provenance = %#v, want raw/effective/allocated 5/10/0 with explicit marker", pressure)
	}
	if pressure.AggregationBasis == "" {
		t.Fatalf("allocated-zero source lost its aggregation basis: %#v", pressure)
	}

	storage := review080DriverNode(result.Nodes, energyDriverCategoryStorageOther, "cooling")
	if storage == nil || storage.Value != 7 || storage.AllocatedValue != 7 || !storage.AllocationApplied || storage.Basis != "heat_balance_share" {
		t.Fatalf("zero-pressure Lab fallback = %#v, want allocated Other/storage 7", storage)
	}
	if !review080HasAll(storage.SourceIDs, "lab-load") || review080HasAny(storage.SourceIDs, "office-load-zero", "office-people-pressure") {
		t.Fatalf("zero-pressure fallback provenance crossed zones: %#v", storage.SourceIDs)
	}
	review080AssertAllocatedDriver(t, result.Nodes, result.Links, energyDriverCategoryStorageOther, 7)

	office := review080ZoneResult(t, result.ZoneResults, "Office")
	if got := review080DriverTotal(office.Nodes, "cooling"); got != 0 {
		t.Fatalf("Office zero-load scope has %g of main cooling drivers, want 0", got)
	}
	lab := review080ZoneResult(t, result.ZoneResults, "Lab")
	if got := review080DriverTotal(lab.Nodes, "cooling"); got != 7 {
		t.Fatalf("Lab zero-pressure scope closure = %g, want 7", got)
	}
	if node := review080DriverNode(lab.Nodes, energyDriverCategoryStorageOther, "cooling"); node == nil || node.Value != 7 {
		t.Fatalf("Lab zone result lost its isolated storage fallback: %#v", node)
	}
}

func TestEPATH080ReviewSyntheticClosureNeverBecomesPressureAfterStorageCategoryMerge(t *testing.T) {
	forward := []energyExplanationSeries{
		review080Load("surface-load", "Office", "cooling", map[int]float64{1: 100}),
		review080Heat(t, "surface-detail", "Wall A", "Surface Inside Face Convection Heat Gain Energy", map[int]float64{1: -10}),
		review080Heat(t, "surface-aggregate", "Office", "Zone Air Heat Balance Surface Convection Rate", map[int]float64{1: 12}),
	}
	reverse := append([]energyExplanationSeries(nil), forward...)
	for left, right := 0, len(reverse)-1; left < right; left, right = left+1, right-1 {
		reverse[left], reverse[right] = reverse[right], reverse[left]
	}
	report := idf.GeometryReport{Surfaces: []idf.GeometrySurface{
		{ID: "wall.a", Name: "Wall A", SurfaceType: "Wall", ZoneName: "Office", OutsideBoundary: "Outdoors"},
	}}
	build := func(series []energyExplanationSeries) EnergyExplanationResult {
		legacy := buildEnergyExplanationResultWithDriverContext(
			series,
			review080Sources(series),
			&PurposeRunPlan{},
			newEnergyDriverBuildContext(report),
		)
		return UpgradeEnergyExplanationV1(legacy)
	}

	forwardResult := build(forward)
	reverseResult := build(reverse)
	for _, result := range []struct {
		name   string
		result EnergyExplanationResult
	}{
		{name: "forward", result: forwardResult},
		{name: "reverse", result: reverseResult},
	} {
		t.Run(result.name, func(t *testing.T) {
			wall := review080DriverNode(result.result.Nodes, energyDriverCategoryExteriorWalls, "cooling")
			if wall == nil || wall.RawValue != 10 || wall.EffectiveValue != 10 || wall.SignedValue != 10 || wall.Value != 83.333 || wall.AllocatedValue != 83.333 {
				t.Fatalf("physical wall allocation = %#v, want pressure 10 and contribution 83.333", wall)
			}
			storage := review080DriverNode(result.result.Nodes, energyDriverCategoryStorageOther, "cooling")
			if storage == nil || storage.Value != 16.667 || storage.AllocatedValue != 16.667 {
				t.Fatalf("surface reconciliation allocation = %#v, want pressure 2 contribution 16.667", storage)
			}
			if got := review080DriverTotal(result.result.Nodes, "cooling"); got != 100 {
				t.Fatalf("surface/reconciliation contribution closure = %g, want 100", got)
			}
			reconciliation := review080SourceWithFormula(result.result.Sources, "signed zone surface-convection aggregate - signed selected surface-source sum")
			if reconciliation == nil || reconciliation.RawValue != 2 || reconciliation.EffectiveValue != 2 || reconciliation.AllocatedValue != 16.667 || !reconciliation.AllocationApplied {
				t.Fatalf("surface reconciliation provenance = %#v, want raw pressure 2 and contribution 16.667", reconciliation)
			}
			closure := review080Source(t, result.result.Sources, "derived-driver-unmapped_zone_balance-office")
			if closure.RawValue != 88 || closure.EffectiveValue != 88 || closure.AllocatedValue != 0 || !closure.AllocationApplied {
				t.Fatalf("synthetic pre-allocation closure = %#v, want raw-only 88 with explicit allocated zero", closure)
			}
			for _, link := range result.result.Links {
				if link.Relation != "driver_to_load" {
					continue
				}
				if link.FromID == wall.ID && (link.FromValue != 83.333 || link.ToValue != 83.333) {
					t.Fatalf("wall link used something other than allocated contribution: %#v", link)
				}
				if link.FromID == storage.ID && (link.FromValue != 16.667 || link.ToValue != 16.667) {
					t.Fatalf("reconciliation link used something other than allocated contribution: %#v", link)
				}
			}
		})
	}
	if !review080EqualCategoryValues(review080DriverValues(forwardResult.Nodes, "cooling"), review080DriverValues(reverseResult.Nodes, "cooling")) {
		t.Fatalf("surface reconciliation allocation changed with input order: forward=%#v reverse=%#v", review080DriverValues(forwardResult.Nodes, "cooling"), review080DriverValues(reverseResult.Nodes, "cooling"))
	}
}

func review080Load(sourceID string, zone string, service string, monthly map[int]float64) energyExplanationSeries {
	total := 0.0
	for _, value := range monthly {
		total = roundedEnergyNumber(total + value)
	}
	label := "Zone cooling load"
	name := "Zone Air System Sensible Cooling Energy"
	if service == "heating" {
		label = "Zone heating load"
		name = "Zone Air System Sensible Heating Energy"
	}
	return canonicalEnergyExplanationSeries(energyExplanationSeries{
		Stage:            "load",
		CanonicalKind:    "load.zone_" + service,
		Level:            "load",
		Kind:             "load.zone_" + service,
		Label:            label,
		Unit:             "kWh",
		ServiceKind:      service,
		PathType:         "zone",
		ZoneName:         zone,
		ThermalComponent: "sensible",
		SourceIDs:        []string{sourceID},
		Total:            total,
		Monthly:          monthly,
		SourceName:       name,
		sourceName:       name,
		sourceKeyValue:   zone,
		sourceFrequency:  "Monthly",
	})
}

func review080Heat(t *testing.T, sourceID string, zone string, name string, monthly map[int]float64) energyExplanationSeries {
	t.Helper()
	definition, ok := energyHeatAliasDefinitionForName(name)
	if !ok {
		t.Fatalf("missing heat alias %q", name)
	}
	total := 0.0
	for _, value := range monthly {
		total = roundedEnergyNumber(total + value)
	}
	return canonicalEnergyExplanationSeries(energyExplanationSeriesForBuilder(&energyExplanationSeriesBuilder{
		dictionary: energyExplanationDictionary{
			row:                sqlOutputDictionaryRow{keyValue: zone, name: name, units: "J"},
			reportingFrequency: "Monthly",
			heat:               &definition,
		},
		unit:    "kWh",
		total:   total,
		monthly: monthly,
	}, sourceID))
}

func review080BuildResult(series []energyExplanationSeries, multipliers map[string]float64) EnergyExplanationResult {
	index := energyEffectiveMultiplierIndex{
		Enabled:          true,
		Zones:            map[string]energyZoneMultiplierRecord{},
		SpaceZones:       map[string]string{},
		OutputKeyZones:   map[string]string{},
		GroupMultipliers: map[string]float64{},
	}
	for zone, multiplier := range multipliers {
		index.Zones[normalizePurposeToken(zone)] = energyZoneMultiplierRecord{ZoneName: zone, ZoneMultiplier: multiplier, GroupMultiplier: 1}
	}
	sources := review080Sources(series)
	legacy := buildEnergyExplanationResultWithDriverContext(series, sources, &PurposeRunPlan{}, energyDriverBuildContext{Enabled: true, Multipliers: index})
	return UpgradeEnergyExplanationV1(legacy)
}

func review080Sources(series []energyExplanationSeries) []EnergyDataSource {
	sources := make([]EnergyDataSource, 0, len(series))
	seen := map[string]bool{}
	for _, item := range series {
		item = canonicalEnergyExplanationSeries(item)
		for _, sourceID := range item.SourceIDs {
			if seen[sourceID] {
				continue
			}
			seen[sourceID] = true
			sources = append(sources, EnergyDataSource{
				ID:                 sourceID,
				SourceType:         "review_fixture",
				KeyValue:           item.SourceKey,
				Name:               item.SourceName,
				Units:              item.Unit,
				ReportingFrequency: item.sourceFrequency,
				ZoneName:           item.ZoneName,
			})
		}
	}
	return sources
}

func review080Period(t *testing.T, periods []EnergyPeriod, id string) *EnergyPeriod {
	t.Helper()
	for index := range periods {
		if periods[index].ID == id {
			return &periods[index]
		}
	}
	t.Fatalf("missing period %q in %#v", id, periods)
	return nil
}

func review080ZoneResult(t *testing.T, results []EnergyExplanationZoneResult, zone string) *EnergyExplanationZoneResult {
	t.Helper()
	for index := range results {
		if results[index].Scope.ZoneName == zone {
			return &results[index]
		}
	}
	t.Fatalf("missing zone result %q in %#v", zone, results)
	return nil
}

func review080DriverNode(nodes []EnergyExplanationNode, category string, service string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].Level == "driver" && nodes[index].DriverCategory == category && nodes[index].ServiceKind == service {
			return &nodes[index]
		}
	}
	return nil
}

func review080AssertAllocatedDriver(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink, category string, want float64) {
	t.Helper()
	node := review080DriverNode(nodes, category, "cooling")
	if node == nil || node.Value != want || node.AllocatedValue != want || !node.AllocationApplied || node.Basis != "heat_balance_share" || node.AllocationExplanation != energyDriverAllocationExplanation {
		t.Fatalf("%s allocated node = %#v, want value/allocated %g with allocation contract", category, node, want)
	}
	matched := false
	for _, link := range links {
		if link.Relation != "driver_to_load" || link.FromID != node.ID {
			continue
		}
		matched = true
		if link.FromValue != want || link.ToValue != want || link.Basis != "heat_balance_share" || link.Explanation != energyDriverAllocationExplanation {
			t.Fatalf("%s main link = %#v, want allocated width %g and non-causal basis", category, link, want)
		}
	}
	if !matched {
		t.Fatalf("missing %s driver_to_load link for %#v", category, node)
	}
}

func review080NodeValue(nodes []EnergyExplanationNode, level string, category string, service string) float64 {
	for _, node := range nodes {
		if node.Level == level && node.ServiceKind == service && (category == "" || node.DriverCategory == category) {
			return node.Value
		}
	}
	return 0
}

func review080DriverTotal(nodes []EnergyExplanationNode, service string) float64 {
	total := 0.0
	for _, node := range nodes {
		if node.Level == "driver" && node.ServiceKind == service {
			total = roundedEnergyNumber(total + node.Value)
		}
	}
	return total
}

func review080DriverValues(nodes []EnergyExplanationNode, service string) map[string]float64 {
	out := map[string]float64{}
	for _, node := range nodes {
		if node.Level == "driver" && node.ServiceKind == service {
			out[node.DriverCategory] = roundedEnergyNumber(out[node.DriverCategory] + node.Value)
		}
	}
	return out
}

func review080SumValues(values map[string]float64) float64 {
	total := 0.0
	for _, value := range values {
		total = roundedEnergyNumber(total + value)
	}
	return total
}

func review080SortedValues(values map[string]float64) []float64 {
	out := make([]float64, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.Float64s(out)
	return out
}

func review080EqualCategoryValues(left map[string]float64, right map[string]float64) bool {
	if len(left) != len(right) {
		return false
	}
	for category, value := range left {
		if right[category] != value {
			return false
		}
	}
	return true
}

func review080Source(t *testing.T, sources []EnergyDataSource, id string) *EnergyDataSource {
	t.Helper()
	for index := range sources {
		if sources[index].ID == id {
			return &sources[index]
		}
	}
	t.Fatalf("missing source %q in %#v", id, sources)
	return nil
}

func review080SourceWithFormula(sources []EnergyDataSource, formula string) *EnergyDataSource {
	for index := range sources {
		if sources[index].Formula == formula {
			return &sources[index]
		}
	}
	return nil
}

func review080HasAll(values []string, wanted ...string) bool {
	for _, want := range wanted {
		if !review080HasAny(values, want) {
			return false
		}
	}
	return true
}

func review080HasAny(values []string, wanted ...string) bool {
	for _, value := range values {
		for _, want := range wanted {
			if value == want {
				return true
			}
		}
	}
	return false
}
