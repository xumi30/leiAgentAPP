package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"leiAgent/dataoperation"
	globalchannel "leiAgent/internal"
	"leiAgent/internal/dispatcher"
	"leiAgent/internal/doclib"
	"leiAgent/internal/memo"
	"leiAgent/internal/proxy"
	"leiAgent/logging"
	"leiAgent/utils"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx context.Context

	agentPool map[string]*dispatcher.Dispatcher
	poolMutex sync.RWMutex
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.agentPool = make(map[string]*dispatcher.Dispatcher)
	a.ctx = ctx
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

func (a *App) DeleteConversation(chatID string) {
	logging.Info("Deleting conversation	 with ID: %s", chatID)

	err := dataoperation.DeleteConversation(chatID)
	if err != nil {
		runtime.EventsEmit(a.ctx, "deleteConversationError", err.Error())
		return
	}
	logging.Info("Conversation with ID: %s deleted successfully", chatID)
	runtime.EventsEmit(a.ctx, "deleteConversationSuccess", chatID)
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
	//如果chatID为空，则生成一个新的chatID，并创建一个新的conversation
	if chatID == "" {
		logging.Info("ChatID is empty, adding new conversation")
		title := proxy.GenerateConversationTitle(context.Background(), message)
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
	err := dataoperation.SendMessage(chatID, messageID, message, role)
	logging.Info("Sending message to conversation successfully")
	if err != nil {
		runtime.EventsEmit(a.ctx, "sendMessageError", err.Error())
		return
	}
	a.GetMessagesByMessageID(messageID)

	// 仅用户消息进入 Dispatcher，避免助手/推理等内容若误走 SendMessage 时再次触发意图识别与死循环。
	if strings.EqualFold(strings.TrimSpace(role), utils.MessageRoleUser) {
		inputChan <- message
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
func GenerateMessageID() string {

	messageID := fmt.Sprintf("%d%06d", time.Now().UnixMilli(), rand.Intn(100000))
	logging.Info("Generated messageID: %s", messageID)
	return messageID
}

// SetLLMThinkingDisabled 为 true 时，后续发往 LLM 的请求会关闭思考/推理（如百炼 Qwen 的 enable_thinking）。
func (a *App) SetLLMThinkingDisabled(disabled bool) {
	proxy.SetLLMThinkingDisabled(disabled)
}

// GetLLMThinkingDisabled 返回当前是否对 LLM 关闭了思考过程。
func (a *App) GetLLMThinkingDisabled() bool {
	return proxy.IsLLMThinkingDisabled()
}

func (a *App) GetReasoningMessage(chatID string) []map[string]interface{} {
	reasonings, err := dataoperation.GetReasonings(chatID)
	if err != nil {
		runtime.EventsEmit(a.ctx, "getReasoningMessageError", err.Error())
		return nil
	}
	//logging.Info("Getting reasoning messages for conversation with ID: %s %v", chatID, reasonings)
	return reasonings
}

func (a *App) dispatcher(chatID string) *dispatcher.Dispatcher {
	a.poolMutex.Lock()
	defer a.poolMutex.Unlock()

	if a.agentPool == nil {
		a.agentPool = make(map[string]*dispatcher.Dispatcher)
	}

	dp, ok := a.agentPool[chatID]
	if !ok {
		if len(a.agentPool) >= 5 {
			logging.Info("agentPool is full, cleaning up oldest agent...")
			var oldestKey string
			for k := range a.agentPool {
				oldestKey = k
				break
			}
			if oldestKey != "" {
				if oldDp, exists := a.agentPool[oldestKey]; exists {
					oldDp.Shutdown()
					delete(a.agentPool, oldestKey)
				}
			}
		}

		// 使用可取消的 context
		ctx, cancel := context.WithCancel(context.Background())
		ctx = context.WithValue(ctx, utils.ChatIDString, chatID)

		var err error
		dp, err = dispatcher.NewDispatcher(ctx, chatID, cancel) // 传递 cancel 函数
		if err != nil {
			logging.Error("创建 Dispatcher 失败: %v", err)
			runtime.EventsEmit(a.ctx, "dispatcherError", err.Error())
			cancel()
			return nil
		}
		a.agentPool[chatID] = dp
		// 启动 并处理返回
		logging.Info("Starting dispatcher for conversation with ChatID: %s", chatID)
		go dp.Run(ctx)
		go a.AppenAgentMessageToFrontRole(ctx, utils.MessageRoleAssistant, chatID)
		go a.AppenAgentMessageToFrontRole(ctx, utils.MessageRoleReasoning, chatID)
	}

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
	logging.Info("Dispatcher 已重新监听 input/output，chatID=%s（保留原 Dispatcher 与意图）", chatID)
}

func (a *App) AppenAgentMessageToFrontRole(ctx context.Context, role, chatID string) {
	outputChan := globalchannel.GetGlobalDialogOutChannel(chatID)
	reasonningOutputChan := globalchannel.GetGlobalReasonOutChannel(chatID)
	eventname := "dialogAppend"

	if role == utils.MessageRoleReasoning {
		eventname = "reasoningAppend"
		outputChan = reasonningOutputChan
	}

	messageID := ""
	content := ""
	shouldGenerateNewID := true
	// 流式首段到达时刻，作为入库 timestamp，避免仅用回合结束写入时间导致多条助手消息排序错乱
	var streamStartTime time.Time

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
		if shouldGenerateNewID {
			messageID = GenerateMessageID()
			content = ""
			streamStartTime = time.Time{}
			shouldGenerateNewID = false
		}

		select {
		case message, ok := <-outputChan:
			if !ok {
				logging.Info("Output channel closed for messageid: %s", messageID)
				emitDialogStreamEnd(messageID)
				return
			}
			if message == "" {
				logging.Info("Received empty message for chatID: %s, skipping...", chatID)
				continue
			}

			if message == utils.FinishString || message == utils.FinishStringEphemeral {
				// 流式回合可能只有 tool_calls、无可见文本；仍会收到 FinishString。
				// FinishStringEphemeral：仅展示（如小说工具内部多轮），不写入对话 DB，也不进入后续 LLM 记忆。
				ephemeral := message == utils.FinishStringEphemeral
				if strings.TrimSpace(content) != "" && !ephemeral {
					ts := streamStartTime
					if ts.IsZero() {
						ts = time.Now()
					}
					if err := dataoperation.SendMessageWithCreateTime(chatID, messageID, content, role, ts); err != nil {
						logging.Error("Failed to save message: %v", err)
					}
				}
				emitDialogStreamEnd(messageID)
				shouldGenerateNewID = true
				continue
			}

			if streamStartTime.IsZero() {
				streamStartTime = time.Now()
			}
			content = fmt.Sprintf("%s%s", content, message)
			appendMessage := map[string]interface{}{
				"chatID":    chatID,
				"messageID": messageID,
				"content":   message,
				"role":      role,
				// 与入库 streamStartTime 一致，便于前端在重载前列顺序/展示时间
				"timestamp": streamStartTime.UTC().Format(time.RFC3339Nano),
			}

			runtime.EventsEmit(a.ctx, eventname, appendMessage)

		case <-ctx.Done():
			emitDialogStreamEnd(messageID)
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
	logging.Info("Switching to chatID: %s", chatID)
	a.dispatcher(chatID)
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

// ReadDocumentForViewer 读取本地文本文件供文库/消息内链接预览（有大小上限）。
func (a *App) ReadDocumentForViewer(path string) (map[string]interface{}, error) {
	text, err := doclib.ReadText(path)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"path":    filepath.Clean(path),
		"content": text,
	}, nil
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
