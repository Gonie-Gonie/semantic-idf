package simulation

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type epathExpectedPayloadUnitInput struct {
	path            string
	manifest        epathRealExpectedManifest
	metrics         []epathRealOracleMetric
	raw, compressed []byte
}

func epathExpectedPayloadUnitSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func epathExpectedPayloadUnitGzip(t *testing.T, data []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func (input *epathExpectedPayloadUnitInput) save(t *testing.T) {
	t.Helper()
	data, err := json.Marshal(input.manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input.path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(input.path), "unit.metrics.json.gz"), input.compressed, 0600); err != nil {
		t.Fatal(err)
	}
}

func (input *epathExpectedPayloadUnitInput) repack(t *testing.T, raw []byte) {
	t.Helper()
	input.raw, input.compressed = raw, epathExpectedPayloadUnitGzip(t, raw)
	input.manifest.MetricPayload.SHA256 = epathExpectedPayloadUnitSHA(input.compressed)
	input.manifest.MetricPayload.UncompressedSHA256 = epathExpectedPayloadUnitSHA(raw)
}

func epathExpectedPayloadUnitFixture(t *testing.T) epathExpectedPayloadUnitInput {
	t.Helper()
	input := epathExpectedPayloadUnitInput{path: filepath.Join(t.TempDir(), "unit.json")}
	for _, group := range epathRealOracleGroups {
		metric := epathRealOracleMetric{Key: group + "|building||annual|literal", Group: group, Scope: "building", Period: "annual", Unit: "kWh", Value: epathOracleNumber(1.2345678901234567)}
		switch group {
		case "endUses":
			metric.Value, metric.Status = nil, "unavailable"
		case "carriers":
			metric.Value = epathOracleNumber(0)
		case "residuals":
			metric.Value = epathOracleNumber(-.0000000001234567)
		case "ratios":
			metric.Unit, metric.Value = "ratio", epathOracleNumber(4.000000000000001)
		case "completeness":
			found, total := 0, 2
			metric.Unit, metric.Value, metric.Status, metric.Found, metric.Total = "count", epathOracleNumber(0), "missing", &found, &total
		case "zoneAllocation":
			metric.Key, metric.Unit, metric.Value, metric.Status = "zoneAllocation|building||annual|zoneAllocatedPct", "%", epathOracleNumber(100.001), "overmapped"
		}
		input.metrics = append(input.metrics, metric)
	}
	keySHA, err := epathExpectedMetricKeysSHA256(input.metrics)
	if err != nil {
		t.Fatal(err)
	}
	input.manifest = epathRealExpectedManifest{Schema: "semantic-idf.energy-path-real-model-expected/v1", FixtureID: "unit", Version: "25.1", ModelSHA256: strings.Repeat("a", 64), WeatherSHA256: strings.Repeat("b", 64), Review: "Unit-only explicit review, not actual model acceptance", MetricPayload: &epathRealExpectedMetricPayload{File: "unit.metrics.json.gz", Count: len(input.metrics), RequiredKeysSHA256: keySHA}}
	raw, err := json.Marshal(input.metrics)
	if err != nil {
		t.Fatal(err)
	}
	input.repack(t, raw)
	input.save(t)
	return input
}

func TestEnergyPathRealExpectedPayloadLosslessAndInlineCompatible(t *testing.T) {
	input := epathExpectedPayloadUnitFixture(t)
	before, _ := os.ReadFile(input.path)
	loaded, err := epathReadRealExpectedManifest(input.path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Metrics, input.metrics) || !reflect.DeepEqual(loaded.MetricPayload, input.manifest.MetricPayload) {
		t.Fatal("compression changed full metrics/precision/unknown/zero/count/status")
	}
	for index := len(input.metrics) - 1; index >= 0; index-- { // Key checksum is a set identity, independent of JSON row order.
		loaded.Metrics[len(input.metrics)-1-index] = input.metrics[index]
	}
	hash, err := epathExpectedMetricKeysSHA256(loaded.Metrics)
	if err != nil || hash != input.manifest.MetricPayload.RequiredKeysSHA256 {
		t.Fatal("key digest depends on metric order")
	}
	after, _ := os.ReadFile(input.path)
	payload, _ := os.ReadFile(filepath.Join(filepath.Dir(input.path), input.manifest.MetricPayload.File))
	if !bytes.Equal(before, after) || !bytes.Equal(payload, input.compressed) {
		t.Fatal("reader changed reviewed artifacts")
	}
	input.manifest.Metrics, input.manifest.MetricPayload = input.metrics, nil
	input.save(t)
	inline, err := epathReadRealExpectedManifest(input.path)
	if err != nil || !reflect.DeepEqual(inline.Metrics, input.metrics) || inline.MetricPayload != nil {
		t.Fatalf("existing plain-inline v1 contract changed: %v", err)
	}
}

func TestEnergyPathRealExpectedPayloadRejectsDescriptorAndPathErrors(t *testing.T) {
	for name, mutate := range map[string]func(*epathExpectedPayloadUnitInput){
		"both representations":    func(i *epathExpectedPayloadUnitInput) { i.manifest.Metrics = i.metrics },
		"no representation":       func(i *epathExpectedPayloadUnitInput) { i.manifest.MetricPayload = nil },
		"no review":               func(i *epathExpectedPayloadUnitInput) { i.manifest.Review = "" },
		"wrong schema":            func(i *epathExpectedPayloadUnitInput) { i.manifest.Schema += ".unknown" },
		"wrong compressed hash":   func(i *epathExpectedPayloadUnitInput) { i.manifest.MetricPayload.SHA256 = strings.Repeat("0", 64) },
		"missing compressed hash": func(i *epathExpectedPayloadUnitInput) { i.manifest.MetricPayload.SHA256 = "" },
		"wrong uncompressed hash": func(i *epathExpectedPayloadUnitInput) {
			i.manifest.MetricPayload.UncompressedSHA256 = strings.Repeat("0", 64)
		},
		"missing uncompressed hash": func(i *epathExpectedPayloadUnitInput) { i.manifest.MetricPayload.UncompressedSHA256 = "" },
		"wrong key hash": func(i *epathExpectedPayloadUnitInput) {
			i.manifest.MetricPayload.RequiredKeysSHA256 = strings.Repeat("0", 64)
		},
		"missing key hash": func(i *epathExpectedPayloadUnitInput) { i.manifest.MetricPayload.RequiredKeysSHA256 = "" },
		"zero count":       func(i *epathExpectedPayloadUnitInput) { i.manifest.MetricPayload.Count = 0 },
		"negative count":   func(i *epathExpectedPayloadUnitInput) { i.manifest.MetricPayload.Count = -1 },
		"unbounded count":  func(i *epathExpectedPayloadUnitInput) { i.manifest.MetricPayload.Count = 1_000_001 },
		"lost row":         func(i *epathExpectedPayloadUnitInput) { i.manifest.MetricPayload.Count++ },
		"extra row":        func(i *epathExpectedPayloadUnitInput) { i.manifest.MetricPayload.Count-- },
	} {
		t.Run(name, func(t *testing.T) {
			input := epathExpectedPayloadUnitFixture(t)
			mutate(&input)
			input.save(t)
			if _, err := epathReadRealExpectedManifest(input.path); err == nil {
				t.Fatal("invalid reviewed payload descriptor accepted")
			}
		})
	}
	for _, file := range []string{"../unit.metrics.json.gz", `..\unit.metrics.json.gz`, "/unit.metrics.json.gz", `C:\unit.metrics.json.gz`, "nested/unit.metrics.json.gz", "unit:stream.metrics.json.gz", ".metrics.json.gz", "unit.json", "unit.metrics.json.gz "} {
		t.Run(file, func(t *testing.T) {
			input := epathExpectedPayloadUnitFixture(t)
			input.manifest.MetricPayload.File = file
			input.save(t)
			if _, err := epathReadRealExpectedManifest(input.path); err == nil {
				t.Fatal("companion path escaped simple same-directory contract")
			}
		})
	}
}

func TestEnergyPathRealExpectedPayloadRejectsContainerAndGzipCorruption(t *testing.T) {
	for _, name := range []string{"truncated gzip", "bad CRC", "second member", "trailing bytes", "trailing JSON", "truncated JSON", "null array", "object not array", "null metric", "missing value", "null value without status", "duplicate metric field", "unknown metric field", "duplicate metric key", "changed required key", "missing group"} {
		t.Run(name, func(t *testing.T) {
			input := epathExpectedPayloadUnitFixture(t)
			raw := append([]byte(nil), input.raw...)
			switch name {
			case "truncated gzip":
				input.compressed = input.compressed[:len(input.compressed)-1]
			case "bad CRC":
				input.compressed[len(input.compressed)-8] ^= 1
			case "second member":
				input.compressed = append(input.compressed, input.compressed...)
			case "trailing bytes":
				input.compressed = append(input.compressed, '\n')
			case "trailing JSON":
				input.repack(t, append(raw, []byte(` {}`)...))
			case "truncated JSON":
				input.repack(t, raw[:len(raw)-1])
			case "null array":
				input.repack(t, []byte("null"))
			case "object not array":
				input.repack(t, []byte("{}"))
			default:
				var rows []json.RawMessage
				if err := json.Unmarshal(raw, &rows); err != nil {
					t.Fatal(err)
				}
				switch name {
				case "null metric":
					rows[0] = json.RawMessage("null")
				case "duplicate metric field":
					rows[0] = json.RawMessage(strings.Replace(string(rows[0]), `"value":`, `"value":0,"value":`, 1))
				case "unknown metric field":
					rows[0] = json.RawMessage(strings.Replace(string(rows[0]), `{`, `{"unsupported":0,`, 1))
				default:
					var fields map[string]json.RawMessage
					if err := json.Unmarshal(rows[0], &fields); err != nil {
						t.Fatal(err)
					}
					switch name {
					case "missing value":
						delete(fields, "value")
					case "null value without status":
						fields["value"] = json.RawMessage("null")
					case "duplicate metric key":
						var other map[string]json.RawMessage
						json.Unmarshal(rows[1], &other)
						fields["key"] = other["key"]
					case "changed required key":
						fields["key"] = json.RawMessage(`"new-independent-key-not-approved"`)
					case "missing group":
						fields["group"] = json.RawMessage(`"loads"`)
					}
					rows[0], _ = json.Marshal(fields)
				}
				reencoded, err := json.Marshal(rows)
				if err != nil {
					t.Fatal(err)
				}
				input.repack(t, reencoded)
			}
			// Rebind the compressed digest so each case reaches its structural
			// guard; corruption is not tested merely as a checksum disagreement.
			input.manifest.MetricPayload.SHA256 = epathExpectedPayloadUnitSHA(input.compressed)
			input.save(t)
			if _, err := epathReadRealExpectedManifest(input.path); err == nil {
				t.Fatal("corrupt/ambiguous metric payload accepted")
			}
		})
	}
}

func TestEnergyPathRealExpectedPayloadHeaderPresenceAndEOF(t *testing.T) {
	for _, name := range []string{"metrics null and payload", "payload null", "duplicate header key", "duplicate descriptor key", "trailing object", "truncated header", "unknown header field"} {
		t.Run(name, func(t *testing.T) {
			input := epathExpectedPayloadUnitFixture(t)
			data, _ := os.ReadFile(input.path)
			switch name {
			case "metrics null and payload":
				data = []byte(strings.Replace(string(data), `{`, `{"metrics":null,`, 1))
			case "payload null":
				var fields map[string]json.RawMessage
				json.Unmarshal(data, &fields)
				fields["metricPayload"] = json.RawMessage("null")
				data, _ = json.Marshal(fields)
			case "duplicate header key":
				data = []byte(strings.Replace(string(data), `"schema":`, `"schema":"duplicate","schema":`, 1))
			case "duplicate descriptor key":
				data = []byte(strings.Replace(string(data), `"count":`, `"count":8,"count":`, 1))
			case "trailing object":
				data = append(data, []byte(" {}")...)
			case "truncated header":
				data = data[:len(data)-1]
			case "unknown header field":
				data = []byte(strings.Replace(string(data), `{`, `{"unsupported":0,`, 1))
			}
			if err := os.WriteFile(input.path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := epathReadRealExpectedManifest(input.path); err == nil {
				t.Fatal("ambiguous or incomplete expected header accepted")
			}
		})
	}
}

func TestEnergyPathRealExpectedPayloadReadBounds(t *testing.T) {
	input := epathExpectedPayloadUnitFixture(t)
	d := *input.manifest.MetricPayload
	if _, err := epathReadExpectedMetricPayload(input.path, d, int64(len(input.compressed)), int64(len(input.raw))); err != nil {
		t.Fatal(err)
	}
	if _, err := epathReadExpectedMetricPayload(input.path, d, int64(len(input.compressed)-1), int64(len(input.raw))); err == nil {
		t.Fatal("compressed read limit ignored")
	}
	if _, err := epathReadExpectedMetricPayload(input.path, d, int64(len(input.compressed)), int64(len(input.raw)-1)); err == nil {
		t.Fatal("decompressed read limit ignored")
	}
	if _, err := epathExpectedReadBounded(filepath.Dir(input.path), 100); err == nil {
		t.Fatal("directory treated as payload bytes")
	}
}

func TestEnergyPathRealExpectedPayloadRejectsPhysicalCompanionEscape(t *testing.T) {
	input := epathExpectedPayloadUnitFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.metrics.json.gz")
	if err := os.WriteFile(outside, input.compressed, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(filepath.Dir(input.path), "linked.metrics.json.gz")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("OS does not permit temporary file symlinks: %v", err)
	}
	input.manifest.MetricPayload.File = filepath.Base(link)
	input.save(t)
	if _, err := epathReadRealExpectedManifest(input.path); err == nil {
		t.Fatal("physical symlink escape accepted despite matching checksums")
	}
}
