#!/usr/bin/env pwsh
[CmdletBinding()]
param([ValidateSet('all','quality','test','security')][string]$Phase = 'all')

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$failed = $false
function Invoke-Check([string]$Name, [scriptblock]$Command) {
    $output = [System.Collections.Generic.List[object]]::new()
    try {
        $global:LASTEXITCODE = 0
        & $Command *>&1 | ForEach-Object { $output.Add($_) }
        if ($LASTEXITCODE -ne 0) { throw "exit code $LASTEXITCODE" }
        Write-Host "[PASS] $Name" -ForegroundColor Green
    } catch {
        $output | Select-Object -Last 40 | Out-Host
        Write-Host "[FAIL] ${Name}: $_" -ForegroundColor Red
        $script:failed = $true
    }
}

Set-Location $PSScriptRoot
if ($Phase -in @('all','quality')) {
    Invoke-Check 'gofmt' { if (gofmt -l .) { throw 'unformatted Go files' } }
    Invoke-Check 'go vet' { go vet ./... }
    Invoke-Check 'cyclomatic complexity (maximum 29)' { go run github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0 -over 29 . }
    Invoke-Check 'duplication report' { go run github.com/mibk/dupl@v1.1.0 -t 100 . }
}
if ($Phase -in @('all','test')) {
    Invoke-Check 'tests' { go test -count=1 ./... }
    if ((go env CGO_ENABLED) -eq '1') {
        Invoke-Check 'race tests' { go test -race -count=1 ./... }
    } else {
        Write-Host '[PASS] race tests (CGO unavailable; enforced by Linux CI)' -ForegroundColor Green
    }
}
if ($Phase -in @('all','security')) {
    Invoke-Check 'production dependency audit (govulncheck)' { go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./... }
    Invoke-Check 'Trivy vulnerabilities, secrets and misconfiguration' {
        trivy fs --scanners vuln,secret,misconfig --severity HIGH,CRITICAL --exit-code 1 --no-progress --skip-dirs .git .
    }
}
if ($failed) { exit 1 }
