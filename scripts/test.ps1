param(
    [switch]$Quick,
    [switch]$Full,
    [string[]]$Area = @(),
    [ValidateSet('frontend', 'backend')][string]$Layer,
    [switch]$Staged,
    [string]$BaseRef,
    [switch]$Plan,
    [switch]$List,
    [ValidatePattern('^\d+(?:ns|us|\xB5s|ms|s|m|h)$')]
    [string]$GoTestTimeout = '20m'
)

$ErrorActionPreference = 'Stop'
$timer = [System.Diagnostics.Stopwatch]::StartNew()
. "$PSScriptRoot\toolchain.ps1"
. "$PSScriptRoot\test-plan.ps1"
$paths = Get-RepoToolchain
$catalog = @(Get-TestCatalog -RepoRoot $paths.RepoRoot)
$Area = @($Area | ForEach-Object { $_.Split(',') } | Where-Object { $_ } | Sort-Object -Unique)
if ($List) {
    $listed = @($catalog | Where-Object { -not $Layer -or $_.Layer -eq $Layer })
    $rows = foreach ($name in @($listed | ForEach-Object { $_.Areas } | Sort-Object -Unique)) {
        $tests = @($listed | Where-Object { $name -in $_.Areas })
        [pscustomobject]@{
            Area = $name
            Fast = @($tests | Where-Object Tier -eq 'fast').Count
            Browser = @($tests | Where-Object Tier -eq 'browser').Count
            Regression = @($tests | Where-Object Tier -eq 'regression').Count
        }
    }
    $rows | Format-Table -AutoSize
    return
}
$selection = New-TestPlan -RepoRoot $paths.RepoRoot -Catalog $catalog -Quick:$Quick -Full:$Full -Area $Area -Layer $Layer -Staged:$Staged -BaseRef $BaseRef
Show-TestPlan -Plan $selection -Catalog $catalog
if ($Plan) { return }
$paths = Use-RepoToolchain -RequireGo
Push-Location $paths.RepoRoot
try {
    if ($selection.Mode -like 'full*') {
        & $paths.GoExe test "-timeout=$GoTestTimeout" '-p=1' ./...
        if ($LASTEXITCODE -ne 0) { throw "Full Go tests failed with exit code $LASTEXITCODE." }
    }
    else {
        foreach ($batch in Get-TestRunBatches -Tests $selection.Tests -Catalog $catalog) {
            $testArgs = @('test', "-timeout=$GoTestTimeout", '-p=1')
            if ($batch.Pattern) { $testArgs += @('-run', $batch.Pattern) }
            $testArgs += $batch.Package
            Write-Host "[run] $($batch.Package): $($batch.Count) tests"
            & $paths.GoExe @testArgs
            if ($LASTEXITCODE -ne 0) { throw "Go tests failed for $($batch.Package) with exit code $LASTEXITCODE." }
        }
    }
}
finally {
    Pop-Location
}
$timer.Stop()
Write-Host ('[ok] Selected Go tests passed in {0:N1}s.' -f $timer.Elapsed.TotalSeconds)
