package utils

import "sync"

// chatID -> sessionID (e.g. Playwright session)
var chatSessionStore sync.Map

func GetSessionID(chatID string) (string, bool) {
	if IsBlank(chatID) {
		return "", false
	}
	v, ok := chatSessionStore.Load(chatID)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok || IsBlank(s) {
		return "", false
	}
	return s, true
}

func SetSessionID(chatID, sessionID string) {
	if IsBlank(chatID) || IsBlank(sessionID) {
		return
	}
	chatSessionStore.Store(chatID, sessionID)
}

func DeleteSessionID(chatID string) {
	if IsBlank(chatID) {
		return
	}
	chatSessionStore.Delete(chatID)
}

