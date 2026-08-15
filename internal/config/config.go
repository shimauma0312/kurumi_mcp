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
	defaultMCPAddr           = "127.0.0.1:8080"
)

type Config struct {
	DiscordBotToken   string
	DiscordChannelID  string
	DiscordAPIBaseURL string
	DiscordEmbedColor string
	MCPTransport      string
	MCPAddr           string
	MCPBearerToken    string
	HTTPTimeout       time.Duration
}

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
