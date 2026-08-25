package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"leiAgent/dataoperation"
	mcpbridge "leiAgent/internal/MCP"
	"leiAgent/internal/agent"
	"leiAgent/internal/capabilities"
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
	"time"

	filefunctions "leiAgent/internal/tools/filefunctions"
	"leiAgent/internal/tools/libraryfs"
	"leiAgent/internal/tools/noveltool"
	"leiAgent/internal/tools/openclawtool"
	searchfunctions "leiAgent/internal/tools/searchfunctions"
	"leiAgent/internal/tools/timetools"
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
	Stop      bool
}

func init() {
	toolRegistry := tools.Getregistry()

	getTime := timetools.NewTimeTool()
	calculateTimeTool := timetools.NewCalculateTimeTool()
	getWeatherTool := searchfunctions.NewWeatherTool()
	getLongitude := searchfunctions.NewGeocodingTool()
	financeMarket := searchfunctions.NewMarketTool()
	getcurrenttime := timetools.NewCurrentTimeTool()
	wikiSearch := searchfunctions.NewWikipediaSearchTool()
	downloadBooks := searchfunctions.NewDownloadBooksTool()
	bashfunction := bashfunction.NewBashTool()

	listMCPTools := mcptool.NewListMCPTools(nil)
	callMCPTool := mcptool.NewCallMCPTool(nil)
	registerMCPFromHub := mcptool.NewRegisterMCPFromHub()
	openClawBaiduSearch := openclawtool.NewBaiduSearchTool()
	installOpenClawSkillFromMarket := openclawtool.NewInstallOpenClawSkillFromMarket(nil)
	readOpenClawSkill := capabilities.NewReadSkillTool()

	toolRegistry.Register(bashfunction)
	toolRegistry.Register(readOpenClawSkill)
	toolRegistry.Register(listMCPTools)
	toolRegistry.Register(callMCPTool)
	toolRegistry.Register(registerMCPFromHub)
	toolRegistry.Register(openClawBaiduSearch)
	toolRegistry.Register(installOpenClawSkillFromMarket)

	toolRegistry.Register(filefunctions.GetWriteFileChunk())
	toolRegistry.Register(filefunctions.GetFileWriteTool())
	toolRegistry.Register(filefunctions.NewFileDownloadTool())
	toolRegistry.Register(libraryfs.New())
	// toolRegistry.Register(memotool.NewMemoWriteTool())
	toolRegistry.Register(noveltool.New())
	toolRegistry.Register(getcurrenttime)
	toolRegistry.Register(financeMarket)
	toolRegistry.Register(getLongitude)
	toolRegistry.Register(getWeatherTool)
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
		case msg, ok := <-globalchannel.GetGlobalInputChannel(d.ChatID):
			if !ok {
				return
			}
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

	//globalchannel.SendAssistantMessageOnce(d.ctx, fmt.Sprintf("%s", "终止任务运行..."))
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
	message := strings.TrimSpace(msg.Content)
	logging.Info("Dispatcher 处理消息: %s", message)

	if len(msg.UserToAgentList) > 0 {
		d.handleMentionedMessage(ctx, message, msg.UserToAgentList)
		return
	}
	if msg.IsAutoToTalk {
		d.handleDefaultGroupMessage(ctx, message)
		return
	}

	d.processMessageWithIntent(context.WithValue(ctx, utils.AgentID, defaultAssistantAgentID), message)
}

func (d *Dispatcher) handleActionGateChat(ctx context.Context, message string) (needAction bool, handled bool, err error) {
	chatId := ctx.Value(utils.ChatIDString).(string)

	p, err := proxy.NewClient(nil)
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
	needAction, handled = interpretActionGateResult(tc)
	if !handled {
		logging.Warn("action-gate 判定为普通对话但未返回正文，回退到意图识别")
		return needAction, handled, nil
	}
	if !needAction {
		memory.AddAssistantContentMessage(chatId, tc.Content)
	}
	return needAction, handled, nil
}

// interpretActionGateResult 只把带有可展示正文的普通对话视为已处理。
// 部分模型会只返回 [needAction:false]；该标签被代理层剥离后正文为空，
// 此时必须回退到完整意图/聊天链路，否则用户看不到任何响应。
func interpretActionGateResult(tc *proxy.ToolAndContent) (needAction bool, handled bool) {
	if tc == nil {
		return false, false
	}
	if tc.NeedAction {
		return true, true
	}
	return false, strings.TrimSpace(tc.Content) != ""
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
		"续写", "写小说", "小说", "章节", "章书", "大纲", "长文", "长篇", "创作", "故事", "安装",
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
			sendAssistantAndRemember(ctx, intent.Content)
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
		globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("%s", "未知意图: "))
		logging.Error("未知意图: %s", d.Intention.Intent)
	}
}

func (d *Dispatcher) handlePlan(ctx context.Context, intent *Intention) {

	userGoal := strings.TrimSpace(intent.Goal)

	if _, ok := ctx.Value(utils.ChatIDString).(string); !ok {
		logging.Error("无法从 context 中获取 chatId")

		globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("%s", "无法从 context 中获取 chatId"))
		return
	}

	ctx = context.WithValue(ctx, utils.IsPlanningString, true)

	globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("%s", "开始进行任务规划...\n"))
	logging.Info("开始进行任务规划...")

	// 单次规划调用：goal 传用户原文。此前此处先 Communicate 再 GeneratePlan，会重复规划且第二次仍用包装后的长串污染 goal。
	pInst, err := planner.GeneratePlan(ctx, userGoal, string(toolsInfo()))
	if err != nil {
		logging.Error("生成计划失败: %v", err)
		globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("生成计划失败: %v", err))
		return
	}
	if pInst == nil {
		globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("%s", "生成计划失败，请确认任务意图是否正确"))
		return
	}
	globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("%s", "规划完成，开始执行..."))

	pInst.DoTask(ctx)

}

func (d *Dispatcher) handleChat(ctx context.Context, intent *Intention) {
	message := intent.Goal
	chatId := ctx.Value(utils.ChatIDString).(string)
	memory.SetSystemPrompt(chatId, utils.ChatPromptTemplate)
	logging.Info("对话系统提示词已加载...")
	p, err := proxy.NewClient(nil)
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
	memory.SetSystemPrompt(chatId, utils.SingleToolSystemPrompt())
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

const (
	defaultAssistantAgentID  = "agentid_0"
	groupRoleMaxOutputTokens = 160
	groupToolmanPrompt       = `你是群聊中第一个发言的“工具人”。先直接处理用户问题，优先保证事实准确、立场客观和信息完整；需要实时信息或外部事实时使用可用工具核验，不确定的地方明确说明边界。不要模仿其他群成员，也不要为了热闹牺牲正确性。`
)

func defaultGroupReplyOrder(agentIDs []string, pickIndex func(int) int) []string {
	eligible := make([]string, 0, len(agentIDs))
	seen := map[string]struct{}{defaultAssistantAgentID: {}}
	for _, id := range agentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		eligible = append(eligible, id)
	}

	order := []string{defaultAssistantAgentID}
	if len(eligible) > 0 {
		order = append(order, eligible[pickIndex(len(eligible))])
	}
	return order
}

func mentionedReplyAgent(agentIDs []string, pickIndex func(int) int) string {
	unique := make([]string, 0, len(agentIDs))
	seen := make(map[string]struct{}, len(agentIDs))
	for _, id := range agentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return ""
	}
	return unique[pickIndex(len(unique))]
}

func (d *Dispatcher) handleMentionedMessage(ctx context.Context, message string, mentionedAgentIDs []string) {
	agentID := mentionedReplyAgent(mentionedAgentIDs, rand.Intn)
	if agentID == "" {
		return
	}
	if agentID == defaultAssistantAgentID {
		d.processMessageWithIntent(context.WithValue(ctx, utils.AgentID, agentID), message)
		return
	}
	if err := d.handleMentionedRoleReply(ctx, message, agentID); err != nil {
		logging.Warn("被 @ 角色回复失败 agent=%s: %v", agentID, err)
	}
}

func (d *Dispatcher) handleDefaultGroupMessage(ctx context.Context, message string) {
	candidates, err := dataoperation.ListAgentsInConversation(d.ChatID)
	if err != nil {
		logging.Warn("读取群聊成员失败，当前轮仅由工具人回复: %v", err)
		candidates = nil
	}

	chatID, _ := ctx.Value(utils.ChatIDString).(string)
	var toolmanAnswer string
	for index, agentID := range defaultGroupReplyOrder(candidates, rand.Intn) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if d.Stop {
			return
		}

		if index == 0 {
			before := latestAssistantContent(memory.GetLocalMemory().GetMessages(chatID))
			d.processMessageWithIntent(withGroupToolmanPolicy(ctx), message)
			after := latestAssistantContent(memory.GetLocalMemory().GetMessages(chatID))
			if after == "" || after == before {
				logging.Warn("工具人本轮没有产生可作为事实基准的新回复，跳过其他角色")
				return
			}
			toolmanAnswer = after
			continue
		}
		if err := d.handleGroupRoleReply(ctx, message, toolmanAnswer, agentID); err != nil {
			logging.Warn("群聊角色回复失败 agent=%s: %v", agentID, err)
		}
	}
}

func latestAssistantContent(messages []*memory.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil || message.Role != memory.MessageRoleAssistant {
			continue
		}
		if content := strings.TrimSpace(message.Content); content != "" {
			return content
		}
	}
	return ""
}

func withGroupToolmanPolicy(ctx context.Context) context.Context {
	extra, _ := ctx.Value(utils.ExtraSystemMessagesString).([]string)
	messages := append([]string(nil), extra...)
	messages = append(messages, groupToolmanPrompt)
	ctx = context.WithValue(ctx, utils.ExtraSystemMessagesString, messages)
	return context.WithValue(ctx, utils.AgentID, defaultAssistantAgentID)
}

func (d *Dispatcher) handleGroupRoleReply(ctx context.Context, message, toolmanAnswer, agentID string) error {
	info, err := dataoperation.GetAgent(agentID)
	if err != nil || info == nil {
		logging.Warn("group agent not found: %s: %v", agentID, err)
		return fmt.Errorf("agent not found: %s", agentID)
	}
	systemPrompt := strings.TrimSpace(fmt.Sprintf(`你的名字是 %s。角色设定：%s

你正在微信群里接着用户和“工具人”聊天。工具人本轮给出的内容是唯一事实基准，你必须接受并以它为准。

严格要求：
- 不得质疑、否定、纠正或重新核验工具人的信息，不得引入与它冲突的新事实或来源。
- 只能基于工具人的信息表达态度、补充生活化角度，或问一句真正有用的问题；没有可补充内容就简短附和。
- 只说一到三句短句，通常不超过 80 个汉字。
- 不写标题、列表、总结、教程或客服腔，不复述工具人已经说清楚的内容。
- 不编造型号、数字、日期、出处或结论。
- 不在开头报自己的名字，不解释角色设定。

今天日期：%s`,
		info["agent_name"],
		info["description"],
		time.Now().Format("2006-01-02 Monday"),
	))
	if systemPrompt == "" {
		return fmt.Errorf("agent %s has no description", agentID)
	}
	messages := buildGroupRoleMessages(systemPrompt, message, toolmanAnswer)
	return sendGroupRoleReply(ctx, agentID, strings.TrimSpace(fmt.Sprint(info["agent_name"])), messages)
}

func (d *Dispatcher) handleMentionedRoleReply(ctx context.Context, message, agentID string) error {
	info, err := dataoperation.GetAgent(agentID)
	if err != nil || info == nil {
		return fmt.Errorf("agent not found: %s", agentID)
	}
	systemPrompt := strings.TrimSpace(fmt.Sprintf(`你的名字是 %s。角色设定：%s

你正在一个微信群里，用户刚刚明确 @ 了你。请由你直接回应，不要等待或代替其他成员发言。

严格要求：
- 结合最近对话理解用户在指什么；如果用户只发了 @名字，就自然回应紧邻的上一段话题。
- 回复像微信群里的真人，只说一到三句短句，通常不超过 80 个汉字。
- 不写标题、列表、教程、总结或客服腔，不复述角色设定。
- 不冒充工具人，不声称做过实际没有做过的搜索或核验；不确定就直说。
- 不在开头报自己的名字。

今天日期：%s`,
		info["agent_name"],
		info["description"],
		time.Now().Format("2006-01-02 Monday"),
	))
	messages := buildMentionedRoleMessages(systemPrompt, buildIntentRecentContext(ctx, message), message)
	return sendGroupRoleReply(ctx, agentID, strings.TrimSpace(fmt.Sprint(info["agent_name"])), messages)
}

func sendGroupRoleReply(ctx context.Context, agentID, agentName string, messages []openaistyle.ChatMessage) error {
	p, err := proxy.NewClient(nil)
	if err != nil {
		return fmt.Errorf("proxy init failed: %v", err)
	}

	ctx = context.WithValue(ctx, utils.AgentID, agentID)
	ctx = context.WithValue(ctx, utils.MaxOutputTokensString, groupRoleMaxOutputTokens)

	var responseAgent *proxy.ToolAndContent
	if responseAgent, err = p.CommunicateWithMessages(ctx, messages); err != nil {
		logging.Warn("group chat failed agent=%s: %v", agentID, err)
		return fmt.Errorf("chat failed: %s: %v", agentID, err)
	}
	if responseAgent != nil && responseAgent.Content != "" {
		chatID, _ := ctx.Value(utils.ChatIDString).(string)
		memory.AddAssistantContentMessage(chatID, fmt.Sprintf("%s: %s", agentName, responseAgent.Content))
	}
	return nil
}

func buildMentionedRoleMessages(systemPrompt string, recent []IntentContextMessage, userMessage string) []openaistyle.ChatMessage {
	messages := []openaistyle.ChatMessage{{Role: openaistyle.RoleSystem, Content: systemPrompt}}
	for _, item := range recent {
		role := strings.TrimSpace(item.Role)
		if role != openaistyle.RoleUser && role != openaistyle.RoleAssistant {
			continue
		}
		if content := strings.TrimSpace(item.Content); content != "" {
			messages = append(messages, openaistyle.ChatMessage{Role: role, Content: content})
		}
	}
	messages = append(messages, openaistyle.ChatMessage{Role: openaistyle.RoleUser, Content: strings.TrimSpace(userMessage)})
	return messages
}

func buildGroupRoleMessages(systemPrompt, userMessage, toolmanAnswer string) []openaistyle.ChatMessage {
	return []openaistyle.ChatMessage{
		{Role: openaistyle.RoleSystem, Content: systemPrompt},
		{
			Role: openaistyle.RoleUser,
			Content: fmt.Sprintf("用户原话：%s\n\n【工具人本轮权威信息】\n%s\n\n请只基于以上权威信息自然接一句。",
				strings.TrimSpace(userMessage),
				strings.TrimSpace(toolmanAnswer),
			),
		},
	}
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
		globalchannel.SendAssistantMessageOnce(ctx, fmt.Sprintf("确认意图失败：%v", err))
		return
	}
	if intent == nil {
		logging.Error("确认意图失败: 返回了空意图且没有错误")
		globalchannel.SendAssistantMessageOnce(ctx, "确认意图失败：模型未返回有效意图，请重试或检查模型与网络配置。")
		return
	}
	logging.Info("确认意图: %s", intent.Intent)
	d.Intention = intent
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
			sendAssistantAndRemember(ctx, d.Intention.Content)
		} else {
			sendAssistantAndRemember(ctx, "我需要先确认一点信息，才能继续处理这个请求。")
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

func sendAssistantAndRemember(ctx context.Context, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	globalchannel.SendAssistantMessageOnce(ctx, content)
	if chatID, ok := ctx.Value(utils.ChatIDString).(string); ok && strings.TrimSpace(chatID) != "" {
		memory.AddAssistantContentMessage(chatID, content)
	}
}
