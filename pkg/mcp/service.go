package mcp

import "context"

// Server interfaces

// PromptServer defines the interface for managing prompts in the MCP protocol.
// It provides functionality for listing available prompts, retrieving specific prompts
// with arguments, and providing prompt argument completions.
type PromptServer interface {
	// ListPrompts returns a paginated list of available prompts.
	//
	// Parameters:
	// - ctx: Context for cancellation and client-server communication
	//   * Implementation must respect context cancellation and return early if cancelled
	// - cursor: Optional pagination cursor from previous ListPrompts call
	//   * Empty string requests the first page
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	//
	// Returns:
	// - PromptList containing:
	//   * Available prompts
	//   * Next page cursor if more results exist
	// - error if:
	//   * Operation fails
	//   * Context is cancelled
	ListPrompts(ctx context.Context, cursor string, progressToken MustString) (PromptList, error)

	// GetPrompt retrieves a specific prompt template by name with the given arguments.
	//
	// Parameters:
	// - ctx: Context for cancellation and client-server communication
	//   * Implementation must respect context cancellation and return early if cancelled
	// - name: Unique identifier of the prompt to retrieve
	// - args: Map of argument name-value pairs
	//   * Must satisfy required arguments defined in prompt's Arguments field
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	//
	// Returns:
	// - Prompt containing:
	//   * Template content
	//   * Associated metadata
	// - error if:
	//   * Prompt not found
	//   * Arguments are invalid
	//   * Context is cancelled
	GetPrompt(ctx context.Context, name string, args map[string]any, progressToken MustString) (Prompt, error)

	// CompletesPrompt provides completion suggestions for a prompt argument.
	// Used to implement interactive argument completion in clients.
	//
	// Parameters:
	// - ctx: Context for cancellation and client-server communication
	//   * Implementation must respect context cancellation and return early if cancelled
	// - name: Unique identifier of the prompt to get completions for
	// - arg: Argument name and partial value to get completions for
	//
	// Returns:
	// - CompletionResult containing:
	//   * Possible completion values
	//   * HasMore flag indicating if additional values exist
	// - error if:
	//   * Prompt doesn't exist
	//   * Completions cannot be generated
	//   * Context is cancelled
	CompletesPrompt(ctx context.Context, name string, arg CompletionArgument) (CompletionResult, error)
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
// It provides functionality for listing, reading, and subscribing to resources,
// as well as managing resource templates.
type ResourceServer interface {
	// ListResources returns a paginated list of available resources.
	//
	// Parameters:
	// - ctx: Context for cancellation and client-server communication
	//   * Implementation must respect context cancellation and return early if cancelled
	// - cursor: Pagination cursor from previous ListResources call
	//   * Empty string requests the first page
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	//
	// Returns:
	// - ResourceList containing:
	//   * Available resources
	//   * Next page cursor if more results exist
	// - error if:
	//   * Operation fails
	//   * Context is cancelled
	ListResources(ctx context.Context, cursor string, progressToken MustString) (*ResourceList, error)

	// ReadResource retrieves a specific resource by its URI.
	//
	// Parameters:
	// - ctx: Context for cancellation and client-server communication
	//   * Implementation must respect context cancellation and return early if cancelled
	// - uri: Unique identifier of the resource to retrieve
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	//
	// Returns:
	// - Resource containing:
	//   * Resource content
	//   * Associated metadata
	// - error if:
	//   * Resource not found
	//   * Resource cannot be read
	//   * Context is cancelled
	ReadResource(ctx context.Context, uri string, progressToken MustString) (*Resource, error)

	// ListResourceTemplates returns all available resource templates.
	//
	// Parameters:
	// - ctx: Context for cancellation and client-server communication
	//   * Implementation must respect context cancellation and return early if cancelled
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	//
	// Returns:
	// - []ResourceTemplate containing:
	//   * Available templates with URI patterns
	//   * Template metadata
	// - error if:
	//   * Templates cannot be retrieved
	//   * Context is cancelled
	ListResourceTemplates(ctx context.Context, progressToken MustString) ([]ResourceTemplate, error)

	// CompletesResourceTemplate provides completion suggestions for a resource template argument.
	//
	// Parameters:
	// - ctx: Context for cancellation and client-server communication
	//   * Implementation must respect context cancellation and return early if cancelled
	// - name: Unique identifier of the template to get completions for
	// - arg: Argument name and partial value to get completions for
	//
	// Returns:
	// - CompletionResult containing:
	//   * Possible completion values
	//   * HasMore flag indicating if additional values exist
	// - error if:
	//   * Template doesn't exist
	//   * Completions cannot be generated
	//   * Context is cancelled
	CompletesResourceTemplate(ctx context.Context, name string, arg CompletionArgument) (CompletionResult, error)

	// SubscribeResource registers interest in a specific resource URI.
	//
	// Parameters:
	// - uri: Unique identifier of the resource to subscribe to
	//   * Must match URI used in ReadResource calls
	SubscribeResource(uri string)
}

// ResourceListUpdater provides an interface for monitoring changes to the available resources list.
// It maintains a channel that emits notifications whenever resources are added, removed, or modified.
type ResourceListUpdater interface {
	// ResourceListUpdates returns a channel that emits notifications when the resource list changes.
	//
	// Returns:
	// - Channel with the following characteristics:
	//   * Remains open for the lifetime of the updater
	//   * Safe for concurrent receives from multiple goroutines
	//   * Never blocks on sends using buffered channels or dropped notifications
	//   * Emits struct{} as only the notification matters, not the value
	ResourceListUpdates() <-chan struct{}
}

// ResourceSubscribedUpdater provides an interface for monitoring changes to subscribed resources.
// It maintains a channel that emits notifications whenever a subscribed resource changes.
type ResourceSubscribedUpdater interface {
	// ResourceSubscriberUpdates returns a channel that emits notifications when subscribed resources change.
	//
	// Returns:
	// - Channel with the following characteristics:
	//   * Remains open for the lifetime of the updater
	//   * Safe for concurrent receives from multiple goroutines
	//   * Never blocks on sends using buffered channels or dropped notifications
	//   * Emits the URI of the resource that changed
	ResourceSubscriberUpdates() <-chan string
}

// ToolServer defines the interface for managing tools in the MCP protocol.
// It provides functionality for listing available tools and executing tool operations.
type ToolServer interface {
	// ListTools returns a paginated list of available tools.
	//
	// Parameters:
	// - ctx: Context for cancellation and client-server communication
	//   * Implementation must respect context cancellation and return early if cancelled
	// - cursor: Pagination cursor from previous ListTools call
	//   * Empty string requests the first page
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	//
	// Returns:
	// - ToolList containing:
	//   * Available tools with name, description and input schema
	//   * Next page cursor if more results exist
	// - error if:
	//   * Operation fails
	//   * Context is cancelled
	ListTools(ctx context.Context, cursor string, progressToken MustString) (*ToolList, error)

	// CallTool executes a specific tool with the given arguments.
	//
	// Parameters:
	// - ctx: Context for cancellation and client-server communication
	//   * Implementation must respect context cancellation and return early if cancelled
	// - name: Unique identifier of the tool to execute
	// - args: Map of argument name-value pairs
	//   * Must satisfy required arguments defined in tool's InputSchema field
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	//
	// Returns:
	// - ToolResult containing:
	//   * Tool execution output
	//   * Error status indicating execution success/failure
	// - error if:
	//   * Tool not found
	//   * Arguments are invalid
	//   * Execution fails
	//   * Context is cancelled
	CallTool(ctx context.Context, name string, args map[string]any, progressToken MustString) (ToolResult, error)
}

// ToolListUpdater provides an interface for monitoring changes to the available tools list.
// It maintains a channel that emits notifications whenever tools are added, removed, or modified.
type ToolListUpdater interface {
	// WatchToolList returns a channel that emits notifications when the tool list changes.
	//
	// Returns:
	// - Channel with the following characteristics:
	//   * Remains open for the lifetime of the updater
	//   * Safe for concurrent receives from multiple goroutines
	//   * Never blocks on sends using buffered channels or dropped notifications
	//   * Emits struct{} as only the notification matters, not the value
	WatchToolList() <-chan struct{}
}

// ProgressReporter provides an interface for reporting progress updates on long-running operations.
// It maintains a channel that emits progress updates for operations identified by progress tokens.
type ProgressReporter interface {
	// ProgressReports returns a channel that emits progress updates for operations.
	//
	// Returns:
	// - Channel with the following characteristics:
	//   * Remains open for the lifetime of the reporter
	//   * Safe for concurrent receives from multiple goroutines
	//   * Never blocks on sends using buffered channels or dropped notifications
	//   * Emits ProgressParams containing:
	//     - Progress token matching the original operation
	//     - Current progress value
	//     - Total expected value when known
	//     - Progress percentage calculable as (progress/total)*100 when total is non-zero
	ProgressReports() <-chan ProgressParams
}

// LogHandler provides an interface for streaming log messages from the MCP server to connected clients.
// It maintains a channel for emitting log messages and allows configuration of minimum severity level.
type LogHandler interface {
	// LogStreams returns a channel that emits log messages with metadata.
	//
	// Returns:
	// - Channel with the following characteristics:
	//   * Remains open for the lifetime of the handler
	//   * Safe for concurrent receives from multiple goroutines
	//   * Never blocks on sends using buffered channels or dropped messages
	//   * Emits LogParams containing:
	//     - Message severity level
	//     - Logger name/identifier
	//     - Message content and structured data
	LogStreams() <-chan LogParams

	// SetLogLevel configures the minimum severity level for emitted log messages.
	//
	// Parameters:
	// - level: Minimum severity level for messages
	//   * Must be one of the defined LogLevel constants
	//   * Messages below this level are filtered out
	//   * Valid levels: Debug, Info, Notice, Warning, Error, Critical, Alert, Emergency
	SetLogLevel(level LogLevel)
}

// RootsListWatcher provides an interface for receiving notifications when the client's root list changes.
// The implementation can use these notifications to update its internal state or perform necessary actions
// when the client's available roots change.
type RootsListWatcher interface {
	// OnRootsListChanged is called when the client notifies that its root list has changed
	OnRootsListChanged()
}

// RootsListHandler defines the interface for retrieving the list of root resources in the MCP protocol.
// Root resources represent top-level entry points in the resource hierarchy that clients can access.
type RootsListHandler interface {
	// RootsList returns the list of available root resources.
	//
	// Parameters:
	// - ctx: Context for cancellation and client-server communication
	//   * Implementation must respect context cancellation and return early if cancelled
	//
	// Returns:
	// - RootList containing:
	//   * URI: A unique identifier for accessing the root resource
	//   * Name: A human-readable name for the root
	// - error if:
	//   * Operation fails
	//   * Context is cancelled
	RootsList(ctx context.Context) (RootList, error)
}

// RootsListUpdater provides an interface for monitoring changes to the available roots list.
// Implementations should maintain a channel that emits notifications whenever the list of
// available roots changes, such as when roots are added, removed, or modified.
type RootsListUpdater interface {
	// RootsListUpdates returns a channel that emits notifications when the root list changes.
	//
	// Returns:
	// - Channel with the following characteristics:
	//   * Remains open for the lifetime of the updater
	//   * Safe for concurrent receives from multiple goroutines
	//   * Never blocks on sends using buffered channels or dropped notifications
	//   * Emits struct{} as only the notification matters, not the value
	RootsListUpdates() <-chan struct{}
}

// SamplingHandler provides an interface for generating AI model responses based on conversation history.
// It handles the core sampling functionality including managing conversation context, applying model preferences,
// and generating appropriate responses while respecting token limits.
type SamplingHandler interface {
	// CreateSampleMessage generates a response message based on the provided conversation history and parameters.
	//
	// Parameters:
	// - ctx: Context for cancellation and client-server communication
	//   * Implementation must respect context cancellation and return early if cancelled
	// - params: The parameters that control the sampling process, including:
	//   * Messages: The conversation history as a sequence of user and assistant messages
	//   * ModelPreferences: Preferences for model selection (cost, speed, intelligence priorities)
	//   * SystemPrompts: System-level prompts that guide the model's behavior
	//   * MaxTokens: Maximum number of tokens allowed in the generated response
	//
	// Returns:
	// - SamplingResult containing:
	//   * Role: The role of the generated message (typically "assistant")
	//   * Content: The generated content with its type and data
	//   * Model: The name/identifier of the model that generated the response
	//   * StopReason: Why the generation stopped
	// - error if:
	//   * Model selection fails
	//   * Generation fails
	//   * Token limit is exceeded
	//   * Context is cancelled
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
	//
	// Parameters:
	// - params: Progress parameters containing:
	//   * progressToken: Unique identifier correlating the progress update to a specific operation
	//   * progress: Current progress value
	//   * total: Total expected value (when known)
	OnProgress(params ProgressParams)
}

// LogReceiver provides an interface for receiving log messages from the server.
// Implementations can use these notifications to display logs in a UI, write them to a file,
// or forward them to a logging service.
type LogReceiver interface {
	// OnLog is called when a log message is received from the server.
	//
	// Parameters:
	// - params: Log parameters containing:
	//   * Level: The severity level of the message
	//   * Logger: The name/identifier of the logger
	//   * Data: The message content and structured data
	OnLog(params LogParams)
}

// PromptList represents a paginated list of prompts returned by ListPrompts.
// NextCursor can be used to retrieve the next page of results.
type PromptList struct {
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

// PromptResult represents the result of a prompt request.
type PromptResult struct {
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

// ResourceList represents a paginated list of resources returned by ListResources.
// NextCursor can be used to retrieve the next page of results.
type ResourceList struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
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

// ResourceTemplate defines a template for generating resource URIs.
// It's returned by ListResourceTemplates and used with CompletesResourceTemplate.
type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ToolList represents a paginated list of tools returned by ListTools.
// NextCursor can be used to retrieve the next page of results.
type ToolList struct {
	Tools      []*Tool `json:"tools"`
	NextCursor string  `json:"nextCursor,omitempty"`
}

// Tool defines a callable tool with its input schema.
// InputSchema defines the expected format of arguments for CallTool.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// ToolResult represents the outcome of a tool invocation via CallTool.
// IsError indicates whether the operation failed, with details in Content.
type ToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError"`
}

// ProgressParams represents the progress status of a long-running operation.
// Progress and Total values indicate completion percentage when Total is non-zero.
type ProgressParams struct {
	ProgressToken string  `json:"progressToken"`
	Progress      float64 `json:"value"`
	Total         float64 `json:"total"`
}

// LogParams represents the parameters for a log message.
type LogParams struct {
	Level  LogLevel `json:"level"`
	Logger string   `json:"logger"`
	Data   LogData  `json:"data"`
}

// LogData represents the data of a log message.
type LogData struct {
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// LogLevel represents the severity level of log messages.
type LogLevel string

// RootList represents a collection of root resources in the system.
type RootList struct {
	Roots []Root `json:"roots"`
}

// Root represents a top-level resource entry point in the system.
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// SamplingParams defines the parameters for generating a sampled message. Contains
// the conversation history as messages, model preferences for sampling behavior,
// system-level prompts to guide the model, and a maximum token limit for the
// generated response.
type SamplingParams struct {
	Messages         []SamplingMessage        `json:"messages"`
	ModelPreferences SamplingModelPreferences `json:"modelPreferences"`
	SystemPrompts    string                   `json:"systemPrompts"`
	MaxTokens        int                      `json:"maxTokens"`
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

type promptsListParams struct {
	Cursor string     `json:"cursor"`
	Meta   paramsMeta `json:"_meta,omitempty"`
}

type promptsGetParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Meta      paramsMeta     `json:"_meta,omitempty"`
}

type resourcesListParams struct {
	Cursor string     `json:"cursor"`
	Meta   paramsMeta `json:"_meta,omitempty"`
}

type resourcesReadParams struct {
	URI  string     `json:"uri"`
	Meta paramsMeta `json:"_meta,omitempty"`
}

type resourcesTemplatesListParams struct {
	Meta paramsMeta `json:"_meta,omitempty"`
}

type resourcesSubscribeParams struct {
	URI string `json:"uri"`
}

type toolsListParams struct {
	Cursor string     `json:"cursor"`
	Meta   paramsMeta `json:"_meta,omitempty"`
}

type toolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Meta      paramsMeta     `json:"_meta,omitempty"`
}

// PromptRole represents the role in a conversation (user or assistant).
const (
	PromptRoleUser      PromptRole = "user"
	PromptRoleAssistant PromptRole = "assistant"
)

// LogLevel represents the severity level of log messages.
const (
	LogLevelDebug     LogLevel = "debug"
	LogLevelInfo      LogLevel = "info"
	LogLevelNotice    LogLevel = "notice"
	LogLevelWarning   LogLevel = "warning"
	LogLevelError     LogLevel = "error"
	LogLevelCritical  LogLevel = "critical"
	LogLevelAlert     LogLevel = "alert"
	LogLevelEmergency LogLevel = "emergency"
)
