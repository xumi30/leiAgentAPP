package gemini

import "leiAgent/internal/provider/openaistyle"

// ConvertFromOpenAIRequest 从OpenAI格式转换为Gemini格式
func ConvertFromOpenAIRequest(openaiReq *openaistyle.ChatCompletionRequest) *ChatCompletionRequest {
	geminiReq := NewChatCompletionRequest()

	// 转换消息内容
	geminiReq.Contents = convertMessages(openaiReq.Messages)

	// 转换生成配置
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

	// 转换停止词
	if len(openaiReq.Stop) > 0 {
		geminiReq.GenerationConfig.StopSequences = openaiReq.Stop
	}

	// 转换响应格式
	if openaiReq.ResponseFormat != nil {
		geminiReq.GenerationConfig.ResponseMimeType = convertMimeType(openaiReq.ResponseFormat.Type)
		// 如果需要 JSON 输出，设置基本的 schema
		if openaiReq.ResponseFormat.Type == "json_object" || openaiReq.ResponseFormat.Type == "json_schema" {
			geminiReq.GenerationConfig.ResponseSchema = &ResponseSchema{
				Type: "object",
			}
		}
	}

	// 转换工具
	if len(openaiReq.Tools) > 0 {
		geminiReq.Tools = convertTools(openaiReq.Tools)
	}

	// 转换工具选择
	if openaiReq.ToolChoice != nil {
		geminiReq.ToolConfig = &ToolConfig{
			FunctionCallingConfig: &FunctionCallingConfig{
				Mode: convertToolChoice(openaiReq.ToolChoice),
			},
		}
	}

	return geminiReq
}

// convertMessages 转换消息// convertMessages 转换消息
func convertMessages(messages []openaistyle.ChatMessage) []Content {
	var contents []Content
	for _, msg := range messages {
		content := Content{
			Role: convertRole(msg.Role),
		}

		// 处理工具调用
		if len(msg.ToolCalls) > 0 {
			for _, toolCall := range msg.ToolCalls {
				if toolCall.Function != nil {
					// 解析参数
					// var args map[string]interface{}
					// if toolCall.Function.Parameters != nil{
					// 	json.Unmarshal([]byte(toolCall.Function.Parameters), &args)
					// }

					content.Parts = append(content.Parts, Part{
						FunctionCall: &FunctionCall{
							Name: toolCall.Function.Name,
							ID:   toolCall.ID,
							Args: toolCall.Function.Parameters,
						},
					})
				}
			}
		} else if msg.Content != nil {
			// 处理文本内容
			if str, ok := msg.Content.(string); ok {
				content.Parts = append(content.Parts, Part{
					Text: str,
				})
			}
		}

		// 只添加有内容的消息
		if len(content.Parts) > 0 {
			contents = append(contents, content)
		}
	}
	return contents
}

// convertRole 转换角色
func convertRole(role string) string {
	switch role {
	case "system":
		return "user" // 或放 systemInstruction
	case "assistant":
		return "model"
	case "tool":
		return "function"
	default:
		return role
	}
}

// convertMimeType 转换MIME类型
func convertMimeType(formatType string) string {
	switch formatType {
	case "json_object", "json_schema":
		return "application/json"
	default:
		return "text/plain"
	}
}

// convertTools 转换工具
func convertTools(tools []openaistyle.Tool) []Tool {
	var geminiTools []Tool
	for _, tool := range tools {
		if tool.Function != nil {
			geminiTools = append(geminiTools, Tool{
				FunctionDeclarations: []FunctionDeclaration{
					{
						Name:        tool.Function.Name,
						Description: tool.Function.Description,
						Parameters:  tool.Function.Parameters,
					},
				},
			})
		}
	}
	return geminiTools
}

// convertToolChoice 转换工具选择
func convertToolChoice(choice *openaistyle.ToolChoice) string {
	if choice == nil {
		return "auto"
	}
	switch choice.Type {
	case "none":
		return "none"
	case "required":
		return "any"
	default:
		return "auto"
	}
}

// GminiToOpenAIResponse 将Gemini响应转换为OpenAI响应
