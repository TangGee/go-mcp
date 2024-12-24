package everything

import (
	"slices"
	"sync"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
)

// SSEServer implements a Server-Sent Events (SSE) server for the "everything" MCP server implementation.
// It provides a complete demonstration of MCP protocol features including prompts, resources, tools,
// and progress reporting. The server maintains subscriptions to resources and handles progress updates
// through channels. The underlying MCPServer handles the SSE transport and protocol details while this
// implementation focuses on the business logic and feature demonstrations.
type SSEServer struct {
	MCPServer mcp.SSEServer

	resourceSubscribers *sync.Map // map[resourceURI]struct{}

	logLevel mcp.LogLevel

	updateResourceSubsChan chan string
	progressChan           chan mcp.ProgressParams
	logChan                chan mcp.LogParams

	doneChan chan struct{}
}

// NewSSEServer creates and initializes a new SSEServer instance that implements the complete MCP protocol.
// It automatically configures all available server interfaces (PromptServer, ResourceServer, ToolServer,
// ProgressReporter) and their corresponding capabilities. The server demonstrates various features including
// echo commands, mathematical operations, long-running tasks with progress updates, environment inspection,
// and LLM sampling. The option parameter allows customizing the underlying MCP server behavior through
// ServerOptions like timeout settings and ping intervals. The returned SSEServer is ready to handle client
// connections and process requests across all supported interfaces.
func NewSSEServer(option ...mcp.ServerOption) *SSEServer {
	s := &SSEServer{
		resourceSubscribers:    new(sync.Map),
		logLevel:               mcp.LogLevelDebug,
		updateResourceSubsChan: make(chan string),
		progressChan:           make(chan mcp.ProgressParams, 10),
		logChan:                make(chan mcp.LogParams, 10),
		doneChan:               make(chan struct{}),
	}

	// Insert all the interfaces at the beginning of the option list,
	// so user can override them.
	option = slices.Insert(option, 0, mcp.WithPromptServer(s))
	option = slices.Insert(option, 0, mcp.WithResourceServer(s))
	option = slices.Insert(option, 0, mcp.WithToolServer(s))
	option = slices.Insert(option, 0, mcp.WithResourceSubscribedUpdater(s))
	option = slices.Insert(option, 0, mcp.WithProgressReporter(s))
	option = slices.Insert(option, 0, mcp.WithLogHandler(s))

	s.MCPServer = mcp.NewSSEServer(s, option...)

	go s.simulateResourceUpdates()

	return s
}

// Info implements mcp.Server interface.
func (s SSEServer) Info() mcp.Info {
	return mcp.Info{
		Name:    "everything",
		Version: "1.0",
	}
}

// RequireRootsListClient implements mcp.Server interface.
func (s SSEServer) RequireRootsListClient() bool {
	return false
}

// RequireSamplingClient implements mcp.Server interface.
func (s SSEServer) RequireSamplingClient() bool {
	return true
}

// Close closes the SSEServer and stops all background tasks.
func (s SSEServer) Close() {
	close(s.doneChan)
}
