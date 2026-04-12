package browsertool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"leiAgent/internal/tools"
	"leiAgent/utils"
)

type BaiduBrowserSearchTool struct {
	pw *BrowserPlaywrightTool
}

func NewBaiduBrowserSearchTool() tools.Tool {
	return &BaiduBrowserSearchTool{
		pw: New().(*BrowserPlaywrightTool),
	}
}

func (t *BaiduBrowserSearchTool) Name() string {
	return "search_baidu_browser"
}

func (t *BaiduBrowserSearchTool) Description() string {
	return `Open Baidu in a real browser and perform a search in ONE tool call.

Use this tool when the user wants to:
- open Baidu
- search a query on Baidu
- see the real browser land on Baidu search results

This is faster and more reliable than manually orchestrating browser_playwright for:
- create_session
- goto homepage
- find the input box
- fill query
- press Enter

Behavior:
- Opens a browser session
- Navigates directly to the Baidu results page for the query
- Returns the sessionId, result page URL, and page observation snapshot
- Keeps the browser open by default so the user can continue viewing the page
`
}

func (t *BaiduBrowserSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The Baidu search query, e.g. 'agent'.",
			},
			"headless": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to use headless mode. Default false so the user can see the browser.",
			},
			"keep_open": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to keep the browser session open after search. Default true.",
			},
			"observe_limit": map[string]interface{}{
				"type":        "integer",
				"description": "How many links/inputs/buttons to include in the observation snapshot. Default 8.",
			},
			"locale": map[string]interface{}{
				"type":        "string",
				"description": "Optional locale for browser context, e.g. zh-CN.",
			},
		},
		"required": []string{"query"},
	}
}

func (t *BaiduBrowserSearchTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Baidu browser search result including sessionId, search URL, and observation snapshot.",
		"properties": map[string]interface{}{
			"sessionId": map[string]interface{}{
				"type":        "string",
				"description": "Browser session ID if the session is kept open.",
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "Current Baidu result page URL.",
			},
			"observation": map[string]interface{}{
				"type":        "object",
				"description": "Observed page state including title, headings, links, inputs, and buttons.",
			},
		},
	}
}

func (t *BaiduBrowserSearchTool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap(utils.ToolTopicBrowser, "一键打开百度并执行搜索，直接落到真实浏览器结果页，适合高频轻量网页操作。")
}

func (t *BaiduBrowserSearchTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *BaiduBrowserSearchTool) Execute(ctx context.Context, args string) (string, error) {
	if err := t.pw.ensureServerHealthy(ctx); err != nil {
		return "", err
	}

	args = utils.ExtractJSON(args)
	args = utils.EscapeRawNewlinesInJSONStrings(args)

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("%w: failed to parse arguments: %v", tools.ErrInvalidParams, err)
	}

	query, _ := params["query"].(string)
	if utils.IsBlank(query) {
		return "", fmt.Errorf("%w: query is required", tools.ErrInvalidParams)
	}

	headless := false
	if v, ok := params["headless"].(bool); ok {
		headless = v
	}

	keepOpen := true
	if v, ok := params["keep_open"].(bool); ok {
		keepOpen = v
	}

	observeLimit := 8
	if v, ok := params["observe_limit"].(float64); ok && int(v) > 0 {
		observeLimit = int(v)
	}

	createBody := map[string]interface{}{
		"headless": headless,
		"locale":   "zh-CN",
	}
	if v, ok := params["locale"]; ok {
		createBody["locale"] = v
	}

	createRaw, err := t.pw.postJSON(ctx, "/sessions", createBody)
	if err != nil {
		return createRaw, err
	}

	var createResp struct {
		OK        bool   `json:"ok"`
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal([]byte(createRaw), &createResp); err != nil {
		return createRaw, fmt.Errorf("%w: failed to parse create_session response: %v", tools.ErrExecutionFailed, err)
	}
	if utils.IsBlank(createResp.SessionID) {
		return createRaw, fmt.Errorf("%w: browser sessionId is empty", tools.ErrExecutionFailed)
	}

	searchURL := "https://www.baidu.com/s?wd=" + url.QueryEscape(query)
	gotoBody := map[string]interface{}{
		"sessionId": createResp.SessionID,
		"action":    "goto",
		"params": map[string]interface{}{
			"url":          searchURL,
			"waitUntil":    "domcontentloaded",
			"observe":      true,
			"observeLimit": observeLimit,
		},
	}

	gotoRaw, err := t.pw.postJSON(ctx, "/actions", gotoBody)
	if err != nil {
		if keepOpen {
			return gotoRaw, err
		}
		_, _ = t.pw.delete(ctx, "/sessions/"+createResp.SessionID)
		return gotoRaw, err
	}

	var gotoResp map[string]interface{}
	if err := json.Unmarshal([]byte(gotoRaw), &gotoResp); err != nil {
		return gotoRaw, nil
	}

	gotoResp["query"] = query
	gotoResp["searchEngine"] = "baidu"
	gotoResp["kept_open"] = keepOpen

	if !keepOpen {
		closeRaw, closeErr := t.pw.delete(ctx, "/sessions/"+createResp.SessionID)
		if closeErr == nil {
			var closeResp map[string]interface{}
			if json.Unmarshal([]byte(closeRaw), &closeResp) == nil {
				gotoResp["close"] = closeResp
			}
		}
		gotoResp["sessionId"] = ""
	}

	return prettyJSONMust(gotoResp), nil
}
