# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Changed

- Refactored parameter naming convention for `Client` request methods to improve consistency between method names and their parameters. Previously, parameter names like `PromptsListParams` and `PromptsGetParams` used noun-verb style while methods used verb-noun style. Now, parameter names follow the same verb-noun pattern as their corresponding methods (e.g., `ListPromptsParams` and `GetPromptParams`).
- Refactored the result name of the request calls, either in `Client` or `Server` interfaces. This is done to improve consistency between method names and their results. For example, `ListPrompts` now returns `ListPromptsResult` instead of `PromptList`.
- Use structured parameter types (such as `ListPromptsParams` or `GetPromptParams`) in `Client` method signatures when making server requests, rather than using individual parameters. For example, instead of passing separate `cursor` and `progressToken` parameters to `ListPrompts`, or `name` and `arguments` to `GetPrompt`, use a dedicated parameter struct.
- Utilize `go-sse` to handle `SSEClient` by @tmaxmax.
- Utilize `go-sse`'s `Session` to sent the event messages from `SSEServer`.

## [0.2.0] - 2024-12-27

This release introduces a major architectural refactor centered around the new `Transport` interface and `Client` struct. The changes simplify the client architecture by moving from multi-session to single-session management, while providing a more flexible foundation for MCP implementations. The introduction of specialized `ServerTransport` and `ClientTransport` interfaces has enabled unified transport implementations and more consistent server implementations. Notable consolidations include merging separate StdIO implementations into a unified struct and relocating request functions from transport-specific clients to the main `Client` struct.

### Added

- `Transport` interface for client-server communication, with specialized `ServerTransport` and `ClientTransport` interfaces.
- `Client` struct for direct interaction with MCP servers.
- `Info` and `ServerRequirement` structs for client configuration management.
- `Serve` function for starting MCP servers.
- `StdIO` struct implementing both `ServerTransport` and `ClientTransport` interfaces.
- `RequestClientFunc` type alias for server-to-client function calls.

### Changed

- Simplified client architecture to support single-session management instead of multi-sessions.
- Relocated request functions from transport-specific clients to the main `Client` struct.
- Implemented unified test suite using the `Transport` interface.
- Enhanced `SSEServer` to implement the `ServerTransport` interface.
- Updated `SSEClient` to implement the `ClientTransport` interface.
- Everything server now utilizes the `Transport` interface.
- Filesystem server now utilizes the `Transport` interface.

### Removed

- Replaced `Client` interface with the new `Client` struct.
- Consolidated `StdIOServer` struct into the unified `StdIO` struct.
- Consolidated `StdIOClient` struct into the unified `StdIO` struct.

## [0.1.0] - 2024-12-24

### Added

- Server interfaces to implement MCP. 
- Client interfaces to interact with MCP servers.
- StdIO transport implementation.
- SSE transport implementation.
- Filesystem server implementation and example.
- Everything server implementation and example.
