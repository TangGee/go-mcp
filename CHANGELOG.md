# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Added

- Interrupt method for `Session` interface.
- Auto-close server session on ping timeout exceeds given threshold.

## [0.5.1] - 2025-02-17

This release focuses on improving error handling and `filesystem` server compatibility. Key improvements include enhanced error messaging in the JSONRPCError struct, better `filesystem` server alignment with the official MCP implementation, and the addition of comprehensive testing. These changes significantly improve debugging capabilities and ensure more reliable `filesystem` operations.

### Added

- Add comprehensive test suite for `filesystem` server implementation to ensure reliability and correctness.

### Changed

- Enhance `filesystem` server compatibility with official MCP implementation.
- Improve error message clarity in JSONRPCError struct for better debugging.
- Implement detailed error string representation in JSONRPCError struct, replacing generic "Internal error" messages.
- Add specific error handling for CallTool requests with dedicated CallToolResult error reporting.
- Exclude .git directory from directory_tree tool results to improve relevance of `filesystem` operations.

## [0.5.0] - 2025-02-09

This release focuses on improving JSON serialization flexibility and simplifying I/O interfaces. The changes include making annotations optional in resource types and streamlining the StdIO transport implementation by delegating closing responsibilities to callers.

### Changed

- Make `Annotations` field nullable in resource types to support optional JSON serialization.
- Simplify `StdIO` transport by using `io.Reader` and `io.Writer` interfaces instead of closeable variants.

### Removed

- Remove `Close` method from `StdIO` transport as part of I/O interface simplification.

## [0.4.2] - 2025-01-21

This release introduces a new capability for runtime server information retrieval through the `ServerInfo` method, enabling clients to dynamically access server configuration and capabilities without additional setup requirements.

### Added

- `ServerInfo` method to `Client` struct for retrieving server information and capabilities at runtime.

## [0.4.1] - 2025-01-19

This release implements audio content support as specified in the latest MCP specification update, adding necessary enum values and maintaining consistency with existing content type structures. The changes enable seamless integration of audio processing capabilities while preserving compatibility with existing image content handling patterns.

### Added

- `ContentTypeAudio` to `ContentType` enum.

## [0.4.0] - 2025-01-10

This release changes how clients handle server capabilities during initialization. Instead of requiring users to define server capabilities when creating a client, which proved impractical in real-world usage where MCP hosts often lack compile-time knowledge of server capabilities, the client can now connect to any server and dynamically check for desired capabilities. This is implemented through new methods like `PromptServerSupported` and `ResourceServerSupported`, allowing runtime verification of specific server features.

### Added

- Added `PromptServerSupported`, `ResourceServerSupported`, `ToolServerSupported`, and `LoggingServerSupported` methods to `Client` struct to check if the server supports specific features.
- Added more tests for `mcp` package.

### Removed

- `ServerRequirement` for creating a new client.

## [0.3.1] - 2025-01-07

This release focuses on adhering to the official JSON Schema specification and improving metadata management. Key changes include the introduction of unified resource content handling, enhanced annotation support, and simplified JSON handling by removing third-party schema validation dependencies. The changes in Tool's InputSchema and CallToolParams' Arguments provide more flexibility by allowing users to implement their own JSON Schema validation.

### Added

- `Annotations` struct for metadata management
- `ResourceContents` struct for unified text and blob handling
- `UnsubscribeResource` function in subscription handler
- `WithInstructions` server option for client guidance
- `String` method for `LogLevel` type

### Changed

- Rename `PromptRole` to `Role` for better clarity
- Enhance `Content` structure with new `Annotations` and `ResourceContent` fields
- Update resource handling to use `ResourceContent` for embedded resources
- Add pagination support with `Cursor` and `NextCursor` fields in template operations
- Add `Total` field to completion results for progress tracking
- Implement `Annotations` support across `Resource` and `ResourceTemplate` types
- Update `LogData` to use `json.RawMessage` for flexible data handling
- Change `InputSchema` field in `Tool` struct to use `json.RawMessage` type
- Change `Arguments` field in `CallToolParams` struct to use `json.RawMessage`
- Adjust `filesystem` and `everything` servers for current schema

### Removed

- Remove `Text` and `Blob` fields from `Resource` type (functionality moved to `ResourceContent`)

## [0.3.0] - 2025-01-05

This release introduces significant improvements in API consistency and modernizes the codebase by adopting Go 1.23's iterator pattern. Key changes include restructured parameter naming conventions, simplified package organization, enhanced transport interfaces, and improved SSE handling through the integration of `go-sse` library. The adoption of iterators for handling sessions, messages, and streaming operations provides a more efficient and safer alternative to channels for sequential data processing.

### Added

- `Session` interface for handling session on `ServerTransport` interfaces.
- `ProgressReporter` function type for reporting progress in server primitive operations
- `ResourceSubscriptionHandler` interface for handling resource subscriptions.

### Changed

- Refactored parameter naming convention for `Client` request methods to improve consistency between method names and their parameters. Previously, parameter names like `PromptsListParams` and `PromptsGetParams` used noun-verb style while methods used verb-noun style. Now, parameter names follow the same verb-noun pattern as their corresponding methods (e.g., `ListPromptsParams` and `GetPromptParams`).
- Refactored the result name of the request calls, either in `Client` or `Server` interfaces. This is done to improve consistency between method names and their results. For example, `ListPrompts` now returns `ListPromptsResult` instead of `PromptList`.
- Use structured parameter types (such as `ListPromptsParams` or `GetPromptParams`) in `Client` method signatures when making server requests, rather than using individual parameters. For example, instead of passing separate `cursor` and `progressToken` parameters to `ListPrompts`, or `name` and `arguments` to `GetPrompt`, use a dedicated parameter struct.
- Utilize `go-sse` to handle `SSEClient` by @tmaxmax.
- Utilize `go-sse`'s `Session` to sent the event messages from `SSEServer`.
- Moved `mcp` package from `pkg/mcp` to root folder and `pkg/servers` package to `servers` package to simplify import paths. The `pkg` directory added unnecessary nesting and noise in import paths (e.g., from `github.com/MegaGrindStone/go-mcp/pkg/mcp` to `github.com/MegaGrindStone/go-mcp`, and from `github.com/MegaGrindStone/go-mcp/pkg/servers` to `github.com/MegaGrindStone/go-mcp/servers`).
- Split `Transport` interface into `ServerTransport` and `ClientTransport` interfaces. `ClientTransport` doesn't need session-based messages as `Client` would just use one `Session` in its lifecycle.
- Use iterator pattern for sessions in `ServerTransport` and messages in `ClientTransport`.
- Use iterator pattern for streaming logs in `LogHandler`.
- Use iterator pattern for all the updaters. 
- Use `io.ReadCloser` and `io.WriteCloser` for `StdIO` transport.
- Adjust `everything` server for current interfaces.
- Adjust `filesystem` server for current interfaces.

### Removed

- `pkg/mcp` package (moved to root for cleaner imports).
- `pkg/servers` package (moved to `servers` for cleaner imports).
- `Transport` interface (replaced by `ServerTransport` and `ClientTransport`)
- `ProgressReporter` interface (replaced with function type for simpler progress reporting)
- `ResourceSubscribedUpdater` interface (replaced with `ResourceSubscriptionHandler`)

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
