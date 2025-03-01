package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ServerOption represents the options for the server.
type ServerOption func(*Server)

// Server implements a Model Context Protocol (MCP) server that enables communication
// between LLM applications and external data sources and tools. It manages the
// connection lifecycle, handles protocol messages, and provides access to MCP
// server capabilities.
type Server struct {
	info Info

	instructions               string
	capabilities               ServerCapabilities
	requiredClientCapabilities ClientCapabilities
	transport                  ServerTransport

	requireRootsListClient bool
	requireSamplingClient  bool

	promptServer      PromptServer
	promptListUpdater PromptListUpdater

	resourceServer              ResourceServer
	resourceListUpdater         ResourceListUpdater
	resourceSubscriptionHandler ResourceSubscriptionHandler

	toolServer      ToolServer
	toolListUpdater ToolListUpdater

	rootsListWatcher RootsListWatcher

	logHandler LogHandler

	pingInterval         time.Duration
	pingTimeout          time.Duration
	pingTimeoutThreshold int
	sendTimeout          time.Duration

	logger *slog.Logger

	onClientConnected    func(string, Info)
	onClientDisconnected func(string)

	sessionsWaitGroup *sync.WaitGroup

	done                     chan struct{}
	promptListClosed         chan struct{}
	resourceListClosed       chan struct{}
	resourceSubscribedClosed chan struct{}
	toolListClosed           chan struct{}
	logClosed                chan struct{}
}

type serverSession struct {
	session Session
	logger  *slog.Logger

	serverCap         ServerCapabilities
	requiredClientCap ClientCapabilities
	serverInfo        Info
	instructions      string

	pingInterval         time.Duration
	pingTimeout          time.Duration
	pingTimeoutThreshold int
	sendTimeout          time.Duration

	promptServer                PromptServer
	resourceServer              ResourceServer
	toolServer                  ToolServer
	resourceSubscriptionHandler ResourceSubscriptionHandler
	logHandler                  LogHandler
	rootsListWatcher            RootsListWatcher
}

var (
	defaultServerPingInterval         = 30 * time.Second
	defaultServerPingTimeout          = 30 * time.Second
	defaultServerPingTimeoutThreshold = 3
	defaultServerSendTimeout          = 30 * time.Second

	errInvalidJSON = errors.New("invalid json")
)

// NewServer creates a new Model Context Protocol (MCP) server with the specified configuration.
func NewServer(info Info, transport ServerTransport, options ...ServerOption) Server {
	s := Server{
		info:                     info,
		transport:                transport,
		logger:                   slog.Default(),
		sessionsWaitGroup:        &sync.WaitGroup{},
		done:                     make(chan struct{}),
		promptListClosed:         make(chan struct{}),
		resourceListClosed:       make(chan struct{}),
		resourceSubscribedClosed: make(chan struct{}),
		toolListClosed:           make(chan struct{}),
		logClosed:                make(chan struct{}),
	}
	for _, opt := range options {
		opt(&s)
	}
	if s.pingInterval == 0 {
		s.pingInterval = defaultServerPingInterval
	}
	if s.pingTimeout == 0 {
		s.pingTimeout = defaultServerPingTimeout
	}
	if s.pingTimeoutThreshold == 0 {
		s.pingTimeoutThreshold = defaultServerPingTimeoutThreshold
	}
	if s.sendTimeout == 0 {
		s.sendTimeout = defaultServerSendTimeout
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

	if s.requireRootsListClient {
		s.requiredClientCapabilities.Roots = &RootsCapability{}
		if s.rootsListWatcher != nil {
			s.requiredClientCapabilities.Roots = &RootsCapability{
				ListChanged: true,
			}
		}
	}

	if s.requireSamplingClient {
		s.requiredClientCapabilities.Sampling = &SamplingCapability{}
	}

	return s
}

// WithRequireRootsListClient sets the roots list client requirement for the server.
func WithRequireRootsListClient() ServerOption {
	return func(s *Server) {
		s.requireRootsListClient = true
	}
}

// WithRequireSamplingClient sets the sampling client requirement for the server.
func WithRequireSamplingClient() ServerOption {
	return func(s *Server) {
		s.requireSamplingClient = true
	}
}

// WithPromptServer sets the prompt server for the server.
func WithPromptServer(srv PromptServer) ServerOption {
	return func(s *Server) {
		s.promptServer = srv
	}
}

// WithPromptListUpdater sets the prompt list watcher for the server.
func WithPromptListUpdater(updater PromptListUpdater) ServerOption {
	return func(s *Server) {
		s.promptListUpdater = updater
	}
}

// WithResourceServer sets the resource server for the server.
func WithResourceServer(srv ResourceServer) ServerOption {
	return func(s *Server) {
		s.resourceServer = srv
	}
}

// WithResourceListUpdater sets the resource list watcher for the server.
func WithResourceListUpdater(updater ResourceListUpdater) ServerOption {
	return func(s *Server) {
		s.resourceListUpdater = updater
	}
}

// WithResourceSubscriptionHandler sets the resource subscription handler for the server.
func WithResourceSubscriptionHandler(handler ResourceSubscriptionHandler) ServerOption {
	return func(s *Server) {
		s.resourceSubscriptionHandler = handler
	}
}

// WithToolServer sets the tool server for the server.
func WithToolServer(srv ToolServer) ServerOption {
	return func(s *Server) {
		s.toolServer = srv
	}
}

// WithToolListUpdater sets the tool list watcher for the server.
func WithToolListUpdater(updater ToolListUpdater) ServerOption {
	return func(s *Server) {
		s.toolListUpdater = updater
	}
}

// WithRootsListWatcher sets the roots list watcher for the server.
func WithRootsListWatcher(watcher RootsListWatcher) ServerOption {
	return func(s *Server) {
		s.rootsListWatcher = watcher
	}
}

// WithLogHandler sets the log handler for the server.
func WithLogHandler(handler LogHandler) ServerOption {
	return func(s *Server) {
		s.logHandler = handler
	}
}

// WithInstructions sets the instructions for the server.
func WithInstructions(instructions string) ServerOption {
	return func(s *Server) {
		s.instructions = instructions
	}
}

// WithServerPingInterval sets the ping interval for the server.
func WithServerPingInterval(interval time.Duration) ServerOption {
	return func(s *Server) {
		s.pingInterval = interval
	}
}

// WithServerPingTimeout sets the ping timeout for the server.
func WithServerPingTimeout(timeout time.Duration) ServerOption {
	return func(s *Server) {
		s.pingTimeout = timeout
	}
}

// WithServerPingTimeoutThreshold sets the ping timeout threshold for the server.
// If the number of consecutive ping timeouts exceeds the threshold, the server will close the session.
func WithServerPingTimeoutThreshold(threshold int) ServerOption {
	return func(s *Server) {
		s.pingTimeoutThreshold = threshold
	}
}

// WithServerSendTimeout sets the send timeout for the server.
func WithServerSendTimeout(timeout time.Duration) ServerOption {
	return func(s *Server) {
		s.sendTimeout = timeout
	}
}

// WithServerOnClientConnected sets the callback for when a client connects.
// The callback's parameter is the ID and Info of the client.
func WithServerOnClientConnected(onClientConnected func(string, Info)) ServerOption {
	return func(s *Server) {
		s.onClientConnected = onClientConnected
	}
}

// WithServerOnClientDisconnected sets the callback for when a client disconnects.
// The callback's parameter is the ID of the client.
func WithServerOnClientDisconnected(onClientDisconnected func(string)) ServerOption {
	return func(s *Server) {
		s.onClientDisconnected = onClientDisconnected
	}
}

// Serve starts a Model Context Protocol (MCP) server and manages its lifecycle. It handles
// client connections, protocol messages, and server capabilities according to the MCP specification.
//
// The server parameter must implement the Server interface to define core MCP capabilities
// like prompts, resources, and tools. The transport parameter specifies how the server
// communicates with clients (e.g., HTTP or stdio).
//
// Serve blocks until the provided context is cancelled, at which point it performs
// a graceful shutdown by closing all active sessions and cleaning up resources.
func (s Server) Serve() {
	broadcasts := make(chan JSONRPCMessage)

	if s.promptListUpdater != nil {
		go s.listenUpdates(methodNotificationsPromptsListChanged, s.promptListUpdater.PromptListUpdates(),
			broadcasts, s.promptListClosed)
	} else {
		close(s.promptListClosed)
	}

	if s.resourceListUpdater != nil {
		go s.listenUpdates(methodNotificationsResourcesListChanged, s.resourceListUpdater.ResourceListUpdates(),
			broadcasts, s.resourceListClosed)
	} else {
		close(s.resourceListClosed)
	}

	if s.resourceSubscriptionHandler != nil {
		go s.listenSubcribedResources(broadcasts)
	} else {
		close(s.resourceSubscribedClosed)
	}

	if s.toolListUpdater != nil {
		go s.listenUpdates(methodNotificationsToolsListChanged, s.toolListUpdater.ToolListUpdates(),
			broadcasts, s.toolListClosed)
	} else {
		close(s.toolListClosed)
	}

	if s.logHandler != nil {
		go s.listenLogs(broadcasts)
	} else {
		close(s.logClosed)
	}

	s.start(broadcasts)
}

// Shutdown gracefully shuts down the server by terminating all active client.
func (s Server) Shutdown(ctx context.Context) error {
	// Signal the server to shutdown and terminates all sessions
	close(s.done)

	// Wait for all sessions to finish
	s.sessionsWaitGroup.Wait()

	// Close the transport so the Sessions loop in the start function breaks.
	if err := s.transport.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown transport: %w", err)
	}

	// Wait for all goroutines to finish

	select {
	case <-ctx.Done():
		return fmt.Errorf("failed to close PromptListUpdater: %w", ctx.Err())
	case <-s.promptListClosed:
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("failed to close ResourceListUpdater: %w", ctx.Err())
	case <-s.resourceListClosed:
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("failed to close ResourceSubscriptionHandler: %w", ctx.Err())
	case <-s.resourceSubscribedClosed:
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("failed to close ToolListUpdater: %w", ctx.Err())
	case <-s.toolListClosed:
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("failed to close LogHandler: %w", ctx.Err())
	case <-s.logClosed:
	}

	return nil
}

func (s Server) start(broadcasts <-chan JSONRPCMessage) {
	// These channles are used to send broadcasts to all sessions in the goroutine below.
	sessions := make(chan serverSession)
	removedSessions := make(chan string)

	go s.broadcast(broadcasts, sessions, removedSessions)

	// This loop would break when the transport is closed.
	for sess := range s.transport.Sessions() {
		ss := serverSession{
			session:                     sess,
			logger:                      s.logger.With(slog.String("sessionID", sess.ID())),
			serverCap:                   s.capabilities,
			requiredClientCap:           s.requiredClientCapabilities,
			serverInfo:                  s.info,
			instructions:                s.instructions,
			pingInterval:                s.pingInterval,
			pingTimeout:                 s.pingTimeout,
			pingTimeoutThreshold:        s.pingTimeoutThreshold,
			sendTimeout:                 s.sendTimeout,
			promptServer:                s.promptServer,
			resourceServer:              s.resourceServer,
			toolServer:                  s.toolServer,
			resourceSubscriptionHandler: s.resourceSubscriptionHandler,
			logHandler:                  s.logHandler,
			rootsListWatcher:            s.rootsListWatcher,
		}
		// Updates the broadcaster about new sessions
		sessions <- ss

		s.sessionsWaitGroup.Add(1)

		// This session would close itself when client failed to initialize or
		// when consecutive pings fail beyond threshold.
		go func() {
			defer s.sessionsWaitGroup.Done()

			if s.onClientConnected != nil {
				s.onClientConnected(ss.session.ID(), ss.serverInfo)
			}

			ss.start(s.done)

			if s.onClientDisconnected != nil {
				s.onClientDisconnected(ss.session.ID())
			}

			// Notify the broadcaster about removed sessions
			select {
			case <-s.done:
			case removedSessions <- ss.session.ID():
			}
		}()
	}
}

func (s Server) broadcast(messages <-chan JSONRPCMessage, sessions <-chan serverSession, removedSession <-chan string) {
	// Store all active sessions in a map for easy lookup
	sessMap := make(map[string]serverSession)

	for {
		select {
		case <-s.done:
			return
		case sess := <-sessions:
			sessMap[sess.session.ID()] = sess
		case sessID := <-removedSession:
			delete(sessMap, sessID)
		case msg := <-messages:
			ctx, cancel := context.WithTimeout(context.Background(), s.sendTimeout)
			// Broadcast the message to all active sessions
			for _, sess := range sessMap {
				if err := sess.session.Send(ctx, msg); err != nil {
					sess.logger.Error("failed to send message",
						slog.Any("message", msg),
						slog.String("err", err.Error()))
				}
			}
			cancel()
		}
	}
}

func (s Server) listenSubcribedResources(messages chan<- JSONRPCMessage) {
	defer close(s.resourceSubscribedClosed)

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
		case messages <- msg:
		}
	}
}

func (s Server) listenLogs(messages chan<- JSONRPCMessage) {
	defer close(s.logClosed)

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
		case messages <- msg:
		}
	}
}

func (s Server) listenUpdates(
	method string,
	updates iter.Seq[struct{}],
	messages chan<- JSONRPCMessage,
	closed chan<- struct{},
) {
	defer close(closed)

	for range updates {
		msg := JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			Method:  method,
		}
		select {
		case <-s.done:
			return
		case messages <- msg:
		}
	}
}

func (s serverSession) start(done <-chan struct{}) {
	// This channel is used to feed the ping goroutine a message ID we received from the client.
	pingMessageIDs := make(chan MustString, 10)
	// Spawn a goroutine to handle the session's lifetime with ping.
	go s.ping(pingMessageIDs, done)
	// This map is used to store the cancellation for the request
	// we receive from the client and forwards to server implementation.
	ctxCancels := make(map[MustString]context.CancelFunc)
	// This map is used to store the result channels for the clientRequester. The channel
	// would be feed once we receive the result back from the client.
	requestResults := make(map[MustString]chan JSONRPCMessage)
	// This base context is to make sure all the operations in the loop below is cancelled
	// when the loop is broken.
	baseCtx, baseCancel := context.WithCancel(context.Background())
	// This flag indicates whether we already established the session with the client.
	// Before this flag is set to true, other than ping and initialization message,
	// we should ignore any other messages from the client.
	initialized := false

	// This loops would break when the session is closed
	for msg := range s.session.Messages() {
		// Validate JSON-RPC version before processing any message
		if msg.JSONRPC != JSONRPCVersion {
			s.logger.Info("failed to handle message",
				slog.Any("message", msg),
				slog.String("err", errInvalidJSON.Error()),
			)
			continue
		}

		switch msg.Method {
		case methodPing:
			pongCtx, pongCancel := context.WithTimeout(context.Background(), s.pingTimeout)
			// Send pong back to the client
			if err := s.session.Send(pongCtx, JSONRPCMessage{
				JSONRPC: JSONRPCVersion,
				ID:      msg.ID,
			}); err != nil {
				s.logger.Error("failed to send pong", slog.String("err", err.Error()))
			}
			pongCancel()
		case methodInitialize:
			// If the initialization request is invalid, we cannot call session.Stop() here,
			// because it would make calling that function twice. Which in turn would cause
			// the "closing of closed channel" panic in the transport implementation.
			s.handleInitializeRequest(msg)
		case MethodPromptsList, MethodPromptsGet, MethodResourcesList, MethodResourcesRead, MethodResourcesTemplatesList,
			MethodResourcesSubscribe, MethodResourcesUnsubscribe, MethodToolsList, MethodToolsCall,
			MethodCompletionComplete, MethodLoggingSetLevel:
			if !initialized {
				// Ignore messages until the session is initialized
				continue
			}
			// All the method above required us to call the server implementation, and all the call is cancellable,
			// so we need to register it to the map, so we can cancel it if the client requests it.
			serverCtx, serverCancel := context.WithCancel(baseCtx)
			ctxCancels[msg.ID] = serverCancel
			// Since the call for the server implementation may use clientRequester that wait for client's response,
			// we need to register the result channel for the clientRequester to receive the response. Also, since
			// waiting for the client's response is a blocking operation, we need to spawn a goroutine to handle it.
			results := make(chan JSONRPCMessage)
			requestResults[msg.ID] = results
			go s.handleServerImplementationMessage(serverCtx, msg, results)
		case methodNotificationsInitialized:
			// Successfully established the session with the client
			initialized = true
		case methodNotificationsCancelled:
			if !initialized {
				// Ignore messages until the session is initialized
				continue
			}
			// Lookup the context cancellation for message ID
			cancel, ok := ctxCancels[msg.ID]
			if ok {
				cancel()
			}
		case methodNotificationsRootsListChanged:
			if !initialized {
				// Ignore messages until the session is initialized
				continue
			}
			if s.rootsListWatcher != nil {
				s.rootsListWatcher.OnRootsListChanged()
			}
		case "":
			// This is the response from the client, either from ping request or
			// clientRequester that called by the server implementation

			// Feed the ping gourotine with the message ID we received from the client.
			select {
			case <-done:
				break
			case pingMessageIDs <- msg.ID:
			}

			// If it is indeed a ping response, it would not stored on the map.
			// And as the other rseponse with invalid message ID, we ingore it.
			results, rOk := requestResults[msg.ID]
			if !rOk {
				continue
			}
			// Feed the stored result channel, but drop it instantly if there is no receiver. The no-receiver case
			// happens when the clientRequester is failed to send the request to the client, but somehow the client
			// response back with this message ID.
			select {
			case results <- msg:
			default:
			}
			delete(requestResults, msg.ID)
		}
	}

	// Cancel all the contexts that we created
	baseCancel()
	// Close the ping message ID channel
	close(pingMessageIDs)
	// Close all the results channel
	for _, results := range requestResults {
		close(results)
	}
}

func (s serverSession) handleInitializeRequest(msg JSONRPCMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), s.sendTimeout)
	defer cancel()

	// Verify client's initialization request
	res, err := s.initializationHandshake(msg)
	if err != nil {
		s.logger.Info("invalid initialization request", slog.String("err", err.Error()))
		// Initialization failed, send the error to the client to notify them to close the session.
		if err := s.session.Send(ctx, JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			ID:      msg.ID,
			Error: &JSONRPCError{
				Code:    jsonRPCInvalidParamsCode,
				Message: err.Error(),
			},
		}); err != nil {
			s.logger.Error("failed to send initialization error", slog.String("err", err.Error()))
		}
		return
	}
	resBs, _ := json.Marshal(res)
	if err := s.session.Send(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      msg.ID,
		Result:  resBs,
	}); err != nil {
		s.logger.Error("failed to send initialization result", slog.String("err", err.Error()))
	}
}

func (s serverSession) ping(messageIDs <-chan MustString, done <-chan struct{}) {
	defer s.session.Stop()

	pingTicker := time.NewTicker(s.pingInterval)
	failedPings := 0
	var msgID MustString

	for {
		if failedPings > s.pingTimeoutThreshold {
			s.logger.Warn("too many pings failed, closing session")
			return
		}

		select {
		case <-done:
			return
		case id := <-messageIDs:
			// Received id from client response, check whether it's the same as the one we sent.
			if id != msgID {
				continue
			}
			// If it's the same, we received a ping response, reset the failed ping counter.
			// s.logger.Info("received ping response, resetting failed ping counter")
			failedPings = 0
			continue
		case <-pingTicker.C:
		}

		ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)

		// Send the ping message to the client.
		msgID = MustString(uuid.New().String())

		if err := s.session.Send(ctx, JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			ID:      msgID,
			Method:  methodPing,
		}); err != nil {
			s.logger.Warn("failed to send ping to client",
				slog.String("err", err.Error()))
			failedPings++
		}
		cancel()
	}
}

func (s serverSession) handleServerImplementationMessage(
	ctx context.Context,
	msg JSONRPCMessage,
	results <-chan JSONRPCMessage,
) {
	// This variables is used to store all the result from the server implementation
	// to be sent back to the client below.
	var result any
	// The err is should always an instance of JSONRPCError, we declare it as an error type,
	// is for the nil-check feature.
	var err error

	switch msg.Method {
	case MethodPromptsList:
		result, err = s.callListPrompts(ctx, msg, results)
	case MethodPromptsGet:
		result, err = s.callGetPrompt(ctx, msg, results)
	case MethodResourcesList:
		result, err = s.callListResources(ctx, msg, results)
	case MethodResourcesRead:
		result, err = s.callReadResource(ctx, msg, results)
	case MethodResourcesTemplatesList:
		result, err = s.callListResourceTemplates(ctx, msg, results)
	case MethodResourcesSubscribe:
		err = s.callSubscribeResource(msg)
	case MethodResourcesUnsubscribe:
		err = s.callUnsubscribeResource(msg)
	case MethodToolsList:
		result, err = s.callListTools(ctx, msg, results)
	case MethodToolsCall:
		result, err = s.callCallTool(ctx, msg, results)
	case MethodCompletionComplete:
		var params CompletesCompletionParams
		if err = json.Unmarshal(msg.Params, &params); err != nil {
			err = JSONRPCError{
				Code:    jsonRPCInvalidParamsCode,
				Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
			}
			break
		}
		switch params.Ref.Type {
		case CompletionRefPrompt:
			result, err = s.callCompletePrompt(ctx, msg, results)
		case CompletionRefResource:
			result, err = s.callCompleteResource(ctx, msg, results)
		default:
		}
	case MethodLoggingSetLevel:
		err = s.callSetLogLevel(msg)
	default:
		return
	}

	resMsg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      msg.ID,
	}

	if err != nil {
		jsonErr := JSONRPCError{}
		if errors.As(err, &jsonErr) {
			s.logger.Error("failed to call server implementation",
				slog.String("method", msg.Method),
				slog.String("err", err.Error()))
			resMsg.Error = &jsonErr
		}
	} else if result != nil {
		// Some call doesn't return any result, so this nil check is needed.
		resMsg.Result, _ = json.Marshal(result)
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.sendTimeout)
	defer cancel()

	if err := s.session.Send(ctx, resMsg); err != nil {
		s.logger.Error("failed to send result", slog.String("err", err.Error()))
	}
}

func (s serverSession) initializationHandshake(msg JSONRPCMessage) (initializeResult, error) {
	var params initializeParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return initializeResult{}, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: fmt.Sprintf("failed to unmarshal params: %s", err.Error()),
		}
	}

	if params.ProtocolVersion != protocolVersion {
		return initializeResult{}, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: fmt.Sprintf("protocol version mismatch: %s != %s", params.ProtocolVersion, protocolVersion),
		}
	}

	if s.requiredClientCap.Roots != nil {
		if params.Capabilities.Roots == nil {
			return initializeResult{}, JSONRPCError{
				Code:    jsonRPCInvalidParamsCode,
				Message: "insufficient client capabilities: missing required capability 'roots'",
			}
		}
		if s.requiredClientCap.Roots.ListChanged {
			if !params.Capabilities.Roots.ListChanged {
				return initializeResult{}, JSONRPCError{
					Code:    jsonRPCInvalidParamsCode,
					Message: "insufficient client capabilities: missing required capability 'roots.listChanged'",
				}
			}
		}
	}

	if s.requiredClientCap.Sampling != nil {
		if params.Capabilities.Sampling == nil {
			return initializeResult{}, JSONRPCError{
				Code:    jsonRPCInvalidParamsCode,
				Message: "insufficient client capabilities: missing required capability 'sampling'",
			}
		}
	}

	return initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    s.serverCap,
		ServerInfo:      s.serverInfo,
		Instructions:    s.instructions,
	}, nil
}

func (s serverSession) progressReporter(msgID MustString) func(ProgressParams) {
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

		ctx, cancel := context.WithTimeout(context.Background(), s.sendTimeout)
		defer cancel()

		if err := s.session.Send(ctx, msg); err != nil {
			s.logger.Error("failed to send message", "err", err)
		}
	}
}

func (s serverSession) clientRequester(msgID MustString, results <-chan JSONRPCMessage) RequestClientFunc {
	return func(msg JSONRPCMessage) (JSONRPCMessage, error) {
		ctx, cancel := context.WithTimeout(context.Background(), s.sendTimeout)
		defer cancel()

		// Override the message ID, so we can intercept the result correctly in the main loop.
		msg.ID = msgID
		if err := s.session.Send(ctx, msg); err != nil {
			return JSONRPCMessage{}, err
		}

		return <-results, nil
	}
}

func (s serverSession) callListPrompts(
	ctx context.Context,
	msg JSONRPCMessage,
	results <-chan JSONRPCMessage,
) (ListPromptResult, error) {
	if s.promptServer == nil {
		return ListPromptResult{}, JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "prompts not supported by server",
		}
	}

	var params ListPromptsParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return ListPromptResult{}, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
		}
	}

	ps, err := s.promptServer.ListPrompts(ctx, params, s.progressReporter(msg.ID), s.clientRequester(msg.ID, results))
	if err != nil {
		nErr := fmt.Errorf("failed to list prompts: %w", err)
		return ListPromptResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: nErr.Error(),
		}
	}

	return ps, nil
}

func (s serverSession) callGetPrompt(
	ctx context.Context,
	msg JSONRPCMessage,
	results <-chan JSONRPCMessage,
) (GetPromptResult, error) {
	if s.promptServer == nil {
		return GetPromptResult{}, JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "prompts not supported by server",
		}
	}

	var params GetPromptParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return GetPromptResult{}, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
		}
	}

	p, err := s.promptServer.GetPrompt(ctx, params, s.progressReporter(msg.ID), s.clientRequester(msg.ID, results))
	if err != nil {
		nErr := fmt.Errorf("failed to get prompt: %w", err)
		return GetPromptResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: nErr.Error(),
		}
	}

	return p, nil
}

func (s serverSession) callListResources(
	ctx context.Context,
	msg JSONRPCMessage,
	results <-chan JSONRPCMessage,
) (ListResourcesResult, error) {
	if s.resourceServer == nil {
		return ListResourcesResult{}, JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "resources not supported by server",
		}
	}

	var params ListResourcesParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return ListResourcesResult{}, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
		}
	}

	rs, err := s.resourceServer.ListResources(ctx, params, s.progressReporter(msg.ID), s.clientRequester(msg.ID, results))
	if err != nil {
		nErr := fmt.Errorf("failed to list resources: %w", err)
		return ListResourcesResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: nErr.Error(),
		}
	}

	return rs, nil
}

func (s serverSession) callReadResource(
	ctx context.Context,
	msg JSONRPCMessage,
	results <-chan JSONRPCMessage,
) (ReadResourceResult, error) {
	if s.resourceServer == nil {
		return ReadResourceResult{}, JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "resources not supported by server",
		}
	}

	var params ReadResourceParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return ReadResourceResult{}, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
		}
	}

	r, err := s.resourceServer.ReadResource(ctx, params, s.progressReporter(msg.ID), s.clientRequester(msg.ID, results))
	if err != nil {
		nErr := fmt.Errorf("failed to read resource: %w", err)
		return ReadResourceResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: nErr.Error(),
		}
	}

	return r, nil
}

func (s serverSession) callListResourceTemplates(
	ctx context.Context,
	msg JSONRPCMessage,
	results <-chan JSONRPCMessage,
) (ListResourceTemplatesResult, error) {
	if s.resourceServer == nil {
		return ListResourceTemplatesResult{}, JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "resources not supported by server",
		}
	}

	var params ListResourceTemplatesParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return ListResourceTemplatesResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
		}
	}

	ts, err := s.resourceServer.ListResourceTemplates(ctx, params,
		s.progressReporter(msg.ID), s.clientRequester(msg.ID, results))
	if err != nil {
		nErr := fmt.Errorf("failed to list resource templates: %w", err)
		return ListResourceTemplatesResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: nErr.Error(),
		}
	}

	return ts, nil
}

func (s serverSession) callSubscribeResource(msg JSONRPCMessage) error {
	if s.resourceSubscriptionHandler == nil {
		return JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "resources subscription not supported by server",
		}
	}

	var params SubscribeResourceParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
		}
	}

	s.resourceSubscriptionHandler.SubscribeResource(params)

	return nil
}

func (s serverSession) callUnsubscribeResource(msg JSONRPCMessage) error {
	if s.resourceSubscriptionHandler == nil {
		return JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "resources subscription not supported by server",
		}
	}

	var params UnsubscribeResourceParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
		}
	}

	s.resourceSubscriptionHandler.UnsubscribeResource(params)

	return nil
}

func (s serverSession) callCompletePrompt(
	ctx context.Context,
	msg JSONRPCMessage,
	results <-chan JSONRPCMessage,
) (CompletionResult, error) {
	if s.promptServer == nil {
		return CompletionResult{}, JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "prompts not supported by server",
		}
	}

	var params CompletesCompletionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return CompletionResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
		}
	}

	result, err := s.promptServer.CompletesPrompt(ctx, params, s.clientRequester(msg.ID, results))
	if err != nil {
		nErr := fmt.Errorf("failed to complete prompt: %w", err)
		return CompletionResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: nErr.Error(),
		}
	}

	return result, nil
}

func (s serverSession) callCompleteResource(
	ctx context.Context,
	msg JSONRPCMessage,
	results <-chan JSONRPCMessage,
) (CompletionResult, error) {
	if s.resourceServer == nil {
		return CompletionResult{}, JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "resources not supported by server",
		}
	}

	var params CompletesCompletionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return CompletionResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
		}
	}

	result, err := s.resourceServer.CompletesResourceTemplate(ctx, params, s.clientRequester(msg.ID, results))
	if err != nil {
		nErr := fmt.Errorf("failed to complete resource template: %w", err)
		return CompletionResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: nErr.Error(),
		}
	}

	return result, nil
}

func (s serverSession) callListTools(
	ctx context.Context,
	msg JSONRPCMessage,
	results <-chan JSONRPCMessage,
) (ListToolsResult, error) {
	if s.toolServer == nil {
		return ListToolsResult{}, JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "tools not supported by server",
		}
	}

	var params ListToolsParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return ListToolsResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
		}
	}

	ts, err := s.toolServer.ListTools(ctx, params, s.progressReporter(msg.ID), s.clientRequester(msg.ID, results))
	if err != nil {
		nErr := fmt.Errorf("failed to list tools: %w", err)
		return ListToolsResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: nErr.Error(),
		}
	}

	return ts, nil
}

func (s serverSession) callCallTool(
	ctx context.Context,
	msg JSONRPCMessage,
	results <-chan JSONRPCMessage,
) (CallToolResult, error) {
	if s.toolServer == nil {
		return CallToolResult{}, JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "tools not supported by server",
		}
	}

	var params CallToolParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return CallToolResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
		}
	}

	result, err := s.toolServer.CallTool(ctx, params, s.progressReporter(msg.ID), s.clientRequester(msg.ID, results))
	if err != nil {
		result = CallToolResult{
			Content: []Content{
				{
					Type: ContentTypeText,
					Text: err.Error(),
				},
			},
			IsError: true,
		}
	}

	return result, nil
}

func (s serverSession) callSetLogLevel(msg JSONRPCMessage) error {
	if s.logHandler == nil {
		return JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "logging not supported by server",
		}
	}

	var params LogParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: fmt.Errorf("failed to unmarshal params: %w", err).Error(),
		}
	}

	s.logHandler.SetLogLevel(params.Level)

	return nil
}
