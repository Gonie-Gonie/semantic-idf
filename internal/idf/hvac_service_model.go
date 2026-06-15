package idf

import (
	"fmt"
	"sort"
	"strings"
)

type HVACServiceModel struct {
	ZoneServices []ZoneServiceSummary `json:"zoneServices"`
	Systems      []SystemSummary      `json:"systems"`
	Couplings    []SystemCoupling     `json:"couplings"`
	Networks     []EnergyNetwork      `json:"networks"`
}

type ZoneServiceSummary struct {
	ID            string            `json:"id"`
	ZoneName      string            `json:"zoneName"`
	SpaceName     string            `json:"spaceName,omitempty"`
	ServedSubject ServedSubjectRef  `json:"servedSubject"`
	Paths         []ZoneServicePath `json:"paths"`
	Issues        []HVACWarning     `json:"issues,omitempty"`
}

type ZoneServicePath struct {
	ID                  string                `json:"id"`
	ZoneName            string                `json:"zoneName"`
	SpaceName           string                `json:"spaceName,omitempty"`
	ServiceKind         string                `json:"serviceKind"`
	PathType            string                `json:"pathType"`
	SourceSystem        *SystemRef            `json:"sourceSystem,omitempty"`
	PlantLoop           *LoopRef              `json:"plantLoop,omitempty"`
	CondenserLoop       *LoopRef              `json:"condenserLoop,omitempty"`
	AirLoop             *LoopRef              `json:"airLoop,omitempty"`
	RefrigerantSystem   *SystemRef            `json:"refrigerantSystem,omitempty"`
	Conditioning        []ComponentRef        `json:"conditioning,omitempty"`
	Delivery            ComponentRef          `json:"delivery"`
	DeliveryEquipment   HVACDeliveryEquipment `json:"deliveryEquipment"`
	DeliveryWrapper     *ComponentRef         `json:"deliveryWrapper,omitempty"`
	ServedSubject       ServedSubjectRef      `json:"servedSubject"`
	SupportingCouplings []string              `json:"supportingCouplingIds,omitempty"`
	TraceIDs            []string              `json:"traceIds,omitempty"`
	Issues              []HVACWarning         `json:"issues,omitempty"`
}

type ComponentRef struct {
	ID                   string   `json:"id"`
	ObjectType           string   `json:"objectType"`
	ObjectName           string   `json:"objectName,omitempty"`
	ObjectIndex          int      `json:"objectIndex,omitempty"`
	DisplayName          string   `json:"displayName,omitempty"`
	Family               string   `json:"family,omitempty"`
	DisplayFamily        string   `json:"displayFamily,omitempty"`
	DeliveryType         string   `json:"deliveryType,omitempty"`
	CouplingType         string   `json:"couplingType,omitempty"`
	Role                 string   `json:"role,omitempty"`
	Mediums              []string `json:"mediums,omitempty"`
	InletNode            string   `json:"inletNode,omitempty"`
	OutletNode           string   `json:"outletNode,omitempty"`
	WaterInletNode       string   `json:"waterInletNode,omitempty"`
	WaterOutletNode      string   `json:"waterOutletNode,omitempty"`
	ResolvedFromADU      bool     `json:"resolvedFromAirDistributionUnit,omitempty"`
	DistributionUnitName string   `json:"distributionUnitName,omitempty"`
}

type LoopRef struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	ObjectIndex int      `json:"objectIndex,omitempty"`
	Mediums     []string `json:"mediums,omitempty"`
}

type SystemRef struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	ObjectType  string   `json:"objectType,omitempty"`
	ObjectName  string   `json:"objectName,omitempty"`
	ObjectIndex int      `json:"objectIndex,omitempty"`
	DisplayName string   `json:"displayName,omitempty"`
	Mediums     []string `json:"mediums,omitempty"`
}

type ServedSubjectRef struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ZoneName    string `json:"zoneName,omitempty"`
	SpaceName   string `json:"spaceName,omitempty"`
	ObjectIndex int    `json:"objectIndex,omitempty"`
}

type HVACDeliveryEquipment struct {
	Component        ComponentRef `json:"component"`
	DeliveryType     string       `json:"deliveryType"`
	DisplayFamily    string       `json:"displayFamily"`
	RequiresAirLoop  bool         `json:"requiresAirLoop"`
	CanUsePlantLoop  bool         `json:"canUsePlantLoop"`
	HasInternalCoils bool         `json:"hasInternalCoils"`
	Mediums          []string     `json:"mediums,omitempty"`
}

type SystemCoupling struct {
	ID                  string             `json:"id"`
	CouplingType        string             `json:"couplingType"`
	Object              ComponentRef       `json:"object"`
	Role                string             `json:"role"`
	ConnectedLoops      []LoopRef          `json:"connectedLoops,omitempty"`
	ConnectedSystems    []SystemRef        `json:"connectedSystems,omitempty"`
	ConnectedComponents []ComponentRef     `json:"connectedComponents,omitempty"`
	ConnectedZones      []ServedSubjectRef `json:"connectedZones,omitempty"`
	Mediums             []string           `json:"mediums,omitempty"`
	PlacementHint       string             `json:"placementHint,omitempty"`
	TraceIDs            []string           `json:"traceIds,omitempty"`
}

type EnergyNetwork struct {
	ID          string         `json:"id"`
	NetworkType string         `json:"networkType"`
	Name        string         `json:"name"`
	Mediums     []string       `json:"mediums,omitempty"`
	Components  []ComponentRef `json:"components,omitempty"`
	CouplingIDs []string       `json:"couplingIds,omitempty"`
}

type SystemSummary struct {
	ID                  string         `json:"id"`
	Type                string         `json:"type"`
	Name                string         `json:"name"`
	ObjectType          string         `json:"objectType,omitempty"`
	ObjectIndex         int            `json:"objectIndex,omitempty"`
	Role                string         `json:"role,omitempty"`
	Mediums             []string       `json:"mediums,omitempty"`
	ConnectedLoops      []LoopRef      `json:"connectedLoops,omitempty"`
	ConnectedComponents []ComponentRef `json:"connectedComponents,omitempty"`
}

type ComponentIndexItem struct {
	Component     ComponentRef          `json:"component"`
	Family        string                `json:"family"`
	DisplayFamily string                `json:"displayFamily"`
	DeliveryType  string                `json:"deliveryType"`
	CouplingType  string                `json:"couplingType"`
	Mediums       []string              `json:"mediums"`
	Occurrences   []ComponentOccurrence `json:"occurrences"`
	InternalRefs  []ComponentRef        `json:"internalRefs"`
}

type ComponentOccurrence struct {
	ContextType string `json:"contextType"`
	LoopName    string `json:"loopName,omitempty"`
	LoopType    string `json:"loopType,omitempty"`
	LoopSide    string `json:"loopSide,omitempty"`
	BranchName  string `json:"branchName,omitempty"`
	ZoneName    string `json:"zoneName,omitempty"`
	SpaceName   string `json:"spaceName,omitempty"`
	RoleHere    string `json:"roleHere,omitempty"`
}

type ComponentIndex struct {
	ByKey map[string]ComponentIndexItem
}

type CouplingIndex struct {
	ByLoop      map[string][]SystemCoupling
	ByComponent map[string][]SystemCoupling
	ByNetwork   map[string][]SystemCoupling
	ByZone      map[string][]SystemCoupling
}

func buildHVACServiceModel(ctx *hvacContext, loops []HVACLoop, relations []HVACZoneChain, graph HVACRuleGraph) HVACServiceModel {
	componentIndex := buildHVACComponentIndex(ctx, loops, relations)
	couplings := buildHVACSystemCouplings(ctx, loops, componentIndex)
	couplingIndex := buildHVACCouplingIndex(couplings)
	paths := buildZoneServicePaths(ctx, loops, relations, graph, componentIndex, couplingIndex)
	return HVACServiceModel{
		ZoneServices: buildZoneServiceSummaries(relations, paths),
		Systems:      buildHVACSystemSummaries(loops),
		Couplings:    couplings,
		Networks:     buildEnergyNetworks(ctx, couplings),
	}
}

func buildHVACComponentIndex(ctx *hvacContext, loops []HVACLoop, relations []HVACZoneChain) ComponentIndex {
	index := ComponentIndex{ByKey: map[string]ComponentIndexItem{}}
	add := func(component HVACComponent, occurrence ComponentOccurrence) {
		key := hvacComponentKey(component)
		if key == "" {
			return
		}
		item := index.ByKey[key]
		if item.Component.ID == "" {
			ref := componentRefFromHVACComponent(component)
			item.Component = ref
			item.Family = ref.Family
			item.DisplayFamily = ref.DisplayFamily
			item.DeliveryType = hvacDeliveryTypeForObject(component.ObjectType)
			couplingType, _ := hvacCouplingTypeAndRoleForObject(component.ObjectType)
			item.CouplingType = couplingType
			item.Mediums = hvacMediumsForComponent(component.ObjectType)
			for _, internal := range internalComponentRefs(ctx, component) {
				item.InternalRefs = appendUniqueComponentRef(item.InternalRefs, internal)
			}
		}
		item.Occurrences = append(item.Occurrences, occurrence)
		index.ByKey[key] = item
	}
	for _, loop := range loops {
		for _, side := range []HVACLoopSide{loop.SupplySide, loop.DemandSide} {
			for _, branch := range side.Branches {
				for _, component := range branch.Components {
					add(component, ComponentOccurrence{
						ContextType: "loop_branch",
						LoopName:    loop.Name,
						LoopType:    loop.Type,
						LoopSide:    side.Name,
						BranchName:  branch.Name,
						RoleHere:    component.RoleHere,
					})
				}
			}
		}
	}
	for _, relation := range relations {
		for _, component := range relation.ZoneEquipment {
			add(component, ComponentOccurrence{
				ContextType: "zone_equipment",
				ZoneName:    relation.ZoneName,
				SpaceName:   relation.SpaceName,
				RoleHere:    component.RoleHere,
			})
		}
		for _, component := range relation.TerminalUnits {
			add(component, ComponentOccurrence{
				ContextType: "zone_terminal",
				ZoneName:    relation.ZoneName,
				SpaceName:   relation.SpaceName,
				RoleHere:    component.RoleHere,
			})
		}
	}
	return index
}

func buildZoneServiceSummaries(relations []HVACZoneChain, paths []ZoneServicePath) []ZoneServiceSummary {
	pathsBySubject := map[string][]ZoneServicePath{}
	for _, path := range paths {
		key := servedSubjectKey(path.ServedSubject)
		pathsBySubject[key] = append(pathsBySubject[key], path)
	}
	var summaries []ZoneServiceSummary
	for _, relation := range relations {
		subject := servedSubjectForRelation(relation)
		key := servedSubjectKey(subject)
		subjectPaths := append([]ZoneServicePath(nil), pathsBySubject[key]...)
		sort.SliceStable(subjectPaths, func(i, j int) bool {
			if subjectPaths[i].ServiceKind != subjectPaths[j].ServiceKind {
				return subjectPaths[i].ServiceKind < subjectPaths[j].ServiceKind
			}
			if subjectPaths[i].PathType != subjectPaths[j].PathType {
				return subjectPaths[i].PathType < subjectPaths[j].PathType
			}
			return subjectPaths[i].ID < subjectPaths[j].ID
		})
		summaries = append(summaries, ZoneServiceSummary{
			ID:            "zone-service:" + servedSubjectKey(subject),
			ZoneName:      relation.ZoneName,
			SpaceName:     relation.SpaceName,
			ServedSubject: subject,
			Paths:         subjectPaths,
			Issues:        append([]HVACWarning(nil), relation.Warnings...),
		})
	}
	return summaries
}

func buildZoneServicePaths(ctx *hvacContext, loops []HVACLoop, relations []HVACZoneChain, graph HVACRuleGraph, componentIndex ComponentIndex, couplingIndex CouplingIndex) []ZoneServicePath {
	_ = graph
	_ = componentIndex
	var paths []ZoneServicePath
	seen := map[string]bool{}
	addPath := func(path ZoneServicePath) {
		if path.PathType == "" || path.ServiceKind == "" || path.Delivery.ID == "" {
			return
		}
		path.ID = zoneServicePathID(path)
		path.SupportingCouplings = supportingCouplingIDsForPath(path, couplingIndex)
		key := path.ID
		if seen[key] {
			return
		}
		seen[key] = true
		paths = append(paths, path)
	}
	for _, relation := range relations {
		subject := servedSubjectForRelation(relation)
		deliveryByLabel := relationDeliveryComponentMap(relation)
		for _, chain := range relation.ServiceChains {
			delivery := deliveryForServiceChain(chain, deliveryByLabel)
			if delivery.ObjectType == "" && chain.Component != "" {
				delivery = componentByServiceLabel(relation.ZoneEquipment, chain.Component)
			}
			if delivery.ObjectType == "" {
				continue
			}
			deliveryInfo := classifyHVACDeliveryEquipment(ctx, delivery)
			if deliveryInfo.DeliveryType == "unknown_zone_equipment" {
				continue
			}
			plantRef := loopRefByName(loops, "PlantLoop", chain.PlantLoop)
			airLoopName := chain.AirLoopName
			if airLoopName == "" && deliveryInfo.RequiresAirLoop && len(relation.AirLoopNames) > 0 {
				airLoopName = relation.AirLoopNames[0]
			}
			airRef := loopRefByName(loops, "AirLoopHVAC", airLoopName)
			path := ZoneServicePath{
				ZoneName:          relation.ZoneName,
				SpaceName:         relation.SpaceName,
				ServiceKind:       serviceKindForServiceChain(chain, delivery),
				PathType:          pathTypeForDelivery(deliveryInfo.DeliveryType, plantRef != nil, airRef != nil, false),
				PlantLoop:         plantRef,
				AirLoop:           airRef,
				Delivery:          deliveryInfo.Component,
				DeliveryEquipment: deliveryInfo,
				ServedSubject:     subject,
				TraceIDs:          append([]string(nil), chain.SourceRelations...),
			}
			if chain.Component != "" && !strings.Contains(strings.ToLower(chain.Component), strings.ToLower(delivery.ObjectName)) {
				if component := componentByServiceLabel(append(loopComponentsForNames(loops, chain.AirLoopName, chain.PlantLoop), relation.ZoneEquipment...), chain.Component); component.ObjectType != "" {
					path.Conditioning = appendUniqueComponentRef(path.Conditioning, componentRefFromHVACComponent(component))
				} else {
					path.Conditioning = appendUniqueComponentRef(path.Conditioning, componentRefFromLabel("conditioning", chain.Component))
				}
			}
			if chain.SourceComponent != "" {
				path.SourceSystem = systemRefFromLabel("source", chain.SourceComponent, hvacMediumsForServiceKind(path.ServiceKind))
			}
			if path.SourceSystem == nil && plantRef == nil && airRef == nil {
				path.SourceSystem = localSourceSystemForDelivery(deliveryInfo.DeliveryType, path.ServiceKind)
			}
			if isVRFDelivery(deliveryInfo.DeliveryType) || serviceChainLooksRefrigerant(chain) {
				path.PathType = "direct_zone_refrigerant"
				path.RefrigerantSystem = refrigerantSystemForDelivery(ctx, delivery)
			}
			addPath(path)
		}
		for _, terminal := range relation.TerminalUnits {
			deliveryInfo := classifyHVACDeliveryEquipment(ctx, terminal)
			for _, airLoopName := range relation.AirLoopNames {
				airRef := loopRefByName(loops, "AirLoopHVAC", airLoopName)
				addPath(ZoneServicePath{
					ZoneName:          relation.ZoneName,
					SpaceName:         relation.SpaceName,
					ServiceKind:       "ventilation",
					PathType:          "central_air",
					AirLoop:           airRef,
					Delivery:          deliveryInfo.Component,
					DeliveryEquipment: deliveryInfo,
					DeliveryWrapper:   aduWrapperRefForTerminal(relation, terminal),
					ServedSubject:     subject,
					TraceIDs:          append([]string(nil), relation.RuleIDs...),
				})
			}
		}
		for _, equipment := range relation.ZoneEquipment {
			if isAirDistributionUnitType(equipment.ObjectType) || isTrueAirTerminalType(equipment.ObjectType) {
				continue
			}
			deliveryInfo := classifyHVACDeliveryEquipment(ctx, equipment)
			if deliveryInfo.DeliveryType == "unknown_zone_equipment" {
				continue
			}
			plantLoops := plantLoopRefsForComponent(ctx, loops, equipment)
			serviceKinds := serviceKindsForDelivery(ctx, equipment, deliveryInfo.DeliveryType)
			if len(plantLoops) == 0 {
				for _, serviceKind := range serviceKinds {
					sourceSystem := localSourceSystemForDelivery(deliveryInfo.DeliveryType, serviceKind)
					path := ZoneServicePath{
						ZoneName:          relation.ZoneName,
						SpaceName:         relation.SpaceName,
						ServiceKind:       serviceKind,
						PathType:          pathTypeForDelivery(deliveryInfo.DeliveryType, false, false, false),
						SourceSystem:      sourceSystem,
						Delivery:          deliveryInfo.Component,
						DeliveryEquipment: deliveryInfo,
						ServedSubject:     subject,
						TraceIDs:          append([]string(nil), relation.RuleIDs...),
					}
					if refrigerant := refrigerantSystemForDelivery(ctx, equipment); refrigerant != nil {
						path.RefrigerantSystem = refrigerant
						path.PathType = "direct_zone_refrigerant"
					}
					addPath(path)
				}
				continue
			}
			for _, plantLoop := range plantLoops {
				for _, serviceKind := range serviceKinds {
					addPath(ZoneServicePath{
						ZoneName:          relation.ZoneName,
						SpaceName:         relation.SpaceName,
						ServiceKind:       serviceKind,
						PathType:          pathTypeForDelivery(deliveryInfo.DeliveryType, true, false, false),
						PlantLoop:         &plantLoop,
						Delivery:          deliveryInfo.Component,
						DeliveryEquipment: deliveryInfo,
						ServedSubject:     subject,
						TraceIDs:          append([]string(nil), relation.RuleIDs...),
					})
				}
			}
		}
	}
	sort.SliceStable(paths, func(i, j int) bool {
		return paths[i].ID < paths[j].ID
	})
	return paths
}

func buildHVACSystemSummaries(loops []HVACLoop) []SystemSummary {
	var systems []SystemSummary
	for _, loop := range loops {
		systems = append(systems, SystemSummary{
			ID:          loop.ID,
			Type:        loop.Type,
			Name:        loop.Name,
			ObjectType:  loop.Type,
			ObjectIndex: loop.ObjectIndex,
			Role:        hvacLoopSystemRole(loop.Type),
			Mediums:     loopMediums(loop.Type),
		})
	}
	sort.SliceStable(systems, func(i, j int) bool {
		if systems[i].Type != systems[j].Type {
			return systems[i].Type < systems[j].Type
		}
		return strings.ToLower(systems[i].Name) < strings.ToLower(systems[j].Name)
	})
	return systems
}

func buildHVACSystemCouplings(ctx *hvacContext, loops []HVACLoop, componentIndex ComponentIndex) []SystemCoupling {
	_ = componentIndex
	var couplings []SystemCoupling
	seen := map[string]bool{}
	add := func(coupling SystemCoupling) {
		if coupling.CouplingType == "" || coupling.Object.ID == "" {
			return
		}
		if coupling.ID == "" {
			coupling.ID = "coupling:" + coupling.CouplingType + ":" + coupling.Object.ID
		}
		if seen[coupling.ID] {
			return
		}
		seen[coupling.ID] = true
		couplings = append(couplings, coupling)
	}
	for _, loop := range loops {
		for _, component := range loopComponents(loop) {
			couplingType, role := hvacCouplingTypeAndRoleForObject(component.ObjectType)
			if couplingType == "" {
				continue
			}
			add(SystemCoupling{
				CouplingType:   couplingType,
				Object:         componentRefFromHVACComponent(component),
				Role:           role,
				ConnectedLoops: []LoopRef{loopRefFromLoop(loop)},
				Mediums:        hvacMediumsForCoupling(couplingType, role, component.ObjectType),
				PlacementHint:  placementHintForCoupling(couplingType, loop.Type),
			})
		}
		if loop.OperationScheme != "" {
			if obj, ok := objectByName(ctx, loop.OperationScheme); ok {
				component := newHVACComponent(ctx, obj.Type, objectName(obj))
				couplingType, role := hvacCouplingTypeAndRoleForObject(obj.Type)
				if couplingType == "" {
					couplingType, role = "operation_scheme", "plant_operation_scheme"
				}
				add(SystemCoupling{
					CouplingType:   couplingType,
					Object:         componentRefFromHVACComponent(component),
					Role:           role,
					ConnectedLoops: []LoopRef{loopRefFromLoop(loop)},
					Mediums:        append([]string(nil), loopMediums(loop.Type)...),
					PlacementHint:  "detail_only",
				})
			}
		}
	}
	for _, obj := range ctx.doc.Objects {
		if componentOnAnyLoop(ctx, obj) {
			continue
		}
		couplingType, role := hvacCouplingTypeAndRoleForObject(obj.Type)
		if couplingType == "" {
			continue
		}
		component := newHVACComponent(ctx, obj.Type, objectName(obj))
		connectedLoops := connectedLoopsForObject(ctx, loops, obj)
		add(SystemCoupling{
			CouplingType:   couplingType,
			Object:         componentRefFromHVACComponent(component),
			Role:           role,
			ConnectedLoops: connectedLoops,
			Mediums:        hvacMediumsForCoupling(couplingType, role, obj.Type),
			PlacementHint:  placementHintForCoupling(couplingType, firstLoopType(connectedLoops)),
		})
	}
	sort.SliceStable(couplings, func(i, j int) bool {
		if couplings[i].CouplingType != couplings[j].CouplingType {
			return couplings[i].CouplingType < couplings[j].CouplingType
		}
		return couplings[i].ID < couplings[j].ID
	})
	return couplings
}

func buildHVACCouplingIndex(couplings []SystemCoupling) CouplingIndex {
	index := CouplingIndex{
		ByLoop:      map[string][]SystemCoupling{},
		ByComponent: map[string][]SystemCoupling{},
		ByNetwork:   map[string][]SystemCoupling{},
		ByZone:      map[string][]SystemCoupling{},
	}
	for _, coupling := range couplings {
		for _, loop := range coupling.ConnectedLoops {
			index.ByLoop[loopRefKey(loop.Type, loop.Name)] = append(index.ByLoop[loopRefKey(loop.Type, loop.Name)], coupling)
		}
		for _, component := range append([]ComponentRef{coupling.Object}, coupling.ConnectedComponents...) {
			index.ByComponent[component.ID] = append(index.ByComponent[component.ID], coupling)
		}
		for _, system := range coupling.ConnectedSystems {
			index.ByNetwork[system.ID] = append(index.ByNetwork[system.ID], coupling)
		}
		for _, zone := range coupling.ConnectedZones {
			index.ByZone[servedSubjectKey(zone)] = append(index.ByZone[servedSubjectKey(zone)], coupling)
		}
	}
	return index
}

func buildEnergyNetworks(ctx *hvacContext, couplings []SystemCoupling) []EnergyNetwork {
	var networks []EnergyNetwork
	electric := EnergyNetwork{ID: "network:electric", NetworkType: "electric_network", Name: "Electric network", Mediums: []string{"electricity"}}
	serviceWater := EnergyNetwork{ID: "network:service_water", NetworkType: "service_water", Name: "Service water network", Mediums: []string{"service_water"}}
	for _, coupling := range couplings {
		if stringSliceContainsFold(coupling.Mediums, "electricity") || coupling.CouplingType == "electric_storage" || coupling.CouplingType == "generator" {
			electric.Components = appendUniqueComponentRef(electric.Components, coupling.Object)
			electric.CouplingIDs = appendUniqueString(electric.CouplingIDs, coupling.ID)
		}
		if stringSliceContainsFold(coupling.Mediums, "service_water") || coupling.CouplingType == "service_water" {
			serviceWater.Components = appendUniqueComponentRef(serviceWater.Components, coupling.Object)
			serviceWater.CouplingIDs = appendUniqueString(serviceWater.CouplingIDs, coupling.ID)
		}
	}
	for _, obj := range ctx.doc.Objects {
		lower := normalizeFieldCatalogKey(obj.Type)
		if strings.HasPrefix(lower, "electricloadcenter:") {
			electric.Components = appendUniqueComponentRef(electric.Components, componentRefFromHVACComponent(newHVACComponent(ctx, obj.Type, objectName(obj))))
		}
		if strings.HasPrefix(lower, "wateruse:") {
			serviceWater.Components = appendUniqueComponentRef(serviceWater.Components, componentRefFromHVACComponent(newHVACComponent(ctx, obj.Type, objectName(obj))))
		}
	}
	if len(electric.Components) > 0 || len(electric.CouplingIDs) > 0 {
		networks = append(networks, electric)
	}
	if len(serviceWater.Components) > 0 || len(serviceWater.CouplingIDs) > 0 {
		networks = append(networks, serviceWater)
	}
	return networks
}

func classifyHVACDeliveryEquipment(ctx *hvacContext, component HVACComponent) HVACDeliveryEquipment {
	deliveryType := hvacDeliveryTypeForObject(component.ObjectType)
	ref := componentRefFromHVACComponent(component)
	ref.DeliveryType = deliveryType
	ref.DisplayFamily = displayFamilyForDeliveryType(deliveryType, component.ObjectType)
	info := HVACDeliveryEquipment{
		Component:        ref,
		DeliveryType:     deliveryType,
		DisplayFamily:    ref.DisplayFamily,
		RequiresAirLoop:  deliveryRequiresAirLoop(deliveryType),
		CanUsePlantLoop:  deliveryCanUsePlantLoop(deliveryType),
		HasInternalCoils: len(internalCoilRefs(ctx, component)) > 0,
		Mediums:          deliveryMediums(deliveryType, component.ObjectType),
	}
	return info
}

func hvacDeliveryTypeForObject(objectType string) string {
	lower := normalizeFieldCatalogKey(objectType)
	switch {
	case isTrueAirTerminalType(objectType) && strings.Contains(lower, "vav:reheat"):
		return "vav_reheat_terminal"
	case isTrueAirTerminalType(objectType) && strings.Contains(lower, "constantvolume"):
		return "constant_volume_terminal"
	case isTrueAirTerminalType(objectType) && (strings.Contains(lower, "seriespiu") || strings.Contains(lower, "parallelpiu")):
		return "fan_powered_terminal"
	case isTrueAirTerminalType(objectType):
		return "air_terminal"
	case isAirDistributionUnitType(objectType):
		return "adu"
	case lower == "zonehvac:fourpipefancoil":
		return "fan_coil"
	case lower == "zonehvac:packagedterminalairconditioner":
		return "ptac"
	case lower == "zonehvac:packagedterminalheatpump":
		return "pthp"
	case lower == "zonehvac:unitventilator":
		return "unit_ventilator"
	case lower == "zonehvac:unitheater":
		return "unit_heater"
	case lower == "zonehvac:windowairconditioner":
		return "window_ac"
	case lower == "zonehvac:terminalunit:variablerefrigerantflow":
		return "vrf_indoor"
	case lower == "zonehvac:watertoairheatpump":
		return "water_to_air_heat_pump"
	case strings.HasPrefix(lower, "zonehvac:lowtemperatureradiant:"):
		return "radiant_floor"
	case strings.HasPrefix(lower, "zonehvac:hightemperatureradiant") || strings.Contains(lower, "coolingpanel:radiant"):
		return "radiant_panel"
	case strings.HasPrefix(lower, "zonehvac:baseboard:"):
		return "baseboard"
	case lower == "zonehvac:idealloadsairsystem":
		return "ideal_loads"
	case lower == "zonehvac:energyrecoveryventilator":
		return "erv"
	case strings.Contains(lower, "exhaust"):
		return "zone_exhaust"
	case strings.HasPrefix(lower, "zonehvac:"):
		return "zone_direct_unit"
	default:
		return "unknown_zone_equipment"
	}
}

func isTrueAirTerminalType(objectType string) bool {
	return strings.HasPrefix(normalizeFieldCatalogKey(objectType), "airterminal:")
}

func displayFamilyForDeliveryType(deliveryType string, objectType string) string {
	switch deliveryType {
	case "air_terminal":
		return "Air Terminal"
	case "adu":
		return "ADU"
	case "vav_reheat_terminal":
		return "VAV Reheat Box"
	case "constant_volume_terminal":
		return "Constant Volume Terminal"
	case "fan_powered_terminal":
		return "Fan Powered Terminal"
	case "fan_coil":
		return "Fan Coil"
	case "ptac":
		return "PTAC"
	case "pthp":
		return "PTHP"
	case "unit_ventilator":
		return "Unit Ventilator"
	case "unit_heater":
		return "Unit Heater"
	case "window_ac":
		return "Window AC"
	case "vrf_indoor":
		return "VRF Indoor Unit"
	case "water_to_air_heat_pump":
		return "Water-to-Air Heat Pump"
	case "radiant_panel":
		return "Radiant Panel"
	case "radiant_floor":
		return "Radiant Floor"
	case "baseboard":
		return "Baseboard"
	case "ideal_loads":
		return "Ideal Loads"
	case "erv":
		return "Energy Recovery Ventilator"
	case "zone_exhaust":
		return "Zone Exhaust"
	case "zone_direct_unit":
		return "Direct Zone Unit"
	default:
		if strings.TrimSpace(objectType) != "" {
			return hvacComponentDisplayLabel(objectType)
		}
		return "HVAC Equipment"
	}
}

func deliveryRequiresAirLoop(deliveryType string) bool {
	switch deliveryType {
	case "air_terminal", "vav_reheat_terminal", "constant_volume_terminal", "fan_powered_terminal":
		return true
	default:
		return false
	}
}

func deliveryCanUsePlantLoop(deliveryType string) bool {
	switch deliveryType {
	case "air_terminal", "vav_reheat_terminal", "constant_volume_terminal", "fan_powered_terminal", "fan_coil", "unit_ventilator", "unit_heater", "water_to_air_heat_pump", "radiant_panel", "radiant_floor", "baseboard", "ptac", "pthp":
		return true
	default:
		return false
	}
}

func deliveryMediums(deliveryType string, objectType string) []string {
	switch deliveryType {
	case "vrf_indoor":
		return []string{"refrigerant", "air"}
	case "fan_coil", "unit_ventilator", "unit_heater", "radiant_panel", "radiant_floor", "baseboard", "water_to_air_heat_pump":
		return []string{"air", "hot_water", "chilled_water"}
	case "ptac", "pthp", "window_ac", "ideal_loads", "zone_direct_unit":
		return []string{"air", "electricity", "fuel"}
	case "erv", "zone_exhaust":
		return []string{"air"}
	case "air_terminal", "adu", "vav_reheat_terminal", "constant_volume_terminal", "fan_powered_terminal":
		return []string{"air"}
	default:
		return hvacMediumsForComponent(objectType)
	}
}

func pathTypeForDelivery(deliveryType string, hasPlantLoop bool, hasAirLoop bool, hasRefrigerant bool) string {
	switch {
	case hasRefrigerant || deliveryType == "vrf_indoor":
		return "direct_zone_refrigerant"
	case deliveryType == "radiant_floor" || deliveryType == "radiant_panel":
		return "radiant"
	case deliveryType == "baseboard":
		return "baseboard"
	case deliveryType == "ideal_loads":
		return "ideal_loads"
	case deliveryType == "erv" || deliveryType == "zone_exhaust":
		return "ventilation_only"
	case hasAirLoop && hasPlantLoop:
		return "central_air_with_plant"
	case hasAirLoop:
		return "central_air"
	case hasPlantLoop:
		return "direct_zone_hydronic"
	case deliveryType == "ptac" || deliveryType == "pthp" || deliveryType == "window_ac" || deliveryType == "unit_ventilator" || deliveryType == "unit_heater" || deliveryType == "fan_coil" || deliveryType == "water_to_air_heat_pump" || deliveryType == "zone_direct_unit":
		return "direct_zone_air"
	default:
		return "local"
	}
}

func serviceKindForServiceChain(chain HVACServicePath, delivery HVACComponent) string {
	text := strings.ToLower(strings.Join([]string{chain.Component, chain.SourceComponent, delivery.ObjectType, delivery.ObjectName}, " "))
	switch {
	case strings.Contains(text, "cool") || strings.Contains(text, "chiller") || strings.Contains(text, "dx"):
		return "cooling"
	case strings.Contains(text, "heat") || strings.Contains(text, "boiler") || strings.Contains(text, "baseboard"):
		return "heating"
	case strings.Contains(text, "exhaust"):
		return "exhaust"
	default:
		return "ventilation"
	}
}

func serviceKindsForDelivery(ctx *hvacContext, component HVACComponent, deliveryType string) []string {
	kinds := map[string]bool{}
	add := func(kind string) {
		if kind != "" {
			kinds[kind] = true
		}
	}
	lower := normalizeFieldCatalogKey(component.ObjectType)
	switch {
	case deliveryType == "ideal_loads":
		add("mixed")
	case deliveryType == "baseboard" || deliveryType == "unit_heater":
		add("heating")
	case deliveryType == "radiant_floor" || deliveryType == "radiant_panel":
		if strings.Contains(lower, "cool") {
			add("radiant_cooling")
		} else {
			add("radiant_heating")
		}
	case deliveryType == "erv" || deliveryType == "unit_ventilator":
		add("ventilation")
	case deliveryType == "zone_exhaust":
		add("exhaust")
	case deliveryType == "window_ac" || deliveryType == "ptac":
		add("cooling")
	case deliveryType == "pthp" || deliveryType == "fan_coil" || deliveryType == "water_to_air_heat_pump":
		add("cooling")
		add("heating")
	case deliveryType == "vrf_indoor":
		add("cooling")
		add("heating")
	default:
		add(serviceKindForObjectType(component.ObjectType))
	}
	for _, ref := range internalComponentRefs(ctx, component) {
		add(serviceKindForObjectType(ref.ObjectType))
	}
	if len(kinds) == 0 {
		add("mixed")
	}
	return sortedStringSet(kinds)
}

func serviceKindForObjectType(objectType string) string {
	lower := normalizeFieldCatalogKey(objectType)
	switch {
	case strings.Contains(lower, "cool"):
		return "cooling"
	case strings.Contains(lower, "heat") || strings.Contains(lower, "boiler") || strings.Contains(lower, "baseboard"):
		return "heating"
	case strings.Contains(lower, "exhaust"):
		return "exhaust"
	case strings.Contains(lower, "ventilat") || strings.Contains(lower, "outdoorair"):
		return "ventilation"
	default:
		return ""
	}
}

func hvacCouplingTypeAndRoleForObject(objectType string) (string, string) {
	lower := normalizeFieldCatalogKey(objectType)
	switch {
	case strings.HasPrefix(lower, "thermalstorage:ice:"):
		return "thermal_storage", "ice_storage"
	case strings.HasPrefix(lower, "thermalstorage:chilledwater:"):
		return "thermal_storage", "chilled_water_storage"
	case lower == "coil:cooling:dx:singlespeed:thermalstorage":
		return "thermal_storage", "dx_thermal_storage"
	case strings.HasPrefix(lower, "coolingtower:"):
		return "heat_rejection", "cooling_tower"
	case strings.HasPrefix(lower, "fluidcooler:") || strings.HasPrefix(lower, "evaporativefluidcooler:"):
		return "heat_rejection", "fluid_cooler"
	case strings.HasPrefix(lower, "groundheatexchanger:") || strings.HasPrefix(lower, "pipingsystem:underground:"):
		return "source_sink", "ground_hx"
	case strings.HasPrefix(lower, "generator:fuelcell:exhaustgastowaterheatexchanger") || strings.HasPrefix(lower, "generator:fuelcell:stackcooler"):
		return "heat_recovery", "fuel_cell_heat_recovery"
	case strings.HasPrefix(lower, "heatexchanger:fluidtofluid") || strings.Contains(lower, "heatexchanger"):
		return "heat_recovery", "fluid_heat_exchanger"
	case strings.HasPrefix(lower, "electricloadcenter:storage:"):
		return "electric_storage", "battery"
	case strings.HasPrefix(lower, "electricloadcenter:"):
		return "generator", "electric_load_center"
	case strings.HasPrefix(lower, "generator:photovoltaic") || strings.HasPrefix(lower, "generator:pvwatts"):
		return "generator", "pv"
	case strings.HasPrefix(lower, "generator:windturbine"):
		return "generator", "wind"
	case strings.HasPrefix(lower, "generator:fuelcell"):
		return "generator", "fuel_cell"
	case strings.HasPrefix(lower, "generator:fuelsupply"):
		return "source_sink", "fuel_supply"
	case strings.HasPrefix(lower, "generator:"):
		return "generator", "generator"
	case strings.HasPrefix(lower, "waterheater:"):
		return "service_water", "water_heater"
	case strings.HasPrefix(lower, "wateruse:"):
		return "service_water", "water_use"
	case strings.HasPrefix(lower, "setpointmanager:"):
		return "control_overlay", "setpoint_manager"
	case strings.HasPrefix(lower, "controller:"):
		return "control_overlay", "controller"
	case strings.HasPrefix(lower, "availabilitymanager:"):
		return "control_overlay", "availability_manager"
	case strings.HasPrefix(lower, "energymanagementsystem:") || strings.HasPrefix(lower, "pythonplugin:") || strings.HasPrefix(lower, "externalinterface:"):
		return "control_overlay", "ems_external"
	case strings.HasPrefix(lower, "faultmodel:"):
		return "fault_overlay", "fault_model"
	case strings.HasPrefix(lower, "plantequipmentoperation:thermalenergystorage"):
		return "operation_scheme", "thermal_storage_operation"
	case strings.HasPrefix(lower, "plantequipmentoperation:"):
		return "operation_scheme", "plant_operation_scheme"
	case isRefrigerantSystemType(objectType):
		return "refrigerant_network", "vrf_outdoor"
	default:
		return "", ""
	}
}

func hvacMediumsForCoupling(couplingType string, role string, objectType string) []string {
	switch couplingType {
	case "thermal_storage":
		return []string{"chilled_water", "electricity"}
	case "electric_storage", "generator":
		if role == "fuel_cell" || role == "generator" {
			return []string{"electricity", "fuel"}
		}
		return []string{"electricity"}
	case "heat_rejection":
		return []string{"condenser_water"}
	case "source_sink":
		if role == "fuel_supply" {
			return []string{"fuel"}
		}
		return []string{"condenser_water", "hot_water", "chilled_water"}
	case "heat_recovery":
		return []string{"hot_water", "chilled_water"}
	case "service_water":
		return []string{"service_water"}
	case "control_overlay", "operation_scheme":
		return []string{"control"}
	case "fault_overlay":
		return []string{"fault"}
	case "refrigerant_network":
		return []string{"refrigerant"}
	default:
		return hvacMediumsForComponent(objectType)
	}
}

func placementHintForCoupling(couplingType string, loopType string) string {
	switch couplingType {
	case "thermal_storage", "heat_recovery", "source_sink", "operation_scheme":
		return "below_plant"
	case "heat_rejection":
		return "right_of_loop"
	case "electric_storage", "generator":
		return "network_panel"
	case "service_water":
		return "network_panel"
	case "control_overlay", "fault_overlay":
		return "detail_only"
	default:
		if strings.EqualFold(loopType, "AirLoopHVAC") {
			return "below_air"
		}
		return "below_plant"
	}
}

func hvacMediumsForComponent(objectType string) []string {
	lower := normalizeFieldCatalogKey(objectType)
	switch {
	case strings.Contains(lower, "airterminal") || strings.Contains(lower, "airloop") || strings.Contains(lower, "fan") || strings.Contains(lower, "outdoorair"):
		return []string{"air"}
	case strings.Contains(lower, "cooling:water") || strings.Contains(lower, "chiller") || strings.Contains(lower, "districtcooling"):
		return []string{"chilled_water"}
	case strings.Contains(lower, "heating:water") || strings.Contains(lower, "boiler") || strings.Contains(lower, "districtheating") || strings.Contains(lower, "waterheater"):
		return []string{"hot_water"}
	case strings.Contains(lower, "coolingtower") || strings.Contains(lower, "condenser"):
		return []string{"condenser_water"}
	case isRefrigerantSystemType(objectType) || strings.Contains(lower, "refrigerant"):
		return []string{"refrigerant"}
	case strings.Contains(lower, "generator") || strings.Contains(lower, "electric") || strings.Contains(lower, "photovoltaic"):
		return []string{"electricity"}
	default:
		return nil
	}
}

func loopMediums(loopType string) []string {
	switch strings.ToLower(strings.TrimSpace(loopType)) {
	case "airloophvac":
		return []string{"air"}
	case "condenserloop":
		return []string{"condenser_water"}
	case "plantloop":
		return []string{"chilled_water", "hot_water"}
	default:
		return nil
	}
}

func hvacMediumsForServiceKind(serviceKind string) []string {
	switch serviceKind {
	case "cooling", "radiant_cooling":
		return []string{"chilled_water", "air"}
	case "heating", "radiant_heating":
		return []string{"hot_water", "air"}
	case "ventilation", "exhaust":
		return []string{"air"}
	default:
		return nil
	}
}

func componentRefFromHVACComponent(component HVACComponent) ComponentRef {
	deliveryType := hvacDeliveryTypeForObject(component.ObjectType)
	couplingType, _ := hvacCouplingTypeAndRoleForObject(component.ObjectType)
	return ComponentRef{
		ID:                   componentRefID(component.ObjectType, component.ObjectName, component.ObjectIndex),
		ObjectType:           component.ObjectType,
		ObjectName:           component.ObjectName,
		ObjectIndex:          component.ObjectIndex,
		DisplayName:          firstNonEmpty(component.ObjectName, component.DisplayLabel, component.ObjectType),
		Family:               component.Family,
		DisplayFamily:        firstNonEmpty(displayFamilyForDeliveryType(deliveryType, component.ObjectType), component.DisplayLabel, component.FamilyLabel),
		DeliveryType:         deliveryType,
		CouplingType:         couplingType,
		Role:                 component.RoleHere,
		Mediums:              hvacMediumsForComponent(component.ObjectType),
		InletNode:            component.InletNode,
		OutletNode:           component.OutletNode,
		WaterInletNode:       component.WaterInletNode,
		WaterOutletNode:      component.WaterOutletNode,
		ResolvedFromADU:      component.ResolvedFromADU,
		DistributionUnitName: component.DistributionUnitName,
	}
}

func componentRefFromLabel(kind string, label string) ComponentRef {
	label = strings.TrimSpace(label)
	return ComponentRef{
		ID:            "component:" + normalizeName(kind) + ":" + normalizeName(label),
		ObjectName:    label,
		DisplayName:   label,
		DisplayFamily: strings.Title(strings.ReplaceAll(kind, "_", " ")),
	}
}

func systemRefFromLabel(kind string, label string, mediums []string) *SystemRef {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	return &SystemRef{
		ID:          "system:" + normalizeName(kind) + ":" + normalizeName(label),
		Type:        kind,
		Name:        label,
		DisplayName: label,
		Mediums:     cleanStrings(mediums),
	}
}

func localSourceSystemForDelivery(deliveryType string, serviceKind string) *SystemRef {
	switch deliveryType {
	case "ptac", "pthp", "window_ac":
		if serviceKind == "heating" {
			return systemRefFromLabel("local", "Local electric/gas/heat pump", []string{"electricity", "fuel"})
		}
		return systemRefFromLabel("local_dx", "Local DX", []string{"refrigerant", "electricity"})
	case "ideal_loads":
		return systemRefFromLabel("ideal_loads", "Ideal Loads", []string{"air"})
	default:
		return nil
	}
}

func componentRefID(objectType string, objectName string, objectIndex int) string {
	if objectIndex >= 0 {
		return fmt.Sprintf("component:%d", objectIndex)
	}
	return "component:" + normalizeFieldCatalogKey(objectType) + ":" + normalizeName(objectName)
}

func appendUniqueComponentRef(values []ComponentRef, candidate ComponentRef) []ComponentRef {
	if candidate.ID == "" {
		return values
	}
	for _, value := range values {
		if value.ID == candidate.ID {
			return values
		}
	}
	return append(values, candidate)
}

func servedSubjectForRelation(relation HVACZoneChain) ServedSubjectRef {
	if relation.SpaceName != "" {
		return ServedSubjectRef{
			Kind:        "space",
			Name:        relation.SpaceName,
			ZoneName:    relation.ZoneName,
			SpaceName:   relation.SpaceName,
			ObjectIndex: relation.SpaceObjectIndex,
		}
	}
	return ServedSubjectRef{
		Kind:        "zone",
		Name:        relation.ZoneName,
		ZoneName:    relation.ZoneName,
		ObjectIndex: relation.ZoneObjectIndex,
	}
}

func servedSubjectKey(subject ServedSubjectRef) string {
	if subject.Kind == "space" && subject.SpaceName != "" {
		return "space:" + normalizeName(subject.SpaceName)
	}
	return "zone:" + normalizeName(subject.ZoneName)
}

func zoneServicePathID(path ZoneServicePath) string {
	parts := []string{
		"service-path",
		servedSubjectKey(path.ServedSubject),
		path.ServiceKind,
		path.PathType,
		path.Delivery.ID,
	}
	if path.PlantLoop != nil {
		parts = append(parts, loopRefKey(path.PlantLoop.Type, path.PlantLoop.Name))
	}
	if path.AirLoop != nil {
		parts = append(parts, loopRefKey(path.AirLoop.Type, path.AirLoop.Name))
	}
	if path.RefrigerantSystem != nil {
		parts = append(parts, path.RefrigerantSystem.ID)
	}
	return strings.Join(parts, ":")
}

func loopRefFromLoop(loop HVACLoop) LoopRef {
	return LoopRef{
		ID:          firstNonEmpty(loop.ID, loopRefKey(loop.Type, loop.Name)),
		Type:        loop.Type,
		Name:        loop.Name,
		ObjectIndex: loop.ObjectIndex,
		Mediums:     loopMediums(loop.Type),
	}
}

func loopRefByName(loops []HVACLoop, loopType string, loopName string) *LoopRef {
	if strings.TrimSpace(loopName) == "" {
		return nil
	}
	for _, loop := range loops {
		if strings.EqualFold(loop.Type, loopType) && strings.EqualFold(loop.Name, loopName) {
			ref := loopRefFromLoop(loop)
			return &ref
		}
	}
	return &LoopRef{ID: loopRefKey(loopType, loopName), Type: loopType, Name: loopName, Mediums: loopMediums(loopType)}
}

func loopRefKey(loopType string, loopName string) string {
	return normalizeFieldCatalogKey(loopType) + ":" + normalizeName(loopName)
}

func hvacLoopSystemRole(loopType string) string {
	switch strings.ToLower(strings.TrimSpace(loopType)) {
	case "airloophvac":
		return "air_system"
	case "plantloop":
		return "plant_loop"
	case "condenserloop":
		return "condenser_loop"
	default:
		return "system"
	}
}

func relationDeliveryComponentMap(relation HVACZoneChain) map[string]HVACComponent {
	out := map[string]HVACComponent{}
	for _, component := range append(append([]HVACComponent{}, relation.TerminalUnits...), relation.ZoneEquipment...) {
		for _, key := range []string{
			normalizeName(component.ObjectName),
			normalizeName(componentLabel(component)),
			normalizeName(component.DisplayLabel + " " + component.ObjectName),
		} {
			if key != "" {
				out[key] = component
			}
		}
	}
	return out
}

func deliveryForServiceChain(chain HVACServicePath, byLabel map[string]HVACComponent) HVACComponent {
	for _, label := range []string{chain.TerminalName, chain.Component} {
		if component, ok := byLabel[normalizeName(label)]; ok {
			return component
		}
	}
	return HVACComponent{}
}

func componentByServiceLabel(components []HVACComponent, label string) HVACComponent {
	wanted := normalizeName(label)
	if wanted == "" {
		return HVACComponent{}
	}
	for _, component := range components {
		for _, candidate := range []string{component.ObjectName, componentLabel(component), component.DisplayLabel + " " + component.ObjectName} {
			if normalizeName(candidate) == wanted || strings.Contains(wanted, normalizeName(component.ObjectName)) {
				return component
			}
		}
	}
	return HVACComponent{}
}

func loopComponentsForNames(loops []HVACLoop, loopNames ...string) []HVACComponent {
	wanted := map[string]bool{}
	for _, name := range loopNames {
		if name != "" {
			wanted[normalizeName(name)] = true
		}
	}
	var components []HVACComponent
	for _, loop := range loops {
		if wanted[normalizeName(loop.Name)] {
			components = append(components, loopComponents(loop)...)
		}
	}
	return components
}

func plantLoopRefsForComponent(ctx *hvacContext, loops []HVACLoop, component HVACComponent) []LoopRef {
	names := map[string]bool{}
	if key := hvacComponentKey(component); key != "" {
		addPlantLoopsForComponentKey(ctx, names, key)
	}
	for key := range typedHVACComponentReferenceKeys(ctx, component) {
		addPlantLoopsForComponentKey(ctx, names, key)
	}
	var refs []LoopRef
	for _, name := range sortedStringSet(names) {
		if ref := loopRefByName(loops, "PlantLoop", name); ref != nil {
			refs = append(refs, *ref)
		}
	}
	return refs
}

func internalComponentRefs(ctx *hvacContext, component HVACComponent) []ComponentRef {
	var refs []ComponentRef
	key := hvacComponentKey(component)
	for _, reference := range ctx.componentReferencesByFromKey[key] {
		if !reference.TargetExists {
			continue
		}
		target := newHVACComponent(ctx, reference.TargetObjectType, reference.TargetObjectName)
		refs = appendUniqueComponentRef(refs, componentRefFromHVACComponent(target))
	}
	return refs
}

func internalCoilRefs(ctx *hvacContext, component HVACComponent) []ComponentRef {
	var refs []ComponentRef
	for _, ref := range internalComponentRefs(ctx, component) {
		if strings.Contains(normalizeFieldCatalogKey(ref.ObjectType), "coil") {
			refs = appendUniqueComponentRef(refs, ref)
		}
	}
	return refs
}

func aduWrapperRefForTerminal(relation HVACZoneChain, terminal HVACComponent) *ComponentRef {
	if !terminal.ResolvedFromADU || terminal.DistributionUnitName == "" {
		return nil
	}
	for _, equipment := range relation.ZoneEquipment {
		if isAirDistributionUnitType(equipment.ObjectType) && strings.EqualFold(equipment.ObjectName, terminal.DistributionUnitName) {
			ref := componentRefFromHVACComponent(equipment)
			return &ref
		}
	}
	return nil
}

func refrigerantSystemForDelivery(ctx *hvacContext, delivery HVACComponent) *SystemRef {
	for _, reference := range ctx.componentReferences {
		if !reference.TargetExists || !strings.EqualFold(reference.TargetObjectType, delivery.ObjectType) || !strings.EqualFold(reference.TargetObjectName, delivery.ObjectName) {
			continue
		}
		if isRefrigerantSystemType(reference.FromObjectType) {
			return &SystemRef{
				ID:          componentRefID(reference.FromObjectType, reference.FromObjectName, reference.FromObjectIndex),
				Type:        "refrigerant_system",
				Name:        reference.FromObjectName,
				ObjectType:  reference.FromObjectType,
				ObjectName:  reference.FromObjectName,
				ObjectIndex: reference.FromObjectIndex,
				DisplayName: reference.FromObjectName,
				Mediums:     []string{"refrigerant"},
			}
		}
	}
	return nil
}

func isVRFDelivery(deliveryType string) bool {
	return deliveryType == "vrf_indoor"
}

func serviceChainLooksRefrigerant(chain HVACServicePath) bool {
	return strings.Contains(strings.ToLower(chain.SourceComponent+" "+chain.Component), "variablerefrigerantflow") ||
		strings.Contains(strings.ToLower(chain.SourceComponent+" "+chain.Component), "vrf")
}

func supportingCouplingIDsForPath(path ZoneServicePath, index CouplingIndex) []string {
	ids := map[string]bool{}
	add := func(couplings []SystemCoupling) {
		for _, coupling := range couplings {
			if coupling.ID != "" {
				ids[coupling.ID] = true
			}
		}
	}
	if path.PlantLoop != nil {
		add(index.ByLoop[loopRefKey(path.PlantLoop.Type, path.PlantLoop.Name)])
	}
	if path.CondenserLoop != nil {
		add(index.ByLoop[loopRefKey(path.CondenserLoop.Type, path.CondenserLoop.Name)])
	}
	if path.AirLoop != nil {
		add(index.ByLoop[loopRefKey(path.AirLoop.Type, path.AirLoop.Name)])
	}
	add(index.ByComponent[path.Delivery.ID])
	return sortedStringSet(ids)
}

func objectByName(ctx *hvacContext, name string) (Object, bool) {
	objects := ctx.objectsByName[normalizeName(name)]
	if len(objects) == 0 {
		return Object{}, false
	}
	return objects[0], true
}

func connectedLoopsForObject(ctx *hvacContext, loops []HVACLoop, obj Object) []LoopRef {
	component := newHVACComponent(ctx, obj.Type, objectName(obj))
	key := hvacComponentKey(component)
	seen := map[string]bool{}
	var refs []LoopRef
	for loopName, loopType := range ctx.componentLoopTypes[key] {
		if ref := loopRefByName(loops, loopType, loopName); ref != nil {
			refs = append(refs, *ref)
			seen[loopRefKey(loopType, loopName)] = true
		}
	}
	for _, loop := range loops {
		if loop.OperationScheme != "" && strings.EqualFold(loop.OperationScheme, objectName(obj)) {
			key := loopRefKey(loop.Type, loop.Name)
			if !seen[key] {
				refs = append(refs, loopRefFromLoop(loop))
				seen[key] = true
			}
		}
	}
	sort.SliceStable(refs, func(i, j int) bool {
		return refs[i].ID < refs[j].ID
	})
	return refs
}

func componentOnAnyLoop(ctx *hvacContext, obj Object) bool {
	key := hvacObjectKey(obj.Type, objectName(obj))
	return len(ctx.componentLoopTypes[key]) > 0
}

func firstLoopType(refs []LoopRef) string {
	if len(refs) == 0 {
		return ""
	}
	return refs[0].Type
}
