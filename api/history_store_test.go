package api

import (
	"path/filepath"
	"testing"
)

func TestHistoryStoreAddAndList(t *testing.T) {
	store := NewHistoryStore(filepath.Join(t.TempDir(), "history.json"))

	if err := store.Add(TransferHistoryEntry{
		FileName:    "a.txt",
		Status:      "success",
		Message:     "ok",
		Size:        123,
		StartedAt:   "2026-06-07T10:00:00Z",
		CompletedAt: "2026-06-07T10:00:01Z",
	}); err != nil {
		t.Fatalf("Add() 失败: %v", err)
	}

	if err := store.Add(TransferHistoryEntry{
		FileName:    "b.txt",
		Status:      "failed",
		Message:     "boom",
		Size:        456,
		StartedAt:   "2026-06-07T10:01:00Z",
		CompletedAt: "2026-06-07T10:01:01Z",
	}); err != nil {
		t.Fatalf("Add() 第二次失败: %v", err)
	}

	items, err := store.List()
	if err != nil {
		t.Fatalf("List() 失败: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("List() 返回 %d 条记录, want 2", len(items))
	}

	if items[0].FileName != "b.txt" || items[1].FileName != "a.txt" {
		t.Fatalf("历史记录顺序错误: got [%s, %s]", items[0].FileName, items[1].FileName)
	}

	if items[0].ID == "" || items[1].ID == "" {
		t.Fatalf("历史记录应自动生成 ID")
	}
}
