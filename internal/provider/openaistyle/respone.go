package openaistyle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ChatCompletionResponse 对话补全响应结构体
type ChatCompletionResponse struct {
	ID            string                 `json:"id"`                       // 任务ID
	RequestID     string                 `json:"request_id"`               // 请求ID
	Created       int64                  `json:"created"`                  // 创建时间(Unix时间戳)
	Model         string                 `json:"model"`                    // 模型名称
	Choices       []ChatCompletionChoice `json:"choices"`                  // 响应列表
	Usage         *TokenUsage            `json:"usage,omitempty"`          // Token使用统计
	VideoResult   []VideoResult          `json:"video_result,omitempty"`   // 视频生成结果
	WebSearch     []WebSearchResult      `json:"web_search,omitempty"`     // 网页搜索结果
	ContentFilter []ContentFilter        `json:"content_filter,omitempty"` // 内容安全信息
}

// ChatCompletionChoice 对话补全选择项
type ChatCompletionChoice struct {
	Index        int                    `json:"index"`         // 结果索引
	Message      *ChatCompletionMessage `json:"message"`       // 消息内容
	Delta        *ChatCompletionDelta   `json:"delta"`         // 增量内容(流式)
	FinishReason string                 `json:"finish_reason"` // 结束原因
}

// ChatCompletionMessage 对话补全消息
type ChatCompletionMessage struct {
	Role             string                   `json:"role"`                        // 角色
	Content          interface{}              `json:"content"`                     // 消息内容
	ReasoningContent string                   `json:"reasoning_content,omitempty"` // 思维链内容
	Audio            *AudioMessage            `json:"audio,omitempty"`             // 音频内容
	ToolCalls        []ChatCompletionToolCall `json:"tool_calls,omitempty"`        // 工具调用
}

// ChatCompletionDelta 对话补全增量内容(流式响应)
type ChatCompletionDelta struct {
	Role             string                   `json:"role,omitempty"`              // 角色
	Content          interface{}              `json:"content,omitempty"`           // 增量内容
	ReasoningContent string                   `json:"reasoning_content,omitempty"` // 思维链增量
	Audio            *AudioMessage            `json:"audio,omitempty"`             // 音频内容
	ToolCalls        []ChatCompletionToolCall `json:"tool_calls,omitempty"`        // 工具调用增量
}

// ChatCompletionToolCall 对话补全工具调用
type ChatCompletionToolCall struct {
	ID       string        `json:"id"`       // 工具调用ID
	Type     string        `json:"type"`     // 工具类型
	Function *FunctionCall `json:"function"` // 函数调用
	Index    int           `json:"index"`
	MCP      *MCPToolCall  `json:"mcp,omitempty"` // MCP工具调用
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string `json:"name"`      // 函数名称
	Arguments string `json:"arguments"` // 函数参数(JSON字符串)
}

// MCPToolCall MCP工具调用
type MCPToolCall struct {
	ID          string                 `json:"id"`               // MCP工具调用ID
	Type        string                 `json:"type"`             // 调用类型
	ServerLabel string                 `json:"server_label"`     // MCP服务器标签
	Name        string                 `json:"name"`             // 工具名称
	Arguments   string                 `json:"arguments"`        // 调用参数
	Error       string                 `json:"error,omitempty"`  // 错误信息
	Output      map[string]interface{} `json:"output,omitempty"` // 工具输出
}

// TokenUsage Token使用统计
type TokenUsage struct {
	PromptTokens        int                  `json:"prompt_tokens"`                   // 输入Token数
	CompletionTokens    int                  `json:"completion_tokens"`               // 输出Token数
	TotalTokens         int                  `json:"total_tokens"`                    // 总Token数
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"` // 输入Token详情
}

// PromptTokensDetails 输入Token详情
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"` // 缓存Token数
}

// VideoResult 视频生成结果
type VideoResult struct {
	URL           string `json:"url"`             // 视频链接
	CoverImageURL string `json:"cover_image_url"` // 视频封面链接
}

// WebSearchResult 网页搜索结果
type WebSearchResult struct {
	Icon        string `json:"icon"`         // 来源网站图标
	Title       string `json:"title"`        // 搜索结果标题
	Link        string `json:"link"`         // 网页链接
	Media       string `json:"media"`        // 媒体来源名称
	PublishDate string `json:"publish_date"` // 发布时间
	Content     string `json:"content"`      // 引用内容
	Refer       string `json:"refer"`        // 角标序号
}

// ContentFilter 内容安全信息
type ContentFilter struct {
	Role  string `json:"role"`  // 安全生效环节
	Level int    `json:"level"` // 严重程度(0-3)
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail 错误详情
type ErrorDetail struct {
	Code    string `json:"code"`    // 错误码
	Message string `json:"message"` // 错误信息
}

// ConvertToChatCompletionResponse 将 HTTP 响应转换为 ChatCompletionResponse 对象
func ConvertToChatCompletionResponse(resp *http.Response) (*ChatCompletionResponse, error) {
	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查是否为错误响应
	var errorResp ErrorResponse
	if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error.Code != "" {
		return nil, fmt.Errorf("API错误: %s - %s", errorResp.Error.Code, errorResp.Error.Message)
	}

	// 转换为成功响应
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &chatResp, nil
}
