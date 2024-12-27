package everything

import (
	"sync"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
)

// Server implements a comprehensive test server that exercises all features of the MCP protocol.
// It provides implementations of prompts, tools, resources, and sampling capabilities primarily
// for testing MCP client implementations.
//
// Server maintains subscriptions for resource updates and supports progress tracking and
// multi-level logging through dedicated channels. While not intended for production use,
// it serves as both a reference implementation and testing tool for MCP protocol features.
type Server struct {
	resourceSubscribers *sync.Map // map[resourceURI]struct{}

	logLevel mcp.LogLevel

	updateResourceSubsChan chan string
	progressChan           chan mcp.ProgressParams
	logChan                chan mcp.LogParams

	doneChan chan struct{}
}

// NewServer creates a new test server that implements all MCP protocol features. It initializes
// internal state and starts background tasks for simulating resource updates.
//
// The server starts with debug-level logging and supports concurrent resource subscriptions
// through thread-safe operations. Resource updates are simulated via background goroutines
// to facilitate testing of client subscription handling.
//
// Callers must call Close when finished to properly cleanup background tasks and release
// resources.
func NewServer() *Server {
	s := &Server{
		resourceSubscribers:    new(sync.Map),
		logLevel:               mcp.LogLevelDebug,
		updateResourceSubsChan: make(chan string),
		progressChan:           make(chan mcp.ProgressParams, 10),
		logChan:                make(chan mcp.LogParams, 10),
		doneChan:               make(chan struct{}),
	}

	go s.simulateResourceUpdates()

	return s
}

// Info implements mcp.Server interface.
func (s Server) Info() mcp.Info {
	return mcp.Info{
		Name:    "everything",
		Version: "1.0",
	}
}

// RequireRootsListClient implements mcp.Server interface.
func (s Server) RequireRootsListClient() bool {
	return false
}

// RequireSamplingClient implements mcp.Server interface.
func (s Server) RequireSamplingClient() bool {
	return true
}

// Close closes the SSEServer and stops all background tasks.
func (s Server) Close() {
	close(s.doneChan)
}
