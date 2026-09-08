package simulation

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// This is an independent original-input declaration, not a production target
// or a source identity inferred from candidate node metadata.
type epathRealSQLRadiantOwner struct {
	EquipmentType string `json:"equipmentType"`
	EquipmentName string `json:"equipmentName"`
	DesignName    string `json:"designName"`
	SurfaceName   string `json:"surfaceName"`
	ZoneName      string `json:"zoneName"`
}

type epathRealSQLRadiantLoadBinding struct {
	Frequency        string                     `json:"frequency"`
	AggregationBasis string                     `json:"aggregationBasis"`
	ThermalBoundary  string                     `json:"thermalBoundary"`
	Owners           []epathRealSQLRadiantOwner `json:"owners"`
}

type epathSQLRadiantOwnerProof struct {
	Owner                                                epathRealSQLRadiantOwner
	EquipmentIndex, DesignIndex, SurfaceIndex, ZoneIndex int
	ZoneMultiplier, ZoneListMultiplier                   float64
}

type epathSQLRadiantLoadSourceIdentity struct {
	Owner             epathSQLRadiantOwnerProof
	Source            epathRealSQLSource
	Service           string
	ThermalBoundary   string
	AggregationBasis  string
	AppliedMultiplier float64
	Precision         epathRealSQLPrecision
	Raw, Effective    [12]epathSQLQuantity
}

func epathSQLRadiantName(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func epathSQLRadiantField(object idf.Object, index int) string {
	if index < 0 || index >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[index].Value)
}

func epathSQLRadiantFactor(text string) (float64, error) {
	if strings.TrimSpace(text) == "" {
		return 1, nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || !epathOracleFinite(value) || value <= 0 || math.Trunc(value) != value {
		return 0, fmt.Errorf("radiant original multiplier must be a positive integer")
	}
	return value, nil
}

// The reviewed schema has a typed ConstantFlow:Design reference at field 1,
// explicit Zone/surface at 3/4, and separate H/C water ports at 10/11 and 16/17.
// No production classifier, owner analyzer, output builder or candidate is used.
func epathSQLRadiantOriginalOwners(originalText string, binding epathRealSQLRadiantLoadBinding) (map[string]epathSQLRadiantOwnerProof, error) {
	if strings.TrimSpace(originalText) == "" || binding.Frequency != "Monthly" || binding.AggregationBasis != "model_total" || binding.ThermalBoundary != "active_surface_source" || len(binding.Owners) == 0 {
		return nil, fmt.Errorf("radiant requires original input and explicit Monthly/model-total/active-surface ownership")
	}
	doc, err := idf.Parse(originalText)
	if err != nil {
		return nil, err
	}
	key := func(kind, name string) string { return epathSQLRadiantName(kind) + "|" + epathSQLRadiantName(name) }
	objects := map[string][]idf.Object{}
	for _, object := range doc.Objects {
		objects[key(object.Type, epathSQLRadiantField(object, 0))] = append(objects[key(object.Type, epathSQLRadiantField(object, 0))], object)
	}
	one := func(kind, name string) (idf.Object, error) {
		items := objects[key(kind, name)]
		if strings.TrimSpace(name) == "" || name == "*" || len(items) != 1 {
			return idf.Object{}, fmt.Errorf("radiant requires exactly one original %s/%s", kind, name)
		}
		return items[0], nil
	}
	proofs := map[string]epathSQLRadiantOwnerProof{}
	zones, surfaces := map[string]bool{}, map[string]bool{}
	for _, declared := range binding.Owners {
		ownerKey, zoneKey, surfaceKey := epathSQLRadiantName(declared.EquipmentName), epathSQLRadiantName(declared.ZoneName), epathSQLRadiantName(declared.SurfaceName)
		if declared.EquipmentType != "ZoneHVAC:LowTemperatureRadiant:ConstantFlow" || ownerKey == "" || zoneKey == "" || surfaceKey == "" || proofs[ownerKey].Owner.EquipmentName != "" || zones[zoneKey] || surfaces[surfaceKey] {
			return nil, fmt.Errorf("radiant declarations require unique native equipment, Zone and direct surface owners")
		}
		parent, err := one(declared.EquipmentType, declared.EquipmentName)
		if err != nil {
			return nil, err
		}
		design, err := one("ZoneHVAC:LowTemperatureRadiant:ConstantFlow:Design", declared.DesignName)
		if err != nil {
			return nil, err
		}
		zone, err := one("Zone", declared.ZoneName)
		if err != nil {
			return nil, err
		}
		surface, err := one("BuildingSurface:Detailed", declared.SurfaceName)
		if err != nil {
			return nil, err
		}
		if len(parent.Fields) < 22 || len(design.Fields) < 2 || !strings.EqualFold(epathSQLRadiantField(parent, 1), declared.DesignName) || !strings.EqualFold(epathSQLRadiantField(parent, 3), declared.ZoneName) || !strings.EqualFold(epathSQLRadiantField(parent, 4), declared.SurfaceName) || !strings.EqualFold(epathSQLRadiantField(surface, 3), declared.ZoneName) || len(objects[key("ZoneHVAC:LowTemperatureRadiant:SurfaceGroup", declared.SurfaceName)]) != 0 {
			return nil, fmt.Errorf("radiant original design/explicit Zone/direct surface disagrees with the reviewed owner")
		}
		if spaceName := epathSQLRadiantField(surface, 4); spaceName != "" {
			space, err := one("Space", spaceName)
			if err != nil || !strings.EqualFold(epathSQLRadiantField(space, 1), declared.ZoneName) {
				return nil, fmt.Errorf("radiant surface has foreign or unknown Space ownership")
			}
		}
		ports := map[string]bool{}
		for _, field := range []int{10, 11, 16, 17} {
			name := epathSQLRadiantName(epathSQLRadiantField(parent, field))
			if name == "" || ports[name] {
				return nil, fmt.Errorf("radiant requires the reviewed separate native water ports")
			}
			ports[name] = true
		}
		var lists []idf.Object
		branchPairs := map[string]bool{}
		for _, object := range doc.Objects {
			for field := 0; field+1 < len(object.Fields); field++ {
				if key(epathSQLRadiantField(object, field), epathSQLRadiantField(object, field+1)) != key(parent.Type, declared.EquipmentName) {
					continue
				}
				switch {
				case strings.EqualFold(object.Type, "ZoneHVAC:EquipmentList") && field >= 2 && (field-2)%6 == 0:
					lists = append(lists, object)
				case strings.EqualFold(object.Type, "Branch") && field >= 2 && (field-2)%4 == 0:
					in, out := epathSQLRadiantField(object, field+2), epathSQLRadiantField(object, field+3)
					pair := epathSQLRadiantName(in) + "|" + epathSQLRadiantName(out)
					matches := strings.EqualFold(in, epathSQLRadiantField(parent, 10)) && strings.EqualFold(out, epathSQLRadiantField(parent, 11)) || strings.EqualFold(in, epathSQLRadiantField(parent, 16)) && strings.EqualFold(out, epathSQLRadiantField(parent, 17))
					if !matches || branchPairs[pair] {
						return nil, fmt.Errorf("radiant has crossed or duplicate original Branch ports")
					}
					if _, err := one("Branch", epathSQLRadiantField(object, 0)); err != nil {
						return nil, err
					}
					branchPairs[pair] = true
				default:
					return nil, fmt.Errorf("radiant has an unsupported, misplaced or foreign typed parent reference")
				}
			}
		}
		if len(lists) != 1 {
			return nil, fmt.Errorf("radiant must appear once in one original EquipmentList")
		}
		listName := epathSQLRadiantField(lists[0], 0)
		if _, err := one("ZoneHVAC:EquipmentList", listName); err != nil {
			return nil, err
		}
		zoneConnections, listConnections := 0, 0
		for _, object := range doc.Objects {
			if !strings.EqualFold(object.Type, "ZoneHVAC:EquipmentConnections") {
				continue
			}
			if strings.EqualFold(epathSQLRadiantField(object, 0), declared.ZoneName) {
				zoneConnections++
			}
			if strings.EqualFold(epathSQLRadiantField(object, 1), listName) {
				listConnections++
				if !strings.EqualFold(epathSQLRadiantField(object, 0), declared.ZoneName) {
					return nil, fmt.Errorf("radiant EquipmentList is owned by a foreign Zone")
				}
			}
		}
		if zoneConnections != 1 || listConnections != 1 {
			return nil, fmt.Errorf("radiant original Zone/EquipmentConnections ownership is missing or ambiguous")
		}
		zoneMultiplier, err := epathSQLRadiantFactor(epathSQLRadiantField(zone, 6))
		if err != nil {
			return nil, err
		}
		listMultiplier, groupCount := 1.0, 0
		for _, group := range doc.Objects {
			if !strings.EqualFold(group.Type, "ZoneGroup") {
				continue
			}
			list, err := one("ZoneList", epathSQLRadiantField(group, 1))
			if err != nil {
				return nil, err
			}
			for field := 1; field < len(list.Fields); field++ {
				if strings.EqualFold(epathSQLRadiantField(list, field), declared.ZoneName) {
					groupCount++
					listMultiplier, err = epathSQLRadiantFactor(epathSQLRadiantField(group, 2))
					if err != nil {
						return nil, err
					}
				}
			}
		}
		if groupCount > 1 || !epathOracleFinite(zoneMultiplier*listMultiplier) {
			return nil, fmt.Errorf("radiant has duplicate/nonfinite original ZoneGroup multiplication")
		}
		declared.EquipmentName, declared.DesignName, declared.SurfaceName, declared.ZoneName = epathSQLRadiantField(parent, 0), epathSQLRadiantField(design, 0), epathSQLRadiantField(surface, 0), epathSQLRadiantField(zone, 0)
		proofs[ownerKey] = epathSQLRadiantOwnerProof{Owner: declared, EquipmentIndex: parent.Index, DesignIndex: design.Index, SurfaceIndex: surface.Index, ZoneIndex: zone.Index, ZoneMultiplier: zoneMultiplier, ZoneListMultiplier: listMultiplier}
		zones[zoneKey], surfaces[surfaceKey] = true, true
	}
	for _, object := range doc.Objects {
		kind := epathSQLRadiantName(object.Type)
		if strings.HasPrefix(kind, "zonehvac:lowtemperatureradiant:") && !strings.HasSuffix(kind, ":design") && !strings.HasSuffix(kind, ":surfacegroup") {
			if kind != "zonehvac:lowtemperatureradiant:constantflow" || proofs[epathSQLRadiantName(epathSQLRadiantField(object, 0))].Owner.EquipmentName == "" {
				return nil, fmt.Errorf("reviewed radiant load omits an original native/unsupported equipment owner")
			}
		}
		if kind == "zonehvac:lowtemperatureradiant:surfacegroup" {
			for field := 1; field < len(object.Fields); field += 2 {
				if surfaces[epathSQLRadiantName(epathSQLRadiantField(object, field))] {
					return nil, fmt.Errorf("direct radiant surface is also claimed by an unreviewed surface group")
				}
			}
		}
	}
	return proofs, nil
}

func epathSQLRadiantLoadObservation(source epathRealSQLSource, service string, owner epathSQLRadiantOwnerProof, zone epathSQLZone, precision epathRealSQLPrecision) (epathSQLRadiantLoadSourceIdentity, error) {
	out := epathSQLRadiantLoadSourceIdentity{}
	name := "Zone Radiant HVAC Cooling Energy"
	if service == "heating" {
		name = "Zone Radiant HVAC Heating Energy"
	} else if service != "cooling" {
		return out, fmt.Errorf("unknown radiant service")
	}
	if owner.Owner.EquipmentType != "ZoneHVAC:LowTemperatureRadiant:ConstantFlow" || owner.Owner.DesignName == "" || owner.Owner.SurfaceName == "" || owner.EquipmentIndex < 0 || owner.DesignIndex < 0 || owner.SurfaceIndex < 0 || owner.ZoneIndex < 0 || owner.ZoneMultiplier <= 0 || owner.ZoneListMultiplier <= 0 || !epathOracleFinite(zone.Multiplier) || zone.Multiplier != owner.ZoneMultiplier*owner.ZoneListMultiplier || !strings.EqualFold(zone.Name, owner.Owner.ZoneName) ||
		source.DictionaryIndex <= 0 || !strings.EqualFold(source.Name, name) || !strings.EqualFold(source.KeyValue, owner.Owner.EquipmentName) || source.SourceUnit != "J" || source.ReportingFrequency != "Monthly" || source.IsMeter || source.Rows != 12 || source.MissingRows != 0 || source.RawSum == nil || source.EnergyKWh == nil {
		return out, fmt.Errorf("radiant source/SQL Zone contradicts exact original ownership or Monthly/J observations")
	}
	values, err := epathSQLMonthly(source, precision)
	if err != nil {
		return out, err
	}
	rawTotal, energyTotal := 0.0, 0.0
	for month, bucket := range source.Months {
		if bucket.MissingRows != 0 || bucket.RawSum == nil || !epathOracleFinite(*bucket.RawSum) || *bucket.RawSum < 0 || values[month].Value < 0 || !values[month].valid() {
			return out, fmt.Errorf("radiant source contains unknown, negative or nonfinite original monthly heat")
		}
		want := *bucket.RawSum / 3600000
		ulp := math.Nextafter(want, math.Inf(1)) - want
		if math.Abs(want-values[month].Value) > 2*ulp || want == 0 && values[month].Value != 0 || want != 0 && values[month].Value == 0 {
			return out, fmt.Errorf("radiant normalized heat differs from its original J observation")
		}
		out.Raw[month], out.Effective[month] = values[month], values[month]
		rawTotal += *bucket.RawSum
		energyTotal += values[month].Value
	}
	if rawTotal != *source.RawSum || energyTotal != *source.EnergyKWh || !epathOracleFinite(rawTotal) || !epathOracleFinite(energyTotal) {
		return out, fmt.Errorf("radiant annual observation differs from its twelve original months")
	}
	out.Owner, out.Source, out.Service = owner, source, service
	out.Precision = precision
	out.ThermalBoundary, out.AggregationBasis, out.AppliedMultiplier = "active_surface_source", "model_total", 1
	return out, nil
}

func epathSQLRadiantLoadDeclaration(load epathRealSQLLoad, originalText string) (map[string]epathSQLRadiantOwnerProof, error) {
	if load.NativeRadiant == nil || load.Component != "combined" || load.Source.IsMeter || load.Source.AllowAbsent || len(load.Source.Alternatives) != 1 {
		return nil, fmt.Errorf("radiant load requires explicit combined native identity and exact observed nonmeter J authority")
	}
	name := "Zone Radiant HVAC Cooling Energy"
	if load.Service == "heating" {
		name = "Zone Radiant HVAC Heating Energy"
	} else if load.Service != "cooling" {
		return nil, fmt.Errorf("unknown radiant load service")
	}
	if load.Source.Alternatives[0] != (epathRealSQLAlternative{Name: name, Unit: "J"}) {
		return nil, fmt.Errorf("radiant authority must use the independently reviewed exact surface-source Energy/J name")
	}
	owners, err := epathSQLRadiantOriginalOwners(originalText, *load.NativeRadiant)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, key := range load.Source.Keys {
		key = epathSQLRadiantName(key)
		if owners[key].Owner.EquipmentName == "" || seen[key] {
			return nil, fmt.Errorf("radiant selector has duplicate/foreign equipment keys")
		}
		seen[key] = true
	}
	if len(seen) != len(owners) {
		return nil, fmt.Errorf("radiant selector omits reviewed original equipment owners")
	}
	return owners, nil
}
