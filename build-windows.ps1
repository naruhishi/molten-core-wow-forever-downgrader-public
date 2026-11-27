param([string]$Version = "v1.9.2", [ValidateSet("private","public")][string]$Channel = "public")
$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    if ($env:GITHUB_ACTIONS -eq 'true' -and $env:GITHUB_REF_TYPE -ne 'tag' -and -not $PSBoundParameters.ContainsKey('Version')) { $Version = 'v1.7.0' }
    if ($Version -notmatch '^v\d+\.\d+\.\d+$') { throw "Expected vX.Y.Z" }
    New-Item -ItemType Directory -Force .\dist | Out-Null
    go run -buildvcs=false ./scripts/winres -version $Version -name Molten-Core-WoW-Forever-Downgrader.exe -product "Molten Core - WoW Forever Downgrader" -icon assets/molten-core-crest.png -out cmd/molten-core/rsrc_windows_amd64.syso
    if ($LASTEXITCODE -ne 0) { throw 'Windows resource build failed.' }
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    go build -buildvcs=false -trimpath -ldflags "-H=windowsgui -X main.appVersion=$Version -X main.updateChannel=$Channel" -o .\dist\Molten-Core-WoW-Forever-Downgrader.exe ./cmd/molten-core
    if ($LASTEXITCODE -ne 0) { throw 'Windows installer build failed.' }
    Get-FileHash .\dist\Molten-Core-WoW-Forever-Downgrader.exe
} finally {
    Pop-Location
}
