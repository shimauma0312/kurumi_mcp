package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// UTF-8の複数行ペルソナを読み込み、ファイル全体の内容を維持しながら
// ファイル先頭と末尾に付いた不要な空白だけを除去することを確認する。
func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persona.md")
	if err := os.WriteFile(path, []byte("\n 投稿方針:\n- 簡潔に書く。\n- 記号は◆。 \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "投稿方針:\n- 簡潔に書く。\n- 記号は◆。"
	if got != want {
		t.Fatalf("Load() = %q, want %q", got, want)
	}
}

// ペルソナファイルが存在しない場合に起動を続行せず、
// 投稿方針なしのMCPサーバーが意図せず公開されないことを確認する。
func TestLoadRejectsMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.md"))
	if err == nil || !strings.Contains(err.Error(), "open persona file") {
		t.Fatalf("Load() error = %v, want missing-file error", err)
	}
}

// 空白だけのファイルを拒否し、設定済みのファイルパスだけで
// 有効なペルソナとして扱わないことを確認する。
func TestLoadRejectsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persona.md")
	if err := os.WriteFile(path, []byte(" \n\t"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("Load() error = %v, want empty-file error", err)
	}
}

// 誤って巨大な文章を指定した場合に読み込み上限で拒否し、
// 起動時の無制限なメモリ使用と過大なMCP Instructions送信を防ぐことを確認する。
func TestLoadRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persona.md")
	content := strings.Repeat("a", maxInstructionsBytes+1)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "must be at most") {
		t.Fatalf("Load() error = %v, want size-limit error", err)
	}
}
