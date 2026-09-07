package simulation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type epathOracleSnapshotProvenance struct {
	Schema           string `json:"schema"`
	CaptureDirectory string `json:"captureDirectory"`
	CaptureSHA256    string `json:"captureSHA256"`
	SQLSHA256        string `json:"sqlSHA256"`
	ExecutedSHA256   string `json:"executedSHA256"`
	EngineSHA256     string `json:"engineSHA256"`
	WeatherSHA256    string `json:"weatherSHA256"`
	ProductionSHA256 string `json:"productionSHA256"`
	CandidateSHA256  string `json:"candidateSHA256"`
	Acceptance       bool   `json:"acceptance"`
}

func epathOracleHashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
func epathOracleProductionDigest(root string) (string, error) {
	paths := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".runtime", "node_modules", "build", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") || path == filepath.Join(root, "go.mod") || path == filepath.Join(root, "go.sum") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	digest := sha256.New()
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		hash, err := epathOracleHashFile(path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(digest, "%s\x00%s\n", filepath.ToSlash(relative), hash)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func epathOracleSnapshotProvenanceFor(root string, evidence epathRealRunEvidence) (epathOracleSnapshotProvenance, error) {
	var out epathOracleSnapshotProvenance
	captureHash, err := epathOracleHashFile(filepath.Join(evidence.RunDirectory, "run-evidence.json"))
	if err != nil {
		return out, err
	}
	codeHash, err := epathOracleProductionDigest(root)
	if err != nil {
		return out, err
	}
	out = epathOracleSnapshotProvenance{Schema: "semantic-idf.energy-path-oracle-candidate/v1", CaptureDirectory: evidence.RunDirectory, CaptureSHA256: captureHash, SQLSHA256: evidence.SQLSHA256, ExecutedSHA256: evidence.ExecutedSHA256, EngineSHA256: evidence.EngineSHA256, WeatherSHA256: evidence.WeatherSHA256, ProductionSHA256: codeHash, Acceptance: false}
	return out, nil
}

func epathOracleSnapshotDestination(root, path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(filepath.Join(root, ".runtime"), absolute)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("candidate snapshot destination must be a new explicitly named .runtime file")
	}
	for _, candidate := range []string{absolute, absolute + ".provenance.json"} {
		if _, err := os.Stat(candidate); err == nil {
			return "", fmt.Errorf("snapshot already exists; never overwrite %s", candidate)
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	return absolute, nil
}

func epathWriteOracleSnapshot(path string, bundle PurposeResultBundle, provenance epathOracleSnapshotProvenance) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(file).Encode(bundle); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	provenance.CandidateSHA256, err = epathOracleHashFile(path)
	if err != nil {
		return err
	}
	sidecar, err := os.OpenFile(path+".provenance.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer sidecar.Close()
	return json.NewEncoder(sidecar).Encode(provenance)
}

func epathReadOracleSnapshot(root, path string, evidence epathRealRunEvidence) (PurposeResultBundle, error) {
	var bundle PurposeResultBundle
	var recorded epathOracleSnapshotProvenance
	if err := epathDecodeOracleFile(path+".provenance.json", &recorded); err != nil {
		return bundle, err
	}
	actual, err := epathOracleSnapshotProvenanceFor(root, evidence)
	if err != nil {
		return bundle, err
	}
	hash, err := epathOracleHashFile(path)
	if err != nil {
		return bundle, err
	}
	actual.CandidateSHA256 = hash
	if recorded != actual {
		return bundle, fmt.Errorf("stale/unbound oracle snapshot: production/capture/input/engine/weather/SQL/candidate identity mismatch")
	}
	file, err := os.Open(path)
	if err != nil {
		return bundle, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&bundle); err != nil {
		return bundle, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return bundle, fmt.Errorf("trailing candidate JSON")
	}
	if bundle.EnergyExplanation.Schema != energyExplanationSchema || bundle.EnergyExplanation.Scope.Kind != "building" {
		return bundle, fmt.Errorf("snapshot must contain original Building canonical v2 result")
	}
	return bundle, nil
}

func TestEnergyPathRealOracleMaterializeCandidate(t *testing.T) {
	directory, destination := os.Getenv("EPATH_REAL_ORACLE_CAPTURE_DIR"), os.Getenv("EPATH_REAL_ORACLE_SNAPSHOT_NEW")
	if directory == "" && destination == "" {
		t.Skip("explicit saved capture and new .runtime snapshot required; not acceptance")
	}
	if directory == "" || destination == "" || os.Getenv("EPATH_REAL_RUN") == "1" || os.Getenv("EPATH_REAL_CAPTURE") == "1" {
		t.Fatal("materialize mode requires both paths and cannot run an engine")
	}
	root, catalog := epathRealDirectories(t)
	var evidence epathRealRunEvidence
	file, err := os.Open(filepath.Join(directory, "run-evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = json.NewDecoder(file).Decode(&evidence)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !epathRealSamePath(directory, evidence.RunDirectory) {
		t.Fatal("capture directory mismatch")
	}
	if err := epathValidateSavedRealEvidence(root, catalog, evidence); err != nil {
		t.Fatal(err)
	}
	path, err := epathOracleSnapshotDestination(root, destination)
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := epathOracleSnapshotProvenanceFor(root, evidence)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := LoadEnergyPathProjection(EnergyPathProjectionRequest{ResultPath: evidence.SQLPath, InputPath: evidence.InputPath, Scope: "building", Period: "annual", Service: "all"})
	if err != nil {
		t.Fatal(err)
	}
	after, err := epathOracleSnapshotProvenanceFor(root, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if after != provenance {
		t.Fatal("production/input provenance changed during canonical rebuild")
	}
	if err := epathWriteOracleSnapshot(path, projection.PurposeResults, provenance); err != nil {
		t.Fatal(err)
	}
	if err := epathValidateSavedRealEvidence(root, catalog, evidence); err != nil {
		t.Fatal(err)
	}
	t.Logf("NEW MATERIALIZED CANDIDATE ONLY, NOT ACCEPTANCE: %s (immutable original capture, SHA-bound production/input/engine/weather/SQL)", path)
}
