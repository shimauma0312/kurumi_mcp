// Package mcpserver は、Discord送信機能をModel Context Protocol経由で公開する。
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"walnut_mcp/internal/discord"
)

const (
	serverName    = "walnut-discord"
	serverVersion = "0.1.0"
	toolName      = "send_discord_embed"
)

// EmbedSender は、固定送信先へのDiscord操作をMCP層とテストから扱える形に抽象化する。
type EmbedSender interface {
	// SendEmbedは実装側にあらかじめ固定された送信先へ投稿する。
	// 呼び出し元からDiscordチャンネルIDを指定することはできない。
	SendEmbed(context.Context, discord.Embed) (discord.Message, error)
}

// SendEmbedInput は、モデルから見える入力スキーマ全体を表す。
// 送信先を固定するため、意図的にチャンネルIDを含めていない。
type SendEmbedInput struct {
	Description string `json:"description" jsonschema:"Discord Embedの本文。1文字以上4096文字以下。"`
	Title       string `json:"title,omitempty" jsonschema:"任意のタイトル。最大256文字。"`
	Color       string `json:"color,omitempty" jsonschema:"任意の色。#RRGGBB形式。省略時はサーバー設定値。"`
	Footer      string `json:"footer,omitempty" jsonschema:"任意のフッター。最大2048文字。"`
}

// SendEmbedOutput は、認証情報や投稿本文を繰り返さずに外部書き込み結果を返す。
type SendEmbedOutput struct {
	Success   bool   `json:"success" jsonschema:"送信に成功したか"`
	MessageID string `json:"message_id" jsonschema:"Discordが発行したメッセージID"`
	ChannelID string `json:"channel_id" jsonschema:"サーバーで固定された送信先チャンネルID"`
}

type service struct {
	sender       EmbedSender
	defaultColor string
}

// New は、Discord Embed送信ツールだけを持つMCPサーバーを生成する。
func New(sender EmbedSender, defaultColor string) *mcp.Server {
	svc := &service{sender: sender, defaultColor: defaultColor}
	server := mcp.NewServer(
		&mcp.Implementation{Name: serverName, Version: serverVersion},
		&mcp.ServerOptions{
			Instructions: "Discordへの投稿専用です。ユーザーが明示的に投稿を依頼した場合だけ send_discord_embed を呼び出してください。送信先チャンネルはサーバー側で固定されています。",
		},
	)

	destructive := false
	openWorld := true
	mcp.AddTool(server, &mcp.Tool{
		Name:        toolName,
		Title:       "DiscordにEmbedを送信",
		Description: "指定された文章をEmbedとして、サーバーに設定済みの単一Discordチャンネルへ送信します。チャンネルは選択できません。実際に外部投稿する書き込み操作です。",
		Annotations: &mcp.ToolAnnotations{
			Title: "DiscordにEmbedを送信",
			// 投稿は外部への書き込みだが、既存データを破壊せず追加だけを行う。
			// 呼び出すたびに新規メッセージが作られるため、非冪等として示す。
			ReadOnlyHint:    false,
			DestructiveHint: &destructive,
			IdempotentHint:  false,
			OpenWorldHint:   &openWorld,
		},
	}, svc.sendEmbed)

	return server
}

func (s *service) sendEmbed(ctx context.Context, _ *mcp.CallToolRequest, input SendEmbedInput) (*mcp.CallToolResult, SendEmbedOutput, error) {
	color := strings.TrimSpace(input.Color)
	if color == "" {
		// サーバー側の既定色で見た目を統一しつつ、明示された場合だけ
		// メッセージ単位の色指定を許可する。
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
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Discordへの送信に成功しました（message_id: %s）", message.ID)},
		},
	}
	return result, output, nil
}
