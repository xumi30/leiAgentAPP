package sqlmemory

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSQLMemoryMigratesLegacyMessagesAgentID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			messageID TEXT NOT NULL UNIQUE,
			chatID TEXT NOT NULL,
			role TEXT NOT NULL CHECK(role IN ('user', 'assistant', 'system', 'reasoning')),
			content TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("create legacy messages table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	mem, err := NewSQLMemory(dbPath)
	if err != nil {
		t.Fatalf("new sql memory: %v", err)
	}
	defer mem.Close()

	const chatID = "1778753904689149"
	if err := mem.SaveConversation(chatID, "chat title", "chat"); err != nil {
		t.Fatalf("save conversation: %v", err)
	}
	if err := mem.SaveMessageWithTimestamp(chatID, "msg-1", "agent-1", "assistant", "hello", time.Now(), 7); err != nil {
		t.Fatalf("save message after migration: %v", err)
	}

	messages, err := mem.GetMessages(chatID)
	if err != nil {
		t.Fatalf("get messages after migration: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if got := messages[0]["agentID"]; got != "agent-1" {
		t.Fatalf("agentID = %v, want agent-1", got)
	}
	if got := messages[0]["total_tokens"]; got != 7 {
		t.Fatalf("total_tokens = %v, want 7", got)
	}
}
