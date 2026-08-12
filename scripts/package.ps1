$ErrorActionPreference = "Stop"

. "$PSScriptRoot\toolchain.ps1"

$paths = Use-RepoToolchain -RequireGo -RequireWails
$projectDir = Join-Path $paths.RepoRoot "cmd\semantic-idf"
Push-Location $projectDir
try {
    & $paths.WailsExe build
    if ($LASTEXITCODE -ne 0) {
        throw "wails build failed with exit code $LASTEXITCODE."
    }
}
finally {
    Pop-Location
}
