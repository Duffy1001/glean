param(
    [ValidateSet("thin", "full")]
    [string]$Variant = "thin",
    [ValidateSet("fast", "high")]
    [string]$Model = "fast",
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\glean",
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$repo = "duffy1001/glean"
$asset = "glean-$Variant-$Model-windows-amd64.exe"
$dest = Join-Path $InstallDir "glean.exe"

if ((Test-Path $dest) -and -not $Force) {
    $current = & $dest --version 2>$null
    $answer = Read-Host "$current is already installed at $dest. Replace it? [y/N]"
    if ($answer -notin @("y", "Y")) {
        Write-Host "Aborted."
        exit 0
    }
}

$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
$binaryAsset = $release.assets | Where-Object name -eq $asset
$checksumAsset = $release.assets | Where-Object name -eq "checksums.txt"
if (-not $binaryAsset -or -not $checksumAsset) {
    throw "Release $($release.tag_name) does not contain $asset and checksums.txt"
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("glean-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    $binary = Join-Path $tmp $asset
    $checksums = Join-Path $tmp "checksums.txt"
    Write-Host "Downloading glean $($release.tag_name) $Variant-$Model (windows/amd64)..."
    Invoke-WebRequest $binaryAsset.browser_download_url -OutFile $binary
    Invoke-WebRequest $checksumAsset.browser_download_url -OutFile $checksums

    $line = Get-Content $checksums | Where-Object { $_ -match "\s$([regex]::Escape($asset))$" }
    if (-not $line) {
        throw "No checksum published for $asset"
    }
    $expected = ($line -split "\s+")[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 $binary).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
        throw "Checksum mismatch for $asset"
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Move-Item -Force $binary $dest
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -notcontains $InstallDir) {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
    Write-Host "Added $InstallDir to your user PATH. Open a new terminal to use it."
}
Write-Host "Installed $(& $dest --version) to $dest"
