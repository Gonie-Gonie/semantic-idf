package simulation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// This explicit shared-pool policy does not invent measured direct equipment
// consumption. The ordinary DirectHVAC compiler and its carrier guard remain
// separate; only the reviewed native Radiant cooling boundary uses this proof.
type epathSQLRadiantCarrierAllocationProof struct {
	Service   epathRealSQLService
	Carrier   string
	Period    string
	Meter     epathRealSQLSource
	Loads     map[string]epathSQLRadiantLoadSourceIdentity
	Precision epathRealSQLPrecision
}

func epathSQLRadiantCarrierPolicy(service epathRealSQLService) error {
	if service.Service != "cooling" || service.Basis != "service_path_allocation" || service.FallbackBasis != "zone_load_allocation" ||
		service.RatioKind != "load_to_site_energy" || service.FallbackRatioKind != "load_to_site_energy" || service.ReconciliationID != "" ||
		len(service.SiteIDs) != 2 || len(service.CarrierReconciliationIDs) != 2 {
		return fmt.Errorf("shared carrier ledger requires the explicit two-pool native Radiant cooling policy")
	}
	for _, carrier := range []string{"electricity", "district_cooling"} {
		if service.CarrierReconciliationIDs[carrier] != "reconcile.zone_hvac_allocation.cooling."+carrier+".annual" {
			return fmt.Errorf("shared carrier ledger lacks the exact cooling/carrier reconciliation identity")
		}
	}
	return nil
}

func epathSQLCompileRadiantCarrierLedger(frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService) (map[string]*epathSQLRadiantCarrierAllocationProof, error) {
	if err := epathSQLRadiantCarrierPolicy(service); err != nil {
		return nil, err
	}
	if len(model.DirectHVACComponents) != 0 || len(model.NativeVRFSystems) != 0 {
		return nil, fmt.Errorf("shared Radiant ledger cannot replace a direct/native constituent service")
	}
	var declaration *epathRealSQLLoad
	for i := range model.Loads {
		if model.Loads[i].Service == service.Service {
			if declaration != nil || model.Loads[i].NativeRadiant == nil || model.Loads[i].Component != "combined" {
				return nil, fmt.Errorf("shared Radiant ledger lacks one exact native combined load declaration")
			}
			declaration = &model.Loads[i]
		}
	}
	served, err := epathSQLDeclaredZones(frames, service.ServedZones)
	if err != nil || declaration == nil || len(served) == 0 || len(declaration.NativeRadiant.Owners) != len(served) {
		return nil, fmt.Errorf("shared carrier ledger requires the complete original native owner roster")
	}
	loads, owners := map[string]epathSQLRadiantLoadSourceIdentity{}, map[string]bool{}
	for _, owner := range declaration.NativeRadiant.Owners {
		zone := strings.ToLower(owner.ZoneName)
		if !served[zone] || owners[zone] {
			return nil, fmt.Errorf("shared carrier ledger has a foreign/duplicate native owner")
		}
		owners[zone] = true
		for month := 1; month <= 12; month++ {
			ids := frames.LoadSourceIDs[epathSQLKey(zone, service.Service, month)]
			if len(ids) != 1 {
				return nil, fmt.Errorf("shared Radiant weight lost its exact monthly equipment source")
			}
			identity, err := epathSQLRadiantFrameLoadSource(frames, ids[0], zone, service.Service, month)
			if err != nil || !reflect.DeepEqual(identity.Owner.Owner, owner) {
				return nil, fmt.Errorf("shared Radiant weight contradicts its original typed owner: %v", err)
			}
			id := fmt.Sprintf("sql-rdd-%d", ids[0])
			if previous, ok := loads[id]; ok && !reflect.DeepEqual(previous, identity) {
				return nil, fmt.Errorf("native Radiant weight changed between monthly observations")
			}
			loads[id] = identity
		}
	}
	if len(loads) != len(owners) {
		return nil, fmt.Errorf("shared Radiant ledger has duplicate equipment ownership")
	}
	sites, seenIDs, seenSources := map[string]epathRealSQLSite{}, map[string]bool{}, map[int]bool{}
	for _, site := range model.Site {
		if site.ID == "" || sites[site.ID].ID != "" {
			return nil, fmt.Errorf("shared carrier ledger has an ambiguous site declaration")
		}
		sites[site.ID] = site
	}
	out := map[string]*epathSQLRadiantCarrierAllocationProof{}
	for _, id := range service.SiteIDs {
		site, exists := sites[id]
		name := map[string]string{"electricity": "Cooling:Electricity", "district_cooling": "Cooling:DistrictCooling"}[site.Carrier]
		if !exists || seenIDs[id] || out[site.Carrier] != nil || name == "" || site.Facility || site.EndUse != "cooling" || site.Tabular != nil ||
			!site.Source.IsMeter || site.Source.AllowAbsent || len(site.Source.Keys) != 1 || site.Source.Keys[0] != "" ||
			len(site.Source.Alternatives) != 1 || site.Source.Alternatives[0] != (epathRealSQLAlternative{Name: name, Unit: "J"}) {
			return nil, fmt.Errorf("shared cooling carrier has a mismatched/duplicate exact broad meter selector")
		}
		seenIDs[id] = true
		indexes := frames.SiteSources[id]
		if len(indexes) != 1 || len(frames.Site[id]) != 12 || seenSources[indexes[0]] {
			return nil, fmt.Errorf("shared cooling carrier requires one distinct twelve-month original meter")
		}
		seenSources[indexes[0]] = true
		meter, ok := frames.SourceIdentities[indexes[0]]
		if !ok || meter.DictionaryIndex != indexes[0] {
			return nil, fmt.Errorf("shared cooling carrier lost its original dictionary binding")
		}
		proof := &epathSQLRadiantCarrierAllocationProof{Service: service, Carrier: site.Carrier, Period: "annual", Meter: meter, Loads: loads, Precision: model.Precision}
		if _, _, err := epathSQLRadiantCarrierQuantities(proof); err != nil {
			return nil, err
		}
		original, _ := epathSQLMonthly(meter, model.Precision)
		if !reflect.DeepEqual(original, frames.SourceRaw[indexes[0]]) {
			return nil, fmt.Errorf("shared carrier source frame differs from its original Monthly meter")
		}
		for month, q := range original {
			if frames.Site[id][month] == nil || !epathSQLZoneCarrierQuantityEqual(*frames.Site[id][month], q) {
				return nil, fmt.Errorf("shared carrier site was multiplied, changed or lost a monthly value")
			}
		}
		out[site.Carrier] = proof
	}
	return out, nil
}

func epathSQLRadiantCarrierQuantities(proof *epathSQLRadiantCarrierAllocationProof) (map[string]epathSQLQuantity, string, error) {
	if proof == nil || epathSQLRadiantCarrierPolicy(proof.Service) != nil || !epathOracleValidPeriod(proof.Period) {
		return nil, "", fmt.Errorf("invalid independently bound shared Radiant ledger")
	}
	name := map[string]string{"electricity": "Cooling:Electricity", "district_cooling": "Cooling:DistrictCooling"}[proof.Carrier]
	meter := proof.Meter
	if name == "" || meter.DictionaryIndex <= 0 || meter.Name != name || meter.KeyValue != "" || !meter.IsMeter || meter.SourceUnit != "J" || meter.ReportingFrequency != "Monthly" || meter.MissingRows != 0 {
		return nil, "", fmt.Errorf("shared Radiant ledger requires the exact original broad Monthly/J meter")
	}
	monthly, err := epathSQLMonthly(meter, proof.Precision)
	if err != nil {
		return nil, "", err
	}
	for _, month := range meter.Months {
		if month.MissingRows != 0 || month.RawSum == nil || !epathOracleFinite(*month.RawSum) || *month.RawSum < 0 || !epathSQLVRFNativeClose(*month.RawSum/3600000, *month.EnergyKWh) {
			return nil, "", fmt.Errorf("shared meter lost the original Joule conversion or known monthly presence")
		}
	}
	owners := map[string]bool{}
	for _, zone := range proof.Service.ServedZones {
		key := strings.ToLower(zone)
		if key == "" || owners[key] {
			return nil, "", fmt.Errorf("ambiguous shared Radiant owner roster")
		}
		owners[key] = true
	}
	if len(owners) == 0 || len(proof.Loads) != len(owners) {
		return nil, "", fmt.Errorf("shared Radiant ledger lost its complete load denominator")
	}
	loadIDs := make([]string, 0, len(proof.Loads))
	for id, identity := range proof.Loads {
		owner := strings.ToLower(identity.Owner.Owner.ZoneName)
		if !owners[owner] || identity.Service != "cooling" || id != fmt.Sprintf("sql-rdd-%d", identity.Source.DictionaryIndex) || epathSQLValidateRadiantLoadSourceIdentity(identity) != nil {
			return nil, "", fmt.Errorf("shared Radiant ledger contains a foreign/invalid native weight")
		}
		delete(owners, owner)
		loadIDs = append(loadIDs, id)
	}
	sort.Strings(loadIDs)
	values := map[string]epathSQLQuantity{"expectedValue": {}, "directValue": {}, "allocatedValue": {}, "unassignedValue": {}}
	method := "unassigned"
	for _, month := range epathSQLPeriodMonths(proof.Period) {
		denominator := epathSQLQuantity{}
		for _, id := range loadIDs {
			denominator = denominator.add(proof.Loads[id].Effective[month-1])
		}
		low, high := denominator.bounds()
		if denominator.Value < 0 || denominator.Value == 0 && high != 0 || denominator.Value > 0 && low <= 0 {
			return nil, "", fmt.Errorf("uncertain native cooling denominator cannot prove shared allocation or unassigned zero")
		}
		pool := monthly[month-1]
		values["expectedValue"] = values["expectedValue"].add(pool)
		field := "unassignedValue"
		if denominator.Value > 0 {
			field = "allocatedValue"
			if pool.Value > 0 {
				method = "service_path_load_share"
			}
		}
		values[field] = values[field].add(pool)
	}
	return values, method, nil
}

func epathSQLRadiantCarrierLedgerChecks(compiled map[string]*epathSQLRadiantCarrierAllocationProof, period string, checks *epathSQLModelChecks) error {
	for _, carrier := range []string{"electricity", "district_cooling"} {
		if compiled[carrier] == nil || len(compiled) != 2 {
			return fmt.Errorf("shared Radiant ledger lost a carrier partition")
		}
		proof := *compiled[carrier]
		proof.Period = period
		values, method, err := epathSQLRadiantCarrierQuantities(&proof)
		if err != nil {
			return err
		}
		id, err := epathSQLAllocationID(proof.Service.CarrierReconciliationIDs[carrier], period)
		if err != nil {
			return err
		}
		start := len(checks.Rows)
		for _, field := range []string{"expectedValue", "directValue", "allocatedValue", "unassignedValue"} {
			q := values[field]
			target := epathRealOracleTarget{Collection: "reconciliation", ID: id, Level: "allocation", Service: "cooling", Basis: "service_path_allocation", AllocationMethod: method, Field: field, Unit: "kWh"}
			if err := checks.add("zoneAllocation", "building", "", period, "cooling/"+carrier+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
				return err
			}
		}
		if err := checks.bindAllocationProof(start); err != nil {
			return err
		}
		checks.Rows[start].Allocation.RadiantCarrier = &proof
	}
	return nil
}

// Run after the unchanged generic four-field allocation validator. Recompute
// from original inputs, then enforce the row's consumption/weight provenance.
func epathCheckSQLRadiantCarrierAllocation(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	if check.Allocation == nil || check.Allocation.RadiantCarrier == nil {
		return fmt.Errorf("missing typed shared Radiant ledger")
	}
	proof := check.Allocation.RadiantCarrier
	values, method, err := epathSQLRadiantCarrierQuantities(proof)
	if err != nil {
		return err
	}
	id, err := epathSQLAllocationID(proof.Service.CarrierReconciliationIDs[proof.Carrier], proof.Period)
	if err != nil || check.Item.Scope != "building" || check.Item.Zone != "" || check.Item.Period != proof.Period || check.Item.Target.ID != id ||
		check.Item.Target.Service != "cooling" || check.Item.Target.Basis != "service_path_allocation" || check.Item.Target.AllocationMethod != method {
		return fmt.Errorf("shared Radiant ledger target changed its exact physical context")
	}
	for field, q := range values {
		bound := check.Allocation.fields()[field]
		if bound == nil || !epathSQLZoneCarrierQuantityEqual(*bound, q.positive()) {
			return fmt.Errorf("shared Radiant ledger changed an original carrier partition")
		}
	}
	_, _, rows, _, err := epathOracleGraph(bundle, "building", "", proof.Period)
	if err != nil {
		return err
	}
	var row *EnergyReconciliation
	for i := range rows {
		if rows[i].ID == id {
			if row != nil {
				return fmt.Errorf("duplicate shared Radiant carrier reconciliation")
			}
			row = &rows[i]
		}
	}
	if row == nil {
		return nil // The preceding generic validator proved whole-row pruning.
	}
	if row.ZoneName != "" || row.Period != proof.Period || row.ServiceKind != "cooling" || row.Basis != "service_path_allocation" || row.AllocationMethod != method {
		return fmt.Errorf("shared Radiant carrier row has contradictory context")
	}
	sources := map[string]EnergyDataSource{}
	for _, source := range bundle.EnergyExplanation.Sources {
		if source.ID == "" || sources[source.ID].ID != "" {
			return fmt.Errorf("shared Radiant carrier trace has ambiguous original sources")
		}
		sources[source.ID] = source
	}
	meterID := fmt.Sprintf("sql-rdd-%d", proof.Meter.DictionaryIndex)
	allowed := map[string]epathSQLOriginalSource{meterID: epathSQLOriginalRDD(proof.Meter)}
	required := map[string]bool{meterID: !values["expectedValue"].includesZero()}
	for id, identity := range proof.Loads {
		original, err := epathSQLOriginalRadiant(identity)
		if err != nil {
			return err
		}
		allowed[id] = original
		for _, month := range epathSQLPeriodMonths(proof.Period) {
			if *proof.Meter.Months[month-1].EnergyKWh > 0 && !identity.Effective[month-1].includesZero() {
				required[id] = true
			}
		}
	}
	return epathSQLVerifyOriginalSources(row.SourceIDs, sources, allowed, required, proof.Period)
}
