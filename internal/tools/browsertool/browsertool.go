package browsertool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"
)

// BrowserPlaywrightTool proxies browser automation to the Node.js Playwright server under frontend/.
// This Go tool is intentionally thin: it only validates/normalizes args and forwards them via HTTP JSON.
type BrowserPlaywrightTool struct {
	baseURL string
	client  *http.Client
}

var playwrightAutoStartMu sync.Mutex

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
- Default server URL: http://127.0.0.1:3111 (override with env PW_SERVER_URL)
- When using the default local server, this tool will auto-start or repair the Playwright server if it is not running yet.
- Recommended repair command:
  cd frontend && npm install && npm run playwright:server
- For generic search-engine browsing, prefer opening a Baidu search results URL directly unless the user explicitly asks for Google.
- For ordinary search tasks, stop after the results page is open. Do not inspect links, buttons, or page structure unless the user explicitly asks for analysis.

Core workflow (always do this):
1) operation=create_session  -> get sessionId
2) operation=action         -> run one action at a time using the same sessionId
3) operation=close_session  -> close the session to release resources

Session policy:
- If the user asks to open or view a webpage, prefer keeping the browser session open at the end.
- Only close the session when the user explicitly asks to close it, or when the task is clearly a one-off extraction/screenshot job.

Supported actions (operation=action):
- goto:        params.url (string, required), params.waitUntil (optional: "load"|"domcontentloaded"|"networkidle"), params.observe (optional bool, default true)
- wait_for_selector: params.selector (string, required)
- click:       params.selector (string, required), params.observe (optional bool, default true)
- click_text:  params.text (string, required), params.exact (optional bool, default true), params.role (optional: "link"|"button"|"tab"|"menuitem"), params.observe (optional bool, default true)
- fill:        params.selector (string, required), params.value (string, required), params.observe (optional bool, default false)
- press:       params.selector (string, required), params.key (string, required; e.g. "Enter"), params.observe (optional bool, default true)
- text:        params.selector (string, required) -> returns textContent
- list_links:  params.limit (optional integer, default 20) -> returns visible-ish link text/title/href candidates for navigation debugging
- list_inputs: params.limit (optional integer, default 12) -> returns form/input metadata such as name/id/placeholder/aria-label
- observe:     params.limit (optional integer, default 8) -> returns page title/url/headings/links/inputs/buttons snapshot
- content:     no params -> returns page HTML
- url:         no params -> returns current page url
- screenshot:  params.fullPage (bool, default true), params.path (optional string). Always returns base64 png in result.bytesBase64.
- evaluate:    params.expression (string, required). The expression MUST evaluate to a function to run in the page context.

Selector tips:
- Prefer stable selectors: data-testid, aria-label, input[name=...], button:has-text("...").
- For nav bars or tabs like "电影", prefer click_text before guessing CSS selectors.
- After goto/click/press, the tool usually returns an observation block with page title, links, and inputs. Use that before guessing selectors.
- If an action fails due to timing, call wait_for_selector first, then retry click/fill.

Failure/self-debug recipe:
1) observe or list_links/list_inputs to inspect what is currently on the page
2) wait_for_selector for the element you expect
3) screenshot (fullPage=true) or content if deeper inspection is still needed

Copy-paste examples:

Create session:
{"operation":"create_session","headless":true}

Open page:
{"operation":"action","sessionId":"<id>","action":"goto","params":{"url":"https://example.com","waitUntil":"domcontentloaded"}}

Wait + fill + press Enter:
{"operation":"action","sessionId":"<id>","action":"wait_for_selector","params":{"selector":"input[name='q']"}}
{"operation":"action","sessionId":"<id>","action":"fill","params":{"selector":"input[name='q']","value":"Playwright"}}
{"operation":"action","sessionId":"<id>","action":"press","params":{"selector":"input[name='q']","key":"Enter"}}

Click nav item by text:
{"operation":"action","sessionId":"<id>","action":"click_text","params":{"text":"电影","role":"link","exact":true}}

Inspect page links:
{"operation":"action","sessionId":"<id>","action":"list_links","params":{"limit":20}}

Inspect form fields:
{"operation":"action","sessionId":"<id>","action":"list_inputs","params":{"limit":10}}

Observe current page:
{"operation":"action","sessionId":"<id>","action":"observe","params":{"limit":8}}

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
				"description": "Required when operation=action. Supported: goto, click, click_text, fill, press, wait_for_selector, list_links, list_inputs, observe, evaluate, screenshot, content, text, url.",
			},
			"params": map[string]interface{}{
				"type":        "object",
				"description": "Action parameters object. Required fields depend on action. Examples: goto{url,waitUntil,observe}; click{selector,observe}; click_text{text,exact,role,observe}; fill{selector,value,observe}; press{selector,key,observe}; wait_for_selector{selector}; list_links{limit}; list_inputs{limit}; observe{limit}; text{selector}; evaluate{expression}; screenshot{fullPage,path}.",
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
		"description": "Result of browser operation. Many navigation actions also return an observation block containing title/url/headings/links/inputs/buttons to help the model choose the next step.",
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
	if err := t.ensureServerHealthy(ctx); err != nil {
		return "", err
	}

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
		if shouldKeepSessionOpen(ctx) {
			return prettyJSONMust(map[string]interface{}{
				"ok":        true,
				"sessionId": sid,
				"closed":    false,
				"kept_open": true,
				"reason":    "session kept open because the user goal is to open or view the page",
			}), nil
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

func shouldKeepSessionOpen(ctx context.Context) bool {
	goal, _ := ctx.Value(utils.UserGoalString).(string)
	goal = strings.ToLower(strings.TrimSpace(goal))
	if goal == "" {
		return false
	}

	closeHints := []string{
		"关闭", "关掉", "退出浏览器", "关闭浏览器", "close browser", "close the browser", "close session",
	}
	for _, hint := range closeHints {
		if strings.Contains(goal, hint) {
			return false
		}
	}

	openHints := []string{
		"打开", "访问", "进入", "看看网页", "打开网页", "打开页面",
		"open", "visit", "browse", "navigate", "open page", "open webpage",
	}
	for _, hint := range openHints {
		if strings.Contains(goal, hint) {
			return true
		}
	}

	return false
}

func (t *BrowserPlaywrightTool) ensureServerHealthy(ctx context.Context) error {
	if err := t.checkServerHealthy(ctx); err == nil {
		return nil
	}

	if !t.canAutoStartLocalServer() {
		return t.checkServerHealthy(ctx)
	}

	if err := t.autoStartServer(ctx); err != nil {
		return err
	}

	return t.checkServerHealthy(ctx)
}

func (t *BrowserPlaywrightTool) checkServerHealthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("%w: failed to create browser health-check request: %v", tools.ErrExecutionFailed, err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: Playwright server is not reachable at %s", tools.ErrExecutionFailed, t.baseURL)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := bytes.TrimSpace(raw)
		if len(msg) == 0 {
			return fmt.Errorf("%w: Playwright server health check failed with status %s. Server URL: %s", tools.ErrExecutionFailed, resp.Status, t.baseURL)
		}
		return fmt.Errorf("%w: Playwright server health check failed with status %s. Response: %s", tools.ErrExecutionFailed, resp.Status, string(msg))
	}

	return nil
}

func (t *BrowserPlaywrightTool) canAutoStartLocalServer() bool {
	return strings.HasPrefix(t.baseURL, "http://127.0.0.1:3111") || strings.HasPrefix(t.baseURL, "http://localhost:3111")
}

func (t *BrowserPlaywrightTool) autoStartServer(ctx context.Context) error {
	playwrightAutoStartMu.Lock()
	defer playwrightAutoStartMu.Unlock()

	healthCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	if err := t.checkServerHealthy(healthCtx); err == nil {
		return nil
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: Playwright server is not reachable at %s and auto-start failed to resolve working directory: %v", tools.ErrExecutionFailed, t.baseURL, err)
	}

	frontendDir := filepath.Join(projectRoot, "frontend")
	if stat, statErr := os.Stat(frontendDir); statErr != nil || !stat.IsDir() {
		return fmt.Errorf("%w: Playwright server is not reachable at %s and auto-start could not find frontend directory at %s", tools.ErrExecutionFailed, t.baseURL, frontendDir)
	}

	logDir := filepath.Join(projectRoot, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("%w: Playwright server is not reachable at %s and auto-start failed to create log directory: %v", tools.ErrExecutionFailed, t.baseURL, err)
	}
	logPath := filepath.Join(logDir, "playwright-server.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("%w: Playwright server is not reachable at %s and auto-start failed to open log file %s: %v", tools.ErrExecutionFailed, t.baseURL, logPath, err)
	}
	defer logFile.Close()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "npm install && npm run playwright:server")
	} else {
		cmd = exec.Command("bash", "-lc", "npm install && npm run playwright:server")
	}
	cmd.Dir = frontendDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	logging.Info("browser_playwright auto-starting Playwright server in %s", frontendDir)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: Playwright server is not reachable at %s and auto-start failed to launch server: %v", tools.ErrExecutionFailed, t.baseURL, err)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}

	deadline := time.Now().Add(60 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		probeCtx, probeCancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		lastErr = t.checkServerHealthy(probeCtx)
		probeCancel()
		if lastErr == nil {
			logging.Info("browser_playwright Playwright server became healthy after auto-start")
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: Playwright server auto-start was interrupted: %v", tools.ErrExecutionFailed, ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}

	return fmt.Errorf("%w: Playwright server is still unavailable at %s after auto-start attempt. Check %s. Last error: %v", tools.ErrExecutionFailed, t.baseURL, logPath, lastErr)
}

func (t *BrowserPlaywrightTool) postJSON(ctx context.Context, path string, body any) (string, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal request body: %v", tools.ErrInvalidParams, err)
	}

	var resp *http.Response
	for attempt := 0; attempt < 2; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+path, bytes.NewReader(b))
		if reqErr != nil {
			return "", fmt.Errorf("%w: failed to create request: %v", tools.ErrExecutionFailed, reqErr)
		}
		req.Header.Set("content-type", "application/json")

		resp, err = t.client.Do(req)
		if err == nil {
			break
		}
		if attempt == 0 && t.canAutoStartLocalServer() {
			if recoverErr := t.ensureServerHealthy(ctx); recoverErr == nil {
				continue
			}
		}
		return "", fmt.Errorf("%w: request failed: %v", tools.ErrExecutionFailed, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			return "", fmt.Errorf("%w: server returned %s", tools.ErrExecutionFailed, resp.Status)
		}
		return msg, fmt.Errorf("%w: server returned %s: %s", tools.ErrExecutionFailed, resp.Status, msg)
	}
	return prettyJSON(raw), nil
}

func (t *BrowserPlaywrightTool) delete(ctx context.Context, path string) (string, error) {
	var (
		resp *http.Response
		err  error
	)
	for attempt := 0; attempt < 2; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodDelete, t.baseURL+path, nil)
		if reqErr != nil {
			return "", fmt.Errorf("%w: failed to create request: %v", tools.ErrExecutionFailed, reqErr)
		}
		resp, err = t.client.Do(req)
		if err == nil {
			break
		}
		if attempt == 0 && t.canAutoStartLocalServer() {
			if recoverErr := t.ensureServerHealthy(ctx); recoverErr == nil {
				continue
			}
		}
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

func prettyJSONMust(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return prettyJSON(raw)
}
