package simulation

import (
	"fmt"
	"reflect"
	"strings"
)

const epathSQLZeroPressureFamilyPrefix = "zero_pressure_fallback:"

// An explicit reviewed exceptional cell is not permission to treat missing
// observations or a small positive matching pressure as zero.
type epathRealSQLZeroPressureFallback struct {
	ZoneName string `json:"zoneName"`
	Month    int    `json:"month"`
	Service  string `json:"service"`
}

func epathSQLZeroPressureDeclarations(frames epathSQLFrames, model epathRealSQLModel) (map[string]epathRealSQLZeroPressureFallback, error) {
	out := map[string]epathRealSQLZeroPressureFallback{}
	for _, family := range model.Families {
		if strings.HasPrefix(family.ID, epathSQLZeroPressureFamilyPrefix) {
			return nil, fmt.Errorf("physical family cannot impersonate a zero-pressure fallback")
		}
	}
	for _, declaration := range model.ZeroPressureFallbacks {
		zone := strings.ToLower(declaration.ZoneName)
		if declaration.ZoneName == "" || strings.TrimSpace(declaration.ZoneName) != declaration.ZoneName || frames.Zones[zone].Name == "" ||
			declaration.Month < 1 || declaration.Month > 12 || declaration.Service != "cooling" && declaration.Service != "heating" {
			return nil, fmt.Errorf("zero-pressure fallback requires an exact observed Zone/month/service")
		}
		key := epathSQLKey(zone, declaration.Service, declaration.Month)
		if _, duplicate := out[key]; duplicate {
			return nil, fmt.Errorf("duplicate zero-pressure fallback declaration %s", key)
		}
		out[key] = declaration
	}
	return out, nil
}

func epathSQLZeroPressureCellKey(declaration epathRealSQLZeroPressureFallback) string {
	return epathSQLKey(declaration.ZoneName, epathSQLZeroPressureFamilyPrefix+declaration.Service, declaration.Month)
}

// Preserve the complete SQL-derived physical roster before any allocation.
// Neither this snapshot nor its quantities come from the candidate graph.
func epathSQLRecordZeroPressurePhysicalCells(frames *epathSQLFrames) {
	frames.ZeroPressurePhysicalCells = map[string]epathSQLCell{}
	for _, declaration := range frames.ZeroPressureFallbacks {
		for key, cell := range frames.Cells {
			if cell == nil || !strings.EqualFold(cell.Zone, declaration.ZoneName) || cell.Month != declaration.Month {
				continue
			}
			copy := *cell
			copy.Allocated = nil
			copy.SourceIDs = append([]int(nil), cell.SourceIDs...)
			if cell.Raw.Bounds != nil {
				bounds := *cell.Raw.Bounds
				copy.Raw.Bounds = &bounds
			}
			if cell.Effective.Bounds != nil {
				bounds := *cell.Effective.Bounds
				copy.Effective.Bounds = &bounds
			}
			frames.ZeroPressurePhysicalCells[key] = copy
		}
	}
}

func epathSQLZeroPressureObservedMonth(frames epathSQLFrames, model epathRealSQLModel, id, month int) bool {
	source, exists := frames.SourceIdentities[id]
	if !exists || len(frames.SourceRaw[id]) != 12 || len(frames.SourceEffective[id]) != 12 {
		return false
	}
	values, err := epathSQLMonthly(source, model.Precision)
	return err == nil && reflect.DeepEqual(values[month-1], frames.SourceRaw[id][month-1]) && frames.SourceEffective[id][month-1].valid()
}

func epathSQLZeroPressureInputs(frames epathSQLFrames, model epathRealSQLModel, declaration epathRealSQLZeroPressureFallback) (epathSQLQuantity, []int, []*epathSQLCell, error) {
	fail := func(reason string) (epathSQLQuantity, []int, []*epathSQLCell, error) {
		return epathSQLQuantity{}, nil, nil, fmt.Errorf("zero-pressure %s/M%d/%s: %s", declaration.ZoneName, declaration.Month, declaration.Service, reason)
	}
	zone, month, service := strings.ToLower(declaration.ZoneName), declaration.Month, declaration.Service
	if frames.Zones[zone].Name == "" || month < 1 || month > 12 || service != "cooling" && service != "heating" {
		return fail("invalid exact declaration")
	}
	loadKey := epathSQLKey(zone, service, month)
	load, observed := frames.Loads[loadKey]
	if !observed || !load.valid() || load.Value <= 0 {
		return fail("fallback requires a positive original delivered load, not missing/zero")
	}
	loadIDs := frames.LoadSourceIDs[loadKey]
	if len(loadIDs) != 1 {
		return fail("fallback requires one exact original Zone sensible-load identity")
	}
	loadID := loadIDs[0]
	loadSource, known := frames.SourceIdentities[loadID]
	if !known || loadID <= 0 || loadSource.DictionaryIndex != loadID || loadSource.IsMeter || loadSource.ReportingFrequency != "Monthly" || !strings.EqualFold(loadSource.KeyValue, zone) || !strings.EqualFold(frames.SourceZone[loadID], zone) {
		return fail("invalid original load identity/owner/frequency")
	}
	declaredLoad := 0
	for _, definition := range model.Loads {
		if definition.Service != service || definition.Component != "sensible" || definition.Source.IsMeter {
			continue
		}
		owner := false
		for _, name := range definition.Source.Keys {
			owner = owner || name == "*" || strings.EqualFold(name, zone)
		}
		if !owner {
			continue
		}
		for _, alternative := range definition.Source.Alternatives {
			if strings.EqualFold(alternative.Name, loadSource.Name) && alternative.Unit == loadSource.SourceUnit {
				declaredLoad++
			}
		}
	}
	if declaredLoad != 1 || !epathSQLZeroPressureObservedMonth(frames, model, loadID, month) ||
		frames.SourceRaw[loadID][month-1].Value*frames.Zones[zone].Multiplier != load.Value || frames.SourceEffective[loadID][month-1].Value != load.Value {
		return fail("load is not its independently observed original monthly quantity")
	}
	roles := map[string]string{}
	for _, family := range model.Families {
		roles[family.ID] = family.Role
		for _, owner := range family.Keys {
			if strings.EqualFold(owner, zone) && frames.Cells[epathSQLKey(zone, family.ID, month)] == nil {
				return fail("declared physical family has a missing month")
			}
		}
	}
	var physical []*epathSQLCell
	rosterCount := 0
	for key, original := range frames.ZeroPressurePhysicalCells {
		if original.Zone != zone || original.Month != month {
			continue
		}
		rosterCount++
		cell := frames.Cells[key]
		if cell == nil {
			return fail("original physical roster has a missing cell")
		}
		copy := *cell
		copy.Allocated = nil
		if !reflect.DeepEqual(copy, original) {
			return fail("original physical identity, signed quantity, or source roster changed")
		}
	}
	if rosterCount == 0 {
		return fail("missing original physical roster")
	}
	for key, cell := range frames.Cells {
		if cell == nil {
			return fail("nil physical cell")
		}
		if cell.Zone != zone || cell.Month != month || cell.ZeroPressureFallback {
			continue
		}
		if _, original := frames.ZeroPressurePhysicalCells[key]; !original {
			return fail("undeclared extra physical pressure cell")
		}
		if key != epathSQLKey(cell.Zone, cell.Family, cell.Month) || strings.HasPrefix(cell.Family, epathSQLZeroPressureFamilyPrefix) || !cell.Raw.valid() || !cell.Effective.valid() ||
			cell.Effective.Value != cell.Raw.Value*frames.Zones[zone].Multiplier || len(cell.SourceIDs) == 0 {
			return fail("physical pressure/context lacks its complete observed signed quantity")
		}
		seen := map[int]bool{}
		for _, id := range cell.SourceIDs {
			source, ok := frames.SourceIdentities[id]
			if id <= 0 || seen[id] || !ok || source.DictionaryIndex != id || source.Name == "" || source.IsMeter || source.ReportingFrequency != "Monthly" ||
				(source.SourceUnit != "J" && source.SourceUnit != "W" && source.SourceUnit != "kWh") || !strings.EqualFold(frames.SourceZone[id], zone) ||
				!epathSQLZeroPressureObservedMonth(frames, model, id, month) {
				return fail("physical source is missing, nonfinite, duplicated, or belongs to another owner")
			}
			seen[id] = true
		}
		role := roles[cell.Family]
		if strings.HasPrefix(cell.Family, "surface:") {
			role = "pressure"
		}
		if role != "pressure" && role != "context" {
			return fail("undeclared physical family")
		}
		if role == "context" {
			continue
		}
		// Use the original signed scalar, not a rounded value or its error
		// budget. Even a tiny actual matching pressure prohibits this fallback.
		if service == "cooling" && cell.Raw.Value > 0 || service == "heating" && cell.Raw.Value < 0 {
			return fail("observed matching pressure is positive; declaration is unnecessary")
		}
		physical = append(physical, cell)
	}
	if len(physical) == 0 {
		return fail("an empty physical roster is not observed zero pressure")
	}
	return load, append([]int(nil), loadIDs...), physical, nil
}

func epathSQLApplyZeroPressureFallback(frames *epathSQLFrames, model epathRealSQLModel, declaration epathRealSQLZeroPressureFallback) error {
	load, ids, physical, err := epathSQLZeroPressureInputs(*frames, model, declaration)
	if err != nil {
		return err
	}
	key := epathSQLZeroPressureCellKey(declaration)
	if frames.Cells[key] != nil {
		return fmt.Errorf("duplicate/replaced zero-pressure synthetic allocation")
	}
	for _, cell := range physical {
		cell.Allocated[declaration.Service] = epathSQLQuantity{}
	}
	frames.Cells[key] = &epathSQLCell{
		Zone: strings.ToLower(declaration.ZoneName), Month: declaration.Month,
		Family: epathSQLZeroPressureFamilyPrefix + declaration.Service, Category: "balance.storage_other", Component: "combined",
		Raw: epathSQLQuantity{}, Effective: epathSQLQuantity{}, BuildingVisible: true, ZeroPressureFallback: true,
		Allocated: map[string]epathSQLQuantity{"cooling": {}, "heating": {}}, SourceIDs: ids,
	}
	frames.Cells[key].Allocated[declaration.Service] = load
	return nil
}

// Every downstream compiler reuses these cells only after this check. A
// synthetic marker cannot turn a physical family/source into balancing energy.
func epathSQLValidateZeroPressureFrames(frames epathSQLFrames, model epathRealSQLModel) error {
	declarations, err := epathSQLZeroPressureDeclarations(frames, model)
	if err != nil {
		return err
	}
	if len(declarations) != len(frames.ZeroPressureFallbacks) {
		return fmt.Errorf("zero-pressure frame/declaration roster differs")
	}
	for key, declaration := range declarations {
		if frames.ZeroPressureFallbacks[key] != declaration {
			return fmt.Errorf("zero-pressure declaration lost its exact monthly binding")
		}
		load, ids, physical, err := epathSQLZeroPressureInputs(frames, model, declaration)
		if err != nil {
			return err
		}
		cell := frames.Cells[epathSQLZeroPressureCellKey(declaration)]
		if cell == nil || !cell.ZeroPressureFallback || cell.Family != epathSQLZeroPressureFamilyPrefix+declaration.Service || cell.Zone != strings.ToLower(declaration.ZoneName) ||
			cell.Month != declaration.Month || cell.Category != "balance.storage_other" || cell.Component != "combined" || !cell.BuildingVisible ||
			!reflect.DeepEqual(cell.SourceIDs, ids) || len(cell.Allocated) != 2 || !epathSQLExactZero(cell.Raw) || !epathSQLExactZero(cell.Effective) {
			return fmt.Errorf("zero-pressure synthetic cell lost its load-only identity or exact raw zero")
		}
		for _, service := range []string{"cooling", "heating"} {
			value, exists := cell.Allocated[service]
			if !exists || service == declaration.Service && !reflect.DeepEqual(value, load) || service != declaration.Service && !epathSQLExactZero(value) {
				return fmt.Errorf("zero-pressure synthetic allocation changed the observed load")
			}
		}
		for _, original := range physical {
			value, exists := original.Allocated[declaration.Service]
			if !exists || !epathSQLExactZero(value) {
				return fmt.Errorf("zero-pressure physical family retained an allocated contribution")
			}
		}
	}
	for key, cell := range frames.Cells {
		if cell == nil || !cell.ZeroPressureFallback && !strings.HasPrefix(cell.Family, epathSQLZeroPressureFamilyPrefix) {
			continue
		}
		bound := false
		for _, declaration := range declarations {
			bound = bound || key == epathSQLZeroPressureCellKey(declaration)
		}
		if !bound || !cell.ZeroPressureFallback {
			return fmt.Errorf("undeclared/duplicated synthetic zero-pressure cell")
		}
	}
	return nil
}

func epathSQLExactZero(q epathSQLQuantity) bool {
	low, high := q.bounds()
	return q.valid() && q.Value == 0 && low == 0 && high == 0
}

// Generic rounded quantities keep their existing comparison slack. A purely
// load-only fallback has stronger independently established exact-zero raw
// scalars; the exception is never applied to mixed monthly/annual categories.
func epathSQLCheckZeroPressureDriver(node EnergyExplanationNode, proof *epathSQLDriverLinkProof) error {
	if !proof.PureZeroPressureFallback {
		return nil
	}
	if proof.Category != "balance.storage_other" || node.DriverCategory != proof.Category || node.ThermalComponent != "combined" ||
		node.RawValue != 0 || node.EffectiveValue != 0 || node.SignedValue != 0 ||
		node.inspectorDecodedFromJSON && node.inspectorValuePresence&3 != 3 || !node.inspectorDecodedFromJSON && !node.AllocationApplied {
		return fmt.Errorf("pure zero-pressure fallback requires present exact-zero raw/effective/signed scalars and combined identity")
	}
	return nil
}
