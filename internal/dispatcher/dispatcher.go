package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	globalchannel "leiAgent/internal"
	"leiAgent/internal/agent"
	"leiAgent/internal/memory"
	"leiAgent/internal/planner"
	"leiAgent/internal/proxy"
	"leiAgent/internal/tools"
	"leiAgent/internal/tools/bashfunction"
	fileFunctions "leiAgent/internal/tools/fileFunction"
	searchFunctions "leiAgent/internal/tools/searchFuctions"
	"leiAgent/internal/tools/timeFunctions"
	"leiAgent/logging"
	"leiAgent/utils"
	"runtime"
	"strings"
)

type Dispatcher struct {
	ctx       context.Context
	cancel    context.CancelFunc
	agent     *agent.Agent
	Intention *Intention
	ChatID    string

	// 移除 planner 字段，统一使用 agent
}

func NewDispatcher(ctx context.Context, chatID string, cancel context.CancelFunc) *Dispatcher {

	globalchannel.RegisterGlobalInputChannel(chatID)
	globalchannel.RegisterGlobalDialogOutChannel(chatID)
	globalchannel.RegisterGlobalReasonOutChannel(chatID)

	return &Dispatcher{
		ctx:    ctx,
		cancel: cancel,
		agent:  agent.NewAgent(agent.WithCtx(ctx)),
		ChatID: chatID,
	}
}

func (d *Dispatcher) Run(ctx context.Context) {
	logging.Info("Dispatcher 开始运行...")
	inputchannel := globalchannel.GetGlobalInputChannel(d.ChatID)
	fmt.Println(inputchannel)
	for {
		select {
		case <-ctx.Done():
			return
		case message := <-globalchannel.GetGlobalInputChannel(d.ChatID):
			logging.Info("Dispatcher 收到消息: %s", message)
			// 统一处理，通过提示词控制行为
			go d.handleMessage(ctx, message)
		}
	}
}

func (d *Dispatcher) Shutdown() {
	if d.cancel != nil {
		d.cancel()
	}
	globalchannel.GetGlobalDialogOutChannel(d.ChatID) <- "终止任务运行..."
}

func (d *Dispatcher) handleMessage(ctx context.Context, message string) {
	dialogOutputChan := globalchannel.GetGlobalDialogOutChannel(d.ChatID)

	logging.Info("Dispatcher 处理消息: %s", message)

	if d.Intention == nil {
		logging.Info("context 中没有 Intent,重新确认意图...")
		dialogOutputChan <- "context 中没有 Intent,重新确认意图..."
		intent, err := ConfirmIntention(ctx, message)
		if err != nil {
			dialogOutputChan <- fmt.Sprintf("确认意图失败: %v", err)
			return // 确认意图失败，直接返回
		}
		d.Intention = intent
		ctx = context.WithValue(ctx, utils.IntentKey, d.Intention.Intent)
		// 确认意图后 清除旧记忆
		chatId := ctx.Value(utils.ChatIDString).(string)
		memory.GetLocalMemory().Clear(chatId)
	}

	// 获取运行机器的系统类型是windosw还是linux
	// 获取运行机器的系统类型
	systemType := runtime.GOOS
	logging.Info("运行机器的系统类型: %s", systemType)

	memory.AddUserMessage(ctx.Value(utils.ChatIDString).(string), fmt.Sprintf("运行的系统类型是: %s", systemType))

	d.Intention.Goal = message
	fmt.Println("意图: ", d.Intention.Intent)
	upperIntent := strings.ToUpper(d.Intention.Intent)
	switch upperIntent {
	case utils.SwitchModeString:
		d.Intention = nil
		dialogOutputChan <- "已重置意图，请继续输入新的消息"
	case utils.ChatModeString:
		logging.Info("切换到聊天模式")
		d.handleChat(ctx, d.Intention)
	case utils.PlanModeString:
		logging.Info("切换到规划模式")
		d.handlePlan(ctx, d.Intention)
	case utils.ToolModeString:
		logging.Info("切换到工具模式")
		d.handleTool(ctx, d.Intention)
	case "":
		logging.Info("意图为空，无法处理")
		logging.Error("意图为空，无法处理")
		return
	default:
		dialogOutputChan <- "未知意图: " + d.Intention.Intent
		logging.Error("未知意图: %s", d.Intention.Intent)
	}

}

func (d *Dispatcher) handlePlan(ctx context.Context, intent *Intention) {
	dialogOutputChan := globalchannel.GetGlobalDialogOutChannel(d.ChatID)
	message := intent.Goal

	chatId, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		logging.Error("无法从 context 中获取 chatId")
		dialogOutputChan <- "无法从 context 中获取 chatId"
		return
	}

	ctx = context.WithValue(ctx, utils.IsPlanningString, true)

	dialogOutputChan <- "开始进行任务规划..."
	// 根据命令行参数设置不同的提示词

	// 如果是规划模式,添加工具信息
	if ctx.Value(utils.IsPlanningString).(bool) {
		js := toolsInfo()
		dialogOutputChan <- "正在加载工具信息...\n"
		dialogOutputChan <- utils.FinishString
		planInput := struct {
			Message string      `json:"message"`
			Goal    string      `json:"goal"`
			Tools   interface{} `json:"TOOL_LIST"`
		}{
			Message: message,
			Goal:    message,
			Tools:   json.RawMessage(js),
		}
		planJSON, err := json.MarshalIndent(planInput, "", "  ")
		if err != nil {
			logging.Error("序列化规划输入失败: %v", err)
			return
		}
		message = "这是你的任务,你理解message内容,进行你的任务规划。" +
			"如果对任务意图有什么不明确,可以询问用户,直到你能明确任务目标,作为goal字段.然后开始你的规划：" + string(planJSON)
	}

	p := proxy.NewProxy(nil)
	memory.GetLocalMemory().SetSystemPrompt(chatId, planner.PlannerPromotion)
	logging.Info("planning系统提示词已加载...")

	memory.AddUserMessage(chatId, message)

	response, err := p.Communicate(ctx)
	if err != nil {
		logging.Error("Response: %s", response.Content)
		dialogOutputChan <- "规划失败，请确认任务意图是否正确"
	}

	logging.Info("Agent 处理消息完成，返回结果: %s", response.Content)

	if err != nil {
		logging.Error("处理消息失败: %v", err)
	}

	// 计划 执行 校验 重试
	planner, err := planner.GeneratePlan(ctx, message, string(toolsInfo()))
	if err != nil {
		logging.Error("生成计划失败: %v", err)
		return
	}
	if planner == nil {
		dialogOutputChan <- "生成计划失败，请确认任务意图是否正确"
		return
	}

	planner.DoTask(ctx)

}

func (d *Dispatcher) handleChat(ctx context.Context, intent *Intention) {
	message := intent.Goal
	chatId := ctx.Value(utils.ChatIDString).(string)
	memory.SetSystemPrompt(chatId, utils.ChatPromptTemplate)
	logging.Info("对话系统提示词已加载...")
	p := proxy.NewProxy(nil)
	memory.AddUserMessage(chatId, message)
	p.Communicate(ctx)
}

func (d *Dispatcher) handleTool(ctx context.Context, intent *Intention) {
	message := intent.Goal
	chatId := ctx.Value(utils.ChatIDString).(string)
	memory.SetSystemPrompt(chatId, utils.ChatPromptTemplate)
	logging.Info("对话系统提示词已加载...")
	d.agent.HandleChat(message)
}

func toolsInfo() []byte {
	toolRegistry := tools.Getregistry()
	getTime := timeFunctions.NewTimeTool()

	calculateTimeTool := timeFunctions.NewCalculateTimeTool()
	getWheatherTool := searchFunctions.NewWeatherTool()
	getLongitude := searchFunctions.NewGeocodingTool()
	financeMarket := searchFunctions.NewMarketTool()
	getcurrenttime := timeFunctions.NewCurrentTimeTool()
	bashfunction := bashfunction.NewBashTool()
	toolRegistry.Register(bashfunction)

	toolRegistry.Register(fileFunctions.GetWriteFileChunk())
	toolRegistry.Register(getcurrenttime)
	toolRegistry.Register(financeMarket)
	toolRegistry.Register(getLongitude)
	toolRegistry.Register(getWheatherTool)
	toolRegistry.Register(getTime)
	toolRegistry.Register(calculateTimeTool)

	js, err := toolRegistry.ConvertToolsToJSON()
	if err != nil {
		logging.Error("ConvertToolsToJSON failed: %v", err)
		return nil
	}
	return js
}
