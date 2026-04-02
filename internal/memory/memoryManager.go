package memory

type MemoryManager interface {
	AddMessage(chatID string, message *Message)
	GetMessages(chatID string) []*Message
	Clear(chatID string)
}
