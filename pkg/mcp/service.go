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

// PromptListWatcher provides an interface for watching changes to the available prompts list.
// It's used in conjunction with PromptsCapability.ListChanged to notify clients when the list
// of available prompts has been modified.
//
// Implementations should maintain a channel that emits empty struct{} values whenever
// the prompts list changes. The empty struct signals that the list has changed without
// specifying what changed - clients should call ListPrompts again to get the updated list.
//
// The channel should remain open for the lifetime of the watcher. Multiple goroutines
// may receive from the channel simultaneously. Implementations must ensure thread-safe
// access to the channel.
type PromptListWatcher interface {
	// WatchPromptList returns a receive-only channel that emits empty struct{} values
	// whenever the list of available prompts changes.
	//
	// The returned channel should:
	// - Remain open for the lifetime of the watcher
	// - Be safe for concurrent receives from multiple goroutines
	// - Never block on sends - implementations should use buffered channels or
	//   drop notifications if receivers are not keeping up
	//
	// Clients should call ListPrompts after receiving a notification to get the
	// updated list of prompts.
	WatchPromptList() <-chan struct{}
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
	// When the resource changes, notifications will be sent via ResourceSubcribesWatcher
	// to all subscribed clients.
	//
	// uri: The unique identifier of the resource to subscribe to.
	// The same URI used in ReadResource calls.
	SubscribeResource(uri string)
}

// ResourceSubscribesWatcher provides an interface for watching resource subscription events.
// It's used in conjunction with ResourcesCapability.Subscribe to notify clients when resources
// they've subscribed to have been modified.
//
// Implementations should maintain a channel that emits the URI of resources that have changed.
// The URI string matches the one used in ResourceServer.SubscribeResource and ReadResource.
// Only URIs that clients have explicitly subscribed to via SubscribeResource should be emitted.
//
// The channel should remain open for the lifetime of the watcher. Multiple goroutines
// may receive from the channel simultaneously. Implementations must ensure thread-safe
// access to the channel.
type ResourceSubscribesWatcher interface {
	// WatchResourceSubscribed returns a receive-only channel that emits URIs of
	// subscribed resources whenever they change.
	//
	// The returned channel should:
	// - Remain open for the lifetime of the watcher
	// - Be safe for concurrent receives from multiple goroutines
	// - Never block on sends - implementations should use buffered channels or
	//   drop notifications if receivers are not keeping up
	//
	// Only URIs that have been registered via ResourceServer.SubscribeResource
	// should be emitted through this channel.
	//
	// Clients should call ReadResource with the received URI to get the
	// updated resource content.
	WatchResourceSubscribed() <-chan string
}

// ResourceListWatcher provides an interface for watching changes to the available resources list.
// It's used in conjunction with ResourcesCapability.ListChanged to notify clients when the list
// of available resources has been modified.
//
// Implementations should maintain a channel that emits empty struct{} values whenever
// the resources list changes. The empty struct signals that the list has changed without
// specifying what changed - clients should call ListResources again to get the updated list.
//
// The channel should remain open for the lifetime of the watcher. Multiple goroutines
// may receive from the channel simultaneously. Implementations must ensure thread-safe
// access to the channel.
type ResourceListWatcher interface {
	// WatchResourceList returns a receive-only channel that emits empty struct{} values
	// whenever the list of available resources changes.
	//
	// The returned channel should:
	// - Remain open for the lifetime of the watcher
	// - Be safe for concurrent receives from multiple goroutines
	// - Never block on sends - implementations should use buffered channels or
	//   drop notifications if receivers are not keeping up
	//
	// Clients should call ListResources after receiving a notification to get the
	// updated list of resources.
	WatchResourceList() <-chan struct{}
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

// ToolListWatcher provides an interface for watching changes to the available tools list.
// It's used in conjunction with ToolsCapability.ListChanged to notify clients when the list
// of available tools has been modified.
//
// Implementations should maintain a channel that emits empty struct{} values whenever
// the tools list changes. The empty struct signals that the list has changed without
// specifying what changed - clients should call ListTools again to get the updated list.
//
// The channel should remain open for the lifetime of the watcher. Multiple goroutines
// may receive from the channel simultaneously. Implementations must ensure thread-safe
// access to the channel.
type ToolListWatcher interface {
	// WatchToolList returns a receive-only channel that emits empty struct{} values
	// whenever the list of available tools changes.
	//
	// The returned channel should:
	// - Remain open for the lifetime of the watcher
	// - Be safe for concurrent receives from multiple goroutines
	// - Never block on sends - implementations should use buffered channels or
	//   drop notifications if receivers are not keeping up
	//
	// Clients should call ListTools after receiving a notification to get the
	// updated list of tools.
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
	// ReportProgress returns a receive-only channel that emits ProgressParams values
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
	ReportProgress() <-chan ProgressParams
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
