package simulation

// Finite Furnace binding. This is not a production topology consumer.
import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathSQLHeatOnlyInputs struct {
	Furnaces    []epathRealSQLHeatOnlyFurnace
	FanPools    []epathRealSQLFanPool
	Services    []epathRealSQLService
	Auxiliaries []epathRealSQLAuxiliary
	Sites       []epathRealSQLSite
}

type epathSQLHeatOnlyBinding struct {
	Inputs                                         epathSQLHeatOnlyInputs
	OriginalText, ExecutedText, SQLPath, SQLSHA256 string
	Original, Executed                             epathSQLHeatOnlyFurnaceOriginalProof
	OutputPlan                                     PurposeRunPlan
	Native                                         epathSQLHeatOnlyNativeFan
	RowDigests                                     map[string]string
}

func epathSQLHeatOnlyInputsFor(m epathRealSQLModel) epathSQLHeatOnlyInputs {
	return epathSQLHeatOnlyInputs{m.HeatOnlyFurnaces, m.FanPools, m.Services, m.Auxiliaries, m.Site}
}

func epathSQLHeatOnlyModelFrom(in epathSQLHeatOnlyInputs) epathRealSQLModel {
	return epathRealSQLModel{HeatOnlyFurnaces: in.Furnaces, FanPools: in.FanPools, Services: in.Services, Auxiliaries: in.Auxiliaries, Site: in.Sites}
}

// Whole original physical type census prevents an extra loose HVAC family
// from escaping the finite route proof. Outputs and execution controls alone
// may differ between separately parsed original and executed documents.
func epathSQLHeatOnlyPhysical(doc idf.Document) ([]epathSQLPVOriginalObject, error) {
	want := map[string]int{"version": 1, "building": 1, "timestep": 1, "surfaceconvectionalgorithm:inside": 1, "surfaceconvectionalgorithm:outside": 1, "heatbalancealgorithm": 1, "site:location": 1, "sizingperiod:designday": 2, "site:groundtemperature:buildingsurface": 1, "material": 9, "windowmaterial:glazing": 1, "construction": 5, "zone": 3, "globalgeometryrules": 1, "buildingsurface:detailed": 20, "fenestrationsurface:detailed": 1, "scheduletypelimits": 5, "schedule:compact": 12, "people": 3, "lights": 2, "electricequipment": 3, "nodelist": 3, "branchlist": 1, "branch": 1, "airloophvac": 1, "availabilitymanagerassignmentlist": 1, "availabilitymanager:scheduled": 1, "zonehvac:equipmentconnections": 3, "zonehvac:equipmentlist": 3, "airloophvac:unitary:furnace:heatonly": 1, "airterminal:singleduct:constantvolume:noreheat": 3, "zonehvac:airdistributionunit": 3, "zonecontrol:thermostat": 1, "thermostatsetpoint:singleheating": 1, "thermostatsetpoint:singlecooling": 1, "airloophvac:supplypath": 1, "airloophvac:returnpath": 1, "airloophvac:zonesplitter": 1, "airloophvac:zonemixer": 1, "coil:heating:fuel": 1, "fan:onoff": 1}
	actual := map[string]int{}
	objects := epathSQLPVHVACPhysical(doc) // lexical field extraction only
	for _, o := range objects {
		actual[strings.ToLower(o.ObjectType)]++
	}
	if len(objects) != 105 || !reflect.DeepEqual(actual, want) {
		return nil, fmt.Errorf("HeatOnly exact 105-object physical census changed")
	}
	return objects, nil
}

func epathSQLHeatOnlyDeclaredInputs(in epathSQLHeatOnlyInputs) error {
	if len(in.Furnaces) != 1 || len(in.FanPools) != 1 || len(in.Services) != 1 || len(in.Auxiliaries) != 1 {
		return fmt.Errorf("HeatOnly requires one Furnace, native fan pool, Heating service and fan auxiliary")
	}
	d, p, s, a := in.Furnaces[0], in.FanPools[0], in.Services[0], in.Auxiliaries[0]
	want, err := epathSQLAirLoopFanZones(d.ServedZones)
	if err != nil || len(want) != 3 {
		return fmt.Errorf("HeatOnly requires its three original recipients")
	}
	for _, zones := range [][]string{p.ServedZones, s.ServedZones, a.ServedZones} {
		got, e := epathSQLAirLoopFanZones(zones)
		if e != nil || !reflect.DeepEqual(got, want) {
			return fmt.Errorf("HeatOnly service/fan recipients differ from original")
		}
	}
	if p.SiteID != "fans.electricity" || p.Name != "Air System Fan Electricity Energy" || !strings.EqualFold(p.Key, d.AirLoopName) || p.Frequency != "Hourly" || p.Unit != "J" {
		return fmt.Errorf("HeatOnly fan pool lost exact native loop/Hourly/J identity")
	}
	if s.Service != "heating" || !reflect.DeepEqual(s.SiteIDs, []string{"heating.electricity", "heating.natural_gas"}) || s.Basis != "service_path_allocation" || s.FallbackBasis != "zone_load_allocation" || s.RatioKind != "efficiency" || s.FallbackRatioKind != "efficiency" || s.ReconciliationID != "" || len(s.CarrierReconciliationIDs) != 2 {
		return fmt.Errorf("HeatOnly service changed its independently qualified delivered-load/fuel policy")
	}
	for _, carrier := range []string{"electricity", "natural_gas"} {
		if _, e := epathSQLAllocationID(s.CarrierReconciliationIDs[carrier], "annual"); e != nil {
			return e
		}
	}
	if a.SiteID != p.SiteID || a.Weight != "cooling_plus_heating" || a.WeightSource != nil || a.AllocationMethod != "air_loop_load_share" {
		return fmt.Errorf("HeatOnly fan auxiliary is not the exact reported-pool load-share policy")
	}
	if _, e := epathSQLAllocationID(a.ReconciliationID, "annual"); e != nil {
		return e
	}
	for _, spec := range []struct{ id, name, endUse, carrier string }{{"fans.electricity", "Fans:Electricity", "fans", "electricity"}, {"heating.electricity", "Heating:Electricity", "heating", "electricity"}, {"heating.natural_gas", "Heating:NaturalGas", "heating", "natural_gas"}} {
		count := 0
		for _, site := range in.Sites {
			if site.ID == spec.id {
				count++
				selector := epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: spec.name, Unit: "J"}}, Keys: []string{""}, IsMeter: true}
				if site.EndUse != spec.endUse || site.Carrier != spec.carrier || site.Facility || site.Tabular != nil || !reflect.DeepEqual(site.Source, selector) {
					return fmt.Errorf("HeatOnly changed exact site meter %s", spec.id)
				}
			}
		}
		if count != 1 {
			return fmt.Errorf("HeatOnly site meter missing/duplicated: %s", spec.id)
		}
	}
	return nil
}

func epathSQLCompileHeatOnlyBinding(observed epathRealOracleEvidence, model epathRealSQLModel) (*epathSQLHeatOnlyBinding, error) {
	if len(model.HeatOnlyFurnaces) != 1 || len(model.PVSystems)+len(model.PVHVACLoops)+len(model.PoolSystems)+len(model.NativeVRFSystems)+len(model.HVACConsumptionPools)+len(model.DirectHVACComponents)+len(model.AirLoopFans) != 0 {
		return nil, fmt.Errorf("HeatOnly cannot borrow another HVAC ownership contract")
	}
	data, err := json.Marshal(epathSQLHeatOnlyInputsFor(model))
	if err != nil {
		return nil, err
	}
	var inputs epathSQLHeatOnlyInputs
	if err = json.Unmarshal(data, &inputs); err != nil {
		return nil, err
	}
	if err = epathSQLHeatOnlyDeclaredInputs(inputs); err != nil {
		return nil, err
	}
	b := &epathSQLHeatOnlyBinding{Inputs: inputs, OriginalText: observed.originalText, ExecutedText: observed.executedText, SQLPath: observed.sqlPath}
	if strings.TrimSpace(b.OriginalText) == "" || strings.TrimSpace(b.ExecutedText) == "" || b.SQLPath == "" {
		return nil, fmt.Errorf("HeatOnly needs actual original/executed/native SQL evidence")
	}
	b.Original, err = epathSQLValidateHeatOnlyFurnaceOriginal(b.OriginalText, inputs.Furnaces[0])
	if err != nil {
		return nil, err
	}
	b.Executed, err = epathSQLValidateHeatOnlyFurnaceOriginal(b.ExecutedText, inputs.Furnaces[0])
	if err != nil {
		return nil, err
	}
	original, err := idf.Parse(b.OriginalText)
	if err != nil {
		return nil, err
	}
	executed, err := idf.Parse(b.ExecutedText)
	if err != nil {
		return nil, err
	}
	if observed.outputPlan == nil {
		return nil, fmt.Errorf("HeatOnly native binding needs its actual executed output plan")
	}
	planData, err := json.Marshal(observed.outputPlan)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(planData, &b.OutputPlan); err != nil {
		return nil, err
	}
	// Reuse the independent lexical request transport validator only. The
	// finite original route and native rows above/below own all physical roles.
	for _, request := range []struct {
		spec      epathSQLPVNativeSpec
		frequency string
	}{{epathSQLPVNativeSpec{Name: "Air System Fan Electricity Energy", Key: inputs.FanPools[0].Key}, "Hourly"}, {epathSQLPVNativeSpec{Name: "Fans:Electricity", IsMeter: true}, "Monthly"}} {
		if _, err = epathSQLPVRequestBinding(original, executed, &b.OutputPlan, request.spec, request.frequency); err != nil {
			return nil, err
		}
	}
	left, err := epathSQLHeatOnlyPhysical(original)
	if err != nil {
		return nil, err
	}
	right, err := epathSQLHeatOnlyPhysical(executed)
	if err != nil {
		return nil, err
	}
	for n, o := range left {
		r := right[n]
		if o.ObjectType != r.ObjectType || o.ObjectName != r.ObjectName || !reflect.DeepEqual(o.Fields, r.Fields) {
			return nil, fmt.Errorf("HeatOnly executed physical fields changed: %s/%s", o.ObjectType, o.ObjectName)
		}
	}
	b.SQLSHA256, err = epathReadRealFileHash(b.SQLPath)
	if err != nil {
		return nil, err
	}
	db, err := epathOpenOracleSQL(b.SQLPath)
	if err != nil {
		return nil, err
	}
	err = epathSQLValidateOriginalZoneMultipliers(db, b.OriginalText, epathSQLOriginalMultiplierContract)
	closeErr := db.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	b.Native, err = epathSQLReadHeatOnlyNativeFan(b.SQLPath, inputs.FanPools[0])
	if err != nil {
		return nil, err
	}
	if hash, e := epathReadRealFileHash(b.SQLPath); e != nil || hash != b.SQLSHA256 {
		return nil, fmt.Errorf("HeatOnly SQL changed during native audit")
	}
	return b, nil
}

func epathSQLValidateHeatOnlyBinding(b *epathSQLHeatOnlyBinding, external *epathRealSQLModel) error {
	if b == nil {
		return fmt.Errorf("missing HeatOnly original/native binding")
	}
	model := epathSQLHeatOnlyModelFrom(b.Inputs)
	if external != nil {
		model = *external
		if !reflect.DeepEqual(b.Inputs, epathSQLHeatOnlyInputsFor(model)) {
			return fmt.Errorf("HeatOnly retained inputs differ from external recipe")
		}
	}
	rebuilt, err := epathSQLCompileHeatOnlyBinding(epathRealOracleEvidence{originalText: b.OriginalText, executedText: b.ExecutedText, sqlPath: b.SQLPath, outputPlan: &b.OutputPlan}, model)
	if err != nil {
		return err
	}
	copy := *b
	copy.RowDigests = nil
	if !reflect.DeepEqual(*rebuilt, copy) {
		return fmt.Errorf("HeatOnly retained original/executed/native evidence changed")
	}
	return nil
}

func epathSQLBindHeatOnlyChecks(observed epathRealOracleEvidence, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	if checks == nil || checks.HeatOnly != nil {
		return fmt.Errorf("HeatOnly requires a fresh registry")
	}
	if len(model.HeatOnlyFurnaces) == 0 {
		if strings.TrimSpace(observed.originalText) != "" {
			doc, e := idf.Parse(observed.originalText)
			if e != nil {
				return e
			}
			for _, o := range doc.Objects {
				if strings.EqualFold(o.Type, "AirLoopHVAC:Unitary:Furnace:HeatOnly") {
					return fmt.Errorf("actual HeatOnly model lost its mandatory independent declaration")
				}
			}
		}
		return nil
	}
	b, err := epathSQLCompileHeatOnlyBinding(observed, model)
	if err != nil {
		return err
	}
	checks.HeatOnly = b
	return nil
}

func epathSQLHeatOnlyCensus(checks epathSQLModelChecks) error {
	if err := epathSQLHeatOnlyContextCensus(checks); err != nil {
		return err
	}
	b := checks.HeatOnly
	if b == nil {
		return fmt.Errorf("HeatOnly census has no binding")
	}
	rows, err := epathSQLPVHVACRegistry(checks)
	if err != nil {
		return err
	} // exact selector registry only
	need := func(group, scope, zone, period, suffix string, target epathRealOracleTarget) error {
		key := strings.Join([]string{group, scope, strings.ToLower(zone), period, suffix}, "|")
		row, ok := rows[key]
		actual := row.Item.Target
		if target.Collection == "sources" && strings.EqualFold(actual.SourceName, target.SourceName) && strings.EqualFold(actual.SourceKey, target.SourceKey) {
			actual.SourceName, actual.SourceKey = target.SourceName, target.SourceKey
		}
		if !ok || !reflect.DeepEqual(actual, target) {
			return fmt.Errorf("HeatOnly missing/changed required selector %s", key)
		}
		if strings.HasPrefix(suffix, "hvac_zone/heating/") && target.Field == "value" && (row.ZoneService == nil || row.ZoneService.Service != "heating" || row.ZoneService.Basis != "service_path_allocation") {
			return fmt.Errorf("HeatOnly Heating selector lost its typed service proof")
		}
		return nil
	}
	p := b.Inputs.FanPools[0]
	for _, field := range []string{"rawValue", "effectiveValue"} {
		if err = need("endUses", "building", "", "annual", "fan_pool/"+p.Key+"/"+field, epathRealOracleTarget{Collection: "sources", Field: field, SourceName: p.Name, SourceKey: p.Key, SourceUnit: "J", Frequency: "Hourly", Unit: "kWh"}); err != nil {
			return err
		}
	}
	for _, zone := range p.ServedZones {
		for _, period := range []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"} {
			target := epathSQLNodeTarget("end_use", "fans", "", "site")
			target.Basis, target.AllowPrunedZero = "service_path_allocation", true
			if err = need("zoneAllocation", "zone", zone, period, "fan_pools/value", target); err != nil {
				return err
			}
			for _, field := range []string{"value", "allocatedValue"} {
				target = epathSQLNodeTarget("end_use", "heating", "", "site")
				target.Field, target.Basis, target.AllowPrunedZero = field, "service_path_allocation", true
				if err = need("zoneAllocation", "zone", zone, period, "hvac_zone/heating/"+field, target); err != nil {
					return err
				}
			}
		}
	}
	if len(checks.FanPoolTotals) != 1 || !reflect.DeepEqual(checks.FanPoolTotals[p.SiteID].Pools, b.Inputs.FanPools) {
		return fmt.Errorf("HeatOnly lost audited fan totals")
	}
	for i, m := range b.Native.Months {
		if m.NonZero == 0 {
			q := checks.FanPoolTotals[p.SiteID].Months[i]
			lo, hi := q.bounds()
			if !q.valid() || q.Value != 0 || q.Error != 0 || lo != 0 || hi != 0 {
				return fmt.Errorf("HeatOnly inactive month lost exact zero accounting")
			}
		}
	}
	return nil
}

func epathSQLSealHeatOnlyChecks(checks *epathSQLModelChecks, model epathRealSQLModel) error {
	if checks == nil {
		return fmt.Errorf("HeatOnly sealing needs checks")
	}
	if checks.HeatOnly == nil {
		if len(model.HeatOnlyFurnaces) != 0 {
			return fmt.Errorf("HeatOnly binding removed before seal")
		}
		return nil
	}
	b := checks.HeatOnly
	if !reflect.DeepEqual(b.Inputs, epathSQLHeatOnlyInputsFor(model)) || b.RowDigests != nil {
		return fmt.Errorf("HeatOnly changed or already sealed inputs")
	}
	if err := epathSQLHeatOnlyCensus(*checks); err != nil {
		return err
	}
	b.RowDigests = map[string]string{}
	for i, row := range checks.Rows {
		digest, err := epathSQLPVHVACCheckDigest(row)
		if err != nil {
			return err
		}
		b.RowDigests[row.Want.Key] = digest
		checks.Rows[i].HeatOnlyBound = true
	}
	return nil
}

func epathSQLPrepareHeatOnlyChecks(checks epathSQLModelChecks, models ...*epathRealSQLModel) error {
	if len(models) > 1 {
		return fmt.Errorf("HeatOnly boundary accepts one external model")
	}
	var external *epathRealSQLModel
	if len(models) == 1 {
		external = models[0]
	}
	required := checks.HeatOnly != nil || external != nil && len(external.HeatOnlyFurnaces) != 0
	for _, row := range checks.Rows {
		required = required || row.HeatOnlyBound
	}
	if !required {
		return nil
	}
	if err := epathSQLValidateHeatOnlyBinding(checks.HeatOnly, external); err != nil {
		return err
	}
	if err := epathSQLHeatOnlyCensus(checks); err != nil {
		return err
	}
	if len(checks.HeatOnly.RowDigests) == 0 || len(checks.HeatOnly.RowDigests) != len(checks.Rows) {
		return fmt.Errorf("HeatOnly retained registry is unsealed/incomplete")
	}
	for _, row := range checks.Rows {
		digest, err := epathSQLPVHVACCheckDigest(row)
		if err != nil || !row.HeatOnlyBound || checks.HeatOnly.RowDigests[row.Want.Key] != digest {
			return fmt.Errorf("HeatOnly retained row changed/lost binding: %s", row.Want.Key)
		}
	}
	return nil
}
