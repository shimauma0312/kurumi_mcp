package discord

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendEmbed(t *testing.T) {
	var received createMessageRequest
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

	if message.ID != "999" || message.ChannelID != "123" {
		t.Fatalf("message = %#v", message)
	}
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
	if received.AllowedMentions.Parse == nil || len(received.AllowedMentions.Parse) != 0 {
		t.Errorf("allowed mentions must be an explicit empty list: %#v", received.AllowedMentions)
	}
}

func TestSendEmbedRejectsInvalidInputBeforeRequest(t *testing.T) {
	requestCount := 0
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
	if requestCount != 0 {
		t.Fatalf("request count = %d, want 0", requestCount)
	}
}

func TestSendEmbedReportsDiscordAPIError(t *testing.T) {
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
