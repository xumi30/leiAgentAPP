package agent

import (
	"context"
	"encoding/json"
	"fmt"
	mcpbridge "leiAgent/internal/MCP"
	"leiAgent/internal/globalchannel"
	"leiAgent/internal/memory"
	"leiAgent/internal/proxy"
	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"
	"strings"
	"time"
)

type Agent struct {
	systemPrompt  string
	proxy         *proxy.Proxy
	taskLoopTimes int
	ctx           context.Context
}

const defaultTaskLoopLimit = 4

type options func(*Agent)

func WithSystemPrompt(description string) options {
	return func(a *Agent) {
		a.systemPrompt = description
	}
}

func WithCtx(ctx context.Context) options {
	return func(a *Agent) {
		a.ctx = ctx
	}
}

func NewAgent(opts ...options) (*Agent, error) {
	p, err := proxy.NewProxy(nil)
	if err != nil {
		return nil, err
	}
	a := &Agent{
		proxy: p,
	}
	for _, opt := range opts {
		opt(a)
	}
	a.taskLoopTimes = defaultTaskLoopLimit

	return a, nil
}

// SetCtx 在会话中断后替换根 context（与 Dispatcher 换 ctx 同步，避免内部仍引用已取消的 ctx）。
func (a *Agent) SetCtx(ctx context.Context) {
	if a == nil {
		return
	}
	a.ctx = ctx
}
func (a *Agent) Run() (string, error) {
	chatId, ok := a.ctx.Value(utils.ChatIDString).(string)
	if !ok || chatId == "" {
		return "", fmt.Errorf("chatId is not found in context")
	}
	inputChan := globalchannel.GetGlobalInputChannel(chatId)

	for {
		select {
		case <-a.ctx.Done():
			return "", a.ctx.Err()
		case msg := <-inputChan:
			if msg == nil || msg.Content == "" {
				continue
			}
			logging.Debug("收到消息: %s", msg.Content)
			rp, err := a.HandleChat(a.ctx, msg.Content)
			if err != nil {
				logging.Error("处理消息失败: %v", err)
			}
			logging.Debug("返回消息: %s", rp)
		}
	}
}

func (a *Agent) HandleChat(ctx context.Context, message string) (string, error) {

	logging.Info("Agent begin to handle chat")

	chatId := ctx.Value(utils.ChatIDString).(string)
	logging.Info("chatId: %s", chatId)

	if a.systemPrompt != "" {
		memory.SetSystemPrompt(chatId, a.systemPrompt)
	}
	memory.AddUserMessage(chatId, message)

	toolAndContent, err := a.proxy.Communicate(ctx)
	logging.Info("代理返回信息: %v", toolAndContent)

	if err != nil {
		return "", fmt.Errorf("通信失败: %w", err)
	}

	if toolAndContent == nil {
		return "", fmt.Errorf("代理返回空内容")
	}

	a.recordMeomoryFromResponse(ctx, toolAndContent)

	return toolAndContent.Content, nil
}

func (a *Agent) BeginTask(ctx context.Context, message string) (string, error) {
	a.taskLoopTimes = defaultTaskLoopLimit
	return a.HandleChat(ctx, message)
}

func (a *Agent) recordMeomoryFromResponse(ctx context.Context, toolAndContent *proxy.ToolAndContent) {

	logging.Info("开始记忆返回信息")

	chatId := ctx.Value(utils.ChatIDString).(string)

	if len(toolAndContent.ToolList) > 0 {
		// 工具执行
		names := make([]string, 0, len(toolAndContent.ToolList))
		for _, tc := range toolAndContent.ToolList {
			if tc.Function.Name != "" {
				names = append(names, tc.Function.Name)
			}
		}
		logging.Info("开始执行工具: count=%d names=%s", len(toolAndContent.ToolList), strings.Join(names, ","))
		a.executeTools(ctx, toolAndContent)
	} else {
		logging.Info("本轮模型未触发工具调用（ToolList 为空）")
	}

	if toolAndContent.Content != "" {
		memory.AddAssistantContentMessage(chatId, toolAndContent.Content)
	}
}

func truncateForLog(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

func (a *Agent) executeTools(ctx context.Context, toolAndContent *proxy.ToolAndContent) {

	chatId := ctx.Value(utils.ChatIDString).(string)

	toolCalls := make([]memory.ToolCall, 0, len(toolAndContent.ToolList))

	for _, tool := range toolAndContent.ToolList {
		jsonStr, err := json.Marshal(tool)
		if err != nil {
			logging.Error("工具信息序列化失败: %v", err)
			continue
		}
		logging.Info("执行工具信息: %s", jsonStr)

		toolCalls = append(toolCalls, memory.ToolCall{
			ID:   tool.ID,
			Type: tool.Type,
			Function: memory.ToolCallFunction{
				Name:      tool.Function.Name,
				Arguments: tool.Function.Arguments,
			},
			Index: tool.Index,
		})
	}
	memory.AddAssistantToolCallsMessage(chatId, toolCalls)

	for _, tool := range toolAndContent.ToolList {
		toolname := tool.Function.Name
		var outStr string

		functl, flag := tools.Getregistry().Get(toolname)

		outStr = fmt.Sprintf("开始调用工具%s, 参数是%s", toolname, tool.Function.Arguments)
		globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", outStr))
		logging.Info("%s", outStr)

		start := time.Now()
		var (
			str string
			err error
		)
		if flag {
			str, err = functl.Execute(ctx, tool.Function.Arguments)
		} else if _, ok := mcpbridge.ResolveDynamicTool(toolname); ok {
			str, err = mcpbridge.ExecuteDynamicTool(ctx, toolname, tool.Function.Arguments)
		} else {
			outStr = fmt.Sprintf("工具%s不存在", toolname)
			memory.AddToolMessage(chatId, tool.ID, outStr)
			logging.Error("%s", outStr)
			break
		}
		elapsed := time.Since(start)
		if err != nil {
			outStr = fmt.Sprintf("工具%s执行失败: %v (elapsed=%s)", toolname, err, elapsed)
			logging.Error("%s", outStr)
			memory.AddToolMessage(chatId, tool.ID, outStr)
			globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", outStr))

			break
		}

		// resultPreview := truncateForLog(str, 1200)
		outStr = fmt.Sprintf("工具%s执行成功 (elapsed=%s): %s", toolname, elapsed, str)
		logging.Info("%s", outStr)
		memory.AddToolMessage(chatId, tool.ID, outStr)
		globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", outStr))
	}

	if a.taskLoopTimes >= 0 {
		logging.Info("工具执行完成,继续请求模型生成最终回复")
		a.taskLoopTimes--

		backInfo, err := a.HandleChat(ctx, fmt.Sprintf("工具已经执行完成,请继续。如果需要调用工具，请继续调用,如果目前工具缺少请返回{needToolToics:[topic1,topic2,topic3],message:string,},可选的topic有 %s。needToolToics 里的值必须是精确的 topic 名称本身，而不是带描述的整句。如果不需要调用工具了，请直接给出最终回复。", utils.ToolTopicsPromptText()))
		if err != nil {
			logging.Error("继续请求模型生成最终回复失败: %v", err)
			return
		}

		// 如果 backInfo 包含 needToolToics 字段，则按请求加载额外工具 topic 后继续对话。
		if needToolTopics, ok := utils.GetNeedToolToics(backInfo); ok {
			for _, topic := range needToolTopics {
				logging.Info("模型请求补充工具话题: %s", topic)
				nextCtx := context.WithValue(ctx, utils.ToolTopicToLoad, topic)

				if _, err := a.HandleChat(nextCtx, fmt.Sprintf("已按你的要求加载%s相关工具，请继续。如果仍需调用工具，请直接调用；如果不需要调用工具了，请直接给出最终回复。", topic)); err != nil {
					logging.Error("加载工具话题 %s 后继续请求失败: %v", topic, err)
					continue
				}
				return
			}

			logging.Warn("模型请求了额外工具话题，但未解析出可用 topic: %s", backInfo)
		}
	}
	logging.Info("工具执行完成,或者达到最大循环次数,结束工具执行")

}
