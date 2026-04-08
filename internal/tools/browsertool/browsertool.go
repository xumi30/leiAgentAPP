package browsertool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"leiAgent/internal/tools"
	"leiAgent/utils"

	"github.com/pkg/browser"
)

// BrowserTool opens URLs in the system default browser (http/https only).
type BrowserTool struct{}

// New returns a Tool that performs basic browser operations on the user's machine.
func New() tools.Tool {
	return &BrowserTool{}
}

func (t *BrowserTool) Name() string {
	return "browser_operate"
}

func (t *BrowserTool) Description() string {
	return "Open a URL in the user's normal desktop browser (Safari/Chrome/Edge, etc.). " +
		"This does NOT create a browser_automation session: you cannot click or extract DOM with browser_automation on that window. " +
		"For automated clicks, scrolling, or reading page text via tools, use browser_automation (navigate with url, or pass url+selector together). " +
		"当用户只想在本机默认浏览器里打开链接、人工查看时使用；若需要自动点击/抓取/多步操作，请用 browser_automation。"
}

func (t *BrowserTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Operation to perform",
				"enum":        []string{"open_url"},
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "Full URL to open; must use http:// or https://",
			},
		},
		"required": []string{"action", "url"},
	}
}

func (t *BrowserTool) Execute(ctx context.Context, args string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	action, _ := params["action"].(string)
	action = strings.TrimSpace(strings.ToLower(action))
	raw, _ := params["url"].(string)
	raw = strings.TrimSpace(raw)

	if action != "open_url" {
		return "", fmt.Errorf("unsupported action %q (only open_url)", action)
	}
	if raw == "" {
		return "", fmt.Errorf("url is required")
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid url: %q", raw)
	}
	if sch := strings.ToLower(u.Scheme); sch != "http" && sch != "https" {
		return "", fmt.Errorf("only http and https URLs are allowed, got scheme %q", u.Scheme)
	}

	// Do not use ctx for cancellation here — request ctx is often already done after streaming.
	if err := browser.OpenURL(raw); err != nil {
		return "", fmt.Errorf("open browser: %w", err)
	}

	out := map[string]interface{}{
		"success": true,
		"action":  "open_url",
		"url":     raw,
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	return string(b), nil
}

func (t *BrowserTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Result of the browser operation",
		"properties": map[string]interface{}{
			"success": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether the operation completed without error",
			},
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The action that was executed",
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL that was opened",
			},
		},
	}
}

func (t *BrowserTool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap(utils.ToolTopicBrowser, "在用户本机默认浏览器中打开 http/https 链接，用于人工浏览，不启动自动化会话。")
}
