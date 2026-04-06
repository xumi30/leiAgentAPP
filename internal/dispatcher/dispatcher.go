package dispatcher

import (
	"context"
	"fmt"
	"leiAgent/internal/globalchannel"
	"leiAgent/internal/agent"
	"leiAgent/internal/memory"
	"leiAgent/internal/planner"
	"leiAgent/internal/proxy"
	"leiAgent/internal/tools"
	"leiAgent/internal/tools/bashfunction"
	fileFunctions "leiAgent/internal/tools/fileFunction"
	"leiAgent/internal/tools/libraryfs"
	"leiAgent/internal/tools/memotool"
	"leiAgent/internal/tools/noveltool"
	searchFunctions "leiAgent/internal/tools/searchFuctions"
	"leiAgent/internal/tools/timeFunctions"
	"leiAgent/logging"
	"leiAgent/utils"
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

func NewDispatcher(ctx context.Context, chatID string, cancel context.CancelFunc) (*Dispatcher, error) {
	if strings.TrimSpace(chatID) == "" {
		return nil, fmt.Errorf("NewDispatcher: chatID 不能为空（空 id 会与全局 OutputChan 混用，导致误把模型输出当成用户输入）")
	}

	globalchannel.RegisterGlobalInputChannel(chatID)
	globalchannel.RegisterGlobalDialogOutChannel(chatID)
	globalchannel.RegisterGlobalReasonOutChannel(chatID)

	ag, err := agent.NewAgent(agent.WithCtx(ctx))
	if err != nil {
		return nil, err
	}

	return &Dispatcher{
		ctx:    ctx,
		cancel: cancel,
		agent:  ag,
		ChatID: chatID,
	}, nil
}

func (d *Dispatcher) Run(ctx context.Context) {
	logging.Info("Dispatcher 开始运行...")
	inputchannel := globalchannel.GetGlobalInputChannel(d.ChatID)
	fmt.Println(inputchannel)
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-globalchannel.GetGlobalInputChannel(d.ChatID):
			if msg == nil || msg.Content == "" {
				continue
			}
			logging.Info("Dispatcher 收到消息: %s", msg.Content)
			// 统一处理，通过提示词控制行为
			go d.handleMessage(ctx, msg.Content)
		}
	}
}

func (d *Dispatcher) Shutdown() {
	if d.cancel != nil {
		d.cancel()
	}
	globalchannel.GetGlobalDialogOutChannel(d.ChatID) <- &globalchannel.Message{Content: "终止任务运行...", Role: utils.MessageRoleAssistant, IsFinished: false}
}

// ReplaceRunContext 在 Shutdown 之后使用：保留同一 Dispatcher（含 Intention），仅换新可取消的 context 并重新 Run。
func (d *Dispatcher) ReplaceRunContext(ctx context.Context, cancel context.CancelFunc) {
	d.ctx = ctx
	d.cancel = cancel
	if d.agent != nil {
		d.agent.SetCtx(ctx)
	}
}

func (d *Dispatcher) handleMessage(ctx context.Context, message string) {
	dialogOutputChan := globalchannel.GetGlobalDialogOutChannel(d.ChatID)

	logging.Info("Dispatcher 处理消息: %s", message)

	if d.Intention == nil {
		logging.Info("context 中没有 Intent,重新确认意图...")
		dialogOutputChan <- &globalchannel.Message{Content: "context 中没有 Intent,重新确认意图...", Role: utils.MessageRoleAssistant, IsFinished: false}
		intent, err := ConfirmIntention(ctx, message)
		if err != nil {
			dialogOutputChan <- &globalchannel.Message{Content: fmt.Sprintf("确认意图失败: %v", err), Role: utils.MessageRoleAssistant, IsFinished: false}
			return // 确认意图失败，直接返回
		}
		d.Intention = intent
		// 确认意图后 清除旧记忆
		chatId := ctx.Value(utils.ChatIDString).(string)
		memory.GetLocalMemory().Clear(chatId)
	} else if ShouldReclassifyIntent(d.Intention.Intent, message) {
		logging.Info("根据规则判断需要刷新意图，重新确认...")
		intent, err := ConfirmIntention(ctx, message)
		if err != nil {
			dialogOutputChan <- &globalchannel.Message{Content: fmt.Sprintf("重新确认意图失败: %v，沿用当前模式处理", err), Role: utils.MessageRoleAssistant, IsFinished: false}
		} else {
			d.Intention = intent
		}
	}

	d.Intention.Goal = message
	if d.Intention != nil {
		ctx = context.WithValue(ctx, utils.IntentKey, strings.ToUpper(strings.TrimSpace(d.Intention.Intent)))
	}
	fmt.Println("意图: ", d.Intention.Intent)
	upperIntent := strings.ToUpper(d.Intention.Intent)
	switch upperIntent {
	case utils.SwitchModeString:
		d.Intention = nil
		dialogOutputChan <- &globalchannel.Message{Content: "已重置意图，请继续输入新的消息", Role: utils.MessageRoleAssistant, IsFinished: false}
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
		dialogOutputChan <- &globalchannel.Message{Content: "未知意图: " + d.Intention.Intent, Role: utils.MessageRoleAssistant, IsFinished: false}
		logging.Error("未知意图: %s", d.Intention.Intent)
	}

}

func (d *Dispatcher) handlePlan(ctx context.Context, intent *Intention) {
	dialogOutputChan := globalchannel.GetGlobalDialogOutChannel(d.ChatID)
	userGoal := strings.TrimSpace(intent.Goal)

	if _, ok := ctx.Value(utils.ChatIDString).(string); !ok {
		logging.Error("无法从 context 中获取 chatId")
		dialogOutputChan <- &globalchannel.Message{Content: "无法从 context 中获取 chatId", Role: utils.MessageRoleAssistant, IsFinished: false}
		return
	}

	ctx = context.WithValue(ctx, utils.IsPlanningString, true)

	dialogOutputChan <- &globalchannel.Message{Content: "开始进行任务规划...", Role: utils.MessageRoleAssistant, IsFinished: false}
	dialogOutputChan <- &globalchannel.Message{Content: "正在加载工具信息...\n", Role: utils.MessageRoleAssistant, IsFinished: false}
	dialogOutputChan <- &globalchannel.Message{Content: utils.FinishString, Role: utils.MessageRoleAssistant, IsFinished: true}

	// 单次规划调用：goal 传用户原文。此前此处先 Communicate 再 GeneratePlan，会重复规划且第二次仍用包装后的长串污染 goal。
	pInst, err := planner.GeneratePlan(ctx, userGoal, string(toolsInfo()))
	if err != nil {
		logging.Error("生成计划失败: %v", err)
		dialogOutputChan <- &globalchannel.Message{Content: err.Error(), Role: utils.MessageRoleAssistant, IsFinished: false}
		return
	}
	if pInst == nil {
		dialogOutputChan <- &globalchannel.Message{Content: "生成计划失败，请确认任务意图是否正确", Role: utils.MessageRoleAssistant, IsFinished: false}
		return
	}

	pInst.DoTask(ctx)

}

func (d *Dispatcher) handleChat(ctx context.Context, intent *Intention) {
	message := intent.Goal
	chatId := ctx.Value(utils.ChatIDString).(string)
	memory.SetSystemPrompt(chatId, utils.ChatPromptTemplate)
	logging.Info("对话系统提示词已加载...")
	p, err := proxy.NewProxy(nil)
	if err != nil {
		logging.Error("创建 LLM 代理失败: %v", err)
		return
	}
	memory.AddUserMessage(chatId, message)
	_, _ = p.Communicate(ctx)
}

func (d *Dispatcher) handleTool(ctx context.Context, intent *Intention) {
	message := intent.Goal
	chatId := ctx.Value(utils.ChatIDString).(string)
	memory.SetSystemPrompt(chatId, utils.ChatPromptTemplate)
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
	toolRegistry.Register(fileFunctions.GetFileWriteTool())
	toolRegistry.Register(libraryfs.New())
	toolRegistry.Register(memotool.NewMemoWriteTool())
	toolRegistry.Register(noveltool.New())
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
