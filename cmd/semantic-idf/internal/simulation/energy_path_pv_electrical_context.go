package simulation

// Native reporting identities and add-missing requests.
// No SQL reader, full electrical topology, allocation, or supply arithmetic.
import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type energyPathPVElectricalDefinition struct {
	// OwnerType is the reporting-family anchor, not an individual target's
	// physical type. The four storage definitions share the exact three native
	// storage types through matchesOwnerType; OriginalOwners retains actual type.
	ID, OwnerType, Name, Role, ParentMeter, NativeEndUse, SignPolicy string
	IsMeter                                                          bool
}

// ParentMeter indicates a native constituent, not permission to add it again
// to an already observed meter. No thermal/transfer definition has an HVAC
// service, Zone owner, derived efficiency, or inferred SOC-energy quantity.
func energyPathPVElectricalDefinitions() []energyPathPVElectricalDefinition {
	return []energyPathPVElectricalDefinition{
		{ID: "pv.dc", OwnerType: "Generator:Photovoltaic", Name: "Generator Produced DC Electricity Energy", Role: "produced_constituent", ParentMeter: "ElectricityProduced:Facility", NativeEndUse: "Photovoltaic", SignPolicy: "nonnegative"},
		{ID: "inverter.dc_input", OwnerType: "ElectricLoadCenter:Inverter:LookUpTable", Name: "Inverter DC Input Electricity Energy", Role: "electrical_transfer_context", SignPolicy: "nonnegative"},
		{ID: "inverter.ac_output", OwnerType: "ElectricLoadCenter:Inverter:LookUpTable", Name: "Inverter AC Output Electricity Energy", Role: "electrical_transfer_context", SignPolicy: "nonnegative"},
		{ID: "inverter.conversion_loss", OwnerType: "ElectricLoadCenter:Inverter:LookUpTable", Name: "Inverter Conversion Loss Energy", Role: "electrical_loss_context", SignPolicy: "nonnegative"},
		{ID: "inverter.loss_decrement", OwnerType: "ElectricLoadCenter:Inverter:LookUpTable", Name: "Inverter Conversion Loss Decrement Energy", Role: "produced_decrement", ParentMeter: "ElectricityProduced:Facility", NativeEndUse: "PowerConversion", SignPolicy: "nonpositive"},
		{ID: "inverter.ancillary_ac", OwnerType: "ElectricLoadCenter:Inverter:LookUpTable", Name: "Inverter Ancillary AC Electricity Energy", Role: "purchased_constituent", ParentMeter: "Electricity:Facility", NativeEndUse: "Cogeneration", SignPolicy: "nonnegative"},
		{ID: "storage.charge", OwnerType: "ElectricLoadCenter:Storage:Battery", Name: "Electric Storage Charge Energy", Role: "storage_transfer_context", SignPolicy: "nonnegative"},
		{ID: "storage.production_decrement", OwnerType: "ElectricLoadCenter:Storage:Battery", Name: "Electric Storage Production Decrement Energy", Role: "produced_decrement", ParentMeter: "ElectricityProduced:Facility", NativeEndUse: "ElectricStorage", SignPolicy: "nonpositive"},
		{ID: "storage.discharge", OwnerType: "ElectricLoadCenter:Storage:Battery", Name: "Electric Storage Discharge Energy", Role: "produced_constituent", ParentMeter: "ElectricityProduced:Facility", NativeEndUse: "ElectricStorage", SignPolicy: "nonnegative"},
		{ID: "storage.thermal_loss", OwnerType: "ElectricLoadCenter:Storage:Battery", Name: "Electric Storage Thermal Loss Energy", Role: "thermal_context", SignPolicy: "signed"},
		{ID: "load_center.generation", OwnerType: "ElectricLoadCenter:Distribution", Name: "Electric Load Center Produced Electricity Energy", Role: "generation_subtotal_context", SignPolicy: "nonnegative"},
		{ID: "load_center.thermal", OwnerType: "ElectricLoadCenter:Distribution", Name: "Electric Load Center Produced Thermal Energy", Role: "thermal_context", SignPolicy: "signed"},
		{ID: "facility.demand", Name: "Electricity:Facility", Role: "facility_demand", SignPolicy: "nonnegative", IsMeter: true},
		{ID: "facility.produced", Name: "ElectricityProduced:Facility", Role: "net_produced", SignPolicy: "signed", IsMeter: true},
		{ID: "facility.purchased", Name: "ElectricityPurchased:Facility", Role: "purchased_supply", SignPolicy: "nonnegative", IsMeter: true},
		{ID: "facility.sold", Name: "ElectricitySurplusSold:Facility", Role: "sold_supply", SignPolicy: "nonnegative", IsMeter: true},
	}
}

type energyPathPVElectricalTarget struct {
	Definition     energyPathPVElectricalDefinition
	Key            string             // Empty for meters; physical original owner name for variables.
	OriginalOwners []idf.ComponentRef // Ambiguous candidates remain visible.
	IdentityValid  bool
	Reason         string
}

type energyPathPVElectricalInventory struct {
	HasOriginal, SchemaReviewed bool
	OriginalVersion             string
	Targets                     []energyPathPVElectricalTarget
	UnreviewedOwners            []idf.ComponentRef
	OriginalOutputs             []energyPathPVElectricalOriginalOutput // Raw census, NEVER signature-deduplicated.
}

type energyPathPVElectricalOriginalOutput struct {
	ObjectType, KeyValue, Name, Frequency, Schedule string
	ObjectIndex                                     int
	NativeShapeValid                                bool
}

func energyPathPVElectricalReadOriginalOutput(object idf.Object) (energyPathPVElectricalOriginalOutput, bool) {
	out := energyPathPVElectricalOriginalOutput{ObjectType: object.Type, ObjectIndex: object.Index}
	switch energyPathPVElectricalKey(object.Type) {
	case "output:variable":
		out.KeyValue, out.Name, out.Frequency, out.Schedule = energyPathPVElectricalField(object, 0), energyPathPVElectricalField(object, 1), energyPathPVElectricalField(object, 2), energyPathPVElectricalField(object, 3)
		out.NativeShapeValid = len(object.Fields) >= 2 && len(object.Fields) <= 4
	case "output:meter":
		out.Name, out.Frequency = energyPathPVElectricalField(object, 0), energyPathPVElectricalField(object, 1)
		out.NativeShapeValid = len(object.Fields) >= 1 && len(object.Fields) <= 2
	default:
		return out, false
	}
	return out, true
}

func (output energyPathPVElectricalOriginalOutput) request() PurposeOutputObject {
	index := output.ObjectIndex
	out := PurposeOutputObject{ObjectType: output.ObjectType, ObjectIndex: &index, State: PurposeOutputStateExisting,
		Fields: []idf.OutputFieldValue{{Name: "Key Value", Value: output.KeyValue}, {Name: "Variable Name", Value: output.Name}, {Name: "Reporting Frequency", Value: output.Frequency}, {Name: "Schedule Name", Value: output.Schedule}}}
	if strings.EqualFold(output.ObjectType, "Output:Meter") {
		out.Fields = []idf.OutputFieldValue{{Name: "Key Name", Value: output.Name}, {Name: "Reporting Frequency", Value: output.Frequency}}
	}
	return out
}

func (definition energyPathPVElectricalDefinition) matchesOwnerType(kind string) bool {
	if strings.EqualFold(definition.OwnerType, "ElectricLoadCenter:Storage:Battery") {
		switch energyPathPVElectricalKey(kind) {
		case "electricloadcenter:storage:simple", "electricloadcenter:storage:battery", "electricloadcenter:storage:liionnmcbattery":
			return true
		}
		return false
	}
	return !definition.IsMeter && strings.EqualFold(definition.OwnerType, kind)
}

func energyPathPVElectricalKey(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func energyPathPVElectricalField(object idf.Object, field int) string {
	if field < 0 || field >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[field].Value)
}
func energyPathPVElectricalRef(object idf.Object) idf.ComponentRef {
	return idf.ComponentRef{ID: fmt.Sprintf("component:%d", object.Index), ObjectType: object.Type, ObjectName: energyPathPVElectricalField(object, 0), ObjectIndex: object.Index}
}

// Finite native reporting namespaces. PVWatts
// emits the same exact DC name. All four native inverter models share output
// registration; the three native storage models likewise share these names.
// Only the three storage types additionally share positive reporting support
// for the four common energies; PV/inverter collision peers remain unreviewed.
// Unrelated Generator/FuelCell/Converter names are not fabricated collisions.
func energyPathPVElectricalReportingPeer(ownerType, peerType string) bool {
	peer := energyPathPVElectricalKey(peerType)
	switch energyPathPVElectricalKey(ownerType) {
	case "generator:photovoltaic":
		return peer == "generator:photovoltaic" || peer == "generator:pvwatts"
	case "electricloadcenter:inverter:lookuptable":
		return peer == "electricloadcenter:inverter:lookuptable" || peer == "electricloadcenter:inverter:simple" || peer == "electricloadcenter:inverter:functionofpower" || peer == "electricloadcenter:inverter:pvwatts"
	case "electricloadcenter:storage:battery":
		return peer == "electricloadcenter:storage:battery" || peer == "electricloadcenter:storage:simple" || peer == "electricloadcenter:storage:liionnmcbattery"
	case "electricloadcenter:distribution":
		return peer == "electricloadcenter:distribution"
	}
	return false
}

// Calling this with a document means an original WAS supplied, even if empty.
// No-original callers retain the zero inventory without invoking this helper.
// The identities intentionally survive disconnected surfaces/lists/inverters,
// missing schedules and bad allocation routes: those are distinct proofs and
// actual absent SQL remains absent. No claim of engine-valid input is made.
func energyPathBuildPVElectricalInventory(doc idf.Document) energyPathPVElectricalInventory {
	out := energyPathPVElectricalInventory{HasOriginal: true}
	versions := 0
	indices := map[int]int{}
	reviewedTypes := map[string]bool{}
	for _, definition := range energyPathPVElectricalDefinitions() {
		if !definition.IsMeter {
			reviewedTypes[energyPathPVElectricalKey(definition.OwnerType)] = true
		}
	}
	reviewedTypes["electricloadcenter:storage:simple"], reviewedTypes["electricloadcenter:storage:liionnmcbattery"] = true, true
	hasReviewedOwner := false
	for _, object := range doc.Objects {
		hasReviewedOwner = hasReviewedOwner || reviewedTypes[energyPathPVElectricalKey(object.Type)]
		indices[object.Index]++
		if output, ok := energyPathPVElectricalReadOriginalOutput(object); ok {
			out.OriginalOutputs = append(out.OriginalOutputs, output)
		}
		if strings.EqualFold(object.Type, "Version") {
			versions++
			out.OriginalVersion = energyPathPVElectricalField(object, 0)
		}
		if !reviewedTypes[energyPathPVElectricalKey(object.Type)] {
			for kind := range reviewedTypes {
				if energyPathPVElectricalReportingPeer(kind, object.Type) {
					out.UnreviewedOwners = append(out.UnreviewedOwners, energyPathPVElectricalRef(object))
					break
				}
			}
		}
	}
	out.SchemaReviewed = versions == 1 && out.OriginalVersion == "25.1"
	if !hasReviewedOwner {
		return out // Original requests and unreviewed-owner evidence still survive.
	}
	for _, definition := range energyPathPVElectricalDefinitions() {
		if definition.IsMeter {
			continue
		}
		seen := map[string]bool{}
		for _, owner := range doc.Objects {
			if !definition.matchesOwnerType(owner.Type) {
				continue
			}
			name := energyPathPVElectricalField(owner, 0)
			key := energyPathPVElectricalKey(name)
			if seen[key] {
				continue
			}
			seen[key] = true
			target := energyPathPVElectricalTarget{Definition: definition, Key: name, Reason: "unresolved_reporting_owner"}
			for _, peer := range doc.Objects {
				if energyPathPVElectricalReportingPeer(definition.OwnerType, peer.Type) && energyPathPVElectricalKey(energyPathPVElectricalField(peer, 0)) == key {
					target.OriginalOwners = append(target.OriginalOwners, energyPathPVElectricalRef(peer))
				}
			}
			target.IdentityValid = out.SchemaReviewed && name != "" && len(target.OriginalOwners) == 1 && owner.Index >= 0 && indices[owner.Index] == 1
			if !out.SchemaReviewed {
				target.Reason = "unreviewed_original_version"
			} else if target.IdentityValid {
				target.Reason = "unique_native_reporting_owner"
			}
			out.Targets = append(out.Targets, target)
		}
	}
	if len(out.Targets) > 0 {
		for _, definition := range energyPathPVElectricalDefinitions() {
			if definition.IsMeter {
				out.Targets = append(out.Targets, energyPathPVElectricalTarget{Definition: definition, IdentityValid: out.SchemaReviewed, Reason: "native_global_meter_intent_not_observation"})
			}
		}
	}
	return out
}

// These flags are metadata for a future native reader; do not feed this record
// to a generic Zone multiplier or mark any unqueried SQL scalar as known zero.
func (target energyPathPVElectricalTarget) sourceBasis() (string, float64, string) {
	return "model_total", 1, "already_model_total"
}
func (definition energyPathPVElectricalDefinition) acceptsNativeEnergy(value *float64) bool {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return false
	}
	switch definition.SignPolicy {
	case "nonnegative":
		return *value >= 0
	case "nonpositive":
		return *value <= 0
	case "signed":
		return true
	}
	return false
}

// Dictionary-family gate only. The future SQL reader must separately verify
// RDD uniqueness, Sum/System (variable) versus meter timestep identity, weather
// calendar, duplicate/missing/NULL rows and separately bound executed ownership.
func (target energyPathPVElectricalTarget) matchesNativeDictionary(name, key, unit, frequency string, isMeter bool) bool {
	if !target.IdentityValid || !strings.EqualFold(strings.TrimSpace(name), target.Definition.Name) || unit != "J" || isMeter != target.Definition.IsMeter || (frequency != "Monthly" && frequency != "Hourly") {
		return false
	}
	if isMeter {
		return strings.TrimSpace(key) == ""
	}
	return target.Key != "" && len(target.OriginalOwners) == 1 && strings.EqualFold(strings.TrimSpace(key), target.Key)
}

type energyPathPVElectricalIntent struct {
	Target    energyPathPVElectricalTarget
	Frequency string
}

func energyPathPVElectricalIntents(inventory energyPathPVElectricalInventory) []energyPathPVElectricalIntent {
	var out []energyPathPVElectricalIntent
	if !inventory.HasOriginal || !inventory.SchemaReviewed {
		return nil
	}
	for _, target := range inventory.Targets {
		if target.IdentityValid {
			for _, frequency := range []string{"Monthly", "Hourly"} {
				out = append(out, energyPathPVElectricalIntent{target, frequency})
			}
		}
	}
	return out
}

func (intent energyPathPVElectricalIntent) request() PurposeOutputObject {
	definition := intent.Target.Definition
	object := PurposeOutputObject{ObjectType: "Output:Variable", Fields: []idf.OutputFieldValue{{Name: "Key Value", Value: intent.Target.Key}, {Name: "Variable Name", Value: definition.Name}, {Name: "Reporting Frequency", Value: intent.Frequency}}, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Reason: "Basic Energy Path", Weight: "medium",
		Description: "Native model-total electrical observation; role=" + definition.Role + ". Source identity is not full electrical topology or additive supply/consumption authority; missing/NULL is not zero."}
	if definition.IsMeter {
		object.ObjectType = "Output:Meter"
		object.Fields = []idf.OutputFieldValue{{Name: "Key Name", Value: definition.Name}, {Name: "Reporting Frequency", Value: intent.Frequency}}
	}
	if intent.Frequency == "Hourly" {
		object.Weight = "heavy"
		object.Description += " Hourly source companion, not a second Monthly budget."
	}
	return object
}

// Native default '*' and explicit '*' variable keys cover all equipment keys.
// A scheduled request is NOT a complete unscheduled observation. Meter wildcard
// support here is the documented suffix '*' family form (e.g. Electricity:*).
// Cumulative/MeterFileOnly and arbitrary regex are not silently equated to an
// ordinary SQL meter request by this bounded helper.
func energyPathPVElectricalRequestCovers(candidate PurposeOutputObject, intent energyPathPVElectricalIntent) bool {
	if !strings.EqualFold(purposeOutputFrequency(candidate.ObjectType, candidate.Fields), intent.Frequency) {
		return false
	}
	definition := intent.Target.Definition
	if definition.IsMeter {
		if !strings.EqualFold(candidate.ObjectType, "Output:Meter") {
			return false
		}
		key := energyPathPVElectricalKey(purposeOutputKeyValue(candidate.Fields))
		name := energyPathPVElectricalKey(definition.Name)
		if key == name {
			return true
		}
		return strings.HasSuffix(key, "*") && strings.Count(key, "*") == 1 && strings.HasPrefix(name, strings.TrimSuffix(key, "*"))
	}
	if !strings.EqualFold(candidate.ObjectType, "Output:Variable") || !strings.EqualFold(purposeOutputVariableName(candidate.Fields), definition.Name) || purposeFieldValue(candidate.Fields, "Schedule Name") != "" {
		return false
	}
	key := purposeOutputKeyValue(candidate.Fields)
	return key == "" || key == "*" || strings.EqualFold(key, intent.Target.Key)
}

// Called inside the standard automatic Hourly pass, after its Monthly/Basic
// Energy eligibility guard and BEFORE it creates an Hourly copy. This is a
// finite PV-intent gate, not a general wildcard rewrite. In particular, one
// keyed Battery/PV request never covers an automatic '*' reporting scope,
// even if it is the only reviewed target: unreviewed peers may also report.
func (builder *purposePlanBuilder) reuseEnergyPathPVElectricalHourlyCoverage(monthly PurposeOutputObject) bool {
	if !purposeIDsContain(monthly.PurposeIDs, SimulationPurposeBasicEnergy) || !strings.EqualFold(purposeOutputFrequency(monthly.ObjectType, monthly.Fields), "Monthly") || purposeFieldValue(monthly.Fields, "Schedule Name") != "" {
		return false
	}
	isMeter := strings.EqualFold(monthly.ObjectType, "Output:Meter")
	if !isMeter && !strings.EqualFold(monthly.ObjectType, "Output:Variable") {
		return false
	}
	name := purposeOutputVariableName(monthly.Fields)
	if isMeter {
		name = purposeOutputKeyValue(monthly.Fields)
	}
	// Fast empty/non-electrical exit before the original ownership census.
	knownName := false
	for _, definition := range energyPathPVElectricalDefinitions() {
		if definition.IsMeter == isMeter && strings.EqualFold(definition.Name, name) {
			knownName = true
			break
		}
	}
	if !knownName {
		return false
	}
	key := purposeOutputKeyValue(monthly.Fields)
	var intent energyPathPVElectricalIntent
	eligible := false
	for _, target := range energyPathBuildPVElectricalInventory(builder.doc).Targets {
		if !target.IdentityValid || target.Definition.IsMeter != isMeter || !strings.EqualFold(target.Definition.Name, name) {
			continue
		}
		if !isMeter && key != "" && key != "*" && !strings.EqualFold(key, target.Key) {
			continue
		}
		intent = energyPathPVElectricalIntent{Target: target, Frequency: "Hourly"}
		eligible = true
		break
	}
	if !eligible {
		return false
	}
	existing := make([]PurposeOutputObject, 0, len(builder.existing))
	for _, object := range builder.existing {
		existing = append(existing, object)
	}
	sort.SliceStable(existing, func(i, j int) bool { return existing[i].Signature < existing[j].Signature })
	for _, candidates := range [][]PurposeOutputObject{existing, builder.objects} {
		for _, candidate := range candidates {
			if !energyPathPVElectricalRequestCovers(candidate, intent) {
				continue
			}
			if !isMeter {
				candidateKey := purposeOutputKeyValue(candidate.Fields)
				if candidateKey != "" && candidateKey != "*" && (key == "" || key == "*" || !strings.EqualFold(candidateKey, key)) {
					continue
				}
			}
			candidate.Fields = append([]idf.OutputFieldValue(nil), candidate.Fields...)
			candidate.PurposeIDs = appendUniquePurposeIDsForPVElectrical(candidate.PurposeIDs, SimulationPurposeBasicEnergy)
			candidate.Reason, candidate.Description = "Basic Energy Path", intent.request().Description
			builder.addObject(candidate)
			return true
		}
	}
	return false
}

// Root calls this once in Energy Path planning AFTER the standard automatic
// Hourly pass, with the scoped coverage hook above installed inside that pass.
// Ordering alone is insufficient: standard Monthly storage requests already
// exist before the pass, so its old literal-'*' check can otherwise duplicate
// an original blank-key Hourly request before this helper gets control.
// Existing original/planned wildcard requests are reused and gain
// BasicEnergy provenance; exact missing intents are added without updates.
func (builder *purposePlanBuilder) addEnergyPathPVElectricalOutputs() {
	inventory := energyPathBuildPVElectricalInventory(builder.doc)
	intents := energyPathPVElectricalIntents(inventory)
	if len(intents) == 0 {
		return
	}
	existing := make([]PurposeOutputObject, 0, len(builder.existing))
	for _, object := range builder.existing {
		existing = append(existing, object)
	}
	sort.SliceStable(existing, func(i, j int) bool { return existing[i].Signature < existing[j].Signature })
	for _, intent := range intents {
		request := intent.request()
		covered := false
		for _, candidates := range [][]PurposeOutputObject{existing, builder.objects} {
			for _, candidate := range candidates {
				if !energyPathPVElectricalRequestCovers(candidate, intent) {
					continue
				}
				candidate.Fields = append([]idf.OutputFieldValue(nil), candidate.Fields...)
				candidate.PurposeIDs = appendUniquePurposeIDsForPVElectrical(candidate.PurposeIDs, SimulationPurposeBasicEnergy)
				candidate.Reason, candidate.Description = request.Reason, request.Description
				builder.addObject(candidate)
				covered = true
				break
			}
			if covered {
				break
			}
		}
		if !covered {
			builder.addObject(request)
		}
	}
}

func appendUniquePurposeIDsForPVElectrical(ids []SimulationPurposeID, id SimulationPurposeID) []SimulationPurposeID {
	return normalizePurposeIDs(append(append([]SimulationPurposeID(nil), ids...), id))
}
