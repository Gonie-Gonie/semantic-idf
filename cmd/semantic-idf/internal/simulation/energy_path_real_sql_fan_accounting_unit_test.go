package simulation

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestEnergyPathRealSQLFanAccountingCountsPoolRoundingSeparately(t *testing.T) {
	path := epathFanPoolUnitSQL(t)
	// Both independently reported pools round down by .0004 kWh. Their
	// conserved allocation is 400, while the separate meter is 400.0008.
	epathOracleEditSQL(t, path, `UPDATE ReportData SET Value=Value*1.000004 WHERE ReportDataDictionaryIndex=101;
UPDATE ReportData SET Value=Value*(300.0004/300.0) WHERE ReportDataDictionaryIndex=202;
UPDATE ReportData SET Value=400.0008*3600000 WHERE ReportDataDictionaryIndex=303;`)
	observed, frames, pools, precision := epathFanPoolUnitInputs(t, path)
	for month := range frames.Site["fans"] {
		frames.Site["fans"][month] = &epathSQLQuantity{Value: 400.0008, Error: .0005}
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelFanPoolChecks(observed, frames, pools, precision, &checks); err != nil {
		t.Fatal(err)
	}
	model := epathRealSQLModel{FanPools: pools}
	aux := epathRealSQLAuxiliary{SiteID: "fans", ServedZones: []string{"A", "B", "C", "D"}, Weight: "cooling_plus_heating", AllocationMethod: "air_loop_load_share", ReconciliationID: "reconcile.fans.annual"}
	meter := *frames.Site["fans"][0]
	var annual epathSQLQuantity
	for month := 1; month <= 12; month++ {
		got, err := epathSQLFanAllocatedMonth(checks, model, aux, month, meter)
		if err != nil || got == nil {
			t.Fatalf("M%d: %v", month, err)
		}
		low, high := got.bounds()
		if math.Abs(got.Value-meter.Value) > 1e-7 || math.Abs((high-low)-.002) > 1e-9 {
			t.Fatalf("pool centers/independent two-source precision changed: %+v", got)
		}
		if err := epathCheckSQLModelQuantity(epathOracleNumber(400), got); err != nil {
			t.Fatalf("conserved separately rounded pool sum failed: %v", err)
		}
		if epathCheckSQLModelQuantity(epathOracleNumber(399.999), got) == nil {
			t.Fatal("unbounded allocation drift accepted")
		}
		annual = annual.add(*got)
	}
	if epathCheckSQLModelQuantity(epathOracleNumber(400), &meter) == nil {
		t.Fatal("regression must distinguish the narrower independent meter interval")
	}
	if !reflect.DeepEqual(meter, *frames.Site["fans"][0]) {
		t.Fatal("allocation changed expected meter observations")
	}
	if err := epathCheckSQLModelQuantity(epathOracleNumber(4800), &annual); err != nil {
		t.Fatalf("Annual must sum twelve completed pool intervals: %v", err)
	}
	model.Precision, model.Auxiliaries = precision, []epathRealSQLAuxiliary{aux}
	for _, service := range []string{"cooling", "heating"} {
		for month := 0; month < 12; month++ {
			frames.Site[service] = append(frames.Site[service], &epathSQLQuantity{Value: 10})
		}
		model.Services = append(model.Services, epathRealSQLService{Service: service, SiteIDs: []string{service}, ServedZones: aux.ServedZones, Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "load_to_site_energy", FallbackRatioKind: "load_to_site_energy", ReconciliationID: "reconcile." + service + ".annual"})
	}
	if err := epathSQLModelServiceChecks(frames, model, &checks); err != nil {
		t.Fatalf("audited fan total was not consumed by auxiliary accounting: %v", err)
	}
	selected := 0
	for _, check := range checks.Rows {
		if check.Item.Key != "zoneAllocation|building||M1|fans/allocatedValue" {
			continue
		}
		selected++
		if check.Allocation == nil || epathCheckSQLModelQuantity(epathOracleNumber(400), check.Allocation.Allocated) != nil || epathCheckSQLModelQuantity(epathOracleNumber(400), check.Allocation.Expected) == nil {
			t.Fatal("whole allocation proof conflates the meter and pool intervals")
		}
	}
	if selected != 1 {
		t.Fatal("missing/duplicate exact monthly fan allocation proof")
	}
	for _, bad := range []epathRealSQLAuxiliary{
		{SiteID: "fans", Weight: "unassigned", AllocationMethod: "unassigned", ReconciliationID: aux.ReconciliationID},
		{SiteID: "fans", Weight: aux.Weight, AllocationMethod: aux.AllocationMethod, ServedZones: []string{"Unserved"}, ReconciliationID: aux.ReconciliationID},
	} {
		m, c := model, epathSQLModelChecks{FanPoolTotals: checks.FanPoolTotals}
		m.Auxiliaries = []epathRealSQLAuxiliary{bad}
		// Force the unrelated owner's load to zero: neither unassigned nor
		// zero-weight control flow may bypass the reviewed-pool policy check.
		for month := 1; month <= 12; month++ {
			frames.Loads[epathSQLKey("Unserved", "cooling", month)] = epathSQLQuantity{}
		}
		if err := epathSQLModelServiceChecks(frames, m, &c); err == nil || !strings.Contains(err.Error(), "fan") {
			t.Fatalf("contradictory pool policy bypassed integration guard: %v", err)
		}
	}
	for name, mutate := range map[string]func(*epathSQLModelChecks, *epathRealSQLModel, *epathRealSQLAuxiliary, *int, *epathSQLQuantity){
		"missing audit": func(c *epathSQLModelChecks, _ *epathRealSQLModel, _ *epathRealSQLAuxiliary, _ *int, _ *epathSQLQuantity) {
			c.FanPoolTotals = nil
		},
		"missing owner": func(_ *epathSQLModelChecks, _ *epathRealSQLModel, a *epathRealSQLAuxiliary, _ *int, _ *epathSQLQuantity) {
			a.ServedZones = []string{"A", "B", "C"}
		},
		"foreign owner": func(_ *epathSQLModelChecks, _ *epathRealSQLModel, a *epathRealSQLAuxiliary, _ *int, _ *epathSQLQuantity) {
			a.ServedZones = []string{"A", "B", "C", "Unserved"}
		},
		"duplicate owner": func(_ *epathSQLModelChecks, _ *epathRealSQLModel, a *epathRealSQLAuxiliary, _ *int, _ *epathSQLQuantity) {
			a.ServedZones = []string{"A", "B", "C", "D", "a"}
		},
		"changed pool": func(_ *epathSQLModelChecks, m *epathRealSQLModel, _ *epathRealSQLAuxiliary, _ *int, _ *epathSQLQuantity) {
			m.FanPools = append([]epathRealSQLFanPool(nil), m.FanPools...)
			m.FanPools[0].Key = "OTHER LOOP"
		},
		"changed method": func(_ *epathSQLModelChecks, _ *epathRealSQLModel, a *epathRealSQLAuxiliary, _ *int, _ *epathSQLQuantity) {
			a.AllocationMethod = "airflow_share"
		},
		"unassigned": func(_ *epathSQLModelChecks, _ *epathRealSQLModel, a *epathRealSQLAuxiliary, _ *int, _ *epathSQLQuantity) {
			a.Weight = "unassigned"
		},
		"bad month": func(_ *epathSQLModelChecks, _ *epathRealSQLModel, _ *epathRealSQLAuxiliary, m *int, _ *epathSQLQuantity) {
			*m = 0
		},
		"changed meter": func(_ *epathSQLModelChecks, _ *epathRealSQLModel, _ *epathRealSQLAuxiliary, _ *int, q *epathSQLQuantity) {
			q.Value = 401
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, m, a, month, q := checks, model, aux, 1, meter
			mutate(&c, &m, &a, &month, &q)
			if _, err := epathSQLFanAllocatedMonth(c, m, a, month, q); err == nil {
				t.Fatal("unreviewed allocation accounting accepted")
			}
		})
	}
	aux.SiteID = "pumps"
	if q, err := epathSQLFanAllocatedMonth(checks, model, aux, 1, epathSQLQuantity{Value: 1e-8}); err != nil || q != nil {
		t.Fatalf("unrelated tiny pump must retain its original independent accounting: %v %v", q, err)
	}
}

func TestEnergyPathRealSQLFanAccountingRequiresCompletedAudit(t *testing.T) {
	path := epathFanPoolUnitSQL(t)
	epathOracleEditSQL(t, path, `UPDATE ReportData SET Value=Value+3600000 WHERE ReportDataDictionaryIndex=303 AND TimeIndex=20001;`)
	observed, frames, pools, precision := epathFanPoolUnitInputs(t, path)
	var checks epathSQLModelChecks
	if err := epathSQLModelFanPoolChecks(observed, frames, pools, precision, &checks); err == nil {
		t.Fatal("nonclosing original pool partition accepted")
	}
	if checks.FanPoolTotals != nil {
		t.Fatal("failed audit left an apparently approved allocation interval")
	}
}
