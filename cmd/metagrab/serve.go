package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/josephgoksu/metagrab"
	"github.com/josephgoksu/metagrab/httphandler"
)

type serveConfig struct {
	addr          string
	apiKey        string
	timeout       time.Duration
	retries       int
	maxBulk       int
	allowPrivate  bool
	shutdownGrace time.Duration
}

// proxyFromEnv reports whether an HTTP(S) proxy is configured in the
// environment.
func proxyFromEnv() bool {
	return envOr("", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy") != ""
}

// envOr returns the first non-empty environment variable, or def.
func envOr(def string, keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return def
}

func parseServeFlags(args []string, stderr io.Writer) (serveConfig, error) {
	var cfg serveConfig

	defAddr := envOr(":8080", "METAGRAB_ADDR")
	if port := os.Getenv("PORT"); port != "" && os.Getenv("METAGRAB_ADDR") == "" {
		defAddr = ":" + port // Cloud Run, Fly.io, Render, Heroku
	}

	fs := flag.NewFlagSet("metagrab serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.addr, "addr", defAddr, "listen address (env METAGRAB_ADDR, or PORT)")
	fs.StringVar(&cfg.apiKey, "api-key", "", "require this X-API-Key on every request except /health (env METAGRAB_API_KEY)")
	fs.DurationVar(&cfg.timeout, "timeout", 10*time.Second, "upstream fetch timeout")
	fs.IntVar(&cfg.retries, "retries", 1, "retry attempts for retryable upstream statuses")
	fs.IntVar(&cfg.maxBulk, "max-bulk", 100, "max URLs per /fetch-bulk request")
	fs.BoolVar(&cfg.allowPrivate, "allow-private-networks", false, "allow fetching private/loopback addresses (unsafe for public services)")
	fs.DurationVar(&cfg.shutdownGrace, "shutdown-grace", 15*time.Second, "time to finish in-flight requests on SIGTERM")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: metagrab serve [flags]\n\nEndpoints: POST /fetch, POST /fetch-bulk, GET /health\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() > 0 {
		return cfg, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if cfg.apiKey == "" {
		cfg.apiKey = os.Getenv("METAGRAB_API_KEY")
	}
	return cfg, nil
}

func newServer(cfg serveConfig) *http.Server {
	opts := []metagrab.Option{
		metagrab.WithTimeout(cfg.timeout),
		metagrab.WithRetries(cfg.retries),
	}
	if !cfg.allowPrivate {
		opts = append(opts, metagrab.WithDenyPrivateNetworks())
	}
	client := metagrab.NewClient(opts...)

	h := httphandler.New(client,
		httphandler.WithAPIKey(cfg.apiKey),
		httphandler.WithMaxBulkURLs(cfg.maxBulk),
	)

	// Bulk requests fetch many URLs; leave room above the fetch timeout.
	writeTimeout := cfg.timeout*time.Duration(max(cfg.retries+1, 1))*4 + 10*time.Second
	// A bulk request of slow URLs can take longer than writeTimeout. Cancel
	// its fetches a little before the server's write deadline, so the client
	// still gets a response (with the unfinished URLs failed) instead of a
	// dropped connection.
	handlerBudget := writeTimeout - 5*time.Second
	bounded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), handlerBudget)
		defer cancel()
		h.ServeHTTP(w, r.WithContext(ctx))
	})

	return &http.Server{
		Addr:              cfg.addr,
		Handler:           bounded,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       60 * time.Second,
	}
}

func runServe(ctx context.Context, args []string, stderr io.Writer) int {
	cfg, err := parseServeFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		fmt.Fprintln(stderr, "error:", err)
		return exitUsage
	}

	log := slog.New(slog.NewJSONHandler(stderr, nil))
	srv := newServer(cfg)

	ln, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		log.Error("listen failed", "addr", cfg.addr, "err", err)
		return exitInternal
	}
	if cfg.apiKey == "" {
		log.Warn("no API key set: the API is open to anyone who can reach it")
	}
	if cfg.allowPrivate {
		log.Warn("private networks allowed: do not expose this server publicly")
	} else if proxyFromEnv() {
		log.Warn("HTTP_PROXY/HTTPS_PROXY is ignored while private networks are denied; fetches connect directly")
	}
	log.Info("metagrab listening", "addr", ln.Addr().String(), "version", versionString())

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "err", err)
			return exitInternal
		}
	case <-ctx.Done():
		log.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), cfg.shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			log.Error("shutdown failed", "err", err)
			return exitInternal
		}
	}
	return exitOK
}
