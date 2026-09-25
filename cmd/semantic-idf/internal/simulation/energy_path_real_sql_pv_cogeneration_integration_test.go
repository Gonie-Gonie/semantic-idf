package simulation

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
)

func epathSQLBindPVCogenerationSources(observed epathRealOracleEvidence, model epathRealSQLModel, frames *epathSQLFrames) error {
	if model.PVCogeneration == nil {
		if frames != nil && frames.PVCogeneration != nil {
			return fmt.Errorf("undeclared CG native frame cannot authorize source/consumption proof")
		}
		return nil
	}
	if err := epathSQLPVCogenerationRequiredDeclaration(model.PVCogeneration, model.PVSystems); err != nil {
		return err
	}
	if frames == nil || frames.PVCogeneration != nil || len(frames.PVSystems) != 1 {
		return fmt.Errorf("CG native binding requires fresh parent and already compiled core frame")
	}
	native, err := epathCompileSQLPVCogenerationFrames(observed, frames.PVSystems[0], filepath.Join(filepath.Dir(observed.sqlPath), model.PVCogeneration.MTDFile))
	if err != nil {
		return err
	}
	if err := epathSQLValidatePVCogenerationExternalEvidence(model.PVCogeneration, model.PVSystems, observed, frames.PVSystems[0], native); err != nil {
		return err
	}
	if err := epathSQLValidatePVNativeBalances(frames.PVSystems[0], native); err != nil {
		return err
	}
	frames.PVCogeneration = &native
	return nil
}

func epathSQLModelPVCogenerationSourceChecks(observed epathRealOracleEvidence, frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	if checks == nil {
		return fmt.Errorf("CG registration requires the real source registry")
	}
	if checks.RequiredPVCogeneration != nil || checks.PVCogenerationRegistry != nil {
		return fmt.Errorf("CG source registration must occur exactly once")
	}
	if model.PVCogeneration == nil {
		if frames.PVCogeneration != nil {
			return fmt.Errorf("CG frame exists without external declaration")
		}
		prepared, err := epathSQLPVCogenerationRegistryLookup(*checks)
		if err != nil {
			return err
		}
		return epathSQLPVCogenerationCheckRegistryCensus(*checks, prepared)
	}
	if err := epathSQLPVCogenerationRequiredDeclaration(model.PVCogeneration, model.PVSystems); err != nil {
		return err
	}
	if frames.PVCogeneration == nil || len(frames.PVSystems) != 1 || checks.PVSourceRegistry == nil || len(checks.PVSourceRegistry.Native) != 1 || observed.sqlPath == "" || !reflect.DeepEqual(model.PVSystems, checks.RequiredPVSystems) || !reflect.DeepEqual(frames.PVSystems[0], checks.PVSourceRegistry.Native[0]) {
		return fmt.Errorf("CG row builder requires the same previously validated native/core declaration")
	}
	// Register requirements independently before source rows. Deep-copy only the
	// small Cg2 frame; never duplicate the40-family native registry here.
	nativeBytes, err := json.Marshal(frames.PVCogeneration)
	if err != nil {
		return err
	}
	var copied epathSQLPVCogenerationFrames
	if err := json.Unmarshal(nativeBytes, &copied); err != nil {
		return err
	}
	next := *checks
	next.Rows = append([]epathSQLModelCheck(nil), checks.Rows...)
	next.Keys = map[string]bool{}
	for key, value := range checks.Keys {
		next.Keys[key] = value
	}
	required := *model.PVCogeneration
	next.RequiredPVCogeneration = &required
	sqlPath, err := filepath.Abs(observed.sqlPath)
	if err != nil {
		return err
	}
	next.PVCogenerationRegistry = &epathSQLPVCogenerationRegistry{Native: copied, SQLPath: sqlPath}
	prepared, err := epathSQLPVCogenerationRegistryLookup(next)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(prepared.canonical))
	for key := range prepared.canonical {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, exists := next.Keys[key]; exists {
			return fmt.Errorf("CG required key was already registered")
		}
		next.Keys[key] = true
		next.Rows = append(next.Rows, prepared.canonical[key])
	}
	if err := epathSQLPVCogenerationCheckRegistryCensus(next, prepared); err != nil {
		return err
	}
	*checks = next
	return nil
}

// Compile-final declaration/census only: the binder already validated native9.
// Evaluation, coverage and pending must use generalized Prepare, not this.
func epathSQLValidatePVAndCogenerationCompiledSourceChecks(checks epathSQLModelChecks, model epathRealSQLModel) error {
	if err := epathSQLValidatePVCompiledSourceChecks(checks, model); err != nil {
		return err
	}
	if !reflect.DeepEqual(model.PVCogeneration, checks.RequiredPVCogeneration) {
		return fmt.Errorf("CG compiled registry lost external model requirement")
	}
	prepared, err := epathSQLPVCogenerationRegistryLookup(checks)
	if err != nil {
		return err
	}
	return epathSQLPVCogenerationCheckRegistryCensus(checks, prepared)
}
