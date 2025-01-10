# go-mcp

[![Go Reference](https://pkg.go.dev/badge/github.com/MegaGrindStone/go-mcp.svg)](https://pkg.go.dev/github.com/MegaGrindStone/go-mcp)
![CI](https://github.com/MegaGrindStone/go-mcp/actions/workflows/ci.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/MegaGrindStone/go-mcp)](https://goreportcard.com/report/github.com/MegaGrindStone/go-mcp)
[![codecov](https://codecov.io/gh/MegaGrindStone/go-mcp/branch/main/graph/badge.svg)](https://codecov.io/gh/MegaGrindStone/go-mcp)

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
go get github.com/MegaGrindStone/go-mcp
```

## Usage

### Server Implementation

There are two main approaches to implementing an `go-mcp` server:

#### 1. Basic Server

```go
type MyMCPServer struct{}

func (MyMCPServer) Info() mcp.Info {
    return mcp.Info{
        Name:    "my-mcp-server",
        Version: "1.0",
    }
}

func (MyMCPServer) RequireRootsListClient() bool {
    return false
}

func (MyMCPServer) RequireSamplingClient() bool {
    return false
}
```

#### 2. Choose Transport Layer

`go-mcp` supports two transport options:

##### Server-Sent Events (SSE)
```go
sseSrv := mcp.NewSSEServer()
go mcp.Serve(ctx, MyMCPServer{}, sseSrv)

// Set up HTTP handlers
http.HandleFunc("/sse", sseSrv.HandleSSE())
http.HandleFunc("/message", sseSrv.HandleMessage())
http.ListenAndServe(":8080", nil)
```

##### Standard IO
```go
stdIOSrv := mcp.NewStdIO(os.Stdin, os.Stdout)
go mcp.Serve(ctx, MyMCPServer{}, stdIOSrv)
```

### Client Implementation

```go
// Create client info
info := mcp.Info{
    Name:    "my-mcp-client",
    Version: "1.0",
}

// Choose transport layer - SSE or Standard IO
// Option 1: Server-Sent Events (SSE)
sseClient := mcp.NewSSEClient("http://localhost:8080/sse", http.DefaultClient)
cli := mcp.NewClient(info, sseClient)

// Option 2: Standard IO
srvReader, srvWriter := io.Pipe()
cliReader, cliWriter := io.Pipe()
cliIO := mcp.NewStdIO(cliReader, srvWriter)
srvIO := mcp.NewStdIO(srvReader, cliWriter)
cli := mcp.NewClient(info, cliIO)

// Connect client
if err := cli.Connect(); err != nil {
    log.Fatal(err)
}
defer cli.Close()
```

#### Making Requests

```go
// List available tools
tools, err := cli.ListTools(ctx, mcp.ListToolsParams{})
if err != nil {
    log.Fatal(err)
}

// Call a tool
result, err := cli.CallTool(ctx, mcp.CallToolParams{
    Name: "echo",
    Arguments: map[string]any{
        "message": "Hello MCP!",
    },
})
if err != nil {
    log.Fatal(err)
}
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
