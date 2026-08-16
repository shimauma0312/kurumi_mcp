package discord

import (
	"errors"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

type fakeGatewaySession struct {
	openCalled  bool
	closeCalled bool
	openErr     error
	closeErr    error
}

func (f *fakeGatewaySession) Open() error {
	f.openCalled = true
	return f.openErr
}

func (f *fakeGatewaySession) Close() error {
	f.closeCalled = true
	return f.closeErr
}

// オンライン表示専用セッションがBot認証を使い、イベントを一切購読せず、
// 初期PresenceをonlineとしてIdentifyする設定になっていることを検証する。
// Message Content Intentなどを誤って追加し、不要なチャンネルイベントを受信する変更を防ぐ。
func TestNewGatewayUsesOnlinePresenceWithoutIntents(t *testing.T) {
	gateway, err := NewGateway(" test-token ")
	if err != nil {
		t.Fatal(err)
	}

	session, ok := gateway.session.(*discordgo.Session)
	if !ok {
		t.Fatalf("session type = %T, want *discordgo.Session", gateway.session)
	}
	if session.Token != "Bot test-token" {
		t.Fatalf("token = %q, want trimmed Bot token", session.Token)
	}
	if session.Identify.Intents != discordgo.IntentsNone {
		t.Fatalf("intents = %d, want none", session.Identify.Intents)
	}
	if session.Identify.Presence.Status != string(discordgo.StatusOnline) {
		t.Fatalf("status = %q, want online", session.Identify.Presence.Status)
	}
	if session.StateEnabled {
		t.Fatal("state cache is enabled, want disabled for presence-only connection")
	}
	if !session.ShouldReconnectOnError {
		t.Fatal("gateway reconnect is disabled")
	}
}

// 空のBot TokenではGatewayクライアントを生成せず、起動前に設定エラーとして返すことを検証する。
func TestNewGatewayRejectsEmptyToken(t *testing.T) {
	_, err := NewGateway("  ")
	if err == nil || !strings.Contains(err.Error(), "bot token") {
		t.Fatalf("error = %v, want bot token validation error", err)
	}
}

// Gatewayラッパーが接続開始と終了を実セッションへ委譲し、それぞれの失敗を
// 呼び出し元へ返すことを検証する。起動失敗を握りつぶしてMCPだけ起動したり、
// 終了失敗を正常扱いしたりする状態を防ぐ。
func TestGatewayPropagatesLifecycleErrors(t *testing.T) {
	openErr := errors.New("open failed")
	closeErr := errors.New("close failed")
	session := &fakeGatewaySession{openErr: openErr, closeErr: closeErr}
	gateway := &Gateway{session: session}

	if err := gateway.Open(); !errors.Is(err, openErr) {
		t.Fatalf("Open() error = %v, want %v", err, openErr)
	}
	if !session.openCalled {
		t.Fatal("Open() did not call the underlying session")
	}
	if err := gateway.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close() error = %v, want %v", err, closeErr)
	}
	if !session.closeCalled {
		t.Fatal("Close() did not call the underlying session")
	}
}
