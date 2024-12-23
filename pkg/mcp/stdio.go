package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// StdIOServer implements a JSON-RPC server over standard I/O.
// It wraps an underlying server implementation and handles message routing
// and protocol handling for stdio-based communication.
type StdIOServer struct {
	srv server
}

// StdIOClient implements a JSON-RPC client over standard I/O.
// It manages bidirectional communication with a StdIOServer, handling
// message routing, session management, and protocol-specific operations.
// The client supports various MCP operations like prompts, resources and tools
// through a unified stdio interface.
type StdIOClient struct {
	cli client
	srv StdIOServer

	writter *stdIOWritter

	currentSessionID string
}

type stdIOWritter struct {
	written []byte
	msgChan chan JSONRPCMessage
}

// NewStdIOServer creates a new StdIOServer instance with the given server implementation
// and optional server configuration options. It automatically disables ping intervals
// since they are not needed for stdio-based communication.
func NewStdIOServer(server Server, option ...ServerOption) StdIOServer {
	// Disable pings for stdio server
	option = append(option, WithServerPingInterval(0))

	return StdIOServer{
		srv: newServer(server, option...),
	}
}

// NewStdIOClient creates a new StdIOClient instance with the given client implementation,
// StdIOServer and optional client configuration options. It automatically disables ping
// intervals since they are not needed for stdio-based communication.
func NewStdIOClient(client Client, srv StdIOServer, option ...ClientOption) *StdIOClient {
	// Disable pings for stdio client
	option = append(option, WithClientPingInterval(0))

	return &StdIOClient{
		cli: newClient(client, option...),
		srv: srv,
		writter: &stdIOWritter{
			msgChan: make(chan JSONRPCMessage),
		},
	}
}

func waitStdIOInput(ctx context.Context, in io.Reader) (JSONRPCMessage, error) {
	inputChan := make(chan []byte)
	errChan := make(chan error)
	go func() {
		bs := make([]byte, 1024)
		n, err := in.Read(bs)
		if err != nil {
			errChan <- err
			return
		}
		inputChan <- bs[:n]
	}()

	var input []byte

	select {
	case <-ctx.Done():
		return JSONRPCMessage{}, ctx.Err()
	case err := <-errChan:
		return JSONRPCMessage{}, err
	case input = <-inputChan:
	}

	var res JSONRPCMessage
	if err := json.Unmarshal(input, &res); err != nil {
		return JSONRPCMessage{}, errInvalidJSON
	}

	return res, nil
}

// Run starts the StdIOClient's main processing loop and manages the client-server communication.
// It initializes a session, processes incoming JSON-RPC messages, and handles various MCP operations.
//
// The provided context controls cancellation and timeout. The method reads JSON-RPC messages
// from the supplied io.Reader and writes responses to the io.Writer. When initialization is
// complete, a signal is sent on readyChan. During operation, any non-fatal errors encountered
// during message processing are sent to errsChan.
//
// The method continues running until either the context is cancelled or a fatal error occurs.
// Non-fatal errors such as invalid JSON are reported through errsChan, while fatal errors
// are returned directly. The errsChan is closed when Run exits.
func (s *StdIOClient) Run(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	readyChan chan<- struct{},
	errsChan chan<- error,
) error {
	s.srv.srv.start()
	defer func() {
		s.srv.srv.stop()
		close(readyChan)
		close(errsChan)
	}()

	go s.listenWritter(ctx)

	s.currentSessionID = s.srv.srv.startSession(ctx, s.writter, nopFormatMsgFunc, nopMsgSentHook)
	s.cli.startSession(ctx, s.writter, s.currentSessionID)

	sessCtx := ctxWithSessionID(ctx, s.currentSessionID)
	if err := s.cli.initialize(sessCtx); err != nil {
		return fmt.Errorf("failed to initialize session: %w", err)
	}

	readyChan <- struct{}{}

	for {
		input, err := waitStdIOInput(ctx, in)
		if err != nil {
			if errors.Is(err, errInvalidJSON) {
				errsChan <- errInvalidJSON
				continue
			}
			return err
		}

		switch input.Method {
		case MethodPromptsList:
			err = s.handlePromptsList(sessCtx, input, out)
		case MethodPromptsGet:
			err = s.handlePromptsGet(sessCtx, input, out)
		case MethodResourcesList:
			err = s.handleResourcesList(sessCtx, input, out)
		case MethodResourcesRead:
			err = s.handleResourcesRead(sessCtx, input, out)
		case MethodResourcesTemplatesList:
			err = s.handleResourcesTemplatesList(sessCtx, input, out)
		case MethodResourcesSubscribe:
			err = s.handleResourcesSubscribe(sessCtx, input, out)
		case MethodToolsList:
			err = s.handleToolsList(sessCtx, input, out)
		case MethodToolsCall:
			err = s.handleToolsCall(sessCtx, input, out)
		default:
			continue
		}

		if err != nil {
			var jsonErr *jsonRPCError
			if !errors.As(err, &jsonErr) {
				errsChan <- err
				continue
			}
			if err := writeError(sessCtx, out, input.ID, *jsonErr, nopFormatMsgFunc); err != nil {
				errsChan <- err
			}
			continue
		}
	}
}

// PromptsCommandsAvailable returns true if the client supports prompt-related commands
// based on the server's capabilities.
func (s *StdIOClient) PromptsCommandsAvailable() bool {
	return s.cli.requiredServerCapabilities.Prompts != nil
}

// ResourcesCommandsAvailable returns true if the client supports resource-related commands
// based on the server's capabilities.
func (s *StdIOClient) ResourcesCommandsAvailable() bool {
	return s.cli.requiredServerCapabilities.Resources != nil
}

// ToolsCommandsAvailable returns true if the client supports tool-related commands
// based on the server's capabilities.
func (s *StdIOClient) ToolsCommandsAvailable() bool {
	return s.cli.requiredServerCapabilities.Tools != nil
}

func (s *StdIOClient) handlePromptsList(ctx context.Context, msg JSONRPCMessage, out io.Writer) error {
	var params PromptsListParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}

	pl, err := s.cli.listPrompts(ctx, params.Cursor, params.Meta.ProgressToken)
	if err != nil {
		return err
	}

	return writeResult(ctx, out, msg.ID, pl, nopFormatMsgFunc)
}

func (s *StdIOClient) handlePromptsGet(ctx context.Context, msg JSONRPCMessage, out io.Writer) error {
	var params PromptsGetParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}

	p, err := s.cli.getPrompt(ctx, params.Name, params.Arguments, params.Meta.ProgressToken)
	if err != nil {
		return err
	}

	return writeResult(ctx, out, msg.ID, p, nopFormatMsgFunc)
}

func (s *StdIOClient) handleResourcesList(ctx context.Context, msg JSONRPCMessage, out io.Writer) error {
	var params ResourcesListParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}

	rl, err := s.cli.listResources(ctx, params.Cursor, params.Meta.ProgressToken)
	if err != nil {
		return err
	}

	return writeResult(ctx, out, msg.ID, rl, nopFormatMsgFunc)
}

func (s *StdIOClient) handleResourcesRead(ctx context.Context, msg JSONRPCMessage, out io.Writer) error {
	var params ResourcesReadParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}

	r, err := s.cli.readResource(ctx, params.URI, params.Meta.ProgressToken)
	if err != nil {
		return err
	}

	return writeResult(ctx, out, msg.ID, r, nopFormatMsgFunc)
}

func (s *StdIOClient) handleResourcesTemplatesList(ctx context.Context, msg JSONRPCMessage, out io.Writer) error {
	var params ResourcesTemplatesListParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}

	rl, err := s.cli.listResourceTemplates(ctx, params.Meta.ProgressToken)
	if err != nil {
		return err
	}

	return writeResult(ctx, out, msg.ID, rl, nopFormatMsgFunc)
}

func (s *StdIOClient) handleResourcesSubscribe(ctx context.Context, msg JSONRPCMessage, out io.Writer) error {
	var params ResourcesSubscribeParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}

	if err := s.cli.subscribeResource(ctx, params.URI); err != nil {
		return err
	}

	return writeResult(ctx, out, msg.ID, nil, nopFormatMsgFunc)
}

func (s *StdIOClient) handleToolsList(ctx context.Context, msg JSONRPCMessage, out io.Writer) error {
	var params ToolsListParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}

	tl, err := s.cli.listTools(ctx, params.Cursor, params.Meta.ProgressToken)
	if err != nil {
		return err
	}

	return writeResult(ctx, out, msg.ID, tl, nopFormatMsgFunc)
}

func (s *StdIOClient) handleToolsCall(ctx context.Context, msg JSONRPCMessage, out io.Writer) error {
	var params ToolsCallParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}

	r, err := s.cli.callTool(ctx, params.Name, params.Arguments, params.Meta.ProgressToken)
	if err != nil {
		return err
	}

	return writeResult(ctx, out, msg.ID, r, nopFormatMsgFunc)
}

func (s *StdIOClient) listenWritter(ctx context.Context) {
	var msg JSONRPCMessage
	for {
		select {
		case <-ctx.Done():
			return
		case msg = <-s.writter.msgChan:
		}

		msgBs, _ := json.Marshal(msg)

		go func() {
			sr := bytes.NewReader(msgBs)
			if err := s.srv.srv.handleMsg(sr, s.currentSessionID); err != nil {
				return
			}
			cr := bytes.NewReader(msgBs)
			if err := s.cli.handleMsg(cr, s.currentSessionID); err != nil {
				return
			}
		}()
	}
}

func (s *stdIOWritter) Write(p []byte) (int, error) {
	s.written = append(s.written, p...)

	var msg JSONRPCMessage
	if err := json.Unmarshal(s.written, &msg); err != nil {
		// Ignore invalid JSON
		//nolint:nilerr // This is a valid error
		return len(p), nil
	}

	s.written = make([]byte, 0)
	s.msgChan <- msg
	return len(p), nil
}
