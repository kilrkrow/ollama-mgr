# Capture Family / Tag / Popular screenshots into docs/screenshots/
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$OutDir = Join-Path $Root "docs\screenshots"
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$Edge = @(
  "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe",
  "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe"
) | Where-Object { Test-Path $_ } | Select-Object -First 1

if (-not $Edge) {
  Write-Error "Microsoft Edge not found (needed for headless screenshots)."
}

Write-Host "Building GUI (console subsystem for -http)..."
$env:CGO_ENABLED = "0"
# Do NOT use -H=windowsgui here — that hides console flags and can break -http mode.
go build -ldflags "-s -w" -o dist/ollama-mgr-gui-http.exe ./cmd/ollama-mgr-gui
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$Addr = "127.0.0.1:8765"
Get-Process ollama-mgr-gui-http -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 300

$Proc = Start-Process -FilePath (Join-Path $Root "dist\ollama-mgr-gui-http.exe") `
  -ArgumentList @("-http", "-addr", $Addr) `
  -PassThru -WindowStyle Hidden

# Wait until API answers
$ok = $false
for ($i = 0; $i -lt 40; $i++) {
  try {
    $r = Invoke-WebRequest -Uri "http://$Addr/api/status" -UseBasicParsing -TimeoutSec 2
    if ($r.StatusCode -eq 200) { $ok = $true; break }
  } catch { }
  Start-Sleep -Milliseconds 250
}
if (-not $ok) {
  if ($Proc -and -not $Proc.HasExited) { Stop-Process -Id $Proc.Id -Force -ErrorAction SilentlyContinue }
  Write-Error "Server at http://$Addr never became ready"
}
Write-Host "Server ready at http://$Addr/"

try {
  $views = @(
    @{ Name = "family";  Query = "view=family";  File = "family.png" },
    @{ Name = "tag";     Query = "view=tag";     File = "tag.png" },
    @{ Name = "popular"; Query = "view=popular"; File = "popular.png" }
  )
  foreach ($v in $views) {
    $url = "http://$Addr/?$($v.Query)"
    $out = Join-Path $OutDir $v.File
    if (Test-Path $out) { Remove-Item $out -Force }

    # Edge --screenshot path: use absolute path with forward slashes
    $outFwd = $out -replace '\\', '/'
    $args = @(
      "--headless=new",
      "--disable-gpu",
      "--no-first-run",
      "--hide-scrollbars",
      "--window-size=1280,900",
      "--virtual-time-budget=15000",
      "--screenshot=$outFwd",
      $url
    )
    Write-Host "Capturing $($v.Name) -> $out"
    # Edge prints "N bytes written..." to stderr; do not treat as failure
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    & $Edge @args 2>$null | Out-Null
    $ErrorActionPreference = $prev
    # Edge may flush the PNG shortly after process exit
    $found = $false
    for ($w = 0; $w -lt 20; $w++) {
      if ((Test-Path $out) -and ((Get-Item $out).Length -gt 10000)) {
        $found = $true
        break
      }
      Start-Sleep -Milliseconds 200
    }
    if (-not $found) {
      Write-Warning "Missing or tiny $out"
    } else {
      Write-Host "  ok $((Get-Item $out).Length) bytes"
    }
  }
} finally {
  if ($Proc -and -not $Proc.HasExited) {
    Stop-Process -Id $Proc.Id -Force -ErrorAction SilentlyContinue
  }
}

Write-Host "Done. Files in $OutDir"
Get-ChildItem $OutDir | Format-Table Name, Length, LastWriteTime
