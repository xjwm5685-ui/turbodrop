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

Write-Host "Adding firewall rules for TurboDrop LAN access..." -ForegroundColor Yellow
Write-Host ""

try {
    $rules = @(
        @{ Name = "TurboDrop Web UI TCP"; Protocol = "TCP"; Port = 48080 },
        @{ Name = "TurboDrop PIN UDP"; Protocol = "UDP"; Port = 8899 },
        @{ Name = "TurboDrop QUIC UDP"; Protocol = "UDP"; Port = 9001 }
    )

    foreach ($rule in $rules) {
        Remove-NetFirewallRule -DisplayName $rule.Name -ErrorAction SilentlyContinue
        New-NetFirewallRule -DisplayName $rule.Name `
                            -Direction Inbound `
                            -Action Allow `
                            -Protocol $rule.Protocol `
                            -LocalPort $rule.Port `
                            -Profile Private `
                            -ErrorAction Stop | Out-Null
    }
    
    Write-Host "[SUCCESS] Firewall rules added successfully!" -ForegroundColor Green
    Write-Host ""
    Write-Host "Rule Details:" -ForegroundColor Cyan
    Write-Host "  TurboDrop Web UI TCP : TCP 48080"
    Write-Host "  TurboDrop PIN UDP    : UDP 8899"
    Write-Host "  TurboDrop QUIC UDP   : UDP 9001"
    Write-Host "  Profile              : Private"
    Write-Host ""
} catch {
    Write-Host "[ERROR] Failed to add firewall rules!" -ForegroundColor Red
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host ""
}

Write-Host "Press any key to exit..."
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
