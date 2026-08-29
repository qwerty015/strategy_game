<#
.SYNOPSIS
    Builds the game into a standalone Windows distribution folder.

.DESCRIPTION
    Compiles cmd/game into bin\strategy_game.exe and makes sure bin\saves\
    exists next to it. That's the whole distribution: assets (every PNG
    under internal/assets) are embedded into the binary at compile time via
    go:embed, so the exe needs nothing else to run -- bin\ can be zipped and
    handed to someone as-is. bin\saves\ isn't strictly required (the game
    creates it itself on the first save, see internal/save.Save's
    os.MkdirAll), but it's created ahead of time so the folder looks right
    to a player who goes looking for their save files before ever saving.

.PARAMETER Release
    Strip debug symbols and the DWARF table (-ldflags "-s -w") for a
    smaller exe. Off by default: this project is still under active
    development, and stripped binaries produce much less useful panic
    stack traces -- worth the extra few MB while bugs are still expected.

.PARAMETER Clean
    Delete bin\ before building, instead of building on top of whatever
    is already there.

.EXAMPLE
    .\tools\build.ps1
    Ordinary development build.

.EXAMPLE
    .\tools\build.ps1 -Release -Clean
    Fresh stripped build, for handing bin\ to someone else.
#>
[CmdletBinding()]
param(
    [switch]$Release,
    [switch]$Clean
)

$ErrorActionPreference = 'Stop'

# tools\build.ps1 -> repo root is this script's parent directory.
$repoRoot = Split-Path -Parent $PSScriptRoot
$binDir = Join-Path $repoRoot 'bin'
$exePath = Join-Path $binDir 'strategy_game.exe'
$savesDir = Join-Path $binDir 'saves'

function Write-Step($message) {
    Write-Host "==> $message" -ForegroundColor Cyan
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "go was not found on PATH. Install Go (https://go.dev/dl/) and reopen this shell." -ForegroundColor Red
    exit 1
}

Push-Location $repoRoot
try {
    if ($Clean -and (Test-Path $binDir)) {
        Write-Step "Removing existing bin\"
        Remove-Item -Recurse -Force $binDir
    }
    New-Item -ItemType Directory -Force -Path $binDir | Out-Null

    # A previous run of the exe (or Explorer previewing it) can hold the
    # file open on Windows; go build's own error for that is a generic
    # "Access is denied", easy to mistake for something else. Fail with a
    # clearer message before even trying.
    if (Test-Path $exePath) {
        try {
            $stream = [System.IO.File]::Open($exePath, 'Open', 'Write', 'None')
            $stream.Close()
        } catch {
            Write-Host "$exePath is in use -- close the running game first, then re-run this script." -ForegroundColor Red
            exit 1
        }
    }

    $ldflags = ''
    $mode = 'development'
    if ($Release) {
        $ldflags = '-s -w'
        $mode = 'release (stripped)'
    }

    Write-Step "Building $mode binary"
    $buildArgs = @('build')
    if ($ldflags) { $buildArgs += @('-ldflags', $ldflags) }
    $buildArgs += @('-o', $exePath, './cmd/game')

    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    & go @buildArgs
    $exitCode = $LASTEXITCODE
    $sw.Stop()

    if ($exitCode -ne 0) {
        Write-Host "Build failed (go exit code $exitCode)." -ForegroundColor Red
        exit $exitCode
    }

    Write-Step "Preparing bin\saves\"
    New-Item -ItemType Directory -Force -Path $savesDir | Out-Null

    $sizeMB = [Math]::Round((Get-Item $exePath).Length / 1MB, 1)
    Write-Host ""
    Write-Host "Build complete in $([Math]::Round($sw.Elapsed.TotalSeconds, 1))s." -ForegroundColor Green
    Write-Host "  $exePath ($sizeMB MB)"
    Write-Host "  $savesDir\"
    Write-Host ""
    Write-Host "bin\ is a complete, standalone copy -- zip it and hand it to anyone; nothing else is needed."
} finally {
    Pop-Location
}
