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

Write-Host "Building ollama-mgr.exe (CLI) version $Version..."
go build -ldflags $ld -o dist/ollama-mgr.exe ./cmd/ollama-mgr
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "Building ollama-mgr-gui.exe (GUI, CGO-free WebView2)..."
$env:CGO_ENABLED = "0"
go build -ldflags "$ld -H=windowsgui" -o dist/ollama-mgr-gui.exe ./cmd/ollama-mgr-gui
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "Done:"
Get-ChildItem dist/*.exe | Format-Table Name, Length, LastWriteTime
