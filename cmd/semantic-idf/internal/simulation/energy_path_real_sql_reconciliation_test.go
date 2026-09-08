package simulation

import (
	"fmt"
	"sort"
	"strings"
)

// All quantities originate in reviewed SQL frames. Candidate records only
// supply the actual side of these assertions, never a denominator or budget.
type epathSQLReconciliationProof struct {
	ID, Level, ZoneName, Service, Period, Basis, Unit, Status string
	// Used only when an independently bounded first retained month admits
	// several historical annual identities. ID remains a member/representative.
	AllowedIDs                    []string
	Expected, Explained, Residual epathSQLQuantity
}

func epathSQLReconciliationMatches(row EnergyReconciliation, proof *epathSQLReconciliationProof) bool {
	if proof == nil {
		return false
	}
	ids := proof.AllowedIDs
	if len(ids) == 0 {
		ids = []string{proof.ID}
	}
	identity := false
	for _, id := range ids {
		identity = identity || row.ID == id
	}
	return identity && row.Level == proof.Level && row.Period == proof.Period && strings.EqualFold(row.ZoneName, proof.ZoneName) && row.ServiceKind == proof.Service && row.Unit == proof.Unit && row.Basis == proof.Basis && (proof.Status == "" || row.Status == proof.Status)
}

func (proof *epathSQLReconciliationProof) fields() map[string]*epathSQLQuantity {
	return map[string]*epathSQLQuantity{"expectedValue": &proof.Expected, "explainedValue": &proof.Explained, "residualValue": &proof.Residual}
}

func epathCheckSQLModelReconciliation(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	p, target := check.Reconciliation, check.Item.Target
	if p == nil || p.ID == "" || p.Level == "" || p.Unit == "" || p.Basis == "" || p.Period != check.Item.Period || target.Collection != "reconciliation" || target.ID != p.ID || target.Level != p.Level || target.Unit != p.Unit || target.Basis != p.Basis || target.Service != p.Service || target.Status != p.Status {
		return fmt.Errorf("reconciliation requires an exact independent identity/context")
	}
	if check.Item.Scope == "zone" && !strings.EqualFold(check.Item.Zone, p.ZoneName) {
		return fmt.Errorf("reconciliation proof routes to a different Zone")
	}
	ids := map[string]bool{p.ID: true}
	if len(p.AllowedIDs) > 0 {
		ids = map[string]bool{}
		for _, id := range p.AllowedIDs {
			if id == "" || ids[id] {
				return fmt.Errorf("empty/duplicate allowed reconciliation identity")
			}
			ids[id] = true
		}
		if !ids[p.ID] {
			return fmt.Errorf("representative reconciliation identity is not independently allowed")
		}
	}
	canPrune := true
	for field, q := range p.fields() {
		if !q.valid() {
			return fmt.Errorf("unknown/nonfinite independent reconciliation %s", field)
		}
		canPrune = canPrune && q.includesZero()
	}
	q := p.fields()[target.Field]
	if q == nil || check.Quantity == nil || !check.Quantity.valid() {
		return fmt.Errorf("missing selected reconciliation quantity")
	}
	ql, qh := q.bounds()
	cl, ch := check.Quantity.bounds()
	if q.Value != check.Quantity.Value || ql != cl || qh != ch {
		return fmt.Errorf("selected quantity contradicts whole reconciliation proof")
	}
	// The signed residual's center must be the independently computed closure.
	if p.Residual.Value != p.Expected.Value-p.Explained.Value {
		// Annual residual is a sum of completed monthly differences: floating
		// subtraction need not be bit-identical to difference of annual sums.
		difference := p.Expected.add(p.Explained.times(-1))
		if err := epathCheckSQLModelQuantity(&p.Residual.Value, &difference); err != nil {
			return fmt.Errorf("independent reconciliation does not close: %w", err)
		}
	}
	_, _, rows, _, err := epathOracleGraph(bundle, check.Item.Scope, check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	validate := func(rows []EnergyReconciliation) error {
		var row *EnergyReconciliation
		for i := range rows {
			if ids[rows[i].ID] {
				if row != nil {
					return fmt.Errorf("duplicate exact reconciliation identity")
				}
				row = &rows[i]
			}
		}
		if row == nil {
			if canPrune {
				return nil
			}
			return fmt.Errorf("missing whole reconciliation with non-prunable independent quantities")
		}
		if !epathSQLReconciliationMatches(*row, p) {
			return fmt.Errorf("present reconciliation has contradictory context")
		}
		for field, value := range map[string]float64{"expectedValue": row.ExpectedValue, "explainedValue": row.ExplainedValue, "residualValue": row.ResidualValue} {
			if err := epathCheckSQLModelQuantity(&value, p.fields()[field]); err != nil {
				return fmt.Errorf("present reconciliation %s: %w", field, err)
			}
		}
		return nil
	}
	if err := validate(rows); err != nil {
		return err
	}
	// The ordinary reader selects top-level annual. Apply the same SQL proof
	// to its duplicate annual wrapper too; neither copy can repair the other.
	if check.Item.Period == "annual" {
		periods := bundle.EnergyExplanation.Periods
		if check.Item.Scope == "zone" {
			for _, zone := range bundle.EnergyExplanation.ZoneResults {
				if strings.EqualFold(zone.Scope.ZoneName, check.Item.Zone) {
					periods = zone.Periods
				}
			}
		}
		found := false
		for _, period := range periods {
			if period.ID != "annual" {
				continue
			}
			if found || period.Kind != "annual" {
				return fmt.Errorf("ambiguous/invalid annual reconciliation wrapper")
			}
			found = true
			if err := validate(period.Reconciliation); err != nil {
				return fmt.Errorf("annual wrapper: %w", err)
			}
		}
		if !found {
			return fmt.Errorf("missing annual reconciliation wrapper")
		}
	}
	return nil
}

// This is the documented ID grammar, deliberately not the production metricID
// helper: collisions in two reviewed Zone names are rejected by the compiler.
func epathSQLReconciliationToken(name string) string {
	var out strings.Builder
	underscore := false
	for _, char := range strings.ToLower(name) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			out.WriteRune(char)
			underscore = false
		} else if !underscore {
			out.WriteByte('_')
			underscore = true
		}
	}
	return strings.Trim(out.String(), "_")
}

func epathSQLAddReconciliation(checks *epathSQLModelChecks, scope, selectedZone string, proof epathSQLReconciliationProof) error {
	for _, field := range []string{"expectedValue", "explainedValue", "residualValue"} {
		target := epathRealOracleTarget{Collection: "reconciliation", ID: proof.ID, Level: proof.Level, Field: field, Basis: proof.Basis, Unit: proof.Unit, Service: proof.Service, Status: proof.Status}
		q := *proof.fields()[field]
		if err := checks.add("residuals", scope, selectedZone, proof.Period, proof.ID+"/"+field, proof.Unit, &q, target, "", nil, nil); err != nil {
			return err
		}
		copy := proof
		checks.Rows[len(checks.Rows)-1].Reconciliation = &copy
	}
	return nil
}

func epathSQLModelThermalReconciliationChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	if len(frames.Zones) == 0 {
		return fmt.Errorf("thermal reconciliation requires independently observed Zones")
	}
	zones, tokens := []string{}, map[string]bool{}
	for key, zone := range frames.Zones {
		token := epathSQLReconciliationToken(zone.Name)
		if key != strings.ToLower(zone.Name) || token == "" || tokens[token] || !epathOracleFinite(zone.Multiplier) || zone.Multiplier <= 0 {
			return fmt.Errorf("unknown/ambiguous thermal reconciliation Zone identity")
		}
		tokens[token] = true
		zones = append(zones, key)
	}
	sort.Strings(zones)
	roles := map[string]string{}
	for _, family := range model.Families {
		if family.ID == "" || roles[family.ID] != "" || (family.Role != "pressure" && family.Role != "context") {
			return fmt.Errorf("unknown/duplicate thermal family role")
		}
		roles[family.ID] = family.Role
		owners := map[string]bool{}
		for _, owner := range family.Keys {
			key := strings.ToLower(owner)
			if frames.Zones[key].Name == "" || owners[key] {
				return fmt.Errorf("unknown/duplicate declared thermal family owner")
			}
			owners[key] = true
			for month := 1; month <= 12; month++ {
				cell := frames.Cells[epathSQLKey(key, family.ID, month)]
				if cell == nil || cell.Category != family.Category || cell.Component != family.Component {
					return fmt.Errorf("missing/misclassified declared thermal family month")
				}
			}
		}
	}
	cellKeys := []string{}
	for key, cell := range frames.Cells {
		if cell == nil || frames.Zones[cell.Zone].Name == "" || cell.Month < 1 || cell.Month > 12 || key != epathSQLKey(cell.Zone, cell.Family, cell.Month) {
			return fmt.Errorf("unknown/misrouted thermal reconciliation cell")
		}
		if !strings.HasPrefix(cell.Family, "surface:") && roles[cell.Family] == "" {
			return fmt.Errorf("undeclared reconciliation cell family")
		}
		cellKeys = append(cellKeys, key)
	}
	sort.Strings(cellKeys)
	periods := []string{"annual"}
	for month := 1; month <= 12; month++ {
		periods = append(periods, fmt.Sprintf("M%d", month))
	}
	for _, service := range []string{"cooling", "heating"} {
		monthly := map[string][]epathSQLReconciliationProof{"": make([]epathSQLReconciliationProof, 12)}
		for _, zone := range zones {
			monthly[zone] = make([]epathSQLReconciliationProof, 12)
			for month := 1; month <= 12; month++ {
				load, exists := frames.Loads[epathSQLKey(zone, service, month)]
				if !exists || !load.valid() || load.Value < 0 {
					return fmt.Errorf("missing/invalid independent load %s/%s/M%d", zone, service, month)
				}
				explained := epathSQLQuantity{}
				for _, key := range cellKeys {
					cell := frames.Cells[key]
					if cell.Zone != zone || cell.Month != month {
						continue
					}
					q, allocated := cell.Allocated[service]
					if !allocated {
						if roles[cell.Family] == "context" {
							continue // explicit context cells are not contributions
						}
						return fmt.Errorf("missing independent allocated contribution is not zero")
					}
					if roles[cell.Family] == "context" {
						return fmt.Errorf("context cell cannot supply an allocated thermal contribution")
					}
					if !q.valid() || q.Value < 0 {
						return fmt.Errorf("unknown/negative independent allocated contribution")
					}
					// Reconciliation precedes the Building-visible presentation mask.
					explained = explained.add(q)
				}
				monthly[zone][month-1] = epathSQLReconciliationProof{Expected: load, Explained: explained, Residual: load.add(explained.times(-1))}
				building := &monthly[""][month-1]
				building.Expected = building.Expected.add(load)
				building.Explained = building.Explained.add(explained)
				building.Residual = building.Expected.add(building.Explained.times(-1))
			}
		}
		for _, zone := range append([]string{""}, zones...) {
			for _, period := range periods {
				proof := epathSQLReconciliationProof{Level: "heat", Service: service, Period: period, Basis: "residual", Unit: "kWh"}
				proof.ID = "reconcile.heat." + service
				if zone != "" {
					proof.ZoneName = frames.Zones[zone].Name
					proof.ID += "." + epathSQLReconciliationToken(proof.ZoneName)
				}
				proof.ID += "." + period
				for _, month := range epathSQLPeriodMonths(period) {
					p := monthly[zone][month-1]
					proof.Expected = proof.Expected.add(p.Expected)
					proof.Explained = proof.Explained.add(p.Explained)
					proof.Residual = proof.Residual.add(p.Residual)
				}
				if err := epathSQLAddReconciliation(checks, "building", "", proof); err != nil {
					return err
				}
				if zone != "" {
					if err := epathSQLAddReconciliation(checks, "zone", proof.ZoneName, proof); err != nil {
						return err
					}
				}
			}
		}
	}
	return epathSQLDriverReconciliationChecks(frames, model, zones, periods, checks)
}

// The finite bindings below refer to reviewed recipe equations, not production
// kind/name classifiers. Unknown families cannot silently become an aggregate.
func epathSQLDriverReconciliationChecks(frames epathSQLFrames, model epathRealSQLModel, zones, periods []string, checks *epathSQLModelChecks) error {
	definitions := map[string]epathRealSQLFamily{}
	for _, family := range model.Families {
		if _, duplicate := definitions[family.ID]; duplicate {
			return fmt.Errorf("duplicate reviewed thermal family")
		}
		definitions[family.ID] = family
	}
	type equation struct {
		family, component, aggregate string
		details                      []string
		surfaces                     bool
	}
	equations := []equation{}
	if _, ok := definitions["surface.balance"]; ok {
		equations = append(equations, equation{"surface", "sensible", "surface.balance", nil, true})
	}
	for _, component := range []string{"sensible", "latent"} {
		id := "internal.other." + component
		if family, ok := definitions[id]; ok {
			equations = append(equations, equation{"internal", component, id, append([]string(nil), family.Subtract...), false})
		}
	}
	if family, ok := definitions["ventilation.sensible"]; ok {
		equations = append(equations, equation{"outdoor_air", "sensible", family.ID, append(append([]string(nil), family.Subtract...), family.ID), false})
	}
	if family, ok := definitions["outdoor.unsplit"]; ok {
		equations = append(equations, equation{"outdoor_air", "sensible", family.ID, []string{family.ID}, false})
	}
	observed := []epathRealSQLSource{}
	for _, source := range frames.SourceIdentities {
		observed = append(observed, source)
	}
	sort.Slice(observed, func(i, j int) bool { return observed[i].DictionaryIndex < observed[j].DictionaryIndex })
	type monthlyEquation struct {
		equation
		months   []epathSQLReconciliationProof
		observed bool
	}
	for _, zone := range zones {
		compiled := []monthlyEquation{}
		for _, eq := range equations {
			family := definitions[eq.aggregate]
			applies := false
			for _, owner := range family.Keys {
				applies = applies || strings.EqualFold(owner, zone)
			}
			if !applies {
				continue
			}
			if family.Component != eq.component || family.Role != "pressure" || len(family.Terms) != 1 || family.Terms[0].Sign != 1 {
				return fmt.Errorf("unreviewed %s reconciliation aggregate declaration", eq.aggregate)
			}
			if eq.surfaces && (len(family.Subtract) != 1 || family.Subtract[0] != "surface.total") || eq.family == "internal" && (family.Category != "internal.other" || len(eq.details) == 0) || eq.family == "outdoor_air" && family.Category != "air.mechanical_ventilation" && family.Category != "balance.storage_other" {
				return fmt.Errorf("unreviewed thermal reconciliation dependency equation")
			}
			selector := family.Terms[0].Source
			selector.Keys, selector.AllowAbsent = []string{frames.Zones[zone].Name}, false
			sources, err := epathSQLSelect(observed, selector)
			if err != nil || len(sources) != 1 {
				return fmt.Errorf("missing/ambiguous independent %s aggregate for %s: %v", eq.aggregate, zone, err)
			}
			source := sources[0]
			raw := frames.SourceRaw[source.DictionaryIndex]
			if len(raw) != 12 || frames.SourceZone[source.DictionaryIndex] != zone {
				return fmt.Errorf("incomplete/misrouted reconciliation aggregate observations")
			}
			observations, err := epathSQLMonthly(source, model.Precision)
			if err != nil {
				return err
			}
			entry := monthlyEquation{equation: eq, months: make([]epathSQLReconciliationProof, 12)}
			for month := 1; month <= 12; month++ {
				if !raw[month-1].valid() || raw[month-1].Value != observations[month-1].Value {
					return fmt.Errorf("unknown aggregate SQL month")
				}
				expected := raw[month-1].times(frames.Zones[zone].Multiplier)
				explained, detailObserved := epathSQLQuantity{}, false
				keys := []string{}
				if eq.surfaces {
					for key, cell := range frames.Cells {
						if cell.Zone == zone && cell.Month == month && strings.HasPrefix(cell.Family, "surface:") {
							keys = append(keys, key)
						}
					}
					if len(keys) == 0 {
						return fmt.Errorf("missing surface-detail observations")
					}
				} else {
					seen := map[string]bool{}
					for _, id := range eq.details {
						if seen[id] {
							return fmt.Errorf("duplicate reconciliation detail family")
						}
						seen[id] = true
						definition, ok := definitions[id]
						if !ok || definition.Component != eq.component || definition.Role != "pressure" || eq.family == "internal" && (!strings.HasPrefix(definition.Category, "internal.") || definition.Category == "internal.other") || eq.family == "outdoor_air" && id != eq.aggregate && definition.Category != "air.infiltration" {
							return fmt.Errorf("unreviewed reconciliation detail %s", id)
						}
						keys = append(keys, epathSQLKey(zone, id, month))
					}
				}
				sort.Strings(keys)
				for _, key := range keys {
					cell := frames.Cells[key]
					if cell == nil || !cell.Effective.valid() {
						return fmt.Errorf("missing/unknown reconciliation detail month")
					}
					explained = explained.add(cell.Effective)
					detailObserved = detailObserved || cell.Effective.Value != 0
				}
				entry.observed = entry.observed || expected.Value != 0 && detailObserved
				entry.months[month-1] = epathSQLReconciliationProof{Expected: expected, Explained: explained, Residual: expected.add(explained.times(-1))}
			}
			compiled = append(compiled, entry)
		}
		components := map[string]map[string]bool{}
		for _, entry := range compiled {
			if components[entry.family] == nil {
				components[entry.family] = map[string]bool{}
			}
			if entry.observed {
				components[entry.family][entry.component] = true
			}
		}
		for _, entry := range compiled {
			if !entry.observed && len(components[entry.family]) < 2 {
				allZero := true
				for _, month := range entry.months {
					allZero = allZero && month.Expected.includesZero() && month.Explained.includesZero() && month.Residual.includesZero()
				}
				if allZero {
					continue // no observed component identity; invented records remain uncovered
				}
			}
			for _, period := range periods {
				p := epathSQLReconciliationProof{ID: "reconcile.driver." + entry.family + "." + epathSQLReconciliationToken(frames.Zones[zone].Name), Level: "driver", ZoneName: frames.Zones[zone].Name, Period: period, Basis: "residual", Unit: "kWh"}
				if len(components[entry.family]) > 1 {
					p.ID += ".component." + entry.component
				}
				p.ID += "." + period
				for _, month := range epathSQLPeriodMonths(period) {
					m := entry.months[month-1]
					p.Expected = p.Expected.add(m.Expected)
					p.Explained = p.Explained.add(m.Explained)
					p.Residual = p.Residual.add(m.Residual)
				}
				for _, scope := range []string{"building", "zone"} {
					selectedZone := ""
					if scope == "zone" {
						selectedZone = p.ZoneName
					}
					if err := epathSQLAddReconciliation(checks, scope, selectedZone, p); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
