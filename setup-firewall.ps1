# TurboDrop Firewall Setup Script
# Run as Administrator

Write-Host "========================================"  -ForegroundColor Cyan
Write-Host "  TurboDrop Firewall Configuration" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Check if running as Administrator
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    Write-Host "[ERROR] This script requires Administrator privileges!" -ForegroundColor Red
    Write-Host ""
    Write-Host "Please right-click this file and select 'Run with PowerShell as Administrator'" -ForegroundColor Yellow
    Write-Host ""
    Pause
    exit 1
}

Write-Host "Adding firewall rule for UDP port 8899..." -ForegroundColor Yellow
Write-Host ""

try {
    # Remove existing rule if exists
    Remove-NetFirewallRule -DisplayName "TurboDrop UDP" -ErrorAction SilentlyContinue
    
    # Add new rule
    New-NetFirewallRule -DisplayName "TurboDrop UDP" `
                        -Direction Inbound `
                        -Action Allow `
                        -Protocol UDP `
                        -LocalPort 8899 `
                        -ErrorAction Stop
    
    Write-Host "[SUCCESS] Firewall rule added successfully!" -ForegroundColor Green
    Write-Host ""
    Write-Host "Rule Details:" -ForegroundColor Cyan
    Write-Host "  Name: TurboDrop UDP"
    Write-Host "  Port: UDP 8899"
    Write-Host "  Direction: Inbound"
    Write-Host "  Action: Allow"
    Write-Host ""
} catch {
    Write-Host "[ERROR] Failed to add firewall rule!" -ForegroundColor Red
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host ""
}

Write-Host "Press any key to exit..."
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
