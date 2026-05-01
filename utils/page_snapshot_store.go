package utils

import (
	"sync"
	"time"
)

type PageSnapshot struct {
	URL       string    `json:"url"`
	HTML      string    `json:"html"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// chatID -> PageSnapshot
var chatPageSnapshotStore sync.Map

func GetPageSnapshot(chatID string) (PageSnapshot, bool) {
	if IsBlank(chatID) {
		return PageSnapshot{}, false
	}
	v, ok := chatPageSnapshotStore.Load(chatID)
	if !ok {
		return PageSnapshot{}, false
	}
	s, ok := v.(PageSnapshot)
	if !ok {
		return PageSnapshot{}, false
	}
	return s, true
}

func SetPageSnapshot(chatID string, snap PageSnapshot) {
	if IsBlank(chatID) {
		return
	}
	chatPageSnapshotStore.Store(chatID, snap)
}

func DeletePageSnapshot(chatID string) {
	if IsBlank(chatID) {
		return
	}
	chatPageSnapshotStore.Delete(chatID)
}
