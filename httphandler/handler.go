// Package httphandler provides a thin net/http adapter around metagrab.Client.
//
// It exposes POST /fetch, POST /fetch-bulk, and GET /health endpoints with
// a flat JSON wire format (Link objects at the top level).
//
// Zero external dependencies — only net/http, encoding/json, and the parent
// metagrab package.
package httphandler

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/josephgoksu/metagrab"
)

const (
	defaultBodyLimit   int64 = 64 * 1024 // 64 KB
	defaultMaxBulkURLs       = 100
)

// Handler is an http.Handler that delegates to a metagrab.Client.
type Handler struct {
	client      *metagrab.Client
	apiKey      string
	bodyLimit   int64
	maxBulkURLs int
	mux         *http.ServeMux
}

// HandlerOption configures a Handler.
type HandlerOption func(*Handler)

// WithAPIKey enables X-API-Key authentication. When set, every request must
// include a matching header. When empty, auth is disabled.
func WithAPIKey(key string) HandlerOption {
	return func(h *Handler) { h.apiKey = key }
}

// WithRequestBodyLimit sets the maximum request body size in bytes.
// Default is 64 KB.
func WithRequestBodyLimit(n int64) HandlerOption {
	return func(h *Handler) {
		if n > 0 {
			h.bodyLimit = n
		}
	}
}

// WithMaxBulkURLs sets the maximum number of URLs allowed in a /fetch-bulk
// request. Default is 100.
func WithMaxBulkURLs(n int) HandlerOption {
	return func(h *Handler) {
		if n > 0 {
			h.maxBulkURLs = n
		}
	}
}

// New creates a Handler wrapping the given metagrab.Client.
func New(client *metagrab.Client, opts ...HandlerOption) *Handler {
	h := &Handler{
		client:      client,
		bodyLimit:   defaultBodyLimit,
		maxBulkURLs: defaultMaxBulkURLs,
	}
	for _, o := range opts {
		o(h)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /fetch", h.handleFetch)
	mux.HandleFunc("POST /fetch-bulk", h.handleFetchBulk)
	mux.HandleFunc("GET /health", h.handleHealth)
	h.mux = mux
	return h
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// /health stays open so load balancers and uptime checks work
	// without the key. It reveals nothing beyond "the process is up".
	if h.apiKey != "" && r.URL.Path != "/health" && !h.checkAuth(r) {
		writeJSON(w, http.StatusUnauthorized, errorResponse{
			Code:    "unauthorized",
			Message: "missing or invalid X-API-Key",
		})
		return
	}
	h.mux.ServeHTTP(w, r)
}

// ---------- request/response types ----------

type fetchRequest struct {
	URL    string         `json:"url"`
	Fields metagrab.Field `json:"fields"`
}

type fetchBulkRequest struct {
	URLs   []string       `json:"urls"`
	Fields metagrab.Field `json:"fields"`
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type healthResponse struct {
	Status string `json:"status"`
}

// ---------- handlers ----------

func (h *Handler) handleFetch(w http.ResponseWriter, r *http.Request) {
	var req fetchRequest
	if err := h.readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Code:    "bad_request",
			Message: err.Error(),
		})
		return
	}
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Code:    "bad_request",
			Message: "url is required",
		})
		return
	}

	mask := req.Fields
	if mask == 0 {
		mask = metagrab.MetadataFields
	}

	link, err := h.client.Fetch(r.Context(), req.URL, mask)
	if err != nil {
		var fe *metagrab.FetchError
		if errors.As(err, &fe) {
			writeJSON(w, httpStatusForError(fe), fe)
		} else {
			writeJSON(w, http.StatusInternalServerError, errorResponse{
				Code:    "internal_error",
				Message: "unexpected error",
			})
		}
		return
	}

	writeJSON(w, http.StatusOK, link)
}

func (h *Handler) handleFetchBulk(w http.ResponseWriter, r *http.Request) {
	var req fetchBulkRequest
	if err := h.readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Code:    "bad_request",
			Message: err.Error(),
		})
		return
	}
	if len(req.URLs) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Code:    "bad_request",
			Message: "urls is required and must not be empty",
		})
		return
	}
	if len(req.URLs) > h.maxBulkURLs {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Code:    "bad_request",
			Message: fmt.Sprintf("urls exceeds maximum of %d", h.maxBulkURLs),
		})
		return
	}

	mask := req.Fields
	if mask == 0 {
		mask = metagrab.MetadataFields
	}

	// Return flat []Link for wire-compatibility with common consumer patterns.
	// Failed URLs are included with only the URL field populated.
	results := h.client.FetchBulkResults(r.Context(), req.URLs, mask)
	links := make([]*metagrab.Link, len(results))
	for i, r := range results {
		if r.Link != nil {
			links[i] = r.Link
		} else {
			links[i] = &metagrab.Link{URL: r.URL}
		}
	}
	writeJSON(w, http.StatusOK, links)
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// ---------- helpers ----------

func (h *Handler) checkAuth(r *http.Request) bool {
	got := r.Header.Get("X-API-Key")
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.apiKey)) == 1
}

func (h *Handler) readJSON(r *http.Request, dst any) error {
	return json.NewDecoder(io.LimitReader(r.Body, h.bodyLimit)).Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func httpStatusForError(fe *metagrab.FetchError) int {
	switch fe.Code {
	case metagrab.ErrorCodeInvalidURL, metagrab.ErrorCodeURLDenied:
		return http.StatusBadRequest
	case metagrab.ErrorCodeCanceled, metagrab.ErrorCodeDeadlineExceeded:
		return http.StatusGatewayTimeout
	case metagrab.ErrorCodeContentType:
		return http.StatusUnprocessableEntity
	default:
		// Upstream failures (including non-2xx upstream statuses) map to
		// 502. The upstream status stays in the body as status_code, so a
		// 401 or 404 from the target is never confused with this API's own.
		return http.StatusBadGateway
	}
}
