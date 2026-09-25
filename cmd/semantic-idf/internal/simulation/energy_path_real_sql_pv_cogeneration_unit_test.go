package simulation

// Literal test observations, in one SQLite transaction via C's existing
// seeder. No engine or candidate quantities; no preserved evidence is edited.
import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func epathSQLPVCogenerationHandMTD(proof epathSQLPVOriginalProof) string {
	d := proof.Declaration
	var text strings.Builder
	fmt.Fprintf(&text, "For Meter=Electricity:Facility [J], ResourceType=Electricity, contents are:\n  ZN_1_FLR_1_SEC_1_LIGHTS:Lights Electricity Energy\n  %s:Inverter Ancillary AC Electricity Energy\n\n", d.InverterName)
	fmt.Fprintln(&text, "For Meter=ElectricityProduced:Facility [J], ResourceType=ElectricityProduced, contents are:")
	fmt.Fprintf(&text, "  %s:Inverter Conversion Loss Decrement Energy\n  %s:Electric Storage Production Decrement Energy\n  %s:Electric Storage Discharge Energy\n", d.InverterName, d.StorageName, d.StorageName)
	for _, pv := range d.Generators {
		fmt.Fprintf(&text, "  %s:Generator Produced DC Electricity Energy\n", pv.Name)
	}
	fmt.Fprintf(&text, "\nFor Meter=Cogeneration:Electricity [J], ResourceType=Electricity, EndUse=Cogeneration, contents are:\n  %s:Inverter Ancillary AC Electricity Energy\n\n", d.InverterName)
	return text.String()
}

func epathSQLPVBalanceHandLiterals() []epathSQLPVSourceLiteral {
	values := map[string]float64{"distribution.electricity": 15, "inverter.dc_input": 14, "inverter.ac_output": 12, "inverter.loss": 2, "inverter.loss_decrement": -2, "inverter.ancillary": 0.1, "storage.charge": 7, "storage.decrement": -7, "storage.discharge": 6, "facility.demand": 20, "facility.produced": 12, "facility.purchased": 8, "facility.sold": 0}
	literals := epathSQLPVSourceLiterals()
	for i := range literals {
		if value, found := values[literals[i].id]; found {
			literals[i].power = value
		}
	}
	return append(literals, epathSQLPVSourceLiteral{id: "cogeneration.electricity", name: "Cogeneration:Electricity", meter: true, power: 0.1})
}

func epathSQLPVCogenerationClone(t *testing.T, input epathSQLPVCogenerationFrames) epathSQLPVCogenerationFrames {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var out epathSQLPVCogenerationFrames
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestEnergyPathSQLPVCogenerationNativeTwoParentsAndNineBalances(t *testing.T) {
	original, proof := epathSQLPVSourceOriginal(t)
	literals := epathSQLPVBalanceHandLiterals()
	path := epathSQLPVSourceSQL(t, literals, `CREATE INDEX native_pv_by_dictionary_time ON ReportData(ReportDataDictionaryIndex,TimeIndex)`)
	mtdPath := filepath.Join(filepath.Dir(path), "eplusout.mtd")
	if err := os.WriteFile(mtdPath, []byte(epathSQLPVCogenerationHandMTD(proof)), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := epathSQLPVFileSHA(path)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	requests, plan := epathSQLPVSourceHandRequests(literals)
	observed.originalText, observed.executedText, observed.outputPlan = original, requests+original, &plan
	core, err := epathCompileSQLPVSourceFrames(observed, proof, epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1})
	if err != nil {
		t.Fatal(err)
	}
	cg, err := epathCompileSQLPVCogenerationFrames(observed, core, mtdPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(core.Sources) != 40 || len(cg.Parents) != 2 || len(cg.AncillaryIDs) != 2 {
		t.Fatal("native2 parent extension changed the core40 census")
	}
	for _, frequency := range []string{"Monthly", "Hourly"} {
		p := cg.Parents[frequency]
		if p.Source.RawSum == nil || p.Source.EnergyKWh == nil || !epathSQLPVNativeNear(p.NativeAnnualKWh, 0.1*8760) || p.Spec.ID != "cogeneration.electricity" || p.OriginalOwnerIndex != nil || p.ExecutedOwnerIndex != nil || p.OutputObjectIndex != nil || !p.RequestBound || p.EffectiveMultiplier != 1 {
			t.Fatal("parent identity or model-total/temporary opener changed")
		}
		if core.Sources[cg.AncillaryIDs[frequency]].Spec.ID != "inverter.ancillary" {
			t.Fatal("parent membership selected another variable")
		}
	}
	if err := epathSQLValidatePVNativeBalances(core, cg); err != nil {
		t.Fatal(err)
	}
	// Thermal contexts intentionally do not satisfy charge-discharge=loss.
	// The nine proved electrical equations must not invent that tenth equation.
	for _, mode := range []string{"member_id", "parent_count", "native_raw", "source_owner", "original_hash", "mtd_members", "mtd_bytes", "executed_ems", "output_request", "opener", "wrong_frequency", "borrowed_core_source"} {
		t.Run(mode, func(t *testing.T) {
			bad := epathSQLPVCogenerationClone(t, cg)
			p := bad.Parents["Monthly"]
			switch mode {
			case "member_id":
				bad.AncillaryIDs["Monthly"] = 2000
			case "parent_count":
				delete(bad.Parents, "Hourly")
			case "native_raw":
				p.NativeMonthlyKWh[0]++
			case "source_owner":
				index := 0
				p.OriginalOwnerIndex = &index
			case "original_hash":
				bad.Membership.OriginalSHA256 = strings.Repeat("a", 64)
			case "mtd_members":
				bad.Membership.Members["Cogeneration:Electricity"] = []string{"other:inverter ancillary ac electricity energy"}
			case "mtd_bytes":
				bad.Membership.MTDText += "\nFor Meter=Cogeneration:Electricity [J], ResourceType=Electricity, EndUse=Cogeneration, contents are:\n  Other:Inverter Ancillary AC Electricity Energy\n"
			case "executed_ems":
				bad.Membership.ExecutedText += "\nEnergyManagementSystem:MeteredOutputVariable,Extra,Erl,SystemTimestep,,Electricity,Plant,OnSiteGeneration,,J;"
			case "output_request":
				for i := range bad.OutputPlan.OutputObjects {
					if bad.OutputPlan.OutputObjects[i].KeyValue == "Cogeneration:Electricity" {
						bad.OutputPlan.OutputObjects[i].State = "unsupported"
					}
				}
			case "opener":
				index := 0
				p.OutputObjectIndex = &index
			case "wrong_frequency":
				p.Dictionary.Frequency = "Hourly"
			case "borrowed_core_source":
				p.Dictionary.Index = 2000
			}
			bad.Parents["Monthly"] = p
			if err := epathSQLValidatePVCogenerationFrames(core, bad); err == nil {
				t.Fatal("changed native Cogeneration proof was accepted")
			}
		})
	}
	if _, err := epathCompileSQLPVCogenerationFrames(observed, core, filepath.Join(t.TempDir(), "eplusout.mtd")); err == nil {
		t.Fatal("unrelated MTD path accepted")
	}
	after, err := epathSQLPVFileSHA(path)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("independent compiler changed input SQL")
	}
}

func TestEnergyPathSQLPVCogenerationOriginalAndMTDNoHiddenContributor(t *testing.T) {
	original, proof := epathSQLPVSourceOriginal(t)
	mtd := epathSQLPVCogenerationHandMTD(proof)
	if _, err := epathSQLPVCogenerationMembershipProof(original, original, mtd, proof); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"custom", "custom_decrement", "EMS_meter", "EMS_actuator", "extra_member", "duplicate_member", "duplicate_section", "wrong_resource", "missing_parent", "charge_as_consumption", "ancillary_not_consumption", "missing_PV"} {
		t.Run(mode, func(t *testing.T) {
			x, changed := original, mtd
			switch mode {
			case "custom":
				x += "\nMeter:Custom,Cogeneration:Electricity,Electricity;"
			case "custom_decrement":
				x += "\nMeter:CustomDecrement,Cogeneration:Electricity,Electricity,Electricity:Facility;"
			case "EMS_meter":
				x += "\nEnergyManagementSystem:MeteredOutputVariable,Extra,Erl,SystemTimestep,,Electricity,Plant,OnSiteGeneration,,J;"
			case "EMS_actuator":
				x += "\nEnergyManagementSystem:Actuator,Override,Kibam,Electrical Storage,Power Draw Rate;"
			case "extra_member":
				changed = strings.TrimSpace(mtd) + "\n  Hidden:Converter Ancillary AC Electricity Energy\n"
			case "duplicate_member":
				changed = strings.TrimSpace(mtd) + "\n  " + proof.Declaration.InverterName + ":Inverter Ancillary AC Electricity Energy\n"
			case "duplicate_section":
				changed = mtd + mtd
			case "wrong_resource":
				changed = strings.Replace(mtd, "Cogeneration:Electricity [J], ResourceType=Electricity,", "Cogeneration:Electricity [J], ResourceType=ElectricityProduced,", 1)
			case "missing_parent":
				changed = strings.Split(mtd, "For Meter=Cogeneration:Electricity")[0]
			case "charge_as_consumption":
				changed = strings.Replace(mtd, "  ZN_1_FLR_1_SEC_1_LIGHTS:Lights Electricity Energy", "  "+proof.Declaration.StorageName+":Electric Storage Charge Energy", 1)
			case "ancillary_not_consumption":
				changed = strings.Replace(mtd, "  "+proof.Declaration.InverterName+":Inverter Ancillary AC Electricity Energy\n", "", 1)
			case "missing_PV":
				changed = strings.Replace(mtd, "  "+proof.Declaration.Generators[0].Name+":Generator Produced DC Electricity Energy\n", "", 1)
			}
			if _, err := epathSQLPVCogenerationMembershipProof(original, x, changed, proof); err == nil {
				t.Fatal("unproved native meter-member boundary accepted")
			}
		})
	}
}

func TestEnergyPathSQLPVCogenerationParentNativeZeroAndUnknown(t *testing.T) {
	spec := epathSQLPVCogenerationParentSpec()
	literal := epathSQLPVSourceLiteral{id: spec.ID, name: spec.Name, meter: true, power: 0}
	for _, tc := range []struct {
		name, sql string
		good      bool
	}{
		{"measured_zero", "", true},
		{"NULL", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=2000 AND TimeIndex=1`, false},
		{"missing", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=2000 AND TimeIndex=1`, false},
		{"duplicate", `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,ReportDataDictionaryIndex,Value FROM ReportData WHERE ReportDataDictionaryIndex=2000 AND TimeIndex=1`, false},
		{"foreign_heating_key", `UPDATE ReportDataDictionary SET KeyValue='Heating:Electricity' WHERE ReportDataDictionaryIndex=2000`, false},
		{"Avg", `UPDATE ReportDataDictionary SET Type='Avg' WHERE ReportDataDictionaryIndex=2000`, false},
		{"wrong_step", `UPDATE ReportDataDictionary SET TimestepType='HVAC System' WHERE ReportDataDictionaryIndex=2000`, false},
		{"schedule", `UPDATE ReportDataDictionary SET ScheduleName='Filtered' WHERE ReportDataDictionaryIndex=2000`, false},
		{"negative", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=2000 AND TimeIndex=1`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutations := []string{}
			if tc.sql != "" {
				mutations = append(mutations, tc.sql)
			}
			path := epathSQLPVSourceSQL(t, []epathSQLPVSourceLiteral{literal}, mutations...)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			weather, err := epathReadOracleWeather(db)
			if err != nil {
				t.Fatal(err)
			}
			dictionary, err := epathSQLPVReadDictionary(db, spec, "Monthly")
			if err != nil {
				if tc.good {
					t.Fatal(err)
				}
				return
			}
			rows, err := epathSQLPVReadRows(db, dictionary)
			if err != nil {
				if tc.good {
					t.Fatal(err)
				}
				return
			}
			source, _, _, _, err := epathSQLPVNativeSummary(spec, dictionary, weather, rows)
			if !tc.good {
				if err == nil {
					t.Fatal("invalid native parent became measured0")
				}
				return
			}
			if err != nil || source.RawSum == nil || source.EnergyKWh == nil || *source.RawSum != 0 || *source.EnergyKWh != 0 || source.Rows != 12 {
				t.Fatalf("native measured0 lost presence: %+v / %v", source, err)
			}
		})
	}
}
