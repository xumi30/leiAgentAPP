package gemini

// ChatCompletionRequest Gemini 聊天请求
type ChatCompletionRequest struct {
	Contents         []Content         `json:"contents"`
	GenerationConfig *GenerationConfig `json:"generationConfig,omitempty"`
	SafetySettings   []SafetySetting   `json:"safetySettings,omitempty"`
	Tools            []Tool            `json:"tools,omitempty"`
	ToolConfig       *ToolConfig       `json:"toolConfig,omitempty"`
	CacheControl     *CacheControl     `json:"cacheControl,omitempty"`
}

// Content 对话内容
// type Content struct {
//     Role  string `json:"role"`  // user/model
//     Parts []Part `json:"parts"`
// }

// // Part 内容部分
// type Part struct {
//     Text      string      `json:"text,omitempty"`
//     InlineData *InlineData `json:"inlineData,omitempty"`
//     // 可扩展其他多模态类型
// }

// InlineData 内联数据(如图片)
type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64编码
}

// GenerationConfig 生成配置
type GenerationConfig struct {
	Temperature      float64         `json:"temperature,omitempty"`
	TopP             float64         `json:"topP,omitempty"`
	TopK             int             `json:"topK,omitempty"`
	MaxOutputTokens  int             `json:"maxOutputTokens,omitempty"`
	StopSequences    []string        `json:"stopSequences,omitempty"`
	ResponseMimeType string          `json:"responseMimeType,omitempty"`
	ResponseSchema   *ResponseSchema `json:"responseSchema,omitempty"`
	CandidateCount   int             `json:"candidateCount,omitempty"`
	Seed             int             `json:"seed,omitempty"`
}

// ResponseSchema 响应Schema
type ResponseSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

// SafetySetting 安全设置
type SafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// Tool 工具定义
type Tool struct {
	FunctionDeclarations []FunctionDeclaration `json:"functionDeclarations,omitempty"`
}

// FunctionDeclaration 函数声明
type FunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolConfig 工具配置
type ToolConfig struct {
	FunctionCallingConfig *FunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

// FunctionCallingConfig 函数调用配置
type FunctionCallingConfig struct {
	Mode             string   `json:"mode"` // auto/any/none
	AllowedFunctions []string `json:"allowedFunctions,omitempty"`
}

// CacheControl 缓存控制
type CacheControl struct {
	TTL string `json:"ttl"` // 如 "3600s"
}
