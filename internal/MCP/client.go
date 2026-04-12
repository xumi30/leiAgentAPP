package mcpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"leiAgent/logging"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Manager struct {
	httpClient *http.Client
}

type ToolInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
}

type ToolCallResult struct {
	ServerLabel string                 `json:"server_label"`
	Name        string                 `json:"name"`
	IsError     bool                   `json:"is_error,omitempty"`
	Content     interface{}            `json:"content,omitempty"`
	Structured  map[string]interface{} `json:"structured_content,omitempty"`
	Raw         map[string]interface{} `json:"raw,omitempty"`
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  interface{}     `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewManager(httpClient *http.Client) *Manager {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Manager{httpClient: httpClient}
}

func (m *Manager) ListTools(ctx context.Context, cfg ServerConfig) ([]ToolInfo, error) {
	if err := m.EnsureReady(ctx, cfg); err != nil {
		return nil, err
	}

	if strings.TrimSpace(cfg.Command) != "" {
		session, err := newStdioSession(ctx, cfg)
		if err != nil {
			return nil, err
		}
		defer session.Close()

		var payload map[string]interface{}
		if err := session.request(ctx, "tools/list", map[string]interface{}{}, &payload); err != nil {
			return nil, err
		}
		logging.Info("MCP tools/list payload: server=%s payload=%v", cfg.Label, payload)
		return parseToolList(payload), nil
	}

	var payload map[string]interface{}
	if err := m.call(ctx, cfg, "tools/list", map[string]interface{}{}, &payload); err != nil {
		return nil, err
	}
	logging.Info("MCP http tools/list payload: server=%s payload=%v", cfg.Label, payload)
	return parseToolList(payload), nil
}

func (m *Manager) CallTool(ctx context.Context, cfg ServerConfig, toolName string, arguments map[string]interface{}) (*ToolCallResult, error) {
	if !toolAllowed(cfg.AllowedTools, toolName) {
		return nil, fmt.Errorf("tool %q is not allowed by MCP server policy", toolName)
	}
	if err := m.EnsureReady(ctx, cfg); err != nil {
		return nil, err
	}

	if strings.TrimSpace(cfg.Command) != "" {
		session, err := newStdioSession(ctx, cfg)
		if err != nil {
			return nil, err
		}
		defer session.Close()

		var payload map[string]interface{}
		if err := session.request(ctx, "tools/call", map[string]interface{}{
			"name":      toolName,
			"arguments": arguments,
		}, &payload); err != nil {
			return nil, err
		}
		logging.Info("MCP tools/call payload: server=%s tool=%s payload=%v", cfg.Label, toolName, payload)
		return parseToolCallResult(cfg, toolName, payload), nil
	}

	var payload map[string]interface{}
	if err := m.call(ctx, cfg, "tools/call", map[string]interface{}{
		"name":      toolName,
		"arguments": arguments,
	}, &payload); err != nil {
		return nil, err
	}
	logging.Info("MCP http tools/call payload: server=%s tool=%s payload=%v", cfg.Label, toolName, payload)

	return parseToolCallResult(cfg, toolName, payload), nil
}

func (m *Manager) call(ctx context.Context, cfg ServerConfig, method string, params map[string]interface{}, out *map[string]interface{}) error {
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("mcp server url is empty")
	}

	if err := m.callJSONRPC(ctx, cfg, method, params, out); err == nil {
		return nil
	}

	return m.callREST(ctx, cfg, method, params, out)
}

func (m *Manager) callJSONRPC(ctx context.Context, cfg ServerConfig, method string, params map[string]interface{}, out *map[string]interface{}) error {
	reqBody := rpcEnvelope{
		JSONRPC: "2.0",
		ID:      fmt.Sprintf("leiagent-%d", time.Now().UnixNano()),
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	applyHeaders(req, cfg.Headers)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("mcp json-rpc request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var envelope rpcEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("invalid mcp json-rpc response: %w", err)
	}
	if envelope.Error != nil {
		return fmt.Errorf("mcp error %d: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if len(envelope.Result) == 0 {
		*out = map[string]interface{}{}
		return nil
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return fmt.Errorf("invalid mcp result payload: %w", err)
	}
	return nil
}

func (m *Manager) callREST(ctx context.Context, cfg ServerConfig, method string, params map[string]interface{}, out *map[string]interface{}) error {
	target, err := joinMethodURL(cfg.URL, method)
	if err != nil {
		return err
	}
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	applyHeaders(req, cfg.Headers)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("mcp rest request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		*out = map[string]interface{}{}
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("invalid mcp rest response: %w", err)
	}
	return nil
}

func joinMethodURL(baseURL, method string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(method, "/")
	return u.String(), nil
}

func applyHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
}

func toolAllowed(allowed []string, name string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		if strings.TrimSpace(item) == strings.TrimSpace(name) {
			return true
		}
	}
	return false
}

func parseToolList(payload map[string]interface{}) []ToolInfo {
	rawTools, _ := payload["tools"].([]interface{})
	tools := make([]ToolInfo, 0, len(rawTools))
	for _, raw := range rawTools {
		obj, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		item := ToolInfo{}
		if v, ok := obj["name"].(string); ok {
			item.Name = v
		}
		if v, ok := obj["description"].(string); ok {
			item.Description = v
		}
		if schema, ok := obj["inputSchema"].(map[string]interface{}); ok {
			item.InputSchema = schema
		} else if schema, ok := obj["input_schema"].(map[string]interface{}); ok {
			item.InputSchema = schema
		}
		if item.Name != "" {
			tools = append(tools, item)
		}
	}
	return tools
}

func parseToolCallResult(cfg ServerConfig, toolName string, payload map[string]interface{}) *ToolCallResult {
	res := &ToolCallResult{
		ServerLabel: cfg.Label,
		Name:        toolName,
		Raw:         payload,
	}
	if isErr, ok := payload["isError"].(bool); ok {
		res.IsError = isErr
	} else if isErr, ok := payload["is_error"].(bool); ok {
		res.IsError = isErr
	}
	if content, ok := payload["content"]; ok {
		res.Content = content
	}
	if structured, ok := payload["structuredContent"].(map[string]interface{}); ok {
		res.Structured = structured
	} else if structured, ok := payload["structured_content"].(map[string]interface{}); ok {
		res.Structured = structured
	}
	return res
}
