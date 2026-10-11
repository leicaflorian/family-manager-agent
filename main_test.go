package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
)

// TestMain keeps off the screen what the agent would write in agent.log (logf).
func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

// logBook holds what the agent would write in agent.log during a test.
type logBook struct {
	mu   sync.Mutex
	text bytes.Buffer
}

func (b *logBook) Write(line []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.Write(line)
}

// lines are the ones that hold the text given.
func (b *logBook) lines(holding string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	found := []string{}
	for _, line := range strings.Split(b.text.String(), "\n") {
		if strings.Contains(line, holding) {
			found = append(found, line)
		}
	}
	return found
}

// logged collects the log of the agent until the end of the test.
func logged(t *testing.T) *logBook {
	t.Helper()
	book := &logBook{}
	log.SetOutput(book)
	t.Cleanup(func() { log.SetOutput(io.Discard) })
	return book
}
