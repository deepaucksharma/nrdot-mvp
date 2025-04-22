# PowerShell script to test core components
Write-Host "Testing NRDOT-MVP core components"

# Test directory structure
Write-Host "Checking directory structure..."
if (Test-Path -Path ".\plugins" -PathType Container) {
    Write-Host "Plugins directory exists"
} else {
    Write-Host "Plugins directory missing"
}

if (Test-Path -Path ".\otel-config" -PathType Container) {
    Write-Host "Config directory exists"
} else {
    Write-Host "Config directory missing"
}

# Check core files
Write-Host "Checking if fixes were applied..."
$mainFile = Get-Content -Path ".\cmd\collector\main.go" -Raw
if ($mainFile -match "import\s+\(.*context" -and $mainFile -match "import\s+\(.*strconv") {
    Write-Host "Fixed missing imports in collector/main.go"
} else {
    Write-Host "Collector imports still missing"
}

$apqFile = Get-Content -Path ".\plugins\apq\plugin.go" -Raw
if ($apqFile -match "NewFactory\(\).*exporter.NewFactory\(") {
    Write-Host "Fixed APQ NewFactory implementation"
} else {
    Write-Host "APQ NewFactory still returning nil"
}

$buildFile = Get-Content -Path ".\build.sh" -Raw
if ($buildFile -match "mkdir -p bin" -and $buildFile -match "plugins/dlq.so") {
    Write-Host "Fixed build.sh to create bin dir and build DLQ plugin"
} else {
    Write-Host "build.sh still missing key fixes"
}

# Check mock-upstream for rand import
$mockFile = Get-Content -Path ".\cmd\mock-upstream\main.go" -Raw
if ($mockFile -match "import\s+\(.*math/rand") {
    Write-Host "Fixed missing math/rand import in mock-upstream"
} else {
    Write-Host "mock-upstream still missing math/rand import"
}

# Check Dockerfile.collector for DLQ plugin
$dockerfile = Get-Content -Path ".\Dockerfile.collector" -Raw
if ($dockerfile -match "COPY.*dlq.so") {
    Write-Host "Dockerfile.collector includes DLQ plugin"
} else {
    Write-Host "Dockerfile.collector missing DLQ plugin"
}

Write-Host "Component validation complete."
