package mcp_test

import (
	"sync"
)

type mockWriter struct {
	sync.RWMutex
	written []byte
}

func (w *mockWriter) Write(p []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	w.written = append(w.written, p...)
	return len(p), nil
}

func (w *mockWriter) getWritten() []byte {
	w.RLock()
	defer w.RUnlock()
	return w.written
}
