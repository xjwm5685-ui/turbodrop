@echo off
chcp 65001 >nul
echo ⚡ 正在编译 TurboDrop...
go build -o turbodrop.exe .
if %errorlevel% equ 0 (
    echo ✅ 编译成功！可执行文件: turbodrop.exe
    echo 💡 如需跨平台构建，请运行: powershell -ExecutionPolicy Bypass -File .\build-all.ps1
) else (
    echo ❌ 编译失败
)
pause
