package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shimauma0312/kurumi_mcp/internal/discord"
)

const testInstructions = "投稿方針:\n- 事実を変えず簡潔に書く。"

// Discordへ通信せず、MCPツールから渡されたEmbedを記録するテスト用送信先。
type fakeSender struct {
	received  discord.Embed
	messages  []discord.RecentMessage
	readLimit int
}

func (f *fakeSender) SendEmbed(_ context.Context, embed discord.Embed) (discord.Message, error) {
	f.received = embed
	return discord.Message{ID: "999", ChannelID: "123"}, nil
}

func (f *fakeSender) ReadRecentMessages(_ context.Context, limit int) ([]discord.RecentMessage, error) {
	f.readLimit = limit
	return f.messages, nil
}

// MCP SDKのインメモリ接続を使い、ツール登録から呼び出しまでを検証。
// send_discord_embedが正常終了し、タイトル・本文・各URLの前後空白が除去され、
// color省略時にサーバー設定の色がDiscord送信層へ渡ることを確認する。
func TestSendDiscordEmbedTool(t *testing.T) {
	ctx := context.Background()
	sender := &fakeSender{}
	server := NewServer(sender, "#5865F2", testInstructions, "◆")
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

	// NewServerへ注入した外部ペルソナが書き換えられず、
	// MCP初期化結果のInstructionsとしてクライアントへ渡ることを確認する。
	initializeResult := clientSession.InitializeResult()
	if initializeResult == nil {
		t.Fatal("initialize result = nil, want server instructions")
	}
	if initializeResult.Instructions != testInstructions {
		t.Fatalf("server instructions = %q, want injected instructions", initializeResult.Instructions)
	}

	// ChatGPTなどのMCPクライアントが取得するtools/listの入力スキーマへ、
	// 任意のimage_urlとlink_urlが実際に公開されていることを確認する。Goの入力構造体へ
	// フィールドを追加しただけでスキーマ生成から漏れた場合、サーバーが処理できても
	// ChatGPTなどのMCPクライアントには表示されないため失敗させる。
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sendToolSchema []byte
	var sendToolDescription string
	for _, tool := range toolsResult.Tools {
		if tool.Name == sendEmbedToolName {
			sendToolDescription = tool.Description
			sendToolSchema, err = json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if !strings.Contains(string(sendToolSchema), `"image_url"`) {
		t.Fatalf("send_discord_embed input schema = %s, want image_url", sendToolSchema)
	}
	if !strings.Contains(string(sendToolSchema), `"link_url"`) {
		t.Fatalf("send_discord_embed input schema = %s, want link_url", sendToolSchema)
	}
	// 入力項目を増やさず、image_url省略時のOGP自動取得がツール説明と
	// image_urlのJSON Schemaへ公開され、MCPクライアントから認識できることを確認する。
	if !strings.Contains(sendToolDescription, "OGP") || !strings.Contains(string(sendToolSchema), "OGP") {
		t.Fatalf("send_discord_embed description/schema does not expose OGP behavior: %q / %s", sendToolDescription, sendToolSchema)
	}

	// 色を指定せず、前後に空白を含む文字列でツールを呼び出す。
	result, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: sendEmbedToolName,
		Arguments: map[string]any{
			"title":       " お知らせ ",
			"description": " 本文 ",
			"image_url":   " https://news.example/images/announcement.png ",
			"link_url":    " https://news.example/article ",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %#v", result.Content)
	}

	// MCP層が本文の前後空白を除去し、設定済みの印を独立行へ補完することを確認する。
	// 出典URLは本文へ混ぜず、Discord Embedのタイトルへ渡す独立フィールドとして保持する。
	if sender.received.Title != "お知らせ" || sender.received.Description != "本文\n\n◆" {
		t.Fatalf("received embed = %#v", sender.received)
	}
	if sender.received.Color != "#5865F2" {
		t.Fatalf("color = %q, want default color", sender.received.Color)
	}
	// 大きい本文表示へ移した後も、小さいfooterへ同じ印を重複表示しないことを確認する。
	if sender.received.Footer != "" {
		t.Fatalf("footer = %q, want empty footer", sender.received.Footer)
	}
	if sender.received.ImageURL != "https://news.example/images/announcement.png" {
		t.Fatalf("image URL = %q, want trimmed direct image URL", sender.received.ImageURL)
	}
	if sender.received.LinkURL != "https://news.example/article" {
		t.Fatalf("link URL = %q, want trimmed source URL", sender.received.LinkURL)
	}
}

// 本文末尾の印の補完を単体で検証する。モデルが印を省略した場合は追加し、
// 本文直後または独立行へ既に置いた場合は、重複させず独立行へ正規化する。
// 印が未設定なら本文だけを返し、空本文は後段のDiscord入力検証で拒否できる状態を保つ。
func TestAppendMessageSuffix(t *testing.T) {
	tests := []struct {
		name        string
		description string
		suffix      string
		want        string
	}{
		{name: "印を補完", description: "本文", suffix: "◆", want: "本文\n\n◆"},
		{name: "URLの後ろへ独立して補完", description: "https://news.example/article", suffix: "◆", want: "https://news.example/article\n\n◆"},
		{name: "本文直後の印を独立行へ移動", description: "本文◆", suffix: "◆", want: "本文\n\n◆"},
		{name: "既存の独立行を維持", description: "本文\n\n◆", suffix: "◆", want: "本文\n\n◆"},
		{name: "印なし", description: " 本文 ", suffix: "", want: "本文"},
		{name: "空本文を維持", description: "  ", suffix: "◆", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := appendMessageSuffix(tt.description, tt.suffix); got != tt.want {
				t.Fatalf("appendMessageSuffix(%q, %q) = %q, want %q", tt.description, tt.suffix, got, tt.want)
			}
		})
	}
}

// MCP SDK経由でread_recent_messagesを呼び出し、Discord層へ渡す件数と
// クライアントへ返す構造化メッセージを検証する。limit省略時は設定上限となり、
// 通常本文とEmbedを含む取得結果がStructuredContentへ格納されることを確認する。
func TestReadRecentMessagesTool(t *testing.T) {
	ctx := context.Background()
	discordService := &fakeSender{
		messages: []discord.RecentMessage{
			{
				ID:         "123",
				AuthorName: "シマ",
				Content:    "しゃべるな。",
				Timestamp:  "2026-08-16T12:00:00.000000+00:00",
				Embeds: []discord.ReceivedEmbed{
					{Title: "出典付き", LinkURL: "https://news.example/article"},
				},
			},
		},
	}
	server := NewServer(discordService, "#5865F2", testInstructions, "")
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "0.1.0"}, nil)

	// ネットワークを使わずにMCPサーバーへ接続し、入力省略時の呼び出しを再現。
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

	result, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      readMessagesToolName,
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %#v", result.Content)
	}
	if discordService.readLimit != discord.MaxRecentMessages {
		t.Fatalf("read limit = %d, want %d", discordService.readLimit, discord.MaxRecentMessages)
	}

	// SDKが返す構造化JSONに、Discord層から受け取った本文とEmbedタイトルの
	// リンク先URLが含まれ、返信生成時に出典を再利用できることを確認する。
	structured, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(structured), "しゃべるな") {
		t.Fatalf("structured content = %s", structured)
	}
	if !strings.Contains(string(structured), "https://news.example/article") {
		t.Fatalf("structured content = %s, want link URL", structured)
	}

	// 上限を超える要求はDiscord層へ到達せず、MCPツールエラーになることを確認。
	invalidResult, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: readMessagesToolName,
		Arguments: map[string]any{
			"limit": discord.MaxRecentMessages + 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !invalidResult.IsError {
		t.Fatalf("over-limit request: IsError = false, want true")
	}
	if discordService.readLimit != discord.MaxRecentMessages {
		t.Fatalf("invalid request reached Discord layer: read limit = %d", discordService.readLimit)
	}
}
