package simulation

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Opt-in timing of a preserved run; never launches EnergyPlus or changes its
// files. The output is the exact result JSON used by the desktop transport.
func TestSimulationResultTransportSavedReplay(t *testing.T) {
	// Decode only the existing unordered source-ID arrays. All other bytes,
	// including large numeric columns, retain their exact original tokens.
	type sourceIDSpan struct{ start, end int }
	skipSpace := func(data []byte, offset int) int {
		for offset < len(data) && bytes.IndexByte([]byte(" \t\r\n"), data[offset]) >= 0 {
			offset++
		}
		return offset
	}
	sourceIDArrays := func(data []byte) []sourceIDSpan {
		var spans []sourceIDSpan
		for offset := 0; offset < len(data); {
			quoted := bytes.IndexByte(data[offset:], '"')
			if quoted < 0 {
				break
			}
			start := offset + quoted
			offset = start + 1
			for offset < len(data) && data[offset] != '"' {
				if data[offset] == '\\' {
					offset++
				}
				offset++
			}
			if offset >= len(data) {
				t.Fatal("unterminated JSON string in transport comparison")
			}
			offset++
			if !bytes.Equal(data[start:offset], []byte(`"sourceIds"`)) {
				continue
			}
			colon := skipSpace(data, offset)
			if colon >= len(data) || data[colon] != ':' {
				continue
			}
			array := skipSpace(data, colon+1)
			if array >= len(data) || data[array] != '[' {
				continue
			}
			decoder := json.NewDecoder(bytes.NewReader(data[array:]))
			var raw json.RawMessage
			if err := decoder.Decode(&raw); err != nil {
				t.Fatalf("invalid source-ID array in transport comparison: %v", err)
			}
			offset = array + int(decoder.InputOffset())
			spans = append(spans, sourceIDSpan{array, offset})
		}
		return spans
	}
	comparableIDs := func(data []byte, span sourceIDSpan) any {
		wrapper := make([]byte, 0, span.end-span.start+14)
		wrapper = append(wrapper, `{"sourceIds":`...)
		wrapper = append(wrapper, data[span.start:span.end]...)
		wrapper = append(wrapper, '}')
		return purposeReplayComparableJSON(t, wrapper)
	}
	comparable := func(baseline, payload []byte) bool {
		if bytes.Equal(baseline, payload) {
			return true
		}
		wantSpans, gotSpans := sourceIDArrays(baseline), sourceIDArrays(payload)
		if len(wantSpans) != len(gotSpans) {
			return false
		}
		wantOffset, gotOffset := 0, 0
		for index, wantSpan := range wantSpans {
			gotSpan := gotSpans[index]
			if !bytes.Equal(baseline[wantOffset:wantSpan.start], payload[gotOffset:gotSpan.start]) || !reflect.DeepEqual(comparableIDs(baseline, wantSpan), comparableIDs(payload, gotSpan)) {
				return false
			}
			wantOffset, gotOffset = wantSpan.end, gotSpan.end
		}
		return bytes.Equal(baseline[wantOffset:], payload[gotOffset:])
	}
	t.Run("comparison", func(t *testing.T) {
		for _, test := range []struct {
			name, before, after string
			equal               bool
		}{
			{"exact bytes", `{"values":[0,-0,null,1.234567890123456789]}`, `{"values":[0,-0,null,1.234567890123456789]}`, true},
			{"source order and duplicate counts", `{"sourceIds":["b","a","a"]}`, `{"sourceIds":["a","b","a"]}`, true},
			{"escaped source values", `{"sourceIds":["q\\path","q\"quote","한글 🔥"]}`, `{"sourceIds":["한글 🔥","q\"quote","q\\path"]}`, true},
			{"nested source fields", `{"nested":[{"sourceIds":["b","a"]}],"sourceIds":["d","c"]}`, `{"nested":[{"sourceIds":["a","b"]}],"sourceIds":["c","d"]}`, true},
			{"source duplicate removed", `{"sourceIds":["b","a","a"]}`, `{"sourceIds":["a","b"]}`, false},
			{"escaped source-like string", `{"label":"\"sourceIds\":[\"b\",\"a\"]","sourceIds":["b","a"]}`, `{"label":"\"sourceIds\":[\"a\",\"b\"]","sourceIds":["a","b"]}`, false},
			{"escaped key is not sourceIds", `{"prefix\"sourceIds":["b","a"],"sourceIds":["b","a"]}`, `{"prefix\"sourceIds":["a","b"],"sourceIds":["a","b"]}`, false},
			{"other array order", `{"values":[1,2],"sourceIds":["b","a"]}`, `{"values":[2,1],"sourceIds":["a","b"]}`, false},
			{"negative zero", `{"values":[-0],"sourceIds":["b","a"]}`, `{"values":[0],"sourceIds":["a","b"]}`, false},
			{"exact decimal tokens", `{"value":0.000000000000000001,"sourceIds":["b","a"]}`, `{"value":0.000000000000000002,"sourceIds":["a","b"]}`, false},
			{"null versus zero", `{"values":[null],"sourceIds":["b","a"]}`, `{"values":[0],"sourceIds":["a","b"]}`, false},
			{"null versus missing", `{"value":null,"sourceIds":["b","a"]}`, `{"sourceIds":["a","b"]}`, false},
			{"source null versus empty", `{"sourceIds":null}`, `{"sourceIds":[]}`, false},
			{"source field moved", `{"a":{"sourceIds":["b","a"]},"b":{}}`, `{"a":{},"b":{"sourceIds":["a","b"]}}`, false},
		} {
			t.Run(test.name, func(t *testing.T) {
				before, after := []byte(test.before), []byte(test.after)
				beforeHash, afterHash := sha256.Sum256(before), sha256.Sum256(after)
				if got := comparable(before, after); got != test.equal {
					t.Fatalf("comparison equal=%v, want %v", got, test.equal)
				}
				if sha256.Sum256(before) != beforeHash || sha256.Sum256(after) != afterHash {
					t.Fatal("comparison mutated original baseline or candidate bytes")
				}
			})
		}
	})
	directory := os.Getenv("SIMULATION_TRANSPORT_REPLAY_DIR")
	if directory == "" {
		t.Skip("set SIMULATION_TRANSPORT_REPLAY_DIR and SIMULATION_TRANSPORT_REPLAY_INPUT for saved-run transport timing")
	}
	input := os.Getenv("SIMULATION_TRANSPORT_REPLAY_INPUT")
	if input == "" {
		t.Fatal("explicit preserved input path required")
	}
	before := epathReplayDirectorySnapshot(t, directory)
	t.Cleanup(func() {
		if !reflect.DeepEqual(before, epathReplayDirectorySnapshot(t, directory)) {
			t.Error("saved capture changed during transport replay")
		}
	})
	data, err := os.ReadFile(filepath.Join(directory, "semantic-idf-run-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plan PurposeRunPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	request := SimulationPurposeRequest{
		Purposes: plan.Purposes, AllocationPolicy: plan.AllocationPolicy,
		BasicEnergyDetail: plan.BasicEnergyDetail, ZoneHeatFlowDetail: plan.ZoneHeatFlowDetail,
		Scope: SimulationPurposeScope{ZoneMode: plan.ZoneMode, ZoneNames: plan.ZoneNames,
			PeriodMode: plan.PeriodMode, PeriodStart: plan.PeriodStart, PeriodEnd: plan.PeriodEnd},
	}
	result := SimulationRunResult{RunID: "transport-saved-replay", Status: "succeeded", InputPath: input,
		OutputDirectory: directory, PurposeRunPlan: &plan}
	started := time.Now()
	readSimulationOutputsWithProgress(&result, result.RunID, func(progress SimulationProgress) {
		t.Logf("%s after %s", progress.Phase, time.Since(started))
	}, input)
	t.Logf("read outputs: %s; series=%d", time.Since(started), len(result.Series))
	if !result.ERR.Completed || result.ERR.Severe != 0 || result.ERR.Fatal != 0 {
		t.Fatalf("capture did not complete successfully: %+v", result.ERR)
	}
	started = time.Now()
	bundle := buildPurposeResultBundleWithProgress(&result, request, nil)
	result.PurposeResults = &bundle
	t.Logf("build bundle: %s; loops=%d", time.Since(started), len(bundle.HVACLoops))
	for _, loop := range bundle.HVACLoops {
		points := 0
		for _, series := range loop.Series {
			points += len(series.Points)
		}
		t.Logf("loop %s: node series=%d, points=%d, components=%d", loop.Name, len(loop.Series), points, len(loop.Components))
	}
	started = time.Now()
	transport := CompactResultTransport(&result)
	t.Logf("compact column preparation: %s", time.Since(started))
	if wire, ok := transport.(*ResultTransport); ok {
		started = time.Now()
		samples := validateResultTransportBindings(t, &result, wire)
		t.Logf("verified exact X, label, value bits for %d samples across %d bindings: %s", samples, len(wire.PointBindings), time.Since(started))
		started = time.Now()
		columnSamples := validateResultTransportColumnBindings(t, &result, wire)
		t.Logf("verified exact column values/masks/labels for %d samples across %d bindings and %d unique columns: %s", columnSamples, len(wire.ColumnBindings), len(wire.ColumnSets), time.Since(started))
	}
	started = time.Now()
	payload, err := json.Marshal(transport)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("compact cache / direct HTTP serialization: %s; bytes=%d", time.Since(started), len(payload))
	if wire, ok := transport.(*ResultTransport); ok {
		points := 0
		for _, set := range wire.PointSets {
			points += len(set.Values)
		}
		t.Logf("unique point sets=%d; timelines=%d; bindings=%d; unique samples=%d", len(wire.PointSets), len(wire.Timelines), len(wire.PointBindings), points)
	}
	if output := os.Getenv("SIMULATION_TRANSPORT_REPLAY_OUTPUT"); output != "" {
		file := purposeReplayArtifact(t, directory, output)
		_, writeErr := file.Write(payload)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("save transport result: %v / %v", writeErr, closeErr)
		}
	}
	if comparison := os.Getenv("SIMULATION_TRANSPORT_REPLAY_COMPARE"); comparison != "" {
		info, err := os.Stat(comparison)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("read original transport baseline metadata: %v", err)
		}
		baseline, err := os.ReadFile(comparison)
		if err != nil {
			t.Fatal(err)
		}
		baselineHash := sha256.Sum256(baseline)
		t.Cleanup(func() {
			file, err := os.Open(comparison)
			if err != nil {
				t.Errorf("transport baseline no longer readable: %v", err)
				return
			}
			defer file.Close()
			current, statErr := file.Stat()
			hash := sha256.New()
			size, readErr := io.Copy(hash, file)
			if statErr != nil || readErr != nil || size != info.Size() || current.Size() != info.Size() || current.Mode() != info.Mode() || !current.ModTime().Equal(info.ModTime()) || !bytes.Equal(hash.Sum(nil), baselineHash[:]) {
				t.Error("original transport baseline changed during read-only comparison")
			}
		})
		if bytes.Equal(baseline, payload) {
			t.Logf("compact transport matches original baseline byte-for-byte: bytes=%d SHA256=%x", len(payload), baselineHash)
		} else {
			if !comparable(baseline, payload) {
				t.Fatalf("compact transport changed beyond source-ID ordering: baseline SHA256=%x, current SHA256=%x", baselineHash, sha256.Sum256(payload))
			}
			t.Log("all compact value tokens, metadata, observation order and provenance membership exactly match the original baseline; only unordered source-ID lists differ")
		}
	}
	if os.Getenv("SIMULATION_TRANSPORT_REPLAY_LEGACY") == "" {
		return
	}
	started = time.Now()
	callback, err := json.Marshal(map[string]any{"result": &result, "callbackid": "transport-replay"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("previous extra Wails result serialization: %s; bytes=%d", time.Since(started), len(callback))
	started = time.Now()
	scriptString, err := json.Marshal(string(callback))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("previous extra Windows Wails callback escaping: %s; bytes=%d (excludes WebView Eval)", time.Since(started), len(scriptString))
}
