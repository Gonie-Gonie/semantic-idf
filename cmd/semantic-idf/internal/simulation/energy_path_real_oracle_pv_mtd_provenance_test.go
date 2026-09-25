package simulation

// Supplementary capture identity only. Native CG membership, calendars and
// nine-balance semantics remain the independent SQL registry's obligation.
import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func epathOracleValidateMTDFields(file, digest string) error {
	if file == "" && digest == "" {
		return nil
	}
	decoded, err := hex.DecodeString(digest)
	if file != "eplusout.mtd" || err != nil || len(decoded) != 32 {
		return fmt.Errorf("optional MTD provenance requires exact eplusout.mtd and a SHA-256 digest")
	}
	return nil
}

// model comes from the external recipe, never retained source/check state.
// Only the exact sibling of this externally supplied SQL can provide MTD bytes.
func epathOraclePVCogenerationMTDForModel(evidence epathRealRunEvidence, model *epathRealSQLModel) (string, string, error) {
	if model == nil || model.PVCogeneration == nil {
		return "", "", nil
	}
	if err := epathSQLPVCogenerationRequiredDeclaration(model.PVCogeneration, model.PVSystems); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(evidence.SQLPath) == "" {
		return "", "", fmt.Errorf("CG MTD provenance requires the external SQL capture path")
	}
	sqlPath, err := filepath.Abs(evidence.SQLPath)
	if err != nil {
		return "", "", err
	}
	mtdPath := filepath.Join(filepath.Dir(sqlPath), model.PVCogeneration.MTDFile)
	for _, path := range []string{sqlPath, mtdPath} {
		info, err := os.Stat(path)
		if err != nil {
			return "", "", err
		}
		if !info.Mode().IsRegular() {
			return "", "", fmt.Errorf("CG provenance input is not a regular file: %s", path)
		}
	}
	sqlSHA, err := epathOracleHashFile(sqlPath)
	if err != nil {
		return "", "", err
	}
	if sqlSHA != evidence.SQLSHA256 {
		return "", "", fmt.Errorf("CG MTD provenance SQL differs from the external capture digest")
	}
	digest, err := epathOracleHashFile(mtdPath)
	if err != nil {
		return "", "", err
	}
	return "eplusout.mtd", digest, nil
}

// Every real workflow already binds this evidence to the external catalog.
// Rereading that recipe here keeps all existing snapshot/pending/reviewed
// callers on one rule, without adding a mutable requirement to run evidence.
func epathOraclePVCogenerationMTDForEvidence(evidence epathRealRunEvidence) (string, string, error) {
	if strings.TrimSpace(evidence.Fixture.OraclePath) == "" {
		// Preserve legacy captures and synthetic integrity-only fixtures that
		// have no SQL-model recipe. Real CG workflows require an external recipe.
		return "", "", nil
	}
	if strings.TrimSpace(evidence.CatalogDirectory) == "" {
		return "", "", fmt.Errorf("CG recipe provenance requires the external catalog directory")
	}
	catalog, err := filepath.Abs(evidence.CatalogDirectory)
	if err != nil {
		return "", "", err
	}
	name := filepath.FromSlash(evidence.Fixture.OraclePath)
	if filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return "", "", fmt.Errorf("oracle recipe must be relative to the external catalog")
	}
	path := filepath.Join(catalog, name)
	relative, err := filepath.Rel(catalog, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("oracle recipe escaped the external catalog")
	}
	recipe, err := epathLoadRealOracleRecipe(path)
	if err != nil {
		return "", "", err
	}
	return epathOraclePVCogenerationMTDForModel(evidence, recipe.SQLModel)
}

// The approved header carries the reviewed bytes anchor independently from
// the live recipe and SQL sibling. This also supports inline metric manifests.
func epathValidateExpectedPVCogenerationMTD(manifest epathRealExpectedManifest, evidence epathRealRunEvidence, model *epathRealSQLModel) error {
	if err := epathOracleValidateMTDFields(manifest.MTDFile, manifest.MTDSHA256); err != nil {
		return err
	}
	file, digest, err := epathOraclePVCogenerationMTDForModel(evidence, model)
	if err != nil {
		return err
	}
	if manifest.MTDFile != file || manifest.MTDSHA256 != digest {
		return fmt.Errorf("approved expected manifest lost or changed the external CG MTD anchor")
	}
	return nil
}
