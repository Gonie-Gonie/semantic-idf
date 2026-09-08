package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type epathSQLQuantity struct {
	Value, Error float64
	Bounds       *[2]float64
}

func (q epathSQLQuantity) bounds() (float64, float64) {
	if q.Bounds != nil {
		return q.Bounds[0], q.Bounds[1]
	}
	return q.Value - q.Error, q.Value + q.Error
}
func epathSQLBounded(value, low, high float64) epathSQLQuantity {
	return epathSQLQuantity{Value: value, Error: math.Max(value-low, high-value), Bounds: &[2]float64{low, high}}
}
func (q epathSQLQuantity) valid() bool {
	low, high := q.bounds()
	// Nil bounds are already defined by Value +/- Error. Subtracting Value
	// again can magnify floating-point cancellation at large SQL quantities;
	// only an independently supplied interval needs the consistency check.
	return epathOracleFinite(q.Value) && epathOracleFinite(q.Error) && q.Error >= 0 && epathOracleFinite(low) && epathOracleFinite(high) && low <= q.Value && q.Value <= high && (q.Bounds == nil || q.Error+1e-12 >= math.Max(q.Value-low, high-q.Value))
}
func (q epathSQLQuantity) includesZero() bool {
	low, high := q.bounds()
	return q.valid() && low <= 0 && high >= 0
}

func (q epathSQLQuantity) add(other epathSQLQuantity) epathSQLQuantity {
	low, high := q.bounds()
	otherLow, otherHigh := other.bounds()
	return epathSQLBounded(q.Value+other.Value, low+otherLow, high+otherHigh)
}
func (q epathSQLQuantity) times(factor float64) epathSQLQuantity {
	low, high := q.bounds()
	if factor < 0 {
		low, high = high, low
	}
	return epathSQLBounded(q.Value*factor, low*factor, high*factor)
}
func (q epathSQLQuantity) positive() epathSQLQuantity {
	low, high := q.bounds()
	return epathSQLBounded(math.Max(0, q.Value), math.Max(0, low), math.Max(0, high))
}

// The SQL center stays intact. Only the presentation contribution may be
// omitted; its upper bound is never doubled by a symmetric zero-crossing pad.
func (q epathSQLQuantity) optionalPresentation() epathSQLQuantity {
	positive := q.positive()
	_, high := positive.bounds()
	return epathSQLBounded(positive.Value, 0, high)
}

type epathSQLZone struct {
	Name       string
	Multiplier float64
}
type epathSQLSurface struct{ Zone, Category string }
type epathSQLCell struct {
	Zone, Family, Category, Component string
	Month                             int
	Raw, Effective                    epathSQLQuantity
	Allocated                         map[string]epathSQLQuantity
	BuildingVisible                   bool
	SourceIDs                         []int // Independently selected original SQL dictionary leaves.
	ZeroPressureFallback              bool  // Explicit reviewed load-only synthetic allocation, never a physical pressure.
}
type epathSQLFrames struct {
	RadiantSurfaceContextIdentities map[int]epathSQLRadiantSurfaceContextIdentity
	NativeVRFSystems                []epathSQLVRFSystemFrame
	NativeVRFAllocations            []epathSQLVRFAllocationProof
	RadiantLoadSourceIdentities     map[int]epathSQLRadiantLoadSourceIdentity
	ZeroPressureFallbacks           map[string]epathRealSQLZeroPressureFallback
	ZeroPressurePhysicalCells       map[string]epathSQLCell
	DirectHVAC                      map[string]epathSQLDirectHVACMonth
	DirectHVACSourceIdentities      map[int]epathSQLDirectHVACSourceIdentity
	TraceSourceIdentities           map[int]epathSQLTraceSourceIdentity
	CellTraceSourceIDs              map[string][]int
	Zones                           map[string]epathSQLZone
	Cells                           map[string]*epathSQLCell
	Loads                           map[string]epathSQLQuantity
	Site                            map[string][]*epathSQLQuantity
	SiteAnnual                      map[string]epathSQLTabularObservation
	SiteSources                     map[string][]int
	SourceRaw                       map[int][]epathSQLQuantity
	SourceEffective                 map[int][]epathSQLQuantity
	SourceZone                      map[int]string
	SourceIdentities                map[int]epathRealSQLSource
	LoadSourceIDs                   map[string][]int
	LoadDetailSourceIDs             map[string][]int
	LoadDetailIdentities            map[int]epathSQLLoadDetailIdentity
}

func epathSQLKey(zone, family string, month int) string {
	return fmt.Sprintf("%s|%s|%02d", strings.ToLower(zone), family, month)
}

// The source class is explicit: an annual cell never manufactures twelve
// monthly zeroes, and one site cannot silently mix two reporting bases.
func epathSQLSiteIsAnnual(frames epathSQLFrames, id string) (bool, error) {
	_, monthly := frames.Site[id]
	annual, yearly := frames.SiteAnnual[id]
	if id == "" || monthly == yearly {
		return false, fmt.Errorf("site %q requires exactly one independently observed reporting basis", id)
	}
	if yearly {
		if err := epathValidateSQLTabularObservation(annual); err != nil {
			return false, fmt.Errorf("site %s has an invalid annual Tabular observation: %w", id, err)
		}
	}
	return yearly, nil
}

func epathSQLSitePeriod(frames epathSQLFrames, id, period string) (*epathSQLQuantity, error) {
	month := 0
	if period != "annual" {
		for candidate := 1; candidate <= 12; candidate++ {
			if period == fmt.Sprintf("M%d", candidate) {
				month = candidate
				break
			}
		}
		if month == 0 {
			return nil, fmt.Errorf("invalid exact site period %q", period)
		}
	}
	annual, err := epathSQLSiteIsAnnual(frames, id)
	if err != nil {
		return nil, err
	}
	if annual {
		if period != "annual" {
			return nil, nil // Not reported, not a known zero or a divided annual total.
		}
		q := frames.SiteAnnual[id].Quantity
		if q.Bounds != nil {
			bounds := *q.Bounds
			q.Bounds = &bounds
		}
		return &q, nil
	}
	values := frames.Site[id]
	if len(values) != 12 {
		return nil, fmt.Errorf("site %s lacks twelve original monthly observations", id)
	}
	q := epathSQLQuantity{}
	for index, value := range values {
		if month != 0 && index+1 != month {
			continue
		}
		if value == nil || !value.valid() || value.Value < 0 {
			return nil, fmt.Errorf("site %s/M%d has unknown/invalid monthly energy", id, index+1)
		}
		q = q.add(*value)
	}
	return &q, nil
}

func epathSQLSurfaceCategory(class string, boundary, index int) (string, error) {
	if class == "Internal Mass" || boundary == index {
		return "balance.storage_other", nil
	}
	if boundary > 0 {
		return "surface.interzone", nil
	}
	if boundary == -1 && class == "Wall" {
		return "balance.storage_other", nil
	}
	if boundary == -1 && class == "Floor" {
		return "surface.ground_floors", nil
	}
	if boundary == 0 {
		switch class {
		case "Wall":
			return "surface.exterior_walls", nil
		case "Roof":
			return "surface.roofs", nil
		case "Floor":
			// Ground / floor taxonomy includes exterior soffits; boundary0
			// remains Outdoors and is not reinterpreted as ground contact.
			return "surface.ground_floors", nil
		case "Window", "Door", "GlassDoor":
			return "surface.windows_doors", nil
		}
	}
	return "", fmt.Errorf("unreviewed surface class/boundary %q/%d", class, boundary)
}

func epathSQLSelect(sources []epathRealSQLSource, selector epathRealSQLSelector) ([]epathRealSQLSource, error) {
	if len(selector.Alternatives) == 0 || len(selector.Keys) == 0 {
		return nil, fmt.Errorf("SQL selector requires reviewed alternatives and keys")
	}
	keys := map[string]bool{}
	wildcard := false
	for _, key := range selector.Keys {
		if key == "*" {
			wildcard = true
		} else if keys[strings.ToLower(key)] {
			return nil, fmt.Errorf("duplicate selector key %q", key)
		} else {
			keys[strings.ToLower(key)] = true
		}
	}
	if wildcard && len(selector.Keys) != 1 {
		return nil, fmt.Errorf("wildcard cannot conceal exact expected keys")
	}
	selected := map[string]epathRealSQLSource{}
	for _, alternative := range selector.Alternatives {
		if alternative.Name == "" || alternative.Unit == "" {
			return nil, fmt.Errorf("empty SQL alternative identity")
		}
		seen := map[string]bool{}
		for _, source := range sources {
			key := strings.ToLower(source.KeyValue)
			if !strings.EqualFold(source.Name, alternative.Name) || source.SourceUnit != alternative.Unit || source.IsMeter != selector.IsMeter || !strings.EqualFold(source.ReportingFrequency, "Monthly") || !wildcard && !keys[key] {
				continue
			}
			if seen[key] {
				return nil, fmt.Errorf("duplicate observed SQL alternative %s/%s", alternative.Name, key)
			}
			seen[key] = true
			if _, exists := selected[key]; !exists {
				selected[key] = source
			}
		}
	}
	if len(selected) == 0 && selector.AllowAbsent {
		return nil, nil
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no actual SQL source matches %s", selector.Alternatives[0].Name)
	}
	if !wildcard {
		for key := range keys {
			if _, ok := selected[key]; !ok {
				return nil, fmt.Errorf("expected source key is missing, not zero: %s/%s", selector.Alternatives[0].Name, key)
			}
		}
	}
	out := []epathRealSQLSource{}
	for _, source := range selected {
		out = append(out, source)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DictionaryIndex < out[j].DictionaryIndex })
	return out, nil
}

func epathSQLMonthly(source epathRealSQLSource, precision epathRealSQLPrecision) ([]epathSQLQuantity, error) {
	if precision.DecimalPlaces != 3 || precision.SourceStages < 1 || precision.SourceStages > 3 || precision.ContributionStages < 1 || precision.ContributionStages > 3 {
		return nil, fmt.Errorf("invalid source precision configuration")
	}
	if len(source.Months) != 12 {
		return nil, fmt.Errorf("source %s/%s does not have twelve actual monthly observations", source.Name, source.KeyValue)
	}
	values := make([]epathSQLQuantity, 12)
	for index, bucket := range source.Months {
		if bucket.Month != index+1 || bucket.Rows != 1 || bucket.EnergyKWh == nil || !epathOracleFinite(*bucket.EnergyKWh) {
			return nil, fmt.Errorf("source %s/%s has unknown/duplicate monthly energy %d", source.Name, source.KeyValue, bucket.Month)
		}
		budget := float64(precision.SourceStages) * .0005
		if *bucket.EnergyKWh == 0 {
			budget = 0
		} // Actual reported zero remains exactly zero through rounding.
		values[index] = epathSQLQuantity{Value: *bucket.EnergyKWh, Error: budget}
	}
	return values, nil
}

func epathCompileSQLModelFrames(sqlPath string, observed []epathRealSQLSource, model epathRealSQLModel, originalText ...string) (epathSQLFrames, error) {
	out := epathSQLFrames{Zones: map[string]epathSQLZone{}, Cells: map[string]*epathSQLCell{}, Loads: map[string]epathSQLQuantity{}, Site: map[string][]*epathSQLQuantity{}, SiteAnnual: map[string]epathSQLTabularObservation{}, SiteSources: map[string][]int{}, SourceRaw: map[int][]epathSQLQuantity{}, SourceEffective: map[int][]epathSQLQuantity{}, SourceZone: map[int]string{}, SourceIdentities: map[int]epathRealSQLSource{}, LoadSourceIDs: map[string][]int{}}
	if len(originalText) > 1 {
		return out, fmt.Errorf("SQL model frames require at most one exact original input")
	}
	if model.Schema != "semantic-idf.energy-path-sql-model/large-office-monthly/v1" || model.Surface.Mapping != "surface_class_boundary/v1" || model.Surface.Sign != -1 || model.Precision.DecimalPlaces != 3 || model.Precision.SourceStages < 1 || model.Precision.SourceStages > 3 || model.Precision.ContributionStages < 1 || model.Precision.ContributionStages > 3 {
		return out, fmt.Errorf("unsupported reviewed sqlModel/precision policy")
	}
	db, err := epathOpenOracleSQL(sqlPath)
	if err != nil {
		return out, err
	}
	defer db.Close()
	zones, err := db.Query(`SELECT ZoneIndex,ZoneName,Multiplier,ListMultiplier FROM Zones ORDER BY ZoneIndex`)
	if err != nil {
		return out, err
	}
	zoneIndexes := map[int]string{}
	for zones.Next() {
		var index int
		var name string
		var multiplier, list float64
		if err := zones.Scan(&index, &name, &multiplier, &list); err != nil {
			zones.Close()
			return out, err
		}
		key := strings.ToLower(name)
		if name == "" || multiplier <= 0 || list <= 0 || !epathOracleFinite(multiplier*list) || out.Zones[key].Name != "" {
			zones.Close()
			return out, fmt.Errorf("invalid/duplicate SQL Zone multiplier identity")
		}
		out.Zones[key] = epathSQLZone{name, multiplier * list}
		zoneIndexes[index] = key
	}
	if err := zones.Err(); err != nil {
		zones.Close()
		return out, err
	}
	zones.Close()
	if len(out.Zones) == 0 {
		return out, fmt.Errorf("SQL has no model Zones")
	}
	rows, err := db.Query(`SELECT SurfaceIndex,SurfaceName,ClassName,ZoneIndex,ExtBoundCond FROM Surfaces WHERE HeatTransferSurf=1 ORDER BY SurfaceIndex`)
	if err != nil {
		return out, err
	}
	surfaces := map[string]epathSQLSurface{}
	for rows.Next() {
		var index, zone, boundary int
		var name, class string
		if err := rows.Scan(&index, &name, &class, &zone, &boundary); err != nil {
			rows.Close()
			return out, err
		}
		key := strings.ToLower(name)
		if _, ok := surfaces[key]; ok || zoneIndexes[zone] == "" {
			rows.Close()
			return out, fmt.Errorf("duplicate/unowned SQL surface %q", name)
		}
		category, err := epathSQLSurfaceCategory(class, boundary, index)
		if err != nil {
			rows.Close()
			return out, err
		}
		surfaces[key] = epathSQLSurface{zoneIndexes[zone], category}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	radiantContext, err := epathSQLRadiantSurfaceContexts(db, observed, model, out.Zones, surfaces, originalText)
	if err != nil {
		return out, err
	}
	out.RadiantSurfaceContextIdentities = radiantContext.Identities
	add := func(zone, family, category, component string, month int, raw epathSQLQuantity, visible bool, sourceIDs []int) {
		key := epathSQLKey(zone, family, month)
		cell := out.Cells[key]
		if cell == nil {
			cell = &epathSQLCell{Zone: zone, Family: family, Category: category, Component: component, Month: month, Allocated: map[string]epathSQLQuantity{}, BuildingVisible: visible}
			out.Cells[key] = cell
		}
		cell.Raw = cell.Raw.add(raw)
		cell.Effective = cell.Raw.times(out.Zones[zone].Multiplier)
		cell.SourceIDs = epathSQLDictionaryUnion(cell.SourceIDs, sourceIDs)
	}
	surfaceSources, err := epathSQLSelect(observed, model.Surface.Source)
	if err != nil {
		return out, err
	}
	for _, source := range surfaceSources {
		out.SourceIdentities[source.DictionaryIndex] = source
		surface, ok := surfaces[strings.ToLower(source.KeyValue)]
		if !ok {
			return out, fmt.Errorf("reported surface %q lacks exact SQL ownership", source.KeyValue)
		}
		values, err := epathSQLMonthly(source, model.Precision)
		if err != nil {
			return out, err
		}
		out.SourceRaw[source.DictionaryIndex] = values
		out.SourceZone[source.DictionaryIndex] = surface.Zone
		effective := make([]epathSQLQuantity, 12)
		for month, value := range values {
			signed := value.times(model.Surface.Sign)
			if _, contextOnly := radiantContext.Identities[source.DictionaryIndex]; !contextOnly {
				add(surface.Zone, "surface:"+surface.Category, surface.Category, "sensible", month+1, signed, true, []int{source.DictionaryIndex})
			}
			effective[month] = value.times(out.Zones[surface.Zone].Multiplier)
		}
		out.SourceEffective[source.DictionaryIndex] = effective
	}
	defined := map[string]epathRealSQLFamily{}
	for _, family := range model.Families {
		if family.ID == "" || strings.HasPrefix(family.ID, "surface:") || family.ID == "surface.total" || len(family.Keys) == 0 || (family.Role != "pressure" && family.Role != "context") || family.Category == "" || family.Component == "" || defined[family.ID].ID != "" {
			return out, fmt.Errorf("invalid/duplicate SQL family declaration %q", family.ID)
		}
		keys := map[string]bool{}
		for _, name := range family.Keys {
			key := strings.ToLower(name)
			if keys[key] || out.Zones[key].Name == "" {
				return out, fmt.Errorf("unknown/duplicate family Zone %q", name)
			}
			keys[key] = true
			for month := 1; month <= 12; month++ {
				if !radiantContext.FamilyZones[family.ID+"|"+key] {
					add(key, family.ID, family.Category, family.Component, month, epathSQLQuantity{}, family.BuildingVisible, nil)
				}
			}
		}
		for _, term := range family.Terms {
			if term.Sign != 1 && term.Sign != -1 {
				return out, fmt.Errorf("family term requires explicit +/-1 sign")
			}
			selector := term.Source
			if len(selector.Keys) == 0 {
				selector.Keys = family.Keys
			}
			selector.AllowAbsent = false
			sources, err := epathSQLSelect(observed, selector)
			if err != nil {
				return out, err
			}
			for _, source := range sources {
				out.SourceIdentities[source.DictionaryIndex] = source
				zone := strings.ToLower(source.KeyValue)
				if !keys[zone] {
					return out, fmt.Errorf("family source has undeclared applicability Zone %s", zone)
				}
				values, err := epathSQLMonthly(source, model.Precision)
				if err != nil {
					return out, err
				}
				out.SourceRaw[source.DictionaryIndex] = values
				out.SourceZone[source.DictionaryIndex] = zone
				effective := make([]epathSQLQuantity, 12)
				for month, value := range values {
					if _, contextOnly := radiantContext.Identities[source.DictionaryIndex]; !contextOnly {
						add(zone, family.ID, family.Category, family.Component, month+1, value.times(term.Sign), family.BuildingVisible, []int{source.DictionaryIndex})
					}
					effective[month] = value.times(out.Zones[zone].Multiplier)
				}
				out.SourceEffective[source.DictionaryIndex] = effective
			}
		}
		for _, dependency := range family.Subtract {
			if dependency != "surface.total" && defined[dependency].ID == "" {
				return out, fmt.Errorf("unknown/forward family dependency %s", dependency)
			}
			for zone := range keys {
				for month := 1; month <= 12; month++ {
					subtract := epathSQLQuantity{}
					var sourceIDs []int
					if dependency == "surface.total" {
						for _, cell := range out.Cells {
							if cell.Zone == zone && cell.Month == month && strings.HasPrefix(cell.Family, "surface:") {
								subtract = subtract.add(cell.Raw)
								sourceIDs = epathSQLDictionaryUnion(sourceIDs, cell.SourceIDs)
							}
						}
					} else if cell := out.Cells[epathSQLKey(zone, dependency, month)]; cell != nil {
						subtract = cell.Raw
						sourceIDs = cell.SourceIDs
					}
					add(zone, family.ID, family.Category, family.Component, month, subtract.times(-1), family.BuildingVisible, sourceIDs)
				}
			}
		}
		defined[family.ID] = family
	}
	loadServices := map[string]bool{}
	for _, load := range model.Loads {
		if (load.Service != "cooling" && load.Service != "heating") || loadServices[load.Service] || load.NativeRadiant == nil && load.Component != "sensible" || load.NativeRadiant != nil && load.Component != "combined" {
			return out, fmt.Errorf("unknown/duplicate delivered-load service")
		}
		loadServices[load.Service] = true
		var radiantOwners map[string]epathSQLRadiantOwnerProof
		if load.NativeRadiant != nil {
			if len(originalText) != 1 || strings.TrimSpace(originalText[0]) == "" {
				return out, fmt.Errorf("native radiant load requires its exact preserved original input")
			}
			radiantOwners, err = epathSQLRadiantLoadDeclaration(load, originalText[0])
			if err != nil {
				return out, err
			}
		}
		sources, err := epathSQLSelect(observed, load.Source)
		if err != nil {
			return out, err
		}
		for _, source := range sources {
			out.SourceIdentities[source.DictionaryIndex] = source
			zone := strings.ToLower(source.KeyValue)
			if load.NativeRadiant != nil {
				owner, found := radiantOwners[epathSQLRadiantName(source.KeyValue)]
				if !found {
					return out, fmt.Errorf("radiant load source has no exact original owner")
				}
				zone = strings.ToLower(owner.Owner.ZoneName)
				identity, err := epathSQLRadiantLoadObservation(source, load.Service, owner, out.Zones[zone], model.Precision)
				if err != nil {
					return out, err
				}
				if out.RadiantLoadSourceIdentities == nil {
					out.RadiantLoadSourceIdentities = map[int]epathSQLRadiantLoadSourceIdentity{}
				}
				out.RadiantLoadSourceIdentities[source.DictionaryIndex] = identity
				out.SourceRaw[source.DictionaryIndex] = append([]epathSQLQuantity(nil), identity.Raw[:]...)
				out.SourceEffective[source.DictionaryIndex] = append([]epathSQLQuantity(nil), identity.Effective[:]...)
				out.SourceZone[source.DictionaryIndex] = zone
				for month, value := range identity.Effective {
					out.Loads[epathSQLKey(zone, load.Service, month+1)] = value
					out.LoadSourceIDs[epathSQLKey(zone, load.Service, month+1)] = []int{source.DictionaryIndex}
				}
				continue // Native surface-source J already includes Zone and List multipliers.
			}
			if out.Zones[zone].Name == "" {
				return out, fmt.Errorf("unowned delivered-load Zone")
			}
			values, err := epathSQLMonthly(source, model.Precision)
			if err != nil {
				return out, err
			}
			out.SourceRaw[source.DictionaryIndex] = values
			out.SourceZone[source.DictionaryIndex] = zone
			effective := make([]epathSQLQuantity, 12)
			for month, value := range values {
				if value.Value < 0 {
					return out, fmt.Errorf("negative delivered-load observation")
				}
				out.Loads[epathSQLKey(zone, load.Service, month+1)] = value.times(out.Zones[zone].Multiplier)
				out.LoadSourceIDs[epathSQLKey(zone, load.Service, month+1)] = []int{source.DictionaryIndex}
				effective[month] = value.times(out.Zones[zone].Multiplier)
			}
			out.SourceEffective[source.DictionaryIndex] = effective
		}
	}
	if len(loadServices) != 2 {
		return out, fmt.Errorf("reviewed model requires both explicit delivered-load observations")
	}
	zeroPressure, err := epathSQLZeroPressureDeclarations(out, model)
	if err != nil {
		return out, err
	}
	out.ZeroPressureFallbacks = zeroPressure
	epathSQLRecordZeroPressurePhysicalCells(&out)
	for zone := range out.Zones {
		for month := 1; month <= 12; month++ {
			for _, service := range []string{"cooling", "heating"} {
				load, ok := out.Loads[epathSQLKey(zone, service, month)]
				if !ok {
					return out, fmt.Errorf("missing delivered-load Zone/month is not zero")
				}
				if declaration, declared := zeroPressure[epathSQLKey(zone, service, month)]; declared {
					if err := epathSQLApplyZeroPressureFallback(&out, model, declaration); err != nil {
						return out, err
					}
					continue
				}
				candidates := []*epathSQLCell{}
				denominator := epathSQLQuantity{}
				for _, cell := range out.Cells {
					if cell.Zone != zone || cell.Month != month || cell.ZeroPressureFallback {
						continue
					}
					if definition, ok := defined[cell.Family]; ok && definition.Role != "pressure" {
						continue
					}
					pressure := cell.Effective
					if service == "heating" {
						pressure = pressure.times(-1)
					}
					pressure = pressure.positive()
					denominator = denominator.add(pressure)
					candidates = append(candidates, cell)
				}
				for _, cell := range candidates {
					pressure := cell.Effective
					if service == "heating" {
						pressure = pressure.times(-1)
					}
					pressure = pressure.positive()
					value, err := epathSQLShare(load, pressure, denominator, model.Precision)
					if err != nil {
						return out, fmt.Errorf("%s/%d/%s: %w", zone, month, service, err)
					}
					cell.Allocated[service] = value
				}
			}
		}
	}
	annualCells := map[int]string{}
	for _, site := range model.Site {
		if err := epathValidateSQLSiteSource(site); err != nil {
			return out, err
		}
		_, annualExists := out.SiteAnnual[site.ID]
		if site.ID == "" || site.Carrier == "" || (site.EndUse == "") == !site.Facility || out.Site[site.ID] != nil || annualExists {
			return out, fmt.Errorf("invalid/duplicate site declaration")
		}
		if site.Tabular != nil {
			if _, err := epathSQLTabularSiteAlias(site); err != nil {
				return out, err
			}
			observation, err := epathReadSQLModelTabular(sqlPath, *site.Tabular)
			if err != nil {
				return out, err
			}
			if observation == nil {
				return out, fmt.Errorf("declared annual site %s has no original Tabular cell", site.ID)
			}
			if prior := annualCells[observation.TabularDataIndex]; prior != "" {
				return out, fmt.Errorf("annual site %s reuses the original cell already owned by %s", site.ID, prior)
			}
			annualCells[observation.TabularDataIndex] = site.ID
			out.SiteAnnual[site.ID] = *observation
			continue
		}
		sources, err := epathSQLSelect(observed, site.Source)
		if err != nil {
			return out, err
		}
		values := make([]*epathSQLQuantity, 12)
		for _, source := range sources {
			out.SourceIdentities[source.DictionaryIndex] = source
			out.SiteSources[site.ID] = append(out.SiteSources[site.ID], source.DictionaryIndex)
			months, err := epathSQLMonthly(source, model.Precision)
			if err != nil {
				return out, err
			}
			out.SourceRaw[source.DictionaryIndex] = months
			out.SourceEffective[source.DictionaryIndex] = months
			for index, value := range months {
				if value.Value < 0 {
					return out, fmt.Errorf("negative site-energy observation")
				}
				if values[index] == nil {
					values[index] = &epathSQLQuantity{}
				}
				sum := values[index].add(value)
				values[index] = &sum
			}
		}
		out.Site[site.ID] = values
	}
	if err := epathCompileSQLNonAdditiveLoadDetails(&out, observed, model); err != nil {
		return out, err
	}
	if err := epathCompileSQLTemporalTraceSources(sqlPath, observed, model, &out); err != nil {
		return out, err
	}
	if err := epathCompileSQLDirectHVACSources(sqlPath, observed, model, &out); err != nil {
		return out, err
	}
	if err := epathSQLValidateZeroPressureFrames(out, model); err != nil {
		return out, err
	}
	if err := epathSQLValidateRadiantSurfaceContextFrames(out); err != nil {
		return out, err
	}
	return out, nil
}

func epathSQLDictionaryUnion(left, right []int) []int {
	seen := map[int]bool{}
	for _, ids := range [][]int{left, right} {
		for _, id := range ids {
			seen[id] = true
		}
	}
	out := make([]int, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

func epathSQLShare(load, pressure, denominator epathSQLQuantity, precision epathRealSQLPrecision) (epathSQLQuantity, error) {
	if precision.DecimalPlaces != 3 || precision.SourceStages < 1 || precision.SourceStages > 3 || precision.ContributionStages < 1 || precision.ContributionStages > 3 {
		return epathSQLQuantity{}, fmt.Errorf("invalid allocation precision configuration")
	}
	for _, q := range []epathSQLQuantity{load, pressure, denominator} {
		if !q.valid() || q.Value < 0 {
			return epathSQLQuantity{}, fmt.Errorf("invalid allocation interval")
		}
	}
	loadLow, loadHigh := load.bounds()
	pressureLow, pressureHigh := pressure.bounds()
	denominatorLow, denominatorHigh := denominator.bounds()
	if loadHigh == 0 {
		return epathSQLQuantity{}, nil
	}
	if denominator.Value <= 0 {
		return epathSQLQuantity{}, fmt.Errorf("positive delivered load has no observed pressure denominator; explicit unassigned proof required")
	}
	value := load.Value * pressure.Value / denominator.Value
	if denominatorLow <= 0 {
		return epathSQLQuantity{}, fmt.Errorf("rounding interval cannot prove a positive allocation denominator")
	}
	if pressureHigh == 0 {
		return epathSQLQuantity{}, nil
	}
	low := math.Max(0, loadLow) * math.Max(0, pressureLow) / denominatorHigh
	high := loadHigh * pressureHigh / denominatorLow
	budget := float64(precision.ContributionStages) * .0005
	return epathSQLBounded(value, math.Max(0, low-budget), high+budget), nil
}
