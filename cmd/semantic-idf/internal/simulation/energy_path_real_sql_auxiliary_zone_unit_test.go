package simulation

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathSQLAuxiliaryZoneUnitFrames() (epathSQLFrames, epathRealSQLModel) {
	frames, model := epathSQLZoneServiceUnitFrames()
	model.Services, model.DirectUses, model.FanPools = nil, nil, nil
	model.Site = []epathRealSQLSite{{ID: "pumps.electricity", EndUse: "pumps", Carrier: "electricity", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Pumps:Electricity", Unit: "J"}}, Keys: []string{""}, IsMeter: true}}}
	model.Auxiliaries = []epathRealSQLAuxiliary{{SiteID: "pumps.electricity", ServedZones: []string{"A", "B"}, Weight: "cooling_plus_heating", AllocationMethod: "plant_loop_load_share", ReconciliationID: "allocation.pumps.annual"}}
	frames.Site = map[string][]*epathSQLQuantity{"pumps.electricity": make([]*epathSQLQuantity, 12)}
	for month := 0; month < 12; month++ {
		frames.Site["pumps.electricity"][month] = &epathSQLQuantity{}
	}
	// Completed model-total loads A40:B100 in M1, A90:B10 in M2.
	// Pumps 14 and20 therefore allocate A4+18=22, B10+2=12.
	frames.Site["pumps.electricity"][0].Value = 14
	frames.Site["pumps.electricity"][1].Value = 20
	frames.Zones["a"] = epathSQLZone{Name: "A", Multiplier: 10}
	source := epathRealSQLSource{DictionaryIndex: 30, Name: "Pumps:Electricity", IsMeter: true, ReportingFrequency: "Monthly", SourceUnit: "J", Rows: 12}
	for month := 1; month <= 12; month++ {
		value := frames.Site["pumps.electricity"][month-1].Value
		source.Months = append(source.Months, epathRealSQLMonth{Month: month, Rows: 1, RawSum: epathOracleNumber(value * 3600000), EnergyKWh: epathOracleNumber(value)})
		if value > 0 {
			frames.Site["pumps.electricity"][month-1].Error = .001 // Two already reviewed source rounding stages.
		}
	}
	frames.SourceIdentities[30], frames.SiteSources["pumps.electricity"] = source, []int{30}
	return frames, model
}

func TestEnergyPathRealSQLAuxiliaryZoneCarrierSupportsAllocatedPumps(t *testing.T) {
	frames, model := epathSQLAuxiliaryZoneUnitFrames()
	checks := epathSQLModelChecks{}
	if err := epathSQLModelAuxiliaryZoneChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 3*13*2 {
		t.Fatalf("all scalar/allocation obligations required: %d", len(checks.Rows))
	}
	for _, check := range checks.Rows {
		want := epathSQLAuxiliaryZoneUnitValue(check.Item.Zone, check.Item.Period)
		if check.Quantity == nil || math.Abs(check.Quantity.Value-want) > 1e-12 {
			t.Fatalf("monthly-first model-total share differs: %+v, want %v", check, want)
		}
	}
	start := len(checks.Rows)
	if err := epathSQLModelZoneCarrierChecks(frames, model, &checks); err != nil {
		t.Fatalf("reviewed allocated Pumps need independent Zone scalar/carrier support: %v", err)
	}
	if len(checks.Rows)-start != 3*13*5 {
		t.Fatalf("carrier plus three accounting fields must cover every context: %d", len(checks.Rows)-start)
	}
	for _, check := range checks.Rows[start:] {
		if check.ZoneCarrier != nil && (check.ZoneCarrier.Carrier != "electricity" || check.ZoneCarrier.Basis != "service_path_allocation" || math.Abs(check.Quantity.Value-epathSQLAuxiliaryZoneUnitValue(check.Item.Zone, check.Item.Period)) > 1e-12) {
			t.Fatalf("allocated pump source changed carrier, directness or multiplier: %+v", check)
		}
	}
}

func epathSQLAuxiliaryZoneUnitValue(zone, period string) float64 {
	if zone == "A" {
		return map[string]float64{"M1": 4, "M2": 18, "annual": 22}[period]
	}
	if zone == "B" {
		return map[string]float64{"M1": 10, "M2": 2, "annual": 12}[period]
	}
	return 0
}

func epathSQLAuxiliaryZoneUnitBundle() PurposeResultBundle {
	frames, _ := epathSQLAuxiliaryZoneUnitFrames()
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	for _, id := range []int{10, 11, 12, 13, 30} {
		source := frames.SourceIdentities[id]
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), Name: source.Name, KeyValue: source.KeyValue, SourceUnit: "J", NormalizedUnit: "kWh", Units: "kWh", SourceType: "sql_report_data", ReportingFrequency: "Monthly", IsMeter: source.IsMeter})
	}
	for _, zone := range []string{"A", "B", "Plenum"} {
		result := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: zone}}
		for _, period := range epathSQLZoneCarrierPeriods() {
			p := EnergyPeriod{ID: period, Kind: "monthly"}
			value := epathSQLAuxiliaryZoneUnitValue(zone, period)
			if value > 0 {
				ids := []string{"sql-rdd-30", fmt.Sprintf("sql-rdd-%d", epathSQLZoneServiceUnitLoadID(zone, "cooling"))}
				if period == "M1" || period == "annual" {
					ids = append(ids, fmt.Sprintf("sql-rdd-%d", epathSQLZoneServiceUnitLoadID(zone, "heating")))
				}
				p.Nodes = []EnergyExplanationNode{{ID: "end_use.pumps." + strings.ToLower(zone), Level: "end_use", EndUse: "pumps", Value: value, AllocatedValue: value, AllocationApplied: true, SourceIDs: ids, ZoneName: zone, Period: period, Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", AggregationBasis: "model_total"}}
			}
			if period == "annual" {
				p.Kind, result.Nodes = "annual", append([]EnergyExplanationNode(nil), p.Nodes...)
			}
			result.Periods = append(result.Periods, p)
		}
		bundle.EnergyExplanation.ZoneResults = append(bundle.EnergyExplanation.ZoneResults, result)
	}
	return bundle
}

func TestEnergyPathRealSQLAuxiliaryZoneScalarOriginalTraceAndMutants(t *testing.T) {
	for _, mutation := range []string{"", "value", "allocated unknown", "wrong basis", "not allocated", "wrong Zone", "wrong period", "wrong domain", "wrong aggregation", "wrong carrier", "missing node", "duplicate node", "meter missing", "weight missing", "other Zone source", "nonoverlap weight", "source unknown", "source frequency", "source not meter", "source wrapper", "unowned zero node"} {
		t.Run(mutation, func(t *testing.T) {
			frames, model := epathSQLAuxiliaryZoneUnitFrames()
			checks := epathSQLModelChecks{}
			if err := epathSQLModelAuxiliaryZoneChecks(frames, model, &checks); err != nil {
				t.Fatal(err)
			}
			bundle := epathSQLAuxiliaryZoneUnitBundle()
			node := &bundle.EnergyExplanation.ZoneResults[0].Periods[1].Nodes[0]
			switch mutation {
			case "value":
				node.Value *= 10
			case "allocated unknown":
				node.inspectorDecodedFromJSON, node.inspectorValuePresence = true, 0
			case "wrong basis":
				node.Basis = "direct_zone_energy"
			case "not allocated":
				node.AllocationApplied = false
			case "wrong Zone":
				node.ZoneName = "B"
			case "wrong period":
				node.Period = "M2"
			case "wrong domain":
				node.ScaleDomain = "thermal"
			case "wrong aggregation":
				node.AggregationBasis = "native_zone"
			case "wrong carrier":
				node.Carrier = "natural_gas"
			case "missing node":
				bundle.EnergyExplanation.ZoneResults[0].Periods[1].Nodes = nil
			case "duplicate node":
				copy := *node
				copy.ID = "duplicate"
				bundle.EnergyExplanation.ZoneResults[0].Periods[1].Nodes = append(bundle.EnergyExplanation.ZoneResults[0].Periods[1].Nodes, copy)
			case "meter missing":
				node.SourceIDs = node.SourceIDs[1:]
			case "weight missing":
				node.SourceIDs = node.SourceIDs[:2]
			case "other Zone source":
				node.SourceIDs[1] = "sql-rdd-12"
			case "nonoverlap weight":
				p := &bundle.EnergyExplanation.ZoneResults[0].Periods[2].Nodes[0]
				p.SourceIDs = append(p.SourceIDs, "sql-rdd-11")
			case "source unknown":
				bundle.EnergyExplanation.Sources = bundle.EnergyExplanation.Sources[:4]
			case "source frequency":
				bundle.EnergyExplanation.Sources[4].ReportingFrequency = "Annual"
			case "source not meter":
				bundle.EnergyExplanation.Sources[4].IsMeter = false
			case "source wrapper":
				bundle.EnergyExplanation.Sources[4].InputSourceIDs = []string{"sql-rdd-10"}
			case "unowned zero node":
				copy := *node
				copy.ZoneName, copy.Value, copy.AllocatedValue = "Plenum", 0, 0
				bundle.EnergyExplanation.ZoneResults[2].Periods[1].Nodes = []EnergyExplanationNode{copy}
			}
			failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks)
			if (len(failures) == 0) != (mutation == "") {
				t.Fatalf("allocated auxiliary mutation %q: %+v", mutation, failures)
			}
		})
	}
}

func TestEnergyPathRealSQLAuxiliaryZoneRejectsUnknownOrRelabeledInputs(t *testing.T) {
	for _, mutation := range []string{"missing month", "nil month", "meter multiplied", "missing source month", "duplicate source month", "source NULL", "negative source", "wrong source family", "wrong frequency", "wrong source key", "wrong carrier", "wrong end use", "facility", "wrong selector", "annual fallback", "missing load", "invalid load", "unknown denominator", "unknown Zone", "duplicate Zone", "unassigned owners", "unknown weight", "unknown allocation", "duplicate pool", "missing trace", "wrong trace owner"} {
		t.Run(mutation, func(t *testing.T) {
			frames, model := epathSQLAuxiliaryZoneUnitFrames()
			source := frames.SourceIdentities[30]
			switch mutation {
			case "missing month":
				frames.Site["pumps.electricity"] = frames.Site["pumps.electricity"][:11]
			case "nil month":
				frames.Site["pumps.electricity"][0] = nil
			case "meter multiplied":
				frames.Site["pumps.electricity"][0] = &epathSQLQuantity{Value: 140, Error: .01}
			case "missing source month":
				source.Months = source.Months[:11]
			case "duplicate source month":
				source.Months[1].Month = 1
			case "source NULL":
				source.Months[0].MissingRows = 1
			case "negative source":
				source.Months[0].EnergyKWh = epathOracleNumber(-1)
			case "wrong source family":
				source.Name = "Fans:Electricity"
			case "wrong frequency":
				source.ReportingFrequency = "Hourly"
			case "wrong source key":
				source.KeyValue = "A"
			case "wrong carrier":
				model.Site[0].Carrier = "natural_gas"
			case "wrong end use":
				model.Site[0].EndUse = "fans"
			case "facility":
				model.Site[0].Facility = true
			case "wrong selector":
				model.Site[0].Source.Keys = []string{"*"}
			case "annual fallback":
				model.Site[0].Tabular = &epathRealSQLTabularSelector{}
			case "missing load":
				delete(frames.Loads, epathSQLKey("a", "cooling", 1))
			case "invalid load":
				frames.Loads[epathSQLKey("a", "cooling", 1)] = epathSQLQuantity{Value: math.NaN()}
			case "unknown denominator":
				frames.Loads[epathSQLKey("a", "cooling", 3)] = epathSQLQuantity{Error: .001}
			case "unknown Zone":
				model.Auxiliaries[0].ServedZones = []string{"unknown"}
			case "duplicate Zone":
				model.Auxiliaries[0].ServedZones = []string{"A", "a"}
			case "unassigned owners":
				model.Auxiliaries[0].Weight = "unassigned"
			case "unknown weight":
				model.Auxiliaries[0].Weight = "airflow"
			case "unknown allocation":
				model.Auxiliaries[0].AllocationMethod = "direct_zone_energy"
			case "duplicate pool":
				model.Auxiliaries = append(model.Auxiliaries, model.Auxiliaries[0])
			case "missing trace":
				delete(frames.LoadSourceIDs, epathSQLKey("a", "cooling", 1))
			case "wrong trace owner":
				s := frames.SourceIdentities[10]
				s.KeyValue = "B"
				frames.SourceIdentities[10] = s
			}
			frames.SourceIdentities[30] = source
			if _, err := epathSQLAuxiliaryZoneProofs(frames, model); err == nil {
				t.Fatalf("unsupported/unknown allocation input %q was accepted", mutation)
			}
		})
	}
}

func TestEnergyPathRealSQLAuxiliaryZoneCarrierRequiresExactPriorRoster(t *testing.T) {
	for _, mutation := range []string{"missing", "duplicate", "carrier", "direct", "annual reweight", "scaled proof", "unowned"} {
		t.Run(mutation, func(t *testing.T) {
			frames, model := epathSQLAuxiliaryZoneUnitFrames()
			checks := epathSQLModelChecks{}
			if err := epathSQLModelAuxiliaryZoneChecks(frames, model, &checks); err != nil {
				t.Fatal(err)
			}
			var index int
			for i, check := range checks.Rows {
				if check.Item.Zone == "A" && check.Item.Period == "annual" && check.Item.Target.Field == "value" {
					index = i
					break
				}
			}
			check := &checks.Rows[index]
			switch mutation {
			case "missing":
				checks.Rows = append(checks.Rows[:index], checks.Rows[index+1:]...)
			case "duplicate":
				checks.Rows = append(checks.Rows, *check)
			case "carrier":
				check.AuxiliaryZone.Carrier = "natural_gas"
			case "direct":
				check.Item.Target.Basis = "direct_zone_energy"
			case "annual reweight":
				check.AuxiliaryZone.Value = epathSQLQuantity{Value: 34 * 130.0 / 240.0}
				check.Quantity = &check.AuxiliaryZone.Value
				check.Want.Value = epathOracleNumber(check.Quantity.Value)
			case "scaled proof":
				check.AuxiliaryZone.Value = check.AuxiliaryZone.Value.times(10)
				check.Quantity = &check.AuxiliaryZone.Value
				check.Want.Value = epathOracleNumber(check.Quantity.Value)
			case "unowned":
				check.AuxiliaryZone.Owned = false
			}
			if _, _, err := epathSQLZoneCarrierInputs(frames, model, checks); err == nil {
				t.Fatalf("carrier accepted incomplete/forged auxiliary scalar %s", mutation)
			}
		})
	}
}

func TestEnergyPathRealSQLAuxiliaryZoneUnassignedAndZeroStayDistinct(t *testing.T) {
	frames, model := epathSQLAuxiliaryZoneUnitFrames()
	// Positive pump consumption, known-zero served loads: none is assigned to
	// the passive Plenum despite its independently reported positive loads.
	for zone := range frames.Zones {
		if zone != "plenum" {
			for _, service := range []string{"cooling", "heating"} {
				for month := 1; month <= 12; month++ {
					frames.Loads[epathSQLKey(zone, service, month)] = epathSQLQuantity{}
				}
			}
		}
	}
	proofs, err := epathSQLAuxiliaryZoneProofs(frames, model)
	if err != nil {
		t.Fatal(err)
	}
	for _, proof := range proofs {
		if proof.Value.Value != 0 || !proof.Value.includesZero() || len(proof.Required) != 0 || len(proof.NodeRequired) != 0 {
			t.Fatalf("unassigned source became a Zone observation: %+v", proof)
		}
	}
	before := epathSQLAuxiliaryZoneUnitBundle()
	bundle := epathSQLAuxiliaryZoneUnitBundle()
	checks := epathSQLModelChecks{}
	if err := epathSQLModelAuxiliaryZoneChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, checks); len(failures) == 0 {
		t.Fatal("invented assigned pump values escaped zero-load obligation")
	}
	if !reflect.DeepEqual(bundle, before) {
		t.Fatal("auxiliary proof modified candidate")
	}
	model.Auxiliaries[0].Weight, model.Auxiliaries[0].AllocationMethod, model.Auxiliaries[0].ServedZones = "unassigned", "unassigned", nil
	proofs, err = epathSQLAuxiliaryZoneProofs(frames, model)
	if err != nil || len(proofs) != 0 {
		t.Fatalf("existing unassigned auxiliary gained new metrics: %d %v", len(proofs), err)
	}
}
