package simulation

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathSQLReconciliationUnitBundle(p epathSQLReconciliationProof) PurposeResultBundle {
	row := EnergyReconciliation{ID: p.ID, Level: p.Level, ZoneName: p.ZoneName, Period: p.Period, ServiceKind: p.Service, Basis: p.Basis, Unit: p.Unit, Status: p.Status, ExpectedValue: p.Expected.Value, ExplainedValue: p.Explained.Value, ResidualValue: p.Residual.Value}
	period := EnergyPeriod{ID: p.Period, Kind: "monthly", Reconciliation: []EnergyReconciliation{row}}
	if p.Period == "annual" {
		period.Kind = "annual"
	}
	return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, AvailableZones: []string{p.ZoneName}, Reconciliation: []EnergyReconciliation{row}, Periods: []EnergyPeriod{period}, ZoneResults: []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: p.ZoneName}, Reconciliation: []EnergyReconciliation{row}, Periods: []EnergyPeriod{period}}}}}
}

func epathSQLReconciliationUnitCheck(t *testing.T, p epathSQLReconciliationProof, scope string) epathSQLModelCheck {
	t.Helper()
	var checks epathSQLModelChecks
	zone := ""
	if scope == "zone" {
		zone = p.ZoneName
	}
	if err := epathSQLAddReconciliation(&checks, scope, zone, p); err != nil {
		t.Fatal(err)
	}
	return checks.Rows[0]
}

func TestEnergyPathRealSQLReconciliationWholeRowAndSignedValues(t *testing.T) {
	for _, values := range [][2]float64{{0, 0}, {0.0001, 0.0002}, {3, 5}, {-3, -5}, {10, 0}} {
		p := epathSQLReconciliationProof{ID: "reconcile.driver.surface.office.annual", Level: "driver", ZoneName: "Office", Period: "annual", Basis: "residual", Unit: "kWh", Expected: epathSQLQuantity{Value: values[0]}, Explained: epathSQLQuantity{Value: values[1]}, Residual: epathSQLQuantity{Value: values[0] - values[1]}}
		for _, scope := range []string{"building", "zone"} {
			check := epathSQLReconciliationUnitCheck(t, p, scope)
			bundle := epathSQLReconciliationUnitBundle(p)
			if err := epathCheckSQLModelReconciliation(bundle, check); err != nil {
				t.Fatalf("known signed/tiny %v/%s: %v", values, scope, err)
			}
			bundle.EnergyExplanation.Reconciliation = nil
			bundle.EnergyExplanation.Periods[0].Reconciliation = nil
			bundle.EnergyExplanation.ZoneResults[0].Reconciliation = nil
			bundle.EnergyExplanation.ZoneResults[0].Periods[0].Reconciliation = nil
			err := epathCheckSQLModelReconciliation(bundle, check)
			if (err == nil) != (values[0] == 0 && values[1] == 0) {
				t.Fatalf("whole row pruning %v: %v", values, err)
			}
		}
	}
	p := epathSQLReconciliationProof{ID: "reconcile.heat.cooling.office.annual", Level: "heat", ZoneName: "Office", Service: "cooling", Period: "annual", Basis: "residual", Unit: "kWh", Expected: epathSQLQuantity{Value: 0.0001, Error: 0.0005}, Explained: epathSQLQuantity{Value: 0.0002, Error: 0.0005}, Residual: epathSQLQuantity{Value: -0.0001, Error: 0.001}}
	check := epathSQLReconciliationUnitCheck(t, p, "building")
	bundle := epathSQLReconciliationUnitBundle(p)
	bundle.EnergyExplanation.Reconciliation = nil
	bundle.EnergyExplanation.Periods[0].Reconciliation = nil
	if err := epathCheckSQLModelReconciliation(bundle, check); err != nil {
		t.Fatalf("known count-bounded zero-possible whole row: %v", err)
	}
}

func TestEnergyPathRealSQLReconciliationRejectsMisroutingDuplicatesAndAnnualDrift(t *testing.T) {
	p := epathSQLReconciliationProof{ID: "reconcile.heat.heating.office.annual", Level: "heat", ZoneName: "Office", Service: "heating", Period: "annual", Basis: "residual", Unit: "kWh", Expected: epathSQLQuantity{Value: 10}, Explained: epathSQLQuantity{Value: 12}, Residual: epathSQLQuantity{Value: -2}}
	check := epathSQLReconciliationUnitCheck(t, p, "building")
	for name, edit := range map[string]func(*EnergyReconciliation){
		"zone": func(r *EnergyReconciliation) { r.ZoneName = "Lab" }, "period": func(r *EnergyReconciliation) { r.Period = "M1" },
		"service": func(r *EnergyReconciliation) { r.ServiceKind = "cooling" }, "level": func(r *EnergyReconciliation) { r.Level = "energy" },
		"basis": func(r *EnergyReconciliation) { r.Basis = "reported_variable" }, "unit": func(r *EnergyReconciliation) { r.Unit = "J" },
		"sign": func(r *EnergyReconciliation) { r.ResidualValue = 2 }, "NaN": func(r *EnergyReconciliation) { r.ExpectedValue = math.NaN() },
		"infinity": func(r *EnergyReconciliation) { r.ExplainedValue = math.Inf(1) },
	} {
		t.Run(name, func(t *testing.T) {
			b := epathSQLReconciliationUnitBundle(p)
			edit(&b.EnergyExplanation.Reconciliation[0])
			if epathCheckSQLModelReconciliation(b, check) == nil {
				t.Fatal("accepted misrouted/nonfinite row")
			}
		})
	}
	for _, scope := range []string{"building", "zone"} {
		check := epathSQLReconciliationUnitCheck(t, p, scope)
		b := epathSQLReconciliationUnitBundle(p)
		if scope == "building" {
			b.EnergyExplanation.Periods[0].Reconciliation[0].ResidualValue = 0
		} else {
			b.EnergyExplanation.ZoneResults[0].Periods[0].Reconciliation[0].ResidualValue = 0
		}
		if epathCheckSQLModelReconciliation(b, check) == nil {
			t.Fatal("annual wrapper drift passed")
		}
	}
	b := epathSQLReconciliationUnitBundle(p)
	b.EnergyExplanation.Reconciliation = append(b.EnergyExplanation.Reconciliation, b.EnergyExplanation.Reconciliation[0])
	if epathCheckSQLModelReconciliation(b, check) == nil {
		t.Fatal("duplicate row passed")
	}
	p.AllowedIDs = []string{p.ID, p.ID + ".other"}
	check = epathSQLReconciliationUnitCheck(t, p, "building")
	b = epathSQLReconciliationUnitBundle(p)
	b.EnergyExplanation.Reconciliation[0].ID = p.AllowedIDs[1]
	b.EnergyExplanation.Periods[0].Reconciliation[0].ID = p.AllowedIDs[1]
	if err := epathCheckSQLModelReconciliation(b, check); err != nil {
		t.Fatalf("independently allowed exact alternative: %v", err)
	}
	b.EnergyExplanation.Reconciliation = append(b.EnergyExplanation.Reconciliation, EnergyReconciliation{ID: p.ID})
	if epathCheckSQLModelReconciliation(b, check) == nil {
		t.Fatal("two independently possible identities cannot both be present")
	}
}

func TestEnergyPathRealSQLReconciliationZoneIdentityIgnoresOnlyCase(t *testing.T) {
	p := epathSQLReconciliationProof{ID: "reconcile.heat.heating.core_mid.annual", Level: "heat", ZoneName: "Core_mid", Service: "heating", Period: "annual", Basis: "residual", Unit: "kWh", Expected: epathSQLQuantity{Value: 10}, Explained: epathSQLQuantity{Value: 8}, Residual: epathSQLQuantity{Value: 2}}
	for _, scope := range []string{"building", "zone"} {
		check := epathSQLReconciliationUnitCheck(t, p, scope)
		bundle := epathSQLReconciliationUnitBundle(p)
		bundle.EnergyExplanation.Reconciliation[0].ZoneName = "CORE_MID"
		bundle.EnergyExplanation.Periods[0].Reconciliation[0].ZoneName = "core_MID"
		bundle.EnergyExplanation.ZoneResults[0].Scope.ZoneName = "CORE_MID"
		bundle.EnergyExplanation.ZoneResults[0].Reconciliation[0].ZoneName = "CORE_mid"
		if err := epathCheckSQLModelReconciliation(bundle, check); err != nil {
			t.Fatalf("actual SQL uppercase Zone identity %s: %v", scope, err)
		}
		if !epathSQLReconciliationMatches(bundle.EnergyExplanation.Reconciliation[0], &p) {
			t.Fatal("coverage disagrees with case-insensitive Zone identity")
		}
	}
	for _, field := range []string{"id", "unit", "period", "service", "basis"} {
		row := epathSQLReconciliationUnitBundle(p).EnergyExplanation.Reconciliation[0]
		switch field {
		case "id":
			row.ID = strings.ToUpper(row.ID)
		case "unit":
			row.Unit = "KWH"
		case "period":
			row.Period = "ANNUAL"
		case "service":
			row.ServiceKind = "HEATING"
		case "basis":
			row.Basis = "RESIDUAL"
		}
		if epathSQLReconciliationMatches(row, &p) {
			t.Fatalf("case folding escaped Zone-only contract: %s", field)
		}
	}
}

func TestEnergyPathRealSQLReconciliationWireMissingIsNotZero(t *testing.T) {
	for _, context := range []string{"building", "building-period", "zone", "zone-period"} {
		for _, field := range []string{"expectedValue", "explainedValue", "residualValue"} {
			for _, null := range []bool{false, true} {
				fixture := epathOracleWireFixture()
				row := epathOracleWireFixtureGraph(fixture, context)["reconciliation"].([]any)[0].(map[string]any)
				if null {
					row[field] = nil
				} else {
					delete(row, field)
				}
				if err := epathOracleWireFixtureCheck(t, fixture); err == nil {
					t.Fatalf("%s/%s null=%v bypassed saved candidate preflight", context, field, null)
				}
			}
		}
	}
}

func TestEnergyPathRealSQLReconciliationActualFramesMultiplierAndHiddenDrivers(t *testing.T) {
	path, model := epathSQLModelUnitFixture(t)
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	for _, cell := range frames.Cells {
		cell.BuildingVisible = false
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelThermalReconciliationChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 234 {
		t.Fatalf("one Zone + Building/global/Zone-copy, 2 services*13 periods*3 fields: %d", len(checks.Rows))
	}
	for _, check := range checks.Rows {
		p := check.Reconciliation
		if p.Service == "cooling" && p.Period == "annual" {
			if math.Abs(p.Expected.Value-216) > 1e-9 || math.Abs(p.Explained.Value-216) > 1e-9 || math.Abs(p.Residual.Value) > 1e-9 {
				t.Fatalf("SQL 3 kWh * Zone2 * Group3 *12 was not retained: %#v", p)
			}
		}
	}
	delete(frames.Loads, epathSQLKey("office", "cooling", 2))
	if epathSQLModelThermalReconciliationChecks(frames, model, &epathSQLModelChecks{}) == nil {
		t.Fatal("missing SQL month fabricated zero")
	}
}

func epathSQLThermalEquationUnitFrames() (epathSQLFrames, epathRealSQLModel) {
	f := epathSQLFrames{Zones: map[string]epathSQLZone{"office": {Name: "Office", Multiplier: 6}}, Cells: map[string]*epathSQLCell{}, Loads: map[string]epathSQLQuantity{}, SourceRaw: map[int][]epathSQLQuantity{}, SourceZone: map[int]string{}, SourceIdentities: map[int]epathRealSQLSource{}}
	m := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}}
	addFamily := func(id, category, component, name string, subtract []string) {
		family := epathRealSQLFamily{ID: id, Keys: []string{"Office"}, Category: category, Component: component, Role: "pressure", Subtract: subtract}
		if name != "" {
			dictionaryID := len(f.SourceIdentities) + 1
			family.Terms = []epathRealSQLTerm{{Sign: 1, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: name, Unit: "J"}}, Keys: []string{"Office"}}}}
			f.SourceIdentities[dictionaryID] = epathRealSQLSource{DictionaryIndex: dictionaryID, Name: name, KeyValue: "Office", SourceUnit: "J", ReportingFrequency: "Monthly"}
			f.SourceZone[dictionaryID] = "office"
			f.SourceRaw[dictionaryID] = make([]epathSQLQuantity, 12)
		}
		m.Families = append(m.Families, family)
	}
	addFamily("people.sensible", "internal.people", "sensible", "", nil)
	addFamily("people.latent", "internal.people", "latent", "", nil)
	addFamily("infiltration.sensible.gain", "air.infiltration", "sensible", "", nil)
	addFamily("surface.balance", "balance.storage_other", "sensible", "Zone Surface", []string{"surface.total"})
	addFamily("internal.other.sensible", "internal.other", "sensible", "Internal Sensible", []string{"people.sensible"})
	addFamily("internal.other.latent", "internal.other", "latent", "Internal Latent", []string{"people.latent"})
	addFamily("ventilation.sensible", "air.mechanical_ventilation", "sensible", "Outdoor Total", []string{"infiltration.sensible.gain"})
	for month := 1; month <= 12; month++ {
		sign := 1.0
		if month > 6 {
			sign = -1
		}
		for id := range f.SourceRaw {
			value := float64(id+1) * sign
			f.SourceRaw[id][month-1] = epathSQLQuantity{Value: value}
			// Selectors require real observed monthly identities; no annual
			// scalar or candidate source can substitute for these twelve frames.
			s := f.SourceIdentities[id]
			s.Months = append(s.Months, epathRealSQLMonth{Month: month, Rows: 1, EnergyKWh: epathOracleNumber(value)})
			f.SourceIdentities[id] = s
		}
		values := map[string]float64{"surface:surface.exterior_walls": sign * 6, "people.sensible": sign * 12, "people.latent": sign * 18, "infiltration.sensible.gain": sign * 6, "surface.balance": sign * 6, "internal.other.sensible": sign * 6, "internal.other.latent": sign * 6, "ventilation.sensible": sign * 24}
		for id, value := range values {
			category, component := "surface.exterior_walls", "sensible"
			for _, family := range m.Families {
				if family.ID == id {
					category, component = family.Category, family.Component
				}
			}
			f.Cells[epathSQLKey("office", id, month)] = &epathSQLCell{Zone: "office", Family: id, Month: month, Category: category, Component: component, Effective: epathSQLQuantity{Value: value}, Raw: epathSQLQuantity{Value: value / 6}, Allocated: map[string]epathSQLQuantity{"cooling": {}, "heating": {}}}
		}
		for _, service := range []string{"cooling", "heating"} {
			f.Loads[epathSQLKey("office", service, month)] = epathSQLQuantity{}
		}
	}
	return f, m
}

func TestEnergyPathRealSQLReconciliationThermalEquationsMonthFirst(t *testing.T) {
	frames, model := epathSQLThermalEquationUnitFrames()
	before, _ := json.Marshal(frames)
	var checks epathSQLModelChecks
	if err := epathSQLModelThermalReconciliationChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	wanted := map[string][3]float64{"surface": {12, 6, 6}, "internal.component.sensible": {18, 12, 6}, "internal.component.latent": {24, 18, 6}, "outdoor_air": {30, 30, 0}}
	seen := map[string]bool{}
	for _, check := range checks.Rows {
		p := check.Reconciliation
		if p.Level != "driver" || check.Item.Scope != "building" || check.Item.Target.Field != "expectedValue" {
			continue
		}
		key := strings.TrimSuffix(strings.TrimPrefix(p.ID, "reconcile.driver."), "."+p.Period)
		key = strings.ReplaceAll(key, ".office", "")
		want, exists := wanted[key]
		if !exists {
			t.Fatalf("unreviewed identity %s", p.ID)
		}
		if p.Period == "M1" {
			seen[key] = true
			if p.Expected.Value != want[0] || p.Explained.Value != want[1] || p.Residual.Value != want[2] {
				t.Fatalf("original SQL term vs dependency equation %s: %#v", key, p)
			}
		}
		if p.Period == "M7" && (p.Expected.Value != -want[0] || p.Explained.Value != -want[1] || p.Residual.Value != -want[2]) {
			t.Fatalf("signed negative month %s: %#v", key, p)
		}
		if p.Period == "annual" && (p.Expected.Value != 0 || p.Explained.Value != 0 || p.Residual.Value != 0) {
			t.Fatalf("month-first signed cancellation %s: %#v", key, p)
		}
	}
	if len(seen) != len(wanted) {
		t.Fatalf("missing nonvacuous independent family checks: %v", seen)
	}
	after, _ := json.Marshal(frames)
	if string(before) != string(after) {
		t.Fatal("compiler mutated independent frames")
	}
}

func TestEnergyPathRealSQLReconciliationRejectsIncompleteOrAmbiguousFrames(t *testing.T) {
	for name, edit := range map[string]func(*epathSQLFrames, *epathRealSQLModel){
		"missing whole cell": func(f *epathSQLFrames, _ *epathRealSQLModel) {
			delete(f.Cells, epathSQLKey("office", "people.sensible", 1))
		},
		"invalid multiplier": func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.Zones["office"] = epathSQLZone{Name: "Office", Multiplier: 0}
		},
		"allocated absent": func(f *epathSQLFrames, _ *epathRealSQLModel) {
			delete(f.Cells[epathSQLKey("office", "people.sensible", 1)].Allocated, "heating")
		},
		"allocated NaN": func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.Cells[epathSQLKey("office", "people.sensible", 1)].Allocated["cooling"] = epathSQLQuantity{Value: math.NaN()}
		},
		"cell wrong month": func(f *epathSQLFrames, _ *epathRealSQLModel) {
			f.Cells[epathSQLKey("office", "people.sensible", 1)].Month = 2
		},
		"aggregate missing":       func(f *epathSQLFrames, _ *epathRealSQLModel) { delete(f.SourceIdentities, 1) },
		"aggregate missing month": func(f *epathSQLFrames, _ *epathRealSQLModel) { f.SourceRaw[1] = f.SourceRaw[1][:11] },
		"aggregate wrong owner":   func(f *epathSQLFrames, _ *epathRealSQLModel) { f.SourceZone[1] = "lab" },
		"duplicate aggregate": func(f *epathSQLFrames, _ *epathRealSQLModel) {
			s := f.SourceIdentities[1]
			s.DictionaryIndex = 20
			f.SourceIdentities[20] = s
		},
		"unknown detail": func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Families[4].Subtract = []string{"mystery"} },
		"duplicate detail": func(_ *epathSQLFrames, m *epathRealSQLModel) {
			m.Families[4].Subtract = []string{"people.sensible", "people.sensible"}
		},
		"outdoor wrong detail": func(_ *epathSQLFrames, m *epathRealSQLModel) { m.Families[6].Subtract = []string{"people.sensible"} },
	} {
		t.Run(name, func(t *testing.T) {
			f, m := epathSQLThermalEquationUnitFrames()
			edit(&f, &m)
			if err := epathSQLModelThermalReconciliationChecks(f, m, &epathSQLModelChecks{}); err == nil {
				t.Fatal("incomplete/ambiguous SQL frames accepted")
			}
		})
	}
	if got := epathSQLReconciliationToken(" Core / Mid 01 "); got != "core_mid_01" {
		t.Fatal(got)
	}
	if !reflect.DeepEqual(epathSQLPeriodMonths("annual"), []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}) {
		t.Fatal("annual period contract changed")
	}
}
