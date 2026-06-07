@echo off
chcp 65001 >nul
echo ⚡ TurboDrop 自动化测试
echo ========================================
echo.
echo 1. 运行 Go 单元测试...
go test ./...
if %errorlevel% neq 0 (
    echo.
    echo ❌ 单元测试失败
    echo ========================================
    pause
    exit /b 1
)
echo.
echo 2. 运行编译验证...
go build ./...
if %errorlevel% neq 0 (
    echo.
    echo ❌ 编译验证失败
    echo ========================================
    pause
    exit /b 1
)
echo.
echo ✅ 测试与编译验证通过
echo.
echo 💡 如需进一步联调：
echo    1. 运行 turbodrop.exe
echo    2. 浏览器打开 http://127.0.0.1:48080/dashboard.html
echo    3. 进行接收/发送端完整传输验证
echo.
echo ========================================
pause
