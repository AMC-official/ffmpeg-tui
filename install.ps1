<#
.SYNOPSIS
  Install FFmpeg-TUI from GitHub releases on Windows.

.EXAMPLE
  irm https://raw.githubusercontent.com/AMC-official/ffmpeg-tui/main/install.ps1 | iex

.EXAMPLE
  .\install.ps1 -Version v1.0.0 -InstallDir "$env:LOCALAPPDATA\ffmpeg-tui\bin"
#>
[CmdletBinding()]
param(
    [string]$Version = "",
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "ffmpeg-tui\bin"),
    [switch]$AddToPath = $true
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$Repo = "AMC-official/ffmpeg-tui"
$Project = "ffmpeg-tui"

function Write-Step([string]$Message) {
    Write-Host $Message
}

function Fail([string]$Message) {
    Write-Error $Message
    exit 1
}

$architecture = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    "x86" { "386" }
    default { Fail "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

if (-not $Version) {
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "ffmpeg-tui-installer" }
        $Version = $release.tag_name
    }
    catch {
        Fail "Could not determine the latest release. Pass -Version (for example -Version v1.0.0)."
    }
}
if (-not $Version) { Fail "No release version resolved." }
$tag = $Version
if (-not $tag.StartsWith("v")) { $tag = "v$tag" }
$cleanVersion = $tag.TrimStart("v")

$archive = "$($Project)_$($cleanVersion)_windows_$architecture.zip"
$baseUrl = "https://github.com/$Repo/releases/download/$tag"

$temp = Join-Path ([System.IO.Path]::GetTempPath()) ("ffmpeg-tui-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $temp | Out-Null
try {
    Write-Step "FFmpeg-TUI $cleanVersion for windows/$architecture"
    $archivePath = Join-Path $temp $archive
    Write-Step "downloading $baseUrl/$archive"
    Invoke-WebRequest -Uri "$baseUrl/$archive" -OutFile $archivePath -UseBasicParsing

    $checksumsPath = Join-Path $temp "checksums.txt"
    $haveChecksums = $false
    try {
        Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile $checksumsPath -UseBasicParsing
        $haveChecksums = $true
    }
    catch {
        Write-Step "warning: checksums.txt not available, skipping verification"
    }
    if ($haveChecksums) {
        # Verification must fail closed: a mismatch or a missing entry aborts the
        # install instead of falling through as a download warning.
        $line = Select-String -Path $checksumsPath -Pattern ([regex]::Escape($archive) + '$') | Select-Object -First 1
        if (-not $line) { Fail "checksums.txt does not list $archive" }
        $expected = ($line.Line -split '\s+')[0].ToLowerInvariant()
        $actual = (Get-FileHash -Algorithm SHA256 -Path $archivePath).Hash.ToLowerInvariant()
        if ($expected -ne $actual) { Fail "checksum mismatch for $archive" }
        Write-Step "checksum verified"
    }

    Expand-Archive -Path $archivePath -DestinationPath $temp -Force
    $exe = Join-Path $temp "$Project.exe"
    if (-not (Test-Path $exe)) { Fail "archive did not contain $Project.exe" }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $target = Join-Path $InstallDir "$Project.exe"
    Copy-Item -Force $exe $target
    Write-Step "installed $target"

    if ($AddToPath) {
        $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
        $entries = @()
        if ($userPath) { $entries = $userPath -split ';' | Where-Object { $_ } }
        if ($entries -notcontains $InstallDir) {
            $newPath = (@($entries) + $InstallDir) -join ';'
            [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
            Write-Step "added $InstallDir to your user PATH (restart your terminal)"
        }
    }

    try { & $target -version | Out-Host } catch { }
}
finally {
    Remove-Item -Recurse -Force $temp -ErrorAction SilentlyContinue
}

Write-Step ""
Write-Step "ffmpeg and ffprobe must be installed separately and available on PATH."
