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

    $parentFull = [System.IO.Path]::GetFullPath($Parent).TrimEnd('\', '/')
    $childFull = [System.IO.Path]::GetFullPath($Child)
    if (-not $childFull.StartsWith($parentFull + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
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
    if (-not $pathFull.StartsWith($baseFull + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
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
    Assert-SafeChildPath $RepoRoot $FrontendDir
    # main.go embeds frontend/dist, which does not exist in a clean checkout.
    # Build it before Go tests and reuse it for the Wails packaging step.
    Invoke-Checked "npm ci" "npm.cmd" @("ci") $FrontendDir
    Invoke-Checked "frontend tests" "npm.cmd" @("test") $FrontendDir
    Invoke-Checked "frontend lint" "npm.cmd" @("run", "lint") $FrontendDir
    Invoke-Checked "frontend formatting" "npm.cmd" @("run", "format:check") $FrontendDir
    Invoke-Checked "TypeScript checks" "npm.cmd" @("exec", "--", "tsc", "--noEmit", "--noUnusedLocals", "--noUnusedParameters") $FrontendDir
    Invoke-Checked "frontend build" "npm.cmd" @("run", "build") $FrontendDir
    Invoke-Checked "go test" "go" @("test", "-count=1", "./...")
    Invoke-Checked "go vet" "go" @("vet", "./...")
    Invoke-Checked "npm audit" "npm.cmd" @("audit", "--audit-level=high") $FrontendDir
    if ($BuildInstaller) {
        Invoke-Checked "wails build --nsis -s -trimpath" "wails" @("build", "--nsis", "-s", "-trimpath")
    } else {
        # -trimpath: keep the build machine's absolute paths (user name, project folder)
        # out of the shipped binary; they otherwise appear in panic traces and strings.
        Invoke-Checked "wails build -s -trimpath" "wails" @("build", "-s", "-trimpath")
    }
}

$exePath = Join-Path $BuildBinDir "clipbox.exe"
if (-not (Test-Path -LiteralPath $exePath)) {
    throw "Build output not found: $exePath"
}
if (-not $SkipBuild) {
    $previousTestExe = [Environment]::GetEnvironmentVariable("CLIPBOX_APP_EXE")
    $env:CLIPBOX_APP_EXE = $exePath
    try { Invoke-Checked "resident lifecycle" "go" @("test", "-count=1", "-run", "TestProductionResidentLifecycle", ".") }
    finally { [Environment]::SetEnvironmentVariable("CLIPBOX_APP_EXE", $previousTestExe) }
}

$releaseName = "ClipBox-$version-windows-amd64"
$releaseDir = Join-Path $DistRoot $releaseName
$zipPath = Join-Path $DistRoot "$releaseName.zip"
$checksumsPath = Join-Path $DistRoot "checksums.sha256"
$packageChecksumPath = Join-Path $DistRoot "$releaseName.sha256"
$manifestPath = Join-Path $releaseDir "release-manifest.json"
$updateManifestPath = Join-Path $DistRoot "update-manifest.json"

New-Item -ItemType Directory -Force -Path $DistRoot | Out-Null
Assert-SafeChildPath $DistRoot $releaseDir
Assert-SafeChildPath $DistRoot $zipPath
Assert-SafeChildPath $DistRoot $checksumsPath
Assert-SafeChildPath $DistRoot $packageChecksumPath
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
foreach ($documentName in @("ARCHITECTURE.md", "QUALITY_0.4.0.md")) {
    $documentSource = Join-Path $RepoRoot $documentName
    if (Test-Path -LiteralPath $documentSource) { Copy-Item -LiteralPath $documentSource -Destination (Join-Path $releaseDir $documentName) }
}
$licenseSource = Join-Path $RepoRoot "LICENSE"
if (Test-Path -LiteralPath $licenseSource) {
    Copy-Item -LiteralPath $licenseSource -Destination (Join-Path $releaseDir "LICENSE")
}

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
foreach ($documentName in @("ARCHITECTURE.md", "QUALITY_0.4.0.md")) {
    $documentCopy = Join-Path $releaseDir $documentName
    if (Test-Path -LiteralPath $documentCopy) { $files += New-FileEntry $documentCopy $releaseDir }
}
$licenseCopy = Join-Path $releaseDir "LICENSE"
if (Test-Path -LiteralPath $licenseCopy) {
    $files += New-FileEntry $licenseCopy $releaseDir
}
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
# Write manifests as UTF-8 without BOM: update.go parses them with json.Unmarshal,
# and Windows PowerShell's "-Encoding UTF8" prepends a BOM that breaks parsing.
$manifestJson = $manifest | ConvertTo-Json -Depth 6
[System.IO.File]::WriteAllText($manifestPath, $manifestJson, (New-Object System.Text.UTF8Encoding($false)))

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
$updateManifestJson = $updateManifest | ConvertTo-Json -Depth 6
[System.IO.File]::WriteAllText($updateManifestPath, $updateManifestJson, (New-Object System.Text.UTF8Encoding($false)))

$checksumEntries = @()
foreach ($artifact in @($zipPath, (Join-Path $releaseDir "ClipBox.exe"), $installerCopy, $updateManifestPath)) {
    if (Test-Path -LiteralPath $artifact) {
        $hash = Get-FileHash -Algorithm SHA256 -LiteralPath $artifact
        $relative = Get-RelativePath $DistRoot (Get-Item -LiteralPath $artifact).FullName
        $checksumEntries += "$($hash.Hash.ToLowerInvariant())  $relative"
    }
}
$checksumEntries | Set-Content -Encoding ASCII -Path $checksumsPath
"$($zipHash.Hash.ToLowerInvariant())  $($zipItem.Name)" | Set-Content -Encoding ASCII -LiteralPath $packageChecksumPath

Write-Host ""
Write-Host "Release package created:"
Write-Host "  $zipPath"
Write-Host "  $manifestPath"
Write-Host "  $updateManifestPath"
Write-Host "  $checksumsPath"
Write-Host "  $packageChecksumPath"
