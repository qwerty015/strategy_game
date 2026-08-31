<#
.SYNOPSIS
    Собирает готовую папку Windows-дистрибутива игры.

.DESCRIPTION
    Скрипт всегда очищает bin\, компилирует strategy_game.exe и копирует
    рядом с ним внешние игровые ассеты. PNG-спрайты и звук намеренно не
    вшиваются в exe: для запуска и распространения нужна вся папка bin\.

.EXAMPLE
    .\tools\build.ps1
#>
$ErrorActionPreference = 'Stop'

# tools\build.ps1 -> repository root.
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$binDir = Join-Path $repoRoot 'bin'
$exePath = Join-Path $binDir 'strategy_game.exe'
$spriteSource = Join-Path $repoRoot 'internal\assets'
$audioSource = Join-Path $repoRoot 'assets\audio'
$assetsDir = Join-Path $binDir 'assets'
$spriteDestination = Join-Path $assetsDir 'sprites'

function Write-Step([string]$Message) {
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Copy-RequiredDirectory([string]$Source, [string]$Destination) {
    if (-not (Test-Path -LiteralPath $Source -PathType Container)) {
        throw "Required asset folder is missing: $Source"
    }
    Copy-Item -LiteralPath $Source -Destination $Destination -Recurse -Force
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'Go was not found on PATH. Install Go (https://go.dev/dl/) and reopen this shell.'
}

# The output is intentionally a fixed, verified child of the repository.
# Never let the cleanup resolve to a broad or user-supplied path.
$expectedBinDir = [System.IO.Path]::GetFullPath((Join-Path $repoRoot 'bin'))
if ($binDir -ne $expectedBinDir) {
    throw "Unsafe build directory: $binDir"
}

Push-Location $repoRoot
try {
    if (Test-Path -LiteralPath $binDir) {
        Write-Step 'Cleaning bin\\'
        Remove-Item -LiteralPath $binDir -Recurse -Force
    }
    New-Item -ItemType Directory -Path $binDir | Out-Null

    Write-Step 'Compiling strategy_game.exe'
    $timer = [System.Diagnostics.Stopwatch]::StartNew()
    & go build -o $exePath ./cmd/game
    $exitCode = $LASTEXITCODE
    $timer.Stop()
    if ($exitCode -ne 0) {
        throw "Build failed (go exit code $exitCode)."
    }

    Write-Step 'Copying external sprites'
    New-Item -ItemType Directory -Path $spriteDestination -Force | Out-Null
    foreach ($folder in 'tiles', 'units', 'generated') {
        Copy-RequiredDirectory (Join-Path $spriteSource $folder) $spriteDestination
    }

    Write-Step 'Copying external audio'
    New-Item -ItemType Directory -Path $assetsDir -Force | Out-Null
    Copy-RequiredDirectory $audioSource $assetsDir

    $sizeMB = [Math]::Round((Get-Item -LiteralPath $exePath).Length / 1MB, 1)
    Write-Host ''
    Write-Host "Build complete in $([Math]::Round($timer.Elapsed.TotalSeconds, 1))s." -ForegroundColor Green
    Write-Host "  $exePath ($sizeMB MB)"
    Write-Host "  $spriteDestination\\"
    Write-Host "  $(Join-Path $assetsDir 'audio')\\"
    Write-Host ''
    Write-Host 'Keep the complete bin\ folder together when distributing the game.'
} finally {
    Pop-Location
}
