package proxy

import (
	"strings"
	"testing"
)

func TestStreamScannerCompletedWhenDoneReceived(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n"
	scanner := NewStreamScanner(strings.NewReader(input))

	var count int
	for scanner.Scan() {
		count++
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner.Err() = %v, want nil", err)
	}
	if count != 1 {
		t.Fatalf("scanner.Scan() count = %d, want 1", count)
	}
	if !scanner.Completed() {
		t.Fatalf("scanner.Completed() = false, want true")
	}
}

func TestStreamScannerNotCompletedWhenStreamEndsWithoutDone(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n"
	scanner := NewStreamScanner(strings.NewReader(input))

	for scanner.Scan() {
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner.Err() = %v, want nil", err)
	}
	if scanner.Completed() {
		t.Fatalf("scanner.Completed() = true, want false")
	}
}
