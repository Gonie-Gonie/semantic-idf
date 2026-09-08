package simulation

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func epathSQLHeatRejectionFixture(t *testing.T) (epathSQLFrames, epathRealSQLModel, PurposeResultBundle) {
	t.Helper()
	precision := epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{}, Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{},
		SourceRaw: map[int][]epathSQLQuantity{}, SourceEffective: map[int][]epathSQLQuantity{}, Loads: map[string]epathSQLQuantity{}, LoadSourceIDs: map[string][]int{}, RadiantLoadSourceIdentities: map[int]epathSQLRadiantLoadSourceIdentity{}}
	model := epathRealSQLModel{Precision: precision, Site: []epathRealSQLSite{{ID: "heat_rejection.electricity", EndUse: "heat_rejection", Carrier: "electricity", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "HeatRejection:Electricity", Unit: "J"}}, Keys: []string{""}, IsMeter: true}}},
		Auxiliaries: []epathRealSQLAuxiliary{{SiteID: "heat_rejection.electricity", ServedZones: []string{"A", "B", "C"}, Weight: "cooling", AllocationMethod: "condenser_loop_load_share", ReconciliationID: "allocation.heat_rejection.annual"}}}
	// This literal original-input ownership is independent of the production
	// owner classifier. Condenser-chain applicability remains the explicit
	// reviewed auxiliary declaration, not an inference from candidate values.
	var original strings.Builder
	original.WriteString("Version,25.1;\n")
	binding := epathRealSQLRadiantLoadBinding{Frequency: "Monthly", AggregationBasis: "model_total", ThermalBoundary: "active_surface_source"}
	for _, zone := range []string{"A", "B", "C"} {
		factor := 1
		if zone == "A" {
			factor = 7
		}
		fmt.Fprintf(&original, `Zone,%[1]s,0,0,0,0,1,%[2]d;
BuildingSurface:Detailed,%[1]s Floor,Floor,Slab,%[1]s,,Ground,,NoSun,NoWind,1,3,0,0,0,1,0,0,0,1,0;
ZoneHVAC:EquipmentConnections,%[1]s,%[1]s List,,,%[1]s Air,;
ZoneHVAC:EquipmentList,%[1]s List,SequentialLoad,ZoneHVAC:LowTemperatureRadiant:ConstantFlow,%[1]s Radiant,1,1,,;
ZoneHVAC:LowTemperatureRadiant:ConstantFlow,%[1]s Radiant,%[1]s Design,Always,%[1]s,%[1]s Floor,400,.0004,,75000,50,%[1]s HW In,%[1]s HW Out,H1,H2,H3,H4,%[1]s CW In,%[1]s CW Out,C1,C2,C3,C4,,;
ZoneHVAC:LowTemperatureRadiant:ConstantFlow:Design,%[1]s Design,ConvectionOnly,.012,.016,.35,MeanAirTemperature,.8,.87,.1,SimpleOff,1;
Branch,%[1]s Heating,,ZoneHVAC:LowTemperatureRadiant:ConstantFlow,%[1]s Radiant,%[1]s HW In,%[1]s HW Out;
Branch,%[1]s Cooling,,ZoneHVAC:LowTemperatureRadiant:ConstantFlow,%[1]s Radiant,%[1]s CW In,%[1]s CW Out;
`, zone, factor)
		frames.Zones[strings.ToLower(zone)] = epathSQLZone{Name: zone, Multiplier: float64(factor)}
		binding.Owners = append(binding.Owners, epathRealSQLRadiantOwner{EquipmentType: "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", EquipmentName: zone + " Radiant", DesignName: zone + " Design", SurfaceName: zone + " Floor", ZoneName: zone})
	}
	original.WriteString("ZoneList,Repeated,A; ZoneGroup,Repeated Group,Repeated,3;\n")
	frames.Zones["a"] = epathSQLZone{Name: "A", Multiplier: 21}
	owners, err := epathSQLRadiantOriginalOwners(original.String(), binding)
	if err != nil {
		t.Fatal(err)
	}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	addSource := func(index int, name, key string, meter bool, values [12]float64) epathRealSQLSource {
		source := epathRealSQLSource{DictionaryIndex: index, Name: name, KeyValue: key, IsMeter: meter, ReportingFrequency: "Monthly", SourceUnit: "J", Rows: 12}
		raw, total := 0.0, 0.0
		for month, value := range values {
			source.Months = append(source.Months, epathRealSQLMonth{Month: month + 1, Rows: 1, RawSum: epathOracleNumber(value * 3600000), EnergyKWh: epathOracleNumber(value)})
			raw, total = raw+value*3600000, total+value
		}
		source.RawSum, source.EnergyKWh = epathOracleNumber(raw), epathOracleNumber(total)
		frames.SourceIdentities[index] = source
		q, err := epathSQLMonthly(source, precision)
		if err != nil {
			t.Fatal(err)
		}
		frames.SourceRaw[index] = q
		candidate := EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", index), Name: name, KeyValue: key, SourceType: "sql_report_data", IsMeter: meter, Units: "J", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", AggregationMethod: "sum", AggregationBasis: "model_total", RawValue: total, EffectiveValue: total, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total"}
		if !meter {
			candidate.ZoneName = strings.Split(key, " ")[0]
			candidate.AggregationMethod = "sum_report_data"
		}
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, candidate)
		return source
	}
	addSource(1, "HeatRejection:Electricity", "", true, [12]float64{60, 120, 30})
	frames.SiteSources["heat_rejection.electricity"] = []int{1}
	for _, q := range frames.SourceRaw[1] {
		copy := q
		frames.Site["heat_rejection.electricity"] = append(frames.Site["heat_rejection.electricity"], &copy)
	}
	cooling := [3][12]float64{{10, 30}, {20, 20}, {30, 10}}
	heating := [3][12]float64{{90, 10, 7}, {40, 50, 11}, {10, 90, 13}}
	for i, service := range []string{"cooling", "heating"} {
		name, values := "Zone Radiant HVAC Cooling Energy", cooling
		if service == "heating" {
			name, values = "Zone Radiant HVAC Heating Energy", heating
		}
		model.Loads = append(model.Loads, epathRealSQLLoad{Service: service, Component: "combined", NativeRadiant: &binding, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: name, Unit: "J"}}, Keys: []string{"A Radiant", "B Radiant", "C Radiant"}}})
		for z, zone := range []string{"A", "B", "C"} {
			index := 101 + 2*z + i
			source := addSource(index, name, zone+" Radiant", false, values[z])
			identity, err := epathSQLRadiantLoadObservation(source, service, owners[strings.ToLower(zone+" Radiant")], frames.Zones[strings.ToLower(zone)], precision)
			if err != nil {
				t.Fatal(err)
			}
			frames.RadiantLoadSourceIdentities[index] = identity
			for month, q := range identity.Effective {
				key := epathSQLKey(zone, service, month+1)
				frames.Loads[key], frames.LoadSourceIDs[key] = q, []int{index}
				frames.SourceEffective[index] = append(frames.SourceEffective[index], q)
			}
		}
	}
	// Hand math: cooling-only M1=10:20:30, M2=60:40:20. Annual
	// 70:60:50 is neither C+H weighting nor a share of the annual load.
	for z, zone := range []string{"A", "B", "C"} {
		result := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: zone}}
		for _, period := range epathSQLZoneCarrierPeriods() {
			value := 0.0
			if period == "M1" {
				value = []float64{10, 20, 30}[z]
			} else if period == "M2" {
				value = []float64{60, 40, 20}[z]
			} else if period == "annual" {
				value = []float64{70, 60, 50}[z]
			}
			row := EnergyPeriod{ID: period, Kind: "monthly"}
			if period == "annual" {
				row.Kind = "annual"
			}
			if value > 0 {
				row.Nodes = []EnergyExplanationNode{
					{ID: "tower", Level: "end_use", EndUse: "heat_rejection", Value: value, AllocatedValue: value, AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", ZoneName: zone, Period: period, Basis: "service_path_allocation", AggregationBasis: "model_total", SourceIDs: []string{"sql-rdd-1", fmt.Sprintf("sql-rdd-%d", 101+2*z)}},
					{ID: "carrier", Level: "carrier", Carrier: "electricity", Value: value, AllocatedValue: value, AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", ZoneName: zone, Period: period, Basis: "service_path_allocation", AggregationBasis: "model_total"},
				}
				row.Links = []EnergyPathLink{{ID: "tower-flow", FromID: "tower", ToID: "carrier", Relation: "direct_end_use_to_carrier", Basis: "service_path_allocation", FromValue: value, ToValue: value, FromUnit: "kWh", ToUnit: "kWh", ZoneName: zone, Period: period, SourceIDs: []string{"sql-rdd-1"}}}
				id := "reconcile.energy.electricity." + period + "." + strings.ToLower(zone)
				if period == "annual" {
					id = "reconcile.energy.electricity.M1." + strings.ToLower(zone) + ".annual"
				}
				row.Reconciliation = []EnergyReconciliation{{ID: id, Level: "energy", Period: period, ZoneName: zone, Basis: "service_path_allocation", Unit: "kWh", Status: "partial", ExpectedValue: value, ExplainedValue: value}}
			}
			result.Periods = append(result.Periods, row)
			if period == "annual" {
				result.Nodes, result.Links, result.Reconciliation = row.Nodes, row.Links, row.Reconciliation
			}
		}
		bundle.EnergyExplanation.ZoneResults = append(bundle.EnergyExplanation.ZoneResults, result)
	}
	return frames, model, bundle
}

func epathSQLHeatRejectionChecks(t *testing.T, frames epathSQLFrames, model epathRealSQLModel) epathSQLModelChecks {
	t.Helper()
	checks := epathSQLModelChecks{}
	for _, compile := range []func(epathSQLFrames, epathRealSQLModel, *epathSQLModelChecks) error{epathSQLModelAuxiliaryZoneChecks, epathSQLModelAuxiliaryFlowChecks, epathSQLModelZoneCarrierChecks} {
		if err := compile(frames, model, &checks); err != nil {
			t.Fatal(err)
		}
	}
	return checks
}

func TestEnergyPathRealSQLHeatRejectionCoolingOnlyNativeSharesAndConsumers(t *testing.T) {
	frames, model, bundle := epathSQLHeatRejectionFixture(t)
	checks := epathSQLHeatRejectionChecks(t, frames, model)
	if len(checks.Rows) != 3*13*11 {
		t.Fatalf("lost scalar/flow/carrier/reconciliation obligations: %d", len(checks.Rows))
	}
	for _, check := range checks.Rows {
		if p := check.AuxiliaryZone; p != nil {
			if p.Weight != "cooling" || p.AllocationMethod != "condenser_loop_load_share" || len(p.Sources) != 1 || p.Sources["sql-rdd-1"].RDD.Name != "HeatRejection:Electricity" {
				t.Fatal("condenser allocation became a pump/fan or individual equipment meter")
			}
			for id, source := range p.NodeSources {
				if id != "sql-rdd-1" && (source.NativeRadiant == nil || source.NativeRadiant.Service != "cooling" || source.NativeRadiant.AppliedMultiplier != 1) {
					t.Fatal("native cooling weight became heating or was multiplied again")
				}
			}
			if check.Item.Zone == "A" && check.Item.Period == "annual" && math.Abs(p.Value.Value-70) > 1e-12 {
				t.Fatal("annual load shortcut or C+H weights replaced 10+60")
			}
		}
	}
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) != 0 {
		t.Fatalf("literal native HeatRejection consumers failed: %v", failures[:min(5, len(failures))])
	}
}

func TestEnergyPathRealSQLHeatRejectionRejectsPolicyAndWeightMutants(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*epathSQLFrames, *epathRealSQLModel)
	}{
		{"wrong_carrier", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Site[0].Carrier = "natural_gas" }},
		{"pump_policy", func(_ *epathSQLFrames, m *epathRealSQLModel) {
			m.Auxiliaries[0].AllocationMethod = "plant_loop_load_share"
		}},
		{"both_service_weights", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Auxiliaries[0].Weight = "cooling_plus_heating" }},
		{"heating_weight", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Auxiliaries[0].Weight = "heating" }},
		{"pump_meter_alias", func(_ *epathSQLFrames, m *epathRealSQLModel) {
			m.Site[0].Source.Alternatives[0].Name = "Pumps:Electricity"
		}},
		{"unknown_owner", func(_ *epathSQLFrames, m *epathRealSQLModel) {
			m.Auxiliaries[0].ServedZones = []string{"A", "B", "Unknown"}
		}},
		{"duplicate_owner", func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Auxiliaries[0].ServedZones = []string{"A", "a", "C"} }},
		{"missing_cooling_month", func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.Loads, epathSQLKey("A", "cooling", 1)) }},
		{"heating_source_as_weight", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.LoadSourceIDs[epathSQLKey("A", "cooling", 1)] = []int{102}
		}},
		{"native_multiplier_twice", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			key := epathSQLKey("A", "cooling", 1)
			f.Loads[key] = f.Loads[key].times(21)
		}},
		{"missing_native_proof", func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.RadiantLoadSourceIdentities, 101) }},
		{"foreign_native_owner", func(f *epathSQLFrames, _ *epathRealSQLModel) {
			p := f.RadiantLoadSourceIdentities[101]
			p.Owner.Owner.ZoneName = "B"
			f.RadiantLoadSourceIdentities[101] = p
		}},
		{"missing_meter_month", func(f *epathSQLFrames, _ *epathRealSQLModel) { f.Site["heat_rejection.electricity"][0] = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			frames, model, _ := epathSQLHeatRejectionFixture(t)
			test.mutate(&frames, &model)
			if _, err := epathSQLAuxiliaryZoneProofs(frames, model); err == nil {
				t.Fatal("unsupported condenser policy, fabricated weight or foreign owner passed")
			}
		})
	}
}

func TestEnergyPathRealSQLHeatRejectionRejectsSourceAndFlowMutants(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*PurposeResultBundle)
	}{
		{"source_foreign_owner", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[1].ZoneName = "B" }},
		{"source_zone_key", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[1].KeyValue = "A" }},
		{"source_factor_twice", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[1].EffectiveMultiplier = 21 }},
		{"weight_as_consumption", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[1].Links[0].SourceIDs = []string{"sql-rdd-101"}
		}},
		{"heating_trace_injection", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[1].Nodes[0].SourceIDs = append(b.EnergyExplanation.ZoneResults[0].Periods[1].Nodes[0].SourceIDs, "sql-rdd-102")
		}},
		{"direct_instead_of_allocated", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[1].Nodes[0].Basis = "direct_zone_energy"
		}},
		{"wrong_carrier", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[1].Nodes[1].Carrier = "natural_gas"
		}},
		{"lost_weight_trace", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[1].Nodes[0].SourceIDs = []string{"sql-rdd-1"}
		}},
		{"missing_annual_wrapper_kind", func(b *PurposeResultBundle) {
			for i := range b.EnergyExplanation.ZoneResults[0].Periods {
				if b.EnergyExplanation.ZoneResults[0].Periods[i].ID == "annual" {
					b.EnergyExplanation.ZoneResults[0].Periods[i].Kind = ""
				}
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			frames, model, bundle := epathSQLHeatRejectionFixture(t)
			test.mutate(&bundle)
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, epathSQLHeatRejectionChecks(t, frames, model)); len(failures) == 0 {
				t.Fatal("allocated broad consumption and native load-weight provenance became interchangeable")
			}
		})
	}
}

func TestEnergyPathRealSQLHeatRejectionHeatingOnlyMonthRemainsUnassigned(t *testing.T) {
	frames, model, bundle := epathSQLHeatRejectionFixture(t)
	// Supply the two existing service declarations with independently known
	// zero site pools so this test reaches the common Building auxiliary ledger.
	for _, service := range []string{"cooling", "heating"} {
		id := service + ".electricity"
		model.Site = append(model.Site, epathRealSQLSite{ID: id, EndUse: service, Carrier: "electricity"})
		model.Services = append(model.Services, epathRealSQLService{Service: service, SiteIDs: []string{id}, ServedZones: []string{"A", "B", "C"}, Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "load_to_site_energy", FallbackRatioKind: "load_to_site_energy", ReconciliationID: "allocation." + service + ".annual"})
		for month := 1; month <= 12; month++ {
			frames.Site[id] = append(frames.Site[id], &epathSQLQuantity{})
		}
	}
	checks := epathSQLModelChecks{}
	if err := epathSQLModelServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	selected := epathSQLModelChecks{}
	for _, check := range checks.Rows {
		if strings.Contains(check.Item.Key, "heat_rejection.electricity/") {
			selected.Rows = append(selected.Rows, check)
		}
	}
	if len(selected.Rows) != 13*4 {
		t.Fatal("missing completed Monthly/Annual condenser ledger fields")
	}
	for _, period := range epathSQLZoneCarrierPeriods() {
		expected, allocated, unassigned := 0.0, 0.0, 0.0
		switch period {
		case "M1":
			expected, allocated = 60, 60
		case "M2":
			expected, allocated = 120, 120
		case "M3":
			expected, unassigned = 30, 30
		case "annual":
			expected, allocated, unassigned = 210, 180, 30
		}
		row := EnergyPeriod{ID: period, Kind: "monthly"}
		if period == "annual" {
			row.Kind = "annual"
		}
		if expected > 0 {
			row.Reconciliation = []EnergyReconciliation{{ID: "allocation.heat_rejection." + strings.ToLower(period), Level: "allocation", Period: period, Unit: "kWh", AllocationMethod: "condenser_loop_load_share", ExpectedValue: expected, AllocatedValue: allocated, UnassignedValue: unassigned}}
		}
		if period == "annual" {
			bundle.EnergyExplanation.Reconciliation = row.Reconciliation
		} else {
			bundle.EnergyExplanation.Periods = append(bundle.EnergyExplanation.Periods, row)
		}
	}
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, selected); len(failures) != 0 {
		t.Fatalf("cooling-only native condenser ledger failed: %v", failures[:min(5, len(failures))])
	}
	for i := range bundle.EnergyExplanation.Periods {
		if bundle.EnergyExplanation.Periods[i].ID == "M3" {
			bundle.EnergyExplanation.Periods[i].Reconciliation[0].AllocatedValue = 30
			bundle.EnergyExplanation.Periods[i].Reconciliation[0].UnassignedValue = 0
		}
	}
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, selected); len(failures) == 0 {
		t.Fatal("heating-only load falsely assigned the condenser pool while keeping the total unchanged")
	}
}
