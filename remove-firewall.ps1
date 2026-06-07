# TurboDrop Firewall Removal Script
# Run as Administrator

Write-Host "========================================"  -ForegroundColor Cyan
Write-Host "  TurboDrop Firewall Removal" -ForegroundColor Cyan
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

Write-Host "Removing TurboDrop firewall rule..." -ForegroundColor Yellow
Write-Host ""

try {
    Remove-NetFirewallRule -DisplayName "TurboDrop UDP" -ErrorAction Stop
    Write-Host "[SUCCESS] Firewall rule removed successfully!" -ForegroundColor Green
    Write-Host ""
} catch {
    Write-Host "[WARNING] Rule not found or removal failed" -ForegroundColor Yellow
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Yellow
    Write-Host ""
}

Write-Host "Press any key to exit..."
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
