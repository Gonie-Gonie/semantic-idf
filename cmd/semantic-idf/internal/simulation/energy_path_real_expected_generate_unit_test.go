package simulation

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// This complete but synthetic ledger tests storage/approval boundaries only.
// It is never an EnergyPlus result or a reviewed repository expected manifest.
func epathReviewedMetricUnitPending(t *testing.T) epathOraclePendingMetrics {
	t.Helper()
	sha := strings.Repeat("a", 64)
	p := epathOraclePendingMetrics{
		Schema: "semantic-idf.energy-path-oracle-pending/v1", ReviewStatus: "pending_independent_review",
		FixtureID: "unit-reviewed", Version: "25.1", ModelSHA256: sha,
		RecipePath: "synthetic/oracle.json", RecipeSHA256: sha, CandidatePath: "synthetic/candidate.json",
		CheckedGroups: append([]string(nil), epathRealOracleGroups...),
		Provenance: epathOracleSnapshotProvenance{
			Schema: "semantic-idf.energy-path-oracle-candidate/v1", CaptureDirectory: "synthetic/capture",
			CaptureSHA256: sha, SQLSHA256: sha, ExecutedSHA256: sha, EngineSHA256: sha,
			WeatherSHA256: sha, ProductionSHA256: sha, CandidateSHA256: sha,
		},
	}
	for _, group := range epathRealOracleGroups {
		for _, scope := range []string{"building", "zone"} {
			for _, period := range []string{"annual", "M1", "M12"} {
				zone := ""
				if scope == "zone" {
					zone = "Office"
				}
				m := epathRealOracleMetric{Key: strings.Join([]string{group, scope, zone, period, "literal"}, "|"), Group: group, Scope: scope, Zone: zone, Period: period, Unit: "kWh", Value: epathOracleNumber(140258.12345678901)}
				switch group {
				case "loads":
					m.Value = epathOracleNumber(0)
				case "endUses":
					m.Value = epathOracleNumber(.157 / 1501.071)
				case "carriers":
					m.Value = epathOracleNumber(1e-12)
				case "ratios":
					zero := 0
					m.Unit, m.Value, m.Status, m.Found, m.Total = "count", nil, "unavailable", &zero, &zero
				case "completeness":
					found, total := 3, 7
					m.Unit, m.Value, m.Status, m.Found, m.Total = "count", epathOracleNumber(3), "partial", &found, &total
				case "residuals":
					m.Value = epathOracleNumber(-.00000000000017)
				case "zoneAllocation":
					m.Unit, m.Value, m.Status = "%", epathOracleNumber(50), "partial"
				}
				p.Metrics = append(p.Metrics, m)
				p.Coverage.Records = append(p.Coverage.Records, epathSQLModelCoverageRecord{
					Scope: scope, Zone: zone, Period: period, Collection: "nodes", ID: group + "-" + period,
					Role: "primary", RequiredFields: []string{"value"}, Selectors: map[string][]string{"value": {m.Key}},
				})
			}
		}
	}
	// Explicit non-flow context has no numeric obligation; it cannot be silently
	// converted to a primary record with no required quantities.
	p.Coverage.Records = append(p.Coverage.Records, epathSQLModelCoverageRecord{Scope: "building", Period: "annual", Collection: "links", ID: "context-only", Role: "non-flow", Reason: "Synthetic source correspondence, not consumption"})
	p.RequiredKeysSHA256 = epathReviewedMetricUnitKeySHA(p.Metrics)
	return p
}

func epathReviewedMetricUnitKeySHA(metrics []epathRealOracleMetric) string {
	keys := make([]string, len(metrics))
	for index, metric := range metrics {
		keys[index] = metric.Key
	}
	sort.Strings(keys)
	hash := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	return hex.EncodeToString(hash[:])
}

func TestEnergyPathRealReviewedMetricPayloadDeterministicCompleteAndLossless(t *testing.T) {
	p := epathReviewedMetricUnitPending(t)
	before, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	sha := epathRealHash(before)
	compressed, descriptor, err := epathBuildReviewedMetricPayload(p, sha, sha)
	if err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 3; iteration++ {
		again, metadata, err := epathBuildReviewedMetricPayload(p, sha, sha)
		if err != nil || !bytes.Equal(compressed, again) || !reflect.DeepEqual(descriptor, metadata) {
			t.Fatalf("repeat %d changed deterministic output: %v", iteration, err)
		}
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) {
		t.Fatal("build sorted/mutated caller metrics or ledger")
	}
	if descriptor.File != p.FixtureID+".metrics.json.gz" || descriptor.Count != len(p.Metrics) || descriptor.SHA256 != epathRealHash(compressed) || descriptor.RequiredKeysSHA256 != epathReviewedMetricUnitKeySHA(p.Metrics) {
		t.Fatalf("descriptor did not bind the complete metric roster: %+v", descriptor)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	if !reader.ModTime.IsZero() || reader.Name != "" || reader.Comment != "" || len(reader.Extra) != 0 {
		t.Fatal("gzip metadata depends on time or input filename")
	}
	plain, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if descriptor.UncompressedSHA256 != epathRealHash(plain) {
		t.Fatal("uncompressed checksum is not the exact JSON bytes")
	}
	var decoded []epathRealOracleMetric
	if err := json.Unmarshal(plain, &decoded); err != nil {
		t.Fatal(err)
	}
	want := append([]epathRealOracleMetric(nil), p.Metrics...)
	sort.Slice(want, func(i, j int) bool { return want[i].Key < want[j].Key })
	if !reflect.DeepEqual(decoded, want) {
		t.Fatal("full payload changed key/identity/precision/signed residual/zero/null/status/count")
	}
	var wire []map[string]json.RawMessage
	if err := json.Unmarshal(plain, &wire); err != nil {
		t.Fatal(err)
	}
	for _, row := range wire {
		if _, present := row["value"]; !present {
			t.Fatal("unknown value was omitted instead of retaining explicit null")
		}
	}
	var best bytes.Buffer
	writer, err := gzip.NewWriterLevel(&best, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(best.Bytes(), compressed) {
		t.Fatal("payload is not the deterministic BestCompression encoding")
	}
}

func TestEnergyPathRealReviewedMetricPayloadRejectsUnboundOrIncompletePending(t *testing.T) {
	for name, mutate := range map[string]func(*epathOraclePendingMetrics){
		"wrong schema":              func(p *epathOraclePendingMetrics) { p.Schema = "semantic-idf.energy-path-real-model-expected/v1" },
		"already approved":          func(p *epathOraclePendingMetrics) { p.Approved = true },
		"acceptance claimed":        func(p *epathOraclePendingMetrics) { p.Acceptance = true },
		"provenance acceptance":     func(p *epathOraclePendingMetrics) { p.Provenance.Acceptance = true },
		"not pending review":        func(p *epathOraclePendingMetrics) { p.ReviewStatus = "approved" },
		"missing fixture":           func(p *epathOraclePendingMetrics) { p.FixtureID = "" },
		"fixture traversal":         func(p *epathOraclePendingMetrics) { p.FixtureID = "../outside" },
		"missing model digest":      func(p *epathOraclePendingMetrics) { p.ModelSHA256 = "" },
		"missing recipe digest":     func(p *epathOraclePendingMetrics) { p.RecipeSHA256 = "" },
		"missing production digest": func(p *epathOraclePendingMetrics) { p.Provenance.ProductionSHA256 = "" },
		"missing checked group":     func(p *epathOraclePendingMetrics) { p.CheckedGroups = p.CheckedGroups[1:] },
		"duplicate checked group":   func(p *epathOraclePendingMetrics) { p.CheckedGroups[0] = p.CheckedGroups[1] },
		"unknown checked group":     func(p *epathOraclePendingMetrics) { p.CheckedGroups[0] = "invented" },
		"empty coverage":            func(p *epathOraclePendingMetrics) { p.Coverage.Records = nil },
		"coverage failure": func(p *epathOraclePendingMetrics) {
			p.Coverage.Failures = []epathSQLModelFailure{{Message: "uncovered"}}
		},
		"duplicate record": func(p *epathOraclePendingMetrics) {
			p.Coverage.Records = append(p.Coverage.Records, p.Coverage.Records[0])
		},
		"primary without fields":  func(p *epathOraclePendingMetrics) { p.Coverage.Records[0].RequiredFields = nil },
		"missing field selector":  func(p *epathOraclePendingMetrics) { p.Coverage.Records[0].Selectors = nil },
		"unknown field selector":  func(p *epathOraclePendingMetrics) { p.Coverage.Records[0].Selectors["value"] = []string{"unobserved"} },
		"unexplained context":     func(p *epathOraclePendingMetrics) { p.Coverage.Records[len(p.Coverage.Records)-1].Reason = "" },
		"missing metric":          func(p *epathOraclePendingMetrics) { p.Metrics = p.Metrics[1:] },
		"duplicate metric":        func(p *epathOraclePendingMetrics) { p.Metrics = append(p.Metrics, p.Metrics[0]) },
		"invalid metric identity": func(p *epathOraclePendingMetrics) { p.Metrics[0].Scope = "global" },
		"NaN":                     func(p *epathOraclePendingMetrics) { p.Metrics[0].Value = epathOracleNumber(math.NaN()) },
		"null without status":     func(p *epathOraclePendingMetrics) { p.Metrics[0].Value = nil },
		"bad counts":              func(p *epathOraclePendingMetrics) { n := -1; p.Metrics[0].Found, p.Metrics[0].Total = &n, &n },
		"changed required digest": func(p *epathOraclePendingMetrics) { p.RequiredKeysSHA256 = strings.Repeat("b", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			p := epathReviewedMetricUnitPending(t)
			mutate(&p)
			sha := strings.Repeat("c", 64)
			if _, _, err := epathBuildReviewedMetricPayload(p, sha, sha); err == nil {
				t.Fatal("invalid pending material became a reviewed metric payload")
			}
		})
	}
	p := epathReviewedMetricUnitPending(t)
	for _, pair := range [][2]string{{"", ""}, {"wrong", "wrong"}, {strings.Repeat("a", 64), strings.Repeat("b", 64)}} {
		if _, _, err := epathBuildReviewedMetricPayload(p, pair[0], pair[1]); err == nil {
			t.Fatalf("missing/changed explicit reviewed SHA accepted: %q/%q", pair[0], pair[1])
		}
	}
}

func epathReviewedMetricUnitHeader(p epathOraclePendingMetrics, sha string, descriptor epathRealExpectedMetricPayload) epathRealExpectedManifest {
	return epathRealExpectedManifest{Schema: "semantic-idf.energy-path-real-model-expected/v1", FixtureID: p.FixtureID, Version: p.Version, ModelSHA256: p.ModelSHA256, WeatherSHA256: p.Provenance.WeatherSHA256, Review: "Synthetic unit-only manual review of pending SHA256 " + sha, MetricPayload: &descriptor}
}

func epathReviewedMetricUnitWriteHeader(t *testing.T, path string, manifest epathRealExpectedManifest) []byte {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return data
}

func epathReviewedMetricUnitDirectory(t *testing.T) string {
	t.Helper()
	// Windows TempDir can use the user's 8.3 alias. Install intentionally accepts
	// an explicit physical path only, so do not confuse that guard with approval.
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestEnergyPathRealReviewedMetricPayloadInstallRequiresManualHeaderAndNeverOverwrites(t *testing.T) {
	p := epathReviewedMetricUnitPending(t)
	sha := strings.Repeat("c", 64)
	payload, descriptor, err := epathBuildReviewedMetricPayload(p, sha, sha)
	if err != nil {
		t.Fatal(err)
	}
	directory := epathReviewedMetricUnitDirectory(t)
	path := filepath.Join(directory, p.FixtureID+".json")
	if err := epathInstallReviewedMetricPayload(path, p, sha, payload, descriptor); err == nil {
		t.Fatal("install invented a missing approved manifest")
	}
	if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 {
		t.Fatalf("missing approval created files: %v / %v", entries, err)
	}
	header := epathReviewedMetricUnitWriteHeader(t, path, epathReviewedMetricUnitHeader(p, sha, descriptor))
	if err := epathInstallReviewedMetricPayload(path, p, sha, payload, descriptor); err != nil {
		t.Fatal(err)
	}
	companion := filepath.Join(directory, descriptor.File)
	got, err := os.ReadFile(companion)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("installed payload bytes differ: %v", err)
	}
	loaded, err := epathReadRealExpectedManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]epathRealOracleMetric(nil), p.Metrics...)
	sort.Slice(want, func(i, j int) bool { return want[i].Key < want[j].Key })
	if !reflect.DeepEqual(loaded.Metrics, want) {
		t.Fatal("actual manifest loader failed complete metric roundtrip")
	}
	if err := epathInstallReviewedMetricPayload(path, p, sha, payload, descriptor); err == nil {
		t.Fatal("second install overwrote existing companion")
	}
	for file, expected := range map[string][]byte{path: header, companion: payload} {
		actual, err := os.ReadFile(file)
		if err != nil || !bytes.Equal(actual, expected) {
			t.Fatalf("install changed approved/header/existing payload bytes %s: %v", file, err)
		}
	}
	if entries, err := os.ReadDir(directory); err != nil || len(entries) != 2 {
		t.Fatalf("unexpected write outside the one companion: %v / %v", entries, err)
	}
}

func TestEnergyPathRealReviewedMetricPayloadInstallRejectsChangedApprovalOrPayload(t *testing.T) {
	for _, name := range []string{"schema", "review missing", "different reviewed SHA", "fixture", "version", "model", "weather", "inline metrics", "missing descriptor", "descriptor count", "descriptor digest", "descriptor name", "payload corruption", "passed descriptor", "header basename"} {
		t.Run(name, func(t *testing.T) {
			p := epathReviewedMetricUnitPending(t)
			sha := strings.Repeat("c", 64)
			payload, descriptor, err := epathBuildReviewedMetricPayload(p, sha, sha)
			if err != nil {
				t.Fatal(err)
			}
			manifest := epathReviewedMetricUnitHeader(p, sha, descriptor)
			directory := epathReviewedMetricUnitDirectory(t)
			path := filepath.Join(directory, p.FixtureID+".json")
			switch name {
			case "schema":
				manifest.Schema = "pending"
			case "review missing":
				manifest.Review = ""
			case "different reviewed SHA":
				manifest.Review = strings.Repeat("b", 64)
			case "fixture":
				manifest.FixtureID = "other"
			case "version":
				manifest.Version = "24.2"
			case "model":
				manifest.ModelSHA256 = strings.Repeat("b", 64)
			case "weather":
				manifest.WeatherSHA256 = strings.Repeat("b", 64)
			case "inline metrics":
				manifest.Metrics = p.Metrics
			case "missing descriptor":
				manifest.MetricPayload = nil
			case "descriptor count":
				manifest.MetricPayload.Count--
			case "descriptor digest":
				manifest.MetricPayload.SHA256 = strings.Repeat("b", 64)
			case "descriptor name":
				manifest.MetricPayload.File = "../outside.metrics.json.gz"
			case "payload corruption":
				payload[len(payload)/2] ^= 1
			case "passed descriptor":
				descriptor.UncompressedSHA256 = strings.Repeat("b", 64)
			case "header basename":
				path = filepath.Join(directory, "other.json")
			}
			header := epathReviewedMetricUnitWriteHeader(t, path, manifest)
			if err := epathInstallReviewedMetricPayload(path, p, sha, payload, descriptor); err == nil {
				t.Fatal("changed/unapproved payload installed")
			}
			actual, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(actual, header) {
				t.Fatalf("failed install rewrote the manual header: %v", err)
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 1 {
				t.Fatalf("failed install created files: %v / %v", entries, err)
			}
		})
	}
}

func TestEnergyPathRealReviewedMetricPayloadInstallRejectsSymlinkHeaderOrCompanion(t *testing.T) {
	for _, kind := range []string{"header", "companion"} {
		t.Run(kind, func(t *testing.T) {
			p := epathReviewedMetricUnitPending(t)
			sha := strings.Repeat("c", 64)
			payload, descriptor, err := epathBuildReviewedMetricPayload(p, sha, sha)
			if err != nil {
				t.Fatal(err)
			}
			directory, outside := epathReviewedMetricUnitDirectory(t), epathReviewedMetricUnitDirectory(t)
			path := filepath.Join(directory, p.FixtureID+".json")
			manifest := epathReviewedMetricUnitHeader(p, sha, descriptor)
			var protected string
			var original []byte
			if kind == "header" {
				protected = filepath.Join(outside, p.FixtureID+".json")
				original = epathReviewedMetricUnitWriteHeader(t, protected, manifest)
				if err := os.Symlink(protected, path); err != nil {
					t.Skipf("OS does not permit this symlink fixture: %v", err)
				}
			} else {
				epathReviewedMetricUnitWriteHeader(t, path, manifest)
				protected, original = filepath.Join(outside, descriptor.File), []byte("unrelated protected payload")
				if err := os.WriteFile(protected, original, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(protected, filepath.Join(directory, descriptor.File)); err != nil {
					t.Skipf("OS does not permit this symlink fixture: %v", err)
				}
			}
			if err := epathInstallReviewedMetricPayload(path, p, sha, payload, descriptor); err == nil {
				t.Fatal("symlink redirected approved install")
			}
			got, err := os.ReadFile(protected)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatalf("install changed protected target: %v", err)
			}
		})
	}
}

func TestEnergyPathRealReviewedMetricPayloadInstallRejectsAmbiguousHeaderMembers(t *testing.T) {
	for _, name := range []string{"null inline metrics", "empty inline metrics", "duplicate schema", "conflicting schema", "duplicate descriptor file", "conflicting descriptor file"} {
		t.Run(name, func(t *testing.T) {
			p := epathReviewedMetricUnitPending(t)
			sha := strings.Repeat("c", 64)
			payload, descriptor, err := epathBuildReviewedMetricPayload(p, sha, sha)
			if err != nil {
				t.Fatal(err)
			}
			base, err := json.Marshal(epathReviewedMetricUnitHeader(p, sha, descriptor))
			if err != nil {
				t.Fatal(err)
			}
			header := string(base)
			switch name {
			case "null inline metrics":
				header = `{"metrics":null,` + header[1:]
			case "empty inline metrics":
				header = `{"metrics":[],` + header[1:]
			case "duplicate schema":
				header = `{"schema":"semantic-idf.energy-path-real-model-expected/v1",` + header[1:]
			case "conflicting schema":
				// Last-value-wins decoding would erase the contradictory first key.
				header = `{"schema":"unapproved-pending",` + header[1:]
			case "duplicate descriptor file":
				header = strings.Replace(header, `"metricPayload":{`, `"metricPayload":{"file":"`+descriptor.File+`",`, 1)
			case "conflicting descriptor file":
				header = strings.Replace(header, `"metricPayload":{`, `"metricPayload":{"file":"outside.metrics.json.gz",`, 1)
			}
			if header == string(base) {
				t.Fatal("header mutant was not applied")
			}
			directory := epathReviewedMetricUnitDirectory(t)
			path := filepath.Join(directory, p.FixtureID+".json")
			if err := os.WriteFile(path, []byte(header), 0600); err != nil {
				t.Fatal(err)
			}
			if err := epathInstallReviewedMetricPayload(path, p, sha, payload, descriptor); err == nil {
				t.Fatal("ambiguous manual approval created a companion rejected by the loader")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != header {
				t.Fatalf("failed install changed the manual header: %v", err)
			}
			if entries, err := os.ReadDir(directory); err != nil || len(entries) != 1 {
				t.Fatalf("ambiguous header created files: %v / %v", entries, err)
			}
		})
	}
}
