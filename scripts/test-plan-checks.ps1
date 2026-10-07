$ErrorActionPreference = 'Stop'

. "$PSScriptRoot\test-plan.ps1"
$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path

function Assert-SelectorCondition {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw "Test selector contract failed: $Message" }
}

function Get-SelectorKeys {
    param([AllowEmptyCollection()][object[]]$Tests)
    return @($Tests | ForEach-Object { $_.Package + ':' + $_.Name } | Sort-Object -CaseSensitive -Unique)
}

function Assert-SelectorSet {
    param([object]$Plan, [AllowEmptyCollection()][object[]]$Expected, [string]$Message)
    $actualKeys = @(Get-SelectorKeys -Tests $Plan.Tests)
    $expectedKeys = @(Get-SelectorKeys -Tests $Expected)
    Assert-SelectorCondition ($Plan.Tests.Count -eq $actualKeys.Count) "$Message (duplicate selections)"
    if (($actualKeys -join "`n") -cne ($expectedKeys -join "`n")) {
        $missing = @($expectedKeys | Where-Object { $_ -cnotin $actualKeys } | Select-Object -First 8)
        $extra = @($actualKeys | Where-Object { $_ -cnotin $expectedKeys } | Select-Object -First 8)
        Assert-SelectorCondition $false "$Message (missing: $($missing -join ', '); extra: $($extra -join ', '))"
    }
}

function Assert-SelectorThrows {
    param([scriptblock]$Action, [string]$ExpectedError, [string]$Message)
    $caught = $false
    try { & $Action | Out-Null }
    catch {
        $caught = $true
        Assert-SelectorCondition ($_.Exception.Message -match $ExpectedError) "$Message (unexpected error: $($_.Exception.Message))"
    }
    Assert-SelectorCondition $caught "$Message (no error)"
}

function New-SelectorFixture {
    param([string]$Package, [string]$Name, [string[]]$Areas, [string]$Tier = 'fast', [bool]$Classified = $true)
    return [pscustomobject]@{
        Package = $Package; File = 'synthetic/' + $Name + '_test.go'; Name = $Name
        Areas = $Areas; Tier = $Tier; Classified = $Classified; Layer = 'backend'
    }
}

# Read the live catalog, then supply explicit paths so these checks neither
# depend on the worktree's current edits nor change Git state.
$catalog = @(Get-TestCatalog -RepoRoot $repoRoot)
Assert-SelectorCondition ($catalog.Count -gt 0) 'the live catalog must contain tests'
Assert-SelectorCondition (@($catalog | Where-Object Tier -eq 'browser').Count -gt 0) 'browser classification must exist'
Assert-SelectorCondition (@($catalog | Where-Object Tier -eq 'regression').Count -gt 0) 'regression classification must exist'

$fastTests = @($catalog | Where-Object Tier -eq 'fast')
$quick = New-TestPlan -RepoRoot $repoRoot -Catalog $catalog -Quick
Assert-SelectorSet $quick $fastTests 'Quick must contain exactly the fast tests'
Assert-SelectorCondition (@($quick.Tests | Where-Object { $_.Tier -in @('browser', 'regression') }).Count -eq 0) 'Quick must not start browser or regression tests'

$profileFrontend = @($catalog | Where-Object { $_.Layer -eq 'frontend' -and 'profile' -in $_.Areas })
$profile = New-TestPlan -RepoRoot $repoRoot -Catalog $catalog -Area @('profile') -Layer frontend
Assert-SelectorSet $profile $profileFrontend 'Area profile with Layer frontend must stay in that feature and layer'
Assert-SelectorCondition (@($profile.Tests | Where-Object Tier -eq 'browser').Count -gt 0) 'explicit Area selection must retain its browser coverage'

$baseline = New-TestPlan -RepoRoot $repoRoot -Catalog $catalog -ChangedPaths @()
Assert-SelectorSet $baseline $fastTests 'an unchanged worktree must retain the fast baseline'
Assert-SelectorCondition ($baseline.Mode -eq 'changed') 'an unchanged worktree must not request full regression'

$profileSource = 'cmd/semantic-idf/frontend/src/js/views/profile-views.js'
$profileChange = New-TestPlan -RepoRoot $repoRoot -Catalog $catalog -ChangedPaths @($profileSource)
Assert-SelectorSet $profileChange @($fastTests + $profileFrontend | Sort-Object Package, Name -Unique) 'a Profile JavaScript edit must add exactly its frontend coverage to the fast baseline'
Assert-SelectorCondition (@($profileChange.Tests | Where-Object { $_.Name -eq 'TestProfileLayoutAndSelectionBrowserHarness' }).Count -eq 1) 'a Profile edit must exercise its layout browser harness'
Assert-SelectorCondition (@($profileChange.Tests | Where-Object { $_.Tier -eq 'regression' -and $_.Package -like '*/simulation' }).Count -eq 0) 'a Profile UI edit must not trigger backend simulation regression'
Assert-SelectorCondition (@($profileChange.Tests | Where-Object { $_.Tier -eq 'browser' -and 'profile' -notin $_.Areas }).Count -eq 0) 'a Profile UI edit must not start unrelated browser tests'

foreach ($sharedPath in @('new-feature/runtime/custom-source.go', 'cmd/semantic-idf/internal/epinput/parse.go')) {
    $fallback = New-TestPlan -RepoRoot $repoRoot -Catalog $catalog -ChangedPaths @($sharedPath)
    Assert-SelectorSet $fallback $catalog "$sharedPath must select the full catalog"
    Assert-SelectorCondition ($fallback.Mode -like 'full*') "$sharedPath must report full fallback"
}

Assert-SelectorThrows {
    Get-ChangedTestPaths -RepoRoot $repoRoot -Staged -BaseRef HEAD
} 'Staged.*BaseRef' 'staged and base selectors must be mutually exclusive'
Assert-SelectorThrows {
    New-TestPlan -RepoRoot $repoRoot -Catalog $catalog -Staged -BaseRef HEAD -ChangedPaths @()
} 'Staged.*BaseRef' 'explicit changed paths must not bypass staged/base validation'
Assert-SelectorThrows {
    New-TestPlan -RepoRoot $repoRoot -Catalog $catalog -Area @('selector-contract-unknown-area')
} 'Unknown test area' 'unknown areas must fail instead of silently selecting no tests'

$duplicateChanges = New-TestPlan -RepoRoot $repoRoot -Catalog $catalog -ChangedPaths @($profileSource, $profileSource)
Assert-SelectorSet $duplicateChanges $profileChange.Tests 'repeated changed paths must select each test once'
$areaUnion = New-TestPlan -RepoRoot $repoRoot -Catalog $catalog -Area @('profile', 'hvac', 'profile')
$expectedUnion = @($catalog | Where-Object { 'profile' -in $_.Areas -or 'hvac' -in $_.Areas })
Assert-SelectorSet $areaUnion $expectedUnion 'overlapping and repeated areas must produce an exact union'

$unclassified = New-SelectorFixture './synthetic/new-package' 'TestNewUnclassifiedFeature' @('new-feature') -Classified $false
$extendedCatalog = @($catalog) + @($unclassified)
$extendedQuick = New-TestPlan -RepoRoot $repoRoot -Catalog $extendedCatalog -Quick
Assert-SelectorSet $extendedQuick @($fastTests + @($unclassified)) 'new unclassified tests must remain in Quick'

$caseCatalog = @(
    New-SelectorFixture './synthetic/case' 'TestCase' @('case-lower')
    New-SelectorFixture './synthetic/case' 'TestCASE' @('case-upper')
)
$casePlan = New-TestPlan -RepoRoot $repoRoot -Catalog $caseCatalog -Area @('case-lower')
Assert-SelectorSet $casePlan @($caseCatalog[0]) 'Go test names differing only by case must remain distinct'

# Long legal Go test names force several command-line batches. The adjacent
# unselected name checks both anchors: a selected prefix must not run its suffix.
$selectedBatchTests = @(
    foreach ($index in 1..260) {
        $name = 'TestBatch_' + ('X' * 80) + '_' + ('{0:D4}' -f $index)
        New-SelectorFixture './synthetic/batching' $name @('batching')
    }
    New-SelectorFixture './synthetic/second-package' 'TestOtherSelected' @('batching')
    New-SelectorFixture './synthetic/batching' 'TestCase' @('batching')
    New-SelectorFixture './synthetic/batching' 'TestCASE' @('batching')
)
$excludedBatchTests = @(
    New-SelectorFixture './synthetic/batching' ($selectedBatchTests[0].Name + 'Extra') @('excluded')
    New-SelectorFixture './synthetic/batching' ('Prefix' + $selectedBatchTests[0].Name) @('excluded')
    New-SelectorFixture './synthetic/second-package' 'TestOtherExcluded' @('excluded')
)
$batchCatalog = @($selectedBatchTests) + @($excludedBatchTests)
$batches = @(Get-TestRunBatches -Tests $selectedBatchTests -Catalog $batchCatalog)
Assert-SelectorCondition ($batches.Count -gt 2) 'long test selections must split into multiple batches'
$hits = [System.Collections.Generic.Dictionary[string, int]]::new([StringComparer]::Ordinal)
foreach ($batch in $batches) {
    Assert-SelectorCondition ($batch.Pattern.Length -le 12000) 'batch regular expressions must stay within the 12000-character limit'
    $matched = @($batchCatalog | Where-Object { $_.Package -ceq $batch.Package -and [regex]::IsMatch($_.Name, $batch.Pattern) })
    Assert-SelectorCondition ($matched.Count -eq $batch.Count) 'each batch must match exactly its declared test count'
    foreach ($test in $matched) {
        $key = $test.Package + ':' + $test.Name
        if (-not $hits.ContainsKey($key)) { $hits[$key] = 0 }
        $hits[$key]++
    }
}
foreach ($test in $selectedBatchTests) {
    $key = $test.Package + ':' + $test.Name
    Assert-SelectorCondition ($hits.ContainsKey($key) -and $hits[$key] -eq 1) 'batch union must cover every selected test exactly once'
}
foreach ($test in $excludedBatchTests) {
    Assert-SelectorCondition (-not $hits.ContainsKey($test.Package + ':' + $test.Name)) 'batch patterns must never include an unselected test'
}
$completePackage = @(Get-TestRunBatches -Tests $selectedBatchTests -Catalog $selectedBatchTests)
Assert-SelectorCondition ($completePackage.Count -eq 2) 'complete package selection must yield one batch per package'
Assert-SelectorCondition (@($completePackage | Where-Object { $_.Pattern }).Count -eq 0) 'complete package selection must retain normal Go package execution'

# Exercise real Git discovery without touching the developer's repository.
# Code points keep this script readable by Windows PowerShell 5 without a BOM.
$temporaryParent = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd([IO.Path]::DirectorySeparatorChar)
$temporaryBoundary = $temporaryParent + [IO.Path]::DirectorySeparatorChar
$fixtureRoot = [IO.Path]::GetFullPath((Join-Path $temporaryParent ('idf-test-selector-' + [guid]::NewGuid().ToString('N'))))
Assert-SelectorCondition ($fixtureRoot.StartsWith($temporaryBoundary, [StringComparison]::OrdinalIgnoreCase)) 'fixture creation must remain within the temporary directory'
# Hooks may export an absolute index/worktree path. Temporary Git operations
# must not inherit those paths and write into the developer's repository.
$savedGitEnvironment = @{}
foreach ($variable in Get-ChildItem Env: | Where-Object Name -like 'GIT_*') {
    $savedGitEnvironment[$variable.Name] = $variable.Value
    Remove-Item -LiteralPath ('Env:' + $variable.Name)
}
try {
    New-Item -ItemType Directory -Path (Join-Path $fixtureRoot 'unicode') -Force | Out-Null
    & git -C $fixtureRoot init -q
    Assert-SelectorCondition ($LASTEXITCODE -eq 0) 'temporary Git initialization must succeed'
    $unicodeName = 'Test' + [char]0x03A9
    $lowerUnicodeName = 'Test' + [char]0x03C9
    $unicodeFile = 'unicode/' + [char]0xD55C + [char]0xAE00 + ' space_test.go'
    $source = @(
        'package fixture',
        'import "testing"',
        'func Test(t *testing.T) {}',
        'func TestBare(t *testing.T) {}',
        "func $unicodeName(t *testing.T) {}",
        "func $lowerUnicodeName(t *testing.T) {}",
        'func Testlower(t *testing.T) {}',
        'func TestMain(m *testing.M) {}',
        'func Fuzz(f *testing.F) {}',
        'func FuzzBoundary(f *testing.F) {}',
        'func Fuzzlower(f *testing.F) {}',
        'func Example() {',
        '    // Output:',
        '}',
        'func Example_fixture() {',
        '    // Output:',
        '}',
        'func Examplelower() {}'
    ) -join "`n"
    $fixtureEncoding = [System.Text.UTF8Encoding]::new($false)
    [IO.File]::WriteAllText((Join-Path $fixtureRoot $unicodeFile), $source, $fixtureEncoding)
    [IO.File]::WriteAllText((Join-Path $fixtureRoot 'root_test.go'), "package fixture`nimport `"testing`"`nfunc TestRoot(t *testing.T) {}`n", $fixtureEncoding)
    & git -c core.autocrlf=false -C $fixtureRoot add -- $unicodeFile
    Assert-SelectorCondition ($LASTEXITCODE -eq 0) 'temporary tracked Unicode fixture must be added successfully'

    $fixtureCatalog = @(Get-TestCatalog -RepoRoot $fixtureRoot)
    $fixturePlan = New-TestPlan -RepoRoot $fixtureRoot -Catalog $fixtureCatalog -Quick
    $expectedFixture = @(
        New-SelectorFixture '.' 'TestRoot' @('fixture') -Classified $false
        New-SelectorFixture './unicode' 'Test' @('fixture') -Classified $false
        New-SelectorFixture './unicode' 'TestBare' @('fixture') -Classified $false
        New-SelectorFixture './unicode' $unicodeName @('fixture') -Classified $false
        New-SelectorFixture './unicode' 'Fuzz' @('fixture') -Classified $false
        New-SelectorFixture './unicode' 'FuzzBoundary' @('fixture') -Classified $false
        New-SelectorFixture './unicode' 'Example' @('fixture') -Classified $false
        New-SelectorFixture './unicode' 'Example_fixture' @('fixture') -Classified $false
    )
    Assert-SelectorSet $fixturePlan $expectedFixture 'Git discovery must include root, bare Test, Unicode, Fuzz and Example names while excluding lowercase helpers and TestMain'
    Assert-SelectorCondition (@($fixtureCatalog | Where-Object { $_.File -ceq $unicodeFile }).Count -eq ($expectedFixture.Count - 1)) 'tracked filenames with Unicode and spaces must survive discovery unchanged'
    Assert-SelectorCondition (@($fixtureCatalog | Where-Object { $_.File -eq 'root_test.go' -and $_.Package -eq '.' }).Count -eq 1) 'untracked root test files must map to the root package'
    Assert-SelectorCondition (@($fixtureCatalog | Where-Object Classified).Count -eq 0) 'new packages must remain unclassified and included in Quick'
}
finally {
    foreach ($name in $savedGitEnvironment.Keys) { Set-Item -LiteralPath ('Env:' + $name) -Value $savedGitEnvironment[$name] }
    if (Test-Path -LiteralPath $fixtureRoot) {
        $resolvedFixture = (Resolve-Path -LiteralPath $fixtureRoot).Path
        Assert-SelectorCondition ($resolvedFixture.StartsWith($temporaryBoundary, [StringComparison]::OrdinalIgnoreCase) -and $resolvedFixture.Equals($fixtureRoot, [StringComparison]::OrdinalIgnoreCase)) 'fixture cleanup must target exactly the created temporary directory'
        Remove-Item -LiteralPath $resolvedFixture -Recurse -Force
    }
}

Write-Output "Test selector behavior checks passed ($($catalog.Count) live tests)."
