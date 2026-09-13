package simulation

import (
	"database/sql"
	"math"
	"path/filepath"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Areas belong to the executed model. Energy values remain in kWh; these
// denominators support display intensity without changing the energy ledger.
type energyFloorAreas struct {
	building float64
	zones    map[string]float64
}

func validEnergyFloorArea(area float64) float64 {
	if area > 0 && !math.IsNaN(area) && !math.IsInf(area, 0) {
		return area
	}
	return 0
}

func energyFloorAreasForRun(result *SimulationRunResult, document *idf.Document) energyFloorAreas {
	for _, file := range result.Files {
		if file.Kind != "sqlite" && !strings.EqualFold(filepath.Ext(file.Name), ".sql") {
			continue
		}
		if areas, available := readEnergyFloorAreasSQL(file.Path); available {
			return areas
		}
	}
	if document == nil {
		return energyFloorAreas{}
	}
	return energyFloorAreasFromDocument(*document)
}

func readEnergyFloorAreasSQL(path string) (energyFloorAreas, bool) {
	db, err := openSimulationSQLiteReadOnly(path)
	if err != nil {
		return energyFloorAreas{}, false
	}
	defer db.Close()
	rows, err := db.Query(`SELECT ZoneName, FloorArea, Multiplier, ListMultiplier, IsPartOfTotalArea FROM Zones`)
	if err != nil {
		return energyFloorAreas{}, false
	}
	defer rows.Close()
	areas := energyFloorAreas{zones: map[string]float64{}}
	seen := map[string]bool{}
	complete, found := true, false
	for rows.Next() {
		var name sql.NullString
		var area, multiplier, listMultiplier sql.NullFloat64
		var included sql.NullInt64
		if rows.Scan(&name, &area, &multiplier, &listMultiplier, &included) != nil {
			return energyFloorAreas{}, false
		}
		found = true
		key := normalizePurposeToken(name.String)
		if key == "" || seen[key] {
			delete(areas.zones, key)
			complete = false
			continue
		}
		seen[key] = true
		effective := area.Float64 * multiplier.Float64 * listMultiplier.Float64
		valid := area.Valid && area.Float64 >= 0 && !math.IsNaN(area.Float64) && !math.IsInf(area.Float64, 0) &&
			multiplier.Valid && validEnergyFloorArea(multiplier.Float64) > 0 &&
			listMultiplier.Valid && validEnergyFloorArea(listMultiplier.Float64) > 0 &&
			!math.IsNaN(effective) && !math.IsInf(effective, 0)
		if valid && effective > 0 {
			areas.zones[key] = effective
		}
		if !included.Valid || included.Int64 != 0 && included.Int64 != 1 {
			complete = false
		} else if included.Int64 == 1 {
			if valid {
				areas.building += effective
			} else {
				complete = false
			}
		}
	}
	if rows.Err() != nil {
		return energyFloorAreas{}, false
	}
	if !complete {
		areas.building = 0
	}
	areas.building = validEnergyFloorArea(areas.building)
	return areas, found
}

func energyFloorAreasFromDocument(document idf.Document) energyFloorAreas {
	areas := energyFloorAreas{zones: map[string]float64{}}
	multipliers := buildEnergyEffectiveMultiplierIndex(document)
	profile := idf.AnalyzeProfile(document)
	for _, zone := range profile.ZoneProfiles {
		multiplier, known := multipliers.resolve(zone.ZoneName)
		if known {
			areas.zones[normalizePurposeToken(zone.ZoneName)] = validEnergyFloorArea(zone.FloorArea * multiplier.effectiveMultiplier())
		}
	}
	complete := true
	for _, object := range document.Objects {
		if !strings.EqualFold(object.Type, "Zone") || strings.EqualFold(energyObjectStringField(object, 12, "part of total floor area"), "No") {
			continue
		}
		area := areas.zones[normalizePurposeToken(purposeObjectName(object))]
		if area > 0 {
			areas.building += area
		} else {
			complete = false
		}
	}
	if !complete {
		areas.building = 0
	}
	areas.building = validEnergyFloorArea(areas.building)
	return areas
}

func applyEnergyFloorAreas(explanation *EnergyExplanationResult, areas energyFloorAreas) {
	setScope := func(scope *EnergyExplanationScope) {
		if scope.Kind == "zone" {
			scope.FloorAreaM2 = validEnergyFloorArea(areas.zones[normalizePurposeToken(scope.ZoneName)])
		} else {
			scope.FloorAreaM2 = validEnergyFloorArea(areas.building)
		}
	}
	setScope(&explanation.Scope)
	setPeriods := func(periods []EnergyPeriod, scope EnergyExplanationScope) {
		for index := range periods {
			if periods[index].Summary != nil {
				periods[index].Summary.Scope = scope
			}
		}
	}
	setPeriods(explanation.Periods, explanation.Scope)
	for index := range explanation.ZoneResults {
		zone := &explanation.ZoneResults[index]
		setScope(&zone.Scope)
		zone.Summary.Scope = zone.Scope
		setPeriods(zone.Periods, zone.Scope)
	}
}
