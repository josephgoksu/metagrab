package metagrab

import (
	"strings"
	"testing"
)

func TestParseFromReader(t *testing.T) {
	doc := `<html><head>
		<title>Parsed</title>
		<link rel="canonical" href="/canon">
		<meta property="og:image" content="img.png">
		<script type="application/ld+json">{"@type":"Recipe"}</script>
	</head><body>hi</body></html>`

	link, err := Parse(strings.NewReader(doc), "https://example.com/a/b", RichFields)
	if err != nil {
		t.Fatal(err)
	}
	if link.Title != "Parsed" || link.CanonicalURL != "https://example.com/canon" ||
		link.Image != "https://example.com/a/img.png" || link.ContentType != "Recipe" ||
		link.Favicon != "https://example.com/favicon.ico" {
		t.Fatalf("unexpected link: %+v", link)
	}
}

func TestParseInvalidURL(t *testing.T) {
	_, err := Parse(strings.NewReader("<title>x</title>"), "not a url", MetadataFields)
	assertFetchErrorCode(t, err, ErrorCodeInvalidURL)
}

func TestParseTitleFallbackAndContent(t *testing.T) {
	link, err := Parse(strings.NewReader("<p>no head</p>"), "https://example.org", AllFields)
	if err != nil {
		t.Fatal(err)
	}
	if link.Title != "example.org" || !strings.Contains(link.Content, "no head") {
		t.Fatalf("unexpected link: %+v", link)
	}
}

func TestParseBodyTooLarge(t *testing.T) {
	c := NewClient(WithMaxBodySize(10))
	_, err := c.Parse(strings.NewReader(strings.Repeat("x", 100)), "https://example.org", AllFields)
	assertFetchErrorCode(t, err, ErrorCodeBodyTooLarge)
}
