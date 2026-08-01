$ErrorActionPreference = "Stop"

. "$PSScriptRoot\toolchain.ps1"

function Assert-LastExitCode {
    param([string]$Operation)

    if ($LASTEXITCODE -ne 0) {
        throw "$Operation failed with exit code $LASTEXITCODE."
    }
}

$paths = Use-RepoToolchain -RequireGo -RequireWails

if (Get-Command git -ErrorAction SilentlyContinue) {
    git -C $paths.RepoRoot diff --check
    Assert-LastExitCode -Operation "git diff --check"
}

& "$PSScriptRoot\frontend-build.ps1"
& $paths.GoExe test ./...
Assert-LastExitCode -Operation "go test ./..."
& $paths.WailsExe build
Assert-LastExitCode -Operation "wails build"
