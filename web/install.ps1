# GenDrive Installer
$BaseUrl = "http://drive.chirag404.me"
$Dest = "$env:USERPROFILE\GenDrive"

Write-Host "Installing GenDrive to $Dest..." -ForegroundColor Green

# Create Directory
if (!(Test-Path $Dest)) {
    New-Item -ItemType Directory -Force -Path $Dest | Out-Null
}

# Download Agent
Write-Host "Downloading Agent..."
try {
    Invoke-RestMethod -Uri "$BaseUrl/agent.exe" -OutFile "$Dest\agent.exe"
} catch {
    Write-Error "Failed to download agent. Ensure Server is running at $BaseUrl"
    exit 1
}

# Run Agent
Write-Host "Starting GenDrive Agent..."
Start-Process "$Dest\agent.exe" -ArgumentList "-server $BaseUrl" -WorkingDirectory $Dest

Write-Host "GenDrive Started! Check the console window for Claim Token." -ForegroundColor Cyan
