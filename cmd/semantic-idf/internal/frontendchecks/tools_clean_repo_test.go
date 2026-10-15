package frontendchecks

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// These tests execute only copies of the cleanup command inside temporary Git
// repositories. Never run the cleanup entry point in the working repository.
func TestCleanRepoNormalAndHardContracts(t *testing.T) {
	fixture := newCleanRepoFixture(t)

	fixture.script(true, "-WhatIf")
	fixture.assertFiles(fixture.files)
	fixture.assertDirectories(fixture.emptyLegacyDirectories)
	fixture.script(true, "-Hard", "-WhatIf")
	fixture.assertFiles(fixture.files)
	fixture.assertDirectories(fixture.emptyLegacyDirectories)

	// An unrelated working directory must not become the cleanup target.
	fixture.dev(true, "clean-repo")
	fixture.assertAbsent(fixture.normalGenerated)
	fixture.assertAbsent([]string{"frontend", "internal"})
	fixture.assertAbsent([]string{
		".runtime/tmp", ".runtime/chrome-cdp-session", ".runtime/chrome-smoke-session",
		".runtime/chrome-hvac-profile", ".runtime/chrome-hvac-profile-session",
		".runtime/edge-cdp-session", ".runtime/edge-smoke-session", ".runtime/edge-hvac-profile",
	})
	fixture.assertFiles(fixture.normalProtected())
	fixture.script(true)
	fixture.assertFiles(fixture.normalProtected())

	// Exercise the real CMD entry point and its --hard translation, rather than
	// checking wrapper source text or bypassing it with a direct -Hard call.
	fixture.makeEmptyLegacyDirectories()
	fixture.dev(true, "clean-repo", "--hard")
	fixture.assertAbsent(fixture.normalGenerated)
	fixture.assertAbsent(fixture.hardGenerated)
	fixture.assertAbsent([]string{"frontend", "internal"})
	fixture.assertFiles(fixture.permanent)
	fixture.dev(true, "clean-repo", "--hard")
	fixture.assertFiles(fixture.permanent)

	t.Run("legacy roots containing user files preserve their entire trees", func(t *testing.T) {
		fixture := newCleanRepoFixture(t)
		hiddenFile := "frontend/wailsjs/go/main/.env"
		fixture.write(hiddenFile, "local frontend settings", true)
		fixture.write("internal/idf/testdata/hvac_external/user-model.idf", "local model", true)
		fixture.emptyLegacyDirectories = append(fixture.emptyLegacyDirectories, "internal/idf/testdata/empty-sibling")
		fixture.makeEmptyLegacyDirectories()
		fixture.run(true, fixture.powershell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
			"$item = Get-Item -LiteralPath "+cleanRepoPowerShellLiteral(filepath.Join(fixture.root, filepath.FromSlash(hiddenFile)))+
				" -Force -ErrorAction Stop; $item.Attributes = $item.Attributes -bor [IO.FileAttributes]::Hidden")

		fixture.script(true)
		fixture.assertFiles(fixture.normalProtected())
		fixture.assertDirectories(fixture.emptyLegacyDirectories)
		fixture.script(true, "-Hard")
		fixture.assertFiles(fixture.permanent)
		fixture.assertDirectories(fixture.emptyLegacyDirectories)
	})

	t.Run("legacy roots containing junctions preserve their entire trees", func(t *testing.T) {
		fixture := newCleanRepoFixture(t)
		outside := t.TempDir()
		marker := filepath.Join(outside, "user-model.idf")
		if err := os.WriteFile(marker, []byte("outside model must survive"), 0o644); err != nil {
			t.Fatal(err)
		}
		fixture.emptyLegacyDirectories = append(fixture.emptyLegacyDirectories, "internal/idf/testdata/empty-sibling")
		fixture.makeEmptyLegacyDirectories()
		for _, relative := range []string{
			"frontend/wailsjs/runtime/external", "internal/idf/testdata/hvac_external/external",
		} {
			junction := filepath.Join(fixture.root, filepath.FromSlash(relative))
			output, err := fixture.command(fixture.powershell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
				"New-Item -ItemType Junction -Path "+cleanRepoPowerShellLiteral(junction)+" -Target "+cleanRepoPowerShellLiteral(outside)+" -ErrorAction Stop | Out-Null")
			if err != nil {
				t.Skipf("cannot create a directory junction on this Windows host: %v\n%s", err, output)
			}
			// Remove the link only, never traverse its external target.
			t.Cleanup(func() {
				if err := os.Remove(junction); err != nil && !os.IsNotExist(err) {
					t.Errorf("remove fixture junction: %v", err)
				}
			})
			fixture.emptyLegacyDirectories = append(fixture.emptyLegacyDirectories, relative)
		}

		fixture.script(true)
		fixture.assertFiles(fixture.normalProtected())
		fixture.assertDirectories(fixture.emptyLegacyDirectories)
		fixture.script(true, "-Hard")
		fixture.assertFiles(fixture.permanent)
		fixture.assertDirectories(fixture.emptyLegacyDirectories)
		contents, err := os.ReadFile(marker)
		if err != nil || string(contents) != "outside model must survive" {
			t.Fatalf("cleanup touched a legacy junction target: %q, %v", contents, err)
		}
	})

	t.Run("missing runtime directory remains optional on repeated cleanup", func(t *testing.T) {
		fixture := newCleanRepoFixture(t)
		runtimePath := filepath.Clean(filepath.Join(fixture.root, ".runtime"))
		if !filepath.IsAbs(runtimePath) || filepath.Dir(runtimePath) != fixture.root {
			t.Fatalf("runtime fixture must stay inside its temporary repository: %s", runtimePath)
		}
		if err := os.RemoveAll(runtimePath); err != nil {
			t.Fatal(err)
		}
		protected := fixture.normalProtected()
		for relative := range protected {
			if strings.HasPrefix(relative, ".runtime/") {
				delete(protected, relative)
			}
		}
		for range 2 {
			fixture.script(true)
			fixture.assertAbsent(fixture.normalGenerated)
			fixture.assertAbsent([]string{".runtime"})
			fixture.assertFiles(protected)
		}
	})
}

func TestCleanRepoRejectsUnsafeRequests(t *testing.T) {
	t.Run("unknown options do not delete anything", func(t *testing.T) {
		fixture := newCleanRepoFixture(t)
		fixture.script(false, "-UnexpectedCleanupOption")
		fixture.assertFiles(fixture.files)
		fixture.assertDirectories(fixture.emptyLegacyDirectories)
		fixture.dev(false, "clean-repo", "--unexpected-cleanup-option")
		fixture.assertFiles(fixture.files)
		fixture.assertDirectories(fixture.emptyLegacyDirectories)
	})

	t.Run("external junction aborts the whole cleanup before deletion", func(t *testing.T) {
		fixture := newCleanRepoFixture(t)
		outside := filepath.Join(t.TempDir(), "outside files")
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(outside, "user-model.idf")
		if err := os.WriteFile(marker, []byte("outside model must survive"), 0o644); err != nil {
			t.Fatal(err)
		}
		junction := filepath.Join(fixture.root, ".runtime", "chrome-cdp-session", "external files")
		output, err := fixture.command(fixture.powershell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
			"New-Item -ItemType Junction -Path "+cleanRepoPowerShellLiteral(junction)+" -Target "+cleanRepoPowerShellLiteral(outside)+" -ErrorAction Stop | Out-Null")
		if err != nil {
			t.Skipf("cannot create a directory junction on this Windows host: %v\n%s", err, output)
		}
		// Remove only the junction itself; os.Remove does not traverse its target.
		t.Cleanup(func() {
			if err := os.Remove(junction); err != nil && !os.IsNotExist(err) {
				t.Errorf("remove fixture junction: %v", err)
			}
		})

		fixture.script(false)
		fixture.assertFiles(fixture.files)
		fixture.assertDirectories(fixture.emptyLegacyDirectories)
		fixture.script(false, "-WhatIf")
		fixture.assertFiles(fixture.files)
		fixture.assertDirectories(fixture.emptyLegacyDirectories)
		fixture.script(false, "-Hard")
		fixture.assertFiles(fixture.files)
		fixture.assertDirectories(fixture.emptyLegacyDirectories)
		contents, err := os.ReadFile(marker)
		if err != nil || string(contents) != "outside model must survive" {
			t.Fatalf("cleanup touched the junction target: %q, %v", contents, err)
		}
	})
}

type cleanRepoFixture struct {
	t                      *testing.T
	root                   string
	cwd                    string
	powershell             string
	cmd                    string
	env                    []string
	files                  map[string]string
	permanent              map[string]string
	normalGenerated        []string
	hardGenerated          []string
	emptyLegacyDirectories []string
}

func newCleanRepoFixture(t *testing.T) *cleanRepoFixture {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("repository cleanup command requires Windows PowerShell and CMD")
	}
	powershell, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("Windows PowerShell is unavailable")
	}
	cmd, err := exec.LookPath("cmd")
	if err != nil {
		t.Skip("Windows CMD is unavailable")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is unavailable for cleanup index protection checks")
	}
	fixture := &cleanRepoFixture{
		t: t, root: filepath.Join(t.TempDir(), "복제 작업 폴더"),
		cwd:        filepath.Join(t.TempDir(), "별도 실행 폴더"),
		powershell: powershell, cmd: cmd,
		env:   cleanRepoGitEnvironment(git),
		files: make(map[string]string), permanent: make(map[string]string),
	}
	if err := os.MkdirAll(fixture.cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	// Go's cache cannot see dependencies read only by child processes, so read
	// all actual implementation inputs before copying them into the fixture.
	for _, relative := range []string{"scripts/clean-repo.ps1", "dev.bat"} {
		contents, err := os.ReadFile(repoPath("../../" + relative))
		if err != nil {
			t.Fatalf("read cleanup dependency %s: %v", relative, err)
		}
		fixture.write(relative, string(contents), true)
	}
	for _, relative := range []string{
		"go.mod", "cmd/semantic-idf/main.go", "cmd/semantic-idf/frontend/src/index.html",
		"build/appicon.png", "build/windows/icon.ico", "bin/tracked-output.txt",
		".runtime/go/tracked-policy.txt",
		".runtime/chrome-cdp-tracked/Default/tracked-policy.txt",
	} {
		fixture.write(relative, "tracked fixture: "+relative, true)
	}
	fixture.run(true, git, "-C", fixture.root, "init", "--quiet")
	fixture.run(true, git, "-C", fixture.root, "add", ".")
	fixture.run(true, git, "-C", fixture.root, "-c", "user.name=Cleanup Test", "-c", "user.email=cleanup@example.invalid",
		"-c", "commit.gpgsign=false", "commit", "--quiet", "--no-verify", "-m", "fixture")

	// New files staged in the index are protected as carefully as committed
	// files, including descendants of directories otherwise removed wholesale.
	for _, relative := range []string{
		"build/release/staged-evidence.json", "build/release/보존 근거.json", ".runtime/gocache/staged-reference.txt",
		".runtime/edge-hvac-profile-staged/Default/staged-reference.txt",
	} {
		fixture.write(relative, "staged fixture: "+relative, true)
		fixture.run(true, git, "-C", fixture.root, "add", "--", relative)
	}
	for _, relative := range []string{
		".env", ".env.local", "models/user-model.idf", "scratch/user-notes.txt",
		"cmd/semantic-idf/internal/idf/testdata/hvac_external/user-model.idf",
		"cmd/semantic-idf/internal/analysis/user-fixture.test",
		"chrome-cdp-user-notes/profile.txt", "scratch/chrome-smoke-session/profile.txt",
		"scratch/chrome-dom.txt", "scratch/geometry-mobile.png", "scratch/hvac_http_stdout.log",
		"chrome-dom.txt", "geometry-390x844.png", "cmd/semantic-idf/hvac_static_screenshot.png",
	} {
		fixture.write(relative, "user-owned fixture: "+relative, true)
	}
	fixture.normalGenerated = []string{
		"cmd/semantic-idf/frontend/wailsjs/runtime.js",
		"node_modules/example/index.js", "cmd/semantic-idf/frontend/node_modules/example/index.js",
		".runtime/gocache/cache-entry", ".runtime/guide-review/review.png",
		".runtime/tmp/session/scratch.txt",
		".runtime/chrome-cdp-session/Default/cache-entry", ".runtime/chrome-cdp/Default/cache-entry",
		".runtime/chrome-smoke-session/Default/cache-entry", ".runtime/chrome-smoke/Default/cache-entry",
		".runtime/chrome-hvac-profile/Default/cache-entry", ".runtime/chrome-hvac-profile-session/Default/cache-entry",
		".runtime/edge-cdp-session/Default/cache-entry", ".runtime/edge-cdp/Default/cache-entry",
		".runtime/edge-smoke-session/Default/cache-entry", ".runtime/edge-smoke/Default/cache-entry",
		".runtime/edge-hvac-profile/Default/cache-entry", ".runtime/edge-hvac-profile-session/Default/cache-entry",
		".runtime/chrome-cdp-tracked/Default/cache-entry", ".runtime/edge-hvac-profile-staged/Default/cache-entry",
		".runtime/chrome-dom.txt", ".runtime/chrome-dom.err.txt", ".runtime/geometry-smoke.html",
		".runtime/hvac_http_stdout.log", ".runtime/hvac_http_stderr.log", ".runtime/hvac_static_screenshot.png",
		".runtime/geometry-390x844.png", ".runtime/geometry-560x844.png", ".runtime/geometry-800x844.png",
		".runtime/geometry-1920x1080.png", ".runtime/geometry-desktop.png", ".runtime/geometry-mobile.png",
		".runtime/geometry-desktop-canvas.png", ".runtime/geometry-mobile-cdp.png",
		".runtime/geometry-desktop-cdp-surface-False.png", ".runtime/geometry-mobile-cdp-surface-True.png",
		".runtime/geometry-desktop-default.png", ".runtime/geometry-mobile-disable-software-rasterizer.png",
		".runtime/geometry-desktop-timeout.png", ".runtime/geometry-mobile-use-angle-swiftshader.png",
		"cleanup.test", "cleanup.test.exe", "cleanup.syso", "coverage.out",
		"cmd/semantic-idf/cleanup.test", "cmd/semantic-idf/cleanup.test.exe",
		"cmd/semantic-idf/cleanup.syso", "cmd/semantic-idf/coverage.out",
		".runtime/cleanup.test", ".runtime/cleanup.test.exe", ".runtime/cleanup.syso", ".runtime/coverage.out",
	}
	fixture.hardGenerated = []string{
		"bin/semantic-idf.exe", "bin/cleanup.test", "build/release/package.zip", "build/windows/generated.manifest",
		"build/cleanup.syso", "cmd/semantic-idf/frontend/dist/index.html", "cmd/semantic-idf/frontend/dist/coverage.out",
		"build/bin/semantic-idf.exe", "build/bin/debug-notes.txt",
		".runtime/go/bin/go.exe", ".runtime/bin/wails.exe", ".runtime/gomodcache/module/source.go",
		".runtime/gopath/module/source.go", ".runtime/cache/toolchain.zip",
		".runtime/acceptance-evidence/report.json", ".runtime/baseline.json", ".runtime/scratch/manual.txt",
		".runtime/screenshot.png", ".runtime/geometry-baseline.png", ".runtime/geometry-desktop-baseline.png",
		".runtime/chrome-install/install.txt", ".runtime/edge-tools/tool.txt",
		".runtime/chrome-cdp-not-a-directory", ".runtime/chrome-cdpnotes/user.txt",
		".runtime/geometry-1024x768.png/user-notes.txt",
		".runtime/evidence/chrome-dom.txt", ".runtime/baselines/geometry-390x844.png",
		".runtime/scratch/edge-smoke-session/profile.txt",
		".tools/analyzer.exe",
	}
	for _, relative := range append(append([]string{}, fixture.normalGenerated...), fixture.hardGenerated...) {
		fixture.write(relative, "generated fixture: "+relative, false)
	}
	fixture.emptyLegacyDirectories = []string{
		"frontend/wailsjs/go/main", "frontend/wailsjs/runtime", "internal/idf/testdata/hvac_external",
	}
	fixture.makeEmptyLegacyDirectories()
	return fixture
}

func (fixture *cleanRepoFixture) makeEmptyLegacyDirectories() {
	fixture.t.Helper()
	for _, relative := range fixture.emptyLegacyDirectories {
		if err := os.MkdirAll(filepath.Join(fixture.root, filepath.FromSlash(relative)), 0o755); err != nil {
			fixture.t.Fatal(err)
		}
	}
}

func (fixture *cleanRepoFixture) write(relative, contents string, permanent bool) {
	fixture.t.Helper()
	path := filepath.Join(fixture.root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fixture.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		fixture.t.Fatal(err)
	}
	fixture.files[relative] = contents
	if permanent {
		fixture.permanent[relative] = contents
	}
}

func (fixture *cleanRepoFixture) normalProtected() map[string]string {
	protected := make(map[string]string, len(fixture.permanent)+len(fixture.hardGenerated))
	for relative, contents := range fixture.permanent {
		protected[relative] = contents
	}
	for _, relative := range fixture.hardGenerated {
		protected[relative] = fixture.files[relative]
	}
	return protected
}

func (fixture *cleanRepoFixture) assertFiles(files map[string]string) {
	fixture.t.Helper()
	for relative, expected := range files {
		actual, err := os.ReadFile(filepath.Join(fixture.root, filepath.FromSlash(relative)))
		if err != nil || string(actual) != expected {
			fixture.t.Errorf("protected fixture %s was changed or removed: %v", relative, err)
		}
	}
	if info, err := os.Stat(filepath.Join(fixture.root, ".git")); err != nil || !info.IsDir() {
		fixture.t.Fatalf("cleanup damaged the Git repository: %v", err)
	}
}

func (fixture *cleanRepoFixture) assertAbsent(paths []string) {
	fixture.t.Helper()
	for _, relative := range paths {
		if _, err := os.Lstat(filepath.Join(fixture.root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			fixture.t.Errorf("generated fixture %s was not removed: %v", relative, err)
		}
	}
}

func (fixture *cleanRepoFixture) assertDirectories(paths []string) {
	fixture.t.Helper()
	for _, relative := range paths {
		info, err := os.Stat(filepath.Join(fixture.root, filepath.FromSlash(relative)))
		if err != nil || !info.IsDir() {
			fixture.t.Errorf("protected fixture directory %s was removed: %v", relative, err)
		}
	}
}

func (fixture *cleanRepoFixture) script(wantSuccess bool, arguments ...string) {
	fixture.t.Helper()
	args := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(fixture.root, "scripts", "clean-repo.ps1")}
	fixture.run(wantSuccess, fixture.powershell, append(args, arguments...)...)
}

func (fixture *cleanRepoFixture) dev(wantSuccess bool, arguments ...string) {
	fixture.t.Helper()
	// CALL avoids CMD's special outer-quote handling when the fixture path
	// contains spaces; os/exec quotes the path as its own argument.
	args := []string{"/D", "/C", "call", filepath.Join(fixture.root, "dev.bat")}
	fixture.run(wantSuccess, fixture.cmd, append(args, arguments...)...)
}

func (fixture *cleanRepoFixture) run(wantSuccess bool, executable string, arguments ...string) {
	fixture.t.Helper()
	output, err := fixture.command(executable, arguments...)
	if (err == nil) != wantSuccess {
		fixture.t.Fatalf("cleanup fixture command %s %v returned %v (want success=%v):\n%s", executable, arguments, err, wantSuccess, output)
	}
}

func (fixture *cleanRepoFixture) command(executable string, arguments ...string) ([]byte, error) {
	fixture.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = fixture.cwd
	command.Env = fixture.env
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		fixture.t.Fatalf("cleanup fixture command timed out: %v\n%s", ctx.Err(), output)
	}
	return output, err
}

func cleanRepoPowerShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func cleanRepoGitEnvironment(gitExecutable string) []string {
	gitDirectory := filepath.Dir(gitExecutable)
	for _, candidate := range []string{filepath.Dir(gitDirectory), filepath.Dir(filepath.Dir(gitDirectory))} {
		cmdDirectory := filepath.Join(candidate, "cmd")
		mingwDirectory := filepath.Join(candidate, "mingw64", "bin")
		cmdGit, cmdErr := os.Stat(filepath.Join(cmdDirectory, "git.exe"))
		mingwGit, mingwErr := os.Stat(filepath.Join(mingwDirectory, "git.exe"))
		if cmdErr != nil || mingwErr != nil || cmdGit.IsDir() || mingwGit.IsDir() {
			continue
		}
		// Git's hook environment can expose both applications to Get-Command.
		// Reproduce that environment even when the parent shell exposes only one,
		// so cleanup must choose one executable rather than pass an array to Start.
		environment := make([]string, 0, len(os.Environ())+1)
		for _, entry := range os.Environ() {
			name, _, _ := strings.Cut(entry, "=")
			if !strings.EqualFold(name, "PATH") {
				environment = append(environment, entry)
			}
		}
		prefix := strings.Join([]string{mingwDirectory, cmdDirectory}, string(os.PathListSeparator))
		return append(environment, "PATH="+prefix+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	// Other Git distributions still exercise every cleanup contract with the
	// ordinary inherited environment; the behavioral tests are never skipped.
	return os.Environ()
}
