package mcp

import (
	"context"
	"strings"
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

	// 初期化結果にクルミの恒常ペルソナが含まれることを確認。
	// キャラクター名だけでなく、一人称、性格、投稿時の禁止事項、
	// 明示依頼時だけ送信する操作制約までクライアントへ渡ることを検証する。
	initializeResult := clientSession.InitializeResult()
	if initializeResult == nil {
		t.Fatal("initialize result = nil, want server instructions")
	}
	for _, phrase := range []string{
		"クルミ（ウォールナット）",
		"一人称は「ボク」",
		"冷静で理性的",
		"舞台裏を投稿文に含めない",
		"明示的に依頼した場合だけ",
	} {
		if !strings.Contains(initializeResult.Instructions, phrase) {
			t.Errorf("server instructions do not contain %q", phrase)
		}
	}

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
