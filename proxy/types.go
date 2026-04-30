package proxy

import "time"

const (
	StreamModeNonStream = 0 // only non-stream
	StreamModeStream    = 1 // only stream
	StreamModeBoth      = 3 // decided by request/context
)

// Backend is one LLM endpoint. When multiple backends exist, they are tried
// in order (failover) by DoRequest.
type Backend struct {
	Name string

	Provider string // "gemini" uses x-goog-api-key, others use Bearer
	BaseURL  string // full chat completion URL
	Model    string
	APIKey   string

	StreamMode      int
	MaxOutputTokens int // optional upper bound baseline, not enforced by DoRequest
}

// Config is the resolved configuration used by Client.
type Config struct {
	Backends []Backend
	// SourcePath is the resolved config file path (config.yaml).
	SourcePath string
}

// Request is the minimal unit DoRequest can send.
// BodyBytes is used (instead of io.Reader) to make retries safe.
type Request struct {
	BodyBytes   []byte
	ContentType string        // default "application/json"
	Timeout     time.Duration // optional per-call override; 0 uses client's timeout
}
