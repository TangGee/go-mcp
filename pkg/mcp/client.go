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

// ClientOption is a function that configures a client.
type ClientOption func(*Client)

// ServerRequirement is a struct that specifies which server capabilities are required.
type ServerRequirement struct {
	PromptServer   bool
	ResourceServer bool
	ToolServer     bool
}

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
//
// Example usage:
//
//	client := NewClient(info, transport, requirement)
//	if err := client.Connect(); err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	// Use client methods...
//	prompts, err := client.ListPrompts(ctx, "", "")
type Client struct {
	capabilities               ClientCapabilities
	info                       Info
	requiredServerCapabilities ServerCapabilities
	transport                  ClientTransport

	sessionID string
	// clientRequests is a map of requestID to chan JSONRPCMessage, used for mapping the result to the original request
	clientRequests sync.Map
	// serverRequests is a map of requestID to request, used for cancelling requests
	serverRequests sync.Map

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

	initialized bool

	errsChan  chan error
	closeChan chan struct{}
}

var (
	defaultClientWriteTimeout = 30 * time.Second
	defaultClientReadTimeout  = 30 * time.Second
	defaultClientPingInterval = 30 * time.Second
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
	serverRequirement ServerRequirement,
	options ...ClientOption,
) *Client {
	c := &Client{
		info:      info,
		transport: transport,
		errsChan:  make(chan error),
		closeChan: make(chan struct{}),
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

	if serverRequirement.PromptServer {
		c.requiredServerCapabilities.Prompts = &PromptsCapability{}
		if c.promptListWatcher != nil {
			c.requiredServerCapabilities.Prompts = &PromptsCapability{
				ListChanged: true,
			}
		}
	}

	if serverRequirement.ResourceServer {
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

	if serverRequirement.ToolServer {
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

	if c.rootsListUpdater != nil {
		go c.listenRootsList()
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
func (c *Client) Connect() error {
	sessID, err := c.transport.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	go c.listenMessages()
	go c.pings()

	c.sessionID = sessID
	if err := c.initialize(); err != nil {
		return fmt.Errorf("failed to initialize client: %w", err)
	}

	return nil
}

// ListPrompts retrieves a paginated list of available prompts from the server.
// It returns a PromptList containing prompt metadata and pagination information.
//
// The cursor parameter enables pagination through the prompt list. An empty cursor
// starts from the beginning. Use the NextCursor from the returned PromptList for
// subsequent pages.
//
// The progressToken allows tracking the operation's progress through progress notifications.
// Pass an empty string if progress tracking is not needed.
//
// If the provided context is cancelled, a cancellation request will be sent to the
// server to stop processing the request.
func (c *Client) ListPrompts(ctx context.Context, cursor string, progressToken MustString) (PromptList, error) {
	params := PromptsListParams{
		Cursor: cursor,
		Meta:   ParamsMeta{ProgressToken: progressToken},
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return PromptList{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodPromptsList,
		Params:  paramsBs,
	})
	if err != nil {
		return PromptList{}, err
	}

	var result PromptList
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return PromptList{}, err
	}

	return result, nil
}

// GetPrompt retrieves a specific prompt by name with the given arguments.
// It returns a PromptResult containing the prompt's content and metadata.
//
// The name parameter specifies which prompt to retrieve. The arguments parameter
// allows passing key-value pairs that will be used to customize the prompt content.
//
// The progressToken allows tracking the operation's progress through progress notifications.
// Pass an empty string if progress tracking is not needed.
//
// If the provided context is cancelled, a cancellation request will be sent to the
// server to stop processing the request. This is particularly useful for long-running
// prompt generations that need to be interrupted.
func (c *Client) GetPrompt(
	ctx context.Context,
	name string,
	arguments map[string]string,
	progressToken MustString,
) (PromptResult, error) {
	params := PromptsGetParams{
		Name:      name,
		Arguments: arguments,
		Meta:      ParamsMeta{ProgressToken: progressToken},
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return PromptResult{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodPromptsGet,
		Params:  paramsBs,
	})
	if err != nil {
		return PromptResult{}, err
	}

	var result PromptResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return PromptResult{}, err
	}

	return result, nil
}

// CompletesPrompt requests completion suggestions for a prompt-based completion.
// It returns a CompletionResult containing the completion suggestions.
//
// The name parameter specifies which prompt to use as the completion source.
// The arg parameter provides the completion context and configuration through
// the CompletionArgument structure.
//
// This method is particularly useful for implementing intelligent code completion
// or text suggestions based on prompt templates.
//
// If the provided context is cancelled, a cancellation request will be sent to the
// server to stop processing the completion request. This allows graceful interruption
// of long-running completion operations.
func (c *Client) CompletesPrompt(ctx context.Context, name string, arg CompletionArgument) (CompletionResult, error) {
	params := CompletionCompleteParams{
		Ref: CompletionCompleteRef{
			Type: CompletionRefPrompt,
			Name: name,
		},
		Argument: arg,
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

	var result CompletionResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return CompletionResult{}, err
	}

	return result, nil
}

// ListResources retrieves a paginated list of available resources from the server.
// It returns a ResourceList containing resource metadata and pagination information.
//
// The cursor parameter enables pagination through the resource list. An empty cursor
// starts from the beginning. Use the NextCursor from the returned ResourceList for
// subsequent pages.
//
// The progressToken allows tracking the operation's progress through progress notifications.
// Pass an empty string if progress tracking is not needed.
//
// If the provided context is cancelled, a cancellation request will be sent to the
// server to stop processing the request.
func (c *Client) ListResources(ctx context.Context, cursor string, progressToken MustString) (ResourceList, error) {
	params := ResourcesListParams{
		Cursor: cursor,
		Meta:   ParamsMeta{ProgressToken: progressToken},
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return ResourceList{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodResourcesList,
		Params:  paramsBs,
	})
	if err != nil {
		return ResourceList{}, err
	}

	var result ResourceList
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return ResourceList{}, err
	}

	return result, nil
}

// ReadResource retrieves the content and metadata of a specific resource identified by its URI.
// It returns a Resource containing the resource's content, type, and associated metadata.
//
// The uri parameter specifies which resource to retrieve. The URI format should follow
// the server's resource addressing scheme.
//
// The progressToken allows tracking the operation's progress through progress notifications.
// Pass an empty string if progress tracking is not needed.
//
// If the provided context is cancelled, a cancellation request will be sent to the
// server to stop processing the request.
func (c *Client) ReadResource(ctx context.Context, uri string, progressToken MustString) (Resource, error) {
	params := ResourcesReadParams{
		URI:  uri,
		Meta: ParamsMeta{ProgressToken: progressToken},
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return Resource{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodResourcesRead,
		Params:  paramsBs,
	})
	if err != nil {
		return Resource{}, err
	}

	var result Resource
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return Resource{}, err
	}

	return result, nil
}

// ListResourceTemplates retrieves a list of available resource templates from the server.
// Resource templates allow servers to expose parameterized resources using URI templates.
// These templates can be used to generate resource URIs by providing arguments,
// which may be auto-completed through the CompletesResourceTemplate.
//
// The progressToken allows tracking the operation's progress through progress notifications.
// Pass an empty string if progress tracking is not needed.
//
// If the provided context is cancelled, a cancellation request will be sent to the
// server to stop processing the request.
func (c *Client) ListResourceTemplates(ctx context.Context, progressToken MustString) ([]ResourceTemplate, error) {
	params := ResourcesTemplatesListParams{
		Meta: ParamsMeta{ProgressToken: progressToken},
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodResourcesTemplatesList,
		Params:  paramsBs,
	})
	if err != nil {
		return nil, err
	}

	var result []ResourceTemplate
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// CompletesResourceTemplate requests completion suggestions for a resource template.
// It returns a CompletionResult containing the completion suggestions based on the
// template's content and structure.
//
// The uri parameter identifies the resource template to use for completion.
// The arg parameter provides completion context and configuration through the
// CompletionArgument structure.
//
// This method is particularly useful for implementing intelligent code completion
// or content suggestions based on resource templates.
//
// If the provided context is cancelled, a cancellation request will be sent to the
// server to stop processing the completion request.
func (c *Client) CompletesResourceTemplate(
	ctx context.Context,
	uri string,
	arg CompletionArgument,
) (CompletionResult, error) {
	params := CompletionCompleteParams{
		Ref: CompletionCompleteRef{
			Type: CompletionRefResource,
			URI:  uri,
		},
		Argument: arg,
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
// The uri parameter identifies the resource to monitor for changes.
func (c *Client) SubscribeResource(ctx context.Context, uri string) error {
	params := ResourcesSubscribeParams{
		URI: uri,
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}
	_, err = c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodResourcesSubscribe,
		Params:  paramsBs,
	})
	if err != nil {
		return err
	}

	return nil
}

// UnsubscribeResource cancels an existing subscription for notifications about changes
// to a specific resource. After unsubscribing, the client will no longer receive
// notifications through the ResourceSubscribedWatcher interface for this resource.
//
// The uri parameter identifies the resource to stop monitoring for changes.
func (c *Client) UnsubscribeResource(ctx context.Context, uri string) error {
	params := ResourcesSubscribeParams{
		URI: uri,
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}
	_, err = c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodResourcesUnsubscribe,
		Params:  paramsBs,
	})
	if err != nil {
		return err
	}

	return nil
}

// ListTools retrieves a paginated list of available tools from the server.
// It returns a ToolList containing tool metadata and pagination information.
//
// The cursor parameter enables pagination through the tool list. An empty cursor
// starts from the beginning. Use the NextCursor from the returned ToolList for
// subsequent pages.
//
// The progressToken allows tracking the operation's progress through progress notifications.
// Pass an empty string if progress tracking is not needed.
//
// If the provided context is cancelled, a cancellation request will be sent to the
// server to stop processing the request.
func (c *Client) ListTools(ctx context.Context, cursor string, progressToken MustString) (ToolList, error) {
	params := ToolsListParams{
		Cursor: cursor,
		Meta:   ParamsMeta{ProgressToken: progressToken},
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return ToolList{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodToolsList,
		Params:  paramsBs,
	})
	if err != nil {
		return ToolList{}, err
	}

	var result ToolList
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return ToolList{}, err
	}

	return result, nil
}

// CallTool executes a specific tool with the given arguments and returns its result.
// It provides a way to invoke server-side tools that can perform specialized operations.
//
// The name parameter identifies which tool to execute. The arguments parameter
// provides tool-specific configuration as key-value pairs that will be passed to
// the tool during execution.
//
// The progressToken allows tracking the operation's progress through progress notifications.
// Pass an empty string if progress tracking is not needed.
//
// If the provided context is cancelled, a cancellation request will be sent to the
// server to stop processing the tool execution. This is particularly useful for
// long-running tool operations that need to be interrupted.
func (c *Client) CallTool(
	ctx context.Context,
	name string,
	arguments map[string]any,
	progressToken MustString,
) (ToolResult, error) {
	params := ToolsCallParams{
		Name:      name,
		Arguments: arguments,
		Meta:      ParamsMeta{ProgressToken: progressToken},
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return ToolResult{}, fmt.Errorf("failed to marshal params: %w", err)
	}
	res, err := c.sendRequest(ctx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodToolsCall,
		Params:  paramsBs,
	})
	if err != nil {
		return ToolResult{}, err
	}

	var result ToolResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return ToolResult{}, err
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
	params := LogParams{
		Level: level,
	}
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}
	_, err = c.sendRequest(context.Background(), JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  MethodLoggingSetLevel,
		Params:  paramsBs,
	})
	if err != nil {
		return err
	}

	return nil
}

// Errors returns a channel that provides access to errors encountered during
// client operations. This includes transport errors, protocol violations,
// and other operational issues that don't directly relate to specific method calls.
//
// The returned channel is receive-only and will be closed when the client is closed.
// Clients should monitor this channel to detect and handle operational issues.
//
// Note that method-specific errors are returned directly by the respective methods
// and won't appear on this channel.
func (c *Client) Errors() <-chan error {
	return c.errsChan
}

// Close terminates the client's connection to the server and releases all associated resources.
// It closes the error channel, stops all background routines, and terminates the transport connection.
//
// After Close is called, the client cannot be reused. A new client must be created to establish
// another connection.
func (c *Client) Close() {
	close(c.errsChan)
	close(c.closeChan)
	c.transport.Close()
}

func (c *Client) initialize() error {
	sCtx, sCancel := context.WithTimeout(context.Background(), c.writeTimeout)
	defer sCancel()

	params := initializeParams{
		ProtocolVersion: protocolVersion,
		Capabilities:    c.capabilities,
		ClientInfo:      c.info,
	}

	paramsBs, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	res, err := c.sendRequest(sCtx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  methodInitialize,
		Params:  paramsBs,
	})
	if err != nil {
		return fmt.Errorf("failed to send initialize request: %w", err)
	}

	if res.Error != nil {
		return res.Error
	}

	var result initializeResult
	if err := json.Unmarshal(res.Result, &result); err != nil {
		return fmt.Errorf("failed to unmarshal initialize result: %w", err)
	}

	if result.ProtocolVersion != protocolVersion {
		nErr := fmt.Errorf("protocol version mismatch: %s != %s", result.ProtocolVersion, protocolVersion)
		if err := c.sendError(context.Background(), res.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgUnsupportedProtocolVersion,
			Data:    map[string]any{"error": nErr},
		}); err != nil {
			nErr = fmt.Errorf("%w: failed to send error on initialize: %w", nErr, err)
		}
		return nErr
	}

	if err := c.checkCapabilities(result, c.requiredServerCapabilities); err != nil {
		nErr := fmt.Errorf("failed to check capabilities: %w", err)
		if err := c.sendError(context.Background(), res.ID, JSONRPCError{
			Code:    jsonRPCInvalidParamsCode,
			Message: errMsgInsufficientClientCapabilities,
			Data:    map[string]any{"error": err},
		}); err != nil {
			nErr = fmt.Errorf("%w: failed to send error on initialize: %w", nErr, err)
		}
		return nErr
	}

	c.initialized = true

	return c.sendNotification(context.Background(), methodNotificationsInitialized, nil)
}

func (c *Client) checkCapabilities(result initializeResult, requiredServerCap ServerCapabilities) error {
	if requiredServerCap.Prompts != nil {
		if result.Capabilities.Prompts == nil {
			nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'prompts'")
			return nErr
		}
		if requiredServerCap.Prompts.ListChanged {
			if !result.Capabilities.Prompts.ListChanged {
				nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'prompts.listChanged'")
				return nErr
			}
		}
	}

	if requiredServerCap.Resources != nil {
		if result.Capabilities.Resources == nil {
			nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'resources'")
			return nErr
		}
		if requiredServerCap.Resources.ListChanged {
			if !result.Capabilities.Resources.ListChanged {
				nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'resources.listChanged'")
				return nErr
			}
		}
		if requiredServerCap.Resources.Subscribe {
			if !result.Capabilities.Resources.Subscribe {
				nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'resources.subscribe'")
				return nErr
			}
		}
	}

	if requiredServerCap.Tools != nil {
		if result.Capabilities.Tools == nil {
			nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'tools'")
			return nErr
		}
		if requiredServerCap.Tools.ListChanged {
			if !result.Capabilities.Tools.ListChanged {
				nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'tools.listChanged'")
				return nErr
			}
		}
	}

	if requiredServerCap.Logging != nil {
		if result.Capabilities.Logging == nil {
			nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'logging'")
			return nErr
		}
	}

	return nil
}

func (c *Client) listenRootsList() {
	lists := c.rootsListUpdater.RootsListUpdates()
	for {
		select {
		case <-c.closeChan:
			return
		case <-lists:
		}
		if err := c.sendNotification(context.Background(), methodNotificationsRootsListChanged, nil); err != nil {
			c.logError(fmt.Errorf("failed to send notification on roots list change: %w", err))
		}
	}
}

func (c *Client) listenMessages() {
	msgs := c.transport.SessionMessages()

	var msg SessionMsgWithErrs
	for {
		select {
		case <-c.closeChan:
			return
		case msg = <-msgs:
		}

		if msg.SessionID != c.sessionID {
			msg.Errs <- fmt.Errorf("invalid session ID: %s", msg.SessionID)
			return
		}

		msg.Errs <- c.handleMsg(msg.Msg)
	}
}

func (c *Client) pings() {
	pingTicker := time.NewTicker(c.pingInterval)

	for {
		select {
		case <-c.closeChan:
			return
		case <-pingTicker.C:
			c.ping()
		}
	}
}

func (c *Client) ping() {
	wCtx, wCancel := context.WithTimeout(context.Background(), c.writeTimeout)
	defer wCancel()

	res, err := c.sendRequest(wCtx, JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  methodPing,
	})
	if err != nil {
		c.logError(fmt.Errorf("failed to send ping: %w", err))
		return
	}
	if res.Error != nil {
		c.logError(fmt.Errorf("error response: %w", res.Error))
	}
}

func (c *Client) handleMsg(msg JSONRPCMessage) error {
	if msg.JSONRPC != JSONRPCVersion {
		nErr := fmt.Errorf("invalid jsonrpc version: %s", msg.JSONRPC)
		c.logError(nErr)
		return nErr
	}

	// Handle basic protocol messages
	if err := c.handleBasicMessages(msg); err != nil {
		return err
	}

	// Handle root-related messages
	if err := c.handleRootMessages(msg); err != nil {
		return err
	}

	// Handle sampling-related messages
	if err := c.handleSamplingMessages(msg); err != nil {
		return err
	}

	// Handle notification messages
	if err := c.handleNotificationMessages(msg); err != nil {
		return err
	}

	// Handle result messages
	if err := c.handleResultMessages(msg); err != nil {
		return err
	}

	return nil
}

func (c *Client) handleBasicMessages(msg JSONRPCMessage) error {
	if msg.Method != methodPing {
		return nil
	}
	if err := c.sendResult(context.Background(), msg.ID, nil); err != nil {
		nErr := fmt.Errorf("failed to handle ping: %w", err)
		c.logError(nErr)
		return nErr
	}

	return nil
}

func (c *Client) handleRootMessages(msg JSONRPCMessage) error {
	if c.rootsListHandler == nil {
		return nil
	}

	if msg.Method != MethodRootsList {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.serverRequests.Store(msg.ID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	rl, err := c.rootsListHandler.RootsList(ctx)
	if err != nil {
		nErr := fmt.Errorf("failed to list roots: %w", err)
		if err := c.sendError(ctx, msg.ID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		}); err != nil {
			nErr = fmt.Errorf("%w: failed to send error on roots list: %w", nErr, err)
		}
		c.logError(nErr)
		return nErr
	}

	if err := c.sendResult(ctx, msg.ID, rl); err != nil {
		nErr := fmt.Errorf("failed to send result on roots list: %w", err)
		c.logError(nErr)
		return nErr
	}

	return nil
}

func (c *Client) handleSamplingMessages(msg JSONRPCMessage) error {
	if c.samplingHandler == nil {
		return nil
	}

	if msg.Method != MethodSamplingCreateMessage {
		return nil
	}
	var params SamplingParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		nErr := fmt.Errorf("failed to unmarshal sampling params: %w", err)
		c.logError(nErr)
		return nErr
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.serverRequests.Store(msg.ID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	rl, err := c.samplingHandler.CreateSampleMessage(ctx, params)
	if err != nil {
		nErr := fmt.Errorf("failed to create sample message: %w", err)
		if err := c.sendError(ctx, msg.ID, JSONRPCError{
			Code:    jsonRPCInternalErrorCode,
			Message: errMsgInternalError,
			Data:    map[string]any{"error": nErr},
		}); err != nil {
			nErr = fmt.Errorf("%w: failed to send error on create sample message: %w", nErr, err)
		}
		c.logError(nErr)
		return nErr
	}

	if err := c.sendResult(ctx, msg.ID, rl); err != nil {
		nErr := fmt.Errorf("failed to send result on create sample message: %w", err)
		c.logError(nErr)
		return nErr
	}

	return nil
}

func (c *Client) handleNotificationMessages(msg JSONRPCMessage) error {
	switch msg.Method {
	case methodNotificationsCancelled:
		var params notificationsCancelledParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			nErr := fmt.Errorf("failed to unmarshal notifications cancelled params: %w", err)
			c.logError(nErr)
			return nErr
		}
		c.handleNotificationsCancelled(params)
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
				nErr := fmt.Errorf("failed to unmarshal resources subscribe params: %w", err)
				c.logError(nErr)
				return nErr
			}
			c.resourceSubscribedWatcher.OnResourceSubscribedChanged(params.URI)
		}
	case methodNotificationsToolsListChanged:
		if c.toolListWatcher != nil {
			c.toolListWatcher.OnToolListChanged()
		}
	case methodNotificationsProgress:
		if c.progressListener == nil {
			return nil
		}

		var params ProgressParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			c.logError(fmt.Errorf("failed to unmarshal progress params: %w", err))
			return nil
		}
		c.progressListener.OnProgress(params)
	case methodNotificationsMessage:
		if c.logReceiver == nil {
			return nil
		}

		var params LogParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			c.logError(fmt.Errorf("failed to unmarshal log params: %w", err))
			return nil
		}
		c.logReceiver.OnLog(params)
	}

	return nil
}

func (c *Client) handleResultMessages(msg JSONRPCMessage) error {
	if msg.Method != "" {
		return nil
	}
	reqID := string(msg.ID)
	rc, ok := c.clientRequests.Load(reqID)
	if !ok {
		return nil
	}
	resChan, _ := rc.(chan JSONRPCMessage)
	resChan <- msg
	return nil
}

func (c *Client) handleNotificationsCancelled(params notificationsCancelledParams) {
	r, ok := c.serverRequests.Load(params.RequestID)
	if !ok {
		return
	}
	req, _ := r.(request)
	req.cancel()
}

func (c *Client) registerRequest() (string, chan JSONRPCMessage) {
	reqID := uuid.New().String()
	resChan := make(chan JSONRPCMessage)
	c.clientRequests.Store(reqID, resChan)
	return reqID, resChan
}

func (c *Client) sendRequest(ctx context.Context, msg JSONRPCMessage) (JSONRPCMessage, error) {
	reqID, resChan := c.registerRequest()
	msg.ID = MustString(reqID)

	sCtx, sCancel := context.WithTimeout(ctx, c.writeTimeout)
	defer sCancel()

	if err := c.transport.Send(sCtx, SessionMsg{
		SessionID: c.sessionID,
		Msg:       msg,
	}); err != nil {
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
		nErr := c.sendNotification(context.Background(), methodNotificationsCancelled, notificationsCancelledParams{
			RequestID: reqID,
			Reason:    userCancelledReason,
		})
		if nErr != nil {
			err = fmt.Errorf("%w: failed to send notification: %w", err, nErr)
		}
		return JSONRPCMessage{}, err
	case resMsg = <-resChan:
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

	if err := c.transport.Send(sCtx, SessionMsg{
		SessionID: c.sessionID,
		Msg:       notif,
	}); err != nil {
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

	if err := c.transport.Send(sCtx, SessionMsg{
		SessionID: c.sessionID,
		Msg:       msg,
	}); err != nil {
		return fmt.Errorf("failed to send result: %w", err)
	}

	return nil
}

func (c *Client) sendError(ctx context.Context, id MustString, err JSONRPCError) error {
	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   &err,
	}

	sCtx, sCancel := context.WithTimeout(ctx, c.writeTimeout)
	defer sCancel()

	if err := c.transport.Send(sCtx, SessionMsg{
		SessionID: c.sessionID,
		Msg:       msg,
	}); err != nil {
		return fmt.Errorf("failed to send error: %w", err)
	}

	return nil
}

func (c *Client) logError(err error) {
	select {
	case c.errsChan <- err:
	default:
	}
}
