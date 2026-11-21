$ErrorActionPreference = 'Stop'
$Version = '0.1.2'
if ($env:OS -ne 'Windows_NT') { throw 'Use install.sh on macOS or Linux.' }
$Cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$Arch = switch ($Cpu.ToUpperInvariant()) { 'ARM64' { 'arm64' }; 'AMD64' { 'amd64' }; default { throw 'Unsupported CPU architecture.' } }
$Asset = "inkspect_${Version}_windows_${Arch}.zip"
$Base = "https://inkspect.si/cli/releases/v$Version"
$Temp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString())
$Destination = if ($env:INKSPECT_INSTALL_DIR) { $env:INKSPECT_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Inkspect\bin' }
try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    New-Item -ItemType Directory -Path $Temp | Out-Null
    Invoke-WebRequest -UseBasicParsing -Uri "$Base/$Asset" -OutFile (Join-Path $Temp $Asset)
    Invoke-WebRequest -UseBasicParsing -Uri "$Base/SHA256SUMS" -OutFile (Join-Path $Temp 'SHA256SUMS')
    $Pattern = '^([a-f0-9]{64})  ' + [regex]::Escape($Asset) + '$'
    $Lines = @(Get-Content (Join-Path $Temp 'SHA256SUMS') | Where-Object { $_ -match $Pattern })
    if ($Lines.Count -ne 1 -or $Lines[0] -notmatch $Pattern) { throw 'Missing or ambiguous checksum.' }
    $Expected = $Matches[1]
    $Actual = (Get-FileHash -Algorithm SHA256 (Join-Path $Temp $Asset)).Hash.ToLowerInvariant()
    if ($Actual -ne $Expected) { throw 'Checksum verification failed.' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $Zip = [IO.Compression.ZipFile]::OpenRead((Join-Path $Temp $Asset))
    try {
        $Entries = @($Zip.Entries | Where-Object { $_.FullName -ceq 'inkspect.exe' })
        if ($Entries.Count -ne 1) { throw 'Invalid release archive.' }
        [IO.Compression.ZipFileExtensions]::ExtractToFile($Entries[0], (Join-Path $Temp 'inkspect.exe'))
    } finally { $Zip.Dispose() }
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    $Target = Join-Path $Destination 'inkspect.exe'
    if ((Test-Path $Target) -and ((Get-Item $Target).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing to replace a link.' }
    Copy-Item (Join-Path $Temp 'inkspect.exe') $Target -Force
    $UserPath = [string][Environment]::GetEnvironmentVariable('Path', 'User')
    if (($UserPath -split ';') -notcontains $Destination) {
        [Environment]::SetEnvironmentVariable('Path', (($UserPath.TrimEnd(';') + ';' + $Destination).TrimStart(';')), 'User')
    }
    $env:Path = "$Destination;$env:Path"
    & $Target --version
    Write-Host "Installed Inkspect $Version. Reopen your terminal to use inkspect everywhere."
} finally { if (Test-Path $Temp) { Remove-Item -LiteralPath $Temp -Recurse -Force } }
