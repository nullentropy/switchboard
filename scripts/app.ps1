$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
New-Item -ItemType Directory -Force -Path dist | Out-Null
go run github.com/nullentropy/caution/cmd/winapp -pkg . -name Switchboard -icon icon.png -o dist -cc gcc
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Output dist\Switchboard.exe
