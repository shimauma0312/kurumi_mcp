package mcp

import (
	"context"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"walnut_mcp/internal/discord"
)

// Discordへ通信せず、MCPツールから渡されたEmbedを記録するテスト用送信先。
type fakeSender struct {
	received discord.Embed
}

func (f *fakeSender) SendEmbed(_ context.Context, embed discord.Embed) (discord.Message, error) {
	f.received = embed
	return discord.Message{ID: "999", ChannelID: "123"}, nil
}

// MCP SDKのインメモリ接続を使い、ツール登録から呼び出しまでを検証。
// send_discord_embedが正常終了し、タイトルと本文の前後空白が除去され、
// color省略時にサーバー設定の色がDiscord送信層へ渡ることを確認する。
func TestSendDiscordEmbedTool(t *testing.T) {
	ctx := context.Background()
	sender := &fakeSender{}
	server := NewServer(sender, "#5865F2")
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "0.1.0"}, nil)

	// ネットワークを使わず、実際のMCPセッションと同じ接続手順を再現。
	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
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

	// 色を指定せず、前後に空白を含む文字列でツールを呼び出す。
	result, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
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

	// MCP層で文字列が整形され、固定送信先の実装へ渡った内容を確認。
	if sender.received.Title != "お知らせ" || sender.received.Description != "本文" {
		t.Fatalf("received embed = %#v", sender.received)
	}
	if sender.received.Color != "#5865F2" {
		t.Fatalf("color = %q, want default color", sender.received.Color)
	}
}
