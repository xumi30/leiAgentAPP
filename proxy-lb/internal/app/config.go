package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"
)

type Config struct {
	ConfigPath string        `yaml:"-"`
	ConfigDir  string        `yaml:"-"`
	Server     ServerConfig  `yaml:"server"`
	Models     []ModelConfig `yaml:"models"`
}

type ServerConfig struct {
	Listen                string `yaml:"listen"`
	AuthToken             string `yaml:"auth_token"`
	DefaultModel          string `yaml:"default_model"`
	RequestTimeoutSeconds int    `yaml:"request_timeout_seconds"`
	AuthDataPath          string `yaml:"auth_data_path"`
}

type ModelConfig struct {
	Name     string          `yaml:"name"`
	Strategy string          `yaml:"strategy"`
	Backends []BackendConfig `yaml:"backends"`
}

type BackendConfig struct {
	Name            string `yaml:"name"`
	Provider        string `yaml:"provider"`
	APIKey          string `yaml:"api_key"`
	BaseURL         string `yaml:"base_url"`
	Model           string `yaml:"model"`
	Enabled         *bool  `yaml:"enabled,omitempty"`
	Weight          int    `yaml:"weight,omitempty"`
	StreamMode      string `yaml:"stream_mode,omitempty"`
	MaxOutputTokens int    `yaml:"max_output_tokens,omitempty"`
}

func LoadConfig() (*Config, error) {
	path := strings.TrimSpace(os.Getenv("PROXY_LB_CONFIG"))
	if path == "" {
		path = filepath.Join("config", "config.yaml")
	}
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.ConfigPath = path
	cfg.ConfigDir = filepath.Dir(path)

	if strings.TrimSpace(cfg.Server.Listen) == "" {
		cfg.Server.Listen = ":8088"
	}
	if cfg.Server.RequestTimeoutSeconds <= 0 {
		cfg.Server.RequestTimeoutSeconds = 300
	}
	if strings.TrimSpace(cfg.Server.AuthDataPath) == "" {
		cfg.Server.AuthDataPath = filepath.Join("data", "auth.json")
	}
	if !filepath.IsAbs(cfg.Server.AuthDataPath) {
		cfg.Server.AuthDataPath = filepath.Join(cfg.ConfigDir, cfg.Server.AuthDataPath)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if len(c.Models) == 0 {
		return fmt.Errorf("models is required")
	}

	modelNames := make(map[string]struct{}, len(c.Models))
	for i, model := range c.Models {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			return fmt.Errorf("models[%d].name is required", i)
		}
		if _, exists := modelNames[name]; exists {
			return fmt.Errorf("duplicate model name %q", name)
		}
		modelNames[name] = struct{}{}
		if len(model.Backends) == 0 {
			return fmt.Errorf("models[%d].backends is required", i)
		}

		enabledCount := 0
		for j, backend := range model.Backends {
			if backend.Enabled != nil && !*backend.Enabled {
				continue
			}
			enabledCount++

			if strings.TrimSpace(backend.BaseURL) == "" {
				return fmt.Errorf("models[%d].backends[%d].base_url is required", i, j)
			}
			if strings.TrimSpace(backend.Model) == "" {
				return fmt.Errorf("models[%d].backends[%d].model is required", i, j)
			}
			if strings.TrimSpace(backend.APIKey) == "" {
				return fmt.Errorf("models[%d].backends[%d].api_key is required", i, j)
			}
		}
		if enabledCount == 0 {
			return fmt.Errorf("models[%d] requires at least one enabled backend", i)
		}
	}

	if c.Server.DefaultModel != "" {
		if _, ok := modelNames[strings.TrimSpace(c.Server.DefaultModel)]; !ok {
			return fmt.Errorf("server.default_model %q not found in models", c.Server.DefaultModel)
		}
	}
	return nil
}
