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
Push-Location $paths.RepoRoot
try {
    & $paths.GoExe test ./...
    Assert-LastExitCode -Operation "go test ./..."
}
finally {
    Pop-Location
}

$projectDir = Join-Path $paths.RepoRoot "cmd\semantic-idf"
Push-Location $projectDir
try {
    & $paths.WailsExe build
    Assert-LastExitCode -Operation "wails build"
}
finally {
    Pop-Location
}
