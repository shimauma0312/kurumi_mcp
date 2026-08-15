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
)

const (
	maxTitleLength       = 256
	maxDescriptionLength = 4096
	maxFooterLength      = 2048
	maxEmbedTotalLength  = 6000
	maxErrorBodyLength   = 4096
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	botToken   string
	channelID  string
}

type Embed struct {
	Title       string
	Description string
	Color       string
	Footer      string
}

type Message struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
}

type discordEmbed struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Footer      *discordFooter `json:"footer,omitempty"`
}

type discordFooter struct {
	Text string `json:"text"`
}

type createMessageRequest struct {
	Embeds          []discordEmbed  `json:"embeds"`
	AllowedMentions allowedMentions `json:"allowed_mentions"`
}

type allowedMentions struct {
	Parse []string `json:"parse"`
}

func NewClient(httpClient *http.Client, baseURL, botToken, channelID string) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("http client is required")
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(botToken) == "" || strings.TrimSpace(channelID) == "" {
		return nil, errors.New("base URL, bot token, and channel ID are required")
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
		botToken:   botToken,
		channelID:  channelID,
	}, nil
}

func (c *Client) SendEmbed(ctx context.Context, embed Embed) (Message, error) {
	color, err := validateEmbed(embed)
	if err != nil {
		return Message{}, err
	}

	payloadEmbed := discordEmbed{
		Title:       embed.Title,
		Description: embed.Description,
		Color:       color,
	}
	if embed.Footer != "" {
		payloadEmbed.Footer = &discordFooter{Text: embed.Footer}
	}
	payload := createMessageRequest{
		Embeds:          []discordEmbed{payloadEmbed},
		AllowedMentions: allowedMentions{Parse: []string{}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Message{}, fmt.Errorf("encode Discord request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/channels/%s/messages", c.baseURL, url.PathEscape(c.channelID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Message{}, fmt.Errorf("create Discord request: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+c.botToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DiscordBot (walnut-mcp, 0.1.0)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("send Discord request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyLength))
		if readErr != nil {
			return Message{}, fmt.Errorf("Discord API returned %s (error body unreadable: %v)", resp.Status, readErr)
		}
		return Message{}, fmt.Errorf("Discord API returned %s: %s", resp.Status, strings.TrimSpace(string(errorBody)))
	}

	var message Message
	if err := json.NewDecoder(resp.Body).Decode(&message); err != nil {
		return Message{}, fmt.Errorf("decode Discord response: %w", err)
	}
	if message.ID == "" {
		return Message{}, errors.New("Discord response did not contain a message ID")
	}
	return message, nil
}

func validateEmbed(embed Embed) (int, error) {
	titleLength := len([]rune(embed.Title))
	descriptionLength := len([]rune(embed.Description))
	footerLength := len([]rune(embed.Footer))
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
		return 0, fmt.Errorf("combined embed text must be at most %d characters", maxEmbedTotalLength)
	}

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
