package simulation

// Retained original/executed HVAC binding for the finite Shop model.
import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type epathSQLPVHVACInputs struct {
	Schema      string
	PVSystems   []epathRealSQLPVSystem
	Loops       []epathRealSQLPVHVACLoop
	FanPools    []epathRealSQLFanPool
	Services    []epathRealSQLService
	Auxiliaries []epathRealSQLAuxiliary
}

type epathSQLPVHVACStamp struct {
	Inputs                     epathSQLPVHVACInputs
	OriginalText, ExecutedText string
	Original                   epathSQLPVHVACOriginalProof
	Executed                   []epathSQLPVOriginalObject
	Digest                     string
	RowDigests                 map[string]string
}

func epathSQLPVHVACInputsFor(model epathRealSQLModel) epathSQLPVHVACInputs {
	return epathSQLPVHVACInputs{model.Schema, model.PVSystems, model.PVHVACLoops, model.FanPools, model.Services, model.Auxiliaries}
}

func epathSQLPVHVACDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func epathSQLPVHVACStampDigest(stamp *epathSQLPVHVACStamp) (string, error) {
	// A detached binding, not a replacement for independently validating the
	// original/executed topology at each retained-evidence boundary.
	return epathSQLPVHVACDigest(struct {
		Inputs                     epathSQLPVHVACInputs
		OriginalText, ExecutedText string
		Original                   epathSQLPVHVACOriginalProof
		Executed                   []epathSQLPVOriginalObject
	}{stamp.Inputs, stamp.OriginalText, stamp.ExecutedText, stamp.Original, stamp.Executed})
}

func epathSQLPVHVACCheckDigest(check epathSQLModelCheck) (string, error) {
	// Keep every selector, expected identity and proof-presence obligation.
	// Existing typed consumers still validate their complete numeric proofs.
	// No physical object or retained hourly row is copied into this digest.
	var kinds []string
	v, typ := reflect.ValueOf(check), reflect.TypeOf(check)
	for n := 0; n < v.NumField(); n++ {
		if v.Field(n).Kind() == reflect.Ptr && !v.Field(n).IsNil() {
			kinds = append(kinds, typ.Field(n).Name)
		}
	}
	return epathSQLPVHVACDigest(struct {
		Item                                      epathRealOracleMetricRecipe
		Want                                      epathRealOracleMetric
		ProofKinds                                []string
		OptionalPresentation, AnnualServiceAbsent bool
	}{check.Item, check.Want, kinds, check.OptionalPresentation, check.AnnualServiceAbsent})
}

func epathSQLPVHVACRowStamp(check epathSQLModelCheck, digest string) string {
	key := check.Item.Key
	if strings.Contains(key, "|fan_pool/") || strings.Contains(key, "|fan_pools/") || strings.Contains(key, "|hvac_zone/") || check.Item.Scope == "building" && (check.Item.Group == "ratios" || check.Item.Group == "zoneAllocation") && (strings.Contains(key, "|cooling/") || strings.Contains(key, "|heating/")) {
		return digest
	}
	// Independent PV/CG registries compare entire canonical source rows.
	// Preserve those rows byte-for-byte; RowDigests still retains their census.
	return ""
}

// Called only by the full SQL-model compiler, before any fan/service builder.
// Low-level PV source registry builders never call this function.
func epathSQLBindPVHVACChecks(observed epathRealOracleEvidence, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	if checks == nil || checks.PVHVACRequired || checks.PVHVAC != nil {
		return fmt.Errorf("Shop HVAC binding requires a fresh full-model check registry")
	}
	if len(model.PVSystems) == 0 && len(model.PVHVACLoops) == 0 {
		return nil
	}
	if len(model.PVSystems) != 1 || len(model.PVHVACLoops) != 5 {
		return fmt.Errorf("full Shop PV model requires five independent HVAC declarations")
	}
	if len(model.AirLoopFans)+len(model.PoolSystems)+len(model.NativeVRFSystems)+len(model.HVACConsumptionPools)+len(model.DirectHVACComponents) != 0 {
		return fmt.Errorf("finite Shop HVAC cannot borrow another system's ownership path")
	}
	inputs := epathSQLPVHVACInputsFor(model)
	data, err := json.Marshal(inputs)
	if err != nil {
		return err
	}
	var detached epathSQLPVHVACInputs
	if err := json.Unmarshal(data, &detached); err != nil {
		return err
	}
	proof, err := epathSQLValidatePVHVACOriginal(observed.originalText, detached.Loops)
	if err != nil {
		return err
	}
	executed, err := epathSQLBindPVHVACExecuted(observed.originalText, observed.executedText, proof)
	if err != nil {
		return err
	}
	if err := epathSQLPVHVACDeclaredInputs(observed.originalText, proof, detached.FanPools, detached.Services, detached.Auxiliaries); err != nil {
		return err
	}
	stamp := &epathSQLPVHVACStamp{Inputs: detached, OriginalText: observed.originalText, ExecutedText: observed.executedText, Original: proof, Executed: executed}
	stamp.Digest, err = epathSQLPVHVACStampDigest(stamp)
	if err != nil {
		return err
	}
	checks.PVHVACRequired, checks.PVHVAC = true, stamp
	return nil
}

func epathSQLPVHVACRegistry(checks epathSQLModelChecks) (map[string]epathSQLModelCheck, error) {
	rows := map[string]epathSQLModelCheck{}
	for _, row := range checks.Rows {
		key := row.Want.Key
		if key == "" || row.Item.Key != key || !checks.Keys[key] || rows[key].Want.Key != "" || row.Item.Group != row.Want.Group || row.Item.Scope != row.Want.Scope || row.Item.Zone != row.Want.Zone || row.Item.Period != row.Want.Period || row.Item.Unit != row.Want.Unit {
			return nil, fmt.Errorf("Shop HVAC lost an exact compiled selector/registry identity")
		}
		rows[key] = row
	}
	if len(rows) != len(checks.Keys) {
		return nil, fmt.Errorf("Shop HVAC compiled registry has missing/orphan keys")
	}
	return rows, nil
}

func epathSQLPVHVACCensus(checks epathSQLModelChecks) error {
	stamp := checks.PVHVAC
	if stamp == nil {
		return fmt.Errorf("Shop HVAC census requires its independent stamp")
	}
	for _, pool := range stamp.Inputs.FanPools {
		if len(pool.ServedZones) != 1 {
			return fmt.Errorf("Shop HVAC fan pool lost its one declared Zone")
		}
	}
	rows, err := epathSQLPVHVACRegistry(checks)
	if err != nil {
		return err
	}
	need := func(group, scope, zone, period, suffix string, target epathRealOracleTarget, proof string) error {
		key := strings.Join([]string{group, scope, strings.ToLower(zone), period, suffix}, "|")
		row, found := rows[key]
		actual := row.Item.Target
		// Match the existing fan source selector's case-insensitive native
		// name/key contract, then retain its actual SQL spelling in RowDigests.
		if target.Collection == "sources" && strings.EqualFold(actual.SourceName, target.SourceName) && strings.EqualFold(actual.SourceKey, target.SourceKey) {
			actual.SourceName, actual.SourceKey = target.SourceName, target.SourceKey
		}
		if !found || !reflect.DeepEqual(actual, target) {
			return fmt.Errorf("Shop HVAC missing/changed required selector %s", key)
		}
		if proof == "conversion" && row.Conversion == nil || proof == "allocation" && row.Allocation == nil || proof == "service" && (row.ZoneService == nil || row.ZoneService.Service != target.Category || row.ZoneService.Basis != target.Basis) {
			return fmt.Errorf("Shop HVAC required selector lost its typed proof: %s", key)
		}
		return nil
	}
	periods := []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"}
	for _, pool := range stamp.Inputs.FanPools {
		for _, field := range []string{"rawValue", "effectiveValue"} {
			target := epathRealOracleTarget{Collection: "sources", Field: field, SourceName: pool.Name, SourceKey: pool.Key, SourceUnit: "J", Frequency: "Hourly", Unit: "kWh"}
			if err := need("endUses", "building", "", "annual", "fan_pool/"+pool.Key+"/"+field, target, ""); err != nil {
				return err
			}
		}
		for _, period := range periods {
			target := epathSQLNodeTarget("end_use", "fans", "", "site")
			target.Basis, target.AllowPrunedZero = "service_path_allocation", true
			if err := need("zoneAllocation", "zone", pool.ServedZones[0], period, "fan_pools/value", target, ""); err != nil {
				return err
			}
		}
	}
	if len(checks.FanPoolTotals) != 1 || !reflect.DeepEqual(checks.FanPoolTotals["fans.electricity"].Pools, stamp.Inputs.FanPools) {
		return fmt.Errorf("Shop HVAC lost the independently audited fan pool partition")
	}
	for _, service := range stamp.Inputs.Services {
		if len(service.CarrierReconciliationIDs) != 0 {
			return fmt.Errorf("Shop HVAC requires its ordinary single-carrier service ledger")
		}
		for _, period := range periods {
			for _, zone := range append([]string{""}, service.ServedZones...) {
				scope, prefix := "zone", "hvac_zone/"+service.Service+"/"
				if zone == "" {
					scope, prefix = "building", service.Service+"/"
				}
				for _, basis := range []string{service.Basis, service.FallbackBasis} {
					kind := service.RatioKind
					if basis == service.FallbackBasis {
						kind = service.FallbackRatioKind
					}
					target := epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: service.Service, Basis: basis, FromUnit: "kWh", ToUnit: "kWh", RatioKind: kind, Aggregate: "sum"}
					if err := need("ratios", scope, zone, period, prefix+basis, target, "conversion"); err != nil {
						return err
					}
				}
				if zone == "" {
					id, err := epathSQLAllocationID(service.ReconciliationID, period)
					if err != nil {
						return err
					}
					for _, field := range []string{"expectedValue", "directValue", "allocatedValue", "unassignedValue"} {
						target := epathRealOracleTarget{Collection: "reconciliation", ID: id, Level: "allocation", Field: field, Unit: "kWh"}
						if err := need("zoneAllocation", scope, zone, period, prefix+field, target, "allocation"); err != nil {
							return err
						}
					}
				} else {
					for _, field := range []string{"value", "allocatedValue"} {
						target := epathSQLNodeTarget("end_use", service.Service, "", "site")
						target.Field, target.Basis, target.AllowPrunedZero = field, service.Basis, true
						proof := ""
						if field == "value" {
							proof = "service"
						}
						if err := need("zoneAllocation", scope, zone, period, prefix+field, target, proof); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

// Compile-final call: original/executed validation already ran in this same
// compiler invocation. Establish obligations only after every ordinary builder.
func epathSQLSealPVHVACChecks(checks *epathSQLModelChecks, model epathRealSQLModel) error {
	if checks == nil {
		return fmt.Errorf("Shop HVAC sealing requires checks")
	}
	if len(model.PVSystems) == 0 && len(model.PVHVACLoops) == 0 && !checks.PVHVACRequired && checks.PVHVAC == nil {
		return nil
	}
	if !checks.PVHVACRequired || checks.PVHVAC == nil || !reflect.DeepEqual(checks.PVHVAC.Inputs, epathSQLPVHVACInputsFor(model)) || checks.PVHVAC.RowDigests != nil {
		return fmt.Errorf("Shop HVAC compile-final binding is missing, changed or already sealed")
	}
	if err := epathSQLPVHVACCensus(*checks); err != nil {
		return err
	}
	rows := map[string]string{}
	for _, row := range checks.Rows {
		if row.PVHVACProofSHA256 != "" {
			return fmt.Errorf("Shop HVAC row was stamped before final compilation")
		}
		digest, err := epathSQLPVHVACCheckDigest(row)
		if err != nil {
			return err
		}
		rows[row.Want.Key] = digest
	}
	checks.PVHVAC.RowDigests = rows
	for n := range checks.Rows {
		checks.Rows[n].PVHVACProofSHA256 = epathSQLPVHVACRowStamp(checks.Rows[n], checks.PVHVAC.Digest)
	}
	return epathSQLValidatePVHVACCompiledChecks(*checks, &model)
}

// Cheap structural validation; valid only inside the compilation call that
// already ran Bind. Retained evidence must use Prepare below.
func epathSQLValidatePVHVACCompiledChecks(checks epathSQLModelChecks, external *epathRealSQLModel) error {
	required := checks.PVHVACRequired || checks.PVHVAC != nil
	for _, row := range checks.Rows {
		required = required || row.PVHVACProofSHA256 != ""
	}
	if external != nil {
		required = required || len(external.PVHVACLoops) != 0 || external.Schema != "" && len(external.PVSystems) != 0
	}
	if !required {
		return nil
	}
	stamp := checks.PVHVAC
	if !checks.PVHVACRequired || stamp == nil || len(stamp.Inputs.PVSystems) != 1 || len(stamp.Inputs.Loops) != 5 || len(stamp.Inputs.FanPools) != 5 || len(stamp.Inputs.Services) != 2 || len(stamp.RowDigests) == 0 {
		return fmt.Errorf("mandatory full-model Shop HVAC binding was removed")
	}
	if external != nil && !reflect.DeepEqual(stamp.Inputs, epathSQLPVHVACInputsFor(*external)) {
		return fmt.Errorf("Shop HVAC retained inputs differ from the external full model")
	}
	if external != nil && len(external.AirLoopFans)+len(external.PoolSystems)+len(external.NativeVRFSystems)+len(external.HVACConsumptionPools)+len(external.DirectHVACComponents) != 0 {
		return fmt.Errorf("finite Shop HVAC external model borrowed another system's ownership path")
	}
	if !epathSQLPVDeclarationsEqual(checks.RequiredPVSystems, stamp.Inputs.PVSystems) || checks.PVSourceRegistry == nil || len(checks.PVSourceRegistry.Native) != 1 {
		return fmt.Errorf("Shop HVAC lost the separately required native PV source anchor")
	}
	native := checks.PVSourceRegistry.Native[0]
	if native.Original.OriginalSHA256 != stamp.Original.OriginalSHA256 || native.ExecutedSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(stamp.ExecutedText))) || !reflect.DeepEqual(native.Original.Declaration, stamp.Inputs.PVSystems[0]) {
		return fmt.Errorf("Shop HVAC original/executed binding differs from the independent native PV proof")
	}
	digest, err := epathSQLPVHVACStampDigest(stamp)
	if err != nil || stamp.Digest != digest {
		return fmt.Errorf("Shop HVAC detached original/executed/input stamp changed")
	}
	if err := epathSQLPVHVACCensus(checks); err != nil {
		return err
	}
	if len(stamp.RowDigests) != len(checks.Rows) {
		return fmt.Errorf("Shop HVAC lost required compiled rows")
	}
	for _, row := range checks.Rows {
		digest, err := epathSQLPVHVACCheckDigest(row)
		if err != nil || row.PVHVACProofSHA256 != epathSQLPVHVACRowStamp(row, stamp.Digest) || stamp.RowDigests[row.Want.Key] != digest {
			return fmt.Errorf("Shop HVAC compiled row changed or lost its binding: %s", row.Want.Key)
		}
	}
	return nil
}

// Evaluation, standalone coverage and pending each call this once before any
// row consumers. A nonempty external Schema denotes a full-model boundary;
// native source diagnostics may pass a schema-less external PV/CG declaration.
// Either a retained stamp or explicit HVAC loops also retains applicability.
func epathSQLPreparePVHVACChecks(checks epathSQLModelChecks, models ...*epathRealSQLModel) error {
	if len(models) > 1 {
		return fmt.Errorf("Shop HVAC boundary accepts at most one external full model")
	}
	var external *epathRealSQLModel
	if len(models) == 1 {
		external = models[0]
	}
	if err := epathSQLValidatePVHVACCompiledChecks(checks, external); err != nil {
		return err
	}
	if checks.PVHVAC == nil {
		return nil
	}
	stamp := checks.PVHVAC
	if err := epathSQLPVHVACDeclaredInputs(stamp.OriginalText, stamp.Original, stamp.Inputs.FanPools, stamp.Inputs.Services, stamp.Inputs.Auxiliaries); err != nil {
		return err
	}
	if !reflect.DeepEqual(stamp.Original.Loops, stamp.Inputs.Loops) {
		return fmt.Errorf("Shop HVAC original proof lost the independently declared loops")
	}
	executed, err := epathSQLBindPVHVACExecuted(stamp.OriginalText, stamp.ExecutedText, stamp.Original)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(executed, stamp.Executed) {
		return fmt.Errorf("Shop HVAC separately parsed executed object identities changed")
	}
	return nil
}
