package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	mcpbridge "leiAgent/internal/MCP"

	"go.yaml.in/yaml/v2"
)

type mcpFileRoot struct {
	EnableLLMConfig bool                     `yaml:"enable_llm_config,omitempty"`
	LLM             llmYAML                  `yaml:"llm,omitempty"`
	LLMBackends     []llmYAML                `yaml:"llm_backends,omitempty"`
	MCPServers      []mcpbridge.ServerConfig `yaml:"mcp_servers,omitempty"`
	OpenClaw        openClawYAML             `yaml:"openclaw,omitempty"`
}

type openClawYAML struct {
	Env map[string]string `yaml:"env,omitempty"`
}

type MCPConfigRow struct {
	Label             string               `json:"label"`
	TransportType     string               `json:"transportType"`
	URL               string               `json:"url"`
	Command           string               `json:"command"`
	ArgsText          string               `json:"argsText"`
	AllowedTools      string               `json:"allowedTools"`
	HeadersText       string               `json:"headersText"`
	EnvText           string               `json:"envText"`
	CachedTools       []string             `json:"cachedTools"`
	CachedToolDetails []mcpbridge.ToolInfo `json:"cachedToolDetails"`
	LastCheckState    string               `json:"lastCheckState"`
	LastCheckMessage  string               `json:"lastCheckMessage"`
	LastCheckedAt     string               `json:"lastCheckedAt"`
}

type MCPConfigFormState struct {
	Servers      []MCPConfigRow `json:"servers"`
	Path         string         `json:"path"`
	UsingExample bool           `json:"usingExample"`
}

type MCPValidationResult struct {
	OK             bool                 `json:"ok"`
	Message        string               `json:"message"`
	Tools          []string             `json:"tools"`
	ToolDetails    []mcpbridge.ToolInfo `json:"toolDetails"`
	Label          string               `json:"label"`
	ToolCount      int                  `json:"toolCount"`
	CheckedAt      string               `json:"checkedAt"`
	ConfigValid    bool                 `json:"configValid"`
	LastCheckState string               `json:"lastCheckState"`
	MissingEnvKeys []string             `json:"missingEnvKeys"`
	Warnings       []string             `json:"warnings"`
}

func (r *MCPConfigRow) UnmarshalJSON(data []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	r.Label = jsonStringAny(m, "label", "Label")
	r.TransportType = jsonStringAny(m, "transportType", "transport_type", "TransportType")
	r.URL = jsonStringAny(m, "url", "URL")
	r.Command = jsonStringAny(m, "command", "Command")
	r.ArgsText = jsonStringAny(m, "argsText", "args_text", "ArgsText")
	r.AllowedTools = jsonStringAny(m, "allowedTools", "allowed_tools", "AllowedTools")
	r.HeadersText = jsonStringAny(m, "headersText", "headers_text", "HeadersText")
	r.EnvText = jsonStringAny(m, "envText", "env_text", "EnvText")
	var cached []string
	if raw, ok := m["cachedTools"]; ok && len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &cached)
	}
	r.CachedTools = cached
	if raw, ok := m["cachedToolDetails"]; ok && len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &r.CachedToolDetails)
	}
	r.LastCheckState = jsonStringAny(m, "lastCheckState", "last_check_state", "LastCheckState")
	r.LastCheckMessage = jsonStringAny(m, "lastCheckMessage", "last_check_message", "LastCheckMessage")
	r.LastCheckedAt = jsonStringAny(m, "lastCheckedAt", "last_checked_at", "LastCheckedAt")
	return nil
}

func GetMCPConfigFormState() (MCPConfigFormState, error) {
	content, path, usingExample, err := ReadLLMConfigForUI()
	if err != nil {
		return MCPConfigFormState{}, err
	}

	data := []byte(strings.ReplaceAll(content, "\r\n", "\n"))
	var root mcpFileRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return MCPConfigFormState{}, fmt.Errorf("YAML 解析失败：%w", err)
	}

	rows := make([]MCPConfigRow, 0, len(root.MCPServers))
	for _, server := range root.MCPServers {
		row := rowFromMCPServer(server)
		if cache, err := mcpbridge.ReadToolCache(server.Label); err == nil && cache != nil {
			row.CachedToolDetails = append([]mcpbridge.ToolInfo(nil), cache.Tools...)
			row.CachedTools = toolNamesFromDetails(cache.Tools)
			row.LastCheckState = cacheState(cache)
			row.LastCheckMessage = cache.Message
			row.LastCheckedAt = cache.CheckedAt
		}
		rows = append(rows, row)
	}

	return MCPConfigFormState{
		Servers:      rows,
		Path:         path,
		UsingExample: usingExample,
	}, nil
}

func SaveMCPConfigForm(servers []MCPConfigRow) (savedPath string, err error) {
	content, _, _, err := ReadLLMConfigForUI()
	if err != nil {
		return "", err
	}

	var root mcpFileRoot
	if len(strings.TrimSpace(content)) > 0 {
		data := []byte(strings.ReplaceAll(content, "\r\n", "\n"))
		if err := yaml.Unmarshal(data, &root); err != nil {
			return "", fmt.Errorf("YAML 解析失败：%w", err)
		}
	}

	nextServers := make([]mcpbridge.ServerConfig, 0, len(servers))
	prevLabels := make(map[string]struct{}, len(root.MCPServers))
	for _, server := range root.MCPServers {
		if label := strings.TrimSpace(server.Label); label != "" {
			prevLabels[label] = struct{}{}
		}
	}
	nextLabels := make(map[string]struct{}, len(servers))
	for i, row := range servers {
		server, err := row.toServerConfig()
		if err != nil {
			return "", fmt.Errorf("mcp_servers[%d]: %w", i, err)
		}
		if strings.TrimSpace(server.Label) == "" {
			continue
		}
		nextLabels[server.Label] = struct{}{}
		nextServers = append(nextServers, server)
	}
	root.MCPServers = nextServers
	for label := range prevLabels {
		if _, ok := nextLabels[label]; ok {
			continue
		}
		_ = mcpbridge.DeleteToolCache(label)
	}

	doc, err := parseYAMLDocumentNode(content)
	if err != nil {
		return "", fmt.Errorf("YAML 解析失败：%w", err)
	}

	mcpNode, err := nodeFromValue(nextServers)
	if err != nil {
		return "", fmt.Errorf("生成 mcp_servers 失败：%w", err)
	}
	upsertRootKey(doc, "mcp_servers", mcpNode)

	out, err := marshalYAMLDocumentNode(doc)
	if err != nil {
		return "", fmt.Errorf("YAML 序列化失败：%w", err)
	}
	return writeLLMConfigBytes(out)
}

func rowFromMCPServer(server mcpbridge.ServerConfig) MCPConfigRow {
	return MCPConfigRow{
		Label:         strings.TrimSpace(server.Label),
		TransportType: strings.TrimSpace(server.TransportType),
		URL:           strings.TrimSpace(server.URL),
		Command:       strings.TrimSpace(server.Command),
		ArgsText:      strings.Join(server.Args, "\n"),
		AllowedTools:  strings.Join(server.AllowedTools, "\n"),
		HeadersText:   mapToLines(server.Headers),
		EnvText:       mapToLines(server.Env),
	}
}

func (r MCPConfigRow) toServerConfig() (mcpbridge.ServerConfig, error) {
	headers, err := parseKeyValueLines(r.HeadersText)
	if err != nil {
		return mcpbridge.ServerConfig{}, fmt.Errorf("headers: %w", err)
	}
	envMap, err := parseKeyValueLines(r.EnvText)
	if err != nil {
		return mcpbridge.ServerConfig{}, fmt.Errorf("env: %w", err)
	}

	server := mcpbridge.ServerConfig{
		Label:         strings.TrimSpace(r.Label),
		TransportType: strings.TrimSpace(r.TransportType),
		URL:           strings.TrimSpace(r.URL),
		Command:       strings.TrimSpace(r.Command),
		Args:          parseMultilineList(r.ArgsText),
		AllowedTools:  parseMultilineList(r.AllowedTools),
		Headers:       headers,
		Env:           envMap,
	}
	if server.Label == "" {
		return server, nil
	}
	if server.URL == "" && server.Command == "" {
		return mcpbridge.ServerConfig{}, fmt.Errorf("url 和 command 至少填写一个")
	}
	if server.TransportType == "" {
		if server.Command != "" {
			server.TransportType = "stdio"
		} else {
			server.TransportType = "streamable_http"
		}
	}
	return server, nil
}

func ValidateMCPConfigRow(row MCPConfigRow) (MCPValidationResult, error) {
	cfg, err := row.toServerConfig()
	if err != nil {
		return MCPValidationResult{
			OK:          false,
			Message:     err.Error(),
			Label:       strings.TrimSpace(row.Label),
			ConfigValid: false,
		}, nil
	}

	manager := mcpbridge.NewManager(nil)
	missingEnvKeys := mcpbridge.MissingRequiredEnvKeys(cfg)
	tools, err := manager.ListTools(context.Background(), cfg)
	if err != nil {
		_ = mcpbridge.WriteToolCache(mcpbridge.ToolCache{
			Label:     cfg.Label,
			OK:        false,
			State:     "error",
			Message:   err.Error(),
			CheckedAt: time.Now().Format(time.RFC3339),
		})
		return MCPValidationResult{
			OK:             false,
			Message:        err.Error(),
			Label:          cfg.Label,
			ConfigValid:    true,
			CheckedAt:      time.Now().Format(time.RFC3339),
			LastCheckState: "error",
			MissingEnvKeys: missingEnvKeys,
		}, nil
	}

	names := toolNamesFromDetails(tools)
	checkedAt := time.Now().Format(time.RFC3339)
	if len(missingEnvKeys) > 0 {
		warnings := []string{
			fmt.Sprintf("已发现 %d 个工具，但缺少运行该 MCP 常用的环境变量：%s。tools/list 能通过，真实调用很可能失败。", len(names), strings.Join(missingEnvKeys, ", ")),
		}
		_ = mcpbridge.WriteToolCache(mcpbridge.ToolCache{
			Label:     cfg.Label,
			OK:        false,
			State:     "warning",
			Message:   warnings[0],
			CheckedAt: checkedAt,
			Tools:     tools,
		})
		return MCPValidationResult{
			OK:             false,
			Message:        warnings[0],
			Tools:          names,
			ToolDetails:    tools,
			Label:          cfg.Label,
			ToolCount:      len(names),
			CheckedAt:      checkedAt,
			ConfigValid:    true,
			LastCheckState: "warning",
			MissingEnvKeys: missingEnvKeys,
			Warnings:       warnings,
		}, nil
	}

	_ = mcpbridge.WriteToolCache(mcpbridge.ToolCache{
		Label:     cfg.Label,
		OK:        true,
		State:     "ok",
		Message:   fmt.Sprintf("发现 %d 个工具", len(names)),
		CheckedAt: checkedAt,
		Tools:     tools,
	})
	return MCPValidationResult{
		OK:             true,
		Message:        fmt.Sprintf("发现 %d 个工具", len(names)),
		Tools:          names,
		ToolDetails:    tools,
		Label:          cfg.Label,
		ToolCount:      len(names),
		CheckedAt:      checkedAt,
		ConfigValid:    true,
		LastCheckState: "ok",
	}, nil
}

func cacheState(cache *mcpbridge.ToolCache) string {
	if cache == nil {
		return ""
	}
	if state := strings.TrimSpace(cache.State); state != "" {
		return state
	}
	return ternary(cache.OK, "ok", "error")
}

func toolNamesFromDetails(tools []mcpbridge.ToolInfo) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ternary(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func parseMultilineList(text string) []string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func parseKeyValueLines(text string) (map[string]string, error) {
	lines := parseMultilineList(text)
	if len(lines) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(lines))
	for _, line := range lines {
		idx := strings.Index(line, ":")
		if idx <= 0 {
			return nil, fmt.Errorf("每行需为 key: value，当前为 %q", line)
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key == "" {
			return nil, fmt.Errorf("存在空 key：%q", line)
		}
		out[key] = value
	}
	return out, nil
}

func mapToLines(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	lines := make([]string, 0, len(m))
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m[k]
		lines = append(lines, fmt.Sprintf("%s: %s", k, v))
	}
	return strings.Join(lines, "\n")
}
