package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	maxTitleLength        = 256
	maxDescriptionLength  = 4096
	maxFooterLength       = 2048
	maxEmbedTotalLength   = 6000
	maxErrorBodyLength    = 4096
	maxResponseBodyLength = 1 << 20
	maxRateLimitDelay     = 30 * time.Second
	maxRateLimitRetries   = 1
)

// 1回の履歴取得上限。
const MaxRecentMessages = 5

// 固定チャンネル専用のDiscord RESTクライアント。
type Client struct {
	httpClient *http.Client
	baseURL    string
	botToken   string
	// 固定送信先。
	channelID    string
	thumbnailURL string
}

// MCPへ公開するRich Embed項目。
type Embed struct {
	Title       string
	Description string
	Color       string
	Footer      string
	ImageURL    string
}

// 作成したメッセージの識別情報。
type Message struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
}

// チャンネル履歴のメッセージ。
type RecentMessage struct {
	ID         string          `json:"message_id" jsonschema:"DiscordメッセージID"`
	AuthorName string          `json:"author_name" jsonschema:"投稿者の表示名"`
	AuthorBot  bool            `json:"author_bot" jsonschema:"Botによる投稿か"`
	Content    string          `json:"content" jsonschema:"通常メッセージの本文"`
	Timestamp  string          `json:"timestamp" jsonschema:"Discordが記録した投稿日時"`
	Embeds     []ReceivedEmbed `json:"embeds" jsonschema:"メッセージに含まれるEmbed"`
}

// 履歴から取得するEmbed項目。
type ReceivedEmbed struct {
	Title       string `json:"title,omitempty" jsonschema:"Embedのタイトル"`
	Description string `json:"description,omitempty" jsonschema:"Embedの本文"`
	Footer      string `json:"footer,omitempty" jsonschema:"Embedのフッター"`
	ImageURL    string `json:"image_url,omitempty" jsonschema:"Embedに表示された画像URL"`
}

type discordEmbed struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Footer      *discordFooter `json:"footer,omitempty"`
	Thumbnail   *discordImage  `json:"thumbnail,omitempty"`
	Image       *discordImage  `json:"image,omitempty"`
}

type discordFooter struct {
	Text string `json:"text"`
}

type discordImage struct {
	URL string `json:"url"`
}

// Discord APIの履歴レスポンス受信用。
// MCPへ不要なDiscord固有項目は定義しない。
type channelMessage struct {
	ID        string                `json:"id"`
	Author    channelMessageAuthor  `json:"author"`
	Content   string                `json:"content"`
	Timestamp string                `json:"timestamp"`
	Embeds    []channelMessageEmbed `json:"embeds"`
}

type channelMessageAuthor struct {
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
	Bot        bool   `json:"bot"`
}

type channelMessageEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Footer      *discordFooter `json:"footer"`
	Image       *discordImage  `json:"image"`
}

type createMessageRequest struct {
	Embeds          []discordEmbed  `json:"embeds"`
	AllowedMentions allowedMentions `json:"allowed_mentions"`
}

type allowedMentions struct {
	Parse []string `json:"parse"`
}

// 固定チャンネル専用クライアントを生成。
func NewClient(httpClient *http.Client, baseURL, botToken, channelID, thumbnailURL string) (*Client, error) {
	// Discord通信と固定送信先に必要な設定を検証。
	if httpClient == nil {
		return nil, errors.New("http client is required")
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(botToken) == "" || strings.TrimSpace(channelID) == "" {
		return nil, errors.New("base URL, bot token, and channel ID are required")
	}

	// 任意サムネイルをDiscordが取得できるURLへ限定。
	thumbnailURL = strings.TrimSpace(thumbnailURL)
	if thumbnailURL != "" {
		parsed, err := url.ParseRequestURI(thumbnailURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, errors.New("thumbnail URL must use http or https")
		}
	}
	// 送信先と認証情報をクライアント内へ固定。
	return &Client{
		httpClient:   httpClient,
		baseURL:      strings.TrimRight(baseURL, "/"),
		botToken:     botToken,
		channelID:    channelID,
		thumbnailURL: thumbnailURL,
	}, nil
}

// Embedを検証し、固定チャンネルへ投稿。
func (c *Client) SendEmbed(ctx context.Context, embed Embed) (Message, error) {
	// Discordの文字数上限と色形式を送信前に検証。
	color, err := validateEmbed(embed)
	if err != nil {
		return Message{}, err
	}

	// MCPの入力をDiscord APIのJSON形式へ変換。
	payloadEmbed := discordEmbed{
		Title:       embed.Title,
		Description: embed.Description,
		Color:       color,
	}
	if embed.Footer != "" {
		payloadEmbed.Footer = &discordFooter{Text: embed.Footer}
	}
	if c.thumbnailURL != "" {
		payloadEmbed.Thumbnail = &discordImage{URL: c.thumbnailURL}
	}
	imageURL := strings.TrimSpace(embed.ImageURL)
	if imageURL != "" {
		if err := validateImageURL(imageURL); err != nil {
			return Message{}, fmt.Errorf("image URL: %w", err)
		}
		payloadEmbed.Image = &discordImage{URL: imageURL}
	}
	payload := createMessageRequest{
		Embeds: []discordEmbed{payloadEmbed},
		// メンション通知を無効化。
		AllowedMentions: allowedMentions{Parse: []string{}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Message{}, fmt.Errorf("encode Discord request: %w", err)
	}

	// 設定済みの固定チャンネルへPOST。
	endpoint := fmt.Sprintf("%s/channels/%s/messages", c.baseURL, url.PathEscape(c.channelID))
	resp, err := c.doDiscordRequest(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return Message{}, fmt.Errorf("send Discord request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Message{}, discordResponseError(resp)
	}

	// Discordが発行したメッセージIDをMCP層へ返却。
	var message Message
	if err := decodeLimitedJSON(resp.Body, &message); err != nil {
		return Message{}, fmt.Errorf("decode Discord response: %w", err)
	}
	if message.ID == "" {
		return Message{}, errors.New("Discord response did not contain a message ID")
	}
	return message, nil
}

// 固定チャンネルの直近履歴を古い順で取得。
func (c *Client) ReadRecentMessages(ctx context.Context, limit int) ([]RecentMessage, error) {
	// MCP以外から呼ばれてもMaxRecentMessages件を超えないようDiscord層でも検証しとく。
	if limit < 1 || limit > MaxRecentMessages {
		return nil, fmt.Errorf("message limit must be between 1 and %d", MaxRecentMessages)
	}

	// 設定済みの固定チャンネルへGET。
	endpoint := fmt.Sprintf("%s/channels/%s/messages?limit=%d", c.baseURL, url.PathEscape(c.channelID), limit)
	resp, err := c.doDiscordRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("send Discord request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, discordResponseError(resp)
	}

	// Discordの履歴レスポンスを受信専用型へデコード。
	var messages []channelMessage
	if err := decodeLimitedJSON(resp.Body, &messages); err != nil {
		return nil, fmt.Errorf("decode Discord response: %w", err)
	}

	// Discordの新しい順を、会話として読みやすい古い順へ反転。
	result := make([]RecentMessage, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]

		// サーバー表示名を優先し、未設定ならユーザー名を使用。
		authorName := message.Author.GlobalName
		if authorName == "" {
			authorName = message.Author.Username
		}

		// Bot投稿の文脈も読めるようEmbedの文章部分を保持。
		embeds := make([]ReceivedEmbed, 0, len(message.Embeds))
		for _, embed := range message.Embeds {
			footer := ""
			if embed.Footer != nil {
				footer = embed.Footer.Text
			}
			embeds = append(embeds, ReceivedEmbed{
				Title:       embed.Title,
				Description: embed.Description,
				Footer:      footer,
				ImageURL:    imageURL(embed.Image),
			})
		}

		result = append(result, RecentMessage{
			ID:         message.ID,
			AuthorName: authorName,
			AuthorBot:  message.Author.Bot,
			Content:    message.Content,
			Timestamp:  message.Timestamp,
			Embeds:     embeds,
		})
	}
	return result, nil
}

// Discordへ渡す画像URLを検証。
func validateImageURL(raw string) error {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("must use an absolute http or https URL without user information")
	}
	return nil
}

// 画像がないEmbedを空文字へ変換。
func imageURL(image *discordImage) string {
	if image == nil {
		return ""
	}
	return image.URL
}

// Discord APIを実行し、短いレート制限だけ1回待って再試行。
func (c *Client) doDiscordRequest(ctx context.Context, method, endpoint string, body []byte) (*http.Response, error) {
	for attempt := 0; attempt <= maxRateLimitRetries; attempt++ {
		var requestBody io.Reader
		if body != nil {
			requestBody = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
		if err != nil {
			return nil, fmt.Errorf("create Discord request: %w", err)
		}
		req.Header.Set("Authorization", "Bot "+c.botToken)
		req.Header.Set("User-Agent", "DiscordBot (walnut-mcp, 0.1.0)")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt == maxRateLimitRetries {
			return resp, nil
		}

		delay, err := readRateLimitDelay(resp)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		}
	}
	return nil, errors.New("Discord rate-limit retry exhausted")
}

// Discordの429本文から待機時間を取得。
func readRateLimitDelay(resp *http.Response) (time.Duration, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyLength+1))
	if err != nil {
		return 0, fmt.Errorf("read Discord rate-limit response: %w", err)
	}
	if len(body) > maxErrorBodyLength {
		return 0, errors.New("Discord rate-limit response exceeded size limit")
	}

	var payload struct {
		RetryAfter *float64 `json:"retry_after"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.RetryAfter == nil || *payload.RetryAfter < 0 {
		return 0, errors.New("Discord rate-limit response did not contain a valid retry_after")
	}
	delay := time.Duration(*payload.RetryAfter * float64(time.Second))
	if delay > maxRateLimitDelay {
		return 0, fmt.Errorf("Discord rate-limit delay exceeds %s", maxRateLimitDelay)
	}
	return delay, nil
}

// Discordのエラー本文を上限付きで整形。
func discordResponseError(resp *http.Response) error {
	errorBody, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyLength+1))
	if err != nil {
		return fmt.Errorf("Discord API returned %s (error body unreadable: %v)", resp.Status, err)
	}
	if len(errorBody) > maxErrorBodyLength {
		return fmt.Errorf("Discord API returned %s (error body exceeded size limit)", resp.Status)
	}
	return fmt.Errorf("Discord API returned %s: %s", resp.Status, strings.TrimSpace(string(errorBody)))
}

// 成功レスポンスを上限付きでJSONへ変換。
func decodeLimitedJSON(body io.Reader, destination any) error {
	data, err := io.ReadAll(io.LimitReader(body, maxResponseBodyLength+1))
	if err != nil {
		return err
	}
	if len(data) > maxResponseBodyLength {
		return errors.New("Discord response exceeded size limit")
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return err
	}
	return nil
}

// Embedを検証し、色を整数へ変換。
func validateEmbed(embed Embed) (int, error) {
	// Discordの上限判定に合わせてUnicode文字数を計測。
	titleLength := len([]rune(embed.Title))
	descriptionLength := len([]rune(embed.Description))
	footerLength := len([]rune(embed.Footer))
	// 各テキスト項目の個別上限を検証。
	if descriptionLength == 0 || descriptionLength > maxDescriptionLength {
		return 0, fmt.Errorf("description must be between 1 and %d characters", maxDescriptionLength)
	}
	if titleLength > maxTitleLength {
		return 0, fmt.Errorf("title must be at most %d characters", maxTitleLength)
	}
	if footerLength > maxFooterLength {
		return 0, fmt.Errorf("footer must be at most %d characters", maxFooterLength)
	}
	if titleLength+descriptionLength+footerLength > maxEmbedTotalLength {
		// Embed内テキストの合計上限。
		return 0, fmt.Errorf("combined embed text must be at most %d characters", maxEmbedTotalLength)
	}

	// 人向けの#RRGGBBをDiscord APIの整数色へ変換。
	hexColor := strings.TrimPrefix(strings.TrimSpace(embed.Color), "#")
	if len(hexColor) != 6 {
		return 0, errors.New("color must be a 6-digit hex value such as #5865F2")
	}
	color, err := strconv.ParseUint(hexColor, 16, 24)
	if err != nil {
		return 0, errors.New("color must be a 6-digit hex value such as #5865F2")
	}
	return int(color), nil
}
