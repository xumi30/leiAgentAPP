package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/memory"
	"leiAgent/internal/proxy"
	"leiAgent/internal/tools"
	"leiAgent/logging"
	"leiAgent/utils"
)

type Agent struct {
	name        string
	description string

	proxy *proxy.Proxy

	taskLoopTimes int
}

type options func(*Agent)

func WithName(name string) options {
	return func(a *Agent) {
		a.name = name
	}
}
func WithDescription(description string) options {
	return func(a *Agent) {
		a.description = description
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
func (a *Agent) Run(ctx context.Context) (string, error) {
	inputChan := utils.InputChan
	chatId := ctx.Value(utils.ChatIDString).(string)
	memory.GetLocalMemory().SetSystemPrompt(chatId, `you are a helpful assistant. 
	在回答问题前，一定要仔细思考，尽可能地分解用户需求，分步骤去解决子任务，尽可能思考工具怎么编排调用。
	Please answer the user's question as accurately as possible.`)

	addUserMessage(chatId, "当前时间是什么时候？")
	addAssistantContentMessage(chatId, "在"+utils.GetdateInfo()+"查询到的时间是："+utils.GetdateInfo()+"。每次询问时间都要重新查询，请确保时间准确。")

	// 持续监听输入channel
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case message := <-inputChan:
			logging.Debug("收到消息: %s", message)
			rp, err := a.HandleChat(ctx, message)
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

	addUserMessage(chatId, message)

	// 调用代理通信
	toolAndContent, err := a.proxy.Communicate(ctx)
	//logging.Info("代理返回信息: %v", toolAndContent)

	if err != nil {
		return "", fmt.Errorf("通信失败: %w", err)
	}

	// 添加空指针检查
	if toolAndContent == nil {
		return "", fmt.Errorf("代理返回空内容")
	}

	a.recordMeomoryFromResponse(ctx, toolAndContent)

	return toolAndContent.Content, nil
}

func (a *Agent) recordMeomoryFromResponse(ctx context.Context, toolAndContent *proxy.ToolAndContent) {
	//fmt.Println("开始记忆返回信息")
	logging.Info("开始记忆返回信息")

	chatId := ctx.Value(utils.ChatIDString).(string)

	if len(toolAndContent.ToolList) > 0 {
		// 工具执行
		logging.Info("开始执行工具:")
		a.executeTools(ctx, toolAndContent)

	}

	// 如果没有工具调用,保存助手响应到内存
	if toolAndContent.Content != "" {
		addAssistantContentMessage(chatId, toolAndContent.Content)
	}
}

func (a *Agent) executeTools(ctx context.Context, toolAndContent *proxy.ToolAndContent) {

	select {
	case <-ctx.Done():
		logging.Info("工具执行过程中，检测到上下文已取消，停止工具执行")
		return
	default:
		// 执行工具逻辑
	}

	chatId := ctx.Value(utils.ChatIDString).(string)

	dialogOutChan := utils.OutputChan
	if dpOutchan, ok := ctx.Value(utils.DPDialogOutputChanString).(chan string); ok {
		//logging.Info("使用Dispatcher的输出通道")
		dialogOutChan = dpOutchan
	}

	// 保存模型要调用的工具信息记忆
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
	addAssistantToolCallsMessage(chatId, toolCalls)

	for _, tool := range toolAndContent.ToolList {
		toolname := tool.Function.Name
		var outStr string

		functl, flag := tools.Getregistry().Get(toolname)
		if !flag {
			outStr = fmt.Sprintf("工具%s不存在", toolname)
			addToolMessage(chatId, tool.ID, outStr)
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
			addToolMessage(chatId, tool.ID, outStr)
			dialogOutChan <- outStr + "\n"

			continue
		}

		outStr = fmt.Sprintf("工具%s执行成功: %s", toolname, str)
		logging.Info("%s", outStr)
		addToolMessage(chatId, tool.ID, outStr)
		dialogOutChan <- outStr + "\n"
	}

	if a.taskLoopTimes >= 0 {
		logging.Info("工具执行完成,继续请求模型生成最终回复")
		a.taskLoopTimes--
		a.HandleChat(ctx, "工具已经执行完成,请继续。如果需要调用工具，请继续调用。如果不需要调用工具了，请直接给出最终回复。")
	}
	logging.Info("工具执行完成,或者达到最大循环次数,结束工具执行")

}

func addToolMessage(chatId, toolid string, toolMessage string) {
	if utils.IsBlank(toolMessage) && utils.IsBlank(toolid) {
		return
	}
	memoryLocal := memory.GetLocalMemory()
	toolResultMsg := memory.Message{
		Role:       memory.MessageRoleTool,
		ToolCallID: toolid,
		Content:    toolMessage,
	}
	memoryLocal.AddMessage(chatId, &toolResultMsg)
}

func addUserMessage(chatId, userMessage string) {
	if utils.IsBlank(userMessage) {
		return
	}
	memoryLocal := memory.GetLocalMemory()
	userMsg := memory.Message{
		Role:    memory.MessageRoleUser,
		Content: userMessage,
	}
	memoryLocal.AddMessage(chatId, &userMsg)
}

func addAssistantToolCallsMessage(chatId string, toolCalls []memory.ToolCall) {
	if len(toolCalls) == 0 {
		return
	}
	memoryLocal := memory.GetLocalMemory()
	assistantMsg := memory.Message{
		Role:      memory.MessageRoleAssistant,
		ToolCalls: toolCalls,
	}
	memoryLocal.AddMessage(chatId, &assistantMsg)
}

func addAssistantContentMessage(chatId string, assistantMessage string) {
	if utils.IsBlank(assistantMessage) {
		return
	}
	memoryLocal := memory.GetLocalMemory()
	assistantMsg := memory.Message{
		Role:    memory.MessageRoleAssistant,
		Content: assistantMessage,
	}
	memoryLocal.AddMessage(chatId, &assistantMsg)
}
