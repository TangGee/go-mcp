package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log"
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
	closeChan       chan struct{}
}

var (
	defaultServerWriteTimeout = 30 * time.Second
	defaultServerReadTimeout  = 30 * time.Second
)

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

func newServer(srv Server, options ...ServerOption) server {
	s := server{
		info:            srv.Info(),
		sessions:        new(sync.Map),
		progresses:      new(sync.Map),
		sessionStopChan: make(chan string),
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
	go s.listenStopSession()
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
}

func (s server) listenStopSession() {
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

func (s server) listenPromptsList() {
	lists := s.promptListUpdater.PromptListUpdates()

	for {
		select {
		case <-s.closeChan:
			return
		case <-lists:
		}

		s.sessions.Range(func(_, value any) bool {
			sess, _ := value.(*serverSession)
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
			sess, _ := value.(*serverSession)
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
			sess, _ := value.(*serverSession)
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
			sess, _ := value.(*serverSession)
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
			sess, _ := value.(*serverSession)
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
		sess, _ := ss.(*serverSession)
		sess.progressChan <- params
	}
}

func (s server) startSession(
	ctx context.Context,
	w io.Writer,
	formatMsgFunc func([]byte) []byte,
	msgSentHook func(string),
) string {
	sCtx, sCancel := context.WithCancel(ctx)

	sessID := uuid.New().String()
	sess := &serverSession{
		id:                     sessID,
		ctx:                    sCtx,
		cancel:                 sCancel,
		writer:                 w,
		writeTimeout:           s.writeTimeout,
		readTimeout:            s.readTimeout,
		pingInterval:           s.pingInterval,
		formatMsgFunc:          formatMsgFunc,
		msgSentHook:            msgSentHook,
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
	if s.pingInterval > 0 {
		go sess.pings()
	}

	return sessID
}

func (s server) handleMsg(r io.Reader, sessionID string) error {
	msg, err := readMessage(r)
	if err != nil {
		return errInvalidJSON
	}

	ss, ok := s.sessions.Load(sessionID)
	if !ok {
		return errSessionNotFound
	}
	sess, _ := ss.(*serverSession)

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

func (s server) handleBasicMessages(sess *serverSession, msg JSONRPCMessage) error {
	switch msg.Method {
	case methodPing:
		go func() {
			if err := sess.handlePing(msg.ID); err != nil {
				log.Printf("failed to handle ping: %v", err)
			}
		}()
		return nil
	case methodInitialize:
		var params initializeParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		go func() {
			if err := sess.handleInitialize(msg.ID, params, s.capabilities,
				s.requiredClientCapabilities, s.info); err != nil {
				log.Printf("failed to handle initialize: %v", err)
				return
			}
		}()
		return nil
	}
	return nil
}

func (s server) handlePromptMessages(sess *serverSession, msg JSONRPCMessage) error {
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
		go func() {
			if err := sess.handlePromptsList(msg.ID, params, s.promptServer); err != nil {
				log.Printf("failed to handle prompts list: %v", err)
			}
		}()
		return nil
	case MethodPromptsGet:
		var params PromptsGetParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		if params.Meta.ProgressToken != "" {
			s.progresses.Store(params.Meta.ProgressToken, sess.id)
		}
		go func() {
			if err := sess.handlePromptsGet(msg.ID, params, s.promptServer); err != nil {
				log.Printf("failed to handle prompts get: %v", err)
			}
		}()
		return nil
	}
	return nil
}

func (s server) handleResourceMessages(sess *serverSession, msg JSONRPCMessage) error {
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
		go func() {
			if err := sess.handleResourcesList(msg.ID, params, s.resourceServer); err != nil {
				log.Printf("failed to handle resources list: %v", err)
			}
		}()
		return nil
	case MethodResourcesRead:
		var params ResourcesReadParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		if params.Meta.ProgressToken != "" {
			s.progresses.Store(params.Meta.ProgressToken, sess.id)
		}
		go func() {
			if err := sess.handleResourcesRead(msg.ID, params, s.resourceServer); err != nil {
				log.Printf("failed to handle resources read: %v", err)
			}
		}()
		return nil
	case MethodResourcesTemplatesList:
		var params ResourcesTemplatesListParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		if params.Meta.ProgressToken != "" {
			s.progresses.Store(params.Meta.ProgressToken, sess.id)
		}
		go func() {
			if err := sess.handleResourcesListTemplates(msg.ID, params, s.resourceServer); err != nil {
				log.Printf("failed to handle resources list templates: %v", err)
			}
		}()
		return nil
	case MethodResourcesSubscribe:
		var params ResourcesSubscribeParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		go func() {
			if err := sess.handleResourcesSubscribe(msg.ID, params, s.resourceServer); err != nil {
				log.Printf("failed to handle resources subscribe: %v", err)
			}
		}()
		return nil
	case MethodResourcesUnsubscribe:
		var params ResourcesSubscribeParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		go func() {
			if err := sess.handleResourcesUnsubscribe(msg.ID, params, s.resourceServer); err != nil {
				log.Printf("failed to handle resources unsubscribe: %v", err)
			}
		}()
		return nil
	}
	return nil
}

func (s server) handleToolMessages(sess *serverSession, msg JSONRPCMessage) error {
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
		go func() {
			if err := sess.handleToolsList(msg.ID, params, s.toolServer); err != nil {
				log.Printf("failed to handle tools list: %v", err)
			}
		}()
		return nil
	case MethodToolsCall:
		var params ToolsCallParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		if params.Meta.ProgressToken != "" {
			s.progresses.Store(params.Meta.ProgressToken, sess.id)
		}
		go func() {
			if err := sess.handleToolsCall(msg.ID, params, s.toolServer); err != nil {
				log.Printf("failed to handle tools call: %v", err)
			}
		}()
		return nil
	}
	return nil
}

func (s server) handleCompletionMessages(sess *serverSession, msg JSONRPCMessage) error {
	if msg.Method != MethodCompletionComplete {
		return nil
	}

	var params CompletionCompleteParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}

	switch params.Ref.Type {
	case CompletionRefPrompt:
		go func() {
			if err := sess.handleCompletePrompt(msg.ID, params, s.promptServer); err != nil {
				log.Printf("failed to handle completion complete prompt: %v", err)
			}
		}()
		return nil
	case CompletionRefResource:
		go func() {
			if err := sess.handleCompleteResource(msg.ID, params, s.resourceServer); err != nil {
				log.Printf("failed to handle completion complete resource: %v", err)
			}
		}()
		return nil
	}
	return nil
}

func (s server) handleNotificationMessages(sess *serverSession, msg JSONRPCMessage) error {
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

func (s server) handleResultMessages(sess *serverSession, msg JSONRPCMessage) {
	if msg.Method != "" {
		return
	}

	go sess.handleResult(msg)
}

func (s server) handleLoggingMessages(sess *serverSession, msg JSONRPCMessage) error {
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
	go sess.handleLoggingSetLevel(params, s.logHandler)

	return nil
}

func (s server) listRoots(ctx context.Context) (RootList, error) {
	ss, ok := s.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return RootList{}, errSessionNotFound
	}
	sess, _ := ss.(*serverSession)
	return sess.listRoots()
}

func (s server) requestSampling(ctx context.Context, params SamplingParams) (SamplingResult, error) {
	ss, ok := s.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return SamplingResult{}, errSessionNotFound
	}
	sess, _ := ss.(*serverSession)
	return sess.createSampleMessage(params)
}

func (s server) stop() {
	s.sessions.Range(func(_, value any) bool {
		sess, _ := value.(*serverSession)
		sess.cancel()
		return true
	})
	close(s.closeChan)
}
