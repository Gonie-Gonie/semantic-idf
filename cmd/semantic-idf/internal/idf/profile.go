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
	ZoneCount           int                         `json:"zoneCount"`
	ItemCount           int                         `json:"itemCount"`
	GroupCount          int                         `json:"groupCount"`
	Dimensions          []ProfileDimensionOption    `json:"dimensions"`
	MetricOptions       []ProfileMetricOption       `json:"metricOptions"`
	ZoneProfiles        []ZoneProfile               `json:"zoneProfiles"`
	Groups              []ProfileGroup              `json:"groups"`
	Matrix              []ProfileMatrixRow          `json:"matrix"`
	Schedules           []ScheduleSummary           `json:"schedules"`
	GraphDataset        ProfileGraphDataset         `json:"graphDataset"`
	ScheduleClusters    []ProfileScheduleCluster    `json:"scheduleClusters"`
	Outliers            []ProfileOutlierHint        `json:"outliers"`
	ParameterCandidates []ProfileParameterCandidate `json:"parameterCandidates"`
	Warnings            []ProfileWarning            `json:"warnings"`
	DefaultSettings     ProfileAnalysisSettings     `json:"defaultSettings"`
}

type ProfileAnalysisSettings struct {
	EnabledDimensions   []string             `json:"enabledDimensions"`
	DisplayMetrics      map[string]string    `json:"displayMetrics"`
	GroupingMetrics     map[string]string    `json:"groupingMetrics"`
	NumericTolerance    float64              `json:"numericTolerance"`
	ScheduleCompareMode string               `json:"scheduleCompareMode"`
	TimeView            string               `json:"timeView"`
	ScaleMode           string               `json:"scaleMode"`
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
	ZoneName         string                    `json:"zoneName"`
	ZoneObjectIndex  int                       `json:"zoneObjectIndex"`
	FloorArea        float64                   `json:"floorArea"`
	Volume           float64                   `json:"volume"`
	ExteriorArea     float64                   `json:"exteriorArea"`
	ExteriorWallArea float64                   `json:"exteriorWallArea"`
	Items            []ProfileItem             `json:"items"`
	Dimensions       []ProfileDimensionSummary `json:"dimensions"`
	Warnings         []ProfileWarning          `json:"warnings,omitempty"`
}

type ProfileItem struct {
	ID                   string           `json:"id"`
	ZoneName             string           `json:"zoneName"`
	Dimension            string           `json:"dimension"`
	ObjectIndex          int              `json:"objectIndex"`
	ObjectType           string           `json:"objectType"`
	ObjectName           string           `json:"objectName,omitempty"`
	SourceTarget         string           `json:"sourceTarget,omitempty"`
	SourceTargetKind     string           `json:"sourceTargetKind,omitempty"`
	ScheduleName         string           `json:"scheduleName,omitempty"`
	SchedulePattern      string           `json:"schedulePattern,omitempty"`
	ScheduleHash         string           `json:"scheduleHash,omitempty"`
	RawMethod            string           `json:"rawMethod,omitempty"`
	RawValue             string           `json:"rawValue,omitempty"`
	AggregationSignature string           `json:"aggregationSignature,omitempty"`
	Normalized           []ProfileMetric  `json:"normalized"`
	DisplayMetric        ProfileMetric    `json:"displayMetric"`
	CloneEligible        bool             `json:"cloneEligible"`
	Warnings             []ProfileWarning `json:"warnings,omitempty"`
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
	Dimension             string           `json:"dimension"`
	Label                 string           `json:"label"`
	MetricID              string           `json:"metricId"`
	MetricLabel           string           `json:"metricLabel"`
	Unit                  string           `json:"unit,omitempty"`
	Value                 float64          `json:"value"`
	DisplayValue          string           `json:"displayValue"`
	Status                string           `json:"status"`
	ScheduleName          string           `json:"scheduleName,omitempty"`
	SchedulePattern       string           `json:"schedulePattern,omitempty"`
	ScheduleHash          string           `json:"scheduleHash,omitempty"`
	ContributionSignature string           `json:"contributionSignature,omitempty"`
	ItemIDs               []string         `json:"itemIds"`
	ItemCount             int              `json:"itemCount"`
	ResolvedItemCount     int              `json:"resolvedItemCount"`
	FallbackMetric        bool             `json:"fallbackMetric,omitempty"`
	Warnings              []ProfileWarning `json:"warnings,omitempty"`

	contributionSignatures map[string]string
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
	name                string
	objectIndex         int
	floorArea           float64
	calculatedFloorArea float64
	hasDeclaredArea     bool
	volume              float64
	hasDeclaredVolume   bool
	ceilingHeight       float64
	exteriorArea        float64
	exteriorWallArea    float64
}

type profileSpaceContext struct {
	name              string
	zoneName          string
	ceilingHeight     float64
	floorArea         float64
	hasDeclaredArea   bool
	volume            float64
	hasDeclaredVolume bool
	exteriorArea      float64
	exteriorWallArea  float64
}

// profileTargetBasis is the portion of a representative zone addressed by one
// input object. SpaceList direct levels are instantiated once per listed Space,
// while area-based methods use the summed floor area of those Spaces.
type profileTargetBasis struct {
	floorArea        float64
	volume           float64
	exteriorArea     float64
	exteriorWallArea float64
	instanceCount    int
	spaceNames       []string
	spaceScoped      bool
}

type profileContext struct {
	doc                   Document
	zones                 []profileZoneContext
	zoneByKey             map[string]profileZoneContext
	zoneLists             map[string][]string
	spaceToZone           map[string]string
	spaceByKey            map[string]profileSpaceContext
	spacesByZone          map[string][]profileSpaceContext
	spaceLists            map[string][]string
	scheduleByKey         map[string]ScheduleSummary
	peopleDensity         map[string]float64
	peopleCount           map[string]float64
	peopleBySpace         map[string]float64
	peopleResolved        map[string]bool
	peopleResolvedBySpace map[string]bool
	peopleInvalid         map[string]bool
	peopleInvalidBySpace  map[string]bool
	implicitRemainder     map[string]bool
	outdoorAirOwners      map[string][]string
	warnings              []ProfileWarning
}

func AnalyzeProfile(doc Document) ProfileReport {
	return analyzeProfileWithGeometry(doc, AnalyzeGeometry(doc))
}

func analyzeProfileWithGeometry(doc Document, geometry GeometryReport) ProfileReport {
	ctx := newProfileContextWithGeometry(doc, geometry)
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
	for zoneKey, items := range zoneItems {
		if _, ok := ctx.zoneByKey[zoneKey]; ok {
			continue
		}
		for _, item := range items {
			report.Warnings = append(report.Warnings, item.Warnings...)
		}
	}

	for _, zone := range ctx.zones {
		items := zoneItems[normalizeName(zone.name)]
		dimensions := summarizeProfileDimensions(items, report.DefaultSettings, zone)
		warnings := zoneProfileWarnings(zone.name, items)
		report.ZoneProfiles = append(report.ZoneProfiles, ZoneProfile{
			ZoneName:         zone.name,
			ZoneObjectIndex:  zone.objectIndex,
			FloorArea:        zone.floorArea,
			Volume:           zone.volume,
			ExteriorArea:     zone.exteriorArea,
			ExteriorWallArea: zone.exteriorWallArea,
			Items:            items,
			Dimensions:       dimensions,
			Warnings:         warnings,
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
	enrichProfileGraphDataset(&report)
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
		TimeView:            "year",
		ScaleMode:           "auto",
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
		{Dimension: ProfileDimensionInfiltration, ID: "flow_per_exterior_wall_area", Label: "Flow per exterior wall area", Unit: "m3/s-m2"},
		{Dimension: ProfileDimensionInfiltration, ID: "ach", Label: "Air changes", Unit: "ACH"},
		{Dimension: ProfileDimensionInfiltration, ID: "effective_leakage_area", Label: "Effective leakage area", Unit: "cm2"},
		{Dimension: ProfileDimensionInfiltration, ID: "flow_coefficient", Label: "Flow coefficient", Unit: "m3/s-Pa^n"},
		{Dimension: ProfileDimensionVentilation, ID: "flow", Label: "Flow", Unit: "m3/s"},
		{Dimension: ProfileDimensionVentilation, ID: "flow_per_person", Label: "Flow per person", Unit: "m3/s-person"},
		{Dimension: ProfileDimensionVentilation, ID: "flow_per_area", Label: "Flow per floor area", Unit: "m3/s-m2"},
		{Dimension: ProfileDimensionVentilation, ID: "ach", Label: "Air changes", Unit: "ACH"},
		{Dimension: ProfileDimensionVentilation, ID: "opening_area", Label: "Opening area", Unit: "m2"},
		{Dimension: ProfileDimensionOutdoorAir, ID: "flow", Label: "Flow", Unit: "m3/s"},
		{Dimension: ProfileDimensionOutdoorAir, ID: "flow_per_person", Label: "Flow per person", Unit: "m3/s-person"},
		{Dimension: ProfileDimensionOutdoorAir, ID: "flow_per_area", Label: "Flow per floor area", Unit: "m3/s-m2"},
		{Dimension: ProfileDimensionOutdoorAir, ID: "ach", Label: "Air changes", Unit: "ACH"},
	}
}

func newProfileContextWithGeometry(doc Document, geometry GeometryReport) profileContext {
	ctx := profileContext{
		doc:                   doc,
		zoneByKey:             map[string]profileZoneContext{},
		zoneLists:             map[string][]string{},
		spaceToZone:           map[string]string{},
		spaceByKey:            map[string]profileSpaceContext{},
		spacesByZone:          map[string][]profileSpaceContext{},
		spaceLists:            map[string][]string{},
		scheduleByKey:         map[string]ScheduleSummary{},
		peopleDensity:         map[string]float64{},
		peopleCount:           map[string]float64{},
		peopleBySpace:         map[string]float64{},
		peopleResolved:        map[string]bool{},
		peopleResolvedBySpace: map[string]bool{},
		peopleInvalid:         map[string]bool{},
		peopleInvalidBySpace:  map[string]bool{},
		implicitRemainder:     map[string]bool{},
	}
	geometryZones := map[string]GeometryZone{}
	for _, zone := range geometry.Zones {
		geometryZones[normalizeName(zone.Name)] = zone
	}
	exteriorArea := map[string]float64{}
	exteriorWallArea := map[string]float64{}
	for _, surface := range geometry.Surfaces {
		if isExteriorSurface(surface.Type, surface.OutsideBoundary) {
			zoneKey := normalizeName(surface.ZoneName)
			exteriorArea[zoneKey] += surface.Area
			if strings.EqualFold(surface.SurfaceType, "Wall") {
				exteriorWallArea[zoneKey] += surface.Area
			}
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
			multiplier := geometryNumericFieldOrDefault(obj, 1, "Multiplier")
			if multiplier <= 0 {
				multiplier = 1
			}
			// Geometry areas are reported on an effective (multiplied) basis.
			// A profile describes the representative physical zone, so normalized
			// loads and air-change rates must remain invariant to Zone Multiplier.
			if area > 0 {
				area /= multiplier
			}
			calculatedArea := area
			declaredArea, hasDeclaredArea := parseFloatField(fieldValueByCatalogName(obj, "Floor Area"))
			if hasDeclaredArea && declaredArea > 0 {
				area = declaredArea
			} else if area <= 0 {
				area = 0
			}
			if volume <= 0 {
				volume = 0
			} else if !geom.HasDeclaredVolume {
				// Autocalculated geometry volume inherits the multiplied floor area.
				volume /= multiplier
			}
			ceilingHeight := geometryNumericFieldOrDefault(obj, 0, "Ceiling Height")
			hasDeclaredVolume := geometryNumericFieldOrDefault(obj, 0, "Volume") > 0
			if !hasDeclaredVolume && ceilingHeight > 0 && area > 0 {
				// A positive Zone Ceiling Height is the authoritative autocalculation
				// basis even when geometry also supplies a calculated volume.
				volume = area * ceilingHeight
			}
			zone := profileZoneContext{
				name:                name,
				objectIndex:         obj.Index,
				floorArea:           area,
				calculatedFloorArea: calculatedArea,
				hasDeclaredArea:     hasDeclaredArea && declaredArea > 0,
				volume:              volume,
				hasDeclaredVolume:   hasDeclaredVolume,
				ceilingHeight:       ceilingHeight,
				exteriorArea:        exteriorArea[normalizeName(name)] / multiplier,
				exteriorWallArea:    exteriorWallArea[normalizeName(name)] / multiplier,
			}
			ctx.zones = append(ctx.zones, zone)
			ctx.zoneByKey[normalizeName(name)] = zone
		case strings.EqualFold(obj.Type, "ZoneList"):
			if name := objectName(obj); name != "" {
				ctx.zoneLists[normalizeName(name)] = zoneListMembers(obj)
			}
		case strings.EqualFold(obj.Type, "Space"):
			spaceName := objectName(obj)
			zoneName := firstNonEmpty(fieldValueByCatalogName(obj, "Zone Name"), findFieldByCommentWords(obj, "zone", "name"))
			if spaceName != "" && zoneName != "" {
				ctx.spaceToZone[normalizeName(spaceName)] = zoneName
				ceilingHeight, _ := parseFloatField(fieldValueByCatalogName(obj, "Ceiling Height"))
				area, hasArea := parseFloatField(fieldValueByCatalogName(obj, "Floor Area"))
				volume, hasVolume := parseFloatField(fieldValueByCatalogName(obj, "Volume"))
				if ceilingHeight <= 0 {
					ceilingHeight = 0
				}
				if area <= 0 {
					area = 0
				}
				if volume <= 0 {
					volume = 0
				}
				space := profileSpaceContext{
					name:              spaceName,
					zoneName:          zoneName,
					ceilingHeight:     ceilingHeight,
					floorArea:         area,
					hasDeclaredArea:   hasArea && area > 0,
					volume:            volume,
					hasDeclaredVolume: hasVolume && volume > 0,
				}
				ctx.spaceByKey[normalizeName(spaceName)] = space
				ctx.spacesByZone[normalizeName(zoneName)] = append(ctx.spacesByZone[normalizeName(zoneName)], space)
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

	ctx.finalizeProfileSpaceAreas(geometry)
	ctx.outdoorAirOwners = profileOutdoorAirEquipmentOwners(doc)

	ctx.seedPeople()
	return ctx
}

func (ctx *profileContext) finalizeProfileSpaceAreas(geometry GeometryReport) {
	// An explicit Space Floor Area has the same precedence as in EnergyPlus.
	// Otherwise sum physical floor surfaces assigned to the Space. Geometry's
	// PhysicalArea intentionally excludes Zone Multiplier.
	surfaceAreaBySpace := map[string]float64{}
	unassignedSurfaceByZone := map[string]bool{}
	for _, surface := range geometry.Surfaces {
		if strings.TrimSpace(surface.SpaceName) == "" {
			if strings.TrimSpace(surface.ZoneName) != "" && !surface.IsShading {
				unassignedSurfaceByZone[normalizeName(surface.ZoneName)] = true
			}
			continue
		}
		key := normalizeName(surface.SpaceName)
		space, ok := ctx.spaceByKey[key]
		if !ok {
			continue
		}
		if strings.EqualFold(surface.SurfaceType, "Floor") && !space.hasDeclaredArea {
			space.floorArea += surface.PhysicalArea
		}
		if strings.EqualFold(surface.SurfaceType, "Floor") {
			surfaceAreaBySpace[key] += surface.PhysicalArea
		}
		if isExteriorSurface(surface.Type, surface.OutsideBoundary) {
			space.exteriorArea += surface.PhysicalArea
			if strings.EqualFold(surface.SurfaceType, "Wall") {
				space.exteriorWallArea += surface.PhysicalArea
			}
		}
		ctx.spaceByKey[key] = space
	}
	for zoneKey, spaces := range ctx.spacesByZone {
		zone := ctx.zoneByKey[zoneKey]
		totalArea := 0.0
		for index := range spaces {
			spaces[index] = ctx.spaceByKey[normalizeName(spaces[index].name)]
			totalArea += spaces[index].floorArea
		}
		explicitSurfaceArea := 0.0
		for _, space := range spaces {
			explicitSurfaceArea += surfaceAreaBySpace[normalizeName(space.name)]
		}
		remainderArea := math.Max(0, zone.calculatedFloorArea-explicitSurfaceArea)
		ctx.implicitRemainder[zoneKey] = unassignedSurfaceByZone[zoneKey] || remainderArea > 1e-9
		basisArea := totalArea + remainderArea
		if zone.hasDeclaredArea {
			if basisArea <= 0 {
				basisArea = totalArea
			}
			if basisArea > 0 {
				ratio := zone.floorArea / basisArea
				totalArea = 0
				for index := range spaces {
					spaces[index].floorArea *= ratio
					totalArea += spaces[index].floorArea
					ctx.spaceByKey[normalizeName(spaces[index].name)] = spaces[index]
				}
			}
		} else if basisArea > 0 {
			// Explicit Space areas override their own calculated surfaces. Add the
			// unassigned floor-surface remainder so the automatic Zone area matches
			// EnergyPlus' implicit <Zone>-Remainder Space.
			zone.floorArea = basisArea
			ctx.zoneByKey[zoneKey] = zone
			for index := range ctx.zones {
				if normalizeName(ctx.zones[index].name) == zoneKey {
					ctx.zones[index].floorArea = zone.floorArea
					break
				}
			}
		}
		if !zone.hasDeclaredVolume && zone.ceilingHeight > 0 && zone.floorArea > 0 {
			zone.volume = zone.floorArea * zone.ceilingHeight
			ctx.zoneByKey[zoneKey] = zone
			for index := range ctx.zones {
				if normalizeName(ctx.zones[index].name) == zoneKey {
					ctx.zones[index].volume = zone.volume
					break
				}
			}
		}
		if zone.hasDeclaredArea && totalArea == 0 && len(spaces) > 1 {
			// Match EnergyPlus: a multi-Space Zone with no usable Space areas
			// cannot be apportioned merely from the Zone total.
			for index := range spaces {
				ctx.spaceByKey[normalizeName(spaces[index].name)] = spaces[index]
			}
		}
		ctx.spacesByZone[zoneKey] = spaces
		totalVolume := 0.0
		for index := range spaces {
			space := ctx.spaceByKey[normalizeName(spaces[index].name)]
			if !space.hasDeclaredVolume && space.volume <= 0 && space.floorArea > 0 && space.ceilingHeight > 0 {
				space.volume = space.floorArea * space.ceilingHeight
			}
			if !space.hasDeclaredVolume && space.volume <= 0 && zone.floorArea > 0 && space.floorArea > 0 {
				space.volume = zone.volume * space.floorArea / zone.floorArea
			}
			ctx.spaceByKey[normalizeName(space.name)] = space
			spaces[index] = space
			totalVolume += space.volume
		}
		if !zone.hasDeclaredVolume && totalVolume > 0 {
			// Geometry or Zone area × ceiling height may include an implicit
			// remainder Space that is absent from the explicit Space list.
			if zone.volume <= 0 {
				zone.volume = totalVolume
			}
			ctx.zoneByKey[zoneKey] = zone
			for index := range ctx.zones {
				if normalizeName(ctx.zones[index].name) == zoneKey {
					ctx.zones[index].volume = zone.volume
					break
				}
			}
		}
		ctx.spacesByZone[zoneKey] = spaces
	}
}

func (ctx *profileContext) seedPeople() {
	for _, obj := range ctx.doc.Objects {
		if !strings.EqualFold(obj.Type, "People") {
			continue
		}
		for _, zoneName := range ctx.resolveObjectTargetZones(obj).zoneNames {
			zone := ctx.zoneByKey[normalizeName(zoneName)]
			count, countOK, _ := ctx.peopleCountForObject(obj, zone)
			basis := ctx.targetBasisForZone(obj, zone)
			memberSpaceArea := 0.0
			for _, spaceName := range basis.spaceNames {
				memberSpaceArea += ctx.spaceByKey[normalizeName(spaceName)].floorArea
			}
			spaceAllocationResolved := basis.spaceScoped || memberSpaceArea > 0 || len(basis.spaceNames) <= 1
			if countOK {
				ctx.peopleResolved[normalizeName(zoneName)] = true
				if spaceAllocationResolved {
					for _, spaceName := range basis.spaceNames {
						ctx.peopleResolvedBySpace[normalizeName(spaceName)] = true
					}
				}
			}
			if !countOK {
				ctx.peopleInvalid[normalizeName(zoneName)] = true
				for _, spaceName := range basis.spaceNames {
					ctx.peopleInvalidBySpace[normalizeName(spaceName)] = true
				}
			}
			if count > 0 {
				ctx.peopleCount[normalizeName(zoneName)] += count
				if len(basis.spaceNames) > 0 && spaceAllocationResolved {
					directSpaceList := basis.spaceScoped && basis.instanceCount > 1 && profilePeopleMethod(obj) == "people"
					for _, spaceName := range basis.spaceNames {
						space := ctx.spaceByKey[normalizeName(spaceName)]
						share := 0.0
						switch {
						case directSpaceList:
							share = count / float64(basis.instanceCount)
						case basis.floorArea > 0 && space.floorArea > 0:
							share = count * space.floorArea / basis.floorArea
						case len(basis.spaceNames) > 0:
							share = count / float64(len(basis.spaceNames))
						}
						ctx.peopleBySpace[normalizeName(spaceName)] += share
					}
				}
				if zone.floorArea > 0 {
					ctx.peopleDensity[normalizeName(zoneName)] += count / zone.floorArea
				}
			}
		}
	}
}

func profilePeopleMethod(obj Object) string {
	method := strings.ToLower(strings.TrimSpace(profileCalculationMethod(obj)))
	switch {
	case strings.Contains(method, "people/area"):
		return "people_per_area"
	case strings.Contains(method, "area/person"):
		return "area_per_person"
	default:
		return "people"
	}
}

func (ctx profileContext) targetBasisForZone(obj Object, zone profileZoneContext) profileTargetBasis {
	target := normalizeName(profileTargetName(obj))
	zoneKey := normalizeName(zone.name)
	if target == "" {
		return profileTargetBasis{
			floorArea:        zone.floorArea,
			volume:           zone.volume,
			exteriorArea:     zone.exteriorArea,
			exteriorWallArea: zone.exteriorWallArea,
			instanceCount:    1,
		}
	}
	if space, ok := ctx.spaceByKey[target]; ok {
		if normalizeName(space.zoneName) != zoneKey {
			return profileTargetBasis{}
		}
		return profileTargetBasis{
			floorArea:        space.floorArea,
			volume:           space.volume,
			exteriorArea:     space.exteriorArea,
			exteriorWallArea: space.exteriorWallArea,
			instanceCount:    1,
			spaceNames:       []string{space.name},
			spaceScoped:      true,
		}
	}
	if members, ok := ctx.spaceLists[target]; ok {
		basis := profileTargetBasis{spaceScoped: true}
		seen := map[string]bool{}
		for _, member := range members {
			space, exists := ctx.spaceByKey[normalizeName(member)]
			key := normalizeName(space.name)
			if !exists || normalizeName(space.zoneName) != zoneKey || seen[key] {
				continue
			}
			seen[key] = true
			basis.spaceNames = append(basis.spaceNames, space.name)
			basis.floorArea += space.floorArea
			basis.volume += space.volume
			basis.exteriorArea += space.exteriorArea
			basis.exteriorWallArea += space.exteriorWallArea
			basis.instanceCount++
		}
		return basis
	}
	// A Zone or ZoneList input is expanded once per Zone. Any explicit Spaces
	// inside that Zone only control EnergyPlus' internal apportionment; their sum
	// remains the representative Zone value.
	basis := profileTargetBasis{
		floorArea:        zone.floorArea,
		volume:           zone.volume,
		exteriorArea:     zone.exteriorArea,
		exteriorWallArea: zone.exteriorWallArea,
		instanceCount:    1,
	}
	for _, space := range ctx.spacesByZone[zoneKey] {
		basis.spaceNames = append(basis.spaceNames, space.name)
	}
	return basis
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
		resolution := ctx.resolveObjectTargetZones(obj)
		var targetWarning ProfileWarning
		if len(resolution.missingMembers) > 0 && resolution.listKind != "" {
			targetName := profileTargetName(obj)
			message := fmt.Sprintf(
				"Profile target %s %q contains unresolved member(s): %s.",
				resolution.listKind,
				targetName,
				strings.Join(resolution.missingMembers, ", "),
			)
			targetWarning = profileWarning("warning", "unresolved_profile_target_member", message, "", dimension, obj)
			ctx.warnings = append(ctx.warnings, targetWarning)
		}
		for _, zoneName := range resolution.zoneNames {
			zone, ok := ctx.zoneByKey[normalizeName(zoneName)]
			if !ok {
				items = append(items, ctx.missingZoneItem(obj, dimension, zoneName))
				continue
			}
			if item, ok := ctx.profileItemForZone(obj, dimension, zone); ok {
				if len(resolution.missingMembers) > 0 && resolution.listKind != "" {
					warning := targetWarning
					warning.ZoneName = zone.name
					item = markProfileItemPartial(item, warning)
				}
				items = append(items, item)
			}
		}
		if len(resolution.zoneNames) == 0 && len(resolution.missingMembers) > 0 {
			item := ctx.missingZoneItem(obj, dimension, profileTargetName(obj))
			if resolution.listKind != "" {
				item.Warnings = []ProfileWarning{targetWarning}
			}
			items = append(items, item)
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

func markProfileItemPartial(item ProfileItem, warning ProfileWarning) ProfileItem {
	item.Warnings = append(item.Warnings, warning)
	for index := range item.Normalized {
		if item.Normalized[index].Status != metricStatusOK {
			continue
		}
		item.Normalized[index].Status = metricStatusPartial
		item.Normalized[index].DisplayValue = profileMetricDisplay(
			item.Normalized[index].Value,
			item.Normalized[index].Unit,
			metricStatusPartial,
			profileMetricPrecision(item.Normalized[index].ID),
		)
	}
	if item.DisplayMetric.Status == metricStatusOK {
		item.DisplayMetric.Status = metricStatusPartial
		item.DisplayMetric.DisplayValue = profileMetricDisplay(
			item.DisplayMetric.Value,
			item.DisplayMetric.Unit,
			metricStatusPartial,
			profileMetricPrecision(item.DisplayMetric.ID),
		)
	}
	return item
}

func profileDimensionForObject(objectType string) string {
	switch strings.ToLower(strings.TrimSpace(objectType)) {
	case "people":
		return ProfileDimensionOccupancy
	case "lights":
		return ProfileDimensionLighting
	case "electricequipment", "gasequipment", "hotwaterequipment", "steamequipment", "otherequipment":
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
		count, countOK, itemWarnings := ctx.peopleCountForObject(obj, zone)
		warnings = append(warnings, itemWarnings...)
		basis := ctx.targetBasisForZone(obj, zone)
		peoplePerArea, peoplePerAreaOK := divide(count, basis.floorArea), countOK && basis.floorArea > 0
		areaPerPerson, areaPerPersonOK := divide(basis.floorArea, count), countOK && basis.floorArea > 0 && count > 0
		switch profilePeopleMethod(obj) {
		case "people_per_area":
			if value, ok := profileNumericValue(obj, []string{"People per Floor Area", "People per Zone Floor Area"}, []string{"people", "floor", "area"}); ok {
				peoplePerArea, peoplePerAreaOK = value, true
				if value > 0 {
					areaPerPerson, areaPerPersonOK = 1/value, true
				}
			}
		case "area_per_person":
			if value, ok := profileNumericValue(obj, []string{"Floor Area per Person", "Zone Floor Area per Person"}, []string{"floor", "area", "person"}); ok && value > 0 {
				areaPerPerson, areaPerPersonOK = value, true
				peoplePerArea, peoplePerAreaOK = 1/value, true
			}
		}
		metrics = append(metrics,
			profileMetric("count", "People", "people", count, countOK, 2),
			profileMetric("people_per_area", "People density", "people/m2", peoplePerArea, peoplePerAreaOK, 5),
			profileMetric("area_per_person", "Area per person", "m2/person", areaPerPerson, areaPerPersonOK, 2),
		)
		rawValue = firstNumericRawValue(obj, "number", "people")
	case ProfileDimensionLighting:
		power, powerOK, itemWarnings := ctx.designPowerForObject(obj, zone, "lighting")
		warnings = append(warnings, itemWarnings...)
		basis := ctx.targetBasisForZone(obj, zone)
		basisPeople, _ := ctx.peopleForTargetBasis(basis, zone)
		powerPerArea, powerPerAreaOK := divide(power, basis.floorArea), powerOK && basis.floorArea > 0
		powerPerPerson, powerPerPersonOK := divide(power, basisPeople), powerOK && basisPeople > 0
		ctx.populateSourcePowerMetrics(obj, &powerPerArea, &powerPerAreaOK, &powerPerPerson, &powerPerPersonOK)
		metrics = append(metrics,
			profileMetric("total_power", "Total power", "W", power, powerOK, 2),
			profileMetric("power_per_area", "Power density", "W/m2", powerPerArea, powerPerAreaOK, 3),
			profileMetric("power_per_person", "Power per person", "W/person", powerPerPerson, powerPerPersonOK, 2),
		)
		rawValue = firstNumericRawValue(obj, "lighting", "level")
	case ProfileDimensionEquipment:
		power, powerOK, itemWarnings := ctx.designPowerForObject(obj, zone, "equipment")
		warnings = append(warnings, itemWarnings...)
		basis := ctx.targetBasisForZone(obj, zone)
		basisPeople, _ := ctx.peopleForTargetBasis(basis, zone)
		powerPerArea, powerPerAreaOK := divide(power, basis.floorArea), powerOK && basis.floorArea > 0
		powerPerPerson, powerPerPersonOK := divide(power, basisPeople), powerOK && basisPeople > 0
		ctx.populateSourcePowerMetrics(obj, &powerPerArea, &powerPerAreaOK, &powerPerPerson, &powerPerPersonOK)
		metrics = append(metrics,
			profileMetric("total_power", "Total power", "W", power, powerOK, 2),
			profileMetric("power_per_area", "Power density", "W/m2", powerPerArea, powerPerAreaOK, 3),
			profileMetric("power_per_person", "Power per person", "W/person", powerPerPerson, powerPerPersonOK, 2),
		)
		rawValue = firstNumericRawValue(obj, "design", "level")
	case ProfileDimensionInfiltration:
		basisZone, basisPeople, peopleResolved, instanceCount := ctx.airflowTargetContext(obj, zone)
		flow, ok, itemWarnings := ctx.airflowForObject(obj, basisZone, basisPeople, peopleResolved, instanceCount, false)
		warnings = append(warnings, itemWarnings...)
		metrics = mergeProfileMetrics(profileAirflowMetrics(flow, ok, basisZone, 0, false), profileAirflowSourceMetrics(obj))
		metrics, itemWarnings = profileAirflowModelStatus(obj, zone.name, dimension, metrics)
		warnings = append(warnings, itemWarnings...)
		rawValue = firstNumericRawValue(obj, "design", "flow", "rate")
	case ProfileDimensionVentilation:
		basisZone, basisPeople, peopleResolved, instanceCount := ctx.airflowTargetContext(obj, zone)
		flow, ok, itemWarnings := ctx.airflowForObject(obj, basisZone, basisPeople, peopleResolved, instanceCount, true)
		warnings = append(warnings, itemWarnings...)
		metrics = mergeProfileMetrics(profileAirflowMetrics(flow, ok, basisZone, basisPeople, true), profileAirflowSourceMetrics(obj))
		if strings.EqualFold(obj.Type, "ZoneVentilation:WindandStackOpenArea") {
			metrics = multiplyProfileMetric(metrics, "opening_area", float64(instanceCount))
		}
		metrics, itemWarnings = profileAirflowModelStatus(obj, zone.name, dimension, metrics)
		warnings = append(warnings, itemWarnings...)
		rawValue = firstNumericRawValue(obj, "design", "flow", "rate")
	default:
		return ProfileItem{}, false
	}

	scheduleName := profileScheduleName(obj)
	schedule := ctx.scheduleByKey[normalizeName(scheduleName)]
	item := ProfileItem{
		ID:                   fmt.Sprintf("profile-item-%d-%s-%s", obj.Index, dimension, safeID(zone.name)),
		ZoneName:             zone.name,
		Dimension:            dimension,
		ObjectIndex:          obj.Index,
		ObjectType:           obj.Type,
		ObjectName:           objectName(obj),
		SourceTarget:         profileTargetName(obj),
		SourceTargetKind:     ctx.profileTargetKind(profileTargetName(obj)),
		ScheduleName:         scheduleName,
		SchedulePattern:      schedule.DetectedPattern,
		ScheduleHash:         schedule.ContentHash,
		RawMethod:            rawMethod,
		RawValue:             rawValue,
		AggregationSignature: profileAggregationSignature(obj, dimension),
		Normalized:           metrics,
		DisplayMetric:        selectProfileMetric(metrics, defaultProfileAnalysisSettings().DisplayMetrics[dimension]),
		CloneEligible:        true,
		Warnings:             warnings,
	}
	if scheduleName != "" && schedule.ScheduleName == "" {
		item.Warnings = append(item.Warnings, profileWarning("warning", "missing_schedule_summary", "Schedule could not be resolved for profile timing.", zone.name, dimension, obj))
	}
	for _, message := range schedule.Warnings {
		item.Warnings = append(item.Warnings, profileWarning("warning", "schedule_profile_fallback", message, zone.name, dimension, obj))
	}
	return item, true
}

func (ctx *profileContext) outdoorAirItems(obj Object) []ProfileItem {
	var items []ProfileItem
	targets := ctx.outdoorAirTargets(objectName(obj))
	var targetZones []string
	for _, target := range targets {
		targetZones = append(targetZones, target.zoneName)
	}
	if len(targetZones) == 0 {
		ctx.warnings = append(ctx.warnings, profileWarning("warning", "unassigned_outdoor_air", "DesignSpecification:OutdoorAir is not linked to a Zone or Space through sizing, ventilation control, or HVAC equipment.", "", ProfileDimensionOutdoorAir, obj))
		return nil
	}
	for _, target := range targets {
		zoneName := target.zoneName
		zone, ok := ctx.zoneByKey[normalizeName(zoneName)]
		if !ok {
			items = append(items, ctx.missingZoneItem(obj, ProfileDimensionOutdoorAir, zoneName))
			continue
		}
		basisZone, basisPeople := target.basis(ctx, zone)
		flow, ok, warnings := target.flowForObject(ctx, obj, zone)
		if profileOutdoorAirNeedsOperationalOccupancy(obj) {
			warnings = append(warnings, profileWarning(
				"warning",
				"nominal_outdoor_air_profile",
				"The time profile is a nominal design-occupancy profile, not an actual operating flow. EnergyPlus may also apply current People schedules and the owning terminal's per-person ventilation mode.",
				zone.name,
				ProfileDimensionOutdoorAir,
				obj,
			))
		}
		metrics := mergeProfileMetrics(profileAirflowMetrics(flow, ok, basisZone, basisPeople, true), profileAirflowSourceMetrics(obj))
		scheduleName := profileScheduleName(obj)
		schedule := ctx.scheduleByKey[normalizeName(scheduleName)]
		sourceTarget := zone.name
		sourceTargetKind := "Zone"
		if !target.wholeZone && len(target.spaceNames) > 0 {
			sourceTarget = strings.Join(target.spaceNames, ", ")
			sourceTargetKind = "Space"
			if len(target.spaceNames) > 1 {
				sourceTargetKind = "SpaceList"
			}
		}
		items = append(items, ProfileItem{
			ID:                   fmt.Sprintf("profile-item-%d-outdoor-air-%s", obj.Index, safeID(zone.name)),
			ZoneName:             zone.name,
			Dimension:            ProfileDimensionOutdoorAir,
			ObjectIndex:          obj.Index,
			ObjectType:           obj.Type,
			ObjectName:           objectName(obj),
			SourceTarget:         sourceTarget,
			SourceTargetKind:     sourceTargetKind,
			ScheduleName:         scheduleName,
			SchedulePattern:      schedule.DetectedPattern,
			ScheduleHash:         schedule.ContentHash,
			RawMethod:            profileCalculationMethod(obj),
			AggregationSignature: profileAggregationSignature(obj, ProfileDimensionOutdoorAir),
			RawValue:             firstNumericRawValue(obj, "outdoor", "air", "flow"),
			Normalized:           metrics,
			DisplayMetric:        selectProfileMetric(metrics, defaultProfileAnalysisSettings().DisplayMetrics[ProfileDimensionOutdoorAir]),
			CloneEligible:        false,
			Warnings:             warnings,
		})
	}
	return items
}

func profileOutdoorAirNeedsOperationalOccupancy(obj Object) bool {
	method := strings.ToLower(strings.TrimSpace(profileCalculationMethod(obj)))
	if method == "" {
		method = "flow/person"
	}
	return strings.Contains(method, "flow/person") ||
		method == "sum" ||
		method == "maximum" ||
		strings.Contains(method, "indoorairqualityprocedure") ||
		strings.Contains(method, "proportionalcontrol")
}

func profileAggregationSignature(obj Object, dimension string) string {
	typeName := strings.ToLower(strings.TrimSpace(obj.Type))
	switch typeName {
	case "zoneinfiltration:flowcoefficient":
		return typeName + "|stack=" + profileRequiredSignatureNumber(obj, "Stack Coefficient") +
			"|n=" + profileSignatureNumber(obj, "Pressure Exponent", 0.67) +
			"|wind=" + profileRequiredSignatureNumber(obj, "Wind Coefficient") +
			"|shelter=" + profileRequiredSignatureNumber(obj, "Shelter Factor")
	case "zoneinfiltration:effectiveleakagearea":
		return typeName + "|stack=" + profileRequiredSignatureNumber(obj, "Stack Coefficient") +
			"|wind=" + profileRequiredSignatureNumber(obj, "Wind Coefficient")
	case "zoneventilation:windandstackopenarea":
		// Opening area is additive only when the weather-response and control
		// model is the same. Keep the complete EnergyPlus 25.1 input basis in
		// the signature so differently oriented/controlled openings are not
		// presented as one equivalent opening.
		return typeName +
			"|effectiveness=" + profileAutocalculatedSignatureNumber(obj, "Opening Effectiveness") +
			"|angle=" + profileSignatureNumber(obj, "Effective Angle", 0) +
			"|height=" + profileSignatureNumber(obj, "Height Difference", 0) +
			"|discharge=" + profileAutocalculatedSignatureNumber(obj, "Discharge Coefficient for Opening") +
			"|min_indoor=" + profileSignatureNumber(obj, "Minimum Indoor Temperature", -100) +
			"|min_indoor_schedule=" + profileSignatureText(obj, "Minimum Indoor Temperature Schedule Name") +
			"|max_indoor=" + profileSignatureNumber(obj, "Maximum Indoor Temperature", 100) +
			"|max_indoor_schedule=" + profileSignatureText(obj, "Maximum Indoor Temperature Schedule Name") +
			"|delta=" + profileSignatureNumber(obj, "Delta Temperature", -100) +
			"|delta_schedule=" + profileSignatureText(obj, "Delta Temperature Schedule Name") +
			"|min_outdoor=" + profileSignatureNumber(obj, "Minimum Outdoor Temperature", -100) +
			"|min_outdoor_schedule=" + profileSignatureText(obj, "Minimum Outdoor Temperature Schedule Name") +
			"|max_outdoor=" + profileSignatureNumber(obj, "Maximum Outdoor Temperature", 100) +
			"|max_outdoor_schedule=" + profileSignatureText(obj, "Maximum Outdoor Temperature Schedule Name") +
			"|max_wind=" + profileSignatureNumber(obj, "Maximum Wind Speed", 40)
	default:
		return strings.ToLower(strings.TrimSpace(dimension + "|" + profileCalculationMethod(obj)))
	}
}

func profileSignatureNumber(obj Object, fieldName string, fallback float64) string {
	raw, _, ok := fieldValueIndexByCatalogName(obj, fieldName)
	if !ok || strings.TrimSpace(raw) == "" {
		return strconv.FormatFloat(fallback, 'g', 12, 64)
	}
	value, parsed := parseFloatField(raw)
	if !parsed {
		return "invalid:" + normalizeName(raw)
	}
	return strconv.FormatFloat(value, 'g', 12, 64)
}

func profileRequiredSignatureNumber(obj Object, fieldName string) string {
	value, ok := profileCatalogNumericValue(obj, fieldName)
	if !ok {
		return "missing"
	}
	return strconv.FormatFloat(value, 'g', 12, 64)
}

func profileAutocalculatedSignatureNumber(obj Object, fieldName string) string {
	raw, _, ok := fieldValueIndexByCatalogName(obj, fieldName)
	if !ok || strings.TrimSpace(raw) == "" || strings.EqualFold(strings.TrimSpace(raw), "Autocalculate") {
		return "autocalculate"
	}
	value, parsed := parseFloatField(raw)
	if !parsed {
		return "invalid:" + normalizeName(raw)
	}
	return strconv.FormatFloat(value, 'g', 12, 64)
}

func profileSignatureText(obj Object, fieldName string) string {
	value := normalizeName(fieldValueByCatalogName(obj, fieldName))
	if value == "" {
		return "none"
	}
	return value
}

type profileOutdoorAirTarget struct {
	zoneName   string
	spaceNames []string
	wholeZone  bool
}

func (target profileOutdoorAirTarget) basis(ctx *profileContext, zone profileZoneContext) (profileZoneContext, float64) {
	if target.wholeZone || len(target.spaceNames) == 0 {
		return zone, ctx.peopleCount[normalizeName(zone.name)]
	}
	basis := zone
	basis.floorArea = 0
	basis.volume = 0
	basis.exteriorArea = 0
	basis.exteriorWallArea = 0
	people := 0.0
	for _, spaceName := range target.spaceNames {
		space := ctx.spaceByKey[normalizeName(spaceName)]
		basis.floorArea += space.floorArea
		basis.volume += space.volume
		basis.exteriorArea += space.exteriorArea
		basis.exteriorWallArea += space.exteriorWallArea
		people += ctx.peopleBySpace[normalizeName(spaceName)]
	}
	return basis, people
}

func (target profileOutdoorAirTarget) flowForObject(ctx *profileContext, obj Object, zone profileZoneContext) (float64, bool, []ProfileWarning) {
	if target.wholeZone || len(target.spaceNames) == 0 {
		return ctx.outdoorAirFlowForObject(obj, zone, ctx.peopleCount[normalizeName(zone.name)], ctx.peopleInvalid[normalizeName(zone.name)])
	}
	// EnergyPlus evaluates a DesignSpecification:OutdoorAir:SpaceList entry for
	// each associated Space before summing it to the Zone. In particular,
	// Maximum must be evaluated per Space (sum(max(components))), not after
	// aggregating all Space component bases (max(sum(components))).
	total := 0.0
	allOK := true
	var warnings []ProfileWarning
	for _, spaceName := range target.spaceNames {
		spaceTarget := profileOutdoorAirTarget{zoneName: target.zoneName, spaceNames: []string{spaceName}}
		basis, people := spaceTarget.basis(ctx, zone)
		flow, ok, itemWarnings := ctx.outdoorAirFlowForObject(obj, basis, people, ctx.peopleInvalidBySpace[normalizeName(spaceName)])
		total += flow
		allOK = allOK && ok
		warnings = append(warnings, itemWarnings...)
	}
	return total, allOK, warnings
}

func (ctx profileContext) outdoorAirTargets(outdoorAirName string) []profileOutdoorAirTarget {
	if strings.TrimSpace(outdoorAirName) == "" {
		return nil
	}
	targets := map[string]*profileOutdoorAirTarget{}
	appendTarget := func(zoneName string, spaceName string) {
		zoneName = strings.TrimSpace(zoneName)
		if zoneName == "" {
			return
		}
		key := normalizeName(zoneName)
		target := targets[key]
		if target == nil {
			target = &profileOutdoorAirTarget{zoneName: zoneName}
			targets[key] = target
		}
		if strings.TrimSpace(spaceName) == "" {
			target.wholeZone = true
			target.spaceNames = nil
		} else if !target.wholeZone {
			target.spaceNames = appendUniqueString(target.spaceNames, spaceName)
		}
	}
	appendReference := func(ownerTarget string, referenceName string) {
		spaceSpecs := ctx.outdoorAirSpaceListSpecs(referenceName)
		if len(spaceSpecs) == 0 && strings.EqualFold(strings.TrimSpace(referenceName), strings.TrimSpace(outdoorAirName)) {
			ownerKey := normalizeName(ownerTarget)
			if zoneName := ctx.spaceToZone[ownerKey]; zoneName != "" {
				appendTarget(zoneName, ownerTarget)
			} else {
				for _, zoneName := range ctx.targetNameZones(ownerTarget) {
					appendTarget(zoneName, "")
				}
			}
		}
		allowedZones := map[string]bool{}
		for _, zoneName := range ctx.targetNameZones(ownerTarget) {
			allowedZones[normalizeName(zoneName)] = true
		}
		for _, spec := range spaceSpecs {
			if !strings.EqualFold(spec.outdoorAirName, outdoorAirName) {
				continue
			}
			if zoneName := ctx.spaceToZone[normalizeName(spec.spaceName)]; zoneName != "" && allowedZones[normalizeName(zoneName)] {
				appendTarget(zoneName, spec.spaceName)
			}
		}
	}
	for _, obj := range ctx.doc.Objects {
		switch strings.ToLower(strings.TrimSpace(obj.Type)) {
		case "sizing:zone":
			ownerTarget := profileTargetName(obj)
			reference := firstNonEmpty(
				fieldValueByCatalogName(obj, "Design Specification Outdoor Air Object Name"),
				findFieldByCommentWords(obj, "design", "specification", "outdoor", "air"),
			)
			appendReference(ownerTarget, reference)
		case "controller:mechanicalventilation":
			for fieldIndex := 5; fieldIndex+1 < len(obj.Fields); fieldIndex += 3 {
				appendReference(obj.Fields[fieldIndex].Value, obj.Fields[fieldIndex+1].Value)
			}
		}
	}
	for referenceName, ownerTargets := range ctx.outdoorAirOwners {
		for _, ownerTarget := range ownerTargets {
			appendReference(ownerTarget, referenceName)
		}
	}
	out := make([]profileOutdoorAirTarget, 0, len(targets))
	for _, target := range targets {
		out = append(out, *target)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].zoneName < out[j].zoneName })
	return out
}

// profileOutdoorAirEquipmentOwners resolves DSOA references that are owned by
// zone equipment and air terminals instead of Sizing:Zone. The HVAC relation
// builder is reused so indirect AirDistributionUnit -> AirTerminal ownership
// follows the same catalog-backed Zone/Space routing as the HVAC view.
func profileOutdoorAirEquipmentOwners(doc Document) map[string][]string {
	owners := map[string][]string{}
	hvac := newHVACContext(doc)
	for _, relation := range buildHVACZoneRelations(hvac, nil) {
		ownerTarget := firstNonEmpty(relation.SpaceName, relation.ZoneName)
		if ownerTarget == "" {
			continue
		}
		components := append(append([]HVACComponent(nil), relation.ZoneEquipment...), relation.TerminalUnits...)
		seenComponents := map[string]bool{}
		for _, component := range components {
			componentKey := hvacObjectKey(component.ObjectType, component.ObjectName)
			if componentKey == "" || seenComponents[componentKey] {
				continue
			}
			seenComponents[componentKey] = true
			obj, ok := hvac.objectsByTypeName[componentKey]
			if !ok {
				continue
			}
			referenceName := fieldValueByCatalogName(obj, "Design Specification Outdoor Air Object Name")
			if referenceName == "" {
				referenceName = findFieldByCommentWords(obj, "design", "specification", "outdoor", "air")
			}
			if referenceName != "" {
				key := normalizeName(referenceName)
				owners[key] = appendUniqueString(owners[key], ownerTarget)
			}
		}
	}
	return owners
}

func (ctx profileContext) targetNameZones(target string) []string {
	target = strings.TrimSpace(target)
	key := normalizeName(target)
	if zones, ok := ctx.zoneLists[key]; ok {
		return cleanProfileNames(zones)
	}
	if zoneName := ctx.spaceToZone[key]; zoneName != "" {
		return []string{zoneName}
	}
	return cleanProfileNames([]string{target})
}

type profileOutdoorAirSpaceSpec struct {
	spaceName      string
	outdoorAirName string
}

func (ctx profileContext) outdoorAirSpaceListSpecs(name string) []profileOutdoorAirSpaceSpec {
	for _, obj := range ctx.doc.Objects {
		if !strings.EqualFold(obj.Type, "DesignSpecification:OutdoorAir:SpaceList") || !strings.EqualFold(objectName(obj), strings.TrimSpace(name)) {
			continue
		}
		var specs []profileOutdoorAirSpaceSpec
		for fieldIndex := 1; fieldIndex+1 < len(obj.Fields); fieldIndex += 2 {
			specs = append(specs, profileOutdoorAirSpaceSpec{
				spaceName:      strings.TrimSpace(obj.Fields[fieldIndex].Value),
				outdoorAirName: strings.TrimSpace(obj.Fields[fieldIndex+1].Value),
			})
		}
		return specs
	}
	return nil
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
		DisplayMetric: ProfileMetric{ID: defaultProfileAnalysisSettings().DisplayMetrics[dimension], DisplayValue: "—", Status: metricStatusMissing},
		Warnings:      []ProfileWarning{warning},
	}
}

type profileTargetResolution struct {
	zoneNames      []string
	missingMembers []string
	listKind       string
}

func (ctx profileContext) resolveObjectTargetZones(obj Object) profileTargetResolution {
	target := profileTargetName(obj)
	if target == "" {
		return profileTargetResolution{}
	}
	key := normalizeName(target)
	if zones, ok := ctx.zoneLists[key]; ok {
		resolution := profileTargetResolution{listKind: "ZoneList"}
		for _, zoneName := range cleanProfileNames(zones) {
			if zone, exists := ctx.zoneByKey[normalizeName(zoneName)]; exists {
				resolution.zoneNames = append(resolution.zoneNames, zone.name)
			} else {
				resolution.missingMembers = append(resolution.missingMembers, zoneName)
			}
		}
		if len(resolution.zoneNames) == 0 && len(resolution.missingMembers) == 0 {
			resolution.missingMembers = []string{target}
		}
		resolution.zoneNames = cleanProfileNames(resolution.zoneNames)
		return resolution
	}
	if spaceNames, ok := ctx.spaceLists[key]; ok {
		resolution := profileTargetResolution{listKind: "SpaceList"}
		for _, spaceName := range cleanProfileNames(spaceNames) {
			zoneName := ctx.spaceToZone[normalizeName(spaceName)]
			if zone, exists := ctx.zoneByKey[normalizeName(zoneName)]; zoneName != "" && exists {
				resolution.zoneNames = append(resolution.zoneNames, zone.name)
			} else {
				resolution.missingMembers = append(resolution.missingMembers, spaceName)
			}
		}
		if len(resolution.zoneNames) == 0 && len(resolution.missingMembers) == 0 {
			resolution.missingMembers = []string{target}
		}
		resolution.zoneNames = cleanProfileNames(resolution.zoneNames)
		return resolution
	}
	if zoneName := ctx.spaceToZone[key]; zoneName != "" {
		if zone, exists := ctx.zoneByKey[normalizeName(zoneName)]; exists {
			return profileTargetResolution{zoneNames: []string{zone.name}}
		}
		return profileTargetResolution{missingMembers: []string{target}}
	}
	if zone, exists := ctx.zoneByKey[key]; exists {
		return profileTargetResolution{zoneNames: []string{zone.name}}
	}
	return profileTargetResolution{missingMembers: []string{target}}
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
	if value := fieldValueByCatalogName(obj,
		"Zone or ZoneList or Space or SpaceList Name",
		"Zone or ZoneList Name",
		"Zone or Space Name",
		"Zone Name",
		"Space Name",
	); value != "" {
		return value
	}
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
	if value := fieldValueByCatalogName(obj,
		"Number of People Schedule Name",
		"Schedule Name",
		"Outdoor Air Flow Rate Fraction Schedule Name",
	); value != "" {
		return value
	}
	for _, field := range obj.Fields {
		comment := strings.ToLower(field.Comment)
		if strings.Contains(comment, "schedule") && strings.Contains(comment, "name") {
			return strings.TrimSpace(field.Value)
		}
	}
	return ""
}

func profileCalculationMethod(obj Object) string {
	for _, name := range []string{
		"Number of People Calculation Method",
		"Design Level Calculation Method",
		"Design Flow Rate Calculation Method",
		"Outdoor Air Method",
		"Calculation Method",
	} {
		if value := fieldValueByCatalogName(obj, name); value != "" {
			return value
		}
	}
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
	basis := ctx.targetBasisForZone(obj, zone)
	var warnings []ProfileWarning
	switch {
	case strings.Contains(method, "people/area"):
		value, ok := profileNumericValue(obj,
			[]string{"People per Floor Area", "People per Zone Floor Area"},
			[]string{"people", "floor", "area"},
		)
		if basis.floorArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_area", "Zone area is required to normalize People/Area.", zone.name, ProfileDimensionOccupancy, obj))
			return 0, false, warnings
		}
		return value * basis.floorArea, ok, warnings
	case strings.Contains(method, "area/person"):
		value, ok := profileNumericValue(obj,
			[]string{"Floor Area per Person", "Zone Floor Area per Person"},
			[]string{"floor", "area", "person"},
		)
		if !ok || value <= 0 {
			return 0, false, warnings
		}
		if basis.floorArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_area", "Zone area is required to normalize Area/Person.", zone.name, ProfileDimensionOccupancy, obj))
			return 0, false, warnings
		}
		return basis.floorArea / value, true, warnings
	default:
		value, ok := profileNumericValue(obj, []string{"Number of People"}, []string{"number", "people"})
		return value * float64(maxInt(1, basis.instanceCount)), ok, warnings
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func profileNumericValue(obj Object, catalogNames []string, commentWords []string) (float64, bool) {
	if value, ok := parseFloatField(fieldValueByCatalogName(obj, catalogNames...)); ok {
		return value, true
	}
	return findNumericFieldByCommentWords(obj, commentWords...)
}

func profileCatalogNumericValue(obj Object, catalogName string) (float64, bool) {
	value, _, ok := fieldValueIndexByCatalogName(obj, catalogName)
	if !ok {
		return 0, false
	}
	return parseFloatField(value)
}

func (ctx profileContext) designPowerForObject(obj Object, zone profileZoneContext, kind string) (float64, bool, []ProfileWarning) {
	method := strings.ToLower(profileCalculationMethod(obj))
	basis := ctx.targetBasisForZone(obj, zone)
	dimension := ProfileDimensionEquipment
	if kind == "lighting" {
		dimension = ProfileDimensionLighting
	}
	var warnings []ProfileWarning
	switch {
	case strings.Contains(method, "watts/area") || strings.Contains(method, "power/area"):
		value, ok := profileNumericValue(obj,
			[]string{"Watts per Floor Area", "Watts per Zone Floor Area", "Power per Floor Area", "Power per Zone Floor Area"},
			[]string{"per", "floor", "area"},
		)
		if basis.floorArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_area", "Zone area is required to normalize Watts/Area.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * basis.floorArea, ok, warnings
	case strings.Contains(method, "watts/person") || strings.Contains(method, "power/person"):
		value, ok := profileNumericValue(obj,
			[]string{"Watts per Person", "Power per Person"},
			[]string{"per", "person"},
		)
		people, peopleResolved := ctx.peopleForTargetBasis(basis, zone)
		if !peopleResolved {
			warnings = append(warnings, profileWarning("warning", "missing_people_reference", "People count is required to normalize Watts/Person.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * people, ok, warnings
	default:
		if kind == "lighting" {
			value, ok := profileNumericValue(obj, []string{"Lighting Level"}, []string{"lighting", "level"})
			return value * float64(maxInt(1, basis.instanceCount)), ok, warnings
		}
		value, ok := profileNumericValue(obj, []string{"Design Level"}, []string{"design", "level"})
		return value * float64(maxInt(1, basis.instanceCount)), ok, warnings
	}
}

func (ctx profileContext) peopleForTargetBasis(basis profileTargetBasis, zone profileZoneContext) (float64, bool) {
	if basis.spaceScoped && len(basis.spaceNames) > 0 {
		total := 0.0
		resolved := true
		for _, spaceName := range basis.spaceNames {
			key := normalizeName(spaceName)
			total += ctx.peopleBySpace[key]
			resolved = resolved && ctx.peopleResolvedBySpace[key] && !ctx.peopleInvalidBySpace[key]
		}
		return total, resolved
	}
	key := normalizeName(zone.name)
	return ctx.peopleCount[key], ctx.peopleResolved[key] && !ctx.peopleInvalid[key]
}

func (ctx profileContext) populateSourcePowerMetrics(obj Object, powerPerArea *float64, powerPerAreaOK *bool, powerPerPerson *float64, powerPerPersonOK *bool) {
	method := strings.ToLower(strings.TrimSpace(profileCalculationMethod(obj)))
	switch {
	case strings.Contains(method, "watts/area") || strings.Contains(method, "power/area"):
		if value, ok := profileNumericValue(obj,
			[]string{"Watts per Floor Area", "Watts per Zone Floor Area", "Power per Floor Area", "Power per Zone Floor Area"},
			[]string{"per", "floor", "area"},
		); ok {
			*powerPerArea, *powerPerAreaOK = value, true
		}
	case strings.Contains(method, "watts/person") || strings.Contains(method, "power/person"):
		if value, ok := profileNumericValue(obj,
			[]string{"Watts per Person", "Power per Person"},
			[]string{"per", "person"},
		); ok {
			*powerPerPerson, *powerPerPersonOK = value, true
		}
	}
}

func (ctx profileContext) airflowTargetContext(obj Object, zone profileZoneContext) (profileZoneContext, float64, bool, int) {
	basis := ctx.targetBasisForZone(obj, zone)
	if !basis.spaceScoped {
		key := normalizeName(zone.name)
		instanceCount := maxInt(1, basis.instanceCount)
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(obj.Type)), "zoneventilation:") {
			instanceCount = ctx.profileZoneSpaceInstanceCount(zone.name)
		}
		return zone, ctx.peopleCount[key], ctx.peopleResolved[key] && !ctx.peopleInvalid[key], instanceCount
	}
	targetZone := zone
	targetZone.floorArea = basis.floorArea
	targetZone.volume = basis.volume
	targetZone.exteriorArea = basis.exteriorArea
	targetZone.exteriorWallArea = basis.exteriorWallArea
	people, peopleResolved := ctx.peopleForTargetBasis(basis, zone)
	return targetZone, people, peopleResolved, maxInt(1, basis.instanceCount)
}

func (ctx profileContext) profileZoneSpaceInstanceCount(zoneName string) int {
	key := normalizeName(zoneName)
	count := len(ctx.spacesByZone[key])
	if ctx.implicitRemainder[key] {
		count++
	}
	return maxInt(1, count)
}

func multiplyProfileMetric(metrics []ProfileMetric, metricID string, factor float64) []ProfileMetric {
	if factor == 1 {
		return metrics
	}
	for index := range metrics {
		if metrics[index].ID != metricID || metrics[index].Status == metricStatusMissing {
			continue
		}
		metrics[index].Value *= factor
		metrics[index].DisplayValue = profileMetricDisplay(
			metrics[index].Value,
			metrics[index].Unit,
			metrics[index].Status,
			profileMetricPrecision(metrics[index].ID),
		)
	}
	return metrics
}

func (ctx profileContext) airflowForObject(obj Object, zone profileZoneContext, peopleCount float64, peopleResolved bool, instanceCount int, usePeople bool) (float64, bool, []ProfileWarning) {
	method := strings.ToLower(profileCalculationMethod(obj))
	dimension := ProfileDimensionInfiltration
	if usePeople {
		dimension = ProfileDimensionVentilation
	}
	var warnings []ProfileWarning
	switch strings.ToLower(strings.TrimSpace(obj.Type)) {
	case "zoneinfiltration:effectiveleakagearea", "zoneinfiltration:flowcoefficient":
		warnings = append(warnings, profileWarning(
			"info",
			"weather_dependent_airflow",
			"A nominal volume flow cannot be derived without weather and pressure conditions for this infiltration model.",
			zone.name,
			dimension,
			obj,
		))
		return 0, false, warnings
	case "zoneventilation:windandstackopenarea":
		warnings = append(warnings, profileWarning(
			"info",
			"weather_dependent_airflow",
			"A nominal volume flow cannot be derived without wind and temperature conditions for this ventilation model.",
			zone.name,
			dimension,
			obj,
		))
		return 0, false, warnings
	}
	switch {
	case strings.Contains(method, "flow/person"):
		value, ok := profileNumericValue(obj, []string{"Flow Rate per Person"}, []string{"flow", "person"})
		if !peopleResolved {
			warnings = append(warnings, profileWarning("warning", "missing_people_reference", "People count is required to normalize flow/person.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * peopleCount, ok, warnings
	case strings.Contains(method, "flow/area") || strings.Contains(method, "flow per zone floor area"):
		value, ok := profileNumericValue(obj, []string{"Flow Rate per Floor Area", "Flow Rate per Zone Floor Area"}, []string{"flow", "area"})
		if zone.floorArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_area", "Zone area is required to normalize flow/area.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * zone.floorArea, ok, warnings
	case strings.Contains(method, "flow/exteriorwall") || strings.Contains(method, "exterior wall"):
		value, ok := profileNumericValue(obj, []string{"Flow Rate per Exterior Surface Area"}, []string{"flow", "exterior", "surface", "area"})
		if zone.exteriorWallArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_exterior_wall_area", "Exterior wall area is required to normalize flow/exterior wall area.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * zone.exteriorWallArea, ok, warnings
	case strings.Contains(method, "flow/exterior") || strings.Contains(method, "exterior"):
		value, ok := profileNumericValue(obj, []string{"Flow Rate per Exterior Surface Area"}, []string{"flow", "exterior", "surface", "area"})
		if zone.exteriorArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_exterior_area", "Exterior area is required to normalize flow/exterior area.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * zone.exteriorArea, ok, warnings
	case strings.Contains(method, "airchanges") || strings.Contains(method, "air changes"):
		value, ok := profileNumericValue(obj, []string{"Air Changes per Hour"}, []string{"air", "changes", "hour"})
		if zone.volume <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_volume", "Zone volume is required to normalize ACH.", zone.name, dimension, obj))
			return 0, false, warnings
		}
		return value * zone.volume / 3600, ok, warnings
	default:
		value, ok := profileNumericValue(obj, []string{"Design Flow Rate"}, []string{"design", "flow", "rate"})
		return value * float64(maxInt(1, instanceCount)), ok, warnings
	}
}

func profileAirflowModelStatus(obj Object, zoneName string, dimension string, metrics []ProfileMetric) ([]ProfileMetric, []ProfileWarning) {
	typeName := strings.ToLower(strings.TrimSpace(obj.Type))
	var warnings []ProfileWarning
	markPartial := false

	switch typeName {
	case "zoneinfiltration:designflowrate", "zoneventilation:designflowrate":
		nonDefault, invalid := profileDesignFlowModifierFields(obj)
		if typeName == "zoneventilation:designflowrate" {
			controlNonDefault, controlInvalid := profileVentilationControlFields(obj)
			nonDefault = append(nonDefault, controlNonDefault...)
			invalid = append(invalid, controlInvalid...)
		}
		if len(nonDefault) > 0 || len(invalid) > 0 {
			markPartial = true
			parts := make([]string, 0, 2)
			if len(nonDefault) > 0 {
				parts = append(parts, "non-default "+strings.Join(nonDefault, ", "))
			}
			if len(invalid) > 0 {
				parts = append(parts, "invalid "+strings.Join(invalid, ", "))
			}
			warnings = append(warnings, profileWarning(
				"warning",
				"weather_modified_design_flow_basis",
				"Profile reports the configured nominal/design flow basis only ("+strings.Join(parts, "; ")+"). Actual airflow also depends on indoor/outdoor temperature and wind speed.",
				zoneName,
				dimension,
				obj,
			))
		}
	case "zoneinfiltration:effectiveleakagearea":
		missing := profileMissingNonnegativeModelFields(obj, "Stack Coefficient", "Wind Coefficient")
		if len(missing) > 0 {
			markPartial = true
			warnings = append(warnings, profileWarning(
				"warning",
				"incomplete_weather_airflow_model",
				"Effective leakage area is only a partial model input because required "+strings.Join(missing, ", ")+" is missing or invalid.",
				zoneName,
				dimension,
				obj,
			))
		}
	case "zoneinfiltration:flowcoefficient":
		missing := profileMissingNonnegativeModelFields(obj, "Stack Coefficient", "Wind Coefficient", "Shelter Factor")
		if raw := strings.TrimSpace(fieldValueByCatalogName(obj, "Pressure Exponent")); raw != "" {
			if value, ok := parseFloatField(raw); !ok || value <= 0 {
				missing = append(missing, "Pressure Exponent")
			}
		}
		if len(missing) > 0 {
			markPartial = true
			warnings = append(warnings, profileWarning(
				"warning",
				"incomplete_weather_airflow_model",
				"Flow coefficient is only a partial model input because required "+strings.Join(missing, ", ")+" is missing or invalid.",
				zoneName,
				dimension,
				obj,
			))
		}
	case "zoneventilation:windandstackopenarea":
		invalid := profileInvalidWindAndStackFields(obj)
		markPartial = true
		if len(invalid) > 0 {
			markPartial = true
			warnings = append(warnings, profileWarning(
				"warning",
				"invalid_weather_airflow_model",
				"Opening area is only a partial model input because "+strings.Join(invalid, ", ")+" is invalid.",
				zoneName,
				dimension,
				obj,
			))
		}
		warnings = append(warnings, profileWarning(
			"info",
			"weather_dependent_opening_profile",
			"The time profile shows scheduled nominal opening area. EnergyPlus calculates operating ventilation from wind, stack temperature difference, and temperature/wind controls.",
			zoneName,
			dimension,
			obj,
		))
	}

	if markPartial {
		metrics = markProfileMetricsPartial(metrics)
	}
	return metrics, warnings
}

func profileDesignFlowModifierFields(obj Object) (nonDefault []string, invalid []string) {
	fields := []struct {
		name     string
		fallback float64
	}{
		{name: "Constant Term Coefficient", fallback: 1},
		{name: "Temperature Term Coefficient", fallback: 0},
		{name: "Velocity Term Coefficient", fallback: 0},
		{name: "Velocity Squared Term Coefficient", fallback: 0},
	}
	for _, item := range fields {
		raw, _, found := fieldValueIndexByCatalogName(obj, item.name)
		if !found || strings.TrimSpace(raw) == "" {
			continue
		}
		value, ok := parseFloatField(raw)
		if !ok {
			invalid = append(invalid, item.name)
			continue
		}
		if math.Abs(value-item.fallback) > 1e-12 {
			nonDefault = append(nonDefault, item.name)
		}
	}
	return nonDefault, invalid
}

func profileVentilationControlFields(obj Object) (nonDefault []string, invalid []string) {
	numericFields := []struct {
		name     string
		fallback float64
		minimum  float64
		maximum  float64
	}{
		{name: "Minimum Indoor Temperature", fallback: -100, minimum: -100, maximum: 100},
		{name: "Maximum Indoor Temperature", fallback: 100, minimum: -100, maximum: 100},
		{name: "Delta Temperature", fallback: -100, minimum: -100, maximum: math.Inf(1)},
		{name: "Minimum Outdoor Temperature", fallback: -100, minimum: -100, maximum: 100},
		{name: "Maximum Outdoor Temperature", fallback: 100, minimum: -100, maximum: 100},
		{name: "Maximum Wind Speed", fallback: 40, minimum: 0, maximum: 40},
	}
	for _, item := range numericFields {
		raw, _, found := fieldValueIndexByCatalogName(obj, item.name)
		raw = strings.TrimSpace(raw)
		if !found || raw == "" {
			continue
		}
		value, ok := parseFloatField(raw)
		if !ok || value < item.minimum || value > item.maximum {
			invalid = append(invalid, item.name)
			continue
		}
		if math.Abs(value-item.fallback) > 1e-12 {
			nonDefault = append(nonDefault, item.name)
		}
	}
	for _, name := range []string{
		"Minimum Indoor Temperature Schedule Name",
		"Maximum Indoor Temperature Schedule Name",
		"Delta Temperature Schedule Name",
		"Minimum Outdoor Temperature Schedule Name",
		"Maximum Outdoor Temperature Schedule Name",
	} {
		if strings.TrimSpace(fieldValueByCatalogName(obj, name)) != "" {
			nonDefault = append(nonDefault, name)
		}
	}
	return nonDefault, invalid
}

func profileMissingNonnegativeModelFields(obj Object, fieldNames ...string) []string {
	missing := make([]string, 0, len(fieldNames))
	for _, fieldName := range fieldNames {
		value, ok := profileCatalogNumericValue(obj, fieldName)
		if !ok || value < 0 {
			missing = append(missing, fieldName)
		}
	}
	return missing
}

func profileInvalidWindAndStackFields(obj Object) []string {
	type numericRule struct {
		name       string
		allowAuto  bool
		minimum    float64
		maximum    float64
		hasMaximum bool
	}
	rules := []numericRule{
		{name: "Opening Area", minimum: 0},
		{name: "Opening Effectiveness", allowAuto: true, minimum: 0, maximum: 1, hasMaximum: true},
		{name: "Effective Angle", minimum: 0, maximum: 360, hasMaximum: true},
		{name: "Height Difference", minimum: 0},
		{name: "Discharge Coefficient for Opening", allowAuto: true, minimum: 0, maximum: 1, hasMaximum: true},
		{name: "Maximum Wind Speed", minimum: 0, maximum: 40, hasMaximum: true},
	}
	var invalid []string
	for _, rule := range rules {
		raw, _, found := fieldValueIndexByCatalogName(obj, rule.name)
		raw = strings.TrimSpace(raw)
		if !found || raw == "" || (rule.allowAuto && strings.EqualFold(raw, "Autocalculate")) {
			if rule.name == "Opening Area" || rule.name == "Height Difference" {
				invalid = append(invalid, rule.name)
			}
			continue
		}
		value, ok := parseFloatField(raw)
		if !ok || value < rule.minimum || (rule.hasMaximum && value > rule.maximum) {
			invalid = append(invalid, rule.name)
		}
	}
	return invalid
}

func markProfileMetricsPartial(metrics []ProfileMetric) []ProfileMetric {
	for index := range metrics {
		if metrics[index].Status == metricStatusOK {
			metrics[index].Status = metricStatusPartial
		}
	}
	return metrics
}

func (ctx profileContext) outdoorAirFlowForObject(obj Object, zone profileZoneContext, peopleCount float64, peopleInvalid bool) (float64, bool, []ProfileWarning) {
	method := strings.ToLower(strings.TrimSpace(profileCalculationMethod(obj)))
	if method == "" {
		method = "flow/person"
	}
	var warnings []ProfileWarning
	flowPerPerson := func() (float64, bool) {
		value, ok := profileCatalogNumericValue(obj, "Outdoor Air Flow per Person")
		if !ok {
			// EnergyPlus defaults this field to 0.00944 m3/s-person.
			value, ok = 0.00944, true
		}
		if peopleInvalid {
			warnings = append(warnings, profileWarning("warning", "missing_people_reference", "A configured People object could not be resolved for outdoor air flow/person.", zone.name, ProfileDimensionOutdoorAir, obj))
			return 0, false
		}
		if peopleCount <= 0 {
			// A zone with no design occupants contributes zero to a per-person
			// outdoor-air requirement. The derived flow/person metric itself
			// remains unavailable because its denominator is zero.
			return 0, ok
		}
		return value * peopleCount, ok
	}
	flowPerArea := func() (float64, bool) {
		value, ok := profileCatalogNumericValue(obj, "Outdoor Air Flow per Zone Floor Area")
		if !ok {
			value, ok = 0, true
		}
		if value == 0 {
			return 0, ok
		}
		if zone.floorArea <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_area", "Zone area is required to normalize outdoor air flow/area.", zone.name, ProfileDimensionOutdoorAir, obj))
			return 0, false
		}
		return value * zone.floorArea, ok
	}
	flowPerZone := func() (float64, bool) {
		value, ok := profileCatalogNumericValue(obj, "Outdoor Air Flow per Zone")
		if !ok {
			value, ok = 0, true
		}
		return value, ok
	}
	flowACH := func() (float64, bool) {
		value, ok := profileCatalogNumericValue(obj, "Outdoor Air Flow Air Changes per Hour")
		if !ok {
			value, ok = 0, true
		}
		if value == 0 {
			return 0, ok
		}
		if zone.volume <= 0 {
			warnings = append(warnings, profileWarning("warning", "missing_zone_volume", "Zone volume is required to normalize outdoor air ACH.", zone.name, ProfileDimensionOutdoorAir, obj))
			return 0, false
		}
		return value * zone.volume / 3600, ok
	}
	switch {
	case method == "sum" || strings.Contains(method, "indoorairqualityprocedure"):
		person, personOK := flowPerPerson()
		area, areaOK := flowPerArea()
		zoneFlow, zoneOK := flowPerZone()
		ach, achOK := flowACH()
		return person + area + zoneFlow + ach, personOK && areaOK && zoneOK && achOK, warnings
	case method == "maximum":
		person, personOK := flowPerPerson()
		area, areaOK := flowPerArea()
		zoneFlow, zoneOK := flowPerZone()
		ach, achOK := flowACH()
		return math.Max(math.Max(person, area), math.Max(zoneFlow, ach)), personOK && areaOK && zoneOK && achOK, warnings
	case strings.Contains(method, "proportionalcontrol"):
		person, personOK := flowPerPerson()
		area, areaOK := flowPerArea()
		warnings = append(warnings, profileWarning(
			"info",
			"dynamic_outdoor_air_control",
			"Proportional outdoor air control varies during operation; the profile reports its full-design peak from the configured outdoor-air inputs.",
			zone.name,
			ProfileDimensionOutdoorAir,
			obj,
		))
		return person + area, personOK && areaOK, warnings
	case strings.Contains(method, "flow/person"):
		flow, ok := flowPerPerson()
		return flow, ok, warnings
	case strings.Contains(method, "flow/area"):
		flow, ok := flowPerArea()
		return flow, ok, warnings
	case strings.Contains(method, "airchanges") || strings.Contains(method, "air changes"):
		flow, ok := flowACH()
		return flow, ok, warnings
	default:
		flow, ok := flowPerZone()
		return flow, ok, warnings
	}
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

func profileAirflowSourceMetrics(obj Object) []ProfileMetric {
	metric := func(id, label, unit string, value float64, ok bool, precision int) []ProfileMetric {
		if !ok {
			return nil
		}
		return []ProfileMetric{profileMetric(id, label, unit, value, true, precision)}
	}
	lowerType := strings.ToLower(strings.TrimSpace(obj.Type))
	method := strings.ToLower(strings.TrimSpace(profileCalculationMethod(obj)))
	switch lowerType {
	case "zoneinfiltration:effectiveleakagearea":
		value, ok := profileNumericValue(obj, []string{"Effective Air Leakage Area"}, []string{"effective", "air", "leakage", "area"})
		metrics := metric("effective_leakage_area", "Effective leakage area", "cm2", value, ok, 2)
		_, stackOK := profileCatalogNumericValue(obj, "Stack Coefficient")
		_, windOK := profileCatalogNumericValue(obj, "Wind Coefficient")
		if len(metrics) > 0 && (!stackOK || !windOK) {
			metrics[0].Status = metricStatusPartial
		}
		return metrics
	case "zoneinfiltration:flowcoefficient":
		value, ok := profileNumericValue(obj, []string{"Flow Coefficient"}, []string{"flow", "coefficient"})
		return metric("flow_coefficient", "Flow coefficient", "m3/s-Pa^n", value, ok, 6)
	case "zoneventilation:windandstackopenarea":
		value, ok := profileNumericValue(obj, []string{"Opening Area"}, []string{"opening", "area"})
		if !ok && strings.TrimSpace(fieldValueByCatalogName(obj, "Opening Area")) == "" {
			// EnergyPlus 25.1 defines a blank Opening Area as 0 m2.
			value, ok = 0, true
		}
		return metric("opening_area", "Opening area", "m2", value, ok, 3)
	case "designspecification:outdoorair":
		var metrics []ProfileMetric
		personValue, personOK := profileCatalogNumericValue(obj, "Outdoor Air Flow per Person")
		usesPersonDefault := method == "" ||
			strings.Contains(method, "flow/person") ||
			method == "sum" || method == "maximum" ||
			strings.Contains(method, "indoorairqualityprocedure") ||
			strings.Contains(method, "proportionalcontrol")
		if !personOK && usesPersonDefault {
			personValue, personOK = 0.00944, true
		}
		if personOK {
			value := personValue
			metrics = append(metrics, profileMetric("flow_per_person", "Flow per person", "m3/s-person", value, true, 6))
		}
		if value, ok := profileCatalogNumericValue(obj, "Outdoor Air Flow per Zone Floor Area"); ok {
			metrics = append(metrics, profileMetric("flow_per_area", "Flow per floor area", "m3/s-m2", value, true, 6))
		}
		// Flow/Zone is a total-flow source only when that method is selected.
		// For Sum/Maximum it is just one component; the derived combined flow
		// must not be overwritten by the source component.
		if strings.Contains(method, "flow/zone") {
			if value, ok := profileCatalogNumericValue(obj, "Outdoor Air Flow per Zone"); ok {
				metrics = append(metrics, profileMetric("flow", "Flow", "m3/s", value, true, 4))
			}
		}
		if value, ok := profileCatalogNumericValue(obj, "Outdoor Air Flow Air Changes per Hour"); ok {
			metrics = append(metrics, profileMetric("ach", "Air changes", "ACH", value, true, 3))
		}
		return metrics
	}
	switch {
	case strings.Contains(method, "flow/person"):
		value, ok := profileNumericValue(obj, []string{"Flow Rate per Person"}, []string{"flow", "person"})
		return metric("flow_per_person", "Flow per person", "m3/s-person", value, ok, 6)
	case strings.Contains(method, "flow/area"):
		value, ok := profileNumericValue(obj, []string{"Flow Rate per Floor Area", "Flow Rate per Zone Floor Area"}, []string{"flow", "area"})
		return metric("flow_per_area", "Flow per floor area", "m3/s-m2", value, ok, 6)
	case strings.Contains(method, "flow/exteriorwall") || strings.Contains(method, "exterior wall"):
		value, ok := profileNumericValue(obj, []string{"Flow Rate per Exterior Surface Area"}, []string{"flow", "exterior", "surface", "area"})
		return metric("flow_per_exterior_wall_area", "Flow per exterior wall area", "m3/s-m2", value, ok, 6)
	case strings.Contains(method, "flow/exterior"):
		value, ok := profileNumericValue(obj, []string{"Flow Rate per Exterior Surface Area"}, []string{"flow", "exterior", "surface", "area"})
		return metric("flow_per_exterior_area", "Flow per exterior area", "m3/s-m2", value, ok, 6)
	case strings.Contains(method, "airchanges") || strings.Contains(method, "air changes"):
		value, ok := profileNumericValue(obj, []string{"Air Changes per Hour"}, []string{"air", "changes", "hour"})
		return metric("ach", "Air changes", "ACH", value, ok, 3)
	default:
		value, ok := profileNumericValue(obj, []string{"Design Flow Rate"}, []string{"design", "flow", "rate"})
		return metric("flow", "Flow", "m3/s", value, ok, 4)
	}
}

func mergeProfileMetrics(primary []ProfileMetric, source []ProfileMetric) []ProfileMetric {
	byID := make(map[string]int, len(primary))
	for index, metric := range primary {
		byID[metric.ID] = index
	}
	for _, metric := range source {
		if index, ok := byID[metric.ID]; ok {
			// Preserve a successfully derived metric; otherwise expose the direct,
			// model-specific input so the profile has an actionable quantity.
			if primary[index].Status == metricStatusMissing && metric.Status != metricStatusMissing {
				primary[index] = metric
			}
			continue
		}
		byID[metric.ID] = len(primary)
		primary = append(primary, metric)
	}
	return primary
}

func summarizeProfileDimensions(items []ProfileItem, settings ProfileAnalysisSettings, zone profileZoneContext) []ProfileDimensionSummary {
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
		preferredMetricID := settings.DisplayMetrics[option.ID]
		candidates := aggregateProfileMetricCandidates(items, dimensionItems, option.ID, preferredMetricID, zone)
		selected := selectProfileMetricAggregate(candidates, preferredMetricID, len(dimensionItems))
		metricID := preferredMetricID
		value := 0.0
		resolvedCount := 0
		completeCount := 0
		metricLabel := profileMetricLabel(option.ID, metricID)
		unit := profileMetricUnit(option.ID, metricID)
		if selected != nil {
			metricID = selected.id
			value = selected.value
			resolvedCount = selected.resolvedCount
			completeCount = selected.completeCount
			metricLabel = selected.label
			unit = selected.unit
		}
		var itemIDs []string
		var scheduleNames []string
		var schedulePatterns []string
		var scheduleHashes []string
		var warnings []ProfileWarning
		for _, item := range dimensionItems {
			itemIDs = append(itemIDs, item.ID)
			scheduleNames = appendUniqueString(scheduleNames, item.ScheduleName)
			schedulePatterns = appendUniqueString(schedulePatterns, item.SchedulePattern)
			scheduleHashes = appendUniqueString(scheduleHashes, item.ScheduleHash)
			warnings = append(warnings, item.Warnings...)
		}
		status := metricStatusOK
		if resolvedCount == 0 {
			status = metricStatusMissing
		} else if completeCount < len(dimensionItems) {
			status = metricStatusPartial
		}
		contributionSignatures := map[string]string{
			"name":     profileScheduleContributionSignature(dimensionItems, metricID, "name", settings.NumericTolerance),
			"resolved": profileScheduleContributionSignature(dimensionItems, metricID, "resolved", settings.NumericTolerance),
		}
		contributionSignature := contributionSignatures[settings.ScheduleCompareMode]
		if settings.ScheduleCompareMode != "none" && contributionSignature == "" {
			contributionSignature = contributionSignatures["name"]
		}
		out = append(out, ProfileDimensionSummary{
			Dimension:              option.ID,
			Label:                  option.Label,
			MetricID:               metricID,
			MetricLabel:            metricLabel,
			Unit:                   unit,
			Value:                  value,
			DisplayValue:           profileMetricDisplay(value, unit, status, profileMetricPrecision(metricID)),
			Status:                 status,
			ScheduleName:           strings.Join(nonEmptyStrings(scheduleNames), " + "),
			SchedulePattern:        strings.Join(nonEmptyStrings(schedulePatterns), " + "),
			ScheduleHash:           strings.Join(nonEmptyStrings(scheduleHashes), "+"),
			ContributionSignature:  contributionSignature,
			ItemIDs:                itemIDs,
			ItemCount:              len(dimensionItems),
			ResolvedItemCount:      resolvedCount,
			FallbackMetric:         selected != nil && selected.id != preferredMetricID,
			Warnings:               warnings,
			contributionSignatures: contributionSignatures,
		})
	}
	return out
}

func profileScheduleContributionSignature(items []ProfileItem, metricID string, compareMode string, tolerance float64) string {
	compareMode = strings.ToLower(strings.TrimSpace(compareMode))
	if compareMode == "none" {
		return ""
	}
	if compareMode != "resolved" {
		compareMode = "name"
	}
	if tolerance <= 0 {
		tolerance = 0.001
	}
	numeratorID, _ := profileGraphNumeratorMetric(metricID)
	contributions := map[string]float64{}
	total := 0.0
	for _, item := range items {
		metric := selectProfileMetric(item.Normalized, numeratorID)
		if metric.Status == metricStatusMissing || math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) {
			continue
		}
		schedule := ""
		if compareMode == "resolved" {
			schedule = firstNonEmpty(item.ScheduleHash, item.ScheduleName)
		} else {
			schedule = firstNonEmpty(item.ScheduleName, item.ScheduleHash)
		}
		if schedule == "" {
			schedule = "always"
		}
		contributions[schedule] += metric.Value
		total += metric.Value
	}

	keys := make([]string, 0, len(contributions))
	for schedule := range contributions {
		keys = append(keys, schedule)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, schedule := range keys {
		fraction := 0.0
		if math.Abs(total) > 1e-12 {
			fraction = contributions[schedule] / total
		}
		bucket := math.Round(fraction/tolerance) * tolerance
		if math.Abs(bucket) < tolerance/2 {
			bucket = 0
		}
		parts = append(parts, fmt.Sprintf("%s:%.6f", schedule, bucket))
	}
	return strings.Join(parts, "+")
}

type profileMetricAggregate struct {
	id            string
	label         string
	unit          string
	value         float64
	resolvedCount int
	completeCount int
	order         int
}

func aggregateProfileMetricCandidates(allItems []ProfileItem, dimensionItems []ProfileItem, dimension string, preferredMetricID string, zone profileZoneContext) []profileMetricAggregate {
	metricIDs := []string{preferredMetricID}
	seen := map[string]bool{preferredMetricID: true}
	for _, item := range dimensionItems {
		for _, metric := range item.Normalized {
			if metric.ID != "" && !seen[metric.ID] {
				seen[metric.ID] = true
				metricIDs = append(metricIDs, metric.ID)
			}
		}
	}
	candidates := make([]profileMetricAggregate, 0, len(metricIDs))
	for order, metricID := range metricIDs {
		candidate := aggregateProfileMetric(allItems, dimensionItems, dimension, metricID, zone)
		candidate.order = order
		if candidate.resolvedCount > 0 {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func aggregateProfileMetric(allItems []ProfileItem, dimensionItems []ProfileItem, dimension string, metricID string, zone profileZoneContext) profileMetricAggregate {
	value, resolved, complete, label, unit := sumProfileItemMetric(dimensionItems, metricID)
	candidate := profileMetricAggregate{id: metricID, label: label, unit: unit, value: value, resolvedCount: resolved, completeCount: complete}
	if candidate.label == "" {
		candidate.label = profileMetricLabel(dimension, metricID)
		candidate.unit = profileMetricUnit(dimension, metricID)
	}
	if profileSourceMetricRequiresCompatibility(metricID) && !profileSourceMetricsCompatible(dimensionItems, metricID) {
		candidate.value = 0
		candidate.resolvedCount = 0
		candidate.completeCount = 0
		return candidate
	}
	var numeratorID string
	denominator := 0.0
	denominatorOK := true
	switch metricID {
	case "people_per_area":
		numeratorID, denominator = "count", zone.floorArea
	case "area_per_person":
		people, peopleResolved, peopleComplete, _, _ := sumProfileItemMetric(dimensionItems, "count")
		if peopleResolved == len(dimensionItems) && people > 0 && zone.floorArea > 0 {
			candidate.value = zone.floorArea / people
			candidate.resolvedCount = len(dimensionItems)
			candidate.completeCount = peopleComplete
		} else if len(dimensionItems) > 1 {
			candidate.value = 0
			candidate.resolvedCount = 0
			candidate.completeCount = 0
		}
		return candidate
	case "power_per_area":
		numeratorID, denominator = "total_power", zone.floorArea
	case "power_per_person":
		numeratorID = "total_power"
		peopleItems := filterProfileItemsByDimension(allItems, ProfileDimensionOccupancy)
		peopleResolved := 0
		denominator, peopleResolved, _, _, _ = sumProfileItemMetric(peopleItems, "count")
		denominatorOK = len(peopleItems) > 0 && peopleResolved == len(peopleItems)
	case "flow_per_area":
		numeratorID, denominator = "flow", zone.floorArea
	case "flow_per_exterior_area":
		numeratorID, denominator = "flow", zone.exteriorArea
	case "flow_per_exterior_wall_area":
		numeratorID, denominator = "flow", zone.exteriorWallArea
	case "flow_per_person":
		numeratorID = "flow"
		peopleItems := filterProfileItemsByDimension(allItems, ProfileDimensionOccupancy)
		peopleResolved := 0
		denominator, peopleResolved, _, _, _ = sumProfileItemMetric(peopleItems, "count")
		denominatorOK = len(peopleItems) > 0 && peopleResolved == len(peopleItems)
	case "ach":
		numeratorID, denominator = "flow", zone.volume/3600
	}
	if numeratorID == "" {
		return candidate
	}
	if denominator <= 0 || !denominatorOK {
		if !profileItemsShareTarget(dimensionItems) {
			candidate.value = 0
			candidate.resolvedCount = 0
			candidate.completeCount = 0
		}
		return candidate
	}
	numerator, numeratorResolved, numeratorComplete, _, _ := sumProfileItemMetric(dimensionItems, numeratorID)
	if numeratorResolved == len(dimensionItems) {
		candidate.value = numerator / denominator
		candidate.resolvedCount = len(dimensionItems)
		candidate.completeCount = numeratorComplete
	} else if !profileItemsShareTarget(dimensionItems) {
		candidate.value = 0
		candidate.resolvedCount = 0
		candidate.completeCount = 0
	}
	return candidate
}

func profileSourceMetricRequiresCompatibility(metricID string) bool {
	switch metricID {
	case "flow_coefficient", "effective_leakage_area", "opening_area":
		return true
	default:
		return false
	}
}

func profileSourceMetricsCompatible(items []ProfileItem, metricID string) bool {
	if len(items) <= 1 {
		return true
	}
	signature := ""
	for _, item := range items {
		if selectProfileMetric(item.Normalized, metricID).Status == metricStatusMissing {
			continue
		}
		current := strings.ToLower(strings.TrimSpace(item.AggregationSignature + "|" + item.SourceTargetKind + "|" + item.SourceTarget))
		if strings.Contains(current, "missing") || strings.Contains(current, "invalid:") {
			return false
		}
		if signature == "" {
			signature = current
			continue
		}
		if current != signature {
			return false
		}
	}
	return true
}

func profileItemsShareTarget(items []ProfileItem) bool {
	target := ""
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.SourceTargetKind + ":" + item.SourceTarget))
		if target == "" {
			target = key
			continue
		}
		if key != target {
			return false
		}
	}
	return true
}

func sumProfileItemMetric(items []ProfileItem, metricID string) (value float64, resolved int, complete int, label string, unit string) {
	for _, item := range items {
		metric := selectProfileMetric(item.Normalized, metricID)
		if metric.Status == metricStatusMissing || math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) {
			continue
		}
		value += metric.Value
		resolved++
		if metric.Status != metricStatusPartial {
			complete++
		}
		label = metric.Label
		unit = metric.Unit
	}
	return
}

func filterProfileItemsByDimension(items []ProfileItem, dimension string) []ProfileItem {
	out := make([]ProfileItem, 0)
	for _, item := range items {
		if item.Dimension == dimension {
			out = append(out, item)
		}
	}
	return out
}

func selectProfileMetricAggregate(candidates []profileMetricAggregate, preferredMetricID string, itemCount int) *profileMetricAggregate {
	for index := range candidates {
		if candidates[index].id == preferredMetricID && candidates[index].completeCount == itemCount {
			return &candidates[index]
		}
	}
	for index := range candidates {
		if candidates[index].completeCount == itemCount {
			return &candidates[index]
		}
	}
	best := -1
	for index := range candidates {
		if best < 0 || candidates[index].resolvedCount > candidates[best].resolvedCount ||
			(candidates[index].resolvedCount == candidates[best].resolvedCount && candidates[index].completeCount > candidates[best].completeCount) ||
			(candidates[index].resolvedCount == candidates[best].resolvedCount && candidates[index].completeCount == candidates[best].completeCount && candidates[index].order < candidates[best].order) {
			best = index
		}
	}
	if best < 0 {
		return nil
	}
	return &candidates[best]
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
		// The summary may have fallen back from the requested grouping metric
		// (for example people/m2) to a different engineering basis (people).
		// Keep the metric that was actually selected, plus its fallback identity,
		// in the key so equal numeric values with different meanings never merge.
		metricID := dimension.MetricID
		if metricID == "" {
			metricID = settings.GroupingMetrics[dimension.Dimension]
		}
		metricRole := "preferred"
		if dimension.FallbackMetric {
			metricRole = "fallback"
		}
		tolerance := settings.NumericTolerance
		if tolerance <= 0 {
			tolerance = 0.001
		}
		valueBucket := math.Round(dimension.Value/tolerance) * tolerance
		compareMode := strings.ToLower(strings.TrimSpace(settings.ScheduleCompareMode))
		if compareMode == "" {
			compareMode = "name"
		}
		schedulePart := ""
		switch compareMode {
		case "none":
			schedulePart = ""
		case "resolved":
			if dimension.contributionSignatures != nil {
				schedulePart = dimension.contributionSignatures["resolved"]
			} else if dimension.ContributionSignature != "" {
				schedulePart = dimension.ContributionSignature
			} else {
				schedulePart = dimension.ScheduleHash
			}
		default:
			if dimension.contributionSignatures != nil {
				schedulePart = dimension.contributionSignatures["name"]
			} else if dimension.ContributionSignature != "" {
				schedulePart = dimension.ContributionSignature
			} else {
				schedulePart = dimension.ScheduleName
			}
		}
		parts = append(parts, fmt.Sprintf("%s:%s:%s:%.6f:%s:%d/%d:%s", dimension.Dimension, metricID, metricRole, valueBucket, dimension.Status, dimension.ResolvedItemCount, dimension.ItemCount, schedulePart))
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
		if interpolation := compactScheduleInterpolationMode(obj); interpolation != "" && !strings.EqualFold(interpolation, "No") {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf(
				"Schedule:Compact Interpolate: %s is shown as a step approximation; the actual interpolated time profile is not evaluated.",
				interpolation,
			))
		}
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

func compactScheduleInterpolationMode(obj Object) string {
	for _, field := range obj.Fields {
		value := strings.TrimSpace(field.Value)
		if strings.HasPrefix(strings.ToLower(value), "interpolate:") {
			return strings.TrimSpace(value[len("interpolate:"):])
		}
	}
	return ""
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
			Value:     interval.value,
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
			Value:     value,
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
		profile[i] = value
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
	for _, rule := range summary.Rules {
		fmt.Fprintf(&b, "%d:%d:%s|", rule.StartDay, rule.EndDay, strings.ToLower(strings.TrimSpace(rule.Selector)))
		for _, interval := range rule.Intervals {
			fmt.Fprintf(&b, "%.6f:%.6f:%.9g,", interval.StartHour, interval.EndHour, interval.Value)
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
	status := metricStatusOK
	display := profileMetricDisplay(value, unit, status, precision)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		status = metricStatusMissing
		value = 0
		display = "—"
	}
	return ProfileMetric{
		ID:    id,
		Label: label,
		Unit:  unit,
		// Keep the normalized engineering value at full precision. Formatting is
		// rounded separately so grouping and downstream conversions do not inherit
		// display-rounding error.
		Value:        value,
		DisplayValue: display,
		Status:       status,
	}
}

func profileMetricDisplay(value float64, unit string, status string, precision int) string {
	if status == metricStatusMissing || math.IsNaN(value) || math.IsInf(value, 0) {
		return "—"
	}
	display := formatMetricNumber(value, precision)
	if value != 0 && math.Abs(value) < math.Pow10(-precision) {
		display = strconv.FormatFloat(value, 'g', 4, 64)
	}
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
	return ProfileMetric{ID: id, Label: profileMetricLabel("", id), DisplayValue: "—", Status: metricStatusMissing}
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
	case "count", "total_power", "area_per_person", "power_per_person", "effective_leakage_area":
		return 2
	case "flow", "flow_per_area", "flow_per_exterior_area", "flow_per_exterior_wall_area", "flow_per_person", "flow_coefficient":
		return 6
	case "power_per_area", "ach", "opening_area":
		return 3
	case "people_per_area":
		return 5
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
	return append([]float64(nil), values...)
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
