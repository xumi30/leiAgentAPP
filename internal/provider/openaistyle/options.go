package openaistyle

// Option 定义配置函数类型
type Option func(*ChatCompletionRequest)

// WithModel 设置模型名称
func WithModel(model string) Option {
	return func(r *ChatCompletionRequest) {
		r.Model = model
	}
}

// WithMessages 设置消息列表
func WithMessages(messages []ChatMessage) Option {
	return func(r *ChatCompletionRequest) {
		r.Messages = append(r.Messages, messages...)
	}
}

// WithStream 设置是否流式输出
func WithStream(stream bool) Option {
	return func(r *ChatCompletionRequest) {
		r.Stream = stream
	}
}

// WithTemperature 设置采样温度
func WithTemperature(temperature float64) Option {
	return func(r *ChatCompletionRequest) {
		r.Temperature = &temperature
	}
}

// WithTopP 设置核采样参数
func WithTopP(topP float64) Option {
	return func(r *ChatCompletionRequest) {
		r.TopP = &topP
	}
}

// WithMaxTokens 设置最大token数
func WithMaxTokens(maxTokens int) Option {
	return func(r *ChatCompletionRequest) {
		r.MaxTokens = &maxTokens
	}
}

// WithDoSample 设置是否启用采样
func WithDoSample(doSample bool) Option {
	return func(r *ChatCompletionRequest) {
		r.DoSample = &doSample
	}
}

// WithTools 设置工具列表
func WithTools(tools []Tool) Option {
	return func(r *ChatCompletionRequest) {
		r.Tools = tools
	}
}

// WithToolChoice 设置工具选择策略
func WithToolChoice(toolChoice *ToolChoice) Option {
	return func(r *ChatCompletionRequest) {
		r.ToolChoice = toolChoice
	}
}

// WithStop 设置停止词
func WithStop(stop []string) Option {
	return func(r *ChatCompletionRequest) {
		r.Stop = stop
	}
}

// WithRequestID 设置请求ID
func WithRequestID(requestID string) Option {
	return func(r *ChatCompletionRequest) {
		r.RequestID = requestID
	}
}

// WithUserID 设置用户ID
func WithUserID(userID string) Option {
	return func(r *ChatCompletionRequest) {
		r.UserID = userID
	}
}

// WithThinking 设置思维链配置
func WithThinking(thinking *ChatThinking) Option {

	return func(r *ChatCompletionRequest) {
		r.Thinking = thinking
	}
}

func WithEnablesearch(enable bool) Option {
	return func(r *ChatCompletionRequest) {
		r.Enablesearch = enable
	}
}

// WithEnableThinking 设置百炼等兼容接口的 enable_thinking（true/false）。
func WithEnableThinking(enable bool) Option {
	return func(r *ChatCompletionRequest) {
		r.EnableThinking = &enable
	}
}

// NewChatCompletionRequest 创建新的对话补全请求
func NewChatCompletionRequest(opts ...Option) *ChatCompletionRequest {
	req := &ChatCompletionRequest{
		// Model:       "",    // 默认空字符串
		// Messages:    nil,   // 默认nil
		// Stream:      false, // 默认不流式
		// Temperature: nil,   // 默认nil
		// TopP:        nil,   // 默认nil
		// MaxTokens:   nil,   // 默认nil
		// DoSample:    nil,   // 默认nil
		// Tools:       nil,   // 默认nil
		// ToolChoice:  nil,   // 默认nil
		// Stop:        nil,   // 默认nil
		// RequestID:   "",    // 默认空字符串
		// UserID:      "",    // 默认空字符串
		// Thinking:    nil,   // 默认nil
	}

	for _, opt := range opts {
		opt(req)
	}

	return req
}

// WithFrequencyPenalty 设置频率惩罚
func WithFrequencyPenalty(penalty float64) Option {
	return func(r *ChatCompletionRequest) {
		r.FrequencyPenalty = &penalty
	}
}

// WithPresencePenalty 设置存在惩罚
func WithPresencePenalty(penalty float64) Option {
	return func(r *ChatCompletionRequest) {
		r.PresencePenalty = &penalty
	}
}

// WithResponseFormat 设置响应格式
func WithResponseFormat(format *ResponseFormat) Option {
	return func(r *ChatCompletionRequest) {
		r.ResponseFormat = format
	}
}

// WithSeed 设置随机种子
func WithSeed(seed int) Option {
	return func(r *ChatCompletionRequest) {
		r.Seed = &seed
	}
}
