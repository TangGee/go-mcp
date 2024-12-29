# go-mcp

[![Go Reference](https://pkg.go.dev/badge/github.com/MegaGrindStone/go-mcp.svg)](https://pkg.go.dev/github.com/MegaGrindStone/go-mcp/pkg/mcp)
![CI](https://github.com/MegaGrindStone/go-mcp/actions/workflows/ci.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/MegaGrindStone/go-mcp)](https://goreportcard.com/report/github.com/MegaGrindStone/go-mcp)
[![codecov](https://codecov.io/gh/MegaGrindStone/go-mcp/branch/main/graph/badge.svg)](https://codecov.io/gh/MegaGrindStone/go-mcp)

A Go implementation of the Model Context Protocol (MCP) - an open protocol that enables seamless integration between LLM applications and external data sources and tools.

> ⚠️ **Warning**: This library is currently in active development and follows semantic versioning. Breaking changes may be introduced with minor version bumps (0.x.0) until v1.0.0 is released. After v1.0.0, the API will be stable and breaking changes will only occur in major version updates. We recommend pinning your dependency to a specific version and reviewing the changelog before upgrading.

## Overview

This repository provides a Go library implementing the Model Context Protocol (MCP) following the [official specification](https://spec.modelcontextprotocol.io/specification/).

## Features

### Core Components
- Complete implementation of MCP specification
- Clean separation between Server interface and transport mechanisms
- Comprehensive client implementation with connection management
- Multiple transport options:
  - Server-Sent Events (SSE) for real-time updates
  - Standard IO for CLI support

### Protocol Features
- **Prompts**: Template-based prompt management with versioning
- **Resources**: Content access and subscription system
- **Tools**: External tool discovery and execution
- **Events**: Real-time updates and notifications

### Technical Features
- Session management with automatic lifecycle handling
- Thread-safe operations and robust concurrency support
- Comprehensive error handling with standardized codes
- Progress tracking and multi-level logging
- Context-aware operations with cancellation support
- Filesystem roots management
- LLM sampling capabilities

### System Extensibility 
- Pluggable transport system via Transport interface
- Custom tool integration support
- Flexible resource handling
- Configurable logging system

## Installation

```bash
go get github.com/MegaGrindStone/go-mcp
```

## Usage

### Server

Let's walk through building your own MCP server step by step.

#### 1. Core Server Implementation

First, implement the required `mcp.Server` interface:

```go
type AwesomeMCPServer struct {}

func (AwesomeMCPServer) Info() mcp.Info {
    return mcp.Info{
        Name:    "awesome-mcp-server",
        Version: "1.0",
    }
}

func (AwesomeMCPServer) RequireRootsListClient() bool {
    // This value determines what capabilities that client must have to connect to your server
    return false
}

func (AwesomeMCPServer) RequireSamplingClient() bool {
    // This value determines what capabilities that client must have to connect to your server
    return false
} 
```

#### 2. Choose a Transport Layer

Your server needs a `mcp.ServerTransport` implementation to handle client communications. `go-mcp` provides two built-in options:

- `mcp.StdIO`: Uses standard input/output for command-line focused communication
- `mcp.SSEServer`: Uses Server-Sent Events (SSE) for real-time updates

You can also implement your own transport by implementing the `mcp.ServerTransport` interface.

#### 3. Start the Server

Important notes about server startup:

- `mcp.Serve` blocks until the provided context is cancelled.
- You must start the `ServerTransport` yourself - `mcp.Serve` won't do it. This gives you flexibility to choose any http framework you like.
- The `errsChan` receives operational errors you might want to handle.
- `mcp.Serve` automatically calls `Close` on the `ServerTransport` when the context is cancelled.

Here's how to start each transport type:

```go
// Using StdIO
errsChan := make(chan error)
stdIOSrv := mcp.NewStdIO(os.Stdin, os.Stdout)

go mcp.Serve(ctx, AwesomeMCPServer{}, stdIOSrv, errsChan)
stdIOSrv.Start()  // You must call Start() for StdIO

// Using SSE
errsChan := make(chan error)
sseSrv := mcp.NewSSEServer()

go mcp.Serve(ctx, AwesomeMCPServer{}, sseSrv, errsChan)
// SSE requires you to set up the HTTP server yourself
messageBaseURL := "http://localhost:8080/message"
http.HandleFunc("/sse", sseSrv.HandleSSE(messageBaseURL))
http.HandleFunc("/message", sseSrv.HandleMessage)
http.ListenAndServe(":8080", sseSrv)
```

#### 4. Add Optional Features

Enhance your server with optional capabilities. Here's an example adding tool support:

```go
type secretTooling struct {}

func (secretTooling) ListTools(ctx context.Context, params mcp.ListToolsParams, requestClient mcp.RequestClientFunc) (mcp.ToolList, error) {
    // Your implementation here
} 

func (secretTooling) CallTool(ctx context.Context, params mcp.CallToolParams, requestClient mcp.RequestClientFunc) (mcp.ToolResult, error) {
    // Your implementation here
}

// Add the tooling service to your server
go mcp.Serve(ctx, AwesomeMCPServer{}, sseSrv, errsChan, 
    mcp.WithToolServer(secretTooling{}))
```

There are other optional interfaces available like `mcp.PromptServer` and `mcp.ResourceServer` that you can implement to add more functionality.

#### Complete Example

Here's a full working example combining all the pieces:

```go
package main

import (
    "context"
    "net/http"
    "os"

    "github.com/MegaGrindStone/go-mcp/pkg/mcp"
)

type AwesomeMCPServer struct{}

func (AwesomeMCPServer) Info() mcp.Info {
    return mcp.Info{
        Name:    "awesome-mcp-server",
        Version: "1.0",
    }
}

func (AwesomeMCPServer) RequireRootsListClient() bool {
    return false
}

func (AwesomeMCPServer) RequireSamplingClient() bool {
    return false
}

type secretTooling struct{}

func (secretTooling) ListTools(ctx context.Context, params mcp.ListToolsParams, requestClient mcp.RequestClientFunc) (mcp.ToolList, error) {
    // Your implementation here
    return mcp.ToolList{}, nil
}

func (secretTooling) CallTool(ctx context.Context, params mcp.CallToolParams, requestClient mcp.RequestClientFunc) (mcp.ToolResult, error) {
    // Your implementation here
    return mcp.ToolResult{}, nil
}

func main() {
    ctx := context.Background()
    errsChan := make(chan error)

    // Using StdIO
    stdIOSrv := mcp.NewStdIO(os.Stdin, os.Stdout)
    go mcp.Serve(ctx, AwesomeMCPServer{}, stdIOSrv, errsChan, 
        mcp.WithToolServer(secretTooling{}))
    stdIOSrv.Start()
    // This would block until the context is cancelled

    // Or using SSE
    sseSrv := mcp.NewSSEServer()
    go mcp.Serve(ctx, AwesomeMCPServer{}, sseSrv, errsChan, 
        mcp.WithToolServer(secretTooling{}))
    
    messageBaseURL := "http://localhost:8080/message"
    http.HandleFunc("/sse", sseSrv.HandleSSE(messageBaseURL))
    http.HandleFunc("/message", sseSrv.HandleMessage)
    http.ListenAndServe(":8080", sseSrv)
}
```

### Client

Let's walk through setting up your own MCP client step by step.

#### 1. Basic Client Setup

First, prepare your client information by implementing `mcp.Info`:

```go
info := mcp.Info{
    Name:    "awesome-mcp-client",
    Version: "1.0", 
}
```

#### 2. Choose a Transport Layer

You'll need a `mcp.ClientTransport` implementation to communicate with the server. `go-mcp` provides two built-in options:

- `mcp.StdIO`: Uses standard input/output for command-line focused communication
- `mcp.SSEClient`: Uses Server-Sent Events (SSE) for real-time updates

An interesting note: `mcp.StdIO` can be used as either `mcp.ServerTransport` or `mcp.ClientTransport`, giving you flexibility in your implementation.

For SSE transport, setup is straightforward:
```go
sseClient := mcp.NewSSEClient("http://localhost:8080/sse", http.DefaultClient)
```

For StdIO transport, the setup requires more consideration. `go-mcp` uses `io.Reader` and `io.Writer` instead of `os.Stdin` and `os.Stdout` to provide maximum flexibility when communicating between OS processes. You have two main approaches:

1. Follow the official MCP implementation (Python and TypeScript) by using `os.Exec` to start a new process and communicate with it
2. Use `io.Pipe` as demonstrated in `example/Filesystem/main.go`:

```go
srvReader, srvWriter := io.Pipe()
cliReader, cliWriter := io.Pipe()

// client's output is server's input
cliIO := mcp.NewStdIO(cliReader, srvWriter)
// server's output is client's input
srvIO := mcp.NewStdIO(srvReader, cliWriter)

go srvIO.Start()
go cliIO.Start()
```

#### 3. Define Server Requirements

Specify which server features your client expects. Set flags to true for features you want the server to support:

```go
srvReq := mcp.ServerRequirement{
    ToolServer: true,
}
```

#### 4. Initialize and Connect

Create your client instance:
```go
cli := mcp.NewClient(info, transport, srvReq)
```

Important: Just like `mcp.Server`, `mcp.Client` won't start the `mcp.ClientTransport` for you.

Before making any requests, you must connect to the server:
```go
err := cli.Connect()
if err != nil {
    log.Fatal(err)
}
```

This initializes the protocol and establishes the connection. Don't forget to clean up resources:
```go
defer cli.Close()  // This also calls Close on the ClientTransport
```

#### 5. Make Requests

Now you can interact with the server:

```go
// List available tools
toolsList, err := cli.ListTools(ctx, "", "")
if err != nil {
    log.Fatal(err)
}

// Call a specific tool
toolResult, err := cli.CallTool(ctx, "calculator", map[string]any{
    "expression": "1+1",
})
if err != nil {
    log.Fatal(err)
}
```

#### Complete Example

Here's a full working example combining all the pieces:

```go
package main

import (
    "context"
    "io"
    "log"
    "net/http"

    "github.com/MegaGrindStone/go-mcp/pkg/mcp"
)

func main() {
    info := mcp.Info{
        Name:    "awesome-mcp-client",
        Version: "1.0", 
    }
    srvReq := mcp.ServerRequirement{
        ToolServer: true,
    }

    // Choose your transport:

    // Option 1: Using StdIO
    srvReader, srvWriter := io.Pipe()
    cliReader, cliWriter := io.Pipe()
    cliIO := mcp.NewStdIO(cliReader, srvWriter)
    srvIO := mcp.NewStdIO(srvReader, cliWriter)
    go srvIO.Start()
    go cliIO.Start()
    cli := mcp.NewClient(info, cliIO, srvReq)

    // Option 2: Using SSE
    sseClient := mcp.NewSSEClient("http://localhost:8080/sse", http.DefaultClient)
    cli := mcp.NewClient(info, sseClient, srvReq)

    defer cli.Close()
    err := cli.Connect()
    if err != nil {
        log.Fatal(err)
    }

    // Make requests
    toolsList, err := cli.ListTools(ctx, "", "")
    if err != nil {
        log.Fatal(err)
    }

    toolResult, err := cli.CallTool(ctx, "calculator", map[string]any{
        "expression": "1+1",
    })
    if err != nil {
        log.Fatal(err)
    }
}
```

## Server Packages

The `pkg/servers` directory contains reference server implementations that mirror those found in the official [modelcontextprotocol/servers](https://github.com/modelcontextprotocol/servers) repository.

## Example

The `example` folder contains sample implementations demonstrating how to use the servers from `pkg/servers` package.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
