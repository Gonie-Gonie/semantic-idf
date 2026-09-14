package simulation

// Native electrical boundary. Native source-role qualification only; no source scalar
// rewrites, fuel-efficiency model, generic ancillary allocation or UI change.
import (
	"math"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const (
	energyPathCogenerationConsumed   = "consumed_input"
	energyPathCogenerationProduced   = "produced_electricity"
	energyPathCogenerationPurchased  = "purchased_electricity"
	energyPathCogenerationSold       = "sold_electricity"
	energyPathCogenerationNet        = "net_electricity_context"
	energyPathCogenerationThermal    = "thermal_transfer_context"
	energyPathCogenerationUnresolved = "unresolved_native_resource"
)

// Optional durable annotation on the qualified constituent. NonFlow is a
// semantic guard, not a value; malformed present annotations must remain deny-
// only at active reload adapters. Do not attach this to unrelated Other nodes.
type energyPathCogenerationResourceBoundary struct {
	Role     string `json:"role"`
	Resource string `json:"resource"`
	SourceID string `json:"sourceId"`
	Reason   string `json:"reason"`
	NonFlow  bool   `json:"nonFlow"`
}

func energyPathCogenerationBoundaryIsNonFlow(b *energyPathCogenerationResourceBoundary) bool {
	if b == nil {
		return false
	}
	switch b.Role {
	case energyPathCogenerationConsumed, energyPathCogenerationProduced, energyPathCogenerationPurchased, energyPathCogenerationSold:
		return b.NonFlow
	default:
		return true // Missing/unknown role never authorizes a new flow.
	}
}

type energyPathCogenerationInventory struct {
	HasOriginal      bool
	ReviewedVersion  bool
	CustomMeterNames map[string]bool
	// This is deliberately a finite single-LookUpTable-inverter census, NOT a
	// global inventory of every possible native Cogeneration contributor.
	SoleInverterKey string
}

func energyPathCogenerationKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func energyPathCogenerationField(o idf.Object, n int) string {
	if n < 0 || n >= len(o.Fields) {
		return ""
	}
	return strings.TrimSpace(o.Fields[n].Value)
}

// Invocation itself means original supplied, including an empty document.
// A zero inventory is the sole omitted-original compatibility state.
func energyPathBuildCogenerationInventory(doc idf.Document) energyPathCogenerationInventory {
	out := energyPathCogenerationInventory{HasOriginal: true, CustomMeterNames: map[string]bool{}}
	versions := 0
	var version string
	var inverters, centers []idf.Object
	finite := true
	for _, o := range doc.Objects {
		typ := energyPathCogenerationKey(o.Type)
		if typ == "version" {
			versions++
			version = energyPathCogenerationField(o, 0)
		}
		if strings.HasPrefix(typ, "meter:custom") {
			out.CustomMeterNames[energyPathCogenerationKey(energyPathCogenerationField(o, 0))] = true
			finite = false
		}
		if strings.HasPrefix(typ, "generator:") && typ != "generator:photovoltaic" {
			finite = false
		}
		// Native EMS metered output can join the same predefined resource /
		// end-use parent without being an ElectricLoadCenter or Generator.
		// This only denies sole-inverter fallback; an observed parent remains
		// the native consumed-input authority regardless of contributor count.
		if typ == "energymanagementsystem:meteredoutputvariable" && energyPathCogenerationKey(energyPathCogenerationField(o, 4)) == "electricity" {
			endUse := energyPathCogenerationKey(energyPathCogenerationField(o, 6))
			if endUse == "onsitegeneration" || endUse == "cogeneration" {
				finite = false
			}
		}
		if !strings.HasPrefix(typ, "electricloadcenter:") {
			continue
		}
		switch typ {
		case "electricloadcenter:inverter:lookuptable":
			inverters = append(inverters, o)
		case "electricloadcenter:distribution":
			centers = append(centers, o)
		case "electricloadcenter:generators", "electricloadcenter:storage:simple", "electricloadcenter:storage:battery", "electricloadcenter:storage:liionnmcbattery":
		default:
			finite = false // Other inverter, converter, transformer, etc.
		}
	}
	out.ReviewedVersion = versions == 1 && version == "25.1"
	if finite && out.ReviewedVersion && len(inverters) == 1 && len(centers) == 1 {
		inv, center := inverters[0], centers[0]
		key := energyPathCogenerationField(inv, 0)
		bus := energyPathCogenerationKey(energyPathCogenerationField(center, 6))
		if key != "" && energyPathCogenerationField(center, 0) != "" && strings.EqualFold(energyPathCogenerationField(center, 7), key) && (bus == "directcurrentwithinverter" || bus == "directcurrentwithinverteracstorage" || bus == "directcurrentwithinverterdcstorage") {
			out.SoleInverterKey = key
		}
	}
	return out
}

func energyPathCogenerationResource(resource string) (role, carrier string) {
	switch energyPathCogenerationKey(resource) {
	case "electricity":
		return energyPathCogenerationConsumed, "electricity"
	case "naturalgas":
		return energyPathCogenerationConsumed, "natural_gas"
	case "propane":
		return energyPathCogenerationConsumed, "propane"
	case "diesel":
		return energyPathCogenerationConsumed, "diesel"
	case "gasoline":
		return energyPathCogenerationConsumed, "gasoline"
	case "coal":
		return energyPathCogenerationConsumed, "coal"
	case "fueloilno1":
		return energyPathCogenerationConsumed, "fuel_oil_1"
	case "fueloilno2":
		return energyPathCogenerationConsumed, "fuel_oil_2"
	case "otherfuel1":
		return energyPathCogenerationConsumed, "other_fuel_1"
	case "otherfuel2":
		return energyPathCogenerationConsumed, "other_fuel_2"
	case "electricityproduced":
		return energyPathCogenerationProduced, "electricity"
	case "electricitypurchased":
		return energyPathCogenerationPurchased, "electricity"
	case "electricitysurplussold":
		return energyPathCogenerationSold, "electricity"
	case "electricitynet":
		return energyPathCogenerationNet, "electricity"
	case "energytransfer":
		return energyPathCogenerationThermal, "energy_transfer"
	}
	return energyPathCogenerationUnresolved, ""
}

func energyPathCogenerationMeterResource(name string) (string, bool) {
	parts := strings.Split(strings.TrimSpace(name), ":")
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Cogeneration") || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return strings.TrimSpace(parts[1]), true
}
func energyPathCogenerationNativeFrequency(f string) bool {
	switch energyPathCogenerationKey(f) {
	case "monthly", "hourly", "daily", "run period", "annual", "timestep", "zone timestep", "hvac system timestep":
		return true
	}
	return false
}
func energyPathCogenerationEnergyUnit(unit string) bool {
	switch strings.TrimSpace(unit) {
	case "J", "kJ", "MJ", "GJ", "Wh", "kWh", "MWh":
		return true
	}
	return false
}
func energyPathCogenerationExactSource(source EnergyDataSource, roster []EnergyDataSource) bool {
	selected := 0
	for _, s := range roster {
		if s.ID == source.ID {
			selected++
			if s.SourceType != source.SourceType || s.IsMeter != source.IsMeter || s.Name != source.Name || s.KeyValue != source.KeyValue || s.SourceUnit != source.SourceUnit || s.Units != source.Units || s.ReportingFrequency != source.ReportingFrequency {
				return false
			}
		}
	}
	return selected == 1 && strings.HasPrefix(source.ID, "sql-rdd-") && strings.TrimPrefix(source.ID, "sql-rdd-") != ""
}

// Public legacy alias helpers remain unchanged. This qualifier runs while a
// series still has one actual dictionary; native meter resource controls role.
// Any relevant but unproved original/source becomes context, never produced
// fuel. A numeric NULL/absence is not represented or repaired by this function.
func qualifyEnergyPathCogenerationMeter(item energyExplanationSeries, source EnergyDataSource, roster []EnergyDataSource, inventory energyPathCogenerationInventory) (energyExplanationSeries, *energyPathCogenerationResourceBoundary) {
	itemName := firstNonEmpty(item.SourceName, item.sourceName)
	resource, sourceRelevant := energyPathCogenerationMeterResource(source.Name)
	itemResource, itemRelevant := energyPathCogenerationMeterResource(itemName)
	if (!sourceRelevant && !itemRelevant) || !inventory.HasOriginal {
		return item, nil
	}
	if !sourceRelevant {
		resource = itemResource
	}
	name := source.Name
	role, carrier := energyPathCogenerationResource(resource)
	b := &energyPathCogenerationResourceBoundary{Role: role, Resource: resource, SourceID: source.ID, Reason: "native_end_use_resource_meter"}
	count := 0
	for _, s := range roster {
		if s.SourceType == "sql_report_data" && strings.EqualFold(s.Name, name) && strings.EqualFold(s.ReportingFrequency, source.ReportingFrequency) {
			count++
		}
	}
	valid := sourceRelevant && itemRelevant && strings.EqualFold(itemName, source.Name) && strings.EqualFold(strings.TrimSpace(name), "Cogeneration:"+resource) && inventory.ReviewedVersion && !inventory.CustomMeterNames[energyPathCogenerationKey(name)] && source.SourceType == "sql_report_data" && source.IsMeter && source.KeyValue == "" && source.SourceUnit == "J" && (source.Units == "" || source.Units == "J") && energyPathCogenerationNativeFrequency(source.ReportingFrequency) && strings.EqualFold(source.ReportingFrequency, item.sourceFrequency) && len(item.SourceIDs) == 1 && item.SourceIDs[0] == source.ID && energyPathCogenerationExactSource(source, roster) && count == 1 && item.Unit == "kWh" && !item.sourceIsRate
	if !valid || role == energyPathCogenerationUnresolved {
		b.Role = energyPathCogenerationUnresolved
		b.Reason = "unproved_native_meter_identity"
	}
	return applyEnergyPathCogenerationRole(item, b, carrier), b
}

func applyEnergyPathCogenerationRole(item energyExplanationSeries, b *energyPathCogenerationResourceBoundary, carrier string) energyExplanationSeries {
	item.ZoneName = "" // The broad native input is not an observed Zone allocation.
	if carrier != "" {
		item.Carrier = carrier
	}
	item.Level = "energy"
	item.MeterHierarchyLevel = "broad_end_use"
	switch b.Role {
	case energyPathCogenerationConsumed:
		item.Stage = "end_use"
		item.Kind = "energy.cogeneration_input"
		item.EndUse = "cogeneration_input"
		item.Label = "Cogeneration input"
		item.SourceClass = "meter"
	case energyPathCogenerationProduced:
		item.Stage = "support"
		item.Kind = "energy.generators"
		item.EndUse = "generators"
		item.Label = "Produced electricity"
	case energyPathCogenerationPurchased:
		item.Stage = "support"
		item.Kind = "energy.electricity_purchased"
		item.EndUse = "electricity_purchased"
		item.Label = "Purchased electricity"
	case energyPathCogenerationSold:
		item.Stage = "support"
		item.Kind = "energy.electricity_sold"
		item.EndUse = "electricity_sold"
		item.Label = "Sold electricity"
	default:
		b.NonFlow = true
		item.Level = "support"
		item.Stage = "context"
		item.Kind = "energy.cogeneration_resource_context"
		item.EndUse = "cogeneration_resource_context"
		item.Label = "Cogeneration resource context"
		item.SourceClass = "cogeneration_resource_context"
	}
	item.CanonicalKind = item.Kind
	// Rebuild identity after the caller installs the private resource/key suffix.
	item.CanonicalFamily = ""
	item.SourceFamily = ""
	return item
}

// Raw native ABUPS identity is retained before the old Generators alias and
// support-row filter. ValueText is intentionally absent: knownness, unit
// conversion and Monthly-authority preference stay with the SQL reader.
type energyPathCogenerationTabularIdentity struct {
	SourceID, ReportName, ReportForString, TableName, RowName, ColumnName, Unit string
}

func qualifyEnergyPathCogenerationTabular(row energyPathCogenerationTabularIdentity, roster []energyPathCogenerationTabularIdentity, inventory energyPathCogenerationInventory) (energyMeterAliasDefinition, *energyPathCogenerationResourceBoundary, bool) {
	relevant := strings.EqualFold(strings.TrimSpace(row.RowName), "Generators")
	if !relevant || !inventory.HasOriginal {
		return energyMeterAliasDefinition{}, nil, false
	}
	resource, ok := energyExplanationTabularCarrier(row.ColumnName)
	role, carrier := energyPathCogenerationResource(resource)
	b := &energyPathCogenerationResourceBoundary{Role: role, Resource: resource, SourceID: row.SourceID, Reason: "native_abups_generators_consumed_input"}
	count := 0
	selected, ids := 0, 0
	for _, r := range roster {
		if r.SourceID == row.SourceID {
			ids++
			if r == row {
				selected++
			}
		}
		if strings.EqualFold(r.ReportName, row.ReportName) && strings.EqualFold(r.ReportForString, row.ReportForString) && strings.EqualFold(r.TableName, row.TableName) && strings.EqualFold(r.RowName, row.RowName) && strings.EqualFold(r.ColumnName, row.ColumnName) {
			count++
		}
	}
	valid := inventory.ReviewedVersion && ok && role == energyPathCogenerationConsumed && strings.EqualFold(row.ReportName, "AnnualBuildingUtilityPerformanceSummary") && strings.EqualFold(row.ReportForString, "Entire Facility") && strings.EqualFold(row.TableName, "End Uses") && energyPathCogenerationEnergyUnit(row.Unit) && row.SourceID != "" && selected == 1 && ids == 1 && count == 1 && !inventory.CustomMeterNames[energyPathCogenerationKey("Cogeneration:"+resource)]
	if !valid {
		b.Role = energyPathCogenerationUnresolved
		b.Reason = "unproved_native_tabular_identity"
		b.NonFlow = true
		return energyMeterAliasDefinition{}, b, true
	}
	return energyMeterAliasDefinition{Kind: "energy.cogeneration_input", Label: "Cogeneration input", Carrier: carrier, EndUse: "cogeneration_input", HierarchyLevel: "broad_end_use", Aliases: []string{"Cogeneration:" + resource}}, b, true
}

// Strict native ancillary fallback is limited to a whole-original finite
// sole-LookUpTable-inverter census. Other ancillary/standby names are not
// guessed additive from wording, and a parent budget always wins once known.
func energyPathCogenerationSoleInverterSource(source EnergyDataSource, roster []EnergyDataSource, inventory energyPathCogenerationInventory) bool {
	if !inventory.ReviewedVersion || inventory.SoleInverterKey == "" || source.SourceType != "sql_report_data" || source.IsMeter || !strings.EqualFold(source.Name, "Inverter Ancillary AC Electricity Energy") || !strings.EqualFold(source.KeyValue, inventory.SoleInverterKey) || source.SourceUnit != "J" || (source.Units != "" && source.Units != "J") || !energyPathCogenerationNativeFrequency(source.ReportingFrequency) || !energyPathCogenerationExactSource(source, roster) {
		return false
	}
	count := 0
	for _, s := range roster {
		if s.SourceType == "sql_report_data" && strings.EqualFold(s.Name, source.Name) && strings.EqualFold(s.KeyValue, source.KeyValue) && strings.EqualFold(s.ReportingFrequency, source.ReportingFrequency) {
			count++
		}
	}
	return count == 1
}

// Inputs must already have a qualified native consumed-resource identity.
// One independent period at a time; nil is absent, Known=false is unknown.
// Never infer the parent budget by adding a component to it or normalize a
// whole broad meter from one component. The caller keeps all native sources.
type energyPathCogenerationBudgetObservation struct {
	Known bool
	Value float64
}
type energyPathCogenerationBudgetChoice struct {
	UseParent, UseSoleMember bool
	Reason                   string
}

func chooseEnergyPathCogenerationBudget(parent, member *energyPathCogenerationBudgetObservation, provedSoleMember bool) energyPathCogenerationBudgetChoice {
	known := func(o *energyPathCogenerationBudgetObservation) bool {
		return o != nil && o.Known && !math.IsNaN(o.Value) && !math.IsInf(o.Value, 0) && o.Value >= 0
	}
	if parent != nil {
		if known(parent) {
			return energyPathCogenerationBudgetChoice{UseParent: true, Reason: "observed_parent_budget"}
		}
		return energyPathCogenerationBudgetChoice{Reason: "parent_budget_unknown"}
	}
	if provedSoleMember && known(member) {
		return energyPathCogenerationBudgetChoice{UseSoleMember: true, Reason: "observed_sole_native_member"}
	}
	return energyPathCogenerationBudgetChoice{Reason: "native_budget_unassigned"}
}
