// Package mcp implements the Model Context Protocol (MCP), providing a framework for integrating
// Large Language Models (LLMs) with external data sources and tools. This implementation follows
// the official specification from https://spec.modelcontextprotocol.io/specification/.
//
// The package enables seamless integration between LLM applications and external data sources
// through a standardized protocol, making it suitable for building AI-powered IDEs, enhancing
// chat interfaces, or creating custom AI workflows.
//
// # Core Architecture
//
// The package is built on a JSON-RPC 2.0 message passing protocol with support for bidirectional
// communication between clients and servers. It provides comprehensive session management,
// including automatic connection lifecycle, health monitoring through ping/pong mechanisms,
// and graceful shutdown procedures.
//
// # Transport Layer
//
// The package supports multiple transport mechanisms:
//
// StdIO Transport uses standard input/output for command-line focused communication. It provides:
//   - Single session support per server-client lifetime
//   - Synchronous operations with JSON-RPC line processing
//   - Context-aware cancellation
//   - Non-blocking message handling
//
// SSE Transport implements Server-Sent Events over HTTP, offering:
//   - Real-time updates with automatic reconnection
//   - Multi-client capabilities
//   - Thread-safe operations using sync.Map
//   - CORS and custom HTTP client support
//   - Channel-based message routing
//
// # Server Components
//
// Servers implement a modular architecture through interfaces:
//
// Base Server interface (required) provides:
//   - Information provision
//   - Capability management
//
// Optional specialized servers:
//
// PromptServer handles template management and prompt operations:
//   - Template-based prompt management
//   - Version control
//   - Argument completion handling
//
// ResourceServer manages content access and subscription systems:
//   - Hierarchical resource organization
//   - Content type handling
//   - Subscription-based change notifications
//
// ToolServer provides tool integration:
//   - Dynamic tool discovery
//   - Synchronous/asynchronous execution
//   - Result streaming
//   - Error handling and recovery
//
// Additional server interfaces support:
//   - Progress reporting for long-running operations
//   - Multi-level logging system
//   - Real-time updates through various updater interfaces
//
// # Client Components
//
// Clients are built around a required base Client struct with optional extensions:
//
// Core Client capabilities:
//   - Connection management
//   - Request/response cycle handling
//   - Protocol feature APIs
//   - Event processing
//   - Automatic request ID generation
//
// Optional interfaces:
//
// RootsListHandler manages filesystem roots:
//   - Root resource management
//   - Path resolution
//   - Change notification handling
//
// SamplingHandler processes LLM requests:
//   - Model response handling
//   - Sampling parameter management
//   - Result processing
//
// Various watchers handle real-time updates:
//   - PromptListWatcher for template changes
//   - ResourceListWatcher for content updates
//   - ToolListWatcher for tool availability
//   - Progress and log monitoring
//
// # Interface Requirements
//
// The package enforces strict capability matching between servers and clients:
//
// Server requirements:
//   - Must implement all capabilities required by connected clients
//   - Base Server interface is mandatory
//   - Optional interfaces based on client needs
//
// Client requirements:
//   - Must implement all capabilities required by their servers
//   - Optional interfaces determined by server capabilities
//
// Required interface pairs:
//   - RootsListWatcher (server) with RootsListUpdater (client)
//   - PromptListUpdater (server) with PromptListWatcher (client)
//   - ResourceListUpdater (server) with ResourceListWatcher (client)
//   - ResourceSubscribedUpdater (server) with ResourceSubscribedWatcher (client)
//   - ToolListUpdater (server) with ToolListWatcher (client)
//   - ProgressReporter (server) with ProgressListener (client)
//   - LogHandler (server) with LogReceiver (client)
//
// # Concurrency Support
//
// Thread-safety is ensured through:
//   - Goroutine-based processing
//   - Synchronized state management
//   - Channel communication
//   - Context-aware operations
//
// # Extension Points
//
// The package supports extensibility through:
//   - Custom transport layer implementations
//   - Pluggable tool architecture
//   - Flexible resource handling
//   - Configurable logging system
package mcp
