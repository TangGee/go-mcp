package mcp

import (
	"context"
)

type mockPromptServer struct {
	listPromptsCalled bool
	getPromptCalled   bool
}

type mockPromptListWatcher struct {
	ch chan struct{}
}

// type mockResourceServer struct {
// 	listResourcesCalled bool
// 	readResourceCalled  bool
// 	listTemplatesCalled bool
// 	subscribeCalled     bool
// 	uri                 string
// }
//
// type mockSamplingHandler struct {
// 	createMessageCalled bool
// }
//
// func (m *mockSamplingHandler) CreateSampleMessage(context.Context, SamplingParams) (SamplingResult, error) {
// 	m.createMessageCalled = true
// 	return SamplingResult{
// 		Role: PromptRoleAssistant,
// 		Content: SamplingContent{
// 			Type: "text",
// 			Text: "Test response",
// 		},
// 		Model:      "test-model",
// 		StopReason: "completed",
// 	}, nil
// }
//
// type mockToolServer struct {
// 	listToolsCalled bool
// 	callToolCalled  bool
// 	toolName        string
// }

// func TestServerSessionHandlePing(t *testing.T) {
// 	writer := &mockWriter{}
// 	sess := &serverSession{
// 		id:            "test-session",
// 		ctx:           context.Background(),
// 		writer:        writer,
// 		writeTimeout:  time.Second,
// 		pingInterval:  time.Second,
// 		formatMsgFunc: nopFormatMsgFunc,
// 		msgSentHook:   nopMsgSentHook,
// 	}
//
// 	err := sess.handlePing("1")
// 	if err != nil {
// 		t.Fatalf("handlePing failed: %v", err)
// 	}
//
// 	var response JSONRPCMessage
// 	err = json.NewDecoder(bytes.NewReader(writer.getWritten())).Decode(&response)
// 	if err != nil {
// 		t.Fatalf("failed to decode response: %v", err)
// 	}
//
// 	if response.JSONRPC != "2.0" {
// 		t.Errorf("got JSONRPC version %s, want 2.0", response.JSONRPC)
// 	}
// 	if response.ID != MustString("1") {
// 		t.Errorf("got ID %v, want 1", response.ID)
// 	}
// 	if response.Error != nil {
// 		t.Errorf("got error %v, want nil", response.Error)
// 	}
// }
//
// func TestServerSessionHandleInitialize(t *testing.T) {
// 	writer := &mockWriter{}
// 	sess := &serverSession{
// 		id:            "test-session",
// 		ctx:           context.Background(),
// 		writer:        writer,
// 		writeTimeout:  time.Second,
// 		formatMsgFunc: nopFormatMsgFunc,
// 		msgSentHook:   nopMsgSentHook,
// 	}
//
// 	serverInfo := Info{Name: "test-server", Version: "1.0"}
// 	serverCap := ServerCapabilities{
// 		Prompts: &PromptsCapability{ListChanged: true},
// 	}
// 	requiredClientCap := ClientCapabilities{}
//
// 	params := initializeParams{
// 		ProtocolVersion: protocolVersion,
// 		ClientInfo:      Info{Name: "test-client", Version: "1.0"},
// 		Capabilities:    ClientCapabilities{},
// 	}
//
// 	err := sess.handleInitialize("1", params, serverCap, requiredClientCap, serverInfo)
// 	if err != nil {
// 		t.Fatalf("handleInitialize failed: %v", err)
// 	}
//
// 	var response JSONRPCMessage
// 	err = json.NewDecoder(bytes.NewReader(writer.getWritten())).Decode(&response)
// 	if err != nil {
// 		t.Fatalf("failed to decode response: %v", err)
// 	}
//
// 	var result initializeResult
// 	err = json.Unmarshal(response.Result, &result)
// 	if err != nil {
// 		t.Fatalf("failed to unmarshal result: %v", err)
// 	}
//
// 	if result.ProtocolVersion != protocolVersion {
// 		t.Errorf("got protocol version %s, want %s", result.ProtocolVersion, protocolVersion)
// 	}
// 	if !reflect.DeepEqual(result.ServerInfo, serverInfo) {
// 		t.Errorf("got server info %+v, want %+v", result.ServerInfo, serverInfo)
// 	}
// 	if !reflect.DeepEqual(result.Capabilities, serverCap) {
// 		t.Errorf("got capabilities %+v, want %+v", result.Capabilities, serverCap)
// 	}
// }
//
// func TestServerSessionHandlePrompts(t *testing.T) {
// 	writer := &mockWriter{}
// 	promptServer := &mockPromptServer{}
//
// 	srv := &mockServer{}
// 	s := newServer(srv,
// 		WithPromptServer(promptServer),
// 		WithServerWriteTimeout(time.Second),
// 	)
// 	sessID := s.startSession(context.Background(), writer, nopFormatMsgFunc, nopMsgSentHook)
//
// 	initServerSession(t, &s, sessID)
//
// 	// Test ListPrompts
// 	listParams := PromptsListParams{
// 		Cursor: "",
// 	}
// 	listParamsBs, _ := json.Marshal(listParams)
// 	listMsg := JSONRPCMessage{
// 		JSONRPC: JSONRPCVersion,
// 		Method:  MethodPromptsList,
// 		ID:      MustString("1"),
// 		Params:  listParamsBs,
// 	}
// 	listBs, _ := json.Marshal(listMsg)
// 	err := s.handleMsg(bytes.NewReader(listBs), sessID)
// 	if err != nil {
// 		t.Fatalf("handleMsg failed: %v", err)
// 	}
// 	if !promptServer.listPromptsCalled {
// 		t.Error("ListPrompts was not called")
// 	}
//
// 	// Test GetPrompt
// 	getParams := PromptsGetParams{
// 		Name: "test-prompt",
// 		Arguments: map[string]string{
// 			"test-arg": "test-value",
// 		},
// 		Meta: ParamsMeta{
// 			ProgressToken: "123",
// 		},
// 	}
// 	getParamsBs, _ := json.Marshal(getParams)
// 	getMsg := JSONRPCMessage{
// 		JSONRPC: JSONRPCVersion,
// 		ID:      MustString("2"),
// 		Method:  MethodPromptsGet,
// 		Params:  getParamsBs,
// 	}
// 	getBs, _ := json.Marshal(getMsg)
// 	err = s.handleMsg(bytes.NewReader(getBs), sessID)
// 	if err != nil {
// 		t.Fatalf("handleMsg failed: %v", err)
// 	}
// 	if !promptServer.getPromptCalled {
// 		t.Error("GetPrompt was not called")
// 	}
// }
//
// func TestServerSessionHandleResources(t *testing.T) {
// 	writer := &mockWriter{}
// 	resourceServer := &mockResourceServer{}
//
// 	srv := &mockServer{}
// 	s := newServer(srv,
// 		WithResourceServer(resourceServer),
// 		WithServerWriteTimeout(time.Second),
// 	)
// 	sessID := s.startSession(context.Background(), writer, nopFormatMsgFunc, nopMsgSentHook)
//
// 	initServerSession(t, &s, sessID)
//
// 	// Test ListResources
// 	listParams := ResourcesListParams{
// 		Cursor: "",
// 	}
// 	listParamsBs, _ := json.Marshal(listParams)
// 	listMsg := JSONRPCMessage{
// 		JSONRPC: JSONRPCVersion,
// 		Method:  MethodResourcesList,
// 		ID:      MustString("1"),
// 		Params:  listParamsBs,
// 	}
// 	listBs, _ := json.Marshal(listMsg)
// 	err := s.handleMsg(bytes.NewReader(listBs), sessID)
// 	if err != nil {
// 		t.Fatalf("handleMsg failed: %v", err)
// 	}
// 	if !resourceServer.listResourcesCalled {
// 		t.Error("ListResources was not called")
// 	}
//
// 	// Test ReadResource
// 	readParams := ResourcesReadParams{
// 		URI: "test-uri",
// 	}
// 	readParamsBs, _ := json.Marshal(readParams)
// 	readMsg := JSONRPCMessage{
// 		JSONRPC: JSONRPCVersion,
// 		ID:      MustString("2"),
// 		Method:  MethodResourcesRead,
// 		Params:  readParamsBs,
// 	}
// 	readBs, _ := json.Marshal(readMsg)
// 	err = s.handleMsg(bytes.NewReader(readBs), sessID)
// 	if err != nil {
// 		t.Fatalf("handleMsg failed: %v", err)
// 	}
// 	if !resourceServer.readResourceCalled {
// 		t.Error("ReadResource was not called")
// 	}
//
// 	// Test Subscribe
// 	subscribeParams := ResourcesSubscribeParams{
// 		URI: "test-uri",
// 	}
// 	subscribeParamsBs, _ := json.Marshal(subscribeParams)
// 	subscribeMsg := JSONRPCMessage{
// 		JSONRPC: JSONRPCVersion,
// 		ID:      MustString("3"),
// 		Method:  MethodResourcesSubscribe,
// 		Params:  subscribeParamsBs,
// 	}
// 	subscribeBs, _ := json.Marshal(subscribeMsg)
// 	err = s.handleMsg(bytes.NewReader(subscribeBs), sessID)
// 	if err != nil {
// 		t.Fatalf("handleMsg failed: %v", err)
// 	}
// 	if !resourceServer.subscribeCalled {
// 		t.Error("SubscribeResource was not called")
// 	}
// 	if resourceServer.uri != "test-uri" {
// 		t.Errorf("got uri %s, want test-uri", resourceServer.uri)
// 	}
// }
//
// func TestServerSessionHandleTools(t *testing.T) {
// 	writer := &mockWriter{}
// 	toolServer := &mockToolServer{}
//
// 	srv := &mockServer{}
// 	s := newServer(srv,
// 		WithToolServer(toolServer),
// 		WithServerWriteTimeout(time.Second),
// 	)
// 	sessID := s.startSession(context.Background(), writer, nopFormatMsgFunc, nopMsgSentHook)
//
// 	initServerSession(t, &s, sessID)
//
// 	// Test ListTools
// 	listParams := JSONRPCMessage{
// 		JSONRPC: JSONRPCVersion,
// 		Method:  MethodToolsList,
// 		ID:      MustString("1"),
// 		Params:  []byte{},
// 	}
// 	listParamsBs, _ := json.Marshal(listParams)
// 	listMsg := JSONRPCMessage{
// 		JSONRPC: JSONRPCVersion,
// 		Method:  MethodToolsList,
// 		ID:      MustString("1"),
// 		Params:  listParamsBs,
// 	}
// 	listBs, _ := json.Marshal(listMsg)
// 	err := s.handleMsg(bytes.NewReader(listBs), sessID)
// 	if err != nil {
// 		t.Fatalf("handleMsg failed: %v", err)
// 	}
// 	if !toolServer.listToolsCalled {
// 		t.Error("ListTools was not called")
// 	}
//
// 	// Test CallTool
// 	callParams := ToolsCallParams{
// 		Name: "test-tool",
// 		Arguments: map[string]any{
// 			"test-arg": "test-value",
// 		},
// 	}
// 	callParamsBs, _ := json.Marshal(callParams)
// 	callMsg := JSONRPCMessage{
// 		JSONRPC: JSONRPCVersion,
// 		ID:      MustString("2"),
// 		Method:  MethodToolsCall,
// 		Params:  callParamsBs,
// 	}
// 	callBs, _ := json.Marshal(callMsg)
// 	err = s.handleMsg(bytes.NewReader(callBs), sessID)
// 	if err != nil {
// 		t.Fatalf("handleMsg failed: %v", err)
// 	}
// 	if !toolServer.callToolCalled {
// 		t.Error("CallTool was not called")
// 	}
// 	if toolServer.toolName != "test-tool" {
// 		t.Errorf("got tool name %s, want test-tool", toolServer.toolName)
// 	}
// }
//
// func TestClientSessionHandlePing(t *testing.T) {
// 	writer := &mockWriter{}
// 	sess := &clientSession{
// 		id:           "test-session",
// 		ctx:          context.Background(),
// 		writter:      writer,
// 		writeTimeout: time.Second,
// 		pingInterval: time.Second,
// 	}
//
// 	err := sess.handlePing(MustString("1"))
// 	if err != nil {
// 		t.Fatalf("handlePing failed: %v", err)
// 	}
//
// 	var response JSONRPCMessage
// 	err = json.NewDecoder(bytes.NewReader(writer.getWritten())).Decode(&response)
// 	if err != nil {
// 		t.Fatalf("failed to decode response: %v", err)
// 	}
//
// 	if response.JSONRPC != "2.0" {
// 		t.Errorf("got JSONRPC version %s, want 2.0", response.JSONRPC)
// 	}
// 	if response.ID != MustString("1") {
// 		t.Errorf("got ID %v, want 1", response.ID)
// 	}
// 	if response.Error != nil {
// 		t.Errorf("got error %v, want nil", response.Error)
// 	}
// }
//
// func TestClientSessionHandleRootsList(t *testing.T) {
// 	writer := &mockWriter{}
// 	handler := &mockRootsListHandler{}
// 	sess := &clientSession{
// 		id:           "test-session",
// 		ctx:          context.Background(),
// 		writter:      writer,
// 		writeTimeout: time.Second,
// 	}
//
// 	err := sess.handleRootsList("1", handler)
// 	if err != nil {
// 		t.Fatalf("handleRootsList failed: %v", err)
// 	}
//
// 	if !handler.listRootsCalled {
// 		t.Error("RootsList was not called")
// 	}
//
// 	var response JSONRPCMessage
// 	err = json.NewDecoder(bytes.NewReader(writer.getWritten())).Decode(&response)
// 	if err != nil {
// 		t.Fatalf("failed to decode response: %v", err)
// 	}
//
// 	var result RootList
// 	err = json.Unmarshal(response.Result, &result)
// 	if err != nil {
// 		t.Fatalf("failed to unmarshal result: %v", err)
// 	}
//
// 	if len(result.Roots) != 1 {
// 		t.Errorf("got %d roots, want 1", len(result.Roots))
// 	}
// 	if result.Roots[0].URI != "test://root" {
// 		t.Errorf("got URI %s, want test://root", result.Roots[0].URI)
// 	}
// }
//
// func TestClientSessionHandleSamplingCreateMessage(t *testing.T) {
// 	writer := &mockWriter{}
// 	handler := &mockSamplingHandler{}
// 	sess := &clientSession{
// 		id:           "test-session",
// 		ctx:          context.Background(),
// 		writter:      writer,
// 		writeTimeout: time.Second,
// 	}
//
// 	params := SamplingParams{
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
// 	}
//
// 	err := sess.handleSamplingCreateMessage("1", params, handler)
// 	if err != nil {
// 		t.Fatalf("handleSamplingCreateMessage failed: %v", err)
// 	}
//
// 	if !handler.createMessageCalled {
// 		t.Error("CreateSampleMessage was not called")
// 	}
//
// 	var response JSONRPCMessage
// 	err = json.NewDecoder(bytes.NewReader(writer.getWritten())).Decode(&response)
// 	if err != nil {
// 		t.Fatalf("failed to decode response: %v", err)
// 	}
//
// 	var result SamplingResult
// 	err = json.Unmarshal(response.Result, &result)
// 	if err != nil {
// 		t.Fatalf("failed to unmarshal result: %v", err)
// 	}
//
// 	if result.Role != PromptRoleAssistant {
// 		t.Errorf("got role %s, want assistant", result.Role)
// 	}
// 	if result.Content.Text != "Test response" {
// 		t.Errorf("got text %s, want 'Test response'", result.Content.Text)
// 	}
// }
//
// func initServerSession(t *testing.T, server *server, sessID string) {
// 	initParams := initializeParams{
// 		ProtocolVersion: protocolVersion,
// 		ClientInfo:      Info{Name: "test-client", Version: "1.0"},
// 		Capabilities:    ClientCapabilities{},
// 	}
// 	initParamsBs, _ := json.Marshal(initParams)
// 	initMsg := JSONRPCMessage{
// 		JSONRPC: JSONRPCVersion,
// 		Method:  methodInitialize,
// 		ID:      MustString("1"),
// 		Params:  initParamsBs,
// 	}
// 	initBs, _ := json.Marshal(initMsg)
// 	err := server.handleMsg(bytes.NewReader(initBs), sessID)
// 	if err != nil {
// 		t.Fatalf("handleMsg failed: %v", err)
// 	}
// 	initNotifMsg := JSONRPCMessage{
// 		JSONRPC: JSONRPCVersion,
// 		Method:  methodNotificationsInitialized,
// 		ID:      MustString("2"),
// 	}
// 	initNotifBs, _ := json.Marshal(initNotifMsg)
// 	err = server.handleMsg(bytes.NewReader(initNotifBs), sessID)
// 	if err != nil {
// 		t.Fatalf("handleMsg failed: %v", err)
// 	}
// }

func (m *mockPromptServer) ListPrompts(context.Context, PromptsListParams) (PromptList, error) {
	m.listPromptsCalled = true
	return PromptList{
		Prompts: []Prompt{
			{Name: "test-prompt"},
		},
	}, nil
}

func (m *mockPromptServer) GetPrompt(_ context.Context, _ PromptsGetParams) (PromptResult, error) {
	m.getPromptCalled = true
	return PromptResult{Description: "Test Prompt"}, nil
}

func (m *mockPromptServer) CompletesPrompt(context.Context, CompletionCompleteParams) (CompletionResult, error) {
	return CompletionResult{}, nil
}

func (m *mockPromptListWatcher) PromptListUpdates() <-chan struct{} {
	if m.ch == nil {
		m.ch = make(chan struct{})
	}
	return m.ch
}

// func (m *mockResourceServer) ListResources(_ context.Context, params ResourcesListParams) (ResourceList, error) {
// 	m.listResourcesCalled = true
// 	return ResourceList{
// 		Resources: []Resource{
// 			{URI: "test-resource", Name: "Test Resource"},
// 		},
// 		NextCursor: params.Cursor,
// 	}, nil
// }
//
// func (m *mockResourceServer) ReadResource(_ context.Context, params ResourcesReadParams) (Resource, error) {
// 	m.readResourceCalled = true
// 	return Resource{URI: params.URI, Name: "Test Resource"}, nil
// }
//
// func (m *mockResourceServer) ListResourceTemplates(
// 	_ context.Context,
// 	_ ResourcesTemplatesListParams,
// ) ([]ResourceTemplate, error) {
// 	m.listTemplatesCalled = true
// 	return []ResourceTemplate{
// 		{URITemplate: "test-template", Name: "Test Template"},
// 	}, nil
// }
//
// func (m *mockResourceServer) SubscribeResource(params ResourcesSubscribeParams) {
// 	m.subscribeCalled = true
// 	m.uri = params.URI
// }
//
// func (m *mockResourceServer) UnsubscribeResource(params ResourcesSubscribeParams) {
// 	m.subscribeCalled = true
// 	m.uri = params.URI
// }
//
// func (m *mockResourceServer) CompletesResourceTemplate(
// 	_ context.Context,
// 	_ CompletionCompleteParams,
// ) (CompletionResult, error) {
// 	return CompletionResult{}, nil
// }
//
// func (m *mockToolServer) ListTools(_ context.Context, params ToolsListParams) (ToolList, error) {
// 	m.listToolsCalled = true
// 	return ToolList{
// 		Tools: []Tool{
// 			{Name: "test-tool", Description: "Test Tool"},
// 		},
// 		NextCursor: params.Cursor,
// 	}, nil
// }
//
// func (m *mockToolServer) CallTool(_ context.Context, params ToolsCallParams) (ToolResult, error) {
// 	m.callToolCalled = true
// 	m.toolName = params.Name
// 	return ToolResult{
// 		Content: []Content{{Type: ContentTypeText, Text: "Tool executed"}},
// 		IsError: false,
// 	}, nil
// }
