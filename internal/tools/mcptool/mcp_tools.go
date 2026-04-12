package mcptool

import (
	"context"
	"encoding/json"
	"fmt"
	mcpbridge "leiAgent/internal/MCP"
	"leiAgent/utils"
	"net/http"
	"strings"
)

type ListMCPTools struct {
	httpClient *http.Client
}

type CallMCPTool struct {
	httpClient *http.Client
}

func NewListMCPTools(httpClient *http.Client) *ListMCPTools {
	return &ListMCPTools{httpClient: httpClient}
}

func NewCallMCPTool(httpClient *http.Client) *CallMCPTool {
	return &CallMCPTool{httpClient: httpClient}
}

func (t *ListMCPTools) Name() string {
	return "list_mcp_tools"
}

func (t *ListMCPTools) Description() string {
	return `List the tools exposed by a configured MCP server.

Use this before calling an unfamiliar MCP server so you can see available tool names and input schemas.
The server is resolved from config/config.yaml -> mcp_servers, or from the inline server_url override if provided.

Recovery rule:
- If this tool fails because the MCP environment is not ready yet, you MAY use execute_command to repair the local environment, then retry list_mcp_tools.
- Typical examples: start Chrome with remote debugging for chrome-devtools MCP, check localhost readiness with curl, or launch a missing local helper process.`
}

func (t *ListMCPTools) Parameters() map[string]interface{} {
	return baseServerParameters([]string{"server_label"})
}

func (t *ListMCPTools) Results() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"server_label": map[string]interface{}{"type": "string"},
			"tools":        map[string]interface{}{"type": "array"},
		},
	}
}

func (t *ListMCPTools) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap("MCP外部工具", "列出配置好的 MCP 服务可调用工具和输入参数。")
}

func (t *ListMCPTools) Execute(ctx context.Context, args string) (string, error) {
	params, err := parseToolArgs(args)
	if err != nil {
		return "", err
	}
	cfg, err := resolveServerConfig(params)
	if err != nil {
		return "", err
	}

	manager := mcpbridge.NewManager(t.httpClient)
	tools, err := manager.ListTools(ctx, cfg)
	if err != nil {
		return "", err
	}

	out, _ := json.MarshalIndent(map[string]interface{}{
		"server_label": cfg.Label,
		"tools":        tools,
	}, "", "  ")
	return string(out), nil
}

func (t *CallMCPTool) Name() string {
	return "call_mcp_tool"
}

func (t *CallMCPTool) Description() string {
	return `Call a tool on a configured MCP server.

Recommended workflow:
1. Use list_mcp_tools to inspect the target server and its input schema.
2. Then call this tool with server_label, tool_name, and a JSON object in arguments.

The server is resolved from config/config.yaml -> mcp_servers, or from the inline server_url override if provided.

Recovery rule:
- If MCP readiness checks fail, you MAY use execute_command to fix the local environment first, then retry.
- Example: for chrome-devtools MCP, execute a safe local command to start Chrome with --remote-debugging-port and verify the port before retrying this tool.`
}

func (t *CallMCPTool) Parameters() map[string]interface{} {
	params := baseServerParameters([]string{"server_label", "tool_name"})
	properties := params["properties"].(map[string]interface{})
	properties["tool_name"] = map[string]interface{}{
		"type":        "string",
		"description": "The MCP tool name to invoke on the server.",
	}
	properties["arguments"] = map[string]interface{}{
		"type":        "object",
		"description": "A JSON object passed as the MCP tool arguments.",
	}
	params["required"] = []string{"server_label", "tool_name"}
	return params
}

func (t *CallMCPTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"server_label":       map[string]interface{}{"type": "string"},
			"name":               map[string]interface{}{"type": "string"},
			"is_error":           map[string]interface{}{"type": "boolean"},
			"content":            map[string]interface{}{},
			"structured_content": map[string]interface{}{"type": "object"},
		},
	}
}

func (t *CallMCPTool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap("MCP外部工具", "调用配置好的 MCP 服务上的指定工具。")
}

func (t *CallMCPTool) Execute(ctx context.Context, args string) (string, error) {
	params, err := parseToolArgs(args)
	if err != nil {
		return "", err
	}
	cfg, err := resolveServerConfig(params)
	if err != nil {
		return "", err
	}

	toolName, _ := params["tool_name"].(string)
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return "", fmt.Errorf("tool_name is required")
	}

	callArgs := map[string]interface{}{}
	if rawArgs, ok := params["arguments"].(map[string]interface{}); ok {
		callArgs = rawArgs
	}

	manager := mcpbridge.NewManager(t.httpClient)
	result, err := manager.CallTool(ctx, cfg, toolName, callArgs)
	if err != nil {
		return "", err
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return string(out), nil
}

func baseServerParameters(required []string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"server_label": map[string]interface{}{
				"type":        "string",
				"description": "Configured MCP server label from config/config.yaml -> mcp_servers[].label.",
			},
			"server_url": map[string]interface{}{
				"type":        "string",
				"description": "Optional override MCP server URL. If provided, it overrides the configured URL.",
			},
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Optional override MCP stdio command, such as npx.",
			},
			"args": map[string]interface{}{
				"type":        "array",
				"description": "Optional override MCP stdio args.",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"env": map[string]interface{}{
				"type":        "object",
				"description": "Optional extra environment variables for the MCP process.",
			},
			"transport_type": map[string]interface{}{
				"type":        "string",
				"description": "Optional transport type hint such as streamable_http or http.",
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "Optional extra HTTP headers used to authenticate or route the MCP request.",
			},
			"allowed_tools": map[string]interface{}{
				"type":        "array",
				"description": "Optional allow-list override for tool names.",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
		},
		"required": required,
	}
}

func parseToolArgs(args string) (map[string]interface{}, error) {
	params := map[string]interface{}{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return nil, err
	}
	return params, nil
}

func resolveServerConfig(params map[string]interface{}) (mcpbridge.ServerConfig, error) {
	label, _ := params["server_label"].(string)
	label = strings.TrimSpace(label)
	cfg := mcpbridge.ServerConfig{}

	if label != "" {
		found, err := mcpbridge.GetServerConfig(label)
		if err == nil {
			cfg = *found
		} else if rawURL, ok := params["server_url"].(string); !ok || strings.TrimSpace(rawURL) == "" {
			return mcpbridge.ServerConfig{}, err
		}
	}

	if cfg.Label == "" {
		cfg.Label = label
	}
	if rawURL, ok := params["server_url"].(string); ok && strings.TrimSpace(rawURL) != "" {
		cfg.URL = strings.TrimSpace(rawURL)
	}
	if rawCommand, ok := params["command"].(string); ok && strings.TrimSpace(rawCommand) != "" {
		cfg.Command = strings.TrimSpace(rawCommand)
	}
	if rawArgs, ok := params["args"].([]interface{}); ok {
		cfg.Args = make([]string, 0, len(rawArgs))
		for _, item := range rawArgs {
			cfg.Args = append(cfg.Args, fmt.Sprintf("%v", item))
		}
	}
	if env, ok := params["env"].(map[string]interface{}); ok {
		cfg.Env = make(map[string]string, len(env))
		for k, v := range env {
			if strings.TrimSpace(k) == "" {
				continue
			}
			cfg.Env[k] = fmt.Sprintf("%v", v)
		}
	}
	if rawTransport, ok := params["transport_type"].(string); ok && strings.TrimSpace(rawTransport) != "" {
		cfg.TransportType = strings.TrimSpace(rawTransport)
	}
	if headers, ok := params["headers"].(map[string]interface{}); ok {
		cfg.Headers = make(map[string]string, len(headers))
		for k, v := range headers {
			if k == "" {
				continue
			}
			cfg.Headers[k] = fmt.Sprintf("%v", v)
		}
	}
	if allowed, ok := params["allowed_tools"].([]interface{}); ok {
		cfg.AllowedTools = make([]string, 0, len(allowed))
		for _, item := range allowed {
			cfg.AllowedTools = append(cfg.AllowedTools, fmt.Sprintf("%v", item))
		}
	}

	if strings.TrimSpace(cfg.Label) == "" {
		cfg.Label = "inline"
	}
	if strings.TrimSpace(cfg.URL) == "" && strings.TrimSpace(cfg.Command) == "" {
		return mcpbridge.ServerConfig{}, fmt.Errorf("either server_url or command is required when server_label cannot be resolved from config")
	}

	return cfg, nil
}
