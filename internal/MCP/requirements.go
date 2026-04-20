package mcpbridge

import (
	"os"
	"sort"
	"strings"
)

// InferRequiredEnvKeys returns a conservative list of environment variables
// that are commonly required by well-known MCP servers. We intentionally keep
// this list small and high-confidence so the settings page stays useful
// without producing too many false positives.
func InferRequiredEnvKeys(cfg ServerConfig) []string {
	candidates := []string{}
	for key, value := range cfg.Env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if strings.TrimSpace(value) == "" {
			candidates = append(candidates, key)
		}
	}

	sampleParts := []string{cfg.Label, cfg.Command, cfg.URL}
	sampleParts = append(sampleParts, cfg.Args...)
	sample := strings.ToLower(strings.Join(sampleParts, " "))

	addIfContains := func(needle string, keys ...string) {
		if !strings.Contains(sample, needle) {
			return
		}
		candidates = append(candidates, keys...)
	}

	addIfContains("tavily", "TAVILY_API_KEY")
	addIfContains("firecrawl", "FIRECRAWL_API_KEY")
	addIfContains("exa", "EXA_API_KEY")
	addIfContains("serpapi", "SERPAPI_API_KEY", "SERPAPI_KEY")
	addIfContains("perplexity", "PERPLEXITY_API_KEY")
	addIfContains("brave", "BRAVE_SEARCH_API_KEY", "BRAVE_API_KEY")
	addIfContains("github", "GITHUB_TOKEN")
	addIfContains("notion", "NOTION_API_KEY")
	addIfContains("slack", "SLACK_BOT_TOKEN")

	seen := make(map[string]struct{}, len(candidates))
	required := make([]string, 0, len(candidates))
	for _, key := range candidates {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		required = append(required, key)
	}
	sort.Strings(required)
	return required
}

func MissingRequiredEnvKeys(cfg ServerConfig) []string {
	required := InferRequiredEnvKeys(cfg)
	if len(required) == 0 {
		return nil
	}

	missing := make([]string, 0, len(required))
	for _, key := range required {
		if value, ok := cfg.Env[key]; ok && strings.TrimSpace(value) != "" {
			continue
		}
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			continue
		}
		missing = append(missing, key)
	}
	return missing
}
