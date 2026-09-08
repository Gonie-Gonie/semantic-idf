package simulation

import (
	"fmt"
	"math"
	"sort"
)

type epathSQLSiteResidualProof struct {
	Carrier                    string
	Expected, Mapped, Residual epathSQLQuantity
	Sources                    map[string]epathSQLOriginalSource
	Required                   map[string]bool
	Unreported                 bool
}

func epathSQLModelSiteResidualChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	for _, period := range []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"} {
		proofs := map[string]*epathSQLSiteResidualProof{}
		facilities, reported := map[string]int{}, map[string]bool{}
		seen := map[string]bool{}
		for _, site := range model.Site {
			if site.ID == "" || seen[site.ID] || site.Carrier == "" {
				return fmt.Errorf("residual source has no unique reviewed site/carrier")
			}
			seen[site.ID] = true
			q, err := epathSQLSitePeriod(frames, site.ID, period)
			if err != nil {
				return err
			}
			if known, exists := reported[site.Carrier]; exists && known != (q != nil) {
				return fmt.Errorf("residual cannot mix unreported and known carrier components")
			}
			reported[site.Carrier] = q != nil
			p := proofs[site.Carrier]
			if p == nil {
				p = &epathSQLSiteResidualProof{Carrier: site.Carrier, Sources: map[string]epathSQLOriginalSource{}, Required: map[string]bool{}, Unreported: q == nil}
				proofs[site.Carrier] = p
			}
			if site.Facility {
				facilities[site.Carrier]++
			}
			if q != nil {
				if site.Facility {
					p.Expected = p.Expected.add(*q)
				} else {
					p.Mapped = p.Mapped.add(*q)
				}
			}
			originals, required, err := epathSQLSiteOriginals(frames, site, period, model.Precision)
			if err != nil {
				return err
			}
			for id, original := range originals {
				p.Sources[id] = original
			}
			for id := range required {
				p.Required[id] = true
			}
		}
		carriers := []string{}
		for carrier := range proofs {
			carriers = append(carriers, carrier)
		}
		sort.Strings(carriers)
		for _, carrier := range carriers {
			if facilities[carrier] != 1 {
				return fmt.Errorf("residual requires exactly one reviewed facility total for %s", carrier)
			}
			p := proofs[carrier]
			p.Residual = p.Expected.add(p.Mapped.times(-1))
			for _, field := range []string{"value", "fromValue", "toValue"} {
				q := p.Residual.positive()
				quantity := &q
				if p.Unreported {
					quantity = nil
				}
				target := epathRealOracleTarget{Collection: "nodes", ID: "residual.site_" + carrier + ".building", Level: "residual", Field: field, Unit: "kWh", ScaleDomain: "site", Basis: "residual"}
				if field != "value" {
					target = epathRealOracleTarget{Collection: "links", Field: field, Relation: "residual", Basis: "residual", FromUnit: "kWh", ToUnit: "kWh", Aggregate: "sum"}
				}
				if err := checks.add("residuals", "building", "", period, "site_residual/"+carrier+"/"+field, "kWh", quantity, target, "", nil, nil); err != nil {
					return err
				}
				checks.Rows[len(checks.Rows)-1].SiteResidual = p
			}
		}
	}
	return nil
}

func epathSQLSiteResidualMatches(nodes map[string]EnergyExplanationNode, link EnergyPathLink, p *epathSQLSiteResidualProof) bool {
	from, to := nodes[link.FromID], nodes[link.ToID]
	return link.Relation == "residual" && link.ZoneName == "" && from.Level == "residual" && from.ScaleDomain == "site" && from.Carrier == p.Carrier && to.Level == "carrier" && to.Carrier == p.Carrier && to.ID == "carrier."+p.Carrier+".building" && to.Unit == "kWh" && to.ScaleDomain == "site" && to.Basis == "reported_meter" && to.AggregationBasis == "model_total" && to.ZoneName == "" && to.ServiceKind == ""
}

func epathCheckSQLSiteResidual(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	p := check.SiteResidual
	if p == nil || p.Carrier == "" || check.Item.Scope != "building" || check.Item.Zone != "" || check.Item.Unit != "kWh" || !p.Expected.valid() || !p.Mapped.valid() || !p.Residual.valid() || p.Expected.Value < 0 || p.Mapped.Value < 0 {
		return fmt.Errorf("invalid independent site residual proof")
	}
	target := check.Item.Target
	if target.Basis != "residual" || target.Collection == "nodes" && (target.ID != "residual.site_"+p.Carrier+".building" || target.Level != "residual" || target.ScaleDomain != "site" || target.Unit != "kWh" || target.Field != "value") || target.Collection == "links" && (target.Relation != "residual" || target.Aggregate != "sum" || target.FromUnit != "kWh" || target.ToUnit != "kWh" || target.Field != "fromValue" && target.Field != "toValue") || target.Collection != "nodes" && target.Collection != "links" {
		return fmt.Errorf("site residual target disagrees with its independent proof")
	}
	if p.Unreported {
		if check.Quantity != nil || check.Want.Value != nil || check.Want.Status != "unavailable" || check.Item.Period == "annual" || len(epathSQLPeriodMonths(check.Item.Period)) != 1 {
			return fmt.Errorf("unreported residual cannot acquire a numeric value")
		}
		nodes, links, _, _, err := epathOracleGraph(bundle, "building", "", check.Item.Period)
		if err != nil {
			return err
		}
		byID := map[string]EnergyExplanationNode{}
		for _, node := range nodes {
			byID[node.ID] = node
			if node.Level == "residual" && node.ScaleDomain == "site" && node.Carrier == p.Carrier {
				return fmt.Errorf("unreported monthly carrier has a fabricated site residual")
			}
		}
		for _, link := range links {
			if link.Relation == "residual" && (byID[link.ToID].Carrier == p.Carrier || link.ToID == "carrier."+p.Carrier+".building" || link.FromID == "residual.site_"+p.Carrier+".building") {
				return fmt.Errorf("unreported monthly carrier has a fabricated residual branch")
			}
		}
		return nil
	}
	difference := p.Expected.add(p.Mapped.times(-1))
	differenceLow, differenceHigh := difference.bounds()
	low, high := p.Residual.bounds()
	if p.Residual.Value != difference.Value || low != differenceLow || high != differenceHigh {
		return fmt.Errorf("site residual must retain the signed independent facility difference")
	}
	positive := p.Residual.positive()
	if check.Quantity == nil || !check.Quantity.valid() || !epathSQLZoneCarrierQuantityEqual(*check.Quantity, positive) {
		return fmt.Errorf("selected site residual quantity contradicts independent positive presentation")
	}
	expectedLow, expectedHigh := p.Expected.bounds()
	// Checklist EPATH-111: show a positive residual exceeding either 2% or
	// the reviewed 0.01-kWh absolute threshold. Negative overmapping is quality
	// evidence, never a positive fabricated source of energy.
	required := low > math.Min(.01, math.Max(0, expectedHigh)*.02) && low > .0005
	allowed := high > math.Min(.01, math.Max(0, expectedLow)*.02) && high >= .0005
	nodes, links, _, _, err := epathOracleGraph(bundle, check.Item.Scope, check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	byID := map[string]EnergyExplanationNode{}
	sources := map[string]EnergyDataSource{}
	for _, source := range bundle.EnergyExplanation.Sources {
		if _, ok := sources[source.ID]; ok {
			return fmt.Errorf("duplicate original residual source")
		}
		sources[source.ID] = source
	}
	var selected *EnergyExplanationNode
	for i := range nodes {
		node := nodes[i]
		byID[node.ID] = node
		if node.Level == "residual" && node.ScaleDomain == "site" && node.Carrier == p.Carrier {
			if selected != nil || node.ID != "residual.site_"+p.Carrier+".building" || node.Basis != "residual" || node.Unit != "kWh" || node.AggregationBasis != "model_total" || node.ZoneName != "" || node.ServiceKind != "" || node.Value <= 0 || node.Value != node.SignedValue {
				return fmt.Errorf("duplicate/invalid positive site residual node")
			}
			selected = &nodes[i]
		}
	}
	if selected == nil {
		if required {
			return fmt.Errorf("independently material site residual is missing")
		}
		return nil
	}
	if !allowed {
		return fmt.Errorf("unjustified visible residual for zero/negative/sub-threshold evidence")
	}
	if err := epathCheckSQLModelQuantity(&selected.Value, &p.Residual); err != nil {
		return err
	}
	if err := epathSQLVerifyOriginalSources(selected.SourceIDs, sources, p.Sources, p.Required, check.Item.Period); err != nil {
		return err
	}
	count := 0
	for _, link := range links {
		if link.FromID != selected.ID {
			continue
		}
		if !epathSQLSiteResidualMatches(byID, link, p) || link.Basis != "residual" || link.FromUnit != "kWh" || link.ToUnit != "kWh" || link.FromValue != selected.Value || link.ToValue != selected.Value || link.Ratio != 0 || link.RatioKind != "" || link.RatioLabel != "" || link.ServiceKind != "" {
			return fmt.Errorf("site residual is not an exact same-domain carrier branch")
		}
		if err := epathSQLVerifyOriginalSources(link.SourceIDs, sources, p.Sources, p.Required, check.Item.Period); err != nil {
			return err
		}
		count++
	}
	if count != 1 {
		return fmt.Errorf("missing/duplicate visible residual link")
	}
	return nil
}
