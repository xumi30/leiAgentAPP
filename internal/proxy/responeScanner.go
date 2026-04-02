package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	gemini "leiAgent/internal/provider/Gemin"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/logging"
	"leiAgent/utils"
	"net/http"
	"strings"
	"time"
)

func (p *Proxy) handleResponse(ctx context.Context, resp *http.Response, isStream bool) (*ToolAndContent, error) {

	if isStream {
		return p.handleStreamResponse(ctx, resp)
	}
	return p.handleNonStreamResponse(ctx, resp)
}

func (p *Proxy) handleStreamResponse(ctx context.Context, resp *http.Response) (*ToolAndContent, error) {
	logging.Info("开始处理流式响应: %v", resp)
	var fullContent strings.Builder
	var fullReasoningContent strings.Builder

	scanner := NewStreamScanner(resp.Body)
	tls := []openaistyle.ChatCompletionToolCall{}

	dialogOutChan := utils.OutputChan
	if dpOutchan, ok := ctx.Value(utils.DPDialogOutputChanString).(*chan string); ok {
		//logging.Info("使用Dispatcher的输出通道")
		dialogOutChan = *dpOutchan
	}

	reasoningOutChan := utils.ReasoningChan
	if dpReasoningOutchan, ok := ctx.Value(utils.DPReasoningOutputChanString).(*chan string); ok {
		//logging.Info("使用Dispatcher的推理输出通道")
		reasoningOutChan = *dpReasoningOutchan
	}

	for scanner.Scan() {

		var response openaistyle.ChatCompletionResponse

		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			logging.Error("解析流式JSON失败: %v", err)
			continue
		}

		if len(response.Choices) == 0 {
			continue
		}

		// 处理工具调用

		tools := response.Choices[0].Delta.ToolCalls
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
		content, ok := response.Choices[0].Delta.Content.(string)
		if ok && content != "" {
			fullContent.WriteString(content)
			//返回生成内容
			//fmt.Println("生成内容: ", content)
			dialogOutChan <- content
		}

		// 处理推理内容
		reasoningContent := response.Choices[0].Delta.ReasoningContent
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
	logging.Info("流式响应处理完成，返回结果")

	return &ToolAndContent{
		ToolList:         tls,
		Content:          result,
		ReasoningContent: reasoningResult,
	}, nil
}

func (p *Proxy) handleNonStreamResponse(ctx context.Context, resp *http.Response) (*ToolAndContent, error) {
	logging.Info("开始处理非流式响应")

	outChan := utils.OutputChan
	if dpOutchan, ok := ctx.Value(utils.DPDialogOutputChanString).(*chan string); ok {
		outChan = *dpOutchan
	}

	openaiResp, err := p.convertResponse(resp)
	if err != nil {
		return nil, err
	}

	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("响应中没有选择")
	}
	logging.Info("响应内容: %s", openaiResp.Choices[0].Message.Content)

	tools := openaiResp.Choices[0].Message.ToolCalls

	content, ok := openaiResp.Choices[0].Message.Content.(string)

	if !ok {
		logging.Error("Message.Content不是字符串类型: %v", openaiResp.Choices[0].Message.Content)

	} else {
		go func() { outChan <- openaiResp.Choices[0].Message.Content.(string) + "\n" }()
	}

	if len(tools) > 0 {
		logging.Info("tools: %s", tools[0].Function.Name)
	}

	return &ToolAndContent{
		ToolList: tools,
		Content:  content + "\n",
	}, nil
}

func (p *Proxy) convertResponse(resp *http.Response) (*openaistyle.ChatCompletionResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}
	logging.Info("响应体: %s", string(body))

	if p.modelAPIInfo.provider == "gemini" {
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
