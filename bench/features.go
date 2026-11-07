package main

import (
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// FeatureRow is one line of the comparison table. Values are "yes", "no",
// "partial" or "n/a", keyed by library column name.
type FeatureRow struct {
	Feature  string            `json:"feature"`
	Measured bool              `json:"measured"` // true: checked by running the library
	Values   map[string]string `json:"values"`
}

// Columns of the feature table: one per library ("metagrab" uses RichFields).
var featureLibs = []string{"metagrab (rich)", "otiai10/opengraph", "dyatlov/go-opengraph", "goquery (hand-rolled)", "badoux/goscraper"}

const cyrillicTitle = "Новости дня"

func fixtureHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cp1251" {
			body, _ := charmap.Windows1251.NewEncoder().String(`<html><head><title>` + cyrillicTitle + `</title></head><body></body></html>`)
			w.Header().Set("Content-Type", "text/html; charset=windows-1251")
			_, _ = w.Write([]byte(body))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func yesNo(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}

// Features runs every library against fixtures on base and records what it
// actually extracts. Rows marked Measured=false come from reading each
// library's source (see docs/benchmarks/README.md).
func Features(base string) []FeatureRow {
	byName := map[string]Lib{}
	for _, l := range Libs() {
		byName[l.Name] = l
	}
	hc := &http.Client{Timeout: 30 * time.Second}

	type got struct{ medium, large, cp1251 Result }
	results := map[string]got{}
	for _, name := range featureLibs {
		l := byName[name]
		var g got
		g.medium, _ = l.Fetch(hc, base+"/medium")
		g.large, _ = l.Fetch(hc, base+"/large")
		g.cp1251, _ = l.Fetch(hc, base+"/cp1251")
		results[name] = g
	}

	abs := func(s string) bool { return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") }
	measured := []struct {
		name  string
		check func(g got) string
	}{
		{"Title", func(g got) string { return yesNo(g.medium.Title != "") }},
		{"Description", func(g got) string { return yesNo(g.medium.Description != "") }},
		{"Preview image as absolute URL", func(g got) string {
			switch {
			case abs(g.medium.Image):
				return "yes"
			case g.medium.Image != "":
				return "partial" // found, but left relative
			}
			return "no"
		}},
		{"Best favicon (apple-touch-icon over icon)", func(g got) string {
			switch {
			case strings.HasSuffix(g.medium.Favicon, "/apple-touch-icon.png") && abs(g.medium.Favicon):
				return "yes"
			case g.medium.Favicon != "":
				return "partial"
			}
			return "no"
		}},
		{"Canonical URL", func(g got) string {
			switch {
			case abs(g.medium.Canonical) && strings.HasSuffix(g.medium.Canonical, "/medium"):
				return "yes"
			case g.medium.Canonical != "":
				return "partial"
			}
			return "no"
		}},
		{"Author and published time", func(g got) string {
			n := 0
			if g.medium.Author != "" {
				n++
			}
			if g.medium.Published != "" {
				n++
			}
			return map[int]string{0: "no", 1: "partial", 2: "yes"}[n]
		}},
		{"Twitter Card", func(g got) string { return yesNo(g.medium.TwitterCard != "") }},
		{"JSON-LD (including in <body>)", func(g got) string { return yesNo(g.large.JSONLD > 0) }},
		{"Non-UTF-8 page decoded", func(g got) string { return yesNo(g.cp1251.Title == cyrillicTitle) }},
	}

	var rows []FeatureRow
	for _, m := range measured {
		row := FeatureRow{Feature: m.name, Measured: true, Values: map[string]string{}}
		for _, name := range featureLibs {
			row.Values[name] = m.check(results[name])
		}
		rows = append(rows, row)
	}

	// From reading each library's source at the pinned version.
	static := []struct {
		name string
		vals []string // same order as featureLibs
	}{
		{"Stops reading after </head> when it can", []string{"yes", "no", "no", "no", "no"}},
		{"SSRF protection (private IPs, redirects, DNS rebinding)", []string{"yes", "no", "n/a", "n/a", "no"}},
		{"Retries with backoff and Retry-After", []string{"yes", "no", "n/a", "n/a", "no"}},
		{"Response size limit", []string{"yes", "no", "n/a", "n/a", "no"}},
		{"context.Context cancellation", []string{"yes", "yes", "n/a", "n/a", "no"}},
		{"oEmbed (YouTube, Vimeo, Spotify, X, ...)", []string{"yes", "no", "n/a", "n/a", "no"}},
		{"Structured errors with codes", []string{"yes", "no", "no", "no", "no"}},
		{"Bulk fetch with bounded concurrency", []string{"yes", "no", "n/a", "n/a", "no"}},
		{"HTTP API and CLI included", []string{"yes", "partial", "no", "no", "no"}},
	}
	for _, s := range static {
		row := FeatureRow{Feature: s.name, Values: map[string]string{}}
		for i, name := range featureLibs {
			row.Values[name] = s.vals[i]
		}
		rows = append(rows, row)
	}
	return rows
}

func libVersions() []LibInfo {
	info, _ := debug.ReadBuildInfo()
	ver := map[string]string{}
	if info != nil {
		for _, d := range info.Deps {
			ver[d.Path] = d.Version
		}
	}
	var out []LibInfo
	seen := map[string]bool{}
	for _, l := range Libs() {
		if seen[l.Module] {
			continue
		}
		seen[l.Module] = true
		v := ver[l.Module]
		if l.Module == "github.com/josephgoksu/metagrab" {
			v = "this repository"
		}
		out = append(out, LibInfo{Name: strings.TrimSuffix(l.Name, " (rich)"), Module: l.Module, Version: v})
	}
	return out
}
