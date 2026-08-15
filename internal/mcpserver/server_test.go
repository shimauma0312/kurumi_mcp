package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"walnut_mcp/internal/discord"
)

type fakeSender struct {
	received discord.Embed
}

func (f *fakeSender) SendEmbed(_ context.Context, embed discord.Embed) (discord.Message, error) {
	f.received = embed
	return discord.Message{ID: "999", ChannelID: "123"}, nil
}

func TestSendDiscordEmbedTool(t *testing.T) {
	ctx := context.Background()
	sender := &fakeSender{}
	server := New(sender, "#5865F2")
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.1.0"}, nil)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: toolName,
		Arguments: map[string]any{
			"title":       " お知らせ ",
			"description": " 本文 ",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %#v", result.Content)
	}
	if sender.received.Title != "お知らせ" || sender.received.Description != "本文" {
		t.Fatalf("received embed = %#v", sender.received)
	}
	if sender.received.Color != "#5865F2" {
		t.Fatalf("color = %q, want default color", sender.received.Color)
	}
}
