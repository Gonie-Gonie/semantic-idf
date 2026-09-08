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
	NonAdditiveLoadDetails   map[int]epathSQLLoadDetailIdentity
	TemporalTraceSources     map[int]epathSQLTraceSourceIdentity
	DriverSources            []int
	RequiredDriverSources    []int
	LoadSources              []int
	RequiredLoadSources      []int
	RequiredLinkLoadSources  map[string][]int                                // Exact category -> independently positive completed months.
	Branches                 map[string]map[string]epathSQLDriverBranchProof // Category -> completed-month owner (empty means multiple Zones).
	BranchChoices            map[string][]epathSQLDriverMonthChoice
}

type epathSQLDriverMonthChoice struct {
	Quantity            epathSQLQuantity
	Branches            map[string]epathSQLDriverBranchProof
	RequiredLoadSources []int
}

type epathSQLDriverBranchProof struct {
	Quantity                   epathSQLQuantity
	DriverSources, LoadSources []int
	TraceSources               []int // Alternate temporal evidence, never another pressure.
	RequiredLoadSources        []int
}

type epathSQLDriverTrace struct{ Allowed, Required, Context []int }
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
			details := map[int]epathSQLLoadDetailIdentity{}
			for id, detail := range frames.LoadDetailIdentities {
				if detail.Service != service || zone != "" && !strings.EqualFold(zone, detail.ZoneName) {
					continue
				}
				if err := epathSQLValidateLoadDetailIdentity(detail); err != nil || id != detail.Source.DictionaryIndex {
					return fmt.Errorf("invalid exact non-additive load detail: %v", err)
				}
				details[id] = detail
			}
			monthly := [12]map[string]epathSQLQuantity{}
			monthlyTrace := [12]map[string]epathSQLDriverTrace{}
			monthlyLoadSources := [12][]int{}
			monthlyRequiredLoadSources := [12][]int{}
			monthlyLoads := [12]epathSQLQuantity{}
			monthlyBranches := [12]map[string]map[string]epathSQLDriverBranchProof{}
			for month := 1; month <= 12; month++ {
				quantities := map[string]epathSQLQuantity{}
				traces := map[string]epathSQLDriverTrace{}
				zoneQuantities := map[string]map[string]epathSQLQuantity{}
				zoneSources := map[string]map[string][]int{}
				zoneTraceSources := map[string]map[string][]int{}
				possibleLoadSources := []int{}
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
					if _, high := load.bounds(); high > 0 {
						possibleLoadSources = epathSQLDictionaryUnion(possibleLoadSources, ids)
					}
					if !load.includesZero() {
						monthlyRequiredLoadSources[month-1] = epathSQLDictionaryUnion(monthlyRequiredLoadSources[month-1], ids)
					}
					for _, id := range frames.LoadDetailSourceIDs[epathSQLKey(current, service, month)] {
						detail, ok := details[id]
						if !ok || !strings.EqualFold(detail.ZoneName, current) {
							return fmt.Errorf("load detail has no exact scope/service owner")
						}
						// Inspector context is permitted on an actual load's trace,
						// but is never a numeric contributor or a required load leaf.
						possibleLoadSources = epathSQLDictionaryUnion(possibleLoadSources, []int{id})
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
					if zoneQuantities[category] == nil {
						zoneQuantities[category] = map[string]epathSQLQuantity{}
						zoneSources[category] = map[string][]int{}
						zoneTraceSources[category] = map[string][]int{}
					}
					owner := strings.ToLower(cell.Zone)
					zoneQuantities[category][owner] = zoneQuantities[category][owner].add(value)
					_, high := value.bounds()
					if high > 0 {
						if err := epathSQLDriverOriginalIDs(frames.SourceIdentities, cell.SourceIDs); err != nil {
							return fmt.Errorf("driver provenance %s/%s: %w", key, service, err)
						}
						trace := traces[category]
						trace.Allowed = epathSQLDictionaryUnion(trace.Allowed, cell.SourceIDs)
						for _, id := range frames.CellTraceSourceIDs[key] {
							alias, known := frames.TraceSourceIdentities[id]
							if !known || epathSQLValidateTemporalTrace(alias) != nil || alias.Source.DictionaryIndex != id || alias.Family != cell.Family || !strings.EqualFold(alias.ZoneName, cell.Zone) {
								return fmt.Errorf("temporal trace lacks exact independently positive family/Zone/month")
							}
							trace.Context = epathSQLDictionaryUnion(trace.Context, []int{id})
							zoneTraceSources[category][owner] = epathSQLDictionaryUnion(zoneTraceSources[category][owner], []int{id})
						}
						if !value.includesZero() {
							trace.Required = epathSQLDictionaryUnion(trace.Required, cell.SourceIDs)
						}
						traces[category] = trace
						zoneSources[category][owner] = epathSQLDictionaryUnion(zoneSources[category][owner], cell.SourceIDs)
					}
				}
				monthly[month-1] = quantities
				monthlyTrace[month-1] = traces
				monthlyBranches[month-1] = map[string]map[string]epathSQLDriverBranchProof{}
				for category, owners := range zoneQuantities {
					monthlyBranches[month-1][category] = epathSQLDriverMonthBranches(quantities[category], owners, zoneSources[category], possibleLoadSources, monthlyRequiredLoadSources[month-1])
					for owner, branch := range monthlyBranches[month-1][category] {
						for current, ids := range zoneTraceSources[category] {
							if owner == "" || owner == current {
								branch.TraceSources = epathSQLDictionaryUnion(branch.TraceSources, ids)
							}
						}
						monthlyBranches[month-1][category][owner] = branch
					}
				}
			}
			for _, period := range periods {
				quantities, load := map[string]epathSQLQuantity{}, epathSQLQuantity{}
				traces, loadSources := map[string]epathSQLDriverTrace{}, []int{}
				requiredLoadSources := []int{}
				requiredLinkLoadSources := map[string][]int{}
				branches := map[string]map[string]epathSQLDriverBranchProof{}
				branchChoices := map[string][]epathSQLDriverMonthChoice{}
				for _, month := range epathSQLPeriodMonths(period) {
					load = load.add(monthlyLoads[month-1])
					loadSources = epathSQLDictionaryUnion(loadSources, monthlyLoadSources[month-1])
					requiredLoadSources = epathSQLDictionaryUnion(requiredLoadSources, monthlyRequiredLoadSources[month-1])
					for category, value := range monthly[month-1] {
						if len(monthlyBranches[month-1][category]) > 0 {
							branchChoices[category] = append(branchChoices[category], epathSQLDriverMonthChoice{Quantity: value, Branches: monthlyBranches[month-1][category], RequiredLoadSources: monthlyRequiredLoadSources[month-1]})
						}
						if branches[category] == nil {
							branches[category] = map[string]epathSQLDriverBranchProof{}
						}
						for owner, next := range monthlyBranches[month-1][category] {
							branch := branches[category][owner]
							branch.Quantity = branch.Quantity.add(next.Quantity)
							branch.DriverSources = epathSQLDictionaryUnion(branch.DriverSources, next.DriverSources)
							branch.LoadSources = epathSQLDictionaryUnion(branch.LoadSources, next.LoadSources)
							branch.TraceSources = epathSQLDictionaryUnion(branch.TraceSources, next.TraceSources)
							branch.RequiredLoadSources = epathSQLDictionaryUnion(branch.RequiredLoadSources, next.RequiredLoadSources)
							branches[category][owner] = branch
						}
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
						trace.Context = epathSQLDictionaryUnion(trace.Context, addition.Context)
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
					trace.Context = epathSQLDictionaryUnion(trace.Context, traces[category].Context)
					traces[""] = trace
				}
				// This proves actual incoming flow, without inventing a balancing
				// contribution when reviewed Building-visible cells omit context.
				quantities[""] = incoming
				for _, category := range append(categories, "") {
					aliases := map[int]epathSQLTraceSourceIdentity{}
					for _, id := range traces[category].Context {
						aliases[id] = frames.TraceSourceIdentities[id]
					}
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
						checks.Rows[len(checks.Rows)-1].DriverLink = &epathSQLDriverLinkProof{Category: category, Service: service, Basis: target.Basis, Load: load, SourceIdentities: frames.SourceIdentities, NonAdditiveLoadDetails: details, DriverSources: traces[category].Allowed, RequiredDriverSources: traces[category].Required, LoadSources: loadSources, RequiredLoadSources: requiredLoadSources, RequiredLinkLoadSources: requiredLinkLoadSources, Branches: branches, BranchChoices: branchChoices}
						checks.Rows[len(checks.Rows)-1].DriverLink.TemporalTraceSources = aliases
					}
				}
			}
		}
	}
	return nil
}

// Monthly canonical branches retain a singleton Zone only when that Zone can
// be the sole visible contributor. Empty owner requires possible multi-Zone
// contributions; it is not a fallback for unknown or erased ownership.
func epathSQLDriverMonthBranches(quantity epathSQLQuantity, zones map[string]epathSQLQuantity, sources map[string][]int, loads, requiredLoads []int) map[string]epathSQLDriverBranchProof {
	possible, definite := []string{}, map[string]bool{}
	for zone, value := range zones {
		if _, high := value.bounds(); high > 0 {
			possible = append(possible, zone)
			definite[zone] = !value.includesZero()
		}
	}
	sort.Strings(possible)
	owners := []string{}
	if len(possible) > 1 {
		owners = append(owners, "")
	}
	for _, zone := range possible {
		sole := true
		for other, known := range definite {
			sole = sole && (other == zone || !known)
		}
		if sole {
			owners = append(owners, zone)
		}
	}
	out := map[string]epathSQLDriverBranchProof{}
	for _, owner := range owners {
		branch := epathSQLDriverBranchProof{Quantity: quantity, LoadSources: loads}
		if len(owners) == 1 && !quantity.includesZero() {
			branch.RequiredLoadSources = requiredLoads
		} else {
			branch.Quantity = quantity.optionalPresentation()
		}
		for _, zone := range possible {
			if owner == "" || owner == zone {
				branch.DriverSources = epathSQLDictionaryUnion(branch.DriverSources, sources[zone])
			}
		}
		out[owner] = branch
	}
	return out
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
	branchTotals := map[string]map[string]float64{}
	branchLeaves := map[string]map[string][]map[int]bool{}
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
		owner := strings.ToLower(link.ZoneName)
		if branchTotals[from.DriverCategory] == nil {
			branchTotals[from.DriverCategory] = map[string]float64{}
			branchLeaves[from.DriverCategory] = map[string][]map[int]bool{}
		}
		branchTotals[from.DriverCategory][owner] += link.FromValue
		branchLeaves[from.DriverCategory][owner] = append(branchLeaves[from.DriverCategory][owner], leaves.Original)
		for id := range leaves.Driver {
			observedLeaves[id] = true
		}
		originalIDs, paths := []int{}, append([]string(nil), link.RelatedPathIDs...)
		for id := range leaves.Original {
			_, detail := proof.NonAdditiveLoadDetails[id]
			_, temporal := proof.TemporalTraceSources[id]
			if !detail && !temporal {
				originalIDs = append(originalIDs, id)
			}
		}
		sort.Ints(originalIDs)
		sort.Strings(paths)
		// Rule/description wording and opaque derived-ID aliases do not make
		// another physical branch. Neither do non-additive inspector details;
		// they remain checked above as provenance, not physical contributors.
		// Distinct additive original sources/paths still do.
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
	for category, branches := range proof.Branches {
		if proof.Category != "" && category != proof.Category {
			continue
		}
		for owner, branch := range branches {
			value := branchTotals[category][owner]
			if err := epathCheckSQLModelQuantity(&value, &branch.Quantity); err != nil {
				return fmt.Errorf("independent monthly driver branch %s/%s: %w", category, owner, err)
			}
		}
		if err := epathSQLDriverChoicesContain(proof.BranchChoices[category], branchTotals[category], branchLeaves[category]); err != nil {
			return fmt.Errorf("driver category %s: %w", category, err)
		}
	}
	return epathCheckSQLModelQuantity(epathOracleNumber(total), check.Quantity)
}

// One completed month belongs wholly to one possible canonical owner. Keep
// the disjoint assignments rather than accepting their convex [0,total] hull.
// The same assignment must explain quantities and original source leaves.
func epathSQLDriverChoicesContain(choices []epathSQLDriverMonthChoice, actual map[string]float64, leaves map[string][]map[int]bool) error {
	if len(choices) > 12 {
		return fmt.Errorf("too many completed monthly driver choices")
	}
	owners := map[string]bool{}
	orderedChoices := make([][]string, len(choices))
	for index, choice := range choices {
		if !choice.Quantity.valid() || choice.Quantity.Value < 0 || len(choice.Branches) == 0 {
			return fmt.Errorf("invalid independent monthly driver choice")
		}
		for owner := range choice.Branches {
			owners[owner] = true
			// An absent owner has no physical link or source obligations. A
			// positive whole-month assignment cannot fit it; an optional zero
			// assignment is exactly the omission already searched below.
			if actual[owner] == 0 && len(leaves[owner]) == 0 {
				continue
			}
			orderedChoices[index] = append(orderedChoices[index], owner)
		}
		sort.Strings(orderedChoices[index])
	}
	for owner := range actual {
		if !owners[owner] {
			return fmt.Errorf("driver owner has no completed monthly choice")
		}
	}
	visits, capped := 0, false
	var search func(int, map[string]epathSQLDriverBranchProof) bool
	search = func(index int, assigned map[string]epathSQLDriverBranchProof) bool {
		visits++
		if visits > 65536 {
			capped = true
			return false
		}
		if index == len(choices) {
			for owner := range owners {
				branch := assigned[owner]
				value := actual[owner]
				if epathCheckSQLModelQuantity(&value, &branch.Quantity) != nil {
					return false
				}
				allowed := map[int]bool{}
				for _, id := range epathSQLDictionaryUnion(epathSQLDictionaryUnion(branch.DriverSources, branch.LoadSources), branch.TraceSources) {
					allowed[id] = true
				}
				for _, original := range leaves[owner] {
					for id := range original {
						if !allowed[id] {
							return false
						}
					}
					for _, id := range branch.RequiredLoadSources {
						if !original[id] {
							return false
						}
					}
				}
			}
			return true
		}
		choice := choices[index]
		for _, owner := range orderedChoices[index] {
			branch := assigned[owner]
			branch.Quantity = branch.Quantity.add(choice.Quantity)
			low, _ := branch.Quantity.bounds()
			if low > actual[owner]+1e-8*math.Max(1, math.Abs(actual[owner])) {
				continue
			}
			month := choice.Branches[owner]
			branch.DriverSources = epathSQLDictionaryUnion(branch.DriverSources, month.DriverSources)
			branch.LoadSources = epathSQLDictionaryUnion(branch.LoadSources, month.LoadSources)
			branch.TraceSources = epathSQLDictionaryUnion(branch.TraceSources, month.TraceSources)
			branch.RequiredLoadSources = epathSQLDictionaryUnion(branch.RequiredLoadSources, choice.RequiredLoadSources)
			next := make(map[string]epathSQLDriverBranchProof, len(assigned)+1)
			for key, value := range assigned {
				next[key] = value
			}
			next[owner] = branch
			if search(index+1, next) {
				return true
			}
			if capped {
				return false
			}
		}
		return choice.Quantity.includesZero() && search(index+1, assigned)
	}
	if search(0, map[string]epathSQLDriverBranchProof{}) {
		return nil
	}
	if capped {
		return fmt.Errorf("independent driver owner assignment exceeds bounded search limit")
	}
	return fmt.Errorf("driver branches are not a whole-month quantity/source assignment")
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
	for id, detail := range proof.NonAdditiveLoadDetails {
		original, known := proof.SourceIdentities[id]
		if err := epathSQLValidateLoadDetailIdentity(detail); err != nil || id != detail.Source.DictionaryIndex || detail.Service != proof.Service || driver[id] || load[id] || !known || original.DictionaryIndex != id || original.Name != detail.Source.Name || original.KeyValue != detail.Source.KeyValue || original.IsMeter != detail.Source.IsMeter || original.ReportingFrequency != detail.Source.ReportingFrequency || original.SourceUnit != detail.Source.SourceUnit || to.ZoneName != "" && !strings.EqualFold(to.ZoneName, detail.ZoneName) {
			return nil, fmt.Errorf("invalid non-additive load detail trace")
		}
		allowed[id] = true // Deliberately not driver[id] or load[id].
	}
	for id, alias := range proof.TemporalTraceSources {
		original, known := proof.SourceIdentities[id]
		authority, bound := proof.SourceIdentities[alias.Authority.DictionaryIndex]
		if epathSQLValidateTemporalTrace(alias) != nil || !known || id != alias.Source.DictionaryIndex || original.DictionaryIndex != id || original.Name != alias.Source.Name || original.KeyValue != alias.Source.KeyValue || original.SourceUnit != alias.Source.SourceUnit || original.ReportingFrequency != alias.Source.ReportingFrequency || original.IsMeter != alias.Source.IsMeter || !bound || authority.Name != alias.Authority.Name || authority.KeyValue != alias.Authority.KeyValue || authority.SourceUnit != alias.Authority.SourceUnit || authority.ReportingFrequency != alias.Authority.ReportingFrequency || authority.IsMeter != alias.Authority.IsMeter || !driver[alias.Authority.DictionaryIndex] || driver[id] || load[id] || to.ZoneName != "" && !strings.EqualFold(to.ZoneName, alias.ZoneName) {
			return nil, fmt.Errorf("temporal trace has no independent primary family source/owner")
		}
		if _, detail := proof.NonAdditiveLoadDetails[id]; detail {
			return nil, fmt.Errorf("ambiguous context source role")
		}
		allowed[id] = true
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
			for dictionaryID, detail := range proof.NonAdditiveLoadDetails {
				if id == fmt.Sprintf("sql-rdd-%d", dictionaryID) || strings.EqualFold(source.Name, detail.Source.Name) && strings.EqualFold(source.KeyValue, detail.Source.KeyValue) {
					if !epathSQLLoadDetailSourceMatches(source, detail) {
						return fmt.Errorf("load detail %s is not its exact original context leaf", id)
					}
				}
			}
			visiting[id] = true
			// A declared original cannot bypass its metadata checks by claiming
			// derived inputs. Unrelated derived wrappers still resolve normally.
			for dictionaryID, alias := range proof.TemporalTraceSources {
				if id == fmt.Sprintf("sql-rdd-%d", dictionaryID) || strings.EqualFold(source.Name, alias.Source.Name) && strings.EqualFold(source.KeyValue, alias.Source.KeyValue) && source.ReportingFrequency == alias.Source.ReportingFrequency {
					if !epathSQLTemporalTraceSourceMatches(source, alias) {
						return fmt.Errorf("temporal source %s is not its exact original reconciliation leaf", id)
					}
				}
			}
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
						if detail, context := proof.NonAdditiveLoadDetails[dictionaryID]; context && !epathSQLLoadDetailSourceMatches(source, detail) {
							return fmt.Errorf("load detail %s lost its exact non-additive owner/metadata", id)
						}
						if alias, context := proof.TemporalTraceSources[dictionaryID]; context && !epathSQLTemporalTraceSourceMatches(source, alias) {
							return fmt.Errorf("temporal source %s lost its exact original metadata", id)
						}
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
	branch, knownBranch := proof.Branches[from.DriverCategory][strings.ToLower(link.ZoneName)]
	if !knownBranch {
		return nil, fmt.Errorf("driver link %s has no independently possible monthly owner branch", link.ID)
	}
	branchAllowed := map[int]bool{}
	for _, id := range epathSQLDictionaryUnion(epathSQLDictionaryUnion(branch.DriverSources, branch.LoadSources), branch.TraceSources) {
		branchAllowed[id] = true
	}
	for id := range leaves {
		if !branchAllowed[id] {
			return nil, fmt.Errorf("driver link %s injects SQL source %d outside its independently possible branch months", link.ID, id)
		}
	}
	for _, id := range branch.RequiredLoadSources {
		if !load[id] || !leaves[id] {
			return nil, fmt.Errorf("driver link %s omits required overlapping delivered-load SQL source %d", link.ID, id)
		}
	}
	fromTrace, loadTrace := false, false
	ownedDriverLeaves := map[int]bool{}
	for id := range leaves {
		if _, temporal := proof.TemporalTraceSources[id]; temporal && !fromLeaves[id] {
			return nil, fmt.Errorf("driver link %s moved reconciliation temporal provenance to its load endpoint", link.ID)
		}
		if _, detail := proof.NonAdditiveLoadDetails[id]; detail && !toLeaves[id] {
			return nil, fmt.Errorf("driver link %s moved load inspector detail to its driver endpoint", link.ID)
		}
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
