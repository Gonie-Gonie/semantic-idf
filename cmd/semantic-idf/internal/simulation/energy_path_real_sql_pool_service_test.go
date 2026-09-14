package simulation

// Independent acceptance support. Future glue supplies Frames.PoolSystems, ModelCheck.PoolBoundary
// and ZoneService.NativePool. No generic/production helper is changed here.
import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const epathSQLPoolHeatingUnquantified = "non_zone_demand_unquantified"

type epathSQLPoolHeatingService struct {
	Native  epathSQLPoolSourceFrames
	Service epathRealSQLService
	Zones   []string
	Parents map[string]epathSQLPoolParentClosure
}

func epathSQLCompilePoolHeatingService(frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService) (*epathSQLPoolHeatingService, error) {
	if service.Service != "heating" || len(frames.PoolSystems) != 1 || len(model.PoolSystems) != 1 || !reflect.DeepEqual(model.PoolSystems[0], frames.PoolSystems[0].Original.Declaration) || model.OriginalZoneMultiplierProof != epathSQLOriginalMultiplierContract {
		return nil, fmt.Errorf("Pool Heating requires one original/native system and explicit multiplier proof")
	}
	native := frames.PoolSystems[0]
	zones, err := epathSQLPoolBoundaryOriginalZones(native.Original)
	if err != nil {
		return nil, err
	}
	if err := epathSQLValidatePoolSourceFrames(native); err != nil {
		return nil, err
	}
	if native.Precision != model.Precision || service.Basis != "service_path_allocation" || service.FallbackBasis != "zone_load_allocation" || service.RatioKind != "load_to_site_energy" || service.FallbackRatioKind != "load_to_site_energy" || service.ReconciliationID != "" || len(service.SiteIDs) != 2 || len(service.CarrierReconciliationIDs) != 2 {
		return nil, fmt.Errorf("Pool Heating declaration lost its exact carrier-separated semantic boundary")
	}
	served, err := epathSQLDeclaredZones(frames, service.ServedZones)
	if err != nil || len(served) != 5 {
		return nil, fmt.Errorf("Pool Heating lacks the complete original served roster: %v", err)
	}
	for _, zone := range native.Original.ServedZones {
		if !served[epathSQLPoolBoundaryKey(zone)] {
			return nil, fmt.Errorf("Pool Heating service substituted an original recipient")
		}
	}
	if len(frames.Zones) != len(zones) {
		return nil, fmt.Errorf("Pool Heating canonical Zone census differs from original")
	}
	out := &epathSQLPoolHeatingService{Native: native, Service: service, Parents: map[string]epathSQLPoolParentClosure{}}
	for key, zone := range frames.Zones {
		if !zones[key] || key != strings.ToLower(zone.Name) || !epathOracleFinite(zone.Multiplier) || zone.Multiplier <= 0 {
			return nil, fmt.Errorf("Pool Heating has a foreign/invalid canonical Zone")
		}
		out.Zones = append(out.Zones, zone.Name)
		for month := 1; month <= 12; month++ {
			q, found := frames.Loads[epathSQLKey(key, "heating", month)]
			if !found || !q.valid() || q.Value < 0 {
				return nil, fmt.Errorf("Pool Heating demand is observed separately; missing load cannot be called zero")
			}
		}
	}
	sort.Strings(out.Zones)
	sites := map[string]epathRealSQLSite{}
	for _, site := range model.Site {
		if site.ID == "" || sites[site.ID].ID != "" {
			return nil, fmt.Errorf("Pool Heating has a duplicate/empty site identity")
		}
		sites[site.ID] = site
		if site.EndUse == "heating" && site.ID != "heating.electricity" && site.ID != "heating.natural_gas" {
			return nil, fmt.Errorf("Pool Heating contains an unreviewed extra source pool")
		}
	}
	seen := map[string]bool{}
	for _, id := range service.SiteIDs {
		site, found := sites[id]
		carrier := ""
		switch id {
		case "heating.electricity":
			carrier = "electricity"
		case "heating.natural_gas":
			carrier = "natural_gas"
		default:
			return nil, fmt.Errorf("Pool Heating substituted a reviewed parent site")
		}
		parentName := "Heating:Electricity"
		if carrier == "natural_gas" {
			parentName = "Heating:NaturalGas"
		}
		parent := native.Parents[parentName]
		if !found || seen[id] || site.Facility || site.EndUse != "heating" || site.Carrier != carrier || site.Tabular != nil || site.Source.AllowAbsent || !site.Source.IsMeter || len(site.Source.Keys) != 1 || site.Source.Keys[0] != "" || len(site.Source.Alternatives) != 1 || site.Source.Alternatives[0].Name != parentName || site.Source.Alternatives[0].Unit != "J" {
			return nil, fmt.Errorf("Pool Heating site declaration changed native meter/resource/knownness")
		}
		seen[id] = true
		if service.CarrierReconciliationIDs[carrier] != "reconcile.zone_hvac_allocation.heating."+carrier+".annual" {
			return nil, fmt.Errorf("Pool Heating requires exact carrier ledger IDs from the reviewed recipe")
		}
		if _, annual := frames.SiteAnnual[id]; annual {
			return nil, fmt.Errorf("Pool Heating native Monthly source cannot borrow annual Tabular authority")
		}
		ids := frames.SiteSources[id]
		if frames.SourceZone[parent.Source.DictionaryIndex] != "" {
			return nil, fmt.Errorf("Pool Heating parent meter cannot acquire a Zone reporting owner")
		}
		if len(ids) != 1 || ids[0] != parent.Source.DictionaryIndex || !reflect.DeepEqual(frames.SourceIdentities[ids[0]], parent.Source) || len(frames.Site[id]) != 12 || len(frames.SourceRaw[ids[0]]) != 12 || len(frames.SourceEffective[ids[0]]) != 12 {
			return nil, fmt.Errorf("Pool Heating parent binding differs from native source identity")
		}
		for month, want := range parent.Monthly {
			q := frames.Site[id][month]
			if q == nil || !epathSQLZoneCarrierQuantityEqual(*q, want) || !epathSQLZoneCarrierQuantityEqual(frames.SourceRaw[ids[0]][month], want) || !epathSQLZoneCarrierQuantityEqual(frames.SourceEffective[ids[0]][month], want) {
				return nil, fmt.Errorf("Pool Heating replaced a native parent observation or model-total factor")
			}
		}
		out.Parents[carrier] = parent
	}
	return out, nil
}

func epathSQLPoolHeatingBoundary(native epathSQLPoolSourceFrames, scope, zone, period string) *epathSQLPoolBoundaryProof {
	return &epathSQLPoolBoundaryProof{Native: native, Scope: scope, Zone: zone, Period: period}
}

func epathSQLPoolHeatingRatioCheck(checks *epathSQLModelChecks, boundary *epathSQLPoolBoundaryProof) error {
	if checks == nil || boundary == nil {
		return fmt.Errorf("Pool Heating denial requires an exact check destination")
	}
	target := epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: "heating", FromUnit: "kWh", ToUnit: "kWh", RatioKind: "load_to_site_energy", Aggregate: "sum"}
	// Empty Basis selects every basis. The supplemental native denial rejects
	// all other kinds too; no positive Conversion{0,0} or SQL-missing status.
	if err := checks.add("ratios", boundary.Scope, boundary.Zone, boundary.Period, "heating/non_zone_demand_unquantified", "ratio", nil, target, epathSQLPoolHeatingUnquantified, nil, nil); err != nil {
		return err
	}
	checks.Rows[len(checks.Rows)-1].PoolBoundary = boundary
	return nil
}

func epathSQLPoolHeatingBuildingServiceChecks(frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService, checks *epathSQLModelChecks) error {
	if checks == nil {
		return fmt.Errorf("Pool Heating has no Building check destination")
	}
	proof, err := epathSQLCompilePoolHeatingService(frames, model, service)
	if err != nil {
		return err
	}
	for _, period := range epathSQLZoneCarrierPeriods() {
		boundary := epathSQLPoolHeatingBoundary(proof.Native, "building", "", period)
		if err := epathSQLPoolHeatingRatioCheck(checks, boundary); err != nil {
			return err
		}
		for _, carrier := range []string{"electricity", "natural_gas"} {
			q := epathSQLQuantity{}
			for _, month := range epathSQLPeriodMonths(period) {
				q = q.add(proof.Parents[carrier].Monthly[month-1])
			}
			if !q.valid() || q.Value < 0 {
				return fmt.Errorf("Pool Heating annual native parent sum is invalid")
			}
			id, err := epathSQLAllocationID(service.CarrierReconciliationIDs[carrier], period)
			if err != nil {
				return err
			}
			start := len(checks.Rows)
			for _, field := range []string{"expectedValue", "directValue", "allocatedValue", "unassignedValue"} {
				value := epathSQLQuantity{}
				if field == "expectedValue" || field == "unassignedValue" {
					value = q
				}
				target := epathRealOracleTarget{Collection: "reconciliation", ID: id, Level: "allocation", Field: field, Unit: "kWh", Service: "heating"}
				if err := checks.add("zoneAllocation", "building", "", period, "heating/"+carrier+"/"+field, "kWh", &value, target, "", nil, nil); err != nil {
					return err
				}
				checks.Rows[len(checks.Rows)-1].PoolBoundary = boundary
			}
			if err := checks.bindAllocationProof(start); err != nil {
				return err
			}
		}
	}
	return nil
}

func epathSQLPoolHeatingZoneServiceChecks(frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService, checks *epathSQLModelChecks) error {
	if checks == nil {
		return fmt.Errorf("Pool Heating has no Zone check destination")
	}
	compiled, err := epathSQLCompilePoolHeatingService(frames, model, service)
	if err != nil {
		return err
	}
	for _, zone := range compiled.Zones {
		for _, period := range epathSQLZoneCarrierPeriods() {
			boundary := epathSQLPoolHeatingBoundary(compiled.Native, "zone", zone, period)
			proof := &epathSQLZoneServiceProof{NativePool: boundary, Service: "heating", Basis: service.Basis, Carriers: map[string]epathSQLQuantity{"electricity": {}, "natural_gas": {}}, CarrierSources: map[string]map[string]epathRealSQLSource{}, RequiredSites: map[string]map[string]bool{}, LoadSources: map[string]epathRealSQLSource{}}
			for carrier, parent := range compiled.Parents {
				proof.CarrierSources[carrier] = map[string]epathRealSQLSource{fmt.Sprintf("sql-rdd-%d", parent.Source.DictionaryIndex): parent.Source}
				proof.RequiredSites[carrier] = map[string]bool{}
			}
			for _, field := range []string{"value", "allocatedValue"} {
				zero := epathSQLQuantity{}
				target := epathSQLNodeTarget("end_use", "heating", "", "site")
				target.Field, target.AllowPrunedZero = field, true
				target.Basis = service.Basis
				// Keep the established carrier-input target. The attached typed
				// consumer independently checks every actual Heating basis, so a
				// direct/fallback node cannot evade this scalar's semantic contract.
				if err := checks.add("zoneAllocation", "zone", zone, period, "hvac_zone/heating/"+field, "kWh", &zero, target, "", nil, nil); err != nil {
					return err
				}
				checks.Rows[len(checks.Rows)-1].PoolBoundary = boundary
				if field == "value" {
					checks.Rows[len(checks.Rows)-1].ZoneService = proof
				}
			}
			if err := epathSQLPoolHeatingRatioCheck(checks, boundary); err != nil {
				return err
			}
		}
	}
	return nil
}

// Called before generic ZoneService validation. A semantically unquantified
// shared HW source gives an observed zero ALLOCATION, not zero native input or
// unavailable SQL. Keep both carriers so total Zone carrier proof stays closed.
func epathSQLPoolHeatingQuantityZero(q epathSQLQuantity) bool {
	low, high := q.bounds()
	return q.valid() && q.Value == 0 && low == 0 && high == 0
}

func epathCheckSQLPoolHeatingZoneService(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	proof := check.ZoneService
	if proof == nil || proof.NativePool == nil || proof.NativePool.Scope != "zone" || proof.NativePool.Zone != check.Item.Zone || proof.NativePool.Period != check.Item.Period || check.Item.Scope != "zone" || proof.Service != "heating" || proof.Basis != "service_path_allocation" || proof.Unavailable || proof.Unowned || proof.AnnualTabular || proof.DirectHVAC || proof.NativeVRF != nil || proof.HVACConsumption != nil || check.Quantity == nil || !epathSQLPoolHeatingQuantityZero(*check.Quantity) || len(proof.Carriers) != 2 || len(proof.CarrierSources) != 2 || len(proof.RequiredSites) != 2 || check.Item.Target.Collection != "nodes" || check.Item.Target.Field != "value" || check.Item.Target.Level != "end_use" || check.Item.Target.Category != "heating" || check.Item.Target.Basis != proof.Basis || !check.Item.Target.AllowPrunedZero {
		return fmt.Errorf("Pool Heating Zone consumer lost its typed observed-zero allocation proof")
	}
	if check.Item.Group != "zoneAllocation" || check.Item.Unit != "kWh" || check.Item.Target.Unit != "kWh" || check.Item.Target.ScaleDomain != "site" || check.Want.Status != "" || check.PoolBoundary != proof.NativePool {
		return fmt.Errorf("Pool Heating Zone target changed its units, domain, status or native proof binding")
	}
	for _, carrier := range []string{"electricity", "natural_gas"} {
		q, found := proof.Carriers[carrier]
		if !found || !epathSQLPoolHeatingQuantityZero(q) {
			return fmt.Errorf("Pool Heating Zone carrier is not an exact zero allocation")
		}
		parentName := "Heating:Electricity"
		if carrier == "natural_gas" {
			parentName = "Heating:NaturalGas"
		}
		parent := proof.NativePool.Native.Parents[parentName]
		want := map[string]epathRealSQLSource{fmt.Sprintf("sql-rdd-%d", parent.Source.DictionaryIndex): parent.Source}
		if !reflect.DeepEqual(proof.CarrierSources[carrier], want) || len(proof.RequiredSites[carrier]) != 0 {
			return fmt.Errorf("Pool Heating Zone zero borrowed or required a false native consumer source")
		}
	}
	if err := epathSQLCheckPoolBoundary(bundle, proof.NativePool); err != nil {
		return err
	}
	nodes, links, _, _, err := epathOracleGraph(bundle, "zone", check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	if err := epathSQLPoolHeatingZeroZoneGraph(nodes, links); err != nil {
		return err
	}
	for _, zone := range bundle.EnergyExplanation.ZoneResults {
		if !strings.EqualFold(zone.Scope.ZoneName, check.Item.Zone) {
			continue
		}
		for _, period := range zone.Periods {
			if period.ID == check.Item.Period {
				if err := epathSQLPoolHeatingZeroZoneGraph(period.Nodes, period.Links); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func epathSQLPoolHeatingZeroZoneGraph(nodes []EnergyExplanationNode, links []EnergyPathLink) error {
	heating := map[string]bool{}
	for _, node := range nodes {
		if node.Level != "end_use" || !epathSQLPoolBoundaryHeatingNode(node) {
			continue
		}
		heating[node.ID] = true
		for _, value := range []float64{node.Value, node.RawValue, node.EffectiveValue, node.AllocatedValue, node.DisplayValue, node.SignedValue} {
			if !epathOracleFinite(value) || value != 0 {
				return fmt.Errorf("Pool Heating invented Zone native or allocated consumption")
			}
		}
	}
	for _, link := range links {
		if !heating[link.FromID] && !heating[link.ToID] {
			continue
		}
		if epathSQLPoolBoundaryKey(link.Relation) == "end_use_to_carrier" || epathSQLPoolBoundaryKey(link.Relation) == "direct_end_use_to_carrier" {
			for _, value := range []float64{link.FromValue, link.ToValue, link.Ratio} {
				if !epathOracleFinite(value) || value != 0 {
					return fmt.Errorf("Pool Heating invented a positive Zone carrier branch")
				}
			}
		}
	}
	return nil
}
