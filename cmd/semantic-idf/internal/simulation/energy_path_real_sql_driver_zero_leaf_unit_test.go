package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

// Independent hand SQL: existing pressure1 plus an explicitly observed zero
// second term. The hand graph and all 18/14.4/3.6 quantities remain unchanged.
func epathSQLDriverZeroLeafFixture(t *testing.T, cancellation ...bool) (epathSQLFrames, epathRealSQLModel, PurposeResultBundle) {
	t.Helper()
	_, _, _, bundle := epathSQLDriverLinksFixture(t)
	path, model := epathSQLModelUnitFixture(t)
	epathOracleEditSQL(t, path, `INSERT INTO ReportDataDictionary VALUES(15,'Zero People Gain','Office',0,'Monthly','J','Zone');
INSERT INTO ReportData SELECT 30000+TimeIndex,15,TimeIndex,0 FROM ReportData WHERE ReportDataDictionaryIndex=14`)
	model.Families[0].Terms = append(model.Families[0].Terms, epathRealSQLTerm{Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Zero People Gain", Unit: "J"}}, Keys: []string{"Office"}}, Sign: 1})
	if len(cancellation) > 0 && cancellation[0] {
		epathOracleEditSQL(t, path, `UPDATE ReportData SET Value=CASE TimeIndex WHEN 1 THEN 3600000 WHEN 2 THEN -3600000 ELSE 0 END WHERE ReportDataDictionaryIndex=15;
INSERT INTO ReportDataDictionary VALUES(16,'Opposite People Gain','Office',0,'Monthly','J','Zone');
INSERT INTO ReportData SELECT 40000+TimeIndex,16,TimeIndex,-Value FROM ReportData WHERE ReportDataDictionaryIndex=15`)
		model.Families[0].Terms = append(model.Families[0].Terms, epathRealSQLTerm{Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Opposite People Gain", Unit: "J"}}, Keys: []string{"Office"}}, Sign: 1})
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: "zero-source", SourceType: "sql_report_data", Name: "Zero People Gain", KeyValue: "Office", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"})
	return frames, model, bundle
}

func TestEnergyPathRealSQLDriverLinksObservedZeroCompoundLeaf(t *testing.T) {
	frames, model, bundle := epathSQLDriverZeroLeafFixture(t)
	before, _ := json.Marshal(frames)
	var checks epathSQLModelChecks
	if err := epathSQLModelDriverLinkChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	for _, check := range checks.Rows {
		if err := epathCheckSQLModelDriverLink(bundle, check); err != nil {
			t.Fatalf("%s: %v", check.Want.Key, err)
		}
		if check.DriverLink.Service == "cooling" && check.DriverLink.Category == "internal.people" {
			if !reflect.DeepEqual(check.DriverLink.DriverSources, []int{14, 15}) || !reflect.DeepEqual(check.DriverLink.RequiredDriverSources, []int{14}) {
				t.Fatalf("zero observation must stay allowed without becoming a required positive contributor: %+v", check.DriverLink)
			}
		}
	}
	// The same non-additive zero leaf is also valid when explicitly retained.
	r := &bundle.EnergyExplanation
	retain := func(nodes []EnergyExplanationNode, links []EnergyPathLink) {
		nodes[1].SourceIDs = append(nodes[1].SourceIDs, "zero-source")
		links[1].SourceIDs = append(links[1].SourceIDs, "zero-source")
	}
	retain(r.Nodes, r.Links)
	for i := range r.Periods {
		retain(r.Periods[i].Nodes, r.Periods[i].Links)
	}
	for z := range r.ZoneResults {
		retain(r.ZoneResults[z].Nodes, r.ZoneResults[z].Links)
		for i := range r.ZoneResults[z].Periods {
			retain(r.ZoneResults[z].Periods[i].Nodes, r.ZoneResults[z].Periods[i].Links)
		}
	}
	for _, check := range checks.Rows {
		if err := epathCheckSQLModelDriverLink(bundle, check); err != nil {
			t.Fatalf("explicit zero leaf %s: %v", check.Want.Key, err)
		}
	}
	// Source metrics are separate mandatory checks, including known-zero values.
	observed := []epathRealSQLSource{}
	for _, source := range frames.SourceIdentities {
		observed = append(observed, source)
	}
	var sources epathSQLModelChecks
	if err := epathSQLModelSourceChecks(frames, observed, model, &sources); err != nil {
		t.Fatal(err)
	}
	zeroChecks := 0
	for _, check := range sources.Rows {
		if check.Item.Target.SourceName == "Zero People Gain" {
			zeroChecks++
			if check.Quantity == nil || check.Quantity.Value != 0 || check.Quantity.Error != 0 || check.OptionalPresentation || check.Item.Target.AllowPrunedZero {
				t.Fatalf("observed zero source coverage weakened: %+v", check)
			}
		}
	}
	if zeroChecks != 4 {
		t.Fatalf("source raw/effective x Building/Zone checks=%d, want4", zeroChecks)
	}
	after, _ := json.Marshal(frames)
	if string(before) != string(after) {
		t.Fatal("proof construction changed independent native frames")
	}
}

func TestEnergyPathRealSQLDriverLinksZeroLeafFailsClosed(t *testing.T) {
	frames, model, _ := epathSQLDriverZeroLeafFixture(t)
	for name, edit := range map[string]func(*epathSQLFrames){
		"missing dictionary":        func(f *epathSQLFrames) { delete(f.SourceIdentities, 15) },
		"missing raw frame":         func(f *epathSQLFrames) { delete(f.SourceRaw, 15) },
		"missing effective frame":   func(f *epathSQLFrames) { delete(f.SourceEffective, 15) },
		"foreign owner":             func(f *epathSQLFrames) { f.SourceZone[15] = "Other" },
		"raw frame fabricated zero": func(f *epathSQLFrames) { f.SourceRaw[15][0].Error = .001 },
		"effective frame nonzero":   func(f *epathSQLFrames) { f.SourceEffective[15][0].Value = 1 },
		"wrong dictionary index": func(f *epathSQLFrames) {
			s := f.SourceIdentities[15]
			s.DictionaryIndex = 99
			f.SourceIdentities[15] = s
		},
		"wrong unit": func(f *epathSQLFrames) { s := f.SourceIdentities[15]; s.SourceUnit = "W"; f.SourceIdentities[15] = s },
		"wrong frequency": func(f *epathSQLFrames) {
			s := f.SourceIdentities[15]
			s.ReportingFrequency = "Hourly"
			f.SourceIdentities[15] = s
		},
		"meter": func(f *epathSQLFrames) { s := f.SourceIdentities[15]; s.IsMeter = true; f.SourceIdentities[15] = s },
		"missing month": func(f *epathSQLFrames) {
			s := f.SourceIdentities[15]
			s.Months = s.Months[:11]
			f.SourceIdentities[15] = s
		},
		"wrong month":               func(f *epathSQLFrames) { f.SourceIdentities[15].Months[0].Month = 2 },
		"duplicate native rows":     func(f *epathSQLFrames) { f.SourceIdentities[15].Months[0].Rows = 2 },
		"NULL row":                  func(f *epathSQLFrames) { f.SourceIdentities[15].Months[0].MissingRows = 1 },
		"missing native raw":        func(f *epathSQLFrames) { f.SourceIdentities[15].Months[0].RawSum = nil },
		"missing native energy":     func(f *epathSQLFrames) { f.SourceIdentities[15].Months[0].EnergyKWh = nil },
		"nonfinite native":          func(f *epathSQLFrames) { f.SourceIdentities[15].Months[0].EnergyKWh = epathOracleNumber(math.NaN()) },
		"tiny positive is not zero": func(f *epathSQLFrames) { f.SourceIdentities[15].Months[0].EnergyKWh = epathOracleNumber(.00000001) },
		"negative is not zero":      func(f *epathSQLFrames) { f.SourceIdentities[15].Months[0].EnergyKWh = epathOracleNumber(-1) },
		"raw underflow is not zero": func(f *epathSQLFrames) {
			f.SourceIdentities[15].Months[0].RawSum = epathOracleNumber(math.SmallestNonzeroFloat64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			data, _ := json.Marshal(frames)
			var changed epathSQLFrames
			if err := json.Unmarshal(data, &changed); err != nil {
				t.Fatal(err)
			}
			edit(&changed)
			if epathSQLDriverObservedZeroLeaf(changed, model, changed.Cells[epathSQLKey("Office", "people", 1)], 15) {
				t.Fatal("unproved or nonzero leaf became optional")
			}
		})
	}
}

func TestEnergyPathRealSQLDriverLinksRequiredLeafUsesMonthlyNotAnnualZero(t *testing.T) {
	frames, model, bundle := epathSQLDriverZeroLeafFixture(t, true)
	// Handwritten +1/-1 months cancel annually, but both are physical inputs.
	// An opposite paired term keeps this completed family's pressure unchanged.
	for _, id := range []int{15, 16} {
		if source := frames.SourceIdentities[id]; source.EnergyKWh == nil || *source.EnergyKWh != 0 {
			t.Fatal("handwritten annual signed cancellation changed")
		}
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelDriverLinkChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	for _, check := range checks.Rows {
		if check.DriverLink.Service != "cooling" || check.DriverLink.Category != "internal.people" {
			continue
		}
		want := check.Item.Period == "annual" || check.Item.Period == "M1" || check.Item.Period == "M2"
		for _, requiredID := range []int{15, 16} {
			found := false
			for _, id := range check.DriverLink.RequiredDriverSources {
				found = found || id == requiredID
			}
			if found != want {
				t.Fatalf("%s: annual cancellation changed monthly required leaf%d: %v", check.Want.Key, requiredID, check.DriverLink.RequiredDriverSources)
			}
		}
		err := epathCheckSQLModelDriverLink(bundle, check)
		if want && (err == nil || !strings.Contains(err.Error(), fmt.Sprintf("SQL source %d", 15))) {
			t.Fatalf("nonzero leaf deletion not rejected: %v", err)
		}
		if !want && err != nil {
			t.Fatalf("other exact zero month became required: %v", err)
		}
	}
}
