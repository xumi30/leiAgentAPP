package mcpbridge

import (
	"fmt"
	"os"
	"strings"

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
		configPath = "config/config.yaml"
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
	for _, server := range servers {
		if server.Label == label {
			cp := server
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("mcp server %q not found in config", label)
}
