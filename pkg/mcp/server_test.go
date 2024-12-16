package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"
)

type mockWriter struct {
	sync.Mutex
	written []byte
}

type mockServer struct{}

func TestNewServer(t *testing.T) {
	tests := []struct {
		name     string
		options  []ServerOption
		expected ServerCapabilities
	}{
		{
			name:     "empty server",
			options:  nil,
			expected: ServerCapabilities{},
		},
		{
			name: "with prompt server",
			options: []ServerOption{
				WithPromptServer(&mockPromptServer{}),
			},
			expected: ServerCapabilities{
				Prompts: &PromptsCapability{},
			},
		},
		{
			name: "with prompt server and watcher",
			options: []ServerOption{
				WithPromptServer(&mockPromptServer{}),
				WithPromptListWatcher(&mockPromptListWatcher{}),
			},
			expected: ServerCapabilities{
				Prompts: &PromptsCapability{
					ListChanged: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &mockServer{}
			s := newServer(srv, tt.options...)
			if !reflect.DeepEqual(tt.expected, s.capabilities) {
				t.Errorf("got capabilities %+v, want %+v", s.capabilities, tt.expected)
			}
		})
	}
}

func TestServerStart(t *testing.T) {
	srv := &mockServer{}
	s := newServer(srv)

	s.start()

	if s.sessionStopChan == nil {
		t.Error("sessionStopChan is nil")
	}
	if s.closeChan == nil {
		t.Error("closeChan is nil")
	}

	// Clean up
	s.stop()
}

func TestServerHandleMsg(t *testing.T) {
	writer := &mockWriter{}
	sess := &serverSession{
		id:           "test-session",
		ctx:          context.Background(),
		writter:      writer,
		writeTimeout: time.Second,
	}

	srv := &mockServer{}
	s := newServer(srv, WithWriteTimeout(time.Second))

	s.sessions.Store(sess.id, sess)

	// Test ping message
	pingMsg := `{"jsonrpc": "2.0", "method": "ping", "id": "1"}`
	err := s.handleMsg(bytes.NewReader([]byte(pingMsg)), sess.id)
	if err != nil {
		t.Fatalf("handleMsg failed: %v", err)
	}

	var response jsonRPCMessage
	err = json.NewDecoder(bytes.NewReader(writer.getWritten())).Decode(&response)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.ID != MustString("1") {
		t.Errorf("got ID %v, want 1", response.ID)
	}
}

func TestServerStartSession(t *testing.T) {
	srv := &mockServer{}
	s := newServer(srv, WithWriteTimeout(time.Second))
	s.start()

	writer := &mockWriter{}
	ctx := context.Background()

	s.startSession(ctx, writer)

	var sessionCount int
	s.sessions.Range(func(_, _ interface{}) bool {
		sessionCount++
		return true
	})

	if sessionCount != 1 {
		t.Errorf("got %d sessions, want 1", sessionCount)
	}

	// Clean up
	s.stop()
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

func (m *mockServer) Info() Info {
	return Info{Name: "test-server", Version: "1.0"}
}

func (m *mockServer) RequiredClientCapabilities() ClientCapabilities {
	return ClientCapabilities{}
}
