package frontendchecks

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDevelopmentTestSelectorContracts(t *testing.T) {
	// The Go test cache cannot see files read only by a child PowerShell process.
	// Read each input here so changes to the selector or its manifests invalidate it.
	dependencies := []string{"test-plan.ps1", "test-plan-checks.ps1", "test-impact.json"}
	for _, name := range dependencies {
		if _, err := os.ReadFile(repoPath("../../scripts/" + name)); err != nil {
			t.Fatalf("read selector dependency %s: %v", name, err)
		}
	}
	manifests, err := filepath.Glob(repoPath("../../scripts/test-groups-*.json"))
	if err != nil {
		t.Fatalf("discover selector manifests: %v", err)
	}
	for _, path := range manifests {
		if _, err := os.ReadFile(path); err != nil {
			t.Fatalf("read selector manifest %s: %v", path, err)
		}
	}
	powershell, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("Windows PowerShell is unavailable for development test selector checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, powershell, "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", repoPath("../../scripts/test-plan-checks.ps1"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("development test selector behavior checks failed: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}
