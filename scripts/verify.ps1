$ErrorActionPreference = "Stop"

. "$PSScriptRoot\toolchain.ps1"

$paths = Use-RepoToolchain -RequireGo -RequireWails

if (Get-Command git -ErrorAction SilentlyContinue) {
    git -C $paths.RepoRoot diff --check
}

& "$PSScriptRoot\frontend-build.ps1"
& $paths.GoExe test ./...
& $paths.WailsExe build
