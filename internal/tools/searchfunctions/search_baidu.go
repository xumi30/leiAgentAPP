package searchfunctions

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/utils"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type BaiduSearchTool struct{}

func NewBaiduSearchTool() *BaiduSearchTool {
	return &BaiduSearchTool{}
}

func (t *BaiduSearchTool) Name() string {
	return "baidu_search"
}

func (t *BaiduSearchTool) Description() string {
	return `
Search the web for general information.

DO NOT use this tool for:
- Financial market data
- Weather queries
- Structured data queries

Use specialized tools instead when available.

Only use this tool when:
- The query is about general knowledge
- No specialized tool can solve the problem
`
}
func (t *BaiduSearchTool) Parameters() map[string]interface{} {
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
		},
		"required": []string{"query"},
	}
}

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Content string `json:"content,omitempty"`
}

func (t *BaiduSearchTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *BaiduSearchTool) Execute(ctx context.Context, args string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", err
	}

	query := params["query"].(string)

	numResults := 5
	if num, ok := params["num_results"].(float64); ok {
		numResults = int(num)
	}

	// Step1: Query Rewrite（模拟思考）
	queries := t.generateQueries(query)

	resultMap := make(map[string]SearchResult)

	// Step2: 多轮搜索
	for _, q := range queries {
		results, err := t.searchOnce(ctx, q, numResults)
		if err != nil {
			continue
		}

		for _, r := range results {
			if t.isHighQuality(r) {
				resultMap[r.URL] = r
			}
		}
	}

	// 转slice
	finalResults := make([]SearchResult, 0, len(resultMap))
	for _, v := range resultMap {
		finalResults = append(finalResults, v)
	}

	// Step3: 二跳抓取（核心升级）
	for i := 0; i < len(finalResults) && i < 3; i++ {
		content := t.fetchPageContent(ctx, finalResults[i].URL)
		finalResults[i].Content = t.trimContent(content, 500)
	}

	jsonBytes, _ := json.MarshalIndent(map[string]interface{}{
		"results": finalResults,
	}, "", "  ")
	return string(jsonBytes), nil
}

// Query Rewrite（模拟LLM思考）
func (t *BaiduSearchTool) generateQueries(query string) []string {
	return []string{
		query,
		query + " 是什么",
		query + " 原理",
		query + " 教程",
		query + " 最新",
	}
}

// 单次搜索
func (t *BaiduSearchTool) searchOnce(ctx context.Context, query string, num int) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://www.baidu.com/s?wd=%s&rn=%d",
		url.QueryEscape(query), num)

	req, _ := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []SearchResult

	doc.Find(".result").Each(func(i int, s *goquery.Selection) {
		title := strings.TrimSpace(s.Find("h3").Text())
		link, _ := s.Find("a").Attr("href")
		snippet := strings.TrimSpace(s.Find(".c-abstract").Text())

		if title != "" && link != "" {
			results = append(results, SearchResult{
				Title:   title,
				URL:     link,
				Snippet: snippet,
			})
		}
	})

	return results, nil
}

// 打开网页抓正文（关键能力）
func (t *BaiduSearchTool) fetchPageContent(ctx context.Context, pageURL string) string {
	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return ""
	}

	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return ""
	}

	// 粗暴抽正文（去掉script/style）
	doc.Find("script, style").Remove()

	text := strings.TrimSpace(doc.Text())

	return text
}

// 内容裁剪（防止token爆炸）
func (t *BaiduSearchTool) trimContent(content string, maxLen int) string {
	if len(content) > maxLen {
		return content[:maxLen]
	}
	return content
}

// 质量过滤
func (t *BaiduSearchTool) isHighQuality(r SearchResult) bool {
	if len(r.Title) < 5 {
		return false
	}
	if len(r.Snippet) < 20 {
		return false
	}
	return true
}
func (t *BaiduSearchTool) SimpleInfo() map[string]string {
	return utils.SimpleInfoMap("百度搜索", "抓取百度网页搜索结果，返回标题、链接与摘要等通用检索信息。")
}

func (t *BaiduSearchTool) Results() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": "Baidu search results (wrapped object).",
		"properties": map[string]interface{}{
			"results": map[string]interface{}{
				"type":        "array",
				"description": "Array of search results from Baidu search",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "Title of the search result",
							"example":     "Example Article Title",
						},
						"url": map[string]interface{}{
							"type":        "string",
							"description": "URL of the search result",
							"example":     "https://example.com/article",
						},
						"snippet": map[string]interface{}{
							"type":        "string",
							"description": "Brief description or snippet of the search result",
							"example":     "This is a brief description of the article content...",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Full content of the page (truncated to 500 characters for top 3 results)",
							"example":     "This is the full content of the page...",
						},
					},
				},
			},
		},
	}
}
