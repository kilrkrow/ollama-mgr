# Build Windows executables for ollama-mgr
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

New-Item -ItemType Directory -Force -Path dist | Out-Null

$Version = "0.1.0"
try {
    $git = (git describe --tags --always --dirty 2>$null)
    if ($git) { $Version = $git.Trim() }
} catch { }

$ld = "-s -w -X github.com/kilrkrow/ollama-mgr/internal/config.Version=$Version"
$Icon = Join-Path $Root "assets\appicon.ico"

# Embed appicon.ico into each main package via go-winres (.syso next to main)
function Ensure-WinIcon {
    param([string]$CmdDir)
    if (-not (Test-Path $Icon)) {
        Write-Host "No assets/appicon.ico — building without embedded icon." -ForegroundColor Yellow
        return
    }
    $winres = Get-Command go-winres -ErrorAction SilentlyContinue
    if (-not $winres) {
        Write-Host "Installing go-winres for icon embedding..."
        go install github.com/tc-hib/go-winres@latest
        if ($LASTEXITCODE -ne 0) {
            Write-Host "go-winres install failed — building without embedded icon." -ForegroundColor Yellow
            return
        }
        $gopath = (go env GOPATH).Trim()
        $env:PATH = "$gopath\bin;$env:PATH"
        $winres = Get-Command go-winres -ErrorAction SilentlyContinue
        if (-not $winres) {
            Write-Host "go-winres not on PATH — building without embedded icon." -ForegroundColor Yellow
            return
        }
    }
    Push-Location $CmdDir
    try {
        # Drops rsrc_windows_*.syso in this package dir for go build to pick up
        & go-winres simply --icon $Icon
        if ($LASTEXITCODE -ne 0) {
            Write-Host "go-winres failed in $CmdDir — continuing without icon." -ForegroundColor Yellow
        }
    } finally {
        Pop-Location
    }
}

Ensure-WinIcon (Join-Path $Root "cmd\ollama-mgr")
Ensure-WinIcon (Join-Path $Root "cmd\ollama-mgr-gui")

Write-Host "Building ollama-mgr.exe (CLI) version $Version..."
go build -ldflags $ld -o dist/ollama-mgr.exe ./cmd/ollama-mgr
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "Building ollama-mgr-gui.exe (GUI, CGO-free WebView2)..."
$env:CGO_ENABLED = "0"
go build -ldflags "$ld -H=windowsgui" -o dist/ollama-mgr-gui.exe ./cmd/ollama-mgr-gui
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "Done:"
Get-ChildItem dist/*.exe | Format-Table Name, Length, LastWriteTime
