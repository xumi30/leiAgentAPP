package proxy

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode"

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
	p, err := NewClient(client)
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

	text, err := p.completeText(ctx, []openaistyle.ChatMessage{
		{Role: openaistyle.RoleSystem, Content: chatTitleSystemPrompt},
		{Role: openaistyle.RoleUser, Content: userSnippet},
	}, 64, 0.35)
	if err != nil {
		logging.Warn("生成对话标题失败，使用兜底: %v", err)
		return fallbackChatTitle(msg)
	}
	title := normalizeChatTitle(text)
	if title != "" {
		return title
	}
	return fallbackChatTitle(msg)
}
