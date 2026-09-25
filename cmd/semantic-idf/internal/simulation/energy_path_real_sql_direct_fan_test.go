package simulation

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

type epathSQLDirectFanProof struct {
	Zone, Period, SiteID string
	Value                epathSQLQuantity
	Sources              map[string]epathSQLDirectHVACSourceIdentity
}

type epathSQLDirectFanFrames struct {
	MeterSource   epathRealSQLSource
	Precision     epathRealSQLPrecision
	SiteID        string
	Zones         map[string][12]epathSQLQuantity
	Sources       map[string]map[string]epathSQLDirectHVACSourceIdentity
	Meter, Direct [12]epathSQLQuantity
}

// A fan is an additive native site consumer, not a cooling load or a share
// of an AirLoop pool. Every declared owner/month is an observation, including 0.
func epathSQLCompileDirectFans(frames epathSQLFrames, model epathRealSQLModel) (*epathSQLDirectFanFrames, error) {
	var declaration *epathRealSQLDirectHVACComponent
	for index := range model.DirectHVACComponents {
		item := &model.DirectHVACComponents[index]
		if !epathSQLSupportedDirectFanFamily(item.ID) {
			continue
		}
		if declaration != nil {
			return nil, fmt.Errorf("duplicate native direct fan declaration")
		}
		declaration = item
	}
	if declaration == nil {
		for _, identity := range frames.DirectHVACSourceIdentities {
			if epathSQLSupportedDirectFanFamily(identity.FamilyID) || identity.Service == "fans" {
				return nil, fmt.Errorf("undeclared native fan source")
			}
		}
		return nil, nil
	}
	owners, err := epathSQLDirectHVACOwners(*declaration)
	if err != nil {
		return nil, err
	}
	out := &epathSQLDirectFanFrames{SiteID: declaration.SiteID, Zones: map[string][12]epathSQLQuantity{}, Sources: map[string]map[string]epathSQLDirectHVACSourceIdentity{}}
	sites := 0
	for _, site := range model.Site {
		if site.ID != out.SiteID {
			continue
		}
		sites++
		if site.EndUse != "fans" || site.Carrier != "electricity" || site.Facility || site.Tabular != nil || !site.Source.IsMeter || len(site.Source.Alternatives) != 1 || site.Source.Alternatives[0] != (epathRealSQLAlternative{Name: "Fans:Electricity", Unit: "J"}) {
			return nil, fmt.Errorf("direct fan needs its exact Monthly broad fan meter")
		}
	}
	if sites != 1 || len(frames.SiteSources[out.SiteID]) != 1 {
		return nil, fmt.Errorf("native fan needs one independently observed broad fan source")
	}
	meterID := frames.SiteSources[out.SiteID][0]
	meter := frames.SourceIdentities[meterID]
	if meter.Name != "Fans:Electricity" || !meter.IsMeter || meter.ReportingFrequency != "Monthly" || meter.SourceUnit != "J" || meter.Rows != 12 || meter.MissingRows != 0 {
		return nil, fmt.Errorf("native fan broad source is absent/incomplete")
	}
	out.MeterSource, out.Precision = meter, model.Precision
	for _, pool := range model.FanPools {
		if pool.SiteID == out.SiteID {
			return nil, fmt.Errorf("native local fans cannot also consume a shared AirLoop fan pool")
		}
	}
	auxiliaries := 0
	for _, aux := range model.Auxiliaries {
		if aux.SiteID != out.SiteID {
			continue
		}
		auxiliaries++
		if aux.Weight != "native_direct" || aux.AllocationMethod != "direct_only" || aux.WeightSource != nil || aux.ReconciliationID != "reconcile.zone_auxiliary_allocation.fans.electricity.annual" {
			return nil, fmt.Errorf("native fan auxiliary policy cannot invent thermal/flow weighting")
		}
		served, err := epathSQLDeclaredZones(frames, aux.ServedZones)
		if err != nil {
			return nil, err
		}
		ownerZones := map[string]bool{}
		for _, owner := range owners {
			ownerZones[strings.ToLower(owner.ZoneName)] = true
		}
		if !reflect.DeepEqual(served, ownerZones) {
			return nil, fmt.Errorf("fan auxiliary ownership differs from exact native owners")
		}
	}
	if auxiliaries != 1 {
		return nil, fmt.Errorf("native direct fan lacks its single explicit auxiliary consumer contract")
	}
	seen := map[string]bool{}
	for id, identity := range frames.DirectHVACSourceIdentities {
		if (epathSQLSupportedDirectFanFamily(identity.FamilyID) || identity.Service == "fans") && identity.FamilyID != declaration.ID {
			return nil, fmt.Errorf("native fan source belongs to an undeclared or mixed family cohort")
		}
		if identity.FamilyID != declaration.ID {
			continue
		}
		key := strings.ToLower(identity.Owner.KeyValue)
		owner, exists := owners[key]
		if !exists || seen[key] || identity.SiteID != out.SiteID || identity.Source.DictionaryIndex != id || !reflect.DeepEqual(owner, identity.Owner) {
			return nil, fmt.Errorf("native fan has duplicate/foreign owner or source")
		}
		if err := epathSQLValidateDirectHVACSourceIdentity(identity); err != nil {
			return nil, err
		}
		seen[key] = true
		zone := strings.ToLower(owner.ZoneName)
		if frames.Zones[zone].Name == "" {
			return nil, fmt.Errorf("native fan belongs to unknown Zone")
		}
		if out.Sources[zone] == nil {
			out.Sources[zone] = map[string]epathSQLDirectHVACSourceIdentity{}
		}
		out.Sources[zone][fmt.Sprintf("sql-rdd-%d", id)] = identity
		values, err := epathSQLMonthly(identity.Source, identity.Precision)
		if err != nil {
			return nil, err
		}
		months := out.Zones[zone]
		for month, value := range values {
			months[month] = months[month].add(value.positive())
			out.Direct[month] = out.Direct[month].add(value.positive())
		}
		out.Zones[zone] = months
	}
	if len(seen) != len(owners) {
		return nil, fmt.Errorf("missing native direct fan observation is not zero")
	}
	for month := 1; month <= 12; month++ {
		value, err := epathSQLSiteSum(frames, []string{out.SiteID}, month)
		if err != nil || !value.valid() {
			return nil, fmt.Errorf("native fan missing broad monthly budget: %v", err)
		}
		out.Meter[month-1] = value.positive()
		// This reviewed cohort is the complete original fan meter roster. Do
		// not bless a leftover as a hidden fan or fit a scale to close it.
		if math.Abs(value.Value-out.Direct[month-1].Value) > 1e-10*math.Max(1, value.Value) {
			return nil, fmt.Errorf("native fan source roster does not close original Monthly meter")
		}
		for zone := range frames.Zones {
			observation, exists := frames.DirectHVAC[epathSQLDirectHVACKey(zone, "fans", "electricity", month)]
			_, owned := out.Sources[zone]
			if exists != owned {
				return nil, fmt.Errorf("native fan frame has absent/undeclared ownership")
			}
			if !owned {
				continue
			}
			if !observation.Present || !epathSQLZoneCarrierQuantityEqual(observation.Quantity, out.Zones[zone][month-1]) || len(observation.SourceIDs) != len(out.Sources[zone]) {
				return nil, fmt.Errorf("native fan month differs from independent source sum")
			}
			ids := map[int]bool{}
			for _, id := range observation.SourceIDs {
				if ids[id] || out.Sources[zone][fmt.Sprintf("sql-rdd-%d", id)].Source.DictionaryIndex != id {
					return nil, fmt.Errorf("native fan frame has duplicate/foreign source")
				}
				ids[id] = true
			}
		}
	}
	return out, nil
}

func epathSQLModelDirectFanChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	fans, err := epathSQLCompileDirectFans(frames, model)
	if err != nil || fans == nil {
		return err
	}
	zones := []string{}
	for _, zone := range frames.Zones {
		zones = append(zones, zone.Name)
	}
	sort.Strings(zones)
	for _, period := range epathSQLZoneCarrierPeriods() {
		start := len(checks.Rows)
		expected, direct := epathSQLQuantity{}, epathSQLQuantity{}
		for _, month := range epathSQLPeriodMonths(period) {
			expected = expected.add(fans.Meter[month-1])
			direct = direct.add(fans.Direct[month-1])
		}
		// Independent SQL closes exactly; presentation can expose a small
		// positive or negative rounding residual without changing native sums.
		residual := expected.add(direct.times(-1)).positive()
		if math.Abs(residual.Value) < 1e-10*math.Max(1, expected.Value) {
			low, high := residual.bounds()
			residual = epathSQLBounded(0, math.Min(0, low), high)
		}
		for _, field := range []string{"expectedValue", "directValue", "allocatedValue", "unassignedValue"} {
			q := epathSQLQuantity{}
			switch field {
			case "expectedValue":
				q = expected
			case "directValue":
				q = direct
			case "unassignedValue":
				q = residual
			}
			target := epathRealOracleTarget{Collection: "reconciliation", ID: "reconcile.zone_auxiliary_allocation.fans.electricity." + strings.ToLower(period), Level: "allocation", Field: field, Unit: "kWh"}
			if err := checks.add("zoneAllocation", "building", "", period, fans.SiteID+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
				return err
			}
		}
		if err := checks.bindAllocationProof(start); err != nil {
			return err
		}
		checks.Rows[start].Allocation.DirectFan = fans
		for _, zone := range zones {
			key := strings.ToLower(zone)
			q := epathSQLQuantity{}
			for _, month := range epathSQLPeriodMonths(period) {
				q = q.add(fans.Zones[key][month-1])
			}
			proof := &epathSQLDirectFanProof{Zone: zone, Period: period, SiteID: fans.SiteID, Value: q, Sources: fans.Sources[key]}
			for _, field := range []string{"value", "rawValue", "effectiveValue", "allocatedValue"} {
				target := epathSQLNodeTarget("end_use", "fans", "", "site")
				target.Field, target.Basis = field, "direct_zone_energy"
				target.AllowPrunedZero = field == "value" || field == "allocatedValue"
				if err := checks.add("zoneAllocation", "zone", zone, period, "native_fan/"+field, "kWh", &q, target, "", nil, nil); err != nil {
					return err
				}
				check := &checks.Rows[len(checks.Rows)-1]
				check.DirectFan = proof
				if field == "rawValue" || field == "effectiveValue" {
					// Only the whole observed-zero presentation may disappear;
					// the native source and every present scalar remain required.
					check.OptionalPresentation = q.includesZero()
				}
			}
		}
	}
	return nil
}

func epathSQLDirectFanProofQuantity(proof *epathSQLDirectFanProof) error {
	if proof == nil || proof.Zone == "" || !epathOracleValidPeriod(proof.Period) || proof.SiteID == "" || !proof.Value.valid() {
		return fmt.Errorf("invalid native fan consumer proof")
	}
	q := epathSQLQuantity{}
	family := ""
	for id, identity := range proof.Sources {
		if !epathSQLSupportedDirectFanFamily(identity.FamilyID) || family != "" && family != identity.FamilyID || identity.SiteID != proof.SiteID || !strings.EqualFold(identity.Owner.ZoneName, proof.Zone) || id != fmt.Sprintf("sql-rdd-%d", identity.Source.DictionaryIndex) {
			return fmt.Errorf("native fan consumer source belongs to another owner/role")
		}
		family = identity.FamilyID
		if err := epathSQLValidateDirectHVACSourceIdentity(identity); err != nil {
			return err
		}
		months, _ := epathSQLMonthly(identity.Source, identity.Precision)
		for _, month := range epathSQLPeriodMonths(proof.Period) {
			q = q.add(months[month-1].positive())
		}
	}
	if !epathSQLZoneCarrierQuantityEqual(q, proof.Value) {
		return fmt.Errorf("native fan consumer differs from original source energy")
	}
	return nil
}

func epathCheckSQLDirectFan(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	p := check.DirectFan
	if err := epathSQLDirectFanProofQuantity(p); err != nil {
		return err
	}
	if check.Quantity == nil || !epathSQLZoneCarrierQuantityEqual(*check.Quantity, p.Value) || check.Item.Scope != "zone" || check.Item.Zone != p.Zone || check.Item.Period != p.Period {
		return fmt.Errorf("native fan proof escaped its exact Zone/period/scalar")
	}
	nodes, links, _, _, err := epathOracleGraph(bundle, "zone", p.Zone, p.Period)
	if err != nil {
		return err
	}
	actualSources, err := epathSQLDirectHVACSourceMap(bundle)
	if err != nil {
		return err
	}
	for id, identity := range p.Sources {
		source, exists := actualSources[id]
		if !exists {
			return fmt.Errorf("native fan source missing even at measured zero")
		}
		if err := epathSQLMatchDirectHVACSource(source, identity); err != nil {
			return err
		}
	}
	verify := func(ids []string) error {
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] || p.Sources[id].Source.DictionaryIndex == 0 {
				return fmt.Errorf("fan trace contains a foreign or duplicate consuming source")
			}
			seen[id] = true
		}
		if len(seen) != len(p.Sources) {
			return fmt.Errorf("native fan trace lost exact original source")
		}
		return nil
	}
	byID := map[string]EnergyExplanationNode{}
	selected := map[string]bool{}
	for _, node := range nodes {
		byID[node.ID] = node
		if node.Level != "end_use" || node.EndUse != "fans" {
			continue
		}
		if node.Basis != "direct_zone_energy" || node.Unit != "kWh" || node.ScaleDomain != "site" || node.AggregationBasis != "model_total" || node.AllocationApplied {
			return fmt.Errorf("native local fan was marked shared or lost model-total units")
		}
		if err := verify(node.SourceIDs); err != nil {
			return err
		}
		for _, value := range []float64{node.Value, node.RawValue, node.EffectiveValue, node.AllocatedValue} {
			if err := epathCheckSQLModelQuantity(&value, &p.Value); err != nil {
				return err
			}
		}
		selected[node.ID] = true
	}
	if len(selected) > 1 || len(selected) == 0 && !p.Value.includesZero() {
		return fmt.Errorf("native fan has missing/duplicate Zone end-use node")
	}
	count := 0
	for _, link := range links {
		from, to := byID[link.FromID], byID[link.ToID]
		if from.EndUse != "fans" && to.EndUse != "fans" {
			continue
		}
		if link.Relation == "source_correspondence" {
			continue
		}
		if !selected[link.FromID] || to.Level != "carrier" || to.Carrier != "electricity" || link.Relation != "direct_end_use_to_carrier" || link.Basis != "direct_zone_energy" || link.FromUnit != "kWh" || link.ToUnit != "kWh" || link.Ratio != 0 || link.RatioKind != "" {
			return fmt.Errorf("fan consumer created an allocated/thermal/foreign-carrier branch")
		}
		if err := verify(link.SourceIDs); err != nil {
			return err
		}
		if err := epathCheckSQLModelQuantity(&link.FromValue, &p.Value); err != nil {
			return err
		}
		if link.ToValue != link.FromValue {
			return fmt.Errorf("native fan branch lost site conservation")
		}
		count++
	}
	if count > 1 || count == 0 && !p.Value.includesZero() {
		return fmt.Errorf("native fan has missing/duplicate carrier branch")
	}
	return nil
}
