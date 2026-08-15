package mcp

import (
	"context"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"walnut_mcp/internal/discord"
)

const (
	serverName    = "walnut-discord"
	serverVersion = "0.1.0"
	toolName      = "send_discord_embed"
)

// 固定チャンネルへのEmbed送信。
type EmbedSender interface {
	// 固定チャンネルへ投稿。
	SendEmbed(context.Context, discord.Embed) (discord.Message, error)
}

// MCPツールの入力。チャンネルIDは含めない。
type SendEmbedInput struct {
	Description string `json:"description" jsonschema:"Discord Embedの本文。1文字以上4096文字以下。"`
	Title       string `json:"title,omitempty" jsonschema:"任意のタイトル。最大256文字。"`
	Color       string `json:"color,omitempty" jsonschema:"任意の色。#RRGGBB形式。省略時はサーバー設定値。"`
	Footer      string `json:"footer,omitempty" jsonschema:"任意のフッター。最大2048文字。"`
}

// MCPツールの送信結果。
type SendEmbedOutput struct {
	Success   bool   `json:"success" jsonschema:"送信に成功したか"`
	MessageID string `json:"message_id" jsonschema:"Discordが発行したメッセージID"`
	ChannelID string `json:"channel_id" jsonschema:"サーバーで固定された送信先チャンネルID"`
}

type service struct {
	sender       EmbedSender
	defaultColor string
}

// Embed送信専用MCPサーバーを生成。
func NewServer(sender EmbedSender, defaultColor string) *mcpsdk.Server {
	svc := &service{sender: sender, defaultColor: defaultColor}
	server := mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: serverName, Version: serverVersion},
		&mcpsdk.ServerOptions{
			Instructions: "Discordへの投稿専用です。ユーザーが明示的に投稿を依頼した場合だけ send_discord_embed を呼び出してください。送信先チャンネルはサーバー側で固定されています。",
		},
	)

	destructive := false
	openWorld := true
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        toolName,
		Title:       "DiscordにEmbedを送信",
		Description: "指定された文章をEmbedとして、サーバーに設定済みの単一Discordチャンネルへ送信します。チャンネルは選択できません。実際に外部投稿する書き込み操作です。",
		Annotations: &mcpsdk.ToolAnnotations{
			Title: "DiscordにEmbedを送信",
			// 追加のみ、非冪等の外部書き込み。
			ReadOnlyHint:    false,
			DestructiveHint: &destructive,
			IdempotentHint:  false,
			OpenWorldHint:   &openWorld,
		},
	}, svc.sendEmbed)

	return server
}

func (s *service) sendEmbed(ctx context.Context, _ *mcpsdk.CallToolRequest, input SendEmbedInput) (*mcpsdk.CallToolResult, SendEmbedOutput, error) {
	color := strings.TrimSpace(input.Color)
	if color == "" {
		// 色の省略時はサーバー設定値を使用。
		color = s.defaultColor
	}

	message, err := s.sender.SendEmbed(ctx, discord.Embed{
		Title:       strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description),
		Color:       color,
		Footer:      strings.TrimSpace(input.Footer),
	})
	if err != nil {
		return nil, SendEmbedOutput{}, fmt.Errorf("send Discord embed: %w", err)
	}

	output := SendEmbedOutput{
		Success:   true,
		MessageID: message.ID,
		ChannelID: message.ChannelID,
	}
	result := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: fmt.Sprintf("Discordへの送信に成功しました（message_id: %s）", message.ID)},
		},
	}
	return result, output, nil
}
