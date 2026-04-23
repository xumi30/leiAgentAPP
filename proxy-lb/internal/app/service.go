package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"proxy-lb/internal/logging"
)

type backendRuntime struct {
	cfg        BackendConfig
	streamMode streamMode
}

type modelRuntime struct {
	name      string
	strategy  string
	backends  []*backendRuntime
	rrCounter atomic.Uint64
}

type Service struct {
	cfg        *Config
	httpClient *http.Client
	models     map[string]*modelRuntime
	auth       *authStore
	failures   *backendFailureTracker
}

type streamMode int

const (
	streamModeNonStream streamMode = 0
	streamModeStream    streamMode = 1
	streamModeBoth      streamMode = 3
)

type openAIErrorEnvelope struct {
	Error openAIError `json:"error"`
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
}

func NewService(cfg *Config) (*Service, error) {
	auth, err := newAuthStore(cfg.Server.AuthDataPath)
	if err != nil {
		return nil, err
	}
	svc := &Service{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.Server.RequestTimeoutSeconds) * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        200,
				MaxIdleConnsPerHost: 50,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		models: make(map[string]*modelRuntime, len(cfg.Models)),
		auth:   auth,
		failures: newBackendFailureTracker(),
	}

	for _, model := range cfg.Models {
		rt := &modelRuntime{
			name:     model.Name,
			strategy: NormalizeStrategy(model.Strategy),
		}
		for _, backend := range model.Backends {
			if backend.Enabled != nil && !*backend.Enabled {
				continue
			}
			if strings.TrimSpace(backend.Name) == "" {
				backend.Name = defaultBackendName(backend)
			}
			weight := backend.Weight
			if weight <= 0 {
				weight = 1
			}
			sm := parseStreamMode(backend.StreamMode, strings.EqualFold(backend.Provider, "gemini"))
			for i := 0; i < weight; i++ {
				rt.backends = append(rt.backends, &backendRuntime{cfg: backend, streamMode: sm})
			}
		}
		if len(rt.backends) == 0 {
			return nil, fmt.Errorf("model %q has no enabled backends", model.Name)
		}
		logging.Info("model initialized route=%s strategy=%s usable_backends=%d", model.Name, rt.strategy, len(rt.backends))
		svc.models[model.Name] = rt
	}
	return svc, nil
}

func defaultBackendName(cfg BackendConfig) string {
	model := strings.TrimSpace(cfg.Model)
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if model == "" && baseURL == "" {
		return "backend"
	}
	if model == "" {
		return baseURL
	}
	if baseURL == "" {
		return model
	}
	return model + "@" + baseURL
}

func (s *Service) authenticateBearerToken(token string) (*authPrincipal, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}
	if staticToken := strings.TrimSpace(s.cfg.Server.AuthToken); staticToken != "" && token == staticToken {
		logging.Debug("static auth token accepted")
		return &authPrincipal{
			Username: "bootstrap",
			IsStatic: true,
		}, nil
	}
	return s.auth.authenticate(token)
}

func NormalizeStrategy(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "round_robin", "rr":
		return "round_robin"
	default:
		return "round_robin"
	}
}

func parseStreamMode(raw string, geminiDefault bool) streamMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "nonstream", "false", "0", "off":
		return streamModeNonStream
	case "stream", "true", "1", "on":
		return streamModeStream
	case "both":
		return streamModeBoth
	default:
		if geminiDefault {
			return streamModeNonStream
		}
		return streamModeBoth
	}
}

func (s *Service) resolveModel(requestModel string) (*modelRuntime, error) {
	name := strings.TrimSpace(requestModel)
	if name == "" {
		name = strings.TrimSpace(s.cfg.Server.DefaultModel)
	}
	if name == "" {
		if len(s.models) == 1 {
			for _, model := range s.models {
				return model, nil
			}
		}
		return nil, fmt.Errorf("model is required")
	}
	model, ok := s.models[name]
	if !ok {
		return nil, fmt.Errorf("model %q is not configured", name)
	}
	return model, nil
}

func (s *Service) listModels() []ginModel {
	items := make([]ginModel, 0, len(s.models))
	for name := range s.models {
		items = append(items, ginModel{
			ID:      name,
			Object:  "model",
			OwnedBy: "proxy-lb",
		})
	}
	return items
}

func (s *Service) handleChatCompletions(ctx context.Context, req *ChatCompletionRequest) (*upstreamResult, int, error) {
	model, err := s.resolveModel(req.Model)
	if err != nil {
		logging.WarnfCtx(ctx, "chat completion rejected: %v", err)
		return nil, http.StatusBadRequest, err
	}
	logging.InfofCtx(ctx, "chat completion start route_model=%s requested_model=%s stream=%v messages=%d", model.name, strings.TrimSpace(req.Model), req.Stream, len(req.Messages))

	candidates := model.pickBackends()
	candidates = s.failures.reorderCandidates(candidates)
	var lastErr error
	for idx, backend := range candidates {
		logging.InfofCtx(ctx, "attempt backend=%s index=%d/%d upstream_model=%s provider=%s", backend.cfg.Name, idx+1, len(candidates), backend.cfg.Model, backend.cfg.Provider)
		result, err := s.callBackend(ctx, backend, req, model.name)
		if err == nil {
			s.failures.recordSuccess(backendKey(backend.cfg))
			logging.InfofCtx(ctx, "chat completion success backend=%s stream=%v", backend.cfg.Name, result.Stream)
			return result, http.StatusOK, nil
		}
		s.failures.recordFailure(backendKey(backend.cfg))
		lastErr = err
		logging.WarnfCtx(ctx, "backend failed backend=%s err=%s", backend.cfg.Name, loggableBackendError(err))
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no upstream backends available")
	}
	logging.ErrorfCtx(ctx, "all backends failed route_model=%s err=%s", model.name, loggableBackendError(lastErr))
	return nil, http.StatusBadGateway, lastErr
}

type backendFailureState struct {
	consecutiveFailures int
	cooldownUntil       time.Time
}

type backendFailureTracker struct {
	mu   sync.Mutex
	now  func() time.Time
	data map[string]*backendFailureState
}

func newBackendFailureTracker() *backendFailureTracker {
	return &backendFailureTracker{
		now:  time.Now,
		data: make(map[string]*backendFailureState),
	}
}

func backendKey(cfg BackendConfig) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(cfg.APIKey)))
	return strings.TrimSpace(cfg.Model) + "|" + strings.TrimSpace(cfg.BaseURL) + "|" + fmt.Sprintf("%x", sum[:])
}

func (t *backendFailureTracker) getLocked(key string) *backendFailureState {
	st := t.data[key]
	if st == nil {
		st = &backendFailureState{}
		t.data[key] = st
	}
	return st
}

func (t *backendFailureTracker) recordFailure(key string) {
	if key == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.getLocked(key)
	st.consecutiveFailures++
	// Exponential backoff cooldown: 2s, 4s, 8s, ... capped at 60s.
	cooldown := 2 * time.Second
	for i := 1; i < st.consecutiveFailures; i++ {
		cooldown *= 2
		if cooldown >= 60*time.Second {
			cooldown = 60 * time.Second
			break
		}
	}
	st.cooldownUntil = t.now().Add(cooldown)
}

func (t *backendFailureTracker) recordSuccess(key string) {
	if key == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.getLocked(key)
	st.consecutiveFailures = 0
	st.cooldownUntil = time.Time{}
}

func (t *backendFailureTracker) penalized(key string) bool {
	if key == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.data[key]
	if st == nil {
		return false
	}
	return !st.cooldownUntil.IsZero() && t.now().Before(st.cooldownUntil)
}

func (t *backendFailureTracker) reorderCandidates(in []*backendRuntime) []*backendRuntime {
	if len(in) <= 1 {
		return in
	}
	// Stable partition: non-penalized first, penalized last.
	out := append([]*backendRuntime(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		pi := t.penalized(backendKey(out[i].cfg))
		pj := t.penalized(backendKey(out[j].cfg))
		if pi == pj {
			return false
		}
		return !pi && pj
	})
	return out
}

func (m *modelRuntime) pickBackends() []*backendRuntime {
	total := len(m.backends)
	if total <= 1 {
		return append([]*backendRuntime(nil), m.backends...)
	}
	start := int(m.rrCounter.Add(1)-1) % total
	ordered := make([]*backendRuntime, 0, total)
	for i := 0; i < total; i++ {
		ordered = append(ordered, m.backends[(start+i)%total])
	}
	return ordered
}

type upstreamResult struct {
	BackendName string
	Stream      bool
	Response    *http.Response
	OpenAIResp  *ChatCompletionResponse
}

type upstreamStatusError struct {
	Backend    string
	StatusCode int
	Body       string
}

func (e *upstreamStatusError) Error() string {
	if e == nil {
		return ""
	}
	if e.Body == "" {
		return fmt.Sprintf("%s upstream status %d", e.Backend, e.StatusCode)
	}
	return fmt.Sprintf("%s upstream status %d: %s", e.Backend, e.StatusCode, e.Body)
}

func summarizeForLog(limit int, s string) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if limit <= 0 || len(s) <= limit {
		return s
	}
	if limit <= 3 {
		return s[:limit]
	}
	return s[:limit-3] + "..."
}

func loggableBackendError(err error) string {
	var statusErr *upstreamStatusError
	if errors.As(err, &statusErr) {
		summary := summarizeForLog(240, statusErr.Body)
		if summary == "" {
			return fmt.Sprintf("%s upstream status %d", statusErr.Backend, statusErr.StatusCode)
		}
		return fmt.Sprintf("%s upstream status %d body=%q", statusErr.Backend, statusErr.StatusCode, summary)
	}
	return err.Error()
}

func (s *Service) callBackend(ctx context.Context, backend *backendRuntime, req *ChatCompletionRequest, publicModel string) (*upstreamResult, error) {
	// Add lightweight client-side timing so we can prove where latency comes from:
	// DNS / TCP connect / TLS handshake / connection reuse / time-to-first-byte.
	//
	// Logged at DEBUG level to avoid noisy prod logs.
	type traceState struct {
		start              time.Time
		dnsStart           time.Time
		connectStart       time.Time
		tlsStart           time.Time
		gotConnAt          time.Time
		wroteRequestAt     time.Time
		firstResponseByte  time.Time
		dnsDur             time.Duration
		connectDur         time.Duration
		tlsDur             time.Duration
		gotConnInfo        httptrace.GotConnInfo
		remoteAddr         string
	}
	st := &traceState{start: time.Now()}
	trace := &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) { st.dnsStart = time.Now() },
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			if !st.dnsStart.IsZero() {
				st.dnsDur += time.Since(st.dnsStart)
			}
		},
		ConnectStart: func(network, addr string) {
			st.connectStart = time.Now()
			st.remoteAddr = addr
			_ = network
		},
		ConnectDone: func(network, addr string, err error) {
			_ = network
			_ = addr
			if !st.connectStart.IsZero() {
				st.connectDur += time.Since(st.connectStart)
			}
			_ = err
		},
		TLSHandshakeStart: func() { st.tlsStart = time.Now() },
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if !st.tlsStart.IsZero() {
				st.tlsDur += time.Since(st.tlsStart)
			}
			_ = err
		},
		GotConn: func(info httptrace.GotConnInfo) {
			st.gotConnAt = time.Now()
			st.gotConnInfo = info
		},
		WroteRequest: func(_ httptrace.WroteRequestInfo) { st.wroteRequestAt = time.Now() },
		GotFirstResponseByte: func() { st.firstResponseByte = time.Now() },
	}

	// Only attach the trace when debug logging is enabled? We don't currently have a
	// cheap "isDebug" check, so we always attach and only log the results at DEBUG.
	traceCtx := httptrace.WithClientTrace(ctx, trace)

	upstreamReq, err := s.buildUpstreamRequest(traceCtx, backend, req)
	if err != nil {
		return nil, fmt.Errorf("%s build request: %w", backend.cfg.Name, err)
	}

	startDo := time.Now()
	resp, err := s.httpClient.Do(upstreamReq)
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", backend.cfg.Name, err)
	}
	totalDoDur := time.Since(startDo)
	ttfbDur := time.Duration(0)
	if !st.firstResponseByte.IsZero() {
		ttfbDur = st.firstResponseByte.Sub(startDo)
	}
	reused := st.gotConnInfo.Reused
	wasIdle := st.gotConnInfo.WasIdle
	idleFor := st.gotConnInfo.IdleTime
	if st.gotConnInfo.Conn != nil {
		if ra := st.gotConnInfo.Conn.RemoteAddr(); ra != nil {
			st.remoteAddr = ra.String()
		}
	}
	logging.DebugfCtx(ctx,
		"upstream timing backend=%s status=%d do=%s ttfb=%s dns=%s connect=%s tls=%s conn_reused=%v conn_was_idle=%v conn_idle=%s remote=%s",
		backend.cfg.Name,
		resp.StatusCode,
		totalDoDur.Round(time.Millisecond),
		ttfbDur.Round(time.Millisecond),
		st.dnsDur.Round(time.Millisecond),
		st.connectDur.Round(time.Millisecond),
		st.tlsDur.Round(time.Millisecond),
		reused,
		wasIdle,
		idleFor.Round(time.Millisecond),
		strings.TrimSpace(st.remoteAddr),
	)
	logging.InfofCtx(ctx, "upstream responded backend=%s status=%d", backend.cfg.Name, resp.StatusCode)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &upstreamStatusError{
			Backend:    backend.cfg.Name,
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(body)),
		}
	}

	streamRequested := req.Stream && backend.streamMode != streamModeNonStream && !strings.EqualFold(backend.cfg.Provider, "gemini")
	if streamRequested {
		logging.InfofCtx(ctx, "stream passthrough backend=%s", backend.cfg.Name)
		return &upstreamResult{
			BackendName: backend.cfg.Name,
			Stream:      true,
			Response:    resp,
		}, nil
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s read body: %w", backend.cfg.Name, err)
	}

	normalized, err := normalizeResponseBody(body, backend.cfg.Provider)
	if err != nil {
		return nil, fmt.Errorf("%s decode body: %w", backend.cfg.Name, err)
	}
	if normalized.Model == "" {
		normalized.Model = publicModel
	}
	completionCount := 0
	if normalized.Usage != nil {
		completionCount = normalized.Usage.TotalTokens
	}
	logging.InfofCtx(ctx, "non-stream response normalized backend=%s choices=%d total_tokens=%d", backend.cfg.Name, len(normalized.Choices), completionCount)

	return &upstreamResult{
		BackendName: backend.cfg.Name,
		OpenAIResp:  normalized,
	}, nil
}

func (s *Service) buildUpstreamRequest(ctx context.Context, backend *backendRuntime, req *ChatCompletionRequest) (*http.Request, error) {
	payload := *req
	payload.Model = backend.cfg.Model
	if backend.cfg.MaxOutputTokens > 0 {
		maxTok := backend.cfg.MaxOutputTokens
		payload.MaxTokens = &maxTok
	}
	if backend.streamMode == streamModeNonStream {
		payload.Stream = false
		payload.StreamOptions = nil
	}

	var data []byte
	var err error
	if strings.EqualFold(backend.cfg.Provider, "gemini") {
		converted := ConvertFromOpenAIRequest(&payload)
		data, err = json.Marshal(converted)
	} else {
		data, err = json.Marshal(&payload)
	}
	if err != nil {
		return nil, err
	}

	targetURL, err := normalizedBackendURL(backend.cfg)
	if err != nil {
		return nil, err
	}
	logging.InfofCtx(ctx, "build upstream request backend=%s target=%s", backend.cfg.Name, targetURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if strings.EqualFold(backend.cfg.Provider, "gemini") {
		httpReq.Header.Set("x-goog-api-key", backend.cfg.APIKey)
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+backend.cfg.APIKey)
	}
	return httpReq, nil
}

func normalizeResponseBody(body []byte, provider string) (*ChatCompletionResponse, error) {
	if strings.EqualFold(provider, "gemini") {
		var gemResp GeminiChatCompletionResponse
		if err := json.Unmarshal(body, &gemResp); err != nil {
			return nil, err
		}
		return ConvertToOpenAIResponse(&gemResp), nil
	}
	var openAIResp ChatCompletionResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return nil, err
	}
	return &openAIResp, nil
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(openAIErrorEnvelope{
		Error: openAIError{
			Message: msg,
			Type:    http.StatusText(status),
			Code:    fmt.Sprintf("http_%d", status),
		},
	})
}

func streamOpenAIResponse(w http.ResponseWriter, result *upstreamResult) {
	defer result.Response.Body.Close()

	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming unsupported by response writer")
		return
	}

	reader := result.Response.Body
	buf := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			if err == io.EOF {
				return
			}
			return
		}
	}
}

func streamSyntheticResponse(w http.ResponseWriter, resp *ChatCompletionResponse) {
	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming unsupported by response writer")
		return
	}

	id := resp.ID
	if id == "" {
		id = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}
	model := resp.Model
	if model == "" {
		model = "proxy-lb"
	}
	created := resp.Created
	if created == 0 {
		created = time.Now().Unix()
	}

	roleChunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"delta": map[string]interface{}{
					"role": "assistant",
				},
			},
		},
	}
	writeSSEData(w, roleChunk)
	flusher.Flush()

	if len(resp.Choices) > 0 && resp.Choices[0].Message != nil {
		msg := resp.Choices[0].Message
		delta := map[string]interface{}{}
		if content, ok := msg.Content.(string); ok && content != "" {
			delta["content"] = content
		} else if msg.Content != nil {
			delta["content"] = fmt.Sprint(msg.Content)
		}
		if msg.ReasoningContent != "" {
			delta["reasoning_content"] = msg.ReasoningContent
		}
		if len(msg.ToolCalls) > 0 {
			delta["tool_calls"] = msg.ToolCalls
		}
		if len(delta) > 0 {
			contentChunk := map[string]interface{}{
				"id":      id,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": delta,
					},
				},
			}
			writeSSEData(w, contentChunk)
			flusher.Flush()
		}
	}

	finishReason := "stop"
	if len(resp.Choices) > 0 && strings.TrimSpace(resp.Choices[0].FinishReason) != "" {
		finishReason = resp.Choices[0].FinishReason
	}
	finalChunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"delta":         map[string]interface{}{},
				"finish_reason": finishReason,
			},
		},
	}
	if resp.Usage != nil {
		finalChunk["usage"] = resp.Usage
	}
	writeSSEData(w, finalChunk)
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func writeSSEData(w http.ResponseWriter, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		errPayload := map[string]interface{}{
			"error": err.Error(),
		}
		data, _ = json.Marshal(errPayload)
	}
	_, _ = io.WriteString(w, "data: ")
	_, _ = w.Write(data)
	_, _ = io.WriteString(w, "\n\n")
}

type ginModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

type ginModelsResponse struct {
	Object string     `json:"object"`
	Data   []ginModel `json:"data"`
}

type healthResponse struct {
	OK            bool     `json:"ok"`
	DefaultModel  string   `json:"default_model,omitempty"`
	ConfiguredIDs []string `json:"configured_models"`
}

func configuredModelNames(models map[string]*modelRuntime) []string {
	out := make([]string, 0, len(models))
	for name := range models {
		out = append(out, name)
	}
	return out
}

type healthProbe struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (s *Service) checkBackends(ctx context.Context) map[string][]healthProbe {
	type pair struct {
		model string
		item  healthProbe
	}

	out := make(map[string][]healthProbe, len(s.models))
	ch := make(chan pair)
	var wg sync.WaitGroup

	for modelName, model := range s.models {
		seen := make(map[string]struct{})
		for _, backend := range model.backends {
			if _, ok := seen[backend.cfg.Name]; ok {
				continue
			}
			seen[backend.cfg.Name] = struct{}{}

			wg.Add(1)
			go func(modelName string, backend *backendRuntime) {
				defer wg.Done()
				status := "ok"
				errMsg := ""
				reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()

				req, err := http.NewRequestWithContext(reqCtx, http.MethodHead, backend.cfg.BaseURL, nil)
				if err != nil {
					status = "error"
					errMsg = err.Error()
				} else {
					if strings.EqualFold(backend.cfg.Provider, "gemini") {
						req.Header.Set("x-goog-api-key", backend.cfg.APIKey)
					} else {
						req.Header.Set("Authorization", "Bearer "+backend.cfg.APIKey)
					}
					resp, err := s.httpClient.Do(req)
					if err != nil {
						status = "error"
						errMsg = err.Error()
					} else {
						_ = resp.Body.Close()
						if resp.StatusCode >= http.StatusInternalServerError {
							status = "error"
							errMsg = fmt.Sprintf("status %d", resp.StatusCode)
						}
					}
				}
				ch <- pair{model: modelName, item: healthProbe{Name: backend.cfg.Name, Status: status, Error: errMsg}}
			}(modelName, backend)
		}
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for item := range ch {
		out[item.model] = append(out[item.model], item.item)
	}
	return out
}

func normalizedBackendURL(cfg BackendConfig) (string, error) {
	raw := strings.TrimSpace(cfg.BaseURL)
	if raw == "" {
		return "", fmt.Errorf("backend %q missing base_url", cfg.Name)
	}
	if strings.EqualFold(cfg.Provider, "gemini") {
		return raw, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("backend %q invalid base_url: %w", cfg.Name, err)
	}
	lowerPath := strings.ToLower(strings.TrimRight(u.Path, "/"))
	switch {
	case strings.HasSuffix(lowerPath, "/chat/completions"):
		return u.String(), nil
	case lowerPath == "":
		u.Path = "/v1/chat/completions"
	case lowerPath == "/v1":
		u.Path = "/v1/chat/completions"
	default:
		u.Path = path.Join(strings.TrimRight(u.Path, "/"), "chat/completions")
	}
	return u.String(), nil
}
