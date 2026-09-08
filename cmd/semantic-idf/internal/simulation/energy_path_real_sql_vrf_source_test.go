package simulation

import (
	"fmt"
	"reflect"
	"strings"
)

type epathSQLVRFSourceIdentity struct {
	System      epathRealSQLVRFSystem
	Observation epathSQLVRFSourceFrame
	Precision   epathRealSQLPrecision
	Allocation  *epathSQLVRFSourceAllocation // Independent annual month-first proof; scalar checks only.
}

func epathSQLValidateVRFSourceIdentity(identity epathSQLVRFSourceIdentity) error {
	owners, err := epathSQLVRFDeclaration(identity.System)
	if err != nil {
		return err
	}
	observation := identity.Observation
	matched := false
	for _, role := range epathSQLVRFRoles(identity.System) {
		if role.ID != observation.Role {
			continue
		}
		key := epathSQLVRFName(observation.Source.KeyValue)
		zone := owners[key]
		if role.Shared {
			zone = ""
		}
		matched = role.Shared == observation.Shared && role.Service == observation.Service && strings.EqualFold(role.Name, observation.Source.Name) &&
			strings.EqualFold(zone, observation.ZoneName) && (role.Shared && key == epathSQLVRFName(identity.System.OutdoorUnit.ObjectName) || !role.Shared && zone != "")
	}
	if !matched || observation.EquipmentObjectIndex < 0 || observation.RequestObjectIndex != nil && *observation.RequestObjectIndex < 0 {
		return fmt.Errorf("VRF source identity has no exact reviewed role/physical owner")
	}
	months, err := epathSQLVRFSourceMonths(observation.Source, identity.Precision)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(months, observation.Months) {
		return fmt.Errorf("VRF identity changed original monthly quantity authority")
	}
	return nil
}

func epathSQLVRFSourceAnnual(identity epathSQLVRFSourceIdentity) (epathSQLQuantity, error) {
	if err := epathSQLValidateVRFSourceIdentity(identity); err != nil {
		return epathSQLQuantity{}, err
	}
	// A source inspector scalar is the original annual observation, rounded
	// once at presentation. It is not the sum of rounded monthly node budgets.
	q := epathSQLQuantity{Value: *identity.Observation.Source.EnergyKWh}
	if q.Value != 0 {
		q.Error = .0005
	}
	return q, nil
}

func epathSQLModelVRFSourceChecks(frames epathSQLFrames, checks *epathSQLModelChecks) error {
	if len(frames.NativeVRFSystems) == 0 {
		return nil
	}
	if len(frames.NativeVRFSystems) != len(frames.NativeVRFAllocations) {
		return fmt.Errorf("VRF source checks lack independent allocation frames")
	}
	for index, system := range frames.NativeVRFSystems {
		if err := epathSQLValidateVRFSystemFrame(system); err != nil {
			return err
		}
		allocation := frames.NativeVRFAllocations[index]
		if !reflect.DeepEqual(allocation.System, system) {
			return fmt.Errorf("VRF allocation proof belongs to a different original system")
		}
		for _, observation := range system.Sources {
			identity := epathSQLVRFSourceIdentity{System: system.Declaration, Observation: observation, Precision: system.Precision}
			allocated, exists := allocation.Annual.Sources[observation.Source.DictionaryIndex]
			if !exists {
				return fmt.Errorf("VRF original source has no independent annual allocation proof")
			}
			identity.Allocation = &allocated
			annual, err := epathSQLVRFSourceAnnual(identity)
			if err != nil {
				return err
			}
			zones := []string{""}
			if observation.Shared {
				for _, terminal := range system.Declaration.Terminals {
					zones = append(zones, terminal.ZoneName)
				}
			} else {
				zones = append(zones, observation.ZoneName)
			}
			for _, zone := range zones {
				scope := "building"
				if zone != "" {
					scope = "zone"
				}
				for _, field := range []string{"rawValue", "effectiveValue", "allocatedValue"} {
					q := annual
					var expected *epathSQLQuantity = &q
					if observation.Shared && scope == "zone" {
						if field != "allocatedValue" {
							expected = nil
						} else {
							row, exists := allocation.Annual.Sources[observation.Source.DictionaryIndex]
							share, present := row.Shares[zone]
							if !exists || !present || !row.Shared || row.Role != observation.Role || row.SourceID != observation.Source.DictionaryIndex {
								return fmt.Errorf("VRF shared source allocation lacks exact original source/recipient")
							}
							q = epathSQLQuantity{Value: float64(share.DisplayMilliKWh) / 1000}
						}
					}
					target := epathRealOracleTarget{Collection: "sources", Field: field, SourceName: observation.Source.Name, SourceKey: observation.Source.KeyValue, SourceUnit: "J", Frequency: "Monthly", Unit: "kWh"}
					key := "native_vrf_source/" + observation.Role + "/" + observation.Source.KeyValue + "/" + field
					if err := checks.add("endUses", scope, zone, "annual", key, "kWh", expected, target, "", nil, nil); err != nil {
						return err
					}
					proof := identity
					checks.Rows[len(checks.Rows)-1].NativeVRFSource = &proof
				}
			}
		}
	}
	return nil
}

func epathSQLVRFSourceCheckExpected(identity epathSQLVRFSourceIdentity, check epathSQLModelCheck) (*epathSQLQuantity, error) {
	annual, err := epathSQLVRFSourceAnnual(identity)
	if err != nil {
		return nil, err
	}
	item, target := check.Item, check.Item.Target
	observation := identity.Observation
	if item.Group != "endUses" || item.Period != "annual" || item.Unit != "kWh" || target.Collection != "sources" || target.SourceName != observation.Source.Name || target.SourceKey != observation.Source.KeyValue || target.SourceUnit != "J" || target.Frequency != "Monthly" || target.Unit != "kWh" ||
		(target.Field != "rawValue" && target.Field != "effectiveValue" && target.Field != "allocatedValue") || item.Scope != "building" && item.Scope != "zone" || item.Scope == "building" && item.Zone != "" {
		return nil, fmt.Errorf("VRF source proof escaped exact original quantity/temporal/target binding")
	}
	if item.Scope == "zone" {
		owner := false
		for _, terminal := range identity.System.Terminals {
			owner = owner || strings.EqualFold(terminal.ZoneName, item.Zone)
		}
		if !owner || !observation.Shared && !strings.EqualFold(item.Zone, observation.ZoneName) {
			return nil, fmt.Errorf("VRF source proof escaped original owner roster")
		}
		if observation.Shared {
			if target.Field != "allocatedValue" {
				if check.Quantity != nil {
					return nil, fmt.Errorf("individual-Zone outdoor observation is unknown, not measured zero")
				}
				return nil, nil
			}
			allocation := identity.Allocation
			if allocation == nil || allocation.SourceID != observation.Source.DictionaryIndex || !allocation.Shared || allocation.Role != observation.Role || allocation.Service != observation.Service {
				return nil, fmt.Errorf("VRF allocated source lacks independently bound annual source proof")
			}
			share, exists := allocation.Shares[item.Zone]
			if !exists || share.DisplayMilliKWh < 0 {
				return nil, fmt.Errorf("VRF allocated source has no exact original recipient")
			}
			q := epathSQLQuantity{Value: float64(share.DisplayMilliKWh) / 1000}
			if check.Quantity == nil || !reflect.DeepEqual(*check.Quantity, q) {
				return nil, fmt.Errorf("VRF allocated source changed its exact integer-display quantity")
			}
			return &q, nil
		}
	}
	if check.Quantity == nil || !reflect.DeepEqual(*check.Quantity, annual) {
		return nil, fmt.Errorf("VRF source proof changed original annual authority")
	}
	return &annual, nil
}

// allocatedValue is not a generic source-oracle field: its knownness depends
// on the native VRF local/shared policy and the independent integer allocation.
// Validate that complete typed contract before reusing the generic source
// selector's property/identity restrictions. This copy changes no observation,
// expectation, required metric key, or source field used by the scalar reader.
func epathSQLVRFSourceTarget(check epathSQLModelCheck) error {
	if check.NativeVRFSource == nil {
		return fmt.Errorf("native VRF source target requires original typed evidence")
	}
	if _, err := epathSQLVRFSourceCheckExpected(*check.NativeVRFSource, check); err != nil {
		return err
	}
	target := check.Item.Target
	if target.Field == "allocatedValue" {
		target.Field = "effectiveValue"
	}
	return epathValidateOracleTarget(target, check.Item.Unit)
}

// Scalar presence is read explicitly. An omitted shared raw/effective field is
// unavailable; an explicit 0 in that field would incorrectly claim measurement.
func epathSQLVRFSourceScalar(source EnergyDataSource, item epathRealOracleMetricRecipe) (*float64, error) {
	field := item.Target.Field
	value, bit := source.RawValue, uint8(1)
	if field == "effectiveValue" {
		value, bit = source.EffectiveValue, 2
	} else if field == "allocatedValue" {
		value, bit = source.AllocatedValue, 0
	}
	decoded, presence := source.inspectorDecodedFromJSON, source.inspectorValuePresence
	if item.Scope == "zone" {
		count := 0
		for _, detail := range source.ScopeDetails {
			if !strings.EqualFold(detail.Scope.ZoneName, item.Zone) {
				continue
			}
			count++
			if detail.Scope.Kind != "zone" || detail.Scope.AggregationBasis != "model_total" || detail.AggregationBasis != "model_total" {
				return nil, fmt.Errorf("VRF source has wrong Zone scope basis")
			}
			value = detail.RawValue
			if field == "effectiveValue" {
				value = detail.EffectiveValue
			} else if field == "allocatedValue" {
				value = detail.AllocatedValue
			}
			decoded, presence = detail.inspectorDecodedFromJSON, detail.inspectorValuePresence
		}
		if count != 1 {
			return nil, fmt.Errorf("VRF source requires exactly one original owner scope detail")
		}
	}
	if decoded && bit != 0 && presence&bit == 0 {
		return nil, nil
	}
	if !decoded {
		return nil, fmt.Errorf("VRF source scalar requires original-wire presence proof")
	}
	// allocatedValue is omitempty and has no raw/effective presence bit. Its
	// knownness is proved by the explicit observed/allocation policy below;
	// the original-wire preflight separately rejects an explicit JSON null.
	if !epathOracleFinite(value) {
		return nil, fmt.Errorf("VRF source scalar is nonfinite")
	}
	return epathOracleNumber(value), nil
}

// A parent source may expose only its original physical owner (local) or the
// complete reviewed outdoor recipient roster (shared). Checking just the
// currently selected detail would leave foreign/duplicate details unaudited.
func epathSQLVRFSourceScopeRoster(source EnergyDataSource, identity epathSQLVRFSourceIdentity) error {
	owners := map[string]bool{}
	if identity.Observation.Shared {
		for _, terminal := range identity.System.Terminals {
			owners[epathSQLVRFName(terminal.ZoneName)] = true
		}
	} else {
		owners[epathSQLVRFName(identity.Observation.ZoneName)] = true
	}
	seen := map[string]bool{}
	for _, detail := range source.ScopeDetails {
		key := epathSQLVRFName(detail.Scope.ZoneName)
		if !owners[key] || seen[key] || detail.Scope.Kind != "zone" || detail.Scope.AggregationBasis != "model_total" || detail.AggregationBasis != "model_total" {
			return fmt.Errorf("VRF parent source has a foreign, duplicate, or malformed Zone detail")
		}
		seen[key] = true
	}
	if len(seen) != len(owners) {
		return fmt.Errorf("VRF parent source lost an original owner detail")
	}
	return nil
}

func epathCheckSQLVRFSource(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	if check.NativeVRFSource == nil {
		return fmt.Errorf("missing independently observed VRF source proof")
	}
	identity := *check.NativeVRFSource
	expected, err := epathSQLVRFSourceCheckExpected(identity, check)
	if err != nil {
		return err
	}
	count := 0
	var matched EnergyDataSource
	for _, source := range bundle.EnergyExplanation.Sources {
		if source.ID != fmt.Sprintf("sql-rdd-%d", identity.Observation.Source.DictionaryIndex) && !(strings.EqualFold(source.Name, identity.Observation.Source.Name) && strings.EqualFold(source.KeyValue, identity.Observation.Source.KeyValue) && source.ReportingFrequency == "Monthly") {
			continue
		}
		count++
		if err := epathSQLMatchVRFSource(source, identity.Observation); err != nil {
			return err
		}
		matched = source
	}
	if count != 1 {
		return fmt.Errorf("VRF scalar requires exactly one original source identity")
	}
	if err := epathSQLVRFSourceScopeRoster(matched, identity); err != nil {
		return err
	}
	if matched.AllocationApplied || !matched.inspectorDecodedFromJSON || matched.inspectorValuePresence&3 != 3 {
		return fmt.Errorf("Building source must retain explicit original raw/effective observations")
	}
	if check.Item.Scope == "zone" {
		for _, detail := range matched.ScopeDetails {
			if !strings.EqualFold(detail.Scope.ZoneName, check.Item.Zone) {
				continue
			}
			if identity.Observation.Shared {
				if !detail.AllocationApplied || detail.EffectiveMultiplier != 0 || detail.MultiplierApplication != "" || !detail.inspectorDecodedFromJSON || detail.inspectorValuePresence&3 != 0 {
					return fmt.Errorf("VRF outdoor Zone scope falsely claims a local measurement")
				}
			} else if detail.AllocationApplied || detail.EffectiveMultiplier != 1 || detail.MultiplierApplication != "already_model_total" || !detail.inspectorDecodedFromJSON || detail.inspectorValuePresence&3 != 3 {
				return fmt.Errorf("VRF local source lost observed model-total ownership")
			}
		}
	}
	actual, err := epathSQLVRFSourceScalar(matched, check.Item)
	if err != nil {
		return err
	}
	if expected != nil && actual != nil && (expected.Value == 0 || identity.Observation.Shared && check.Item.Scope == "zone" && check.Item.Target.Field == "allocatedValue") && *actual != expected.Value {
		return fmt.Errorf("VRF observed zero or integer allocation changed its exact scalar")
	}
	return epathCheckSQLModelQuantity(actual, expected)
}
