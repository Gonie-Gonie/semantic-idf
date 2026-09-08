package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Native VRF is a reviewed shared outdoor system plus separately measured local
// terminal inputs. It is not a complete zone-direct PTAC/PTHP constituent cohort.
// This test-only declaration never contains a candidate value or SQL source ID.
type epathRealSQLVRFObject struct {
	ObjectType string `json:"objectType"`
	ObjectName string `json:"objectName"`
}

type epathRealSQLVRFTerminal struct {
	TerminalUnit epathRealSQLVRFObject `json:"terminalUnit"`
	ZoneName     string                `json:"zoneName"`
}

type epathRealSQLVRFSelectors struct {
	LocalCooling    epathRealSQLSelector `json:"localCooling"`
	LocalHeating    epathRealSQLSelector `json:"localHeating"`
	SharedCooling   epathRealSQLSelector `json:"sharedCooling"`
	SharedCrankcase epathRealSQLSelector `json:"sharedCrankcase"`
	SharedHeating   epathRealSQLSelector `json:"sharedHeating"`
	SharedDefrost   epathRealSQLSelector `json:"sharedDefrost"`
}

type epathRealSQLVRFSystem struct {
	OutdoorUnit      epathRealSQLVRFObject     `json:"outdoorUnit"`
	TerminalUnitList epathRealSQLVRFObject     `json:"terminalUnitList"`
	Terminals        []epathRealSQLVRFTerminal `json:"terminals"`
	Sources          epathRealSQLVRFSelectors  `json:"sources"`
	CoolingSiteID    string                    `json:"coolingSiteId"`
	HeatingSiteID    string                    `json:"heatingSiteId"`
	Frequency        string                    `json:"frequency"`
	AggregationBasis string                    `json:"aggregationBasis"`
	AllocationPolicy string                    `json:"allocationPolicy"`
}

type epathSQLVRFSourceFrame struct {
	Role, Service, ZoneName string
	Shared                  bool
	Source                  epathRealSQLSource
	Months                  [12]epathSQLQuantity
	RequestObjectIndex      *int
	EquipmentObjectIndex    int
}

type epathSQLVRFSystemFrame struct {
	Declaration epathRealSQLVRFSystem
	Sources     []epathSQLVRFSourceFrame
	Precision   epathRealSQLPrecision
}

type epathSQLVRFRole struct {
	ID, Name, Service string
	Shared            bool
	Selector          epathRealSQLSelector
}

// Literal original RDD/MTD taxonomy: crankcase belongs to cooling; defrost to
// heating. Terminal electricity is additive parasitic input, not outdoor input.
func epathSQLVRFRoles(system epathRealSQLVRFSystem) []epathSQLVRFRole {
	return []epathSQLVRFRole{
		{"local_cooling", "Zone VRF Air Terminal Cooling Electricity Energy", "cooling", false, system.Sources.LocalCooling},
		{"local_heating", "Zone VRF Air Terminal Heating Electricity Energy", "heating", false, system.Sources.LocalHeating},
		{"shared_cooling", "VRF Heat Pump Cooling Electricity Energy", "cooling", true, system.Sources.SharedCooling},
		{"shared_crankcase", "VRF Heat Pump Crankcase Heater Electricity Energy", "cooling", true, system.Sources.SharedCrankcase},
		{"shared_heating", "VRF Heat Pump Heating Electricity Energy", "heating", true, system.Sources.SharedHeating},
		{"shared_defrost", "VRF Heat Pump Defrost Electricity Energy", "heating", true, system.Sources.SharedDefrost},
	}
}

func epathSQLVRFName(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func epathSQLVRFDeclaration(system epathRealSQLVRFSystem) (map[string]string, error) {
	if system.OutdoorUnit.ObjectType != "AirConditioner:VariableRefrigerantFlow" || system.TerminalUnitList.ObjectType != "ZoneTerminalUnitList" ||
		epathSQLVRFName(system.OutdoorUnit.ObjectName) == "" || epathSQLVRFName(system.TerminalUnitList.ObjectName) == "" || len(system.Terminals) == 0 ||
		system.Frequency != "Monthly" || system.AggregationBasis != "model_total" || system.AllocationPolicy != "per_constituent_millikwh_largest_remainder_v1" ||
		system.CoolingSiteID == "" || system.HeatingSiteID == "" || system.CoolingSiteID == system.HeatingSiteID {
		return nil, fmt.Errorf("VRF requires exact native system identities and reviewed Monthly/model_total/integer-allocation policy")
	}
	owners, zones := map[string]string{}, map[string]bool{}
	for _, terminal := range system.Terminals {
		key, zone := epathSQLVRFName(terminal.TerminalUnit.ObjectName), epathSQLVRFName(terminal.ZoneName)
		if terminal.TerminalUnit.ObjectType != "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow" || key == "" || key == "*" || zone == "" || zone == "*" || owners[key] != "" || zones[zone] {
			return nil, fmt.Errorf("VRF requires unique explicit terminal-to-Zone owners")
		}
		owners[key], zones[zone] = terminal.ZoneName, true
	}
	for _, role := range epathSQLVRFRoles(system) {
		selector := role.Selector
		if selector.IsMeter || selector.AllowAbsent || len(selector.Alternatives) != 1 || selector.Alternatives[0] != (epathRealSQLAlternative{Name: role.Name, Unit: "J"}) {
			return nil, fmt.Errorf("VRF %s must declare its exact original nonmeter J source", role.ID)
		}
		keys := map[string]bool{}
		for _, key := range selector.Keys {
			key = epathSQLVRFName(key)
			if keys[key] || role.Shared && key != epathSQLVRFName(system.OutdoorUnit.ObjectName) || !role.Shared && owners[key] == "" {
				return nil, fmt.Errorf("VRF %s has duplicate, foreign or wildcard source keys", role.ID)
			}
			keys[key] = true
		}
		if role.Shared && len(keys) != 1 || !role.Shared && len(keys) != len(owners) {
			return nil, fmt.Errorf("VRF %s omits an original constituent owner", role.ID)
		}
	}
	return owners, nil
}

func epathSQLVRFSourceMonths(source epathRealSQLSource, precision epathRealSQLPrecision) ([12]epathSQLQuantity, error) {
	var out [12]epathSQLQuantity
	if source.DictionaryIndex <= 0 || source.ReportingFrequency != "Monthly" || source.SourceUnit != "J" || source.IsMeter || source.Rows != 12 || source.MissingRows != 0 || source.RawSum == nil || source.EnergyKWh == nil {
		return out, fmt.Errorf("VRF requires twelve observed native Monthly/J rows, including explicit zeros")
	}
	values, err := epathSQLMonthly(source, precision)
	if err != nil {
		return out, err
	}
	rawTotal, energyTotal := 0.0, 0.0
	for i, month := range source.Months {
		if month.MissingRows != 0 || month.RawSum == nil || !epathOracleFinite(*month.RawSum) || *month.RawSum < 0 || month.EnergyKWh == nil || !epathOracleFinite(*month.EnergyKWh) || *month.EnergyKWh < 0 {
			return out, fmt.Errorf("VRF has unknown, negative or nonfinite original monthly consumption")
		}
		want := *month.RawSum / 3600000
		ulp := math.Nextafter(want, math.Inf(1)) - want
		if math.Abs(want-*month.EnergyKWh) > 2*ulp || want == 0 && *month.EnergyKWh != 0 || want != 0 && *month.EnergyKWh == 0 {
			return out, fmt.Errorf("VRF normalized observation does not match its original J row")
		}
		out[i] = values[i]
		rawTotal += *month.RawSum
		energyTotal += *month.EnergyKWh
	}
	// These totals were accumulated from the same ordered twelve SQL rows;
	// equality is exact and is unrelated to display precision.
	if rawTotal != *source.RawSum || energyTotal != *source.EnergyKWh || !epathOracleFinite(rawTotal) || !epathOracleFinite(energyTotal) {
		return out, fmt.Errorf("VRF annual source is not the sum of its original twelve months")
	}
	return out, nil
}

func epathSQLValidateVRFSystemFrame(frame epathSQLVRFSystemFrame) error {
	owners, err := epathSQLVRFDeclaration(frame.Declaration)
	if err != nil {
		return err
	}
	roles := map[string]epathSQLVRFRole{}
	for _, role := range epathSQLVRFRoles(frame.Declaration) {
		roles[role.ID] = role
	}
	seen, ids := map[string]bool{}, map[int]bool{}
	for _, source := range frame.Sources {
		role, exists := roles[source.Role]
		key := epathSQLVRFName(source.Source.KeyValue)
		identity := source.Role + "|" + key
		zone := owners[key]
		if role.Shared {
			zone = ""
		}
		if !exists || seen[identity] || ids[source.Source.DictionaryIndex] || source.Shared != role.Shared || source.Service != role.Service ||
			!strings.EqualFold(source.Source.Name, role.Name) || !strings.EqualFold(source.ZoneName, zone) ||
			role.Shared && key != epathSQLVRFName(frame.Declaration.OutdoorUnit.ObjectName) || !role.Shared && zone == "" ||
			source.EquipmentObjectIndex < 0 || source.RequestObjectIndex != nil && *source.RequestObjectIndex < 0 {
			return fmt.Errorf("VRF source frame contradicts its exact original role/owner/source identity")
		}
		months, err := epathSQLVRFSourceMonths(source.Source, frame.Precision)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(months, source.Months) {
			return fmt.Errorf("VRF frame changed native source quantity or its existing precision budget")
		}
		seen[identity], ids[source.Source.DictionaryIndex] = true, true
	}
	if len(seen) != 2*len(owners)+4 {
		return fmt.Errorf("VRF frame lacks a local, main, crankcase or defrost constituent")
	}
	return nil
}

// Only this exact request supplies the Output jump. A physical object index or
// an original Timestep request cannot fill an unindexed generated Monthly one.
func epathSQLVRFRequestIndex(plan *PurposeRunPlan, name, key, zone string) (*int, error) {
	if plan == nil {
		return nil, fmt.Errorf("VRF requires the captured output plan")
	}
	count := 0
	var index *int
	for _, output := range plan.OutputObjects {
		if !strings.EqualFold(output.VariableName, name) || !strings.EqualFold(output.KeyValue, key) || output.ReportingFrequency != "Monthly" {
			continue
		}
		basic := false
		for _, purpose := range output.PurposeIDs {
			basic = basic || purpose == SimulationPurposeBasicEnergy
		}
		if output.ObjectType != "Output:Variable" || !basic || !strings.EqualFold(output.ScopeZoneName, zone) || output.ObjectIndex != nil && *output.ObjectIndex < 0 {
			return nil, fmt.Errorf("VRF source has a wrong exact request type/purpose/scope/index")
		}
		count++
		if output.ObjectIndex != nil {
			value := *output.ObjectIndex
			index = &value
		}
	}
	if count != 1 {
		return nil, fmt.Errorf("VRF source needs exactly one original Monthly request: %s/%s", name, key)
	}
	return index, nil
}

func epathSQLVRFField(object idf.Object, index int) string {
	if index >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[index].Value)
}

// Independent original IDF walk. No production service model, target catalog,
// AnalyzeHVAC, output builder or candidate graph participates in this proof.
func epathSQLVRFOriginalOwners(text string, declarations []epathRealSQLVRFSystem) (map[string]int, error) {
	doc, err := idf.Parse(text)
	if err != nil {
		return nil, err
	}
	objectKey := func(kind, name string) string { return epathSQLVRFName(kind) + "|" + epathSQLVRFName(name) }
	objects := map[string][]idf.Object{}
	for _, object := range doc.Objects {
		key := objectKey(object.Type, epathSQLVRFField(object, 0))
		objects[key] = append(objects[key], object)
	}
	one := func(kind, name string) (idf.Object, error) {
		matches := objects[objectKey(kind, name)]
		if name == "" || len(matches) != 1 {
			return idf.Object{}, fmt.Errorf("VRF needs one original typed object %s/%s", kind, name)
		}
		return matches[0], nil
	}
	indices, systems, terminalNames := map[string]int{}, map[string]bool{}, map[string]bool{}
	for _, declaration := range declarations {
		owners, err := epathSQLVRFDeclaration(declaration)
		if err != nil {
			return nil, err
		}
		outdoor, err := one(declaration.OutdoorUnit.ObjectType, declaration.OutdoorUnit.ObjectName)
		if err != nil {
			return nil, err
		}
		outdoorName := epathSQLVRFName(declaration.OutdoorUnit.ObjectName)
		if systems[outdoorName] {
			return nil, fmt.Errorf("duplicate reviewed VRF outdoor system")
		}
		systems[outdoorName] = true
		if !strings.EqualFold(epathSQLVRFField(outdoor, 36), declaration.TerminalUnitList.ObjectName) || !strings.EqualFold(epathSQLVRFField(outdoor, 37), "No") ||
			!strings.EqualFold(epathSQLVRFField(outdoor, 49), "Resistive") || !strings.EqualFold(epathSQLVRFField(outdoor, 55), "AirCooled") || !strings.EqualFold(epathSQLVRFField(outdoor, 66), "Electricity") {
			return nil, fmt.Errorf("unreviewed VRF outdoor fuel/defrost/condenser/heat-recovery/list physics")
		}
		list, err := one(declaration.TerminalUnitList.ObjectType, declaration.TerminalUnitList.ObjectName)
		if err != nil {
			return nil, err
		}
		members := map[string]bool{}
		for i := 1; i < len(list.Fields); i++ {
			key := epathSQLVRFName(epathSQLVRFField(list, i))
			if members[key] || owners[key] == "" {
				return nil, fmt.Errorf("original VRF list has duplicate/foreign/unreviewed terminal")
			}
			members[key] = true
		}
		if len(members) != len(owners) {
			return nil, fmt.Errorf("reviewed VRF owner roster is not its complete original list")
		}
		indices[objectKey(outdoor.Type, epathSQLVRFField(outdoor, 0))] = outdoor.Index
		for _, terminal := range declaration.Terminals {
			name := epathSQLVRFName(terminal.TerminalUnit.ObjectName)
			if terminalNames[name] {
				return nil, fmt.Errorf("terminal shared between reviewed outdoor systems")
			}
			terminalNames[name] = true
			tu, err := one(terminal.TerminalUnit.ObjectType, terminal.TerminalUnit.ObjectName)
			if err != nil {
				return nil, err
			}
			if _, err := one("Zone", terminal.ZoneName); err != nil {
				return nil, err
			}
			indices[objectKey(tu.Type, epathSQLVRFField(tu, 0))] = tu.Index
			if !strings.EqualFold(epathSQLVRFField(tu, 12), "DrawThrough") || epathSQLVRFField(tu, 15) != "" || epathSQLVRFField(tu, 16) != "" || epathSQLVRFField(tu, 26) != "" || epathSQLVRFField(tu, 27) != "" {
				return nil, fmt.Errorf("unreviewed VRF terminal fan/mixer/supplemental-coil physics")
			}
			for _, index := range []int{8, 9, 10, 21, 22} {
				value, err := strconv.ParseFloat(epathSQLVRFField(tu, index), 64)
				if err != nil || !epathOracleFinite(value) || value < 0 || index <= 10 && value != 0 {
					return nil, fmt.Errorf("VRF terminal has unreviewed outdoor/parasitic input")
				}
			}
			parts := []idf.Object{}
			for _, part := range []struct {
				position int
				kind     string
			}{{17, "Coil:Cooling:DX:VariableRefrigerantFlow"}, {19, "Coil:Heating:DX:VariableRefrigerantFlow"}, {13, "Fan:ConstantVolume"}} {
				if !strings.EqualFold(epathSQLVRFField(tu, part.position), part.kind) {
					return nil, fmt.Errorf("VRF terminal contains a different typed constituent")
				}
				component, err := one(part.kind, epathSQLVRFField(tu, part.position+1))
				if err != nil {
					return nil, err
				}
				refs := 0
				for _, object := range doc.Objects {
					for i := 0; i+1 < len(object.Fields); i++ {
						if objectKey(epathSQLVRFField(object, i), epathSQLVRFField(object, i+1)) == objectKey(component.Type, epathSQLVRFField(component, 0)) {
							refs++
							if object.Index != tu.Index || i != part.position {
								return nil, fmt.Errorf("VRF coil/fan has shared or foreign typed ownership")
							}
						}
					}
				}
				if refs != 1 {
					return nil, fmt.Errorf("VRF component lacks unique original parent")
				}
				parts = append(parts, component)
			}
			for _, pair := range [][2]string{{epathSQLVRFField(tu, 2), epathSQLVRFField(parts[0], 7)}, {epathSQLVRFField(parts[0], 8), epathSQLVRFField(parts[1], 4)}, {epathSQLVRFField(parts[1], 5), epathSQLVRFField(parts[2], 7)}, {epathSQLVRFField(parts[2], 8), epathSQLVRFField(tu, 3)}} {
				if pair[0] == "" || !strings.EqualFold(pair[0], pair[1]) {
					return nil, fmt.Errorf("VRF terminal component air chain is disconnected")
				}
			}
			var equipmentLists, mixers []idf.Object
			for _, object := range doc.Objects {
				for i := 0; i+1 < len(object.Fields); i++ {
					if objectKey(epathSQLVRFField(object, i), epathSQLVRFField(object, i+1)) != objectKey(tu.Type, epathSQLVRFField(tu, 0)) {
						continue
					}
					if strings.EqualFold(object.Type, "ZoneHVAC:EquipmentList") && i >= 2 && (i-2)%6 == 0 {
						equipmentLists = append(equipmentLists, object)
					} else if strings.EqualFold(object.Type, "AirTerminal:SingleDuct:Mixer") && i == 1 {
						mixers = append(mixers, object)
					} else {
						return nil, fmt.Errorf("VRF terminal has an unsupported/shared typed reference")
					}
				}
			}
			if len(equipmentLists) != 1 || len(mixers) != 1 {
				return nil, fmt.Errorf("VRF terminal needs its exact original EquipmentList and DOAS mixer")
			}
			if _, err := one(equipmentLists[0].Type, epathSQLVRFField(equipmentLists[0], 0)); err != nil {
				return nil, err
			}
			if _, err := one(mixers[0].Type, epathSQLVRFField(mixers[0], 0)); err != nil {
				return nil, err
			}
			connections, zoneConnections := []idf.Object{}, 0
			for _, object := range doc.Objects {
				if strings.EqualFold(object.Type, "ZoneHVAC:EquipmentConnections") {
					if strings.EqualFold(epathSQLVRFField(object, 0), terminal.ZoneName) {
						zoneConnections++
					}
					if strings.EqualFold(epathSQLVRFField(object, 1), epathSQLVRFField(equipmentLists[0], 0)) {
						connections = append(connections, object)
					}
				}
			}
			if zoneConnections != 1 || len(connections) != 1 || !strings.EqualFold(epathSQLVRFField(connections[0], 0), terminal.ZoneName) {
				return nil, fmt.Errorf("VRF equipment list has missing/foreign/duplicate Zone ownership")
			}
			mixer := mixers[0]
			inlet := strings.EqualFold(epathSQLVRFField(mixer, 6), "InletSide") && strings.EqualFold(epathSQLVRFField(mixer, 3), epathSQLVRFField(tu, 2))
			supply := strings.EqualFold(epathSQLVRFField(mixer, 6), "SupplySide") && strings.EqualFold(epathSQLVRFField(mixer, 5), epathSQLVRFField(tu, 3))
			if !inlet && !supply {
				return nil, fmt.Errorf("VRF original mixer is not connected to its declared terminal")
			}
		}
	}
	// A declaration may not hide another native/unsupported outdoor system or
	// a second list reference merely by omitting it from selected scope.
	listOwners, terminalLists := map[string]int{}, map[string]int{}
	for _, object := range doc.Objects {
		if strings.HasPrefix(epathSQLVRFName(object.Type), "zonehvac:terminalunit:variablerefrigerantflow") &&
			(!strings.EqualFold(object.Type, "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow") || !terminalNames[epathSQLVRFName(epathSQLVRFField(object, 0))]) {
			return nil, fmt.Errorf("unreviewed or unsupported original VRF terminal")
		}
		if strings.HasPrefix(epathSQLVRFName(object.Type), "airconditioner:variablerefrigerantflow") {
			if !strings.EqualFold(object.Type, "AirConditioner:VariableRefrigerantFlow") || !systems[epathSQLVRFName(epathSQLVRFField(object, 0))] {
				return nil, fmt.Errorf("unreviewed original VRF outdoor system")
			}
			listOwners[epathSQLVRFName(epathSQLVRFField(object, 36))]++
		}
		if strings.EqualFold(object.Type, "ZoneTerminalUnitList") {
			for i := 1; i < len(object.Fields); i++ {
				terminalLists[epathSQLVRFName(epathSQLVRFField(object, i))]++
			}
		}
	}
	for _, declaration := range declarations {
		if listOwners[epathSQLVRFName(declaration.TerminalUnitList.ObjectName)] != 1 {
			return nil, fmt.Errorf("VRF list is shared or unowned")
		}
		for _, terminal := range declaration.Terminals {
			if terminalLists[epathSQLVRFName(terminal.TerminalUnit.ObjectName)] != 1 {
				return nil, fmt.Errorf("VRF terminal occurs in multiple original lists")
			}
		}
	}
	return indices, nil
}

func epathCompileSQLVRFSystems(sqlPath, originalText string, plan *PurposeRunPlan, observed []epathRealSQLSource, model epathRealSQLModel, frames epathSQLFrames) ([]epathSQLVRFSystemFrame, error) {
	if len(model.NativeVRFSystems) == 0 {
		return nil, nil
	}
	indices, err := epathSQLVRFOriginalOwners(originalText, model.NativeVRFSystems)
	if err != nil {
		return nil, err
	}
	out := []epathSQLVRFSystemFrame{}
	allSources := map[int]epathSQLVRFSourceFrame{}
	for _, declaration := range model.NativeVRFSystems {
		frame := epathSQLVRFSystemFrame{Declaration: declaration, Precision: model.Precision}
		frame.Declaration.Terminals = append([]epathRealSQLVRFTerminal(nil), declaration.Terminals...)
		owners, _ := epathSQLVRFDeclaration(declaration)
		for key, zone := range owners {
			actual, exists := frames.Zones[epathSQLVRFName(zone)]
			if !exists || !strings.EqualFold(actual.Name, zone) || !epathOracleFinite(actual.Multiplier) || actual.Multiplier <= 0 {
				return nil, fmt.Errorf("VRF requires independently observed SQL Zone ownership/multiplier")
			}
			owners[key] = actual.Name
		}
		for index := range frame.Declaration.Terminals {
			terminal := &frame.Declaration.Terminals[index]
			terminal.ZoneName = owners[epathSQLVRFName(terminal.TerminalUnit.ObjectName)]
		}
		for _, pool := range []struct{ id, service string }{{declaration.CoolingSiteID, "cooling"}, {declaration.HeatingSiteID, "heating"}} {
			count := 0
			for _, site := range model.Site {
				if site.ID == pool.id {
					count++
					if site.Facility || site.Carrier != "electricity" || site.EndUse != pool.service || site.Tabular != nil || !site.Source.IsMeter {
						return nil, fmt.Errorf("VRF has an incompatible broad site declaration")
					}
				}
			}
			if count != 1 || len(frames.SiteSources[pool.id]) != 1 {
				return nil, fmt.Errorf("VRF requires one original broad service meter")
			}
			annual, err := epathSQLSiteIsAnnual(frames, pool.id)
			if err != nil || annual {
				return nil, fmt.Errorf("VRF broad site must be independently observed Monthly")
			}
			if _, err := epathSQLSitePeriod(frames, pool.id, "annual"); err != nil {
				return nil, err
			}
		}
		for _, role := range epathSQLVRFRoles(declaration) {
			sources, err := epathSQLSelect(observed, role.Selector)
			if err != nil {
				return nil, err
			}
			for _, source := range sources {
				zone, kind := owners[epathSQLVRFName(source.KeyValue)], "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow"
				if role.Shared {
					zone, kind = "", "AirConditioner:VariableRefrigerantFlow"
				}
				request, err := epathSQLVRFRequestIndex(plan, role.Name, source.KeyValue, zone)
				if err != nil {
					return nil, err
				}
				months, err := epathSQLVRFSourceMonths(source, model.Precision)
				if err != nil {
					return nil, err
				}
				index, exists := indices[epathSQLVRFName(kind)+"|"+epathSQLVRFName(source.KeyValue)]
				if !exists || allSources[source.DictionaryIndex].Source.DictionaryIndex != 0 || frames.SourceIdentities[source.DictionaryIndex].DictionaryIndex != 0 {
					return nil, fmt.Errorf("VRF source has missing physical ownership or overlaps existing numeric authority")
				}
				item := epathSQLVRFSourceFrame{Role: role.ID, Service: role.Service, ZoneName: zone, Shared: role.Shared, Source: source, Months: months, RequestObjectIndex: request, EquipmentObjectIndex: index}
				frame.Sources = append(frame.Sources, item)
				allSources[source.DictionaryIndex] = item
			}
		}
		if err := epathSQLValidateVRFSystemFrame(frame); err != nil {
			return nil, err
		}
		sort.Slice(frame.Sources, func(i, j int) bool {
			return frame.Sources[i].Source.DictionaryIndex < frame.Sources[j].Source.DictionaryIndex
		})
		out = append(out, frame)
	}
	if err := epathSQLVRFAuditOriginalSQL(sqlPath, allSources); err != nil {
		return nil, err
	}
	return out, nil
}

func epathSQLVRFAuditOriginalSQL(path string, selected map[int]epathSQLVRFSourceFrame) error {
	db, err := epathOpenOracleSQL(path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := epathReadOracleWeather(db); err != nil {
		return err
	}
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,Name,KeyValue,Units,IsMeter,ReportingFrequency FROM ReportDataDictionary WHERE Name COLLATE NOCASE IN (?,?,?,?,?,?)`,
		"Zone VRF Air Terminal Cooling Electricity Energy", "Zone VRF Air Terminal Heating Electricity Energy", "VRF Heat Pump Cooling Electricity Energy", "VRF Heat Pump Crankcase Heater Electricity Energy", "VRF Heat Pump Heating Electricity Energy", "VRF Heat Pump Defrost Electricity Energy")
	if err != nil {
		return err
	}
	seen := map[int]bool{}
	for rows.Next() {
		var id, meter int
		var name, key, unit, frequency string
		if err := rows.Scan(&id, &name, &key, &unit, &meter, &frequency); err != nil {
			rows.Close()
			return err
		}
		if frequency != "Monthly" {
			continue
		}
		original, exists := selected[id]
		if !exists || seen[id] || meter != 0 || unit != "J" || !strings.EqualFold(name, original.Source.Name) || !strings.EqualFold(key, original.Source.KeyValue) {
			rows.Close()
			return fmt.Errorf("VRF dictionary contains duplicate, unobserved, foreign or mismatched original identity")
		}
		seen[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(seen) != len(selected) {
		return fmt.Errorf("VRF selected source lost its original SQL dictionary")
	}
	for id, source := range selected {
		rows, err := db.Query(`SELECT t.TimeIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,t.SimulationDays,t.EnvironmentPeriodIndex,t.WarmupFlag,e.EnvironmentType,r.Value
FROM ReportData r LEFT JOIN "Time" t ON t.TimeIndex=r.TimeIndex LEFT JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
WHERE r.ReportDataDictionaryIndex=? ORDER BY t.TimeIndex`, id)
		if err != nil {
			return err
		}
		err = func() error {
			defer rows.Close()
			seenMonths := map[int]bool{}
			environment := int64(0)
			for rows.Next() {
				var timeID, year, month, day, hour, minute, intervalType, days, env, warmup, envType sql.NullInt64
				var interval, value sql.NullFloat64
				if err := rows.Scan(&timeID, &year, &month, &day, &hour, &minute, &interval, &intervalType, &days, &env, &warmup, &envType, &value); err != nil {
					return err
				}
				if envType.Valid && envType.Int64 != 3 || warmup.Valid && warmup.Int64 != 0 {
					continue
				}
				if !timeID.Valid || timeID.Int64 <= 0 || !year.Valid || year.Int64 != 2017 || !month.Valid || month.Int64 < 1 || month.Int64 > 12 || !env.Valid || env.Int64 <= 0 || !envType.Valid || envType.Int64 != 3 || !intervalType.Valid || intervalType.Int64 != 3 {
					return fmt.Errorf("VRF source has an invalid original weather Monthly axis")
				}
				last := time.Date(2017, time.Month(month.Int64)+1, 0, 0, 0, 0, 0, time.UTC)
				if seenMonths[int(month.Int64)] || !day.Valid || day.Int64 != int64(last.Day()) || !hour.Valid || hour.Int64 != 24 || !minute.Valid || minute.Int64 != 0 || !interval.Valid || interval.Float64 != float64(last.Day()*1440) || !days.Valid || days.Int64 != int64(last.YearDay()) || environment != 0 && environment != env.Int64 {
					return fmt.Errorf("VRF requires twelve complete original calendar intervals, not partial/duplicate months")
				}
				if !value.Valid || !epathOracleFinite(value.Float64) || value.Float64 < 0 {
					return fmt.Errorf("VRF source has unknown/negative actual SQL value")
				}
				original := source.Source.Months[int(month.Int64)-1]
				if original.RawSum == nil || value.Float64 != *original.RawSum {
					return fmt.Errorf("VRF observed source does not equal the exact original SQL row")
				}
				environment = env.Int64
				seenMonths[int(month.Int64)] = true
			}
			if err := rows.Err(); err != nil {
				return err
			}
			if len(seenMonths) != 12 {
				return fmt.Errorf("VRF source lacks full twelve-month original coverage")
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func epathSQLMatchVRFSource(actual EnergyDataSource, original epathSQLVRFSourceFrame) error {
	if actual.ID != fmt.Sprintf("sql-rdd-%d", original.Source.DictionaryIndex) || actual.SourceType != "sql_report_data" || actual.IsMeter || !strings.EqualFold(actual.Name, original.Source.Name) || !strings.EqualFold(actual.KeyValue, original.Source.KeyValue) || !strings.EqualFold(actual.ZoneName, original.ZoneName) || actual.SourceUnit != "J" || actual.Units != "J" || actual.NormalizedUnit != "kWh" || actual.ReportingFrequency != "Monthly" || actual.AggregationMethod != "sum" || actual.AggregationBasis != "model_total" || actual.MultiplierApplication != "already_model_total" || actual.EffectiveMultiplier != 1 || len(actual.InputSourceIDs) != 0 || !reflect.DeepEqual(actual.ObjectIndex, original.RequestObjectIndex) {
		return fmt.Errorf("VRF source contradicts original variable/owner/model-total/request-index provenance")
	}
	return nil
}
