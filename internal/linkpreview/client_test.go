package linkpreview

import (
	"context"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Fetchがページ取得時にHTMLだけを要求し、レスポンスの最終URLを基準に相対OGP画像を
// 絶対URLへ変換することを検証する。取得先サーバーはテスト用RoundTripperで置き換え、
// 実際のインターネットへ通信せずHTTP取得から解析までの業務フローを通す。
func TestFetchParsesMetadataFromHTMLResponse(t *testing.T) {
	htmlDocument := `<!doctype html><html><head>
		<meta content="テストサイト" property="og:site_name">
		<meta property="og:title" content="ニュースのタイトル">
		<meta name="description" content="概要">
		<meta property="og:image" content="../images/news.png">
	</head></html>`

	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Accept") != "text/html, application/xhtml+xml" {
			t.Errorf("Accept = %q", req.Header.Get("Accept"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			Body:       io.NopCloser(strings.NewReader(htmlDocument)),
			Request:    req.Clone(req.Context()),
		}, nil
	})}}

	metadata, err := client.Fetch(context.Background(), "https://news.example/articles/2026/story")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Title != "ニュースのタイトル" || metadata.Description != "概要" || metadata.SiteName != "テストサイト" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata.ImageURL != "https://news.example/articles/images/news.png" {
		t.Fatalf("image URL = %q", metadata.ImageURL)
	}
}

// 外部入力URLを取得する前に、認証情報付きURL、非HTTPスキーム、標準外ポート、
// localhost・単一ラベル名・プライベートIPを拒否することを検証する。
// SSRF対策が失敗した場合を検出できるよう、RoundTripperが一度でも呼ばれたらテストを失敗させる。
func TestFetchRejectsUnsafeTargetBeforeRequest(t *testing.T) {
	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP request was sent for an unsafe target")
		return nil, nil
	})}}

	unsafeTargets := []string{
		"file:///etc/passwd",
		"https://user:pass@example.com/news",
		"http://example.com:8080/news",
		"http://localhost/news",
		"http://service/news",
		"http://127.0.0.1/news",
		"http://[::1]/news",
		"http://169.254.169.254/latest/meta-data/",
	}
	for _, target := range unsafeTargets {
		t.Run(target, func(t *testing.T) {
			if _, err := client.Fetch(context.Background(), target); err == nil {
				t.Fatal("Fetch() error = nil, want unsafe-target error")
			}
		})
	}
}

// HTML以外のレスポンスと上限を超えるHTMLを解析せずエラーにすることを検証する。
// 外部サーバーが巨大な本文や画像ファイルを返しても、無制限にメモリへ読み込まないことを確認する。
func TestFetchLimitsResponseTypeAndSize(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "non HTML", contentType: "image/png", body: "not-html"},
		{name: "oversized HTML", contentType: "text/html; charset=utf-8", body: strings.Repeat("a", maxHTMLBytes+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     http.Header{"Content-Type": []string{tt.contentType}},
					Body:       io.NopCloser(strings.NewReader(tt.body)),
					Request:    req,
				}, nil
			})}}
			if _, err := client.Fetch(context.Background(), "https://news.example/article"); err == nil {
				t.Fatal("Fetch() error = nil, want response validation error")
			}
		})
	}
}

// DNS解決後の接続判定が公開IPだけを許可し、プライベート、loopback、link-local、
// CGNAT、文書用アドレスを拒否することを検証する。URL文字列の検査だけでは防げない、
// 公開ホスト名が内部IPへ解決されるSSRF経路の判定条件を直接確認する。
func TestIsPublicAddress(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{address: "8.8.8.8", want: true},
		{address: "2606:4700:4700::1111", want: true},
		{address: "10.0.0.1", want: false},
		{address: "127.0.0.1", want: false},
		{address: "169.254.169.254", want: false},
		{address: "100.64.0.1", want: false},
		{address: "192.0.2.1", want: false},
		{address: "::1", want: false},
		{address: "fc00::1", want: false},
		{address: "2001:db8::1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			if got := isPublicAddress(netip.MustParseAddr(tt.address)); got != tt.want {
				t.Fatalf("isPublicAddress(%s) = %t, want %t", tt.address, got, tt.want)
			}
		})
	}
}

// 本番用クライアントがゼロ以下のタイムアウトを受け入れず、外部ページ取得が
// 無期限に待機する設定を起動時に作れないことを検証する。
func TestNewClientRequiresPositiveTimeout(t *testing.T) {
	if _, err := NewClient(0); err == nil {
		t.Fatal("NewClient(0) error = nil, want timeout validation error")
	}
	if _, err := NewClient(time.Second); err != nil {
		t.Fatalf("NewClient(time.Second) error = %v", err)
	}
}
