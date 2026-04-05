package proxy

import "sync/atomic"

var llmThinkingDisabled atomic.Bool

// SetLLMThinkingDisabled 为 true 时，发往 LLM 的补全请求会关闭思考/推理链
//（对应阿里云百炼 OpenAI 兼容接口的 enable_thinking 等，见官方兼容模式文档）。
func SetLLMThinkingDisabled(disabled bool) {
	llmThinkingDisabled.Store(disabled)
}

// IsLLMThinkingDisabled 表示当前是否对 LLM 关闭了思考过程。
func IsLLMThinkingDisabled() bool {
	return llmThinkingDisabled.Load()
}
