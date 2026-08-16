package discord

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/shimauma0312/kurumi_mcp/internal/linkpreview"
)

type fakeLinkPreviewFetcher struct {
	metadata linkpreview.Metadata
	err      error
	calls    int
	lastURL  string
}

func (f *fakeLinkPreviewFetcher) Fetch(_ context.Context, pageURL string) (linkpreview.Metadata, error) {
	f.calls++
	f.lastURL = pageURL
	return f.metadata, f.err
}

// Discord APIへ送るHTTPリクエストと、成功レスポンスの変換を一通り検証。
// 確認対象はPOSTメソッド、固定チャンネルのURL、Bot認証ヘッダー、
// Embed本文・色・フッター・固定サムネイル・タイトルのリンク先、メンション無効化、
// 返却されたメッセージID。
func TestSendEmbed(t *testing.T) {
	var received createMessageRequest

	// Discord APIの代わりにリクエストを記録し、固定の成功レスポンスを返す。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/channels/123/messages" {
			t.Errorf("path = %s, want /channels/123/messages", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bot test-token" {
			t.Errorf("Authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"999","channel_id":"123"}`))
	}))
	defer server.Close()

	// テストサーバーをDiscord APIとしてクライアントからEmbedを送信。
	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "https://cdn.example/walnut.png")
	if err != nil {
		t.Fatal(err)
	}
	message, err := client.SendEmbed(context.Background(), Embed{
		Title:       "お知らせ",
		Description: "テスト本文",
		Color:       "#5865F2",
		Footer:      "walnut",
		ImageURL:    "https://news.example/images/announcement.png",
		LinkURL:     " https://news.example/articles/announcement ",
	})
	if err != nil {
		t.Fatal(err)
	}

	// DiscordのJSONレスポンスが公開用Messageへ正しく変換されることを確認。
	if message.ID != "999" || message.ChannelID != "123" {
		t.Fatalf("message = %#v", message)
	}

	// 入力したEmbedが1件だけ送られ、色が16進文字列から整数へ変換されることを確認。
	if len(received.Embeds) != 1 {
		t.Fatalf("embed count = %d, want 1", len(received.Embeds))
	}
	got := received.Embeds[0]
	if got.Title != "お知らせ" || got.Description != "テスト本文" || got.Color != 0x5865F2 {
		t.Errorf("embed = %#v", got)
	}
	if got.Footer == nil || got.Footer.Text != "walnut" {
		t.Errorf("footer = %#v", got.Footer)
	}
	if got.Thumbnail == nil || got.Thumbnail.URL != "https://cdn.example/walnut.png" {
		t.Errorf("thumbnail = %#v", got.Thumbnail)
	}
	if got.Image == nil || got.Image.URL != "https://news.example/images/announcement.png" {
		t.Errorf("image = %#v", got.Image)
	}
	// link_urlは通常本文へ露出させず、EmbedタイトルのURLへ格納する。
	// 前後空白を除去し、タイトルをクリックして出典へ移動できる形式を確認する。
	if got.URL != "https://news.example/articles/announcement" {
		t.Errorf("embed URL = %q, want trimmed link URL", got.URL)
	}

	// nilではなく空配列を送ることで、Discord側の既定動作に依存せず通知を無効化。
	if received.AllowedMentions.Parse == nil || len(received.AllowedMentions.Parse) != 0 {
		t.Errorf("allowed mentions must be an explicit empty list: %#v", received.AllowedMentions)
	}
}

// link_urlがありimage_urlが省略された投稿では、独立したlinkpreview取得処理から
// OGP画像URLを補完し、Discord Embedのimageへ設定することを検証する。
// MCP入力スキーマへ新しい項目を追加せず、既存の投稿フロー内で自動処理されることも確認する。
func TestSendEmbedUsesLinkPreviewImageWhenImageIsOmitted(t *testing.T) {
	var received createMessageRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"999","channel_id":"123"}`))
	}))
	defer server.Close()

	preview := &fakeLinkPreviewFetcher{metadata: linkpreview.Metadata{ImageURL: "https://cdn.example/ogp.png"}}
	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "", WithLinkPreview(preview))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendEmbed(context.Background(), Embed{
		Title:       "ニュース",
		Description: "本文",
		Color:       "#5865F2",
		LinkURL:     "https://news.example/article",
	})
	if err != nil {
		t.Fatal(err)
	}

	if preview.calls != 1 || preview.lastURL != "https://news.example/article" {
		t.Fatalf("preview calls = %d, URL = %q", preview.calls, preview.lastURL)
	}
	if len(received.Embeds) != 1 || received.Embeds[0].Image == nil || received.Embeds[0].Image.URL != "https://cdn.example/ogp.png" {
		t.Fatalf("Discord embeds = %#v", received.Embeds)
	}
}

// image_urlが明示された場合はAIが選んだ画像を優先し、リンク先ページを取得しないことを検証する。
// 不要な外部HTTP通信を避けるとともに、既存の明示指定動作がOGP自動補完で上書きされないことを保証する。
func TestSendEmbedPrefersExplicitImageOverLinkPreview(t *testing.T) {
	var received createMessageRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"999","channel_id":"123"}`))
	}))
	defer server.Close()

	preview := &fakeLinkPreviewFetcher{metadata: linkpreview.Metadata{ImageURL: "https://cdn.example/ogp.png"}}
	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "", WithLinkPreview(preview))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendEmbed(context.Background(), Embed{
		Title:       "ニュース",
		Description: "本文",
		Color:       "#5865F2",
		ImageURL:    "https://cdn.example/selected.png",
		LinkURL:     "https://news.example/article",
	})
	if err != nil {
		t.Fatal(err)
	}

	if preview.calls != 0 {
		t.Fatalf("preview calls = %d, want 0", preview.calls)
	}
	if received.Embeds[0].Image == nil || received.Embeds[0].Image.URL != "https://cdn.example/selected.png" {
		t.Fatalf("Discord image = %#v", received.Embeds[0].Image)
	}
}

// OGP取得先がエラーを返しても、リンクと本文を持つEmbed投稿は継続することを検証する。
// OGP未対応サイトや一時障害によってDiscordへの投稿全体が失敗せず、画像だけを省略する動作を固定する。
func TestSendEmbedContinuesWhenLinkPreviewFails(t *testing.T) {
	var received createMessageRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"999","channel_id":"123"}`))
	}))
	defer server.Close()

	preview := &fakeLinkPreviewFetcher{err: errors.New("preview unavailable")}
	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "", WithLinkPreview(preview))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendEmbed(context.Background(), Embed{
		Title:       "ニュース",
		Description: "本文",
		Color:       "#5865F2",
		LinkURL:     "https://news.example/article",
	})
	if err != nil {
		t.Fatal(err)
	}

	if preview.calls != 1 {
		t.Fatalf("preview calls = %d, want 1", preview.calls)
	}
	if received.Embeds[0].Image != nil {
		t.Fatalf("Discord image = %#v, want nil", received.Embeds[0].Image)
	}
}

// image_urlにはDiscordが直接取得できる絶対HTTP(S) URLだけを許可することを検証。
// ローカルファイル、独自スキーム、認証情報入りURLを送信前に拒否し、Bot Tokenとは無関係な
// 外部画像指定でも入力形式を曖昧にしないことを確認する。
func TestSendEmbedRejectsInvalidImageURLBeforeRequest(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, imageURL := range []string{
		"file:///tmp/news.png",
		"data:image/png;base64,AAAA",
		"https://user:password@example.com/news.png",
	} {
		_, err := client.SendEmbed(context.Background(), Embed{
			Description: "test",
			Color:       "#5865F2",
			ImageURL:    imageURL,
		})
		if err == nil || !strings.Contains(err.Error(), "image URL") {
			t.Errorf("image URL %q: error = %v, want validation error", imageURL, err)
		}
	}
	if requestCount != 0 {
		t.Fatalf("request count = %d, want 0", requestCount)
	}
}

// link_urlをEmbedタイトルへ安全に設定できる絶対HTTP(S) URLへ限定することを検証。
// ローカルファイル、独自スキーム、認証情報入りURLをAPI通信前に拒否し、
// 不正入力でも外部リクエストが発生しないことを確認する。
func TestSendEmbedRejectsInvalidLinkURLBeforeRequest(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, linkURL := range []string{
		"file:///tmp/news.html",
		"javascript:alert(1)",
		"https://user:password@example.com/news",
	} {
		_, err := client.SendEmbed(context.Background(), Embed{
			Description: "test",
			Color:       "#5865F2",
			LinkURL:     linkURL,
		})
		if err == nil || !strings.Contains(err.Error(), "link URL") {
			t.Errorf("link URL %q: error = %v, want validation error", linkURL, err)
		}
	}

	// DiscordではEmbed URLがタイトルに結び付くため、リンクだけを指定して
	// 画面上から出典へ移動できなくなる入力も送信前に拒否する。
	_, err = client.SendEmbed(context.Background(), Embed{
		Description: "test",
		Color:       "#5865F2",
		LinkURL:     "https://example.com/news",
	})
	if err == nil || !strings.Contains(err.Error(), "requires a title") {
		t.Errorf("missing title: error = %v, want link/title validation error", err)
	}
	if requestCount != 0 {
		t.Fatalf("request count = %d, want 0", requestCount)
	}
}

// 不正なEmbedをDiscordへ送る前に拒否することを検証。
// 空の本文と、各フィールドの個別上限内でも合計6000文字を超える入力を試し、
// どちらの場合もHTTPリクエストが一度も発生しないことを確認する。
func TestSendEmbedRejectsInvalidInputBeforeRequest(t *testing.T) {
	requestCount := 0
	// 呼び出された回数だけを記録する。正常ならこのサーバーには到達しない。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendEmbed(context.Background(), Embed{Description: "", Color: "#5865F2"})
	if err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("error = %v, want description validation error", err)
	}

	// 256 + 4096 + 1649 = 6001文字。各項目は個別上限内だが合計上限を超える。
	_, err = client.SendEmbed(context.Background(), Embed{
		Title:       strings.Repeat("a", 256),
		Description: strings.Repeat("b", 4096),
		Footer:      strings.Repeat("c", 1649),
		Color:       "#5865F2",
	})
	if err == nil || !strings.Contains(err.Error(), "combined embed text") {
		t.Fatalf("error = %v, want combined length validation error", err)
	}

	// 入力検証が通信より先に実行されたことをリクエスト数で確認。
	if requestCount != 0 {
		t.Fatalf("request count = %d, want 0", requestCount)
	}
}

// Discord APIがエラーを返した場合、成功扱いせずステータスを含む
// 診断可能なエラーとして呼び出し元へ返すことを検証。
func TestSendEmbedReportsDiscordAPIError(t *testing.T) {
	// Discordで権限が不足した状況を403レスポンスで再現。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Missing Permissions"}`, http.StatusForbidden)
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendEmbed(context.Background(), Embed{Description: "test", Color: "#5865F2"})
	if err == nil || !strings.Contains(err.Error(), "403 Forbidden") {
		t.Fatalf("error = %v, want Discord 403 error", err)
	}
}

// サムネイルにDiscordが取得できるhttpまたはhttps URLだけを許可することを検証。
// ローカルファイルや独自スキームを起動時に拒否し、Discord APIを呼び出してから
// エラーになる状態を防ぐ。
func TestNewClientRejectsInvalidThumbnailURL(t *testing.T) {
	_, err := NewClient(http.DefaultClient, "https://discord.example/api", "test-token", "123", "file:///tmp/walnut.png")
	if err == nil || !strings.Contains(err.Error(), "thumbnail URL") {
		t.Fatalf("error = %v, want thumbnail URL validation error", err)
	}
}

// 固定チャンネルから指定件数の履歴を取得し、MCPへ返す形式へ変換することを検証。
// Discord APIが返す新しい順の配列を会話向けの古い順へ並べ替え、表示名とBot判定、
// 通常本文、Bot投稿のEmbed本文・フッター・画像URL・リンク先URLを失わず保持することを確認する。
func TestReadRecentMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/channels/123/messages" {
			t.Errorf("path = %s, want /channels/123/messages", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != strconv.Itoa(MaxRecentMessages) {
			t.Errorf("limit = %q, want %d", got, MaxRecentMessages)
		}
		if got := r.Header.Get("Authorization"); got != "Bot test-token" {
			t.Errorf("Authorization = %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"id":"2",
				"author":{"id":"user-1","username":"shima","global_name":"シマ","bot":false},
				"content":"しゃべるな。でも面白く返せ。",
				"timestamp":"2026-08-16T12:01:00.000000+00:00",
				"embeds":[]
			},
			{
				"id":"1",
				"author":{"id":"bot-1","username":"walnut","global_name":null,"bot":true},
				"content":"",
				"timestamp":"2026-08-16T12:00:00.000000+00:00",
				"embeds":[{"title":"前のお知らせ","url":"https://news.example/articles/previous","description":"前の本文","footer":{"text":"walnut"},"image":{"url":"https://news.example/images/previous.png"}}]
			}
		]`))
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "")
	if err != nil {
		t.Fatal(err)
	}
	messages, err := client.ReadRecentMessages(context.Background(), MaxRecentMessages)
	if err != nil {
		t.Fatal(err)
	}

	// API応答の2件が古い投稿から順に並び、会話の流れを追えることを確認。
	if len(messages) != 2 || messages[0].ID != "1" || messages[1].ID != "2" {
		t.Fatalf("messages = %#v, want IDs 1 then 2", messages)
	}
	if !messages[0].AuthorBot || messages[0].AuthorName != "walnut" {
		t.Errorf("bot author = %#v", messages[0])
	}
	if len(messages[0].Embeds) != 1 || messages[0].Embeds[0].Description != "前の本文" || messages[0].Embeds[0].Footer != "walnut" {
		t.Errorf("bot embeds = %#v", messages[0].Embeds)
	}
	if messages[0].Embeds[0].ImageURL != "https://news.example/images/previous.png" {
		t.Errorf("bot image URL = %q", messages[0].Embeds[0].ImageURL)
	}
	if messages[0].Embeds[0].LinkURL != "https://news.example/articles/previous" {
		t.Errorf("bot link URL = %q", messages[0].Embeds[0].LinkURL)
	}
	if messages[1].AuthorBot || messages[1].AuthorName != "シマ" || !strings.Contains(messages[1].Content, "しゃべるな") {
		t.Errorf("user message = %#v", messages[1])
	}
}

// Discordのレート制限が短時間なら、同じ投稿を無制限に増やさず1回だけ再試行することを検証。
// 1回目はretry_after=0の429、2回目は成功を返し、最終結果と合計リクエスト数の両方を確認する。
func TestSendEmbedRetriesRateLimitOnce(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		if requestCount == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"retry_after":0}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"999","channel_id":"123"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "")
	if err != nil {
		t.Fatal(err)
	}
	message, err := client.SendEmbed(context.Background(), Embed{Description: "test", Color: "#5865F2"})
	if err != nil {
		t.Fatal(err)
	}
	if message.ID != "999" || requestCount != 2 {
		t.Fatalf("message = %#v, request count = %d, want successful second request", message, requestCount)
	}
}

// 成功ステータスでも異常に大きい本文を読み続けず、JSON変換前に上限超過として拒否することを検証。
// Discordまたは設定先が想定外の応答を返しても、プロセスのメモリ使用量をレスポンスサイズに比例させない。
func TestSendEmbedRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBodyLength+1)))
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendEmbed(context.Background(), Embed{Description: "test", Color: "#5865F2"})
	if err == nil || !strings.Contains(err.Error(), "exceeded size limit") {
		t.Fatalf("error = %v, want response size error", err)
	}
}

// Discordが2xxと壊れたJSONを返した場合に、空の成功結果へ変換せずデコード失敗を返すことを検証。
// HTTP成功とアプリケーション応答の妥当性を別々に判定し、不完全なmessage_idを成功扱いしない。
func TestSendEmbedRejectsMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":`))
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendEmbed(context.Background(), Embed{Description: "test", Color: "#5865F2"})
	if err == nil || !strings.Contains(err.Error(), "decode Discord response") {
		t.Fatalf("error = %v, want JSON decode error", err)
	}
}

// 呼び出し元が取り消したcontextをHTTPリクエストへ伝え、Discord通信を継続しないことを検証。
// 送信前に取り消したcontextでcontext.Canceledを返すため、MCPの停止やタイムアウトに追従できる。
func TestSendEmbedHonorsCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.SendEmbed(ctx, Embed{Description: "test", Color: "#5865F2"})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

// Discordの文字数がUTF-8バイト数ではなくUnicode文字数で検証されることを確認。
// 日本語4096文字は許可し、1文字増やした4097文字は拒否することで境界の数え方を固定する。
func TestValidateEmbedCountsUnicodeCharacters(t *testing.T) {
	if _, err := validateEmbed(Embed{Description: strings.Repeat("栗", maxDescriptionLength), Color: "#5865F2"}); err != nil {
		t.Fatalf("4096 Japanese characters: %v", err)
	}
	if _, err := validateEmbed(Embed{Description: strings.Repeat("栗", maxDescriptionLength+1), Color: "#5865F2"}); err == nil {
		t.Fatal("4097 Japanese characters: error = nil, want description length error")
	}
}

// Discord APIへ接続する前に取得件数を設定上限へ制限することを検証。
// MCP層の入力検証を回避されても、大量取得できない多層防御を期待する。
func TestReadRecentMessagesRejectsOutOfRangeLimit(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(server.Client(), server.URL, "test-token", "123", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{0, MaxRecentMessages + 1} {
		if _, err := client.ReadRecentMessages(context.Background(), limit); err == nil {
			t.Errorf("limit %d: error = nil, want validation error", limit)
		}
	}
	if requestCount != 0 {
		t.Fatalf("request count = %d, want 0", requestCount)
	}
}
