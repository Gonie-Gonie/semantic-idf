package simulation

import (
	"math"
	"testing"
)

func epathDirectUseUnitInputs() ([]epathRealSQLSource, epathSQLFrames, epathRealSQLModel) {
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{"a": {"A", 10}, "b": {"B", 1}, "plenum": {"Plenum", 1}}}
	model := epathRealSQLModel{Precision: epathRealSQLPrecision{3, 2, 2}, DirectUses: []epathRealSQLDirectUse{{EndUse: "lighting", Carrier: "electricity", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{"Zone Lights Electricity Energy", "J"}}, Keys: []string{"A", "B"}}}}}
	sources := []epathRealSQLSource{}
	for index, key := range []string{"A", "B"} {
		source := epathRealSQLSource{DictionaryIndex: index + 1, Name: "Zone Lights Electricity Energy", KeyValue: key, SourceUnit: "J", ReportingFrequency: "Monthly"}
		for month := 1; month <= 12; month++ {
			source.Months = append(source.Months, epathRealSQLMonth{Month: month, Rows: 1, EnergyKWh: epathOracleNumber(float64(2 - index))})
		}
		sources = append(sources, source)
	}
	return sources, frames, model
}

func epathDirectUseUnitCandidate() PurposeResultBundle {
	zone := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "A", AggregationBasis: "model_total"}}
	zone.Nodes = []EnergyExplanationNode{
		{ID: "lighting", Level: "end_use", EndUse: "lighting", ZoneName: "A", Period: "annual", Value: 240, RawValue: 24, EffectiveValue: 240, AllocatedValue: 240, Basis: "direct_zone_energy", Unit: "kWh", ScaleDomain: "site", AggregationBasis: "model_total", Multiplier: 10, SourceIDs: []string{"sql-rdd-1"}},
		{ID: "electricity", Level: "carrier", Carrier: "electricity", ZoneName: "A", Period: "annual", Value: 240, Basis: "reported_meter", Unit: "kWh", ScaleDomain: "site"},
	}
	zone.Links = []EnergyPathLink{{ID: "direct", FromID: "lighting", ToID: "electricity", Relation: "direct_end_use_to_carrier", Basis: "direct_zone_energy", FromValue: 240, ToValue: 240, FromUnit: "kWh", ToUnit: "kWh", Period: "annual", ZoneName: "A", SourceIDs: []string{"sql-rdd-1"}}}
	return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, ZoneResults: []EnergyExplanationZoneResult{zone}, Sources: []EnergyDataSource{
		{ID: "sql-rdd-1", SourceType: "sql_report_data", Name: "Zone Lights Electricity Energy", KeyValue: "A", SourceUnit: "J", ReportingFrequency: "Monthly"},
		{ID: "sql-rdd-2", SourceType: "sql_report_data", Name: "Zone Lights Electricity Energy", KeyValue: "B", SourceUnit: "J", ReportingFrequency: "Monthly"},
	}}}
}

func TestEnergyPathRealSQLDirectUseMultiplierAndExactTrace(t *testing.T) {
	sources, frames, model := epathDirectUseUnitInputs()
	checks := epathSQLModelChecks{}
	if err := epathSQLModelDirectUseChecks(sources, frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 164 {
		t.Fatalf("missing Zone/month/source obligations: %d", len(checks.Rows))
	}
	var selected epathSQLModelCheck
	for _, check := range checks.Rows {
		if check.Item.Zone == "A" && check.Item.Period == "annual" && check.Item.Target.Collection == "nodes" {
			want := 240.0
			if check.Item.Target.Field == "rawValue" {
				want = 24
			}
			if check.Quantity == nil || check.Quantity.Value != want {
				t.Fatalf("raw/effective multiplier expectation wrong: %+v", check)
			}
			if check.DirectUse != nil {
				selected = check
			}
		}
	}
	if selected.DirectUse == nil {
		t.Fatal("direct-use endpoint proof missing")
	}
	for _, test := range []struct {
		name   string
		mutate func(*PurposeResultBundle)
	}{
		{"good", nil},
		{"swapped Zone contribution", func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Nodes[0].Value = 12 }},
		{"wrong multiplier", func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Nodes[0].Multiplier = 1 }},
		{"wrong basis", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Nodes[0].Basis = "zone_load_allocation"
		}},
		{"thermal node service", func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Nodes[0].ServiceKind = "heating" }},
		{"thermal link service", func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Links[0].ServiceKind = "heating" }},
		{"fake conversion", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Links[0].Ratio = 1
			b.EnergyExplanation.ZoneResults[0].Links[0].RatioKind = "COP"
		}},
		{"fake ratio label", func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Links[0].RatioLabel = "COP 1" }},
		{"wrong domain", func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Nodes[1].ScaleDomain = "thermal" }},
		{"missing link", func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Links = nil }},
		{"unequal site endpoints", func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Links[0].ToValue = 239 }},
		{"unequal inside SQL bounds", func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Links[0].ToValue = 240.001 }},
		{"other real Zone source", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Links[0].SourceIDs = []string{"sql-rdd-2"}
		}},
		{"wrong carrier", func(b *PurposeResultBundle) { b.EnergyExplanation.ZoneResults[0].Nodes[1].Carrier = "natural_gas" }},
		{"duplicate link", func(b *PurposeResultBundle) {
			row := b.EnergyExplanation.ZoneResults[0].Links[0]
			row.ID = "extra"
			b.EnergyExplanation.ZoneResults[0].Links = append(b.EnergyExplanation.ZoneResults[0].Links, row)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := epathDirectUseUnitCandidate()
			if test.mutate != nil {
				test.mutate(&bundle)
			}
			failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, bundle, epathSQLModelChecks{Rows: []epathSQLModelCheck{selected}})
			if (len(failures) == 0) != (test.mutate == nil) {
				t.Fatalf("unexpected failures: %+v", failures)
			}
		})
	}
}

func TestEnergyPathRealSQLDirectUseUnknownIsNotZero(t *testing.T) {
	for _, name := range []string{"missing month", "NULL month", "negative", "nonfinite", "undeclared owner", "wildcard", "missing key"} {
		t.Run(name, func(t *testing.T) {
			sources, frames, model := epathDirectUseUnitInputs()
			switch name {
			case "missing month":
				sources[0].Months = sources[0].Months[:11]
			case "NULL month":
				sources[0].Months[0].EnergyKWh = nil
			case "negative":
				sources[0].Months[0].EnergyKWh = epathOracleNumber(-1)
			case "nonfinite":
				sources[0].Months[0].EnergyKWh = epathOracleNumber(math.Inf(1))
			case "undeclared owner":
				extra := sources[0]
				extra.DictionaryIndex = 9
				extra.KeyValue = "Plenum"
				sources = append(sources, extra)
			case "wildcard":
				model.DirectUses[0].Source.Keys = []string{"*"}
			case "missing key":
				sources = sources[:1]
			}
			if err := epathSQLModelDirectUseChecks(sources, frames, model, &epathSQLModelChecks{}); err == nil {
				t.Fatal("unknown/unreviewed direct-use evidence accepted")
			}
		})
	}
}
