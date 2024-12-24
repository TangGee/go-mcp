package mcp_test

import (
	"context"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
	"github.com/qri-io/jsonschema"
)

type mockServer struct {
	requireRootsListClient bool
	requireSamplingClient  bool
}

type mockPromptServer struct{}

type mockResourceServer struct{}

type mockToolServer struct{}

type mockLogHandler struct{}

type mockRootsListWatcher struct{}

func (m mockServer) Info() mcp.Info {
	return mcp.Info{Name: "test-server", Version: "1.0"}
}

func (m mockServer) RequireRootsListClient() bool {
	return m.requireRootsListClient
}

func (m mockServer) RequireSamplingClient() bool {
	return m.requireSamplingClient
}

func (m mockPromptServer) ListPrompts(context.Context, mcp.PromptsListParams) (mcp.PromptList, error) {
	return mcp.PromptList{
		Prompts: []mcp.Prompt{
			{Name: "test-prompt"},
		},
	}, nil
}

func (m mockPromptServer) GetPrompt(context.Context, mcp.PromptsGetParams) (mcp.PromptResult, error) {
	return mcp.PromptResult{
		Description: "Test Prompt",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.PromptRoleAssistant,
				Content: mcp.Content{
					Type: mcp.ContentTypeText,
					Text: "Test response message",
				},
			},
		},
	}, nil
}

func (m mockPromptServer) CompletesPrompt(context.Context, mcp.CompletionCompleteParams) (mcp.CompletionResult, error) {
	return mcp.CompletionResult{
		Completion: struct {
			Values  []string `json:"values"`
			HasMore bool     `json:"hasMore"`
		}{
			Values:  []string{"test-value1", "test-value2"},
			HasMore: true,
		},
	}, nil
}

func (m mockResourceServer) ListResources(context.Context, mcp.ResourcesListParams) (mcp.ResourceList, error) {
	return mcp.ResourceList{
		Resources: []mcp.Resource{
			{
				URI:         "test://resource1",
				Name:        "Test Resource 1",
				Description: "First test resource",
				MimeType:    "text/plain",
				Text:        "Resource 1 content",
			},
			{
				URI:         "test://resource2",
				Name:        "Test Resource 2",
				Description: "Second test resource",
				MimeType:    "application/json",
				Text:        "{\"key\": \"value\"}",
			},
		},
	}, nil
}

func (m mockResourceServer) ReadResource(context.Context, mcp.ResourcesReadParams) (mcp.Resource, error) {
	return mcp.Resource{
		URI:         "test://resource",
		Name:        "Test Resource",
		Description: "A test resource",
		MimeType:    "text/plain",
		Text:        "This is the resource content",
	}, nil
}

func (m mockResourceServer) ListResourceTemplates(context.Context, mcp.ResourcesTemplatesListParams) ([]mcp.ResourceTemplate, error) {
	return []mcp.ResourceTemplate{
		{
			URITemplate: "test://resource/{name}",
			Name:        "Test Template 1",
			Description: "First test template",
			MimeType:    "text/plain",
		},
		{
			URITemplate: "test://resource/{id}",
			Name:        "Test Template 2",
			Description: "Second test template",
			MimeType:    "application/json",
		},
	}, nil
}

func (m mockResourceServer) SubscribeResource(params mcp.ResourcesSubscribeParams) {
}

func (m mockResourceServer) UnsubscribeResource(params mcp.ResourcesSubscribeParams) {
}

func (m mockResourceServer) CompletesResourceTemplate(
	_ context.Context,
	_ mcp.CompletionCompleteParams,
) (mcp.CompletionResult, error) {
	return mcp.CompletionResult{}, nil
}

func (m mockToolServer) ListTools(context.Context, mcp.ToolsListParams) (mcp.ToolList, error) {
	return mcp.ToolList{
		Tools: []mcp.Tool{
			{
				Name:        "test-tool",
				Description: "Test Tool",
				InputSchema: jsonschema.Must(`{
            "type": "object",
            "properties": {
              "param1": { "type": "string" }
            }
          }`),
			},
		},
	}, nil
}

func (m mockToolServer) CallTool(context.Context, mcp.ToolsCallParams) (mcp.ToolResult, error) {
	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: "Tool execution result",
			},
			{
				Type:     mcp.ContentTypeImage,
				Data:     "base64data",
				MimeType: "image/png",
			},
		},
		IsError: false,
	}, nil
}

func (m mockLogHandler) LogStreams() <-chan mcp.LogParams {
	return nil
}

func (m mockLogHandler) SetLogLevel(level mcp.LogLevel) {
}

func (m mockRootsListWatcher) OnRootsListChanged() {
}

// func TestNewServer(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		options  []ServerOption
// 		expected ServerCapabilities
// 	}{
// 		{
// 			name:     "empty server",
// 			options:  nil,
// 			expected: ServerCapabilities{},
// 		},
// 		{
// 			name: "with prompt server",
// 			options: []ServerOption{
// 				WithPromptServer(&mockPromptServer{}),
// 			},
// 			expected: ServerCapabilities{
// 				Prompts: &PromptsCapability{},
// 			},
// 		},
// 		{
// 			name: "with prompt server and watcher",
// 			options: []ServerOption{
// 				WithPromptServer(&mockPromptServer{}),
// 				WithPromptListUpdater(&mockPromptListWatcher{}),
// 			},
// 			expected: ServerCapabilities{
// 				Prompts: &PromptsCapability{
// 					ListChanged: true,
// 				},
// 			},
// 		},
// 	}
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			srv := &mockServer{}
// 			s := newServer(srv, tt.options...)
// 			if !reflect.DeepEqual(tt.expected, s.capabilities) {
// 				t.Errorf("got capabilities %+v, want %+v", s.capabilities, tt.expected)
// 			}
// 		})
// 	}
// }
//
// func TestServerStart(t *testing.T) {
// 	srv := &mockServer{}
// 	s := newServer(srv)
//
// 	s.start()
//
// 	if s.sessionStopChan == nil {
// 		t.Error("sessionStopChan is nil")
// 	}
// 	if s.closeChan == nil {
// 		t.Error("closeChan is nil")
// 	}
//
// 	// Clean up
// 	s.stop()
// }
//
// func TestServerHandleMsg(t *testing.T) {
// 	writer := &mockWriter{}
// 	sess := &serverSession{
// 		id:            "test-session",
// 		ctx:           context.Background(),
// 		formatMsgFunc: nopFormatMsgFunc,
// 		msgSentHook:   nopMsgSentHook,
// 		writer:        writer,
// 		writeTimeout:  time.Second,
// 	}
//
// 	srv := &mockServer{}
// 	s := newServer(srv, WithServerWriteTimeout(time.Second))
//
// 	s.sessions.Store(sess.id, sess)
//
// 	// Test ping message
// 	pingMsg := `{"jsonrpc": "2.0", "method": "ping", "id": "1"}`
// 	err := s.handleMsg(bytes.NewReader([]byte(pingMsg)), sess.id)
// 	if err != nil {
// 		t.Fatalf("handleMsg failed: %v", err)
// 	}
//
// 	// handleMsg is using a goroutine, so we need to wait a little bit
// 	time.Sleep(2 * time.Second)
//
// 	var response JSONRPCMessage
// 	err = json.NewDecoder(bytes.NewReader(writer.getWritten())).Decode(&response)
// 	if err != nil {
// 		t.Fatalf("failed to decode response: %v", err)
// 	}
// 	if response.ID != MustString("1") {
// 		t.Errorf("got ID %v, want 1", response.ID)
// 	}
// }
//
// func TestServerStartSession(t *testing.T) {
// 	srv := &mockServer{}
// 	s := newServer(srv, WithServerWriteTimeout(time.Second))
// 	s.start()
//
// 	writer := &mockWriter{}
// 	ctx := context.Background()
//
// 	s.startSession(ctx, writer, nopFormatMsgFunc, nopMsgSentHook)
//
// 	var sessionCount int
// 	s.sessions.Range(func(_, _ interface{}) bool {
// 		sessionCount++
// 		return true
// 	})
//
// 	if sessionCount != 1 {
// 		t.Errorf("got %d sessions, want 1", sessionCount)
// 	}
//
// 	// Clean up
// 	s.stop()
// }
//
// func TestServerListRoots(t *testing.T) {
// 	writer := &mockWriter{}
// 	srv := &mockServer{}
// 	s := newServer(srv, WithServerWriteTimeout(time.Second), WithServerReadTimeout(time.Second))
//
// 	ctx := context.Background()
// 	sessID := s.startSession(ctx, writer, nopFormatMsgFunc, nopMsgSentHook)
//
// 	// Start goroutine to handle mock response
// 	go func() {
// 		// Loop until we get a request
// 		var msg JSONRPCMessage
// 		for {
// 			wbs := writer.getWritten()
// 			if len(wbs) == 0 {
// 				continue
// 			}
// 			err := json.NewDecoder(bytes.NewReader(wbs)).Decode(&msg)
// 			if err != nil {
// 				t.Errorf("failed to decode request: %v", err)
// 				return
// 			}
// 			break
// 		}
//
// 		// Verify request
// 		if msg.Method != MethodRootsList {
// 			t.Errorf("expected method %s, got %s", MethodRootsList, msg.Method)
// 		}
//
// 		// Send mock response
// 		mockResponse := RootList{
// 			Roots: []Root{
// 				{URI: "test://root1", Name: "Root 1"},
// 				{URI: "test://root2", Name: "Root 2"},
// 			},
// 		}
// 		mockBs, err := json.Marshal(mockResponse)
// 		if err != nil {
// 			t.Errorf("failed to marshal mock response: %v", err)
// 		}
//
// 		responseMsg := JSONRPCMessage{
// 			JSONRPC: JSONRPCVersion,
// 			ID:      msg.ID,
// 			Result:  mockBs,
// 		}
//
// 		responseBs, _ := json.Marshal(responseMsg)
// 		err = s.handleMsg(bytes.NewReader(responseBs), sessID)
// 		if err != nil {
// 			t.Errorf("handleMsg failed: %v", err)
// 		}
// 	}()
//
// 	// Call listRoots
// 	ctx = ctxWithSessionID(ctx, sessID)
// 	result, err := s.listRoots(ctx)
// 	if err != nil {
// 		t.Fatalf("listRoots failed: %v", err)
// 	}
//
// 	// Verify result
// 	if len(result.Roots) != 2 {
// 		t.Errorf("expected 2 roots, got %d", len(result.Roots))
// 	}
// 	if result.Roots[0].URI != "test://root1" || result.Roots[0].Name != "Root 1" {
// 		t.Errorf("unexpected root[0]: %+v", result.Roots[0])
// 	}
// 	if result.Roots[1].URI != "test://root2" || result.Roots[1].Name != "Root 2" {
// 		t.Errorf("unexpected root[1]: %+v", result.Roots[1])
// 	}
// }
//
// func TestServerRequestSampling(t *testing.T) {
// 	writer := &mockWriter{}
// 	srv := &mockServer{}
// 	s := newServer(srv, WithServerWriteTimeout(time.Second), WithServerReadTimeout(time.Second))
//
// 	ctx := context.Background()
// 	sessID := s.startSession(ctx, writer, nopFormatMsgFunc, nopMsgSentHook)
//
// 	// Start goroutine to handle mock response
// 	go func() {
// 		// Loop until we get a request
// 		var msg JSONRPCMessage
// 		for {
// 			wbs := writer.getWritten()
// 			if len(wbs) == 0 {
// 				continue
// 			}
// 			err := json.NewDecoder(bytes.NewReader(wbs)).Decode(&msg)
// 			if err != nil {
// 				t.Errorf("failed to decode request: %v", err)
// 				return
// 			}
// 			break
// 		}
//
// 		// Verify request
// 		if msg.Method != MethodSamplingCreateMessage {
// 			t.Errorf("expected method %s, got %s", MethodSamplingCreateMessage, msg.Method)
// 		}
//
// 		// Verify params
// 		var receivedParams SamplingParams
// 		if err := json.Unmarshal(msg.Params, &receivedParams); err != nil {
// 			t.Errorf("failed to decode params: %v", err)
// 			return
// 		}
//
// 		// Send mock response
// 		mockResponse := SamplingResult{
// 			Role: PromptRoleAssistant,
// 			Content: SamplingContent{
// 				Type: "text",
// 				Text: "Hello! How can I help you?",
// 			},
// 			Model:      "test-model",
// 			StopReason: "completed",
// 		}
// 		mockBs, err := json.Marshal(mockResponse)
// 		if err != nil {
// 			t.Errorf("failed to marshal mock response: %v", err)
// 		}
//
// 		responseMsg := JSONRPCMessage{
// 			JSONRPC: JSONRPCVersion,
// 			ID:      msg.ID,
// 			Result:  mockBs,
// 		}
//
// 		responseBs, _ := json.Marshal(responseMsg)
// 		err = s.handleMsg(bytes.NewReader(responseBs), sessID)
// 		if err != nil {
// 			t.Errorf("handleMsg failed: %v", err)
// 		}
// 	}()
//
// 	// Call createSampleMessage
// 	ctx = ctxWithSessionID(ctx, sessID)
// 	// Test params
// 	result, err := s.requestSampling(ctx, SamplingParams{
// 		Messages: []SamplingMessage{
// 			{
// 				Role: PromptRoleUser,
// 				Content: SamplingContent{
// 					Type: "text",
// 					Text: "Hello",
// 				},
// 			},
// 		},
// 		ModelPreferences: SamplingModelPreferences{
// 			CostPriority:         1,
// 			SpeedPriority:        2,
// 			IntelligencePriority: 3,
// 		},
// 		SystemPrompts: "Be helpful",
// 		MaxTokens:     100,
// 	})
// 	if err != nil {
// 		t.Fatalf("createSampleMessage failed: %v", err)
// 	}
//
// 	// Verify result
// 	if result.Role != PromptRoleAssistant {
// 		t.Errorf("expected role %s, got %s", PromptRoleAssistant, result.Role)
// 	}
// 	if result.Content.Text != "Hello! How can I help you?" {
// 		t.Errorf("unexpected content text: %s", result.Content.Text)
// 	}
// 	if result.Model != "test-model" {
// 		t.Errorf("expected model test-model, got %s", result.Model)
// 	}
// 	if result.StopReason != "completed" {
// 		t.Errorf("expected stop reason completed, got %s", result.StopReason)
// 	}
// }
