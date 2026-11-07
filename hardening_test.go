package metagrab

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
)

func serveHTML(t *testing.T, contentType, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestIsPublicAddr(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8":              true,
		"1.1.1.1":              true,
		"2606:4700::1111":      true,
		"127.0.0.1":            false,
		"10.1.2.3":             false,
		"172.16.0.1":           false,
		"192.168.1.1":          false,
		"169.254.169.254":      false, // cloud metadata
		"100.64.0.1":           false, // CGNAT
		"0.0.0.0":              false,
		"0.1.2.3":              false,
		"198.18.0.1":           false,
		"224.0.0.1":            false,
		"255.255.255.255":      false,
		"::1":                  false,
		"::":                   false,
		"fc00::1":              false,
		"fe80::1":              false,
		"::ffff:127.0.0.1":     false, // IPv4-mapped loopback
		"::ffff:10.0.0.1":      false,
		"64:ff9b::a00:1":       false, // NAT64 to 10.0.0.1
		"2001:db8::1":          false,
		"ff02::1":              false,
		"::ffff:93.184.216.34": true,
	}
	for in, want := range cases {
		if got := IsPublicAddr(netip.MustParseAddr(in)); got != want {
			t.Errorf("IsPublicAddr(%s) = %v, want %v", in, got, want)
		}
	}
}

func TestDenyPrivateIPsLiteralIPv6Loopback(t *testing.T) {
	u, _ := url.Parse("http://[::1]:8080/")
	if err := DenyPrivateIPs()(u); !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("err = %v, want ErrPrivateAddress", err)
	}
}

func TestDenyPrivateNetworksBlocksAtDial(t *testing.T) {
	srv := serveHTML(t, "text/html", "<title>internal</title>")
	c := NewClient(WithDenyPrivateNetworks())
	_, err := c.Fetch(context.Background(), srv.URL, MetadataFields)
	assertFetchErrorCode(t, err, ErrorCodeURLDenied)
	if !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("err = %v, want it to wrap ErrPrivateAddress", err)
	}
}

func TestURLPolicyRunsOnRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/secret", http.StatusFound)
			return
		}
		fmt.Fprint(w, "<title>secret</title>")
	}))
	defer srv.Close()

	policy := func(u *url.URL) error {
		if strings.HasPrefix(u.Path, "/secret") {
			return errors.New("blocked")
		}
		return nil
	}
	c := NewClient(WithURLPolicy(policy))
	_, err := c.Fetch(context.Background(), srv.URL+"/start", MetadataFields)
	assertFetchErrorCode(t, err, ErrorCodeURLDenied)
}

func TestRedirectToUnsupportedSchemeRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "ftp://example.com/file", http.StatusFound)
	}))
	defer srv.Close()
	_, err := NewClient().Fetch(context.Background(), srv.URL, MetadataFields)
	assertFetchErrorCode(t, err, ErrorCodeURLDenied)
}

func TestRedirectLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewClient(WithMaxRedirects(2)).Fetch(context.Background(), srv.URL+"/", MetadataFields)
	assertFetchErrorCode(t, err, ErrorCodeHTTPStatus)
	var fe *FetchError
	errors.As(err, &fe)
	if fe.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", fe.StatusCode)
	}

	_, err = NewClient(WithMaxRedirects(0)).Fetch(context.Background(), srv.URL+"/", MetadataFields)
	assertFetchErrorCode(t, err, ErrorCodeHTTPStatus)
}

func TestRelativeURLsResolveAgainstFinalURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/blog/post", http.StatusMovedPermanently)
			return
		}
		fmt.Fprint(w, `<html><head><link rel="icon" href="icon.png"><meta property="og:image" content="cover.jpg"></head></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL+"/old", PreviewFields)
	if err != nil {
		t.Fatal(err)
	}
	if want := srv.URL + "/blog/icon.png"; link.Favicon != want {
		t.Errorf("Favicon = %q, want %q", link.Favicon, want)
	}
	if want := srv.URL + "/blog/cover.jpg"; link.Image != want {
		t.Errorf("Image = %q, want %q", link.Image, want)
	}
}

func TestRetryAfterIsCapped(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, "<title>ok</title>")
	}))
	defer srv.Close()

	c := NewClient(WithRetries(1), WithMaxRetryWait(20*time.Millisecond))
	start := time.Now()
	link, err := c.Fetch(context.Background(), srv.URL, MetadataFields)
	if err != nil {
		t.Fatal(err)
	}
	if link.Title != "ok" || time.Since(start) > 2*time.Second {
		t.Fatalf("title=%q elapsed=%s", link.Title, time.Since(start))
	}
}

func TestCharsetFromHeader(t *testing.T) {
	body, _ := charmap.Windows1251.NewEncoder().String("<html><head><title>Привет мир</title></head></html>")
	for _, mask := range []Field{MetadataFields, AllFields} {
		srv := serveHTML(t, "text/html; charset=windows-1251", body)
		link, err := Fetch(context.Background(), srv.URL, mask)
		if err != nil {
			t.Fatal(err)
		}
		if link.Title != "Привет мир" {
			t.Errorf("mask %d: Title = %q", mask, link.Title)
		}
	}
}

func TestCharsetFromMetaTag(t *testing.T) {
	body, _ := japanese.ShiftJIS.NewEncoder().String(`<html><head><meta charset="shift_jis"><title>こんにちは</title></head></html>`)
	for _, mask := range []Field{MetadataFields, AllFields} {
		srv := serveHTML(t, "text/html", body)
		link, err := Fetch(context.Background(), srv.URL, mask)
		if err != nil {
			t.Fatal(err)
		}
		if link.Title != "こんにちは" {
			t.Errorf("mask %d: Title = %q", mask, link.Title)
		}
	}
}

func TestUndeclaredUTF8IsNotMangled(t *testing.T) {
	// More than 1024 bytes of ASCII before the first non-ASCII byte, so
	// charset sniffing alone would guess windows-1252.
	pad := "<!-- " + strings.Repeat("a", 1100) + " -->"
	body := "<html><head>" + pad + "<title>Café ünïcode ✓</title></head></html>"
	for _, mask := range []Field{MetadataFields, AllFields} {
		srv := serveHTML(t, "text/html", body)
		link, err := Fetch(context.Background(), srv.URL, mask)
		if err != nil {
			t.Fatal(err)
		}
		if link.Title != "Café ünïcode ✓" {
			t.Errorf("mask %d: Title = %q", mask, link.Title)
		}
	}
}

func TestJSONLDInBodyWithRichFields(t *testing.T) {
	srv := serveHTML(t, "text/html", `<html><head><title>Post</title></head><body>
		<svg><title>icon label</title></svg>
		<meta property="og:title" content="body meta must be ignored">
		<script>var x = "<title>";</script>
		<script type="application/ld+json">{"@type":"Article","headline":"Hi"}</script>
	</body></html>`)

	for _, mask := range []Field{RichFields, AllFields} {
		link, err := Fetch(context.Background(), srv.URL, mask)
		if err != nil {
			t.Fatal(err)
		}
		if len(link.JSONLD) != 1 || link.ContentType != "Article" {
			t.Errorf("mask %d: JSONLD=%v ContentType=%q", mask, link.JSONLD, link.ContentType)
		}
		if link.Title != "Post" {
			t.Errorf("mask %d: Title = %q, want head title", mask, link.Title)
		}
		if _, ok := link.OpenGraph["og:title"]; ok {
			t.Errorf("mask %d: og:title from <body> should be ignored", mask)
		}
	}
}

func TestJSONLDBlockLimitIsExact(t *testing.T) {
	arr := "[" + strings.TrimSuffix(strings.Repeat(`{"@type":"Thing"},`, 15), ",") + "]"
	srv := serveHTML(t, "text/html", `<html><head><script type="application/ld+json">`+arr+`</script></head></html>`)
	link, err := Fetch(context.Background(), srv.URL, RichFields)
	if err != nil {
		t.Fatal(err)
	}
	if len(link.JSONLD) != maxJSONLDBlocks {
		t.Fatalf("got %d blocks, want %d", len(link.JSONLD), maxJSONLDBlocks)
	}
}

func TestTitleWhitespaceCollapsed(t *testing.T) {
	srv := serveHTML(t, "text/html", "<html><head><title>\n  Hello\n\t  World  </title></head></html>")
	for _, mask := range []Field{MetadataFields, AllFields} {
		link, err := Fetch(context.Background(), srv.URL, mask)
		if err != nil {
			t.Fatal(err)
		}
		if link.Title != "Hello World" {
			t.Errorf("mask %d: Title = %q", mask, link.Title)
		}
	}
}

func TestImageFallsBackToTwitter(t *testing.T) {
	srv := serveHTML(t, "text/html", `<html><head><meta name="twitter:image" content="https://cdn.example.com/t.png"></head></html>`)
	link, err := Fetch(context.Background(), srv.URL, MetadataFields)
	if err != nil {
		t.Fatal(err)
	}
	if link.Image != "https://cdn.example.com/t.png" {
		t.Fatalf("Image = %q", link.Image)
	}
}

func TestFirstOGImageWins(t *testing.T) {
	srv := serveHTML(t, "text/html", `<html><head>
		<meta property="og:image" content="https://a.example/1.png">
		<meta property="og:image" content="https://a.example/2.png">
	</head></html>`)
	link, err := Fetch(context.Background(), srv.URL, MetadataFields)
	if err != nil {
		t.Fatal(err)
	}
	if link.Image != "https://a.example/1.png" || link.OpenGraph["og:image"] != "https://a.example/1.png" {
		t.Fatalf("Image = %q, og:image = %q", link.Image, link.OpenGraph["og:image"])
	}
}

func TestFaviconRelTokens(t *testing.T) {
	srv := serveHTML(t, "text/html", `<html><head>
		<link rel="mask-icon" href="/mask.svg">
		<link rel="icon shortcut" href="/short.ico">
		<link rel="ICON" sizes="16x16 32x32" href="/icon32.png">
		<link rel="icon" sizes="24x24" href="/icon24.png">
	</head></html>`)
	link, err := Fetch(context.Background(), srv.URL, PreviewFields)
	if err != nil {
		t.Fatal(err)
	}
	if want := srv.URL + "/icon32.png"; link.Favicon != want {
		t.Fatalf("Favicon = %q, want %q", link.Favicon, want)
	}
}

func TestFieldIs16Bit(t *testing.T) {
	var f Field = 1 << 15
	if f == 0 {
		t.Fatal("Field must hold 16 bits")
	}
}
