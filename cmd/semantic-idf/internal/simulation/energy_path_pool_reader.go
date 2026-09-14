package simulation

import (
	"database/sql"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Private physical evidence. None of these component series is an additional
// Building end use, a Zone-air load, or a passive surface driver.
type energyPathPoolEvidence struct {
	Inventory    energyPathPoolInventory
	Observations []energyPathPoolObservation
	LoadSeries   []energyExplanationSeries // Selected, multiplier-applied canonical load evidence.
}

type energyPathPoolObservation struct {
	Definition energyPathPoolOutputDefinition
	Component  idf.ComponentRef
	Loop       idf.ComponentRef
	Series     energyExplanationSeries // Unique Monthly Energy authority, never E+R.
	Valid      bool
}

func readEnergyPathPoolEvidence(db *sql.DB, sourceFile string, plan *PurposeRunPlan, context energyDriverBuildContext) (energyPathPoolEvidence, []EnergyDataSource, []string) {
	evidence := energyPathPoolEvidence{}
	if !context.Enabled || !context.PoolInventory.HasNativePool || !energyExplanationPlanUsesEnergyPath(plan) {
		return evidence, nil, nil
	}
	evidence.Inventory = context.PoolInventory
	axis := energyPathHVACConsumptionMonthlyAxis(db)
	hourly := newEnergySourceHourlyCollector(db)
	var sources []EnergyDataSource
	type target struct {
		component, loop idf.ComponentRef
		fuel            string
		valid           bool
	}
	var targets []target
	for _, pool := range evidence.Inventory.Pools {
		targets = append(targets, target{component: pool.Component, loop: pool.Loop, valid: pool.IdentityValid})
	}
	for _, source := range evidence.Inventory.Sources {
		targets = append(targets, target{component: source.Component, loop: source.Loop, fuel: source.FuelType, valid: source.IdentityValid})
	}
	for _, target := range targets {
		for _, definition := range energyPathPoolOutputDefinitions() {
			if !definition.matchesOriginal(target.component.ObjectType, target.fuel) {
				continue
			}
			observation := energyPathPoolObservation{Definition: definition, Component: target.component, Loop: target.loop, Valid: target.valid && evidence.Inventory.SchemaReviewed}
			if observation.Valid {
				dictionaries, err := energyPathPoolDictionaries(db, sourceFile, target.component.ObjectName, definition)
				if err != nil {
					observation.Valid = false
				}
				identities := map[string]int{}
				for _, dictionary := range dictionaries {
					identities[energyPathPoolToken(dictionary.row.name)+"|"+energyPathPoolToken(dictionary.reportingFrequency)]++
				}
				for _, dictionary := range dictionaries {
					source, item := energyPathReadPoolObservation(db, dictionary, axis, hourly, definition, target.component)
					sources = append(sources, source)
					if strings.EqualFold(dictionary.reportingFrequency, "Monthly") && strings.EqualFold(dictionary.row.name, definition.EnergyName) {
						if identities[energyPathPoolToken(dictionary.row.name)+"|monthly"] != 1 {
							observation.Valid = false
						} else {
							observation.Series = item
						}
					}
				}
			}
			evidence.Observations = append(evidence.Observations, observation)
		}
	}
	var labels []string
	if hourly != nil {
		labels = hourly.labels
	}
	return evidence, sources, labels
}

func energyPathPoolDictionaries(db *sql.DB, sourceFile, key string, definition energyPathPoolOutputDefinition) ([]energyExplanationDictionary, error) {
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,KeyValue,Name,Units,ReportingFrequency FROM ReportDataDictionary
WHERE IsMeter=0 AND LOWER(TRIM(KeyValue))=LOWER(TRIM(?))
AND LOWER(TRIM(Name)) IN (LOWER(?),LOWER(?))
AND LOWER(TRIM(ReportingFrequency)) IN ('monthly','hourly') ORDER BY ReportDataDictionaryIndex`, key, definition.EnergyName, definition.RateName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []energyExplanationDictionary
	for rows.Next() {
		dictionary := energyExplanationDictionary{sourceFile: sourceFile}
		if err := rows.Scan(&dictionary.row.index, &dictionary.row.keyValue, &dictionary.row.name, &dictionary.row.units, &dictionary.reportingFrequency); err != nil {
			return nil, err
		}
		// Unit integration authorization only, not an additive load alias.
		if strings.EqualFold(strings.TrimSpace(dictionary.row.name), definition.RateName) {
			dictionary.load = &energyLoadAliasDefinition{ServiceKind: definition.ServiceKind}
		}
		out = append(out, dictionary)
	}
	return out, rows.Err()
}

func energyPathReadPoolObservation(db *sql.DB, dictionary energyExplanationDictionary, axis map[int64]int, hourly *energySourceHourlyCollector, definition energyPathPoolOutputDefinition, component idf.ComponentRef) (EnergyDataSource, energyExplanationSeries) {
	source := energyDataSourceForDictionary(dictionary)
	source.DriverRole, source.InspectorSection = energyDriverSourceRoleContext, energyDriverInspectorSectionContext
	source.AggregationBasis, source.EffectiveMultiplier, source.MultiplierApplication = "model_total", 1, "already_model_total"
	objectIndex := component.ObjectIndex
	source.ObjectIndex = &objectIndex
	source.RelatedEntityIDs = appendUniqueStrings(source.RelatedEntityIDs, component.ID)
	source.Explanation = "Native purchased component consumption, included in its resource end-use meter. Nonadditive source context; a complete original demand roster and independently observed constituent budget are required for any allocation."
	if definition.Role == "thermal_context" {
		source.Explanation = "Native plant thermal transfer, model-total factor 1. Nonadditive context: not purchased fuel, not Zone-air delivered load, and not an additional passive surface driver. Thermal output does not establish a fuel or pump service split."
	}
	item := energyExplanationSeries{Stage: "context", Level: "context", Kind: "context." + definition.ID,
		EndUse: definition.EndUse, ServiceKind: definition.ServiceKind, Carrier: definition.Carrier,
		SourceName: dictionary.row.name, SourceKey: dictionary.row.keyValue, sourceName: dictionary.row.name, sourceKeyValue: dictionary.row.keyValue,
		sourceFrequency: dictionary.reportingFrequency, Unit: "kWh", SourceIDs: []string{source.ID}, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", multiplierApplied: true}
	return energyPathReadNativeConstituentObservation(db, dictionary, axis, hourly, source, item)
}

// Existing native-baseboard pools retain their direct constituents. A matching
// boiler member is replaced, not counted twice; its former Zone-only path proof
// cannot authorize a source whose native plant also serves pool water.
func mergeEnergyPathPoolHVACConsumption(existing []energyPathHVACConsumptionPool, evidence energyPathPoolEvidence, series []energyExplanationSeries) []energyPathHVACConsumptionPool {
	if !evidence.Inventory.HasNativePool {
		return existing
	}
	out := append([]energyPathHVACConsumptionPool(nil), existing...)
	for _, carrier := range []string{"natural_gas", "electricity"} {
		var meterIDs []string
		for _, item := range series {
			if item.Stage == "end_use" && item.ZoneName == "" && item.EndUse == "heating" && item.Carrier == carrier && item.directComponentID == "" {
				meterIDs = appendUniqueStrings(meterIDs, item.SourceIDs...)
			}
		}
		if len(meterIDs) == 0 {
			continue
		}
		index := -1
		for i, pool := range out {
			if pool.ServiceKind == "heating" && pool.Carrier == carrier && len(energyPathZoneHVACIntersectPaths(pool.MeterSourceIDs, meterIDs)) > 0 {
				index = i
				break
			}
		}
		if index < 0 {
			out = append(out, energyPathHVACConsumptionPool{ID: "native.pool.heating." + carrier, ServiceKind: "heating", Carrier: carrier, MeterSourceIDs: meterIDs, Valid: true})
			index = len(out) - 1
		}
		pool := &out[index]
		pool.Members = append([]energyPathHVACConsumptionMember(nil), pool.Members...)
		for _, observation := range evidence.Observations {
			definition := observation.Definition
			if definition.Role != "purchased_constituent" || definition.EndUse != "heating" || definition.Carrier != carrier {
				continue
			}
			member := energyPathHVACConsumptionMember{ID: definition.ID + ":" + energyPathPoolToken(observation.Component.ObjectName), ObjectType: observation.Component.ObjectType,
				ObjectName: observation.Component.ObjectName, OutputName: definition.EnergyName}
			if observation.Valid {
				member.Series = observation.Series
			}
			member.RelatedPathIDs = energyPathPoolEligibleSourcePaths(evidence.Inventory, observation.Component, "heating")
			replaced := false
			for i, previous := range pool.Members {
				if energyPathPoolEqual(previous.ObjectType, member.ObjectType) && energyPathPoolEqual(previous.ObjectName, member.ObjectName) && energyPathPoolEqual(previous.OutputName, member.OutputName) {
					pool.Members[i] = member
					replaced = true
					break
				}
			}
			if !replaced {
				pool.Members = append(pool.Members, member)
			}
		}
		// Empty roster deliberately remains invalid-to-allocate, never a signal
		// to restore broad load shares after an unsupported native identity.
		sort.Strings(pool.MeterSourceIDs)
	}
	return out
}

func energyPathPoolEligibleSourcePaths(inventory energyPathPoolInventory, component idf.ComponentRef, service string) []string {
	if !inventory.HasNativePool || !inventory.SchemaReviewed || len(inventory.UnresolvedPoolIndices) != 0 {
		return nil
	}
	for _, source := range inventory.Sources {
		if !energyPathPoolAirSameRef(source.Component, component) || !source.IdentityValid || !source.WaterOwnerValid {
			continue
		}
		for _, loop := range inventory.Loops {
			if loop.Component.ObjectIndex != source.Loop.ObjectIndex || !loop.WaterTopologyComplete || !loop.SupplyRosterComplete || !loop.DemandRosterComplete || loop.HasNonZoneDemand || !loop.AirRoutesComplete || len(loop.Demands) == 0 {
				continue
			}
			var paths []string
			for _, demand := range loop.Demands {
				if demand.Kind != service+"_coil" || !demand.AirRouteComplete || len(demand.RelatedPathIDs) == 0 {
					return nil
				}
				paths = appendUniqueStrings(paths, demand.RelatedPathIDs...)
			}
			sort.Strings(paths)
			return paths
		}
	}
	return nil
}
