package simulation

// Boundary denial only: no battery efficiency, SOC energy,
// Facility subtraction, generation sum, quantity rounding or source inference.
import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const (
	energyPathStorageChargeDCCanonicalBus = "DirectCurrentWithInverterDCStorage"
	energyPathStorageChargeNative         = "native_storage_non_consumption"
	energyPathStorageChargeUnresolved     = "unresolved_native_storage_boundary"
)

type energyPathStorageBoundaryObject struct {
	ObjectType         string `json:"objectType"`
	ObjectName         string `json:"objectName"`
	ObjectIndex        int    `json:"objectIndex"`
	objectIndexMissing bool
}

// Missing/null JSON metadata must not become a valid original object at index
// zero. Keep the malformed reference and the surrounding deny boundary; the
// validator reports the missing index, and re-serialization keeps it missing.
func (ref *energyPathStorageBoundaryObject) UnmarshalJSON(data []byte) error {
	var wire struct {
		ObjectType  string `json:"objectType"`
		ObjectName  string `json:"objectName"`
		ObjectIndex *int   `json:"objectIndex"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*ref = energyPathStorageBoundaryObject{ObjectType: wire.ObjectType, ObjectName: wire.ObjectName, objectIndexMissing: wire.ObjectIndex == nil}
	if wire.ObjectIndex != nil {
		ref.ObjectIndex = *wire.ObjectIndex
	}
	return nil
}
func (ref energyPathStorageBoundaryObject) MarshalJSON() ([]byte, error) {
	var index *int
	if !ref.objectIndexMissing {
		value := ref.ObjectIndex
		index = &value
	}
	return json.Marshal(struct {
		ObjectType  string `json:"objectType"`
		ObjectName  string `json:"objectName"`
		ObjectIndex *int   `json:"objectIndex,omitempty"`
	}{ref.ObjectType, ref.ObjectName, index})
}

// Persist this optional, explicit engineering record on the ORIGINAL charge
// node, not on canonical Other or a global electricity carrier. Its source
// references are descriptive; omitting them must never restore a flow edge.
// A malformed present record remains denial-only and raises a warning; it is
// never silently converted into nil/legacy authorization on JSON reopen.
type energyPathStorageChargeBoundary struct {
	Schema          string                            `json:"schema"`
	State           string                            `json:"state"`
	SourceKey       string                            `json:"sourceKey"`
	OriginalVersion string                            `json:"originalVersion,omitempty"`
	BusType         string                            `json:"busType"`
	Reason          string                            `json:"reason"`
	Objects         []energyPathStorageBoundaryObject `json:"objects,omitempty"`
	SourceIDs       []string                          `json:"sourceIds,omitempty"`
}

type energyPathStorageChargeInventory struct {
	HasOriginal     bool
	OriginalVersion string
	ByKey           map[string]energyPathStorageChargeBoundary
}

func energyPathStorageBoundaryKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
func energyPathStorageBoundaryField(object idf.Object, index int) string {
	if index < 0 || index >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[index].Value)
}
func energyPathStorageBoundaryRef(object idf.Object) energyPathStorageBoundaryObject {
	return energyPathStorageBoundaryObject{ObjectType: object.Type, ObjectName: energyPathStorageBoundaryField(object, 0), ObjectIndex: object.Index}
}
func energyPathStorageBoundaryObjects(doc idf.Document, prefix, name string) []idf.Object {
	var out []idf.Object
	for _, object := range doc.Objects {
		if strings.HasPrefix(energyPathStorageBoundaryKey(object.Type), energyPathStorageBoundaryKey(prefix)) && strings.EqualFold(energyPathStorageBoundaryField(object, 0), strings.TrimSpace(name)) {
			out = append(out, object)
		}
	}
	return out
}
func energyPathStorageBoundaryUnique(doc idf.Document, prefix, exactType, name string) (idf.Object, bool) {
	objects := energyPathStorageBoundaryObjects(doc, prefix, name)
	if strings.TrimSpace(name) == "" || len(objects) != 1 || !strings.EqualFold(objects[0].Type, exactType) {
		return idf.Object{}, false
	}
	return objects[0], true
}

func energyPathStorageChargeNativeType(kind string) bool {
	switch energyPathStorageBoundaryKey(kind) {
	case "electricloadcenter:storage:simple", "electricloadcenter:storage:battery", "electricloadcenter:storage:liionnmcbattery":
		return true
	}
	return false
}
func energyPathStorageChargeNativeBus(bus string) bool {
	switch energyPathStorageBoundaryKey(bus) {
	case "alternatingcurrentwithstorage", "directcurrentwithinverteracstorage", "directcurrentwithinverterdcstorage":
		return true
	}
	return false
}

// Proves a native reporting owner and its distribution association ONLY. The
// shared 25.1 registration has no PV, inverter, converter or bus-dependent
// Facility charge membership. Full electrical/thermal topology is a separate
// proof: this helper neither validates nor rejects those other relationships.
// Original field positions, not editable comments, bind every claim.
func energyPathBuildStorageChargeInventory(doc idf.Document) energyPathStorageChargeInventory {
	// Calling the builder is explicit original presence, including an empty or
	// comment-only parsed document. Only a zero inventory means no original.
	out := energyPathStorageChargeInventory{HasOriginal: true, ByKey: map[string]energyPathStorageChargeBoundary{}}
	users := map[string][]idf.Object{}
	storage := map[string][]idf.Object{}
	versions := 0
	for _, object := range doc.Objects {
		if strings.EqualFold(object.Type, "Version") {
			versions++
			out.OriginalVersion = energyPathStorageBoundaryField(object, 0)
		}
		if strings.EqualFold(object.Type, "ElectricLoadCenter:Distribution") {
			key := energyPathStorageBoundaryKey(energyPathStorageBoundaryField(object, 8))
			if key != "" {
				users[key] = append(users[key], object)
			}
		}
		if strings.HasPrefix(energyPathStorageBoundaryKey(object.Type), "electricloadcenter:storage:") && !strings.EqualFold(object.Type, "ElectricLoadCenter:Storage:Converter") {
			key := energyPathStorageBoundaryKey(energyPathStorageBoundaryField(object, 0))
			if key != "" {
				storage[key] = append(storage[key], object)
			}
		}
	}
	keys := map[string]bool{}
	for key := range storage {
		keys[key] = true
	}
	for key := range users {
		keys[key] = true
	}
	for key := range keys {
		owners, centers := storage[key], users[key]
		b := energyPathStorageChargeBoundary{Schema: "semantic-idf.storage-charge-boundary/v1", State: energyPathStorageChargeUnresolved, SourceKey: key, OriginalVersion: out.OriginalVersion, Reason: "unresolved_storage_reporting_owner"}
		for _, owner := range owners {
			b.Objects = append(b.Objects, energyPathStorageBoundaryRef(owner))
		}
		for _, center := range centers {
			b.Objects = append(b.Objects, energyPathStorageBoundaryRef(center))
		}
		if len(centers) == 1 {
			b.BusType = energyPathStorageBoundaryField(centers[0], 6)
		}
		switch {
		case versions != 1 || out.OriginalVersion != "25.1":
			b.Reason = "unreviewed_original_version"
		case len(owners) != 1 || !energyPathStorageChargeNativeType(owners[0].Type):
		case len(centers) != 1 || !energyPathStorageChargeNativeBus(b.BusType):
			b.Reason = "unresolved_storage_distribution"
		default:
			if _, ok := energyPathStorageBoundaryUnique(doc, "ElectricLoadCenter:Distribution", "ElectricLoadCenter:Distribution", energyPathStorageBoundaryField(centers[0], 0)); !ok {
				b.Reason = "unresolved_storage_distribution"
			} else {
				b.State = energyPathStorageChargeNative
				b.Reason = "native_charge_is_not_facility_consumption"
			}
		}
		b.Objects = energyPathStableStorageBoundaryObjects(b.Objects)
		out.ByKey[key] = b
	}
	return out
}

func energyPathStableStorageBoundaryObjects(input []energyPathStorageBoundaryObject) []energyPathStorageBoundaryObject {
	if len(input) == 0 {
		return nil
	}
	out := append([]energyPathStorageBoundaryObject(nil), input...)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ObjectIndex != b.ObjectIndex {
			return a.ObjectIndex < b.ObjectIndex
		}
		if a.ObjectType != b.ObjectType {
			return a.ObjectType < b.ObjectType
		}
		if a.ObjectName != b.ObjectName {
			return a.ObjectName < b.ObjectName
		}
		return !a.objectIndexMissing && b.objectIndexMissing
	})
	write := 0
	for _, object := range out {
		if write > 0 && out[write-1] == object {
			continue
		}
		out[write] = object
		write++
	}
	return out[:write]
}
func cloneEnergyPathStorageChargeBoundary(input *energyPathStorageChargeBoundary) *energyPathStorageChargeBoundary {
	if input == nil {
		return nil
	}
	out := *input
	out.Objects = append([]energyPathStorageBoundaryObject(nil), input.Objects...)
	out.SourceIDs = append([]string(nil), input.SourceIDs...)
	return &out
}
func unionEnergyPathStorageChargeBoundaries(left, right []energyPathStorageChargeBoundary) []energyPathStorageChargeBoundary {
	if len(left)+len(right) == 0 {
		return nil
	}
	byKey := map[string]energyPathStorageChargeBoundary{}
	add := func(input energyPathStorageChargeBoundary) {
		identity, _ := json.Marshal([]string{input.Schema, input.State, input.SourceKey, input.OriginalVersion, input.BusType, input.Reason})
		key := string(identity)
		out, exists := byKey[key]
		if !exists {
			out = *cloneEnergyPathStorageChargeBoundary(&input)
		} else {
			out.Objects = append(out.Objects, input.Objects...)
			out.SourceIDs = appendUniqueStrings(out.SourceIDs, input.SourceIDs...)
		}
		out.Objects = energyPathStableStorageBoundaryObjects(out.Objects)
		out.SourceIDs = appendUniqueStrings(nil, out.SourceIDs...)
		sort.Strings(out.SourceIDs)
		byKey[key] = out
	}
	for _, b := range left {
		add(b)
	}
	for _, b := range right {
		add(b)
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]energyPathStorageChargeBoundary, 0, len(keys))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}
func energyPathStorageChargeBoundariesAreNonFlow(boundaries []energyPathStorageChargeBoundary) bool {
	return len(boundaries) > 0
}
func validateEnergyPathStorageChargeBoundary(input *energyPathStorageChargeBoundary) error {
	if input == nil {
		return nil
	}
	if input.Schema != "semantic-idf.storage-charge-boundary/v1" || strings.TrimSpace(input.SourceKey) == "" {
		return fmt.Errorf("incomplete explicit storage boundary")
	}
	if input.State != energyPathStorageChargeNative && input.State != energyPathStorageChargeUnresolved {
		return fmt.Errorf("unknown explicit storage boundary state")
	}
	center, storage := 0, 0
	indices := map[int]bool{}
	for _, ref := range input.Objects {
		if ref.objectIndexMissing || ref.ObjectIndex < 0 || strings.TrimSpace(ref.ObjectName) == "" {
			return fmt.Errorf("invalid or missing original storage boundary reference")
		}
		if indices[ref.ObjectIndex] {
			return fmt.Errorf("duplicate original storage boundary object index")
		}
		indices[ref.ObjectIndex] = true
		switch {
		case strings.EqualFold(ref.ObjectType, "ElectricLoadCenter:Distribution"):
			center++
		case energyPathStorageChargeNativeType(ref.ObjectType):
			storage++
			if input.State == energyPathStorageChargeNative && !strings.EqualFold(ref.ObjectName, input.SourceKey) {
				return fmt.Errorf("native reporting owner differs from source key")
			}
		case input.State == energyPathStorageChargeUnresolved && strings.HasPrefix(energyPathStorageBoundaryKey(ref.ObjectType), "electricloadcenter:storage:"):
		default:
			return fmt.Errorf("unreviewed original storage boundary reference")
		}
	}
	if input.State == energyPathStorageChargeNative {
		if input.OriginalVersion != "25.1" || !energyPathStorageChargeNativeBus(input.BusType) || center != 1 || storage != 1 || len(input.Objects) != 2 || input.Reason != "native_charge_is_not_facility_consumption" {
			return fmt.Errorf("explicit native storage proof is incomplete")
		}
	} else {
		switch input.Reason {
		case "unreviewed_original_version", "unresolved_storage_reporting_owner", "unresolved_storage_distribution", "unbound_source_key_in_original_model", "unproved_native_charge_reporting_identity":
		default:
			return fmt.Errorf("unknown unresolved native storage reason")
		}
	}
	return nil
}

func energyPathStorageChargeBoundaryForKey(inventory energyPathStorageChargeInventory, key string) *energyPathStorageChargeBoundary {
	if !inventory.HasOriginal {
		return nil
	} // Legacy/no-original compatibility is NOT positive native proof.
	if boundary, ok := inventory.ByKey[energyPathStorageBoundaryKey(key)]; ok {
		return cloneEnergyPathStorageChargeBoundary(&boundary)
	}
	return &energyPathStorageChargeBoundary{Schema: "semantic-idf.storage-charge-boundary/v1", State: energyPathStorageChargeUnresolved, SourceKey: strings.TrimSpace(key), OriginalVersion: inventory.OriginalVersion, Reason: "unbound_source_key_in_original_model"}
}
func energyPathIsNativeStorageChargeSeries(item energyExplanationSeries) bool {
	return strings.EqualFold(strings.TrimSpace(firstNonEmpty(item.SourceName, item.sourceName)), "Electric Storage Charge Energy")
}

// Checks metadata only, not observed zero/NULL/value presence. Native Charge
// Energy is a System/Sum/J Output:Variable; the independent capture must also
// verify the dictionary's native Type/TimestepType. The normalized series and
// selected source records available here cannot prove those two raw columns.
// Each pre-preference series must still represent exactly one dictionary.
func energyPathStorageChargeReportingMatches(item energyExplanationSeries, sources []EnergyDataSource) bool {
	if !energyPathIsNativeStorageChargeSeries(item) || !strings.EqualFold(item.Carrier, "electricity") || !strings.EqualFold(item.EndUse, "storage_charge") || item.Unit != "kWh" || item.sourceIsRate || len(item.SourceIDs) != 1 {
		return false
	}
	var selected *EnergyDataSource
	for i := range sources {
		if sources[i].ID == item.SourceIDs[0] {
			if selected != nil {
				return false
			}
			selected = &sources[i]
		}
	}
	if selected == nil || selected.SourceType != "sql_report_data" || selected.IsMeter || !strings.HasPrefix(selected.ID, "sql-rdd-") || strings.TrimPrefix(selected.ID, "sql-rdd-") == "" || !strings.EqualFold(selected.Name, "Electric Storage Charge Energy") || !strings.EqualFold(selected.KeyValue, firstNonEmpty(item.SourceKey, item.sourceKeyValue)) || strings.TrimSpace(selected.KeyValue) == "" || selected.SourceUnit != "J" || !strings.EqualFold(selected.ReportingFrequency, item.sourceFrequency) {
		return false
	}
	if selected.Units != "" && selected.Units != "J" {
		return false
	}
	identities := 0
	for _, source := range sources {
		if source.SourceType == "sql_report_data" && strings.EqualFold(source.Name, selected.Name) && strings.EqualFold(source.KeyValue, selected.KeyValue) && strings.EqualFold(source.ReportingFrequency, selected.ReportingFrequency) {
			identities++
		}
	}
	if identities != 1 {
		return false
	}
	switch energyPathStorageBoundaryKey(selected.ReportingFrequency) {
	case "monthly", "hourly", "daily", "run period", "annual", "timestep", "zone timestep", "hvac system timestep":
		return true
	}
	return false
}

// Apply BEFORE preferredEnergyExplanationSeries and accounting. The returned
// metadata must be carried alongside the series; scalar/maps are untouched.
func qualifyEnergyPathStorageChargeSeries(item energyExplanationSeries, inventory energyPathStorageChargeInventory, sources []EnergyDataSource) (energyExplanationSeries, *energyPathStorageChargeBoundary) {
	if !energyPathIsNativeStorageChargeSeries(item) {
		return item, nil
	}
	boundary := energyPathStorageChargeBoundaryForKey(inventory, firstNonEmpty(item.SourceKey, item.sourceKeyValue))
	if boundary == nil {
		return item, nil
	}
	if boundary.State == energyPathStorageChargeNative && !energyPathStorageChargeReportingMatches(item, sources) {
		boundary.State = energyPathStorageChargeUnresolved
		boundary.Reason = "unproved_native_charge_reporting_identity"
	}
	if boundary.State == energyPathStorageChargeNative && validateEnergyPathStorageChargeBoundary(boundary) != nil {
		boundary.State = energyPathStorageChargeUnresolved
		boundary.Reason = "unresolved_storage_reporting_owner"
	}
	boundary.SourceIDs = appendUniqueStrings(nil, item.SourceIDs...)
	item.Stage = "support"
	// The v1 node builder must consume this level; it currently hardcodes energy.
	item.Level = "support"
	item.SourceClass = "storage_charge_context" // Never label an unmetered transfer "meter".
	item.ZoneName = ""                          // Native transfer is not an owned Zone thermal load.
	return item, boundary
}

// Append to both canonical family AND physical preference keys only when the
// original model is available. This includes all AC/DC and unresolved keys, so
// one constituent cannot steal/absorb a different storage reporting identity.
func energyPathStorageChargeSelectionSuffix(item energyExplanationSeries, inventory energyPathStorageChargeInventory) string {
	if !inventory.HasOriginal || !energyPathIsNativeStorageChargeSeries(item) {
		return ""
	}
	return "|storage_charge_key|" + energyPathStorageBoundaryKey(firstNonEmpty(item.SourceKey, item.sourceKeyValue))
}
func energyPathStorageChargeNodeID(item energyExplanationSeries, boundary *energyPathStorageChargeBoundary) string {
	if boundary == nil {
		return energyExplanationEnergyNodeID(item)
	}
	// Hex encoding avoids lossy slug collisions (A-B versus A_B). V2 may merge these
	// into its existing support.storage_charge.building context-only node.
	encoded := hex.EncodeToString([]byte(energyPathStorageBoundaryKey(boundary.SourceKey)))
	return "energy.support.storage_charge.electricity." + encoded
}
func energyPathStorageChargeIsNonFlow(boundary *energyPathStorageChargeBoundary) bool {
	return boundary != nil
}

// Reload defense: an explicit boundary always denies both consumption and
// supply. Validation failure becomes a diagnostic, never authorization. No
// generic source-ID intersection is used, so omitted IDs cannot bypass denial.
func filterEnergyPathStorageChargeLegacyEdges(input []EnergyExplanationEdge, boundaries map[string]*energyPathStorageChargeBoundary) []EnergyExplanationEdge {
	if len(boundaries) == 0 || input == nil {
		return input
	}
	out := make([]EnergyExplanationEdge, 0, len(input))
	for _, edge := range input {
		if energyPathStorageChargeIsNonFlow(boundaries[edge.FromID]) || energyPathStorageChargeIsNonFlow(boundaries[edge.ToID]) {
			continue
		}
		out = append(out, edge)
	}
	return out
}
func filterEnergyPathStorageChargeLinks(input []EnergyPathLink, boundaries map[string]*energyPathStorageChargeBoundary) []EnergyPathLink {
	if len(boundaries) == 0 || input == nil {
		return input
	}
	out := make([]EnergyPathLink, 0, len(input))
	for _, link := range input {
		if energyPathStorageChargeIsNonFlow(boundaries[link.FromID]) || energyPathStorageChargeIsNonFlow(boundaries[link.ToID]) {
			continue
		}
		out = append(out, link)
	}
	return out
}
