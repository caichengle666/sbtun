$ErrorActionPreference = 'Stop'

wails build

$out = Join-Path $PSScriptRoot 'build\bin'
$dist = Join-Path $PSScriptRoot 'dist\sbtun'
New-Item -ItemType Directory -Force (Join-Path $out 'rules') | Out-Null

Copy-Item (Join-Path $dist 'sing-box.exe') $out -Force
Copy-Item (Join-Path $dist 'wintun.dll') $out -Force
Copy-Item (Join-Path $dist 'rules\*.srs') (Join-Path $out 'rules') -Force
New-Item -ItemType Directory -Force (Join-Path $out 'resources') | Out-Null
Copy-Item (Join-Path $PSScriptRoot 'resources\icon.png') (Join-Path $out 'resources\icon.png') -Force
New-Item -ItemType Directory -Force (Join-Path $out 'runtime-data') | Out-Null
if (Test-Path (Join-Path $dist 'runtime-data\config.json')) {
    Copy-Item (Join-Path $dist 'runtime-data\config.json') (Join-Path $out 'runtime-data\config.json') -Force
}

Write-Host "Portable build ready: $out\sbtun.exe"
