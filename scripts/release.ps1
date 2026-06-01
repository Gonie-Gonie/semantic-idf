param(
    [ValidateSet("auto", "major", "minor", "patch")]
    [string]$Bump = "auto",
    [string]$Version = "",
    [switch]$Package,
    [switch]$Commit,
    [switch]$Tag,
    [switch]$Push,
    [switch]$Publish,
    [switch]$ExistingTag,
    [switch]$Draft,
    [switch]$Prerelease,
    [switch]$KeepUnreleased,
    [switch]$AllowDirty
)

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\toolchain.ps1"

function Normalize-NewLine {
    param([string]$Text)

    return (($Text -replace "`r`n", "`n") -replace "`r", "`n")
}

function Write-TextFile {
    param(
        [string]$Path,
        [string]$Text
    )

    $parent = Split-Path -Parent $Path
    if ($parent -and -not (Test-Path -LiteralPath $parent)) {
        New-Item -ItemType Directory -Path $parent | Out-Null
    }

    $normalized = Normalize-NewLine -Text $Text
    if (-not $normalized.EndsWith("`n")) {
        $normalized += "`n"
    }
    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $normalized, $encoding)
}

function Get-WailsProductVersion {
    param([string]$Path)

    $config = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    if (-not $config.info -or -not $config.info.productVersion) {
        throw "Missing info.productVersion in $Path"
    }
    return [string]$config.info.productVersion
}

function Get-WailsProductName {
    param([string]$Path)

    $config = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    if ($config.info -and $config.info.productName) {
        return [string]$config.info.productName
    }
    if ($config.name) {
        return [string]$config.name
    }
    return "IDF Analyzer"
}

function Set-WailsReleaseMetadata {
    param(
        [string]$Path,
        [string]$TargetVersion
    )

    $text = Normalize-NewLine -Text (Get-Content -LiteralPath $Path -Raw)
    $text = Set-JsonStringProperty -Text $text -Name "productVersion" -Value $TargetVersion
    $text = Set-JsonStringProperty -Text $text -Name "outputfilename" -Value "idf-analyzer-v$TargetVersion"
    Write-TextFile -Path $Path -Text $text
}

function Set-JsonStringProperty {
    param(
        [string]$Text,
        [string]$Name,
        [string]$Value
    )

    $pattern = '("' + [regex]::Escape($Name) + '"\s*:\s*)"[^"]*"'
    $regex = New-Object System.Text.RegularExpressions.Regex($pattern)
    if (-not $regex.IsMatch($Text)) {
        throw "Missing JSON string property: $Name"
    }
    $evaluator = [System.Text.RegularExpressions.MatchEvaluator]{
        param($match)
        return $match.Groups[1].Value + '"' + $Value + '"'
    }
    return $regex.Replace($Text, $evaluator, 1)
}

function Set-AppInfoModuleVersion {
    param(
        [string]$Path,
        [string]$TargetVersion
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        throw "Missing app info module: $Path"
    }

    $text = Normalize-NewLine -Text (Get-Content -LiteralPath $Path -Raw)
    $versionRegex = New-Object System.Text.RegularExpressions.Regex('version:\s*"[^"]+"')
    $titleRegex = New-Object System.Text.RegularExpressions.Regex('title:\s*"[^"]+"')
    $outputRegex = New-Object System.Text.RegularExpressions.Regex('outputFilename:\s*"[^"]+"')
    $text = $versionRegex.Replace($text, "version: `"$TargetVersion`"", 1)
    $text = $titleRegex.Replace($text, "title: `"IDF Analyzer v$TargetVersion`"", 1)
    $text = $outputRegex.Replace($text, "outputFilename: `"idf-analyzer-v$TargetVersion`"", 1)
    Write-TextFile -Path $Path -Text $text
}

function Set-StaticHTMLAppVersion {
    param(
        [string[]]$Paths,
        [string]$ProductName,
        [string]$TargetVersion
    )

    $brandLabel = "$($ProductName.ToUpperInvariant()) V$TargetVersion"
    foreach ($path in $Paths) {
        if (-not (Test-Path -LiteralPath $path)) {
            throw "Missing static HTML file: $path"
        }

        $text = Normalize-NewLine -Text (Get-Content -LiteralPath $path -Raw)
        $text = [regex]::Replace(
            $text,
            '(?i)(<[^>]*\bdata-app-version\b[^>]*>)v\d+\.\d+\.\d+(</[^>]+>)',
            [System.Text.RegularExpressions.MatchEvaluator]{
                param($match)
                return $match.Groups[1].Value + "v$TargetVersion" + $match.Groups[2].Value
            }
        )
        $text = [regex]::Replace(
            $text,
            '(?i)(<[^>]*\bdata-app-brand-version\b[^>]*>)[^<]*(</[^>]+>)',
            [System.Text.RegularExpressions.MatchEvaluator]{
                param($match)
                return $match.Groups[1].Value + $brandLabel + $match.Groups[2].Value
            }
        )
        Write-TextFile -Path $path -Text $text
    }
}

function Parse-SemVer {
    param([string]$Text)

    $value = $Text.Trim()
    if ($value.StartsWith("v")) {
        $value = $value.Substring(1)
    }

    if ($value -notmatch "^(\d+)\.(\d+)\.(\d+)$") {
        throw "Version must use MAJOR.MINOR.PATCH format: $Text"
    }

    return [pscustomobject]@{
        Major = [int]$Matches[1]
        Minor = [int]$Matches[2]
        Patch = [int]$Matches[3]
        Text = "$($Matches[1]).$($Matches[2]).$($Matches[3])"
    }
}

function Add-SemVerBump {
    param(
        [string]$CurrentVersion,
        [string]$BumpKind
    )

    $version = Parse-SemVer -Text $CurrentVersion
    switch ($BumpKind) {
        "major" { return "$($version.Major + 1).0.0" }
        "minor" { return "$($version.Major).$($version.Minor + 1).0" }
        "patch" { return "$($version.Major).$($version.Minor).$($version.Patch + 1)" }
        default { throw "Unknown bump kind: $BumpKind" }
    }
}

function Get-LatestReleaseTag {
    param([string]$RepoRoot)

    if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
        return ""
    }

    $tags = @(git -C $RepoRoot tag --list "v[0-9]*" --sort=-version:refname)
    if ($tags.Count -eq 0) {
        return ""
    }
    return [string]$tags[0]
}

function Assert-CleanGitTree {
    param([string]$RepoRoot)

    if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
        throw "git is required for commit/tag/push/publish release operations."
    }

    $status = @(git -C $RepoRoot status --porcelain)
    if ($status.Count -gt 0) {
        throw "Working tree must be clean for commit/tag/push/publish. Commit or stash changes, or pass -AllowDirty for local prepare-only testing."
    }
}

function Read-ReleaseNoteBody {
    param([string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return [pscustomobject]@{
            Body = ""
            HasEntries = $false
        }
    }

    $raw = Normalize-NewLine -Text (Get-Content -LiteralPath $Path -Raw)
    $lines = $raw -split "`n"
    $kept = New-Object System.Collections.Generic.List[string]
    $inComment = $false

    foreach ($line in $lines) {
        $trimmed = $line.Trim()

        if ($trimmed.StartsWith("<!--")) {
            $inComment = $true
        }
        if ($inComment) {
            if ($trimmed.EndsWith("-->")) {
                $inComment = $false
            }
            continue
        }

        if ($line -match "^\s*#\s+") {
            continue
        }
        if ($line -match "^\s*Released:\s*") {
            continue
        }
        if ($line -match "^\s*-\s*_(None|TBD|Add entries before release)\._\s*$") {
            continue
        }

        $kept.Add($line) | Out-Null
    }

    while ($kept.Count -gt 0 -and [string]::IsNullOrWhiteSpace($kept[0])) {
        $kept.RemoveAt(0)
    }
    while ($kept.Count -gt 0 -and [string]::IsNullOrWhiteSpace($kept[$kept.Count - 1])) {
        $kept.RemoveAt($kept.Count - 1)
    }

    $body = ($kept.ToArray() -join "`n").Trim()
    $body = [regex]::Replace($body, "(?ms)^##[^\n]*(?:\n[ \t]*)*(?=^##|\z)", "").Trim()
    $hasEntries = $false
    foreach ($line in ($body -split "`n")) {
        if ($line -match "^\s*-\s+\S") {
            $hasEntries = $true
            break
        }
    }

    return [pscustomobject]@{
        Body = $body
        HasEntries = $hasEntries
    }
}

function Get-ReleaseNoteSource {
    param(
        [string]$UnreleasedPath,
        [string]$VersionedPath
    )

    $unreleased = Read-ReleaseNoteBody -Path $UnreleasedPath
    if ($unreleased.HasEntries) {
        return [pscustomobject]@{
            Body = $unreleased.Body
            Source = "unreleased"
        }
    }

    $versioned = Read-ReleaseNoteBody -Path $VersionedPath
    if ($versioned.HasEntries) {
        return [pscustomobject]@{
            Body = $versioned.Body
            Source = "versioned"
        }
    }

    throw "Release notes are empty. Add entries to $UnreleasedPath before preparing a release."
}

function Get-InferredBump {
    param([string]$ReleaseNoteBody)

    if ($ReleaseNoteBody -match "(?im)^##\s+Breaking Changes\b" -or $ReleaseNoteBody -match "(?im)\bBREAKING CHANGE\b") {
        return "major"
    }

    if ($ReleaseNoteBody -match "(?im)^##\s+(Added|Features?)\b") {
        return "minor"
    }

    return "patch"
}

function Get-TargetVersion {
    param(
        [string]$CurrentVersion,
        [string]$RequestedVersion,
        [string]$RequestedBump,
        [string]$ReleaseNoteBody,
        [string]$LatestTag
    )

    if (-not [string]::IsNullOrWhiteSpace($RequestedVersion)) {
        $parsed = Parse-SemVer -Text $RequestedVersion
        return [pscustomobject]@{
            Version = $parsed.Text
            Bump = "explicit"
        }
    }

    if ([string]::IsNullOrWhiteSpace($LatestTag) -and $RequestedBump -eq "auto" -and $CurrentVersion -ne "0.0.0") {
        return [pscustomobject]@{
            Version = $CurrentVersion
            Bump = "initial"
        }
    }

    $bumpKind = $RequestedBump
    if ($bumpKind -eq "auto") {
        $bumpKind = Get-InferredBump -ReleaseNoteBody $ReleaseNoteBody
    }

    return [pscustomobject]@{
        Version = Add-SemVerBump -CurrentVersion $CurrentVersion -BumpKind $bumpKind
        Bump = $bumpKind
    }
}

function Update-Changelog {
    param(
        [string]$Path,
        [string]$TargetVersion,
        [string]$ReleaseDate,
        [string]$ReleaseNoteBody
    )

    $header = "# Changelog`n`nAll notable changes to IDF Analyzer are recorded here from release notes.`n`n"
    if (Test-Path -LiteralPath $Path) {
        $text = Normalize-NewLine -Text (Get-Content -LiteralPath $Path -Raw)
    } else {
        $text = $header
    }

    if ([string]::IsNullOrWhiteSpace($text)) {
        $text = $header
    }

    $escapedVersion = [regex]::Escape($TargetVersion)
    $sectionPattern = "(?ms)^## \[$escapedVersion\] - \d{4}-\d{2}-\d{2}\n.*?(?=^## \[|\z)"
    $text = [regex]::Replace($text, $sectionPattern, "")
    $text = $text.TrimEnd() + "`n`n"

    $entry = "## [$TargetVersion] - $ReleaseDate`n`n$ReleaseNoteBody`n`n"
    $firstVersionSection = [regex]::Match($text, "(?m)^## \[")
    if ($firstVersionSection.Success) {
        $text = $text.Substring(0, $firstVersionSection.Index) + $entry + $text.Substring($firstVersionSection.Index)
    } else {
        $text += $entry
    }

    Write-TextFile -Path $Path -Text $text.TrimEnd()
}

function Write-VersionedReleaseNotes {
    param(
        [string]$Path,
        [string]$TargetVersion,
        [string]$ReleaseDate,
        [string]$ReleaseNoteBody
    )

    $content = "# IDF Analyzer v$TargetVersion`n`nReleased: $ReleaseDate`n`n$ReleaseNoteBody"
    Write-TextFile -Path $Path -Text $content
}

function Reset-UnreleasedReleaseNotes {
    param([string]$Path)

    $content = @"
# Unreleased Release Notes

<!--
Add release-note entries under the section that best describes the change.
The release script infers bump size from these sections:
- Breaking Changes: major
- Added or Features: minor
- Fixed, Changed, Performance, Security, Documentation, or internal-only notes: patch
-->

## Breaking Changes

- _None._

## Added

- _None._

## Changed

- _None._

## Fixed

- _None._
"@
    Write-TextFile -Path $Path -Text $content
}

function New-ReleasePackage {
    param(
        [string]$RepoRoot,
        [string]$TargetVersion,
        [string]$OutputFilename,
        [string]$OutputDir
    )

    $verifyOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot "verify.ps1")
    $verifyExitCode = $LASTEXITCODE
    foreach ($line in $verifyOutput) {
        Write-Host $line
    }
    if ($verifyExitCode -ne 0) {
        throw "Release verification failed."
    }

    $exePath = Join-Path $RepoRoot "build\bin\$OutputFilename.exe"
    if (-not (Test-Path -LiteralPath $exePath)) {
        throw "Expected build output was not found: $exePath"
    }

    if (-not (Test-Path -LiteralPath $OutputDir)) {
        New-Item -ItemType Directory -Path $OutputDir | Out-Null
    }

    $zipPath = Join-Path $OutputDir "idf-analyzer-v$TargetVersion-windows-amd64.zip"
    if (Test-Path -LiteralPath $zipPath) {
        Remove-Item -LiteralPath $zipPath -Force
    }

    Compress-Archive -Path $exePath -DestinationPath $zipPath
    return $zipPath
}

function Convert-ToGitPath {
    param(
        [string]$RepoRoot,
        [string]$Path
    )

    $rootFull = [System.IO.Path]::GetFullPath($RepoRoot).TrimEnd([char[]]@('\', '/'))
    $pathFull = [System.IO.Path]::GetFullPath($Path)
    if (-not $pathFull.StartsWith($rootFull + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Path is outside the repository: $Path"
    }

    return $pathFull.Substring($rootFull.Length + 1).Replace('\', '/')
}

function Invoke-GitReleaseCommit {
    param(
        [string]$RepoRoot,
        [string[]]$Paths,
        [string]$TargetVersion
    )

    foreach ($path in $Paths) {
        if (Test-Path -LiteralPath $path) {
            git -C $RepoRoot add -- (Convert-ToGitPath -RepoRoot $RepoRoot -Path $path)
        }
    }

    git -C $RepoRoot diff --cached --quiet
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[release] No tracked release metadata changes to commit."
        return
    }

    git -C $RepoRoot commit -m "Release v$TargetVersion"
}

function Invoke-GitTag {
    param(
        [string]$RepoRoot,
        [string]$TargetVersion
    )

    $tagName = "v$TargetVersion"
    git -C $RepoRoot rev-parse -q --verify "refs/tags/$tagName" *> $null
    if ($LASTEXITCODE -eq 0) {
        throw "Tag already exists: $tagName"
    }

    git -C $RepoRoot tag -a $tagName -m "Release $tagName"
}

function Assert-ExistingGitTag {
    param(
        [string]$RepoRoot,
        [string]$TargetVersion
    )

    $tagName = "v$TargetVersion"
    git -C $RepoRoot rev-parse -q --verify "refs/tags/$tagName" *> $null
    if ($LASTEXITCODE -ne 0) {
        throw "Existing tag was requested but tag was not found: $tagName"
    }
}

function Invoke-GitPush {
    param(
        [string]$RepoRoot,
        [string]$TargetVersion
    )

    $branch = $env:GITHUB_REF_NAME
    if ([string]::IsNullOrWhiteSpace($branch)) {
        $branch = (git -C $RepoRoot branch --show-current | Out-String).Trim()
    }
    if ([string]::IsNullOrWhiteSpace($branch)) {
        throw "Could not determine branch name for push."
    }

    git -C $RepoRoot push origin "HEAD:$branch"
    git -C $RepoRoot push origin "v$TargetVersion"
}

function Test-GitHubReleaseExists {
    param([string]$TagName)

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        gh release view $TagName 1>$null 2>$null
        return $LASTEXITCODE -eq 0
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
}

function Invoke-GitHubRelease {
    param(
        [string]$TargetVersion,
        [string]$ReleaseNotesPath,
        [string]$AssetPath,
        [switch]$DraftRelease,
        [switch]$PrereleaseRelease
    )

    if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
        throw "GitHub CLI (gh) is required to publish the release."
    }
    if (-not (Test-Path -LiteralPath $AssetPath)) {
        throw "Release asset is missing: $AssetPath"
    }

    $tagName = "v$TargetVersion"
    if (Test-GitHubReleaseExists -TagName $tagName) {
        Write-Host "[release] GitHub Release already exists: $tagName"
        $editArgs = @(
            "release",
            "edit",
            $tagName,
            "--title",
            "IDF Analyzer $tagName",
            "--notes-file",
            $ReleaseNotesPath
        )
        if ($DraftRelease) {
            $editArgs += "--draft"
        }
        if ($PrereleaseRelease) {
            $editArgs += "--prerelease"
        }

        gh @editArgs
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to update GitHub Release: $tagName"
        }

        gh release upload $tagName $AssetPath --clobber
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to upload release asset: $AssetPath"
        }
        return
    }

    $args = @(
        "release",
        "create",
        $tagName,
        $AssetPath,
        "--title",
        "IDF Analyzer $tagName",
        "--notes-file",
        $ReleaseNotesPath
    )
    if ($DraftRelease) {
        $args += "--draft"
    }
    if ($PrereleaseRelease) {
        $args += "--prerelease"
    }

    gh @args
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to create GitHub Release: $tagName"
    }
}

$repoRoot = Get-RepoRoot
$releaseNotesDir = Join-Path $repoRoot "docs\release-notes"
$unreleasedPath = Join-Path $releaseNotesDir "unreleased.md"
$wailsPath = Join-Path $repoRoot "wails.json"
$appInfoPath = Join-Path $repoRoot "frontend\dist\js\app-info.js"
$staticHTMLPaths = @(
    (Join-Path $repoRoot "frontend\dist\index.html"),
    (Join-Path $repoRoot "frontend\dist\guide.html"),
    (Join-Path $repoRoot "frontend\dist\tools.html"),
    (Join-Path $repoRoot "frontend\dist\settings.html")
)
$changelogPath = Join-Path $repoRoot "CHANGELOG.md"
$releaseDate = (Get-Date).ToString("yyyy-MM-dd")

if ($ExistingTag -and ($Commit -or $Tag -or $Push)) {
    throw "-ExistingTag cannot be combined with -Commit, -Tag, or -Push."
}

$shouldCommit = $Commit -or $Tag -or $Push -or ($Publish -and -not $ExistingTag)
$shouldTag = ($Tag -or $Publish) -and -not $ExistingTag
$shouldPush = ($Push -or $Publish) -and -not $ExistingTag
$shouldPackage = $Package -or $Publish

if (($shouldCommit -or $ExistingTag) -and -not $AllowDirty) {
    Assert-CleanGitTree -RepoRoot $repoRoot
}

$currentVersion = Get-WailsProductVersion -Path $wailsPath
$temporaryVersion = "0.0.0"
if (-not [string]::IsNullOrWhiteSpace($Version)) {
    $temporaryVersion = (Parse-SemVer -Text $Version).Text
} else {
    $temporaryVersion = $currentVersion
}
$temporaryVersionedPath = Join-Path $releaseNotesDir "v$temporaryVersion.md"
$noteSource = Get-ReleaseNoteSource -UnreleasedPath $unreleasedPath -VersionedPath $temporaryVersionedPath
$latestTag = Get-LatestReleaseTag -RepoRoot $repoRoot
$target = Get-TargetVersion -CurrentVersion $currentVersion -RequestedVersion $Version -RequestedBump $Bump -ReleaseNoteBody $noteSource.Body -LatestTag $latestTag
$targetVersion = $target.Version
$versionedNotesPath = Join-Path $releaseNotesDir "v$targetVersion.md"
$outputFilename = "idf-analyzer-v$targetVersion"

if ($ExistingTag) {
    Assert-ExistingGitTag -RepoRoot $repoRoot -TargetVersion $targetVersion
}

if ($noteSource.Source -eq "versioned" -and $temporaryVersionedPath -ne $versionedNotesPath) {
    $noteSource = Get-ReleaseNoteSource -UnreleasedPath $unreleasedPath -VersionedPath $versionedNotesPath
}

Set-WailsReleaseMetadata -Path $wailsPath -TargetVersion $targetVersion
Set-AppInfoModuleVersion -Path $appInfoPath -TargetVersion $targetVersion
Set-StaticHTMLAppVersion -Paths $staticHTMLPaths -ProductName (Get-WailsProductName -Path $wailsPath) -TargetVersion $targetVersion
Update-Changelog -Path $changelogPath -TargetVersion $targetVersion -ReleaseDate $releaseDate -ReleaseNoteBody $noteSource.Body
Write-VersionedReleaseNotes -Path $versionedNotesPath -TargetVersion $targetVersion -ReleaseDate $releaseDate -ReleaseNoteBody $noteSource.Body

if ($noteSource.Source -eq "unreleased" -and -not $KeepUnreleased) {
    Reset-UnreleasedReleaseNotes -Path $unreleasedPath
}

$assetPath = ""
if ($shouldPackage) {
    $assetPath = New-ReleasePackage -RepoRoot $repoRoot -TargetVersion $targetVersion -OutputFilename $outputFilename -OutputDir (Join-Path $repoRoot "build\release")
}

if ($shouldCommit) {
    $metadataPaths = @(
        $wailsPath,
        $appInfoPath
    ) + $staticHTMLPaths + @(
        $changelogPath,
        $unreleasedPath,
        $versionedNotesPath
    )
    Invoke-GitReleaseCommit -RepoRoot $repoRoot -Paths $metadataPaths -TargetVersion $targetVersion
}

if ($shouldTag) {
    Invoke-GitTag -RepoRoot $repoRoot -TargetVersion $targetVersion
}

if ($shouldPush) {
    Invoke-GitPush -RepoRoot $repoRoot -TargetVersion $targetVersion
}

if ($Publish) {
    Invoke-GitHubRelease -TargetVersion $targetVersion -ReleaseNotesPath $versionedNotesPath -AssetPath $assetPath -DraftRelease:$Draft -PrereleaseRelease:$Prerelease
}

Write-Host "[release] version: $targetVersion"
Write-Host "[release] bump: $($target.Bump)"
if ($ExistingTag) {
    Write-Host "[release] existing tag: v$targetVersion"
}
Write-Host "[release] notes: $versionedNotesPath"
Write-Host "[release] changelog: $changelogPath"
if ($assetPath) {
    Write-Host "[release] asset: $assetPath"
}
