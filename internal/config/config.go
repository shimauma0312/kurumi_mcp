package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// 実行時の設定。
type Config struct {
	DiscordBotToken          string
	DiscordChannelID         string
	DiscordAPIBaseURL        string
	DiscordEmbedColor        string
	DiscordEmbedThumbnailURL string

	MCPTransport   string
	MCPAddr        string
	MCPBearerToken string

	// Discord APIのタイムアウト。
	HTTPTimeout time.Duration
}

// 環境変数の読み込みと一括検証。
func Load() (Config, error) {
	// 環境変数をConfigへ集約。
	cfg := Config{
		DiscordBotToken:          strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		DiscordChannelID:         strings.TrimSpace(os.Getenv("DISCORD_CHANNEL_ID")),
		DiscordAPIBaseURL:        strings.TrimSpace(os.Getenv("DISCORD_API_BASE_URL")),
		DiscordEmbedColor:        strings.TrimSpace(os.Getenv("DISCORD_EMBED_COLOR")),
		DiscordEmbedThumbnailURL: strings.TrimSpace(os.Getenv("DISCORD_EMBED_THUMBNAIL_URL")),
		MCPTransport:             strings.ToLower(strings.TrimSpace(os.Getenv("MCP_TRANSPORT"))),
		MCPAddr:                  strings.TrimSpace(os.Getenv("MCP_ADDR")),
		MCPBearerToken:           strings.TrimSpace(os.Getenv("MCP_BEARER_TOKEN")),
		HTTPTimeout:              15 * time.Second,
	}

	// Discord操作とMCP起動に共通する必須項目を検証。
	var problems []error
	if cfg.DiscordBotToken == "" {
		problems = append(problems, errors.New("DISCORD_BOT_TOKEN is required"))
	}
	if cfg.DiscordChannelID == "" {
		problems = append(problems, errors.New("DISCORD_CHANNEL_ID is required"))
	}
	if cfg.DiscordAPIBaseURL == "" {
		problems = append(problems, errors.New("DISCORD_API_BASE_URL is required"))
	}
	if cfg.DiscordEmbedColor == "" {
		problems = append(problems, errors.New("DISCORD_EMBED_COLOR is required"))
	}

	// MCPトランスポート固有の設定を検証。
	if cfg.MCPTransport == "" {
		problems = append(problems, errors.New("MCP_TRANSPORT is required"))
	} else if cfg.MCPTransport != "stdio" && cfg.MCPTransport != "http" {
		problems = append(problems, fmt.Errorf("MCP_TRANSPORT must be stdio or http, got %q", cfg.MCPTransport))
	}
	if cfg.MCPTransport == "http" {
		if cfg.MCPAddr == "" {
			problems = append(problems, errors.New("MCP_ADDR is required when MCP_TRANSPORT=http"))
		}
		if cfg.MCPBearerToken == "" {
			// HTTP公開時の最低限の認証。
			problems = append(problems, errors.New("MCP_BEARER_TOKEN is required when MCP_TRANSPORT=http"))
		}
	}

	// 不足項目を一度に修正できるよう全エラーを結合。
	return cfg, errors.Join(problems...)
}
