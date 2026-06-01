$ErrorActionPreference = "Stop"

$frontendRoot = Join-Path $PSScriptRoot "..\frontend"
$assetRoot = Join-Path $frontendRoot "src"
$index = Join-Path $assetRoot "index.html"
$tools = Join-Path $assetRoot "tools.html"
$guide = Join-Path $assetRoot "guide.html"
$settings = Join-Path $assetRoot "settings.html"
$entry = Join-Path $assetRoot "app.js"
$moduleDir = Join-Path $assetRoot "js"

if (-not (Test-Path $index)) {
    throw "Missing frontend/src/index.html"
}

if (-not (Test-Path $tools)) {
    throw "Missing frontend/src/tools.html"
}

if (-not (Test-Path $guide)) {
    throw "Missing frontend/src/guide.html"
}

if (-not (Test-Path $settings)) {
    throw "Missing frontend/src/settings.html"
}

if (-not (Test-Path $entry)) {
    throw "Missing frontend/src/app.js"
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
    "output-views.js",
    "profile-views.js",
    "sample.js",
    "settings-client.js",
    "shortcuts.js",
    "state.js",
    "tools.js",
    "view-history.js"
)

foreach ($module in $modules) {
    $path = Join-Path $moduleDir $module
    if (-not (Test-Path $path)) {
        throw "Missing frontend/src/js/$module"
    }
}

$wailsPath = Join-Path $PSScriptRoot "..\wails.json"
$appInfo = Join-Path $moduleDir "app-info.js"
$wailsConfig = Get-Content -LiteralPath $wailsPath -Raw | ConvertFrom-Json
$productVersion = [string]$wailsConfig.info.productVersion
if ([string]::IsNullOrWhiteSpace($productVersion)) {
    throw "Missing info.productVersion in wails.json"
}

$appInfoText = Get-Content -LiteralPath $appInfo -Raw
if ($appInfoText -notmatch 'version:\s*"([^"]+)"') {
    throw "Missing bundled app version in frontend/src/js/app-info.js"
}
if ($Matches[1] -ne $productVersion) {
    throw "App version mismatch: wails.json=$productVersion app-info.js=$($Matches[1])"
}
if ($appInfoText -notmatch ('outputFilename:\s*"idf-analyzer-v' + [regex]::Escape($productVersion) + '"')) {
    throw "App output filename does not match version $productVersion in frontend/src/js/app-info.js"
}

$staticVersionChecks = @(
    @($index, 'data-app-version[^>]*>v' + [regex]::Escape($productVersion) + '<'),
    @($tools, 'data-app-brand-version[^>]*>IDF ANALYZER V' + [regex]::Escape($productVersion) + '<'),
    @($guide, 'data-app-brand-version[^>]*>IDF ANALYZER V' + [regex]::Escape($productVersion) + '<'),
    @($settings, 'data-app-brand-version[^>]*>IDF ANALYZER V' + [regex]::Escape($productVersion) + '<')
)
foreach ($check in $staticVersionChecks) {
    $path = [string]$check[0]
    $pattern = [string]$check[1]
    $text = Get-Content -LiteralPath $path -Raw
    if ($text -notmatch $pattern) {
        throw "Static app version placeholder in $path does not match $productVersion"
    }
}

$threeModule = Join-Path $assetRoot "vendor\three.module.js"
if (-not (Test-Path $threeModule)) {
    throw "Missing frontend/src/vendor/three.module.js"
}

$defaultSample = Join-Path $assetRoot "samples\RefBldgLargeOfficeNew2004_Chicago.idf"
if (-not (Test-Path $defaultSample)) {
    throw "Missing frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf"
}

Write-Host "Static frontend is ready."
