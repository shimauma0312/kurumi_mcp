package config

import (
	"strings"
	"testing"
)

// 明示した環境変数がConfigへ読み込まれることを検証。
// stdioではHTTP用の待受アドレスとBearer Tokenを省略でき、
// Secure MCP Tunnelから余分なネットワーク設定なしで起動できることも確認する。
func TestLoadReadsExplicitConfiguration(t *testing.T) {
	// 実際のDiscordやMCPサーバーへ接続しないテスト用設定。
	t.Setenv("DISCORD_BOT_TOKEN", "test-token")
	t.Setenv("DISCORD_CHANNEL_ID", "123")
	t.Setenv("DISCORD_API_BASE_URL", "https://discord.example/api")
	t.Setenv("DISCORD_EMBED_COLOR", "#123456")
	t.Setenv("DISCORD_EMBED_THUMBNAIL_URL", "https://cdn.example/walnut.png")
	t.Setenv("MCP_TRANSPORT", "stdio")
	t.Setenv("MCP_ADDR", "")
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
		{name: "MCPAddr", got: cfg.MCPAddr, want: ""},
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
	} {
		if !strings.Contains(err.Error(), name+" is required") {
			t.Errorf("Load() error does not mention %s: %v", name, err)
		}
	}
}

// HTTPトランスポートだけが待受アドレスとBearer Tokenを要求することを検証。
// Discord側の必須項目をすべて満たした状態でHTTP固有の2項目だけを空にし、
// stdioでは不要な設定がHTTP選択時には両方とも不足として報告されることを確認する。
func TestLoadRequiresHTTPConfiguration(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "test-token")
	t.Setenv("DISCORD_CHANNEL_ID", "123")
	t.Setenv("DISCORD_API_BASE_URL", "https://discord.example/api")
	t.Setenv("DISCORD_EMBED_COLOR", "#123456")
	t.Setenv("DISCORD_EMBED_THUMBNAIL_URL", "")
	t.Setenv("MCP_TRANSPORT", "http")
	t.Setenv("MCP_ADDR", "")
	t.Setenv("MCP_BEARER_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want HTTP-setting errors")
	}
	for _, message := range []string{
		"MCP_ADDR is required when MCP_TRANSPORT=http",
		"MCP_BEARER_TOKEN is required when MCP_TRANSPORT=http",
	} {
		if !strings.Contains(err.Error(), message) {
			t.Errorf("Load() error does not contain %q: %v", message, err)
		}
	}
}
