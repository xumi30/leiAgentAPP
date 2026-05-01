package shellapproval

import (
	"context"
	"errors"
	"testing"
	"time"
)

type approvalNotice struct {
	chatID    string
	requestID string
	command   string
}

func prepareApprovalTest(t *testing.T) chan approvalNotice {
	t.Helper()
	t.Setenv("LEIAGENT_SHELL_AUTO_APPROVE", "")

	mu.Lock()
	pending = map[string]*waiter{}
	mu.Unlock()

	originalNotify := NotifyUI
	notices := make(chan approvalNotice, 1)
	NotifyUI = func(chatID, requestID, command string) {
		notices <- approvalNotice{chatID: chatID, requestID: requestID, command: command}
	}
	t.Cleanup(func() {
		NotifyUI = originalNotify
		mu.Lock()
		pending = map[string]*waiter{}
		mu.Unlock()
	})
	return notices
}

func waitForApprovalNotice(t *testing.T, notices <-chan approvalNotice) approvalNotice {
	t.Helper()
	select {
	case notice := <-notices:
		return notice
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for shell approval notification")
		return approvalNotice{}
	}
}

func TestWaitApproveRequiresMatchingChatBeforeApproval(t *testing.T) {
	notices := prepareApprovalTest(t)
	result := make(chan error, 1)
	go func() {
		result <- WaitApprove(context.Background(), "chat-a", "request-1", "echo ok")
	}()

	notice := waitForApprovalNotice(t, notices)
	if notice.chatID != "chat-a" || notice.requestID != "request-1" || notice.command != "echo ok" {
		t.Fatalf("unexpected approval notice: %+v", notice)
	}
	if err := Respond("chat-b", "request-1", true); !errors.Is(err, ErrChatMismatch) {
		t.Fatalf("mismatched chat Respond() error = %v, want %v", err, ErrChatMismatch)
	}
	if err := Respond("chat-a", "request-1", true); err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if err := <-result; err != nil {
		t.Fatalf("WaitApprove() error = %v", err)
	}
	if err := Respond("chat-a", "request-1", true); !errors.Is(err, ErrUnknownRequest) {
		t.Fatalf("second Respond() error = %v, want %v", err, ErrUnknownRequest)
	}
}

func TestWaitApproveReturnsDenied(t *testing.T) {
	notices := prepareApprovalTest(t)
	result := make(chan error, 1)
	go func() {
		result <- WaitApprove(context.Background(), "chat-a", "request-2", "rm file")
	}()

	waitForApprovalNotice(t, notices)
	if err := Respond("chat-a", "request-2", false); err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if err := <-result; !errors.Is(err, ErrDenied) {
		t.Fatalf("WaitApprove() error = %v, want %v", err, ErrDenied)
	}
}

func TestWaitApproveCancellationCleansPendingRequest(t *testing.T) {
	notices := prepareApprovalTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- WaitApprove(ctx, "chat-a", "request-3", "long command")
	}()

	waitForApprovalNotice(t, notices)
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitApprove() error = %v, want %v", err, context.Canceled)
	}
	if err := Respond("chat-a", "request-3", true); !errors.Is(err, ErrUnknownRequest) {
		t.Fatalf("Respond() after cancellation error = %v, want %v", err, ErrUnknownRequest)
	}
}
