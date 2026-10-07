package simulation

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStorageInstancesValidateOwnershipAndPruneDeadSessions(t *testing.T) {
	storageTestSettings(t)
	if err := RegisterStorageInstance(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = UnregisterStorageInstance() })
	if err := RegisterStorageInstance(); err != nil {
		t.Fatal(err)
	}
	root, directory, err := storageInstanceDirectory()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	registered := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			registered++
		}
	}
	if registered != 1 {
		t.Fatalf("idempotent registration created %d records", registered)
	}
	dead := storageInstancePresence{Schema: storageInstanceSchema, Root: root, ProcessID: 2147483647, Token: strings.Repeat("ab", 16), RegisteredAt: time.Now().Format(time.RFC3339)}
	if storageProcessIsRunning(dead.ProcessID) {
		t.Fatal("fixture PID unexpectedly names a live process")
	}
	path := filepath.Join(directory, storageInstanceFilename(dead))
	payload, _ := json.Marshal(dead)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := WithStorageCleanupGate(func() error { called = true; return nil }); err != nil || !called {
		t.Fatalf("dead session blocked cleanup: called=%v error=%v", called, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("verified dead session was not pruned: %v", err)
	}
	unknown := filepath.Join(directory, "unexpected.json")
	if err := os.WriteFile(unknown, []byte(`{"processId":2147483647}`), 0o600); err != nil {
		t.Fatal(err)
	}
	called = false
	if err := WithStorageCleanupGate(func() error { called = true; return nil }); err == nil || called {
		t.Fatalf("unknown registry record allowed deletion: called=%v error=%v", called, err)
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatalf("unknown registry record was removed: %v", err)
	}
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
	}
	callbackError := errors.New("fixture cleanup error")
	if err := WithStorageCleanupGate(func() error { return callbackError }); !errors.Is(err, callbackError) {
		t.Fatalf("cleanup callback error was lost: %v", err)
	}
	if err := WithStorageCleanupGate(func() error { return nil }); err != nil {
		t.Fatalf("error callback did not release process gate: %v", err)
	}
	release, err := acquireStorageInstanceGate(root, directory, 0)
	if err != nil {
		t.Fatal(err)
	}
	called = false
	gateErr := WithStorageCleanupGate(func() error { called = true; return nil })
	release()
	if gateErr == nil || called {
		t.Fatalf("distinct gate handles in one process were not exclusive: called=%v error=%v", called, gateErr)
	}
	if err := UnregisterStorageInstance(); err != nil {
		t.Fatal(err)
	}
	entries, err = os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			t.Fatalf("shutdown left its own presence: %s", entry.Name())
		}
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RegisterStorageInstance(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("startup did not prune verified crash presence: %v", err)
	}
}

func TestStorageInstancesRejectLinkedRegistryAndBoundScanning(t *testing.T) {
	storageTestSettings(t)
	_, directory, err := storageInstanceDirectory()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 257; i++ {
		if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("unknown-%03d.json", i)), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	called := false
	if err := WithStorageCleanupGate(func() error { called = true; return nil }); err == nil || called || !strings.Contains(err.Error(), "scan limit") {
		t.Fatalf("unbounded or incomplete scan allowed cleanup: called=%v error=%v", called, err)
	}
	// The fixture root itself is a reparse/symlink boundary, so registration must
	// reject it before adding any session file to the external destination.
	other := t.TempDir()
	link := filepath.Join(t.TempDir(), "linked-local-data")
	if err := os.Symlink(other, link); err != nil {
		t.Logf("symlink boundary probe unavailable: %v", err)
		return
	}
	t.Setenv("LOCALAPPDATA", link)
	if err := RegisterStorageInstance(); err == nil {
		t.Fatal("linked registry root was accepted")
	}
	if entries, err := os.ReadDir(other); err != nil || len(entries) != 0 {
		t.Fatalf("registration wrote through linked root: entries=%v error=%v", entries, err)
	}
}

func TestStorageInstancesSerializeAppPresenceAcrossProcesses(t *testing.T) {
	if mode := os.Getenv("SEMANTIC_IDF_STORAGE_INSTANCE_CHILD"); mode != "" {
		wait := func() error {
			fmt.Println("storage-child-ready")
			_, err := bufio.NewReader(os.Stdin).ReadString('\n')
			return err
		}
		if mode == "presence" {
			if err := RegisterStorageInstance(); err != nil {
				t.Fatal(err)
			}
			defer UnregisterStorageInstance()
			if err := wait(); err != nil {
				t.Fatal(err)
			}
		} else if mode == "gate" {
			if err := WithStorageCleanupGate(wait); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Fatalf("unknown child mode %q", mode)
		}
		return
	}
	storageTestSettings(t)
	if err := RegisterStorageInstance(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = UnregisterStorageInstance() })
	child := startStorageInstanceChild(t, "presence")
	called := false
	if err := WithStorageCleanupGate(func() error { called = true; return nil }); err == nil || called || !strings.Contains(err.Error(), "another app instance") {
		t.Fatalf("live consumer process was not protected: called=%v error=%v", called, err)
	}
	child.finish(t)
	if err := WithStorageCleanupGate(func() error { return nil }); err != nil {
		t.Fatalf("closed app presence blocked cleanup: %v", err)
	}
	if err := UnregisterStorageInstance(); err != nil {
		t.Fatal(err)
	}
	gate := startStorageInstanceChild(t, "gate")
	called = false
	started := time.Now()
	if err := WithStorageCleanupGate(func() error { called = true; return nil }); err == nil || called || time.Since(started) > time.Second {
		t.Fatalf("cross-process cleanup gate did not fail promptly: called=%v elapsed=%v error=%v", called, time.Since(started), err)
	}
	registered := make(chan error, 1)
	go func() { registered <- RegisterStorageInstance() }()
	select {
	case err := <-registered:
		t.Fatalf("new app registered during active cleanup: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	gate.finish(t)
	select {
	case err := <-registered:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("new app did not register after cleanup finished")
	}
	if err := WithStorageCleanupGate(func() error { return nil }); err != nil {
		t.Fatalf("released cross-process gate remained locked: %v", err)
	}
}

type storageInstanceChild struct {
	command *exec.Cmd
	input   io.WriteCloser
	done    bool
}

func startStorageInstanceChild(t *testing.T, mode string) *storageInstanceChild {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, executable, "-test.run=^TestStorageInstancesSerializeAppPresenceAcrossProcesses$")
	command.Env = append(os.Environ(), "SEMANTIC_IDF_STORAGE_INSTANCE_CHILD="+mode)
	command.Stderr = os.Stderr
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	child := &storageInstanceChild{command: command, input: input}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !child.done {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "storage-child-ready" {
		t.Fatalf("child storage setup failed: output=%q error=%v", line, err)
	}
	return child
}

func (child *storageInstanceChild) finish(t *testing.T) {
	t.Helper()
	if _, err := fmt.Fprintln(child.input); err != nil {
		t.Fatal(err)
	}
	_ = child.input.Close()
	if err := child.command.Wait(); err != nil {
		t.Fatalf("storage child did not exit cleanly: %v", err)
	}
	child.done = true
}
