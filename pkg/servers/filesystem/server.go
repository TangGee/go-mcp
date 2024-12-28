package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
)

// Server implements the Model Context Protocol (MCP) for filesystem operations. It provides
// access to the local filesystem through a restricted root directory, exposing standard
// filesystem operations as MCP tools.
//
// Server ensures all operations remain within the configured root directory path for security.
// It implements both the mcp.Server and mcp.ToolServer interfaces to provide filesystem
// functionality through the MCP protocol.
type Server struct {
	rootPath string
}

// NewServer creates a new filesystem MCP server that provides access to files under the specified root directory.
//
// The server validates that the root path exists and is an accessible directory. All filesystem operations
// are restricted to this directory and its subdirectories for security.
//
// It returns an error if the root path does not exist, is not a directory, or cannot be accessed.
func NewServer(root string) (Server, error) {
	info, err := os.Stat(filepath.Clean(root))
	if err != nil {
		return Server{}, fmt.Errorf("failed to stat root directory: %w", err)
	}
	if !info.IsDir() {
		return Server{}, fmt.Errorf("root directory is not a directory: %s", root)
	}

	s := Server{
		rootPath: root,
	}

	return s, nil
}

// Info implements mcp.Server interface.
func (s Server) Info() mcp.Info {
	return mcp.Info{
		Name:    "filesystem",
		Version: "1.0",
	}
}

// RequireRootsListClient implements mcp.Server interface.
func (s Server) RequireRootsListClient() bool {
	return false
}

// RequireSamplingClient implements mcp.Server interface.
func (s Server) RequireSamplingClient() bool {
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
func (s Server) ListTools(context.Context, mcp.ListToolsParams, mcp.RequestClientFunc) (mcp.ToolList, error) {
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
func (s Server) CallTool(
	ctx context.Context,
	params mcp.CallToolParams,
	_ mcp.RequestClientFunc,
) (mcp.ToolResult, error) {
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
