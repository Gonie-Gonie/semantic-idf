package simulation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func epathSQLPoolBoundaryHandNative(t *testing.T) epathSQLPoolSourceFrames {
	t.Helper()
	proofs, err := epathSQLValidatePoolOriginal(epathSQLPoolOriginalFixture(t), []epathRealSQLPoolSystem{epathSQLPoolOriginalDeclaration()})
	if err != nil || len(proofs) != 1 {
		t.Fatalf("original Pool proof: %v", err)
	}
	return epathSQLPoolSourceHandFramesForOriginal(t, proofs[0])
}

// Hand graph quantities are deliberately not used as SQL/scalar expectations.
// Positive Heating load and site remain present; only their unproved ratio is
// denied. Independent scalar/source/ledger validators own numerical closure.
func epathSQLPoolBoundaryHandGraph(scope EnergyExplanationScope, period string) ([]EnergyExplanationNode, []EnergyPathLink, EnergyExplanationSummary) {
	suffix := "building"
	if scope.Kind == "zone" {
		suffix = strings.ReplaceAll(strings.ToLower(scope.ZoneName), " ", "_")
	}
	id := func(kind string) string { return kind + "." + suffix }
	nodes := []EnergyExplanationNode{
		{ID: id("load.heating"), Level: "load", Kind: "load.heating", ServiceKind: "heating", Value: 100, Unit: "kWh", ScaleDomain: "thermal"},
		{ID: id("end_use.heating"), Level: "end_use", Kind: "end_use.heating", ServiceKind: "heating", EndUse: "heating", Value: 40, Unit: "kWh", ScaleDomain: "site"},
		{ID: id("carrier.natural_gas"), Level: "carrier", Kind: "carrier.natural_gas", Carrier: "natural_gas", Value: 40, Unit: "kWh", ScaleDomain: "site"},
		{ID: id("load.cooling"), Level: "load", Kind: "load.cooling", ServiceKind: "cooling", Value: 80, Unit: "kWh", ScaleDomain: "thermal"},
		{ID: id("end_use.cooling"), Level: "end_use", Kind: "end_use.cooling", ServiceKind: "cooling", EndUse: "cooling", Value: 20, Unit: "kWh", ScaleDomain: "site"},
		{ID: id("end_use.fans"), Level: "end_use", Kind: "end_use.fans", EndUse: "fans", Value: 3, Unit: "kWh", ScaleDomain: "site"},
		{ID: id("end_use.pumps"), Level: "end_use", Kind: "end_use.pumps", EndUse: "pumps", Value: 7, Unit: "kWh", ScaleDomain: "site"},
		{ID: id("carrier.electricity"), Level: "carrier", Kind: "carrier.electricity", Carrier: "electricity", Value: 30, Unit: "kWh", ScaleDomain: "site"},
	}
	for i := range nodes {
		nodes[i].Period, nodes[i].ZoneName, nodes[i].AggregationBasis = period, scope.ZoneName, "model_total"
	}
	links := []EnergyPathLink{
		{ID: "heating-purchased", FromID: id("end_use.heating"), ToID: id("carrier.natural_gas"), Relation: "end_use_to_carrier", ServiceKind: "heating", FromValue: 40, ToValue: 40, FromUnit: "kWh", ToUnit: "kWh"},
		{ID: "cooling-conversion", FromID: id("load.cooling"), ToID: id("end_use.cooling"), Relation: "load_to_end_use", ServiceKind: "cooling", Basis: "service_path_allocation", FromValue: 80, ToValue: 20, FromUnit: "kWh", ToUnit: "kWh", Ratio: 4, RatioKind: "load_to_site_energy"},
		{ID: "cooling-purchased", FromID: id("end_use.cooling"), ToID: id("carrier.electricity"), Relation: "end_use_to_carrier", ServiceKind: "cooling", FromValue: 20, ToValue: 20, FromUnit: "kWh", ToUnit: "kWh"},
		{ID: "fan-purchased", FromID: id("end_use.fans"), ToID: id("carrier.electricity"), Relation: "end_use_to_carrier", FromValue: 3, ToValue: 3, FromUnit: "kWh", ToUnit: "kWh"},
		{ID: "cw-pump-purchased", FromID: id("end_use.pumps"), ToID: id("carrier.electricity"), Relation: "end_use_to_carrier", FromValue: 7, ToValue: 7, FromUnit: "kWh", ToUnit: "kWh"},
	}
	for i := range links {
		links[i].Period, links[i].ZoneName = period, scope.ZoneName
	}
	summary := EnergyExplanationSummary{Scope: scope, Period: period,
		Loads:       []EnergyExplanationSummaryItem{{ID: id("load.heating"), ServiceKind: "heating", Value: 100, Unit: "kWh"}},
		EndUses:     []EnergyExplanationSummaryItem{{ID: id("end_use.heating"), EndUse: "heating", Value: 40, Unit: "kWh"}},
		Ratios:      []EnergyExplanationSummaryItem{{ID: "cooling-ratio", ServiceKind: "cooling", Value: 4, DenominatorLabel: id("end_use.cooling")}},
		DerivedKPIs: []EnergyExplanationSummaryItem{{ID: "kpi.cooling_cop", Kind: "kpi.cooling_cop", ServiceKind: "cooling", Value: 4}},
	}
	return nodes, links, summary
}

func epathSQLPoolBoundaryHandBundle() PurposeResultBundle {
	building := EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}
	nodes, links, summary := epathSQLPoolBoundaryHandGraph(building, "annual")
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: building, Nodes: nodes, Links: links}, EnergyExplanationSummary: summary}
	periods := func(scope EnergyExplanationScope) []EnergyPeriod {
		out := []EnergyPeriod{}
		for month := 0; month <= 12; month++ {
			id, kind := "annual", "annual"
			if month > 0 {
				id, kind = fmt.Sprintf("M%d", month), "monthly"
			}
			n, l, s := epathSQLPoolBoundaryHandGraph(scope, id)
			out = append(out, EnergyPeriod{ID: id, Kind: kind, Nodes: n, Links: l, Summary: &s})
		}
		return out
	}
	bundle.EnergyExplanation.Periods = periods(building)
	for _, name := range []string{"SPACE1-1", "SPACE2-1", "SPACE3-1", "SPACE4-1", "SPACE5-1", "PLENUM-1"} {
		scope := EnergyExplanationScope{Kind: "zone", ZoneName: name, AggregationBasis: "model_total"}
		n, l, s := epathSQLPoolBoundaryHandGraph(scope, "annual")
		bundle.EnergyExplanation.ZoneResults = append(bundle.EnergyExplanation.ZoneResults, EnergyExplanationZoneResult{Scope: scope, Nodes: n, Links: l, Summary: s, Periods: periods(scope)})
		bundle.EnergyExplanation.AvailableZones = append(bundle.EnergyExplanation.AvailableZones, name)
	}
	return bundle
}

func epathSQLPoolBoundaryBadLink(nodes []EnergyExplanationNode, period, zone string) EnergyPathLink {
	return EnergyPathLink{ID: "unproved-heating", FromID: nodes[0].ID, ToID: nodes[1].ID, Relation: "load_to_end_use", ServiceKind: "heating", FromValue: 100, ToValue: 40, FromUnit: "kWh", ToUnit: "kWh", Ratio: 2.5, RatioKind: "coefficient_of_performance", Period: period, ZoneName: zone}
}

func TestEnergyPathRealSQLPoolBoundaryPreservesPositiveEnergyAndIndependentCooling(t *testing.T) {
	native := epathSQLPoolBoundaryHandNative(t)
	bundle := epathSQLPoolBoundaryHandBundle()
	before := epathSQLPoolBoundaryHandBundle()
	for _, selection := range []struct{ scope, zone string }{{"building", ""}, {"zone", "SPACE1-1"}, {"zone", "SPACE2-1"}, {"zone", "PLENUM-1"}} {
		for _, period := range []string{"annual", "M1", "M12"} {
			proof := &epathSQLPoolBoundaryProof{Native: native, Scope: selection.scope, Zone: selection.zone, Period: period}
			if err := epathSQLCheckPoolBoundary(bundle, proof); err != nil {
				t.Fatalf("%s/%s/%s: %v", selection.scope, selection.zone, period, err)
			}
		}
	}
	if !reflect.DeepEqual(bundle, before) {
		t.Fatal("negative consumer changed observed loads/site, source traces, CW/fan branches or cached summary")
	}
}

func TestEnergyPathRealSQLPoolBoundaryRejectsAnyHeatingConversionIdentity(t *testing.T) {
	native := epathSQLPoolBoundaryHandNative(t)
	mutations := map[string]func(*EnergyPathLink, []EnergyExplanationNode){
		"arbitrary direct basis":     func(l *EnergyPathLink, _ []EnergyExplanationNode) { l.Basis = "direct_zone_energy" },
		"arbitrary allocation basis": func(l *EnergyPathLink, _ []EnergyExplanationNode) { l.Basis = "service_path_allocation" },
		"no basis ratio kind sources": func(l *EnergyPathLink, _ []EnergyExplanationNode) {
			l.Basis = ""
			l.RatioKind = ""
			l.Ratio = 0
			l.SourceIDs = nil
		},
		"different ratio kind": func(l *EnergyPathLink, _ []EnergyExplanationNode) { l.RatioKind = "efficiency" },
		"zero-valued conversion": func(l *EnergyPathLink, _ []EnergyExplanationNode) {
			l.FromValue = 0
			l.ToValue = 0
			l.Ratio = 0
			l.RatioKind = ""
		},
		"misleading service": func(l *EnergyPathLink, _ []EnergyExplanationNode) { l.ServiceKind = "cooling" },
		"erased service endpoint metadata": func(l *EnergyPathLink, n []EnergyExplanationNode) {
			l.ServiceKind = ""
			for _, i := range []int{0, 1} {
				n[i].ServiceKind = ""
				n[i].EndUse = ""
				n[i].Kind = ""
			}
		},
		"opaque endpoint ids still typed heating": func(l *EnergyPathLink, n []EnergyExplanationNode) {
			n[0].ID = "opaque-thermal"
			n[1].ID = "opaque-site"
			l.FromID = n[0].ID
			l.ToID = n[1].ID
			l.ServiceKind = ""
		},
		"case-space relation": func(l *EnergyPathLink, _ []EnergyExplanationNode) {
			l.Relation = " LOAD_TO_END_USE "
			l.ServiceKind = " HEATING "
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			bundle := epathSQLPoolBoundaryHandBundle()
			link := epathSQLPoolBoundaryBadLink(bundle.EnergyExplanation.Nodes, "annual", "")
			mutate(&link, bundle.EnergyExplanation.Nodes)
			// Keep unrelated references consistent when the hand mutation renames
			// endpoints, so rejection is semantic rather than a dangling-edge test.
			bundle.EnergyExplanation.Links[0].FromID = bundle.EnergyExplanation.Nodes[1].ID
			bundle.EnergyExplanation.Links = append(bundle.EnergyExplanation.Links, link)
			if err := epathSQLCheckPoolBoundary(bundle, &epathSQLPoolBoundaryProof{Native: native, Scope: "building", Period: "annual"}); err == nil {
				t.Fatal("unproved heating conversion escaped through candidate labels/basis/sources")
			}
		})
	}
}

func TestEnergyPathRealSQLPoolBoundaryRejectsInvalidProofAndMissingView(t *testing.T) {
	native := epathSQLPoolBoundaryHandNative(t)
	for _, proof := range []*epathSQLPoolBoundaryProof{nil, {}, {Native: native, Scope: "all", Period: "annual"}, {Native: native, Scope: "building", Zone: "SPACE1-1", Period: "annual"}, {Native: native, Scope: "zone", Period: "annual"}, {Native: native, Scope: "zone", Zone: "foreign", Period: "annual"}, {Native: native, Scope: "building", Period: "M01"}, {Native: native, Scope: "building", Period: "M13"}} {
		if err := epathSQLCheckPoolBoundary(epathSQLPoolBoundaryHandBundle(), proof); err == nil {
			t.Fatal("nil/foreign/noncanonical denial proof accepted")
		}
	}
	for name, mutate := range map[string]func(*PurposeResultBundle){
		"missing monthly": func(b *PurposeResultBundle) { b.EnergyExplanation.Periods = b.EnergyExplanation.Periods[:1] },
		"duplicate monthly": func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods = append(b.EnergyExplanation.Periods, b.EnergyExplanation.Periods[1])
		},
		"wrong monthly kind": func(b *PurposeResultBundle) { b.EnergyExplanation.Periods[1].Kind = "annual" },
		"wrong root scope":   func(b *PurposeResultBundle) { b.EnergyExplanation.Scope.Kind = "zone" },
	} {
		t.Run(name, func(t *testing.T) {
			b := epathSQLPoolBoundaryHandBundle()
			mutate(&b)
			if err := epathSQLCheckPoolBoundary(b, &epathSQLPoolBoundaryProof{Native: native, Scope: "building", Period: "M1"}); err == nil {
				t.Fatal("missing or conflicting selected graph accepted")
			}
		})
	}
	bad := native
	bad.Original.ServedZones = append([]string(nil), native.Original.ServedZones[:4]...)
	if err := epathSQLCheckPoolBoundary(epathSQLPoolBoundaryHandBundle(), &epathSQLPoolBoundaryProof{Native: bad, Scope: "building", Period: "annual"}); err == nil {
		t.Fatal("shrunk original native served roster granted denial proof")
	}
	bad = native
	bad.Parents = map[string]epathSQLPoolParentClosure{}
	if err := epathSQLCheckPoolBoundary(epathSQLPoolBoundaryHandBundle(), &epathSQLPoolBoundaryProof{Native: bad, Scope: "building", Period: "annual"}); err == nil {
		t.Fatal("missing native parent evidence granted denial proof")
	}
}

func TestEnergyPathRealSQLPoolBoundaryCacheIdentityDoesNotDependOnPositiveValue(t *testing.T) {
	native := epathSQLPoolBoundaryHandNative(t)
	for name, item := range map[string]EnergyExplanationSummaryItem{
		"explicit service with zero": {ID: "opaque", ServiceKind: "heating", Value: 0},
		"explicit end use":           {ID: "opaque", EndUse: "heating", Value: 2},
		"legacy id contradicts kind": {ID: "kpi.heating_cop", Kind: "kpi.cooling_cop", ServiceKind: "cooling", Value: 2},
		"legacy kind contradicts id": {ID: "kpi.cooling_cop", Kind: "kpi.heating_cop", ServiceKind: "cooling", Value: 2},
		"typed opaque denominator":   {ID: "opaque-ratio", DenominatorLabel: "opaque-heating-site", Value: 2},
		"canonical numerator":        {ID: "opaque-ratio", NumeratorLabel: "load.heating.building", Value: 2},
	} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/legacy=%t", name, legacy), func(t *testing.T) {
				bundle := epathSQLPoolBoundaryHandBundle()
				bundle.EnergyExplanation.Nodes[1].ID = "opaque-heating-site"
				bundle.EnergyExplanation.Links[0].FromID = "opaque-heating-site"
				if legacy {
					bundle.EnergyExplanationSummary.DerivedKPIs = []EnergyExplanationSummaryItem{item}
				} else {
					bundle.EnergyExplanationSummary.Ratios = []EnergyExplanationSummaryItem{item}
				}
				if err := epathSQLCheckPoolBoundary(bundle, &epathSQLPoolBoundaryProof{Native: native, Scope: "building", Period: "annual"}); err == nil {
					t.Fatal("unsafe cached Heating ratio escaped through zero, missing sources or contradictory metadata")
				}
			})
		}
	}
	// Exact identity is not a label or substring heuristic. These unrelated
	// contexts may contain the word heating without asserting Heating COP.
	bundle := epathSQLPoolBoundaryHandBundle()
	bundle.EnergyExplanationSummary.Ratios = append(bundle.EnergyExplanationSummary.Ratios, EnergyExplanationSummaryItem{ID: "water_heating_context", Label: "Heating COP documentation", ServiceKind: "cooling", DenominatorLabel: "end_use.heating_auxiliary.building", Value: 1})
	if err := epathSQLCheckPoolBoundary(bundle, &epathSQLPoolBoundaryProof{Native: native, Scope: "building", Period: "annual"}); err != nil {
		t.Fatalf("label/substring overreach erased unrelated context: %v", err)
	}
}

func TestEnergyPathRealSQLPoolBoundaryPoolDemandMatchesExactSourceOwner(t *testing.T) {
	native := epathSQLPoolBoundaryHandNative(t)
	for name, mutate := range map[string]func(*epathSQLPoolOriginalOwner){
		"foreign object index":            func(o *epathSQLPoolOriginalOwner) { o.ObjectIndex = 99999 },
		"different typed object":          func(o *epathSQLPoolOriginalOwner) { o.ObjectType = "SwimmingPool:Outdoor" },
		"different name":                  func(o *epathSQLPoolOriginalOwner) { o.ObjectName = "Other Pool" },
		"case-only changed original name": func(o *epathSQLPoolOriginalOwner) { o.ObjectName = strings.ToUpper(o.ObjectName) },
		"different Zone":                  func(o *epathSQLPoolOriginalOwner) { o.ZoneName = "SPACE2-1" },
		"different plant":                 func(o *epathSQLPoolOriginalOwner) { o.PlantLoopName = native.Original.Declaration.ChilledWaterLoopName },
	} {
		t.Run(name, func(t *testing.T) {
			frame := native
			frame.Original.HotWaterDemands = append([]epathSQLPoolOriginalOwner(nil), native.Original.HotWaterDemands...)
			found := false
			for i := range frame.Original.HotWaterDemands {
				if frame.Original.HotWaterDemands[i].ObjectType == "SwimmingPool:Indoor" {
					mutate(&frame.Original.HotWaterDemands[i])
					found = true
					break
				}
			}
			if !found {
				t.Fatal("hand native proof lost Pool demand")
			}
			if err := epathSQLCheckPoolBoundary(epathSQLPoolBoundaryHandBundle(), &epathSQLPoolBoundaryProof{Native: frame, Scope: "building", Period: "annual"}); err == nil {
				t.Fatal("a different original demand borrowed the native Pool source owner")
			}
		})
	}
	// The unchanged positive frame must remain usable after all mutations;
	// this also checks that the mutation fixtures do not edit shared evidence.
	if err := epathSQLCheckPoolBoundary(epathSQLPoolBoundaryHandBundle(), &epathSQLPoolBoundaryProof{Native: native, Scope: "building", Period: "annual"}); err != nil {
		t.Fatal(err)
	}
}

// Raw original wire is built explicitly. Do not invoke the application's
// Bundle/Result/Period/Summary Marshal/Unmarshal compatibility repair paths.
func epathSQLPoolBoundaryHandWire() map[string]any {
	b := epathSQLPoolBoundaryHandBundle()
	summary := func(s EnergyExplanationSummary) map[string]any {
		return map[string]any{"scope": s.Scope, "period": s.Period, "loads": s.Loads, "endUses": s.EndUses, "ratios": s.Ratios, "derivedKpis": s.DerivedKPIs}
	}
	graph := func(n []EnergyExplanationNode, l []EnergyPathLink) map[string]any {
		type plainNode EnergyExplanationNode
		plain := make([]plainNode, len(n))
		for i := range n {
			plain[i] = plainNode(n[i])
		}
		return map[string]any{"nodes": plain, "links": l}
	}
	periods := func(rows []EnergyPeriod) []any {
		out := []any{}
		for _, p := range rows {
			g := graph(p.Nodes, p.Links)
			g["id"], g["kind"] = p.ID, p.Kind
			if p.Summary != nil {
				g["summary"] = summary(*p.Summary)
			}
			out = append(out, g)
		}
		return out
	}
	root := graph(b.EnergyExplanation.Nodes, b.EnergyExplanation.Links)
	root["schema"], root["scope"], root["availableZones"] = energyExplanationSchema, b.EnergyExplanation.Scope, b.EnergyExplanation.AvailableZones
	root["periods"] = periods(b.EnergyExplanation.Periods)
	zones := []any{}
	for _, z := range b.EnergyExplanation.ZoneResults {
		g := graph(z.Nodes, z.Links)
		g["scope"], g["summary"], g["periods"] = z.Scope, summary(z.Summary), periods(z.Periods)
		zones = append(zones, g)
	}
	root["zoneResults"] = zones
	return map[string]any{"energyExplanation": root, "energyExplanationSummary": summary(b.EnergyExplanationSummary)}
}

func epathSQLPoolBoundaryWireView(wire map[string]any, zone bool, period string) map[string]any {
	g := wire["energyExplanation"].(map[string]any)
	if zone {
		g = g["zoneResults"].([]any)[0].(map[string]any)
	}
	if period != "root" {
		for _, row := range g["periods"].([]any) {
			p := row.(map[string]any)
			if p["id"] == period {
				return p
			}
		}
		panic("missing hand wire period")
	}
	return g
}

func TestEnergyPathRealSQLPoolBoundaryOriginalWireRejectsEveryCache(t *testing.T) {
	native := epathSQLPoolBoundaryHandNative(t)
	cases := []struct {
		name                 string
		zone                 bool
		period, cache, field string
	}{
		{"root link", false, "root", "", ""}, {"annual alias link", false, "annual", "", ""}, {"monthly link", false, "M1", "", ""},
		{"Zone root link", true, "root", "", ""}, {"Zone annual alias link", true, "annual", "", ""}, {"Zone monthly link", true, "M1", "", ""},
		{"top cached ratio", false, "root", "bundle", "ratios"}, {"top legacy KPI", false, "root", "bundle", "derivedKpis"},
		{"annual alias cached KPI", false, "annual", "graph", "derivedKpis"}, {"monthly cached ratio", false, "M1", "graph", "ratios"},
		{"Zone cached legacy KPI", true, "root", "graph", "derivedKpis"}, {"Zone annual cached ratio", true, "annual", "graph", "ratios"},
		{"Zone monthly cached KPI", true, "M1", "graph", "derivedKpis"}, {"selected Zone top cache", true, "M1", "bundle", "derivedKpis"},
		{"selected monthly top cache", false, "M1", "bundle", "ratios"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire := epathSQLPoolBoundaryHandWire()
			period := tc.period
			if period == "root" {
				period = "annual"
			}
			scope, zone := "building", ""
			if tc.zone {
				scope, zone = "zone", "SPACE1-1"
			}
			proof := &epathSQLPoolBoundaryProof{Native: native, Scope: scope, Zone: zone, Period: period}
			graph := epathSQLPoolBoundaryWireView(wire, tc.zone, tc.period)
			if tc.cache != "" {
				var cache map[string]any
				if tc.cache == "bundle" {
					cache = wire["energyExplanationSummary"].(map[string]any)
					cache["scope"] = EnergyExplanationScope{Kind: scope, ZoneName: zone}
					cache["period"] = period
				} else {
					cache = graph["summary"].(map[string]any)
				}
				// Contradictory cooling service must not overrule exact legacy
				// heating identity or a heating denominator endpoint.
				bad := map[string]any{"id": "kpi.heating_cop", "kind": "kpi.cooling_cop", "serviceKind": "cooling", "value": 2.5}
				if tc.zone && period == "M1" {
					bad = map[string]any{"id": "opaque-cached-ratio", "serviceKind": "cooling", "denominatorLabel": "end_use.heating.space1-1", "value": 2.5}
				}
				cache[tc.field] = []any{bad}
			} else {
				suffix := "building"
				if tc.zone {
					suffix = "space1-1"
				}
				bad := map[string]any{"id": "raw-unproved-heating", "fromId": "load.heating." + suffix, "toId": "end_use.heating." + suffix, "relation": "load_to_end_use", "serviceKind": "cooling", "basis": "", "ratioKind": "", "ratio": 0, "fromValue": 100, "toValue": 40, "fromUnit": "kWh", "toUnit": "kWh", "period": period, "zoneName": zone}
				graph["links"] = []any{bad}
			}
			data, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			original := append([]byte(nil), data...)
			decoded, err := epathDecodeOriginalOracleCandidate(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if decoded.EnergyExplanation.sanitizedOnRead || decoded.EnergyExplanation.upgradedFromV1 || !bytes.Equal(data, original) {
				t.Fatal("raw candidate was repaired before denial review")
			}
			if err := epathSQLCheckPoolBoundary(decoded, proof); err == nil {
				t.Fatal("raw unsafe conversion/cache was discarded or escaped independent denial")
			}
		})
	}
}

func TestEnergyPathRealSQLPoolBoundaryOriginalWireKeepsSafeLegacyCaches(t *testing.T) {
	wire := epathSQLPoolBoundaryHandWire()
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := epathDecodeOriginalOracleCandidate(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.EnergyExplanationSummary.DerivedKPIs) != 1 || len(bundle.EnergyExplanation.Periods[0].Summary.DerivedKPIs) != 1 || len(bundle.EnergyExplanation.ZoneResults[0].Summary.DerivedKPIs) != 1 || len(bundle.EnergyExplanation.ZoneResults[0].Periods[0].Summary.DerivedKPIs) != 1 {
		t.Fatal("independent original-wire decoder lost legacy derivedKpis before audit")
	}
	native := epathSQLPoolBoundaryHandNative(t)
	for _, p := range []*epathSQLPoolBoundaryProof{{Native: native, Scope: "building", Period: "annual"}, {Native: native, Scope: "building", Period: "M1"}, {Native: native, Scope: "zone", Zone: "SPACE1-1", Period: "annual"}, {Native: native, Scope: "zone", Zone: "SPACE1-1", Period: "M1"}} {
		if err := epathSQLCheckPoolBoundary(bundle, p); err != nil {
			t.Fatal(err)
		}
	}
}
