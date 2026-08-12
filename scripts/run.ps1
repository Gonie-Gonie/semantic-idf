$ErrorActionPreference = "Stop"

. "$PSScriptRoot\toolchain.ps1"

$paths = Use-RepoToolchain -RequireGo
Push-Location $paths.RepoRoot
try {
    & $paths.GoExe run ./cmd/semantic-idf
    if ($LASTEXITCODE -ne 0) {
        throw "go run ./cmd/semantic-idf failed with exit code $LASTEXITCODE."
    }
}
finally {
    Pop-Location
}
