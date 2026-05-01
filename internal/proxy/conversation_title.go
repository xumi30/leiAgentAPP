package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	gemini "leiAgent/internal/provider/gemini"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/logging"
	"leiAgent/utils"
)

const chatTitleSystemPrompt = `你是对话标题生成器。根据用户的第一条消息，输出一个简短的对话标题。
要求：只输出标题正文，不要引号、不要序号、不要“标题：”“题目：”等前缀；优先使用中文；总长度不超过15个字（含标点）。`

func normalizeChatTitle(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	for _, p := range []string{"标题：", "标题:", "题目：", "题目:", "Title:", "title:"} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimSpace(strings.TrimPrefix(s, p))
		}
	}
	s = strings.Trim(s, `"'「」『』`)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	s = strings.TrimSpace(b.String())
	return utils.TruncateRunes(s, utils.ChatTitleMaxRunes)
}

func messageTextFromCompletion(resp *openaistyle.ChatCompletionResponse) (string, error) {
	if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		return "", fmt.Errorf("no completion message")
	}
	msg := resp.Choices[0].Message
	switch c := msg.Content.(type) {
	case string:
		return c, nil
	default:
		if msg.Content != nil {
			return fmt.Sprint(msg.Content), nil
		}
		return "", nil
	}
}

func parseCompletionBody(body []byte, info *ModelAPIInfo) (*openaistyle.ChatCompletionResponse, error) {
	if info.provider == "gemini" {
		geminiResponse := &gemini.ChatCompletionResponse{}
		if err := json.Unmarshal(body, geminiResponse); err != nil {
			return nil, err
		}
		return gemini.ConvertToOpenAIResponse(geminiResponse), nil
	}
	openaiResp := &openaistyle.ChatCompletionResponse{}
	if err := json.Unmarshal(body, openaiResp); err != nil {
		return nil, err
	}
	return openaiResp, nil
}

func buildTitleRequestJSON(info *ModelAPIInfo, userText string) ([]byte, error) {
	maxTok := 64
	temp := 0.35
	opts := []openaistyle.Option{
		openaistyle.WithModel(info.modelName),
		openaistyle.WithMessages([]openaistyle.ChatMessage{
			{Role: openaistyle.RoleSystem, Content: chatTitleSystemPrompt},
			{Role: openaistyle.RoleUser, Content: userText},
		}),
		openaistyle.WithMaxTokens(maxTok),
		openaistyle.WithStream(false),
		openaistyle.WithTemperature(temp),
		openaistyle.WithEnableThinking(false),
		openaistyle.WithThinking(&openaistyle.ChatThinking{Type: openaistyle.ThinkingDisabled}),
	}
	req := openaistyle.NewChatCompletionRequest(opts...)
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if info.provider == "gemini" {
		greq := gemini.ConvertFromOpenAIRequest(req)
		return json.Marshal(greq)
	}
	return jsonData, nil
}

func fallbackChatTitle(firstUserMessage string) string {
	s := strings.TrimSpace(firstUserMessage)
	if s == "" {
		return "新对话"
	}
	t := utils.TruncateRunes(s, utils.ChatTitleMaxRunes)
	if t == "" {
		return "新对话"
	}
	return t
}

// GenerateConversationTitle 根据用户首条消息调用已配置的 LLM 生成不超过 15 个字的标题；失败时返回截断后的兜底标题。
func GenerateConversationTitle(ctx context.Context, firstUserMessage string) string {
	msg := strings.TrimSpace(firstUserMessage)
	if msg == "" {
		return "新对话"
	}

	userSnippet := msg
	if r := []rune(msg); len(r) > 800 {
		userSnippet = string(r[:800])
	}

	client := &http.Client{Timeout: 40 * time.Second}
	p, err := NewProxy(client)
	if err != nil {
		logging.Warn("生成对话标题：无法创建 LLM 代理，使用兜底标题: %v", err)
		return fallbackChatTitle(msg)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 38*time.Second)
		defer cancel()
	}

	var lastErr error
	for i, info := range p.backends {
		label := info.logLabel()
		if label == "" {
			label = fmt.Sprintf("#%d", i)
		}
		jsonData, err := buildTitleRequestJSON(info, userSnippet)
		if err != nil {
			lastErr = err
			logging.Warn("生成对话标题：后端 %s 构造请求失败: %v", label, err)
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, info.url, bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := p.doRequest(req, info)
		if err != nil {
			lastErr = err
			logging.Warn("生成对话标题：后端 %s 请求失败: %v", label, err)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		openaiResp, err := parseCompletionBody(body, info)
		if err != nil {
			lastErr = err
			logging.Warn("生成对话标题：后端 %s 解析失败: %v", label, err)
			continue
		}
		text, err := messageTextFromCompletion(openaiResp)
		if err != nil {
			lastErr = err
			continue
		}
		title := normalizeChatTitle(text)
		if title != "" {
			logging.Info("生成对话标题成功（后端 %s）: %q", label, title)
			return title
		}
		lastErr = fmt.Errorf("empty title from model")
	}

	if lastErr != nil {
		logging.Warn("生成对话标题：全部后端失败，使用兜底: %v", lastErr)
	}
	return fallbackChatTitle(msg)
}
