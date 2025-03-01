package mcp

import (
	"context"
	"iter"
)

// ServerTransport provides the server-side communication layer in the MCP protocol.
type ServerTransport interface {
	// Sessions returns an iterator that yields new client sessions as they are initiated.
	// Each yielded Session represents a unique client connection and provides methods for
	// bidirectional communication. The implementation must guarantee that each session ID
	// is unique across all active connections.
	//
	// The implementation should exit the iteration when the Shutdown method is called.
	Sessions() iter.Seq[Session]

	// Shutdown gracefully shuts down the ServerTransport to clean up resources. The implementations should not
	// close all the Session it produce, the caller would already do that when callling this method. The caller
	// is guaranteed to call this method only once.
	Shutdown(ctx context.Context) error
}

// ClientTransport provides the client-side communication layer in the MCP protocol.
type ClientTransport interface {
	// StartSession initiates a new session with the server and returns an iterator that yields server messages.
	// The transport signals its readiness to send messages through the ready channel by either closing it or
	// feeding an error. Operations are canceled when the context is canceled, and appropriate errors are returned
	// for connection or protocol failures. Refer to the returned iterator's documentation for details on message
	// handling.
	StartSession(ctx context.Context) (Session, error)
}

// Session represents a bidirectional communication channel between server and client.
type Session interface {
	// ID returns the unique identifier for this session. The implementation must
	// guarantee that session IDs are unique across all active sessions managed.
	ID() string

	// Send transmits a message to the client.
	Send(ctx context.Context, msg JSONRPCMessage) error

	// Messages returns an iterator that yields messages received from the other party.
	// The implementations should exit the iteration if the session is closed.
	Messages() iter.Seq[JSONRPCMessage]

	// Stop stops the session.
	// The implementation should not called this, as the caller is guaranteed to call
	// this method once.
	Stop()
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
//
// The notifications are used by the MCP server to inform connected clients about prompt list
// changes via the "notifications/prompts/list_changed" method. Clients can then refresh their
// cached prompt lists by calling ListPrompts again.
//
// A struct{} is sent through the iterator as only the notification matters, not the value.
type PromptListUpdater interface {
	PromptListUpdates() iter.Seq[struct{}]
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
}

// ResourceListUpdater provides an interface for monitoring changes to the available resources list.
//
// The notifications are used by the MCP server to inform connected clients about resource list
// changes. Clients can then refresh their cached resource lists by calling ListResources again.
//
// A struct{} is sent through the channel as only the notification matters, not the value.
type ResourceListUpdater interface {
	ResourceListUpdates() iter.Seq[struct{}]
}

// ResourceSubscriptionHandler defines the interface for handling subscription for resources.
type ResourceSubscriptionHandler interface {
	// SubscribeResource subscribes to a resource.
	SubscribeResource(SubscribeResourceParams)
	// UnsubscribeResource unsubscribes from a resource.
	UnsubscribeResource(UnsubscribeResourceParams)
	// SubscribedResourceUpdates returns an iterator that emits notifications whenever a subscribed resource changes.
	SubscribedResourceUpdates() iter.Seq[string]
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
//
// The notifications are used by the MCP server to inform connected clients about tool list
// changes. Clients can then refresh their cached tool lists by calling ListTools again.
//
// A struct{} is sent through the channel as only the notification matters, not the value.
type ToolListUpdater interface {
	ToolListUpdates() iter.Seq[struct{}]
}

// LogHandler provides an interface for streaming log messages from the MCP server to connected clients.
type LogHandler interface {
	// LogStreams returns an iterator that emits log messages with metadata.
	LogStreams() iter.Seq[LogParams]

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
	// RootsListUpdates returns an iterator that emits notifications when the root list changes.
	RootsListUpdates() iter.Seq[struct{}]
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
	Role    Role            `json:"role"`
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
	Role       Role            `json:"role"`
	Content    SamplingContent `json:"content"`
	Model      string          `json:"model"`
	StopReason string          `json:"stopReason"`
}

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
