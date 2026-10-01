// Package logbus — журнал для интерфейса: хранит последние строки и рассылает новые подписчикам.
package logbus

import (
	"strings"
	"sync"
)

type Bus struct {
	mu    sync.Mutex
	lines []string
	subs  map[chan string]struct{}
	max   int
}

func New(max int) *Bus { return &Bus{subs: map[chan string]struct{}{}, max: max} }

func (b *Bus) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\r\n")
	b.mu.Lock()
	b.lines = append(b.lines, line)
	if len(b.lines) > b.max {
		b.lines = b.lines[len(b.lines)-b.max:]
	}
	for c := range b.subs {
		select {
		case c <- line:
		default: // медленный клиент — пропускаем строку
		}
	}
	b.mu.Unlock()
	return len(p), nil
}

func (b *Bus) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.lines...)
}

func (b *Bus) Subscribe() chan string {
	c := make(chan string, 64)
	b.mu.Lock()
	b.subs[c] = struct{}{}
	b.mu.Unlock()
	return c
}

func (b *Bus) Unsubscribe(c chan string) {
	b.mu.Lock()
	delete(b.subs, c)
	b.mu.Unlock()
}
