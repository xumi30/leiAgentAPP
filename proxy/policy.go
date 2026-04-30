package proxy

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryPolicy controls per-backend retry behavior.
// It applies within a single backend before failing over to the next.
type RetryPolicy struct {
	// MaxAttempts is total attempts per backend (>=1). 1 means no retries.
	MaxAttempts int

	// BaseDelay is the initial backoff delay.
	BaseDelay time.Duration

	// MaxDelay caps exponential backoff delay.
	MaxDelay time.Duration

	// JitterRatio adds random jitter: delay *= (1 ± jitter).
	// Recommended range: 0.0 ~ 0.3
	JitterRatio float64
}

func defaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 2,
		BaseDelay:   250 * time.Millisecond,
		MaxDelay:    2 * time.Second,
		JitterRatio: 0.2,
	}
}

func (p RetryPolicy) normalized() RetryPolicy {
	if p.MaxAttempts < 1 {
		p.MaxAttempts = 1
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = 250 * time.Millisecond
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = 2 * time.Second
	}
	if p.MaxDelay < p.BaseDelay {
		p.MaxDelay = p.BaseDelay
	}
	if p.JitterRatio < 0 {
		p.JitterRatio = 0
	}
	if p.JitterRatio > 0.9 {
		p.JitterRatio = 0.9
	}
	return p
}

// CircuitBreakerPolicy is a simple consecutive-failure breaker.
type CircuitBreakerPolicy struct {
	Enabled bool

	// FailuresToOpen opens the circuit after this many consecutive failures.
	FailuresToOpen int

	// CoolDown is how long a circuit stays open before allowing one trial.
	CoolDown time.Duration
}

func defaultBreakerPolicy() CircuitBreakerPolicy {
	return CircuitBreakerPolicy{
		Enabled:        true,
		FailuresToOpen: 3,
		CoolDown:       15 * time.Second,
	}
}

func (p CircuitBreakerPolicy) normalized() CircuitBreakerPolicy {
	if !p.Enabled {
		return p
	}
	if p.FailuresToOpen < 1 {
		p.FailuresToOpen = 1
	}
	if p.CoolDown <= 0 {
		p.CoolDown = 15 * time.Second
	}
	return p
}

type Prober interface {
	Probe(ctx context.Context, b Backend, httpClient *http.Client) error
}

// ModelsProber probes GET /models derived from an OpenAI-compatible chat URL.
// It is intentionally conservative: 404 is treated as success (some gateways omit /models).
type ModelsProber struct {
	Timeout time.Duration
}

func (p ModelsProber) Probe(ctx context.Context, b Backend, httpClient *http.Client) error {
	if strings.EqualFold(strings.TrimSpace(b.Provider), "gemini") {
		return nil
	}
	modelsURL, ok := chatURLToModelsURL(b.BaseURL)
	if !ok {
		return nil
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.APIKey)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNotFound:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("probe auth failed")
	default:
		return errors.New("probe http status " + strconv.Itoa(resp.StatusCode))
	}
}

func chatURLToModelsURL(chatURL string) (string, bool) {
	u := strings.TrimSpace(chatURL)
	if u == "" {
		return "", false
	}
	lower := strings.ToLower(u)
	switch {
	case strings.HasSuffix(lower, "/v1/chat/completions"):
		return u[:len(u)-len("/v1/chat/completions")] + "/v1/models", true
	case strings.HasSuffix(lower, "/chat/completions"):
		return u[:len(u)-len("/chat/completions")] + "/models", true
	default:
		return "", false
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func backoffDelay(p RetryPolicy, attempt int) time.Duration {
	// attempt: 1..MaxAttempts ; backoff applies before attempt>1
	if attempt <= 1 {
		return 0
	}
	exp := float64(attempt - 2) // 0 for 2nd attempt
	base := float64(p.BaseDelay)
	delay := base * math.Pow(2, exp)
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}

	j := p.JitterRatio
	if j <= 0 {
		return time.Duration(delay)
	}
	// random in [-j, +j]
	r := cryptoRandFloat64()
	factor := 1 + (2*r-1)*j
	if factor < 0 {
		factor = 0
	}
	return time.Duration(delay * factor)
}

func cryptoRandFloat64() float64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		u := binary.LittleEndian.Uint64(b[:])
		// map to [0,1)
		return float64(u>>11) / (1 << 53)
	}
	// fallback: deterministic-ish but safe
	return 0.5
}

func isRetriableHTTPStatus(st int) bool {
	if st == http.StatusTooManyRequests {
		return true
	}
	return st >= 500 && st <= 599
}

func isRetriableError(err error) bool {
	if err == nil {
		return false
	}
	// context cancellation shouldn't be retried
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) {
		// timeouts/temporary are usually safe to retry
		return ne.Timeout() || ne.Temporary()
	}
	// best-effort: some transports wrap net.OpError
	var oe *net.OpError
	if errors.As(err, &oe) {
		return true
	}
	return false
}
