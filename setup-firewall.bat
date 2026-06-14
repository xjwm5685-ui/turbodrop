@echo off
echo TurboDrop Firewall Setup
echo ========================================
echo.
echo This script will add Windows Firewall rule for TurboDrop
echo Allow Web UI TCP 48080, PIN UDP 8899, and QUIC UDP 9001 inbound connections
echo.
echo Administrator privilege required!
echo.
pause

echo.
echo Adding firewall rules...
netsh advfirewall firewall delete rule name="TurboDrop Web UI TCP" >nul 2>nul
netsh advfirewall firewall delete rule name="TurboDrop PIN UDP" >nul 2>nul
netsh advfirewall firewall delete rule name="TurboDrop QUIC UDP" >nul 2>nul
netsh advfirewall firewall add rule name="TurboDrop Web UI TCP" dir=in action=allow protocol=TCP localport=48080 profile=private
netsh advfirewall firewall add rule name="TurboDrop PIN UDP" dir=in action=allow protocol=UDP localport=8899 profile=private
netsh advfirewall firewall add rule name="TurboDrop QUIC UDP" dir=in action=allow protocol=UDP localport=9001 profile=private

if %errorlevel% equ 0 (
    echo.
    echo [SUCCESS] Firewall rules added successfully!
    echo.
    echo Rule name: TurboDrop Web UI TCP
    echo Port: TCP 48080
    echo Rule name: TurboDrop PIN UDP
    echo Port: UDP 8899
    echo Rule name: TurboDrop QUIC UDP
    echo Port: UDP 9001
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
