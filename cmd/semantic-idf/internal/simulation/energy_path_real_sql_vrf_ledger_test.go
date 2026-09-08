package simulation

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

// Discrete display budgets cannot use the generic relative numeric slack:
// at a large annual value that slack would admit a changed millikWh.
func epathSQLVRFDisplayMilli(q epathSQLQuantity) (int64, error) {
	low, high := q.bounds()
	if !q.valid() || low != q.Value || high != q.Value || math.Abs(q.Value*1000) > 1<<50 {
		return 0, fmt.Errorf("VRF display is not a finite exact supported millikWh quantity")
	}
	milli := int64(math.Round(q.Value * 1000))
	value := float64(milli) / 1000
	scale := math.Max(math.Abs(value), math.Abs(q.Value))
	ulp := math.Nextafter(scale, math.Inf(1)) - scale
	if math.Abs(value-q.Value) > 32*ulp {
		return 0, fmt.Errorf("VRF display has an unreviewed sub-millikWh value")
	}
	return milli, nil
}

func epathSQLVRFRequireDisplayValue(actual float64, expected epathSQLQuantity) error {
	want, err := epathSQLVRFDisplayMilli(expected)
	if err != nil {
		return err
	}
	got, err := epathSQLVRFDisplayMilli(epathSQLQuantity{Value: actual})
	if err != nil || got != want {
		return fmt.Errorf("VRF displayed millikWh differs: actual %.12g, expected %.12g", actual, expected.Value)
	}
	return nil
}

func epathSQLVRFLedgerFields(p *epathSQLVRFAllocationLedgerProof) map[string]epathSQLQuantity {
	return map[string]epathSQLQuantity{"expectedValue": p.Expected, "directValue": p.Direct, "allocatedValue": p.Allocated, "unassignedValue": p.Unassigned, "residualValue": p.Residual}
}

func epathSQLVRFNativeClose(a, b float64) bool {
	if !epathOracleFinite(a) || !epathOracleFinite(b) || a < 0 || b < 0 {
		return false
	}
	scale := math.Max(a, b)
	return math.Abs(a-b) <= 128*(math.Nextafter(scale, math.Inf(1))-scale)
}

// Recompute classification and displayed totals from independently observed
// constituent identities. Ledger columns cannot move energy between measured
// local and shared allocation merely because their grand total still closes.
func epathSQLVRFLedgerSourceTotals(p *epathSQLVRFAllocationLedgerProof) (int64, int64, float64, error) {
	if p.Period == "annual" || len(p.Sources) == 0 {
		return 0, 0, 0, fmt.Errorf("VRF Monthly ledger lacks original constituent bindings")
	}
	month := epathSQLPeriodMonths(p.Period)[0]
	type roster struct {
		declaration   epathRealSQLVRFSystem
		locals, roles map[string]bool
	}
	systems := map[string]*roster{}
	zoneSystems := map[string]string{}
	ids := []int{}
	for id := range p.Sources {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	direct, allocated := int64(0), int64(0)
	native, nativeAllocated := 0.0, 0.0
	for _, id := range ids {
		binding := p.Sources[id]
		identity, a := binding.Identity, binding.Allocation
		o := identity.Observation
		if identity.Allocation != nil {
			return 0, 0, 0, fmt.Errorf("Monthly ledger cannot substitute an annual source allocation")
		}
		if err := epathSQLValidateVRFSourceIdentity(identity); err != nil {
			return 0, 0, 0, err
		}
		if id <= 0 || id != o.Source.DictionaryIndex || id != a.SourceID || o.Service != p.Service || a.Service != p.Service || o.Role != a.Role || o.Shared != a.Shared || !strings.EqualFold(o.ZoneName, a.ZoneName) || !reflect.DeepEqual(o.Months[month-1], a.Native) {
			return 0, 0, 0, fmt.Errorf("VRF ledger source changed original ID/role/owner/month")
		}
		budget, err := epathSQLVRFMilliBudget(o.Months[month-1].Value)
		if err != nil || a.BudgetMilliKWh != budget || a.UnassignedMilliKWh < 0 || a.UnassignedMilliKWh > budget {
			return 0, 0, 0, fmt.Errorf("VRF source budget is not its original Monthly round")
		}
		key := identity.System.OutdoorUnit.ObjectType + "|" + epathSQLVRFName(identity.System.OutdoorUnit.ObjectName)
		r := systems[key]
		if r == nil {
			r = &roster{declaration: identity.System, locals: map[string]bool{}, roles: map[string]bool{}}
			systems[key] = r
		} else if !reflect.DeepEqual(r.declaration, identity.System) {
			return 0, 0, 0, fmt.Errorf("VRF ledger mixed conflicting original system declarations")
		}
		owners := map[string]bool{}
		for _, terminal := range identity.System.Terminals {
			zone := epathSQLVRFName(terminal.ZoneName)
			owners[zone] = true
			if other := zoneSystems[zone]; other != "" && other != key {
				return 0, 0, 0, fmt.Errorf("VRF ledger shares an owner between original systems")
			}
			zoneSystems[zone] = key
		}
		shareMilli, shareNative := int64(0), 0.0
		seen := map[string]bool{}
		zones := []string{}
		for zone := range a.Shares {
			zones = append(zones, zone)
		}
		sort.Strings(zones)
		for _, name := range zones {
			share := a.Shares[name]
			zone := epathSQLVRFName(name)
			if !owners[zone] || seen[zone] || !epathOracleFinite(share.Native) || share.Native < 0 || share.DisplayMilliKWh < 0 {
				return 0, 0, 0, fmt.Errorf("VRF source has an unknown/duplicate/negative recipient")
			}
			seen[zone] = true
			shareMilli, err = epathSQLVRFMilliAdd(shareMilli, share.DisplayMilliKWh)
			if err != nil {
				return 0, 0, 0, err
			}
			shareNative += share.Native
		}
		if !epathSQLVRFNativeClose(shareNative+a.UnassignedNative, a.Native.Value) || shareMilli+a.UnassignedMilliKWh != budget {
			return 0, 0, 0, fmt.Errorf("VRF source shares do not preserve native and displayed budgets")
		}
		if o.Shared {
			if r.roles[o.Role] || len(seen) != len(owners) || a.UnassignedNative < 0 || !epathOracleFinite(a.UnassignedNative) {
				return 0, 0, 0, fmt.Errorf("VRF shared source lost its exact role/full owner roster")
			}
			r.roles[o.Role] = true
			// Complete independently observed denominators allocate the entire
			// source or leave the entire source unassigned; never a partial pool.
			if a.UnassignedNative != 0 {
				if !epathSQLVRFNativeClose(a.UnassignedNative, a.Native.Value) || a.UnassignedMilliKWh != budget || shareMilli != 0 || shareNative != 0 {
					return 0, 0, 0, fmt.Errorf("VRF shared source fabricated a partial denominator")
				}
			} else if a.UnassignedMilliKWh != 0 {
				return 0, 0, 0, fmt.Errorf("VRF shared display invented an unassigned source budget")
			}
			allocated, err = epathSQLVRFMilliAdd(allocated, shareMilli)
			nativeAllocated += shareNative
		} else {
			zone := epathSQLVRFName(o.ZoneName)
			if r.locals[zone] || len(seen) != 1 || !seen[zone] || a.UnassignedNative != 0 || a.UnassignedMilliKWh != 0 || shareMilli != budget || !epathSQLVRFNativeClose(shareNative, a.Native.Value) {
				return 0, 0, 0, fmt.Errorf("VRF local source was reassigned, duplicated or relabelled")
			}
			r.locals[zone] = true
			direct, err = epathSQLVRFMilliAdd(direct, budget)
		}
		if err != nil {
			return 0, 0, 0, err
		}
		native += a.Native.Value
	}
	for _, r := range systems {
		if len(r.locals) != len(r.declaration.Terminals) || len(r.roles) != 2 {
			return 0, 0, 0, fmt.Errorf("VRF ledger omitted an original local/shared constituent")
		}
	}
	if !epathSQLVRFNativeClose(native, p.NativeConstituents.Value) {
		return 0, 0, 0, fmt.Errorf("VRF ledger native total differs from its original source sum")
	}
	return direct, allocated, nativeAllocated, nil
}

func epathSQLValidateVRFLedger(p *epathSQLVRFAllocationLedgerProof) error {
	if p == nil || !epathOracleValidPeriod(p.Period) || !p.NativeExpected.valid() || !p.NativeConstituents.valid() || !epathSQLVRFNativeClose(p.NativeExpected.Value, p.NativeConstituents.Value) {
		return fmt.Errorf("VRF ledger lacks exact original broad/constituent closure")
	}
	id, err := epathSQLAllocationID(p.AnnualID, p.Period)
	if err != nil || p.ID != id || p.Service != "cooling" && p.Service != "heating" || p.Basis != "service_path_allocation" {
		return fmt.Errorf("VRF ledger lost its exact reviewed ID/service/basis")
	}
	values := map[string]int64{}
	for field, q := range epathSQLVRFLedgerFields(p) {
		value, err := epathSQLVRFDisplayMilli(q)
		if err != nil || field != "residualValue" && value < 0 {
			return fmt.Errorf("invalid VRF displayed ledger field %s: %v", field, err)
		}
		values[field] = value
	}
	if p.Period != "annual" {
		direct, allocated, nativeAllocated, err := epathSQLVRFLedgerSourceTotals(p)
		if err != nil {
			return err
		}
		if direct != values["directValue"] || allocated != values["allocatedValue"] {
			return fmt.Errorf("VRF ledger moved displayed energy between original local and shared sources")
		}
		method := "unassigned"
		if nativeAllocated > 1e-9 {
			method = "service_path_load_share"
		} else if direct > 0 && values["unassignedValue"] == 0 {
			method = "direct_only"
		}
		if p.Method != method {
			return fmt.Errorf("VRF Monthly allocation method contradicts its original source allocation")
		}
		if len(p.Months) != 0 || values["expectedValue"] != int64(math.Round(p.NativeExpected.Value*1000)) ||
			values["residualValue"] != values["expectedValue"]-values["directValue"]-values["allocatedValue"] ||
			values["unassignedValue"] != max(int64(0), values["residualValue"]) {
			return fmt.Errorf("VRF Monthly ledger changed its original display budget or signed residual")
		}
		return nil
	}
	if len(p.Months) != 12 || len(p.Sources) != 0 {
		return fmt.Errorf("VRF Annual ledger requires twelve completed original Monthly ledgers")
	}
	sums := map[string]int64{}
	nativeExpected, nativeConstituents := 0.0, 0.0
	method := "unassigned"
	for index := range p.Months {
		month := &p.Months[index]
		if month.Period != fmt.Sprintf("M%d", index+1) || month.AnnualID != p.AnnualID || month.Service != p.Service || month.Basis != p.Basis {
			return fmt.Errorf("VRF Annual ledger duplicated or lost an original month")
		}
		if err := epathSQLValidateVRFLedger(month); err != nil {
			return err
		}
		if len(month.Sources) != len(p.Months[0].Sources) {
			return fmt.Errorf("VRF Annual source roster changed between months")
		}
		for id, source := range month.Sources {
			first, exists := p.Months[0].Sources[id]
			if !exists || !reflect.DeepEqual(first.Identity, source.Identity) {
				return fmt.Errorf("VRF Annual ledger substituted a different original source identity")
			}
		}
		if month.Method == "service_path_load_share" {
			method = "service_path_load_share"
		}
		for field, q := range epathSQLVRFLedgerFields(month) {
			value, _ := epathSQLVRFDisplayMilli(q)
			sums[field] += value
		}
		nativeExpected += month.NativeExpected.Value
		nativeConstituents += month.NativeConstituents.Value
	}
	for field, value := range values {
		if value != sums[field] {
			return fmt.Errorf("VRF Annual ledger is not its completed Monthly %s sum", field)
		}
	}
	if !epathSQLVRFNativeClose(nativeExpected, p.NativeExpected.Value) || !epathSQLVRFNativeClose(nativeConstituents, p.NativeConstituents.Value) {
		return fmt.Errorf("VRF Annual native authority is not its original Monthly sum")
	}
	if method != "service_path_load_share" && values["directValue"] > 0 && values["unassignedValue"] == 0 {
		method = "direct_only"
	}
	if p.Method != method {
		return fmt.Errorf("VRF Annual method is not its completed Monthly allocation provenance")
	}
	return nil
}

// This narrowly typed proof retains native SQL closure and the separately
// derived display ledger. The legacy raw-conservation validator is unchanged.
func epathCheckSQLVRFAllocation(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	if check.Allocation == nil || check.Allocation.NativeVRF == nil {
		return fmt.Errorf("missing independent VRF display ledger")
	}
	p, target := check.Allocation.NativeVRF, check.Item.Target
	if err := epathSQLValidateVRFLedger(p); err != nil {
		return err
	}
	if check.Item.Scope != "building" || check.Item.Zone != "" || check.Item.Period != p.Period || check.Item.Group != "zoneAllocation" || target.Collection != "reconciliation" || target.Level != "allocation" || target.ID != p.ID || target.Unit != "kWh" {
		return fmt.Errorf("VRF display ledger escaped its exact Building allocation context")
	}
	if err := epathValidateOracleTarget(target, check.Item.Unit); err != nil {
		return err
	}
	fields := epathSQLVRFLedgerFields(p)
	for field, q := range check.Allocation.fields() {
		if q == nil || epathSQLVRFRequireDisplayValue(q.Value, fields[field]) != nil {
			return fmt.Errorf("VRF allocation check changed independently compiled field %s", field)
		}
	}
	selected, ok := fields[target.Field]
	if !ok || check.Quantity == nil || epathSQLVRFRequireDisplayValue(check.Quantity.Value, selected) != nil {
		return fmt.Errorf("VRF selected ledger scalar contradicts its independent presentation proof")
	}
	_, _, rows, _, err := epathOracleGraph(bundle, "building", "", p.Period)
	if err != nil {
		return err
	}
	var row *EnergyReconciliation
	for index := range rows {
		if rows[index].ID == target.ID {
			if row != nil {
				return fmt.Errorf("duplicate native VRF allocation ledger")
			}
			row = &rows[index]
		}
	}
	if row == nil {
		if p.Expected.Value == 0 && p.Direct.Value == 0 && p.Allocated.Value == 0 && p.Unassigned.Value == 0 && p.Residual.Value == 0 {
			return nil
		}
		return fmt.Errorf("missing native VRF allocation ledger")
	}
	if row.ID != p.ID || row.ServiceKind != p.Service || row.AllocationMethod != p.Method || row.Basis != p.Basis || row.ZoneName != "" || row.Period != p.Period || row.Level != "allocation" || row.Unit != "kWh" {
		return fmt.Errorf("VRF allocation row changed its original service/method/basis/context")
	}
	actual, err := epathReadOracleCandidate(bundle, check.Item, check.Want)
	if err != nil || actual == nil {
		return fmt.Errorf("VRF allocation row has contradictory metadata: %v", err)
	}
	for field, value := range map[string]float64{"expectedValue": row.ExpectedValue, "directValue": row.DirectValue, "allocatedValue": row.AllocatedValue, "unassignedValue": row.UnassignedValue, "residualValue": row.ResidualValue} {
		if err := epathSQLVRFRequireDisplayValue(value, fields[field]); err != nil {
			return fmt.Errorf("VRF ledger %s: %w", field, err)
		}
	}
	overmapped := math.Max(0, -p.Residual.Value)
	if p.Period == "annual" {
		overmapped = 0
		for _, month := range p.Months {
			overmapped += math.Max(0, -month.Residual.Value)
		}
	}
	return epathSQLVRFRequireDisplayValue(row.OvermappedValue, epathSQLQuantity{Value: overmapped})
}
