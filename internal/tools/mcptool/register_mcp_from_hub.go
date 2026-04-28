package mcptool

import (
	"context"
	"encoding/json"
	"fmt"
	mcpbridge "leiAgent/internal/MCP"
	"leiAgent/internal/proxy"
	"leiAgent/utils"
	"sort"
	"strings"
)

// RegisterMCPFromHub searches LobeHub MCP marketplace and writes the selected deployment connection
// into config/config.yaml -> mcp_servers via proxy.SaveMCPConfigForm.
type RegisterMCPFromHub struct{}

func NewRegisterMCPFromHub() *RegisterMCPFromHub {
	return &RegisterMCPFromHub{}
}

func (t *RegisterMCPFromHub) Name() string {
	return "register_mcp_from_hub"
}

func (t *RegisterMCPFromHub) Description() string {
	return `Search LobeHub MCP Marketplace and register (configure) an MCP server into config/config.yaml -> mcp_servers.

This tool will:
1) (Optional) auto-register Hub identity if needed.
2) Search marketplace by query (or use explicit identifier).
3) Fetch plugin detail to get deployment options.
4) Select a deployment option (recommended by default).
5) Upsert an MCP server entry into config/config.yaml.

Use it when the user says: "帮我注册 Playwright MCP" / "install MCP plugin X".`
}

func (t *RegisterMCPFromHub) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search keyword, e.g. 'playwright'. Ignored if identifier is provided.",
			},
			"identifier": map[string]interface{}{
				"type":        "string",
				"description": "Exact marketplace plugin identifier. If provided, search step is skipped.",
			},
			"installation_method": map[string]interface{}{
				"type":        "string",
				"description": "Optional preferred installationMethod from deploymentOptions (exact match). If omitted, pick recommended or first option.",
			},
			"label": map[string]interface{}{
				"type":        "string",
				"description": "Optional mcp_servers[].label override. Defaults to plugin identifier.",
			},
			"overwrite": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, overwrite existing server with same label. Default false (will error if label exists).",
				"default":     false,
			},
			"auto_register_hub": map[string]interface{}{
				"type":        "boolean",
				"description": "If true and Hub identity is missing, try to register using hub_name + hub_description.",
				"default":     false,
			},
			"hub_name": map[string]interface{}{
				"type":        "string",
				"description": "Hub identity display name used when auto_register_hub is true.",
			},
			"hub_description": map[string]interface{}{
				"type":        "string",
				"description": "Hub identity description used when auto_register_hub is true.",
			},
		},
		"required": []string{},
	}
}

func (t *RegisterMCPFromHub) Results() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"ok":              map[string]interface{}{"type": "boolean"},
			"identifier":      map[string]interface{}{"type": "string"},
			"label":           map[string]interface{}{"type": "string"},
			"installation":    map[string]interface{}{"type": "string"},
			"transport_type":  map[string]interface{}{"type": "string"},
			"command":         map[string]interface{}{"type": "string"},
			"url":             map[string]interface{}{"type": "string"},
			"args":            map[string]interface{}{"type": "array"},
			"env_keys":        map[string]interface{}{"type": "array"},
			"headers_keys":    map[string]interface{}{"type": "array"},
			"config_path":     map[string]interface{}{"type": "string"},
			"hub_registered":  map[string]interface{}{"type": "boolean"},
			"hub_message":     map[string]interface{}{"type": "string"},
			"warning":         map[string]interface{}{"type": "string"},
			"selected_option": map[string]interface{}{"type": "object"},
		},
	}
}

func (t *RegisterMCPFromHub) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap("MCP外部工具", "从 LobeHub Marketplace 搜索并写入 mcp_servers，实现一键注册 MCP 服务。")
}

func (t *RegisterMCPFromHub) Execute(ctx context.Context, args string) (string, error) {
	params, err := parseToolArgs(args)
	if err != nil {
		return "", err
	}

	query, _ := params["query"].(string)
	query = strings.TrimSpace(query)
	identifier, _ := params["identifier"].(string)
	identifier = strings.TrimSpace(identifier)
	installMethod, _ := params["installation_method"].(string)
	installMethod = strings.TrimSpace(installMethod)
	labelOverride, _ := params["label"].(string)
	labelOverride = strings.TrimSpace(labelOverride)
	overwrite := boolFromAny(params["overwrite"])

	autoRegisterHub := boolFromAny(params["auto_register_hub"])
	hubName, _ := params["hub_name"].(string)
	hubDesc, _ := params["hub_description"].(string)

	status, err := proxy.GetMCPHubStatus()
	if err != nil {
		return "", err
	}
	if !status.Registered {
		if autoRegisterHub {
			if strings.TrimSpace(hubName) == "" || strings.TrimSpace(hubDesc) == "" {
				return "", fmt.Errorf("未注册 LobeHub Marketplace（%s），且 auto_register_hub=true 但 hub_name / hub_description 缺失", status.Message)
			}
			if _, err := proxy.RegisterMCPHub(hubName, hubDesc); err != nil {
				return "", err
			}
			status, err = proxy.GetMCPHubStatus()
			if err != nil {
				return "", err
			}
		}
	}

	if identifier == "" {
		if query == "" {
			return "", fmt.Errorf("query 或 identifier 至少提供一个")
		}
		search, err := proxy.SearchMCPHub(query, "", 1, 12)
		if err != nil {
			return "", err
		}
		best := pickBestHubItem(search.Items)
		if best.Identifier == "" {
			return "", fmt.Errorf("未在 MCP Hub 搜到匹配项：%q", query)
		}
		identifier = best.Identifier
	}

	detail, err := proxy.GetMCPHubPluginDetail(identifier)
	if err != nil {
		return "", err
	}

	opt, err := selectDeploymentOption(detail.DeploymentOptions, installMethod)
	if err != nil {
		return "", err
	}

	cfg := serverConfigFromHub(detail, opt)
	if labelOverride != "" {
		cfg.Label = labelOverride
	}
	if strings.TrimSpace(cfg.Label) == "" {
		cfg.Label = strings.TrimSpace(detail.Identifier)
	}
	if strings.TrimSpace(cfg.Label) == "" {
		cfg.Label = normalizeLabelFromName(detail.Name)
	}
	if strings.TrimSpace(cfg.Label) == "" {
		return "", fmt.Errorf("无法生成 mcp server label（identifier/name 均为空）")
	}

	state, err := proxy.GetMCPConfigFormState()
	if err != nil {
		return "", err
	}

	// Detect duplicates by label (case-insensitive).
	existingIdx := -1
	for i := range state.Servers {
		if strings.EqualFold(strings.TrimSpace(state.Servers[i].Label), cfg.Label) {
			existingIdx = i
			break
		}
	}
	if existingIdx >= 0 && !overwrite {
		return "", fmt.Errorf("mcp_servers 已存在 label=%q；如需覆盖请设置 overwrite=true", cfg.Label)
	}

	nextRow := rowFromServerConfig(cfg)
	if existingIdx >= 0 {
		state.Servers[existingIdx] = nextRow
	} else {
		state.Servers = append(state.Servers, nextRow)
	}

	savedPath, err := proxy.SaveMCPConfigForm(state.Servers)
	if err != nil {
		return "", err
	}

	envKeys := sortedKeys(cfg.Env)
	headerKeys := sortedKeys(cfg.Headers)
	warning := ""
	if missing := mcpbridge.MissingRequiredEnvKeys(cfg); len(missing) > 0 {
		warning = fmt.Sprintf("已写入配置，但可能缺少常用环境变量：%s", strings.Join(missing, ", "))
	}

	out, _ := json.MarshalIndent(map[string]interface{}{
		"ok":             true,
		"identifier":     detail.Identifier,
		"label":          cfg.Label,
		"installation":   opt.InstallationMethod,
		"transport_type": cfg.TransportType,
		"command":        cfg.Command,
		"url":            cfg.URL,
		"args":           cfg.Args,
		"env_keys":       envKeys,
		"headers_keys":   headerKeys,
		"config_path":    savedPath,
		"hub_registered": status.Registered,
		"hub_message":    status.Message,
		"warning":        warning,
		"selected_option": map[string]interface{}{
			"installationMethod": opt.InstallationMethod,
			"isRecommended":      opt.IsRecommended,
			"description":        opt.Description,
			"connectionType":     opt.Connection.Type,
		},
	}, "", "  ")
	return string(out), nil
}

func pickBestHubItem(items []proxy.MCPHubSearchItem) proxy.MCPHubSearchItem {
	best := proxy.MCPHubSearchItem{}
	bestScore := int64(-1)
	for _, it := range items {
		score := int64(0)
		if it.IsOfficial {
			score += 1_000_000
		}
		if it.IsValidated {
			score += 100_000
		}
		if it.IsFeatured {
			score += 10_000
		}
		score += int64(it.InstallCount) * 10
		score += int64(it.RatingCount) * 2
		score += int64(it.RatingAverage * 100)
		if strings.TrimSpace(it.Identifier) == "" {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = it
		}
	}
	return best
}

func selectDeploymentOption(options []proxy.MCPHubDeploymentOption, installMethod string) (proxy.MCPHubDeploymentOption, error) {
	if len(options) == 0 {
		return proxy.MCPHubDeploymentOption{}, fmt.Errorf("该插件缺少 deploymentOptions")
	}
	if installMethod != "" {
		for _, opt := range options {
			if strings.EqualFold(strings.TrimSpace(opt.InstallationMethod), installMethod) {
				return opt, nil
			}
		}
		return proxy.MCPHubDeploymentOption{}, fmt.Errorf("未找到 installation_method=%q 对应的 deploymentOption", installMethod)
	}
	for _, opt := range options {
		if opt.IsRecommended {
			return opt, nil
		}
	}
	return options[0], nil
}

func serverConfigFromHub(detail proxy.MCPHubPluginDetail, opt proxy.MCPHubDeploymentOption) mcpbridge.ServerConfig {
	c := opt.Connection
	transport := strings.TrimSpace(c.Type)
	if transport == "" {
		transport = strings.TrimSpace(detail.ConnectionType)
	}
	return mcpbridge.ServerConfig{
		Label:         strings.TrimSpace(detail.Identifier),
		TransportType: transport,
		Command:       strings.TrimSpace(c.Command),
		URL:           strings.TrimSpace(c.URL),
		Args:          append([]string(nil), c.Args...),
		Env:           cloneStringMap(c.Env),
		Headers:       cloneStringMap(c.Headers),
	}
}

func rowFromServerConfig(cfg mcpbridge.ServerConfig) proxy.MCPConfigRow {
	return proxy.MCPConfigRow{
		Label:         strings.TrimSpace(cfg.Label),
		TransportType: strings.TrimSpace(cfg.TransportType),
		URL:           strings.TrimSpace(cfg.URL),
		Command:       strings.TrimSpace(cfg.Command),
		ArgsText:      strings.Join(cfg.Args, "\n"),
		AllowedTools:  strings.Join(cfg.AllowedTools, "\n"),
		HeadersText:   mapToLines(cfg.Headers),
		EnvText:       mapToLines(cfg.Env),
	}
}

func mapToLines(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if strings.TrimSpace(k) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s: %s", k, m[k]))
	}
	return strings.Join(lines, "\n")
}

func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if strings.TrimSpace(k) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cloneStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func boolFromAny(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "yes", "y", "on":
			return true
		default:
			return false
		}
	case float64:
		return x != 0
	default:
		return false
	}
}

func normalizeLabelFromName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	replacer := strings.NewReplacer("_", "-", "/", "-", "\\", "-", ".", "-", ":", "-", "@", "-", "#", "-")
	s = replacer.Replace(s)
	s = strings.Trim(s, "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}

