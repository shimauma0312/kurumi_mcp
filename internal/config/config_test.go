package config

import "testing"

func TestLoadUsesDefaultMCPAddress(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "test-token")
	t.Setenv("DISCORD_CHANNEL_ID", "123")
	t.Setenv("MCP_TRANSPORT", "stdio")
	t.Setenv("MCP_ADDR", "")
	t.Setenv("MCP_BEARER_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MCPAddr != "127.0.0.1:18080" {
		t.Fatalf("MCPAddr = %q, want %q", cfg.MCPAddr, "127.0.0.1:18080")
	}
}
