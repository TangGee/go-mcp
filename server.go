package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Server represents the main MCP server interface that users will implement.
type Server interface {
	Info() Info
	RequireRootsListClient() bool
	RequireSamplingClient() bool
}

// ServerOption represents the options for the server.
type ServerOption func(*server)

type server struct {
	Server

	instructions               string
	capabilities               ServerCapabilities
	requiredClientCapabilities ClientCapabilities
	transport                  ServerTransport

	promptServer      PromptServer
	promptListUpdater PromptListUpdater

	resourceServer              ResourceServer
	resourceListUpdater         ResourceListUpdater
	resourceSubscriptionHandler ResourceSubscriptionHandler

	toolServer      ToolServer
	toolListUpdater ToolListUpdater

	rootsListWatcher RootsListWatcher

	logHandler LogHandler

	writeTimeout time.Duration
	readTimeout  time.Duration
	pingInterval time.Duration

	initialized bool
	logger      *slog.Logger

	startSessions   chan Session
	stopSessions    chan string
	sessionMessages chan sessionMsg
	broadcasts      chan JSONRPCMessage
	waitForResults  chan waitForResultReq
	results         chan JSONRPCMessage
	registerCancels chan cancelRequest
	cancelRequests  chan string
	done            chan struct{}
}

var (
	defaultServerWriteTimeout = 30 * time.Second
	defaultServerReadTimeout  = 30 * time.Second
	defaultServerPingInterval = 30 * time.Second

	errInvalidJSON = errors.New("invalid json")
)

// Serve starts a Model Context Protocol (MCP) server and manages its lifecycle. It handles
// client connections, protocol messages, and server capabilities according to the MCP specification.
//
// The server parameter must implement the Server interface to define core MCP capabilities
// like prompts, resources, and tools. The transport parameter specifies how the server
// communicates with clients (e.g., HTTP or stdio).
//
// Serve blocks until the provided context is cancelled, at which point it performs
// a graceful shutdown by closing all active sessions and cleaning up resources.
func Serve(ctx context.Context, server Server, transport ServerTransport, options ...ServerOption) {
	s := newServer(server, transport, options...)
	if s.promptListUpdater != nil {
		go s.listenUpdates(methodNotificationsPromptsListChanged, s.promptListUpdater.PromptListUpdates())
	}
	if s.resourceListUpdater != nil {
		go s.listenUpdates(methodNotificationsResourcesListChanged, s.resourceListUpdater.ResourceListUpdates())
	}
	if s.resourceSubscriptionHandler != nil {
		go s.listenSubcribedResources()
	}
	if s.toolListUpdater != nil {
		go s.listenUpdates(methodNotificationsToolsListChanged, s.toolListUpdater.ToolListUpdates())
	}
	if s.logHandler != nil {
		go s.listenLogs()
	}
	go s.listenSessions()
	s.start(ctx)
}

// WithPromptServer sets the prompt server for the server.
func WithPromptServer(srv PromptServer) ServerOption {
	return func(s *server) {
		s.promptServer = srv
	}
}

// WithPromptListUpdater sets the prompt list watcher for the server.
func WithPromptListUpdater(updater PromptListUpdater) ServerOption {
	return func(s *server) {
		s.promptListUpdater = updater
	}
}

// WithResourceServer sets the resource server for the server.
func WithResourceServer(srv ResourceServer) ServerOption {
	return func(s *server) {
		s.resourceServer = srv
	}
}

// WithResourceListUpdater sets the resource list watcher for the server.
func WithResourceListUpdater(updater ResourceListUpdater) ServerOption {
	return func(s *server) {
		s.resourceListUpdater = updater
	}
}

// WithResourceSubscriptionHandler sets the resource subscription handler for the server.
func WithResourceSubscriptionHandler(handler ResourceSubscriptionHandler) ServerOption {
	return func(s *server) {
		s.resourceSubscriptionHandler = handler
	}
}

// WithToolServer sets the tool server for the server.
func WithToolServer(srv ToolServer) ServerOption {
	return func(s *server) {
		s.toolServer = srv
	}
}

// WithToolListUpdater sets the tool list watcher for the server.
func WithToolListUpdater(updater ToolListUpdater) ServerOption {
	return func(s *server) {
		s.toolListUpdater = updater
	}
}

// WithRootsListWatcher sets the roots list watcher for the server.
func WithRootsListWatcher(watcher RootsListWatcher) ServerOption {
	return func(s *server) {
		s.rootsListWatcher = watcher
	}
}

// WithLogHandler sets the log handler for the server.
func WithLogHandler(handler LogHandler) ServerOption {
	return func(s *server) {
		s.logHandler = handler
	}
}

// WithInstructions sets the instructions for the server.
func WithInstructions(instructions string) ServerOption {
	return func(s *server) {
		s.instructions = instructions
	}
}

// WithServerWriteTimeout sets the write timeout for the server.
func WithServerWriteTimeout(timeout time.Duration) ServerOption {
	return func(s *server) {
		s.writeTimeout = timeout
	}
}

// WithServerReadTimeout sets the read timeout for the server.
func WithServerReadTimeout(timeout time.Duration) ServerOption {
	return func(s *server) {
		s.readTimeout = timeout
	}
}

// WithServerPingInterval sets the ping interval for the server.
// If set to 0, the server will not send pings.
func WithServerPingInterval(interval time.Duration) ServerOption {
	return func(s *server) {
		s.pingInterval = interval
	}
}

func newServer(srv Server, transport ServerTransport, options ...ServerOption) server {
	s := server{
		Server:          srv,
		transport:       transport,
		logger:          slog.Default(),
		startSessions:   make(chan Session),
		stopSessions:    make(chan string),
		sessionMessages: make(chan sessionMsg),
		broadcasts:      make(chan JSONRPCMessage),
		waitForResults:  make(chan waitForResultReq),
		results:         make(chan JSONRPCMessage),
		registerCancels: make(chan cancelRequest),
		cancelRequests:  make(chan string),
		done:            make(chan struct{}),
	}
	for _, opt := range options {
		opt(&s)
	}
	if s.writeTimeout == 0 {
		s.writeTimeout = defaultServerWriteTimeout
	}
	if s.readTimeout == 0 {
		s.readTimeout = defaultServerReadTimeout
	}
	if s.pingInterval == 0 {
		s.pingInterval = defaultServerPingInterval
	}

	// Prepares the server's capabilities based on the provided server implementations.

	s.capabilities = ServerCapabilities{}

	if s.promptServer != nil {
		s.capabilities.Prompts = &PromptsCapability{}
		if s.promptListUpdater != nil {
			s.capabilities.Prompts.ListChanged = true
		}
	}
	if s.resourceServer != nil {
		s.capabilities.Resources = &ResourcesCapability{}
		if s.resourceListUpdater != nil {
			s.capabilities.Resources.ListChanged = true
		}
		if s.resourceSubscriptionHandler != nil {
			s.capabilities.Resources.Subscribe = true
		}
	}
	if s.toolServer != nil {
		s.capabilities.Tools = &ToolsCapability{}
		if s.toolListUpdater != nil {
			s.capabilities.Tools.ListChanged = true
		}
	}
	if s.logHandler != nil {
		s.capabilities.Logging = &LoggingCapability{}
	}

	s.requiredClientCapabilities = ClientCapabilities{}

	if srv.RequireRootsListClient() {
		s.requiredClientCapabilities.Roots = &RootsCapability{}
		if s.rootsListWatcher != nil {
			s.requiredClientCapabilities.Roots = &RootsCapability{
				ListChanged: true,
			}
		}
	}

	if srv.RequireSamplingClient() {
		s.requiredClientCapabilities.Sampling = &SamplingCapability{}
	}

	return s
}

func (s server) listenSessions() {
	// We maintain a continuous loop to handle new client sessions and their lifecycle
	for sess := range s.transport.Sessions() {
		select {
		case <-s.done:
			return
		case s.startSessions <- sess:
		}

		done := make(chan struct{})
		// We spawn a goroutine to handle messages for this session
		go func() {
			s.listenMessages(sess.ID(), sess.Messages())
			close(done)
		}()

		// We spawn a separate goroutine for session management and ping handling
		// This ensures each session maintains its own heartbeat independently
		go func(sessID string) {
			pingTicker := time.NewTicker(s.pingInterval)

			for {
				select {
				case <-s.done:
					return
				case <-done:
					s.stopSessions <- sessID
				case <-pingTicker.C:
					s.ping(sessID)
				}
			}
		}(sess.ID())
	}
}

func (s server) start(ctx context.Context) {
	// We maintain three core maps for session state management:
	// - sessions: tracks active client sessions
	// - waitForResults: correlates message IDs with their response channels
	// - cancels: stores cancellation functions for ongoing operations
	sessions := make(map[string]Session)                   // map[sessionID]context
	waitForResults := make(map[string]chan JSONRPCMessage) // map[msgID]chan JSONRPCMessage
	cancels := make(map[string]context.CancelFunc)         // map[msgID]context.CancelFunc

	// Main event loop for handling various server operations
	for {
		select {
		case <-ctx.Done():
			close(s.done)
			return
		case sess := <-s.startSessions:
			sessions[sess.ID()] = sess
		case sessID := <-s.stopSessions:
			delete(sessions, sessID)
		case sessMsg := <-s.sessionMessages:
			sess, ok := sessions[sessMsg.sessionID]
			if !ok {
				continue
			}

			go func(sess Session, msg JSONRPCMessage) {
				ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
				defer cancel()

				if err := sess.Send(ctx, msg); err != nil {
					s.logger.Error("failed to send message", "err", err)
				}
			}(sess, sessMsg.msg)
		case msg := <-s.broadcasts:
			for _, sess := range sessions {
				go func(sess Session, msg JSONRPCMessage) {
					ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
					defer cancel()

					if err := sess.Send(ctx, msg); err != nil {
						s.logger.Error("failed to send message", "err", err)
					}
				}(sess, msg)
			}
		case req := <-s.waitForResults:
			resChan := make(chan JSONRPCMessage)
			waitForResults[req.msgID] = resChan
			req.resChan <- resChan
		case msg := <-s.results:
			resChan, ok := waitForResults[string(msg.ID)]
			if !ok {
				continue
			}
			resChan <- msg
			delete(waitForResults, string(msg.ID))
		case cancelReq := <-s.registerCancels:
			cancelCtx, cancel := context.WithCancel(ctx)
			cancelReq.ctxChan <- cancelCtx
			cancels[cancelReq.msgID] = cancel
		case msgID := <-s.cancelRequests:
			cancel, ok := cancels[msgID]
			if !ok {
				continue
			}
			cancel()
			delete(cancels, msgID)
		}
	}
}

func (s server) ping(sessID string) {
	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      MustString(uuid.New().String()),
		Method:  methodPing,
	}

	requester := s.clientRequester(sessID)
	res, err := requester(msg)
	if err != nil {
		s.logger.Error("failed to send ping", "err", err)
		return
	}
	if res.Error != nil {
		s.logger.Error("error response", "err", res.Error)
	}
}

func (s server) listenSubcribedResources() {
	for uri := range s.resourceSubscriptionHandler.SubscribedResourceUpdates() {
		params := notificationsResourcesUpdatedParams{
			URI: uri,
		}
		paramsBs, err := json.Marshal(params)
		if err != nil {
			s.logger.Error("failed to marshal resources updated params", "err", err)
			continue
		}
		msg := JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			Method:  methodNotificationsResourcesUpdated,
			Params:  paramsBs,
		}
		select {
		case <-s.done:
			return
		case s.broadcasts <- msg:
		}
	}
}

func (s server) listenLogs() {
	for params := range s.logHandler.LogStreams() {
		paramsBs, err := json.Marshal(params)
		if err != nil {
			s.logger.Error("failed to marshal log params", "err", err)
			continue
		}
		msg := JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			Method:  methodNotificationsMessage,
			Params:  paramsBs,
		}
		select {
		case <-s.done:
			return
		case s.broadcasts <- msg:
		}
	}
}

func (s server) listenUpdates(method string, updates iter.Seq[struct{}]) {
	for range updates {
		msg := JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			Method:  method,
		}
		select {
		case <-s.done:
			return
		case s.broadcasts <- msg:
		}
	}
}

func (s server) listenMessages(sessID string, msgs iter.Seq[JSONRPCMessage]) {
	for msg := range msgs {
		// We validate JSON-RPC version before processing any message
		if msg.JSONRPC != JSONRPCVersion {
			s.logger.Error("failed to handle message", "err", errInvalidJSON)
			continue
		}

		// We use a switch statement to route messages to appropriate handlers
		// Each handler runs in its own goroutine to prevent blocking
		switch msg.Method {
		case methodPing:
			go s.sendResult(sessID, msg.ID, nil)
		case methodInitialize:
			go s.handleInitialize(sessID, msg, s.capabilities, s.requiredClientCapabilities, s.Info())
		case MethodPromptsList:
			go s.handleListPrompts(sessID, msg)
		case MethodPromptsGet:
			go s.handleGetPrompt(sessID, msg)
		case MethodResourcesList:
			go s.handleListResources(sessID, msg)
		case MethodResourcesRead:
			go s.handleReadResource(sessID, msg)
		case MethodResourcesTemplatesList:
			go s.handleListResourceTemplates(sessID, msg)
		case MethodResourcesSubscribe:
			go s.handleSubscribeResource(sessID, msg)
		case MethodToolsList:
			go s.handleListTools(sessID, msg)
		case MethodToolsCall:
			go s.handleCallTool(sessID, msg)
		case MethodCompletionComplete:
			var params CompletesCompletionParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				s.sendError(sessID, msg.ID, JSONRPCError{
					Code:    jsonRPCInvalidParamsCode,
					Message: errMsgInvalidJSON,
					Data:    map[string]any{"error": err},
				})
				continue
			}
			switch params.Ref.Type {
			case CompletionRefPrompt:
				go s.handleCompletePrompt(sessID, msg)
			case CompletionRefResource:
				go s.handleCompleteResource(sessID, msg)
			default:
				err := fmt.Errorf("unsupported completion reference type: %s", params.Ref.Type)
				s.sendError(sessID, msg.ID, JSONRPCError{
					Code:    jsonRPCInvalidParamsCode,
					Message: errMsgInvalidJSON,
					Data:    map[string]any{"error": err},
				})
			}
		case MethodLoggingSetLevel:
			go s.handleSetLogLevel(sessID, msg)
		case methodNotificationsInitialized:
			s.initialized = true
		case methodNotificationsCancelled:
			select {
			case <-s.done:
			case s.cancelRequests <- string(msg.ID):
			}
		case methodNotificationsRootsListChanged:
			if s.rootsListWatcher != nil {
				s.rootsListWatcher.OnRootsListChanged()
			}
		case "":
			select {
			case <-s.done:
			case s.results <- msg:
			}
		}
	}
}

func (s server) handleInitialize(
	sessID string,
	msg JSONRPCMessage,
	serverCap ServerCapabilities,
	requiredClientCap ClientCapabilities,
	serverInfo Info,
) {
	var params initializeParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInvalidJSON,
			Data:    map[string]any{"error": err},
		})
		return
	}

	if params.ProtocolVersion != protocolVersion {
		err := fmt.Errorf("protocol version mismatch: %s != %s", params.ProtocolVersion, protocolVersion)
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgUnsupportedProtocolVersion,
			Data:    map[string]any{"error": err},
		})
		return
	}

	if requiredClientCap.Roots != nil {
		if params.Capabilities.Roots == nil {
			err := fmt.Errorf("insufficient client capabilities: missing required capability 'roots'")
			s.sendError(sessID, msg.ID, JSONRPCError{
				Code:    jsonRPCInvalidParamsCode,
				Message: errMsgInsufficientClientCapabilities,
				Data:    map[string]any{"error": err},
			})
			return
		}
		if requiredClientCap.Roots.ListChanged {
			if !params.Capabilities.Roots.ListChanged {
				err := fmt.Errorf("insufficient client capabilities: missing required capability 'roots.listChanged'")
				s.sendError(sessID, msg.ID, JSONRPCError{
					Code:    jsonRPCInvalidParamsCode,
					Message: errMsgInsufficientClientCapabilities,
					Data:    map[string]any{"error": err},
				})
				return
			}
		}
	}

	if requiredClientCap.Sampling != nil {
		if params.Capabilities.Sampling == nil {
			err := fmt.Errorf("insufficient client capabilities: missing required capability 'sampling'")
			s.sendError(sessID, msg.ID, JSONRPCError{
				Code:    jsonRPCInvalidParamsCode,
				Message: errMsgInsufficientClientCapabilities,
				Data:    map[string]any{"error": err},
			})
			return
		}
	}

	s.sendResult(sessID, msg.ID, initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    serverCap,
		ServerInfo:      serverInfo,
		Instructions:    s.instructions,
	})
}

func (s server) handleListPrompts(sessID string, msg JSONRPCMessage) {
	if s.promptServer == nil || !s.initialized {
		return
	}

	var params ListPromptsParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInvalidJSON,
			Data:    map[string]any{"error": err},
		})
		return
	}

	ctx := s.registerCancel(string(msg.ID))

	ps, err := s.promptServer.ListPrompts(ctx, params, s.progressReporter(sessID, msg.ID), s.clientRequester(sessID))
	if err != nil {
		nErr := fmt.Errorf("failed to list prompts: %w", err)
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(sessID, msg.ID, ps)
}

func (s server) handleGetPrompt(sessID string, msg JSONRPCMessage) {
	if s.promptServer == nil || !s.initialized {
		return
	}

	var params GetPromptParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInvalidJSON,
			Data:    map[string]any{"error": err},
		})
		return
	}

	ctx := s.registerCancel(string(msg.ID))

	p, err := s.promptServer.GetPrompt(ctx, params, s.progressReporter(sessID, msg.ID), s.clientRequester(sessID))
	if err != nil {
		nErr := fmt.Errorf("failed to get prompt: %w", err)
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(sessID, msg.ID, p)
}

func (s server) handleListResources(sessID string, msg JSONRPCMessage) {
	if s.resourceServer == nil || !s.initialized {
		return
	}

	var params ListResourcesParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInvalidJSON,
			Data:    map[string]any{"error": err},
		})
		return
	}

	ctx := s.registerCancel(string(msg.ID))

	rs, err := s.resourceServer.ListResources(ctx, params, s.progressReporter(sessID, msg.ID), s.clientRequester(sessID))
	if err != nil {
		nErr := fmt.Errorf("failed to list resources: %w", err)
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(sessID, msg.ID, rs)
}

func (s server) handleReadResource(sessID string, msg JSONRPCMessage) {
	if s.resourceServer == nil || !s.initialized {
		return
	}

	var params ReadResourceParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInvalidJSON,
			Data:    map[string]any{"error": err},
		})
		return
	}

	ctx := s.registerCancel(string(msg.ID))

	r, err := s.resourceServer.ReadResource(ctx, params, s.progressReporter(sessID, msg.ID), s.clientRequester(sessID))
	if err != nil {
		nErr := fmt.Errorf("failed to read resource: %w", err)
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(sessID, msg.ID, r)
}

func (s server) handleListResourceTemplates(sessID string, msg JSONRPCMessage) {
	if s.resourceServer == nil || !s.initialized {
		return
	}

	var params ListResourceTemplatesParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInvalidJSON,
			Data:    map[string]any{"error": err},
		})
		return
	}

	ctx := s.registerCancel(string(msg.ID))

	ts, err := s.resourceServer.ListResourceTemplates(ctx, params,
		s.progressReporter(sessID, msg.ID), s.clientRequester(sessID))
	if err != nil {
		nErr := fmt.Errorf("failed to list resource templates: %w", err)
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(sessID, msg.ID, ts)
}

func (s server) handleSubscribeResource(sessID string, msg JSONRPCMessage) {
	if s.resourceSubscriptionHandler == nil || !s.initialized {
		return
	}

	var params SubscribeResourceParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInvalidJSON,
			Data:    map[string]any{"error": err},
		})
		return
	}

	s.resourceSubscriptionHandler.SubscribeResource(params)

	s.sendResult(sessID, msg.ID, nil)
}

func (s server) handleCompletePrompt(sessID string, msg JSONRPCMessage) {
	if s.promptServer == nil || !s.initialized {
		return
	}

	var params CompletesCompletionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInvalidJSON,
			Data:    map[string]any{"error": err},
		})
		return
	}

	ctx := s.registerCancel(string(msg.ID))

	result, err := s.promptServer.CompletesPrompt(ctx, params, s.clientRequester(sessID))
	if err != nil {
		nErr := fmt.Errorf("failed to complete prompt: %w", err)
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(sessID, msg.ID, result)
}

func (s server) handleCompleteResource(sessID string, msg JSONRPCMessage) {
	if s.resourceServer == nil || !s.initialized {
		return
	}

	var params CompletesCompletionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInvalidJSON,
			Data:    map[string]any{"error": err},
		})
		return
	}

	ctx := s.registerCancel(string(msg.ID))

	result, err := s.resourceServer.CompletesResourceTemplate(ctx, params, s.clientRequester(sessID))
	if err != nil {
		nErr := fmt.Errorf("failed to complete resource template: %w", err)
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(sessID, msg.ID, result)
}

func (s server) handleListTools(sessID string, msg JSONRPCMessage) {
	if s.toolServer == nil || !s.initialized {
		return
	}

	var params ListToolsParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInvalidJSON,
			Data:    map[string]any{"error": err},
		})
		return
	}

	ctx := s.registerCancel(string(msg.ID))

	ts, err := s.toolServer.ListTools(ctx, params, s.progressReporter(sessID, msg.ID), s.clientRequester(sessID))
	if err != nil {
		nErr := fmt.Errorf("failed to list tools: %w", err)
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(sessID, msg.ID, ts)
}

func (s server) handleCallTool(sessID string, msg JSONRPCMessage) {
	if s.toolServer == nil || !s.initialized {
		return
	}

	var params CallToolParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInvalidJSON,
			Data:    map[string]any{"error": err},
		})
		return
	}

	ctx := s.registerCancel(string(msg.ID))

	result, err := s.toolServer.CallTool(ctx, params, s.progressReporter(sessID, msg.ID), s.clientRequester(sessID))
	if err != nil {
		nErr := fmt.Errorf("failed to call tool: %w", err)
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(sessID, msg.ID, result)
}

func (s server) handleSetLogLevel(sessID string, msg JSONRPCMessage) {
	if s.logHandler == nil || !s.initialized {
		return
	}

	var params LogParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(sessID, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInvalidJSON,
			Data:    map[string]any{"error": err},
		})
		return
	}

	s.logHandler.SetLogLevel(params.Level)
}

func (s server) progressReporter(sessID string, msgID MustString) func(ProgressParams) {
	return func(params ProgressParams) {
		paramsBs, err := json.Marshal(params)
		if err != nil {
			s.logger.Error("failed to marshal progress params", "err", err)
			return
		}

		msg := JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			ID:      msgID,
			Method:  methodNotificationsProgress,
			Params:  paramsBs,
		}

		sessMsg := sessionMsg{
			sessionID: sessID,
			msg:       msg,
		}

		select {
		case <-s.done:
		case s.sessionMessages <- sessMsg:
		}
	}
}

func (s server) clientRequester(sessID string) func(msg JSONRPCMessage) (JSONRPCMessage, error) {
	// We return a closure that handles the complexity of making client requests
	// This includes message correlation and timeout management
	return func(msg JSONRPCMessage) (JSONRPCMessage, error) {
		// We generate a unique ID for request-response correlation
		msgID := uuid.New().String()
		msg.ID = MustString(msgID)

		// We set up channels for handling the response asynchronously
		resChanChan := make(chan chan JSONRPCMessage)
		wfrReq := waitForResultReq{
			msgID:   msgID,
			resChan: resChanChan,
		}

		select {
		case <-s.done:
			return JSONRPCMessage{}, errors.New("server closed")
		case s.waitForResults <- wfrReq:
		}

		resChan := <-resChanChan

		sessMsg := sessionMsg{
			sessionID: sessID,
			msg:       msg,
		}

		select {
		case <-s.done:
			return JSONRPCMessage{}, errors.New("server closed")
		case s.sessionMessages <- sessMsg:
		}

		ticker := time.NewTicker(s.readTimeout)

		var resMsg JSONRPCMessage

		select {
		case <-ticker.C:
			err := errors.New("request timeout")
			s.logger.Error("failed to wait for result", "err", err)
			return JSONRPCMessage{}, err
		case <-s.done:
			return JSONRPCMessage{}, errors.New("server closed")
		case resMsg = <-resChan:
		}

		return resMsg, nil
	}
}

func (s server) sendResult(sessID string, msgID MustString, result any) {
	var resBs []byte
	if result != nil {
		var err error
		resBs, err = json.Marshal(result)
		if err != nil {
			s.logger.Error("failed to marshal result", "err", err)
		}
	}

	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      msgID,
		Result:  resBs,
	}

	sessMsg := sessionMsg{
		sessionID: sessID,
		msg:       msg,
	}

	select {
	case <-s.done:
	case s.sessionMessages <- sessMsg:
	}
}

func (s server) sendError(sessID string, msgID MustString, err JSONRPCError) {
	s.logger.Error("request error", "err", err)
	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      msgID,
		Error:   &err,
	}

	sessMsg := sessionMsg{
		sessionID: sessID,
		msg:       msg,
	}

	select {
	case <-s.done:
	case s.sessionMessages <- sessMsg:
	}
}

func (s server) registerCancel(msgID string) context.Context {
	ctxChan := make(chan context.Context)
	req := cancelRequest{
		msgID:   msgID,
		ctxChan: ctxChan,
	}

	select {
	case <-s.done:
		return nil
	case s.registerCancels <- req:
	}
	return <-ctxChan
}
