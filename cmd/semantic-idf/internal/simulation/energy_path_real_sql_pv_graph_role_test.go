package simulation

// Finite original-wire roles and native support transport.
// Native9 and the full source/identity/calendar/knownness gates MUST
// have succeeded once at this boundary before FromValidated is called.
// No production classifier, inventory, allocator, private marker or repair is
// consulted. Other consumption/residual scalars keep their existing oracles.
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type epathSQLPVGraphIdentity struct {
	SpecID, Name, Key, Frequency string
	IsMeter, HasNegative         bool
	Amounts                      epathSQLPVGraphAmounts
}

type epathSQLPVGraphRoleProof struct {
	OriginalSHA, ExecutedSHA, SQLSHA, MTDSHA string
	Identities                               map[string]epathSQLPVGraphIdentity
	seal                                     string
}

func epathSQLPVGraphRoleSeal(p epathSQLPVGraphRoleProof) string {
	b, _ := json.Marshal(p) // private seal is deliberately outside the payload.
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// This cheap constructor must be adjacent to the caller's successful native9
// validation. It does not itself bless unvalidated native rows or proof hashes.
// Keep the returned proof local to that evaluation/coverage boundary: never
// retain it in the mutable check registry as a persistent "validated" flag.
func epathSQLPVGraphRolesFromValidated(core epathSQLPVSourceFrames, cg epathSQLPVCogenerationFrames) (epathSQLPVGraphRoleProof, error) {
	p := epathSQLPVGraphRoleProof{OriginalSHA: core.Original.OriginalSHA256, ExecutedSHA: core.ExecutedSHA256, SQLSHA: core.SQLSHA256, MTDSHA: cg.Membership.MTDSHA256, Identities: map[string]epathSQLPVGraphIdentity{}}
	for _, hash := range []string{p.OriginalSHA, p.ExecutedSHA, p.SQLSHA, p.MTDSHA} {
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != 32 {
			return p, fmt.Errorf("PV graph role provenance hash is missing/malformed")
		}
	}
	if len(core.Sources) != 40 || !core.DictionaryCensusComplete || len(cg.Parents) != 2 || len(cg.AncillaryIDs) != 2 || cg.SQLSHA256 != core.SQLSHA256 || cg.Membership.OriginalSHA256 != p.OriginalSHA || cg.Membership.ExecutedSHA256 != p.ExecutedSHA {
		return p, fmt.Errorf("PV graph role proof lost native40/CG2 provenance")
	}
	specs, err := epathSQLPVSourceSpecs(core.Original)
	if err != nil {
		return p, err
	}
	wanted := map[string]epathSQLPVNativeSpec{}
	for _, s := range specs {
		wanted[s.ID] = s
	}
	wanted["cogeneration.electricity"] = epathSQLPVCogenerationParentSpec()
	seen := map[string]bool{}
	add := func(s epathSQLPVSourceIdentity) error {
		want, ok := wanted[s.Spec.ID]
		f := s.Dictionary.Frequency
		id := fmt.Sprintf("sql-rdd-%d", s.Dictionary.Index)
		pair := s.Spec.ID + "/" + f
		if !ok || !reflect.DeepEqual(want, s.Spec) || s.Dictionary.Index <= 0 || s.Dictionary.Index != s.Source.DictionaryIndex || (f != "Monthly" && f != "Hourly") || seen[pair] || p.Identities[id].SpecID != "" || !s.RequestBound || s.Dictionary.Unit != "J" || s.Source.SourceUnit != "J" || s.Source.ReportingFrequency != f || s.Source.IsMeter != want.IsMeter || s.Dictionary.IsMeter != want.IsMeter || !strings.EqualFold(s.Source.Name, want.Name) || !strings.EqualFold(s.Dictionary.Name, s.Source.Name) || !strings.EqualFold(s.Source.KeyValue, want.Key) || !strings.EqualFold(s.Dictionary.Key, s.Source.KeyValue) {
			return fmt.Errorf("PV graph role identity substituted native family/request/dictionary")
		}
		amounts, err := epathSQLPVGraphNativeAmounts(s)
		if err != nil {
			return err
		}
		seen[pair] = true
		p.Identities[id] = epathSQLPVGraphIdentity{SpecID: s.Spec.ID, Name: s.Source.Name, Key: s.Source.KeyValue, Frequency: f, IsMeter: want.IsMeter, HasNegative: s.HasNegativeValue, Amounts: amounts}
		return nil
	}
	for id, s := range core.Sources {
		if id != s.Dictionary.Index {
			return p, fmt.Errorf("PV graph native dictionary key changed")
		}
		if err := add(s); err != nil {
			return p, err
		}
	}
	for _, f := range []string{"Monthly", "Hourly"} {
		s, ok := cg.Parents[f]
		if !ok || s.Dictionary.Frequency != f {
			return p, fmt.Errorf("PV graph parent frequency missing")
		}
		if err := add(s); err != nil {
			return p, err
		}
		a, ok := core.Sources[cg.AncillaryIDs[f]]
		if !ok || a.Spec.ID != "inverter.ancillary" || a.Dictionary.Frequency != f {
			return p, fmt.Errorf("PV graph parent membership changed")
		}
	}
	for id := range wanted {
		for _, f := range []string{"Monthly", "Hourly"} {
			if !seen[id+"/"+f] {
				return p, fmt.Errorf("PV graph required role pair missing")
			}
		}
	}
	if len(p.Identities) != 42 {
		return p, fmt.Errorf("PV graph role census differs")
	}
	p.seal = epathSQLPVGraphRoleSeal(p)
	return p, nil
}

type epathSQLPVGraphRoleWalk struct {
	proof            epathSQLPVGraphRoleProof
	sources          map[string]EnergyDataSource
	negativeProduced bool
	zoneBudgetBundle PurposeResultBundle
	zoneBudgetChecks *epathSQLModelChecks
	zoneBudgetNodes  map[string]EnergyExplanationNode
}

func epathSQLCheckRequiredPVGraph(bundle PurposeResultBundle, checks epathSQLModelChecks, prepared epathSQLPVCogenerationValidatedSources) error {
	if !checks.PVHVACRequired {
		return nil // source-only diagnostics are not full fixture acceptance
	}
	if prepared.graph == nil {
		return fmt.Errorf("full Shop graph lost its boundary-prepared native role proof")
	}
	return epathSQLCheckPVGraphRoles(bundle, *prepared.graph, checks)
}

// Source labels never authorize a role. Traverse IDs and derived input IDs so
// a forbidden native constituent cannot be laundered through a renamed source.
func (w epathSQLPVGraphRoleWalk) roles(ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	visiting := map[string]bool{}
	done := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if id == "" || visiting[id] {
			return fmt.Errorf("PV graph has blank/cyclic source ancestry")
		}
		if done[id] {
			return nil
		}
		s, ok := w.sources[id]
		if !ok {
			return fmt.Errorf("PV graph source %s absent from original wire", id)
		}
		visiting[id] = true
		if p, ok := w.proof.Identities[id]; ok {
			out[p.SpecID] = true
		}
		for _, parent := range s.InputSourceIDs {
			if err := visit(parent); err != nil {
				return err
			}
		}
		delete(visiting, id)
		done[id] = true
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func epathSQLPVGraphSupport(endUse, kind string) string {
	if endUse == "" {
		endUse = strings.TrimPrefix(kind, "energy.")
	}
	switch endUse {
	case "storage_charge":
		return "storage.charge"
	case "storage_discharge":
		return "storage.discharge"
	case "generators", "onsite_production", "production":
		return "facility.produced"
	case "electricity_purchased", "purchased", "purchased_electricity":
		return "facility.purchased"
	case "electricity_sold", "sold", "sold_electricity", "surplus_sold":
		return "facility.sold"
	}
	return ""
}

func (w epathSQLPVGraphRoleWalk) single(ids []string, spec string) bool {
	return len(ids) == 1 && w.proof.Identities[ids[0]].SpecID == spec && w.proof.Identities[ids[0]].Frequency == "Monthly"
}

// Canonical Other ribbons can aggregate unrelated consumed sources. Exactly
// one actual Monthly CG parent must be present; no other protected PV role or
// derived wrapper can replace it. Site-flow proof still owns the full amount.
func (w epathSQLPVGraphRoleWalk) consumedParent(ids []string) bool {
	count := 0
	for _, id := range ids {
		if p, ok := w.proof.Identities[id]; ok {
			if p.SpecID != "cogeneration.electricity" || p.Frequency != "Monthly" {
				return false
			}
			count++
		} else {
			ancestors, err := w.roles([]string{id})
			if err != nil || len(ancestors) > 0 {
				return false
			}
		}
	}
	roles, err := w.roles(ids)
	return err == nil && len(roles) == 1 && roles["cogeneration.electricity"] && count == 1
}

func epathSQLPVGraphConsumedService(service string) bool {
	return service == "" || service == "other" || service == "cogeneration_input"
}

func epathSQLPVGraphUnallocated(n EnergyExplanationNode) bool {
	// Graph AllocatedValue is also the historical display scalar for an
	// unallocated node; it is not the source-level allocation ownership bit.
	return n.ZoneName == "" && n.ServiceKind == "" && n.PathType == "" && n.LoopName == "" && len(n.RelatedPathIDs) == 0 && !n.AllocationApplied && n.AllocationExplanation == "" && n.Basis != "service_path_allocation" && n.Basis != "zone_load_allocation" && n.AggregationBasis != "representative_zone" && n.AggregationBasis != "zone_subtotal"
}

func (w epathSQLPVGraphRoleWalk) noNative(ids []string) error {
	r, e := w.roles(ids)
	if e != nil {
		return e
	}
	if len(r) != 0 {
		return fmt.Errorf("PV native source entered thermal/Zone/ratio context")
	}
	return nil
}

func (w epathSQLPVGraphRoleWalk) graph(nodes []EnergyExplanationNode, links []EnergyPathLink, rows []EnergyReconciliation, period string, zone bool) error {
	byID := map[string]EnergyExplanationNode{}
	support := map[string]string{}
	counts := map[string]int{}
	for _, n := range nodes {
		if n.ID == "" {
			return fmt.Errorf("PV graph blank node identity")
		}
		if _, ok := byID[n.ID]; ok {
			return fmt.Errorf("PV graph duplicate node identity")
		}
		byID[n.ID] = n
		r, err := w.roles(n.SourceIDs)
		if err != nil {
			return err
		}
		role := epathSQLPVGraphSupport(n.EndUse, n.Kind)
		if role != "" {
			if zone || role == "storage.discharge" || n.Level != "support" || n.Carrier != "electricity" || n.Unit != "kWh" || n.ScaleDomain != "site" || !epathSQLPVGraphUnallocated(n) || !w.single(n.SourceIDs, role) || role == "facility.produced" && w.negativeProduced {
				return fmt.Errorf("PV support promoted component/charge/Zone/signed source or borrowed role")
			}
			counts[role]++
			if counts[role] > 1 {
				return fmt.Errorf("PV duplicated native M/H or component support role")
			}
			support[n.ID] = role
		} else {
			zoneBudget := false
			if zone && r["facility.demand"] {
				var err error
				zoneBudget, err = w.zoneBudgetNode(n, period)
				if err != nil {
					return err
				}
			}
			for native := range r {
				if zoneBudget && native == "facility.demand" {
					continue
				}
				ownership := n
				if native == "cogeneration.electricity" && n.Level == "end_use" && epathSQLPVGraphConsumedService(n.ServiceKind) {
					ownership.ServiceKind = ""
				}
				if zone || !epathSQLPVGraphUnallocated(ownership) || n.Unit != "kWh" || n.ScaleDomain != "site" {
					return fmt.Errorf("PV native graph source became Zone/path allocation: period=%s zone=%t node=%s native=%s level=%s service=%s path=%s loop=%s related=%v basis=%s aggregation=%s unit=%s domain=%s allocation=%t explanation=%q", period, zone, n.ID, native, n.Level, n.ServiceKind, n.PathType, n.LoopName, n.RelatedPathIDs, n.Basis, n.AggregationBasis, n.Unit, n.ScaleDomain, n.AllocationApplied, n.AllocationExplanation)
				}
				// A site residual carries demand and consumed-input lineage, not
				// another copy of either full quantity. Existing residual scalar
				// proof remains mandatory; no support/component ID is permitted.
				if n.Level == "residual" && n.Basis == "residual" && n.Carrier == "electricity" && (native == "facility.demand" || native == "cogeneration.electricity") {
					continue
				}
				switch native {
				case "facility.demand":
					if n.Level != "carrier" || n.Carrier != "electricity" || !w.single(n.SourceIDs, native) {
						return fmt.Errorf("Facility demand is not sole consumption carrier authority")
					}
				case "cogeneration.electricity":
					if n.Level != "end_use" || (n.EndUse != "other" && n.EndUse != "cogeneration_input") || (n.Carrier != "" && n.Carrier != "electricity") || !w.consumedParent(n.SourceIDs) {
						return fmt.Errorf("Cogeneration parent is not Building consumed input")
					}
				default:
					return fmt.Errorf("PV component/transfer/thermal source became additive graph authority: %s", native)
				}
				counts[native]++
				if counts[native] > 1 {
					return fmt.Errorf("PV native consumed role duplicated")
				}
			}
		}
		for _, part := range n.LoadBreakdown {
			if err := w.noNative(part.SourceIDs); err != nil {
				return err
			}
		}
		for _, part := range n.OffsetEffects {
			if err := w.noNative(part.SourceIDs); err != nil {
				return err
			}
		}
		if n.SimultaneousLoad != nil {
			if err := w.noNative(n.SimultaneousLoad.SourceIDs); err != nil {
				return err
			}
		}
	}
	supplyCount := map[string]int{}
	for _, l := range links {
		r, err := w.roles(l.SourceIDs)
		if err != nil {
			return err
		}
		from, to := byID[l.FromID], byID[l.ToID]
		if support[l.FromID] == "storage.charge" || support[l.ToID] == "storage.charge" {
			return fmt.Errorf("storage charge support has a flow edge, including zero flow")
		}
		if (support[l.FromID] != "" || support[l.ToID] != "") && l.Relation != "support_supply" {
			return fmt.Errorf("PV support endpoint acquired a non-supply flow")
		}
		if l.Relation == "support_supply" {
			role := support[l.FromID]
			if zone || role == "" || role == "storage.charge" || to.Level != "carrier" || to.Carrier != "electricity" || !w.single(to.SourceIDs, "facility.demand") || !w.single(l.SourceIDs, role) || l.ZoneName != "" || l.ServiceKind != "" || len(l.RelatedPathIDs) > 0 || l.RatioKind != "" || l.Ratio != 0 || l.FromUnit != "kWh" || l.ToUnit != "kWh" {
				return fmt.Errorf("PV supply edge lacks one exact global native support/carrier role")
			}
			supplyCount[role]++
			if supplyCount[role] > 1 {
				return fmt.Errorf("PV support supply edge duplicated")
			}
			continue
		}
		for native := range r {
			if !zone && l.Relation == "residual" && from.Level == "residual" && from.Basis == "residual" && to.Level == "carrier" && to.Carrier == "electricity" && l.Basis == "residual" && l.ZoneName == "" && l.ServiceKind == "" && len(l.RelatedPathIDs) == 0 && l.RatioKind == "" && l.Ratio == 0 && (native == "facility.demand" || native == "cogeneration.electricity") {
				continue
			}
			if zone || native != "cogeneration.electricity" || l.Relation != "direct_end_use_to_carrier" || from.Level != "end_use" || (from.EndUse != "other" && from.EndUse != "cogeneration_input") || to.Level != "carrier" || to.Carrier != "electricity" || !w.consumedParent(l.SourceIDs) || l.ZoneName != "" || !epathSQLPVGraphConsumedService(l.ServiceKind) || len(l.RelatedPathIDs) > 0 || l.RatioKind != "" || l.Ratio != 0 {
				return fmt.Errorf("PV source entered consumption/supply/thermal edge with wrong role")
			}
		}
	}
	for _, row := range rows {
		r, err := w.roles(row.SourceIDs)
		if err != nil {
			return err
		}
		if zone && r["facility.demand"] {
			allowed, err := w.zoneBudgetRow(row, period, nodes)
			if err != nil {
				return err
			}
			if allowed {
				continue
			}
		}
		for native := range r {
			if zone || row.Level != "energy" || row.ZoneName != "" || row.ServiceKind != "" || row.AllocationMethod != "" || row.AllocatedValue != 0 || (native != "facility.demand" && native != "cogeneration.electricity") {
				return fmt.Errorf("PV context source entered reconciliation or allocation")
			}
		}
	}
	return w.supportAmounts(nodes, links, period, zone)
}

func (w epathSQLPVGraphRoleWalk) items(items []EnergyExplanationSummaryItem, collection string, zone bool, context ...string) error {
	counts := map[string]int{}
	for _, item := range items {
		r, err := w.roles(item.SourceIDs)
		if err != nil {
			return err
		}
		if epathSQLPVGraphSupport(item.EndUse, item.Kind) != "" {
			return fmt.Errorf("PV support/charge became numeric cached summary")
		}
		for native := range r {
			counts[native]++
			if counts[native] > 1 {
				return fmt.Errorf("PV native role duplicated inside one cached collection")
			}
			if zone && native == "facility.demand" && w.zoneBudgetItem(item, collection, context) {
				continue
			}
			serviceOK := item.ServiceKind == "" || native == "cogeneration.electricity" && collection == "endUses" && epathSQLPVGraphConsumedService(item.ServiceKind)
			if zone || item.ZoneName != "" || !serviceOK || item.PathType != "" || item.Basis == "service_path_allocation" || item.Basis == "zone_load_allocation" || item.AggregationBasis == "representative_zone" || item.AggregationBasis == "zone_subtotal" {
				return fmt.Errorf("PV native source became cached Zone/path allocation")
			}
			switch native {
			case "facility.demand":
				if !(collection == "carriers" && item.Carrier == "electricity") && !(collection == "residuals" && item.Level == "residual" && item.Basis == "residual" && item.Carrier == "electricity") {
					return fmt.Errorf("Facility demand cache changed consumption role")
				}
			case "cogeneration.electricity":
				if !(collection == "endUses" && (item.EndUse == "other" || item.EndUse == "cogeneration_input") && w.consumedParent(item.SourceIDs)) && !(collection == "residuals" && item.Level == "residual" && item.Basis == "residual" && item.Carrier == "electricity") {
					return fmt.Errorf("Cogeneration cache changed consumed-input role")
				}
			default:
				return fmt.Errorf("PV component/charge/thermal source entered cached numeric summary")
			}
		}
	}
	return nil
}

func (w epathSQLPVGraphRoleWalk) summary(s EnergyExplanationSummary, zone bool, context ...string) error {
	zone = zone || s.Scope.Kind == "zone" || s.Scope.ZoneName != ""
	for _, group := range []struct {
		name  string
		items []EnergyExplanationSummaryItem
	}{{"drivers", s.Drivers}, {"loads", s.Loads}, {"endUses", s.EndUses}, {"carriers", s.Carriers}, {"ratios", s.Ratios}, {"residuals", s.Residuals}, {"topZones", s.TopZones}, {"carriers", s.EnergyByCarrier}, {"endUses", s.EnergyByEndUse}, {"loads", s.DeliveredLoadByService}, {"ratios", s.DerivedKPIs}, {"drivers", s.HeatDrivers}, {"drivers", s.TopHeatDrivers}} {
		if err := w.items(group.items, group.name, zone, context...); err != nil {
			return err
		}
	}
	return nil
}

func (w epathSQLPVGraphRoleWalk) periods(periods []EnergyPeriod, zone bool) error {
	seen := map[string]bool{}
	for _, period := range periods {
		if seen[period.ID] {
			return fmt.Errorf("PV graph period duplicated")
		}
		seen[period.ID] = true
		if len(period.Edges) > 0 {
			return fmt.Errorf("original-wire v2 PV guard cannot silently skip legacy period edges")
		}
		if err := w.graph(period.Nodes, period.Links, period.Reconciliation, period.ID, zone); err != nil {
			return fmt.Errorf("%s: %w", period.ID, err)
		}
		if period.Summary != nil {
			if err := w.summary(*period.Summary, zone, period.ID); err != nil {
				return err
			}
		}
		if err := w.items(period.ZoneContributions, "zoneContributions", true); err != nil {
			return err
		}
	}
	if !zone {
		if !seen["annual"] {
			return fmt.Errorf("PV graph annual period copy absent")
		}
		for m := 1; m <= 12; m++ {
			if !seen[fmt.Sprintf("M%d", m)] {
				return fmt.Errorf("PV graph required native monthly context absent")
			}
		}
	}
	return nil
}

func epathSQLCheckPVGraphRoles(bundle PurposeResultBundle, p epathSQLPVGraphRoleProof, checks ...epathSQLModelChecks) error {
	if len(p.Identities) != 42 || p.seal == "" || p.seal != epathSQLPVGraphRoleSeal(p) {
		return fmt.Errorf("PV graph role guard lost its prepared native proof")
	}
	if len(checks) > 1 {
		return fmt.Errorf("PV graph has ambiguous independent check registry")
	}
	w := epathSQLPVGraphRoleWalk{proof: p, sources: map[string]EnergyDataSource{}, zoneBudgetBundle: bundle, zoneBudgetNodes: map[string]EnergyExplanationNode{}}
	if len(checks) == 1 {
		w.zoneBudgetChecks = &checks[0]
	}
	r := bundle.EnergyExplanation
	if r.Schema != energyExplanationSchema || r.Scope.Kind != "building" || r.Scope.ZoneName != "" || len(r.Edges) > 0 {
		return fmt.Errorf("PV graph guard needs unmodified original-wire v2 Building root")
	}
	for _, s := range r.Sources {
		if s.ID == "" {
			return fmt.Errorf("PV graph source has blank ID")
		}
		if _, ok := w.sources[s.ID]; ok {
			return fmt.Errorf("PV graph source ID duplicated")
		}
		w.sources[s.ID] = s
	}
	for id, native := range p.Identities {
		s, ok := w.sources[id]
		if !ok || s.SourceType != "sql_report_data" || s.Name != native.Name || s.KeyValue != native.Key || s.IsMeter != native.IsMeter || s.ReportingFrequency != native.Frequency || s.SourceUnit != "J" {
			return fmt.Errorf("PV graph guard lost exact native source identity")
		}
		if native.SpecID == "facility.produced" && native.HasNegative {
			w.negativeProduced = true
		}
	}
	for _, s := range r.Sources {
		roles, err := w.roles([]string{s.ID})
		if err != nil {
			return err
		}
		if len(roles) > 0 && (s.ZoneName != "" || len(s.ScopeDetails) > 0 || s.AllocationApplied || s.AllocatedValue != 0 || s.AllocationFactor != 0 || s.AllocationFormula != "" || s.AllocationExplanation != "") {
			return fmt.Errorf("PV native ancestry has source-level Zone/allocation metadata")
		}
	}
	if err := w.graph(r.Nodes, r.Links, r.Reconciliation, "annual", false); err != nil {
		return err
	}
	if err := w.summary(bundle.EnergyExplanationSummary, false); err != nil {
		return err
	}
	if err := w.items(r.ZoneContributions, "zoneContributions", true); err != nil {
		return err
	}
	if err := w.periods(r.Periods, false); err != nil {
		return err
	}
	for _, z := range r.ZoneResults {
		if z.Scope.Kind != "zone" || z.Scope.ZoneName == "" {
			return fmt.Errorf("PV graph Zone wrapper malformed")
		}
		if err := w.graph(z.Nodes, z.Links, z.Reconciliation, "annual", true); err != nil {
			return err
		}
		if err := w.summary(z.Summary, true, "annual"); err != nil {
			return err
		}
		if err := w.items(z.ZoneContributions, "zoneContributions", true); err != nil {
			return err
		}
		if err := w.periods(z.Periods, true); err != nil {
			return err
		}
	}
	return nil
}
