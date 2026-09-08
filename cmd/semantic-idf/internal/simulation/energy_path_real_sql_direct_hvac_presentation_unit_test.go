package simulation

import (
	"encoding/json"
	"testing"
)

func TestEnergyPathSQLDirectHVACPresentationFromOriginalSQL(t *testing.T) {
	path, model, _ := epathSQLDirectHVACSourceUnitFixture(t)
	epathOracleEditSQL(t, path, `UPDATE Zones SET Multiplier=1,ListMultiplier=1;
UPDATE ReportDataDictionary SET Name='Zone Air System Sensible Heating Energy' WHERE ReportDataDictionaryIndex=11;
UPDATE ReportData SET Value=0.10993587350426377*3600000 WHERE ReportDataDictionaryIndex=11 AND TimeIndex=1;
UPDATE ReportData SET Value=-20*3600000 WHERE ReportDataDictionaryIndex=14 AND TimeIndex=1;
UPDATE ReportData SET Value=0.11024376310208626*3600000 WHERE ReportDataDictionaryIndex=42 AND TimeIndex=1;
UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=43;`)
	model.Loads[1].Source.Alternatives[0].Name = "Zone Air System Sensible Heating Energy"
	model.Services = []epathRealSQLService{{Service: "heating", SiteIDs: []string{"heating.electricity", "heating.natural_gas"}}}
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	before, _ := json.Marshal(frames)
	pair, err := epathSQLDirectHVACDisplayedPair(frames, model, "Office", "heating", 1)
	if err != nil || pair.From.Value != .110 || pair.To.Value != .110 {
		t.Fatalf("original source quantization: pair=%+v err=%v", pair, err)
	}
	kind, err := epathSQLConversionRatioKind("efficiency", pair.From, pair.To)
	if err != nil || kind != "efficiency" {
		t.Fatalf("independent displayed equality must be efficiency: %s/%v", kind, err)
	}
	rawFrom := frames.Loads[epathSQLKey("Office", "heating", 1)]
	rawTo := frames.DirectHVAC[epathSQLDirectHVACKey("Office", "heating", "natural_gas", 1)].Quantity
	if _, err := epathSQLConversionRatioKind("efficiency", rawFrom, rawTo); err == nil {
		t.Fatal("fixture must retain the original broad-interval classification ambiguity")
	}
	after, _ := json.Marshal(frames)
	if string(before) != string(after) || rawFrom.Value == .110 || rawTo.Value == .110 {
		t.Fatal("display proof changed raw SQL observations or their intervals")
	}
	delete(frames.DirectHVAC, epathSQLDirectHVACKey("Office", "heating", "electricity", 1))
	if _, err := epathSQLDirectHVACDisplayedPair(frames, model, "Office", "heating", 1); err == nil {
		t.Fatal("missing zero carrier was mistaken for a complete direct observation")
	}
}

func TestEnergyPathSQLDirectHVACExactPresentationDoesNotWidenBounds(t *testing.T) {
	bundle, check := epathSQLModelConversionFixture()
	check.Item.Scope, check.Item.Zone, check.Item.Period = "zone", "Office", "M1"
	check.Item.Target.Service, check.Item.Target.Basis, check.Item.Target.RatioKind = "heating", "direct_zone_energy", "efficiency"
	check.Conversion = &epathSQLConversionProof{
		From: epathSQLQuantity{Value: .10993587350426377, Error: .001}, To: epathSQLQuantity{Value: .11024376310208626, Error: .001},
		ExactPresentation: &epathSQLConversionProof{From: epathSQLQuantity{Value: .110}, To: epathSQLQuantity{Value: .110}},
	}
	check.Quantity = &epathSQLQuantity{Value: check.Conversion.From.Value / check.Conversion.To.Value}
	nodes, links := bundle.EnergyExplanation.Nodes, bundle.EnergyExplanation.Links
	for index := range nodes {
		nodes[index].ServiceKind, nodes[index].ZoneName, nodes[index].Period, nodes[index].Value = "heating", "Office", "M1", .110
	}
	links[0].ServiceKind, links[0].Basis, links[0].ZoneName, links[0].Period = "heating", "direct_zone_energy", "Office", "M1"
	links[0].FromValue, links[0].ToValue, links[0].Ratio, links[0].RatioKind = .110, .110, 1, "efficiency"
	bundle.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}, Periods: []EnergyPeriod{{ID: "M1", Nodes: nodes, Links: links}}}}
	if err := epathCheckSQLModelConversion(bundle, check); err != nil {
		t.Fatalf("exact independent displayed pair: %v", err)
	}
	link := &bundle.EnergyExplanation.ZoneResults[0].Periods[0].Links[0]
	for _, mutate := range []func(*EnergyPathLink){
		func(l *EnergyPathLink) { l.RatioKind = "load_to_fuel" },
		func(l *EnergyPathLink) {
			l.ToValue = .1095
			l.Ratio = l.FromValue / l.ToValue
			l.RatioKind = "efficiency"
		},
		func(l *EnergyPathLink) { l.FromValue = .5; l.Ratio = l.FromValue / l.ToValue },
		func(l *EnergyPathLink) { l.Ratio = check.Quantity.Value },
	} {
		original := *link
		mutate(link)
		if err := epathCheckSQLModelConversion(bundle, check); err == nil {
			t.Fatal("exact display proof accepted a wrong label, raw quotient, widened value or non-grid endpoint")
		}
		*link = original
	}
	check.Conversion.From = epathSQLQuantity{Value: .0006, Error: .001}.positive()
	check.Conversion.To = epathSQLQuantity{Value: .0007, Error: .001}.positive()
	check.Conversion.ExactPresentation = &epathSQLConversionProof{From: epathSQLQuantity{Value: .001}, To: epathSQLQuantity{Value: .001}}
	bundle.EnergyExplanation.ZoneResults[0].Periods[0].Links = nil
	if err := epathCheckSQLModelConversion(bundle, check); err == nil {
		t.Fatal("broad raw intervals pruned a pair whose original-source display proof requires positive endpoints")
	}
}
