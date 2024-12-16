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

type mockPromptServer struct {
	listPromptsCalled bool
	getPromptCalled   bool
}

type mockPromptListWatcher struct {
	ch chan struct{}
}

type mockResourceServer struct {
	listResourcesCalled bool
	readResourceCalled  bool
	listTemplatesCalled bool
	subscribeCalled     bool
	uri                 string
}

type mockToolServer struct {
	listToolsCalled bool
	callToolCalled  bool
	toolName        string
}

type mockServer struct{}

func TestNewSessionDispatcher(t *testing.T) {
	tests := []struct {
		name     string
		options  []ServerOption
		expected ServerCapabilities
	}{
		{
			name:     "empty dispatcher",
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
			server := &mockServer{}
			dispatcher := newSessionDispatcher(server, tt.options...)
			if !reflect.DeepEqual(tt.expected, dispatcher.capabilities) {
				t.Errorf("got capabilities %+v, want %+v", dispatcher.capabilities, tt.expected)
			}
		})
	}
}

func TestSessionHandlePing(t *testing.T) {
	writer := &mockWriter{}
	sess := &session{
		id:           "test-session",
		ctx:          context.Background(),
		writter:      writer,
		writeTimeout: time.Second,
	}

	err := sess.handlePing("1")
	if err != nil {
		t.Fatalf("handlePing failed: %v", err)
	}

	var response jsonRPCMessage
	err = json.NewDecoder(bytes.NewReader(writer.getWritten())).Decode(&response)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.JSONRPC != "2.0" {
		t.Errorf("got JSONRPC version %s, want 2.0", response.JSONRPC)
	}
	if response.ID != MustString("1") {
		t.Errorf("got ID %v, want 1", response.ID)
	}
	if response.Error != nil {
		t.Errorf("got error %v, want nil", response.Error)
	}
}

func TestSessionHandleInitialize(t *testing.T) {
	writer := &mockWriter{}
	sess := &session{
		id:           "test-session",
		ctx:          context.Background(),
		writter:      writer,
		writeTimeout: time.Second,
	}

	serverInfo := Info{Name: "test-server", Version: "1.0"}
	serverCap := ServerCapabilities{
		Prompts: &PromptsCapability{ListChanged: true},
	}
	requiredClientCap := ClientCapabilities{}

	params := initializeParams{
		ProtocolVersion: protocolVersion,
		ClientInfo:      Info{Name: "test-client", Version: "1.0"},
		Capabilities:    ClientCapabilities{},
	}

	err := sess.handleInitialize("1", params, serverCap, requiredClientCap, serverInfo)
	if err != nil {
		t.Fatalf("handleInitialize failed: %v", err)
	}

	var response jsonRPCMessage
	err = json.NewDecoder(bytes.NewReader(writer.getWritten())).Decode(&response)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	var result initializeResult
	err = json.Unmarshal(response.Result, &result)
	if err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if result.ProtocolVersion != protocolVersion {
		t.Errorf("got protocol version %s, want %s", result.ProtocolVersion, protocolVersion)
	}
	if !reflect.DeepEqual(result.ServerInfo, serverInfo) {
		t.Errorf("got server info %+v, want %+v", result.ServerInfo, serverInfo)
	}
	if !reflect.DeepEqual(result.Capabilities, serverCap) {
		t.Errorf("got capabilities %+v, want %+v", result.Capabilities, serverCap)
	}
}

func TestSessionDispatcherStart(t *testing.T) {
	server := &mockServer{}
	dispatcher := newSessionDispatcher(server, WithWriteTimeout(time.Second))

	dispatcher.start()

	if dispatcher.sessionStopChan == nil {
		t.Error("sessionStopChan is nil")
	}
	if dispatcher.closeChan == nil {
		t.Error("closeChan is nil")
	}

	// Clean up
	dispatcher.stop()
}

func TestSessionDispatcherHandleMsg(t *testing.T) {
	writer := &mockWriter{}
	sess := &session{
		id:           "test-session",
		ctx:          context.Background(),
		writter:      writer,
		writeTimeout: time.Second,
	}

	server := &mockServer{}
	dispatcher := newSessionDispatcher(server, WithWriteTimeout(time.Second))
	dispatcher.sessions.Store(sess.id, sess)

	// Test ping message
	pingMsg := `{"jsonrpc": "2.0", "method": "ping", "id": "1"}`
	err := dispatcher.handleMsg(bytes.NewReader([]byte(pingMsg)), sess.id)
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

func TestSessionDispatcherStartSession(t *testing.T) {
	dispatcher := sessionDispatcher{
		sessions:        &sync.Map{},
		progresses:      &sync.Map{},
		sessionStopChan: make(chan string),
		closeChan:       make(chan struct{}),
		writeTimeout:    time.Second,
	}

	writer := &mockWriter{}
	ctx := context.Background()

	dispatcher.startSession(ctx, writer)

	var sessionCount int
	dispatcher.sessions.Range(func(_, _ interface{}) bool {
		sessionCount++
		return true
	})

	if sessionCount != 1 {
		t.Errorf("got %d sessions, want 1", sessionCount)
	}

	// Clean up
	dispatcher.stop()
}

func TestSessionHandlePrompts(t *testing.T) {
	writer := &mockWriter{}
	promptServer := &mockPromptServer{}

	server := &mockServer{}
	dispatcher := newSessionDispatcher(server,
		WithPromptServer(promptServer),
		WithWriteTimeout(time.Second),
	)
	sessID := dispatcher.startSession(context.Background(), writer)

	initSession(t, &dispatcher, sessID)

	// Test ListPrompts
	listParams := promptsListParams{
		Cursor: "",
	}
	listParamsBs, _ := json.Marshal(listParams)
	listMsg := jsonRPCMessage{
		JSONRPC: jsonRPCVersion,
		Method:  methodPromptsList,
		ID:      MustString("1"),
		Params:  listParamsBs,
	}
	listBs, _ := json.Marshal(listMsg)
	err := dispatcher.handleMsg(bytes.NewReader(listBs), sessID)
	if err != nil {
		t.Fatalf("handleMsg failed: %v", err)
	}
	if !promptServer.listPromptsCalled {
		t.Error("ListPrompts was not called")
	}

	// Test GetPrompt
	//nolint:lll
	getMsg := `
{"jsonrpc": "2.0", "method": "prompts/get", "id": "2", "params": {"name": "test-prompt", "arguments": {}, "_meta": {"progressToken": "123"}}}
`
	err = dispatcher.handleMsg(bytes.NewReader([]byte(getMsg)), sessID)
	if err != nil {
		t.Fatalf("handleMsg failed: %v", err)
	}
	if !promptServer.getPromptCalled {
		t.Error("GetPrompt was not called")
	}
}

func TestSessionHandleResources(t *testing.T) {
	writer := &mockWriter{}
	resourceServer := &mockResourceServer{}

	server := &mockServer{}
	dispatcher := newSessionDispatcher(server,
		WithResourceServer(resourceServer),
		WithWriteTimeout(time.Second),
	)
	sessID := dispatcher.startSession(context.Background(), writer)

	initSession(t, &dispatcher, sessID)

	// Test ListResources
	listParams := resourcesListParams{
		Cursor: "",
	}
	listParamsBs, _ := json.Marshal(listParams)
	listMsg := jsonRPCMessage{
		JSONRPC: jsonRPCVersion,
		Method:  methodResourcesList,
		ID:      MustString("1"),
		Params:  listParamsBs,
	}
	listBs, _ := json.Marshal(listMsg)
	err := dispatcher.handleMsg(bytes.NewReader(listBs), sessID)
	if err != nil {
		t.Fatalf("handleMsg failed: %v", err)
	}
	if !resourceServer.listResourcesCalled {
		t.Error("ListResources was not called")
	}

	// Test ReadResource
	readParams := resourcesReadParams{
		URI: "test-uri",
	}
	readParamsBs, _ := json.Marshal(readParams)
	readMsg := jsonRPCMessage{
		JSONRPC: jsonRPCVersion,
		ID:      MustString("2"),
		Method:  methodResourcesRead,
		Params:  readParamsBs,
	}
	readBs, _ := json.Marshal(readMsg)
	err = dispatcher.handleMsg(bytes.NewReader(readBs), sessID)
	if err != nil {
		t.Fatalf("handleMsg failed: %v", err)
	}
	if !resourceServer.readResourceCalled {
		t.Error("ReadResource was not called")
	}

	// Test Subscribe
	subscribeParams := resourcesSubscribeParams{
		URI: "test-uri",
	}
	subscribeParamsBs, _ := json.Marshal(subscribeParams)
	subscribeMsg := jsonRPCMessage{
		JSONRPC: jsonRPCVersion,
		ID:      MustString("3"),
		Method:  methodResourcesSubscribe,
		Params:  subscribeParamsBs,
	}
	subscribeBs, _ := json.Marshal(subscribeMsg)
	err = dispatcher.handleMsg(bytes.NewReader(subscribeBs), sessID)
	if err != nil {
		t.Fatalf("handleMsg failed: %v", err)
	}
	if !resourceServer.subscribeCalled {
		t.Error("SubscribeResource was not called")
	}
	if resourceServer.uri != "test-uri" {
		t.Errorf("got uri %s, want test-uri", resourceServer.uri)
	}
}

func TestSessionHandleTools(t *testing.T) {
	writer := &mockWriter{}
	toolServer := &mockToolServer{}

	server := &mockServer{}
	dispatcher := newSessionDispatcher(server,
		WithToolServer(toolServer),
		WithWriteTimeout(time.Second),
	)
	sessID := dispatcher.startSession(context.Background(), writer)

	initSession(t, &dispatcher, sessID)

	// Test ListTools
	listParams := jsonRPCMessage{
		JSONRPC: jsonRPCVersion,
		Method:  methodToolsList,
		ID:      MustString("1"),
		Params:  []byte{},
	}
	listParamsBs, _ := json.Marshal(listParams)
	listMsg := jsonRPCMessage{
		JSONRPC: jsonRPCVersion,
		Method:  methodToolsList,
		ID:      MustString("1"),
		Params:  listParamsBs,
	}
	listBs, _ := json.Marshal(listMsg)
	err := dispatcher.handleMsg(bytes.NewReader(listBs), sessID)
	if err != nil {
		t.Fatalf("handleMsg failed: %v", err)
	}
	if !toolServer.listToolsCalled {
		t.Error("ListTools was not called")
	}

	// Test CallTool
	callParams := toolsCallParams{
		Name: "test-tool",
		Arguments: map[string]any{
			"test-arg": "test-value",
		},
	}
	callParamsBs, _ := json.Marshal(callParams)
	callMsg := jsonRPCMessage{
		JSONRPC: jsonRPCVersion,
		ID:      MustString("2"),
		Method:  methodToolsCall,
		Params:  callParamsBs,
	}
	callBs, _ := json.Marshal(callMsg)
	err = dispatcher.handleMsg(bytes.NewReader(callBs), sessID)
	if err != nil {
		t.Fatalf("handleMsg failed: %v", err)
	}
	if !toolServer.callToolCalled {
		t.Error("CallTool was not called")
	}
	if toolServer.toolName != "test-tool" {
		t.Errorf("got tool name %s, want test-tool", toolServer.toolName)
	}
}

func initSession(t *testing.T, dispatcher *sessionDispatcher, sessID string) {
	// Initialize session
	initParams := initializeParams{
		ProtocolVersion: protocolVersion,
		ClientInfo:      Info{Name: "test-client", Version: "1.0"},
		Capabilities:    ClientCapabilities{},
	}
	initParamsBs, _ := json.Marshal(initParams)
	initMsg := jsonRPCMessage{
		JSONRPC: jsonRPCVersion,
		Method:  methodInitialize,
		ID:      MustString("1"),
		Params:  initParamsBs,
	}
	initBs, _ := json.Marshal(initMsg)
	err := dispatcher.handleMsg(bytes.NewReader(initBs), sessID)
	if err != nil {
		t.Fatalf("handleMsg failed: %v", err)
	}
	initNotifMsg := jsonRPCMessage{
		JSONRPC: jsonRPCVersion,
		Method:  methodNotificationsInitialized,
		ID:      MustString("2"),
	}
	initNotifBs, _ := json.Marshal(initNotifMsg)
	err = dispatcher.handleMsg(bytes.NewReader(initNotifBs), sessID)
	if err != nil {
		t.Fatalf("handleMsg failed: %v", err)
	}
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

func (m *mockPromptServer) ListPrompts(context.Context, string, MustString) (PromptList, error) {
	m.listPromptsCalled = true
	return PromptList{
		Prompts: []Prompt{
			{Name: "test-prompt"},
		},
	}, nil
}

func (m *mockPromptServer) GetPrompt(_ context.Context, name string, _ map[string]any, _ MustString) (Prompt, error) {
	m.getPromptCalled = true
	return Prompt{Name: name}, nil
}

func (m *mockPromptServer) CompletesPrompt(context.Context, string, CompletionArgument) (CompletionResult, error) {
	return CompletionResult{}, nil
}

func (m *mockPromptListWatcher) WatchPromptList() <-chan struct{} {
	if m.ch == nil {
		m.ch = make(chan struct{})
	}
	return m.ch
}

func (m *mockResourceServer) ListResources(_ context.Context, cursor string, _ MustString) (*ResourceList, error) {
	m.listResourcesCalled = true
	return &ResourceList{
		Resources: []Resource{
			{URI: "test-resource", Name: "Test Resource"},
		},
		NextCursor: cursor,
	}, nil
}

func (m *mockResourceServer) ReadResource(_ context.Context, uri string, _ MustString) (*Resource, error) {
	m.readResourceCalled = true
	return &Resource{URI: uri, Name: "Test Resource"}, nil
}

func (m *mockResourceServer) ListResourceTemplates(_ context.Context, _ MustString) ([]ResourceTemplate, error) {
	m.listTemplatesCalled = true
	return []ResourceTemplate{
		{URITemplate: "test-template", Name: "Test Template"},
	}, nil
}

func (m *mockResourceServer) SubscribeResource(uri string) {
	m.subscribeCalled = true
	m.uri = uri
}

func (m *mockResourceServer) CompletesResourceTemplate(
	_ context.Context,
	_ string,
	_ CompletionArgument,
) (CompletionResult, error) {
	return CompletionResult{}, nil
}

func (m *mockToolServer) ListTools(_ context.Context, cursor string, _ MustString) (*ToolList, error) {
	m.listToolsCalled = true
	return &ToolList{
		Tools: []*Tool{
			{Name: "test-tool", Description: "Test Tool"},
		},
		NextCursor: cursor,
	}, nil
}

func (m *mockToolServer) CallTool(_ context.Context, name string, _ map[string]any, _ MustString) (ToolResult, error) {
	m.callToolCalled = true
	m.toolName = name
	return ToolResult{
		Content: []Content{{Type: ContentTypeText, Text: "Tool executed"}},
		IsError: false,
	}, nil
}

func (m *mockServer) Info() Info {
	return Info{Name: "test-server", Version: "1.0"}
}

func (m *mockServer) RequiredClientCapabilities() ClientCapabilities {
	return ClientCapabilities{}
}
