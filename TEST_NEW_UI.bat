@echo off
chcp 65001 >nul
title TurboDrop - Test New UI

echo.
echo ⚡ TurboDrop - Velocity Terminal UI Test
echo ========================================
echo.

echo 📋 Checking files...
echo.

if exist "webui\dashboard.html" (
    echo ✅ dashboard.html found
) else (
    echo ❌ dashboard.html NOT found
    goto :error
)

if exist "webui\dashboard-old.html" (
    echo ✅ dashboard-old.html found (backup)
) else (
    echo ⚠️  dashboard-old.html NOT found (no backup)
)

echo.
echo 🔍 Checking file sizes...
echo.

for %%F in (webui\dashboard.html) do echo    New UI: %%~zF bytes
for %%F in (webui\dashboard-old.html) do echo    Old UI: %%~zF bytes

echo.
echo 🚀 Starting TurboDrop server...
echo.
echo    Web UI will be available at:
echo    http://127.0.0.1:48080/dashboard.html
echo.
echo    Press Ctrl+C to stop the server
echo.
echo ========================================
echo.

turbodrop.exe

goto :end

:error
echo.
echo ❌ Error: Required files not found
echo    Please run the project from the turbodrop directory
echo.
pause
exit /b 1

:end
