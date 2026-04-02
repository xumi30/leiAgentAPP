package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/memory"
	gemini "leiAgent/internal/provider/Gemin"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"

	"net/http"
	"time"
)

type Proxy struct {
	httpClient   *http.Client
	modelAPIInfo *ModelAPIInfo
	// TODO: add fields
}

func NewProxy(httpClient *http.Client) *Proxy {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 300 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	}
	modelAPIInfo := selectProviderStrategy()
	return &Proxy{
		httpClient:   httpClient,
		modelAPIInfo: modelAPIInfo,
	}

}

func (p *Proxy) Communicate(ctx context.Context) (*ToolAndContent, error) {

	isStream := true
	if val, ok := ctx.Value(utils.IsStreamString).(bool); ok {
		isStream = val
	}

	jsonData, err := p.makeRequestJson(ctx)
	if err != nil {
		return nil, err
	}

	requestinfo, err := http.NewRequest("POST", p.modelAPIInfo.url, bytes.NewBuffer(jsonData))
	if err != nil {
		logging.Error("创建请求失败：%v", err)
		return nil, err
	}

	resp, err := p.doRequest(requestinfo)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if p.modelAPIInfo.isStream == 0 {
		isStream = false
	}

	toolAndContent, err := p.handleResponse(ctx, resp, isStream)
	logging.Info("处理响应成功")

	return toolAndContent, nil
}

func (p *Proxy) doRequest(requestinfo *http.Request) (*http.Response, error) {
	switch p.modelAPIInfo.provider {
	case "gemini":
		requestinfo.Header.Set("x-goog-api-key", p.modelAPIInfo.token)

	default:
		requestinfo.Header.Set("Authorization", "Bearer "+p.modelAPIInfo.token)
	}

	requestinfo.Header.Set("Content-Type", "application/json")

	logging.Info("开始发送请求 %v", requestinfo)
	resp, err := p.httpClient.Do(requestinfo)

	if err != nil {
		logging.Error("发送请求失败：%v", err)
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		logging.Error("请求失败,状态码：%d", resp.StatusCode)
		return nil, fmt.Errorf("请求失败,状态码：%d", resp.StatusCode)
	}
	logging.Info("发送请求成功")
	return resp, nil
}

func (p *Proxy) makeRequestJson(ctx context.Context) ([]byte, error) {
	chatID := ctx.Value(utils.ChatIDString).(string)

	isStream := true
	if val, ok := ctx.Value(utils.IsStreamString).(bool); ok {
		isStream = val
	}

	intent, ok := ctx.Value(utils.IntentKey).(string)
	if !ok {
		intent = ""
	}
	tls := []openaistyle.Tool{}
	if intent == utils.ToolModeString {
		toolRegister := tools.Getregistry()
		tls = toolRegister.ConvertTools()
	}

	// 依靠记忆传递对话信息
	chatMessages := convertMessages(memory.GetLocalMemory().GetMessages(chatID))

	req := openaistyle.NewChatCompletionRequest(
		openaistyle.WithModel(p.modelAPIInfo.modelName),
		openaistyle.WithMessages(chatMessages),
		// Token 控制
		openaistyle.WithMaxTokens(3000),
		// 采样参数
		// openaistyle.WithTemperature(0.4),
		// openaistyle.WithTopP(0.9),
		// openaistyle.WithDoSample(true),
		// 频率惩罚
		// openaistyle.WithFrequencyPenalty(0.3),
		// 存在惩罚
		// openaistyle.WithPresencePenalty(0.2),
		// 停止词
		// openaistyle.WithStop(nil),
		// 流式输出
		openaistyle.WithStream(isStream),
		// 用户标识
		// openaistyle.WithUserID(""),
		// // 请求ID
		// openaistyle.WithRequestID(""),
		// 工具调用
		openaistyle.WithTools(tls),

		// openaistyle.WithEnablesearch(true),
		// 思维链配置
		// openaistyle.WithThinking(nil),

		// openaistyle.WithThinking(&chatThinking),

	)

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	// fmt.Println(1111, string(jsonData))
	if p.modelAPIInfo.provider == "gemini" {
		// 2. 转换为 Gemini 格式
		req := gemini.ConvertFromOpenAIRequest(req)

		jsonData, err = json.Marshal(req)
		if err != nil {
			return nil, err
		}
	}

	//logging.Info("请求体：%s", string(jsonData))
	return jsonData, nil
}

func convertMessages(messages []*memory.Message) []openaistyle.ChatMessage {
	logging.Info("convertMessages")
	chatMessages := make([]openaistyle.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		chatMsg := openaistyle.ChatMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		}

		// 如果有工具调用,添加工具调用信息
		if len(msg.ToolCalls) > 0 {
			toolCalls := make([]openaistyle.ToolCall, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				toolName := tc.Function.Name
				tl, flag := tools.Getregistry().Get(toolName)
				if flag != true {
					logging.Error("工具 %s 不存在", toolName)
					continue
				}
				toolCalls = append(toolCalls, openaistyle.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: &openaistyle.Function{
						Name:        tc.Function.Name,
						Description: tl.Description(),
						Parameters:  tl.Parameters(),
						Arguments:   tl.Parameters(),
					},
					Index: tc.Index,
				})
			}
			chatMsg.ToolCalls = toolCalls
		}

		// 如果是工具结果消息,添加工具调用ID
		if msg.ToolCallID != "" {
			chatMsg.ToolCallID = msg.ToolCallID
		}

		chatMessages = append(chatMessages, chatMsg)
	}
	return chatMessages
}

func selectProviderStrategy() *ModelAPIInfo {
	// TODO: 实现选择提供商的策略
	//panic("未实现选择提供商的策略")
	//return "zhipu", "GLM-4-Flash-250414", "63491ae217a9403fb667ac808e095b85.sktEuU7i3XhwjnrI"
	//"zhipu", "qwen-plus", "sk-da6d7a7da1914f6581d4825d7b790389"
	// ModelAPIInfozhipu := &ModelAPIInfo{
	// 	modelName: "glm-4.7-flash",
	// 	token:     "6b4c950aa280432db0b47ecbcaea1cf0.HnGjIJXNRY4ttNKs",
	// 	url:       "https://open.bigmodel.cn/api/paas/v4/chat/completions",
	// 	isStream:  3,
	// }

	ModelAPIInfo2 := &ModelAPIInfo{
		modelName: "qwen3.5-flash",
		token:     "sk-da6d7a7da1914f6581d4825d7b790389",
		url:       "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		isStream:  3,
	}

	// ModelAPIInfo2 := &ModelAPIInfo{
	// 	modelName: "Pro/zai-org/GLM-4.7",
	// 	token:     "sk-ubwbvugdhbqzbrddvqzgndxoaimnzwilhavgytxzvjbnsaev",
	// 	url:       "https://api.siliconflow.cn/v1/chat/completions",
	// }

	// ModelAPIInfoGemini := &ModelAPIInfo{
	// 	provider:  "gemini",
	// 	modelName: "gemini-3-flash-preview",
	// 	token:     "AIzaSyCdvYIXmvG9N2wdmh-lWJUAu_-ZtbuTMpA",
	// 	url:       "https://generativelanguage.googleapis.com/v1beta/models/gemini-3-flash-preview:generateContent",
	// 	isStream:  0,
	// }

	return ModelAPIInfo2
}
