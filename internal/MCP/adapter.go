package mcpbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/utils"
	"sort"
	"strings"
)

const dynamicToolPrefix = "mcp__"

type DynamicTool struct {
	Server ServerConfig
	Tool   ToolInfo
	Topic  string
}

type SimpleServerInfo struct {
	Label       string   `json:"label"`
	Topic       string   `json:"topic"`
	Description string   `json:"description"`
	ToolCount   int      `json:"tool_count"`
	ToolNames   []string `json:"tool_names,omitempty"`
}

func BuildDynamicToolsByTopic(topic string) []openaistyle.Tool {
	dynamics := ListDynamicToolsByTopic(topic)
	out := make([]openaistyle.Tool, 0, len(dynamics))
	for _, item := range dynamics {
		params := item.Tool.InputSchema
		if params == nil {
			params = map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
		}
		out = append(out, openaistyle.Tool{
			Type: openaistyle.ToolTypeFunction,
			Function: &openaistyle.Function{
				Name:        dynamicToolName(item.Server.Label, item.Tool.Name),
				Description: buildDynamicToolDescription(item),
				Parameters:  params,
			},
		})
	}
	return out
}

func ListDynamicToolsByTopic(topic string) []DynamicTool {
	servers, err := LoadServerConfigs()
	if err != nil {
		return nil
	}
	out := make([]DynamicTool, 0)
	for _, server := range servers {
		cache, err := ReadToolCache(server.Label)
		if err != nil || cache == nil || !cache.OK {
			continue
		}
		serverTopic := inferServerTopic(server, cache.Tools)
		if strings.TrimSpace(topic) != "" && serverTopic != topic {
			continue
		}
		for _, tool := range cache.Tools {
			if strings.TrimSpace(tool.Name) == "" {
				continue
			}
			out = append(out, DynamicTool{
				Server: server,
				Tool:   tool,
				Topic:  serverTopic,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Server.Label == out[j].Server.Label {
			return out[i].Tool.Name < out[j].Tool.Name
		}
		return out[i].Server.Label < out[j].Server.Label
	})
	return out
}

func ResolveDynamicTool(name string) (*DynamicTool, bool) {
	label, toolName, ok := parseDynamicToolName(name)
	if !ok {
		return nil, false
	}
	servers, err := LoadServerConfigs()
	if err != nil {
		return nil, false
	}
	var cfg *ServerConfig
	for _, server := range servers {
		if sanitizeSegment(server.Label) == label {
			cp := server
			cfg = &cp
			break
		}
	}
	if cfg == nil {
		return nil, false
	}
	cache, err := ReadToolCache(cfg.Label)
	if err != nil || cache == nil {
		return nil, false
	}
	for _, tool := range cache.Tools {
		if sanitizeSegment(tool.Name) == toolName {
			return &DynamicTool{
				Server: *cfg,
				Tool:   tool,
				Topic:  inferServerTopic(*cfg, cache.Tools),
			}, true
		}
	}
	return nil, false
}

func ExecuteDynamicTool(ctx context.Context, name string, arguments string) (string, error) {
	item, ok := ResolveDynamicTool(name)
	if !ok {
		return "", fmt.Errorf("mcp dynamic tool %q not found", name)
	}
	callArgs := map[string]interface{}{}
	if strings.TrimSpace(arguments) != "" {
		if err := json.Unmarshal([]byte(arguments), &callArgs); err != nil {
			return "", fmt.Errorf("invalid mcp tool arguments: %w", err)
		}
	}
	manager := NewManager(nil)
	res, err := manager.CallTool(ctx, item.Server, item.Tool.Name, callArgs)
	if err != nil {
		return "", err
	}
	out, _ := json.MarshalIndent(res, "", "  ")
	return string(out), nil
}

func GetDynamicToolMeta(name string) (description string, params map[string]interface{}, ok bool) {
	item, found := ResolveDynamicTool(name)
	if !found {
		return "", nil, false
	}
	return buildDynamicToolDescription(*item), item.Tool.InputSchema, true
}

func GetMCPSimpleInfos() []SimpleServerInfo {
	servers, err := LoadServerConfigs()
	if err != nil {
		return nil
	}
	out := make([]SimpleServerInfo, 0, len(servers))
	for _, server := range servers {
		cache, err := ReadToolCache(server.Label)
		if err != nil || cache == nil || !cache.OK {
			continue
		}
		names := make([]string, 0, len(cache.Tools))
		for _, tool := range cache.Tools {
			if strings.TrimSpace(tool.Name) != "" {
				names = append(names, strings.TrimSpace(tool.Name))
			}
		}
		sort.Strings(names)
		topic := inferServerTopic(server, cache.Tools)
		out = append(out, SimpleServerInfo{
			Label:       server.Label,
			Topic:       topic,
			Description: buildServerSimpleDescription(server, cache.Tools, topic),
			ToolCount:   len(names),
			ToolNames:   names,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

func dynamicToolName(label, toolName string) string {
	return dynamicToolPrefix + sanitizeSegment(label) + "__" + sanitizeSegment(toolName)
}

func parseDynamicToolName(name string) (label, toolName string, ok bool) {
	if !strings.HasPrefix(name, dynamicToolPrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(name, dynamicToolPrefix)
	parts := strings.SplitN(rest, "__", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func sanitizeSegment(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func inferServerTopic(server ServerConfig, tools []ToolInfo) string {
	sample := strings.ToLower(server.Label + " " + strings.Join(toolNames(tools), " ") + " " + toolDescriptions(tools))
	switch {
	case strings.Contains(sample, "chrome") || strings.Contains(sample, "devtools") || strings.Contains(sample, "browser") || strings.Contains(sample, "page") || strings.Contains(sample, "navigate"):
		return utils.ToolTopicBrowser
	case strings.Contains(sample, "search") || strings.Contains(sample, "query") || strings.Contains(sample, "document") || strings.Contains(sample, "lookup"):
		return utils.ToolTopicSearch
	case strings.Contains(sample, "file") || strings.Contains(sample, "directory") || strings.Contains(sample, "workspace"):
		return utils.ToolTopicFiles
	case strings.Contains(sample, "time") || strings.Contains(sample, "date") || strings.Contains(sample, "calendar"):
		return utils.ToolTopicTime
	case strings.Contains(sample, "write") || strings.Contains(sample, "novel") || strings.Contains(sample, "story"):
		return utils.ToolTopicWriting
	default:
		return utils.ToolTopicMCP
	}
}

func buildServerSimpleDescription(server ServerConfig, tools []ToolInfo, topic string) string {
	names := toolNames(tools)
	if len(names) > 4 {
		names = names[:4]
	}
	return fmt.Sprintf("MCP 服务 %s，主要归类为%s，可用 %d 个工具，例如：%s。", server.Label, topic, len(tools), strings.Join(names, ", "))
}

func buildDynamicToolDescription(item DynamicTool) string {
	desc := strings.TrimSpace(item.Tool.Description)
	if desc == "" {
		desc = fmt.Sprintf("MCP tool %s from server %s.", item.Tool.Name, item.Server.Label)
	}
	return fmt.Sprintf("[MCP:%s] %s", item.Server.Label, desc)
}

func toolNames(tools []ToolInfo) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) != "" {
			out = append(out, strings.TrimSpace(tool.Name))
		}
	}
	return out
}

func toolDescriptions(tools []ToolInfo) string {
	parts := make([]string, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Description) != "" {
			parts = append(parts, strings.TrimSpace(tool.Description))
		}
	}
	return strings.Join(parts, " ")
}
