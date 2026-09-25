package simulation

// Temporary file/serialization fixtures only; no SQL compiler, production
// projection, engine or repository candidate/expected quantity is used here.
import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type epathOracleMTDUnitInput struct {
	root, candidate string
	evidence        epathRealRunEvidence
	recipe          epathRealOracleRecipe
}

func epathOracleMTDUnitWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func epathOracleMTDUnitRecipe(t *testing.T, input epathOracleMTDUnitInput) {
	t.Helper()
	data, err := json.Marshal(input.recipe)
	if err != nil {
		t.Fatal(err)
	}
	epathOracleMTDUnitWrite(t, filepath.Join(input.evidence.CatalogDirectory, input.evidence.Fixture.OraclePath), data)
}

func epathOracleMTDUnitFixture(t *testing.T) epathOracleMTDUnitInput {
	t.Helper()
	input := epathOracleMTDUnitInput{root: t.TempDir()}
	input.evidence.CatalogDirectory = filepath.Join(input.root, "catalog")
	input.evidence.Fixture.OraclePath = "oracles/hand-cg.json"
	input.evidence.RunDirectory = filepath.Join(input.root, "capture")
	input.evidence.SQLPath = filepath.Join(input.evidence.RunDirectory, "eplusout.sql")
	epathOracleMTDUnitWrite(t, filepath.Join(input.evidence.RunDirectory, "run-evidence.json"), []byte("literal original capture identity"))
	epathOracleMTDUnitWrite(t, input.evidence.SQLPath, []byte("literal SQL capture bytes; not queried"))
	epathOracleMTDUnitWrite(t, filepath.Join(input.evidence.RunDirectory, "eplusout.mtd"), []byte("literal MTD capture bytes; native membership tested separately"))
	var err error
	input.evidence.SQLSHA256, err = epathOracleHashFile(input.evidence.SQLPath)
	if err != nil {
		t.Fatal(err)
	}
	input.evidence.ExecutedSHA256, input.evidence.EngineSHA256, input.evidence.WeatherSHA256 = strings.Repeat("b", 64), strings.Repeat("c", 64), strings.Repeat("d", 64)
	declaration := epathSQLPVOriginalDeclaration()
	input.recipe = epathRealOracleRecipe{
		Schema: "semantic-idf.energy-path-sql-oracle-recipe/v1", Review: "literal provenance-only fixture",
		SQLModel: &epathRealSQLModel{
			PVSystems:      []epathRealSQLPVSystem{declaration},
			PVCogeneration: &epathRealSQLPVCogeneration{SystemID: declaration.ID, Boundary: "shop-25.1-native-cogeneration", MTDFile: "eplusout.mtd"},
		},
	}
	epathOracleMTDUnitRecipe(t, input)
	input.candidate = filepath.Join(input.root, "snapshot.json")
	wire := epathOracleWireFixture()
	wire["energyExplanation"].(map[string]any)["scope"] = map[string]any{"kind": "building"}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	epathOracleMTDUnitWrite(t, input.candidate, data)
	return input
}

func epathOracleMTDUnitSidecar(t *testing.T, input epathOracleMTDUnitInput, provenance epathOracleSnapshotProvenance) []byte {
	t.Helper()
	var err error
	provenance.CandidateSHA256, err = epathOracleHashFile(input.candidate)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(provenance)
	if err != nil {
		t.Fatal(err)
	}
	epathOracleMTDUnitWrite(t, input.candidate+".provenance.json", data)
	return data
}

func TestEnergyPathOracleMTDProvenanceLegacyJSONBytesUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name  string
		wire  string
		value any
	}{
		{"snapshot", `{"schema":"s","captureDirectory":"c","captureSHA256":"a","sqlSHA256":"b","executedSHA256":"d","engineSHA256":"e","weatherSHA256":"f","productionSHA256":"g","candidateSHA256":"h","acceptance":false}`, &epathOracleSnapshotProvenance{}},
		{"manifest", `{"schema":"s","fixtureId":"f","version":"v","modelSHA256":"m","weatherSHA256":"w","review":"r"}`, &epathRealExpectedManifest{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.wire), tc.value); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(tc.value)
			if err != nil || string(encoded) != tc.wire {
				t.Fatalf("legacy bytes/hash changed: %s / %v", encoded, err)
			}
		})
	}
	input := epathOracleMTDUnitFixture(t)
	input.recipe.SQLModel.PVCogeneration = nil
	epathOracleMTDUnitRecipe(t, input)
	// Presence of sibling bytes alone cannot create the optional requirement.
	provenance, err := epathOracleSnapshotProvenanceFor(input.root, input.evidence)
	if err != nil || provenance.MTDFile != "" || provenance.MTDSHA256 != "" {
		t.Fatalf("legacy recipe inferred CG from file presence: %+v / %v", provenance, err)
	}
}

func TestEnergyPathOracleMTDSnapshotRequiresExternalRecipeAndLiveSibling(t *testing.T) {
	for _, mutation := range []string{"", "missing both anchors", "missing digest", "wrong filename", "changed digest", "live MTD changed", "live MTD missing", "live SQL changed", "external option removed", "external system changed", "external recipe missing", "external recipe outside catalog", "MTD is directory"} {
		t.Run(mutation, func(t *testing.T) {
			input := epathOracleMTDUnitFixture(t)
			provenance, err := epathOracleSnapshotProvenanceFor(input.root, input.evidence)
			if err != nil || provenance.MTDFile != "eplusout.mtd" || len(provenance.MTDSHA256) != 64 {
				t.Fatalf("optional capture lost external MTD anchor: %+v / %v", provenance, err)
			}
			mtd := filepath.Join(input.evidence.RunDirectory, "eplusout.mtd")
			switch mutation {
			case "missing both anchors":
				provenance.MTDFile, provenance.MTDSHA256 = "", ""
			case "missing digest":
				provenance.MTDSHA256 = ""
			case "wrong filename":
				provenance.MTDFile = "other.mtd"
			case "changed digest":
				provenance.MTDSHA256 = strings.Repeat("0", 64)
			case "live MTD changed":
				epathOracleMTDUnitWrite(t, mtd, []byte("changed actual sibling bytes"))
			case "live MTD missing", "MTD is directory":
				if err := os.Remove(mtd); err != nil {
					t.Fatal(err)
				}
				if mutation == "MTD is directory" {
					if err := os.Mkdir(mtd, 0700); err != nil {
						t.Fatal(err)
					}
				}
			case "live SQL changed":
				epathOracleMTDUnitWrite(t, input.evidence.SQLPath, []byte("changed SQL bytes"))
			case "external option removed":
				input.recipe.SQLModel.PVCogeneration = nil
				epathOracleMTDUnitRecipe(t, input)
			case "external system changed":
				input.recipe.SQLModel.PVCogeneration.SystemID = "foreign"
				epathOracleMTDUnitRecipe(t, input)
			case "external recipe missing":
				input.evidence.Fixture.OraclePath = "oracles/missing.json"
			case "external recipe outside catalog":
				input.evidence.Fixture.OraclePath = "../foreign.json"
			}
			before := epathOracleMTDUnitSidecar(t, input, provenance)
			_, err = epathReadOracleSnapshot(input.root, input.candidate, input.evidence)
			if (err == nil) != (mutation == "") {
				t.Fatalf("mutation %q was not rejected by actual snapshot reload: %v", mutation, err)
			}
			after, readErr := os.ReadFile(input.candidate + ".provenance.json")
			if readErr != nil || !bytes.Equal(before, after) {
				t.Fatal("snapshot reload silently wrote or repaired an MTD anchor")
			}
		})
	}
}

func TestEnergyPathOracleMTDPendingEntryRejectsUndeclaredAnchor(t *testing.T) {
	input := epathOraclePendingUnitFixture(t)
	var provenance epathOracleSnapshotProvenance
	if err := epathDecodeOracleFile(input.candidate+".provenance.json", &provenance); err != nil {
		t.Fatal(err)
	}
	provenance.MTDFile, provenance.MTDSHA256 = "eplusout.mtd", strings.Repeat("a", 64)
	data, err := json.Marshal(provenance)
	if err != nil {
		t.Fatal(err)
	}
	epathOracleMTDUnitWrite(t, input.candidate+".provenance.json", data)
	if err := input.write(); err == nil || !strings.Contains(err.Error(), "stale/unbound candidate snapshot") {
		t.Fatalf("actual pending entry discarded optional anchor mismatch: %v", err)
	}
	if _, err := os.Stat(input.destination); !os.IsNotExist(err) {
		t.Fatal("failed MTD provenance gate produced pending material")
	}
}

func TestEnergyPathOracleMTDReviewedHeaderAndReloadBinding(t *testing.T) {
	pending := epathReviewedMetricUnitPending(t)
	sha := strings.Repeat("c", 64)
	legacyPayload, legacyDescriptor, err := epathBuildReviewedMetricPayload(pending, sha, sha)
	if err != nil {
		t.Fatal(err)
	}
	input := epathOracleMTDUnitFixture(t)
	file, digest, err := epathOraclePVCogenerationMTDForModel(input.evidence, input.recipe.SQLModel)
	if err != nil {
		t.Fatal(err)
	}
	pending.Provenance.MTDFile, pending.Provenance.MTDSHA256 = file, digest
	payload, descriptor, err := epathBuildReviewedMetricPayload(pending, sha, sha)
	if err != nil || !bytes.Equal(payload, legacyPayload) || descriptor != legacyDescriptor {
		t.Fatal("optional MTD provenance changed deterministic metric payload bytes", err)
	}
	for _, mutation := range []string{"", "missing both", "missing digest", "wrong filename", "changed digest"} {
		t.Run(mutation, func(t *testing.T) {
			manifest := epathReviewedMetricUnitHeader(pending, sha, descriptor)
			manifest.MTDFile, manifest.MTDSHA256 = file, digest
			switch mutation {
			case "missing both":
				manifest.MTDFile, manifest.MTDSHA256 = "", ""
			case "missing digest":
				manifest.MTDSHA256 = ""
			case "wrong filename":
				manifest.MTDFile = "foreign.mtd"
			case "changed digest":
				manifest.MTDSHA256 = strings.Repeat("f", 64)
			}
			directory := epathReviewedMetricUnitDirectory(t)
			path := filepath.Join(directory, pending.FixtureID+".json")
			before := epathReviewedMetricUnitWriteHeader(t, path, manifest)
			err := epathInstallReviewedMetricPayload(path, pending, sha, payload, descriptor)
			if (err == nil) != (mutation == "") {
				t.Fatalf("manual MTD header binding %q: %v", mutation, err)
			}
			if err := epathValidateExpectedPVCogenerationMTD(manifest, input.evidence, input.recipe.SQLModel); (err == nil) != (mutation == "") {
				t.Fatalf("approved/live MTD acceptance gate %q: %v", mutation, err)
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(before, after) {
				t.Fatal("install or validation rewrote manual approval header")
			}
			if mutation != "" {
				return
			}
			loaded, err := epathReadRealExpectedManifest(path)
			if err != nil || loaded.MTDFile != file || loaded.MTDSHA256 != digest {
				t.Fatal("actual approved manifest reload lost optional anchor", err)
			}
			if err := epathValidateExpectedPVCogenerationMTD(loaded, input.evidence, nil); err == nil {
				t.Fatal("approved CG anchor survived a deleted external option")
			}
			changed := input.evidence
			changed.SQLSHA256 = strings.Repeat("0", 64)
			if err := epathValidateExpectedPVCogenerationMTD(loaded, changed, input.recipe.SQLModel); err == nil {
				t.Fatal("approved CG MTD anchor borrowed unrelated external SQL")
			}
		})
	}
	for _, tc := range []struct{ file, digest string }{{"eplusout.mtd", ""}, {"", digest}, {"foreign.mtd", digest}, {file, "zz"}} {
		changed := pending
		changed.Provenance.MTDFile, changed.Provenance.MTDSHA256 = tc.file, tc.digest
		if _, _, err := epathBuildReviewedMetricPayload(changed, sha, sha); err == nil {
			t.Fatal("reviewed packager accepted malformed optional MTD fields")
		}
		manifest := epathReviewedMetricUnitHeader(pending, sha, descriptor)
		manifest.MetricPayload, manifest.Metrics = nil, pending.Metrics
		manifest.MTDFile, manifest.MTDSHA256 = tc.file, tc.digest
		path := filepath.Join(t.TempDir(), "inline.json")
		epathReviewedMetricUnitWriteHeader(t, path, manifest)
		if _, err := epathReadRealExpectedManifest(path); err == nil {
			t.Fatal("approved inline manifest loader accepted malformed MTD fields")
		}
	}
	manifest := epathReviewedMetricUnitHeader(pending, sha, descriptor)
	manifest.MTDFile, manifest.MTDSHA256 = file, digest
	epathOracleMTDUnitWrite(t, filepath.Join(input.evidence.RunDirectory, "eplusout.mtd"), []byte("later changed live sibling"))
	if err := epathValidateExpectedPVCogenerationMTD(manifest, input.evidence, input.recipe.SQLModel); err == nil {
		t.Fatal("approved MTD hash was not compared with current external sibling bytes")
	}
}
