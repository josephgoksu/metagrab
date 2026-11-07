package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/josephgoksu/metagrab"
)

func TestParseFields(t *testing.T) {
	cases := map[string]metagrab.Field{
		"metadata": metagrab.MetadataFields,
		"Preview":  metagrab.PreviewFields,
		"rich":     metagrab.RichFields,
		"all":      metagrab.AllFields,
		"47":       47,
	}
	for in, want := range cases {
		got, err := parseFields(in)
		if err != nil || got != want {
			t.Errorf("parseFields(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "0", "nope", "-1", "70000"} {
		if _, err := parseFields(bad); err == nil {
			t.Errorf("parseFields(%q) should fail", bad)
		}
	}
}

func page(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `<html><head><title>Hello</title></head></html>`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRunFetchSingle(t *testing.T) {
	srv := page(t)
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"-quiet", "-compact", srv.URL}, &out, &errb)
	if code != exitOK {
		t.Fatalf("exit %d, stderr %s", code, errb.String())
	}
	var link metagrab.Link
	if err := json.Unmarshal(out.Bytes(), &link); err != nil || link.Title != "Hello" {
		t.Fatalf("link=%+v err=%v out=%s", link, err, out.String())
	}
	if strings.Count(strings.TrimSpace(out.String()), "\n") != 0 {
		t.Fatal("-compact should print one line")
	}
}

func TestRunFetchBulkPartialFailure(t *testing.T) {
	srv := page(t)
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"-quiet", srv.URL, srv.URL + "/missing"}, &out, &errb)
	if code != exitFetch {
		t.Fatalf("exit %d, want %d", code, exitFetch)
	}
	var results []metagrab.BulkResult
	if err := json.Unmarshal(out.Bytes(), &results); err != nil || len(results) != 2 {
		t.Fatalf("results=%v err=%v", results, err)
	}
	if !strings.Contains(errb.String(), "1 of 2 URLs failed") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestRunUsageErrors(t *testing.T) {
	if code := run(context.Background(), nil, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("no args: exit %d", code)
	}
	if code := run(context.Background(), []string{"-fields=bogus", "https://example.com"}, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("bad fields: exit %d", code)
	}
	if code := run(context.Background(), []string{"-h"}, io.Discard, io.Discard); code != exitOK {
		t.Fatalf("-h: exit %d", code)
	}
}

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if code := run(context.Background(), []string{"version"}, &out, io.Discard); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(out.String(), "metagrab ") {
		t.Fatalf("version output %q", out.String())
	}
}

func TestParseServeFlagsEnv(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("METAGRAB_API_KEY", "k")
	cfg, err := parseServeFlags(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.addr != ":9999" || cfg.apiKey != "k" || cfg.allowPrivate {
		t.Fatalf("cfg = %+v", cfg)
	}

	t.Setenv("METAGRAB_ADDR", "127.0.0.1:7000")
	cfg, _ = parseServeFlags([]string{"-api-key", "flag"}, io.Discard)
	if cfg.addr != "127.0.0.1:7000" || cfg.apiKey != "flag" {
		t.Fatalf("cfg = %+v", cfg)
	}

	if _, err := parseServeFlags([]string{"extra"}, io.Discard); err == nil {
		t.Fatal("stray arguments should fail")
	}
}

func TestServerDeniesPrivateNetworksByDefault(t *testing.T) {
	upstream := page(t)
	for _, allow := range []bool{false, true} {
		srv := httptest.NewServer(newServer(serveConfig{timeout: 2e9, maxBulk: 10, allowPrivate: allow}).Handler)
		resp, err := http.Post(srv.URL+"/fetch", "application/json", strings.NewReader(fmt.Sprintf(`{"url":%q}`, upstream.URL)))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		srv.Close()
		want := http.StatusBadRequest
		if allow {
			want = http.StatusOK
		}
		if resp.StatusCode != want {
			t.Errorf("allowPrivate=%v: status %d, want %d", allow, resp.StatusCode, want)
		}
	}
}
