package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

func TestStdIOServer(t *testing.T) {
	mockSrv := &mockServer{}
	srv := NewStdIOServer(mockSrv)

	// Verify ping interval is disabled
	if srv.srv.pingInterval != 0 {
		t.Error("ping interval should be disabled for stdio server")
	}

	// Test handling a message
	writer := &mockWriter{}
	sessID := srv.srv.startSession(context.Background(), writer)

	// Initialize session
	initParams := initializeParams{
		ProtocolVersion: protocolVersion,
		ClientInfo:      Info{Name: "test-client", Version: "1.0"},
		Capabilities:    ClientCapabilities{},
	}
	initParamsBs, _ := json.Marshal(initParams)
	initMsg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  methodInitialize,
		ID:      MustString("1"),
		Params:  initParamsBs,
	}
	initBs, _ := json.Marshal(initMsg)

	err := srv.srv.handleMsg(bytes.NewReader(initBs), sessID)
	if err != nil {
		t.Fatalf("handleMsg failed: %v", err)
	}

	// Verify response was written
	var response JSONRPCMessage
	err = json.NewDecoder(bytes.NewReader(writer.getWritten())).Decode(&response)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.ID != MustString("1") {
		t.Errorf("got ID %v, want 1", response.ID)
	}
}

func TestStdIOClient(t *testing.T) {
	mockCli := &mockClient{}
	mockSrv := &mockServer{}
	srv := NewStdIOServer(mockSrv, WithPromptServer(&mockPromptServer{}))
	cli := NewStdIOClient(mockCli, srv)

	// Verify ping interval is disabled
	if cli.cli.pingInterval != 0 {
		t.Error("ping interval should be disabled for stdio client")
	}

	// Create test context and channels
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	errsChan := make(chan error)

	// Create test input/output
	params := PromptsListParams{
		Cursor: "",
	}
	paramsBs, _ := json.Marshal(params)
	input := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      MustString("1"),
		Method:  MethodPromptsList,
		Params:  paramsBs,
	}
	inputBs, _ := json.Marshal(input)
	inReader := bytes.NewReader(inputBs)
	outWriter := &mockWriter{}

	// Run client in goroutine
	go func() {
		_ = cli.Run(ctx, inReader, outWriter, errsChan)
	}()

	// Wait for response or timeout
	select {
	case err := <-errsChan:
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
				t.Fatalf("received error: %v", err)
			}
		}
	case <-time.After(time.Second):
		// Success - no errors received
	}

	// Verify response was written
	var response JSONRPCMessage
	err := json.NewDecoder(bytes.NewReader(outWriter.getWritten())).Decode(&response)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.ID != MustString("1") {
		t.Errorf("got ID %v, want 1", response.ID)
	}
}

func TestStdIOClientCommandsAvailable(t *testing.T) {
	mockCli := &mockClient{}
	mockSrv := &mockServer{}
	srv := NewStdIOServer(mockSrv)
	cli := NewStdIOClient(mockCli, srv)

	// Initially no commands should be available
	if cli.PromptsCommandsAvailable() {
		t.Error("prompts commands should not be available")
	}
	if cli.ResourcesCommandsAvailable() {
		t.Error("resources commands should not be available")
	}
	if cli.ToolsCommandsAvailable() {
		t.Error("tools commands should not be available")
	}

	// Set required capabilities
	cli.cli.requiredServerCapabilities = ServerCapabilities{
		Prompts:   &PromptsCapability{},
		Resources: &ResourcesCapability{},
		Tools:     &ToolsCapability{},
	}

	// Now commands should be available
	if !cli.PromptsCommandsAvailable() {
		t.Error("prompts commands should be available")
	}
	if !cli.ResourcesCommandsAvailable() {
		t.Error("resources commands should be available")
	}
	if !cli.ToolsCommandsAvailable() {
		t.Error("tools commands should be available")
	}
}
