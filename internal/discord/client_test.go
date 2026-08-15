package discord

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Discord APIへ送るHTTPリクエストと、成功レスポンスの変換を一通り検証。
// 確認対象はPOSTメソッド、固定チャンネルのURL、Bot認証ヘッダー、
// Embed本文・色・フッター、メンション無効化、返却されたメッセージID。
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
	client, err := NewClient(server.Client(), server.URL, "test-token", "123")
	if err != nil {
		t.Fatal(err)
	}
	message, err := client.SendEmbed(context.Background(), Embed{
		Title:       "お知らせ",
		Description: "テスト本文",
		Color:       "#5865F2",
		Footer:      "walnut",
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

	// nilではなく空配列を送ることで、Discord側の既定動作に依存せず通知を無効化。
	if received.AllowedMentions.Parse == nil || len(received.AllowedMentions.Parse) != 0 {
		t.Errorf("allowed mentions must be an explicit empty list: %#v", received.AllowedMentions)
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

	client, err := NewClient(server.Client(), server.URL, "test-token", "123")
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

	client, err := NewClient(server.Client(), server.URL, "test-token", "123")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendEmbed(context.Background(), Embed{Description: "test", Color: "#5865F2"})
	if err == nil || !strings.Contains(err.Error(), "403 Forbidden") {
		t.Fatalf("error = %v, want Discord 403 error", err)
	}
}
