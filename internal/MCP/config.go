package mcpbridge

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"leiAgent/internal/appruntime"

	"go.yaml.in/yaml/v2"
)

type ServerConfig struct {
	Label         string            `yaml:"label" json:"label"`
	URL           string            `yaml:"url" json:"url"`
	Command       string            `yaml:"command" json:"command,omitempty"`
	Args          []string          `yaml:"args" json:"args,omitempty"`
	Env           map[string]string `yaml:"env" json:"env,omitempty"`
	TransportType string            `yaml:"transport_type" json:"transport_type,omitempty"`
	AllowedTools  []string          `yaml:"allowed_tools" json:"allowed_tools,omitempty"`
	Headers       map[string]string `yaml:"headers" json:"headers,omitempty"`
	CachedTools   []string          `yaml:"cached_tools,omitempty" json:"cached_tools,omitempty"`
}

type appConfig struct {
	MCPServers []ServerConfig `yaml:"mcp_servers"`
}

func LoadServerConfigs() ([]ServerConfig, error) {
	configPath := strings.TrimSpace(os.Getenv("LEIAGENT_CONFIG_PATH"))
	if configPath == "" {
		configPath = appruntime.ResolvePath("config/config.yaml")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg appConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	servers := make([]ServerConfig, 0, len(cfg.MCPServers))
	for _, server := range cfg.MCPServers {
		server.Label = strings.TrimSpace(server.Label)
		server.URL = strings.TrimSpace(server.URL)
		server.Command = strings.TrimSpace(server.Command)
		server.TransportType = strings.TrimSpace(server.TransportType)
		if server.Label == "" {
			continue
		}
		if server.URL == "" && server.Command == "" {
			continue
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func GetServerConfig(label string) (*ServerConfig, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, fmt.Errorf("mcp server label is required")
	}

	servers, err := LoadServerConfigs()
	if err != nil {
		return nil, err
	}

	lowerLabel := strings.ToLower(label)
	for _, server := range servers {
		if strings.EqualFold(server.Label, label) {
			cp := server
			return &cp, nil
		}
	}

	normalizedLabel := normalizeServerLabel(label)
	var candidates []ServerConfig
	for _, server := range servers {
		serverLower := strings.ToLower(server.Label)
		serverNormalized := normalizeServerLabel(server.Label)
		if serverNormalized == normalizedLabel ||
			strings.Contains(serverLower, lowerLabel) ||
			strings.Contains(serverNormalized, normalizedLabel) ||
			strings.Contains(normalizedLabel, serverNormalized) {
			candidates = append(candidates, server)
		}
	}
	if len(candidates) == 1 {
		cp := candidates[0]
		return &cp, nil
	}

	available := make([]string, 0, len(servers))
	for _, server := range servers {
		available = append(available, server.Label)
	}
	sort.Strings(available)

	if len(candidates) > 1 {
		names := make([]string, 0, len(candidates))
		for _, server := range candidates {
			names = append(names, server.Label)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("mcp server %q is ambiguous; possible matches: %s", label, strings.Join(names, ", "))
	}

	return nil, fmt.Errorf("mcp server %q not found in config; available servers: %s", label, strings.Join(available, ", "))
}

func normalizeServerLabel(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	replacer := strings.NewReplacer("-", "", "_", "", " ", "", ".", "", ":", "", "/", "")
	return replacer.Replace(label)
}
