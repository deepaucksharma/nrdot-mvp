# Simple verification script
Write-Host "Verifying NRDOT-MVP fixes implementation"

# Check for presence of key files
$fileChecks = @(
    @{Path="cmd\collector\main.go"; Name="Collector main"},
    @{Path="cmd\mock-upstream\main.go"; Name="Mock upstream"},
    @{Path="plugins\apq\plugin.go"; Name="APQ plugin"},
    @{Path="plugins\cl\plugin.go"; Name="CL plugin"},
    @{Path="plugins\dlq\file_storage.go"; Name="DLQ plugin"},
    @{Path="build.sh"; Name="Build script"}
)

foreach ($check in $fileChecks) {
    if (Test-Path $check.Path) {
        Write-Host "$($check.Name) exists" -ForegroundColor Green
    } else {
        Write-Host "$($check.Name) missing" -ForegroundColor Red
    }
}

# Check for key fixes in build.sh
$buildSh = Get-Content -Path "build.sh" -Raw -ErrorAction SilentlyContinue
if ($buildSh -match "mkdir") {
    Write-Host "build.sh contains mkdir command - Fixed" -ForegroundColor Green
} else {
    Write-Host "build.sh missing mkdir command" -ForegroundColor Red
}

if ($buildSh -match "dlq.so") {
    Write-Host "build.sh builds DLQ plugin - Fixed" -ForegroundColor Green
} else {
    Write-Host "build.sh missing DLQ plugin build" -ForegroundColor Red
}

# Check for fixes in collector main.go
$collectorMain = Get-Content -Path "cmd\collector\main.go" -Raw -ErrorAction SilentlyContinue
if ($collectorMain -match "import.*context") {
    Write-Host "collector/main.go includes context import - Fixed" -ForegroundColor Green
} else {
    Write-Host "collector/main.go missing context import" -ForegroundColor Red
}

if ($collectorMain -match "import.*strconv") {
    Write-Host "collector/main.go includes strconv import - Fixed" -ForegroundColor Green
} else {
    Write-Host "collector/main.go missing strconv import" -ForegroundColor Red
}

# Check for fixes in APQ plugin
$apqPlugin = Get-Content -Path "plugins\apq\plugin.go" -Raw -ErrorAction SilentlyContinue
if ($apqPlugin -match "NewFactory\(\).*return exporter\.NewFactory\(") {
    Write-Host "APQ plugin implements NewFactory properly - Fixed" -ForegroundColor Green
} else {
    Write-Host "APQ plugin missing proper NewFactory implementation" -ForegroundColor Red
}

# Check for fixes in mock-upstream
$mockUpstream = Get-Content -Path "cmd\mock-upstream\main.go" -Raw -ErrorAction SilentlyContinue
if ($mockUpstream -match "import.*math/rand") {
    Write-Host "mock-upstream includes math/rand import - Fixed" -ForegroundColor Green
} else {
    Write-Host "mock-upstream missing math/rand import" -ForegroundColor Red
}

Write-Host "`nVerification complete."
