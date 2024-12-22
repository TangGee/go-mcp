package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
)

// StdIOServer implements a filesystem server over standard I/O.
// It provides access to files and directories within a specified root path,
// enforcing path restrictions and implementing the mcp.Server interface.
//
// The server supports various filesystem operations through tools including:
//   - Reading and writing files
//   - Creating and listing directories
//   - Moving and searching files
//   - Getting file metadata
//
// All operations are restricted to paths within the specified root directory
// for security. Attempts to access paths outside the root will result in errors.
type StdIOServer struct {
	MCPServer mcp.StdIOServer
	rootPath  string
}

// NewStdIOServer creates a new filesystem server that operates over standard I/O.
// It validates that the provided root path exists and is a directory, then initializes
// a server instance with the given options. The server restricts all operations to
// paths within the specified root directory.
//
// The root parameter specifies the base directory path for all filesystem operations.
// The option parameter allows passing additional server configuration options.
//
// Returns the initialized server instance and any error encountered during setup.
// Returns error if root path doesn't exist or is not a directory.
func NewStdIOServer(root string, option ...mcp.ServerOption) (StdIOServer, error) {
	info, err := os.Stat(filepath.Clean(root))
	if err != nil {
		return StdIOServer{}, fmt.Errorf("failed to stat root directory: %w", err)
	}
	if !info.IsDir() {
		return StdIOServer{}, fmt.Errorf("root directory is not a directory: %s", root)
	}

	s := StdIOServer{
		rootPath: root,
	}

	// Insert tool server at the beginning of the option list, so user can override it
	option = slices.Insert(option, 0, mcp.WithToolServer(s))
	s.MCPServer = mcp.NewStdIOServer(s, option...)

	return s, nil
}

// Info implements mcp.Server interface.
// Returns metadata about the filesystem server implementation including
// server name and version information as defined in the mcp.Info struct.
func (s StdIOServer) Info() mcp.Info {
	return mcp.Info{
		Name:    "filesystem",
		Version: "1.0",
	}
}

// RequireSamplingClient implements mcp.Server interface.
// Indicates whether this server implementation requires a sampling-capable client.
// For filesystem operations, sampling is not required so this always returns false.
func (s StdIOServer) RequireSamplingClient() bool {
	return false
}

// ListTools implements mcp.ToolServer interface.
// Returns the list of available filesystem tools supported by this server.
// The tools provide various filesystem operations like reading, writing, and managing files.
//
// The ctx parameter provides context for the operation.
// The params parameter contains pagination and metadata for the listing operation.
//
// Returns a ToolList containing all available filesystem tools and any error encountered.
func (s StdIOServer) ListTools(_ context.Context, _ mcp.ToolsListParams) (mcp.ToolList, error) {
	return toolList, nil
}

// CallTool implements mcp.ToolServer interface.
// Executes a specified filesystem tool with the given parameters.
// All operations are restricted to paths within the server's root directory.
//
// The ctx parameter provides context for the operation.
// The params parameter contains the tool name to execute and its arguments.
//
// Returns the tool's execution result and any error encountered.
// Returns error if the tool is not found or if execution fails.
func (s StdIOServer) CallTool(ctx context.Context, params mcp.ToolsCallParams) (mcp.ToolResult, error) {
	switch params.Name {
	case "read_file":
		return readFile(ctx, s.rootPath, params)
	case "read_multiple_files":
		return readMultipleFiles(ctx, s.rootPath, params)
	case "write_file":
		return writeFile(ctx, s.rootPath, params)
	case "edit_file":
		return editFile(ctx, s.rootPath, params)
	case "create_directory":
		return createDirectory(ctx, s.rootPath, params)
	case "list_directory":
		return listDirectory(ctx, s.rootPath, params)
	case "directory_tree":
		return directoryTree(ctx, s.rootPath, params)
	case "move_file":
		return moveFile(ctx, s.rootPath, params)
	case "search_files":
		return searchFiles(ctx, s.rootPath, params)
	case "get_file_info":
		return getFileInfo(ctx, s.rootPath, params)
	default:
		return mcp.ToolResult{}, fmt.Errorf("tool not found: %s", params.Name)
	}
}
