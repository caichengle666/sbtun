$ErrorActionPreference = 'Stop'

$root = $PSScriptRoot
$temp = Join-Path ([System.IO.Path]::GetTempPath()) ('sbtun-build-' + [guid]::NewGuid().ToString('N'))
$headers = @{ Accept = 'application/vnd.github+json'; 'User-Agent' = 'sbtun-build' }
New-Item -ItemType Directory -Force $temp | Out-Null

if (Test-Path (Join-Path $PSScriptRoot 'build\windows\icon.ico')) {
    Remove-Item (Join-Path $PSScriptRoot 'build\windows\icon.ico') -Force
}
$wails = Get-Command wails -ErrorAction SilentlyContinue
if (-not $wails -and (Test-Path (Join-Path $HOME 'go\bin\wails.exe'))) {
    $wails = Get-Item (Join-Path $HOME 'go\bin\wails.exe')
}
if (-not $wails) { throw '未找到 Wails CLI' }
$wailsPath = $wails.Source
if (-not $wailsPath) { $wailsPath = $wails.FullName }
Copy-Item (Join-Path $root 'resources\icon.png') (Join-Path $root 'build\appicon.png') -Force
& $wailsPath build -clean -platform windows/amd64

$out = Join-Path $root 'build\bin'
$dist = Join-Path $root 'dist\sbtun'
$singRelease = Invoke-RestMethod -Headers $headers -Uri 'https://api.github.com/repos/SagerNet/sing-box/releases/latest'
$singAsset = $singRelease.assets | Where-Object { $_.name -match 'windows-amd64\.zip$' } | Select-Object -First 1
if (-not $singAsset -or -not $singAsset.digest) { throw 'latest sing-box release 缺少 Windows amd64 资产或 SHA256 digest' }
$singZip = Join-Path $temp 'sing-box.zip'
Invoke-WebRequest -Uri $singAsset.browser_download_url -OutFile $singZip
$singHash = (Get-FileHash $singZip -Algorithm SHA256).Hash.ToLower()
$expectedSingHash = ($singAsset.digest -replace '^sha256:', '').ToLower()
if ($singHash -ne $expectedSingHash) { throw "sing-box SHA256 校验失败: $singHash" }
Expand-Archive -Path $singZip -DestinationPath (Join-Path $temp 'sing-box') -Force
$wintunPage = Invoke-WebRequest -Uri 'https://www.wintun.net/'
$wintunMatch = [regex]::Match($wintunPage.Content, 'builds/wintun-(?<version>[0-9.]+)\.zip')
if (-not $wintunMatch.Success) { throw 'Wintun 官网没有找到最新构建包' }
$wintunVersion = $wintunMatch.Groups['version'].Value
$wintunZip = Join-Path $temp 'wintun.zip'
Invoke-WebRequest -Uri "https://www.wintun.net/builds/wintun-$wintunVersion.zip" -OutFile $wintunZip
Expand-Archive -Path $wintunZip -DestinationPath (Join-Path $temp 'wintun') -Force
$rulesSource = Join-Path $temp 'rules'
New-Item -ItemType Directory -Force $rulesSource | Out-Null
$rules = @{
    'geosite-geolocation-cn.srs' = 'https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-geolocation-cn.srs'
    'geosite-geolocation-!cn.srs' = 'https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-geolocation-!cn.srs'
    'geoip-cn.srs' = 'https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-cn.srs'
}
foreach ($rule in $rules.GetEnumerator()) {
    Invoke-WebRequest -Uri $rule.Value -OutFile (Join-Path $rulesSource $rule.Key)
}
$singExe = Get-ChildItem (Join-Path $temp 'sing-box') -Filter sing-box.exe -Recurse | Select-Object -First 1
$singDir = Split-Path $singExe.FullName -Parent
New-Item -ItemType Directory -Force $dist, (Join-Path $dist 'rules'), (Join-Path $dist 'resources') | Out-Null
Copy-Item $singExe.FullName (Join-Path $dist 'sing-box.exe') -Force
Get-ChildItem $singDir -File | Where-Object { $_.Name -ne 'sing-box.exe' } | Copy-Item -Destination $dist -Force
Copy-Item (Join-Path $temp 'wintun\wintun\bin\amd64\wintun.dll') (Join-Path $dist 'wintun.dll') -Force
Copy-Item (Join-Path $rulesSource '*.srs') (Join-Path $dist 'rules') -Force
Copy-Item (Join-Path $root 'VERSION') (Join-Path $dist 'VERSION') -Force
Copy-Item (Join-Path $root 'resources\icon.png') (Join-Path $dist 'resources\icon.png') -Force
Copy-Item (Join-Path $root 'build\windows\icon.ico') (Join-Path $dist 'resources\sbtun.ico') -Force
@"
sbtun Windows 便携版

运行 sbtun.exe 即可启动桌面程序。
TUN 模式需要管理员权限。

内置组件：
- sing-box $($singRelease.tag_name)
- Wintun $wintunVersion
- sing-box SRS 分流规则集
"@ | Set-Content (Join-Path $dist 'README.txt') -Encoding UTF8
New-Item -ItemType Directory -Force (Join-Path $out 'rules') | Out-Null
New-Item -ItemType Directory -Force (Join-Path $out 'resources') | Out-Null

Copy-Item (Join-Path $dist 'sing-box.exe') $out -Force
Copy-Item (Join-Path $dist 'libcronet.dll') $out -Force -ErrorAction SilentlyContinue
Copy-Item (Join-Path $dist 'wintun.dll') $out -Force
Copy-Item (Join-Path $dist 'rules\*.srs') (Join-Path $out 'rules') -Force
Copy-Item (Join-Path $PSScriptRoot 'resources\icon.png') (Join-Path $out 'resources\icon.png') -Force
Copy-Item (Join-Path $PSScriptRoot 'build\windows\icon.ico') (Join-Path $out 'resources\sbtun.ico') -Force
New-Item -ItemType Directory -Force (Join-Path $out 'runtime-data') | Out-Null
if (Test-Path (Join-Path $dist 'runtime-data\config.json')) {
    Copy-Item (Join-Path $dist 'runtime-data\config.json') (Join-Path $out 'runtime-data\config.json') -Force
}

Write-Host "Portable build ready: $out\sbtun.exe"
Write-Host "sing-box: $($singRelease.tag_name); Wintun: $wintunVersion"
