package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

// This consumer fixture uses real ReportData rows, but its graph is handwritten.
// The Monthly authority is 1 native kWh except observed-zero February. The
// Hourly W source integrates independently to that same quantity, not another
// pressure. Net surface pressure4, multiplier6 and load18 therefore allocate
// 14.4/3.6, or18/0 in February; annual totals are176.4/39.6 with load216.
func epathSQLDriverTemporalFixture(t *testing.T) (epathSQLFrames, epathRealSQLModel, epathSQLModelChecks, PurposeResultBundle) {
	t.Helper()
	path, model := epathSQLTemporalTraceUnitFixture(t)
	var internal epathRealSQLFamily
	for _, family := range model.Families {
		if family.ID == "internal.other.sensible" {
			internal = family
		}
	}
	if internal.ID == "" {
		t.Fatal("actual temporal SQL fixture lacks the independent internal authority")
	}
	internal.Subtract = nil
	model.Families = []epathRealSQLFamily{internal}
	for month := 1; month <= 12; month++ {
		energy := 1.0
		if month == 2 {
			energy = 0
		}
		hours := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day() * 24
		epathOracleEditSQL(t, path, fmt.Sprintf(`UPDATE ReportData SET Value=%.17g WHERE ReportDataDictionaryIndex=83 AND TimeIndex=%d;
UPDATE ReportData SET Value=%.17g WHERE ReportDataDictionaryIndex=81 AND TimeIndex IN (SELECT TimeIndex FROM Time WHERE Month=%d);`, energy*3600000, month, energy*1000/float64(hours), month))
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	without := model
	without.Families = append([]epathRealSQLFamily(nil), model.Families...)
	without.Families[0].TraceSources = nil
	baseline, err := epathCompileSQLModelFrames(path, observed.Sources, without)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(frames.Cells, baseline.Cells) || !reflect.DeepEqual(frames.Loads, baseline.Loads) || !reflect.DeepEqual(frames.LoadSourceIDs, baseline.LoadSourceIDs) {
		t.Fatal("temporal trace changed the independent monthly quantity, precision or canonical authority")
	}
	var checks, prior epathSQLModelChecks
	if err := epathSQLModelDriverLinkChecks(baseline, without, &prior); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelDriverLinkChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != len(prior.Rows) {
		t.Fatal("temporal trace changed the physical obligation roster")
	}
	for i, check := range checks.Rows {
		before := prior.Rows[i]
		if !reflect.DeepEqual(check.Want, before.Want) || !reflect.DeepEqual(check.Quantity, before.Quantity) || !reflect.DeepEqual(check.DriverLink.DriverSources, before.DriverLink.DriverSources) || !reflect.DeepEqual(check.DriverLink.RequiredDriverSources, before.DriverLink.RequiredDriverSources) || !reflect.DeepEqual(check.DriverLink.LoadSources, before.DriverLink.LoadSources) || !reflect.DeepEqual(check.DriverLink.RequiredLoadSources, before.DriverLink.RequiredLoadSources) {
			t.Fatal("Hourly context changed a numeric check or required Monthly source identity")
		}
	}
	_, _, _, bundle := epathSQLDriverLinksFixture(t)
	r := &bundle.EnergyExplanation
	for i := range r.Sources {
		if r.Sources[i].ID == "people-source" {
			r.Sources[i].ID = "sql-rdd-83"
			r.Sources[i].Name = "Zone Total Internal Convective Heating Energy"
		}
	}
	// Exact original metadata is intentionally literal and independent of the
	// new matcher. Raw source scalar rounding is tested separately by the core.
	r.Sources = append(r.Sources,
		EnergyDataSource{ID: "sql-rdd-81", SourceType: "sql_report_data", Name: "Zone Air Heat Balance Internal Convective Heat Gain Rate", KeyValue: "Office", ZoneName: "Office", Units: "W", SourceUnit: "W", NormalizedUnit: "kWh", ReportingFrequency: "Hourly", AggregationMethod: "integrate_rate_by_time_interval", AggregationBasis: "model_total", DriverRole: "reconciliation", DriverCategory: "internal.other", DriverComponent: "internal.reconciliation.convective", HeatDirection: "gain", InspectorSection: "Context", MultiplierApplication: "requires_zone_multiplier", EffectiveMultiplier: 6},
		EnergyDataSource{ID: "derived-internal", SourceType: "derived_formula", Formula: "independently observed internal balance", InputSourceIDs: []string{"sql-rdd-83", "sql-rdd-81"}},
	)
	adjust := func(nodes *[]EnergyExplanationNode, links *[]EnergyPathLink, period string) {
		wall, storage, load := 14.4, 3.6, 18.0
		if period == "annual" {
			wall, storage, load = 176.4, 39.6, 216
		} else if period == "M2" {
			wall, storage = 18, 0
		}
		(*nodes)[0].Value = wall
		(*nodes)[1].DriverCategory, (*nodes)[1].Value = "balance.storage_other", storage
		(*nodes)[1].SourceIDs = []string{"derived-internal"}
		(*nodes)[2].Value = load
		(*links)[0].FromValue, (*links)[0].ToValue = wall, wall
		(*links)[1].FromValue, (*links)[1].ToValue = storage, storage
		(*links)[1].SourceIDs = []string{"derived-internal", "cooling-source"}
		if period == "M2" {
			*nodes = append((*nodes)[:1], (*nodes)[2:]...)
			*links = (*links)[:1]
		}
	}
	adjust(&r.Nodes, &r.Links, "annual")
	for i := range r.Periods {
		p := &r.Periods[i]
		adjust(&p.Nodes, &p.Links, p.ID)
	}
	for i := range r.ZoneResults {
		z := &r.ZoneResults[i]
		adjust(&z.Nodes, &z.Links, "annual")
		for j := range z.Periods {
			p := &z.Periods[j]
			adjust(&p.Nodes, &p.Links, p.ID)
		}
	}
	return frames, model, checks, bundle
}

func epathSQLDriverTemporalCheck(t *testing.T, checks epathSQLModelChecks, period, category string) epathSQLModelCheck {
	t.Helper()
	for _, check := range checks.Rows {
		if check.Item.Scope == "building" && check.Item.Period == period && check.Item.Target.Field == "fromValue" && check.DriverLink.Service == "cooling" && check.DriverLink.Category == category {
			return check
		}
	}
	t.Fatal("missing explicit driver temporal consumer obligation")
	return epathSQLModelCheck{}
}

func epathSQLDriverTemporalClone(t *testing.T, bundle PurposeResultBundle) PurposeResultBundle {
	t.Helper()
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	copy, err := epathDecodeOriginalOracleCandidate(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return copy
}

func TestEnergyPathRealSQLDriverLinksTemporalContext(t *testing.T) {
	frames, _, checks, bundle := epathSQLDriverTemporalFixture(t)
	for _, check := range checks.Rows {
		if err := epathCheckSQLModelDriverLink(bundle, check); err != nil {
			t.Fatalf("%s: %v", check.Want.Key, err)
		}
	}
	if len(checks.Rows) != 312 {
		t.Fatalf("expected all2scopes*13periods*2services*3categories*2ends, got%d", len(checks.Rows))
	}
	annual := epathSQLDriverTemporalCheck(t, checks, "annual", "balance.storage_other")
	if math.Abs(annual.Quantity.Value-39.6) > 1e-10 || !reflect.DeepEqual(annual.DriverLink.RequiredDriverSources, []int{83}) || len(annual.DriverLink.TemporalTraceSources) != 1 || frames.TraceSourceIdentities[81].Source.Rows != 8760 {
		t.Fatal("independent annual sum, Monthly authority or exact Hourly observations lost")
	}
	for _, test := range []struct{ period, category string }{{"M2", "balance.storage_other"}, {"M1", "surface.exterior_walls"}} {
		check := epathSQLDriverTemporalCheck(t, checks, test.period, test.category)
		if len(check.DriverLink.TemporalTraceSources) != 0 {
			t.Fatal("unrelated category or fully observed zero-pressure month acquired Hourly permission")
		}
	}
	t.Logf("all%d handwritten monthly/annual and Building/Zone endpoints passed; Monthly11native kWh remains66effective, not132", len(checks.Rows))
}

func TestEnergyPathRealSQLDriverLinksTemporalRejectsProvenanceAndDoubleCounting(t *testing.T) {
	_, _, checks, bundle := epathSQLDriverTemporalFixture(t)
	check := epathSQLDriverTemporalCheck(t, checks, "annual", "balance.storage_other")
	for _, test := range []struct {
		name string
		edit func(*EnergyExplanationResult)
	}{
		{"wrong Zone", func(r *EnergyExplanationResult) { r.Sources[len(r.Sources)-2].ZoneName = "Lab" }},
		{"wrong key", func(r *EnergyExplanationResult) { r.Sources[len(r.Sources)-2].KeyValue = "Lab" }},
		{"wrong original unit", func(r *EnergyExplanationResult) { r.Sources[len(r.Sources)-2].SourceUnit = "J" }},
		{"wrong normalized unit", func(r *EnergyExplanationResult) { r.Sources[len(r.Sources)-2].NormalizedUnit = "W" }},
		{"wrong frequency", func(r *EnergyExplanationResult) { r.Sources[len(r.Sources)-2].ReportingFrequency = "Monthly" }},
		{"main flow disguise", func(r *EnergyExplanationResult) { r.Sources[len(r.Sources)-2].DriverRole = "main_flow" }},
		{"unrelated physical family", func(r *EnergyExplanationResult) {
			r.Sources[len(r.Sources)-2].DriverComponent = "surface.reconciliation"
		}},
		{"wrong multiplier", func(r *EnergyExplanationResult) { r.Sources[len(r.Sources)-2].EffectiveMultiplier = 1 }},
		{"original disguised as derived wrapper", func(r *EnergyExplanationResult) {
			s := &r.Sources[len(r.Sources)-2]
			s.SourceType, s.Formula, s.InputSourceIDs = "derived_formula", "claimed Monthly original", []string{"sql-rdd-83"}
		}},
		{"Hourly alone replaces Monthly authority", func(r *EnergyExplanationResult) { r.Sources[len(r.Sources)-1].InputSourceIDs = []string{"sql-rdd-81"} }},
		{"alias moved from driver to load", func(r *EnergyExplanationResult) {
			r.Sources[len(r.Sources)-1].InputSourceIDs = []string{"sql-rdd-83"}
			r.Nodes[2].SourceIDs = append(r.Nodes[2].SourceIDs, "sql-rdd-81")
			r.Links[1].SourceIDs = append(r.Links[1].SourceIDs, "sql-rdd-81")
		}},
		{"alias alone replaces link driver", func(r *EnergyExplanationResult) { r.Links[1].SourceIDs = []string{"sql-rdd-81", "cooling-source"} }},
		{"alias duplicated in physical quantity", func(r *EnergyExplanationResult) {
			r.Nodes[1].Value += 39.6
			r.Links[1].FromValue += 39.6
			r.Links[1].ToValue += 39.6
		}},
		{"alias duplicated in delivered load", func(r *EnergyExplanationResult) { r.Nodes[2].Value += 66 }},
		{"half branches differ only in temporal alias", func(r *EnergyExplanationResult) {
			first := r.Links[1]
			first.FromValue /= 2
			first.ToValue /= 2
			second := first
			second.ID = "alias-does-not-create-another-physical-branch"
			first.SourceIDs = []string{"sql-rdd-83", "cooling-source"}
			r.Links[1] = first
			r.Links = append(r.Links, second)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := epathSQLDriverTemporalClone(t, bundle)
			test.edit(&candidate.EnergyExplanation)
			if err := epathCheckSQLModelDriverLink(candidate, check); err == nil {
				t.Fatal("temporal context escaped independent primary-source/quantity ownership")
			}
		})
	}
	for _, period := range []string{"M1", "M2"} {
		t.Run("unrelated surface branch "+period, func(t *testing.T) {
			candidate := epathSQLDriverTemporalClone(t, bundle)
			for i := range candidate.EnergyExplanation.Periods {
				p := &candidate.EnergyExplanation.Periods[i]
				if p.ID == period {
					p.Nodes[0].SourceIDs = append(p.Nodes[0].SourceIDs, "sql-rdd-81")
					p.Links[0].SourceIDs = append(p.Links[0].SourceIDs, "sql-rdd-81")
				}
			}
			selected := epathSQLDriverTemporalCheck(t, checks, period, "surface.exterior_walls")
			if err := epathCheckSQLModelDriverLink(candidate, selected); err == nil {
				t.Fatal("global Hourly identity bypassed family/month-positive permission")
			}
		})
	}
}

func TestEnergyPathRealSQLDriverLinksTemporalCompilerAndProofGuards(t *testing.T) {
	frames, model, checks, bundle := epathSQLDriverTemporalFixture(t)
	for _, test := range []struct {
		name string
		edit func(*epathSQLFrames)
	}{
		{"cell alias from another family", func(f *epathSQLFrames) {
			f.CellTraceSourceIDs[epathSQLKey("Office", "surface:surface.exterior_walls", 1)] = []int{81}
		}},
		{"unbound original alias", func(f *epathSQLFrames) { delete(f.TraceSourceIdentities, 81) }},
		{"foreign Zone alias", func(f *epathSQLFrames) {
			a := f.TraceSourceIdentities[81]
			a.ZoneName = "Lab"
			f.TraceSourceIdentities[81] = a
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(frames)
			if err != nil {
				t.Fatal(err)
			}
			var copy epathSQLFrames
			if err := json.Unmarshal(data, &copy); err != nil {
				t.Fatal(err)
			}
			test.edit(&copy)
			var got epathSQLModelChecks
			if err := epathSQLModelDriverLinkChecks(copy, model, &got); err == nil {
				t.Fatal("compiler accepted an alias outside its independently completed family/Zone cell")
			}
		})
	}
	check := epathSQLDriverTemporalCheck(t, checks, "annual", "balance.storage_other")
	for _, test := range []struct {
		name string
		edit func(*epathSQLDriverLinkProof)
	}{
		{"alias promoted to primary driver", func(p *epathSQLDriverLinkProof) { p.DriverSources = append(p.DriverSources, 81) }},
		{"alias promoted to primary load", func(p *epathSQLDriverLinkProof) { p.LoadSources = append(p.LoadSources, 81) }},
		{"Monthly authority removed", func(p *epathSQLDriverLinkProof) { p.DriverSources = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := check
			proof := *check.DriverLink
			copy.DriverLink = &proof
			test.edit(&proof)
			if err := epathCheckSQLModelDriverLink(bundle, copy); err == nil {
				t.Fatal("temporal proof was allowed to alter canonical numeric authority")
			}
		})
	}
}
