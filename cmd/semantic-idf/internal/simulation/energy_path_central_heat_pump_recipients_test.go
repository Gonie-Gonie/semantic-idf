package simulation

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathCentralHeatPumpSourceLocalCompleteRecipients(t *testing.T) {
	doc := energyPathCentralHeatPumpOriginal(t)
	targets := energyPathCentralHeatPumpOutputTargets(doc)
	report := idf.AnalyzeHVAC(doc)
	paths := energyPathCentralHeatPumpBindRecipientPaths(doc, targets, report.ServiceModel.ZoneServices)
	if len(paths) != 2 {
		t.Fatalf("two separate service cohorts missing: %#v", paths)
	}
	seen := map[string]bool{}
	for _, target := range targets {
		if target.Definition.Role != energyPathCentralHeatPumpPurchased {
			continue
		}
		ids := paths[target.Definition.ID+":"+target.System.ID]
		if len(ids) != 5 {
			t.Fatalf("paid source does not own exactly five native recipients: %#v", ids)
		}
		for _, id := range ids {
			if id == "" || seen[id] {
				t.Fatal("Cooling/Heating pools reused one path")
			}
			seen[id] = true
		}
	}
	context := newEnergyDriverBuildContext(idf.GeometryReport{}, doc)
	if !reflect.DeepEqual(context.CentralHeatPumpPaths, paths) {
		t.Fatal("actual context omitted qualified source paths")
	}
	path, db, plan, hand, parents := centralHeatPumpMonthlyFixture(t)
	hand.CentralHeatPumpPaths = paths
	pools, sources := readEnergyPathCentralHeatPumpMonthlyPools(db, path, &plan, hand, parents)
	if len(pools) != 2 || len(sources) != 2 || sources[0].RawValue != 110 || sources[1].RawValue != 48 {
		t.Fatal("recipient attachment changed native budgets")
	}
	for _, pool := range pools {
		if !reflect.DeepEqual(pool.Members[0].RelatedPathIDs, paths[pool.Members[0].ID]) {
			t.Fatal("reader attached another source/service's paths")
		}
	}
	// Hand deletion exercises the downstream complete-roster guard: the other
	// service remains intact, and an unrelated boiler path is not a substitute.
	removed := false
	for i := range report.ServiceModel.ZoneServices {
		var keep []idf.ZoneServicePath
		for _, p := range report.ServiceModel.ZoneServices[i].Paths {
			if !removed && p.ServiceKind == "heating" && p.SourceSystem != nil && strings.HasPrefix(p.SourceSystem.Name, "CentralHeatPumpSystem ") {
				removed = true
				continue
			}
			keep = append(keep, p)
		}
		report.ServiceModel.ZoneServices[i].Paths = keep
	}
	if !removed {
		t.Fatal("missing hand deletion target")
	}
	partial := energyPathCentralHeatPumpBindRecipientPaths(doc, targets, report.ServiceModel.ZoneServices)
	if len(partial) != 1 || len(partial["central_heat_pump.cooling.electricity:component:269"]) != 5 {
		t.Fatalf("partial Heating cohort renormalized or erased Cooling: %#v", partial)
	}
}

func TestEnergyPathCentralHeatPumpThermalChannelsAreNotAdditiveAliases(t *testing.T) {
	for _, name := range []string{"Chiller Heater System Cooling Energy", "Chiller Heater System Heating Energy", "Chiller Heater System Source Heat Transfer Energy", "Chiller Heater System Cooling Rate", "Chiller Heater System Heating Rate", "Chiller Heater System Source Heat Transfer Rate"} {
		if _, ok := energyMeterAliasOrOtherDefinitionForName(name); ok {
			t.Fatalf("thermal context became purchased meter: %s", name)
		}
		if _, ok := energyVariableAliasDefinitionForName(name); ok {
			t.Fatalf("thermal context became paid variable: %s", name)
		}
		if _, ok := energyLoadAliasDefinitionForName(name); ok {
			t.Fatalf("plant transfer duplicated delivered Zone loads: %s", name)
		}
		if _, ok := energyHeatAliasDefinitionForName(name); ok {
			t.Fatalf("plant transfer became a Zone pressure: %s", name)
		}
	}
}
