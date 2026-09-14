package simulation

// Test-only finite original proof. This finite DC-supply/thermal proof is stricter
// than the independent common native-storage charge non-consumption rule.
// Only the lexical parser and existing test-only exact-name lookup are shared.

import (
	"crypto/sha256"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathRealSQLPVSystem struct {
	ID                string                    `json:"id"`
	LoadCenterName    string                    `json:"loadCenterName"`
	GeneratorListName string                    `json:"generatorListName"`
	InverterName      string                    `json:"inverterName"`
	StorageName       string                    `json:"storageName"`
	PerformanceName   string                    `json:"performanceName"`
	Generators        []epathRealSQLPVGenerator `json:"generators"`
}

type epathRealSQLPVGenerator struct {
	Name        string `json:"name"`
	SurfaceName string `json:"surfaceName"`
	ZoneName    string `json:"zoneName"`
}

type epathSQLPVOriginalObject struct {
	ObjectType, ObjectName string
	ObjectIndex            int
	Fields                 []string // Exact physical field values; comments are not authority.
}

type epathSQLPVOriginalProof struct {
	Declaration     epathRealSQLPVSystem
	OriginalSHA256  string
	Owners          []epathSQLPVOriginalObject // Load center, inverter, storage, five PVs.
	PhysicalObjects []epathSQLPVOriginalObject // Also list, performance, surfaces, Zones, battery curves.
}

func epathSQLPVObject(o idf.Object) epathSQLPVOriginalObject {
	r := epathSQLPVOriginalObject{ObjectType: o.Type, ObjectName: epathSQLSharedField(o, 0), ObjectIndex: o.Index}
	for i := range o.Fields {
		r.Fields = append(r.Fields, epathSQLSharedField(o, i))
	}
	return r
}

func epathSQLPVNumber(o idf.Object, field int, low, high float64) bool {
	v, err := strconv.ParseFloat(epathSQLSharedField(o, field), 64)
	return err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= low && v <= high
}

func epathSQLValidatePVOriginal(original string, declarations []epathRealSQLPVSystem) ([]epathSQLPVOriginalProof, error) {
	if len(declarations) == 0 {
		return nil, nil
	}
	if len(declarations) != 1 || strings.TrimSpace(original) == "" {
		return nil, fmt.Errorf("PV original requires one finite declared system")
	}
	d := declarations[0]
	for _, s := range []string{d.ID, d.LoadCenterName, d.GeneratorListName, d.InverterName, d.StorageName, d.PerformanceName} {
		if strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("PV declaration has blank identity")
		}
	}
	if len(d.Generators) != 5 {
		return nil, fmt.Errorf("PV declaration requires five generators")
	}
	doc, err := idf.Parse(original)
	if err != nil {
		return nil, err
	}
	w := epathSQLSharedOriginal{doc: doc}
	counts := map[string]int{}
	for _, o := range doc.Objects {
		counts[epathSQLSharedKey(o.Type)]++
	}
	for typ, n := range map[string]int{"version": 1, "zone": 5, "zonegroup": 0, "electricloadcenter:distribution": 1, "electricloadcenter:generators": 1, "electricloadcenter:storage:battery": 1, "electricloadcenter:inverter:lookuptable": 1, "generator:photovoltaic": 5, "photovoltaicperformance:simple": 1} {
		if counts[typ] != n {
			return nil, fmt.Errorf("PV original %s census=%d want=%d", typ, counts[typ], n)
		}
	}
	// A different native storage/inverter/generator cannot hide behind a same
	// reporting key, and a second converter/transformer changes this AC closure.
	for _, group := range []struct {
		prefix string
		count  int
	}{{"electricloadcenter:", 4}, {"generator:", 5}, {"photovoltaicperformance:", 1}} {
		n := 0
		for _, o := range doc.Objects {
			if strings.HasPrefix(epathSQLSharedKey(o.Type), group.prefix) {
				n++
			}
		}
		if n != group.count {
			return nil, fmt.Errorf("PV original hidden/unsupported %s owner", group.prefix)
		}
	}
	for _, o := range doc.Objects {
		if strings.EqualFold(o.Type, "Version") && epathSQLSharedField(o, 0) != "25.1" {
			return nil, fmt.Errorf("PV original requires reviewed25.1 schema")
		}
	}
	lc, err := w.one("ElectricLoadCenter:Distribution", d.LoadCenterName)
	if err != nil {
		return nil, err
	}
	inv, err := w.one("ElectricLoadCenter:Inverter:LookUpTable", d.InverterName)
	if err != nil {
		return nil, err
	}
	bat, err := w.one("ElectricLoadCenter:Storage:Battery", d.StorageName)
	if err != nil {
		return nil, err
	}
	list, err := w.one("ElectricLoadCenter:Generators", d.GeneratorListName)
	if err != nil {
		return nil, err
	}
	perf, err := w.one("PhotovoltaicPerformance:Simple", d.PerformanceName)
	if err != nil {
		return nil, err
	}
	for field, want := range map[int]string{1: d.GeneratorListName, 2: "TrackElectrical", 6: "DirectCurrentWithInverterDCStorage", 7: d.InverterName, 8: d.StorageName, 10: "TrackFacilityElectricDemandStoreExcessOnSite"} {
		if !epathSQLSharedSame(epathSQLSharedField(lc, field), want) {
			return nil, fmt.Errorf("PV distribution field%d differs", field)
		}
	}
	for _, field := range []int{4, 5, 9, 11, 12} {
		if epathSQLSharedField(lc, field) != "" {
			return nil, fmt.Errorf("PV distribution has additional control/conversion reference")
		}
	}
	if len(lc.Fields) != 15 || !epathSQLPVNumber(lc, 13, 0, 1) || !epathSQLPVNumber(lc, 14, 0, 1) {
		return nil, fmt.Errorf("PV distribution finite shape/SOC limits differ")
	}
	maxSOC, _ := strconv.ParseFloat(epathSQLSharedField(lc, 13), 64)
	minSOC, _ := strconv.ParseFloat(epathSQLSharedField(lc, 14), 64)
	if minSOC > maxSOC {
		return nil, fmt.Errorf("PV inverted SOC limits")
	}
	if len(inv.Fields) != 13 || len(bat.Fields) != 21 || epathSQLSharedField(inv, 2) != "" || epathSQLSharedField(bat, 2) != "" {
		return nil, fmt.Errorf("PV inverter/battery shape or Zone-heat boundary differs")
	}
	if len(list.Fields) != 26 || len(perf.Fields) != 5 || !epathSQLSharedSame(epathSQLSharedField(perf, 2), "Fixed") || epathSQLSharedField(perf, 4) != "" || !epathSQLPVNumber(perf, 1, 0, 1) || !epathSQLPVNumber(perf, 3, 0, 1) {
		return nil, fmt.Errorf("PV generator list or fixed performance differs")
	}
	p := epathSQLPVOriginalProof{Declaration: d, OriginalSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(original)))}
	p.Declaration.Generators = append([]epathRealSQLPVGenerator(nil), d.Generators...)
	for _, o := range []idf.Object{lc, inv, bat} {
		p.Owners = append(p.Owners, epathSQLPVObject(o))
		p.PhysicalObjects = append(p.PhysicalObjects, epathSQLPVObject(o))
	}
	for _, o := range []idf.Object{list, perf} {
		p.PhysicalObjects = append(p.PhysicalObjects, epathSQLPVObject(o))
	}
	// The battery's actual curves are physical references, not Ah->kWh formulae.
	for _, c := range []struct {
		field int
		typ   string
	}{{12, "Curve:RectangularHyperbola2"}, {13, "Curve:RectangularHyperbola2"}, {20, "Curve:DoubleExponentialDecay"}} {
		name := epathSQLSharedField(bat, c.field)
		curve, err := w.one(c.typ, name)
		if err != nil {
			return nil, err
		}
		n := 0
		for _, o := range doc.Objects {
			if strings.HasPrefix(epathSQLSharedKey(o.Type), "curve:") && epathSQLSharedSame(epathSQLSharedField(o, 0), name) {
				n++
			}
		}
		if n != 1 {
			return nil, fmt.Errorf("PV battery curve reporting namespace ambiguous")
		}
		p.PhysicalObjects = append(p.PhysicalObjects, epathSQLPVObject(curve))
	}
	seenPV, seenSurface, seenZone := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, g := range d.Generators {
		for _, v := range []struct {
			s    string
			seen map[string]bool
		}{{g.Name, seenPV}, {g.SurfaceName, seenSurface}, {g.ZoneName, seenZone}} {
			key := epathSQLSharedKey(v.s)
			if key == "" || v.seen[key] {
				return nil, fmt.Errorf("PV duplicate/blank declared generator/surface/Zone")
			}
			v.seen[key] = true
		}
		pv, err := w.one("Generator:Photovoltaic", g.Name)
		if err != nil {
			return nil, err
		}
		surface, err := w.one("BuildingSurface:Detailed", g.SurfaceName)
		if err != nil {
			return nil, err
		}
		zone, err := w.one("Zone", g.ZoneName)
		if err != nil {
			return nil, err
		}
		if len(pv.Fields) != 7 || !epathSQLSharedSame(epathSQLSharedField(pv, 1), g.SurfaceName) || !epathSQLSharedSame(epathSQLSharedField(pv, 2), "PhotovoltaicPerformance:Simple") || !epathSQLSharedSame(epathSQLSharedField(pv, 3), d.PerformanceName) || !epathSQLSharedSame(epathSQLSharedField(pv, 4), "Decoupled") {
			return nil, fmt.Errorf("PV exact generator/performance/Decoupled identity differs for %s", g.Name)
		}
		if !epathSQLPVNumber(pv, 5, 1, 1) || !epathSQLPVNumber(pv, 6, 1, 1) {
			return nil, fmt.Errorf("PV original module configuration differs")
		}
		if !epathSQLSharedSame(epathSQLSharedField(surface, 1), "Ceiling") || !epathSQLSharedSame(epathSQLSharedField(surface, 3), g.ZoneName) || epathSQLSharedField(surface, 4) != "" || !epathSQLSharedSame(epathSQLSharedField(surface, 5), "Outdoors") || !epathSQLPVNumber(zone, 6, 1, 1) {
			return nil, fmt.Errorf("PV original surface/Zone/factor boundary differs for %s", g.Name)
		}
		// EnergyPlus surface names are shared across surface subtypes.
		nSurface := 0
		for _, o := range doc.Objects {
			typ := epathSQLSharedKey(o.Type)
			for _, prefix := range []string{"buildingsurface:", "fenestrationsurface:", "shading:", "wall:", "roof:", "roofceiling:", "ceiling:", "floor:", "window:", "door:", "glazeddoor:"} {
				if strings.HasPrefix(typ, prefix) && epathSQLSharedSame(epathSQLSharedField(o, 0), g.SurfaceName) {
					nSurface++
					break
				}
			}
		}
		if nSurface != 1 {
			return nil, fmt.Errorf("PV surface namespace ambiguous for %s", g.SurfaceName)
		}
		nList := 0
		for i := 1; i < len(list.Fields); i += 5 {
			if epathSQLSharedSame(epathSQLSharedField(list, i), g.Name) {
				nList++
				if !epathSQLSharedSame(epathSQLSharedField(list, i+1), "Generator:Photovoltaic") || !epathSQLPVNumber(list, i+2, math.SmallestNonzeroFloat64, math.MaxFloat64) || epathSQLSharedField(list, i+3) != "" || epathSQLSharedField(list, i+4) != "" {
					return nil, fmt.Errorf("PV native generator-list tuple differs")
				}
			}
		}
		if nList != 1 {
			return nil, fmt.Errorf("PV generator-list membership ambiguous/missing for %s", g.Name)
		}
		p.Owners = append(p.Owners, epathSQLPVObject(pv))
		for _, o := range []idf.Object{pv, surface, zone} {
			p.PhysicalObjects = append(p.PhysicalObjects, epathSQLPVObject(o))
		}
	}
	return []epathSQLPVOriginalProof{p}, nil
}

// Future native-source compiler must call this using separately hash-bound
// original/executed evidence text, never infer execution indices from offsets.
func epathSQLBindPVExecuted(original, executed string, p epathSQLPVOriginalProof) ([]epathSQLPVOriginalObject, error) {
	rebuilt, err := epathSQLValidatePVOriginal(original, []epathRealSQLPVSystem{p.Declaration})
	if err != nil || len(rebuilt) != 1 || !reflect.DeepEqual(rebuilt[0], p) {
		return nil, fmt.Errorf("PV original proof was altered/unbound")
	}
	x, err := epathSQLValidatePVOriginal(executed, []epathRealSQLPVSystem{p.Declaration})
	if err != nil {
		return nil, err
	}
	if len(x) != 1 || len(x[0].PhysicalObjects) != len(p.PhysicalObjects) {
		return nil, fmt.Errorf("PV executed physical roster differs")
	}
	for i, o := range p.PhysicalObjects {
		actual := x[0].PhysicalObjects[i]
		if !epathSQLSharedSame(actual.ObjectType, o.ObjectType) || !epathSQLSharedSame(actual.ObjectName, o.ObjectName) || !reflect.DeepEqual(actual.Fields, o.Fields) {
			return nil, fmt.Errorf("PV executed physical fields changed: %s/%s", o.ObjectType, o.ObjectName)
		}
	}
	return x[0].Owners, nil
}
