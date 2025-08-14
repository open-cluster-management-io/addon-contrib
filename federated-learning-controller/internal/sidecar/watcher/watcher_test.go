package watcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileWatcher(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "watcher_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.txt")

	fw, err := New(testFile)
	if err != nil {
		t.Fatalf("Failed to create new watcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	contentChan := fw.Start(ctx)

	go func() {
		defer cancel() // Cancel context when goroutine finishes

		// Give watcher a moment to start up
		time.Sleep(100 * time.Millisecond)

		// First operation: create file with "hello"
		if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
			t.Errorf("Failed to write initial file: %v", err)
			return
		}

		// Give watcher a moment to process the event
		time.Sleep(100 * time.Millisecond)

		// Second operation: append " world"
		f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			t.Errorf("Failed to open file for append: %v", err)
			return
		}
		if _, err := f.WriteString(" world"); err != nil {
			f.Close()
			t.Errorf("Failed to append to file: %v", err)
			return
		}
		f.Close()

		// Give watcher a final moment to process the last event
		time.Sleep(100 * time.Millisecond)
	}()

	var receivedContents []string
	for content := range contentChan {
		receivedContents = append(receivedContents, string(content))
	}

	// Deduplicate the received contents to handle filesystem event quirks
	var distinctContents []string
	if len(receivedContents) > 0 {
		distinctContents = append(distinctContents, receivedContents[0])
		for i := 1; i < len(receivedContents); i++ {
			if receivedContents[i] != receivedContents[i-1] {
				distinctContents = append(distinctContents, receivedContents[i])
			}
		}
	}

	// Now check the distinct contents
	expected := []string{"hello", "hello world"}

	if len(distinctContents) != len(expected) {
		t.Fatalf("Expected %d distinct content updates, but got %d. Received: %s", len(expected), len(distinctContents), strings.Join(receivedContents, ", "))
	}

	for i, want := range expected {
		if distinctContents[i] != want {
			t.Errorf("Update %d: expected content '%s', got '%s'", i+1, want, distinctContents[i])
		}
	}
}