package mcp

import (
	"time"
)

// Server represents the main MCP server interface that users will implement.
type Server interface {
	Info() Info
	RequiredClientCapabilities() ClientCapabilities
}

// LoggingHandler is an interface for logging.
type LoggingHandler interface {
	LogStream() <-chan LogParams
}

// ServerOption represents the options for the server.
type ServerOption func(*sessionDispatcher)

// WithPromptServer sets the prompt server for the server.
func WithPromptServer(server PromptServer) ServerOption {
	return func(s *sessionDispatcher) {
		s.promptServer = server
	}
}

// WithPromptListWatcher sets the prompt list watcher for the server.
func WithPromptListWatcher(watcher PromptListWatcher) ServerOption {
	return func(s *sessionDispatcher) {
		s.promptListWatcher = watcher
	}
}

// WithResourceServer sets the resource server for the server.
func WithResourceServer(server ResourceServer) ServerOption {
	return func(s *sessionDispatcher) {
		s.resourceServer = server
	}
}

// WithResourceListWatcher sets the resource list watcher for the server.
func WithResourceListWatcher(watcher ResourceListWatcher) ServerOption {
	return func(s *sessionDispatcher) {
		s.resourceListWatcher = watcher
	}
}

// WithResourceSubscribeWatcher sets the resource subscribe watcher for the server.
func WithResourceSubscribeWatcher(watcher ResourceSubscribesWatcher) ServerOption {
	return func(s *sessionDispatcher) {
		s.resourceSubscribesWatcher = watcher
	}
}

// WithToolServer sets the tool server for the server.
func WithToolServer(server ToolServer) ServerOption {
	return func(s *sessionDispatcher) {
		s.toolServer = server
	}
}

// WithToolListWatcher sets the tool list watcher for the server.
func WithToolListWatcher(watcher ToolListWatcher) ServerOption {
	return func(s *sessionDispatcher) {
		s.toolListWatcher = watcher
	}
}

// WithLogHandler sets the log handler for the server.
func WithLogHandler(watcher LogWatcher) ServerOption {
	return func(s *sessionDispatcher) {
		s.logWatcher = watcher
	}
}

// WithProgressReporter sets the progress reporter for the server.
func WithProgressReporter(reporter ProgressReporter) ServerOption {
	return func(s *sessionDispatcher) {
		s.progressReporter = reporter
	}
}

// WithWriteTimeout sets the write timeout for the server.
func WithWriteTimeout(timeout time.Duration) ServerOption {
	return func(s *sessionDispatcher) {
		s.writeTimeout = timeout
	}
}

// WithPingInterval sets the ping interval for the server.
func WithPingInterval(interval time.Duration) ServerOption {
	return func(s *sessionDispatcher) {
		s.pingInterval = interval
	}
}
