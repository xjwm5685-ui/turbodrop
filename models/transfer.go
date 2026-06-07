package models

// FileMetadata 文件元数据，通过控制流发送
type FileMetadata struct {
	FileName    string `json:"file_name"`     // 文件名
	FileSize    int64  `json:"file_size"`     // 文件总大小（字节）
	ChunkSize   int64  `json:"chunk_size"`    // 每个分块大小
	TotalChunks int    `json:"total_chunks"`  // 总分块数
	Blake3Hash  string `json:"blake3_hash"`   // 文件完整 BLAKE3 哈希
}

// TransferProgress 传输进度信息
type TransferProgress struct {
	BytesTransferred int64   // 已传输字节数
	TotalBytes       int64   // 总字节数
	SpeedMBps        float64 // 当前速度 (MB/s)
	Percentage       float64 // 完成百分比
}

// ResumeRequest 断点续传请求（接收端 -> 发送端）
type ResumeRequest struct {
	Resume       bool   `json:"resume"`         // 是否需要断点续传
	CompletedMap []byte `json:"completed_map"`  // Bitset 字节数组
	Message      string `json:"message"`        // 状态消息
}

// ResumeACK 断点续传确认（发送端 -> 接收端）
type ResumeACK struct {
	Acknowledged bool   `json:"acknowledged"`   // 确认接收到续传信息
	SkippedCount int    `json:"skipped_count"`  // 跳过的块数
	Message      string `json:"message"`        // 确认消息
}
