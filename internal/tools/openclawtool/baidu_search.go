package openclawtool

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/openclawskill"
	"leiAgent/internal/tools"
	"leiAgent/utils"
	"os/exec"
	"strconv"
	"strings"
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
		return "", fmt.Errorf("未安装 baidu-search skill：请在设置页粘贴 `npx clawhub@latest install baidu-search` 安装")
	}
	skill = openclawskill.CheckRequirements(skill)
	if !skill.Ready {
		return "", fmt.Errorf("baidu-search skill 尚不可用：%s", skill.StatusDetail)
	}
	script, err := openclawskill.BaiduSearchScriptPath()
	if err != nil {
		return "", err
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
