package simulation

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// This fixture is a hand-defined boundary example, not a thermal/pool split:
// Heating has 20 gas + 10 electricity; Cooling 40 and Fans 5 are independent.
// The gas source services an unquantified non-Zone demand, so no efficiency or
// Zone-air delivery ratio is inferred from the combined Heating denominator.
func epathBoundaryLegacyFixture(gas float64, policy string) EnergyExplanationV1 {
	nodes := []EnergyExplanationNode{
		{ID: "energy.carrier.electricity", Level: "energy", Value: 55, Unit: "kWh", Carrier: "electricity", EndUse: "total"},
		{ID: "energy.carrier.gas", Level: "energy", Value: gas, Unit: "kWh", Carrier: "natural_gas", EndUse: "total"},
		{ID: "energy.heating.electricity", Level: "energy", Value: 10, Unit: "kWh", Carrier: "electricity", EndUse: "heating", SourceIDs: []string{"sql-rdd-301"}},
		{ID: "energy.heating.gas", Level: "energy", Value: gas, Unit: "kWh", Carrier: "natural_gas", EndUse: "heating", SourceIDs: []string{"sql-rdd-201"}, serviceBoundaryRestrictions: []energyPathServiceBoundaryRestriction{epathBoundaryTestRestriction()}},
		{ID: "energy.cooling", Level: "energy", Value: 40, Unit: "kWh", Carrier: "electricity", EndUse: "cooling", SourceIDs: []string{"sql-rdd-401"}},
		{ID: "energy.fans", Level: "energy", Value: 5, Unit: "kWh", Carrier: "electricity", EndUse: "fans", SourceIDs: []string{"sql-rdd-501"}},
		{ID: "load.heating.a", Level: "load", Value: 15, Unit: "kWh", ZoneName: "A", ServiceKind: "heating", SourceIDs: []string{"sql-rdd-601"}},
		{ID: "load.cooling.a", Level: "load", Value: 80, Unit: "kWh", ZoneName: "A", ServiceKind: "cooling", SourceIDs: []string{"sql-rdd-602"}},
	}
	edges := []EnergyExplanationEdge{
		{FromID: "energy.carrier.electricity", ToID: "energy.heating.electricity", Value: 10, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
		{FromID: "energy.carrier.gas", ToID: "energy.heating.gas", Value: gas, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
		{FromID: "energy.carrier.electricity", ToID: "energy.cooling", Value: 40, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
		{FromID: "energy.carrier.electricity", ToID: "energy.fans", Value: 5, Unit: "kWh", Relation: "meter_enduse", Basis: "measured_meter"},
		{FromID: "energy.heating.gas", ToID: "load.heating.a", Value: 15, Unit: "kWh", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "heating"},
		// Deliberately omit every restricted source from this competing link:
		// it still cannot certify the entire canonical Heating denominator.
		{FromID: "energy.heating.electricity", ToID: "load.heating.a", Value: 15, Unit: "kWh", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "heating", SourceIDs: []string{"sql-rdd-301"}},
		{FromID: "energy.cooling", ToID: "load.cooling.a", Value: 80, Unit: "kWh", Relation: "delivered_load", Basis: "measured_variable", ServiceKind: "cooling"},
	}
	return EnergyExplanationV1{Schema: energyExplanationV1Schema, AllocationPolicy: policy, Nodes: nodes, Edges: edges}
}

func epathBoundaryCheckGraph(t *testing.T, result EnergyExplanationResult, heating float64) {
	t.Helper()
	wantNodes := map[string]float64{"end_use.heating.building": heating, "end_use.cooling.building": 40, "end_use.fans.building": 5, "carrier.electricity.building": 55}
	for _, node := range result.Nodes {
		if want, exists := wantNodes[node.ID]; exists {
			if node.Value != want {
				t.Errorf("%s observed energy changed: got %g want %g", node.ID, node.Value, want)
			}
			delete(wantNodes, node.ID)
		}
		if node.ID == "end_use.heating.building" && !energyPathServiceBoundaryDeniesConversion(node) {
			t.Error("canonical Heating lost its explicit source boundary")
		}
		if node.ID == "end_use.cooling.building" && energyPathServiceBoundaryDeniesConversion(node) {
			t.Error("unrelated Cooling inherited Heating restriction")
		}
	}
	if len(wantNodes) != 0 {
		t.Fatalf("observed end uses/carriers disappeared: %v", wantNodes)
	}
	cooling, heatingCarrier := 0, 0.0
	for _, link := range result.Links {
		if link.Relation == "load_to_end_use" {
			if link.ToID == "end_use.heating.building" {
				t.Errorf("unsupported Heating service conversion survived: %+v", link)
			}
			if link.ToID == "end_use.cooling.building" {
				cooling++
				if link.FromValue != 80 || link.ToValue != 40 || link.Ratio != 2 {
					t.Errorf("independent Cooling conversion changed: %+v", link)
				}
			}
		}
		if link.FromID == "end_use.heating.building" && energyPathLinkIsCarrierSplit(link) {
			heatingCarrier += link.ToValue
		}
	}
	if cooling != 1 || heatingCarrier != heating {
		t.Errorf("Cooling conversion count=%d; Heating carrier subtotal=%g want %g", cooling, heatingCarrier, heating)
	}
	for _, summary := range []EnergyExplanationSummary{buildEnergyExplanationSummary(result), buildEnergyExplanationSummaryV2(result)} {
		for _, ratio := range append(summary.Ratios, summary.DerivedKPIs...) {
			if ratio.ServiceKind == "heating" || ratio.Kind == "kpi.heating_cop" {
				t.Errorf("summary fallback recreated unsupported Heating ratio: %+v", ratio)
			}
		}
	}
}

func TestEnergyPathServiceBoundaryUpgradePoliciesAndTwoReloads(t *testing.T) {
	for _, policy := range []string{PurposeAllocationPolicyDirectOnly, PurposeAllocationPolicyByServicePathLoadShare} {
		for _, gas := range []float64{20, 0} {
			for _, decodedUnknown := range []bool{false, true} {
				name := policy
				if gas == 0 {
					name += "/zero"
				} else {
					name += "/positive"
				}
				if decodedUnknown {
					name += "/stored_unknown_mask"
				}
				t.Run(name, func(t *testing.T) {
					legacy := epathBoundaryLegacyFixture(gas, policy)
					if decodedUnknown {
						legacy.Nodes[3].inspectorDecodedFromJSON = true
						legacy.Nodes[3].inspectorValuePresence = 0
					}
					result := UpgradeEnergyExplanationV1(legacy)
					for reload := 0; reload < 3; reload++ {
						epathBoundaryCheckGraph(t, result, gas+10)
						if reload == 2 {
							break
						}
						wire, err := json.Marshal(result)
						if err != nil {
							t.Fatal(err)
						}
						var next EnergyExplanationResult
						if err := json.Unmarshal(wire, &next); err != nil {
							t.Fatal(err)
						}
						result = next
					}
				})
			}
		}
	}
}

func TestEnergyPathServiceBoundaryStoredV1RetainsCanonicalRestriction(t *testing.T) {
	legacy := epathBoundaryLegacyFixture(20, PurposeAllocationPolicyDirectOnly)
	wire, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var reopened EnergyExplanationResult
	if err := json.Unmarshal(wire, &reopened); err != nil {
		t.Fatal(err)
	}
	if !reopened.upgradedFromV1 {
		t.Fatal("fixture missed stored-v1 upgrade boundary")
	}
	epathBoundaryCheckGraph(t, reopened, 30)
}

func TestEnergyPathServiceBoundaryStoredV1CannotRestoreGenericAllocation(t *testing.T) {
	for _, nativeIDs := range []bool{true, false} {
		legacy := epathBoundaryLegacyFixture(20, PurposeAllocationPolicyByServicePathLoadShare)
		if !nativeIDs {
			r := &legacy.Nodes[3].serviceBoundaryRestrictions[0]
			r.Reason = energyPathBoundaryTopologyIncomplete
			r.ConsumerSourceIDs, r.MeterSourceIDs = nil, nil
			legacy.Nodes[3].SourceIDs = []string{"tabular.heating.gas"}
		}
		legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(legacy)
		if len(legacy.buildingHVACAllocationEdges) == 0 {
			t.Fatal("fixture missed purpose preallocation sidecar")
		}
		wire, err := json.Marshal(legacy)
		if err != nil {
			t.Fatal(err)
		}
		var result EnergyExplanationResult
		if err := json.Unmarshal(wire, &result); err != nil {
			t.Fatal(err)
		}
		for reopen := 0; reopen < 3; reopen++ {
			if len(result.ZoneResults) == 0 {
				t.Fatal("fixture missed Zone projections")
			}
			for _, zone := range result.ZoneResults {
				for _, node := range zone.Nodes {
					if node.Level == "carrier" && node.Carrier == "natural_gas" && node.Value != 0 {
						t.Errorf("nativeIDs=%v reload=%d restored generic allocation of non-Zone gas: %+v", nativeIDs, reopen, node)
					}
				}
				for _, link := range zone.Links {
					if energyPathLinkIsCarrierSplit(link) && strings.HasPrefix(link.ToID, "carrier.natural_gas.") && link.ToValue != 0 {
						t.Error("private evidence loss restored restricted carrier allocation")
					}
				}
			}
			// Denial of Zone allocation never removes the observed Building gas.
			foundGas := false
			for _, node := range result.Nodes {
				if node.ID == "carrier.natural_gas.building" {
					foundGas = node.Value == 20
				}
			}
			if !foundGas {
				t.Fatal("deny-only fallback removed or invented observed Building gas")
			}
			if reopen == 2 {
				break
			}
			wire, err = json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var next EnergyExplanationResult
			if err := json.Unmarshal(wire, &next); err != nil {
				t.Fatal(err)
			}
			result = next
		}
	}
}

func TestEnergyPathServiceBoundaryTopologyOnlyNoNativeSource(t *testing.T) {
	legacy := epathBoundaryLegacyFixture(20, PurposeAllocationPolicyDirectOnly)
	r := epathBoundaryTestRestriction()
	r.Reason = energyPathBoundaryTopologyIncomplete
	r.ConsumerSourceIDs, r.MeterSourceIDs = nil, nil
	legacy.Nodes[3].serviceBoundaryRestrictions = []energyPathServiceBoundaryRestriction{r}
	legacy.Nodes[3].SourceIDs = []string{"tabular.heating.gas"}
	result := UpgradeEnergyExplanationV1(legacy)
	for reload := 0; reload < 3; reload++ {
		epathBoundaryCheckGraph(t, result, 30)
		for _, node := range result.Nodes {
			for _, restriction := range node.serviceBoundaryRestrictions {
				if len(restriction.ConsumerSourceIDs)+len(restriction.MeterSourceIDs) != 0 {
					t.Fatal("topology-only denial invented native source references")
				}
			}
		}
		if reload == 2 {
			break
		}
		wire, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var next EnergyExplanationResult
		if err := json.Unmarshal(wire, &next); err != nil {
			t.Fatal(err)
		}
		result = next
	}
}

func TestEnergyPathServiceBoundaryAnnualCensusKeepsPrunedMonth(t *testing.T) {
	legacy := epathBoundaryLegacyFixture(0, PurposeAllocationPolicyDirectOnly)
	// Annual input deliberately lacks the restriction, while M1 observes its
	// source at zero. M1 has no positive Heating end use; M2 supplies 10.
	legacy.Nodes[3].serviceBoundaryRestrictions = nil
	m1 := cloneEnergyExplanationNodes(legacy.Nodes)
	m1[2].Value = 0
	m1[3].serviceBoundaryRestrictions = []energyPathServiceBoundaryRestriction{epathBoundaryTestRestriction()}
	m2 := cloneEnergyExplanationNodes(legacy.Nodes)
	legacy.Periods = []EnergyPeriod{
		{ID: "M1", Kind: "monthly", Nodes: m1, Edges: legacy.Edges},
		{ID: "M2", Kind: "monthly", Nodes: m2, Edges: legacy.Edges},
		{ID: "annual", Kind: "annual", Nodes: cloneEnergyExplanationNodes(legacy.Nodes), Edges: legacy.Edges},
	}
	legacy.canonicalMonthlyBasis = true
	result := UpgradeEnergyExplanationV1(legacy)
	var annual *EnergyExplanationNode
	for index := range result.Nodes {
		if result.Nodes[index].ID == "end_use.heating.building" {
			annual = &result.Nodes[index]
		}
	}
	if annual == nil || annual.Value != 10 || !energyPathServiceBoundaryDeniesConversion(*annual) {
		t.Fatalf("pruned month's boundary or monthly numerical precedence lost: %+v", annual)
	}
	for _, link := range result.Links {
		if link.Relation == "load_to_end_use" && link.ToID == annual.ID {
			t.Fatal("annual ratio returned from an unrestricted positive month")
		}
	}
	for _, period := range result.Periods {
		if period.ID == "M1" {
			for _, node := range period.Nodes {
				if node.ID == annual.ID {
					t.Fatal("metadata preservation invented a visible zero Heating node")
				}
			}
		}
		if period.ID == "annual" {
			for _, node := range period.Nodes {
				if node.ID == annual.ID && (!energyPathServiceBoundaryDeniesConversion(node) || node.Value != 10) {
					t.Fatal("annual cached period diverged from canonical metadata/value")
				}
			}
		}
	}
}

func TestEnergyPathServiceBoundaryCanonicalMonthlySameIDDoesNotReplaceValues(t *testing.T) {
	annual := epathBoundaryTestNode("end_use.heating.building", "heating", 999, epathBoundaryTestRestriction())
	monthly := epathBoundaryTestNode(annual.ID, "heating", 10)
	result := EnergyExplanationResult{
		Scope:   EnergyExplanationScope{Kind: "building"},
		Nodes:   []EnergyExplanationNode{annual},
		Periods: []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: []EnergyExplanationNode{monthly}, Links: []EnergyPathLink{{ID: "old", Relation: "load_to_end_use", ToID: monthly.ID, FromValue: 5, ToValue: 10, FromUnit: "kWh", ToUnit: "kWh"}}}},
	}
	applyCanonicalMonthlyBasisToEnergyPathResult(&result)
	for _, node := range result.Nodes {
		if node.ID != monthly.ID {
			continue
		}
		if node.Value != 10 || node.RawValue != 10 || node.EffectiveValue != 10 || !reflect.DeepEqual(node.SourceIDs, monthly.SourceIDs) || !energyPathServiceBoundaryDeniesConversion(node) {
			t.Errorf("annual same-ID path changed quantities/provenance or dropped metadata: %+v", node)
		}
	}
	if len(result.Links) != 0 {
		t.Fatal("same-ID monthly conversion bypassed annual metadata")
	}
}

func TestEnergyPathServiceBoundaryScopeExclusionAndExactProjection(t *testing.T) {
	restricted := epathBoundaryTestNode("energy.heating.a", "heating", 20, epathBoundaryTestRestriction())
	restricted.Level, restricted.ZoneName, restricted.Carrier = "energy", "A", "natural_gas"
	other := epathBoundaryTestNode("energy.heating.b", "heating", 10)
	other.Level, other.ZoneName, other.Carrier = "energy", "B", "electricity"
	loads := []EnergyExplanationNode{{ID: "load.a", Level: "load", Value: 10, Unit: "kWh", ServiceKind: "heating", ZoneName: "A"}, {ID: "load.b", Level: "load", Value: 10, Unit: "kWh", ServiceKind: "heating", ZoneName: "B"}}
	nodes := append([]EnergyExplanationNode{restricted, other}, loads...)
	edges := []EnergyExplanationEdge{{FromID: restricted.ID, ToID: "load.a", Relation: "delivered_load", Unit: "kWh", Value: 10, ServiceKind: "heating"}, {FromID: other.ID, ToID: "load.b", Relation: "delivered_load", Unit: "kWh", Value: 10, ServiceKind: "heating"}}
	for _, policy := range []string{PurposeAllocationPolicyDirectOnly, PurposeAllocationPolicyByServicePathLoadShare} {
		scoped, links := upgradeEnergyExplanationGraph(nodes, edges, nil, EnergyExplanationScope{Kind: "zone", ZoneName: "B"}, policy, false, nil)
		conversionCount := 0
		for _, node := range scoped {
			if energyPathServiceBoundaryDeniesConversion(node) {
				t.Fatal("excluded Zone A boundary leaked into Zone B")
			}
		}
		for _, link := range links {
			if link.Relation == "load_to_end_use" {
				conversionCount++
			}
		}
		if conversionCount != 1 {
			t.Fatalf("unrelated exact Zone conversion count=%d", conversionCount)
		}
	}
	// The broad Pump meter includes both HW and CW. Only an independently
	// qualified CW consuming source may clear inherited HW metadata; its trace
	// still includes the shared meter, which is not positive disjoint proof.
	r := epathBoundaryTestRestriction()
	r.EndUse, r.Carrier = "pumps", "electricity"
	pump := epathBoundaryTestNode("energy.pumps", "pumps", 30, r)
	pump.Level, pump.Carrier = "energy", "electricity"
	pump.SourceIDs = []string{"sql-rdd-201"}
	pumpLoad := loads[1]
	pumpLoad.ID, pumpLoad.ServiceKind = "load.cooling.b", "cooling"
	pumpNodes := []EnergyExplanationNode{pump, pumpLoad}
	pumpEdges := []EnergyExplanationEdge{{FromID: pump.ID, ToID: pumpLoad.ID, Relation: energyPathAuxiliaryAllocationRelation, RuleID: energyRelationshipRuleAllocatedAuxiliaryServicePath, Basis: "service_path_allocation", ServiceKind: "cooling", Value: 4, SourceIDs: []string{"sql-rdd-201", "sql-rdd-102"}, serviceBoundaryConsumerSourceIDs: []string{"sql-rdd-102"}, serviceBoundaryExactConsumers: true}}
	for _, exact := range []bool{true, false} {
		pumpEdges[0].serviceBoundaryExactConsumers = exact
		projections := energyExplanationZoneAllocationProjections(pumpNodes, pumpEdges, EnergyExplanationScope{Kind: "zone", ZoneName: "B"}, PurposeAllocationPolicyByServicePathLoadShare)
		projected, ok := energyExplanationNodeForScope(pump, EnergyExplanationScope{Kind: "zone", ZoneName: "B"}, projections)
		if !ok || projected.Value != 4 || (len(projected.serviceBoundaryRestrictions) == 0) != exact {
			t.Errorf("exact=%v projection borrowed broad meter authority or changed allocation: %+v, included=%v", exact, projected, ok)
		}
	}
	pumpEdges[0].serviceBoundaryExactConsumers = true
	duplicates := append(append([]EnergyExplanationEdge(nil), pumpEdges...), pumpEdges[0])
	projections := energyExplanationZoneAllocationProjections(pumpNodes, duplicates, EnergyExplanationScope{Kind: "zone", ZoneName: "B"}, PurposeAllocationPolicyByServicePathLoadShare)
	projected, ok := energyExplanationNodeForScope(pump, EnergyExplanationScope{Kind: "zone", ZoneName: "B"}, projections)
	if !ok || projected.Value != 4 || len(projected.serviceBoundaryRestrictions) == 0 {
		t.Fatal("numeric duplicate suppression became source-cohort authority")
	}
	// A sibling source's share can remain an existing generic numerical
	// fallback, but it never certifies this node's consuming cohort.
	sibling := pump
	sibling.ID, sibling.serviceBoundaryRestrictions = "sibling.pump", nil
	borrowedEdges := append([]EnergyExplanationEdge(nil), pumpEdges...)
	borrowedEdges[0].FromID = sibling.ID
	projections = energyExplanationZoneAllocationProjections(append(pumpNodes, sibling), borrowedEdges, EnergyExplanationScope{Kind: "zone", ZoneName: "B"}, PurposeAllocationPolicyByServicePathLoadShare)
	projected, ok = energyExplanationNodeForScope(pump, EnergyExplanationScope{Kind: "zone", ZoneName: "B"}, projections)
	if !ok || len(projected.serviceBoundaryRestrictions) == 0 {
		t.Fatal("sibling fallback incorrectly certified exact consumer disjointness")
	}
}

func TestEnergyPathServiceBoundaryNodeWireKeepsLegacyQuantities(t *testing.T) {
	type plainNode EnergyExplanationNode
	for _, allocatedDriver := range []bool{false, true} {
		for _, known := range []bool{false, true} {
			node := EnergyExplanationNode{ID: "test", Level: "end_use", EndUse: "heating", Unit: "kWh", AllocationApplied: allocatedDriver, inspectorDecodedFromJSON: true}
			if allocatedDriver {
				node.Level = "driver"
			}
			if known {
				node.inspectorValuePresence = 7
			}
			legacyWire, err := json.Marshal(plainNode(node))
			if err != nil {
				t.Fatal(err)
			}
			if allocatedDriver {
				var raw, effective, allocated *float64
				if known {
					zero := 0.0
					raw, effective, allocated = &zero, &zero, &zero
				}
				legacyWire, err = json.Marshal(struct {
					plainNode
					RawValue       *float64 `json:"rawValue,omitempty"`
					EffectiveValue *float64 `json:"effectiveValue,omitempty"`
					AllocatedValue *float64 `json:"allocatedValue,omitempty"`
				}{plainNode(node), raw, effective, allocated})
				if err != nil {
					t.Fatal(err)
				}
			}
			withoutMetadata, err := json.Marshal(node)
			if err != nil || !bytes.Equal(legacyWire, withoutMetadata) {
				t.Fatalf("nil metadata changed historical Node writer: %s != %s (%v)", withoutMetadata, legacyWire, err)
			}
			node.serviceBoundaryRestrictions = []energyPathServiceBoundaryRestriction{epathBoundaryTestRestriction()}
			for reopen := 0; reopen < 2; reopen++ {
				wire, err := json.Marshal(node)
				if err != nil {
					t.Fatal(err)
				}
				var fields, oldFields map[string]json.RawMessage
				if err := json.Unmarshal(wire, &fields); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(legacyWire, &oldFields); err != nil {
					t.Fatal(err)
				}
				if len(fields["serviceBoundaryRestrictions"]) == 0 {
					t.Fatal("private metadata was not written")
				}
				delete(fields, "serviceBoundaryRestrictions")
				if !reflect.DeepEqual(fields, oldFields) {
					t.Fatal("adding metadata changed zero/missing quantity presence")
				}
				var next EnergyExplanationNode
				if err := json.Unmarshal(wire, &next); err != nil {
					t.Fatal(err)
				}
				if len(next.serviceBoundaryRestrictions) != 1 {
					t.Fatal("stored Node lost boundary metadata")
				}
				node = next
			}
		}
	}
}

func TestEnergyPathServiceBoundaryCachedSummariesAndStoredLinks(t *testing.T) {
	result := UpgradeEnergyExplanationV1(epathBoundaryLegacyFixture(20, PurposeAllocationPolicyDirectOnly))
	stale := buildEnergyExplanationSummary(result)
	stale.Ratios = append(stale.Ratios, EnergyExplanationSummaryItem{ID: "stale.heating", ServiceKind: "heating", Value: 0.5})
	stale.DerivedKPIs = []EnergyExplanationSummaryItem{{ID: "kpi.heating_cop", Value: 0.5}}
	period := EnergyPeriod{ID: "M1", Kind: "monthly", Nodes: cloneEnergyExplanationNodes(result.Nodes), Links: append([]EnergyPathLink(nil), result.Links...), Summary: &stale}
	period.Links = append(period.Links, EnergyPathLink{ID: "stale.conversion", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", FromValue: 15, ToValue: 30, FromUnit: "kWh", ToUnit: "kWh", ServiceKind: "heating", Ratio: 0.5, RatioKind: "efficiency"})
	result.Periods = []EnergyPeriod{period}
	selected := buildEnergyExplanationSummaryForPeriod(result, "M1")
	for _, ratio := range append(selected.Ratios, selected.DerivedKPIs...) {
		if ratio.ServiceKind == "heating" || ratio.ID == "kpi.heating_cop" {
			t.Fatal("cached-period summary bypassed boundary")
		}
	}
	if len(period.Summary.DerivedKPIs) != 1 {
		t.Fatal("reading cached summary mutated original cache")
	}
	// Result/period writers reject cached semantic links too, without changing
	// observed quantities. Incoming stored links are separately tested below.
	for pass := 0; pass < 2; pass++ {
		bundle := PurposeResultBundle{EnergyExplanation: result, EnergyExplanationSummary: stale}
		wire, err := json.Marshal(bundle)
		if err != nil {
			t.Fatal(err)
		}
		var reopened PurposeResultBundle
		if err := json.Unmarshal(wire, &reopened); err != nil {
			t.Fatal(err)
		}
		epathBoundaryCheckGraph(t, reopened.EnergyExplanation, 30)
		for _, ratio := range append(reopened.EnergyExplanationSummary.Ratios, reopened.EnergyExplanationSummary.DerivedKPIs...) {
			if ratio.ServiceKind == "heating" || ratio.ID == "kpi.heating_cop" {
				t.Fatal("root bundle cached summary recreated Heating ratio")
			}
		}
		for _, p := range reopened.EnergyExplanation.Periods {
			for _, link := range p.Links {
				if link.Relation == "load_to_end_use" && strings.Contains(link.ToID, "heating") {
					t.Fatal("stored period retained unsupported conversion")
				}
			}
		}
		result = reopened.EnergyExplanation
	}
	// Bypass the protected writer to model an older stale stored-v2 payload.
	type rawResult EnergyExplanationResult
	result.Links = append(result.Links, period.Links[len(period.Links)-1])
	wire, err := json.Marshal(rawResult(result))
	if err != nil {
		t.Fatal(err)
	}
	var reopened EnergyExplanationResult
	if err := json.Unmarshal(wire, &reopened); err != nil {
		t.Fatal(err)
	}
	epathBoundaryCheckGraph(t, reopened, 30)
	if !reopened.sanitizedOnRead {
		t.Fatal("stored unsafe conversion was not reported as sanitized")
	}
}
