package idf

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	summaryStatusOK      = "ok"
	summaryStatusPartial = "partial"
	summaryStatusMissing = "missing"
)

type SummaryReport struct {
	Categories  []SummaryCategory `json:"categories"`
	MetricCount int               `json:"metricCount"`
}

type SummaryCategory struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Metrics []SummaryMetric `json:"metrics"`
}

type SummaryMetric struct {
	ID           string   `json:"id"`
	CategoryID   string   `json:"categoryId"`
	Category     string   `json:"category"`
	Name         string   `json:"name"`
	Value        any      `json:"value,omitempty"`
	DisplayValue string   `json:"displayValue"`
	Unit         string   `json:"unit,omitempty"`
	Status       string   `json:"status"`
	Source       string   `json:"source,omitempty"`
	Confidence   string   `json:"confidence,omitempty"`
	Visibility   string   `json:"visibility,omitempty"`
	Badges       []string `json:"badges,omitempty"`
	Evidence     string   `json:"evidence,omitempty"`
}

type SummaryDefinition struct {
	ID          string `json:"id"`
	CategoryID  string `json:"categoryId"`
	Category    string `json:"category"`
	Name        string `json:"name"`
	Unit        string `json:"unit,omitempty"`
	Precision   int    `json:"precision"`
	Source      string `json:"source"`
	Method      string `json:"method"`
	Assumptions string `json:"assumptions"`
	MissingData string `json:"missingData"`
}

type SummaryGuide struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Name        string `json:"name"`
	Unit        string `json:"unit,omitempty"`
	Source      string `json:"source"`
	Method      string `json:"method"`
	Assumptions string `json:"assumptions"`
	MissingData string `json:"missingData"`
}

type SummaryAnalysisOptions struct {
	IncludeHeavyReadiness bool
}

type summaryCategoryDef struct {
	id   string
	name string
}

var summaryCategories = []summaryCategoryDef{
	{id: "model_inventory", name: "Model & Inventory"},
	{id: "geometry_areas", name: "Geometry & Areas"},
	{id: "envelope_fenestration", name: "Envelope & Fenestration"},
	{id: "internal_loads", name: "Internal Loads"},
	{id: "schedules_operation", name: "Schedules & Operation"},
	{id: "hvac_conditioning", name: "HVAC & Conditioning"},
}

var summaryDefinitions = []SummaryDefinition{
	def("energyplus_version", "model_inventory", "EnergyPlus version", "", 0, "Version / Version Identifier.", "Reads the version identifier directly from the Version object.", "Accepted versions are checked elsewhere; this metric only reports the value found in the model.", "N/A when no Version object or version identifier is present."),
	def("building_name", "model_inventory", "Building name", "", 0, "Building / Name.", "Reads the Building object name field.", "The first Building object is used when more than one is present.", "N/A when no Building name is present."),
	def("building_north_axis_deg", "model_inventory", "Building north axis", "deg", 1, "Building / North Axis.", "Reads the numeric north-axis rotation.", "This rotation is added to calculated surface azimuths for orientation bins.", "N/A when the field is blank or nonnumeric."),
	def("object_count", "model_inventory", "Object count", "", 0, "Parsed IDF or epJSON object list.", "Counts all parsed objects.", "No filtering is applied.", "Always available; zero means the input contains no parsed objects."),
	def("object_type_count", "model_inventory", "Object type count", "", 0, "Parsed IDF or epJSON object list.", "Counts distinct object type names.", "Type names are compared exactly after parsing.", "Always available; zero means the input contains no parsed object types."),
	def("zone_count", "model_inventory", "Zone count", "", 0, "Zone objects.", "Counts Zone objects with or without names.", "Spaces are not counted as zones.", "Always available; zero means no Zone objects were found."),
	def("space_count", "model_inventory", "Space count", "", 0, "Space objects.", "Counts Space objects.", "The metric is independent from Zone count.", "Always available; zero means no Space objects were found."),
	def("schedule_count", "model_inventory", "Schedule count", "", 0, "Objects whose type starts with Schedule:.", "Counts all schedule objects.", "ScheduleTypeLimits is not counted as a schedule.", "Always available; zero means no schedule objects were found."),
	def("construction_count", "model_inventory", "Construction count", "", 0, "Objects whose type starts with Construction.", "Counts construction objects.", "All Construction:* variants are included.", "Always available; zero means no construction objects were found."),
	def("material_count", "model_inventory", "Material count", "", 0, "Material* and WindowMaterial* objects.", "Counts opaque and window material objects.", "Construction objects are excluded.", "Always available; zero means no material objects were found."),

	def("gross_floor_area_m2", "geometry_areas", "Gross floor area", "m2", 2, "Zone floor fields and floor surfaces.", "Sums zone floor area fields when present; otherwise sums Floor surface polygon or rectangle areas.", "Zone multipliers are applied. Surface-derived floor area includes all floor surfaces assigned to zones.", "N/A when no numeric zone floor area or floor surface area can be computed."),
	def("conditioned_floor_area_m2", "geometry_areas", "Conditioned floor area", "m2", 2, "Gross floor area and conditioned-zone detection.", "Sums floor area for zones referenced by ZoneHVAC, ZoneHVAC:EquipmentConnections, or ZoneControl:Thermostat objects.", "Zone multipliers are applied. Only zones with known floor area can contribute.", "N/A when conditioned zones or floor areas cannot be identified."),
	def("unconditioned_floor_area_m2", "geometry_areas", "Unconditioned floor area", "m2", 2, "Gross and conditioned floor area.", "Subtracts conditioned floor area from gross floor area.", "The value is clamped at zero to avoid negative results from partial data.", "N/A when gross or conditioned floor area is unavailable."),
	def("footprint_area_m2", "geometry_areas", "Footprint area", "m2", 2, "Lowest floor surfaces.", "Sums ground-contact floor surfaces, or the lowest horizontal floor surfaces when ground boundary data is absent.", "Zone multipliers are applied to surface areas. Bounding-box dimensions are reported separately and are not used as footprint area.", "N/A when no floor surface area can be computed."),
	def("bounding_box_area_m2", "geometry_areas", "Bounding box area", "m2", 2, "Detailed surface vertices.", "Calculates the XY bounding-box area from all detailed vertices.", "This is a coarse geometric extent, not a true footprint polygon.", "N/A when detailed vertex extents are unavailable."),
	def("total_zone_volume_m3", "geometry_areas", "Total zone volume", "m3", 2, "Zone / Volume and surface-derived zone heights.", "Sums numeric Zone Volume fields; when missing, estimates volume from zone floor area times zone height.", "Zone multipliers are applied. Estimated values are marked partial.", "N/A when neither declared nor estimated zone volume is available."),
	def("average_floor_height_m", "geometry_areas", "Average floor height", "m", 2, "Zone / Ceiling Height and detailed surface extents.", "Averages numeric zone ceiling heights; falls back to max-min vertex Z height by zone.", "Each zone contributes once; autocalculate fields require geometry fallback.", "N/A when no zone height can be read or inferred."),
	def("building_long_side_m", "geometry_areas", "Building long side", "m", 2, "Detailed surface vertices.", "Uses the larger of the model XY bounding-box width and depth.", "Only detailed vertices are considered.", "N/A when detailed vertex coordinates are unavailable."),
	def("building_short_side_m", "geometry_areas", "Building short side", "m", 2, "Detailed surface vertices.", "Uses the smaller nonzero side of the model XY bounding box.", "Only detailed vertices are considered.", "N/A when detailed vertex coordinates are unavailable or one side is zero."),
	def("footprint_aspect_ratio", "geometry_areas", "Footprint aspect ratio", "", 3, "Detailed surface vertices.", "Divides building long side by building short side.", "A value of 1.0 indicates a square bounding footprint.", "N/A when either side length is unavailable."),
	def("envelope_area_m2", "geometry_areas", "Gross envelope area", "m2", 2, "Exterior walls, roofs, and ground floors.", "Sums exterior opaque wall area, roof area, and ground-contact floor area.", "Fenestration is not subtracted from opaque base surfaces; see net opaque envelope area for the subtracted value.", "N/A when no envelope surface area can be computed."),
	def("net_opaque_envelope_area_m2", "geometry_areas", "Net opaque envelope area", "m2", 2, "Gross envelope area and fenestration area.", "Subtracts recognized window and opaque door area from gross envelope area.", "This is an analyzer area balance and depends on resolved fenestration geometry.", "N/A when gross envelope area cannot be computed."),
	def("envelope_area_to_volume_ratio", "geometry_areas", "Envelope area to volume ratio", "1/m", 3, "Envelope area and total zone volume.", "Divides envelope area by total zone volume.", "Uses partial volume estimates when declared volumes are incomplete.", "N/A when envelope area or volume is unavailable."),
	def("floor_area_to_volume_ratio", "geometry_areas", "Floor area to volume ratio", "1/m", 3, "Gross floor area and total zone volume.", "Divides gross floor area by total zone volume.", "Uses partial volume estimates when declared volumes are incomplete.", "N/A when floor area or volume is unavailable."),

	def("exterior_wall_area_m2", "envelope_fenestration", "Exterior wall area", "m2", 2, "Building wall surfaces with exterior boundary conditions.", "Sums Wall surfaces exposed to Outdoors or exterior wall object types.", "Zone multipliers are applied. Wall area is gross wall area.", "N/A when no exterior wall area can be computed."),
	def("roof_area_m2", "envelope_fenestration", "Roof area", "m2", 2, "Roof and RoofCeiling surfaces.", "Sums exterior roof or roof/ceiling surface area.", "Zone multipliers are applied.", "N/A when no roof surface area can be computed."),
	def("ground_floor_area_m2", "envelope_fenestration", "Ground floor area", "m2", 2, "Floor surfaces with Ground boundary or lowest floor fallback.", "Sums floor surfaces adjacent to ground; uses lowest floor surfaces when boundary data is absent.", "Zone multipliers are applied.", "N/A when no ground or lowest floor area can be computed."),
	def("window_area_m2", "envelope_fenestration", "Window area", "m2", 2, "FenestrationSurface:Detailed and simple Window objects.", "Sums fenestration objects whose surface type indicates window or glass door.", "Fenestration multipliers and base-zone multipliers are applied.", "N/A when no window area can be computed."),
	def("door_area_m2", "envelope_fenestration", "Door area", "m2", 2, "Fenestration and Door objects.", "Sums non-glazed door area from detailed or rectangular geometry.", "Fenestration multipliers and base-zone multipliers are applied.", "N/A when no door area can be computed."),
	def("total_wwr_percent", "envelope_fenestration", "Total WWR", "%", 1, "Exterior wall area and window area.", "Divides total window area by gross exterior wall area.", "Glass doors count as window area. Wall area is gross and not net of windows.", "N/A when wall area is zero or unavailable."),
	def("north_wwr_percent", "envelope_fenestration", "North WWR", "%", 1, "Surface azimuth and WWR inputs.", "Divides north-facing window area by north-facing exterior wall area.", "North is [315,360) plus [0,45) degrees after building and zone rotations.", "N/A when no north-facing exterior wall area is available."),
	def("east_wwr_percent", "envelope_fenestration", "East WWR", "%", 1, "Surface azimuth and WWR inputs.", "Divides east-facing window area by east-facing exterior wall area.", "East is [45,135) degrees after building and zone rotations.", "N/A when no east-facing exterior wall area is available."),
	def("south_wwr_percent", "envelope_fenestration", "South WWR", "%", 1, "Surface azimuth and WWR inputs.", "Divides south-facing window area by south-facing exterior wall area.", "South is [135,225) degrees after building and zone rotations.", "N/A when no south-facing exterior wall area is available."),
	def("west_wwr_percent", "envelope_fenestration", "West WWR", "%", 1, "Surface azimuth and WWR inputs.", "Divides west-facing window area by west-facing exterior wall area.", "West is [225,315) degrees after building and zone rotations.", "N/A when no west-facing exterior wall area is available."),
	def("skylight_roof_ratio_percent", "envelope_fenestration", "Skylight roof ratio", "%", 1, "Window area on roof base surfaces and roof area.", "Divides skylight window area by roof area.", "A fenestration object is treated as skylight when its base surface is a roof.", "N/A when roof area is zero or no skylight area can be identified."),

	def("total_lighting_power_w", "internal_loads", "Total lighting power", "W", 2, "Lights objects.", "Sums LightingLevel directly and resolves Watts/Area or Watts/Person methods where supporting area or people data is available.", "Zone and ZoneList references are expanded where possible; zone multipliers are applied.", "N/A when no lighting design level can be computed."),
	def("average_lighting_power_density_w_per_m2", "internal_loads", "Average lighting power density", "W/m2", 3, "Lighting power and floor area.", "Divides total lighting power by conditioned floor area, falling back to gross floor area.", "Partial lighting or floor-area inputs produce a partial status.", "N/A when lighting power or floor area is unavailable."),
	def("total_equipment_power_w", "internal_loads", "Total equipment power", "W", 2, "ElectricEquipment objects.", "Sums EquipmentLevel directly and resolves Watts/Area or Watts/Person methods where supporting area or people data is available.", "Zone and ZoneList references are expanded where possible; zone multipliers are applied.", "N/A when no equipment design level can be computed."),
	def("average_equipment_power_density_w_per_m2", "internal_loads", "Average equipment power density", "W/m2", 3, "Equipment power and floor area.", "Divides total equipment power by conditioned floor area, falling back to gross floor area.", "Partial equipment or floor-area inputs produce a partial status.", "N/A when equipment power or floor area is unavailable."),
	def("total_people", "internal_loads", "Total people", "people", 2, "People objects.", "Sums People, People/Area, or Area/Person methods where supporting floor area is available.", "Zone and ZoneList references are expanded where possible; zone multipliers are applied.", "N/A when no people count can be computed."),
	def("people_density_per_100m2", "internal_loads", "People density", "people/100m2", 3, "People count and floor area.", "Divides total people by floor area and scales to 100 m2.", "Conditioned floor area is preferred; gross floor area is used as fallback.", "N/A when people count or floor area is unavailable."),
	def("internal_load_object_count", "internal_loads", "Internal load object count", "", 0, "People, Lights, and ElectricEquipment objects.", "Counts the main internal load objects represented in this summary.", "Other load types can be added as new catalog metrics.", "Always available; zero means no supported internal load objects were found."),
	def("internal_load_method_coverage", "internal_loads", "Internal load method coverage", "", 0, "People, Lights, and ElectricEquipment calculation methods.", "Reports resolved_objects / total_objects and unresolved_method_count for supported internal load summary objects.", "A method is unresolved when the required people, floor-area, or design-level fields cannot be resolved.", "Always available; zero totals mean no supported internal load objects were found."),

	def("referenced_schedule_count", "schedules_operation", "Referenced schedule count", "", 0, "Fields whose comments include Schedule Name.", "Counts distinct schedule names referenced by non-schedule objects.", "Schedule names are matched case-insensitively after trimming.", "Always available; zero means no schedule references were found."),
	def("supported_schedule_count", "schedules_operation", "Supported annual schedule count", "", 0, "Schedule:Constant and Schedule:Compact objects.", "Counts schedules whose annual active hours can be evaluated by the v1 parser.", "Schedule value greater than zero is treated as active.", "Always available; zero means no supported annual schedules were found."),
	def("unsupported_schedule_count", "schedules_operation", "Unsupported schedule count", "", 0, "Schedule objects.", "Subtracts supported schedules from total schedule objects.", "Unsupported schedules remain visible so operating-hour results can be interpreted.", "Always available; zero means every schedule was supported or no schedules exist."),
	def("model_operating_hours_h", "schedules_operation", "Representative operating hours", "h", 1, "Referenced supported schedules.", "Uses the maximum active annual hours among referenced supported schedules; falls back to all supported schedules.", "A non-leap 8760-hour year is used. Compact weekday/weekend rules use a fixed Monday-start annual calendar.", "N/A when no supported schedule hours are available."),
	def("average_schedule_operating_hours_h", "schedules_operation", "Average schedule operating hours", "h", 1, "Supported schedules.", "Averages annual active hours across supported schedules.", "A non-leap 8760-hour year is used. Schedule value greater than zero is active.", "N/A when no supported schedule hours are available."),

	def("hvac_object_count", "hvac_conditioning", "HVAC object count", "", 0, "HVAC-related object types.", "Counts objects whose types indicate HVAC, air loops, plant loops, coils, fans, pumps, boilers, chillers, or setpoint managers.", "This is a broad inventory count, not a system simulation result.", "Always available; zero means no recognized HVAC objects were found."),
	def("zone_hvac_object_count", "hvac_conditioning", "Zone HVAC object count", "", 0, "ZoneHVAC:* objects.", "Counts objects whose type starts with ZoneHVAC:.", "ZoneHVAC:EquipmentConnections is included.", "Always available; zero means no ZoneHVAC objects were found."),
	def("thermostat_count", "hvac_conditioning", "Thermostat count", "", 0, "ZoneControl:Thermostat and ThermostatSetpoint:* objects.", "Counts thermostat control and setpoint objects.", "Both controllers and setpoint definitions are included.", "Always available; zero means no thermostat objects were found."),
	def("conditioned_zone_count", "hvac_conditioning", "Conditioned zone count", "", 0, "ZoneHVAC, SpaceHVAC, and thermostat zone references.", "Counts distinct zones referenced by ZoneHVAC, SpaceHVAC, ZoneHVAC:EquipmentConnections, or ZoneControl:Thermostat objects.", "ZoneList references are expanded when a matching ZoneList object exists. SpaceHVAC references are mapped through Space parent zones.", "Always available; zero means no conditioned-zone references were found."),
	def("conditioned_zone_evidence_breakdown", "hvac_conditioning", "Conditioned zone evidence", "", 0, "Conditioned-zone source buckets.", "Counts conditioned zones separately by equipment connections, ZoneHVAC objects, thermostat controls, and SpaceHVAC objects.", "A zone may appear in more than one evidence bucket; the total conditioned zone count is deduplicated.", "Always available; zero buckets mean no conditioned-zone references were found."),
	def("hvac_node_connection_count", "hvac_conditioning", "HVAC node connection count", "", 0, "Typed HVAC loop branch components.", "Counts typed inlet-to-outlet edges produced by AnalyzeHVAC for parsed loop branch components.", "The result is an analyzer topology count, not a full EnergyPlus simulation solve.", "Always available; zero means no typed loop component edges were parsed."),
	def("geometry_coverage_percent", "model_inventory", "Geometry coverage", "%", 1, "Detailed geometry inputs.", "Compares surfaces/fenestration with usable detailed vertices against geometry-bearing objects.", "Simple geometry objects are counted as uncovered because detailed polygon checks cannot verify them.", "Always available; zero means no detailed geometry was available."),
	def("profile_coverage_percent", "model_inventory", "Profile coverage", "%", 1, "Schedule references and supported annual-hour parser.", "Divides referenced schedules that can be evaluated for annual hours by all referenced schedules.", "Unreferenced schedules are excluded from this readiness metric.", "N/A when no schedule references are present."),
	def("hvac_rule_edge_count", "hvac_conditioning", "HVAC rule edge count", "", 0, "HVAC RuleGraph edges.", "Counts HVAC graph edges that were created from EnergyPlus object and field rules.", "Only rule-resolved edges are counted; invalid relations remain diagnostics.", "Always available; zero means no HVAC rule edges were resolved."),
	def("diagnostics_by_source", "model_inventory", "Diagnostics by source", "", 0, "Analyzer diagnostics.", "Counts diagnostics grouped by source such as energyplus_rule, analyzer_limitation, user_quality_check, or heuristic_inference.", "Counts depend on the analyzer diagnostic rules enabled in this build.", "Always available; empty means no diagnostics were emitted."),
	def("output_readiness_percent", "model_inventory", "Output readiness", "%", 1, "Output:* requests.", "Compares requested simulation outputs against the standard output groups recognized by the analyzer.", "This is a reporting-readiness metric and does not guarantee a successful simulation.", "Always available; zero means no recognized output requests were present."),
}

func def(id, categoryID, name, unit string, precision int, source, method, assumptions, missing string) SummaryDefinition {
	return SummaryDefinition{
		ID:          id,
		CategoryID:  categoryID,
		Category:    summaryCategoryName(categoryID),
		Name:        name,
		Unit:        unit,
		Precision:   precision,
		Source:      source,
		Method:      method,
		Assumptions: assumptions,
		MissingData: missing,
	}
}

func summaryCategoryName(categoryID string) string {
	for _, category := range summaryCategories {
		if category.id == categoryID {
			return category.name
		}
	}
	return categoryID
}

func SummaryDefinitions() []SummaryDefinition {
	out := make([]SummaryDefinition, len(summaryDefinitions))
	copy(out, summaryDefinitions)
	return out
}

func SummaryGuides() []SummaryGuide {
	definitions := SummaryDefinitions()
	guides := make([]SummaryGuide, 0, len(definitions))
	for _, definition := range definitions {
		guides = append(guides, SummaryGuide{
			ID:          definition.ID,
			Category:    definition.Category,
			Name:        definition.Name,
			Unit:        definition.Unit,
			Source:      definition.Source,
			Method:      definition.Method,
			Assumptions: definition.Assumptions,
			MissingData: definition.MissingData,
		})
	}
	return guides
}

func AnalyzeSummary(doc Document) SummaryReport {
	return AnalyzeSummaryWithOptions(doc, SummaryAnalysisOptions{IncludeHeavyReadiness: true})
}

func AnalyzeSummaryQuick(doc Document) SummaryReport {
	return AnalyzeSummaryWithOptions(doc, SummaryAnalysisOptions{})
}

func AnalyzeSummaryWithOptions(doc Document, options SummaryAnalysisOptions) SummaryReport {
	facts := collectSummaryFactsWithOptions(doc, options)
	values := facts.metricValues()
	categories := make([]SummaryCategory, 0, len(summaryCategories))
	categoryIndexes := map[string]int{}

	for _, category := range summaryCategories {
		categoryIndexes[category.id] = len(categories)
		categories = append(categories, SummaryCategory{ID: category.id, Name: category.name})
	}

	for _, definition := range summaryDefinitions {
		value, ok := values[definition.ID]
		if !ok {
			value = missingSummaryValue()
		}
		metric := SummaryMetric{
			ID:           definition.ID,
			CategoryID:   definition.CategoryID,
			Category:     definition.Category,
			Name:         definition.Name,
			Value:        value.Value,
			DisplayValue: value.DisplayValue,
			Unit:         definition.Unit,
			Status:       value.Status,
			Source:       summaryMetricSource(definition.ID, definition.Source),
			Confidence:   summaryMetricConfidence(definition.ID, value.Status),
			Visibility:   summaryMetricVisibility(definition.ID),
			Badges:       summaryMetricBadges(definition.ID, value.Status),
			Evidence:     summaryMetricEvidence(definition.ID, definition.Method, facts),
		}
		if index, ok := categoryIndexes[definition.CategoryID]; ok {
			categories[index].Metrics = append(categories[index].Metrics, metric)
		}
	}

	return SummaryReport{Categories: categories, MetricCount: len(summaryDefinitions)}
}

type summaryExportMetric struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Value      any      `json:"value"`
	Unit       string   `json:"unit,omitempty"`
	Status     string   `json:"status"`
	Source     string   `json:"source,omitempty"`
	Confidence string   `json:"confidence,omitempty"`
	Visibility string   `json:"visibility,omitempty"`
	Badges     []string `json:"badges,omitempty"`
}

func ExportSummaryJSON(summary SummaryReport) (string, error) {
	out := map[string]map[string]summaryExportMetric{}
	for _, category := range summary.Categories {
		metrics := map[string]summaryExportMetric{}
		for _, metric := range category.Metrics {
			metrics[metric.ID] = summaryExportMetric{
				ID:         metric.ID,
				Name:       metric.Name,
				Value:      metric.Value,
				Unit:       metric.Unit,
				Status:     metric.Status,
				Source:     metric.Source,
				Confidence: metric.Confidence,
				Visibility: metric.Visibility,
				Badges:     metric.Badges,
			}
		}
		out[category.Name] = metrics
	}
	content, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(content) + "\n", nil
}

func ExportSummaryCSV(summary SummaryReport) (string, error) {
	var b bytes.Buffer
	writer := csv.NewWriter(&b)
	if err := writer.Write([]string{"name", "value"}); err != nil {
		return "", err
	}
	names := SummaryCSVMetricNames(summary)
	for _, category := range summary.Categories {
		for _, metric := range category.Metrics {
			if err := writer.Write([]string{names[metric.ID], metric.DisplayValue}); err != nil {
				return "", err
			}
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return b.String(), nil
}

func SummaryCSVMetricNames(summary SummaryReport) map[string]string {
	return summaryCSVNames(summary)
}

func summaryCSVNames(summary SummaryReport) map[string]string {
	out := map[string]string{}
	seen := map[string]int{}
	for _, category := range summary.Categories {
		for _, metric := range category.Metrics {
			name := summaryCSVVariableName(metric)
			seen[name]++
			if seen[name] > 1 {
				name = fmt.Sprintf("%s_%d", name, seen[name])
			}
			out[metric.ID] = summaryCSVMetricName(name, summaryCSVUnitLabel(metric.Unit))
		}
	}
	return out
}

func summaryCSVVariableName(metric SummaryMetric) string {
	return trimSummaryCSVUnitSuffix(metric.ID, metric.Unit)
}

func summaryCSVMetricName(name string, unit string) string {
	return name + " [" + unit + "]"
}

func summaryCSVUnitLabel(unit string) string {
	unit = strings.TrimSpace(unit)
	if strings.HasPrefix(unit, "[") && strings.HasSuffix(unit, "]") {
		unit = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(unit, "["), "]"))
	}
	if unit == "" {
		return "-"
	}
	return unit
}

func trimSummaryCSVUnitSuffix(id string, unit string) string {
	suffixes := summaryCSVUnitSuffixes(summaryCSVUnitLabel(unit))
	for _, suffix := range suffixes {
		if strings.HasSuffix(id, suffix) {
			return strings.TrimSuffix(id, suffix)
		}
	}
	return id
}

func summaryCSVUnitSuffixes(unit string) []string {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "-":
		return nil
	case "%":
		return []string{"_percent", "_pct"}
	case "deg":
		return []string{"_deg"}
	case "h":
		return []string{"_h"}
	case "m":
		return []string{"_m"}
	case "m2":
		return []string{"_m2"}
	case "m3":
		return []string{"_m3"}
	case "w":
		return []string{"_w"}
	case "w/m2":
		return []string{"_w_per_m2"}
	case "people/100m2":
		return []string{"_people_per_100m2", "_per_100m2"}
	default:
		return nil
	}
}

type summaryMetricValue struct {
	Value        any
	DisplayValue string
	Status       string
}

func countSummaryValue(value int) summaryMetricValue {
	return summaryMetricValue{Value: value, DisplayValue: strconv.Itoa(value), Status: summaryStatusOK}
}

func stringSummaryValue(value string) summaryMetricValue {
	value = strings.TrimSpace(value)
	if value == "" {
		return missingSummaryValue()
	}
	return summaryMetricValue{Value: value, DisplayValue: value, Status: summaryStatusOK}
}

func numberSummaryValue(value float64, precision int, status string) summaryMetricValue {
	if !isFinitePositiveOrZero(value) {
		return missingSummaryValue()
	}
	if status == "" {
		status = summaryStatusOK
	}
	return summaryMetricValue{Value: roundedNumber(value, precision), DisplayValue: formatSummaryNumber(value, precision), Status: status}
}

func ratioSummaryValue(numerator, denominator float64, precision int, status string) summaryMetricValue {
	if denominator <= 0 || !isFinitePositiveOrZero(numerator) {
		return missingSummaryValue()
	}
	return numberSummaryValue(numerator/denominator, precision, status)
}

func percentSummaryValue(numerator, denominator float64, precision int, status string) summaryMetricValue {
	if denominator <= 0 || !isFinitePositiveOrZero(numerator) {
		return missingSummaryValue()
	}
	return numberSummaryValue(numerator/denominator*100, precision, status)
}

func missingSummaryValue() summaryMetricValue {
	return summaryMetricValue{DisplayValue: "N/A", Status: summaryStatusMissing}
}

func roundedNumber(value float64, precision int) float64 {
	if precision <= 0 {
		return math.Round(value)
	}
	scale := math.Pow10(precision)
	return math.Round(value*scale) / scale
}

func formatSummaryNumber(value float64, precision int) string {
	if precision <= 0 {
		return strconv.FormatInt(int64(math.Round(value)), 10)
	}
	displayPrecision := summaryDisplayPrecision(value, precision)
	rounded := roundedNumber(value, displayPrecision)
	if rounded == 0 {
		rounded = 0
	}
	return strconv.FormatFloat(rounded, 'f', displayPrecision, 64)
}

func summaryDisplayPrecision(value float64, precision int) int {
	if precision <= 0 {
		return 0
	}
	rounded := roundedNumber(value, precision)
	if rounded == math.Round(rounded) {
		return min(precision, 1)
	}
	if summaryDisplayedDigitCount(value, precision) >= 4 {
		return min(precision, 1)
	}
	return precision
}

func summaryDisplayedDigitCount(value float64, precision int) int {
	text := strconv.FormatFloat(math.Abs(roundedNumber(value, precision)), 'f', precision, 64)
	count := 0
	seenSignificant := false
	for _, char := range text {
		if char == '.' {
			continue
		}
		if char == '0' && !seenSignificant {
			continue
		}
		seenSignificant = true
		count++
	}
	return count
}

func isFinitePositiveOrZero(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

type summaryFacts struct {
	version                     string
	buildingName                string
	buildingNorthAxis           float64
	hasBuildingNorthAxis        bool
	objectCount                 int
	objectTypeCount             int
	zoneCount                   int
	spaceCount                  int
	scheduleCount               int
	constructionCount           int
	materialCount               int
	zoneMultipliers             map[string]float64
	zoneDirections              map[string]float64
	zoneFloorAreas              map[string]float64
	zoneHeights                 map[string]float64
	declaredZoneVolumes         map[string]float64
	zoneLists                   map[string][]string
	conditionedZones            map[string]bool
	conditionedZoneEvidence     map[string]map[string]bool
	spaceZones                  map[string]string
	referencedSchedules         map[string]bool
	scheduleHours               map[string]float64
	supportedScheduleCount      int
	unsupportedScheduleCount    int
	internalLoadObjectCount     int
	totalPeople                 float64
	hasPeople                   bool
	peoplePartial               bool
	totalLightingPower          float64
	hasLightingPower            bool
	lightingPartial             bool
	totalEquipmentPower         float64
	hasEquipmentPower           bool
	equipmentPartial            bool
	internalLoadResolvedCount   int
	internalLoadUnresolvedCount int
	hvacObjectCount             int
	zoneHVACObjectCount         int
	thermostatCount             int
	hvacNodeConnectionCount     int
	heavyReadinessCaptured      bool
	geometryObjectCount         int
	detailedGeometryCount       int
	profileReferenceCount       int
	supportedProfileReferences  int
	hvacRuleEdgeCount           int
	diagnosticSourceCounts      map[string]int
	outputRequestCount          int
	recognizedOutputCount       int
	grossFloorArea              float64
	hasGrossFloorArea           bool
	footprintArea               float64
	hasFootprintArea            bool
	boundingBoxArea             float64
	hasBoundingBoxArea          bool
	totalZoneVolume             float64
	hasZoneVolume               bool
	zoneVolumePartial           bool
	averageFloorHeight          float64
	hasAverageFloorHeight       bool
	averageFloorHeightPartial   bool
	buildingLongSide            float64
	buildingShortSide           float64
	hasBuildingSides            bool
	envelopeArea                float64
	hasEnvelopeArea             bool
	exteriorWallArea            float64
	hasExteriorWallArea         bool
	roofArea                    float64
	hasRoofArea                 bool
	groundFloorArea             float64
	hasGroundFloorArea          bool
	windowArea                  float64
	hasWindowArea               bool
	doorArea                    float64
	hasDoorArea                 bool
	skylightArea                float64
	fenestrationMissingBase     int
	orientationFieldCount       int
	orientationComputedCount    int
	orientationMissingCount     int
	windowBaseOrientationCount  int
	wallAreaByOrientation       map[string]float64
	windowAreaByOrientation     map[string]float64
	bounds                      geometryBounds
	vertexEntryDirection        string
	surfaces                    []surfaceInfo
	floorSurfaces               []surfaceInfo
	lowestFloorSurfaces         []surfaceInfo
}

type geometryBounds struct {
	minX, maxX  float64
	minY, maxY  float64
	initialized bool
}

func (b *geometryBounds) add(point point3) {
	if !b.initialized {
		b.minX, b.maxX = point.x, point.x
		b.minY, b.maxY = point.y, point.y
		b.initialized = true
		return
	}
	b.minX = math.Min(b.minX, point.x)
	b.maxX = math.Max(b.maxX, point.x)
	b.minY = math.Min(b.minY, point.y)
	b.maxY = math.Max(b.maxY, point.y)
}

type surfaceInfo struct {
	name          string
	surfaceType   string
	zoneName      string
	outside       string
	area          float64
	azimuth       float64
	azimuthSource string
	orientation   string
	minZ          float64
	maxZ          float64
	avgZ          float64
	exterior      bool
	ground        bool
}

func collectSummaryFacts(doc Document) summaryFacts {
	return collectSummaryFactsWithOptions(doc, SummaryAnalysisOptions{IncludeHeavyReadiness: true})
}

func collectSummaryFactsWithOptions(doc Document, options SummaryAnalysisOptions) summaryFacts {
	facts := summaryFacts{
		objectCount:             len(doc.Objects),
		zoneMultipliers:         map[string]float64{},
		zoneDirections:          map[string]float64{},
		zoneFloorAreas:          map[string]float64{},
		zoneHeights:             map[string]float64{},
		declaredZoneVolumes:     map[string]float64{},
		zoneLists:               map[string][]string{},
		conditionedZones:        map[string]bool{},
		conditionedZoneEvidence: map[string]map[string]bool{},
		spaceZones:              map[string]string{},
		referencedSchedules:     map[string]bool{},
		scheduleHours:           map[string]float64{},
		diagnosticSourceCounts:  map[string]int{},
		wallAreaByOrientation:   map[string]float64{"north": 0, "east": 0, "south": 0, "west": 0},
		windowAreaByOrientation: map[string]float64{"north": 0, "east": 0, "south": 0, "west": 0},
		vertexEntryDirection:    "counterclockwise",
	}

	typeCounts := map[string]int{}
	for _, obj := range doc.Objects {
		typeCounts[obj.Type]++
		facts.captureInventoryObject(obj)
	}
	facts.objectTypeCount = len(typeCounts)

	for _, obj := range doc.Objects {
		if !isBuildingSurfaceType(obj.Type) {
			continue
		}
		surface, ok := facts.surfaceInfo(obj)
		if !ok {
			continue
		}
		facts.captureSurface(surface)
	}
	facts.finalizeGeometry()

	for _, obj := range doc.Objects {
		facts.captureConditioningAndSchedules(obj)
	}
	for _, obj := range doc.Objects {
		facts.captureScheduleHours(obj)
	}
	facts.unsupportedScheduleCount = facts.scheduleCount - facts.supportedScheduleCount
	if facts.unsupportedScheduleCount < 0 {
		facts.unsupportedScheduleCount = 0
	}

	for _, obj := range doc.Objects {
		if strings.EqualFold(obj.Type, "People") {
			facts.capturePeople(obj)
		}
	}
	for _, obj := range doc.Objects {
		switch {
		case strings.EqualFold(obj.Type, "Lights"):
			facts.captureLighting(obj)
		case strings.EqualFold(obj.Type, "ElectricEquipment"):
			facts.captureEquipment(obj)
		}
	}

	surfaceByName := facts.surfaceMap()
	for _, obj := range doc.Objects {
		if !isFenestrationType(obj.Type) {
			continue
		}
		facts.captureFenestration(obj, surfaceByName)
	}

	facts.finalizeVolumeAndHeights()
	facts.captureReadiness(doc, options.IncludeHeavyReadiness)
	return facts
}

func (facts *summaryFacts) captureReadiness(doc Document, includeHeavy bool) {
	for _, obj := range doc.Objects {
		if isBuildingSurfaceType(obj.Type) || isFenestrationType(obj.Type) {
			facts.geometryObjectCount++
			if _, ok := detailedVertices(obj); ok {
				facts.detailedGeometryCount++
			}
		}
	}

	facts.profileReferenceCount = len(facts.referencedSchedules)
	for schedule := range facts.referencedSchedules {
		if _, ok := facts.scheduleHours[schedule]; ok {
			facts.supportedProfileReferences++
		}
	}

	if !includeHeavy {
		return
	}
	facts.heavyReadinessCaptured = true
	hvacReport := AnalyzeHVAC(doc)
	facts.hvacRuleEdgeCount = len(hvacReport.RuleGraph.Edges)
	facts.hvacNodeConnectionCount = summaryHVACTypedNodeConnectionCount(hvacReport)

	for _, diagnostic := range AnalyzeDiagnostics(doc) {
		source := strings.TrimSpace(diagnostic.Source)
		if source == "" {
			source = "unspecified"
		}
		facts.diagnosticSourceCounts[source]++
	}

	outputReport := AnalyzeOutput(doc)
	facts.outputRequestCount = len(outputReport.Recommendations)
	for _, recommendation := range outputReport.Recommendations {
		if recommendation.Exists {
			facts.recognizedOutputCount++
		}
	}
}

func summaryHVACTypedNodeConnectionCount(report HVACReport) int {
	count := 0
	for _, loop := range report.Loops {
		for _, side := range []HVACLoopSide{loop.SupplySide, loop.DemandSide} {
			for _, branch := range side.Branches {
				for _, component := range branch.Components {
					if component.InletNode != "" && component.OutletNode != "" {
						count++
					}
					if component.WaterInletNode != "" && component.WaterOutletNode != "" {
						count++
					}
				}
			}
		}
	}
	return count
}

func (facts *summaryFacts) captureInventoryObject(obj Object) {
	lowerType := strings.ToLower(obj.Type)
	name := objectName(obj)

	if strings.EqualFold(obj.Type, "Version") {
		facts.version = findFieldByCommentWords(obj, "version", "identifier")
		if facts.version == "" && len(obj.Fields) > 0 {
			facts.version = strings.TrimSpace(obj.Fields[0].Value)
		}
	}

	if strings.EqualFold(obj.Type, "Building") {
		if facts.buildingName == "" {
			facts.buildingName = name
			if facts.buildingName == "" {
				facts.buildingName = findFieldByCommentWords(obj, "name")
			}
			if facts.buildingName == "" && len(obj.Fields) > 0 {
				facts.buildingName = strings.TrimSpace(obj.Fields[0].Value)
			}
		}
		if value, ok := parseFloatField(findFieldByCommentWords(obj, "north", "axis")); ok {
			facts.buildingNorthAxis = value
			facts.hasBuildingNorthAxis = true
		}
	}

	if strings.EqualFold(obj.Type, "GlobalGeometryRules") {
		if value := findFieldByCommentWords(obj, "vertex", "entry", "direction"); value != "" {
			facts.vertexEntryDirection = strings.ToLower(strings.TrimSpace(value))
		}
	}

	if strings.EqualFold(obj.Type, "Zone") {
		facts.zoneCount++
		zoneKey := normalizeName(name)
		if zoneKey != "" {
			facts.zoneMultipliers[zoneKey] = numericFieldOrDefault(obj, 1, "multiplier")
			facts.zoneDirections[zoneKey] = numericFieldOrDefault(obj, 0, "direction", "relative", "north")
			if value, ok := parseFloatField(findFieldByCommentWords(obj, "floor", "area")); ok {
				facts.zoneFloorAreas[zoneKey] = value * facts.zoneMultipliers[zoneKey]
			}
			if value, ok := parseFloatField(findFieldByCommentWords(obj, "ceiling", "height")); ok {
				facts.zoneHeights[zoneKey] = value
			}
			if value, ok := parseFloatField(findFieldByCommentWords(obj, "volume")); ok {
				facts.declaredZoneVolumes[zoneKey] = value * facts.zoneMultipliers[zoneKey]
			}
		}
	}

	if strings.EqualFold(obj.Type, "Space") {
		facts.spaceCount++
		spaceKey := normalizeName(name)
		if spaceKey != "" {
			zoneName := findFieldByCommentWords(obj, "zone", "name")
			if zoneName == "" {
				zoneName = fieldValueByCatalogName(obj, "Zone Name")
			}
			if zoneName == "" && len(obj.Fields) > 1 {
				zoneName = strings.TrimSpace(obj.Fields[1].Value)
			}
			if zoneName != "" {
				facts.spaceZones[spaceKey] = zoneName
			}
		}
	}
	if isScheduleType(obj.Type) {
		facts.scheduleCount++
	}
	if strings.HasPrefix(lowerType, "construction") {
		facts.constructionCount++
	}
	if strings.HasPrefix(lowerType, "material") || strings.HasPrefix(lowerType, "windowmaterial") {
		facts.materialCount++
	}
	if strings.EqualFold(obj.Type, "ZoneList") && name != "" {
		facts.zoneLists[normalizeName(name)] = zoneListMembers(obj)
	}
}

func (facts *summaryFacts) captureSurface(surface surfaceInfo) {
	if surface.area <= 0 {
		return
	}
	facts.surfaces = append(facts.surfaces, surface)
	if surface.zoneName != "" {
		zoneKey := normalizeName(surface.zoneName)
		if _, ok := facts.zoneMultipliers[zoneKey]; !ok {
			facts.zoneMultipliers[zoneKey] = 1
		}
	}

	switch strings.ToLower(surface.surfaceType) {
	case "floor":
		facts.floorSurfaces = append(facts.floorSurfaces, surface)
		if surface.zoneName != "" {
			facts.grossFloorArea += surface.area
			facts.hasGrossFloorArea = true
			facts.zoneFloorAreas[normalizeName(surface.zoneName)] += surface.area
		}
		if surface.ground {
			facts.groundFloorArea += surface.area
			facts.hasGroundFloorArea = true
		}
	case "wall":
		if surface.exterior {
			facts.exteriorWallArea += surface.area
			facts.hasExteriorWallArea = true
			if surface.orientation != "" {
				facts.wallAreaByOrientation[surface.orientation] += surface.area
			}
			facts.recordOrientationSource(surface.azimuthSource)
		}
	case "roof", "roofceiling", "ceiling":
		if surface.exterior {
			facts.roofArea += surface.area
			facts.hasRoofArea = true
		}
	}

	if surface.exterior || surface.ground {
		if strings.EqualFold(surface.surfaceType, "Wall") ||
			strings.EqualFold(surface.surfaceType, "Roof") ||
			strings.EqualFold(surface.surfaceType, "RoofCeiling") ||
			strings.EqualFold(surface.surfaceType, "Ceiling") ||
			strings.EqualFold(surface.surfaceType, "Floor") {
			facts.envelopeArea += surface.area
			facts.hasEnvelopeArea = true
		}
	}
}

func (facts *summaryFacts) finalizeGeometry() {
	if !facts.hasGroundFloorArea {
		facts.lowestFloorSurfaces = lowestFloorSurfaces(facts.floorSurfaces)
		for _, surface := range facts.lowestFloorSurfaces {
			facts.groundFloorArea += surface.area
		}
		facts.hasGroundFloorArea = facts.groundFloorArea > 0
	}

	if facts.hasGroundFloorArea {
		facts.footprintArea = facts.groundFloorArea
		facts.hasFootprintArea = true
	}

	if facts.bounds.initialized {
		width := facts.bounds.maxX - facts.bounds.minX
		depth := facts.bounds.maxY - facts.bounds.minY
		if width > 0 && depth > 0 {
			facts.boundingBoxArea = width * depth
			facts.hasBoundingBoxArea = true
		}
		longSide := math.Max(width, depth)
		shortSide := math.Min(width, depth)
		if longSide > 0 && shortSide > 0 {
			facts.buildingLongSide = longSide
			facts.buildingShortSide = shortSide
			facts.hasBuildingSides = true
		}
	}
}

func (facts *summaryFacts) captureConditioningAndSchedules(obj Object) {
	lowerType := strings.ToLower(obj.Type)
	if isHVACSummaryType(lowerType) {
		facts.hvacObjectCount++
	}
	if strings.HasPrefix(lowerType, "zonehvac:") {
		facts.zoneHVACObjectCount++
	}
	if strings.EqualFold(obj.Type, "ZoneControl:Thermostat") || strings.HasPrefix(lowerType, "thermostatsetpoint:") {
		facts.thermostatCount++
	}
	switch {
	case strings.EqualFold(obj.Type, "ZoneHVAC:EquipmentConnections"):
		facts.markConditionedZones(facts.objectTargetZones(obj), "by_equipment_connections")
	case strings.HasPrefix(lowerType, "zonehvac:"):
		facts.markConditionedZones(facts.objectTargetZones(obj), "by_zone_hvac")
	case strings.EqualFold(obj.Type, "ZoneControl:Thermostat"):
		facts.markConditionedZones(facts.objectTargetZones(obj), "by_thermostat")
	case strings.HasPrefix(lowerType, "spacehvac:"):
		facts.markConditionedZones(facts.objectTargetSpaceZones(obj), "by_space_hvac")
	}
	for _, scheduleName := range referencedScheduleNames(obj) {
		facts.referencedSchedules[normalizeName(scheduleName)] = true
	}
}

func (facts *summaryFacts) markConditionedZones(zones []string, evidence string) {
	evidence = strings.TrimSpace(evidence)
	if evidence == "" {
		evidence = "by_unspecified"
	}
	if facts.conditionedZoneEvidence[evidence] == nil {
		facts.conditionedZoneEvidence[evidence] = map[string]bool{}
	}
	for _, zone := range zones {
		zoneKey := normalizeName(zone)
		if zoneKey == "" {
			continue
		}
		facts.conditionedZones[zoneKey] = true
		facts.conditionedZoneEvidence[evidence][zoneKey] = true
	}
}

func (facts *summaryFacts) captureScheduleHours(obj Object) {
	if !isScheduleType(obj.Type) {
		return
	}
	hours, ok := annualScheduleHours(obj)
	if !ok {
		return
	}
	name := objectName(obj)
	if name == "" {
		return
	}
	facts.scheduleHours[normalizeName(name)] = hours
	facts.supportedScheduleCount++
}

func (facts *summaryFacts) capturePeople(obj Object) {
	facts.internalLoadObjectCount++
	value, ok, partial := facts.peopleObjectValue(obj)
	if !ok {
		facts.peoplePartial = true
		facts.internalLoadUnresolvedCount++
		return
	}
	facts.internalLoadResolvedCount++
	facts.totalPeople += value
	facts.hasPeople = true
	if partial {
		facts.peoplePartial = true
	}
}

func (facts *summaryFacts) captureLighting(obj Object) {
	facts.internalLoadObjectCount++
	value, ok, partial := facts.designPowerValue(obj, "lighting")
	if !ok {
		facts.lightingPartial = true
		facts.internalLoadUnresolvedCount++
		return
	}
	facts.internalLoadResolvedCount++
	facts.totalLightingPower += value
	facts.hasLightingPower = true
	if partial {
		facts.lightingPartial = true
	}
}

func (facts *summaryFacts) captureEquipment(obj Object) {
	facts.internalLoadObjectCount++
	value, ok, partial := facts.designPowerValue(obj, "equipment")
	if !ok {
		facts.equipmentPartial = true
		facts.internalLoadUnresolvedCount++
		return
	}
	facts.internalLoadResolvedCount++
	facts.totalEquipmentPower += value
	facts.hasEquipmentPower = true
	if partial {
		facts.equipmentPartial = true
	}
}

func (facts *summaryFacts) captureFenestration(obj Object, surfaceByName map[string]surfaceInfo) {
	area, ok := objectArea(obj)
	if !ok || area <= 0 {
		return
	}
	baseSurfaceName := findFieldByCommentWords(obj, "surface", "name")
	baseSurface, hasBaseSurface := surfaceByName[normalizeName(baseSurfaceName)]
	multiplier := numericFieldOrDefault(obj, 1, "multiplier")
	if hasBaseSurface {
		area *= multiplier
	} else {
		area *= multiplier
	}
	if hasBaseSurface {
		area *= zoneMultiplierFor(facts.zoneMultipliers, baseSurface.zoneName)
	}

	fenestrationType := fenestrationSurfaceType(obj)
	lowerType := strings.ToLower(fenestrationType)
	isWindow := strings.Contains(lowerType, "window") || strings.Contains(lowerType, "glassdoor") || strings.Contains(lowerType, "glass door")
	isDoor := strings.Contains(lowerType, "door") && !isWindow

	if isWindow {
		facts.windowArea += area
		facts.hasWindowArea = true
		orientation := ""
		if hasBaseSurface {
			orientation = baseSurface.orientation
			if strings.EqualFold(baseSurface.surfaceType, "roof") || strings.EqualFold(baseSurface.surfaceType, "roofceiling") || strings.EqualFold(baseSurface.surfaceType, "ceiling") {
				facts.skylightArea += area
			}
			if orientation != "" {
				facts.windowBaseOrientationCount++
			}
		} else {
			facts.fenestrationMissingBase++
		}
		if orientation == "" {
			if azimuth, source, ok := facts.objectAzimuthSource(obj, ""); ok {
				orientation = orientationFromAzimuth(azimuth)
				facts.recordOrientationSource(source)
			} else {
				facts.orientationMissingCount++
			}
		}
		if orientation != "" {
			facts.windowAreaByOrientation[orientation] += area
		}
	}
	if isDoor {
		facts.doorArea += area
		facts.hasDoorArea = true
	}
}

func (facts *summaryFacts) finalizeVolumeAndHeights() {
	zoneNames := map[string]bool{}
	for zone := range facts.zoneMultipliers {
		zoneNames[zone] = true
	}
	for zone := range facts.zoneFloorAreas {
		zoneNames[zone] = true
	}

	var heightSum float64
	var heightCount int
	for zone := range zoneNames {
		if height, ok := facts.zoneHeights[zone]; ok && height > 0 {
			heightSum += height
			heightCount++
			continue
		}
		if height, ok := facts.geometryHeightForZone(zone); ok {
			facts.zoneHeights[zone] = height
			heightSum += height
			heightCount++
			facts.averageFloorHeightPartial = true
		}
	}
	if heightCount > 0 {
		facts.averageFloorHeight = heightSum / float64(heightCount)
		facts.hasAverageFloorHeight = true
	}

	for zone := range zoneNames {
		if volume, ok := facts.declaredZoneVolumes[zone]; ok {
			facts.totalZoneVolume += volume
			facts.hasZoneVolume = true
			continue
		}
		area, hasArea := facts.zoneFloorAreas[zone]
		height, hasHeight := facts.zoneHeights[zone]
		if hasArea && hasHeight && area > 0 && height > 0 {
			facts.totalZoneVolume += area * height
			facts.hasZoneVolume = true
			facts.zoneVolumePartial = true
		} else if len(zoneNames) > 0 {
			facts.zoneVolumePartial = true
		}
	}
}

func (facts summaryFacts) metricValues() map[string]summaryMetricValue {
	values := map[string]summaryMetricValue{
		"energyplus_version":                  stringSummaryValue(facts.version),
		"building_name":                       stringSummaryValue(facts.buildingName),
		"object_count":                        countSummaryValue(facts.objectCount),
		"object_type_count":                   countSummaryValue(facts.objectTypeCount),
		"zone_count":                          countSummaryValue(facts.zoneCount),
		"space_count":                         countSummaryValue(facts.spaceCount),
		"schedule_count":                      countSummaryValue(facts.scheduleCount),
		"construction_count":                  countSummaryValue(facts.constructionCount),
		"material_count":                      countSummaryValue(facts.materialCount),
		"internal_load_object_count":          countSummaryValue(facts.internalLoadObjectCount),
		"internal_load_method_coverage":       summaryInternalLoadCoverageValue(facts.internalLoadResolvedCount, facts.internalLoadObjectCount, facts.internalLoadUnresolvedCount),
		"referenced_schedule_count":           countSummaryValue(len(facts.referencedSchedules)),
		"supported_schedule_count":            countSummaryValue(facts.supportedScheduleCount),
		"unsupported_schedule_count":          countSummaryValue(facts.unsupportedScheduleCount),
		"hvac_object_count":                   countSummaryValue(facts.hvacObjectCount),
		"zone_hvac_object_count":              countSummaryValue(facts.zoneHVACObjectCount),
		"thermostat_count":                    countSummaryValue(facts.thermostatCount),
		"conditioned_zone_count":              countSummaryValue(len(facts.conditionedZones)),
		"conditioned_zone_evidence_breakdown": stringSummaryValue(summaryConditionedEvidenceDisplay(facts.conditionedZoneEvidence)),
	}
	if facts.heavyReadinessCaptured {
		values["hvac_node_connection_count"] = countSummaryValue(facts.hvacNodeConnectionCount)
		values["diagnostics_by_source"] = stringSummaryValue(summarySourceCountsDisplay(facts.diagnosticSourceCounts))
	}

	if facts.hasBuildingNorthAxis {
		values["building_north_axis_deg"] = numberSummaryValue(facts.buildingNorthAxis, precisionFor("building_north_axis_deg"), summaryStatusOK)
	} else {
		values["building_north_axis_deg"] = missingSummaryValue()
	}
	if facts.hasGrossFloorArea {
		values["gross_floor_area_m2"] = numberSummaryValue(facts.grossFloorArea, precisionFor("gross_floor_area_m2"), summaryStatusOK)
	}
	conditionedArea, hasConditionedArea := facts.conditionedFloorArea()
	if hasConditionedArea {
		values["conditioned_floor_area_m2"] = numberSummaryValue(conditionedArea, precisionFor("conditioned_floor_area_m2"), floorAreaStatus(conditionedArea, facts.grossFloorArea, facts.hasGrossFloorArea))
	}
	if hasConditionedArea && facts.hasGrossFloorArea {
		unconditioned := math.Max(0, facts.grossFloorArea-conditionedArea)
		values["unconditioned_floor_area_m2"] = numberSummaryValue(unconditioned, precisionFor("unconditioned_floor_area_m2"), floorAreaStatus(conditionedArea, facts.grossFloorArea, true))
	}
	if facts.hasFootprintArea {
		values["footprint_area_m2"] = numberSummaryValue(facts.footprintArea, precisionFor("footprint_area_m2"), summaryStatusOK)
	}
	if facts.hasBoundingBoxArea {
		values["bounding_box_area_m2"] = numberSummaryValue(facts.boundingBoxArea, precisionFor("bounding_box_area_m2"), summaryStatusOK)
	}
	if facts.hasZoneVolume {
		values["total_zone_volume_m3"] = numberSummaryValue(facts.totalZoneVolume, precisionFor("total_zone_volume_m3"), partialIf(facts.zoneVolumePartial))
	}
	if facts.hasAverageFloorHeight {
		values["average_floor_height_m"] = numberSummaryValue(facts.averageFloorHeight, precisionFor("average_floor_height_m"), partialIf(facts.averageFloorHeightPartial))
	}
	if facts.hasBuildingSides {
		values["building_long_side_m"] = numberSummaryValue(facts.buildingLongSide, precisionFor("building_long_side_m"), summaryStatusOK)
		values["building_short_side_m"] = numberSummaryValue(facts.buildingShortSide, precisionFor("building_short_side_m"), summaryStatusOK)
		values["footprint_aspect_ratio"] = ratioSummaryValue(facts.buildingLongSide, facts.buildingShortSide, precisionFor("footprint_aspect_ratio"), summaryStatusOK)
	}
	if facts.hasEnvelopeArea {
		values["envelope_area_m2"] = numberSummaryValue(facts.envelopeArea, precisionFor("envelope_area_m2"), summaryStatusOK)
		netOpaque := math.Max(0, facts.envelopeArea-facts.windowArea-facts.doorArea)
		values["net_opaque_envelope_area_m2"] = numberSummaryValue(netOpaque, precisionFor("net_opaque_envelope_area_m2"), summaryStatusOK)
	}
	if facts.hasEnvelopeArea && facts.hasZoneVolume {
		values["envelope_area_to_volume_ratio"] = ratioSummaryValue(facts.envelopeArea, facts.totalZoneVolume, precisionFor("envelope_area_to_volume_ratio"), partialIf(facts.zoneVolumePartial))
	}
	if facts.hasGrossFloorArea && facts.hasZoneVolume {
		values["floor_area_to_volume_ratio"] = ratioSummaryValue(facts.grossFloorArea, facts.totalZoneVolume, precisionFor("floor_area_to_volume_ratio"), partialIf(facts.zoneVolumePartial))
	}
	if facts.hasExteriorWallArea {
		values["exterior_wall_area_m2"] = numberSummaryValue(facts.exteriorWallArea, precisionFor("exterior_wall_area_m2"), summaryStatusOK)
		values["total_wwr_percent"] = percentSummaryValue(facts.windowArea, facts.exteriorWallArea, precisionFor("total_wwr_percent"), partialIf(!facts.hasWindowArea))
	}
	if facts.hasRoofArea {
		values["roof_area_m2"] = numberSummaryValue(facts.roofArea, precisionFor("roof_area_m2"), summaryStatusOK)
		values["skylight_roof_ratio_percent"] = percentSummaryValue(facts.skylightArea, facts.roofArea, precisionFor("skylight_roof_ratio_percent"), partialIf(facts.fenestrationMissingBase > 0))
	}
	if facts.hasGroundFloorArea {
		values["ground_floor_area_m2"] = numberSummaryValue(facts.groundFloorArea, precisionFor("ground_floor_area_m2"), summaryStatusOK)
	}
	if facts.hasWindowArea {
		values["window_area_m2"] = numberSummaryValue(facts.windowArea, precisionFor("window_area_m2"), summaryStatusOK)
	}
	if facts.hasDoorArea {
		values["door_area_m2"] = numberSummaryValue(facts.doorArea, precisionFor("door_area_m2"), summaryStatusOK)
	}
	for _, orientation := range []string{"north", "east", "south", "west"} {
		id := orientation + "_wwr_percent"
		if facts.wallAreaByOrientation[orientation] > 0 {
			values[id] = percentSummaryValue(facts.windowAreaByOrientation[orientation], facts.wallAreaByOrientation[orientation], precisionFor(id), partialIf(facts.orientationMissingCount > 0))
		}
	}
	if facts.hasLightingPower {
		values["total_lighting_power_w"] = numberSummaryValue(facts.totalLightingPower, precisionFor("total_lighting_power_w"), partialIf(facts.lightingPartial))
		if area, ok := facts.loadDensityArea(); ok {
			values["average_lighting_power_density_w_per_m2"] = ratioSummaryValue(facts.totalLightingPower, area, precisionFor("average_lighting_power_density_w_per_m2"), partialIf(facts.lightingPartial))
		}
	}
	if facts.hasEquipmentPower {
		values["total_equipment_power_w"] = numberSummaryValue(facts.totalEquipmentPower, precisionFor("total_equipment_power_w"), partialIf(facts.equipmentPartial))
		if area, ok := facts.loadDensityArea(); ok {
			values["average_equipment_power_density_w_per_m2"] = ratioSummaryValue(facts.totalEquipmentPower, area, precisionFor("average_equipment_power_density_w_per_m2"), partialIf(facts.equipmentPartial))
		}
	}
	if facts.hasPeople {
		values["total_people"] = numberSummaryValue(facts.totalPeople, precisionFor("total_people"), partialIf(facts.peoplePartial))
		if area, ok := facts.loadDensityArea(); ok {
			values["people_density_per_100m2"] = numberSummaryValue(facts.totalPeople/area*100, precisionFor("people_density_per_100m2"), partialIf(facts.peoplePartial))
		}
	}
	if hours, ok := facts.modelOperatingHours(); ok {
		values["model_operating_hours_h"] = numberSummaryValue(hours, precisionFor("model_operating_hours_h"), partialIf(facts.unsupportedScheduleCount > 0))
	}
	if hours, ok := facts.averageScheduleHours(); ok {
		values["average_schedule_operating_hours_h"] = numberSummaryValue(hours, precisionFor("average_schedule_operating_hours_h"), partialIf(facts.unsupportedScheduleCount > 0))
	}
	if facts.geometryObjectCount > 0 {
		values["geometry_coverage_percent"] = percentSummaryValue(float64(facts.detailedGeometryCount), float64(facts.geometryObjectCount), precisionFor("geometry_coverage_percent"), partialIf(facts.detailedGeometryCount < facts.geometryObjectCount))
	} else {
		values["geometry_coverage_percent"] = missingSummaryValue()
	}
	if facts.profileReferenceCount > 0 {
		values["profile_coverage_percent"] = percentSummaryValue(float64(facts.supportedProfileReferences), float64(facts.profileReferenceCount), precisionFor("profile_coverage_percent"), partialIf(facts.supportedProfileReferences < facts.profileReferenceCount))
	}
	if facts.heavyReadinessCaptured {
		values["hvac_rule_edge_count"] = countSummaryValue(facts.hvacRuleEdgeCount)
		if facts.outputRequestCount > 0 {
			values["output_readiness_percent"] = percentSummaryValue(float64(facts.recognizedOutputCount), float64(facts.outputRequestCount), precisionFor("output_readiness_percent"), partialIf(facts.recognizedOutputCount < facts.outputRequestCount))
		} else {
			values["output_readiness_percent"] = missingSummaryValue()
		}
	}

	for _, definition := range summaryDefinitions {
		if _, ok := values[definition.ID]; !ok {
			values[definition.ID] = missingSummaryValue()
		}
	}
	return values
}

func precisionFor(metricID string) int {
	for _, definition := range summaryDefinitions {
		if definition.ID == metricID {
			return definition.Precision
		}
	}
	return 2
}

func partialIf(condition bool) string {
	if condition {
		return summaryStatusPartial
	}
	return summaryStatusOK
}

func summaryMetricSource(metricID string, definitionSource string) string {
	switch metricID {
	case "geometry_coverage_percent", "profile_coverage_percent", "output_readiness_percent":
		return "analyzer_readiness"
	case "north_wwr_percent", "east_wwr_percent", "south_wwr_percent", "west_wwr_percent":
		return "surface_azimuth"
	case "skylight_roof_ratio_percent":
		return "base_surface_resolution"
	case "bounding_box_area_m2":
		return "analyzer_inference"
	case "internal_load_method_coverage":
		return "analyzer_coverage"
	case "conditioned_zone_count", "conditioned_zone_evidence_breakdown":
		return "hvac_semantic_evidence"
	case "hvac_rule_edge_count":
		return "energyplus_rule_graph"
	case "diagnostics_by_source":
		return "diagnostics"
	}
	if strings.Contains(strings.ToLower(definitionSource), "fallback") || strings.Contains(strings.ToLower(definitionSource), "detection") {
		return "analyzer_inference"
	}
	return "idf_fields"
}

func summaryMetricConfidence(metricID string, status string) string {
	if status == summaryStatusMissing {
		return "missing"
	}
	switch metricID {
	case "conditioned_floor_area_m2", "unconditioned_floor_area_m2", "conditioned_zone_count", "conditioned_zone_evidence_breakdown":
		if status == summaryStatusPartial {
			return "partial"
		}
		return "inferred"
	case "north_wwr_percent", "east_wwr_percent", "south_wwr_percent", "west_wwr_percent", "skylight_roof_ratio_percent":
		if status == summaryStatusPartial {
			return "partial"
		}
		return "computed"
	case "bounding_box_area_m2", "building_long_side_m", "building_short_side_m", "footprint_aspect_ratio", "model_operating_hours_h", "average_schedule_operating_hours_h":
		if status == summaryStatusPartial {
			return "partial"
		}
		return "inferred"
	case "diagnostics_by_source", "geometry_coverage_percent", "profile_coverage_percent", "internal_load_method_coverage", "output_readiness_percent", "hvac_rule_edge_count":
		return "computed"
	default:
		if status == summaryStatusPartial {
			return "partial"
		}
		return "direct_or_computed"
	}
}

func summaryMetricVisibility(metricID string) string {
	switch metricID {
	case "average_schedule_operating_hours_h", "unconditioned_floor_area_m2", "bounding_box_area_m2", "building_long_side_m", "building_short_side_m", "footprint_aspect_ratio", "envelope_area_to_volume_ratio", "floor_area_to_volume_ratio", "hvac_node_connection_count", "hvac_rule_edge_count", "diagnostics_by_source":
		return "advanced"
	default:
		return "primary"
	}
}

func summaryMetricBadges(metricID string, status string) []string {
	var badges []string
	if status == summaryStatusPartial {
		badges = append(badges, "partial")
	}
	if status == summaryStatusMissing {
		badges = append(badges, "missing")
	}
	switch metricID {
	case "conditioned_floor_area_m2", "unconditioned_floor_area_m2", "conditioned_zone_count", "conditioned_zone_evidence_breakdown", "bounding_box_area_m2", "building_long_side_m", "building_short_side_m", "footprint_aspect_ratio", "model_operating_hours_h", "average_schedule_operating_hours_h":
		badges = append(badges, "inferred")
	case "north_wwr_percent", "east_wwr_percent", "south_wwr_percent", "west_wwr_percent":
		badges = append(badges, "orientation")
	case "skylight_roof_ratio_percent":
		badges = append(badges, "base-surface")
	case "geometry_coverage_percent", "profile_coverage_percent", "output_readiness_percent":
		badges = append(badges, "readiness")
	case "diagnostics_by_source":
		badges = append(badges, "diagnostic")
	}
	return badges
}

func summaryMetricEvidence(metricID string, defaultMethod string, facts summaryFacts) string {
	switch metricID {
	case "north_wwr_percent", "east_wwr_percent", "south_wwr_percent", "west_wwr_percent":
		return fmt.Sprintf("%s Orientation sources: field_azimuth:%d, computed_normal:%d, base_surface:%d, missing:%d.",
			defaultMethod,
			facts.orientationFieldCount,
			facts.orientationComputedCount,
			facts.windowBaseOrientationCount,
			facts.orientationMissingCount,
		)
	case "skylight_roof_ratio_percent":
		return fmt.Sprintf("%s Fenestration base surfaces unresolved:%d.", defaultMethod, facts.fenestrationMissingBase)
	default:
		return defaultMethod
	}
}

func summarySourceCountsDisplay(counts map[string]int) string {
	if len(counts) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", key, counts[key]))
	}
	return strings.Join(parts, ", ")
}

func summaryConditionedEvidenceDisplay(evidence map[string]map[string]bool) string {
	order := []string{"by_equipment_connections", "by_zone_hvac", "by_thermostat", "by_space_hvac"}
	parts := make([]string, 0, len(order))
	for _, key := range order {
		parts = append(parts, fmt.Sprintf("%s:%d", key, len(evidence[key])))
	}
	return strings.Join(parts, ", ")
}

func summaryInternalLoadCoverageValue(resolved int, total int, unresolved int) summaryMetricValue {
	display := fmt.Sprintf("resolved:%d/%d, unresolved_method_count:%d", resolved, total, unresolved)
	return summaryMetricValue{
		Value:        display,
		DisplayValue: display,
		Status:       partialIf(unresolved > 0),
	}
}

func floorAreaStatus(conditionedArea, grossArea float64, hasGross bool) string {
	if !hasGross || conditionedArea > grossArea {
		return summaryStatusPartial
	}
	return summaryStatusOK
}

func (facts summaryFacts) conditionedFloorArea() (float64, bool) {
	var total float64
	for zone := range facts.conditionedZones {
		area, ok := facts.zoneFloorAreas[zone]
		if !ok {
			continue
		}
		total += area
	}
	return total, total > 0
}

func (facts summaryFacts) loadDensityArea() (float64, bool) {
	if area, ok := facts.conditionedFloorArea(); ok {
		return area, true
	}
	if facts.hasGrossFloorArea {
		return facts.grossFloorArea, true
	}
	return 0, false
}

func (facts summaryFacts) modelOperatingHours() (float64, bool) {
	var maxHours float64
	found := false
	for schedule := range facts.referencedSchedules {
		hours, ok := facts.scheduleHours[schedule]
		if !ok {
			continue
		}
		maxHours = math.Max(maxHours, hours)
		found = true
	}
	if found {
		return maxHours, true
	}
	for _, hours := range facts.scheduleHours {
		maxHours = math.Max(maxHours, hours)
		found = true
	}
	return maxHours, found
}

func (facts summaryFacts) averageScheduleHours() (float64, bool) {
	if len(facts.scheduleHours) == 0 {
		return 0, false
	}
	var total float64
	for _, hours := range facts.scheduleHours {
		total += hours
	}
	return total / float64(len(facts.scheduleHours)), true
}

func (facts summaryFacts) surfaceMap() map[string]surfaceInfo {
	out := map[string]surfaceInfo{}
	for _, surface := range facts.surfaces {
		if surface.name != "" {
			out[normalizeName(surface.name)] = surface
		}
	}
	return out
}

func (facts *summaryFacts) surfaceInfo(obj Object) (surfaceInfo, bool) {
	area, ok := objectArea(obj)
	if !ok || area <= 0 {
		return surfaceInfo{}, false
	}
	vertices, hasVertices := detailedVertices(obj)
	if hasVertices {
		for _, vertex := range vertices {
			facts.bounds.add(vertex)
		}
	}

	zoneName := findFieldByCommentWords(obj, "zone", "name")
	multiplier := zoneMultiplierFor(facts.zoneMultipliers, zoneName)
	area *= multiplier
	minZ, maxZ, avgZ := verticesZStats(vertices)
	azimuth, azimuthSource, hasAzimuth := facts.objectAzimuthSource(obj, zoneName)
	orientation := ""
	if hasAzimuth {
		orientation = orientationFromAzimuth(azimuth)
	}
	outside := findFieldByCommentWords(obj, "outside", "boundary", "condition")
	surfaceType := buildingSurfaceType(obj)
	return surfaceInfo{
		name:          objectName(obj),
		surfaceType:   surfaceType,
		zoneName:      zoneName,
		outside:       outside,
		area:          area,
		azimuth:       azimuth,
		azimuthSource: azimuthSource,
		orientation:   orientation,
		minZ:          minZ,
		maxZ:          maxZ,
		avgZ:          avgZ,
		exterior:      isExteriorSurface(obj.Type, outside),
		ground:        isGroundSurface(outside),
	}, true
}

func (facts summaryFacts) objectAzimuth(obj Object, zoneName string) (float64, bool) {
	azimuth, _, ok := facts.objectAzimuthSource(obj, zoneName)
	return azimuth, ok
}

func (facts summaryFacts) objectAzimuthSource(obj Object, zoneName string) (float64, string, bool) {
	if value, ok := parseFloatField(findFieldByCommentWords(obj, "azimuth")); ok {
		return normalizeDegrees(value + facts.azimuthRotation(zoneName)), "field_azimuth", true
	}
	vertices, ok := detailedVertices(obj)
	if !ok {
		return 0, "", false
	}
	normal, ok := polygonNormal(vertices)
	if !ok {
		return 0, "", false
	}
	if strings.EqualFold(facts.vertexEntryDirection, "clockwise") {
		normal.x *= -1
		normal.y *= -1
		normal.z *= -1
	}
	horizontalLength := math.Hypot(normal.x, normal.y)
	if horizontalLength <= 1e-9 {
		return 0, "", false
	}
	azimuth := math.Atan2(normal.x, normal.y) * 180 / math.Pi
	return normalizeDegrees(azimuth + facts.azimuthRotation(zoneName)), "computed_normal", true
}

func (facts *summaryFacts) recordOrientationSource(source string) {
	switch source {
	case "field_azimuth":
		facts.orientationFieldCount++
	case "computed_normal":
		facts.orientationComputedCount++
	case "":
		facts.orientationMissingCount++
	}
}

func (facts summaryFacts) azimuthRotation(zoneName string) float64 {
	rotation := 0.0
	if facts.hasBuildingNorthAxis {
		rotation += facts.buildingNorthAxis
	}
	if zoneName != "" {
		rotation += facts.zoneDirections[normalizeName(zoneName)]
	}
	return rotation
}

func (facts summaryFacts) geometryHeightForZone(zone string) (float64, bool) {
	minZ := math.Inf(1)
	maxZ := math.Inf(-1)
	for _, surface := range facts.floorSurfaces {
		if normalizeName(surface.zoneName) != zone {
			continue
		}
		minZ = math.Min(minZ, surface.minZ)
		maxZ = math.Max(maxZ, surface.maxZ)
	}
	for _, surface := range facts.surfaces {
		if normalizeName(surface.zoneName) != zone {
			continue
		}
		minZ = math.Min(minZ, surface.minZ)
		maxZ = math.Max(maxZ, surface.maxZ)
	}
	if math.IsInf(minZ, 0) || math.IsInf(maxZ, 0) || maxZ <= minZ {
		return 0, false
	}
	return maxZ - minZ, true
}

func (facts summaryFacts) objectTargetZones(obj Object) []string {
	target := findFieldByCommentWords(obj, "zone", "name")
	if target == "" {
		target = findFieldByCommentWords(obj, "zone", "zonelist", "name")
	}
	if target == "" {
		return nil
	}
	if zones, ok := facts.zoneLists[normalizeName(target)]; ok {
		return zones
	}
	return []string{target}
}

func (facts summaryFacts) objectTargetSpaceZones(obj Object) []string {
	spaceName := findFieldByCommentWords(obj, "space", "name")
	if spaceName == "" {
		spaceName = fieldValueByCatalogName(obj, "Space Name")
	}
	if spaceName == "" && len(obj.Fields) > 0 {
		spaceName = strings.TrimSpace(obj.Fields[0].Value)
	}
	if spaceName == "" {
		return nil
	}
	zoneName := facts.spaceZones[normalizeName(spaceName)]
	if zoneName == "" {
		return nil
	}
	return []string{zoneName}
}

func (facts summaryFacts) targetArea(obj Object) (float64, bool) {
	var total float64
	for _, zone := range facts.objectTargetZones(obj) {
		total += facts.zoneFloorAreas[normalizeName(zone)]
	}
	return total, total > 0
}

func (facts summaryFacts) targetPeople(obj Object) (float64, bool) {
	var total float64
	for _, zone := range facts.objectTargetZones(obj) {
		total += facts.peopleForZone(zone)
	}
	return total, total > 0
}

func (facts summaryFacts) targetMultiplier(obj Object) float64 {
	total := 0.0
	for _, zone := range facts.objectTargetZones(obj) {
		total += zoneMultiplierFor(facts.zoneMultipliers, zone)
	}
	if total == 0 {
		return 1
	}
	return total
}

func (facts summaryFacts) peopleForZone(zoneName string) float64 {
	zoneKey := normalizeName(zoneName)
	area := facts.zoneFloorAreas[zoneKey]
	if area <= 0 || facts.totalPeople <= 0 || !facts.hasGrossFloorArea || facts.grossFloorArea <= 0 {
		return 0
	}
	return facts.totalPeople * area / facts.grossFloorArea
}

func (facts summaryFacts) peopleObjectValue(obj Object) (float64, bool, bool) {
	method := strings.ToLower(findFieldByCommentWords(obj, "calculation", "method"))
	multiplier := facts.targetMultiplier(obj)
	switch {
	case strings.Contains(method, "people/area"):
		value, ok := findNumericFieldByCommentWords(obj, "people", "zone", "floor", "area")
		area, hasArea := facts.targetArea(obj)
		return value * area, ok && hasArea, !hasArea
	case strings.Contains(method, "area/person"):
		value, ok := findNumericFieldByCommentWords(obj, "zone", "floor", "area", "person")
		area, hasArea := facts.targetArea(obj)
		if !ok || !hasArea || value <= 0 {
			return 0, false, true
		}
		return area / value, true, false
	default:
		value, ok := findNumericFieldByCommentWords(obj, "number", "people")
		return value * multiplier, ok, false
	}
}

func (facts summaryFacts) designPowerValue(obj Object, kind string) (float64, bool, bool) {
	method := strings.ToLower(findFieldByCommentWords(obj, "calculation", "method"))
	multiplier := facts.targetMultiplier(obj)
	switch {
	case strings.Contains(method, "watts/area"):
		value, ok := findNumericFieldByCommentWords(obj, "watts", "zone", "floor", "area")
		area, hasArea := facts.targetArea(obj)
		return value * area, ok && hasArea, !hasArea
	case strings.Contains(method, "watts/person"):
		value, ok := findNumericFieldByCommentWords(obj, "watts", "person")
		people, hasPeople := facts.targetPeople(obj)
		return value * people, ok && hasPeople, !hasPeople
	default:
		fieldWords := []string{"design", "level"}
		if kind == "lighting" {
			fieldWords = []string{"lighting", "level"}
		}
		value, ok := findNumericFieldByCommentWords(obj, fieldWords...)
		return value * multiplier, ok, false
	}
}

func zoneMultiplierFor(multipliers map[string]float64, zoneName string) float64 {
	if zoneName == "" {
		return 1
	}
	multiplier, ok := multipliers[normalizeName(zoneName)]
	if !ok || multiplier <= 0 {
		return 1
	}
	return multiplier
}

func zoneListMembers(obj Object) []string {
	var zones []string
	for _, field := range obj.Fields[1:] {
		value := strings.TrimSpace(field.Value)
		if value != "" {
			zones = append(zones, value)
		}
	}
	return zones
}

func referencedScheduleNames(obj Object) []string {
	if isScheduleType(obj.Type) {
		return nil
	}
	var names []string
	for _, field := range obj.Fields {
		comment := strings.ToLower(field.Comment)
		if strings.Contains(comment, "schedule") && strings.Contains(comment, "name") {
			if value := strings.TrimSpace(field.Value); value != "" {
				names = append(names, value)
			}
		}
	}
	return names
}

func parseFloatField(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	lower := strings.ToLower(value)
	if lower == "autocalculate" || lower == "autosize" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

func findNumericFieldByCommentWords(obj Object, words ...string) (float64, bool) {
	for _, field := range obj.Fields {
		comment := strings.ToLower(field.Comment)
		matched := true
		for _, word := range words {
			if !strings.Contains(comment, strings.ToLower(word)) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		if value, ok := parseFloatField(field.Value); ok {
			return value, true
		}
	}
	return 0, false
}

func numericFieldOrDefault(obj Object, fallback float64, words ...string) float64 {
	if value, ok := parseFloatField(findFieldByCommentWords(obj, words...)); ok {
		return value
	}
	return fallback
}

func objectArea(obj Object) (float64, bool) {
	if vertices, ok := detailedVertices(obj); ok {
		return polygonArea(vertices)
	}
	if length, ok := parseFloatField(findFieldByCommentWords(obj, "length")); ok {
		if height, ok := parseFloatField(findFieldByCommentWords(obj, "height")); ok {
			return length * height, true
		}
		if width, ok := parseFloatField(findFieldByCommentWords(obj, "width")); ok {
			return length * width, true
		}
	}
	if width, ok := parseFloatField(findFieldByCommentWords(obj, "width")); ok {
		if height, ok := parseFloatField(findFieldByCommentWords(obj, "height")); ok {
			return width * height, true
		}
	}
	return 0, false
}

type point3 struct {
	x float64
	y float64
	z float64
}

var vertexCommentPattern = regexp.MustCompile(`(?i)vertex\s+(\d+)\s+([xyz])-coordinate`)

func detailedVertices(obj Object) ([]point3, bool) {
	values := map[int]map[string]float64{}
	for _, field := range obj.Fields {
		matches := vertexCommentPattern.FindStringSubmatch(field.Comment)
		if matches == nil {
			continue
		}
		index, err := strconv.Atoi(matches[1])
		if err != nil || index <= 0 {
			continue
		}
		value, ok := parseFloatField(field.Value)
		if !ok {
			continue
		}
		if values[index] == nil {
			values[index] = map[string]float64{}
		}
		values[index][strings.ToLower(matches[2])] = value
	}
	if vertices, ok := orderedPointValues(values); ok {
		return vertices, true
	}

	vertexCountIndex := -1
	vertexCount := 0
	for index, field := range obj.Fields {
		if strings.Contains(strings.ToLower(field.Comment), "number") && strings.Contains(strings.ToLower(field.Comment), "vertices") {
			if value, ok := parseFloatField(field.Value); ok {
				vertexCountIndex = index
				vertexCount = int(math.Round(value))
				break
			}
		}
	}
	if vertexCountIndex < 0 || vertexCount <= 0 || len(obj.Fields) < vertexCountIndex+1+vertexCount*3 {
		return nil, false
	}
	vertices := make([]point3, 0, vertexCount)
	for index := 0; index < vertexCount; index++ {
		offset := vertexCountIndex + 1 + index*3
		x, okX := parseFloatField(obj.Fields[offset].Value)
		y, okY := parseFloatField(obj.Fields[offset+1].Value)
		z, okZ := parseFloatField(obj.Fields[offset+2].Value)
		if !okX || !okY || !okZ {
			return nil, false
		}
		vertices = append(vertices, point3{x: x, y: y, z: z})
	}
	return vertices, true
}

func orderedPointValues(values map[int]map[string]float64) ([]point3, bool) {
	if len(values) == 0 {
		return nil, false
	}
	indexes := make([]int, 0, len(values))
	for index := range values {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	vertices := make([]point3, 0, len(indexes))
	for _, index := range indexes {
		fields := values[index]
		x, okX := fields["x"]
		y, okY := fields["y"]
		z, okZ := fields["z"]
		if !okX || !okY || !okZ {
			return nil, false
		}
		vertices = append(vertices, point3{x: x, y: y, z: z})
	}
	return vertices, true
}

func polygonArea(vertices []point3) (float64, bool) {
	normal, ok := polygonNormal(vertices)
	if !ok {
		return 0, false
	}
	area := 0.5 * math.Sqrt(normal.x*normal.x+normal.y*normal.y+normal.z*normal.z)
	return area, area > 0
}

func polygonNormal(vertices []point3) (point3, bool) {
	if len(vertices) < 3 {
		return point3{}, false
	}
	var normal point3
	for i, current := range vertices {
		next := vertices[(i+1)%len(vertices)]
		normal.x += (current.y - next.y) * (current.z + next.z)
		normal.y += (current.z - next.z) * (current.x + next.x)
		normal.z += (current.x - next.x) * (current.y + next.y)
	}
	if math.Abs(normal.x)+math.Abs(normal.y)+math.Abs(normal.z) <= 1e-9 {
		return point3{}, false
	}
	return normal, true
}

func verticesZStats(vertices []point3) (float64, float64, float64) {
	if len(vertices) == 0 {
		return 0, 0, 0
	}
	minZ := vertices[0].z
	maxZ := vertices[0].z
	var total float64
	for _, vertex := range vertices {
		minZ = math.Min(minZ, vertex.z)
		maxZ = math.Max(maxZ, vertex.z)
		total += vertex.z
	}
	return minZ, maxZ, total / float64(len(vertices))
}

func lowestFloorSurfaces(surfaces []surfaceInfo) []surfaceInfo {
	if len(surfaces) == 0 {
		return nil
	}
	minZ := math.Inf(1)
	for _, surface := range surfaces {
		minZ = math.Min(minZ, surface.avgZ)
	}
	var out []surfaceInfo
	for _, surface := range surfaces {
		if math.Abs(surface.avgZ-minZ) <= 0.1 {
			out = append(out, surface)
		}
	}
	return out
}

func normalizeDegrees(value float64) float64 {
	value = math.Mod(value, 360)
	if value < 0 {
		value += 360
	}
	return value
}

func orientationFromAzimuth(azimuth float64) string {
	azimuth = normalizeDegrees(azimuth)
	switch {
	case azimuth >= 315 || azimuth < 45:
		return "north"
	case azimuth >= 45 && azimuth < 135:
		return "east"
	case azimuth >= 135 && azimuth < 225:
		return "south"
	default:
		return "west"
	}
}

func buildingSurfaceType(obj Object) string {
	if value := findFieldByCommentWords(obj, "surface", "type"); value != "" {
		return value
	}
	lowerType := strings.ToLower(obj.Type)
	switch {
	case strings.Contains(lowerType, "wall"):
		return "Wall"
	case strings.Contains(lowerType, "roof"):
		return "Roof"
	case strings.Contains(lowerType, "ceiling"):
		return "Ceiling"
	case strings.Contains(lowerType, "floor"):
		return "Floor"
	default:
		return obj.Type
	}
}

func fenestrationSurfaceType(obj Object) string {
	if value := findFieldByCommentWords(obj, "surface", "type"); value != "" {
		return value
	}
	return obj.Type
}

func isBuildingSurfaceType(objectType string) bool {
	lower := strings.ToLower(objectType)
	if lower == "buildingsurface:detailed" {
		return true
	}
	return lower == "wall:exterior" ||
		lower == "wall:adiabatic" ||
		lower == "wall:underground" ||
		lower == "wall:interzone" ||
		lower == "roof" ||
		lower == "roofceiling:detailed" ||
		lower == "ceiling:adiabatic" ||
		lower == "ceiling:interzone" ||
		lower == "floor:groundcontact" ||
		lower == "floor:adiabatic" ||
		lower == "floor:interzone" ||
		lower == "floor:detailed"
}

func isFenestrationType(objectType string) bool {
	lower := strings.ToLower(objectType)
	if strings.HasPrefix(lower, "windowmaterial") {
		return false
	}
	return lower == "fenestrationsurface:detailed" ||
		lower == "window" ||
		lower == "window:interzone" ||
		lower == "door" ||
		lower == "door:interzone" ||
		lower == "glazeddoor" ||
		lower == "glazeddoor:interzone"
}

func isExteriorSurface(objectType string, outside string) bool {
	lowerOutside := strings.ToLower(strings.TrimSpace(outside))
	lowerType := strings.ToLower(objectType)
	return lowerOutside == "outdoors" ||
		strings.Contains(lowerOutside, "outside") ||
		strings.Contains(lowerOutside, "weather") ||
		strings.Contains(lowerType, "exterior") ||
		lowerType == "roof"
}

func isGroundSurface(outside string) bool {
	lower := strings.ToLower(strings.TrimSpace(outside))
	return lower == "ground" || strings.Contains(lower, "ground")
}

func isHVACSummaryType(lowerType string) bool {
	return strings.Contains(lowerType, "hvac") ||
		strings.HasPrefix(lowerType, "airloop") ||
		strings.HasPrefix(lowerType, "plantloop") ||
		strings.HasPrefix(lowerType, "coil:") ||
		strings.HasPrefix(lowerType, "fan:") ||
		strings.HasPrefix(lowerType, "pump:") ||
		strings.HasPrefix(lowerType, "boiler:") ||
		strings.HasPrefix(lowerType, "chiller:") ||
		strings.HasPrefix(lowerType, "setpointmanager:")
}

type scheduleInterval struct {
	hours float64
	value float64
}

type compactScheduleRule struct {
	startDay  int
	endDay    int
	selector  string
	intervals []scheduleInterval
}

func annualScheduleHours(obj Object) (float64, bool) {
	if strings.EqualFold(obj.Type, "Schedule:Constant") {
		value := ""
		if len(obj.Fields) > 2 {
			value = obj.Fields[2].Value
		}
		if field := findFieldByCommentWords(obj, "hourly", "value"); field != "" {
			value = field
		}
		number, ok := parseFloatField(value)
		if !ok {
			return 0, false
		}
		if number > 0 {
			return 8760, true
		}
		return 0, true
	}
	if !strings.EqualFold(obj.Type, "Schedule:Compact") {
		return 0, false
	}
	return compactScheduleHours(obj)
}

func compactScheduleHours(obj Object) (float64, bool) {
	if len(obj.Fields) <= 2 {
		return 0, false
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
				return 0, false
			}
			periodStart = previousThrough + 1
			periodEnd = day
			previousThrough = day
			index++
		case strings.HasPrefix(lower, "for:"):
			selector := strings.TrimSpace(value[len("for:"):])
			intervals, next, ok := parseCompactIntervals(obj.Fields, index+1)
			if !ok || !recognizedDaySelector(selector) {
				return 0, false
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
	if len(rules) == 0 {
		return 0, false
	}

	var total float64
	for day := 1; day <= 365; day++ {
		for _, rule := range rules {
			if day < rule.startDay || day > rule.endDay || !dayMatchesSelector(day, rule.selector) {
				continue
			}
			for _, interval := range rule.intervals {
				if interval.value > 0 {
					total += interval.hours
				}
			}
			break
		}
	}
	return total, true
}

func parseCompactIntervals(fields []Field, start int) ([]scheduleInterval, int, bool) {
	var intervals []scheduleInterval
	previousHour := 0.0
	index := start
	for index < len(fields) {
		value := strings.TrimSpace(fields[index].Value)
		lower := strings.ToLower(value)
		if strings.HasPrefix(lower, "through:") || strings.HasPrefix(lower, "for:") {
			break
		}
		if !strings.HasPrefix(lower, "until:") {
			index++
			continue
		}
		hour, ok := parseScheduleHour(strings.TrimSpace(value[len("until:"):]))
		if !ok || index+1 >= len(fields) {
			return nil, index, false
		}
		nextValue, ok := parseFloatField(fields[index+1].Value)
		if !ok || hour < previousHour {
			return nil, index, false
		}
		intervals = append(intervals, scheduleInterval{hours: hour - previousHour, value: nextValue})
		previousHour = hour
		index += 2
	}
	return intervals, index, len(intervals) > 0
}

func parseMonthDay(value string) (int, bool) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0, false
	}
	month, errMonth := strconv.Atoi(strings.TrimSpace(parts[0]))
	day, errDay := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errMonth != nil || errDay != nil || month < 1 || month > 12 {
		return 0, false
	}
	daysByMonth := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if day < 1 || day > daysByMonth[month-1] {
		return 0, false
	}
	out := day
	for i := 0; i < month-1; i++ {
		out += daysByMonth[i]
	}
	return out, true
}

func parseScheduleHour(value string) (float64, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, false
	}
	hour, errHour := strconv.Atoi(strings.TrimSpace(parts[0]))
	minute, errMinute := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errHour != nil || errMinute != nil || hour < 0 || hour > 24 || minute < 0 || minute >= 60 {
		return 0, false
	}
	if hour == 24 && minute != 0 {
		return 0, false
	}
	return float64(hour) + float64(minute)/60, true
}

func recognizedDaySelector(selector string) bool {
	tokens := scheduleSelectorTokens(selector)
	if len(tokens) == 0 {
		return false
	}
	for _, token := range tokens {
		switch token {
		case "alldays", "everyday", "weekdays", "weekends", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday", "allotherdays", "holiday", "holidays", "summerdesignday", "winterdesignday", "customday1", "customday2":
		default:
			return false
		}
	}
	return true
}

func dayMatchesSelector(day int, selector string) bool {
	dayOfWeek := (day - 1) % 7 // Fixed non-leap Monday-start calendar.
	for _, token := range scheduleSelectorTokens(selector) {
		switch token {
		case "alldays", "everyday", "allotherdays":
			return true
		case "weekdays":
			if dayOfWeek >= 0 && dayOfWeek <= 4 {
				return true
			}
		case "weekends":
			if dayOfWeek == 5 || dayOfWeek == 6 {
				return true
			}
		case "monday":
			if dayOfWeek == 0 {
				return true
			}
		case "tuesday":
			if dayOfWeek == 1 {
				return true
			}
		case "wednesday":
			if dayOfWeek == 2 {
				return true
			}
		case "thursday":
			if dayOfWeek == 3 {
				return true
			}
		case "friday":
			if dayOfWeek == 4 {
				return true
			}
		case "saturday":
			if dayOfWeek == 5 {
				return true
			}
		case "sunday":
			if dayOfWeek == 6 {
				return true
			}
		}
	}
	return false
}

func normalizeScheduleSelector(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func scheduleSelectorTokens(value string) []string {
	value = strings.ReplaceAll(value, ",", " ")
	parts := strings.Fields(value)
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		token := normalizeScheduleSelector(part)
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	if len(tokens) == 0 {
		if token := normalizeScheduleSelector(value); token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func init() {
	if len(summaryDefinitions) != 59 {
		panic(fmt.Sprintf("summary metric registry has %d metrics, want 59", len(summaryDefinitions)))
	}
}
