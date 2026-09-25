package simulation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestEnergyPathSQLPVRecipeKeepsCGIdentityInCanonicalOther(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "oracles", "pv-storage-25-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recipe epathRealOracleRecipe
	if err := json.Unmarshal(raw, &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.SQLModel == nil {
		t.Fatal("full native recipe missing")
	}
	count := 0
	for _, site := range recipe.SQLModel.Site {
		if site.ID != "cogeneration.electricity" {
			continue
		}
		count++
		if site.EndUse != "other" || site.Carrier != "electricity" || site.Facility || !site.Source.IsMeter || len(site.Source.Keys) != 1 || site.Source.Keys[0] != "" || len(site.Source.Alternatives) != 1 || site.Source.Alternatives[0].Name != "Cogeneration:Electricity" || site.Source.Alternatives[0].Unit != "J" {
			t.Fatal("canonical Other presentation changed exact consumed CG source authority")
		}
	}
	if count != 1 || recipe.SQLModel.PVCogeneration == nil || recipe.SQLModel.PVCogeneration.SystemID != "shop-pv-dc-storage" {
		t.Fatal("CG parent declaration missing/duplicated")
	}
	// Availability identities classify source groups, not presentation nodes.
	found := false
	for _, group := range recipe.SQLModel.Availability {
		if group.ID == "cogeneration.electricity" && group.Stage == "endUses" && group.IsMeter && len(group.RequestedNames) == 1 && group.RequestedNames[0] == "Cogeneration:Electricity" {
			found = true
		}
	}
	if !found {
		t.Fatal("presentation correction erased the consumed CG availability group")
	}
}

// Independent literal inputs: native Monthly parent4 in M1, zero in M2..12;
// optional unrelated Other2 also in M1. Ancillary4 is context, never another
// site declaration. Canonical Other may aggregate independent consumed inputs.
func epathSQLPVCGOtherHand(t *testing.T, withOther bool) (PurposeResultBundle, epathSQLModelChecks) {
	t.Helper()
	frames := epathSQLFrames{Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{}}
	model := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	for _, item := range []struct {
		id, name string
		index    int
		amount   float64
	}{
		{"cogeneration.electricity", "Cogeneration:Electricity", 61, 4},
		{"unrelated.electricity", "OtherConsumed:Electricity", 63, 2},
	} {
		if item.index == 63 && !withOther {
			continue
		}
		source := epathRealSQLSource{DictionaryIndex: item.index, Name: item.name, SourceUnit: "J", ReportingFrequency: "Monthly", IsMeter: true}
		for month := 1; month <= 12; month++ {
			value := 0.0
			if month == 1 {
				value = item.amount
			}
			source.Months = append(source.Months, epathRealSQLMonth{Month: month, Rows: 1, RawSum: epathOracleNumber(value * 3600000), EnergyKWh: epathOracleNumber(value)})
			frames.Site[item.id] = append(frames.Site[item.id], &epathSQLQuantity{Value: value})
		}
		frames.SiteSources[item.id], frames.SourceIdentities[item.index] = []int{item.index}, source
		model.Site = append(model.Site, epathRealSQLSite{ID: item.id, EndUse: "other", Carrier: "electricity"})
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", item.index), Name: item.name, SourceType: "sql_report_data", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", IsMeter: true})
	}
	bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, EnergyDataSource{ID: "sql-rdd-62", Name: "Inverter Ancillary AC Electricity Energy", KeyValue: "INV", SourceType: "sql_report_data", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"})
	for _, period := range epathSQLZoneCarrierPeriods() {
		p := EnergyPeriod{ID: period, Kind: "monthly"}
		if period == "annual" {
			p.Kind = "annual"
		}
		if period == "annual" || period == "M1" {
			value, ids := 4.0, []string{"sql-rdd-61"}
			if withOther {
				value, ids = 6, []string{"sql-rdd-61", "sql-rdd-63"}
			}
			p.Nodes = []EnergyExplanationNode{
				{ID: "other", Level: "end_use", Kind: "energy.other", EndUse: "other", Value: value, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", AggregationBasis: "model_total", Period: period, SourceIDs: ids},
				{ID: "electricity", Level: "carrier", Carrier: "electricity", Value: 10, Unit: "kWh", ScaleDomain: "site", Basis: "reported_meter", AggregationBasis: "model_total", Period: period},
			}
			p.Links = []EnergyPathLink{{ID: "other-to-electricity", FromID: "other", ToID: "electricity", Relation: "direct_end_use_to_carrier", Basis: "reported_meter", FromValue: value, ToValue: value, FromUnit: "kWh", ToUnit: "kWh", Period: period, SourceIDs: ids}}
		}
		if period == "annual" {
			bundle.EnergyExplanation.Nodes, bundle.EnergyExplanation.Links = p.Nodes, p.Links
		}
		bundle.EnergyExplanation.Periods = append(bundle.EnergyExplanation.Periods, p)
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelSiteChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelSiteFlowChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	return bundle, checks
}

func TestEnergyPathSQLPVCGCanonicalOtherUsesParentOnce(t *testing.T) {
	for _, withOther := range []bool{false, true} {
		b, checks := epathSQLPVCGOtherHand(t, withOther)
		if len(checks.Rows) != 65 {
			t.Fatal("canonical aggregation lost thirteen scalar and fifty-two paired endpoint checks")
		}
		if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, b, checks); len(failures) != 0 {
			t.Fatal(failures)
		}
	}
	for _, test := range []struct {
		name   string
		mutate func(*PurposeResultBundle)
	}{
		{"legacy category", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[0].EndUse = "cogeneration_input" }},
		{"ancillary replaces parent", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes[0].SourceIDs = []string{"sql-rdd-62"}
			b.EnergyExplanation.Links[0].SourceIDs = []string{"sql-rdd-62"}
		}},
		{"double-count ancillary", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes[0].Value = 8
			b.EnergyExplanation.Links[0].FromValue, b.EnergyExplanation.Links[0].ToValue = 8, 8
		}},
		{"renamed native source", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].Name = "ElectricityProduced:Facility" }},
		{"supply edge", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].Relation = "support_supply" }},
		{"unproved source union", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes[0].SourceIDs = append(b.EnergyExplanation.Nodes[0].SourceIDs, "sql-rdd-62")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, checks := epathSQLPVCGOtherHand(t, false)
			test.mutate(&b)
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, b, checks); len(failures) == 0 {
				t.Fatal("CG consumed-input authority changed under Other label")
			}
		})
	}
}
