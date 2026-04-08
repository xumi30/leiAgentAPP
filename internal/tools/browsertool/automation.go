package browsertool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"leiAgent/internal/tools"
	"leiAgent/utils"

	"github.com/chromedp/chromedp"
)

// BrowserAutomationTool executes LLM-produced JSON instructions against Chrome via CDP.
// Request context is only used to read chatID; all CDP work uses timeouts derived from tabCtx (see runtime.go).
type BrowserAutomationTool struct{}

func NewAutomationTool() tools.Tool {
	return &BrowserAutomationTool{}
}

func (t *BrowserAutomationTool) Name() string {
	return "browser_automation"
}

func (t *BrowserAutomationTool) Description() string {
	return "Execute structured browser instructions in an automated Chrome (CDP), one session per chat. " +
		"The LLM should combine the user goal with prior tool results (page_text, list_links) to choose the next action and parameters. " +
		"Recommended flow: navigate → list_links or page_text → click. " +
		"browser_operate opens the desktop browser only and does not create this session. " +
		"Requires Chrome/Chromium. Call close_session when done. " +
		"由模型根据用户意图与网页回传数据生成 JSON 指令；本工具只负责忠实执行。"
}

func (t *BrowserAutomationTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Instruction name from the LLM",
				"enum": []string{
					"navigate", "list_links", "wait_visible", "click", "scroll",
					"extract_text", "extract_text_all", "page_text", "close_session",
				},
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "navigate: target URL. Other actions: optional bootstrap URL if no session yet",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS selector for wait_visible, click, extract_text, extract_text_all",
			},
			"headless": map[string]interface{}{
				"type":        "boolean",
				"description": "When creating a new session only: false shows Chrome window. Default true",
			},
			"timeout_ms": map[string]interface{}{
				"type":        "integer",
				"description": "Operation timeout in ms",
			},
			"scroll_x": map[string]interface{}{"type": "integer", "description": "scroll: horizontal px (default 0)"},
			"scroll_y": map[string]interface{}{"type": "integer", "description": "scroll: vertical px (default 400)"},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "extract_text_all: max chunks (default 20, max 50). list_links: max after filter (default 40, max 120)",
			},
			"href_contains": map[string]interface{}{"type": "string", "description": "list_links: filter href"},
			"text_contains": map[string]interface{}{"type": "string", "description": "list_links: filter link text"},
			"max_chars":     map[string]interface{}{"type": "integer", "description": "page_text: max chars (default 12000, max 50000)"},
		},
		"required": []string{"action"},
	}
}

func (t *BrowserAutomationTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "JSON result for the LLM (success, action-specific fields, error if any)",
		"properties": map[string]interface{}{
			"success": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether the operation completed without error",
			},
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The action that was executed",
			},
			"error": map[string]interface{}{
				"type":        "string",
				"description": "Error message if success=false",
			},
		},
		"additionalProperties": true,
	}
}

func (t *BrowserAutomationTool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap(utils.ToolTopicBrowser, "通过 Chrome CDP 会话执行导航、点击、滚动、提取正文与链接等自动化操作。")
}

func (t *BrowserAutomationTool) Execute(ctx context.Context, args string) (string, error) {
	chatID, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok || strings.TrimSpace(chatID) == "" {
		return "", fmt.Errorf("browser_automation requires chatID in context")
	}
	_ = ctx // only chatID is used; never pass ctx to chromedp

	params, err := parseBrowserAutomationArgs(args)
	if err != nil {
		return "", err
	}
	action := inferActionIfMissing(params)
	if action == "" {
		return "", fmt.Errorf("missing action")
	}
	action = strings.TrimSpace(strings.ToLower(action))

	switch action {
	case "close_session":
		m := closeSession(chatID)
		b, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	case "navigate":
		return t.doNavigate(chatID, params)
	case "wait_visible":
		return t.doWaitVisible(chatID, params)
	case "click":
		return t.doClick(chatID, params)
	case "scroll":
		return t.doScroll(chatID, params)
	case "extract_text":
		return t.doExtractText(chatID, params, false)
	case "extract_text_all":
		return t.doExtractText(chatID, params, true)
	case "page_text":
		return t.doPageText(chatID, params)
	case "list_links":
		return t.doListLinks(chatID, params)
	default:
		return "", fmt.Errorf("unknown action %q", action)
	}
}

func marshalResult(m map[string]interface{}, err error) (string, error) {
	if err != nil {
		m["success"] = false
		m["error"] = err.Error()
		b, e := json.MarshalIndent(m, "", "  ")
		if e != nil {
			return "", e
		}
		return string(b), err
	}
	m["success"] = true
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return "", e
	}
	return string(b), nil
}

func (t *BrowserAutomationTool) opTimeout(params map[string]interface{}, def time.Duration) time.Duration {
	if v, ok := params["timeout_ms"].(float64); ok && v > 0 {
		return time.Duration(v) * time.Millisecond
	}
	return def
}

func (t *BrowserAutomationTool) doNavigate(chatID string, params map[string]interface{}) (string, error) {
	raw := getStringParam(params, "url", "URL", "link", "href", "page_url")
	if raw == "" {
		return "", fmt.Errorf("navigate requires url")
	}
	if err := validateHTTPURL(raw); err != nil {
		return "", err
	}
	headless := envHeadlessDefaultTrue()
	if v, ok := params["headless"].(bool); ok {
		headless = v
	}
	if _, err := getOrCreateSession(chatID, headless); err != nil {
		return marshalResult(map[string]interface{}{"action": "navigate"}, err)
	}
	err := runChrome(chatID, t.opTimeout(params, 45*time.Second),
		chromedp.Navigate(raw),
		chromedp.WaitReady("body"),
	)
	if err != nil {
		return marshalResult(map[string]interface{}{"action": "navigate", "url": raw}, err)
	}
	return marshalResult(map[string]interface{}{
		"action": "navigate",
		"url":    raw,
		"note":   "session kept for this chat; close_session when finished",
	}, nil)
}

func (t *BrowserAutomationTool) ensureBootstrap(chatID string, params map[string]interface{}) error {
	if _, err := sessionMustExist(chatID); err == nil {
		return nil
	}
	raw := getStringParam(params, "url", "URL", "link", "href", "page_url")
	if raw == "" {
		return errNoAutomationSession()
	}
	if err := validateHTTPURL(raw); err != nil {
		return err
	}
	headless := envHeadlessDefaultTrue()
	if v, ok := params["headless"].(bool); ok {
		headless = v
	}
	if _, err := getOrCreateSession(chatID, headless); err != nil {
		return err
	}
	return runChrome(chatID, t.opTimeout(params, 45*time.Second),
		chromedp.Navigate(raw),
		chromedp.WaitReady("body"),
	)
}

func (t *BrowserAutomationTool) doWaitVisible(chatID string, params map[string]interface{}) (string, error) {
	sel, err := requireSelector(params)
	if err != nil {
		return "", err
	}
	if err := t.ensureBootstrap(chatID, params); err != nil {
		return marshalResult(map[string]interface{}{"action": "wait_visible", "selector": sel}, err)
	}
	err = runChrome(chatID, t.opTimeout(params, 30*time.Second), chromedp.WaitVisible(sel, chromedp.ByQuery))
	return marshalResult(map[string]interface{}{"action": "wait_visible", "selector": sel}, err)
}

func (t *BrowserAutomationTool) doClick(chatID string, params map[string]interface{}) (string, error) {
	sel, err := requireSelector(params)
	if err != nil {
		return "", err
	}
	if err := t.ensureBootstrap(chatID, params); err != nil {
		return marshalResult(map[string]interface{}{"action": "click", "selector": sel}, err)
	}
	err = runChrome(chatID, t.opTimeout(params, 25*time.Second),
		chromedp.WaitVisible(sel, chromedp.ByQuery),
		chromedp.Click(sel, chromedp.NodeVisible),
	)
	return marshalResult(map[string]interface{}{"action": "click", "selector": sel}, err)
}

func (t *BrowserAutomationTool) doScroll(chatID string, params map[string]interface{}) (string, error) {
	sx := 0.0
	if v, ok := params["scroll_x"].(float64); ok {
		sx = v
	}
	sy := 400.0
	if v, ok := params["scroll_y"].(float64); ok {
		sy = v
	}
	if err := t.ensureBootstrap(chatID, params); err != nil {
		return marshalResult(map[string]interface{}{"action": "scroll"}, err)
	}
	script := fmt.Sprintf(`window.scrollBy(%v, %v)`, sx, sy)
	err := runChrome(chatID, t.opTimeout(params, 15*time.Second), chromedp.Evaluate(script, nil))
	return marshalResult(map[string]interface{}{
		"action": "scroll", "scroll_x": sx, "scroll_y": sy,
	}, err)
}

func (t *BrowserAutomationTool) doExtractText(chatID string, params map[string]interface{}, all bool) (string, error) {
	sel, err := requireSelector(params)
	if err != nil {
		return "", err
	}
	if err := t.ensureBootstrap(chatID, params); err != nil {
		return marshalResult(map[string]interface{}{
			"action":   map[bool]string{true: "extract_text_all", false: "extract_text"}[all],
			"selector": sel,
		}, err)
	}
	actionName := "extract_text"
	if all {
		actionName = "extract_text_all"
	}
	op := t.opTimeout(params, 25*time.Second)

	if !all {
		var text string
		err = runChrome(chatID, op,
			chromedp.WaitVisible(sel, chromedp.ByQuery),
			chromedp.Text(sel, &text, chromedp.NodeVisible, chromedp.ByQuery),
		)
		if err != nil {
			return marshalResult(map[string]interface{}{"action": actionName, "selector": sel}, err)
		}
		return marshalResult(map[string]interface{}{
			"action": actionName, "selector": sel, "text": strings.TrimSpace(text),
		}, nil)
	}

	limit := 20
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var texts []string
	script := fmt.Sprintf(`(function(){
  var sel = %s;
  var els = document.querySelectorAll(sel);
  var out = [];
  var lim = %d;
  for (var i = 0; i < els.length && out.length < lim; i++) {
    var t = (els[i].innerText || '').trim();
    if (t) out.push(t);
  }
  return out;
})()`, jsStringLiteral(sel), limit)

	err = runChrome(chatID, op, chromedp.Evaluate(script, &texts))
	if err != nil {
		return marshalResult(map[string]interface{}{"action": actionName, "selector": sel}, err)
	}
	return marshalResult(map[string]interface{}{
		"action": actionName, "selector": sel, "texts": texts, "count": len(texts),
	}, nil)
}

func (t *BrowserAutomationTool) doPageText(chatID string, params map[string]interface{}) (string, error) {
	if err := t.ensureBootstrap(chatID, params); err != nil {
		return marshalResult(map[string]interface{}{"action": "page_text"}, err)
	}
	maxChars := 12000
	if v, ok := params["max_chars"].(float64); ok {
		maxChars = int(v)
	}
	if maxChars <= 0 {
		maxChars = 12000
	}
	if maxChars > 50000 {
		maxChars = 50000
	}
	var body string
	err := runChrome(chatID, t.opTimeout(params, 30*time.Second),
		chromedp.Evaluate(`document.body ? document.body.innerText : ''`, &body),
	)
	if err != nil {
		return marshalResult(map[string]interface{}{"action": "page_text"}, err)
	}
	body = strings.TrimSpace(body)
	truncated := false
	if len([]rune(body)) > maxChars {
		r := []rune(body)
		body = string(r[:maxChars])
		truncated = true
	}
	return marshalResult(map[string]interface{}{
		"action": "page_text", "text": body, "truncated": truncated,
		"max_chars": maxChars, "char_count": len([]rune(body)),
	}, nil)
}

type linkRow struct {
	Href string `json:"href"`
	Text string `json:"text"`
}

func (t *BrowserAutomationTool) doListLinks(chatID string, params map[string]interface{}) (string, error) {
	if err := t.ensureBootstrap(chatID, params); err != nil {
		return marshalResult(map[string]interface{}{"action": "list_links"}, err)
	}
	limit := 40
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}
	if limit <= 0 {
		limit = 40
	}
	if limit > 120 {
		limit = 120
	}
	hrefNeedle := strings.ToLower(strings.TrimSpace(getStringParam(params, "href_contains", "hrefContains", "href_substring")))
	textNeedle := strings.ToLower(strings.TrimSpace(getStringParam(params, "text_contains", "textContains", "text_substring")))

	script := `(function(){
  var base = location.href;
  var out = [];
  var nodes = document.querySelectorAll('a[href]');
  var max = 600;
  for (var i = 0; i < nodes.length && out.length < max; i++) {
    var a = nodes[i];
    var href = a.getAttribute('href');
    if (!href) continue;
    try { href = new URL(href, base).href; } catch (e) { continue; }
    var text = (a.innerText || '').trim().replace(/\s+/g, ' ');
    if (text.length > 240) text = text.slice(0, 240) + '…';
    out.push({ href: href, text: text });
  }
  return out;
})()`

	var raw []linkRow
	err := runChrome(chatID, t.opTimeout(params, 35*time.Second), chromedp.Evaluate(script, &raw))
	if err != nil {
		return marshalResult(map[string]interface{}{"action": "list_links"}, err)
	}
	filtered := make([]linkRow, 0, len(raw))
	for _, row := range raw {
		if hrefNeedle != "" && !strings.Contains(strings.ToLower(row.Href), hrefNeedle) {
			continue
		}
		if textNeedle != "" && !strings.Contains(strings.ToLower(row.Text), textNeedle) {
			continue
		}
		filtered = append(filtered, row)
		if len(filtered) >= limit {
			break
		}
	}
	return marshalResult(map[string]interface{}{
		"action": "list_links", "links": filtered, "count": len(filtered),
		"href_contains": hrefNeedle, "text_contains": textNeedle,
		"note":            "Use list results to choose selector or next action",
		"total_collected": len(raw),
	}, nil)
}

func requireSelector(params map[string]interface{}) (string, error) {
	s := getStringParam(params, "selector", "Selector", "css", "css_selector", "elementSelector")
	if s == "" {
		return "", fmt.Errorf("selector is required for this action")
	}
	return s, nil
}

func jsStringLiteral(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
