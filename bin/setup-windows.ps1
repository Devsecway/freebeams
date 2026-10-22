<#
.SYNOPSIS
Installs everything needed to build the FreeBeams Windows installer, then builds it.

.DESCRIPTION
One command on a fresh Windows machine. Installs with winget whatever is
missing (Go, Git, MSYS2, Inno Setup), installs the MinGW compiler and zip
inside MSYS2, and runs bin/build-windows in an MSYS2 UCRT64 shell, which
produces releases\FreeBeams-<version>-Setup.exe and the portable zip.

The Setup.exe this builds is self-contained: people installing FreeBeams need
none of these tools.

Run from a PowerShell window opened as administrator (MSYS2 installs to
C:\msys64), from the repository folder:

    powershell -ExecutionPolicy Bypass -File bin\setup-windows.ps1

Rerunning is safe: installed tools are skipped.

.PARAMETER NoBuild
Install the tools only; do not run bin/build-windows.

.PARAMETER Repo
The repository root. Default: the parent of this script's folder.
#>
[CmdletBinding()]
param(
    [switch]$NoBuild,
    [string]$Repo = (Split-Path -Parent $PSScriptRoot)
)

$ErrorActionPreference = 'Stop'

function Step($message) {
    Write-Host ""
    Write-Host "==> $message" -ForegroundColor Cyan
}

# Installers extend the machine or user PATH; pick those changes up without a new window.
function Update-SessionPath {
    $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' +
                [Environment]::GetEnvironmentVariable('Path', 'User')
}

function Install-Tool($id, [scriptblock]$present) {
    if (& $present) {
        Write-Host "    $id already installed"
        return
    }
    Write-Host "    installing $id"
    winget install --id $id --exact --silent --accept-source-agreements --accept-package-agreements
    if ($LASTEXITCODE -ne 0) {
        throw "winget install $id failed (exit code $LASTEXITCODE)"
    }
    Update-SessionPath
}

Step "winget"
if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
    throw "winget is not available. Install 'App Installer' from the Microsoft Store (Windows 10 1809 or later), then rerun."
}

$msys = 'C:\msys64'
$bash = Join-Path $msys 'usr\bin\bash.exe'
$isccPaths = @(
    "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
    "$env:ProgramFiles\Inno Setup 6\ISCC.exe",
    "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe"
)

Step "build tools"
Install-Tool 'GoLang.Go'            { Get-Command go -ErrorAction SilentlyContinue }
Install-Tool 'Git.Git'              { Get-Command git -ErrorAction SilentlyContinue }
Install-Tool 'MSYS2.MSYS2'          { Test-Path $bash }
Install-Tool 'JRSoftware.InnoSetup' { @($isccPaths | Where-Object { Test-Path $_ }).Count -gt 0 }

if (-not (Test-Path $bash)) {
    throw "MSYS2 was not found at $msys after the install. Install it by hand (https://www.msys2.org) and rerun."
}

Step "MSYS2 packages (gcc, zip, and make, nasm, zlib for the bundled ffmpeg)"
$env:MSYSTEM = 'UCRT64'           # the shell bin/build-windows expects
$env:MSYS2_PATH_TYPE = 'inherit'  # Go and ISCC from the Windows PATH are visible inside
$env:CHERE_INVOKING = '1'         # the login shell starts in the current directory, not $HOME
# A fresh MSYS2 upgrades its core first and closes the shell to finish; the
# second -Syu completes the rest, and --noconfirm answers every prompt.
& $bash -lc 'pacman -Syu --noconfirm --needed' | Out-Host
& $bash -lc 'pacman -Syu --noconfirm --needed' | Out-Host
& $bash -lc 'pacman -S --noconfirm --needed mingw-w64-ucrt-x86_64-gcc zip make diffutils nasm mingw-w64-ucrt-x86_64-pkgconf mingw-w64-ucrt-x86_64-zlib' | Out-Host
if ($LASTEXITCODE -ne 0) {
    throw "pacman failed (exit code $LASTEXITCODE)"
}

& $bash -lc 'go version && gcc --version | head -n 1' | Out-Host
if ($LASTEXITCODE -ne 0) {
    throw "go or gcc is not visible inside the MSYS2 shell; open a new PowerShell window and rerun"
}

if ($NoBuild) {
    Step "done (tools only). Build with: bin/build-windows in the MSYS2 UCRT64 shell, or rerun this script"
    return
}

Step "building the bundle, the zip and the installer (bin/build-windows)"
Set-Location $Repo
& $bash -lc 'bin/build-windows' | Out-Host
if ($LASTEXITCODE -ne 0) {
    throw "bin/build-windows failed (exit code $LASTEXITCODE)"
}

Step "done"
Get-ChildItem (Join-Path $Repo 'releases') -Filter 'FreeBeams-*' | ForEach-Object {
    Write-Host "    $($_.FullName)"
}
