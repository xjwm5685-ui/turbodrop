@echo off
echo TurboDrop Firewall Removal
echo ========================================
echo.
echo This script will remove TurboDrop firewall rule
echo.
pause

echo.
echo Removing firewall rule...
netsh advfirewall firewall delete rule name="TurboDrop UDP"

if %errorlevel% equ 0 (
    echo.
    echo [SUCCESS] Firewall rule removed successfully!
    echo.
) else (
    echo.
    echo [WARNING] Rule not found or removal failed
    echo.
)

pause
