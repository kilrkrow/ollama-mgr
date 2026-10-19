$ErrorActionPreference = 'Stop'

# Remove what the app's own toggles created for this install only:
#  - HKCU Run value "KilrKrowOllamaMgr" (Start with Windows)
#  - %APPDATA%\Microsoft\Windows\Start Menu\Programs\ollama-mgr.lnk (Add to Start Menu)
# Each is removed only if it points into this package folder, so a copy of
# ollama-mgr installed some other way keeps its settings.
# User settings/caches (%APPDATA%\ollama-mgr, %LOCALAPPDATA%\ollama-mgr) are kept.
$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$prefix   = $toolsDir.TrimEnd('\') + '\'

function Test-InPackage([string]$path) {
    if (-not $path) { return $false }
    $p = $path.Trim().Trim('"')
    return $p.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)
}

$runKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
$runVal = 'KilrKrowOllamaMgr'
$cmd = (Get-ItemProperty -Path $runKey -Name $runVal -ErrorAction SilentlyContinue).$runVal
if ($cmd) {
    $exe = if ($cmd -match '^\s*"([^"]+)"') { $Matches[1] } else { ($cmd.Trim() -split '\s+')[0] }
    if (Test-InPackage $exe) {
        Remove-ItemProperty -Path $runKey -Name $runVal -ErrorAction SilentlyContinue
        Write-Host "Removed Start with Windows entry ($runVal)."
    }
}

$lnk = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\ollama-mgr.lnk'
if (Test-Path -LiteralPath $lnk) {
    $target = (New-Object -ComObject WScript.Shell).CreateShortcut($lnk).TargetPath
    if (Test-InPackage $target) {
        Remove-Item -LiteralPath $lnk -Force
        Write-Host 'Removed Start Menu shortcut (ollama-mgr.lnk).'
    }
}
