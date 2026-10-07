package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

const cliStorageProbeEnvironment = "SEMANTIC_IDF_CLI_STORAGE_PROBE"

func TestMain(m *testing.M) {
	// All CLI fixture reads now participate in storage protection. Never let a
	// package test create or prune records in the real desktop's cache directory.
	if os.Getenv(cliStorageProbeEnvironment) != "" {
		os.Exit(m.Run())
	}
	directory, err := os.MkdirTemp("", "semantic-idf-cli-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, name := range []string{"LOCALAPPDATA", "APPDATA", "XDG_CACHE_HOME"} {
		if err := os.Setenv(name, directory); err != nil {
			fmt.Fprintln(os.Stderr, err)
			_ = os.RemoveAll(directory)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(directory)
	os.Exit(code)
}

func TestCLIStorageFilesystemInputsRemainUserOwned(t *testing.T) {
	for _, command := range []string{"metrics", "convert", "batch-metrics"} {
		t.Run(command, func(t *testing.T) {
			isolateCLIStorage(t)
			paths := []string{managedCLIStorageInput(t, "selected-model")}
			if command == "batch-metrics" {
				paths = append(paths, managedCLIStorageInput(t, "second-model"))
			}
			settings := simulation.DefaultSettings()
			if usage := simulation.InspectRunStorage(settings, nil); usage.ReclaimableRunCount != len(paths) {
				t.Fatalf("fixture inputs are not disposable app runs before selection: %+v", usage)
			}
			args := []string{command, "-format", "text"}
			if command == "convert" {
				args = []string{command, "-to", "idf"}
			}
			args = append(args, paths...)
			var stderr bytes.Buffer
			output := &cliStorageCheckingWriter{check: func() { assertCLIStorageGate(t, true) }}
			if code := runCLI(args, strings.NewReader(""), output, &stderr, "test"); code != 0 || output.Len() == 0 {
				t.Fatalf("CLI filesystem command failed: code=%d, stderr=%s", code, stderr.String())
			}
			assertCLIStorageGate(t, false)
			var cleaned simulation.RunStorageCleanupResult
			if err := simulation.WithStorageCleanupGate(func() error {
				var err error
				cleaned, err = simulation.CleanRunStorage(settings, simulation.RunStorageCleanupRequest{}, nil)
				return err
			}); err != nil || cleaned.RemovedRunCount != 0 {
				t.Fatalf("cleanup removed a CLI-selected source: %+v, %v", cleaned, err)
			}
			for _, path := range paths {
				content, err := os.ReadFile(path)
				if err != nil || string(content) != cliFixtureIDF {
					t.Fatalf("CLI-selected model was not retained unchanged: %s, %v", path, err)
				}
				marker, err := os.ReadFile(filepath.Join(filepath.Dir(path), ".semantic-idf-storage.json"))
				if err != nil || !bytes.Contains(marker, []byte(`"managed":false`)) || !bytes.Contains(marker, []byte(`"protectedReason":"user_input"`)) {
					t.Fatalf("CLI input was not permanently promoted to user data: %s, %v", marker, err)
				}
			}
		})
	}
}

func TestCLIStorageStdinAndHelpAvoidRegistration(t *testing.T) {
	isolateCLIStorage(t)
	for _, args := range [][]string{{"help"}, {"version"}, {"metrics", "--help"}, {"energy-path", "--help"}, {"metrics", "-format", "text", "-"}} {
		var stdout, stderr bytes.Buffer
		if code := runCLI(args, strings.NewReader(cliFixtureIDF), &stdout, &stderr, "test"); code != 0 {
			t.Fatalf("CLI non-filesystem command failed: %v: %d, %s", args, code, stderr.String())
		}
		path := filepath.Join(filepath.Dir(simulation.DefaultSettings().RunDirectory), "storage-sessions")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stdin/help created a storage registry: %v, %s, %v", args, path, err)
		}
	}
}

func TestCLIStorageInputAliasesPreserveActualModel(t *testing.T) {
	for _, kind := range []string{"directory", "file"} {
		t.Run(kind, func(t *testing.T) {
			isolateCLIStorage(t)
			input := managedCLIStorageInput(t, "selected-through-alias")
			alias := filepath.Join(t.TempDir(), "selected-model")
			target := filepath.Dir(input)
			if kind == "file" {
				target = input
				alias += ".idf"
			}
			err := os.Symlink(target, alias)
			if err != nil && kind == "directory" && runtime.GOOS == "windows" {
				output, junctionErr := exec.Command("cmd", "/c", "mklink", "/J", alias, target).CombinedOutput()
				if junctionErr != nil {
					t.Skipf("directory aliases are unavailable: %v, %s", junctionErr, output)
				}
				err = nil
			}
			if err != nil {
				t.Skipf("file aliases are unavailable: %v", err)
			}
			t.Cleanup(func() { _ = os.Remove(alias) })
			path := alias
			if kind == "directory" {
				path = filepath.Join(alias, "model.idf")
			}
			var stdout, stderr bytes.Buffer
			if code := runCLI([]string{"convert", "-to", "idf", path}, strings.NewReader(""), &stdout, &stderr, "test"); code != 0 {
				t.Fatalf("CLI alias source failed: %d, %s", code, stderr.String())
			}
			settings := simulation.DefaultSettings()
			if usage := simulation.InspectRunStorage(settings, nil); usage.ReclaimableRunCount != 0 || usage.ProtectedRunCount != 1 {
				t.Fatalf("actual source target remained disposable through its alias: %+v", usage)
			}
			var result simulation.RunStorageCleanupResult
			if err := simulation.WithStorageCleanupGate(func() error {
				var err error
				result, err = simulation.CleanRunStorage(settings, simulation.RunStorageCleanupRequest{}, nil)
				return err
			}); err != nil || result.RemovedRunCount != 0 {
				t.Fatalf("cleanup removed actual aliased source: %+v, %v", result, err)
			}
			if data, err := os.ReadFile(input); err != nil || string(data) != cliFixtureIDF {
				t.Fatalf("actual source was not retained unchanged: %v", err)
			}
		})
	}
}

func TestCLIStorageConcurrentEnergyPathKeepsOtherCommandProtected(t *testing.T) {
	isolateCLIStorage(t)
	input := filepath.Join(t.TempDir(), "ordinary-model.idf")
	if err := os.WriteFile(input, []byte(cliFixtureIDF), 0o600); err != nil {
		t.Fatal(err)
	}
	writer := &cliStorageHoldingWriter{began: make(chan struct{}), resume: make(chan struct{})}
	defer writer.release()
	done := make(chan int, 1)
	go func() {
		done <- runCLI([]string{"metrics", "-format", "text", input}, strings.NewReader(""), writer, io.Discard, "test")
	}()
	select {
	case <-writer.began:
	case code := <-done:
		t.Fatalf("filesystem command ended before export was held: %d", code)
	case <-time.After(5 * time.Second):
		t.Fatal("filesystem command did not reach its export")
	}
	assertCLIStorageGate(t, true)
	// A separate Energy Path lifetime must not unregister the ongoing command,
	// including when its loader fails after acquiring process protection.
	if code := runCLI([]string{"energy-path", filepath.Join(t.TempDir(), "missing.sql")}, strings.NewReader(""), io.Discard, io.Discard, "test"); code == 0 {
		t.Fatal("missing Energy Path fixture unexpectedly loaded")
	}
	assertCLIStorageGate(t, true)
	writer.release()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("held filesystem command failed: %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("filesystem command did not finish after releasing its export")
	}
	assertCLIStorageGate(t, false)
}

func TestCLIStoragePresenceProbe(t *testing.T) {
	if os.Getenv(cliStorageProbeEnvironment) == "" {
		return
	}
	err := simulation.WithStorageCleanupGate(func() error { return nil })
	if err == nil {
		fmt.Println("CLI_STORAGE_GATE_FREE")
	} else if strings.Contains(err.Error(), "another app instance is open") {
		fmt.Println("CLI_STORAGE_GATE_BUSY")
	} else {
		t.Fatalf("storage probe failed unexpectedly: %v", err)
	}
}

func assertCLIStorageGate(t *testing.T, busy bool) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestCLIStoragePresenceProbe$")
	command.Env = append(os.Environ(), cliStorageProbeEnvironment+"=1")
	output, err := command.CombinedOutput()
	expected := "CLI_STORAGE_GATE_FREE"
	if busy {
		expected = "CLI_STORAGE_GATE_BUSY"
	}
	if err != nil || !strings.Contains(string(output), expected) {
		t.Fatalf("cross-process storage protection was incorrect: want=%s, err=%v\n%s", expected, err, output)
	}
}

func isolateCLIStorage(t *testing.T) {
	t.Helper()
	directory := t.TempDir()
	for _, name := range []string{"LOCALAPPDATA", "APPDATA", "XDG_CACHE_HOME"} {
		t.Setenv(name, directory)
	}
}

func managedCLIStorageInput(t *testing.T, name string) string {
	t.Helper()
	root := simulation.DefaultSettings().RunDirectory
	directory := filepath.Join(root, name)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(directory, "model.idf")
	states := make(map[string]any)
	for name, content := range map[string]string{"model.idf": cliFixtureIDF, "eplusout.sql": "generated SQL fixture"} {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		states[name] = map[string]any{"size": info.Size(), "modifiedAt": info.ModTime().UnixNano()}
	}
	marker := map[string]any{
		"schema": "semantic-idf.run-storage/v1", "root": root, "directory": directory, "runId": name,
		"processId": os.Getpid(), "managed": true,
		"createdAt": time.Now().Add(-49 * time.Hour).Format(time.RFC3339), "finishedAt": time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
		"generatedFiles": []string{"model.idf", "eplusout.sql", ".semantic-idf-storage.json"}, "generatedFileState": states,
	}
	payload, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".semantic-idf-storage.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return input
}

type cliStorageCheckingWriter struct {
	bytes.Buffer
	once  sync.Once
	check func()
}

func (writer *cliStorageCheckingWriter) Write(data []byte) (int, error) {
	writer.once.Do(writer.check)
	return writer.Buffer.Write(data)
}

type cliStorageHoldingWriter struct {
	began       chan struct{}
	resume      chan struct{}
	beginOnce   sync.Once
	releaseOnce sync.Once
}

func (writer *cliStorageHoldingWriter) Write(data []byte) (int, error) {
	writer.beginOnce.Do(func() { close(writer.began) })
	<-writer.resume
	return len(data), nil
}

func (writer *cliStorageHoldingWriter) release() {
	writer.releaseOnce.Do(func() { close(writer.resume) })
}
