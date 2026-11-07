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

// WithMaxRetryWait caps how long the client waits before a retry, including
// waits requested by a Retry-After header. Default is 30s.
func WithMaxRetryWait(d time.Duration) Option {
	return func(c *Client) {
		c.maxRetryWait = d
	}
}

// WithMaxRedirects sets how many redirects the default transport follows.
// Default is 5. Use 0 to not follow redirects (a 3xx is then returned as
// an http_status_error). Ignored when WithHTTPClient is also set.
func WithMaxRedirects(n int) Option {
	return func(c *Client) {
		if n >= 0 {
			c.maxRedirects = n
		}
	}
}

// WithURLPolicy sets a URLPolicy hook for allow/deny decisions before fetch.
// When the client builds its own transport (no WithHTTPClient), the policy
// also runs on every redirect target.
func WithURLPolicy(policy URLPolicy) Option {
	return func(c *Client) {
		c.urlPolicy = policy
	}
}

// WithDenyPrivateNetworks blocks connections to private, loopback,
// link-local and other reserved addresses (see IsPublicAddr). The check runs
// in the dialer after DNS resolution, so it covers redirects and DNS
// rebinding. Use it whenever URLs come from untrusted input.
//
// HTTP_PROXY and HTTPS_PROXY are ignored while it is set, because through a
// proxy the dialer only sees the proxy's address and could not enforce the
// check. If your egress must go through a proxy, enforce the policy there.
//
// Ignored when WithHTTPClient is also set: in that case, configure your own
// transport (for example with a net.Dialer Control function).
func WithDenyPrivateNetworks() Option {
	return func(c *Client) {
		c.denyPrivate = true
	}
}

// WithHTTPClient sets a custom *http.Client.
// When set, timeout, transport and redirect policy are fully managed by the caller.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}
