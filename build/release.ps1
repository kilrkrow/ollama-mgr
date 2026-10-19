<#
.SYNOPSIS
  Build the ollama-mgr release zip (same bytes locally and in CI).

.DESCRIPTION
  Builds ollama-mgr.exe (CLI) and ollama-mgr-gui.exe (WebView2 GUI) for
  windows/amd64 with a reproducible Go build (-trimpath, -buildvcs=false,
  CGO disabled), embeds the app icon and version info with a pinned
  go-winres, stages the exes with LICENSE and README.md, and writes:

    <OutDir>\ollama-mgr-win-x64-v<Version>.zip
    <OutDir>\ollama-mgr-win-x64-v<Version>.zip.sha256   ("<hex>  <zip name>")

  The zip is written by build/mkzip (Go) with byte-sorted entries,
  forward-slash names and a fixed timestamp, so the same commit + same Go
  toolchain (pinned by the go directive in go.mod) gives the same zip bytes
  on any machine, from Windows PowerShell 5.1 or PowerShell 7.

  <OutDir> defaults to release\ and is self-ignored (a .gitignore with "*"
  is written into it), so nothing built here can be committed.

.PARAMETER Version
  SemVer without the leading "v" (for example 0.1.0 or 0.1.0-rc.1).
  Defaults to the Version in internal/config/config.go.

.EXAMPLE
  .\build\release.ps1
  .\build\release.ps1 -Version 0.1.0
#>
[CmdletBinding()]
param(
    [string]$Version,
    [string]$OutDir = 'release'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 3

$AppName   = 'ollama-mgr'
$AssetBase = 'ollama-mgr-win-x64'
$Module    = 'github.com/kilrkrow/ollama-mgr'
$GoWinres  = 'github.com/tc-hib/go-winres@v0.3.3'   # pinned; bump deliberately
$Commands  = @(
    @{ Dir = 'cmd\ollama-mgr';     Exe = 'ollama-mgr.exe';     Gui = $false; Desc = 'ollama-mgr CLI' },
    @{ Dir = 'cmd\ollama-mgr-gui'; Exe = 'ollama-mgr-gui.exe'; Gui = $true;  Desc = 'ollama-mgr GUI' }
)
$Extras    = @('LICENSE', 'README.md')

$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

if (-not $Version) {
    $cfg = Get-Content -Raw (Join-Path $Root 'internal\config\config.go')
    $m = [regex]::Match($cfg, 'var\s+Version\s*=\s*"([^"]+)"')
    if (-not $m.Success) { throw 'Could not read Version from internal/config/config.go; pass -Version.' }
    $Version = $m.Groups[1].Value
}
$Version = $Version.TrimStart('v')
if ($Version -notmatch '^(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z][0-9A-Za-z.-]*)?$') {
    throw "Version '$Version' is not SemVer (x.y.z or x.y.z-pre)."
}
$NumericVersion = '{0}.{1}.{2}.0' -f $Matches[1], $Matches[2], $Matches[3]

if (-not [IO.Path]::IsPathRooted($OutDir)) { $OutDir = Join-Path $Root $OutDir }
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$selfIgnore = Join-Path $OutDir '.gitignore'
if (-not (Test-Path $selfIgnore)) { [IO.File]::WriteAllText($selfIgnore, "*`n") }

$ZipName = "$AssetBase-v$Version.zip"
$ZipPath = Join-Path $OutDir $ZipName
$Stage   = Join-Path $OutDir "stage\$AssetBase-v$Version"
if (Test-Path $Stage) { Remove-Item -Recurse -Force $Stage }
New-Item -ItemType Directory -Force -Path $Stage | Out-Null

$Icon = Join-Path $Root 'assets\appicon.ico'
if (-not (Test-Path $Icon)) { throw "Missing $Icon" }

# Reproducible, pure-Go Windows build regardless of host settings.
$saved = @{}
foreach ($k in 'CGO_ENABLED', 'GOOS', 'GOARCH', 'GOFLAGS') { $saved[$k] = [Environment]::GetEnvironmentVariable($k) }
$env:CGO_ENABLED = '0'; $env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:GOFLAGS = $null

try {
    Write-Host "Go: $(go version)"
    foreach ($c in $Commands) {
        $pkgDir = Join-Path $Root $c.Dir
        $syso = Join-Path $pkgDir 'rsrc_windows_amd64.syso'
        try {
            # Icon + version resource next to main; removed again in finally.
            Push-Location $pkgDir
            try {
                & go run $GoWinres simply --arch amd64 --out rsrc --icon $Icon `
                    --product-name $AppName --product-version $NumericVersion `
                    --file-version $NumericVersion --file-description $c.Desc `
                    --original-filename $c.Exe --copyright 'Copyright (c) 2026 kilrkrow'
                if ($LASTEXITCODE -ne 0) { throw "go-winres failed in $($c.Dir)" }
            } finally { Pop-Location }

            $ld = "-s -w -buildid= -X $Module/internal/config.Version=$Version"
            if ($c.Gui) { $ld = "$ld -H=windowsgui" }
            $out = Join-Path $Stage $c.Exe
            Write-Host "Building $($c.Exe) $Version"
            & go build -trimpath -buildvcs=false -ldflags $ld -o $out "./$($c.Dir -replace '\\', '/')"
            if ($LASTEXITCODE -ne 0) { throw "go build failed for $($c.Dir)" }
        } finally {
            Remove-Item -Force -ErrorAction SilentlyContinue $syso
        }
    }
} finally {
    foreach ($k in @($saved.Keys)) { [Environment]::SetEnvironmentVariable($k, $saved[$k]) }
}

foreach ($f in $Extras) { Copy-Item -Force (Join-Path $Root $f) (Join-Path $Stage $f) }

# Deterministic zip, written by the pinned Go toolchain (build/mkzip):
# byte-sorted entries, '/' separators, fixed timestamp, same deflate output
# on every machine.
if (Test-Path $ZipPath) { Remove-Item -Force $ZipPath }
& go run ./build/mkzip -dir $Stage -out $ZipPath
if ($LASTEXITCODE -ne 0) { throw "mkzip failed ($LASTEXITCODE)" }

$hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $ZipPath).Hash.ToLowerInvariant()
[IO.File]::WriteAllText("$ZipPath.sha256", "$hash  $ZipName`n", [Text.Encoding]::ASCII)

Write-Host "Zip:    $ZipPath"
Write-Host "SHA256: $hash"
Write-Output $ZipPath
