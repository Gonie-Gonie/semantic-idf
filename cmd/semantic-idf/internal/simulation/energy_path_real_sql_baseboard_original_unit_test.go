package simulation

import (
	"database/sql"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLBaseboardUnitDeclaration() epathRealSQLDirectHVACComponent {
	return epathRealSQLDirectHVACComponent{ID: "heating.baseboard.electricity", Service: "heating", Carrier: "electricity", SiteID: "electricHeating", Frequency: "Monthly", AggregationBasis: "model_total",
		Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Baseboard Electricity Energy", Unit: "J"}}, Keys: []string{"Baseboard"}}, Owners: []epathRealSQLDirectHVACOwner{epathSQLBaseboardOwner("Office", "Baseboard")}}
}

func TestEnergyPathRealSQLBaseboardIndependentNativeConsumptionSources(t *testing.T) {
	for _, tc := range []struct {
		name, mutation string
		bad            bool
	}{
		{"complete native zero", "", false},
		{"missing Monthly observation", `DELETE FROM ReportData WHERE TimeIndex=1`, true},
		{"NULL observation", `UPDATE ReportData SET Value=NULL WHERE TimeIndex=1`, true},
		{"dictionary duplicate without observations", `INSERT INTO ReportDataDictionary VALUES(41,'Baseboard','Baseboard Electricity Energy','J',0,'Monthly','HVAC')`, true},
		{"crossed native key", `UPDATE ReportDataDictionary SET KeyValue='Other Baseboard' WHERE ReportDataDictionaryIndex=40`, true},
		{"wrong native unit", `UPDATE ReportDataDictionary SET Units='W' WHERE ReportDataDictionaryIndex=40`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, _, _, _ := epathSQLBaseboardContextUnit(t, "Baseboard Total Heating Energy", "Monthly", func(int) float64 { return 0 })
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE ReportDataDictionary SET Name='Baseboard Electricity Energy'`); err != nil {
				t.Fatal(err)
			}
			if tc.mutation != "" {
				if _, err := db.Exec(tc.mutation); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			observed, err := epathReadRealSQLOracle(path)
			if err != nil {
				if tc.bad {
					return
				}
				t.Fatal(err)
			}
			model := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}, DirectHVACComponents: []epathRealSQLDirectHVACComponent{epathSQLBaseboardUnitDeclaration()}, Site: []epathRealSQLSite{{ID: "electricHeating", EndUse: "heating", Carrier: "electricity"}}}
			frames := epathSQLBaseboardContextUnitFrames()
			frames.Site = map[string][]*epathSQLQuantity{"electricHeating": make([]*epathSQLQuantity, 12)}
			for month := range frames.Site["electricHeating"] {
				frames.Site["electricHeating"][month] = &epathSQLQuantity{Value: 4}
			}
			err = epathCompileSQLDirectHVACSources(path, observed.Sources, model, &frames)
			if tc.bad {
				if err == nil || len(frames.DirectHVAC) != 0 || len(frames.DirectHVACSourceIdentities) != 0 {
					t.Fatal("invalid native source committed direct proof")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			identity := frames.DirectHVACSourceIdentities[40]
			if len(frames.DirectHVAC) != 12 || identity.FamilyID != "heating.baseboard.electricity" || identity.Owner.EquipmentName != "Baseboard" {
				t.Fatal("native source did not reuse exact independent direct identity")
			}
			for month := 1; month <= 12; month++ {
				q := frames.DirectHVAC[epathSQLDirectHVACKey("Office", "heating", "electricity", month)]
				if !q.Present || q.Quantity.Value != 0 || q.Quantity.Error != 0 || len(q.SourceIDs) != 1 || q.SourceIDs[0] != 40 {
					t.Fatal("native known zero became missing or was multiplied")
				}
			}
		})
	}
}

func TestEnergyPathRealSQLBaseboardIndependentOriginalSelfOwner(t *testing.T) {
	doc := energyPathBaseboardHand(t)
	declaration := epathSQLBaseboardUnitDeclaration()
	model := epathRealSQLModel{DirectHVACComponents: []epathRealSQLDirectHVACComponent{declaration}}
	if err := epathSQLValidateDirectHVACOriginalModel(doc.String(), model); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *idf.Document)
	}{
		{"unsupported same reporting key", func(t *testing.T, d *idf.Document) {
			p := *directHVACFixtureObject(t, d, epathSQLNativeBaseboardType, "Baseboard")
			p.Type = "ZoneHVAC:Baseboard:Convective:Electric"
			energyPathBaseboardDuplicate(d, p)
		}},
		{"original duplicate parent", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, epathSQLNativeBaseboardType, "Baseboard"))
		}},
		{"disconnected second owner", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "List"))
			d.Objects[len(d.Objects)-1].Fields[0].Value = "Unconnected List"
		}},
		{"same name wrong list slot", func(t *testing.T, d *idf.Document) {
			p := directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentList", "List")
			p.Fields = append(p.Fields[:2], append([]idf.Field{{Value: "unexpected"}}, p.Fields[2:]...)...)
		}},
		{"Zone connection reassigned", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentConnections", "Office").Fields[0].Value = "Other"
		}},
		{"duplicate Zone connection", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, *directHVACFixtureObject(t, d, "ZoneHVAC:EquipmentConnections", "Office"))
		}},
		{"recipient crosses Zone", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, "BuildingSurface:Detailed", "Floor").Fields[3].Value = "Other"
		}},
		{"fraction is NaN", func(t *testing.T, d *idf.Document) {
			directHVACFixtureObject(t, d, epathSQLNativeBaseboardType, "Baseboard").Fields[10].Value = "NaN"
		}},
		{"foreign typed wrapper", func(t *testing.T, d *idf.Document) {
			energyPathBaseboardDuplicate(d, idf.Object{Type: "Unsupported:Wrapper", Fields: []idf.Field{{Value: "Wrapper"}, {Value: epathSQLNativeBaseboardType}, {Value: "Baseboard"}}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := energyPathBaseboardHand(t)
			tc.mutate(t, &copy)
			if err := epathSQLValidateDirectHVACOriginalModel(copy.String(), model); err == nil {
				t.Fatal("independent oracle accepted contradictory original ownership")
			}
		})
	}
	bad := declaration
	bad.Owners = append([]epathRealSQLDirectHVACOwner(nil), declaration.Owners...)
	bad.Owners[0].EquipmentName = "Packaged Parent"
	if _, err := epathSQLDirectHVACOwners(bad); err == nil {
		t.Fatal("native electricity borrowed a fictitious parent")
	}
	if got := epathSQLDirectHVACParentFamilies(epathSQLNativeBaseboardType); len(got) != 1 || got[0] != "heating.baseboard.electricity" {
		t.Fatal("baseboard native constituent roster is not exact")
	}
	if len(epathSQLDirectHVACParentFamilies("ZoneHVAC:PackagedTerminalAirConditioner")) != 5 || len(epathSQLDirectHVACParentFamilies("ZoneHVAC:PackagedTerminalHeatPump")) != 7 {
		t.Fatal("existing packaged oracle constituent rosters changed")
	}
	for _, owner := range []epathRealSQLDirectHVACOwner{epathSQLBaseboardOwner("SPACE2-1", "SPACE2-1 Baseboard"), epathSQLBaseboardOwner("SPACE4-1", "SPACE4-1 Baseboard")} {
		if err := epathSQLValidateNativeBaseboardOriginalOwner(energyPathBaseboardOriginal(t), owner); err != nil {
			t.Fatal(err)
		}
	}
}
