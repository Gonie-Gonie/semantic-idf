package simulation

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	epathExpectedCompressedLimit   int64 = 32 << 20
	epathExpectedUncompressedLimit int64 = 128 << 20
)

type epathRealExpectedMetricPayload struct {
	File               string `json:"file"`
	SHA256             string `json:"sha256"`
	UncompressedSHA256 string `json:"uncompressedSHA256"`
	Count              int    `json:"count"`
	RequiredKeysSHA256 string `json:"requiredKeysSHA256"`
}

// The descriptor is storage only. Once decoded, the existing acceptance path
// still compares every independent metric/key/status/count without a subset.
func epathReadRealExpectedManifest(path string) (epathRealExpectedManifest, error) {
	var manifest epathRealExpectedManifest
	data, err := epathExpectedReadBounded(path, epathExpectedUncompressedLimit)
	if err != nil {
		return manifest, err
	}
	fields, err := epathExpectedUniqueObject(data)
	if err != nil {
		return manifest, err
	}
	if err := epathExpectedDecodeOne(data, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Schema != "semantic-idf.energy-path-real-model-expected/v1" || strings.TrimSpace(manifest.Review) == "" {
		return manifest, fmt.Errorf("expected manifest lacks approved schema/review")
	}
	_, inline := fields["metrics"]
	_, compressed := fields["metricPayload"]
	if inline == compressed {
		return manifest, fmt.Errorf("expected manifest requires exclusively inline metrics or one metricPayload")
	}
	if compressed {
		if manifest.MetricPayload == nil {
			return manifest, fmt.Errorf("metricPayload cannot be null")
		}
		if _, err := epathExpectedUniqueObject(fields["metricPayload"]); err != nil {
			return manifest, err
		}
		manifest.Metrics, err = epathReadExpectedMetricPayload(path, *manifest.MetricPayload, epathExpectedCompressedLimit, epathExpectedUncompressedLimit)
		if err != nil {
			return manifest, err
		}
	}
	if err := epathValidateOracleMetricGroups(manifest.Metrics); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func epathExpectedReadBounded(path string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid expected payload size limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("expected artifact is not a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("expected artifact exceeds its size limit")
	}
	return data, nil
}

func epathExpectedDecodeOne(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected artifact has trailing JSON data")
	}
	return nil
}

// json.Unmarshal ordinarily accepts duplicate members. In reviewed headers and
// metric entries, even two equal values are ambiguous and must be rejected.
func epathExpectedUniqueObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, fmt.Errorf("expected non-null JSON object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("invalid JSON member name")
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, fmt.Errorf("duplicate JSON member %q", key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return nil, fmt.Errorf("unterminated JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("JSON object has trailing data")
	}
	return fields, nil
}

func epathExpectedCompanionPath(manifestPath, name string) (string, error) {
	if len(name) <= len(".metrics.json.gz") || !strings.HasSuffix(name, ".metrics.json.gz") || name[0] == '.' || filepath.Base(name) != name {
		return "", fmt.Errorf("metric payload must be a simple companion .metrics.json.gz basename")
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return "", fmt.Errorf("invalid metric payload companion name")
		}
	}
	abs, err := filepath.Abs(manifestPath)
	if err != nil {
		return "", err
	}
	directory, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	payload, err := filepath.EvalSymlinks(filepath.Join(directory, name))
	if err != nil {
		return "", err
	}
	if !epathRealSamePath(filepath.Dir(payload), directory) {
		return "", fmt.Errorf("metric payload escapes its physical expected directory")
	}
	return payload, nil
}

func epathExpectedCheckSHA(data []byte, expected string) error {
	decoded, err := hex.DecodeString(expected)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("expected payload requires an exact SHA-256")
	}
	actual := sha256.Sum256(data)
	if !bytes.Equal(actual[:], decoded) {
		return fmt.Errorf("expected payload checksum mismatch")
	}
	return nil
}

func epathExpectedMetricKeysSHA256(metrics []epathRealOracleMetric) (string, error) {
	if err := epathValidateOracleMetricGroups(metrics); err != nil {
		return "", err
	}
	keys := make([]string, len(metrics))
	for index, metric := range metrics {
		keys[index] = metric.Key
	}
	sort.Strings(keys)
	hash := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	return hex.EncodeToString(hash[:]), nil
}

func epathReadExpectedMetricPayload(manifestPath string, descriptor epathRealExpectedMetricPayload, compressedLimit, uncompressedLimit int64) ([]epathRealOracleMetric, error) {
	if descriptor.Count <= 0 || descriptor.Count > 1_000_000 || compressedLimit <= 0 || uncompressedLimit <= 0 {
		return nil, fmt.Errorf("invalid expected metric count/size limit")
	}
	path, err := epathExpectedCompanionPath(manifestPath, descriptor.File)
	if err != nil {
		return nil, err
	}
	compressed, err := epathExpectedReadBounded(path, compressedLimit)
	if err != nil {
		return nil, err
	}
	if err := epathExpectedCheckSHA(compressed, descriptor.SHA256); err != nil {
		return nil, err
	}
	input := bytes.NewReader(compressed)
	reader, err := gzip.NewReader(input)
	if err != nil {
		return nil, err
	}
	reader.Multistream(false)
	data, readErr := io.ReadAll(io.LimitReader(reader, uncompressedLimit+1))
	closeErr := reader.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > uncompressedLimit {
		return nil, fmt.Errorf("uncompressed expected metrics exceed size limit")
	}
	if input.Len() != 0 {
		return nil, fmt.Errorf("expected metric gzip has another member or trailing bytes")
	}
	if err := epathExpectedCheckSHA(data, descriptor.UncompressedSHA256); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('[') {
		return nil, fmt.Errorf("compressed metrics require a non-null JSON array")
	}
	metrics := []epathRealOracleMetric{}
	for decoder.More() {
		if len(metrics) >= descriptor.Count {
			return nil, fmt.Errorf("metric payload contains more than the approved count")
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		fields, err := epathExpectedUniqueObject(raw)
		if err != nil {
			return nil, err
		}
		if _, present := fields["value"]; !present {
			return nil, fmt.Errorf("compressed metric omits explicit value; absent is not an approved null")
		}
		var metric epathRealOracleMetric
		if err := epathExpectedDecodeOne(raw, &metric); err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim(']') {
		return nil, fmt.Errorf("unterminated expected metric array")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("expected metric array has trailing JSON")
	}
	if len(metrics) != descriptor.Count {
		return nil, fmt.Errorf("metric payload count differs from approved count")
	}
	keys, err := epathExpectedMetricKeysSHA256(metrics)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(keys, descriptor.RequiredKeysSHA256) {
		return nil, fmt.Errorf("metric payload required-key checksum mismatch")
	}
	return metrics, nil
}
