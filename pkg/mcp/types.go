package mcp

// LogWatcher provides an interface for watching and controlling log messages in the MCP protocol.
// It's used in conjunction with LoggingCapability to enable clients to receive log messages
// and control the logging level.
//
// Implementations should maintain a channel that emits LogParams values containing:
// - The severity level of the log message
// - The logger name/identifier
// - The message content and optional structured details
//
// The channel should remain open for the lifetime of the watcher. Multiple goroutines
// may receive from the channel simultaneously. Implementations must ensure thread-safe
// access to the channel.
type LogWatcher interface {
	// WatchLog returns a receive-only channel that emits LogParams values
	// containing log messages from the system.
	//
	// The returned channel should:
	// - Remain open for the lifetime of the watcher
	// - Be safe for concurrent receives from multiple goroutines
	// - Never block on sends - implementations should use buffered channels or
	//   drop notifications if receivers are not keeping up
	//
	// The LogParams emitted will contain:
	// - Level: The severity level of the log message (debug through emergency)
	// - Logger: The name/identifier of the logging source
	// - Data: The log message content and optional structured details
	WatchLog() <-chan LogParams

	// SetLogLevel configures the minimum severity level of log messages that
	// will be emitted through the WatchLog channel.
	//
	// Messages with a severity level lower than the configured level will be
	// filtered out and not sent through the channel.
	//
	// The level parameter must be one of the defined LogLevel constants:
	// LogLevelDebug, LogLevelInfo, LogLevelNotice, LogLevelWarning,
	// LogLevelError, LogLevelCritical, LogLevelAlert, or LogLevelEmergency
	SetLogLevel(level LogLevel)
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

// ContentType represents the type of content in messages.
const (
	ContentTypeText     ContentType = "text"
	ContentTypeImage    ContentType = "image"
	ContentTypeResource ContentType = "resource"
)
