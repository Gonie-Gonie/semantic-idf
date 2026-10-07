$ErrorActionPreference = "Stop"

. "$PSScriptRoot\toolchain.ps1"

$paths = Use-RepoToolchain -RequireGo
Push-Location $paths.RepoRoot
try {
    & $paths.GoExe test ./...
    if ($LASTEXITCODE -ne 0) {
        throw "go test ./... failed with exit code $LASTEXITCODE."
    }
}
finally {
    Pop-Location
}
