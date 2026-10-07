package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSettingsInputPreferenceRoundTripPreservesProfileTimeView(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	app := NewApp()
	settings := defaultAppSettings()
	settings.Appearance.DefaultInputView = " TABLE "
	settings.Profile.TimeView = "duration"
	if _, err := app.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := app.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Settings.Appearance.DefaultInputView != "table" || loaded.Settings.Profile.TimeView != "duration" {
		t.Fatalf("saved preferences were lost: %#v", loaded.Settings)
	}
	settings.Appearance.DefaultInputView = "obsolete"
	if got := normalizeAppSettings(settings).Appearance.DefaultInputView; got != "text" {
		t.Fatalf("invalid input preference = %q", got)
	}
	if defaultAppSettings().Storage.AutoClean {
		t.Fatal("automatic cleanup must require an explicit setting")
	}
}

func TestConcurrentSettingsWritesKeepCompleteJSONAndLeaveNoTemporaryFiles(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	app := NewApp()
	initial, err := app.SaveSettings(defaultAppSettings())
	if err != nil {
		t.Fatal(err)
	}
	errors := make(chan error, 16)
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for iteration := 0; iteration < 6; iteration++ {
				settings := defaultAppSettings()
				settings.Appearance.GraphFontSize = 9 + worker
				if _, err := app.SaveSettings(settings); err != nil {
					errors <- err
					return
				}
				if _, err := app.GetSettings(); err != nil {
					errors <- err
					return
				}
			}
		}(worker)
	}
	// A separate process does not share settingsFileMu. Read the actual file
	// directly to exercise replacement rather than only the app's mutex.
	workers.Add(1)
	go func() {
		defer workers.Done()
		for iteration := 0; iteration < 100; iteration++ {
			payload, err := readSettingsFile(initial.Path)
			if err != nil || !json.Valid(payload) {
				errors <- fmt.Errorf("settings reader observed incomplete JSON: %v", err)
				return
			}
		}
	}()
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(initial.Path), ".settings-*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary settings files remain: %v (%v)", leftovers, err)
	}
}

func TestStorageHTTPMethodsAndEmptyManagedRoot(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	handler := appAssetHandler(NewApp())
	for _, test := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodGet, "/api/storage", "", http.StatusOK},
		{http.MethodPost, "/api/storage", "{}", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/storage/clean", "", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/storage/input", "", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/storage/input", "{", http.StatusBadRequest},
		{http.MethodPost, "/api/storage/input", `{"path":""}`, http.StatusOK},
		{http.MethodPost, "/api/storage/clean", "{", http.StatusBadRequest},
		{http.MethodPost, "/api/storage/clean", `{"olderThanDays":0}`, http.StatusOK},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if response.Code != test.status {
			t.Fatalf("%s %s: status %d, want %d: %s", test.method, test.path, response.Code, test.status, response.Body)
		}
		if test.status == http.StatusOK && !json.Valid(response.Body.Bytes()) {
			t.Fatalf("invalid storage response: %s", response.Body)
		}
	}
}
