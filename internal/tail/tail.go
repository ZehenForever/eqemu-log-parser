package tail

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type TailOptions struct {
	StartAtEnd   bool
	PollInterval time.Duration
}

type Tailer struct {
	path string
	opts TailOptions

	mu      sync.Mutex
	file    *os.File
	offset  int64
	buf     []byte
	stopped bool
}

func NewTailer(path string, opts TailOptions) (*Tailer, error) {
	if path == "" {
		return nil, errors.New("tail: empty path")
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 100 * time.Millisecond
	}
	return &Tailer{path: path, opts: opts}, nil
}

func (t *Tailer) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return nil
	}
	t.stopped = true
	if t.file != nil {
		err := t.file.Close()
		t.file = nil
		return err
	}
	return nil
}

func (t *Tailer) Run(ctx context.Context, onLine func(line string)) error {
	if onLine == nil {
		return errors.New("tail: onLine is nil")
	}

	f, err := os.Open(t.path)
	if err != nil {
		return err
	}
	defer f.Close()

	t.mu.Lock()
	t.file = f
	t.stopped = false
	t.mu.Unlock()

	if t.opts.StartAtEnd {
		off, err := f.Seek(0, io.SeekEnd)
		if err != nil {
			return err
		}
		t.offset = off
	} else {
		off, err := f.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}
		t.offset = off
	}

	readBuf := make([]byte, 32*1024)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		fi, err := f.Stat()
		if err != nil {
			return err
		}
		if fi.Size() < t.offset {
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
			t.offset = 0
			t.buf = t.buf[:0]
		}

		n, rerr := f.Read(readBuf)
		if n > 0 {
			t.offset += int64(n)
			t.buf = append(t.buf, readBuf[:n]...)
			for {
				idx := indexByte(t.buf, '\n')
				if idx < 0 {
					break
				}
				lineBytes := t.buf[:idx]
				if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\r' {
					lineBytes = lineBytes[:len(lineBytes)-1]
				}
				if len(lineBytes) > 0 {
					onLine(string(lineBytes))
				}
				t.buf = t.buf[idx+1:]
			}

			if len(t.buf) > 0 {
				// Make sure we don't keep references to old large buffers.
				left := make([]byte, len(t.buf))
				copy(left, t.buf)
				t.buf = left
			}
		}

		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				time.Sleep(t.opts.PollInterval)
				continue
			}
			return rerr
		}

		if n == 0 {
			time.Sleep(t.opts.PollInterval)
			continue
		}
	}
}

func indexByte(b []byte, c byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}

func stripTrailingCR(s string) string {
	return strings.TrimSuffix(s, "\r")
}
