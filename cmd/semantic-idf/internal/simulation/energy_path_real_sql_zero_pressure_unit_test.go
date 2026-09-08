package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathSQLZeroPressureUnitFixture(t *testing.T) (string, epathRealSQLModel) {
	t.Helper()
	path, model := epathSQLModelUnitFixture(t)
	// Original signs are surface +4 / People +1 kWh for cooling. Only M8
	// has heating .25 kWh, multiplied by the original Zone 2*3 => 1.5.
	// All other heating months are measured exact zero, not absent.
	epathOracleEditSQL(t, path, `UPDATE ReportData SET Value=900000 WHERE ReportDataDictionaryIndex=11 AND TimeIndex=8`)
	model.ZeroPressureFallbacks = []epathRealSQLZeroPressureFallback{{ZoneName: "Office", Month: 8, Service: "heating"}}
	return path, model
}

func epathSQLZeroPressureUnitFrames(t *testing.T) (epathSQLFrames, epathRealSQLModel) {
	t.Helper()
	path, model := epathSQLZeroPressureUnitFixture(t)
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	return frames, model
}

func TestEnergyPathRealSQLZeroPressureIndependentMonthFirst(t *testing.T) {
	frames, model := epathSQLZeroPressureUnitFrames(t)
	// Literal .25 kWh * 6 permits only machine arithmetic roundoff here,
	// not the separate displayed-value precision budget.
	literalEqual := func(value, want float64) bool {
		return math.Abs(value-want) <= 4*(math.Nextafter(want, math.Inf(1))-want)
	}
	loadEqual := func(value float64) bool { return literalEqual(value, 1.5) }
	if len(frames.ZeroPressureFallbacks) != 1 || len(frames.ZeroPressurePhysicalCells) != 2 || len(frames.Cells) != 25 {
		t.Fatal("declaration changed or duplicated the original physical monthly roster")
	}
	fallback := frames.Cells[epathSQLZeroPressureCellKey(model.ZeroPressureFallbacks[0])]
	if !epathSQLExactZero(fallback.Raw) || !epathSQLExactZero(fallback.Effective) || !loadEqual(fallback.Allocated["heating"].Value) || !reflect.DeepEqual(fallback.SourceIDs, []int{11}) || fallback.Component != "combined" {
		t.Fatalf("load-only fallback lost observed zero/multiplier/native identity: %#v", fallback)
	}
	for month := 1; month <= 12; month++ {
		surface := frames.Cells[epathSQLKey("Office", "surface:surface.exterior_walls", month)]
		people := frames.Cells[epathSQLKey("Office", "people", month)]
		if !literalEqual(surface.Raw.Value, 4) || !literalEqual(surface.Effective.Value, 24) || !literalEqual(people.Raw.Value, 1) || !literalEqual(surface.Allocated["cooling"].Value, 14.4) || !literalEqual(people.Allocated["cooling"].Value, 3.6) || !epathSQLExactZero(surface.Allocated["heating"]) || !epathSQLExactZero(people.Allocated["heating"]) {
			t.Fatalf("M%d fallback altered original physical values, cooling shares, or zero heating allocations: surface=%#v people=%#v", month, surface, people)
		}
	}
	if _, err := epathSQLShare(epathSQLQuantity{Value: 1.5}, epathSQLQuantity{}, epathSQLQuantity{}, model.Precision); err == nil {
		t.Fatal("generic positive-load/zero-denominator guard was weakened")
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelLoadDriverChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelDriverLinkChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelThermalReconciliationChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	seenValues, seenSources, seenHeat := 0, 0, 0
	for _, check := range checks.Rows {
		if check.Item.Period != "annual" && check.Item.Period != "M8" {
			continue
		}
		if check.Item.Group == "drivers" && check.Item.Target.Category == "balance.storage_other" && check.Item.Target.Service == "heating" {
			want := 0.0
			if check.Item.Target.Field == "value" {
				want = 1.5
			}
			if check.Quantity == nil || want == 0 && !epathSQLExactZero(*check.Quantity) || want != 0 && !loadEqual(check.Quantity.Value) {
				t.Fatalf("independent raw/effective/allocation proof differs: %s", check.Want.Key)
			}
			seenValues++
		}
		if p := check.DriverLink; p != nil && p.Service == "heating" && p.Category == "balance.storage_other" {
			if !p.PureZeroPressureFallback || !loadEqual(check.Quantity.Value) || !reflect.DeepEqual(p.RequiredDriverSources, []int{11}) || !reflect.DeepEqual(p.LoadSources, []int{11}) {
				t.Fatal("fallback monthly/annual trace borrowed a physical residual source")
			}
			seenSources++
		}
		if p := check.Reconciliation; p != nil && p.Level == "heat" && p.Service == "heating" {
			if !loadEqual(p.Expected.Value) || !loadEqual(p.Explained.Value) || p.Residual.Value != 0 {
				t.Fatal("fallback became HVAC unassigned or a physical residual")
			}
			seenHeat++
		}
	}
	if seenValues != 12 || seenSources != 8 || seenHeat != 18 {
		t.Fatalf("missing Building/Zone/month/annual obligations: values=%d sources=%d heat=%d", seenValues, seenSources, seenHeat)
	}
}

func TestEnergyPathRealSQLZeroPressureRejectsDeclarationsAndUnknownSQL(t *testing.T) {
	for _, test := range []struct {
		name, query string
		edit        func(*epathRealSQLModel)
	}{
		{name: "undeclared", edit: func(m *epathRealSQLModel) { m.ZeroPressureFallbacks = nil }},
		{name: "duplicate case-insensitive owner", edit: func(m *epathRealSQLModel) {
			d := m.ZeroPressureFallbacks[0]
			d.ZoneName = "OFFICE"
			m.ZeroPressureFallbacks = append(m.ZeroPressureFallbacks, d)
		}},
		{name: "wrong Zone", edit: func(m *epathRealSQLModel) { m.ZeroPressureFallbacks[0].ZoneName = "Lab" }},
		{name: "whitespace owner", edit: func(m *epathRealSQLModel) { m.ZeroPressureFallbacks[0].ZoneName = " Office" }},
		{name: "wrong month", edit: func(m *epathRealSQLModel) { m.ZeroPressureFallbacks[0].Month = 7 }},
		{name: "month out of range", edit: func(m *epathRealSQLModel) { m.ZeroPressureFallbacks[0].Month = 13 }},
		{name: "wrong service", edit: func(m *epathRealSQLModel) { m.ZeroPressureFallbacks[0].Service = "Cooling" }},
		{name: "unnecessary positive cooling pressure", edit: func(m *epathRealSQLModel) { m.ZeroPressureFallbacks[0].Service = "cooling" }},
		{name: "reserved physical family", edit: func(m *epathRealSQLModel) { m.Families[0].ID = epathSQLZeroPressureFamilyPrefix + "heating" }},
		{name: "zero load", query: `UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=11 AND TimeIndex=8`},
		{name: "missing load", query: `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=11 AND TimeIndex=8`},
		{name: "NULL load", query: `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=11 AND TimeIndex=8`},
		{name: "missing physical source", query: `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=14 AND TimeIndex=8`},
		{name: "NULL physical source", query: `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=14 AND TimeIndex=8`},
		{name: "tiny actual heating pressure", query: `UPDATE ReportData SET Value=-0.000001 WHERE ReportDataDictionaryIndex=14 AND TimeIndex=8`},
		{name: "duplicate physical observation", query: `INSERT INTO ReportData VALUES(999991,14,8,3600000)`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, model := epathSQLZeroPressureUnitFixture(t)
			if test.query != "" {
				epathOracleEditSQL(t, path, test.query)
			}
			if test.edit != nil {
				test.edit(&model)
			}
			observed, err := epathReadRealSQLOracle(path)
			if err == nil {
				_, err = epathCompileSQLModelFrames(path, observed.Sources, model)
			}
			if err == nil {
				t.Fatal("invalid declaration/observation healed a zero denominator")
			}
		})
	}
}

func TestEnergyPathRealSQLZeroPressureExactZeroBothServiceDeclarations(t *testing.T) {
	path, model := epathSQLZeroPressureUnitFixture(t)
	epathOracleEditSQL(t, path, `UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex IN(12,13,14) AND TimeIndex=8`)
	model.ZeroPressureFallbacks = append(model.ZeroPressureFallbacks, epathRealSQLZeroPressureFallback{ZoneName: "OFFICE", Month: 8, Service: "cooling"})
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames.ZeroPressurePhysicalCells) != 2 || len(frames.ZeroPressureFallbacks) != 2 || len(frames.Cells) != 26 {
		t.Fatal("two independently declared services duplicated the physical roster")
	}
	for _, declaration := range model.ZeroPressureFallbacks {
		cell := frames.Cells[epathSQLZeroPressureCellKey(declaration)]
		load := frames.Loads[epathSQLKey("Office", declaration.Service, 8)]
		if !epathSQLExactZero(cell.Raw) || !epathSQLExactZero(cell.Effective) || !reflect.DeepEqual(cell.Allocated[declaration.Service], load) {
			t.Fatal("observed all-zero pressure was not allocated independently per service")
		}
	}
}

func TestEnergyPathRealSQLZeroPressureRejectsSyntheticAndRosterSpoofing(t *testing.T) {
	original, model := epathSQLZeroPressureUnitFrames(t)
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	fallbackKey := epathSQLZeroPressureCellKey(model.ZeroPressureFallbacks[0])
	physicalKey := epathSQLKey("Office", "surface:surface.exterior_walls", 8)
	for name, edit := range map[string]func(*epathSQLFrames){
		"missing physical roster":    func(f *epathSQLFrames) { f.ZeroPressurePhysicalCells = nil },
		"missing physical cell":      func(f *epathSQLFrames) { delete(f.Cells, physicalKey) },
		"physical marker masquerade": func(f *epathSQLFrames) { f.Cells[physicalKey].ZeroPressureFallback = true },
		"extra physical cell": func(f *epathSQLFrames) {
			c := *f.Cells[physicalKey]
			c.Family = "surface:surface.roofs"
			f.Cells[epathSQLKey(c.Zone, c.Family, 8)] = &c
		},
		"physical sign changed":        func(f *epathSQLFrames) { f.Cells[physicalKey].Raw.Value = -0.00000001 },
		"physical allocation retained": func(f *epathSQLFrames) { f.Cells[physicalKey].Allocated["heating"] = epathSQLQuantity{Value: .001} },
		"original source NULL": func(f *epathSQLFrames) {
			s := f.SourceIdentities[14]
			s.Months[7].EnergyKWh = nil
			f.SourceIdentities[14] = s
		},
		"original source nonfinite": func(f *epathSQLFrames) { f.SourceRaw[14][7].Value = math.NaN() },
		"source other Zone":         func(f *epathSQLFrames) { f.SourceZone[14] = "Lab" },
		"load other frequency": func(f *epathSQLFrames) {
			s := f.SourceIdentities[11]
			s.ReportingFrequency = "Hourly"
			f.SourceIdentities[11] = s
		},
		"load wrong kind": func(f *epathSQLFrames) {
			s := f.SourceIdentities[11]
			s.Name = "People Gain"
			f.SourceIdentities[11] = s
		},
		"load another owner":                             func(f *epathSQLFrames) { s := f.SourceIdentities[11]; s.KeyValue = "Lab"; f.SourceIdentities[11] = s },
		"raw zero contaminated":                          func(f *epathSQLFrames) { f.Cells[fallbackKey].Raw = epathSQLQuantity{Value: .001} },
		"effective zero uncertain":                       func(f *epathSQLFrames) { f.Cells[fallbackKey].Effective = epathSQLQuantity{Error: .001} },
		"synthetic pressure source":                      func(f *epathSQLFrames) { f.Cells[fallbackKey].SourceIDs = []int{14} },
		"internal derived leaves cannot substitute load": func(f *epathSQLFrames) { f.Cells[fallbackKey].SourceIDs = []int{15, 27, 193, 341} },
		"duplicate synthetic source":                     func(f *epathSQLFrames) { f.Cells[fallbackKey].SourceIDs = []int{11, 11} },
		"synthetic missing own source":                   func(f *epathSQLFrames) { f.Cells[fallbackKey].SourceIDs = nil },
		"synthetic load copied to wrong month":           func(f *epathSQLFrames) { f.Cells[fallbackKey].Month = 9 },
		"synthetic allocation changed":                   func(f *epathSQLFrames) { f.Cells[fallbackKey].Allocated["heating"] = epathSQLQuantity{Value: 1.501} },
		"duplicate synthetic cell":                       func(f *epathSQLFrames) { c := *f.Cells[fallbackKey]; f.Cells[fallbackKey+"other"] = &c },
		"undeclared synthetic cell":                      func(f *epathSQLFrames) { f.ZeroPressureFallbacks = nil },
	} {
		t.Run(name, func(t *testing.T) {
			var frames epathSQLFrames
			if err := json.Unmarshal(data, &frames); err != nil {
				t.Fatal(err)
			}
			edit(&frames)
			if epathSQLValidateZeroPressureFrames(frames, model) == nil {
				t.Fatal("forged fallback frame accepted")
			}
			var checks epathSQLModelChecks
			if epathSQLModelDriverLinkChecks(frames, model, &checks) == nil || epathSQLModelThermalReconciliationChecks(frames, model, &checks) == nil {
				t.Fatal("downstream compiler bypassed exact original roster")
			}
		})
	}
}

func epathSQLZeroPressureUnitBundle(t *testing.T) PurposeResultBundle {
	t.Helper()
	_, _, _, bundle := epathSQLDriverLinksFixture(t)
	add := func(nodes *[]EnergyExplanationNode, links *[]EnergyPathLink, period, zone string) {
		if period != "M8" && period != "annual" {
			return
		}
		*nodes = append(*nodes,
			EnergyExplanationNode{ID: "fallback", Level: "driver", Value: 1.5, AllocatedValue: 1.5, AllocationApplied: true, DriverCategory: "balance.storage_other", ThermalComponent: "combined", ServiceKind: "heating", Unit: "kWh", ScaleDomain: "thermal", Basis: "heat_balance_share", Period: period, ZoneName: zone, SourceIDs: []string{"heating-source"}},
			EnergyExplanationNode{ID: "heating", Level: "load", Value: 1.5, ServiceKind: "heating", Unit: "kWh", ScaleDomain: "thermal", Basis: "reported_variable", Period: period, ZoneName: zone, SourceIDs: []string{"heating-source"}})
		*links = append(*links, EnergyPathLink{ID: "fallback-to-heating", FromID: "fallback", ToID: "heating", Relation: "driver_to_load", ServiceKind: "heating", FromValue: 1.5, ToValue: 1.5, FromUnit: "kWh", ToUnit: "kWh", Basis: "heat_balance_share", Period: period, ZoneName: "Office", SourceIDs: []string{"heating-source"}})
	}
	r := &bundle.EnergyExplanation
	add(&r.Nodes, &r.Links, "annual", "")
	for i := range r.Periods {
		p := &r.Periods[i]
		add(&p.Nodes, &p.Links, p.ID, "")
	}
	for i := range r.ZoneResults {
		z := &r.ZoneResults[i]
		add(&z.Nodes, &z.Links, "annual", z.Scope.ZoneName)
		for j := range z.Periods {
			p := &z.Periods[j]
			add(&p.Nodes, &p.Links, p.ID, z.Scope.ZoneName)
		}
	}
	return bundle
}

func TestEnergyPathRealSQLZeroPressureCandidateLinksAndQuality(t *testing.T) {
	frames, model := epathSQLZeroPressureUnitFrames(t)
	var checks epathSQLModelChecks
	if err := epathSQLModelLoadDriverChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelDriverLinkChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	bundle := epathSQLZeroPressureUnitBundle(t)
	var selected epathSQLModelCheck
	for _, check := range checks.Rows {
		if check.DriverLink != nil {
			if err := epathCheckSQLModelDriverLink(bundle, check); err != nil {
				t.Fatalf("%s: %v", check.Want.Key, err)
			}
			if check.Item.Scope == "building" && check.Item.Period == "M8" && check.DriverLink.Category == "balance.storage_other" && check.DriverLink.Service == "heating" {
				selected = check
			}
		}
		if check.Item.Group == "drivers" && check.Item.Target.Category == "balance.storage_other" && check.Item.Target.Service == "heating" {
			actual, err := epathReadOracleCandidate(bundle, check.Item, check.Want)
			if err != nil || epathCheckSQLModelPresentation(bundle, check, actual) != nil {
				t.Fatalf("raw-zero/value candidate proof %s: %v", check.Want.Key, err)
			}
		}
	}
	if selected.DriverLink == nil {
		t.Fatal("missing exact fallback branch proof")
	}
	for name, edit := range map[string]func(*EnergyExplanationNode){
		"tiny raw":                 func(n *EnergyExplanationNode) { n.RawValue = 1e-9 },
		"tiny effective":           func(n *EnergyExplanationNode) { n.EffectiveValue = 1e-9 },
		"tiny signed":              func(n *EnergyExplanationNode) { n.SignedValue = -1e-9 },
		"missing raw":              func(n *EnergyExplanationNode) { n.inspectorDecodedFromJSON = true; n.inspectorValuePresence = 2 },
		"missing effective":        func(n *EnergyExplanationNode) { n.inspectorDecodedFromJSON = true; n.inspectorValuePresence = 1 },
		"wrong fallback component": func(n *EnergyExplanationNode) { n.ThermalComponent = "sensible" },
	} {
		t.Run(name, func(t *testing.T) {
			b := epathSQLZeroPressureUnitBundle(t)
			p := &b.EnergyExplanation.Periods[7]
			edit(&p.Nodes[len(p.Nodes)-2])
			if epathCheckSQLModelDriverLink(b, selected) == nil {
				t.Fatal("pure fallback lost strict zero/presence identity")
			}
		})
	}
	for _, period := range []string{"annual", "M8"} {
		for _, scope := range []string{"building", "zone"} {
			zone := ""
			if scope == "zone" {
				zone = "Office"
			}
			deps := epathSQLQualityAccountingDependencies(checks.Rows, "driverToLoadClosedPct", scope, zone, period)
			check := epathSQLModelCheck{Item: epathRealOracleMetricRecipe{Scope: scope, Zone: zone, Period: period}, Quality: &epathSQLQualityProof{Field: "driverToLoadClosedPct", Dependencies: deps}}
			nodes, links, _, _, err := epathOracleGraph(bundle, scope, zone, period)
			if err != nil {
				t.Fatal(err)
			}
			value, _, err := epathSQLQualityClosure(bundle, check, nodes, links)
			if err != nil || value == nil || *value != 100 {
				t.Fatalf("fallback not independently closed in %s/%s: %v %v", scope, period, value, err)
			}
		}
	}
	for _, sourceIDs := range [][]string{{"people-source"}, {"heating-source", "people-source"}, {"derived-internal"}, {}, {"cooling-source"}} {
		b := epathSQLZeroPressureUnitBundle(t)
		b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, EnergyDataSource{ID: "derived-internal", SourceType: "derived_formula", InputSourceIDs: []string{"people-source"}})
		p := &b.EnergyExplanation.Periods[7]
		p.Nodes[len(p.Nodes)-2].SourceIDs = sourceIDs
		p.Links[len(p.Links)-1].SourceIDs = append([]string{"heating-source"}, sourceIDs...)
		if epathCheckSQLModelDriverLink(b, selected) == nil {
			t.Fatalf("non-load fallback provenance accepted: %v", sourceIDs)
		}
	}
	// Presence is not inferred from zero. Original-wire decoder distinguishes
	// both a missing and an explicit null driver scalar from known numeric 0.
	for _, field := range []string{"rawValue", "effectiveValue"} {
		for _, raw := range []string{"", fmt.Sprintf(",%q:null", field)} {
			text := fmt.Sprintf(`{"energyExplanation":{"schema":"%s","scope":{"kind":"building"},"nodes":[{"id":"fallback","level":"driver","value":1.5,"unit":"kWh","scaleDomain":"thermal","driverCategory":"balance.storage_other","serviceKind":"heating","basis":"heat_balance_share"%s}]}}`, energyExplanationSchema, raw)
			b, err := epathDecodeOriginalOracleCandidate(strings.NewReader(text))
			if err != nil {
				t.Fatal(err)
			}
			for _, check := range checks.Rows {
				if check.Item.Scope == "building" && check.Item.Period == "annual" && check.Item.Group == "drivers" && check.Item.Target.Category == "balance.storage_other" && check.Item.Target.Service == "heating" && check.Item.Target.Field == field {
					value, err := epathReadOracleCandidate(b, check.Item, check.Want)
					if err == nil && epathCheckSQLModelPresentation(b, check, value) == nil {
						t.Fatal("missing/null fallback scalar became observed zero")
					}
				}
			}
		}
	}
}

func TestEnergyPathRealSQLZeroPressurePureFlagKeepsPhysicalContext(t *testing.T) {
	for _, sharedBuilding := range []bool{false, true} {
		t.Run(fmt.Sprintf("otherZone=%v", sharedBuilding), func(t *testing.T) {
			path, model := epathSQLZeroPressureUnitFixture(t)
			// A real physical storage contribution in another month or owner
			// must not inherit the exception's exact-zero scalar requirement.
			model.Families[0].Category = "balance.storage_other"
			if !sharedBuilding {
				epathOracleEditSQL(t, path, `UPDATE ReportData SET Value=-3600000 WHERE ReportDataDictionaryIndex=14 AND TimeIndex=7;
UPDATE ReportData SET Value=7200000 WHERE ReportDataDictionaryIndex=11 AND TimeIndex=7`)
			} else {
				epathOracleEditSQL(t, path, `INSERT INTO Zones VALUES(2,'Lab',1,1);
INSERT INTO Surfaces VALUES(3,'Lab Wall','Wall',2,0,1);
INSERT INTO ReportDataDictionary VALUES(50,'Sensible Cooling','Lab',0,'Monthly','J','Zone'),(51,'Sensible Heating','Lab',0,'Monthly','J','Zone'),(52,'Surface Exchange','Lab Wall',0,'Monthly','J','Surface'),(53,'People Gain','Lab',0,'Monthly','J','Zone');
INSERT INTO ReportData SELECT 40000+d.ReportDataDictionaryIndex*20+t.TimeIndex,d.ReportDataDictionaryIndex,t.TimeIndex,CASE d.ReportDataDictionaryIndex WHEN 51 THEN 3600000 WHEN 53 THEN -3600000 ELSE 0 END FROM ReportDataDictionary d CROSS JOIN "Time" t WHERE d.ReportDataDictionaryIndex IN(50,51,52,53) AND t.TimeIndex BETWEEN 1 AND 12`)
				model.Surface.Source.Keys = append(model.Surface.Source.Keys, "Lab Wall")
				model.Families[0].Keys = append(model.Families[0].Keys, "Lab")
				model.Families[0].Terms[0].Source.Keys = append(model.Families[0].Terms[0].Source.Keys, "Lab")
				for i := range model.Loads {
					model.Loads[i].Source.Keys = append(model.Loads[i].Source.Keys, "Lab")
				}
			}
			observed, err := epathReadRealSQLOracle(path)
			if err != nil {
				t.Fatal(err)
			}
			frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
			if err != nil {
				t.Fatal(err)
			}
			var checks epathSQLModelChecks
			if err := epathSQLModelDriverLinkChecks(frames, model, &checks); err != nil {
				t.Fatal(err)
			}
			seen := 0
			for _, check := range checks.Rows {
				p := check.DriverLink
				if p.Service != "heating" || p.Category != "balance.storage_other" || check.Item.Period != "annual" && check.Item.Period != "M8" {
					continue
				}
				want := check.Item.Period == "M8"
				if sharedBuilding {
					want = check.Item.Scope == "zone" && check.Item.Zone == "Office"
				}
				if p.PureZeroPressureFallback != want {
					t.Fatalf("exception escaped exact contributor context %s: pure=%v want=%v", check.Want.Key, p.PureZeroPressureFallback, want)
				}
				if !want {
					node := EnergyExplanationNode{RawValue: 1, EffectiveValue: 6, SignedValue: -6}
					if err := epathSQLCheckZeroPressureDriver(node, p); err != nil {
						t.Fatal("mixed context was forced to zero")
					}
				}
				seen++
			}
			wantCount := 8
			if sharedBuilding {
				wantCount = 12
			}
			if seen != wantCount {
				t.Fatalf("lost mixed scope/annual obligations: %d", seen)
			}
		})
	}
}
