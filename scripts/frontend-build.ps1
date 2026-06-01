$ErrorActionPreference = "Stop"

$dist = Join-Path $PSScriptRoot "..\frontend\dist"
$index = Join-Path $dist "index.html"
$tools = Join-Path $dist "tools.html"
$guide = Join-Path $dist "guide.html"
$settings = Join-Path $dist "settings.html"
$entry = Join-Path $dist "app.js"
$moduleDir = Join-Path $dist "js"

if (-not (Test-Path $index)) {
    throw "Missing frontend/dist/index.html"
}

if (-not (Test-Path $tools)) {
    throw "Missing frontend/dist/tools.html"
}

if (-not (Test-Path $guide)) {
    throw "Missing frontend/dist/guide.html"
}

if (-not (Test-Path $settings)) {
    throw "Missing frontend/dist/settings.html"
}

if (-not (Test-Path $entry)) {
    throw "Missing frontend/dist/app.js"
}

$modules = @(
    "actions.js",
    "analysis-views.js",
    "app-info.js",
    "geometry-loader.js",
    "geometry-view.js",
    "input-views.js",
    "layout.js",
    "main.js",
    "navigation.js",
    "profile-views.js",
    "sample.js",
    "settings-client.js",
    "shortcuts.js",
    "scroll-ux.js",
    "state.js",
    "tools.js",
    "view-history.js"
)

foreach ($module in $modules) {
    $path = Join-Path $moduleDir $module
    if (-not (Test-Path $path)) {
        throw "Missing frontend/dist/js/$module"
    }
}

$threeModule = Join-Path $dist "vendor\three.module.js"
if (-not (Test-Path $threeModule)) {
    throw "Missing frontend/dist/vendor/three.module.js"
}

$defaultSample = Join-Path $dist "samples\RefBldgLargeOfficeNew2004_Chicago.idf"
if (-not (Test-Path $defaultSample)) {
    throw "Missing frontend/dist/samples/RefBldgLargeOfficeNew2004_Chicago.idf"
}

Write-Host "Static frontend is ready."
