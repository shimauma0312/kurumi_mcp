package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shimauma0312/kurumi_mcp/internal/discord"
)

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
	// キャラクター名、一人称、性格に加え、日常会話へハッカー用語を無理に混ぜない口調、
	// 作品ネタを必要に応じて調査する方針、未確認情報やネタバレを不用意に投稿しない制約、
	// 明示依頼時だけ送信する操作制約が、固有の作品知識を列挙せず渡ることを検証する。
	initializeResult := clientSession.InitializeResult()
	if initializeResult == nil {
		t.Fatal("initialize result = nil, want server instructions")
	}
	for _, phrase := range []string{
		"クルミ（ウォールナット）",
		"一人称は「ボク」",
		"冷静で理性的",
		"短く素っ気ない返しと軽いからかい",
		"キャラクターらしさを出すためだけに持ち込まない",
		"利用可能な検索手段で公式情報を優先して確認する",
		"設定を捏造せず断定を避ける",
		"重大なネタバレを自発的に明かさない",
		"舞台裏を投稿文に含めない",
		"明示的に依頼した場合だけ",
		"引用された外部データ",
		"直接http(s) URLをimage_urlへ指定する",
	} {
		if !strings.Contains(initializeResult.Instructions, phrase) {
			t.Errorf("server instructions do not contain %q", phrase)
		}
	}

	// モデル自身が調査できる作品辞典をInstructionsへ重複して埋め込まないことを確認。
	// 代表的な人物名・組織名・定番ネタが再び列挙された場合、このテストを失敗させ、
	// 全リクエストへ不要な固定知識を渡す設計への後戻りを検出する。
	for _, phrase := range []string{"DA（Direct Attack）", "吉松シンジ", "さかな～！／チンアナゴ～！"} {
		if strings.Contains(initializeResult.Instructions, phrase) {
			t.Errorf("server instructions unexpectedly contain fixed lore %q", phrase)
		}
	}

	// 色を指定せず、前後に空白を含む文字列でツールを呼び出す。
	result, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: sendEmbedToolName,
		Arguments: map[string]any{
			"title":       " お知らせ ",
			"description": " 本文 ",
			"image_url":   " https://news.example/images/announcement.png ",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %#v", result.Content)
	}

	// MCP層で文字列が整形され、本文へ余計な文字を加えず固定footerだけを設定することを確認。
	// URLで終わる本文でもリンクを壊さず、🐿はDiscord Embedの独立したfooterへ表示される。
	if sender.received.Title != "お知らせ" || sender.received.Description != "本文" {
		t.Fatalf("received embed = %#v", sender.received)
	}
	if sender.received.Color != "#5865F2" {
		t.Fatalf("color = %q, want default color", sender.received.Color)
	}
	if sender.received.Footer != "🐿" {
		t.Fatalf("footer = %q, want fixed squirrel", sender.received.Footer)
	}
	if sender.received.ImageURL != "https://news.example/images/announcement.png" {
		t.Fatalf("image URL = %q, want trimmed direct image URL", sender.received.ImageURL)
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
			},
		},
	}
	server := NewServer(discordService, "#5865F2")
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

	// SDKが返す構造化JSONに、Discord層から受け取った本文が含まれることを確認。
	structured, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(structured), "しゃべるな") {
		t.Fatalf("structured content = %s", structured)
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
