$ErrorActionPreference = "Stop"

$frontendRoot = Join-Path $PSScriptRoot "..\cmd\semantic-idf\frontend"
$assetRoot = Join-Path $frontendRoot "src"
$index = Join-Path $assetRoot "index.html"
$tools = Join-Path $assetRoot "tools.html"
$guide = Join-Path $assetRoot "guide.html"
$batch = Join-Path $assetRoot "batch.html"
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

if (-not (Test-Path $batch)) {
    throw "Missing frontend/src/batch.html"
}

if (-not (Test-Path $settings)) {
    throw "Missing frontend/src/settings.html"
}

if (-not (Test-Path $entry)) {
    throw "Missing frontend/src/app.js"
}

$modules = @(
    "actions.js",
    "app-info.js",
    "command-palette.js",
    "topology-loader.js",
    "layout.js",
    "main.js",
    "navigation.js",
    "navigation-chooser.js",
    "navigation-link-bar.js",
    "panel-navigation-actions.js",
    "panel-navigation-adapters.js",
    "panel-navigation-registry.js",
    "sample.js",
    "selection-controller.js",
    "semantic-navigation-cache.js",
    "settings-client.js",
    "shortcuts.js",
    "state.js",
    "thermal-topology-targets.js",
    "tools.js",
    "view-history.js"
)

foreach ($module in $modules) {
    $path = Join-Path $moduleDir $module
    if (-not (Test-Path $path)) {
        throw "Missing frontend/src/js/$module"
    }
}

$nestedModules = @(
    "views/analysis-views.js",
    "views/topology-view.js",
    "views/thermal-topology-view.js",
    "views/thermal-topology-layout.js",
    "views/thermal-topology-inspector.js",
    "views/hvac-views.js",
    "views/input-views.js",
    "views/profile-views.js",
    "views/simulation-views.js",
    "tools/multi-simulation.js"
)

foreach ($module in $nestedModules) {
    $path = Join-Path $moduleDir $module
    if (-not (Test-Path $path)) {
        throw "Missing frontend/src/js/$module"
    }
}

$styles = @(
    "styles.css",
    "styles/base.css",
    "styles/topology.css",
    "styles/hvac.css",
    "styles/output.css",
    "styles/profile.css",
    "styles/responsive.css",
    "styles/simulation.css",
    "styles/workspace.css"
)

foreach ($style in $styles) {
    $path = Join-Path $assetRoot $style
    if (-not (Test-Path $path)) {
        throw "Missing frontend/src/$style"
    }
}

$wailsPath = Join-Path $PSScriptRoot "..\cmd\semantic-idf\wails.json"
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
if ($appInfoText -notmatch ('outputFilename:\s*"semantic-idf-v' + [regex]::Escape($productVersion) + '"')) {
    throw "App output filename does not match version $productVersion in frontend/src/js/app-info.js"
}

$staticVersionChecks = @(
    @($tools, 'data-app-brand-version[^>]*>SemanticIDF v' + [regex]::Escape($productVersion) + '<'),
    @($guide, 'data-app-brand-version[^>]*>SemanticIDF v' + [regex]::Escape($productVersion) + '<'),
    @($batch, 'data-app-brand-version[^>]*>SemanticIDF v' + [regex]::Escape($productVersion) + '<'),
    @($settings, 'data-app-brand-version[^>]*>SemanticIDF v' + [regex]::Escape($productVersion) + '<')
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
