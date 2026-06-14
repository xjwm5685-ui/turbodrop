@echo off
chcp 65001 >nul
cd /d "%~dp0"
title TurboDrop

echo.
echo ========================================
echo   TurboDrop
echo ========================================
echo.
echo Starting local Web UI...
echo Computer: http://localhost:48080/dashboard.html
echo Phone:    use the LAN URL printed by TurboDrop
echo.

start "" powershell -NoProfile -WindowStyle Hidden -Command "Start-Sleep -Seconds 2; Start-Process 'http://localhost:48080/dashboard.html'"
turbodrop.exe

echo.
echo TurboDrop has stopped.
pause
