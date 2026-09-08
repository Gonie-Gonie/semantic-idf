package simulation

import (
	"fmt"
	"reflect"
	"strings"
)

// The native proof roster must survive every frame-to-check handoff. A missing
// raw map entry must not silently remove an original source's obligations.
func epathSQLValidateRadiantSourceFrameRegistry(frames epathSQLFrames, observed []epathRealSQLSource, model epathRealSQLModel) error {
	for id, identity := range frames.RadiantLoadSourceIdentities {
		if err := epathSQLValidateRadiantLoadSourceIdentity(identity); err != nil {
			return err
		}
		matches := 0
		for _, source := range observed {
			if source.DictionaryIndex == id {
				matches++
				if !reflect.DeepEqual(source, identity.Source) {
					return fmt.Errorf("radiant frame registry changed its original observed source")
				}
			}
		}
		zoneKey := frames.SourceZone[id]
		if matches != 1 || identity.Source.DictionaryIndex != id || !reflect.DeepEqual(frames.SourceIdentities[id], identity.Source) ||
			!reflect.DeepEqual(frames.SourceRaw[id], identity.Raw[:]) || !reflect.DeepEqual(frames.SourceEffective[id], identity.Effective[:]) ||
			!strings.EqualFold(zoneKey, identity.Owner.Owner.ZoneName) || !strings.EqualFold(frames.Zones[zoneKey].Name, identity.Owner.Owner.ZoneName) {
			return fmt.Errorf("radiant frame registry lost an exact original source, quantity or Zone owner")
		}
	}
	for _, load := range model.Loads {
		if load.NativeRadiant == nil {
			continue
		}
		sources, err := epathSQLSelect(observed, load.Source)
		if err != nil {
			return err
		}
		for _, source := range sources {
			identity, ok := frames.RadiantLoadSourceIdentities[source.DictionaryIndex]
			if !ok || identity.Service != load.Service || !reflect.DeepEqual(identity.Source, source) {
				return fmt.Errorf("declared native radiant load lost its original typed frame proof")
			}
		}
	}
	return nil
}

func epathSQLValidateRadiantLoadSourceIdentity(identity epathSQLRadiantLoadSourceIdentity) error {
	owner := identity.Owner
	actual, err := epathSQLRadiantLoadObservation(identity.Source, identity.Service, owner,
		epathSQLZone{Name: owner.Owner.ZoneName, Multiplier: owner.ZoneMultiplier * owner.ZoneListMultiplier}, identity.Precision)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, identity) {
		return fmt.Errorf("radiant identity changed original quantities or explicit model-total policy")
	}
	return nil
}

func epathSQLMatchRadiantLoadSource(source EnergyDataSource, identity epathSQLRadiantLoadSourceIdentity) error {
	if err := epathSQLValidateRadiantLoadSourceIdentity(identity); err != nil {
		return err
	}
	original, owner := identity.Source, identity.Owner.Owner
	if source.ID != fmt.Sprintf("sql-rdd-%d", original.DictionaryIndex) || source.SourceType != "sql_report_data" || source.IsMeter ||
		!strings.EqualFold(source.Name, original.Name) || !strings.EqualFold(source.KeyValue, owner.EquipmentName) || !strings.EqualFold(source.ZoneName, owner.ZoneName) ||
		source.Units != "J" || source.SourceUnit != "J" || source.NormalizedUnit != "kWh" || source.ReportingFrequency != "Monthly" || source.AggregationMethod != "sum_report_data" ||
		source.AggregationBasis != "model_total" || source.EffectiveMultiplier != 1 || source.MultiplierApplication != "already_model_total" || len(source.InputSourceIDs) != 0 {
		return fmt.Errorf("radiant source lost exact original equipment/Zone/model-total provenance")
	}
	seen := false
	for _, detail := range source.ScopeDetails {
		if seen || detail.Scope.Kind != "zone" || !strings.EqualFold(detail.Scope.ZoneName, owner.ZoneName) || detail.AggregationBasis != "model_total" || detail.EffectiveMultiplier != 1 || detail.MultiplierApplication != "already_model_total" {
			return fmt.Errorf("radiant source has foreign, duplicate or rescaled original owner details")
		}
		seen = true
	}
	return nil
}

func epathCheckSQLRadiantLoadSource(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	identity := check.NativeRadiantSource
	if identity == nil || epathSQLValidateRadiantLoadSourceIdentity(*identity) != nil {
		return fmt.Errorf("radiant scalar requires independently validated original identity")
	}
	item, target := check.Item, check.Item.Target
	if item.Group != "loads" || item.Period != "annual" || item.Unit != "kWh" || target.Collection != "sources" || target.SourceName != identity.Source.Name || target.SourceKey != identity.Source.KeyValue || target.SourceUnit != "J" || target.Frequency != "Monthly" || target.Unit != "kWh" ||
		(target.Field != "rawValue" && target.Field != "effectiveValue") || item.Scope != "building" && item.Scope != "zone" || item.Scope == "building" && item.Zone != "" || item.Scope == "zone" && !strings.EqualFold(item.Zone, identity.Owner.Owner.ZoneName) {
		return fmt.Errorf("radiant scalar escaped its exact original owner/field/annual target")
	}
	q := epathSQLQuantity{}
	values := identity.Raw
	if target.Field == "effectiveValue" {
		values = identity.Effective
	}
	for _, value := range values {
		q = q.add(value)
	}
	if check.Quantity == nil || !reflect.DeepEqual(*check.Quantity, q) {
		return fmt.Errorf("radiant scalar changed the independently propagated original monthly quantity")
	}
	count := 0
	for _, source := range bundle.EnergyExplanation.Sources {
		if source.ID != fmt.Sprintf("sql-rdd-%d", identity.Source.DictionaryIndex) && !(strings.EqualFold(source.Name, identity.Source.Name) && strings.EqualFold(source.KeyValue, identity.Source.KeyValue) && source.ReportingFrequency == "Monthly") {
			continue
		}
		count++
		if err := epathSQLMatchRadiantLoadSource(source, *identity); err != nil {
			return err
		}
	}
	if count != 1 {
		return fmt.Errorf("radiant scalar requires one exact original source")
	}
	actual, err := epathReadOracleSourceCandidate(bundle.EnergyExplanation.Sources, item)
	if err != nil {
		return err
	}
	return epathCheckSQLModelQuantity(actual, &q)
}
