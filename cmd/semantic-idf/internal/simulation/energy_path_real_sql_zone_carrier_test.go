package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// These are already model-total quantities proved by the independent direct,
// HVAC and fan compilers. Do not multiply them again or use facility totals.
type epathSQLZoneCarrierProof struct {
	Carrier, Basis, ZoneName, Period string
	Value                            epathSQLQuantity
	Unavailable                      bool
}

type epathSQLZoneCarrierPart struct {
	ByCarrier                  map[string]epathSQLQuantity
	Direct                     bool
	AnnualTabular, Unavailable bool
}

func epathSQLZoneCarrierContext(zone, period string) string {
	return strings.ToLower(zone) + "\x00" + period
}

func epathSQLZoneCarrierQuantityEqual(a, b epathSQLQuantity) bool {
	if !a.valid() || !b.valid() {
		return false
	}
	al, ah := a.bounds()
	bl, bh := b.bounds()
	for _, pair := range [][2]float64{{a.Value, b.Value}, {al, bl}, {ah, bh}} {
		if math.Abs(pair[0]-pair[1]) > 1e-10*math.Max(1, math.Max(math.Abs(pair[0]), math.Abs(pair[1]))) {
			return false
		}
	}
	return true
}

func epathSQLZoneCarrierInputs(frames epathSQLFrames, model epathRealSQLModel, checks epathSQLModelChecks) (map[string]map[string]epathSQLZoneCarrierPart, []string, error) {
	if len(frames.Zones) == 0 {
		return nil, nil, fmt.Errorf("Zone carrier requires observed Zone identities")
	}
	sites, carriers := map[string]epathRealSQLSite{}, map[string]bool{}
	for _, site := range model.Site {
		if site.ID == "" || site.Carrier == "" || sites[site.ID].ID != "" {
			return nil, nil, fmt.Errorf("Zone carrier requires unique reviewed site/carrier identities")
		}
		sites[site.ID], carriers[site.Carrier] = site, true
	}
	families := map[string]map[string]bool{}
	annualFamilies := map[string]bool{}
	for _, service := range model.Services {
		key := "service/" + service.Service
		if families[key] != nil || len(service.SiteIDs) == 0 || service.Basis != "service_path_allocation" {
			return nil, nil, fmt.Errorf("missing/duplicate supported Zone service declaration")
		}
		families[key] = map[string]bool{}
		annualOnly, err := epathSQLServiceIsAnnualOnly(frames, service)
		if err != nil {
			return nil, nil, err
		}
		annualFamilies[key] = annualOnly
		for _, id := range service.SiteIDs {
			site, ok := sites[id]
			if !ok || site.Facility || site.EndUse != service.Service {
				return nil, nil, fmt.Errorf("Zone service pool has a contradictory carrier mapping")
			}
			families[key][site.Carrier] = true
		}
	}
	directOwners := map[string]map[string]map[string]bool{}
	for _, direct := range model.DirectUses {
		key := "direct/" + direct.EndUse
		if direct.EndUse == "" || direct.Carrier == "" || len(direct.Source.Keys) == 0 {
			return nil, nil, fmt.Errorf("direct carrier requires exact reviewed owners")
		}
		if families[key] == nil {
			families[key], directOwners[key] = map[string]bool{}, map[string]map[string]bool{}
		}
		if families[key][direct.Carrier] {
			return nil, nil, fmt.Errorf("duplicate direct-use carrier declaration")
		}
		families[key][direct.Carrier], carriers[direct.Carrier] = true, true
		for _, zone := range direct.Source.Keys {
			owner := strings.ToLower(zone)
			if frames.Zones[owner].Name == "" || zone == "*" {
				return nil, nil, fmt.Errorf("unreviewed direct carrier Zone %s", zone)
			}
			if directOwners[key][owner] == nil {
				directOwners[key][owner] = map[string]bool{}
			}
			directOwners[key][owner][direct.Carrier] = true
		}
	}
	fanSites, fanCarriers := map[string]bool{}, map[string]bool{}
	for _, pool := range model.FanPools {
		site, ok := sites[pool.SiteID]
		if !ok || site.Facility || site.EndUse != "fans" || site.Carrier != "electricity" || pool.Name != "Air System Fan Electricity Energy" || pool.Frequency != "Hourly" || pool.Unit != "J" {
			return nil, nil, fmt.Errorf("fan scalar lacks the exact reviewed electricity pool mapping")
		}
		fanSites[pool.SiteID], fanCarriers[site.Carrier] = true, true
	}
	if len(model.FanPools) > 0 {
		if len(fanCarriers) != 1 {
			return nil, nil, fmt.Errorf("carrier-agnostic fan scalar cannot prove multiple carriers")
		}
		families["fan"] = fanCarriers
	}
	for _, auxiliary := range model.Auxiliaries {
		if auxiliary.Weight != "unassigned" && !fanSites[auxiliary.SiteID] {
			return nil, nil, fmt.Errorf("allocated auxiliary lacks an independent Zone carrier proof: %s", auxiliary.SiteID)
		}
	}
	if len(families) == 0 || len(carriers) == 0 {
		return nil, nil, fmt.Errorf("empty independent Zone carrier recipe")
	}
	parts := map[string]map[string]epathSQLZoneCarrierPart{}
	for _, check := range checks.Rows {
		family := ""
		part := epathSQLZoneCarrierPart{ByCarrier: map[string]epathSQLQuantity{}}
		if check.ZoneService != nil {
			if check.Item.Target.Field != "value" {
				continue
			}
			family = "service/" + check.ZoneService.Service
			part.AnnualTabular, part.Unavailable = check.ZoneService.AnnualTabular, check.ZoneService.Unavailable
			for carrier, q := range check.ZoneService.Carriers {
				part.ByCarrier[carrier] = q
			}
		} else if check.DirectUse != nil {
			family, part.Direct = "direct/"+check.DirectUse.EndUse, true
			if check.DirectUse.Multiplier != frames.Zones[strings.ToLower(check.Item.Zone)].Multiplier {
				return nil, nil, fmt.Errorf("direct proof has a conflicting Zone multiplier")
			}
			for carrier, value := range check.DirectUse.Carriers {
				part.ByCarrier[carrier] = value.Value
			}
		} else if strings.HasSuffix(check.Want.Key, "|fan_pools/value") {
			family = "fan"
			if check.Quantity != nil {
				for carrier := range fanCarriers {
					part.ByCarrier[carrier] = *check.Quantity
				}
			}
		}
		if family == "" {
			continue
		}
		target, zone := check.Item.Target, strings.ToLower(check.Item.Zone)
		unavailable := part.AnnualTabular && annualFamilies[family] && part.Unavailable && check.Item.Period != "annual" && check.Quantity == nil && check.Want.Value == nil && check.Want.Status == "unavailable" && len(part.ByCarrier) == 0
		known := check.Quantity != nil && check.Quantity.valid() && check.Want.Value != nil && *check.Want.Value == check.Quantity.Value
		if families[family] == nil || part.AnnualTabular != annualFamilies[family] || part.Unavailable != unavailable || check.Item.Scope != "zone" || frames.Zones[zone].Name != check.Item.Zone || !epathOracleValidPeriod(check.Item.Period) || check.Item.Key != check.Want.Key || !checks.Keys[check.Want.Key] || check.Want.Scope != "zone" || check.Want.Zone != check.Item.Zone || check.Want.Period != check.Item.Period || check.Item.Unit != "kWh" || check.Want.Unit != "kWh" || target.Collection != "nodes" || target.Level != "end_use" || target.Field != "value" || target.Unit != "kWh" || target.ScaleDomain != "site" || !known && !unavailable {
			return nil, nil, fmt.Errorf("missing/invalid independent Zone carrier scalar identity %s", check.Want.Key)
		}
		category, basis := strings.TrimPrefix(family, "service/"), "service_path_allocation"
		if part.Direct {
			category, basis = strings.TrimPrefix(family, "direct/"), "direct_zone_energy"
		} else if family == "fan" {
			category = "fans"
		}
		if target.Category != category || target.Basis != basis {
			return nil, nil, fmt.Errorf("independent carrier component has a conflicting category/basis")
		}
		allowed := families[family]
		if part.Direct {
			allowed = directOwners[family][zone]
		}
		if !unavailable && len(part.ByCarrier) != len(allowed) {
			return nil, nil, fmt.Errorf("missing/extra carrier component in %s/%s", family, zone)
		}
		sum := epathSQLQuantity{}
		for carrier, q := range part.ByCarrier {
			if !allowed[carrier] || !q.valid() || q.Value < 0 {
				return nil, nil, fmt.Errorf("unknown/negative/carrier-swapped independent subtotal component")
			}
			low, _ := q.bounds()
			if low < 0 {
				return nil, nil, fmt.Errorf("site component has an invalid negative presentation bound")
			}
			sum = sum.add(q)
		}
		if !unavailable && !epathSQLZoneCarrierQuantityEqual(sum, *check.Quantity) {
			return nil, nil, fmt.Errorf("carrier-specific proof contradicts the independently checked end-use scalar")
		}
		context := epathSQLZoneCarrierContext(zone, check.Item.Period)
		if parts[context] == nil {
			parts[context] = map[string]epathSQLZoneCarrierPart{}
		}
		if _, duplicate := parts[context][family]; duplicate {
			return nil, nil, fmt.Errorf("duplicate independent Zone carrier component")
		}
		parts[context][family] = part
	}
	for key, zone := range frames.Zones {
		if key != strings.ToLower(zone.Name) || zone.Name == "" || !epathOracleFinite(zone.Multiplier) || zone.Multiplier <= 0 {
			return nil, nil, fmt.Errorf("invalid original Zone identity/multiplier")
		}
		for _, period := range epathSQLZoneCarrierPeriods() {
			for family := range families {
				if _, ok := parts[epathSQLZoneCarrierContext(key, period)][family]; !ok {
					return nil, nil, fmt.Errorf("missing required independent carrier component %s/%s/%s; absent is not zero", zone.Name, period, family)
				}
			}
		}
		for family := range families {
			annual := parts[epathSQLZoneCarrierContext(key, "annual")][family]
			if annualFamilies[family] {
				if !annual.AnnualTabular || annual.Unavailable {
					return nil, nil, fmt.Errorf("annual carrier family lacks its annual observation")
				}
				for month := 1; month <= 12; month++ {
					if !parts[epathSQLZoneCarrierContext(key, fmt.Sprintf("M%d", month))][family].Unavailable {
						return nil, nil, fmt.Errorf("annual-only carrier family manufactured monthly quantities")
					}
				}
				continue
			}
			for carrier, expected := range annual.ByCarrier {
				sum := epathSQLQuantity{}
				for month := 1; month <= 12; month++ {
					sum = sum.add(parts[epathSQLZoneCarrierContext(key, fmt.Sprintf("M%d", month))][family].ByCarrier[carrier])
				}
				if !epathSQLZoneCarrierQuantityEqual(sum, expected) {
					return nil, nil, fmt.Errorf("annual carrier component is not the sum of completed monthly shares: %s/%s/%s", zone.Name, family, carrier)
				}
			}
		}
	}
	ordered := []string{}
	for carrier := range carriers {
		ordered = append(ordered, carrier)
	}
	sort.Strings(ordered)
	return parts, ordered, nil
}

func epathSQLZoneCarrierPeriods() []string {
	return []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"}
}

// Compatibility IDs are not source identities. Reproduce only their literal
// ASCII token grammar here, then independently validate typed Zone and period.
func epathSQLZoneCarrierToken(name string) string {
	var token strings.Builder
	separator := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			token.WriteRune(r)
			separator = false
		} else if !separator {
			token.WriteByte('_')
			separator = true
		}
	}
	return strings.Trim(token.String(), "_")
}

func epathSQLZoneCarrierRowIDs(carrier, zone, period string, monthly [12]epathSQLQuantity) ([]string, error) {
	token := epathSQLZoneCarrierToken(zone)
	if carrier == "" || token == "" || !epathOracleValidPeriod(period) {
		return nil, fmt.Errorf("unsupported empty carrier/Zone compatibility identity")
	}
	monthlyID := func(month int) string { return fmt.Sprintf("reconcile.energy.%s.M%d.%s", carrier, month, token) }
	if period != "annual" {
		return []string{fmt.Sprintf("reconcile.energy.%s.%s.%s", carrier, period, token)}, nil
	}
	ids := []string{}
	for month, q := range monthly {
		if !q.valid() || q.Value < 0 {
			return nil, fmt.Errorf("annual carrier ID requires twelve known monthly quantities")
		}
		low, high := q.bounds()
		if high <= 0 {
			continue
		}
		id := monthlyID(month + 1)
		if strings.HasPrefix(token, "m") {
			// Historical annual adapter treats a final m-prefixed token as a
			// month even when it is a Zone token. Typed Zone remains mandatory.
			id = strings.TrimSuffix(id, token) + "annual"
		} else {
			id += ".annual"
		}
		ids = append(ids, id)
		if low > 0 {
			break // A definitely retained month excludes every later first ID.
		}
	}
	if len(ids) == 0 {
		// The whole independently known zero row may be absent. Retaining a
		// zero representative cannot authorize any positive invented amount.
		id := monthlyID(1)
		if strings.HasPrefix(token, "m") {
			id = strings.TrimSuffix(id, token) + "annual"
		} else {
			id += ".annual"
		}
		ids = []string{id}
	}
	return ids, nil
}

// A monthly direct-only graph has no inherited allocation accounting row.
// Its fresh carrier row therefore has the plain carrier/period identity. This
// narrow mode is proved from original reporting classes, never candidate IDs:
// all declared HVAC inputs are annual-only, with no fan/allocated auxiliary.
// An allocation in ANY carrier can retain inherited accounting in another
// carrier, so checking only the selected carrier's HVAC pools is insufficient.
func epathSQLZoneCarrierPlainMonthlyIDs(frames epathSQLFrames, model epathRealSQLModel) (map[string]bool, error) {
	out := map[string]bool{}
	if len(model.Services) == 0 || len(model.FanPools) != 0 {
		return out, nil
	}
	for _, auxiliary := range model.Auxiliaries {
		if auxiliary.Weight != "unassigned" {
			return out, nil
		}
	}
	for _, service := range model.Services {
		annual, err := epathSQLServiceIsAnnualOnly(frames, service)
		if err != nil {
			return nil, err
		}
		if !annual {
			return out, nil
		}
	}
	monthly, annual := map[string]bool{}, map[string]bool{}
	for _, site := range model.Site {
		yearly, err := epathSQLSiteIsAnnual(frames, site.ID)
		if err != nil {
			return nil, err
		}
		if yearly {
			annual[site.Carrier] = true
		} else {
			if _, err := epathSQLSitePeriod(frames, site.ID, "annual"); err != nil {
				return nil, err
			}
			monthly[site.Carrier] = true
		}
	}
	for _, direct := range model.DirectUses {
		if monthly[direct.Carrier] && !annual[direct.Carrier] {
			out[direct.Carrier] = true
		}
	}
	return out, nil
}

func epathSQLModelZoneCarrierChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	parts, carriers, err := epathSQLZoneCarrierInputs(frames, model, *checks)
	if err != nil {
		return err
	}
	plainMonthlyIDs, err := epathSQLZoneCarrierPlainMonthlyIDs(frames, model)
	if err != nil {
		return err
	}
	zones := []string{}
	for key := range frames.Zones {
		zones = append(zones, key)
	}
	sort.Strings(zones)
	for _, zone := range zones {
		for _, carrier := range carriers {
			monthly, direct := [12]epathSQLQuantity{}, [12]epathSQLQuantity{}
			for month := 1; month <= 12; month++ {
				context := parts[epathSQLZoneCarrierContext(zone, fmt.Sprintf("M%d", month))]
				keys := []string{}
				for family := range context {
					keys = append(keys, family)
				}
				sort.Strings(keys)
				for _, family := range keys {
					part := context[family]
					q := part.ByCarrier[carrier]
					monthly[month-1] = monthly[month-1].add(q)
					if part.Direct {
						direct[month-1] = direct[month-1].add(q)
					}
				}
			}
			for _, period := range epathSQLZoneCarrierPeriods() {
				q, d := epathSQLQuantity{}, epathSQLQuantity{}
				available, annualPresent := false, false
				for _, part := range parts[epathSQLZoneCarrierContext(zone, period)] {
					if value, ok := part.ByCarrier[carrier]; ok {
						available = true
						q = q.add(value)
						if part.Direct {
							d = d.add(value)
						}
						annualPresent = annualPresent || part.AnnualTabular
					}
				}
				if !available && period != "annual" {
					annual := parts[epathSQLZoneCarrierContext(zone, "annual")]
					for _, part := range annual {
						if _, ok := part.ByCarrier[carrier]; ok && part.AnnualTabular {
							annualPresent = true
						}
					}
				}
				if !available && annualPresent {
					if err := epathSQLZoneCarrierAbsentChecks(frames.Zones[zone].Name, carrier, period, checks); err != nil {
						return err
					}
					continue
				}
				basis := "service_path_allocation"
				dlow, dhigh := d.bounds()
				if dlow > 0 {
					basis = "direct_zone_energy"
				} else if dhigh > 0 && q.Value > 0 {
					return fmt.Errorf("ambiguous zero-crossing direct-source presence cannot select carrier basis for %s/%s/%s", zone, carrier, period)
				}
				proof := &epathSQLZoneCarrierProof{Carrier: carrier, Basis: basis, ZoneName: frames.Zones[zone].Name, Period: period, Value: q}
				for _, field := range []string{"value", "allocatedValue"} {
					target := epathSQLNodeTarget("carrier", carrier, "", "site")
					target.Field, target.Basis, target.AggregationBasis, target.Aggregate, target.AllowPrunedZero = field, basis, "model_total", "", true
					if err := checks.add("carriers", "zone", proof.ZoneName, period, "zone_subtotal/"+carrier+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
						return err
					}
					checks.Rows[len(checks.Rows)-1].ZoneCarrier = proof
				}
				ids, err := epathSQLZoneCarrierRowIDs(carrier, proof.ZoneName, period, monthly)
				if plainMonthlyIDs[carrier] {
					// The annual sum of plain M# rows also has a plain annual ID;
					// this does not apply to annual-only purchased-energy fallback.
					ids = []string{"reconcile.energy." + carrier + "." + period}
				}
				if period == "annual" && annualPresent {
					anyMonthly := false
					for _, value := range monthly {
						_, high := value.bounds()
						anyMonthly = anyMonthly || high > 0
					}
					if !anyMonthly {
						token := epathSQLZoneCarrierToken(proof.ZoneName)
						id := "reconcile.energy." + carrier + ".annual." + token
						if strings.HasPrefix(token, "m") {
							id = strings.TrimSuffix(id, token) + "annual"
						} else {
							id += ".annual"
						}
						ids = []string{id}
					}
				}
				if err != nil {
					return err
				}
				reconciliation := &epathSQLReconciliationProof{ID: ids[0], AllowedIDs: ids, Level: "energy", ZoneName: proof.ZoneName, Period: period, Basis: basis, Unit: "kWh", Status: "partial", Expected: q, Explained: q, Residual: epathSQLQuantity{}}
				for _, field := range []string{"expectedValue", "explainedValue", "residualValue"} {
					value := *reconciliation.fields()[field]
					target := epathRealOracleTarget{Collection: "reconciliation", ID: reconciliation.ID, Level: "energy", Field: field, Unit: "kWh", Basis: basis, Status: "partial"}
					if err := checks.add("residuals", "zone", proof.ZoneName, period, "zone_subtotal/"+carrier+"/"+field, "kWh", &value, target, "", nil, nil); err != nil {
						return err
					}
					checks.Rows[len(checks.Rows)-1].Reconciliation = reconciliation
				}
			}
		}
	}
	return nil
}

func epathCheckSQLZoneCarrier(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	p, target := check.ZoneCarrier, check.Item.Target
	if p != nil && p.Unavailable {
		return epathCheckSQLZoneCarrierAbsent(bundle, check)
	}
	if p == nil || p.Carrier == "" || p.Basis != "direct_zone_energy" && p.Basis != "service_path_allocation" || check.Item.Scope != "zone" || p.ZoneName != check.Item.Zone || p.Period != check.Item.Period || !p.Value.valid() || p.Value.Value < 0 || check.Quantity == nil || !epathSQLZoneCarrierQuantityEqual(*check.Quantity, p.Value) || target.Collection != "nodes" || target.Level != "carrier" || target.Category != p.Carrier || target.Unit != "kWh" || target.ScaleDomain != "site" || target.AggregationBasis != "model_total" || target.Basis != p.Basis || target.Aggregate != "" || target.Field != "value" && target.Field != "allocatedValue" {
		return fmt.Errorf("exact independent Zone carrier proof required")
	}
	nodes, _, _, _, err := epathOracleGraph(bundle, "zone", p.ZoneName, p.Period)
	if err != nil {
		return err
	}
	var selected *EnergyExplanationNode
	for index := range nodes {
		node := &nodes[index]
		if node.Level != "carrier" || node.Carrier != p.Carrier {
			continue
		}
		if selected != nil {
			return fmt.Errorf("duplicate canonical Zone carrier subtotal")
		}
		if !strings.EqualFold(node.ZoneName, p.ZoneName) || node.Period != p.Period || node.Unit != "kWh" || node.ScaleDomain != "site" || node.AggregationBasis != "model_total" || node.Basis != p.Basis {
			return fmt.Errorf("Zone carrier has contradictory scope/period/unit/domain/basis")
		}
		selected = node
	}
	if selected == nil {
		if p.Value.includesZero() {
			return nil
		}
		return fmt.Errorf("required positive independent Zone subtotal is absent")
	}
	actual, err := epathReadOracleCandidate(bundle, check.Item, check.Want)
	if err != nil {
		return err
	}
	if actual == nil || *actual < 0 {
		return fmt.Errorf("present carrier lacks a nonnegative selected numeric field")
	}
	return epathCheckSQLModelQuantity(actual, &p.Value)
}
