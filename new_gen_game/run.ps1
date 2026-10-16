# Render one model headlessly:  .\new_gen_game\run.ps1 warehouse [resolution] [samples]
# Override the Blender location with $env:BLENDER if it moves.
param(
    [Parameter(Mandatory = $true)][string]$Model,
    [int]$Resolution = 1024,
    [int]$Samples = 192
)
$ErrorActionPreference = 'Stop'
$blender = $env:BLENDER
if (-not $blender) { $blender = 'M:\blender-5.2.3-candidate+v52.32ede9dba158-windows.amd64-release\blender.exe' }
$root = $PSScriptRoot
$script = Join-Path $root "blender\$Model.py"
$out = Join-Path $root "renders\$Model.png"
& $blender -b --factory-startup --python $script -- $out $Resolution $Samples
if ($LASTEXITCODE -ne 0) { throw "Blender failed ($LASTEXITCODE)" }
