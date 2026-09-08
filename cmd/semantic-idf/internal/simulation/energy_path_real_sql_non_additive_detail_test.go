package simulation

import (
	"fmt"
	"strings"
)

// Source is an independently observed SQL identity, not a candidate source.
// The exact owner-to-Zone association is reviewed against the original IDF
// equipment connections/list and the executed plan's ScopeZoneName.
type epathSQLLoadDetailIdentity struct {
	ZoneName, Service, OwnerName string
	Source                       epathRealSQLSource
}

func epathSQLValidateLoadDetailIdentity(detail epathSQLLoadDetailIdentity) error {
	if strings.TrimSpace(detail.ZoneName) == "" || strings.TrimSpace(detail.OwnerName) == "" || (detail.Service != "cooling" && detail.Service != "heating") || detail.Source.DictionaryIndex <= 0 || detail.Source.IsMeter || !strings.EqualFold(detail.Source.KeyValue, detail.OwnerName) || detail.Source.ReportingFrequency != "Monthly" {
		return fmt.Errorf("invalid non-additive load detail owner/service/source identity")
	}
	serviceName := "Cooling"
	if detail.Service == "heating" {
		serviceName = "Heating"
	}
	name := "Zone Ideal Loads Supply Air Latent " + serviceName
	if !(detail.Source.Name == name+" Energy" && detail.Source.SourceUnit == "J" || detail.Source.Name == name+" Rate" && detail.Source.SourceUnit == "W") {
		return fmt.Errorf("non-additive load detail requires its exact reviewed latent service name and unit")
	}
	return nil
}

func epathCompileSQLNonAdditiveLoadDetails(frames *epathSQLFrames, observed []epathRealSQLSource, model epathRealSQLModel) error {
	if len(model.NonAdditiveLoadDetails) == 0 {
		return nil
	}
	if frames == nil {
		return fmt.Errorf("non-additive load details require independently compiled frames")
	}
	identities := map[int]epathSQLLoadDetailIdentity{}
	monthsByID := map[int][]epathSQLQuantity{}
	ownerZones := map[string]string{}
	for _, declaration := range model.NonAdditiveLoadDetails {
		zoneKey := strings.ToLower(declaration.ZoneName)
		zone, ok := frames.Zones[zoneKey]
		if !ok || zone.Name == "" || len(declaration.Source.Keys) != 1 || declaration.Source.Keys[0] == "*" || !strings.EqualFold(declaration.Source.Keys[0], declaration.OwnerName) || len(declaration.Source.Alternatives) != 1 || declaration.Source.AllowAbsent || declaration.Source.IsMeter {
			return fmt.Errorf("non-additive load detail requires one exact known Zone and equipment source")
		}
		ownerKey := strings.ToLower(declaration.OwnerName)
		if previous, exists := ownerZones[ownerKey]; exists && previous != zoneKey {
			return fmt.Errorf("non-additive load detail equipment has contradictory Zone ownership")
		}
		ownerZones[ownerKey] = zoneKey
		sources, err := epathSQLSelect(observed, declaration.Source)
		if err != nil || len(sources) != 1 {
			return fmt.Errorf("non-additive load detail requires one actual SQL observation: %v", err)
		}
		source := sources[0]
		detail := epathSQLLoadDetailIdentity{ZoneName: zone.Name, Service: declaration.Service, OwnerName: declaration.OwnerName, Source: source}
		if err := epathSQLValidateLoadDetailIdentity(detail); err != nil {
			return err
		}
		if _, duplicate := identities[source.DictionaryIndex]; duplicate {
			return fmt.Errorf("duplicate non-additive load detail source")
		}
		if _, additive := frames.SourceIdentities[source.DictionaryIndex]; additive {
			return fmt.Errorf("non-additive load detail is already an additive/source authority")
		}
		months, err := epathSQLMonthly(source, model.Precision)
		if err != nil {
			return err
		}
		for index, month := range months {
			load, exists := frames.Loads[epathSQLKey(zoneKey, detail.Service, index+1)]
			if !exists || !load.valid() || !month.valid() || month.Value < 0 {
				return fmt.Errorf("non-additive detail lacks known selected Zone/service load or valid raw energy")
			}
		}
		identities[source.DictionaryIndex], monthsByID[source.DictionaryIndex] = detail, months
	}
	if frames.LoadDetailIdentities == nil {
		frames.LoadDetailIdentities = map[int]epathSQLLoadDetailIdentity{}
	}
	if frames.LoadDetailSourceIDs == nil {
		frames.LoadDetailSourceIDs = map[string][]int{}
	}
	if frames.SourceRaw == nil {
		frames.SourceRaw = map[int][]epathSQLQuantity{}
	}
	if frames.SourceEffective == nil {
		frames.SourceEffective = map[int][]epathSQLQuantity{}
	}
	if frames.SourceZone == nil {
		frames.SourceZone = map[int]string{}
	}
	if frames.SourceIdentities == nil {
		frames.SourceIdentities = map[int]epathRealSQLSource{}
	}
	for id, detail := range identities {
		zoneKey := strings.ToLower(detail.ZoneName)
		frames.LoadDetailIdentities[id] = detail
		frames.SourceIdentities[id] = detail.Source
		frames.SourceZone[id] = zoneKey
		frames.SourceRaw[id] = append([]epathSQLQuantity(nil), monthsByID[id]...)
		// Ideal Loads equipment output is already model-total; Zone ownership
		// does not authorize multiplying the same measured quantity again.
		frames.SourceEffective[id] = append([]epathSQLQuantity(nil), monthsByID[id]...)
		for month := 1; month <= 12; month++ {
			key := epathSQLKey(zoneKey, detail.Service, month)
			frames.LoadDetailSourceIDs[key] = epathSQLDictionaryUnion(frames.LoadDetailSourceIDs[key], []int{id})
		}
	}
	return nil
}

func epathSQLLoadDetailSourceMatches(source EnergyDataSource, detail epathSQLLoadDetailIdentity) bool {
	if epathSQLValidateLoadDetailIdentity(detail) != nil || !epathSQLOriginalSourceMatches(source, epathSQLOriginalRDD(detail.Source), "annual") {
		return false
	}
	component := "load.dehumidification"
	if detail.Service == "heating" {
		component = "load.humidification"
	}
	method := "sum_report_data"
	if detail.Source.SourceUnit == "W" {
		method = "integrate_rate_by_time_interval"
	}
	return source.ID == fmt.Sprintf("sql-rdd-%d", detail.Source.DictionaryIndex) && strings.EqualFold(source.ZoneName, detail.ZoneName) && source.Units == detail.Source.SourceUnit && source.DriverRole == "context" && source.DriverCategory == "load."+detail.Service && source.DriverComponent == component && source.HeatDirection == detail.Service && source.InspectorSection == "Breakdown" && source.AggregationMethod == method && source.AggregationBasis == "model_total" && source.MultiplierApplication == "already_model_total" && source.EffectiveMultiplier == 1 && source.AllocationFactor == 1 && source.Formula == "" && len(source.InputSourceIDs) == 0
}

func epathCheckSQLLoadDetailSource(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	detail := check.LoadDetail
	if detail == nil || epathSQLValidateLoadDetailIdentity(*detail) != nil {
		return fmt.Errorf("missing valid independent load detail proof")
	}
	target := check.Item.Target
	if check.Item.Period != "annual" || check.Item.Group != "loads" || target.Collection != "sources" || (target.Field != "rawValue" && target.Field != "effectiveValue") || target.SourceName != detail.Source.Name || !strings.EqualFold(target.SourceKey, detail.Source.KeyValue) || target.Frequency != "Monthly" || target.SourceUnit != detail.Source.SourceUnit || target.Unit != "kWh" || check.Quantity == nil || !check.Quantity.valid() || (check.Item.Scope != "building" && check.Item.Scope != "zone") || check.Item.Scope == "building" && check.Item.Zone != "" || check.Item.Scope == "zone" && !strings.EqualFold(check.Item.Zone, detail.ZoneName) {
		return fmt.Errorf("load detail source proof is not bound to its exact quantity target")
	}
	count := 0
	for _, source := range bundle.EnergyExplanation.Sources {
		if source.ID != fmt.Sprintf("sql-rdd-%d", detail.Source.DictionaryIndex) && !(strings.EqualFold(source.Name, detail.Source.Name) && strings.EqualFold(source.KeyValue, detail.Source.KeyValue) && source.ReportingFrequency == detail.Source.ReportingFrequency) {
			continue
		}
		count++
		if !epathSQLLoadDetailSourceMatches(source, *detail) {
			return fmt.Errorf("non-additive load detail source metadata/owner contradicts original observation")
		}
	}
	if count != 1 {
		return fmt.Errorf("non-additive load detail requires exactly one original candidate source")
	}
	return nil
}
