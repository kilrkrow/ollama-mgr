$ErrorActionPreference = 'Stop'

$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition

# Filled in by build/choco-pack.ps1 from the GitHub release asset.
$packageArgs = @{
    packageName    = $env:ChocolateyPackageName
    unzipLocation  = $toolsDir
    url64bit       = '__URL64__'
    checksum64     = '__SHA256__'
    checksumType64 = 'sha256'
}

Install-ChocolateyZipPackage @packageArgs

# Make the GUI shim a GUI shim (no console window, does not wait).
New-Item -ItemType File -Force -Path (Join-Path $toolsDir 'ollama-mgr-gui.exe.gui') | Out-Null
