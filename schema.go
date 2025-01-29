package mcp

import (
	"encoding/json"
	"fmt"
)

// MustString is a type that enforces string representation for fields that can be either string or integer
// in the protocol specification, such as request IDs and progress tokens. It handles automatic conversion
// during JSON marshaling/unmarshaling.
type MustString string

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

// ListPromptsParams contains parameters for listing available prompts.
type ListPromptsParams struct {
	// Cursor is an optional pagination cursor from previous ListPrompts call.
	// Empty string requests the first page.
	Cursor string `json:"cursor,omitempty"`

	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ListPromptResult represents a paginated list of prompts returned by ListPrompts.
// NextCursor can be used to retrieve the next page of results.
type ListPromptResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

// GetPromptParams contains parameters for retrieving a specific prompt.
type GetPromptParams struct {
	// Name is the unique identifier of the prompt to retrieve
	Name string `json:"name"`

	// Arguments is a map of argument name-value pairs
	// Must satisfy required arguments defined in prompt's Arguments field
	Arguments map[string]string `json:"arguments"`

	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// GetPromptResult represents the result of a prompt request.
type GetPromptResult struct {
	Messages    []PromptMessage `json:"messages"`
	Description string          `json:"description,omitempty"`
}

// ListResourcesParams contains parameters for listing available resources.
type ListResourcesParams struct {
	// Cursor is a pagination cursor from previous ListResources call.
	// Empty string requests the first page.
	Cursor string `json:"cursor"`

	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ListResourcesResult represents a paginated list of resources returned by ListResources.
// NextCursor can be used to retrieve the next page of results.
type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// ReadResourceParams contains parameters for retrieving a specific resource.
type ReadResourceParams struct {
	// URI is the unique identifier of the resource to retrieve.
	URI string `json:"uri"`

	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ReadResourceResult represents the result of a read resource request.
type ReadResourceResult struct {
	Contents []ResourceContents `json:"contents"`
}

// ListResourceTemplatesParams contains parameters for listing available resource templates.
type ListResourceTemplatesParams struct {
	// Cursor is a pagination cursor from previous ListResourceTemplates call.
	// Empty string requests the first page.
	Cursor string `json:"cursor"`

	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ListResourceTemplatesResult represents the result of a list resource templates request.
type ListResourceTemplatesResult struct {
	Templates  []ResourceTemplate `json:"resourceTemplates"`
	NextCursor string             `json:"nextCursor,omitempty"`
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

// ListToolsParams contains parameters for listing available tools.
type ListToolsParams struct {
	// Cursor is a pagination cursor from previous ListTools call.
	// Empty string requests the first page.
	Cursor string `json:"cursor"`

	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// ListToolsResult represents a paginated list of tools returned by ListTools.
// NextCursor can be used to retrieve the next page of results.
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// CallToolParams contains parameters for executing a specific tool.
type CallToolParams struct {
	// Name is the unique identifier of the tool to execute
	Name string `json:"name"`

	// Arguments is a JSON object of argument name-value pairs
	// Must satisfy required arguments defined in tool's InputSchema field
	Arguments json.RawMessage `json:"arguments"`

	// Meta contains optional metadata including progressToken for tracking operation progress.
	// The progressToken is used by ProgressReporter to emit progress updates if supported.
	Meta ParamsMeta `json:"_meta,omitempty"`
}

// CallToolResult represents the outcome of a tool invocation via CallTool.
// IsError indicates whether the operation failed, with details in Content.
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError"`
}

// RootList represents a collection of root resources in the system.
type RootList struct {
	Roots []Root `json:"roots"`
}

// LogParams represents the parameters for a log message.
type LogParams struct {
	// Level indicates the severity level of the message.
	// Must be one of the defined LogLevel constants.
	Level LogLevel `json:"level"`
	// Logger identifies the source/component that generated the message.
	Logger string `json:"logger"`
	// Data contains the message content and any structured metadata.
	Data json.RawMessage `json:"data"`
}

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

// Info contains metadata about a server or client instance including its name and version.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
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

// PromptMessage represents a message in a prompt.
type PromptMessage struct {
	Role    Role    `json:"role"`
	Content Content `json:"content"`
}

// Role represents the role in a conversation (user or assistant).
type Role string

// Content represents a message content with its type.
type Content struct {
	Type        ContentType  `json:"type"`
	Annotations *Annotations `json:"annotations,omitempty"`

	// For ContentTypeText
	Text string `json:"text,omitempty"`

	// For ContentTypeImage or ContentTypeAudio
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`

	// For ContentTypeResource
	Resource *ResourceContents `json:"resource,omitempty"`
}

// Annotations represents the annotations for a message. The client can use annotations
// to inform how objects are used or displayed.
type Annotations struct {
	// Audience describes who the intended customer of this object or data is.
	// It can include multiple entries to indicate content useful for multiple audiences.
	Audience []Role `json:"audience,omitempty"`
	// Priority describes how important this data is for operating the server.
	// A value of 1 means "most important," and indicates that the data is
	// effectively required, while 0 means "least important," and indicates that
	// the data is entirely optional.
	Priority int `json:"priority,omitempty"`
}

// ContentType represents the type of content in messages.
type ContentType string

// ResourceContents represents either text or blob resource contents.
type ResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"` // For text resources
	Blob     string `json:"blob,omitempty"` // For binary resources
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

// CompletionResult contains the response data for a completion request, including
// possible completion values and whether more completions are available.
type CompletionResult struct {
	Completion struct {
		Values  []string `json:"values"`
		HasMore bool     `json:"hasMore,omitempty"`
		Total   int      `json:"total,omitempty"`
	} `json:"completion"`
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

// Resource represents a content resource in the system with associated metadata.
// The content can be provided either as Text or Blob, with MimeType indicating the format.
type Resource struct {
	Annotations *Annotations `json:"annotations,omitempty"`
	URI         string       `json:"uri"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	MimeType    string       `json:"mimeType,omitempty"`
}

// ResourceTemplate defines a template for generating resource URIs.
// It's returned by ListResourceTemplates and used with CompletesResourceTemplate.
type ResourceTemplate struct {
	Annotations *Annotations `json:"annotations,omitempty"`
	URITemplate string       `json:"uriTemplate"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	MimeType    string       `json:"mimeType,omitempty"`
}

// Tool defines a callable tool with its input schema.
// InputSchema defines the expected format of arguments for CallTool.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// Root represents a root directory or file that the server can operate on.
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// LogLevel represents the severity level of log messages.
type LogLevel int

// ProgressParams represents the progress status of a long-running operation.
type ProgressParams struct {
	// ProgressToken uniquely identifies the operation this progress update relates to
	ProgressToken MustString `json:"progressToken"`
	// Progress represents the current progress value
	Progress float64 `json:"progress"`
	// Total represents the expected final value when known.
	// When non-zero, completion percentage can be calculated as (Progress/Total)*100
	Total float64 `json:"total,omitempty"`
}

// ParamsMeta contains optional metadata that can be included with request parameters.
// It is used to enable features like progress tracking for long-running operations.
type ParamsMeta struct {
	// ProgressToken uniquely identifies an operation for progress tracking.
	// When provided, the server can emit progress updates via ProgressReporter.
	ProgressToken MustString `json:"progressToken"`
}

type initializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Info               `json:"clientInfo"`
}

type initializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Info               `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

type notificationsCancelledParams struct {
	RequestID string `json:"requestId"`
	Reason    string `json:"reason"`
}

type notificationsResourcesUpdatedParams struct {
	URI string `json:"uri"`
}

// Role represents the role in a conversation (user or assistant).
const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ContentType represents the type of content in messages.
const (
	ContentTypeText     ContentType = "text"
	ContentTypeImage    ContentType = "image"
	ContentTypeAudio    ContentType = "audio"
	ContentTypeResource ContentType = "resource"
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

func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "debug"
	case LogLevelInfo:
		return "info"
	case LogLevelNotice:
		return "notice"
	case LogLevelWarning:
		return "warning"
	case LogLevelError:
		return "error"
	case LogLevelCritical:
		return "critical"
	case LogLevelAlert:
		return "alert"
	case LogLevelEmergency:
		return "emergency"
	default:
		return "unknown"
	}
}

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
