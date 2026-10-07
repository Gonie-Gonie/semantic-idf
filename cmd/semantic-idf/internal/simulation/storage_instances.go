package simulation

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const storageInstanceSchema = "semantic-idf.storage-instance/v1"

type storageInstancePresence struct {
	Schema       string `json:"schema"`
	Root         string `json:"root"`
	ProcessID    int    `json:"processId"`
	Token        string `json:"token"`
	RegisteredAt string `json:"registeredAt"`
}

var storageInstanceState = struct {
	sync.Mutex
	owned map[string]storageInstancePresence
}{owned: make(map[string]storageInstancePresence)}

// Presence covers the desktop's entire lifetime, including references to old
// results produced by an already closed process. Cleanup conservatively refuses
// while another participating desktop is open. Registration and cleanup share
// an OS lock, so a new desktop cannot begin using results during deletion.
func RegisterStorageInstance() error {
	root, directory, err := storageInstanceDirectory()
	if err != nil {
		return err
	}
	storageInstanceState.Lock()
	defer storageInstanceState.Unlock()
	if presence, ok := storageInstanceState.owned[storagePathKey(root)]; ok {
		path := filepath.Join(directory, storageInstanceFilename(presence))
		var recorded storageInstancePresence
		if readStorageInstance(path, root, &recorded) == nil && recorded == presence {
			return nil
		}
	}
	release, err := acquireStorageInstanceGate(root, directory, 5*time.Second)
	if err != nil {
		return err
	}
	defer release()
	// Crashes cannot leave an unbounded ledger merely because the user never
	// invokes cleanup. Keep live/unknown records, and prune only verified dead
	// owners within the same bounded registry scan used by cleanup.
	if err := scanStorageInstances(root, directory, false); err != nil {
		return err
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return err
	}
	presence := storageInstancePresence{Schema: storageInstanceSchema, Root: root, ProcessID: os.Getpid(), Token: hex.EncodeToString(token), RegisteredAt: time.Now().Format(time.RFC3339)}
	path := filepath.Join(directory, storageInstanceFilename(presence))
	payload, err := json.Marshal(presence)
	if err != nil {
		return err
	}
	if err := validateStoragePath(directory); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(payload, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		if validateStoragePath(path) == nil {
			_ = os.Remove(path)
		}
		return writeErr
	}
	if closeErr != nil {
		if validateStoragePath(path) == nil {
			_ = os.Remove(path)
		}
		return closeErr
	}
	storageInstanceState.owned[storagePathKey(root)] = presence
	return nil
}

func UnregisterStorageInstance() error {
	root, directory, err := storageInstanceDirectory()
	if err != nil {
		return err
	}
	storageInstanceState.Lock()
	defer storageInstanceState.Unlock()
	presence, ok := storageInstanceState.owned[storagePathKey(root)]
	if !ok {
		return nil
	}
	release, err := acquireStorageInstanceGate(root, directory, 5*time.Second)
	if err != nil {
		return err
	}
	defer release()
	path := filepath.Join(directory, storageInstanceFilename(presence))
	var recorded storageInstancePresence
	if err := readStorageInstance(path, root, &recorded); err != nil {
		if os.IsNotExist(err) {
			delete(storageInstanceState.owned, storagePathKey(root))
			return nil
		}
		return err
	}
	if recorded != presence {
		return fmt.Errorf("storage instance ownership changed")
	}
	if err := validateStoragePath(path); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	delete(storageInstanceState.owned, storagePathKey(root))
	return nil
}

// The callback runs under the process gate until all deletion and reporting have
// finished. Other app instances, malformed registry records and unavailable
// locking fail closed; this never deletes an unverified session record.
func WithStorageCleanupGate(clean func() error) error {
	root, directory, err := storageInstanceDirectory()
	if err != nil {
		return err
	}
	release, err := acquireStorageInstanceGate(root, directory, 0)
	if err != nil {
		return err
	}
	defer release()
	if err := inspectStorageInstances(root, directory); err != nil {
		return err
	}
	return clean()
}

func storageInstanceDirectory() (string, string, error) {
	root, err := filepath.Abs(filepath.Dir(DefaultSettings().RunDirectory))
	if err != nil {
		return "", "", err
	}
	directory := filepath.Join(root, "storage-sessions")
	for existing := directory; ; existing = filepath.Dir(existing) {
		if _, err := os.Lstat(existing); err == nil {
			if err := validateStoragePath(existing); err != nil {
				return "", "", err
			}
			break
		} else if !os.IsNotExist(err) {
			return "", "", err
		}
		if filepath.Dir(existing) == existing {
			return "", "", fmt.Errorf("storage instance directory is unavailable")
		}
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", err
	}
	if err := validateStoragePath(directory); err != nil {
		return "", "", err
	}
	return root, directory, nil
}

func storageInstanceFilename(presence storageInstancePresence) string {
	return strconv.Itoa(presence.ProcessID) + "-" + presence.Token + ".json"
}

func readStorageInstance(path, root string, presence *storageInstancePresence) error {
	if err := validateStoragePath(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 16384 {
		return fmt.Errorf("unrecognized storage instance record")
	}
	if err := readStorageJSON(path, presence); err != nil {
		return err
	}
	token, tokenErr := hex.DecodeString(presence.Token)
	_, timeErr := time.Parse(time.RFC3339, presence.RegisteredAt)
	if presence.Schema != storageInstanceSchema || presence.ProcessID <= 0 || storagePathKey(presence.Root) != storagePathKey(root) || tokenErr != nil || len(token) != 16 || timeErr != nil || filepath.Base(path) != storageInstanceFilename(*presence) {
		return fmt.Errorf("unrecognized storage instance record")
	}
	return nil
}

func inspectStorageInstances(root, directory string) error {
	return scanStorageInstances(root, directory, true)
}

func scanStorageInstances(root, directory string, cleanup bool) error {
	if err := validateStoragePath(directory); err != nil {
		return err
	}
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	entries, err := file.ReadDir(257)
	if err != nil && err != io.EOF {
		return err
	}
	if len(entries) > 256 {
		if cleanup {
			return fmt.Errorf("storage instance scan limit reached; cleanup is unavailable")
		}
		entries = entries[:256]
	}
	for _, entry := range entries {
		if entry.Name() == ".cleanup.lock" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		var presence storageInstancePresence
		if !strings.HasSuffix(entry.Name(), ".json") || readStorageInstance(path, root, &presence) != nil {
			if cleanup {
				return fmt.Errorf("unrecognized storage instance record; cleanup is unavailable")
			}
			continue
		}
		if presence.ProcessID == os.Getpid() {
			continue
		}
		if storageProcessIsRunning(presence.ProcessID) {
			if cleanup {
				return fmt.Errorf("another app instance is open; close it before cleaning generated files")
			}
			continue
		}
		if err := validateStoragePath(path); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			if cleanup {
				return err
			}
		}
	}
	return nil
}
