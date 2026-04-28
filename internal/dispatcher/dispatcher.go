package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/dataoperation"
	mcpbridge "leiAgent/internal/MCP"
	"leiAgent/internal/agent"
	"leiAgent/internal/globalchannel"
	"leiAgent/internal/memory"
	"leiAgent/internal/memory/compressor"
	"leiAgent/internal/memory/compressstore"
	"leiAgent/internal/planner"
	"leiAgent/internal/provider/openaistyle"
	"leiAgent/internal/proxy"
	"leiAgent/internal/tools"
	"leiAgent/internal/tools/bashfunction"
	"leiAgent/internal/tools/crontab"
	"leiAgent/internal/tools/mcptool"
	"math/rand"
	"sync"
	"time"

	fileFunctions "leiAgent/internal/tools/fileFunction"
	"leiAgent/internal/tools/libraryfs"
	"leiAgent/internal/tools/noveltool"
	"leiAgent/internal/tools/openclawtool"
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
	agentSpeakHistory []string // 记录agent发言顺序，用于下一轮排序
	// 移除 planner 字段，统一使用 agent
	isPlanning bool // 是否正在规划
	Stop       bool // 是否停止
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
	downloadBooks := searchFunctions.NewDownloadBooksTool()
	bashfunction := bashfunction.NewBashTool()

	listMCPTools := mcptool.NewListMCPTools(nil)
	callMCPTool := mcptool.NewCallMCPTool(nil)
	registerMCPFromHub := mcptool.NewRegisterMCPFromHub()
	openClawBaiduSearch := openclawtool.NewBaiduSearchTool()
	installOpenClawSkillFromMarket := openclawtool.NewInstallOpenClawSkillFromMarket(nil)

	toolRegistry.Register(bashfunction)
	toolRegistry.Register(listMCPTools)
	toolRegistry.Register(callMCPTool)
	toolRegistry.Register(registerMCPFromHub)
	toolRegistry.Register(openClawBaiduSearch)
	toolRegistry.Register(installOpenClawSkillFromMarket)

	toolRegistry.Register(fileFunctions.GetWriteFileChunk())
	toolRegistry.Register(fileFunctions.GetFileWriteTool())
	toolRegistry.Register(fileFunctions.NewFileDownloadTool())
	toolRegistry.Register(libraryfs.New())
	// toolRegistry.Register(memotool.NewMemoWriteTool())
	toolRegistry.Register(noveltool.New())
	toolRegistry.Register(getcurrenttime)
	toolRegistry.Register(financeMarket)
	toolRegistry.Register(getLongitude)
	toolRegistry.Register(getWheatherTool)
	toolRegistry.Register(wikiSearch)
	toolRegistry.Register(downloadBooks)
	toolRegistry.Register(getTime)
	toolRegistry.Register(calculateTimeTool)
	toolRegistry.Register(crontab.NewCreateScheduledTaskTool())
	toolRegistry.Register(crontab.NewUpdateScheduledTaskTool())
	toolRegistry.Register(crontab.NewDeleteScheduledTaskTool())
	toolRegistry.Register(crontab.NewListScheduledTasksTool())

	// Best-effort: load memory compression config and sync trigger thresholds.
	// Keep defaults if config is missing/invalid.
	if cfg, err := proxy.LoadMemoryCompressionConfig(); err == nil {
		memory.SetAutoCompressEveryAssistantTurns(cfg.Trigger.EveryAssistantTurns)
		memory.SetAutoCompressYAMLMessageThreshold(cfg.Trigger.YAMLMessageThreshold)
	}

	memory.SetAutoCompressHook(func(ctx context.Context, chatID string) {
		cfg, err := proxy.LoadMemoryCompressionConfig()
		if err != nil {
			logging.Error("读取记忆压缩配置失败: %v", err)
			return
		}
		if !cfg.Enabled {
			return
		}

		cid := strings.TrimSpace(chatID)
		if cid == "" {
			return
		}
		raw := memory.GetLocalMemory().GetMessages(cid)
		artifact := compressor.CompressRulesOnly(raw, compressor.Options{
			ChatID:             cid,
			RecentTailMessages: cfg.Context.RecentTailMessages,
			SystemCardPrefix:   cfg.Context.SystemCardPrefix,
			TLDRSentences:      cfg.Outputs.TLDRSentences,
			BulletMax:          cfg.Outputs.BulletMax,
		})
		if _, err := compressstore.PersistYAML(cfg.PersistDir, cid, &artifact); err != nil {
			logging.Error("写入 compress 记忆失败 chatID=%s: %v", cid, err)
			return
		}
		// Best-effort cleanup: sweep localmemory/*.yaml and remove empty snapshots.
		if err := memory.CleanupEmptyLocalMemoryYAMLDir(); err != nil {
			logging.Warn("清理空 localmemory YAML 目录失败: %v", err)
		}
		logging.Info("自动记忆压缩完成 chatID=%s", cid)
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
	globalchannel.RegisterGlobalTaskStateChannel(chatID)

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

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-globalchannel.GetGlobalInputChannel(d.ChatID):
			if msg == nil || msg.Content == "" {
				continue
			}
			logging.Info("Dispatcher 收到消息: %s", msg.Content)
			// 同一 chatID 下严格串行处理，避免共享 agent / memory / intention 并发踩踏。
			globalchannel.SendTaskState(ctx, true)
			memory.AddUserMessage(d.ChatID, fmt.Sprintf("User:%s", msg.Content))
			d.Stop = false
			d.handleMessage(ctx, msg)
			globalchannel.SendTaskState(ctx, false)
		}
	}
}

func (d *Dispatcher) Shutdown() {
	if d.cancel != nil {
		d.cancel()
	}

	//globalchannel.SendAssitantMessageOnce(d.ctx, fmt.Sprintf("%s", "终止任务运行..."))
}

// ReplaceRunContext 在 Shutdown 之后使用：保留同一 Dispatcher（含 Intention），仅换新可取消的 context 并重新 Run。
func (d *Dispatcher) ReplaceRunContext(ctx context.Context, cancel context.CancelFunc) {
	d.ctx = ctx
	d.cancel = cancel
	if d.agent != nil {
		d.agent.SetCtx(ctx)
	}
}

func (d *Dispatcher) handleMessage(ctx context.Context, msg *globalchannel.Message) {

	message := msg.Content
	userToAgentList := msg.UserToAgentList
	if len(userToAgentList) > 0 {
		go chatWithAitessistant(ctx, message, userToAgentList)
		return
	}

	chatIDForPersist, _ := ctx.Value(utils.ChatIDString).(string)
	logging.Info("Dispatcher 处理消息: %s", message)

	round := 0
	for msg.IsAutoToTalk {
		if d.Stop {
			return
		}
		if message != "继续对话controller" {
			go d.processMessageWithIntent(ctx, message)
			// 先等待一轮对话，再继续自动对话
			time.Sleep(time.Duration(3+rand.Intn(4)) * time.Second)
			if d.isPlanning {
				time.Sleep(time.Duration(15+rand.Intn(10)) * time.Second)
				d.isPlanning = false
			}
			time.Sleep(time.Duration(1+rand.Intn(4)) * time.Second)

		}
		// 根据chatid获取agent列表
		logging.Info("自动对话模式，获取对话agent列表")
		if agentList, err := dataoperation.ListAgentsInConversation(chatIDForPersist); err != nil {
			logging.Error("获取对话agent列表失败: %v", err)
			return
		} else if len(agentList) > 0 {
			// 随机选择最多3个agent
			selectedAgentIDs := d.selectRandomAgents(agentList, 3)

			// 随机顺序调用handleAgentChat
			for _, agentid := range selectedAgentIDs {
				if d.Stop {
					return
				}
				round += 1

				handleAgentChat(ctx, message, agentid)
				logging.Info("处理agent聊天: %s", agentid)

				time.Sleep(time.Duration(4+rand.Intn(3)) * time.Second)

				// 每4轮验证一次话题偏移
				if round%4 == 0 {
					verifyGoal(ctx, msg.Content)
				}
			}
		}
		time.Sleep(time.Duration(1+rand.Intn(5)) * time.Second)
		message = "继续对话controller"
	}
	d.processMessageWithIntent(ctx, message)

}

func (d *Dispatcher) handleActionGateChat(ctx context.Context, message string) (needAction bool, handled bool, err error) {
	chatId := ctx.Value(utils.ChatIDString).(string)

	p, err := proxy.NewProxy(nil)
	if err != nil {
		return false, false, err
	}

	ctx = context.WithValue(ctx, utils.NeedActionHeaderString, true)
	tc, err := p.CommunicateWithMessages(ctx, buildActionGateMessages(ctx, message))
	if err != nil {
		return false, false, err
	}
	if tc == nil {
		return false, false, fmt.Errorf("action-gate 返回空响应")
	}
	if strings.TrimSpace(tc.Content) != "" && !tc.NeedAction {
		memory.AddAssistantContentMessage(chatId, tc.Content)
	}
	return tc.NeedAction, true, nil
}

func buildActionGateMessages(ctx context.Context, message string) []openaistyle.ChatMessage {
	messages := []openaistyle.ChatMessage{
		{
			Role:    openaistyle.RoleSystem,
			Content: utils.ActionGateChatPromptTemplate,
		},
	}
	for _, recent := range buildIntentRecentContext(ctx, message) {
		role := strings.TrimSpace(recent.Role)
		if role == "" || role == string(memory.MessageRoleSystem) {
			continue
		}
		messages = append(messages, openaistyle.ChatMessage{
			Role:    role,
			Content: recent.Content,
		})
	}
	messages = append(messages, openaistyle.ChatMessage{
		Role:    openaistyle.RoleUser,
		Content: message,
	})
	return messages
}

func shouldUseActionGate(message string) bool {
	low := strings.ToLower(strings.TrimSpace(message))
	if low == "" {
		return true
	}
	actionHints := []string{
		"几点", "现在时间", "当前时间", "今天几号", "今天星期", "日期", "timezone", "time now", "current time",
		"天气", "气温", "weather", "温度",
		"股价", "行情", "价格", "汇率", "bitcoin", "btc", "market",
		"搜索", "查一下", "查询", "搜一下", "百度", "google", "最新", "新闻",
		"打开网页", "浏览器", "点击", "下载", "写入文件", "保存到", "执行命令", "运行命令", "bash",
		"提醒", "定时", "闹钟", "计划", "规划", "分步", "多步", "帮我实现", "开发一个", "搭建",
		"续写", "写小说", "小说", "章节", "章书", "大纲", "长文", "长篇", "创作", "故事","安装",
		"continue the story", "write a novel", "chapter", "chapters", "outline", "long-form", "story",
	}
	for _, hint := range actionHints {
		if strings.Contains(low, hint) {
			return false
		}
	}
	return true
}

func (d *Dispatcher) switchIntent(ctx context.Context, intent *Intention) {
	upperIntent := strings.ToUpper(strings.TrimSpace(intent.Intent))
	switch upperIntent {
	case utils.ChatModeString:
		logging.Info("切换到对话模式")
		if intent.Content != "" {
			globalchannel.SendAssitantMessageOnce(ctx, intent.Content)
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
	logging.Info("开始进行任务规划...")

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
	if blueprint, ok := ctx.Value(utils.ExecutionBlueprintString).(ExecutionBlueprint); ok {
		if strings.EqualFold(strings.TrimSpace(blueprint.Mode), utils.ToolModeString) {
			ctx = d.prepareToolExecutionContext(ctx, blueprint)
		}
	}
	ctx = context.WithValue(ctx, utils.ToolTopicToLoad, intent.ToolTopic)
	ctx = context.WithValue(ctx, utils.ToolSourceToLoad, intent.ToolSource)
	ctx = context.WithValue(ctx, utils.ToolsString, true)
	message := intent.Goal
	ctx = context.WithValue(ctx, utils.UserGoalString, message)
	chatId := ctx.Value(utils.ChatIDString).(string)
	memory.SetSystemPrompt(chatId, utils.SingleToolPromptTemplate)
	logging.Info("tool对话系统提示词已加载...")
	d.agent.BeginTask(ctx, message)
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

func getMCPSimpleInfo() []byte {
	infos := mcpbridge.GetMCPSimpleInfos()
	js, err := json.MarshalIndent(infos, "", "  ")
	if err != nil {
		logging.Error("GetMCPSimpleInfos failed: %v", err)
		return []byte("[]")
	}
	return js
}

func handleAgentChat(ctx context.Context, message string, agentID string) error {
	info, err := dataoperation.GetAgent(agentID)
	if err != nil || info == nil {
		logging.Warn("aite agent not found: %s: %v", agentID, err)
		return fmt.Errorf("agent not found: %s", agentID)
	}
	systemPrompt := strings.TrimSpace(fmt.Sprintf(`你的名字是 %s。角色设定：%s
	你正在一个多人群聊中。你会看到「xxx: ...」格式的消息，xxx 是发言人。
	发言规则：
	- 先理解目前的话题是什么,优先铆钉话题，不要跑题。特别要关注User@你的名字的消息,必须第一时间回复User。
	- 如果有人 @你，优先专门回复他。
	- 你可以针对某人回复，并使用 @xxx。
	- 你也可以不 @任何人，直接发表自己的观点。
	- 回复要像真实群聊成员，可以赞同、反对、补充、提问、自由发挥。
	- 回复不要带上你自己的名字。
	- 回复要简洁，不要冗长。像个真实的人一样聊天。
	- 今天日期：%s`,
		info["agent_name"],
		info["description"],
		time.Now().Format("2006-01-02 Monday"),
	))
	if systemPrompt == "" {
		return fmt.Errorf("agent %s has no description", agentID)
	}
	p, err := proxy.NewProxy(nil)
	if err != nil {
		logging.Warn("aite proxy init failed: %v", err)
		return fmt.Errorf("proxy init failed: %v", err)
	}
	// Build a minimal chat: agent persona + recent context + current user message.
	messages := []openaistyle.ChatMessage{}
	if systemPrompt != "" {
		messages = append(messages, openaistyle.ChatMessage{
			Role:    openaistyle.RoleSystem,
			Content: systemPrompt,
		})
	}
	for _, recent := range buildIntentRecentContext(ctx, message) {
		role := strings.TrimSpace(recent.Role)
		if role != openaistyle.RoleUser && role != openaistyle.RoleAssistant {
			continue
		}
		if strings.TrimSpace(recent.Content) == "" {
			continue
		}
		messages = append(messages, openaistyle.ChatMessage{Role: role, Content: recent.Content})
	}

	ctx = context.WithValue(ctx, utils.AgentID, agentID)

	var responeseAgent *proxy.ToolAndContent
	if responeseAgent, err = p.CommunicateWithMessages(ctx, messages); err != nil {
		logging.Warn("aite chat failed agent=%s: %v", agentID, err)
		return fmt.Errorf("chat failed: %s: %v", agentID, err)
	}
	if responeseAgent != nil && responeseAgent.Content != "" {
		chatId := ctx.Value(utils.ChatIDString).(string)
		memory.AddAssistantContentMessage(chatId, fmt.Sprintf("%s: %s", info["agent_name"], responeseAgent.Content))
	}
	return nil
}
func chatWithAitessistant(ctx context.Context, message string, agentList []string) error {
	if len(agentList) == 0 {
		return nil
	}
	unique := make([]string, 0, len(agentList))
	seen := map[string]struct{}{}
	for _, id := range agentList {
		s := strings.TrimSpace(id)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		unique = append(unique, s)
	}

	// 使用 WaitGroup 等待所有 goroutine 完成
	var wg sync.WaitGroup
	errChan := make(chan error, len(unique))

	for _, aid := range unique {
		wg.Add(1)
		go func(agentID string) {
			defer wg.Done()
			if err := handleAgentChat(ctx, message, agentID); err != nil {
				errChan <- err
			}
		}(aid)
	}

	// 等待所有 goroutine 完成
	wg.Wait()
	close(errChan)

	// 收集所有错误
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("multiple errors occurred: %v", errors)
	}

	return nil
}

// selectRandomAgents 从agent列表中随机选择指定数量的agent ID
func (d *Dispatcher) selectRandomAgents(agentList []string, maxCount int) []string {
	if len(agentList) == 0 {
		return nil
	}

	// 确定实际选择的数量
	actualCount := min(len(agentList), maxCount)

	// 根据发言历史重新排序agentList
	// 将最近发言的agent放在列表后面
	spokenAgents := make(map[string]int)

	// 记录每个agent在历史中的位置（越近的位置值越大）
	for i, agentID := range d.agentSpeakHistory {
		spokenAgents[agentID] = i
	}

	// 分离已发言和未发言的agent
	unspokenAgents := make([]string, 0)
	spokenAgentsList := make([]string, 0)

	for _, agentID := range agentList {
		if _, ok := spokenAgents[agentID]; ok {
			spokenAgentsList = append(spokenAgentsList, agentID)
		} else {
			unspokenAgents = append(unspokenAgents, agentID)
		}
	}

	// 将未发言的agent放在前面，已发言的agent按时间倒序排列（最近发言的越靠后）
	// 对已发言的agent按历史顺序倒序排列
	for i := len(spokenAgentsList) - 1; i >= 0; i-- {
		unspokenAgents = append(unspokenAgents, spokenAgentsList[i])
	}

	// 随机打乱未发言的agent列表
	rand.Shuffle(len(unspokenAgents), func(i, j int) {
		unspokenAgents[i], unspokenAgents[j] = unspokenAgents[j], unspokenAgents[i]
	})

	// 提取前actualCount个agent
	selectedAgentIDs := make([]string, 0, actualCount)
	for i := 0; i < actualCount && i < len(unspokenAgents); i++ {
		selectedAgentIDs = append(selectedAgentIDs, unspokenAgents[i])
	}

	// 更新发言历史
	d.agentSpeakHistory = append(selectedAgentIDs, d.agentSpeakHistory...)
	// 限制历史记录长度，避免无限增长
	if len(d.agentSpeakHistory) > 20 {
		d.agentSpeakHistory = d.agentSpeakHistory[:20]
	}

	return selectedAgentIDs
}

func (d *Dispatcher) processMessageWithIntent(ctx context.Context, message string) {
	ctx = d.attachProfileContext(ctx)
	if shouldUseActionGate(message) {
		needAction, handled, err := d.handleActionGateChat(ctx, message)
		if err == nil && handled && !needAction {
			return
		}
		if err != nil {
			logging.Warn("action-gate chat 失败，回退到意图识别: %v", err)
		}
	} else {
		logging.Info("明显需要工具/规划，跳过 action-gate: %s", message)
	}
	intent, err := ConfirmIntention(ctx, message, d.Intention)
	if err != nil {
		logging.Error("确认意图失败: %v", err)
		globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("%s", "确认意图失败..."))
		return
	}
	if intent == nil {
		logging.Error("确认意图失败: 返回了空意图且没有错误")
		globalchannel.SendAssitantMessageOnce(ctx, "确认意图失败...")
		return
	}
	logging.Info("确认意图: %s", intent.Intent)
	d.Intention = intent
	if intent.Intent == utils.PlanModeString {
		d.isPlanning = true
	}
	taskProfile := AnalyzeTask(message, d.Intention)
	executionBlueprint := BuildExecutionBlueprint(taskProfile, d.Intention)
	ctx = context.WithValue(ctx, utils.TaskProfileString, taskProfile)
	ctx = context.WithValue(ctx, utils.ExecutionBlueprintString, executionBlueprint)
	if strings.TrimSpace(executionBlueprint.ToolSource) != "" {
		d.Intention.ToolSource = executionBlueprint.ToolSource
	}
	if strings.TrimSpace(executionBlueprint.ToolTopic) != "" {
		d.Intention.ToolTopic = executionBlueprint.ToolTopic
	}

	logging.Info("意图: %s", d.Intention.Intent)

	if d.Intention.RequiresClarification {
		if d.Intention.Content != "" {
			globalchannel.SendAssitantMessageOnce(ctx, d.Intention.Content)
		} else {
			globalchannel.SendAssitantMessageOnce(ctx, "我需要先确认一点信息，才能继续处理这个请求。")
		}
		return
	}

	if d.Intention.Intent != "" {
		d.switchIntent(ctx, d.Intention)
	}

	subIntentions := d.Intention.SubIntents

	for _, subIntent := range subIntentions {
		if strings.TrimSpace(subIntent.Intent) == "" {
			continue
		}
		d.switchIntent(ctx, &Intention{
			Goal:       subIntent.Goal,
			Intent:     subIntent.Intent,
			Content:    subIntent.Content,
			ToolTopic:  subIntent.ToolTopic,
			ToolSource: subIntent.ToolSource,
		})
	}
}

func verifyGoal(ctx context.Context, goal string) error {
	agent_name := "话题维护者"
	agentID := "话题维护者"
	systemPrompt := strings.TrimSpace(fmt.Sprintf(`你叫 话题维护者。这轮话题是：%s.
	角色设定：超级智慧的社交leader.
	你正在一个多人群聊中。你会看到「From xxx: ...」格式的消息，xxx 是发言人。
	你要对他们的发言进行话题维护，确保他们的话题在群里是合适的。及时纠正话题偏离。引导他们回到合适的话题。启发式开启对应的话题。对偏离话题的特定人的发言进行针对性回复。
	今天日期：%s`, goal, time.Now().Format("2006-01-02 Monday"),
	))

	p, err := proxy.NewProxy(nil)
	if err != nil {
		logging.Warn("aite proxy init failed: %v", err)
		return fmt.Errorf("proxy init failed: %v", err)
	}
	// Build a minimal chat: agent persona + recent context + current user message.
	messages := []openaistyle.ChatMessage{}
	if systemPrompt != "" {
		messages = append(messages, openaistyle.ChatMessage{
			Role:    openaistyle.RoleSystem,
			Content: systemPrompt,
		})
	}
	for _, recent := range buildIntentRecentContext(ctx, "开始话题维护") {
		role := strings.TrimSpace(recent.Role)
		if role != openaistyle.RoleUser && role != openaistyle.RoleAssistant {
			continue
		}
		if strings.TrimSpace(recent.Content) == "" {
			continue
		}
		messages = append(messages, openaistyle.ChatMessage{Role: role, Content: recent.Content})
	}
	ctx = context.WithValue(ctx, utils.AgentID, agentID)

	var responeseAgent *proxy.ToolAndContent
	if responeseAgent, err = p.CommunicateWithMessages(ctx, messages); err != nil {
		logging.Warn("aite chat failed agent=%s: %v", agentID, err)
		return fmt.Errorf("chat failed: %s: %v", agentID, err)
	}
	if responeseAgent != nil && responeseAgent.Content != "" {
		chatId := ctx.Value(utils.ChatIDString).(string)
		memory.AddAssistantContentMessage(chatId, fmt.Sprintf("From %s: %s", agent_name, responeseAgent.Content))
	}
	return nil
}
