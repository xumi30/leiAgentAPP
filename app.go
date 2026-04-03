package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"leiAgent/dataoperation"
	"leiAgent/internal/dispatcher"
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
	if message == "" {
		logging.Info("Message is empty, not sending")
		runtime.EventsEmit(a.ctx, "sendMessageError", "messages is empty, not sending")
		return
	}
	//如果chatID为空，则生成一个新的chatID，并创建一个新的conversation
	if chatID == "" {
		chatID = a.AddConversation(message)
	}

	messageID := GenerateMessageID()

	//logging.Info("Sending message to conversation with ID: %s, messageID: %s, Message: %s, Role: %s", chatID, messageID, message, role)
	err := dataoperation.SendMessage(chatID, messageID, message, role)
	logging.Info("Sending message to conversation successfully")
	if err != nil {
		runtime.EventsEmit(a.ctx, "sendMessageError", err.Error())
		return
	}
	a.GetMessagesByMessageID(messageID)

	//logging.Info("Sending message to conversation with ID: %s, messageID: %s, Message: %s, Role: %s", chatID, messageID, message, role)

	dp := a.dispatcher(chatID)
	dp.InputChan <- message
	logging.Info("Sending message to conversation successfully")

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
		ctx = context.WithValue(ctx, "chatID", chatID)

		dp = dispatcher.NewDispatcher(ctx, cancel) // 传递 cancel 函数
		a.agentPool[chatID] = dp
		// 启动 并处理返回
		go dp.Run(ctx)
		go a.AppenAgentMessageToFrontRole(utils.MessageRoleAssistant, chatID)
		go a.AppenAgentMessageToFrontRole(utils.MessageRoleReasoning, chatID)
	}

	logging.Info("Getting dispatcher for conversation with ChatID: %s %v", chatID, dp)
	return dp
}

func (a *App) AppenAgentMessageToFrontRole(role, chatID string) {
	dp := a.dispatcher(chatID)
	outputChan := dp.DialogOutputChan
	eventname := "dialogAppend"

	if role == utils.MessageRoleReasoning {
		eventname = "reasoningAppend"
		outputChan = dp.ReasonningOutputChan
	}

	messageID := ""
	content := ""
	shouldGenerateNewID := true

	for {
		if shouldGenerateNewID {
			messageID = GenerateMessageID()
			content = ""
			shouldGenerateNewID = false
		}

		select {
		case message, ok := <-outputChan:
			if message == "" {
				logging.Info("Received empty message for chatID: %s, skipping...", chatID)
				continue
			}
			if !ok {
				logging.Info("Output channel closed for messageid: %s", messageID)
				return
			}

			if message == utils.FinishString {
				if err := dataoperation.SendMessage(chatID, messageID, content, role); err != nil {
					logging.Error("Failed to save message: %v", err)
				}
				shouldGenerateNewID = true
				continue
			}

			content = fmt.Sprintf("%s%s", content, message)
			appendMessage := map[string]interface{}{
				"chatID":    chatID,
				"messageID": messageID,
				"content":   message,
				"role":      "assistant",
			}

			runtime.EventsEmit(a.ctx, eventname, appendMessage)

		case <-a.ctx.Done():
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
		delete(a.agentPool, chatID)
		logging.Info("Stopped and removed dispatcher for chatID: %s", chatID)
	} else {
		logging.Info("No dispatcher found for chatID: %s to stop", chatID)
	}
}
