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
	transport           mcp.ServerTransport
	resourceSubscribers *sync.Map // map[resourceURI]struct{}

	logLevel mcp.LogLevel

	updateResourceSubsChan chan string
	progressChan           chan mcp.ProgressParams
	logChan                chan mcp.LogParams

	doneChan chan struct{}
}

// NewServer creates and initializes a new Server instance with the provided transport layer.
// It sets up internal channels for resource updates, progress tracking, and logging, and
// starts a background goroutine to simulate resource updates.
//
// The server begins with debug-level logging enabled and can handle multiple concurrent
// resource subscriptions. The caller should invoke Close when the server is no longer needed.
func NewServer(transport mcp.ServerTransport) *Server {
	s := &Server{
		transport:              transport,
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

// Transport implements mcp.Server interface.
func (s Server) Transport() mcp.ServerTransport {
	return s.transport
}

// Close closes the SSEServer and stops all background tasks.
func (s Server) Close() {
	close(s.doneChan)
}
