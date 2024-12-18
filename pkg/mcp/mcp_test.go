package mcp

import "sync"

type mockWriter struct {
	sync.Mutex
	written []byte
}

func (w *mockWriter) Write(p []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	w.written = append(w.written, p...)
	return len(p), nil
}

func (w *mockWriter) getWritten() []byte {
	w.Lock()
	defer w.Unlock()
	return w.written
}
