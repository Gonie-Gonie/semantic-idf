package simulation

import (
	"fmt"
	"reflect"
	"testing"
)

func epathSQLAnnualZoneDetailUnitFrames(t *testing.T) (epathSQLFrames, epathRealSQLModel) {
	t.Helper()
	frames, model := epathSQLAnnualZoneUnitFrames(t)
	frames.LoadDetailSourceIDs = map[string][]int{}
	frames.LoadDetailIdentities = map[int]epathSQLLoadDetailIdentity{}
	for _, actual := range epathSQLLoadDetailUnitSources() {
		var id int
		fmt.Sscanf(actual.ID, "sql-rdd-%d", &id)
		service := "cooling"
		if id >= 42 {
			service = "heating"
		}
		source := epathRealSQLSource{DictionaryIndex: id, Name: actual.Name, KeyValue: "A Ideal", SourceUnit: actual.SourceUnit, ReportingFrequency: "Monthly"}
		frames.SourceIdentities[id] = source
		frames.LoadDetailIdentities[id] = epathSQLLoadDetailIdentity{ZoneName: "A", Service: service, OwnerName: "A Ideal", Source: source}
		for month := 1; month <= 12; month++ {
			key := epathSQLKey("A", service, month)
			frames.LoadDetailSourceIDs[key] = epathSQLDictionaryUnion(frames.LoadDetailSourceIDs[key], []int{id})
		}
	}
	return frames, model
}

func epathSQLAnnualZoneDetailUnitBundle() PurposeResultBundle {
	b := epathSQLAnnualZoneUnitBundle()
	for _, source := range epathSQLLoadDetailUnitSources() {
		source.ZoneName, source.KeyValue = "A", "A Ideal"
		source.ScopeDetails = nil
		b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, source)
	}
	z := &b.EnergyExplanation.ZoneResults[0]
	add := func(nodes []EnergyExplanationNode, links []EnergyPathLink) {
		for i := range nodes {
			if nodes[i].Level == "end_use" {
				ids := []string{"sql-rdd-40", "sql-rdd-41"}
				if nodes[i].EndUse == "heating" {
					ids = []string{"sql-rdd-42", "sql-rdd-43"}
				}
				nodes[i].SourceIDs = append(append([]string(nil), nodes[i].SourceIDs...), ids...)
			}
		}
		for i := range links {
			if links[i].Relation == "load_to_end_use" {
				ids := []string{"sql-rdd-40", "sql-rdd-41"}
				if links[i].ServiceKind == "heating" {
					ids = []string{"sql-rdd-42", "sql-rdd-43"}
				}
				links[i].SourceIDs = append(append([]string(nil), links[i].SourceIDs...), ids...)
			}
		}
	}
	add(z.Nodes, z.Links)
	add(z.Periods[0].Nodes, z.Periods[0].Links)
	return b
}

func TestEnergyPathRealSQLAnnualZoneNonAdditiveDetailTrace(t *testing.T) {
	frames, model := epathSQLAnnualZoneDetailUnitFrames(t)
	var checks epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, epathSQLAnnualZoneDetailUnitBundle(), checks); len(failures) > 0 {
		t.Fatalf("explicit annual detail trace rejected: %+v", failures[:min(5, len(failures))])
	}
	frames.LoadDetailSourceIDs, frames.LoadDetailIdentities = nil, nil
	var baseline epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &baseline); err != nil {
		t.Fatal(err)
	}
	if len(baseline.Rows) != len(checks.Rows) {
		t.Fatal("context changed numeric obligation count")
	}
	for i, c := range checks.Rows {
		if !reflect.DeepEqual(c.Want, baseline.Rows[i].Want) || !reflect.DeepEqual(c.Quantity, baseline.Rows[i].Quantity) || !reflect.DeepEqual(c.Conversion, baseline.Rows[i].Conversion) {
			t.Fatal("detail changed annual share/paired quantity or invented monthly energy")
		}
		if c.ZoneService == nil {
			continue
		}
		if !reflect.DeepEqual(c.ZoneService.LoadSources, baseline.Rows[i].ZoneService.LoadSources) {
			t.Fatal("detail replaced selected load authority")
		}
		want := 0
		if c.Item.Period == "annual" && c.Item.Zone == "A" {
			want = 2
		}
		if len(c.ZoneService.LoadDetails) != want {
			t.Fatalf("wrong scoped detail allowance in %s", c.Want.Key)
		}
	}
}

func TestEnergyPathRealSQLAnnualZoneNonAdditiveDetailRejectContextLeak(t *testing.T) {
	frames, model := epathSQLAnnualZoneDetailUnitFrames(t)
	var checks epathSQLModelChecks
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PurposeResultBundle){
		"wrong owner": func(b *PurposeResultBundle) {
			for i := range b.EnergyExplanation.Sources {
				if b.EnergyExplanation.Sources[i].ID == "sql-rdd-40" {
					b.EnergyExplanation.Sources[i].KeyValue = "B Ideal"
				}
			}
		},
		"main flow disguise": func(b *PurposeResultBundle) {
			for i := range b.EnergyExplanation.Sources {
				if b.EnergyExplanation.Sources[i].ID == "sql-rdd-40" {
					b.EnergyExplanation.Sources[i].DriverRole = "main_flow"
				}
			}
		},
		"context used as carrier consumption": func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Links[1].SourceIDs = append(b.EnergyExplanation.ZoneResults[0].Links[1].SourceIDs, "sql-rdd-40")
		},
		"other Zone trace": func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[1].Links[0].SourceIDs = append(b.EnergyExplanation.ZoneResults[1].Links[0].SourceIDs, "sql-rdd-40")
		},
		"opposite service trace": func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Links[2].SourceIDs = append(b.EnergyExplanation.ZoneResults[0].Links[2].SourceIDs, "sql-rdd-40")
		},
		"detail replaces required canonical load": func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Links[0].SourceIDs = []string{"tabular-cooling", "sql-rdd-40", "sql-rdd-41"}
		},
		"monthly fabricated zero consumption": func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults[0].Periods[1].Nodes = []EnergyExplanationNode{{ID: "fake", Level: "end_use", EndUse: "cooling", Period: "M1", ZoneName: "A", Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", SourceIDs: []string{"sql-rdd-40"}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			b := epathSQLAnnualZoneDetailUnitBundle()
			mutate(&b)
			if failures := epathEvaluateSQLModelChecks(&epathRealOracleEvidence{}, b, checks); len(failures) == 0 {
				t.Fatal("unreviewed or additive detail trace passed")
			}
		})
	}
	for name, mutate := range map[string]func(*epathSQLFrames){
		"wrong month owner":   func(f *epathSQLFrames) { f.LoadDetailSourceIDs[epathSQLKey("B", "cooling", 1)] = []int{40} },
		"wrong month service": func(f *epathSQLFrames) { f.LoadDetailSourceIDs[epathSQLKey("A", "heating", 1)] = []int{40} },
		"missing original":    func(f *epathSQLFrames) { delete(f.SourceIdentities, 40) },
	} {
		t.Run(name, func(t *testing.T) {
			f, m := epathSQLAnnualZoneDetailUnitFrames(t)
			mutate(&f)
			var c epathSQLModelChecks
			if err := epathSQLModelZoneServiceChecks(f, m, &c); err == nil {
				t.Fatal("wrong independently scoped detail binding passed")
			}
		})
	}
}
