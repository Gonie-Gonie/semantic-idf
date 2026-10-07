# Select test functions while compiling each complete Go package and its shared helpers.
function Test-AnyPattern {
    param([string]$Value, [string[]]$Patterns)
    foreach ($pattern in $Patterns) { if ($Value -like $pattern) { return $true } }
    return $false
}

function Get-TestCatalog {
    param([string]$RepoRoot)
    $definitions = @{}
    foreach ($file in Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'test-groups-*.json') {
        $manifest = Get-Content -LiteralPath $file.FullName -Raw -Encoding utf8 | ConvertFrom-Json
        if ($manifest.version -ne 1) { throw "Unsupported test manifest: $($file.Name)" }
        foreach ($definition in $manifest.packages) {
            if ($definitions.ContainsKey($definition.package)) { throw "Duplicate test package: $($definition.package)" }
            $definitions[$definition.package] = $definition
        }
    }
    if ($definitions.Count -eq 0) { throw 'No test classification manifests found.' }
    $tierRank = @{ fast = 0; browser = 1; regression = 2 }
    $raw = & git -C $RepoRoot ls-files --cached --others --exclude-standard -z -- '*_test.go'
    if ($LASTEXITCODE -ne 0) { throw 'Failed to discover Go test files.' }
    $files = @(([string]::Join("`n", @($raw))).Split([char]0) | Where-Object { $_ })
    foreach ($relative in $files | Sort-Object -Unique) {
        $absolute = Join-Path $RepoRoot $relative
        if (-not (Test-Path -LiteralPath $absolute)) { continue }
        $separator = $relative.LastIndexOf('/')
        $package = $(if ($separator -lt 0) { '.' } else { './' + $relative.Substring(0, $separator) })
        $filename = Split-Path -Leaf $relative
        $definition = $definitions[$package]
        $fileRules = @()
        if ($definition) { $fileRules = @($definition.rules | Where-Object { Test-AnyPattern $filename $_.files }) }
        $text = Get-Content -LiteralPath $absolute -Raw -Encoding utf8
        foreach ($match in [regex]::Matches($text, '(?m)^func\s+((?:Test|Fuzz|Example)(?!\p{Ll})\w*)\s*\(')) {
            $name = $match.Groups[1].Value
            if ($name -eq 'TestMain') { continue }
            $areas = @(); $tier = 'fast'; $classified = $false
            if ($definition) {
                $areas = @($definition.defaultAreas); $tier = $definition.defaultTier
                if (-not $tierRank.ContainsKey([string]$tier)) { throw "Invalid default test tier in $package" }
                foreach ($rule in $fileRules) {
                    if ($rule.tests -and -not (Test-AnyPattern $name $rule.tests)) { continue }
                    $classified = $true
                    if ($rule.areas) { $areas += @($rule.areas) }
                    if ($rule.tier) {
                        if (-not $tierRank.ContainsKey([string]$rule.tier)) { throw "Invalid test tier: $($rule.tier)" }
                        if ($tierRank[$rule.tier] -gt $tierRank[$tier]) { $tier = $rule.tier }
                    }
                }
            }
            else { $areas = @((Split-Path -Leaf $package)) }
            [pscustomobject]@{
                Package = $package; File = $relative; Name = $name
                Areas = @($areas | Sort-Object -Unique); Tier = $tier; Classified = $classified
                Layer = $(if ($package -like '*/frontendchecks') { 'frontend' } else { 'backend' })
            }
        }
    }
}

function Get-ChangedTestPaths {
    param([string]$RepoRoot, [switch]$Staged, [string]$BaseRef)
    if ($Staged -and $BaseRef) { throw '-Staged and -BaseRef cannot be combined.' }
    # NUL separators preserve spaces and Unicode names; include both sides of renames.
    $diffArgs = @('-C', $RepoRoot, 'diff', '--name-only', '-z', '--no-renames')
    if ($Staged) { $diffArgs += '--cached' }
    elseif ($BaseRef) { $diffArgs += $BaseRef }
    else { $diffArgs += 'HEAD' }
    $diffArgs += '--'
    $raw = & git @diffArgs
    if ($LASTEXITCODE -ne 0) { throw 'Failed to inspect changed files.' }
    $paths = @(([string]::Join("`n", @($raw))).Split([char]0) | Where-Object { $_ })
    if (-not $Staged) {
        $raw = & git -C $RepoRoot ls-files --others --exclude-standard -z
        if ($LASTEXITCODE -ne 0) { throw 'Failed to inspect untracked files.' }
        $paths += @(([string]::Join("`n", @($raw))).Split([char]0) | Where-Object { $_ })
    }
    return @($paths | Sort-Object -Unique)
}

function New-TestPlan {
    param(
        [string]$RepoRoot, [object[]]$Catalog, [switch]$Quick, [switch]$Full,
        [string[]]$Area, [string]$Layer, [switch]$Staged, [string]$BaseRef,
        [AllowEmptyCollection()][string[]]$ChangedPaths
    )
    if ($Staged -and $BaseRef) { throw '-Staged and -BaseRef cannot be combined.' }
    $modeCount = [int]$Quick.IsPresent + [int]$Full.IsPresent + [int]($Area.Count -gt 0)
    if ($modeCount -gt 1) { throw 'Choose one of -Quick, -Full, or -Area.' }
    if (($Staged -or $BaseRef) -and $modeCount -gt 0) { throw 'Change selectors cannot be combined with -Quick, -Full, or -Area.' }
    if ($Full -and $Layer) { throw '-Full always includes every layer.' }
    $knownAreas = @($Catalog | ForEach-Object { $_.Areas } | Sort-Object -Unique)
    foreach ($requested in $Area) {
        if ($requested -notin $knownAreas) { throw "Unknown test area '$requested'. Use -List to see available areas." }
    }
    $selected = [System.Collections.Generic.Dictionary[string, bool]]::new([StringComparer]::Ordinal)
    $reasons = @(); $changes = @(); $mode = 'changed'
    if ($Full) { $mode = 'full' }
    elseif ($Quick) { $mode = 'quick' }
    elseif ($Area.Count -gt 0) { $mode = 'area' }
    foreach ($test in $Catalog) {
        $include = $Full -or (($Quick -or $mode -eq 'changed') -and $test.Tier -eq 'fast')
        if ($mode -eq 'area') { $include = @($test.Areas | Where-Object { $_ -in $Area }).Count -gt 0 }
        if ($Layer -and $test.Layer -ne $Layer) { $include = $false }
        if ($include) { $selected[($test.Package + ':' + $test.Name)] = $true }
    }
    if ($mode -eq 'changed') {
        if ($Layer) { throw '-Layer requires -Quick or -Area; automatic selection includes dependent layers.' }
        if ($PSBoundParameters.ContainsKey('ChangedPaths')) { $changes = @($ChangedPaths) }
        else { $changes = @(Get-ChangedTestPaths -RepoRoot $RepoRoot -Staged:$Staged -BaseRef $BaseRef) }
        $impact = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'test-impact.json') -Raw -Encoding utf8 | ConvertFrom-Json
        if ($impact.version -ne 1) { throw 'Unsupported source impact manifest.' }
        $fullReasons = @()
        foreach ($path in $changes) {
            $matched = $false
            if ($path -like '*_test.go') {
                $separator = $path.LastIndexOf('/')
                $package = $(if ($separator -lt 0) { '.' } else { './' + $path.Substring(0, $separator) })
                $packageTests = @($Catalog | Where-Object { $_.Package -eq $package })
                if ($packageTests.Count -gt 0) {
                    foreach ($test in $packageTests) { $selected[($test.Package + ':' + $test.Name)] = $true }
                    $reasons += "$path -> package $package (shared test helpers)"
                    $matched = $true
                }
            }
            if (-not $matched) {
                # Feature rules precede shared/package fallbacks. First match wins.
                foreach ($rule in $impact.rules) {
                    if (-not (Test-AnyPattern $path $rule.paths)) { continue }
                    $matched = $true
                    if ($rule.full) { $fullReasons += $path; break }
                    if ($rule.ignore) { break }
                    foreach ($test in $Catalog) {
                        if ($rule.packages -and -not (Test-AnyPattern $test.Package $rule.packages)) { continue }
                        if ($rule.areas -and @($test.Areas | Where-Object { $_ -in $rule.areas }).Count -eq 0) { continue }
                        $selected[($test.Package + ':' + $test.Name)] = $true
                    }
                    $reasons += "$path -> $($rule.description)"
                    break
                }
            }
            if (-not $matched) { $fullReasons += $path }
        }
        if ($fullReasons.Count -gt 0) {
            $mode = 'full (fallback)'
            $reasons += 'Shared or unclassified changes require full regression: ' + ($fullReasons -join ', ')
            foreach ($test in $Catalog) { $selected[($test.Package + ':' + $test.Name)] = $true }
        }
    }
    $tests = @($Catalog | Where-Object { $selected.ContainsKey($_.Package + ':' + $_.Name) })
    return [pscustomobject]@{ Mode = $mode; Tests = $tests; Reasons = $reasons; Changes = $changes }
}

function Show-TestPlan {
    param([object]$Plan, [object[]]$Catalog)
    Write-Host "[test] $($Plan.Mode): $($Plan.Tests.Count)/$($Catalog.Count) tests"
    foreach ($reason in $Plan.Reasons) { Write-Host "  $reason" }
    foreach ($group in $Plan.Tests | Group-Object Package -CaseSensitive | Sort-Object Name) {
        $counts = @($group.Group | Group-Object Tier | Sort-Object Name | ForEach-Object { "$($_.Name)=$($_.Count)" })
        Write-Host "  $($group.Name): $($counts -join ', ')"
    }
    $unclassified = @($Catalog | Where-Object { -not $_.Classified } | Select-Object -ExpandProperty File -Unique)
    if ($unclassified.Count -gt 0) { Write-Warning "Unclassified tests are included in fast checks: $($unclassified -join ', ')" }
}

function Get-TestRunBatches {
    param([object[]]$Tests, [object[]]$Catalog)
    foreach ($group in $Tests | Group-Object Package -CaseSensitive | Sort-Object Name) {
        $all = @($Catalog | Where-Object { $_.Package -ceq $group.Name })
        if ($all.Count -eq $group.Count) {
            [pscustomobject]@{ Package = $group.Name; Pattern = ''; Count = $group.Count }
            continue
        }
        # Stay well below Windows' 32K process command-line limit.
        $names = @(); $length = 0
        foreach ($name in $group.Group.Name | Sort-Object -CaseSensitive -Unique) {
            if ($length + $name.Length + 5 -gt 12000 -and $names.Count -gt 0) {
                [pscustomobject]@{ Package = $group.Name; Pattern = '^(' + ($names -join '|') + ')$'; Count = $names.Count }
                $names = @(); $length = 0
            }
            $names += $name; $length += $name.Length + 1
        }
        if ($names.Count -gt 0) { [pscustomobject]@{ Package = $group.Name; Pattern = '^(' + ($names -join '|') + ')$'; Count = $names.Count } }
    }
}
