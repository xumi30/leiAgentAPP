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

func (t *SerpapiSearch) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Search results from SerpAPI containing organic results, knowledge graph, and other search engine data",
		"properties": map[string]interface{}{
			"search_metadata": map[string]interface{}{
				"type":        "object",
				"description": "Metadata about the search request",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Unique search ID",
						"example":     "1234567890",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"description": "Status of the search request",
						"example":     "Success",
					},
					"json_endpoint": map[string]interface{}{
						"type":        "string",
						"description": "API endpoint used for the search",
						"example":     "/searches/1234567890.json",
					},
					"created_at": map[string]interface{}{
						"type":        "string",
						"description": "Timestamp when the search was created",
						"example":     "2024-01-15 10:30:00 UTC",
					},
					"processed_at": map[string]interface{}{
						"type":        "string",
						"description": "Timestamp when the search was processed",
						"example":     "2024-01-15 10:30:01 UTC",
					},
					"google_url": map[string]interface{}{
						"type":        "string",
						"description": "URL of the Google search",
						"example":     "https://www.google.com/search?q=example",
					},
					"raw_html_file": map[string]interface{}{
						"type":        "string",
						"description": "Path to raw HTML file",
						"example":     "/searches/1234567890.html",
					},
					"total_time_taken": map[string]interface{}{
						"type":        "number",
						"description": "Total time taken for the search in seconds",
						"example":     1.23,
					},
				},
			},
			"search_parameters": map[string]interface{}{
				"type":        "object",
				"description": "Parameters used for the search",
				"properties": map[string]interface{}{
					"engine": map[string]interface{}{
						"type":        "string",
						"description": "Search engine used",
						"example":     "google",
					},
					"q": map[string]interface{}{
						"type":        "string",
						"description": "Search query",
						"example":     "example query",
					},
					"num": map[string]interface{}{
						"type":        "string",
						"description": "Number of results requested",
						"example":     "10",
					},
				},
			},
			"search_information": map[string]interface{}{
				"type":        "object",
				"description": "Information about the search results",
				"properties": map[string]interface{}{
					"query_displayed": map[string]interface{}{
						"type":        "string",
						"description": "Query as displayed in search results",
						"example":     "example query",
					},
					"total_results": map[string]interface{}{
						"type":        "string",
						"description": "Total number of results found",
						"example":     "1000000",
					},
					"time_taken_displayed": map[string]interface{}{
						"type":        "number",
						"description": "Time taken to display results in seconds",
						"example":     0.5,
					},
				},
			},
			"organic_results": map[string]interface{}{
				"type":        "array",
				"description": "Organic search results",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"position": map[string]interface{}{
							"type":        "integer",
							"description": "Position of the result in search results",
							"example":     1,
						},
						"title": map[string]interface{}{
							"type":        "string",
							"description": "Title of the search result",
							"example":     "Example Title",
						},
						"link": map[string]interface{}{
							"type":        "string",
							"description": "URL of the search result",
							"example":     "https://example.com",
						},
						"snippet": map[string]interface{}{
							"type":        "string",
							"description": "Snippet or description of the search result",
							"example":     "This is a snippet of the search result...",
						},
					},
				},
			},
			"answer_box": map[string]interface{}{
				"type":        "object",
				"description": "Featured snippet or answer box if present",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Type of answer box",
						"example":     "organic_result",
					},
					"title": map[string]interface{}{
						"type":        "string",
						"description": "Title of the answer box",
						"example":     "Example Title",
					},
					"answer": map[string]interface{}{
						"type":        "string",
						"description": "Answer in the answer box",
						"example":     "This is the answer...",
					},
					"source": map[string]interface{}{
						"type":        "string",
						"description": "Source of the answer",
						"example":     "Example Source",
					},
				},
			},
			"knowledge_graph": map[string]interface{}{
				"type":        "object",
				"description": "Knowledge graph information if present",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type":        "string",
						"description": "Title in knowledge graph",
						"example":     "Example Entity",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Type of entity",
						"example":     "Organization",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Description of the entity",
						"example":     "This is a description of the entity...",
					},
					"website": map[string]interface{}{
						"type":        "string",
						"description": "Website of the entity",
						"example":     "https://example.com",
					},
					"images": map[string]interface{}{
						"type":        "string",
						"description": "URL of an image of the entity",
						"example":     "https://example.com/image.jpg",
					},
				},
			},
		},
	}
}
