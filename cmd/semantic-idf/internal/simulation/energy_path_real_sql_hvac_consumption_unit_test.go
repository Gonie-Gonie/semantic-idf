package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathSQLHVACConsumptionUnitSource(id int, name, key string, meter bool, values [12]float64) epathRealSQLSource {
	source := epathRealSQLSource{DictionaryIndex: id, Name: name, KeyValue: key, IsMeter: meter, SourceUnit: "J", ReportingFrequency: "Monthly", Rows: 12, RawSum: epathOracleNumber(0), EnergyKWh: epathOracleNumber(0)}
	for i, value := range values {
		source.Months = append(source.Months, epathRealSQLMonth{Month: i + 1, Rows: 1, RawSum: epathOracleNumber(value * 3600000), EnergyKWh: epathOracleNumber(value)})
		*source.RawSum += value * 3600000
		*source.EnergyKWh += value
	}
	return source
}

func epathSQLHVACConsumptionUnitFrames(t *testing.T, local2, local4, central [12]float64) (epathSQLFrames, epathRealSQLModel) {
	t.Helper()
	identity := epathSQLTestHVACSharedIdentity(central)
	service := epathRealSQLService{Service: "heating", SiteIDs: []string{"heat.e", "heat.g"}, ServedZones: append([]string(nil), identity.Member.ServedZones...), Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "load_to_site_energy", FallbackRatioKind: "load_to_site_energy", CarrierReconciliationIDs: map[string]string{"electricity": "reconcile.zone_hvac_allocation.heating.electricity.annual", "natural_gas": "reconcile.zone_hvac_allocation.heating.natural_gas.annual"}}
	declaration := epathRealSQLHVACConsumptionPool{SiteID: "heat.e", Shared: []epathRealSQLHVACSharedMember{identity.Member}}
	model := epathRealSQLModel{Precision: identity.Precision, Site: []epathRealSQLSite{{ID: "heat.e", EndUse: "heating", Carrier: "electricity"}, {ID: "heat.g", EndUse: "heating", Carrier: "natural_gas"}}, Services: []epathRealSQLService{service}, HVACConsumptionPools: []epathRealSQLHVACConsumptionPool{declaration}}
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{}, Loads: map[string]epathSQLQuantity{}, LoadSourceIDs: map[string][]int{}, Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceRaw: map[int][]epathSQLQuantity{}, SourceIdentities: map[int]epathRealSQLSource{}, DirectHVAC: map[string]epathSQLDirectHVACMonth{}, DirectHVACSourceIdentities: map[int]epathSQLDirectHVACSourceIdentity{}, HVACConsumptionPools: []epathSQLHVACConsumptionPoolFrame{{Declaration: declaration, Shared: []epathSQLHVACSharedSourceIdentity{identity}}}}
	for n, name := range append(append([]string(nil), service.ServedZones...), "PLENUM-1") {
		zone := strings.ToLower(name)
		frames.Zones[zone] = epathSQLZone{Name: name, Multiplier: 1}
		loads := [12]float64{}
		for m := range loads {
			loads[m] = 10
			if name == "PLENUM-1" {
				loads[m] = 500
			}
		}
		source := epathSQLHVACConsumptionUnitSource(100+n, "Zone Air System Sensible Heating Energy", name, false, loads)
		values, err := epathSQLMonthly(source, model.Precision)
		if err != nil {
			t.Fatal(err)
		}
		frames.SourceIdentities[source.DictionaryIndex] = source
		for m, q := range values {
			key := epathSQLKey(zone, "heating", m+1)
			// The native source stays unscaled; the selected load frame is
			// independently normalized by the original Zone multiplier once,
			// including its interval representation when that multiplier is 1.
			frames.Loads[key] = q.times(frames.Zones[zone].Multiplier)
			frames.LoadSourceIDs[key] = []int{source.DictionaryIndex}
		}
	}
	component := epathRealSQLDirectHVACComponent{ID: "heating.baseboard.electricity", Service: "heating", Carrier: "electricity", SiteID: "heat.e", Frequency: "Monthly", AggregationBasis: "model_total", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Baseboard Electricity Energy", Unit: "J"}}}}
	for n, item := range []struct {
		zone   string
		values [12]float64
	}{{"SPACE2-1", local2}, {"SPACE4-1", local4}} {
		name := item.zone + " Baseboard"
		owner := epathRealSQLDirectHVACOwner{KeyValue: name, ZoneName: item.zone, EquipmentType: "ZoneHVAC:Baseboard:RadiantConvective:Electric", EquipmentName: name, ComponentType: "ZoneHVAC:Baseboard:RadiantConvective:Electric"}
		component.Owners = append(component.Owners, owner)
		component.Source.Keys = append(component.Source.Keys, name)
		source := epathSQLHVACConsumptionUnitSource(200+n, "Baseboard Electricity Energy", name, false, item.values)
		values, err := epathSQLMonthly(source, model.Precision)
		if err != nil {
			t.Fatal(err)
		}
		frames.DirectHVACSourceIdentities[source.DictionaryIndex] = epathSQLDirectHVACSourceIdentity{FamilyID: component.ID, Service: "heating", Carrier: "electricity", SiteID: "heat.e", AggregationBasis: "model_total", Owner: owner, Source: source, Precision: model.Precision}
		for m, q := range values {
			frames.DirectHVAC[epathSQLDirectHVACKey(item.zone, "heating", "electricity", m+1)] = epathSQLDirectHVACMonth{Present: true, Quantity: q.positive(), SourceIDs: []int{source.DictionaryIndex}}
		}
	}
	model.DirectHVACComponents = []epathRealSQLDirectHVACComponent{component}
	for n, site := range model.Site {
		values := [12]float64{}
		for m := range values {
			if site.Carrier == "electricity" {
				values[m] = local2[m] + local4[m] + central[m]
			} else if m == 0 {
				values[m] = 100
			}
		}
		source := epathSQLHVACConsumptionUnitSource(1+n, "Heating:"+map[string]string{"electricity": "Electricity", "natural_gas": "NaturalGas"}[site.Carrier], "", true, values)
		monthly, err := epathSQLMonthly(source, model.Precision)
		if err != nil {
			t.Fatal(err)
		}
		frames.SiteSources[site.ID] = []int{source.DictionaryIndex}
		frames.SourceIdentities[source.DictionaryIndex] = source
		frames.SourceRaw[source.DictionaryIndex] = monthly
		for _, q := range monthly {
			q := q
			frames.Site[site.ID] = append(frames.Site[site.ID], &q)
		}
	}
	return frames, model
}

func TestEnergyPathRealSQLHVACConsumptionSourceLocalFiveZones(t *testing.T) {
	f, m := epathSQLHVACConsumptionUnitFrames(t, [12]float64{10}, [12]float64{30}, [12]float64{10})
	before := f.Loads[epathSQLKey("space2-1", "heating", 1)]
	p, err := epathSQLCompileHVACConsumptionService(f, m, m.Services[0])
	if err != nil {
		t.Fatal(err)
	}
	for n, want := range []float64{2, 12, 2, 32, 2} {
		zone := fmt.Sprintf("space%d-1", n+1)
		part := p.Direct.Monthly[0].Zones[zone]["electricity"]
		if part.Direct.Value+part.Allocated.Value != want || part.Allocated.Value != 2 {
			t.Fatalf("%s local+shared %g+%g want%g", zone, part.Direct.Value, part.Allocated.Value, want)
		}
		if p.Direct.Monthly[0].Zones[zone]["natural_gas"].Allocated.Value != 20 {
			t.Fatal("central gas lost original direct-electric recipient")
		}
	}
	if p.Direct.Monthly[0].Zones["plenum-1"]["electricity"].Allocated.Value != 0 || !reflect.DeepEqual(before, f.Loads[epathSQLKey("space2-1", "heating", 1)]) {
		t.Fatal("passive plenum or nonadditive load contaminated")
	}
	proof, _, err := epathSQLHVACConsumptionZoneProof(p, "space2-1", "M1")
	if err != nil {
		t.Fatal(err)
	}
	if proof.Branches["direct_zone_energy|electricity|end_use_to_carrier"].Quantity.Value != 10 || proof.Branches["service_path_allocation|electricity|end_use_to_carrier"].Quantity.Value != 2 || proof.Carriers["electricity"].Value != 12 {
		t.Fatal("same-carrier direct and shared branch did not coexist")
	}
	if err := epathSQLValidateHVACConsumptionService(p); err != nil {
		t.Fatal(err)
	}
	changed := p.Direct.Monthly[0].Zones["space2-1"]["electricity"]
	changed.Allocated.Value = 0
	p.Direct.Monthly[0].Zones["space2-1"]["electricity"] = changed
	if err := epathSQLValidateHVACConsumptionService(p); err == nil {
		t.Fatal("legacy whole-direct-Zone exclusion passed source-local proof")
	}
}

func TestEnergyPathRealSQLHVACConsumptionBuildingConversionRequiresOnlyServedSources(t *testing.T) {
	frames, model := epathSQLHVACConsumptionUnitFrames(t, [12]float64{10}, [12]float64{30}, [12]float64{10})
	pool, err := epathSQLCompileHVACConsumptionService(frames, model, model.Services[0])
	if err != nil {
		t.Fatal(err)
	}
	served, err := epathSQLDeclaredZones(frames, model.Services[0].ServedZones)
	if err != nil {
		t.Fatal(err)
	}
	for _, period := range []string{"M1", "annual"} {
		proof := &epathSQLConversionProof{From: epathSQLQuantity{Value: 50}, To: epathSQLQuantity{Value: 150}}
		if err := epathSQLDirectHVACBindBuildingSources(frames, model.Services[0], served, &pool.Direct, period, proof); err != nil {
			t.Fatal(err)
		}
		if len(proof.DirectHVACSources) != 7 || len(proof.DirectHVACRequired) != 7 {
			t.Fatalf("conversion source roster must contain five served native loads and two broad meters: %+v", proof)
		}
		var sources []EnergyDataSource
		for id, source := range frames.SourceIdentities {
			sources = append(sources, EnergyDataSource{
				ID: fmt.Sprintf("sql-rdd-%d", id), SourceType: "sql_report_data", IsMeter: source.IsMeter,
				Name: source.Name, KeyValue: source.KeyValue, SourceUnit: source.SourceUnit, NormalizedUnit: "kWh", ReportingFrequency: source.ReportingFrequency,
			})
		}
		ids := []string{"sql-rdd-1", "sql-rdd-2", "sql-rdd-100", "sql-rdd-101", "sql-rdd-102", "sql-rdd-103", "sql-rdd-104"}
		check := epathSQLModelCheck{Item: epathRealOracleMetricRecipe{Scope: "building", Period: period, Target: epathRealOracleTarget{Relation: "load_to_end_use", Service: "heating", Basis: "service_path_allocation"}}, Conversion: proof}
		for _, mutation := range []string{"exact served sources", "canonical passive plenum", "missing required served load"} {
			selected := append([]string(nil), ids...)
			switch mutation {
			case "canonical passive plenum":
				selected = append(selected, "sql-rdd-105")
			case "missing required served load":
				selected = selected[:len(selected)-1]
			}
			nodes := []EnergyExplanationNode{{ID: "load.heating.building", Level: "load", ServiceKind: "heating", Period: period, Value: 50, Unit: "kWh"}, {ID: "end_use.heating.building", Level: "end_use", EndUse: "heating", Period: period, Value: 150, Unit: "kWh"}}
			links := []EnergyPathLink{{ID: "conversion.heating", FromID: nodes[0].ID, ToID: nodes[1].ID, Period: period, Relation: "load_to_end_use", ServiceKind: "heating", Basis: "service_path_allocation", SourceIDs: selected}}
			bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{
				Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
				Sources: sources, Nodes: nodes, Links: links, Periods: []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: nodes, Links: links}},
			}}
			err := epathCheckSQLDirectHVACBuildingSources(bundle, check)
			if (err == nil) != (mutation == "exact served sources") {
				t.Errorf("%s/%s Building source proof=%v", period, mutation, err)
			}
		}
	}
}

func TestEnergyPathRealSQLHVACConsumptionNativeRoundingAndAnnualShares(t *testing.T) {
	f, m := epathSQLHVACConsumptionUnitFrames(t, [12]float64{1.6096, 2.4806}, [12]float64{3.9496, 12.5996}, [12]float64{})
	p, err := epathSQLCompileHVACConsumptionService(f, m, m.Services[0])
	if err != nil {
		t.Fatal(err)
	}
	ledger := &epathSQLHVACConsumptionLedgerProof{Service: p, Carrier: "electricity", Period: "annual", ID: m.Services[0].CarrierReconciliationIDs["electricity"]}
	fields, err := epathSQLHVACConsumptionLedgerFields(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if fields["overmappedValue"].Value != .002 || math.Abs(fields["residualValue"].Value+.002) > 1e-12 || fields["unassignedValue"].Value != 0 {
		t.Fatalf("rounding overlap was discarded: %#v", fields)
	}
	if len(p.Sources[0][1000].Shares) != 5 || p.Sources[0][1000].Native.Value != 0 {
		t.Fatal("observed zero central source disappeared")
	}
	// Change month 2's original weights, not a candidate graph. Completed
	// monthly allocations must be summed; an annual reweight is different.
	f, m = epathSQLHVACConsumptionUnitFrames(t, [12]float64{10, 0}, [12]float64{30, 0}, [12]float64{10, 20})
	source := f.SourceIdentities[100]
	*source.Months[1].EnergyKWh = 60
	*source.Months[1].RawSum = 60 * 3600000
	*source.EnergyKWh += 50
	*source.RawSum += 50 * 3600000
	f.SourceIdentities[100] = source
	values, err := epathSQLMonthly(source, m.Precision)
	if err != nil {
		t.Fatal(err)
	}
	f.Loads[epathSQLKey("space1-1", "heating", 2)] = values[1].times(f.Zones["space1-1"].Multiplier)
	p, err = epathSQLCompileHVACConsumptionService(f, m, m.Services[0])
	if err != nil {
		t.Fatal(err)
	}
	if p.Sources[0][1000].Shares["space1-1"] != 2000 || p.Sources[1][1000].Shares["space1-1"] != 12000 {
		t.Fatal("month-specific original denominator lost")
	}
	annual, _, err := epathSQLHVACConsumptionZoneProof(p, "space1-1", "annual")
	if err != nil {
		t.Fatal(err)
	}
	if annual.Carriers["electricity"].Value != 14 {
		t.Fatal("annual reweighted instead of summing completed source shares")
	}
}

func TestEnergyPathRealSQLHVACConsumptionMultiplierAndNoEligibleLoad(t *testing.T) {
	f, m := epathSQLHVACConsumptionUnitFrames(t, [12]float64{10}, [12]float64{30}, [12]float64{14})
	owner := f.Zones["space2-1"]
	owner.Multiplier = 3
	f.Zones["space2-1"] = owner
	for month := 1; month <= 12; month++ {
		key := epathSQLKey("space2-1", "heating", month)
		f.Loads[key] = f.Loads[key].times(3)
	}
	p, err := epathSQLCompileHVACConsumptionService(f, m, m.Services[0])
	if err != nil {
		t.Fatal(err)
	}
	part := p.Direct.Monthly[0].Zones["space2-1"]["electricity"]
	if part.Direct.Value != 10 || part.Allocated.Value != 6 || p.Sources[0][1000].BudgetMilliKWh != 14000 {
		t.Fatal("Zone multiplier reapplied to native model-total consumption")
	}
	f, m = epathSQLHVACConsumptionUnitFrames(t, [12]float64{10}, [12]float64{30}, [12]float64{10})
	for _, name := range m.Services[0].ServedZones {
		zone := strings.ToLower(name)
		id := f.LoadSourceIDs[epathSQLKey(zone, "heating", 1)][0]
		source := epathSQLHVACConsumptionUnitSource(id, "Zone Air System Sensible Heating Energy", name, false, [12]float64{})
		f.SourceIdentities[id] = source
		values, err := epathSQLMonthly(source, m.Precision)
		if err != nil {
			t.Fatal(err)
		}
		for n, q := range values {
			f.Loads[epathSQLKey(zone, "heating", n+1)] = q.times(f.Zones[zone].Multiplier)
		}
	}
	p, err = epathSQLCompileHVACConsumptionService(f, m, m.Services[0])
	if err != nil {
		t.Fatal(err)
	}
	row := p.Direct.Monthly[0].ByCarrier["electricity"]
	if row.Direct.Value != 40 || row.Allocated.Value != 0 || row.Unassigned.Value != 10 || len(p.Sources[0][1000].Shares) != 0 || p.Sources[0][1000].UnassignedMilliKWh != 10000 {
		t.Fatal("no eligible original load invented central allocation or lost direct observation")
	}
}

func TestEnergyPathRealSQLHVACConsumptionLedgerConsumerRejectsOverlapRepair(t *testing.T) {
	f, m := epathSQLHVACConsumptionUnitFrames(t, [12]float64{1.6096}, [12]float64{3.9496}, [12]float64{})
	p, err := epathSQLCompileHVACConsumptionService(f, m, m.Services[0])
	if err != nil {
		t.Fatal(err)
	}
	checks := epathSQLModelChecks{}
	if err := epathSQLHVACConsumptionBuildingLedgerChecks(m.Services[0], p, "annual", &checks); err != nil {
		t.Fatal(err)
	}
	var selected epathSQLModelCheck
	for _, check := range checks.Rows {
		if check.Allocation.HVACConsumption.Carrier == "electricity" && check.Item.Target.Field == "expectedValue" {
			selected = check
		}
	}
	if selected.Allocation == nil {
		t.Fatal("independent carrier ledger missing")
	}
	// Hand-authored arithmetic: 1.610+3.950 versus separately rounded5.559.
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Reconciliation: []EnergyReconciliation{{ID: m.Services[0].CarrierReconciliationIDs["electricity"], Level: "allocation", Period: "annual", ServiceKind: "heating", Basis: "service_path_allocation", Unit: "kWh", ExpectedValue: 5.559, DirectValue: 5.560, ExplainedValue: 5.560, OvermappedValue: .001, ResidualValue: -.001, AllocationMethod: "direct_only"}}}}
	if err := epathCheckSQLHVACConsumptionAllocation(bundle, selected); err != nil {
		t.Fatal(err)
	}
	bundle.EnergyExplanation.Reconciliation[0].OvermappedValue = 0
	if err := epathCheckSQLHVACConsumptionAllocation(bundle, selected); err == nil {
		t.Fatal("display rounding overlap silently repaired")
	}
}

func TestEnergyPathRealSQLHVACConsumptionRejectsUnknownAndDuplicate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*epathSQLFrames, *epathRealSQLModel)
	}{
		{"missing pool", func(f *epathSQLFrames, _ *epathRealSQLModel) { f.HVACConsumptionPools = nil }},
		{"missing shared month", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.HVACConsumptionPools[0].Shared[0].Source.Months[0].EnergyKWh = nil
		}},
		{"absent not zero", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.HVACConsumptionPools[0].Shared[0].Source.Months[0].MissingRows = 1
		}},
		{"duplicate member", func(f *epathSQLFrames, m *epathRealSQLModel) {
			f.HVACConsumptionPools[0].Shared = append(f.HVACConsumptionPools[0].Shared, f.HVACConsumptionPools[0].Shared[0])
			m.HVACConsumptionPools[0].Shared = append(m.HVACConsumptionPools[0].Shared, m.HVACConsumptionPools[0].Shared[0])
			f.HVACConsumptionPools[0].Declaration = m.HVACConsumptionPools[0]
		}},
		{"foreign recipient", func(f *epathSQLFrames, m *epathRealSQLModel) {
			f.HVACConsumptionPools[0].Shared[0].Member.ServedZones = append(f.HVACConsumptionPools[0].Shared[0].Member.ServedZones, "PLENUM-1")
			m.HVACConsumptionPools[0].Shared[0] = f.HVACConsumptionPools[0].Shared[0].Member
			f.HVACConsumptionPools[0].Declaration = m.HVACConsumptionPools[0]
		}},
		{"unknown recipient load", func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.Loads, epathSQLKey("space2-1", "heating", 1)) }},
		{"multiplier twice", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.Loads[epathSQLKey("space2-1", "heating", 1)] = f.Loads[epathSQLKey("space2-1", "heating", 1)].times(2)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, m := epathSQLHVACConsumptionUnitFrames(t, [12]float64{10}, [12]float64{30}, [12]float64{10})
			tc.change(&f, &m)
			if _, err := epathSQLCompileHVACConsumptionService(f, m, m.Services[0]); err == nil {
				t.Fatal("unknown/contradictory original pool accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLHVACConsumptionScopedSourceUnknownRaw(t *testing.T) {
	f, m := epathSQLHVACConsumptionUnitFrames(t, [12]float64{10}, [12]float64{30}, [12]float64{10})
	p, err := epathSQLCompileHVACConsumptionService(f, m, m.Services[0])
	if err != nil {
		t.Fatal(err)
	}
	// A typed Building result is required on the real save/reopen boundary.
	// An empty-schema, source-only object is intentionally marshaled as {}.
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{
		Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
		Sources: []EnergyDataSource{{
			ID: "sql-rdd-1000", SourceType: "sql_report_data", Name: "Boiler Ancillary Electricity Energy", KeyValue: "Central Boiler",
			Units: "J", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", AggregationMethod: "sum_report_data",
			AggregationBasis: "model_total", EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", DriverRole: "context", InspectorSection: "Context",
			RawValue: 10, EffectiveValue: 10, inspectorDecodedFromJSON: true, inspectorValuePresence: 3,
			ScopeDetails: []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE2-1", AggregationBasis: "model_total"}, AllocationApplied: true, AllocationFactor: .2, AllocatedValue: 2, AggregationBasis: "model_total", inspectorScopedValuePresence: true}},
		}},
	}}
	if err := epathSQLHVACConsumptionScopedSources(bundle, p, "space2-1"); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var reopened PurposeResultBundle
	if err := json.Unmarshal(data, &reopened); err != nil {
		t.Fatal(err)
	}
	if reopened.EnergyExplanation.Schema != energyExplanationSchema || reopened.EnergyExplanation.Scope.Kind != "building" || len(reopened.EnergyExplanation.Sources) != 1 || reopened.EnergyExplanation.Sources[0].ID != "sql-rdd-1000" || reopened.EnergyExplanation.Sources[0].RawValue != 10 || reopened.EnergyExplanation.Sources[0].EffectiveValue != 10 {
		t.Fatal("typed Building round trip lost the native shared source observation")
	}
	if err := epathSQLHVACConsumptionScopedSources(reopened, p, "space2-1"); err != nil {
		t.Fatal(err)
	}
	bundle.EnergyExplanation.Sources[0].ScopeDetails[0].RawValue = 2
	if err := epathSQLHVACConsumptionScopedSources(bundle, p, "space2-1"); err == nil {
		t.Fatal("allocated share passed as independently measured Zone raw")
	}
}
