package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"

	"github.com/qri-io/jsonschema"
)

// ServerTransport provides the server-side communication layer in the MCP protocol.
type ServerTransport interface {
	// Sessions returns an iterator that yields new client sessions as they are initiated.
	// Each yielded Session represents a unique client connection and provides methods for
	// bidirectional communication. The implementation must guarantee that each session ID
	// is unique across all active connections.
	Sessions() iter.Seq[Session]
}

// ClientTransport provides the client-side communication layer in the MCP protocol.
type ClientTransport interface {
	// StartSession initiates a new session with the server and returns an iterator
	// that yields server messages. The ready channel is used to signal when the transport
	// is ready to send messages.
	//
	// The implementation must ensure StartSession is called only once and must close or feed
	// the ready channel when prepared to send messages. Operations should be canceled if the
	// context is canceled, and appropriate errors should be returned for connection or protocol
	// failures.
	StartSession(ctx context.Context, ready chan<- error) (iter.Seq[JSONRPCMessage], error)

	// Send transmits a message to the server. The implementation must only allow sending
	// after the ready channel from StartSession is closed or fed. Operations should be
	// canceled if the context is canceled, and appropriate errors should be returned for
	// transmission failures.
	Send(ctx context.Context, msg JSONRPCMessage) error
}

// Session represents a bidirectional communication channel between server and client.
type Session interface {
	// ID returns the unique identifier for this session. The implementation must
	// guarantee that session IDs are unique across all active sessions managed by
	// the ServerTransport.
	ID() string

	// Send transmits a message to the client. The implementation should cancel
	// operations if the context is canceled and return appropriate errors for
	// transmission failures.
	Send(ctx context.Context, msg JSONRPCMessage) error

	// Messages returns an iterator that yields messages received from the client.
	Messages() iter.Seq[JSONRPCMessage]
}

// Server interfaces

// PromptServer defines the interface for managing prompts in the MCP protocol.
type PromptServer interface {
	// ListPrompts returns a paginated list of available prompts. The ProgressReporter
	// can be used to report operation progress, and RequestClientFunc enables
	// client-server communication during execution.
	// Returns error if operation fails or context is cancelled.
	ListPrompts(context.Context, ListPromptsParams, ProgressReporter, RequestClientFunc) (ListPromptResult, error)

	// GetPrompt retrieves a specific prompt template by name with the given arguments.
	// The ProgressReporter can be used to report operation progress, and RequestClientFunc
	// enables client-server communication during execution.
	// Returns error if prompt not found, arguments are invalid, or context is cancelled.
	GetPrompt(context.Context, GetPromptParams, ProgressReporter, RequestClientFunc) (GetPromptResult, error)

	// CompletesPrompt provides completion suggestions for a prompt argument.
	// Used to implement interactive argument completion in clients. The RequestClientFunc
	// enables client-server communication during execution.
	// Returns error if prompt doesn't exist, completions cannot be generated, or context is cancelled.
	CompletesPrompt(context.Context, CompletesCompletionParams, RequestClientFunc) (CompletionResult, error)
}

// PromptListUpdater provides an interface for monitoring changes to the available prompts list.
// It maintains a channel that emits notifications whenever prompts are added, removed, or modified.
//
// The notifications are used by the MCP server to inform connected clients about prompt list
// changes via the "notifications/prompts/list_changed" method. Clients can then refresh their
// cached prompt lists by calling ListPrompts again.
//
// The channel returned by PromptListUpdates must:
// - Remain open for the lifetime of the updater
// - Be safe for concurrent receives from multiple goroutines
// - Never block on sends using buffered channels or dropped notifications
//
// A struct{} is sent through the channel as only the notification matters, not the value.
type PromptListUpdater interface {
	PromptListUpdates() <-chan struct{}
}

// ResourceServer defines the interface for managing resources in the MCP protocol.
type ResourceServer interface {
	// ListResources returns a paginated list of available resources. The ProgressReporter
	// can be used to report operation progress, and RequestClientFunc enables
	// client-server communication during execution.
	// Returns error if operation fails or context is cancelled.
	ListResources(context.Context, ListResourcesParams, ProgressReporter, RequestClientFunc) (
		ListResourcesResult, error)

	// ReadResource retrieves a specific resource by its URI. The ProgressReporter
	// can be used to report operation progress, and RequestClientFunc enables
	// client-server communication during execution.
	// Returns error if resource not found, cannot be read, or context is cancelled.
	ReadResource(context.Context, ReadResourceParams, ProgressReporter, RequestClientFunc) (
		ReadResourceResult, error)

	// ListResourceTemplates returns all available resource templates. The ProgressReporter
	// can be used to report operation progress, and RequestClientFunc enables
	// client-server communication during execution.
	// Returns error if templates cannot be retrieved or context is cancelled.
	ListResourceTemplates(context.Context, ListResourceTemplatesParams, ProgressReporter, RequestClientFunc) (
		ListResourceTemplatesResult, error)

	// CompletesResourceTemplate provides completion suggestions for a resource template argument.
	// The RequestClientFunc enables client-server communication during execution.
	// Returns error if template doesn't exist, completions cannot be generated, or context is cancelled.
	CompletesResourceTemplate(context.Context, CompletesCompletionParams, RequestClientFunc) (CompletionResult, error)

	// SubscribeResource registers interest in a specific resource URI.
	SubscribeResource(SubscribeResourceParams)

	// UnsubscribeResource unregisters interest in a specific resource URI.
	UnsubscribeResource(UnsubscribeResourceParams)
}

// ResourceListUpdater provides an interface for monitoring changes to the available resources list.
// It maintains a channel that emits notifications whenever resources are added, removed, or modified.
//
// The notifications are used by the MCP server to inform connected clients about resource list
// changes. Clients can then refresh their cached resource lists by calling ListResources again.
//
// The channel returned by ResourceListUpdates must:
// - Remain open for the lifetime of the updater
// - Be safe for concurrent receives from multiple goroutines
// - Never block on sends using buffered channels or dropped notifications
//
// A struct{} is sent through the channel as only the notification matters, not the value.
type ResourceListUpdater interface {
	ResourceListUpdates() <-chan struct{}
}

// ResourceSubscribedUpdater provides an interface for monitoring changes to subscribed resources.
// It maintains a channel that emits notifications whenever a subscribed resource changes.
//
// The notifications are used by the MCP server to inform connected clients about changes to
// resources they have subscribed to. The channel emits the URI of the changed resource.
//
// The channel returned by ResourceSubscribedUpdates must:
// - Remain open for the lifetime of the updater
// - Be safe for concurrent receives from multiple goroutines
// - Never block on sends using buffered channels or dropped notifications
//
// A string (resource URI) is sent through the channel to identify which resource changed.
type ResourceSubscribedUpdater interface {
	ResourceSubscribedUpdates() <-chan string
}

// ToolServer defines the interface for managing tools in the MCP protocol.
type ToolServer interface {
	// ListTools returns a paginated list of available tools. The ProgressReporter
	// can be used to report operation progress, and RequestClientFunc enables
	// client-server communication during execution.
	// Returns error if operation fails or context is cancelled.
	ListTools(context.Context, ListToolsParams, ProgressReporter, RequestClientFunc) (ListToolsResult, error)

	// CallTool executes a specific tool with the given arguments. The ProgressReporter
	// can be used to report operation progress, and RequestClientFunc enables
	// client-server communication during execution.
	// Returns error if tool not found, arguments are invalid, execution fails, or context is cancelled.
	CallTool(context.Context, CallToolParams, ProgressReporter, RequestClientFunc) (CallToolResult, error)
}

// ToolListUpdater provides an interface for monitoring changes to the available tools list.
// It maintains a channel that emits notifications whenever tools are added, removed, or modified.
//
// The notifications are used by the MCP server to inform connected clients about tool list
// changes. Clients can then refresh their cached tool lists by calling ListTools again.
//
// The channel returned by ToolListUpdates must:
// - Remain open for the lifetime of the updater
// - Be safe for concurrent receives from multiple goroutines
// - Never block on sends using buffered channels or dropped notifications
//
// A struct{} is sent through the channel as only the notification matters, not the value.
type ToolListUpdater interface {
	ToolListUpdates() <-chan struct{}
}

// LogHandler provides an interface for streaming log messages from the MCP server to connected clients.
// It maintains a channel for emitting log messages and allows configuration of minimum severity level.
type LogHandler interface {
	// LogStreams returns a channel that emits log messages with metadata.
	// The channel remains open for the lifetime of the handler and is safe for concurrent receives.
	LogStreams() <-chan LogParams

	// SetLogLevel configures the minimum severity level for emitted log messages.
	// Messages below this level are filtered out.
	SetLogLevel(level LogLevel)
}

// RootsListWatcher provides an interface for receiving notifications when the client's root list changes.
// The implementation can use these notifications to update its internal state or perform necessary actions
// when the client's available roots change.
type RootsListWatcher interface {
	// OnRootsListChanged is called when the client notifies that its root list has changed
	OnRootsListChanged()
}

// Client interfaces

// RootsListHandler defines the interface for retrieving the list of root resources in the MCP protocol.
// Root resources represent top-level entry points in the resource hierarchy that clients can access.
type RootsListHandler interface {
	// RootsList returns the list of available root resources.
	// Returns error if operation fails or context is cancelled.
	RootsList(ctx context.Context) (RootList, error)
}

// RootsListUpdater provides an interface for monitoring changes to the available roots list.
// Implementations should maintain a channel that emits notifications whenever the list of
// available roots changes, such as when roots are added, removed, or modified.
type RootsListUpdater interface {
	// RootsListUpdates returns a channel that emits notifications when the root list changes.
	// The returned channel remains open for the lifetime of the updater and is safe for concurrent use.
	RootsListUpdates() <-chan struct{}
}

// SamplingHandler provides an interface for generating AI model responses based on conversation history.
// It handles the core sampling functionality including managing conversation context, applying model preferences,
// and generating appropriate responses while respecting token limits.
type SamplingHandler interface {
	// CreateSampleMessage generates a response message based on the provided conversation history and parameters.
	// Returns error if model selection fails, generation fails, token limit is exceeded, or context is cancelled.
	CreateSampleMessage(ctx context.Context, params SamplingParams) (SamplingResult, error)
}

// PromptListWatcher provides an interface for receiving notifications when the server's prompt list changes.
// Implementations can use these notifications to update their internal state or trigger UI updates when
// available prompts are added, removed, or modified.
type PromptListWatcher interface {
	// OnPromptListChanged is called when the server notifies that its prompt list has changed.
	// This can happen when prompts are added, removed, or modified on the server side.
	OnPromptListChanged()
}

// ResourceListWatcher provides an interface for receiving notifications when the server's resource list changes.
// Implementations can use these notifications to update their internal state or trigger UI updates when
// available resources are added, removed, or modified.
type ResourceListWatcher interface {
	// OnResourceListChanged is called when the server notifies that its resource list has changed.
	// This can happen when resources are added, removed, or modified on the server side.
	OnResourceListChanged()
}

// ResourceSubscribedWatcher provides an interface for receiving notifications when a subscribed resource changes.
// Implementations can use these notifications to update their internal state or trigger UI updates when
// specific resources they are interested in are modified.
type ResourceSubscribedWatcher interface {
	// OnResourceSubscribedChanged is called when the server notifies that a subscribed resource has changed.
	//
	// Parameters:
	// - uri: The unique identifier of the resource that changed
	OnResourceSubscribedChanged(uri string)
}

// ToolListWatcher provides an interface for receiving notifications when the server's tool list changes.
// Implementations can use these notifications to update their internal state or trigger UI updates when
// available tools are added, removed, or modified.
type ToolListWatcher interface {
	// OnToolListChanged is called when the server notifies that its tool list has changed.
	// This can happen when tools are added, removed, or modified on the server side.
	OnToolListChanged()
}

// ProgressListener provides an interface for receiving progress updates on long-running operations.
// Implementations can use these notifications to update progress bars, status indicators, or other
// UI elements that show operation progress to users.
type ProgressListener interface {
	// OnProgress is called when a progress update is received for an operation.
	OnProgress(params ProgressParams)
}

// LogReceiver provides an interface for receiving log messages from the server.
// Implementations can use these notifications to display logs in a UI, write them to a file,
// or forward them to a logging service.
type LogReceiver interface {
	// OnLog is called when a log message is received from the server.
	OnLog(params LogParams)
}

// JSONRPCMessage represents a JSON-RPC 2.0 message used for communication in the MCP protocol.
// It can represent either a request, response, or notification depending on which fields are populated:
//   - Request: JSONRPC, ID, Method, and Params are set
//   - Response: JSONRPC, ID, and either Result or Error are set
//   - Notification: JSONRPC and Method are set (no ID)
type JSONRPCMessage struct {
	// JSONRPC must always be "2.0" per the JSON-RPC specification
	JSONRPC string `json:"jsonrpc"`
	// ID uniquely identifies request-response pairs and must be a string or number
	ID MustString `json:"id,omitempty"`
	// Method contains the RPC method name for requests and notifications
	Method string `json:"method,omitempty"`
	// Params contains the parameters for the method call as a raw JSON message
	Params json.RawMessage `json:"params,omitempty"`
	// Result contains the successful response data as a raw JSON message
	Result json.RawMessage `json:"result,omitempty"`
	// Error contains error details if the request failed
	Error *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError represents an error response in the JSON-RPC 2.0 protocol.
// It follows the standard error object format defined in the JSON-RPC 2.0 specification.
type JSONRPCError struct {
	// Code indicates the error type that occurred.
	// Must use standard JSON-RPC error codes or custom codes outside the reserved range.
	Code int `json:"code"`

	// Message provides a short description of the error.
	// Should be limited to a concise single sentence.
	Message string `json:"message"`

	// Data contains additional information about the error.
	// The value is unstructured and may be omitted.
	Data map[string]any `json:"data,omitempty"`
}

// Info contains metadata about a server or client instance including its name and version.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MustString is a type that enforces string representation for fields that can be either string or integer
// in the protocol specification, such as request IDs and progress tokens. It handles automatic conversion
// during JSON marshaling/unmarshaling.
type MustString string

// ListPromptsParams contains parameters for listing available prompts.
type ListPromptsParams struct {
	// Cursor is an optional pagination cursor from previous ListPrompts call.
	// Empty string requests the first page.
	Cursor string `json:"cursor"`

	// Meta contains optional metadata including:
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// GetPromptParams contains parameters for retrieving a specific prompt.
type GetPromptParams struct {
	// Name is the unique identifier of the prompt to retrieve
	Name string `json:"name"`

	// Arguments is a map of argument name-value pairs
	// Must satisfy required arguments defined in prompt's Arguments field
	Arguments map[string]string `json:"arguments"`

	// Meta contains optional metadata including:
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ListPromptResult represents a paginated list of prompts returned by ListPrompts.
// NextCursor can be used to retrieve the next page of results.
type ListPromptResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

// Prompt defines a template for generating prompts with optional arguments.
// It's returned by GetPrompt and contains metadata about the prompt.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument defines a single argument that can be passed to a prompt.
// Required indicates whether the argument must be provided when using the prompt.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// GetPromptResult represents the result of a prompt request.
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages,omitempty"`
}

// PromptMessage represents a message in a prompt.
type PromptMessage struct {
	Role    PromptRole `json:"role"`
	Content Content    `json:"content"`
}

// PromptRole represents the role in a conversation (user or assistant).
type PromptRole string

// ListResourcesParams contains parameters for listing available resources.
type ListResourcesParams struct {
	// Cursor is a pagination cursor from previous ListResources call.
	// Empty string requests the first page.
	Cursor string `json:"cursor"`

	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ReadResourceParams contains parameters for retrieving a specific resource.
type ReadResourceParams struct {
	// URI is the unique identifier of the resource to retrieve.
	URI string `json:"uri"`

	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ListResourceTemplatesParams contains parameters for listing available resource templates.
type ListResourceTemplatesParams struct {
	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// SubscribeResourceParams contains parameters for subscribing to a resource.
type SubscribeResourceParams struct {
	// URI is the unique identifier of the resource to subscribe to.
	// Must match URI used in ReadResource calls.
	URI string `json:"uri"`
}

// UnsubscribeResourceParams contains parameters for unsubscribing from a resource.
type UnsubscribeResourceParams struct {
	// URI is the unique identifier of the resource to unsubscribe from.
	// Must match URI used in ReadResource calls.
	URI string `json:"uri"`
}

// ListResourcesResult represents a paginated list of resources returned by ListResources.
// NextCursor can be used to retrieve the next page of results.
type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// ReadResourceResult represents the result of a read resource request.
type ReadResourceResult struct {
	Contents []Resource `json:"contents"`
}

// Resource represents a content resource in the system with associated metadata.
// The content can be provided either as Text or Blob, with MimeType indicating the format.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	Text        string `json:"text,omitempty"`
	Blob        string `json:"blob,omitempty"`
}

// ListResourceTemplatesResult represents the result of a list resource templates request.
type ListResourceTemplatesResult struct {
	Templates []ResourceTemplate `json:"resourceTemplates"`
}

// ResourceTemplate defines a template for generating resource URIs.
// It's returned by ListResourceTemplates and used with CompletesResourceTemplate.
type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ListToolsParams contains parameters for listing available tools.
type ListToolsParams struct {
	// Cursor is a pagination cursor from previous ListTools call.
	// Empty string requests the first page.
	Cursor string `json:"cursor"`

	// Meta contains optional metadata including:
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// CallToolParams contains parameters for executing a specific tool.
type CallToolParams struct {
	// Name is the unique identifier of the tool to execute
	Name string `json:"name"`

	// Arguments is a map of argument name-value pairs
	// Must satisfy required arguments defined in tool's InputSchema field
	Arguments map[string]any `json:"arguments"`

	// Meta contains optional metadata including:
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ListToolsResult represents a paginated list of tools returned by ListTools.
// NextCursor can be used to retrieve the next page of results.
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// Tool defines a callable tool with its input schema.
// InputSchema defines the expected format of arguments for CallTool.
type Tool struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	InputSchema *jsonschema.Schema `json:"inputSchema,omitempty"`
}

// CallToolResult represents the outcome of a tool invocation via CallTool.
// IsError indicates whether the operation failed, with details in Content.
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError"`
}

// CompletesCompletionParams contains parameters for requesting completion suggestions.
// It includes a reference to what is being completed (e.g. a prompt or resource template)
// and the specific argument that needs completion suggestions.
type CompletesCompletionParams struct {
	// Ref identifies what is being completed (e.g. prompt, resource template)
	Ref CompletionRef `json:"ref"`
	// Argument specifies which argument needs completion suggestions
	Argument CompletionArgument `json:"argument"`
}

// CompletionRef identifies what is being completed in a completion request.
// Type must be one of:
//   - "ref/prompt": Completing a prompt argument, Name field must be set to prompt name
//   - "ref/resource": Completing a resource template argument, URI field must be set to template URI
type CompletionRef struct {
	// Type specifies what kind of completion is being requested.
	// Must be either "ref/prompt" or "ref/resource".
	Type string `json:"type"`
	// Name contains the prompt name when Type is "ref/prompt".
	Name string `json:"name,omitempty"`
	// URI contains the resource template URI when Type is "ref/resource".
	URI string `json:"uri,omitempty"`
}

// CompletionArgument defines the structure for arguments passed in completion requests,
// containing the argument name and its corresponding value.
type CompletionArgument struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CompletionResult contains the response data for a completion request, including
// possible completion values and whether more completions are available.
type CompletionResult struct {
	Completion struct {
		Values  []string `json:"values"`
		HasMore bool     `json:"hasMore"`
	} `json:"completion"`
}

// ProgressParams represents the progress status of a long-running operation.
type ProgressParams struct {
	// ProgressToken uniquely identifies the operation this progress update relates to
	ProgressToken MustString `json:"progressToken"`
	// Progress represents the current progress value
	Progress float64 `json:"progress"`
	// Total represents the expected final value when known.
	// When non-zero, completion percentage can be calculated as (Progress/Total)*100
	Total float64 `json:"total"`
}

// LogParams represents the parameters for a log message.
type LogParams struct {
	// Level indicates the severity level of the message.
	// Must be one of the defined LogLevel constants.
	Level LogLevel `json:"level"`
	// Logger identifies the source/component that generated the message.
	Logger string `json:"logger"`
	// Data contains the message content and any structured metadata.
	Data LogData `json:"data"`
}

// LogData represents the data of a log message.
type LogData struct {
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// LogLevel represents the severity level of log messages.
type LogLevel int

// ParamsMeta contains optional metadata that can be included with request parameters.
// It is used to enable features like progress tracking for long-running operations.
type ParamsMeta struct {
	// ProgressToken uniquely identifies an operation for progress tracking.
	// When provided, the server can emit progress updates via ProgressReporter.
	ProgressToken MustString `json:"progressToken"`
}

// RootList represents a collection of root resources in the system.
type RootList struct {
	Roots []Root `json:"roots"`
}

// Root represents a top-level resource entry point in the system.
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// SamplingParams defines the parameters for generating a sampled message.
//
// The params are used by SamplingHandler.CreateSampleMessage to generate appropriate
// AI model responses while respecting the specified constraints and preferences.
type SamplingParams struct {
	// Messages contains the conversation history as a sequence of user and assistant messages
	Messages []SamplingMessage `json:"messages"`

	// ModelPreferences controls model selection through cost, speed, and intelligence priorities
	ModelPreferences SamplingModelPreferences `json:"modelPreferences"`

	// SystemPrompts provides system-level instructions to guide the model's behavior
	SystemPrompts string `json:"systemPrompts"`

	// MaxTokens specifies the maximum number of tokens allowed in the generated response
	MaxTokens int `json:"maxTokens"`
}

// SamplingMessage represents a message in the sampling conversation history. Contains
// a role indicating the message sender (user or assistant) and the content of the
// message with its type and data.
type SamplingMessage struct {
	Role    PromptRole      `json:"role"`
	Content SamplingContent `json:"content"`
}

// SamplingContent represents the content of a sampling message. Contains the content
// type identifier, plain text content for text messages, or binary data with MIME
// type for non-text content. Either Text or Data should be populated based on the
// content Type.
type SamplingContent struct {
	Type ContentType `json:"type"`

	Text string `json:"text"`

	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

// SamplingModelPreferences defines preferences for model selection and behavior. Contains
// hints to guide model selection, and priority values for different aspects (cost,
// speed, intelligence) that influence the sampling process and model choice.
type SamplingModelPreferences struct {
	Hints []struct {
		Name string `json:"name"`
	} `json:"hints"`
	CostPriority         int `json:"costPriority"`
	SpeedPriority        int `json:"speedPriority"`
	IntelligencePriority int `json:"intelligencePriority"`
}

// SamplingResult represents the output of a sampling operation. Contains the role of
// the generated message, its content, the name of the model that generated it, and
// the reason why generation stopped (e.g., max tokens reached, natural completion).
type SamplingResult struct {
	Role       PromptRole      `json:"role"`
	Content    SamplingContent `json:"content"`
	Model      string          `json:"model"`
	StopReason string          `json:"stopReason"`
}

// Content represents a message content with its type.
type Content struct {
	Type ContentType `json:"type"`

	Text string `json:"text,omitempty"`

	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`

	Resource *Resource `json:"resource,omitempty"`
}

// ContentType represents the type of content in messages.
type ContentType string

// ProgressReporter is a function type used to report progress updates for long-running operations.
// Server implementations use this callback to inform clients about operation progress by passing
// a ProgressParams struct containing the progress details. When Total is non-zero in the params,
// progress percentage can be calculated as (Progress/Total)*100.
type ProgressReporter func(progress ProgressParams)

// RequestClientFunc is a function type that handles JSON-RPC message communication between client and server.
// It takes a JSON-RPC request message as input and returns the corresponding response message.
//
// The function is used by server implementations to send requests to clients and receive responses
// during method handling. For example, when a server needs to request additional information from
// a client while processing a method call.
//
// It should respect the JSON-RPC 2.0 specification for error handling and message formatting.
type RequestClientFunc func(msg JSONRPCMessage) (JSONRPCMessage, error)

// ServerCapabilities represents server capabilities.
type ServerCapabilities struct {
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Logging   *LoggingCapability   `json:"logging,omitempty"`
}

// ClientCapabilities represents client capabilities.
type ClientCapabilities struct {
	Roots    *RootsCapability    `json:"roots,omitempty"`
	Sampling *SamplingCapability `json:"sampling,omitempty"`
}

// PromptsCapability represents prompts-specific capabilities.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability represents resources-specific capabilities.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// ToolsCapability represents tools-specific capabilities.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// LoggingCapability represents logging-specific capabilities.
type LoggingCapability struct{}

// RootsCapability represents roots-specific capabilities.
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SamplingCapability represents sampling-specific capabilities.
type SamplingCapability struct{}

type initializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Info               `json:"clientInfo"`
}

type initializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Info               `json:"serverInfo"`
}

type notificationsCancelledParams struct {
	RequestID string `json:"requestId"`
	Reason    string `json:"reason"`
}

type notificationsResourcesUpdatedParams struct {
	URI string `json:"uri"`
}

type waitForResultReq struct {
	msgID   string
	resChan chan<- chan JSONRPCMessage
}

type cancelRequest struct {
	msgID   string
	ctxChan chan<- context.Context
}

type sessionMsg struct {
	sessionID string
	msg       JSONRPCMessage
}

const (
	// JSONRPCVersion specifies the JSON-RPC protocol version used for communication.
	JSONRPCVersion = "2.0"

	// MethodPromptsList is the method name for retrieving a list of available prompts.
	MethodPromptsList = "prompts/list"
	// MethodPromptsGet is the method name for retrieving a specific prompt by identifier.
	MethodPromptsGet = "prompts/get"

	// MethodResourcesList is the method name for listing available resources.
	MethodResourcesList = "resources/list"
	// MethodResourcesRead is the method name for reading the content of a specific resource.
	MethodResourcesRead = "resources/read"
	// MethodResourcesTemplatesList is the method name for listing available resource templates.
	MethodResourcesTemplatesList = "resources/templates/list"
	// MethodResourcesSubscribe is the method name for subscribing to resource updates.
	MethodResourcesSubscribe = "resources/subscribe"
	// MethodResourcesUnsubscribe is the method name for unsubscribing from resource updates.
	MethodResourcesUnsubscribe = "resources/unsubscribe"

	// MethodToolsList is the method name for retrieving a list of available tools.
	MethodToolsList = "tools/list"
	// MethodToolsCall is the method name for invoking a specific tool.
	MethodToolsCall = "tools/call"

	// MethodRootsList is the method name for retrieving a list of root resources.
	MethodRootsList = "roots/list"
	// MethodSamplingCreateMessage is the method name for creating a new sampling message.
	MethodSamplingCreateMessage = "sampling/createMessage"

	// MethodCompletionComplete is the method name for requesting completion suggestions.
	MethodCompletionComplete = "completion/complete"

	// MethodLoggingSetLevel is the method name for setting the minimum severity level for emitted log messages.
	MethodLoggingSetLevel = "logging/setLevel"

	// CompletionRefPrompt is used in CompletionRef.Type for prompt argument completion.
	CompletionRefPrompt = "ref/prompt"
	// CompletionRefResource is used in CompletionRef.Type for resource template argument completion.
	CompletionRefResource = "ref/resource"

	protocolVersion = "2024-11-05"

	errMsgInvalidJSON                    = "Invalid json"
	errMsgUnsupportedProtocolVersion     = "Unsupported protocol version"
	errMsgInsufficientClientCapabilities = "Insufficient client capabilities"
	errMsgInternalError                  = "Internal error"
	errMsgWriteTimeout                   = "Write timeout"
	errMsgReadTimeout                    = "Read timeout"

	methodPing       = "ping"
	methodInitialize = "initialize"

	methodNotificationsInitialized          = "notifications/initialized"
	methodNotificationsCancelled            = "notifications/cancelled"
	methodNotificationsPromptsListChanged   = "notifications/prompts/list_changed"
	methodNotificationsResourcesListChanged = "notifications/resources/list_changed"
	methodNotificationsResourcesUpdated     = "notifications/resources/updated"
	methodNotificationsToolsListChanged     = "notifications/tools/list_changed"
	methodNotificationsProgress             = "notifications/progress"
	methodNotificationsMessage              = "notifications/message"

	methodNotificationsRootsListChanged = "notifications/roots/list_changed"

	userCancelledReason = "User requested cancellation"

	jsonRPCParseErrorCode     = -32700
	jsonRPCInvalidRequestCode = -32600
	jsonRPCMethodNotFoundCode = -32601
	jsonRPCInvalidParamsCode  = -32602
	jsonRPCInternalErrorCode  = -32603
)

// PromptRole represents the role in a conversation (user or assistant).
const (
	PromptRoleUser      PromptRole = "user"
	PromptRoleAssistant PromptRole = "assistant"
)

// LogLevel represents the severity level of log messages.
const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelNotice
	LogLevelWarning
	LogLevelError
	LogLevelCritical
	LogLevelAlert
	LogLevelEmergency
)

// ContentType represents the type of content in messages.
const (
	ContentTypeText     ContentType = "text"
	ContentTypeImage    ContentType = "image"
	ContentTypeResource ContentType = "resource"
)

// UnmarshalJSON implements json.Unmarshaler to convert JSON data into MustString,
// handling both string and numeric input formats.
func (m *MustString) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	switch v := v.(type) {
	case string:
		*m = MustString(v)
	case float64:
		*m = MustString(fmt.Sprintf("%d", int(v)))
	case int:
		*m = MustString(fmt.Sprintf("%d", v))
	default:
		return fmt.Errorf("invalid type: %T", v)
	}

	return nil
}

// MarshalJSON implements json.Marshaler to convert MustString into its JSON representation,
// always encoding as a string value.
func (m MustString) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(m))
}

func (j JSONRPCError) Error() string {
	return fmt.Sprintf("request error, code: %d, message: %s, data %v", j.Code, j.Message, j.Data)
}
