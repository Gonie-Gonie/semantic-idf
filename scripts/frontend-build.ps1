$ErrorActionPreference = "Stop"

$dist = Join-Path $PSScriptRoot "..\frontend\dist"
$index = Join-Path $dist "index.html"
$guide = Join-Path $dist "guide.html"
$entry = Join-Path $dist "app.js"
$moduleDir = Join-Path $dist "js"

if (-not (Test-Path $index)) {
    throw "Missing frontend/dist/index.html"
}

if (-not (Test-Path $guide)) {
    throw "Missing frontend/dist/guide.html"
}

if (-not (Test-Path $entry)) {
    throw "Missing frontend/dist/app.js"
}

$modules = @(
    "actions.js",
    "analysis-views.js",
    "input-views.js",
    "layout.js",
    "main.js",
    "navigation.js",
    "sample.js",
    "state.js"
)

foreach ($module in $modules) {
    $path = Join-Path $moduleDir $module
    if (-not (Test-Path $path)) {
        throw "Missing frontend/dist/js/$module"
    }
}

Write-Host "Static frontend is ready."
