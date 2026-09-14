package simulation

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const energyPathBaseboardElectricType = "ZoneHVAC:Baseboard:RadiantConvective:Electric"
const energyPathBaseboardConvectiveElectricType = "ZoneHVAC:Baseboard:Convective:Electric"

func energyPathBaseboardElectricTypeSupported(objectType string) bool {
	return strings.EqualFold(strings.TrimSpace(objectType), energyPathBaseboardElectricType) ||
		strings.EqualFold(strings.TrimSpace(objectType), energyPathBaseboardConvectiveElectricType)
}

type energyPathBaseboardRecipient struct {
	SurfaceName        string
	SurfaceObjectIndex int
	ZoneName           string
	Fraction           float64
}

// The equipment response and electricity are already model-total observations.
// Recipients describe declared radiation destinations, never a passive/active
// split of the observed surface response or an additional delivered Zone load.
type energyPathBaseboardTarget struct {
	Component       idf.ComponentRef
	KeyValue        string
	ZoneName        string
	RadiantFraction float64
	PeopleFraction  float64
	Recipients      []energyPathBaseboardRecipient
	OriginalOutputs []energyPathBaseboardOriginalOutput
}

type energyPathBaseboardOriginalOutput struct {
	KeyValue    string
	Name        string
	Frequency   string
	ObjectIndex int
}

type energyPathBaseboardReference struct{ object, field int }

type energyPathBaseboardOriginalIndex struct {
	doc        idf.Document
	objects    map[string][]int
	references map[string][]energyPathBaseboardReference
}

func energyPathBaseboardField(object idf.Object, index int) string {
	if index < 0 || index >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[index].Value)
}

func newEnergyPathBaseboardOriginalIndex(doc idf.Document) energyPathBaseboardOriginalIndex {
	index := energyPathBaseboardOriginalIndex{doc: doc, objects: map[string][]int{}, references: map[string][]energyPathBaseboardReference{}}
	for position, object := range doc.Objects {
		key := energyPathDirectHVACComponentKey(object.Type, energyPathBaseboardField(object, 0))
		index.objects[key] = append(index.objects[key], position)
		for field := 0; field+1 < len(object.Fields); field++ {
			ref := energyPathDirectHVACComponentKey(energyPathBaseboardField(object, field), energyPathBaseboardField(object, field+1))
			index.references[ref] = append(index.references[ref], energyPathBaseboardReference{position, field})
		}
	}
	return index
}

func (index energyPathBaseboardOriginalIndex) unique(objectType, name string) (idf.Object, bool) {
	positions := index.objects[energyPathDirectHVACComponentKey(objectType, name)]
	if strings.TrimSpace(name) == "" || len(positions) != 1 {
		return idf.Object{}, false
	}
	return index.doc.Objects[positions[0]], true
}

func energyPathHasNativeBaseboard(doc idf.Document) bool {
	for _, object := range doc.Objects {
		if energyPathBaseboardElectricTypeSupported(object.Type) {
			return true
		}
	}
	return false
}

func energyPathBaseboardComponentRef(object idf.Object) idf.ComponentRef {
	name := energyPathBaseboardField(object, 0)
	return idf.ComponentRef{ID: fmt.Sprintf("component:%d", object.Index), ObjectType: object.Type, ObjectName: name, ObjectIndex: object.Index, DisplayName: name}
}

// AnalyzeHVAC supplies catalog-resolved EquipmentList field positions. Validate
// those positions against the entire original document: a selected Zone cannot
// hide a duplicate object, disconnected second owner, or contradictory wrapper.
// This does not broaden the separate PTAC/PTHP parent contract.
func energyPathBaseboardTargets(doc idf.Document) []energyPathBaseboardTarget {
	if !energyPathHasNativeBaseboard(doc) {
		return nil
	}
	index := newEnergyPathBaseboardOriginalIndex(doc)
	type owner struct {
		zone string
		item idf.HVACComponent
	}
	owners := map[string][]owner{}
	for _, relation := range idf.AnalyzeHVAC(doc).ZoneRelations {
		if relation.SpaceName != "" {
			continue
		}
		for _, item := range relation.ZoneEquipment {
			if energyPathBaseboardElectricTypeSupported(item.ObjectType) {
				key := energyPathDirectHVACComponentKey(item.ObjectType, item.ObjectName)
				owners[key] = append(owners[key], owner{relation.ZoneName, item})
			}
		}
	}
	nameCounts := map[string]int{}
	for _, object := range doc.Objects {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(object.Type)), "zonehvac:baseboard:") {
			// Other native baseboard types share the Total Heating reporting
			// key/name. Reject the whole target if that key is not unique.
			nameCounts[normalizePurposeToken(energyPathBaseboardField(object, 0))]++
		}
	}
	var out []energyPathBaseboardTarget
	for _, object := range doc.Objects {
		if !energyPathBaseboardElectricTypeSupported(object.Type) {
			continue
		}
		name := energyPathBaseboardField(object, 0)
		key := energyPathDirectHVACComponentKey(object.Type, name)
		if name == "" || len(index.objects[key]) != 1 || nameCounts[normalizePurposeToken(name)] != 1 || len(owners[key]) != 1 || len(index.references[key]) != 1 {
			continue
		}
		owned := owners[key][0]
		zone, zoneOK := index.unique("Zone", owned.zone)
		ref := index.references[key][0]
		list := doc.Objects[ref.object]
		listName := energyPathBaseboardField(list, 0)
		_, listOK := index.unique("ZoneHVAC:EquipmentList", listName)
		if !zoneOK || !listOK || !strings.EqualFold(list.Type, "ZoneHVAC:EquipmentList") || !owned.item.Exists ||
			owned.item.ObjectIndex != object.Index || owned.item.SourceOwnerObjectIndex != list.Index ||
			!strings.EqualFold(owned.item.SourceOwnerType, list.Type) || !strings.EqualFold(owned.item.SourceOwnerName, listName) ||
			owned.item.TypeFieldIndex != ref.field || owned.item.NameFieldIndex != ref.field+1 {
			continue
		}
		zoneName := energyPathBaseboardField(zone, 0)
		listConnections, zoneConnections := 0, 0
		valid := true
		for _, connection := range doc.Objects {
			if !strings.EqualFold(strings.TrimSpace(connection.Type), "ZoneHVAC:EquipmentConnections") {
				continue
			}
			connectionZone := energyPathBaseboardField(connection, 0)
			if strings.EqualFold(connectionZone, zoneName) {
				zoneConnections++
			}
			if strings.EqualFold(energyPathBaseboardField(connection, 1), listName) {
				listConnections++
				valid = valid && strings.EqualFold(connectionZone, zoneName)
			}
		}
		if !valid || listConnections != 1 || zoneConnections != 1 {
			continue
		}
		target := energyPathBaseboardTarget{Component: energyPathBaseboardComponentRef(object), KeyValue: name, ZoneName: zoneName}
		if !energyPathBaseboardResponse(index, object, &target) {
			continue
		}
		for _, output := range doc.Objects {
			if !strings.EqualFold(strings.TrimSpace(output.Type), "Output:Variable") {
				continue
			}
			keyValue := energyPathBaseboardField(output, 0)
			if keyValue == "*" || keyValue == "" || strings.EqualFold(keyValue, name) {
				target.OriginalOutputs = append(target.OriginalOutputs, energyPathBaseboardOriginalOutput{KeyValue: keyValue, Name: energyPathBaseboardField(output, 1), Frequency: canonicalPurposeFrequency(energyPathBaseboardField(output, 2)), ObjectIndex: output.Index})
			}
		}
		out = append(out, target)
	}
	sort.Slice(out, func(i, j int) bool {
		return normalizePurposeToken(out[i].KeyValue) < normalizePurposeToken(out[j].KeyValue)
	})
	return out
}

func energyPathBaseboardFraction(value string) (float64, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) && number >= 0 && number <= 1
}

func energyPathBaseboardResponse(index energyPathBaseboardOriginalIndex, object idf.Object, target *energyPathBaseboardTarget) bool {
	if strings.EqualFold(strings.TrimSpace(object.Type), energyPathBaseboardElectricType) {
		return energyPathBaseboardRecipients(index, object, target)
	}
	// This exact native type has seven schema positions, ending at efficiency6,
	// not seven mandatory serialized fields. EnergyPlus 25.1 IdfParser pads
	// min-fields and InputProcessor supplies omitted method/efficiency defaults.
	// The selected capacity is still explicitly required by BaseboardElectric's
	// numeric-blank checks. Never reinterpret another type or a radiant tail.
	if !strings.EqualFold(strings.TrimSpace(object.Type), energyPathBaseboardConvectiveElectricType) || len(object.Fields) < 4 || len(object.Fields) > 7 || !energyPathBaseboardConvectiveCapacityValid(object) {
		return false
	}
	efficiency, valid := energyPathBaseboardConvectiveNumber(firstNonEmpty(energyPathBaseboardField(object, 6), "1"))
	// Convective IDD permits zero, but the engine divides power by efficiency.
	// A zero-efficiency configuration cannot prove a finite consumption source.
	return valid && efficiency > 0 && efficiency <= 1 && target.RadiantFraction == 0 && target.PeopleFraction == 0 && len(target.Recipients) == 0
}

func energyPathBaseboardConvectiveCapacityValid(object idf.Object) bool {
	method := strings.ToLower(firstNonEmpty(energyPathBaseboardField(object, 2), "HeatingDesignCapacity"))
	field, strictlyPositive := 3, false
	switch method {
	case "heatingdesigncapacity":
		if strings.EqualFold(energyPathBaseboardField(object, 3), "Autosize") {
			return true
		}
	case "capacityperfloorarea":
		field, strictlyPositive = 4, true
	case "fractionofautosizedheatingcapacity":
		field = 5
	default:
		return false
	}
	// IDD defaults (Autosize and fraction1) do not erase NumBlank. Tagged
	// BaseboardElectric GetBaseboardInput rejects a blank selected numeric;
	// area capacity additionally rejects zero, whereas design/fraction allow it.
	value, valid := energyPathBaseboardConvectiveNumber(energyPathBaseboardField(object, field))
	return valid && value >= 0 && (!strictlyPositive || value > 0)
}

func energyPathBaseboardConvectiveNumber(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	// Native IDF real fields accept decimal/scientific numbers, not Go hex
	// literals or underscore separators accepted by strconv.ParseFloat.
	for _, character := range value {
		if !strings.ContainsRune("0123456789+-.eE", character) {
			return 0, false
		}
	}
	number, err := strconv.ParseFloat(value, 64)
	return number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func energyPathBaseboardRecipients(index energyPathBaseboardOriginalIndex, object idf.Object, target *energyPathBaseboardTarget) bool {
	// Native capacity-method schema: efficiency6, radiant7, people8, then
	// surface/fraction pairs. Do not interpret an older inline schema as this.
	if len(object.Fields) < 9 || (len(object.Fields)-9)%2 != 0 {
		return false
	}
	efficiency, efficient := energyPathBaseboardFraction(energyPathBaseboardField(object, 6))
	radiant, radiantOK := energyPathBaseboardFraction(energyPathBaseboardField(object, 7))
	people, peopleOK := energyPathBaseboardFraction(energyPathBaseboardField(object, 8))
	if !efficient || efficiency <= 0 || !radiantOK || !peopleOK {
		return false
	}
	target.RadiantFraction, target.PeopleFraction = radiant, people
	seen := map[string]bool{}
	sum := people
	for field := 9; field < len(object.Fields); field += 2 {
		name := energyPathBaseboardField(object, field)
		fraction, fractionOK := energyPathBaseboardFraction(energyPathBaseboardField(object, field+1))
		if name == "" && energyPathBaseboardField(object, field+1) == "" {
			continue
		}
		surface, surfaceOK := index.unique("BuildingSurface:Detailed", name)
		key := normalizePurposeToken(name)
		if !fractionOK || !surfaceOK || seen[key] || !strings.EqualFold(energyPathBaseboardField(surface, 3), target.ZoneName) {
			return false
		}
		seen[key] = true
		if spaceName := energyPathBaseboardField(surface, 4); spaceName != "" {
			space, ok := index.unique("Space", spaceName)
			if !ok || !strings.EqualFold(energyPathBaseboardField(space, 1), target.ZoneName) {
				return false
			}
		}
		sum += fraction
		if radiant > 0 && fraction > 0 {
			target.Recipients = append(target.Recipients, energyPathBaseboardRecipient{SurfaceName: energyPathBaseboardField(surface, 0), SurfaceObjectIndex: surface.Index, ZoneName: target.ZoneName, Fraction: fraction})
		}
	}
	return radiant == 0 || math.Abs(sum-1) <= 1e-9
}

func energyPathBaseboardElectricityDefinition() energyPathDirectHVACComponentDefinition {
	return energyPathDirectHVACComponentDefinition{ID: "heating.baseboard.electricity", ObjectType: energyPathBaseboardElectricType,
		Energy: energyMeterAliasDefinition{Kind: "energy.heating", Label: "Baseboard electricity", Carrier: "electricity", EndUse: "heating", HierarchyLevel: "zone_direct_use", Aliases: []string{"Baseboard Electricity Energy"}}}
}

func energyPathBaseboardElectricityDefinitionForType(objectType string) (energyPathDirectHVACComponentDefinition, bool) {
	if !energyPathBaseboardElectricTypeSupported(objectType) {
		return energyPathDirectHVACComponentDefinition{}, false
	}
	definition := energyPathBaseboardElectricityDefinition()
	if strings.EqualFold(strings.TrimSpace(objectType), energyPathBaseboardConvectiveElectricType) {
		definition.ObjectType = energyPathBaseboardConvectiveElectricType
	}
	return definition, true
}

func energyPathBaseboardElectricityDefinitions() []energyPathDirectHVACComponentDefinition {
	radiant := energyPathBaseboardElectricityDefinition()
	convective, _ := energyPathBaseboardElectricityDefinitionForType(energyPathBaseboardConvectiveElectricType)
	return []energyPathDirectHVACComponentDefinition{radiant, convective}
}

func energyPathBaseboardDirectTargets(targets []energyPathBaseboardTarget) []energyPathDirectHVACComponentTarget {
	out := make([]energyPathDirectHVACComponentTarget, 0, len(targets))
	for _, target := range targets {
		definition, supported := energyPathBaseboardElectricityDefinitionForType(target.Component.ObjectType)
		if supported {
			out = append(out, energyPathDirectHVACComponentTarget{Definition: definition, KeyValue: target.KeyValue, ZoneName: target.ZoneName})
		}
	}
	return out
}

func (builder *purposePlanBuilder) addEnergyPathBaseboardOutputs() {
	selected, scoped := purposeSelectedZoneSet(builder.request.Scope)
	for _, target := range energyPathBaseboardTargets(builder.doc) {
		if scoped && !selected[normalizePurposeToken(target.ZoneName)] {
			continue
		}
		for _, name := range []string{"Baseboard Electricity Energy", "Baseboard Electricity Rate", "Baseboard Total Heating Energy", "Baseboard Total Heating Rate"} {
			builder.addVariableWithReasonAndScopeZone(SimulationPurposeBasicEnergy, target.KeyValue, name, "Monthly", "medium",
				"Native exclusively Zone-owned electric baseboard source; Total Heating and rate companions are non-additive context, not additional Zone-air load or site energy.", "Basic Energy Path", target.ZoneName)
		}
	}
}
