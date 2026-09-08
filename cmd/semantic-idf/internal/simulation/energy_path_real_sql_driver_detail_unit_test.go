package simulation

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func epathSQLDriverDetailFixture(t *testing.T) (epathSQLModelChecks, PurposeResultBundle) {
	t.Helper()
	path, model := epathSQLLoadDetailUnitFixture(t)
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelDriverLinkChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	_, _, baseline, bundle := epathSQLDriverLinksFixture(t)
	if len(checks.Rows) != len(baseline.Rows) {
		t.Fatal("context changed obligation roster")
	}
	for i, check := range checks.Rows {
		if !reflect.DeepEqual(check.Quantity, baseline.Rows[i].Quantity) || !reflect.DeepEqual(check.DriverLink.LoadSources, baseline.Rows[i].DriverLink.LoadSources) || !reflect.DeepEqual(check.DriverLink.RequiredLoadSources, baseline.Rows[i].DriverLink.RequiredLoadSources) || !reflect.DeepEqual(check.DriverLink.DriverSources, baseline.Rows[i].DriverLink.DriverSources) {
			t.Fatal("non-additive context changed quantity or canonical source authority")
		}
	}
	bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, epathSQLLoadDetailUnitSources()...)
	add := func(nodes []EnergyExplanationNode, links []EnergyPathLink) {
		for i := range nodes {
			if nodes[i].Level == "load" && nodes[i].ServiceKind == "cooling" {
				nodes[i].SourceIDs = append(nodes[i].SourceIDs, "sql-rdd-40", "sql-rdd-41")
			}
		}
		for i := range links {
			links[i].SourceIDs = append(links[i].SourceIDs, "sql-rdd-40", "sql-rdd-41")
		}
	}
	r := &bundle.EnergyExplanation
	add(r.Nodes, r.Links)
	for i := range r.Periods {
		add(r.Periods[i].Nodes, r.Periods[i].Links)
	}
	for i := range r.ZoneResults {
		z := &r.ZoneResults[i]
		add(z.Nodes, z.Links)
		for j := range z.Periods {
			add(z.Periods[j].Nodes, z.Periods[j].Links)
		}
	}
	return checks, bundle
}

func TestEnergyPathRealSQLDriverLinksNonAdditiveContext(t *testing.T) {
	checks, bundle := epathSQLDriverDetailFixture(t)
	for _, check := range checks.Rows {
		if err := epathCheckSQLModelDriverLink(bundle, check); err != nil {
			t.Fatalf("%s: %v", check.Want.Key, err)
		}
	}
	check := epathSQLDriverAnnualWallCheck(t, checks)
	for _, test := range []struct {
		name string
		edit func(*EnergyExplanationResult)
	}{
		{"wrong owner", func(r *EnergyExplanationResult) {
			for i := range r.Sources {
				if r.Sources[i].ID == "sql-rdd-40" {
					r.Sources[i].ZoneName = "Other Zone"
				}
			}
		}},
		{"wrong service", func(r *EnergyExplanationResult) {
			for i := range r.Sources {
				if r.Sources[i].ID == "sql-rdd-40" {
					r.Sources[i].DriverCategory = "load.heating"
				}
			}
		}},
		{"main flow disguise", func(r *EnergyExplanationResult) {
			for i := range r.Sources {
				if r.Sources[i].ID == "sql-rdd-40" {
					r.Sources[i].DriverRole = "main_flow"
				}
			}
		}},
		{"derived leaf disguise", func(r *EnergyExplanationResult) {
			for i := range r.Sources {
				if r.Sources[i].ID == "sql-rdd-40" {
					r.Sources[i].InputSourceIDs = []string{"cooling-source"}
				}
			}
		}},
		{"context replaces load endpoint", func(r *EnergyExplanationResult) { r.Nodes[2].SourceIDs = []string{"sql-rdd-40", "sql-rdd-41"} }},
		{"context replaces link load", func(r *EnergyExplanationResult) {
			r.Links[0].SourceIDs = []string{"surface-a", "surface-b", "sql-rdd-40", "sql-rdd-41"}
		}},
		{"context replaces driver", func(r *EnergyExplanationResult) { r.Nodes[0].SourceIDs = []string{"sql-rdd-40"} }},
		{"load context moved to driver", func(r *EnergyExplanationResult) {
			r.Nodes[2].SourceIDs = []string{"cooling-source"}
			for i := range r.Nodes {
				if r.Nodes[i].Level == "driver" {
					r.Nodes[i].SourceIDs = append(r.Nodes[i].SourceIDs, "sql-rdd-40", "sql-rdd-41")
				}
			}
		}},
		{"opposite service detail", func(r *EnergyExplanationResult) {
			r.Nodes[2].SourceIDs = append(r.Nodes[2].SourceIDs, "sql-rdd-42")
			r.Links[0].SourceIDs = append(r.Links[0].SourceIDs, "sql-rdd-42")
		}},
		{"detail double counting", func(r *EnergyExplanationResult) { r.Nodes[2].Value += 14 }},
		{"same physical branch with different context", func(r *EnergyExplanationResult) {
			first := r.Links[0]
			first.FromValue /= 2
			first.ToValue /= 2
			second := first
			second.ID = "context-is-not-another-physical-branch"
			first.SourceIDs = []string{"surface-a", "surface-b", "cooling-source"}
			r.Links[0] = first
			r.Links = append(r.Links, second)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(bundle)
			if err != nil {
				t.Fatal(err)
			}
			candidate, err := epathDecodeOriginalOracleCandidate(strings.NewReader(string(data)))
			if err != nil {
				t.Fatal(err)
			}
			test.edit(&candidate.EnergyExplanation)
			if err := epathCheckSQLModelDriverLink(candidate, check); err == nil {
				t.Fatal("invalid context escaped strict primary-load provenance")
			}
		})
	}
}

func TestEnergyPathRealSQLDriverLinksNonAdditiveProofRejectsAuthority(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*epathSQLDriverLinkProof)
	}{
		{"numeric load authority", func(p *epathSQLDriverLinkProof) { p.LoadSources = append(p.LoadSources, 40) }},
		{"numeric driver authority", func(p *epathSQLDriverLinkProof) { p.DriverSources = append(p.DriverSources, 40) }},
		{"unbound original", func(p *epathSQLDriverLinkProof) { delete(p.SourceIdentities, 40) }},
		{"changed original identity", func(p *epathSQLDriverLinkProof) {
			s := p.SourceIdentities[40]
			s.KeyValue = "Other Equipment"
			p.SourceIdentities[40] = s
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			checks, bundle := epathSQLDriverDetailFixture(t)
			check := epathSQLDriverAnnualWallCheck(t, checks)
			test.edit(check.DriverLink)
			if err := epathCheckSQLModelDriverLink(bundle, check); err == nil {
				t.Fatal("context proof gained primary quantity authority")
			}
		})
	}
}
