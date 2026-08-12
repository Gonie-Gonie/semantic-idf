package frontendchecks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationCommandLivesOutsideRepositoryRoot(t *testing.T) {
	rootGoFiles, err := filepath.Glob(repoPath("../../*.go"))
	if err != nil {
		t.Fatalf("glob root Go files: %v", err)
	}
	if len(rootGoFiles) != 0 {
		t.Fatalf("repository root contains Go sources: %v", rootGoFiles)
	}
	for _, removedRoot := range []string{"../../frontend", "../../internal"} {
		if _, err := os.Stat(repoPath(removedRoot)); !os.IsNotExist(err) {
			t.Fatalf("application source directory remains at repository root: %s (stat error: %v)", removedRoot, err)
		}
	}

	for _, path := range []string{
		"main.go",
		"app.go",
		"analysis_cache.go",
		"batch_app.go",
		"batch_cache.go",
		"simulation_app.go",
		"app_test.go",
		"batch_cache_test.go",
		"batch_refactor_test.go",
		"wails.json",
		"frontend/assets.go",
		"internal/idf/analyze.go",
		"internal/frontendchecks/root_layout_static_test.go",
	} {
		if _, err := os.Stat(repoPath(path)); err != nil {
			t.Fatalf("required application file %s: %v", path, err)
		}
	}

	mainSource := readTestFile(t, "main.go")
	for _, required := range []string{
		`webassets "github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/frontend"`,
		"Assets:  webassets.Assets",
		"//go:embed wails.json",
	} {
		if !strings.Contains(mainSource, required) {
			t.Fatalf("command asset/config contract missing %q", required)
		}
	}

	assetSource := readTestFile(t, "frontend/assets.go")
	for _, required := range []string{"//go:embed all:src", `fs.Sub(embedded, "src")`} {
		if !strings.Contains(assetSource, required) {
			t.Fatalf("frontend embed contract missing %q", required)
		}
	}
}

func TestMovedWailsProjectKeepsRepositoryBuildPaths(t *testing.T) {
	var config struct {
		AssetDir       string `json:"assetdir"`
		FrontendDir    string `json:"frontend:dir"`
		BuildDir       string `json:"build:dir"`
		ReloadDirs     string `json:"reloaddirs"`
		OutputFilename string `json:"outputfilename"`
	}
	if err := json.Unmarshal([]byte(readTestFile(t, "wails.json")), &config); err != nil {
		t.Fatalf("parse moved Wails config: %v", err)
	}
	if config.AssetDir != "frontend/src" || config.FrontendDir != "frontend" {
		t.Fatalf("moved Wails frontend paths = asset %q, frontend %q", config.AssetDir, config.FrontendDir)
	}
	if config.BuildDir != "../../build" {
		t.Fatalf("moved Wails build path = %q, want ../../build", config.BuildDir)
	}
	if !strings.Contains(config.ReloadDirs, "internal") || !strings.Contains(config.ReloadDirs, "frontend/src") {
		t.Fatalf("moved Wails reload paths = %q", config.ReloadDirs)
	}

	for path, required := range map[string]string{
		"../../scripts/run.ps1":            "go run ./cmd/semantic-idf",
		"../../scripts/package.ps1":        `"cmd\semantic-idf"`,
		"../../scripts/verify.ps1":         `"cmd\semantic-idf"`,
		"../../scripts/frontend-build.ps1": `"..\cmd\semantic-idf\frontend"`,
		"../../scripts/release.ps1":        `"cmd\semantic-idf\frontend\src\js\app-info.js"`,
	} {
		if content := readTestFile(t, path); !strings.Contains(content, required) {
			t.Fatalf("%s does not reference the moved command/config: want %q", path, required)
		}
	}
}
