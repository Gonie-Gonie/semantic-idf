package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type epathSQLQuantity struct{ Value, Error float64 }

func (q epathSQLQuantity) add(other epathSQLQuantity) epathSQLQuantity {
	return epathSQLQuantity{q.Value + other.Value, q.Error + other.Error}
}
func (q epathSQLQuantity) times(factor float64) epathSQLQuantity {
	return epathSQLQuantity{q.Value * factor, q.Error * math.Abs(factor)}
}
func (q epathSQLQuantity) positive() epathSQLQuantity {
	return epathSQLQuantity{math.Max(0, q.Value), q.Error}
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
}
type epathSQLFrames struct {
	Zones           map[string]epathSQLZone
	Cells           map[string]*epathSQLCell
	Loads           map[string]epathSQLQuantity
	Site            map[string][]*epathSQLQuantity
	SourceRaw       map[int][]epathSQLQuantity
	SourceEffective map[int][]epathSQLQuantity
	SourceZone      map[int]string
}

func epathSQLKey(zone, family string, month int) string {
	return fmt.Sprintf("%s|%s|%02d", strings.ToLower(zone), family, month)
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
	if len(source.Months) != 12 {
		return nil, fmt.Errorf("source %s/%s does not have twelve actual monthly observations", source.Name, source.KeyValue)
	}
	values := make([]epathSQLQuantity, 12)
	for index, bucket := range source.Months {
		if bucket.Month != index+1 || bucket.Rows != 1 || bucket.EnergyKWh == nil || !epathOracleFinite(*bucket.EnergyKWh) {
			return nil, fmt.Errorf("source %s/%s has unknown/duplicate monthly energy %d", source.Name, source.KeyValue, bucket.Month)
		}
		values[index] = epathSQLQuantity{*bucket.EnergyKWh, float64(precision.SourceStages) * .0005}
	}
	return values, nil
}

func epathCompileSQLModelFrames(sqlPath string, observed []epathRealSQLSource, model epathRealSQLModel) (epathSQLFrames, error) {
	out := epathSQLFrames{Zones: map[string]epathSQLZone{}, Cells: map[string]*epathSQLCell{}, Loads: map[string]epathSQLQuantity{}, Site: map[string][]*epathSQLQuantity{}, SourceRaw: map[int][]epathSQLQuantity{}, SourceEffective: map[int][]epathSQLQuantity{}, SourceZone: map[int]string{}}
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
	add := func(zone, family, category, component string, month int, raw epathSQLQuantity, visible bool) {
		key := epathSQLKey(zone, family, month)
		cell := out.Cells[key]
		if cell == nil {
			cell = &epathSQLCell{Zone: zone, Family: family, Category: category, Component: component, Month: month, Allocated: map[string]epathSQLQuantity{}, BuildingVisible: visible}
			out.Cells[key] = cell
		}
		cell.Raw = cell.Raw.add(raw)
		cell.Effective = cell.Raw.times(out.Zones[zone].Multiplier)
	}
	surfaceSources, err := epathSQLSelect(observed, model.Surface.Source)
	if err != nil {
		return out, err
	}
	for _, source := range surfaceSources {
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
			add(surface.Zone, "surface:"+surface.Category, surface.Category, "sensible", month+1, signed, true)
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
				add(key, family.ID, family.Category, family.Component, month, epathSQLQuantity{}, family.BuildingVisible)
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
					add(zone, family.ID, family.Category, family.Component, month+1, value.times(term.Sign), family.BuildingVisible)
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
					if dependency == "surface.total" {
						for _, cell := range out.Cells {
							if cell.Zone == zone && cell.Month == month && strings.HasPrefix(cell.Family, "surface:") {
								subtract = subtract.add(cell.Raw)
							}
						}
					} else if cell := out.Cells[epathSQLKey(zone, dependency, month)]; cell != nil {
						subtract = cell.Raw
					}
					add(zone, family.ID, family.Category, family.Component, month, subtract.times(-1), family.BuildingVisible)
				}
			}
		}
		defined[family.ID] = family
	}
	loadServices := map[string]bool{}
	for _, load := range model.Loads {
		if (load.Service != "cooling" && load.Service != "heating") || loadServices[load.Service] || load.Component != "sensible" {
			return out, fmt.Errorf("unknown/duplicate delivered-load service")
		}
		loadServices[load.Service] = true
		sources, err := epathSQLSelect(observed, load.Source)
		if err != nil {
			return out, err
		}
		for _, source := range sources {
			zone := strings.ToLower(source.KeyValue)
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
				effective[month] = value.times(out.Zones[zone].Multiplier)
			}
			out.SourceEffective[source.DictionaryIndex] = effective
		}
	}
	if len(loadServices) != 2 {
		return out, fmt.Errorf("reviewed model requires both explicit delivered-load observations")
	}
	for zone := range out.Zones {
		for month := 1; month <= 12; month++ {
			for _, service := range []string{"cooling", "heating"} {
				load, ok := out.Loads[epathSQLKey(zone, service, month)]
				if !ok {
					return out, fmt.Errorf("missing delivered-load Zone/month is not zero")
				}
				candidates := []*epathSQLCell{}
				denominator := epathSQLQuantity{}
				for _, cell := range out.Cells {
					if cell.Zone != zone || cell.Month != month {
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
	for _, site := range model.Site {
		if site.ID == "" || site.Carrier == "" || (site.EndUse == "") == !site.Facility || out.Site[site.ID] != nil {
			return out, fmt.Errorf("invalid/duplicate site declaration")
		}
		sources, err := epathSQLSelect(observed, site.Source)
		if err != nil {
			return out, err
		}
		values := make([]*epathSQLQuantity, 12)
		for _, source := range sources {
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
	return out, nil
}

func epathSQLShare(load, pressure, denominator epathSQLQuantity, precision epathRealSQLPrecision) (epathSQLQuantity, error) {
	for _, q := range []epathSQLQuantity{load, pressure, denominator} {
		if !epathOracleFinite(q.Value) || !epathOracleFinite(q.Error) || q.Value < 0 || q.Error < 0 {
			return epathSQLQuantity{}, fmt.Errorf("invalid allocation interval")
		}
	}
	if load.Value == 0 {
		return epathSQLQuantity{}, nil
	}
	if denominator.Value <= 0 {
		return epathSQLQuantity{}, fmt.Errorf("positive delivered load has no observed pressure denominator; explicit unassigned proof required")
	}
	value := load.Value * pressure.Value / denominator.Value
	lowDenominator := denominator.Value - denominator.Error
	if lowDenominator <= 0 {
		return epathSQLQuantity{}, fmt.Errorf("rounding interval cannot prove a positive allocation denominator")
	}
	low := math.Max(0, load.Value-load.Error) * math.Max(0, pressure.Value-pressure.Error) / (denominator.Value + denominator.Error)
	high := (load.Value + load.Error) * (pressure.Value + pressure.Error) / lowDenominator
	return epathSQLQuantity{value, math.Max(value-low, high-value) + float64(precision.ContributionStages)*.0005}, nil
}
