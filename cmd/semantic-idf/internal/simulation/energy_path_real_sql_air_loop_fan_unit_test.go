package simulation

import (
	"os"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathSQLAirLoopFanOriginal(t *testing.T) {
	raw, err := os.ReadFile("testdata/energy_path_real_models/models/25.1/5ZoneElectricBaseboard.idf")
	if err != nil {
		t.Fatal(err)
	}
	declaration := epathRealSQLAirLoopFan{SiteID: "fans.electricity", ObjectType: "Fan:VariableVolume", ObjectName: "Supply Fan 1", AirLoopName: "VAV Sys 1", ServedZones: []string{"SPACE1-1", "SPACE2-1", "SPACE3-1", "SPACE4-1", "SPACE5-1"}}
	parse := func() idf.Document {
		t.Helper()
		doc, err := idf.Parse(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	if err := epathSQLAirLoopFanOriginal(parse(), declaration); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*idf.Document){
		"additional local fan": func(doc *idf.Document) {
			doc.Objects = append(doc.Objects, idf.Object{Type: "Fan:OnOff", Fields: []idf.Field{{Value: "Other Fan"}}})
		},
		"same-key different fan type": func(doc *idf.Document) {
			doc.Objects = append(doc.Objects, idf.Object{Type: "Fan:OnOff", Fields: []idf.Field{{Value: "Supply Fan 1"}}})
		},
		"native fan inlet": func(doc *idf.Document) {
			for i := range doc.Objects {
				if doc.Objects[i].Type == "Fan:VariableVolume" {
					doc.Objects[i].Fields[15].Value = "Disconnected"
				}
			}
		},
		"native fan outlet": func(doc *idf.Document) {
			for i := range doc.Objects {
				if doc.Objects[i].Type == "Fan:VariableVolume" {
					doc.Objects[i].Fields[16].Value = "Disconnected"
				}
			}
		},
		"branch fan port": func(doc *idf.Document) {
			for i := range doc.Objects {
				if doc.Objects[i].Type == "Branch" && epathSQLBaseboardField(doc.Objects[i], 0) == "VAV Sys 1 Main Branch" {
					doc.Objects[i].Fields[len(doc.Objects[i].Fields)-1].Value = "Disconnected"
				}
			}
		},
		"air loop outer port": func(doc *idf.Document) {
			for i := range doc.Objects {
				if doc.Objects[i].Type == "AirLoopHVAC" {
					doc.Objects[i].Fields[9].Value = "Disconnected"
				}
			}
		},
		"air loop supply inlet": func(doc *idf.Document) {
			for i := range doc.Objects {
				if doc.Objects[i].Type == "AirLoopHVAC" {
					doc.Objects[i].Fields[6].Value = "Disconnected"
				}
			}
		},
		"duplicate air loop": func(doc *idf.Document) {
			for _, o := range doc.Objects {
				if o.Type == "AirLoopHVAC" {
					doc.Objects = append(doc.Objects, o)
					break
				}
			}
		},
		"shared branch": func(doc *idf.Document) {
			doc.Objects = append(doc.Objects, idf.Object{Type: "BranchList", Fields: []idf.Field{{Value: "Other BranchList"}, {Value: "VAV Sys 1 Main Branch"}}})
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			doc := parse()
			mutate(&doc)
			if err := epathSQLAirLoopFanOriginal(doc, declaration); err == nil {
				t.Fatal("invalid original fan graph accepted")
			}
		})
	}
	for _, zones := range [][]string{{"PLENUM-1", "SPACE2-1", "SPACE3-1", "SPACE4-1", "SPACE5-1"}, {"SPACE1-1", "SPACE2-1", "SPACE3-1", "SPACE4-1"}, {"SPACE1-1", "SPACE1-1"}} {
		bad := declaration
		bad.ServedZones = zones
		if err := epathSQLAirLoopFanOriginal(parse(), bad); err == nil {
			t.Fatal("incorrect fan served roster accepted")
		}
	}
}

func TestEnergyPathSQLAirLoopFanMonthlyMath(t *testing.T) {
	frames, model := epathSQLAuxiliaryZoneUnitFrames()
	site := model.Site[0]
	site.ID = "fans.electricity"
	site.EndUse = "fans"
	site.Source.Alternatives[0].Name = "Fans:Electricity"
	model.Site = []epathRealSQLSite{site}
	frames.Site[site.ID] = frames.Site["pumps.electricity"]
	delete(frames.Site, "pumps.electricity")
	frames.SiteSources[site.ID] = frames.SiteSources["pumps.electricity"]
	delete(frames.SiteSources, "pumps.electricity")
	for id, source := range frames.SourceIdentities {
		if source.Name == "Pumps:Electricity" {
			source.Name = "Fans:Electricity"
			frames.SourceIdentities[id] = source
		}
	}
	aux := model.Auxiliaries[0]
	aux.SiteID = site.ID
	aux.AllocationMethod = "air_loop_load_share"
	aux.ReconciliationID = "allocation.fans.annual"
	model.Auxiliaries = []epathRealSQLAuxiliary{aux}
	owner := epathRealSQLAirLoopFan{SiteID: site.ID, ObjectType: "Fan:VariableVolume", ObjectName: "Fan", AirLoopName: "Loop", ServedZones: []string{"A", "B"}}
	model.AirLoopFans = []epathRealSQLAirLoopFan{owner}
	frames.AirLoopFans = map[string]epathRealSQLAirLoopFan{site.ID: owner}
	proofs, err := epathSQLAuxiliaryZoneProofs(frames, model)
	if err != nil {
		t.Fatal(err)
	}
	for zone, want := range map[string]float64{"A": 22, "B": 12, "Plenum": 0} {
		proof := proofs[epathSQLAuxiliaryZoneKey(site.ID, zone, "annual")]
		if proof == nil || proof.Value.Value != want {
			t.Fatalf("%s annual fan share: got %#v want %g", zone, proof, want)
		}
		if proof.Basis != "service_path_allocation" {
			t.Fatal("estimated fan share became measured")
		}
		for _, source := range proof.Sources {
			if source.RDD == nil || source.RDD.Name != "Fans:Electricity" || !source.RDD.IsMeter || source.Tabular != nil || source.DirectHVAC != nil {
				t.Fatal("native fan source invented for broad fan meter")
			}
		}
	}
	delete(frames.AirLoopFans, site.ID)
	if _, err := epathSQLAuxiliaryZoneProofs(frames, model); err == nil {
		t.Fatal("unbound fan allocation accepted")
	}
	frames.AirLoopFans = map[string]epathRealSQLAirLoopFan{site.ID: owner}
	changed := owner
	changed.ServedZones = []string{"A", "Plenum"}
	frames.AirLoopFans[site.ID] = changed
	if _, err := epathSQLAuxiliaryZoneProofs(frames, model); err == nil {
		t.Fatal("changed original fan owner accepted")
	}
	if strings.Contains(model.Auxiliaries[0].AllocationMethod, "airflow") {
		t.Fatal("thermal load weight mislabeled as measured airflow")
	}
}
