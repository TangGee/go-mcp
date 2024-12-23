package mcp

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"
)

// Client represents the main MCP client interface that users will implement.
type Client interface {
	Info() Info
	RequirePromptServer() bool
	RequireResourceServer() bool
	RequireToolServer() bool
}

// ClientOption is a function that configures a client.
type ClientOption func(*client)

type client struct {
	capabilities               ClientCapabilities
	info                       Info
	requiredServerCapabilities ServerCapabilities

	sessions *sync.Map // map[sessionID]*clientSession

	rootsListHandler RootsListHandler
	rootsListUpdater RootsListUpdater

	samplingHandler SamplingHandler

	promptListWatcher PromptListWatcher

	resourceListWatcher       ResourceListWatcher
	resourceSubscribedWatcher ResourceSubscribedWatcher

	toolListWatcher ToolListWatcher

	progressListener ProgressListener
	logReceiver      LogReceiver

	writeTimeout time.Duration
	readTimeout  time.Duration
	pingInterval time.Duration

	sessionStopChan chan string
	closeChan       chan struct{}
}

var (
	defaultClientWriteTimeout = 30 * time.Second
	defaultClientReadTimeout  = 30 * time.Second
)

// WithRootsListHandler sets the roots list handler for the client.
func WithRootsListHandler(handler RootsListHandler) ClientOption {
	return func(c *client) {
		c.rootsListHandler = handler
	}
}

// WithRootsListUpdater sets the roots list updater for the client.
func WithRootsListUpdater(updater RootsListUpdater) ClientOption {
	return func(c *client) {
		c.rootsListUpdater = updater
	}
}

// WithSamplingHandler sets the sampling handler for the client.
func WithSamplingHandler(handler SamplingHandler) ClientOption {
	return func(c *client) {
		c.samplingHandler = handler
	}
}

// WithPromptListWatcher sets the prompt list watcher for the client.
func WithPromptListWatcher(watcher PromptListWatcher) ClientOption {
	return func(c *client) {
		c.promptListWatcher = watcher
	}
}

// WithResourceListWatcher sets the resource list watcher for the client.
func WithResourceListWatcher(watcher ResourceListWatcher) ClientOption {
	return func(c *client) {
		c.resourceListWatcher = watcher
	}
}

// WithResourceSubscribedWatcher sets the resource subscribe watcher for the client.
func WithResourceSubscribedWatcher(watcher ResourceSubscribedWatcher) ClientOption {
	return func(c *client) {
		c.resourceSubscribedWatcher = watcher
	}
}

// WithToolListWatcher sets the tool list watcher for the client.
func WithToolListWatcher(watcher ToolListWatcher) ClientOption {
	return func(c *client) {
		c.toolListWatcher = watcher
	}
}

// WithProgressListener sets the progress listener for the client.
func WithProgressListener(listener ProgressListener) ClientOption {
	return func(c *client) {
		c.progressListener = listener
	}
}

// WithLogReceiver sets the log receiver for the client.
func WithLogReceiver(receiver LogReceiver) ClientOption {
	return func(c *client) {
		c.logReceiver = receiver
	}
}

// WithClientWriteTimeout sets the write timeout for the client.
func WithClientWriteTimeout(timeout time.Duration) ClientOption {
	return func(c *client) {
		c.writeTimeout = timeout
	}
}

// WithClientReadTimeout sets the read timeout for the client.
func WithClientReadTimeout(timeout time.Duration) ClientOption {
	return func(c *client) {
		c.readTimeout = timeout
	}
}

// WithClientPingInterval sets the ping interval for the client.
func WithClientPingInterval(interval time.Duration) ClientOption {
	return func(c *client) {
		c.pingInterval = interval
	}
}

func newClient(cli Client, options ...ClientOption) client {
	c := client{
		info:            cli.Info(),
		sessions:        new(sync.Map),
		sessionStopChan: make(chan string),
		closeChan:       make(chan struct{}),
	}
	for _, opt := range options {
		opt(&c)
	}

	if c.writeTimeout == 0 {
		c.writeTimeout = defaultClientWriteTimeout
	}
	if c.readTimeout == 0 {
		c.readTimeout = defaultClientReadTimeout
	}

	c.capabilities = ClientCapabilities{}

	if c.rootsListHandler != nil {
		c.capabilities.Roots = &RootsCapability{}
		if c.rootsListUpdater != nil {
			c.capabilities.Roots.ListChanged = true
		}
	}
	if c.samplingHandler != nil {
		c.capabilities.Sampling = &SamplingCapability{}
	}

	c.requiredServerCapabilities = ServerCapabilities{}

	if cli.RequirePromptServer() {
		c.requiredServerCapabilities.Prompts = &PromptsCapability{}
		if c.promptListWatcher != nil {
			c.requiredServerCapabilities.Prompts = &PromptsCapability{
				ListChanged: true,
			}
		}
	}

	if cli.RequireResourceServer() {
		rlc := false
		rsc := false
		if c.resourceListWatcher != nil {
			rlc = true
		}
		if c.resourceSubscribedWatcher != nil {
			rsc = true
		}
		c.requiredServerCapabilities.Resources = &ResourcesCapability{
			ListChanged: rlc,
			Subscribe:   rsc,
		}
	}

	if cli.RequireToolServer() {
		c.requiredServerCapabilities.Tools = &ToolsCapability{}
		if c.toolListWatcher != nil {
			c.requiredServerCapabilities.Tools = &ToolsCapability{
				ListChanged: true,
			}
		}
	}

	if c.logReceiver != nil {
		c.requiredServerCapabilities.Logging = &LoggingCapability{}
	}

	return c
}

func (c client) start() {
	go c.listenStopSession()
	if c.rootsListUpdater != nil {
		go c.listenRootsList()
	}
}

func (c client) listenStopSession() {
	for {
		var id string
		select {
		case <-c.closeChan:
			return
		case id = <-c.sessionStopChan:
		}
		c.sessions.Delete(id)
	}
}

func (c client) listenRootsList() {
	lists := c.rootsListUpdater.RootsListUpdates()

	for {
		select {
		case <-c.closeChan:
			return
		case <-lists:
		}

		c.sessions.Range(func(_, value any) bool {
			sess, _ := value.(*clientSession)
			sess.rootsListChan <- struct{}{}
			return true
		})
	}
}

func (c client) startSession(ctx context.Context, w io.Writer, id string) {
	cCtx, cCancel := context.WithCancel(ctx)

	sess := &clientSession{
		id:            id,
		ctx:           cCtx,
		cancel:        cCancel,
		writter:       w,
		writeTimeout:  c.writeTimeout,
		readTimeout:   c.readTimeout,
		pingInterval:  c.pingInterval,
		stopChan:      c.sessionStopChan,
		rootsListChan: make(chan struct{}),
	}

	c.sessions.Store(id, sess)
	go sess.listen()
	if c.pingInterval > 0 {
		go sess.pings()
	}
}

func (c client) handleMsg(r io.Reader, sessionID string) error {
	msg, err := readMessage(r)
	if err != nil {
		return errInvalidJSON
	}

	ss, ok := c.sessions.Load(sessionID)
	if !ok {
		return errSessionNotFound
	}
	sess, _ := ss.(*clientSession)

	// Handle basic protocol messages
	if err := c.handleBasicMessages(sess, msg); err != nil {
		return err
	}

	// Handle root-related messages
	if err := c.handleRootMessages(sess, msg); err != nil {
		return err
	}

	// Handle sampling-related messages
	if err := c.handleSamplingMessages(sess, msg); err != nil {
		return err
	}

	// Handle notification messages
	if err := c.handleNotificationMessages(sess, msg); err != nil {
		return err
	}

	// Handle result messages
	if err := c.handleResultMessages(sess, msg); err != nil {
		return err
	}

	return nil
}

func (c client) handleBasicMessages(sess *clientSession, msg JSONRPCMessage) error {
	if msg.Method != methodPing {
		return nil
	}
	return sess.handlePing(msg.ID)
}

func (c client) handleRootMessages(sess *clientSession, msg JSONRPCMessage) error {
	if c.rootsListHandler == nil {
		return nil
	}

	if msg.Method != MethodRootsList {
		return nil
	}

	return sess.handleRootsList(msg.ID, c.rootsListHandler)
}

func (c client) handleSamplingMessages(sess *clientSession, msg JSONRPCMessage) error {
	if c.samplingHandler == nil {
		return nil
	}

	if msg.Method != MethodSamplingCreateMessage {
		return nil
	}
	var params SamplingParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errInvalidJSON
	}
	return sess.handleSamplingCreateMessage(msg.ID, params, c.samplingHandler)
}

func (c client) handleNotificationMessages(sess *clientSession, msg JSONRPCMessage) error {
	switch msg.Method {
	case methodNotificationsCancelled:
		var params notificationsCancelledParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		sess.handleNotificationsCancelled(params)
	case methodNotificationsPromptsListChanged:
		if c.promptListWatcher != nil {
			c.promptListWatcher.OnPromptListChanged()
		}
	case methodNotificationsResourcesListChanged:
		if c.resourceListWatcher != nil {
			c.resourceListWatcher.OnResourceListChanged()
		}
	case methodNotificationsResourcesUpdated:
		if c.resourceSubscribedWatcher != nil {
			var params ResourcesSubscribeParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return errInvalidJSON
			}
			c.resourceSubscribedWatcher.OnResourceSubscribedChanged(params.URI)
		}
	case methodNotificationsToolsListChanged:
		if c.toolListWatcher != nil {
			c.toolListWatcher.OnToolListChanged()
		}
	case methodNotificationsProgress:
		var params ProgressParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		c.progressListener.OnProgress(params)
	case methodNotificationsMessage:
		var params LogParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errInvalidJSON
		}
		c.logReceiver.OnLog(params)
	}

	return nil
}

func (c client) handleResultMessages(sess *clientSession, msg JSONRPCMessage) error {
	if msg.Method != "" {
		return nil
	}

	return sess.handleResult(msg)
}

func (c client) initialize(ctx context.Context) error {
	ss, ok := c.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.initialize(c.capabilities, c.info, c.requiredServerCapabilities)
}

func (c client) listPrompts(ctx context.Context, cursor string, progressToken MustString) (PromptList, error) {
	ss, ok := c.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return PromptList{}, errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.listPrompts(cursor, progressToken)
}

func (c client) getPrompt(
	ctx context.Context,
	name string,
	arguments map[string]string,
	progressToken MustString,
) (PromptResult, error) {
	ss, ok := c.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return PromptResult{}, errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.getPrompt(name, arguments, progressToken)
}

func (c client) completesPrompt(ctx context.Context, name string, arg CompletionArgument) (CompletionResult, error) {
	ss, ok := c.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return CompletionResult{}, errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.completesPrompt(name, arg)
}

func (c client) listResources(ctx context.Context, cursor string, progressToken MustString) (ResourceList, error) {
	ss, ok := c.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return ResourceList{}, errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.listResources(cursor, progressToken)
}

func (c client) readResource(ctx context.Context, uri string, progressToken MustString) (Resource, error) {
	ss, ok := c.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return Resource{}, errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.readResource(uri, progressToken)
}

func (c client) listResourceTemplates(ctx context.Context, progressToken MustString) ([]ResourceTemplate, error) {
	ss, ok := c.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return nil, errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.listResourceTemplates(progressToken)
}

func (c client) completesResourceTemplate(
	ctx context.Context,
	name string,
	arg CompletionArgument,
) (CompletionResult, error) {
	ss, ok := c.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return CompletionResult{}, errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.completesResourceTemplate(name, arg)
}

func (c client) subscribeResource(ctx context.Context, uri string) error {
	ss, ok := c.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.subscribeResource(uri)
}

func (c client) unsubscribeResource(ctx context.Context, uri string) error {
	ss, ok := c.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.unsubscribeResource(uri)
}

func (c client) listTools(ctx context.Context, cursor string, progressToken MustString) (ToolList, error) {
	ss, ok := c.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return ToolList{}, errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.listTools(cursor, progressToken)
}

func (c client) callTool(
	ctx context.Context,
	name string,
	arguments map[string]any,
	progressToken MustString,
) (ToolResult, error) {
	ss, ok := c.sessions.Load(sessionIDFromContext(ctx))
	if !ok {
		return ToolResult{}, errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.callTool(name, arguments, progressToken)
}

func (c client) setLogLevel(level LogLevel) error {
	ss, ok := c.sessions.Load(sessionIDFromContext(context.Background()))
	if !ok {
		return errSessionNotFound
	}
	sess, _ := ss.(*clientSession)
	return sess.setLogLevel(level)
}

func (c client) stop() {
	c.sessions.Range(func(_, value any) bool {
		sess, _ := value.(*clientSession)
		sess.cancel()
		return true
	})
	close(c.closeChan)
}
