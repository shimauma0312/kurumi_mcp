package linkpreview

import (
	"net/url"
	"testing"
)

// OGPの主要項目を属性順序に依存せず抽出し、secure imageを通常のog:imageより優先し、
// base要素を基準に画像URLを絶対化することを検証する。後で別モジュールへ切り出しても、
// Discord固有処理を持たないMetadataとして同じ解析結果を利用できることを固定する。
func TestParseMetadataPrefersOpenGraphValues(t *testing.T) {
	document := []byte(`<!doctype html><html><head>
		<title>通常タイトル</title>
		<base href="https://cdn.example/assets/">
		<meta content="通常画像.jpg" property="og:image">
		<meta content="安全画像.jpg" property="og:image:secure_url">
		<meta content="OGPタイトル" property="og:title">
		<meta content="OGP概要" property="og:description">
		<meta content="サイト名" property="og:site_name">
	</head></html>`)
	pageURL, err := url.Parse("https://news.example/articles/1")
	if err != nil {
		t.Fatal(err)
	}

	metadata, err := parseMetadata(document, pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Title != "OGPタイトル" || metadata.Description != "OGP概要" || metadata.SiteName != "サイト名" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata.ImageURL != "https://cdn.example/assets/%E5%AE%89%E5%85%A8%E7%94%BB%E5%83%8F.jpg" {
		t.Fatalf("image URL = %q", metadata.ImageURL)
	}
}

// OGPがないページではTwitter Cardとtitle要素を順にフォールバックし、
// 画像メタデータがない場合もエラーではなく空のImageURLを返すことを検証する。
// OGP非対応ページの投稿まで失敗させず、リンク付きEmbedだけは送れる状態を保証する。
func TestParseMetadataUsesFallbacks(t *testing.T) {
	document := []byte(`<!doctype html><html><head>
		<title> 通常タイトル </title>
		<meta name="twitter:description" content="Twitter概要">
	</head></html>`)
	pageURL, err := url.Parse("https://news.example/article")
	if err != nil {
		t.Fatal(err)
	}

	metadata, err := parseMetadata(document, pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Title != "通常タイトル" || metadata.Description != "Twitter概要" || metadata.ImageURL != "" {
		t.Fatalf("metadata = %#v", metadata)
	}
}
