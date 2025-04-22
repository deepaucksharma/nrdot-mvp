# NRDOT-MVP Command Line Tool (PowerShell version)
# A unified interface for managing the NRDOT-MVP system on Windows

# Configuration with defaults
$COLLECTOR_URL = if ($env:COLLECTOR_URL) { $env:COLLECTOR_URL } else { "http://localhost:4318" }
$UPSTREAM_URL = if ($env:UPSTREAM_URL) { $env:UPSTREAM_URL } else { "http://localhost:4319" }
$PROM_URL = if ($env:PROM_URL) { $env:PROM_URL } else { "http://localhost:9090" }
$SCRIPT_DIR = $PSScriptRoot

# Function to display help
function Show-Help {
  Write-Host "NRDOT-MVP Command Line Tool (PowerShell)"
  Write-Host
  Write-Host "Usage: .\nrdot-cli.ps1 <command> [options]"
  Write-Host
  Write-Host "Commands:"
  Write-Host "  build               Build all components"
  Write-Host "  start               Start the system (shorthand for 'up')"
  Write-Host "  up                  Start all containers"
  Write-Host "  down                Stop all containers"
  Write-Host "  status              Show system status"
  Write-Host "  report              Generate detailed system report"
  Write-Host "  test                Run full test sequence"
  Write-Host "  clean               Clean all build artifacts"
  Write-Host "  reset               Reset the system completely"
  Write-Host
  Write-Host "Environment variables:"
  Write-Host "  `$env:COLLECTOR_URL  Collector URL (default: $COLLECTOR_URL)"
  Write-Host "  `$env:UPSTREAM_URL   Mock upstream URL (default: $UPSTREAM_URL)"
  Write-Host "  `$env:PROM_URL       Prometheus URL (default: $PROM_URL)"
  Write-Host
  Write-Host "Examples:"
  Write-Host "  .\nrdot-cli.ps1 start         Start the system"
  Write-Host "  .\nrdot-cli.ps1 status        Check status" 
  Write-Host
}

# Function to check prerequisites
function Check-Prerequisites {
  $missing = 0
  
  if (-not (Get-Command "curl" -ErrorAction SilentlyContinue)) {
    Write-Host "❌ curl is required but not found"
    $missing = 1
  }
  
  if (-not (Get-Command "docker" -ErrorAction SilentlyContinue)) {
    Write-Host "❌ docker is required but not found"
    $missing = 1
  }
  
  if ($missing -eq 1) {
    Write-Host "Please install missing prerequisites and try again."
    exit 1
  }
}

# Function to check if system is running
function Check-System {
  try {
    $response = Invoke-WebRequest -Uri "$COLLECTOR_URL/v1/metrics" -Method Head -UseBasicParsing -ErrorAction SilentlyContinue
    $upstreamResponse = Invoke-WebRequest -Uri "$UPSTREAM_URL/control/status" -Method Head -UseBasicParsing -ErrorAction SilentlyContinue
    return $true
  }
  catch {
    Write-Host "❌ System not running. Start with: .\nrdot-cli.ps1 start"
    return $false
  }
}

# If no command provided, show help
if ($args.Count -eq 0) {
  Show-Help
  exit 0
}

# Process commands
$command = $args[0]

switch ($command) {
  "help" {
    Show-Help
  }
  
  "build" {
    Write-Host "Building all components..."
    Set-Location $SCRIPT_DIR
    if (Test-Path build.sh) {
      # Try with Git Bash if available
      if (Get-Command "bash" -ErrorAction SilentlyContinue) {
        bash build.sh
      }
      else {
        Write-Host "Error: bash not found. Please install Git Bash or run in WSL."
      }
    }
    else {
      Write-Host "Error: build.sh not found"
    }
  }
  
  { ($_ -eq "up") -or ($_ -eq "start") } {
    Write-Host "Starting all containers..."
    Set-Location $SCRIPT_DIR
    
    if (-not (Test-Path "data/dlq")) {
      New-Item -Path "data/dlq" -ItemType Directory -Force | Out-Null
    }
    
    docker-compose up -d
    Write-Host "System started. Check status with: .\nrdot-cli.ps1 status"
  }
  
  { ($_ -eq "down") -or ($_ -eq "stop") } {
    Write-Host "Stopping all containers..."
    Set-Location $SCRIPT_DIR
    docker-compose down
    Write-Host "System stopped."
  }
  
  "status" {
    if (Check-System) {
      Write-Host "✅ System is running"
      Write-Host
      Write-Host "Container status:"
      docker ps --format "table {{.Names}}`t{{.Status}}`t{{.Ports}}" | Select-String -Pattern "collector|mock-upstream|prometheus|grafana"
      Write-Host
      Write-Host "Mock upstream status:"
      Invoke-RestMethod -Uri "$UPSTREAM_URL/control/status" | ConvertTo-Json
    }
  }
  
  "report" {
    if (Check-System) {
      Set-Location $SCRIPT_DIR
      if (Get-Command "bash" -ErrorAction SilentlyContinue) {
        $env:COLLECTOR_URL = $COLLECTOR_URL
        $env:UPSTREAM_URL = $UPSTREAM_URL
        $env:PROM_URL = $PROM_URL
        bash -c "chmod +x ./scripts/report.sh && ./scripts/report.sh"
      }
      else {
        Write-Host "Error: bash not found. Please install Git Bash or run in WSL."
      }
    }
  }
  
  "clean" {
    Write-Host "Cleaning build artifacts..."
    Set-Location $SCRIPT_DIR
    if (Test-Path "bin") {
      Remove-Item -Path "bin/*" -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path "plugins/*.so") {
      Remove-Item -Path "plugins/*.so" -Force -ErrorAction SilentlyContinue
    }
    Write-Host "Clean complete."
  }
  
  "reset" {
    Write-Host "Resetting the system..."
    Set-Location $SCRIPT_DIR
    docker-compose down -v
    if (Test-Path "data/dlq") {
      Remove-Item -Path "data/dlq/*" -Force -Recurse -ErrorAction SilentlyContinue
    }
    Write-Host "Starting fresh system..."
    docker-compose up -d
    Write-Host "System reset complete."
  }
  
  "test" {
    Write-Host "Running test is not fully supported in PowerShell version."
    Write-Host "Please use bash version or run tests manually."
    Write-Host "You can still use individual commands like status, report, etc."
  }
  
  default {
    Write-Host "Unknown command: $command"
    Write-Host
    Show-Help
  }
}
