param([switch]$Migrate)
$ErrorActionPreference = 'Stop'
$backend = Join-Path $PSScriptRoot '../backend'
$envFile = Join-Path $backend '.env'
if (!(Test-Path $envFile)) { throw 'Copy backend/.env.example to backend/.env first.' }
foreach ($line in Get-Content -LiteralPath $envFile) {
    if ($line -match '^([A-Z][A-Z0-9_]*)=(.*)$') { [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2].Trim('"').Trim("'"), 'Process') }
}
Push-Location $backend
try {
    if ($Migrate) { go run ./cmd/api -migrate } else { go run ./cmd/api }
    if ($LASTEXITCODE -ne 0) { throw 'Backend command failed' }
} finally { Pop-Location }
