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
	"analysis-stage-queue.js",
    "app-info.js",
    "auxiliary-navigation.js",
    "command-palette.js",
    "comfort-inspection-data.js",
    "guide-manual.js",
    "i18n.js",
    "localized-text.js",
    "topology-loader.js",
    "topology-focus.js",
    "layout.js",
    "main.js",
    "navigation.js",
    "navigation-chooser.js",
    "panel-navigation-actions.js",
    "panel-navigation-adapters.js",
    "panel-navigation-registry.js",
    "panel-navigation-policy.js",
    "sample.js",
    "simulation-result-transport.js",
    "selection-controller.js",
    "semantic-navigation-cache.js",
    "settings-client.js",
    "shortcuts.js",
    "state.js",
    "thermal-topology-targets.js",
    "tools.js",
    "view-history.js",
    "view-presentation.js"
)

foreach ($module in $modules) {
    $path = Join-Path $moduleDir $module
    if (-not (Test-Path $path)) {
        throw "Missing frontend/src/js/$module"
    }
}

$nestedModules = @(
    "locales/index.js",
    "locales/en.js",
    "locales/ko.js",
    "locales/ja.js",
    "locales/hi.js",
    "locales/es.js",
    "locales/fr.js",
    "views/analysis-views.js",
    "views/topology-view.js",
    "views/thermal-topology-view.js",
    "views/thermal-topology-layout.js",
    "views/thermal-topology-details.js",
    "views/hvac-views.js",
    "views/input-views.js",
    "views/profile-views.js",
    "views/simulation-views.js",
    "views/comfort-inspection-view.js",
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
    "styles/comfort-inspection.css",
    "styles/guide-manual.css",
    "styles/workspace.css"
)

foreach ($style in $styles) {
    $path = Join-Path $assetRoot $style
    if (-not (Test-Path $path)) {
        throw "Missing frontend/src/$style"
    }
}

$manualRoot = Join-Path $assetRoot "manual"
$manualManifestPath = Join-Path $manualRoot "manifest.json"
if (-not (Test-Path -LiteralPath $manualManifestPath -PathType Leaf)) {
    throw "Missing frontend/src/manual/manifest.json"
}
$manualManifest = Get-Content -LiteralPath $manualManifestPath -Raw -Encoding UTF8 | ConvertFrom-Json
if ($manualManifest.version -ne 1 -or @($manualManifest.chapters).Count -eq 0) {
    throw "Invalid technical reference manifest"
}
$manualChapterIds = @{}
foreach ($chapter in $manualManifest.chapters) {
    $chapterId = [string]$chapter.id
    if ($chapterId -cnotmatch '^[a-z][a-z0-9-]*$' -or $manualChapterIds.ContainsKey($chapterId)) {
        throw "Invalid or duplicate technical reference chapter ID: $chapterId"
    }
    $manualChapterIds[$chapterId] = $true
    foreach ($language in @("en", "ko")) {
        $expectedFile = "$chapterId.$language.md"
        if ($chapter.file.$language -cne $expectedFile -or [string]::IsNullOrWhiteSpace($chapter.title.$language)) {
            throw "Invalid $language source for technical reference chapter $chapterId"
        }
        $chapterPath = Join-Path $manualRoot $expectedFile
        if (-not (Test-Path -LiteralPath $chapterPath -PathType Leaf)) {
            throw "Missing frontend/src/manual/$expectedFile"
        }
        if ([string]::IsNullOrWhiteSpace((Get-Content -LiteralPath $chapterPath -Raw -Encoding UTF8))) {
            throw "Empty technical reference chapter $expectedFile"
        }
    }
}
if (-not (Test-Path -LiteralPath (Join-Path $manualRoot "metric-guides.json") -PathType Leaf)) {
    throw "Missing frontend/src/manual/metric-guides.json"
}
$koreanMetricPath = Join-Path $manualRoot "metric-guides.ko.json"
if (-not (Test-Path -LiteralPath $koreanMetricPath -PathType Leaf)) {
    throw "Missing frontend/src/manual/metric-guides.ko.json"
}
$koreanMetrics = Get-Content -LiteralPath $koreanMetricPath -Raw -Encoding UTF8 | ConvertFrom-Json
if ($koreanMetrics.version -ne 1 -or @($koreanMetrics.guides.PSObject.Properties).Count -eq 0) {
    throw "Invalid Korean metric description overlay"
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
