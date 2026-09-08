package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

// Literal independent arithmetic, using only the original typed owner roster.
// The private values below are model totals; no model multiplier is reapplied.
func vrfAllocationFixture(t *testing.T, months int) (energyPathVRFConsumptionCohort, []energyPathVRFLoadObservation) {
	t.Helper()
	systems := energyPathVRFSystems(energyPathVRFDocument(t))
	if len(systems) != 1 {
		t.Fatalf("original VRF systems=%d, want1", len(systems))
	}
	cohort := energyPathVRFConsumptionCohort{System: systems[0], Requested: true, Months: map[int]bool{}}
	local := map[string][]float64{"cooling": {0, 2, 3, 4, 5}, "heating": {1, 0, 3, 4, 5}}
	for _, target := range cohort.System.Targets {
		observation := energyPathVRFConsumptionObservation{Target: target, Requested: true, Monthly: map[int]float64{}, Source: EnergyDataSource{
			ID: target.Definition.ID + "/" + target.KeyValue, Name: target.Definition.Energy.Aliases[0], KeyValue: target.KeyValue,
			SourceType: "sql_report_data", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", ZoneName: target.ZoneName,
		}}
		base := map[string]float64{"cooling.vrf.outdoor_electricity": 100, "cooling.vrf.crankcase_electricity": 10,
			"heating.vrf.outdoor_electricity": 200, "heating.vrf.defrost_electricity": 20}[target.Definition.ID]
		if !target.Definition.Shared {
			for zone := 1; zone <= 5; zone++ {
				if target.ZoneName == fmt.Sprintf("SPACE%d-1", zone) {
					base = local[target.Definition.Energy.EndUse][zone-1]
				}
			}
		}
		for month := 1; month <= months; month++ {
			observation.Monthly[month] = base * float64(month)
			cohort.Months[month] = true
		}
		cohort.Observations = append(cohort.Observations, observation)
	}
	loads := []energyPathVRFLoadObservation{}
	for zone := 1; zone <= 5; zone++ {
		for _, service := range []string{"cooling", "heating"} {
			name := fmt.Sprintf("SPACE%d-1", zone)
			load := energyPathVRFLoadObservation{ZoneName: name, ServiceKind: service, Monthly: map[int]float64{}, SourceIDs: []string{"load/" + name + "/" + service}}
			for month := 1; month <= months; month++ {
				weight := zone
				if (month%2 == 0) != (service == "heating") {
					weight = 6 - zone
				}
				load.Monthly[month] = float64(10 * weight * month)
			}
			loads = append(loads, load)
		}
	}
	return cohort, loads
}

func vrfAllocationZone(t *testing.T, plan energyPathVRFAllocationPlan, service, zone string, month int) energyPathVRFZoneAllocation {
	t.Helper()
	var found []energyPathVRFZoneAllocation
	for _, item := range plan.Zones {
		if item.ServiceKind == service && item.ZoneName == zone && item.Month == month {
			found = append(found, item)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s/%s/month%d rows=%d, want1; issues=%v", service, zone, month, len(found), plan.Issues)
	}
	return found[0]
}

func vrfAllocationNear(t *testing.T, actual, expected float64) {
	t.Helper()
	if math.IsNaN(actual) || math.IsInf(actual, 0) || math.Abs(actual-expected) > 1e-10 {
		t.Fatalf("value=%.15g, want%.15g", actual, expected)
	}
}

func TestEnergyPathVRFAllocationZeroLocalAndMonthFirst(t *testing.T) {
	cohort, loads := vrfAllocationFixture(t, 3)
	plan := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads)
	if len(plan.Zones) != 40 || len(plan.Services) != 8 || len(plan.Sources) != 120 {
		t.Fatalf("VRF complete three-month+Annual roster=%d/%d/%d, want40/8/120", len(plan.Zones), len(plan.Services), len(plan.Sources))
	}
	first := vrfAllocationZone(t, plan, "cooling", "SPACE1-1", 1)
	if !first.DirectKnown || !first.AllocatedKnown || !first.TotalKnown || !first.LoadKnown || first.DirectValue != 0 {
		t.Fatalf("known-zero terminal lost eligibility: %#v", first)
	}
	vrfAllocationNear(t, first.AllocatedValue, 110.0/15)
	vrfAllocationNear(t, first.TotalValue, 110.0/15)
	annual := vrfAllocationZone(t, plan, "cooling", "SPACE1-1", 0)
	// 110*(1/15) + 220*(5/15) + 330*(1/15), not an Annual load-share reallocation.
	vrfAllocationNear(t, annual.AllocatedValue, 308.0/3)
	if !annual.TotalKnown || len(annual.Months) != 3 || annual.PeriodID != "annual" {
		t.Fatalf("actual three-month completeness lost: %#v", annual)
	}
	for _, record := range plan.Services {
		factor := float64(record.Month)
		if record.Month == 0 {
			factor = 6
		}
		direct, shared := 14.0, 110.0
		if record.ServiceKind == "heating" {
			direct, shared = 13, 220
		}
		if !record.DirectKnown || !record.SharedKnown || !record.AllocatedKnown || !record.UnassignedKnown || !record.LoadKnown {
			t.Fatalf("complete service marked unknown: %#v", record)
		}
		vrfAllocationNear(t, record.DirectValue, direct*factor)
		vrfAllocationNear(t, record.SharedValue, shared*factor)
		vrfAllocationNear(t, record.AllocatedValue, shared*factor)
		vrfAllocationNear(t, record.UnassignedValue, 0)
	}
}

func vrfAllocationObservationFor(t *testing.T, cohort *energyPathVRFConsumptionCohort, role, zone string) *energyPathVRFConsumptionObservation {
	t.Helper()
	for i := range cohort.Observations {
		item := &cohort.Observations[i]
		if item.Target.Definition.ID == role && item.Target.ZoneName == zone {
			return item
		}
	}
	t.Fatalf("missing fixture observation %s/%s", role, zone)
	return nil
}

func vrfAllocationService(t *testing.T, plan energyPathVRFAllocationPlan, service string, month int) energyPathVRFServiceAllocation {
	t.Helper()
	for _, item := range plan.Services {
		if item.ServiceKind == service && item.Month == month {
			return item
		}
	}
	t.Fatalf("missing service %s/month%d: %v", service, month, plan.Issues)
	return energyPathVRFServiceAllocation{}
}

func TestEnergyPathVRFAllocationIncompleteSharedCohort(t *testing.T) {
	for _, kind := range []string{"missing", "invalid_month", "nonfinite", "negative", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			cohort, loads := vrfAllocationFixture(t, 3)
			bad := vrfAllocationObservationFor(t, &cohort, "cooling.vrf.outdoor_electricity", "")
			switch kind {
			case "missing":
				delete(bad.Monthly, 2)
			case "invalid_month":
				bad.InvalidMonths = map[int]bool{2: true}
			case "nonfinite":
				bad.Monthly[2] = math.Inf(1)
			case "negative":
				bad.Monthly[2] = -1
			case "duplicate":
				cohort.Observations = append(cohort.Observations, *bad)
			}
			plan := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads)
			item := vrfAllocationZone(t, plan, "cooling", "SPACE1-1", 2)
			if !item.DirectKnown || item.SharedKnown || item.AllocatedKnown || item.TotalKnown || item.AllocatedValue != 0 {
				t.Fatalf("incomplete shared cohort allocated sibling or fabricated total: %#v", item)
			}
			record := vrfAllocationService(t, plan, "cooling", 2)
			if record.SharedKnown || record.UnassignedKnown || record.SharedValue != 20 || record.UnassignedValue != 20 {
				t.Fatalf("valid sibling must remain partial observed/unassigned context: %#v", record)
			}
			for _, trace := range plan.Sources {
				if trace.Month == 2 && trace.Target.Definition.ID == "cooling.vrf.crankcase_electricity" {
					if !trace.ObservedKnown || trace.ObservedValue != 20 || trace.AllocatedKnown || trace.AllocatedValue != 0 || trace.Target.ZoneName != "" {
						t.Fatalf("valid sibling trace lost or laundered: %#v", trace)
					}
				}
			}
			annual := vrfAllocationZone(t, plan, "cooling", "SPACE1-1", 0)
			if annual.TotalKnown || annual.SharedKnown || annual.AllocatedKnown {
				t.Fatal("incomplete month became complete Annual")
			}
			if kind != "duplicate" {
				vrfAllocationNear(t, annual.AllocatedValue, 110.0/15+330.0/15)
				if !vrfAllocationZone(t, plan, "cooling", "SPACE1-1", 1).TotalKnown {
					t.Fatal("one invalid month poisoned another observed month")
				}
			}
			if !vrfAllocationZone(t, plan, "heating", "SPACE2-1", 2).TotalKnown {
				t.Fatal("cooling gap poisoned independent heating service")
			}
		})
	}
}

func TestEnergyPathVRFAllocationMissingLocalDoesNotVetoShared(t *testing.T) {
	cohort, loads := vrfAllocationFixture(t, 3)
	delete(vrfAllocationObservationFor(t, &cohort, "cooling.vrf.terminal_electricity", "SPACE1-1").Monthly, 2)
	plan := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads)
	item := vrfAllocationZone(t, plan, "cooling", "SPACE1-1", 2)
	if item.DirectKnown || !item.SharedKnown || !item.AllocatedKnown || item.TotalKnown {
		t.Fatalf("missing local incorrectly blocked shared or completed total: %#v", item)
	}
	vrfAllocationNear(t, item.AllocatedValue, 220.0/3)
	if !vrfAllocationZone(t, plan, "cooling", "SPACE2-1", 2).TotalKnown {
		t.Fatal("missing one local poisoned another complete owner")
	}
	if vrfAllocationZone(t, plan, "cooling", "SPACE1-1", 0).TotalKnown {
		t.Fatal("missing local month became known Annual total")
	}
}

func TestEnergyPathVRFAllocationPartialRequestPreservesValidSiblings(t *testing.T) {
	cohort, loads := vrfAllocationFixture(t, 3)
	cohort.Requested = false // One original request is absent, not every source.
	for i := range cohort.Observations {
		if cohort.Observations[i].Target.Definition.ID == "cooling.vrf.terminal_electricity" && cohort.Observations[i].Target.ZoneName == "SPACE1-1" {
			cohort.Observations = append(cohort.Observations[:i], cohort.Observations[i+1:]...)
			break
		}
	}
	plan := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads)
	missing := vrfAllocationZone(t, plan, "cooling", "SPACE1-1", 1)
	if missing.DirectKnown || !missing.SharedKnown || !missing.AllocatedKnown || missing.TotalKnown {
		t.Fatalf("missing one request changed independently observed shared pool: %#v", missing)
	}
	if !vrfAllocationZone(t, plan, "cooling", "SPACE2-1", 1).TotalKnown || !vrfAllocationZone(t, plan, "heating", "SPACE1-1", 0).TotalKnown {
		t.Fatal("valid requested sibling observations were discarded")
	}
}

func TestEnergyPathVRFAllocationDoesNotShrinkLoadDenominator(t *testing.T) {
	for _, kind := range []string{"missing", "invalid", "nonfinite", "negative", "duplicate", "missing_trace"} {
		t.Run(kind, func(t *testing.T) {
			cohort, loads := vrfAllocationFixture(t, 3)
			for i := range loads {
				if loads[i].ZoneName != "SPACE5-1" || loads[i].ServiceKind != "cooling" {
					continue
				}
				switch kind {
				case "missing":
					delete(loads[i].Monthly, 1)
				case "invalid":
					loads[i].InvalidMonths = map[int]bool{1: true}
				case "nonfinite":
					loads[i].Monthly[1] = math.NaN()
				case "negative":
					loads[i].Monthly[1] = -1
				case "duplicate":
					loads = append(loads, loads[i])
				case "missing_trace":
					loads[i].SourceIDs = nil
				}
				break
			}
			// An unrelated observed positive load is not an eligible substitute.
			loads = append(loads, energyPathVRFLoadObservation{ZoneName: "PLENUM-1", ServiceKind: "cooling", Monthly: map[int]float64{1: 900}, SourceIDs: []string{"plenum"}})
			plan := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads)
			record := vrfAllocationService(t, plan, "cooling", 1)
			if record.LoadKnown || record.AllocatedKnown || !record.SharedKnown || !record.UnassignedKnown || record.AllocatedValue != 0 || record.UnassignedValue != 110 {
				t.Fatalf("missing owner shrank denominator or lost unassigned pool: %#v", record)
			}
			for _, item := range plan.Zones {
				if item.ZoneName == "PLENUM-1" {
					t.Fatal("unowned load became VRF recipient")
				}
				if item.ServiceKind == "cooling" && item.Month == 1 && (item.AllocatedKnown || item.AllocatedValue != 0 || item.TotalKnown) {
					t.Fatalf("partial load cohort allocated: %#v", item)
				}
			}
		})
	}
}

func TestEnergyPathVRFAllocationObservedZeroPoolsAndZeroLoads(t *testing.T) {
	for _, emptyPool := range []bool{false, true} {
		cohort, loads := vrfAllocationFixture(t, 1)
		if emptyPool {
			vrfAllocationObservationFor(t, &cohort, "cooling.vrf.outdoor_electricity", "").Monthly[1] = 0
			vrfAllocationObservationFor(t, &cohort, "cooling.vrf.crankcase_electricity", "").Monthly[1] = 0
		} else {
			for i := range loads {
				if loads[i].ServiceKind == "cooling" {
					loads[i].Monthly[1] = 0
				}
			}
		}
		plan := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads)
		record := vrfAllocationService(t, plan, "cooling", 1)
		if !record.SharedKnown || !record.LoadKnown || !record.UnassignedKnown || record.AllocatedValue != 0 {
			t.Fatalf("known zero boundary lost: %#v", record)
		}
		if emptyPool {
			if !record.AllocatedKnown || record.UnassignedValue != 0 || !vrfAllocationZone(t, plan, "cooling", "SPACE1-1", 1).TotalKnown {
				t.Fatal("known-zero pool became unknown or fabricated consumption")
			}
		} else if record.AllocatedKnown || record.UnassignedValue != 110 {
			t.Fatal("positive pool was distributed without positive service load")
		}
	}
}

func TestEnergyPathVRFAllocationOriginalSourcePrecisionAndTrace(t *testing.T) {
	cohort, loads := vrfAllocationFixture(t, 1)
	main := vrfAllocationObservationFor(t, &cohort, "cooling.vrf.outdoor_electricity", "")
	main.Monthly[1] = .0004
	main.Source.RawValue, main.Source.EffectiveValue, main.Source.EffectiveMultiplier = .0004, .0004, 10
	vrfAllocationObservationFor(t, &cohort, "cooling.vrf.crankcase_electricity", "").Monthly[1] = 0
	before, _ := json.Marshal(cohort)
	plan := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads)
	after, _ := json.Marshal(cohort)
	if string(before) != string(after) {
		t.Fatal("allocator mutated original observations")
	}
	zone := vrfAllocationZone(t, plan, "cooling", "SPACE1-1", 1)
	vrfAllocationNear(t, zone.AllocatedValue, .0004/15)
	if zone.AllocatedValue == 0 || len(zone.ConsumptionSourceIDs) != 3 || len(zone.LoadSourceIDs) != 1 || len(zone.WeightSourceIDs) != 5 {
		t.Fatalf("precision or source roles collapsed: %#v", zone)
	}
	for _, id := range zone.ConsumptionSourceIDs {
		if strings.HasPrefix(id, "load/") {
			t.Fatal("weight source laundered into consumption identity")
		}
	}
	found := 0
	for _, trace := range plan.Sources {
		if trace.Target.Definition.ID != "cooling.vrf.outdoor_electricity" || trace.ZoneName != "SPACE1-1" {
			continue
		}
		found++
		if !trace.Shared || !trace.ObservedKnown || !trace.AllocatedKnown || trace.Target.ZoneName != "" || trace.Source.ZoneName != "" || trace.Source.ID != main.Source.ID || len(trace.LoadSourceIDs) != 5 {
			t.Fatalf("shared trace missing original identity/blank ownership: %#v", trace)
		}
		vrfAllocationNear(t, trace.ObservedValue, .0004)
		vrfAllocationNear(t, trace.AllocatedValue, .0004/15)
		if trace.Source.EffectiveMultiplier != 10 || trace.Source.EffectiveValue != .0004 {
			t.Fatal("original metadata changed or model-total multiplied twice")
		}
	}
	if found != 2 {
		t.Fatalf("monthly+Annual exact source traces=%d, want2", found)
	}
}

func TestEnergyPathVRFAllocationIdentityAndRequestGuards(t *testing.T) {
	for _, kind := range []string{"unrequested", "invalid_source", "hourly", "wrong_unit", "wrong_source_owner", "wrong_original_owner", "missing_original_target", "duplicate_system", "missing_axis"} {
		t.Run(kind, func(t *testing.T) {
			cohort, loads := vrfAllocationFixture(t, 1)
			main := vrfAllocationObservationFor(t, &cohort, "cooling.vrf.outdoor_electricity", "")
			cohorts := []energyPathVRFConsumptionCohort{}
			switch kind {
			case "unrequested":
				cohort.Requested = false
				for i := range cohort.Observations {
					cohort.Observations[i].Requested = false
				}
			case "invalid_source":
				main.Invalid = true
			case "hourly":
				main.Source.ReportingFrequency = "Hourly"
			case "wrong_unit":
				main.Source.SourceUnit = "W"
			case "wrong_source_owner":
				main.Source.ZoneName = "SPACE1-1"
			case "wrong_original_owner":
				main.Target.ZoneName = "SPACE1-1"
			case "missing_original_target":
				cohort.System.Targets = cohort.System.Targets[1:]
			case "duplicate_system":
				cohorts = append(cohorts, cohort)
			case "missing_axis":
				cohort.Months = nil
			}
			cohorts = append(cohorts, cohort)
			plan := buildEnergyPathVRFAllocationPlan(cohorts, loads)
			for _, item := range plan.Zones {
				if item.ServiceKind == "cooling" && (item.SharedKnown || item.AllocatedKnown || item.TotalKnown || item.AllocatedValue != 0) {
					t.Fatalf("invalid identity/request was accepted: %#v", item)
				}
			}
			if len(plan.Zones) == 0 && len(plan.Issues) == 0 {
				t.Fatal("invalid original roster disappeared without an explicit issue")
			}
		})
	}
}

func TestEnergyPathVRFAllocationPermutationAndSeparateSystems(t *testing.T) {
	cohort, loads := vrfAllocationFixture(t, 3)
	forward := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads)
	for left, right := 0, len(cohort.Observations)-1; left < right; left, right = left+1, right-1 {
		cohort.Observations[left], cohort.Observations[right] = cohort.Observations[right], cohort.Observations[left]
	}
	for left, right := 0, len(cohort.System.Targets)-1; left < right; left, right = left+1, right-1 {
		cohort.System.Targets[left], cohort.System.Targets[right] = cohort.System.Targets[right], cohort.System.Targets[left]
	}
	for left, right := 0, len(loads)-1; left < right; left, right = left+1, right-1 {
		loads[left], loads[right] = loads[right], loads[left]
	}
	reverse := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{cohort}, loads)
	if !reflect.DeepEqual(forward, reverse) {
		t.Fatal("permutation changed exact quantities/provenance")
	}
	other, otherLoads := vrfAllocationFixture(t, 3)
	other.System.OutdoorUnit.ID += "/other"
	other.System.OutdoorUnit.ObjectName += " OTHER"
	for i := range other.System.Terminals {
		other.System.Terminals[i].ZoneName += " OTHER"
		other.System.Terminals[i].Terminal.ObjectName += " OTHER"
	}
	for i := range other.System.Targets {
		other.System.Targets[i].KeyValue += " OTHER"
		if other.System.Targets[i].ZoneName != "" {
			other.System.Targets[i].ZoneName += " OTHER"
		}
	}
	for i := range other.Observations {
		item := &other.Observations[i]
		item.Target.KeyValue += " OTHER"
		item.Source.KeyValue += " OTHER"
		item.Source.ID += "/other"
		if item.Target.ZoneName != "" {
			item.Target.ZoneName += " OTHER"
			item.Source.ZoneName += " OTHER"
		}
		for month, value := range item.Monthly {
			item.Monthly[month] = value * 2
		}
	}
	for i := range otherLoads {
		otherLoads[i].ZoneName += " OTHER"
		otherLoads[i].SourceIDs = []string{otherLoads[i].SourceIDs[0] + "/other"}
	}
	combined := buildEnergyPathVRFAllocationPlan([]energyPathVRFConsumptionCohort{other, cohort}, append(loads, otherLoads...))
	if len(combined.Zones) != 80 || len(combined.Issues) != 0 {
		t.Fatalf("independent systems merged/vanished: %#v", combined.Issues)
	}
	vrfAllocationNear(t, vrfAllocationZone(t, combined, "cooling", "SPACE1-1", 0).TotalValue, 308.0/3)
	vrfAllocationNear(t, vrfAllocationZone(t, combined, "cooling", "SPACE1-1 OTHER", 0).TotalValue, 616.0/3)
}
