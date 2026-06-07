package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDialogSelection(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "empty output",
			output: "",
			want:   "",
		},
		{
			name:   "single line path",
			output: "D:\\Downloads\\TurboDrop\r\n",
			want:   "D:\\Downloads\\TurboDrop",
		},
		{
			name:   "takes last non-empty line",
			output: "some notice\r\nD:\\Receive Files\r\n\r\n",
			want:   "D:\\Receive Files",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDialogSelection(tc.output)
			if got != tc.want {
				t.Fatalf("parseDialogSelection(%q) = %q, want %q", tc.output, got, tc.want)
			}
		})
	}
}

func TestBuildDialogStartDir(t *testing.T) {
	tempDir, err := os.MkdirTemp(".", "dialog-start-dir-*")
	if err != nil {
		t.Fatalf("os.MkdirTemp 失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	relativeDir, err := filepath.Rel(".", tempDir)
	if err != nil {
		t.Fatalf("filepath.Rel 失败: %v", err)
	}

	got := buildDialogStartDir(relativeDir)
	want, err := filepath.Abs(relativeDir)
	if err != nil {
		t.Fatalf("filepath.Abs 失败: %v", err)
	}
	if got != want {
		t.Fatalf("buildDialogStartDir(%q) = %q, want %q", relativeDir, got, want)
	}

	if got := buildDialogStartDir(filepath.Join(tempDir, "missing")); got != "" {
		t.Fatalf("缺失目录应返回空字符串, got %q", got)
	}
}

func TestEscapePowerShellSingleQuoted(t *testing.T) {
	input := `D:\Users\O'Brien\Downloads`
	want := `D:\Users\O''Brien\Downloads`
	if got := escapePowerShellSingleQuoted(input); got != want {
		t.Fatalf("escapePowerShellSingleQuoted(%q) = %q, want %q", input, got, want)
	}
}
