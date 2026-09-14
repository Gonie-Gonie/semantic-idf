package simulation

// Native reader integration tests. Uses the frozen literal pipeline
// fixture builder, not a native capture or candidate. Root alone runs Go.
import (
	"database/sql"
	"reflect"
	"testing"
)

func epathCogenReadDraft(t *testing.T, spec epathCogenPipelineSpec, edit string) energyPathCogenerationReadResult {
	t.Helper()
	f := epathCogenPipelineSQL(t, spec)
	plan, _ := epathCogenPipelinePlan(f, PurposeAllocationPolicyDirectOnly)
	db, err := sql.Open("sqlite", f.Files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if edit != "" {
		if _, err := db.Exec(edit); err != nil {
			t.Fatal(err)
		}
	} // Only this fresh t.TempDir literal SQL.
	pv, sources, _, err := readEnergyPathPVElectricalObservations(db, "literal", &plan, energyPathBuildPVElectricalInventory(f.Document), nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := readEnergyPathCogenerationInputs(db, "literal", &plan, energyPathBuildCogenerationInventory(f.Document), pv, sources)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestEnergyPathCogenerationReaderMembershipIsNotObservationKnownness(t *testing.T) {
	result := epathCogenReadDraft(t, epathCogenPipelineSpec{Parent: "positive", Member: "positive", ExtraOriginal: "Meter:Custom,Cogeneration:Electricity,Electricity;"}, "")
	if len(result.Series) != 0 {
		t.Fatal("custom meter collision authorized predefined consumption")
	}
	for _, id := range []string{"sql-rdd-61", "sql-rdd-62"} {
		source := energyExplanationSourceByID(result.Sources, id)
		if source == nil || !energyDataSourceValueKnown(*source, energySourceObservedRaw) || source.RawValue != 4 {
			t.Fatalf("membership denial destroyed actual measured quantity: %s %+v", id, source)
		}
	}
	if !result.MonthlyParentPresence["electricity"] {
		t.Fatal("custom/unqualified existing parent became absent")
	}
}

func TestEnergyPathCogenerationReaderPartialMonthlyAndNULLTabularNoFill(t *testing.T) {
	result := epathCogenReadDraft(t, epathCogenPipelineSpec{Parent: "partial", Member: "positive", TableMode: "positive", TableColumn: "Electricity", TableValue: 10}, "")
	if len(result.Series) != 1 || result.Series[0].Total != 4 || result.Series[0].Monthly[1] != 4 {
		t.Fatalf("valid native M1 was not retained: %+v", result.Series)
	}
	if _, found := result.Series[0].Monthly[2]; found {
		t.Fatal("NULL M2 was filled with ancillary or TAB")
	}
	parent := energyExplanationSourceByID(result.Sources, "sql-rdd-61")
	if parent == nil || energyDataSourceValueKnown(*parent, energySourceObservedRaw) || energyDataSourceValueKnown(*parent, energySourceObservedEffective) {
		t.Fatal("partial parent acquired full annual observation proof")
	}
	annual := energyExplanationSourceByID(result.Sources, "sql-tabular-cogeneration-101")
	if annual == nil || annual.RawValue != 10 || !energyDataSourceValueKnown(*annual, energySourceObservedRaw) {
		t.Fatal("suppressed TAB graph budget lost its independent observed annual source")
	}
}

func TestEnergyPathCogenerationReaderNativeDictionaryAndHourlyParentBlock(t *testing.T) {
	for _, edit := range []string{
		`UPDATE ReportDataDictionary SET Type='Avg' WHERE ReportDataDictionaryIndex=61`,
		`UPDATE ReportDataDictionary SET TimestepType='HVAC System' WHERE ReportDataDictionaryIndex=61`,
		`UPDATE ReportDataDictionary SET ScheduleName='Scheduled' WHERE ReportDataDictionaryIndex=61`,
		`UPDATE ReportDataDictionary SET IsMeter=0 WHERE ReportDataDictionaryIndex=61`,
		`UPDATE ReportDataDictionary SET KeyValue='FOREIGN' WHERE ReportDataDictionaryIndex=61`,
		`UPDATE ReportDataDictionary SET ReportingFrequency='Hourly' WHERE ReportDataDictionaryIndex=61`,
	} {
		t.Run(edit, func(t *testing.T) {
			result := epathCogenReadDraft(t, epathCogenPipelineSpec{Parent: "positive", Member: "positive"}, edit)
			if len(result.Series) != 0 {
				t.Fatal("invalid/unobserved parent allowed own or member budget")
			}
			if !result.MonthlyParentPresence["electricity"] {
				t.Fatal("present unproved parent lost deny state")
			}
			parent := energyExplanationSourceByID(result.Sources, "sql-rdd-61")
			if parent == nil || energyDataSourceValueKnown(*parent, energySourceObservedRaw) {
				t.Fatal("invalid native identity/axis acquired annual scalar proof")
			}
		})
	}
}

func TestEnergyPathCogenerationReaderSoleMemberAndLegacyMergeIsolation(t *testing.T) {
	result := epathCogenReadDraft(t, epathCogenPipelineSpec{Parent: "absent", Member: "positive"}, "")
	if len(result.Series) != 1 || result.Series[0].Total != 4 || !epathCogenPipelineContains(result.Series[0].SourceIDs, "sql-rdd-62") {
		t.Fatalf("unique native sole member not selected: %+v", result.Series)
	}
	legacy := []energyExplanationSeries{{sourceName: "Cogeneration:Electricity"}, {sourceName: "Inverter Ancillary AC Electricity Energy"}, {sourceName: "Electricity:Facility"}}
	before := append([]energyExplanationSeries(nil), legacy...)
	merged := mergeEnergyPathCogenerationSeries(legacy, result)
	if len(merged) != 2 || merged[0].sourceName != "Electricity:Facility" || merged[1].EndUse != "cogeneration_input" {
		t.Fatal("native replacement left legacy double count or removed unrelated meter")
	}
	if !reflect.DeepEqual(before, legacy) || !reflect.DeepEqual(mergeEnergyPathCogenerationSeries(legacy, energyPathCogenerationReadResult{}), legacy) {
		t.Fatal("input mutation or omitted-original behavior changed")
	}
}

func TestEnergyPathCogenerationReaderDuplicateTabularRejectsMembershipNotRawCells(t *testing.T) {
	result := epathCogenReadDraft(t, epathCogenPipelineSpec{Parent: "absent", Member: "absent", TableMode: "positive", TableColumn: "Electricity", TableValue: 4}, `INSERT INTO TabularDataWithStrings SELECT 102,ReportName,ReportForString,TableName,RowName,ColumnName,Units,RowId,ColumnId,Value FROM TabularDataWithStrings WHERE TabularDataIndex=101`)
	if len(result.Series) != 0 {
		t.Fatal("duplicate native table identity authorized consumption")
	}
	for _, id := range []string{"sql-tabular-cogeneration-101", "sql-tabular-cogeneration-102"} {
		source := energyExplanationSourceByID(result.Sources, id)
		if source == nil || source.RawValue != 4 || !energyDataSourceValueKnown(*source, energySourceObservedRaw) {
			t.Fatal("duplicate membership denial discarded an actual cell")
		}
	}
}

func TestEnergyPathCogenerationReaderNegativeParentNeverBecomesPositiveOnlyAnnual(t *testing.T) {
	result := epathCogenReadDraft(t, epathCogenPipelineSpec{Parent: "positive", Member: "positive"}, `UPDATE ReportData SET Value=-3600000 WHERE ReportDataDictionaryIndex=61 AND TimeIndex=2`)
	if len(result.Series) != 0 {
		t.Fatal("signed parent was reinterpreted as a positive-only annual consumed total")
	}
	source := energyExplanationSourceByID(result.Sources, "sql-rdd-61")
	if source == nil || !energyDataSourceValueKnown(*source, energySourceObservedRaw) || source.RawValue != 3 {
		t.Fatal("signed native 4 + -1 observation was changed or hidden")
	}
}
