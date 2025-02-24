package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MegaGrindStone/go-mcp"
)

// Server implements the Model Context Protocol (MCP) for filesystem operations. It provides
// access to the local filesystem through a restricted root directory, exposing standard
// filesystem operations as MCP tools.
//
// Server ensures all operations remain within the configured root directory path for security.
// It implements both the mcp.Server and mcp.ToolServer interfaces to provide filesystem
// functionality through the MCP protocol.
type Server struct {
	rootPaths []string
}

// NewServer creates a new filesystem MCP server that provides access to files under the specified root directory.
//
// The server validates that the root path exists and is an accessible directory. All filesystem operations
// are restricted to this directory and its subdirectories for security.
//
// It returns an error if the root path does not exist, is not a directory, or cannot be accessed.
func NewServer(roots []string) (Server, error) {
	for _, root := range roots {
		info, err := os.Stat(filepath.Clean(root))
		if err != nil {
			return Server{}, fmt.Errorf("failed to stat root directory: %w", err)
		}
		if !info.IsDir() {
			return Server{}, fmt.Errorf("root directory is not a directory: %s", root)
		}
	}

	s := Server{
		rootPaths: roots,
	}

	return s, nil
}

// ListTools implements mcp.ToolServer interface.
// Returns the list of available filesystem tools supported by this server.
// The tools provide various filesystem operations like reading, writing, and managing files.
//
// Returns a ToolList containing all available filesystem tools and any error encountered.
func (s Server) ListTools(
	context.Context,
	mcp.ListToolsParams,
	mcp.ProgressReporter,
	mcp.RequestClientFunc,
) (mcp.ListToolsResult, error) {
	return toolList, nil
}

// CallTool implements mcp.ToolServer interface.
// Executes a specified filesystem tool with the given parameters.
// All operations are restricted to paths within the server's root directory.
//
// Returns the tool's execution result and any error encountered.
// Returns error if the tool is not found or if execution fails.
func (s Server) CallTool(
	_ context.Context,
	params mcp.CallToolParams,
	_ mcp.ProgressReporter,
	_ mcp.RequestClientFunc,
) (mcp.CallToolResult, error) {
	switch params.Name {
	case "read_file":
		return readFile(s.rootPaths, params)
	case "read_multiple_files":
		return readMultipleFiles(s.rootPaths, params)
	case "write_file":
		return writeFile(s.rootPaths, params)
	case "edit_file":
		return editFile(s.rootPaths, params)
	case "create_directory":
		return createDirectory(s.rootPaths, params)
	case "list_directory":
		return listDirectory(s.rootPaths, params)
	case "directory_tree":
		return directoryTree(s.rootPaths, params)
	case "move_file":
		return moveFile(s.rootPaths, params)
	case "search_files":
		return searchFiles(s.rootPaths, params)
	case "get_file_info":
		return getFileInfo(s.rootPaths, params)
	case "list_allowed_directories":
		return listAllowedDirectories(s.rootPaths, params)
	default:
		return mcp.CallToolResult{}, fmt.Errorf("tool not found: %s", params.Name)
	}
}
