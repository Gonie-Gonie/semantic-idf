package simulation

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func epathSQLOriginalUnitFixture(t *testing.T, raw string) (epathRealSQLSite, epathSQLTabularObservation, epathSQLOriginalSource, EnergyDataSource) {
	t.Helper()
	path, selector := epathSQLTabularUnitFixture(t), epathSQLTabularUnitSelector()
	epathOracleEditSQL(t, path, fmt.Sprintf("UPDATE TabularDataWithStrings SET Value='%s' WHERE TabularDataIndex=1", raw))
	observation, err := epathReadSQLModelTabular(path, selector)
	if err != nil || observation == nil {
		t.Fatalf("original fixture cell unavailable: %v", err)
	}
	site := epathRealSQLSite{ID: "cooling.district_cooling", EndUse: "cooling", Carrier: "district_cooling", Tabular: &selector}
	proof, err := epathSQLOriginalTabular(site, *observation)
	if err != nil {
		t.Fatal(err)
	}
	source := EnergyDataSource{ID: "opaque-tabular-cooling", SourceType: "sql_tabular", IsMeter: true, Name: "Cooling:DistrictCooling", KeyValue: "Cooling:DistrictCooling", Units: "kWh", SourceUnit: "kWh", NormalizedUnit: "kWh", ReportingFrequency: "Annual", AggregationMethod: "tabular_annual_value", AggregationBasis: "model_total", TableName: "End Uses", RowName: "Cooling", ColumnName: "District Cooling [kWh]", RawValue: observation.Quantity.Value, EffectiveValue: observation.Quantity.Value}
	return site, *observation, proof, source
}

func epathSQLOriginalLoadDetailUnitFixture() (epathSQLLoadDetailIdentity, EnergyDataSource) {
	detail := epathSQLLoadDetailIdentity{ZoneName: "Office", Service: "cooling", OwnerName: "Ideal Office", Source: epathRealSQLSource{DictionaryIndex: 7, Name: "Zone Ideal Loads Supply Air Latent Cooling Energy", KeyValue: "Ideal Office", SourceUnit: "J", ReportingFrequency: "Monthly", Rows: 12}}
	for month := 1; month <= 12; month++ {
		detail.Source.Months = append(detail.Source.Months, epathRealSQLMonth{Month: month, Rows: 1, RawSum: epathOracleNumber(3 * 3600000), EnergyKWh: epathOracleNumber(3)})
	}
	source := EnergyDataSource{ID: "sql-rdd-7", SourceType: "sql_report_data", Name: detail.Source.Name, KeyValue: detail.OwnerName, ZoneName: detail.ZoneName, Units: "J", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", DriverRole: "context", DriverCategory: "load.cooling", DriverComponent: "load.dehumidification", HeatDirection: "cooling", InspectorSection: "Breakdown", AggregationMethod: "sum_report_data", AggregationBasis: "model_total", MultiplierApplication: "already_model_total", EffectiveMultiplier: 1, AllocationFactor: 1, RawValue: 36, EffectiveValue: 36}
	return detail, source
}

func TestEnergyPathRealSQLOriginalSourceNonAdditiveDetailBinding(t *testing.T) {
	detail, source := epathSQLOriginalLoadDetailUnitFixture()
	proof, err := epathSQLOriginalLoadDetail(detail)
	if err != nil {
		t.Fatal(err)
	}
	key, err := epathSQLOriginalKey(proof)
	if err != nil || key != "sql-rdd-7" {
		t.Fatal("non-additive context lost original dictionary identity")
	}
	allowed := map[string]epathSQLOriginalSource{key: proof}
	for _, period := range []string{"annual", "M1", "M12"} {
		if err := epathSQLVerifyOriginalSources([]string{source.ID}, map[string]EnergyDataSource{source.ID: source}, allowed, map[string]bool{key: true}, period); err != nil {
			t.Fatal(err)
		}
	}
	for name, edit := range map[string]func(*EnergyDataSource){
		"ordinary RDD flags":  func(s *EnergyDataSource) { s.DriverRole = ""; s.InspectorSection = "" },
		"promoted driver":     func(s *EnergyDataSource) { s.DriverRole = "pressure" },
		"different equipment": func(s *EnergyDataSource) { s.KeyValue = "Other Ideal" },
		"different Zone":      func(s *EnergyDataSource) { s.ZoneName = "Other Zone" },
		"different service":   func(s *EnergyDataSource) { s.DriverCategory = "load.heating" },
		"summed context":      func(s *EnergyDataSource) { s.AggregationMethod = "sum_monthly" },
		"double multiplied":   func(s *EnergyDataSource) { s.EffectiveMultiplier = 2 },
		"derived masquerade":  func(s *EnergyDataSource) { s.InputSourceIDs = []string{"sql-rdd-1"} },
	} {
		t.Run(name, func(t *testing.T) {
			mutant := source
			edit(&mutant)
			if epathSQLOriginalSourceMatches(mutant, proof, "annual") {
				t.Fatal("non-additive context lost exact original owner/role")
			}
		})
	}
	mutant := proof
	other := *proof.RDD
	other.Name = "Zone Air System Sensible Cooling Energy"
	mutant.RDD = &other
	if _, err := epathSQLOriginalKey(mutant); err == nil {
		t.Fatal("detail annotation rebound to a different numeric RDD")
	}
}

func TestEnergyPathRealSQLOriginalSourceTypedUnion(t *testing.T) {
	_, observation, tabular, source := epathSQLOriginalUnitFixture(t, "12.34")
	tabularKey, err := epathSQLOriginalKey(tabular)
	if err != nil || !strings.HasPrefix(tabularKey, "sql-tabular:") || strings.HasPrefix(tabularKey, "sql-rdd-") {
		t.Fatalf("Tabular received a fake dictionary ID: %s %v", tabularKey, err)
	}
	rdd := epathSQLOriginalRDD(epathRealSQLSource{DictionaryIndex: 7, Name: "Sensible Cooling", KeyValue: "Office", SourceUnit: "J", ReportingFrequency: "Monthly"})
	rddKey, err := epathSQLOriginalKey(rdd)
	if err != nil || rddKey != "sql-rdd-7" {
		t.Fatal("existing original RDD identity changed")
	}
	allowed := map[string]epathSQLOriginalSource{tabularKey: tabular, rddKey: rdd}
	actual := map[string]EnergyDataSource{source.ID: source, rddKey: {ID: rddKey, SourceType: "sql_report_data", Name: "sensible cooling", KeyValue: "OFFICE", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "monthly"}}
	before := observation
	if err := epathSQLVerifyOriginalSources([]string{source.ID, rddKey}, actual, allowed, map[string]bool{tabularKey: true, rddKey: true}, "annual"); err != nil {
		t.Fatal(err)
	}
	actual["derived"] = EnergyDataSource{ID: "derived", SourceType: "calculated", InputSourceIDs: []string{source.ID, rddKey}}
	leaves, err := epathSQLOriginalSourceLeaves([]string{"derived"}, actual, allowed, "annual")
	if err != nil || len(leaves) != 2 || !leaves[tabularKey] || !leaves[rddKey] {
		t.Fatalf("mixed original leaf ownership lost: %#v %v", leaves, err)
	}
	if !reflect.DeepEqual(before, observation) {
		t.Fatal("source proof changed original annual observation")
	}
	for month := 1; month <= 12; month++ {
		period := fmt.Sprintf("M%d", month)
		if _, err := epathSQLOriginalSourceLeaves([]string{"derived"}, actual, allowed, period); err == nil {
			t.Fatalf("Annual Tabular supplied %s consumption", period)
		}
		if err := epathSQLVerifyOriginalSources([]string{source.ID}, actual, allowed, map[string]bool{tabularKey: true}, period); err == nil {
			t.Fatalf("direct Annual cell supplied %s", period)
		}
		if err := epathSQLVerifyOriginalSources([]string{rddKey}, actual, map[string]epathSQLOriginalSource{rddKey: rdd}, map[string]bool{rddKey: true}, period); err != nil {
			t.Fatalf("unchanged Monthly RDD rejected: %v", err)
		}
	}
	alias := actual[rddKey]
	alias.ID = "renamed-rdd"
	actual[alias.ID] = alias
	if _, err := epathSQLOriginalSourceLeaves([]string{alias.ID}, actual, map[string]epathSQLOriginalSource{rddKey: rdd}, "M1"); err != nil {
		t.Fatal("existing recursive metadata RDD matching changed")
	}
	if err := epathSQLVerifyOriginalSources([]string{alias.ID}, actual, allowed, map[string]bool{rddKey: true}, "M1"); err == nil {
		t.Fatal("strict direct RDD identity was relaxed")
	}
}

func TestEnergyPathRealSQLOriginalSourceRejectWireMutants(t *testing.T) {
	_, _, proof, source := epathSQLOriginalUnitFixture(t, "12.34")
	key, _ := epathSQLOriginalKey(proof)
	allowed := map[string]epathSQLOriginalSource{key: proof}
	for name, edit := range map[string]func(*EnergyDataSource){
		"type":                        func(s *EnergyDataSource) { s.SourceType = "sql_report_data" },
		"not meter":                   func(s *EnergyDataSource) { s.IsMeter = false },
		"alias name":                  func(s *EnergyDataSource) { s.Name = "Cooling:Electricity" },
		"alias key":                   func(s *EnergyDataSource) { s.KeyValue = "Heating:DistrictHeating" },
		"original unit":               func(s *EnergyDataSource) { s.SourceUnit = "GJ" },
		"display unit":                func(s *EnergyDataSource) { s.Units = "GJ" },
		"normalized unit":             func(s *EnergyDataSource) { s.NormalizedUnit = "J" },
		"monthly frequency":           func(s *EnergyDataSource) { s.ReportingFrequency = "Monthly" },
		"aggregation method":          func(s *EnergyDataSource) { s.AggregationMethod = "sum_monthly" },
		"aggregation basis":           func(s *EnergyDataSource) { s.AggregationBasis = "single_zone" },
		"fabricated Zone observation": func(s *EnergyDataSource) { s.ZoneName = "Office" },
		"different table":             func(s *EnergyDataSource) { s.TableName = "Source Energy End Use Components Summary" },
		"facility substituted":        func(s *EnergyDataSource) { s.RowName = "Total End Uses" },
		"carrier column":              func(s *EnergyDataSource) { s.ColumnName = "District Heating Steam [kWh]" },
		"column unit":                 func(s *EnergyDataSource) { s.ColumnName = "District Cooling [GJ]" },
		"derived original":            func(s *EnergyDataSource) { s.InputSourceIDs = []string{s.ID} },
	} {
		t.Run(name, func(t *testing.T) {
			mutant := source
			edit(&mutant)
			actual := map[string]EnergyDataSource{mutant.ID: mutant}
			if _, err := epathSQLOriginalSourceLeaves([]string{mutant.ID}, actual, allowed, "annual"); err == nil {
				t.Fatal("wrong original Tabular wire identity accepted")
			}
			if err := epathSQLVerifyOriginalSources([]string{mutant.ID}, actual, allowed, map[string]bool{key: true}, "annual"); err == nil {
				t.Fatal("wrong direct Tabular wire identity accepted")
			}
		})
	}
	actual := map[string]EnergyDataSource{source.ID: source}
	for _, ids := range [][]string{nil, {"missing"}, {source.ID, source.ID}} {
		if err := epathSQLVerifyOriginalSources(ids, actual, allowed, map[string]bool{key: true}, "annual"); err == nil {
			t.Fatal("missing/duplicate direct source passed")
		}
	}
	duplicate := source
	duplicate.ID = "other-opaque-ID"
	actual[duplicate.ID] = duplicate
	if err := epathSQLVerifyOriginalSources([]string{source.ID, duplicate.ID}, actual, allowed, map[string]bool{key: true}, "annual"); err == nil {
		t.Fatal("duplicated physical original cell passed under alternate IDs")
	}
	actual["cycle"] = EnergyDataSource{ID: "cycle", InputSourceIDs: []string{"cycle"}}
	if _, err := epathSQLOriginalSourceLeaves([]string{"cycle"}, actual, allowed, "annual"); err == nil {
		t.Fatal("cyclic derived trace passed")
	}
	if _, err := epathSQLOriginalSourceLeaves([]string{source.ID}, actual, map[string]epathSQLOriginalSource{"fake-rdd-key": proof}, "annual"); err == nil {
		t.Fatal("Tabular hidden in fabricated RDD identity")
	}
}

func TestEnergyPathRealSQLOriginalSourceRetainsIndependentCellProof(t *testing.T) {
	site, observation, _, _ := epathSQLOriginalUnitFixture(t, "12.34")
	for name, edit := range map[string]func(*epathSQLTabularObservation){
		"wrong report":               func(o *epathSQLTabularObservation) { o.Selector.ReportName = "SourceEnergyEndUseComponentsSummary" },
		"wrong facility":             func(o *epathSQLTabularObservation) { o.Selector.ReportForString = "Elsewhere" },
		"no original cell":           func(o *epathSQLTabularObservation) { o.TabularDataIndex = 0 },
		"missing original text":      func(o *epathSQLTabularObservation) { o.RawText = "" },
		"changed observed value":     func(o *epathSQLTabularObservation) { o.RawValue = 99 },
		"widened original precision": func(o *epathSQLTabularObservation) { o.Quantity = epathSQLBounded(12.34, 0, 100) },
		"no weather proof":           func(o *epathSQLTabularObservation) { o.Weather = epathRealSQLWeather{} },
		"wrong year":                 func(o *epathSQLTabularObservation) { o.Weather.Year = 2020 },
		"partial calendar":           func(o *epathSQLTabularObservation) { o.Weather.Months = []int{1} },
	} {
		t.Run(name, func(t *testing.T) {
			mutant := observation
			edit(&mutant)
			if _, err := epathSQLOriginalTabular(site, mutant); err == nil {
				t.Fatal("invalid original observation became source proof")
			}
		})
	}
	for name, edit := range map[string]func(*epathRealSQLSite){
		"wrong carrier":    func(s *epathRealSQLSite) { s.Carrier = "district_heating" },
		"wrong end use":    func(s *epathRealSQLSite) { s.EndUse = "heating" },
		"claimed facility": func(s *epathRealSQLSite) { s.Facility = true },
	} {
		t.Run(name, func(t *testing.T) {
			mutant := site
			edit(&mutant)
			if _, err := epathSQLOriginalTabular(mutant, observation); err == nil {
				t.Fatal("wrong site taxonomy bound to cell")
			}
		})
	}
	_, zero, proof, source := epathSQLOriginalUnitFixture(t, "0.00")
	if zero.RawValue != 0 || zero.Quantity.Value != 0 || !zero.Quantity.includesZero() || !epathSQLOriginalSourceMatches(source, proof, "annual") {
		t.Fatal("observed literal zero treated as missing or relaxed monthly source")
	}
	if _, err := epathSQLOriginalKey(epathSQLOriginalSource{}); err == nil {
		t.Fatal("missing union identity accepted")
	}
	proof.RDD = &epathRealSQLSource{DictionaryIndex: 1, Name: "Energy", SourceUnit: "J", ReportingFrequency: "Monthly"}
	if _, err := epathSQLOriginalKey(proof); err == nil {
		t.Fatal("mixed RDD/Tabular proof accepted")
	}
}
