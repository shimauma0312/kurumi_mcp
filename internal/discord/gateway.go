package discord

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type gatewaySession interface {
	Open() error
	Close() error
}

// Discord上のオンライン表示を維持するGateway接続。
type Gateway struct {
	session gatewaySession
}

// イベントを購読しないGateway接続を生成。
func NewGateway(botToken string) (*Gateway, error) {
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return nil, errors.New("bot token is required")
	}

	session, err := discordgo.New("Bot " + botToken)
	if err != nil {
		return nil, fmt.Errorf("create Discord Gateway session: %w", err)
	}
	// オンライン表示だけを目的とし、メッセージなどのイベントは受信しない。
	session.Identify.Intents = discordgo.IntentsNone
	session.Identify.Presence.Status = string(discordgo.StatusOnline)
	session.StateEnabled = false
	session.ShouldReconnectOnError = true

	return &Gateway{session: session}, nil
}

// Gatewayへ接続し、READYの受信まで待機。
func (g *Gateway) Open() error {
	if err := g.session.Open(); err != nil {
		return fmt.Errorf("open Discord Gateway: %w", err)
	}
	return nil
}

// heartbeatとWebSocketを停止。
func (g *Gateway) Close() error {
	if err := g.session.Close(); err != nil {
		return fmt.Errorf("close Discord Gateway: %w", err)
	}
	return nil
}
