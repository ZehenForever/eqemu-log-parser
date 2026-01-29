package tail

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestTailer_PartialLineBufferedUntilNewline(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(p, []byte(""), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	tr, err := NewTailer(p, TailOptions{StartAtEnd: false, PollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewTailer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	lines := make(chan string, 4)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = tr.Run(ctx, func(line string) { lines <- line })
	}()

	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString("hello"); err != nil {
		t.Fatalf("write partial: %v", err)
	}
	f.Sync()

	select {
	case got := <-lines:
		t.Fatalf("unexpected line before newline: %q", got)
	case <-time.After(60 * time.Millisecond):
	}

	if _, err := f.WriteString("\r\n"); err != nil {
		t.Fatalf("write newline: %v", err)
	}
	f.Sync()

	select {
	case got := <-lines:
		if got != "hello" {
			t.Fatalf("line=%q want=hello", got)
		}
	case <-ctx.Done():
		t.Fatalf("timeout")
	}

	cancel()
	wg.Wait()
}

func TestTailer_TruncationResetsOffset(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(p, []byte("a\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	tr, err := NewTailer(p, TailOptions{StartAtEnd: false, PollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewTailer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	lines := make(chan string, 8)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = tr.Run(ctx, func(line string) { lines <- line })
	}()

	// Expect initial "a"
	select {
	case got := <-lines:
		if got != "a" {
			t.Fatalf("line=%q want=a", got)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for initial line")
	}

	// Append another line
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := f.WriteString("b\n"); err != nil {
		t.Fatalf("append: %v", err)
	}
	f.Close()

	select {
	case got := <-lines:
		if got != "b" {
			t.Fatalf("line=%q want=b", got)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for appended line")
	}

	// Truncate and write new content
	if err := os.Truncate(p, 0); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f2, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	if _, err := f2.WriteString("c\n"); err != nil {
		t.Fatalf("append after truncate: %v", err)
	}
	f2.Close()

	select {
	case got := <-lines:
		if got != "c" {
			t.Fatalf("line=%q want=c", got)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for line after truncate")
	}

	cancel()
	wg.Wait()
}
