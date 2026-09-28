package frontendchecks

import (
	"strings"
	"testing"
)

func TestReleaseScriptReadsTrackedTextAsUTF8(t *testing.T) {
	script := readTestFile(t, "../../scripts/release.ps1")
	for lineNumber, line := range strings.Split(script, "\n") {
		if !strings.Contains(line, "Get-Content") || !strings.Contains(line, "-Raw") {
			continue
		}
		if !strings.Contains(strings.ToLower(line), "-encoding utf8") {
			t.Fatalf("release script raw text read on line %d must specify UTF-8: %s", lineNumber+1, line)
		}
	}
}

func TestVerificationUsesBoundedGoAndReleaseTimeouts(t *testing.T) {
	verify := readTestFile(t, "../../scripts/verify.ps1")
	for _, required := range []string{
		`[string]$GoTestTimeout = "20m"`,
		`& $paths.GoExe test "-timeout=$GoTestTimeout" ./...`,
	} {
		if !strings.Contains(verify, required) {
			t.Fatalf("verification must retain a bounded Go test timeout with CI headroom, missing %q", required)
		}
	}

	workflow := readTestFile(t, "../../.github/workflows/release.yml")
	if !strings.Contains(workflow, "timeout-minutes: 45") {
		t.Fatal("release workflow must retain an outer timeout for stalled setup, verification, build, or publish steps")
	}
}

func TestReleaseScriptChecksGitMutationsAndPushesAtomically(t *testing.T) {
	script := readTestFile(t, "../../scripts/release.ps1")

	clean := sliceBetween(script, "function Assert-CleanGitTree", "function Read-ReleaseNoteBody")
	for _, required := range []string{`$statusExitCode = $LASTEXITCODE`, `if ($statusExitCode -ne 0)`, `throw "Failed to inspect the Git working tree`} {
		if !strings.Contains(clean, required) {
			t.Fatalf("release cleanliness check must fail closed, missing %q", required)
		}
	}

	commit := sliceBetween(script, "function Invoke-GitReleaseCommit", "function Invoke-GitTag")
	for _, required := range []string{`throw "Failed to stage release metadata`, `$diffExitCode = $LASTEXITCODE`, `if ($diffExitCode -gt 1)`, `throw "Failed to inspect staged release metadata`, `git -C $RepoRoot commit`, `if ($LASTEXITCODE -ne 0)`, `throw "Failed to create release commit`} {
		if !strings.Contains(commit, required) {
			t.Fatalf("release commit must fail closed, missing %q", required)
		}
	}

	tag := sliceBetween(script, "function Invoke-GitTag", "function Assert-ExistingGitTag")
	for _, required := range []string{`git -C $RepoRoot tag -a`, `if ($LASTEXITCODE -ne 0)`, `throw "Failed to create release tag`} {
		if !strings.Contains(tag, required) {
			t.Fatalf("release tag creation must fail closed, missing %q", required)
		}
	}

	push := sliceBetween(script, "function Invoke-GitPush", "function Test-GitHubReleaseExists")
	for _, required := range []string{`git -C $RepoRoot push --atomic origin`, `"HEAD:$branch"`, `"refs/tags/${tagName}:refs/tags/${tagName}"`, `if ($LASTEXITCODE -ne 0)`, `throw "Failed to atomically push`} {
		if !strings.Contains(push, required) {
			t.Fatalf("release push must atomically fail closed, missing %q", required)
		}
	}
}

func TestReleaseScriptResetsSupportedReleaseNoteSections(t *testing.T) {
	script := readTestFile(t, "../../scripts/release.ps1")
	reset := sliceBetween(script, "function Reset-UnreleasedReleaseNotes", "function New-ReleasePackage")
	for _, section := range []string{"## Breaking Changes", "## Added", "## Changed", "## Fixed", "## Performance"} {
		if !strings.Contains(reset, section) {
			t.Fatalf("reset release-note template is missing %q", section)
		}
	}
}

func TestReleaseScriptAutoBumpIsTagBasedAndCannotReusePublishedNotes(t *testing.T) {
	script := readTestFile(t, "../../scripts/release.ps1")

	noteSource := sliceBetween(script, "function Get-ReleaseNoteSource", "function Get-InferredBump")
	for _, required := range []string{"[switch]$AllowVersionedFallback", "if ($AllowVersionedFallback)"} {
		if !strings.Contains(noteSource, required) {
			t.Fatalf("versioned release-note fallback must be explicit, missing %q", required)
		}
	}

	target := sliceBetween(script, "function Get-TargetVersion", "function Update-Changelog")
	for _, required := range []string{`$baseVersion = $CurrentVersion`, `$baseVersion = (Parse-SemVer -Text $LatestTag).Text`, `Add-SemVerBump -CurrentVersion $baseVersion`} {
		if !strings.Contains(target, required) {
			t.Fatalf("automatic bump must use the latest tag as its stable base, missing %q", required)
		}
	}
	if strings.Contains(target, "Add-SemVerBump -CurrentVersion $CurrentVersion") {
		t.Fatal("automatic bump still compounds a prepared product version")
	}

	main := script[strings.Index(script, "$repoRoot = Get-RepoRoot"):]
	if !strings.Contains(main, `-AllowVersionedFallback:$allowVersionedNoteSource`) {
		t.Fatal("release main path does not gate reuse of versioned release notes")
	}
}
