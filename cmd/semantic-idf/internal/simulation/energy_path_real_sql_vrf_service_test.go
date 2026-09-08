package simulation

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

// This is the independently completed service presentation, not a production
// allocation plan. Consumption originals and denominator observations remain
// separate: a weight never becomes purchased energy.
type epathSQLVRFZoneServiceProof struct {
	ZoneName, Service, Period string
	Value, Direct, Allocated  epathSQLQuantity
	PairFrom, PairTo          epathSQLQuantity
	NativePairFrom            epathSQLQuantity
	Originals                 map[string]map[string]epathSQLOriginalSource
	Required                  map[string]map[string]bool
	Weights                   map[string]epathSQLOriginalSource
	Branches                  map[string]epathSQLVRFServiceBranch
	Owned                     bool
}

type epathSQLVRFServiceBranch struct {
	Relation  string
	Value     epathSQLQuantity
	Originals map[string]epathSQLOriginalSource
	Required  map[string]bool
}

type epathSQLVRFServiceFrames struct {
	Monthly [12]epathSQLAllocationProof
	Ledgers [12]epathSQLVRFAllocationLedgerProof
	Zones   map[string]map[string]*epathSQLZoneServiceProof
}

// Native closure and public rounded accounting have distinct authorities.
// Annual is the sum of the twelve explicit completed Monthly ledgers.
type epathSQLVRFAllocationLedgerProof struct {
	AnnualID, ID, Service, Method, Basis              string
	Period                                            string
	Expected, Direct, Allocated, Unassigned, Residual epathSQLQuantity
	NativeExpected, NativeConstituents                epathSQLQuantity
	Sources                                           map[int]epathSQLVRFLedgerSource
	Months                                            []epathSQLVRFAllocationLedgerProof
}

// Monthly-only source binding. Role/owner and the unrounded native observation
// come from the original source frame; display shares come from the independent
// source-wise integer allocation proof, never from the candidate ledger.
type epathSQLVRFLedgerSource struct {
	Identity   epathSQLVRFSourceIdentity
	Allocation epathSQLVRFSourceAllocation
}

func epathSQLVRFExactMilli(value int64) epathSQLQuantity {
	return epathSQLQuantity{Value: float64(value) / 1000}
}

// Policy B produces integer millikWh, so generic magnitude-relative SQL
// tolerance cannot authorize a different integer budget or a fractional milli.
func epathSQLVRFCheckDisplay(actual float64, expected epathSQLQuantity) error {
	if !expected.valid() || expected.Value < 0 || !epathOracleFinite(actual) || actual < 0 {
		return fmt.Errorf("invalid exact native VRF display obligation")
	}
	return epathSQLVRFRequireDisplayValue(actual, expected)
}

// This first reviewed native-system consumer requires that every served owner
// and every service pool is explained by the declared systems. An undeclared
// central remainder is not evidence for a new distribution policy.
func epathSQLCompileVRFService(frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService) (*epathSQLVRFServiceFrames, error) {
	if len(model.NativeVRFSystems) == 0 {
		if len(frames.NativeVRFSystems) != 0 || len(frames.NativeVRFAllocations) != 0 {
			return nil, fmt.Errorf("undeclared native VRF frames")
		}
		return nil, nil
	}
	if len(model.DirectHVACComponents) != 0 || len(model.NativeVRFSystems) != len(frames.NativeVRFSystems) || len(frames.NativeVRFSystems) != len(frames.NativeVRFAllocations) || service.Basis != "service_path_allocation" || service.FallbackBasis != "zone_load_allocation" || len(service.CarrierReconciliationIDs) != 0 {
		return nil, fmt.Errorf("native VRF service requires complete exclusive native frames and its broad electric ledger")
	}
	served, err := epathSQLDeclaredZones(frames, service.ServedZones)
	if err != nil {
		return nil, err
	}
	owners, siteIDs := map[string]bool{}, map[string]bool{}
	for index, system := range frames.NativeVRFSystems {
		if !reflect.DeepEqual(system.Declaration, model.NativeVRFSystems[index]) {
			return nil, fmt.Errorf("native VRF frame differs from reviewed system")
		}
		fresh, err := epathSQLCompileVRFAllocation(frames, system)
		if err != nil || !reflect.DeepEqual(fresh, frames.NativeVRFAllocations[index]) {
			return nil, fmt.Errorf("native VRF allocation differs from independent original-source arithmetic: %v", err)
		}
		id := system.Declaration.CoolingSiteID
		if service.Service == "heating" {
			id = system.Declaration.HeatingSiteID
		} else if service.Service != "cooling" {
			return nil, fmt.Errorf("unknown native VRF service")
		}
		siteIDs[id] = true
		for _, zone := range fresh.Zones {
			key := strings.ToLower(zone)
			if !served[key] || owners[key] {
				return nil, fmt.Errorf("foreign/duplicate native VRF service owner %s", zone)
			}
			owners[key] = true
		}
	}
	if len(owners) != len(served) || len(siteIDs) != len(service.SiteIDs) {
		return nil, fmt.Errorf("native VRF cannot infer an undeclared served owner or site pool")
	}
	seenSites := map[string]bool{}
	for _, id := range service.SiteIDs {
		if !siteIDs[id] || seenSites[id] {
			return nil, fmt.Errorf("native VRF service has a duplicate or foreign site identity")
		}
		seenSites[id] = true
		found := 0
		for _, site := range model.Site {
			if site.ID == id {
				found++
				if site.Facility || site.Carrier != "electricity" || site.EndUse != service.Service || site.Tabular != nil || !site.Source.IsMeter {
					return nil, fmt.Errorf("native VRF broad source is not its original Monthly service electricity meter")
				}
			}
		}
		if found != 1 || len(frames.SiteSources[id]) == 0 {
			return nil, fmt.Errorf("native VRF broad pool lacks a unique original identity")
		}
	}
	out := &epathSQLVRFServiceFrames{Zones: map[string]map[string]*epathSQLZoneServiceProof{}}
	for month := 1; month <= 12; month++ {
		broad, err := epathSQLSiteSum(frames, service.SiteIDs, month)
		if err != nil {
			return nil, err
		}
		observed := 0.0
		nativeAllocated := 0.0
		bindings := map[int]epathSQLVRFLedgerSource{}
		direct, allocated := int64(0), int64(0)
		for _, system := range frames.NativeVRFAllocations {
			current := system.Months[month]
			for _, original := range system.System.Sources {
				if original.Service != service.Service {
					continue
				}
				id := original.Source.DictionaryIndex
				source, present := current.Sources[id]
				if _, duplicate := bindings[id]; !present || duplicate {
					return nil, fmt.Errorf("native VRF ledger lost a unique original constituent")
				}
				identity := epathSQLVRFSourceIdentity{System: system.System.Declaration, Observation: original, Precision: system.System.Precision}
				if err := epathSQLValidateVRFSourceIdentity(identity); err != nil {
					return nil, err
				}
				if source.SourceID != id || source.Role != original.Role || source.Service != service.Service || source.Shared != original.Shared || !strings.EqualFold(source.ZoneName, original.ZoneName) || !reflect.DeepEqual(source.Native, original.Months[month-1]) {
					return nil, fmt.Errorf("native VRF ledger changed its original constituent role/month/quantity")
				}
				// Give the ledger its own map so a later proof mutation cannot
				// alter the separately retained independent allocation frame.
				shares := map[string]epathSQLVRFAllocatedShare{}
				for zone, share := range source.Shares {
					shares[zone] = share
					if original.Shared {
						nativeAllocated += share.Native
					}
				}
				source.Shares = shares
				bindings[id] = epathSQLVRFLedgerSource{Identity: identity, Allocation: source}
				observed += source.Native.Value
			}
			for _, values := range current.Zones {
				row := values[service.Service]
				direct += row.DirectMilliKWh
				allocated += row.AllocatedMilliKWh
			}
		}
		// Native physics uses only a counted floating-point summation bound,
		// never the broader public 3dp serialization interval.
		if !epathSQLVRFNativeClose(observed, broad.Value) {
			return nil, fmt.Errorf("native VRF constituents do not close their broad Monthly meter at raw precision")
		}
		d, a := epathSQLVRFExactMilli(direct), epathSQLVRFExactMilli(allocated)
		displayed := epathSQLQuantity{Value: math.Round(broad.Value*1000) / 1000}
		r := epathSQLQuantity{Value: (math.Round(broad.Value*1000) - float64(direct) - float64(allocated)) / 1000}
		u := epathSQLQuantity{Value: math.Max(0, r.Value)}
		period := fmt.Sprintf("M%d", month)
		id, err := epathSQLAllocationID(service.ReconciliationID, period)
		if err != nil {
			return nil, err
		}
		method := "unassigned"
		// Finite public accounting taxonomy: an actually used shared path is
		// distinguished from a local-only observation, even if display rounds
		// a very small allocated share to zero.
		if nativeAllocated > 1e-9 {
			method = "service_path_load_share"
		} else if d.Value > 0 && u.Value == 0 {
			method = "direct_only"
		}
		out.Ledgers[month-1] = epathSQLVRFAllocationLedgerProof{AnnualID: service.ReconciliationID, ID: id, Service: service.Service, Method: method, Basis: service.Basis, Period: period, Sources: bindings, Expected: displayed, Direct: d, Allocated: a, Unassigned: u, Residual: r, NativeExpected: broad, NativeConstituents: epathSQLQuantity{Value: observed}}
		out.Monthly[month-1] = epathSQLAllocationProof{Expected: &displayed, Direct: &d, Allocated: &a, Unassigned: &u}
	}
	zones := []string{}
	for key := range frames.Zones {
		zones = append(zones, key)
	}
	sort.Strings(zones)
	for _, zone := range zones {
		out.Zones[zone] = map[string]*epathSQLZoneServiceProof{}
		for _, period := range epathSQLZoneCarrierPeriods() {
			native := &epathSQLVRFZoneServiceProof{ZoneName: frames.Zones[zone].Name, Service: service.Service, Period: period,
				Owned: owners[zone], Originals: map[string]map[string]epathSQLOriginalSource{"electricity": {}}, Required: map[string]map[string]bool{"electricity": {}},
				Weights: map[string]epathSQLOriginalSource{}, Branches: map[string]epathSQLVRFServiceBranch{}}
			proof := &epathSQLZoneServiceProof{NativeVRF: native, Service: service.Service, Basis: service.Basis,
				Carriers: map[string]epathSQLQuantity{}, LoadSources: map[string]epathRealSQLSource{}}
			for _, system := range frames.NativeVRFAllocations {
				if !epathSQLVRFServiceOwns(system, native.ZoneName) {
					continue
				}
				for _, source := range system.System.Sources {
					if source.Service != service.Service || !source.Shared && !strings.EqualFold(source.ZoneName, native.ZoneName) {
						continue
					}
					original, err := epathSQLOriginalVRF(epathSQLVRFSourceIdentity{System: system.System.Declaration, Observation: source, Precision: system.System.Precision})
					if err != nil {
						return nil, err
					}
					key, err := epathSQLOriginalKey(original)
					if err != nil {
						return nil, err
					}
					native.Originals["electricity"][key], native.Required["electricity"][key] = original, true
				}
				for _, month := range epathSQLPeriodMonths(period) {
					row := system.Months[month].Zones[native.ZoneName][service.Service]
					native.Direct = native.Direct.add(epathSQLVRFExactMilli(row.DirectMilliKWh))
					native.Allocated = native.Allocated.add(epathSQLVRFExactMilli(row.AllocatedMilliKWh))
					native.PairFrom = native.PairFrom.add(epathSQLVRFExactMilli(row.PairedLoadDisplayMilliKWh))
					native.PairTo = native.PairTo.add(epathSQLVRFExactMilli(row.PairedSiteMilliKWh))
					native.NativePairFrom = native.NativePairFrom.add(row.PairedLoad)
					relation := "direct_end_use_to_carrier"
					if row.PairedSiteMilliKWh > 0 {
						relation = "end_use_to_carrier"
					}
					branch := native.Branches[relation]
					branch.Relation, branch.Originals, branch.Required = relation, native.Originals["electricity"], native.Required["electricity"]
					branch.Value = branch.Value.add(epathSQLVRFExactMilli(row.DirectMilliKWh + row.AllocatedMilliKWh))
					native.Branches[relation] = branch
					for _, owner := range system.Zones {
						for _, id := range system.Months[month].Zones[owner][service.Service].LoadSourceIDs {
							source, exists := frames.SourceIdentities[id]
							if !exists {
								return nil, fmt.Errorf("native VRF denominator lost original source")
							}
							key := fmt.Sprintf("sql-rdd-%d", id)
							native.Weights[key] = epathSQLOriginalRDD(source)
							if strings.EqualFold(owner, native.ZoneName) {
								proof.LoadSources[key] = source
							}
						}
					}
				}
			}
			native.Value = native.Direct.add(native.Allocated)
			proof.Carriers["electricity"] = native.Value
			out.Zones[zone][period] = proof
		}
	}
	return out, nil
}

func epathSQLVRFServiceOwns(system epathSQLVRFAllocationProof, zone string) bool {
	for _, owner := range system.Zones {
		if strings.EqualFold(owner, zone) {
			return true
		}
	}
	return false
}

func epathSQLVRFServiceLedger(native *epathSQLVRFServiceFrames, period string) *epathSQLVRFAllocationLedgerProof {
	if period != "annual" {
		copy := native.Ledgers[epathSQLPeriodMonths(period)[0]-1]
		return &copy
	}
	first := native.Ledgers[0]
	p := &epathSQLVRFAllocationLedgerProof{AnnualID: first.AnnualID, ID: first.AnnualID, Service: first.Service, Basis: first.Basis, Method: "unassigned", Period: period}
	for _, m := range native.Ledgers {
		if m.Method == "service_path_load_share" {
			p.Method = "service_path_load_share"
		}
		p.Months = append(p.Months, m)
		p.Expected = p.Expected.add(m.Expected)
		p.Direct = p.Direct.add(m.Direct)
		p.Allocated = p.Allocated.add(m.Allocated)
		p.Unassigned = p.Unassigned.add(m.Unassigned)
		p.Residual = p.Residual.add(m.Residual)
		p.NativeExpected = p.NativeExpected.add(m.NativeExpected)
		p.NativeConstituents = p.NativeConstituents.add(m.NativeConstituents)
	}
	if p.Method != "service_path_load_share" && p.Direct.Value > 0 && p.Unassigned.Value == 0 {
		p.Method = "direct_only"
	}
	return p
}

func epathSQLVRFZoneServiceChecks(frames epathSQLFrames, service epathRealSQLService, native *epathSQLVRFServiceFrames, zones []string, checks *epathSQLModelChecks) error {
	for _, zone := range zones {
		for _, period := range epathSQLZoneCarrierPeriods() {
			proof := native.Zones[zone][period]
			if proof == nil || proof.NativeVRF == nil {
				return fmt.Errorf("native VRF service omitted a Zone/period obligation")
			}
			value := proof.NativeVRF.Value
			for _, field := range []string{"value", "allocatedValue"} {
				target := epathSQLNodeTarget("end_use", service.Service, "", "site")
				target.Field, target.Basis, target.AllowPrunedZero = field, service.Basis, true
				if err := checks.add("zoneAllocation", "zone", frames.Zones[zone].Name, period, "hvac_zone/"+service.Service+"/"+field, "kWh", &value, target, "", nil, nil); err != nil {
					return err
				}
				if field == "value" {
					checks.Rows[len(checks.Rows)-1].ZoneService = proof
				}
			}
			for _, basis := range []string{service.Basis, service.FallbackBasis} {
				pair, kind := &epathSQLConversionProof{}, service.FallbackRatioKind
				if basis == service.Basis {
					pair = &epathSQLConversionProof{From: proof.NativeVRF.NativePairFrom, To: proof.NativeVRF.PairTo}
					kind = service.RatioKind
				}
				var ratio *epathSQLQuantity
				if pair.From.Value > 0 && pair.To.Value > 0 {
					ratio = &epathSQLQuantity{Value: pair.From.Value / pair.To.Value}
				}
				target := epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: service.Service, Basis: basis, FromUnit: "kWh", ToUnit: "kWh", RatioKind: kind, Aggregate: "sum"}
				if err := checks.add("ratios", "zone", frames.Zones[zone].Name, period, "hvac_zone/"+service.Service+"/"+basis, "ratio", ratio, target, "", nil, nil); err != nil {
					return err
				}
				checks.Rows[len(checks.Rows)-1].Conversion = pair
			}
		}
	}
	return nil
}

func epathSQLVRFZoneCoveredLink(nodes []EnergyExplanationNode, link EnergyPathLink, proof *epathSQLZoneServiceProof) bool {
	if proof == nil || proof.NativeVRF == nil || !proof.NativeVRF.Owned || link.Basis != proof.Basis || link.ServiceKind != proof.Service || link.FromUnit != "kWh" || link.ToUnit != "kWh" {
		return false
	}
	branch, exists := proof.NativeVRF.Branches[link.Relation]
	if !exists || branch.Value.Value <= 0 {
		return false
	}
	var from, to EnergyExplanationNode
	for _, node := range nodes {
		if node.ID == link.FromID {
			from = node
		}
		if node.ID == link.ToID {
			to = node
		}
	}
	return from.Level == "end_use" && from.EndUse == proof.Service && (from.ServiceKind == "" || from.ServiceKind == proof.Service) && from.Basis == proof.Basis && to.Level == "carrier" && to.Carrier == "electricity" && from.ScaleDomain == "site" && to.ScaleDomain == "site" && from.Unit == "kWh" && to.Unit == "kWh" && from.AggregationBasis == "model_total" && to.AggregationBasis == "model_total" && strings.EqualFold(from.ZoneName, proof.NativeVRF.ZoneName) && strings.EqualFold(to.ZoneName, from.ZoneName) && from.Period == proof.NativeVRF.Period && to.Period == from.Period
}

func epathSQLValidateVRFZoneServiceProof(proof *epathSQLZoneServiceProof) error {
	if proof == nil || proof.NativeVRF == nil || proof.DirectHVAC || proof.AnnualTabular {
		return fmt.Errorf("missing exclusive native VRF proof")
	}
	p := proof.NativeVRF
	if !p.Owned {
		if p.Value.Value != 0 || p.PairFrom.Value != 0 || p.PairTo.Value != 0 || len(p.Weights) != 0 || len(p.Originals["electricity"]) != 0 {
			return fmt.Errorf("unowned VRF proof invented an observation")
		}
		return nil
	}
	if len(p.Originals) != 1 || len(p.Originals["electricity"]) != 3 || len(p.Required["electricity"]) != 3 || len(proof.LoadSources) != 1 {
		return fmt.Errorf("native VRF service lost exact local/shared/own-load identities")
	}
	var system *epathRealSQLVRFSystem
	roles := map[string]bool{}
	for id, original := range p.Originals["electricity"] {
		if original.NativeVRF == nil || !p.Required["electricity"][id] {
			return fmt.Errorf("native VRF consumption lacks its typed required original")
		}
		identity := original.NativeVRF
		observation := identity.Observation
		if observation.Service != p.Service || roles[observation.Role] || !observation.Shared && !strings.EqualFold(observation.ZoneName, p.ZoneName) {
			return fmt.Errorf("native VRF consumption belongs to another owner/service/role")
		}
		roles[observation.Role] = true
		if system != nil && !reflect.DeepEqual(*system, identity.System) {
			return fmt.Errorf("native VRF service mixed outdoor systems")
		}
		copy := identity.System
		system = &copy
	}
	if err := epathSQLValidateOriginalSources(p.Originals["electricity"]); err != nil {
		return err
	}
	owners := map[string]bool{}
	for _, terminal := range system.Terminals {
		owners[strings.ToLower(terminal.ZoneName)] = true
	}
	if !owners[strings.ToLower(p.ZoneName)] || len(p.Weights) != len(owners) {
		return fmt.Errorf("native VRF denominator lost an original owner")
	}
	seen := map[string]bool{}
	name := "Zone Air System Sensible Cooling Energy"
	if p.Service == "heating" {
		name = "Zone Air System Sensible Heating Energy"
	}
	for id, original := range p.Weights {
		source := original.RDD
		if source == nil || original.NativeVRF != nil || source.Name != name || source.SourceUnit != "J" || source.ReportingFrequency != "Monthly" || source.IsMeter || !owners[strings.ToLower(source.KeyValue)] || seen[strings.ToLower(source.KeyValue)] {
			return fmt.Errorf("native VRF denominator has a foreign/duplicate/consumption source")
		}
		seen[strings.ToLower(source.KeyValue)] = true
		if strings.EqualFold(source.KeyValue, p.ZoneName) {
			own, exists := proof.LoadSources[id]
			if !exists || !reflect.DeepEqual(own, *source) {
				return fmt.Errorf("native VRF selected load differs from its denominator observation")
			}
		}
	}
	if err := epathSQLValidateOriginalSources(p.Weights); err != nil {
		return err
	}
	sum := epathSQLQuantity{}
	for relation, branch := range p.Branches {
		if relation != "end_use_to_carrier" && relation != "direct_end_use_to_carrier" || branch.Relation != relation || !reflect.DeepEqual(branch.Originals, p.Originals["electricity"]) || !reflect.DeepEqual(branch.Required, p.Required["electricity"]) {
			return fmt.Errorf("native VRF branch lost exact original/source-role binding")
		}
		if err := epathSQLVRFCheckDisplay(branch.Value.Value, branch.Value); err != nil {
			return err
		}
		sum = sum.add(branch.Value)
	}
	if err := epathSQLVRFCheckDisplay(sum.Value, p.Value); err != nil {
		return err
	}
	if err := epathSQLVRFCheckDisplay(p.Branches["end_use_to_carrier"].Value.Value, p.PairTo); err != nil {
		return err
	}
	if (p.PairFrom.Value > 0) != (p.PairTo.Value > 0) {
		return fmt.Errorf("native VRF pair contains only one positive endpoint")
	}
	return nil
}

func epathCheckSQLVRFZoneService(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	proof := check.ZoneService
	if proof == nil || proof.NativeVRF == nil || proof.DirectHVAC || proof.AnnualTabular || check.Item.Scope != "zone" || check.Item.Target.Collection != "nodes" || check.Item.Target.Field != "value" || check.Item.Target.Category != proof.Service || check.Item.Target.Basis != proof.Basis || proof.Basis != "service_path_allocation" || check.Quantity == nil {
		return fmt.Errorf("invalid native VRF service proof context")
	}
	p := proof.NativeVRF
	if err := epathSQLValidateVRFZoneServiceProof(proof); err != nil {
		return err
	}
	if p.ZoneName != check.Item.Zone || p.Period != check.Item.Period || p.Service != proof.Service || !reflect.DeepEqual(p.Value, *check.Quantity) || len(proof.Carriers) != 1 || !reflect.DeepEqual(proof.Carriers["electricity"], p.Value) || !reflect.DeepEqual(p.Direct.add(p.Allocated), p.Value) {
		return fmt.Errorf("native VRF proof escaped its exact owner/period/quantity")
	}
	sources, err := epathSQLDirectHVACSourceMap(bundle)
	if err != nil {
		return err
	}
	nodes, links, _, _, err := epathOracleGraph(bundle, "zone", check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	if p.Owned {
		if len(p.Originals) != 1 || len(p.Originals["electricity"]) != 3 || len(p.Required["electricity"]) != 3 || len(p.Weights) == 0 || len(proof.LoadSources) == 0 {
			return fmt.Errorf("native VRF service lost local/shared/denominator original identities")
		}
		if err := epathSQLValidateOriginalSources(p.Originals["electricity"]); err != nil {
			return err
		}
	}
	uses, byID := map[string]bool{}, map[string]EnergyExplanationNode{}
	for _, node := range nodes {
		byID[node.ID] = node
		if node.Level != "end_use" || node.EndUse != proof.Service && node.ServiceKind != proof.Service {
			continue
		}
		if !p.Owned || p.Value.Value == 0 || node.EndUse != proof.Service || node.ServiceKind != "" && node.ServiceKind != proof.Service || node.Carrier != "" || node.Basis != proof.Basis || node.AggregationBasis != "model_total" || node.Multiplier != 1 || node.Unit != "kWh" || node.ScaleDomain != "site" || node.RawValue != 0 || !node.AllocationApplied {
			return fmt.Errorf("unowned/zero/wrong-domain native VRF service node")
		}
		uses[node.ID] = true
		if err := epathSQLVRFCheckDisplay(node.Value, p.Value); err != nil {
			return err
		}
		if err := epathSQLVRFCheckDisplay(node.AllocatedValue, p.Value); err != nil {
			return err
		}
		if err := epathSQLVRFCheckDisplay(node.EffectiveValue, p.Value); err != nil {
			return err
		}
		if err := epathSQLVerifyOriginalSources(node.SourceIDs, sources, p.Originals["electricity"], p.Required["electricity"], p.Period); err != nil {
			return err
		}
	}
	if len(uses) > 1 || p.Value.Value > 0 && len(uses) != 1 {
		return fmt.Errorf("missing/duplicate native VRF canonical service node")
	}
	counts, conversions := map[string]int{}, 0
	for _, link := range links {
		if !uses[link.FromID] && !uses[link.ToID] {
			if link.ServiceKind == proof.Service && (link.Relation == "load_to_end_use" || link.Relation == "end_use_to_carrier" || link.Relation == "direct_end_use_to_carrier") {
				return fmt.Errorf("unowned or wrongly connected native VRF service link")
			}
			continue
		}
		if link.Relation == "source_correspondence" {
			continue
		}
		if link.Relation == "load_to_end_use" {
			conversions++
			from := byID[link.FromID]
			if conversions != 1 || !uses[link.ToID] || link.Basis != proof.Basis || link.ServiceKind != proof.Service || from.Level != "load" || from.ServiceKind != proof.Service || from.ScaleDomain != "thermal" || from.Unit != "kWh" || link.FromUnit != "kWh" || link.ToUnit != "kWh" || p.PairFrom.Value <= 0 || p.PairTo.Value <= 0 {
				return fmt.Errorf("duplicate/unpaired/wrong-load native VRF conversion")
			}
			if err := epathSQLVRFCheckDisplay(link.FromValue, p.PairFrom); err != nil {
				return err
			}
			if err := epathSQLVRFCheckDisplay(link.ToValue, p.PairTo); err != nil {
				return err
			}
			allowed, required := map[string]epathSQLOriginalSource{}, map[string]bool{}
			for id, source := range p.Originals["electricity"] {
				allowed[id], required[id] = source, true
			}
			for id, source := range p.Weights {
				allowed[id], required[id] = source, true
			}
			if err := epathSQLVerifyOriginalSources(link.SourceIDs, sources, allowed, required, p.Period); err != nil {
				return err
			}
			continue
		}
		if !epathSQLVRFZoneCoveredLink(nodes, link, proof) {
			return fmt.Errorf("unreviewed native VRF consumption branch")
		}
		branch := p.Branches[link.Relation]
		counts[link.Relation]++
		if counts[link.Relation] != 1 || !epathOracleFinite(link.FromValue) || link.FromValue != link.ToValue || link.Ratio != 0 || link.RatioKind != "" || link.RatioLabel != "" {
			return fmt.Errorf("duplicate/nonconserving native VRF consumption branch")
		}
		if err := epathSQLVRFCheckDisplay(link.FromValue, branch.Value); err != nil {
			return err
		}
		if err := epathSQLVerifyOriginalSources(link.SourceIDs, sources, branch.Originals, branch.Required, p.Period); err != nil {
			return err
		}
	}
	if p.PairFrom.Value > 0 && p.PairTo.Value > 0 && conversions != 1 {
		return fmt.Errorf("missing complete native VRF conversion")
	}
	for relation, branch := range p.Branches {
		if branch.Value.Value > 0 && counts[relation] != 1 {
			return fmt.Errorf("missing native VRF consumption branch %s", relation)
		}
	}
	return nil
}
