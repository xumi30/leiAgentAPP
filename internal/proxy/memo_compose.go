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

	gemini "leiAgent/internal/provider/gemini"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/logging"
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
	p, err := NewProxy(client)
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

	maxTok := 4096
	temp := 0.4
	opts := []openaistyle.Option{
		openaistyle.WithModel(""),
		openaistyle.WithMessages([]openaistyle.ChatMessage{
			{Role: openaistyle.RoleSystem, Content: memoComposeSystem},
			{Role: openaistyle.RoleUser, Content: user},
		}),
		openaistyle.WithMaxTokens(maxTok),
		openaistyle.WithStream(false),
		openaistyle.WithTemperature(temp),
		openaistyle.WithEnableThinking(false),
		openaistyle.WithThinking(&openaistyle.ChatThinking{Type: openaistyle.ThinkingDisabled}),
	}

	var lastErr error
	for i, info := range p.backends {
		label := info.logLabel()
		if label == "" {
			label = fmt.Sprintf("#%d", i)
		}
		opts[0] = openaistyle.WithModel(info.modelName)
		req := openaistyle.NewChatCompletionRequest(opts...)
		jsonData, err := jsonMarshalRequest(req, info)
		if err != nil {
			lastErr = err
			logging.Warn("备忘录 LLM：后端 %s 构造请求失败: %v", label, err)
			continue
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, info.url, bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := p.doRequest(httpReq, info)
		if err != nil {
			lastErr = err
			logging.Warn("备忘录 LLM：后端 %s 请求失败: %v", label, err)
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
			continue
		}
		text, err := messageTextFromCompletion(openaiResp)
		if err != nil {
			lastErr = err
			continue
		}
		out := strings.TrimSpace(text)
		out = stripOuterFence(out)
		if out == "" {
			lastErr = fmt.Errorf("模型返回空内容")
			continue
		}
		logging.Info("备忘录 LLM 生成成功（后端 %s）", label)
		return out, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("未配置可用 LLM 后端")
}

func jsonMarshalRequest(req *openaistyle.ChatCompletionRequest, info *ModelAPIInfo) ([]byte, error) {
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
