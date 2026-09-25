package simulation

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func energyPathCentralHeatPumpOriginal(t *testing.T) idf.Document {
	t.Helper()
	path := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "CentralChillerHeaterSystem_Simultaneous_Cooling_Heating.idf")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if hash := fmt.Sprintf("%x", sha256.Sum256(raw)); hash != "84d389f1c40ab098eea05105664dd71d54c58509a4d812bf6d5dfe4e933c28f0" {
		t.Fatalf("original hash changed: %s", hash)
	}
	doc, err := idf.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestEnergyPathCentralHeatPumpExactSystemDefinitions(t *testing.T) {
	targets := energyPathCentralHeatPumpOutputTargets(energyPathCentralHeatPumpOriginal(t))
	if len(targets) != 5 {
		t.Fatalf("want5 system definition pairs, not module-multiplied targets: %+v", targets)
	}
	seen := map[string]bool{}
	purchased := 0
	for _, target := range targets {
		definition := target.Definition
		if target.System.ObjectIndex != 269 || target.System.ObjectName != "ChillerBank" || seen[definition.ID] {
			t.Fatalf("wrong original/duplicate definition: %+v", target)
		}
		seen[definition.ID] = true
		if !target.Binding.PortBindingsComplete || len(target.Binding.Modules) != 1 || target.Binding.Modules[0].Count != 3 {
			t.Fatalf("typed original lost native identity: %+v", target)
		}
		if definition.Role == energyPathCentralHeatPumpPurchased {
			purchased++
			if definition.Carrier != "electricity" || definition.EndUse != definition.ServiceKind || (definition.ServiceKind != "cooling" && definition.ServiceKind != "heating") {
				t.Fatalf("incorrect purchased classification: %+v", definition)
			}
			if !energyPathCentralHeatPumpMonthlyBudgetIdentity(target, definition.EnergyName, "CHILLERBANK", "J", "Monthly", false) {
				t.Fatal("exact Monthly native energy identity rejected")
			}
		} else {
			if definition.ServiceKind != "" || definition.EndUse != "" || definition.Carrier != "" || energyPathCentralHeatPumpMonthlyBudgetIdentity(target, definition.EnergyName, "CHILLERBANK", "J", "Monthly", false) {
				t.Fatalf("water/source transfer promoted to purchased energy: %+v", definition)
			}
		}
		for _, name := range []string{definition.RateName, "Chiller Heater Cooling Electricity Energy Unit 1", "Chiller Heater Heating Electricity Energy Unit 2", "Chiller Heater System Total Electricity Energy"} {
			if energyPathCentralHeatPumpMonthlyBudgetIdentity(target, name, "CHILLERBANK", "J", "Monthly", false) {
				t.Fatalf("module/rate/invented aggregate borrowed budget name: %s", name)
			}
		}
		for _, bad := range []struct {
			key, unit, frequency string
			meter                bool
		}{
			{"CHILLERHEATERMODULE", "J", "Monthly", false}, {"", "J", "Monthly", false}, {"CHILLERBANK", "W", "Monthly", false},
			{"CHILLERBANK", "kWh", "Monthly", false}, {"CHILLERBANK", "J", "Hourly", false}, {"CHILLERBANK", "J", "Zone Timestep", false}, {"CHILLERBANK", "J", "Monthly", true},
		} {
			if energyPathCentralHeatPumpMonthlyBudgetIdentity(target, definition.EnergyName, bad.key, bad.unit, bad.frequency, bad.meter) {
				t.Fatalf("noncanonical native dictionary became budget: %+v", bad)
			}
		}
	}
	if purchased != 2 {
		t.Fatalf("exact two end-use electricity partitions required, got%d", purchased)
	}
	for oi := range targets {
		for oj := range targets {
			if oi != oj && energyPathCentralHeatPumpMonthlyBudgetIdentity(targets[oi], targets[oj].Definition.EnergyName, "CHILLERBANK", "J", "Monthly", false) {
				t.Fatal("one system reporting key collapsed distinct source names")
			}
		}
	}
}

func TestEnergyPathCentralHeatPumpBrokenRouteDoesNotEraseOutputIdentity(t *testing.T) {
	doc := energyPathCentralHeatPumpOriginal(t)
	for oi := range doc.Objects {
		if strings.EqualFold(doc.Objects[oi].Type, "Branch") && doc.Objects[oi].Fields[0].Value == "Hot Water Branch" {
			doc.Objects[oi].Fields[5].Value = "Disconnected"
		}
	}
	targets := energyPathCentralHeatPumpOutputTargets(doc)
	if len(targets) != 5 {
		t.Fatal("unresolved allocation must retain the five independently named system observations")
	}
	for _, target := range targets {
		if target.Binding.PortBindingsComplete {
			t.Fatal("broken water route claimed eligibility")
		}
		if target.Definition.Role == energyPathCentralHeatPumpPurchased && !energyPathCentralHeatPumpMonthlyBudgetIdentity(target, target.Definition.EnergyName, "CHILLERBANK", "J", "Monthly", false) {
			t.Fatal("source identity erased by route failure")
		}
		if _, ok := idf.NativeCentralHeatPumpServicePort(target.Binding, target.Definition.ServiceKind, "PlantLoop", "Hot Water Loop"); ok {
			t.Fatal("unresolved source acquired a service path")
		}
	}
}

// Independent hand quantities exercise the existing source-pool reservation,
// not an observed physical fixture or a native-source reader acceptance. The
// two canonical budgets are 10 cooling + 4 heating = 14; not 28 or 3*14.
// Extra broad-meter amounts remain unassigned. Missing Heating is not borrowed
// from Cooling, and observed zero keeps rows while missing does not.
func TestEnergyPathCentralHeatPumpHandServiceBudgetsStayDistinct(t *testing.T) {
	for _, scenario := range []string{"known", "zero heating", "missing heating"} {
		t.Run(scenario, func(t *testing.T) {
			nodes := []EnergyExplanationNode{}
			topology := energyServicePathIndex{byZoneService: map[string][]string{}}
			pools := []energyPathHVACConsumptionPool{}
			for _, service := range []string{"cooling", "heating"} {
				broad, observed := 20.0, 10.0
				if service == "heating" {
					broad, observed = 10, 4
				}
				meter, source := "sql.meter."+service, "sql.system."+service
				paths := []string{service + ".Office", service + ".Lab"}
				nodes = append(nodes, epath100AuditEndUseNode(service, "electricity", broad, "M1", meter, paths))
				for _, zone := range []string{"Office", "Lab"} {
					path := service + "." + zone
					topology.byZoneService[normalizePurposeToken(zone)+"|"+service] = []string{path}
					nodes = append(nodes, epath100AuditLoadNode(zone, service, 1, "M1", "sql.load."+service+"."+zone, []string{path}))
				}
				nodes = append(nodes, epath100AuditLoadNode("PLENUM", service, 1000, "M1", "sql.load."+service+".PLENUM", nil))
				if service == "heating" && scenario == "zero heating" {
					observed = 0
				}
				series := energyExplanationSeries{Stage: "end_use", Level: "energy", Kind: "energy." + service, Unit: "kWh", ServiceKind: service, EndUse: service, Carrier: "electricity", SourceIDs: []string{source}, MonthlySourceIDs: []string{source}, Monthly: map[int]float64{1: observed}, RawMonthly: map[int]float64{1: observed}, Total: observed, RawTotal: observed, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", multiplierApplied: true, sourceFrequency: "Monthly"}
				if service == "heating" && scenario == "missing heating" {
					series.Monthly = nil
					series.RawMonthly = nil
					series.Total = 0
					series.RawTotal = 0
				}
				output := "Chiller Heater System Cooling Electricity Energy"
				if service == "heating" {
					output = "Chiller Heater System Heating Electricity Energy"
				}
				pools = append(pools, energyPathHVACConsumptionPool{ID: "native.central." + service + ".electricity", ServiceKind: service, Carrier: "electricity", MeterSourceIDs: []string{meter}, Valid: true, Members: []energyPathHVACConsumptionMember{{ID: "central." + service + ":component:269", ObjectType: "CentralHeatPumpSystem", ObjectName: "ChillerBank", OutputName: output, RelatedPathIDs: paths, Series: series}}})
			}
			plan := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, nil, "M1", "monthly", true, topology)
			plan = reserveEnergyPathHVACConsumptionPools(plan, nodes, topology, pools, "M1", "monthly", true)
			if len(plan.Records) != 2 {
				t.Fatalf("distinct services collapsed into one system key: %+v", plan.Records)
			}
			allocated := 0.0
			seenServices := map[string]bool{}
			for _, row := range plan.Records {
				if (row.ServiceKind != "cooling" && row.ServiceKind != "heating") || seenServices[row.ServiceKind] || row.Carrier != "electricity" {
					t.Fatalf("incorrect service/carrier census: %+v", row)
				}
				seenServices[row.ServiceKind] = true
				broad, want := 20.0, 10.0
				if row.ServiceKind == "heating" {
					broad, want = 10, 4
					if scenario != "known" {
						want = 0
					}
				}
				if row.ExpectedValue != broad || row.DirectValue != 0 || row.AllocatedValue != want || row.UnassignedValue != broad-want || row.OvermappedValue != 0 {
					t.Fatalf("source budget changed: %+v wantallocated%g", row, want)
				}
				allocated += row.AllocatedValue
			}
			wantTotal := 14.0
			if scenario != "known" {
				wantTotal = 10
			}
			if allocated != wantTotal {
				t.Fatalf("double-counted channel/module budget: %g", allocated)
			}
			wantRows := 4
			if scenario == "missing heating" {
				wantRows = 2
			}
			if len(plan.ConsumptionSourceAllocations) != wantRows {
				t.Fatalf("known zero confused with missing: %+v", plan.ConsumptionSourceAllocations)
			}
			for _, row := range plan.ConsumptionSourceAllocations {
				want := 10.0
				if row.ServiceKind == "heating" {
					want = 4
					if scenario == "zero heating" {
						want = 0
					}
				}
				if (row.ZoneName != "Office" && row.ZoneName != "Lab") || row.ObservedValue != want || row.AllocatedValue != want/2 || len(row.SourceIDs) != 1 || row.SourceIDs[0] != "sql.system."+row.ServiceKind {
					t.Fatalf("native source factor/knownness/path changed: %+v", row)
				}
			}
			wantEdges := 4
			if scenario != "known" {
				wantEdges = 2
			}
			if len(plan.Edges) != wantEdges {
				t.Fatalf("wrong positive-only edge census: %+v", plan.Edges)
			}
			for _, edge := range plan.Edges {
				if edge.ZoneName == "PLENUM" {
					t.Fatal("positive nonrecipient plenum load acquired native source consumption")
				}
				for _, service := range []string{"cooling", "heating"} {
					if strings.Contains(edge.FromID, service) && stringSliceContains(edge.SourceIDs, "sql.system."+map[string]string{"cooling": "heating", "heating": "cooling"}[service]) {
						t.Fatalf("cross-service source provenance: %+v", edge)
					}
				}
			}
		})
	}
}
