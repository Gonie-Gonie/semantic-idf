package simulation

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
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
	case "heating.coil.dx_electricity":
		return "Heating Coil Electricity Energy", "Coil:Heating:DX:SingleSpeed", "heating", "electricity", true
	case "heating.coil.defrost_electricity":
		return "Heating Coil Defrost Electricity Energy", "Coil:Heating:DX:SingleSpeed", "heating", "electricity", true
	case "heating.coil.crankcase_electricity":
		return "Heating Coil Crankcase Heater Electricity Energy", "Coil:Heating:DX:SingleSpeed", "heating", "electricity", true
	}
	return "", "", "", "", false
}

func epathSQLDirectHVACParentFamilies(objectType string) []string {
	switch objectType {
	case "ZoneHVAC:PackagedTerminalAirConditioner":
		return []string{"cooling.coil.electricity", "cooling.coil.crankcase_electricity", "heating.coil.natural_gas", "heating.coil.ancillary_natural_gas", "heating.coil.electricity"}
	case "ZoneHVAC:PackagedTerminalHeatPump":
		return []string{"cooling.coil.electricity", "heating.coil.dx_electricity", "heating.coil.defrost_electricity", "heating.coil.crankcase_electricity", "heating.coil.natural_gas", "heating.coil.ancillary_natural_gas", "heating.coil.electricity"}
	}
	return nil
}

func epathSQLDirectHVACParentSupports(objectType, family string) bool {
	for _, id := range epathSQLDirectHVACParentFamilies(objectType) {
		if id == family {
			return true
		}
	}
	return false
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
		if !keys[key] || owners[key].KeyValue != "" || strings.TrimSpace(owner.ZoneName) == "" || strings.TrimSpace(owner.EquipmentName) == "" || !epathSQLDirectHVACParentSupports(owner.EquipmentType, declaration.ID) || owner.ComponentType != component {
			return nil, fmt.Errorf("direct HVAC has a missing, duplicate or incompatible explicit coil owner")
		}
		owners[key] = owner
	}
	return owners, nil
}

func epathSQLValidateDirectHVACSourceIdentity(identity epathSQLDirectHVACSourceIdentity) error {
	name, component, service, carrier, ok := epathSQLDirectHVACTaxonomy(identity.FamilyID)
	source, owner := identity.Source, identity.Owner
	if !ok || identity.Service != service || identity.Carrier != carrier || identity.SiteID == "" || identity.AggregationBasis != "model_total" || owner.ComponentType != component || !epathSQLDirectHVACParentSupports(owner.EquipmentType, identity.FamilyID) || strings.TrimSpace(owner.EquipmentName) == "" || strings.TrimSpace(owner.ZoneName) == "" || strings.TrimSpace(owner.KeyValue) == "" || !strings.EqualFold(strings.TrimSpace(source.KeyValue), strings.TrimSpace(owner.KeyValue)) || !strings.EqualFold(source.Name, name) || source.SourceUnit != "J" || source.IsMeter || source.DictionaryIndex <= 0 || source.ReportingFrequency != "Monthly" || source.Rows != 12 || source.MissingRows != 0 {
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

// Real-evidence entry points additionally bind this text to the original model
// hash. Only the low-level IDF parser is shared; typed field positions and the
// ownership walk below are independent of production HVAC discovery. SQL cannot
// distinguish a DX/Fuel object's type from their shared output-variable name.
func epathSQLValidateDirectHVACOriginalModel(text string, model epathRealSQLModel) error {
	if len(model.DirectHVACComponents) == 0 {
		return nil
	}
	doc, err := idf.Parse(text)
	if err != nil {
		return err
	}
	key := func(objectType, name string) string {
		return strings.ToLower(strings.TrimSpace(objectType)) + "|" + strings.ToLower(strings.TrimSpace(name))
	}
	field := func(object idf.Object, index int) string {
		if index >= len(object.Fields) {
			return ""
		}
		return strings.TrimSpace(object.Fields[index].Value)
	}
	objects := map[string][]idf.Object{}
	for _, object := range doc.Objects {
		if field(object, 0) != "" {
			k := key(object.Type, field(object, 0))
			objects[k] = append(objects[k], object)
		}
	}
	one := func(objectType, name string) (idf.Object, error) {
		values := objects[key(objectType, name)]
		if strings.TrimSpace(name) == "" || len(values) != 1 {
			return idf.Object{}, fmt.Errorf("original direct owner needs one exact %s/%s object", objectType, name)
		}
		return values[0], nil
	}
	for _, declaration := range model.DirectHVACComponents {
		owners, err := epathSQLDirectHVACOwners(declaration)
		if err != nil {
			return err
		}
		for _, owner := range owners {
			parent, err := one(owner.EquipmentType, owner.EquipmentName)
			if err != nil {
				return err
			}
			coil, err := one(owner.ComponentType, owner.KeyValue)
			if err != nil {
				return err
			}
			if owner.ComponentType == "Coil:Heating:Fuel" && !strings.EqualFold(field(coil, 2), "NaturalGas") {
				return fmt.Errorf("original supplemental coil is not the declared NaturalGas consumer")
			}
			position := -1
			switch owner.EquipmentType {
			case "ZoneHVAC:PackagedTerminalAirConditioner":
				if owner.ComponentType == "Coil:Heating:Fuel" {
					position = 15
				} else if owner.ComponentType == "Coil:Cooling:DX:SingleSpeed" {
					position = 17
				}
			case "ZoneHVAC:PackagedTerminalHeatPump":
				switch owner.ComponentType {
				case "Coil:Heating:DX:SingleSpeed":
					position = 15
				case "Coil:Cooling:DX:SingleSpeed":
					position = 18
				case "Coil:Heating:Fuel":
					position = 21
				}
			}
			componentKey, parentKey := key(owner.ComponentType, owner.KeyValue), key(owner.EquipmentType, owner.EquipmentName)
			if position < 0 || key(field(parent, position), field(parent, position+1)) != componentKey {
				return fmt.Errorf("recipe coil type/key does not match the original parent's exact physical role")
			}
			coilReferences, listReferences, mixerReferences := 0, []idf.Object{}, 0
			for _, object := range doc.Objects {
				for index := 0; index+1 < len(object.Fields); index++ {
					reference := key(field(object, index), field(object, index+1))
					if reference == componentKey {
						coilReferences++
						if object.Index != parent.Index || index != position {
							return fmt.Errorf("original coil is shared or referenced outside its declared typed parent field")
						}
					}
					if reference != parentKey {
						continue
					}
					switch {
					case strings.EqualFold(object.Type, "ZoneHVAC:EquipmentList") && index >= 2 && (index-2)%6 == 0:
						listReferences = append(listReferences, object)
					case strings.EqualFold(object.Type, "AirTerminal:SingleDuct:Mixer") && index == 1:
						mixerReferences++
						if _, err := one(object.Type, field(object, 0)); err != nil {
							return err
						}
						connection := field(object, 6)
						inlet := strings.EqualFold(connection, "InletSide") && field(parent, 2) != "" && strings.EqualFold(field(object, 3), field(parent, 2))
						outlet := strings.EqualFold(connection, "SupplySide") && field(parent, 3) != "" && strings.EqualFold(field(object, 5), field(parent, 3))
						if !inlet && !outlet {
							return fmt.Errorf("original DOAS mixer is not this package's exact air connection")
						}
					default:
						return fmt.Errorf("original package has an unsupported/shared typed parent reference")
					}
				}
			}
			if coilReferences != 1 || len(listReferences) != 1 || mixerReferences > 1 {
				return fmt.Errorf("original direct package or coil does not have unique physical ownership")
			}
			list := listReferences[0]
			if _, err := one(list.Type, field(list, 0)); err != nil {
				return err
			}
			if _, err := one("Zone", owner.ZoneName); err != nil {
				return err
			}
			connections, zoneConnections := 0, 0
			for _, object := range doc.Objects {
				if !strings.EqualFold(object.Type, "ZoneHVAC:EquipmentConnections") {
					continue
				}
				if strings.EqualFold(field(object, 0), owner.ZoneName) {
					zoneConnections++
				}
				if strings.EqualFold(field(object, 1), field(list, 0)) {
					connections++
					if !strings.EqualFold(field(object, 0), owner.ZoneName) {
						return fmt.Errorf("original equipment list belongs to a different Zone")
					}
				}
			}
			if connections != 1 || zoneConnections != 1 {
				return fmt.Errorf("original equipment list/Zone ownership is missing or ambiguous")
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
	parentTypes := map[string]string{}
	// A literal output name is not a physical role: DX and Fuel heating
	// electricity share a name but have separately reviewed key/type owners.
	nameKeys := map[string]map[string]string{}
	for _, declaration := range model.DirectHVACComponents {
		owners, err := epathSQLDirectHVACOwners(declaration)
		if err != nil {
			return err
		}
		name := strings.ToLower(declaration.Source.Alternatives[0].Name)
		if nameKeys[name] == nil {
			nameKeys[name] = map[string]string{}
		}
		for key := range owners {
			if nameKeys[name][key] != "" {
				return fmt.Errorf("same-name direct HVAC roles have a duplicate physical source key")
			}
			nameKeys[name][key] = declaration.ID
		}
	}
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
			declaredRole := nameKeys[strings.ToLower(name)][strings.ToLower(strings.TrimSpace(key))]
			if declaredRole == "" || meter != 0 || unit != "J" {
				rows.Close()
				return fmt.Errorf("direct HVAC original dictionary has an undeclared or inconsistent physical source")
			}
			if declaredRole != declaration.ID {
				continue // The other exact typed role validates its own rows.
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
			if prior := parentTypes[parent]; prior != "" && prior != owner.EquipmentType {
				return fmt.Errorf("direct HVAC package has contradictory original equipment types")
			}
			parentTypes[parent] = owner.EquipmentType
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
	for parent, families := range parentFamilies {
		roster := epathSQLDirectHVACParentFamilies(parentTypes[parent])
		if len(families) != len(roster) || len(roster) == 0 {
			return fmt.Errorf("direct HVAC package is missing a reviewed additive constituent, not a complete direct observation")
		}
		for _, family := range roster {
			if !families[family] {
				return fmt.Errorf("direct HVAC package lacks its exact parent-specific constituent %s", family)
			}
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
