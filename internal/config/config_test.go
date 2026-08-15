package config

import (
	"strings"
	"testing"
)

func TestLoadReadsExplicitConfiguration(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "test-token")
	t.Setenv("DISCORD_CHANNEL_ID", "123")
	t.Setenv("DISCORD_API_BASE_URL", "https://discord.example/api")
	t.Setenv("DISCORD_EMBED_COLOR", "#123456")
	t.Setenv("MCP_TRANSPORT", "stdio")
	t.Setenv("MCP_ADDR", "127.0.0.1:19000")
	t.Setenv("MCP_BEARER_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MCPAddr != "127.0.0.1:19000" {
		t.Fatalf("MCPAddr = %q, want %q", cfg.MCPAddr, "127.0.0.1:19000")
	}
}

func TestLoadRequiresExplicitConfiguration(t *testing.T) {
	for _, name := range []string{
		"DISCORD_BOT_TOKEN",
		"DISCORD_CHANNEL_ID",
		"DISCORD_API_BASE_URL",
		"DISCORD_EMBED_COLOR",
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
