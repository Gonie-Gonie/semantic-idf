package simulation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// The selected thermal quantity is already model-total fluid/surface heat.
// Its independent sources do not measure a sensible/latent decomposition.
type epathSQLRadiantLoadNodeProof struct {
	Zone, Service string
	Sources       map[int]epathSQLRadiantLoadSourceIdentity
}

func epathSQLRadiantSameOriginalSource(a, b epathRealSQLSource) bool {
	return reflect.DeepEqual(a, b)
}

func epathSQLNativeRadiantService(model epathRealSQLModel, service string) bool {
	for _, load := range model.Loads {
		if load.Service == service && load.NativeRadiant != nil {
			return true
		}
	}
	return false
}

func epathSQLRadiantFrameLoadSource(frames epathSQLFrames, index int, zone, service string, month int) (epathSQLRadiantLoadSourceIdentity, error) {
	identity, ok := frames.RadiantLoadSourceIdentities[index]
	original, exists := frames.SourceIdentities[index]
	quantity, observed := frames.Loads[epathSQLKey(zone, service, month)]
	if !ok || !exists || !observed || index <= 0 || month < 1 || month > 12 || identity.Source.DictionaryIndex != index ||
		!reflect.DeepEqual(original, identity.Source) || identity.Service != service || !strings.EqualFold(identity.Owner.Owner.ZoneName, zone) ||
		epathSQLValidateRadiantLoadSourceIdentity(identity) != nil || !reflect.DeepEqual(quantity, identity.Effective[month-1]) {
		return epathSQLRadiantLoadSourceIdentity{}, fmt.Errorf("radiant frame load lost exact original source/owner/service/month binding")
	}
	return identity, nil
}

func epathSQLRadiantLoadNodeProofFor(frames epathSQLFrames, zone, service string) (*epathSQLRadiantLoadNodeProof, error) {
	proof := &epathSQLRadiantLoadNodeProof{Zone: zone, Service: service, Sources: map[int]epathSQLRadiantLoadSourceIdentity{}}
	for key := range frames.Zones {
		if zone != "" && !strings.EqualFold(key, zone) {
			continue
		}
		for month := 1; month <= 12; month++ {
			ids := frames.LoadSourceIDs[epathSQLKey(key, service, month)]
			if len(ids) != 1 {
				return nil, fmt.Errorf("native radiant load needs one exact original equipment observation per Zone/month")
			}
			identity, err := epathSQLRadiantFrameLoadSource(frames, ids[0], key, service, month)
			if err != nil {
				return nil, err
			}
			proof.Sources[ids[0]] = identity
		}
	}
	if len(proof.Sources) == 0 {
		return nil, fmt.Errorf("native radiant load has no independently observed owner")
	}
	return proof, nil
}

func epathCheckSQLRadiantLoadNode(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	proof, target := check.NativeRadiantLoad, check.Item.Target
	if proof == nil || len(proof.Sources) == 0 || check.Quantity == nil || !check.Quantity.valid() ||
		check.Item.Group != "loads" || target.Collection != "nodes" || target.Field != "value" || target.Level != "load" || target.Service != proof.Service ||
		target.Basis != "reported_variable" || target.Unit != "kWh" || target.ScaleDomain != "thermal" || target.AggregationBasis != "model_total" ||
		check.Item.Scope == "building" && (check.Item.Zone != "" || proof.Zone != "") ||
		check.Item.Scope == "zone" && (proof.Zone == "" || !strings.EqualFold(check.Item.Zone, proof.Zone)) ||
		check.Item.Scope != "building" && check.Item.Scope != "zone" || !epathOracleValidPeriod(check.Item.Period) {
		return fmt.Errorf("native radiant node requires its exact scope/service/quantity target")
	}
	allowed, required := map[string]epathSQLOriginalSource{}, map[string]bool{}
	quantity, owners := epathSQLQuantity{}, map[string]bool{}
	indexes := []int{}
	for index := range proof.Sources {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		identity := proof.Sources[index]
		owner := strings.ToLower(identity.Owner.Owner.ZoneName)
		if index != identity.Source.DictionaryIndex || identity.Service != proof.Service || owners[owner] || proof.Zone != "" && !strings.EqualFold(proof.Zone, owner) {
			return fmt.Errorf("native radiant node proof contains a foreign/duplicate owner or service")
		}
		original, err := epathSQLOriginalRadiant(identity)
		if err != nil {
			return err
		}
		owners[owner] = true
		id := fmt.Sprintf("sql-rdd-%d", index)
		allowed[id] = original
		contribution := epathSQLQuantity{}
		for _, month := range epathSQLPeriodMonths(check.Item.Period) {
			contribution = contribution.add(identity.Effective[month-1])
		}
		required[id] = !contribution.includesZero()
		quantity = quantity.add(contribution)
	}
	// Only summation order may differ from the frame compiler. This is a
	// source-to-expectation binding, not an additional candidate tolerance.
	qLow, qHigh := quantity.bounds()
	cLow, cHigh := check.Quantity.bounds()
	if !epathSQLVRFNativeClose(quantity.Value, check.Quantity.Value) || !epathSQLVRFNativeClose(max(0, qLow), max(0, cLow)) || !epathSQLVRFNativeClose(qHigh, cHigh) {
		return fmt.Errorf("native radiant node quantity differs from its original monthly source sum")
	}
	nodes, _, _, _, err := epathOracleGraph(bundle, check.Item.Scope, check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	actual := map[string]EnergyDataSource{}
	for _, source := range bundle.EnergyExplanation.Sources {
		if _, exists := actual[source.ID]; source.ID == "" || exists {
			return fmt.Errorf("ambiguous source identity in native radiant graph")
		}
		actual[source.ID] = source
	}
	count := 0
	for _, node := range nodes {
		if node.Level != "load" || node.ServiceKind != proof.Service {
			continue
		}
		count++
		if count != 1 || node.ThermalBoundary != "active_surface_source" || node.ThermalComponent != "combined" || len(node.LoadBreakdown) != 0 ||
			node.Unit != "kWh" || node.ScaleDomain != "thermal" || node.Basis != "reported_variable" || node.AggregationBasis != "model_total" {
			return fmt.Errorf("native radiant load is not one combined active-surface boundary with unknown sensible/latent decomposition")
		}
		if err := epathSQLVerifyOriginalSources(node.SourceIDs, actual, allowed, required, check.Item.Period); err != nil {
			return err
		}
	}
	if count == 0 && !quantity.includesZero() {
		return fmt.Errorf("positive original radiant load lost its canonical node")
	}
	return nil
}
