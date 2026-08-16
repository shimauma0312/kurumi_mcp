package persona

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const maxInstructionsBytes = 64 * 1024

// 投稿ペルソナをGit管理外のファイルから読み込み。
func Load(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open persona file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxInstructionsBytes+1))
	if err != nil {
		return "", fmt.Errorf("read persona file: %w", err)
	}
	if len(content) > maxInstructionsBytes {
		return "", fmt.Errorf("persona file must be at most %d bytes", maxInstructionsBytes)
	}

	instructions := strings.TrimSpace(string(content))
	if instructions == "" {
		return "", fmt.Errorf("persona file must not be empty")
	}
	return instructions, nil
}
