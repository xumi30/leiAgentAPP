package browsertool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"leiAgent/internal/tools"
	"leiAgent/utils"
)

// BrowserPlaywrightTool proxies browser automation to the Node.js Playwright server under frontend/.
// This Go tool is intentionally thin: it only validates/normalizes args and forwards them via HTTP JSON.
type BrowserPlaywrightTool struct {
	baseURL string
	client  *http.Client
}

func New() tools.Tool {
	baseURL := os.Getenv("PW_SERVER_URL")
	if utils.IsBlank(baseURL) {
		baseURL = "http://127.0.0.1:3111"
	}
	return &BrowserPlaywrightTool{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 90 * time.Second},
	}
}

func (t *BrowserPlaywrightTool) Name() string {
	return "browser_playwright"
}

func (t *BrowserPlaywrightTool) Description() string {
	return `Control a real browser via a local Node.js Playwright server (HTTP JSON). This tool is for REAL browser automation (not just opening a link).

Important:
- You MUST start the Playwright server first (once per machine):
  cd frontend && npm install && npm run playwright:server
- Default server URL: http://127.0.0.1:3111 (override with env PW_SERVER_URL)

Core workflow (always do this):
1) operation=create_session  -> get sessionId
2) operation=action         -> run one action at a time using the same sessionId
3) operation=close_session  -> close the session to release resources

Supported actions (operation=action):
- goto:        params.url (string, required), params.waitUntil (optional: "load"|"domcontentloaded"|"networkidle")
- wait_for_selector: params.selector (string, required)
- click:       params.selector (string, required)
- fill:        params.selector (string, required), params.value (string, required)
- press:       params.selector (string, required), params.key (string, required; e.g. "Enter")
- text:        params.selector (string, required) -> returns textContent
- content:     no params -> returns page HTML
- url:         no params -> returns current page url
- screenshot:  params.fullPage (bool, default true), params.path (optional string). Always returns base64 png in result.bytesBase64.
- evaluate:    params.expression (string, required). The expression MUST evaluate to a function to run in the page context.

Selector tips:
- Prefer stable selectors: data-testid, aria-label, input[name=...], button:has-text("...").
- If an action fails due to timing, call wait_for_selector first, then retry click/fill.

Failure/self-debug recipe:
1) wait_for_selector for the element you expect
2) screenshot (fullPage=true)
3) content (to inspect DOM) or url (to confirm navigation)

Copy-paste examples:

Create session:
{"operation":"create_session","headless":true}

Open page:
{"operation":"action","sessionId":"<id>","action":"goto","params":{"url":"https://example.com","waitUntil":"domcontentloaded"}}

Wait + fill + press Enter:
{"operation":"action","sessionId":"<id>","action":"wait_for_selector","params":{"selector":"input[name='q']"}}
{"operation":"action","sessionId":"<id>","action":"fill","params":{"selector":"input[name='q']","value":"Playwright"}}
{"operation":"action","sessionId":"<id>","action":"press","params":{"selector":"input[name='q']","key":"Enter"}}

Screenshot:
{"operation":"action","sessionId":"<id>","action":"screenshot","params":{"fullPage":true}}

Close session:
{"operation":"close_session","sessionId":"<id>"}`
}

func (t *BrowserPlaywrightTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"create_session", "close_session", "action"},
				"description": "create_session: open a new browser session; close_session: close it; action: run one Playwright action in that session.",
			},
			"sessionId": map[string]interface{}{
				"type":        "string",
				"description": "Required for close_session and action.",
			},
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Required when operation=action. Supported: goto, click, fill, press, wait_for_selector, evaluate, screenshot, content, text, url.",
			},
			"params": map[string]interface{}{
				"type":        "object",
				"description": "Action parameters object. Required fields depend on action. Examples: goto{url,waitUntil}; click{selector}; fill{selector,value}; press{selector,key}; wait_for_selector{selector}; text{selector}; evaluate{expression}; screenshot{fullPage,path}.",
			},
			"timeoutMs": map[string]interface{}{
				"type":        "integer",
				"description": "Optional per-action timeout in milliseconds (server default 30000).",
			},
			"headless": map[string]interface{}{
				"type":        "boolean",
				"description": "Only for create_session. Default true.",
			},
			"viewport": map[string]interface{}{
				"type":        "object",
				"description": "Only for create_session. Example: {\"width\":1280,\"height\":720}.",
			},
			"userAgent": map[string]interface{}{
				"type":        "string",
				"description": "Only for create_session.",
			},
			"locale": map[string]interface{}{
				"type":        "string",
				"description": "Only for create_session.",
			},
		},
		"required": []string{"operation"},
	}
}

func (t *BrowserPlaywrightTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Result of browser operation. The actual tool return is a JSON string from the Playwright server; this schema provides a minimal, consistent shape for UIs.",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Human-readable result message (or a compact summary).",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Absolute file path (if available in message), e.g. screenshot path when saved to disk.",
			},
		},
	}
}

func (t *BrowserPlaywrightTool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap(utils.ToolTopicBrowser, "通过本地 Node.js Playwright 服务间接控制真实浏览器：打开网页、点击输入、抓取内容、截图。")
}

func (t *BrowserPlaywrightTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *BrowserPlaywrightTool) Execute(ctx context.Context, args string) (string, error) {
	args = utils.ExtractJSON(args)
	args = utils.EscapeRawNewlinesInJSONStrings(args)

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("%w: failed to parse arguments: %v", tools.ErrInvalidParams, err)
	}

	op, _ := params["operation"].(string)
	switch op {
	case "create_session":
		body := map[string]interface{}{}
		for _, k := range []string{"headless", "viewport", "userAgent", "locale"} {
			if v, ok := params[k]; ok {
				body[k] = v
			}
		}
		return t.postJSON(ctx, "/sessions", body)

	case "close_session":
		sid, _ := params["sessionId"].(string)
		if utils.IsBlank(sid) {
			return "", fmt.Errorf("%w: sessionId is required for close_session", tools.ErrInvalidParams)
		}
		return t.delete(ctx, "/sessions/"+sid)

	case "action":
		sid, _ := params["sessionId"].(string)
		if utils.IsBlank(sid) {
			return "", fmt.Errorf("%w: sessionId is required for action", tools.ErrInvalidParams)
		}
		act, _ := params["action"].(string)

		// Be forgiving to common LLM payload mistakes:
		// Some models put action/url directly inside "params", like:
		// {"operation":"action","sessionId":"...","params":{"action":"goto","url":"https://..."}}
		var actionParams interface{} = nil
		if v, ok := params["params"]; ok {
			actionParams = v
			if utils.IsBlank(act) {
				if m, ok := v.(map[string]interface{}); ok {
					if a2, ok := m["action"].(string); ok && !utils.IsBlank(a2) {
						act = a2
						// Remove nested action key when forwarding to server.
						cp := make(map[string]interface{}, len(m))
						for k, vv := range m {
							if k == "action" {
								continue
							}
							cp[k] = vv
						}
						actionParams = cp
					}
				}
			}
		}

		if utils.IsBlank(act) {
			return "", fmt.Errorf("%w: action is required when operation=action", tools.ErrInvalidParams)
		}

		body := map[string]interface{}{
			"sessionId": sid,
			"action":    act,
		}
		if actionParams != nil {
			body["params"] = actionParams
		}
		if v, ok := params["timeoutMs"]; ok {
			body["timeoutMs"] = v
		}
		return t.postJSON(ctx, "/actions", body)

	default:
		return "", fmt.Errorf("%w: unknown operation %q", tools.ErrInvalidParams, op)
	}
}

func (t *BrowserPlaywrightTool) postJSON(ctx context.Context, path string, body any) (string, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal request body: %v", tools.ErrInvalidParams, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("%w: failed to create request: %v", tools.ErrExecutionFailed, err)
	}
	req.Header.Set("content-type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: request failed: %v", tools.ErrExecutionFailed, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return string(raw), fmt.Errorf("%w: server returned %s", tools.ErrExecutionFailed, resp.Status)
	}
	return prettyJSON(raw), nil
}

func (t *BrowserPlaywrightTool) delete(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, t.baseURL+path, nil)
	if err != nil {
		return "", fmt.Errorf("%w: failed to create request: %v", tools.ErrExecutionFailed, err)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: request failed: %v", tools.ErrExecutionFailed, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return string(raw), fmt.Errorf("%w: server returned %s", tools.ErrExecutionFailed, resp.Status)
	}
	return prettyJSON(raw), nil
}

func prettyJSON(raw []byte) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(bytes.TrimSpace(raw))
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(bytes.TrimSpace(raw))
	}
	return string(out)
}
