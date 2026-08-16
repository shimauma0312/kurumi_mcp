package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	discordChannelIDPattern = regexp.MustCompile(`^[0-9]{17,20}$`)
	embedColorPattern       = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

// 実行時の設定。
type Config struct {
	DiscordBotToken          string
	DiscordChannelID         string
	DiscordAPIBaseURL        string
	DiscordEmbedColor        string
	DiscordEmbedThumbnailURL string

	MCPTransport     string
	MCPAddr          string
	MCPBearerToken   string
	MCPPersonaFile   string
	MCPMessageSuffix string

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
		MCPPersonaFile:           strings.TrimSpace(os.Getenv("MCP_PERSONA_FILE")),
		MCPMessageSuffix:         strings.TrimSpace(os.Getenv("MCP_MESSAGE_SUFFIX")),
		HTTPTimeout:              15 * time.Second,
	}

	// Discord操作とMCP起動に共通する必須項目を検証。
	var problems []error
	if cfg.DiscordBotToken == "" {
		problems = append(problems, errors.New("DISCORD_BOT_TOKEN is required"))
	}
	if cfg.DiscordChannelID == "" {
		problems = append(problems, errors.New("DISCORD_CHANNEL_ID is required"))
	} else if !discordChannelIDPattern.MatchString(cfg.DiscordChannelID) {
		problems = append(problems, errors.New("DISCORD_CHANNEL_ID must be a 17 to 20 digit Discord snowflake"))
	}
	if cfg.DiscordAPIBaseURL == "" {
		problems = append(problems, errors.New("DISCORD_API_BASE_URL is required"))
	} else if err := validateDiscordAPIBaseURL(cfg.DiscordAPIBaseURL); err != nil {
		problems = append(problems, err)
	}
	if cfg.DiscordEmbedColor == "" {
		problems = append(problems, errors.New("DISCORD_EMBED_COLOR is required"))
	} else if !embedColorPattern.MatchString(cfg.DiscordEmbedColor) {
		problems = append(problems, errors.New("DISCORD_EMBED_COLOR must use #RRGGBB format"))
	}
	if cfg.DiscordEmbedThumbnailURL != "" {
		if err := validateThumbnailURL(cfg.DiscordEmbedThumbnailURL); err != nil {
			problems = append(problems, err)
		}
	}
	if cfg.MCPPersonaFile == "" {
		problems = append(problems, errors.New("MCP_PERSONA_FILE is required"))
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
		} else if err := validateLoopbackAddress(cfg.MCPAddr); err != nil {
			problems = append(problems, err)
		}
		if cfg.MCPBearerToken == "" {
			problems = append(problems, errors.New("MCP_BEARER_TOKEN is required when MCP_TRANSPORT=http"))
		} else if len(cfg.MCPBearerToken) < 32 {
			problems = append(problems, errors.New("MCP_BEARER_TOKEN must be at least 32 characters"))
		}
	}

	// 不足項目を一度に修正できるよう全エラーを結合。
	return cfg, errors.Join(problems...)
}

// Discord APIの本番URLを検証。
func validateDiscordAPIBaseURL(raw string) error {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "discord.com" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || strings.TrimRight(parsed.EscapedPath(), "/") != "/api/v10" {
		return errors.New("DISCORD_API_BASE_URL must be https://discord.com/api/v10")
	}
	return nil
}

// Embed画像URLを検証。
func validateThumbnailURL(raw string) error {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("DISCORD_EMBED_THUMBNAIL_URL must use http or https without user information")
	}
	return nil
}

// ローカルHTTP待受アドレスを検証。
func validateLoopbackAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("MCP_ADDR must be a loopback host and port, such as 127.0.0.1:8765")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return errors.New("MCP_ADDR port must be between 1 and 65535")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("MCP_ADDR must use a loopback address")
	}
	return nil
}
