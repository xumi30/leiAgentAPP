package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	mcpbridge "leiAgent/internal/MCP"
	"leiAgent/internal/memory"
	gemini "leiAgent/internal/provider/Gemin"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	streamModeNonStream = 0 // 仅非流式
	streamModeStream    = 1 // 仅流式
	streamModeBoth      = 3 // 由请求/上下文决定

	// 默认补全上限（原 3000 易导致长 JSON / 多段正文在流式下被 length 截断）
	defaultMaxOutputTokens = 8192
	// 任务规划阶段模型常输出大块 JSON（含多步、长 content），单独抬高下限
	planningMinOutputTokens = 16384
)

type Proxy struct {
	httpClient *http.Client
	backends   []*ModelAPIInfo // 按顺序尝试，失败则故障转移到下一条
}

var (
	defaultHTTPClient     *http.Client
	defaultHTTPClientOnce sync.Once
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

// NewProxy 创建代理。httpClient 为 nil 时使用进程内共享的默认 Client。
// 配置见 config/config.yaml：llm（api_key、base_url、model）或多后端 llm_backends（按顺序 failover）。
func NewProxy(httpClient *http.Client) (*Proxy, error) {
	if httpClient == nil {
		httpClient = sharedHTTPClient()
	}
	backends, err := loadModelConfigs()
	if err != nil {
		return nil, err
	}
	if len(backends) == 0 {
		return nil, fmt.Errorf("未加载到任何 LLM 后端")
	}
	return &Proxy{
		httpClient: httpClient,
		backends:   backends,
	}, nil
}

func (p *Proxy) Communicate(ctx context.Context) (*ToolAndContent, error) {
	chatIDVal := ctx.Value(utils.ChatIDString)
	chatID, ok := chatIDVal.(string)
	if !ok || chatID == "" {
		return nil, fmt.Errorf("context 缺少有效的 chatID")
	}

	var sourceMessages []*memory.Message
	if override, ok := ctx.Value(utils.MemoryMessagesOverrideString).([]*memory.Message); ok && len(override) > 0 {
		sourceMessages = override
	} else {
		sourceMessages = memory.GetLocalMemory().GetMessages(chatID)
	}

	return p.communicateWithChatMessages(ctx, convertMessages(sourceMessages))
}

func (p *Proxy) CommunicateWithMessages(ctx context.Context, messages []openaistyle.ChatMessage) (*ToolAndContent, error) {
	return p.communicateWithChatMessages(ctx, messages)
}

func (p *Proxy) communicateWithChatMessages(ctx context.Context, chatMessages []openaistyle.ChatMessage) (*ToolAndContent, error) {
	isStream := true
	if val, ok := ctx.Value(utils.IsStreamString).(bool); ok {
		isStream = val
	}

	var lastErr error
	for i, info := range p.backends {
		label := info.logLabel()
		if label == "" {
			label = fmt.Sprintf("#%d", i)
		}

		jsonData, err := p.makeRequestJSONFromChatMessages(ctx, info, chatMessages)
		if err != nil {
			lastErr = err
			logging.Error("LLM 后端 %s 构造请求失败: %v", label, err)
			continue
		}

		req, err := http.NewRequestWithContext(ctx, "POST", info.url, bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = err
			logging.Error("LLM 后端 %s 创建 HTTP 请求失败: %v", label, err)
			continue
		}

		resp, err := p.doRequest(req, info)
		if err != nil {
			lastErr = err
			logging.Warn("LLM 后端 %s 请求失败，尝试下一后端: %v", label, err)
			continue
		}

		if info.isStream == streamModeNonStream {
			isStream = false
		}

		toolAndContent, err := p.handleResponse(ctx, resp, isStream, info)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			logging.Warn("LLM 后端 %s 解析响应失败，尝试下一后端: %v", label, err)
			continue
		}

		logging.Info("LLM 后端 %s 处理响应成功", label)
		return toolAndContent, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("全部 LLM 后端均失败（共 %d 条）: %w", len(p.backends), lastErr)
	}
	return nil, fmt.Errorf("全部 LLM 后端均失败（共 %d 条）", len(p.backends))
}

func (p *Proxy) doRequest(requestinfo *http.Request, info *ModelAPIInfo) (*http.Response, error) {
	switch info.provider {
	case "gemini":
		requestinfo.Header.Set("x-goog-api-key", info.token)

	default:
		requestinfo.Header.Set("Authorization", "Bearer "+info.token)
	}

	requestinfo.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(requestinfo)

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
func resolveMaxOutputTokens(ctx context.Context, info *ModelAPIInfo) int {
	maxTok := defaultMaxOutputTokens
	if info.maxOutputTokens > 0 {
		maxTok = info.maxOutputTokens
	}
	if v := strings.TrimSpace(os.Getenv("LEIAGENT_LLM_MAX_OUTPUT_TOKENS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxTok = n
		}
	}
	if v, ok := ctx.Value(utils.IsPlanningString).(bool); ok && v {
		if maxTok < planningMinOutputTokens {
			maxTok = planningMinOutputTokens
		}
	}
	return maxTok
}

func (p *Proxy) makeRequestJson(ctx context.Context, info *ModelAPIInfo) ([]byte, error) {
	chatIDVal := ctx.Value(utils.ChatIDString)
	chatID, ok := chatIDVal.(string)
	if !ok || chatID == "" {
		return nil, fmt.Errorf("context 缺少有效的 chatID")
	}

	var sourceMessages []*memory.Message
	if override, ok := ctx.Value(utils.MemoryMessagesOverrideString).([]*memory.Message); ok && len(override) > 0 {
		sourceMessages = override
	} else {
		sourceMessages = memory.GetLocalMemory().GetMessages(chatID)
	}

	return p.makeRequestJSONFromChatMessages(ctx, info, convertMessages(sourceMessages))
}

func (p *Proxy) makeRequestJSONFromChatMessages(ctx context.Context, info *ModelAPIInfo, chatMessages []openaistyle.ChatMessage) ([]byte, error) {
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
		if ok && topic != "" {
			logging.Info("正在加载工具话题 %v, source=%s", topic, source)
		}
		toolRegister := tools.Getregistry()
		switch strings.TrimSpace(source) {
		case utils.ToolSourceMCP:
			logging.Info("正在加载 MCP 工具")
			tls = mcpbridge.BuildDynamicToolsByTopic(topic)
		case utils.ToolSourceMixed:
			logging.Info("正在加载混合工具")
			tls = append(tls, toolRegister.ConvertToolsByTopic(topic)...)
			tls = append(tls, mcpbridge.BuildDynamicToolsByTopic(topic)...)
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
		for _, t := range tls {
			logging.Info("加载完成的工具 %v", t.Function.Name)
		}
	}

	maxTok := resolveMaxOutputTokens(ctx, info)
	logging.Info("LLM 请求 max_tokens=%d", maxTok)

	opts := []openaistyle.Option{
		openaistyle.WithModel(info.modelName),
		openaistyle.WithMessages(chatMessages),
		openaistyle.WithMaxTokens(maxTok),
		openaistyle.WithStream(isStream),
		openaistyle.WithStreamIncludeUsage(isStream),
		openaistyle.WithTools(tls),
		openaistyle.WithToolChoice(toolChoice),
	}
	// Some OpenAI-compatible gateways reject non-standard thinking fields even
	// when they are set to "disabled". In disabled mode we simply omit them.
	if IsLLMThinkingDisabled() {
		opts = append(opts,
			openaistyle.WithEnableThinking(false),
			openaistyle.WithThinking(&openaistyle.ChatThinking{Type: openaistyle.ThinkingDisabled}),
		)
	}

	req := openaistyle.NewChatCompletionRequest(opts...)

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if info.provider == "gemini" {
		req := gemini.ConvertFromOpenAIRequest(req)

		jsonData, err = json.Marshal(req)
		if err != nil {
			return nil, err
		}
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
