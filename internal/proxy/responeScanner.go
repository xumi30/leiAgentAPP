package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	globalchannel "leiAgent/internal"
	gemini "leiAgent/internal/provider/Gemin"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/logging"
	"leiAgent/utils"
	"net/http"
	"strings"
	"time"
)

func (p *Proxy) handleResponse(ctx context.Context, resp *http.Response, isStream bool, info *ModelAPIInfo) (*ToolAndContent, error) {

	if isStream {
		return p.handleStreamResponse(ctx, resp)
	}
	return p.handleNonStreamResponse(ctx, resp, info)
}

func (p *Proxy) handleStreamResponse(ctx context.Context, resp *http.Response) (*ToolAndContent, error) {
	logging.Info("开始处理流式响应: %v", resp)
	var fullContent strings.Builder
	var fullReasoningContent strings.Builder

	scanner := NewStreamScanner(resp.Body)
	tls := []openaistyle.ChatCompletionToolCall{}
	var lastFinishReason string
	var lastUsage *openaistyle.TokenUsage

	chatId, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		logging.Error("无法从 context 中获取 chatId")
		return nil, fmt.Errorf("无法从 context 中获取 chatId")
	}

	dialogOutChan := globalchannel.GetGlobalDialogOutChannel(chatId)

	reasoningOutChan := globalchannel.GetGlobalReasonOutChannel(chatId)

	for scanner.Scan() {

		var response openaistyle.ChatCompletionResponse

		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			logging.Error("解析流式JSON失败: %v", err)
			continue
		}

		if len(response.Choices) == 0 {
			continue
		}

		ch0 := response.Choices[0]
		if fr := strings.TrimSpace(ch0.FinishReason); fr != "" {
			lastFinishReason = fr
		}
		if response.Usage != nil {
			lastUsage = response.Usage
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
			fullContent.WriteString(content)
			//返回生成内容
			//fmt.Println("生成内容: ", content)
			dialogOutChan <- content
		}

		// 处理推理内容
		reasoningContent := delta.ReasoningContent
		if reasoningContent != "" {
			fullReasoningContent.WriteString(reasoningContent)
			//可选：是否输出推理内容
			if reasoningContent != "" {
				//fmt.Println("推理内容: ", reasoningContent)
				reasoningOutChan <- reasoningContent
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("扫描流式响应失败: %w", err)
	}

	result := fullContent.String()
	reasoningResult := fullReasoningContent.String()

	//fmt.Println("最终生成内容: ", result)
	//fmt.Println("最终推理内容: ", reasoningResult)

	//logging.Info("响应内容: %s", result)
	tlsJSON, err := json.Marshal(tls)
	if err != nil {
		logging.Error("序列化工具列表失败: %v", err)
	} else {
		logging.Info("返回调用tools: %s", string(tlsJSON))
	}
	// 如果流结束，发送一个"[DONE]"
	dialogOutChan <- utils.FinishString
	reasoningOutChan <- utils.FinishString
	if lastUsage != nil {
		logging.Info("流式响应结束 finish_reason=%q completion_tokens=%d prompt_tokens=%d total=%d 正文长度=%d 字符",
			lastFinishReason,
			lastUsage.CompletionTokens,
			lastUsage.PromptTokens,
			lastUsage.TotalTokens,
			len(result))
	} else {
		logging.Info("流式响应结束 finish_reason=%q（无 usage 块）正文长度=%d 字符", lastFinishReason, len(result))
	}
	if lastFinishReason == "length" {
		logging.Warn("模型因 max_tokens 上限结束（finish_reason=length），输出可能被截断；可在 config 增加 max_output_tokens 或设置环境变量 LEIAGENT_LLM_MAX_OUTPUT_TOKENS")
	}

	return &ToolAndContent{
		ToolList:         tls,
		Content:          result,
		ReasoningContent: reasoningResult,
	}, nil
}

func (p *Proxy) handleNonStreamResponse(ctx context.Context, resp *http.Response, info *ModelAPIInfo) (*ToolAndContent, error) {
	logging.Info("开始处理非流式响应")

	chatId, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		logging.Error("无法从 context 中获取 chatId")
		return nil, fmt.Errorf("无法从 context 中获取 chatId")
	}

	dialogOutChan := globalchannel.GetGlobalDialogOutChannel(chatId)

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
	dialogOutChan <- content + "\n"

	if len(tools) > 0 {
		logging.Info("tools: %s", tools[0].Function.Name)
	}

	return &ToolAndContent{
		ToolList: tools,
		Content:  content + "\n",
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
}

func NewStreamScanner(r io.Reader) *StreamScanner {
	return &StreamScanner{
		scanner:   bufio.NewScanner(r),
		requestID: fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		created:   time.Now().Unix(),
	}
}

func (s *StreamScanner) Scan() bool {
	for s.scanner.Scan() {
		line := s.scanner.Text()

		logging.Debug("line: %s", line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		//fmt.Println("line: %s", line)
		dataStr := strings.TrimPrefix(line, "data: ")
		if strings.TrimSpace(dataStr) == "[DONE]" {
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

// 定义结构体用于返回工具对象和内容
type ToolAndContent struct {
	ToolList         []openaistyle.ChatCompletionToolCall
	Content          string
	ReasoningContent string
}
