package transfer

import (
	"crypto/rand"
	"fmt"
	"os"
)

// GenerateDummyFile 生成指定大小的测试文件
func GenerateDummyFile(filePath string, sizeMB int) error {
	fmt.Printf("📝 生成 %d MB 测试文件...\n", sizeMB)

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// 每次写入 1MB
	bufferSize := 1024 * 1024
	buffer := make([]byte, bufferSize)
	totalBytes := int64(sizeMB) * 1024 * 1024

	for written := int64(0); written < totalBytes; written += int64(bufferSize) {
		// 生成随机数据
		rand.Read(buffer)
		
		// 最后一块可能不足 1MB
		remaining := totalBytes - written
		if remaining < int64(bufferSize) {
			buffer = buffer[:remaining]
		}

		if _, err := file.Write(buffer); err != nil {
			return err
		}

		// 显示进度
		progress := float64(written+int64(len(buffer))) / float64(totalBytes) * 100
		fmt.Printf("\r   进度: %.1f%%", progress)
	}

	fmt.Printf("\r✅ 测试文件已生成: %s (%.2f MB)\n\n", filePath, float64(totalBytes)/(1024*1024))
	return nil
}
