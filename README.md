# go-mcp

A Go implementation of the Model Context Protocol (MCP) - an open protocol that enables seamless integration between LLM applications and external data sources and tools.

## Overview

This repository provides a Go library implementing the Model Context Protocol (MCP) following the [official specification](https://spec.modelcontextprotocol.io/specification/).

### Demo

Watch this demo showcasing the example implementation from `example/everything`:

[![asciicast](https://asciinema.org/a/695973.svg)](https://asciinema.org/a/695973)

## Installation

```bash
go get github.com/MegaGrindStone/go-mcp
```

## Features

- Flexible server and client interfaces
- Multiple transport options (StdIO and SSE)
- Support for MCP primitives:
  - Prompts
  - Resources
  - Tools
- Progress reporting and logging capabilities
- Filesystem roots management
- LLM sampling support

## Packages

### pkg/mcp

Core package containing the protocol implementation:
- Server interfaces
- Client interfaces
- StdIO transport
- SSE transport

### pkg/servers

Reference server implementations aligned with [modelcontextprotocol/servers](https://github.com/modelcontextprotocol/servers).

## Documentation

### Core Package (pkg/mcp)

The core package provides flexible interfaces for implementing MCP servers and clients. The capabilities of servers and clients are determined by which interfaces they implement:

- The server must satisfy all capabilities required by the client, and vice versa
- Client requirements are specified via the `RequireXXX` methods in the `Client` interface (e.g., if `RequirePromptServer` returns true, the server must implement `PromptServer`)
- Server requirements are specified via the `RequireXXX` methods in the `Server` interface (e.g., if `RequireSamplingClient` returns true, the client must implement `SamplingHandler`)
- Only the base `Server` and `Client` interfaces are required - all other capabilities are optional

Additionally, there are paired interfaces that must be implemented together:
- If the server implements `RootsListWatcher`, the client must implement `RootsListUpdater`
- If the client implements `PromptListWatcher`, the server must implement `PromptListUpdater`
- If the client implements `ResourceListWatcher`, the server must implement `ResourceListUpdater`
- If the client implements `ResourceSubscribedWatcher`, the server must implement `ResourceSubscribedUpdater`
- If the client implements `ToolListWatcher`, the server must implement `ToolListUpdater`
- If the client implements `ProgressListener`, the server must implement `ProgressReporter`
- If the client implements `LogReceiver`, the server must implement `LogHandler`

The core package provides these key interfaces:

#### Server Implementation
- Implement the required `Server` interface and optional primitive interfaces:
  - `PromptServer` - For prompt management
  - `ResourceServer` - For resource handling 
  - `ToolServer` - For tool operations
- Optional interfaces for updates and notifications:
  - List updaters (`PromptListUpdater`, `ResourceListUpdater`, etc.)
  - `ProgressReporter` for long-running operations
  - `LogHandler` for log streaming
  - `RootsListWatcher` for filesystem root changes

#### Client Implementation
- Implement the required `Client` interface and optional interfaces based on needs
- Key optional interfaces:
  - `RootsListHandler`/`RootsListUpdater` - Manage filesystem roots
  - `SamplingHandler` - Handle LLM sampling requests
  - Watchers for primitives (`PromptListWatcher`, `ResourceListWatcher`, etc.)
  - `ProgressListener` - Monitor long-running operations
  - `LogReceiver` - Receive server logs

### Transport Options

#### StdIO Transport
- Uses standard input/output for communication
- Single session per server-client lifetime
- Usage:
  ```go
  // Server setup
  stdioServer := mcp.NewStdIOServer(serverImpl)
  
  // Client setup
  stdioClient := mcp.NewStdIOClient(clientImpl, stdioServer)
  ```

#### SSE Transport
- Server-Sent Events based communication
- Flexible HTTP server integration
- Usage:
  ```go
  // Server setup
  sseServer := mcp.NewSSEServer(serverImpl)
  http.HandleFunc("/sse", sseServer.HandleSSE)
  
  // Client setup
  sseClient := mcp.NewSSEClient(clientImpl, httpClient)
  ```

For detailed implementation examples, check the `example` folder and `pkg/servers` reference implementations.

