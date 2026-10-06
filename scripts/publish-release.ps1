<#
.SYNOPSIS
  Build ollama-mgr Windows binaries, zip them, and print SHA256 (KilrKrow release pattern).

.PARAMETER Version
  Semver without a leading v. Defaults to git describe or 0.1.0.

.EXAMPLE
  .\scripts\publish-release.ps1
  .\scripts\publish-release.ps1 -Version 0.2.0
#>
[CmdletBinding()]
param(
  [string]$Version = ''
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

if (-not $Version) {
  try {
    $git = (git describe --tags --always --dirty 2>$null)
    if ($git -match '^v?(\d+\.\d+\.\d+)') { $Version = $Matches[1] }
    elseif ($git) { $Version = $git.Trim() -replace '^v','' }
  } catch { }
  if (-not $Version) { $Version = '0.1.0' }
}

& (Join-Path $Root 'scripts\build.ps1')
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$artifactRoot = Join-Path $Root 'artifacts'
New-Item -ItemType Directory -Force -Path $artifactRoot | Out-Null

$stage = Join-Path $artifactRoot "ollama-mgr-win-x64-v$Version"
if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
New-Item -ItemType Directory -Force -Path $stage | Out-Null

Copy-Item (Join-Path $Root 'dist\ollama-mgr.exe') $stage
Copy-Item (Join-Path $Root 'dist\ollama-mgr-gui.exe') $stage
Copy-Item (Join-Path $Root 'LICENSE') $stage -ErrorAction SilentlyContinue
Copy-Item (Join-Path $Root 'README.md') $stage -ErrorAction SilentlyContinue

$zipName = "ollama-mgr-win-x64-v$Version.zip"
$zipPath = Join-Path $artifactRoot $zipName
if (Test-Path $zipPath) { Remove-Item $zipPath -Force }

Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zipPath -CompressionLevel Optimal

$hash = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash.ToUpperInvariant()
$sizeMb = [Math]::Round((Get-Item $zipPath).Length / 1MB, 2)

Write-Host ""
Write-Host "ZIP     $zipPath"
Write-Host "SIZE    $sizeMb MB"
Write-Host "SHA256  $hash"
Write-Host ""
Write-Host "Create the GitHub Release:"
Write-Host "  gh release create v$Version `"$zipPath`" --title `"ollama-mgr v$Version`" --notes-file -"
Write-Host ""
Write-Host "Suggested notes:"
Write-Host "  Windows builds of ollama-mgr (CLI) and ollama-mgr-gui (WebView2 + tray)."
Write-Host "  SHA256: $hash"