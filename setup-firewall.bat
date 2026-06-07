@echo off
echo TurboDrop Firewall Setup
echo ========================================
echo.
echo This script will add Windows Firewall rule for TurboDrop
echo Allow UDP port 8899 inbound connection
echo.
echo Administrator privilege required!
echo.
pause

echo.
echo Adding firewall rule...
netsh advfirewall firewall add rule name="TurboDrop UDP" dir=in action=allow protocol=UDP localport=8899

if %errorlevel% equ 0 (
    echo.
    echo [SUCCESS] Firewall rule added successfully!
    echo.
    echo Rule name: TurboDrop UDP
    echo Port: UDP 8899
    echo Direction: Inbound
    echo.
) else (
    echo.
    echo [ERROR] Failed to add rule! Please run as Administrator
    echo.
    echo Right-click this file and select "Run as administrator"
    echo.
)

pause
