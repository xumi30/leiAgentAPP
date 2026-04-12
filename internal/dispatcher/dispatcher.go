package dispatcher

import (
	"context"
	"fmt"
	"leiAgent/internal/agent"
	"leiAgent/internal/globalchannel"
	"leiAgent/internal/memory"
	"leiAgent/internal/memory/memoryagent"
	"leiAgent/internal/planner"
	"leiAgent/internal/proxy"
	"leiAgent/internal/tools"
	"leiAgent/internal/tools/bashfunction"

	fileFunctions "leiAgent/internal/tools/fileFunction"
	"leiAgent/internal/tools/libraryfs"
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

func init() {
	toolRegistry := tools.Getregistry()

	getTime := timeFunctions.NewTimeTool()
	calculateTimeTool := timeFunctions.NewCalculateTimeTool()
	getWheatherTool := searchFunctions.NewWeatherTool()
	getLongitude := searchFunctions.NewGeocodingTool()
	financeMarket := searchFunctions.NewMarketTool()
	getcurrenttime := timeFunctions.NewCurrentTimeTool()
	wikiSearch := searchFunctions.NewWikipediaSearchTool()
	bashfunction := bashfunction.NewBashTool()
	// browser := browsertool.New()

	toolRegistry.Register(bashfunction)
	// toolRegistry.Register(browser)

	toolRegistry.Register(fileFunctions.GetWriteFileChunk())
	toolRegistry.Register(fileFunctions.GetFileWriteTool())
	toolRegistry.Register(libraryfs.New())
	// toolRegistry.Register(memotool.NewMemoWriteTool())
	toolRegistry.Register(noveltool.New())
	toolRegistry.Register(getcurrenttime)
	toolRegistry.Register(financeMarket)
	toolRegistry.Register(getLongitude)
	toolRegistry.Register(getWheatherTool)
	toolRegistry.Register(wikiSearch)
	toolRegistry.Register(getTime)
	toolRegistry.Register(calculateTimeTool)

	memory.SetAutoCompressHook(func(ctx context.Context, chatID string) {
		if _, err := memoryagent.Compress(ctx, chatID); err != nil {
			logging.Error("自动记忆压缩失败: %v", err)
		} else {
			logging.Info("自动记忆压缩完成 chatID=%s", chatID)
		}
	})
}

// LoadLocalMemorySnapshotForChat 切换会话时从 localmemory/{chatID}.yaml 按顺序载入到 memory.GetLocalMemory()。
func LoadLocalMemorySnapshotForChat(chatID string) {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return
	}
	if err := memory.LoadLocalMemoryFromYAMLFile(cid); err != nil {
		logging.Error("LoadLocalMemoryFromYAMLFile(%s): %v", cid, err)
	}
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

	globalchannel.SendAssitantMessageOnce(d.ctx, fmt.Sprintf("%s", "终止任务运行..."))
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
	chatIDForPersist, _ := ctx.Value(utils.ChatIDString).(string)
	defer func() {
		cid := strings.TrimSpace(chatIDForPersist)
		if cid == "" {
			return
		}
		if err := memory.PersistLocalMemoryToYAMLFile(cid); err != nil {
			logging.Error("本轮对话后写入本地记忆 YAML 失败 chatID=%s: %v", cid, err)
		}
	}()

	logging.Info("Dispatcher 处理消息: %s", message)
	memory.AddUserMessage(chatIDForPersist, fmt.Sprintf("用户请求: %s", message))
	intent, err := ConfirmIntention(ctx, message)
	if err != nil {
		globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", "确认意图失败..."))
		return // 确认意图失败，直接返回
	}
	d.Intention = intent

	chatId := ctx.Value(utils.ChatIDString).(string)
	memory.GetLocalMemory().ClearSystemPrompt(chatId)
	// globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("清除系统提示词: %s", chatId))

	// if d.Intention == nil {
	// 	logging.Info("context 中没有 Intent,重新确认意图...")

	// 	globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", "context 中没有 Intent,重新确认意图..."))
	// 	intent, err := ConfirmIntention(ctx, message)
	// 	if err != nil {
	// 		globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", "确认意图失败..."))
	// 		return // 确认意图失败，直接返回
	// 	}
	// 	d.Intention = intent
	// 	// 确认意图后 清除旧记忆
	// 	chatId := ctx.Value(utils.ChatIDString).(string)
	// 	memory.GetLocalMemory().Clear(chatId)
	// } else if ShouldReclassifyIntent(d.Intention.Intent, message) {
	// 	logging.Info("根据规则判断需要刷新意图，重新确认...")
	// 	intent, err := ConfirmIntention(ctx, message)
	// 	if err != nil {
	// 		globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", "重新确认意图失败，沿用当前模式处理"))
	// 	} else {
	// 		d.Intention = intent
	// 	}
	// }

	// if d.Intention != nil {
	// 	ctx = context.WithValue(ctx, utils.IntentKey, strings.ToUpper(strings.TrimSpace(d.Intention.Intent)))
	// }
	// fmt.Println("意图: ", d.Intention.Intent)
	logging.Info("意图: %s", d.Intention.Intent)

	if d.Intention.Intent != "" {
		d.switchIntent(ctx, &Intention{
			Goal:    d.Intention.Goal,
			Content: d.Intention.Content,
		})
	}

	subIntentions := d.Intention.SubIntents

	for _, subIntent := range subIntentions {
		if subIntent.Content != "" {
			globalchannel.SendAssitantMessageOnce(ctx, subIntent.Content)
			globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("子意图内容: %s 开始处理", subIntent.Content))

		}
		if subIntent.Intent != "" {
			d.switchIntent(ctx, &Intention{
				Goal:    subIntent.Goal,
				Content: subIntent.Content,
			})
		}

	}
}

func (d *Dispatcher) switchIntent(ctx context.Context, intent *Intention) {
	upperIntent := strings.ToUpper(d.Intention.Intent)
	switch upperIntent {
	case utils.ChatModeString:
		logging.Info("切换到对话模式")
		if intent.Content != "" {
			chatId := ctx.Value(utils.ChatIDString).(string)
			// 意图分类不再写入共享记忆，此处补一条 user+assistant，保持与主对话一致
			if g := strings.TrimSpace(intent.Goal); g != "" {
				memory.AddAssistantContentMessage(chatId, fmt.Sprintf("assistant分解的子意图: %s", g))
			}
			globalchannel.SendAssitantMessageOnce(ctx, intent.Content)
			memory.AddAssistantContentMessage(chatId, intent.Content)
		} else {
			logging.Info("对话模式下，意图内容为空，不发送")
			// 如果意图内容为空，则直接把goal用户原文发送给模型，进行对话
			d.handleChat(ctx, intent)
		}

	case utils.PlanModeString:
		logging.Info("切换到规划模式")
		d.handlePlan(ctx, intent)
	case utils.ToolModeString:
		logging.Info("切换到工具模式")
		d.handleTool(ctx, intent)
	case "":
		logging.Info("意图为空，无法处理")
		logging.Error("意图为空，无法处理")

	default:
		globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", "未知意图: "))
		logging.Error("未知意图: %s", d.Intention.Intent)
	}
}

func (d *Dispatcher) handlePlan(ctx context.Context, intent *Intention) {

	userGoal := strings.TrimSpace(intent.Goal)

	if _, ok := ctx.Value(utils.ChatIDString).(string); !ok {
		logging.Error("无法从 context 中获取 chatId")

		globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", "无法从 context 中获取 chatId"))
		return
	}

	ctx = context.WithValue(ctx, utils.IsPlanningString, true)

	globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", "开始进行任务规划...\n"))

	// 单次规划调用：goal 传用户原文。此前此处先 Communicate 再 GeneratePlan，会重复规划且第二次仍用包装后的长串污染 goal。
	pInst, err := planner.GeneratePlan(ctx, userGoal, string(toolsInfo()))
	if err != nil {
		logging.Error("生成计划失败: %v", err)
		globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("生成计划失败: %v", err))
		return
	}
	if pInst == nil {
		globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", "生成计划失败，请确认任务意图是否正确"))
		return
	}
	globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", "规划完成，开始执行..."))

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
	memory.AddAssistantContentMessage(chatId, fmt.Sprintf("assistant分解的子意图: %s", message))
	tc, err := p.Communicate(ctx)
	if err != nil {
		logging.Error("CHAT 模式通信失败: %v", err)
		return
	}
	if tc != nil && strings.TrimSpace(tc.Content) != "" {
		memory.AddAssistantContentMessage(chatId, tc.Content)
	}
}

func (d *Dispatcher) handleTool(ctx context.Context, intent *Intention) {
	ctx = context.WithValue(ctx, utils.ToolTopicToLoad, false)
	ctx = context.WithValue(ctx, utils.ToolsString, true)
	message := intent.Goal
	chatId := ctx.Value(utils.ChatIDString).(string)
	memory.SetSystemPrompt(chatId, utils.SingleToolPromptTemplate)
	logging.Info("tool对话系统提示词已加载...")
	d.agent.HandleChat(ctx, message)
}

func toolsInfo() []byte {
	toolRegistry := tools.Getregistry()

	js, err := toolRegistry.ConvertToolsToJSON()
	if err != nil {
		logging.Error("ConvertToolsToJSON failed: %v", err)
		return nil
	}
	return js
}

func getToolsSimpleInfo() []byte {
	toolRegistry := tools.Getregistry()
	js, err := toolRegistry.GetToolsSimpleInfo()
	if err != nil {
		logging.Error("GetToolsSimpleInfo failed: %v", err)
		return nil
	}
	return js
}
