param(
    [switch]$SkipBuild,
    [switch]$BuildInstaller,
    [switch]$IncludeInstaller,
    [string]$DownloadUrl = "",
    [string]$ReleaseNotesUrl = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir "..")
$FrontendDir = Join-Path $RepoRoot "frontend"
$BuildBinDir = Join-Path $RepoRoot "build\bin"
$DistRoot = Join-Path $RepoRoot "dist\release"

function Invoke-Checked {
    param(
        [string]$Label,
        [string]$FilePath,
        [string[]]$Arguments,
        [string]$WorkingDirectory = $RepoRoot
    )

    Write-Host "==> $Label"
    Push-Location $WorkingDirectory
    try {
        & $FilePath @Arguments
        $exitCode = $LASTEXITCODE
    } finally {
        Pop-Location
    }
    if ($exitCode -ne 0) {
        throw "$Label failed with exit code $exitCode"
    }
}

function Get-MetadataVersion {
    $metadataPath = Join-Path $RepoRoot "metadata.go"
    $metadata = Get-Content -Raw -Encoding UTF8 -Path $metadataPath
    $match = [regex]::Match($metadata, 'appVersion\s*=\s*"([^"]+)"')
    if (-not $match.Success) {
        throw "Could not find appVersion in metadata.go"
    }
    return $match.Groups[1].Value
}

function Get-ProjectInfo {
    $wailsPath = Join-Path $RepoRoot "wails.json"
    return Get-Content -Raw -Encoding UTF8 -Path $wailsPath | ConvertFrom-Json
}

function Assert-SafeChildPath {
    param(
        [string]$Parent,
        [string]$Child
    )

    $parentFull = [System.IO.Path]::GetFullPath($Parent)
    $childFull = [System.IO.Path]::GetFullPath($Child)
    if (-not $childFull.StartsWith($parentFull, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to modify path outside $parentFull`: $childFull"
    }
}

function Get-RelativePath {
    param(
        [string]$BasePath,
        [string]$Path
    )

    $baseFull = [System.IO.Path]::GetFullPath($BasePath).TrimEnd('\', '/')
    $pathFull = [System.IO.Path]::GetFullPath($Path)
    if (-not $pathFull.StartsWith($baseFull, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Path is not under base path: $pathFull"
    }
    return $pathFull.Substring($baseFull.Length).TrimStart('\', '/') -replace '\\', '/'
}

function New-FileEntry {
    param(
        [string]$Path,
        [string]$BasePath
    )

    $item = Get-Item -LiteralPath $Path
    $hash = Get-FileHash -Algorithm SHA256 -LiteralPath $Path
    $relative = Get-RelativePath $BasePath $item.FullName
    return [ordered]@{
        path = $relative
        bytes = $item.Length
        sha256 = $hash.Hash.ToLowerInvariant()
    }
}

$project = Get-ProjectInfo
$version = Get-MetadataVersion
$productVersion = $project.info.productVersion
if ($version -ne $productVersion) {
    throw "Version mismatch: metadata.go appVersion=$version, wails.json info.productVersion=$productVersion"
}
if ($BuildInstaller) {
    $IncludeInstaller = $true
}

if (-not $SkipBuild) {
    Invoke-Checked "go test" "go" @("test", "./...")
    Invoke-Checked "go vet" "go" @("vet", "./...")
    Invoke-Checked "npm audit" "npm.cmd" @("audit", "--audit-level=high") $FrontendDir
    if ($BuildInstaller) {
        Invoke-Checked "wails build --nsis" "wails" @("build", "--nsis")
    } else {
        Invoke-Checked "wails build" "wails" @("build")
    }
}

$exePath = Join-Path $BuildBinDir "clipbox.exe"
if (-not (Test-Path -LiteralPath $exePath)) {
    throw "Build output not found: $exePath"
}

$releaseName = "ClipBox-$version-windows-amd64"
$releaseDir = Join-Path $DistRoot $releaseName
$zipPath = Join-Path $DistRoot "$releaseName.zip"
$checksumsPath = Join-Path $DistRoot "checksums.sha256"
$manifestPath = Join-Path $releaseDir "release-manifest.json"
$updateManifestPath = Join-Path $DistRoot "update-manifest.json"

New-Item -ItemType Directory -Force -Path $DistRoot | Out-Null
Assert-SafeChildPath $DistRoot $releaseDir
Assert-SafeChildPath $DistRoot $zipPath
Assert-SafeChildPath $DistRoot $updateManifestPath
if (Test-Path -LiteralPath $releaseDir) {
    Remove-Item -LiteralPath $releaseDir -Recurse -Force
}
if (Test-Path -LiteralPath $zipPath) {
    Remove-Item -LiteralPath $zipPath -Force
}
if (Test-Path -LiteralPath $updateManifestPath) {
    Remove-Item -LiteralPath $updateManifestPath -Force
}
New-Item -ItemType Directory -Force -Path $releaseDir | Out-Null

Copy-Item -LiteralPath $exePath -Destination (Join-Path $releaseDir "ClipBox.exe")
Copy-Item -LiteralPath (Join-Path $RepoRoot "README.md") -Destination (Join-Path $releaseDir "README.md")

$installerPath = Join-Path $BuildBinDir "clipbox-amd64-installer.exe"
if ($IncludeInstaller) {
    if (Test-Path -LiteralPath $installerPath) {
        Copy-Item -LiteralPath $installerPath -Destination (Join-Path $releaseDir "ClipBox-$version-windows-amd64-installer.exe")
    } else {
        Write-Warning "Installer not found at $installerPath. Run 'wails build --nsis' on a machine with NSIS to include it."
    }
}

$files = @()
$files += New-FileEntry (Join-Path $releaseDir "ClipBox.exe") $releaseDir
$files += New-FileEntry (Join-Path $releaseDir "README.md") $releaseDir
$installerCopy = Join-Path $releaseDir "ClipBox-$version-windows-amd64-installer.exe"
if (Test-Path -LiteralPath $installerCopy) {
    $files += New-FileEntry $installerCopy $releaseDir
}

$manifest = [ordered]@{
    app = "ClipBox"
    version = $version
    platform = "windows"
    arch = "amd64"
    package = "portable"
    builtAtUtc = (Get-Date).ToUniversalTime().ToString("o")
    source = [ordered]@{
        wailsProject = "clipbox"
        wailsProductName = $project.info.productName
        wailsCompanyName = $project.info.companyName
    }
    files = $files
}
if (-not [string]::IsNullOrWhiteSpace($DownloadUrl)) {
    $manifest["downloadUrl"] = $DownloadUrl.Trim()
}
if (-not [string]::IsNullOrWhiteSpace($ReleaseNotesUrl)) {
    $manifest["releaseNotesUrl"] = $ReleaseNotesUrl.Trim()
}
$manifest | ConvertTo-Json -Depth 6 | Set-Content -Encoding UTF8 -Path $manifestPath

$packageFiles = @(Get-ChildItem -LiteralPath $releaseDir -File | Select-Object -ExpandProperty FullName)
if ($packageFiles.Count -eq 0) {
    throw "No files found to package in $releaseDir"
}
Compress-Archive -LiteralPath $packageFiles -DestinationPath $zipPath -Force

$zipItem = Get-Item -LiteralPath $zipPath
$zipHash = Get-FileHash -Algorithm SHA256 -LiteralPath $zipPath
$zipEntry = [ordered]@{
    path = Get-RelativePath $DistRoot $zipItem.FullName
    bytes = $zipItem.Length
    sha256 = $zipHash.Hash.ToLowerInvariant()
}
$updateManifest = [ordered]@{
    app = "ClipBox"
    version = $version
    platform = "windows"
    arch = "amd64"
    package = "portable"
    builtAtUtc = $manifest["builtAtUtc"]
    packageSha256 = $zipEntry.sha256
    packageBytes = $zipEntry.bytes
    files = @($zipEntry)
}
if (-not [string]::IsNullOrWhiteSpace($DownloadUrl)) {
    $updateManifest["downloadUrl"] = $DownloadUrl.Trim()
}
if (-not [string]::IsNullOrWhiteSpace($ReleaseNotesUrl)) {
    $updateManifest["releaseNotesUrl"] = $ReleaseNotesUrl.Trim()
}
$updateManifest | ConvertTo-Json -Depth 6 | Set-Content -Encoding UTF8 -Path $updateManifestPath

$checksumEntries = @()
foreach ($artifact in @($zipPath, (Join-Path $releaseDir "ClipBox.exe"), $installerCopy, $updateManifestPath)) {
    if (Test-Path -LiteralPath $artifact) {
        $hash = Get-FileHash -Algorithm SHA256 -LiteralPath $artifact
        $relative = Get-RelativePath $DistRoot (Get-Item -LiteralPath $artifact).FullName
        $checksumEntries += "$($hash.Hash.ToLowerInvariant())  $relative"
    }
}
$checksumEntries | Set-Content -Encoding ASCII -Path $checksumsPath

Write-Host ""
Write-Host "Release package created:"
Write-Host "  $zipPath"
Write-Host "  $manifestPath"
Write-Host "  $updateManifestPath"
Write-Host "  $checksumsPath"
