package simulation

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func epathSQLVRFCarrierIDUnitInputs(t *testing.T, zeroJuly bool) (epathSQLFrames, epathRealSQLModel) {
	t.Helper()
	frames, model := epathSQLVRFServiceUnitFrames(t)
	if zeroJuly {
		system := &frames.NativeVRFSystems[0]
		for _, source := range append([]epathSQLVRFSourceFrame(nil), system.Sources...) {
			if source.Service != "heating" {
				continue
			}
			var values [12]float64
			for month, q := range source.Months {
				values[month] = q.Value
			}
			values[6] = 0 // Seven observed zeros, not missing source identities.
			epathSQLVRFAllocationUnitReplaceSource(system, source.Role, source.ZoneName, values)
		}
		var values [12]float64
		for month := range values {
			values[month] = 233 * float64(month+1)
		}
		values[6] = 0
		meter := epathSQLVRFAllocationUnitSource(2, "Heating:Electricity", "", values)
		meter.IsMeter = true
		frames.SourceIdentities[2] = meter
		quantities, err := epathSQLMonthly(meter, model.Precision)
		if err != nil {
			t.Fatal(err)
		}
		frames.SourceRaw[2] = quantities
		for month, q := range quantities {
			copy := q
			frames.Site["heating.electricity"][month] = &copy
		}
		epathSQLVRFServiceUnitRecompile(t, &frames)
	}
	return frames, model
}

func epathSQLVRFCarrierIDUnitChecks(t *testing.T, zeroJuly bool) epathSQLModelChecks {
	t.Helper()
	frames, model := epathSQLVRFCarrierIDUnitInputs(t, zeroJuly)
	checks := epathSQLVRFServiceUnitChecks(t, frames, model)
	start := len(checks.Rows)
	if err := epathSQLModelZoneCarrierChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return epathSQLModelChecks{Rows: checks.Rows[start:]}
}

func TestEnergyPathRealSQLVRFCarrierIDsCompleteOriginalOwners(t *testing.T) {
	for _, zeroJuly := range []bool{false, true} {
		t.Run(fmt.Sprintf("JulyZero=%v", zeroJuly), func(t *testing.T) {
			checks := epathSQLVRFCarrierIDUnitChecks(t, zeroJuly)
			if len(checks.Rows) != 6*13*5 {
				t.Fatalf("ID route changed scalar obligations: %d", len(checks.Rows))
			}
			plain := 0
			for _, check := range checks.Rows {
				if check.Reconciliation == nil || check.Item.Zone == "Plenum" {
					continue
				}
				plain++
				want := "reconcile.energy.electricity." + check.Item.Period
				if check.Reconciliation.ID != want || !reflect.DeepEqual(check.Reconciliation.AllowedIDs, []string{want}) {
					t.Fatalf("native owner lacks single independently derived ID: %+v", check.Reconciliation)
				}
				if check.Item.Zone == "SPACE1-1" && check.Item.Target.Field == "expectedValue" {
					// Independent handfixture: C M1 7.334, H M1 74.334;
					// M7 C51.334 plus H520.334 unless observed H is zero.
					want := map[string]float64{"M1": 81.668, "M7": 571.668, "annual": 5138}[check.Item.Period]
					if zeroJuly && check.Item.Period == "M7" {
						want = 51.334
					}
					if zeroJuly && check.Item.Period == "annual" {
						want = 4617.666
					}
					if want != 0 && !epathSQLVRFNativeClose(check.Quantity.Value, want) {
						t.Fatalf("identity choice changed %s native/display arithmetic: %.15g want %.15g", check.Item.Period, check.Quantity.Value, want)
					}
				}
			}
			if plain != 5*13*3 {
				t.Fatalf("missing owned exact reconciliation fields: %d", plain)
			}
		})
	}
}

func epathSQLVRFCarrierIDUnitBundle(period string, value float64) PurposeResultBundle {
	row := EnergyReconciliation{ID: "reconcile.energy.electricity." + period, Level: "energy", ZoneName: "SPACE1-1", Period: period,
		Unit: "kWh", Basis: "service_path_allocation", Status: "partial", ExpectedValue: value, ExplainedValue: value}
	zone := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE1-1"},
		Periods: []EnergyPeriod{{ID: period, Reconciliation: []EnergyReconciliation{row}}}}
	if period == "annual" {
		zone.Periods[0].Kind = "annual"
		zone.Reconciliation = []EnergyReconciliation{row}
	}
	return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema,
		Scope: EnergyExplanationScope{Kind: "building"}, ZoneResults: []EnergyExplanationZoneResult{zone}}}
}

func TestEnergyPathRealSQLVRFCarrierIDsKeepExactRecordValidation(t *testing.T) {
	checks := epathSQLVRFCarrierIDUnitChecks(t, false)
	for _, period := range []string{"M1", "annual"} {
		var selected epathSQLModelCheck
		for _, check := range checks.Rows {
			if check.Reconciliation != nil && check.Item.Zone == "SPACE1-1" && check.Item.Period == period && check.Item.Target.Field == "expectedValue" {
				selected = check
			}
		}
		value := 81.668
		if period == "annual" {
			value = 5138
		}
		for _, mutation := range []string{"", "qualified ID", "wrong Zone", "wrong period", "wrong basis", "wrong carrier", "duplicate", "missing", "quantity", "wrapper only"} {
			t.Run(period+"/"+mutation, func(t *testing.T) {
				bundle := epathSQLVRFCarrierIDUnitBundle(period, value)
				zone := &bundle.EnergyExplanation.ZoneResults[0]
				row := &zone.Periods[0].Reconciliation[0]
				switch mutation {
				case "qualified ID":
					row.ID = "reconcile.energy.electricity.M1.space1_1"
					if period == "annual" {
						row.ID += ".annual"
					}
				case "wrong Zone":
					row.ZoneName = "SPACE2-1"
				case "wrong period":
					row.Period = "M2"
				case "wrong basis":
					row.Basis = "direct_zone_energy"
				case "wrong carrier":
					row.ID = strings.ReplaceAll(row.ID, "electricity", "natural_gas")
				case "duplicate":
					zone.Periods[0].Reconciliation = append(zone.Periods[0].Reconciliation, *row)
				case "missing":
					zone.Periods[0].Reconciliation = nil
				case "quantity":
					row.ExpectedValue++
				case "wrapper only":
					if period != "annual" {
						t.Skip("annual wrapper boundary")
					}
					zone.Reconciliation[0].ID += ".space1_1"
				}
				err := epathCheckSQLModelReconciliation(bundle, selected)
				if (err != nil) != (mutation != "") {
					t.Fatalf("native plain-ID validation %q: %v", mutation, err)
				}
			})
		}
	}
}

func TestEnergyPathRealSQLVRFCarrierIDsFailClosed(t *testing.T) {
	mutations := []string{"missing declaration", "missing source frame", "missing source", "duplicate source", "changed allocation", "missing allocation month", "missing service", "duplicate service", "extra service", "omitted owner", "foreign owner", "foreign Zone", "mixed direct", "undeclared direct frame", "fan pool", "allocated auxiliary", "unassigned owner", "unassigned method", "unassigned weight source", "duplicate auxiliary", "foreign HVAC site", "missing broad", "null month", "altered broad", "altered source cache", "wrong broad identity"}
	for _, mutation := range mutations {
		t.Run(mutation, func(t *testing.T) {
			frames, model := epathSQLVRFCarrierIDUnitInputs(t, true)
			zone := "SPACE1-1"
			switch mutation {
			case "missing declaration":
				model.NativeVRFSystems = nil
			case "missing source frame":
				frames.NativeVRFSystems = nil
			case "missing source":
				frames.NativeVRFSystems[0].Sources = frames.NativeVRFSystems[0].Sources[1:]
			case "duplicate source":
				frames.NativeVRFSystems[0].Sources = append(frames.NativeVRFSystems[0].Sources, frames.NativeVRFSystems[0].Sources[0])
			case "changed allocation":
				row := frames.NativeVRFAllocations[0].Months[1].Zones["SPACE1-1"]["cooling"]
				row.AllocatedMilliKWh++
				frames.NativeVRFAllocations[0].Months[1].Zones["SPACE1-1"]["cooling"] = row
			case "missing allocation month":
				delete(frames.NativeVRFAllocations[0].Months, 7)
			case "missing service":
				model.Services = model.Services[:1]
			case "duplicate service":
				model.Services[1] = model.Services[0]
			case "extra service":
				model.Services = append(model.Services, epathRealSQLService{Service: "fans"})
			case "omitted owner":
				model.Services[0].ServedZones = model.Services[0].ServedZones[1:]
			case "foreign owner":
				model.Services[0].ServedZones = append(model.Services[0].ServedZones, "Plenum")
			case "foreign Zone":
				zone = "Unobserved"
			case "mixed direct":
				model.DirectHVACComponents = []epathRealSQLDirectHVACComponent{{}}
			case "undeclared direct frame":
				frames.DirectHVAC = map[string]epathSQLDirectHVACMonth{"foreign": {Present: true}}
			case "fan pool":
				model.FanPools = []epathRealSQLFanPool{{SiteID: "fans.electricity"}}
			case "allocated auxiliary":
				model.Auxiliaries = []epathRealSQLAuxiliary{{SiteID: "pumps.electricity", Weight: "cooling_plus_heating", AllocationMethod: "plant_loop_load_share"}}
			case "unassigned owner":
				model.Auxiliaries = []epathRealSQLAuxiliary{{SiteID: "fans.electricity", Weight: "unassigned", AllocationMethod: "unassigned", ServedZones: []string{"SPACE1-1"}}}
			case "unassigned method":
				model.Auxiliaries = []epathRealSQLAuxiliary{{SiteID: "fans.electricity", Weight: "unassigned", AllocationMethod: "service_path_load_share"}}
			case "unassigned weight source":
				model.Auxiliaries = []epathRealSQLAuxiliary{{SiteID: "fans.electricity", Weight: "unassigned", AllocationMethod: "unassigned", WeightSource: &epathRealSQLSelector{}}}
			case "duplicate auxiliary":
				aux := epathRealSQLAuxiliary{SiteID: "fans.electricity", Weight: "unassigned", AllocationMethod: "unassigned"}
				model.Auxiliaries = []epathRealSQLAuxiliary{aux, aux}
			case "foreign HVAC site":
				model.Site = append(model.Site, epathRealSQLSite{ID: "other.heating", EndUse: "heating", Carrier: "electricity"})
			case "missing broad":
				delete(frames.SiteSources, "heating.electricity")
			case "null month":
				frames.Site["heating.electricity"][6] = nil
			case "altered broad":
				frames.Site["heating.electricity"][0].Value += .0006
			case "altered source cache":
				frames.SourceRaw[2][0].Value += .0006
			case "wrong broad identity":
				source := frames.SourceIdentities[2]
				source.Name = "Cooling:Electricity"
				frames.SourceIdentities[2] = source
			}
			if mode, err := epathSQLZoneCarrierNativeVRFMonthlyIDs(frames, model, zone); err == nil || mode {
				t.Fatalf("unproved native ID mode accepted %q: %v/%v", mutation, mode, err)
			}
		})
	}
}

func TestEnergyPathRealSQLVRFCarrierIDsDoNotChangeOtherRoutes(t *testing.T) {
	if mode, err := epathSQLZoneCarrierNativeVRFMonthlyIDs(epathSQLFrames{}, epathRealSQLModel{}, "legacy"); mode || err != nil {
		t.Fatalf("non-native model acquired a new prerequisite: %v/%v", mode, err)
	}
	frames, model := epathSQLVRFCarrierIDUnitInputs(t, true)
	model.Auxiliaries = []epathRealSQLAuxiliary{{SiteID: "fans.electricity", Weight: "unassigned", AllocationMethod: "unassigned"}}
	for _, zone := range []string{"SPACE1-1", "space2-1", "Plenum"} {
		mode, err := epathSQLZoneCarrierNativeVRFMonthlyIDs(frames, model, zone)
		if err != nil || mode != !strings.EqualFold(zone, "Plenum") {
			t.Fatalf("known-zero owner/unserved Zone confused: %s %v/%v", zone, mode, err)
		}
	}
}
