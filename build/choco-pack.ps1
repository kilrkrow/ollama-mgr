<#
.SYNOPSIS
  Build the Chocolatey package for an ollama-mgr release zip.

.DESCRIPTION
  Copies choco\ into a staging folder, fills in version, download URL,
  SHA256 and release notes, copies LICENSE to tools\LICENSE.txt, and runs
  "choco pack". Nothing is pushed here; pushing is a separate, gated step.

  -BaseUrl changes where the package downloads the zip from. CI uses a
  file:/// URL to test-install a package from the local zip (draft release
  assets are not publicly downloadable). That test package must never be
  pushed.

.PARAMETER Version   SemVer without "v" (0.1.0, 0.1.0-rc.1).
.PARAMETER ZipPath   Release zip made by build\release.ps1.
.PARAMETER BaseUrl   Folder URL that holds the zip. Default: the GitHub release download URL for v<Version>.
.PARAMETER OutDir    Where the .nupkg goes. Default: release\choco.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Version,
    [Parameter(Mandatory)][string]$ZipPath,
    [string]$BaseUrl,
    [string]$OutDir = 'release\choco',
    [string]$Repo = 'kilrkrow/ollama-mgr'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 3

$PackageId = 'ollama-mgr'
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

$Version = $Version.TrimStart('v')
if ($Version -notmatch '^(\d+\.\d+\.\d+)(?:-([0-9A-Za-z.-]+))?$') { throw "Version '$Version' is not SemVer." }
# Chocolatey/NuGet v2 prerelease labels must be alphanumeric: 0.1.0-rc.1 -> 0.1.0-rc1
$ChocoVersion = $Matches[1]
if ($Matches[2]) { $ChocoVersion = "$ChocoVersion-" + ($Matches[2] -replace '[^0-9A-Za-z]', '') }
$Tag = "v$Version"

$ZipPath = (Resolve-Path -LiteralPath $ZipPath).Path
$ZipName = Split-Path -Leaf $ZipPath
$Sha256  = (Get-FileHash -Algorithm SHA256 -LiteralPath $ZipPath).Hash.ToLowerInvariant()
if (-not $BaseUrl) { $BaseUrl = "https://github.com/$Repo/releases/download/$Tag" }
$Url = $BaseUrl.TrimEnd('/') + '/' + $ZipName
$Notes = "https://github.com/$Repo/releases/tag/$Tag"

if (-not [IO.Path]::IsPathRooted($OutDir)) { $OutDir = Join-Path $Root $OutDir }
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$Stage = Join-Path $OutDir 'pkgsrc'
if (Test-Path $Stage) { Remove-Item -Recurse -Force $Stage }
Copy-Item -Recurse (Join-Path $Root 'choco') $Stage
Copy-Item (Join-Path $Root 'LICENSE') (Join-Path $Stage 'tools\LICENSE.txt')

$tokens = @{
    '__VERSION__'       = $ChocoVersion
    '__SEMVER__'        = $Version
    '__TAG__'           = $Tag
    '__URL64__'         = $Url
    '__SHA256__'        = $Sha256
    '__RELEASE_NOTES__' = $Notes
}
foreach ($f in Get-ChildItem -LiteralPath $Stage -Recurse -File -Include *.nuspec, *.ps1, *.txt) {
    $text = [IO.File]::ReadAllText($f.FullName)
    foreach ($k in $tokens.Keys) { $text = $text.Replace($k, $tokens[$k]) }
    if ($text -match '__[A-Z0-9_]+__') { throw "Unfilled token $($Matches[0]) in $($f.Name)" }
    if ($text -match '[^\x00-\x7F]') { throw "Non-ASCII text in $($f.Name)" }
    [IO.File]::WriteAllText($f.FullName, $text, (New-Object Text.UTF8Encoding($false)))
}

# choco pack ignores --out-dir on some hosts; pack in place, then move.
Push-Location $Stage
try {
    & choco pack "$PackageId.nuspec" --limit-output | ForEach-Object { Write-Host $_ }   # keep stdout = nupkg path only
    if ($LASTEXITCODE -ne 0) { throw "choco pack failed ($LASTEXITCODE)" }
} finally { Pop-Location }
$nupkgName = "$PackageId.$ChocoVersion.nupkg"
$nupkg = Join-Path $OutDir $nupkgName
if (Test-Path $nupkg) { Remove-Item -Force $nupkg }
Move-Item (Join-Path $Stage $nupkgName) $nupkg
Remove-Item -Recurse -Force $Stage

Write-Host "Package: $nupkg"
Write-Host "Version: $ChocoVersion  Url: $Url  SHA256: $Sha256"
Write-Output $nupkg
