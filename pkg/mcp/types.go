package mcp

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

// ContentType represents the type of content in messages.
const (
	ContentTypeText     ContentType = "text"
	ContentTypeImage    ContentType = "image"
	ContentTypeResource ContentType = "resource"
)
