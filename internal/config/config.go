// Package config は、walnut-mcpの環境変数ベースの設定を読み込み、検証する。
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultDiscordAPIBaseURL = "https://discord.com/api/v10"
	defaultEmbedColor        = "#5865F2"
	defaultMCPAddr           = "127.0.0.1:18080"
)

// Config は、実行時に必要なすべての設定を保持する。
type Config struct {
	// Discord関連の値はサーバー側だけで管理する。
	// これにより、MCP呼び出し元による認証情報の差し替えや送信先変更を防ぐ。
	DiscordBotToken   string
	DiscordChannelID  string
	DiscordAPIBaseURL string
	DiscordEmbedColor string

	// MCPTransportでは、子プロセス接続（stdio）またはネットワーク接続（HTTP）を選択する。
	// HTTPを選択した場合はMCPBearerTokenも必須となる。
	MCPTransport   string
	MCPAddr        string
	MCPBearerToken string

	// HTTPTimeoutは、Discordへの通信停止によってツール呼び出しが
	// 無期限に占有されることを防ぐ。
	HTTPTimeout time.Duration
}

// Load はプロセスの環境変数から設定を読み込む。
// 設定ミスを一度に修正できるよう、検証エラーはまとめて返す。
func Load() (Config, error) {
	cfg := Config{
		DiscordBotToken:   strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		DiscordChannelID:  strings.TrimSpace(os.Getenv("DISCORD_CHANNEL_ID")),
		DiscordAPIBaseURL: envOrDefault("DISCORD_API_BASE_URL", defaultDiscordAPIBaseURL),
		DiscordEmbedColor: envOrDefault("DISCORD_EMBED_COLOR", defaultEmbedColor),
		MCPTransport:      strings.ToLower(envOrDefault("MCP_TRANSPORT", "stdio")),
		MCPAddr:           envOrDefault("MCP_ADDR", defaultMCPAddr),
		MCPBearerToken:    strings.TrimSpace(os.Getenv("MCP_BEARER_TOKEN")),
		HTTPTimeout:       15 * time.Second,
	}

	var problems []error
	if cfg.DiscordBotToken == "" {
		problems = append(problems, errors.New("DISCORD_BOT_TOKEN is required"))
	}
	if cfg.DiscordChannelID == "" {
		problems = append(problems, errors.New("DISCORD_CHANNEL_ID is required"))
	}
	if cfg.MCPTransport != "stdio" && cfg.MCPTransport != "http" {
		problems = append(problems, fmt.Errorf("MCP_TRANSPORT must be stdio or http, got %q", cfg.MCPTransport))
	}
	if cfg.MCPTransport == "http" && cfg.MCPBearerToken == "" {
		// 書き込み可能なHTTPトランスポートを、少なくとも本MVPで提供する
		// 単一ユーザー認証なしで公開しない。
		problems = append(problems, errors.New("MCP_BEARER_TOKEN is required when MCP_TRANSPORT=http"))
	}

	return cfg, errors.Join(problems...)
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
