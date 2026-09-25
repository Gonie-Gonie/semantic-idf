package simulation

import (
	"fmt"
	"sort"
)

func epathSQLBindPVSources(observed epathRealOracleEvidence, model epathRealSQLModel, frames *epathSQLFrames) error {
	if len(model.PVSystems) == 0 {
		if frames != nil && len(frames.PVSystems) != 0 {
			return fmt.Errorf("undeclared PV native frame cannot authorize a source proof")
		}
		return nil
	}
	if frames == nil || len(frames.PVSystems) != 0 {
		return fmt.Errorf("PV source binding requires fresh independent SQL frames")
	}
	originals, err := epathSQLValidatePVOriginal(observed.originalText, model.PVSystems)
	if err != nil {
		return err
	}
	if len(originals) != 1 {
		return fmt.Errorf("PV source binding requires exactly one finite original proof")
	}
	native, err := epathCompileSQLPVSourceFrames(observed, originals[0], model.Precision)
	if err != nil {
		return err
	}
	frames.PVSystems = []epathSQLPVSourceFrames{native}
	return nil
}

func epathSQLModelPVSourceChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	if checks == nil {
		return fmt.Errorf("PV source registration requires the real check registry")
	}
	if len(checks.RequiredPVSystems) != 0 || checks.PVSourceRegistry != nil {
		return fmt.Errorf("PV source registration must occur exactly once")
	}
	if len(model.PVSystems) == 0 {
		if len(frames.PVSystems) != 0 {
			return fmt.Errorf("PV source frames exist without a required model declaration")
		}
		_, err := epathSQLPreparePVSourceChecks(*checks)
		return err
	}
	// epathSQLBindPVSources immediately precedes this builder in the compiler and
	// fully validates all native rows. The builder makes a separate required
	// declaration/native registry before appending any generated source row.
	next := *checks
	next.Rows = append([]epathSQLModelCheck(nil), checks.Rows...)
	next.Keys = map[string]bool{}
	for key, value := range checks.Keys {
		next.Keys[key] = value
	}
	next.RequiredPVSystems = epathSQLPVCloneDeclarations(model.PVSystems)
	next.PVSourceRegistry = &epathSQLPVSourceRegistry{Native: append([]epathSQLPVSourceFrames(nil), frames.PVSystems...)}
	prepared, err := epathSQLPVRegistryLookup(next.RequiredPVSystems, next.PVSourceRegistry)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(prepared.canonical))
	for key := range prepared.canonical {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, found := next.Keys[key]; found {
			return fmt.Errorf("PV source required key was pre-registered")
		}
		next.Keys[key] = true
		next.Rows = append(next.Rows, prepared.canonical[key])
	}
	if err := epathSQLPVCheckRegistryCensus(next, prepared); err != nil {
		return err
	}
	*checks = next
	return nil
}
