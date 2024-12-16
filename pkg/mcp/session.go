package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

type sessionDispatcher struct {
	capabilities               ServerCapabilities
	serverInfo                 Info
	requiredClientCapabilities ClientCapabilities

	sessions   *sync.Map // map[sessionID]*serverSession
	progresses *sync.Map // map[progressToken]sessionID

	promptServer      PromptServer
	promptListWatcher PromptListWatcher

	resourceServer            ResourceServer
	resourceListWatcher       ResourceListWatcher
	resourceSubscribesWatcher ResourceSubscribesWatcher

	toolServer      ToolServer
	toolListWatcher ToolListWatcher

	logWatcher       LogWatcher
	progressReporter ProgressReporter

	writeTimeout time.Duration
	pingInterval time.Duration

	sessionStopChan chan string
	closeChan       chan struct{}
}

type session struct {
	id      string
	ctx     context.Context
	cancel  context.CancelFunc
	writter io.Writer

	clientCapabilities ClientCapabilities
	clientInfo         Info

	writeTimeout time.Duration
	pingInterval time.Duration

	requests            sync.Map // map[requestID]clientRequest
	subscribedResources sync.Map // map[uri]struct{}

	promptsListChan        chan struct{}
	resourcesListChan      chan struct{}
	resourcesSubscribeChan chan string
	toolsListChan          chan struct{}
	logChan                chan LogParams
	progressChan           chan ProgressParams
	stopChan               chan<- string

	initLock    sync.RWMutex
	initialized bool
}

type clientRequest struct {
	ctx    context.Context
	cancel context.CancelFunc
}

var (
	defaultWriteTimeout = 30 * time.Second
	defaultPingInterval = 30 * time.Second

	errInvalidJSON     = errors.New("invalid json")
	errSessionNotFound = errors.New("session not found")
)

func newSessionDispatcher(server Server, options ...ServerOption) sessionDispatcher {
	s := sessionDispatcher{
		serverInfo:                 server.Info(),
		requiredClientCapabilities: server.RequiredClientCapabilities(),
		sessions:                   new(sync.Map),
		progresses:                 new(sync.Map),
		sessionStopChan:            make(chan string),
		closeChan:                  make(chan struct{}),
	}
	for _, opt := range options {
		opt(&s)
	}

	s.capabilities = ServerCapabilities{}

	if s.promptServer != nil {
		s.capabilities.Prompts = &PromptsCapability{}
		if s.promptListWatcher != nil {
			s.capabilities.Prompts.ListChanged = true
		}
	}
	if s.resourceServer != nil {
		s.capabilities.Resources = &ResourcesCapability{}
		if s.resourceListWatcher != nil {
			s.capabilities.Resources.ListChanged = true
		}
		if s.resourceSubscribesWatcher != nil {
			s.capabilities.Resources.Subscribe = true
		}
	}
	if s.toolServer != nil {
		s.capabilities.Tools = &ToolsCapability{}
		if s.toolListWatcher != nil {
			s.capabilities.Tools.ListChanged = true
		}
	}
	if s.logWatcher != nil {
		s.capabilities.Logging = &LoggingCapability{}
	}

	return s
}

func (s sessionDispatcher) start() {
	if s.writeTimeout == 0 {
		s.writeTimeout = defaultWriteTimeout
	}
	if s.pingInterval == 0 {
		s.pingInterval = defaultPingInterval
	}

	go s.listenStopSession()
	if s.promptListWatcher != nil {
		go s.listenPromptsList()
	}
	if s.resourceListWatcher != nil {
		go s.listenResourcesList()
	}
	if s.resourceSubscribesWatcher != nil {
		go s.listenResourcesSubscribe()
	}
	if s.toolListWatcher != nil {
		go s.listenToolsList()
	}
	if s.logWatcher != nil {
		go s.listenLog()
	}
	if s.progressReporter != nil {
		go s.listenProgress()
	}
}

func (s sessionDispatcher) listenStopSession() {
	for {
		var id string
		select {
		case <-s.closeChan:
			return
		case id = <-s.sessionStopChan:
		}
		s.sessions.Delete(id)
	}
}

func (s sessionDispatcher) listenPromptsList() {
	lists := s.promptListWatcher.WatchPromptList()

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

func (s sessionDispatcher) listenResourcesList() {
	lists := s.resourceListWatcher.WatchResourceList()

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

func (s sessionDispatcher) listenResourcesSubscribe() {
	subscribes := s.resourceSubscribesWatcher.WatchResourceSubscribed()
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

func (s sessionDispatcher) listenToolsList() {
	lists := s.toolListWatcher.WatchToolList()

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

func (s sessionDispatcher) listenLog() {
	logs := s.logWatcher.WatchLog()
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

func (s sessionDispatcher) listenProgress() {
	progresses := s.progressReporter.ReportProgress()
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

func (s sessionDispatcher) startSession(ctx context.Context, w io.Writer) string {
	sCtx, sCancel := context.WithCancel(ctx)

	sessID := uuid.New().String()
	sess := &session{
		id:                     sessID,
		ctx:                    sCtx,
		cancel:                 sCancel,
		writter:                w,
		writeTimeout:           s.writeTimeout,
		pingInterval:           s.pingInterval,
		stopChan:               s.sessionStopChan,
		promptsListChan:        make(chan struct{}),
		resourcesListChan:      make(chan struct{}),
		resourcesSubscribeChan: make(chan string),
		toolsListChan:          make(chan struct{}),
		logChan:                make(chan LogParams),
		progressChan:           make(chan ProgressParams),
	}

	s.sessions.Store(sessID, sess)
	go sess.listen()

	return sessID
}

func (s sessionDispatcher) handleMsg(r io.Reader, sessionID string) error {
	msg, err := readMessage(r)
	if err != nil {
		return errInvalidJSON
	}

	ss, ok := s.sessions.Load(sessionID)
	if !ok {
		return errSessionNotFound
	}
	sess, _ := ss.(*session)

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

	return nil
}

func (s sessionDispatcher) handleBasicMessages(sess *session, msg jsonRPCMessage) error {
	switch msg.Method {
	case methodPing:
		return sess.handlePing(msg.ID)
	case methodInitialize:
		var params initializeParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		return sess.handleInitialize(msg.ID, params, s.capabilities,
			s.requiredClientCapabilities, s.serverInfo)
	}
	return nil
}

func (s sessionDispatcher) handlePromptMessages(sess *session, msg jsonRPCMessage) error {
	if s.promptServer == nil {
		return nil
	}

	switch msg.Method {
	case methodPromptsList:
		var params promptsListParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		return sess.handlePromptsList(msg.ID, params, s.promptServer)
	case methodPromptsGet:
		var params promptsGetParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		return sess.handlePromptsGet(msg.ID, params, s.promptServer)
	}
	return nil
}

func (s sessionDispatcher) handleResourceMessages(sess *session, msg jsonRPCMessage) error {
	if s.resourceServer == nil {
		return nil
	}

	switch msg.Method {
	case methodResourcesList:
		var params resourcesListParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		return sess.handleResourcesList(msg.ID, params, s.resourceServer)
	case methodResourcesRead:
		var params resourcesReadParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		return sess.handleResourcesRead(msg.ID, params, s.resourceServer)
	case methodResourcesTemplatesList:
		var params resourcesTemplatesListParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		return sess.handleResourcesListTemplates(msg.ID, params, s.resourceServer)
	case methodResourcesSubscribe:
		var params resourcesSubscribeParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		return sess.handleResourcesSubscribe(msg.ID, params, s.resourceServer)
	}
	return nil
}

func (s sessionDispatcher) handleToolMessages(sess *session, msg jsonRPCMessage) error {
	if s.toolServer == nil {
		return nil
	}

	switch msg.Method {
	case methodToolsList:
		var params toolsListParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		return sess.handleToolsList(msg.ID, params, s.toolServer)
	case methodToolsCall:
		var params toolsCallParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		return sess.handleToolsCall(msg.ID, params, s.toolServer)
	}
	return nil
}

func (s sessionDispatcher) handleCompletionMessages(sess *session, msg jsonRPCMessage) error {
	if msg.Method != methodCompletionComplete {
		return nil
	}

	var params completionCompleteParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}

	switch params.Ref.Type {
	case "ref/prompt":
		return sess.handleCompletePrompt(msg.ID, params.Ref.Name, params.Argument, s.promptServer)
	case "ref/resource":
		return sess.handleCompleteResource(msg.ID, params.Ref.Name, params.Argument, s.resourceServer)
	}
	return nil
}

func (s sessionDispatcher) handleNotificationMessages(sess *session, msg jsonRPCMessage) error {
	switch msg.Method {
	case methodNotificationsInitialized:
		sess.handleNotificationsInitialized()
	case methodNotificationsCancelled:
		var params notificationsCancelledParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		sess.handleNotificationsCancelled(params)
	}

	return nil
}

func (s sessionDispatcher) stop() {
	s.sessions.Range(func(_, value any) bool {
		sess, _ := value.(*session)
		sess.cancel()
		return true
	})
	close(s.closeChan)
}

func (s *session) listen() {
	pingTicker := time.NewTicker(s.pingInterval)

	for {
		select {
		case <-s.ctx.Done():
			s.stopChan <- s.id
			return
		case <-s.promptsListChan:
			_ = writeResult(s.ctx, s.writter, methodNotificationsPromptsListChanged, nil)
		case <-s.resourcesListChan:
			_ = writeResult(s.ctx, s.writter, methodNotificationsResourcesListChanged, nil)
		case uri := <-s.resourcesSubscribeChan:
			_, ok := s.subscribedResources.Load(uri)
			if !ok {
				continue
			}
			_ = writeResult(s.ctx, s.writter, methodNotificationsResourcesSubscribe, uri)
		case <-s.toolsListChan:
			_ = writeResult(s.ctx, s.writter, methodNotificationsToolsListChanged, nil)
		case params := <-s.logChan:
			_ = writeResult(s.ctx, s.writter, methodNotificationsMessage, params)
		case params := <-s.progressChan:
			_ = writeResult(s.ctx, s.writter, methodNotificationsProgress, params)
		case <-pingTicker.C:
			_ = writeResult(s.ctx, s.writter, methodPing, nil)
		}
	}
}

func (s *session) handlePing(msgID MustString) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer cancel()

	return writeResult(ctx, s.writter, msgID, nil)
}

func (s *session) handleInitialize(
	msgID MustString,
	params initializeParams,
	serverCap ServerCapabilities,
	requiredClientCap ClientCapabilities,
	serverInfo Info,
) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer cancel()

	if params.ProtocolVersion != protocolVersion {
		nErr := fmt.Errorf("protocol version mismatch: %s != %s", params.ProtocolVersion, protocolVersion)
		log.Print(nErr)
		return s.sendError(ctx, jsonRPCInvalidParamsCode, errMsgUnsupportedProtocolVersion, msgID, nErr)
	}

	if requiredClientCap.Roots != nil {
		if params.Capabilities.Roots == nil {
			nErr := fmt.Errorf("insufficient client capabilities: missing required capability 'roots'")
			log.Print(nErr)
			return s.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientClientCapabilities, msgID, nErr)
		}
		if requiredClientCap.Roots.ListChanged {
			if !params.Capabilities.Roots.ListChanged {
				nErr := fmt.Errorf("insufficient client capabilities: missing required capability 'roots.listChanged'")
				log.Print(nErr)
				return s.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientClientCapabilities, msgID, nErr)
			}
		}
	}

	if requiredClientCap.Sampling != nil {
		if params.Capabilities.Sampling == nil {
			nErr := fmt.Errorf("insufficient client capabilities: missing required capability 'sampling'")
			log.Print(nErr)
			return s.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientClientCapabilities, msgID, nErr)
		}
	}

	s.clientCapabilities = params.Capabilities
	s.clientInfo = params.ClientInfo

	return writeResult(ctx, s.writter, msgID, initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    serverCap,
		ServerInfo:      serverInfo,
	})
}

func (s *session) handlePromptsList(
	msgID MustString,
	params promptsListParams,
	server PromptServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	ps, err := server.ListPrompts(ctx, params.Cursor, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, ps)
}

func (s *session) handlePromptsGet(
	msgID MustString,
	params promptsGetParams,
	server PromptServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	p, err := server.GetPrompt(ctx, params.Name, params.Arguments, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, p)
}

func (s *session) handleCompletePrompt(
	msgID MustString,
	name string,
	argument CompletionArgument,
	server PromptServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	result, err := server.CompletesPrompt(ctx, name, argument)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, result)
}

func (s *session) handleResourcesList(
	msgID MustString,
	params resourcesListParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	rs, err := server.ListResources(ctx, params.Cursor, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, rs)
}

func (s *session) handleResourcesRead(
	msgID MustString,
	params resourcesReadParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	r, err := server.ReadResource(ctx, params.URI, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, r)
}

func (s *session) handleResourcesListTemplates(
	msgID MustString,
	params resourcesTemplatesListParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	ts, err := server.ListResourceTemplates(ctx, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, ts)
}

func (s *session) handleResourcesSubscribe(
	msgID MustString,
	params resourcesSubscribeParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	server.SubscribeResource(params.URI)
	s.subscribedResources.Store(params.URI, struct{}{})

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, nil)
}

func (s *session) handleToolsList(
	msgID MustString,
	params toolsListParams,
	server ToolServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	ts, err := server.ListTools(ctx, params.Cursor, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, ts)
}

func (s *session) handleToolsCall(
	msgID MustString,
	params toolsCallParams,
	server ToolServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	result, err := server.CallTool(ctx, params.Name, params.Arguments, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, result)
}

func (s *session) handleCompleteResource(
	msgID MustString,
	name string,
	argument CompletionArgument,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	result, err := server.CompletesResourceTemplate(ctx, name, argument)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, result)
}

func (s *session) handleNotificationsInitialized() {
	s.initLock.Lock()
	defer s.initLock.Unlock()

	s.initialized = true
}

func (s *session) handleNotificationsCancelled(params notificationsCancelledParams) {
	s.initLock.Lock()
	defer s.initLock.Unlock()

	r, ok := s.requests.Load(params.RequestID)
	if !ok {
		return
	}
	req, _ := r.(clientRequest)

	log.Printf("Cancelled request %s: %s", params.RequestID, params.Reason)
	req.cancel()
}

func (s *session) isInitialized() bool {
	s.initLock.RLock()
	defer s.initLock.RUnlock()

	return s.initialized
}

func (s *session) sendError(ctx context.Context, code int, message string, msgID MustString, err error) error {
	return writeError(ctx, s.writter, msgID, jsonRPCError{
		Code:    code,
		Message: message,
		Data:    map[string]any{"error": err},
	})
}
