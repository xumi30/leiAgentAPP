package searchFunctions

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/logging"
	"os"

	"github.com/serpapi/serpapi-golang"
)

type SerpapiSearch struct{}

func NewSerpapiSearch() *SerpapiSearch {
	return &SerpapiSearch{}
}

func (t *SerpapiSearch) Name() string {
	return "serpapi_search"
}

func (t *SerpapiSearch) Description() string {
	return `
Search the web for general information using SerpAPI.

DO NOT use this tool for:
- Financial market data
- Weather queries
- Structured data queries

Use specialized tools instead when available.

Only use this tool when:
- The query is about general knowledge
- No specialized tool can solve the problem

Supported search engines: google, baidu, bing, yahoo, duckduckgo
`
}

func (t *SerpapiSearch) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query",
			},
			"num_results": map[string]interface{}{
				"type":        "integer",
				"description": "Max results",
			},
			"engine": map[string]interface{}{
				"type":        "string",
				"description": "Search engine to use (google, baidu, bing, yahoo, etc.)",
				"default":     "google",
				"enum":        []string{"google", "baidu", "bing", "yahoo", "duckduckgo"},
			},
		},
		"required": []string{"query"},
	}
}

func (t *SerpapiSearch) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *SerpapiSearch) Execute(ctx context.Context, args string) (string, error) {
	// 解析输入参数
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("解析参数失败: %v", err)
	}

	// 获取查询字符串
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("缺少必需的 query 参数")
	}

	// 获取结果数量，默认为5
	numResults := 5
	if num, ok := params["num_results"].(float64); ok {
		numResults = int(num)
	}

	// 获取搜索引擎，默认为google
	engine := "google"
	if eng, ok := params["engine"].(string); ok && eng != "" {
		engine = eng
	}

	// 初始化 SerpAPI 客户端
	setting := serpapi.NewSerpApiClientSetting(os.Getenv("SERPAPI_KEY"))
	setting.Engine = engine
	client := serpapi.NewClient(setting)

	// 设置搜索参数
	parameter := map[string]string{
		"q":      query,
		"num":    fmt.Sprintf("%d", numResults),
		"engine": engine,
	}

	// 执行搜索
	results, err := client.Search(parameter)
	if err != nil {
		logging.Error("搜索失败: %v", err)
		return "", fmt.Errorf("搜索失败: %v", err)
	}

	// 将结果转换为JSON格式
	jsonResults, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		logging.Error("序列化结果失败: %v", err)
		return "", fmt.Errorf("序列化结果失败: %v", err)
	}

	return string(jsonResults), nil
}

// Query Rewrite（模拟LLM思考）
func (t *SerpapiSearch) generateQueries(query string) []string {
	return []string{
		query,
		query + " 是什么",
		query + " 原理",
		query + " 教程",
		query + " 最新",
	}
}
