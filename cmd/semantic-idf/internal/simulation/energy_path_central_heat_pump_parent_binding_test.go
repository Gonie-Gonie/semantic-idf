package simulation

import (
	"fmt"
	"path/filepath"
	"testing"
)

// Start at the native SQL parser, not a hand-written parent Basis. Typed
// recipient proof is separately tested; this fixture isolates its transport
// through the parser's native paid pools into the existing reservation stage.
func TestEnergyPathCentralHeatPumpParsedParentsReserveNativeConstituents(t *testing.T) {
	path, _, plan, context, _ := centralHeatPumpMonthlyFixture(t)
	context.CentralHeatPumpPaths = map[string][]string{}
	nodes := []EnergyExplanationNode{}
	topology := energyServicePathIndex{byZoneService: map[string][]string{}}
	for index, service := range []string{"cooling", "heating"} {
		paths := []string{service + ".Office", service + ".Lab"}
		for _, target := range context.CentralHeatPumpTargets {
			if target.Definition.Role == energyPathCentralHeatPumpPurchased && target.Definition.ServiceKind == service {
				context.CentralHeatPumpPaths[target.Definition.ID+":"+target.System.ID] = paths
			}
		}
		broad := 10.0
		if service == "heating" {
			broad = 4
		}
		nodes = append(nodes, epath100AuditEndUseNode(service, "electricity", broad, "M2", fmt.Sprintf("sql-rdd-%d", 30+index), paths))
		for _, zone := range []string{"Office", "Lab"} {
			pathID := service + "." + zone
			topology.byZoneService[normalizePurposeToken(zone)+"|"+service] = []string{pathID}
			nodes = append(nodes, epath100AuditLoadNode(zone, service, 1, "M2", "load."+service+"."+zone, []string{pathID}))
		}
	}
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.HVACConsumptionPools) != 2 {
		t.Fatalf("native paid pool census changed: %#v", parsed.HVACConsumptionPools)
	}
	allocation := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, nil, "M2", "monthly", true, topology)
	allocation = reserveEnergyPathHVACConsumptionPools(allocation, nodes, topology, parsed.HVACConsumptionPools, "M2", "monthly", true)
	if len(allocation.ConsumptionSourceAllocations) != 4 || len(allocation.Edges) != 4 || len(allocation.Records) != 2 {
		t.Fatalf("native pool binding fell back to broad allocation: %#v", allocation)
	}
	for _, row := range allocation.ConsumptionSourceAllocations {
		want, source := 5.0, "sql-rdd-10"
		if row.ServiceKind == "heating" {
			want, source = 2, "sql-rdd-11"
		} else if row.ServiceKind != "cooling" {
			t.Fatalf("unexpected service: %#v", row)
		}
		if row.AllocatedValue != want || row.ObservedValue != 2*want || len(row.SourceIDs) != 1 || row.SourceIDs[0] != source || (row.ZoneName != "Office" && row.ZoneName != "Lab") {
			t.Fatalf("paid service source/quantity changed: %#v", row)
		}
	}
	for _, edge := range allocation.Edges {
		parent, source, foreign := "sql-rdd-30", "sql-rdd-10", "sql-rdd-11"
		if edge.ServiceKind == "heating" {
			parent, source, foreign = "sql-rdd-31", "sql-rdd-11", "sql-rdd-10"
		}
		if edge.RuleID != energyRelationshipRuleAllocatedHVACConsumptionPool || !stringSliceContains(edge.SourceIDs, parent) || !stringSliceContains(edge.SourceIDs, source) || stringSliceContains(edge.SourceIDs, foreign) {
			t.Fatalf("edge lost its own native paid source or borrowed its sibling: %#v", edge)
		}
	}
	for _, row := range allocation.Records {
		if row.DirectValue != 0 || row.AllocatedValue != row.ExpectedValue || row.UnassignedValue != 0 || row.OvermappedValue != 0 {
			t.Fatalf("paid pool changed the native parent budget: %#v", row)
		}
	}
}

func TestEnergyPathCentralHeatPumpParentRejectsNonMeterAndForeignSeries(t *testing.T) {
	path, db, plan, context, parents := centralHeatPumpMonthlyFixture(t)
	for _, test := range []struct {
		name string
		edit func(*energyExplanationSeries)
	}{
		{"obsolete fabricated basis", func(s *energyExplanationSeries) { s.Basis = "reported_meter" }},
		{"energy variable", func(s *energyExplanationSeries) { s.Basis = "measured_energy_variable" }},
		{"tabular substitute", func(s *energyExplanationSeries) { s.Basis = "sql_tabular" }},
		{"Zone owner", func(s *energyExplanationSeries) { s.ZoneName = "Office" }},
		{"direct component", func(s *energyExplanationSeries) { s.directComponentID = "local" }},
		{"carrier", func(s *energyExplanationSeries) { s.Carrier = "natural_gas" }},
		{"source name", func(s *energyExplanationSeries) {
			s.SourceName, s.sourceName = "Electricity:Facility", "Electricity:Facility"
		}},
		{"context stage", func(s *energyExplanationSeries) { s.Stage = "context" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := append([]energyExplanationSeries(nil), parents...)
			test.edit(&changed[1])
			pools, _ := readEnergyPathCentralHeatPumpMonthlyPools(db, filepath.Base(path), &plan, context, changed)
			if len(pools) != 2 || len(pools[0].MeterSourceIDs) != 1 || pools[0].MeterSourceIDs[0] != "sql-rdd-30" || len(pools[1].MeterSourceIDs) != 0 {
				t.Fatalf("non-parent series authorized a pool or changed Cooling: %#v", pools)
			}
		})
	}
}
