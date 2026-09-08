package simulation

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

type epathSQLDirectHVACMonth struct {
	Quantity  epathSQLQuantity
	SourceIDs []int
	Present   bool // An observed zero is present; absence is never inferred from Value.
}

type epathSQLDirectHVACSourceIdentity struct {
	FamilyID, Service, Carrier, SiteID, AggregationBasis string
	Owner                                                epathRealSQLDirectHVACOwner
	Source                                               epathRealSQLSource
	Precision                                            epathRealSQLPrecision
}

func epathSQLDirectHVACKey(zone, service, carrier string, month int) string {
	return fmt.Sprintf("%s|%s|%s|%02d", strings.ToLower(strings.TrimSpace(zone)), service, carrier, month)
}

// This is an independent finite recipe taxonomy, not the production alias or
// component ownership helper. Names were reviewed against original RDD/MTD.
func epathSQLDirectHVACTaxonomy(id string) (name, component, service, carrier string, ok bool) {
	switch id {
	case "cooling.coil.electricity":
		return "Cooling Coil Electricity Energy", "Coil:Cooling:DX:SingleSpeed", "cooling", "electricity", true
	case "cooling.coil.crankcase_electricity":
		return "Cooling Coil Crankcase Heater Electricity Energy", "Coil:Cooling:DX:SingleSpeed", "cooling", "electricity", true
	case "heating.coil.natural_gas":
		return "Heating Coil NaturalGas Energy", "Coil:Heating:Fuel", "heating", "natural_gas", true
	case "heating.coil.ancillary_natural_gas":
		return "Heating Coil Ancillary NaturalGas Energy", "Coil:Heating:Fuel", "heating", "natural_gas", true
	case "heating.coil.electricity":
		return "Heating Coil Electricity Energy", "Coil:Heating:Fuel", "heating", "electricity", true
	}
	return "", "", "", "", false
}

func epathSQLDirectHVACOwners(declaration epathRealSQLDirectHVACComponent) (map[string]epathRealSQLDirectHVACOwner, error) {
	name, component, service, carrier, ok := epathSQLDirectHVACTaxonomy(declaration.ID)
	if !ok || declaration.Service != service || declaration.Carrier != carrier || declaration.SiteID == "" || declaration.Frequency != "Monthly" || declaration.AggregationBasis != "model_total" || declaration.Source.IsMeter || declaration.Source.AllowAbsent || len(declaration.Source.Alternatives) != 1 || declaration.Source.Alternatives[0] != (epathRealSQLAlternative{Name: name, Unit: "J"}) || len(declaration.Source.Keys) == 0 || len(declaration.Source.Keys) != len(declaration.Owners) {
		return nil, fmt.Errorf("direct HVAC requires an exact reviewed constituent, Monthly/J and model_total")
	}
	owners, keys := map[string]epathRealSQLDirectHVACOwner{}, map[string]bool{}
	for _, key := range declaration.Source.Keys {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" || key == "*" || keys[key] {
			return nil, fmt.Errorf("direct HVAC source requires unique exact coil keys")
		}
		keys[key] = true
	}
	for _, owner := range declaration.Owners {
		key := strings.ToLower(strings.TrimSpace(owner.KeyValue))
		if !keys[key] || owners[key].KeyValue != "" || strings.TrimSpace(owner.ZoneName) == "" || strings.TrimSpace(owner.EquipmentName) == "" || owner.EquipmentType != "ZoneHVAC:PackagedTerminalAirConditioner" || owner.ComponentType != component {
			return nil, fmt.Errorf("direct HVAC has a missing, duplicate or incompatible explicit coil owner")
		}
		owners[key] = owner
	}
	return owners, nil
}

func epathSQLValidateDirectHVACSourceIdentity(identity epathSQLDirectHVACSourceIdentity) error {
	name, component, service, carrier, ok := epathSQLDirectHVACTaxonomy(identity.FamilyID)
	source, owner := identity.Source, identity.Owner
	if !ok || identity.Service != service || identity.Carrier != carrier || identity.SiteID == "" || identity.AggregationBasis != "model_total" || owner.ComponentType != component || owner.EquipmentType != "ZoneHVAC:PackagedTerminalAirConditioner" || strings.TrimSpace(owner.EquipmentName) == "" || strings.TrimSpace(owner.ZoneName) == "" || strings.TrimSpace(owner.KeyValue) == "" || !strings.EqualFold(strings.TrimSpace(source.KeyValue), strings.TrimSpace(owner.KeyValue)) || !strings.EqualFold(source.Name, name) || source.SourceUnit != "J" || source.IsMeter || source.DictionaryIndex <= 0 || source.ReportingFrequency != "Monthly" || source.Rows != 12 || source.MissingRows != 0 {
		return fmt.Errorf("direct HVAC source lacks its exact independent owner/service/carrier/monthly identity")
	}
	if _, err := epathSQLMonthly(source, identity.Precision); err != nil {
		return err
	}
	for _, month := range source.Months {
		if month.MissingRows != 0 || month.RawSum == nil || !epathOracleFinite(*month.RawSum) || *month.RawSum < 0 || month.EnergyKWh == nil || *month.EnergyKWh < 0 {
			return fmt.Errorf("direct HVAC M%d has unknown/negative original energy", month.Month)
		}
		// Monthly contains one original J row. Permit only ordinary binary
		// conversion roundoff, never the display rounding budget or a scale fit.
		want := *month.RawSum / 3600000
		ulp := math.Nextafter(want, math.Inf(1)) - want
		if math.Abs(want-*month.EnergyKWh) > 2*ulp || want == 0 && *month.EnergyKWh != 0 || want != 0 && *month.EnergyKWh == 0 {
			return fmt.Errorf("direct HVAC normalized M%d does not match its original J row", month.Month)
		}
	}
	return nil
}

func epathSQLValidateDirectHVACRequests(plan *PurposeRunPlan, model epathRealSQLModel) error {
	if len(model.DirectHVACComponents) == 0 {
		return nil
	}
	if plan == nil {
		return fmt.Errorf("direct HVAC requires the actual executed output plan")
	}
	for _, declaration := range model.DirectHVACComponents {
		owners, err := epathSQLDirectHVACOwners(declaration)
		if err != nil {
			return err
		}
		for key, owner := range owners {
			count := 0
			for _, output := range plan.OutputObjects {
				if !strings.EqualFold(output.VariableName, declaration.Source.Alternatives[0].Name) || !strings.EqualFold(output.ReportingFrequency, "Monthly") || strings.ToLower(strings.TrimSpace(output.KeyValue)) != key {
					continue
				}
				basic := false
				for _, purpose := range output.PurposeIDs {
					basic = basic || purpose == SimulationPurposeBasicEnergy
				}
				if !strings.EqualFold(output.ObjectType, "Output:Variable") || !strings.EqualFold(strings.TrimSpace(output.ScopeZoneName), strings.TrimSpace(owner.ZoneName)) || !basic {
					return fmt.Errorf("direct HVAC request has a conflicting exact component owner")
				}
				count++
			}
			if count != 1 {
				return fmt.Errorf("direct HVAC requires exactly one executed Monthly request for %s/%s", declaration.ID, key)
			}
		}
	}
	return nil
}

func epathCompileSQLDirectHVACSources(sqlPath string, observed []epathRealSQLSource, model epathRealSQLModel, frames *epathSQLFrames) error {
	if len(model.DirectHVACComponents) == 0 {
		return nil
	}
	if frames == nil || len(frames.Zones) == 0 || len(frames.DirectHVAC) != 0 || len(frames.DirectHVACSourceIdentities) != 0 {
		return fmt.Errorf("direct HVAC requires fresh independently observed Zone frames")
	}
	db, err := epathOpenOracleSQL(sqlPath)
	if err != nil {
		return err
	}
	defer db.Close()
	identities, months := map[int]epathSQLDirectHVACSourceIdentity{}, map[string]epathSQLDirectHVACMonth{}
	seenFamilies, componentOwners, parentZones := map[string]bool{}, map[string]string{}, map[string]string{}
	parentFamilies := map[string]map[string]bool{}
	parentComponents := map[string]string{}
	for _, declaration := range model.DirectHVACComponents {
		owners, err := epathSQLDirectHVACOwners(declaration)
		if err != nil || seenFamilies[declaration.ID] {
			return fmt.Errorf("invalid/duplicate direct HVAC family: %v", err)
		}
		seenFamilies[declaration.ID] = true
		siteCount := 0
		for _, site := range model.Site {
			if site.ID == declaration.SiteID {
				siteCount++
				if site.Facility || site.EndUse != declaration.Service || site.Carrier != declaration.Carrier || site.Tabular != nil || len(frames.Site[site.ID]) != 12 {
					return fmt.Errorf("direct HVAC constituent belongs to an incompatible site pool")
				}
			}
		}
		if siteCount != 1 {
			return fmt.Errorf("direct HVAC constituent needs one independently observed site pool")
		}
		sources, err := epathSQLSelect(observed, declaration.Source)
		if err != nil {
			return err
		}
		selected := map[int]epathRealSQLSource{}
		for _, source := range sources {
			selected[source.DictionaryIndex] = source
		}
		// Include dictionaries with no ReportData rows: absence of observations
		// must not conceal a duplicated identity or undeclared physical owner.
		rows, err := db.Query(`SELECT ReportDataDictionaryIndex,Name,KeyValue,IsMeter,ReportingFrequency,Units FROM ReportDataDictionary WHERE Name=? COLLATE NOCASE`, declaration.Source.Alternatives[0].Name)
		if err != nil {
			return err
		}
		dictionarySeen := map[int]bool{}
		for rows.Next() {
			var id, meter int
			var name, key, frequency, unit string
			if err := rows.Scan(&id, &name, &key, &meter, &frequency, &unit); err != nil {
				rows.Close()
				return err
			}
			if !strings.EqualFold(frequency, "Monthly") {
				continue
			}
			original, exists := selected[id]
			if !exists || dictionarySeen[id] || meter != 0 || unit != "J" || !strings.EqualFold(name, original.Name) || !strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(original.KeyValue)) || owners[strings.ToLower(strings.TrimSpace(key))].KeyValue == "" {
				rows.Close()
				return fmt.Errorf("direct HVAC original dictionary has duplicate, undeclared or inconsistent identity")
			}
			dictionarySeen[id] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(dictionarySeen) != len(sources) {
			return fmt.Errorf("direct HVAC observation lost its original dictionary row")
		}
		for _, source := range sources {
			owner := owners[strings.ToLower(strings.TrimSpace(source.KeyValue))]
			zoneKey := strings.ToLower(strings.TrimSpace(owner.ZoneName))
			zone, exists := frames.Zones[zoneKey]
			if !exists || !strings.EqualFold(zone.Name, owner.ZoneName) || identities[source.DictionaryIndex].Source.DictionaryIndex != 0 || frames.SourceIdentities[source.DictionaryIndex].DictionaryIndex != 0 {
				return fmt.Errorf("direct HVAC source has unknown Zone or reused source identity")
			}
			owner.ZoneName = zone.Name
			parent := strings.ToLower(strings.TrimSpace(owner.EquipmentName))
			component := strings.ToLower(owner.ComponentType) + "|" + strings.ToLower(strings.TrimSpace(owner.KeyValue))
			binding := parent + "|" + zoneKey
			if componentOwners[component] != "" && componentOwners[component] != binding || parentZones[parent] != "" && parentZones[parent] != zoneKey {
				return fmt.Errorf("direct HVAC component or package has conflicting reviewed ownership")
			}
			componentOwners[component], parentZones[parent] = binding, zoneKey
			parentComponent := parent + "|" + strings.ToLower(owner.ComponentType)
			if prior := parentComponents[parentComponent]; prior != "" && prior != component {
				return fmt.Errorf("direct HVAC package declares different physical coils for one constituent cohort")
			}
			parentComponents[parentComponent] = component
			if parentFamilies[parent] == nil {
				parentFamilies[parent] = map[string]bool{}
			}
			if parentFamilies[parent][declaration.ID] {
				return fmt.Errorf("direct HVAC package has duplicate constituent ownership")
			}
			parentFamilies[parent][declaration.ID] = true
			identity := epathSQLDirectHVACSourceIdentity{FamilyID: declaration.ID, Service: declaration.Service, Carrier: declaration.Carrier, SiteID: declaration.SiteID, AggregationBasis: declaration.AggregationBasis, Owner: owner, Source: source, Precision: model.Precision}
			if err := epathSQLValidateDirectHVACSourceIdentity(identity); err != nil {
				return err
			}
			values, _ := epathSQLMonthly(source, model.Precision)
			identities[source.DictionaryIndex] = identity
			for index, q := range values {
				key := epathSQLDirectHVACKey(zone.Name, declaration.Service, declaration.Carrier, index+1)
				month := months[key]
				month.Quantity = month.Quantity.add(q.positive())
				month.SourceIDs = epathSQLDictionaryUnion(month.SourceIDs, []int{source.DictionaryIndex})
				month.Present = true
				months[key] = month
			}
		}
	}
	for _, families := range parentFamilies {
		if len(families) != 5 {
			return fmt.Errorf("direct HVAC package is missing a reviewed additive constituent, not a complete direct observation")
		}
	}
	// Commit only after all validation succeeds; no failed partial source frame.
	frames.DirectHVAC, frames.DirectHVACSourceIdentities = months, identities
	if frames.SourceIdentities == nil {
		frames.SourceIdentities = map[int]epathRealSQLSource{}
	}
	if frames.SourceZone == nil {
		frames.SourceZone = map[int]string{}
	}
	for id, identity := range identities {
		frames.SourceIdentities[id] = identity.Source
		frames.SourceZone[id] = strings.ToLower(identity.Owner.ZoneName)
	}
	return nil
}

func epathSQLMatchDirectHVACSource(actual EnergyDataSource, identity epathSQLDirectHVACSourceIdentity) error {
	if err := epathSQLValidateDirectHVACSourceIdentity(identity); err != nil {
		return err
	}
	original := identity.Source
	if actual.ID != fmt.Sprintf("sql-rdd-%d", original.DictionaryIndex) || actual.SourceType != "sql_report_data" || actual.IsMeter || !strings.EqualFold(actual.Name, original.Name) || !strings.EqualFold(strings.TrimSpace(actual.KeyValue), strings.TrimSpace(original.KeyValue)) || !strings.EqualFold(actual.ZoneName, identity.Owner.ZoneName) || actual.SourceUnit != "J" || actual.Units != "J" || actual.NormalizedUnit != "kWh" || actual.ReportingFrequency != "Monthly" || actual.AggregationMethod != "sum_report_data" || actual.AggregationBasis != "model_total" || actual.EffectiveMultiplier != 1 || actual.MultiplierApplication != "already_model_total" || len(actual.InputSourceIDs) != 0 {
		return fmt.Errorf("direct HVAC candidate source contradicts its original identity/model-total owner")
	}
	return nil
}

func epathSQLDirectHVACAnnualQuantity(identity epathSQLDirectHVACSourceIdentity) (epathSQLQuantity, error) {
	if err := epathSQLValidateDirectHVACSourceIdentity(identity); err != nil {
		return epathSQLQuantity{}, err
	}
	values, _ := epathSQLMonthly(identity.Source, identity.Precision)
	out := epathSQLQuantity{}
	for _, q := range values {
		out = out.add(q.positive())
	}
	return out, nil
}

func epathSQLModelDirectHVACSourceChecks(frames epathSQLFrames, checks *epathSQLModelChecks) error {
	ids := []int{}
	for id := range frames.DirectHVACSourceIdentities {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		identity := frames.DirectHVACSourceIdentities[id]
		if id != identity.Source.DictionaryIndex || !reflect.DeepEqual(frames.SourceIdentities[id], identity.Source) || frames.SourceZone[id] != strings.ToLower(identity.Owner.ZoneName) {
			return fmt.Errorf("direct HVAC proof lost its independently registered source/owner")
		}
		q, err := epathSQLDirectHVACAnnualQuantity(identity)
		if err != nil {
			return err
		}
		for _, scope := range []string{"building", "zone"} {
			zone := ""
			if scope == "zone" {
				zone = identity.Owner.ZoneName
			}
			for _, field := range []string{"rawValue", "effectiveValue"} {
				source := identity.Source
				target := epathRealOracleTarget{Collection: "sources", Field: field, SourceName: source.Name, SourceKey: source.KeyValue, SourceUnit: "J", Frequency: "Monthly", Unit: "kWh"}
				if err := checks.add("endUses", scope, zone, "annual", "direct_hvac_source/"+identity.FamilyID+"/"+source.KeyValue+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
					return err
				}
				proof := identity
				checks.Rows[len(checks.Rows)-1].DirectHVACSource = &proof
			}
		}
	}
	return nil
}

func epathCheckSQLDirectHVACSource(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	identity, target := check.DirectHVACSource, check.Item.Target
	if identity == nil {
		return fmt.Errorf("missing independent direct HVAC source proof")
	}
	q, err := epathSQLDirectHVACAnnualQuantity(*identity)
	if err != nil {
		return err
	}
	if check.Item.Group != "endUses" || check.Item.Period != "annual" || check.Item.Unit != "kWh" || check.Quantity == nil || !reflect.DeepEqual(*check.Quantity, q) || target.Collection != "sources" || target.Field != "rawValue" && target.Field != "effectiveValue" || target.SourceName != identity.Source.Name || target.SourceKey != identity.Source.KeyValue || target.SourceUnit != "J" || target.Frequency != "Monthly" || target.Unit != "kWh" || check.Item.Scope != "building" && check.Item.Scope != "zone" || check.Item.Scope == "building" && check.Item.Zone != "" || check.Item.Scope == "zone" && !strings.EqualFold(check.Item.Zone, identity.Owner.ZoneName) {
		return fmt.Errorf("direct HVAC source proof escaped its exact numeric/context binding")
	}
	count := 0
	var matched *EnergyDataSource
	for _, source := range bundle.EnergyExplanation.Sources {
		if source.ID != fmt.Sprintf("sql-rdd-%d", identity.Source.DictionaryIndex) && !(strings.EqualFold(source.Name, identity.Source.Name) && strings.EqualFold(source.KeyValue, identity.Source.KeyValue) && strings.EqualFold(source.ReportingFrequency, "Monthly")) {
			continue
		}
		count++
		if err := epathSQLMatchDirectHVACSource(source, *identity); err != nil {
			return err
		}
		copy := source
		matched = &copy
	}
	if count != 1 {
		return fmt.Errorf("direct HVAC proof needs exactly one original candidate source")
	}
	low, high := q.bounds()
	if check.Item.Scope == "zone" && q.Value == 0 && low == 0 && high == 0 &&
		bundle.EnergyExplanation.Schema == energyExplanationSchema && matched != nil &&
		len(matched.ScopeDetails) == 0 && matched.inspectorDecodedFromJSON {
		// A pruned all-zero coil has no Zone graph and therefore no generated
		// scope detail. Its original annual scalar is still explicitly present
		// and already belongs to this exact independently proved Zone owner.
		// Never use this path for nonzero values, an existing scope detail, an
		// undecoded/sparse scalar, an unowned Building pool, or a monthly value.
		value, bit := matched.RawValue, uint8(1)
		if target.Field == "effectiveValue" {
			value, bit = matched.EffectiveValue, 2
		}
		if value == 0 && matched.inspectorValuePresence&bit != 0 {
			return epathCheckSQLModelQuantity(&value, &q)
		}
	}
	actual, err := epathReadOracleCandidate(bundle, check.Item, check.Want)
	if err != nil {
		return err
	}
	return epathCheckSQLModelQuantity(actual, &q)
}
