// Package shellapproval gates local shell execution behind an optional UI approval handshake.
package shellapproval

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
)

var (
	mu       sync.Mutex
	pending  = map[string]*waiter{}
	NotifyUI func(chatID, requestID, command string) // App startup wires Wails EventsEmit here
)

var (
	// ErrDenied 用户在界面选择拒绝。
	ErrDenied = errors.New("shell command denied by user")
	// ErrUnknownRequest 无效的 request id 或已处理完毕。
	ErrUnknownRequest = errors.New("shell approval request not found")
	// ErrChatMismatch request 不属于该 chat。
	ErrChatMismatch = errors.New("shell approval chat mismatch")
)

type waiter struct {
	chatID string
	ch     chan bool
}

func autoApproveEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("LEIAGENT_SHELL_AUTO_APPROVE")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// WaitApprove 阻塞直至用户批准、拒绝或上下文取消。
// 批准返回 nil；拒绝返回 ErrDenied；取消返回 ctx.Err。
func WaitApprove(ctx context.Context, chatID, requestID, command string) error {
	rid := strings.TrimSpace(requestID)
	if rid == "" {
		return errors.New("empty shell approval request id")
	}
	if autoApproveEnv() {
		return nil
	}

	cid := strings.TrimSpace(chatID)
	ch := make(chan bool, 1)
	mu.Lock()
	pending[rid] = &waiter{chatID: cid, ch: ch}
	mu.Unlock()

	defer func() {
		mu.Lock()
		delete(pending, rid)
		mu.Unlock()
	}()

	if NotifyUI != nil {
		NotifyUI(cid, rid, command)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case ok := <-ch:
		if ok {
			return nil
		}
		return ErrDenied
	}
}

// Respond 由前端在用户点击「允许」/「拒绝」后调用。
func Respond(chatID, requestID string, approve bool) error {
	mu.Lock()
	w, ok := pending[strings.TrimSpace(requestID)]
	if !ok {
		mu.Unlock()
		return ErrUnknownRequest
	}
	if w.chatID != strings.TrimSpace(chatID) {
		mu.Unlock()
		return ErrChatMismatch
	}
	delete(pending, strings.TrimSpace(requestID))
	mu.Unlock()

	select {
	case w.ch <- approve:
	default:
	}
	return nil
}
