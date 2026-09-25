package simulation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// The reviewed declaration cannot establish original physical ownership on its
// own. Both real-run entry points bind the exact catalog bytes before the
// independent typed source compiler receives them. The executed output plan
// stays separate: its object indexes are not original equipment indexes.
func epathBindRealSQLVRFOriginal(evidence epathRealRunEvidence, recipe epathRealOracleRecipe, observed *epathRealOracleEvidence) error {
	if recipe.SQLModel == nil {
		return nil
	}
	heatOnly := evidence.Fixture.ID == "no-cooling-25-1" && evidence.Fixture.Version == "25.1" || len(recipe.SQLModel.HeatOnlyFurnaces) > 0
	central := evidence.Fixture.ID == "simultaneous-25-1" && evidence.Fixture.Version == "25.1"
	simpleVentilation := evidence.Fixture.ID == "no-heating-ventilation-25-1" && evidence.Fixture.Version == "25.1"
	for _, component := range recipe.SQLModel.DirectHVACComponents {
		simpleVentilation = simpleVentilation || component.ID == epathSQLSimpleVentilationFanFamily
	}
	needsOriginal := central || heatOnly || simpleVentilation || recipe.SQLModel.OriginalZoneMultiplierProof != "" || len(recipe.SQLModel.NativeVRFSystems) > 0 || len(recipe.SQLModel.BaseboardContexts) > 0 || len(recipe.SQLModel.HVACConsumptionPools) > 0 || len(recipe.SQLModel.AirLoopFans) > 0 || len(recipe.SQLModel.PoolSystems) > 0 || len(recipe.SQLModel.PVSystems) > 0 || len(recipe.SQLModel.PVHVACLoops) > 0
	for _, load := range recipe.SQLModel.Loads {
		needsOriginal = needsOriginal || load.NativeRadiant != nil
	}
	if !needsOriginal {
		return nil
	}
	if observed == nil || evidence.CatalogDirectory == "" || evidence.Fixture.ModelPath == "" ||
		len(evidence.ModelSHA256) != 64 || evidence.ModelSHA256 != evidence.Fixture.ModelSHA256 ||
		!epathRealSamePath(evidence.OriginalInputPath, filepath.Join(evidence.CatalogDirectory, filepath.FromSlash(evidence.Fixture.ModelPath))) {
		return fmt.Errorf("native VRF original model lacks exact catalog path/hash binding")
	}
	data, err := os.ReadFile(evidence.OriginalInputPath)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != evidence.ModelSHA256 {
		return fmt.Errorf("native VRF original model bytes differ from the reviewed catalog")
	}
	var executed string
	if central || heatOnly || simpleVentilation || len(recipe.SQLModel.BaseboardContexts) > 0 || len(recipe.SQLModel.PoolSystems) > 0 || len(recipe.SQLModel.PVSystems) > 0 || len(recipe.SQLModel.PVHVACLoops) > 0 {
		if evidence.InputPath == "" || evidence.RunDirectory == "" || len(evidence.ExecutedSHA256) != 64 || !epathRealSamePath(filepath.Dir(evidence.InputPath), evidence.RunDirectory) {
			return fmt.Errorf("baseboard recipient indexes lack exact executed input path/hash binding")
		}
		runData, err := os.ReadFile(evidence.InputPath)
		if err != nil {
			return err
		}
		runDigest := sha256.Sum256(runData)
		if hex.EncodeToString(runDigest[:]) != evidence.ExecutedSHA256 {
			return fmt.Errorf("baseboard executed input bytes differ from the successful original run")
		}
		executed = string(runData)
	}
	if central {
		proof, err := epathSQLValidateCentralOriginal(string(data))
		if err != nil {
			return err
		}
		if proof.OriginalSHA256 != "84d389f1c40ab098eea05105664dd71d54c58509a4d812bf6d5dfe4e933c28f0" {
			return fmt.Errorf("finite Central catalog original is not the reviewed literal model")
		}
		if _, err := epathSQLBindCentralExecuted(string(data), executed, proof); err != nil {
			return err
		}
	}
	observed.originalText, observed.executedText = string(data), executed
	return nil
}

func epathSQLBindVRFFrames(observed epathRealOracleEvidence, model epathRealSQLModel, frames *epathSQLFrames) error {
	if frames == nil {
		return fmt.Errorf("native VRF proof requires SQL frames")
	}
	systems, err := epathCompileSQLVRFSystems(observed.sqlPath, observed.originalText, observed.outputPlan, observed.Sources, model, *frames)
	if err != nil {
		return err
	}
	allocations := make([]epathSQLVRFAllocationProof, 0, len(systems))
	for _, system := range systems {
		allocation, err := epathSQLCompileVRFAllocation(*frames, system)
		if err != nil {
			return err
		}
		allocations = append(allocations, allocation)
	}
	frames.NativeVRFSystems, frames.NativeVRFAllocations = systems, allocations
	return nil
}
