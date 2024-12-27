package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
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
	capabilities               ServerCapabilities
	info                       Info
	requiredClientCapabilities ClientCapabilities
	transport                  ServerTransport

	sessions   *sync.Map // map[sessionID]*serverSession
	progresses *sync.Map // map[progressToken]sessionID

	promptServer      PromptServer
	promptListUpdater PromptListUpdater

	resourceServer            ResourceServer
	resourceListUpdater       ResourceListUpdater
	resourceSubscribedUpdater ResourceSubscribedUpdater

	toolServer      ToolServer
	toolListUpdater ToolListUpdater

	rootsListWatcher RootsListWatcher

	logHandler       LogHandler
	progressReporter ProgressReporter

	writeTimeout time.Duration
	readTimeout  time.Duration
	pingInterval time.Duration

	sessionStopChan chan string
	errsChan        chan error
	closeChan       chan struct{}
}

type session struct {
	id        string
	ctx       context.Context
	cancel    context.CancelFunc
	transport ServerTransport

	writeTimeout time.Duration
	readTimeout  time.Duration
	pingInterval time.Duration

	// clientRequests is a map of requestID to request, used for cancelling requests
	clientRequests sync.Map
	// serverRequests is a map of requestID to chan JSONRPCMessage, used for mapping the result to the original request
	serverRequests      sync.Map
	subscribedResources sync.Map // map[uri]struct{}

	promptsListChan        chan struct{}
	resourcesListChan      chan struct{}
	resourcesSubscribeChan chan string
	toolsListChan          chan struct{}
	logChan                chan LogParams
	progressChan           chan ProgressParams
	errsChan               chan error
	stopChan               chan<- string

	initLock    sync.RWMutex
	initialized bool
}

type request struct {
	ctx    context.Context
	cancel context.CancelFunc
}

var (
	defaultServerWriteTimeout = 30 * time.Second
	defaultServerReadTimeout  = 30 * time.Second

	errInvalidJSON     = errors.New("invalid json")
	errSessionNotFound = errors.New("session not found")
)

// Serve starts a Model Context Protocol (MCP) server and manages its lifecycle. It handles
// client connections, protocol messages, and server capabilities according to the MCP specification.
//
// The server parameter must implement the Server interface to define core MCP capabilities
// like prompts, resources, and tools. The transport parameter specifies how the server
// communicates with clients (e.g., HTTP, WebSocket, stdio). Errors encountered during
// server operation are sent to errsChan.
//
// Serve blocks until the provided context is cancelled, at which point it performs
// a graceful shutdown by closing all active sessions and cleaning up resources.
//
// Example usage:
//
//	srv := &MyMCPServer{} // implements Server interface
//	transport := mcp.NewHTTPTransport(":8080")
//	errChan := make(chan error, 10)
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	// Start server with custom options
//	mcp.Serve(ctx, srv, transport, errChan,
//	    mcp.WithServerWriteTimeout(30*time.Second),
//	    mcp.WithProgressReporter(myReporter),
//	)
func Serve(
	ctx context.Context,
	server Server,
	transport ServerTransport,
	errsChan chan error,
	options ...ServerOption,
) {
	s := newServer(server, transport, errsChan, options...)
	s.start()

	<-ctx.Done()
	s.stop()
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

// WithResourceSubscribedUpdater sets the resource subscribe watcher for the server.
func WithResourceSubscribedUpdater(updater ResourceSubscribedUpdater) ServerOption {
	return func(s *server) {
		s.resourceSubscribedUpdater = updater
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

// WithProgressReporter sets the progress reporter for the server.
func WithProgressReporter(reporter ProgressReporter) ServerOption {
	return func(s *server) {
		s.progressReporter = reporter
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

func newServer(srv Server, transport ServerTransport, errsChan chan error, options ...ServerOption) server {
	s := server{
		info:            srv.Info(),
		transport:       transport,
		sessions:        new(sync.Map),
		progresses:      new(sync.Map),
		sessionStopChan: make(chan string),
		errsChan:        errsChan,
		closeChan:       make(chan struct{}),
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
		if s.resourceSubscribedUpdater != nil {
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

func (s server) start() {
	if s.promptListUpdater != nil {
		go s.listenPromptsList()
	}
	if s.resourceListUpdater != nil {
		go s.listenResourcesList()
	}
	if s.resourceSubscribedUpdater != nil {
		go s.listenResourcesSubscribe()
	}
	if s.toolListUpdater != nil {
		go s.listenToolsList()
	}

	if s.logHandler != nil {
		go s.listenLog()
	}
	if s.progressReporter != nil {
		go s.listenProgress()
	}

	go s.listenSessions()
}

func (s server) listenSessions() {
	ctxs := s.transport.Sessions()
	msgs := s.transport.SessionMessages()

	for {
		select {
		case <-s.closeChan:
			return
		case id := <-s.sessionStopChan:
			s.sessions.Delete(id)
		case ctx := <-ctxs:
			s.startSession(ctx.Ctx, ctx.ID)
		case msg := <-msgs:
			msg.Errs <- s.handleMsg(msg.SessionID, msg.Msg)
		}
	}
}

func (s server) listenPromptsList() {
	lists := s.promptListUpdater.PromptListUpdates()

	for {
		select {
		case <-s.closeChan:
			return
		case <-lists:
		}

		s.sessions.Range(func(_, value any) bool {
			sess, _ := value.(*session)
			sess.promptsListChan <- struct{}{}
			return true
		})
	}
}

func (s server) listenResourcesList() {
	lists := s.resourceListUpdater.ResourceListUpdates()

	for {
		select {
		case <-s.closeChan:
			return
		case <-lists:
		}

		s.sessions.Range(func(_, value any) bool {
			sess, _ := value.(*session)
			sess.resourcesListChan <- struct{}{}
			return true
		})
	}
}

func (s server) listenResourcesSubscribe() {
	subscribes := s.resourceSubscribedUpdater.ResourceSubscribedUpdates()
	var uri string

	for {
		select {
		case <-s.closeChan:
			return
		case uri = <-subscribes:
		}

		s.sessions.Range(func(_, value any) bool {
			sess, _ := value.(*session)
			sess.resourcesSubscribeChan <- uri
			return true
		})
	}
}

func (s server) listenToolsList() {
	lists := s.toolListUpdater.ToolListUpdates()

	for {
		select {
		case <-s.closeChan:
			return
		case <-lists:
		}

		s.sessions.Range(func(_, value any) bool {
			sess, _ := value.(*session)
			sess.toolsListChan <- struct{}{}
			return true
		})
	}
}

func (s server) listenLog() {
	logs := s.logHandler.LogStreams()
	var params LogParams

	for {
		select {
		case <-s.closeChan:
			return
		case params = <-logs:
		}

		s.sessions.Range(func(_, value any) bool {
			sess, _ := value.(*session)
			sess.logChan <- params
			return true
		})
	}
}

func (s server) listenProgress() {
	progresses := s.progressReporter.ProgressReports()
	var params ProgressParams

	for {
		select {
		case <-s.closeChan:
			return
		case params = <-progresses:
		}

		sessID, ok := s.progresses.Load(params.ProgressToken)
		if !ok {
			continue
		}
		ss, ok := s.sessions.Load(sessID)
		if !ok {
			continue
		}
		sess, _ := ss.(*session)
		sess.progressChan <- params
	}
}

func (s server) startSession(ctx context.Context, sessID string) {
	sCtx, sCancel := context.WithCancel(ctx)

	sess := &session{
		id:                     sessID,
		ctx:                    sCtx,
		cancel:                 sCancel,
		transport:              s.transport,
		writeTimeout:           s.writeTimeout,
		readTimeout:            s.readTimeout,
		pingInterval:           s.pingInterval,
		promptsListChan:        make(chan struct{}),
		resourcesListChan:      make(chan struct{}),
		resourcesSubscribeChan: make(chan string),
		toolsListChan:          make(chan struct{}),
		logChan:                make(chan LogParams),
		progressChan:           make(chan ProgressParams),
		errsChan:               s.errsChan,
		stopChan:               s.sessionStopChan,
	}

	s.sessions.Store(sessID, sess)
	go sess.listen()
	if s.pingInterval > 0 {
		go sess.pings()
	}
}

func (s server) handleMsg(sessionID string, msg JSONRPCMessage) error {
	if msg.JSONRPC != JSONRPCVersion {
		return errInvalidJSON
	}

	ss, ok := s.sessions.Load(sessionID)
	if !ok {
		return errSessionNotFound
	}
	sess, _ := ss.(*session)

	// We musn't wait for the below handler to finish, as it might be blocking
	// the client's request, and since these handlers might 'call' the client back,
	// that would cause a deadlock. So, in each handlers below, once the params
	// is proven to be valid, we launch a goroutine to continue the processing.

	// Handle basic protocol messages
	if err := s.handleBasicMessages(sess, msg); err != nil {
		return err
	}

	// Handle prompt-related messages
	if err := s.handlePromptMessages(sess, msg); err != nil {
		return err
	}

	// Handle resource-related messages
	if err := s.handleResourceMessages(sess, msg); err != nil {
		return err
	}

	// Handle tool-related messages
	if err := s.handleToolMessages(sess, msg); err != nil {
		return err
	}

	// Handle completion messages
	if err := s.handleCompletionMessages(sess, msg); err != nil {
		return err
	}

	// Handle notification messages
	if err := s.handleNotificationMessages(sess, msg); err != nil {
		return err
	}

	// Handle result messages
	s.handleResultMessages(sess, msg)

	// Handle logging messages
	if err := s.handleLoggingMessages(sess, msg); err != nil {
		return err
	}

	return nil
}

func (s server) handleBasicMessages(sess *session, msg JSONRPCMessage) error {
	switch msg.Method {
	case methodPing:
		go sess.handlePing(msg.ID)
		return nil
	case methodInitialize:
		var params initializeParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		go sess.handleInitialize(msg.ID, params, s.capabilities,
			s.requiredClientCapabilities, s.info)
		return nil
	}
	return nil
}

func (s server) handlePromptMessages(sess *session, msg JSONRPCMessage) error {
	if s.promptServer == nil {
		return nil
	}

	switch msg.Method {
	case MethodPromptsList:
		var params PromptsListParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		if params.Meta.ProgressToken != "" {
			s.progresses.Store(params.Meta.ProgressToken, sess.id)
		}
		go sess.handlePromptsList(msg.ID, params, s.promptServer)
		return nil
	case MethodPromptsGet:
		var params PromptsGetParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		if params.Meta.ProgressToken != "" {
			s.progresses.Store(params.Meta.ProgressToken, sess.id)
		}
		go sess.handlePromptsGet(msg.ID, params, s.promptServer)
		return nil
	}
	return nil
}

func (s server) handleResourceMessages(sess *session, msg JSONRPCMessage) error {
	if s.resourceServer == nil {
		return nil
	}

	switch msg.Method {
	case MethodResourcesList:
		var params ResourcesListParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		if params.Meta.ProgressToken != "" {
			s.progresses.Store(params.Meta.ProgressToken, sess.id)
		}
		go sess.handleResourcesList(msg.ID, params, s.resourceServer)
		return nil
	case MethodResourcesRead:
		var params ResourcesReadParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		if params.Meta.ProgressToken != "" {
			s.progresses.Store(params.Meta.ProgressToken, sess.id)
		}
		go sess.handleResourcesRead(msg.ID, params, s.resourceServer)
		return nil
	case MethodResourcesTemplatesList:
		var params ResourcesTemplatesListParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		if params.Meta.ProgressToken != "" {
			s.progresses.Store(params.Meta.ProgressToken, sess.id)
		}
		go sess.handleResourcesListTemplates(msg.ID, params, s.resourceServer)
		return nil
	case MethodResourcesSubscribe:
		var params ResourcesSubscribeParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		go sess.handleResourcesSubscribe(msg.ID, params, s.resourceServer)
		return nil
	case MethodResourcesUnsubscribe:
		var params ResourcesSubscribeParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		go sess.handleResourcesUnsubscribe(msg.ID, params, s.resourceServer)
		return nil
	}
	return nil
}

func (s server) handleToolMessages(sess *session, msg JSONRPCMessage) error {
	if s.toolServer == nil {
		return nil
	}

	switch msg.Method {
	case MethodToolsList:
		var params ToolsListParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		if params.Meta.ProgressToken != "" {
			s.progresses.Store(params.Meta.ProgressToken, sess.id)
		}
		go sess.handleToolsList(msg.ID, params, s.toolServer)
		return nil
	case MethodToolsCall:
		var params ToolsCallParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		if params.Meta.ProgressToken != "" {
			s.progresses.Store(params.Meta.ProgressToken, sess.id)
		}
		go sess.handleToolsCall(msg.ID, params, s.toolServer)
		return nil
	}
	return nil
}

func (s server) handleCompletionMessages(sess *session, msg JSONRPCMessage) error {
	if msg.Method != MethodCompletionComplete {
		return nil
	}

	var params CompletionCompleteParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}

	switch params.Ref.Type {
	case CompletionRefPrompt:
		go sess.handleCompletePrompt(msg.ID, params, s.promptServer)
		return nil
	case CompletionRefResource:
		go sess.handleCompleteResource(msg.ID, params, s.resourceServer)
		return nil
	}
	return nil
}

func (s server) handleNotificationMessages(sess *session, msg JSONRPCMessage) error {
	switch msg.Method {
	case methodNotificationsInitialized:
		go sess.handleNotificationsInitialized()
	case methodNotificationsCancelled:
		var params notificationsCancelledParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		go sess.handleNotificationsCancelled(params)
	case methodNotificationsRootsListChanged:
		if s.rootsListWatcher != nil {
			s.rootsListWatcher.OnRootsListChanged()
		}
	}

	return nil
}

func (s server) handleResultMessages(sess *session, msg JSONRPCMessage) {
	if msg.Method != "" {
		return
	}

	go sess.handleResult(msg)
}

func (s server) handleLoggingMessages(sess *session, msg JSONRPCMessage) error {
	if s.logHandler == nil {
		return nil
	}

	if msg.Method != MethodLoggingSetLevel {
		return nil
	}

	var params LogParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}
	go sess.handleLoggingSetLevel(msg.ID, params, s.logHandler)

	return nil
}

func (s server) stop() {
	s.sessions.Range(func(_, value any) bool {
		sess, _ := value.(*session)
		sess.cancel()
		return true
	})
	close(s.errsChan)
	close(s.closeChan)
	s.transport.Close()
}

func (s *session) listen() {
	for {
		select {
		case <-s.ctx.Done():
			s.stopChan <- s.id
			return
		case <-s.promptsListChan:
			s.sendNotification(methodNotificationsPromptsListChanged, nil)
		case <-s.resourcesListChan:
			s.sendNotification(methodNotificationsResourcesListChanged, nil)
		case uri := <-s.resourcesSubscribeChan:
			_, ok := s.subscribedResources.Load(uri)
			if !ok {
				continue
			}
			s.sendNotification(methodNotificationsResourcesUpdated, notificationsResourcesUpdatedParams{
				URI: uri,
			})
		case <-s.toolsListChan:
			s.sendNotification(methodNotificationsToolsListChanged, nil)
		case params := <-s.logChan:
			s.sendNotification(methodNotificationsMessage, params)
		case params := <-s.progressChan:
			s.sendNotification(methodNotificationsProgress, params)
		}
	}
}

func (s *session) pings() {
	pingTicker := time.NewTicker(s.pingInterval)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-pingTicker.C:
			s.ping()
		}
	}
}

func (s *session) handlePing(msgID MustString) {
	s.sendResult(msgID, nil)
}

func (s *session) handleInitialize(
	msgID MustString,
	params initializeParams,
	serverCap ServerCapabilities,
	requiredClientCap ClientCapabilities,
	serverInfo Info,
) {
	if params.ProtocolVersion != protocolVersion {
		nErr := fmt.Errorf("protocol version mismatch: %s != %s", params.ProtocolVersion, protocolVersion)
		s.logError(nErr)
		s.sendError(msgID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgUnsupportedProtocolVersion,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	if requiredClientCap.Roots != nil {
		if params.Capabilities.Roots == nil {
			nErr := fmt.Errorf("insufficient client capabilities: missing required capability 'roots'")
			s.logError(nErr)
			s.sendError(msgID, JSONRPCError{
				Code:    jsonRPCInvalidParamsCode,
				Message: errMsgInsufficientClientCapabilities,
				Data:    map[string]any{"error": nErr},
			})
			return
		}
		if requiredClientCap.Roots.ListChanged {
			if !params.Capabilities.Roots.ListChanged {
				nErr := fmt.Errorf("insufficient client capabilities: missing required capability 'roots.listChanged'")
				s.logError(nErr)
				s.sendError(msgID, JSONRPCError{
					Code:    jsonRPCInvalidParamsCode,
					Message: errMsgInsufficientClientCapabilities,
					Data:    map[string]any{"error": nErr},
				})
				return
			}
		}
	}

	if requiredClientCap.Sampling != nil {
		if params.Capabilities.Sampling == nil {
			nErr := fmt.Errorf("insufficient client capabilities: missing required capability 'sampling'")
			s.logError(nErr)
			s.sendError(msgID, JSONRPCError{
				Code:    jsonRPCInvalidParamsCode,
				Message: errMsgInsufficientClientCapabilities,
				Data:    map[string]any{"error": nErr},
			})
			return
		}
	}

	s.sendResult(msgID, initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    serverCap,
		ServerInfo:      serverInfo,
	})
}

func (s *session) handlePromptsList(
	msgID MustString,
	params PromptsListParams,
	server PromptServer,
) {
	if !s.isInitialized() {
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ps, err := server.ListPrompts(ctx, params, s.sendRequest)
	if err != nil {
		nErr := fmt.Errorf("failed to list prompts: %w", err)
		s.sendError(msgID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(msgID, ps)
}

func (s *session) handlePromptsGet(
	msgID MustString,
	params PromptsGetParams,
	server PromptServer,
) {
	if !s.isInitialized() {
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	p, err := server.GetPrompt(ctx, params, s.sendRequest)
	if err != nil {
		nErr := fmt.Errorf("failed to get prompt: %w", err)
		s.sendError(msgID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(msgID, p)
}

func (s *session) handleCompletePrompt(
	msgID MustString,
	params CompletionCompleteParams,
	server PromptServer,
) {
	if !s.isInitialized() {
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	result, err := server.CompletesPrompt(ctx, params, s.sendRequest)
	if err != nil {
		nErr := fmt.Errorf("failed to complete prompt: %w", err)
		s.sendError(msgID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(msgID, result)
}

func (s *session) handleResourcesList(
	msgID MustString,
	params ResourcesListParams,
	server ResourceServer,
) {
	if !s.isInitialized() {
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	rs, err := server.ListResources(ctx, params, s.sendRequest)
	if err != nil {
		nErr := fmt.Errorf("failed to list resources: %w", err)
		s.sendError(msgID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(msgID, rs)
}

func (s *session) handleResourcesRead(
	msgID MustString,
	params ResourcesReadParams,
	server ResourceServer,
) {
	if !s.isInitialized() {
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	r, err := server.ReadResource(ctx, params, s.sendRequest)
	if err != nil {
		nErr := fmt.Errorf("failed to read resource: %w", err)
		s.sendError(msgID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(msgID, r)
}

func (s *session) handleResourcesListTemplates(
	msgID MustString,
	params ResourcesTemplatesListParams,
	server ResourceServer,
) {
	if !s.isInitialized() {
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ts, err := server.ListResourceTemplates(ctx, params, s.sendRequest)
	if err != nil {
		nErr := fmt.Errorf("failed to list resource templates: %w", err)
		s.sendError(msgID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(msgID, ts)
}

func (s *session) handleResourcesSubscribe(
	msgID MustString,
	params ResourcesSubscribeParams,
	server ResourceServer,
) {
	if !s.isInitialized() {
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	server.SubscribeResource(params)
	s.subscribedResources.Store(params.URI, struct{}{})

	s.sendResult(msgID, nil)
}

func (s *session) handleResourcesUnsubscribe(
	msgID MustString,
	params ResourcesSubscribeParams,
	server ResourceServer,
) {
	if !s.isInitialized() {
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	server.UnsubscribeResource(params)
	s.subscribedResources.Delete(params.URI)

	s.sendResult(msgID, nil)
}

func (s *session) handleCompleteResource(
	msgID MustString,
	params CompletionCompleteParams,
	server ResourceServer,
) {
	if !s.isInitialized() {
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	result, err := server.CompletesResourceTemplate(ctx, params, s.sendRequest)
	if err != nil {
		nErr := fmt.Errorf("failed to complete resource template: %w", err)
		s.sendError(msgID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(msgID, result)
}

func (s *session) handleToolsList(
	msgID MustString,
	params ToolsListParams,
	server ToolServer,
) {
	if !s.isInitialized() {
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ts, err := server.ListTools(ctx, params, s.sendRequest)
	if err != nil {
		nErr := fmt.Errorf("failed to list tools: %w", err)
		s.sendError(msgID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(msgID, ts)
}

func (s *session) handleToolsCall(msgID MustString, params ToolsCallParams, server ToolServer) {
	if !s.isInitialized() {
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	result, err := server.CallTool(ctx, params, s.sendRequest)
	if err != nil {
		nErr := fmt.Errorf("failed to call tool: %w", err)
		s.sendError(msgID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		})
		return
	}

	s.sendResult(msgID, result)
}

func (s *session) handleNotificationsInitialized() {
	s.initLock.Lock()
	defer s.initLock.Unlock()

	s.initialized = true
}

func (s *session) handleNotificationsCancelled(params notificationsCancelledParams) {
	r, ok := s.clientRequests.Load(params.RequestID)
	if !ok {
		return
	}
	req, _ := r.(request)

	s.logError(fmt.Errorf("cancelled request %s: %s", params.RequestID, params.Reason))
	req.cancel()
}

func (s *session) handleResult(msg JSONRPCMessage) {
	reqID := string(msg.ID)
	rc, ok := s.serverRequests.Load(reqID)
	if !ok {
		return
	}
	resChan, _ := rc.(chan JSONRPCMessage)
	resChan <- msg
}

func (s *session) handleLoggingSetLevel(msgID MustString, params LogParams, handler LogHandler) {
	if !s.isInitialized() {
		return
	}

	handler.SetLogLevel(params.Level)

	s.sendResult(msgID, nil)
}

func (s *session) isInitialized() bool {
	s.initLock.RLock()
	defer s.initLock.RUnlock()

	return s.initialized
}

func (s *session) registerRequest() (string, chan JSONRPCMessage) {
	reqID := uuid.New().String()
	resChan := make(chan JSONRPCMessage)
	s.serverRequests.Store(reqID, resChan)
	return reqID, resChan
}

func (s *session) ping() {
	resMsg, err := s.sendRequest(JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      MustString(uuid.New().String()),
		Method:  methodPing,
		Params:  nil,
	})
	if err != nil {
		s.logError(fmt.Errorf("failed to send ping: %w", err))
		return
	}
	if resMsg.Error != nil {
		s.logError(fmt.Errorf("error response: %w", resMsg.Error))
		return
	}
}

func (s *session) sendNotification(method string, params any) {
	paramsBs, err := json.Marshal(params)
	if err != nil {
		s.logError(fmt.Errorf("failed to marshal params: %w", err))
		return
	}

	notif := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  paramsBs,
	}

	sCtx, sCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer sCancel()

	if err := s.transport.Send(sCtx, SessionMsg{
		SessionID: s.id,
		Msg:       notif,
	}); err != nil {
		s.logError(fmt.Errorf("failed to send notification: %w", err))
		return
	}
}

func (s *session) sendResult(id MustString, result any) {
	resBs, err := json.Marshal(result)
	if err != nil {
		s.logError(fmt.Errorf("failed to marshal result: %w", err))
		return
	}

	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  resBs,
	}

	sCtx, sCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer sCancel()

	if err := s.transport.Send(sCtx, SessionMsg{
		SessionID: s.id,
		Msg:       msg,
	}); err != nil {
		s.logError(fmt.Errorf("failed to send result: %w", err))
	}
}

func (s *session) sendError(id MustString, err JSONRPCError) {
	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   &err,
	}

	sCtx, sCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer sCancel()

	if err := s.transport.Send(sCtx, SessionMsg{
		SessionID: s.id,
		Msg:       msg,
	}); err != nil {
		s.logError(fmt.Errorf("failed to send error: %w", err))
	}
}

func (s *session) sendRequest(msg JSONRPCMessage) (JSONRPCMessage, error) {
	reqID, resChan := s.registerRequest()
	msg.ID = MustString(reqID)

	sCtx, sCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer sCancel()

	if err := s.transport.Send(sCtx, SessionMsg{
		SessionID: s.id,
		Msg:       msg,
	}); err != nil {
		s.logError(fmt.Errorf("failed to send request: %w", err))
		return JSONRPCMessage{}, err
	}

	ticker := time.NewTicker(s.readTimeout)

	var resMsg JSONRPCMessage

	select {
	case <-ticker.C:
		s.logError(fmt.Errorf("request timeout"))
		return JSONRPCMessage{}, fmt.Errorf("request timeout")
	case <-sCtx.Done():
		return JSONRPCMessage{}, sCtx.Err()
	case resMsg = <-resChan:
	}

	return resMsg, nil
}

func (s *session) logError(err error) {
	select {
	case s.errsChan <- err:
	default:
	}
}
