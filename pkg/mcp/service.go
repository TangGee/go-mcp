package mcp

import (
	"context"

	"github.com/qri-io/jsonschema"
)

// Server interfaces

// PromptServer defines the interface for managing prompts in the MCP protocol.
// It provides functionality for listing available prompts, retrieving specific prompts
// with arguments, and providing prompt argument completions.
type PromptServer interface {
	// ListPrompts returns a paginated list of available prompts.
	// Returns error if operation fails or context is cancelled.
	ListPrompts(ctx context.Context, params PromptsListParams) (PromptList, error)

	// GetPrompt retrieves a specific prompt template by name with the given arguments.
	// Returns error if prompt not found, arguments are invalid, or context is cancelled.
	GetPrompt(ctx context.Context, params PromptsGetParams) (PromptResult, error)

	// CompletesPrompt provides completion suggestions for a prompt argument.
	// Used to implement interactive argument completion in clients.
	// Returns error if prompt doesn't exist, completions cannot be generated, or context is cancelled.
	CompletesPrompt(ctx context.Context, params CompletionCompleteParams) (CompletionResult, error)
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
	// Returns error if operation fails or context is cancelled.
	ListResources(ctx context.Context, params ResourcesListParams) (*ResourceList, error)

	// ReadResource retrieves a specific resource by its URI.
	// Returns error if resource not found, cannot be read, or context is cancelled.
	ReadResource(ctx context.Context, params ResourcesReadParams) (*Resource, error)

	// ListResourceTemplates returns all available resource templates.
	// Returns error if templates cannot be retrieved or context is cancelled.
	ListResourceTemplates(ctx context.Context, params ResourcesTemplatesListParams) ([]ResourceTemplate, error)

	// CompletesResourceTemplate provides completion suggestions for a resource template argument.
	// Returns error if template doesn't exist, completions cannot be generated, or context is cancelled.
	CompletesResourceTemplate(ctx context.Context, params CompletionCompleteParams) (CompletionResult, error)

	// SubscribeResource registers interest in a specific resource URI.
	SubscribeResource(params ResourcesSubscribeParams)
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
// It provides functionality for listing available tools and executing tool operations.
type ToolServer interface {
	// ListTools returns a paginated list of available tools.
	// Returns error if operation fails or context is cancelled.
	ListTools(ctx context.Context, params ToolsListParams) (ToolList, error)

	// CallTool executes a specific tool with the given arguments.
	// Returns error if tool not found, arguments are invalid, execution fails, or context is cancelled.
	CallTool(ctx context.Context, params ToolsCallParams) (ToolResult, error)
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

// ProgressReporter provides an interface for reporting progress updates on long-running operations.
// It maintains a channel that emits progress updates for operations identified by progress tokens.
type ProgressReporter interface {
	// ProgressReports returns a channel that emits progress updates for operations.
	// The channel remains open for the lifetime of the reporter and is safe for concurrent receives.
	ProgressReports() <-chan ProgressParams
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

// PromptsListParams contains parameters for listing available prompts.
type PromptsListParams struct {
	// Cursor is an optional pagination cursor from previous ListPrompts call.
	// Empty string requests the first page.
	Cursor string `json:"cursor"`

	// Meta contains optional metadata including:
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// PromptsGetParams contains parameters for retrieving a specific prompt.
type PromptsGetParams struct {
	// Name is the unique identifier of the prompt to retrieve
	Name string `json:"name"`

	// Arguments is a map of argument name-value pairs
	// Must satisfy required arguments defined in prompt's Arguments field
	Arguments map[string]any `json:"arguments"`

	// Meta contains optional metadata including:
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	Meta ParamsMeta `json:"_meta,omitempty"`
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

// ResourcesListParams contains parameters for listing available resources.
type ResourcesListParams struct {
	// Cursor is a pagination cursor from previous ListResources call.
	// Empty string requests the first page.
	Cursor string `json:"cursor"`

	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ResourcesReadParams contains parameters for retrieving a specific resource.
type ResourcesReadParams struct {
	// URI is the unique identifier of the resource to retrieve.
	URI string `json:"uri"`

	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ResourcesTemplatesListParams contains parameters for listing available resource templates.
type ResourcesTemplatesListParams struct {
	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ResourcesSubscribeParams contains parameters for subscribing to a resource.
type ResourcesSubscribeParams struct {
	// URI is the unique identifier of the resource to subscribe to.
	// Must match URI used in ReadResource calls.
	URI string `json:"uri"`
}

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

// ToolsListParams contains parameters for listing available tools.
type ToolsListParams struct {
	// Cursor is a pagination cursor from previous ListTools call.
	// Empty string requests the first page.
	Cursor string `json:"cursor"`

	// Meta contains optional metadata including:
	// - progressToken: Unique token for tracking operation progress
	//   * Used by ProgressReporter to emit progress updates if supported
	//   * Optional - may be ignored if progress tracking not supported
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ToolsCallParams contains parameters for executing a specific tool.
type ToolsCallParams struct {
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

// ToolList represents a paginated list of tools returned by ListTools.
// NextCursor can be used to retrieve the next page of results.
type ToolList struct {
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

// ToolResult represents the outcome of a tool invocation via CallTool.
// IsError indicates whether the operation failed, with details in Content.
type ToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError"`
}

// ProgressParams represents the progress status of a long-running operation.
type ProgressParams struct {
	// ProgressToken uniquely identifies the operation this progress update relates to
	ProgressToken string `json:"progressToken"`
	// Progress represents the current progress value
	Progress float64 `json:"value"`
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
type LogLevel string

// RootList represents a collection of root resources in the system.
// Contains:
// - Roots: Array of Root objects, each containing:
//   - URI: A unique identifier for accessing the root resource
//   - Name: A human-readable name for the root
type RootList struct {
	Roots []Root `json:"roots"`
}

// Root represents a top-level resource entry point in the system.
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// SamplingParams defines the parameters for generating a sampled message.
// It provides complete control over the sampling process including:
//   - Conversation history as a sequence of user and assistant messages
//   - Model selection preferences for balancing cost, speed, and intelligence
//   - System-level prompts that guide the model's behavior
//   - Token limit constraints for the generated response
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
