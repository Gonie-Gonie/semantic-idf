package simulation

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// One finite equation in the existing independent compiler, not a classifier.
// Four distinct nonnegative J authorities retain their signed thermal roles;
// the W aggregate subtracts sensible detail only, never paid fan electricity.
func epathSQLSimpleVentilationOutdoorDeclaration(model epathRealSQLModel) (bool, error) {
	names := map[string]string{
		"ventilation.sensible.gain": "Zone Ventilation Sensible Heat Gain Energy",
		"ventilation.sensible.loss": "Zone Ventilation Sensible Heat Loss Energy",
		"ventilation.latent.gain":   "Zone Ventilation Latent Heat Gain Energy",
		"ventilation.latent.loss":   "Zone Ventilation Latent Heat Loss Energy",
		"outdoor.balance":           "Zone Air Heat Balance Outdoor Air Transfer Rate",
	}
	active := false
	for _, family := range model.Families {
		active = active || family.ID == "outdoor.balance"
		for _, term := range family.Terms {
			for _, a := range term.Source.Alternatives {
				for id, name := range names {
					active = active || id != "outdoor.balance" && a.Name == name
				}
			}
		}
	}
	if !active {
		return false, nil
	}
	if len(model.DirectHVACComponents) != 1 || model.DirectHVACComponents[0].ID != epathSQLSimpleVentilationFanFamily {
		return true, fmt.Errorf("finite outdoor balance requires original simple-ventilation cohort")
	}
	wantedOwners := []string{"zone 1", "zone 2", "zone 3"}
	found, uses := map[string]bool{}, map[string]int{}
	for _, family := range model.Families {
		for _, term := range family.Terms {
			for _, a := range term.Source.Alternatives {
				for _, name := range names {
					if a.Name == name {
						uses[name]++
					}
				}
			}
		}
		name, relevant := names[family.ID]
		if !relevant {
			continue
		}
		if found[family.ID] {
			return true, fmt.Errorf("duplicate finite outdoor family")
		}
		found[family.ID] = true
		owners, err := epathSQLAirLoopFanZones(family.Keys)
		if err != nil || !reflect.DeepEqual(owners, wantedOwners) || family.Role != "pressure" || !family.BuildingVisible || len(family.Terms) != 1 || len(family.TraceSources) != 0 {
			return true, fmt.Errorf("finite outdoor family lost original owners/role")
		}
		term := family.Terms[0]
		unit, component, category, sign := "J", "sensible", "air.mechanical_ventilation", 1.0
		if strings.Contains(family.ID, ".latent.") {
			component = "latent"
		}
		if strings.HasSuffix(family.ID, ".loss") {
			sign = -1
		}
		if family.ID == "outdoor.balance" {
			unit, category = "W", "balance.storage_other"
		}
		if family.Category != category || family.Component != component || term.Sign != sign || term.Source.IsMeter || term.Source.AllowAbsent ||
			!reflect.DeepEqual(term.Source.Alternatives, []epathRealSQLAlternative{{Name: name, Unit: unit}}) {
			return true, fmt.Errorf("finite outdoor family has substituted unit/name/sign/component")
		}
		if len(term.Source.Keys) != 0 {
			keys, e := epathSQLAirLoopFanZones(term.Source.Keys)
			if e != nil || !reflect.DeepEqual(keys, wantedOwners) {
				return true, fmt.Errorf("finite outdoor term owner differs")
			}
		}
		if family.ID == "outdoor.balance" {
			if !reflect.DeepEqual(family.Subtract, []string{"ventilation.sensible.gain", "ventilation.sensible.loss"}) {
				return true, fmt.Errorf("outdoor sensible aggregate must subtract only signed sensible gain/loss")
			}
		} else if len(family.Subtract) != 0 {
			return true, fmt.Errorf("native ventilation detail cannot subtract another authority")
		}
	}
	for id, name := range names {
		if !found[id] || uses[name] != 1 {
			return true, fmt.Errorf("missing or duplicated finite outdoor authority %s", id)
		}
	}
	for _, family := range model.Families {
		if family.ID == "ventilation.sensible" || family.ID == "outdoor.unsplit" {
			return true, fmt.Errorf("finite outdoor details cannot coexist with an aggregate fallback")
		}
	}
	return true, nil
}

// Reuses the existing bound original, native-direct fan, canonical-zero service
// and Hourly companion proof paths. It adds no retained approval flag or registry.
func epathSQLValidateSimpleVentilationOutdoorEvidence(observed epathRealOracleEvidence, frames epathSQLFrames, model epathRealSQLModel) error {
	active, err := epathSQLSimpleVentilationOutdoorDeclaration(model)
	if err != nil || !active {
		return err
	}
	if err := epathSQLSimpleVentilationZeroServices(frames, model, observed); err != nil {
		return fmt.Errorf("outdoor original boundary: %w", err)
	}
	original, err := idf.Parse(observed.originalText)
	if err != nil {
		return err
	}
	executed, err := idf.Parse(observed.executedText)
	if err != nil {
		return err
	}
	db, err := epathOpenOracleSQL(observed.sqlPath)
	if err != nil {
		return err
	}
	defer db.Close()
	var negatives, orphans int
	if err := db.QueryRow(`SELECT COALESCE(SUM(CASE WHEN d.Units='J' AND r.Value<0 THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN t.TimeIndex IS NULL OR e.EnvironmentPeriodIndex IS NULL THEN 1 ELSE 0 END),0) FROM ReportData r JOIN ReportDataDictionary d USING(ReportDataDictionaryIndex) LEFT JOIN Time t USING(TimeIndex) LEFT JOIN EnvironmentPeriods e USING(EnvironmentPeriodIndex) WHERE d.Name IN ('Zone Ventilation Sensible Heat Gain Energy','Zone Ventilation Sensible Heat Loss Energy','Zone Ventilation Latent Heat Gain Energy','Zone Ventilation Latent Heat Loss Energy','Zone Air Heat Balance Outdoor Air Transfer Rate') AND d.ReportingFrequency IN ('Monthly','Hourly')`).Scan(&negatives, &orphans); err != nil {
		return err
	}
	if negatives != 0 {
		return fmt.Errorf("negative original native gain/loss Energy")
	}
	if orphans != 0 {
		return fmt.Errorf("orphan original native outdoor row")
	}
	for _, family := range model.Families {
		switch family.ID {
		case "outdoor.balance", "ventilation.sensible.gain", "ventilation.sensible.loss", "ventilation.latent.gain", "ventilation.latent.loss":
		default:
			continue
		}
		a := family.Terms[0].Source.Alternatives[0]
		for _, frequency := range []string{"Monthly", "Hourly"} {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM ReportDataDictionary WHERE Name=? COLLATE NOCASE AND ReportingFrequency=?`, a.Name, frequency).Scan(&count); err != nil {
				return err
			}
			if count != 3 {
				return fmt.Errorf("outdoor native family has foreign/missing reporting owners")
			}
		}
		declarations := 0
		for _, h := range model.HourlyCompanions {
			if h.Name == a.Name {
				keys, e := epathSQLAirLoopFanZones(h.Keys)
				if e != nil || h.Unit != a.Unit || !reflect.DeepEqual(keys, []string{"zone 1", "zone 2", "zone 3"}) {
					return fmt.Errorf("outdoor companion declaration lost exact owners/unit")
				}
				declarations++
			}
		}
		if declarations != 1 {
			return fmt.Errorf("outdoor family requires one complete Hourly companion declaration")
		}
		for _, zone := range family.Keys {
			companions := 0
			for _, trace := range frames.TraceSourceIdentities {
				p := trace.NativeCompanion
				if p == nil || p.Authority.Name != a.Name || !strings.EqualFold(p.ZoneName, zone) {
					continue
				}
				if err := epathSQLValidateHourlyCompanion(*p); err != nil {
					return err
				}
				if p.Multiplier != 1 || !reflect.DeepEqual(frames.SourceIdentities[p.Authority.DictionaryIndex], p.Authority) || !strings.EqualFold(frames.SourceZone[p.Authority.DictionaryIndex], zone) {
					return fmt.Errorf("outdoor native authority lost original factor-one owner")
				}
				if a.Unit == "J" {
					for _, value := range p.HourlyValues {
						if value < 0 {
							return fmt.Errorf("negative native gain/loss Energy")
						}
					}
				}
				companions++
			}
			if companions != 1 {
				return fmt.Errorf("outdoor family lost exact complete native Hourly companion")
			}
			for _, frequency := range []string{"Monthly", "Hourly"} {
				spec := epathSQLPVNativeSpec{Name: a.Name, Key: zone}
				if _, err := epathSQLPVRequestBinding(original, executed, observed.outputPlan, spec, frequency); err != nil {
					return fmt.Errorf("outdoor actual request: %w", err)
				}
				rows, err := db.Query(`SELECT Units,IsMeter,Type,IndexGroup,TimestepType,COALESCE(ScheduleName,'') FROM ReportDataDictionary WHERE Name=? COLLATE NOCASE AND KeyValue=? COLLATE NOCASE AND ReportingFrequency=?`, a.Name, zone, frequency)
				if err != nil {
					return err
				}
				count := 0
				for rows.Next() {
					var unit, typ, group, step, schedule string
					var meter int
					if err := rows.Scan(&unit, &meter, &typ, &group, &step, &schedule); err != nil {
						rows.Close()
						return err
					}
					wantType := "Sum"
					if a.Unit == "W" {
						wantType = "Avg"
					}
					if unit != a.Unit || meter != 0 || typ != wantType || group != "System" || step != "HVAC System" || schedule != "" {
						rows.Close()
						return fmt.Errorf("outdoor native metadata changed")
					}
					count++
				}
				err = rows.Err()
				rows.Close()
				if err != nil {
					return err
				}
				if count != 1 {
					return fmt.Errorf("outdoor dictionary census changed")
				}
			}
		}
	}
	return nil
}
