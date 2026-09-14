package simulation

// Independent literal integration regressions. No EnergyPlus execution,
// candidate values, accepted fixture values or production-generated oracle.
// The two-month/three-hour SQL is a controlled observation axis, not an annual
// physical balance. Install with the reader/marker/root glue before running.
import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func pvElectricalIntegrationItem(name, key, frequency string, id int, total float64) energyExplanationSeries {
	item := energyExplanationSeries{Level: "energy", Kind: "literal." + name, Unit: "kWh", Carrier: "electricity", EndUse: name, SourceName: name, SourceKey: key, sourceName: name, sourceKeyValue: key, sourceFrequency: frequency, SourceIDs: []string{fmt.Sprintf("sql-rdd-%d", id)}, Total: total}
	if frequency == "Monthly" {
		item.Monthly = map[int]float64{1: total, 2: 0}
	}
	if frequency == "Hourly" {
		item.Monthly = map[int]float64{1: total}
		item.Hourly = map[int]float64{1: total / 2, 2: 0, 3: total / 2}
	}
	return item
}

func TestPVElectricalIntegrationGateKeepsFourFacilityFamiliesOnce(t *testing.T) {
	literals := pvElectricalReaderLiterals()
	literals[17].value = 4 // Native signed meter is positive in every weather cell.
	path := pvElectricalReaderCreateSQLWithLiterals(t, literals)
	evidence, _, _ := pvElectricalReaderRead(t, path, pvElectricalReaderDocument(t), nil)
	var input []energyExplanationSeries
	for index := 16; index < 20; index++ {
		for frequencyIndex, frequency := range []string{"Monthly", "Hourly"} {
			input = append(input, pvElectricalIntegrationItem(literals[index].name, "", frequency, 900+2*index+frequencyIndex, literals[index].value))
		}
	}
	kept, warnings := filterEnergyPathPVElectricalSeries(input, evidence)
	if len(kept) != 8 || len(warnings) != 0 {
		t.Fatalf("valid four meter x M/H observations removed: kept=%d warnings=%+v", len(kept), warnings)
	}
	// Existing preference, not the gate, selects the quantity once. Different
	// reporting frequencies remain separate native sources and chart evidence.
	for index := range kept {
		kept[index] = canonicalEnergyExplanationSeries(kept[index])
	}
	selected := preferredEnergyExplanationSeries(kept)
	if len(selected) != 4 {
		t.Fatalf("Facility frequency duplication: %#v", selected)
	}
	for _, item := range selected {
		index := -1
		for i := 16; i < 20; i++ {
			if item.SourceName == literals[i].name {
				index = i
			}
		}
		if index < 0 || item.Total != literals[index].value || item.Monthly[1] != literals[index].value || item.Monthly[2] != 0 || len(item.Monthly) != 2 {
			t.Fatalf("M/H were added or changed: %+v", item)
		}
		want := fmt.Sprintf("sql-rdd-%d", 900+2*index)
		if !reflect.DeepEqual(item.AnnualSourceIDs, []string{want}) || !reflect.DeepEqual(item.MonthlySourceIDs, []string{want}) {
			t.Fatalf("annual/monthly authority was borrowed: %+v", item)
		}
	}
	ordinary := pvElectricalIntegrationItem("InteriorLights:Electricity", "", "Monthly", 700, 5)
	kept, warnings = filterEnergyPathPVElectricalSeries([]energyExplanationSeries{ordinary}, evidence)
	if !reflect.DeepEqual(kept, []energyExplanationSeries{ordinary}) || len(warnings) != 0 {
		t.Fatal("unrelated ordinary meter changed")
	}
	legacy := pvElectricalIntegrationItem("Electric Storage Discharge Energy", "BATTERY", "Timestep", 701, 6)
	legacyFacility := pvElectricalIntegrationItem("Electricity:Facility", "", "Monthly", 702, 9)
	for _, absent := range []energyPathPVElectricalEvidence{{}, {Inventory: energyPathPVElectricalInventory{HasOriginal: true}}} {
		kept, warnings = filterEnergyPathPVElectricalSeries([]energyExplanationSeries{legacy, ordinary, legacyFacility}, absent)
		if !reflect.DeepEqual(kept, []energyExplanationSeries{legacy, ordinary, legacyFacility}) || len(warnings) != 0 {
			t.Fatal("unreviewed/no-original compatibility changed")
		}
	}
}

func TestPVElectricalIntegrationGateComponentsRemainNonadditiveAtEveryFrequency(t *testing.T) {
	path := pvElectricalReaderCreateSQL(t)
	evidence, _, _ := pvElectricalReaderRead(t, path, pvElectricalReaderDocument(t), nil)
	for index, literal := range pvElectricalReaderLiterals()[:16] {
		for _, frequency := range []string{"Monthly", "Hourly", "Timestep", "Detailed", "RunPeriod", "Annual"} {
			t.Run(literal.name+"/"+literal.key+"/"+frequency, func(t *testing.T) {
				id := 900 + 2*index
				if frequency == "Hourly" {
					id++
				}
				if frequency != "Monthly" && frequency != "Hourly" {
					id = 2000 + index
				} // A distinct actual-frequency RDD, not a borrowed M/H identity.
				item := pvElectricalIntegrationItem(literal.name, literal.key, frequency, id, literal.value)
				kept, _ := filterEnergyPathPVElectricalSeries([]energyExplanationSeries{item}, evidence)
				if literal.name == "Electric Storage Charge Energy" {
					// This is retention, not flow permission. The durable charge
					// qualifier is verified separately in the actual SQL pipeline.
					if len(kept) != 1 || kept[0].SourceName != item.SourceName || kept[0].sourceFrequency != frequency || !reflect.DeepEqual(kept[0].SourceIDs, item.SourceIDs) || kept[0].Total != literal.value {
						t.Fatal("charge context lost its native quantity/identity or existing qualifier path")
					}
				} else if len(kept) != 0 {
					t.Fatalf("native component became a second graph budget: %+v", kept)
				}
			})
		}
	}
	for _, name := range []string{"Electricity:Facility", "ElectricityProduced:Facility", "ElectricityPurchased:Facility", "ElectricitySurplusSold:Facility"} {
		item := pvElectricalIntegrationItem(name, "", "Timestep", 999, 100)
		if kept, _ := filterEnergyPathPVElectricalSeries([]energyExplanationSeries{item}, evidence); len(kept) != 0 {
			t.Fatalf("unvalidated Timestep resurrected %s", name)
		}
	}
}

func TestPVElectricalIntegrationGateRejectsBorrowedKeyAndMixedSources(t *testing.T) {
	path := pvElectricalReaderCreateSQL(t)
	evidence, _, _ := pvElectricalReaderRead(t, path, pvElectricalReaderDocument(t), nil)
	for _, mutate := range []struct {
		name  string
		apply func(*energyExplanationSeries)
	}{
		{"foreign_key", func(item *energyExplanationSeries) { item.SourceKey = "FOREIGN"; item.sourceKeyValue = "FOREIGN" }},
		{"mixed_source", func(item *energyExplanationSeries) { item.SourceIDs = append(item.SourceIDs, "sql-rdd-924") }},
		{"wrong_frequency", func(item *energyExplanationSeries) { item.sourceFrequency = "Hourly" }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			item := pvElectricalIntegrationItem("Electric Storage Charge Energy", "BATTERY", "Monthly", 920, 7)
			mutate.apply(&item)
			if kept, _ := filterEnergyPathPVElectricalSeries([]energyExplanationSeries{item}, evidence); len(kept) != 0 {
				t.Fatalf("source identity was borrowed: %+v", kept)
			}
		})
	}
	bad := evidence
	bad.SourceSnapshots = append([]EnergyDataSource(nil), evidence.SourceSnapshots...)
	for index := range bad.SourceSnapshots {
		if bad.SourceSnapshots[index].ID == "sql-rdd-932" {
			bad.SourceSnapshots[index].IsMeter = false
		}
	}
	item := pvElectricalIntegrationItem("Electricity:Facility", "", "Monthly", 932, 9)
	if kept, _ := filterEnergyPathPVElectricalSeries([]energyExplanationSeries{item}, bad); len(kept) != 0 {
		t.Fatal("a non-meter snapshot authorized the Facility meter budget")
	}
}

func TestPVElectricalIntegrationGateUsesUnroundedNativeNegativeCells(t *testing.T) {
	for _, frequency := range []string{"Monthly", "Hourly"} {
		t.Run(frequency, func(t *testing.T) {
			id := 934
			mutation := `UPDATE ReportData SET Value=CASE TimeIndex WHEN 101 THEN -360 WHEN 102 THEN 7200000 ELSE Value END WHERE ReportDataDictionaryIndex=934`
			if frequency == "Hourly" {
				id = 935
				mutation = `UPDATE ReportData SET Value=CASE TimeIndex WHEN 201 THEN -360 WHEN 202 THEN 0 WHEN 203 THEN 7200000 ELSE Value END WHERE ReportDataDictionaryIndex=935`
			}
			path := pvElectricalReaderCreateSQL(t, mutation)
			evidence, sources, _ := pvElectricalReaderRead(t, path, pvElectricalReaderDocument(t), nil)
			source := pvElectricalReaderSource(t, sources, id)
			if !energyDataSourceValueKnown(source, energySourceObservedRaw) || math.Abs(source.RawValue-1.9999) > 1e-12 {
				t.Fatalf("native signed total must stay known and positive: %+v", source)
			}
			observation := pvElectricalReaderObservation(t, evidence, "ElectricityProduced:Facility", frequency)
			if observation.Status != "observed" || !observation.HasNegativeValue {
				t.Fatalf("native tiny negative was discarded: %+v", observation)
			}
			item := pvElectricalIntegrationItem("ElectricityProduced:Facility", "", frequency, id, 2)
			item.Monthly = map[int]float64{1: 0, 2: 2}
			item.Hourly = map[int]float64{1: 0, 2: 0, 3: 2}
			if energyPathPVElectricalSeriesHasNegativePeriod(item) {
				t.Fatal("fixture must hide native tiny negative after display rounding")
			}
			if kept, _ := filterEnergyPathPVElectricalSeries([]energyExplanationSeries{item}, evidence); len(kept) != 0 {
				t.Fatal("rounded 0 hid a native negative and invented positive supply")
			}
		})
	}
	// Negative design-day/warmup distractors are not native weather evidence.
	literals := pvElectricalReaderLiterals()
	literals[17].value = 4
	path := pvElectricalReaderCreateSQLWithLiterals(t, literals, `UPDATE ReportData SET Value=-999999999999 WHERE TimeIndex IN (1,2) AND ReportDataDictionaryIndex IN (934,935)`)
	evidence, _, _ := pvElectricalReaderRead(t, path, pvElectricalReaderDocument(t), nil)
	for offset, frequency := range []string{"Monthly", "Hourly"} {
		observation := pvElectricalReaderObservation(t, evidence, "ElectricityProduced:Facility", frequency)
		if observation.HasNegativeValue {
			t.Fatal("excluded design/warmup row tainted the weather sign")
		}
		if kept, _ := filterEnergyPathPVElectricalSeries([]energyExplanationSeries{pvElectricalIntegrationItem("ElectricityProduced:Facility", "", frequency, 934+offset, 4)}, evidence); len(kept) != 1 {
			t.Fatal("valid weather-only production was suppressed")
		}
	}
}

type pvElectricalIntegrationWant struct {
	values   map[int]float64
	keys     map[int]string
	unknown  map[int]bool
	absent   map[int]bool
	blocked  map[string]bool
	facility float64
	produced float64
}

func pvElectricalIntegrationNativeWant(literals []pvElectricalReaderLiteral) pvElectricalIntegrationWant {
	want := pvElectricalIntegrationWant{values: map[int]float64{}, keys: map[int]string{}, unknown: map[int]bool{}, absent: map[int]bool{}, blocked: map[string]bool{}, facility: literals[16].value, produced: literals[17].value}
	for index, literal := range literals {
		for offset := 0; offset < 2; offset++ {
			id := 900 + 2*index + offset
			want.values[id] = math.Round(literal.value*1000) / 1000
			want.keys[id] = literal.key
			if index < 16 && index != 10 || index == 17 && literal.value < 0 {
				want.blocked[fmt.Sprintf("sql-rdd-%d", id)] = true
			}
		}
	}
	return want
}

func pvElectricalIntegrationCheckSources(t *testing.T, sources []EnergyDataSource, literals []pvElectricalReaderLiteral, want pvElectricalIntegrationWant) {
	t.Helper()
	for index, literal := range literals {
		for offset, frequency := range []string{"Monthly", "Hourly"} {
			id := 900 + 2*index + offset
			if want.absent[id] {
				for _, source := range sources {
					if source.ID == fmt.Sprintf("sql-rdd-%d", id) {
						t.Fatalf("missing dictionary fabricated: %+v", source)
					}
				}
				continue
			}
			source := pvElectricalReaderSource(t, sources, id)
			if source.Name != literal.name || source.KeyValue != want.keys[id] || source.ReportingFrequency != frequency || source.IsMeter != literal.meter || source.SourceUnit != "J" || source.Units != "J" || source.NormalizedUnit != "kWh" {
				t.Fatalf("exact native identity changed: %+v", source)
			}
			for _, bit := range []uint8{energySourceObservedRaw, energySourceObservedEffective} {
				if energyDataSourceValueKnown(source, bit) == want.unknown[id] {
					t.Fatalf("known zero/NULL presence changed for %d: %+v", id, source)
				}
			}
			if !want.unknown[id] && (math.Abs(source.RawValue-want.values[id]) > 1e-12 || source.EffectiveValue != source.RawValue) {
				t.Fatalf("native scalar/sign changed for %d: %+v want %.12g", id, source, want.values[id])
			}
			if want.unknown[id] && (source.RawValue != 0 || source.EffectiveValue != 0 || source.HourlyEnergy != nil) {
				t.Fatalf("unknown scalar/curve backfilled for %d: %+v", id, source)
			}
			if source.ObjectIndex != nil || source.ZoneName != "" || source.EffectiveMultiplier != 1 || source.AggregationBasis != "model_total" || source.MultiplierApplication != "already_model_total" || source.AllocationApplied || source.AllocatedValue != 0 || len(source.ScopeDetails) != 0 || len(source.RelatedEntityIDs) != 0 {
				t.Fatalf("native observation acquired physical index/Zone/allocation: %+v", source)
			}
			if source.DriverRole != energyDriverSourceRoleContext || source.InspectorSection != energyDriverInspectorSectionContext {
				t.Fatalf("source display role became numerical permission: %+v", source)
			}
		}
	}
}

func pvElectricalIntegrationCheckAvailability(t *testing.T, completeness EnergyCompleteness, literals []pvElectricalReaderLiteral, want pvElectricalIntegrationWant) {
	t.Helper()
	for index, literal := range literals {
		for offset, frequency := range []string{"Monthly", "Hourly"} {
			key := literal.key
			if key == "" {
				key = "meter"
			}
			name := fmt.Sprintf("%s [%s; %s]", literal.name, key, frequency)
			count := 0
			for _, entry := range completeness.SourceAvailability {
				if !strings.EqualFold(entry.Name, name) {
					continue
				}
				count++
				id := 900 + 2*index + offset
				expected := "found"
				if want.unknown[id] || want.absent[id] {
					expected = "missing"
				}
				if entry.Status != expected || entry.Level != "context" {
					t.Fatalf("native observation coverage %s: %+v want %s", name, entry, expected)
				}
			}
			if count != 1 {
				t.Fatalf("native observation coverage %s count=%d want1", name, count)
			}
		}
	}
}

func pvElectricalIntegrationCheckGraph(t *testing.T, nodes []EnergyExplanationNode, links []EnergyPathLink, edges []EnergyExplanationEdge, want pvElectricalIntegrationWant, period string) {
	t.Helper()
	checkIDs := func(ids []string, owner string, chargeAllowed bool) {
		for _, id := range ids {
			if want.blocked[id] {
				t.Errorf("%s/%s used forbidden native graph source %s", period, owner, id)
			}
			if !chargeAllowed && (id == "sql-rdd-920" || id == "sql-rdd-921" || id == "sql-rdd-1002") {
				t.Errorf("%s/%s charge context became flow", period, owner)
			}
		}
	}
	facilityCount, producedCount := 0, 0
	for _, node := range nodes {
		charge := node.Level == "support" && node.EndUse == "storage_charge"
		checkIDs(node.SourceIDs, node.ID, charge)
		if node.Kind == "energy.electricity.total" || node.Level == "carrier" && node.Carrier == "electricity" {
			facilityCount++
			expected := want.facility
			if period == "M2" {
				expected = 0
			}
			if math.Abs(node.Value-expected) > 1e-12 {
				t.Errorf("%s Facility changed/doubled: %g want %g", period, node.Value, expected)
			}
		}
		if node.Kind == "energy.generators" || node.EndUse == "generators" || node.EndUse == "onsite_production" {
			producedCount++
			expected := want.produced
			if period == "M2" {
				expected = 0
			}
			if expected < 0 || math.Abs(node.Value-expected) > 1e-12 {
				t.Errorf("%s Produced changed/doubled/abs: %+v want %g", period, node, expected)
			}
		}
	}
	if (period == "root" || period == "annual" || period == "M1") && want.facility > 0 && facilityCount != 1 {
		t.Errorf("%s Facility count=%d want1", period, facilityCount)
	}
	if (period == "root" || period == "annual" || period == "M1") && want.produced > 0 && producedCount != 1 {
		t.Errorf("%s Produced count=%d want1", period, producedCount)
	}
	if facilityCount > 1 || producedCount > 1 {
		t.Errorf("%s native M/H or components duplicated a meter: facility=%d produced=%d", period, facilityCount, producedCount)
	}
	for _, link := range links {
		checkIDs(link.SourceIDs, link.ID, false)
	}
	for _, edge := range edges {
		checkIDs(edge.SourceIDs, edge.ID, false)
	}
}

func TestPVElectricalIntegrationPublicSQLV1TwoJSONAndV2RetainSourcesNotBudgets(t *testing.T) {
	for _, mode := range []string{"positive", "signed", "tiny_signed", "null_and_timestep", "missing_dictionary_and_timestep", "duplicate_dictionary", "foreign_meter_key_alias"} {
		t.Run(mode, func(t *testing.T) {
			literals := pvElectricalReaderLiterals()
			literals[17].value = 4
			var mutations []string
			if mode == "signed" {
				literals[17].value = -.25
			}
			want := pvElectricalIntegrationNativeWant(literals)
			if mode == "tiny_signed" {
				mutations = append(mutations, `UPDATE ReportData SET Value=CASE TimeIndex WHEN 101 THEN -360 WHEN 102 THEN 7200000 ELSE Value END WHERE ReportDataDictionaryIndex=934`, `UPDATE ReportData SET Value=CASE TimeIndex WHEN 201 THEN -360 WHEN 202 THEN 0 WHEN 203 THEN 7200000 ELSE Value END WHERE ReportDataDictionaryIndex=935`)
				want.values[934], want.values[935] = 2, 2
				want.produced = -1
				want.blocked["sql-rdd-934"], want.blocked["sql-rdd-935"] = true, true
			}
			if mode == "null_and_timestep" || mode == "missing_dictionary_and_timestep" {
				mutations = append(mutations, `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex IN (920,921,932,933,934,935) AND TimeIndex NOT IN (1,2)`)
				for _, id := range []int{920, 921, 932, 933, 934, 935} {
					want.unknown[id] = true
					if id != 920 && id != 921 {
						want.blocked[fmt.Sprintf("sql-rdd-%d", id)] = true
					}
				}
				want.facility, want.produced = 0, -1
				if mode == "missing_dictionary_and_timestep" {
					mutations = append(mutations, `DELETE FROM ReportData WHERE ReportDataDictionaryIndex IN (934,935)`, `DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex IN (934,935)`)
					want.absent[934], want.absent[935] = true, true
				}
			}
			if mode == "duplicate_dictionary" {
				mutations = append(mutations, `INSERT INTO ReportDataDictionary SELECT 1100,IsMeter,Type,IndexGroup,TimestepType,KeyValue,Name,ReportingFrequency,ScheduleName,Units FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=934`, `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,1100,Value FROM ReportData WHERE ReportDataDictionaryIndex=934`, `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=935 AND TimeIndex NOT IN (1,2)`)
				want.unknown[934], want.unknown[935] = true, true
				want.produced = -1
				want.blocked["sql-rdd-934"], want.blocked["sql-rdd-935"], want.blocked["sql-rdd-1100"] = true, true, true
			}
			if mode == "foreign_meter_key_alias" {
				// Generic alias selection may prefer this invalid key over the
				// actual dictionary Name. Native ID ownership must still deny
				// its flow, without rewriting the actual foreign-key source.
				mutations = append(mutations, `UPDATE ReportDataDictionary SET KeyValue='Heating:Electricity' WHERE ReportDataDictionaryIndex IN (932,933)`)
				for _, id := range []int{932, 933} {
					want.keys[id] = "Heating:Electricity"
					want.unknown[id] = true
					want.blocked[fmt.Sprintf("sql-rdd-%d", id)] = true
				}
				want.facility = 0
			}
			// Actual existing-style Timestep declarations/rows cannot authorize
			// another Produced or Discharge budget, even when M/H are invalid.
			mutations = append(mutations, `INSERT INTO ReportDataDictionary VALUES (1000,1,'Sum','Facility:ElectricityProduced','Zone',NULL,'ElectricityProduced:Facility','Timestep',NULL,'J'),(1001,0,'Sum','System','HVAC System','BATTERY','Electric Storage Discharge Energy','Timestep',NULL,'J'),(1002,0,'Sum','System','HVAC System','BATTERY','Electric Storage Charge Energy','Timestep',NULL,'J')`, `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES (201,1000,180000000),(202,1000,180000000),(203,1000,180000000),(201,1001,7200000),(202,1001,7200000),(203,1001,7200000),(201,1002,7200000),(202,1002,7200000),(203,1002,7200000)`)
			want.blocked["sql-rdd-1000"], want.blocked["sql-rdd-1001"] = true, true
			path := pvElectricalReaderCreateSQLWithLiterals(t, literals, mutations...)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc := pvElectricalReaderDocument(t)
			pvElectricalReaderAppendOriginal(t, &doc, `Zone,Office,0,0,0,0,1,32;
Output:Meter,ElectricityProduced:Facility,Timestep;
Output:Variable,BATTERY,Electric Storage Discharge Energy,Timestep;
Output:Variable,BATTERY,Electric Storage Charge Energy,Timestep;`)
			plan := pvElectricalReaderPlan(doc)
			context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
			legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			checkV2 := func(result EnergyExplanationResult) {
				pvElectricalIntegrationCheckSources(t, result.Sources, literals, want)
				pvElectricalIntegrationCheckAvailability(t, result.Completeness, literals, want)
				legacyContext := pvElectricalReaderSource(t, result.Sources, 1001)
				if legacyContext.pvObservationProtection == nil || !legacyContext.pvObservationProtection.LegacyContextOnly || legacyContext.AllocationApplied || legacyContext.AllocatedValue != 0 || len(legacyContext.ScopeDetails) != 0 {
					t.Fatalf("removed Timestep discharge context acquired allocation: %+v", legacyContext)
				}
				pvElectricalIntegrationCheckGraph(t, result.Nodes, result.Links, nil, want, "root")
				for _, period := range result.Periods {
					if period.ID == "annual" || period.ID == "M1" || period.ID == "M2" {
						pvElectricalIntegrationCheckGraph(t, period.Nodes, period.Links, nil, want, period.ID)
					}
				}
			}
			for pass := 0; pass < 3; pass++ {
				pvElectricalIntegrationCheckSources(t, legacy.Sources, literals, want)
				pvElectricalIntegrationCheckAvailability(t, legacy.Completeness, literals, want)
				pvElectricalIntegrationCheckGraph(t, legacy.Nodes, nil, legacy.Edges, want, "root")
				checkV2(UpgradeEnergyExplanationV1(legacy))
				if pass == 2 {
					break
				}
				raw, err := json.Marshal(legacy)
				if err != nil {
					t.Fatal(err)
				}
				var next EnergyExplanationV1
				if err := json.Unmarshal(raw, &next); err != nil {
					t.Fatal(err)
				}
				legacy = next
			}
			result := UpgradeEnergyExplanationV1(legacy)
			for pass := 0; pass < 2; pass++ {
				raw, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				var next EnergyExplanationResult
				if err := json.Unmarshal(raw, &next); err != nil {
					t.Fatal(err)
				}
				result = next
				checkV2(result)
			}
			if mode == "duplicate_dictionary" {
				source := pvElectricalReaderSource(t, result.Sources, 1100)
				if energyDataSourceValueKnown(source, energySourceObservedRaw) || energyDataSourceValueKnown(source, energySourceObservedEffective) {
					t.Fatalf("duplicate actual RDD borrowed known quantity: %+v", source)
				}
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if sha256.Sum256(before) != sha256.Sum256(after) {
				t.Fatal("public read pipeline modified native SQL")
			}
		})
	}
}
