package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type SelectFolderRequest struct {
	CurrentPath string `json:"current_path,omitempty"`
}

type SelectFolderResponse struct {
	Success  bool   `json:"success"`
	Path     string `json:"path,omitempty"`
	Canceled bool   `json:"canceled,omitempty"`
	Message  string `json:"message,omitempty"`
}

func (s *Server) handleSelectSaveDir(w http.ResponseWriter, r *http.Request) {
	var req SelectFolderRequest
	if err := decodeJSONBody(w, r, &req, maxJSONBodyBytes); err != nil && !errors.Is(err, io.EOF) {
		respondError(w, http.StatusBadRequest, "无效的文件夹选择请求")
		return
	}

	currentPath := strings.TrimSpace(req.CurrentPath)
	if currentPath == "" {
		currentPath = s.getConfig().SaveDir
	}

	selectedPath, canceled, err := selectFolder(currentPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "打开文件夹选择器失败: "+err.Error())
		return
	}
	if canceled {
		respondJSON(w, http.StatusOK, SelectFolderResponse{
			Success:  true,
			Canceled: true,
			Message:  "已取消选择",
		})
		return
	}

	respondJSON(w, http.StatusOK, SelectFolderResponse{
		Success: true,
		Path:    selectedPath,
		Message: "已选择默认保存位置",
	})
}

func selectFolder(currentPath string) (string, bool, error) {
	switch runtime.GOOS {
	case "windows":
		return selectFolderWindows(currentPath)
	case "darwin":
		return selectFolderMacOS()
	case "linux":
		return selectFolderLinux()
	default:
		return "", false, fmt.Errorf("当前平台暂不支持目录选择器: %s", runtime.GOOS)
	}
}

func selectFolderWindows(currentPath string) (string, bool, error) {
	startDir := buildDialogStartDir(currentPath)
	scriptLines := []string{
		"[Console]::OutputEncoding = [System.Text.Encoding]::UTF8",
		"Add-Type -AssemblyName System.Windows.Forms",
		"$dialog = New-Object System.Windows.Forms.FolderBrowserDialog",
		"$dialog.Description = '选择 TurboDrop 默认保存位置'",
		"$dialog.ShowNewFolderButton = $true",
	}
	if startDir != "" {
		scriptLines = append(scriptLines, fmt.Sprintf("$dialog.SelectedPath = '%s'", escapePowerShellSingleQuoted(startDir)))
	}
	scriptLines = append(scriptLines, "if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $dialog.SelectedPath }")

	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-Command", strings.Join(scriptLines, "; "))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}

	selectedPath := parseDialogSelection(string(output))
	if selectedPath == "" {
		return "", true, nil
	}
	return selectedPath, false, nil
}

func selectFolderMacOS() (string, bool, error) {
	cmd := exec.Command("osascript", "-e", `POSIX path of (choose folder with prompt "选择 TurboDrop 默认保存位置")`)
	output, err := cmd.CombinedOutput()
	if err != nil {
		errText := strings.TrimSpace(string(output))
		if strings.Contains(errText, "User canceled") {
			return "", true, nil
		}
		return "", false, fmt.Errorf("%w: %s", err, errText)
	}

	selectedPath := parseDialogSelection(string(output))
	if selectedPath == "" {
		return "", true, nil
	}
	return selectedPath, false, nil
}

func selectFolderLinux() (string, bool, error) {
	commands := [][]string{
		{"zenity", "--file-selection", "--directory", "--title=选择 TurboDrop 默认保存位置"},
		{"kdialog", "--getexistingdirectory", "", "选择 TurboDrop 默认保存位置"},
	}

	for _, args := range commands {
		if _, err := exec.LookPath(args[0]); err != nil {
			continue
		}

		cmd := exec.Command(args[0], args[1:]...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if len(output) == 0 {
				return "", true, nil
			}
			return "", false, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
		}

		selectedPath := parseDialogSelection(string(output))
		if selectedPath == "" {
			return "", true, nil
		}
		return selectedPath, false, nil
	}

	return "", false, fmt.Errorf("未找到可用的目录选择器，请安装 zenity 或 kdialog")
}

func buildDialogStartDir(currentPath string) string {
	path := strings.TrimSpace(currentPath)
	if path == "" {
		return ""
	}

	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err == nil {
			path = absPath
		}
	}

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return ""
	}
	return path
}

func parseDialogSelection(output string) string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func escapePowerShellSingleQuoted(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
