package mcp

import "context"

// PromptServer defines the interface for managing prompts in the MCP protocol.
// Implementations handle the core prompt functionality including listing available prompts,
// retrieving specific prompts with arguments, and providing prompt argument completions.
type PromptServer interface {
	// ListPrompts returns a paginated list of available prompts.
	//
	// cursor: Optional pagination cursor from a previous ListPrompts call.
	// Empty string requests the first page.
	//
	// progressToken: A unique token for tracking operation progress.
	// If the implementation supports ProgressReporter, it will emit progress updates
	// using this token. Progress reporting is optional - implementations may ignore
	// this token if they don't support progress tracking.
	// However, if progress reporting is implemented, this token must be used to
	// correlate progress events with the specific operation.
	//
	// Returns PromptList containing available prompts and next page cursor.
	// Returns error if the operation fails.
	ListPrompts(ctx context.Context, cursor string, progressToken MustString) (PromptList, error)

	// GetPrompt retrieves a specific prompt template by name with the given arguments.
	//
	// name: The unique identifier of the prompt to retrieve
	//
	// args: Map of argument name to value pairs to be applied to the prompt template.
	// Must satisfy the required arguments defined in the prompt's Arguments field.
	//
	// progressToken: A unique token for tracking operation progress.
	// If the implementation supports ProgressReporter, it will emit progress updates
	// using this token. Progress reporting is optional - implementations may ignore
	// this token if they don't support progress tracking.
	// However, if progress reporting is implemented, this token must be used to
	// correlate progress events with the specific operation.
	//
	// Returns the Prompt with the template and metadata.
	// Returns error if prompt not found or arguments are invalid.
	GetPrompt(ctx context.Context, name string, args map[string]any, progressToken MustString) (Prompt, error)

	// CompletesPrompt provides completion suggestions for a prompt argument.
	// Used to implement interactive argument completion in clients.
	//
	// name: The unique identifier of the prompt to get completions for
	//
	// arg: The argument name and partial value to get completions for
	//
	// Returns CompletionResult containing possible values that complete the partial argument.
	// The HasMore field indicates if additional completion values are available.
	// Returns error if the prompt doesn't exist or completions cannot be generated.
	CompletesPrompt(ctx context.Context, name string, arg CompletionArgument) (CompletionResult, error)
}

// PromptListUpdater provides an interface for monitoring changes to the available prompts list.
// Implementations should maintain a channel that emits notifications whenever the list of
// available prompts changes, such as when prompts are added, removed, or modified.
//
// The notifications are used by the MCP server to inform connected clients about prompt list
// changes via the "notifications/prompts/list_changed" method. Clients can then refresh their
// cached prompt lists by calling ListPrompts again.
//
// The channel returned by PromptListUpdates should:
//   - Remain open for the lifetime of the updater
//   - Be safe for concurrent receives from multiple goroutines
//   - Never block on sends - implementations should use buffered channels or
//     drop notifications if receivers are not keeping up
//
// A struct{} is sent through the channel as the actual value is not important,
// only the notification of a change matters.
//
// Example implementation:
//
//	type MyPromptServer struct {
//	    prompts    []Prompt
//	    updateChan chan struct{}
//	}
//
//	func (s *MyPromptServer) PromptListUpdates() <-chan struct{} {
//	    return s.updateChan
//	}
//
//	func (s *MyPromptServer) AddPrompt(p Prompt) {
//	    s.prompts = append(s.prompts, p)
//	    // Notify listeners that prompt list has changed
//	    s.updateChan <- struct{}{}
//	}
type PromptListUpdater interface {
	PromptListUpdates() <-chan struct{}
}

// ResourceServer defines the interface for managing resources in the MCP protocol.
// Implementations handle core resource functionality including listing available resources,
// reading specific resources, managing resource templates, and resource subscriptions.
type ResourceServer interface {
	// ListResources returns a paginated list of available resources.
	//
	// cursor: Optional pagination cursor from a previous ListResources call.
	// Empty string requests the first page.
	//
	// progressToken: A unique token for tracking operation progress.
	// If the implementation supports ProgressReporter, it will emit progress updates
	// using this token. Progress reporting is optional - implementations may ignore
	// this token if they don't support progress tracking.
	// However, if progress reporting is implemented, this token must be used to
	// correlate progress events with the specific operation.
	//
	// Returns ResourceList containing available resources and next page cursor.
	// Returns error if the operation fails.
	ListResources(ctx context.Context, cursor string, progressToken MustString) (*ResourceList, error)

	// ReadResource retrieves a specific resource by its URI.
	//
	// uri: The unique identifier of the resource to retrieve.
	//
	// progressToken: A unique token for tracking operation progress.
	// If the implementation supports ProgressReporter, it will emit progress updates
	// using this token. Progress reporting is optional - implementations may ignore
	// this token if they don't support progress tracking.
	// However, if progress reporting is implemented, this token must be used to
	// correlate progress events with the specific operation.
	//
	// Returns the Resource with its content and metadata.
	// Returns error if resource not found or cannot be read.
	ReadResource(ctx context.Context, uri string, progressToken MustString) (*Resource, error)

	// ListResourceTemplates returns all available resource templates.
	// Templates define patterns for generating resource URIs and provide metadata
	// about the expected resource format.
	//
	// progressToken: A unique token for tracking operation progress.
	// If the implementation supports ProgressReporter, it will emit progress updates
	// using this token. Progress reporting is optional - implementations may ignore
	// this token if they don't support progress tracking.
	// However, if progress reporting is implemented, this token must be used to
	// correlate progress events with the specific operation.
	//
	// Returns slice of ResourceTemplate containing available templates.
	// Returns error if templates cannot be retrieved.
	ListResourceTemplates(ctx context.Context, progressToken MustString) ([]ResourceTemplate, error)

	// CompletesResourceTemplate provides completion suggestions for a resource template argument.
	// Used to implement interactive URI template completion in clients.
	//
	// name: The unique identifier of the template to get completions for
	//
	// arg: The argument name and partial value to get completions for
	//
	// Returns CompletionResult containing possible values that complete the partial argument.
	// The HasMore field indicates if additional completion values are available.
	// Returns error if the template doesn't exist or completions cannot be generated.
	CompletesResourceTemplate(ctx context.Context, name string, arg CompletionArgument) (CompletionResult, error)

	// SubscribeResource registers interest in a specific resource URI.
	// When the resource changes, notifications will be sent via ResourceSubcribedUpdater
	// to all subscribed clients.
	//
	// uri: The unique identifier of the resource to subscribe to.
	// The same URI used in ReadResource calls.
	SubscribeResource(uri string)
}

// ResourceListUpdater provides an interface for monitoring changes to the available resources list.
// Implementations should maintain a channel that emits notifications whenever the list of
// available resources changes, such as when resources are added, removed, or modified.
//
// The notifications are used by the MCP server to inform connected clients about resource list
// changes via the "notifications/resources/list_changed" method. Clients can then refresh their
// cached resource lists by calling ListResources again.
//
// The channel returned by ResourceListUpdates should:
//   - Remain open for the lifetime of the updater
//   - Be safe for concurrent receives from multiple goroutines
//   - Never block on sends - implementations should use buffered channels or
//     drop notifications if receivers are not keeping up
//
// A struct{} is sent through the channel as the actual value is not important,
// only the notification of a change matters.
//
// Example usage:
//
//	type MyResourceServer struct {
//	    resources  []Resource
//	    updateChan chan struct{}
//	}
//
//	func (s *MyResourceServer) ResourceListUpdates() <-chan struct{} {
//	    return s.updateChan
//	}
//
//	func (s *MyResourceServer) AddResource(r Resource) {
//	    s.resources = append(s.resources, r)
//	    // Notify listeners that resource list has changed
//	    s.updateChan <- struct{}{}
//	}
type ResourceListUpdater interface {
	ResourceListUpdates() <-chan struct{}
}

// ResourceSubscribedUpdater provides an interface for monitoring changes to subscribed resources.
// Implementations should maintain a channel that emits notifications whenever a subscribed resource
// changes, such as when its content is modified or updated.
//
// The notifications are used by the MCP server to inform connected clients about resource changes
// via the "notifications/resources/subscribe" method. Clients that have subscribed to specific
// resources using ResourceServer.SubscribeResource will receive these notifications and can then
// refresh their cached resource content by calling ReadResource again.
//
// The channel returned by ResourceSubscriberUpdates should:
//   - Remain open for the lifetime of the updater
//   - Be safe for concurrent receives from multiple goroutines
//   - Never block on sends - implementations should use buffered channels or
//     drop notifications if receivers are not keeping up
//   - Emit the URI of the resource that changed
//
// Example implementation:
//
//	type MyResourceServer struct {
//	    resources  map[string]*Resource
//	    updateChan chan string
//	}
//
//	func (s *MyResourceServer) ResourceSubscriberUpdates() <-chan string {
//	    return s.updateChan
//	}
//
//	func (s *MyResourceServer) UpdateResource(uri string, content []byte) {
//	    s.resources[uri] = &Resource{URI: uri, Content: content}
//	    // Notify subscribers that resource has changed
//	    s.updateChan <- uri
//	}
type ResourceSubscribedUpdater interface {
	ResourceSubscriberUpdates() <-chan string
}

// ToolServer defines the interface for managing tools in the MCP protocol.
// Implementations handle core tool functionality including listing available tools
// and executing tool operations with arguments.
type ToolServer interface {
	// ListTools returns a paginated list of available tools.
	//
	// cursor: Optional pagination cursor from a previous ListTools call.
	// Empty string requests the first page.
	//
	// progressToken: A unique token for tracking operation progress.
	// If the implementation supports ProgressReporter, it will emit progress updates
	// using this token. Progress reporting is optional - implementations may ignore
	// this token if they don't support progress tracking.
	// However, if progress reporting is implemented, this token must be used to
	// correlate progress events with the specific operation.
	//
	// Returns ToolList containing available tools and next page cursor.
	// The ToolList includes each tool's name, description and input schema.
	// Returns error if the operation fails.
	ListTools(ctx context.Context, cursor string, progressToken MustString) (*ToolList, error)

	// CallTool executes a specific tool with the given arguments.
	//
	// name: The unique identifier of the tool to execute
	//
	// args: Map of argument name to value pairs to be passed to the tool.
	// Must satisfy the required arguments defined in the tool's InputSchema field.
	//
	// progressToken: A unique token for tracking operation progress.
	// If the implementation supports ProgressReporter, it will emit progress updates
	// using this token. Progress reporting is optional - implementations may ignore
	// this token if they don't support progress tracking.
	// However, if progress reporting is implemented, this token must be used to
	// correlate progress events with the specific operation.
	//
	// Returns ToolResult containing the execution output and error status.
	// The Content field contains the tool's output data.
	// The IsError field indicates if the tool execution failed.
	// Returns error if tool not found, arguments are invalid, or execution fails.
	CallTool(ctx context.Context, name string, args map[string]any, progressToken MustString) (ToolResult, error)
}

// ToolListUpdater provides an interface for monitoring changes to the available tools list.
// Implementations should maintain a channel that emits notifications whenever the list of
// available tools changes, such as when tools are added, removed, or modified.
//
// The notifications are used by the MCP server to inform connected clients about tool list
// changes via the "notifications/tools/list_changed" method. Clients can then refresh their
// cached tool lists by calling ListTools again.
//
// The channel returned by WatchToolList should:
//   - Remain open for the lifetime of the updater
//   - Be safe for concurrent receives from multiple goroutines
//   - Never block on sends - implementations should use buffered channels or
//     drop notifications if receivers are not keeping up
//
// A struct{} is sent through the channel as the actual value is not important,
// only the notification of a change matters.
//
// Example implementation:
//
//	type MyToolServer struct {
//	    tools     []*Tool
//	    updateChan chan struct{}
//	}
//
//	func (s *MyToolServer) WatchToolList() <-chan struct{} {
//	    return s.updateChan
//	}
//
//	func (s *MyToolServer) AddTool(t *Tool) {
//	    s.tools = append(s.tools, t)
//	    // Notify listeners that tool list has changed
//	    s.updateChan <- struct{}{}
//	}
type ToolListUpdater interface {
	WatchToolList() <-chan struct{}
}

// ProgressReporter provides an interface for reporting progress updates on long-running operations.
// It's used to provide feedback during operations that may take significant time to complete.
//
// The progress updates are correlated to specific operations using the progressToken parameter
// that is passed to various server methods (ListPrompts, GetPrompt, ListResources, etc).
// Each progress update includes both the current progress value and total expected value,
// allowing calculation of completion percentage.
//
// Implementations should maintain a channel that emits ProgressParams values containing:
// - The progressToken matching the operation being reported on
// - The current progress value
// - The total expected value (when known)
//
// The channel should remain open for the lifetime of the reporter. Multiple goroutines
// may receive from the channel simultaneously. Implementations must ensure thread-safe
// access to the channel.
type ProgressReporter interface {
	// ProgressReports returns a receive-only channel that emits ProgressParams values
	// containing progress updates for long-running operations.
	//
	// The returned channel should:
	// - Remain open for the lifetime of the reporter
	// - Be safe for concurrent receives from multiple goroutines
	// - Never block on sends - implementations should use buffered channels or
	//   drop notifications if receivers are not keeping up
	//
	// The ProgressParams emitted will contain:
	// - progressToken: Matching the token provided to the original operation
	// - progress: Current progress value
	// - total: Total expected value (when known)
	//
	// Progress percentage can be calculated as: (progress / total) * 100
	// when total is non-zero.
	ProgressReports() <-chan ProgressParams
}

// LogHandler provides an interface for streaming log messages from the MCP server to connected clients.
// It allows clients to receive log messages in real-time and configure the minimum severity level
// of messages they want to receive.
//
// Log messages are emitted through the channel returned by LogStreams() and include:
// - The severity level of the message (debug, info, notice, warning, error, etc.)
// - The logger name/identifier that generated the message
// - The message content and any additional structured data/details
//
// The server uses this interface to:
// 1. Stream log messages to clients via the "notifications/message" method
// 2. Allow clients to filter messages by severity level
//
// Example implementation:
//
//	type MyLogHandler struct {
//	    logChan chan LogParams
//	    level   LogLevel
//	    mu      sync.RWMutex
//	}
//
//	func (h *MyLogHandler) LogStreams() <-chan LogParams {
//	    return h.logChan
//	}
//
//	func (h *MyLogHandler) SetLogLevel(level LogLevel) {
//	    h.mu.Lock()
//	    defer h.mu.Unlock()
//	    h.level = level
//	}
//
//	func (h *MyLogHandler) Log(msg LogParams) {
//	    h.mu.RLock()
//	    level := h.level
//	    h.mu.RUnlock()
//
//	    if shouldEmit(msg.Level, level) {
//	        h.logChan <- msg
//	    }
//	}
type LogHandler interface {
	// LogStreams returns a receive-only channel that emits LogParams containing
	// log messages and their metadata.
	//
	// The returned channel should:
	// - Remain open for the lifetime of the handler
	// - Be safe for concurrent receives from multiple goroutines
	// - Never block on sends - implementations should use buffered channels or
	//   drop messages if receivers are not keeping up
	//
	// The LogParams emitted will contain:
	// - Level: The severity level of the message
	// - Logger: The name/identifier of the logger that generated the message
	// - Data: The message content and any additional structured data
	LogStreams() <-chan LogParams

	// SetLogLevel configures the minimum severity level of log messages that
	// will be emitted through the LogStreams channel.
	//
	// Messages with a severity level lower than the configured level will be
	// filtered out and not sent through the channel.
	//
	// The level parameter must be one of the defined LogLevel constants:
	// LogLevelDebug, LogLevelInfo, LogLevelNotice, LogLevelWarning,
	// LogLevelError, LogLevelCritical, LogLevelAlert, or LogLevelEmergency
	SetLogLevel(level LogLevel)
}

// RootsListWatcher provides an interface for receiving notifications when the client's root list changes.
// The implementation can use these notifications to update its internal state or perform necessary actions
// when the client's available roots change.
type RootsListWatcher interface {
	// OnRootsListChanged is called when the client notifies that its root list has changed
	OnRootsListChanged()
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
	Type string `json:"type"`

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
