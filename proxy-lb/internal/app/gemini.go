package app

import (
	"encoding/json"
	"time"
)

type GeminiChatCompletionRequest struct {
	Contents         []GeminiContent         `json:"contents"`
	GenerationConfig *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	SafetySettings   []GeminiSafetySetting   `json:"safetySettings,omitempty"`
	Tools            []GeminiTool            `json:"tools,omitempty"`
	ToolConfig       *GeminiToolConfig       `json:"toolConfig,omitempty"`
	CacheControl     *GeminiCacheControl     `json:"cacheControl,omitempty"`
}

type GeminiGenerationConfig struct {
	Temperature      float64               `json:"temperature,omitempty"`
	TopP             float64               `json:"topP,omitempty"`
	TopK             int                   `json:"topK,omitempty"`
	MaxOutputTokens  int                   `json:"maxOutputTokens,omitempty"`
	StopSequences    []string              `json:"stopSequences,omitempty"`
	ResponseMimeType string                `json:"responseMimeType,omitempty"`
	ResponseSchema   *GeminiResponseSchema `json:"responseSchema,omitempty"`
	CandidateCount   int                   `json:"candidateCount,omitempty"`
	Seed             int                   `json:"seed,omitempty"`
}

type GeminiResponseSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

type GeminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type GeminiTool struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type GeminiFunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type GeminiToolConfig struct {
	FunctionCallingConfig *GeminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type GeminiFunctionCallingConfig struct {
	Mode             string   `json:"mode"`
	AllowedFunctions []string `json:"allowedFunctions,omitempty"`
}

type GeminiCacheControl struct {
	TTL string `json:"ttl"`
}

type GeminiChatCompletionResponse struct {
	Candidates     []GeminiCandidate     `json:"candidates"`
	UsageMetadata  *GeminiUsageMetadata  `json:"usageMetadata,omitempty"`
	ModelVersion   string                `json:"modelVersion,omitempty"`
	ResponseID     string                `json:"responseId,omitempty"`
	PromptFeedback *GeminiPromptFeedback `json:"promptFeedback,omitempty"`
}

type GeminiCandidate struct {
	Content          GeminiContent           `json:"content"`
	FinishReason     string                  `json:"finishReason"`
	Index            int                     `json:"index"`
	SafetyRatings    []GeminiSafetyRating    `json:"safetyRatings,omitempty"`
	FinishMessage    string                  `json:"finishMessage,omitempty"`
	CitationMetadata *GeminiCitationMetadata `json:"citationMetadata,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text                string                     `json:"text,omitempty"`
	FunctionCall        *GeminiFunctionCall        `json:"functionCall,omitempty"`
	FunctionResponse    *GeminiFunctionResponse    `json:"functionResponse,omitempty"`
	ExecutableCode      *GeminiExecutableCode      `json:"executableCode,omitempty"`
	CodeExecutionResult *GeminiCodeExecutionResult `json:"codeExecutionResult,omitempty"`
	FileData            *GeminiFileData            `json:"fileData,omitempty"`
	ThoughtSignature    string                     `json:"thoughtSignature,omitempty"`
}

type GeminiFunctionCall struct {
	Name string                 `json:"name"`
	ID   string                 `json:"id,omitempty"`
	Args map[string]interface{} `json:"args"`
}

type GeminiFunctionResponse struct {
	Name     string                 `json:"name,omitempty"`
	Response map[string]interface{} `json:"response"`
}

type GeminiExecutableCode struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

type GeminiCodeExecutionResult struct {
	Outcome string `json:"outcome"`
	Output  string `json:"output,omitempty"`
}

type GeminiFileData struct {
	MimeType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
}

type GeminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type GeminiSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
	Severity    string `json:"severity,omitempty"`
}

type GeminiPromptFeedback struct {
	BlockReason   string               `json:"blockReason,omitempty"`
	SafetyRatings []GeminiSafetyRating `json:"safetyRatings,omitempty"`
}

type GeminiCitationMetadata struct {
	CitationSources []GeminiCitationSource `json:"citationSources,omitempty"`
}

type GeminiCitationSource struct {
	URI        string `json:"uri,omitempty"`
	License    string `json:"license,omitempty"`
	StartIndex int    `json:"startIndex,omitempty"`
	EndIndex   int    `json:"endIndex,omitempty"`
}

func NewGeminiChatCompletionRequest() *GeminiChatCompletionRequest {
	return &GeminiChatCompletionRequest{
		GenerationConfig: &GeminiGenerationConfig{},
	}
}

func ConvertFromOpenAIRequest(openaiReq *ChatCompletionRequest) *GeminiChatCompletionRequest {
	geminiReq := NewGeminiChatCompletionRequest()
	geminiReq.Contents = convertGeminiMessages(openaiReq.Messages)

	if openaiReq.Temperature != nil {
		geminiReq.GenerationConfig.Temperature = *openaiReq.Temperature
	}
	if openaiReq.TopP != nil {
		geminiReq.GenerationConfig.TopP = *openaiReq.TopP
	}
	if openaiReq.MaxTokens != nil {
		geminiReq.GenerationConfig.MaxOutputTokens = *openaiReq.MaxTokens
	}
	if openaiReq.Seed != nil {
		geminiReq.GenerationConfig.Seed = *openaiReq.Seed
	}
	if len(openaiReq.Stop) > 0 {
		geminiReq.GenerationConfig.StopSequences = openaiReq.Stop
	}
	if openaiReq.ResponseFormat != nil {
		geminiReq.GenerationConfig.ResponseMimeType = convertGeminiMimeType(openaiReq.ResponseFormat.Type)
		if openaiReq.ResponseFormat.Type == "json_object" || openaiReq.ResponseFormat.Type == "json_schema" {
			geminiReq.GenerationConfig.ResponseSchema = &GeminiResponseSchema{Type: "object"}
		}
	}
	if len(openaiReq.Tools) > 0 {
		geminiReq.Tools = convertGeminiTools(openaiReq.Tools)
	}
	if openaiReq.ToolChoice != nil {
		geminiReq.ToolConfig = &GeminiToolConfig{
			FunctionCallingConfig: &GeminiFunctionCallingConfig{
				Mode: convertGeminiToolChoice(openaiReq.ToolChoice),
			},
		}
	}
	return geminiReq
}

func convertGeminiMessages(messages []ChatMessage) []GeminiContent {
	var contents []GeminiContent
	for _, msg := range messages {
		content := GeminiContent{Role: convertGeminiRole(msg.Role)}
		if len(msg.ToolCalls) > 0 {
			for _, toolCall := range msg.ToolCalls {
				if toolCall.Function == nil {
					continue
				}
				content.Parts = append(content.Parts, GeminiPart{
					FunctionCall: &GeminiFunctionCall{
						Name: toolCall.Function.Name,
						ID:   toolCall.ID,
						Args: toolCall.Function.Parameters,
					},
				})
			}
		} else if msg.Content != nil {
			switch v := msg.Content.(type) {
			case string:
				if v != "" {
					content.Parts = append(content.Parts, GeminiPart{Text: v})
				}
			case []interface{}:
				// OpenAI-compatible multimodal "content": [{"type":"text","text":"..."}...]
				var textBuf string
				for _, item := range v {
					m, ok := item.(map[string]interface{})
					if !ok {
						continue
					}
					typ, _ := m["type"].(string)
					switch typ {
					case "text":
						txt, _ := m["text"].(string)
						if txt == "" {
							continue
						}
						if textBuf != "" {
							textBuf += "\n"
						}
						textBuf += txt
					case "image_url", "video_url", "file_url":
						// Gemini generateContent doesn't accept arbitrary external URLs as "parts" directly
						// in this proxy's current mapping, so we preserve them as readable text markers.
						var url string
						key := typ
						if anyURL, ok := m[key].(map[string]interface{}); ok {
							if s, ok := anyURL["url"].(string); ok {
								url = s
							}
						}
						marker := "[unsupported " + typ + "]"
						if url != "" {
							marker += " " + url
						}
						if textBuf != "" {
							textBuf += "\n"
						}
						textBuf += marker
					case "input_audio":
						format, _ := m["format"].(string)
						marker := "[unsupported input_audio]"
						if format != "" {
							marker += " format=" + format
						}
						if textBuf != "" {
							textBuf += "\n"
						}
						textBuf += marker
					default:
						// Unknown part type: keep a marker so content isn't silently lost.
						if typ == "" {
							typ = "unknown"
						}
						if textBuf != "" {
							textBuf += "\n"
						}
						textBuf += "[unsupported part type: " + typ + "]"
					}
				}
				if textBuf != "" {
					content.Parts = append(content.Parts, GeminiPart{Text: textBuf})
				}
			}
		}
		if len(content.Parts) > 0 {
			contents = append(contents, content)
		}
	}
	return contents
}

func convertGeminiRole(role string) string {
	switch role {
	case "system":
		return "user"
	case "assistant":
		return "model"
	case "tool":
		return "function"
	default:
		return role
	}
}

func convertGeminiMimeType(formatType string) string {
	switch formatType {
	case "json_object", "json_schema":
		return "application/json"
	default:
		return "text/plain"
	}
}

func convertGeminiTools(tools []Tool) []GeminiTool {
	var geminiTools []GeminiTool
	for _, tool := range tools {
		if tool.Function == nil {
			continue
		}
		geminiTools = append(geminiTools, GeminiTool{
			FunctionDeclarations: []GeminiFunctionDeclaration{{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			}},
		})
	}
	return geminiTools
}

func convertGeminiToolChoice(choice *ToolChoice) string {
	if choice == nil {
		return "auto"
	}
	switch choice.Type {
	case ToolChoiceNone:
		return "none"
	case ToolChoiceRequired:
		return "any"
	default:
		return "auto"
	}
}

func ConvertToOpenAIResponse(geminiResp *GeminiChatCompletionResponse) *ChatCompletionResponse {
	response := &ChatCompletionResponse{
		ID:      geminiResp.ResponseID,
		Created: time.Now().Unix(),
		Model:   geminiResp.ModelVersion,
	}
	if len(geminiResp.Candidates) > 0 {
		response.Choices = make([]ChatCompletionChoice, len(geminiResp.Candidates))
		for i, candidate := range geminiResp.Candidates {
			choice := ChatCompletionChoice{
				Index:        i,
				FinishReason: candidate.FinishReason,
				Message:      &ChatCompletionMessage{Role: candidate.Content.Role},
			}
			if choice.Message.Role == "" {
				choice.Message.Role = "assistant"
			}
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					choice.Message.Content = part.Text
				}
				if part.FunctionCall != nil {
					if choice.Message.ToolCalls == nil {
						choice.Message.ToolCalls = make([]ChatCompletionToolCall, 0)
					}
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)
					choice.Message.ToolCalls = append(choice.Message.ToolCalls, ChatCompletionToolCall{
						ID:   part.FunctionCall.ID,
						Type: "function",
						Function: &FunctionCall{
							Name:      part.FunctionCall.Name,
							Arguments: string(argsJSON),
						},
					})
				}
			}
			response.Choices[i] = choice
		}
	}
	if geminiResp.UsageMetadata != nil {
		response.Usage = &TokenUsage{
			PromptTokens:     geminiResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
		}
	}
	return response
}
