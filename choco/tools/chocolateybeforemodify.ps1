$ErrorActionPreference = 'Stop'

# Stop copies of ollama-mgr that run from this package (upgrade/uninstall
# would otherwise fail on locked files). Ollama itself is never touched.
$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$prefix   = $toolsDir.TrimEnd('\') + '\'

Get-Process -Name 'ollama-mgr-gui', 'ollama-mgr' -ErrorAction SilentlyContinue |
    Where-Object { $_.Path -and $_.Path.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase) } |
    ForEach-Object {
        Write-Host "Stopping $($_.Name) (PID $($_.Id))"
        Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue
    }
