package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"leiAgent/internal/globalchannel"
	gemini "leiAgent/internal/provider/Gemin"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/logging"
	"leiAgent/utils"
	"net/http"
	"strings"
	"time"
)

func effectiveUsageTotal(u *openaistyle.TokenUsage) int {
	if u == nil {
		return 0
	}
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	if u.PromptTokens > 0 || u.CompletionTokens > 0 {
		return u.PromptTokens + u.CompletionTokens
	}
	return 0
}

func intFromJSONAny(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	default:
		return 0
	}
}

func extractUsageLoose(raw []byte) *openaistyle.TokenUsage {
	var top map[string]interface{}
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil
	}
	v, ok := top["usage"]
	if !ok || v == nil {
		return nil
	}
	um, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	pt := intFromJSONAny(um["prompt_tokens"])
	ct := intFromJSONAny(um["completion_tokens"])
	tt := intFromJSONAny(um["total_tokens"])
	if tt == 0 {
		tt = intFromJSONAny(um["total"])
	}
	if tt == 0 {
		it := intFromJSONAny(um["input_tokens"])
		ot := intFromJSONAny(um["output_tokens"])
		if it > 0 || ot > 0 {
			tt = it + ot
		}
	}
	if tt == 0 && (pt > 0 || ct > 0) {
		tt = pt + ct
	}
	if tt <= 0 && pt <= 0 && ct <= 0 {
		return nil
	}
	return &openaistyle.TokenUsage{
		PromptTokens:     pt,
		CompletionTokens: ct,
		TotalTokens:      tt,
	}
}

func (p *Proxy) handleResponse(ctx context.Context, resp *http.Response, isStream bool, info *ModelAPIInfo) (*ToolAndContent, error) {

	if isStream {
		return p.handleStreamResponse(ctx, resp)
	}
	return p.handleNonStreamResponse(ctx, resp, info)
}

// effectiveDialogChatID 用于 UI 管道：可与 memory 用的 ChatIDString（临时子会话）不同。

func (p *Proxy) handleStreamResponse(ctx context.Context, resp *http.Response) (*ToolAndContent, error) {
	logging.Info("开始处理流式响应: %v", resp)
	var fullContent strings.Builder
	var fullReasoningContent strings.Builder
	stripNeedActionHeader := false
	if v, ok := ctx.Value(utils.NeedActionHeaderString).(bool); ok && v {
		stripNeedActionHeader = true
	}
	var needAction bool
	var needActionHeaderParsed bool
	var needActionHeaderBuffer strings.Builder

	scanner := NewStreamScanner(resp.Body)
	tls := []openaistyle.ChatCompletionToolCall{}
	var lastFinishReason string
	var lastUsage *openaistyle.TokenUsage

	d_mesageid := utils.GenerateMessageID()
	r_message := utils.GenerateMessageID()
	memChatID, _ := ctx.Value(utils.ChatIDString).(string)
	for scanner.Scan() {
		raw := scanner.Bytes()
		var response openaistyle.ChatCompletionResponse

		if err := json.Unmarshal(raw, &response); err != nil {
			logging.Error("解析流式JSON失败: %v", err)
			continue
		}

		if response.Usage != nil && effectiveUsageTotal(response.Usage) > effectiveUsageTotal(lastUsage) {
			lastUsage = response.Usage
		}
		if loose := extractUsageLoose(raw); loose != nil && effectiveUsageTotal(loose) > effectiveUsageTotal(lastUsage) {
			lastUsage = loose
		}

		if len(response.Choices) == 0 {
			continue
		}

		ch0 := response.Choices[0]
		if fr := strings.TrimSpace(ch0.FinishReason); fr != "" {
			lastFinishReason = fr
		}
		if ch0.Delta == nil {
			continue
		}
		delta := ch0.Delta

		// 处理工具调用

		tools := delta.ToolCalls
		if len(tools) > 0 {
			for _, tool := range tools {
				index := tool.Index
				if index >= len(tls) {
					t := openaistyle.ChatCompletionToolCall{
						Index: index,
						Function: &openaistyle.FunctionCall{
							Arguments: "",
							Name:      "",
						},
						Type: "",
					}
					tls = append(tls, t)
				}
				if tool.ID != "" {
					tls[index].ID = tool.ID
				}
				if tool.Function.Arguments != "" {
					tls[index].Function.Arguments += tool.Function.Arguments
				}
				if tool.Function.Name != "" {
					tls[index].Function.Name = tool.Function.Name
				}
				if tool.Type != "" {
					tls[index].Type = tool.Type
				}

			}

		}

		// 处理普通内容
		content, ok := delta.Content.(string)
		if ok && content != "" {
			outgoingContent := content
			if stripNeedActionHeader && !needActionHeaderParsed {
				needActionHeaderBuffer.WriteString(content)
				parsed, parsedNeedAction, rest, flushRaw := consumeNeedActionHeader(needActionHeaderBuffer.String())
				if !parsed && !flushRaw {
					continue
				}
				needActionHeaderParsed = true
				needAction = parsedNeedAction
				if flushRaw {
					outgoingContent = needActionHeaderBuffer.String()
				} else {
					outgoingContent = rest
				}
			}
			if outgoingContent != "" {
				fullContent.WriteString(outgoingContent)
				//logging.Info("为chatid %s 返回的内容:%s", ctx.Value(utils.ChatIDString).(string), outgoingContent)
				globalchannel.SendAssitantMessageStream(ctx, outgoingContent, d_mesageid, false, 0)
			}

		}

		// 处理推理内容
		reasoningContent := delta.ReasoningContent
		if reasoningContent != "" {
			fullReasoningContent.WriteString(reasoningContent)
			globalchannel.SendAReasonningMessageStream(ctx, reasoningContent, r_message, false)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("扫描流式响应失败: %w", err)
	}

	if stripNeedActionHeader && !needActionHeaderParsed && needActionHeaderBuffer.Len() > 0 {
		bufferedContent := needActionHeaderBuffer.String()
		fullContent.WriteString(bufferedContent)
		globalchannel.SendAssitantMessageStream(ctx, bufferedContent, d_mesageid, false, 0)
	}

	result := fullContent.String()
	reasoningResult := fullReasoningContent.String()

	// 关键：发送一次“流结束”信号（IsFinished=true），让 AppenAgentMessageToFrontRole 做收口入库并 emit dialogStreamEnd。
	// 否则前端虽然能看到流式拼接的内容，但刷新后因 DB 未落库而消失。
	globalchannel.SendAssitantMessageStream(ctx, "", d_mesageid, true, effectiveUsageTotal(lastUsage))
	globalchannel.SendAReasonningMessageStream(ctx, "", r_message, true)
	//fmt.Println("最终生成内容: ", result)
	//fmt.Println("最终推理内容: ", reasoningResult)

	logging.Info("响应内容: %s", result)
	tlsJSON, err := json.Marshal(tls)
	if err != nil {
		logging.Error("序列化工具列表失败: %v", err)
	} else {
		logging.Info("返回调用tools: %s", string(tlsJSON))
	}

	if lastUsage != nil {
		logging.Info("流式响应结束 finish_reason=%q completion_tokens=%d prompt_tokens=%d total=%d 正文长度=%d 字符",
			lastFinishReason,
			lastUsage.CompletionTokens,
			lastUsage.PromptTokens,
			lastUsage.TotalTokens,
			len(result))
	} else {
		logging.Info("流式响应结束 finish_reason=%q（无 usage 块）正文长度=%d 字符 chatID=%s", lastFinishReason, len(result), memChatID)
	}
	if !scanner.Completed() && strings.TrimSpace(lastFinishReason) == "" {
		if !hasUsableStreamPayload(result, reasoningResult, tls) {
			logging.Warn("流式响应未收到 [DONE]，且未提供 finish_reason；本次输出没有有效正文/推理/工具调用，视为异常结束并交由上层重试/兜底")
			return nil, fmt.Errorf("流式响应异常结束：未收到 [DONE] 或 finish_reason")
		}
		if !streamPayloadLooksComplete(result, reasoningResult, tls) {
			logging.Warn("流式响应未收到 [DONE]，且未提供 finish_reason；正文/工具调用未完整收尾，疑似中途断流，交由上层重试/兜底")
			return nil, fmt.Errorf("流式响应疑似中途截断：未收到 [DONE] 或 finish_reason，且内容未完整收尾")
		}
		logging.Warn("流式响应未收到 [DONE]，且未提供 finish_reason；内容看起来已完整收尾，按兼容模式视为正常 EOF，避免重复重试")
	}
	if lastFinishReason == "length" {
		logging.Warn("模型因 max_tokens 上限结束（finish_reason=length），输出可能被截断；可在 config 增加 max_output_tokens 或设置环境变量 LEIAGENT_LLM_MAX_OUTPUT_TOKENS")
	}

	return &ToolAndContent{
		ToolList:         tls,
		Content:          result,
		ReasoningContent: reasoningResult,
		NeedAction:       needAction,
	}, nil
}

func hasUsableStreamPayload(content string, reasoningContent string, tools []openaistyle.ChatCompletionToolCall) bool {
	return strings.TrimSpace(content) != "" || strings.TrimSpace(reasoningContent) != "" || len(tools) > 0
}

func streamPayloadLooksComplete(content string, reasoningContent string, tools []openaistyle.ChatCompletionToolCall) bool {
	if len(tools) > 0 {
		return toolCallsLookComplete(tools)
	}
	text := strings.TrimSpace(content)
	if text == "" {
		text = strings.TrimSpace(reasoningContent)
	}
	if text == "" {
		return false
	}
	if strings.Count(text, "```")%2 != 0 {
		return false
	}
	if (strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[")) && json.Valid([]byte(text)) {
		return true
	}

	text = strings.TrimRight(text, "\"'”’)]}）】」》")
	if text == "" {
		return false
	}
	runes := []rune(text)
	switch runes[len(runes)-1] {
	case '.', '!', '?', '。', '！', '？', '…':
		return true
	default:
		return false
	}
}

func toolCallsLookComplete(tools []openaistyle.ChatCompletionToolCall) bool {
	for _, tool := range tools {
		if tool.Function == nil {
			return false
		}
		if strings.TrimSpace(tool.Function.Name) == "" {
			return false
		}
		args := strings.TrimSpace(tool.Function.Arguments)
		if args == "" || !json.Valid([]byte(args)) {
			return false
		}
	}
	return true
}

func consumeNeedActionHeader(buffer string) (parsed bool, needAction bool, rest string, flushRaw bool) {
	const trueHeader = "[needAction:true]"
	const falseHeader = "[needAction:false]"

	trimmed := strings.TrimLeft(buffer, " \t\r\n")
	if strings.HasPrefix(trimmed, trueHeader) {
		return true, true, trimmed[len(trueHeader):], false
	}
	if strings.HasPrefix(trimmed, falseHeader) {
		return true, false, trimmed[len(falseHeader):], false
	}
	if strings.HasPrefix(trueHeader, trimmed) || strings.HasPrefix(falseHeader, trimmed) {
		return false, false, "", false
	}
	if len([]rune(trimmed)) < len([]rune(falseHeader)) && strings.HasPrefix("[needAction:", trimmed) {
		return false, false, "", false
	}
	if len([]rune(trimmed)) < 64 {
		return false, false, "", false
	}
	return false, false, "", true
}

func (p *Proxy) handleNonStreamResponse(ctx context.Context, resp *http.Response, info *ModelAPIInfo) (*ToolAndContent, error) {
	logging.Info("开始处理非流式响应")

	memChatID, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok || strings.TrimSpace(memChatID) == "" {
		logging.Error("无法从 context 中获取 chatId")
		return nil, fmt.Errorf("无法从 context 中获取 chatId")
	}

	skipDialog := false
	if v, ok := ctx.Value(utils.SkipDialogToUIString).(bool); ok && v {
		skipDialog = true
	}

	openaiResp, err := p.convertResponse(resp, info)
	if err != nil {
		return nil, err
	}

	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("响应中没有选择")
	}
	fr := strings.TrimSpace(openaiResp.Choices[0].FinishReason)
	if openaiResp.Usage != nil {
		logging.Info("非流式响应 finish_reason=%q completion_tokens=%d prompt_tokens=%d total=%d",
			fr, openaiResp.Usage.CompletionTokens, openaiResp.Usage.PromptTokens, openaiResp.Usage.TotalTokens)
	} else {
		logging.Info("非流式响应 finish_reason=%q（无 usage）", fr)
	}
	if fr == "length" {
		logging.Warn("模型因 max_tokens 上限结束（finish_reason=length），输出可能被截断；可调大 max_output_tokens 或 LEIAGENT_LLM_MAX_OUTPUT_TOKENS")
	}
	logging.Info("响应内容: %s", openaiResp.Choices[0].Message.Content)
	tools := openaiResp.Choices[0].Message.ToolCalls

	var content string
	switch c := openaiResp.Choices[0].Message.Content.(type) {
	case string:
		content = c
	default:
		if openaiResp.Choices[0].Message.Content != nil {
			content = fmt.Sprint(openaiResp.Choices[0].Message.Content)
		}
	}
	needAction := false
	if v, ok := ctx.Value(utils.NeedActionHeaderString).(bool); ok && v {
		parsed, parsedNeedAction, rest, flushRaw := consumeNeedActionHeader(content)
		if parsed && !flushRaw {
			needAction = parsedNeedAction
			content = rest
		}
	}

	// 工具续问模式：如果模型输出的是 tool-complete JSON，则前端/DB 只展示 payload.Content。
	if payload, ok := utils.ParseToolCompletePayload(content); ok {
		if s := strings.TrimSpace(payload.Content); s != "" {
			content = s
		}
	}

	if !skipDialog {
		if strings.TrimSpace(content) != "" {
			globalchannel.SendAssitantMessageOnce(ctx, content, effectiveUsageTotal(openaiResp.Usage))
		}
	}

	if len(tools) > 0 {
		logging.Info("tools: %s", tools[0].Function.Name)
	}

	return &ToolAndContent{
		ToolList:   tools,
		Content:    content + "\n",
		NeedAction: needAction,
	}, nil
}

func (p *Proxy) convertResponse(resp *http.Response, info *ModelAPIInfo) (*openaistyle.ChatCompletionResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}
	logging.Info("响应体: %s", string(body))

	if info.provider == "gemini" {
		//fmt.Println("convertResponse gemini")

		geminiResponse := &gemini.ChatCompletionResponse{}
		if err := json.Unmarshal(body, geminiResponse); err != nil {
			//fmt.Println("convertResponse err: ", err)
			logging.Error("解析Gemini JSON失败:")
			return nil, fmt.Errorf("解析JSON失败: %w", err)
		}

		openaiResp := gemini.ConvertToOpenAIResponse(geminiResponse)
		logging.Info("openaiResp: %v", openaiResp)
		return openaiResp, nil
	}

	openaiResp := &openaistyle.ChatCompletionResponse{}
	if err := json.Unmarshal(body, openaiResp); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}
	return openaiResp, nil

}

type StreamScanner struct {
	scanner     *bufio.Scanner
	requestID   string
	created     int64
	current     []byte
	err         error
	hasSentRole bool
	completed   bool
}

func NewStreamScanner(r io.Reader) *StreamScanner {
	const maxLine = 1024 * 1024
	buf := make([]byte, 0, 64*1024)
	sc := bufio.NewScanner(r)
	sc.Buffer(buf, maxLine)
	return &StreamScanner{
		scanner:   sc,
		requestID: fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		created:   time.Now().Unix(),
	}
}

func (s *StreamScanner) Scan() bool {
	for s.scanner.Scan() {
		line := s.scanner.Text()

		//logging.Info("line: %s", line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		//fmt.Println("line: %s", line)
		dataStr := strings.TrimPrefix(line, "data: ")
		if strings.TrimSpace(dataStr) == "[DONE]" {
			s.completed = true
			return false
		}

		s.current = []byte(dataStr)
		return true

	}
	if s.scanner.Err() != nil {
		s.err = s.scanner.Err()
	}
	return false
}

func (s *StreamScanner) Bytes() []byte {
	return s.current
}

func (s *StreamScanner) Err() error {
	return s.err
}

func (s *StreamScanner) Completed() bool {
	return s.completed
}

// 定义结构体用于返回工具对象和内容
type ToolAndContent struct {
	ToolList         []openaistyle.ChatCompletionToolCall
	Content          string
	ReasoningContent string
	NeedAction       bool
}
