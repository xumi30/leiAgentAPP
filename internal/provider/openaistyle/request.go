package openaistyle

const (
	// RoleSystem 系统角色
	RoleSystem = "system"
	// RoleUser 用户角色
	RoleUser = "user"
	// RoleAssistant 机器人角色
	RoleAssistant = "assistant"
	// RoleTool 工具角色
	RoleTool = "tool"

	// 工具类型: function/retrieval/web_search/mcp
	ToolTypeFunction  = "function"
	ToolTypeRetrieval = "retrieval"
	ToolTypeWebSearch = "web_search"
	ToolTypeMCP       = "mcp"

	// 是否开启思维链: enabled/disabled
	ThinkingEnabled  = "enabled"
	ThinkingDisabled = "disabled"

	// 内容类型: text/input_audio
	ContentTypeText       = "text"
	ContentTypeInputAudio = "input_audio"

	ToolChoiceEnabled = "auto"
)

// ChatCompletionRequest 对话补全请求基础结构体
// ChatCompletionRequest 对话补全请求基础结构体
type ChatCompletionRequest struct {
	Model            string          `json:"model"`                       // 模型名称
	Messages         []ChatMessage   `json:"messages"`                    // 消息列表
	Stream           bool            `json:"stream,omitempty"`            // 是否流式输出
	Temperature      *float64        `json:"temperature,omitempty"`       // 采样温度
	TopP             *float64        `json:"top_p,omitempty"`             // 核采样参数
	MaxTokens        *int            `json:"max_tokens,omitempty"`        // 最大token数
	DoSample         *bool           `json:"do_sample,omitempty"`         // 是否启用采样
	Tools            []Tool          `json:"tools,omitempty"`             // 工具列表
	ToolChoice       *ToolChoice     `json:"tool_choice,omitempty"`       // 工具选择策略
	Stop             []string        `json:"stop,omitempty"`              // 停止词
	RequestID        string          `json:"request_id,omitempty"`        // 请求ID
	UserID           string          `json:"user_id,omitempty"`           // 用户ID
	Thinking         *ChatThinking   `json:"thinking,omitempty"`          // 思维链配置
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"` // 频率惩罚
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`  // 存在惩罚
	ResponseFormat   *ResponseFormat `json:"response_format,omitempty"`   // 响应格式
	Seed             *int            `json:"seed,omitempty"`              // 随机种子
	Enablesearch     bool            `json:"enable_search,omitempty"`     // 是否启用搜索QWEN
	EnableThinking   *bool           `json:"enable_thinking,omitempty"`     // 百炼/DashScope Qwen 思考开关（兼容 OpenAI 扩展字段）
}

// ChatMessage 对话消息结构体
type ChatMessage struct {
	Role       string        `json:"role"`                   // 角色: system/user/assistant/tool
	Content    interface{}   `json:"content"`                // 消息内容，可以是字符串或多模态内容数组
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`   // 工具调用信息
	ToolCallID string        `json:"tool_call_id,omitempty"` // 工具调用ID
	Audio      *AudioMessage `json:"audio,omitempty"`        // 音频消息(仅GLM-4-Voice)
}

// Tool 工具接口
type Tool struct {
	Type      string     `json:"type"`                 // 工具类型: function/retrieval/web_search/mcp
	Function  *Function  `json:"function,omitempty"`   // 函数工具
	Retrieval *Retrieval `json:"retrieval,omitempty"`  // 检索工具
	WebSearch *WebSearch `json:"web_search,omitempty"` // 网络搜索工具
	MCP       *MCP       `json:"mcp,omitempty"`        // MCP工具
}

// Function 函数工具定义
type Function struct {
	Name        string                 `json:"name"`        // 函数名称
	Description string                 `json:"description"` // 函数描述
	Parameters  map[string]interface{} `json:"parameters"`  // 参数定义(JSON Schema)
	Arguments   map[string]interface{} `json:"arguments"`   // 函数参数
}

// Retrieval 检索工具定义
type Retrieval struct {
	KnowledgeID    string `json:"knowledge_id"`              // 知识库ID
	PromptTemplate string `json:"prompt_template,omitempty"` // 提示模板
}

// WebSearch 网络搜索工具定义
type WebSearch struct {
	Enable              bool   `json:"enable"`                          // 是否启用搜索
	SearchEngine        string `json:"search_engine"`                   // 搜索引擎类型
	SearchQuery         string `json:"search_query,omitempty"`          // 强制触发搜索
	SearchIntent        string `json:"search_intent,omitempty"`         // 搜索意图识别
	Count               *int   `json:"count,omitempty"`                 // 返回结果条数
	SearchDomainFilter  string `json:"search_domain_filter,omitempty"`  // 域名过滤
	SearchRecencyFilter string `json:"search_recency_filter,omitempty"` // 时间范围过滤
	ContentSize         string `json:"content_size,omitempty"`          // 内容大小控制
	ResultSequence      string `json:"result_sequence,omitempty"`       // 结果顺序
	SearchResult        bool   `json:"search_result,omitempty"`         // 是否返回搜索详情
	RequireSearch       bool   `json:"require_search,omitempty"`        // 是否强制搜索
	SearchPrompt        string `json:"search_prompt,omitempty"`         // 搜索提示词
}

// MCP MCP工具定义
type MCP struct {
	ServerLabel   string            `json:"server_label"`             // MCP服务器标识
	ServerURL     string            `json:"server_url,omitempty"`     // MCP服务器地址
	TransportType string            `json:"transport_type,omitempty"` // 传输类型
	AllowedTools  []string          `json:"allowed_tools,omitempty"`  // 允许的工具集合
	Headers       map[string]string `json:"headers,omitempty"`        // 鉴权信息
}

// ToolChoice 工具选择策略
type ToolChoice struct {
	Type string `json:"type"` // 目前仅支持 "auto"
}

// ToolCall 工具调用信息
type ToolCall struct {
	ID       string    `json:"id"`       // 工具调用ID
	Type     string    `json:"type"`     // 工具类型
	Function *Function `json:"function"` // 函数调用信息
	Index    int       `json:"index"`    // 工具调用索引
}

// ChatThinking 思维链配置
type ChatThinking struct {
	Type          string `json:"type"`                      // 是否开启思维链: enabled/disabled
	ClearThinking bool   `json:"clear_thinking,omitempty"` // 是否清除历史思维链
}

// VisionMultimodalContent 视觉多模态内容
type VisionMultimodalContent struct {
	Type     string    `json:"type"`                // 内容类型: text/image_url/video_url/file_url
	Text     string    `json:"text,omitempty"`      // 文本内容
	ImageURL *MediaURL `json:"image_url,omitempty"` // 图片URL
	VideoURL *MediaURL `json:"video_url,omitempty"` // 视频URL
	FileURL  *MediaURL `json:"file_url,omitempty"`  // 文件URL
}

// AudioMultimodalContent 音频多模态内容
type AudioMultimodalContent struct {
	Type       string      `json:"type"`                  // 内容类型: text/input_audio
	Text       string      `json:"text,omitempty"`        // 文本内容
	InputAudio *InputAudio `json:"input_audio,omitempty"` // 音频输入
}

// MediaURL 媒体URL结构
type MediaURL struct {
	URL string `json:"url"` // 媒体URL地址
}

// InputAudio 音频输入结构
type InputAudio struct {
	Data   string `json:"data"`   // base64编码的音频数据
	Format string `json:"format"` // 音频格式: wav/mp3
}

// AudioMessage 音频消息结构
type AudioMessage struct {
	ID        string `json:"id"`         // 音频消息ID
	Data      string `json:"data"`       // base64编码的音频数据
	ExpiresAt string `json:"expires_at"` // 过期时间
}

// ResponseFormat 响应格式
type ResponseFormat struct {
	Type string `json:"type"` // text 或 json_object
}
