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

Write-Host "Removing TurboDrop firewall rules..." -ForegroundColor Yellow
Write-Host ""

try {
    $rules = @(
        "TurboDrop Web UI TCP",
        "TurboDrop PIN UDP",
        "TurboDrop QUIC UDP",
        "TurboDrop UDP"
    )

    foreach ($rule in $rules) {
        Remove-NetFirewallRule -DisplayName $rule -ErrorAction SilentlyContinue
    }

    Write-Host "[SUCCESS] Firewall rules removed successfully!" -ForegroundColor Green
    Write-Host ""
} catch {
    Write-Host "[WARNING] Rules not found or removal failed" -ForegroundColor Yellow
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Yellow
    Write-Host ""
}

Write-Host "Press any key to exit..."
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
