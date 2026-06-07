package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xjwm5685-ui/turbodrop/api"
)

func main() {
	fmt.Println("⚡ TurboDrop - 极速局域网文件传输工具")
	fmt.Println("========================================")
	fmt.Println("🚀 Phase 4: Web API + 实时通信")
	fmt.Println("========================================")
	fmt.Println()

	// 创建 webui 目录（如果不存在）
	os.MkdirAll("./webui", 0755)
	os.MkdirAll("./received_files", 0755)
	os.MkdirAll("./uploads", 0755)
	os.MkdirAll("./data", 0755)

	cfg, err := api.LoadOrCreateConfig()
	if err != nil {
		fmt.Printf("❌ 读取配置失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(cfg.SaveDir, 0755); err != nil {
		fmt.Printf("❌ 创建默认保存目录失败: %v\n", err)
		os.Exit(1)
	}

	// 启动 API 服务器
	server := api.NewServerWithConfig(cfg)

	// 在后台启动服务器
	go func() {
		if err := server.Start(); err != nil {
			fmt.Printf("❌ API 服务器启动失败: %v\n", err)
			os.Exit(1)
		}
	}()

	fmt.Println("✅ TurboDrop API 服务器已启动")
	fmt.Println("📖 使用指南:")
	fmt.Println("   1. 在浏览器中打开 dashboard.html")
	fmt.Printf("   2. 或访问 http://%s/dashboard.html\n", cfg.ListenAddr())
	fmt.Println("   3. 使用 Web 界面进行文件传输")
	fmt.Println()
	fmt.Println("💡 按 Ctrl+C 停止服务器")
	fmt.Println()

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println()
	fmt.Println("👋 正在关闭服务器...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Printf("⚠️  关闭服务器时发生错误: %v\n", err)
	}
}
