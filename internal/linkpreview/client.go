package linkpreview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

const (
	maxHTMLBytes          = 1 << 20
	maxResponseHeaderSize = 64 << 10
	maxRedirects          = 5
)

// リンク先から取得する表示用メタデータ。
type Metadata struct {
	Title       string
	Description string
	ImageURL    string
	SiteName    string
}

// リンク先の表示用メタデータ取得。
type Fetcher interface {
	Fetch(context.Context, string) (Metadata, error)
}

// 外部ページを安全に取得するクライアント。
type Client struct {
	httpClient *http.Client
}

// プライベートネットワークへ接続しないクライアントを生成。
func NewClient(timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		return nil, errors.New("timeout must be greater than zero")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	// 環境変数のHTTPプロキシ経由で接続制限を迂回されないよう直接接続に固定。
	transport.Proxy = nil
	transport.DialContext = dialPublicAddress
	transport.MaxResponseHeaderBytes = maxResponseHeaderSize

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			if _, err := parseTargetURL(req.URL.String()); err != nil {
				return fmt.Errorf("unsafe redirect target: %w", err)
			}
			return nil
		},
	}
	return &Client{httpClient: httpClient}, nil
}

// HTMLを上限付きで取得し、OGPとTwitter Cardを解析。
func (c *Client) Fetch(ctx context.Context, rawURL string) (Metadata, error) {
	target, err := parseTargetURL(rawURL)
	if err != nil {
		return Metadata{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return Metadata{}, fmt.Errorf("create link preview request: %w", err)
	}
	req.Header.Set("Accept", "text/html, application/xhtml+xml")
	req.Header.Set("User-Agent", "go-linkpreview/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Metadata{}, fmt.Errorf("fetch link preview: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Metadata{}, fmt.Errorf("link preview returned %s", resp.Status)
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || (mediaType != "text/html" && mediaType != "application/xhtml+xml") {
		return Metadata{}, errors.New("link preview response is not HTML")
	}

	limited := io.LimitReader(resp.Body, maxHTMLBytes+1)
	decoded, err := charset.NewReader(limited, resp.Header.Get("Content-Type"))
	if err != nil {
		return Metadata{}, fmt.Errorf("decode link preview HTML: %w", err)
	}
	body, err := io.ReadAll(decoded)
	if err != nil {
		return Metadata{}, fmt.Errorf("read link preview HTML: %w", err)
	}
	if len(body) > maxHTMLBytes {
		return Metadata{}, fmt.Errorf("link preview HTML exceeded %d bytes", maxHTMLBytes)
	}

	baseURL := target
	if resp.Request != nil && resp.Request.URL != nil {
		baseURL = resp.Request.URL
	}
	metadata, err := parseMetadata(body, baseURL)
	if err != nil {
		return Metadata{}, fmt.Errorf("parse link preview HTML: %w", err)
	}
	return metadata, nil
}

// URL形式、接続先ポート、明白なローカル宛先を検証。
func parseTargetURL(raw string) (*url.URL, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("URL must be an absolute http or https URL without user information")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("URL must use http or https")
	}

	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if hostname == "" {
		return nil, errors.New("URL hostname is required")
	}
	if parsed.Port() != "" {
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || (parsed.Scheme == "http" && port != 80) || (parsed.Scheme == "https" && port != 443) {
			return nil, errors.New("URL must use the standard port for its scheme")
		}
	}
	if address, err := netip.ParseAddr(hostname); err == nil {
		if !isPublicAddress(address) {
			return nil, errors.New("URL address must be public")
		}
	} else if isLocalHostname(hostname) {
		return nil, errors.New("URL hostname must be public")
	}

	parsed.Fragment = ""
	return parsed, nil
}

func isLocalHostname(hostname string) bool {
	if !strings.Contains(hostname, ".") {
		return true
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".home.arpa"} {
		if strings.HasSuffix(hostname, suffix) {
			return true
		}
	}
	return false
}

// DNS解決した公開IPへ直接接続し、DNS rebindingと内部ネットワーク到達を防止。
func dialPublicAddress(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split link preview address: %w", err)
	}

	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve link preview host: %w", err)
	}
	if len(addresses) == 0 {
		return nil, errors.New("link preview host did not resolve")
	}
	for _, candidate := range addresses {
		if !isPublicAddress(candidate) {
			return nil, fmt.Errorf("link preview host resolved to a non-public address: %s", candidate)
		}
	}

	var connectionErrors []error
	dialer := net.Dialer{}
	for _, candidate := range addresses {
		connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
		if err == nil {
			return connection, nil
		}
		connectionErrors = append(connectionErrors, err)
	}
	return nil, fmt.Errorf("connect to link preview host: %w", errors.Join(connectionErrors...))
}

// 公開インターネットとして接続を許可するIPか判定。
func isPublicAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsUnspecified() {
		return false
	}

	for _, prefix := range blockedAddressPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

var blockedAddressPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
}
