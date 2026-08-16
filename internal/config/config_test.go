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
	t.Setenv("DISCORD_CHANNEL_ID", "123456789012345678")
	t.Setenv("DISCORD_API_BASE_URL", "https://discord.com/api/v10")
	t.Setenv("DISCORD_EMBED_COLOR", "#123456")
	t.Setenv("DISCORD_EMBED_THUMBNAIL_URL", "https://cdn.example/walnut.png")
	t.Setenv("MCP_TRANSPORT", "stdio")
	t.Setenv("MCP_ADDR", "")
	t.Setenv("MCP_BEARER_TOKEN", "")
	t.Setenv("MCP_PERSONA_FILE", "persona.md")
	t.Setenv("MCP_MESSAGE_SUFFIX", "◆")

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
		{name: "DiscordChannelID", got: cfg.DiscordChannelID, want: "123456789012345678"},
		{name: "DiscordAPIBaseURL", got: cfg.DiscordAPIBaseURL, want: "https://discord.com/api/v10"},
		{name: "DiscordEmbedColor", got: cfg.DiscordEmbedColor, want: "#123456"},
		{name: "DiscordEmbedThumbnailURL", got: cfg.DiscordEmbedThumbnailURL, want: "https://cdn.example/walnut.png"},
		{name: "MCPTransport", got: cfg.MCPTransport, want: "stdio"},
		{name: "MCPAddr", got: cfg.MCPAddr, want: ""},
		{name: "MCPBearerToken", got: cfg.MCPBearerToken, want: ""},
		{name: "MCPPersonaFile", got: cfg.MCPPersonaFile, want: "persona.md"},
		{name: "MCPMessageSuffix", got: cfg.MCPMessageSuffix, want: "◆"},
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
		"MCP_PERSONA_FILE",
		"MCP_MESSAGE_SUFFIX",
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
		"MCP_PERSONA_FILE",
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
	t.Setenv("DISCORD_CHANNEL_ID", "123456789012345678")
	t.Setenv("DISCORD_API_BASE_URL", "https://discord.com/api/v10")
	t.Setenv("DISCORD_EMBED_COLOR", "#123456")
	t.Setenv("DISCORD_EMBED_THUMBNAIL_URL", "")
	t.Setenv("MCP_TRANSPORT", "http")
	t.Setenv("MCP_ADDR", "")
	t.Setenv("MCP_BEARER_TOKEN", "")
	t.Setenv("MCP_PERSONA_FILE", "persona.md")
	t.Setenv("MCP_MESSAGE_SUFFIX", "")

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

// Discord設定の形式検証で、誤送信や認証情報の漏えいにつながる値を起動前に拒否できることを検証。
// API URLはBot Tokenの送信先になるため公式HTTPSエンドポイントだけを許可し、チャンネルID、色、
// サムネイルURLもDiscordへ最初に送信するまで異常が隠れないよう、Loadの段階でまとめて検出する。
func TestLoadRejectsInvalidDiscordConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     string
		wantError string
	}{
		{name: "short channel ID", key: "DISCORD_CHANNEL_ID", value: "123", wantError: "17 to 20 digit"},
		{name: "non-numeric channel ID", key: "DISCORD_CHANNEL_ID", value: "12345678901234567x", wantError: "17 to 20 digit"},
		{name: "plain HTTP API", key: "DISCORD_API_BASE_URL", value: "http://discord.com/api/v10", wantError: "https://discord.com/api/v10"},
		{name: "third-party API host", key: "DISCORD_API_BASE_URL", value: "https://discord.example/api/v10", wantError: "https://discord.com/api/v10"},
		{name: "wrong API version", key: "DISCORD_API_BASE_URL", value: "https://discord.com/api/v9", wantError: "https://discord.com/api/v10"},
		{name: "invalid embed color", key: "DISCORD_EMBED_COLOR", value: "5865F2", wantError: "#RRGGBB"},
		{name: "thumbnail credentials", key: "DISCORD_EMBED_THUMBNAIL_URL", value: "https://user:pass@example.com/image.png", wantError: "without user information"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 各ケースで変更しない項目はすべて有効値に固定し、対象項目だけが失敗理由になる状態を作る。
			setValidEnvironment(t, "stdio")
			t.Setenv(tt.key, tt.value)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Load() error = %v, want error containing %q", err, tt.wantError)
			}
		})
	}
}

// HTTPトランスポートがローカルプロセス間の利用に限定され、外部公開アドレスや弱いTokenを拒否することを検証。
// IPv4、IPv6、localhostは許可し、全インターフェース待受、ポート不備、短いTokenはそれぞれ拒否する。
func TestLoadValidatesHTTPBoundary(t *testing.T) {
	tests := []struct {
		name      string
		addr      string
		token     string
		wantError string
	}{
		{name: "IPv4 loopback", addr: "127.0.0.1:8765", token: strings.Repeat("a", 32)},
		{name: "IPv6 loopback", addr: "[::1]:8765", token: strings.Repeat("b", 32)},
		{name: "localhost", addr: "localhost:8765", token: strings.Repeat("c", 32)},
		{name: "all interfaces", addr: "0.0.0.0:8765", token: strings.Repeat("d", 32), wantError: "loopback address"},
		{name: "missing port", addr: "127.0.0.1", token: strings.Repeat("e", 32), wantError: "loopback host and port"},
		{name: "invalid port", addr: "127.0.0.1:70000", token: strings.Repeat("f", 32), wantError: "between 1 and 65535"},
		{name: "short token", addr: "127.0.0.1:8765", token: "short-token", wantError: "at least 32 characters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// HTTP固有値以外は有効値で揃え、待受境界とToken強度だけを独立して確認する。
			setValidEnvironment(t, "http")
			t.Setenv("MCP_ADDR", tt.addr)
			t.Setenv("MCP_BEARER_TOKEN", tt.token)

			_, err := Load()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Load() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Load() error = %v, want error containing %q", err, tt.wantError)
			}
		})
	}
}

// 形式検証テストで共有する有効な環境変数一式。
// 各テストはこの状態から検証対象だけを上書きするため、失敗原因を一項目へ限定できる。
func setValidEnvironment(t *testing.T, transport string) {
	t.Helper()
	t.Setenv("DISCORD_BOT_TOKEN", "test-token")
	t.Setenv("DISCORD_CHANNEL_ID", "123456789012345678")
	t.Setenv("DISCORD_API_BASE_URL", "https://discord.com/api/v10")
	t.Setenv("DISCORD_EMBED_COLOR", "#5865F2")
	t.Setenv("DISCORD_EMBED_THUMBNAIL_URL", "")
	t.Setenv("MCP_TRANSPORT", transport)
	t.Setenv("MCP_ADDR", "")
	t.Setenv("MCP_BEARER_TOKEN", "")
	t.Setenv("MCP_PERSONA_FILE", "persona.md")
	t.Setenv("MCP_MESSAGE_SUFFIX", "")
}
