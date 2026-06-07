package transfer

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/bits-and-blooms/bitset"
	"github.com/xjwm5685-ui/turbodrop/models"
)

// TransferState 传输状态，持久化到 .turbo 文件
type TransferState struct {
	Metadata       models.FileMetadata `json:"metadata"`         // 文件元数据
	CompletedMask  []uint64            `json:"completed_mask"`   // Bitset 的二进制表示
	TotalChunks    int                 `json:"total_chunks"`     // 总块数
	CompletedCount int                 `json:"completed_count"`  // 已完成块数
}

// StateManager 状态管理器，管理 .turbo 影子文件
type StateManager struct {
	filePath    string         // 目标文件路径
	stateFile   string         // .turbo 状态文件路径
	bitset      *bitset.BitSet // 位图，记录每个 Chunk 的完成状态
	totalChunks int            // 总块数
	mutex       sync.RWMutex   // 读写锁，保护并发访问
	metadata    models.FileMetadata
}

// NewStateManager 创建状态管理器
func NewStateManager(filePath string, metadata models.FileMetadata) *StateManager {
	return &StateManager{
		filePath:    filePath,
		stateFile:   filePath + ".turbo",
		bitset:      bitset.New(uint(metadata.TotalChunks)),
		totalChunks: metadata.TotalChunks,
		metadata:    metadata,
	}
}

// LoadState 从 .turbo 文件加载状态
func LoadState(filePath string) (*StateManager, error) {
	stateFile := filePath + ".turbo"

	// 读取状态文件
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, fmt.Errorf("读取状态文件失败: %w", err)
	}

	// 反序列化
	var state TransferState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("解析状态文件失败: %w", err)
	}

	// 重建 Bitset
	bs := bitset.New(uint(state.TotalChunks))
	for i := 0; i < state.TotalChunks; i++ {
		wordIdx := i / 64
		bitIdx := uint(i % 64)
		if wordIdx < len(state.CompletedMask) {
			if (state.CompletedMask[wordIdx] & (1 << bitIdx)) != 0 {
				bs.Set(uint(i))
			}
		}
	}

	sm := &StateManager{
		filePath:    filePath,
		stateFile:   stateFile,
		bitset:      bs,
		totalChunks: state.TotalChunks,
		metadata:    state.Metadata,
	}

	return sm, nil
}

// CheckExists 检查是否存在未完成的传输
func CheckExists(filePath string) bool {
	stateFile := filePath + ".turbo"
	_, err := os.Stat(stateFile)
	return err == nil
}

// MarkChunkComplete 标记某个 Chunk 为已完成
func (sm *StateManager) MarkChunkComplete(chunkIndex int) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// 设置位图
	sm.bitset.Set(uint(chunkIndex))

	// 持久化到磁盘
	return sm.save()
}

// IsChunkComplete 检查某个 Chunk 是否已完成
func (sm *StateManager) IsChunkComplete(chunkIndex int) bool {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.bitset.Test(uint(chunkIndex))
}

// GetCompletedCount 获取已完成的 Chunk 数量
func (sm *StateManager) GetCompletedCount() int {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return int(sm.bitset.Count())
}

// GetCompletionPercentage 获取完成百分比
func (sm *StateManager) GetCompletionPercentage() float64 {
	completed := sm.GetCompletedCount()
	return float64(completed) / float64(sm.totalChunks) * 100
}

// GetBitsetBytes 获取 Bitset 的字节表示（用于网络传输）
func (sm *StateManager) GetBitsetBytes() []byte {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	bytes, _ := sm.bitset.MarshalBinary()
	return bytes
}

// SetBitsetFromBytes 从字节数据设置 Bitset（用于网络接收）
func (sm *StateManager) SetBitsetFromBytes(data []byte) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	return sm.bitset.UnmarshalBinary(data)
}

// GetMissingChunks 获取所有未完成的 Chunk 索引列表
func (sm *StateManager) GetMissingChunks() []int {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	var missing []int
	for i := 0; i < sm.totalChunks; i++ {
		if !sm.bitset.Test(uint(i)) {
			missing = append(missing, i)
		}
	}
	return missing
}

// save 保存状态到 .turbo 文件（内部方法，调用前需加锁）
func (sm *StateManager) save() error {
	// 获取 Words（手动实现）
	numWords := (sm.totalChunks + 63) / 64
	words := make([]uint64, numWords)
	for i := 0; i < sm.totalChunks; i++ {
		if sm.bitset.Test(uint(i)) {
			wordIdx := i / 64
			bitIdx := uint(i % 64)
			words[wordIdx] |= (1 << bitIdx)
		}
	}

	// 构造状态对象
	state := TransferState{
		Metadata:       sm.metadata,
		CompletedMask:  words,
		TotalChunks:    sm.totalChunks,
		CompletedCount: int(sm.bitset.Count()),
	}

	// 序列化为 JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化状态失败: %w", err)
	}

	// 原子写入（先写临时文件，再重命名）
	tempFile := sm.stateFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("写入临时状态文件失败: %w", err)
	}

	if err := os.Rename(tempFile, sm.stateFile); err != nil {
		return fmt.Errorf("重命名状态文件失败: %w", err)
	}

	return nil
}

// SaveState 手动保存状态（外部调用）
func (sm *StateManager) SaveState() error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	return sm.save()
}

// DeleteState 删除状态文件（传输完成后调用）
func (sm *StateManager) DeleteState() error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if err := os.Remove(sm.stateFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除状态文件失败: %w", err)
	}

	fmt.Println("🗑️  状态文件已删除")
	return nil
}

// GetMetadata 获取文件元数据
func (sm *StateManager) GetMetadata() models.FileMetadata {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return sm.metadata
}

// PrintStatus 打印当前状态（调试用）
func (sm *StateManager) PrintStatus() {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	completed := sm.bitset.Count()
	percentage := float64(completed) / float64(sm.totalChunks) * 100

	fmt.Printf("📊 传输状态: %d/%d 块 (%.1f%%)\n", completed, sm.totalChunks, percentage)
	fmt.Printf("   状态文件: %s\n", sm.stateFile)
}
