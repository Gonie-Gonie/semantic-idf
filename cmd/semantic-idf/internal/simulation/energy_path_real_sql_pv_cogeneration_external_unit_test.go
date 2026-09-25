package simulation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnergyPathSQLPVCogenerationExternalCaptureAnchor(t *testing.T) {
	// File contents here are provenance-only hand bytes, not a purported SQL
	// physics fixture. Native row/IDF/MTD semantic checks have their own tests.
	for _, mode := range []string{"valid", "missing_declaration", "changed_system", "changed_boundary", "changed_filename", "foreign_path_same_bytes", "forged_text_and_hash", "changed_actual_mtd", "changed_actual_sql", "forged_original_and_hash", "forged_executed_and_hash", "changed_plan", "missing_external_sql", "missing_retained_path"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			sqlPath, mtdPath := filepath.Join(dir, "eplusout.sql"), filepath.Join(dir, "eplusout.mtd")
			write := func(path, text string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(sqlPath, "literal native SQL bytes")
			write(mtdPath, "literal native MTD bytes")
			declaration := epathRealSQLPVSystem{ID: "shop-pv-dc-storage"}
			required := &epathRealSQLPVCogeneration{SystemID: declaration.ID, Boundary: "shop-25.1-native-cogeneration", MTDFile: "eplusout.mtd"}
			plan := PurposeRunPlan{BasicEnergyDetail: "energy_path"}
			external := epathRealOracleEvidence{sqlPath: sqlPath, originalText: "original model bytes", executedText: "executed model bytes", outputPlan: &plan}
			core := epathSQLPVSourceFrames{Original: epathSQLPVOriginalProof{Declaration: declaration, OriginalSHA256: epathSQLPVProofSHA(external.originalText)}, ExecutedSHA256: epathSQLPVProofSHA(external.executedText), SQLSHA256: epathSQLPVProofSHA("literal native SQL bytes")}
			cg := epathSQLPVCogenerationFrames{SQLSHA256: core.SQLSHA256, MTDPath: mtdPath, OutputPlan: plan, Membership: epathSQLPVCogenerationMembership{OriginalText: external.originalText, ExecutedText: external.executedText, OriginalSHA256: core.Original.OriginalSHA256, ExecutedSHA256: core.ExecutedSHA256, MTDText: "literal native MTD bytes", MTDSHA256: epathSQLPVProofSHA("literal native MTD bytes")}}
			switch mode {
			case "missing_declaration":
				required = nil
			case "changed_system":
				required.SystemID = "other"
			case "changed_boundary":
				required.Boundary = "name_only_legacy"
			case "changed_filename":
				required.MTDFile = "../eplusout.mtd"
			case "foreign_path_same_bytes":
				cg.MTDPath = filepath.Join(t.TempDir(), "eplusout.mtd")
				write(cg.MTDPath, cg.Membership.MTDText)
			case "forged_text_and_hash":
				cg.Membership.MTDText = "changed coherent-looking MTD"
				cg.Membership.MTDSHA256 = epathSQLPVProofSHA(cg.Membership.MTDText)
			case "changed_actual_mtd":
				write(mtdPath, "changed actual MTD")
			case "changed_actual_sql":
				write(sqlPath, "changed actual SQL")
			case "forged_original_and_hash":
				cg.Membership.OriginalText = "forged original"
				cg.Membership.OriginalSHA256 = epathSQLPVProofSHA(cg.Membership.OriginalText)
				core.Original.OriginalSHA256 = cg.Membership.OriginalSHA256
			case "forged_executed_and_hash":
				cg.Membership.ExecutedText = "forged executed"
				cg.Membership.ExecutedSHA256 = epathSQLPVProofSHA(cg.Membership.ExecutedText)
				core.ExecutedSHA256 = cg.Membership.ExecutedSHA256
			case "changed_plan":
				cg.OutputPlan.BasicEnergyDetail = "monthly"
			case "missing_external_sql":
				external.sqlPath = ""
			case "missing_retained_path":
				cg.MTDPath = ""
			}
			err := epathSQLValidatePVCogenerationExternalEvidence(required, []epathRealSQLPVSystem{declaration}, external, core, cg)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("external capture anchor %s: %v", mode, err)
			}
		})
	}
}
