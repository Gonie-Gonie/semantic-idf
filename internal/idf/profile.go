package idf

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	ProfileDimensionOccupancy    = "occupancy"
	ProfileDimensionLighting     = "lighting"
	ProfileDimensionEquipment    = "equipment"
	ProfileDimensionInfiltration = "infiltration"
	ProfileDimensionVentilation  = "ventilation"
	ProfileDimensionOutdoorAir   = "outdoor_air"
)

type ProfileReport struct {
	ZoneCount       int                      `json:"zoneCount"`
	ItemCount       int                      `json:"itemCount"`
	GroupCount      int                      `json:"groupCount"`
	Dimensions      []ProfileDimensionOption `json:"dimensions"`
	MetricOptions   []ProfileMetricOption    `json:"metricOptions"`
	ZoneProfiles    []ZoneProfile            `json:"zoneProfiles"`
	Groups          []ProfileGroup           `json:"groups"`
	Matrix          []ProfileMatrixRow       `json:"matrix"`
	Schedules       []ScheduleSummary        `json:"schedules"`
	Warnings        []ProfileWarning         `json:"warnings"`
	DefaultSettings ProfileAnalysisSettings  `json:"defaultSettings"`
}

type ProfileAnalysisSettings struct {
	EnabledDimensions   []string             `json:"enabledDimensions"`
	DisplayMetrics      map[string]string    `json:"displayMetrics"`
	GroupingMetrics     map[string]string    `json:"groupingMetrics"`
	NumericTolerance    float64              `json:"numericTolerance"`
	ScheduleCompareMode string               `json:"scheduleCompareMode"`
	GraphMode           string               `json:"graphMode"`
	ScheduleSummaryMode string               `json:"scheduleSummaryMode"`
	ApplyBehavior       ProfileApplyBehavior `json:"applyBehavior"`
}

type ProfileApplyBehavior struct {
	DefaultMode           string `json:"defaultMode"`
	AllowZoneListEdit     bool   `json:"allowZoneListEdit"`
	CreateMissingZoneList bool   `json:"createMissingZoneList"`
	NameSuffix            string `json:"nameSuffix"`
	ReplaceExistingPolicy string `json:"replaceExistingPolicy"`
}

type ProfileDimensionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type ProfileMetricOption struct {
	Dimension string `json:"dimension"`
	ID        string `json:"id"`
	Label     string `json:"label"`
	Unit      string `json:"unit,omitempty"`
}

type ZoneProfile struct {
	ZoneName        string                    `json:"zoneName"`
	ZoneObjectIndex int                       `json:"zoneObjectIndex"`
	FloorArea       float64                   `json:"floorArea"`
	Volume          float64                   `json:"volume"`
	ExteriorArea    float64                   `json:"exteriorArea"`
	Items           []ProfileItem             `json:"items"`
	Dimensions      []ProfileDimensionSummary `json:"dimensions"`
	Warnings        []ProfileWarning          `json:"warnings,omitempty"`
}

type ProfileItem struct {
	ID               string           `json:"id"`
	ZoneName         string           `json:"zoneName"`
	Dimension        string           `json:"dimension"`
	ObjectIndex      int              `json:"objectIndex"`
	ObjectType       string           `json:"objectType"`
	ObjectName       string           `json:"objectName,omitempty"`
	SourceTarget     string           `json:"sourceTarget,omitempty"`
	SourceTargetKind string           `json:"sourceTargetKind,omitempty"`
	ScheduleName     string           `json:"scheduleName,omitempty"`
	SchedulePattern  string           `json:"schedulePattern,omitempty"`
	ScheduleHash     string           `json:"scheduleHash,omitempty"`
	RawMethod        string           `json:"rawMethod,omitempty"`
	RawValue         string           `json:"rawValue,omitempty"`
	Normalized       []ProfileMetric  `json:"normalized"`
	DisplayMetric    ProfileMetric    `json:"displayMetric"`
	CloneEligible    bool             `json:"cloneEligible"`
	Warnings         []ProfileWarning `json:"warnings,omitempty"`
}

type ProfileMetric struct {
	ID           string  `json:"id"`
	Label        string  `json:"label"`
	Unit         string  `json:"unit,omitempty"`
	Value        float64 `json:"value"`
	DisplayValue string  `json:"displayValue"`
	Status       string  `json:"status"`
}

type ProfileDimensionSummary struct {
	Dimension       string           `json:"dimension"`
	Label           string           `json:"label"`
	MetricID        string           `json:"metricId"`
	MetricLabel     string           `json:"metricLabel"`
	Unit            string           `json:"unit,omitempty"`
	Value           float64          `json:"value"`
	DisplayValue    string           `json:"displayValue"`
	Status          string           `json:"status"`
	ScheduleName    string           `json:"scheduleName,omitempty"`
	SchedulePattern string           `json:"schedulePattern,omitempty"`
	ScheduleHash    string           `json:"scheduleHash,omitempty"`
	ItemIDs         []string         `json:"itemIds"`
	ItemCount       int              `json:"itemCount"`
	Warnings        []ProfileWarning `json:"warnings,omitempty"`
}

type ProfileGroup struct {
	ID         string                    `json:"id"`
	Name       string                    `json:"name"`
	Key        string                    `json:"key"`
	ZoneNames  []string                  `json:"zoneNames"`
	ZoneCount  int                       `json:"zoneCount"`
	Dimensions []ProfileDimensionSummary `json:"dimensions"`
	ItemIDs    []string                  `json:"itemIds"`
	Warnings   []ProfileWarning          `json:"warnings,omitempty"`
}

type ProfileMatrixRow struct {
	ZoneName        string                    `json:"zoneName"`
	ZoneObjectIndex int                       `json:"zoneObjectIndex"`
	Dimensions      []ProfileDimensionSummary `json:"dimensions"`
	Warnings        []ProfileWarning          `json:"warnings,omitempty"`
}

type ProfileWarning struct {
	Severity    string `json:"severity"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	ZoneName    string `json:"zoneName,omitempty"`
	Dimension   string `json:"dimension,omitempty"`
	ObjectIndex int    `json:"objectIndex,omitempty"`
	ObjectType  string `json:"objectType,omitempty"`
	ObjectName  string `json:"objectName,omitempty"`
}

type ScheduleSummary struct {
	ScheduleName    string              `json:"scheduleName"`
	ScheduleType    string              `json:"scheduleType"`
	ObjectIndex     int                 `json:"objectIndex"`
	Resolved        bool                `json:"resolved"`
	DetectedPattern string              `json:"detectedPattern"`
	WeekdayProfile  []float64           `json:"weekdayProfile"`
	SaturdayProfile []float64           `json:"saturdayProfile"`
	SundayProfile   []float64           `json:"sundayProfile"`
	HolidayProfile  []float64           `json:"holidayProfile"`
	WeeklyProfile   []float64           `json:"weeklyProfile,omitempty"`
	Rules           []ScheduleRule      `json:"rules,omitempty"`
	AnnualStats     ScheduleAnnualStats `json:"annualStats"`
	ContentHash     string              `json:"contentHash"`
	Warnings        []string            `json:"warnings,omitempty"`
}

type ScheduleRule struct {
	StartDay  int                `json:"startDay"`
	EndDay    int                `json:"endDay"`
	Through   string             `json:"through"`
	Selector  string             `json:"selector"`
	Label     string             `json:"label"`
	Intervals []ScheduleInterval `json:"intervals"`
}

type ScheduleInterval struct {
	StartHour float64 `json:"startHour"`
	EndHour   float64 `json:"endHour"`
	Value     float64 `json:"value"`
	Label     string  `json:"label"`
}

type ScheduleAnnualStats struct {
	Average             float64 `json:"average"`
	Max                 float64 `json:"max"`
	P95                 float64 `json:"p95"`
	OperatingHours      float64 `json:"operatingHours"`
	AboveHalfHours      float64 `json:"aboveHalfHours"`
	EquivalentFullHours float64 `json:"equivalentFullHours"`
}

type profileZoneContext struct {
	name         string
	objectIndex  int
	floorArea    float64
	volume       float64
	exteriorArea float64
}

type profileContext struct {
	doc           Document
	zones         []profileZoneContext
	zoneByKey     map[string]profileZoneContext
	zoneLists     map[string][]string
	spaceToZone   map[string]string
	spaceLists    map[string][]string
	scheduleByKey map[string]ScheduleSummary
	peopleDensity map[string]float64
	peopleCount   map[string]float64
	warnings      []ProfileWarning
}

func AnalyzeProfile(doc Document) ProfileReport {
	ctx := newProfileContext(doc)
	report := ProfileReport{
		ZoneCount:       len(ctx.zones),
		Dimensions:      profileDimensionOptions(),
		MetricOptions:   profileMetricOptions(),
		Schedules:       profileScheduleList(ctx.scheduleByKey),
		DefaultSettings: defaultProfileAnalysisSettings(),
	}

	zoneItems := map[string][]ProfileItem{}
	for _, item := range ctx.collectProfileItems() {
		zoneItems[normalizeName(item.ZoneName)] = append(zoneItems[normalizeName(item.ZoneName)], item)
		report.ItemCount++
	}

	for _, zone := range ctx.zones {
		items := zoneItems[normalizeName(zone.name)]
		dimensions := summarizeProfileDimensions(items, defaultProfileAnalysisSettings().DisplayMetrics)
		warnings := zoneProfileWarnings(zone.name, items)
		report.ZoneProfiles = append(report.ZoneProfiles, ZoneProfile{
			ZoneName:        zone.name,
			ZoneObjectIndex: zone.objectIndex,
			FloorArea:       roundedNumber(zone.floorArea, 3),
			Volume:          roundedNumber(zone.volume, 3),
			ExteriorArea:    roundedNumber(zone.exteriorArea, 3),
			Items:           items,
			Dimensions:      dimensions,
			Warnings:        warnings,
		})
		report.Matrix = append(report.Matrix, ProfileMatrixRow{
			ZoneName:        zone.name,
			ZoneObjectIndex: zone.objectIndex,
			Dimensions:      dimensions,
			Warnings:        warnings,
		})
		report.Warnings = append(report.Warnings, warnings...)
	}

	report.Warnings = append(report.Warnings, ctx.warnings...)
	report.Groups = buildDefaultProfileGroups(report.ZoneProfiles, report.DefaultSettings)
	report.GroupCount = len(report.Groups)
	return report
}

func defaultProfileAnalysisSettings() ProfileAnalysisSettings {
	return ProfileAnalysisSettings{
		EnabledDimensions: []string{
			ProfileDimensionOccupancy,
			ProfileDimensionLighting,
			ProfileDimensionEquipment,
			ProfileDimensionInfiltration,
			ProfileDimensionVentilation,
			ProfileDimensionOutdoorAir,
		},
		DisplayMetrics: map[string]string{
			ProfileDimensionOccupancy:    "people_per_area",
			ProfileDimensionLighting:     "power_per_area",
			ProfileDimensionEquipment:    "power_per_area",
			ProfileDimensionInfiltration: "ach",
			ProfileDimensionVentilation:  "flow_per_person",
			ProfileDimensionOutdoorAir:   "flow_per_person",
		},
		GroupingMetrics: map[string]string{
			ProfileDimensionOccupancy:    "people_per_area",
			ProfileDimensionLighting:     "power_per_area",
			ProfileDimensionEquipment:    "power_per_area",
			ProfileDimensionInfiltration: "ach",
			ProfileDimensionVentilation:  "flow_per_person",
			ProfileDimensionOutdoorAir:   "flow_per_person",
		},
		NumericTolerance:    0.001,
		ScheduleCompareMode: "name",
		GraphMode:           "actual_value",
		ScheduleSummaryMode: "annual_heatmap",
		ApplyBehavior: ProfileApplyBehavior{
			DefaultMode:           "clone",
			AllowZoneListEdit:     false,
			CreateMissingZoneList: false,
			NameSuffix:            " Profile Copy",
			ReplaceExistingPolicy: "replace",
		},
	}
}

func DefaultProfileAnalysisSettings() ProfileAnalysisSettings {
	return defaultProfileAnalysisSettings()
}

func profileDimensionOptions() []ProfileDimensionOption {
	return []ProfileDimensionOption{
		{ID: ProfileDimensionOccupancy, Label: "Occupancy"},
		{ID: ProfileDimensionLighting, Label: "Lighting"},
		{ID: ProfileDimensionEquipment, Label: "Equipment"},
		{ID: ProfileDimensionInfiltration, Label: "Infiltration"},
		{ID: ProfileDimensionVentilation, Label: "Ventilation"},
		{ID: ProfileDimensionOutdoorAir, Label: "Outdoor Air"},
	}
}

func profileMetricOptions() []ProfileMetricOption {
	return []ProfileMetricOption{
		{Dimension: ProfileDimensionOccupancy, ID: "count", Label: "People", Unit: "people"},
		{Dimension: ProfileDimensionOccupancy, ID: "people_per_area", Label: "People density", Unit: "people/m2"},
		{Dimension: ProfileDimensionOccupancy, ID: "area_per_person", Label: "Area per person", Unit: "m2/person"},
		{Dimension: ProfileDimensionLighting, ID: "total_power", Label: "Total power", Unit: "W"},
		{Dimension: ProfileDimensionLighting, ID: "power_per_area", Label: "Power density", Unit: "W/m2"},
		{Dimension: ProfileDimensionLighting, ID: "power_per_person", Label: "Power per person", Unit: "W/person"},
		{Dimension: ProfileDimensionEquipment, ID: "total_power", Label: "Total power", Unit: "W"},
		{Dimension: ProfileDimensionEquipment, ID: "power_per_area", Label: "Power density", Unit: "W/m2"},
		{Dimension: ProfileDimensionEquipment, ID: "power_per_person", Label: "Power per person", Unit: "W/person"},
		{Dimension: ProfileDimensionInfiltration, ID: "flow", Label: "Flow", Unit: "m3/s"},
		{Dimension: ProfileDimensionInfiltration, ID: "flow_per_area", Label: "Flow per floor area", Unit: "m3/s-m2"},
		{Dimension: ProfileDimensionInfiltration, ID: "flow_per_exterior_area", Label: "Flow per exterior area", Unit: "m3/s-m2"},
		{Dimension: ProfileDimensionInfiltration, ID: "ach", Label: "Air changes", Unit: "ACH"},
		{Dimension: ProfileDimensionVentilation, ID: "flow", Label: "Flow", Unit: "m3/s"},
		{Dimension: ProfileDimensionVentilation, ID: "flow_per_person", Label: "Flow per person", Unit: "m3/s-person"},
		{Dimension: ProfileDimensionVentilation, ID: "flow_per_area", Label: "Flow per floor area", Unit: "m3/s-m2"},
		{Dimension: ProfileDimensionVentilation, ID: "ach", Label: "Air changes", Unit: "ACH"},
		{Dimension: ProfileDimensionOutdoorAir, ID: "flow", Label: "Flow", Unit: "m3/s"},
		{Dimension: ProfileDimensionOutdoorAir, ID: "flow_per_person", Label: "Flow per person", Unit: "m3/s-person"},
		{Dimension: ProfileDimensionOutdoorAir, ID: "flow_per_area", Label: "Flow per floor area", Unit: "m3/s-m2"},
		{Dimension: ProfileDimensionOutdoorAir, ID: "ach", Label: "Air changes", Unit: "ACH"},
	}
}

func newProfileContext(doc Document) profileContext {
	ctx := profileContext{
		doc:           doc,
		zoneByKey:     map[string]profileZoneContext{},
		zoneLists:     map[string][]string{},
		spaceToZone:   map[string]string{},
		spaceLists:    map[string][]string{},
		scheduleByKey: map[string]ScheduleSummary{},
		peopleDensity: map[string]float64{},
		peopleCount:   map[string]float64{},
	}
	geometry := AnalyzeGeometry(doc)
	geometryZones := map[string]GeometryZone{}
	for _, zone := range geometry.Zones {
		geometryZones[normalizeName(zone.Name)] = zone
	}
	exteriorArea := map[string]float64{}
	for _, surface := range geometry.Surfaces {
		if isExteriorSurface(surface.Type, surface.OutsideBoundary) {
			exteriorArea[normalizeName(surface.ZoneName)] += surface.Area
		}
	}

	for _, obj := range doc.Objects {
		switch {
		case strings.EqualFold(obj.Type, "Zone"):
			name := objectName(obj)
			if name == "" {
				continue
			}
			geom := geometryZones[normalizeName(name)]
			area := geom.FloorArea
			volume := geom.Volume
			if area <= 0 {
				area = numericFieldOrDefault(obj, 0, "floor", "area")
			}
			if volume <= 0 {
				volume = numericFieldOrDefault(obj, 0, "volume")
			}
			zone := profileZoneContext{
				name:         name,
				objectIndex:  obj.Index,
				floorArea:    area,
				volume:       volume,
				exteriorArea: exteriorArea[normalizeName(name)],
			}
			ctx.zones = append(ctx.zones, zone)
			ctx.zoneByKey[normalizeName(name)] = zone
		case strings.EqualFold(obj.Type, "ZoneList"):
			if name := objectName(obj); name != "" {
				ctx.zoneLists[normalizeName(name)] = zoneListMembers(obj)
			}
		case strings.EqualFold(obj.Type, "Space"):
			spaceName := objectName(obj)
			zoneName := findFieldByCommentWords(obj, "zone", "name")
			if spaceName != "" && zoneName != "" {
				ctx.spaceToZone[normalizeName(spaceName)] = zoneName
			}
		case strings.EqualFold(obj.Type, "SpaceList"):
			if name := objectName(obj); name != "" {
				ctx.spaceLists[normalizeName(name)] = zoneListMembers(obj)
			}
		case isScheduleType(obj.Type):
			if name := objectName(obj); name != "" {
				ctx.scheduleByKey[normalizeName(name)] = summarizeSchedule(obj)
			}
		}
	}

	ctx.seedPeople()
	return ctx
}

func (ctx *profileContext) seedPeople() {
	for _, obj := range ctx.doc.Objects {
		if !strings.EqualFold(obj.Type, "People") {
			continue
		}
		for _, zoneName := range ctx.objectTargetZones(obj) {
			zone := ctx.zoneByKey[normalizeName(zoneName)]
			count, _, _ := ctx.peopleCountForObject(obj, zone)
			if count > 0 {
				ctx.peopleCount[normalizeName(zoneName)] += count
				if zone.floorArea > 0 {
					ctx.peopleDensity[normalizeName(zoneName)] += count / zone.floorArea
				}
			}
		}
	}
}

func (ctx *profileContext) collectProfileItems() []ProfileItem {
	var items []ProfileItem
	for _, obj := range ctx.doc.Objects {
		dimension := profileDimensionForObject(obj.Type)
		if dimension == "" {
			continue
		}
		if strings.EqualFold(obj.Type, "DesignSpecification:OutdoorAir") {
			items = append(items, ctx.outdoorAirItems(obj)...)
			continue
		}
		for _, zoneName := range ctx.objectTargetZones(obj) {
			zone, ok := ctx.zoneByKey[normalizeName(zoneName)]
			if !ok {
				items = append(items, ctx.missingZoneItem(obj, dimension, zoneName))
				continue
			}
			if item, ok := ctx.profileItemForZone(obj, dimension, zone); ok {
				items = append(items, item)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ZoneName == items[j].ZoneName {
			if items[i].Dimension == items[j].Dimension {
				return items[i].ObjectIndex < items[j].ObjectIndex
			}
			return items[i].Dimension < items[j].Dimension
		}
		return items[i].ZoneName < items[j].ZoneName
	})
	return items
}

func profileDimensionForObject(objectType string) string {
	switch strings.ToLower(strings.TrimSpace(objectType)) {
	case "people":
		return ProfileDimensionOccupancy
	case "lights":
		return ProfileDimensionLighting
	case "electricequipment", "gasequipment", "otherequipment":
		return ProfileDimensionEquipment
	case "zoneinfiltration:designflowrate", "zoneinfiltration:effectiveleakagearea", "zoneinfiltration:flowcoefficient":
		return ProfileDimensionInfiltration
	case "zoneventilation:designflowrate", "zoneventilation:windandstackopenarea":
		return ProfileDimensionVentilation
	case "designspecification:outdoorair":
		return ProfileDimensionOutdoorAir
	default:
		return ""
	}
}

func (ctx profileContext) profileItemForZone(obj Object, dimension string, zone profileZoneContext) (ProfileItem, bool) {
	var metrics []ProfileMetric
	rawMethod := profileCalculationMethod(obj)
	rawValue := ""
	warnings := []ProfileWarning{}

	switch dimension {
	case ProfileDimensionOccupancy:
		count, ok, itemWarnings := ctx.peopleCountForObject(obj, zone)
		warnings = append(warnings, itemWarnings...)
		metrics = append(metrics,
			profileMetric("count", "People", "people", count, ok, 2),
			profileMetric("people_per_area", "People density", "people/m2", divide(count, zone.floorArea), ok && zone.floorArea > 0, 4),
			profileMetric("area_per_person", "Area per person", "m2/person", divide(zone.floorArea, count), ok && zone.floorArea > 0 && count > 0, 2),
		)
		rawValue = firstNumericRawValue(obj, "number", "people")
	case ProfileDimensionLighting:
		power, ok, itemWarnings := ctx.designPowerForObject(obj, zone, "lighting")
		warnings = append(warnings, itemWarnings...)
		metrics = append(metrics,
			profileMetric("total_power", "Total power", "W", power, ok, 2),
			profileMetric("power_per_area", "Power density", "W/m2", divide(power, zone.floorArea), ok && zone.floorArea > 0, 3),
			profileMetric("power_per_person", "Power per person", "W/person", divide(power, ctx.peopleCount[normalizeName(zone.name)]), ok && ctx.peopleCount[normalizeName(zone.name)] > 0, 2),
		)
		rawValue = firstNumericRawValue(obj, "lighting", "level")
	case ProfileDimensionEquipment:
		power, ok, itemWarnings := ctx.designPowerForObject(obj, zone, "equipment")
		warnings = append(warnings, itemWarnings...)
		metrics = append(metrics,
			profileMetric("total_power", "Total power", "W", power, ok, 2),
			profileMetric("power_per_area", "Power density", "W/m2", divide(power, zone.floorArea), ok && zone.floorArea > 0, 3),
			profileMetric("power_per_person", "Power per person", "W/person", divide(power, ctx.peopleCount[normalizeName(zone.name)]), ok && ctx.peopleCount[normalizeName(zone.name)] > 0, 2),
		)
		rawValue = firstNumericRawValue(obj, "design", "level")
	case ProfileDimensionInfiltration:
		flow, ok, itemWarnings := ctx.airflowForObject(obj, zone, false)
		warnings = append(warnings, itemWarnings...)
		metrics = profileAirflowMetrics(flow, ok, zone, 0, false)
		rawValue = firstNumericRawValue(obj, "design", "flow", "rate")
	case ProfileDimensionVentilation:
		flow, ok, itemWarnings := ctx.airflowForObject(obj, zone, true)
		warnings = append(warnings, itemWarnings...)
		metrics = profileAirflowMetrics(flow, ok, zone, ctx.peopleCount[normalizeName(zone.name)], true)
		rawValue = firstNumericRawValue(obj, "design", "flow", "rate")
	default:
		return ProfileItem{}, false
	}

	scheduleName := profileScheduleName(obj)
	schedule := ctx.scheduleByKey[normalizeName(scheduleName)]
	item := ProfileItem{
		ID:               fmt.Sprintf("profile-item-%d-%s-%s", obj.Index, dimension, safeID(zone.name)),
		ZoneName:         zone.name,
		Dimension:        dimension,
		ObjectIndex:      obj.Index,
		ObjectType:       obj.Type,
		ObjectName:       objectName(obj),
		SourceTarget:     profileTargetName(obj),
		SourceTargetKind: ctx.profileTargetKind(profileTargetName(obj)),
		ScheduleName:     scheduleName,
		SchedulePattern:  schedule.DetectedPattern,
		ScheduleHash:     schedule.ContentHash,
		RawMethod:        rawMethod,
		RawValue:         rawValue,
		Normalized:       metrics,
		DisplayMetric:    selectProfileMetric(metrics, defaultProfileAnalysisSettings().DisplayMetrics[dimension]),
		CloneEligible:    true,
		Warnings:         warnings,
	}
	if scheduleName != "" && schedule.ScheduleName == "" {
		item.Warnings = append(item.Warnings, profileWarning("warning", "missing_schedule_summary", "Schedule could not be resolved for profile timing.", zone.name, dimension, obj))
	}
	return item, true
}

func (ctx *profileContext) outdoorAirItems(obj Object) []ProfileItem {
	var items []ProfileItem
	targetZones := ctx.outdoorAirTargetZones(objectName(obj))
	if len(targetZones) == 0 {
		ctx.warnings = append(ctx.warnings, profileWarning("warning", "unassigned_outdoor_air", "DesignSpecification:OutdoorAir is not linked to a Sizing:Zone target.", "", ProfileDimensionOutdoorAir, obj))
		return nil
	}
	for _, zoneName := range targetZones {
		zone, ok := ctx.zoneByKey[normalizeName(zoneName)]
		if !ok {
			items = append(items, ctx.missingZoneItem(obj, ProfileDimensionOutdoorAir, zoneName))
			continue
		}
		flow, ok, warnings := ctx.outdoorAirFlowForObject(obj, zone)
		metrics := profileAirflowMetrics(flow, ok, zone, ctx.peopleCount[normalizeName(zone.name)], true)
		scheduleName := profileScheduleName(obj)
		schedule := ctx.scheduleByKey[normalizeName(scheduleName)]
		items = append(items, ProfileItem{
			ID:              fmt.Sprintf("profile-item-%d-outdoor-air-%s", obj.Index, safeID(zone.name)),
			ZoneName:        zone.name,
			Dimension:       ProfileDimensionOutdoorAir,
			ObjectIndex:     obj.Index,
			ObjectType:      obj.Type,
			ObjectName:      objectName(obj),
			ScheduleName:    scheduleName,
			SchedulePattern: schedule.DetectedPattern,
			ScheduleHash:    schedule.ContentHash,
			RawMethod:       profileCalculationMethod(obj),
			RawValue:        firstNumericRawValue(obj, "outdoor", "air", "flow"),
			Normalized:      metrics,
			DisplayMetric:   selectProfileMetric(metrics, defaultProfileAnalysisSettings().DisplayMetrics[ProfileDimensionOutdoorAir]),
			CloneEligible:   false,
			Warnings:        warnings,
		})
	}
	return items
}

func (ctx profileContext) missingZoneItem(obj Object, dimension string, zoneName string) ProfileItem {
	warning := profileWarning("warning", "missing_profile_zone", "Profile object references a zone, ZoneList, space, or SpaceList that could not be resolved.", zoneName, dimension, obj)
	return ProfileItem{
		ID:            fmt.Sprintf("profile-item-%d-%s-missing", obj.Index, dimension),
		ZoneName:      zoneName,
		Dimension:     dimension,
		ObjectIndex:   obj.Index,
		ObjectType:    obj.Type,
		ObjectName:    objectName(obj),
		RawMethod:     profileCalculationMethod(obj),
		Normalized:    nil,
		DisplayMetric: ProfileMetric{ID: defaultProfileAnalysisSettings().DisplayMetrics[dimension], DisplayValue: "N/A", Status: summaryStatusMissing},
		Warnings:      []ProfileWarning{warning},
	}
}

func (ctx profileContext) objectTargetZones(obj Object) []string {
	target := profileTargetName(obj)
	if target == "" {
		return nil
	}
	key := normalizeName(target)
	if zones, ok := ctx.zoneLists[key]; ok {
		return cleanProfileNames(zones)
	}
	if spaceNames, ok := ctx.spaceLists[key]; ok {
		var zones []string
		for _, spaceName := range spaceNames {
			if zone := ctx.spaceToZone[normalizeName(spaceName)]; zone != "" {
				zones = append(zones, zone)
			}
		}
		return cleanProfileNames(zones)
	}
	if zone := ctx.spaceToZone[key]; zone != "" {
		return []string{zone}
	}
	return []string{target}
}

func (ctx profileContext) profileTargetKind(target string) string {
	key := normalizeName(target)
	switch {
	case target == "":
		return ""
	case ctx.zoneLists[key] != nil:
		return "ZoneList"
	case ctx.spaceLists[key] != nil:
		return "SpaceList"
	case ctx.spaceToZone[key] != "":
		return "Space"
	default:
		return "Zone"
	}
}

func profileTargetName(obj Object) string {
	if value := findFieldByCommentWords(obj, "zone", "zonelist", "name"); value != "" {
		return value
	}
	if value := findFieldByCommentWords(obj, "space", "spacelist", "name"); value != "" {
		return value
	}
	if value := findFieldByCommentWords(obj, "zone", "name"); value != "" {
		return value
	}
	if value := findFieldByCommentWords(obj, "space", "name"); value != "" {
		return value
	}
	return ""
}

func profileScheduleName(obj Object) string {
	for _, field := range obj.Fields {
		comment := strings.ToLower(field.Comment)
		if strings.Contains(comment, "schedule") && strings.Contains(comment, "name") {
			return strings.TrimSpace(field.Value)
		}
	}
	return ""
}

func profileCalculationMethod(obj Object) string {
	if value := findFieldByCommentWords(obj, "calculation", "method"); value != "" {
		return value
	}
	if value := findFieldByCommentWords(obj, "method"); value != "" {
		return value
	}
	return ""
}

func (ctx profileContext) peopleCountForObject(obj Object, zone profileZoneContext) (float64, bool, []ProfileWarning) {
	method := strings.ToLower(profileCalculationMethod(obj))
	var warnings []ProfileWarning
	switch {
	case strings.Contains(method, "people/area"):
		value, ok := findNumericFieldByCommentWords(obj, "people", "zone", "floor", "area")
		if !ok {
			value, ok = findNumericFieldByCommentWords(obj, "people", "area")
		}
		if zone.floorArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_area", "Zone area is required to normalize People/Area.", zone.name, ProfileDimensionOccupancy, obj))
			return 0, false, warnings
		}
		return value * zone.floorArea, ok, warnings
	case strings.Contains(method, "area/person"):
		value, ok := findNumericFieldByCommentWords(obj, "zone", "floor", "area", "person")
		if !ok || value <= 0 {
			return 0, false, warnings
		}
		if zone.floorArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_area", "Zone area is required to normalize Area/Person.", zone.name, ProfileDimensionOccupancy, obj))
			return 0, false, warnings
		}
		return zone.floorArea / value, true, warnings
	default:
		value, ok := findNumericFieldByCommentWords(obj, "number", "people")
		return value, ok, warnings
	}
}

func (ctx profileContext) designPowerForObject(obj Object, zone profileZoneContext, kind string) (float64, bool, []ProfileWarning) {
	method := strings.ToLower(profileCalculationMethod(obj))
	dimension := ProfileDimensionEquipment
	if kind == "lighting" {
		dimension = ProfileDimensionLighting
	}
	var warnings []ProfileWarning
	switch {
	case strings.Contains(method, "watts/area"):
		value, ok := findNumericFieldByCommentWords(obj, "watts", "zone", "floor", "area")
		if !ok {
			value, ok = findNumericFieldByCommentWords(obj, "watts", "area")
		}
		if zone.floorArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_area", "Zone area is required to normalize Watts/Area.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * zone.floorArea, ok, warnings
	case strings.Contains(method, "watts/person"):
		value, ok := findNumericFieldByCommentWords(obj, "watts", "person")
		people := ctx.peopleCount[normalizeName(zone.name)]
		if people <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_people_reference", "People count is required to normalize Watts/Person.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * people, ok, warnings
	default:
		if kind == "lighting" {
			value, ok := findNumericFieldByCommentWords(obj, "lighting", "level")
			return value, ok, warnings
		}
		value, ok := findNumericFieldByCommentWords(obj, "design", "level")
		return value, ok, warnings
	}
}

func (ctx profileContext) airflowForObject(obj Object, zone profileZoneContext, usePeople bool) (float64, bool, []ProfileWarning) {
	method := strings.ToLower(profileCalculationMethod(obj))
	dimension := ProfileDimensionInfiltration
	if usePeople {
		dimension = ProfileDimensionVentilation
	}
	var warnings []ProfileWarning
	switch {
	case strings.Contains(method, "flow/person"):
		value, ok := findNumericFieldByCommentWords(obj, "flow", "person")
		people := ctx.peopleCount[normalizeName(zone.name)]
		if people <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_people_reference", "People count is required to normalize flow/person.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * people, ok, warnings
	case strings.Contains(method, "flow/area") || strings.Contains(method, "flow per zone floor area"):
		value, ok := findNumericFieldByCommentWords(obj, "flow", "zone", "floor", "area")
		if !ok {
			value, ok = findNumericFieldByCommentWords(obj, "flow", "area")
		}
		if zone.floorArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_area", "Zone area is required to normalize flow/area.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * zone.floorArea, ok, warnings
	case strings.Contains(method, "flow/exterior") || strings.Contains(method, "exterior"):
		value, ok := findNumericFieldByCommentWords(obj, "flow", "exterior", "surface", "area")
		if zone.exteriorArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_exterior_area", "Exterior area is required to normalize flow/exterior area.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * zone.exteriorArea, ok, warnings
	case strings.Contains(method, "airchanges") || strings.Contains(method, "air changes"):
		value, ok := findNumericFieldByCommentWords(obj, "air", "changes", "hour")
		if zone.volume <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_volume", "Zone volume is required to normalize ACH.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * zone.volume / 3600, ok, warnings
	default:
		value, ok := findNumericFieldByCommentWords(obj, "design", "flow", "rate")
		if !ok {
			value, ok = findNumericFieldByCommentWords(obj, "flow", "rate")
		}
		return value, ok, warnings
	}
}

func (ctx profileContext) outdoorAirFlowForObject(obj Object, zone profileZoneContext) (float64, bool, []ProfileWarning) {
	method := strings.ToLower(profileCalculationMethod(obj))
	var warnings []ProfileWarning
	switch {
	case strings.Contains(method, "flow/person"):
		value, ok := findNumericFieldByCommentWords(obj, "outdoor", "air", "flow", "person")
		people := ctx.peopleCount[normalizeName(zone.name)]
		if people <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_people_reference", "People count is required to normalize outdoor air flow/person.", zone.name, ProfileDimensionOutdoorAir, obj))
			return 0, false, warnings
		}
		return value * people, ok, warnings
	case strings.Contains(method, "flow/area"):
		value, ok := findNumericFieldByCommentWords(obj, "outdoor", "air", "flow", "floor", "area")
		if zone.floorArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_area", "Zone area is required to normalize outdoor air flow/area.", zone.name, ProfileDimensionOutdoorAir, obj))
			return 0, false, warnings
		}
		return value * zone.floorArea, ok, warnings
	case strings.Contains(method, "airchanges") || strings.Contains(method, "air changes"):
		value, ok := findNumericFieldByCommentWords(obj, "outdoor", "air", "changes", "hour")
		if !ok {
			value, ok = findNumericFieldByCommentWords(obj, "air", "changes", "hour")
		}
		if zone.volume <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_volume", "Zone volume is required to normalize outdoor air ACH.", zone.name, ProfileDimensionOutdoorAir, obj))
			return 0, false, warnings
		}
		return value * zone.volume / 3600, ok, warnings
	default:
		value, ok := findNumericFieldByCommentWords(obj, "outdoor", "air", "flow", "zone")
		if !ok {
			value, ok = findNumericFieldByCommentWords(obj, "flow", "zone")
		}
		return value, ok, warnings
	}
}

func (ctx profileContext) outdoorAirTargetZones(outdoorAirName string) []string {
	if strings.TrimSpace(outdoorAirName) == "" {
		return nil
	}
	var zones []string
	for _, obj := range ctx.doc.Objects {
		if !strings.EqualFold(obj.Type, "Sizing:Zone") {
			continue
		}
		ref := findFieldByCommentWords(obj, "design", "specification", "outdoor", "air")
		if !strings.EqualFold(strings.TrimSpace(ref), strings.TrimSpace(outdoorAirName)) {
			continue
		}
		zones = append(zones, ctx.objectTargetZones(obj)...)
	}
	return cleanProfileNames(zones)
}

func profileAirflowMetrics(flow float64, ok bool, zone profileZoneContext, peopleCount float64, includePeople bool) []ProfileMetric {
	metrics := []ProfileMetric{
		profileMetric("flow", "Flow", "m3/s", flow, ok, 4),
		profileMetric("flow_per_area", "Flow per floor area", "m3/s-m2", divide(flow, zone.floorArea), ok && zone.floorArea > 0, 6),
		profileMetric("ach", "Air changes", "ACH", divide(flow*3600, zone.volume), ok && zone.volume > 0, 3),
	}
	if includePeople {
		metrics = append(metrics, profileMetric("flow_per_person", "Flow per person", "m3/s-person", divide(flow, peopleCount), ok && peopleCount > 0, 6))
	} else {
		metrics = append(metrics, profileMetric("flow_per_exterior_area", "Flow per exterior area", "m3/s-m2", divide(flow, zone.exteriorArea), ok && zone.exteriorArea > 0, 6))
	}
	return metrics
}

func summarizeProfileDimensions(items []ProfileItem, displayMetrics map[string]string) []ProfileDimensionSummary {
	byDimension := map[string][]ProfileItem{}
	for _, item := range items {
		byDimension[item.Dimension] = append(byDimension[item.Dimension], item)
	}
	var out []ProfileDimensionSummary
	for _, option := range profileDimensionOptions() {
		dimensionItems := byDimension[option.ID]
		if len(dimensionItems) == 0 {
			continue
		}
		metricID := displayMetrics[option.ID]
		sum := 0.0
		okCount := 0
		var itemIDs []string
		var scheduleNames []string
		var schedulePatterns []string
		var scheduleHashes []string
		var warnings []ProfileWarning
		var metricLabel, unit string
		for _, item := range dimensionItems {
			metric := selectProfileMetric(item.Normalized, metricID)
			if metric.Status == summaryStatusOK || metric.Status == summaryStatusPartial {
				sum += metric.Value
				okCount++
				metricLabel = metric.Label
				unit = metric.Unit
			}
			itemIDs = append(itemIDs, item.ID)
			scheduleNames = appendUniqueString(scheduleNames, item.ScheduleName)
			schedulePatterns = appendUniqueString(schedulePatterns, item.SchedulePattern)
			scheduleHashes = appendUniqueString(scheduleHashes, item.ScheduleHash)
			warnings = append(warnings, item.Warnings...)
		}
		status := summaryStatusOK
		if okCount == 0 {
			status = summaryStatusMissing
		} else if okCount < len(dimensionItems) {
			status = summaryStatusPartial
		}
		if metricLabel == "" {
			metricLabel = profileMetricLabel(option.ID, metricID)
			unit = profileMetricUnit(option.ID, metricID)
		}
		out = append(out, ProfileDimensionSummary{
			Dimension:       option.ID,
			Label:           option.Label,
			MetricID:        metricID,
			MetricLabel:     metricLabel,
			Unit:            unit,
			Value:           roundedNumber(sum, profileMetricPrecision(metricID)),
			DisplayValue:    profileMetricDisplay(sum, unit, status, profileMetricPrecision(metricID)),
			Status:          status,
			ScheduleName:    strings.Join(nonEmptyStrings(scheduleNames), " + "),
			SchedulePattern: strings.Join(nonEmptyStrings(schedulePatterns), " + "),
			ScheduleHash:    strings.Join(nonEmptyStrings(scheduleHashes), "+"),
			ItemIDs:         itemIDs,
			ItemCount:       len(dimensionItems),
			Warnings:        warnings,
		})
	}
	return out
}

func buildDefaultProfileGroups(zones []ZoneProfile, settings ProfileAnalysisSettings) []ProfileGroup {
	type groupState struct {
		group ProfileGroup
	}
	groupsByKey := map[string]*groupState{}
	for _, zone := range zones {
		key := profileGroupKey(zone.Dimensions, settings)
		if key == "" {
			key = "empty"
		}
		state := groupsByKey[key]
		if state == nil {
			state = &groupState{group: ProfileGroup{
				ID:         "profile-group-" + strconv.Itoa(len(groupsByKey)+1),
				Key:        key,
				Dimensions: zone.Dimensions,
			}}
			groupsByKey[key] = state
		}
		state.group.ZoneNames = append(state.group.ZoneNames, zone.ZoneName)
		state.group.ItemIDs = append(state.group.ItemIDs, profileItemIDs(zone.Items)...)
		state.group.Warnings = append(state.group.Warnings, zone.Warnings...)
	}

	var groups []ProfileGroup
	for _, state := range groupsByKey {
		state.group.ZoneCount = len(state.group.ZoneNames)
		state.group.Name = fmt.Sprintf("Profile %s", strings.TrimPrefix(state.group.ID, "profile-group-"))
		groups = append(groups, state.group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].ZoneCount == groups[j].ZoneCount {
			return groups[i].ID < groups[j].ID
		}
		return groups[i].ZoneCount > groups[j].ZoneCount
	})
	for index := range groups {
		groups[index].ID = "profile-group-" + strconv.Itoa(index+1)
		groups[index].Name = fmt.Sprintf("Profile %s", string(rune('A'+index%26)))
	}
	return groups
}

func profileGroupKey(dimensions []ProfileDimensionSummary, settings ProfileAnalysisSettings) string {
	var parts []string
	enabled := stringSet(settings.EnabledDimensions)
	for _, dimension := range dimensions {
		if len(enabled) > 0 && !enabled[dimension.Dimension] {
			continue
		}
		metricID := settings.GroupingMetrics[dimension.Dimension]
		if metricID == "" {
			metricID = dimension.MetricID
		}
		tolerance := settings.NumericTolerance
		if tolerance <= 0 {
			tolerance = 0.001
		}
		valueBucket := math.Round(dimension.Value/tolerance) * tolerance
		schedulePart := ""
		switch settings.ScheduleCompareMode {
		case "none":
			schedulePart = ""
		case "resolved":
			schedulePart = dimension.ScheduleHash
		default:
			schedulePart = dimension.ScheduleName
		}
		parts = append(parts, fmt.Sprintf("%s:%s:%.6f:%s", dimension.Dimension, metricID, valueBucket, schedulePart))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func zoneProfileWarnings(zoneName string, items []ProfileItem) []ProfileWarning {
	counts := map[string]int{}
	var warnings []ProfileWarning
	for _, item := range items {
		counts[item.Dimension]++
	}
	for dimension, count := range counts {
		if count > 1 {
			warnings = append(warnings, ProfileWarning{
				Severity:  "warning",
				Code:      "multiple_profile_items",
				Message:   fmt.Sprintf("%d %s profile items are assigned to the same zone.", count, profileDimensionLabel(dimension)),
				ZoneName:  zoneName,
				Dimension: dimension,
			})
		}
	}
	return warnings
}

func summarizeSchedule(obj Object) ScheduleSummary {
	summary := ScheduleSummary{
		ScheduleName: objectName(obj),
		ScheduleType: obj.Type,
		ObjectIndex:  obj.Index,
	}
	switch {
	case strings.EqualFold(obj.Type, "Schedule:Constant"):
		value, ok := scheduleConstantValue(obj)
		if !ok {
			summary.DetectedPattern = "irregular"
			summary.Warnings = append(summary.Warnings, "Constant schedule value could not be parsed; profile graph uses a design-level fallback.")
			value = 1
		}
		summary.Resolved = ok
		summary.WeekdayProfile = filledProfile(value)
		summary.SaturdayProfile = filledProfile(value)
		summary.SundayProfile = filledProfile(value)
		summary.HolidayProfile = filledProfile(value)
		summary.WeeklyProfile = weeklyProfileFromDayProfiles(summary.WeekdayProfile, summary.SaturdayProfile, summary.SundayProfile)
		summary.Rules = []ScheduleRule{constantScheduleRule(value)}
		summary.AnnualStats = annualStatsFromProfiles(summary.WeekdayProfile, summary.SaturdayProfile, summary.SundayProfile)
	case strings.EqualFold(obj.Type, "Schedule:Compact"):
		rules, ok := compactScheduleRules(obj)
		if !ok {
			summary.DetectedPattern = "irregular"
			summary.Warnings = append(summary.Warnings, "Compact schedule could not be reduced; profile graph uses a design-level fallback.")
			summary.WeekdayProfile = filledProfile(1)
			summary.SaturdayProfile = filledProfile(1)
			summary.SundayProfile = filledProfile(1)
			summary.HolidayProfile = filledProfile(1)
			summary.WeeklyProfile = weeklyProfileFromDayProfiles(summary.WeekdayProfile, summary.SaturdayProfile, summary.SundayProfile)
			summary.Rules = []ScheduleRule{constantScheduleRule(1)}
			summary.AnnualStats = annualStatsFromProfiles(summary.WeekdayProfile, summary.SaturdayProfile, summary.SundayProfile)
			break
		}
		summary.Resolved = true
		summary.WeekdayProfile = representativeProfileForSelector(rules, "Weekdays")
		summary.SaturdayProfile = representativeProfileForSelector(rules, "Saturday")
		summary.SundayProfile = representativeProfileForSelector(rules, "Sunday")
		summary.HolidayProfile = representativeProfileForSelector(rules, "AllDays")
		summary.WeeklyProfile = weeklyProfileFromRules(rules)
		summary.Rules = scheduleRulesFromCompact(rules)
		summary.AnnualStats = annualStatsFromRules(rules)
		if compactRulesAreSeasonal(rules) {
			summary.Warnings = append(summary.Warnings, "Schedule has seasonal rule changes; representative days are a simplification.")
		}
	default:
		summary.DetectedPattern = "irregular"
		summary.Warnings = append(summary.Warnings, "Schedule type is not yet parsed; profile graph uses a design-level fallback.")
		summary.WeekdayProfile = filledProfile(1)
		summary.SaturdayProfile = filledProfile(1)
		summary.SundayProfile = filledProfile(1)
		summary.HolidayProfile = filledProfile(1)
		summary.WeeklyProfile = weeklyProfileFromDayProfiles(summary.WeekdayProfile, summary.SaturdayProfile, summary.SundayProfile)
		summary.Rules = []ScheduleRule{constantScheduleRule(1)}
		summary.AnnualStats = annualStatsFromProfiles(summary.WeekdayProfile, summary.SaturdayProfile, summary.SundayProfile)
	}
	if summary.DetectedPattern == "" {
		summary.DetectedPattern = detectSchedulePattern(summary)
	}
	summary.ContentHash = scheduleContentHash(summary)
	return summary
}

func scheduleConstantValue(obj Object) (float64, bool) {
	if value := findFieldByCommentWords(obj, "hourly", "value"); value != "" {
		return parseFloatField(value)
	}
	if len(obj.Fields) >= 3 {
		return parseFloatField(obj.Fields[2].Value)
	}
	if len(obj.Fields) >= 2 {
		return parseFloatField(obj.Fields[len(obj.Fields)-1].Value)
	}
	return 0, false
}

func compactScheduleRules(obj Object) ([]compactScheduleRule, bool) {
	if len(obj.Fields) <= 2 {
		return nil, false
	}
	var rules []compactScheduleRule
	periodStart := 1
	periodEnd := 365
	previousThrough := 0
	for index := 2; index < len(obj.Fields); {
		value := strings.TrimSpace(obj.Fields[index].Value)
		lower := strings.ToLower(value)
		switch {
		case strings.HasPrefix(lower, "through:"):
			day, ok := parseMonthDay(strings.TrimSpace(value[len("through:"):]))
			if !ok {
				return nil, false
			}
			periodStart = previousThrough + 1
			periodEnd = day
			previousThrough = day
			index++
		case strings.HasPrefix(lower, "for:"):
			selector := strings.TrimSpace(value[len("for:"):])
			intervals, next, ok := parseCompactIntervals(obj.Fields, index+1)
			if !ok || !recognizedDaySelector(selector) {
				return nil, false
			}
			rules = append(rules, compactScheduleRule{
				startDay:  periodStart,
				endDay:    periodEnd,
				selector:  selector,
				intervals: intervals,
			})
			index = next
		default:
			index++
		}
	}
	return rules, len(rules) > 0
}

func representativeProfileForSelector(rules []compactScheduleRule, selector string) []float64 {
	targetDay := 1
	switch strings.ToLower(selector) {
	case "saturday":
		targetDay = 6
	case "sunday":
		targetDay = 7
	default:
		targetDay = 1
	}
	for _, rule := range rules {
		if targetDay < rule.startDay || targetDay > rule.endDay || !dayMatchesSelector(targetDay, rule.selector) {
			continue
		}
		return profileFromIntervals(rule.intervals)
	}
	for _, rule := range rules {
		if dayMatchesSelector(targetDay, rule.selector) {
			return profileFromIntervals(rule.intervals)
		}
	}
	return filledProfile(0)
}

func weeklyProfileFromRules(rules []compactScheduleRule) []float64 {
	var out []float64
	for day := 1; day <= 7; day++ {
		out = append(out, profileForScheduleDay(rules, day)...)
	}
	return out
}

func weeklyProfileFromDayProfiles(weekday []float64, saturday []float64, sunday []float64) []float64 {
	var out []float64
	for day := 1; day <= 7; day++ {
		switch day {
		case 6:
			out = append(out, saturday...)
		case 7:
			out = append(out, sunday...)
		default:
			out = append(out, weekday...)
		}
	}
	return out
}

func profileForScheduleDay(rules []compactScheduleRule, day int) []float64 {
	for _, rule := range rules {
		if day < rule.startDay || day > rule.endDay || !dayMatchesSelector(day, rule.selector) {
			continue
		}
		return profileFromIntervals(rule.intervals)
	}
	return filledProfile(0)
}

func profileFromIntervals(intervals []scheduleInterval) []float64 {
	profile := make([]float64, 24)
	previous := 0.0
	for _, interval := range intervals {
		end := previous + interval.hours
		startHour := int(math.Floor(previous))
		endHour := int(math.Ceil(end))
		for hour := startHour; hour < endHour && hour < 24; hour++ {
			if hour >= 0 {
				profile[hour] = interval.value
			}
		}
		previous = end
	}
	return roundedProfile(profile)
}

func scheduleRulesFromCompact(rules []compactScheduleRule) []ScheduleRule {
	out := make([]ScheduleRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, ScheduleRule{
			StartDay:  rule.startDay,
			EndDay:    rule.endDay,
			Through:   dayOfYearLabel(rule.endDay),
			Selector:  rule.selector,
			Label:     fmt.Sprintf("Through %s / For %s", dayOfYearLabel(rule.endDay), strings.TrimSpace(rule.selector)),
			Intervals: scheduleIntervalsFromCompact(rule.intervals),
		})
	}
	return out
}

func scheduleIntervalsFromCompact(intervals []scheduleInterval) []ScheduleInterval {
	out := make([]ScheduleInterval, 0, len(intervals))
	start := 0.0
	for _, interval := range intervals {
		end := start + interval.hours
		out = append(out, ScheduleInterval{
			StartHour: roundedNumber(start, 2),
			EndHour:   roundedNumber(end, 2),
			Value:     roundedNumber(interval.value, 4),
			Label:     fmt.Sprintf("%s-%s", hourLabel(start), hourLabel(end)),
		})
		start = end
	}
	return out
}

func constantScheduleRule(value float64) ScheduleRule {
	return ScheduleRule{
		StartDay: 1,
		EndDay:   365,
		Through:  "12/31",
		Selector: "AllDays",
		Label:    "Through 12/31 / For AllDays",
		Intervals: []ScheduleInterval{{
			StartHour: 0,
			EndHour:   24,
			Value:     roundedNumber(value, 4),
			Label:     "00:00-24:00",
		}},
	}
}

func dayOfYearLabel(day int) string {
	daysByMonth := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if day < 1 {
		day = 1
	}
	if day > 365 {
		day = 365
	}
	month := 1
	for _, days := range daysByMonth {
		if day <= days {
			break
		}
		day -= days
		month++
	}
	return fmt.Sprintf("%02d/%02d", month, day)
}

func hourLabel(hour float64) string {
	wholeHour := int(math.Floor(hour))
	minute := int(math.Round((hour - float64(wholeHour)) * 60))
	if minute == 60 {
		wholeHour++
		minute = 0
	}
	if wholeHour > 24 {
		wholeHour = 24
		minute = 0
	}
	return fmt.Sprintf("%02d:%02d", wholeHour, minute)
}

func filledProfile(value float64) []float64 {
	profile := make([]float64, 24)
	for i := range profile {
		profile[i] = roundedNumber(value, 4)
	}
	return profile
}

func annualStatsFromRules(rules []compactScheduleRule) ScheduleAnnualStats {
	var values []float64
	for day := 1; day <= 365; day++ {
		profile := filledProfile(0)
		for _, rule := range rules {
			if day >= rule.startDay && day <= rule.endDay && dayMatchesSelector(day, rule.selector) {
				profile = profileFromIntervals(rule.intervals)
				break
			}
		}
		values = append(values, profile...)
	}
	return annualStatsFromValues(values)
}

func annualStatsFromProfiles(weekday []float64, saturday []float64, sunday []float64) ScheduleAnnualStats {
	var values []float64
	for day := 1; day <= 365; day++ {
		switch (day - 1) % 7 {
		case 5:
			values = append(values, saturday...)
		case 6:
			values = append(values, sunday...)
		default:
			values = append(values, weekday...)
		}
	}
	return annualStatsFromValues(values)
}

func annualStatsFromValues(values []float64) ScheduleAnnualStats {
	if len(values) == 0 {
		return ScheduleAnnualStats{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	var total, maxValue, operating, aboveHalf float64
	for _, value := range values {
		total += value
		maxValue = math.Max(maxValue, value)
		if value > 0 {
			operating++
		}
		if value >= 0.5 {
			aboveHalf++
		}
	}
	p95Index := int(math.Ceil(float64(len(sorted))*0.95)) - 1
	if p95Index < 0 {
		p95Index = 0
	}
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	return ScheduleAnnualStats{
		Average:             roundedNumber(total/float64(len(values)), 4),
		Max:                 roundedNumber(maxValue, 4),
		P95:                 roundedNumber(sorted[p95Index], 4),
		OperatingHours:      roundedNumber(operating, 1),
		AboveHalfHours:      roundedNumber(aboveHalf, 1),
		EquivalentFullHours: roundedNumber(total, 1),
	}
}

func detectSchedulePattern(summary ScheduleSummary) string {
	if profileAlways(summary.WeekdayProfile, 1) && profileAlways(summary.SaturdayProfile, 1) && profileAlways(summary.SundayProfile, 1) {
		return "always_on"
	}
	if profileAlways(summary.WeekdayProfile, 0) && profileAlways(summary.SaturdayProfile, 0) && profileAlways(summary.SundayProfile, 0) {
		return "always_off"
	}
	weekdayStart, weekdayEnd, weekdayOK := activeRange(summary.WeekdayProfile)
	satStart, satEnd, satOK := activeRange(summary.SaturdayProfile)
	sundayActive := profileActiveHours(summary.SundayProfile) > 0
	if weekdayOK && weekdayStart == 9 && weekdayEnd == 18 && !sundayActive {
		if satOK && satStart == 9 && satEnd == 15 {
			return "weekday_9_to_6_sat_9_to_3"
		}
		if !satOK {
			return "weekday_9_to_6"
		}
	}
	if profileNightHours(summary.WeekdayProfile)+profileNightHours(summary.SaturdayProfile)+profileNightHours(summary.SundayProfile) > 24 {
		return "night_operation"
	}
	if profileActiveHours(summary.SaturdayProfile)+profileActiveHours(summary.SundayProfile) > profileActiveHours(summary.WeekdayProfile)*0.8 {
		return "weekend_operation"
	}
	if len(summary.Warnings) > 0 {
		return "seasonal_operation"
	}
	return "regular_operation"
}

func scheduleContentHash(summary ScheduleSummary) string {
	var b strings.Builder
	for _, profile := range [][]float64{summary.WeekdayProfile, summary.SaturdayProfile, summary.SundayProfile, summary.HolidayProfile} {
		for _, value := range profile {
			fmt.Fprintf(&b, "%.4f,", value)
		}
		b.WriteByte('|')
	}
	sum := sha1.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:12]
}

func compactRulesAreSeasonal(rules []compactScheduleRule) bool {
	if len(rules) == 0 {
		return false
	}
	for _, rule := range rules {
		if rule.startDay != 1 || rule.endDay != 365 {
			return true
		}
	}
	return false
}

func profileMetric(id, label, unit string, value float64, ok bool, precision int) ProfileMetric {
	status := summaryStatusOK
	display := profileMetricDisplay(value, unit, status, precision)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		status = summaryStatusMissing
		value = 0
		display = "N/A"
	}
	return ProfileMetric{
		ID:           id,
		Label:        label,
		Unit:         unit,
		Value:        roundedNumber(value, precision),
		DisplayValue: display,
		Status:       status,
	}
}

func profileMetricDisplay(value float64, unit string, status string, precision int) string {
	if status == summaryStatusMissing || math.IsNaN(value) || math.IsInf(value, 0) {
		return "N/A"
	}
	display := formatSummaryNumber(value, precision)
	if unit != "" {
		display += " " + unit
	}
	return display
}

func selectProfileMetric(metrics []ProfileMetric, id string) ProfileMetric {
	for _, metric := range metrics {
		if metric.ID == id {
			return metric
		}
	}
	return ProfileMetric{ID: id, Label: profileMetricLabel("", id), DisplayValue: "N/A", Status: summaryStatusMissing}
}

func profileMetricLabel(dimension, metricID string) string {
	for _, option := range profileMetricOptions() {
		if option.ID == metricID && (dimension == "" || option.Dimension == dimension) {
			return option.Label
		}
	}
	return metricID
}

func profileMetricUnit(dimension, metricID string) string {
	for _, option := range profileMetricOptions() {
		if option.ID == metricID && (dimension == "" || option.Dimension == dimension) {
			return option.Unit
		}
	}
	return ""
}

func profileMetricPrecision(metricID string) int {
	switch metricID {
	case "count", "total_power":
		return 2
	case "flow", "flow_per_area", "flow_per_exterior_area", "flow_per_person":
		return 6
	case "people_per_area", "power_per_area", "ach":
		return 3
	default:
		return 2
	}
}

func profileDimensionLabel(dimension string) string {
	for _, option := range profileDimensionOptions() {
		if option.ID == dimension {
			return option.Label
		}
	}
	return dimension
}

func profileWarning(severity, code, message, zoneName, dimension string, obj Object) ProfileWarning {
	return ProfileWarning{
		Severity:    severity,
		Code:        code,
		Message:     message,
		ZoneName:    zoneName,
		Dimension:   dimension,
		ObjectIndex: obj.Index,
		ObjectType:  obj.Type,
		ObjectName:  objectName(obj),
	}
}

func profileScheduleList(items map[string]ScheduleSummary) []ScheduleSummary {
	out := make([]ScheduleSummary, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ScheduleName < out[j].ScheduleName
	})
	return out
}

func firstNumericRawValue(obj Object, words ...string) string {
	for _, field := range obj.Fields {
		comment := strings.ToLower(field.Comment)
		matched := true
		for _, word := range words {
			if !strings.Contains(comment, strings.ToLower(word)) {
				matched = false
				break
			}
		}
		if matched {
			return strings.TrimSpace(field.Value)
		}
	}
	return ""
}

func divide(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func cleanProfileNames(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := normalizeName(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func nonEmptyStrings(values []string) []string {
	var out []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func profileItemIDs(items []ProfileItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func roundedProfile(values []float64) []float64 {
	out := make([]float64, len(values))
	for i, value := range values {
		out[i] = roundedNumber(value, 4)
	}
	return out
}

func profileAlways(profile []float64, want float64) bool {
	for _, value := range profile {
		if math.Abs(value-want) > 0.0001 {
			return false
		}
	}
	return len(profile) > 0
}

func profileActiveHours(profile []float64) float64 {
	var hours float64
	for _, value := range profile {
		if value > 0 {
			hours++
		}
	}
	return hours
}

func profileNightHours(profile []float64) float64 {
	var hours float64
	for hour, value := range profile {
		if value > 0 && (hour < 6 || hour >= 20) {
			hours++
		}
	}
	return hours
}

func activeRange(profile []float64) (int, int, bool) {
	start := -1
	end := -1
	for hour, value := range profile {
		if value < 0.5 {
			continue
		}
		if start < 0 {
			start = hour
		}
		end = hour + 1
	}
	return start, end, start >= 0
}

func safeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "item"
	}
	return out
}
