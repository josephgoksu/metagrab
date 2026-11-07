// Command bench compares metagrab with other Go link-preview and Open Graph
// libraries on a deterministic, offline corpus, and writes the results and
// charts to ../docs/benchmarks.
//
//	cd bench && go run .            # full run (a few minutes)
//	cd bench && go run . -quick     # one sample per case
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Measurement is the median of several testing.Benchmark samples.
type Measurement struct {
	Lib       string  `json:"lib"`
	Page      string  `json:"page"`
	Mode      string  `json:"mode"` // "parse" or "fetch"
	NsPerOp   float64 `json:"ns_per_op"`
	BytesOp   int64   `json:"alloc_bytes_per_op"`
	AllocsOp  int64   `json:"allocs_per_op"`
	ReadPerOp int64   `json:"body_bytes_read_per_op,omitempty"`
	Error     string  `json:"error,omitempty"`
}

type Report struct {
	Date      string        `json:"date"`
	GoVersion string        `json:"go_version"`
	OS        string        `json:"os"`
	CPU       string        `json:"cpu"`
	Samples   int           `json:"samples"`
	Pages     []PageInfo    `json:"pages"`
	Results   []Measurement `json:"results"`
	Features  []FeatureRow  `json:"features"`
	Libs      []LibInfo     `json:"libs"`
}

type PageInfo struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Bytes    int    `json:"bytes"`
	HeadSize int    `json:"head_bytes"`
}

type LibInfo struct {
	Name    string `json:"name"`
	Module  string `json:"module"`
	Version string `json:"version"`
}

// countingTransport counts response body bytes the library actually read.
type countingTransport struct {
	base http.RoundTripper
	n    atomic.Int64
}

type countingBody struct {
	io.ReadCloser
	n *atomic.Int64
}

func (b countingBody) Read(p []byte) (int, error) {
	k, err := b.ReadCloser.Read(p)
	b.n.Add(int64(k))
	return k, err
}

func (t *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(r)
	if err == nil {
		resp.Body = countingBody{resp.Body, &t.n}
	}
	return resp, err
}

func median(xs []float64) float64 {
	sort.Float64s(xs)
	return xs[len(xs)/2]
}

func measure(samples int, fn func() error) (Measurement, error) {
	var ns []float64
	var m Measurement
	if err := fn(); err != nil { // warm-up and error check
		return m, err
	}
	for i := 0; i < samples; i++ {
		r := testing.Benchmark(func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := fn(); err != nil {
					b.Fatal(err)
				}
			}
		})
		ns = append(ns, float64(r.T.Nanoseconds())/float64(r.N))
		m.BytesOp, m.AllocsOp = r.AllocedBytesPerOp(), r.AllocsPerOp()
	}
	m.NsPerOp = median(ns)
	return m, nil
}

func cpuModel() string {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "model name") {
				return strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			}
		}
	}
	return runtime.GOARCH
}

func main() {
	quick := flag.Bool("quick", false, "one sample per case")
	out := flag.String("out", "../docs/benchmarks", "output directory")
	flag.Parse()
	samples := 5
	if *quick {
		samples = 1
	}

	pages := Corpus()
	served := map[string][]byte{} // filled before the first request
	srv := httptest.NewServer(fixtureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, ok := served[r.URL.Path]; ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()
	for _, p := range pages {
		served["/"+p.Name] = p.WithBase(srv.URL)
	}

	rep := Report{
		Date:      time.Now().UTC().Format("2006-01-02"),
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS + "/" + runtime.GOARCH,
		CPU:       cpuModel(),
		Samples:   samples,
		Libs:      libVersions(),
	}
	for _, p := range pages {
		rep.Pages = append(rep.Pages, PageInfo{p.Name, p.Label, len(p.HTML), p.HeadSize})
	}

	for _, lib := range Libs() {
		for _, p := range pages {
			if lib.Parse != nil {
				const base = "https://example.com"
				u := base + "/" + p.Name
				html := p.WithBase(base)
				m, err := measure(samples, func() error { _, err := lib.Parse(reader(html), u); return err })
				m.Lib, m.Page, m.Mode = lib.Name, p.Name, "parse"
				if err != nil {
					m.Error = err.Error()
				}
				rep.Results = append(rep.Results, m)
				log.Printf("parse %-22s %-7s %10.0f ns/op %9d B/op", lib.Name, p.Name, m.NsPerOp, m.BytesOp)
			}

			ct := &countingTransport{base: &http.Transport{MaxIdleConnsPerHost: 16}}
			hc := &http.Client{Transport: ct, Timeout: 30 * time.Second}
			u := srv.URL + "/" + p.Name
			var ops atomic.Int64
			m, err := measure(samples, func() error { ops.Add(1); _, err := lib.Fetch(hc, u); return err })
			m.Lib, m.Page, m.Mode = lib.Name, p.Name, "fetch"
			if err != nil {
				m.Error = err.Error()
			}
			if n := ops.Load(); n > 0 {
				m.ReadPerOp = ct.n.Load() / n
			}
			rep.Results = append(rep.Results, m)
			log.Printf("fetch %-22s %-7s %10.0f ns/op %9d B read/op", lib.Name, p.Name, m.NsPerOp, m.ReadPerOp)
		}
	}

	rep.Features = Features(srv.URL)

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	js, _ := json.MarshalIndent(rep, "", "  ")
	must(os.WriteFile(filepath.Join(*out, "results.json"), append(js, '\n'), 0o644))
	must(writeCharts(rep, *out))
	must(os.WriteFile(filepath.Join(*out, "README.md"), []byte(markdown(rep)), 0o644))
	fmt.Println("wrote", *out)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
