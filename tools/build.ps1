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
$iconSource = Join-Path $repoRoot 'assets\app.png'
$iconResource = $null
$iconResourceCreated = $false

function Write-Step([string]$Message) {
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Copy-RequiredDirectory([string]$Source, [string]$Destination) {
    if (-not (Test-Path -LiteralPath $Source -PathType Container)) {
        throw "Required asset folder is missing: $Source"
    }
    Copy-Item -LiteralPath $Source -Destination $Destination -Recurse -Force
}

# A visual pack can be expanded incrementally. Optional families are copied as
# soon as their first PNG is added, but do not make the normal build fail while
# an empty future family (for example resource icons) has not been drawn yet.
function Copy-OptionalDirectory([string]$Source, [string]$Destination) {
    if (Test-Path -LiteralPath $Source -PathType Container) {
        Copy-Item -LiteralPath $Source -Destination $Destination -Recurse -Force
    }
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
	if (-not (Test-Path -LiteralPath $iconSource -PathType Leaf)) {
		throw "Application icon is missing: $iconSource"
	}
	$targetOS = (& go env GOOS).Trim()
	$targetArch = (& go env GOARCH).Trim()
	if ($targetOS -ne 'windows' -or $targetArch -notin @('amd64', '386', 'arm64')) {
		throw 'build.ps1 requires a Windows Go target (amd64, 386 or arm64).'
	}
	$iconResource = Join-Path $repoRoot "cmd\game\appicon_windows_$targetArch.syso"
	if (Test-Path -LiteralPath $iconResource) {
		throw "Icon resource already exists; check it before building: $iconResource"
	}
    if (Test-Path -LiteralPath $binDir) {
        Write-Step 'Cleaning bin\\'
        Remove-Item -LiteralPath $binDir -Recurse -Force
    }
    New-Item -ItemType Directory -Path $binDir | Out-Null

	Write-Step 'Preparing application icon'
	$iconResourceCreated = $true
	& go run ./tools/iconbuild $iconSource $iconResource (Join-Path $binDir 'app.ico') $targetArch
	if ($LASTEXITCODE -ne 0) { throw 'Icon generation failed.' }

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
    foreach ($folder in 'buildings', 'resources') {
        Copy-OptionalDirectory (Join-Path $spriteSource $folder) $spriteDestination
    }

    Write-Step 'Copying external audio'
    New-Item -ItemType Directory -Path $assetsDir -Force | Out-Null
	Copy-Item -LiteralPath $iconSource -Destination (Join-Path $assetsDir 'app.png')
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
	if ($iconResourceCreated -and (Test-Path -LiteralPath $iconResource)) {
		Remove-Item -LiteralPath $iconResource -Force
	}
    Pop-Location
}
