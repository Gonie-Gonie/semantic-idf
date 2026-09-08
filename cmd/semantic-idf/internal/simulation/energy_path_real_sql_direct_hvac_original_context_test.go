package simulation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Real-run entry points must prove typed ownership from the hash-bound original
// model before evaluating direct component declarations. A recipe alone cannot
// establish whether an identically named DX/Fuel output belongs to that object.
// Mathematical unit frames remain independent of the real capture filesystem.
func epathValidateRealSQLDirectHVACOriginal(evidence epathRealRunEvidence, recipe epathRealOracleRecipe) error {
	if recipe.SQLModel == nil || len(recipe.SQLModel.DirectHVACComponents) == 0 {
		return nil
	}
	if evidence.CatalogDirectory == "" || evidence.Fixture.ModelPath == "" || len(evidence.ModelSHA256) != 64 || evidence.ModelSHA256 != evidence.Fixture.ModelSHA256 ||
		!epathRealSamePath(evidence.OriginalInputPath, filepath.Join(evidence.CatalogDirectory, filepath.FromSlash(evidence.Fixture.ModelPath))) {
		return fmt.Errorf("direct HVAC original model lacks exact catalog path/hash binding")
	}
	data, err := os.ReadFile(evidence.OriginalInputPath)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != evidence.ModelSHA256 {
		return fmt.Errorf("direct HVAC original model bytes differ from the reviewed catalog")
	}
	return epathSQLValidateDirectHVACOriginalModel(string(data), *recipe.SQLModel)
}
