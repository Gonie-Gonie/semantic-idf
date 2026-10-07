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
	fixture.script(true, "-Hard", "-WhatIf")
	fixture.assertFiles(fixture.files)

	// An unrelated working directory must not become the cleanup target.
	fixture.dev(true, "clean-repo")
	fixture.assertAbsent(fixture.normalGenerated)
	fixture.assertFiles(fixture.normalProtected())
	fixture.script(true)
	fixture.assertFiles(fixture.normalProtected())

	// Exercise the real CMD entry point and its --hard translation, rather than
	// checking wrapper source text or bypassing it with a direct -Hard call.
	fixture.dev(true, "clean-repo", "--hard")
	fixture.assertAbsent(fixture.normalGenerated)
	fixture.assertAbsent(fixture.hardGenerated)
	fixture.assertFiles(fixture.permanent)
	fixture.dev(true, "clean-repo", "--hard")
	fixture.assertFiles(fixture.permanent)
}

func TestCleanRepoRejectsUnsafeRequests(t *testing.T) {
	t.Run("unknown options do not delete anything", func(t *testing.T) {
		fixture := newCleanRepoFixture(t)
		fixture.script(false, "-UnexpectedCleanupOption")
		fixture.assertFiles(fixture.files)
		fixture.dev(false, "clean-repo", "--unexpected-cleanup-option")
		fixture.assertFiles(fixture.files)
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
		junction := filepath.Join(fixture.root, "build", "release", "external files")
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
		fixture.script(false, "-Hard")
		fixture.assertFiles(fixture.files)
		contents, err := os.ReadFile(marker)
		if err != nil || string(contents) != "outside model must survive" {
			t.Fatalf("cleanup touched the junction target: %q, %v", contents, err)
		}
	})
}

type cleanRepoFixture struct {
	t               *testing.T
	root            string
	cwd             string
	powershell      string
	cmd             string
	env             []string
	files           map[string]string
	permanent       map[string]string
	normalGenerated []string
	hardGenerated   []string
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
	} {
		fixture.write(relative, "tracked fixture: "+relative, true)
	}
	fixture.run(true, git, "-C", fixture.root, "init", "--quiet")
	fixture.run(true, git, "-C", fixture.root, "add", ".")
	fixture.run(true, git, "-C", fixture.root, "-c", "user.name=Cleanup Test", "-c", "user.email=cleanup@example.invalid",
		"-c", "commit.gpgsign=false", "commit", "--quiet", "--no-verify", "-m", "fixture")

	// New files staged in the index are protected as carefully as committed
	// files, including descendants of directories otherwise removed wholesale.
	for _, relative := range []string{"build/release/staged-evidence.json", "build/release/보존 근거.json", ".runtime/gocache/staged-reference.txt"} {
		fixture.write(relative, "staged fixture: "+relative, true)
		fixture.run(true, git, "-C", fixture.root, "add", "--", relative)
	}
	for _, relative := range []string{
		".env", ".env.local", "models/user-model.idf", "scratch/user-notes.txt",
		"cmd/semantic-idf/internal/idf/testdata/hvac_external/user-model.idf",
		"cmd/semantic-idf/internal/analysis/user-fixture.test",
	} {
		fixture.write(relative, "user-owned fixture: "+relative, true)
	}
	fixture.normalGenerated = []string{
		"bin/semantic-idf.exe", "build/release/package.zip", "build/windows/generated.manifest",
		"cmd/semantic-idf/frontend/wailsjs/runtime.js", "cmd/semantic-idf/frontend/dist/index.html",
		"node_modules/example/index.js", "cmd/semantic-idf/frontend/node_modules/example/index.js",
		".runtime/gocache/cache-entry", ".runtime/guide-review/review.png",
		"cleanup.test", "cleanup.test.exe", "cleanup.syso", "coverage.out",
		"cmd/semantic-idf/cleanup.test", "cmd/semantic-idf/cleanup.test.exe",
		"cmd/semantic-idf/cleanup.syso", "cmd/semantic-idf/coverage.out",
		".runtime/cleanup.test", ".runtime/cleanup.test.exe",
	}
	fixture.hardGenerated = []string{
		"build/bin/semantic-idf.exe", "build/bin/debug-notes.txt",
		".runtime/go/bin/go.exe", ".runtime/bin/wails.exe", ".runtime/gomodcache/module/source.go",
		".runtime/gopath/module/source.go", ".runtime/cache/toolchain.zip",
		".runtime/acceptance-evidence/report.json", ".runtime/baseline.json", ".runtime/scratch/manual.txt",
		".tools/analyzer.exe",
	}
	for _, relative := range append(append([]string{}, fixture.normalGenerated...), fixture.hardGenerated...) {
		fixture.write(relative, "generated fixture: "+relative, false)
	}
	return fixture
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
