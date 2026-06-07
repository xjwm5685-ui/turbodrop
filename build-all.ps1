param(
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Dist = Join-Path $Root "dist"

if (Test-Path $Dist) {
    Remove-Item -Recurse -Force $Dist
}
New-Item -ItemType Directory -Path $Dist | Out-Null

$targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Suffix = ".exe" },
    @{ GOOS = "windows"; GOARCH = "arm64"; Suffix = ".exe" },
    @{ GOOS = "linux"; GOARCH = "amd64"; Suffix = "" },
    @{ GOOS = "linux"; GOARCH = "arm64"; Suffix = "" },
    @{ GOOS = "darwin"; GOARCH = "amd64"; Suffix = "" },
    @{ GOOS = "darwin"; GOARCH = "arm64"; Suffix = "" }
)

Push-Location $Root
try {
    foreach ($target in $targets) {
        $name = "turbodrop-$Version-$($target.GOOS)-$($target.GOARCH)$($target.Suffix)"
        $output = Join-Path $Dist $name

        Write-Host "==> Building $name"
        $env:GOOS = $target.GOOS
        $env:GOARCH = $target.GOARCH
        go build -o $output .
        if ($LASTEXITCODE -ne 0) {
            throw "构建失败: $name"
        }
    }
}
finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Pop-Location
}

Write-Host ""
Write-Host "Build finished. Output directory: $Dist"
