package simulation

import (
	"fmt"
	"sort"
	"strings"
)

type epathSQLDirectUseCarrier struct {
	Value   epathSQLQuantity
	Sources map[string]epathRealSQLSource
}

type epathSQLDirectUseProof struct {
	EndUse     string
	Multiplier float64
	Carriers   map[string]epathSQLDirectUseCarrier
}

// This compiler reads declared SQL variables directly. It does not reuse the
// production direct-use classifier, source preference or multiplier helper.
func epathSQLModelDirectUseChecks(observed []epathRealSQLSource, frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	type directMonths struct {
		Raw, Effective [12]epathSQLQuantity
		Sources        map[string]epathRealSQLSource
	}
	byEndUse := map[string]map[string]map[string]*directMonths{}
	declarations := map[string]bool{}
	for _, declaration := range model.DirectUses {
		key := declaration.EndUse + "/" + declaration.Carrier
		if declaration.EndUse == "" || declaration.Carrier == "" || declarations[key] || declaration.Source.IsMeter || declaration.Source.AllowAbsent {
			return fmt.Errorf("invalid/duplicate direct-use declaration %q", key)
		}
		declarations[key] = true
		for _, zone := range declaration.Source.Keys {
			if zone == "*" || frames.Zones[strings.ToLower(zone)].Name == "" {
				return fmt.Errorf("direct-use source requires exact reviewed Zone ownership: %q", zone)
			}
		}
		sources, err := epathSQLSelect(observed, declaration.Source)
		if err != nil {
			return err
		}
		selected := map[int]bool{}
		for _, source := range sources {
			selected[source.DictionaryIndex] = true
		}
		// No actual owner of the preferred selected output may silently vanish
		// from the reviewed declaration merely because its candidate is absent.
		for _, source := range observed {
			for _, chosen := range sources {
				if strings.EqualFold(source.Name, chosen.Name) && source.SourceUnit == chosen.SourceUnit && !source.IsMeter && strings.EqualFold(source.ReportingFrequency, "Monthly") && !selected[source.DictionaryIndex] {
					return fmt.Errorf("undeclared actual direct-use owner %s/%s", source.Name, source.KeyValue)
				}
			}
		}
		if byEndUse[declaration.EndUse] == nil {
			byEndUse[declaration.EndUse] = map[string]map[string]*directMonths{}
		}
		for _, source := range sources {
			zoneKey := strings.ToLower(source.KeyValue)
			zone, ok := frames.Zones[zoneKey]
			if !ok || !epathOracleFinite(zone.Multiplier) || zone.Multiplier <= 0 {
				return fmt.Errorf("unowned direct-use SQL source %s", source.KeyValue)
			}
			values, err := epathSQLMonthly(source, model.Precision)
			if err != nil {
				return err
			}
			if byEndUse[declaration.EndUse][zoneKey] == nil {
				byEndUse[declaration.EndUse][zoneKey] = map[string]*directMonths{}
			}
			months := &directMonths{Sources: map[string]epathRealSQLSource{fmt.Sprintf("sql-rdd-%d", source.DictionaryIndex): source}}
			for index, value := range values {
				if value.Value < 0 {
					return fmt.Errorf("negative direct-use energy %s/%s", source.Name, source.KeyValue)
				}
				months.Raw[index], months.Effective[index] = value.positive(), value.times(zone.Multiplier).positive()
			}
			for _, scope := range []string{"building", "zone"} {
				owner := ""
				if scope == "zone" {
					owner = zone.Name
				}
				for _, field := range []string{"rawValue", "effectiveValue"} {
					q := epathSQLQuantity{}
					for month := 0; month < 12; month++ {
						value := months.Raw[month]
						if field == "effectiveValue" {
							value = months.Effective[month]
						}
						q = q.add(value)
					}
					target := epathRealOracleTarget{Collection: "sources", Field: field, SourceName: source.Name, SourceKey: source.KeyValue, SourceUnit: source.SourceUnit, Frequency: source.ReportingFrequency, Unit: "kWh"}
					if err := checks.add("zoneAllocation", scope, owner, "annual", "direct_source/"+source.Name+"/"+source.KeyValue+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
						return err
					}
				}
			}
			byEndUse[declaration.EndUse][zoneKey][declaration.Carrier] = months
		}
	}
	endUses, zones := []string{}, []string{}
	for endUse := range byEndUse {
		endUses = append(endUses, endUse)
	}
	for zone := range frames.Zones {
		zones = append(zones, zone)
	}
	sort.Strings(endUses)
	sort.Strings(zones)
	for _, endUse := range endUses {
		for _, zoneKey := range zones {
			zone := frames.Zones[zoneKey]
			for _, period := range []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"} {
				proof := &epathSQLDirectUseProof{EndUse: endUse, Multiplier: zone.Multiplier, Carriers: map[string]epathSQLDirectUseCarrier{}}
				raw, effective := epathSQLQuantity{}, epathSQLQuantity{}
				carriers := []string{}
				for carrier := range byEndUse[endUse][zoneKey] {
					carriers = append(carriers, carrier)
				}
				sort.Strings(carriers)
				for _, carrier := range carriers {
					months := byEndUse[endUse][zoneKey][carrier]
					q := epathSQLQuantity{}
					for _, month := range epathSQLPeriodMonths(period) {
						raw = raw.add(months.Raw[month-1])
						q = q.add(months.Effective[month-1])
					}
					effective = effective.add(q)
					proof.Carriers[carrier] = epathSQLDirectUseCarrier{Value: q, Sources: months.Sources}
				}
				for _, field := range []string{"value", "rawValue", "effectiveValue", "allocatedValue"} {
					q := effective
					if field == "rawValue" {
						q = raw
					}
					target := epathSQLNodeTarget("end_use", endUse, "", "site")
					target.Field, target.Basis = field, "direct_zone_energy"
					target.AllowPrunedZero = field == "value" || field == "allocatedValue"
					if err := checks.add("zoneAllocation", "zone", zone.Name, period, "direct/"+endUse+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
						return err
					}
					check := &checks.Rows[len(checks.Rows)-1]
					if field == "value" {
						check.DirectUse = proof
					}
					if field == "rawValue" || field == "effectiveValue" {
						check.OptionalPresentation = effective.includesZero()
					}
				}
			}
		}
	}
	return nil
}

func epathSQLDirectUseNodeMatches(node EnergyExplanationNode, proof *epathSQLDirectUseProof) bool {
	return node.Level == "end_use" && node.EndUse == proof.EndUse
}

func epathSQLDirectUseLinkMatches(link EnergyPathLink, nodes map[string]EnergyExplanationNode, proof *epathSQLDirectUseProof) bool {
	return epathSQLDirectUseNodeMatches(nodes[link.FromID], proof) && link.Relation != "source_correspondence"
}

func epathSQLDirectUseServiceMatches(endUse, service string) bool {
	// Optional legacy direct-use tags are not a thermal cooling/heating service.
	return service == "" || endUse == "lighting" && service == "interior_lighting" || endUse == "equipment" && service == "interior_equipment"
}

func epathCheckSQLDirectUseEndpoints(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	proof := check.DirectUse
	if proof == nil || proof.EndUse == "" || check.Item.Scope != "zone" || check.Quantity == nil || !check.Quantity.valid() {
		return fmt.Errorf("invalid direct-use proof")
	}
	nodes, links, _, _, err := epathOracleGraph(bundle, check.Item.Scope, check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	byID, selected := map[string]EnergyExplanationNode{}, map[string]bool{}
	expectedSources := map[string]epathRealSQLSource{}
	for _, carrier := range proof.Carriers {
		for id, source := range carrier.Sources {
			expectedSources[id] = source
		}
	}
	sources := map[string]EnergyDataSource{}
	for _, source := range bundle.EnergyExplanation.Sources {
		if _, duplicate := sources[source.ID]; duplicate {
			return fmt.Errorf("duplicate source identity %s", source.ID)
		}
		sources[source.ID] = source
	}
	verifySources := func(ids []string, want map[string]epathRealSQLSource) error {
		seen := map[string]bool{}
		for _, id := range ids {
			original, ok := want[id]
			actual, exists := sources[id]
			if !ok || !exists || seen[id] || actual.SourceType != "sql_report_data" || actual.IsMeter || !strings.EqualFold(actual.Name, original.Name) || !strings.EqualFold(actual.KeyValue, original.KeyValue) || actual.SourceUnit != original.SourceUnit || !strings.EqualFold(actual.ReportingFrequency, original.ReportingFrequency) {
				return fmt.Errorf("direct-use trace does not match exact SQL identity %s", id)
			}
			seen[id] = true
		}
		if len(seen) != len(want) {
			return fmt.Errorf("direct-use trace lost required SQL identities")
		}
		return nil
	}
	for _, node := range nodes {
		byID[node.ID] = node
		if !epathSQLDirectUseNodeMatches(node, proof) {
			continue
		}
		if node.Basis != "direct_zone_energy" || node.ScaleDomain != "site" || node.Unit != "kWh" || node.AggregationBasis != "model_total" || node.Multiplier != proof.Multiplier || !epathSQLDirectUseServiceMatches(proof.EndUse, node.ServiceKind) {
			return fmt.Errorf("direct-use node has incorrect basis/domain/multiplier")
		}
		if err := verifySources(node.SourceIDs, expectedSources); err != nil {
			return err
		}
		selected[node.ID] = true
	}
	if len(selected) > 1 || len(selected) == 0 && !check.Quantity.includesZero() {
		return fmt.Errorf("missing/duplicate exact direct-use node")
	}
	seen := map[string]bool{}
	for _, link := range links {
		if !epathSQLDirectUseLinkMatches(link, byID, proof) {
			continue
		}
		target := byID[link.ToID]
		carrier, ok := proof.Carriers[target.Carrier]
		if !ok || !selected[link.FromID] || target.Level != "carrier" || target.Unit != "kWh" || target.ScaleDomain != "site" || link.Relation != "direct_end_use_to_carrier" || link.Basis != "direct_zone_energy" || link.FromUnit != "kWh" || link.ToUnit != "kWh" || link.FromValue != link.ToValue || seen[target.Carrier] {
			return fmt.Errorf("direct-use link has incorrect/duplicate carrier relation")
		}
		if !epathSQLDirectUseServiceMatches(proof.EndUse, link.ServiceKind) || link.Ratio != 0 || link.RatioKind != "" || link.RatioLabel != "" {
			return fmt.Errorf("direct-use correspondence cannot invent a thermal service or conversion ratio")
		}
		if err := epathCheckSQLModelQuantity(&link.FromValue, &carrier.Value); err != nil {
			return err
		}
		if err := epathCheckSQLModelQuantity(&link.ToValue, &carrier.Value); err != nil {
			return err
		}
		if err := verifySources(link.SourceIDs, carrier.Sources); err != nil {
			return err
		}
		seen[target.Carrier] = true
	}
	for name, carrier := range proof.Carriers {
		if !seen[name] && !carrier.Value.includesZero() {
			return fmt.Errorf("missing direct-use carrier link %s", name)
		}
	}
	return nil
}
