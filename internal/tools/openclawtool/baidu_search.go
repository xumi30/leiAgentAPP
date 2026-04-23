package openclawtool

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/openclawskill"
	"leiAgent/internal/tools"
	"leiAgent/utils"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type BaiduSearchTool struct{}

func NewBaiduSearchTool() tools.Tool {
	return &BaiduSearchTool{}
}

func (t *BaiduSearchTool) Name() string {
	return "openclaw_baidu_search"
}

func (t *BaiduSearchTool) Description() string {
	return `Search the Chinese web through the installed OpenClaw/ClawHub baidu-search skill.

Use this for current Chinese-language information, documentation, news, company/product research, and topics where Baidu coverage is more relevant.
Requires the baidu-search skill installed under ./skills and BAIDU_API_KEY configured in the app environment.`
}

func (t *BaiduSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query.",
			},
			"count": map[string]interface{}{
				"type":        "integer",
				"description": "Number of results to return, 1-50. Defaults to 10.",
			},
			"freshness": map[string]interface{}{
				"type":        "string",
				"description": "Optional freshness filter: pd, pw, pm, py, or YYYY-MM-DDtoYYYY-MM-DD.",
			},
		},
		"required": []string{"query"},
	}
}

func (t *BaiduSearchTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Query     string `json:"query"`
		Count     int    `json:"count"`
		Freshness string `json:"freshness"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("解析搜索参数失败：%w", err)
	}
	params.Query = strings.TrimSpace(params.Query)
	if params.Query == "" {
		return "", fmt.Errorf("query is required")
	}
	if params.Count <= 0 {
		params.Count = 10
	}
	if params.Count > 50 {
		params.Count = 50
	}

	skill, ok := openclawskill.Find("baidu-search")
	if !ok {
		return "", fmt.Errorf("未安装 baidu-search skill：请在设置页粘贴 `claw skill install official/baidu-search` 安装")
	}
	skill = openclawskill.CheckRequirements(skill)
	if !skill.Ready {
		return "", fmt.Errorf("baidu-search skill 尚不可用：%s", skill.StatusDetail)
	}

	// Prefer the legacy script-based skill. If the installed skill does not ship scripts/search.py
	// (e.g. official/baidu-search), fall back to calling the Baidu API directly from Go.
	script, err := openclawskill.BaiduSearchScriptPath()
	if err != nil {
		return baiduSearchDirect(params.Query, params.Count, params.Freshness)
	}
	python := "python3"
	if skill.PythonDeps != nil {
		if candidate := openclawskill.SkillPythonPath(skill.Path); candidate != "" {
			python = candidate
		}
	}

	payload := map[string]interface{}{
		"query": params.Query,
		"count": params.Count,
	}
	if strings.TrimSpace(params.Freshness) != "" {
		payload["freshness"] = strings.TrimSpace(params.Freshness)
	}
	jsonArg, err := openclawskill.MarshalForCommand(payload)
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, python, script, jsonArg)
	cmd.Dir = skill.Path
	cmd.Env = openclawskill.EnvForSkill(skill)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("baidu-search 执行失败：%s", text)
	}
	return normalizeOutput(text, params.Count), nil
}

func baiduSearchDirect(query string, count int, freshness string) (string, error) {
	apiKey, _ := openclawskill.ResolveEnvValue("BAIDU_API_KEY")
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", fmt.Errorf("未设置 BAIDU_API_KEY：请在设置页填写 openclaw.env 或设置环境变量 BAIDU_API_KEY")
	}

	searchFilter := map[string]interface{}{}
	freshness = strings.TrimSpace(freshness)
	if freshness != "" {
		now := time.Now()
		endDate := now.Add(24 * time.Hour).Format("2006-01-02")
		var startDate string
		switch freshness {
		case "pd":
			startDate = now.Add(-24 * time.Hour).Format("2006-01-02")
		case "pw":
			startDate = now.Add(-6 * 24 * time.Hour).Format("2006-01-02")
		case "pm":
			startDate = now.Add(-30 * 24 * time.Hour).Format("2006-01-02")
		case "py":
			startDate = now.Add(-364 * 24 * time.Hour).Format("2006-01-02")
		default:
			pattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}to\d{4}-\d{2}-\d{2}$`)
			if pattern.MatchString(freshness) {
				parts := strings.SplitN(freshness, "to", 2)
				startDate, endDate = parts[0], parts[1]
			} else {
				return "", fmt.Errorf("freshness (%s) 必须是 pd/pw/pm/py 或 YYYY-MM-DDtoYYYY-MM-DD", freshness)
			}
		}
		searchFilter = map[string]interface{}{
			"range": map[string]interface{}{
				"page_time": map[string]interface{}{
					"gte": startDate,
					"lt":  endDate,
				},
			},
		}
	}

	body := map[string]interface{}{
		"messages": []map[string]string{
			{"content": query, "role": "user"},
		},
		"search_source":        "baidu_search_v2",
		"resource_type_filter": []map[string]interface{}{{"type": "web", "top_k": count}},
		"search_filter":        searchFilter,
	}

	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, "https://qianfan.baidubce.com/v2/ai_search/web_search", strings.NewReader(string(b)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("X-Appbuilder-From", "openclaw")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("百度搜索请求失败：%w", err)
	}
	defer resp.Body.Close()

	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", fmt.Errorf("百度搜索响应解析失败：%w", err)
	}
	if resp.StatusCode != 200 {
		if msg, _ := raw["message"].(string); strings.TrimSpace(msg) != "" {
			return "", fmt.Errorf("百度搜索 API 错误：%s", msg)
		}
		out, _ := json.Marshal(raw)
		return "", fmt.Errorf("百度搜索 API 错误：status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(out)))
	}

	refs, _ := raw["references"].([]interface{})
	if refs == nil {
		refs = []interface{}{}
	}
	// Remove "snippet" for consistency with legacy script.
	for i := range refs {
		if m, ok := refs[i].(map[string]interface{}); ok {
			delete(m, "snippet")
		}
	}
	out, _ := json.MarshalIndent(refs, "", "  ")
	return string(out), nil
}

func (t *BaiduSearchTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Baidu AI Search result returned by the OpenClaw baidu-search skill.",
	}
}

func (t *BaiduSearchTool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap(utils.ToolTopicSearch, "通过 OpenClaw baidu-search skill 调用百度 AI 搜索。")
}

func normalizeOutput(text string, count int) string {
	if text == "" {
		return "{}"
	}
	var raw interface{}
	if err := json.Unmarshal([]byte(text), &raw); err == nil {
		out, _ := json.MarshalIndent(raw, "", "  ")
		return string(out)
	}
	data, _ := json.MarshalIndent(map[string]string{
		"count":  strconv.Itoa(count),
		"output": text,
	}, "", "  ")
	return string(data)
}
