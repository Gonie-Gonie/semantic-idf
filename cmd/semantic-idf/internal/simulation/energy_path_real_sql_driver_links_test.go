package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// The quantities are compiled from independent completed Zone-month cells.
// Category == "" denotes the entire matching-service incoming flow, not a
// wildcard that licenses an unvalidated candidate branch.
type epathSQLDriverLinkProof struct {
	Category, Service, Basis string
	Load                     epathSQLQuantity
	SourceIdentities         map[int]epathRealSQLSource
	DriverSources            []int
	RequiredDriverSources    []int
	LoadSources              []int
	RequiredLoadSources      []int
	RequiredLinkLoadSources  map[string][]int // Exact category -> independently positive completed months.
}

type epathSQLDriverTrace struct{ Allowed, Required []int }
type epathSQLDriverLinkLeaves struct{ Original, Driver map[int]bool }

// Coverage uses this same predicate only after the full proof succeeds. A bad
// destination is deliberately not filtered out: the evaluator must reject it.
func epathSQLDriverLinkMatches(nodes map[string]EnergyExplanationNode, link EnergyPathLink, proof *epathSQLDriverLinkProof) bool {
	if proof == nil || link.Relation != "driver_to_load" || link.ServiceKind != proof.Service || link.Basis != proof.Basis {
		return false
	}
	from, ok := nodes[link.FromID]
	return ok && from.Level == "driver" && from.ServiceKind == proof.Service && (proof.Category == "" || from.DriverCategory == proof.Category)
}

func epathSQLModelDriverLinkChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	zones, cellKeys := []string{}, []string{}
	for key, zone := range frames.Zones {
		if key != strings.ToLower(zone.Name) || zone.Name == "" {
			return fmt.Errorf("driver link proof has an invalid Zone identity")
		}
		zones = append(zones, zone.Name)
	}
	if len(zones) == 0 {
		return fmt.Errorf("driver link proof has no observed Zones")
	}
	sort.Strings(zones)
	for key, cell := range frames.Cells {
		if cell == nil || cell.Category == "" || cell.Month < 1 || cell.Month > 12 || frames.Zones[strings.ToLower(cell.Zone)].Name == "" {
			return fmt.Errorf("driver link proof has an invalid completed cell %s", key)
		}
		cellKeys = append(cellKeys, key)
	}
	sort.Strings(cellKeys)
	loads := map[string]epathRealSQLSelector{}
	for _, load := range model.Loads {
		if load.Service != "cooling" && load.Service != "heating" || len(loads[load.Service].Alternatives) != 0 || load.Component != "sensible" || len(load.Source.Alternatives) == 0 || len(load.Source.Keys) == 0 {
			return fmt.Errorf("driver link proof requires explicit unique delivered-load identities")
		}
		loads[load.Service] = load.Source
	}
	if len(loads) != 2 {
		return fmt.Errorf("driver link proof requires both observed delivered services")
	}
	periods := []string{"annual"}
	for month := 1; month <= 12; month++ {
		periods = append(periods, fmt.Sprintf("M%d", month))
	}
	for _, zone := range append([]string{""}, zones...) {
		scope := "building"
		if zone != "" {
			scope = "zone"
		}
		for _, service := range []string{"cooling", "heating"} {
			monthly := [12]map[string]epathSQLQuantity{}
			monthlyTrace := [12]map[string]epathSQLDriverTrace{}
			monthlyLoadSources := [12][]int{}
			monthlyRequiredLoadSources := [12][]int{}
			monthlyLoads := [12]epathSQLQuantity{}
			for month := 1; month <= 12; month++ {
				quantities := map[string]epathSQLQuantity{}
				traces := map[string]epathSQLDriverTrace{}
				for _, current := range zones {
					if zone != "" && zone != current {
						continue
					}
					load, ok := frames.Loads[epathSQLKey(current, service, month)]
					if !ok || !load.valid() || load.Value < 0 {
						return fmt.Errorf("driver links require a known nonnegative load for %s/%s/M%d", current, service, month)
					}
					monthlyLoads[month-1] = monthlyLoads[month-1].add(load)
					ids := frames.LoadSourceIDs[epathSQLKey(current, service, month)]
					if err := epathSQLDriverOriginalIDs(frames.SourceIdentities, ids); err != nil {
						return fmt.Errorf("load provenance %s/%s/M%d: %w", current, service, month, err)
					}
					monthlyLoadSources[month-1] = epathSQLDictionaryUnion(monthlyLoadSources[month-1], ids)
					if !load.includesZero() {
						monthlyRequiredLoadSources[month-1] = epathSQLDictionaryUnion(monthlyRequiredLoadSources[month-1], ids)
					}
				}
				interzone := epathSQLQuantity{}
				if scope == "building" {
					for _, key := range cellKeys {
						cell := frames.Cells[key]
						if cell.Month == month && cell.BuildingVisible && (cell.Category == "surface.interzone" || cell.Category == "air.interzone") {
							if value, ok := cell.Allocated[service]; ok {
								interzone = interzone.add(value)
							}
						}
					}
				}
				for _, key := range cellKeys {
					cell := frames.Cells[key]
					if cell.Month != month || zone != "" && !strings.EqualFold(cell.Zone, zone) || scope == "building" && !cell.BuildingVisible {
						continue
					}
					value, allocated := cell.Allocated[service]
					if !allocated { // Context observations never become flow.
						continue
					}
					if !value.valid() || value.Value < 0 {
						return fmt.Errorf("invalid independent allocated contribution %s/%s", key, service)
					}
					category := cell.Category
					if category == "internal.other" {
						category = "balance.storage_other"
					}
					if scope == "building" && (category == "surface.interzone" || category == "air.interzone") {
						category = "balance.storage_other"
						if monthlyLoads[month-1].Value > 0 && interzone.Value/monthlyLoads[month-1].Value >= .05 {
							category = "interzone.transfer"
						}
					}
					quantities[category] = quantities[category].add(value)
					_, high := value.bounds()
					if high > 0 {
						if err := epathSQLDriverOriginalIDs(frames.SourceIdentities, cell.SourceIDs); err != nil {
							return fmt.Errorf("driver provenance %s/%s: %w", key, service, err)
						}
						trace := traces[category]
						trace.Allowed = epathSQLDictionaryUnion(trace.Allowed, cell.SourceIDs)
						if !value.includesZero() {
							trace.Required = epathSQLDictionaryUnion(trace.Required, cell.SourceIDs)
						}
						traces[category] = trace
					}
				}
				monthly[month-1] = quantities
				monthlyTrace[month-1] = traces
			}
			for _, period := range periods {
				quantities, load := map[string]epathSQLQuantity{}, epathSQLQuantity{}
				traces, loadSources := map[string]epathSQLDriverTrace{}, []int{}
				requiredLoadSources := []int{}
				requiredLinkLoadSources := map[string][]int{}
				for _, month := range epathSQLPeriodMonths(period) {
					load = load.add(monthlyLoads[month-1])
					loadSources = epathSQLDictionaryUnion(loadSources, monthlyLoadSources[month-1])
					requiredLoadSources = epathSQLDictionaryUnion(requiredLoadSources, monthlyRequiredLoadSources[month-1])
					for category, value := range monthly[month-1] {
						quantities[category] = quantities[category].add(value)
						if _, exists := requiredLinkLoadSources[category]; !exists {
							requiredLinkLoadSources[category] = nil
						}
						if !value.includesZero() {
							requiredLinkLoadSources[category] = epathSQLDictionaryUnion(requiredLinkLoadSources[category], monthlyRequiredLoadSources[month-1])
						}
						trace, addition := traces[category], monthlyTrace[month-1][category]
						trace.Allowed = epathSQLDictionaryUnion(trace.Allowed, addition.Allowed)
						trace.Required = epathSQLDictionaryUnion(trace.Required, addition.Required)
						traces[category] = trace
					}
				}
				categories := []string{}
				for category := range quantities {
					categories = append(categories, category)
				}
				sort.Strings(categories)
				incoming := epathSQLQuantity{}
				for _, category := range categories {
					incoming = incoming.add(quantities[category])
					trace := traces[""]
					trace.Allowed = epathSQLDictionaryUnion(trace.Allowed, traces[category].Allowed)
					trace.Required = epathSQLDictionaryUnion(trace.Required, traces[category].Required)
					traces[""] = trace
				}
				// This proves actual incoming flow, without inventing a balancing
				// contribution when reviewed Building-visible cells omit context.
				quantities[""] = incoming
				for _, category := range append(categories, "") {
					for _, field := range []string{"fromValue", "toValue"} {
						q := quantities[category]
						key := category
						if key == "" {
							key = "all_incoming"
						}
						target := epathRealOracleTarget{Collection: "links", Field: field, Relation: "driver_to_load", Service: service, Basis: "heat_balance_share", FromUnit: "kWh", ToUnit: "kWh", Aggregate: "sum"}
						if err := checks.add("loads", scope, zone, period, "driver_links/"+service+"/"+key+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
							return err
						}
						checks.Rows[len(checks.Rows)-1].DriverLink = &epathSQLDriverLinkProof{Category: category, Service: service, Basis: target.Basis, Load: load, SourceIdentities: frames.SourceIdentities, DriverSources: traces[category].Allowed, RequiredDriverSources: traces[category].Required, LoadSources: loadSources, RequiredLoadSources: requiredLoadSources, RequiredLinkLoadSources: requiredLinkLoadSources}
					}
				}
			}
		}
	}
	return nil
}

func epathCheckSQLModelDriverLink(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	proof, target := check.DriverLink, check.Item.Target
	if proof == nil || check.Quantity == nil || !check.Quantity.valid() || check.Quantity.Value < 0 || !proof.Load.valid() || proof.Load.Value < 0 || (proof.Service != "cooling" && proof.Service != "heating") || proof.Basis != "heat_balance_share" || target.Collection != "links" || target.Relation != "driver_to_load" || target.Service != proof.Service || target.Basis != proof.Basis || target.FromUnit != "kWh" || target.ToUnit != "kWh" || target.Aggregate != "sum" || target.ID != "" || target.FromID != "" || target.ToID != "" || target.Field != "fromValue" && target.Field != "toValue" || check.Item.Unit != "kWh" {
		return fmt.Errorf("invalid independent driver-link proof/target")
	}
	if err := epathValidateOracleTarget(target, check.Item.Unit); err != nil {
		return err
	}
	nodes, links, _, _, err := epathOracleGraph(bundle, check.Item.Scope, check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	byID, sources := map[string]EnergyExplanationNode{}, map[string]EnergyDataSource{}
	for _, node := range nodes {
		byID[node.ID] = node
	}
	for _, source := range bundle.EnergyExplanation.Sources {
		if source.ID == "" {
			return fmt.Errorf("empty source identity in driver-link proof")
		}
		if _, duplicate := sources[source.ID]; duplicate {
			return fmt.Errorf("duplicate source identity in driver-link proof")
		}
		sources[source.ID] = source
	}
	selected, incoming, outgoing := 0, map[string]float64{}, map[string]float64{}
	seenBranches, sourceNodes, observedLeaves := map[string]bool{}, map[string]bool{}, map[int]bool{}
	total := 0.0
	for _, link := range links {
		if !epathSQLDriverLinkMatches(byID, link, proof) {
			continue
		}
		from, to := byID[link.FromID], byID[link.ToID]
		if to.Level != "load" || to.ServiceKind != proof.Service || from.Basis != proof.Basis || from.DriverCategory == "" || from.ScaleDomain != "thermal" || to.ScaleDomain != "thermal" || from.Unit != "kWh" || to.Unit != "kWh" || link.FromUnit != "kWh" || link.ToUnit != "kWh" || link.Ratio != 0 || link.RatioKind != "" {
			return fmt.Errorf("driver link %s has wrong endpoint/service/basis/thermal units", link.ID)
		}
		if !epathOracleFinite(link.FromValue) || !epathOracleFinite(link.ToValue) || link.FromValue <= 0 || link.ToValue <= 0 || math.Abs(link.FromValue-link.ToValue) > 1e-9 {
			return fmt.Errorf("driver link %s does not preserve a positive paired thermal quantity", link.ID)
		}
		if !epathOracleFinite(from.Value) || from.Value <= 0 || !epathOracleFinite(to.Value) || to.Value <= 0 {
			return fmt.Errorf("driver link %s has a missing/nonpositive physical endpoint", link.ID)
		}
		if err := epathCheckSQLModelQuantity(epathOracleNumber(to.Value), &proof.Load); err != nil {
			return fmt.Errorf("driver link %s targets wrong independent delivered load: %w", link.ID, err)
		}
		leaves, err := epathSQLDriverLinkTrace(link, from, to, sources, proof)
		if err != nil {
			return err
		}
		for id := range leaves.Driver {
			observedLeaves[id] = true
		}
		originalIDs, paths := []int{}, append([]string(nil), link.RelatedPathIDs...)
		for id := range leaves.Original {
			originalIDs = append(originalIDs, id)
		}
		sort.Ints(originalIDs)
		sort.Strings(paths)
		// Rule/description wording and opaque derived-ID aliases do not make
		// another physical branch. Distinct original sources/paths still do.
		identity, _ := json.Marshal([]any{link.FromID, link.ToID, link.Relation, link.ServiceKind, link.Basis, originalIDs, paths})
		if seenBranches[string(identity)] {
			return fmt.Errorf("duplicate semantic driver-link branch %s", link.ID)
		}
		seenBranches[string(identity)] = true
		selected++
		sourceNodes[from.ID] = true
		outgoing[from.ID] += link.FromValue
		incoming[to.ID] += link.ToValue
		if target.Field == "fromValue" {
			total += link.FromValue
		} else {
			total += link.ToValue
		}
	}
	if len(incoming) > 1 {
		return fmt.Errorf("driver links ambiguously target multiple canonical loads for one service")
	}
	for _, node := range nodes {
		if node.Level != "driver" || node.ServiceKind != proof.Service || node.Basis != proof.Basis || proof.Category != "" && node.DriverCategory != proof.Category {
			continue
		}
		if node.Value > 0 && !sourceNodes[node.ID] {
			return fmt.Errorf("positive driver %s has no matching load link", node.ID)
		}
		if !epathOracleFinite(node.Value) || math.Abs(outgoing[node.ID]-node.Value) > 1e-8*math.Max(1, math.Abs(node.Value)) {
			return fmt.Errorf("driver %s outgoing thermal values do not equal its unchanged node value", node.ID)
		}
	}
	if selected == 0 {
		if check.Quantity.includesZero() {
			return nil
		}
		return fmt.Errorf("required independently positive driver-link quantity is absent")
	}
	for _, id := range proof.RequiredDriverSources {
		if !observedLeaves[id] {
			return fmt.Errorf("driver links omit required independently contributing SQL source %d", id)
		}
	}
	return epathCheckSQLModelQuantity(epathOracleNumber(total), check.Quantity)
}

func epathSQLDriverOriginalIDs(identities map[int]epathRealSQLSource, ids []int) error {
	if len(ids) == 0 {
		return fmt.Errorf("missing independently selected original SQL leaves")
	}
	for _, id := range ids {
		source, ok := identities[id]
		if !ok || id <= 0 || source.DictionaryIndex != id || source.Name == "" || source.ReportingFrequency == "" || source.SourceUnit == "" {
			return fmt.Errorf("unknown original SQL dictionary identity %d", id)
		}
	}
	return nil
}

// Derived-ID spelling is not evidence. Recursively resolve declared inputs to
// original name/key/unit/frequency identities selected by the SQL compiler.
func epathSQLDriverLinkTrace(link EnergyPathLink, from, to EnergyExplanationNode, sources map[string]EnergyDataSource, proof *epathSQLDriverLinkProof) (*epathSQLDriverLinkLeaves, error) {
	if len(link.SourceIDs) == 0 || len(from.SourceIDs) == 0 || len(to.SourceIDs) == 0 {
		return nil, fmt.Errorf("driver link %s lacks endpoint/source evidence", link.ID)
	}
	allowed, driver, load := map[int]bool{}, map[int]bool{}, map[int]bool{}
	for _, id := range proof.DriverSources {
		allowed[id], driver[id] = true, true
	}
	for _, id := range proof.LoadSources {
		allowed[id], load[id] = true, true
	}
	if err := epathSQLDriverOriginalIDs(proof.SourceIdentities, epathSQLDictionaryUnion(proof.DriverSources, proof.LoadSources)); err != nil {
		return nil, err
	}
	flatten := func(ids []string) (map[int]bool, error) {
		leaves, visiting, visited := map[int]bool{}, map[string]bool{}, map[string]bool{}
		var visit func(string) error
		visit = func(id string) error {
			if visiting[id] {
				return fmt.Errorf("cyclic derived driver source %s", id)
			}
			if visited[id] {
				return nil
			}
			source, ok := sources[id]
			if !ok || id == "" {
				return fmt.Errorf("driver link %s refers to missing source %s", link.ID, id)
			}
			visiting[id] = true
			if len(source.InputSourceIDs) > 0 {
				for _, input := range source.InputSourceIDs {
					if err := visit(input); err != nil {
						return err
					}
				}
			} else {
				if source.SourceType != "sql_report_data" {
					return fmt.Errorf("driver source %s is not an observed SQL leaf and has no explicit input leaves", id)
				}
				matched := 0
				for dictionaryID := range allowed {
					original := proof.SourceIdentities[dictionaryID]
					if strings.EqualFold(source.Name, original.Name) && strings.EqualFold(source.KeyValue, original.KeyValue) && source.IsMeter == original.IsMeter && source.SourceUnit == original.SourceUnit && source.NormalizedUnit == "kWh" && strings.EqualFold(source.ReportingFrequency, original.ReportingFrequency) {
						leaves[dictionaryID] = true
						matched++
					}
				}
				if matched != 1 {
					return fmt.Errorf("driver source %s has %d exact independently permitted SQL identities", id, matched)
				}
			}
			delete(visiting, id)
			visited[id] = true
			return nil
		}
		for _, id := range ids {
			if err := visit(id); err != nil {
				return nil, err
			}
		}
		return leaves, nil
	}
	seen := map[string]bool{}
	for _, id := range link.SourceIDs {
		if seen[id] {
			return nil, fmt.Errorf("duplicate driver-link source reference %s", id)
		}
		seen[id] = true
	}
	leaves, err := flatten(link.SourceIDs)
	if err != nil {
		return nil, err
	}
	fromLeaves, err := flatten(from.SourceIDs)
	if err != nil {
		return nil, err
	}
	toLeaves, err := flatten(to.SourceIDs)
	if err != nil {
		return nil, err
	}
	// Only independently positive completed Zone-month load contributors are
	// required. Zero/tiny whole-node pruning may remove optional identities;
	// the complete observed roster remains allowed, including derived inputs.
	for _, id := range proof.RequiredLoadSources {
		if !load[id] || !toLeaves[id] {
			return nil, fmt.Errorf("driver link %s omits required delivered-load SQL source %d on its load endpoint", link.ID, id)
		}
	}
	// Annual endpoints describe the entire year; a physical branch preserves
	// only its completed monthly contributions. Requiring another month's load
	// leaves on that branch would manufacture temporal overlap. The all-incoming
	// proof must also select each actual branch's independently compiled category.
	linkRequired, knownCategory := proof.RequiredLinkLoadSources[from.DriverCategory]
	if !knownCategory {
		return nil, fmt.Errorf("driver link %s has no independently reviewed category overlap", link.ID)
	}
	for _, id := range linkRequired {
		if !load[id] || !leaves[id] {
			return nil, fmt.Errorf("driver link %s omits required overlapping delivered-load SQL source %d", link.ID, id)
		}
	}
	fromTrace, loadTrace := false, false
	ownedDriverLeaves := map[int]bool{}
	for id := range leaves {
		if !fromLeaves[id] && !toLeaves[id] {
			return nil, fmt.Errorf("driver link %s has a SQL leaf unrelated to either endpoint", link.ID)
		}
		fromTrace = fromTrace || driver[id] && fromLeaves[id]
		if driver[id] && fromLeaves[id] {
			ownedDriverLeaves[id] = true
		}
		loadTrace = loadTrace || load[id] && toLeaves[id]
	}
	if !fromTrace || !loadTrace {
		return nil, fmt.Errorf("driver link %s lacks independently owned driver/load SQL leaves", link.ID)
	}
	return &epathSQLDriverLinkLeaves{Original: leaves, Driver: ownedDriverLeaves}, nil
}
