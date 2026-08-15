package config

import (
	"strings"
	"testing"
)

// 明示した環境変数がConfigへ読み込まれることを検証。
// stdioではBearer Tokenを省略でき、MCP_ADDRにはコード内の既定値ではなく
// 環境変数で指定した値が使われることも確認する。
func TestLoadReadsExplicitConfiguration(t *testing.T) {
	// 実際のDiscordやMCPサーバーへ接続しないテスト用設定。
	t.Setenv("DISCORD_BOT_TOKEN", "test-token")
	t.Setenv("DISCORD_CHANNEL_ID", "123")
	t.Setenv("DISCORD_API_BASE_URL", "https://discord.example/api")
	t.Setenv("DISCORD_EMBED_COLOR", "#123456")
	t.Setenv("DISCORD_EMBED_THUMBNAIL_URL", "https://cdn.example/walnut.png")
	t.Setenv("MCP_TRANSPORT", "stdio")
	t.Setenv("MCP_ADDR", "127.0.0.1:19000")
	t.Setenv("MCP_BEARER_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	// 各環境変数が対応するConfigフィールドへ入り、別の値で補完されないことを確認。
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "DiscordBotToken", got: cfg.DiscordBotToken, want: "test-token"},
		{name: "DiscordChannelID", got: cfg.DiscordChannelID, want: "123"},
		{name: "DiscordAPIBaseURL", got: cfg.DiscordAPIBaseURL, want: "https://discord.example/api"},
		{name: "DiscordEmbedColor", got: cfg.DiscordEmbedColor, want: "#123456"},
		{name: "DiscordEmbedThumbnailURL", got: cfg.DiscordEmbedThumbnailURL, want: "https://cdn.example/walnut.png"},
		{name: "MCPTransport", got: cfg.MCPTransport, want: "stdio"},
		{name: "MCPAddr", got: cfg.MCPAddr, want: "127.0.0.1:19000"},
		{name: "MCPBearerToken", got: cfg.MCPBearerToken, want: ""},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

// 必須設定が空の場合、Loadが設定不足をまとめて報告することを検証。
// 最初のエラーだけで終了せず、各キーのエラーを一度に返すことで、
// 利用者が不足項目をまとめて修正できる状態を期待する。
// MCP_BEARER_TOKENはHTTPトランスポート選択時だけ必須なので対象外。
func TestLoadRequiresExplicitConfiguration(t *testing.T) {
	// 開発PCに設定済みの環境変数がテスト結果へ影響しないよう全項目を空にする。
	for _, name := range []string{
		"DISCORD_BOT_TOKEN",
		"DISCORD_CHANNEL_ID",
		"DISCORD_API_BASE_URL",
		"DISCORD_EMBED_COLOR",
		"DISCORD_EMBED_THUMBNAIL_URL",
		"MCP_TRANSPORT",
		"MCP_ADDR",
		"MCP_BEARER_TOKEN",
	} {
		t.Setenv(name, "")
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want required-setting errors")
	}

	// 必須キーごとのエラーが結合されたエラー内にすべて含まれることを確認。
	for _, name := range []string{
		"DISCORD_BOT_TOKEN",
		"DISCORD_CHANNEL_ID",
		"DISCORD_API_BASE_URL",
		"DISCORD_EMBED_COLOR",
		"MCP_TRANSPORT",
		"MCP_ADDR",
	} {
		if !strings.Contains(err.Error(), name+" is required") {
			t.Errorf("Load() error does not mention %s: %v", name, err)
		}
	}
}
