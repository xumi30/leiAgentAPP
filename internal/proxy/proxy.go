package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	mcpbridge "leiAgent/internal/MCP"
	"leiAgent/internal/capabilities"
	"leiAgent/internal/memory"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// 默认补全上限（原 3000 易导致长 JSON / 多段正文在流式下被 length 截断）
	defaultMaxOutputTokens = 8192
	// 任务规划阶段模型常输出大块 JSON（含多步、长 content），单独抬高下限
	planningMinOutputTokens = 16384
)

type Client struct {
	httpClient *http.Client
	config     *modelConfig
}

var (
	defaultHTTPClient     *http.Client
	defaultHTTPClientOnce sync.Once
	NotifyLLMProblem      func(ctx context.Context, message string)
)

func sharedHTTPClient() *http.Client {
	defaultHTTPClientOnce.Do(func() {
		defaultHTTPClient = &http.Client{
			Timeout: 300 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	})
	return defaultHTTPClient
}

// NewClient 创建单一 OpenAI-compatible LLM 客户端。
func NewClient(httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		httpClient = sharedHTTPClient()
	}
	config, err := loadModelConfig()
	if err != nil {
		return nil, err
	}
	return &Client{
		httpClient: httpClient,
		config:     config,
	}, nil
}

func (p *Client) Communicate(ctx context.Context) (*ToolAndContent, error) {
	chatIDVal := ctx.Value(utils.ChatIDString)
	chatID, ok := chatIDVal.(string)
	if !ok || chatID == "" {
		return nil, fmt.Errorf("context 缺少有效的 chatID")
	}

	var sourceMessages []*memory.Message
	if override, ok := ctx.Value(utils.MemoryMessagesOverrideString).([]*memory.Message); ok && len(override) > 0 {
		sourceMessages = override
	} else {
		raw := memory.GetLocalMemory().GetMessages(chatID)
		if built, ok, err := BuildMemoryMessagesForLLM(chatID, raw); err == nil && ok && len(built) > 0 {
			sourceMessages = built
		} else {
			sourceMessages = raw
		}
	}

	return p.communicateWithChatMessages(ctx, convertMessages(sourceMessages))
}

func (p *Client) CommunicateWithMessages(ctx context.Context, messages []openaistyle.ChatMessage) (*ToolAndContent, error) {
	return p.communicateWithChatMessages(ctx, messages)
}

func (p *Client) communicateWithChatMessages(ctx context.Context, chatMessages []openaistyle.ChatMessage) (*ToolAndContent, error) {
	isStream := true
	if val, ok := ctx.Value(utils.IsStreamString).(bool); ok {
		isStream = val
	}

	jsonData, err := p.makeRequestJSONFromChatMessages(ctx, chatMessages)
	if err == nil {
		var request *http.Request
		request, err = http.NewRequestWithContext(ctx, http.MethodPost, p.config.url, bytes.NewBuffer(jsonData))
		if err == nil {
			var response *http.Response
			response, err = p.doRequest(request)
			if err == nil {
				defer response.Body.Close()
				return p.handleResponse(ctx, response, isStream)
			}
		}
	}
	if NotifyLLMProblem != nil {
		NotifyLLMProblem(ctx, err.Error())
	}
	return nil, err
}

func (p *Client) doRequest(request *http.Request) (*http.Response, error) {
	if p.config.token != "" {
		request.Header.Set("Authorization", "Bearer "+p.config.token)
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(request)

	if err != nil {
		logging.Error("发送请求失败：%v", err)
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		msg := strings.TrimSpace(string(body))
		logging.Error("请求失败,状态码：%d, body: %s", resp.StatusCode, msg)
		if msg != "" {
			return nil, fmt.Errorf("请求失败,状态码：%d：%s", resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("请求失败,状态码：%d", resp.StatusCode)
	}
	logging.Info("发送请求成功")
	return resp, nil
}

// resolveMaxOutputTokens 决定本次请求的 max_tokens：配置项、环境变量、规划模式下限。
func resolveMaxOutputTokens(ctx context.Context, config *modelConfig) int {
	maxTok := defaultMaxOutputTokens
	if config.maxOutputTokens > 0 {
		maxTok = config.maxOutputTokens
	}
	if v, ok := ctx.Value(utils.IsPlanningString).(bool); ok && v {
		if maxTok < planningMinOutputTokens {
			maxTok = planningMinOutputTokens
		}
	}
	return maxTok
}

func (p *Client) makeRequestJSONFromChatMessages(ctx context.Context, chatMessages []openaistyle.ChatMessage) ([]byte, error) {
	isStream := true
	if val, ok := ctx.Value(utils.IsStreamString).(bool); ok {
		isStream = val
	}
	if extraSystemMessages, ok := ctx.Value(utils.ExtraSystemMessagesString).([]string); ok && len(extraSystemMessages) > 0 {
		prefixed := make([]openaistyle.ChatMessage, 0, len(extraSystemMessages)+len(chatMessages))
		for _, msg := range extraSystemMessages {
			msg = strings.TrimSpace(msg)
			if msg == "" {
				continue
			}
			prefixed = append(prefixed, openaistyle.ChatMessage{
				Role:    openaistyle.RoleSystem,
				Content: msg,
			})
		}
		prefixed = append(prefixed, chatMessages...)
		chatMessages = prefixed
	}

	tls := []openaistyle.Tool{}
	var toolChoice *openaistyle.ToolChoice
	if istool, ok := ctx.Value(utils.ToolsString).(bool); ok && istool {
		logging.Info("正在加载工具")
		toolChoice = &openaistyle.ToolChoice{Type: openaistyle.ToolChoiceAuto}
		topic, ok := ctx.Value(utils.ToolTopicToLoad).(string)
		source, _ := ctx.Value(utils.ToolSourceToLoad).(string)
		source = capabilities.DefaultToolSource(source)
		if prompt := capabilities.BuildSystemPrompt(topic, source); strings.TrimSpace(prompt) != "" {
			chatMessages = append([]openaistyle.ChatMessage{{
				Role:    openaistyle.RoleSystem,
				Content: prompt,
			}}, chatMessages...)
		}
		if ok && topic != "" {
			logging.Info("正在加载工具话题 %v, source=%s", topic, source)
		}
		toolRegister := tools.Getregistry()
		switch strings.TrimSpace(source) {
		case utils.ToolSourceMCP:
			logging.Info("正在加载 MCP 工具")
			tls = capabilities.BuildMCPToolsByTopic(topic)
		case utils.ToolSourceMixed:
			logging.Info("正在加载混合工具")
			tls = append(tls, toolRegister.ConvertToolsByTopic(topic)...)
			tls = append(tls, capabilities.BuildMCPToolsByTopic(topic)...)
		case utils.ToolSourceLocal:
			logging.Info("正在加载本地工具")
			if ok {
				tls = toolRegister.ConvertToolsByTopic(topic)
			} else {
				tls = toolRegister.ConvertTools()
			}
		}
		if len(tls) == 0 && strings.TrimSpace(source) == utils.ToolSourceMCP {
			logging.Warn("MCP source requested but no dynamic tools found for topic=%s, fallback to local tools", topic)
			if ok {
				tls = toolRegister.ConvertToolsByTopic(topic)
			} else {
				tls = toolRegister.ConvertTools()
			}
		}
		if !ok {
			tls = toolRegister.ConvertTools()
		}
		tls = capabilities.AppendUnique(tls, capabilities.SupportTools()...)
		for _, t := range tls {
			logging.Info("加载完成的工具 %v", t.Function.Name)
		}
	}

	maxTok := resolveMaxOutputTokens(ctx, p.config)
	logging.Info("LLM 请求 max_tokens=%d", maxTok)

	opts := []openaistyle.Option{
		openaistyle.WithModel(p.config.modelName),
		openaistyle.WithMessages(chatMessages),
		openaistyle.WithMaxTokens(maxTok),
		openaistyle.WithStream(isStream),
		openaistyle.WithStreamIncludeUsage(isStream),
		openaistyle.WithTools(tls),
		openaistyle.WithToolChoice(toolChoice),
	}
	req := openaistyle.NewChatCompletionRequest(opts...)

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	logging.Info("请求参数：%s", string(jsonData))
	return jsonData, nil
}

func convertMessages(messages []*memory.Message) []openaistyle.ChatMessage {
	logging.Info("convertMessages")
	chatMessages := make([]openaistyle.ChatMessage, 0, len(messages))
	// 清理历史中残留的 <tool_code> 文本，防止模型模仿
	toolCodeRe := regexp.MustCompile("(?s)<tool_code>\\s*.*?\\s*</tool_code>")
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		content := msg.Content
		if content != "" && strings.Contains(content, "<tool_code>") {
			content = toolCodeRe.ReplaceAllString(content, "")
			content = strings.TrimSpace(content)
		}
		if content == "" && len(msg.ToolCalls) == 0 && msg.ToolCallID == "" {
			continue
		}
		chatMsg := openaistyle.ChatMessage{
			Role:    string(msg.Role),
			Content: content,
		}

		if len(msg.ToolCalls) > 0 {
			toolCalls := make([]openaistyle.ToolCall, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				toolName := tc.Function.Name
				tl, regOK := tools.Getregistry().Get(toolName)
				var desc string
				var params map[string]interface{}
				if regOK {
					desc = tl.Description()
					params = tl.Parameters()
				} else if dynDesc, dynParams, dynOK := mcpbridge.GetDynamicToolMeta(toolName); dynOK {
					desc = dynDesc
					params = dynParams
				} else {
					logging.Error("工具 %s 不存在", toolName)
				}
				var args map[string]interface{}
				if tc.Function.Arguments != "" {
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						logging.Error("解析工具调用参数失败 %s: %v", toolName, err)
					}
				}
				toolCalls = append(toolCalls, openaistyle.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: &openaistyle.Function{
						Name:        tc.Function.Name,
						Description: desc,
						Parameters:  params,
						Arguments:   args,
					},
					Index: tc.Index,
				})
			}
			chatMsg.ToolCalls = toolCalls
		}

		if msg.ToolCallID != "" {
			chatMsg.ToolCallID = msg.ToolCallID
		}

		chatMessages = append(chatMessages, chatMsg)
	}
	return chatMessages
}
