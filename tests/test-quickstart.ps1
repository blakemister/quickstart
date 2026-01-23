# Quickstart Tests
# Run with: .\tests\test-quickstart.ps1

$ErrorActionPreference = "Stop"
$scriptPath = Join-Path $PSScriptRoot "..\scripts\quickstart.ps1"
$testsPassed = 0
$testsFailed = 0

function Test-Assert {
    param([bool]$Condition, [string]$Message)

    if ($Condition) {
        Write-Host "  [PASS] $Message" -ForegroundColor Green
        $script:testsPassed++
    } else {
        Write-Host "  [FAIL] $Message" -ForegroundColor Red
        $script:testsFailed++
    }
}

Write-Host ""
Write-Host "  Quickstart Test Suite" -ForegroundColor Cyan
Write-Host "  =====================" -ForegroundColor Cyan
Write-Host ""

# ============================================================================
# Test 1: Script exists
# ============================================================================
Write-Host "  Test: Script exists" -ForegroundColor Yellow
Test-Assert (Test-Path $scriptPath) "quickstart.ps1 exists"

# ============================================================================
# Test 2: No hardcoded personal paths
# ============================================================================
Write-Host ""
Write-Host "  Test: No hardcoded personal paths" -ForegroundColor Yellow

$scriptContent = Get-Content $scriptPath -Raw
Test-Assert (-not ($scriptContent -match "bcmister")) "No 'bcmister' in script"
Test-Assert (-not ($scriptContent -match "\\\.1dev")) "No '.1dev' hardcoded path in script"
Test-Assert (-not ($scriptContent -match "C:\\Users\\[A-Za-z]+\\")) "No hardcoded user paths"
Test-Assert (-not ($scriptContent -match '\$env:USERPROFILE\\dev')) "No default projects directory suggestion"

# ============================================================================
# Test 3: Default config is generic
# ============================================================================
Write-Host ""
Write-Host "  Test: Default config is generic" -ForegroundColor Yellow

# Check that default is 1 window per monitor, not a specific setup
Test-Assert ($scriptContent -match '\$DefaultMonitorConfig = @\{[\s\S]*?0 = @\{ Windows = 1') "Default is 1 window per monitor"
Test-Assert ($scriptContent -match '\[string\]\$ProjectsDir = ""') "ProjectsDir defaults to empty (prompts user)"

# ============================================================================
# Test 4: Required parameters
# ============================================================================
Write-Host ""
Write-Host "  Test: Required parameters exist" -ForegroundColor Yellow

Test-Assert ($scriptContent -match '\[string\]\$ProjectsDir') "ProjectsDir parameter exists"
Test-Assert ($scriptContent -match '\[string\]\$PostCommand') "PostCommand parameter exists"
Test-Assert ($scriptContent -match '\[string\]\$Windows') "Windows parameter exists"
Test-Assert ($scriptContent -match '\[switch\]\$Init') "Init switch exists"
Test-Assert ($scriptContent -match '\[switch\]\$List') "List switch exists"

# ============================================================================
# Test 5: Monitor detection code exists
# ============================================================================
Write-Host ""
Write-Host "  Test: Monitor detection" -ForegroundColor Yellow

Test-Assert ($scriptContent -match 'EnumDisplayMonitors') "Uses EnumDisplayMonitors API"
Test-Assert ($scriptContent -match 'GetMonitorInfo') "Uses GetMonitorInfo API"
Test-Assert ($scriptContent -match 'rcWork') "Uses work area (not full monitor bounds)"

# ============================================================================
# Test 6: Pane splitting support
# ============================================================================
Write-Host ""
Write-Host "  Test: Pane splitting support" -ForegroundColor Yellow

Test-Assert ($scriptContent -match 'sp -V') "Supports vertical split"
Test-Assert ($scriptContent -match 'sp -H') "Supports horizontal split"
Test-Assert ($scriptContent -match 'mf left') "Supports move focus for grid layout"

# ============================================================================
# Test 7: -List shows usage
# ============================================================================
Write-Host ""
Write-Host "  Test: -List shows usage without launching" -ForegroundColor Yellow

$listOutput = & powershell -ExecutionPolicy Bypass -File $scriptPath -List 2>&1 | Out-String
Test-Assert ($listOutput -match "Usage") "List shows usage information"
Test-Assert ($listOutput -match "Detected") "List shows detected monitors"

# ============================================================================
# Test 8: No windows parameter uses defaults
# ============================================================================
Write-Host ""
Write-Host "  Test: Windows parameter parsing" -ForegroundColor Yellow

# The -Windows parameter should parse comma-separated values
Test-Assert ($scriptContent -match '\$windowCounts = \$Windows -split ","') "Parses comma-separated window counts"
Test-Assert ($scriptContent -match 'if \(\$count -eq 1\) \{ "full"') "Single window uses full layout"
Test-Assert ($scriptContent -match 'elseif \(\$count -le 2\) \{ "vertical"') "2 windows uses vertical layout"
Test-Assert ($scriptContent -match 'else \{ "grid"') "3+ windows uses grid layout"

# ============================================================================
# Summary
# ============================================================================
Write-Host ""
Write-Host "  =============================================" -ForegroundColor Cyan
Write-Host "  Results: $testsPassed passed, $testsFailed failed" -ForegroundColor $(if ($testsFailed -eq 0) { "Green" } else { "Red" })
Write-Host ""

if ($testsFailed -gt 0) {
    exit 1
}
