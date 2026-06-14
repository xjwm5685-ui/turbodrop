@echo off
echo TurboDrop Firewall Removal
echo ========================================
echo.
echo This script will remove TurboDrop firewall rules
echo.
pause

echo.
echo Removing firewall rules...
netsh advfirewall firewall delete rule name="TurboDrop Web UI TCP"
netsh advfirewall firewall delete rule name="TurboDrop PIN UDP"
netsh advfirewall firewall delete rule name="TurboDrop QUIC UDP"
netsh advfirewall firewall delete rule name="TurboDrop UDP"

if %errorlevel% equ 0 (
    echo.
    echo [SUCCESS] Firewall rules removed successfully!
    echo.
) else (
    echo.
    echo [WARNING] Rule not found or removal failed
    echo.
)

pause
