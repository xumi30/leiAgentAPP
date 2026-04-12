package mcpbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
)

func (m *Manager) EnsureReady(ctx context.Context, cfg ServerConfig) error {
	if strings.TrimSpace(cfg.Command) != "" {
		if err := ensureCommandAvailable(cfg.Command); err != nil {
			return err
		}
		if err := m.ensureBrowserDebugEndpoint(ctx, cfg); err != nil {
			return err
		}
		return nil
	}

	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("mcp server configuration is invalid: missing both server url and command")
	}
	return nil
}

func ensureCommandAvailable(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("mcp stdio command is empty")
	}
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Errorf("mcp command %q not found in PATH. You can use the bash tool to install or fix it", command)
	}
	return nil
}

func (m *Manager) ensureBrowserDebugEndpoint(ctx context.Context, cfg ServerConfig) error {
	if hasAutoConnectFlag(cfg.Args) {
		return nil
	}

	browserURL, ok := extractFlagValue(cfg.Args, "--browser-url", "--browserUrl", "-u")
	if ok && strings.TrimSpace(browserURL) != "" {
		return m.checkBrowserURL(ctx, browserURL)
	}

	if wsEndpoint, ok := extractFlagValue(cfg.Args, "--ws-endpoint", "--wsEndpoint", "-w"); ok && strings.TrimSpace(wsEndpoint) != "" {
		if _, err := url.Parse(strings.TrimSpace(wsEndpoint)); err != nil {
			return fmt.Errorf("invalid ws-endpoint %q: %w", wsEndpoint, err)
		}
	}

	return nil
}

func (m *Manager) checkBrowserURL(ctx context.Context, browserURL string) error {
	versionURL, err := browserVersionURL(browserURL)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return err
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("chrome remote debugging is not reachable at %s. On macOS, prefer launching a fresh instance with: open -na \"Google Chrome\" --args --remote-debugging-port=9222 --user-data-dir=/tmp/chrome-mcp . If an old Chrome process is blocking the args, first run: pkill -f \"Google Chrome\" . The agent can use the bash tool to run these commands", versionURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("chrome remote debugging endpoint %s returned status=%d body=%s", versionURL, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return err
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("chrome remote debugging endpoint %s did not return valid json: %w", versionURL, err)
	}
	if ws, _ := payload["webSocketDebuggerUrl"].(string); strings.TrimSpace(ws) == "" {
		return fmt.Errorf("chrome remote debugging endpoint %s is up, but webSocketDebuggerUrl is missing. Confirm Chrome was started with --remote-debugging-port", versionURL)
	}

	return nil
}

func browserVersionURL(browserURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(browserURL))
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid browser-url %q", browserURL)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/json/version"
	return u.String(), nil
}

func hasAutoConnectFlag(args []string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == "--autoConnect" {
			return true
		}
	}
	return false
}

func extractFlagValue(args []string, names ...string) (string, bool) {
	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[name] = struct{}{}
	}
	for i, arg := range args {
		arg = strings.TrimSpace(arg)
		for name := range nameSet {
			if arg == name && i+1 < len(args) {
				return strings.TrimSpace(args[i+1]), true
			}
			prefix := name + "="
			if strings.HasPrefix(arg, prefix) {
				return strings.TrimSpace(strings.TrimPrefix(arg, prefix)), true
			}
		}
	}
	return "", false
}
