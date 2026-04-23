package proxy

import "sync/atomic"

var llmThinkingDisabled atomic.Bool

// SetLLMThinkingDisabled 为 true 时，不再向 LLM 请求发送 thinking 类扩展字段。
func SetLLMThinkingDisabled(disabled bool) {
	llmThinkingDisabled.Store(disabled)
}

// IsLLMThinkingDisabled 表示当前是否对 LLM 关闭了思考过程。
func IsLLMThinkingDisabled() bool {
	return llmThinkingDisabled.Load()
}
