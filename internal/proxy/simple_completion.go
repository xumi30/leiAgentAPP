package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"leiAgent/internal/provider/openaistyle"
)

func (p *Client) completeText(
	ctx context.Context,
	messages []openaistyle.ChatMessage,
	maxTokens int,
	temperature float64,
) (string, error) {
	requestBody, err := json.Marshal(openaistyle.NewChatCompletionRequest(
		openaistyle.WithModel(p.config.modelName),
		openaistyle.WithMessages(messages),
		openaistyle.WithMaxTokens(maxTokens),
		openaistyle.WithStream(false),
		openaistyle.WithTemperature(temperature),
	))
	if err != nil {
		return "", err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.url, bytes.NewReader(requestBody))
	if err != nil {
		return "", err
	}
	response, err := p.doRequest(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	var completion openaistyle.ChatCompletionResponse
	if err := json.Unmarshal(body, &completion); err != nil {
		return "", fmt.Errorf("解析 LLM 响应失败: %w", err)
	}
	return messageTextFromCompletion(&completion)
}

func messageTextFromCompletion(response *openaistyle.ChatCompletionResponse) (string, error) {
	if len(response.Choices) == 0 || response.Choices[0].Message == nil {
		return "", fmt.Errorf("LLM 响应缺少 completion message")
	}
	content := response.Choices[0].Message.Content
	if text, ok := content.(string); ok {
		return text, nil
	}
	if content == nil {
		return "", nil
	}
	return fmt.Sprint(content), nil
}
