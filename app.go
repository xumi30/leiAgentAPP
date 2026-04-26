package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"leiAgent/dataoperation"

	"leiAgent/internal/crontabthread"
	"leiAgent/internal/dispatcher"
	"leiAgent/internal/doclib"
	"leiAgent/internal/globalchannel"
	"leiAgent/internal/memo"
	"leiAgent/internal/memory"
	"leiAgent/internal/openclawskill"
	"leiAgent/internal/profile"
	"leiAgent/internal/proxy"
	"leiAgent/internal/tools/noveltool"
	"leiAgent/logging"
	"leiAgent/utils"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type userSendPayload struct {
	Content string   `json:"content"`
	Aite    []string `json:"aite"`
}

func stripLeadingMentions(s string) string {
	// Remove leading "@xxx" tokens separated by spaces/commas, e.g.:
	// "@A @B hello" -> "hello"
	// "@A, @B，hello" -> "hello"
	in := strings.TrimLeft(s, " \t\r\n")
	for {
		if !strings.HasPrefix(in, "@") {
			break
		}
		// scan token "@<non-separator>"
		i := 1
		for i < len(in) {
			ch := in[i]
			if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == ',' {
				break
			}
			// UTF-8 comma '，' may appear; treat as separator if it starts at this byte.
			if strings.HasPrefix(in[i:], "，") {
				break
			}
			i++
		}
		// consume mention token
		in = in[i:]
		// consume separators after token
		in = strings.TrimLeft(in, " \t\r\n")
		for strings.HasPrefix(in, ",") || strings.HasPrefix(in, "，") {
			in = strings.TrimLeft(in[1:], " \t\r\n")
		}
	}
	return strings.TrimSpace(in)
}

func parseUserSendPayload(raw string) (content string, aite []string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil, false
	}
	if !strings.HasPrefix(raw, "{") {
		return "", nil, false
	}
	var p userSendPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return "", nil, false
	}
	c := strings.TrimSpace(p.Content)
	if c == "" {
		return "", nil, false
	}
	out := make([]string, 0, len(p.Aite))
	seen := map[string]struct{}{}
	for _, id := range p.Aite {
		s := strings.TrimSpace(id)
		if s == "" {
			continue
		}
		if _, exists := seen[s]; exists {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	plain := stripLeadingMentions(c)
	if plain != "" {
		c = plain
	}
	return c, out, true
}

// App struct
type App struct {
	ctx context.Context

	agentPool    map[string]*dispatcher.Dispatcher
	poolLastUsed map[string]time.Time // 与 poolMutex 共用：最近一次 SwitchChat/dispatcher 命中时间，用于满池时 LRU 驱逐
	poolMutex    sync.RWMutex

	switchMu         sync.Mutex
	lastActiveChatID string // 当前前端选中的会话，用于切换前落盘与关闭时落盘
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.agentPool = make(map[string]*dispatcher.Dispatcher)
	a.poolLastUsed = make(map[string]time.Time)
	a.ctx = ctx
	// 必须先于其它包调用 sqlmemory.GetSqlInstance，否则 sync.Once 会锁在错误的默认库路径上
	if dataoperation.GetSqlInstance() == nil {
		logging.Error("启动时未能打开对话数据库 data/memory.db")
	}
	if err := dataoperation.SyncPresetAgents(); err != nil {
		logging.Warn("同步预设 agents 失败: %v", err)
	}
	// Start scheduled task runner (polls due tasks, claims with optimistic lock, executes).
	_ = crontabthread.NewRunner(2*time.Second, 100, nil).Start(ctx)

	// Best-effort: silently install default skills once (non-blocking).
	// Disable by setting LEIAGENT_AUTO_INSTALL_SKILLS=0/false/off.
	go func() {
		v := strings.ToLower(strings.TrimSpace(os.Getenv("LEIAGENT_AUTO_INSTALL_SKILLS")))
		if v == "0" || v == "false" || v == "off" || v == "no" {
			return
		}
		if _, ok := openclawskill.Find("baidu-search"); ok {
			return
		}
		installCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if _, err := openclawskill.Install(installCtx, "claw skill install official/baidu-search"); err != nil {
			logging.Warn("自动安装 baidu-search skill 失败（可在设置页手动安装）：%v", err)
			return
		}
		// Optional deps install: ignore errors (python missing / network).
		if skill, ok := openclawskill.Find("baidu-search"); ok {
			_, _ = openclawskill.InstallDeps(skill.Path)
		}
	}()
}
func (a *App) ListConversation() []map[string]interface{} {
	// 模拟对话数据
	conversations := dataoperation.ListConverstions()
	return conversations

}

func (a *App) AddConversation(title string) string {

	chatID := GenerateChatID() // 生成随机对话ID
	logging.Info("Adding conversation with ID: %s and Title: %s", chatID, title)
	// 模拟添加对话
	err := dataoperation.AddConversation(chatID, title)
	if err != nil {
		runtime.EventsEmit(a.ctx, "addConversationError", err.Error())
		return ""
	}
	a.GetConversation(chatID)
	logging.Info("Conversation with ID: %s added successfully", chatID)
	return chatID
}

func (a *App) GetConversation(chatID string) {
	logging.Info("Getting conversation with ID: %s", chatID)
	conversation := dataoperation.GetConversation(chatID)
	if conversation == nil {
		runtime.EventsEmit(a.ctx, "getConversationError", "Conversation not found")
		return
	}
	runtime.EventsEmit(a.ctx, "getConversation", conversation)
}

func (a *App) DeleteConversation(chatID string) error {
	logging.Info("Deleting conversation	 with ID: %s", chatID)

	err := dataoperation.DeleteConversation(chatID)
	if err != nil {
		runtime.EventsEmit(a.ctx, "deleteConversationError", err.Error())
		return err
	}
	if err := profile.Delete(chatID); err != nil {
		logging.Warn("删除画像失败 chatID=%s: %v", chatID, err)
	}
	logging.Info("Conversation with ID: %s deleted successfully", chatID)
	runtime.EventsEmit(a.ctx, "deleteConversationSuccess", chatID)
	return nil
}

func (a *App) UpdateConversationTitle(chatID, newTitle string) {
	logging.Info("Updating conversation with ID: %s to new Title: %s", chatID, newTitle)
	err := dataoperation.UpdateConversationTitle(chatID, newTitle)
	if err != nil {
		runtime.EventsEmit(a.ctx, "updateConversationError", err.Error())
		return
	}
	a.GetConversation(chatID)
}

func (a *App) GetMessages(chatID string) []map[string]interface{} {
	messages := dataoperation.GetDialogs(chatID)
	//logging.Info("Getting messages for conversation with ID: %s %v", chatID, messages)
	return messages
}

// ListScheduledTasks returns scheduled tasks stored in data/scheduled_tasks.db.
// status can be: active/paused/deleted or empty for all (excluding deleted by default).
func (a *App) ListScheduledTasks(status string, includeDeleted bool, limit int, offset int) ([]map[string]interface{}, error) {
	logging.Info("[ui] ListScheduledTasks status=%q includeDeleted=%v limit=%d offset=%d", strings.TrimSpace(status), includeDeleted, limit, offset)
	return crontabthread.ListTasks(crontabthread.ListOptions{
		Status:         status,
		IncludeDeleted: includeDeleted,
		Limit:          limit,
		Offset:         offset,
	})
}

func (a *App) DeleteScheduledTask(id string) error {
	logging.Info("[ui] DeleteScheduledTask id=%q", strings.TrimSpace(id))
	return crontabthread.DeleteTask(id)
}

func (a *App) SetScheduledTaskStatus(id string, status string) error {
	logging.Info("[ui] SetScheduledTaskStatus id=%q status=%q", strings.TrimSpace(id), strings.TrimSpace(status))
	return crontabthread.SetTaskStatus(id, status)
}

func (a *App) UpdateScheduledTaskBasics(id string, title string, actionPayload string) error {
	logging.Info("[ui] UpdateScheduledTaskBasics id=%q", strings.TrimSpace(id))
	return crontabthread.UpdateTaskBasics(id, title, actionPayload)
}

// GetLocalMemoryMessages 返回当前 chat 的 localMemory（用于调试/查看 LLM 上下文）。
func (a *App) GetLocalMemoryMessages(chatID string) []map[string]interface{} {
	cid := strings.TrimSpace(chatID)
	msgs := memory.GetLocalMemory().GetMessages(cid)
	out := make([]map[string]interface{}, 0, len(msgs))
	for i, m := range msgs {
		if m == nil {
			continue
		}
		item := map[string]interface{}{
			"idx":         i,
			"role":        string(m.Role),
			"content":     m.Content,
			"toolCallID":  m.ToolCallID,
			"toolCalls":   m.ToolCalls,
			"toolCallCnt": len(m.ToolCalls),
		}
		out = append(out, item)
	}
	return out
}

// GetUserProfile 返回当前 chat 的结构化用户画像；不存在时返回空画像。
func (a *App) GetUserProfile(chatID string) map[string]interface{} {
	cid := strings.TrimSpace(chatID)
	raw, _ := json.Marshal(profile.Get(cid))
	out := map[string]interface{}{}
	_ = json.Unmarshal(raw, &out)
	return out
}

// RefreshUserProfile 基于会话历史刷新结构化用户画像。
func (a *App) RefreshUserProfile(chatID string) (map[string]interface{}, error) {
	cid := strings.TrimSpace(chatID)
	p, err := profile.Refresh(a.ctx, cid)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(p)
	out := map[string]interface{}{}
	_ = json.Unmarshal(raw, &out)
	return out, nil
}

func (a *App) GetMessagesEvent(chatID string) {
	messages := dataoperation.GetDialogs(chatID)
	//logging.Info("GetMessagesEvent for conversation with ID: %s %v", chatID, messages)
	runtime.EventsEmit(a.ctx, "ListMessages", messages)
}

func (a *App) GetMessagesByMessageID(messageID string) {
	message := dataoperation.GetDialogsByMessageID(messageID)
	logging.Info("Getting message for conversation with messageID: %s %v", messageID, message)
	runtime.EventsEmit(a.ctx, "GetMessagesByMessageID", message)

}

func (a *App) SendMessage(chatID, message, role string) {

	//如果message为空，则不发送
	if strings.TrimSpace(message) == "" {
		logging.Info("Message is empty, not sending")
		runtime.EventsEmit(a.ctx, "sendMessageError", "messages is empty, not sending")
		return
	}

	content := message
	contentForPersist := message
	var agentList []string
	if strings.EqualFold(strings.TrimSpace(role), utils.MessageRoleUser) {
		if c, aite, ok := parseUserSendPayload(message); ok {
			// Persist original display content (with @mentions), but process with plain content.
			if rawContent, ok2 := func() (string, bool) {
				var p userSendPayload
				if err := json.Unmarshal([]byte(strings.TrimSpace(message)), &p); err != nil {
					return "", false
				}
				raw := strings.TrimSpace(p.Content)
				if raw == "" {
					return "", false
				}
				return raw, true
			}(); ok2 {
				contentForPersist = rawContent
			}
			content = c
			agentList = aite
			logging.Info("SendMessage payload parsed: content_len=%d aite=%v raw_prefix=%q", len(content), agentList, utils.TruncateRunes(strings.TrimSpace(message), 120))
		} else {
			logging.Info("SendMessage payload NOT parsed (treat as plain text): raw_prefix=%q", utils.TruncateRunes(strings.TrimSpace(message), 120))
		}
	}
	//如果chatID为空，则生成一个新的chatID，并创建一个新的conversation
	if chatID == "" {
		logging.Info("ChatID is empty, adding new conversation")
		title := proxy.GenerateConversationTitle(context.Background(), content)
		chatID = a.AddConversation(title)
		logging.Info("New conversation added with ID: %s", chatID)
		a.SwitchChat(chatID)
	}
	// StopChat 会从 agentPool 移除 dispatcher，无 goroutine 再接收 inputChan，此处会永久阻塞；用户消息需先重新拉起 dispatcher。
	if strings.EqualFold(strings.TrimSpace(role), utils.MessageRoleUser) {
		a.dispatcher(chatID)
	}
	inputChan := globalchannel.GetGlobalInputChannel(chatID)

	messageID := GenerateMessageID()

	//logging.Info("Sending message to conversation with ID: %s, messageID: %s, Message: %s, Role: %s", chatID, messageID, message, role)
	err := dataoperation.SendMessage(chatID, messageID, contentForPersist, role)
	logging.Info("Sending message to conversation successfully")
	if err != nil {
		runtime.EventsEmit(a.ctx, "sendMessageError", err.Error())
		return
	}
	a.GetMessagesByMessageID(messageID)

	// 仅用户消息进入 Dispatcher，避免助手/推理等内容若误走 SendMessage 时再次触发意图识别与死循环。
	if strings.EqualFold(strings.TrimSpace(role), utils.MessageRoleUser) {
		msg := &globalchannel.Message{
			MessageID:  utils.GenerateMessageID(),
			Content:    content,
			Role:       utils.MessageRoleUser,
			IsFinished: true,
			AgentList:  agentList,
		}

		inputChan <- msg
	}
	logging.Info("Sending message to conversation successfully")

}

// SendUserDisplayOnly 将用户消息写入对话并通知前端，但不送入 Dispatcher（用于「暂停」等控制话术，避免再次触发意图识别）。
func (a *App) SendUserDisplayOnly(chatID, message string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		logging.Info("SendUserDisplayOnly: message is empty")
		runtime.EventsEmit(a.ctx, "sendMessageError", "messages is empty, not sending")
		return
	}
	if chatID == "" {
		logging.Info("SendUserDisplayOnly: ChatID is empty, adding new conversation")
		title := proxy.GenerateConversationTitle(context.Background(), msg)
		chatID = a.AddConversation(title)
		logging.Info("New conversation added with ID: %s", chatID)
		a.SwitchChat(chatID)
	}
	messageID := GenerateMessageID()
	err := dataoperation.SendMessage(chatID, messageID, msg, utils.MessageRoleUser)
	if err != nil {
		runtime.EventsEmit(a.ctx, "sendMessageError", err.Error())
		return
	}
	a.GetMessagesByMessageID(messageID)
}

func GenerateChatID() string {
	chatID := fmt.Sprintf("%d%03d", time.Now().UnixMilli(), rand.Intn(1000))
	return chatID
}

func GenerateAgentID() string {
	agentID := fmt.Sprintf("agent_%d%04d", time.Now().UnixMilli(), rand.Intn(10000))
	return agentID
}

func GenerateMessageID() string {

	messageID := fmt.Sprintf("%d%06d", time.Now().UnixMilli(), rand.Intn(100000))
	logging.Info("Generated messageID: %s", messageID)
	return messageID
}

// SetLLMThinkingDisabled 为 true 时，前端隐藏推理面板，后端也不再发送 thinking 相关扩展字段。
func (a *App) SetLLMThinkingDisabled(disabled bool) {
	proxy.SetLLMThinkingDisabled(disabled)
}

func (a *App) ListAgents() ([]map[string]interface{}, error) {
	if err := dataoperation.SyncPresetAgents(); err != nil {
		return nil, err
	}
	return dataoperation.ListAgents()
}

func (a *App) CreateAgent(agentName, avatarImage, description string) (map[string]interface{}, error) {
	return dataoperation.CreateAgent(GenerateAgentID(), agentName, avatarImage, description)
}

func (a *App) DeleteCustomAgent(agentID string) error {
	return dataoperation.DeleteCustomAgent(agentID)
}

func (a *App) AddAgentToConversation(chatID, agentID string) error {
	return dataoperation.AddAgentToConversation(chatID, agentID)
}

func (a *App) GetConversationAgents(chatID string) ([]map[string]interface{}, error) {
	return dataoperation.ListConversationAgents(chatID)
}

// GetLLMThinkingDisabled 返回当前是否对 LLM 关闭了思考过程。
func (a *App) GetLLMThinkingDisabled() bool {
	return proxy.IsLLMThinkingDisabled()
}

func (a *App) GetReasoningMessage(chatID string) []map[string]interface{} {
	reasonings, err := dataoperation.GetReasonings(chatID)
	if err != nil {
		runtime.EventsEmit(a.ctx, "getReasoningMessageError", err.Error())
	}
	return reasonings
}

// ProxyAuthRequest forwards an auth request (login/register/send-code) to the
// proxy-lb server from Go to avoid CORS issues in the Wails webview.
// For login/register, if a token is returned the LLM config is updated automatically.
func (a *App) ProxyAuthRequest(path string, body map[string]interface{}) (map[string]interface{}, error) {
	username, _ := body["username"].(string)
	logging.Info("ProxyAuthRequest: path=%s username=%s", path, username)
	url := "http://127.0.0.1:7077" + path
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		logging.Error("ProxyAuthRequest 请求失败: path=%s err=%v", path, err)
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		result = map[string]interface{}{"raw": string(respBytes)}
	}
	result["_statusCode"] = resp.StatusCode
	logging.Info("ProxyAuthRequest: path=%s status=%d", path, resp.StatusCode)

	// 登录或注册成功后，自动写入 LLM 配置
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		token, _ := result["token"].(string)
		if token != "" {
			if saveErr := a.saveAuthConfig(token); saveErr != nil {
				logging.Error("登录成功但写入配置失败: %v", saveErr)
			}
		}
	}
	return result, nil
}

// saveAuthConfig 将认证 token 写入 LLM 配置的第一条后端。
func (a *App) saveAuthConfig(token string) error {
	state, err := proxy.GetLLMConfigFormState()
	if err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}
	currentRows := state.Backends
	nextRow := proxy.LLMConfigRow{
		Name:            "proxy-lb",
		APIKey:          strings.TrimSpace(token),
		BaseURL:         "http://127.0.0.1:7077/v1/chat/completions",
		Model:           "qwen",
		Provider:        "",
		StreamMode:      "both",
		MaxOutputTokens: 0,
		Enabled:         true,
	}
	var nextRows []proxy.LLMConfigRow
	if len(currentRows) > 0 {
		merged := currentRows[0]
		merged.Name = nextRow.Name
		merged.APIKey = nextRow.APIKey
		merged.BaseURL = nextRow.BaseURL
		merged.Model = nextRow.Model
		merged.StreamMode = nextRow.StreamMode
		nextRows = append([]proxy.LLMConfigRow{merged}, currentRows[1:]...)
	} else {
		nextRows = []proxy.LLMConfigRow{nextRow}
	}
	_, err = proxy.SaveLLMConfigForm(proxy.LLMConfigRow{}, nextRows)
	return err
}

// evictLRUDispatcherLocked 在 poolMutex 已持有时调用：驱逐最久未访问的会话 dispatcher（避免 map 随机迭代误杀正在跑的对话）。
func (a *App) evictLRUDispatcherLocked() {
	if len(a.agentPool) == 0 {
		return
	}
	victim := ""
	for k := range a.agentPool {
		if victim == "" {
			victim = k
			continue
		}
		tk, tv := a.poolLastUsed[k], a.poolLastUsed[victim]
		if tk.Before(tv) || (tk.Equal(tv) && k < victim) {
			victim = k
		}
	}
	if victim == "" {
		return
	}
	logging.Info("agentPool is full, evicting LRU dispatcher chatID=%s", victim)
	if err := memory.PersistLocalMemoryToYAMLFile(victim); err != nil {
		logging.Error("LRU 驱逐前写入本地记忆失败 chatID=%s: %v", victim, err)
	}
	if oldDp, exists := a.agentPool[victim]; exists {
		oldDp.Shutdown()
		delete(a.agentPool, victim)
		delete(a.poolLastUsed, victim)
	}
}

func (a *App) dispatcher(chatID string) *dispatcher.Dispatcher {
	a.poolMutex.Lock()
	defer a.poolMutex.Unlock()

	if a.agentPool == nil {
		a.agentPool = make(map[string]*dispatcher.Dispatcher)
	}
	if a.poolLastUsed == nil {
		a.poolLastUsed = make(map[string]time.Time)
	}

	dp, ok := a.agentPool[chatID]
	if !ok {
		if len(a.agentPool) >= 5 {
			a.evictLRUDispatcherLocked()
		}

		// 使用可取消的 context
		ctx, cancel := context.WithCancel(context.Background())
		ctx = context.WithValue(ctx, utils.ChatIDString, chatID)

		var err error
		dp, err = dispatcher.NewDispatcher(ctx, chatID, cancel) // 传递 cancel 函数
		if err != nil {
			logging.Error("创建 Dispatcher 失败: %v", err)
			errMsg := err.Error()
			if strings.Contains(errMsg, "未配置 API Key") {
				runtime.EventsEmit(a.ctx, "needLogin", errMsg)
			} else {
				globalchannel.SendAssitantMessageOnce(ctx, "创建 Dispatcher 失败: "+errMsg)
				runtime.EventsEmit(a.ctx, "dispatcherError", errMsg)
				if a.ctx != nil {
					_, dlgErr := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
						Type:    runtime.ErrorDialog,
						Title:   "无法启动对话引擎",
						Message: "创建 Dispatcher 失败：\n" + errMsg,
					})
					if dlgErr != nil {
						logging.Error("MessageDialog: %v", dlgErr)
					}
				}
			}
			cancel()
			return nil
		}
		a.agentPool[chatID] = dp
		// 启动 并处理返回
		logging.Info("Starting dispatcher for conversation with ChatID: %s", chatID)
		go dp.Run(ctx)
		go a.AppenAgentMessageToFrontRole(ctx, utils.MessageRoleAssistant, chatID)
		// go a.AppenAgentMessageToFrontRole(ctx, utils.MessageRoleReasoning, chatID)
		go a.AppendTaskStateToFront(ctx, chatID)
	}

	a.poolLastUsed[chatID] = time.Now()
	logging.Info("Getting dispatcher for conversation with ChatID: %s %v", chatID, dp)
	return dp
}

// restartDispatcherBackground 中断后重新拉起 Run 与输出监听，不重建 Dispatcher，以保留 Intention 等内存态。
func (a *App) restartDispatcherBackground(chatID string, dp *dispatcher.Dispatcher) {
	if dp == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, utils.ChatIDString, chatID)
	dp.ReplaceRunContext(ctx, cancel)
	go dp.Run(ctx)
	go a.AppenAgentMessageToFrontRole(ctx, utils.MessageRoleAssistant, chatID)
	go a.AppenAgentMessageToFrontRole(ctx, utils.MessageRoleReasoning, chatID)
	go a.AppendTaskStateToFront(ctx, chatID)
	logging.Info("Dispatcher 已重新监听 input/output，chatID=%s（保留原 Dispatcher 与意图）", chatID)
}

func (a *App) AppendTaskStateToFront(ctx context.Context, chatID string) {
	taskStateChan := globalchannel.GetGlobalTaskStateChannel(chatID)
	for {
		select {
		case msg, ok := <-taskStateChan:
			if !ok {
				return
			}
			if msg == nil {
				continue
			}
			runtime.EventsEmit(a.ctx, "chatTaskState", map[string]interface{}{
				"chatID": chatID,
				"busy":   strings.EqualFold(strings.TrimSpace(msg.Content), "busy"),
			})
		case <-ctx.Done():
			return
		}
	}
}

func (a *App) AppenAgentMessageToFrontRole(ctx context.Context, role, chatID string) {
	outputChan := globalchannel.GetGlobalDialogOutChannel(chatID)
	reasonningOutputChan := globalchannel.GetGlobalReasonOutChannel(chatID)
	eventname := "dialogAppend"

	if role == utils.MessageRoleReasoning {
		eventname = "reasoningAppend"
		outputChan = reasonningOutputChan
	}

	type streamBuf struct {
		content   strings.Builder
		startTime time.Time
	}
	// key: 发送方提供的 msg.MessageID（允许并发交错）
	streams := make(map[string]*streamBuf)

	emitDialogStreamEnd := func(mid string) {
		if eventname != "dialogAppend" || mid == "" {
			return
		}
		runtime.EventsEmit(a.ctx, "dialogStreamEnd", map[string]interface{}{
			"chatID":    chatID,
			"messageID": mid,
		})
	}

	for {
		select {
		case msg, ok := <-outputChan:
			if !ok {
				// channel 关闭时，尽力对未收口的流做收尾（不强制入库，避免半截内容污染）
				for mid := range streams {
					emitDialogStreamEnd(mid)
				}
				return
			}
			if msg == nil {
				continue
			}

			mid := strings.TrimSpace(msg.MessageID)
			if mid == "" {
				logging.Error("Output message missing MessageID, skipping (role=%s chatID=%s)", role, chatID)
				continue
			}

			buf, exists := streams[mid]
			if !exists {
				buf = &streamBuf{}
				streams[mid] = buf
			}
			if buf.startTime.IsZero() {
				buf.startTime = time.Now()
			}
			buf.content.WriteString(msg.Content)

			appendMessage := map[string]interface{}{
				"chatID":    chatID,
				"messageID": mid,
				"content":   msg.Content,
				"role":      role,
				// 与入库 startTime 一致，便于前端在重载前列顺序/展示时间
				"timestamp":    buf.startTime.UTC().Format(time.RFC3339Nano),
				"total_tokens": msg.TotalTokens,
				"agentID":      msg.AgentID,
			}
			runtime.EventsEmit(a.ctx, eventname, appendMessage)

			if !msg.IsFinished {
				continue
			}

			final := buf.content.String()
			toSave := final
			wasToolCompletePayload := false
			// 工具续问模式：落库前把 tool-complete JSON 净化为最终 content，避免刷新后看到控制 JSON。
			if role == utils.MessageRoleAssistant {
				if payload, ok := utils.ParseToolCompletePayload(final); ok {
					if s := strings.TrimSpace(payload.Content); s != "" {
						toSave = s
						wasToolCompletePayload = true
					}
				}
			}
			if strings.TrimSpace(final) != "" {
				if err := dataoperation.SendMessageWithCreateTimeAndTokens(msg.AgentID, chatID, mid, toSave, role, buf.startTime, msg.TotalTokens); err != nil {
					logging.Error("Failed to save message: %v", err)
				} else if wasToolCompletePayload {
					// 该条消息在流式阶段可能展示了控制 JSON；落库净化后，主动再 emit 一次 DB 最终内容用于界面替换。
					a.GetMessagesByMessageID(mid)
				}
			}
			emitDialogStreamEnd(mid)
			delete(streams, mid)

		case <-ctx.Done():
			for mid := range streams {
				emitDialogStreamEnd(mid)
			}
			logging.Info("App context cancelled for chat: %s", chatID)
			return
		}
	}
}

func (a *App) StopChat(chatID string) {
	a.poolMutex.Lock()
	defer a.poolMutex.Unlock()

	if dp, ok := a.agentPool[chatID]; ok {
		dp.Shutdown()
		a.restartDispatcherBackground(chatID, dp)
		logging.Info("已中断当前任务并保留 Dispatcher（上下文/意图仍在）chatID=%s", chatID)
	} else {
		logging.Info("No dispatcher found for chatID: %s to stop", chatID)
	}
}

func (a *App) SwitchChat(chatID string) {
	newID := strings.TrimSpace(chatID)
	logging.Info("Switching to chatID: %s", newID)

	a.switchMu.Lock()
	prev := a.lastActiveChatID
	if prev != "" && prev != newID {
		if err := memory.PersistLocalMemoryToYAMLFile(prev); err != nil {
			logging.Error("切换会话前写入本地记忆失败 chatID=%s: %v", prev, err)
		}
	}
	a.lastActiveChatID = newID
	shouldRestore := newID != "" && prev != newID && a.shouldRestoreLocalMemorySnapshot(newID)
	a.switchMu.Unlock()

	if shouldRestore {
		dispatcher.LoadLocalMemorySnapshotForChat(newID)
	}

	if newID != "" {
		a.dispatcher(newID)
	}
}

func (a *App) shouldRestoreLocalMemorySnapshot(chatID string) bool {
	cid := strings.TrimSpace(chatID)
	if cid == "" {
		return false
	}

	a.poolMutex.RLock()
	_, hasDispatcher := a.agentPool[cid]
	a.poolMutex.RUnlock()
	if hasDispatcher {
		return false
	}

	return len(memory.GetLocalMemory().GetMessages(cid)) == 0
}

// shutdown 应用退出时把当前会话的本地记忆写入 localmemory/{chatID}.yaml。
func (a *App) shutdown(_ context.Context) {
	a.switchMu.Lock()
	defer a.switchMu.Unlock()
	if a.lastActiveChatID != "" {
		if err := memory.PersistLocalMemoryToYAMLFile(a.lastActiveChatID); err != nil {
			logging.Error("退出时写入本地记忆失败 chatID=%s: %v", a.lastActiveChatID, err)
		}
	}
	if err := logging.CloseAll(); err != nil {
		fmt.Fprintf(os.Stderr, "shutdown logging flush failed: %v\n", err)
	}
}

// GetLLMConnectionStatus 加载配置并对首个 OpenAI 兼容后端请求 /v1/models（或 /models）做轻量探测。
func (a *App) GetLLMConnectionStatus() proxy.LLMConnectionStatus {
	return proxy.CheckLLMConnectionStatus(context.Background())
}

// GetLLMResolvedConfigPath 返回当前使用的配置文件绝对路径；未找到时为空。
func (a *App) GetLLMResolvedConfigPath() string {
	return proxy.GetResolvedConfigPath()
}

// GetLLMConfigEditorState 返回 YAML 全文、保存路径；若尚无配置文件则内容为示例，usingExample 为 true。
func (a *App) GetLLMConfigEditorState() (map[string]interface{}, error) {
	content, path, usingExample, err := proxy.ReadLLMConfigForUI()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"content":      content,
		"path":         path,
		"usingExample": usingExample,
	}, nil
}

// SaveLLMConfigText 校验并写入配置文件。
func (a *App) SaveLLMConfigText(content string) (string, error) {
	return proxy.SaveLLMConfigText(content)
}

// GetLLMConfigFormState 返回 LLM 配置的表格编辑数据（多后端列表；旧版仅 llm 时会合成一行展示）。
func (a *App) GetLLMConfigFormState() (proxy.LLMConfigFormState, error) {
	return proxy.GetLLMConfigFormState()
}

// SaveLLMConfigForm 将表格数据序列化为 YAML 并校验、写入。
func (a *App) SaveLLMConfigForm(primary proxy.LLMConfigRow, backends []proxy.LLMConfigRow) (string, error) {
	return proxy.SaveLLMConfigForm(primary, backends)
}

// GetMCPConfigFormState 返回 MCP 配置的表格编辑数据。
func (a *App) GetMCPConfigFormState() (proxy.MCPConfigFormState, error) {
	return proxy.GetMCPConfigFormState()
}

// SaveMCPConfigForm 将 MCP 表格数据序列化为 YAML 并写入，保留现有 LLM 配置。
func (a *App) SaveMCPConfigForm(servers []proxy.MCPConfigRow) (string, error) {
	return proxy.SaveMCPConfigForm(servers)
}

// ValidateMCPConfigRow 校验单条 MCP 配置并执行 tools/list，用于前端配置页状态展示。
func (a *App) ValidateMCPConfigRow(row proxy.MCPConfigRow) (proxy.MCPValidationResult, error) {
	return proxy.ValidateMCPConfigRow(row)
}

// GetMCPHubStatus 返回 MCP Hub 注册状态。
func (a *App) GetMCPHubStatus() (proxy.MCPHubStatus, error) {
	return proxy.GetMCPHubStatus()
}

// RegisterMCPHub 在 LobeHub Marketplace 注册当前设备身份。
func (a *App) RegisterMCPHub(name, description string) (proxy.MCPHubRegisterResult, error) {
	return proxy.RegisterMCPHub(name, description)
}

// SearchMCPHub 搜索 LobeHub Marketplace 中的 MCP 服务。
func (a *App) SearchMCPHub(query, category string, page, pageSize int) (proxy.MCPHubSearchResult, error) {
	return proxy.SearchMCPHub(query, category, page, pageSize)
}

// GetMCPHubPluginDetail 获取指定 MCP Hub 条目的详情与部署方式。
func (a *App) GetMCPHubPluginDetail(identifier string) (proxy.MCPHubPluginDetail, error) {
	return proxy.GetMCPHubPluginDetail(identifier)
}

// GetOpenClawSkillState 扫描当前工作区已安装的 OpenClaw/ClawHub skills。
func (a *App) GetOpenClawSkillState() proxy.OpenClawSkillState {
	return proxy.GetOpenClawSkillState()
}

// InstallOpenClawSkill 受控执行 ClawHub 安装命令，并将 skill 安装到当前工作区。
func (a *App) InstallOpenClawSkill(input string) (interface{}, error) {
	return proxy.InstallOpenClawSkill(context.Background(), input)
}

// DeleteOpenClawSkill 删除当前工作区 skills 目录下的指定 skill。
func (a *App) DeleteOpenClawSkill(path string) (interface{}, error) {
	return proxy.DeleteOpenClawSkill(path)
}

// InstallOpenClawSkillDeps 为指定 skill 创建隔离 Python venv 并安装依赖。
func (a *App) InstallOpenClawSkillDeps(path string) (interface{}, error) {
	return proxy.InstallOpenClawSkillDeps(path)
}

// GetMemoContent 读取备忘录全文（主存 SQLite；与 memo_write 工具共用同一存储）。
func (a *App) GetMemoContent() (string, error) {
	return memo.Read()
}

// GetMemoFilePath 返回备忘录 SQLite 库路径（主存储）；旧版 data/memo.md 会在首次打开时自动导入。
func (a *App) GetMemoFilePath() string {
	return memo.StoreDBPath()
}

// GetMemoReferencedMessageIDs 返回已在备忘录中标记过的对话 messageID（<!--leiAgent-memo-src:...-->）。
func (a *App) GetMemoReferencedMessageIDs() ([]string, error) {
	return memo.ReferencedMessageIDs()
}

// ComposeMemoWithLLM 根据对话摘录与用户提示调用 LLM 生成一条完整 Markdown 备忘。
func (a *App) ComposeMemoWithLLM(draftMarkdown, userHint string) (string, error) {
	return proxy.GenerateMemoFromDraft(context.Background(), draftMarkdown, userHint)
}

// AppendMemoMarkdown 在末尾追加一条或多条 # 分节（由 LLM 或界面生成）。
func (a *App) AppendMemoMarkdown(block string) error {
	return memo.AppendMarkdownRaw(block)
}

// SaveMemoContent 保存备忘录全文（覆盖写入）。
func (a *App) SaveMemoContent(content string) error {
	return memo.WriteAll(content)
}

// GetMemoCalendarDates 返回备忘录中出现的日历日期（YYYY-MM-DD），用于侧边栏日历上的「有备忘」标记。
func (a *App) GetMemoCalendarDates() ([]string, error) {
	s, err := memo.Read()
	if err != nil {
		return nil, err
	}
	return memo.CalendarDates(s), nil
}

// ListDocumentLibrary 返回文库中文档列表（file_write / write_file_chunk 登记 + 历史消息里出现过的现存文件路径）。
func (a *App) ListDocumentLibrary() ([]map[string]interface{}, error) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	bodies := dataoperation.GetAllMessageContentsForDocHarvest()
	return doclib.List(cwd, bodies)
}

// ReadDocumentForViewer 读取本地文件供文库/消息内链接预览（有大小上限）：文本为 UTF-8 字符串，PDF 为 base64。
func (a *App) ReadDocumentForViewer(path string) (map[string]interface{}, error) {
	return doclib.ReadForViewer(path)
}

// RevealDocumentInExplorer 在系统文件管理器中定位到该文件。
func (a *App) RevealDocumentInExplorer(path string) error {
	return doclib.RevealInFileManager(path)
}

// GetLibraryWorkspaceRoot 返回文库根目录（工作目录下 workspace/）的绝对路径。
func (a *App) GetLibraryWorkspaceRoot() (string, error) {
	return doclib.LibraryRootAbs()
}

// ListLibraryWorkspaceDir 列出文库内相对路径 rel 下的文件与文件夹（rel 为空表示根目录）。
func (a *App) ListLibraryWorkspaceDir(rel string) (map[string]interface{}, error) {
	root, listed, parent, entries, err := doclib.ListWorkspaceDir(rel)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"rootAbs":    root,
		"currentRel": listed,
		"parentRel":  parent,
		"entries":    entries,
	}, nil
}

// LibraryWorkspaceMkdir 在文库内创建目录（可多级）。
func (a *App) LibraryWorkspaceMkdir(rel string) error {
	_, err := doclib.WorkspaceMkdir(rel)
	return err
}

// LibraryWorkspaceWriteFile 在文库内覆盖写入文本文件。
func (a *App) LibraryWorkspaceWriteFile(rel string, content string) error {
	_, err := doclib.WorkspaceWriteFile(rel, content)
	return err
}

// LibraryWorkspaceDelete 删除文库内的文件或目录；recursive 为 true 时删除非空目录树。
func (a *App) LibraryWorkspaceDelete(rel string, recursive bool) error {
	return doclib.WorkspaceDelete(rel, recursive)
}

// LibraryWorkspaceRename 在文库根内移动或重命名。
func (a *App) LibraryWorkspaceRename(oldRel string, newRel string) error {
	return doclib.WorkspaceRename(oldRel, newRel)
}

// GetNovelResumeOutputDir 根据文库中选中的相对路径，若对应长篇小说工程（含 chapter_*.md）则返回用于 novel_longform 的 output_dir（相对 workspace），否则返回空字符串。
func (a *App) GetNovelResumeOutputDir(entryRel string, isDir bool) (string, error) {
	root, err := doclib.LibraryRootAbs()
	if err != nil {
		return "", err
	}
	return noveltool.ResumeOutputDirForLibraryEntry(root, entryRel, isDir)
}

// ResumeNovelLongform 从文库触发长篇小说续写：在后台以 resume=true 调用 novel_longform，模型流式输出写入当前会话。chapterCount<=0 时使用工具默认批次数。
func (a *App) ResumeNovelLongform(chatID, outputDirRel, premise, authorNotes string, chapterCount int) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return fmt.Errorf("请先选择或新建一个对话，续写将关联到当前会话")
	}
	root, err := doclib.LibraryRootAbs()
	if err != nil {
		return err
	}
	outRel := filepath.ToSlash(strings.TrimSpace(outputDirRel))
	if outRel == "" || !noveltool.CanResumeAtWorkspaceRel(root, outRel) {
		return fmt.Errorf("当前路径不是可续写的长篇目录（需含 chapter_*.md）")
	}
	premise = strings.TrimSpace(premise)
	if premise == "" {
		premise = "请根据已有章节、大纲与小说圣经续写后续内容，保持人物与伏笔一致。"
	}
	args := map[string]interface{}{
		"premise":    premise,
		"output_dir": outRel,
		"resume":     true,
	}
	if chapterCount > 0 {
		args["chapter_count"] = chapterCount
	}
	if notes := strings.TrimSpace(authorNotes); notes != "" {
		args["author_notes"] = notes
	}
	payload, err := json.Marshal(args)
	if err != nil {
		return err
	}

	a.dispatcher(chatID)
	tool := noveltool.New()
	go func() {
		ctx := context.WithValue(context.Background(), utils.ChatIDString, chatID)
		globalchannel.SendAssitantMessageOnce(ctx, "开始续写任务")
		result, runErr := tool.Execute(ctx, string(payload))
		if runErr != nil {
			globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("续写失败：%s", runErr.Error()))
			return
		}
		globalchannel.SendAssitantMessageOnce(ctx, fmt.Sprintf("续写完成: %s", result))
	}()
	return nil
}
