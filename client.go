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
// using Close() when it's no longer needed.
type Client struct {
	capabilities       ClientCapabilities
	info               Info
	serverInfo         Info
	serverCapabilities ServerCapabilities
	transport          ClientTransport

	rootsListHandler RootsListHandler
	rootsListUpdater RootsListUpdater

	samplingHandler SamplingHandler

	promptListWatcher PromptListWatcher

	resourceListWatcher       ResourceListWatcher
	resourceSubscribedWatcher ResourceSubscribedWatcher

	toolListWatcher ToolListWatcher

	progressListener ProgressListener
	logReceiver      LogReceiver

	writeTimeout         time.Duration
	readTimeout          time.Duration
	pingInterval         time.Duration
	pingTimeoutThreshold int

	initialized bool
	logger      *slog.Logger

	waitForResults  chan waitForResultReq
	results         chan JSONRPCMessage
	registerCancels chan cancelRequest
	cancelRequests  chan string
}

var (
	defaultClientWriteTimeout = 30 * time.Second
	defaultClientReadTimeout  = 30 * time.Second
	defaultClientPingInterval = 30 * time.Second

	defaultClientPingTimeoutThreshold = 3
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

// WithClientWriteTimeout sets the write timeout for the client.
func WithClientWriteTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.writeTimeout = timeout
	}
}

// WithClientReadTimeout sets the read timeout for the client.
func WithClientReadTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.readTimeout = timeout
	}
}

// WithClientPingInterval sets the ping interval for the client.
func WithClientPingInterval(interval time.Duration) ClientOption {
	return func(c *Client) {
		c.pingInterval = interval
	}
}

// WithClientPingTimeoutThreshold sets the ping timeout threshold for the client.
// If the number of consecutive ping timeouts exceeds the threshold, the client will close the session.
func WithClientPingTimeoutThreshold(threshold int) ClientOption {
	return func(c *Client) {
		c.pingTimeoutThreshold = threshold
	}
}

// NewClient creates a new Model Context Protocol (MCP) client with the specified configuration.
// It establishes a client that can communicate with MCP servers according to the protocol
// specification at https://spec.modelcontextprotocol.io/specification/.
//
// The info parameter provides client identification and version information. The transport
// parameter defines how the client communicates with the server. ServerRequirement specifies
// which server capabilities are required for this client instance.
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
		info:            info,
		transport:       transport,
		logger:          slog.Default(),
		waitForResults:  make(chan waitForResultReq, 10),
		results:         make(chan JSONRPCMessage),
		registerCancels: make(chan cancelRequest),
		cancelRequests:  make(chan string),
	}
	for _, opt := range options {
		opt(c)
	}

	if c.writeTimeout == 0 {
		c.writeTimeout = defaultClientWriteTimeout
	}
	if c.readTimeout == 0 {
		c.readTimeout = defaultClientReadTimeout
	}
	if c.pingInterval == 0 {
		c.pingInterval = defaultClientPingInterval
	}
	if c.pingTimeoutThreshold == 0 {
		c.pingTimeoutThreshold = defaultClientPingTimeoutThreshold
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
func (c *Client) Connect(ctx context.Context, ready chan<- struct{}) error {
	transportReady := make(chan error)
	msgs, err := c.transport.StartSession(ctx, transportReady)
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	if err = <-transportReady; err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	initMsgID := uuid.New().String()
	if err := c.sendInitialize(ctx, MustString(initMsgID)); err != nil {
		return fmt.Errorf("failed to send initialize request: %w", err)
	}

	return c.listenMessages(ctx, initMsgID, msgs, ready)
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
	if !c.initialized {
		return ListPromptResult{}, errors.New("client not initialized")
	}
	if c.serverCapabilities.Prompts == nil {
		return ListPromptResult{}, errors.New("prompts not supported by server")
	}

	paramsBs, err := json.Marshal(params)
	if err != nil {
		return ListPromptResult{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodPromptsList,
		Params:  paramsBs,
	})
	if err != nil {
		return ListPromptResult{}, err
	}

	if res.Error != nil {
		return ListPromptResult{}, fmt.Errorf("result error: %w", res.Error)
	}

	var result ListPromptResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return ListPromptResult{}, err
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
	if !c.initialized {
		return GetPromptResult{}, errors.New("client not initialized")
	}
	if c.serverCapabilities.Prompts == nil {
		return GetPromptResult{}, errors.New("prompts not supported by server")
	}

	paramsBs, err := json.Marshal(params)
	if err != nil {
		return GetPromptResult{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodPromptsGet,
		Params:  paramsBs,
	})
	if err != nil {
		return GetPromptResult{}, err
	}

	if res.Error != nil {
		return GetPromptResult{}, fmt.Errorf("result error: %w", res.Error)
	}

	var result GetPromptResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return GetPromptResult{}, err
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
	if !c.initialized {
		return CompletionResult{}, errors.New("client not initialized")
	}
	if c.serverCapabilities.Prompts == nil {
		return CompletionResult{}, errors.New("prompts not supported by server")
	}

	paramsBs, err := json.Marshal(params)
	if err != nil {
		return CompletionResult{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodCompletionComplete,
		Params:  paramsBs,
	})
	if err != nil {
		return CompletionResult{}, err
	}

	if res.Error != nil {
		return CompletionResult{}, fmt.Errorf("result error: %w", res.Error)
	}

	var result CompletionResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return CompletionResult{}, err
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
	if !c.initialized {
		return ListResourcesResult{}, errors.New("client not initialized")
	}
	if c.serverCapabilities.Resources == nil {
		return ListResourcesResult{}, errors.New("resources not supported by server")
	}

	paramsBs, err := json.Marshal(params)
	if err != nil {
		return ListResourcesResult{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodResourcesList,
		Params:  paramsBs,
	})
	if err != nil {
		return ListResourcesResult{}, err
	}

	if res.Error != nil {
		return ListResourcesResult{}, fmt.Errorf("result error: %w", res.Error)
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
	if !c.initialized {
		return ReadResourceResult{}, errors.New("client not initialized")
	}
	if c.serverCapabilities.Resources == nil {
		return ReadResourceResult{}, errors.New("resources not supported by server")
	}

	paramsBs, err := json.Marshal(params)
	if err != nil {
		return ReadResourceResult{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodResourcesRead,
		Params:  paramsBs,
	})
	if err != nil {
		return ReadResourceResult{}, err
	}

	if res.Error != nil {
		return ReadResourceResult{}, fmt.Errorf("result error: %w", res.Error)
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
	if !c.initialized {
		return ListResourceTemplatesResult{}, errors.New("client not initialized")
	}
	if c.serverCapabilities.Resources == nil {
		return ListResourceTemplatesResult{}, errors.New("resources not supported by server")
	}

	paramsBs, err := json.Marshal(params)
	if err != nil {
		return ListResourceTemplatesResult{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodResourcesTemplatesList,
		Params:  paramsBs,
	})
	if err != nil {
		return ListResourceTemplatesResult{}, err
	}

	if res.Error != nil {
		return ListResourceTemplatesResult{}, fmt.Errorf("result error: %w", res.Error)
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
	if !c.initialized {
		return CompletionResult{}, errors.New("client not initialized")
	}
	if c.serverCapabilities.Resources == nil {
		return CompletionResult{}, errors.New("resources not supported by server")
	}

	paramsBs, err := json.Marshal(params)
	if err != nil {
		return CompletionResult{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodCompletionComplete,
		Params:  paramsBs,
	})
	if err != nil {
		return CompletionResult{}, err
	}

	if res.Error != nil {
		return CompletionResult{}, fmt.Errorf("result error: %w", res.Error)
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
	if !c.initialized {
		return errors.New("client not initialized")
	}
	if c.serverCapabilities.Resources == nil {
		return errors.New("resources not supported by server")
	}

	paramsBs, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodResourcesSubscribe,
		Params:  paramsBs,
	})
	if err != nil {
		return err
	}

	if res.Error != nil {
		return fmt.Errorf("result error: %w", res.Error)
	}

	return nil
}

// UnsubscribeResource unregisters the client for notifications about changes to a specific resource.
func (c *Client) UnsubscribeResource(ctx context.Context, params UnsubscribeResourceParams) error {
	if !c.initialized {
		return errors.New("client not initialized")
	}
	if c.serverCapabilities.Resources == nil {
		return errors.New("resources not supported by server")
	}

	paramsBs, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodResourcesUnsubscribe,
		Params:  paramsBs,
	})
	if err != nil {
		return err
	}

	if res.Error != nil {
		return fmt.Errorf("result error: %w", res.Error)
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
	if !c.initialized {
		return ListToolsResult{}, errors.New("client not initialized")
	}
	if c.serverCapabilities.Tools == nil {
		return ListToolsResult{}, errors.New("tools not supported by server")
	}

	paramsBs, err := json.Marshal(params)
	if err != nil {
		return ListToolsResult{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodToolsList,
		Params:  paramsBs,
	})
	if err != nil {
		return ListToolsResult{}, err
	}

	if res.Error != nil {
		return ListToolsResult{}, fmt.Errorf("result error: %w", res.Error)
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
	if !c.initialized {
		return CallToolResult{}, errors.New("client not initialized")
	}
	if c.serverCapabilities.Tools == nil {
		return CallToolResult{}, errors.New("tools not supported by server")
	}

	paramsBs, err := json.Marshal(params)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodToolsCall,
		Params:  paramsBs,
	})
	if err != nil {
		return CallToolResult{}, err
	}

	if res.Error != nil {
		return CallToolResult{}, fmt.Errorf("result error: %w", res.Error)
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
func (c *Client) SetLogLevel(level LogLevel) error {
	if !c.initialized {
		return errors.New("client not initialized")
	}
	if c.serverCapabilities.Logging == nil {
		return errors.New("logging not supported by server")
	}

	params := LogParams{
		Level: level,
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}
	return c.sendRequestWithoutResult(context.Background(), JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodLoggingSetLevel,
		Params:  paramsBs,
	})
}

// ServerInfo returns the server's info.
func (c *Client) ServerInfo() Info {
	return c.serverInfo
}

// PromptServerSupported returns true if the server supports prompt management.
func (c *Client) PromptServerSupported() bool {
	return c.serverCapabilities.Prompts != nil
}

// ResourceServerSupported returns true if the server supports resource management.
func (c *Client) ResourceServerSupported() bool {
	return c.serverCapabilities.Resources != nil
}

// ToolServerSupported returns true if the server supports tool management.
func (c *Client) ToolServerSupported() bool {
	return c.serverCapabilities.Tools != nil
}

// LoggingServerSupported returns true if the server supports logging.
func (c *Client) LoggingServerSupported() bool {
	return c.serverCapabilities.Logging != nil
}

func (c *Client) start(ctx context.Context, errs chan<- error) {
	defer close(errs)

	if c.rootsListUpdater != nil {
		go c.listenListRootUpdates(ctx)
	}

	pingTicker := time.NewTicker(c.pingInterval)
	failedPings := 0

	// We maintain two maps for handling request/response correlation and cancellation:
	// - waitForResults: tracks pending requests awaiting responses
	// - cancels: stores cancellation functions for active requests
	waitForResults := make(map[string]chan JSONRPCMessage) // map[msgID]chan JSONRPCMessage
	cancels := make(map[string]context.CancelFunc)         // map[msgID]context.CancelFunc

	for {
		select {
		case <-ctx.Done():
			return
		case <-pingTicker.C:
			if err := c.ping(ctx); err != nil {
				c.logger.Error("failed to send ping", "err", err)
				failedPings++
				if failedPings > c.pingTimeoutThreshold {
					errs <- fmt.Errorf("too many ping failures: %d", failedPings)
					return
				}
			} else {
				failedPings = 0
			}
		case req := <-c.waitForResults:
			resChan := make(chan JSONRPCMessage)
			waitForResults[req.msgID] = resChan
			req.resChan <- resChan
		case msg := <-c.results:
			resChan, ok := waitForResults[string(msg.ID)]
			if !ok {
				continue
			}
			resChan <- msg
			delete(waitForResults, string(msg.ID))
		case cancelReq := <-c.registerCancels:
			cancelCtx, cancel := context.WithCancel(ctx)
			cancelReq.ctxChan <- cancelCtx
			cancels[cancelReq.msgID] = cancel
		case msgID := <-c.cancelRequests:
			cancel, ok := cancels[msgID]
			if !ok {
				continue
			}
			cancel()
			delete(cancels, msgID)
		}
	}
}

func (c Client) listenListRootUpdates(ctx context.Context) {
	for range c.rootsListUpdater.RootsListUpdates() {
		if err := c.sendNotification(ctx, methodNotificationsRootsListChanged, nil); err != nil {
			c.logger.Error("failed to send notification on roots list change", "err", err)
		}
	}
}

func (c *Client) ping(ctx context.Context) error {
	wCtx, wCancel := context.WithTimeout(ctx, c.writeTimeout)
	defer wCancel()

	res, err := c.sendRequest(wCtx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  methodPing,
	})
	if err != nil {
		return fmt.Errorf("failed to send ping: %w", err)
	}
	if res.Error != nil {
		return fmt.Errorf("error response: %w", res.Error)
	}

	return nil
}

func (c *Client) listenMessages(
	ctx context.Context,
	initMsgID string,
	msgs iter.Seq[JSONRPCMessage],
	ready chan<- struct{},
) error {
	defer close(ready)

	startErrs := make(chan error)

	for msg := range msgs {
		if msg.JSONRPC != JSONRPCVersion {
			c.logger.Error("invalid jsonrpc version", "version", msg.JSONRPC)
			continue
		}

		// We handle different message types through a switch statement.
		// Each notification type is processed asynchronously to prevent blocking
		switch msg.Method {
		case methodPing:
			if err := c.sendResult(ctx, msg.ID, nil); err != nil {
				c.logger.Error("failed to handle ping", "err", err)
			}
		case MethodRootsList:
			go c.handleListRoots(ctx, msg) // Async handling of roots list requests
		case MethodSamplingCreateMessage:
			go c.handleSamplingMessages(ctx, msg) // Async handling of sampling messages
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
				var params SubscribeResourceParams
				if err := json.Unmarshal(msg.Params, &params); err != nil {
					c.logger.Error("failed to unmarshal resources subscribe params", "err", err)
					continue
				}
				c.resourceSubscribedWatcher.OnResourceSubscribedChanged(params.URI)
			}
		case methodNotificationsToolsListChanged:
			if c.toolListWatcher != nil {
				c.toolListWatcher.OnToolListChanged()
			}
		case methodNotificationsProgress:
			if c.progressListener == nil {
				continue
			}

			var params ProgressParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				c.logger.Error("failed to unmarshal progress params", "err", err)
				continue
			}
			c.progressListener.OnProgress(params)
		case methodNotificationsMessage:
			if c.logReceiver == nil {
				continue
			}

			var params LogParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				c.logger.Error("failed to unmarshal log params", "err", err)
				continue
			}
			c.logReceiver.OnLog(params)
		case methodNotificationsCancelled:
			c.cancelRequests <- string(msg.ID)
		case "":
			if string(msg.ID) == initMsgID {
				if err := c.handleInitialize(ctx, msg); err != nil {
					return err
				}
				go c.start(ctx, startErrs)
				ready <- struct{}{}
				continue
			}
			c.results <- msg
		}
	}

	return <-startErrs
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

	return c.transport.Send(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      msgID,
		Method:  methodInitialize,
		Params:  paramsBs,
	})
}

func (c *Client) handleInitialize(ctx context.Context, msg JSONRPCMessage) error {
	if msg.Error != nil {
		return fmt.Errorf("initialize error: %w", msg.Error)
	}

	var result initializeResult
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return fmt.Errorf("failed to unmarshal initialize result: %w", err)
	}

	if result.ProtocolVersion != protocolVersion {
		nErr := fmt.Errorf("protocol version mismatch: %s != %s", result.ProtocolVersion, protocolVersion)
		if err := c.sendError(ctx, msg.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgUnsupportedProtocolVersion,
			Data:    map[string]any{"error": nErr},
		}); err != nil {
			nErr = fmt.Errorf("%w: failed to send error on initialize: %w", nErr, err)
		}
		return nErr
	}

	c.serverInfo = result.ServerInfo
	c.serverCapabilities = result.Capabilities
	c.initialized = true

	return c.sendNotification(context.Background(), methodNotificationsInitialized, nil)
}

func (c *Client) handleListRoots(ctx context.Context, msg JSONRPCMessage) {
	if c.rootsListHandler == nil {
		return
	}

	roots, err := c.rootsListHandler.RootsList(ctx)
	if err != nil {
		c.logger.Error("failed to list roots", "err", err)
		return
	}
	if err := c.sendResult(ctx, msg.ID, roots); err != nil {
		c.logger.Error("failed to send result", "err", err)
	}
}

func (c *Client) handleSamplingMessages(ctx context.Context, msg JSONRPCMessage) {
	if c.samplingHandler == nil {
		return
	}

	var params SamplingParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		c.logger.Error("failed to unmarshal sampling params", "err", err)
		return
	}

	result, err := c.samplingHandler.CreateSampleMessage(ctx, params)
	if err != nil {
		c.logger.Error("failed to create sample message", "err", err)
		return
	}

	if err := c.sendResult(ctx, msg.ID, result); err != nil {
		c.logger.Error("failed to send result", "err", err)
	}
}

func (c *Client) sendRequestWithoutResult(ctx context.Context, msg JSONRPCMessage) error {
	sCtx, sCancel := context.WithTimeout(ctx, c.writeTimeout)
	defer sCancel()

	return c.transport.Send(sCtx, msg)
}

func (c *Client) sendRequest(ctx context.Context, msg JSONRPCMessage) (JSONRPCMessage, error) {
	msgID := uuid.New().String()
	msg.ID = MustString(msgID)

	// We create a channel to receive the response channel, allowing for async request handling
	resChannels := make(chan chan JSONRPCMessage)
	wfrReq := waitForResultReq{
		msgID:   msgID,
		resChan: resChannels,
	}

	select {
	case <-ctx.Done():
		return JSONRPCMessage{}, errors.New("client closed")
	case c.waitForResults <- wfrReq:
	}

	results := <-resChannels

	sCtx, sCancel := context.WithTimeout(ctx, c.writeTimeout)
	defer sCancel()

	if err := c.transport.Send(sCtx, msg); err != nil {
		return JSONRPCMessage{}, err
	}

	ticker := time.NewTicker(c.readTimeout)

	var resMsg JSONRPCMessage

	select {
	case <-ticker.C:
		return JSONRPCMessage{}, errors.New("request timeout")
	case <-sCtx.Done():
		err := sCtx.Err()
		if !errors.Is(err, context.Canceled) {
			return JSONRPCMessage{}, err
		}
		err = nil
		nErr := c.sendNotification(ctx, methodNotificationsCancelled, notificationsCancelledParams{
			RequestID: msgID,
			Reason:    userCancelledReason,
		})
		if nErr != nil {
			err = fmt.Errorf("%w: failed to send notification: %w", err, nErr)
		}
		return JSONRPCMessage{}, err
	case resMsg = <-results:
	}

	return resMsg, nil
}

func (c *Client) sendNotification(ctx context.Context, method string, params any) error {
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	notif := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  paramsBs,
	}

	sCtx, sCancel := context.WithTimeout(ctx, c.writeTimeout)
	defer sCancel()

	if err := c.transport.Send(sCtx, notif); err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}

	return nil
}

func (c *Client) sendResult(ctx context.Context, id MustString, result any) error {
	resBs, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  resBs,
	}

	sCtx, sCancel := context.WithTimeout(ctx, c.writeTimeout)
	defer sCancel()

	if err := c.transport.Send(sCtx, msg); err != nil {
		return fmt.Errorf("failed to send result: %w", err)
	}

	return nil
}

func (c *Client) sendError(ctx context.Context, id MustString, err JSONRPCError) error {
	c.logger.Error("request error", "err", err)
	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   &err,
	}

	sCtx, sCancel := context.WithTimeout(ctx, c.writeTimeout)
	defer sCancel()

	if err := c.transport.Send(sCtx, msg); err != nil {
		return fmt.Errorf("failed to send error: %w", err)
	}

	return nil
}
