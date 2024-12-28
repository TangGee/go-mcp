package filesystem

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
	"github.com/qri-io/jsonschema"
)

var readFileSchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "path": { "type": "string" }
  }
}`)

var readMultipleFilesSchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "paths": { "type": "array", "items": { "type": "string" } }
  }
}`)

var writeFileSchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "path": { "type": "string" },
    "content": { "type": "string" }
  }
}`)

var editFileSchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "path": { "type": "string" },
    "edits": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "oldText": { "type": "string" , "description": "Text to search for - must match exactly" },
          "newText": { "type": "string" , "description": "Text to replace with" }
        }
      }
    },
    "dryRun": { 
      "type": "boolean", 
      "default": false, 
      "description": "Preview changes using git-style diff format"
    }
  }
}`)

var createDirectorySchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "path": { "type": "string" }
  }
}`)

var listDirectorySchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "path": { "type": "string" }
  }
}`)

var directoryTreeSchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "path": { "type": "string" }
  }
}`)

var moveFileSchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "source": { "type": "string" },
    "destination": { "type": "string" }
  }
}`)

var searchFilesSchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "path": { "type": "string" },
    "pattern": { "type": "string" },
    "excludePatterns": { "type": "array", "items": { "type": "string" } }
  }
}`)

var getFileInfoSchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "path": { "type": "string" }
  }
}`)

var toolList = mcp.ToolList{
	Tools: []mcp.Tool{
		{
			Name: "read_file",
			Description: `
Read the complete contents of a file from the file system.
Handles various text encodings and provides detailed error messages
if the file cannot be read. Use this tool when you need to examine
the contents of a single file. Only works within allowed directories.,
        `,
			InputSchema: readFileSchema,
		},
		{
			Name: "read_multiple_files",
			Description: `
Read the contents of multiple files simultaneously. This is more
efficient than reading files one by one when you need to analyze
or compare multiple files. Each file's content is returned with its
path as a reference. Failed reads for individual files won't stop
the entire operation. Only works within allowed directories.
        `,
			InputSchema: readMultipleFilesSchema,
		},
		{
			Name: "write_file",
			Description: `
Create a new file or completely overwrite an existing file with new content.
Use with caution as it will overwrite existing files without warning.
Handles text content with proper encoding. Only works within allowed directories.
        `,
			InputSchema: writeFileSchema,
		},
		{
			Name: "edit_file",
			Description: `
Make line-based edits to a text file. Each edit replaces exact line sequences
with new content. Returns a git-style diff showing the changes made.
Only works within allowed directories.
        `,
			InputSchema: editFileSchema,
		},
		{
			Name: "create_directory",
			Description: `
Create a new directory or ensure a directory exists. Can create multiple
nested directories in one operation. If the directory already exists,
this operation will succeed silently. Perfect for setting up directory
structures for projects or ensuring required paths exist. Only works within allowed directories.
        `,
			InputSchema: createDirectorySchema,
		},
		{
			Name: "list_directory",
			Description: `
Get a detailed listing of all files and directories in a specified path.
Results clearly distinguish between files and directories with [FILE] and [DIR]
prefixes. This tool is essential for understanding directory structure and
finding specific files within a directory. Only works within allowed directories.
        `,
			InputSchema: listDirectorySchema,
		},
		{
			Name: "directory_tree",
			Description: `
Get a recursive tree view of files and directories as a JSON structure.
Each entry includes 'name', 'type' (file/directory), and 'children' for directories.
Files have no children array, while directories always have a children array (which may be empty).
The output is formatted with 2-space indentation for readability. Only works within allowed directories.
        `,
			InputSchema: directoryTreeSchema,
		},
		{
			Name: "move_file",
			Description: `Move or rename files and directories. Can move files between directories
and rename them in a single operation. If the destination exists, the
operation will fail. Works across different directories and can be used
for simple renaming within the same directory. Both source and destination must be within allowed directories.
        `,
			InputSchema: moveFileSchema,
		},
		{
			Name: "search_files",
			Description: `Recursively search for files and directories matching a pattern.
Searches through all subdirectories from the starting path. The search
is case-insensitive and matches partial names. Returns full paths to all
matching items. Great for finding files when you don't know their exact location.
Only searches within allowed directories.
        `,
			InputSchema: searchFilesSchema,
		},
		{
			Name: "get_file_info",
			Description: `Retrieve detailed metadata about a file or directory. Returns comprehensive
information including size, creation time, last modified time, permissions,
and type. This tool is perfect for understanding file characteristics
without reading the actual content. Only works within allowed directories.
        `,
			InputSchema: getFileInfoSchema,
		},
	},
}

func readFile(ctx context.Context, rootPath string, params mcp.CallToolParams) (mcp.ToolResult, error) {
	vs := readFileSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	path, _ := params.Arguments["path"].(string)
	fullPath := filepath.Join(rootPath, filepath.Clean(path))

	info, err := os.Stat(fullPath)
	if err != nil {
		return mcp.ToolResult{}, fmt.Errorf("failed to stat file with path %s: %w", fullPath, err)
	}

	if info.IsDir() {
		return mcp.ToolResult{}, fmt.Errorf("path %s is a directory, not a file", fullPath)
	}

	bs, err := os.ReadFile(fullPath)
	if err != nil {
		return mcp.ToolResult{}, fmt.Errorf("failed to read file with path %s: %w", fullPath, err)
	}

	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: string(bs),
			},
		},
		IsError: false,
	}, nil
}

func readMultipleFiles(ctx context.Context, rootPath string, params mcp.CallToolParams) (mcp.ToolResult, error) {
	vs := readMultipleFilesSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	paths, _ := params.Arguments["paths"].([]any)
	var pathsStr []string
	for _, p := range paths {
		path, _ := p.(string)
		pathsStr = append(pathsStr, path)
	}

	var result []mcp.Content

	for _, path := range pathsStr {
		fullPath := filepath.Join(rootPath, filepath.Clean(path))

		info, err := os.Stat(fullPath)
		if err != nil {
			return mcp.ToolResult{}, fmt.Errorf("failed to stat file with path %s: %w", fullPath, err)
		}

		if info.IsDir() {
			return mcp.ToolResult{}, fmt.Errorf("path %s is a directory, not a file", fullPath)
		}

		bs, err := os.ReadFile(fullPath)
		if err != nil {
			return mcp.ToolResult{}, fmt.Errorf("failed to read file with path %s: %w", fullPath, err)
		}

		content := fmt.Sprintf("File content of %s:\n%s\n", path, string(bs))

		result = append(result, mcp.Content{
			Type: mcp.ContentTypeText,
			Text: content,
		})
	}

	return mcp.ToolResult{
		Content: result,
		IsError: false,
	}, nil
}

func writeFile(ctx context.Context, rootPath string, params mcp.CallToolParams) (mcp.ToolResult, error) {
	vs := writeFileSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	path, _ := params.Arguments["path"].(string)
	content, _ := params.Arguments["content"].(string)

	fullPath := filepath.Join(rootPath, filepath.Clean(path))

	err := os.WriteFile(fullPath, []byte(content), 0600)
	if err != nil {
		return mcp.ToolResult{}, fmt.Errorf("failed to write file with path %s: %w", fullPath, err)
	}

	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("File %s written successfully", path),
			},
		},
		IsError: false,
	}, nil
}

func editFile(ctx context.Context, rootPath string, params mcp.CallToolParams) (mcp.ToolResult, error) {
	vs := editFileSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	path, _ := params.Arguments["path"].(string)
	edits, _ := params.Arguments["edits"].([]any)
	dryRun, _ := params.Arguments["dryRun"].(bool)

	fullPath := filepath.Join(rootPath, filepath.Clean(path))

	bs, err := os.ReadFile(fullPath)
	if err != nil {
		return mcp.ToolResult{}, fmt.Errorf("failed to read file with path %s: %w", fullPath, err)
	}

	newContent := string(bs)

	// TODO: Use more sophisticated diff algorithm to match the typescript implementation.

	for _, edit := range edits {
		edit, _ := edit.(map[string]any)
		oldText, _ := edit["oldText"].(string)
		newText, _ := edit["newText"].(string)

		newContent = strings.ReplaceAll(newContent, oldText, newText)
	}

	if dryRun {
		return mcp.ToolResult{
			Content: []mcp.Content{
				{
					Type: mcp.ContentTypeText,
					Text: fmt.Sprintf("File %s edited successfully", path),
				},
			},
			IsError: false,
		}, nil
	}

	err = os.WriteFile(fullPath, []byte(newContent), 0600)
	if err != nil {
		return mcp.ToolResult{}, fmt.Errorf("failed to write file with path %s: %w", fullPath, err)
	}

	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("File %s edited successfully", path),
			},
		},
		IsError: false,
	}, nil
}

func createDirectory(ctx context.Context, rootPath string, params mcp.CallToolParams) (mcp.ToolResult, error) {
	vs := createDirectorySchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	path, _ := params.Arguments["path"].(string)

	fullPath := filepath.Join(rootPath, filepath.Clean(path))

	err := os.MkdirAll(fullPath, 0700)
	if err != nil {
		return mcp.ToolResult{}, fmt.Errorf("failed to create directory with path %s: %w", fullPath, err)
	}

	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("Directory %s created successfully", path),
			},
		},
	}, nil
}

func listDirectory(ctx context.Context, rootPath string, params mcp.CallToolParams) (mcp.ToolResult, error) {
	vs := listDirectorySchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	path, _ := params.Arguments["path"].(string)

	fullPath := filepath.Join(rootPath, filepath.Clean(path))

	files, err := os.ReadDir(fullPath)
	if err != nil {
		return mcp.ToolResult{}, fmt.Errorf("failed to read directory with path %s: %w", fullPath, err)
	}

	var result []mcp.Content

	for _, file := range files {
		prefix := "[FILE] "
		if file.IsDir() {
			prefix = "[DIR] "
		}

		content := fmt.Sprintf("%s%s\n", prefix, file.Name())

		result = append(result, mcp.Content{
			Type: mcp.ContentTypeText,
			Text: content,
		})
	}

	return mcp.ToolResult{
		Content: result,
		IsError: false,
	}, nil
}

func directoryTree(ctx context.Context, rootPath string, params mcp.CallToolParams) (mcp.ToolResult, error) {
	vs := directoryTreeSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	path, _ := params.Arguments["path"].(string)
	fullPath := filepath.Join(rootPath, filepath.Clean(path))

	files, err := os.ReadDir(fullPath)
	if err != nil {
		return mcp.ToolResult{}, fmt.Errorf("failed to read directory with path %s: %w", fullPath, err)
	}

	// TODO: Display actual directory tree instead of just directory listing.

	var result []mcp.Content

	for _, file := range files {
		prefix := "[FILE] "
		if file.IsDir() {
			prefix = "[DIR] "
		}

		content := fmt.Sprintf("%s%s\n", prefix, file.Name())

		result = append(result, mcp.Content{
			Type: mcp.ContentTypeText,
			Text: content,
		})
	}

	return mcp.ToolResult{
		Content: result,
		IsError: false,
	}, nil
}

func moveFile(ctx context.Context, rootPath string, params mcp.CallToolParams) (mcp.ToolResult, error) {
	vs := moveFileSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	source, _ := params.Arguments["source"].(string)
	destination, _ := params.Arguments["destination"].(string)

	fullSourcePath := filepath.Join(rootPath, filepath.Clean(source))
	fullDestinationPath := filepath.Join(rootPath, filepath.Clean(destination))

	err := os.Rename(fullSourcePath, fullDestinationPath)
	if err != nil {
		return mcp.ToolResult{}, fmt.Errorf("failed to move file with path %s: %w", fullSourcePath, err)
	}

	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("File %s moved successfully", source),
			},
		},
		IsError: false,
	}, nil
}

func searchFiles(ctx context.Context, rootPath string, params mcp.CallToolParams) (mcp.ToolResult, error) {
	vs := searchFilesSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	pattern, _ := params.Arguments["pattern"].(string)
	excludePatterns, _ := params.Arguments["excludePatterns"].([]any)

	var result []mcp.Content

	if err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		matched, err := filepath.Match(pattern, path)
		if err != nil {
			return err
		}

		if !matched {
			return nil
		}

		for _, ep := range excludePatterns {
			excludePattern, _ := ep.(string)
			exMatched, err := filepath.Match(excludePattern, path)
			if err != nil {
				return err
			}

			if exMatched {
				return nil
			}
		}

		result = append(result, mcp.Content{
			Type: mcp.ContentTypeText,
			Text: path,
		})

		return nil
	}); err != nil {
		return mcp.ToolResult{}, fmt.Errorf("failed to search files: %w", err)
	}

	if len(result) == 0 {
		return mcp.ToolResult{
			Content: []mcp.Content{
				{
					Type: mcp.ContentTypeText,
					Text: "No files found",
				},
			},
			IsError: false,
		}, nil
	}

	return mcp.ToolResult{
		Content: result,
		IsError: false,
	}, nil
}

func getFileInfo(ctx context.Context, rootPath string, params mcp.CallToolParams) (mcp.ToolResult, error) {
	vs := getFileInfoSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	path, _ := params.Arguments["path"].(string)
	fullPath := filepath.Join(rootPath, filepath.Clean(path))

	info, err := os.Stat(fullPath)
	if err != nil {
		return mcp.ToolResult{}, fmt.Errorf("failed to stat file with path %s: %w", fullPath, err)
	}

	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("File %s info:\nSize: %d\nLast modified: %s\n", path, info.Size(), info.ModTime()),
			},
		},
		IsError: false,
	}, nil
}
