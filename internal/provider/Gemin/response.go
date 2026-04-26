package gemini

import (
	"encoding/json"
	"leiAgent/internal/provider/openaistyle"
	"strings"

	"time"
)

// ChatCompletionResponse Gemini 聊天响应
type ChatCompletionResponse struct {
	Candidates     []Candidate     `json:"candidates"`
	UsageMetadata  *UsageMetadata  `json:"usageMetadata,omitempty"`
	ModelVersion   string          `json:"modelVersion,omitempty"`
	ResponseID     string          `json:"responseId,omitempty"`
	PromptFeedback *PromptFeedback `json:"promptFeedback,omitempty"`
}

// Candidate 候选回复
type Candidate struct {
	Content          Content           `json:"content"` // 修正字段名
	FinishReason     string            `json:"finishReason"`
	Index            int               `json:"index"`
	SafetyRatings    []SafetyRating    `json:"safetyRatings,omitempty"`
	FinishMessage    string            `json:"finishMessage,omitempty"`
	CitationMetadata *CitationMetadata `json:"citationMetadata,omitempty"`
}

// Content 内容
type Content struct {
	Role  string `json:"role,omitempty"`
	Parts []Part `json:"parts"` // 修正字段名
}

// Part 内容部分
type Part struct {
	Text                string               `json:"text,omitempty"`
	FunctionCall        *FunctionCall        `json:"functionCall,omitempty"`
	FunctionResponse    *FunctionResponse    `json:"functionResponse,omitempty"`
	ExecutableCode      *ExecutableCode      `json:"executableCode,omitempty"`
	CodeExecutionResult *CodeExecutionResult `json:"codeExecutionResult,omitempty"`
	FileData            *FileData            `json:"fileData,omitempty"`
	ThoughtSignature    string               `json:"thoughtSignature,omitempty"`
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name string                 `json:"name"`
	ID   string                 `json:"id,omitempty"`
	Args map[string]interface{} `json:"args"`
}

// FunctionResponse 函数响应
type FunctionResponse struct {
	Name     string                 `json:"name,omitempty"`
	Response map[string]interface{} `json:"response"`
}

// ExecutableCode 可执行代码
type ExecutableCode struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

// CodeExecutionResult 代码执行结果
type CodeExecutionResult struct {
	Outcome string `json:"outcome"`
	Output  string `json:"output,omitempty"`
}

// FileData 文件数据
type FileData struct {
	MimeType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
}

// UsageMetadata 使用统计
type UsageMetadata struct {
	PromptTokenCount        int            `json:"promptTokenCount"`
	CandidatesTokenCount    int            `json:"candidatesTokenCount"`
	TotalTokenCount         int            `json:"totalTokenCount"`
	PromptTokensDetails     []TokenDetails `json:"promptTokensDetails,omitempty"`
	CandidatesTokensDetails []TokenDetails `json:"candidatesTokensDetails,omitempty"`
	ThoughtsTokenCount      int            `json:"thoughtsTokenCount,omitempty"`
}

// TokenDetails 令牌详情
type TokenDetails struct {
	Modality   string `json:"modality"`
	TokenCount int    `json:"tokenCount"`
}

// SafetyRating 安全评级
type SafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
	Severity    string `json:"severity,omitempty"`
}

// PromptFeedback 提示反馈
type PromptFeedback struct {
	BlockReason   string         `json:"blockReason,omitempty"`
	SafetyRatings []SafetyRating `json:"safetyRatings,omitempty"`
}

// CitationMetadata 引用元数据
type CitationMetadata struct {
	CitationSources []CitationSource `json:"citationSources,omitempty"`
}

// CitationSource 引用来源
type CitationSource struct {
	URI        string `json:"uri,omitempty"`
	License    string `json:"license,omitempty"`
	StartIndex int    `json:"startIndex,omitempty"`
	EndIndex   int    `json:"endIndex,omitempty"`
}

// GroundingAttribution 归因信息
type GroundingAttribution struct {
	Sources []GroundingSource `json:"sources,omitempty"`
}

// GroundingSource 归因来源
type GroundingSource struct {
	URI        string `json:"uri,omitempty"`
	License    string `json:"license,omitempty"`
	StartIndex int    `json:"startIndex,omitempty"`
	EndIndex   int    `json:"endIndex,omitempty"`
}

// ConvertToOpenAIResponse 将 Gemini 响应转换为 OpenAI 风格响应
// ConvertToOpenAIResponse 将 Gemini 响应转换为 OpenAI 风格响应
func ConvertToOpenAIResponse(geminiResp *ChatCompletionResponse) *openaistyle.ChatCompletionResponse {
	response := &openaistyle.ChatCompletionResponse{
		ID:      geminiResp.ResponseID,
		Created: time.Now().Unix(),
		Model:   geminiResp.ModelVersion,
	}

	// 转换 choices
	if len(geminiResp.Candidates) > 0 {
		response.Choices = make([]openaistyle.ChatCompletionChoice, len(geminiResp.Candidates))
		for i, candidate := range geminiResp.Candidates {
			choice := openaistyle.ChatCompletionChoice{
				Index:        i,
				FinishReason: candidate.FinishReason,
			}

			// 始终创建 Message 对象，避免空指针
			choice.Message = &openaistyle.ChatCompletionMessage{
				Role: candidate.Content.Role,
			}

			// 如果 Role 为空，设置默认值
			if choice.Message.Role == "" {
				choice.Message.Role = "assistant"
			}

			// 处理 parts
			var textParts []string
			for _, part := range candidate.Content.Parts {
				// 处理文本内容
				if part.Text != "" {
					textParts = append(textParts, part.Text)
				}

				// 处理工具调用
				if part.FunctionCall != nil {
					if choice.Message.ToolCalls == nil {
						choice.Message.ToolCalls = make([]openaistyle.ChatCompletionToolCall, 0)
					}

					// 将参数转换为 JSON 字符串
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)

					toolCall := openaistyle.ChatCompletionToolCall{
						ID:   part.FunctionCall.ID,
						Type: "function",
						Function: &openaistyle.FunctionCall{
							Name:      part.FunctionCall.Name,
							Arguments: string(argsJSON),
						},
					}
					choice.Message.ToolCalls = append(choice.Message.ToolCalls, toolCall)
				}
			}
			if len(textParts) > 0 {
				choice.Message.Content = strings.Join(textParts, "")
			}

			response.Choices[i] = choice
		}
	}

	// 转换 usage
	if geminiResp.UsageMetadata != nil {
		response.Usage = &openaistyle.TokenUsage{
			PromptTokens:     geminiResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
		}
	}

	return response
}
