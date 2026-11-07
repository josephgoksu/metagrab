package metagrab

import (
	"strings"
	"testing"
	"testing/iotest"
)

func scanBlocks(t *testing.T, body string) []map[string]any {
	t.Helper()
	link := &Link{}
	e := &extractor{link: link, mask: JSONLDField}
	if err := scanJSONLD(strings.NewReader(body), e); err != nil {
		t.Fatal(err)
	}
	// Same input one byte at a time, to exercise buffer boundaries.
	slow := &Link{}
	if err := scanJSONLD(iotest.OneByteReader(strings.NewReader(body)), &extractor{link: slow, mask: JSONLDField}); err != nil {
		t.Fatal(err)
	}
	if len(slow.JSONLD) != len(link.JSONLD) {
		t.Fatalf("one-byte reader found %d blocks, full reader %d", len(slow.JSONLD), len(link.JSONLD))
	}
	return link.JSONLD
}

func TestScanJSONLDEdgeCases(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"plain", `<script type="application/ld+json">{"@type":"A"}</script>`, 1},
		{"upper case tag and type", `<SCRIPT TYPE="Application/LD+JSON">{"@type":"A"}</SCRIPT>`, 1},
		{"single quotes, extra attrs", `<script id='x' type='application/ld+json' nonce="a>b">{"@type":"A"}</script>`, 1},
		{"unquoted type", `<script type=application/ld+json>{"@type":"A"}</script>`, 1},
		{"inside comment ignored", `<!-- <script type="application/ld+json">{"@type":"A"}</script> -->`, 0},
		{"inside other script ignored", `<script>var s = '<script type="application/ld+json">{"@type":"A"}';</script>`, 0},
		{"scripty tag name ignored", `<scripts type="application/ld+json">{"@type":"A"}</scripts>`, 0},
		{"JS script then LD", `<script>if (a<b) {}</script><p>x</p><script type="application/ld+json">[{"@type":"A"},{"@type":"B"}]</script>`, 2},
		{"unclosed LD script", `<script type="application/ld+json">{"@type":"A"}`, 1},
		{"unclosed comment", `<!-- <script type="application/ld+json">{"@type":"A"}</script>`, 0},
		{"unterminated tag", `<script type="application/ld+json"`, 0},
		{"long body", strings.Repeat("<p>filler</p>", 20000) + `<script type="application/ld+json">{"@type":"A"}</script>`, 1},
		{"not LD", `<script type="application/json">{"@type":"A"}</script>`, 0},
	}
	for _, c := range cases {
		if got := len(scanBlocks(t, c.body)); got != c.want {
			t.Errorf("%s: got %d blocks, want %d", c.name, got, c.want)
		}
	}
}

func TestJSONLDAfterHeadWithoutBodyTag(t *testing.T) {
	srv := serveHTML(t, "text/html", `<html><head><title>T</title></head>
		<meta property="og:title" content="ignored">
		<script type="application/ld+json">{"@type":"Event"}</script>`)
	link, err := Fetch(t.Context(), srv.URL, RichFields)
	if err != nil {
		t.Fatal(err)
	}
	if link.ContentType != "Event" || link.OpenGraph["og:title"] != "" {
		t.Fatalf("ContentType=%q og:title=%q", link.ContentType, link.OpenGraph["og:title"])
	}
}
