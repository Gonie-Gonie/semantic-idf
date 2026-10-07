$ErrorActionPreference = "Stop"

. "$PSScriptRoot\toolchain.ps1"

$paths = Get-RepoToolchain
$hooksDir = Join-Path $paths.RepoRoot ".git\hooks"
if (-not (Test-Path -LiteralPath $hooksDir)) {
    throw "Git hooks directory not found: $hooksDir"
}

$hookPath = Join-Path $hooksDir "pre-commit"
$hook = @'
#!/bin/sh
if command -v pwsh >/dev/null 2>&1; then
  pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify.ps1 -Staged
else
  powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/verify.ps1 -Staged
fi
'@

[System.IO.File]::WriteAllText($hookPath, $hook.Replace("`r`n", "`n"), [System.Text.Encoding]::ASCII)
Write-Host "[ok] Installed pre-commit hook: scripts/verify.ps1 -Staged"
