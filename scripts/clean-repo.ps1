[CmdletBinding(SupportsShouldProcess = $true, PositionalBinding = $false)]
param([switch]$Hard)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path)
$separator = [IO.Path]::DirectorySeparatorChar
$rootPrefix = $repoRoot.TrimEnd([char[]]@('\', '/')) + $separator
$comparison = [StringComparison]::OrdinalIgnoreCase

function Read-Git {
    param([string]$Arguments)

    # Read NUL-delimited UTF-8 paths without PowerShell's native output decoding.
    $start = New-Object Diagnostics.ProcessStartInfo
    $gitCommand = Get-Command git -CommandType Application -ErrorAction Stop | Select-Object -First 1
    $start.FileName = $gitCommand.Source
    $start.Arguments = '-C "' + $repoRoot + '" ' + $Arguments
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $start.StandardOutputEncoding = New-Object Text.UTF8Encoding($false)
    $process = [Diagnostics.Process]::Start($start)
    try {
        $output = $process.StandardOutput.ReadToEnd()
        $errorText = $process.StandardError.ReadToEnd()
        $process.WaitForExit()
        if ($process.ExitCode -ne 0) { throw "Git inspection failed: $errorText" }
        return $output
    }
    finally { $process.Dispose() }
}

function Assert-RepoPath {
    param([string]$Path)

    $full = [IO.Path]::GetFullPath($Path)
    if (-not $full.StartsWith($rootPrefix, $comparison)) {
        throw "Refusing to clean outside the repository: $full"
    }
    $relative = $full.Substring($rootPrefix.Length).Replace('\', '/')
    if ($relative -eq '.git' -or $relative.StartsWith('.git/', $comparison)) {
        throw "Refusing to clean Git metadata: $full"
    }
    return $full
}

function Assert-NoReparsePoint {
    param([string]$Path)

    $item = Get-Item -LiteralPath $Path -Force
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Refusing to clean a symbolic link or junction: $Path"
    }
}

function Assert-SafeAncestors {
    param([string]$Path)

    # Check ancestors before enumerating; never follow a junction during discovery.
    $current = $Path
    while ($current -and $current.StartsWith($rootPrefix, $comparison)) {
        if (Test-Path -LiteralPath $current) { Assert-NoReparsePoint -Path $current }
        $current = [IO.Path]::GetDirectoryName($current)
    }
}

function Assert-SafeTree {
    param([string]$Path)

    Assert-SafeAncestors -Path $Path
    $pending = New-Object 'Collections.Generic.Stack[string]'
    $pending.Push($Path)
    while ($pending.Count -gt 0) {
        $entry = $pending.Pop()
        Assert-NoReparsePoint -Path $entry
        if (Test-Path -LiteralPath $entry -PathType Container) {
            foreach ($child in Get-ChildItem -LiteralPath $entry -Force) {
                if (($child.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                    throw "Refusing to clean a symbolic link or junction: $($child.FullName)"
                }
                if ($child.PSIsContainer) { $pending.Push($child.FullName) }
            }
        }
    }
}

function Test-EmptyDirectoryTree {
    param([string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Container)) { return $false }
    $pending = New-Object 'Collections.Generic.Stack[string]'
    $pending.Push($Path)
    while ($pending.Count -gt 0) {
        $entry = Get-Item -LiteralPath $pending.Pop() -Force
        if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { return $false }
        foreach ($child in Get-ChildItem -LiteralPath $entry.FullName -Force) {
            if (-not $child.PSIsContainer -or ($child.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                return $false
            }
            $pending.Push($child.FullName)
        }
    }
    return $true
}

Assert-NoReparsePoint -Path $repoRoot
$gitRoot = [IO.Path]::GetFullPath((Read-Git 'rev-parse --show-toplevel').Trim())
if (-not $gitRoot.Equals($repoRoot, $comparison)) {
    throw "Cleanup must run from the root of its own Git checkout: $repoRoot"
}
$tracked = @((Read-Git 'ls-files --cached -z').Split([char]0) | Where-Object { $_ })
$plan = New-Object 'Collections.Generic.List[string]'

function Add-CleanupPath {
    param([string]$Path)

    $full = Assert-RepoPath -Path $Path
    $relative = $full.Substring($rootPrefix.Length).Replace('\', '/')
    if (-not $Hard) {
        foreach ($outputDirectory in @('bin', 'build', 'cmd/semantic-idf/frontend/dist')) {
            if ($relative.Equals($outputDirectory, $comparison) -or $relative.StartsWith($outputDirectory + '/', $comparison)) {
                return
            }
        }
    }
    $hasTrackedChild = $false
    foreach ($trackedPath in $tracked) {
        if ($relative.Equals($trackedPath, $comparison) -or $relative.StartsWith($trackedPath + '/', $comparison)) {
            return
        }
        if ($trackedPath.StartsWith($relative + '/', $comparison)) { $hasTrackedChild = $true }
    }
    Assert-SafeAncestors -Path $full
    if (-not (Test-Path -LiteralPath $full)) { return }
    if ($hasTrackedChild) {
        foreach ($child in Get-ChildItem -LiteralPath $full -Force) {
            Add-CleanupPath -Path $child.FullName
        }
        return
    }
    $plan.Add($full)
}

# These locations are generated by this repository's development workflow.
# Ignore rules alone are not a deletion policy: models, .env, editor settings,
# optional external fixtures and unknown untracked files are never selected.
$generatedDirectories = @(
    'cmd/semantic-idf/frontend/wailsjs',
    'node_modules',
    'cmd/semantic-idf/frontend/node_modules'
)
if ($Hard) {
    $generatedDirectories += @('bin', 'build', 'cmd/semantic-idf/frontend/dist', '.runtime', '.tools')
}
else {
    $generatedDirectories += @('.runtime/gocache', '.runtime/guide-review', '.runtime/tmp')
}
foreach ($relative in $generatedDirectories) {
    Add-CleanupPath -Path (Join-Path $repoRoot $relative)
}

# Moving application sources to cmd/semantic-idf can leave empty directories.
# These former roots are not disposable workspaces: any file or link protects
# the whole tree, including optional external fixtures and old build outputs.
foreach ($relative in @('frontend', 'internal')) {
    $directory = Assert-RepoPath -Path (Join-Path $repoRoot $relative)
    if (Test-EmptyDirectoryTree -Path $directory) { Add-CleanupPath -Path $directory }
}

# Legacy browser investigations predate the dedicated .runtime/tmp workspace.
# Match only their known top-level profile and capture names, never arbitrary
# screenshots, browser installations, simulation evidence or comparison baselines.
$browserProfilePattern = '^(?:chrome|edge)-(?:cdp|smoke|hvac-profile)(?:-|$)'
$geometryCapturePattern = '^geometry-(?:\d+x\d+|(?:desktop|mobile)(?:-(?:canvas|cdp(?:-surface-(?:False|True))?|default|disable-software-rasterizer|timeout|use-angle-swiftshader))?)\.png$'
$browserCaptureFiles = @(
    'chrome-dom.txt', 'chrome-dom.err.txt', 'geometry-smoke.html',
    'hvac_http_stdout.log', 'hvac_http_stderr.log', 'hvac_static_screenshot.png'
)
foreach ($relative in @('.', 'cmd/semantic-idf', '.runtime')) {
    if ($Hard -and $relative -eq '.runtime') { continue }
    $directory = Join-Path $repoRoot $relative
    Assert-SafeAncestors -Path $directory
    if (-not (Test-Path -LiteralPath $directory -PathType Container)) { continue }
    Assert-NoReparsePoint -Path $directory
    foreach ($entry in Get-ChildItem -LiteralPath $directory -Force) {
        if ($entry.PSIsContainer) {
            if ($relative -eq '.runtime' -and $entry.Name -match $browserProfilePattern) {
                Add-CleanupPath -Path $entry.FullName
            }
            continue
        }
        $isTestBinary = $entry.Name -like '*.test' -or $entry.Name -like '*.test.exe'
        $isBuildFile = $entry.Name -like '*.syso' -or $entry.Name -eq 'coverage.out'
        $isBrowserCapture = $relative -eq '.runtime' -and ($entry.Name -in $browserCaptureFiles -or $entry.Name -match $geometryCapturePattern)
        if ($isTestBinary -or $isBuildFile -or $isBrowserCapture) { Add-CleanupPath -Path $entry.FullName }
    }
}

# Validate the complete plan before the first removal, including nested links.
foreach ($path in $plan) { Assert-SafeTree -Path $path }
if ($plan.Count -eq 0) {
    Write-Host '[ok] No generated files to clean.'
}
foreach ($path in $plan) {
    $relative = $path.Substring($rootPrefix.Length).Replace('\', '/')
    if ($PSCmdlet.ShouldProcess($path, 'Remove generated repository files')) {
        # Recheck immediately before a recursive operation, using one shell and
        # literal paths throughout discovery, validation and deletion.
        $safePath = Assert-RepoPath -Path $path
        Assert-SafeTree -Path $safePath
        if ($relative -match '^(?:frontend|internal)(?:/|$)' -and -not (Test-EmptyDirectoryTree -Path $safePath)) {
            throw "Legacy source directory is no longer empty: $safePath"
        }
        Remove-Item -LiteralPath $safePath -Recurse -Force
        Write-Host "[removed] $relative"
    }
}
if ($Hard) {
    Write-Host '[ok] Hard cleanup selected. Run dev setup before the next build or test.'
}
else {
    Write-Host '[ok] Build outputs (bin, build, frontend/dist), development tools and retained local evidence are preserved.'
}
