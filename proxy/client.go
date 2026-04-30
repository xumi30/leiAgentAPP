package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client is a multi-backend HTTP requester with ordered failover.
// It is intentionally "thin": it does not build JSON payloads; it only
// loads config and sends requests.
type Client struct {
	httpClient *http.Client
	backends   []Backend
	sourcePath string

	retry   RetryPolicy
	breaker CircuitBreakerPolicy
	prober  Prober

	mu     sync.Mutex
	states []backendState
}

type Option func(*clientOptions)

type clientOptions struct {
	httpClient *http.Client
	retry      *RetryPolicy
	breaker    *CircuitBreakerPolicy
	prober     Prober
}

// WithHTTPClient injects a custom http.Client (timeouts, proxy, transport).
func WithHTTPClient(c *http.Client) Option {
	return func(o *clientOptions) {
		o.httpClient = c
	}
}

// WithRetryPolicy enables per-backend retries with backoff.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(o *clientOptions) {
		o.retry = &p
	}
}

// WithCircuitBreaker enables skipping unhealthy backends.
func WithCircuitBreaker(p CircuitBreakerPolicy) Option {
	return func(o *clientOptions) {
		o.breaker = &p
	}
}

// WithProber enables optional probing before attempting an open backend.
// This is most useful together with a circuit breaker.
func WithProber(p Prober) Option {
	return func(o *clientOptions) {
		o.prober = p
	}
}

// New creates a Client by loading configuration from disk/env with fallback.
func New(opts ...Option) (*Client, error) {
	var o clientOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	if len(cfg.Backends) == 0 {
		return nil, errors.New("no backends loaded")
	}
	hc := o.httpClient
	if hc == nil {
		hc = &http.Client{
			Timeout: 300 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	}

	retry := defaultRetryPolicy()
	if o.retry != nil {
		retry = o.retry.normalized()
	} else {
		retry = retry.normalized()
	}
	breaker := defaultBreakerPolicy()
	if o.breaker != nil {
		breaker = o.breaker.normalized()
	} else {
		breaker = breaker.normalized()
	}

	return &Client{
		httpClient: hc,
		backends:   cfg.Backends,
		sourcePath: cfg.SourcePath,
		retry:      retry,
		breaker:    breaker,
		prober:     o.prober,
		states:     make([]backendState, len(cfg.Backends)),
	}, nil
}

// BackendError carries structured information for observability.
type BackendError struct {
	Backend Backend
	// Stage is one of: "build_request", "http_do", "http_status".
	Stage       string
	HTTPStatus  int
	BodySnippet string
	Cause       error
}

func (e *BackendError) Error() string {
	label := e.Backend.Name
	if strings.TrimSpace(label) == "" {
		label = e.Backend.Model
	}
	switch e.Stage {
	case "http_status":
		if e.BodySnippet != "" {
			return fmt.Sprintf("backend %s returned HTTP %d: %s", label, e.HTTPStatus, e.BodySnippet)
		}
		return fmt.Sprintf("backend %s returned HTTP %d", label, e.HTTPStatus)
	default:
		return fmt.Sprintf("backend %s %s failed: %v", label, e.Stage, e.Cause)
	}
}

func (e *BackendError) Unwrap() error { return e.Cause }

type backendState struct {
	consecutiveFailures int
	openUntil           time.Time
	halfOpenInFlight    bool
}

func (c *Client) backendAllowedLocked(i int, now time.Time) bool {
	if !c.breaker.Enabled {
		return true
	}
	st := c.states[i]
	if st.openUntil.IsZero() {
		return true
	}
	// still open
	if now.Before(st.openUntil) {
		return false
	}
	// cool down passed: allow a single half-open trial at a time
	if st.halfOpenInFlight {
		return false
	}
	st.halfOpenInFlight = true
	st.openUntil = time.Time{}
	c.states[i] = st
	return true
}

func (c *Client) backendDoneLocked(i int, ok bool, now time.Time) {
	if !c.breaker.Enabled {
		return
	}
	st := c.states[i]
	st.halfOpenInFlight = false
	if ok {
		st.consecutiveFailures = 0
		st.openUntil = time.Time{}
		c.states[i] = st
		return
	}
	st.consecutiveFailures++
	if st.consecutiveFailures >= c.breaker.FailuresToOpen {
		st.openUntil = now.Add(c.breaker.CoolDown)
	}
	c.states[i] = st
}

// DoRequest sends a POST request with automatic ordered failover.
//
// It returns the first successful HTTP 200 response.
// The caller must close the returned response body.
func (c *Client) DoRequest(ctx context.Context, req Request) (*http.Response, Backend, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(c.backends) == 0 {
		return nil, Backend{}, errors.New("client has no backends")
	}
	contentType := strings.TrimSpace(req.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}

	var lastErr error
	now := time.Now()
	for i, b := range c.backends {
		// breaker: skip open backends
		c.mu.Lock()
		allowed := c.backendAllowedLocked(i, now)
		c.mu.Unlock()
		if !allowed {
			continue
		}

		// optional probe (best-effort). If probe fails, count as a failure and continue.
		if c.prober != nil {
			if err := c.prober.Probe(ctx, b, c.httpClient); err != nil {
				c.mu.Lock()
				c.backendDoneLocked(i, false, time.Now())
				c.mu.Unlock()
				lastErr = &BackendError{Backend: b, Stage: "probe", Cause: err}
				continue
			}
		}

		// retry within this backend
		var backendOK bool
		for attempt := 1; attempt <= c.retry.MaxAttempts; attempt++ {
			if attempt > 1 {
				if err := sleepWithContext(ctx, backoffDelay(c.retry, attempt)); err != nil {
					lastErr = err
					break
				}
			}

			rctx := ctx
			var cancel context.CancelFunc
			if req.Timeout > 0 {
				rctx, cancel = context.WithTimeout(ctx, req.Timeout)
			}

			httpReq, err := http.NewRequestWithContext(rctx, http.MethodPost, b.BaseURL, bytes.NewReader(req.BodyBytes))
			if err != nil {
				if cancel != nil {
					cancel()
				}
				lastErr = &BackendError{Backend: b, Stage: "build_request", Cause: err}
				backendOK = false
				break
			}

			switch strings.ToLower(strings.TrimSpace(b.Provider)) {
			case "gemini":
				httpReq.Header.Set("x-goog-api-key", b.APIKey)
			default:
				httpReq.Header.Set("Authorization", "Bearer "+b.APIKey)
			}
			httpReq.Header.Set("Content-Type", contentType)

			resp, err := c.httpClient.Do(httpReq)
			if cancel != nil {
				cancel()
			}
			if err != nil {
				lastErr = &BackendError{Backend: b, Stage: "http_do", Cause: err}
				backendOK = false
				if attempt < c.retry.MaxAttempts && isRetriableError(err) {
					continue
				}
				break
			}

			if resp.StatusCode != http.StatusOK {
				snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
				_ = resp.Body.Close()
				msg := strings.TrimSpace(string(snippet))
				lastErr = &BackendError{
					Backend:     b,
					Stage:       "http_status",
					HTTPStatus:  resp.StatusCode,
					BodySnippet: msg,
					Cause:       fmt.Errorf("non-200 status"),
				}
				backendOK = false
				if attempt < c.retry.MaxAttempts && isRetriableHTTPStatus(resp.StatusCode) {
					continue
				}
				break
			}

			backendOK = true
			c.mu.Lock()
			c.backendDoneLocked(i, true, time.Now())
			c.mu.Unlock()
			return resp, b, nil
		}

		c.mu.Lock()
		c.backendDoneLocked(i, backendOK, time.Now())
		c.mu.Unlock()
		continue
	}

	if lastErr != nil {
		return nil, Backend{}, fmt.Errorf("all backends failed (%d): %w", len(c.backends), lastErr)
	}
	return nil, Backend{}, fmt.Errorf("all backends failed (%d)", len(c.backends))
}
