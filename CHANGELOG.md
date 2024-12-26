# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
