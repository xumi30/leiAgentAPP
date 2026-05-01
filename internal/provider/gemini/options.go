package gemini

// Option 配置函数类型
type Option func(*ChatCompletionRequest)

// NewChatCompletionRequest 创建新请求
func NewChatCompletionRequest(opts ...Option) *ChatCompletionRequest {
	req := &ChatCompletionRequest{
		GenerationConfig: &GenerationConfig{
			Temperature:     0.7,
			TopP:            0.95,
			TopK:            40,
			MaxOutputTokens: 2048,
			CandidateCount:  1,
		},
	}
	for _, opt := range opts {
		opt(req)
	}
	return req
}

// WithContents 设置对话内容
func WithContents(contents []Content) Option {
	return func(r *ChatCompletionRequest) {
		r.Contents = contents
	}
}

// WithTemperature 设置温度
func WithTemperature(temp float64) Option {
	return func(r *ChatCompletionRequest) {
		r.GenerationConfig.Temperature = temp
	}
}

// WithMaxOutputTokens 设置最大输出token
func WithMaxOutputTokens(tokens int) Option {
	return func(r *ChatCompletionRequest) {
		r.GenerationConfig.MaxOutputTokens = tokens
	}
}

// WithResponseFormat 设置响应格式
func WithResponseFormat(mimeType string, schema *ResponseSchema) Option {
	return func(r *ChatCompletionRequest) {
		r.GenerationConfig.ResponseMimeType = mimeType
		r.GenerationConfig.ResponseSchema = schema
	}
}

// WithTools 设置工具
func WithTools(tools []Tool) Option {
	return func(r *ChatCompletionRequest) {
		r.Tools = tools
	}
}

// WithSafetySettings 设置安全设置
func WithSafetySettings(settings []SafetySetting) Option {
	return func(r *ChatCompletionRequest) {
		r.SafetySettings = settings
	}
}
