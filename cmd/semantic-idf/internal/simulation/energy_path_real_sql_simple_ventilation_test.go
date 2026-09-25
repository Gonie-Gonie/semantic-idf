package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Independent finite oracle taxonomy. Do not import the production ventilation
// target resolver, alias catalog, multiplier or allocation helpers here.
const epathSQLSimpleVentilationFanFamily = "fans.simple_ventilation.electricity"
const epathSQLSimpleVentilationFanName = "Zone Ventilation Fan Electricity Energy"

func epathSQLSupportedDirectFanFamily(family string) bool {
	return family == epathSQLDirectFanFamily || family == epathSQLSimpleVentilationFanFamily
}

func epathSQLSimpleVentilationOwner(owner epathRealSQLDirectHVACOwner) bool {
	zone := strings.ToLower(strings.TrimSpace(owner.ZoneName))
	return (zone == "zone 1" || zone == "zone 2" || zone == "zone 3") &&
		owner.EquipmentType == "Zone" && owner.ComponentType == "Zone" &&
		strings.EqualFold(strings.TrimSpace(owner.KeyValue), zone) &&
		strings.EqualFold(strings.TrimSpace(owner.EquipmentName), zone)
}

// This is only the reviewed VentilationSimpleTest cohort, not general Zone,
// Space or ZoneList ventilation support. The caller separately binds the exact
// preserved original hash. A complete original census prevents a convenient
// subset or a second Zone 3 member from becoming a separate reporting source.
func epathSQLValidateSimpleVentilationOriginal(doc idf.Document, declaration epathRealSQLDirectHVACComponent) error {
	owners, err := epathSQLDirectHVACOwners(declaration)
	if err != nil || declaration.ID != epathSQLSimpleVentilationFanFamily || len(owners) != 3 {
		return fmt.Errorf("simple ventilation requires its exact three-Zone declaration: %v", err)
	}
	normal := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	field := func(object idf.Object, index int) string {
		if index >= len(object.Fields) {
			return ""
		}
		return strings.TrimSpace(object.Fields[index].Value)
	}
	type member struct{ kind, zone, ventilationType string }
	wanted := map[string]member{
		"zone 1 ventl 1": {"zoneventilation:designflowrate", "zone 1", "natural"},
		"zone 2 ventl 1": {"zoneventilation:designflowrate", "zone 2", "intake"},
		"zone 3 ventl 1": {"zoneventilation:designflowrate", "zone 3", "exhaust"},
		"zone 3 ventl 2": {"zoneventilation:windandstackopenarea", "zone 3", ""},
	}
	zones, members := map[string]bool{}, map[string]bool{}
	versions := 0
	for _, object := range doc.Objects {
		kind, name := normal(object.Type), normal(field(object, 0))
		if kind == "version" {
			versions++
			if field(object, 0) != "25.1" {
				return fmt.Errorf("simple ventilation oracle is restricted to native 25.1")
			}
		}
		if kind == "zone" {
			if owners[name].ZoneName == "" || zones[name] {
				return fmt.Errorf("simple ventilation has a foreign or duplicate original Zone")
			}
			multiplier, err := strconv.ParseFloat(field(object, 6), 64)
			if err != nil || multiplier != 1 {
				return fmt.Errorf("reviewed original Zone multiplier must be exactly one")
			}
			zones[name] = true
		}
		switch kind {
		case "zonelist", "zonegroup", "space", "spacelist", "zoneairbalance:outdoorair":
			return fmt.Errorf("simple ventilation original gained unsupported selector expansion or combined/Quadrature reporting")
		}
		for _, prefix := range []string{"airloophvac", "plantloop", "zonehvac:", "zonecontrol:", "coil:", "coilsystem:", "fan:", "pump:", "boiler:", "chiller:", "district", "airconditioner:", "centralheatpumpsystem", "zonecooltower:", "zoneearthtube", "zonethermalchimney", "airflownetwork:"} {
			if strings.HasPrefix(kind, prefix) {
				return fmt.Errorf("uncontrolled ventilation original gained an unreviewed physical source %s", object.Type)
			}
		}
		if !strings.HasPrefix(kind, "zoneventilation:") {
			continue
		}
		want, exists := wanted[name]
		if !exists || members[name] || kind != want.kind || normal(field(object, 1)) != want.zone {
			return fmt.Errorf("simple ventilation member is missing, duplicated, foreign or assigned to another Zone")
		}
		if want.ventilationType != "" {
			if normal(field(object, 8)) != want.ventilationType {
				return fmt.Errorf("original ventilation member changed its reviewed physical fan role")
			}
			for _, spec := range []struct {
				index    int
				positive bool
			}{{9, false}, {10, true}} {
				value, err := strconv.ParseFloat(field(object, spec.index), 64)
				if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || spec.positive && (value == 0 || value > 1) {
					return fmt.Errorf("invalid original ventilation fan pressure/efficiency")
				}
			}
		}
		members[name] = true
	}
	if versions != 1 || len(zones) != 3 || len(members) != 4 {
		return fmt.Errorf("simple ventilation original requires one 25.1 Version, three exact Zones and all four original members")
	}
	return nil
}

// The generic direct reader validates exact keys, complete Monthly weather
// observations, nonnegative J, request ownership and duplicate dictionaries.
// This new finite family additionally requires its actual native reporting
// semantics. Hourly is distinct context and cannot replace Monthly evidence.
func epathSQLValidateSimpleVentilationDictionary(db *sql.DB, declaration epathRealSQLDirectHVACComponent) error {
	if declaration.ID != epathSQLSimpleVentilationFanFamily {
		return nil
	}
	owners, err := epathSQLDirectHVACOwners(declaration)
	if err != nil {
		return err
	}
	rows, err := db.Query(`SELECT KeyValue,Units,IsMeter,Type,TimestepType,COALESCE(ScheduleName,'') FROM ReportDataDictionary WHERE Name=? COLLATE NOCASE AND ReportingFrequency='Monthly' COLLATE NOCASE`, epathSQLSimpleVentilationFanName)
	if err != nil {
		return err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var key, unit, reportType, timestep, schedule string
		var meter int
		if err := rows.Scan(&key, &unit, &meter, &reportType, &timestep, &schedule); err != nil {
			return err
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if owners[key].KeyValue == "" || seen[key] || unit != "J" || meter != 0 || reportType != "Sum" || timestep != "HVAC System" || schedule != "" {
			return fmt.Errorf("simple ventilation native dictionary contradicts its exact Zone aggregate Sum/J/System identity")
		}
		seen[key] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(seen) != 3 {
		return fmt.Errorf("simple ventilation requires three actual Monthly native dictionaries, not inferred zero")
	}
	return nil
}
