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

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
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
	t.Run("retired input view", func(t *testing.T) {
		for _, legacyKey := range []bool{false, true} {
			saved := defaultAppSettings()
			saved.Appearance.DefaultInputView = "semantic"
			saved.Interaction.Shortcuts["inputText"] = "Ctrl+1"
			saved.Interaction.Shortcuts["inputJson"] = "Ctrl+2"
			saved.Interaction.Shortcuts["inputTable"] = "Ctrl+3"
			if legacyKey {
				saved.Interaction.Shortcuts["inputSemantic"] = "Ctrl+9"
			}
			if _, err := app.SaveSettings(saved); err != nil {
				t.Fatal(err)
			}
			loaded, err := app.GetSettings()
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Settings.Appearance.DefaultInputView != "text" {
				t.Fatal("retired input view was restored instead of Text")
			}
			shortcuts := loaded.Settings.Interaction.Shortcuts
			if _, exists := shortcuts["inputSemantic"]; exists {
				t.Fatal("retired shortcut was retained")
			}
			for key, expected := range map[string]string{"inputText": "Ctrl+1", "inputJson": "Ctrl+2", "inputTable": "Ctrl+3"} {
				if shortcuts[key] != expected {
					t.Fatalf("visible custom shortcut %s changed: %q", key, shortcuts[key])
				}
			}
		}
	})
	t.Run("setpoint preferences", func(t *testing.T) {
		settings := defaultAppSettings()
		settings.Profile.EnabledDimensions = []string{" COOLING_SETPOINT ", "heating_setpoint", "cooling_setpoint", "unknown"}
		settings.Profile.DisplayMetrics = nil
		settings.Profile.GroupingMetrics = nil
		if _, err := app.SaveSettings(settings); err != nil {
			t.Fatal(err)
		}
		loaded, err := app.GetSettings()
		if err != nil {
			t.Fatal(err)
		}
		profile := loaded.Settings.Profile
		if got := strings.Join(profile.EnabledDimensions, ","); got != "cooling_setpoint,heating_setpoint" {
			t.Fatalf("setpoint selections changed during save/load: %q", got)
		}
		for _, dimension := range []string{idf.ProfileDimensionCoolingSetpoint, idf.ProfileDimensionHeatingSetpoint} {
			if profile.DisplayMetrics[dimension] != "temperature" || profile.GroupingMetrics[dimension] != "temperature" {
				t.Fatalf("setpoint metric defaults lost for %s: %#v", dimension, profile)
			}
		}
	})
	t.Run("legacy selections", func(t *testing.T) {
		settings := defaultAppSettings()
		settings.Profile.EnabledDimensions = []string{"lighting", "occupancy"}
		delete(settings.Profile.DisplayMetrics, idf.ProfileDimensionCoolingSetpoint)
		delete(settings.Profile.DisplayMetrics, idf.ProfileDimensionHeatingSetpoint)
		delete(settings.Profile.GroupingMetrics, idf.ProfileDimensionCoolingSetpoint)
		delete(settings.Profile.GroupingMetrics, idf.ProfileDimensionHeatingSetpoint)
		if _, err := app.SaveSettings(settings); err != nil {
			t.Fatal(err)
		}
		loaded, err := app.GetSettings()
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(loaded.Settings.Profile.EnabledDimensions, ","); got != "lighting,occupancy" {
			t.Fatalf("new dimensions changed existing checked selections: %q", got)
		}
		for _, dimension := range []string{idf.ProfileDimensionCoolingSetpoint, idf.ProfileDimensionHeatingSetpoint} {
			if loaded.Settings.Profile.DisplayMetrics[dimension] != "temperature" || loaded.Settings.Profile.GroupingMetrics[dimension] != "temperature" {
				t.Fatalf("legacy preferences did not gain selectable metric defaults for %s", dimension)
			}
		}
	})
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
