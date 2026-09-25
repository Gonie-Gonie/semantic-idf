package simulation

import (
	"encoding/json"
	"strings"
	"testing"
)

// These integers are hand arithmetic, not actual capture/candidate quantities.
func epathSQLSimpleVentilationOutdoorUnit() (epathSQLFrames, epathRealSQLModel) {
	zones := []string{"ZONE 1", "ZONE 2", "ZONE 3"}
	model := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2},
		DirectHVACComponents: []epathRealSQLDirectHVACComponent{epathSQLSimpleVentilationUnitDeclaration()}}
	f := epathSQLFrames{Zones: map[string]epathSQLZone{}, Cells: map[string]*epathSQLCell{}, SourceIdentities: map[int]epathRealSQLSource{}, SourceRaw: map[int][]epathSQLQuantity{}, SourceZone: map[int]string{}}
	for _, spec := range []struct {
		id, name, component, unit string
		sign                      float64
	}{
		{"ventilation.sensible.gain", "Zone Ventilation Sensible Heat Gain Energy", "sensible", "J", 1},
		{"ventilation.sensible.loss", "Zone Ventilation Sensible Heat Loss Energy", "sensible", "J", -1},
		{"ventilation.latent.gain", "Zone Ventilation Latent Heat Gain Energy", "latent", "J", 1},
		{"ventilation.latent.loss", "Zone Ventilation Latent Heat Loss Energy", "latent", "J", -1},
		{"outdoor.balance", "Zone Air Heat Balance Outdoor Air Transfer Rate", "sensible", "W", 1},
	} {
		family := epathRealSQLFamily{ID: spec.id, Keys: append([]string(nil), zones...), Category: "air.mechanical_ventilation", Component: spec.component, Role: "pressure", BuildingVisible: true,
			Terms: []epathRealSQLTerm{{Sign: spec.sign, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: spec.name, Unit: spec.unit}}}}}}
		if spec.id == "outdoor.balance" {
			family.Category = "balance.storage_other"
			family.Subtract = []string{"ventilation.sensible.gain", "ventilation.sensible.loss"}
		}
		model.Families = append(model.Families, family)
		model.HourlyCompanions = append(model.HourlyCompanions, epathRealSQLHourlyCompanion{Name: spec.name, Unit: spec.unit, Keys: append([]string(nil), zones...)})
		for _, zone := range zones {
			z := strings.ToLower(zone)
			f.Zones[z] = epathSQLZone{Name: zone, Multiplier: 1}
			id := len(f.SourceIdentities) + 1
			s := epathRealSQLSource{DictionaryIndex: id, Name: spec.name, KeyValue: zone, SourceUnit: spec.unit, ReportingFrequency: "Monthly"}
			f.SourceZone[id] = z
			for month := 1; month <= 12; month++ {
				value := map[string]float64{"ventilation.sensible.gain": 20, "ventilation.sensible.loss": 15, "ventilation.latent.gain": 100, "ventilation.latent.loss": 40, "outdoor.balance": 8}[spec.id]
				if month > 6 {
					value = map[string]float64{"ventilation.sensible.gain": 2, "ventilation.sensible.loss": 9, "ventilation.latent.gain": 70, "ventilation.latent.loss": 10, "outdoor.balance": -10}[spec.id]
				}
				s.Months = append(s.Months, epathRealSQLMonth{Month: month, Rows: 1, EnergyKWh: epathOracleNumber(value)})
				f.SourceRaw[id] = append(f.SourceRaw[id], epathSQLQuantity{Value: value})
				cellValue := value * spec.sign
				if spec.id == "outdoor.balance" {
					cellValue = 3
					if month > 6 {
						cellValue = -3
					}
				}
				f.Cells[epathSQLKey(z, spec.id, month)] = &epathSQLCell{Zone: z, Family: spec.id, Category: family.Category, Component: spec.component, Month: month, Raw: epathSQLQuantity{Value: cellValue}, Effective: epathSQLQuantity{Value: cellValue}, BuildingVisible: true, SourceIDs: []int{id}}
			}
			f.SourceIdentities[id] = s
		}
	}
	return f, model
}

func TestEnergyPathSQLSimpleVentilationOutdoorSignedSensibleOnlyLiteral(t *testing.T) {
	f, model := epathSQLSimpleVentilationOutdoorUnit()
	before, _ := json.Marshal(f)
	var checks epathSQLModelChecks
	if err := epathSQLDriverReconciliationChecks(f, model, []string{"zone 1", "zone 2", "zone 3"}, []string{"M1", "M7", "annual"}, &checks); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, check := range checks.Rows {
		p := check.Reconciliation
		if p == nil || p.Level != "driver" || !strings.HasPrefix(p.ID, "reconcile.driver.outdoor_air.") {
			t.Fatal("lost finite outdoor reconciliation")
		}
		want := map[string][3]float64{"M1": {8, 5, 3}, "M7": {-10, -7, -3}, "annual": {-12, -12, 0}}[p.Period]
		if p.Expected.Value != want[0] || p.Explained.Value != want[1] || p.Residual.Value != want[2] {
			t.Fatalf("latent detail or aggregate duplicated: %s %#v", p.Period, p)
		}
		if check.Item.Target.Field == "expectedValue" && check.Item.Scope == "building" {
			seen[p.ZoneName+"|"+p.Period] = true
		}
	}
	if len(seen) != 9 {
		t.Fatalf("missing Zone/month-first literal obligations: %v", seen)
	}
	after, _ := json.Marshal(f)
	if string(before) != string(after) {
		t.Fatal("equation compiler mutated native frames")
	}
	// Latent detail can change without entering sensible closure.
	for _, cell := range f.Cells {
		if cell.Component == "latent" {
			cell.Effective.Value *= 1000
			cell.Raw.Value *= 1000
		}
	}
	var changed epathSQLModelChecks
	if err := epathSQLDriverReconciliationChecks(f, model, []string{"zone 1", "zone 2", "zone 3"}, []string{"M1", "M7", "annual"}, &changed); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(checks)
	b, _ := json.Marshal(changed)
	if string(a) != string(b) {
		t.Fatal("latent magnitudes entered sensible reconciliation")
	}
}

func TestEnergyPathSQLSimpleVentilationOutdoorRejectsUnreviewedDeclarations(t *testing.T) {
	for name, edit := range map[string]func(*epathRealSQLModel){
		"missing detail":                 func(m *epathRealSQLModel) { m.Families = m.Families[1:] },
		"missing aggregate":              func(m *epathRealSQLModel) { m.Families = m.Families[:4] },
		"latent in sensible subtraction": func(m *epathRealSQLModel) { m.Families[4].Subtract[1] = "ventilation.latent.loss" },
		"loss sign":                      func(m *epathRealSQLModel) { m.Families[1].Terms[0].Sign = 1 },
		"gain sign":                      func(m *epathRealSQLModel) { m.Families[0].Terms[0].Sign = -1 },
		"rate substituted": func(m *epathRealSQLModel) {
			m.Families[0].Terms[0].Source.Alternatives[0] = epathRealSQLAlternative{Name: "Zone Ventilation Sensible Heat Gain Rate", Unit: "W"}
		},
		"fan substituted": func(m *epathRealSQLModel) {
			m.Families[0].Terms[0].Source.Alternatives[0].Name = epathSQLSimpleVentilationFanName
		},
		"wrong unit":                 func(m *epathRealSQLModel) { m.Families[4].Terms[0].Source.Alternatives[0].Unit = "J" },
		"missing Zone":               func(m *epathRealSQLModel) { m.Families[2].Keys = m.Families[2].Keys[:2] },
		"duplicate Zone":             func(m *epathRealSQLModel) { m.Families[2].Keys[2] = "ZONE 1" },
		"foreign term key":           func(m *epathRealSQLModel) { m.Families[2].Terms[0].Source.Keys = []string{"ZONE 2"} },
		"missing owner cohort":       func(m *epathRealSQLModel) { m.DirectHVACComponents = nil },
		"context hides detail":       func(m *epathRealSQLModel) { m.Families[2].Role = "context" },
		"wrong component":            func(m *epathRealSQLModel) { m.Families[2].Component = "sensible" },
		"hidden duplicate aggregate": func(m *epathRealSQLModel) { x := m.Families[4]; x.ID = "hidden"; m.Families = append(m.Families, x) },
		"fallback coexists":          func(m *epathRealSQLModel) { m.Families = append(m.Families, epathRealSQLFamily{ID: "outdoor.unsplit"}) },
		"meter substitution":         func(m *epathRealSQLModel) { m.Families[0].Terms[0].Source.IsMeter = true },
		"missing becomes zero":       func(m *epathRealSQLModel) { m.Families[0].Terms[0].Source.AllowAbsent = true },
	} {
		t.Run(name, func(t *testing.T) {
			_, m := epathSQLSimpleVentilationOutdoorUnit()
			edit(&m)
			if active, err := epathSQLSimpleVentilationOutdoorDeclaration(m); !active || err == nil {
				t.Fatalf("invalid finite equation accepted: %v/%v", active, err)
			}
		})
	}
	if active, err := epathSQLSimpleVentilationOutdoorDeclaration(epathRealSQLModel{}); active || err != nil {
		t.Fatal("ordinary model behavior changed")
	}
}

func TestEnergyPathSQLSimpleVentilationOutdoorRejectsMissingNativeDetailFrames(t *testing.T) {
	for _, remove := range []string{"ventilation.sensible.gain", "ventilation.sensible.loss"} {
		f, m := epathSQLSimpleVentilationOutdoorUnit()
		delete(f.Cells, epathSQLKey("zone 2", remove, 3))
		if err := epathSQLDriverReconciliationChecks(f, m, []string{"zone 1", "zone 2", "zone 3"}, []string{"M3"}, &epathSQLModelChecks{}); err == nil {
			t.Fatal("missing native detail became zero")
		}
	}
	f, m := epathSQLSimpleVentilationOutdoorUnit()
	for id, s := range f.SourceIdentities {
		if s.Name == "Zone Air Heat Balance Outdoor Air Transfer Rate" && s.KeyValue == "ZONE 2" {
			f.SourceZone[id] = "zone 1"
		}
	}
	if err := epathSQLDriverReconciliationChecks(f, m, []string{"zone 1", "zone 2", "zone 3"}, []string{"M3"}, &epathSQLModelChecks{}); err == nil {
		t.Fatal("foreign aggregate owner accepted")
	}
}
