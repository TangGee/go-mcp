# go-mcp

[![Go Reference](https://pkg.go.dev/badge/github.com/TangGee/go-mcp.svg)](https://pkg.go.dev/github.com/TangGee/go-mcp)
![CI](https://github.com/TangGee/go-mcp/actions/workflows/ci.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/TangGee/go-mcp)](https://goreportcard.com/report/github.com/TangGee/go-mcp)
[![codecov](https://codecov.io/gh/TangGee/go-mcp/branch/main/graph/badge.svg)](https://codecov.io/gh/TangGee/go-mcp)

A Go implementation of the Model Context Protocol (MCP) - an open protocol that enables seamless integration between LLM applications and external data sources and tools.

> ⚠️ **Warning**: The main branch contains unreleased changes and may be unstable. We recommend using the latest tagged release for stability. This library follows semantic versioning - breaking changes may be introduced with minor version bumps (0.x.0) until v1.0.0 is released. After v1.0.0, the API will be stable and breaking changes will only occur in major version updates. We recommend pinning your dependency to a specific version and reviewing the changelog before upgrading.

## Overview

This repository provides a Go library implementing the Model Context Protocol (MCP) following the [official specification](https://spec.modelcontextprotocol.io/specification/).

## Features

### Core Protocol
- Complete MCP protocol implementation with JSON-RPC 2.0 messaging
- Pluggable transport system supporting SSE and Standard IO
- Session-based client-server communication
- Comprehensive error handling and progress tracking

### Server Features
- Modular server implementation with optional capabilities
- Support for prompts, resources, and tools
- Real-time notifications and updates
- Built-in logging system
- Resource subscription management

### Client Features
- Flexible client configuration with optional capabilities
- Automatic session management and health monitoring
- Support for streaming and pagination
- Progress tracking and cancellation support
- Configurable timeouts and retry logic

### Transport Options
- Server-Sent Events (SSE) for web-based real-time updates
- Standard IO for command-line tool integration

## Installation

```bash
go get github.com/TangGee/go-mcp
```

## Usage

### Server Implementation

There are two main steps to implementing an MCP server:

#### 1. Create a Server Implementation

Create a server implementation that provides the capabilities you need:

```go
// Example implementing a server with tool support
type MyToolServer struct{}

func (s *MyToolServer) ListTools(ctx context.Context, params mcp.ListToolsParams, 
    progress mcp.ProgressReporter, requestClient mcp.RequestClientFunc) (mcp.ListToolsResult, error) {
    // Return available tools
    return mcp.ListToolsResult{
        Tools: []mcp.Tool{
            {
                Name: "example-tool",
                Description: "An example tool",
                // Additional tool properties...
            },
        },
    }, nil
}

func (s *MyToolServer) CallTool(ctx context.Context, params mcp.CallToolParams, 
    progress mcp.ProgressReporter, requestClient mcp.RequestClientFunc) (mcp.CallToolResult, error) {
    // Implement tool functionality
    return mcp.CallToolResult{
        Content: []mcp.Content{
            {
                Type: mcp.ContentTypeText,
                Text: "Tool result",
            },
        },
    }, nil
}
```

#### 2. Initialize and Serve

Create and configure the server with your implementation and chosen transport:

```go
// Create server with your implementation
toolServer := &MyToolServer{}

// Choose a transport method
// Option 1: Server-Sent Events (SSE)
sseSrv := mcp.NewSSEServer("/message")
srv := mcp.NewServer(mcp.Info{
    Name:    "my-mcp-server",
    Version: "1.0",
}, sseSrv, 
    mcp.WithToolServer(toolServer),
    mcp.WithServerPingInterval(30*time.Second),
    // Add other capabilities as needed
)

// Set up HTTP handlers for SSE
http.Handle("/sse", sseSrv.HandleSSE())
http.Handle("/message", sseSrv.HandleMessage())
go http.ListenAndServe(":8080", nil)

// Option 2: Standard IO
srvIO := mcp.NewStdIO(os.Stdin, os.Stdout)
srv := mcp.NewServer(mcp.Info{
    Name:    "my-mcp-server", 
    Version: "1.0",
}, srvIO,
    mcp.WithToolServer(toolServer),
    // Add other capabilities as needed
)

// Start the server - this blocks until shutdown
go srv.Serve()

// To shutdown gracefully
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
srv.Shutdown(ctx)
```

#### Available Server Options

Configure your server with additional capabilities:

```go
// Prompt capabilities
mcp.WithPromptServer(promptServer)
mcp.WithPromptListUpdater(promptListUpdater)

// Resource capabilities
mcp.WithResourceServer(resourceServer)
mcp.WithResourceListUpdater(resourceListUpdater)
mcp.WithResourceSubscriptionHandler(subscriptionHandler)

// Tool capabilities
mcp.WithToolServer(toolServer)
mcp.WithToolListUpdater(toolListUpdater)

// Roots and logging capabilities
mcp.WithRootsListWatcher(rootsListWatcher)
mcp.WithLogHandler(logHandler)

// Server behavior configuration
mcp.WithServerPingInterval(interval)
mcp.WithServerPingTimeout(timeout)
mcp.WithServerPingTimeoutThreshold(threshold)
mcp.WithServerSendTimeout(timeout)
mcp.WithInstructions(instructions)

// Event callbacks
mcp.WithServerOnClientConnected(func(id string, info mcp.Info) {
    fmt.Printf("Client connected: %s\n", id)
})
mcp.WithServerOnClientDisconnected(func(id string) {
    fmt.Printf("Client disconnected: %s\n", id)
})
```

### Client Implementation

The client implementation involves creating a client with transport options and capabilities, connecting to a server, and executing MCP operations.

#### Creating and Connecting a Client

```go
// Create client info
info := mcp.Info{
    Name:    "my-mcp-client",
    Version: "1.0",
}

// Create a context for connection and operations
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Choose transport layer - SSE or Standard IO
// Option 1: Server-Sent Events (SSE)
sseClient := mcp.NewSSEClient("http://localhost:8080/sse", http.DefaultClient)
cli := mcp.NewClient(info, sseClient,
    // Optional client configurations
    mcp.WithClientPingInterval(30*time.Second),
    mcp.WithProgressListener(progressListener),
    mcp.WithLogReceiver(logReceiver),
)

// Option 2: Standard IO
srvReader, srvWriter := io.Pipe()
cliReader, cliWriter := io.Pipe()
cliIO := mcp.NewStdIO(cliReader, srvWriter)
srvIO := mcp.NewStdIO(srvReader, cliWriter)
cli := mcp.NewClient(info, cliIO)

// Connect client (requires context)
if err := cli.Connect(ctx); err != nil {
    log.Fatal(err)
}
// Ensure proper cleanup
defer func() {
    disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    cli.Disconnect(disconnectCtx)
}()
```

#### Making Requests

```go
// List available tools
tools, err := cli.ListTools(ctx, mcp.ListToolsParams{})
if err != nil {
    log.Fatal(err)
}

// Call a tool (with proper argument structure)
args := map[string]string{"message": "Hello MCP!"}
argsBs, _ := json.Marshal(args)

result, err := cli.CallTool(ctx, mcp.CallToolParams{
    Name:      "echo",
    Arguments: argsBs,
})
if err != nil {
    log.Fatal(err)
}

// Work with resources
resources, err := cli.ListResources(ctx, mcp.ListResourcesParams{})
if err != nil {
    log.Fatal(err)
}

// Subscribe to resource updates
err = cli.SubscribeResource(ctx, mcp.SubscribeResourceParams{
    URI: "resource-uri",
})
if err != nil {
    log.Fatal(err)
}

// Work with prompts
prompts, err := cli.ListPrompts(ctx, mcp.ListPromptsParams{})
if err != nil {
    log.Fatal(err)
}

prompt, err := cli.GetPrompt(ctx, mcp.GetPromptParams{
    Name: "my-prompt",
})
if err != nil {
    log.Fatal(err)
}
```

#### Implementing Handlers for Client Capabilities

```go
// Implement required interfaces for client capabilities
type myClient struct {
    // ...client fields
}

// For sampling capability
func (c *myClient) CreateSampleMessage(ctx context.Context, params mcp.SamplingParams) (mcp.SamplingResult, error) {
    // Generate sample LLM output
    return mcp.SamplingResult{
        Role: mcp.RoleAssistant,
        Content: mcp.SamplingContent{
            Type: mcp.ContentTypeText,
            Text: "Sample response text",
        },
        Model: "my-llm-model",
    }, nil
}

// For resource subscription notifications
func (c *myClient) OnResourceSubscribedChanged(uri string) {
    fmt.Printf("Resource %s was updated\n", uri)
}

// For progress tracking
func (c *myClient) OnProgress(params mcp.ProgressParams) {
    fmt.Printf("Progress: %.2f/%.2f\n", params.Progress, params.Total)
}

// Pass these handlers when creating the client
cli := mcp.NewClient(info, transport,
    mcp.WithSamplingHandler(client),
    mcp.WithResourceSubscribedWatcher(client),
    mcp.WithProgressListener(client),
)
```

### Complete Examples

For complete working examples:

- See `example/everything/` for a comprehensive server and client implementation with all features
- See `example/filesystem/` for a focused example of file operations using Standard IO transport

These examples demonstrate:
- Server and client lifecycle management
- Transport layer setup
- Error handling
- Tool implementation
- Resource management
- Progress tracking
- Logging integration

For more details, check the [example directory](example/) in the repository.

## Server Packages

The `servers` directory contains reference server implementations that mirror those found in the official [modelcontextprotocol/servers](https://github.com/modelcontextprotocol/servers) repository.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
