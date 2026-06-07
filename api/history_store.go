package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	dataDir         = "./data"
	historyFilePath = "./data/transfer_history.json"
	maxHistoryItems = 200
)

// TransferHistoryEntry 表示一条持久化传输历史
type TransferHistoryEntry struct {
	ID          string `json:"id"`
	FileName    string `json:"filename"`
	FilePath    string `json:"filepath,omitempty"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Size        int64  `json:"size"`
	PIN         string `json:"pin,omitempty"`
	DeviceName  string `json:"device_name,omitempty"`
	DeviceIP    string `json:"device_ip,omitempty"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at,omitempty"`
}

// HistoryStore 管理历史记录的本地持久化
type HistoryStore struct {
	filePath string
	mutex    sync.RWMutex
}

func NewHistoryStore(filePath string) *HistoryStore {
	return &HistoryStore{filePath: filePath}
}

func (s *HistoryStore) List() ([]TransferHistoryEntry, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.readUnlocked()
}

func (s *HistoryStore) Add(entry TransferHistoryEntry) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	entries, err := s.readUnlocked()
	if err != nil {
		return err
	}

	entry.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	entries = append([]TransferHistoryEntry{entry}, entries...)
	if len(entries) > maxHistoryItems {
		entries = entries[:maxHistoryItems]
	}

	return s.writeUnlocked(entries)
}

func (s *HistoryStore) readUnlocked() ([]TransferHistoryEntry, error) {
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		return nil, fmt.Errorf("创建历史目录失败: %w", err)
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []TransferHistoryEntry{}, nil
		}
		return nil, fmt.Errorf("读取历史文件失败: %w", err)
	}

	if len(data) == 0 {
		return []TransferHistoryEntry{}, nil
	}

	var entries []TransferHistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("解析历史文件失败: %w", err)
	}

	return entries, nil
}

func (s *HistoryStore) writeUnlocked(entries []TransferHistoryEntry) error {
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		return fmt.Errorf("创建历史目录失败: %w", err)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化历史失败: %w", err)
	}

	tempFile := s.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("写入历史临时文件失败: %w", err)
	}

	if err := os.Rename(tempFile, s.filePath); err != nil {
		return fmt.Errorf("保存历史文件失败: %w", err)
	}

	return nil
}
