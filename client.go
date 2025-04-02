package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ClientOption is a function that configures a client.
type ClientOption func(*Client)

// Client implements a Model Context Protocol (MCP) client that enables communication
// between LLM applications and external data sources and tools. It manages the
// connection lifecycle, handles protocol messages, and provides access to MCP
// server capabilities.
//
// The client supports various server interactions including prompt management,
// resource handling, tool execution, and logging. It maintains session state and
// provides automatic connection health monitoring through periodic pings.
//
// A Client must be created using NewClient() and requires Connect() to be called
// before any operations can be performed. The client should be properly closed
// using Disconnect() when it's no longer needed.
type Client struct {
	capabilities ClientCapabilities
	info         Info
	transport    ClientTransport
	session      Session
	serverState  *serverState

	rootsListHandler RootsListHandler
	rootsListUpdater RootsListUpdater

	samplingHandler SamplingHandler

	promptListWatcher PromptListWatcher

	resourceListWatcher       ResourceListWatcher
	resourceSubscribedWatcher ResourceSubscribedWatcher

	toolListWatcher ToolListWatcher

	progressListener ProgressListener
	logReceiver      LogReceiver

	pingInterval time.Duration
	pingTimeout  time.Duration
	onPingFailed func(error)

	logger *slog.Logger

	resultManager *clientResultManager

	closed          chan struct{}
	pingClosed      chan struct{}
	rootsListClosed chan struct{}
}

type clientResultManager struct {
	lock     sync.Mutex
	channels map[string]chan JSONRPCMessage
	closed   bool
}

type serverState struct {
	lock         sync.Mutex
	initialized  bool
	info         Info
	capabilities ServerCapabilities
	stopped      bool
}

var (
	defaultClientPingInterval = 30 * time.Second
	defaultClientPingTimeout  = 30 * time.Second
)

// WithRootsListHandler sets the roots list handler for the client.
func WithRootsListHandler(handler RootsListHandler) ClientOption {
	return func(c *Client) {
		c.rootsListHandler = handler
	}
}

// WithRootsListUpdater sets the roots list updater for the client.
func WithRootsListUpdater(updater RootsListUpdater) ClientOption {
	return func(c *Client) {
		c.rootsListUpdater = updater
	}
}

// WithSamplingHandler sets the sampling handler for the client.
func WithSamplingHandler(handler SamplingHandler) ClientOption {
	return func(c *Client) {
		c.samplingHandler = handler
	}
}

// WithPromptListWatcher sets the prompt list watcher for the client.
func WithPromptListWatcher(watcher PromptListWatcher) ClientOption {
	return func(c *Client) {
		c.promptListWatcher = watcher
	}
}

// WithResourceListWatcher sets the resource list watcher for the client.
func WithResourceListWatcher(watcher ResourceListWatcher) ClientOption {
	return func(c *Client) {
		c.resourceListWatcher = watcher
	}
}

// WithResourceSubscribedWatcher sets the resource subscribe watcher for the client.
func WithResourceSubscribedWatcher(watcher ResourceSubscribedWatcher) ClientOption {
	return func(c *Client) {
		c.resourceSubscribedWatcher = watcher
	}
}

// WithToolListWatcher sets the tool list watcher for the client.
func WithToolListWatcher(watcher ToolListWatcher) ClientOption {
	return func(c *Client) {
		c.toolListWatcher = watcher
	}
}

// WithProgressListener sets the progress listener for the client.
func WithProgressListener(listener ProgressListener) ClientOption {
	return func(c *Client) {
		c.progressListener = listener
	}
}

// WithLogReceiver sets the log receiver for the client.
func WithLogReceiver(receiver LogReceiver) ClientOption {
	return func(c *Client) {
		c.logReceiver = receiver
	}
}

// WithClientPingInterval sets the ping interval for the client.
func WithClientPingInterval(interval time.Duration) ClientOption {
	return func(c *Client) {
		c.pingInterval = interval
	}
}

// WithClientPingTimeout sets the ping timeout for the client.
func WithClientPingTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.pingTimeout = timeout
	}
}

// WithClientOnPingFailed sets the callback for when the client ping fails.
func WithClientOnPingFailed(onPingFailed func(error)) ClientOption {
	return func(c *Client) {
		c.onPingFailed = onPingFailed
	}
}

// WithClientLogger sets the logger for the client.
func WithClientLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

// NewClient creates a new Model Context Protocol (MCP) client with the specified configuration.
// It establishes a client that can communicate with MCP servers according to the protocol
// specification at https://spec.modelcontextprotocol.io/specification/.
//
// The info parameter provides client identification and version information. The transport
// parameter defines how the client communicates with the server.
//
// Optional client behaviors can be configured through ClientOption functions. These include
// handlers for roots management, sampling, resource management, tool operations, progress
// tracking, and logging. Timeouts and intervals can also be configured through options.
//
// The client will not be connected until Connect() is called. After creation, use
// Connect() to establish the session with the server and initialize the protocol.
func NewClient(
	info Info,
	transport ClientTransport,
	options ...ClientOption,
) *Client {
	c := &Client{
		info:      info,
		transport: transport,
		logger:    slog.Default(),
		serverState: &serverState{
			stopped: true,
		},
		resultManager: &clientResultManager{
			channels: make(map[string]chan JSONRPCMessage),
		},
		closed:          make(chan struct{}),
		pingClosed:      make(chan struct{}),
		rootsListClosed: make(chan struct{}),
	}
	for _, opt := range options {
		opt(c)
	}

	if c.pingInterval == 0 {
		c.pingInterval = defaultClientPingInterval
	}
	if c.pingTimeout == 0 {
		c.pingTimeout = defaultClientPingTimeout
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

	return c
}

// Connect establishes a session with the MCP server and initializes the protocol handshake.
// It starts background routines for message handling and server health checks through periodic pings.
//
// The initialization process verifies protocol version compatibility and required server capabilities.
// If the server's capabilities don't match the client's requirements, Connect returns an error.
//
// Connect must be called after creating a new client and before making any other client method calls.
// It returns an error if the session cannot be established or if the initialization fails.
func (c *Client) Connect(ctx context.Context) error {
	// Start session using the transport.
	sess, err := c.transport.StartSession(ctx)
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	c.session = sess

	// Initialize the result manager.
	c.resultManager.init()

	// Spawn the main loop for handling messages. And because we already started this,
	// if then the initialization fails, we should call Disconnect to clean up resources.
	go c.start()

	// Register the initialization result channel and initiate the initialization request.
	initMsgID := uuid.New().String()
	results := c.resultManager.register(initMsgID)
	if err := c.sendInitialize(ctx, MustString(initMsgID)); err != nil {
		c.logger.Error("failed to send initialize request", slog.String("err", err.Error()))
		if disconnectErr := c.Disconnect(ctx); disconnectErr != nil {
			c.logger.Error("failed to disconnect", slog.String("err", disconnectErr.Error()))
		}
		return err
	}

	// Wait for the initialization result.
	var initResMsg JSONRPCMessage
	select {
	case <-ctx.Done():
		c.logger.Error("failed to initialize", slog.String("err", ctx.Err().Error()))
		if disconnectErr := c.Disconnect(ctx); disconnectErr != nil {
			c.logger.Error("failed to disconnect", slog.String("err", disconnectErr.Error()))
		}
		return fmt.Errorf("failed to initialize: %w", ctx.Err())
	case initResMsg = <-results:
	}

	if initResMsg.Error != nil {
		// Server told our initialization request failed.
		c.logger.Error("failed to initialize", slog.String("err", initResMsg.Error.Error()))
		if disconnectErr := c.Disconnect(ctx); disconnectErr != nil {
			c.logger.Error("failed to disconnect", slog.String("err", disconnectErr.Error()))
		}
		return fmt.Errorf("failed to initialize: %w", initResMsg.Error)
	}

	// Verify the server's initialization result.
	initRes, err := c.verifyInitialize(initResMsg)
	if err != nil {
		nErr := fmt.Errorf("failed to verify initialize result: %w", err)
		c.logger.Error("failed to verify initialize result", slog.String("err", err.Error()))
		// Server initialization result is invalid, we should send the error back to them and disconnect the client.
		if err := c.session.Send(ctx, JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			ID:      initResMsg.ID,
			Error: &JSONRPCError{
				Code:    jsonRPCInvalidParamsCode,
				Message: err.Error(),
			},
		}); err != nil {
			c.logger.Error("failed to send initialization error", slog.String("err", err.Error()))
			nErr = fmt.Errorf("failed to send initialization error: %w", err)
		}
		if disconnectErr := c.Disconnect(ctx); disconnectErr != nil {
			c.logger.Error("failed to disconnect", slog.String("err", disconnectErr.Error()))
		}
		return nErr
	}

	// Send the initialization notification to server.
	if err := c.session.Send(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  methodNotificationsInitialized,
	}); err != nil {
		c.logger.Error("failed to send initialization notification", slog.String("err", err.Error()))
		if disconnectErr := c.Disconnect(ctx); disconnectErr != nil {
			c.logger.Error("failed to disconnect", slog.String("err", disconnectErr.Error()))
		}
		return fmt.Errorf("failed to send initialization notification: %w", err)
	}

	// Initialize the server state.
	c.serverState.init(initRes.ServerInfo, initRes.Capabilities)

	return nil
}

// Disconnect closes the client session and resets the server state.
// It ensures proper cleanup of resources, including all pending requests and background routines.
//
// If the client implements a RootsListUpdater, this method will wait for it to finish
// before closing the session. The method is idempotent and can be safely called multiple times.
//
// It returns an error if the disconnection process fails or times out.
func (c *Client) Disconnect(ctx context.Context) error {
	// Close the result manager to close all the result channels.
	c.resultManager.close()

	// Wait for the roots list updater to finish, if the client implements it.
	if c.rootsListUpdater != nil {
		select {
		case <-ctx.Done():
			return fmt.Errorf("failed to close RootsListUpdater: %w", ctx.Err())
		case <-c.rootsListClosed:
		}
	}

	// If Client failed when calling transport.StartSession, then session will be nil,
	// which mean we can return at this point.
	if c.session == nil {
		return nil
	}

	// If user called Disconnect multiple times, or called it after failed at
	// Connect (that function automaticly call Disconnect), then we should return at this point,
	// because we guarantee to call Session.Stop only once.
	if c.serverState.isStopped() {
		return nil
	}

	// Stop the session to signal the main loop to exit.
	c.session.Stop()

	// Wait for the main loop to finish.
	select {
	case <-ctx.Done():
		return fmt.Errorf("failed to close Client: %w", ctx.Err())
	case <-c.closed:
	}

	// Reset the server state.
	c.serverState.reset()

	return nil
}

// ListPrompts retrieves a paginated list of available prompts from the server.
// It returns a ListPromptsResult containing prompt metadata and pagination information.
//
// The request can be cancelled via the context. When cancelled, a cancellation
// request will be sent to the server to stop processing.
//
// See ListPromptsParams for details on available parameters including cursor for pagination
// and optional progress tracking.
func (c *Client) ListPrompts(ctx context.Context, params ListPromptsParams) (ListPromptResult, error) {
	if !c.serverState.isInitialized() {
		return ListPromptResult{}, errors.New("client not initialized")
	}
	if !c.serverState.promptServerAvailable() {
		return ListPromptResult{}, errors.New("prompt server not supported by server")
	}

	res, err := c.sendRequest(ctx, MethodPromptsList, params)
	if err != nil {
		return ListPromptResult{}, err
	}

	var result ListPromptResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return ListPromptResult{}, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return result, nil
}

// GetPrompt retrieves a specific prompt by name with the given arguments.
// It returns a GetPromptResult containing the prompt's content and metadata.
//
// The request can be cancelled via the context. When cancelled, a cancellation
// request will be sent to the server to stop processing.
//
// See GetPromptParams for details on available parameters including prompt name,
// arguments, and optional progress tracking.
func (c *Client) GetPrompt(ctx context.Context, params GetPromptParams) (GetPromptResult, error) {
	if !c.serverState.isInitialized() {
		return GetPromptResult{}, errors.New("client not initialized")
	}
	if !c.serverState.promptServerAvailable() {
		return GetPromptResult{}, errors.New("prompt server not supported by server")
	}

	res, err := c.sendRequest(ctx, MethodPromptsGet, params)
	if err != nil {
		return GetPromptResult{}, err
	}

	var result GetPromptResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return GetPromptResult{}, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return result, nil
}

// CompletesPrompt requests completion suggestions for a prompt-based completion.
// It returns a CompletionResult containing the completion suggestions.
//
// The request can be cancelled via the context. When cancelled, a cancellation
// request will be sent to the server to stop processing.
//
// See CompletesCompletionParams for details on available parameters including
// completion reference and argument information.
func (c *Client) CompletesPrompt(ctx context.Context, params CompletesCompletionParams) (CompletionResult, error) {
	if !c.serverState.isInitialized() {
		return CompletionResult{}, errors.New("client not initialized")
	}
	if !c.serverState.promptServerAvailable() {
		return CompletionResult{}, errors.New("prompt server not supported by server")
	}

	res, err := c.sendRequest(ctx, MethodCompletionComplete, params)
	if err != nil {
		return CompletionResult{}, err
	}

	var result CompletionResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return CompletionResult{}, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return result, nil
}

// ListResources retrieves a paginated list of available resources from the server.
// It returns a ListResourcesResult containing resource metadata and pagination information.
//
// The request can be cancelled via the context. When cancelled, a cancellation
// request will be sent to the server to stop processing.
//
// See ListResourcesParams for details on available parameters including cursor for
// pagination and optional progress tracking.
func (c *Client) ListResources(ctx context.Context, params ListResourcesParams) (ListResourcesResult, error) {
	if !c.serverState.isInitialized() {
		return ListResourcesResult{}, errors.New("client not initialized")
	}
	if !c.serverState.resourceServerAvailable() {
		return ListResourcesResult{}, errors.New("resource server not supported by server")
	}

	res, err := c.sendRequest(ctx, MethodResourcesList, params)
	if err != nil {
		return ListResourcesResult{}, err
	}

	var result ListResourcesResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return ListResourcesResult{}, err
	}

	return result, nil
}

// ReadResource retrieves the content and metadata of a specific resource.
// It returns a Resource containing the resource's content, type, and associated metadata.
//
// The request can be cancelled via the context. When cancelled, a cancellation
// request will be sent to the server to stop processing.
//
// See ReadResourceParams for details on available parameters including resource URI
// and optional progress tracking.
func (c *Client) ReadResource(ctx context.Context, params ReadResourceParams) (ReadResourceResult, error) {
	if !c.serverState.isInitialized() {
		return ReadResourceResult{}, errors.New("client not initialized")
	}
	if !c.serverState.resourceServerAvailable() {
		return ReadResourceResult{}, errors.New("resource server not supported by server")
	}

	res, err := c.sendRequest(ctx, MethodResourcesRead, params)
	if err != nil {
		return ReadResourceResult{}, err
	}

	var result ReadResourceResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return ReadResourceResult{}, err
	}

	return result, nil
}

// ListResourceTemplates retrieves a list of available resource templates from the server.
// Resource templates allow servers to expose parameterized resources using URI templates.
//
// The request can be cancelled via the context. When cancelled, a cancellation
// request will be sent to the server to stop processing.
//
// See ListResourceTemplatesParams for details on available parameters including
// optional progress tracking.
func (c *Client) ListResourceTemplates(
	ctx context.Context,
	params ListResourceTemplatesParams,
) (ListResourceTemplatesResult, error) {
	if !c.serverState.isInitialized() {
		return ListResourceTemplatesResult{}, errors.New("client not initialized")
	}
	if !c.serverState.resourceServerAvailable() {
		return ListResourceTemplatesResult{}, errors.New("resource server not supported by server")
	}

	res, err := c.sendRequest(ctx, MethodResourcesTemplatesList, params)
	if err != nil {
		return ListResourceTemplatesResult{}, err
	}

	var result ListResourceTemplatesResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return ListResourceTemplatesResult{}, err
	}

	return result, nil
}

// CompletesResourceTemplate requests completion suggestions for a resource template.
// It returns a CompletionResult containing the completion suggestions.
//
// The request can be cancelled via the context. When cancelled, a cancellation
// request will be sent to the server to stop processing.
//
// See CompletesCompletionParams for details on available parameters including
// completion reference and argument information.
func (c *Client) CompletesResourceTemplate(
	ctx context.Context,
	params CompletesCompletionParams,
) (CompletionResult, error) {
	if !c.serverState.isInitialized() {
		return CompletionResult{}, errors.New("client not initialized")
	}
	if !c.serverState.resourceServerAvailable() {
		return CompletionResult{}, errors.New("resource server not supported by server")
	}

	res, err := c.sendRequest(ctx, MethodCompletionComplete, params)
	if err != nil {
		return CompletionResult{}, err
	}

	var result CompletionResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return CompletionResult{}, err
	}

	return result, nil
}

// SubscribeResource registers the client for notifications about changes to a specific resource.
// When the resource is modified, the client will receive notifications through the
// ResourceSubscribedWatcher interface if one was set using WithResourceSubscribedWatcher.
//
// See SubscribeResourceParams for details on available parameters including resource URI.
func (c *Client) SubscribeResource(ctx context.Context, params SubscribeResourceParams) error {
	if !c.serverState.isInitialized() {
		return errors.New("client not initialized")
	}
	if !c.serverState.resourceServerAvailable() {
		return errors.New("resource server not supported by server")
	}

	_, err := c.sendRequest(ctx, MethodResourcesSubscribe, params)
	if err != nil {
		return err
	}

	return nil
}

// UnsubscribeResource unregisters the client for notifications about changes to a specific resource.
func (c *Client) UnsubscribeResource(ctx context.Context, params UnsubscribeResourceParams) error {
	if !c.serverState.isInitialized() {
		return errors.New("client not initialized")
	}
	if !c.serverState.resourceServerAvailable() {
		return errors.New("resource server not supported by server")
	}

	_, err := c.sendRequest(ctx, MethodResourcesUnsubscribe, params)
	if err != nil {
		return err
	}

	return nil
}

// ListTools retrieves a paginated list of available tools from the server.
// It returns a ListToolsResult containing tool metadata and pagination information.
//
// The request can be cancelled via the context. When cancelled, a cancellation
// request will be sent to the server to stop processing.
//
// See ListToolsParams for details on available parameters including cursor for
// pagination and optional progress tracking.
func (c *Client) ListTools(ctx context.Context, params ListToolsParams) (ListToolsResult, error) {
	if !c.serverState.isInitialized() {
		return ListToolsResult{}, errors.New("client not initialized")
	}
	if !c.serverState.toolServerAvailable() {
		return ListToolsResult{}, errors.New("tool server not supported by server")
	}

	res, err := c.sendRequest(ctx, MethodToolsList, params)
	if err != nil {
		return ListToolsResult{}, err
	}

	var result ListToolsResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return ListToolsResult{}, err
	}

	return result, nil
}

// CallTool executes a specific tool and returns its result.
// It provides a way to invoke server-side tools that can perform specialized operations.
//
// The request can be cancelled via the context. When cancelled, a cancellation
// request will be sent to the server to stop processing.
//
// See CallToolParams for details on available parameters including tool name,
// arguments, and optional progress tracking.
func (c *Client) CallTool(ctx context.Context, params CallToolParams) (CallToolResult, error) {
	if !c.serverState.isInitialized() {
		return CallToolResult{}, errors.New("client not initialized")
	}
	if !c.serverState.toolServerAvailable() {
		return CallToolResult{}, errors.New("tool server not supported by server")
	}

	res, err := c.sendRequest(ctx, MethodToolsCall, params)
	if err != nil {
		return CallToolResult{}, err
	}

	var result CallToolResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return CallToolResult{}, err
	}

	return result, nil
}

// SetLogLevel configures the logging level for the MCP server.
// It allows dynamic adjustment of the server's logging verbosity during runtime.
//
// The level parameter specifies the desired logging level. Valid levels are defined
// by the LogLevel type. The server will adjust its logging output to match the
// requested level.
func (c *Client) SetLogLevel(ctx context.Context, level LogLevel) error {
	if !c.serverState.isInitialized() {
		return errors.New("client not initialized")
	}
	if !c.serverState.loggingAvailable() {
		return errors.New("logging not supported by server")
	}

	params := LogParams{
		Level: level,
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}
	return c.session.Send(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      MustString(uuid.New().String()),
		Method:  MethodLoggingSetLevel,
		Params:  paramsBs,
	})
}

// ServerInfo returns the server's info.
func (c *Client) ServerInfo() Info {
	return c.serverState.serverInfo()
}

// PromptServerSupported returns true if the server supports prompt management.
func (c *Client) PromptServerSupported() bool {
	return c.serverState.promptServerAvailable()
}

// ResourceServerSupported returns true if the server supports resource management.
func (c *Client) ResourceServerSupported() bool {
	return c.serverState.resourceServerAvailable()
}

// ToolServerSupported returns true if the server supports tool management.
func (c *Client) ToolServerSupported() bool {
	return c.serverState.toolServerAvailable()
}

// LoggingServerSupported returns true if the server supports logging.
func (c *Client) LoggingServerSupported() bool {
	return c.serverState.loggingAvailable()
}

func (c *Client) start() {
	defer close(c.closed)

	// This channel would be used to notify ping goroutine to stop.
	pingDone := make(chan struct{})
	defer close(pingDone)

	// Spawn a goroutine to ping the server.
	go c.ping(pingDone)
	// Spawn a goroutine to handle the roots list updater, if it is implemented by user.
	if c.rootsListUpdater != nil {
		go c.listenListRootUpdates()
	}
	// This loops would break when the transport is shutdown.
	for msg := range c.session.Messages() {
		if msg.JSONRPC != JSONRPCVersion {
			c.logger.Error("invalid jsonrpc version", "version", msg.JSONRPC)
			continue
		}

		switch msg.Method {
		case methodPing:
			go func(msgID MustString) {
				// Send pong back to the server.
				pongCtx, pongCancel := context.WithTimeout(context.Background(), c.pingTimeout)
				if err := c.session.Send(pongCtx, JSONRPCMessage{
					JSONRPC: JSONRPCVersion,
					ID:      msgID,
				}); err != nil {
					c.logger.Error("failed to send pong", slog.String("err", err.Error()))
				}
				pongCancel()
			}(msg.ID)
		case MethodRootsList, MethodSamplingCreateMessage:
			go c.handleHandlerImplementationMessage(msg)
		case methodNotificationsPromptsListChanged:
			if c.serverState.isInitialized() && c.promptListWatcher != nil {
				c.promptListWatcher.OnPromptListChanged()
			}
		case methodNotificationsResourcesListChanged:
			if c.serverState.isInitialized() && c.resourceListWatcher != nil {
				c.resourceListWatcher.OnResourceListChanged()
			}
		case methodNotificationsResourcesUpdated:
			if c.serverState.isInitialized() && c.resourceSubscribedWatcher != nil {
				var params SubscribeResourceParams
				if err := json.Unmarshal(msg.Params, &params); err != nil {
					c.logger.Error("failed to unmarshal resources subscribe params", slog.String("err", err.Error()))
				}
				c.resourceSubscribedWatcher.OnResourceSubscribedChanged(params.URI)
			}
		case methodNotificationsToolsListChanged:
			if c.serverState.isInitialized() && c.toolListWatcher != nil {
				c.toolListWatcher.OnToolListChanged()
			}
		case methodNotificationsProgress:
			if c.serverState.isInitialized() && c.progressListener == nil {
				continue
			}

			var params ProgressParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				c.logger.Error("failed to unmarshal progress params", slog.String("err", err.Error()))
				continue
			}
			c.progressListener.OnProgress(params)
		case methodNotificationsMessage:
			if c.serverState.isInitialized() && c.logReceiver == nil {
				continue
			}

			var params LogParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				c.logger.Error("failed to unmarshal log params", "err", err)
				continue
			}
			c.logReceiver.OnLog(params)
		case "":
			// This should be a result from the server from our request earlier, including initialization result.
			c.resultManager.feed(msg)
		}
	}
}

func (c *Client) handleHandlerImplementationMessage(msg JSONRPCMessage) {
	// This variables is used to store all the result from the handler implementation
	// to be sent back to the server below.
	var result any
	// The err is should always an instance of JSONRPCError, we declare it as an error type,
	// is for the nil-check feature.
	var err error

	switch msg.Method {
	case MethodRootsList:
		result, err = c.callListRoots()
	case MethodSamplingCreateMessage:
		result, err = c.callSamplingMessages(msg)
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
			c.logger.Error("failed to call handler implementation",
				slog.String("method", msg.Method),
				slog.String("err", err.Error()))
			resMsg.Error = &jsonErr
		}
	}

	resMsg.Result, _ = json.Marshal(result)

	ctx, cancel := context.WithTimeout(context.Background(), c.pingInterval)
	defer cancel()

	if err := c.session.Send(ctx, resMsg); err != nil {
		c.logger.Error("failed to send result", slog.String("err", err.Error()))
	}
}

func (c *Client) sendRequest(ctx context.Context, method string, params any) (JSONRPCMessage, error) {
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return JSONRPCMessage{}, fmt.Errorf("failed to marshal params: %w", err)
	}

	msgID := uuid.New().String()
	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      MustString(msgID),
		Method:  method,
		Params:  paramsBs,
	}

	results := c.resultManager.register(msgID)

	if err := c.session.Send(ctx, msg); err != nil {
		return JSONRPCMessage{}, fmt.Errorf("failed to send request: %w", err)
	}

	var res JSONRPCMessage
	select {
	case <-ctx.Done():
		err := ctx.Err()
		if !errors.Is(err, context.Canceled) {
			return JSONRPCMessage{}, fmt.Errorf("request timeout: %w", err)
		}

		// If the context is canceled, we should send a notification to the server to indicate the request was cancelled.

		cCtx, cCancel := context.WithTimeout(context.Background(), c.pingInterval)
		defer cCancel()

		params := notificationsCancelledParams{
			RequestID: msgID,
			Reason:    userCancelledReason,
		}

		cancelParamsBs, _ := json.Marshal(params)

		err = nil
		nErr := c.session.Send(cCtx, JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			ID:      MustString(msgID),
			Method:  methodNotificationsCancelled,
			Params:  cancelParamsBs,
		})
		if nErr != nil {
			err = fmt.Errorf("%w: failed to send notification: %w", err, nErr)
		}
		return JSONRPCMessage{}, err
	case res = <-results:
	}

	if res.Error != nil {
		return JSONRPCMessage{}, fmt.Errorf("result error: %w", res.Error)
	}

	return res, nil
}

func (c *Client) listenListRootUpdates() {
	defer close(c.rootsListClosed)

	for range c.rootsListUpdater.RootsListUpdates() {
		ctx, cancel := context.WithTimeout(context.Background(), c.pingInterval)
		if err := c.session.Send(ctx, JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			Method:  methodNotificationsRootsListChanged,
			Params:  nil,
		}); err != nil {
			c.logger.Error("failed to send notification on roots list change", "err", err)
		}
		cancel()
	}
}

func (c *Client) ping(done <-chan struct{}) {
	defer close(c.pingClosed)

	pingTicker := time.NewTicker(c.pingInterval)
	for {
		select {
		case <-done:
			return
		case <-pingTicker.C:
		}

		ctx, cancel := context.WithTimeout(context.Background(), c.pingTimeout)

		msgID := uuid.New().String()
		msg := JSONRPCMessage{
			JSONRPC: JSONRPCVersion,
			ID:      MustString(msgID),
			Method:  methodPing,
		}

		if err := c.session.Send(ctx, msg); err != nil {
			cancel()
			c.logger.Error("failed to send ping to server",
				slog.String("err", err.Error()),
				slog.Any("message", msg))
			if c.onPingFailed != nil {
				c.onPingFailed(err)
			}
			continue
		}

		// We expect pong response from server, so register the result channel for the request.
		results := c.resultManager.register(msgID)

		// Wait for the pong response.
		select {
		case <-ctx.Done():
			cancel()
			nErr := fmt.Errorf("failed to receive pong response from server: %w", ctx.Err())
			c.logger.Error("failed to receive pong response from server", slog.String("err", ctx.Err().Error()))
			if c.onPingFailed != nil {
				c.onPingFailed(nErr)
			}
			continue
		case <-done:
			cancel()
			return
		case res := <-results:
			cancel()
			if res.Error != nil {
				nErr := fmt.Errorf("received pong response error from server: %w", res.Error)
				c.logger.Error("received pong response error from server", slog.String("err", res.Error.Error()))
				if c.onPingFailed != nil {
					c.onPingFailed(nErr)
				}
				continue
			}
		}
		cancel()
	}
}

func (c *Client) sendInitialize(ctx context.Context, msgID MustString) error {
	params := initializeParams{
		ProtocolVersion: protocolVersion,
		Capabilities:    c.capabilities,
		ClientInfo:      c.info,
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal initialize params: %w", err)
	}

	return c.session.Send(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      msgID,
		Method:  methodInitialize,
		Params:  paramsBs,
	})
}

func (c *Client) verifyInitialize(msg JSONRPCMessage) (initializeResult, error) {
	if msg.Error != nil {
		return initializeResult{}, fmt.Errorf("initialize error: %w", msg.Error)
	}

	var result initializeResult
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return initializeResult{}, fmt.Errorf("failed to unmarshal initialize result: %w", err)
	}

	if result.ProtocolVersion != protocolVersion {
		return initializeResult{}, fmt.Errorf("protocol version mismatch: %s != %s", result.ProtocolVersion, protocolVersion)
	}

	return result, nil
}

func (c *Client) callListRoots() (RootList, error) {
	if !c.serverState.isInitialized() {
		return RootList{}, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: "client not initialized",
		}
	}
	if c.rootsListHandler == nil {
		return RootList{}, JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "roots/list not supported by client",
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.pingInterval)
	defer cancel()

	roots, err := c.rootsListHandler.RootsList(ctx)
	if err != nil {
		return RootList{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: fmt.Sprintf("failed to list roots: %s", err.Error()),
		}
	}

	return roots, nil
}

func (c *Client) callSamplingMessages(msg JSONRPCMessage) (SamplingResult, error) {
	if !c.serverState.isInitialized() {
		return SamplingResult{}, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: "client not initialized",
		}
	}
	if c.samplingHandler == nil {
		return SamplingResult{}, JSONRPCError{
			Code:    jsonRPCMethodNotFoundCode,
			Message: "sampling not supported by client",
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.pingInterval)
	defer cancel()

	var params SamplingParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return SamplingResult{}, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: fmt.Sprintf("failed to unmarshal params: %s", err.Error()),
		}
	}

	result, err := c.samplingHandler.CreateSampleMessage(ctx, params)
	if err != nil {
		return SamplingResult{}, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: fmt.Sprintf("failed to create sample message: %s", err.Error()),
		}
	}

	return result, nil
}

func (c *clientResultManager) init() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.closed = false
}

func (c *clientResultManager) register(msgID string) <-chan JSONRPCMessage {
	c.lock.Lock()
	defer c.lock.Unlock()

	results := make(chan JSONRPCMessage)
	c.channels[msgID] = results

	return results
}

func (c *clientResultManager) feed(result JSONRPCMessage) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// It's already closed, so we should return.
	if c.closed {
		return
	}

	ch, ok := c.channels[string(result.ID)]
	if !ok {
		// Ignore the result if it's not registered.
		return
	}

	// Feed the result to the registered channel and drop it if there is no receiver.
	select {
	case ch <- result:
	default:
	}

	// Remove the channel from the map to avoid memory leaks.
	delete(c.channels, string(result.ID))
}

func (c *clientResultManager) close() {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Close all the result channels
	for _, results := range c.channels {
		close(results)
	}

	c.closed = true
}

func (s *serverState) init(info Info, capabilities ServerCapabilities) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.initialized = true
	s.info = info
	s.capabilities = capabilities
	s.stopped = false
}

func (s *serverState) isInitialized() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.initialized
}

func (s *serverState) serverInfo() Info {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.info
}

func (s *serverState) promptServerAvailable() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.capabilities.Prompts != nil
}

func (s *serverState) resourceServerAvailable() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.capabilities.Resources != nil
}

func (s *serverState) toolServerAvailable() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.capabilities.Tools != nil
}

func (s *serverState) loggingAvailable() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.capabilities.Logging != nil
}

func (s *serverState) isStopped() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.stopped
}

func (s *serverState) reset() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.initialized = false
	s.stopped = true
}
