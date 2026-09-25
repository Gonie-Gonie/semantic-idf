package simulation

// Supplemental required4 registry. Core20/40/80 is unchanged. These helpers
// require the root-owned Model/Frames/Checks optional fields listed in the note.
import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type epathSQLPVCogenerationRegistry struct {
	Native  epathSQLPVCogenerationFrames
	SQLPath string // Separate compile-time file anchor; external caller rebinds it.
}

type epathSQLPVCogenerationValidatedSources struct {
	canonical map[string]epathSQLModelCheck
	adapter   epathSQLPVValidatedSources // detached C scalar/shape/chart consumer inputs
}

func epathSQLPVCogenerationHasSourceCheck(check epathSQLModelCheck) bool {
	return check.PVCogenerationSource != nil || strings.Contains(check.Item.Key, "|pv_cogeneration_source/") || strings.Contains(check.Want.Key, "|pv_cogeneration_source/")
}

func epathSQLPVCogenerationCanonicalSourceCheck(systemID string, identity epathSQLPVSourceIdentity, field string) (epathSQLModelCheck, error) {
	if !reflect.DeepEqual(identity.Spec, epathSQLPVCogenerationParentSpec()) {
		return epathSQLModelCheck{}, fmt.Errorf("CG canonical source is not the independently typed parent")
	}
	check, err := epathSQLPVCanonicalSourceCheck(systemID, identity, field)
	if err != nil {
		return check, err
	}
	check.Item.Key = strings.Replace(check.Item.Key, "|pv_native_source/", "|pv_cogeneration_source/", 1)
	check.Want.Key = check.Item.Key
	check.PVCogenerationSource = check.PVSource
	check.PVSource = nil
	return check, nil
}

func epathSQLPVCogenerationRegistryLookup(checks epathSQLModelChecks) (epathSQLPVCogenerationValidatedSources, error) {
	out := epathSQLPVCogenerationValidatedSources{canonical: map[string]epathSQLModelCheck{}, adapter: epathSQLPVValidatedSources{canonical: map[string]epathSQLModelCheck{}, identities: map[int]epathSQLPVValidatedIdentity{}}}
	if checks.RequiredPVCogeneration == nil {
		if checks.PVCogenerationRegistry != nil {
			return out, fmt.Errorf("CG native registry has no independent required declaration")
		}
		return out, nil
	}
	if err := epathSQLPVCogenerationRequiredDeclaration(checks.RequiredPVCogeneration, checks.RequiredPVSystems); err != nil {
		return out, err
	}
	if checks.PVSourceRegistry == nil || len(checks.PVSourceRegistry.Native) != 1 || checks.PVCogenerationRegistry == nil {
		return out, fmt.Errorf("CG required option lost its core/parent registry")
	}
	core := checks.PVSourceRegistry.Native[0]
	reg := checks.PVCogenerationRegistry
	cg := reg.Native
	if reg.SQLPath == "" || len(core.Sources) != 40 || len(cg.Parents) != 2 || len(cg.AncillaryIDs) != 2 || cg.SQLSHA256 != core.SQLSHA256 || cg.Membership.OriginalSHA256 != core.Original.OriginalSHA256 || cg.Membership.ExecutedSHA256 != core.ExecutedSHA256 || !reflect.DeepEqual(core.Original.Declaration, checks.RequiredPVSystems[0]) {
		return out, fmt.Errorf("CG complete2/source/capture registry changed")
	}
	seen := map[int]bool{}
	for _, frequency := range []string{"Monthly", "Hourly"} {
		identity, found := cg.Parents[frequency]
		id := identity.Source.DictionaryIndex
		if !found || id <= 0 || id != identity.Dictionary.Index || seen[id] || identity.Source.ReportingFrequency != frequency || identity.Dictionary.Frequency != frequency || !identity.RequestBound || identity.OriginalOwnerIndex != nil || identity.ExecutedOwnerIndex != nil || !reflect.DeepEqual(identity.Spec, epathSQLPVCogenerationParentSpec()) {
			return out, fmt.Errorf("CG registry substituted a parent ID/spec/frequency/owner")
		}
		if _, overlap := core.Sources[id]; overlap {
			return out, fmt.Errorf("CG parent duplicated a core40 source")
		}
		member, found := core.Sources[cg.AncillaryIDs[frequency]]
		if !found || member.Spec.ID != "inverter.ancillary" || member.Dictionary.Frequency != frequency {
			return out, fmt.Errorf("CG parent registry lost its sole native member reference")
		}
		seen[id] = true
		quantity, err := epathSQLPVSerializedSourceQuantity(identity)
		if err != nil {
			return out, err
		}
		snapshot := epathSQLPVValidatedIdentity{DictionaryIndex: id, Name: identity.Source.Name, Key: identity.Source.KeyValue, Frequency: frequency, IndexGroup: identity.Source.IndexGroup, IsMeter: identity.Source.IsMeter, Quantity: quantity}
		chart, err := epathSQLPVSourceChartExpectation(identity, core.Weather)
		if err != nil {
			return out, err
		}
		snapshot.Chart = chart
		if identity.OutputObjectIndex != nil {
			index := *identity.OutputObjectIndex
			snapshot.OutputObjectIndex = &index
		}
		out.adapter.identities[id] = snapshot
		for _, field := range []string{"rawValue", "effectiveValue"} {
			check, err := epathSQLPVCogenerationCanonicalSourceCheck(checks.RequiredPVCogeneration.SystemID, identity, field)
			if err != nil {
				return out, err
			}
			if check.Want.Group != "endUses" || check.Want.Scope != "building" || check.Want.Period != "annual" || check.Want.Zone != "" {
				return out, fmt.Errorf("CG parent source required context changed")
			}
			if _, duplicate := out.canonical[check.Want.Key]; duplicate {
				return out, fmt.Errorf("duplicate CG required scalar")
			}
			out.canonical[check.Want.Key] = check
			adapted := check
			adapted.PVSource = adapted.PVCogenerationSource
			adapted.PVCogenerationSource = nil
			out.adapter.canonical[adapted.Want.Key] = adapted
		}
	}
	if len(out.canonical) != 4 || len(out.adapter.identities) != 2 {
		return out, fmt.Errorf("CG required2parent/4scalar census changed")
	}
	return out, nil
}

func epathSQLPVCogenerationCheckRegistryCensus(checks epathSQLModelChecks, prepared epathSQLPVCogenerationValidatedSources) error {
	seen := map[string]bool{}
	for _, check := range checks.Rows {
		if !epathSQLPVCogenerationHasSourceCheck(check) {
			continue
		}
		want, found := prepared.canonical[check.Want.Key]
		if !found || seen[check.Want.Key] || !checks.Keys[check.Want.Key] || !reflect.DeepEqual(want, check) {
			return fmt.Errorf("CG required source row changed/lost its exact selector/proof/quantity: %s", check.Want.Key)
		}
		seen[check.Want.Key] = true
	}
	for key := range prepared.canonical {
		if !seen[key] || !checks.Keys[key] {
			return fmt.Errorf("CG required source row/key removed together: %s", key)
		}
	}
	for key, enabled := range checks.Keys {
		if strings.Contains(key, "|pv_cogeneration_source/") && (!enabled || !seen[key]) {
			return fmt.Errorf("CG unexpected/orphan scalar registry key %s", key)
		}
	}
	return nil
}

// Standalone retained-check guard. SQLPath was separately captured at compile;
// the stronger ForModel entry also compares it to the external run's SQLPath.
func epathSQLPVCogenerationRetainedFiles(core epathSQLPVSourceFrames, registry *epathSQLPVCogenerationRegistry) error {
	if registry == nil || registry.SQLPath == "" || registry.Native.MTDPath == "" {
		return fmt.Errorf("CG retained file anchors are missing")
	}
	sqlPath, err := filepath.Abs(registry.SQLPath)
	if err != nil {
		return err
	}
	want := filepath.Clean(filepath.Join(filepath.Dir(sqlPath), "eplusout.mtd"))
	actual, err := filepath.Abs(registry.Native.MTDPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(actual), want) {
		return fmt.Errorf("CG retained MTD is not the captured SQL sibling")
	}
	for _, path := range []string{sqlPath, want} {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("CG retained input is not a regular file")
		}
	}
	digest, err := epathSQLPVFileSHA(sqlPath)
	if err != nil {
		return err
	}
	if digest != core.SQLSHA256 || digest != registry.Native.SQLSHA256 {
		return fmt.Errorf("CG actual SQL hash changed after source compilation")
	}
	bytes, err := os.ReadFile(want)
	if err != nil {
		return err
	}
	if string(bytes) != registry.Native.Membership.MTDText || epathSQLPVProofSHA(string(bytes)) != registry.Native.Membership.MTDSHA256 {
		return fmt.Errorf("CG actual MTD bytes changed after original membership proof")
	}
	return nil
}

func epathSQLPreparePVAndCogenerationSourceChecks(checks epathSQLModelChecks) (epathSQLPVValidatedSources, epathSQLPVCogenerationValidatedSources, error) {
	var core epathSQLPVValidatedSources
	var cg epathSQLPVCogenerationValidatedSources
	var err error
	if checks.RequiredPVCogeneration == nil {
		if checks.PVCogenerationRegistry != nil {
			return core, cg, fmt.Errorf("CG native registry has no independent required declaration")
		}
		core, err = epathSQLPreparePVSourceChecks(checks)
		if err != nil {
			return core, cg, err
		}
	} else {
		if err := epathSQLPVCogenerationRequiredDeclaration(checks.RequiredPVCogeneration, checks.RequiredPVSystems); err != nil {
			return core, cg, err
		}
		if checks.PVSourceRegistry == nil || len(checks.PVSourceRegistry.Native) != 1 || checks.PVCogenerationRegistry == nil {
			return core, cg, fmt.Errorf("CG required option lost its core/parent registry")
		}
		native := checks.PVSourceRegistry.Native[0]
		if err := epathSQLPVCogenerationRetainedFiles(native, checks.PVCogenerationRegistry); err != nil {
			return core, cg, err
		}
		// Native9 validates core40 and parent2 once before either lookup makes
		// detached charts. Do not call core Prepare after this boundary gate.
		if err := epathSQLValidatePVNativeBalances(native, checks.PVCogenerationRegistry.Native); err != nil {
			return core, cg, err
		}
		core, err = epathSQLPVRegistryLookup(checks.RequiredPVSystems, checks.PVSourceRegistry)
		if err != nil {
			return core, cg, err
		}
		if err := epathSQLPVCheckRegistryCensus(checks, core); err != nil {
			return core, cg, err
		}
	}
	cg, err = epathSQLPVCogenerationRegistryLookup(checks)
	if err != nil {
		return core, cg, err
	}
	if err := epathSQLPVCogenerationCheckRegistryCensus(checks, cg); err != nil {
		return core, cg, err
	}
	return core, cg, nil
}

func epathSQLPreparePVAndCogenerationSourceChecksForModel(checks epathSQLModelChecks, model *epathRealSQLModel, external *epathRealOracleEvidence) (epathSQLPVValidatedSources, epathSQLPVCogenerationValidatedSources, error) {
	emptyCore, emptyCG := epathSQLPVValidatedSources{}, epathSQLPVCogenerationValidatedSources{}
	var systems []epathRealSQLPVSystem
	var required *epathRealSQLPVCogeneration
	if model != nil {
		systems, required = model.PVSystems, model.PVCogeneration
	}
	if !epathSQLPVDeclarationsEqual(systems, checks.RequiredPVSystems) || !reflect.DeepEqual(required, checks.RequiredPVCogeneration) {
		return emptyCore, emptyCG, fmt.Errorf("PV/CG check declarations differ from the external required recipe")
	}
	if required != nil {
		if external == nil || checks.PVSourceRegistry == nil || len(checks.PVSourceRegistry.Native) != 1 || checks.PVCogenerationRegistry == nil {
			return emptyCore, emptyCG, fmt.Errorf("CG required external run/native registry is missing")
		}
		actual, err := filepath.Abs(external.sqlPath)
		if err != nil {
			return emptyCore, emptyCG, err
		}
		recorded, err := filepath.Abs(checks.PVCogenerationRegistry.SQLPath)
		if err != nil {
			return emptyCore, emptyCG, err
		}
		if external.sqlPath == "" || checks.PVCogenerationRegistry.SQLPath == "" || !strings.EqualFold(filepath.Clean(actual), filepath.Clean(recorded)) {
			return emptyCore, emptyCG, fmt.Errorf("CG retained SQL path differs from the external original run")
		}
		if err := epathSQLValidatePVCogenerationExternalEvidence(required, systems, *external, checks.PVSourceRegistry.Native[0], checks.PVCogenerationRegistry.Native); err != nil {
			return emptyCore, emptyCG, err
		}
	}
	return epathSQLPreparePVAndCogenerationSourceChecks(checks)
}

func epathSQLCheckPVCogenerationSourceConsumer(bundle PurposeResultBundle, check epathSQLModelCheck, prepared epathSQLPVCogenerationValidatedSources) error {
	if check.PVCogenerationSource == nil {
		return fmt.Errorf("CG required native source lost its typed proof")
	}
	want, found := prepared.canonical[check.Want.Key]
	if !found || !reflect.DeepEqual(want, check) {
		return fmt.Errorf("CG source row escaped its canonical native proof/selector/quantity")
	}
	// Local adapter only; do not mutate canonical/persisted check or registry.
	adapted := check
	adapted.PVSource = adapted.PVCogenerationSource
	adapted.PVCogenerationSource = nil
	return epathSQLCheckPVSourceConsumerComplete(bundle, adapted, prepared.adapter)
}
