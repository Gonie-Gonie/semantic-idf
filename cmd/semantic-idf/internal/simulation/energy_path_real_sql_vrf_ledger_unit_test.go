package simulation

import (
	"fmt"
	"testing"
)

func epathSQLVRFLedgerUnitAnnual(t *testing.T) epathSQLVRFAllocationLedgerProof {
	t.Helper()
	frames, model := epathSQLVRFServiceUnitFrames(t)
	// Genuine independent source arithmetic: all months have raw total
	// 1.0004, while the original source-wise rounded totals alternate 1.001
	// and .999. Five local source budgets total .400 in both cases.
	var local, main, crank, meter [12]float64
	for month := 0; month < 12; month++ {
		local[month], main[month], crank[month], meter[month] = .07996, .3006, .3000, 1.0004
		if month%2 == 1 {
			local[month], main[month], crank[month] = .08016, .2992, .3004
		}
	}
	for index := 1; index <= 5; index++ {
		epathSQLVRFAllocationUnitReplaceSource(&frames.NativeVRFSystems[0], "local_cooling", fmt.Sprintf("SPACE%d-1", index), local)
	}
	epathSQLVRFAllocationUnitReplaceSource(&frames.NativeVRFSystems[0], "shared_cooling", "", main)
	epathSQLVRFAllocationUnitReplaceSource(&frames.NativeVRFSystems[0], "shared_crankcase", "", crank)
	original := epathSQLVRFAllocationUnitSource(1, "Cooling:Electricity", "", meter)
	original.IsMeter = true
	frames.SourceIdentities[1] = original
	quantities, err := epathSQLMonthly(original, model.Precision)
	if err != nil {
		t.Fatal(err)
	}
	frames.SourceRaw[1] = quantities
	for month, q := range quantities {
		copy := q
		frames.Site["cooling.electricity"][month] = &copy
	}
	epathSQLVRFServiceUnitRecompile(t, &frames)
	native, err := epathSQLCompileVRFService(frames, model, model.Services[0])
	if err != nil {
		t.Fatal(err)
	}
	return *epathSQLVRFServiceLedger(native, "annual")
}

func TestEnergyPathRealSQLVRFLedgerKeepsNativeAndMonthlyDisplayAuthorities(t *testing.T) {
	annual := epathSQLVRFLedgerUnitAnnual(t)
	if err := epathSQLValidateVRFLedger(&annual); err != nil {
		t.Fatal(err)
	}
	if annual.Residual.Value != 0 || annual.Unassigned.Value <= 0 {
		t.Fatal("opposite monthly differences were netted before the Annual ledger")
	}
	for index, month := range annual.Months {
		allocated, residual := .601, -.001
		if index%2 == 1 {
			allocated, residual = .599, .001
		}
		if month.Expected.Value != 1 || month.Direct.Value != .4 || month.Allocated.Value != allocated || month.Residual.Value != residual || len(month.Sources) != 7 {
			t.Fatalf("original source fixture did not preserve literal monthly accounting: %#v", month)
		}
	}
	for name, mutate := range map[string]func(*epathSQLVRFAllocationLedgerProof){
		"annual re-round":               func(p *epathSQLVRFAllocationLedgerProof) { p.Expected = epathSQLQuantity{Value: 12.005} },
		"monthly source gap":            func(p *epathSQLVRFAllocationLedgerProof) { p.Months[0].NativeExpected.Value += .0006 },
		"lost month":                    func(p *epathSQLVRFAllocationLedgerProof) { p.Months = p.Months[:11] },
		"duplicate month":               func(p *epathSQLVRFAllocationLedgerProof) { p.Months[1].Period = "M1" },
		"hidden annual unassigned":      func(p *epathSQLVRFAllocationLedgerProof) { p.Unassigned = epathSQLQuantity{} },
		"negative residual erased":      func(p *epathSQLVRFAllocationLedgerProof) { p.Months[0].Residual.Value = 0 },
		"display source budget changed": func(p *epathSQLVRFAllocationLedgerProof) { p.Months[0].Allocated.Value += .001 },
		"native annual reweight":        func(p *epathSQLVRFAllocationLedgerProof) { p.NativeConstituents.Value += .0006 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := epathSQLVRFLedgerUnitAnnual(t)
			mutate(&bad)
			if err := epathSQLValidateVRFLedger(&bad); err == nil {
				t.Fatal("contradictory native/display ledger accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLVRFLedgerExactMilliDoesNotUseRelativeSlack(t *testing.T) {
	want := epathSQLQuantity{Value: 1000000}
	if err := epathSQLVRFRequireDisplayValue(1000000, want); err != nil {
		t.Fatal(err)
	}
	for _, actual := range []float64{1000000.001, 999999.999, 1000000.0001} {
		if err := epathSQLVRFRequireDisplayValue(actual, want); err == nil {
			t.Fatalf("changed discrete budget accepted: %.12f", actual)
		}
	}
}

func epathSQLVRFLedgerUnitCheck(t *testing.T) (PurposeResultBundle, epathSQLModelCheck) {
	p := epathSQLVRFLedgerUnitAnnual(t)
	allocation := &epathSQLAllocationProof{NativeVRF: &p, Expected: &p.Expected, Direct: &p.Direct, Allocated: &p.Allocated, Unassigned: &p.Unassigned}
	row := EnergyReconciliation{ID: p.ID, Level: "allocation", Unit: "kWh", Period: "annual", ServiceKind: p.Service, AllocationMethod: p.Method, Basis: p.Basis,
		ExpectedValue: p.Expected.Value, DirectValue: p.Direct.Value, AllocatedValue: p.Allocated.Value, UnassignedValue: p.Unassigned.Value,
		ResidualValue: p.Residual.Value, OvermappedValue: .006}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Reconciliation: []EnergyReconciliation{row}}}
	item := epathRealOracleMetricRecipe{Group: "zoneAllocation", Scope: "building", Period: "annual", Unit: "kWh",
		Target: epathRealOracleTarget{Collection: "reconciliation", ID: row.ID, Level: "allocation", Field: "expectedValue", Unit: "kWh"}}
	check := epathSQLModelCheck{Item: item, Quantity: &p.Expected, Allocation: allocation,
		Want: epathRealOracleMetric{Group: "zoneAllocation", Scope: "building", Period: "annual", Unit: "kWh", Value: &p.Expected.Value}}
	return bundle, check
}

func TestEnergyPathRealSQLVRFLedgerValidatesAllPublishedFields(t *testing.T) {
	bundle, check := epathSQLVRFLedgerUnitCheck(t)
	if err := epathCheckSQLModelAllocation(bundle, check); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PurposeResultBundle, *epathSQLModelCheck){
		"changed allocated milli": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Reconciliation[0].AllocatedValue += .001
		},
		"missing positive overlap": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Reconciliation[0].OvermappedValue = 0
		},
		"erased monthly unassigned": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Reconciliation[0].UnassignedValue = 0
		},
		"duplicate row": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Reconciliation = append(b.EnergyExplanation.Reconciliation, b.EnergyExplanation.Reconciliation[0])
		},
		"missing row":           func(b *PurposeResultBundle, _ *epathSQLModelCheck) { b.EnergyExplanation.Reconciliation = nil },
		"wrong selected period": func(_ *PurposeResultBundle, c *epathSQLModelCheck) { c.Item.Period = "M1" },
		"wrong service": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Reconciliation[0].ServiceKind = "heating"
		},
		"wrong method": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Reconciliation[0].AllocationMethod = "direct_only"
		},
		"wrong basis": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Reconciliation[0].Basis = "zone_load_allocation"
		},
		"wrong Zone": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Reconciliation[0].ZoneName = "SPACE1-1"
		},
		"wrong row period": func(b *PurposeResultBundle, _ *epathSQLModelCheck) {
			b.EnergyExplanation.Reconciliation[0].Period = "M1"
		},
		"wrong ID and target": func(b *PurposeResultBundle, c *epathSQLModelCheck) {
			b.EnergyExplanation.Reconciliation[0].ID = "renamed.annual"
			c.Item.Target.ID = "renamed.annual"
		},
		"generic proof must not bypass native/display distinction": func(_ *PurposeResultBundle, c *epathSQLModelCheck) { c.Allocation.NativeVRF = nil },
	} {
		t.Run(name, func(t *testing.T) {
			b, c := epathSQLVRFLedgerUnitCheck(t)
			mutate(&b, &c)
			if err := epathCheckSQLModelAllocation(b, c); err == nil {
				t.Fatal("VRF ledger field/context mutation accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLVRFLedgerRejectsBalancedDirectAllocatedLaundering(t *testing.T) {
	for _, period := range []string{"M1", "annual"} {
		t.Run(period, func(t *testing.T) {
			bundle, check := epathSQLVRFLedgerUnitCheck(t)
			if period != "annual" {
				month := check.Allocation.NativeVRF.Months[0]
				check.Allocation = &epathSQLAllocationProof{NativeVRF: &month, Expected: &month.Expected, Direct: &month.Direct, Allocated: &month.Allocated, Unassigned: &month.Unassigned}
				check.Quantity = &month.Expected
				check.Item.Period, check.Item.Target.ID = period, month.ID
				check.Want.Period, check.Want.Value = period, &month.Expected.Value
				row := EnergyReconciliation{ID: month.ID, Level: "allocation", Unit: "kWh", Period: period, ServiceKind: month.Service, AllocationMethod: month.Method, Basis: month.Basis,
					ExpectedValue: month.Expected.Value, DirectValue: month.Direct.Value, AllocatedValue: month.Allocated.Value, UnassignedValue: month.Unassigned.Value, ResidualValue: month.Residual.Value, OvermappedValue: .001}
				bundle.EnergyExplanation.Reconciliation = nil
				bundle.EnergyExplanation.Periods = []EnergyPeriod{{ID: period, Reconciliation: []EnergyReconciliation{row}}}
			}
			if err := epathCheckSQLModelAllocation(bundle, check); err != nil {
				t.Fatal("invalid unmodified hand ledger:", err)
			}
			p := check.Allocation.NativeVRF
			p.Direct = epathSQLQuantity{Value: p.Direct.Value + .001}
			p.Allocated = epathSQLQuantity{Value: p.Allocated.Value - .001}
			if period == "annual" {
				p.Months[0].Direct = epathSQLQuantity{Value: p.Months[0].Direct.Value + .001}
				p.Months[0].Allocated = epathSQLQuantity{Value: p.Months[0].Allocated.Value - .001}
				bundle.EnergyExplanation.Reconciliation[0].DirectValue = p.Direct.Value
				bundle.EnergyExplanation.Reconciliation[0].AllocatedValue = p.Allocated.Value
			} else {
				bundle.EnergyExplanation.Periods[0].Reconciliation[0].DirectValue = p.Direct.Value
				bundle.EnergyExplanation.Periods[0].Reconciliation[0].AllocatedValue = p.Allocated.Value
			}
			// Every displayed ledger total still agrees with the mutant proof;
			// only the untouched original source budgets expose this transfer.
			if err := epathCheckSQLModelAllocation(bundle, check); err == nil {
				t.Fatal("candidate and ledger jointly relabelled shared energy as measured direct")
			}
		})
	}
}

func TestEnergyPathRealSQLVRFLedgerRequiresOriginalSourceBindings(t *testing.T) {
	for _, name := range []string{"missing local", "changed budget", "local labelled shared", "different month", "foreign recipient", "different RDD", "split local", "partial shared pool", "changed annual source identity", "method and candidate changed"} {
		t.Run(name, func(t *testing.T) {
			annual := epathSQLVRFLedgerUnitAnnual(t)
			p := &annual.Months[0]
			id := 0
			for key, binding := range p.Sources {
				if binding.Allocation.Shared == (name == "partial shared pool") {
					id = key
					break
				}
			}
			binding := p.Sources[id]
			switch name {
			case "missing local":
				delete(p.Sources, id)
			case "changed budget":
				binding.Allocation.BudgetMilliKWh++
				p.Sources[id] = binding
			case "local labelled shared":
				binding.Allocation.Shared = true
				p.Sources[id] = binding
			case "different month":
				binding.Allocation.Native = binding.Identity.Observation.Months[1]
				p.Sources[id] = binding
			case "foreign recipient":
				for zone, share := range binding.Allocation.Shares {
					delete(binding.Allocation.Shares, zone)
					binding.Allocation.Shares["Plenum"] = share
					break
				}
				p.Sources[id] = binding
			case "different RDD":
				binding.Allocation.SourceID++
				p.Sources[id] = binding
			case "split local":
				for zone, share := range binding.Allocation.Shares {
					share.Native /= 2
					share.DisplayMilliKWh /= 2
					binding.Allocation.Shares[zone] = share
					other := "SPACE1-1"
					if zone == other {
						other = "SPACE2-1"
					}
					binding.Allocation.Shares[other] = share
					break
				}
				p.Sources[id] = binding
			case "partial shared pool":
				for zone, share := range binding.Allocation.Shares {
					share.Native -= .001
					share.DisplayMilliKWh--
					binding.Allocation.Shares[zone] = share
					break
				}
				binding.Allocation.UnassignedNative = .001
				binding.Allocation.UnassignedMilliKWh = 1
				p.Sources[id] = binding
			case "changed annual source identity":
				binding = annual.Months[1].Sources[id]
				binding.Identity.Observation.EquipmentObjectIndex++
				annual.Months[1].Sources[id] = binding
			case "method and candidate changed":
				p.Method = "direct_only"
			}
			if err := epathSQLValidateVRFLedger(&annual); err == nil {
				t.Fatal("source/metadata mutation passed independent ledger")
			}
		})
	}
}
