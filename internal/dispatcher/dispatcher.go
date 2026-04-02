package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/internal/agent"
	"leiAgent/internal/memory"
	"leiAgent/internal/planner"
	"leiAgent/internal/proxy"
	"leiAgent/internal/tools"
	fileFunctions "leiAgent/internal/tools/fileFunction"
	searchFunctions "leiAgent/internal/tools/searchFuctions"
	"leiAgent/internal/tools/timeFunctions"
	"leiAgent/logging"
	"leiAgent/utils"
	"strings"
)

type Dispatcher struct {
	ctx                  context.Context
	cancel               context.CancelFunc
	agent                *agent.Agent
	InputChan            chan string
	DialogOutputChan     chan string
	ReasonningOutputChan chan string
	Intention            *Intention

	// 移除 planner 字段，统一使用 agent
}

func NewDispatcher(ctx context.Context, cancel context.CancelFunc) *Dispatcher {
	return &Dispatcher{
		ctx:                  ctx,
		cancel:               cancel,
		agent:                agent.NewAgent(),
		InputChan:            make(chan string, 100),
		DialogOutputChan:     make(chan string, 100),
		ReasonningOutputChan: make(chan string, 100),
	}
}

func (d *Dispatcher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case message := <-d.InputChan:
			logging.Debug("Dispatcher 收到消息: %s", message)
			// 统一处理，通过提示词控制行为
			go d.handleMessage(ctx, message)
		}
	}
}

func (d *Dispatcher) Shutdown() {
	if d.cancel != nil {
		d.cancel()
	}
}

// 获取outputChan 输出内容
func (d *Dispatcher) GetOutput() <-chan string {
	return d.DialogOutputChan
}

func (d *Dispatcher) handleMessage(ctx context.Context, message string) {

	ctx = context.WithValue(ctx, utils.DPDialogOutputChanString, &d.DialogOutputChan)
	ctx = context.WithValue(ctx, utils.DPReasoningOutputChanString, &d.ReasonningOutputChan)

	logging.Info("Dispatcher 处理消息: %s", message)

	
	if d.Intention == nil {
		logging.Info("context 中没有 Intent,重新确认意图...")
		d.DialogOutputChan <- "context 中没有 Intent,重新确认意图..."
		d.Intention = ConfirmIntention(ctx, message)
		ctx = context.WithValue(ctx, utils.IntentKey, d.Intention.Intent)
		// 确认意图后 清除旧记忆
		chatId := ctx.Value(utils.ChatIDString).(string)
		memory.GetLocalMemory().Clear(chatId)
	}

	d.Intention.Goal = message
	fmt.Println("意图: ", d.Intention.Intent)
	upperIntent := strings.ToUpper(d.Intention.Intent)
	switch upperIntent {
	case utils.SwitchModeString:
		d.Intention = nil
		d.DialogOutputChan <- "已重置意图，请继续输入新的消息"
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
		d.DialogOutputChan <- "未知意图: " + d.Intention.Intent
		logging.Error("未知意图: %s", d.Intention.Intent)
	}

}

func (d *Dispatcher) handlePlan(ctx context.Context, intent *Intention) {

	message := intent.Goal

	chatId, ok := ctx.Value(utils.ChatIDString).(string)
	if !ok {
		logging.Error("无法从 context 中获取 chatId")
		d.DialogOutputChan <- "无法从 context 中获取 chatId"
		return
	}

	ctx = context.WithValue(ctx, utils.IsPlanningString, true)

	d.DialogOutputChan <- "开始进行任务规划..."
	// 根据命令行参数设置不同的提示词

	// 如果是规划模式,添加工具信息
	if ctx.Value(utils.IsPlanningString).(bool) {
		js := toolsInfo()
		planInput := struct {
			Message string      `json:"message"`
			Goal    string      `json:"goal"`
			Tools   interface{} `json:"TOOL_LIST"`
		}{
			Message: message,
			Goal:    "",
			Tools:   json.RawMessage(js),
		}
		planJSON, err := json.MarshalIndent(planInput, "", "  ")
		if err != nil {
			logging.Error("序列化规划输入失败: %v", err)
			return
		}
		message = "这是你的任务模版,你先理解message内容,如过是任务意图,就把它填充到goal字段,进行你的任务规划。" +
			"如果不是任务意图,请继续询问用户意图,直到你能明确任务目标,能填充goal字段.如果还需要其他信息可以继续提问，最后开始你的规划：" + string(planJSON)
	}

	p := proxy.NewProxy(nil)
	memory.GetLocalMemory().SetSystemPrompt(chatId, planner.PlannerPromotion)
	logging.Info("planning系统提示词已加载...")

	memory.AddUserMessage(chatId, message)

	response, err := p.Communicate(ctx)
	if err != nil {
		logging.Error("Response: %s", response.Content)
		d.DialogOutputChan <- "规划失败，请确认任务意图是否正确"
	}

	logging.Info("Agent 处理消息完成，返回结果: %s", response.Content)

	if err != nil {
		logging.Error("处理消息失败: %v", err)
	}

	pstr, err := planner.DoExe(ctx, extractJSON(response.Content))
	if err != nil {
		logging.Error("执行规划失败: %v", err)
		memory.AddUserMessage(chatId, "执行规划失败，返回的错误是："+err.Error())
		response, err = p.Communicate(ctx)
	}

	memory.AddUserMessage(chatId, "全部规划执行完成，以下是执行结果："+pstr)
	d.DialogOutputChan <- "规划执行完成，以下是执行结果：" + pstr

	memory.AddUserMessage(chatId, "任务规划完成，执行步骤结果是："+pstr+".集合以上信息，请给我任务的最终结果。或者是否需要继续执行其他步骤？")
	response, err = p.Communicate(ctx)

}

func (d *Dispatcher) handleChat(ctx context.Context, intent *Intention) {
	message := intent.Goal
	chatId := ctx.Value(utils.ChatIDString).(string)
	memory.AddSetSystemPrompt(chatId, utils.ChatPromptTemplate)
	logging.Info("对话系统提示词已加载...")
	p := proxy.NewProxy(nil)
	memory.AddUserMessage(chatId, message)
	p.Communicate(ctx)
}

func (d *Dispatcher) handleTool(ctx context.Context, intent *Intention) {
	message := intent.Goal
	chatId := ctx.Value(utils.ChatIDString).(string)
	memory.AddSetSystemPrompt(chatId, utils.ChatPromptTemplate)
	logging.Info("对话系统提示词已加载...")
	d.agent.HandleChat(ctx, message)
}

func toolsInfo() []byte {
	toolRegistry := tools.Getregistry()
	getTime := timeFunctions.NewTimeTool()
	calculateTimeTool := timeFunctions.NewCalculateTimeTool()
	getWheatherTool := searchFunctions.NewWeatherTool()
	getLongitude := searchFunctions.NewGeocodingTool()

	toolRegistry.Register(fileFunctions.GetWriteFileChunk())
	financeMarket := searchFunctions.NewMarketTool()
	getcurrenttime := timeFunctions.NewCurrentTimeTool()
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
