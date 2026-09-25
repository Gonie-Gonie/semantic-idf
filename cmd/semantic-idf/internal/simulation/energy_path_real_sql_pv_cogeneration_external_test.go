package simulation

// External trust anchor, separate from retained check/registry state. This is
// not another native SQL row scan or a substitute for the nine-balance gate.
import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type epathRealSQLPVCogeneration struct {
	SystemID string `json:"systemId"`
	Boundary string `json:"boundary"`
	MTDFile  string `json:"mtdFile"`
}

func epathSQLPVCogenerationRequiredDeclaration(required *epathRealSQLPVCogeneration, systems []epathRealSQLPVSystem) error {
	if required == nil {
		return fmt.Errorf("native Cogeneration registry has no external required declaration")
	}
	if len(systems) != 1 || required.SystemID == "" || required.SystemID != systems[0].ID || required.Boundary != "shop-25.1-native-cogeneration" || required.MTDFile != "eplusout.mtd" {
		return fmt.Errorf("native Cogeneration external finite system/boundary/MTD declaration changed")
	}
	return nil
}

// external must come from the already validated original run/caller, never be
// reconstructed from cg.Membership or its path fields. Pending also compares
// external.sqlPath/outputPlan with run evidence before invoking this helper.
func epathSQLValidatePVCogenerationExternalEvidence(required *epathRealSQLPVCogeneration, systems []epathRealSQLPVSystem, external epathRealOracleEvidence, core epathSQLPVSourceFrames, cg epathSQLPVCogenerationFrames) error {
	if err := epathSQLPVCogenerationRequiredDeclaration(required, systems); err != nil {
		return err
	}
	if !reflect.DeepEqual(systems[0], core.Original.Declaration) || external.sqlPath == "" || external.originalText == "" || external.executedText == "" || external.outputPlan == nil {
		return fmt.Errorf("Cogeneration external original/SQL/executed/plan evidence is missing")
	}
	if !reflect.DeepEqual(external.outputPlan, &cg.OutputPlan) || !reflect.DeepEqual(external.Weather, core.Weather) ||
		external.originalText != cg.Membership.OriginalText || external.executedText != cg.Membership.ExecutedText ||
		epathSQLPVProofSHA(external.originalText) != core.Original.OriginalSHA256 || epathSQLPVProofSHA(external.originalText) != cg.Membership.OriginalSHA256 ||
		epathSQLPVProofSHA(external.executedText) != core.ExecutedSHA256 || epathSQLPVProofSHA(external.executedText) != cg.Membership.ExecutedSHA256 {
		return fmt.Errorf("Cogeneration retained proof differs from external original/executed/weather/request evidence")
	}
	sqlPath, err := filepath.Abs(external.sqlPath)
	if err != nil {
		return err
	}
	wantMTD := filepath.Clean(filepath.Join(filepath.Dir(sqlPath), required.MTDFile))
	if cg.MTDPath == "" {
		return fmt.Errorf("Cogeneration retained MTD path absent")
	}
	actualMTD, err := filepath.Abs(cg.MTDPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(actualMTD), wantMTD) {
		return fmt.Errorf("Cogeneration retained MTD path is not the external SQL capture sibling")
	}
	for _, path := range []string{sqlPath, wantMTD} {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Cogeneration evidence is not a regular file: %s", path)
		}
	}
	sqlHash, err := epathSQLPVFileSHA(sqlPath)
	if err != nil {
		return err
	}
	if sqlHash != core.SQLSHA256 || sqlHash != cg.SQLSHA256 {
		return fmt.Errorf("Cogeneration actual SQL bytes differ from retained native source proof")
	}
	mtd, err := os.ReadFile(wantMTD)
	if err != nil {
		return err
	}
	if string(mtd) != cg.Membership.MTDText || epathSQLPVProofSHA(string(mtd)) != cg.Membership.MTDSHA256 {
		return fmt.Errorf("Cogeneration actual MTD bytes differ from retained membership proof")
	}
	return nil
}
