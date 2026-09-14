package simulation

// Independent acceptance support. Requires future model.PoolSystems and frames.PoolSystems /
// PoolAirLoopFans fields when installed with the documented call-site patch.

import (
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLPoolFanDeclaration() epathRealSQLAirLoopFan {
	d := epathSQLPoolOriginalDeclaration()
	return epathRealSQLAirLoopFan{SiteID: "fans.electricity", ObjectType: "Fan:VariableVolume", ObjectName: d.FanName, AirLoopName: d.AirLoopName, ServedZones: append([]string(nil), d.ServedZones...)}
}

func epathSQLPoolFanOriginalAndSources(t *testing.T) (string, epathRealSQLPoolSystem, epathSQLPoolSourceFrames) {
	t.Helper()
	original := epathSQLPoolOriginalFixture(t)
	d := epathSQLPoolOriginalDeclaration()
	proofs, err := epathSQLValidatePoolOriginal(original, []epathRealSQLPoolSystem{d})
	if err != nil || len(proofs) != 1 {
		t.Fatalf("original proof: %v", err)
	}
	return original, d, epathSQLPoolSourceHandFramesForOriginal(t, proofs[0])
}

func TestEnergyPathSQLPoolFanRequiresOriginalAndNativeFrames(t *testing.T) {
	original, d, source := epathSQLPoolFanOriginalAndSources(t)
	fan := epathSQLPoolFanDeclaration()
	bindings, err := epathSQLBindPoolAirLoopFans(original, []epathRealSQLPoolSystem{d}, []epathSQLPoolSourceFrames{source}, []epathRealSQLAirLoopFan{fan})
	if err != nil || len(bindings) != 1 {
		t.Fatalf("native Pool fan binding: %v", err)
	}
	bound := bindings[fan.SiteID]
	if !reflect.DeepEqual(bound.Original, source.Original) || !reflect.DeepEqual(bound.Fan, fan) {
		t.Fatal("Pool fan stamp changed original proof or declaration")
	}
	if err = epathSQLValidatePoolAirLoopFanBinding(bound, []epathRealSQLPoolSystem{d}, []epathSQLPoolSourceFrames{source}, fan); err != nil {
		t.Fatal(err)
	}
	// The previous mixer-only path must still reject this OA coil train when
	// explicitly invoked: Pool is not accepted by broadening that validator.
	doc, err := idf.Parse(original)
	if err != nil {
		t.Fatal(err)
	}
	if err = epathSQLAirLoopFanOriginal(doc, fan); err == nil {
		t.Fatal("legacy mixer-only validator was broadened for Pool")
	}
}

func TestEnergyPathSQLPoolFanRejectsUnboundOrAlteredOriginal(t *testing.T) {
	for _, mutation := range []string{"missing source frames", "duplicate source frames", "missing fan", "duplicate fan", "source hash", "source declaration", "source owner index", "source incomplete", "wrong fan name", "wrong fan type", "wrong air loop", "wrong site", "missing recipient", "plenum recipient", "duplicate recipient", "changed original outside path"} {
		t.Run(mutation, func(t *testing.T) {
			original, d, source := epathSQLPoolFanOriginalAndSources(t)
			fan := epathSQLPoolFanDeclaration()
			sources := []epathSQLPoolSourceFrames{source}
			fans := []epathRealSQLAirLoopFan{fan}
			switch mutation {
			case "missing source frames":
				sources = nil
			case "duplicate source frames":
				sources = append(sources, source)
			case "missing fan":
				fans = nil
			case "duplicate fan":
				fans = append(fans, fan)
			case "source hash":
				sources[0].Original.OriginalSHA256 = strings.Repeat("a", 64)
			case "source declaration":
				sources[0].Original.Declaration.AirLoopName = "Another Air Loop"
			case "source owner index":
				sources[0].Original.Owners[5].ObjectIndex++
			case "source incomplete":
				delete(sources[0].Sources, sources[0].Families["pump.cw.electricity"].CanonicalID)
			case "wrong fan name":
				fans[0].ObjectName = "Another Fan"
			case "wrong fan type":
				fans[0].ObjectType = "Fan:OnOff"
			case "wrong air loop":
				fans[0].AirLoopName = "Another Air Loop"
			case "wrong site":
				fans[0].SiteID = "pumps.electricity"
			case "missing recipient":
				fans[0].ServedZones = fans[0].ServedZones[:4]
			case "plenum recipient":
				fans[0].ServedZones[4] = "PLENUM-1"
			case "duplicate recipient":
				fans[0].ServedZones[4] = fans[0].ServedZones[0]
			case "changed original outside path":
				original = epathSQLPoolMutatedOriginal(t, original, func(doc *idf.Document) {
					epathSQLPoolUnitObject(t, doc, "Coil:Cooling:Water", "OA Cooling Coil 1").Fields[11].Value = "Broken outside path"
				})
			}
			bindings, err := epathSQLBindPoolAirLoopFans(original, []epathRealSQLPoolSystem{d}, sources, fans)
			if err == nil || bindings != nil {
				t.Fatal("invalid Pool evidence published a fan binding")
			}
		})
	}
}

func TestEnergyPathSQLPoolFanBindingStampDoesNotAliasCurrentSources(t *testing.T) {
	for _, mutation := range []string{"hash", "owners", "demands", "served roster", "declaration roster", "fan roster", "native source rows"} {
		t.Run(mutation, func(t *testing.T) {
			original, d, source := epathSQLPoolFanOriginalAndSources(t)
			fan := epathSQLPoolFanDeclaration()
			bindings, err := epathSQLBindPoolAirLoopFans(original, []epathRealSQLPoolSystem{d}, []epathSQLPoolSourceFrames{source}, []epathRealSQLAirLoopFan{fan})
			if err != nil {
				t.Fatal(err)
			}
			bound := bindings[fan.SiteID]
			before := bound.Original.OriginalSHA256
			switch mutation {
			case "hash":
				source.Original.OriginalSHA256 = strings.Repeat("b", 64)
			case "owners":
				source.Original.Owners[5].ObjectIndex++
			case "demands":
				source.Original.ChilledWaterDemands[0].ObjectName = "Another demand"
			case "served roster":
				source.Original.ServedZones[0] = "PLENUM-1"
			case "declaration roster":
				source.Original.Declaration.Terminals[0].ADUName = "Another ADU"
			case "fan roster":
				fan.ServedZones[0] = "PLENUM-1"
			case "native source rows":
				id := source.Families["pump.cw.electricity"].CanonicalID
				s := source.Sources[id]
				s.RequestBound = false
				source.Sources[id] = s
			}
			if err = epathSQLValidatePoolAirLoopFanBinding(bound, []epathRealSQLPoolSystem{d}, []epathSQLPoolSourceFrames{source}, fan); err == nil {
				t.Fatal("changed original or native source stamp accepted")
			}
			if bound.Original.OriginalSHA256 != before || bound.Fan.ServedZones[0] != "SPACE1-1" || bound.Original.ServedZones[0] != "SPACE1-1" || bound.Original.Declaration.Terminals[0].ADUName != "SPACE1-1 ATU" {
				t.Fatal("caller mutation rewrote detached original stamp")
			}
		})
	}
}

func TestEnergyPathSQLPoolFanLegacyAndForeignOwnerBoundaries(t *testing.T) {
	bindings, err := epathSQLBindPoolAirLoopFans("not an IDF", nil, nil, nil)
	if err != nil || bindings != nil {
		t.Fatal("absent Pool opt-in changed legacy behavior")
	}
	if _, err = epathSQLBindPoolAirLoopFans("", nil, []epathSQLPoolSourceFrames{{}}, nil); err == nil {
		t.Fatal("stale Pool source frame gained legacy fallback")
	}
	raw, err := os.ReadFile("testdata/energy_path_real_models/models/25.1/5ZoneElectricBaseboard.idf")
	if err != nil {
		t.Fatal(err)
	}
	fan := epathSQLPoolFanDeclaration()
	model := epathRealSQLModel{AirLoopFans: []epathRealSQLAirLoopFan{fan}}
	frames := epathSQLFrames{}
	if err = epathSQLBindAirLoopFans(string(raw), model, &frames); err != nil {
		t.Fatal("legacy Mixed fan binding changed", err)
	}
	if !reflect.DeepEqual(frames.AirLoopFans[fan.SiteID], fan) {
		t.Fatal("legacy fan declaration changed")
	}
	pool := epathSQLPoolOriginalFixture(t)
	foreign := epathSQLPoolMutatedOriginal(t, pool, func(doc *idf.Document) {
		doc.Objects = append(doc.Objects, idf.Object{Type: "AirLoopHVAC:UnitarySystem", Fields: []idf.Field{{Value: "Foreign unit"}, {Value: "Fan:VariableVolume"}, {Value: fan.ObjectName}}})
	})
	if err = epathSQLPoolFanOriginalOccurrences(foreign, fan); err == nil {
		t.Fatal("native fan gained a second consuming parent")
	}
}

func epathSQLPoolFanHandNumeric(t *testing.T) (epathSQLFrames, epathRealSQLModel) {
	t.Helper()
	_, d, native := epathSQLPoolFanOriginalAndSources(t)
	fan := epathSQLPoolFanDeclaration()
	precision := native.Precision
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{}, Loads: map[string]epathSQLQuantity{}, LoadSourceIDs: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{}, Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, AirLoopFans: map[string]epathRealSQLAirLoopFan{fan.SiteID: fan}, PoolSystems: []epathSQLPoolSourceFrames{native}}
	model := epathRealSQLModel{Precision: precision, PoolSystems: []epathRealSQLPoolSystem{d}, AirLoopFans: []epathRealSQLAirLoopFan{fan}, Site: []epathRealSQLSite{{ID: fan.SiteID, EndUse: "fans", Carrier: "electricity", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Fans:Electricity", Unit: "J"}}, Keys: []string{""}, IsMeter: true}}}, Auxiliaries: []epathRealSQLAuxiliary{{SiteID: fan.SiteID, ServedZones: fan.ServedZones, Weight: "cooling_plus_heating", AllocationMethod: "air_loop_load_share", ReconciliationID: "allocation.fans.annual"}}}
	monthSource := func(index int, name, key string, meter bool, values [12]float64) epathRealSQLSource {
		s := epathRealSQLSource{DictionaryIndex: index, Name: name, KeyValue: key, IsMeter: meter, ReportingFrequency: "Monthly", SourceUnit: "J", Rows: 12}
		for month, value := range values {
			s.Months = append(s.Months, epathRealSQLMonth{Month: month + 1, Rows: 1, RawSum: epathOracleNumber(value * 3600000), EnergyKWh: epathOracleNumber(value)})
		}
		return s
	}
	zones := append(append([]string(nil), d.ServedZones...), d.ReturnPlenumZoneName)
	for z, zone := range zones {
		frames.Zones[strings.ToLower(zone)] = epathSQLZone{Name: zone, Multiplier: 3}
		for serviceIndex, service := range []string{"cooling", "heating"} {
			values := [12]float64{}
			if z < 5 {
				m1 := []float64{1, 2, 3, 4, 10}[z]
				m2 := []float64{10, 4, 3, 2, 1}[z]
				// Both services remain independently active fan weights even
				// while the whole Heating purchased conversion is restricted.
				if serviceIndex == 0 {
					values[0], values[1] = m1*0.6, m2*0.25
				} else {
					values[0], values[1] = m1*0.4, m2*0.75
				}
			} else {
				values[0], values[1] = 999, 999
			}
			name := "Zone Air System Sensible Cooling Energy"
			if service == "heating" {
				name = "Zone Air System Sensible Heating Energy"
			}
			id := 1000 + z*2 + serviceIndex
			source := monthSource(id, name, zone, false, values)
			frames.SourceIdentities[id] = source
			monthly, err := epathSQLMonthly(source, precision)
			if err != nil {
				t.Fatal(err)
			}
			for month, value := range monthly {
				k := epathSQLKey(strings.ToLower(zone), service, month+1)
				frames.Loads[k] = value.times(3)
				frames.LoadSourceIDs[k] = []int{id}
			}
		}
	}
	meters := [12]float64{20, 40}
	meter := monthSource(9000, "Fans:Electricity", "", true, meters)
	frames.SourceIdentities[9000] = meter
	frames.SiteSources[fan.SiteID] = []int{9000}
	monthly, err := epathSQLMonthly(meter, precision)
	if err != nil {
		t.Fatal(err)
	}
	for i := range monthly {
		value := monthly[i]
		frames.Site[fan.SiteID] = append(frames.Site[fan.SiteID], &value)
	}
	return frames, model
}

func TestEnergyPathSQLPoolFanRetainsExistingMonthlyMathAndSourceBoundary(t *testing.T) {
	frames, model := epathSQLPoolFanHandNumeric(t)
	// The future integration must refuse numeric fan proofs until original
	// binding has actually been published; mere source/frame presence is not it.
	if _, err := epathSQLAuxiliaryZoneProofs(frames, model); err == nil {
		t.Fatal("unbound Pool fan obtained numeric acceptance")
	}
	original := epathSQLPoolOriginalFixture(t)
	frames.AirLoopFans = nil
	if err := epathSQLBindAirLoopFans(original, model, &frames); err != nil {
		t.Fatal(err)
	}
	proofs, err := epathSQLAuxiliaryZoneProofs(frames, model)
	if err != nil {
		t.Fatal(err)
	}
	for z, zone := range append(append([]string(nil), model.AirLoopFans[0].ServedZones...), "PLENUM-1") {
		for _, period := range epathSQLZoneCarrierPeriods() {
			want := 0.0
			if z < 5 {
				switch period {
				case "M1":
					want = []float64{1, 2, 3, 4, 10}[z]
				case "M2":
					want = []float64{20, 8, 6, 4, 2}[z]
				case "annual":
					want = []float64{21, 10, 9, 8, 12}[z]
				}
			}
			p := proofs[epathSQLAuxiliaryZoneKey("fans.electricity", zone, period)]
			if p == nil || math.Abs(p.Value.Value-want) > 1e-10 || p.Basis != "service_path_allocation" || p.Owned != (z < 5) {
				t.Fatalf("fan monthly-first value/owner changed: %s/%s got%#v want%g", zone, period, p, want)
			}
			if len(p.Sources) != 1 || p.Sources["sql-rdd-9000"].RDD == nil || p.Sources["sql-rdd-9000"].RDD.Name != "Fans:Electricity" {
				t.Fatal("native Pool or boiler/pump budget was added to Fans")
			}
			for id, source := range p.NodeSources {
				if id == "sql-rdd-9000" {
					continue
				}
				if source.RDD == nil || source.RDD.KeyValue != zone || !strings.HasPrefix(source.RDD.Name, "Zone Air System Sensible ") {
					t.Fatalf("fan weight source crossed its original Zone/service boundary: %s", id)
				}
			}
		}
	}
	if len(frames.PoolAirLoopFans) != 1 {
		t.Fatal("Pool fan binding stamp was not published")
	}
	// Candidate-independent fixture values show monthly-first behavior: annual
	// budget60 times annual first-Zone weight11/40=16.5, not the correct21.
	if p := proofs[epathSQLAuxiliaryZoneKey("fans.electricity", "SPACE1-1", "annual")]; math.Abs(p.Value.Value-16.5) < 1e-10 {
		t.Fatal("annual-ratio recomputation replaced Monthly fan shares")
	}
	if got := len(proofs); got != 6*13 {
		t.Fatalf("full zero/positive/PLENUM fan context roster=%d want%d", got, 6*13)
	}
}
