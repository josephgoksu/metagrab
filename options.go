package metagrab

import (
	"net/http"
	"time"
)

// Option configures a Client.
type Option func(*Client)

// WithTimeout sets the HTTP client timeout used by the default client transport.
// Ignored when WithHTTPClient is also set.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.timeout = d
	}
}

// WithUserAgent sets the User-Agent header for all requests.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		c.userAgent = ua
	}
}

// WithMaxBodySize sets the maximum response body size in bytes.
func WithMaxBodySize(bytes int64) Option {
	return func(c *Client) {
		c.maxBodySize = bytes
	}
}

// WithConcurrency sets the maximum concurrent fetches for bulk operations.
func WithConcurrency(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.concurrency = n
		}
	}
}

// WithRetries sets max retry attempts for retryable HTTP statuses (429, 502, 503, 504).
// Default is 0 (no retries).
func WithRetries(n int) Option {
	return func(c *Client) {
		if n >= 0 {
			c.maxRetries = n
		}
	}
}

// WithRetryDelay sets base delay used between retries when Retry-After is not present.
func WithRetryDelay(d time.Duration) Option {
	return func(c *Client) {
		c.retryDelay = d
	}
}

// WithURLPolicy sets a URLPolicy hook for allow/deny decisions before fetch.
func WithURLPolicy(policy URLPolicy) Option {
	return func(c *Client) {
		c.urlPolicy = policy
	}
}

// WithHTTPClient sets a custom *http.Client.
// When set, timeout and transport are fully managed by the caller.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}
