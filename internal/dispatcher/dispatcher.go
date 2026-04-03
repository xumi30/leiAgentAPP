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
	d.DialogOutputChan <- "终止任务运行..."
}

// 获取outputChan 输出内容
func (d *Dispatcher) GetOutput() <-chan string {
	return d.DialogOutputChan
}

func (d *Dispatcher) handleMessage(ctx context.Context, message string) {

	ctx = context.WithValue(ctx, utils.DPDialogOutputChanString, d.DialogOutputChan)
	ctx = context.WithValue(ctx, utils.DPReasoningOutputChanString, d.ReasonningOutputChan)

	logging.Info("Dispatcher 处理消息: %s", message)

	if d.Intention == nil {
		logging.Info("context 中没有 Intent,重新确认意图...")
		d.DialogOutputChan <- "context 中没有 Intent,重新确认意图..."
		intent, err := ConfirmIntention(ctx, message)
		if err != nil {
			d.DialogOutputChan <- fmt.Sprintf("确认意图失败: %v", err)
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
		d.DialogOutputChan <- "正在加载工具信息...\n"
		d.DialogOutputChan <- utils.FinishString
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
		d.DialogOutputChan <- "规划失败，请确认任务意图是否正确"
	}

	logging.Info("Agent 处理消息完成，返回结果: %s", response.Content)

	if err != nil {
		logging.Error("处理消息失败: %v", err)
	}

	// 计划 执行 校验 重试
	planner := planner.NewPlanner(intent.Goal)
	pstr, err := planner.DoExe(ctx, response.Content)
	fmt.Println("初始执行规划结果: ", pstr, planner.Status, planner.RetryCount)

	if err != nil {
		logging.Error("执行规划失败: %v", err)
		memory.AddUserMessage(chatId, "执行规划失败，返回的错误是："+err.Error())
	}

	for planner.Status == "failed" && planner.RetryCount > 0 {
		fmt.Printf("执行规划失败，正在进行第%d次重试...\n", planner.RetryCount)
		planner.RetryCount--
		retryResult, err := planner.VerifyResult(ctx, pstr)
		if err != nil {
			logging.Error("Failed to retry verify result: %v", err)
			return
		}
		pstr, err = planner.DoExe(ctx, retryResult)

		if err != nil {
			logging.Error("执行规划失败倒数第%d次: %v", planner.RetryCount, err)
			d.DialogOutputChan <- fmt.Sprintf("执行规划失败倒数第%d次: %v", planner.RetryCount, err)
			memory.AddUserMessage(chatId, fmt.Sprintf("执行规划失败倒数第%d次: %v", planner.RetryCount, err))
		}

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
