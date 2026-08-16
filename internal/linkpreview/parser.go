package linkpreview

import (
	"bytes"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// HTMLから表示用メタデータを抽出。
func parseMetadata(document []byte, pageURL *url.URL) (Metadata, error) {
	root, err := html.Parse(bytes.NewReader(document))
	if err != nil {
		return Metadata{}, err
	}

	values := make(map[string]string)
	var documentTitle string
	baseURL := pageURL
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch strings.ToLower(node.Data) {
			case "meta":
				name := strings.ToLower(firstAttribute(node, "property", "name"))
				content := strings.TrimSpace(attribute(node, "content"))
				if name != "" && content != "" && values[name] == "" {
					values[name] = content
				}
			case "base":
				if href := strings.TrimSpace(attribute(node, "href")); href != "" && baseURL == pageURL {
					if parsed, err := pageURL.Parse(href); err == nil {
						baseURL = parsed
					}
				}
			case "title":
				if documentTitle == "" {
					documentTitle = strings.TrimSpace(textContent(node))
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)

	imageURL := firstValue(values, "og:image:secure_url", "og:image:url", "og:image", "twitter:image", "twitter:image:src")
	if imageURL != "" {
		resolved, err := baseURL.Parse(imageURL)
		if err != nil {
			imageURL = ""
		} else if safeImageURL, err := parseTargetURL(resolved.String()); err != nil {
			imageURL = ""
		} else {
			imageURL = safeImageURL.String()
		}
	}

	return Metadata{
		Title:       firstNonEmpty(firstValue(values, "og:title", "twitter:title"), documentTitle),
		Description: firstValue(values, "og:description", "twitter:description", "description"),
		ImageURL:    imageURL,
		SiteName:    firstValue(values, "og:site_name"),
	}, nil
}

func attribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func firstAttribute(node *html.Node, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(attribute(node, name)); value != "" {
			return value
		}
	}
	return ""
}

func firstValue(values map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(values[name]); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func textContent(node *html.Node) string {
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			text.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return text.String()
}
