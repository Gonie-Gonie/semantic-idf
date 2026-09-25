package simulation

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathSQLHeatOnlyCombustionBoundaryRejectsUnprovedFuel(t *testing.T) {
	baseline, baseModel, baseFrames := epathSQLHeatOnlyServiceUnit(t)
	data, err := os.ReadFile(baseline.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	queries := map[string]string{
		"positive-electric": `UPDATE ReportData SET Value=1 WHERE ReportDataDictionaryIndex=401 AND TimeIndex=20001`,
		"tiny-electric":     `UPDATE ReportData SET Value=0.000000001 WHERE ReportDataDictionaryIndex=401 AND TimeIndex=20001`,
		"zero-annual-gas":   `UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=402`,
		"negative-gas":      `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=402 AND TimeIndex=20001`,
		"missing-meter":     `DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=401`,
		"foreign-key":       `UPDATE ReportDataDictionary SET KeyValue='other' WHERE ReportDataDictionaryIndex=401`,
		"scheduled-meter":   `UPDATE ReportDataDictionary SET ScheduleName='Always On' WHERE ReportDataDictionaryIndex=401`,
		"wrong-step":        `UPDATE ReportDataDictionary SET TimestepType='HVAC System' WHERE ReportDataDictionaryIndex=401`,
		"wrong-group":       `UPDATE ReportDataDictionary SET IndexGroup='Facility:Electricity:Cooling' WHERE ReportDataDictionaryIndex=401`,
		"duplicate-meter":   `INSERT INTO ReportDataDictionary SELECT 499,Name,KeyValue,IsMeter,ReportingFrequency,Units,IndexGroup,Type,TimestepType,ScheduleName FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=401`,
		"missing-month":     `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=401 AND TimeIndex=20002`,
		"null-month":        `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=401 AND TimeIndex=20002`,
		"duplicate-month":   `INSERT INTO ReportData VALUES(9999999,401,20002,0)`,
		"orphan-month":      `INSERT INTO ReportData VALUES(9999999,401,9999999,0)`,
	}
	names := []string{"positive-electric", "tiny-electric", "zero-annual-gas", "negative-gas", "missing-meter", "foreign-key", "scheduled-meter", "wrong-step", "wrong-group", "duplicate-meter", "missing-month", "null-month", "duplicate-month", "orphan-month", "missing-request", "scheduled-request", "missing-frame", "uncertain-frame", "wrong-frame-source"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			observed := epathSQLHeatOnlyUnitSQLCopy(t, baseline, data)
			model, frames := epathSQLHeatOnlyClone(t, baseModel), epathSQLHeatOnlyClone(t, baseFrames)
			if query := queries[name]; query != "" {
				epathOracleEditSQL(t, observed.sqlPath, query)
			}
			switch name {
			case "missing-request":
				observed.executedText = strings.Replace(observed.executedText, "Output:Meter,Heating:Electricity,Monthly;\n", "", 1)
				for i, request := range observed.outputPlan.OutputObjects {
					if request.ObjectType == "Output:Meter" && request.KeyValue == "Heating:Electricity" {
						observed.outputPlan.OutputObjects = append(observed.outputPlan.OutputObjects[:i], observed.outputPlan.OutputObjects[i+1:]...)
						break
					}
				}
			case "scheduled-request":
				observed.executedText = strings.Replace(observed.executedText, "Output:Meter,Heating:Electricity,Monthly;", "Output:Meter,Heating:Electricity,Monthly,Always On;", 1)
				for i := range observed.outputPlan.OutputObjects {
					if observed.outputPlan.OutputObjects[i].KeyValue == "Heating:Electricity" {
						observed.outputPlan.OutputObjects[i].Fields = append(observed.outputPlan.OutputObjects[i].Fields, idf.OutputFieldValue{Value: "Always On"})
					}
				}
			case "missing-frame":
				frames.Site["heating.electricity"][1] = nil
			case "uncertain-frame":
				frames.Site["heating.electricity"][1] = &epathSQLQuantity{Error: 1e-12}
			case "wrong-frame-source":
				frames.SiteSources["heating.electricity"] = []int{402}
			}
			// Rebind the altered file/requests: stale SQL hashes are not the
			// reason these native calendar, carrier and frame negatives fail.
			binding, err := epathSQLCompileHeatOnlyBinding(observed, model)
			if err != nil {
				t.Fatal(err)
			}
			if err = epathSQLHeatOnlySingleService(frames, model, binding); err == nil {
				t.Fatal("unproved single-combustion boundary accepted")
			}
		})
	}
}

func epathSQLHeatOnlyCarrierUnit(t *testing.T) (epathSQLFrames, epathRealSQLModel, epathSQLModelChecks) {
	t.Helper()
	observed, model, frames := epathSQLHeatOnlyServiceUnit(t)
	// Literal independent direct equipment observation: 11 kWh per month in
	// each of the three actual original Equipment Zones. No native HVAC row or
	// amount is changed, and no unsupported West Zone Lights are invented.
	model.DirectUses = []epathRealSQLDirectUse{{EndUse: "equipment", Carrier: "electricity", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Zone Electric Equipment Electricity Energy", Unit: "J"}}, Keys: append([]string(nil), model.HeatOnlyFurnaces[0].ServedZones...)}}}
	for i, zone := range model.HeatOnlyFurnaces[0].ServedZones {
		source := epathRealSQLSource{DictionaryIndex: 501 + i, Name: "Zone Electric Equipment Electricity Energy", KeyValue: zone, ReportingFrequency: "Monthly", SourceUnit: "J", IndexGroup: "Zone", Rows: 12, RawSum: epathOracleNumber(132 * 3600000), EnergyKWh: epathOracleNumber(132)}
		for month := 1; month <= 12; month++ {
			source.Months = append(source.Months, epathRealSQLMonth{Month: month, Rows: 1, RawSum: epathOracleNumber(11 * 3600000), EnergyKWh: epathOracleNumber(11)})
		}
		observed.Sources = append(observed.Sources, source)
	}
	checks := epathSQLModelChecks{}
	if err := epathSQLBindHeatOnlyChecks(observed, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelFanPoolChecks(observed, frames, model.FanPools, model.Precision, &checks, checks.HeatOnly); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelDirectUseChecks(observed.Sources, frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return frames, model, checks
}

func TestEnergyPathSQLHeatOnlyInactiveMonthHasOneLiteralPlainID(t *testing.T) {
	frames, model, checks := epathSQLHeatOnlyCarrierUnit(t)
	if err := epathSQLModelZoneCarrierChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, check := range checks.Rows {
		if check.Reconciliation == nil || check.Item.Scope != "zone" || !strings.HasSuffix(check.Want.Key, "zone_subtotal/electricity/residualValue") {
			continue
		}
		period := check.Item.Period
		want := "reconcile.energy.electricity." + period
		if period == "annual" {
			want = "reconcile.energy.electricity.M1." + epathSQLZoneCarrierToken(check.Item.Zone) + ".annual"
		} else if period == "M1" {
			want += "." + epathSQLZoneCarrierToken(check.Item.Zone)
		}
		proof := check.Reconciliation
		if proof.ID != want || !reflect.DeepEqual(proof.AllowedIDs, []string{want}) || check.Item.Target.ID != want || proof.Basis != "direct_zone_energy" {
			t.Fatalf("period %s did not retain exactly its independently justified ID: %+v", period, proof)
		}
		if period == "M2" && (proof.Expected.Value != 11 || proof.Explained.Value != 11 || !epathSQLHeatOnlyExactZero(proof.Residual)) {
			t.Fatal("inactive-month direct equipment arithmetic changed")
		}
		counts[period]++
	}
	for _, period := range epathSQLZoneCarrierPeriods() {
		if counts[period] != 3 {
			t.Fatalf("missing three literal Zone IDs for %s: %v", period, counts)
		}
	}
	for _, pair := range []struct {
		from, to float64
		kind     string
	}{{85, 100, "efficiency"}, {125, 100, "load_to_fuel"}} {
		kind, err := epathSQLConversionRatioKind("efficiency", epathSQLQuantity{Value: pair.from}, epathSQLQuantity{Value: pair.to})
		if err != nil || kind != pair.kind {
			t.Fatalf("existing canonical delivered-load/fuel classifier changed: %s %v", kind, err)
		}
	}
}

func TestEnergyPathSQLHeatOnlyInactiveIDsRejectMissingOrUncertainParts(t *testing.T) {
	frames, model, checks := epathSQLHeatOnlyCarrierUnit(t)
	parts, _, err := epathSQLZoneCarrierInputs(frames, model, checks)
	if err != nil {
		t.Fatal(err)
	}
	key := epathSQLZoneCarrierContext("West Zone", "M2")
	for _, name := range []string{"missing-binding", "missing-family", "missing-carrier", "uncertain-low", "uncertain-high", "tiny-positive", "other-carrier", "direct-part", "unavailable", "uncertain-native-site", "missing-native-site"} {
		t.Run(name, func(t *testing.T) {
			bad := epathSQLHeatOnlyClone(t, parts)
			f := epathSQLHeatOnlyClone(t, frames)
			binding := checks.HeatOnly
			heat := bad[key]["service/heating"]
			switch name {
			case "missing-binding":
				binding = nil
			case "missing-family":
				delete(bad[key], "fan")
			case "missing-carrier":
				delete(heat.ByCarrier, "natural_gas")
			case "uncertain-low":
				heat.ByCarrier["natural_gas"] = epathSQLBounded(0, -1e-12, 0)
			case "uncertain-high":
				heat.ByCarrier["natural_gas"] = epathSQLBounded(0, 0, 1e-12)
			case "tiny-positive":
				heat.ByCarrier["natural_gas"] = epathSQLQuantity{Value: 1e-12}
			case "other-carrier":
				bad[key]["auxiliary/other"] = epathSQLZoneCarrierPart{ByCarrier: map[string]epathSQLQuantity{"district_heating": {Value: 1}}}
			case "direct-part":
				heat.DirectByCarrier = map[string]epathSQLQuantity{"natural_gas": {Value: 1e-12}}
			case "unavailable":
				heat.Unavailable = true
			case "uncertain-native-site":
				f.Site["fans.electricity"][1] = &epathSQLQuantity{Error: 1e-12}
			case "missing-native-site":
				f.Site["fans.electricity"][1] = nil
			}
			bad[key]["service/heating"] = heat
			if _, err := epathSQLHeatOnlyPlainMonthlyIDs(f, model, binding, bad); err == nil {
				t.Fatal("missing, uncertain or cross-carrier non-direct allocation authorized a plain ID")
			}
		})
	}
	plain, err := epathSQLHeatOnlyPlainMonthlyIDs(frames, model, checks.HeatOnly, parts)
	if err != nil || len(plain) != 33 || plain[epathSQLZoneCarrierContext("West Zone", "M1")] || plain[epathSQLZoneCarrierContext("West Zone", "annual")] {
		t.Fatalf("inactive-only map changed annual/active months: %v %v", plain, err)
	}
	for month := 2; month <= 12; month++ {
		if !plain[epathSQLZoneCarrierContext("West Zone", fmt.Sprintf("M%d", month))] {
			t.Fatal("known inactive month lost its independently qualified direct-only option")
		}
	}
}
