package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"leiAgent/internal/provider/openaistyle"
)

const memoComposeSystem = `你是备忘录编辑助手。用户会提供从对话中摘选的多段 Markdown（含「用户」「助手」标签），以及可选的额外要求。
请输出一段可直接保存为备忘录正文的 Markdown，必须满足：
1. 以一级标题开头，单独一行：# 标题（从内容提炼，简短，不要与正文重复另一个一级标题）
2. 正文结构清晰，可适度合并重复、理顺语序，不要捏造事实或添加对话中不存在的信息
3. 只输出最终 Markdown，不要前言、不要代码围栏包裹全文`

// GenerateMemoFromDraft calls the configured LLM to turn a dialog excerpt into a polished memo (Markdown).
func GenerateMemoFromDraft(ctx context.Context, draftMarkdown, userHint string) (string, error) {
	draft := strings.TrimSpace(draftMarkdown)
	if draft == "" {
		return "", fmt.Errorf("草稿为空")
	}
	user := draft
	if h := strings.TrimSpace(userHint); h != "" {
		user = draft + "\n\n### 用户额外要求\n" + h
	}

	client := &http.Client{Timeout: 120 * time.Second}
	p, err := NewClient(client)
	if err != nil {
		return "", err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 118*time.Second)
		defer cancel()
	}

	text, err := p.completeText(ctx, []openaistyle.ChatMessage{
		{Role: openaistyle.RoleSystem, Content: memoComposeSystem},
		{Role: openaistyle.RoleUser, Content: user},
	}, 4096, 0.4)
	if err != nil {
		return "", err
	}
	out := stripOuterFence(strings.TrimSpace(text))
	if out == "" {
		return "", fmt.Errorf("模型返回空内容")
	}
	return out, nil
}

func stripOuterFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	rest := strings.TrimPrefix(s, "```")
	if i := strings.Index(rest, "\n"); i >= 0 {
		rest = rest[i+1:]
	}
	if j := strings.LastIndex(rest, "```"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}
