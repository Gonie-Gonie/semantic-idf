package simulation

import (
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func epathSQLVRFServiceUnitFrames(t *testing.T) (epathSQLFrames, epathRealSQLModel) {
	t.Helper()
	frames, system := epathSQLVRFAllocationUnitFrames(t)
	frames.Zones["plenum"] = epathSQLZone{Name: "Plenum", Multiplier: 1}
	frames.Site, frames.SiteSources, frames.SourceRaw = map[string][]*epathSQLQuantity{}, map[string][]int{}, map[int][]epathSQLQuantity{}
	model := epathRealSQLModel{Precision: system.Precision, NativeVRFSystems: []epathRealSQLVRFSystem{system.Declaration}}
	for index, service := range []string{"cooling", "heating"} {
		id, name, coefficient := "cooling.electricity", "Cooling:Electricity", 124.0
		if service == "heating" {
			id, name, coefficient = "heating.electricity", "Heating:Electricity", 233
		}
		model.Site = append(model.Site, epathRealSQLSite{ID: id, EndUse: service, Carrier: "electricity", Source: epathRealSQLSelector{IsMeter: true, Keys: []string{""}, Alternatives: []epathRealSQLAlternative{{Name: name, Unit: "J"}}}})
		served := []string{}
		for _, terminal := range system.Declaration.Terminals {
			served = append(served, terminal.ZoneName)
		}
		model.Services = append(model.Services, epathRealSQLService{Service: service, SiteIDs: []string{id}, ServedZones: served,
			Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "coefficient_of_performance", FallbackRatioKind: "coefficient_of_performance", ReconciliationID: "allocation." + service + ".annual"})
		var values [12]float64
		for month := range values {
			values[month] = coefficient * float64(month+1)
		}
		source := epathSQLVRFAllocationUnitSource(index+1, name, "", values)
		source.IsMeter = true
		frames.SourceIdentities[source.DictionaryIndex] = source
		frames.SiteSources[id] = []int{source.DictionaryIndex}
		quantities, err := epathSQLMonthly(source, model.Precision)
		if err != nil {
			t.Fatal(err)
		}
		frames.SourceRaw[source.DictionaryIndex] = quantities
		for _, q := range quantities {
			copy := q
			frames.Site[id] = append(frames.Site[id], &copy)
		}
	}
	frames.NativeVRFSystems = []epathSQLVRFSystemFrame{system}
	epathSQLVRFServiceUnitRecompile(t, &frames)
	return frames, model
}

func epathSQLVRFServiceUnitRecompile(t *testing.T, frames *epathSQLFrames) {
	t.Helper()
	proof, err := epathSQLCompileVRFAllocation(*frames, frames.NativeVRFSystems[0])
	if err != nil {
		t.Fatal(err)
	}
	frames.NativeVRFAllocations = []epathSQLVRFAllocationProof{proof}
}

func epathSQLVRFServiceUnitChecks(t *testing.T, frames epathSQLFrames, model epathRealSQLModel) epathSQLModelChecks {
	t.Helper()
	checks := epathSQLModelChecks{}
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return checks
}

func epathSQLVRFServiceUnitCheck(t *testing.T, checks epathSQLModelChecks, zone, service, period string) epathSQLModelCheck {
	t.Helper()
	for _, check := range checks.Rows {
		if check.ZoneService != nil && check.Item.Zone == zone && check.ZoneService.Service == service && check.Item.Period == period {
			return check
		}
	}
	t.Fatalf("missing service check %s/%s/%s", zone, service, period)
	return epathSQLModelCheck{}
}

// Candidate handgraphs use the explicit proof fixture's public amounts, not a
// production allocator/builder. Literal arithmetic is asserted separately.
func epathSQLVRFServiceUnitCandidate(check epathSQLModelCheck) PurposeResultBundle {
	p := check.ZoneService.NativeVRF
	out := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	byID := map[string]epathSQLOriginalSource{}
	for id, original := range p.Originals["electricity"] {
		byID[id] = original
	}
	for id, original := range p.Weights {
		byID[id] = original
	}
	keys := []string{}
	for id := range byID {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		original := byID[id]
		s := original.RDD
		actual := EnergyDataSource{ID: id, SourceType: "sql_report_data", Name: s.Name, KeyValue: s.KeyValue, Units: "J", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", AggregationMethod: "sum", AggregationBasis: "model_total", EffectiveMultiplier: 1, MultiplierApplication: "already_model_total"}
		if original.NativeVRF != nil {
			actual.ZoneName = original.NativeVRF.Observation.ZoneName
			actual.ObjectIndex = original.NativeVRF.Observation.RequestObjectIndex
		}
		out.EnergyExplanation.Sources = append(out.EnergyExplanation.Sources, actual)
	}
	ids := []string{}
	for id := range p.Originals["electricity"] {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	nodes, links := []EnergyExplanationNode{}, []EnergyPathLink{}
	if p.Value.Value > 0 {
		nodes = append(nodes, EnergyExplanationNode{ID: "use", Level: "end_use", EndUse: p.Service, Value: p.Value.Value, EffectiveValue: p.Value.Value, AllocatedValue: p.Value.Value, AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", AggregationBasis: "model_total", Multiplier: 1, ZoneName: p.ZoneName, Period: p.Period, SourceIDs: append([]string(nil), ids...)},
			EnergyExplanationNode{ID: "electricity", Level: "carrier", Carrier: "electricity", Value: p.Value.Value, Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", AggregationBasis: "model_total", ZoneName: p.ZoneName, Period: p.Period})
		for relation, branch := range p.Branches {
			if branch.Value.Value == 0 {
				continue
			}
			links = append(links, EnergyPathLink{ID: relation, FromID: "use", ToID: "electricity", Relation: relation, Basis: "service_path_allocation", ServiceKind: p.Service, FromValue: branch.Value.Value, ToValue: branch.Value.Value, FromUnit: "kWh", ToUnit: "kWh", ZoneName: p.ZoneName, Period: p.Period, SourceIDs: append([]string(nil), ids...)})
		}
	}
	if p.PairFrom.Value > 0 && p.PairTo.Value > 0 {
		nodes = append(nodes, EnergyExplanationNode{ID: "load", Level: "load", ServiceKind: p.Service, Value: p.PairFrom.Value, Unit: "kWh", ScaleDomain: "thermal", ZoneName: p.ZoneName, Period: p.Period})
		links = append(links, EnergyPathLink{ID: "conversion", FromID: "load", ToID: "use", Relation: "load_to_end_use", Basis: "service_path_allocation", ServiceKind: p.Service, FromValue: p.PairFrom.Value, ToValue: p.PairTo.Value, FromUnit: "kWh", ToUnit: "kWh", ZoneName: p.ZoneName, Period: p.Period, SourceIDs: append([]string(nil), keys...), Ratio: p.PairFrom.Value / p.PairTo.Value, RatioKind: "coefficient_of_performance"})
	}
	zone := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: p.ZoneName}}
	if p.Period == "annual" {
		zone.Nodes, zone.Links = nodes, links
	} else {
		zone.Periods = []EnergyPeriod{{ID: p.Period, Nodes: nodes, Links: links}}
	}
	out.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{zone}
	return out
}

func epathSQLVRFServiceUnitGraph(bundle *PurposeResultBundle, period string) (*[]EnergyExplanationNode, *[]EnergyPathLink) {
	z := &bundle.EnergyExplanation.ZoneResults[0]
	if period == "annual" {
		return &z.Nodes, &z.Links
	}
	return &z.Periods[0].Nodes, &z.Periods[0].Links
}

func TestEnergyPathRealSQLVRFServiceExactLocalSharedAndBroadLedger(t *testing.T) {
	frames, model := epathSQLVRFServiceUnitFrames(t)
	checks := epathSQLVRFServiceUnitChecks(t, frames, model)
	if len(checks.Rows) != 6*2*13*4 {
		t.Fatalf("missing context/field roster: %d", len(checks.Rows))
	}
	month := epathSQLVRFServiceUnitCheck(t, checks, "SPACE1-1", "cooling", "M1")
	if month.Quantity.Value != 7.334 || month.ZoneService.NativeVRF.Direct.Value != 0 || month.ZoneService.NativeVRF.PairFrom.Value != 10 {
		t.Fatalf("wrong hand source-wise 100/15 + 10/15 remainder allocation: %#v", month.ZoneService.NativeVRF)
	}
	annual := epathSQLVRFServiceUnitCheck(t, checks, "SPACE1-1", "cooling", "annual")
	if annual.Quantity.Value != 1804 {
		t.Fatalf("annual is not independently summed completed-month budgets: %v", annual.Quantity.Value)
	}
	for _, check := range checks.Rows {
		if check.ZoneService == nil {
			continue
		}
		bundle := epathSQLVRFServiceUnitCandidate(check)
		if err := epathCheckSQLZoneServiceEndpoints(bundle, check); err != nil {
			t.Fatalf("%s: %v", check.Want.Key, err)
		}
	}
	var building epathSQLModelChecks
	if err := epathSQLModelServiceChecks(frames, model, &building); err != nil {
		t.Fatal(err)
	}
	values := map[string]float64{}
	for _, check := range building.Rows {
		if check.Item.Period == "M1" && strings.HasPrefix(check.Item.Target.ID, "allocation.cooling.") {
			values[check.Item.Target.Field] = check.Quantity.Value
		}
		if check.Conversion != nil && check.Item.Period == "M1" && check.Item.Target.Service == "cooling" && check.Item.Target.Basis == "service_path_allocation" && (check.Conversion.From.Value != 150 || check.Conversion.To.Value != 124) {
			t.Fatalf("Building broad pair was replaced by a Zone component amount: %#v", check.Conversion)
		}
	}
	if !reflect.DeepEqual(values, map[string]float64{"expectedValue": 124, "directValue": 14, "allocatedValue": 110, "unassignedValue": 0}) {
		t.Fatalf("broad meter was doubled or direct/shared conflated: %#v", values)
	}
}

func TestEnergyPathRealSQLVRFServiceRejectsTraceAndEndpointMutants(t *testing.T) {
	frames, model := epathSQLVRFServiceUnitFrames(t)
	checks := epathSQLVRFServiceUnitChecks(t, frames, model)
	check := epathSQLVRFServiceUnitCheck(t, checks, "SPACE1-1", "cooling", "M1")
	for _, name := range []string{"node source removed", "node source moved to conversion", "missing source record", "foreign owner source", "missing denominator source", "consumption duplicate split", "wrong load endpoint", "wrong domain", "wrong effective", "wrong carrier", "wrong aggregation", "double multiplier", "missing conversion", "wrong consumption value", "consumption weight injection", "derived source wrapper"} {
		t.Run(name, func(t *testing.T) {
			bundle := epathSQLVRFServiceUnitCandidate(check)
			nodes, links := epathSQLVRFServiceUnitGraph(&bundle, "M1")
			find := func(relation string) int {
				for i, l := range *links {
					if l.Relation == relation {
						return i
					}
				}
				t.Fatal("missing hand link")
				return -1
			}
			switch name {
			case "node source removed", "node source moved to conversion":
				(*nodes)[0].SourceIDs = (*nodes)[0].SourceIDs[1:]
			case "missing source record":
				bundle.EnergyExplanation.Sources = bundle.EnergyExplanation.Sources[1:]
			case "foreign owner source":
				for i := range bundle.EnergyExplanation.Sources {
					if bundle.EnergyExplanation.Sources[i].ZoneName != "" {
						bundle.EnergyExplanation.Sources[i].ZoneName = "SPACE2-1"
						break
					}
				}
			case "missing denominator source":
				i := find("load_to_end_use")
				(*links)[i].SourceIDs = (*links)[i].SourceIDs[:len((*links)[i].SourceIDs)-1]
			case "consumption duplicate split":
				i := find("end_use_to_carrier")
				duplicate := (*links)[i]
				(*links)[i].FromValue /= 2
				(*links)[i].ToValue /= 2
				duplicate.FromValue = (*links)[i].FromValue
				duplicate.ToValue = (*links)[i].ToValue
				duplicate.ID = "other-rule"
				duplicate.RuleID = "different words"
				*links = append(*links, duplicate)
			case "wrong load endpoint":
				for i := range *nodes {
					if (*nodes)[i].ID == "load" {
						(*nodes)[i].ServiceKind = "heating"
					}
				}
			case "wrong domain":
				(*nodes)[0].ScaleDomain = "thermal"
			case "wrong effective":
				(*nodes)[0].EffectiveValue = 999
			case "wrong carrier":
				(*nodes)[0].Carrier = "natural_gas"
			case "wrong aggregation":
				(*nodes)[0].AggregationBasis = "zone_equivalent"
			case "double multiplier":
				(*nodes)[0].Multiplier = 10
			case "missing conversion":
				i := find("load_to_end_use")
				*links = append((*links)[:i], (*links)[i+1:]...)
			case "wrong consumption value":
				i := find("end_use_to_carrier")
				(*links)[i].FromValue++
				(*links)[i].ToValue++
			case "consumption weight injection":
				i := find("end_use_to_carrier")
				for id := range check.ZoneService.NativeVRF.Weights {
					(*links)[i].SourceIDs = append((*links)[i].SourceIDs, id)
					break
				}
			case "derived source wrapper":
				bundle.EnergyExplanation.Sources[0].InputSourceIDs = []string{"another"}
			}
			if err := epathCheckSQLZoneServiceEndpoints(bundle, check); err == nil {
				t.Fatal("mutant passed exact VRF service proof")
			}
		})
	}
}

func TestEnergyPathRealSQLVRFServiceZeroLoadAndUnownedAbsence(t *testing.T) {
	frames, model := epathSQLVRFServiceUnitFrames(t)
	var load [12]float64
	for month := range load {
		load[month] = frames.Loads[epathSQLKey("SPACE2-1", "cooling", month+1)].Value
	}
	load[0] = 0
	epathSQLVRFAllocationUnitReplaceLoad(&frames, "SPACE2-1", "cooling", load)
	epathSQLVRFServiceUnitRecompile(t, &frames)
	checks := epathSQLVRFServiceUnitChecks(t, frames, model)
	check := epathSQLVRFServiceUnitCheck(t, checks, "SPACE2-1", "cooling", "M1")
	if check.Quantity.Value != 2 || check.ZoneService.NativeVRF.PairFrom.Value != 0 || check.ZoneService.NativeVRF.PairTo.Value != 0 {
		t.Fatalf("zero load erased local2 or invented a bridge: %#v", check.ZoneService.NativeVRF)
	}
	bundle := epathSQLVRFServiceUnitCandidate(check)
	if err := epathCheckSQLZoneServiceEndpoints(bundle, check); err != nil {
		t.Fatal(err)
	}
	annual := epathSQLVRFServiceUnitCheck(t, checks, "SPACE2-1", "cooling", "annual")
	if math.Abs(annual.Quantity.Value-annual.ZoneService.NativeVRF.PairTo.Value-2) > 1e-10 {
		t.Fatal("Annual paired energy included the unpaired local-only month")
	}
	annualBundle := epathSQLVRFServiceUnitCandidate(annual)
	if err := epathCheckSQLZoneServiceEndpoints(annualBundle, annual); err != nil {
		t.Fatal(err)
	}
	_, annualLinks := epathSQLVRFServiceUnitGraph(&annualBundle, "annual")
	for i := range *annualLinks {
		if (*annualLinks)[i].Relation == "load_to_end_use" {
			(*annualLinks)[i].ToValue = annual.Quantity.Value
		}
	}
	if err := epathCheckSQLZoneServiceEndpoints(annualBundle, annual); err == nil {
		t.Fatal("Annual total consumption was substituted for completed paired months")
	}
	for _, period := range []string{"M1", "annual"} {
		plenum := epathSQLVRFServiceUnitCheck(t, checks, "Plenum", "cooling", period)
		candidate := epathSQLVRFServiceUnitCandidate(plenum)
		if err := epathCheckSQLZoneServiceEndpoints(candidate, plenum); err != nil {
			t.Fatal(err)
		}
		nodes, _ := epathSQLVRFServiceUnitGraph(&candidate, period)
		*nodes = append(*nodes, EnergyExplanationNode{ID: "invented", Level: "end_use", EndUse: "cooling", Value: 0, Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", ZoneName: "Plenum", Period: period})
		if err := epathCheckSQLZoneServiceEndpoints(candidate, plenum); err == nil {
			t.Fatal("unowned zero HVAC node was laundered into a known observation")
		}
	}
}

func TestEnergyPathRealSQLVRFServiceRejectsChangedOriginalObligations(t *testing.T) {
	for _, name := range []string{"broad mismatch", "broad subdisplay mismatch", "extra owner", "foreign site", "missing month", "forged allocation"} {
		t.Run(name, func(t *testing.T) {
			frames, model := epathSQLVRFServiceUnitFrames(t)
			switch name {
			case "broad mismatch":
				frames.Site["cooling.electricity"][0].Value++
			case "broad subdisplay mismatch":
				frames.Site["cooling.electricity"][0].Value += .0006
			case "extra owner":
				model.Services[0].ServedZones = append(model.Services[0].ServedZones, "Plenum")
			case "foreign site":
				model.Site[0].Carrier = "natural_gas"
			case "missing month":
				delete(frames.NativeVRFAllocations[0].Months, 1)
			case "forged allocation":
				m := frames.NativeVRFAllocations[0].Months[1]
				r := m.Zones["SPACE1-1"]["cooling"]
				r.AllocatedMilliKWh++
				m.Zones["SPACE1-1"]["cooling"] = r
			}
			if _, err := epathSQLCompileVRFService(frames, model, model.Services[0]); err == nil {
				t.Fatal("changed original obligation passed")
			}
		})
	}
}

func TestEnergyPathRealSQLVRFServiceEmptyModelKeepsExistingChecks(t *testing.T) {
	frames, model := epathSQLZoneServiceUnitFrames()
	var before, after epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &before); err != nil {
		t.Fatal(err)
	}
	model.NativeVRFSystems = []epathRealSQLVRFSystem{}
	if err := epathSQLModelZoneServiceChecks(frames, model, &after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("empty native declaration changed prior metric identities or quantities")
	}
	if compiled, err := epathSQLCompileVRFService(frames, model, model.Services[0]); err != nil || compiled != nil {
		t.Fatalf("empty native path must be a no-op: %v", err)
	}
}

func TestEnergyPathRealSQLVRFServiceExactIntegerDisplay(t *testing.T) {
	q := epathSQLQuantity{Value: 1000000}
	if err := epathSQLVRFCheckDisplay(q.Value, q); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLVRFCheckDisplay(q.Value, q.add(epathSQLQuantity{})); err != nil {
		t.Fatal("an exact point interval became unknown:", err)
	}
	if err := epathSQLVRFCheckDisplay(q.Value, epathSQLQuantity{Value: q.Value, Error: .001}); err == nil {
		t.Fatal("a non-point interval was accepted as an exact display budget")
	}
	for _, delta := range []float64{.001, -.001, .0001} {
		if err := epathSQLVRFCheckDisplay(q.Value+delta, q); err == nil {
			t.Fatalf("large magnitude swallowed display mutation %g", delta)
		}
	}
}
