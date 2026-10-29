param(
    [switch]$Quick,
    [switch]$Full,
    [string[]]$Area = @(),
    [ValidateSet('frontend', 'backend')][string]$Layer,
    [switch]$Staged,
    [string]$BaseRef,
    [string]$BuildOutputFilename = $env:SEMANTICIDF_VERIFY_OUTPUT,
    [ValidatePattern('^\d+(?:ns|us|\xB5s|ms|s|m|h)$')]
    [string]$GoTestTimeout = "20m"
)

$ErrorActionPreference = "Stop"

if ($BuildOutputFilename -and $BuildOutputFilename -cnotmatch '\A[A-Za-z0-9][A-Za-z0-9._-]*\.[eE][xX][eE]\z') {
    throw "BuildOutputFilename must be an ASCII filename ending in .exe, without a directory path."
}

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
    git -C $paths.RepoRoot diff --cached --check
    Assert-LastExitCode -Operation "git diff --cached --check"
}

& "$PSScriptRoot\frontend-build.ps1"
$testOptions = @{ Quick = $Quick; Full = $Full; Area = $Area; Staged = $Staged; BaseRef = $BaseRef }
if ($Layer) { $testOptions.Layer = $Layer }
& "$PSScriptRoot\test.ps1" @testOptions -GoTestTimeout $GoTestTimeout

$projectDir = Join-Path $paths.RepoRoot "cmd\semantic-idf"
Push-Location $projectDir
try {
    if ($BuildOutputFilename) {
        & $paths.WailsExe build -o $BuildOutputFilename
    }
    else {
        & $paths.WailsExe build
    }
    Assert-LastExitCode -Operation "wails build"
}
finally {
    Pop-Location
}
