package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

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
				WithPromptListUpdater(&mockPromptListWatcher{}),
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
		id:            "test-session",
		ctx:           context.Background(),
		formatMsgFunc: nopFormatMsgFunc,
		msgSentHook:   nopMsgSentHook,
		writer:        writer,
		writeTimeout:  time.Second,
	}

	srv := &mockServer{}
	s := newServer(srv, WithServerWriteTimeout(time.Second))

	s.sessions.Store(sess.id, sess)

	// Test ping message
	pingMsg := `{"jsonrpc": "2.0", "method": "ping", "id": "1"}`
	err := s.handleMsg(bytes.NewReader([]byte(pingMsg)), sess.id)
	if err != nil {
		t.Fatalf("handleMsg failed: %v", err)
	}

	// handleMsg is using a goroutine, so we need to wait a little bit
	time.Sleep(2 * time.Second)

	var response JSONRPCMessage
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
	s := newServer(srv, WithServerWriteTimeout(time.Second))
	s.start()

	writer := &mockWriter{}
	ctx := context.Background()

	s.startSession(ctx, writer, nopFormatMsgFunc, nopMsgSentHook)

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

func TestServerListRoots(t *testing.T) {
	writer := &mockWriter{}
	srv := &mockServer{}
	s := newServer(srv, WithServerWriteTimeout(time.Second), WithServerReadTimeout(time.Second))

	ctx := context.Background()
	sessID := s.startSession(ctx, writer, nopFormatMsgFunc, nopMsgSentHook)

	// Start goroutine to handle mock response
	go func() {
		// Loop until we get a request
		var msg JSONRPCMessage
		for {
			wbs := writer.getWritten()
			if len(wbs) == 0 {
				continue
			}
			err := json.NewDecoder(bytes.NewReader(wbs)).Decode(&msg)
			if err != nil {
				t.Errorf("failed to decode request: %v", err)
				return
			}
			break
		}

		// Verify request
		if msg.Method != MethodRootsList {
			t.Errorf("expected method %s, got %s", MethodRootsList, msg.Method)
		}

		// Send mock response
		mockResponse := RootList{
			Roots: []Root{
				{URI: "test://root1", Name: "Root 1"},
				{URI: "test://root2", Name: "Root 2"},
			},
		}
		mockBs, err := json.Marshal(mockResponse)
		if err != nil {
			t.Errorf("failed to marshal mock response: %v", err)
		}

		responseMsg := JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			ID:      msg.ID,
			Result:  mockBs,
		}

		responseBs, _ := json.Marshal(responseMsg)
		err = s.handleMsg(bytes.NewReader(responseBs), sessID)
		if err != nil {
			t.Errorf("handleMsg failed: %v", err)
		}
	}()

	// Call listRoots
	ctx = ctxWithSessionID(ctx, sessID)
	result, err := s.listRoots(ctx)
	if err != nil {
		t.Fatalf("listRoots failed: %v", err)
	}

	// Verify result
	if len(result.Roots) != 2 {
		t.Errorf("expected 2 roots, got %d", len(result.Roots))
	}
	if result.Roots[0].URI != "test://root1" || result.Roots[0].Name != "Root 1" {
		t.Errorf("unexpected root[0]: %+v", result.Roots[0])
	}
	if result.Roots[1].URI != "test://root2" || result.Roots[1].Name != "Root 2" {
		t.Errorf("unexpected root[1]: %+v", result.Roots[1])
	}
}

func TestServerRequestSampling(t *testing.T) {
	writer := &mockWriter{}
	srv := &mockServer{}
	s := newServer(srv, WithServerWriteTimeout(time.Second), WithServerReadTimeout(time.Second))

	ctx := context.Background()
	sessID := s.startSession(ctx, writer, nopFormatMsgFunc, nopMsgSentHook)

	// Start goroutine to handle mock response
	go func() {
		// Loop until we get a request
		var msg JSONRPCMessage
		for {
			wbs := writer.getWritten()
			if len(wbs) == 0 {
				continue
			}
			err := json.NewDecoder(bytes.NewReader(wbs)).Decode(&msg)
			if err != nil {
				t.Errorf("failed to decode request: %v", err)
				return
			}
			break
		}

		// Verify request
		if msg.Method != MethodSamplingCreateMessage {
			t.Errorf("expected method %s, got %s", MethodSamplingCreateMessage, msg.Method)
		}

		// Verify params
		var receivedParams SamplingParams
		if err := json.Unmarshal(msg.Params, &receivedParams); err != nil {
			t.Errorf("failed to decode params: %v", err)
			return
		}

		// Send mock response
		mockResponse := SamplingResult{
			Role: PromptRoleAssistant,
			Content: SamplingContent{
				Type: "text",
				Text: "Hello! How can I help you?",
			},
			Model:      "test-model",
			StopReason: "completed",
		}
		mockBs, err := json.Marshal(mockResponse)
		if err != nil {
			t.Errorf("failed to marshal mock response: %v", err)
		}

		responseMsg := JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			ID:      msg.ID,
			Result:  mockBs,
		}

		responseBs, _ := json.Marshal(responseMsg)
		err = s.handleMsg(bytes.NewReader(responseBs), sessID)
		if err != nil {
			t.Errorf("handleMsg failed: %v", err)
		}
	}()

	// Call createSampleMessage
	ctx = ctxWithSessionID(ctx, sessID)
	// Test params
	result, err := s.requestSampling(ctx, SamplingParams{
		Messages: []SamplingMessage{
			{
				Role: PromptRoleUser,
				Content: SamplingContent{
					Type: "text",
					Text: "Hello",
				},
			},
		},
		ModelPreferences: SamplingModelPreferences{
			CostPriority:         1,
			SpeedPriority:        2,
			IntelligencePriority: 3,
		},
		SystemPrompts: "Be helpful",
		MaxTokens:     100,
	})
	if err != nil {
		t.Fatalf("createSampleMessage failed: %v", err)
	}

	// Verify result
	if result.Role != PromptRoleAssistant {
		t.Errorf("expected role %s, got %s", PromptRoleAssistant, result.Role)
	}
	if result.Content.Text != "Hello! How can I help you?" {
		t.Errorf("unexpected content text: %s", result.Content.Text)
	}
	if result.Model != "test-model" {
		t.Errorf("expected model test-model, got %s", result.Model)
	}
	if result.StopReason != "completed" {
		t.Errorf("expected stop reason completed, got %s", result.StopReason)
	}
}

func (m *mockServer) Info() Info {
	return Info{Name: "test-server", Version: "1.0"}
}

func (m *mockServer) RequireSamplingClient() bool {
	return false
}
