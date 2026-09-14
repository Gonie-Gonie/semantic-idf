package simulation

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func epathSQLPoolHeatingServiceHand(t *testing.T) (epathSQLFrames, epathRealSQLModel, epathRealSQLService) {
	t.Helper()
	native := epathSQLPoolBoundaryHandNative(t)
	frames := epathSQLFrames{PoolSystems: []epathSQLPoolSourceFrames{native}, Zones: map[string]epathSQLZone{}, Loads: map[string]epathSQLQuantity{}, Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{}, SourceRaw: map[int][]epathSQLQuantity{}, SourceEffective: map[int][]epathSQLQuantity{}}
	service := epathRealSQLService{Service: "heating", SiteIDs: []string{"heating.electricity", "heating.natural_gas"}, ServedZones: append([]string(nil), native.Original.ServedZones...), Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "load_to_site_energy", FallbackRatioKind: "load_to_site_energy", CarrierReconciliationIDs: map[string]string{"electricity": "reconcile.zone_hvac_allocation.heating.electricity.annual", "natural_gas": "reconcile.zone_hvac_allocation.heating.natural_gas.annual"}}
	model := epathRealSQLModel{Precision: native.Precision, OriginalZoneMultiplierProof: epathSQLOriginalMultiplierContract, PoolSystems: []epathRealSQLPoolSystem{native.Original.Declaration}, Services: []epathRealSQLService{service}}
	for _, name := range append(append([]string(nil), native.Original.ServedZones...), native.Original.Declaration.ReturnPlenumZoneName) {
		key := strings.ToLower(name)
		frames.Zones[key] = epathSQLZone{Name: name, Multiplier: 3}
		for month := 1; month <= 12; month++ {
			frames.Loads[epathSQLKey(key, "heating", month)] = epathSQLQuantity{Value: 30}
		}
	}
	for _, carrier := range []string{"electricity", "natural_gas"} {
		name := "Heating:Electricity"
		if carrier == "natural_gas" {
			name = "Heating:NaturalGas"
		}
		parent := native.Parents[name]
		id := "heating." + carrier
		model.Site = append(model.Site, epathRealSQLSite{ID: id, EndUse: "heating", Carrier: carrier, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: name, Unit: "J"}}, Keys: []string{""}, IsMeter: true}})
		for _, month := range parent.Monthly {
			q := month
			frames.Site[id] = append(frames.Site[id], &q)
		}
		index := parent.Source.DictionaryIndex
		frames.SiteSources[id] = []int{index}
		frames.SourceIdentities[index] = parent.Source
		frames.SourceRaw[index] = append([]epathSQLQuantity(nil), parent.Monthly...)
		frames.SourceEffective[index] = append([]epathSQLQuantity(nil), parent.Monthly...)
	}
	return frames, model, service
}

func TestEnergyPathRealSQLPoolHeatingServiceObservedParentsUnassignedNotMissing(t *testing.T) {
	frames, model, service := epathSQLPoolHeatingServiceHand(t)
	var checks epathSQLModelChecks
	if err := epathSQLPoolHeatingBuildingServiceChecks(frames, model, service, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 13*9 {
		t.Fatalf("Building Heating check census=%d", len(checks.Rows))
	}
	for _, check := range checks.Rows {
		if check.PoolBoundary == nil || check.PoolBoundary.Scope != "building" || check.PoolBoundary.Period != check.Item.Period {
			t.Fatal("missing exact native Building boundary")
		}
		if check.Item.Group == "ratios" {
			if check.Quantity != nil || check.Want.Value != nil || check.Want.Status != epathSQLPoolHeatingUnquantified || check.Conversion != nil || check.Item.Target.Basis != "" || check.Item.Target.RatioKind != "load_to_site_energy" {
				t.Fatal("semantic denial was mislabeled unavailable or fabricated as Conversion{0,0}")
			}
			continue
		}
		if check.Allocation == nil || check.Quantity == nil || check.Want.Value == nil || check.Want.Status != "" {
			t.Fatal("actual native parent was changed into unavailable SQL")
		}
		carrier := "electricity"
		if strings.Contains(check.Item.Target.ID, "natural_gas") {
			carrier = "natural_gas"
		}
		want := 0.0
		if carrier == "natural_gas" && (check.Item.Target.Field == "expectedValue" || check.Item.Target.Field == "unassignedValue") {
			// B's native hand observations are a constant 4 kW gas parent.
			for _, month := range epathSQLPeriodMonths(check.Item.Period) {
				want += frames.PoolSystems[0].Parents["Heating:NaturalGas"].Monthly[month-1].Value
			}
		}
		if check.Quantity.Value != want {
			t.Fatalf("Building source-local %s=%g want%g", check.Item.Key, check.Quantity.Value, want)
		}
	}
	if err := epathSQLPoolHeatingZoneServiceChecks(frames, model, service, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 13*9+6*13*3 {
		t.Fatalf("combined Heating check census=%d", len(checks.Rows))
	}
	for _, check := range checks.Rows {
		if check.Item.Scope != "zone" {
			continue
		}
		if check.Item.Group == "ratios" {
			if check.Want.Status != epathSQLPoolHeatingUnquantified || check.Quantity != nil || check.Conversion != nil {
				t.Fatal("Zone semantic ratio denial became a missing source or a zero ratio")
			}
			continue
		}
		if check.Quantity == nil || !epathSQLZoneCarrierQuantityEqual(*check.Quantity, epathSQLQuantity{}) || check.Want.Status != "" {
			t.Fatal("Zone allocation is not exact zero")
		}
		if check.Item.Target.Field == "value" && (check.ZoneService == nil || check.ZoneService.NativePool == nil || check.ZoneService.Unavailable || check.ZoneService.Unowned || len(check.ZoneService.Carriers) != 2) {
			t.Fatal("Zone carrier aggregator lost its typed known-zero Heating component")
		}
	}
	parts, carriers, err := epathSQLZoneCarrierInputs(frames, model, checks)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(carriers, []string{"electricity", "natural_gas"}) || len(parts) != 6*13 {
		t.Fatalf("native zero carrier census differs: carriers=%v contexts=%d", carriers, len(parts))
	}
	for _, families := range parts {
		part, found := families["service/heating"]
		if !found || part.Unavailable || len(part.ByCarrier) != 2 {
			t.Fatal("required Heating carrier0 family was dropped")
		}
		for _, q := range part.ByCarrier {
			if !epathSQLZoneCarrierQuantityEqual(q, epathSQLQuantity{}) {
				t.Fatal("native parent leaked into a Zone carrier")
			}
		}
	}
}

func epathSQLPoolHeatingPrunedBundle() PurposeResultBundle {
	bundle := epathSQLPoolBoundaryHandBundle()
	prune := func(nodes []EnergyExplanationNode, links []EnergyPathLink) ([]EnergyExplanationNode, []EnergyPathLink) {
		removed := map[string]bool{}
		outNodes := []EnergyExplanationNode{}
		outLinks := []EnergyPathLink{}
		for _, node := range nodes {
			if node.Level == "end_use" && node.EndUse == "heating" || node.Level == "carrier" && node.Carrier == "natural_gas" {
				removed[node.ID] = true
				continue
			}
			outNodes = append(outNodes, node)
		}
		for _, link := range links {
			if !removed[link.FromID] && !removed[link.ToID] {
				outLinks = append(outLinks, link)
			}
		}
		return outNodes, outLinks
	}
	for i := range bundle.EnergyExplanation.ZoneResults {
		z := &bundle.EnergyExplanation.ZoneResults[i]
		z.Nodes, z.Links = prune(z.Nodes, z.Links)
		z.Summary.EndUses = nil
		for p := range z.Periods {
			period := &z.Periods[p]
			period.Nodes, period.Links = prune(period.Nodes, period.Links)
			period.Summary.EndUses = nil
		}
	}
	return bundle
}

func epathSQLPoolHeatingSelectedCheck(t *testing.T, frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService, period string) epathSQLModelCheck {
	t.Helper()
	var checks epathSQLModelChecks
	if err := epathSQLPoolHeatingZoneServiceChecks(frames, model, service, &checks); err != nil {
		t.Fatal(err)
	}
	for _, check := range checks.Rows {
		if check.Item.Zone == "SPACE1-1" && check.Item.Period == period && check.ZoneService != nil {
			return check
		}
	}
	t.Fatal("missing hand Pool Heating check")
	return epathSQLModelCheck{}
}

func TestEnergyPathRealSQLPoolHeatingZoneConsumerPrunedOrZeroButNeverBorrowed(t *testing.T) {
	frames, model, service := epathSQLPoolHeatingServiceHand(t)
	check := epathSQLPoolHeatingSelectedCheck(t, frames, model, service, "annual")
	bundle := epathSQLPoolHeatingPrunedBundle()
	before := epathSQLPoolHeatingPrunedBundle()
	if err := epathCheckSQLPoolHeatingZoneService(bundle, check); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bundle, before) {
		t.Fatal("Pool Heating consumer edited Building energy or distinct CW/fan/Cooling quantities")
	}
	for _, value := range []float64{0, 1, -1} {
		t.Run(fmt.Sprintf("present=%g", value), func(t *testing.T) {
			b := epathSQLPoolHeatingPrunedBundle()
			z := &b.EnergyExplanation.ZoneResults[0]
			z.Nodes = append(z.Nodes, EnergyExplanationNode{ID: "end_use.heating.space1-1", Level: "end_use", Kind: "end_use.heating", EndUse: "heating", Value: value, Unit: "kWh", ScaleDomain: "site", Period: "annual", ZoneName: "SPACE1-1", Basis: "invented_direct"})
			err := epathCheckSQLPoolHeatingZoneService(b, check)
			if (err == nil) != (value == 0) {
				t.Fatalf("present zero/pruned contract accepted %g or rejected zero: %v", value, err)
			}
		})
	}
	for name, mutate := range map[string]func(*PurposeResultBundle){
		"positive annual alias": func(b *PurposeResultBundle) {
			z := &b.EnergyExplanation.ZoneResults[0]
			z.Periods[0].Nodes = append(z.Periods[0].Nodes, EnergyExplanationNode{ID: "end_use.heating.alias", Level: "end_use", EndUse: "heating", Value: 5, Unit: "kWh", ScaleDomain: "site", Period: "annual", ZoneName: "SPACE1-1"})
		},
		"ratio without site input": func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Summary.Ratios = append(b.EnergyExplanation.ZoneResults[0].Summary.Ratios, EnergyExplanationSummaryItem{ID: "kpi.heating_cop", Value: 3})
		},
	} {
		t.Run(name, func(t *testing.T) {
			b := epathSQLPoolHeatingPrunedBundle()
			mutate(&b)
			if err := epathCheckSQLPoolHeatingZoneService(b, check); err == nil {
				t.Fatal("unproved Heating alias or ratio escaped")
			}
		})
	}
}

func TestEnergyPathRealSQLPoolHeatingServiceRejectsForeignOrMissingEvidence(t *testing.T) {
	for name, mutate := range map[string]func(*epathSQLFrames, *epathRealSQLModel, *epathRealSQLService){
		"no original declaration": func(_ *epathSQLFrames, m *epathRealSQLModel, _ *epathRealSQLService) { m.PoolSystems = nil },
		"foreign parent Zone": func(f *epathSQLFrames, _ *epathRealSQLModel, _ *epathRealSQLService) {
			f.SourceZone = map[int]string{f.PoolSystems[0].Parents["Heating:NaturalGas"].Source.DictionaryIndex: "space1-1"}
		},
		"no native system": func(f *epathSQLFrames, _ *epathRealSQLModel, _ *epathRealSQLService) { f.PoolSystems = nil },
		"duplicate native system": func(f *epathSQLFrames, _ *epathRealSQLModel, _ *epathRealSQLService) {
			f.PoolSystems = append(f.PoolSystems, f.PoolSystems[0])
		},
		"wrong service":   func(_ *epathSQLFrames, _ *epathRealSQLModel, s *epathRealSQLService) { s.Service = "cooling" },
		"missing carrier": func(_ *epathSQLFrames, _ *epathRealSQLModel, s *epathRealSQLService) { s.SiteIDs = s.SiteIDs[:1] },
		"aggregate ledger replacing carriers": func(_ *epathSQLFrames, _ *epathRealSQLModel, s *epathRealSQLService) {
			s.ReconciliationID = "all.annual"
		},
		"wrong carrier ledger": func(_ *epathSQLFrames, _ *epathRealSQLModel, s *epathRealSQLService) {
			s.CarrierReconciliationIDs["natural_gas"] = "reconcile.zone_hvac_allocation.heating.electricity.annual"
		},
		"plenum as served": func(_ *epathSQLFrames, _ *epathRealSQLModel, s *epathRealSQLService) { s.ServedZones[0] = "PLENUM-1" },
		"missing canonical plenum load": func(f *epathSQLFrames, _ *epathRealSQLModel, _ *epathRealSQLService) {
			delete(f.Loads, epathSQLKey("plenum-1", "heating", 1))
		},
		"zero used for missing native parent": func(f *epathSQLFrames, _ *epathRealSQLModel, _ *epathRealSQLService) {
			f.Site["heating.natural_gas"][0] = nil
		},
		"gas multiplied again": func(f *epathSQLFrames, _ *epathRealSQLModel, _ *epathRealSQLService) {
			q := f.Site["heating.natural_gas"][0].times(3)
			f.Site["heating.natural_gas"][0] = &q
		},
		"wrong native dictionary": func(f *epathSQLFrames, _ *epathRealSQLModel, _ *epathRealSQLService) {
			f.SiteSources["heating.natural_gas"] = f.SiteSources["heating.electricity"]
		},
		"missing original multiplier proof": func(_ *epathSQLFrames, m *epathRealSQLModel, _ *epathRealSQLService) {
			m.OriginalZoneMultiplierProof = ""
		},
		"native zero allowed absent": func(_ *epathSQLFrames, m *epathRealSQLModel, _ *epathRealSQLService) {
			m.Site[0].Source.AllowAbsent = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			f, m, s := epathSQLPoolHeatingServiceHand(t)
			mutate(&f, &m, &s)
			if _, err := epathSQLCompilePoolHeatingService(f, m, s); err == nil {
				t.Fatal("foreign/missing native Heating evidence granted a semantic exception")
			}
		})
	}
}

func TestEnergyPathRealSQLPoolHeatingZoneTypedProofCannotBecomeUnavailableOrAllocate(t *testing.T) {
	frames, model, service := epathSQLPoolHeatingServiceHand(t)
	for name, mutate := range map[string]func(*epathSQLModelCheck){
		"missing native boundary":             func(c *epathSQLModelCheck) { c.ZoneService.NativePool = nil },
		"missing carrier component":           func(c *epathSQLModelCheck) { delete(c.ZoneService.Carriers, "electricity") },
		"positive carrier component":          func(c *epathSQLModelCheck) { c.ZoneService.Carriers["natural_gas"] = epathSQLQuantity{Value: 1} },
		"tiny nonzero carrier":                func(c *epathSQLModelCheck) { c.ZoneService.Carriers["natural_gas"] = epathSQLQuantity{Value: 1e-12} },
		"uncertain zero carrier":              func(c *epathSQLModelCheck) { c.ZoneService.Carriers["natural_gas"] = epathSQLQuantity{Error: 1e-12} },
		"unavailable instead of unquantified": func(c *epathSQLModelCheck) { c.ZoneService.Unavailable = true },
		"unowned instead of shared demand":    func(c *epathSQLModelCheck) { c.ZoneService.Unowned = true },
		"wrong selected period":               func(c *epathSQLModelCheck) { c.Item.Period = "M2" },
		"wrong basis target":                  func(c *epathSQLModelCheck) { c.Item.Target.Basis = "zone_load_allocation" },
		"wrong unit":                          func(c *epathSQLModelCheck) { c.Item.Target.Unit = "J" },
	} {
		t.Run(name, func(t *testing.T) {
			check := epathSQLPoolHeatingSelectedCheck(t, frames, model, service, "M1")
			mutate(&check)
			if err := epathCheckSQLPoolHeatingZoneService(epathSQLPoolHeatingPrunedBundle(), check); err == nil {
				t.Fatal("typed Pool Heating zero proof was weakened")
			}
		})
	}
}

func TestEnergyPathRealSQLPoolHeatingCarrierLedgerRetainsNativeAmountAndRejectsBorrowing(t *testing.T) {
	frames, model, service := epathSQLPoolHeatingServiceHand(t)
	var checks epathSQLModelChecks
	if err := epathSQLPoolHeatingBuildingServiceChecks(frames, model, service, &checks); err != nil {
		t.Fatal(err)
	}
	selected := []epathSQLModelCheck{}
	for _, check := range checks.Rows {
		if check.Item.Period == "M1" && check.Allocation != nil {
			selected = append(selected, check)
		}
	}
	if len(selected) != 8 {
		t.Fatal("missing exact carrier-separated four-field checks")
	}
	bundle := epathSQLPoolHeatingPrunedBundle()
	// January has 744 actual hours in the independent 2017 hand weather.
	// Four kW purchased gas is 2976 kWh; its allocation remains unassigned.
	row := EnergyReconciliation{ID: "reconcile.zone_hvac_allocation.heating.natural_gas.m1", Level: "allocation", Period: "M1", ServiceKind: "heating", ExpectedValue: 2976, ResidualValue: 2976, UnassignedValue: 2976, Unit: "kWh", Basis: "service_path_allocation"}
	bundle.EnergyExplanation.Periods[1].Reconciliation = []EnergyReconciliation{row}
	for _, check := range selected {
		if err := epathCheckSQLModelAllocation(bundle, check); err != nil {
			t.Fatalf("native gas retained / exact zero electricity may be pruned: %v", err)
		}
		if err := epathSQLCheckPoolBoundary(bundle, check.PoolBoundary); err != nil {
			t.Fatal(err)
		}
	}
	for name, mutate := range map[string]func(*EnergyReconciliation){
		"borrow pool gas for Zones": func(r *EnergyReconciliation) { r.AllocatedValue = 1; r.UnassignedValue = 2975 },
		"replace gas with zero":     func(r *EnergyReconciliation) { r.ExpectedValue = 0; r.UnassignedValue = 0; r.ResidualValue = 0 },
		"wrong carrier ID":          func(r *EnergyReconciliation) { r.ID = "reconcile.zone_hvac_allocation.heating.electricity.m1" },
		"wrong service":             func(r *EnergyReconciliation) { r.ServiceKind = "cooling" },
	} {
		t.Run(name, func(t *testing.T) {
			b := epathSQLPoolHeatingPrunedBundle()
			bad := row
			mutate(&bad)
			b.EnergyExplanation.Periods[1].Reconciliation = []EnergyReconciliation{bad}
			failed := false
			for _, check := range selected {
				if strings.Contains(check.Item.Target.ID, "natural_gas") && epathCheckSQLModelAllocation(b, check) != nil {
					failed = true
					break
				}
			}
			if !failed {
				t.Fatal("source-local native gas ledger was silently allocated, zeroed or rebound")
			}
		})
	}
}
