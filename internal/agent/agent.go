package agent

import (
	"context"
	"encoding/json"
	"fmt"
	globalchannel "leiAgent/internal"
	"leiAgent/internal/memory"
	"leiAgent/internal/proxy"
	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"
)

type Agent struct {
	systemPrompt  string
	proxy         *proxy.Proxy
	taskLoopTimes int
	ctx           context.Context
}

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

func NewAgent(opts ...options) *Agent {
	a := &Agent{
		proxy: proxy.NewProxy(nil),
	}
	for _, opt := range opts {
		opt(a)
	}
	a.taskLoopTimes = 10 //单个任务的最大循环次数，防止死循环，默认5次

	return a
}
func (a *Agent) Run() (string, error) {
	inputChan := utils.InputChan

	for {
		select {
		case <-a.ctx.Done():
			return "", a.ctx.Err()
		case message := <-inputChan:
			logging.Debug("收到消息: %s", message)
			rp, err := a.HandleChat(message)
			if err != nil {
				logging.Error("处理消息失败: %v", err)
			}
			logging.Debug("返回消息: %s", rp)
		}
	}
}

func (a *Agent) HandleChat(message string) (string, error) {

	logging.Info("Agent begin to handle chat")

	chatId := a.ctx.Value(utils.ChatIDString).(string)
	logging.Info("chatId: %s", chatId)

	if a.systemPrompt != "" {
		memory.SetSystemPrompt(chatId, a.systemPrompt)
	}
	memory.AddUserMessage(chatId, message)

	toolAndContent, err := a.proxy.Communicate(a.ctx)
	//logging.Info("代理返回信息: %v", toolAndContent)

	if err != nil {
		return "", fmt.Errorf("通信失败: %w", err)
	}

	if toolAndContent == nil {
		return "", fmt.Errorf("代理返回空内容")
	}

	a.recordMeomoryFromResponse(a.ctx, toolAndContent)

	return toolAndContent.Content, nil
}

func (a *Agent) recordMeomoryFromResponse(ctx context.Context, toolAndContent *proxy.ToolAndContent) {

	logging.Info("开始记忆返回信息")

	chatId := ctx.Value(utils.ChatIDString).(string)

	if len(toolAndContent.ToolList) > 0 {
		// 工具执行
		logging.Info("开始执行工具:")
		a.executeTools(ctx, toolAndContent)

	}

	if toolAndContent.Content != "" {
		memory.AddAssistantContentMessage(chatId, toolAndContent.Content)
	}
}

func (a *Agent) executeTools(ctx context.Context, toolAndContent *proxy.ToolAndContent) {

	chatId := ctx.Value(utils.ChatIDString).(string)

	dialogOutChan := globalchannel.GetGlobalDialogOutChannel(chatId)

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
		if !flag {
			outStr = fmt.Sprintf("工具%s不存在", toolname)
			memory.AddToolMessage(chatId, tool.ID, outStr)
			logging.Error("%s", outStr)
			continue
		}

		outStr = fmt.Sprintf("开始调用工具%s, 参数是%s", toolname, tool.Function.Arguments)
		dialogOutChan <- outStr + "\n"
		logging.Info("%s", outStr)

		str, err := functl.Execute(ctx, tool.Function.Arguments)
		if err != nil {
			outStr = fmt.Sprintf("工具%s执行失败: %v", toolname, err)
			logging.Error("%s", outStr)
			memory.AddToolMessage(chatId, tool.ID, outStr)
			dialogOutChan <- outStr + "\n"

			continue
		}

		outStr = fmt.Sprintf("工具%s执行成功: %s", toolname, str)
		logging.Info("%s", outStr)
		memory.AddToolMessage(chatId, tool.ID, outStr)
		dialogOutChan <- outStr + "\n"
	}

	if a.taskLoopTimes >= 0 {
		logging.Info("工具执行完成,继续请求模型生成最终回复")
		a.taskLoopTimes--
		a.HandleChat("工具已经执行完成,请继续。如果需要调用工具，请继续调用。如果不需要调用工具了，请直接给出最终回复。")
	}
	logging.Info("工具执行完成,或者达到最大循环次数,结束工具执行")

}
